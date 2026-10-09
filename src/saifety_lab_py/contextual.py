"""Context-aware research inputs and paired ablations; never promote weights."""

import json
import random
from collections import Counter, defaultdict
from pathlib import Path

from .bridge import Bridge
from .evaluate import measure, summarize
from .io import samples, sha256, write_json
from .native import NativeModel, train

CONTRACT = "context-json-v1"
VIEWS = ("context", "payload_only", "task_only")


def input_parts(row: dict) -> tuple[str, list[str]]:
    value = json.loads(row["text"])
    if (
        not isinstance(value, dict)
        or set(value) != {"contract", "task", "documents"}
        or value["contract"] != CONTRACT
        or not isinstance(value["task"], str)
        or not value["task"].strip()
        or not isinstance(value["documents"], list)
        or not value["documents"]
    ):
        raise ValueError("invalid context input contract")
    texts = []
    for doc in value["documents"]:
        if (
            not isinstance(doc, dict)
            or set(doc) != {"kind", "trust", "text"}
            or doc["kind"] not in ("mcp", "file", "web")
            or doc["trust"] != "untrusted"
            or not isinstance(doc["text"], str)
            or not doc["text"].strip()
        ):
            raise ValueError("invalid document or elevated trust")
        texts.append(doc["text"])
    if row.get("documents") != texts or row.get("payload_text") != "\n\n".join(texts):
        raise ValueError("raw/context views do not match")
    return value["task"], texts


def partitions(directory: Path) -> tuple[list[dict], list[dict], dict]:
    manifest = json.loads((directory / "manifest.json").read_text())
    if manifest.get("contract") != CONTRACT:
        raise ValueError("unsupported contextual corpus")
    names = {"train.jsonl", "evaluation.jsonl", "development.jsonl"}
    if set(manifest.get("output_sha256", {})) != names:
        raise ValueError("incomplete corpus manifest")
    for name in names:
        if sha256(directory / name) != manifest["output_sha256"][name]:
            raise ValueError("corpus checksum mismatch")
    training = samples(directory / "train.jsonl")
    evaluation = samples(directory / "evaluation.jsonl", evaluation=True)
    development = samples(directory / "development.jsonl")
    ids = set()
    groups = {}
    for expected, rows in (
        ("train", training),
        ("evaluation", evaluation),
        ("development", development),
    ):
        for row in rows:
            if not all(
                isinstance(row.get(k), str) and row[k]
                for k in ("id", "group", "split", "category", "language")
            ):
                raise ValueError("missing contextual metadata")
            if row["id"] in ids:
                raise ValueError("duplicate contextual id")
            ids.add(row["id"])
            if expected != "evaluation" and row["split"] != expected:
                raise ValueError("development/test cannot enter training")
            previous = groups.setdefault(row["group"], row["split"])
            if previous != row["split"]:
                raise ValueError("contextual family crosses splits")
            input_parts(row)
    if sum(map(len, (training, evaluation, development))) != manifest.get("rows"):
        raise ValueError("corpus row count mismatch")
    return training, evaluation, manifest


def fingerprint(directory: Path) -> dict:
    return {
        name: sha256(directory / name)
        for name in (
            "manifest.json",
            "source.json",
            "train.jsonl",
            "evaluation.jsonl",
            "development.jsonl",
        )
    }


def view_text(row: dict, view: str) -> str:
    task, _ = input_parts(row)
    return {"context": row["text"], "payload_only": row["payload_text"], "task_only": task}[view]


def train_context(
    directory: Path, output: Path, bridge: Path, *, epochs: int = 40, seed: int = 1
) -> dict:
    training, _, manifest = partitions(directory)
    corpus = fingerprint(directory)
    models = {}
    for view in VIEWS:
        target = output / view
        target.mkdir(parents=True, exist_ok=True)
        path = target / "train.jsonl"
        rows = [{**row, "text": view_text(row, view)} for row in training]
        path.write_text(
            "".join(json.dumps(row, ensure_ascii=False, allow_nan=False) + "\n" for row in rows)
        )
        write_json(
            target / "source.json",
            {
                "corpus": corpus,
                "view": view,
                "source": json.loads((directory / "source.json").read_text()),
            },
        )
        weights = target / "weights.json"
        metadata = train(path, weights, Bridge(bridge), epochs=epochs, seed=seed)
        metadata.update(
            input_contract=CONTRACT if view == "context" else "raw-text-v1",
            view=view,
            corpus=corpus,
        )
        write_json(Path(str(weights) + ".meta.json"), metadata)
        models[view] = {
            "weights_sha256": sha256(weights),
            "metadata_sha256": sha256(Path(str(weights) + ".meta.json")),
        }
    report = {
        "schema": 1,
        "input_contract": CONTRACT,
        "corpus": corpus,
        "models": models,
        "samples": len(training),
        "families": len({r["group"] for r in training}),
        "epochs": epochs,
        "seed": seed,
        "production_ready": False,
        "corpus_review": manifest["review_status"],
    }
    write_json(output / "training.json", report)
    return report


def clustered_metric(rows: list[dict], scores: list[float], threshold: float) -> dict:
    result = measure(rows, scores, threshold)
    # Translations and paired controls are correlated; row-wise Wilson intervals
    # would misrepresent the number of independent scenarios.
    result.pop("recall_ci95")
    result.pop("fpr_ci95")
    groups = defaultdict(list)
    for i, row in enumerate(rows):
        groups[row["group"]].append(i)
    result["families"] = len(groups)
    result["interval_status"] = {}
    for metric in ("recall", "fpr"):
        result[f"{metric}_cluster_ci95"] = None
        result["interval_status"][metric] = "insufficient_families"
    if len(groups) < 2:
        return result
    keys = sorted(groups)
    rng = random.Random(1)
    values = {"recall": [], "fpr": []}
    for _ in range(1000):
        indexes = [i for key in rng.choices(keys, k=len(keys)) for i in groups[key]]
        measurement = measure([rows[i] for i in indexes], [scores[i] for i in indexes], threshold)
        for metric in values:
            if measurement[metric] is not None:
                values[metric].append(measurement[metric])
    for metric, observations in values.items():
        if observations and len(set(observations)) > 1:
            result["interval_status"][metric] = "exploratory_family_bootstrap"
            observations.sort()
            result[f"{metric}_cluster_ci95"] = [
                observations[int(0.025 * (len(observations) - 1))],
                observations[int(0.975 * (len(observations) - 1))],
            ]
    for metric in values:
        if result[f"{metric}_cluster_ci95"] is None:
            result["interval_status"][metric] = "no_observed_variation_not_a_bound"
    return result


def contextual_metrics(
    rows: list[dict], scores: dict[str, list[float]], budget: float
) -> tuple[dict, dict]:
    metrics, thresholds = summarize(rows, scores, budget)
    for name, values in scores.items():
        for split in ("validation", "test"):
            ids = [i for i, row in enumerate(rows) if row["split"] == split]
            metrics[name][split] = clustered_metric(
                [rows[i] for i in ids], [values[i] for i in ids], thresholds[name]
            )
        groups = defaultdict(list)
        for i, row in enumerate(rows):
            if row["split"] != "test":
                continue
            groups[f"category:{row['category']}"].append(i)
            groups[f"language:{row['language']}"].append(i)
        metrics[name]["breakdown"] = {
            key: clustered_metric(
                [rows[i] for i in ids], [values[i] for i in ids], thresholds[name]
            )
            for key, ids in sorted(groups.items())
        }
    return metrics, thresholds


def compare_context(
    directory: Path,
    models: Path,
    bridge_path: Path,
    output: Path,
    *,
    max_fpr: float = 0.01,
    deberta: Bridge | None = None,
) -> dict:
    _, rows, manifest = partitions(directory)
    corpus = fingerprint(directory)
    training = json.loads((models / "training.json").read_text())
    if training.get("corpus") != corpus or training.get("input_contract") != CONTRACT:
        raise ValueError("candidate belongs to a different corpus/contract")
    exclusions = []
    if deberta:
        texts = [text for row in rows for text in row["documents"]]
        counts = iter(deberta.run(texts, "tokens"))
        selected = []
        for row in rows:
            lengths = [next(counts)["tokens"] for _ in row["documents"]]
            if max(lengths) > 512:
                exclusions.append(
                    {"id": row["id"], "reason": "document_over_shared_512_token_window"}
                )
            else:
                selected.append(row)
        rows = selected
    if not rows:
        raise ValueError("no paired rows within shared window")
    scores, identities, deltas = {}, {}, {}
    for view in VIEWS:
        weights = models / view / "weights.json"
        meta_path = Path(str(weights) + ".meta.json")
        expected = training["models"][view]
        if (
            sha256(weights) != expected["weights_sha256"]
            or sha256(meta_path) != expected["metadata_sha256"]
        ):
            raise ValueError("candidate weights/metadata mismatch")
        metadata = json.loads(meta_path.read_text())
        if metadata.get("corpus") != corpus or metadata.get("view") != view:
            raise ValueError("candidate input view mismatch")
        texts = [view_text(row, view) for row in rows]
        candidate = Bridge(bridge_path, weights=weights)
        scores[view] = candidate.scores(texts)
        python_model = NativeModel.load(weights)
        feature_bridge = Bridge(bridge_path)
        python_scores = [python_model.score(vector) for vector in feature_bridge.features(texts)]
        delta = max(abs(a - b) for a, b in zip(scores[view], python_scores, strict=True))
        if delta > 1e-12:
            raise ValueError("context candidate Python/Go mismatch")
        identities[view], deltas[view] = candidate.identity, delta
    flat = [text for row in rows for text in row["documents"]]

    def bundle(values: list[float]) -> list[float]:
        it = iter(values)
        return [max(next(it) for _ in row["documents"]) for row in rows]

    shipped = Bridge(bridge_path)
    scores["shipped_raw"] = bundle(shipped.scores(flat))
    identities["shipped_raw"] = shipped.identity
    if deberta:
        scores["deberta_raw"] = bundle(deberta.scores(flat))
        identities["deberta_raw"] = deberta.identity
    metrics, thresholds = contextual_metrics(rows, scores, max_fpr)
    paired = defaultdict(list)
    for i, row in enumerate(rows):
        if row["split"] == "test" and "authorization-pair" in row["tags"]:
            paired[(row["group"], row["language"])].append(i)
    pair_results = {}
    for name, values in scores.items():
        correct = ordered = 0
        for indexes in paired.values():
            if len(indexes) != 2 or {rows[i]["label"] for i in indexes} != {0, 1}:
                raise ValueError("invalid authorization pair")
            positive = next(i for i in indexes if rows[i]["label"] == 1)
            negative = next(i for i in indexes if rows[i]["label"] == 0)
            correct += int(values[positive] >= thresholds[name] > values[negative])
            ordered += int(values[positive] > values[negative])
        pair_results[name] = {
            "pairs": len(paired),
            "both_correct": correct,
            "attack_score_higher": ordered,
            "independent_families": len({group for group, _ in paired}),
        }
    scanner = Bridge(bridge_path)
    observed = scanner.run(flat, "scan")
    actions = [value["verdict"]["action"] for value in observed]
    if any(action not in ("pass", "warn", "sanitize", "block") for action in actions):
        raise ValueError("unexpected scanner action")
    scanner_scores = {
        "nonpass": bundle([float(action != "pass") for action in actions]),
        "sanitize_or_block": bundle([float(action in ("sanitize", "block")) for action in actions]),
    }
    test_indexes = [i for i, row in enumerate(rows) if row["split"] == "test"]
    scanner_metrics = {}
    for name, values in scanner_scores.items():
        groups = defaultdict(list)
        for i in test_indexes:
            groups[f"category:{rows[i]['category']}"].append(i)
            groups[f"language:{rows[i]['language']}"].append(i)
        scanner_metrics[name] = {
            "test": clustered_metric(
                [rows[i] for i in test_indexes], [values[i] for i in test_indexes], 0.5
            ),
            "breakdown": {
                key: clustered_metric([rows[i] for i in ids], [values[i] for i in ids], 0.5)
                for key, ids in sorted(groups.items())
            },
        }
    output.mkdir(parents=True, exist_ok=True)
    (output / "predictions.jsonl").write_text(
        "".join(
            json.dumps(
                {
                    **row,
                    "scores": {name: values[i] for name, values in scores.items()},
                    "scanner": {name: values[i] for name, values in scanner_scores.items()},
                },
                ensure_ascii=False,
                allow_nan=False,
            )
            + "\n"
            for i, row in enumerate(rows)
        )
    )
    report = {
        "schema": 1,
        "input_contract": CONTRACT,
        "production_ready": False,
        "corpus": corpus,
        "review_status": manifest["review_status"],
        "training": training,
        "models": identities,
        "python_go_max_delta": deltas,
        "validation_fpr_budget": max_fpr,
        "thresholds": thresholds,
        "metrics": metrics,
        "scanner": {
            "policy": "strict",
            "document_kind": "tool-result",
            "aggregation": "any document",
            "identity": scanner.identity,
            "actions": dict(Counter(actions)),
            "metrics": scanner_metrics,
        },
        "exclusions": exclusions,
        "authorization_pairs_test": pair_results,
        "protocol": (
            "same scenario splits; separately trained context/payload/task ablations; "
            "validation-only thresholds; raw shipped/DeBERTa are context-blind references"
        ),
        "intervals": (
            "exploratory whole-family percentile bootstrap, seed=1, 1000 replicates; "
            "single-family and degenerate intervals omitted; not a low-FPR guarantee"
        ),
        "limitations": [
            "small authored synthetic corpus; independent human review pending",
            "translations and paired controls are not independent",
            "raw references lack task context; no universal model ranking",
            (
                "scanner receives each document as tool-result, "
                "not native carrier or full application context"
            ),
            "no agent attack-success/task-completion measurement",
            "context weights are incompatible with raw-input production without an adapter",
        ],
    }
    write_json(output / "report.json", report)
    return report
