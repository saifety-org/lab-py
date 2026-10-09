"""Validation-only calibration, paired test inputs and category/language reports."""

import json
import math
from collections import defaultdict
from pathlib import Path

from .bridge import Bridge
from .io import samples, sha256, text_hash, write_json
from .native import NativeModel


def wilson(successes: int, total: int) -> list[float] | None:
    if total == 0:
        return None
    z = 1.959963984540054
    p = successes / total
    denominator = 1 + z * z / total
    center = (p + z * z / (2 * total)) / denominator
    radius = z * math.sqrt(p * (1 - p) / total + z * z / (4 * total * total)) / denominator
    return [center - radius, center + radius]


def measure(rows: list[dict], scores: list[float], threshold: float) -> dict:
    tp = fp = tn = fn = 0
    for row, score in zip(rows, scores, strict=True):
        positive = score >= threshold
        if positive and row["label"] == 1:
            tp += 1
        elif positive:
            fp += 1
        elif row["label"] == 1:
            fn += 1
        else:
            tn += 1

    def ratio(a: int, b: int) -> float | None:
        return a / b if b else None

    return {
        "n": len(rows),
        "tp": tp,
        "fp": fp,
        "tn": tn,
        "fn": fn,
        "precision": ratio(tp, tp + fp),
        "recall": ratio(tp, tp + fn),
        "fpr": ratio(fp, fp + tn),
        "recall_ci95": wilson(tp, tp + fn),
        "fpr_ci95": wilson(fp, fp + tn),
        "threshold": threshold,
    }


def calibrate(rows: list[dict], scores: list[float], max_fpr: float) -> float:
    if not 0 <= max_fpr <= 1 or {r["label"] for r in rows} != {0, 1}:
        raise ValueError("calibration requires both classes and a valid FPR budget")
    candidates = sorted(set(scores) | {math.nextafter(1.0, math.inf)})
    best = None
    for threshold in candidates:
        metric = measure(rows, scores, threshold)
        if metric["fpr"] <= max_fpr:
            key = (metric["recall"], -metric["fpr"], threshold)
            if best is None or key > best[0]:
                best = (key, threshold)
    return best[1]


def compare(
    data: Path,
    weights: Path,
    bridge_path: Path,
    output: Path,
    *,
    max_fpr: float = 0.01,
    deberta: Bridge | None = None,
    translations: Path | None = None,
) -> dict:
    rows = samples(data, evaluation=True)
    model = NativeModel.load(weights)
    metadata = json.loads(Path(str(weights) + ".meta.json").read_text())
    if metadata["weights_sha256"] != sha256(weights):
        raise ValueError("candidate metadata does not match weights")
    exclusions = []
    if deberta is not None:
        counts = deberta.run([row["text"] for row in rows], "tokens")
        selected = []
        for row, count in zip(rows, counts, strict=True):
            if count["tokens"] > 512:
                exclusions.append({"id": row["id"], "reason": "over_shared_512_token_window"})
            else:
                selected.append(row)
        rows = selected
    texts = [row["text"] for row in rows]
    feature_provider = Bridge(bridge_path)
    vectors = feature_provider.features(texts)
    python_scores = [model.score(vector) for vector in vectors]
    native_candidate = Bridge(bridge_path, weights=weights)
    go_scores = native_candidate.scores(texts)
    delta = max((abs(a - b) for a, b in zip(python_scores, go_scores, strict=True)), default=0)
    if delta > 1e-12:
        raise ValueError(f"Python/Go candidate prediction mismatch: {delta}")
    scores = {"candidate": go_scores, "shipped": feature_provider.scores(texts)}
    if deberta is not None:
        scores["deberta"] = deberta.scores(texts)
    if translations is not None:
        from .io import read_jsonl

        translated = {row["id"]: row for row in read_jsonl(translations)}
        translated_texts = []
        for row in rows:
            value = translated.get(row["id"])
            if value is None or value.get("text_sha256") != text_hash(row["text"]):
                raise ValueError("translation is missing or does not match original")
            if not isinstance(value.get("translated_text"), str) or not value["translated_text"]:
                raise ValueError("empty translation")
            translated_texts.append(value["translated_text"])
        translated_scores = native_candidate.scores(translated_texts)
        scores["candidate_with_translation"] = [
            max(a, b) for a, b in zip(go_scores, translated_scores, strict=True)
        ]
    metrics, thresholds = summarize(rows, scores, max_fpr)
    output.mkdir(parents=True, exist_ok=True)
    with (output / "predictions.jsonl").open("w") as target:
        for i, row in enumerate(rows):
            target.write(
                json.dumps(
                    {**row, "scores": {k: v[i] for k, v in scores.items()}},
                    ensure_ascii=False,
                    allow_nan=False,
                )
                + "\n"
            )
    report = {
        "schema": 1,
        "evaluation_sha256": sha256(data),
        "weights_sha256": sha256(weights),
        "training": metadata,
        "feature_provider": feature_provider.identity,
        "candidate": native_candidate.identity,
        "deberta": deberta.identity if deberta else None,
        "python_go_max_delta": delta,
        "validation_fpr_budget": max_fpr,
        "thresholds": thresholds,
        "metrics": metrics,
        "exclusions": exclusions,
        "translations_sha256": sha256(translations) if translations else None,
        "protocol": "paired raw inputs; validation-only calibration; text-only evaluation",
        "limitations": [
            "existing BIPIA labels lack authorized-task context",
            "scores are not measured agent attack success",
            "latency must be measured in the Go benchmark, not cross-process timings",
        ],
    }
    write_json(output / "report.json", report)
    return report


def summarize(
    rows: list[dict], scores: dict[str, list[float]], max_fpr: float
) -> tuple[dict, dict]:
    valid_idx = [i for i, row in enumerate(rows) if row["split"] == "validation"]
    test_idx = [i for i, row in enumerate(rows) if row["split"] == "test"]
    if not valid_idx or not test_idx:
        raise ValueError("both frozen validation and test splits are required")
    metrics = {}
    thresholds = {}
    for name, values in scores.items():
        validation = [rows[i] for i in valid_idx]
        threshold = calibrate(validation, [values[i] for i in valid_idx], max_fpr)
        thresholds[name] = threshold
        test = [rows[i] for i in test_idx]
        groups: dict[str, list[int]] = defaultdict(list)
        for i in test_idx:
            groups[f"language:{rows[i]['language']}"].append(i)
            groups[f"category:{rows[i]['category']}"].append(i)
        metrics[name] = {
            "validation": measure(validation, [values[i] for i in valid_idx], threshold),
            "operating_point_status": "no_alerts" if threshold > 1 else "calibrated",
            "test": measure(test, [values[i] for i in test_idx], threshold),
            "breakdown": {
                key: measure([rows[i] for i in indexes], [values[i] for i in indexes], threshold)
                for key, indexes in sorted(groups.items())
            },
        }
    return metrics, thresholds


def compare_onnx(
    data: Path,
    bundle: Path,
    parity: Path,
    bridge: Path,
    library: Path,
    output: Path,
    *,
    max_fpr: float = 0.01,
    deberta: Bridge | None = None,
) -> dict:
    manifest = json.loads((bundle / "manifest.json").read_text())
    verified = json.loads(parity.read_text())
    if (
        verified.get("passed") is not True
        or verified["model_sha256"] != sha256(bundle / "model.onnx")
        or verified["manifest_sha256"] != sha256(bundle / "manifest.json")
        or verified["runtime_sha256"] != sha256(library)
    ):
        raise ValueError("matching successful Go/ONNX parity report is required")
    for name, expected in manifest["files"].items():
        if sha256(bundle / name) != expected:
            raise ValueError("exported bundle checksum mismatch")
    candidate = Bridge(
        bridge,
        backend="onnx",
        model=bundle / "model.onnx",
        tokenizer=bundle / "tokenizer.json",
        library=library,
        attack_index=manifest["attack_index"],
    )
    rows = samples(data, evaluation=True)
    counts = candidate.run([row["text"] for row in rows], "tokens")
    other_counts = deberta.run([row["text"] for row in rows], "tokens") if deberta else counts
    selected = []
    exclusions = []
    for row, count, other in zip(rows, counts, other_counts, strict=True):
        if count["tokens"] > min(512, manifest["max_tokens"]) or other["tokens"] > 512:
            exclusions.append({"id": row["id"], "reason": "over_shared_token_window"})
        else:
            selected.append(row)
    rows = selected
    texts = [row["text"] for row in rows]
    shipped = Bridge(bridge)
    scores = {"candidate": candidate.scores(texts), "shipped": shipped.scores(texts)}
    if deberta is not None:
        scores["deberta"] = deberta.scores(texts)
    metrics, thresholds = summarize(rows, scores, max_fpr)
    output.mkdir(parents=True, exist_ok=True)
    with (output / "predictions.jsonl").open("w") as target:
        for i, row in enumerate(rows):
            target.write(
                json.dumps(
                    {**row, "scores": {k: v[i] for k, v in scores.items()}},
                    ensure_ascii=False,
                    allow_nan=False,
                )
                + "\n"
            )
    report = {
        "schema": 1,
        "evaluation_sha256": sha256(data),
        "manifest": manifest,
        "parity": verified,
        "candidate": candidate.identity,
        "shipped": shipped.identity,
        "deberta": deberta.identity if deberta else None,
        "exclusions": exclusions,
        "thresholds": thresholds,
        "metrics": metrics,
        "validation_fpr_budget": max_fpr,
        "protocol": "paired raw inputs; validation-only calibration; text-only evaluation",
        "limitations": [
            "existing BIPIA labels lack authorized-task context",
            "not an agent attack-success measurement",
        ],
    }
    write_json(output / "report.json", report)
    return report
