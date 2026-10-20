package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saifety-org/lab-py/research/internal/contextcollection"
)

func TestPlanPinnedOfflineAndNoOverwrite(t *testing.T) {
	b, err := os.ReadFile("../../datasets/contextual/v5/protocol.json")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "plan.json")
	if run([]string{"plan", "-input", "../../datasets/contextual/v5/protocol.json", "-sha256", strings.Repeat("0", 64), "-out", out}) == nil {
		t.Fatal("unpinned protocol accepted")
	}
	args := []string{"plan", "-input", "../../datasets/contextual/v5/protocol.json", "-sha256", contextcollection.Hash(b), "-out", out}
	if err = run(args); err != nil {
		t.Fatal(err)
	}
	if run(args) == nil {
		t.Fatal("plan overwritten")
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var p contextcollection.Plan
	if err = json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Slots) != 6380 {
		t.Fatalf("wrong quota reservations:%d", len(p.Slots))
	}
	counts := map[string]int{}
	for n, s := range p.Slots {
		if s.Index != n {
			t.Fatal("unstable tickets")
		}
		counts[s.Purpose+"/"+s.Language]++
	}
	for _, lang := range []string{"en", "ru", "es", "zh"} {
		if counts["validation/"+lang] != 600 || counts["confirmation/"+lang] != 600 || counts["train/"+lang] != 175 {
			t.Fatal(counts)
		}
	}
	if run([]string{"collect", "-input", out, "-sha256", strings.Repeat("f", 64), "-out", filepath.Join(t.TempDir(), "nothing")}) == nil {
		t.Fatal("unpinned plan accepted")
	}
	if run([]string{"collect-paired", "-input", out, "-sha256", strings.Repeat("f", 64), "-out", filepath.Join(t.TempDir(), "nothing-paired")}) == nil {
		t.Fatal("unpinned paired plan accepted")
	}
	if run([]string{"collect-contracts", "-input", out, "-sha256", strings.Repeat("f", 64), "-out", filepath.Join(t.TempDir(), "nothing-contracts")}) == nil {
		t.Fatal("unpinned contract plan accepted")
	}
	if err = run([]string{"collect-contracts-v4", "-input", out, "-sha256", strings.Repeat("f", 64), "-out", filepath.Join(t.TempDir(), "nothing-v4")}); err == nil || !strings.Contains(err.Error(), "plan hash differs") {
		t.Fatalf("v4 command did not enforce immutable plan before model calls: %v", err)
	}
	if err = run([]string{"collect-contracts-v5", "-input", out, "-sha256", strings.Repeat("f", 64), "-out", filepath.Join(t.TempDir(), "nothing-v5")}); err == nil || !strings.Contains(err.Error(), "plan hash differs") {
		t.Fatalf("v5 command did not enforce immutable plan before model calls: %v", err)
	}
	if run([]string{"preload", "-ollama-url", "http://example.invalid", "-model", "local", "-model-digest", strings.Repeat("a", 64), "-out", filepath.Join(t.TempDir(), "load")}) == nil {
		t.Fatal("nonlocal preload accepted")
	}
	if run([]string{"unknown"}) == nil || run(nil) == nil {
		t.Fatal("unknown operation accepted")
	}
}
