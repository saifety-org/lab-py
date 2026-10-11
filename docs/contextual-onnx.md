# Contextual encoder ONNX export and Go parity

This exports the **accepted E5 context checkpoint**, not a new fit, architecture
comparison or quality experiment. E5-small uses a BERT encoder with an XLM-R
Unigram tokenizer, masked mean pooling, L2 normalization and a binary head.
All components are included in a dynamic batch/sequence FP32 opset-17 ONNX graph.

The versioned artifact identity and Go integrity verifier belong to
[prompt-injection-model](https://github.com/saifety-org/prompt-injection-model/tree/main/models/contextual-e5-v4).
Go inference/tokenizer tooling belongs to lab. This repository owns export,
parity, tests and reports. The application still uses its existing native model.

## Reproduce

Use Python 3.12, uv.lock and Go 1.26. The existing v4 training/evaluation
workflow supplies `artifacts/encoder-v4/context`, `data/encoder` and the frozen
`thresholds-before-test.json`; export does not refit or recalibrate them.
Use an empty output path; do not overwrite accepted artifacts or parity reports.

```sh
uv sync --locked --extra ml
make bootstrap-encoder-onnx
uv run --locked --extra ml saifety-lab export-encoder-onnx \
  --candidate artifacts/encoder-v4/context \
  --thresholds artifacts/encoder-v4-comparison/thresholds-before-test.json \
  --out artifacts/contextual-e5-v4
uv run --locked --extra ml saifety-lab encoder-parity-inputs \
  --evaluation data/encoder/evaluation.jsonl \
  --bundle artifacts/contextual-e5-v4 --out artifacts/parity-inputs.jsonl
uv run --locked --extra ml saifety-lab check-encoder-onnx \
  --candidate artifacts/encoder-v4/context --bundle artifacts/contextual-e5-v4 \
  --data artifacts/parity-inputs.jsonl --bridge bin/context-onnx-bridge \
  --library /path/to/libonnxruntime --out artifacts/encoder-parity.json
```

`encoder-onnx.tools.lock.json` pins the research Go bridge separately from the
frozen corpus/training locks; model PR#3 → lab PR#11 → lab-py PR#5 is the merge
order. Go installs immutable commits with GOWORK=off. No local replace, live
`latest` model download or Python runtime dependency is added to the app.

To load a supplied bundle, compare manifest.json with the trusted pinned model
repository manifest (SHA256 recorded in the tools lock), then run its
`verify-context-bundle` command. Both Python and Go verify every file hash and
reject unsupported schema/preprocessing. Hashes authenticate bytes only when
the manifest itself has a trusted origin. Large ONNX/tokenizer files are local
ignored artifacts, not Git data or a hosted release. Safetensors provenance,
base revision and file hashes, corpus version, licenses and calibration are
recorded in the manifest. Base MIT; corpus NOASSERTION; derived distribution
license remains unresolved. There is no binary publication in this change.

## Exact input contract

Only context-json-v2 with task and ordered documents is accepted. Each document
has kind mcp/file/web and trust untrusted/user-provided; the latter conveys no
system authority. Preserve supplied JSON UTF-8 bytes, prepend `query: `, and
apply the unchanged checkpoint tokenizer.json with special tokens. Do not
reserialize, trim, lowercase, translate or independently normalize input.
JSON whitespace, field order and escaping affect model tokens.

Maximum 512 tokens; overflow is an explicit error. There is no silent
truncation, row exclusion, fallback or long-context coverage claim. Softmax
class 1 is compared with the frozen validation-only threshold
0.3123948574066162 using >=. The exporter verifies that threshold against the
accepted candidate metadata. The raw-transformer exporter remains separate.

The dedicated lab ID-only tokenizer path preserves the SentencePiece charmap,
full vocabulary and scores. It addresses sugarme v0.3.0's Unicode Precompiled
and 20-byte Unigram defects; original-text offsets are unsupported. See
[lab tokenizer details](https://github.com/saifety-org/lab/blob/main/docs/contextual-onnx.md).
Transformers emits a known unrelated Mistral-regex warning for this saved
XLM-R tokenizer; no regex patch is applied. Export copies tokenizer files byte
for byte and checks original/export/Go IDs on every parity input.

## Measured result

[Machine-readable report](../reports/contextual-onnx-v4.json): 560 already observed
v4 test inputs plus 12 fixed multilingual/Unicode probes and three inputs of
exactly 510/511/512 tokens. Exact Python/Go token IDs and all frozen-threshold
decisions agree. Max score delta PyTorch→ONNX 7.74e-6, Python ORT→Go ORT 1.52e-6,
within the **predeclared 1e-5** FP32 tolerance. No comparison threshold is retuned.
Different runtime binaries are explicitly hashed/versioned. This is numerical
export validation, **not a fresh blind quality evaluation**.

The report records actual bundle size, sequential CPU latency including Go
tokenization/JSONL IPC, thread counts and sampled RSS. Apple M5, 10 logical CPUs,
24 GiB RAM; Go ORT intra/inter-op 1. Python PyTorch/ORT are also resident.
No same-workload resource comparison with historical DeBERTa measurements is
claimed; first cold request is included, RSS is sampled after replies and does
not represent startup/training/lifetime peak.

CI runs a tiny locally initialized encoder with SentencePiece-style tokenizer:
real export, Python/Go IDs/scores/decisions, checksum/schema rejection and
explicit >512-token Go failure. It downloads no real model and performs no full
research training/evaluation. Run `make check` after preparing the corpora.
Before `make test-ml`, run `uv run --locked saifety-lab bootstrap --onnx`
and `make bootstrap-encoder-onnx`: the legacy native `bootstrap-context`
otherwise overwrites the raw bridge in bin/ without ONNX support.

Observed v4 detector FPR remains 1.43% (>1% target), with authored synthetic
families and no agent ASR measurement. This bundle is research-only: no default
model replacement, application adapter, release or promotion is authorized by
passing numerical parity. Future quality changes need fresh confirmation data.
