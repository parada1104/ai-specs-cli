"""Group 3.2 — `healthcheck(port, timeout)` HTTP GET /status."""
from __future__ import annotations

import http.server
import importlib.util
import socket
import sys
import threading
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "lib" / "_internal" / "mcp-daemon.py"


def load_module():
    spec = importlib.util.spec_from_file_location("mcp_daemon", MODULE_PATH)
    mod = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    sys.modules[spec.name] = mod
    spec.loader.exec_module(mod)
    return mod


class _StatusHandlerFactory:
    """Build a quiet BaseHTTPRequestHandler subclass returning a fixed code."""

    @staticmethod
    def with_code(code: int):
        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):  # noqa: N802 (BaseHTTPRequestHandler API)
                self.send_response(code)
                self.send_header("Content-Length", "0")
                self.end_headers()

            def log_message(self, *args, **kwargs):  # silence test noise
                return

        return Handler


def _start_server(handler_cls):
    srv = http.server.HTTPServer(("127.0.0.1", 0), handler_cls)
    port = srv.server_address[1]
    thread = threading.Thread(target=srv.serve_forever, daemon=True)
    thread.start()
    return srv, port, thread


def _free_port_no_listener() -> int:
    # bind then close → the kernel will not have anyone listening immediately.
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return port


class HealthcheckTests(unittest.TestCase):
    def setUp(self):
        self.mod = load_module()

    def test_returns_true_when_server_responds_200(self):
        srv, port, _ = _start_server(_StatusHandlerFactory.with_code(200))
        self.addCleanup(srv.server_close)
        self.addCleanup(srv.shutdown)
        self.assertTrue(self.mod.healthcheck(port, timeout=2.0))

    def test_returns_false_when_nothing_is_listening(self):
        port = _free_port_no_listener()
        self.assertFalse(self.mod.healthcheck(port, timeout=0.5))

    def test_returns_false_when_server_responds_500(self):
        srv, port, _ = _start_server(_StatusHandlerFactory.with_code(500))
        self.addCleanup(srv.server_close)
        self.addCleanup(srv.shutdown)
        self.assertFalse(self.mod.healthcheck(port, timeout=2.0))


if __name__ == "__main__":
    unittest.main()
