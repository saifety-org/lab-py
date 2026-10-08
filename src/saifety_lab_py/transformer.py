"""Optional local transformer fine-tuning and export. Imports stay out of base CLI."""

import json
import unicodedata
from pathlib import Path

from .bridge import Bridge
from .io import samples, sha256, text_hash, write_json


def canonical(text: str) -> str:
    return " ".join(unicodedata.normalize("NFKC", text).casefold().split())


def validate_partitions(train: list[dict], validation: list[dict]) -> None:
    if any(row.get("split", "train") != "train" for row in train):
        raise ValueError("training input contains a validation/test row")
    if any(row.get("split", "validation") != "validation" for row in validation):
        raise ValueError("validation input contains another split")
    if {canonical(r["text"]) for r in train} & {canonical(r["text"]) for r in validation}:
        raise ValueError("training/validation text leakage")
    train_groups = {r["group"] for r in train if r.get("group")}
    if train_groups & {r["group"] for r in validation if r.get("group")}:
        raise ValueError("training/validation scenario-group leakage")


def checkpoint_hashes(path: Path) -> dict:
    files = {str(p.relative_to(path)): sha256(p) for p in sorted(path.rglob("*")) if p.is_file()}
    if not files:
        raise ValueError("empty local checkpoint")
    return files


def train_transformer(
    model_path: Path,
    train_path: Path,
    validation_path: Path,
    output: Path,
    *,
    epochs: float = 1,
    seed: int = 1,
    batch_size: int = 8,
    max_tokens: int = 512,
    allow_training_truncation: bool = False,
) -> dict:
    import torch
    from transformers import (
        AutoModelForSequenceClassification,
        AutoTokenizer,
        DataCollatorWithPadding,
        Trainer,
        TrainingArguments,
        set_seed,
    )

    if epochs <= 0 or batch_size < 1 or not 1 <= max_tokens <= 512:
        raise ValueError("invalid training parameters")
    if output.exists() and any(output.iterdir()):
        raise ValueError("output directory must be empty; do not overwrite a checkpoint")
    train_rows = samples(train_path)
    all_validation = samples(validation_path)
    validation_rows = [r for r in all_validation if r.get("split", "validation") == "validation"]
    if not validation_rows or {r["label"] for r in train_rows} != {0, 1}:
        raise ValueError("training requires both classes and nonempty validation")
    validate_partitions(train_rows, validation_rows)
    source_hashes = checkpoint_hashes(model_path)
    set_seed(seed)
    torch.set_num_threads(1)
    tokenizer = AutoTokenizer.from_pretrained(
        model_path, local_files_only=True, trust_remote_code=False, use_fast=True
    )
    model = AutoModelForSequenceClassification.from_pretrained(
        model_path,
        local_files_only=True,
        trust_remote_code=False,
        use_safetensors=True,
        num_labels=2,
        id2label={0: "benign", 1: "attack"},
        label2id={"benign": 0, "attack": 1},
    )
    truncated = 0

    class Dataset(torch.utils.data.Dataset):
        def __init__(self, rows: list[dict], allow_truncation: bool):
            nonlocal truncated
            self.items = []
            for row in rows:
                tokens = tokenizer(row["text"], truncation=False)
                if len(tokens["input_ids"]) > max_tokens:
                    if not allow_truncation:
                        raise ValueError(
                            "sample exceeds token window; use prepared shared-window data"
                        )
                    truncated += 1
                    tokens = tokenizer(row["text"], truncation=True, max_length=max_tokens)
                self.items.append({**tokens, "labels": int(row["label"])})

        def __len__(self):
            return len(self.items)

        def __getitem__(self, index):
            return self.items[index]

    trainer = Trainer(
        model=model,
        args=TrainingArguments(
            output_dir=str(output),
            use_cpu=True,
            seed=seed,
            data_seed=seed,
            num_train_epochs=epochs,
            per_device_train_batch_size=batch_size,
            per_device_eval_batch_size=batch_size,
            eval_strategy="epoch",
            save_strategy="no",
            report_to=[],
            disable_tqdm=True,
        ),
        train_dataset=Dataset(train_rows, allow_training_truncation),
        eval_dataset=Dataset(validation_rows, False),
        data_collator=DataCollatorWithPadding(tokenizer),
        processing_class=tokenizer,
    )
    trainer.train()
    trainer.save_model(str(output))
    tokenizer.save_pretrained(output)
    metadata = {
        "schema": 1,
        "source_checkpoint": source_hashes,
        "training_sha256": sha256(train_path),
        "validation_sha256": sha256(validation_path),
        "train_samples": len(train_rows),
        "validation_samples": len(validation_rows),
        "seed": seed,
        "epochs": epochs,
        "batch_size": batch_size,
        "max_tokens": max_tokens,
        "truncated_training_samples": truncated,
        "input_contract": "raw-text-binary-classifier-v1",
        "attack_index": 1,
        "metrics": "validation loss only; independent test is a separate command",
    }
    write_json(output / "training.json", metadata)
    return metadata


def export_onnx(checkpoint: Path, output: Path) -> dict:
    import onnx
    from optimum.exporters.onnx import main_export

    if output.exists() and any(output.iterdir()):
        raise ValueError("export output directory must be empty")
    training = json.loads((checkpoint / "training.json").read_text())
    main_export(
        str(checkpoint.resolve()),
        output=output,
        task="text-classification",
        opset=17,
        local_files_only=True,
        trust_remote_code=False,
        do_validation=True,
    )
    graph = onnx.load(output / "model.onnx", load_external_data=False)
    onnx.checker.check_model(str(output / "model.onnx"))
    inputs = [node.name for node in graph.graph.input]
    outputs = [node.name for node in graph.graph.output]
    if set(inputs) != {"input_ids", "attention_mask"} or outputs != ["logits"]:
        raise ValueError(
            "export is incompatible with current Go backend: requires two inputs/logits"
        )
    manifest = {
        "schema": 1,
        "task": "prompt-injection",
        "id": "saifety-prompt-injection",
        "input_contract": "raw-text-binary-classifier-v1",
        "production_ready": False,
        "format": "onnx",
        "inputs": inputs,
        "outputs": outputs,
        "attack_index": training["attack_index"],
        "max_tokens": training["max_tokens"],
        "training": training,
        "files": checkpoint_hashes(output),
        "go_parity": "requires separate matching parity report",
    }
    write_json(output / "manifest.json", manifest)
    return manifest


def check_onnx(
    bundle: Path, data: Path, bridge: Path, library: Path, output: Path, *, tolerance: float = 1e-6
) -> dict:
    import numpy as np
    import onnxruntime as ort
    from transformers import AutoTokenizer

    if tolerance <= 0:
        raise ValueError("tolerance must be positive")
    manifest = json.loads((bundle / "manifest.json").read_text())
    for name, expected in manifest["files"].items():
        if sha256(bundle / name) != expected:
            raise ValueError("exported bundle checksum mismatch")
    texts = [r["text"] for r in samples(data)]
    tokenizer = AutoTokenizer.from_pretrained(
        bundle, local_files_only=True, trust_remote_code=False
    )
    session = ort.InferenceSession(str(bundle / "model.onnx"), providers=["CPUExecutionProvider"])
    go = Bridge(
        bridge,
        backend="onnx",
        model=bundle / "model.onnx",
        tokenizer=bundle / "tokenizer.json",
        library=library,
        attack_index=manifest["attack_index"],
    )
    counts = go.run(texts, "tokens")
    scores = []
    for text, count in zip(texts, counts, strict=True):
        encoded = tokenizer(text, return_tensors="np", truncation=False)
        length = encoded["input_ids"].shape[1]
        if length != count["tokens"]:
            raise ValueError("Python/Go tokenizer length mismatch")
        if length > min(512, manifest["max_tokens"]):
            raise ValueError("parity input exceeds exported token window")
        logits = session.run(
            ["logits"], {key: encoded[key] for key in ("input_ids", "attention_mask")}
        )[0]
        if logits.shape != (1, 2):
            raise ValueError("expected two binary-classification logits")
        probabilities = np.exp(logits[0] - np.max(logits[0]))
        scores.append(float(probabilities[manifest["attack_index"]] / probabilities.sum()))
    go_scores = go.scores(texts)
    delta = max(abs(a - b) for a, b in zip(scores, go_scores, strict=True))
    result = {
        "schema": 1,
        "samples": len(texts),
        "data_sha256": sha256(data),
        "model_sha256": sha256(bundle / "model.onnx"),
        "runtime_sha256": sha256(library),
        "manifest_sha256": sha256(bundle / "manifest.json"),
        "go_bridge": go.identity,
        "tolerance": tolerance,
        "max_absolute_delta": delta,
        "passed": delta <= tolerance,
    }
    write_json(output, result)
    if delta > tolerance:
        raise ValueError(f"ONNX Python/Go parity failed: {delta}")
    return result


def translate(
    model_path: Path,
    data: Path,
    output: Path,
    *,
    source_language: str,
    target_language: str = "eng_Latn",
    max_tokens: int = 512,
) -> dict:
    import torch
    from transformers import AutoModelForSeq2SeqLM, AutoTokenizer

    if output.exists():
        raise ValueError("translation output already exists")
    if max_tokens < 1:
        raise ValueError("translation token budget must be positive")
    rows = samples(data, evaluation=True)
    tokenizer = AutoTokenizer.from_pretrained(
        model_path, local_files_only=True, trust_remote_code=False, src_lang=source_language
    )
    target_id = tokenizer.convert_tokens_to_ids(target_language)
    if target_id is None or target_id == tokenizer.unk_token_id:
        raise ValueError("translation checkpoint does not support target language token")
    model = AutoModelForSeq2SeqLM.from_pretrained(
        model_path, local_files_only=True, trust_remote_code=False, use_safetensors=True
    )
    model.eval()
    torch.set_num_threads(1)
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_suffix(output.suffix + ".partial")
    with temporary.open("w") as target:
        for row in rows:
            encoded = tokenizer(row["text"], return_tensors="pt", truncation=False)
            if encoded["input_ids"].shape[1] > max_tokens:
                raise ValueError("translation input exceeds token window")
            with torch.inference_mode():
                generated = model.generate(
                    **encoded, forced_bos_token_id=target_id, max_new_tokens=max_tokens
                )
            if generated[0, -1].item() != model.config.eos_token_id:
                raise ValueError("translation did not finish; output would be truncated")
            translated = tokenizer.decode(generated[0], skip_special_tokens=True)
            if not translated.strip():
                raise ValueError("empty translation")
            target.write(
                json.dumps(
                    {
                        "id": row["id"],
                        "text_sha256": text_hash(row["text"]),
                        "translated_text": translated,
                    },
                    ensure_ascii=False,
                )
                + "\n"
            )
    temporary.replace(output)
    metadata = {
        "source_checkpoint": checkpoint_hashes(model_path),
        "data_sha256": sha256(data),
        "translations_sha256": sha256(output),
        "source_language": source_language,
        "target_language": target_language,
        "samples": len(rows),
        "limitations": "explicit NLLB-style source language; not automatic language detection",
    }
    write_json(Path(str(output) + ".meta.json"), metadata)
    return metadata
