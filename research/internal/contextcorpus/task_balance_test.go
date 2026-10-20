package contextcorpus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTaskAuditRejectsObservedV3Shortcut(t *testing.T) {
	c, err := Load("../../datasets/contextual/v3")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.TaskBalanceAudit(); err == nil || !strings.Contains(err.Error(), "task-label shortcut") {
		t.Fatalf("v3 shortcut not caught: %v", err)
	}
}

func TestBalancedV4ReviewFreezeCoverageAndReproduction(t *testing.T) {
	dir := "../../datasets/contextual/v4"
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	h, err := Load("../../datasets/contextual/v3")
	if err != nil {
		t.Fatal(err)
	}
	l, err := LoadReview(filepath.Join(dir, "review.json"))
	if err != nil {
		t.Fatal(err)
	}
	cat, err := os.ReadFile(filepath.Join(dir, "scenarios.json"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := LoadFreeze(filepath.Join(dir, "holdout.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = CheckFreeze(c, h, l, cat, f); err != nil {
		t.Fatal(err)
	}
	if f.Rows != 560 || f.Families != 14 || len(c.Rows) != 2772 {
		t.Fatalf("bad v4 sizes: %+v rows=%d", f, len(c.Rows))
	}
	audit, err := c.TaskBalanceAudit()
	if err != nil {
		t.Fatal(err)
	}
	for _, split := range []string{"train", "validation", "test"} {
		if audit.Rows[split] != 560 || audit.TaskOnlyAccuracyUpperBound[split] != .5 || audit.AuthorizationPairs[split] != 70 {
			t.Fatalf("bad audit: %+v", audit)
		}
	}
	counts := c.Coverage()["rows_by_split_type_document_language_label"].(map[string]int)
	for _, split := range []string{"train", "validation", "test"} {
		for _, typ := range c.Taxonomy.Types {
			for _, lang := range []string{"en", "ru", "es", "zh", "mul"} {
				for _, label := range []string{"0", "1"} {
					if counts[split+"/"+typ.ID+"/"+lang+"/"+label] != 8 {
						t.Fatalf("missing coverage %s/%s/%s/%s", split, typ.ID, lang, label)
					}
				}
			}
		}
	}
	// Frozen preparation and the generator must reproduce the committed bytes.
	out := filepath.Join(t.TempDir(), "reproduced")
	if err = Expand(filepath.Join(dir, "scenarios.json"), "../../datasets/contextual/v3", out); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		got, e := os.ReadFile(filepath.Join(out, file.Name()))
		if e != nil {
			t.Fatal(e)
		}
		want, e := os.ReadFile(filepath.Join(dir, file.Name()))
		if e != nil || string(got) != string(want) {
			t.Fatalf("stale %s: %v", file.Name(), e)
		}
	}
	prepared := t.TempDir()
	if err = Prepare(c, prepared); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(filepath.Join(prepared, "manifest.json"))
	b, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if string(a) != string(b) {
		t.Fatal("v4 manifest stale")
	}
	// Changing frozen holdout fails; old v3 lock remains separately checked by existing tests.
	f.TestSHA256 = strings.Repeat("0", 64)
	if err = CheckFreeze(c, h, l, cat, f); err == nil {
		t.Fatal("changed freeze accepted")
	}
}

func TestV4CounterfactualBalanceAndTrust(t *testing.T) {
	catalog, err := LoadScenarios("../../datasets/contextual/v4/scenarios.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range catalog.Scenarios {
		rows := catalogRows(catalog, scenario)
		if len(rows) != 40 {
			t.Fatal("v4 must have all variants")
		}
		if err = Validate(rows, map[string]bool{scenario.PrimaryType: true}); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < len(rows); i += 8 {
			a, b, p, e := rows[i], rows[i+1], rows[i+2], rows[i+3]
			if a.Task != b.Task || p.Task != e.Task || a.Task == p.Task || *a.Label != 1 || *b.Label != 0 || *p.Label != 0 || *e.Label != 1 || !reflect.DeepEqual(a.Documents, p.Documents) {
				t.Fatalf("broken matched-task/authorization variants %s", a.ID)
			}
			for j := 0; j < 4; j++ {
				original, provided := rows[i+j], rows[i+j+4]
				if original.Task != provided.Task || *original.Label != *provided.Label {
					t.Fatal("provenance changed task/label")
				}
				for k, d := range provided.Documents {
					if d.Trust != "user-provided" {
						t.Fatal("missing provenance variant")
					}
					d.Trust = "untrusted"
					if !reflect.DeepEqual(d, original.Documents[k]) {
						t.Fatal("provenance changed other document fields")
					}
				}
			}
		}
	}
}

func TestTaskBaselineIgnoresTestScoresAndIsReproducible(t *testing.T) {
	c, err := Load("../../datasets/contextual/v4")
	if err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(t.TempDir(), "baseline")
	if err = c.WriteTaskBaseline(first); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(filepath.Join(first, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile("../../datasets/contextual/v4/task-only-baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	var gotReport, wantReport map[string]any
	if err = json.Unmarshal(actual, &gotReport); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(committed, &wantReport); err != nil {
		t.Fatal(err)
	}
	// math.Exp implementations can change low bits of fitted weights across
	// architectures. Check all inputs/options/metrics exactly; the file hash
	// is checked against its own weights below, not another platform's weights.
	for _, key := range []string{"execution", "weights_sha256"} {
		delete(gotReport, key)
		delete(wantReport, key)
	}
	if !reflect.DeepEqual(gotReport, wantReport) {
		t.Fatal("stale committed baseline inputs or metrics")
	}
	for i := range c.Rows {
		if c.Rows[i].Split == "test" {
			c.Rows[i].Task += " test-only-text-must-not-reach-features"
		}
	}
	second := filepath.Join(t.TempDir(), "baseline")
	if err = c.WriteTaskBaseline(second); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"weights.json", "report.json"} {
		a, _ := os.ReadFile(filepath.Join(first, name))
		b, _ := os.ReadFile(filepath.Join(second, name))
		if string(a) != string(b) {
			t.Fatalf("test affected %s", name)
		}
	}
	b, err := os.ReadFile(filepath.Join(first, "weights.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Weights    string `json:"weights_sha256"`
		TestScored bool   `json:"test_scored"`
	}
	raw, err := os.ReadFile(filepath.Join(first, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.TestScored || report.Weights != hash(b) {
		t.Fatal("bad baseline provenance")
	}
	if err = c.WriteTaskBaseline(first); err == nil {
		t.Fatal("baseline evidence overwritten")
	}
}

func TestTaskAuditRequiresAuthorizationAndProvenancePairs(t *testing.T) {
	for _, mode := range []string{"authorization", "provenance"} {
		t.Run(mode, func(t *testing.T) {
			c, err := Load("../../datasets/contextual/v4")
			if err != nil {
				t.Fatal(err)
			}
			for i := range c.Rows {
				r := &c.Rows[i]
				if r.Split != "train" {
					continue
				}
				if mode == "authorization" {
					r.Tags = nil
				} else if r.Documents[0].Trust == "user-provided" {
					r.Documents[0].Text += " Changed provenance content."
					break
				}
			}
			if _, err = c.TaskBalanceAudit(); err == nil {
				t.Fatalf("missing %s pair accepted", mode)
			}
		})
	}
}

func TestGlobalTaskBalanceCannotHideFamilyShortcut(t *testing.T) {
	c := &Corpus{}
	for _, split := range []string{"train", "validation", "test"} {
		for label := 0; label < 2; label++ {
			for _, trust := range []string{"untrusted", "user-provided"} {
				y := label
				c.Rows = append(c.Rows, Record{Family: split + string(rune('a'+label)), Split: split, Task: "Same global task", Language: "en", Label: &y, Documents: []Document{{ID: "d1", Kind: "file", Trust: trust, Text: "Same content"}}})
			}
		}
	}
	if _, err := c.TaskBalanceAudit(); err == nil || !strings.Contains(err.Error(), "task-label shortcut") {
		t.Fatalf("family shortcut hidden by global balance: %v", err)
	}
}
