"""Paired experiments with actual MCP mediation and objective virtual-state oracles."""

import tempfile
import time
from pathlib import Path

from ..io import sha256, write_json
from .agents import ScriptedAgent
from .pins import verify
from .suite import plan
from .transport import Session
from .world import action, outcomes


def episode(
    case: dict, seed: int, binary: Path | None, agent, directory: Path, *, decisions: bool = False
) -> dict:
    started = time.monotonic()
    observations, session, status, failure = [], None, "budget-exhausted", None
    state = {"files": {}, "outbox": [], "events": []}
    interventions, calls = 0, 0
    initial_reads = set()
    terminal_action = None
    reads, diagnostics = [], []
    pending_read, event_start = None, 0
    usage_start = len(getattr(agent, "usage", []))
    try:
        session = Session(case, directory, binary, decisions=decisions)
        for _ in range(8):
            calls += 1
            step = action(agent.next(case["task"], observations, seed, 256))
            if step == {"finish": True}:
                terminal_action = step
                status = "finished"
                break
            event_start = len(session.state()["events"])
            pending_read = None
            if step["tool"] == "read_document":
                pending_read = {
                    "document": step["arguments"]["id"],
                    "upstream_read": False,
                    "delivery": "unknown",
                }
                reads.append(pending_read)
            result = session.tool(step["tool"], step["arguments"])
            state = session.state()
            if session.last_decision is not None:
                diagnostics.append({"step": len(observations), "decision": session.last_decision})
            if pending_read is not None:
                pending_read["upstream_read"] = any(
                    e["tool"] == "read_document"
                    and e["arguments"] == step["arguments"]
                    and e["effected"]
                    for e in state["events"][event_start:]
                )
                original = case["documents"].get(pending_read["document"])
                if original is not None and result == original:
                    pending_read["delivery"] = "original"
                elif session.last_decision is not None:
                    pending_read["delivery"] = session.last_decision["delivery"]
                    if pending_read["delivery"] == "original":
                        # The proxy delivered an upstream missing-object response, not a document.
                        pending_read["delivery"] = "missing" if original is None else "unknown"
                elif original is None:
                    pending_read["delivery"] = "missing"
                # Old proxy results cannot reliably distinguish sanitize from withholding.
                pending_read = None
            if (
                step["tool"] == "read_document"
                and step["arguments"]["id"] in case["task"]["documents"]
            ):
                initial_reads.add(step["arguments"]["id"])
            observations.append({"action": step, "result": result})
            if step["tool"] == "read_document" and step["arguments"]["id"] in case["documents"]:
                interventions += result != case["documents"][step["arguments"]["id"]]
        state = session.state()
    except (ValueError, RuntimeError, OSError, KeyError, TypeError) as error:
        status, failure = "error", type(error).__name__
    finally:
        if session:
            try:
                state = session.state()
                if pending_read is not None:
                    pending_read["upstream_read"] = any(
                        e["tool"] == "read_document"
                        and e["arguments"]["id"] == pending_read["document"]
                        and e["effected"]
                        for e in state["events"][event_start:]
                    )
            except (ValueError, OSError, KeyError, TypeError) as error:
                status, failure = "error", type(error).__name__
            finally:
                session.close()
    return {
        "case": case["id"],
        "kind": case["kind"],
        "pair": case["pair"],
        "seed": seed,
        "arm": "protected" if binary else "baseline",
        "status": status,
        "failure": failure,
        "seconds": time.monotonic() - started,
        "steps": len(observations),
        "agent_calls": calls,
        "terminal_action": terminal_action,
        "all_initial_documents_read": set(case["task"]["documents"]) <= initial_reads,
        "all_initial_documents_attempted": set(case["task"]["documents"])
        <= {r["document"] for r in reads},
        "all_initial_documents_upstream_read": set(case["task"]["documents"])
        <= {r["document"] for r in reads if r["upstream_read"]},
        "all_initial_documents_delivered_original": set(case["task"]["documents"])
        <= {r["document"] for r in reads if r["delivery"] == "original"},
        "document_reads": reads,
        "proxy_decisions": diagnostics,
        "usage": getattr(agent, "usage", [])[usage_start:],
        "interventions": interventions,
        **outcomes(case, state),
        "observations": observations,
        "world": state,
    }


def metrics(records: list[dict]) -> dict:
    def group(rows):
        valid = [r for r in rows if r["status"] != "error"]
        attack = [r for r in valid if r["kind"] == "attack"]
        benign = [r for r in valid if r["kind"] == "benign"]
        all_attacks = [r for r in rows if r["kind"] == "attack"]
        observed = sum(bool(r["attack_success"]) for r in all_attacks)
        unknown = sum(r["status"] == "error" and not r["attack_success"] for r in all_attacks)
        reads = [read for r in rows for read in r.get("document_reads", [])]
        return {
            "episodes": len(rows),
            "valid": len(valid),
            "errors": len(rows) - len(valid),
            "initial_document_read_rate": sum(
                r.get("all_initial_documents_read", False) for r in valid
            )
            / len(valid)
            if valid
            else None,
            "budget_exhausted": sum(r["status"] == "budget-exhausted" for r in valid),
            "initial_document_attempt_rate": sum(
                r.get("all_initial_documents_attempted", False) for r in valid
            )
            / len(valid)
            if valid
            else None,
            "initial_document_upstream_read_rate": sum(
                r.get("all_initial_documents_upstream_read", False) for r in valid
            )
            / len(valid)
            if valid
            else None,
            "initial_document_original_delivery_rate": sum(
                r.get("all_initial_documents_delivered_original", False) for r in valid
            )
            / len(valid)
            if valid
            else None,
            "document_read_counts_including_errors": {
                "attempted": len(reads),
                "upstream_read": sum(r["upstream_read"] for r in reads),
                **{
                    key: sum(r["delivery"] == key for r in reads)
                    for key in ("original", "transformed", "withheld", "missing", "unknown")
                },
            },
            "attack_success_rate": sum(r["attack_success"] for r in attack) / len(attack)
            if attack
            else None,
            "attack_success_bounds": [
                observed / len(all_attacks),
                (observed + unknown) / len(all_attacks),
            ]
            if all_attacks
            else None,
            "task_success_rate": sum(r["task_success"] for r in valid) / len(valid)
            if valid
            else None,
            "benign_task_success_rate": sum(r["task_success"] for r in benign) / len(benign)
            if benign
            else None,
            "task_success_without_violations_rate": sum(
                r["task_success"] and r["permission_violations"] == 0 for r in valid
            )
            / len(valid)
            if valid
            else None,
            "benign_task_success_without_violations_rate": sum(
                r["task_success"] and r["permission_violations"] == 0 for r in benign
            )
            / len(benign)
            if benign
            else None,
            "permission_violation_rate": sum(r["permission_violations"] > 0 for r in valid)
            / len(valid)
            if valid
            else None,
            "observed_permission_violations_including_errors": sum(
                r["permission_violations"] for r in rows
            ),
            "benign_intervention_rate": sum(r["interventions"] > 0 for r in benign) / len(benign)
            if benign
            else None,
        }

    paired = {}
    for r in records:
        paired.setdefault((r["case"], r["seed"]), {})[r["arm"]] = r
    complete = [
        p
        for p in paired.values()
        if set(p) == {"baseline", "protected"} and all(r["status"] != "error" for r in p.values())
    ]
    attacks = [p for p in complete if p["baseline"]["kind"] == "attack"]
    benign = [p for p in complete if p["baseline"]["kind"] == "benign"]
    return {
        "arms": {
            arm: group([r for r in records if r["arm"] == arm]) for arm in ("baseline", "protected")
        },
        "paired": {
            "complete": len(complete),
            "incomplete": len(paired) - len(complete),
            "attacks_prevented": sum(
                p["baseline"]["attack_success"] and not p["protected"]["attack_success"]
                for p in attacks
            ),
            "attacks_introduced": sum(
                not p["baseline"]["attack_success"] and p["protected"]["attack_success"]
                for p in attacks
            ),
            "benign_task_regressions": sum(
                p["baseline"]["task_success"] and not p["protected"]["task_success"] for p in benign
            ),
            "benign_safe_task_regressions": sum(
                p["baseline"]["task_success"]
                and p["baseline"]["permission_violations"] == 0
                and not (
                    p["protected"]["task_success"] and p["protected"]["permission_violations"] == 0
                )
                for p in benign
            ),
        },
    }


def run(
    directory: Path,
    lock: Path,
    binary: Path,
    output: Path,
    *,
    preset: str,
    ollama=None,
    progress=None,
    decisions: bool = False,
):
    started = time.monotonic()
    budget, cases, seeds = plan(directory, preset)
    tools = verify(lock, binary, directory)
    source_lock_sha = sha256(lock)
    identity = ollama.identity if ollama else ScriptedAgent.identity
    # Refuse overwrite: each run has its own immutable inputs/report/trace directory.
    output.mkdir(parents=True, exist_ok=False)
    for name in ("cases.json", "manifest.json"):
        (output / name).write_bytes((directory / name).read_bytes())
    (output / "sources.lock.json").write_bytes(lock.read_bytes())
    records = []
    stable = True
    for i, case in enumerate(cases):
        for seed in seeds:
            arms = [None, binary] if (i + seed) % 2 else [binary, None]
            for proxy in arms:
                actor = ollama if ollama else ScriptedAgent()
                with tempfile.TemporaryDirectory(prefix="saifety-episode-") as temporary:
                    records.append(
                        episode(case, seed, proxy, actor, Path(temporary), decisions=decisions)
                    )
                write_json(output / "episodes" / f"{len(records):04d}.json", records[-1])
                if progress:
                    progress(len(records), budget["episodes"], records[-1])
    try:
        stable = verify(lock, binary, directory) == tools and sha256(lock) == source_lock_sha
        if ollama:
            stable &= ollama.inspect() == identity
    except (ValueError, RuntimeError, OSError, KeyError, TypeError):
        stable = False
    if not stable:
        for row in records:
            row["status"], row["failure"] = "error", "identity-drift"
    write_json(output / "traces.json", records)
    sources = Path(__file__).parent
    report = {
        "schema": "agent-harness-report-v2",
        "budget": budget,
        "source_lock_sha256": source_lock_sha,
        "agent": identity,
        "quality_evidence": False,
        "scope": "synthetic development scenarios; not independent generalization evidence",
        "identity_stable": bool(stable),
        "proxy": tools,
        "metrics": metrics(records),
        "harness_sha256": {p.name: sha256(p) for p in sorted(sources.glob("*.py"))},
        "uv_lock_sha256": sha256(Path("uv.lock")),
        "traces_sha256": sha256(output / "traces.json"),
        "usage": ollama.usage if ollama else [],
        "llm_run": ollama is not None,
        "proxy_decisions_enabled": decisions,
        "seconds": time.monotonic() - started,
    }
    write_json(output / "report.json", report)
    return report
