package contextcorpus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExpandedCorpusReviewFreezeAndCoverage(t *testing.T) {
	dir := "../../datasets/contextual/v3"
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	history, err := Load("../../datasets/contextual/v2")
	if err != nil {
		t.Fatal(err)
	}
	l, err := LoadReview(filepath.Join(dir, "review.json"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := os.ReadFile(filepath.Join(dir, "scenarios.json"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := LoadFreeze(filepath.Join(dir, "holdout.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = CheckFreeze(c, history, l, catalog, f); err != nil {
		t.Fatal(err)
	}
	if f.Rows != 280 || f.Families != 14 || c.Contract() != ContractV2 {
		t.Fatalf("unexpected holdout/contract: %+v %s", f, c.Contract())
	}
	p, _ := c.BuildReview()
	r, err := c.CheckReview(p, l)
	if err != nil || r.Status != "complete" || r.ReviewedRows != 1092 || r.Quarantined != 8 || r.Method != "model" || l.Human || l.Independent || len(p.Pairs) != 0 {
		t.Fatalf("bad expansion review: %+v %v", r, err)
	}
	counts, groups := map[string]int{}, map[string]bool{}
	for _, row := range c.Rows {
		counts[row.Split]++
		groups[row.Family] = true
	}
	if !reflect.DeepEqual(counts, map[string]int{"train": 280, "validation": 280, "test": 280, "development": 252}) || len(groups) != 72 {
		t.Fatalf("bad counts: %v families=%d", counts, len(groups))
	}
	coverage := c.Coverage()
	want, err := json.MarshalIndent(coverage, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "coverage.json"))
	if err != nil || strings.TrimSpace(string(got)) != string(want) {
		t.Fatalf("stale coverage: %v", err)
	}
	byCell := coverage["rows_by_split_type_document_language_label"].(map[string]int)
	for _, split := range []string{"train", "validation", "test"} {
		for _, typ := range c.Taxonomy.Types {
			for _, lang := range []string{"en", "ru", "es", "zh", "mul"} {
				for _, label := range []string{"0", "1"} {
					key := split + "/" + typ.ID + "/" + lang + "/" + label
					if byCell[key] != 4 {
						t.Fatalf("coverage hole: %s = %d", key, byCell[key])
					}
				}
			}
		}
	}
	// The lock binds the actual prepared test representation; v3 has a distinct
	// contract, so older Python consumers must explicitly add support before use.
	out := t.TempDir()
	if err = Prepare(c, out); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil || string(manifest) != string(committed) {
		t.Fatalf("stale v3 manifest: %v", err)
	}
	bad := f
	bad.TestSHA256 = strings.Repeat("0", 64)
	if err = CheckFreeze(c, history, l, catalog, bad); err == nil {
		t.Fatal("changed holdout hash accepted")
	}
}

func TestScenarioCounterfactualsAndTrustVersioning(t *testing.T) {
	catalog, err := LoadScenarios("../../datasets/contextual/v3/scenarios.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range catalog.Scenarios {
		rows := scenarioRows(scenario, catalog.License)
		if err = Validate(rows, map[string]bool{scenario.PrimaryType: true}); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < len(rows); i += 4 {
			a, permission, source, control := rows[i], rows[i+1], rows[i+2], rows[i+3]
			if *a.Label != 1 || *permission.Label != 0 || *source.Label != 1 || *control.Label != 0 || a.Task == permission.Task || !reflect.DeepEqual(a.Documents, permission.Documents) {
				t.Fatalf("broken permission pair: %s", a.ID)
			}
			if a.Task != source.Task || len(a.Documents) != len(source.Documents) {
				t.Fatal("trust pair changed task/documents")
			}
			for j := range a.Documents {
				d := source.Documents[j]
				if d.Trust != "user-provided" {
					t.Fatal("missing user-provided trust variant")
				}
				d.Trust = "untrusted"
				if !reflect.DeepEqual(d, a.Documents[j]) {
					t.Fatal("trust pair has confounding document change")
				}
			}
			if control.Documents[0].Trust != "user-provided" || !strings.Contains(control.Task, "example for security analysis") {
				t.Fatal("control source was not explicitly supplied as example data")
			}
		}
	}
	r := scenarioRows(catalog.Scenarios[0], catalog.License)[2]
	types := map[string]bool{r.PrimaryType: true}
	r.Schema = 1
	if err = Validate([]Record{r}, types); err == nil {
		t.Fatal("new trust silently accepted by historical schema")
	}
	r.Schema = 2
	r.Documents[0].Trust = "system"
	if err = Validate([]Record{r}, types); err == nil {
		t.Fatal("document escalated itself to system trust")
	}
}

func TestExpansionReproducesCommittedDataAndRefusesOverwrite(t *testing.T) {
	dir := "../../datasets/contextual/v3"
	out := filepath.Join(t.TempDir(), "reproduced")
	if err := Expand(filepath.Join(dir, "scenarios.json"), "../../datasets/contextual/v2", out); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		got, e := os.ReadFile(filepath.Join(out, entry.Name()))
		if e != nil {
			t.Fatal(e)
		}
		want, e := os.ReadFile(filepath.Join(dir, entry.Name()))
		if e != nil || string(got) != string(want) {
			t.Fatalf("non-reproducible %s: %v", entry.Name(), e)
		}
	}
	if err = Expand(filepath.Join(dir, "scenarios.json"), "../../datasets/contextual/v2", out); err == nil {
		t.Fatal("existing expansion overwritten")
	}
}

func TestFreezeRefusesOverwriteAndAmbiguousKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "holdout.json")
	f := HoldoutFreeze{Contract: FreezeContract}
	if err := WriteFreeze(path, f); err != nil {
		t.Fatal(err)
	}
	if err := WriteFreeze(path, f); err == nil {
		t.Fatal("holdout lock overwritten")
	}
	if err := os.WriteFile(path, []byte(`{"contract":"context-holdout-v1","Contract":"other"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFreeze(path); err == nil {
		t.Fatal("case-aliased lock field accepted")
	}
}

func TestFreezeDetectsReviewCatalogAndHistoricalDrift(t *testing.T) {
	catalog, err := LoadScenarios("../../datasets/contextual/v3/scenarios.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog.Scenarios = catalog.Scenarios[:1]
	catalog.Scenarios[0].Split = "test"
	data, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	prior := fixture(t)
	prior.Rows = prior.Rows[:1]
	prior.Rows[0].Provenance.Review = "model-reviewed"
	old := prior.Rows[0]
	old.Schema, old.Split = 2, "development"
	c := &Corpus{Rows: append([]Record{old}, scenarioRows(catalog.Scenarios[0], catalog.License)...), Taxonomy: prior.Taxonomy, Sources: map[string]string{}}
	complete := func() ReviewLedger {
		p, l := c.BuildReview()
		l = completedTestLedger(c, p, l)
		l.Method, l.Human, l.Independent = "model", false, false
		l.Model, l.Protocol, l.Limitations = "test model", "test protocol", "test fixture only"
		return l
	}
	l := complete()
	f, err := NewFreeze(c, prior, l, data, "2026-10-11T05:41:49Z")
	if err != nil {
		t.Fatal(err)
	}
	changed := l
	changed.ReconciliationNotes += " changed decision record"
	if err = CheckFreeze(c, prior, changed, data, f); err == nil {
		t.Fatal("changed review accepted")
	}
	if err = CheckFreeze(c, prior, l, append(data, '\n'), f); err == nil {
		t.Fatal("changed catalog bytes accepted")
	}
	c.Rows[0].Split = "train"
	if _, err = NewFreeze(c, prior, complete(), data, f.FrozenAt); err == nil {
		t.Fatal("observed historical row promoted into training")
	}
	c.Rows[0].Split = "development"
	c.Rows[0].Task += " altered historical task"
	if _, err = NewFreeze(c, prior, complete(), data, f.FrozenAt); err == nil {
		t.Fatal("historical content silently rewritten")
	}
}
