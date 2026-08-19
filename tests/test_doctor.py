import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from _blackbox import invoke, isolated_home, cache_project_dir
from _fixture_catalog import populate_catalog

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"


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


def ai_specs_init(path: Path, agents: list[str] | None = None, cli_home: Path | None = None) -> None:
    if cli_home is not None:
        result = invoke(path, "init", cli_home=cli_home)
    else:
        result = invoke(path, "init")
    assert result.returncode == 0, f"init failed: {result.stderr}"
    toml_path = path / "ai-specs" / "ai-specs.toml"
    if agents is not None:
        update_toml_field(toml_path, "agents", "enabled", agents)
    else:
        # Tests assume no enabled agents until sync; template may default to a trio.
        update_toml_field(toml_path, "agents", "enabled", [])


class DoctorCommandAvailabilityTests(unittest.TestCase):
    def _cli_home(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        home = isolated_home(Path(td.name))
        return home

    def test_help_lists_doctor(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            result = invoke(target, "help", cli_home=self._cli_home())
            self.assertIn("doctor", result.stdout)
            self.assertIn("diagnose", result.stdout.lower())

    def test_doctor_accepts_target_path(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        result = invoke(target, "doctor", cli_home=home)
        # normalize_output maps the temp path to <TEMP>; assert on the printed
        # doctor banner + summary rather than the raw absolute path.
        self.assertIn("ai-specs doctor", result.stdout)
        self.assertIn("Summary:", result.stdout)
        self.assertIn("manifest", result.stdout)

    def test_doctor_is_read_only(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        before = set(_find_files(target))
        result = invoke(target, "doctor", cli_home=home)
        after = set(_find_files(target))
        self.assertEqual(before, after)


class CoreProjectStructureTests(unittest.TestCase):
    def _cli_home(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        return isolated_home(Path(td.name))

    def _doctor(self, target, home):
        invoke(target, "refresh-bundled", cli_home=home)
        return invoke(target, "doctor", cli_home=home)

    def test_manifest_exists_reports_ok(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        result = self._doctor(target, home)
        self.assertIn("OK", result.stdout)
        self.assertIn("manifest", result.stdout)

    def test_manifest_missing_reports_error(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        result = invoke(target, "doctor", cli_home=home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("ERROR", result.stdout)
        self.assertIn("manifest", result.stdout.lower())

    def test_agents_md_exists_reports_ok(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        result = self._doctor(target, home)
        self.assertIn("OK", result.stdout)
        self.assertIn("AGENTS", result.stdout)

    def test_agents_md_missing_reports_error(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        (target / "ai-specs").mkdir()
        (target / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "orphan"\n'
        )
        home = self._cli_home()
        result = invoke(target, "doctor", cli_home=home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("ERROR", result.stdout)
class AgentDiagnosticsTests(unittest.TestCase):
    def _cli_home(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        return isolated_home(Path(td.name))

    def _doctor(self, target, home):
        invoke(target, "refresh-bundled", cli_home=home)
        return invoke(target, "doctor", cli_home=home)

    def test_no_enabled_agents_reports_warn(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        update_toml_field(
            target / "ai-specs" / "ai-specs.toml",
            "agents", "enabled", []
        )
        result = self._doctor(target, home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("WARN", result.stdout)
        self.assertIn("enabled", result.stdout.lower())

    def test_unknown_enabled_agent_reports_error(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        update_toml_field(
            target / "ai-specs" / "ai-specs.toml",
            "agents", "enabled", ["fakerobot"]
        )
        result = self._doctor(target, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("ERROR", result.stdout)

    def test_enabled_agent_output_present_reports_ok(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, agents=["claude"], cli_home=home)
        sync = invoke(target, "sync-agent", cli_home=home)
        self.assertEqual(sync.returncode, 0)
        result = self._doctor(target, home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("OK", result.stdout)

    def test_pi_is_in_platform_dict(self):
        """Pi agent platform registration, observed via sync-agent + doctor:
        skills_dir .pi/skills, mcp_config_path .mcp.json, mcp_key mcpServers,
        and no commands_dir (pi has no native slash-command fan-out)."""
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, agents=["pi"], cli_home=home)
        update_toml_field(
            target / "ai-specs" / "ai-specs.toml",
            "mcp", "demo", {"command": "npx"}
        )
        sync = invoke(target, "sync-agent", cli_home=home)
        self.assertEqual(sync.returncode, 0)
        # skills_dir == ".pi/skills"
        self.assertIn(".pi/skills", self._doctor(target, home).stdout)
        # mcp_config_path == ".mcp.json" surfaces in the mcp-pi check message
        doctor = invoke(target, "doctor", cli_home=home)
        self.assertIn("mcp-pi", doctor.stdout)
        self.assertIn(".mcp.json present", doctor.stdout)
        # mcp_key == "mcpServers" — top-level JSON key of the emitted config
        mcp_json = target / ".mcp.json"
        self.assertTrue(mcp_json.is_file())
        self.assertIn('"mcpServers"', mcp_json.read_text())
        # commands_dir == "" — pi fan-out creates no .pi/commands output
        self.assertNotIn(".pi/commands", doctor.stdout)
        self.assertFalse((target / ".pi" / "commands").exists())

    def test_pi_not_rejected_as_unknown_agent(self):
        """Pi in enabled agents must not produce 'unsupported agent' ERROR."""
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, agents=["pi"], cli_home=home)
        result = self._doctor(target, home)
        # Before sync, pi should NOT be flagged as unsupported agent
        self.assertNotIn("unsupported agent", result.stdout.lower())
        self.assertIn("pi", result.stdout)

    def test_pi_output_present_reports_ok(self):
        """Pi with valid .pi/skills symlink reports OK."""
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, agents=["pi"], cli_home=home)
        sync = invoke(target, "sync-agent", cli_home=home)
        self.assertEqual(sync.returncode, 0)
        result = self._doctor(target, home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("OK", result.stdout)
        self.assertIn(".pi/skills", result.stdout)

    def test_omp_is_in_platform_dict(self):
        """omp agent platform values observed via sync-agent + doctor:
        skills_dir .omp/skills, instructions_path .omp/AGENTS.md,
        mcp_config_path .omp/mcp.json, mcp_key mcpServers, and commands_dir
        .omp/commands (the PR #70 delta vs pi)."""
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, agents=["omp"], cli_home=home)
        update_toml_field(
            target / "ai-specs" / "ai-specs.toml",
            "mcp", "demo", {"command": "npx"}
        )
        sync = invoke(target, "sync-agent", cli_home=home)
        self.assertEqual(sync.returncode, 0)
        result = invoke(target, "doctor", cli_home=home)
        # skills_dir == ".omp/skills"
        self.assertIn(".omp/skills", result.stdout)
        # instructions_path == ".omp/AGENTS.md" — valid symlink check
        self.assertIn(".omp/AGENTS.md", result.stdout)
        self.assertTrue((target / ".omp" / "AGENTS.md").is_symlink())
        # mcp_config_path == ".omp/mcp.json" + mcp_key mcpServers
        self.assertIn("mcp-omp", result.stdout)
        self.assertIn(".omp/mcp.json present", result.stdout)
        mcp_json = target / ".omp" / "mcp.json"
        self.assertTrue(mcp_json.is_file())
        self.assertIn('"mcpServers"', mcp_json.read_text())
        # commands_dir == ".omp/commands" — bundled commands fan out there
        self.assertIn(".omp/commands", result.stdout)
        self.assertTrue((target / ".omp" / "commands").is_dir())

    def test_omp_not_rejected_as_unknown_agent(self):
        """omp in enabled agents must not produce 'unsupported agent' ERROR."""
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, agents=["omp"], cli_home=home)
        result = self._doctor(target, home)
        self.assertNotIn("unsupported agent", result.stdout.lower())
        self.assertIn("omp", result.stdout)

    def test_enabled_agent_output_missing_reports_error(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, agents=["claude"], cli_home=home)
        (target / "AGENTS.md").unlink()
        result = self._doctor(target, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("ERROR", result.stdout)


class BundledAssetDiagnosticsTests(unittest.TestCase):
    def _cli_home(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        return isolated_home(Path(td.name))

    def _doctor(self, target, home):
        invoke(target, "refresh-bundled", cli_home=home)
        return invoke(target, "doctor", cli_home=home)

    def test_bundled_skills_present_reports_ok(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        result = self._doctor(target, home)
        self.assertIn("OK", result.stdout)
        self.assertIn("skill-creator", result.stdout)

    def test_bundled_skill_missing_reports_error(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        # Flatten first so the shared home's cache has the bundled tree, then
        # delete one skill; doctor must NOT re-refresh in between or it would
        # restore the entry.
        invoke(target, "refresh-bundled", cli_home=home)
        bundled = cache_project_dir(target, home) / ".bundled" / "skills" / "skill-sync"
        self.assertTrue(bundled.is_dir(), f"cache bundled skill missing: {bundled}")
        shutil.rmtree(bundled)
        result = invoke(target, "doctor", cli_home=home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("ERROR", result.stdout)
        self.assertIn("skill-sync", result.stdout)

    def test_tracked_bundled_leftover_warns_without_git_rm(self):
        """Doctor WARNs when git still tracks a removed CLI-bundled skill path."""
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        subprocess.run(["git", "init", "-q", str(target)], check=True)
        subprocess.run(
            ["git", "-C", str(target), "config", "user.email", "t@example.com"],
            check=True,
        )
        subprocess.run(
            ["git", "-C", str(target), "config", "user.name", "t"],
            check=True,
        )
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        skill = target / "ai-specs" / "skills" / "skill-creator"
        skill.mkdir(parents=True)
        (skill / "SKILL.md").write_text("# leftover\n")
        subprocess.run(["git", "-C", str(target), "add", "-A"], check=True)
        subprocess.run(
            ["git", "-C", str(target), "commit", "-qm", "track bundled leftover"],
            check=True,
        )
        shutil.rmtree(skill)
        before = subprocess.run(
            ["git", "-C", str(target), "ls-files", "ai-specs/skills/skill-creator"],
            capture_output=True, text=True, check=True,
        ).stdout
        self.assertIn("SKILL.md", before)
        result = self._doctor(target, home)
        self.assertIn("WARN", result.stdout)
        self.assertIn("tracked-bundled", result.stdout)
        self.assertIn("git rm -r --cached", result.stdout)
        self.assertIn("skill-creator", result.stdout)
        after = subprocess.run(
            ["git", "-C", str(target), "ls-files", "ai-specs/skills/skill-creator"],
            capture_output=True, text=True, check=True,
        ).stdout
        self.assertEqual(before, after)

    def test_bundled_commands_present_reports_ok(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        result = self._doctor(target, home)
        self.assertIn("OK", result.stdout)
        self.assertIn("commands", result.stdout)

    def test_bundled_command_present_reports_ok_by_name(self):
        """Per-bundled-command-id OK check names each bundled command."""
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        result = self._doctor(target, home)
        self.assertIn("OK", result.stdout)
        self.assertIn("rules-audit", result.stdout)
        self.assertIn("skills-as-rules", result.stdout)

    def test_bundled_command_missing_reports_error(self):
        """A bundled command id missing from {cache}/.bundled/commands/ is the
        ERROR signal now (mirrors the per-bundled-skill check); an empty
        hand-authored ai-specs/commands/ is unrelated and healthy."""
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        # Flatten first so doctor sees the intact bundled tree, then unlink one
        # command id and run doctor without re-refreshing.
        invoke(target, "refresh-bundled", cli_home=home)
        bundled_cmd = cache_project_dir(target, home) / ".bundled" / "commands" / "rules-audit.md"
        self.assertTrue(bundled_cmd.is_file(), f"cache bundled command missing: {bundled_cmd}")
        bundled_cmd.unlink()
        result = invoke(target, "doctor", cli_home=home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("ERROR", result.stdout)
        self.assertIn("rules-audit", result.stdout)
        self.assertIn("sync", result.stdout)

    def test_empty_ai_specs_commands_dir_is_healthy(self):
        """An empty hand-authored ai-specs/commands/ is healthy (bundled
        commands resolve from the cache, never from the project surface) —
        the old aggregate 'any command present' WARN no longer applies."""
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        shutil.rmtree(target / "ai-specs" / "commands")
        result = self._doctor(target, home)
        self.assertEqual(result.returncode, 0)
        command_lines = [
            ln for ln in result.stdout.splitlines()
            if "bundled-command" in ln or "ai-specs/commands" in ln
        ]
        self.assertFalse(
            any("WARN" in ln or "ERROR" in ln for ln in command_lines),
            f"empty ai-specs/commands/ must not be flagged; got: {command_lines}",
        )

    def test_tracked_bundled_command_leftover_warns_without_git_rm(self):
        """Doctor WARNs when git still tracks a removed CLI-bundled command path."""
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        subprocess.run(["git", "init", "-q", str(target)], check=True)
        subprocess.run(
            ["git", "-C", str(target), "config", "user.email", "t@example.com"],
            check=True,
        )
        subprocess.run(
            ["git", "-C", str(target), "config", "user.name", "t"],
            check=True,
        )
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        leftover = target / "ai-specs" / "commands" / "rules-audit.md"
        leftover.write_text("# leftover\n")
        subprocess.run(["git", "-C", str(target), "add", "-A"], check=True)
        subprocess.run(
            ["git", "-C", str(target), "commit", "-qm", "track bundled command leftover"],
            check=True,
        )
        leftover.unlink()
        before = subprocess.run(
            ["git", "-C", str(target), "ls-files", "ai-specs/commands/rules-audit.md"],
            capture_output=True, text=True, check=True,
        ).stdout
        self.assertIn("rules-audit.md", before)
        result = self._doctor(target, home)
        self.assertIn("WARN", result.stdout)
        self.assertIn("tracked-bundled", result.stdout)
        self.assertIn("git rm --cached", result.stdout)
        self.assertIn("rules-audit", result.stdout)
        after = subprocess.run(
            ["git", "-C", str(target), "ls-files", "ai-specs/commands/rules-audit.md"],
            capture_output=True, text=True, check=True,
        ).stdout
        self.assertEqual(before, after)


class SymlinkDiagnosticsTests(unittest.TestCase):
    def _cli_home(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        return isolated_home(Path(td.name))

    def _doctor(self, target, home):
        invoke(target, "refresh-bundled", cli_home=home)
        return invoke(target, "doctor", cli_home=home)

    def test_instruction_symlink_valid_reports_ok(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, agents=["claude"], cli_home=home)
        sync = invoke(target, "sync-agent", cli_home=home)
        self.assertEqual(sync.returncode, 0)
        result = self._doctor(target, home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("OK", result.stdout)
        self.assertIn("CLAUDE.md", result.stdout)

    def test_stale_commands_reports_warn(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, agents=["cursor"], cli_home=home)
        sync = invoke(target, "sync-agent", cli_home=home)
        self.assertEqual(sync.returncode, 0)
        (target / ".cursor" / "commands" / "stale.md").write_text("# stale\n")
        result = self._doctor(target, home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("WARN", result.stdout)
        self.assertIn("stale", result.stdout)

    def test_instruction_symlink_invalid_reports_error(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, agents=["claude"], cli_home=home)
        sync = invoke(target, "sync-agent", cli_home=home)
        self.assertEqual(sync.returncode, 0)
        claude_md = target / "CLAUDE.md"
        if claude_md.is_symlink() or claude_md.exists():
            claude_md.unlink()
        claude_md.write_text("stale content")
        result = self._doctor(target, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("ERROR", result.stdout)
        self.assertIn("CLAUDE.md", result.stdout)

    def test_skill_symlink_valid_reports_ok(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, agents=["claude"], cli_home=home)
        sync = invoke(target, "sync-agent", cli_home=home)
        self.assertEqual(sync.returncode, 0)
        result = self._doctor(target, home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("OK", result.stdout)
        self.assertIn("skills", result.stdout)

    def test_copied_skill_directory_valid_reports_ok(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, agents=["opencode"], cli_home=home)
        sync = invoke(target, "sync-agent", cli_home=home)
        self.assertEqual(sync.returncode, 0)
        result = self._doctor(target, home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("OK", result.stdout)


class MCPDiagnosticsTests(unittest.TestCase):
    def _cli_home(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        return isolated_home(Path(td.name))

    def _doctor(self, target, home):
        invoke(target, "refresh-bundled", cli_home=home)
        return invoke(target, "doctor", cli_home=home)

    def test_no_mcp_servers_reports_warn(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        result = self._doctor(target, home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("WARN", result.stdout)
        self.assertIn("mcp", result.stdout.lower())

    def test_mcp_config_present_reports_ok(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, agents=["claude"], cli_home=home)
        update_toml_field(
            target / "ai-specs" / "ai-specs.toml",
            "mcp", "demo",
            {"command": "npx"}
        )
        sync = invoke(target, "sync-agent", cli_home=home)
        self.assertEqual(sync.returncode, 0)
        result = self._doctor(target, home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("OK", result.stdout)
        self.assertIn("mcp", result.stdout.lower())

    def test_mcp_config_missing_reports_error(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, agents=["claude"], cli_home=home)
        update_toml_field(
            target / "ai-specs" / "ai-specs.toml",
            "mcp", "demo",
            {"command": "npx"}
        )
        mcp_file = target / ".mcp.json"
        if mcp_file.exists():
            mcp_file.unlink()
        result = self._doctor(target, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("ERROR", result.stdout)
        self.assertIn("mcp", result.stdout.lower())


class ReportAndExitCodeTests(unittest.TestCase):
    def _cli_home(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        return isolated_home(Path(td.name))

    def _doctor(self, target, home):
        invoke(target, "refresh-bundled", cli_home=home)
        return invoke(target, "doctor", cli_home=home)

    def test_healthy_project_exits_zero(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        result = self._doctor(target, home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("OK", result.stdout)

    def test_project_with_errors_exits_nonzero(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        result = invoke(target, "doctor", cli_home=home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("ERROR", result.stdout)

    def test_severity_labels_present(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        result = self._doctor(target, home)
        found = False
        for label in ("OK", "WARN", "ERROR"):
            if label in result.stdout:
                found = True
                break
        self.assertTrue(found)
        self.assertIn("Summary", result.stdout)

    def test_non_ok_includes_actionable_guidance(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        result = invoke(target, "doctor", cli_home=home)
        if "ERROR" in result.stdout:
            words = result.stdout.lower()
            self.assertTrue(
                "init" in words or "sync" in words or "missing" in words
            )
class PlatformGetTests(unittest.TestCase):
    """Platform field values, observed through the CLI surfaces that consume
    them: sync-agent emit + doctor per-agent checks. Each platform field
    (skills_dir, mcp_config_path, mcp_key, native, instructions_path,
    commands_dir, runtime_hooks_target) surfaces as a doctor check
    name/message or as a materialized symlink/file in the sync-agent tree."""

    def _cli_home(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        return isolated_home(Path(td.name))

    def _target(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        return target

    # TRIAGE: kept coupled so platform.sh internal-field contracts still run;
    # used ONLY by the tests marked TRIAGE below.
    PLATFORM_SH = ROOT / "lib" / "_internal" / "platform.sh"

    def _platform_get(self, agent: str, field: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            ["bash", "-c", f'source "{self.PLATFORM_SH}" && platform_get {agent} {field}'],
            capture_output=True, text=True, check=False,
        )

    def _prep(self, agents, *, mcp=False, hooks_recipe=False):
        """Project + shared home; optionally adds an [mcp.*] server."""
        target = self._target()
        home = self._cli_home()
        if hooks_recipe:
            # A catalog home with the wt-hook runtime-hook recipe, so
            # sync can render hooks to the omp extensions target.
            home = self._hooks_home()
        ai_specs_init(target, agents=agents, cli_home=home)
        if mcp:
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "mcp", "demo", {"command": "npx"},
            )
        return target, home

    def _hooks_home(self):
        """Temp home whose catalog also ships a wt-hook runtime-hook recipe."""
        from _fixture_catalog import populate_catalog

        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = isolated_home(Path(tmp.name))
        catalog = home / "catalog"
        catalog.unlink()
        recipes = catalog / "recipes"
        recipes.mkdir(parents=True)
        (recipes / "wt-hook" / "hooks").mkdir(parents=True)
        (recipes / "wt-hook" / "hooks" / "gate.sh").write_text(
            "#!/usr/bin/env bash\nexit 0\n"
        )
        (recipes / "wt-hook" / "recipe.toml").write_text(
            "[recipe]\n"
            'id = "wt-hook"\n'
            'name = "WT Hook"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            "[[provides.hooks]]\n"
            'id = "gate"\n'
            'event = "pre-tool-use"\n'
            'script = "hooks/gate.sh"\n'
            'matcher = "Edit|Write"\n'
            "blocking = true\n"
        )
        populate_catalog(recipes)
        return home

    def _sync_doc(self, target, home):
        sync = invoke(target, "sync-agent", cli_home=home)
        doctor = invoke(target, "doctor", cli_home=home)
        return sync, doctor

    # --- Pi agent field tests ---

    def test_pi_skills_dir(self):
        """pi skills_dir == .pi/skills: doctor reports a .pi/skills check."""
        target, home = self._prep(["pi"])
        sync, doctor = self._sync_doc(target, home)
        self.assertEqual(sync.returncode, 0)
        self.assertIn(".pi/skills", doctor.stdout)
        self.assertTrue((target / ".pi" / "skills").is_symlink())

    def test_pi_mcp_config_path(self):
        """pi mcp_config_path == .mcp.json: mcp-pi row names .mcp.json."""
        target, home = self._prep(["pi"], mcp=True)
        sync, doctor = self._sync_doc(target, home)
        self.assertEqual(sync.returncode, 0)
        self.assertIn("mcp-pi", doctor.stdout)
        self.assertIn(".mcp.json present", doctor.stdout)
        self.assertTrue((target / ".mcp.json").is_file())

    def test_pi_mcp_key(self):
        """pi mcp_key == mcpServers: emitted .mcp.json keys by that name."""
        target, home = self._prep(["pi"], mcp=True)
        sync, _ = self._sync_doc(target, home)
        self.assertEqual(sync.returncode, 0)
        mcp_json = target / ".mcp.json"
        self.assertTrue(mcp_json.is_file())
        self.assertIn("mcpServers", mcp_json.read_text())

    def test_pi_native_true(self):
        """pi native true: sync-agent does not reject pi as unknown agent."""
        target, home = self._prep(["pi"])
        sync, doctor = self._sync_doc(target, home)
        self.assertEqual(sync.returncode, 0)
        self.assertNotIn("unknown agent", doctor.stdout.lower())
        self.assertNotIn("unsupported agent", doctor.stdout.lower())

    def test_pi_instructions_path_empty(self):
        """pi instructions_path empty: no instruction symlink materializes."""
        target, home = self._prep(["pi"])
        sync, _ = self._sync_doc(target, home)
        self.assertEqual(sync.returncode, 0)
        self.assertFalse((target / ".pi" / "AGENTS.md").exists())
        self.assertFalse((target / ".pi" / "CLAUDE.md").exists())

    def test_pi_commands_dir_empty(self):
        """pi commands_dir empty: .pi/commands is never created."""
        target, home = self._prep(["pi"])
        sync, _ = self._sync_doc(target, home)
        self.assertEqual(sync.returncode, 0)
        self.assertFalse((target / ".pi" / "commands").exists())

    # TRIAGE: test_pi_agents_dir_empty — the platform.sh agents_dir field is
    # not consumed by any CLI verb: doctor/sync output and the emitted tree
    # contain no agents_dir value. Ran `ai-specs doctor` and `ai-specs
    # sync-agent` on a pi-enabled project via invoke; exit code, stdout/stderr
    # and tree expose nothing to assert against, so the empty-string contract
    # stays coupled.
    def test_pi_agents_dir_empty(self):
        result = self._platform_get("pi", "agents_dir")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "")

    # TRIAGE: test_pi_invalid_field_exits_nonzero — platform_get validates the
    # field name internally; no bin/ai-specs verb accepts a field-name
    # argument, so nothing the CLI does reproduces the non-zero exit.
    def test_pi_invalid_field_exits_nonzero(self):
        result = self._platform_get("pi", "nonexistent_field")
        self.assertNotEqual(result.returncode, 0)

    # --- Omp agent field tests ---

    def test_omp_skills_dir(self):
        """omp skills_dir == .omp/skills: doctor reports a .omp/skills check."""
        target, home = self._prep(["omp"])
        sync, doctor = self._sync_doc(target, home)
        self.assertEqual(sync.returncode, 0)
        self.assertIn(".omp/skills", doctor.stdout)
        self.assertTrue((target / ".omp" / "skills").is_symlink())

    def test_omp_mcp_config_path(self):
        """omp mcp_config_path == .omp/mcp.json: mcp-omp names that path."""
        target, home = self._prep(["omp"], mcp=True)
        sync, doctor = self._sync_doc(target, home)
        self.assertEqual(sync.returncode, 0)
        self.assertIn("mcp-omp", doctor.stdout)
        self.assertIn(".omp/mcp.json present", doctor.stdout)
        self.assertTrue((target / ".omp" / "mcp.json").is_file())

    def test_omp_mcp_key(self):
        """omp mcp_key == mcpServers: .omp/mcp.json keys by that name."""
        target, home = self._prep(["omp"], mcp=True)
        sync, _ = self._sync_doc(target, home)
        self.assertEqual(sync.returncode, 0)
        mcp_json = target / ".omp" / "mcp.json"
        self.assertTrue(mcp_json.is_file())
        self.assertIn("mcpServers", mcp_json.read_text())

    def test_omp_native_true(self):
        """omp native true: sync-agent never flags omp as unknown."""
        target, home = self._prep(["omp"])
        sync, doctor = self._sync_doc(target, home)
        self.assertEqual(sync.returncode, 0)
        self.assertNotIn("unknown agent", doctor.stdout.lower())
        self.assertNotIn("unsupported agent", doctor.stdout.lower())

    def test_omp_instructions_path_native_slot(self):
        """omp instructions_path == .omp/AGENTS.md: symlink materializes."""
        target, home = self._prep(["omp"])
        sync, doctor = self._sync_doc(target, home)
        self.assertEqual(sync.returncode, 0)
        self.assertTrue((target / ".omp" / "AGENTS.md").is_symlink())
        self.assertIn(".omp/AGENTS.md", doctor.stdout)

    def test_omp_commands_dir(self):
        """omp commands_dir == .omp/commands: bundled commands fan out there."""
        target, home = self._prep(["omp"])
        sync, doctor = self._sync_doc(target, home)
        self.assertEqual(sync.returncode, 0)
        self.assertTrue((target / ".omp" / "commands").is_dir())
        self.assertIn(".omp/commands", doctor.stdout)

    # TRIAGE: test_omp_agents_dir_empty — same as pi: no CLI verb consumes
    # platform.sh's agents_dir, and doctor/sync output emits no omp agents_dir
    # value (verified via invoke(sync-agent) + invoke(doctor) on omp-enabled
    # project: exit code, tree and stdout carry no agents_dir evidence).
    def test_omp_agents_dir_empty(self):
        result = self._platform_get("omp", "agents_dir")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "")

    def test_omp_runtime_hooks_target(self):
        """omp runtime_hooks_target == .omp/extensions: sync with a runtime
        hook recipe emits the extensions dir."""
        target, home = self._prep(["omp"], hooks_recipe=True)
        toml = target / "ai-specs" / "ai-specs.toml"
        if "[recipes.wt-hook]" not in toml.read_text():
            toml.write_text(
                toml.read_text().rstrip()
                + '\n[recipes.wt-hook]\nenabled = true\nversion = "1.0"\n'
            )
        sync = invoke(target, "sync", cli_home=home)
        self.assertEqual(sync.returncode, 0)
        self.assertTrue((target / ".omp" / "extensions").is_dir())

    # TRIAGE: test_omp_invalid_field_exits_nonzero — identical internal
    # field-name validation as pi; no bin/ai-specs verb accepts a field name.
    def test_omp_invalid_field_exits_nonzero(self):
        result = self._platform_get("omp", "nonexistent_field")
        self.assertNotEqual(result.returncode, 0)

    # --- Regression: existing agents still work ---

    def test_claude_skills_dir_unchanged(self):
        """claude skills_dir == .claude/skills: doctor reports that check."""
        target, home = self._prep(["claude"])
        sync, doctor = self._sync_doc(target, home)
        self.assertEqual(sync.returncode, 0)
        self.assertIn(".claude/skills", doctor.stdout)
        self.assertTrue((target / ".claude" / "skills").is_symlink())

    def test_cursor_skills_dir(self):
        """cursor skills_dir == .cursor/skills: doctor reports that check."""
        target, home = self._prep(["cursor"])
        sync, doctor = self._sync_doc(target, home)
        self.assertEqual(sync.returncode, 0)
        self.assertIn(".cursor/skills", doctor.stdout)
        self.assertTrue((target / ".cursor" / "skills").is_symlink())

    def test_opencode_mcp_key_unchanged(self):
        """opencode mcp_key == mcp: opencode.json keys by that name."""
        target, home = self._prep(["opencode"], mcp=True)
        sync, _ = self._sync_doc(target, home)
        self.assertEqual(sync.returncode, 0)
        oc_json = target / "opencode.json"
        self.assertTrue(oc_json.is_file())
        self.assertIn('"mcp"', oc_json.read_text())

    def test_invalid_agent_exits_nonzero(self):
        """Unknown agent: doctor reports unsupported agent ERROR — the CLI
        surface of platform.sh's unknown-agent rejection."""
        target, home = self._prep(["fakerobot"])
        result = invoke(target, "doctor", cli_home=home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unsupported agent", result.stdout)
        self.assertIn("fakerobot", result.stdout)


class BriefRenderPolicyDoctorTests(unittest.TestCase):
    def _cli_home(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        return isolated_home(Path(td.name))

    def _doctor(self, target, home):
        invoke(target, "refresh-bundled", cli_home=home)
        return invoke(target, "doctor", cli_home=home)

    def _append_brief_render_false(self, toml_path: Path) -> None:
        text = toml_path.read_text().rstrip() + "\n\n[brief]\nrender = false\n"
        toml_path.write_text(text + "\n")

    def test_render_disabled_with_agents_md_reports_info(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        self._append_brief_render_false(target / "ai-specs" / "ai-specs.toml")
        result = self._doctor(target, home)
        self.assertEqual(result.returncode, 0)
        self.assertIn("INFO", result.stdout)
        self.assertIn("brief-render", result.stdout)

    def test_render_disabled_missing_agents_md_reports_error(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        (target / "AGENTS.md").unlink()
        self._append_brief_render_false(target / "ai-specs" / "ai-specs.toml")
        result = self._doctor(target, home)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("ERROR", result.stdout)
        self.assertIn("brief.render = false", result.stdout)

    def test_render_disabled_with_recipe_fragments_reports_warn(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        self._append_brief_render_false(target / "ai-specs" / "ai-specs.toml")
        result = self._doctor(target, home)
        self.assertIn("WARN", result.stdout)
        self.assertIn("brief-fragments-unused", result.stdout)
        # S2: Also assert the INFO brief-render signal is emitted alongside
        # the WARN so a future regression dropping INFO is caught.
        self.assertIn("INFO", result.stdout)
        self.assertIn("brief-render", result.stdout)


class CliVersionDoctorTests(unittest.TestCase):
    def _cli_home(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        return isolated_home(Path(td.name))

    def _doctor(self, target, home, *, refresh=True):
        if refresh:
            invoke(target, "refresh-bundled", cli_home=home)
        return invoke(target, "doctor", cli_home=home)

    def _append_tool_section(self, toml_path: Path, body: str) -> None:
        text = toml_path.read_text().rstrip()
        toml_path.write_text(text + "\n\n[tool]\n" + body + "\n")

    def test_exact_pin_aligned_reports_ok(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
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
        result = self._doctor(target, home)
        self.assertIn("cli-version", result.stdout)
        self.assertIn("OK", result.stdout)
        self.assertIn(installed, result.stdout)

    def test_exact_pin_mismatch_reports_error(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        self._append_tool_section(
            target / "ai-specs" / "ai-specs.toml",
            'version = "99.99.99"\npolicy = "exact"',
        )
        result = self._doctor(target, home)
        self.assertIn("cli-version", result.stdout)
        self.assertIn("ERROR", result.stdout)
        self.assertNotEqual(result.returncode, 0)

    def test_no_pin_stale_last_sync_reports_warn(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        lock_path = target / "ai-specs" / ".ai-specs.lock"
        lock_path.write_text(
            '[meta]\ncli_version = "0.10.0"\n'
            'synced_at = "2026-01-01T00:00:00Z"\n'
        )
        result = self._doctor(target, home)
        self.assertIn("cli-version", result.stdout)
        self.assertIn("WARN", result.stdout)

    def test_doctor_cli_version_is_read_only(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        self._append_tool_section(
            target / "ai-specs" / "ai-specs.toml",
            'version = "99.99.99"\npolicy = "exact"',
        )
        toml_path = target / "ai-specs" / "ai-specs.toml"
        before = toml_path.read_text()
        self._doctor(target, home)
        self.assertEqual(before, toml_path.read_text())


def _find_files(root: Path):
    for p in root.rglob("*"):
        if p.is_file():
            yield p


class RecipeCliDepsDoctorTests(unittest.TestCase):
    """CLI-driven recipe-dep checks: a home whose catalog ships a demo recipe
    with [[deps.cli]] entries; binary presence is controlled via PATH so the
    doctor subprocess's shutil.which is observable and deterministic."""

    def _cli_home_with_recipe(self, recipe_toml: str) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = isolated_home(Path(tmp.name))
        catalog = home / "catalog"
        catalog.unlink()
        recipes = catalog / "recipes"
        recipes.mkdir(parents=True)
        (recipes / "demo-recipe").mkdir()
        (recipes / "demo-recipe" / "recipe.toml").write_text(recipe_toml)
        populate_catalog(recipes)
        return home

    def _write_project(self, home: Path, enabled: bool = True) -> Path:
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        ai_specs_init(target, cli_home=home)
        flag = "true" if enabled else "false"
        (target / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "demo"\n\n'
            '[agents]\nenabled = []\n\n'
            f'[recipes.demo-recipe]\nenabled = {flag}\nversion = "1.0"\n'
        )
        return target

    @staticmethod
    def _path_without(binary: str) -> str:
        """PATH excluding every dir that contains `binary`."""
        kept = [
            d
            for d in os.environ.get("PATH", "").split(os.pathsep)
            if d and not (Path(d) / binary).exists()
        ]
        return os.pathsep.join(kept)

    @staticmethod
    def _fakebin(*names: str) -> str:
        """A temp bin dir with executable stubs; returned as a PATH string."""
        d = tempfile.mkdtemp(prefix="ai-specs-fakebin-")
        for name in names:
            p = Path(d) / name
            p.write_text("#!/bin/sh\nexit 0\n")
            p.chmod(0o755)
        return d

    def test_recipe_cli_deps_warn_when_missing(self):
        """demo-recipe with a required gh dep, gh NOT on PATH → WARN,
        exit 0 (WARN never flips the code), guidance shows the install URL."""
        home = self._cli_home_with_recipe(
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
            'install_url = "https://cli.github.com/"\n'
        )
        target = self._write_project(home)
        with patch.dict("os.environ", {"PATH": self._path_without("gh")}):
            invoke(target, "refresh-bundled", cli_home=home)
            result = invoke(target, "doctor", cli_home=home)
        dep_lines = [l for l in result.stdout.splitlines() if "recipe-dep" in l]
        self.assertTrue(
            any("WARN" in l and "gh" in l for l in dep_lines),
            f"expected WARN recipe-dep for missing gh; got: {dep_lines}",
        )
        self.assertTrue(
            any("https://cli.github.com/" in l for l in dep_lines),
            f"install_url guidance missing; got: {dep_lines}",
        )
        self.assertEqual(result.returncode, 0)

    def test_recipe_cli_deps_info_when_optional_missing(self):
        """Optional dep absent -> INFO recipe-dep row, never WARN/ERROR."""
        home = self._cli_home_with_recipe(
            "[recipe]\n"
            'id = "demo-recipe"\n'
            'name = "Demo"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            "\n"
            "[[deps.cli]]\n"
            'binary = "jq-optional-missing-xyz"\n'
            'purpose = "JSON"\n'
            "required = false\n"
        )
        target = self._write_project(home)
        with patch.dict("os.environ", {"PATH": self._path_without("jq-optional-missing-xyz")}):
            invoke(target, "refresh-bundled", cli_home=home)
            result = invoke(target, "doctor", cli_home=home)
        dep_lines = [l for l in result.stdout.splitlines() if "recipe-dep" in l]
        self.assertTrue(
            any("INFO" in l and "optional jq-optional-missing-xyz" in l for l in dep_lines),
            f"expected INFO optional jq-optional-missing-xyz; got: {dep_lines}",
        )

    def test_recipe_cli_deps_ok_when_found(self):
        """gh found on PATH -> OK recipe-dep row."""
        home = self._cli_home_with_recipe(
            "[recipe]\n"
            'id = "demo-recipe"\n'
            'name = "Demo"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            "\n"
            "[[deps.cli]]\n"
            'binary = "gh"\n'
            'purpose = "Create PRs"\n'
        )
        target = self._write_project(home)
        fakebin = self._fakebin("gh")
        with patch.dict(
            "os.environ",
            {"PATH": fakebin + os.pathsep + self._path_without("gh")},
        ):
            invoke(target, "refresh-bundled", cli_home=home)
            result = invoke(target, "doctor", cli_home=home)
        dep_lines = [l for l in result.stdout.splitlines() if "recipe-dep" in l]
        self.assertTrue(
            any("OK" in l and "gh available" in l for l in dep_lines),
            f"expected OK gh available; got: {dep_lines}",
        )

    def test_doctor_no_crash_when_no_recipes(self):
        """No enabled recipes -> doctor exits and emits no recipe-dep row."""
        home = self._cli_home_with_recipe(
            "[recipe]\n"
            'id = "demo-recipe"\n'
            'name = "Demo"\n'
            'description = "D"\n'
            'version = "1.0"\n'
        )
        target = self._write_project(home, enabled=False)
        invoke(target, "refresh-bundled", cli_home=home)
        result = invoke(target, "doctor", cli_home=home)
        self.assertNotIn("recipe-dep", result.stdout)
        self.assertIsInstance(result.returncode, int)

    def test_doctor_exit_code_unchanged(self):
        """WARN-only recipe-dep rows must not flip the exit code (0)."""
        home = self._cli_home_with_recipe(
            "[recipe]\n"
            'id = "demo-recipe"\n'
            'name = "Demo"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            "\n"
            "[[deps.cli]]\n"
            'binary = "missing-cli-xyz"\n'
            'purpose = "demo"\n'
            "required = true\n"
        )
        target = self._write_project(home)
        with patch.dict("os.environ", {"PATH": self._path_without("missing-cli-xyz")}):
            invoke(target, "refresh-bundled", cli_home=home)
            result = invoke(target, "doctor", cli_home=home)
        dep_lines = [l for l in result.stdout.splitlines() if "recipe-dep" in l]
        self.assertTrue(dep_lines, "expected at least one recipe-dep row")
        self.assertFalse(
            any("ERROR" in l for l in dep_lines),
            f"recipe-dep must never be ERROR; got: {dep_lines}",
        )
        self.assertEqual(result.returncode, 0)


class HarnessEnvDoctorTests(unittest.TestCase):
    """CLI-driven harness-env checks: demo-recipe provides an MCP env var;
    direnv availability is controlled via PATH; .envrc / ai-specs.env are
    fixture files written directly into the test project."""

    def _cli_home_with_recipe(self, recipe_toml: str) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = isolated_home(Path(tmp.name))
        catalog = home / "catalog"
        catalog.unlink()
        recipes = catalog / "recipes"
        recipes.mkdir(parents=True)
        (recipes / "demo-recipe").mkdir()
        (recipes / "demo-recipe" / "recipe.toml").write_text(
            recipe_toml, encoding="utf-8"
        )
        populate_catalog(recipes)
        return home

    def _write_mcp_project(self, home: Path, recipe_toml: str) -> Path:
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        ai_specs_init(target, cli_home=home)
        (target / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = \"demo\"\n\n"
            "[agents]\nenabled = []\n\n"
            "[recipes.demo-recipe]\nenabled = true\nversion = \"1.0\"\n",
            encoding="utf-8",
        )
        return target

    @staticmethod
    def _path_without(name: str) -> str:
        kept = [
            d
            for d in os.environ.get("PATH", "").split(os.pathsep)
            if d and not (Path(d) / name).exists()
        ]
        return os.pathsep.join(kept)

    @staticmethod
    def _fakebin(*names: str) -> str:
        d = tempfile.mkdtemp(prefix="ai-specs-fakebin-")
        for name in names:
            p = Path(d) / name
            p.write_text("#!/bin/sh\nexit 0\n")
            p.chmod(0o755)
        return d

    def _mcp_recipe(self) -> str:
        return (
            "[recipe]\n"
            'id = "demo-recipe"\n'
            'name = "Demo"\n'
            'description = "D"\n'
            'version = "1.0"\n\n'
            "[[provides.mcp]]\n"
            'id = "trello"\n'
            'command = "npx"\n'
            "env = { TRELLO_TOKEN = \"$TRELLO_TOKEN\" }\n"
        )

    def test_direnv_warn_when_mcp_env_required(self):
        """MCP env recipe + no direnv on PATH -> WARN direnv row."""
        home = self._cli_home_with_recipe(self._mcp_recipe())
        target = self._write_mcp_project(home, self._mcp_recipe())
        with patch.dict("os.environ", {"PATH": self._path_without("direnv")}):
            invoke(target, "refresh-bundled", cli_home=home)
            result = invoke(target, "doctor", cli_home=home)
        rows = [l for l in result.stdout.splitlines() if " direnv " in l]
        self.assertTrue(
            any("WARN" in l for l in rows),
            f"expected WARN direnv; got rows: {rows}",
        )

    def test_no_direnv_warn_without_mcp_env(self):
        """Recipe without MCP env -> no direnv row at all."""
        recipe = (
            "[recipe]\n"
            'id = "demo-recipe"\n'
            'name = "Demo"\n'
            'description = "D"\n'
            'version = "1.0"\n'
        )
        home = self._cli_home_with_recipe(recipe)
        target = self._write_mcp_project(home, recipe)
        with patch.dict("os.environ", {"PATH": self._path_without("direnv")}):
            invoke(target, "refresh-bundled", cli_home=home)
            result = invoke(target, "doctor", cli_home=home)
        self.assertFalse(
            any("direnv" in l for l in result.stdout.splitlines()),
            "no direnv check expected without MCP env",
        )

    def test_managed_envrc_and_harness_key_warns(self):
        """Non-managed .envrc + empty harness key -> envrc-managed WARN and
        harness-env WARN naming TRELLO_TOKEN (never 'secret')."""
        home = self._cli_home_with_recipe(self._mcp_recipe())
        target = self._write_mcp_project(home, self._mcp_recipe())
        (target / ".envrc").write_text("use nix\n", encoding="utf-8")
        (target / "ai-specs.env").write_text(
            "TRELLO_TOKEN=\n", encoding="utf-8"
        )
        with patch.dict("os.environ", {"PATH": self._fakebin("direnv") + os.pathsep + self._path_without("direnv")}):
            invoke(target, "refresh-bundled", cli_home=home)
            result = invoke(target, "doctor", cli_home=home)
        rows = result.stdout.splitlines()
        self.assertTrue(
            any("envrc-managed" in l and "WARN" in l for l in rows),
            f"expected envrc-managed WARN; got: {[l for l in rows if 'envrc' in l]}",
        )
        harness = [l for l in rows if "harness-env" in l]
        self.assertTrue(
            any("TRELLO_TOKEN" in l for l in harness),
            f"expected TRELLO_TOKEN in harness-env; got: {harness}",
        )
        self.assertFalse(
            any("secret" in l.lower() for l in harness),
            f"harness-env must not leak 'secret'; got: {harness}",
        )

    def test_present_harness_key_ok(self):
        """Managed .envrc + non-empty key -> harness-env OK, no WARN."""
        home = self._cli_home_with_recipe(self._mcp_recipe())
        target = self._write_mcp_project(home, self._mcp_recipe())
        (target / ".envrc").write_text(
            "# managed-by: ai-specs (do not remove block)\n"
            "dotenv_if_exists .env\n"
            "dotenv_if_exists ai-specs.env\n"
            "# end managed-by: ai-specs\n",
            encoding="utf-8",
        )
        (target / "ai-specs.env").write_text(
            "TRELLO_TOKEN=filled-value\n",
            encoding="utf-8",
        )
        with patch.dict("os.environ", {"PATH": self._fakebin("direnv") + os.pathsep + self._path_without("direnv")}):
            invoke(target, "refresh-bundled", cli_home=home)
            result = invoke(target, "doctor", cli_home=home)
        rows = [l for l in result.stdout.splitlines() if "harness-env" in l]
        self.assertTrue(
            any("OK" in l for l in rows),
            f"expected harness-env OK when key present; got: {rows}",
        )
        self.assertFalse(
            any("WARN" in l for l in rows),
            f"harness-env must not WARN when key is non-empty; got: {rows}",
        )

    def test_stale_managed_body_warns(self):
        """JD-8: markers with nested ai-specs/.env body -> WARN envrc-managed."""
        home = self._cli_home_with_recipe(self._mcp_recipe())
        target = self._write_mcp_project(home, self._mcp_recipe())
        (target / ".envrc").write_text(
            "# managed-by: ai-specs (do not remove block)\n"
            "dotenv_if_exists .env\n"
            "dotenv_if_exists ai-specs/.env\n"
            "# end managed-by: ai-specs\n",
            encoding="utf-8",
        )
        (target / "ai-specs.env").write_text(
            "TRELLO_TOKEN=filled-value\n",
            encoding="utf-8",
        )
        with patch.dict("os.environ", {"PATH": self._fakebin("direnv") + os.pathsep + self._path_without("direnv")}):
            invoke(target, "refresh-bundled", cli_home=home)
            result = invoke(target, "doctor", cli_home=home)
        rows = [l for l in result.stdout.splitlines() if "envrc-managed" in l]
        self.assertTrue(
            any("WARN" in l and "stale" in l.lower() for l in rows),
            f"expected envrc-managed WARN for stale body; got: {rows}",
        )

    def test_doctor_never_calls_install(self):
        """Doctor never executes brew/apt-get: plant spy executables on PATH
        that record when invoked, run doctor with a missing direnv, and assert
        the spies never fired."""
        spy_dir = Path(tempfile.mkdtemp(prefix="ai-specs-spies-"))
        marker = spy_dir / "marker-hit"
        for name in ("brew", "apt-get"):
            s = spy_dir / name
            s.write_text(
                f"#!/bin/sh\nprintf '%s\\n' {name} >> {marker}\nexit 0\n"
            )
            s.chmod(0o755)
        home = self._cli_home_with_recipe(self._mcp_recipe())
        target = self._write_mcp_project(home, self._mcp_recipe())
        clean_path = self._path_without("direnv")
        with patch.dict(
            "os.environ",
            {"PATH": str(spy_dir) + os.pathsep + clean_path},
        ):
            invoke(target, "refresh-bundled", cli_home=home)
            result = invoke(target, "doctor", cli_home=home)
        self.assertEqual(result.returncode, 0)
        self.assertFalse(
            marker.exists(),
            f"doctor invoked an installer; marker {marker} exists",
        )


class CacheAwareCommandsDoctorTests(unittest.TestCase):
    """Doctor must treat cache-managed commands as 'expected', not stale
    extras. Cache commands are planted directly into the shared home's cache
    (path derivable via cache_project_dir) before running the CLI."""

    def _cli_home(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        return isolated_home(Path(td.name))

    def _doctor(self, target, home):
        invoke(target, "refresh-bundled", cli_home=home)
        return invoke(target, "doctor", cli_home=home)

    def test_bundled_commands_ok_when_only_cache_has_commands(self):
        """bundled-commands must report OK when cache commands/ is non-empty,
        even if ai-specs/commands/ is empty."""
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        # Clear hand-authored commands
        commands_root = target / "ai-specs" / "commands"
        shutil.rmtree(commands_root, ignore_errors=True)
        commands_root.mkdir(parents=True)
        # Populate cache-managed commands in the shared home's cache
        cache_cmds = cache_project_dir(target, home) / "commands"
        cache_cmds.mkdir(parents=True, exist_ok=True)
        (cache_cmds / "recipe-cmd.md").write_text("# recipe command\n")
        result = self._doctor(target, home)
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
        """Doctor must not flag agent commands as stale when they come from
        the cache."""
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, agents=["cursor"], cli_home=home)
        # Populate cache with a recipe-managed command
        cache_cmds = cache_project_dir(target, home) / "commands"
        cache_cmds.mkdir(parents=True, exist_ok=True)
        (cache_cmds / "recipe-cmd.md").write_text("# recipe command\n")
        # Sync so agent commands dir is populated
        sync = invoke(target, "sync-agent", "--cursor", cli_home=home)
        self.assertEqual(sync.returncode, 0)
        result = self._doctor(target, home)
        cmd_lines = [
            ln for ln in result.stdout.splitlines()
            if ".cursor/commands" in ln
        ]
        self.assertFalse(
            any("stale" in ln.lower() for ln in cmd_lines),
            f"Cache-managed commands must not be flagged as stale; got: {cmd_lines}"
        )


class CommandsEmptyExpectedDoctorTests(unittest.TestCase):
    """Doctor must not WARN when both expected and actual command sets are
    empty. A fresh isolated home (never populated with cache commands) plus an
    empty .claude/commands directory is the observable equivalent of an empty
    expected ∪ actual set."""

    def _cli_home(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        return isolated_home(Path(td.name))

    def _make_project(self) -> Path:
        """Minimal project: manifest with claude enabled + AGENTS.md."""
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        (target / "AGENTS.md").write_text("# agents\n")
        (target / "ai-specs").mkdir()
        (target / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "demo"\n[agents]\nenabled = ["claude"]\n'
        )
        return target

    def _doctor_with_empty_commands(self) -> tuple:
        target = self._make_project()
        home = self._cli_home()
        # An empty commands dir for the claude agent; the fresh home keeps the
        # cache empty so expected == actual == {}.
        (target / ".claude" / "commands").mkdir(parents=True)
        return target, home, invoke(target, "doctor", cli_home=home)

    def test_empty_commands_dir_with_no_expected_reports_ok_not_warn(self):
        """Empty agent commands dir + zero expected commands → OK, not WARN."""
        _target, _home, result = self._doctor_with_empty_commands()
        cmd_lines = [
            ln for ln in result.stdout.splitlines()
            if ".claude/commands" in ln
        ]
        self.assertTrue(cmd_lines, "expected .claude/commands rows in doctor output")
        self.assertFalse(
            any("WARN" in ln for ln in cmd_lines),
            f"Should not WARN when no commands configured; got: {cmd_lines}",
        )

    def test_empty_commands_dir_with_no_expected_emits_ok_label(self):
        """Empty agent commands dir + zero expected commands → at least one
        OK row for commands."""
        _target, _home, result = self._doctor_with_empty_commands()
        ok_rows = [
            ln for ln in result.stdout.splitlines()
            if ".claude/commands" in ln and "OK" in ln
        ]
        self.assertTrue(
            ok_rows,
            f"Should emit OK when no commands configured; got: "
            f"{[l for l in result.stdout.splitlines() if 'claude/commands' in l]}",
        )



class RepoTopologyDoctorTests(unittest.TestCase):
    def _cli_home(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        return isolated_home(Path(td.name))

    def _enable_worktree_flow(self, target: Path) -> None:
        manifest = target / "ai-specs" / "ai-specs.toml"
        # Always append an enabled block — the init template may contain a
        # commented [recipes.worktree-flow] example that must not count.
        manifest.write_text(
            manifest.read_text()
            + "\n[recipes.worktree-flow]\nenabled = true\n\n"
            + "[recipes.worktree-flow.config]\nrepo_topology = \"auto\"\n"
        )

    def test_repo_topology_info_when_worktree_flow_enabled(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        self._enable_worktree_flow(target)
        result = invoke(target, "doctor", cli_home=home)
        self.assertIn("repo-topology", result.stdout)
        self.assertIn("INFO", result.stdout)

    def test_stale_override_warns(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        home = self._cli_home()
        ai_specs_init(target, cli_home=home)
        self._enable_worktree_flow(target)
        dest = (
            target / "ai-specs" / "recipes" / "worktree-flow" / "overrides"
            / "bin" / "worktree-cleanup.sh"
        )
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text("# customized\n")
        result = invoke(target, "doctor", cli_home=home)
        self.assertIn("stale-override", result.stdout)
        self.assertIn("WARN", result.stdout)
        # read-only: file unchanged
        self.assertEqual(dest.read_text(), "# customized\n")


class GateProvenanceDoctorTests(unittest.TestCase):
    """3.3 — RED (CLI): doctor warns on customized/missing gate provenance,
    stays quiet when the lock baseline matches. Uses the real worktree-flow
    recipe (the shipped catalog ships its hooks), synced via CLI so the lock
    baseline is produced by the CLI, not by test imports."""

    def _cli_home(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        return isolated_home(Path(td.name))

    def _write_project(self, home: Path, *, recipes: bool = True,
                       seed_gate: str | None = None) -> Path:
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        target = Path(td.name) / "prj"
        target.mkdir()
        ai_specs_init(target, agents=["claude"], cli_home=home)
        extra = ""
        if recipes:
            extra = "\n[recipes.worktree-flow]\nenabled = true\n"
        (target / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'p'\n\n"
            "[agents]\nenabled = ['claude']\n"
            + extra
        )
        if seed_gate is not None:
            gate = target / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
            gate.parent.mkdir(parents=True, exist_ok=True)
            gate.write_text(seed_gate)
        return target

    def _gate(self, project: Path) -> Path:
        return (
            project / "ai-specs" / "recipes" / "worktree-flow"
            / "hooks" / "worktree-gate.sh"
        )

    def test_doctor_warns_on_customized_gate(self):
        """sync records a baseline; modifying the gate afterwards yields WARN
        gate-provenance user-modified rows."""
        home = self._cli_home()
        project = self._write_project(home)
        sync = invoke(project, "sync", cli_home=home)
        self.assertEqual(sync.returncode, 0)
        gate = self._gate(project)
        self.assertTrue(gate.is_file(), "sync must materialize the gate")
        gate.write_text("#!/usr/bin/env bash\nexit 1  # customized\n")
        doctor = invoke(project, "doctor", cli_home=home)
        rows = [l for l in doctor.stdout.splitlines() if "gate-provenance" in l]
        self.assertTrue(rows, "expected gate-provenance row(s)")
        for row in rows:
            self.assertIn("WARN", row)
            self.assertIn("gate.sh", row)
            self.assertIn("user-modified", row)

    def test_doctor_quiet_when_gate_baseline_matches(self):
        """Unmodified sync'd gate: the lock baseline matches, doctor emits no
        gate-provenance row."""
        home = self._cli_home()
        project = self._write_project(home)
        result = invoke(project, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0)
        doctor = invoke(project, "doctor", cli_home=home)
        rows = [l for l in doctor.stdout.splitlines() if "gate-provenance" in l]
        self.assertEqual(
            rows, [],
            "doctor must stay quiet for gates whose baseline matches",
        )

    def test_doctor_warns_on_missing_gate_provenance(self):
        """A gate present without a lock baseline -> WARN gate-provenance."""
        home = self._cli_home()
        project = self._write_project(
            home, seed_gate="#!/usr/bin/env bash\nexit 0\n"
        )
        doctor = invoke(project, "doctor", cli_home=home)
        rows = [l for l in doctor.stdout.splitlines() if "gate-provenance" in l]
        self.assertTrue(rows, "expected gate-provenance rows")
        for row in rows:
            self.assertIn("WARN", row)
            self.assertIn("provenance", row.lower())

    def test_doctor_no_hook_recipes_quiet(self):
        """Manifest without a hook recipe -> no gate-provenance row."""
        home = self._cli_home()
        project = self._write_project(home, recipes=False, seed_gate=None)
        doctor = invoke(project, "doctor", cli_home=home)
        rows = [l for l in doctor.stdout.splitlines() if "gate-provenance" in l]
        self.assertEqual(rows, [])


if __name__ == "__main__":
    unittest.main()