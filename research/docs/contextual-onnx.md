# Contextual ONNX research bridge

`cmd/context-onnx-bridge` is a resident JSONL research tool, separate from the
application's raw DeBERTa backend. It requires a verified contextual bundle
from prompt-injection-model and a local ONNX Runtime shared library.

```sh
go build -tags onnx -o bin/context-onnx-bridge ./cmd/context-onnx-bridge
bin/context-onnx-bridge -bundle /path/to/bundle -library /path/to/libonnxruntime
```

The first JSON line identifies the manifest/runtime hashes, runtime version
and single-thread configuration. Each request `{"text":"<context-json-v2 JSON>"}`
returns the input hash, complete token IDs, count, score and frozen-threshold
decision. `-mode tokens` counts even overflow inputs without loading ORT.
Score mode rejects >512 tokens, invalid context/trust, unsupported manifests,
checksum failures and inference errors; no truncation or fallback.

Input is validated and preserved byte for byte before adding `query: `.
See the [model contract](https://github.com/saifety-org/prompt-injection-model/tree/main/models/contextual-e5-v4).
`user-provided` never confers system authority. This tool does not execute
instructions, launch an agent or implement an application policy.

## XLM-R tokenizer compatibility

E5-small uses a BERT encoder and XLM-R Unigram tokenizer. The pinned sugarme
v0.3.0 path has two defects relevant here: Precompiled alignment/transformation
can panic for Unicode composition/expansion, and Unigram caps pieces at 20
**bytes**, splitting valid long Cyrillic pieces. The narrow `internal/e5tokenizer`
path keeps the original charmap and vocabulary scores. It follows HF's
[Precompiled text algorithm](https://github.com/huggingface/tokenizers/blob/v0.22.2/tokenizers/src/normalizers/precompiled.rs)
and scores full vocabulary pieces with Viterbi, fusing adjacent unknowns.
It uses sugarme for the remaining tokenizer pipeline.

This path returns **IDs only**; original-text offsets are not supported.
It explicitly accepts Sequence(Precompiled, Replace repeated spaces), Unigram
without byte fallback. It does not substitute NFKC, trim, lowercase, translate,
change checkpoint regexes or alter tokenizer.json. Different layouts require
an explicit contract/implementation. Whole Unicode equivalence is not proven.

Validation: regression tests for >20-byte pieces/unknown fusion, offline tiny
encoder export + Go parity in lab-py CI, and actual E5 parity on 575 already
observed test/Unicode/boundary inputs. Full report and reproduction live in
[lab-py](https://github.com/saifety-org/lab-py/blob/main/docs/contextual-onnx.md).
No datasets or model weights are copied into this repository by this change.
