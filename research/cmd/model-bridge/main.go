// Command model-bridge exposes the pinned Go implementation over JSONL.
// It never executes input text and never falls back after inference errors.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"runtime/debug"

	model "github.com/saifety-org/prompt-injection-model"
	"github.com/saifety-org/sAIfety/pkg/inference"
	"github.com/saifety-org/sAIfety/pkg/scanner"
)

type request struct {
	Text *string `json:"text"`
}
type response struct {
	Tokens          *int             `json:"tokens,omitempty"`
	TextSHA256      string           `json:"text_sha256"`
	Features        map[int]float64  `json:"features,omitempty"`
	Score           *float64         `json:"score,omitempty"`
	Verdict         *scanner.Verdict `json:"verdict,omitempty"`
	Dim             int              `json:"dim"`
	FeatureSchema   string           `json:"feature_schema"`
	ModelModule     string           `json:"model_module"`
	WeightsSHA256   string           `json:"weights_sha256"`
	TokenizerSHA256 string           `json:"tokenizer_sha256,omitempty"`
	RuntimeSHA256   string           `json:"runtime_sha256,omitempty"`
	AttackIndex     *int             `json:"attack_index,omitempty"`
}

func version() string {
	if b, ok := debug.ReadBuildInfo(); ok {
		for _, d := range b.Deps {
			if d.Path == "github.com/saifety-org/prompt-injection-model" {
				if d.Replace != nil {
					return "local"
				}
				return d.Version
			}
		}
	}
	return "unknown"
}
func loadWeights(path string) (*model.Model, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	m, err := model.Load(b)
	if err != nil {
		return nil, "", err
	}
	if m.Dim != model.Dim || len(m.Weights) != model.Dim || math.IsNaN(m.Bias) || math.IsInf(m.Bias, 0) {
		return nil, "", fmt.Errorf("invalid model dimensions or bias")
	}
	for _, v := range m.Weights {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, "", fmt.Errorf("non-finite model weight")
		}
	}
	return m, fmt.Sprintf("%x", sha256.Sum256(b)), nil
}
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	// Read-only close cannot invalidate the bytes already hashed.
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
func run(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("model-bridge", flag.ContinueOnError)
	mode := fs.String("mode", "features", "features | score | scan | tokens")
	weights := fs.String("weights", "", "explicit native candidate JSON; empty uses shipped model")
	backend := fs.String("backend", "native", "native | onnx (requires -tags onnx and explicit/cached artifacts)")
	library := fs.String("library", "", "ONNX runtime library")
	onnxModel := fs.String("model", "", "ONNX model path")
	tokenizer := fs.String("tokenizer", "", "tokenizer.json path")
	attackIndex := fs.Int("attack-index", 1, "attack output index")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *mode != "features" && *mode != "score" && *mode != "scan" && *mode != "tokens" {
		return fmt.Errorf("unknown mode %q", *mode)
	}
	if *backend != "native" && *backend != "onnx" {
		return fmt.Errorf("unknown backend %q", *backend)
	}
	if *backend == "onnx" && ((*mode != "score" && *mode != "tokens") || *weights != "") {
		return fmt.Errorf("ONNX is supported only in checked score/tokens modes, without native weights")
	}
	var scorer inference.Scorer
	hash := inference.EmbeddedSHA256()
	var tokenizerHash, runtimeHash string
	var resolvedAttackIndex *int
	if *backend == "native" {
		m, err := model.Default()
		if err != nil {
			return err
		}
		if *weights != "" {
			m, hash, err = loadWeights(*weights)
			if err != nil {
				return err
			}
		}
		scorer = m
	} else {
		var cfg inference.ONNXConfig
		if *onnxModel == "" && *tokenizer == "" && *library == "" {
			var err error
			cfg, err = inference.CachedONNXConfig()
			if err != nil {
				return err
			}
		} else {
			cfg = inference.ONNXConfig{LibraryPath: *library, ModelPath: *onnxModel, TokenizerPath: *tokenizer, InjectionIndex: *attackIndex}
		}
		if cfg.InjectionIndex < 0 || cfg.InjectionIndex > 1 {
			return fmt.Errorf("attack index must be 0 or 1")
		}
		var err error
		hash, err = fileHash(cfg.ModelPath)
		if err != nil {
			return err
		}
		tokenizerHash, err = fileHash(cfg.TokenizerPath)
		if err != nil {
			return err
		}
		runtimeHash, err = fileHash(cfg.LibraryPath)
		if err != nil {
			return err
		}
		resolvedAttackIndex = &cfg.InjectionIndex
		scorer, err = inference.NewONNX(cfg)
		if err != nil {
			return err
		}
	}
	sc, err := scanner.New(scanner.Config{Classifier: scorer, Policy: "strict"})
	if err != nil {
		return err
	}
	input := bufio.NewScanner(in)
	input.Buffer(make([]byte, 4096), 4<<20)
	enc := json.NewEncoder(out)
	for line := 1; input.Scan(); line++ {
		var r request
		if err := json.Unmarshal(input.Bytes(), &r); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if r.Text == nil {
			return fmt.Errorf("line %d: missing text", line)
		}
		res := response{TextSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(*r.Text))), Dim: model.Dim, FeatureSchema: model.FeatureSchema, ModelModule: version(), WeightsSHA256: hash}
		if *backend == "onnx" {
			res.Dim = 0
			res.FeatureSchema = "onnx-input-ids-v1"
			res.ModelModule = "external-onnx"
			res.TokenizerSHA256 = tokenizerHash
			res.RuntimeSHA256 = runtimeHash
			res.AttackIndex = resolvedAttackIndex
		}
		switch *mode {
		case "features":
			res.Features = model.Features(*r.Text)
		case "tokens":
			c, ok := scorer.(interface{ TokenCount(string) (int, error) })
			if !ok {
				return fmt.Errorf("token counting requires ONNX")
			}
			n, err := c.TokenCount(*r.Text)
			if err != nil {
				return err
			}
			res.Tokens = &n
		case "score":
			var score float64
			if c, ok := scorer.(interface {
				ScoreChecked(string) (float64, error)
				TokenCount(string) (int, error)
			}); ok {
				n, err := c.TokenCount(*r.Text)
				if err != nil {
					return err
				}
				if n > 512 {
					return fmt.Errorf("line %d: exceeds shared 512-token window", line)
				}
				score, err = c.ScoreChecked(*r.Text)
				if err != nil {
					return err
				}
			} else {
				score = scorer.Score(*r.Text)
			}
			if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > 1 {
				return fmt.Errorf("line %d: invalid score", line)
			}
			res.Score = &score
		case "scan":
			v := sc.Scan(ctx, &scanner.Document{Source: "lab-py/fixture", Kind: scanner.KindToolResult, Raw: *r.Text})
			res.Verdict = &v
		}
		if err := enc.Encode(res); err != nil {
			return err
		}
	}
	return input.Err()
}
func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
