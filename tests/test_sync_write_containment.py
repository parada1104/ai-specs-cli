"""Judgment Day containment regressions: a redirected CLI cache never redirects
a write outside the CLI home.

Each test drives the native CLI through its process boundary, plants a symlink
in the per-project cache (commands, recipe skills, gate backups) and proves the
run refuses with rc 1 while leaving the project, the CLI home and the outside
target byte-identical.
"""

from __future__ import annotations

import shutil
import tempfile
import unittest
from pathlib import Path

from _blackbox import (
    cache_project_dir,
    invoke,
    isolated_home,
    populate_catalog,
    snapshot,
)

COMMAND_RECIPE = """\
[recipe]
id = "jd-cmd"
name = "jd-cmd"
description = "containment fixture"
version = "1.0.0"

[provides]
commands = [{ id = "jd-demo", path = "commands/jd-demo.md" }]
"""

SKILL_RECIPE = """\
[recipe]
id = "jd-skill"
name = "jd-skill"
description = "containment fixture"
version = "1.0.0"

[provides]
skills = [{ id = "jd-skill-a", source = "bundled" }]
"""


class _ContainmentFixture(unittest.TestCase):
    def setUp(self) -> None:
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-jd-contain-")
        self.addCleanup(tmp.cleanup)
        self.base = Path(tmp.name).resolve()
        self.home = isolated_home(self.base)
        self.project = self.base / "project"
        for name in ("skills", "commands"):
            (self.project / "ai-specs" / name).mkdir(parents=True)
        (self.project / "AGENTS.md").write_text("# brief\n", encoding="utf-8")

    def write_manifest(self, recipes: str) -> None:
        (self.project / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'jd'\n\n[agents]\nenabled = ['claude']\n\n"
            + recipes,
            encoding="utf-8",
        )

    def sync(self, *args: str):
        return invoke(self.project, "sync", *args, cli_home=self.home)

    def seed_command_recipe(self) -> Path:
        rdir = populate_catalog(self.home, "jd-cmd", COMMAND_RECIPE)
        (rdir / "commands").mkdir()
        (rdir / "commands" / "jd-demo.md").write_text("# jd-demo\n", encoding="utf-8")
        self.write_manifest("[recipes.jd-cmd]\nenabled = true\nversion = \"1.0.0\"\n")
        return cache_project_dir(self.project, self.home) / "commands" / "jd-demo.md"

    def seed_skill_recipe(self) -> Path:
        rdir = populate_catalog(self.home, "jd-skill", SKILL_RECIPE)
        (rdir / "skills" / "jd-skill-a").mkdir(parents=True)
        (rdir / "skills" / "jd-skill-a" / "SKILL.md").write_text(
            "# jd-skill-a\n", encoding="utf-8"
        )
        self.write_manifest("[recipes.jd-skill]\nenabled = true\nversion = \"1.0.0\"\n")
        return cache_project_dir(self.project, self.home) / ".recipe" / "jd-skill" / "skills"


class CacheCommandContainmentTests(_ContainmentFixture):
    def test_symlinked_live_cache_command_refuses_before_writing(self) -> None:
        cmd = self.seed_command_recipe()
        self.assertEqual(self.sync().returncode, 0)

        outside = self.base / "outside"
        outside.mkdir()
        target = outside / "jd-demo.md"
        target.write_text("# outside\n", encoding="utf-8")
        cmd.unlink()
        cmd.symlink_to(target)

        before = [snapshot(root) for root in (self.project, self.home, outside)]
        proc = self.sync()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("refuse to write", proc.stderr)
        self.assertEqual(
            [snapshot(root) for root in (self.project, self.home, outside)], before
        )
        self.assertEqual(target.read_text(encoding="utf-8"), "# outside\n")


class CacheRecipeSkillsContainmentTests(_ContainmentFixture):
    def test_symlinked_recipe_skills_dir_refuses_standalone_sync_agent(self) -> None:
        skills = self.seed_skill_recipe()
        proc = self.sync()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertTrue((skills / "jd-skill-a" / "SKILL.md").is_file())

        outside = self.base / "outside"
        outside.mkdir()
        real = outside / "skills"
        real.mkdir()
        (real / "jd-skill-a").mkdir()
        (real / "jd-skill-a" / "SKILL.md").write_text("# outside\n", encoding="utf-8")
        shutil.rmtree(skills)
        skills.symlink_to(real, target_is_directory=True)

        before = [snapshot(root) for root in (self.project, self.home, outside)]
        proc = invoke(self.project, "sync-agent", cli_home=self.home)
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("refuse to write", proc.stderr)
        self.assertEqual(
            [snapshot(root) for root in (self.project, self.home, outside)], before
        )
        self.assertEqual(
            (real / "jd-skill-a" / "SKILL.md").read_text(encoding="utf-8"), "# outside\n"
        )


class GateBackupContainmentTests(_ContainmentFixture):
    def test_symlinked_gate_backups_dir_refuses_refresh_gates(self) -> None:
        self.write_manifest("[recipes.worktree-flow]\nenabled = true\n")
        proc = self.sync()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)

        outside = self.base / "outside"
        outside.mkdir()
        backups = cache_project_dir(self.project, self.home) / "backups"
        if backups.exists():
            (outside / "backups").mkdir()
            backups.rmdir()
        backups.symlink_to(outside / "backups", target_is_directory=True)

        before = [snapshot(root) for root in (self.project, self.home, outside)]
        proc = self.sync("--refresh-gates")
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("refuse to write", proc.stderr)
        self.assertEqual(
            [snapshot(root) for root in (self.project, self.home, outside)], before
        )


if __name__ == "__main__":
    unittest.main()
