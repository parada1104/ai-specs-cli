"""Behavior goldens: what the compiled CLI actually does to a real project.

Named `test_*.py` so `./tests/run.sh` gates it, and LLM-free by construction: the
sandboxes only run the Go binary, git and the filesystem.
"""
from __future__ import annotations

import json
import unittest

from tests._go_cli import LEGACY_CLI, cli
from tests.behavior.cases import CASES
from tests.behavior.run import GOLDENS, diff, run_case


class BehaviorGoldenTests(unittest.TestCase):
    def test_go_binary_is_available(self) -> None:
        # A silent legacy fallback would report green without measuring.
        self.assertNotEqual(cli(), LEGACY_CLI, "the Go build is unavailable; behavior goldens cannot measure")


def _make(name, factory, mode):
    def test(self: BehaviorGoldenTests) -> None:
        path = GOLDENS / f"{name}.json"
        self.assertTrue(path.exists(), f"golden missing: {path} (regenerate with `python3 tests/behavior/run.py --update`)")
        expected = json.loads(path.read_text(encoding="utf-8"))
        problems = diff(name, expected, run_case(name, factory, mode))
        self.assertEqual([], problems, "\n".join(problems))

    return test


for _name, _factory, _mode in CASES:
    setattr(BehaviorGoldenTests, f"test_{_name.replace('-', '_')}", _make(_name, _factory, _mode))
