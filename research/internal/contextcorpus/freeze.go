package contextcorpus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"time"
)

const FreezeContract = "context-holdout-v1"

// HoldoutFreeze binds the review, complete source corpus, scenario catalog and
// exact prepared test bytes. It detects drift, not prior observation or intent.
type HoldoutFreeze struct {
	Contract         string `json:"contract"`
	FrozenAt         string `json:"frozen_at"`
	CorpusSHA256     string `json:"corpus_sha256"`
	HistoricalSHA256 string `json:"historical_corpus_sha256"`
	CatalogSHA256    string `json:"catalog_sha256"`
	ReviewSHA256     string `json:"review_sha256"`
	TestSHA256       string `json:"prepared_test_sha256"`
	Rows             int    `json:"test_rows"`
	Families         int    `json:"test_families"`
	Policy           string `json:"policy"`
}

func NewFreeze(c, historical *Corpus, l ReviewLedger, catalog []byte, at string) (HoldoutFreeze, error) {
	f := HoldoutFreeze{Contract: FreezeContract, FrozenAt: at,
		Policy: "Freeze before downstream model fitting/selection and threshold tuning; tune on train/validation only. Observed historical rows are development-only. Translations and counterfactuals are clustered by family. Author-model reviewed synthetic holdout, not independent authorship or agent-protection evidence. A content change requires a new corpus version and explicit re-freeze; never refresh this lock just to pass CI."}
	if c == nil || historical == nil || len(c.Rows) == 0 || len(historical.Rows) == 0 {
		return f, fmt.Errorf("freeze requires nonempty current and historical corpora")
	}
	if _, err := time.Parse(time.RFC3339, at); err != nil {
		return f, err
	}
	p, _ := c.BuildReview()
	result, err := c.CheckReview(p, l)
	if err != nil || result.Status != "complete" {
		return f, fmt.Errorf("freeze requires complete bound review: %v", err)
	}
	source, err := decodeScenarios(catalog)
	if err != nil {
		return f, err
	}
	if source.Version == 4 {
		if _, err := c.TaskBalanceAudit(); err != nil {
			return f, err
		}
	}
	// Prevent a plausible-looking but unrelated catalog hash from becoming evidence.
	expected := map[string]Record{}
	for _, s := range source.Scenarios {
		for _, r := range catalogRows(source, s) {
			if _, exists := expected[r.ID]; exists {
				return f, fmt.Errorf("duplicate catalog row %s", r.ID)
			}
			expected[r.ID] = r
		}
	}
	history := map[string]Record{}
	for _, r := range historical.Rows {
		r.Split = "development"
		r.Schema = c.Rows[0].Schema
		history[r.ID] = r
	}
	var test bytes.Buffer
	families := map[string]bool{}
	rows := append([]Record(nil), c.Rows...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	for _, r := range rows {
		if prior, ok := history[r.ID]; ok {
			if !reflect.DeepEqual(r, prior) {
				return f, fmt.Errorf("historical row %s changed or escaped development", r.ID)
			}
			delete(history, r.ID)
		} else {
			want, ok := expected[r.ID]
			if !ok || !reflect.DeepEqual(r, want) {
				return f, fmt.Errorf("row %s does not match reviewed scenario catalog", r.ID)
			}
			delete(expected, r.ID)
		}
		if r.Split != "test" {
			continue
		}
		s, e := Render(r)
		if e != nil {
			return f, e
		}
		b, e := encode(s)
		if e != nil {
			return f, e
		}
		test.Write(b)
		test.WriteByte('\n')
		f.Rows++
		families[r.Family] = true
	}
	if len(history) != 0 || len(expected) != 0 || f.Rows == 0 {
		return f, fmt.Errorf("incomplete history/catalog or empty test")
	}
	f.Families = len(families)
	f.CorpusSHA256 = p.CorpusSHA256
	hp, _ := historical.BuildReview()
	f.HistoricalSHA256 = hp.CorpusSHA256
	f.CatalogSHA256 = hash(catalog)
	b, err := encode(l)
	if err != nil {
		return f, err
	}
	f.ReviewSHA256 = hash(b)
	f.TestSHA256 = hash(test.Bytes())
	return f, nil
}

func LoadFreeze(path string) (HoldoutFreeze, error) {
	var f HoldoutFreeze
	b, err := os.ReadFile(path)
	if err != nil {
		return f, err
	}
	if err = uniqueReviewKeys(b); err != nil {
		return f, err
	}
	err = strict(b, &f)
	return f, err
}

func CheckFreeze(c, historical *Corpus, l ReviewLedger, catalog []byte, frozen HoldoutFreeze) error {
	want, err := NewFreeze(c, historical, l, catalog, frozen.FrozenAt)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(frozen, want) {
		return fmt.Errorf("holdout freeze mismatch: corpus, catalog, review, history or test changed; use an explicitly reviewed new version")
	}
	return nil
}

func WriteFreeze(path string, f HoldoutFreeze) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, err = out.Write(append(b, '\n'))
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Coverage counts rows and scenario families separately, with language referring
// to documents (tasks in this expansion deliberately remain English).
func (c *Corpus) Coverage() map[string]any {
	counts := map[string]int{}
	families := map[string]map[string]bool{}
	for _, r := range c.Rows {
		key := fmt.Sprintf("%s/%s/%s/%d", r.Split, r.PrimaryType, r.Language, *r.Label)
		counts[key]++
		group := r.Split + "/" + r.PrimaryType
		if families[group] == nil {
			families[group] = map[string]bool{}
		}
		families[group][r.Family] = true
	}
	groups := map[string]int{}
	for key, set := range families {
		groups[key] = len(set)
	}
	caution := "Rows sharing family, translation, payload or permission counterfactual are dependent; do not treat row count as independent sample count. V3 task language is English."
	for _, r := range c.Rows {
		for _, tag := range r.Tags {
			if tag == "expansion-v4" {
				caution = "V4 tasks remain English; task/provenance balance is structural only. Translations, controls and history are dependent; no natural or independent benchmark claim."
			}
		}
	}
	return map[string]any{"contract": c.Contract(), "rows_by_split_type_document_language_label": counts,
		"families_by_split_type": groups, "caution": caution}
}
