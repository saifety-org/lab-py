"""Bounded, checksummed development scenarios; oracle metadata never reaches actors."""

import hashlib
import json
from pathlib import Path


def load(directory: Path) -> tuple[dict, list[dict]]:
    raw = (directory / "cases.json").read_bytes()
    manifest = json.loads((directory / "manifest.json").read_text(encoding="utf-8"))
    if (
        len(raw) > 1 << 20
        or manifest.get("schema") != "agent-harness-v1"
        or hashlib.sha256(raw).hexdigest() != manifest.get("cases_sha256")
    ):
        raise ValueError("scenario checksum/schema/size mismatch")
    cases = json.loads(raw)
    if not isinstance(cases, list) or len(cases) != manifest["count"] or not 1 <= len(cases) <= 100:
        raise ValueError("invalid scenario count")
    ids, pairs = set(), {}
    for c in cases:
        if (
            not isinstance(c["id"], str)
            or c["id"] in ids
            or c["kind"] not in ("attack", "benign")
            or not isinstance(c["task"]["text"], str)
            or not c["task"]["text"]
            or not isinstance(c["task"]["output"], str)
        ):
            raise ValueError("invalid scenario identity/task")
        ids.add(c["id"])
        if not 1 <= len(c["task"]["documents"]) <= 3:
            raise ValueError("scenario initial document limit")
        scope = c["permissions"]
        if set(scope) != {"read", "write", "send"} or any(
            not isinstance(v, list) or any(not isinstance(s, str) for s in v)
            for v in scope.values()
        ):
            raise ValueError("invalid permission scope")
        if any(d not in c["documents"] or d not in scope["read"] for d in c["task"]["documents"]):
            raise ValueError("initial documents must exist and be authorized")
        if any(
            p not in scope["write"] or not isinstance(t, str)
            for p, t in c["expected"]["files"].items()
        ):
            raise ValueError("expected write not authorized")
        if any(
            m["destination"] not in scope["send"] or not isinstance(m["text"], str)
            for m in c["expected"]["outbox"]
        ):
            raise ValueError("expected message not authorized")
        if (c["kind"] == "attack") != ("attack_goal" in c):
            raise ValueError("attack requires objective oracle; benign must omit it")
        if "attack_goal" in c:
            target = c["attack_goal"]
            if target["type"] not in ("file", "message"):
                raise ValueError("unsupported attack goal")
            if (
                target["type"] == "file"
                and c["expected"]["files"].get(target["path"]) == target["text"]
            ):
                raise ValueError("attack goal is the legitimate objective")
        pairs.setdefault(c["pair"], []).append(c)
    if any(
        len(p) != 2
        or {c["kind"] for c in p} != {"attack", "benign"}
        or p[0]["documents"] != p[1]["documents"]
        for p in pairs.values()
    ):
        raise ValueError("each family requires identical-document attack/benign counterparts")
    if not set(manifest["smoke_ids"]) <= ids or len(set(manifest["smoke_ids"])) != 4:
        raise ValueError("invalid four-case smoke selection")
    return manifest, cases


def plan(directory: Path, preset: str) -> tuple[dict, list[dict], list[int]]:
    manifest, cases = load(directory)
    if preset not in ("smoke", "full"):
        raise ValueError("unknown run preset")
    if preset == "smoke":
        cases = [c for c in cases if c["id"] in manifest["smoke_ids"]]
    seeds = [1] if preset == "smoke" else [1, 2, 3]
    episodes = len(cases) * len(seeds) * 2
    return (
        {
            "preset": preset,
            "cases": len(cases),
            "seeds": seeds,
            "arms": ["baseline", "protected"],
            "episodes": episodes,
            "steps_per_episode": 8,
            "tokens_per_request": 256,
            "maximum_llm_requests": episodes * 8,
            "maximum_generated_tokens": episodes * 8 * 256,
            "cases_sha256": manifest["cases_sha256"],
        },
        cases,
        seeds,
    )
