package regression_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/saifety-org/sAIfety/pkg/inference"
	scan "github.com/saifety-org/sAIfety/pkg/scanner"
)

type adversarialCase struct {
	ID, Family, Label string
	Documents         []struct {
		Source string
		Kind   scan.Kind
		Text   string
	}
	MinLevel string `json:"min_level"`
	MaxLevel string `json:"max_level"`
	Category scan.Category
}

// This is a regression corpus, never training input. A sample must trigger
// the intended category and action level, not merely an unrelated warning.
func TestAdversarialCorpus(t *testing.T) {
	b, err := os.ReadFile("../../testdata/adversarial/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []adversarialCase
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("empty adversarial corpus")
	}
	seen := map[string]bool{}
	for _, c := range cases {
		_, minOK := scan.ParseLevel(c.MinLevel)
		_, maxOK := scan.ParseLevel(c.MaxLevel)
		if c.ID == "" || seen[c.ID] || c.Family == "" || len(c.Documents) == 0 || !minOK || !maxOK || (c.Label != "attack" && c.Label != "benign") || (c.Label == "attack" && c.Category == "") {
			t.Fatalf("invalid corpus case: %+v", c)
		}
		seen[c.ID] = true
	}
	trained, err := inference.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []struct {
		name       string
		classifier inference.Scorer
	}{
		{"rules", nil}, {"lexical", inference.Lexical{}}, {"trained", trained},
	} {
		for _, profile := range []string{"balanced", "strict"} {
			t.Run(mode.name+"/"+profile, func(t *testing.T) {
				sc, err := scan.New(scan.Config{Policy: profile, Classifier: mode.classifier})
				if err != nil {
					t.Fatal(err)
				}
				var tp, fp, tn, fn int
				for _, c := range cases {
					t.Run(c.ID, func(t *testing.T) {
						var docs []*scan.Document
						for _, input := range c.Documents {
							docs = append(docs, &scan.Document{Source: input.Source, Kind: input.Kind, Raw: input.Text})
						}
						verdicts := sc.ScanDocuments(context.Background(), docs)
						for i, doc := range docs {
							if verdicts[i].Action == scan.ActionBlock && strings.Contains(scan.Sanitize(doc, verdicts[i]), "FIXTURE_ATTACK_MARKER") {
								t.Error("blocked payload reached sanitized output")
							}
						}
						level := scan.LevelNone
						min, _ := scan.ParseLevel(c.MinLevel)
						max, _ := scan.ParseLevel(c.MaxLevel)
						categoryFound := c.Category == ""
						for _, v := range verdicts {
							if v.Level > level {
								level = v.Level
							}
							for _, f := range v.Findings {
								if f.Category == c.Category && f.Level >= min {
									categoryFound = true
								}
							}
						}
						// Use the same positive threshold for both labels; warnings
						// on benign samples are separately rejected by max_level.
						positive := level >= scan.LevelMedium
						switch {
						case c.Label == "attack" && positive:
							tp++
						case c.Label == "attack":
							fn++
						case positive:
							fp++
						default:
							tn++
						}
						if level < min || level > max || !categoryFound {
							t.Errorf("family=%s level=%s want=%s..%s category=%s found=%t", c.Family, level, min, max, c.Category, categoryFound)
						}
					})
				}
				t.Logf("threshold=medium precision=%.3f recall=%.3f TP=%d FP=%d TN=%d FN=%d", ratio(tp, tp+fp), ratio(tp, tp+fn), tp, fp, tn, fn)
			})
		}
	}
}
