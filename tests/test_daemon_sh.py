"""Group 5.2 — `lib/daemon.sh` Bash wrapper.

`lib/daemon.sh <stop|status|restart>` must resolve `git_root` via
`git rev-parse --show-toplevel` and delegate to
`python3 lib/_internal/mcp-daemon.py <subcmd> <git_root> [--named-config ...]`.

The dash in the module filename forces direct-path invocation (not `-m`).
Tests use `AI_SPECS_MCP_DAEMON_FAKE=1` to stub the real `uvx mcp-proxy` spawn.
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
DAEMON_SH = ROOT / "lib" / "daemon.sh"


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


def _run_daemon(git_root: Path, *args: str, fake: bool = True, env_extra: dict[str, str] | None = None):
    env = dict(os.environ)
    if fake:
        env["AI_SPECS_MCP_DAEMON_FAKE"] = "1"
    if env_extra:
        env.update(env_extra)
    return subprocess.run(
        ["bash", str(DAEMON_SH), *args],
        cwd=str(git_root),
        env=env,
        capture_output=True,
        text=True,
        timeout=30,
    )


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


class DaemonShDispatchTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="ai-specs-daemon-sh-")
        self.addCleanup(self.tmp.cleanup)
        self.git_root = Path(self.tmp.name).resolve()
        _git_init(self.git_root)
        self.addCleanup(_reap_pid_from, self.git_root)

    def test_script_exists_and_is_bash(self):
        self.assertTrue(DAEMON_SH.is_file(), f"missing {DAEMON_SH}")
        head = DAEMON_SH.read_text().splitlines()[:1]
        self.assertTrue(head and head[0].startswith("#!"), "missing shebang")

    def test_stop_delegates_to_python_module(self):
        # No daemon active → expect graceful "no daemon was running".
        proc = _run_daemon(self.git_root, "stop", fake=True)
        self.assertEqual(proc.returncode, 0, msg=proc.stderr or proc.stdout)
        self.assertIn("no daemon was running", proc.stdout)

    def test_status_delegates_to_python_module(self):
        # No daemon active → CLI exit 1 ("no daemon running").
        proc = _run_daemon(self.git_root, "status", fake=True)
        self.assertEqual(proc.returncode, 1, msg=proc.stdout + proc.stderr)
        self.assertIn("no daemon running", proc.stdout)

    def test_restart_passes_named_config(self):
        _stage_named_config(self.git_root)
        proc = _run_daemon(self.git_root, "restart", fake=True)
        self.assertEqual(proc.returncode, 0, msg=proc.stderr or proc.stdout)
        # restart_daemon ends in ensure_daemon → state files materialise.
        pid_file = self.git_root / ".ai-specs" / "run" / "proxy.pid"
        self.assertTrue(pid_file.is_file(), "restart must spawn a daemon")

    def test_restart_without_named_config_fails_explicitly(self):
        # No proxy.named-config.json staged.
        proc = _run_daemon(self.git_root, "restart", fake=True)
        self.assertNotEqual(proc.returncode, 0, "restart must fail without named-config")
        combined = proc.stdout + proc.stderr
        self.assertTrue(
            "named-config" in combined.lower() or "named_config" in combined.lower(),
            f"expected named-config error guidance, got:\n{combined}",
        )

    def test_unknown_subcommand_prints_usage(self):
        proc = _run_daemon(self.git_root, "bogus", fake=True)
        self.assertNotEqual(proc.returncode, 0)
        combined = (proc.stdout + proc.stderr).lower()
        self.assertIn("usage", combined)

    def test_missing_subcommand_prints_usage(self):
        proc = _run_daemon(self.git_root, fake=True)
        self.assertNotEqual(proc.returncode, 0)
        combined = (proc.stdout + proc.stderr).lower()
        self.assertIn("usage", combined)

    def test_outside_git_repo_fails_with_clear_message(self):
        with tempfile.TemporaryDirectory(prefix="ai-specs-no-git-") as nogit:
            env = dict(os.environ)
            env["AI_SPECS_MCP_DAEMON_FAKE"] = "1"
            proc = subprocess.run(
                ["bash", str(DAEMON_SH), "status"],
                cwd=nogit,
                env=env,
                capture_output=True,
                text=True,
                timeout=15,
            )
        self.assertNotEqual(proc.returncode, 0)
        combined = (proc.stdout + proc.stderr).lower()
        self.assertIn("git", combined)


if __name__ == "__main__":
    unittest.main()
