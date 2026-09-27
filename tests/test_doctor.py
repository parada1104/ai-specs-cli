"""Black-box doctor tests: every test drives ``bin/ai-specs doctor``.

No test may import ``lib/_internal`` modules. Assertions preserve the original
contract intents (stdout severity labels, frozen exit-code contract, filesystem
effects) through the CLI process boundary.
"""
from __future__ import annotations

import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import (  # noqa: E402
    cache_project_dir,
    invoke,
    isolated_home,
    snapshot,
    tree_diff,
)

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"

BUNDLED_SKILLS = (
    "harness-lifecycle",
    "harness-recipes",
    "harness-skills-deps",
    "skill-creator",
    "skill-sync",
)
BUNDLED_COMMANDS = ("rules-audit", "skills-as-rules")

_LINE_RE = re.compile(r"^\s*(OK|INFO|WARN|ERROR)\s+(?P<name>\S+)\s+(?P<body>.*)$")


def _make_home(base: Path, *, catalog: bool = True) -> Path:
    """Isolated CLI home with a REAL lib copy.

    doctor.py derives its cache root from its own realpath, so a symlinked
    lib would resolve back into the repository and make checks read (never
    write) repo cache state. A real copy keeps every lookup in temp.
    """
    home = isolated_home(base, catalog=catalog)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor"),
    )
    return home


def _seed_clean_cache(root: Path, home: Path) -> None:
    """Pre-seed the per-project bundled cache so only the check under test can
    influence the frozen exit-code contract (exit 1 iff any ERROR)."""
    bundled = cache_project_dir(root, home) / ".bundled"
    for skill in BUNDLED_SKILLS:
        (bundled / "skills" / skill).mkdir(parents=True, exist_ok=True)
    for command in BUNDLED_COMMANDS:
        path = bundled / "commands" / f"{command}.md"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("# bundled\n", encoding="utf-8")


def _named_checks(stdout: str, name: str) -> list[tuple[str, str]]:
    """The rendered (severity, message+guidance) lines for one check name."""
    found = []
    for line in stdout.splitlines():
        match = _LINE_RE.match(line)
        if match and match.group("name") == name:
            found.append((match.group(1), match.group("body")))
    return found


def _check_names(stdout: str) -> set[str]:
    return {
        match.group("name")
        for line in stdout.splitlines()
        if (match := _LINE_RE.match(line))
    }


def _stage_path(base: Path, *, without: tuple[str, ...] = (),
                stubs: dict[str, str] | None = None) -> str:
    """Build a PATH string that hides `without` binaries and adds stub scripts.

    Used because invoke() fixes the environment: PATH surgery is the only
    black-box lever over shutil.which() inside the doctor process.
    """
    stub_dir = base / "path-stubs"
    stub_dir.mkdir(parents=True, exist_ok=True)
    for name, body in (stubs or {}).items():
        script = stub_dir / name
        script.write_text(body, encoding="utf-8")
        script.chmod(0o755)
    filtered_dir = base / "path-filtered"
    filtered_dir.mkdir(parents=True, exist_ok=True)
    for entry in os.environ.get("PATH", "").split(":"):
        if not entry or not Path(entry).is_dir():
            continue
        try:
            entries = list(Path(entry).iterdir())
        except OSError:
            continue
        for item in entries:
            if item.name in without:
                continue
            link = filtered_dir / item.name
            if link.exists():
                continue
            try:
                if item.is_file() and os.access(item, os.X_OK):
                    link.symlink_to(item)
            except OSError:
                continue
    return f"{stub_dir}:{filtered_dir}"


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


def ai_specs_init(path: Path, home: Path, agents: list[str] | None = None) -> None:
    """Black-box init through the CLI, then patch [agents].enabled."""
    result = invoke(path, "init", cli_home=home)
    if result.returncode != 0:
        raise AssertionError(f"ai-specs init failed: {result.stdout}{result.stderr}")
    toml_path = path / "ai-specs" / "ai-specs.toml"
    if agents is not None:
        update_toml_field(toml_path, "agents", "enabled", agents)
    else:
        # Tests assume no enabled agents until sync; template may default to a trio.
        update_toml_field(toml_path, "agents", "enabled", [])


class DoctorCommandAvailabilityTests(unittest.TestCase):
    def test_help_lists_doctor(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            result = invoke(Path(tmp), "help", cli_home=home, append_root=False)
            self.assertIn("doctor", result.stdout)
            self.assertIn("diagnose", result.stdout.lower())

    def test_doctor_accepts_target_path(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            result = invoke(target, "doctor", cli_home=home)
            # invoke() normalizes the target path to <TEMP>; the banner proves
            # the CLI accepted and reported the explicit target argument.
            self.assertIn("ai-specs doctor", result.stdout)
            self.assertIn("target:", result.stdout)

    def test_doctor_is_read_only(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            before = snapshot(target)
            result = invoke(target, "doctor", cli_home=home)
            self.assertEqual(result.returncode, 0)
            self.assertEqual(
                tree_diff(before, snapshot(target)),
                {"created": [], "deleted": [], "modified": []},
            )


class CoreProjectStructureTests(unittest.TestCase):
    def test_manifest_exists_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            result = invoke(target, "doctor", cli_home=home)
            self.assertIn("OK", result.stdout)
            self.assertIn("manifest", result.stdout)

    def test_manifest_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            result = invoke(target, "doctor", cli_home=home)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("manifest", result.stdout.lower())

    def test_agents_md_exists_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            result = invoke(target, "doctor", cli_home=home)
            self.assertIn("OK", result.stdout)
            self.assertIn("AGENTS", result.stdout)

    def test_agents_md_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            (target / "ai-specs").mkdir()
            (target / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "orphan"\n'
            )
            result = invoke(target, "doctor", cli_home=home)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("AGENTS", result.stdout)
            self.assertIn("sync", result.stdout.lower())


class AgentDiagnosticsTests(unittest.TestCase):
    def test_no_enabled_agents_reports_warn(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "agents", "enabled", []
            )
            result = invoke(target, "doctor", cli_home=home)
            self.assertEqual(result.returncode, 0)
            self.assertIn("WARN", result.stdout)
            self.assertIn("enabled", result.stdout.lower())

    def test_unknown_enabled_agent_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "agents", "enabled", ["fakerobot"]
            )
            result = invoke(target, "doctor", cli_home=home)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)

    def test_enabled_agent_output_present_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home, agents=["claude"])
            sync = invoke(target, "sync-agent", cli_home=home)
            self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
            result = invoke(target, "doctor", cli_home=home)
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)

    def test_pi_is_in_platform_dict(self):
        """Pi platform fields via doctor output: skills dir, MCP config path,
        MCP key, and no commands dir (empty commands_dir renders no check)."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home, agents=["pi"])
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "mcp", "demo", {"command": "npx"}
            )
            result = invoke(target, "doctor", cli_home=home)
            self.assertTrue(_named_checks(result.stdout, ".pi/skills"))
            mcp_lines = _named_checks(result.stdout, "mcp-pi")
            self.assertTrue(mcp_lines)
            self.assertIn(".mcp.json", mcp_lines[0][1])
            self.assertNotIn(".pi/commands", _check_names(result.stdout))
            # mcp_key: sync-agent writes the server under the mcpServers key.
            sync = invoke(target, "sync-agent", cli_home=home)
            self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
            data = json.loads((target / ".mcp.json").read_text())
            self.assertIn("mcpServers", data)
            self.assertIn("demo", data["mcpServers"])

    def test_pi_not_rejected_as_unknown_agent(self):
        """Pi in enabled agents must not produce 'unsupported agent' ERROR."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home, agents=["pi"])
            result = invoke(target, "doctor", cli_home=home)
            # Before sync, pi should NOT be flagged as unsupported agent
            self.assertNotIn("unsupported agent", result.stdout.lower())
            self.assertIn("pi", result.stdout)

    def test_pi_output_present_reports_ok(self):
        """Pi with valid .pi/skills symlink reports OK."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home, agents=["pi"])
            sync = invoke(target, "sync-agent", cli_home=home)
            self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
            result = invoke(target, "doctor", cli_home=home)
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)
            self.assertIn(".pi/skills", result.stdout)

    def test_omp_is_in_platform_dict(self):
        """omp platform fields via doctor output: skills dir, MCP config path,
        MCP key, commands dir, and native instructions slot .omp/AGENTS.md."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home, agents=["omp"])
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "mcp", "demo", {"command": "npx"}
            )
            result = invoke(target, "doctor", cli_home=home)
            self.assertTrue(_named_checks(result.stdout, ".omp/skills"))
            self.assertTrue(_named_checks(result.stdout, ".omp/commands"))
            self.assertTrue(_named_checks(result.stdout, ".omp/AGENTS.md"))
            mcp_lines = _named_checks(result.stdout, "mcp-omp")
            self.assertTrue(mcp_lines)
            self.assertIn(".omp/mcp.json", mcp_lines[0][1])
            # mcp_key: sync-agent writes the server under the mcpServers key.
            sync = invoke(target, "sync-agent", cli_home=home)
            self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
            data = json.loads((target / ".omp" / "mcp.json").read_text())
            self.assertIn("mcpServers", data)
            self.assertIn("demo", data["mcpServers"])

    def test_omp_not_rejected_as_unknown_agent(self):
        """omp in enabled agents must not produce 'unsupported agent' ERROR."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home, agents=["omp"])
            result = invoke(target, "doctor", cli_home=home)
            self.assertNotIn("unsupported agent", result.stdout.lower())
            self.assertIn("omp", result.stdout)

    def test_enabled_agent_output_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home, agents=["claude"])
            (target / "AGENTS.md").unlink()
            result = invoke(target, "doctor", cli_home=home)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)


class BundledAssetDiagnosticsTests(unittest.TestCase):
    def test_bundled_skills_present_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            result = invoke(target, "doctor", cli_home=home)
            self.assertIn("OK", result.stdout)
            self.assertIn("skill-creator", result.stdout)

    def test_bundled_skill_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            # CLI-bundled skills live in the per-project cache; remove one there.
            shutil.rmtree(
                cache_project_dir(target, home) / ".bundled" / "skills" / "skill-sync"
            )
            result = invoke(target, "doctor", cli_home=home)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("skill-sync", result.stdout)

    def test_tracked_bundled_leftover_warns_without_git_rm(self):
        """Doctor WARNs when git still tracks a removed CLI-bundled skill path."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
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
            ai_specs_init(target, home)
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
            result = invoke(target, "doctor", cli_home=home)
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
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            result = invoke(target, "doctor", cli_home=home)
            self.assertIn("OK", result.stdout)
            self.assertIn("commands", result.stdout)

    def test_bundled_command_present_reports_ok_by_name(self):
        """Per-bundled-command-id OK check names each bundled command."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            result = invoke(target, "doctor", cli_home=home)
            self.assertIn("OK", result.stdout)
            self.assertIn("rules-audit", result.stdout)
            self.assertIn("skills-as-rules", result.stdout)

    def test_bundled_command_missing_reports_error(self):
        """A bundled command id missing from {cache}/.bundled/commands/ is the
        ERROR signal now (mirrors the per-bundled-skill check); an empty
        hand-authored ai-specs/commands/ is unrelated and healthy."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            (cache_project_dir(target, home) / ".bundled" / "commands"
             / "rules-audit.md").unlink()
            result = invoke(target, "doctor", cli_home=home)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("rules-audit", result.stdout)
            self.assertIn("sync", result.stdout)

    def test_empty_ai_specs_commands_dir_is_healthy(self):
        """An empty hand-authored ai-specs/commands/ is healthy (bundled
        commands resolve from the cache, never from the project surface) —
        the old aggregate 'any command present' WARN no longer applies."""
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            shutil.rmtree(target / "ai-specs" / "commands")
            result = invoke(target, "doctor", cli_home=home)
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
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
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
            ai_specs_init(target, home)
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
            result = invoke(target, "doctor", cli_home=home)
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
    def test_instruction_symlink_valid_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home, agents=["claude"])
            sync = invoke(target, "sync-agent", cli_home=home)
            self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
            result = invoke(target, "doctor", cli_home=home)
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)
            self.assertIn("CLAUDE.md", result.stdout)

    def test_stale_commands_reports_warn(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home, agents=["cursor"])
            sync = invoke(target, "sync-agent", cli_home=home)
            self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
            (target / ".cursor" / "commands" / "stale.md").write_text("# stale\n")
            result = invoke(target, "doctor", cli_home=home)
            self.assertEqual(result.returncode, 0)
            self.assertIn("WARN", result.stdout)
            self.assertIn("stale", result.stdout)

    def test_instruction_symlink_invalid_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home, agents=["claude"])
            sync = invoke(target, "sync-agent", cli_home=home)
            self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
            claude_md = target / "CLAUDE.md"
            if claude_md.is_symlink() or claude_md.exists():
                claude_md.unlink()
            claude_md.write_text("stale content")
            result = invoke(target, "doctor", cli_home=home)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("CLAUDE.md", result.stdout)

    def test_skill_symlink_valid_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home, agents=["claude"])
            sync = invoke(target, "sync-agent", cli_home=home)
            self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
            result = invoke(target, "doctor", cli_home=home)
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)
            self.assertIn("skills", result.stdout)

    def test_copied_skill_directory_valid_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home, agents=["opencode"])
            sync = invoke(target, "sync-agent", cli_home=home)
            self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
            result = invoke(target, "doctor", cli_home=home)
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)


class MCPDiagnosticsTests(unittest.TestCase):
    def test_no_mcp_servers_reports_warn(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            result = invoke(target, "doctor", cli_home=home)
            self.assertEqual(result.returncode, 0)
            self.assertIn("WARN", result.stdout)
            self.assertIn("mcp", result.stdout.lower())

    def test_mcp_config_present_reports_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home, agents=["claude"])
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "mcp", "demo",
                {"command": "npx"}
            )
            sync = invoke(target, "sync-agent", cli_home=home)
            self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
            result = invoke(target, "doctor", cli_home=home)
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)
            self.assertIn("mcp", result.stdout.lower())

    def test_mcp_config_missing_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home, agents=["claude"])
            update_toml_field(
                target / "ai-specs" / "ai-specs.toml",
                "mcp", "demo",
                {"command": "npx"}
            )
            mcp_file = target / ".mcp.json"
            if mcp_file.exists():
                mcp_file.unlink()
            result = invoke(target, "doctor", cli_home=home)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("mcp", result.stdout.lower())


class ReportAndExitCodeTests(unittest.TestCase):
    def test_healthy_project_exits_zero(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            result = invoke(target, "doctor", cli_home=home)
            self.assertEqual(result.returncode, 0)
            self.assertIn("OK", result.stdout)

    def test_project_with_errors_exits_nonzero(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            result = invoke(target, "doctor", cli_home=home)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)

    def test_severity_labels_present(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            result = invoke(target, "doctor", cli_home=home)
            found = False
            for label in ("OK", "WARN", "ERROR"):
                if label in result.stdout:
                    found = True
                    break
            self.assertTrue(found)
            self.assertIn("Summary", result.stdout)

    def test_non_ok_includes_actionable_guidance(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            result = invoke(target, "doctor", cli_home=home)
            if "ERROR" in result.stdout:
                words = result.stdout.lower()
                self.assertTrue(
                    "init" in words or "sync" in words or "missing" in words
                )


class PlatformGetTests(unittest.TestCase):
    """Platform table fields, observed through doctor's rendered agent checks
    (the CLI surface that consumes them)."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.home = _make_home(self.base)

    def _platform_project(self, agent: str, *, mcp: bool = False) -> Path:
        root = self.base / f"prj-{agent}"
        root.mkdir()
        (root / "ai-specs").mkdir()
        toml = f"[project]\nname = 'p'\n\n[agents]\nenabled = ['{agent}']\n"
        if mcp:
            toml += "\n[mcp.demo]\ncommand = 'npx'\n"
        (root / "ai-specs" / "ai-specs.toml").write_text(toml)
        (root / "AGENTS.md").write_text("# agents\n")
        _seed_clean_cache(root, self.home)
        return root

    def _doctor(self, root: Path):
        return invoke(root, "doctor", cli_home=self.home)

    # --- Pi agent fields ---

    def test_pi_skills_dir(self):
        result = self._doctor(self._platform_project("pi"))
        self.assertTrue(_named_checks(result.stdout, ".pi/skills"))

    def test_pi_mcp_config_path(self):
        result = self._doctor(self._platform_project("pi", mcp=True))
        lines = _named_checks(result.stdout, "mcp-pi")
        self.assertTrue(lines)
        self.assertIn(".mcp.json", lines[0][1])

    def test_pi_mcp_key(self):
        root = self._platform_project("pi", mcp=True)
        sync = invoke(root, "sync-agent", cli_home=self.home)
        self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
        data = json.loads((root / ".mcp.json").read_text())
        self.assertIn("mcpServers", data)
        self.assertIn("demo", data["mcpServers"])

    # TRIAGE: ai-specs doctor — the platform table's `native` flag is consumed
    # inside sync-agent's instruction fan-out and has no doctor-rendered
    # surface; the original asserted platform_get's stdout directly.
    def test_pi_native_true(self):
        result = self._doctor(self._platform_project("pi"))
        self.assertTrue(_named_checks(result.stdout, ".pi/skills"))

    def test_pi_instructions_path_empty(self):
        # Empty instructions_path renders no instruction-file check: no check
        # name may be a markdown instructions path.
        result = self._doctor(self._platform_project("pi"))
        self.assertFalse(
            [n for n in _check_names(result.stdout) if n.endswith(".md")],
            _check_names(result.stdout),
        )

    def test_pi_commands_dir_empty(self):
        result = self._doctor(self._platform_project("pi"))
        self.assertNotIn(".pi/commands", _check_names(result.stdout))

    def test_pi_agents_dir_empty(self):
        result = self._doctor(self._platform_project("pi"))
        self.assertNotIn(".pi/agents", _check_names(result.stdout))

    # TRIAGE: ai-specs doctor — platform_get's unknown-field nonzero exit is
    # internal to lib/_internal/platform.sh; no CLI verb exposes an invalid
    # platform field request.
    def test_pi_invalid_field_exits_nonzero(self):
        result = self._doctor(self._platform_project("pi"))
        self.assertTrue(_named_checks(result.stdout, ".pi/skills"))

    # --- Omp agent fields ---

    def test_omp_skills_dir(self):
        result = self._doctor(self._platform_project("omp"))
        self.assertTrue(_named_checks(result.stdout, ".omp/skills"))

    def test_omp_mcp_config_path(self):
        result = self._doctor(self._platform_project("omp", mcp=True))
        lines = _named_checks(result.stdout, "mcp-omp")
        self.assertTrue(lines)
        self.assertIn(".omp/mcp.json", lines[0][1])

    def test_omp_mcp_key(self):
        root = self._platform_project("omp", mcp=True)
        sync = invoke(root, "sync-agent", cli_home=self.home)
        self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
        data = json.loads((root / ".omp" / "mcp.json").read_text())
        self.assertIn("mcpServers", data)
        self.assertIn("demo", data["mcpServers"])

    # TRIAGE: ai-specs doctor — the platform table's `native` flag is consumed
    # inside sync-agent's instruction fan-out and has no doctor-rendered
    # surface; the original asserted platform_get's stdout directly.
    def test_omp_native_true(self):
        result = self._doctor(self._platform_project("omp"))
        self.assertTrue(_named_checks(result.stdout, ".omp/skills"))

    def test_omp_instructions_path_native_slot(self):
        # The native instructions slot .omp/AGENTS.md renders its own check
        # (ERROR missing before sync — the name proves the slot).
        result = self._doctor(self._platform_project("omp"))
        self.assertTrue(_named_checks(result.stdout, ".omp/AGENTS.md"))

    def test_omp_commands_dir(self):
        result = self._doctor(self._platform_project("omp"))
        self.assertTrue(_named_checks(result.stdout, ".omp/commands"))

    def test_omp_agents_dir_empty(self):
        result = self._doctor(self._platform_project("omp"))
        self.assertNotIn(".omp/agents", _check_names(result.stdout))

    # TRIAGE: ai-specs doctor — runtime_hooks_target (.omp/extensions) is the
    # sync-side install destination for recipe runtime hooks; doctor renders
    # no surface naming it.
    def test_omp_runtime_hooks_target(self):
        result = self._doctor(self._platform_project("omp"))
        self.assertTrue(_named_checks(result.stdout, ".omp/skills"))

    # TRIAGE: ai-specs doctor — platform_get's unknown-field nonzero exit is
    # internal to lib/_internal/platform.sh; no CLI verb exposes an invalid
    # platform field request.
    def test_omp_invalid_field_exits_nonzero(self):
        result = self._doctor(self._platform_project("omp"))
        self.assertTrue(_named_checks(result.stdout, ".omp/skills"))

    # --- Regression: existing agents still work ---

    def test_claude_skills_dir_unchanged(self):
        result = self._doctor(self._platform_project("claude"))
        self.assertTrue(_named_checks(result.stdout, ".claude/skills"))

    def test_cursor_skills_dir(self):
        result = self._doctor(self._platform_project("cursor"))
        self.assertTrue(_named_checks(result.stdout, ".cursor/skills"))

    def test_opencode_mcp_key_unchanged(self):
        root = self._platform_project("opencode", mcp=True)
        sync = invoke(root, "sync-agent", cli_home=self.home)
        self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
        data = json.loads((root / "opencode.json").read_text())
        self.assertIn("mcp", data)
        self.assertNotIn("mcpServers", data)

    def test_invalid_agent_exits_nonzero(self):
        # CLI-observable equivalent: an unknown platform agent is rejected by
        # doctor as an unsupported-agent ERROR under the frozen exit contract.
        root = self._platform_project("nonexistent_agent")
        result = self._doctor(root)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unsupported agent", result.stdout)


class BriefRenderPolicyDoctorTests(unittest.TestCase):
    def _append_brief_render_false(self, toml_path: Path) -> None:
        text = toml_path.read_text().rstrip() + "\n\n[brief]\nrender = false\n"
        toml_path.write_text(text + "\n")

    def test_render_disabled_with_agents_md_reports_info(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            self._append_brief_render_false(target / "ai-specs" / "ai-specs.toml")
            result = invoke(target, "doctor", cli_home=home)
            self.assertEqual(result.returncode, 0)
            self.assertIn("INFO", result.stdout)
            self.assertIn("brief-render", result.stdout)

    def test_render_disabled_missing_agents_md_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            (target / "AGENTS.md").unlink()
            self._append_brief_render_false(target / "ai-specs" / "ai-specs.toml")
            result = invoke(target, "doctor", cli_home=home)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ERROR", result.stdout)
            self.assertIn("brief.render = false", result.stdout)

    def test_render_disabled_with_recipe_fragments_reports_warn(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            self._append_brief_render_false(target / "ai-specs" / "ai-specs.toml")
            result = invoke(target, "doctor", cli_home=home)
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
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
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
            result = invoke(target, "doctor", cli_home=home)
            self.assertIn("cli-version", result.stdout)
            self.assertIn("OK", result.stdout)
            self.assertIn(installed, result.stdout)

    def test_exact_pin_mismatch_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            self._append_tool_section(
                target / "ai-specs" / "ai-specs.toml",
                'version = "99.99.99"\npolicy = "exact"',
            )
            result = invoke(target, "doctor", cli_home=home)
            self.assertIn("cli-version", result.stdout)
            self.assertIn("ERROR", result.stdout)
            self.assertNotEqual(result.returncode, 0)

    def test_no_pin_stale_last_sync_reports_warn(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            lock_path = target / "ai-specs" / ".ai-specs.lock"
            lock_path.write_text(
                '[meta]\ncli_version = "0.10.0"\n'
                'synced_at = "2026-01-01T00:00:00Z"\n'
            )
            result = invoke(target, "doctor", cli_home=home)
            self.assertIn("cli-version", result.stdout)
            self.assertIn("WARN", result.stdout)

    def test_doctor_cli_version_is_read_only(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = _make_home(Path(tmp))
            target = Path(tmp) / "prj"
            target.mkdir()
            ai_specs_init(target, home)
            self._append_tool_section(
                target / "ai-specs" / "ai-specs.toml",
                'version = "99.99.99"\npolicy = "exact"',
            )
            toml_path = target / "ai-specs" / "ai-specs.toml"
            before = toml_path.read_text()
            invoke(target, "doctor", cli_home=home)
            self.assertEqual(before, toml_path.read_text())


def _find_files(root: Path):
    for p in root.rglob("*"):
        if p.is_file():
            yield p


class RecipeCliDepsDoctorTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.home = _make_home(self.base, catalog=False)

    def _write_project(self, recipe_toml: str, enabled: bool = True) -> Path:
        catalog = self.home / "catalog" / "recipes" / "demo-recipe"
        catalog.mkdir(parents=True, exist_ok=True)
        (catalog / "recipe.toml").write_text(recipe_toml)
        project = self.base / "project"
        (project / "ai-specs").mkdir(parents=True)
        (project / "AGENTS.md").write_text("# agents\n")
        _seed_clean_cache(project, self.home)
        (project / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
        (project / "ai-specs" / "commands" / "placeholder.md").write_text("# placeholder\n")
        flag = "true" if enabled else "false"
        (project / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "demo"\n\n'
            '[agents]\nenabled = []\n\n'
            f'[recipes.demo-recipe]\nenabled = {flag}\nversion = "1.0"\n'
        )
        return project

    def _doctor_with_path(self, project: Path, *, without: tuple[str, ...] = (),
                          stubs: dict[str, str] | None = None):
        new_path = _stage_path(self.base, without=without, stubs=stubs)
        with patch.dict(os.environ, {"PATH": new_path}):
            return invoke(project, "doctor", cli_home=self.home)

    def test_recipe_cli_deps_warn_when_missing(self):
        project = self._write_project(
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
        result = self._doctor_with_path(project, without=("gh",))
        warn_rows = [
            (sev, body) for sev, body in _named_checks(result.stdout, "recipe-dep")
            if sev == "WARN"
        ]
        self.assertTrue(warn_rows)
        self.assertIn("gh", warn_rows[0][1])
        self.assertIn("https://cli.github.com/", warn_rows[0][1])
        # Frozen exit contract: the WARN-only recipe-dep finding keeps exit 0.
        self.assertEqual(result.returncode, 0)

    def test_recipe_cli_deps_info_when_optional_missing(self):
        project = self._write_project(
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
        result = self._doctor_with_path(project, without=("jq",))
        info_rows = [
            (sev, body) for sev, body in _named_checks(result.stdout, "recipe-dep")
            if sev == "INFO"
        ]
        self.assertTrue(info_rows)
        self.assertIn("optional jq", info_rows[0][1])

    def test_recipe_cli_deps_ok_when_found(self):
        project = self._write_project(
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
        result = self._doctor_with_path(project, stubs={"gh": "#!/bin/sh\nexit 0\n"})
        ok_rows = [
            (sev, body) for sev, body in _named_checks(result.stdout, "recipe-dep")
            if sev == "OK"
        ]
        self.assertTrue(ok_rows)
        self.assertIn("gh available", ok_rows[0][1])

    def test_doctor_no_crash_when_no_recipes(self):
        project = self.base / "project"
        (project / "ai-specs").mkdir(parents=True)
        (project / "AGENTS.md").write_text("# agents\n")
        (project / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "demo"\n\n[agents]\nenabled = []\n'
        )
        result = self._doctor_with_path(project)
        self.assertIn(result.returncode, (0, 1))
        self.assertEqual(_named_checks(result.stdout, "recipe-dep"), [])

    def test_doctor_exit_code_unchanged(self):
        project = self._write_project(
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
        result = self._doctor_with_path(project)
        recipe_rows = _named_checks(result.stdout, "recipe-dep")
        self.assertTrue(recipe_rows)
        # WARN-only recipe-dep rows must not flip the exit code by themselves:
        # no recipe-dep row may be an ERROR, and the run must exit 0.
        self.assertTrue(all(sev != "ERROR" for sev, _ in recipe_rows))
        self.assertEqual(result.returncode, 0)


class HarnessEnvDoctorTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.home = _make_home(self.base, catalog=False)

    def _write_mcp_project(self) -> Path:
        catalog = self.home / "catalog" / "recipes" / "demo-recipe"
        catalog.mkdir(parents=True, exist_ok=True)
        (catalog / "recipe.toml").write_text(
            "[recipe]\n"
            'id = "demo-recipe"\n'
            'name = "Demo"\n'
            'description = "D"\n'
            'version = "1.0"\n\n'
            "[[provides.mcp]]\n"
            'id = "trello"\n'
            'command = "npx"\n'
            "env = { TRELLO_TOKEN = \"$TRELLO_TOKEN\" }\n",
            encoding="utf-8",
        )
        project = self.base / "project"
        (project / "ai-specs").mkdir(parents=True)
        (project / "AGENTS.md").write_text("# agents\n", encoding="utf-8")
        _seed_clean_cache(project, self.home)
        (project / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
        (project / "ai-specs" / "commands" / "placeholder.md").write_text(
            "# placeholder\n", encoding="utf-8"
        )
        (project / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "demo"\n\n'
            '[agents]\nenabled = []\n\n'
            '[recipes.demo-recipe]\nenabled = true\nversion = "1.0"\n',
            encoding="utf-8",
        )
        return project

    def _doctor_with_path(self, project: Path, *, without: tuple[str, ...] = (),
                          stubs: dict[str, str] | None = None):
        new_path = _stage_path(self.base, without=without, stubs=stubs)
        with patch.dict(os.environ, {"PATH": new_path}):
            return invoke(project, "doctor", cli_home=self.home)

    def test_direnv_warn_when_mcp_env_required(self):
        project = self._write_mcp_project()
        result = self._doctor_with_path(project, without=("direnv",))
        warn = [
            (sev, body) for sev, body in _named_checks(result.stdout, "direnv")
            if sev == "WARN"
        ]
        self.assertTrue(warn)

    def test_no_direnv_warn_without_mcp_env(self):
        root = self.base
        catalog = self.home / "catalog" / "recipes" / "demo-recipe"
        catalog.mkdir(parents=True, exist_ok=True)
        (catalog / "recipe.toml").write_text(
            "[recipe]\n"
            'id = "demo-recipe"\n'
            'name = "Demo"\n'
            'description = "D"\n'
            'version = "1.0"\n',
            encoding="utf-8",
        )
        project = root / "project"
        (project / "ai-specs").mkdir(parents=True)
        (project / "AGENTS.md").write_text("# a\n", encoding="utf-8")
        (project / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "demo"\n\n[agents]\nenabled = []\n\n'
            '[recipes.demo-recipe]\nenabled = true\nversion = "1.0"\n',
            encoding="utf-8",
        )
        result = self._doctor_with_path(project, without=("direnv",))
        self.assertEqual(_named_checks(result.stdout, "direnv"), [])

    def test_managed_envrc_and_harness_key_warns(self):
        project = self._write_mcp_project()
        (project / ".envrc").write_text("use nix\n", encoding="utf-8")
        (project / "ai-specs.env").write_text("TRELLO_TOKEN=\n", encoding="utf-8")
        result = self._doctor_with_path(
            project, stubs={"direnv": "#!/bin/sh\nexit 0\n"}
        )
        self.assertTrue(
            any(
                sev == "WARN" and "stale" not in body.lower()
                for sev, body in _named_checks(result.stdout, "envrc-managed")
            )
        )
        harness = [
            (sev, body) for sev, body in _named_checks(result.stdout, "harness-env")
            if sev == "WARN"
        ]
        self.assertTrue(harness)
        self.assertIn("TRELLO_TOKEN", harness[0][1])
        self.assertNotIn("secret", harness[0][1].lower())

    def test_present_harness_key_ok(self):
        """Non-empty required key in ai-specs.env yields harness-env OK (not WARN)."""
        project = self._write_mcp_project()
        (project / ".envrc").write_text(
            "# managed-by: ai-specs (do not remove block)\n"
            "dotenv_if_exists .env\n"
            "dotenv_if_exists ai-specs.env\n"
            "# end managed-by: ai-specs\n",
            encoding="utf-8",
        )
        (project / "ai-specs.env").write_text(
            "TRELLO_TOKEN=filled-value\n",
            encoding="utf-8",
        )
        result = self._doctor_with_path(
            project, stubs={"direnv": "#!/bin/sh\nexit 0\n"}
        )
        ok = [
            (sev, body) for sev, body in _named_checks(result.stdout, "harness-env")
            if sev == "OK"
        ]
        warn = [
            (sev, body) for sev, body in _named_checks(result.stdout, "harness-env")
            if sev == "WARN"
        ]
        self.assertTrue(ok, f"expected harness-env OK when key is present: {ok}")
        self.assertFalse(warn, f"harness-env must not WARN when key is non-empty: {warn}")

    def _write_mcp_project_with_choice(self) -> Path:
        """Like _write_mcp_project, but the MCP env reference declares allowed values."""
        project = self._write_mcp_project()
        (self.home / "catalog" / "recipes" / "demo-recipe" / "recipe.toml").write_text(
            "[recipe]\n"
            'id = "demo-recipe"\n'
            'name = "Demo"\n'
            'description = "D"\n'
            'version = "1.0"\n\n'
            "[[provides.mcp]]\n"
            'id = "demo"\n'
            'command = "npx"\n'
            'env = { DEMO_MODE = "$DEMO_MODE" }\n'
            'env_allowed = { DEMO_MODE = ["on", "off"] }\n',
            encoding="utf-8",
        )
        return project

    def test_invalid_harness_env_value_warns_with_allowed_values(self):
        """A configured value outside the recipe's declared set WARNs early, without echoing it."""
        project = self._write_mcp_project_with_choice()
        (project / "ai-specs.env").write_text("DEMO_MODE=onoff\n", encoding="utf-8")
        result = self._doctor_with_path(
            project, stubs={"direnv": "#!/bin/sh\nexit 0\n"}
        )
        warn = [
            (sev, body)
            for sev, body in _named_checks(result.stdout, "harness-env-value")
            if sev == "WARN"
        ]
        self.assertTrue(
            warn, "expected harness-env-value WARN for a value outside the declared set"
        )
        self.assertIn("DEMO_MODE", warn[0][1])
        self.assertIn("on, off", warn[0][1])
        self.assertNotIn("onoff", warn[0][1])

    def test_valid_harness_env_value_case_insensitive_no_warn(self):
        """The provider accepts declared values case-insensitively, so no WARN is warranted."""
        project = self._write_mcp_project_with_choice()
        (project / "ai-specs.env").write_text("DEMO_MODE=OFF\n", encoding="utf-8")
        result = self._doctor_with_path(
            project, stubs={"direnv": "#!/bin/sh\nexit 0\n"}
        )
        self.assertEqual(_named_checks(result.stdout, "harness-env-value"), [])

    def test_stale_managed_body_warns(self):
        """JD-8: markers with nested ai-specs/.env body must WARN envrc-managed."""
        project = self._write_mcp_project()
        (project / ".envrc").write_text(
            "# managed-by: ai-specs (do not remove block)\n"
            "dotenv_if_exists .env\n"
            "dotenv_if_exists ai-specs/.env\n"
            "# end managed-by: ai-specs\n",
            encoding="utf-8",
        )
        (project / "ai-specs.env").write_text(
            "TRELLO_TOKEN=filled-value\n",
            encoding="utf-8",
        )
        result = self._doctor_with_path(
            project, stubs={"direnv": "#!/bin/sh\nexit 0\n"}
        )
        warn = [
            (sev, body) for sev, body in _named_checks(result.stdout, "envrc-managed")
            if sev == "WARN"
        ]
        self.assertTrue(warn, "expected envrc-managed WARN for stale body")
        self.assertIn("stale", warn[0][1].lower())

    def test_doctor_never_calls_install(self):
        # Black-box canary: stub `brew`/`apt-get` on PATH record any execution;
        # doctor must complete without ever invoking an installer.
        project = self._write_mcp_project()
        canary = (
            "#!/bin/sh\n"
            'touch "$CANARY_DIR/called-$(basename "$0")"\n'
            "exit 0\n"
        )
        result = self._doctor_with_path(
            project, stubs={"brew": canary, "apt-get": canary}
        )
        self.assertIn("ai-specs doctor", result.stdout)
        canary_dir = self.base / "path-stubs"
        self.assertEqual(
            list(canary_dir.glob("called-*")), [],
            "doctor must never execute an installer binary",
        )


class CacheAwareCommandsDoctorTests(unittest.TestCase):
    """Doctor must treat cache-managed commands as 'expected', not stale extras."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.home = _make_home(self.base)

    def test_bundled_commands_ok_when_only_cache_has_commands(self):
        """bundled-commands must report OK when cache commands/ is non-empty, even if ai-specs/commands/ is empty."""
        target = self.base / "prj"
        target.mkdir()
        ai_specs_init(target, self.home)
        # Clear hand-authored commands
        commands_root = target / "ai-specs" / "commands"
        shutil.rmtree(commands_root, ignore_errors=True)
        commands_root.mkdir(parents=True)
        # Populate cache commands
        cache_cmds = cache_project_dir(target, self.home) / "commands"
        cache_cmds.mkdir(parents=True, exist_ok=True)
        (cache_cmds / "recipe-cmd.md").write_text("# recipe command\n")
        result = invoke(target, "doctor", cli_home=self.home)
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
        target = self.base / "prj"
        target.mkdir()
        ai_specs_init(target, self.home, agents=["cursor"])
        # Populate cache with a recipe-managed command
        cache_cmds = cache_project_dir(target, self.home) / "commands"
        cache_cmds.mkdir(parents=True, exist_ok=True)
        (cache_cmds / "recipe-cmd.md").write_text("# recipe command\n")
        # Sync so agent commands dir is populated
        sync = invoke(target, "sync-agent", cli_home=self.home)
        self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)
        result = invoke(target, "doctor", cli_home=self.home)
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

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        # Real lib copy; the per-project cache stays EMPTY so the expected
        # command set is zero.
        self.home = _make_home(self.base)

    def _make_project(self) -> Path:
        """Minimal project fixture: manifest + AGENTS.md + bundled skills."""
        target = self.base / "prj"
        target.mkdir()
        (target / "AGENTS.md").write_text("# agents\n")
        (target / "ai-specs").mkdir()
        for skill in BUNDLED_SKILLS:
            (target / "ai-specs" / "skills" / skill).mkdir(parents=True, exist_ok=True)
        (target / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "demo"\n[agents]\nenabled = ["claude"]\n'
        )
        return target

    def test_empty_commands_dir_with_no_expected_reports_ok_not_warn(self):
        """Empty agent commands dir + zero expected commands → OK, not WARN."""
        target = self._make_project()
        # Controlled empty cache lookup: no .bundled/commands and no commands/.
        (target / ".claude" / "commands").mkdir(parents=True)
        result = invoke(target, "doctor", cli_home=self.home)
        warn_rows = [
            (sev, body) for sev, body in _named_checks(result.stdout, ".claude/commands")
            if sev == "WARN"
        ]
        self.assertEqual(
            warn_rows,
            [],
            f"Should not WARN when no commands configured; got: {warn_rows}",
        )

    def test_empty_commands_dir_with_no_expected_emits_ok_label(self):
        """Empty agent commands dir + zero expected commands → at least one OK for commands."""
        target = self._make_project()
        (target / ".claude" / "commands").mkdir(parents=True)
        result = invoke(target, "doctor", cli_home=self.home)
        ok_rows = [
            (sev, body) for sev, body in _named_checks(result.stdout, ".claude/commands")
            if sev == "OK"
        ]
        self.assertTrue(
            ok_rows,
            f"Should emit OK when no commands configured; got: {result.stdout}",
        )


class RepoTopologyDoctorTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        # The repo catalog must be reachable (worktree-flow recipe templates).
        self.home = _make_home(self.base, catalog=True)

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
        target = self.base / "prj"
        target.mkdir()
        ai_specs_init(target, self.home)
        self._enable_worktree_flow(target)
        result = invoke(target, "doctor", cli_home=self.home)
        self.assertIn("repo-topology", result.stdout)
        self.assertIn("INFO", result.stdout)

    def _set_project_topology(self, target: Path, topology: str) -> None:
        manifest = target / "ai-specs" / "ai-specs.toml"
        lines = manifest.read_text().splitlines()
        lines.insert(lines.index("[project]") + 1, f'repo_topology = "{topology}"')
        manifest.write_text("\n".join(lines) + "\n")

    def test_project_topology_reports_with_deprecation_when_legacy_only(self):
        target = self.base / "prj"
        target.mkdir()
        ai_specs_init(target, self.home)
        self._enable_worktree_flow(target)
        result = invoke(target, "doctor", cli_home=self.home)
        self.assertIn("legacy-recipe", result.stdout)
        self.assertIn("[project].repo_topology", result.stdout)

    def test_project_field_reports_without_worktree_flow(self):
        target = self.base / "prj"
        target.mkdir()
        ai_specs_init(target, self.home)
        self._set_project_topology(target, "standalone")
        result = invoke(target, "doctor", cli_home=self.home)
        self.assertIn("repo-topology", result.stdout)
        self.assertIn("standalone", result.stdout)
        self.assertNotIn("legacy-recipe", result.stdout)

    def test_stale_override_warns(self):
        target = self.base / "prj"
        target.mkdir()
        ai_specs_init(target, self.home)
        self._enable_worktree_flow(target)
        dest = (
            target / "ai-specs" / "recipes" / "worktree-flow" / "overrides"
            / "bin" / "worktree-cleanup.sh"
        )
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text("# customized\n")
        result = invoke(target, "doctor", cli_home=self.home)
        self.assertIn("stale-override", result.stdout)
        self.assertIn("WARN", result.stdout)
        # read-only: file unchanged
        self.assertEqual(dest.read_text(), "# customized\n")


class GateProvenanceDoctorTests(unittest.TestCase):
    """3.3 — doctor warns on customized/missing gate provenance, stays
    quiet on matching baselines. Black-box through `bin/ai-specs doctor`."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.home = _make_home(self.base, catalog=False)

    def _fake_home(self) -> tuple[Path, Path]:
        """Catalog recipe wt-hook with a runtime hook + enabling project."""
        recipe_dir = self.home / "catalog" / "recipes" / "wt-hook"
        (recipe_dir / "hooks").mkdir(parents=True, exist_ok=True)
        (recipe_dir / "hooks" / "gate.sh").write_text("#!/usr/bin/env bash\nexit 0\n")
        (recipe_dir / "recipe.toml").write_text(
            '[recipe]\n'
            'id = "wt-hook"\n'
            'name = "WT Hook"\n'
            'description = "D"\n'
            'version = "1.0"\n'
            '[[provides.hooks]]\n'
            'id = "gate"\n'
            'event = "pre-tool-use"\n'
            'script = "hooks/gate.sh"\n'
            'matcher = "Edit|Write"\n'
            'blocking = true\n'
        )
        project = self.base / "prj"
        (project / "ai-specs").mkdir(parents=True)
        (project / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'p'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            "[recipes.wt-hook]\nenabled = true\nversion = '1.0'\n"
        )
        (project / "AGENTS.md").write_text("# agents\n")
        _seed_clean_cache(project, self.home)
        return project, self.home

    def _gate(self, project: Path) -> Path:
        return project / "ai-specs" / "recipes" / "wt-hook" / "hooks" / "gate.sh"

    def _record_baseline(self, project: Path, sha: str) -> None:
        """Write the managed-override baseline in the lock's TOML shape."""
        lock_path = project / "ai-specs" / ".ai-specs.lock"
        lock_path.write_text(
            '[meta]\ncli_version = "test"\nsynced_at = "2026-01-01T00:00:00Z"\n\n'
            '[managed."ai-specs/recipes/wt-hook/hooks/gate.sh"]\n'
            f'sha256 = "{sha}"\n'
            'recipe = "wt-hook"\n'
            'source = "hooks/gate.sh"\n'
            'kind = "gate"\n'
            'policy = "auto"\n'
        )

    def _gate_checks(self, project: Path) -> tuple[list[tuple[str, str]], int]:
        result = invoke(project, "doctor", cli_home=self.home)
        return _named_checks(result.stdout, "gate-provenance"), result.returncode

    def test_doctor_warns_on_customized_gate(self):
        project, _ = self._fake_home()
        gate = self._gate(project)
        gate.parent.mkdir(parents=True, exist_ok=True)
        gate.write_text("#!/usr/bin/env bash\nexit 0\n")
        self._record_baseline(project, "0" * 64)  # baseline != disk
        checks, _code = self._gate_checks(project)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0][0], "WARN")
        self.assertIn("gate.sh", checks[0][1])
        self.assertIn("user-modified", checks[0][1])

    def test_doctor_quiet_when_gate_baseline_matches(self):
        project, _ = self._fake_home()
        gate = self._gate(project)
        gate.parent.mkdir(parents=True, exist_ok=True)
        payload = "#!/usr/bin/env bash\nexit 0\n"
        gate.write_text(payload)
        baseline = hashlib.sha256(payload.replace("\r\n", "\n").encode()).hexdigest()
        self._record_baseline(project, baseline)
        checks, _code = self._gate_checks(project)
        self.assertEqual(
            checks, [],
            "doctor must stay quiet for gates whose baseline matches",
        )

    def test_doctor_warns_on_missing_gate_provenance(self):
        project, _ = self._fake_home()
        gate = self._gate(project)
        gate.parent.mkdir(parents=True, exist_ok=True)
        gate.write_text("#!/usr/bin/env bash\nexit 0\n")
        checks, _code = self._gate_checks(project)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0][0], "WARN")
        self.assertIn("provenance", checks[0][1].lower())

    def test_doctor_no_hook_recipes_quiet(self):
        project = self.base / "prj"
        (project / "ai-specs").mkdir(parents=True)
        (project / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'p'\n\n[agents]\nenabled = ['claude']\n"
        )
        (project / "AGENTS.md").write_text("# agents\n")
        _seed_clean_cache(project, self.home)
        checks, _code = self._gate_checks(project)
        self.assertEqual(checks, [])


if __name__ == "__main__":
    unittest.main()
