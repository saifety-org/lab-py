"""Real stdio MCP sessions over the pinned Go proxy or the same fake server."""

import hashlib
import json
import os
import queue
import signal
import subprocess
import sys
import threading
from pathlib import Path

from ..io import write_json
from .world import TOOLS


def stop(process: subprocess.Popen) -> None:
    if process.stdin:
        try:
            process.stdin.close()
        except OSError:
            pass
    try:
        process.wait(timeout=3)
    except subprocess.TimeoutExpired:
        if os.name == "nt":
            subprocess.run(
                ["taskkill", "/PID", str(process.pid), "/T", "/F"],
                capture_output=True,
                timeout=10,
                check=False,
            )
        else:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
        process.wait(timeout=10)
    # A crashed parent may leave its upstream alive in the private process group.
    if os.name != "nt":
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass


class Session:
    def __init__(
        self,
        case: dict,
        directory: Path,
        executable: Path | None,
        *,
        timeout: float = 15,
        decisions: bool = False,
    ):
        self.directory, self.timeout, self.ident = directory, timeout, 0
        self.responses = queue.Queue(maxsize=32)
        self.logs = []
        self.threads = []
        self.process = None
        self.decision_path = (
            directory / "proxy-decisions.jsonl" if executable and decisions else None
        )
        self.last_decision = None
        directory.mkdir(parents=True, exist_ok=True)
        write_json(directory / "case.json", case)
        server = [
            sys.executable,
            "-m",
            "saifety_lab_py.harness.server",
            str(directory / "case.json"),
            str(directory / "world.json"),
        ]
        # No inherited credentials, user configs, proxy variables or real client config.
        env = {
            k: os.environ[k]
            for k in ("PATH", "SYSTEMROOT", "WINDIR", "TMP", "TEMP")
            if k in os.environ
        }
        env.update(
            HOME=str(directory),
            USERPROFILE=str(directory),
            XDG_CONFIG_HOME=str(directory),
            XDG_STATE_HOME=str(directory),
            XDG_CACHE_HOME=str(directory),
            PYTHONNOUSERSITE="1",
            PYTHONIOENCODING="utf-8",
            PYTHONUTF8="1",
            PYTHONPATH=str(Path(__file__).resolve().parents[2]),
            SAIFETY_STATE=str(directory / "blocklist.json"),
        )
        if executable:
            config = {
                "classifier": "trained",
                "policy": "strict",
                "redact": {"noModel": True},
                "mcpServers": {"fixture": {"command": server[0], "args": server[1:]}},
            }
            write_json(directory / "config.json", config)
            command = [
                str(executable.resolve()),
                "proxy",
                "-config",
                str(directory / "config.json"),
                "-state",
                str(directory / "blocklist.json"),
                "-task",
                case["task"]["text"],
            ]
            if self.decision_path:
                command += ["-decision-log", str(self.decision_path)]
        else:
            command = server
        self.process = subprocess.Popen(
            command,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            encoding="utf-8",
            env=env,
            start_new_session=os.name != "nt",
            creationflags=(subprocess.CREATE_NEW_PROCESS_GROUP if os.name == "nt" else 0),
        )
        self.threads = [
            threading.Thread(target=self._read, daemon=True),
            threading.Thread(target=self._logs, daemon=True),
        ]
        for thread in self.threads:
            thread.start()
        try:
            self.call(
                "initialize",
                {
                    "protocolVersion": "2025-06-18",
                    "capabilities": {},
                    "clientInfo": {"name": "agent-harness", "version": "1"},
                },
            )
            self.notify("notifications/initialized", {})
            names = {t["name"] for t in self.call("tools/list", {})["tools"]}
            prefix = "fixture__" if executable else ""
            self.names = {name: prefix + name for name in TOOLS}
            if set(self.names.values()) != names:
                raise ValueError("MCP tool catalog mismatch")
        except BaseException:
            self.close()
            raise

    def _read(self) -> None:
        try:
            while True:
                line = self.process.stdout.readline((1 << 20) + 1)
                if not line:
                    raise RuntimeError("MCP process ended")
                if len(line) > 1 << 20:
                    raise ValueError("MCP response too large")
                message = json.loads(line)
                if "id" in message:
                    self.responses.put_nowait(message)
        except (ValueError, RuntimeError, queue.Full, OSError):
            try:
                self.responses.put_nowait({"error": {"message": "MCP reader failed"}})
            except queue.Full:
                pass

    def _logs(self) -> None:
        for _ in range(1024):
            line = self.process.stderr.readline(4096)
            if not line:
                return
            # Save only structured shadow metadata; never arbitrary runtime diagnostics.
            marker = "experimental-context: "
            if marker in line:
                try:
                    self.logs.append(json.loads(line.split(marker, 1)[1]))
                except ValueError:
                    pass
        # Continue draining to prevent child deadlock without retaining unbounded output.
        while self.process.stderr.read(4096):
            pass

    def notify(self, method: str, params: dict) -> None:
        self.process.stdin.write(
            json.dumps({"jsonrpc": "2.0", "method": method, "params": params}) + "\n"
        )
        self.process.stdin.flush()

    def call(self, method: str, params: dict) -> dict:
        self.ident += 1
        self.process.stdin.write(
            json.dumps({"jsonrpc": "2.0", "id": self.ident, "method": method, "params": params})
            + "\n"
        )
        self.process.stdin.flush()
        try:
            result = self.responses.get(timeout=self.timeout)
        except queue.Empty as error:
            raise RuntimeError("MCP deadline exceeded") from error
        if "error" in result or result.get("id") != self.ident:
            raise RuntimeError("MCP response error or identity mismatch")
        return result["result"]

    def tool(self, name: str, arguments: dict) -> dict:
        self.last_decision = None
        result = self.call("tools/call", {"name": self.names[name], "arguments": arguments})
        if self.decision_path:
            with self.decision_path.open("rb") as stream:
                raw = stream.read((1 << 20) + 1)
            if len(raw) > 1 << 20:
                raise ValueError("proxy decision log exceeds limit")
            # The real proxy writes the event before replying. Never trust result _meta.
            rows = [json.loads(line) for line in raw.splitlines()]
            if any(not isinstance(row, dict) for row in rows):
                raise ValueError("invalid proxy decision log")
            expected = hashlib.sha256(str(self.ident).encode()).hexdigest()
            matches = [r for r in rows if r.get("request_sha256") == expected]
            if len(matches) != 1:
                raise ValueError("missing or duplicate proxy decision")
            decision = matches[0]
            if (
                decision.get("schema") != "saifety-proxy-decision-v1"
                or decision.get("delivery") not in ("original", "transformed", "withheld")
                or type(decision.get("upstream_called")) is not bool
            ):
                raise ValueError("invalid proxy decision")
            self.last_decision = decision
        return result

    def state(self) -> dict:
        return json.loads((self.directory / "world.json").read_text(encoding="utf-8"))

    def close(self) -> None:
        if self.process:
            stop(self.process)
            for thread in self.threads:
                thread.join(timeout=3)
            for stream in (self.process.stdout, self.process.stderr):
                stream.close()
            self.process = None
