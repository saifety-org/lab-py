"""Strict artifact I/O and checksums."""

import hashlib
import json
import math
from pathlib import Path


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def text_hash(text: str) -> str:
    return hashlib.sha256(text.encode()).hexdigest()


def write_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2, allow_nan=False) + "\n")


def read_jsonl(path: Path) -> list[dict]:
    rows = []
    with path.open() as source:
        for number, line in enumerate(source, 1):
            if not line.strip():
                raise ValueError(f"{path}:{number}: empty row")
            row = json.loads(line)
            if not isinstance(row, dict):
                raise ValueError(f"{path}:{number}: expected object")
            rows.append(row)
    if not rows:
        raise ValueError(f"{path}: empty corpus")
    return rows


def samples(path: Path, *, evaluation: bool = False) -> list[dict]:
    rows = read_jsonl(path)
    seen = set()
    groups: dict[str, str] = {}
    for row in rows:
        if not isinstance(row.get("text"), str) or not row["text"].strip():
            raise ValueError("sample must have nonempty text")
        if type(row.get("label")) not in (int, float) or row["label"] not in (0, 1):
            raise ValueError("label must be 0 or 1")
        if evaluation:
            if not all(
                isinstance(row.get(k), str) and row[k]
                for k in ("id", "group", "split", "language", "category")
            ):
                raise ValueError("evaluation sample is missing context")
            if row["id"] in seen or row["split"] not in ("validation", "test"):
                raise ValueError("duplicate id or invalid evaluation split")
            seen.add(row["id"])
            old = groups.setdefault(row["group"], row["split"])
            if old != row["split"]:
                raise ValueError("a scenario group crosses validation and test")
    return rows


def finite(value: float) -> float:
    if not math.isfinite(value):
        raise ValueError("non-finite value")
    return value
