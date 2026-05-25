"""Group 3.3 → 3.7 — `ensure_daemon` lifecycle branches.

The real `mcp-proxy` binary is not exercised: tests inject a fake
`_POPEN` that spawns a real long-sleeping Python child (gives us a
valid live PID we can SIGTERM), and they patch `healthcheck` directly
on the loaded module to drive the branching.
"""
from __future__ import annotations

import importlib.util
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
MODULE_PATH = ROOT / "lib" / "_internal" / "mcp-daemon.py"

SLEEPER_CODE = "import sys, time; time.sleep(120)"


def load_module():
    spec = importlib.util.spec_from_file_location("mcp_daemon", MODULE_PATH)
    mod = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    sys.modules[spec.name] = mod
    spec.loader.exec_module(mod)
    return mod


class _SpawnRecorder:
    """Fake Popen that launches a real sleeping Python child."""

    def __init__(self):
        self.calls: list[list[str]] = []
        self.kwargs: list[dict] = []
        self.children: list[subprocess.Popen] = []

    def __call__(self, cmd, **kwargs):
        self.calls.append(list(cmd))
        self.kwargs.append(kwargs)
        # Drop stdout/stderr fd plumbing — sleeper does not need it,
        # and the test owns/closes the fds via the module's `finally`.
        sleeper = subprocess.Popen(
            [sys.executable, "-c", SLEEPER_CODE],
            start_new_session=True,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            close_fds=True,
        )
        self.children.append(sleeper)
        return sleeper


class EnsureDaemonBase(unittest.TestCase):
    def setUp(self):
        self.mod = load_module()
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.named_config = self.root / "named-config.json"
        self.named_config.write_text(
            json.dumps({"mcpServers": {"trello": {"command": "echo", "args": ["hi"]}}})
        )
        self.recorder = _SpawnRecorder()
        self._orig_popen = self.mod._POPEN
        self._orig_healthcheck = self.mod.healthcheck
        self.mod._POPEN = self.recorder
        # Default: assume any healthcheck returns True; individual tests override.
        self.mod.healthcheck = lambda port, timeout=2.0: True
        self.addCleanup(self._restore_module)
        self.addCleanup(self._kill_children)

    def _restore_module(self):
        self.mod._POPEN = self._orig_popen
        self.mod.healthcheck = self._orig_healthcheck

    def _kill_children(self):
        for child in self.recorder.children:
            with self.subTest(child=child.pid):
                try:
                    child.terminate()
                    child.wait(timeout=2)
                except Exception:
                    try:
                        child.kill()
                    except Exception:
                        pass

    def _state_dir(self) -> Path:
        return self.root / ".ai-specs" / "run"


class SpawnPathTests(EnsureDaemonBase):
    def test_no_state_files_spawns_and_persists(self):
        port = self.mod.ensure_daemon(self.root, self.named_config)
        sd = self._state_dir()
        self.assertEqual(len(self.recorder.calls), 1, "exactly one spawn expected")
        cmd = self.recorder.calls[0]
        self.assertEqual(cmd[0:2], ["uvx", "mcp-proxy"])
        self.assertIn("--port", cmd)
        self.assertIn(str(port), cmd)
        self.assertIn("--named-server-config", cmd)
        self.assertIn(str(self.named_config), cmd)
        kwargs = self.recorder.kwargs[0]
        self.assertTrue(kwargs.get("start_new_session"), "child must be detached via setsid")
        self.assertTrue((sd / "proxy.pid").is_file())
        self.assertTrue((sd / "proxy.port").is_file())
        self.assertEqual(int((sd / "proxy.port").read_text()), port)
        recorded_pid = int((sd / "proxy.pid").read_text())
        self.assertEqual(recorded_pid, self.recorder.children[0].pid)


class IdempotencyTests(EnsureDaemonBase):
    def test_existing_healthy_daemon_is_reused(self):
        first = self.mod.ensure_daemon(self.root, self.named_config)
        second = self.mod.ensure_daemon(self.root, self.named_config)
        self.assertEqual(first, second)
        self.assertEqual(len(self.recorder.calls), 1, "second ensure must NOT spawn")


class DeadPidRestartTests(EnsureDaemonBase):
    def test_dead_pid_triggers_clean_respawn(self):
        sd = self._state_dir()
        sd.mkdir(parents=True, exist_ok=True)
        # Plant stale state pointing at a PID that does not exist.
        (sd / "proxy.pid").write_text("999999")
        (sd / "proxy.port").write_text("12345")
        (sd / "proxy.config-hash").write_text("stale")
        port = self.mod.ensure_daemon(self.root, self.named_config)
        self.assertEqual(len(self.recorder.calls), 1, "dead PID must trigger spawn")
        self.assertNotEqual(port, 12345, "must allocate a fresh free port")
        self.assertEqual(int((sd / "proxy.pid").read_text()), self.recorder.children[0].pid)


class StaleHealthcheckRestartTests(EnsureDaemonBase):
    def test_pid_alive_but_port_dead_triggers_sigterm_and_respawn(self):
        # First call brings up our fake sleeper as the "daemon".
        first_port = self.mod.ensure_daemon(self.root, self.named_config)
        first_pid = self.recorder.children[0].pid
        self.assertTrue(self.mod._is_pid_alive(first_pid))
        # Now flip healthcheck → False so the next ensure considers the port dead.
        self.mod.healthcheck = lambda port, timeout=2.0: False
        second_port = self.mod.ensure_daemon(self.root, self.named_config)
        self.assertEqual(len(self.recorder.calls), 2, "stale port must trigger respawn")
        # Old process received SIGTERM and exited.
        deadline = time.monotonic() + 3.0
        while time.monotonic() < deadline and self.mod._is_pid_alive(first_pid):
            time.sleep(0.05)
        self.assertFalse(self.mod._is_pid_alive(first_pid), "old pid must be reaped after SIGTERM")
        self.assertNotEqual(first_port, second_port)


class ConfigChangeRestartTests(EnsureDaemonBase):
    def test_config_hash_change_forces_restart_even_when_healthy(self):
        first_port = self.mod.ensure_daemon(self.root, self.named_config)
        first_pid = self.recorder.children[0].pid
        # Mutate the named-config so its hash changes.
        self.named_config.write_text(
            json.dumps({"mcpServers": {"trello": {"command": "echo", "args": ["DIFFERENT"]}}})
        )
        # healthcheck stays True (daemon is "alive") yet ensure must restart.
        second_port = self.mod.ensure_daemon(self.root, self.named_config)
        self.assertEqual(len(self.recorder.calls), 2, "config change must trigger respawn")
        # First child should have been SIGTERMed.
        deadline = time.monotonic() + 3.0
        while time.monotonic() < deadline and self.mod._is_pid_alive(first_pid):
            time.sleep(0.05)
        self.assertFalse(self.mod._is_pid_alive(first_pid))
        self.assertNotEqual(first_port, second_port)


if __name__ == "__main__":
    unittest.main()
