// Package contextcorpus validates data without executing document instructions.
package contextcorpus

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const Contract = "context-json-v1"
const ContractV2 = "context-json-v2"

func contractForSchema(schema int) string {
	if schema == 2 {
		return ContractV2
	}
	return Contract
}

func (c *Corpus) Contract() string {
	if len(c.Rows) != 0 {
		return contractForSchema(c.Rows[0].Schema)
	}
	return Contract
}

var slug = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type Type struct {
	ID         string `json:"id"`
	Definition string `json:"definition"`
	Boundary   string `json:"boundary"`
}
type Taxonomy struct {
	Version int    `json:"version"`
	Types   []Type `json:"types"`
}
type Document struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Locator  string `json:"locator"`
	Trust    string `json:"trust"`
	Language string `json:"language"`
	Text     string `json:"text"`
}
type Provenance struct {
	Origin      string `json:"origin"`
	OriginID    string `json:"origin_id"`
	License     string `json:"license"`
	Review      string `json:"review"`
	DerivedFrom string `json:"derived_from,omitempty"`
}
type Record struct {
	Schema           int        `json:"schema"`
	ID               string     `json:"id"`
	Family           string     `json:"family"`
	Split            string     `json:"split"`
	PrimaryType      string     `json:"primary_type"`
	Tags             []string   `json:"tags"`
	Language         string     `json:"language"`
	Task             string     `json:"authorized_task"`
	Documents        []Document `json:"documents"`
	Label            *int       `json:"label"`
	ExpectedBehavior string     `json:"expected_behavior"`
	Rationale        string     `json:"rationale"`
	Provenance       Provenance `json:"provenance"`
}

// Input deliberately excludes labels, rationale, IDs, language tags and provenance.
// Trust/kind are supplied by the caller; text cannot overwrite these fields.
type InputDocument struct {
	Kind  string `json:"kind"`
	Trust string `json:"trust"`
	Text  string `json:"text"`
}
type Input struct {
	Contract  string          `json:"contract"`
	Task      string          `json:"task"`
	Documents []InputDocument `json:"documents"`
}
type Sample struct {
	ID          string   `json:"id"`
	Group       string   `json:"group"`
	Split       string   `json:"split"`
	Category    string   `json:"category"`
	Language    string   `json:"language"`
	Label       int      `json:"label"`
	Text        string   `json:"text"`
	PayloadText string   `json:"payload_text"`
	Documents   []string `json:"documents"`
	Tags        []string `json:"tags"`
	Review      string   `json:"review,omitempty"`
}
type Corpus struct {
	Rows     []Record
	Sources  map[string]string
	Taxonomy Taxonomy
}

func strict(b []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON value")
	}
	return nil
}
func hash(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func canonical(s string) string {
	return strings.Join(strings.Fields(cases.Fold().String(norm.NFKC.String(s))), " ")
}
func encode(v any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
func Render(r Record) (Sample, error) {
	if r.Label == nil {
		return Sample{}, fmt.Errorf("missing label")
	}
	input := Input{Contract: contractForSchema(r.Schema), Task: r.Task, Documents: make([]InputDocument, 0, len(r.Documents))}
	texts := make([]string, 0, len(r.Documents))
	for _, d := range r.Documents {
		input.Documents = append(input.Documents, InputDocument{d.Kind, d.Trust, d.Text})
		texts = append(texts, d.Text)
	}
	b, err := encode(input)
	if err != nil {
		return Sample{}, err
	}
	review := ""
	if r.Provenance.Review != "author-reviewed-human-review-pending" {
		review = r.Provenance.Review
	}
	return Sample{r.ID, r.Family, r.Split, r.PrimaryType, r.Language, *r.Label, string(b), strings.Join(texts, "\n\n"), texts, r.Tags, review}, nil
}
func Load(dir string) (*Corpus, error) {
	c := &Corpus{Sources: map[string]string{}}
	b, err := os.ReadFile(filepath.Join(dir, "taxonomy.json"))
	if err != nil {
		return nil, err
	}
	c.Sources["taxonomy.json"] = hash(b)
	var taxonomy Taxonomy
	if err = strict(b, &taxonomy); err != nil {
		return nil, err
	}
	if taxonomy.Version != 1 || len(taxonomy.Types) == 0 {
		return nil, fmt.Errorf("invalid taxonomy")
	}
	c.Taxonomy = taxonomy
	types := map[string]bool{}
	for _, t := range taxonomy.Types {
		if !slug.MatchString(t.ID) || types[t.ID] || t.Definition == "" || t.Boundary == "" {
			return nil, fmt.Errorf("invalid type %q", t.ID)
		}
		types[t.ID] = true
		name := t.ID + ".jsonl"
		b, err = os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		c.Sources[name] = hash(b)
		sc := bufio.NewScanner(bytes.NewReader(b))
		sc.Buffer(make([]byte, 4096), 4<<20)
		count := 0
		for sc.Scan() {
			var r Record
			if err = strict(sc.Bytes(), &r); err != nil {
				return nil, fmt.Errorf("%s line %d: %w", name, count+1, err)
			}
			if r.PrimaryType != t.ID {
				return nil, fmt.Errorf("category/file mismatch")
			}
			c.Rows = append(c.Rows, r)
			count++
		}
		if err = sc.Err(); err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, fmt.Errorf("empty category %s", t.ID)
		}
	}
	if err = Validate(c.Rows, types); err != nil {
		return nil, err
	}
	sort.Slice(c.Rows, func(i, j int) bool { return c.Rows[i].ID < c.Rows[j].ID })
	return c, nil
}
func Validate(rows []Record, types map[string]bool) error {
	ids, families, origins := map[string]bool{}, map[string]string{}, map[string]string{}
	for _, r := range rows {
		if (r.Schema != 1 && r.Schema != 2) || r.Schema != rows[0].Schema || !slug.MatchString(r.ID) || !slug.MatchString(r.Family) || ids[r.ID] {
			return fmt.Errorf("invalid/duplicate ID %q", r.ID)
		}
		ids[r.ID] = true
		if !types[r.PrimaryType] || r.Label == nil || (*r.Label != 0 && *r.Label != 1) || len(r.Tags) == 0 {
			return fmt.Errorf("%s: invalid label/type/tags", r.ID)
		}
		if r.Split != "train" && r.Split != "validation" && r.Split != "test" && r.Split != "development" {
			return fmt.Errorf("invalid split")
		}
		if old, ok := families[r.Family]; ok && old != r.Split {
			return fmt.Errorf("family crosses splits: %s", r.Family)
		}
		families[r.Family] = r.Split
		if r.Provenance.OriginID == "" || r.Provenance.License == "" || !validSourceReview(r.Provenance.Review) || r.Provenance.Origin != "synthetic" {
			return fmt.Errorf("%s: missing/unsupported provenance", r.ID)
		}
		if r.Provenance.Review == "model-review-quarantined" && r.Split != "development" {
			return fmt.Errorf("%s: quarantined example outside development", r.ID)
		}
		if old, ok := origins[r.Provenance.OriginID]; ok && old != r.Family {
			return fmt.Errorf("one origin must stay in one family")
		}
		origins[r.Provenance.OriginID] = r.Family
		if r.Provenance.DerivedFrom != "" && r.Split != "development" && strings.HasPrefix(r.Provenance.DerivedFrom, "known-miss:") {
			return fmt.Errorf("known miss outside development")
		}
		for _, s := range []string{r.Language, r.Task, r.ExpectedBehavior, r.Rationale} {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("%s: missing context", r.ID)
			}
		}
		if len(r.Documents) == 0 {
			return fmt.Errorf("%s: missing documents", r.ID)
		}
		docIDs := map[string]bool{}
		for _, d := range r.Documents {
			validTrust := d.Trust == "untrusted" || (r.Schema == 2 && d.Trust == "user-provided")
			if d.ID == "" || docIDs[d.ID] || d.Locator == "" || d.Language == "" || strings.TrimSpace(d.Text) == "" || !validTrust {
				return fmt.Errorf("%s: invalid document/trust", r.ID)
			}
			docIDs[d.ID] = true
			if d.Kind != "mcp" && d.Kind != "file" && d.Kind != "web" {
				return fmt.Errorf("invalid source kind")
			}
		}
	}
	// All language derivatives are explicitly grouped. This lexical check catches
	// undeclared exact/near-copy leakage; it cannot discover semantic translations.
	// Cache normalization and grams once per row; repeated corpus checks in CI
	// otherwise rebuild them for every pair, particularly expensive under -race.
	contexts := map[string]string{}
	type payload struct {
		family, id string
		grams      map[string]bool
	}
	unique := []payload{}
	seenPayload := map[string]bool{}
	for _, a := range rows {
		sa, err := Render(a)
		if err != nil {
			return err
		}
		text := canonical(sa.Text)
		if prior, ok := contexts[text]; ok {
			return fmt.Errorf("duplicate context: %s/%s", a.ID, prior)
		}
		contexts[text] = a.ID
		key := a.Family + "/" + canonical(sa.PayloadText)
		if seenPayload[key] {
			continue
		}
		seenPayload[key] = true
		current := payload{a.Family, a.ID, grams(sa.PayloadText)}
		for _, prior := range unique {
			if current.family != prior.family && possibleNear(current.grams, prior.grams) && jaccard(current.grams, prior.grams) >= .8 {
				return fmt.Errorf("near-copy must share family: %s/%s", current.id, prior.id)
			}
		}
		unique = append(unique, current)
	}
	return nil
}
func validSourceReview(s string) bool {
	return s == "author-reviewed-human-review-pending" || s == "model-reviewed" || s == "model-review-quarantined"
}
func grams(s string) map[string]bool {
	r := []rune(canonical(s))
	m := map[string]bool{}
	if len(r) < 5 {
		m[string(r)] = true
		return m
	}
	for i := 0; i+5 <= len(r); i++ {
		m[string(r[i:i+5])] = true
	}
	return m
}
func (c *Corpus) Manifest() map[string]any {
	counts := map[string]int{}
	families := map[string]bool{}
	for _, r := range c.Rows {
		counts[r.Split+"/"+r.PrimaryType+"/"+r.Language+fmt.Sprintf("/%d", *r.Label)]++
		families[r.Family] = true
	}
	m := map[string]any{"schema": 1, "contract": Contract, "rows": len(c.Rows), "families": len(families), "counts": counts, "source_sha256": c.Sources, "split_policy": "author-assigned scenario families before translations/paired controls; no random row split", "seed": 0, "near_duplicate_policy": "NFKC/casefold/whitespace character 5-gram Jaccard >= 0.8 requires shared family", "review_status": "human review pending; synthetic development corpus, not independent protection benchmark", "license": "NOASSERTION; no external data imported"}
	reviews := map[string]int{}
	m["contract"] = c.Contract()
	for _, r := range c.Rows {
		reviews[r.Provenance.Review]++
	}
	if reviews["model-reviewed"]+reviews["model-review-quarantined"] > 0 {
		m["review_status"] = "model review recorded in review.json; not independent human review or an independent benchmark"
		m["review_counts"] = reviews
		m["split_policy"] = "versioned family corrections in REVIEW.md; known-miss derivatives and quarantined families stay in development; no random row split"
	}
	return m
}
func write(path string, data []byte) error { return os.WriteFile(path, append(data, '\n'), 0644) }
func Prepare(c *Corpus, out string) error {
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	sets := map[string][]Sample{"train": {}, "evaluation": {}, "development": {}}
	for _, r := range c.Rows {
		s, err := Render(r)
		if err != nil {
			return err
		}
		key := r.Split
		if key == "validation" || key == "test" {
			key = "evaluation"
		}
		sets[key] = append(sets[key], s)
	}
	hashes := map[string]string{}
	for _, key := range []string{"train", "evaluation", "development"} {
		var b bytes.Buffer
		for _, s := range sets[key] {
			data, err := encode(s)
			if err != nil {
				return err
			}
			b.Write(data)
			b.WriteByte('\n')
		}
		name := key + ".jsonl"
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0644); err != nil {
			return err
		}
		hashes[name] = hash(b.Bytes())
	}
	m := c.Manifest()
	m["output_sha256"] = hashes
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return write(filepath.Join(out, "manifest.json"), b)
}
