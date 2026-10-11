"""Local harness commands. Planning and scripted runs never contact an LLM."""

import sys
from pathlib import Path

from . import pins
from .suite import plan


def add_parser(sub):
    for command in (
        "bootstrap-agent-harness",
        "prepare-agent-harness",
        "agent-harness-plan",
        "run-agent-harness",
    ):
        p = sub.add_parser(command)
        p.add_argument("--lock", type=Path, default=Path("agent-harness.sources.lock.json"))
        if command == "bootstrap-agent-harness":
            p.add_argument("--out", type=Path, default=Path("bin/agent-harness"))
        else:
            p.add_argument("--data", type=Path, default=Path("data/agent-harness"))
        if command in ("agent-harness-plan", "run-agent-harness"):
            p.add_argument("--preset", choices=("smoke", "full"), default="smoke")
        if command == "run-agent-harness":
            p.add_argument(
                "--binary", type=Path, default=Path("bin/agent-harness") / pins.binary_name()
            )
            p.add_argument("--out", type=Path, required=True)
            p.add_argument("--agent", choices=("scripted", "ollama"), required=True)
            p.add_argument("--model")
            p.add_argument("--ollama-url", default="http://127.0.0.1:11434")


def dispatch(args):
    match args.command:
        case "bootstrap-agent-harness":
            return pins.bootstrap(args.lock, args.out)
        case "prepare-agent-harness":
            return pins.prepare(args.lock, args.data)
        case "agent-harness-plan":
            return plan(args.data, args.preset)[0]
        case "run-agent-harness":
            from .agents import OllamaAgent
            from .runner import run

            # Verify scenario/binary identities before any local model request.
            pins.verify(args.lock, args.binary, args.data)
            plan(args.data, args.preset)
            if args.out.exists():
                raise ValueError("output directory already exists; choose a fresh run directory")
            if args.agent == "ollama" and not args.model:
                raise ValueError("--model is required for Ollama; select an installed exact tag")
            if args.agent == "scripted" and args.model:
                raise ValueError("--model is only supported with --agent ollama")
            agent = OllamaAgent(args.ollama_url, args.model) if args.agent == "ollama" else None

            def progress(done, total, row):
                print(
                    f"agent-harness {done}/{total}: {row['case']} {row['arm']} {row['status']}",
                    file=sys.stderr,
                )

            result = run(
                args.data,
                args.lock,
                args.binary,
                args.out,
                preset=args.preset,
                ollama=agent,
                progress=progress,
            )
            if any(a["errors"] for a in result["metrics"]["arms"].values()):
                raise RuntimeError("episodes failed; inspect report.json and traces.json")
            return result
