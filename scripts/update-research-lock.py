"""Refresh the reviewable Go source identity after an intentional source change."""

import hashlib
import json
from pathlib import Path

root = Path(__file__).resolve().parents[1]
paths = [root / "research/go.mod", root / "research/go.sum"]
paths.extend(
    p
    for p in (root / "research").rglob("*.go")
    if p.relative_to(root / "research").parts[0] not in {"bin", "artifacts", "legacy-sources"}
)
value = {
    "schema": "local-research-tools-v1",
    "module": "github.com/saifety-org/lab-py/research",
    "files": [
        {
            "path": p.relative_to(root).as_posix(),
            "sha256": hashlib.sha256(p.read_bytes()).hexdigest(),
        }
        for p in sorted(paths)
    ],
}
target = root / "research/tools.lock.json"
target.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8", newline="\n")
print(hashlib.sha256(target.read_bytes()).hexdigest())
