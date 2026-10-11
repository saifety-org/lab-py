"""Frozen-corpus, end-to-end multilingual encoder experiments (research only)."""

import json
import math
import random
import re
import tempfile
from collections import defaultdict
from pathlib import Path

from .contextual import VIEWS, contextual_metrics, fingerprint, partitions, view_text
from .evaluate import calibrate, measure
from .io import sha256, write_json
from .sources import prepare_context
from .transformer import checkpoint_hashes


def verified_corpus(data: Path, lock: Path, source: Path, tool: Path) -> tuple:
    """Re-run canonical review/history/freeze gates before fitting or scoring."""
    pinned = json.loads(lock.read_text())
    if pinned.get("input_contract") != "context-json-v2":
        raise ValueError("encoder requires a reviewed context-json-v2 source lock")
    with tempfile.TemporaryDirectory() as temporary:
        prepared = Path(temporary)
        prepare_context(lock, source, prepared, tool)
        if fingerprint(prepared) != fingerprint(data):
            raise ValueError("prepared corpus differs from verified frozen sources")
    return partitions(data)


def model_files(path: Path) -> dict:
    return {k: v for k, v in checkpoint_hashes(path).items() if not k.startswith(".cache/")}


def verify_model(path: Path, expected: dict) -> None:
    if model_files(path) != expected:
        raise ValueError("checkpoint/tokenizer checksum mismatch")


def fetch_encoder(lock_path: Path, destination: Path) -> None:
    from huggingface_hub import snapshot_download

    identity = json.loads(lock_path.read_text())
    if identity.get("repository") != "intfloat/multilingual-e5-small" or not re.fullmatch(
        r"[0-9a-f]{40}", identity.get("revision", "")
    ):
        raise ValueError("expected an immutable E5 checkpoint revision")
    snapshot_download(
        identity["repository"],
        revision=identity["revision"],
        local_dir=destination,
        token=False,
        allow_patterns=list(identity["files"]),
    )
    verify_model(destination, identity["files"])


def texts(rows: list[dict], view: str, prefix: str = "query: ") -> list[str]:
    return [prefix + view_text(row, view) for row in rows]


def encoded(tokenizer, values: list[str], limit: int):
    if not 1 <= limit <= 512:
        raise ValueError("token limit must be within 1..512")
    result = tokenizer(values, truncation=False, padding=True, return_tensors="pt")
    if result["input_ids"].shape[1] > limit:
        raise ValueError("input exceeds token window; no silent truncation or row exclusion")
    return result


def classifier(base: Path):
    import torch
    from transformers import AutoModel

    class MeanClassifier(torch.nn.Module):
        def __init__(self):
            super().__init__()
            self.encoder = AutoModel.from_pretrained(
                base, local_files_only=True, trust_remote_code=False, use_safetensors=True
            )
            self.head = torch.nn.Linear(self.encoder.config.hidden_size, 2)

        def forward(self, **batch):
            hidden = self.encoder(**batch).last_hidden_state
            mask = batch["attention_mask"].unsqueeze(-1).to(hidden.dtype)
            pooled = (hidden * mask).sum(1) / mask.sum(1).clamp(min=1)
            return self.head(torch.nn.functional.normalize(pooled, dim=-1))

    return MeanClassifier()


def load_candidate(path: Path, device: str = "cpu"):
    from safetensors.torch import load_file
    from transformers import AutoTokenizer

    metadata = json.loads((path / "candidate.json").read_text())
    verify_model(path / "checkpoint", metadata["files"])
    model = classifier(path / "checkpoint")
    model.head.load_state_dict(load_file(path / "checkpoint/head.safetensors"))
    model.to(device).eval()
    tokenizer = AutoTokenizer.from_pretrained(path / "checkpoint", local_files_only=True)
    return model, tokenizer, metadata


def predict(model, tokenizer, values: list[str], limit: int, device: str = "cpu") -> list[float]:
    import torch

    scores = []
    with torch.inference_mode():
        for start in range(0, len(values), 8):
            batch = encoded(tokenizer, values[start : start + 8], limit).to(device)
            scores.extend(model(**batch).softmax(-1)[:, 1].cpu().tolist())
    if any(not math.isfinite(value) or not 0 <= value <= 1 for value in scores):
        raise ValueError("invalid encoder score")
    return scores


def fit_view(
    base: Path,
    training: list[dict],
    validation: list[dict],
    output: Path,
    view: str,
    protocol: dict,
) -> dict:
    import torch
    from safetensors.torch import save_file
    from transformers import AutoTokenizer, set_seed

    if output.exists():
        raise ValueError("candidate output exists; refusing to overwrite a completed experiment")
    set_seed(protocol["seed"])
    torch.set_num_threads(protocol["cpu_threads"])
    device = protocol["training_device"]
    tokenizer = AutoTokenizer.from_pretrained(base, local_files_only=True)
    train_texts, val_texts = texts(training, view), texts(validation, view)
    # Preflight train/validation only, before fitting. Test is checked during final evaluation.
    for values in (train_texts, val_texts):
        for start in range(0, len(values), 8):
            encoded(tokenizer, values[start : start + 8], protocol["max_tokens"])
    model = classifier(base).to(device)
    initial = model.encoder.embeddings.word_embeddings.weight[0].detach().cpu().clone()
    optimizer = torch.optim.AdamW(
        model.parameters(), lr=protocol["learning_rate"], weight_decay=protocol["weight_decay"]
    )
    rng = random.Random(protocol["seed"])
    history = []
    for epoch in range(protocol["epochs"]):
        model.train()
        indexes = list(range(len(training)))
        rng.shuffle(indexes)
        loss_total = 0.0
        for start in range(0, len(indexes), protocol["batch_size"]):
            ids = indexes[start : start + protocol["batch_size"]]
            batch = encoded(tokenizer, [train_texts[i] for i in ids], protocol["max_tokens"])
            labels = torch.tensor([training[i]["label"] for i in ids], device=device).long()
            optimizer.zero_grad(set_to_none=True)
            loss = torch.nn.functional.cross_entropy(model(**batch.to(device)), labels)
            loss.backward()
            torch.nn.utils.clip_grad_norm_(model.parameters(), 1.0)
            optimizer.step()
            loss_total += float(loss.detach().cpu()) * len(ids)
        history.append({"epoch": epoch + 1, "train_loss": loss_total / len(training)})
        print(json.dumps({"view": view, **history[-1]}), flush=True)
    delta = float(
        (model.encoder.embeddings.word_embeddings.weight[0].detach().cpu() - initial).abs().max()
    )
    target = output / "checkpoint"
    model.encoder.to("cpu").save_pretrained(target, safe_serialization=True)
    save_file(
        {k: v.detach().cpu().contiguous() for k, v in model.head.state_dict().items()},
        target / "head.safetensors",
    )
    tokenizer.save_pretrained(target)
    model.to("cpu").eval()
    diagnostic_rows = training + validation
    diagnostic_scores = predict(model, tokenizer, train_texts + val_texts, protocol["max_tokens"])
    prediction_path = output / "train-validation-predictions.jsonl"
    prediction_path.write_text(
        "".join(
            json.dumps({"id": r["id"], "split": r["split"], "score": s}) + "\n"
            for r, s in zip(diagnostic_rows, diagnostic_scores, strict=True)
        )
    )
    threshold = calibrate(
        validation, diagnostic_scores[len(training) :], protocol.get("max_fpr", 0.01)
    )
    diagnostics = {}
    for split, selected, values in (
        ("train", training, diagnostic_scores[: len(training)]),
        ("validation", validation, diagnostic_scores[len(training) :]),
    ):
        metric = measure(selected, values, threshold)
        metric.pop("recall_ci95")
        metric.pop("fpr_ci95")
        diagnostics[split] = {**metric, "pr_auc_ap": average_precision(selected, values)}
    metadata = {
        "input_contract": "context-json-v2" if view == "context" else "raw-text-v1",
        "view": view,
        "prefix": "query: ",
        "pooling": "masked-mean-l2-normalized",
        "files": model_files(target),
        "protocol": protocol,
        "history": history,
        "encoder_embedding_probe_max_delta": delta,
        "end_to_end": True,
        "production_ready": False,
        "diagnostics": diagnostics,
        "train_validation_predictions_sha256": sha256(prediction_path),
    }
    write_json(output / "candidate.json", metadata)
    return metadata


def validate_protocol(protocol: dict) -> None:
    if (
        protocol.get("schema") != 1
        or protocol.get("views") != list(VIEWS)
        or type(protocol.get("epochs")) is not int
        or protocol["epochs"] < 1
        or type(protocol.get("seed")) is not int
        or type(protocol.get("batch_size")) is not int
        or protocol["batch_size"] < 1
        or not 0 < protocol.get("learning_rate", 0) < 1
        or not 0 <= protocol.get("weight_decay", -1) < 1
        or not 1 <= protocol.get("max_tokens", 0) <= 512
        or not 0 <= protocol.get("max_fpr", -1) < 1
        or protocol.get("training_device") not in ("cpu", "mps")
        or protocol.get("cpu_threads") != 1
        or protocol.get("reference_input", "raw") not in ("raw", "context-and-raw")
    ):
        raise ValueError("invalid fixed encoder protocol")


def reference_views(rows: list[dict], mode: str) -> dict:
    views = {"raw": [row["documents"] for row in rows]}
    if mode == "context-and-raw":
        views["context"] = [[row["text"]] for row in rows]
    return {
        f"{name}_{view}": (backend, documents)
        for view, documents in views.items()
        for name, backend in (("shipped", "native"), ("deberta", "onnx"))
    }


def train_encoder(
    data: Path,
    base: Path,
    output: Path,
    protocol_path: Path,
    model_lock: Path,
    source_lock: Path,
    source: Path,
    tool: Path,
) -> dict:
    import torch
    import transformers

    training, evaluation, manifest = verified_corpus(data, source_lock, source, tool)
    validation = [row for row in evaluation if row["split"] == "validation"]
    protocol = json.loads(protocol_path.read_text())
    validate_protocol(protocol)
    identity = json.loads(model_lock.read_text())
    verify_model(base, identity["files"])
    if output.exists():
        raise ValueError("experiment output exists")
    output.mkdir(parents=True)
    # Write immutable experiment identity before any model fit or test inference.
    report = {
        "schema": 1,
        "corpus": fingerprint(data),
        "source_lock_sha256": sha256(source_lock),
        "protocol": protocol,
        "protocol_sha256": sha256(protocol_path),
        "base": identity,
        "versions": {"torch": torch.__version__, "transformers": transformers.__version__},
        "train_rows": len(training),
        "validation_rows": len(validation),
        "review_status": manifest["review_status"],
        "production_ready": False,
        "models": {},
    }
    write_json(output / "experiment.json", report)
    for view in VIEWS:
        report["models"][view] = fit_view(base, training, validation, output / view, view, protocol)
        write_json(output / "experiment.json", report)
    return report


def average_precision(rows: list[dict], scores: list[float]) -> float | None:
    """Non-interpolated PR area (AP); score ties enter as a whole threshold block."""
    positives = sum(row["label"] for row in rows)
    if not positives:
        return None
    blocks = defaultdict(lambda: [0, 0])
    for row, score in zip(rows, scores, strict=True):
        blocks[score][0] += row["label"]
        blocks[score][1] += 1
    tp = count = 0
    terms = []
    for score in sorted(blocks, reverse=True):
        new_positive, n = blocks[score]
        tp += new_positive
        count += n
        terms.append(new_positive / positives * tp / count)
    return min(1.0, math.fsum(terms))


def ap_interval(rows: list[dict], scores: list[float]) -> dict:
    groups = defaultdict(list)
    for i, row in enumerate(rows):
        groups[row["group"]].append(i)
    result = {"pr_auc_ap": average_precision(rows, scores), "pr_auc_ap_cluster_ci95": None}
    if len(groups) < 2:
        return result
    rng = random.Random(1)
    values = []
    for _ in range(1000):
        ids = [i for key in rng.choices(sorted(groups), k=len(groups)) for i in groups[key]]
        value = average_precision([rows[i] for i in ids], [scores[i] for i in ids])
        if value is not None:
            values.append(value)
    if values and max(values) - min(values) > 1e-12:
        values.sort()
        result["pr_auc_ap_cluster_ci95"] = [
            values[int(0.025 * (len(values) - 1))],
            values[int(0.975 * (len(values) - 1))],
        ]
    return result


def pair_metrics(rows: list[dict], scores: list[float], threshold: float) -> dict:
    pairs = defaultdict(list)
    for i, row in enumerate(rows):
        if row["split"] == "test" and "authorization-pair" in row["tags"]:
            pairs[(row["group"], row["language"])].append(i)
    correct = ordered = 0
    families = defaultdict(list)
    for (group, _language), ids in pairs.items():
        if len(ids) != 2 or {rows[i]["label"] for i in ids} != {0, 1}:
            raise ValueError("incomplete authorization pair")
        positive = next(i for i in ids if rows[i]["label"] == 1)
        negative = next(i for i in ids if rows[i]["label"] == 0)
        both = int(scores[positive] >= threshold > scores[negative])
        order = int(scores[positive] > scores[negative])
        correct += both
        ordered += order
        families[group].append((both, order))
    intervals = {}
    if len(families) >= 2:
        rng = random.Random(1)
        observations = [[], []]
        for _ in range(1000):
            sampled = [
                v for key in rng.choices(sorted(families), k=len(families)) for v in families[key]
            ]
            for column in (0, 1):
                observations[column].append(sum(v[column] for v in sampled) / len(sampled))
        for name, values in zip(
            ("both_correct_rate", "attack_score_higher_rate"), observations, strict=True
        ):
            values.sort()
            intervals[name + "_cluster_ci95"] = (
                [values[int(0.025 * (len(values) - 1))], values[int(0.975 * (len(values) - 1))]]
                if len(set(values)) > 1
                else None
            )
    return {
        **intervals,
        "pairs": len(pairs),
        "both_correct": correct,
        "attack_score_higher": ordered,
        "families": len({group for group, _ in pairs}),
    }


def compare_encoder(
    data: Path,
    models: Path,
    output: Path,
    source_lock: Path,
    source: Path,
    tool: Path,
    bridge: Path,
) -> dict:
    import torch

    from .bridge import Bridge

    _, rows, manifest = verified_corpus(data, source_lock, source, tool)
    experiment = json.loads((models / "experiment.json").read_text())
    if experiment["corpus"] != fingerprint(data) or set(experiment["models"]) != set(VIEWS):
        raise ValueError("experiment corpus/models mismatch")
    if output.exists():
        raise ValueError("evaluation output exists; test is a one-shot report")
    output.mkdir(parents=True)
    protocol = experiment["protocol"]
    validate_protocol(protocol)
    if experiment["source_lock_sha256"] != sha256(source_lock):
        raise ValueError("source lock differs from training record")
    torch.set_num_threads(protocol["cpu_threads"])
    scores = {}
    identities = {}
    # All thresholds are committed before any test scoring.
    thresholds = {}
    for split in ("validation", "test"):
        selected = [row for row in rows if row["split"] == split]
        split_scores = {}
        for view in VIEWS:
            candidate, tokenizer, metadata = load_candidate(models / view)
            if metadata != experiment["models"][view]:
                raise ValueError("candidate metadata differs from training record")
            if (
                "train_validation_predictions_sha256" in metadata
                and sha256(models / view / "train-validation-predictions.jsonl")
                != metadata["train_validation_predictions_sha256"]
            ):
                raise ValueError("train/validation predictions changed")
            split_scores[f"encoder_{view}"] = predict(
                candidate, tokenizer, texts(selected, view), protocol["max_tokens"]
            )
            del candidate
        for name, (backend, documents) in reference_views(
            selected, protocol.get("reference_input", "raw")
        ).items():
            flat = [doc for docs in documents for doc in docs]
            reference = Bridge(bridge, backend=backend)
            if backend == "onnx" and any(r["tokens"] > 512 for r in reference.run(flat, "tokens")):
                raise ValueError("DeBERTa token window exceeded; shared cohort cannot be evaluated")
            values = iter(reference.scores(flat))
            split_scores[name] = [max(next(values) for _ in docs) for docs in documents]
            identities[name] = reference.identity
        if split == "validation":
            thresholds = {
                name: calibrate(selected, values, protocol["max_fpr"])
                for name, values in split_scores.items()
            }
            write_json(output / "thresholds-before-test.json", thresholds)
        # Persist each inference pass before derived reporting, so report fixes
        # can use saved scores without running the observed holdout again.
        write_json(
            output / f"{split}-scores.json",
            {
                "ids": [row["id"] for row in selected],
                "scores": split_scores,
                "identities": identities,
            },
        )
        for name, values in split_scores.items():
            scores.setdefault(name, []).extend(values)
    # Prepared corpus order is not assumed; scores above are split-major.
    rows = [row for split in ("validation", "test") for row in rows if row["split"] == split]
    metrics, recomputed = contextual_metrics(rows, scores, protocol["max_fpr"])
    if thresholds != recomputed:
        raise ValueError("threshold drift")
    for name, values in scores.items():
        for split in ("validation", "test"):
            ids = [i for i, row in enumerate(rows) if row["split"] == split]
            metrics[name][split].update(
                ap_interval([rows[i] for i in ids], [values[i] for i in ids])
            )
        for key, metric in metrics[name]["breakdown"].items():
            field, value = key.split(":", 1)
            ids = [
                i for i, row in enumerate(rows) if row["split"] == "test" and row[field] == value
            ]
            metric.update(ap_interval([rows[i] for i in ids], [values[i] for i in ids]))
    report = {
        "schema": 1,
        "input_contract": "context-json-v2",
        "corpus_license": manifest.get("license"),
        "production_ready": False,
        "experiment": experiment,
        "identities": identities,
        "thresholds": thresholds,
        "metrics": metrics,
        "exclusions": [],
        "review_status": manifest["review_status"],
        "reference_policy": protocol["references"],
        "comparison_limits": "Same frozen rows and validation-only FPR rule; context references "
        "receive identical JSON without E5's query prefix. Stock references are not fine-tuned; "
        "this is not an equal-training-budget architecture ranking. Token-window overflow "
        "fails the whole run, with no truncation/exclusions. Raw references are separate.",
        "authorization_pairs_test": {
            name: pair_metrics(rows, values, thresholds[name]) for name, values in scores.items()
        },
    }
    if json.loads(source_lock.read_text()).get("task_balance_contract"):
        from .encoder_pairs import matched_task_metrics, provenance_metrics

        report["matched_task_pairs_test"] = {
            name: matched_task_metrics(rows, values, thresholds[name])
            for name, values in scores.items()
        }
        report["provenance_pairs_test"] = {
            name: provenance_metrics(rows, values, thresholds[name])
            for name, values in scores.items()
        }
    write_json(output / "report.json", report)
    (output / "predictions.jsonl").write_text(
        "".join(
            json.dumps(
                {
                    "id": row["id"],
                    "split": row["split"],
                    "scores": {name: values[i] for name, values in scores.items()},
                },
                allow_nan=False,
            )
            + "\n"
            for i, row in enumerate(rows)
        )
    )
    return report
