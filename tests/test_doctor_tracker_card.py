"""Doctor renders the Go `tracker-ledger` finding; it no longer grades links.

The tracker ledger has exactly one authoritative grader — the Go `--ledger`
predicate — and doctor is a JSON-consumption host with no write side effect
(A8/A10/D15). These tests pin:

  * doctor renders the verdict's ``doctor`` finding under the check name
    ``tracker-ledger`` with the A10 severities (INFO unbound, WARN ambiguous /
    declared-not-bound / missing-witness / conflict, ERROR infra);
  * doctor fails the check closed to ERROR when no verified binary resolves;
  * a legacy ``## Tracker`` section never changes the verdict, because doctor
    stopped grading it;
  * the runtime brief and generated agent files gain no static dormancy line.
"""
from __future__ import annotations

import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
DOCTOR_PY = ROOT / "lib" / "_internal" / "doctor.py"

STUB_BINARY = """#!/usr/bin/env bash
# Emits the canned verdict in $STUB_JSON (already a full JSON object).
printf '%s\\n' "${STUB_JSON}"
[ "${STUB_EXIT:-0}" = 2 ] && exit 2
exit 0
"""

STUB_ENV_KEYS = ("WORKTREE_GATE_BIN", "STUB_JSON", "STUB_EXIT")


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


def verdict(*, severity: str, message: str, reason: str = "") -> str:
    return json.dumps({
        "capability": "tracker",
        "active": severity == "OK",
        "checkpoint": "work-start",
        "mode": "warn",
        "decision": "allow",
        "reason": reason,
        "identity": {"common_dir": "", "branch": "", "change": None, "key": ""},
        "item": None,
        "conflict": None,
        "prompt": None,
        "doctor": {"severity": severity, "name": "tracker-ledger", "message": message},
    })


class DoctorTrackerLedgerTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.doctor_mod = load_module(DOCTOR_PY, "doctor_tracker_ledger_under_test")

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self._env: dict = {}

    def _project(
        self,
        *,
        recipe_enabled: bool = True,
        declaration: bool = False,
        init_git: bool = False,
    ) -> Path:
        root = Path(self.tmp.name) / "prj"
        root.mkdir()
        ai = root / "ai-specs"
        ai.mkdir()
        (ai / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = []\n\n"
            "[recipes.trello-mcp-workflow]\n"
            f"enabled = {'true' if recipe_enabled else 'false'}\n"
            "[recipes.trello-mcp-workflow.config]\n"
            'board_id = "69ec097f13e2d38ecd89a557"\n'
        )
        (root / "AGENTS.md").write_text("# agents\n")
        if declaration:
            openspec = root / "openspec"
            openspec.mkdir()
            (openspec / "config.yaml").write_text("tracking: trello\n")
        if init_git:
            subprocess.run(["git", "-C", str(root), "init", "-q"], check=True,
                           capture_output=True, text=True)
        return root

    def _write_witness(self, root: Path, payload: object) -> None:
        ledger_dir = root / ".git" / "ai-specs" / "ledger"
        ledger_dir.mkdir(parents=True, exist_ok=True)
        (ledger_dir / "witness.json").write_text(json.dumps(payload))

    def _stub(self, payload: str, *, exit_code: int = 0) -> Path:
        path = Path(self.tmp.name) / "worktree-gate-stub"
        path.write_text(STUB_BINARY)
        path.chmod(0o755)
        self._env = dict(self._env)
        self._env.update({
            "WORKTREE_GATE_BIN": str(path),
            "STUB_JSON": payload,
            "STUB_EXIT": str(exit_code),
        })
        return path

    def _checks(self, root: Path) -> list:
        doc = self.doctor_mod.Doctor(root)
        clean = {k: v for k, v in os.environ.items() if k not in STUB_ENV_KEYS}
        clean.update(self._env)
        with mock.patch.dict(os.environ, clean, clear=True):
            doc._check_tracker_ledger()
        return [c for c in doc.checks if c.name == "tracker-ledger"]

    def _render(self, root: Path) -> list:
        return self._checks(root)

    # --- severity rendering (one grader: doctor renders the Go finding) ---

    def test_bound_healthy_renders_ok(self):
        root = self._project()
        self._stub(verdict(severity="OK", message=""))
        checks = self._render(root)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0].severity, self.doctor_mod.Severity.OK)

    def test_unbound_renders_info_with_guidance(self):
        root = self._project()
        self._stub(verdict(severity="INFO", reason="unbound",
                           message="no tracker recipe bound; enable one or ignore"))
        checks = self._render(root)
        self.assertEqual(checks[0].severity, self.doctor_mod.Severity.INFO)
        self.assertIn("no tracker recipe bound", checks[0].message)
        self.assertIn("enable one tracker recipe or ignore", checks[0].guidance)

    def test_missing_witness_renders_warn(self):
        root = self._project()
        self._stub(verdict(severity="WARN", reason="witness-missing",
                           message="witness missing; run ai-specs sync"))
        checks = self._render(root)
        self.assertEqual(checks[0].severity, self.doctor_mod.Severity.WARN)
        self.assertIn("ai-specs sync", checks[0].guidance)

    def test_ambiguous_renders_warn_with_binding_guidance(self):
        root = self._project()
        self._stub(verdict(severity="WARN", reason="ambiguous",
                           message="ambiguous tracker binding"))
        checks = self._render(root)
        self.assertEqual(checks[0].severity, self.doctor_mod.Severity.WARN)
        self.assertIn('[[bindings]] capability="tracker"', checks[0].guidance)

    def test_declared_not_bound_renders_warn(self):
        root = self._project(recipe_enabled=False, declaration=True)
        self._stub(verdict(severity="WARN", reason="declared-not-bound",
                           message="tracking is declared but no tracker recipe is bound"))
        checks = self._render(root)
        self.assertEqual(checks[0].severity, self.doctor_mod.Severity.WARN)
        self.assertIn("remove the tracking declaration", checks[0].guidance)

    def test_conflict_renders_warn(self):
        root = self._project()
        self._stub(verdict(severity="WARN", reason="conflict",
                           message="conflict recorded; adjudicate at the next checkpoint"))
        checks = self._render(root)
        self.assertEqual(checks[0].severity, self.doctor_mod.Severity.WARN)
        self.assertIn("adjudicate", checks[0].guidance)

    def test_infra_unevaluable_renders_error(self):
        root = self._project()
        self._stub(verdict(severity="ERROR", reason="store-corrupt",
                           message="ledger store unreadable; run ai-specs sync"))
        checks = self._render(root)
        self.assertEqual(checks[0].severity, self.doctor_mod.Severity.ERROR)

    # --- relevance gate: nothing to report when the ledger is not in play ---

    def test_irrelevant_project_is_silent(self):
        root = self._project(recipe_enabled=False)
        self.assertEqual(self._render(root), [])

    def test_declaration_keeps_the_ledger_visible(self):
        root = self._project(recipe_enabled=False, declaration=True)
        self._stub(verdict(severity="WARN", reason="declared-not-bound",
                           message="tracking is declared but no tracker recipe is bound"))
        self.assertEqual(len(self._render(root)), 1)

    def test_witness_keeps_the_ledger_visible(self):
        root = self._project(recipe_enabled=False, init_git=True)
        self._write_witness(root, {"v": 1, "capability": "tracker", "state": "bound"})
        self._stub(verdict(severity="OK", message=""))
        self.assertEqual(len(self._render(root)), 1)

    # --- infra fail-closed ---

    def test_missing_binary_renders_error(self):
        root = self._project()
        missing = Path(self.tmp.name) / "does-not-exist"
        self._env = {"WORKTREE_GATE_BIN": str(missing)}
        checks = self._render(root)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0].severity, self.doctor_mod.Severity.ERROR)
        self.assertIn("failing open", checks[0].message)

    def test_unparseable_verdict_renders_error(self):
        root = self._project()
        self._stub("not json at all")
        checks = self._render(root)
        self.assertEqual(checks[0].severity, self.doctor_mod.Severity.ERROR)

    # --- legacy compatibility: doctor no longer grades the ## Tracker section ---

    def test_legacy_tracker_link_section_is_not_graded(self):
        root = self._project()
        for slug, body in (
            ("bad", "# proposal\n"),
            ("good", "## Tracker\n\n- **card_id**: `6a622e6ad8dd4cefb8c09b81`\n"),
        ):
            change = root / "openspec" / "changes" / slug
            change.mkdir(parents=True)
            (change / "proposal.md").write_text(body)
        self._stub(verdict(severity="WARN", reason="witness-missing",
                           message="witness missing; run ai-specs sync"))
        checks = self._render(root)
        self.assertEqual(len(checks), 1)
        blob = " ".join(c.message for c in checks)
        self.assertNotIn("## Tracker", blob)
        self.assertNotIn("link section", blob)

    def test_legacy_grader_is_gone(self):
        source = DOCTOR_PY.read_text(encoding="utf-8")
        self.assertNotIn("_check_tracker_card_link", source)
        self.assertNotIn("_load_trello_link", source)
        self.assertNotIn("is_valid_link", source)

    def test_doctor_is_read_only(self):
        root = self._project()
        (root / "openspec" / "changes" / "no-card").mkdir(parents=True)
        (root / "openspec" / "changes" / "no-card" / "proposal.md").write_text("# p\n")
        self._stub(verdict(severity="WARN", reason="witness-missing",
                           message="witness missing; run ai-specs sync"))

        def walk(base: Path):
            return sorted(str(p.relative_to(base)) for p in base.rglob("*") if p.is_file())

        before = walk(root)
        self._render(root)
        self.assertEqual(before, walk(root))

    # --- D15: no static dormancy line in the runtime brief / agent files ---

    def test_runtime_brief_sources_add_no_dormancy_line(self):
        sources = list((ROOT / "catalog" / "recipes").rglob("recipe.toml"))
        sources.append(ROOT / "lib" / "_internal" / "agents-render.py")
        for path in sources:
            with self.subTest(path=path.name):
                self.assertNotIn("tracker-ledger", path.read_text(encoding="utf-8"))
        agents = ROOT / "AGENTS.md"
        if agents.is_file():
            self.assertNotIn("tracker-ledger", agents.read_text(encoding="utf-8"))


if __name__ == "__main__":
    unittest.main()
