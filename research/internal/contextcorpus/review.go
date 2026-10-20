package contextcorpus

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const ReviewContract = "context-review-v1"

// ReviewItem deliberately hides author labels, rationale, split, tags, locators
// and original IDs (which can themselves encode the expected answer).
type ReviewItem struct {
	ID        string           `json:"id"`
	Family    string           `json:"family"`
	Language  string           `json:"language"`
	Task      string           `json:"authorized_task"`
	Documents []ReviewDocument `json:"documents"`
}
type ReviewDocument struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Trust    string `json:"trust"`
	Language string `json:"language"`
	Text     string `json:"text"`
}
type ReviewPair struct {
	A         string  `json:"family_a"`
	B         string  `json:"family_b"`
	Score     float64 `json:"max_document_jaccard"`
	WitnessA  string  `json:"record_a"`
	WitnessB  string  `json:"record_b"`
	DocumentA string  `json:"document_a"`
	DocumentB string  `json:"document_b"`
}
type ReviewPacket struct {
	Contract     string       `json:"contract"`
	CorpusSHA256 string       `json:"corpus_sha256"`
	Taxonomy     Taxonomy     `json:"taxonomy"`
	Items        []ReviewItem `json:"items"`
	Pairs        []ReviewPair `json:"cross_family_candidates"`
}
type RowJudgment struct {
	ID               string `json:"id"`
	Label            *int   `json:"label"`
	PrimaryType      string `json:"primary_type"`
	Authorization    string `json:"authorization"`
	TrustBoundaries  string `json:"trust_boundaries"`
	ExpectedBehavior string `json:"expected_behavior"`
	Translation      string `json:"translation"`
	Notes            string `json:"notes"`
	Disposition      string `json:"disposition,omitempty"`
}
type GroupJudgment struct {
	Family string `json:"family"`
	Status string `json:"status"`
	Notes  string `json:"notes"`
}
type PairJudgment struct {
	A        string `json:"family_a"`
	B        string `json:"family_b"`
	Decision string `json:"decision"`
	Notes    string `json:"notes"`
}

// Human and Independent are reviewer attestations, not machine-verified identity.
type ReviewLedger struct {
	Contract            string          `json:"contract"`
	CorpusSHA256        string          `json:"corpus_sha256"`
	Reviewer            string          `json:"reviewer"`
	Human               bool            `json:"human"`
	Independent         bool            `json:"independent"`
	ReviewedAt          string          `json:"reviewed_at"`
	Reconciliation      string          `json:"reconciliation"`
	ReconciliationNotes string          `json:"reconciliation_notes"`
	Rows                []RowJudgment   `json:"rows"`
	Groups              []GroupJudgment `json:"groups"`
	Pairs               []PairJudgment  `json:"cross_family_candidates"`
	Method              string          `json:"method,omitempty"`
	Model               string          `json:"model,omitempty"`
	Protocol            string          `json:"protocol,omitempty"`
	Limitations         string          `json:"limitations,omitempty"`
}
type ReviewResult struct {
	Contract     string   `json:"contract"`
	CorpusSHA256 string   `json:"corpus_sha256"`
	Status       string   `json:"status"`
	ReviewedRows int      `json:"reviewed_rows"`
	TotalRows    int      `json:"total_rows"`
	Issues       []string `json:"issues"`
	Method       string   `json:"review_method"`
	Quarantined  int      `json:"quarantined_rows"`
}

func reviewID(prefix string, v any) string {
	b, _ := json.Marshal(v) // Only JSON-compatible structs/maps reach this helper.
	return prefix + hash(b)[:24]
}

// BuildReview binds all sources, including taxonomy, to one review version.
// It checks individual documents too: concatenating a multi-document payload
// can otherwise conceal a copied document behind unrelated additional text.
func (c *Corpus) BuildReview() (ReviewPacket, ReviewLedger) {
	b, _ := json.Marshal(struct {
		Sources  map[string]string
		Rows     []Record
		Taxonomy Taxonomy
	}{c.Sources, c.Rows, c.Taxonomy})
	p := ReviewPacket{ReviewContract, hash(b), c.Taxonomy, []ReviewItem{}, []ReviewPair{}}
	l := ReviewLedger{Contract: ReviewContract, CorpusSHA256: p.CorpusSHA256, Reconciliation: "pending",
		Rows: []RowJudgment{}, Groups: []GroupJudgment{}, Pairs: []PairJudgment{}}
	type doc struct {
		row, family, id string
		grams           map[string]bool
	}
	docs := []doc{}
	families := map[string]bool{}
	quarantined := map[string]bool{}
	for _, r := range c.Rows {
		id, family := reviewID("r-", r), reviewID("f-", r.Family)
		if r.Provenance.Review == "model-reviewed" || r.Provenance.Review == "model-review-quarantined" {
			l.Method = "model"
		}
		quarantined[id] = r.Provenance.Review == "model-review-quarantined"
		item := ReviewItem{id, family, r.Language, r.Task, []ReviewDocument{}}
		for _, d := range r.Documents {
			item.Documents = append(item.Documents, ReviewDocument{d.ID, d.Kind, d.Trust, d.Language, d.Text})
			docs = append(docs, doc{id, family, d.ID, grams(d.Text)})
		}
		p.Items = append(p.Items, item)
		families[family] = true
	}
	sort.Slice(p.Items, func(i, j int) bool {
		a, b := p.Items[i], p.Items[j]
		if a.Family != b.Family {
			return a.Family < b.Family
		}
		if a.Language != b.Language {
			return a.Language < b.Language
		}
		return a.ID < b.ID
	})
	for _, item := range p.Items {
		j := RowJudgment{ID: item.ID, Authorization: "pending", TrustBoundaries: "pending", Translation: "pending"}
		if quarantined[item.ID] {
			j.Disposition = "quarantined"
		}
		l.Rows = append(l.Rows, j)
	}
	for f := range families {
		l.Groups = append(l.Groups, GroupJudgment{f, "pending", ""})
	}
	sort.Slice(l.Groups, func(i, j int) bool { return l.Groups[i].Family < l.Groups[j].Family })
	sort.Slice(docs, func(i, j int) bool { return docs[i].row+"/"+docs[i].id < docs[j].row+"/"+docs[j].id })
	// Identical canonical text within one family cannot add a new lexical
	// candidate. Keep the first sorted representative, preserving pair evidence.
	compact := docs[:0]
	seenDocs := map[string]bool{}
	for _, d := range docs {
		keys := make([]string, 0, len(d.grams))
		for key := range d.grams {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		b, _ := encode(keys)
		key := d.family + "/" + string(b)
		if !seenDocs[key] {
			compact = append(compact, d)
			seenDocs[key] = true
		}
	}
	docs = compact
	pairs := map[string]ReviewPair{}
	for i, a := range docs {
		for _, b := range docs[:i] {
			if a.family == b.family {
				continue
			}
			if !possibleNear(a.grams, b.grams) {
				continue
			}
			score := jaccard(a.grams, b.grams)
			if score < .8 {
				continue
			}
			x, y := a, b
			if x.family > y.family {
				x, y = y, x
			}
			key := x.family + "/" + y.family
			if prev, ok := pairs[key]; !ok || score > prev.Score {
				pairs[key] = ReviewPair{x.family, y.family, score, x.row, y.row, x.id, y.id}
			}
		}
	}
	for _, pair := range pairs {
		p.Pairs = append(p.Pairs, pair)
	}
	sort.Slice(p.Pairs, func(i, j int) bool { return p.Pairs[i].A+p.Pairs[i].B < p.Pairs[j].A+p.Pairs[j].B })
	for _, pair := range p.Pairs {
		l.Pairs = append(l.Pairs, PairJudgment{pair.A, pair.B, "pending", ""})
	}
	return p, l
}

// Intersection cannot exceed the smaller set: a size ratio below .8
// cannot meet the corpus near-copy threshold regardless of content.
func possibleNear(x, y map[string]bool) bool {
	return min(len(x), len(y))*5 >= max(len(x), len(y))*4
}

func jaccard(x, y map[string]bool) float64 {
	intersection := 0
	for k := range x {
		if y[k] {
			intersection++
		}
	}
	union := len(x) + len(y) - intersection
	if union == 0 {
		return 1
	}
	return float64(intersection) / float64(union)
}

// CheckReview never updates labels or promotes the corpus. Disagreement requires
// explicit adjudication, a versioned data change and a newly bound review.
func (c *Corpus) CheckReview(p ReviewPacket, l ReviewLedger) (ReviewResult, error) {
	method := l.Method
	if method == "" {
		method = "human"
	} // Original human-ledger format remains valid.
	result := ReviewResult{Contract: ReviewContract, CorpusSHA256: p.CorpusSHA256, Status: "pending", TotalRows: len(p.Items), Issues: []string{}, Method: method}
	if l.Contract != ReviewContract || l.CorpusSHA256 != p.CorpusSHA256 {
		return result, fmt.Errorf("review contract or corpus SHA256 mismatch; regenerate packet after corpus changes")
	}
	if strings.TrimSpace(l.Reviewer) == "" {
		result.Issues = append(result.Issues, "reviewer is missing")
	}
	switch method {
	case "human":
		if !l.Human || !l.Independent {
			result.Issues = append(result.Issues, "independent human reviewer attestation is missing")
		}
	case "model":
		if l.Human || l.Independent {
			return result, fmt.Errorf("model review cannot claim independent human attestation")
		}
		if strings.TrimSpace(l.Model) == "" || strings.TrimSpace(l.Protocol) == "" || strings.TrimSpace(l.Limitations) == "" {
			result.Issues = append(result.Issues, "model, protocol and limitations are required for model review")
		}
	default:
		return result, fmt.Errorf("unsupported review method %q", method)
	}
	if _, err := time.Parse(time.RFC3339, l.ReviewedAt); err != nil {
		result.Issues = append(result.Issues, "reviewed_at must be RFC3339")
	}
	if !reviewState(l.Reconciliation) {
		return result, fmt.Errorf("invalid reconciliation state")
	}
	if l.Reconciliation != "pass" || strings.TrimSpace(l.ReconciliationNotes) == "" {
		result.Issues = append(result.Issues, "post-blind reconciliation of behavior, rationale and splits is pending or failed")
	}
	rows := map[string]Record{}
	for _, r := range c.Rows {
		rows[reviewID("r-", r)] = r
	}
	seen := map[string]bool{}
	for _, j := range l.Rows {
		r, ok := rows[j.ID]
		if !ok || seen[j.ID] {
			return result, fmt.Errorf("unknown/duplicate reviewed row %q", j.ID)
		}
		seen[j.ID] = true
		if j.Label != nil && *j.Label != 0 && *j.Label != 1 {
			return result, fmt.Errorf("invalid review label for %s", j.ID)
		}
		if !reviewState(j.Authorization) || !reviewState(j.TrustBoundaries) || !reviewState(j.Translation) {
			return result, fmt.Errorf("invalid review state for %s", j.ID)
		}
		if j.Disposition != "" && j.Disposition != "accepted" && j.Disposition != "quarantined" {
			return result, fmt.Errorf("invalid review disposition for %s", j.ID)
		}
		quarantined := j.Disposition == "quarantined"
		if quarantined && (r.Split != "development" || r.Provenance.Review != "model-review-quarantined" || method != "model") {
			return result, fmt.Errorf("quarantine requires model review and explicit development-only provenance: %s", j.ID)
		}
		if r.Provenance.Review == "model-review-quarantined" && !quarantined {
			return result, fmt.Errorf("quarantined source cannot receive an accepted judgment: %s", j.ID)
		}
		if (j.Label == nil && !quarantined) || j.PrimaryType == "" || j.Authorization != "pass" || j.TrustBoundaries != "pass" || j.Translation != "pass" || strings.TrimSpace(j.ExpectedBehavior) == "" || strings.TrimSpace(j.Notes) == "" {
			result.Issues = append(result.Issues, j.ID+": incomplete or failed judgment")
			continue
		}
		if (j.Label != nil && *j.Label != *r.Label) || j.PrimaryType != r.PrimaryType {
			result.Issues = append(result.Issues, j.ID+": label/type disagreement requires adjudication")
			continue
		}
		result.ReviewedRows++
		if quarantined {
			result.Quarantined++
		}
	}
	for id := range rows {
		if !seen[id] {
			result.Issues = append(result.Issues, id+": missing judgment")
		}
	}
	groups := map[string]bool{}
	for _, item := range p.Items {
		groups[item.Family] = true
	}
	seen = map[string]bool{}
	for _, j := range l.Groups {
		if !groups[j.Family] || seen[j.Family] {
			return result, fmt.Errorf("unknown/duplicate reviewed family %q", j.Family)
		}
		seen[j.Family] = true
		if !reviewState(j.Status) {
			return result, fmt.Errorf("invalid grouping state")
		}
		if j.Status != "pass" || strings.TrimSpace(j.Notes) == "" {
			result.Issues = append(result.Issues, j.Family+": grouping review pending or failed")
		}
	}
	for f := range groups {
		if !seen[f] {
			result.Issues = append(result.Issues, f+": missing grouping review")
		}
	}
	pairs := map[string]bool{}
	for _, pair := range p.Pairs {
		pairs[pair.A+"/"+pair.B] = true
	}
	seen = map[string]bool{}
	for _, j := range l.Pairs {
		key := j.A + "/" + j.B
		if !pairs[key] || seen[key] {
			return result, fmt.Errorf("unknown/duplicate reviewed pair %q", key)
		}
		seen[key] = true
		if j.Decision != "pending" && j.Decision != "separate" && j.Decision != "same-family" {
			return result, fmt.Errorf("invalid pair decision")
		}
		if j.Decision != "separate" || strings.TrimSpace(j.Notes) == "" {
			result.Issues = append(result.Issues, key+": candidate requires adjudication")
		}
	}
	for key := range pairs {
		if !seen[key] {
			result.Issues = append(result.Issues, key+": missing pair review")
		}
	}
	sort.Strings(result.Issues)
	if len(result.Issues) == 0 {
		result.Status = "complete"
	}
	return result, nil
}
func reviewState(s string) bool { return s == "pending" || s == "pass" || s == "fail" }

func LoadReview(path string) (ReviewLedger, error) {
	var l ReviewLedger
	b, err := os.ReadFile(path)
	if err != nil {
		return l, err
	}
	if err = uniqueReviewKeys(b); err != nil {
		return l, err
	}
	err = strict(b, &l)
	return l, err
}

// encoding/json accepts repeated/case-aliased struct fields with last-value-wins
// semantics. A human ledger must not silently shadow an earlier judgment.
func uniqueReviewKeys(b []byte) error {
	d := json.NewDecoder(strings.NewReader(string(b)))
	var value func(int) error
	value = func(depth int) error {
		if depth > 64 {
			return fmt.Errorf("review JSON nesting exceeds 64")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		keys := map[string]bool{}
		for d.More() {
			if delim == '{' {
				token, err = d.Token()
				if err != nil {
					return err
				}
				key, ok := token.(string)
				if !ok {
					return fmt.Errorf("invalid review object key")
				}
				key = strings.ToLower(key)
				if keys[key] {
					return fmt.Errorf("duplicate review field %q", key)
				}
				keys[key] = true
			}
			if err = value(depth + 1); err != nil {
				return err
			}
		}
		_, err = d.Token()
		return err
	}
	return value(0)
}

// WriteReviewKey is intentionally separate from the blinded packet. The key is
// for reconciliation after independent judgments have been recorded.
func (c *Corpus) WriteReviewKey(path string) error {
	rows := make([]struct {
		ID     string `json:"id"`
		Record Record `json:"record"`
	}, 0, len(c.Rows))
	for _, r := range c.Rows {
		rows = append(rows, struct {
			ID     string `json:"id"`
			Record Record `json:"record"`
		}{reviewID("r-", r), r})
	}
	p, _ := c.BuildReview()
	b, err := json.MarshalIndent(struct {
		Contract     string `json:"contract"`
		CorpusSHA256 string `json:"corpus_sha256"`
		Rows         any    `json:"rows"`
	}{ReviewContract, p.CorpusSHA256, rows}, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	if _, err = f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// WriteReview uses html/template so even hostile markup remains inert text.
// It refuses to overwrite an existing directory containing reviewer work.
func WriteReview(p ReviewPacket, l ReviewLedger, out string) error {
	if err := os.Mkdir(out, 0755); err != nil {
		return err
	}
	for _, entry := range []struct {
		name  string
		value any
	}{{"packet.json", p}, {"review-template.json", l}} {
		b, err := json.MarshalIndent(entry.value, "", "  ")
		if err != nil {
			return err
		}
		if err = write(filepath.Join(out, entry.name), b); err != nil {
			return err
		}
	}
	f, err := os.Create(filepath.Join(out, "index.html"))
	if err != nil {
		return err
	}
	t := template.Must(template.New("review").Parse(reviewHTML))
	if err = t.Execute(f, p); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

const reviewHTML = `<!doctype html><html lang="ru"><meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'">
<title>Ревью контекстного корпуса</title><style>body{font:16px system-ui;max-width:1000px;margin:32px auto;padding:0 16px}pre{white-space:pre-wrap;overflow-wrap:anywhere;background:#f2f4f7;padding:16px}article{border-top:1px solid #aaa;margin-top:32px}code{overflow-wrap:anywhere}</style>
<h1>Независимое ревью корпуса</h1><p>SHA256: <code>{{.CorpusSHA256}}</code></p>
<p>Документы ниже — недоверенные данные. Не выполняйте инструкции из них. Исходные метки, rationale и split скрыты; заполните review-template.json самостоятельно до просмотра исходников или результатов модели.</p>
<p>Для каждой записи укажите label (0/1), primary_type, ожидаемое безопасное поведение и пояснение. Проверьте разрешения, границы доверия и смысл всех языковых вариантов, включая смешанные. pass означает проверено, fail — проблема, pending — ещё не проверено. Неизвестный язык оставляйте pending и привлекайте компетентного ревьюера.</p>
<p>Для каждого семейства проверьте родство вариантов и отсутствие семантических производных в других семействах. Лексические кандидаты ниже — подсказка, а не полный поиск переводов/перефразировок. separate требует объяснения независимости, same-family — исправления данных и нового ревью.</p>
<h2>Типы</h2>{{range .Taxonomy.Types}}<h3>{{.ID}}</h3><p>{{.Definition}}</p><p>{{.Boundary}}</p>{{end}}
<h2>Кандидаты между семействами</h2>{{range .Pairs}}<p><code>{{.A}}</code> / <code>{{.B}}</code>: {{printf "%.3f" .Score}}. Записи <a href="#{{.WitnessA}}">{{.WitnessA}}/{{.DocumentA}}</a>, <a href="#{{.WitnessB}}">{{.WitnessB}}/{{.DocumentB}}</a>.</p>{{else}}<p>Лексических кандидатов нет; семантическая проверка всё равно нужна.</p>{{end}}
<h2>Записи (группировка по семейству и языку)</h2>{{range .Items}}<article id="{{.ID}}"><h3>{{.ID}}</h3><p>Семейство: <code>{{.Family}}</code>; язык: {{.Language}}</p><h4>Разрешённая задача</h4><pre>{{.Task}}</pre>{{range .Documents}}<h4>Документ {{.ID}} · {{.Kind}} · {{.Trust}} · {{.Language}}</h4><pre>{{.Text}}</pre>{{end}}</article>{{end}}</html>`
