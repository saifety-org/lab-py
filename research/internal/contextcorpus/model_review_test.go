package contextcorpus

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCommittedModelReviewAndManifest(t *testing.T) {
	c, err := Load("../../datasets/contextual/v2")
	if err != nil {
		t.Fatal(err)
	}
	p, blank := c.BuildReview()
	r, err := c.CheckReview(p, blank)
	if err != nil || r.Status != "pending" || r.Method != "model" {
		t.Fatalf("blank template accepted or invalid: %+v %v", r, err)
	}
	l, err := LoadReview("../../datasets/contextual/v2/review.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err = c.CheckReview(p, l)
	if err != nil || r.Status != "complete" || r.Method != "model" || r.Quarantined != 8 || r.ReviewedRows != 252 || l.Human || l.Independent {
		t.Fatalf("bad model review: %+v %v", r, err)
	}
	counts := map[string]int{}
	families := map[string]bool{}
	for _, row := range c.Rows {
		counts[row.Split]++
		families[row.Family] = true
	}
	if !reflect.DeepEqual(counts, map[string]int{"train": 98, "validation": 48, "test": 64, "development": 42}) || len(families) != 30 {
		t.Fatalf("unexpected splits/families: %v %d", counts, len(families))
	}
	out := t.TempDir()
	if err = Prepare(c, out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../datasets/contextual/v2/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("stale v2 manifest")
	}
	for _, name := range []string{"train.jsonl", "evaluation.jsonl"} {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		// Check decoded IDs, not just counts: a quarantined sibling/translation
		// must never accidentally become a supervised example.
		var sets []Sample
		for _, line := range bytes.Split(bytes.TrimSpace(b), []byte("\n")) {
			var sample Sample
			if err = json.Unmarshal(line, &sample); err != nil {
				t.Fatal(err)
			}
			sets = append(sets, sample)
		}
		for _, sample := range sets {
			for _, row := range c.Rows {
				if row.ID == sample.ID && (row.Provenance.Review == "model-review-quarantined" || row.Split == "development") {
					t.Fatal("development/quarantine leaked into supervised data")
				}
			}
		}
	}
}

func TestQuarantineStatusSurvivesPreparation(t *testing.T) {
	c, err := Load("../../datasets/contextual/v2")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range c.Rows {
		if row.Provenance.Review == "model-review-quarantined" {
			s, err := Render(row)
			if err != nil || s.Review != "model-review-quarantined" {
				t.Fatal("prepared sample lost quarantine status")
			}
		}
	}
}

func TestModelReviewCannotClaimHumanReviewOrAcceptQuarantine(t *testing.T) {
	c, err := Load("../../datasets/contextual/v2")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.BuildReview()
	base, err := LoadReview("../../datasets/contextual/v2/review.json")
	if err != nil {
		t.Fatal(err)
	}
	clone := func() ReviewLedger {
		b, _ := json.Marshal(base)
		var l ReviewLedger
		if err = json.Unmarshal(b, &l); err != nil {
			t.Fatal(err)
		}
		return l
	}
	for _, flag := range []string{"human", "independent", "unknown-method", "accept-quarantine"} {
		l := clone()
		switch flag {
		case "human":
			l.Human = true
		case "independent":
			l.Independent = true
		case "unknown-method":
			l.Method = "automatically-trusted"
		case "accept-quarantine":
			for i := range l.Rows {
				if l.Rows[i].Disposition == "quarantined" {
					l.Rows[i].Disposition = "accepted"
					break
				}
			}
		}
		if _, err = c.CheckReview(p, l); err == nil {
			t.Fatalf("accepted %s", flag)
		}
	}
	l := clone()
	l.Limitations = ""
	r, err := c.CheckReview(p, l)
	if err != nil || r.Status == "complete" {
		t.Fatal("model limitations omitted")
	}
	for _, row := range c.Rows {
		if row.Provenance.Review == "model-review-quarantined" {
			row.Split = "train"
			if err = Validate([]Record{row}, categorySet(c)); err == nil {
				t.Fatal("quarantine promotion accepted")
			}
			break
		}
	}
}
