"""Group 5.3 — `bin/ai-specs daemon …` dispatch + help.

`bin/ai-specs` must route `daemon` to `lib/daemon.sh`, and the help text
must list the `daemon` subcommand.
"""
from __future__ import annotations

import json
import os
import signal
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"


def _git_init(path: Path) -> None:
    subprocess.run(
        ["git", "init", "-q", str(path)],
        check=True,
        capture_output=True,
    )


def _stage_named_config(git_root: Path) -> Path:
    run_dir = git_root / ".ai-specs" / "run"
    run_dir.mkdir(parents=True, exist_ok=True)
    named = run_dir / "proxy.named-config.json"
    named.write_text(json.dumps({"mcpServers": {"trello": {"command": "echo"}}}))
    return named


def _reap_pid_from(git_root: Path) -> None:
    pid_file = git_root / ".ai-specs" / "run" / "proxy.pid"
    if not pid_file.exists():
        return
    try:
        pid = int(pid_file.read_text().strip())
    except ValueError:
        return
    try:
        os.kill(pid, signal.SIGTERM)
    except ProcessLookupError:
        pass


def _run_cli(*args: str, cwd: Path | None = None, fake: bool = True):
    env = dict(os.environ)
    if fake:
        env["AI_SPECS_MCP_DAEMON_FAKE"] = "1"
    return subprocess.run(
        [str(CLI), *args],
        cwd=str(cwd) if cwd else None,
        env=env,
        capture_output=True,
        text=True,
        timeout=30,
    )


class AiSpecsDaemonDispatchTests(unittest.TestCase):
    def test_help_lists_daemon(self):
        proc = _run_cli("help", fake=False)
        self.assertEqual(proc.returncode, 0, msg=proc.stderr)
        self.assertIn("daemon", proc.stdout, "help must mention 'daemon'")
        # Mention the three subcommands so users know the surface.
        for verb in ("stop", "status", "restart"):
            self.assertIn(verb, proc.stdout, f"help must mention '{verb}'")

    def test_dispatch_to_daemon_stop(self):
        with tempfile.TemporaryDirectory(prefix="ai-specs-dispatch-") as tmp:
            git_root = Path(tmp).resolve()
            _git_init(git_root)
            self.addCleanup(_reap_pid_from, git_root)
            proc = _run_cli("daemon", "stop", cwd=git_root, fake=True)
        self.assertEqual(proc.returncode, 0, msg=proc.stderr or proc.stdout)
        self.assertIn("no daemon was running", proc.stdout)

    def test_dispatch_to_daemon_status(self):
        with tempfile.TemporaryDirectory(prefix="ai-specs-dispatch-") as tmp:
            git_root = Path(tmp).resolve()
            _git_init(git_root)
            self.addCleanup(_reap_pid_from, git_root)
            proc = _run_cli("daemon", "status", cwd=git_root, fake=True)
        self.assertEqual(proc.returncode, 1)
        self.assertIn("no daemon running", proc.stdout)

    def test_dispatch_to_daemon_restart(self):
        with tempfile.TemporaryDirectory(prefix="ai-specs-dispatch-") as tmp:
            git_root = Path(tmp).resolve()
            _git_init(git_root)
            _stage_named_config(git_root)
            self.addCleanup(_reap_pid_from, git_root)
            proc = _run_cli("daemon", "restart", cwd=git_root, fake=True)
            self.assertEqual(proc.returncode, 0, msg=proc.stderr or proc.stdout)
            self.assertTrue((git_root / ".ai-specs" / "run" / "proxy.pid").is_file())


if __name__ == "__main__":
    unittest.main()
