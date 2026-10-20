// Package contextcollection collects unreviewed candidates without executing content.
// It never assigns training splits or certifies statistical independence.
package contextcollection

import (
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
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const Schema = "saifety-context-candidates-v1"
const MaxFamilyBytes = 128 << 10

var slug = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,119}$`)
var hexDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)
var types = []string{"instruction-override", "authority-spoofing", "answer-contamination", "data-exfiltration", "verification-suppression", "tool-output-laundering", "evidence-fabrication"}
var languages = []string{"en", "ru", "es", "zh", "de", "fr", "pt", "ar", "hi", "ja", "ko", "tr", "mixed", "unknown", "code", "encoded"}
var channels = []string{"file", "web", "mcp-text", "mcp-structured", "mcp-resource"}

type Content struct {
	Task                 string   `json:"authorized_task"`
	Permissions          []string `json:"permissions"`
	PermittedTask        string   `json:"permitted_task"`
	PermittedPermissions []string `json:"permitted_permissions"`
	Payload              []string `json:"payload"`
	Benign               []string `json:"benign_payload"`
	Escalation           []string `json:"escalation_payload"`
	Rationale            string   `json:"rationale"`
}
type Origin struct {
	Method         string          `json:"method"`
	Author         string          `json:"author"`
	Recipe         string          `json:"recipe"`
	Model          string          `json:"model,omitempty"`
	Digest         string          `json:"model_digest,omitempty"`
	Runtime        string          `json:"runtime,omitempty"`
	PromptSHA256   string          `json:"prompt_sha256,omitempty"`
	ResponseSHA256 string          `json:"response_sha256,omitempty"`
	DerivedFrom    []string        `json:"derived_from"`
	Stages         []StageEvidence `json:"stages,omitempty"`
	ContractSHA256 string          `json:"contract_sha256,omitempty"`
}
type StageEvidence struct {
	Name           string `json:"name"`
	PromptSHA256   string `json:"prompt_sha256"`
	RequestSHA256  string `json:"request_sha256"`
	ResponseSHA256 string `json:"response_sha256"`
}
type Family struct {
	Tags            []string `json:"tags"`
	DocumentTrust   string   `json:"document_trust"`
	Schema          string   `json:"schema"`
	ID              string   `json:"id"`
	Root            string   `json:"source_root"`
	Template        string   `json:"template_root,omitempty"`
	ProvenanceGroup string   `json:"provenance_group"`
	Language        string   `json:"primary_language"`
	Type            string   `json:"primary_type"`
	Channel         string   `json:"channel"`
	License         string   `json:"license"`
	Review          string   `json:"review_status"`
	Origin          Origin   `json:"origin"`
	Content         Content  `json:"content"`
}
type Protocol struct {
	Schema      string   `json:"schema"`
	ID          string   `json:"id"`
	ThreatTypes []string `json:"threat_types"`
	Languages   struct {
		Blocking    []string `json:"candidate_blocking"`
		Exploratory []string `json:"exploratory"`
	} `json:"languages"`
	Data struct {
		Minimum map[string]int `json:"minimum_families"`
	} `json:"data"`
}
type Slot struct {
	Index    int    `json:"index"`
	Language string `json:"language"`
	Type     string `json:"primary_type"`
	Channel  string `json:"channel"`
	Purpose  string `json:"reservation"`
}
type Plan struct {
	Schema         string `json:"schema"`
	ProtocolSHA256 string `json:"protocol_sha256"`
	Slots          []Slot `json:"slots"`
	Policy         string `json:"policy"`
}

func Hash(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func Contains(values []string, s string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func jsonValue(d *json.Decoder, depth int) error {
	if depth > 64 {
		return fmt.Errorf("JSON nesting exceeds limit")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate JSON field")
			}
			seen[name] = true
			if err = jsonValue(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := jsonValue(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
	_, err = d.Token()
	return err
}
func Strict(b []byte, out any) error {
	if !utf8.Valid(b) {
		return fmt.Errorf("invalid UTF-8 JSON")
	}
	keys := json.NewDecoder(bytes.NewReader(b))
	if err := jsonValue(keys, 0); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("extra JSON value")
	}
	return nil
}
func canonical(s string) string {
	return strings.Join(strings.Fields(cases.Fold().String(norm.NFKC.String(s))), " ")
}
func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func Validate(f Family) error {
	if f.Schema != Schema || !slug.MatchString(f.ID) || !slug.MatchString(f.Root) || !slug.MatchString(f.ProvenanceGroup) || (f.Template != "" && !slug.MatchString(f.Template)) {
		return fmt.Errorf("invalid candidate identity")
	}
	if f.DocumentTrust != "untrusted" || len(f.Tags) == 0 || strings.TrimSpace(f.Origin.Author) == "" {
		return fmt.Errorf("missing source trust/tags/author")
	}
	seenTags := map[string]bool{}
	for _, tag := range f.Tags {
		if !slug.MatchString(tag) || seenTags[tag] {
			return fmt.Errorf("invalid tags")
		}
		seenTags[tag] = true
	}
	if !Contains(types, f.Type) || !Contains(languages, f.Language) || !Contains(channels, f.Channel) {
		return fmt.Errorf("unknown type/language/channel")
	}
	if f.License != "CC0-1.0" && f.License != "MIT" && f.License != "NOASSERTION" {
		return fmt.Errorf("unsupported license declaration")
	}
	if f.Review != "pending" {
		return fmt.Errorf("collection cannot declare a completed review")
	}
	if f.Origin.Method != "authored" && f.Origin.Method != "ollama" {
		return fmt.Errorf("unsupported origin method")
	}
	if !slug.MatchString(f.Origin.Recipe) {
		return fmt.Errorf("missing generation recipe")
	}
	if f.Origin.Method == "ollama" && (f.Origin.Model == "" || !hexDigest.MatchString(f.Origin.Digest) || f.Origin.Runtime == "" || !hexDigest.MatchString(f.Origin.PromptSHA256) || !hexDigest.MatchString(f.Origin.ResponseSHA256)) {
		return fmt.Errorf("missing generation receipt")
	}
	contract := strings.HasPrefix(f.Origin.Recipe, "contract-v3-") || strings.HasPrefix(f.Origin.Recipe, "contract-v4-")
	if contract && !hexDigest.MatchString(f.Origin.ContractSHA256) {
		return fmt.Errorf("missing contract plan evidence")
	}
	if len(f.Origin.Stages) > 0 || strings.HasPrefix(f.Origin.Recipe, "paired-") || contract {
		if f.Origin.Method != "ollama" || len(f.Origin.Stages) != 2 {
			return fmt.Errorf("missing paired generation stages")
		}
		names := []string{"base", "escalation"}
		if contract {
			names = []string{"intent", "documents"}
		}
		for i, stage := range f.Origin.Stages {
			if stage.Name != names[i] || !hexDigest.MatchString(stage.PromptSHA256) || !hexDigest.MatchString(stage.RequestSHA256) || !hexDigest.MatchString(stage.ResponseSHA256) {
				return fmt.Errorf("invalid paired generation evidence")
			}
		}
		prompts, _ := json.Marshal([]string{f.Origin.Stages[0].PromptSHA256, f.Origin.Stages[1].PromptSHA256})
		replies, _ := json.Marshal([]string{f.Origin.Stages[0].ResponseSHA256, f.Origin.Stages[1].ResponseSHA256})
		if f.Origin.PromptSHA256 != Hash(prompts) || f.Origin.ResponseSHA256 != Hash(replies) {
			return fmt.Errorf("paired aggregate evidence differs from stages")
		}
	}
	for _, parent := range f.Origin.DerivedFrom {
		if !slug.MatchString(parent) {
			return fmt.Errorf("invalid derivation root")
		}
	}
	if err := validateContent(f.Content, true); err != nil {
		return err
	}
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if len(b) > MaxFamilyBytes {
		return fmt.Errorf("candidate exceeds size limit")
	}
	return nil
}

func equivalentDocs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if canonical(a[i]) != canonical(b[i]) {
			return false
		}
	}
	return true
}

func validateContent(c Content, withEscalation bool) error {
	if strings.TrimSpace(c.Task) == "" {
		return fmt.Errorf("contract_missing_task: authorized_task is empty")
	}
	if strings.TrimSpace(c.PermittedTask) == "" {
		return fmt.Errorf("contract_missing_task: permitted_task is empty")
	}
	if canonical(c.Task) == canonical(c.PermittedTask) {
		return fmt.Errorf("contract_equal_tasks: trusted task strings must differ")
	}
	if len(c.Permissions) == 0 {
		return fmt.Errorf("contract_missing_permissions: permissions is empty")
	}
	if len(c.PermittedPermissions) == 0 {
		return fmt.Errorf("contract_missing_permissions: permitted_permissions is empty")
	}
	if strings.TrimSpace(c.Rationale) == "" {
		return fmt.Errorf("contract_missing_rationale")
	}
	for _, permission := range append(append([]string{}, c.Permissions...), c.PermittedPermissions...) {
		if strings.TrimSpace(permission) == "" {
			return fmt.Errorf("empty permission")
		}
	}
	if len(c.Payload) == 0 || len(c.Payload) > 32 || len(c.Benign) != len(c.Payload) || (withEscalation && len(c.Escalation) != len(c.Payload)) {
		return fmt.Errorf("document boundary mismatch")
	}
	for _, docs := range [][]string{c.Payload, c.Benign, c.Escalation} {
		for _, text := range docs {
			if strings.TrimSpace(text) == "" {
				return fmt.Errorf("empty document")
			}
		}
	}
	if equivalentDocs(c.Payload, c.Benign) || (withEscalation && (equivalentDocs(c.Payload, c.Escalation) || equivalentDocs(c.Benign, c.Escalation))) {
		return fmt.Errorf("counterfactual documents must differ")
	}
	return nil
}
func Load(path string) ([]Family, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	b, err := io.ReadAll(io.LimitReader(file, (64<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 64<<20 {
		return nil, fmt.Errorf("collection exceeds64MiB")
	}
	lines := bytes.Split(bytes.TrimSpace(b), []byte("\n"))
	out := make([]Family, 0, len(lines))
	ids := map[string]bool{}
	for n, line := range lines {
		if len(line) > MaxFamilyBytes {
			return nil, fmt.Errorf("line%d too large", n+1)
		}
		var f Family
		if err := Strict(line, &f); err != nil {
			return nil, fmt.Errorf("line%d: %w", n+1, err)
		}
		if err := Validate(f); err != nil {
			return nil, fmt.Errorf("line%d: %w", n+1, err)
		}
		if ids[f.ID] {
			return nil, fmt.Errorf("duplicate candidate ID")
		}
		ids[f.ID] = true
		out = append(out, f)
	}
	return out, nil
}
func MakePlan(raw []byte) (Plan, error) {
	var p Protocol
	// The complete foreign protocol is hash-pinned; its unused fields are preserved in the receipt.
	if err := json.Unmarshal(raw, &p); err != nil {
		return Plan{}, err
	}
	if p.Schema != "saifety-evaluation-protocol-v1" || p.ID != "analyzer-quality-v1" || !equal(p.ThreatTypes, types) || !equal(p.Languages.Blocking, []string{"en", "ru", "es", "zh"}) {
		return Plan{}, fmt.Errorf("unsupported protocol")
	}
	for _, key := range []string{"train", "validation_per_candidate_language", "confirmation_per_candidate_language", "new_development", "exploratory_per_language"} {
		if p.Data.Minimum[key] <= 0 || p.Data.Minimum[key] > 10000 {
			return Plan{}, fmt.Errorf("invalid quota %s", key)
		}
	}
	if !equal(p.Languages.Exploratory, []string{"de", "fr", "pt", "ar", "hi", "ja", "ko", "tr"}) {
		return Plan{}, fmt.Errorf("unsupported exploratory matrix")
	}
	out := Plan{Schema: "saifety-context-collection-plan-v1", ProtocolSHA256: Hash(raw), Slots: []Slot{}, Policy: "Reservations are candidate authoring targets, NOT assigned training splits or independent/reviewed families. Group derivations/near-duplicates and audit before freeze. No fitting from collection outputs."}
	add := func(lang, purpose string, n int) {
		for i := 0; i < n; i++ {
			j := len(out.Slots)
			out.Slots = append(out.Slots, Slot{j, lang, types[i%len(types)], channels[i%len(channels)], purpose})
		}
	}
	for i := 0; i < p.Data.Minimum["train"]; i++ {
		j := len(out.Slots)
		out.Slots = append(out.Slots, Slot{j, p.Languages.Blocking[i%len(p.Languages.Blocking)], types[i%len(types)], channels[i%len(channels)], "train"})
	}
	for _, purpose := range []string{"validation", "confirmation"} {
		for _, lang := range p.Languages.Blocking {
			add(lang, purpose, p.Data.Minimum[purpose+"_per_candidate_language"])
		}
	}
	add("en", "development", p.Data.Minimum["new_development"])
	for _, lang := range p.Languages.Exploratory {
		add(lang, "development", p.Data.Minimum["exploratory_per_language"])
	}
	for _, lang := range []string{"mixed", "unknown", "code", "encoded"} {
		add(lang, "development", p.Data.Minimum["stress_per_kind"])
	}
	add("en", "over-limit-development", p.Data.Minimum["over_limit"])
	return out, nil
}

type Coverage struct {
	Schema     string              `json:"schema"`
	Candidates int                 `json:"candidate_families"`
	Connected  int                 `json:"source_connected_groups"`
	Rows       int                 `json:"counterfactual_rows"`
	Reviewed   int                 `json:"reviewed_families"`
	Counts     map[string]int      `json:"candidate_counts"`
	Groups     map[string][]string `json:"connected_groups"`
	Unlicensed int                 `json:"unresolved_license_candidates"`
	Missing    []string            `json:"blocking_gaps"`
	Ready      bool                `json:"training_ready"`
}

func Report(rows []Family) Coverage {
	r := Coverage{Schema: "saifety-context-collection-coverage-v1", Candidates: len(rows), Rows: len(rows) * 4, Counts: map[string]int{}, Groups: map[string][]string{}, Missing: []string{"model review with disclosed author/generator limitations and semantic/lexical duplicate audit pending", "actual pinned tokenizer/window coverage unmeasured; long padding is not independent evidence", "no accepted frozen train/validation/confirmation splits; all candidates pending", "protocol quotas apply to reviewed source-connected families, not candidate IDs"}}
	parent := map[string]string{}
	var find func(string) string
	find = func(k string) string {
		if _, ok := parent[k]; !ok {
			parent[k] = k
		}
		if parent[k] != k {
			parent[k] = find(parent[k])
		}
		return parent[k]
	}
	join := func(a, b string) {
		a, b = find(a), find(b)
		if a > b {
			a, b = b, a
		}
		parent[b] = a
	}
	// Explicit source roots, scenario templates, derivations and exact document copies
	// bind families across files/languages; invented IDs do not create new groups.
	docs := map[string]string{}
	for _, f := range rows {
		node := "id:" + f.ID
		join(node, "root:"+f.Root)
		if f.Template != "" {
			join(node, "template:"+f.Template)
		}
		for _, d := range f.Origin.DerivedFrom {
			join(node, "root:"+d)
		}
		for _, texts := range [][]string{f.Content.Payload, f.Content.Benign, f.Content.Escalation} {
			for _, text := range texts {
				k := Hash([]byte(canonical(text)))
				if old, ok := docs[k]; ok {
					join(node, old)
				} else {
					docs[k] = node
				}
			}
		}
		r.Counts[f.Language+"/"+f.Type+"/"+f.Channel]++
		if f.License == "NOASSERTION" {
			r.Unlicensed++
		}
	}
	for _, f := range rows {
		k := find("id:" + f.ID)
		r.Groups[k] = append(r.Groups[k], f.ID)
	}
	for k := range r.Groups {
		sort.Strings(r.Groups[k])
	}
	r.Connected = len(r.Groups)
	if r.Unlicensed > 0 {
		r.Missing = append(r.Missing, "unresolved distribution rights; candidates are local-only")
	}
	return r
}
func WriteJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
func Publish(rows []Family, out string) error {
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.ID] {
			return fmt.Errorf("duplicate candidate ID")
		}
		seen[r.ID] = true
		if err := Validate(r); err != nil {
			return err
		}
		if r.License == "NOASSERTION" {
			return fmt.Errorf("unresolved license: cannot publish candidates")
		}
	}
	if err := os.Mkdir(out, 0755); err != nil {
		return err
	}
	for _, typ := range types {
		var b bytes.Buffer
		for _, r := range rows {
			if r.Type == typ {
				x, err := json.Marshal(r)
				if err != nil {
					return err
				}
				b.Write(x)
				b.WriteByte('\n')
			}
		}
		if b.Len() == 0 {
			continue
		}
		if err := os.WriteFile(filepath.Join(out, typ+".jsonl"), b.Bytes(), 0644); err != nil {
			return err
		}
	}
	return WriteJSON(filepath.Join(out, "coverage.json"), Report(rows))
}
