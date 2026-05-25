"""Group 9: shared helpers for daemon end-to-end integration tests.

These tests spawn a REAL `uvx mcp-proxy` process (no `AI_SPECS_MCP_DAEMON_FAKE`),
verify the HTTP `/status` endpoint, and assert that per-agent renders contain a
URL pointing at the running daemon. The helpers below cover fixture setup
(`ai-specs init` + git init + manifest overwrite + pre-staged named-config) and
deterministic cleanup (SIGTERM the spawned proxy + its process group).
"""
from __future__ import annotations

import json
import os
import shutil
import signal
import subprocess
import time
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
SYNC_SH = ROOT / "lib" / "sync.sh"
DAEMON_PY = ROOT / "lib" / "_internal" / "mcp-daemon.py"
RECIPE_MATERIALIZE_PY = ROOT / "lib" / "_internal" / "recipe-materialize.py"

# A real lightweight stdio MCP that mcp-proxy can host. Cached by uvx after
# first invocation (~<1s subsequent). mcp-proxy's HTTP listener only comes up
# after at least one upstream MCP has handshaked, so an empty or stub upstream
# leaves /status unreachable.
INNER_MCP_TIME = {"command": "uvx", "args": ["mcp-server-time"]}


def uvx_available() -> bool:
    return shutil.which("uvx") is not None


def init_workspace(parent: Path, name: str = "workspace") -> Path:
    """Run `ai-specs init` then `git init` so the workspace looks like a real project."""
    workspace = (parent / name).resolve()
    workspace.mkdir()
    subprocess.run(
        [str(CLI), "init", str(workspace)],
        check=True, text=True, capture_output=True,
    )
    subprocess.run(
        ["git", "init", "-q", str(workspace)],
        check=True, capture_output=True,
    )
    # Identity for any git commits a downstream step might attempt.
    subprocess.run(
        ["git", "-C", str(workspace), "config", "user.email", "g9@test"],
        check=True, capture_output=True,
    )
    subprocess.run(
        ["git", "-C", str(workspace), "config", "user.name", "g9-test"],
        check=True, capture_output=True,
    )
    return workspace


def write_manifest_toml(workspace: Path, body: str) -> None:
    """Overwrite ai-specs/ai-specs.toml with `body`."""
    toml = workspace / "ai-specs" / "ai-specs.toml"
    toml.write_text(body)


def stage_named_config(workspace: Path, servers: dict) -> Path:
    """Pre-create `.ai-specs/run/proxy.named-config.json` so sync.sh fires ensure-daemon.

    `servers` is the inner `mcpServers` dict mapping id -> {command, args, env?}.
    The file is what `recipe-materialize` would normally emit when a recipe with
    `mode = "shared"` is enabled; pre-staging lets the manifest-only fixture
    trigger the daemon path without inventing a synthetic recipe.
    """
    run_dir = workspace / ".ai-specs" / "run"
    run_dir.mkdir(parents=True, exist_ok=True)
    named = run_dir / "proxy.named-config.json"
    named.write_text(json.dumps({"mcpServers": servers}, indent=2) + "\n")
    os.chmod(named, 0o600)
    return named


def run_sync(
    workspace: Path,
    *,
    extra_env: dict | None = None,
    timeout: int = 180,
) -> subprocess.CompletedProcess:
    env = dict(os.environ)
    env["AI_SPECS_HOME"] = str(ROOT)
    # NEVER set AI_SPECS_MCP_DAEMON_FAKE here — G9 is the real-spawn suite.
    env.pop("AI_SPECS_MCP_DAEMON_FAKE", None)
    if extra_env:
        env.update(extra_env)
    return subprocess.run(
        [str(CLI), "sync", str(workspace)],
        env=env, capture_output=True, text=True, timeout=timeout,
    )


def run_daemon_stop(workspace: Path, timeout: int = 30) -> subprocess.CompletedProcess:
    env = dict(os.environ)
    env["AI_SPECS_HOME"] = str(ROOT)
    return subprocess.run(
        [str(CLI), "daemon", "stop"],
        cwd=str(workspace),
        env=env, capture_output=True, text=True, timeout=timeout,
    )


def read_pid(workspace: Path) -> int | None:
    return _read_int(workspace / ".ai-specs" / "run" / "proxy.pid")


def read_port(workspace: Path) -> int | None:
    return _read_int(workspace / ".ai-specs" / "run" / "proxy.port")


def _read_int(path: Path) -> int | None:
    try:
        return int(path.read_text().strip())
    except (FileNotFoundError, ValueError):
        return None


def pid_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def reap_proxy(workspace: Path) -> None:
    """SIGTERM the spawned mcp-proxy and its process group; SIGKILL on holdout.

    `mcp-daemon._spawn` starts the proxy with `start_new_session=True`, so the
    PID is a session/process-group leader — killing the group reaps both the
    proxy and its child stdio MCPs.
    """
    pid = read_pid(workspace)
    if pid is None or not pid_alive(pid):
        return
    _signal_group(pid, signal.SIGTERM)
    deadline = time.monotonic() + 5.0
    while time.monotonic() < deadline and pid_alive(pid):
        time.sleep(0.05)
    if pid_alive(pid):
        _signal_group(pid, signal.SIGKILL)
        time.sleep(0.1)


def _signal_group(pid: int, sig: int) -> None:
    try:
        pgid = os.getpgid(pid)
    except ProcessLookupError:
        return
    try:
        os.killpg(pgid, sig)
    except (ProcessLookupError, PermissionError):
        try:
            os.kill(pid, sig)
        except ProcessLookupError:
            pass


def wait_for_status(port: int, *, timeout: float = 20.0):
    """Poll `GET http://localhost:{port}/status` until 200 or `timeout` expires.

    Returns the urllib response (already-consumed bytes available via .body).
    Raises RuntimeError on timeout — caller asserts.
    """
    deadline = time.monotonic() + timeout
    last_err: Exception | None = None
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(  # noqa: S310 — localhost only
                f"http://localhost:{port}/status", timeout=2
            ) as resp:
                body = resp.read()
                return _Resp(resp.status, body)
        except (urllib.error.URLError, OSError, ValueError) as exc:
            last_err = exc
            time.sleep(0.2)
    raise RuntimeError(
        f"daemon /status on port {port} never returned within {timeout}s "
        f"(last error: {last_err!r})"
    )


class _Resp:
    __slots__ = ("status", "body")

    def __init__(self, status: int, body: bytes) -> None:
        self.status = status
        self.body = body
