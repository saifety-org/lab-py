"""Build and attest the Go tools owned by this checkout, without a second repository."""

import json
import os
import subprocess
from pathlib import Path, PurePosixPath

from .io import sha256


def root() -> Path:
    for candidate in (Path.cwd().resolve(), *Path.cwd().resolve().parents):
        if (candidate / "research/go.mod").is_file() and (candidate / "pyproject.toml").is_file():
            return candidate
    raise ValueError("run from a lab-py checkout containing research/go.mod")


def local_file(relative: str) -> Path:
    path = PurePosixPath(relative)
    if (
        path.is_absolute()
        or not path.parts
        or path.parts[0] != "research"
        or ".." in path.parts
        or "\\" in relative
        or ":" in relative
    ):
        raise ValueError("unsafe local research path")
    base = root()
    target = base.joinpath(*path.parts)
    if not target.resolve().is_relative_to(base / "research"):
        raise ValueError("local research path escapes checkout")
    return target


def identity() -> str:
    manifest = local_file("research/tools.lock.json")
    value = json.loads(manifest.read_text(encoding="utf-8"))
    if (
        value.get("schema") != "local-research-tools-v1"
        or value.get("module") != "github.com/saifety-org/lab-py/research"
    ):
        raise ValueError("invalid local research tools manifest")
    expected = {item["path"]: item["sha256"] for item in value["files"]}
    base = root()
    actual = {
        p.relative_to(base).as_posix()
        for p in (base / "research").rglob("*.go")
        if p.relative_to(base / "research").parts[0] not in {"bin", "artifacts", "legacy-sources"}
    }
    actual |= {"research/go.mod", "research/go.sum"}
    if set(expected) != actual:
        raise ValueError("research Go inputs changed; run scripts/update-research-lock.py")
    for path, digest in expected.items():
        if sha256(local_file(path)) != digest:
            raise ValueError(f"research tool source checksum mismatch: {path}")
    return "sha256:" + sha256(manifest)


def executable(path: Path) -> Path:
    if os.name == "nt" and path.suffix != ".exe" and not path.exists():
        return path.with_name(path.name + ".exe")
    return path


def build(command: str, destination: Path, *, onnx: bool = False) -> Path:
    if command not in {
        "comparison",
        "model-bridge",
        "context-corpus",
        "context-collect",
        "context-onnx-bridge",
    }:
        raise ValueError("unsupported research command")
    before = identity()
    destination = destination.resolve()
    destination.mkdir(parents=True, exist_ok=True)
    binary = destination / (command + (".exe" if os.name == "nt" else ""))
    args = ["go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false"]
    if onnx:
        args += ["-tags", "onnx"]
    args += ["-o", str(binary), "./cmd/" + command]
    subprocess.run(args, cwd=root() / "research", env=dict(os.environ, GOWORK="off"), check=True)
    if identity() != before:
        raise ValueError("research sources changed during build")
    return binary
