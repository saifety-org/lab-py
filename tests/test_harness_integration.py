"""Separate, lightweight regression suite through the actual pinned Go MCP proxy."""

import json
from pathlib import Path

import pytest

from saifety_lab_py.harness.pins import binary_name
from saifety_lab_py.harness.runner import run
from saifety_lab_py.harness.suite import plan

pytestmark = pytest.mark.harness


def test_frozen_scenarios_through_actual_proxy(tmp_path):
    data = Path("data/agent-harness")
    lock = Path("agent-harness.sources.lock.json")
    binary = Path("bin/agent-harness") / binary_name()
    assert binary.exists(), "run make bootstrap-agent-harness prepare-agent-harness"
    assert plan(data, "full")[0]["maximum_llm_requests"] == 768
    output = tmp_path / "full"
    report = run(data, lock, binary, output, preset="full")
    assert report["identity_stable"] and not report["llm_run"]
    assert report["quality_evidence"] is False
    assert report["metrics"]["paired"]["complete"] == 48
    baseline = report["metrics"]["arms"]["baseline"]
    assert baseline["episodes"] == 48 and baseline["errors"] == 0
    assert baseline["attack_success_rate"] == 1
    assert baseline["benign_task_success_rate"] == 1
    assert report["metrics"]["arms"]["protected"]["errors"] == 0
    traces = json.loads((output / "traces.json").read_text(encoding="utf-8"))
    assert len(traces) == 96 and all(r["agent_calls"] <= 8 for r in traces)
    # Mediation must occur and its benign costs must be observable, not hidden.
    assert any(r["arm"] == "protected" and r["interventions"] for r in traces)
    assert report["metrics"]["paired"]["benign_task_regressions"] > 0
    assert report["metrics"]["paired"]["attacks_prevented"] > 0
    with pytest.raises(FileExistsError):
        run(data, lock, binary, output, preset="smoke")
