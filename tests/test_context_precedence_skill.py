import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import cache_project_dir, invoke, isolated_home  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
FIXTURE_ROOT = ROOT / "tests" / "fixtures" / "sync-workspace" / "root"
SKILL = ROOT / "catalog" / "skills" / "context-precedence" / "SKILL.md"
README = ROOT / "README.md"

ORDER = "canonical docs > project skills > packs > handoffs > session memory > proposed context"


class ContextPrecedenceSkillTests(unittest.TestCase):
    def assertContainsAll(self, haystack, needles):
        for needle in needles:
            with self.subTest(needle=needle):
                self.assertIn(needle, haystack)

    def test_context_precedence_skill_states_order_sources_examples_and_boundary(self):
        text = SKILL.read_text()

        self.assertEqual(text.count(ORDER), 1)
        self.assertContainsAll(
            text,
            [
                "# Context Precedence",
                "## Canonical Rule",
                "## Source Classes",
                "## Conflict Examples",
                "## Audit Checklist",
                "**canonical docs**",
                "**project skills**",
                "**packs**",
                "**handoffs**",
                "**session memory**",
                "**proposed context**",
                "Docs vs session memory",
                "Project skills vs packs",
                "Handoffs vs session memory",
                "Proposed context vs existing sources",
                "This MVP is a decision policy, not a runtime merge engine.",
                "MUST NOT require `[memory]`, `[precedence]`, `[packs]`, or any other new manifest section.",
            ],
        )

    def test_readme_points_to_catalog_and_does_not_duplicate_precedence_rule(self):
        readme = README.read_text()

        self.assertEqual(readme.count(ORDER), 0)
        self.assertNotIn("## Context precedence", readme)

    def test_sync_renders_agents_reference_when_bundled_skill_present(self):
        tmp = Path(tempfile.mkdtemp(prefix="ai-specs-precedence-"))
        workspace = tmp / "workspace"
        upstream = tmp / "upstream-catalog"
        home = isolated_home(tmp)
        try:
            shutil.copytree(FIXTURE_ROOT, workspace)
            init = invoke(workspace, "init", cli_home=home)
            self.assertEqual(init.returncode, 0, init.stdout + init.stderr)
            # Local `git clone` only sees committed files — use a tiny upstream repo fixture.
            skill_src = ROOT / "catalog" / "skills" / "context-precedence"
            dst_skill = upstream / "catalog" / "skills" / "context-precedence"
            dst_skill.parent.mkdir(parents=True, exist_ok=True)
            shutil.copytree(skill_src, dst_skill)
            subprocess.run(
                ["git", "init", "-q", str(upstream)],
                check=True,
                text=True,
            )
            subprocess.run(
                ["git", "-C", str(upstream), "add", "catalog"],
                check=True,
                text=True,
            )
            subprocess.run(
                ["git", "-C", str(upstream), "commit", "-q", "-m", "init"],
                check=True,
                text=True,
            )
            toml = (
                "[project]\n"
                "name = 'fixture-precedence'\n\n"
                "[[deps]]\n"
                'id = "context-precedence"\n'
                f"source = {json.dumps(str(upstream))}\n"
                'path = "catalog/skills/context-precedence"\n'
                'scope = ["root"]\n'
                'license = "MIT"\n'
                'auto_invoke = ["Resolving conflicts between documentation, skills, memory, and proposed context"]\n'
            )
            (workspace / "ai-specs" / "ai-specs.toml").write_text(toml)
            sync = invoke(workspace, "sync", cli_home=home)
            self.assertEqual(sync.returncode, 0, sync.stdout + sync.stderr)

            agents = (workspace / "AGENTS.md").read_text()
            self.assertNotIn("## Context Precedence", agents)
            # Verify the dep skill is flattened into the per-project CLI cache
            # (frozen cache-key contract, parity contract §4).
            resolved_skill = (
                cache_project_dir(workspace, home)
                / "resolved-skills"
                / "context-precedence"
                / "SKILL.md"
            )
            self.assertTrue(resolved_skill.is_file())
        finally:
            shutil.rmtree(tmp)


if __name__ == "__main__":
    unittest.main()
