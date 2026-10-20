# Training data sources

The classifier is trained on a mix of synthetic templates (generated in
`internal/training/data.go`), legitimate commands harvested from local repository docs, and the
following public datasets (used for the attack and benign examples):

- **deepset/prompt-injections** — Apache-2.0 — https://huggingface.co/datasets/deepset/prompt-injections
- **jackhhao/jailbreak-classification** — Apache-2.0 — https://huggingface.co/datasets/jackhhao/jailbreak-classification

`external.jsonl` is a normalized copy (text,label) of the above, redistributed
under Apache-2.0. `local_benign.jsonl` is legitimate command/doc lines
harvested from the operator's own repositories (secrets/PII masked).

Fetch external data with `go run ./cmd/fetch-data` (or the shell wrapper
`scripts/fetch-classifier-data.sh`). The local benign corpus is a historical
masked snapshot; no local harvesting script is included. Train a candidate
with `make train`, or use `make compare` for the independent evaluation protocol.

Legacy training metadata uses a random held-out row split of the external
data. It does not prove generalization or absence of duplicates. The separate
comparison protocol checks overlap, groups examples, and freezes validation/test sets.
