package contextcorpus

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"

	"github.com/saifety-org/lab-py/research/internal/evaluation"
	"github.com/saifety-org/lab-py/research/internal/training"
)

// WriteTaskBaseline fits on train tasks and scores train/validation only.
// Callers must verify bound review/frozen history before invoking it.
func (c *Corpus) WriteTaskBaseline(out string) error {
	if _, err := c.TaskBalanceAudit(); err != nil {
		return err
	}
	train := []training.Sample{}
	input := []Sample{}
	for _, r := range c.Rows {
		if r.Split != "train" {
			continue
		}
		train = append(train, training.Sample{Text: r.Task, Label: float64(*r.Label)})
		// Fingerprint only the consumed input, not full text or observed test.
		input = append(input, Sample{ID: r.ID, Group: r.Family, Language: r.Language, Category: r.PrimaryType, Split: r.Split, Text: r.Task, Label: *r.Label})
	}
	opts := training.DefaultTrain
	m := training.Train(train, opts)
	weights, err := m.JSON()
	if err != nil {
		return err
	}
	consumed, err := encode(input)
	if err != nil {
		return err
	}
	predictions := map[string][]evaluation.Prediction{"train": {}, "validation": {}}
	for _, r := range c.Rows {
		if r.Split != "train" && r.Split != "validation" {
			continue
		}
		predictions[r.Split] = append(predictions[r.Split], evaluation.Prediction{
			Sample: evaluation.Sample{ID: r.ID, Text: r.Task, Label: *r.Label, Family: r.Family, Group: r.Family, Category: r.PrimaryType, Language: r.Language, Split: r.Split}, Ours: m.Score(r.Task)})
	}
	threshold := math.Nextafter(1, math.Inf(1))
	bestRecall, bestFPR := -1.0, 2.0
	for _, r := range predictions["validation"] {
		metric := evaluation.Measure(predictions["validation"], "ours", r.Ours)
		if *metric.FPR <= .01 && (*metric.Recall > bestRecall || (*metric.Recall == bestRecall && (*metric.FPR < bestFPR || (*metric.FPR == bestFPR && r.Ours > threshold)))) {
			bestRecall, bestFPR, threshold = *metric.Recall, *metric.FPR, r.Ours
		}
	}
	// The no-alerts candidate has zero recall/FPR and wins over any positive-FPR
	// candidate with zero recall. In this balanced corpus every alert has both labels.
	if bestRecall <= 0 {
		threshold = math.Nextafter(1, math.Inf(1))
	}
	metrics := map[string]any{}
	for split, rows := range predictions {
		measures := map[string]any{}
		for name, t := range map[string]float64{"fixed_0_5": .5, "validation_fpr_0_01": threshold} {
			v := evaluation.Measure(rows, "ours", t)
			measures[name] = map[string]any{"n": v.N, "tp": v.TP, "fp": v.FP, "tn": v.TN, "fn": v.FN,
				"accuracy": v.Accuracy, "recall": v.Recall, "fpr": v.FPR, "pr_auc_ap": v.AveragePrecision, "threshold": t}
		}
		metrics[split] = measures
	}
	report := map[string]any{"schema": 1, "model": "native logistic regression on task text only", "feature_schema": "hashed-char-ngram-v1",
		"execution": map[string]string{"go_version": runtime.Version(), "goos": runtime.GOOS, "goarch": runtime.GOARCH},
		"options":   opts, "train_input_sha256": hash(consumed), "weights_sha256": hash(weights), "source_sha256": c.Sources,
		"metrics": metrics, "test_scored": false, "production_ready": false,
		"limitations": "Diagnostic for task-only leakage; no new encoder fit/test comparison, no guarantee against document/style shortcuts. Dependent translations and provenance variants; no row-wise confidence intervals."}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	// Exclusive output keeps repeated diagnostics from silently overwriting evidence.
	if err = os.Mkdir(out, 0755); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(out, "weights.json"), weights, 0644); err != nil {
		return err
	}
	if err = write(filepath.Join(out, "report.json"), b); err != nil {
		return err
	}
	return nil
}
