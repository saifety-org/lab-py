"""Fixed fake MCP server. All tools manipulate World; no shell/network/file tools."""

import json
import sys
from pathlib import Path

from .world import TOOLS, World


def serve(case: dict, state: Path, stdin, stdout) -> None:
    world = World(case)
    state.write_text(json.dumps(world.snapshot()))
    while line := stdin.readline((1 << 20) + 1):
        if len(line) > 1 << 20:
            raise ValueError("MCP input too large")
        req = json.loads(line)
        if "id" not in req:
            continue
        method, params = req["method"], req.get("params", {})
        if method == "initialize":
            result = {
                "protocolVersion": "2025-06-18",
                "capabilities": {"tools": {}},
                "serverInfo": {"name": "synthetic-world", "version": "1"},
            }
        elif method == "tools/list":
            result = {
                "tools": [
                    {
                        "name": name,
                        "description": f"Operate on virtual {name} data",
                        "inputSchema": {
                            "type": "object",
                            "required": list(args),
                            "properties": {k: {"type": t} for k, t in args.items()},
                            "additionalProperties": False,
                        },
                    }
                    for name, args in TOOLS.items()
                ]
            }
        elif method == "tools/call":
            try:
                result = world.call(params["name"], params["arguments"])
            except (KeyError, ValueError):
                result = {
                    "isError": True,
                    "content": [{"type": "text", "text": "Invalid virtual action"}],
                }
            temporary = state.with_suffix(".partial")
            temporary.write_text(json.dumps(world.snapshot()))
            temporary.replace(state)
        else:
            stdout.write(
                json.dumps(
                    {
                        "jsonrpc": "2.0",
                        "id": req["id"],
                        "error": {"code": -32601, "message": "Unsupported method"},
                    }
                )
                + "\n"
            )
            stdout.flush()
            continue
        stdout.write(json.dumps({"jsonrpc": "2.0", "id": req["id"], "result": result}) + "\n")
        stdout.flush()


if __name__ == "__main__":
    serve(
        json.loads(Path(sys.argv[1]).read_text(encoding="utf-8")),
        Path(sys.argv[2]),
        sys.stdin,
        sys.stdout,
    )
