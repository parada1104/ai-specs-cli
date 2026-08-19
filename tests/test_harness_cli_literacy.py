"""Black-box tests for always-on harness CLI literacy skills."""

from __future__ import annotations

import hashlib
import re
import subprocess
import tempfile
import unittest
from pathlib import Path

from _blackbox import cache_project_dir, invoke, isolated_home, temp_project

ROOT = Path(__file__).resolve().parents[1]
BUNDLED = ROOT / "bundled-skills"
AI_SPECS_BIN = ROOT / "bin" / "ai-specs"

HARNESS_SKILLS = (
    "harness-lifecycle",
    "harness-recipes",
    "harness-skills-deps",
)

POINTER_NEEDLES = (
    "harness-lifecycle",
    "harness-recipes",
    "harness-skills-deps",
)


def public_cli_commands() -> set[str]:
    """Parse top-level command names from the real CLI help output."""
    result = subprocess.run(
        [str(AI_SPECS_BIN), "help"], text=True, capture_output=True, check=True
    )
    commands: set[str] = set()
    in_commands = False
    for line in result.stdout.splitlines():
        if line.startswith("Commands:"):
            in_commands = True
            continue
        if in_commands and line.startswith("Path defaults"):
            break
        if not in_commands:
            continue
        match = re.match(r"\s{2}([a-z][a-z0-9-]*)\b", line)
        if match:
            commands.add(match.group(1))
    # Bare invocation is routed to hub even though help lists no bare command.
    commands.add("hub")
    return commands


class HarnessCliLiteracyTests(unittest.TestCase):
    def _project(self, *, name: str = "literacy-fixture") -> tuple[Path, Path]:
        project_td, project = temp_project(name=name, agents=("claude",))
        self.addCleanup(project_td.cleanup)
        home_td = tempfile.TemporaryDirectory(prefix="literacy-home-")
        self.addCleanup(home_td.cleanup)
        return project, isolated_home(Path(home_td.name))

    def test_bundled_harness_skills_exist(self):
        project, home = self._project()
        result = invoke(project, "refresh-bundled", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        cache_skills = cache_project_dir(project, home) / ".bundled" / "skills"
        for name in HARNESS_SKILLS:
            path = cache_skills / name / "SKILL.md"
            self.assertTrue(path.is_file(), f"missing cached {path.relative_to(cache_skills)}")
            self.assertIn(f"name: {name}", path.read_text())

    def test_harness_skills_frontmatter_valid(self):
        project, home = self._project()
        result = invoke(project, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        for name in HARNESS_SKILLS:
            path = project / ".claude" / "skills" / name / "SKILL.md"
            self.assertTrue(path.is_file(), f"skill was not exposed: {name}")
            text = path.read_text()
            self.assertIn("scope: [root]", text, name)
            self.assertRegex(text, r"(?m)^  auto_invoke:\s*$", name)
            self.assertRegex(text, rf"(?m)^name: {re.escape(name)}$", name)

    def test_refresh_bundled_flattens_harness_skills_to_cache(self):
        project, home = self._project()
        result = invoke(project, "refresh-bundled", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        cache_skills = cache_project_dir(project, home) / ".bundled" / "skills"
        for name in HARNESS_SKILLS:
            self.assertTrue((cache_skills / name / "SKILL.md").is_file())
            self.assertFalse((project / "ai-specs" / "skills" / name).exists())
        self.assertIn("flattened", result.stdout)

    def test_refresh_bundled_migrates_inproject_copy_via_lock_hash(self):
        project, home = self._project(name="mig")
        old = project / "ai-specs" / "skills" / "harness-lifecycle" / "SKILL.md"
        old.parent.mkdir(parents=True)
        content = "# harness-lifecycle (older CLI, untouched)\n"
        old.write_text(content)
        lock = project / "ai-specs" / ".ai-specs.lock"
        lock.write_text(
            '[skills."harness-lifecycle"]\n"SKILL.md" = '
            f'"{hashlib.sha256(content.encode()).hexdigest()}"\n'
        )
        result = invoke(project, "refresh-bundled", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(old.parent.exists(), "untouched legacy copy should be migrated")
        self.assertTrue(
            (cache_project_dir(project, home) / ".bundled" / "skills" / "harness-lifecycle" / "SKILL.md").is_file()
        )
        self.assertIn("flattened", result.stdout)

    def test_refresh_bundled_init_flattens_commands_without_writing_project(self):
        project, home = self._project(name="literacy-cmd-fixture")
        result = invoke(project, "refresh-bundled", "--init", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        bundled = cache_project_dir(project, home) / ".bundled" / "commands"
        self.assertTrue((bundled / "rules-audit.md").is_file())
        self.assertTrue((bundled / "skills-as-rules.md").is_file())
        self.assertEqual(list((project / "ai-specs" / "commands").glob("*.md")), [])
        self.assertEqual(list((project / "ai-specs" / "commands").glob("*.new")), [])

    def test_refresh_bundled_removes_byte_identical_command_leftover(self):
        project, home = self._project(name="leftover-cmd")
        leftover = project / "ai-specs" / "commands" / "rules-audit.md"
        leftover.write_text((ROOT / "bundled-commands" / "rules-audit.md").read_text())
        result = invoke(project, "refresh-bundled", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(leftover.exists(), "byte-identical bundled command must be removed")
        self.assertFalse((leftover.parent / "rules-audit.md.new").exists())
        self.assertTrue((cache_project_dir(project, home) / ".bundled" / "commands" / leftover.name).is_file())

    def test_refresh_bundled_keeps_customized_command_with_notice(self):
        project, home = self._project(name="customized-cmd")
        customized = project / "ai-specs" / "commands" / "rules-audit.md"
        customized.write_text("# rules-audit (locally edited)\n")
        result = invoke(project, "refresh-bundled", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(customized.exists(), "customized copy must be preserved")
        self.assertIn("customized", result.stderr + result.stdout)
        self.assertFalse((customized.parent / "rules-audit.md.new").exists())

    def test_agents_render_emits_harness_literacy_pointer(self):
        project, home = self._project(name="literacy-pointer")
        result = invoke(project, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        text = (project / "AGENTS.md").read_text()
        self.assertIn("## Useful Commands", text)
        for needle in POINTER_NEEDLES:
            self.assertIn(needle, text)
        self.assertRegex(text, r"[Hh]arness|ai-specs harness|harness operations")
        self.assertNotIn("under `ai-specs/skills/`", text)
        self.assertNotIn("under ai-specs/skills/", text)

    def test_harness_lifecycle_documents_cache_flatten(self):
        text = (BUNDLED / "harness-lifecycle" / "SKILL.md").read_text()
        self.assertRegex(text, r"\.bundled|cache.*bundled|flatten", re.I)
        self.assertNotIn("SKILL.md.new", text)
        self.assertNotRegex(text, r"First install copies into `ai-specs/skills/")

    def test_refresh_prints_tracked_leftover_remediation(self):
        project, home = self._project(name="lit-track")
        leftover = project / "ai-specs" / "skills" / "skill-creator"
        leftover.mkdir(parents=True)
        (leftover / "SKILL.md").write_text((BUNDLED / "skill-creator" / "SKILL.md").read_text())
        subprocess.run(["git", "init", "-q", str(project)], check=True)
        subprocess.run(["git", "-C", str(project), "config", "user.email", "t@example.com"], check=True)
        subprocess.run(["git", "-C", str(project), "config", "user.name", "t"], check=True)
        subprocess.run(["git", "-C", str(project), "add", "-A"], check=True)
        subprocess.run(["git", "-C", str(project), "commit", "-qm", "track leftover"], check=True)
        result = invoke(project, "refresh-bundled", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(leftover.exists())
        self.assertIn("git rm -r --cached", result.stdout + result.stderr)
        self.assertIn("skill-creator", result.stdout + result.stderr)
        tracked = subprocess.run(
            ["git", "-C", str(project), "ls-files", "ai-specs/skills/skill-creator"],
            capture_output=True, text=True, check=True,
        ).stdout
        self.assertIn("SKILL.md", tracked)

    def test_harness_skill_commands_match_cli_help(self):
        known = public_cli_commands()
        self.assertIn("sync", known)
        self.assertIn("recipe", known)
        pattern = re.compile(
            r"(?:`ai-specs\s+([a-z][a-z0-9-]*)|^ai-specs\s+([a-z][a-z0-9-]*))",
            re.MULTILINE,
        )
        unknown: list[str] = []
        for name in HARNESS_SKILLS:
            path = BUNDLED / name / "SKILL.md"
            self.assertTrue(path.is_file(), f"missing {path}")
            for match in pattern.finditer(path.read_text()):
                command = match.group(1) or match.group(2)
                if command not in known:
                    unknown.append(f"{name}:{command}")
        self.assertEqual(unknown, [], f"unknown CLI commands in literacy skills: {unknown}")

    def test_harness_recipes_documents_assisted_configure_contract(self):
        text = (BUNDLED / "harness-recipes" / "SKILL.md").read_text()
        for needle in ("recipe configure", "inspect", "recommend", "approval", "--sync", "preserve", "report"):
            self.assertIn(needle, text.lower(), needle)
        self.assertIn("no-secret", text.lower())

    def test_lifecycle_cross_links_noninteractive_helper(self):
        text = (BUNDLED / "harness-lifecycle" / "SKILL.md").read_text().lower()
        self.assertIn("recipe configure", text)


if __name__ == "__main__":
    unittest.main()
