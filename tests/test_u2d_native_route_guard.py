"""Guard: the U2d launcher-driven modules drive the native Go binary.

Each module below set ``CLI = ROOT / "bin" / "ai-specs"`` and therefore ran the
bash launcher. This guard pins that every listed module resolves its CLI through
``_blackbox.cli_path()`` instead, so a regression to the launcher fails loudly.
"""
from __future__ import annotations

import re
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

ROOT = Path(__file__).resolve().parents[1]
TESTS = ROOT / "tests"

IN_SCOPE_MODULES = (
    "test_agents_md_render_opt_out",
    "test_deps_pinning",
    "test_doctor",
    "test_false_success_fixes",
    "test_hub",
    "test_init_tui",
    "test_jinna_consumer_recipe",
    "test_release_materialization",
    "test_rules_audit",
    "test_runtime_brief_baseline",
    "test_sync_env_scaffold",
    "test_sync_pipeline",
    "test_tracker_ledger_witness",
    "test_worktree_root_propagation",
)

LAUNCHER_CONSTANT = re.compile(r'^CLI\s*=\s*ROOT\s*/\s*"bin"\s*/\s*"ai-specs"\s*$', re.M)


class NativeRouteGuardTests(unittest.TestCase):
    def test_in_scope_modules_do_not_drive_the_launcher(self):
        offenders = [
            name for name in IN_SCOPE_MODULES
            if LAUNCHER_CONSTANT.search((TESTS / f"{name}.py").read_text())
        ]
        self.assertEqual(
            offenders, [],
            "modules still drive bin/ai-specs instead of the native binary: "
            + ", ".join(offenders),
        )

    def test_in_scope_modules_resolve_the_native_binary(self):
        for name in IN_SCOPE_MODULES:
            text = (TESTS / f"{name}.py").read_text()
            with self.subTest(module=name):
                self.assertRegex(text, r"(?m)^CLI = cli_path\(\)\s*$")


if __name__ == "__main__":
    unittest.main()
