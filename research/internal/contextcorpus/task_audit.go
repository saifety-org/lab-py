package contextcorpus

import (
	"fmt"
	"sort"
	"strings"
)

type TaskBalance struct {
	Contract                   string             `json:"contract"`
	Cells                      map[string]int     `json:"balanced_cells"`
	Rows                       map[string]int     `json:"supervised_rows"`
	TaskOnlyAccuracyUpperBound map[string]float64 `json:"empirical_task_only_accuracy_upper_bound"`
	AuthorizationPairs         map[string]int     `json:"authorization_pairs"`
	Policy                     string             `json:"policy"`
}

// TaskBalanceAudit is a structural audit, not model scoring on holdout.
// Balance is required within family/language and caller-supplied input metadata,
// so task wording or provenance alone cannot predict supervised labels.
func (c *Corpus) TaskBalanceAudit() (TaskBalance, error) {
	out := TaskBalance{Contract: "context-task-balance-v1", Cells: map[string]int{}, Rows: map[string]int{}, TaskOnlyAccuracyUpperBound: map[string]float64{}, AuthorizationPairs: map[string]int{},
		Policy: "Development history excluded; require both labels in every exact/canonical task and family/language/kind/trust/document-count cell. This removes marginal task/metadata shortcuts, not all content/semantic shortcuts or dependence."}
	cells := map[string][2]int{}
	tasks := map[string][2]int{}
	pairs := map[string][]Record{}
	groups := map[string]bool{}
	sources := map[string][2]int{}
	sourceLabels := map[string]int{}
	for _, r := range c.Rows {
		if r.Split == "development" {
			continue
		}
		if r.Label == nil || (*r.Label != 0 && *r.Label != 1) || len(r.Documents) == 0 {
			return out, fmt.Errorf("invalid supervised row")
		}
		group := r.Split + "/" + r.Family + "/" + r.Language
		groups[group] = true
		normalized := append([]Document(nil), r.Documents...)
		trust := r.Documents[0].Trust
		if trust != "untrusted" && trust != "user-provided" {
			return out, fmt.Errorf("unsupported provenance")
		}
		for i := range normalized {
			if normalized[i].Trust != trust {
				return out, fmt.Errorf("v4 provenance pair requires uniform source trust")
			}
			normalized[i].Trust = "untrusted"
		}
		raw, err := encode(normalized)
		if err != nil {
			return out, err
		}
		sourceKey := group + "/" + r.Task + "/" + string(raw)
		if label, ok := sourceLabels[sourceKey]; ok && label != *r.Label {
			return out, fmt.Errorf("provenance changed label")
		}
		sourceLabels[sourceKey] = *r.Label
		counts := sources[sourceKey]
		if trust == "untrusted" {
			counts[0]++
		} else {
			counts[1]++
		}
		sources[sourceKey] = counts
		out.Rows[r.Split]++
		parts := []string{}
		for _, d := range r.Documents {
			parts = append(parts, d.Kind+"/"+d.Trust)
		}
		metadata := strings.Join(parts, ",")
		for _, task := range []string{r.Task, canonical(r.Task)} {
			key := r.Split + "/" + r.Family + "/" + r.Language + "/" + metadata + "/" + task
			counts := cells[key]
			counts[*r.Label]++
			cells[key] = counts
		}
		key := r.Split + "/" + r.Task
		counts = tasks[key]
		counts[*r.Label]++
		tasks[key] = counts
		for _, tag := range r.Tags {
			if tag == "authorization-pair" {
				pairs[r.Split+"/"+r.Family+"/"+r.Language] = append(pairs[r.Split+"/"+r.Family+"/"+r.Language], r)
			}
		}
	}
	keys := make([]string, 0, len(cells))
	for key := range cells {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		counts := cells[key]
		if counts[0] == 0 || counts[0] != counts[1] {
			return out, fmt.Errorf("task-label shortcut: %s has benign=%d attack=%d", key, counts[0], counts[1])
		}
		out.Cells[strings.SplitN(key, "/", 2)[0]]++
	}
	correct := map[string]int{}
	for key, counts := range tasks {
		correct[strings.SplitN(key, "/", 2)[0]] += max(counts[0], counts[1])
	}
	for _, split := range []string{"train", "validation", "test"} {
		if out.Rows[split] == 0 {
			return out, fmt.Errorf("missing balanced split %s", split)
		}
		out.TaskOnlyAccuracyUpperBound[split] = float64(correct[split]) / float64(out.Rows[split])
	}
	for key := range groups {
		if len(pairs[key]) != 2 {
			return out, fmt.Errorf("missing authorization pair %s", key)
		}
	}
	for key, counts := range sources {
		if counts[0] != 1 || counts[1] != 1 {
			return out, fmt.Errorf("missing/duplicate provenance pair %s", key)
		}
	}
	for key, rows := range pairs {
		if len(rows) != 2 || *rows[0].Label == *rows[1].Label || rows[0].Task == rows[1].Task {
			return out, fmt.Errorf("invalid authorization pair %s", key)
		}
		a, _ := encode(rows[0].Documents)
		b, _ := encode(rows[1].Documents)
		if string(a) != string(b) {
			return out, fmt.Errorf("authorization pair changed payload %s", key)
		}
		out.AuthorizationPairs[strings.SplitN(key, "/", 2)[0]]++
	}
	return out, nil
}
