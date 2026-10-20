package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/saifety-org/lab-py/research/internal/contextcorpus"
)

func TestReviewCommandDoesNotCertifyTemplate(t *testing.T) {
	source := "../../datasets/contextual/v1"
	out := filepath.Join(t.TempDir(), "review")
	var b bytes.Buffer
	if err := execute([]string{"-source", source, "-review-out", out}, &b); err != nil {
		t.Fatal(err)
	}
	var result contextcorpus.ReviewResult
	if err := json.Unmarshal(b.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "pending" || result.TotalRows != 252 || result.ReviewedRows != 0 {
		t.Fatalf("unexpected review status: %+v", result)
	}
	b.Reset()
	if err := execute([]string{"-source", source, "-reviews", filepath.Join(out, "review-template.json"), "-require-reviewed"}, &b); err == nil {
		t.Fatal("incomplete review passed acceptance gate")
	}
	if err := json.Unmarshal(b.Bytes(), &result); err != nil {
		t.Fatal("missing machine-readable failure result")
	}
	if err := execute([]string{"-source", source, "-review-out", out}, &b); err == nil {
		t.Fatal("review work overwritten")
	}
}

func TestChangedHoldoutDoesNotWritePreparedData(t *testing.T) {
	source := "../../datasets/contextual/v3"
	f, err := contextcorpus.LoadFreeze(filepath.Join(source, "holdout.json"))
	if err != nil {
		t.Fatal(err)
	}
	f.TestSHA256 = "changed-test"
	tmp := t.TempDir()
	lock := filepath.Join(tmp, "bad-lock.json")
	if err = contextcorpus.WriteFreeze(lock, f); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(tmp, "prepared")
	err = execute([]string{"-source", source, "-reviews", filepath.Join(source, "review.json"),
		"-freeze", lock, "-history", "../../datasets/contextual/v2", "-catalog", filepath.Join(source, "scenarios.json"), "-out", out}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("changed test accepted")
	}
	if _, err = os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("preparation wrote output before verifying lock: %v", err)
	}
}

func TestReviewFlagErrors(t *testing.T) {
	for _, args := range [][]string{
		{"-require-reviewed"},
		{"-out", "out", "-review-out", "review"},
		{"-review-out", "review", "-reviews", "ledger.json"},
		{"-review-out", "review", "-review-key-out", "key"},
		{"-review-key-out", "key", "-require-reviewed"},
		{"unexpected"},
		{"-freeze", "lock.json"},
		{"-freeze-out", "lock.json", "-reviews", "review.json"},
		{"-history", "old"},
		{"-catalog", "scenarios.json"},
		{"-freeze", "a", "-freeze-out", "b", "-reviews", "r", "-history", "h", "-catalog", "c"},
	} {
		if err := execute(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("invalid flags accepted: %v", args)
		}
	}
}

func TestTaskBaselineRequiresVerifiedFreezeAndBalance(t *testing.T) {
	for _, args := range [][]string{
		{"-task-baseline-out", "out"},
		{"-task-baseline-out", "out", "-freeze", "lock", "-require-reviewed"},
		{"-task-audit-out", "audit"},
		{"-task-audit-out", "audit", "-require-task-balanced"},
	} {
		if err := execute(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("unsafe baseline/audit flags accepted: %v", args)
		}
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "prepared")
	audit := filepath.Join(tmp, "audit.json")
	source := "../../datasets/contextual/v3"
	err := execute([]string{"-source", source, "-reviews", filepath.Join(source, "review.json"), "-require-reviewed", "-require-task-balanced", "-task-audit-out", audit, "-out", out}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("observed v3 shortcut accepted")
	}
	for _, path := range []string{out, audit} {
		if _, err = os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("wrote artifacts before balance gate")
		}
	}
}
