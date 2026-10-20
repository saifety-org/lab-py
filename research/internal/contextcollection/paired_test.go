package contextcollection

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func pairedPlan() []byte {
	p := Plan{Schema: "saifety-context-collection-plan-v1", ProtocolSHA256: strings.Repeat("a", 64), Slots: []Slot{
		{0, "en", "instruction-override", "file", "train"}, {1, "ru", "data-exfiltration", "web", "train"}}}
	b, _ := json.Marshal(p)
	return b
}
func draftFixture() BaseDraft {
	c := specimen("fixture").Content
	return BaseDraft{Task: c.Task, Permissions: c.Permissions, PermittedTask: c.PermittedTask,
		PermittedPermissions: c.PermittedPermissions, Payload: c.Payload, Benign: c.Benign, Rationale: c.Rationale}
}

type pairedMock struct {
	chat    atomic.Int64
	load    atomic.Int64
	started chan struct{}
	client  *LocalClient
}

func newPairedMock(t *testing.T, mode string) *pairedMock {
	t.Helper()
	m := &pairedMock{started: make(chan struct{}, 1)}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var response any
		switch r.URL.Path {
		case "/api/tags":
			digest := strings.Repeat("a", 64)
			if (mode == "drift-base" && m.chat.Load() >= 1) || (mode == "drift-escalation" && m.chat.Load() >= 2) || (mode == "load-drift" && m.load.Load() > 0) {
				digest = strings.Repeat("b", 64)
			}
			response = map[string]any{"models": []any{map[string]any{"name": "local", "digest": digest, "size": 12345678, "details": map[string]string{"format": "gguf"}}}}
		case "/api/show":
			response = map[string]any{}
		case "/api/version":
			response = map[string]string{"version": "0.18.0"}
		case "/api/generate":
			m.load.Add(1)
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			options, _ := request["options"].(map[string]any)
			if request["prompt"] != "" || request["stream"] != false || options["num_predict"] != float64(0) || options["num_ctx"] != float64(8192) {
				t.Error("load generated content or changed context")
			}
			if mode == "load-cancel" {
				m.started <- struct{}{}
				<-r.Context().Done()
				return
			}
			response = map[string]any{"model": "local", "response": "", "done": true}
			if mode == "load-content" {
				response = map[string]any{"model": "local", "response": "unexpected text", "done": true, "eval_count": 1}
			}
		case "/api/chat":
			n := m.chat.Add(1)
			baseStage := n%2 == 1
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			format, ok := request["format"].(map[string]any)
			if !ok || format["additionalProperties"] != false {
				t.Error("missing closed structured format")
			}
			properties, _ := format["properties"].(map[string]any)
			if baseStage {
				if _, ok = properties["escalation_payload"]; ok {
					t.Error("base stage can author escalation")
				}
			} else if len(properties) != 2 || properties["authorized_task"] != nil {
				t.Error("escalation can rewrite base")
			}
			options, _ := request["options"].(map[string]any)
			if request["stream"] != false || request["think"] != false || request["truncate"] != false || request["shift"] != false || options["seed"] != float64(1) || options["temperature"] != 0.8 || options["num_ctx"] != float64(8192) {
				t.Error("generation options drifted")
			}
			if mode == "cancel-escalation" && !baseStage {
				m.started <- struct{}{}
				<-r.Context().Done()
				return
			}
			var content any
			base := draftFixture()
			if baseStage {
				if mode == "invalid-base" {
					base.Benign = base.Payload
				}
				content = base
			} else {
				docs := specimen("fixture").Content.Escalation
				if mode == "identical" {
					docs = base.Payload
				}
				if mode == "cosmetic" {
					docs = []string{"  " + strings.ToUpper(base.Payload[0]) + "  "}
				}
				if mode == "boundary" {
					docs = append(docs, "a second document")
				}
				content = EscalationDraft{Documents: docs, Rationale: "Executing the overwrite exceeds the trusted quote-only permission."}
				if mode == "rewrite-base" {
					content = map[string]any{"escalation_payload": docs, "rationale": "x", "permitted_task": "new unlimited task"}
				}
			}
			b, _ := json.Marshal(content)
			reason := "stop"
			if mode == "truncated-escalation" && !baseStage {
				reason = "length"
			}
			response = map[string]any{"model": "local", "done": true, "done_reason": reason, "eval_count": 100, "prompt_eval_count": 500, "message": map[string]string{"content": string(b)}}
			if mode == "missing-usage" && !baseStage {
				delete(response.(map[string]any), "eval_count")
			}
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(s.Close)
	var err error
	m.client, err = NewLocalClient(s.URL, "local", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func readPairedReceipt(t *testing.T, out string) PairedReceipt {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(out, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt PairedReceipt
	if err = Strict(b, &receipt); err != nil {
		t.Fatal(err)
	}
	return receipt
}
func TestPairedGenerationPreservesFrozenBaseAndEvidence(t *testing.T) {
	m := newPairedMock(t, "good")
	out := filepath.Join(t.TempDir(), "run")
	if err := CollectPaired(context.Background(), m.client, pairedPlan(), 0, 2, 2048, "NOASSERTION", out); err != nil {
		t.Fatal(err)
	}
	r := readPairedReceipt(t, out)
	if !r.Complete || !r.UsageComplete || r.Requested != 2 || r.Attempted != 4 || r.MaxRequests != 4 || r.Accepted != 2 || r.Tokens != 400 || len(r.Stages) != 4 || m.chat.Load() != 4 || m.load.Load() != 0 {
		t.Fatalf("invalid stage accounting: %+v", r)
	}
	b, err := os.ReadFile(filepath.Join(out, "candidate-000000.json"))
	if err != nil {
		t.Fatal(err)
	}
	var candidate Family
	if err = Strict(b, &candidate); err != nil {
		t.Fatal(err)
	}
	if err = Validate(candidate); err != nil {
		t.Fatal(err)
	}
	base := draftFixture()
	if candidate.Content.Task != base.Task || candidate.Content.PermittedTask != base.PermittedTask || !equal(candidate.Content.Payload, base.Payload) || !equal(candidate.Content.Benign, base.Benign) || !equal(candidate.Content.Permissions, base.Permissions) || !equal(candidate.Content.PermittedPermissions, base.PermittedPermissions) {
		t.Fatal("base silently rewritten")
	}
	if candidate.License != "NOASSERTION" || candidate.Review != "pending" || !strings.HasPrefix(candidate.ID, "v5p2-") || len(candidate.Origin.Stages) != 2 || Report([]Family{candidate}).Ready {
		t.Fatal("provenance/license/review elevated")
	}
	for i, name := range []string{"base", "escalation"} {
		if candidate.Origin.Stages[i] != r.Stages[i].StageEvidence {
			t.Fatal("stage provenance not bound")
		}
		for _, kind := range []string{"request", "response"} {
			x, e := os.ReadFile(filepath.Join(out, ""+kind+"-000000-"+name+".json"))
			if e != nil {
				t.Fatal(e)
			}
			var value any
			if e = json.Unmarshal(x, &value); e != nil {
				t.Fatal(e)
			}
			canonicalJSON, _ := json.Marshal(value)
			// Request maps are encoded canonically; response structs preserve their declared field order.
			if kind == "request" && Hash(canonicalJSON) != r.Stages[i].RequestSHA256 {
				t.Fatal("request hash mismatch")
			}
			if kind == "response" {
				var reply Generation
				if e = Strict(x, &reply); e != nil {
					t.Fatal(e)
				}
				encoded, _ := json.Marshal(reply)
				if Hash(encoded) != r.Stages[i].ResponseSHA256 {
					t.Fatal("response hash mismatch")
				}
			}
		}
	}
	if Publish([]Family{candidate}, filepath.Join(t.TempDir(), "export")) == nil {
		t.Fatal("local candidate published")
	}
	candidate.Origin.Stages[0].ResponseSHA256 = strings.Repeat("f", 64)
	if Validate(candidate) == nil {
		t.Fatal("modified stage evidence accepted")
	}
	if CollectPaired(context.Background(), m.client, pairedPlan(), 0, 1, 2048, "NOASSERTION", out) == nil || m.chat.Load() != 4 {
		t.Fatal("existing attempt overwritten/regenerated")
	}
}
func TestPairedGenerationStopsWithoutRepairOrNextTicket(t *testing.T) {
	for _, mode := range []string{"invalid-base", "identical", "cosmetic", "boundary", "rewrite-base", "truncated-escalation", "missing-usage", "drift-base", "drift-escalation"} {
		t.Run(mode, func(t *testing.T) {
			m := newPairedMock(t, mode)
			out := filepath.Join(t.TempDir(), "run")
			if CollectPaired(context.Background(), m.client, pairedPlan(), 0, 2, 2048, "NOASSERTION", out) == nil {
				t.Fatal("invalid stages accepted")
			}
			expected := int64(2)
			if mode == "invalid-base" || mode == "drift-base" {
				expected = 1
			}
			r := readPairedReceipt(t, out)
			if m.chat.Load() != expected || r.Attempted != int(expected) || r.Complete || r.Accepted != 0 || len(r.Stages) != int(expected) {
				t.Fatalf("failure retried or hidden: %+v", r)
			}
			if mode == "missing-usage" && r.UsageComplete {
				t.Fatal("missing usage reported complete")
			}
			paths, _ := filepath.Glob(filepath.Join(out, "candidate-*.json"))
			if len(paths) != 0 {
				t.Fatal("failed candidate checkpointed")
			}
			paths, _ = filepath.Glob(filepath.Join(out, "response-*.json"))
			if len(paths) != int(expected) {
				t.Fatal("received failed replies lost")
			}
		})
	}
}
func TestPairedCancellationAccountsForUnansweredEscalation(t *testing.T) {
	m := newPairedMock(t, "cancel-escalation")
	out := filepath.Join(t.TempDir(), "run")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- CollectPaired(ctx, m.client, pairedPlan(), 0, 2, 2048, "NOASSERTION", out) }()
	select {
	case <-m.started:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("escalation not reached")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancel ignored")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not stop")
	}
	r := readPairedReceipt(t, out)
	if r.Attempted != 2 || r.Tokens != 100 || r.UsageComplete || r.Complete || r.Accepted != 0 || r.Phase != "escalation" || m.chat.Load() != 2 {
		t.Fatalf("unknown usage hidden: %+v", r)
	}
}
func TestPreloadSeparatedFromGeneration(t *testing.T) {
	for _, mode := range []string{"good", "load-content", "load-drift"} {
		t.Run(mode, func(t *testing.T) {
			m := newPairedMock(t, mode)
			out := filepath.Join(t.TempDir(), "load")
			err := Preload(context.Background(), m.client, out)
			if (err == nil) != (mode == "good") || m.load.Load() != 1 || m.chat.Load() != 0 {
				t.Fatalf("%v load=%d chat=%d", err, m.load.Load(), m.chat.Load())
			}
			if m.client.client.Timeout != 180*time.Second {
				t.Fatal("chat timeout modified")
			}
			b, e := os.ReadFile(filepath.Join(out, "receipt.json"))
			if e != nil {
				t.Fatal(e)
			}
			var r LoadReceipt
			if e = Strict(b, &r); e != nil {
				t.Fatal(e)
			}
			if r.Complete != (mode == "good") || r.ModelRequests != 1 {
				t.Fatal("invalid load receipt")
			}
			if mode == "good" && r.ReportedTokens != nil {
				t.Fatal("missing usage fabricated")
			}
			if Preload(context.Background(), m.client, out) == nil || m.load.Load() != 1 {
				t.Fatal("load overwritten/retried")
			}
		})
	}
}
func TestPairedBoundsFailBeforeModelRequests(t *testing.T) {
	m := newPairedMock(t, "good")
	for _, license := range []string{"", "CC0-1.0", "MIT"} {
		if CollectPaired(context.Background(), m.client, pairedPlan(), 0, 1, 2048, license, filepath.Join(t.TempDir(), "run")) == nil {
			t.Fatal("public/unspecified license accepted")
		}
	}
	if CollectPaired(context.Background(), m.client, pairedPlan(), 0, 65, 2048, "NOASSERTION", filepath.Join(t.TempDir(), "run")) == nil || m.chat.Load() != 0 {
		t.Fatal("unbounded requests accepted")
	}
}

func TestPreloadCancellationStopsBeforeGeneration(t *testing.T) {
	m := newPairedMock(t, "load-cancel")
	out := filepath.Join(t.TempDir(), "load")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Preload(ctx, m.client, out) }()
	select {
	case <-m.started:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("load not reached")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("load cancellation ignored")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("load cancellation did not stop")
	}
	b, err := os.ReadFile(filepath.Join(out, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var r LoadReceipt
	if err = Strict(b, &r); err != nil {
		t.Fatal(err)
	}
	if r.Complete || r.ModelRequests != 1 || r.Phase != "model-load" || m.chat.Load() != 0 || m.load.Load() != 1 {
		t.Fatalf("load failure hidden: %+v", r)
	}
}
