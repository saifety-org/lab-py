import json
from pathlib import Path

import pytest
from test_contextual import corpus, rewrite_manifest, row

from saifety_lab_py.contextual import input_parts, partitions
from saifety_lab_py.encoder import (
    ap_interval,
    average_precision,
    pair_metrics,
    texts,
    verified_corpus,
)
from saifety_lab_py.io import sha256


def test_v2_provenance_is_not_instruction_authority():
    sample = row("r", "train", "g", 1)
    value = json.loads(sample["text"])
    value["contract"] = "context-json-v2"
    value["documents"][0]["trust"] = "user-provided"
    sample["text"] = json.dumps(value)
    assert input_parts(sample) == ("Summarize it", ["Document data"])
    assert "label" not in json.loads(texts([sample], "context", prefix="")[0])
    value["documents"][0]["trust"] = "system"
    sample["text"] = json.dumps(value)
    with pytest.raises(ValueError, match="trust"):
        input_parts(sample)


def test_mixed_contracts_rejected(tmp_path):
    data = corpus(tmp_path)
    p = data / "train.jsonl"
    values = [json.loads(line) for line in p.read_text().splitlines()]
    context = json.loads(values[0]["text"])
    context["contract"] = "context-json-v2"
    values[0]["text"] = json.dumps(context)
    p.write_text("".join(json.dumps(value) + "\n" for value in values))
    rewrite_manifest(data)
    with pytest.raises(ValueError, match="mixed"):
        partitions(data)


def test_ap_ties_are_order_invariant_and_family_bootstrap():
    rows = [{"label": 1, "group": "a"}, {"label": 0, "group": "b"}]
    assert average_precision(rows, [0.5, 0.5]) == 0.5
    assert average_precision(rows[::-1], [0.5, 0.5]) == 0.5
    assert average_precision(rows, [1, 0]) == 1
    assert average_precision(rows, [0, 1]) == 0.5
    assert ap_interval(rows, [0, 1]) == ap_interval(rows * 3, [0, 1] * 3)


def test_incomplete_counterfactual_pair_fails():
    sample = row("a", "test", "f", 1)
    sample["tags"] = ["authorization-pair"]
    with pytest.raises(ValueError, match="incomplete"):
        pair_metrics([sample], [0.9], 0.5)


def test_verified_corpus_checks_prepared_bytes_again(tmp_path, monkeypatch):
    data = corpus(tmp_path)
    lock = tmp_path / "lock.json"
    lock.write_text('{"input_contract":"context-json-v2"}')

    def fake_prepare(_lock, _source, output, _tool):
        import shutil

        shutil.copytree(data, output, dirs_exist_ok=True)
        (output / "train.jsonl").write_text("changed")

    monkeypatch.setattr("saifety_lab_py.encoder.prepare_context", fake_prepare)
    with pytest.raises(ValueError, match="verified frozen"):
        verified_corpus(data, lock, tmp_path, Path("unused"))


@pytest.mark.ml
def test_real_encoder_fit_reload_and_no_silent_truncation(tmp_path):
    torch = pytest.importorskip("torch")
    from transformers import BertConfig, BertModel, BertTokenizerFast

    from saifety_lab_py.encoder import encoded, fit_view, load_candidate, predict

    base = tmp_path / "base"
    base.mkdir()
    vocab = ["[PAD]", "[UNK]", "[CLS]", "[SEP]", "[MASK]", "query", ":", "Document", "data"]
    (base / "vocab.txt").write_text("\n".join(vocab))
    tokenizer = BertTokenizerFast(vocab_file=str(base / "vocab.txt"))
    tokenizer.save_pretrained(base)
    BertModel(
        BertConfig(
            vocab_size=len(vocab),
            hidden_size=16,
            intermediate_size=32,
            num_hidden_layers=1,
            num_attention_heads=2,
        )
    ).save_pretrained(base)
    protocol = {
        "seed": 1,
        "cpu_threads": 1,
        "training_device": "cpu",
        "epochs": 1,
        "batch_size": 2,
        "learning_rate": 0.001,
        "weight_decay": 0.01,
        "max_tokens": 64,
    }
    training = [row("a", "train", "t", 1), row("b", "train", "t", 0)]
    validation = [row("c", "validation", "v", 1), row("d", "validation", "v", 0)]
    before = sha256(base / "model.safetensors")
    metadata = fit_view(base, training, validation, tmp_path / "fit", "payload_only", protocol)
    assert metadata["end_to_end"]
    assert set(metadata["diagnostics"]) == {"train", "validation"}
    assert metadata["train_validation_predictions_sha256"] == sha256(
        tmp_path / "fit/train-validation-predictions.jsonl"
    )
    assert sha256(tmp_path / "fit/checkpoint/model.safetensors") != before
    model, tokenizer, _ = load_candidate(tmp_path / "fit")
    scores = predict(model, tokenizer, texts(validation, "payload_only"), 64)
    assert len(scores) == 2 and all(0 <= score <= 1 for score in scores)
    with pytest.raises(ValueError, match="token window"):
        encoded(tokenizer, ["data " * 70], 64)
    with pytest.raises(ValueError, match="overwrite"):
        fit_view(base, training, validation, tmp_path / "fit", "payload_only", protocol)
    assert not model.training
    assert torch.isfinite(torch.tensor(scores)).all()


def test_protocol_cannot_silently_drop_ablation_or_change_window():
    from saifety_lab_py.encoder import validate_protocol

    protocol = json.loads(Path("encoder.protocol.json").read_text())
    validate_protocol(protocol)
    with pytest.raises(ValueError, match="protocol"):
        validate_protocol({**protocol, "views": ["context"]})
    with pytest.raises(ValueError, match="protocol"):
        validate_protocol({**protocol, "max_tokens": 1024})


def test_raw_pipeline_rejects_v2_context():
    from saifety_lab_py.transformer import reject_context_input

    with pytest.raises(ValueError, match="dedicated"):
        reject_context_input([{"text": '{"contract":"context-json-v2"}'}])


def test_frozen_encoder_integration():
    import os

    name = os.environ.get("SAIFETY_ENCODER_DATA")
    if not name:
        pytest.skip("set SAIFETY_ENCODER_DATA after prepare-encoder")
    training, evaluation, manifest = partitions(Path(name))
    assert manifest["contract"] == "context-json-v2"
    assert len(training) == 560
    assert len(evaluation) == 1120
    for split in ("validation", "test"):
        selected = [row for row in evaluation if row["split"] == split]
        assert len({row["group"] for row in selected}) == 14
        assert pair_metrics(selected, [0.5] * 560, 0.5)["pairs"] == (70 if split == "test" else 0)
    from saifety_lab_py.encoder_pairs import matched_task_metrics, provenance_metrics

    scores = [0.5] * len(evaluation)
    assert matched_task_metrics(evaluation, scores, 0.5)["pairs"] == 280
    assert provenance_metrics(evaluation, scores, 0.5)["decision_agreement_rate"] == 1
    assert provenance_metrics(evaluation, scores, 0.5)["pairs"] == 280


def test_resident_benchmark_checks_protocol_and_counts(tmp_path):
    import sys

    from saifety_lab_py.encoder_benchmark import resident

    script = tmp_path / "worker.py"
    script.write_text(
        "import json,sys,hashlib\n"
        'print(json.dumps({"ready":True}),flush=True)\n'
        "for line in sys.stdin:\n"
        ' text=json.loads(line)["text"]\n'
        " digest=hashlib.sha256(text.encode()).hexdigest()\n"
        ' print(json.dumps({"score":0.5,"text_sha256":digest}),flush=True)\n'
    )
    pytest.importorskip("psutil")
    result = resident([sys.executable, str(script)], [["one"], ["two", "three"]], warmup=1)
    assert result["requests"] == 6
    assert result["sampled_resident_rss_bytes_max"] > 0


def test_perfect_ap_has_no_spurious_interval_from_float_rounding():
    rows = [{"label": label, "group": str(group)} for group in range(14) for label in (0, 1)]
    values = [0.9 if row["label"] else 0.1 for row in rows]
    assert ap_interval(rows, values) == {"pr_auc_ap": 1.0, "pr_auc_ap_cluster_ci95": None}
