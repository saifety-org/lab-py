package contextcorpus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) *Corpus {
	t.Helper()
	c, err := Load("../../datasets/contextual/v1")
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func categorySet(c *Corpus) map[string]bool {
	m := map[string]bool{}
	for _, r := range c.Rows {
		m[r.PrimaryType] = true
	}
	return m
}
func TestCorpusCoverage(t *testing.T) {
	c := fixture(t)
	counts := map[string][2]int{}
	development := 0
	for _, r := range c.Rows {
		if r.Split == "development" {
			development++
			if !strings.HasPrefix(r.Provenance.DerivedFrom, "known-miss:") {
				t.Fatal("missing regression origin")
			}
			continue
		}
		if r.Language == "mul" {
			continue
		}
		key := r.PrimaryType + "/" + r.Split + "/" + r.Language
		n := counts[key]
		n[*r.Label]++
		counts[key] = n
	}
	if development != 10 {
		t.Fatal("must preserve ten development cases")
	}
	for kind := range categorySet(c) {
		for _, split := range []string{"train", "validation", "test"} {
			for _, language := range []string{"en", "ru", "es", "zh"} {
				n := counts[kind+"/"+split+"/"+language]
				if n[0] == 0 || n[1] == 0 {
					t.Fatalf("missing balanced coverage: %s %s %s", kind, split, language)
				}
			}
		}
	}
}
func TestRejectFamilyLeakageAndTrustElevation(t *testing.T) {
	c := fixture(t)
	r := c.Rows[0]
	copy := r
	copy.ID += "-copy"
	copy.Split = "test"
	if r.Split == "test" {
		copy.Split = "train"
	}
	if err := Validate([]Record{r, copy}, categorySet(c)); err == nil || !strings.Contains(err.Error(), "family crosses") {
		t.Fatalf("accepted family leakage: %v", err)
	}
	copy = r
	copy.Documents = append([]Document(nil), r.Documents...)
	copy.Documents[0].Trust = "system"
	if err := Validate([]Record{copy}, categorySet(c)); err == nil {
		t.Fatal("accepted document self-elevation")
	}
	copy = r
	copy.Label = nil
	if err := Validate([]Record{copy}, categorySet(c)); err == nil {
		t.Fatal("accepted missing label")
	}
}
func TestRejectNearCopyAndKnownMissPromotion(t *testing.T) {
	c := fixture(t)
	r := c.Rows[0]
	copy := r
	copy.ID += "-derivative"
	copy.Family += "-other"
	copy.Provenance.OriginID = copy.Family
	copy.Task += " Additional context."
	if err := Validate([]Record{r, copy}, categorySet(c)); err == nil || !strings.Contains(err.Error(), "near-copy") {
		t.Fatalf("accepted undeclared derivative: %v", err)
	}
	for _, r := range c.Rows {
		if r.Split == "development" {
			r.Split = "train"
			if err := Validate([]Record{r}, categorySet(c)); err == nil {
				t.Fatal("promoted known miss")
			}
			break
		}
	}
}
func TestPreparationDeterministicAndNoLabelInInput(t *testing.T) {
	c := fixture(t)
	a, b := t.TempDir(), t.TempDir()
	if err := Prepare(c, a); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(c, b); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"train.jsonl", "evaluation.jsonl", "development.jsonl", "manifest.json"} {
		x, err := os.ReadFile(filepath.Join(a, name))
		if err != nil {
			t.Fatal(err)
		}
		y, err := os.ReadFile(filepath.Join(b, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(x) != string(y) {
			t.Fatal("non-deterministic artifacts")
		}
	}
	r := c.Rows[0]
	s, err := Render(r)
	if err != nil {
		t.Fatal(err)
	}
	var input map[string]json.RawMessage
	if err = json.Unmarshal([]byte(s.Text), &input); err != nil {
		t.Fatal(err)
	}
	if len(input) != 3 || input["contract"] == nil || input["task"] == nil || input["documents"] == nil {
		t.Fatal("unexpected model fields")
	}
	for _, key := range []string{"label", "split", "rationale", "primary_type", "provenance", "language"} {
		if input[key] != nil {
			t.Fatal("label metadata leaked into input")
		}
	}
}
func TestAuthorizationPairsNeedContext(t *testing.T) {
	c := fixture(t)
	pairs := map[string][]Record{}
	for _, r := range c.Rows {
		for _, tag := range r.Tags {
			if tag == "authorization-pair" {
				pairs[r.Language+"/"+r.Family] = append(pairs[r.Language+"/"+r.Family], r)
			}
		}
	}
	if len(pairs) != 8 {
		t.Fatal("missing authorization languages")
	}
	for _, pair := range pairs {
		if len(pair) != 2 || *pair[0].Label == *pair[1].Label {
			t.Fatal("invalid paired labels")
		}
		a, err := Render(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		b, err := Render(pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if a.PayloadText != b.PayloadText || a.Text == b.Text {
			t.Fatal("pair does not isolate authorization")
		}
	}
}

func TestCommittedManifestMatchesCorpus(t *testing.T) {
	c := fixture(t)
	out := t.TempDir()
	if err := Prepare(c, out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../datasets/contextual/v1/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("manifest stale: make prepare-context; copy artifacts/contextual/manifest.json into datasets/contextual/v1/manifest.json")
	}
}
