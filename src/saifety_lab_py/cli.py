"""Command-line entry points. Heavy ML dependencies are optional."""

import argparse
import json
import subprocess
from pathlib import Path

from . import sources
from .bridge import Bridge
from .evaluate import compare, compare_onnx
from .io import write_json
from .native import train


def parser() -> argparse.ArgumentParser:
    root = argparse.ArgumentParser(description="sAIfety Python research tools")
    sub = root.add_subparsers(dest="command", required=True)
    for command in ("bootstrap", "sync", "prepare"):
        p = sub.add_parser(command)
        p.add_argument("--lock", type=Path, default=Path("sources.lock.json"))
        if command == "bootstrap":
            p.add_argument("--out", type=Path, default=Path("bin"))
            p.add_argument("--onnx", action="store_true")
        else:
            p.add_argument("--source", type=Path, default=Path("data/source"))
            if command == "prepare":
                p.add_argument("--out", type=Path, default=Path("data/comparison"))
                p.add_argument("--comparison", type=Path, default=Path("bin/comparison"))
    p = sub.add_parser("bootstrap-context")
    p.add_argument("--lock", type=Path, default=Path("context.sources.lock.json"))
    p.add_argument("--out", type=Path, default=Path("bin"))
    p.add_argument("--onnx", action="store_true")
    p = sub.add_parser("prepare-context")
    p.add_argument("--lock", type=Path, default=Path("context.sources.lock.json"))
    p.add_argument("--source", type=Path, default=Path("data/context-source"))
    p.add_argument("--out", type=Path, default=Path("data/contextual"))
    p.add_argument("--tool", type=Path, default=Path("bin/context-corpus"))
    p = sub.add_parser("train-context")
    p.add_argument("--data", type=Path, default=Path("data/contextual"))
    p.add_argument("--out", type=Path, default=Path("artifacts/contextual"))
    p.add_argument("--bridge", type=Path, default=Path("bin/model-bridge"))
    p.add_argument("--epochs", type=int, default=40)
    p.add_argument("--seed", type=int, default=1)
    p = sub.add_parser("compare-context")
    p.add_argument("--data", type=Path, default=Path("data/contextual"))
    p.add_argument("--models", type=Path, default=Path("artifacts/contextual"))
    p.add_argument("--out", type=Path, default=Path("artifacts/contextual/comparison"))
    p.add_argument("--bridge", type=Path, default=Path("bin/model-bridge"))
    p.add_argument("--max-fpr", type=float, default=0.01)
    p.add_argument("--deberta", action="store_true")
    p = sub.add_parser("fetch-encoder")
    p.add_argument("--lock", type=Path, default=Path("encoder.model.lock.json"))
    p.add_argument("--out", type=Path, default=Path("artifacts/e5-base"))
    for command in ("train-encoder", "compare-encoder"):
        p = sub.add_parser(command)
        p.add_argument("--data", type=Path, default=Path("data/encoder"))
        p.add_argument("--lock", type=Path, default=Path("encoder-v4.sources.lock.json"))
        p.add_argument("--source", type=Path, default=Path("data/encoder-source"))
        p.add_argument("--tool", type=Path, default=Path("bin/encoder/context-corpus"))
        if command == "train-encoder":
            p.add_argument("--base", type=Path, required=True)
            p.add_argument("--model-lock", type=Path, default=Path("encoder.model.lock.json"))
            p.add_argument("--protocol", type=Path, default=Path("encoder-v4.protocol.json"))
            p.add_argument("--out", type=Path, default=Path("artifacts/encoder-v4"))
        else:
            p.add_argument("--models", type=Path, default=Path("artifacts/encoder-v4"))
            p.add_argument("--out", type=Path, default=Path("artifacts/encoder-v4-comparison"))
            p.add_argument("--bridge", type=Path, default=Path("bin/encoder/model-bridge"))
    p = sub.add_parser("train-native")
    p.add_argument("--train", type=Path, default=Path("data/comparison/train.jsonl"))
    p.add_argument("--out", type=Path, default=Path("artifacts/native/weights.json"))
    p.add_argument("--bridge", type=Path, default=Path("bin/model-bridge"))
    p.add_argument("--epochs", type=int, default=40)
    p.add_argument("--seed", type=int, default=1)
    p = sub.add_parser("compare-native")
    p.add_argument("--data", type=Path, default=Path("data/comparison/evaluation.jsonl"))
    p.add_argument("--weights", type=Path, default=Path("artifacts/native/weights.json"))
    p.add_argument("--bridge", type=Path, default=Path("bin/model-bridge"))
    p.add_argument("--out", type=Path, default=Path("artifacts/native/comparison"))
    p.add_argument("--max-fpr", type=float, default=0.01)
    p.add_argument("--deberta", action="store_true", help="requires ONNX bridge and cached DeBERTa")
    p.add_argument("--translations", type=Path)
    p = sub.add_parser("misses")
    p.add_argument(
        "--cases", type=Path, default=Path("data/source/testdata/missed-injections/cases.json")
    )
    p.add_argument("--bridge", type=Path, default=Path("bin/model-bridge"))
    p.add_argument("--out", type=Path, default=Path("artifacts/missed-injections.json"))
    p = sub.add_parser("train-transformer")
    p.add_argument("--model", type=Path, required=True, help="local safetensors checkpoint")
    p.add_argument("--train", type=Path, required=True)
    p.add_argument("--validation", type=Path, required=True)
    p.add_argument("--out", type=Path, required=True)
    p.add_argument("--epochs", type=float, default=1)
    p.add_argument("--seed", type=int, default=1)
    p.add_argument("--batch-size", type=int, default=8)
    p.add_argument("--max-tokens", type=int, default=512)
    p.add_argument("--allow-training-truncation", action="store_true")
    p = sub.add_parser("bootstrap-encoder-onnx")
    p.add_argument("--lock", type=Path, default=Path("encoder-onnx.tools.lock.json"))
    p.add_argument("--out", type=Path, default=Path("bin"))
    p = sub.add_parser("encoder-parity-inputs")
    p.add_argument("--evaluation", type=Path, required=True)
    p.add_argument("--bundle", type=Path, required=True)
    p.add_argument("--out", type=Path, required=True)
    p = sub.add_parser("export-encoder-onnx")
    p.add_argument("--candidate", type=Path, required=True)
    p.add_argument("--thresholds", type=Path, required=True)
    p.add_argument("--model-lock", type=Path, default=Path("encoder.model.lock.json"))
    p.add_argument("--out", type=Path, required=True)
    p = sub.add_parser("check-encoder-onnx")
    p.add_argument("--candidate", type=Path, required=True)
    p.add_argument("--bundle", type=Path, required=True)
    p.add_argument("--data", type=Path, required=True)
    p.add_argument("--bridge", type=Path, required=True)
    p.add_argument("--library", type=Path, required=True)
    p.add_argument("--out", type=Path, required=True)
    p = sub.add_parser("export-onnx")
    p.add_argument("--checkpoint", type=Path, required=True)
    p.add_argument("--out", type=Path, required=True)
    p = sub.add_parser("check-onnx")
    p.add_argument("--bundle", type=Path, required=True)
    p.add_argument("--data", type=Path, required=True)
    p.add_argument("--bridge", type=Path, default=Path("bin/model-bridge"))
    p.add_argument("--library", type=Path, required=True)
    p.add_argument("--out", type=Path, default=Path("artifacts/onnx-parity.json"))
    p.add_argument("--tolerance", type=float, default=1e-6)
    p = sub.add_parser("translate")
    p.add_argument("--model", type=Path, required=True)
    p.add_argument("--data", type=Path, required=True)
    p.add_argument("--out", type=Path, required=True)
    p.add_argument("--source-language", required=True)
    p.add_argument("--target-language", default="eng_Latn")
    p = sub.add_parser("compare-onnx")
    p.add_argument("--data", type=Path, default=Path("data/comparison/evaluation.jsonl"))
    p.add_argument("--bundle", type=Path, required=True)
    p.add_argument("--parity", type=Path, required=True)
    p.add_argument("--library", type=Path, required=True)
    p.add_argument("--bridge", type=Path, default=Path("bin/model-bridge"))
    p.add_argument("--out", type=Path, default=Path("artifacts/transformer/comparison"))
    p.add_argument("--max-fpr", type=float, default=0.01)
    p.add_argument("--deberta", action="store_true")
    from .harness.cli import add_parser

    add_parser(sub)
    return root


def main(argv: list[str] | None = None) -> None:
    root = parser()
    args = root.parse_args(argv)
    result = None
    try:
        match args.command:
            case (
                "bootstrap-agent-harness"
                | "prepare-agent-harness"
                | "agent-harness-plan"
                | "run-agent-harness"
            ):
                from .harness.cli import dispatch

                result = dispatch(args)
            case "bootstrap":
                sources.bootstrap(args.lock, args.out, onnx=args.onnx)
            case "sync":
                result = sources.sync(args.lock, args.source)
            case "prepare":
                sources.prepare(args.lock, args.source, args.out, args.comparison)
            case "bootstrap-context":
                sources.bootstrap(args.lock, args.out, onnx=args.onnx, context=True)
            case "prepare-context":
                sources.prepare_context(args.lock, args.source, args.out, args.tool)
            case "train-context":
                from .contextual import train_context

                result = train_context(
                    args.data, args.out, args.bridge, epochs=args.epochs, seed=args.seed
                )
            case "compare-context":
                from .contextual import compare_context

                result = compare_context(
                    args.data,
                    args.models,
                    args.bridge,
                    args.out,
                    max_fpr=args.max_fpr,
                    deberta=Bridge(args.bridge, backend="onnx") if args.deberta else None,
                )
            case "fetch-encoder":
                from .encoder import fetch_encoder

                fetch_encoder(args.lock, args.out)
            case "train-encoder":
                from .encoder import train_encoder

                result = train_encoder(
                    args.data,
                    args.base,
                    args.out,
                    args.protocol,
                    args.model_lock,
                    args.lock,
                    args.source,
                    args.tool,
                )
            case "compare-encoder":
                from .encoder import compare_encoder

                result = compare_encoder(
                    args.data, args.models, args.out, args.lock, args.source, args.tool, args.bridge
                )
            case "train-native":
                result = train(
                    args.train, args.out, Bridge(args.bridge), epochs=args.epochs, seed=args.seed
                )
            case "compare-native":
                result = compare(
                    args.data,
                    args.weights,
                    args.bridge,
                    args.out,
                    max_fpr=args.max_fpr,
                    translations=args.translations,
                    deberta=Bridge(args.bridge, backend="onnx") if args.deberta else None,
                )
            case "compare-onnx":
                result = compare_onnx(
                    args.data,
                    args.bundle,
                    args.parity,
                    args.bridge,
                    args.library,
                    args.out,
                    max_fpr=args.max_fpr,
                    deberta=Bridge(args.bridge, backend="onnx") if args.deberta else None,
                )
            case "misses":
                cases = json.loads(args.cases.read_text())
                if not cases or any(not isinstance(c.get("payload"), str) for c in cases):
                    raise ValueError("invalid contextual fixture")
                bridge = Bridge(args.bridge)
                observations = bridge.run([case["payload"] for case in cases], "scan")
                result = {
                    "corpus": "selected known misses; not an independent benchmark",
                    "bridge": bridge.identity,
                    "cases": [
                        {**case, "verdict": observed["verdict"]}
                        for case, observed in zip(cases, observations, strict=True)
                    ],
                }
                write_json(args.out, result)
            case "train-transformer":
                from .transformer import train_transformer

                result = train_transformer(
                    args.model,
                    args.train,
                    args.validation,
                    args.out,
                    epochs=args.epochs,
                    seed=args.seed,
                    batch_size=args.batch_size,
                    max_tokens=args.max_tokens,
                    allow_training_truncation=args.allow_training_truncation,
                )
            case "bootstrap-encoder-onnx":
                result = sources.bootstrap_encoder_onnx(args.lock, args.out)
            case "encoder-parity-inputs":
                from .encoder_export import parity_inputs

                result = parity_inputs(args.evaluation, args.bundle, args.out)
            case "export-encoder-onnx":
                from .encoder_export import export_encoder

                result = export_encoder(args.candidate, args.thresholds, args.model_lock, args.out)
            case "check-encoder-onnx":
                from .encoder_export import parity

                result = parity(
                    args.bundle, args.candidate, args.data, args.bridge, args.library, args.out
                )
                result = {k: v for k, v in result.items() if k != "predictions"}
            case "export-onnx":
                from .transformer import export_onnx

                result = export_onnx(args.checkpoint, args.out)
            case "check-onnx":
                from .transformer import check_onnx

                result = check_onnx(
                    args.bundle,
                    args.data,
                    args.bridge,
                    args.library,
                    args.out,
                    tolerance=args.tolerance,
                )
            case "translate":
                from .transformer import translate

                result = translate(
                    args.model,
                    args.data,
                    args.out,
                    source_language=args.source_language,
                    target_language=args.target_language,
                )
    except ImportError as error:
        root.exit(1, f"ML dependencies unavailable: {error}. Run uv sync --locked --extra ml.\n")
    except (ValueError, RuntimeError, OSError, subprocess.CalledProcessError) as error:
        root.exit(1, f"error: {error}\n")
    if result is not None:
        print(json.dumps(result, ensure_ascii=False, indent=2, allow_nan=False))
