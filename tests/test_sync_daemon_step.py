"""Group 5.1 — `sync.sh` conditional `ensure mcp-proxy daemon` step.

The pipeline must:
  - invoke `python3 lib/_internal/mcp-daemon.py ensure <root> --named-config <p>`
    AFTER recipe-materialize and BEFORE the target fan-out, but only when
    `<root>/.ai-specs/run/proxy.named-config.json` exists post-materialize.
  - skip the daemon step entirely when that file does not exist.
  - abort the pipeline (no fan-out) when the daemon ensure fails.

Tests pre-stage `proxy.named-config.json` to simulate the post-materialize
state for a workspace whose manifest has no enabled recipes — materialize
then leaves the pre-staged file alone, sync.sh sees it, and the conditional
fires. `AI_SPECS_MCP_DAEMON_FAKE=1` stubs the real `uvx mcp-proxy` spawn.
"""
from __future__ import annotations

import json
import os
import shutil
import signal
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
SYNC_SH = ROOT / "lib" / "sync.sh"


def _ai_specs_init(workspace: Path) -> None:
    subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True, capture_output=True)
    # Clear default agents so fan-out is a fast no-op when it runs.
    toml = workspace / "ai-specs" / "ai-specs.toml"
    text = toml.read_text()
    import re
    text = re.sub(r"(?m)^enabled\s*=\s*\[.*?\]\s*$", "enabled = []", text, count=1)
    toml.write_text(text)


def _stage_named_config(workspace: Path) -> Path:
    """Pre-create the post-materialize named-config to trigger the conditional."""
    run_dir = workspace / ".ai-specs" / "run"
    run_dir.mkdir(parents=True, exist_ok=True)
    named = run_dir / "proxy.named-config.json"
    named.write_text(json.dumps({"mcpServers": {"trello": {"command": "echo", "args": ["hi"]}}}))
    return named


def _run_sync(workspace: Path, *, fake: bool = True, extra_env: dict[str, str] | None = None):
    env = dict(os.environ)
    if fake:
        env["AI_SPECS_MCP_DAEMON_FAKE"] = "1"
    if extra_env:
        env.update(extra_env)
    return subprocess.run(
        [str(CLI), "sync", str(workspace)],
        env=env,
        capture_output=True,
        text=True,
        timeout=120,
    )


def _reap_pid_from(workspace: Path) -> None:
    pid_file = workspace / ".ai-specs" / "run" / "proxy.pid"
    if not pid_file.exists():
        return
    try:
        pid = int(pid_file.read_text().strip())
    except ValueError:
        return
    try:
        os.kill(pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    # Best-effort reap; the spawned sleeper is detached so wait may not apply.
    try:
        os.waitpid(pid, os.WNOHANG)
    except ChildProcessError:
        pass


class SyncEnsureDaemonStepTests(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory(prefix="ai-specs-sync-daemon-")
        self.addCleanup(self.tmpdir.cleanup)
        self.workspace = Path(self.tmpdir.name).resolve() / "workspace"
        self.workspace.mkdir()
        # Pre-resolve in case macOS adds /private prefix; sync.sh uses realpath.
        self.workspace = self.workspace.resolve()
        _ai_specs_init(self.workspace)
        self.addCleanup(_reap_pid_from, self.workspace)

    def test_sync_with_shared_named_config_invokes_ensure_daemon(self):
        _stage_named_config(self.workspace)

        proc = _run_sync(self.workspace, fake=True)

        self.assertEqual(proc.returncode, 0, msg=proc.stderr or proc.stdout)
        self.assertIn("ensure mcp-proxy daemon", proc.stdout)
        pid_file = self.workspace / ".ai-specs" / "run" / "proxy.pid"
        self.assertTrue(
            pid_file.is_file(),
            f"expected proxy.pid at {pid_file}; sync stdout:\n{proc.stdout}",
        )

    def test_sync_without_named_config_skips_daemon_step(self):
        # No proxy.named-config.json staged → conditional must skip.
        proc = _run_sync(self.workspace, fake=True)

        self.assertEqual(proc.returncode, 0, msg=proc.stderr or proc.stdout)
        self.assertNotIn("ensure mcp-proxy daemon", proc.stdout)
        pid_file = self.workspace / ".ai-specs" / "run" / "proxy.pid"
        self.assertFalse(pid_file.exists(), "no daemon should have been started")

    def test_sync_daemon_failure_aborts_before_fanout(self):
        named = _stage_named_config(self.workspace)
        # Sabotage: chmod 000 → ensure_daemon raises PermissionError on read.
        os.chmod(named, 0o000)
        self.addCleanup(os.chmod, named, 0o600)

        # Do NOT set FAKE; we want the real ensure_daemon path that reads the
        # named-config first and dies on PermissionError before any spawn.
        proc = _run_sync(self.workspace, fake=False)

        self.assertNotEqual(proc.returncode, 0, msg="sync should fail when ensure_daemon fails")
        # sync.sh prints "ERROR: daemon ensure failed" on the daemon-ensure failure path.
        self.assertIn("daemon ensure failed", proc.stdout + proc.stderr)
        # Fan-out must NOT have executed — sync-agent would emit "▸ root:."
        # ahead of any agent work; assert it never reached that step.
        self.assertNotIn("target fan-out", proc.stdout + proc.stderr)


if __name__ == "__main__":
    unittest.main()
