// Command missed-injections reports current scanner verdicts on a development
// corpus of contextual prompt injections. It never executes fixture content.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	injectionmodel "github.com/saifety-org/prompt-injection-model"
	"github.com/saifety-org/sAIfety/pkg/inference"
	"github.com/saifety-org/sAIfety/pkg/scanner"
)

type Case struct {
	ID               string `json:"id"`
	Payload          string `json:"payload"`
	Source           string `json:"source"`
	SourceKind       string `json:"source_kind"`
	Trust            string `json:"trust"`
	Language         string `json:"language"`
	AuthorizedTask   string `json:"authorized_task"`
	ExpectedBehavior string `json:"expected_behavior"`
	ExpectedAction   string `json:"expected_action"`
	Rationale        string `json:"rationale"`
}

type Observation struct {
	Case
	Score     float64 `json:"score"`
	Level     string  `json:"level"`
	Action    string  `json:"action"`
	Findings  int     `json:"findings"`
	Unchanged bool    `json:"payload_unchanged"`
	Missed    bool    `json:"missed"`
}

type Report struct {
	ApplicationModule string        `json:"application_module"`
	ModelModule       string        `json:"model_module"`
	WeightsSHA256     string        `json:"weights_sha256"`
	FeatureSchema     string        `json:"feature_schema"`
	Policy            string        `json:"policy"`
	Threshold         float64       `json:"classifier_threshold"`
	Total             int           `json:"total"`
	Misses            int           `json:"misses"`
	Observations      []Observation `json:"observations"`
}

func readCases(path string) ([]Case, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []Case
	if err = json.Unmarshal(b, &cases); err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("empty case corpus")
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if c.ID == "" || seen[c.ID] || c.Payload == "" || c.Source == "" || c.AuthorizedTask == "" || c.ExpectedBehavior == "" || c.Rationale == "" {
			return nil, fmt.Errorf("missing context or duplicate case: %q", c.ID)
		}
		if c.SourceKind != "tool-result" || c.Trust != "untrusted" || c.ExpectedAction != "sanitize-or-block" {
			return nil, fmt.Errorf("unsupported source or expected action: %q", c.ID)
		}
		seen[c.ID] = true
	}
	return cases, nil
}

func moduleVersion(path string) string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == path {
				if dep.Replace != nil {
					if dep.Replace.Version == "" {
						return "local"
					}
					return dep.Replace.Version
				}
				return dep.Version
			}
		}
	}
	return "unknown"
}

func observe(ctx context.Context, cases []Case) (Report, error) {
	model, err := inference.Default()
	if err != nil {
		return Report{}, err
	}
	sc, err := scanner.New(scanner.Config{Classifier: model, Policy: "strict"})
	if err != nil {
		return Report{}, err
	}
	report := Report{
		ApplicationModule: moduleVersion("github.com/saifety-org/sAIfety"),
		ModelModule:       moduleVersion("github.com/saifety-org/prompt-injection-model"),
		WeightsSHA256:     inference.EmbeddedSHA256(), FeatureSchema: injectionmodel.FeatureSchema,
		Policy: "strict", Threshold: .8, Total: len(cases),
	}
	for _, c := range cases {
		doc := &scanner.Document{Source: c.Source + "/" + c.ID, Kind: scanner.KindToolResult, Raw: c.Payload}
		verdict := sc.Scan(ctx, doc)
		cleaned := c.Payload
		if verdict.Action.String() != "pass" {
			cleaned = scanner.Sanitize(doc, verdict)
		}
		unchanged := cleaned == c.Payload
		missed := verdict.Action.String() == "pass" && unchanged
		report.Observations = append(report.Observations, Observation{
			Case: c, Score: model.Score(c.Payload), Level: verdict.Level.String(), Action: verdict.Action.String(),
			Findings: len(verdict.Findings), Unchanged: unchanged, Missed: missed,
		})
		if missed {
			report.Misses++
		}
	}
	return report, nil
}

func run(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("missed-injections", flag.ContinueOnError)
	data := fs.String("data", "testdata/missed-injections/cases.json", "contextual cases JSON")
	format := fs.String("format", "text", "output format: text or json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *format != "text" && *format != "json" {
		return fmt.Errorf("unknown format %q", *format)
	}
	cases, err := readCases(*data)
	if err != nil {
		return err
	}
	report, err := observe(ctx, cases)
	if err != nil {
		return err
	}
	if *format == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	fmt.Fprintf(out, "app: %s\nmodel: %s\nweights: %s\n", report.ApplicationModule, report.ModelModule, report.WeightsSHA256)
	for _, o := range report.Observations {
		fmt.Fprintf(out, "%-28s score=%.6f level=%s action=%s unchanged=%t missed=%t\n", o.ID, o.Score, o.Level, o.Action, o.Unchanged, o.Missed)
	}
	_, err = fmt.Fprintf(out, "Known-miss development corpus: %d/%d currently pass unchanged; not an independent quality benchmark.\n", report.Misses, report.Total)
	return err
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
