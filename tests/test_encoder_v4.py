import json
import sys
from pathlib import Path
from types import SimpleNamespace

import pytest
from test_contextual import row

from saifety_lab_py.encoder import compare_encoder, reference_views
from saifety_lab_py.encoder_pairs import matched_task_metrics, provenance_metrics
from saifety_lab_py.evaluate import calibrate
from saifety_lab_py.io import sha256
from saifety_lab_py.sources import lock, prepare_context


def example(identifier, split, label, trust="untrusted", text=None):
    sample = row(identifier, split, split, label)
    content = text or f"{split}-marker document {label}"
    sample["documents"] = [content]
    sample["payload_text"] = content
    sample["text"] = json.dumps(
        {
            "contract": "context-json-v2",
            "task": f"{split}-marker task",
            "documents": [{"kind": "file", "trust": trust, "text": content}],
        }
    )
    return sample


def test_v4_lock_cannot_omit_balance_gate(tmp_path):
    value = json.loads(Path("encoder-v4.sources.lock.json").read_text())
    del value["task_balance_contract"]
    path = tmp_path / "lock.json"
    path.write_text(json.dumps(value))
    with pytest.raises(ValueError, match="task-balance"):
        lock(path)
    assert lock(Path("encoder.sources.lock.json"))["source_corpus"].endswith("v3")


def test_prepare_v4_invokes_canonical_balance_gate(tmp_path, monkeypatch):
    pinned = lock(Path("encoder-v4.sources.lock.json"))
    monkeypatch.setattr("saifety_lab_py.sources.sync", lambda *_: pinned)
    tool = tmp_path / "context-corpus"
    tool.write_text("fixture tool")
    (tmp_path / "tools.json").write_text(
        json.dumps(
            {
                "lab_version": pinned["tools_version"],
                "binaries": {tool.name: sha256(tool)},
            }
        )
    )
    source = tmp_path / "source"
    manifest = source / pinned["source_corpus"] / "manifest.json"
    manifest.parent.mkdir(parents=True)
    manifest.write_text("fixture manifest")
    output = tmp_path / "prepared"

    def run(args, **_kwargs):
        assert "-require-task-balanced" in args
        assert "-require-reviewed" in args and "-freeze" in args
        output.mkdir()
        (output / "manifest.json").write_bytes(manifest.read_bytes())

    monkeypatch.setattr("saifety_lab_py.sources.subprocess.run", run)
    prepare_context(Path("encoder-v4.sources.lock.json"), source, output, tool)


def test_matched_task_and_provenance_metrics_reject_incomplete_pairs():
    values = [
        example("a", "test", 1, text="attack"),
        example("b", "test", 0, text="benign"),
    ]
    result = matched_task_metrics(values, [0.9, 0.1], 0.5)
    assert result["pairs"] == result["both_correct"] == result["attack_score_higher"] == 1
    with pytest.raises(ValueError, match="matched-task"):
        matched_task_metrics(values[:1], [0.9], 0.5)
    values = [example("a", "test", 1), example("b", "test", 1, "user-provided")]
    result = provenance_metrics(values, [0.9, 0.8], 0.5)
    assert result["pairs"] == result["decision_agreement"] == result["both_correct"] == 1
    assert result["mean_absolute_score_delta"] == pytest.approx(0.1)
    values[1]["label"] = 0
    with pytest.raises(ValueError, match="label-changing"):
        provenance_metrics(values, [0.9, 0.8], 0.5)


def test_reference_context_is_identical_to_encoder_input():
    rows = [example("a", "validation", 0)]
    views = reference_views(rows, "context-and-raw")
    for name in ("shipped_context", "deberta_context"):
        assert views[name][1] == [[rows[0]["text"]]]
    assert views["deberta_raw"][1] == [rows[0]["documents"]]


def test_all_thresholds_and_validation_scores_exist_before_test_inference(tmp_path, monkeypatch):
    rows = [
        example(f"{split}{label}", split, label)
        for split in ("validation", "test")
        for label in (0, 1)
    ]
    output, models = tmp_path / "evaluation", tmp_path / "models"
    models.mkdir()
    source_lock = tmp_path / "lock.json"
    source_lock.write_text('{"input_contract":"context-json-v2"}')
    protocol = json.loads(Path("encoder-v4.protocol.json").read_text())
    (models / "experiment.json").write_text(
        json.dumps(
            {
                "corpus": {},
                "source_lock_sha256": sha256(source_lock),
                "protocol": protocol,
                "models": {view: {} for view in ("context", "payload_only", "task_only")},
            }
        )
    )
    monkeypatch.setitem(sys.modules, "torch", SimpleNamespace(set_num_threads=lambda *_: None))
    monkeypatch.setattr(
        "saifety_lab_py.encoder.verified_corpus",
        lambda *_: ([], rows, {"review_status": "complete"}),
    )
    monkeypatch.setattr("saifety_lab_py.encoder.fingerprint", lambda *_: {})
    monkeypatch.setattr("saifety_lab_py.encoder.load_candidate", lambda *_: (None, None, {}))

    def inference(values):
        if "test-marker" in values[0]:
            frozen = json.loads((output / "thresholds-before-test.json").read_text())
            assert len(frozen) == 7
            assert (output / "validation-scores.json").exists()
        return [0.1, 0.9]

    monkeypatch.setattr(
        "saifety_lab_py.encoder.predict",
        lambda _model, _tokenizer, values, _limit: inference(values),
    )

    class Reference:
        identity = {"fixture": True}

        def __init__(self, *_args, **_kwargs):
            pass

        def run(self, values, _mode):
            inference(values)
            return [{"tokens": 100} for _ in values]

        def scores(self, values):
            return inference(values)

    monkeypatch.setattr("saifety_lab_py.bridge.Bridge", Reference)

    # Counterfactual and family reporting have separate tests; keep this test
    # focused on inference ordering and common reference inputs.
    def metrics(selected, scores, budget):
        validation = [r for r in selected if r["split"] == "validation"]
        thresholds = {k: calibrate(validation, v[:2], budget) for k, v in scores.items()}
        return {k: {"validation": {}, "test": {}, "breakdown": {}} for k in scores}, thresholds

    monkeypatch.setattr("saifety_lab_py.encoder.contextual_metrics", metrics)
    compare_encoder(tmp_path, models, output, source_lock, tmp_path, tmp_path, tmp_path)
    assert (output / "test-scores.json").exists()
    with pytest.raises(ValueError, match="one-shot"):
        compare_encoder(tmp_path, models, output, source_lock, tmp_path, tmp_path, tmp_path)
