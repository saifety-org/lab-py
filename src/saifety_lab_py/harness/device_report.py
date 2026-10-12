"""Bind a compact publication candidate to saved device reports and traces, offline."""

import argparse
import json
from collections import Counter
from pathlib import Path

from ..io import sha256, write_json
from .runner import metrics


def compact_run(directory: Path, job: dict, episodes: int) -> dict:
    report_path = directory / "report.json"
    report = json.loads(report_path.read_text(encoding="utf-8"))
    trace = directory / "traces.json"
    records = json.loads(trace.read_text(encoding="utf-8"))
    source = Path(job["source_lock"])
    lock = json.loads(source.read_text(encoding="utf-8"))
    if (
        report.get("schema") != "agent-harness-report-v2"
        or report.get("llm_run") is not True
        or report.get("quality_evidence") is not False
        or report.get("identity_stable") is not True
        or report.get("proxy_decisions_enabled") is not True
        or report["agent"]["model"] != job["model"]
        or report["agent"]["digest"] != job["model_digest"]
        or report["agent"]["version"] != "0.18.0"
        or report["traces_sha256"] != sha256(trace)
        or report["source_lock_sha256"] != sha256(source)
        or sha256(directory / "sources.lock.json") != sha256(source)
        or report["proxy"]["core"] != lock["core"]
        or report["uv_lock_sha256"] != sha256(Path("uv.lock"))
        or report["budget"]["episodes"] != episodes
        or len(records) != episodes
        or report["metrics"] != metrics(records)
        or report["metrics"]["paired"]["complete"] != episodes // 2
        or report["metrics"]["paired"]["incomplete"] != 0
        or any(r["status"] != "finished" for r in records)
    ):
        raise ValueError("device results do not match the pinned complete local-LLM experiment")
    for item in lock["lab"]["files"]:
        if sha256(directory / item["name"]) != item["sha256"]:
            raise ValueError("saved scenario inputs differ from request")
    sources = Path(__file__).parent
    if report["harness_sha256"] != {p.name: sha256(p) for p in sources.glob("*.py")}:
        raise ValueError("harness source changed after execution")
    decisions, detectors = Counter(), Counter()
    for row in records:
        expected = len(row["observations"]) if row["arm"] == "protected" else 0
        if len(row["proxy_decisions"]) != expected:
            raise ValueError("incomplete private proxy decisions")
        for event in row["proxy_decisions"]:
            d = event["decision"]
            decisions[(d["action"], d["reason"], d["delivery"])] += 1
            detectors.update(f["detector"] for f in d.get("findings", []))
    keys = (
        "schema",
        "budget",
        "agent",
        "quality_evidence",
        "scope",
        "identity_stable",
        "proxy",
        "metrics",
        "source_lock_sha256",
        "harness_sha256",
        "uv_lock_sha256",
        "traces_sha256",
        "seconds",
        "llm_run",
        "proxy_decisions_enabled",
    )
    return {
        **{key: report[key] for key in keys},
        "source_report_sha256": sha256(report_path),
        "generation_requests": len(report["usage"]),
        "generated_tokens": sum(u.get("eval_count") or 0 for u in report["usage"]),
        "prompt_tokens": sum(u.get("prompt_eval_count") or 0 for u in report["usage"]),
        "decision_counts": [
            dict(action=action, reason=reason, delivery=delivery, count=count)
            for (action, reason, delivery), count in sorted(decisions.items())
        ],
        "recorded_finding_detector_counts": dict(sorted(detectors.items())),
    }


def summarize(directory: Path) -> Path:
    job_path = Path("device-jobs/proxy-refusal-v2.json")
    job = json.loads(job_path.read_text(encoding="utf-8"))
    metadata_path = directory / "device.json"
    metadata = json.loads(metadata_path.read_text(encoding="utf-8-sig"))
    output = directory / "candidate-report.json"
    if output.exists():
        raise ValueError("publication candidate already exists")
    if (
        metadata["job"] != job["id"]
        or metadata["job_sha256"] != sha256(job_path)
        or metadata["source_lock_sha256"] != sha256(Path(job["source_lock"]))
    ):
        raise ValueError("device receipt differs from request")
    smoke = compact_run(directory / "smoke", job, 8)
    full = compact_run(directory / "full", job, 96)
    if smoke["agent"] != full["agent"] or smoke["proxy"] != full["proxy"]:
        raise ValueError("smoke/full model or proxy identity changed")
    write_json(
        output,
        {
            "schema": "saifety-device-result-v1",
            "job": job["id"],
            "task": job["task"],
            "lab_py_commit": metadata["lab_py_commit"],
            "job_sha256": sha256(job_path),
            "device_receipt_sha256": sha256(metadata_path),
            "gpu_snapshot_sha256": sha256(directory / "gpu.csv"),
            "residency_snapshot_sha256": sha256(directory / "ollama-ps.txt"),
            "quality_evidence": False,
            "historical_reference_sha256": sha256(Path(job["reference"])),
            "smoke": smoke,
            "full": full,
        },
    )
    return output


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run", type=Path, required=True)
    args = parser.parse_args()
    print(summarize(args.run))


if __name__ == "__main__":
    main()
