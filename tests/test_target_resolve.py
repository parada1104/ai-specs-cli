import importlib.util
import json
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
TARGET_RESOLVE_PATH = ROOT / "lib" / "_internal" / "target-resolve.py"
FIXTURES = ROOT / "tests" / "fixtures" / "target-resolve"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class TargetResolveTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(TARGET_RESOLVE_PATH, "target_resolve_internal")

    def test_multi_target_fixture_contains_declared_package_directories(self):
        fixture = FIXTURES / "multi-target"
        self.assertTrue((fixture / "packages" / "a").is_dir())
        self.assertTrue((fixture / "packages" / "b").is_dir())

    def test_resolves_root_and_subrepos_in_manifest_order_with_dedup(self):
        plan = self.mod.resolve_target_plan(FIXTURES / "multi-target")
        self.assertEqual([t["rel"] for t in plan["targets"]], [".", "packages/a", "packages/b"])
        self.assertEqual(plan["gitmodules"]["mode"], "advisory-only")
        self.assertEqual(
            plan["targets"][1]["derived_artifacts"],
            ["AGENTS.md", "ai-specs/.gitignore", "ai-specs/skills/**", "ai-specs/commands/**", "agent-configs"],
        )

    def test_root_only_manifest_keeps_single_target(self):
        plan = self.mod.resolve_target_plan(FIXTURES / "root-only")
        self.assertEqual(len(plan["targets"]), 1)
        self.assertEqual(plan["targets"][0]["kind"], "root")

    def test_normalized_project_subrepos_preserve_existing_resolution_semantics(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "packages" / "a").mkdir(parents=True)
            (root / "packages" / "b").mkdir(parents=True)
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name='fixture'\n"
                "subrepos=['packages/a', ' packages/a ', '', 'packages//b', 7]\n"
            )

            plan = self.mod.resolve_target_plan(root)

            self.assertEqual([t["rel"] for t in plan["targets"]], [".", "packages/a", "packages/b"])

    def test_rejects_escape_path(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname='bad'\nsubrepos=['../escape']\n\n[agents]\nenabled=['claude']\n"
            )
            with self.assertRaises(self.mod.ResolutionError) as ctx:
                self.mod.resolve_target_plan(root)
            self.assertIn("escapes the root", str(ctx.exception))

    def test_rejects_missing_directory_via_cli(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname='bad'\nsubrepos=['packages/missing']\n\n[agents]\nenabled=['claude']\n"
            )
            proc = subprocess.run(
                ["python3", str(TARGET_RESOLVE_PATH), str(root)],
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertNotEqual(proc.returncode, 0)
            payload = json.loads(proc.stderr.strip())
            self.assertEqual(payload["error"]["path"], "packages/missing")
            self.assertIn("does not exist", payload["error"]["reason"])

    def test_plan_emits_declared_only_topology_and_planning_root(self):
        """1.3 — RED: plan carries declared_only, topology, and one planning root."""
        plan = self.mod.resolve_target_plan(FIXTURES / "multi-target")
        self.assertIs(plan["declared_only"], True)
        self.assertEqual(plan["fanout_targets"], ["packages/a", "packages/b"])
        self.assertEqual(plan["planning_root"], str(Path(plan["root"]).resolve()))
        self.assertIn(plan["topology"]["resolved"], (
            "standalone", "monorepo-apps", "monorepo-submodules"
        ))
        self.assertIn(plan["topology"]["via"], ("auto", "config"))
        self.assertEqual(plan["worktrees_dir"], ".worktrees")

    def test_all_targets_share_one_planning_root(self):
        """1.3 — RED: every fan-out target shares the root planning root."""
        plan = self.mod.resolve_target_plan(FIXTURES / "multi-target")
        roots = {t["planning_root"] for t in plan["targets"]}
        self.assertEqual(len(roots), 1)
        self.assertEqual(roots.pop(), plan["planning_root"])

    def test_empty_subrepos_emit_empty_fanout_with_gitmodules_present(self):
        """1.3 — RED: empty project.subrepos means no fan-out, never expansion."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname='empty'\nsubrepos=[]\n\n[agents]\nenabled=['claude']\n"
            )
            (root / ".gitmodules").write_text(
                '[submodule "apps/api"]\n\tpath = apps/api\n\turl = ../api.git\n'
            )
            plan = self.mod.resolve_target_plan(root)
            self.assertEqual(plan["fanout_targets"], [])
            self.assertIs(plan["declared_only"], True)
            self.assertEqual([t["rel"] for t in plan["targets"]], ["."])

    def test_gitmodules_never_expands_the_target_set(self):
        """1.3 — RED: .gitmodules entries not declared stay out of fan-out."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "packages" / "a").mkdir(parents=True)
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname='x'\nsubrepos=['packages/a']\n\n[agents]\nenabled=['claude']\n"
            )
            (root / ".gitmodules").write_text(
                '[submodule "packages/b"]\n\tpath = packages/b\n\turl = ../b.git\n'
            )
            plan = self.mod.resolve_target_plan(root)
            self.assertEqual(plan["fanout_targets"], ["packages/a"])
            self.assertEqual([t["rel"] for t in plan["targets"]], [".", "packages/a"])

    def test_plan_topology_reflects_configured_repo_topology(self):
        """1.3 — RED: explicit monorepo-apps stays stable, never reclassified."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname='apps'\nsubrepos=[]\n\n[agents]\nenabled=['claude']\n"
                "[recipes.worktree-flow]\nenabled = true\n"
                "[recipes.worktree-flow.config]\nrepo_topology = 'monorepo-apps'\n"
            )
            plan = self.mod.resolve_target_plan(root)
            self.assertEqual(plan["topology"]["resolved"], "monorepo-apps")
            self.assertEqual(plan["topology"]["via"], "config")


if __name__ == "__main__":
    unittest.main()
