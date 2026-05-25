"""Group 3.8 — `_acquire_lock` serializes concurrent `ensure_daemon`."""
from __future__ import annotations

import importlib.util
import json
import subprocess
import sys
import tempfile
import threading
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


class SlowSpawnRecorder:
    """Fake Popen that delays so the second caller observes the race."""

    def __init__(self, delay: float = 0.5):
        self.delay = delay
        self.calls = 0
        self.children = []
        self.lock = threading.Lock()

    def __call__(self, cmd, **kwargs):
        with self.lock:
            self.calls += 1
        time.sleep(self.delay)
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


class FileLockTests(unittest.TestCase):
    def setUp(self):
        self.mod = load_module()
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.named_config = self.root / "named-config.json"
        self.named_config.write_text(json.dumps({"mcpServers": {"trello": {"command": "echo"}}}))
        self.recorder = SlowSpawnRecorder(delay=0.5)
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

    def test_two_concurrent_ensures_yield_exactly_one_spawn(self):
        results: list[int] = []
        errors: list[BaseException] = []

        def run():
            try:
                results.append(self.mod.ensure_daemon(self.root, self.named_config))
            except BaseException as exc:  # noqa: BLE001
                errors.append(exc)

        threads = [threading.Thread(target=run) for _ in range(2)]
        for t in threads:
            t.start()
        for t in threads:
            t.join(timeout=10)

        self.assertEqual(errors, [], f"ensure_daemon raised: {errors}")
        self.assertEqual(len(results), 2)
        self.assertEqual(results[0], results[1], "both threads must return the same port")
        self.assertEqual(self.recorder.calls, 1, "lock must serialize → exactly one spawn")


if __name__ == "__main__":
    unittest.main()
