"""Typed client for the pinned Go implementation. No Python feature extractor."""

import json
import subprocess
from pathlib import Path

from .io import finite, sha256, text_hash


class Bridge:
    def __init__(
        self,
        executable: Path,
        *,
        backend: str = "native",
        weights: Path | None = None,
        model: Path | None = None,
        tokenizer: Path | None = None,
        library: Path | None = None,
        attack_index: int = 1,
    ):
        self.args = [str(executable.resolve()), "-backend", backend]
        for name, value in (
            ("weights", weights),
            ("model", model),
            ("tokenizer", tokenizer),
            ("library", library),
        ):
            if value is not None:
                self.args += [f"-{name}", str(value.resolve())]
        self.args += ["-attack-index", str(attack_index)]
        self.identity: dict = {}
        self.binary_hash = sha256(Path(self.args[0]))
        self.batch_size = 512 if backend == "onnx" else 64

    def run(self, texts: list[str], mode: str) -> list[dict]:
        result = []
        for start in range(0, len(texts), self.batch_size):
            batch = texts[start : start + self.batch_size]
            content = "".join(
                json.dumps({"text": text}, ensure_ascii=False) + "\n" for text in batch
            )
            run = subprocess.run(
                self.args + ["-mode", mode],
                input=content,
                text=True,
                capture_output=True,
                timeout=180,
            )
            if run.returncode:
                raise RuntimeError(f"Go bridge failed: {run.stderr.strip()}")
            rows = [json.loads(line) for line in run.stdout.splitlines()]
            if len(rows) != len(batch):
                raise ValueError("bridge returned an incomplete batch")
            for text, row in zip(batch, rows, strict=True):
                if row.get("text_sha256") != text_hash(text):
                    raise ValueError("bridge response does not match input")
                identity = {
                    key: row[key]
                    for key in ("model_module", "feature_schema", "dim", "weights_sha256")
                }
                identity["bridge_sha256"] = self.binary_hash
                if self.identity and self.identity != identity:
                    raise ValueError("bridge model changed during the run")
                self.identity = identity
            result.extend(rows)
        return result

    def features(self, texts: list[str]) -> list[list[tuple[int, float]]]:
        out = []
        for row in self.run(texts, "features"):
            values = sorted(
                (int(key), finite(float(value))) for key, value in row.get("features", {}).items()
            )
            if any(index < 0 or index >= row["dim"] for index, _ in values):
                raise ValueError("feature outside model dimensions")
            out.append(values)
        return out

    def scores(self, texts: list[str]) -> list[float]:
        values = [finite(float(row["score"])) for row in self.run(texts, "score")]
        if any(not 0 <= value <= 1 for value in values):
            raise ValueError("invalid score")
        return values
