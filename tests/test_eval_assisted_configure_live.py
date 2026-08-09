"""Deterministic contract checks for the assisted-configure live client."""
from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from tests.evals.eval_assisted_configure_live import (
    _assert_gate_mode_changed,
    _assert_helper_invocation,
    _assert_path_contract,
    _assert_structured_report,
)


class AssistedConfigureEvalContractTests(unittest.TestCase):
    def test_path_contract_checks_required_and_forbidden_globs(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text("manifest")
            contract = {
                "required_path_globs": ["ai-specs/ai-specs.toml"],
                "forbidden_path_globs": [".worktrees/**"],
            }
            _assert_path_contract(self, root, ["ai-specs/ai-specs.toml"], contract, "apply")
            with self.assertRaises(AssertionError):
                _assert_path_contract(self, root, [".worktrees/topic/file"], contract, "apply")

    def test_gate_mode_assertion_requires_expected_materialized_value(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest = Path(tmp) / "ai-specs" / "ai-specs.toml"
            manifest.parent.mkdir()
            manifest.write_text(
                '[recipes.trello-mcp-workflow.config]\n'
                'board_id = "69ec097f13e2d38ecd89a557"\n'
                'gate_mode = "warn"\n'
            )
            _assert_gate_mode_changed(self, Path(tmp), "warn", "apply")
            with self.assertRaises(AssertionError):
                _assert_gate_mode_changed(self, Path(tmp), "off", "apply")
    def test_apply_oracle_requires_noninteractive_helper_invocation(self):
        transcript = 'run: ai-specs recipe configure trello-mcp-workflow --set board_id=...'
        _assert_helper_invocation(self, transcript, "ai-specs recipe configure trello-mcp-workflow", "apply")
        with self.assertRaises(AssertionError):
            _assert_helper_invocation(self, "I applied the config directly", "ai-specs recipe configure trello-mcp-workflow", "apply")

    def test_apply_oracle_requires_structured_report_fields(self):
        report = '{"report_version": 1, "status": "ok", "applied": {}, "sync": {}, "verify": {}}'
        _assert_structured_report(self, report, ["status", "applied", "sync", "verify"], "apply")
        with self.assertRaises(AssertionError):
            _assert_structured_report(self, '{"status": "ok", "sync": {}}', ["status", "applied", "sync", "verify"], "apply")



if __name__ == "__main__":
    unittest.main()
