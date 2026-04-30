import json
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
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

    def test_readme_points_to_context_precedence_skill_without_duplicating_rule(self):
        readme = README.read_text()

        self.assertContainsAll(
            readme,
            [
                "## Context precedence",
                "catalog/README.md",
                "[`ai-specs/skills/context-precedence/SKILL.md`](ai-specs/skills/context-precedence/SKILL.md)",
            ],
        )
        self.assertEqual(readme.count(ORDER), 0)

    def test_sync_renders_agents_reference_when_bundled_skill_present(self):
        tmp = Path(tempfile.mkdtemp(prefix="ai-specs-precedence-"))
        workspace = tmp / "workspace"
        upstream = tmp / "upstream-catalog"
        try:
            shutil.copytree(FIXTURE_ROOT, workspace)
            subprocess.run([str(CLI), "init", str(workspace)], check=True, text=True)
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
            subprocess.run([str(CLI), "sync", str(workspace)], check=True, text=True)

            agents = (workspace / "AGENTS.md").read_text()
            registry = (workspace / "ai-specs" / ".skill-registry.md").read_text()
            self.assertNotIn("## Context Precedence", agents)
            self.assertContainsAll(
                registry,
                [
                    "| `context-precedence` | dep |",
                    ".deps/context-precedence/skills/context-precedence/SKILL.md",
                    "| Resolving conflicts between documentation, skills, memory, and proposed context | `context-precedence` | `root` |",
                ],
            )
        finally:
            shutil.rmtree(tmp)


if __name__ == "__main__":
    unittest.main()
