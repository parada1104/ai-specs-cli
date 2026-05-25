"""Group 3.12 — CLI entrypoint of `lib/_internal/mcp-daemon.py`.

The CLI is invoked as `python3 lib/_internal/mcp-daemon.py <subcmd> ...`
(the module name contains a dash so `-m lib._internal.mcp-daemon` is not
viable). Tests opt into a fake-spawn mode via the environment variable
`AI_SPECS_MCP_DAEMON_FAKE=1`, which makes the daemon module substitute
its `_POPEN` for a sleeper-spawning stub and force `healthcheck` to True.
"""
from __future__ import annotations

import json
import os
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "lib" / "_internal" / "mcp-daemon.py"


def _run(args, **kwargs):
    env = dict(os.environ)
    env["AI_SPECS_MCP_DAEMON_FAKE"] = "1"
    return subprocess.run(
        [sys.executable, str(SCRIPT), *args],
        capture_output=True,
        text=True,
        env=env,
        timeout=30,
        **kwargs,
    )


class _CliBase(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.named_config = self.root / "named-config.json"
        self.named_config.write_text(json.dumps({"mcpServers": {"trello": {"command": "echo"}}}))
        self._spawned_pids: list[int] = []
        self.addCleanup(self._reap)

    def _state_dir(self) -> Path:
        return self.root / ".ai-specs" / "run"

    def _record_pid_from_state(self) -> int | None:
        pid_file = self._state_dir() / "proxy.pid"
        if not pid_file.exists():
            return None
        try:
            pid = int(pid_file.read_text().strip())
        except ValueError:
            return None
        self._spawned_pids.append(pid)
        return pid

    def _reap(self):
        for pid in self._spawned_pids:
            try:
                os.kill(pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            except PermissionError:
                pass


class CliEnsureTests(_CliBase):
    def test_ensure_writes_state_files_and_prints_port(self):
        result = _run(["ensure", str(self.root), "--named-config", str(self.named_config)])
        self.assertEqual(result.returncode, 0, f"stderr={result.stderr}\nstdout={result.stdout}")
        port = int(result.stdout.strip().splitlines()[-1])
        self.assertGreater(port, 1024)
        pid = self._record_pid_from_state()
        self.assertIsNotNone(pid)
        sd = self._state_dir()
        self.assertEqual(int((sd / "proxy.port").read_text()), port)


class CliStopTests(_CliBase):
    def test_stop_cleans_state_after_ensure(self):
        _run(["ensure", str(self.root), "--named-config", str(self.named_config)])
        self._record_pid_from_state()
        stop_res = _run(["stop", str(self.root)])
        self.assertEqual(stop_res.returncode, 0, stop_res.stderr)
        sd = self._state_dir()
        self.assertFalse((sd / "proxy.pid").exists())
        self.assertFalse((sd / "proxy.port").exists())

    def test_stop_without_daemon_returns_zero_exit(self):
        # stop_daemon spec scenario: must not error if nothing is running.
        result = _run(["stop", str(self.root)])
        self.assertEqual(result.returncode, 0, result.stderr)


class CliStatusTests(_CliBase):
    def test_status_alive_returns_zero(self):
        _run(["ensure", str(self.root), "--named-config", str(self.named_config)])
        self._record_pid_from_state()
        # give the spawned sleeper a moment to register
        time.sleep(0.1)
        result = _run(["status", str(self.root)])
        self.assertEqual(result.returncode, 0, result.stderr)
        # status prints a JSON object on success
        payload = json.loads(result.stdout.strip().splitlines()[-1])
        self.assertIn("pid", payload)
        self.assertIn("port", payload)

    def test_status_no_daemon_returns_one(self):
        result = _run(["status", str(self.root)])
        self.assertEqual(result.returncode, 1, result.stdout)


class CliRestartTests(_CliBase):
    def test_restart_reboots_daemon(self):
        first = _run(["ensure", str(self.root), "--named-config", str(self.named_config)])
        self.assertEqual(first.returncode, 0)
        first_pid = self._record_pid_from_state()
        self.assertIsNotNone(first_pid)
        result = _run(["restart", str(self.root), "--named-config", str(self.named_config)])
        self.assertEqual(result.returncode, 0, result.stderr)
        new_pid = self._record_pid_from_state()
        self.assertIsNotNone(new_pid)
        self.assertNotEqual(first_pid, new_pid)


if __name__ == "__main__":
    unittest.main()
