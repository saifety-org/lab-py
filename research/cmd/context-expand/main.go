// Command context-expand reproduces reviewed synthetic contextual expansions.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/saifety-org/lab-py/research/internal/contextcorpus"
)

func run(args []string) error {
	f := flag.NewFlagSet("context-expand", flag.ContinueOnError)
	source := f.String("scenarios", "datasets/contextual/v3/scenarios.json", "authored scenario catalog")
	previous := f.String("previous", "datasets/contextual/v2", "reviewed historical corpus")
	out := f.String("out", "", "new output directory (must not exist)")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *out == "" || f.NArg() != 0 {
		return fmt.Errorf("-out is required; positional arguments are unsupported")
	}
	return contextcorpus.Expand(*source, *previous, *out)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
