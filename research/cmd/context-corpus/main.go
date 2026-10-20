// Command context-corpus validates and prepares contextual research fixtures.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/saifety-org/lab-py/research/internal/contextcorpus"
)

func run(args []string) error {
	return execute(args, os.Stdout)
}

func execute(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("context-corpus", flag.ContinueOnError)
	dir := flags.String("source", "datasets/contextual/v4", "corpus directory")
	out := flags.String("out", "", "optional prepared artifact directory")
	reviewOut := flags.String("review-out", "", "new directory for blinded human review packet")
	reviewKey := flags.String("review-key-out", "", "new unblinded answer key file for post-blind reconciliation; keep separate from reviewers")
	reviews := flags.String("reviews", "", "human/model review ledger to validate")
	requireReviewed := flags.Bool("require-reviewed", false, "fail unless declared human/model review is complete (requires -reviews)")
	freeze := flags.String("freeze", "", "verify committed holdout lock before preparing data")
	freezeOut := flags.String("freeze-out", "", "create a new holdout lock; refuses overwrite")
	history := flags.String("history", "", "historical corpus required for holdout verification")
	requireBalanced := flags.Bool("require-task-balanced", false, "reject task/metadata-label shortcuts in supervised splits")
	baselineOut := flags.String("task-baseline-out", "", "new directory for train/validation task-only diagnostic; requires verified existing freeze")
	auditOut := flags.String("task-audit-out", "", "write structural task-balance audit after all gates")
	catalog := flags.String("catalog", "", "reviewed scenario catalog required for holdout verification")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *baselineOut != "" && (*freeze == "" || !*requireReviewed || !*requireBalanced || *out != "") {
		return fmt.Errorf("task baseline requires existing freeze, complete review and task-balance gate; cannot combine with preparation")
	}
	if *auditOut != "" && *reviews == "" {
		return fmt.Errorf("audit output requires reviewed corpus")
	}
	if *auditOut != "" && !*requireBalanced {
		return fmt.Errorf("task audit requires -require-task-balanced")
	}
	locked := *freeze != "" || *freezeOut != ""
	if locked && (*reviews == "" || *history == "" || *catalog == "" || *reviewOut != "" || (*freeze != "" && *freezeOut != "")) {
		return fmt.Errorf("freeze requires -reviews, -history and -catalog; incompatible with review generation or another freeze operation")
	}
	if !locked && (*history != "" || *catalog != "") {
		return fmt.Errorf("-history and -catalog require a freeze operation")
	}
	if *reviewKey != "" && (*reviewOut != "" || *reviews != "" || *out != "" || *requireReviewed || locked) {
		return fmt.Errorf("answer key generation must be a separate operation")
	}
	if *requireReviewed && *reviews == "" {
		return fmt.Errorf("-require-reviewed requires -reviews")
	}
	if *reviewOut != "" && *reviews != "" {
		return fmt.Errorf("packet generation and review validation are separate operations")
	}
	if *out != "" && (*reviewOut != "" || (*reviews != "" && !locked)) {
		return fmt.Errorf("preparation and review are separate operations")
	}
	c, err := contextcorpus.Load(*dir)
	if err != nil {
		return err
	}
	var audit contextcorpus.TaskBalance
	if *requireBalanced {
		audit, err = c.TaskBalanceAudit()
		if err != nil {
			return err
		}
	}
	if *reviewKey != "" {
		return c.WriteReviewKey(*reviewKey)
	}
	if *reviewOut != "" || *reviews != "" {
		p, l := c.BuildReview()
		if *reviews != "" {
			l, err = contextcorpus.LoadReview(*reviews)
			if err != nil {
				return err
			}
		}
		result, err := c.CheckReview(p, l)
		if err != nil {
			return err
		}
		if *reviewOut != "" {
			if err = contextcorpus.WriteReview(p, l, *reviewOut); err != nil {
				return err
			}
		}
		if (*requireReviewed || locked) && result.Status != "complete" {
			if err = json.NewEncoder(output).Encode(result); err != nil {
				return err
			}
			return fmt.Errorf("review incomplete (%d issues)", len(result.Issues))
		}
		if locked {
			h, e := contextcorpus.Load(*history)
			if e != nil {
				return e
			}
			b, e := os.ReadFile(*catalog)
			if e != nil {
				return e
			}
			if *freeze != "" {
				f, e := contextcorpus.LoadFreeze(*freeze)
				if e != nil {
					return e
				}
				if e = contextcorpus.CheckFreeze(c, h, l, b, f); e != nil {
					return e
				}
			} else {
				f, e := contextcorpus.NewFreeze(c, h, l, b, time.Now().UTC().Format(time.RFC3339))
				if e != nil {
					return e
				}
				if e = contextcorpus.WriteFreeze(*freezeOut, f); e != nil {
					return e
				}
			}
		}
		if *auditOut != "" {
			b, e := json.MarshalIndent(audit, "", "  ")
			if e != nil {
				return e
			}
			if e = os.WriteFile(*auditOut, append(b, '\n'), 0644); e != nil {
				return e
			}
		}
		if *baselineOut != "" {
			if e := c.WriteTaskBaseline(*baselineOut); e != nil {
				return e
			}
		}
		if *out != "" {
			if err = contextcorpus.Prepare(c, *out); err != nil {
				return err
			}
		}
		return json.NewEncoder(output).Encode(result)
	}
	if *out != "" {
		if err = contextcorpus.Prepare(c, *out); err != nil {
			return err
		}
	}
	return json.NewEncoder(output).Encode(c.Manifest())
}
func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
