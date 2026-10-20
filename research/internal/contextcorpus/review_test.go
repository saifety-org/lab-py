package contextcorpus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Synthetic ledger used ONLY to exercise acceptance gates. This is not an
// independent review of the repository corpus and is never published as one.
func completedTestLedger(c *Corpus, p ReviewPacket, l ReviewLedger) ReviewLedger {
	l.Reviewer = "test-fixture"
	l.Human = true
	l.Independent = true
	l.ReviewedAt = "2026-10-10T19:00:00Z"
	l.Reconciliation = "pass"
	l.ReconciliationNotes = "test-only reconciliation"
	rows := map[string]Record{}
	for _, r := range c.Rows {
		rows[reviewID("r-", r)] = r
	}
	for i := range l.Rows {
		j := &l.Rows[i]
		r := rows[j.ID]
		j.Label = r.Label
		j.PrimaryType = r.PrimaryType
		j.Authorization = "pass"
		j.TrustBoundaries = "pass"
		j.Translation = "pass"
		j.ExpectedBehavior = "test-only safe behavior"
		j.Notes = "test-only reasoning"
	}
	for i := range l.Groups {
		l.Groups[i].Status = "pass"
		l.Groups[i].Notes = "test-only grouping review"
	}
	for i := range l.Pairs {
		l.Pairs[i].Decision = "separate"
		l.Pairs[i].Notes = "test-only distinction"
	}
	return l
}

func TestReviewIsBlindedDeterministicAndPending(t *testing.T) {
	c := fixture(t)
	p, l := c.BuildReview()
	p2, l2 := c.BuildReview()
	if !reflect.DeepEqual(p, p2) || !reflect.DeepEqual(l, l2) {
		t.Fatal("unstable review bundle")
	}
	if len(p.Items) != 252 || len(l.Groups) != 38 {
		t.Fatal("lost records or groups")
	}
	for _, item := range p.Items {
		b, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err = json.Unmarshal(b, &fields); err != nil {
			t.Fatal(err)
		}
		if len(fields) != 5 {
			t.Fatal("unexpected review metadata")
		}
		for _, key := range []string{"label", "rationale", "split", "tags", "provenance", "expected_behavior", "primary_type"} {
			if _, ok := fields[key]; ok {
				t.Fatalf("author judgment leaked: %s", key)
			}
		}
		for _, r := range c.Rows {
			if item.ID == r.ID || item.Family == r.Family {
				t.Fatal("original ID leaks labels/splits")
			}
		}
		for _, d := range item.Documents {
			if strings.Contains(string(b), "locator") || d.Trust != "untrusted" {
				t.Fatal("locator leaked or trust changed")
			}
		}
	}
	result, err := c.CheckReview(p, l)
	if err != nil || result.Status != "pending" || result.ReviewedRows != 0 {
		t.Fatalf("template counted as human review: %+v %v", result, err)
	}
}

func TestReviewRejectsStaleDataIncludingSplitsAndTaxonomy(t *testing.T) {
	for _, change := range []string{"text", "split", "label", "source", "taxonomy"} {
		t.Run(change, func(t *testing.T) {
			c := fixture(t)
			p, l := c.BuildReview()
			l = completedTestLedger(c, p, l)
			switch change {
			case "text":
				c.Rows[0].Documents[0].Text += " changed"
			case "split":
				c.Rows[0].Split = "development"
			case "label":
				x := 1 - *c.Rows[0].Label
				c.Rows[0].Label = &x
			case "source":
				c.Sources["taxonomy.json"] = "changed"
			case "taxonomy":
				c.Taxonomy.Types[0].Definition += " changed"
			}
			p2, _ := c.BuildReview()
			if _, err := c.CheckReview(p2, l); err == nil {
				t.Fatal("stale review accepted")
			}
		})
	}
}

func TestReviewAcceptanceNeedsEveryJudgmentAndAttestation(t *testing.T) {
	c := fixture(t)
	p, base := c.BuildReview()
	mutations := map[string]func(*ReviewLedger){
		"human":          func(l *ReviewLedger) { l.Human = false },
		"independent":    func(l *ReviewLedger) { l.Independent = false },
		"reviewer":       func(l *ReviewLedger) { l.Reviewer = " " },
		"date":           func(l *ReviewLedger) { l.ReviewedAt = "yesterday" },
		"reconciliation": func(l *ReviewLedger) { l.Reconciliation = "pending" },
		"label":          func(l *ReviewLedger) { x := 1 - *l.Rows[0].Label; l.Rows[0].Label = &x },
		"category":       func(l *ReviewLedger) { l.Rows[0].PrimaryType = "wrong" },
		"translation":    func(l *ReviewLedger) { l.Rows[0].Translation = "fail" },
		"trust":          func(l *ReviewLedger) { l.Rows[0].TrustBoundaries = "fail" },
		"authorization":  func(l *ReviewLedger) { l.Rows[0].Authorization = "pending" },
		"notes":          func(l *ReviewLedger) { l.Rows[0].Notes = " " },
		"behavior":       func(l *ReviewLedger) { l.Rows[0].ExpectedBehavior = "" },
		"row":            func(l *ReviewLedger) { l.Rows = l.Rows[1:] },
		"group":          func(l *ReviewLedger) { l.Groups = l.Groups[1:] },
		"group-fail":     func(l *ReviewLedger) { l.Groups[0].Status = "fail" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			// Deep copy: a shallow ledger copy would mutate other test cases.
			b, _ := json.Marshal(base)
			var l ReviewLedger
			if err := json.Unmarshal(b, &l); err != nil {
				t.Fatal(err)
			}
			l = completedTestLedger(c, p, l)
			mutate(&l)
			result, err := c.CheckReview(p, l)
			if err != nil || result.Status == "complete" {
				t.Fatalf("invalid completion: %+v %v", result, err)
			}
		})
	}
	l := completedTestLedger(c, p, base)
	result, err := c.CheckReview(p, l)
	if err != nil || result.Status != "complete" || result.ReviewedRows != 252 {
		t.Fatalf("valid fixture rejected: %+v %v", result, err)
	}
	l.Rows = append(l.Rows, l.Rows[0])
	if _, err = c.CheckReview(p, l); err == nil {
		t.Fatal("duplicate decisions accepted")
	}
}

func TestIndividualDocumentCandidatesAndPairAdjudication(t *testing.T) {
	c := fixture(t)
	a, b := c.Rows[0], c.Rows[1]
	a.Family = "one"
	b.Family = "two"
	b.ID += "-different"
	a.Documents = []Document{{ID: "shared", Text: "A long shared document with synthetic content to detect copied portions."}, {ID: "other", Text: "Additional unrelated material in the first scenario."}}
	b.Documents = []Document{{ID: "shared", Text: a.Documents[0].Text}, {ID: "other", Text: "The second scenario contains different surrounding text."}}
	c.Rows = []Record{a, b}
	p, l := c.BuildReview()
	if len(p.Pairs) != 1 || p.Pairs[0].Score != 1 || p.Pairs[0].DocumentA != "shared" {
		t.Fatal("copied document missed")
	}
	l = completedTestLedger(c, p, l)
	l.Pairs[0].Decision = "same-family"
	result, err := c.CheckReview(p, l)
	if err != nil || result.Status == "complete" {
		t.Fatal("required family correction ignored")
	}
	l.Pairs = nil
	result, err = c.CheckReview(p, l)
	if err != nil || result.Status == "complete" {
		t.Fatal("omitted candidate ignored")
	}
}

func TestReviewHTMLDoesNotExecutePayloadOrOverwriteWork(t *testing.T) {
	c := fixture(t)
	c.Rows[0].Task = `</pre><script>alert("task")</script>`
	c.Rows[0].Documents[0].Text = `<img src="https://example.invalid/" onerror="alert(1)">`
	p, l := c.BuildReview()
	out := filepath.Join(t.TempDir(), "packet")
	if err := WriteReview(p, l, out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "<script>") || strings.Contains(string(b), "<img src=") || !strings.Contains(string(b), "&lt;script&gt;") {
		t.Fatal("active markup in review page")
	}
	if err = WriteReview(p, l, out); err == nil {
		t.Fatal("overwrote existing reviewer work")
	}
	key := filepath.Join(t.TempDir(), "key.json")
	if err = c.WriteReviewKey(key); err != nil {
		t.Fatal(err)
	}
	if err = c.WriteReviewKey(key); err == nil {
		t.Fatal("overwrote existing reconciliation key")
	}
	for _, bad := range []string{`{"unknown":true}`, `{} {}`, `{"human":true,"human":false}`, `{"human":true,"Human":false}`, `{"rows":[{"id":"one","id":"two"}]}`} {
		path := filepath.Join(t.TempDir(), "bad.json")
		if err = os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = LoadReview(path); err == nil {
			t.Fatal("non-strict review JSON accepted")
		}
	}
}
