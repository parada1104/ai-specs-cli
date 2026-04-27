import importlib.util
import sys
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
DOCTOR_SH = ROOT / "lib" / "doctor.sh"
DOCTOR_PY = ROOT / "lib" / "_internal" / "doctor.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    # Pre-register so dataclasses (Python 3.12+) can resolve cls.__module__ during exec.
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


def toml_value(v):
    """Serialize a Python value to a TOML literal."""
    if v is None:
        return ""
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, int):
        return str(v)
    if isinstance(v, str):
        return f'"{v}"'
    if isinstance(v, list):
        items = ", ".join(toml_value(x) for x in v)
        return f"[{items}]"
    if isinstance(v, dict):
        pairs = ", ".join(f"{toml_value(kk)} = {toml_value(vv)}" for kk, vv in v.items())
        return f"{{{pairs}}}"
    return str(v)


def update_toml_field(path: Path, section: str, key: str, value) -> None:
    """Surgical edits for tests — preserves the rest of ai-specs.toml from init."""
    import re

    text = path.read_text()
    if section == "agents" and key == "enabled":
        rep = f"enabled = {toml_value(value)}"
        new, n = re.subn(
            r"(?m)^enabled\s*=\s*\[.*?\]\s*$",
            rep,
            text,
            count=1,
        )
        if n != 1:
            raise AssertionError("could not patch [agents].enabled in test manifest")
        path.write_text(new)
        return
    if section == "mcp":
        block = f"\n[mcp.{key}]\n"
        for kk, vv in (value or {}).items():
            block += f"{kk} = {toml_value(vv)}\n"
        path.write_text(text.rstrip() + block + "\n")
        return
    raise ValueError(f"unsupported test update: {section}.{key}")


def ai_specs_init(path: Path, agents: list[str] | None = None) -> None:
    subprocess.run([str(CLI), "init", str(path)], check=True, text=True)
    toml_path = path / "ai-specs" / "ai-specs.toml"
    if agents is not None:
        update_toml_field(toml_path, "agents", "enabled", agents)
    else:
        # Tests assume no enabled agents until sync; template may default to a trio.
        update_toml_field(toml_path, "agents", "enabled", [])


class DoctorCommandAvailabilityTests(unittest.TestCase):
    def test_help_lists_doctor(self):
        result = subprocess.run(
            [str(CLI), "help"], capture_output=True, text=True, check=False
        )
        self.assertIn("doctor", result.stdout)
        self.assertIn("diagnose", result.stdout.lower())

    def test_doctor_accepts_target_path(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertIn(target.name, result.stdout)

    def test_doctor_is_read_only(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            before = set(_find_files(target))
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            after = set(_find_files(target))
            self.assertEqual(before, after)


class CoreProjectStructureTests(unittest.TestCase):
    def test_manifest_exists_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertIn("OK", result.stdout)
            self.assertIn("manifest", result.stdout)

    def test_manifest_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("manifest", result.stdout.lower())

    def test_agents_md_exists_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertIn("OK", result.stdout)
            self.assertIn("AGENTS", result.stdout)

    def test_agents_md_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            (target / "ai-specs").mkdir()
            (target / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "orphan"\n'
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("AGENTS", result.stdout)
            self.assertIn("sync", result.stdout.lower())


class AgentDiagnosticsTests(unittest.TestCase):
    def test_no_enabled_agents_reports_warn(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "agents", "enabled", []
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("WARN", result.stdout)
            self.assertIn("enabled", result.stdout.lower())

    def test_unknown_enabled_agent_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "agents", "enabled", ["fakerobot"]
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)

    def test_enabled_agent_output_present_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["claude"])
            subprocess.run(
                [str(CLI), "sync-agent", str(target)],
                check=True,
                text=True,
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)

    def test_enabled_agent_output_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["claude"])
            (target / "AGENTS.md").unlink()
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)


class BundledAssetDiagnosticsTests(unittest.TestCase):
    def test_bundled_skills_present_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertIn("OK", result.stdout)
            self.assertIn("skill-creator", result.stdout)

    def test_bundled_skill_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            shutil.rmtree(target / "ai-specs" / "skills" / "skill-sync")
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("skill-sync", result.stdout)

    def test_bundled_commands_present_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertIn("OK", result.stdout)
            self.assertIn("commands", result.stdout)

    def test_bundled_commands_missing_reports_warn(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            shutil.rmtree(target / "ai-specs" / "commands")
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("WARN", result.stdout)


class SymlinkDiagnosticsTests(unittest.TestCase):
    def test_instruction_symlink_valid_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["claude"])
            subprocess.run(
                [str(CLI), "sync-agent", str(target)],
                check=True,
                text=True,
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)
            self.assertIn("CLAUDE.md", result.stdout)

    def test_instruction_symlink_invalid_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["claude"])
            subprocess.run(
                [str(CLI), "sync-agent", str(target)],
                check=True,
                text=True,
            )
            claude_md = target / "CLAUDE.md"
            if claude_md.is_symlink() or claude_md.exists():
                claude_md.unlink()
            claude_md.write_text("stale content")
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("CLAUDE.md", result.stdout)

    def test_skill_symlink_valid_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["claude"])
            subprocess.run(
                [str(CLI), "sync-agent", str(target)],
                check=True,
                text=True,
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)
            self.assertIn("skills", result.stdout)

    def test_copied_skill_directory_valid_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["opencode"])
            subprocess.run(
                [str(CLI), "sync-agent", str(target)],
                check=True,
                text=True,
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)


class MCPDiagnosticsTests(unittest.TestCase):
    def test_no_mcp_servers_reports_warn(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("WARN", result.stdout)
            self.assertIn("mcp", result.stdout.lower())

    def test_mcp_config_present_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["claude"])
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "mcp", "demo",
                {"command": "npx"}
            )
            subprocess.run(
                [str(CLI), "sync-agent", str(target)],
                check=True, text=True
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)
            self.assertIn("mcp", result.stdout.lower())

    def test_mcp_config_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["claude"])
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "mcp", "demo",
                {"command": "npx"}
            )
            mcp_file = target / ".mcp.json"
            if mcp_file.exists():
                mcp_file.unlink()
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("mcp", result.stdout.lower())


class ReportAndExitCodeTests(unittest.TestCase):
    def test_healthy_project_exits_zero(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)

    def test_project_with_errors_exits_nonzero(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)

    def test_severity_labels_present(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            found = False
            for label in ("OK", "WARN", "ERROR"):
                if label in result.stdout:
                    found = True
                    break
            self.assertTrue(found)
            self.assertIn("Summary", result.stdout)

    def test_non_ok_includes_actionable_guidance(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            if "ERROR" in result.stdout:
                words = result.stdout.lower()
                self.assertTrue(
                    "init" in words or "sync" in words or "missing" in words
                )


def _find_files(root: Path):
    for p in root.rglob("*"):
        if p.is_file():
            yield p


def _append_sdd(toml: Path, *, enabled: bool, store: str) -> None:
    extra = f"""

[sdd]
enabled = {str(enabled).lower()}
provider = "openspec"
artifact_store = "{store}"
"""
    toml.write_text(toml.read_text().rstrip() + extra + "\n")


class SddDoctorChecksTests(unittest.TestCase):
    def _run_doctor_module(self, target: Path):
        doc = load_module(DOCTOR_PY, "doctor_sdd_probe")
        d = doc.Doctor(target)
        code = d.run()
        return d, code

    def test_sdd_disabled_emits_no_sdd_openspec_lines(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            d, _ = self._run_doctor_module(target)
            labels = " ".join(c.name for c in d.checks)
            self.assertNotIn("sdd-openspec", labels)

    @patch.object(shutil, "which", return_value="/fake/openspec")
    def test_sdd_filesystem_missing_tree_errors(self, _mock_which):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            _append_sdd(target / "ai-specs" / "ai-specs.toml", enabled=True, store="filesystem")
            d, code = self._run_doctor_module(target)
            self.assertNotEqual(code, 0)
            self.assertTrue(
                any(c.name == "sdd-openspec-dir" and "ERROR" in c.severity.value for c in d.checks)
            )

    @patch.object(shutil, "which", return_value="/fake/openspec")
    def test_sdd_memory_missing_tree_warns_not_errors(self, _mock_which):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            _append_sdd(target / "ai-specs" / "ai-specs.toml", enabled=True, store="memory")
            d, code = self._run_doctor_module(target)
            self.assertEqual(code, 0)
            warns = [c for c in d.checks if c.name == "sdd-openspec-dir"]
            self.assertTrue(warns and warns[0].severity.value == "WARN")


if __name__ == "__main__":
    unittest.main()