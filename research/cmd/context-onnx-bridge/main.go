//go:build onnx

// Command context-onnx-bridge performs checked, resident research inference.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/saifety-org/lab-py/research/internal/e5tokenizer"
	"github.com/saifety-org/prompt-injection-model/bundle"
	"github.com/sugarme/tokenizer"
	ort "github.com/yalue/onnxruntime_go"
)

func hash(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	h := sha256.New()
	_, e = io.Copy(h, f)
	ce := f.Close()
	if e != nil {
		return "", e
	}
	if ce != nil {
		return "", ce
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
func infer(session *ort.DynamicAdvancedSession, ids []int) (float64, error) {
	shape := ort.NewShape(1, int64(len(ids)))
	values := make([]int64, len(ids))
	mask := make([]int64, len(ids))
	for i, id := range ids {
		values[i] = int64(id)
		mask[i] = 1
	}
	a, e := ort.NewTensor(shape, values)
	if e != nil {
		return 0, e
	}
	defer func() { _ = a.Destroy() }()
	b, e := ort.NewTensor(shape, mask)
	if e != nil {
		return 0, e
	}
	defer func() { _ = b.Destroy() }()
	c, e := ort.NewEmptyTensor[float32](ort.NewShape(1, 2))
	if e != nil {
		return 0, e
	}
	defer func() { _ = c.Destroy() }()
	if e = session.Run([]ort.Value{a, b}, []ort.Value{c}); e != nil {
		return 0, e
	}
	logits := c.GetData()
	for _, v := range logits {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return 0, fmt.Errorf("nonfinite logits")
		}
	}
	difference := float64(logits[0]) - float64(logits[1])
	score := 1 / (1 + math.Exp(difference))
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0, fmt.Errorf("nonfinite logits")
	}
	return score, nil
}
func run() (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("context inference panic: %v", p)
		}
	}()
	dir := flag.String("bundle", "", "verified bundle directory")
	library := flag.String("library", "", "ONNX runtime library")
	mode := flag.String("mode", "score", "score | tokens")
	flag.Parse()
	if *mode != "score" && *mode != "tokens" {
		return fmt.Errorf("unsupported mode")
	}
	m, e := bundle.Load(*dir)
	if e != nil {
		return e
	}
	tk, e := e5tokenizer.Load(filepath.Join(*dir, "tokenizer.json"))
	if e != nil {
		return e
	}
	var session *ort.DynamicAdvancedSession
	runtimeHash := ""
	runtimeVersion := ""
	if *mode == "score" {
		runtimeHash, e = hash(*library)
		if e != nil {
			return e
		}
		ort.SetSharedLibraryPath(*library)
		if e = ort.InitializeEnvironment(); e != nil {
			return e
		}
		defer func() { _ = ort.DestroyEnvironment() }()
		runtimeVersion = ort.GetVersion()
		options, e := ort.NewSessionOptions()
		if e != nil {
			return e
		}
		defer func() { _ = options.Destroy() }()
		if e = options.SetIntraOpNumThreads(1); e != nil {
			return e
		}
		if e = options.SetInterOpNumThreads(1); e != nil {
			return e
		}
		session, e = ort.NewDynamicAdvancedSession(filepath.Join(*dir, "model.onnx"), m.Contract.Inputs, m.Contract.Outputs, options)
		if e != nil {
			return e
		}
		defer func() { _ = session.Destroy() }()
	}
	manifestHash, e := hash(filepath.Join(*dir, "manifest.json"))
	if e != nil {
		return e
	}
	out := json.NewEncoder(os.Stdout)
	if e = out.Encode(map[string]any{"ready": true, "manifest_sha256": manifestHash, "runtime_sha256": runtimeHash, "threads": 1, "runtime_version": runtimeVersion}); e != nil {
		return e
	}
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 4096), 4<<20)
	for in.Scan() {
		var req struct {
			Text *string `json:"text"`
		}
		if e = json.Unmarshal(in.Bytes(), &req); e != nil {
			return e
		}
		if req.Text == nil {
			return fmt.Errorf("missing text")
		}
		text, e := m.Input(*req.Text)
		if e != nil {
			return e
		}
		encoded, e := tk.Encode(tokenizer.NewSingleEncodeInput(tokenizer.NewInputSequence(text)), true)
		if e != nil {
			return e
		}
		if encoded == nil || len(encoded.Ids) == 0 {
			return fmt.Errorf("empty tokenization")
		}
		reply := map[string]any{"text_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(*req.Text))), "input_ids": encoded.Ids, "tokens": len(encoded.Ids)}
		if *mode == "score" {
			if len(encoded.Ids) > m.Contract.MaxTokens {
				return fmt.Errorf("input exceeds 512-token window; no truncation")
			}
			score, e := infer(session, encoded.Ids)
			if e != nil {
				return e
			}
			reply["score"] = score
			reply["decision"] = score >= m.Calibration.Threshold
		}
		if e = out.Encode(reply); e != nil {
			return e
		}
	}
	return in.Err()
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
