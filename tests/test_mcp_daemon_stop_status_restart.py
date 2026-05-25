"""Groups 3.9 / 3.10 / 3.11 — stop_daemon, status_daemon, restart_daemon."""
from __future__ import annotations

import importlib.util
import json
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "lib" / "_internal" / "mcp-daemon.py"

SLEEPER = "import time; time.sleep(120)"


def load_module():
    spec = importlib.util.spec_from_file_location("mcp_daemon", MODULE_PATH)
    mod = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    sys.modules[spec.name] = mod
    spec.loader.exec_module(mod)
    return mod


class _Recorder:
    def __init__(self):
        self.calls = 0
        self.children: list[subprocess.Popen] = []

    def __call__(self, cmd, **kwargs):
        self.calls += 1
        child = subprocess.Popen(
            [sys.executable, "-c", SLEEPER],
            start_new_session=True,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            close_fds=True,
        )
        self.children.append(child)
        return child


class _DaemonTestBase(unittest.TestCase):
    def setUp(self):
        self.mod = load_module()
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.named_config = self.root / "named-config.json"
        self.named_config.write_text(json.dumps({"mcpServers": {"trello": {"command": "echo"}}}))
        self.recorder = _Recorder()
        self._orig_popen = self.mod._POPEN
        self._orig_hc = self.mod.healthcheck
        self.mod._POPEN = self.recorder
        self.mod.healthcheck = lambda port, timeout=2.0: True
        self.addCleanup(self._restore)
        self.addCleanup(self._kill_children)

    def _restore(self):
        self.mod._POPEN = self._orig_popen
        self.mod.healthcheck = self._orig_hc

    def _kill_children(self):
        for c in self.recorder.children:
            try:
                c.terminate()
                c.wait(timeout=2)
            except Exception:
                try:
                    c.kill()
                except Exception:
                    pass

    def _state_dir(self) -> Path:
        return self.root / ".ai-specs" / "run"


class StopDaemonTests(_DaemonTestBase):
    def test_active_daemon_sigterm_and_state_cleanup(self):
        port = self.mod.ensure_daemon(self.root, self.named_config)
        pid = self.recorder.children[0].pid
        # Plant a fake proxy.named-config.json too — stop_daemon must clean it.
        sd = self._state_dir()
        (sd / "proxy.named-config.json").write_text("{}")
        self.assertTrue(self.mod._is_pid_alive(pid))
        stopped = self.mod.stop_daemon(self.root)
        self.assertTrue(stopped)
        try:
            self.recorder.children[0].wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.fail("sleeper did not exit after SIGTERM")
        self.assertFalse(self.mod._is_pid_alive(pid))
        for name in ("proxy.pid", "proxy.port", "proxy.named-config.json", "proxy.config-hash"):
            self.assertFalse((sd / name).exists(), f"{name} must be cleaned")
        self.assertEqual(port, port)  # port var is referenced

    def test_no_daemon_returns_false_without_error(self):
        self.assertFalse(self.mod.stop_daemon(self.root))

    def test_stale_pid_files_are_cleaned_and_returns_false(self):
        sd = self._state_dir()
        sd.mkdir(parents=True, exist_ok=True)
        (sd / "proxy.pid").write_text("999999")
        (sd / "proxy.port").write_text("12345")
        result = self.mod.stop_daemon(self.root)
        self.assertFalse(result)
        self.assertFalse((sd / "proxy.pid").exists())
        self.assertFalse((sd / "proxy.port").exists())


class StatusDaemonTests(_DaemonTestBase):
    def test_alive_returns_dict_with_pid_and_port(self):
        port = self.mod.ensure_daemon(self.root, self.named_config)
        info = self.mod.status_daemon(self.root)
        self.assertIsInstance(info, dict)
        self.assertEqual(info["port"], port)
        self.assertEqual(info["pid"], self.recorder.children[0].pid)
        self.assertIn("uptime_s", info)
        self.assertGreaterEqual(info["uptime_s"], 0)

    def test_no_state_returns_none(self):
        self.assertIsNone(self.mod.status_daemon(self.root))

    def test_dead_pid_returns_none(self):
        sd = self._state_dir()
        sd.mkdir(parents=True, exist_ok=True)
        (sd / "proxy.pid").write_text("999999")
        (sd / "proxy.port").write_text("12345")
        self.assertIsNone(self.mod.status_daemon(self.root))


class RestartDaemonTests(_DaemonTestBase):
    def test_restart_replaces_running_daemon(self):
        first_port = self.mod.ensure_daemon(self.root, self.named_config)
        first_pid = self.recorder.children[0].pid
        new_port = self.mod.restart_daemon(self.root, self.named_config)
        self.assertEqual(len(self.recorder.children), 2)
        new_pid = self.recorder.children[1].pid
        self.assertNotEqual(first_pid, new_pid)
        # First child should be gone after SIGTERM in stop_daemon; reap the zombie.
        try:
            self.recorder.children[0].wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.fail("first sleeper did not exit after SIGTERM")
        self.assertFalse(self.mod._is_pid_alive(first_pid))
        self.assertTrue(self.mod._is_pid_alive(new_pid))
        sd = self._state_dir()
        self.assertEqual(int((sd / "proxy.pid").read_text()), new_pid)
        self.assertEqual(int((sd / "proxy.port").read_text()), new_port)


if __name__ == "__main__":
    unittest.main()
