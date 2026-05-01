"""Tests for SDD / OpenSpec integration (sdd.py, refresh preset)."""
from __future__ import annotations

import importlib.util
import shutil
import subprocess
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
SDD_PY = ROOT / "lib" / "_internal" / "sdd.py"
REFRESH_PY = ROOT / "lib" / "_internal" / "refresh-bundled.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


def append_sdd(toml: Path, *, enabled: bool, store: str) -> None:
    extra = f"""

[sdd]
enabled = {str(enabled).lower()}
provider = "openspec"
artifact_store = "{store}"
"""
    toml.write_text(toml.read_text().rstrip() + extra + "\n")


class SddCliHelpTests(unittest.TestCase):
    def test_sdd_help_exits_zero(self):
        r = subprocess.run(
            [str(CLI), "sdd", "--help"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(r.returncode, 0)
        self.assertIn("openspec", r.stdout.lower())
        self.assertIn("README", r.stdout)

    def test_main_help_lists_sdd(self):
        r = subprocess.run([str(CLI), "help"], capture_output=True, text=True, check=False)
        self.assertEqual(r.returncode, 0)
        self.assertIn("sdd", r.stdout)


class SddPythonUnitTests(unittest.TestCase):
    def test_validate_sdd_dict_rejects_unknown_key(self):
        sdd = load_module(SDD_PY, "sdd_u")
        errs = sdd.validate_sdd_dict(
            {"enabled": True, "provider": "openspec", "artifact_store": "hybrid", "extra": 1}
        )
        self.assertTrue(any("unknown" in e for e in errs))

    def test_replace_sdd_block_roundtrip(self):
        sdd = load_module(SDD_PY, "sdd_r")
        base = '[project]\nname = "p"\n\n[agents]\nenabled = []\n'
        merged = sdd.replace_sdd_block(
            base,
            ['enabled = true', 'provider = "openspec"', 'artifact_store = "filesystem"'],
        )
        self.assertIn("[sdd]", merged)
        self.assertIn("filesystem", merged)
        self.assertIn("[agents]", merged)
        again = sdd.replace_sdd_block(
            merged,
            ['enabled = false', 'provider = "openspec"', 'artifact_store = "hybrid"'],
        )
        self.assertIn("enabled = false", again)
        self.assertEqual(again.count("[sdd]"), 1)


class DecisionMatrixTests(unittest.TestCase):
    def setUp(self):
        self.sdd = load_module(SDD_PY, "sdd_dm")

    def test_load_decision_matrix_returns_none_when_missing(self):
        import tempfile

        with tempfile.TemporaryDirectory() as tmp:
            cfg = Path(tmp) / "config.yaml"
            cfg.write_text("schema: spec-driven\n")
            result = self.sdd.load_decision_matrix(cfg)
            self.assertIsNone(result)

    def test_load_decision_matrix_returns_none_when_mode_formal(self):
        import tempfile

        with tempfile.TemporaryDirectory() as tmp:
            cfg = Path(tmp) / "config.yaml"
            cfg.write_text(
                "schema: spec-driven\n"
                "sdd:\n"
                "  mode: formal\n"
                "  decision_matrix:\n"
                "    trivial:\n"
                "      artifacts: []\n"
                "      worktree_required: false\n"
                "      proposal_required: false\n"
                "      design_required: false\n"
            )
            result = self.sdd.load_decision_matrix(cfg)
            self.assertIsNone(result)

    def test_load_decision_matrix_returns_dict_when_present(self):
        import tempfile

        with tempfile.TemporaryDirectory() as tmp:
            cfg = Path(tmp) / "config.yaml"
            cfg.write_text(
                "schema: spec-driven\n"
                "sdd:\n"
                "  mode: adaptive\n"
                "  decision_matrix:\n"
                "    trivial:\n"
                "      artifacts: []\n"
                "      worktree_required: false\n"
                "      proposal_required: false\n"
                "      design_required: false\n"
                "    local_fix:\n"
                "      artifacts: []\n"
                "      worktree_required: false\n"
                "      proposal_required: false\n"
                "      design_required: false\n"
                "    behavior_change:\n"
                "      artifacts: [\"tasks.md\"]\n"
                "      worktree_required: true\n"
                "      proposal_required: false\n"
                "      design_required: false\n"
                "    domain_change:\n"
                "      artifacts: [\"proposal.md\", \"design.md\", \"tasks.md\"]\n"
                "      worktree_required: true\n"
                "      proposal_required: true\n"
                "      design_required: true\n"
            )
            result = self.sdd.load_decision_matrix(cfg)
            self.assertIsInstance(result, dict)
            self.assertIn("trivial", result)
            self.assertIn("domain_change", result)
            self.assertEqual(result["behavior_change"]["artifacts"], ["tasks.md"])
            self.assertTrue(result["domain_change"]["worktree_required"])

    def test_validate_decision_matrix_missing_level(self):
        matrix = {
            "trivial": {"artifacts": [], "worktree_required": False, "proposal_required": False, "design_required": False},
            "local_fix": {"artifacts": [], "worktree_required": False, "proposal_required": False, "design_required": False},
            "behavior_change": {"artifacts": ["tasks.md"], "worktree_required": True, "proposal_required": False, "design_required": False},
            # missing domain_change
        }
        errs = self.sdd.validate_decision_matrix(matrix)
        self.assertTrue(any("missing level: domain_change" in e for e in errs))

    def test_validate_decision_matrix_extra_level(self):
        matrix = {
            "trivial": {"artifacts": [], "worktree_required": False, "proposal_required": False, "design_required": False},
            "local_fix": {"artifacts": [], "worktree_required": False, "proposal_required": False, "design_required": False},
            "behavior_change": {"artifacts": ["tasks.md"], "worktree_required": True, "proposal_required": False, "design_required": False},
            "domain_change": {"artifacts": ["proposal.md", "design.md", "tasks.md"], "worktree_required": True, "proposal_required": True, "design_required": True},
            "extra_level": {"artifacts": [], "worktree_required": False, "proposal_required": False, "design_required": False},
        }
        errs = self.sdd.validate_decision_matrix(matrix)
        self.assertTrue(any("unknown level: extra_level" in e for e in errs))

    def test_validate_decision_matrix_wrong_type_artifacts(self):
        matrix = {
            "trivial": {"artifacts": "not-a-list", "worktree_required": False, "proposal_required": False, "design_required": False},
            "local_fix": {"artifacts": [], "worktree_required": False, "proposal_required": False, "design_required": False},
            "behavior_change": {"artifacts": ["tasks.md"], "worktree_required": True, "proposal_required": False, "design_required": False},
            "domain_change": {"artifacts": ["proposal.md", "design.md", "tasks.md"], "worktree_required": True, "proposal_required": True, "design_required": True},
        }
        errs = self.sdd.validate_decision_matrix(matrix)
        self.assertTrue(any("'artifacts' must be a list" in e for e in errs))

    def test_validate_decision_matrix_wrong_type_flags(self):
        matrix = {
            "trivial": {"artifacts": [], "worktree_required": "false", "proposal_required": False, "design_required": False},
            "local_fix": {"artifacts": [], "worktree_required": False, "proposal_required": False, "design_required": False},
            "behavior_change": {"artifacts": ["tasks.md"], "worktree_required": True, "proposal_required": False, "design_required": False},
            "domain_change": {"artifacts": ["proposal.md", "design.md", "tasks.md"], "worktree_required": True, "proposal_required": True, "design_required": True},
        }
        errs = self.sdd.validate_decision_matrix(matrix)
        self.assertTrue(any("'worktree_required' must be a boolean" in e for e in errs))

    def test_validate_decision_matrix_valid(self):
        matrix = {
            "trivial": {"artifacts": [], "worktree_required": False, "proposal_required": False, "design_required": False},
            "local_fix": {"artifacts": [], "worktree_required": False, "proposal_required": False, "design_required": False},
            "behavior_change": {"artifacts": ["tasks.md"], "worktree_required": True, "proposal_required": False, "design_required": False},
            "domain_change": {"artifacts": ["proposal.md", "design.md", "tasks.md"], "worktree_required": True, "proposal_required": True, "design_required": True},
        }
        errs = self.sdd.validate_decision_matrix(matrix)
        self.assertEqual(errs, [])


class RefreshBundledPresetTests(unittest.TestCase):
    def test_preset_openspec_installs_catalog_skills(self):
        import tempfile

        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True)
            r = subprocess.run(
                [
                    "python3",
                    str(REFRESH_PY),
                    str(target),
                    str(ROOT),
                    "--preset",
                    "openspec",
                    "--init",
                ],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(r.returncode, 0, msg=r.stderr + r.stdout)
            self.assertTrue((target / "ai-specs" / "skills" / "openspec-sdd-conventions").is_dir())
            self.assertTrue((target / "ai-specs" / "skills" / "testing-foundation").is_dir())


@unittest.skipUnless(shutil.which("openspec"), "openspec not on PATH (optional integration)")
class SddEnableIntegrationTests(unittest.TestCase):
    def test_sdd_enable_dry_run(self):
        import tempfile

        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True)
            r = subprocess.run(
                [str(CLI), "sdd", "enable", str(target), "--dry-run"],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(r.returncode, 0, msg=r.stderr + r.stdout)


if __name__ == "__main__":
    unittest.main()
