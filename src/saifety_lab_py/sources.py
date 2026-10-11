"""Datasets have one owner: saifety-org/lab. Download only locked bytes."""

import hashlib
import json
import os
import re
import subprocess
import urllib.request
from pathlib import Path, PurePosixPath

from .io import sha256, write_json


def lock(path: Path) -> dict:
    value = json.loads(path.read_text())
    if value.get("schema") != 1 or value.get("repository") != "saifety-org/lab":
        raise ValueError("unsupported source lock")
    if not re.fullmatch(r"[0-9a-f]{40}", value.get("commit", "")):
        raise ValueError("source commit must be a full SHA")
    for key in ("source_corpus", "history"):
        if key in value:
            relative = PurePosixPath(value[key])
            if relative.is_absolute() or ".." in relative.parts:
                raise ValueError("unsafe contextual source path")
    balance = value.get("task_balance_contract")
    if balance not in (None, "context-task-balance-v1") or (
        value.get("source_corpus") == "datasets/contextual/v4"
        and balance != "context-task-balance-v1"
    ):
        raise ValueError("v4 source lock requires the task-balance contract")
    if balance and value.get("input_contract") != "context-json-v2":
        raise ValueError("task balance requires context-json-v2")
    for item in value["files"]:
        p = PurePosixPath(item["path"])
        if p.is_absolute() or ".." in p.parts or not re.fullmatch(r"[0-9a-f]{64}", item["sha256"]):
            raise ValueError("unsafe source path or invalid checksum")
    if not re.fullmatch(r"v0\.0\.0-\d{14}-[0-9a-f]{12}", value["tools_version"]):
        raise ValueError("Go tools must be pinned to an immutable pseudo-version")
    return value


def sync(path: Path, destination: Path) -> dict:
    pinned = lock(path)
    for item in pinned["files"]:
        target = destination / item["path"]
        if target.is_symlink():
            raise ValueError("source cache file cannot be a symlink")
        if target.exists():
            if sha256(target) != item["sha256"]:
                raise ValueError(f"cache checksum mismatch: {target}")
            continue
        url = f"https://raw.githubusercontent.com/{pinned['repository']}/{pinned['commit']}/{item['path']}"
        with urllib.request.urlopen(url, timeout=60) as response:
            content = response.read((32 << 20) + 1)
        if len(content) > 32 << 20 or hashlib.sha256(content).hexdigest() != item["sha256"]:
            raise ValueError(f"source checksum/size mismatch: {item['path']}")
        target.parent.mkdir(parents=True, exist_ok=True)
        temporary = target.with_suffix(target.suffix + ".partial")
        temporary.write_bytes(content)
        temporary.replace(target)
    write_json(destination / "source.json", pinned)
    return pinned


def bootstrap(path: Path, destination: Path, *, onnx: bool = False, context: bool = False) -> None:
    pinned = lock(path)
    destination = destination.resolve()
    destination.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ, GOWORK="off", GOBIN=str(destination))
    commands = ["context-corpus" if context else "comparison", "model-bridge"]
    for command in commands:
        args = ["go", "install"]
        if onnx and command == "model-bridge":
            args += ["-tags", "onnx"]
        args += [f"github.com/saifety-org/lab/cmd/{command}@{pinned['tools_version']}"]
        subprocess.run(args, env=env, check=True)
    write_json(
        destination / "tools.json",
        {
            "lab_version": pinned["tools_version"],
            "onnx": onnx,
            "binaries": {name: sha256(destination / name) for name in commands},
        },
    )


def prepare(path: Path, source: Path, output: Path, comparison: Path) -> None:
    pinned = sync(path, source)
    output = output.resolve()
    subprocess.run(
        [str(comparison.resolve()), "prepare", "-dir", str(output)], cwd=source, check=True
    )
    write_json(output / "source.json", pinned)


def prepare_context(path: Path, source: Path, output: Path, tool: Path) -> None:
    pinned = sync(path, source)
    tools = json.loads((tool.parent / "tools.json").read_text())
    if tools.get("lab_version") != pinned["tools_version"] or tools.get("binaries", {}).get(
        tool.name
    ) != sha256(tool):
        raise ValueError("context tool differs from source lock; run bootstrap-context")
    source_corpus = source / pinned.get("source_corpus", "datasets/contextual/v1")
    extra = []
    if pinned.get("input_contract") == "context-json-v2":
        extra = [
            "-reviews",
            str((source_corpus / "review.json").resolve()),
            "-require-reviewed",
            "-freeze",
            str((source_corpus / "holdout.json").resolve()),
            "-history",
            str((source / pinned["history"]).resolve()),
            "-catalog",
            str((source_corpus / "scenarios.json").resolve()),
        ]
    if pinned.get("task_balance_contract"):
        extra.append("-require-task-balanced")
    subprocess.run(
        [
            str(tool.resolve()),
            "-source",
            str(source_corpus.resolve()),
            "-out",
            str(output.resolve()),
            *extra,
        ],
        check=True,
        capture_output=True,
        text=True,
    )
    if sha256(output / "manifest.json") != sha256(source_corpus / "manifest.json"):
        raise ValueError("prepared corpus differs from pinned manifest")
    write_json(output / "source.json", pinned)


def bootstrap_encoder_onnx(path: Path, destination: Path) -> dict:
    """Install the dedicated contextual bridge without changing corpus pins."""
    value = json.loads(path.read_text())
    if (
        value.get("schema") != 1
        or value.get("repository") != "saifety-org/lab"
        or not re.fullmatch(r"[0-9a-f]{40}", value.get("commit", ""))
        or not re.fullmatch(r"v0\.0\.0-\d{14}-[0-9a-f]{12}", value.get("tools_version", ""))
        or not value["tools_version"].endswith("-" + value["commit"][:12])
    ):
        raise ValueError("unsupported contextual ONNX tools lock")
    destination = destination.resolve()
    destination.mkdir(parents=True, exist_ok=True)
    subprocess.run(
        [
            "go",
            "install",
            "-tags",
            "onnx",
            "github.com/saifety-org/lab/cmd/context-onnx-bridge@" + value["tools_version"],
        ],
        env=dict(os.environ, GOWORK="off", GOBIN=str(destination)),
        check=True,
    )
    result = {
        "lock_sha256": sha256(path),
        "lab_version": value["tools_version"],
        "bridge_sha256": sha256(destination / "context-onnx-bridge"),
    }
    write_json(destination / "context-onnx-tools.json", result)
    return result
