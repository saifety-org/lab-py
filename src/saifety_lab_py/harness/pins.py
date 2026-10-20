"""Immutable corpus/core pins and local binary attestation."""

import hashlib
import json
import os
import re
import subprocess
import urllib.request
from pathlib import Path

from .. import research
from ..io import sha256, write_json


def lock(path: Path) -> dict:
    value = json.loads(path.read_text(encoding="utf-8"))
    if (
        (value.get("schema"), value["lab"]["repository"])
        not in {
            ("agent-harness-sources-v1", "saifety-org/lab"),
            ("agent-harness-sources-v2", "saifety-org/lab-py"),
        }
        or not re.fullmatch(r"[0-9a-f]{40}", value["lab"]["commit"])
        or value["core"]["module"] != "github.com/saifety-org/sAIfety"
        or not re.fullmatch(r"v0\.0\.0-\d{14}-[0-9a-f]{12}", value["core"]["version"])
        or not re.fullmatch(r"[0-9a-f]{40}", value["core"]["commit"])
        or not value["core"]["version"].endswith("-" + value["core"]["commit"][:12])
    ):
        raise ValueError("invalid harness source pins")
    if {f["name"] for f in value["lab"]["files"]} != {"cases.json", "manifest.json"}:
        raise ValueError("unexpected harness source files")
    for item in value["lab"]["files"]:
        if not re.fullmatch(r"[0-9a-f]{64}", item["sha256"]):
            raise ValueError("invalid corpus checksum")
        if value["schema"] == "agent-harness-sources-v2":
            research.local_file(item["local_path"])
    return value


def prepare(path: Path, directory: Path) -> dict:
    value = lock(path)
    directory.mkdir(parents=True, exist_ok=True)
    for item in value["lab"]["files"]:
        target = directory / item["name"]
        if target.is_symlink():
            raise ValueError("scenario cache cannot be a symlink")
        if target.exists():
            raw = target.read_bytes()
        elif value["schema"] == "agent-harness-sources-v2":
            raw = research.local_file(item["local_path"]).read_bytes()
        else:
            url = (
                f"https://raw.githubusercontent.com/{value['lab']['repository']}/"
                f"{value['lab']['commit']}/datasets/agent-harness/v1/{item['name']}"
            )
            with urllib.request.urlopen(url, timeout=30) as response:
                raw = response.read((1 << 20) + 1)
        if len(raw) > 1 << 20 or hashlib.sha256(raw).hexdigest() != item["sha256"]:
            raise ValueError("scenario cache/download checksum mismatch")
        target.write_bytes(raw)
    from .suite import load

    load(directory)
    return value


def binary_name() -> str:
    return "saifety.exe" if os.name == "nt" else "saifety"


def bootstrap(path: Path, directory: Path) -> dict:
    value = lock(path)
    directory = directory.resolve()
    directory.mkdir(parents=True, exist_ok=True)
    subprocess.run(
        ["go", "install", value["core"]["module"] + "/cmd/saifety@" + value["core"]["version"]],
        check=True,
        env=dict(os.environ, GOWORK="off", GOBIN=str(directory)),
    )
    binary = directory / binary_name()
    result = {
        "core": value["core"],
        "sha256": sha256(binary),
        "version": subprocess.check_output([str(binary), "version"], text=True, timeout=10),
    }
    write_json(directory / "harness-tools.json", result)
    return result


def verify(path: Path, binary: Path, directory: Path) -> dict:
    value = lock(path)
    tools = json.loads((binary.parent / "harness-tools.json").read_text(encoding="utf-8"))
    if tools["core"] != value["core"] or tools["sha256"] != sha256(binary):
        raise ValueError("proxy identity mismatch; run bootstrap-agent-harness")
    for item in value["lab"]["files"]:
        if sha256(directory / item["name"]) != item["sha256"]:
            raise ValueError("suite differs from source pins")
    return tools
