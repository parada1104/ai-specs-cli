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
