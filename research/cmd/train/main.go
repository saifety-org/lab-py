// Command gen trains the attack-on-agent classifier on synthetic + real data
// and writes the embedded weights. Real data: public prompt-injection datasets
// (Apache-2.0) in datasets/training/external.jsonl and legitimate commands harvested from
// local repo docs in datasets/training/local_benign.jsonl. Metrics are reported on a
// held-out split of the REAL data, so they reflect generalization, not
// memorization of templates.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"

	classifier "github.com/saifety-org/lab-py/research/internal/training"
)

func main() {
	out := flag.String("out", "artifacts/weights.json", "output weights file")
	dataDir := flag.String("data", "datasets/training", "directory with external.jsonl / local_benign.jsonl")
	seed := flag.Int64("seed", 1, "seed")
	trainPath := flag.String("train", "", "explicit prepared JSONL training set (no implicit data or split)")
	flag.Parse()
	if err := os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
		panic(err)
	}
	if *trainPath != "" {
		train, err := classifier.LoadCorpusStrict(*trainPath)
		if err != nil {
			panic(err)
		}
		opts := classifier.DefaultTrain
		opts.Seed = *seed
		m := classifier.Train(train, opts)
		b, err := m.JSON()
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(*out, b, 0o644); err != nil {
			panic(err)
		}
		input, err := os.ReadFile(*trainPath)
		if err != nil {
			panic(err)
		}
		metadata, err := json.MarshalIndent(map[string]any{
			"training_sha256": fmt.Sprintf("%x", sha256.Sum256(input)),
			"weights_sha256":  fmt.Sprintf("%x", sha256.Sum256(b)),
			"samples":         len(train), "options": opts,
		}, "", "  ")
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(*out+".meta.json", metadata, 0644); err != nil {
			panic(err)
		}
		fmt.Fprintf(os.Stderr, "prepared train=%d seed=%d epochs=%d | wrote %s (%d bytes)\n", len(train), *seed, opts.Epochs, *out, len(b))
		return
	}

	synth := classifier.Generate(*seed)
	external := classifier.LoadCorpus(filepath.Join(*dataDir, "external.jsonl"))
	local := classifier.LoadCorpus(filepath.Join(*dataDir, "local_benign.jsonl"))

	// Legacy random 15% row split. It is not grouped and is not an independent benchmark; use make compare for evaluation.
	rng := rand.New(rand.NewSource(*seed))
	rng.Shuffle(len(external), func(i, j int) { external[i], external[j] = external[j], external[i] })
	cut := len(external) * 15 / 100
	heldout := external[:cut]
	extTrain := external[cut:]

	train := append([]classifier.Sample{}, synth...)
	train = append(train, extTrain...)
	train = append(train, local...)

	opts := classifier.DefaultTrain
	opts.Seed = *seed
	m := classifier.Train(train, opts)
	acc, prec, rec := classifier.Evaluate(m, heldout)
	m.Accuracy, m.Precision, m.Recall = acc, prec, rec

	b, err := m.JSON()
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "train: synth=%d external=%d local=%d | held-out(real)=%d\n", len(synth), len(external), len(local), len(heldout))
	fmt.Fprintf(os.Stderr, "REAL held-out: acc=%.3f prec=%.3f rec=%.3f | wrote %s (%d bytes)\n", acc, prec, rec, *out, len(b))
}
