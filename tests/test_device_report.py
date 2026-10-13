"""Synthetic saved-report fixtures: publication checks, never real LLM evidence."""

import json
from pathlib import Path

import pytest

from saifety_lab_py.harness import device_report
from saifety_lab_py.harness.runner import metrics
from saifety_lab_py.io import sha256, write_json


def saved_fixture(tmp_path):
    directory = tmp_path / "run"
    directory.mkdir()
    files = []
    for name in ("cases.json", "manifest.json"):
        (directory / name).write_text("fixture")
        files.append(dict(name=name, sha256=sha256(directory / name)))
    source = tmp_path / "sources.json"
    lock = {"core": {"commit": "fixture"}, "lab": {"files": files}}
    write_json(source, lock)
    write_json(directory / "sources.lock.json", lock)
    job = dict(source_lock=str(source), model="fixture:local", model_digest="a" * 64)
    rows = [
        dict(
            case="fixture",
            kind="benign",
            seed=1,
            arm=arm,
            status="finished",
            attack_success=None,
            task_success=False,
            permission_violations=0,
            interventions=0,
            observations=[],
            proxy_decisions=[],
            document_reads=[],
        )
        for arm in ("baseline", "protected")
    ]
    write_json(directory / "traces.json", rows)
    report = dict(
        schema="agent-harness-report-v2",
        llm_run=True,
        quality_evidence=False,
        identity_stable=True,
        proxy_decisions_enabled=True,
        agent=dict(model=job["model"], digest=job["model_digest"], version="0.18.0"),
        proxy=dict(core=lock["core"]),
        traces_sha256=sha256(directory / "traces.json"),
        source_lock_sha256=sha256(source),
        uv_lock_sha256=sha256(Path("uv.lock")),
        budget=dict(episodes=2),
        metrics=metrics(rows),
        harness_sha256={
            p.name: sha256(p) for p in Path(device_report.__file__).parent.glob("*.py")
        },
        scope="fixture only",
        seconds=1.0,
        usage=[dict(eval_count=3, prompt_eval_count=12)],
    )
    write_json(directory / "report.json", report)
    return directory, job, report


def test_compact_candidate_retains_binding_and_actual_counts(tmp_path):
    directory, job, _ = saved_fixture(tmp_path)
    result = device_report.compact_run(directory, job, 2)
    assert result["generation_requests"] == 1 and result["generated_tokens"] == 3
    assert result["prompt_tokens"] == 12
    assert result["source_report_sha256"] == sha256(directory / "report.json")
    assert result["metrics"]["paired"]["complete"] == 1
    assert "usage" not in result and "observations" not in result


@pytest.mark.parametrize("change", ["metrics", "trace", "scripted", "digest", "errors", "harness"])
def test_candidate_rejects_changed_or_non_llm_evidence(tmp_path, change):
    directory, job, report = saved_fixture(tmp_path)
    if change == "metrics":
        report["metrics"]["arms"]["protected"]["task_success_rate"] = 1
    elif change == "trace":
        (directory / "traces.json").write_text("[]")
    elif change == "scripted":
        report["llm_run"] = False
    elif change == "digest":
        report["agent"]["digest"] = "b" * 64
    elif change == "harness":
        report["harness_sha256"] = {}
    else:
        rows = json.loads((directory / "traces.json").read_text())
        rows[1]["status"] = "error"
        write_json(directory / "traces.json", rows)
        report["traces_sha256"] = sha256(directory / "traces.json")
        report["metrics"] = metrics(rows)
    write_json(directory / "report.json", report)
    with pytest.raises(ValueError):
        device_report.compact_run(directory, job, 2)
