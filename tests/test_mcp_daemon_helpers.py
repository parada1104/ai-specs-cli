"""Group 3.1 — private helpers in `lib/_internal/mcp-daemon.py`.

Covers `_pick_free_port`, `_state_dir`, `_is_pid_alive`, `_hash_config`.
"""
from __future__ import annotations

import importlib.util
import json
import os
import sys
import tempfile
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


class PickFreePortTests(unittest.TestCase):
    def setUp(self):
        self.mod = load_module()

    def test_returns_int_in_ephemeral_range(self):
        port = self.mod._pick_free_port()
        self.assertIsInstance(port, int)
        self.assertGreater(port, 1024)
        self.assertLess(port, 65536)

    def test_consecutive_calls_differ(self):
        # OS-assigned ephemeral ports rarely collide on consecutive binds;
        # accept either path (distinct OR identical-but-unlikely) by retrying.
        seen = {self.mod._pick_free_port() for _ in range(8)}
        self.assertGreater(len(seen), 1, "_pick_free_port should not always return the same port")


class StateDirTests(unittest.TestCase):
    def setUp(self):
        self.mod = load_module()
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)

    def test_returns_dotted_path_and_creates_it(self):
        out = self.mod._state_dir(self.root)
        self.assertEqual(out, self.root / ".ai-specs" / "run")
        self.assertTrue(out.is_dir())

    def test_idempotent_when_already_exists(self):
        first = self.mod._state_dir(self.root)
        (first / "marker").write_text("x")
        second = self.mod._state_dir(self.root)
        self.assertEqual(first, second)
        self.assertEqual((second / "marker").read_text(), "x")


class IsPidAliveTests(unittest.TestCase):
    def setUp(self):
        self.mod = load_module()

    def test_self_pid_is_alive(self):
        self.assertTrue(self.mod._is_pid_alive(os.getpid()))

    def test_nonexistent_pid_is_dead(self):
        # PID 999999 almost certainly does not exist.
        self.assertFalse(self.mod._is_pid_alive(999_999))

    def test_invalid_pid_zero_or_negative_returns_false(self):
        self.assertFalse(self.mod._is_pid_alive(0))
        self.assertFalse(self.mod._is_pid_alive(-1))


class HashConfigTests(unittest.TestCase):
    def setUp(self):
        self.mod = load_module()
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)

    def _write_json(self, name, payload):
        p = self.root / name
        p.write_text(json.dumps(payload))
        return p

    def test_identical_content_yields_identical_hash(self):
        a = self._write_json("a.json", {"mcpServers": {"foo": {"command": "x"}}})
        b = self._write_json("b.json", {"mcpServers": {"foo": {"command": "x"}}})
        self.assertEqual(self.mod._hash_config(a), self.mod._hash_config(b))

    def test_distinct_content_yields_distinct_hash(self):
        a = self._write_json("a.json", {"mcpServers": {"foo": {"command": "x"}}})
        c = self._write_json("c.json", {"mcpServers": {"foo": {"command": "y"}}})
        self.assertNotEqual(self.mod._hash_config(a), self.mod._hash_config(c))

    def test_key_order_does_not_affect_hash(self):
        # canonical JSON sorts keys before hashing
        a = self._write_json("a.json", {"a": 1, "b": 2})
        b = self._write_json("b.json", {"b": 2, "a": 1})
        self.assertEqual(self.mod._hash_config(a), self.mod._hash_config(b))


if __name__ == "__main__":
    unittest.main()
