"""Non-live coverage for the eval harness runtime-hook wiring.

Verifies `wire_runtime_hooks` produces a native `PreToolUse` entry for a
hook-capable runtime (claude) and does NOT wire a file-write hook for cursor
(which exposes no pre-file-write event). No agent is invoked.
"""
from __future__ import annotations

import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from tests.evals.lib.harness import materialize_project, wire_runtime_hooks  # noqa: E402
from tests.evals.lib.project_fixture import recipe_version  # noqa: E402


class EvalHookWiringTests(unittest.TestCase):
    def _fixture(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        version = recipe_version(ROOT / "catalog", "plan-build-flow")
        materialize_project(root, "plan-build-flow", version)
        return root

    def test_wires_pretooluse_hook_for_claude(self):
        root = self._fixture()
        wire_runtime_hooks(root, "claude")
        settings = root / ".claude" / "settings.json"
        self.assertTrue(settings.is_file(), ".claude/settings.json must be wired for claude")
        blob = settings.read_text()
        self.assertIn("PreToolUse", blob)
        self.assertIn("plan-build-gate", blob)

    def test_no_file_write_hook_for_cursor(self):
        root = self._fixture()
        wire_runtime_hooks(root, "cursor-agent")
        # cursor has no pre-file-write event → the gate hook must not be wired,
        # and wiring for cursor must not create claude settings.
        self.assertFalse((root / ".claude" / "settings.json").exists())
        cursor_hooks = root / ".cursor" / "hooks.json"
        if cursor_hooks.is_file():
            self.assertNotIn("plan-build-gate", cursor_hooks.read_text())


if __name__ == "__main__":
    unittest.main()
