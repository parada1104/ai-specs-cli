"""Tests for always-on harness CLI literacy skills + AGENTS.md pointer."""

from __future__ import annotations

import importlib.util
import json
import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BUNDLED = ROOT / "bundled-skills"
AGENTS_RENDER = ROOT / "lib" / "_internal" / "agents-render.py"
SKILL_CONTRACT = ROOT / "lib" / "_internal" / "skill_contract.py"
REFRESH_BUNDLED = ROOT / "lib" / "_internal" / "refresh-bundled.py"
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


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


def public_cli_commands() -> set[str]:
    """Parse top-level command names from `ai-specs help`."""
    text = AI_SPECS_BIN.read_text()
    # help heredoc lists: "  init [path] ..." etc.
    cmds: set[str] = set()
    in_help = False
    for line in text.splitlines():
        if line.startswith("help|-h|--help)"):
            in_help = True
            continue
        if in_help and line.strip() == "EOF":
            break
        if not in_help:
            continue
        m = re.match(r"\s{2}([a-z][a-z0-9-]*)\b", line)
        if m:
            cmds.add(m.group(1))
    # Bare hub is routed when no subcommand; still a public entry.
    cmds.add("hub")
    return cmds


class HarnessCliLiteracyTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.contract = load_module(SKILL_CONTRACT, "skill_contract_harness_literacy")

    def test_bundled_harness_skills_exist(self):
        for name in HARNESS_SKILLS:
            path = BUNDLED / name / "SKILL.md"
            self.assertTrue(path.is_file(), f"missing {path.relative_to(ROOT)}")

    def test_harness_skills_frontmatter_valid(self):
        for name in HARNESS_SKILLS:
            path = BUNDLED / name / "SKILL.md"
            skill = self.contract.from_local_skill(path, compatibility=False)
            meta = self.contract.validate_sync_metadata(skill, path=path)
            self.assertTrue(meta["enabled"], name)
            self.assertIn("root", meta["scope"], name)
            self.assertTrue(meta["auto_invoke"], name)

    def test_refresh_bundled_flattens_harness_skills_to_cache(self):
        pc = load_module(
            ROOT / "lib" / "_internal" / "project-cache.py", "project_cache_literacy"
        )
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp)
            (project / "ai-specs").mkdir()
            (project / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "literacy-fixture"\n\n[agents]\nenabled = ["claude"]\n'
            )
            proc = subprocess.run(
                [sys.executable, str(REFRESH_BUNDLED), str(project), str(ROOT)],
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr or proc.stdout)
            bundled_root = pc.bundled_skills_root(project, cli_home=ROOT) / "skills"
            for name in HARNESS_SKILLS:
                self.assertTrue(
                    (bundled_root / name / "SKILL.md").is_file(),
                    f"not flattened to cache: {name}",
                )
                # CLI-bundled skills must NOT leak into the committed project surface.
                self.assertFalse(
                    (project / "ai-specs" / "skills" / name).exists(),
                    f"leaked into project surface: {name}",
                )

    def test_refresh_bundled_migrates_inproject_copy_via_lock_hash(self):
        """An untouched in-project bundled copy from an older CLI (differs from
        source, but recorded in the legacy lock) is removed on refresh-bundled —
        while the legacy [skills.*] hashes are still in memory."""
        import hashlib
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp)
            (project / "ai-specs" / "skills" / "harness-lifecycle").mkdir(parents=True)
            (project / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "mig"\n\n[agents]\nenabled = ["claude"]\n'
            )
            old = project / "ai-specs" / "skills" / "harness-lifecycle" / "SKILL.md"
            content = "# harness-lifecycle (older CLI, untouched)\n"
            old.write_text(content)
            h = hashlib.sha256(content.encode()).hexdigest()
            (project / "ai-specs" / ".ai-specs.lock").write_text(
                f'[skills."harness-lifecycle"]\n"SKILL.md" = "{h}"\n'
            )
            proc = subprocess.run(
                [sys.executable, str(REFRESH_BUNDLED), str(project), str(ROOT)],
                text=True, capture_output=True, check=False,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr or proc.stdout)
            self.assertFalse(
                (project / "ai-specs" / "skills" / "harness-lifecycle").exists(),
                "untouched in-project copy should be migrated out via lock hash",
            )

    def test_refresh_bundled_init_flattens_commands_without_writing_project(self):
        pc = load_module(
            ROOT / "lib" / "_internal" / "project-cache.py", "project_cache_literacy_cmd"
        )
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp)
            (project / "ai-specs" / "commands").mkdir(parents=True)
            (project / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "literacy-cmd-fixture"\n\n[agents]\nenabled = ["claude"]\n'
            )
            proc = subprocess.run(
                [sys.executable, str(REFRESH_BUNDLED), str(project), str(ROOT), "--init"],
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr or proc.stdout)
            bundled_root = pc.bundled_commands_root(project, cli_home=ROOT)
            self.assertTrue((bundled_root / "rules-audit.md").is_file())
            self.assertTrue((bundled_root / "skills-as-rules.md").is_file())
            self.assertEqual(
                sorted(p.name for p in (project / "ai-specs" / "commands").glob("*.md")),
                [],
                "bundled commands must not be written into ai-specs/commands/",
            )
            self.assertFalse(
                list((project / "ai-specs" / "commands").glob("*.new")),
                "refresh-bundled must never write .new sidecars",
            )

    def test_refresh_bundled_removes_byte_identical_command_leftover(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp)
            (project / "ai-specs" / "commands").mkdir(parents=True)
            (project / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "leftover-cmd"\n\n[agents]\nenabled = ["claude"]\n'
            )
            leftover = project / "ai-specs" / "commands" / "rules-audit.md"
            leftover.write_text((ROOT / "bundled-commands" / "rules-audit.md").read_text())
            proc = subprocess.run(
                [sys.executable, str(REFRESH_BUNDLED), str(project), str(ROOT)],
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr or proc.stdout)
            self.assertFalse(leftover.exists(), "byte-identical bundled command must be removed")
            self.assertFalse((project / "ai-specs" / "commands" / "rules-audit.md.new").exists())

    def test_refresh_bundled_keeps_customized_command_with_notice(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp)
            (project / "ai-specs" / "commands").mkdir(parents=True)
            (project / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "customized-cmd"\n\n[agents]\nenabled = ["claude"]\n'
            )
            customized = project / "ai-specs" / "commands" / "rules-audit.md"
            customized.write_text("# rules-audit (locally edited)\n")
            proc = subprocess.run(
                [sys.executable, str(REFRESH_BUNDLED), str(project), str(ROOT)],
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr or proc.stdout)
            self.assertTrue(customized.exists(), "customized copy must be preserved")
            out = proc.stdout + proc.stderr
            self.assertIn("customized", out)
            self.assertFalse((project / "ai-specs" / "commands" / "rules-audit.md.new").exists())

    def test_agents_render_emits_harness_literacy_pointer(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            toml_path = tmp_path / "ai-specs.toml"
            output_path = tmp_path / "AGENTS.md"
            resolved_path = tmp_path / "resolved.json"
            toml_path.write_text(
                "[project]\n"
                'name = "literacy-pointer"\n\n'
                "[agents]\n"
                'enabled = ["claude"]\n'
            )
            resolved_path.write_text(
                json.dumps(
                    {
                        "project": {"name": "literacy-pointer"},
                        "agents": {"enabled": ["claude"]},
                        "recipes": {},
                        "bindings": {},
                        "enabled": [],
                    }
                )
            )
            proc = subprocess.run(
                [
                    sys.executable,
                    str(AGENTS_RENDER),
                    str(toml_path),
                    str(output_path),
                    "--resolved-config",
                    str(resolved_path),
                ],
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            text = output_path.read_text()
            self.assertIn("## Useful Commands", text)
            for needle in POINTER_NEEDLES:
                self.assertIn(needle, text)
            self.assertRegex(
                text,
                r"[Hh]arness|ai-specs harness|harness operations",
            )
            # Bundled harness skills resolve via agent fan-out / cache — never
            # claim they live on the committed project surface.
            self.assertNotIn("under `ai-specs/skills/`", text)
            self.assertNotIn("under ai-specs/skills/", text)

    def test_harness_lifecycle_documents_cache_flatten(self):
        text = (BUNDLED / "harness-lifecycle" / "SKILL.md").read_text()
        self.assertRegex(text, r"\.bundled|cache.*bundled|flatten", re.I)
        self.assertNotIn("SKILL.md.new", text)
        self.assertNotRegex(
            text,
            r"First install copies into `ai-specs/skills/",
        )

    def test_refresh_prints_tracked_leftover_remediation(self):
        """After removing leftovers, refresh prints git rm --cached guidance."""
        pc = load_module(
            ROOT / "lib" / "_internal" / "project-cache.py", "project_cache_lit_track"
        )
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp)
            subprocess.run(["git", "init", "-q", str(project)], check=True)
            subprocess.run(
                ["git", "-C", str(project), "config", "user.email", "t@example.com"],
                check=True,
            )
            subprocess.run(
                ["git", "-C", str(project), "config", "user.name", "t"],
                check=True,
            )
            (project / "ai-specs").mkdir()
            (project / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "lit-track"\n\n[agents]\nenabled = ["claude"]\n'
            )
            leftover = project / "ai-specs" / "skills" / "skill-creator"
            leftover.mkdir(parents=True)
            (leftover / "SKILL.md").write_text(
                (BUNDLED / "skill-creator" / "SKILL.md").read_text()
            )
            subprocess.run(["git", "-C", str(project), "add", "-A"], check=True)
            subprocess.run(
                ["git", "-C", str(project), "commit", "-qm", "track leftover"],
                check=True,
            )
            proc = subprocess.run(
                [sys.executable, str(REFRESH_BUNDLED), str(project), str(ROOT)],
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr or proc.stdout)
            self.assertFalse(leftover.exists())
            out = proc.stdout + proc.stderr
            self.assertIn("git rm -r --cached", out)
            self.assertIn("skill-creator", out)
            tracked = subprocess.run(
                ["git", "-C", str(project), "ls-files", "ai-specs/skills/skill-creator"],
                capture_output=True, text=True, check=True,
            ).stdout
            self.assertIn("SKILL.md", tracked)
            # Helper agrees with the printed guidance.
            ids = pc.tracked_bundled_skill_leftovers(project, cli_home=ROOT)
            self.assertIn("skill-creator", ids)

    def test_harness_skill_commands_match_cli_help(self):
        known = public_cli_commands()
        self.assertIn("sync", known)
        self.assertIn("recipe", known)
        # Only real invocations: `ai-specs <cmd>` or a line starting with ai-specs.
        pattern = re.compile(
            r"(?:`ai-specs\s+([a-z][a-z0-9-]*)|^ai-specs\s+([a-z][a-z0-9-]*))",
            re.MULTILINE,
        )
        unknown: list[str] = []
        for name in HARNESS_SKILLS:
            path = BUNDLED / name / "SKILL.md"
            self.assertTrue(path.is_file(), f"missing {path}")
            for match in pattern.finditer(path.read_text()):
                cmd = match.group(1) or match.group(2)
                if cmd not in known:
                    unknown.append(f"{name}:{cmd}")
        self.assertEqual(unknown, [], f"unknown CLI commands in literacy skills: {unknown}")


if __name__ == "__main__":
    unittest.main()
