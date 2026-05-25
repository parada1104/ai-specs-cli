#!/usr/bin/env python3
"""Lifecycle manager for the shared ``mcp-proxy`` daemon (one per git root).

This module owns spawning, healthchecking, stopping, status reporting,
and restarting the multiplexing HTTP daemon that serves all MCPs marked
with ``mode = "shared"``. State lives under ``<git-root>/.ai-specs/run/``
so worktrees of the same repo share a single daemon process.

Public API (see `openspec/changes/mcp-compartido-por-proyecto/design.md`):
    - ensure_daemon(git_root, named_config_path) -> int
    - healthcheck(port, timeout=2.0) -> bool
    - stop_daemon(git_root) -> bool
    - status_daemon(git_root) -> dict | None
    - restart_daemon(git_root, named_config_path) -> int

CLI: invoke this file directly (the name contains a dash, so
``python3 -m lib._internal.mcp-daemon`` is not viable)::

    python3 lib/_internal/mcp-daemon.py ensure  <git_root> --named-config <path>
    python3 lib/_internal/mcp-daemon.py stop    <git_root>
    python3 lib/_internal/mcp-daemon.py status  <git_root>
    python3 lib/_internal/mcp-daemon.py restart <git_root> --named-config <path>

Test hooks:
    - Module attribute ``_POPEN`` may be replaced to substitute the real
      ``subprocess.Popen``. Defaults to ``subprocess.Popen``.
    - When the env var ``AI_SPECS_MCP_DAEMON_FAKE=1`` is set, the module
      installs a sleeper-spawning stub and forces ``healthcheck`` to True
      so the CLI can be exercised in tests without requiring ``uvx``.
"""
from __future__ import annotations

import argparse
import contextlib
import errno
import fcntl
import hashlib
import json
import os
import signal
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

STATE_SUBDIR = Path(".ai-specs") / "run"
PID_FILE = "proxy.pid"
PORT_FILE = "proxy.port"
LOCK_FILE = "proxy.lock"
LOG_FILE = "proxy.log"
HASH_FILE = "proxy.config-hash"
NAMED_CONFIG_FILE = "proxy.named-config.json"
HEALTHCHECK_TIMEOUT = 2.0
START_HEALTH_WAIT = 5.0
START_HEALTH_POLL = 0.1
STOP_WAIT = 5.0
STOP_POLL = 0.05

# Test hook: real spawner. Tests overwrite to inject fakes.
_POPEN = subprocess.Popen


# --- helpers ----------------------------------------------------------------


def _state_dir(git_root: Path) -> Path:
    """Return ``<git_root>/.ai-specs/run/``, creating it if missing."""
    d = Path(git_root) / STATE_SUBDIR
    d.mkdir(parents=True, exist_ok=True)
    return d


def _pick_free_port() -> int:
    """Ask the kernel for an ephemeral free port and immediately release it."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("", 0))
        return s.getsockname()[1]


def _is_pid_alive(pid: int) -> bool:
    """Liveness probe via ``os.kill(pid, 0)``. Treats EPERM as alive."""
    if pid is None or pid <= 0:
        return False
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    except OSError as exc:
        return exc.errno == errno.EPERM
    return True


def _hash_config(path: Path) -> str:
    """SHA-256 of the canonical JSON form of ``path`` (sorted keys, compact)."""
    with open(path, "rb") as f:
        data = json.loads(f.read().decode("utf-8"))
    canonical = json.dumps(data, sort_keys=True, separators=(",", ":")).encode("utf-8")
    return hashlib.sha256(canonical).hexdigest()


def _atomic_write(path: Path, content: str) -> None:
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(content)
    os.replace(tmp, path)


@contextlib.contextmanager
def _acquire_lock(state_dir: Path):
    """Exclusive ``fcntl`` flock over ``state_dir/proxy.lock``."""
    state_dir.mkdir(parents=True, exist_ok=True)
    lock_path = state_dir / LOCK_FILE
    fd = os.open(str(lock_path), os.O_RDWR | os.O_CREAT, 0o600)
    try:
        fcntl.flock(fd, fcntl.LOCK_EX)
        yield fd
    finally:
        try:
            fcntl.flock(fd, fcntl.LOCK_UN)
        finally:
            os.close(fd)


# --- healthcheck ------------------------------------------------------------


def healthcheck(port: int, timeout: float = HEALTHCHECK_TIMEOUT) -> bool:
    """True iff ``GET http://localhost:{port}/status`` returns HTTP 200."""
    url = f"http://localhost:{port}/status"
    try:
        with urllib.request.urlopen(url, timeout=timeout) as resp:  # noqa: S310 (loopback)
            return getattr(resp, "status", resp.getcode()) == 200
    except (urllib.error.URLError, urllib.error.HTTPError, socket.timeout, ConnectionError, OSError):
        return False


# --- internal state I/O -----------------------------------------------------


def _read_int(path: Path):
    try:
        return int(path.read_text().strip())
    except (FileNotFoundError, ValueError):
        return None


def _read_pid(state_dir: Path):
    return _read_int(state_dir / PID_FILE)


def _read_port(state_dir: Path):
    return _read_int(state_dir / PORT_FILE)


def _read_hash(state_dir: Path):
    p = state_dir / HASH_FILE
    if not p.exists():
        return None
    try:
        return p.read_text().strip()
    except OSError:
        return None


def _cleanup_state(state_dir: Path, files=(PID_FILE, PORT_FILE, HASH_FILE)) -> None:
    for name in files:
        with contextlib.suppress(FileNotFoundError):
            (state_dir / name).unlink()


def _sigterm_and_wait(pid: int, timeout: float = STOP_WAIT) -> None:
    with contextlib.suppress(ProcessLookupError, PermissionError):
        os.kill(pid, signal.SIGTERM)
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline and _is_pid_alive(pid):
        time.sleep(STOP_POLL)


def _wait_until_healthy(port: int, timeout: float = START_HEALTH_WAIT) -> bool:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if healthcheck(port, timeout=0.5):
            return True
        time.sleep(START_HEALTH_POLL)
    return False


def _spawn(port: int, named_config_path: Path, state_dir: Path) -> int:
    log_path = state_dir / LOG_FILE
    log_fd = open(log_path, "ab")
    try:
        cmd = [
            "uvx", "mcp-proxy",
            "--port", str(port),
            "--named-server-config", str(named_config_path),
        ]
        proc = _POPEN(
            cmd,
            start_new_session=True,
            stdout=log_fd,
            stderr=log_fd,
            stdin=subprocess.DEVNULL,
            close_fds=True,
        )
    finally:
        log_fd.close()
    return proc.pid


def _persist_running(state_dir: Path, pid: int, port: int, config_hash: str) -> None:
    _atomic_write(state_dir / PID_FILE, str(pid))
    _atomic_write(state_dir / PORT_FILE, str(port))
    _atomic_write(state_dir / HASH_FILE, config_hash)


# --- public API -------------------------------------------------------------


def ensure_daemon(git_root: Path, named_config_path: Path) -> int:
    """Idempotent: return the port of a healthy daemon, spawning if needed."""
    git_root = Path(git_root)
    named_config_path = Path(named_config_path)
    state_dir = _state_dir(git_root)
    config_hash = _hash_config(named_config_path)
    with _acquire_lock(state_dir):
        pid = _read_pid(state_dir)
        port = _read_port(state_dir)
        prev_hash = _read_hash(state_dir)
        if pid is not None and port is not None:
            alive = _is_pid_alive(pid)
            healthy = alive and healthcheck(port)
            if alive and healthy and prev_hash == config_hash:
                return port
            if alive:
                _sigterm_and_wait(pid)
            _cleanup_state(state_dir)
        new_port = _pick_free_port()
        new_pid = _spawn(new_port, named_config_path, state_dir)
        _persist_running(state_dir, new_pid, new_port, config_hash)
        # Best-effort wait for healthy; do NOT block forever if healthcheck never
        # returns True (e.g. test stubs that always say False).
        _wait_until_healthy(new_port)
        return new_port


def stop_daemon(git_root: Path) -> bool:
    """SIGTERM the daemon, cleanup state. Returns True iff a live PID was killed."""
    git_root = Path(git_root)
    state_dir = git_root / STATE_SUBDIR
    if not state_dir.exists():
        return False
    with _acquire_lock(state_dir):
        pid = _read_pid(state_dir)
        cleaned = (PID_FILE, PORT_FILE, HASH_FILE, NAMED_CONFIG_FILE)
        if pid is None:
            _cleanup_state(state_dir, cleaned)
            return False
        was_alive = _is_pid_alive(pid)
        if was_alive:
            _sigterm_and_wait(pid)
        _cleanup_state(state_dir, cleaned)
        return was_alive


def status_daemon(git_root: Path):
    """Return ``{pid, port, uptime_s}`` if the daemon is alive, else ``None``."""
    git_root = Path(git_root)
    state_dir = git_root / STATE_SUBDIR
    if not state_dir.exists():
        return None
    pid = _read_pid(state_dir)
    port = _read_port(state_dir)
    if pid is None or port is None or not _is_pid_alive(pid):
        return None
    pid_file = state_dir / PID_FILE
    try:
        uptime_s = max(0, int(time.time() - pid_file.stat().st_mtime))
    except FileNotFoundError:
        uptime_s = 0
    return {"pid": pid, "port": port, "uptime_s": uptime_s}


def restart_daemon(git_root: Path, named_config_path: Path) -> int:
    """``stop_daemon`` followed by ``ensure_daemon``. Returns the new port."""
    stop_daemon(git_root)
    return ensure_daemon(git_root, named_config_path)


# --- test-mode wiring (env-var gated) ---------------------------------------


def _install_test_fakes() -> None:
    """When ``AI_SPECS_MCP_DAEMON_FAKE=1``, replace _POPEN + healthcheck.

    The fake spawns a long-sleeping Python child (a real PID we can SIGTERM)
    and short-circuits healthcheck to True so CLI tests do not need a real
    ``mcp-proxy`` binary in PATH.
    """
    if os.environ.get("AI_SPECS_MCP_DAEMON_FAKE") != "1":
        return
    global _POPEN, healthcheck

    def _fake_popen(cmd, **kwargs):
        # Discard caller's stdout/stderr file descriptors; the sleeper has no output.
        return subprocess.Popen(
            [sys.executable, "-c", "import time; time.sleep(600)"],
            start_new_session=True,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            close_fds=True,
        )

    _POPEN = _fake_popen
    healthcheck = lambda port, timeout=HEALTHCHECK_TIMEOUT: True  # noqa: E731


# --- CLI --------------------------------------------------------------------


def _cli(argv=None) -> int:
    parser = argparse.ArgumentParser(prog="mcp-daemon")
    sub = parser.add_subparsers(dest="cmd", required=True)
    for name in ("ensure", "stop", "status", "restart"):
        sp = sub.add_parser(name)
        sp.add_argument("git_root")
        if name in ("ensure", "restart"):
            sp.add_argument("--named-config", required=True)
    args = parser.parse_args(argv)
    git_root = Path(args.git_root)

    if args.cmd == "ensure":
        port = ensure_daemon(git_root, Path(args.named_config))
        print(port)
        return 0
    if args.cmd == "stop":
        stopped = stop_daemon(git_root)
        print("stopped" if stopped else "no daemon was running")
        return 0
    if args.cmd == "status":
        info = status_daemon(git_root)
        if info is None:
            print("no daemon running")
            return 1
        print(json.dumps(info))
        return 0
    if args.cmd == "restart":
        port = restart_daemon(git_root, Path(args.named_config))
        print(port)
        return 0
    return 2


if __name__ == "__main__":
    _install_test_fakes()
    sys.exit(_cli())
