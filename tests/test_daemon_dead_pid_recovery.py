"""Group 10.1 / 10.3 — daemon edge cases + ``/status`` metadata enrichment.

Covers:

* Dead-PID respawn: after the OS reaps the daemon process (we kill the real
  child here, not via :func:`stop_daemon`), the next :func:`ensure_daemon`
  call must allocate a fresh PID and port — exercising the actual recovery
  branch in :func:`ensure_daemon` (``_is_pid_alive`` returns False).
* Zero-shared-MCPs regression pin: when materialization writes no
  ``proxy.named-config.json`` the ``.ai-specs/run/`` directory must not be
  created by the daemon path either (delegates to the G2 invariant; we just
  pin it for completeness).
* ``status_daemon`` enrichment (Q2 resolution): when the daemon's
  ``/status`` endpoint is reachable, the returned dict carries the real
  payload's ``api_last_activity`` timestamp and the ``servers`` mapping in
  addition to the existing ``pid``/``port``/``uptime_s`` keys.
"""
from __future__ import annotations

import importlib.util
import json
import socket
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "lib" / "_internal" / "mcp-daemon.py"

SLEEPER_CODE = "import time; time.sleep(120)"


def load_module():
    spec = importlib.util.spec_from_file_location("mcp_daemon", MODULE_PATH)
    mod = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    sys.modules[spec.name] = mod
    spec.loader.exec_module(mod)
    return mod


class _SpawnRecorder:
    """Fake ``_POPEN`` that launches a real long-sleeping Python child."""

    def __init__(self):
        self.calls: list[list[str]] = []
        self.children: list[subprocess.Popen] = []

    def __call__(self, cmd, **kwargs):
        self.calls.append(list(cmd))
        child = subprocess.Popen(
            [sys.executable, "-c", SLEEPER_CODE],
            start_new_session=True,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            close_fds=True,
        )
        self.children.append(child)
        return child


class _DaemonRecoveryBase(unittest.TestCase):
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
        self.mod.healthcheck = lambda port, timeout=2.0: True
        self.addCleanup(self._restore_module)
        self.addCleanup(self._kill_children)

    def _restore_module(self):
        self.mod._POPEN = self._orig_popen
        self.mod.healthcheck = self._orig_healthcheck

    def _kill_children(self):
        for child in self.recorder.children:
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


class DeadPidRecoveryTests(_DaemonRecoveryBase):
    """10.1 bullet 1 — externally killed daemon is respawned on next sync."""

    def test_killed_daemon_is_respawned_on_next_ensure(self):
        first_port = self.mod.ensure_daemon(self.root, self.named_config)
        sd = self._state_dir()
        first_pid = int((sd / "proxy.pid").read_text())
        self.assertEqual(first_pid, self.recorder.children[0].pid)

        # Externally kill the daemon (not via stop_daemon — that would also wipe
        # state files and the test would no longer exercise the recovery path).
        self.recorder.children[0].terminate()
        try:
            self.recorder.children[0].wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.fail("sleeper did not exit after SIGTERM")
        # Sanity: state files still point at the now-dead PID.
        self.assertEqual(int((sd / "proxy.pid").read_text()), first_pid)
        self.assertFalse(self.mod._is_pid_alive(first_pid))

        second_port = self.mod.ensure_daemon(self.root, self.named_config)
        self.assertEqual(len(self.recorder.calls), 2, "dead PID must trigger respawn")
        new_pid = int((sd / "proxy.pid").read_text())
        self.assertNotEqual(new_pid, first_pid)
        self.assertEqual(new_pid, self.recorder.children[1].pid)
        self.assertTrue(self.mod._is_pid_alive(new_pid))
        # New port file matches the freshly allocated port.
        self.assertEqual(int((sd / "proxy.port").read_text()), second_port)


class NoSharedMcpSkipsRunDirTests(unittest.TestCase):
    """10.1 bullet 3 — pin: with no shared MCPs nothing under ``.ai-specs/run``.

    The actual ``materialize_recipes`` invariant is owned by G2
    (``test_recipe_materialize_shared_integration.test_no_shared_mcp_does_not_create_named_config``).
    Here we pin the *daemon-side* corollary: ``status_daemon`` and
    ``stop_daemon`` are safe no-ops when the run directory does not exist.
    """

    def setUp(self):
        self.mod = load_module()
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)

    def test_status_and_stop_are_safe_when_run_dir_absent(self):
        run_dir = self.root / ".ai-specs" / "run"
        self.assertFalse(run_dir.exists())
        self.assertIsNone(self.mod.status_daemon(self.root))
        self.assertFalse(self.mod.stop_daemon(self.root))
        self.assertFalse(run_dir.exists(), "no shared MCPs must not create run dir")


class _FakeStatusServer:
    """Minimal HTTP server that returns the real ``mcp-proxy`` ``/status`` shape."""

    def __init__(self, payload: dict):
        self._payload = payload
        body = json.dumps(payload).encode("utf-8")

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *a, **kw):  # silence test noise
                return

            def do_GET(self):
                if self.path == "/status":
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.send_header("Content-Length", str(len(body)))
                    self.end_headers()
                    self.wfile.write(body)
                else:
                    self.send_response(404)
                    self.end_headers()

        self._server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.port = self._server.server_address[1]
        self._thread = threading.Thread(target=self._server.serve_forever, daemon=True)

    def __enter__(self):
        self._thread.start()
        return self

    def __exit__(self, *exc):
        self._server.shutdown()
        self._server.server_close()
        self._thread.join(timeout=2)


class StatusDaemonExposesProxyMetadataTests(unittest.TestCase):
    """10.3 bullets 8 + 9 — ``status_daemon`` surfaces real ``/status`` metadata."""

    def setUp(self):
        self.mod = load_module()
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.run_dir = self.root / ".ai-specs" / "run"
        self.run_dir.mkdir(parents=True)
        # A real, long-lived sleeper gives status_daemon a live PID to inspect.
        self.sleeper = subprocess.Popen(
            [sys.executable, "-c", SLEEPER_CODE],
            start_new_session=True,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            close_fds=True,
        )
        self.addCleanup(self._kill_sleeper)

    def _kill_sleeper(self):
        try:
            self.sleeper.terminate()
            self.sleeper.wait(timeout=2)
        except Exception:
            try:
                self.sleeper.kill()
            except Exception:
                pass

    def test_status_dict_carries_api_last_activity_and_servers(self):
        payload = {
            "api_last_activity": "2026-05-25T07:07:06.006025+00:00",
            "server_instances": {"trello": "configured", "github": "configured"},
        }
        with _FakeStatusServer(payload) as srv:
            (self.run_dir / "proxy.pid").write_text(str(self.sleeper.pid))
            (self.run_dir / "proxy.port").write_text(str(srv.port))
            info = self.mod.status_daemon(self.root)
        self.assertIsInstance(info, dict)
        self.assertEqual(info["pid"], self.sleeper.pid)
        self.assertEqual(info["port"], srv.port)
        self.assertIn("uptime_s", info)
        # New keys driven by Q2 resolution.
        self.assertEqual(info["api_last_activity"], payload["api_last_activity"])
        self.assertEqual(info["servers"], payload["server_instances"])

    def test_status_dict_omits_metadata_when_endpoint_unreachable(self):
        # Pick a free port, do not bind it — fetch must silently fall back.
        with socket.socket() as s:
            s.bind(("", 0))
            dead_port = s.getsockname()[1]
        (self.run_dir / "proxy.pid").write_text(str(self.sleeper.pid))
        (self.run_dir / "proxy.port").write_text(str(dead_port))
        info = self.mod.status_daemon(self.root)
        self.assertIsInstance(info, dict)
        self.assertEqual(info["pid"], self.sleeper.pid)
        self.assertEqual(info["port"], dead_port)
        self.assertNotIn("api_last_activity", info)
        self.assertNotIn("servers", info)


if __name__ == "__main__":
    unittest.main()
