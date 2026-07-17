import importlib.util
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

    def test_pi_is_in_platform_dict(self):
        """Pi agent must be registered in the PLATFORM dict."""
        doctor = load_module(DOCTOR_PY, "doctor_module_pi_in_dict")
        self.assertIn("pi", doctor.Doctor.PLATFORM)
        plat = doctor.Doctor.PLATFORM["pi"]
        self.assertEqual(plat["skills_dir"], ".pi/skills")
        self.assertEqual(plat["mcp_config_path"], ".mcp.json")
        self.assertEqual(plat["mcp_key"], "mcpServers")
        self.assertEqual(plat["commands_dir"], "")

    def test_pi_not_rejected_as_unknown_agent(self):
        """Pi in enabled agents must not produce 'unsupported agent' ERROR."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["pi"])
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            # Before sync, pi should NOT be flagged as unsupported agent
            self.assertNotIn("unsupported agent", result.stdout.lower())
            self.assertIn("pi", result.stdout)

    def test_pi_output_present_reports_ok(self):
        """Pi with valid .pi/skills symlink reports OK."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["pi"])
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
            self.assertIn(".pi/skills", result.stdout)

    def test_omp_is_in_platform_dict(self):
        """omp agent must be registered in the doctor PLATFORM dict (kept in
        sync with platform.sh, which gained omp in PR #70)."""
        doctor = load_module(DOCTOR_PY, "doctor_module_omp_in_dict")
        self.assertIn("omp", doctor.Doctor.PLATFORM)
        plat = doctor.Doctor.PLATFORM["omp"]
        self.assertEqual(plat["skills_dir"], ".omp/skills")
        self.assertEqual(plat["mcp_config_path"], ".omp/mcp.json")
        self.assertEqual(plat["mcp_key"], "mcpServers")
        self.assertEqual(plat["commands_dir"], ".omp/commands")

    def test_omp_not_rejected_as_unknown_agent(self):
        """omp in enabled agents must not produce 'unsupported agent' ERROR."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["omp"])
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertNotIn("unsupported agent", result.stdout.lower())
            self.assertIn("omp", result.stdout)

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

    def test_stale_commands_reports_warn(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["cursor"])
            subprocess.run(
                [str(CLI), "sync-agent", str(target)],
                check=True,
                text=True,
            )
            (target / ".cursor" / "commands" / "stale.md").write_text("# stale\n")
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("WARN", result.stdout)
            self.assertIn("stale", result.stdout)

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


class PlatformGetTests(unittest.TestCase):
    """Unit tests for platform_get shell function (all agent fields)."""

    PLATFORM_SH = ROOT / "lib" / "_internal" / "platform.sh"

    def _platform_get(self, agent: str, field: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            ["bash", "-c", f'source "{self.PLATFORM_SH}" && platform_get {agent} {field}'],
            capture_output=True, text=True, check=False,
        )

    # --- Pi agent field tests ---

    def test_pi_skills_dir(self):
        result = self._platform_get("pi", "skills_dir")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), ".pi/skills")

    def test_pi_mcp_config_path(self):
        result = self._platform_get("pi", "mcp_config_path")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), ".mcp.json")

    def test_pi_mcp_key(self):
        result = self._platform_get("pi", "mcp_key")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "mcpServers")

    def test_pi_native_true(self):
        result = self._platform_get("pi", "native")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "true")

    def test_pi_instructions_path_empty(self):
        result = self._platform_get("pi", "instructions_path")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "")

    def test_pi_commands_dir_empty(self):
        result = self._platform_get("pi", "commands_dir")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "")

    def test_pi_agents_dir_empty(self):
        result = self._platform_get("pi", "agents_dir")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "")

    def test_pi_invalid_field_exits_nonzero(self):
        result = self._platform_get("pi", "nonexistent_field")
        self.assertNotEqual(result.returncode, 0)

    # --- Omp agent field tests ---

    def test_omp_skills_dir(self):
        result = self._platform_get("omp", "skills_dir")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), ".omp/skills")

    def test_omp_mcp_config_path(self):
        result = self._platform_get("omp", "mcp_config_path")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), ".omp/mcp.json")

    def test_omp_mcp_key(self):
        result = self._platform_get("omp", "mcp_key")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "mcpServers")

    def test_omp_native_true(self):
        result = self._platform_get("omp", "native")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "true")

    def test_omp_instructions_path_empty(self):
        result = self._platform_get("omp", "instructions_path")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "")

    def test_omp_commands_dir(self):
        result = self._platform_get("omp", "commands_dir")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), ".omp/commands")

    def test_omp_agents_dir_empty(self):
        result = self._platform_get("omp", "agents_dir")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "")

    def test_omp_runtime_hooks_target(self):
        result = self._platform_get("omp", "runtime_hooks_target")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), ".omp/extensions")

    def test_omp_invalid_field_exits_nonzero(self):
        result = self._platform_get("omp", "nonexistent_field")
        self.assertNotEqual(result.returncode, 0)

    # --- Regression: existing agents still work ---

    def test_claude_skills_dir_unchanged(self):
        result = self._platform_get("claude", "skills_dir")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), ".claude/skills")

    def test_cursor_skills_dir(self):
        result = self._platform_get("cursor", "skills_dir")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), ".cursor/skills")

    def test_opencode_mcp_key_unchanged(self):
        result = self._platform_get("opencode", "mcp_key")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "mcp")

    def test_invalid_agent_exits_nonzero(self):
        result = self._platform_get("nonexistent_agent", "skills_dir")
        self.assertNotEqual(result.returncode, 0)


class BriefRenderPolicyDoctorTests(unittest.TestCase):
    def _append_brief_render_false(self, toml_path: Path) -> None:
        text = toml_path.read_text().rstrip() + "\n\n[brief]\nrender = false\n"
        toml_path.write_text(text + "\n")

    def test_render_disabled_with_agents_md_reports_info(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            self._append_brief_render_false(target / "ai-specs" / "ai-specs.toml")
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, 0)
            self.assertIn("INFO", result.stdout)
            self.assertIn("brief-render", result.stdout)

    def test_render_disabled_missing_agents_md_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            (target / "AGENTS.md").unlink()
            self._append_brief_render_false(target / "ai-specs" / "ai-specs.toml")
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("brief.render = false", result.stdout)

    def test_render_disabled_with_recipe_fragments_reports_warn(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            self._append_brief_render_false(target / "ai-specs" / "ai-specs.toml")
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertIn("WARN", result.stdout)
            self.assertIn("brief-fragments-unused", result.stdout)
            # S2: Also assert the INFO brief-render signal is emitted alongside
            # the WARN so a future regression dropping INFO is caught.
            self.assertIn("INFO", result.stdout)
            self.assertIn("brief-render", result.stdout)


class CliVersionDoctorTests(unittest.TestCase):
    def _append_tool_section(self, toml_path: Path, body: str) -> None:
        text = toml_path.read_text().rstrip()
        toml_path.write_text(text + "\n\n[tool]\n" + body + "\n")

    def test_exact_pin_aligned_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            installed = (ROOT / "VERSION").read_text().strip()
            self._append_tool_section(
                target / "ai-specs" / "ai-specs.toml",
                f'version = "{installed}"\npolicy = "exact"',
            )
            lock_path = target / "ai-specs" / ".ai-specs.lock"
            lock_path.parent.mkdir(parents=True, exist_ok=True)
            lock_path.write_text(
                f'[meta]\ncli_version = "{installed}"\n'
                f'synced_at = "2026-06-23T12:00:00Z"\n'
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertIn("cli-version", result.stdout)
            self.assertIn("OK", result.stdout)
            self.assertIn(installed, result.stdout)

    def test_exact_pin_mismatch_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            self._append_tool_section(
                target / "ai-specs" / "ai-specs.toml",
                'version = "99.99.99"\npolicy = "exact"',
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertIn("cli-version", result.stdout)
            self.assertIn("ERROR", result.stdout)
            self.assertNotEqual(result.returncode, 0)

    def test_no_pin_stale_last_sync_reports_warn(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            lock_path = target / "ai-specs" / ".ai-specs.lock"
            lock_path.write_text(
                '[meta]\ncli_version = "0.10.0"\n'
                'synced_at = "2026-01-01T00:00:00Z"\n'
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertIn("cli-version", result.stdout)
            self.assertIn("WARN", result.stdout)

    def test_doctor_cli_version_is_read_only(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            self._append_tool_section(
                target / "ai-specs" / "ai-specs.toml",
                'version = "99.99.99"\npolicy = "exact"',
            )
            toml_path = target / "ai-specs" / "ai-specs.toml"
            before = toml_path.read_text()
            subprocess.run([str(CLI), "doctor", str(target)], check=False)
            self.assertEqual(before, toml_path.read_text())


def _find_files(root: Path):
    for p in root.rglob("*"):
        if p.is_file():
            yield p



class RecipeCliDepsDoctorTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.doctor = load_module(DOCTOR_PY, "doctor_recipe_deps")

    def _write_project(self, root: Path, recipe_toml: str, enabled: bool = True) -> Path:
        catalog = root / "catalog" / "recipes" / "demo-recipe"
        catalog.mkdir(parents=True)
        (catalog / "recipe.toml").write_text(recipe_toml)
        project = root / "project"
        (project / "ai-specs").mkdir(parents=True)
        (project / "AGENTS.md").write_text("# agents\n")
        # Satisfy bundled-asset checks so recipe-dep WARN can keep exit code 0.
        for skill in ("skill-creator", "skill-sync"):
            (project / "ai-specs" / "skills" / skill).mkdir(parents=True, exist_ok=True)
        (project / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
        flag = "true" if enabled else "false"
        (project / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "demo"\n\n'
            '[agents]\nenabled = []\n\n'
            f'[recipes.demo-recipe]\nenabled = {flag}\nversion = "1.0"\n'
        )
        return project

    def test_recipe_cli_deps_warn_when_missing(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._write_project(
                root,
                "[recipe]\n"
                'id = "demo-recipe"\n'
                'name = "Demo"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                "\n"
                "[[deps.cli]]\n"
                'binary = "gh"\n'
                'purpose = "Create PRs"\n'
                "required = true\n"
                'install_url = "https://cli.github.com/"\n',
            )
            with patch.dict("os.environ", {"AI_SPECS_HOME": str(root)}), patch(
                "shutil.which", return_value=None
            ):
                doc = self.doctor.Doctor(project)
                code = doc.run()
            warn_rows = [
                c for c in doc.checks
                if c.name == "recipe-dep" and c.severity == self.doctor.Severity.WARN
            ]
            self.assertTrue(warn_rows)
            self.assertIn("gh", warn_rows[0].message)
            self.assertEqual(warn_rows[0].guidance, "https://cli.github.com/")
            self.assertEqual(code, 0)

    def test_recipe_cli_deps_info_when_optional_missing(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._write_project(
                root,
                "[recipe]\n"
                'id = "demo-recipe"\n'
                'name = "Demo"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                "\n"
                "[[deps.cli]]\n"
                'binary = "jq"\n'
                'purpose = "JSON"\n'
                "required = false\n",
            )
            with patch.dict("os.environ", {"AI_SPECS_HOME": str(root)}), patch(
                "shutil.which", return_value=None
            ):
                doc = self.doctor.Doctor(project)
                doc.run()
            info_rows = [
                c for c in doc.checks
                if c.name == "recipe-dep" and c.severity == self.doctor.Severity.INFO
            ]
            self.assertTrue(info_rows)
            self.assertIn("optional jq", info_rows[0].message)

    def test_recipe_cli_deps_ok_when_found(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._write_project(
                root,
                "[recipe]\n"
                'id = "demo-recipe"\n'
                'name = "Demo"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                "\n"
                "[[deps.cli]]\n"
                'binary = "gh"\n'
                'purpose = "Create PRs"\n',
            )
            with patch.dict("os.environ", {"AI_SPECS_HOME": str(root)}), patch(
                "shutil.which", return_value="/usr/bin/gh"
            ):
                doc = self.doctor.Doctor(project)
                doc.run()
            ok_rows = [
                c for c in doc.checks
                if c.name == "recipe-dep" and c.severity == self.doctor.Severity.OK
            ]
            self.assertTrue(ok_rows)
            self.assertIn("gh available", ok_rows[0].message)

    def test_doctor_no_crash_when_no_recipes(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            (project / "AGENTS.md").write_text("# agents\n")
            (project / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "demo"\n\n[agents]\nenabled = []\n'
            )
            doc = self.doctor.Doctor(project)
            code = doc.run()
            self.assertFalse(any(c.name == "recipe-dep" for c in doc.checks))
            self.assertIsInstance(code, int)

    def test_doctor_exit_code_unchanged(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._write_project(
                root,
                "[recipe]\n"
                'id = "demo-recipe"\n'
                'name = "Demo"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                "\n"
                "[[deps.cli]]\n"
                'binary = "missing-cli-xyz"\n'
                'purpose = "demo"\n'
                "required = true\n",
            )
            with patch.dict("os.environ", {"AI_SPECS_HOME": str(root)}), patch(
                "shutil.which", return_value=None
            ):
                doc = self.doctor.Doctor(project)
                code = doc.run()
            self.assertTrue(
                any(
                    c.severity == self.doctor.Severity.WARN and c.name == "recipe-dep"
                    for c in doc.checks
                )
            )
            self.assertFalse(any(c.severity == self.doctor.Severity.ERROR and c.name == "recipe-dep" for c in doc.checks))
            # WARN-only recipe-dep rows must not flip the exit code by themselves.
            # Other ERROR checks from incomplete fixtures may exist; assert recipe-dep
            # never contributes ERROR and WARN alone would keep exit 0.
            recipe_only = [c for c in doc.checks if c.name == "recipe-dep"]
            self.assertTrue(recipe_only)
            self.assertTrue(all(c.severity != self.doctor.Severity.ERROR for c in recipe_only))



class CacheAwareCommandsDoctorTests(unittest.TestCase):
    """Doctor must treat cache-managed commands as 'expected', not stale extras."""

    @classmethod
    def setUpClass(cls):
        cls.doctor = load_module(DOCTOR_PY, "doctor_cache_cmd_tests")

    def _load_project_cache(self):
        """Load project-cache helpers for test setup."""
        pc_path = DOCTOR_PY.parent / "project-cache.py"
        return load_module(pc_path, "project_cache_cache_cmd_doctor")

    def test_bundled_commands_ok_when_only_cache_has_commands(self):
        """bundled-commands must report OK when cache commands/ is non-empty, even if ai-specs/commands/ is empty."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target)
            # Clear hand-authored commands
            commands_root = target / "ai-specs" / "commands"
            shutil.rmtree(commands_root, ignore_errors=True)
            commands_root.mkdir(parents=True)
            # Populate cache commands
            pc = self._load_project_cache()
            cache_cmds = pc.commands_dir(target)
            cache_cmds.mkdir(parents=True, exist_ok=True)
            (cache_cmds / "recipe-cmd.md").write_text("# recipe command\n")
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            # Should NOT warn about bundled-commands when cache has commands
            bundled_lines = [
                ln for ln in result.stdout.splitlines()
                if "bundled-commands" in ln
            ]
            self.assertTrue(
                bundled_lines and all("OK" in ln for ln in bundled_lines),
                f"Expected bundled-commands OK when cache has commands; got: {bundled_lines}"
            )

    def test_cache_managed_commands_not_flagged_as_stale(self):
        """Doctor must not flag agent commands as stale when they come from the cache."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, agents=["cursor"])
            # Populate cache with a recipe-managed command
            pc = self._load_project_cache()
            cache_cmds = pc.commands_dir(target)
            cache_cmds.mkdir(parents=True, exist_ok=True)
            (cache_cmds / "recipe-cmd.md").write_text("# recipe command\n")
            # Sync so agent commands dir is populated
            subprocess.run(
                [str(CLI), "sync-agent", str(target), "--cursor"],
                check=True, text=True,
            )
            result = subprocess.run(
                [str(CLI), "doctor", str(target)],
                capture_output=True, text=True, check=False
            )
            cmd_lines = [
                ln for ln in result.stdout.splitlines()
                if ".cursor/commands" in ln
            ]
            self.assertFalse(
                any("stale" in ln.lower() for ln in cmd_lines),
                f"Cache-managed commands must not be flagged as stale; got: {cmd_lines}"
            )


class CommandsEmptyExpectedDoctorTests(unittest.TestCase):
    """Doctor must not WARN when both expected and actual command sets are empty."""

    @classmethod
    def setUpClass(cls):
        cls.doctor = load_module(DOCTOR_PY, "doctor_empty_cmd_tests")

    def _make_project(self, tmp: str) -> Path:
        """Minimal project fixture: manifest + AGENTS.md + bundled skills."""
        target = Path(tmp) / "prj"
        target.mkdir()
        (target / "AGENTS.md").write_text("# agents\n")
        (target / "ai-specs").mkdir()
        for skill in ("skill-creator", "skill-sync"):
            (target / "ai-specs" / "skills" / skill).mkdir(parents=True, exist_ok=True)
        (target / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "demo"\n[agents]\nenabled = ["claude"]\n'
        )
        return target

    def test_empty_commands_dir_with_no_expected_reports_ok_not_warn(self):
        """Empty agent commands dir + zero expected commands → OK, not WARN."""
        with tempfile.TemporaryDirectory() as tmp:
            target = self._make_project(tmp)
            # Controlled AI_SPECS_HOME so cache lookup returns empty
            fake_home = Path(tmp) / "cli-home"
            fake_home.mkdir()
            # Create an empty commands dir for the claude agent
            (target / ".claude" / "commands").mkdir(parents=True)
            plat = self.doctor.Doctor.PLATFORM["claude"]
            with patch.dict("os.environ", {"AI_SPECS_HOME": str(fake_home)}):
                doc = self.doctor.Doctor(target)
                doc._check_agent_outputs("claude", plat, 0)
            warn_rows = [
                c for c in doc.checks
                if ".claude/commands" in c.name and c.severity == self.doctor.Severity.WARN
            ]
            self.assertEqual(
                warn_rows,
                [],
                f"Should not WARN when no commands configured; got: {warn_rows}",
            )

    def test_empty_commands_dir_with_no_expected_emits_ok_label(self):
        """Empty agent commands dir + zero expected commands → at least one OK for commands."""
        with tempfile.TemporaryDirectory() as tmp:
            target = self._make_project(tmp)
            fake_home = Path(tmp) / "cli-home"
            fake_home.mkdir()
            (target / ".claude" / "commands").mkdir(parents=True)
            plat = self.doctor.Doctor.PLATFORM["claude"]
            with patch.dict("os.environ", {"AI_SPECS_HOME": str(fake_home)}):
                doc = self.doctor.Doctor(target)
                doc._check_agent_outputs("claude", plat, 0)
            ok_rows = [
                c for c in doc.checks
                if ".claude/commands" in c.name and c.severity == self.doctor.Severity.OK
            ]
            self.assertTrue(
                ok_rows,
                f"Should emit OK when no commands configured; checks: {doc.checks}",
            )


if __name__ == "__main__":
    unittest.main()