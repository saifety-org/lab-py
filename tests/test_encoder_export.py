"""Closed bundle contracts, real offline encoder export and Go parity."""

import json
import os
from pathlib import Path

import pytest

from saifety_lab_py.encoder_export import CONTRACT, context_input, verify_bundle
from saifety_lab_py.io import write_json
from saifety_lab_py.transformer import checkpoint_hashes


def context(text):
    return json.dumps(
        {
            "contract": "context-json-v2",
            "task": "Summarize",
            "documents": [{"kind": "mcp", "trust": "untrusted", "text": text}],
        },
        ensure_ascii=False,
        separators=(",", ":"),
    )


def test_contract_integrity_and_authority(tmp_path):
    for name in ("model.onnx", "tokenizer.json", "candidate.json", "thresholds-before-test.json"):
        (tmp_path / name).write_text(name)
    manifest = {
        "schema": 1,
        "id": "saifety-contextual-e5-v4",
        "version": "v4-onnx-fp32-1",
        "format": "onnx-fp32-opset17",
        "production_ready": False,
        "contract": CONTRACT,
        "calibration": {
            "threshold": 0.3,
            "decision": "score >= threshold",
            "thresholds_sha256": checkpoint_hashes(tmp_path)["thresholds-before-test.json"],
        },
        "files": checkpoint_hashes(tmp_path),
    }
    write_json(tmp_path / "manifest.json", manifest)
    verify_bundle(tmp_path)
    assert context_input(context("café cafe\u0301")) == "query: " + context("café cafe\u0301")
    with pytest.raises(ValueError, match="trust"):
        context_input(context("data").replace("untrusted", "system"))
    (tmp_path / "model.onnx").write_text("changed")
    with pytest.raises(ValueError, match="checksum"):
        verify_bundle(tmp_path)
    write_json(
        tmp_path / "manifest.json", {**manifest, "contract": {**CONTRACT, "overflow": "truncate"}}
    )
    with pytest.raises(ValueError, match="unsupported"):
        verify_bundle(tmp_path)


@pytest.mark.ml
def test_real_context_encoder_export_and_go_parity(tmp_path):
    import struct

    torch = pytest.importorskip("torch")
    ort = pytest.importorskip("onnxruntime")
    from safetensors.torch import save_file
    from tokenizers import (
        Regex,
        Tokenizer,
        models,
        normalizers,
        pre_tokenizers,
        processors,
        trainers,
    )
    from transformers import BertConfig, BertModel, PreTrainedTokenizerFast

    from saifety_lab_py.encoder import classifier, model_files
    from saifety_lab_py.encoder_export import export_encoder, parity

    bridge = Path(os.environ.get("SAIFETY_CONTEXT_BRIDGE", "bin/context-onnx-bridge"))
    if not bridge.exists():
        pytest.fail("context parity requires bootstrap-encoder-onnx")
    # Tiny identity SentencePiece charmap: all non-NUL labels miss, no external downloads.
    table = struct.pack("<I", 1024) + bytes(1024)
    raw = Tokenizer(models.Unigram())
    raw.normalizer = normalizers.Sequence(
        [normalizers.Precompiled(table), normalizers.Replace(Regex(" {2,}"), " ")]
    )
    raw.pre_tokenizer = pre_tokenizers.Metaspace(replacement="▁", prepend_scheme="always")
    probes = [
        context(s) for s in ("hello world", "русский 中文 español العربية", "café cafe\u0301 🙂")
    ]
    raw.train_from_iterator(
        ["query: " + s for s in probes],
        trainers.UnigramTrainer(
            vocab_size=200, unk_token="<unk>", special_tokens=["<s>", "<pad>", "</s>", "<unk>"]
        ),
    )
    raw.post_processor = processors.TemplateProcessing(
        single="<s> $A </s>", special_tokens=[("<s>", 0), ("</s>", 2)]
    )
    tokenizer = PreTrainedTokenizerFast(
        tokenizer_object=raw,
        bos_token="<s>",
        eos_token="</s>",
        pad_token="<pad>",
        unk_token="<unk>",
        model_input_names=["input_ids", "attention_mask"],
    )
    root = tmp_path / "candidate"
    base = root / "checkpoint"
    base.mkdir(parents=True)
    tokenizer.save_pretrained(base)
    torch.manual_seed(1)
    BertModel(
        BertConfig(
            vocab_size=len(tokenizer),
            hidden_size=16,
            intermediate_size=32,
            num_hidden_layers=1,
            num_attention_heads=2,
            hidden_dropout_prob=0,
            attention_probs_dropout_prob=0,
        )
    ).save_pretrained(base)
    model = classifier(base)
    save_file(model.head.state_dict(), base / "head.safetensors")
    write_json(
        root / "candidate.json",
        {
            "input_contract": "context-json-v2",
            "view": "context",
            "prefix": "query: ",
            "pooling": "masked-mean-l2-normalized",
            "files": model_files(base),
            "protocol": {"max_tokens": 512},
            "diagnostics": {"validation": {"threshold": 0.3}},
        },
    )
    thresholds = tmp_path / "thresholds.json"
    write_json(thresholds, {"encoder_context": 0.3})
    exported = tmp_path / "bundle"
    export_encoder(root, thresholds, Path("encoder.model.lock.json"), exported)
    data = tmp_path / "inputs.jsonl"
    data.write_text("".join(json.dumps({"text": s}, ensure_ascii=False) + "\n" for s in probes))
    library = os.getenv("SAIFETY_ONNX_LIBRARY")
    if library is None:
        library = str(next((Path(ort.__file__).parent / "capi").glob("libonnxruntime.*")))
    report = parity(exported, root, data, bridge, Path(library), tmp_path / "parity.json")
    assert report["passed"] and report["exact_token_ids"] and report["same_decisions"]
    # Overflow is an explicit Go error, never a truncated score.
    import subprocess

    overflow = context("hello " * 600)
    failure = subprocess.run(
        [str(bridge.resolve()), "-bundle", str(exported), "-library", library],
        input=json.dumps({"text": overflow}) + "\n",
        capture_output=True,
        text=True,
        timeout=30,
    )
    assert failure.returncode != 0 and "no truncation" in failure.stderr
    assert len(failure.stdout.splitlines()) == 1  # readiness only; no fabricated score
    with pytest.raises(ValueError, match="overwrite"):
        export_encoder(root, thresholds, Path("encoder.model.lock.json"), exported)
