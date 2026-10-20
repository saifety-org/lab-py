// Command context-collect plans, checks and explicitly collects raw candidates.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"time"

	"github.com/saifety-org/lab-py/research/internal/contextcollection"
)

func read(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("input exceeds size limit")
	}
	return b, nil
}
func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("choose plan, check, check-contracts, review-contracts, publish, merge, preload, collect, collect-paired, collect-contracts or collect-contracts-v4")
	}
	cmd := args[0]
	f := flag.NewFlagSet(cmd, flag.ContinueOnError)
	input := f.String("input", "", "candidate JSONL, or pinned protocol for plan")
	out := f.String("out", "", "new output file/directory (never overwritten)")
	from := f.String("from", "", "local checkpoint directory for merge")
	expected := f.String("sha256", "", "expected protocol/plan SHA256")
	endpoint := f.String("ollama-url", "http://127.0.0.1:11435", "literal HTTP loopback")
	model := f.String("model", "", "exact previously installed local tag")
	digest := f.String("model-digest", "", "exact previously installed digest")
	license := f.String("license", "", "operator-selected new-data license")
	start := f.Int("start", 0, "first plan ticket")
	count := f.Int("count", 4, "bounded batch,1..64")
	tokens := f.Int("tokens", 2048, "per-request token cap,256..8192")
	wall := f.Duration("wall-time", 2*time.Hour, "whole job cap, at most2h")
	preloadTimeout := f.Duration("preload-timeout", 5*time.Minute, "empty-prompt load cap, positive and at most15m")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional argument")
	}
	switch cmd {
	case "plan":
		b, err := read(*input, 128<<10)
		if err != nil {
			return err
		}
		if *expected == "" || contextcollection.Hash(b) != *expected {
			return fmt.Errorf("protocol hash differs from accepted pin")
		}
		p, err := contextcollection.MakePlan(b)
		if err != nil {
			return err
		}
		return contextcollection.WriteJSON(*out, p)
	case "check", "check-contracts", "review-contracts", "publish":
		rows, err := contextcollection.Load(*input)
		if err != nil {
			return err
		}
		if cmd == "publish" {
			return contextcollection.Publish(rows, *out)
		}
		if cmd == "check-contracts" {
			report, err := contextcollection.CheckContracts(rows, *from)
			if err != nil {
				return err
			}
			return contextcollection.WriteJSON(*out, report)
		}
		if cmd == "review-contracts" {
			return contextcollection.ReviewContracts(rows, *count, *out)
		}
		return contextcollection.WriteJSON(*out, contextcollection.Report(rows))
	case "merge":
		rows := []contextcollection.Family{}
		if *input != "" {
			var err error
			rows, err = contextcollection.Load(*input)
			if err != nil {
				return err
			}
		}
		paths, err := filepath.Glob(filepath.Join(*from, "candidate-*.json"))
		if err != nil {
			return err
		}
		if len(paths) == 0 || len(paths) > 64 {
			return fmt.Errorf("expected1..64 checkpoint files")
		}
		sort.Strings(paths)
		for _, path := range paths {
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("checkpoint must be a regular file")
			}
			b, err := read(path, contextcollection.MaxFamilyBytes)
			if err != nil {
				return err
			}
			var candidate contextcollection.Family
			if err = contextcollection.Strict(b, &candidate); err != nil {
				return err
			}
			if err = contextcollection.Validate(candidate); err != nil {
				return err
			}
			rows = append(rows, candidate)
		}
		ids := map[string]bool{}
		var b bytes.Buffer
		for _, r := range rows {
			if ids[r.ID] {
				return fmt.Errorf("duplicate checkpoint/candidate ID")
			}
			ids[r.ID] = true
			x, err := json.Marshal(r)
			if err != nil {
				return err
			}
			b.Write(x)
			b.WriteByte('\n')
		}
		file, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		if _, err = file.Write(b.Bytes()); err != nil {
			_ = file.Close()
			return err
		}
		return file.Close()
	case "preload":
		c, err := contextcollection.NewLocalClient(*endpoint, *model, *digest)
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return contextcollection.PreloadWithTimeout(ctx, c, *out, *preloadTimeout)
	case "collect", "collect-paired", "collect-contracts", "collect-contracts-v4":
		if *wall <= 0 || *wall > 2*time.Hour {
			return fmt.Errorf("wall-time must be positive and at most2h")
		}
		b, err := read(*input, 8<<20)
		if err != nil {
			return err
		}
		if *expected == "" || contextcollection.Hash(b) != *expected {
			return fmt.Errorf("plan hash differs from request")
		}
		c, err := contextcollection.NewLocalClient(*endpoint, *model, *digest)
		if err != nil {
			return err
		}
		interrupt, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		ctx, cancel := context.WithTimeout(interrupt, *wall)
		defer cancel()
		if cmd == "collect-contracts-v4" {
			return contextcollection.CollectContractsV4(ctx, c, b, *start, *count, *tokens, *license, *out)
		}
		if cmd == "collect-contracts" {
			return contextcollection.CollectContracts(ctx, c, b, *start, *count, *tokens, *license, *out)
		}
		if cmd == "collect-paired" {
			return contextcollection.CollectPaired(ctx, c, b, *start, *count, *tokens, *license, *out)
		}
		return contextcollection.Collect(ctx, c, b, *start, *count, *tokens, *license, *out)
	default:
		return fmt.Errorf("unknown command")
	}
}
func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
