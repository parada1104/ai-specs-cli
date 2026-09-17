"""Doctor worktree-gate check tests (Phase 3, task 3.14).

Severity table (design §6.5 / spec "Diagnostics for gate implementation
health"):
  OK    Go binary resolved, version matches stamp, selftest passes
  INFO  gate_impl=bash configured explicitly
  WARN  gate_impl=auto falling back to Bash / version mismatch
  ERROR gate_impl=go with no usable binary (failing open)
  ERROR digest mismatch recorded at last acquisition
"""
from __future__ import annotations

import importlib.util
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
DOCTOR_PY = ROOT / "lib" / "_internal" / "doctor.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class FakeGateBinary:
    """Stand-in for lib/_internal/gate_binary.py (doctor's sibling load)."""

    def __init__(self, root: Path):
        self.root = root
        self._binary = root / "cache" / "worktree-gate"
        self._mismatch = root / "no-mismatch.txt"
        self.version_out = "9.9.9"
        self.selftest_out = None  # None = pass
        self.platform = ("darwin", "arm64")

    def detect_platform(self):
        return self.platform

    def cache_bin_path(self, _home, goos=None, goarch=None):
        return self._binary

    def digest_mismatch_record_path(self, _home):
        return self._mismatch

    def binary_version(self, _path):
        return self.version_out

    def _run_selftest(self, _path):
        return self.selftest_out

    def cache_size(self, _home):
        return 4096


class WorktreeGateDoctorTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.doctor = load_module(DOCTOR_PY, "doctor_worktree_gate_under_test")

    def _project(self, *, gate_impl: str | None = None, enabled: bool = True) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name) / "prj"
        root.mkdir(exist_ok=True)
        ai = root / "ai-specs"
        ai.mkdir()
        cfg = ""
        if gate_impl:
            cfg = f"[recipes.worktree-flow.config]\ngate_impl = '{gate_impl}'\n"
        (ai / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = []\n\n"
            f"[recipes.worktree-flow]\nenabled = {'true' if enabled else 'false'}\n"
            + cfg
        )
        return root

    def _checks(self, root: Path, fake: FakeGateBinary):
        doc = self.doctor.Doctor(root)
        with mock.patch.object(self.doctor, "AI_SPECS_HOME", root), \
             mock.patch.object(doc, "_load_gate_binary", return_value=fake):
            doc._check_worktree_gate()
        return [c for c in doc.checks if c.name == "worktree-gate"]

    def test_recipe_disabled_skips_check(self):
        root = self._project(enabled=False)
        self.assertEqual(self._checks(root, FakeGateBinary(root)), [])

    def test_gate_impl_bash_reports_retired_error(self):
        root = self._project(gate_impl="bash")
        checks = self._checks(root, FakeGateBinary(root))
        errors = [c for c in checks if c.severity == self.doctor.Severity.ERROR]
        self.assertEqual(len(errors), 1)
        self.assertIn("retired", errors[0].message)
        self.assertIn("auto", errors[0].message)
        self.assertIn("go", errors[0].message)
        self.assertIn("sync", errors[0].message)
        blob = " ".join(f"{c.severity} {c.message} {c.guidance}" for c in checks)
        self.assertNotIn("rollback lever", blob)

    def test_stamped_bash_reports_retired_error(self):
        root = self._project(gate_impl="auto")
        launcher = root / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
        launcher.parent.mkdir(parents=True)
        launcher.write_text('stamped_gate_impl="bash"\nstamped_gate_version="9.9.9"\n')
        checks = self._checks(root, FakeGateBinary(root))
        errors = [c for c in checks if c.severity == self.doctor.Severity.ERROR]
        self.assertTrue(errors)
        self.assertIn("retired", errors[0].message)

    def test_leftover_legacy_file_reports_info_with_rm_hint(self):
        root = self._project(gate_impl="auto")
        leftover = (
            root / "ai-specs" / "recipes" / "worktree-flow"
            / "hooks" / "worktree-gate-legacy.sh"
        )
        leftover.parent.mkdir(parents=True)
        leftover.write_text("inert leftover\n")
        fake = FakeGateBinary(root)
        fake._binary.parent.mkdir(parents=True)
        fake._binary.write_bytes(b"bin")
        os.chmod(fake._binary, 0o755)
        launcher = root / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
        launcher.write_text('stamped_gate_version="9.9.9"\n')
        checks = self._checks(root, fake)
        infos = [c for c in checks if c.severity == self.doctor.Severity.INFO]
        self.assertEqual(len(infos), 1)
        self.assertIn("leftover", infos[0].message)
        self.assertIn("rm ai-specs/recipes/worktree-flow/hooks/worktree-gate-legacy.sh",
                      infos[0].guidance)
        self.assertNotIn("stale", infos[0].message.lower())

    def test_go_without_binary_reports_error_failing_open(self):
        root = self._project(gate_impl="go")
        fake = FakeGateBinary(root)
        fake._binary = root / "no" / "binary"  # never created
        checks = self._checks(root, fake)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0].severity, self.doctor.Severity.ERROR)
        self.assertIn("failing open", checks[0].message)
        self.assertIn(str(root / "no" / "binary"), checks[0].message)

    def test_auto_without_binary_reports_error_failing_open(self):
        root = self._project(gate_impl="auto")
        fake = FakeGateBinary(root)
        fake._binary = root / "no" / "binary"
        checks = self._checks(root, fake)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0].severity, self.doctor.Severity.ERROR)
        self.assertIn("failing open", checks[0].message)
        self.assertNotIn("Bash", checks[0].message)
        self.assertNotIn("rollback lever", checks[0].message)

    def test_healthy_binary_reports_ok(self):
        root = self._project()
        launcher = root / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
        launcher.parent.mkdir(parents=True)
        launcher.write_text('stamped_gate_version="9.9.9"\n')
        fake = FakeGateBinary(root)
        fake._binary.parent.mkdir(parents=True)
        fake._binary.write_bytes(b"bin")
        os.chmod(fake._binary, 0o755)
        checks = self._checks(root, fake)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0].severity, self.doctor.Severity.OK)
        self.assertIn("9.9.9", checks[0].message)

    def test_version_mismatch_reports_warn(self):
        root = self._project()
        launcher = root / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
        launcher.parent.mkdir(parents=True)
        launcher.write_text('stamped_gate_version="8.8.8"\n')
        fake = FakeGateBinary(root)
        fake._binary.parent.mkdir(parents=True)
        fake._binary.write_bytes(b"bin")
        os.chmod(fake._binary, 0o755)
        checks = self._checks(root, fake)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0].severity, self.doctor.Severity.WARN)
        self.assertIn("8.8.8", checks[0].message)

    def test_digest_mismatch_record_reports_error(self):
        root = self._project()
        fake = FakeGateBinary(root)
        fake._mismatch.parent.mkdir(parents=True, exist_ok=True)
        fake._mismatch.write_text(
            "worktree-gate: digest mismatch for worktree-gate-darwin-arm64; "
            "artifact deleted and never executed"
        )
        checks = self._checks(root, fake)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0].severity, self.doctor.Severity.ERROR)
        self.assertIn("digest mismatch", checks[0].message)
        self.assertIn("never executed", checks[0].message)


if __name__ == "__main__":
    unittest.main()
