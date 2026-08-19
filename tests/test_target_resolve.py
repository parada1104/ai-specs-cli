import json
import re
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
FIXTURES = ROOT / "tests" / "fixtures" / "target-resolve"

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home


_TARGETS_RE = re.compile(r"targets:\s+(.+)")
_TOPOLOGY_RE = re.compile(r"topology:\s+(\S+)\s+\(via\s+(\w+)\)")
_FANOUT_RE = re.compile(r"fan-out:\s+(\S+)")
_DERIVED_RE = re.compile(r"derived:\s+(.+)")


def _parse_targets(stdout: str) -> list[dict[str, str]]:
    """Parse target entries from sync output like 'root:. subrepo:packages/a'."""
    m = _TARGETS_RE.search(stdout)
    if not m:
        return []
    targets = []
    for entry in m.group(1).split():
        kind, rel = entry.split(":", 1)
        targets.append({"kind": kind, "rel": rel})
    return targets


def _parse_derived(stdout: str) -> list[str]:
    """Parse derived artifacts from sync output."""
    m = _DERIVED_RE.search(stdout)
    if not m:
        return []
    return [x.strip() for x in m.group(1).split(",")]


class TargetResolveTests(unittest.TestCase):
    def setUp(self):
        self._home_td = tempfile.TemporaryDirectory(prefix="target-home-")
        self.addCleanup(self._home_td.cleanup)
        self._home = isolated_home(Path(self._home_td.name))

    def _invoke(self, root: Path, *args: str):
        return invoke(root, *args, cli_home=self._home)

    def test_multi_target_fixture_contains_declared_package_directories(self):
        fixture = FIXTURES / "multi-target"
        self.assertTrue((fixture / "packages" / "a").is_dir())
        self.assertTrue((fixture / "packages" / "b").is_dir())

    def test_resolves_root_and_subrepos_in_manifest_order_with_dedup(self):
        r = self._invoke(FIXTURES / "multi-target", "sync")
        self.assertEqual(r.returncode, 0, r.stderr)
        targets = _parse_targets(r.stdout)
        self.assertEqual([t["rel"] for t in targets], [".", "packages/a", "packages/b"])
        self.assertIn("advisory-only", r.stdout)
        derived = _parse_derived(r.stdout)
        self.assertEqual(
            derived,
            ["AGENTS.md", "ai-specs/.gitignore", "ai-specs/skills/**", "ai-specs/commands/**", "agent-configs"],
        )

    def test_root_only_manifest_keeps_single_target(self):
        r = self._invoke(FIXTURES / "root-only", "sync")
        self.assertEqual(r.returncode, 0, r.stderr)
        targets = _parse_targets(r.stdout)
        self.assertEqual(len(targets), 1)
        self.assertEqual(targets[0]["kind"], "root")

    def test_normalized_project_subrepos_preserve_existing_resolution_semantics(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "packages" / "a").mkdir(parents=True)
            (root / "packages" / "b").mkdir(parents=True)
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "skills").mkdir()
            (root / "ai-specs" / "commands").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\n"
                "name='fixture'\n"
                "subrepos=['packages/a', ' packages/a ', '', 'packages//b', 7]\n\n"
                "[agents]\nenabled=['claude']\n"
            )

            r = self._invoke(root, "sync")
            self.assertEqual(r.returncode, 0, r.stderr)
            targets = _parse_targets(r.stdout)
            self.assertEqual([t["rel"] for t in targets], [".", "packages/a", "packages/b"])

    def test_rejects_escape_path(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "skills").mkdir()
            (root / "ai-specs" / "commands").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname='bad'\nsubrepos=['../escape']\n\n[agents]\nenabled=['claude']\n"
            )
            r = self._invoke(root, "sync")
            self.assertNotEqual(r.returncode, 0)
            self.assertIn("escapes the root", r.stderr)

    def test_rejects_missing_directory_via_cli(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "skills").mkdir()
            (root / "ai-specs" / "commands").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname='bad'\nsubrepos=['packages/missing']\n\n[agents]\nenabled=['claude']\n"
            )
            r = self._invoke(root, "sync")
            self.assertNotEqual(r.returncode, 0)
            payload = json.loads(r.stderr.splitlines()[0].strip())
            self.assertEqual(payload["error"]["path"], "packages/missing")
            self.assertIn("does not exist", payload["error"]["reason"])

    def test_plan_emits_declared_only_topology_and_planning_root(self):
        """1.3 — RED: plan carries declared_only, topology, and one planning root."""
        r = self._invoke(FIXTURES / "multi-target", "sync")
        self.assertEqual(r.returncode, 0, r.stderr)
        targets = _parse_targets(r.stdout)
        self.assertEqual([t["rel"] for t in targets[1:]], ["packages/a", "packages/b"])
        self.assertIn("planning:", r.stdout)
        m = _TOPOLOGY_RE.search(r.stdout)
        self.assertIsNotNone(m)
        self.assertIn(m.group(1), ("standalone", "monorepo-apps", "monorepo-submodules"))
        self.assertIn(m.group(2), ("auto", "config"))
        self.assertIn("declared-only", r.stdout)

    def test_all_targets_share_one_planning_root(self):
        """1.3 — RED: every fan-out target shares the root planning root."""
        r = self._invoke(FIXTURES / "multi-target", "sync")
        self.assertEqual(r.returncode, 0, r.stderr)
        planning_lines = [l for l in r.stdout.splitlines() if "planning:" in l]
        self.assertEqual(len(planning_lines), 1)
        targets = _parse_targets(r.stdout)
        root_targets = [t for t in targets if t["kind"] == "root"]
        self.assertEqual(len(root_targets), 1)

    def test_empty_subrepos_emit_empty_fanout_with_gitmodules_present(self):
        """1.3 — RED: empty project.subrepos means no fan-out, never expansion."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "skills").mkdir()
            (root / "ai-specs" / "commands").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname='empty'\nsubrepos=[]\n\n[agents]\nenabled=['claude']\n"
            )
            (root / ".gitmodules").write_text(
                '[submodule "apps/api"]\n\tpath = apps/api\n\turl = ../api.git\n'
            )
            r = self._invoke(root, "sync")
            self.assertEqual(r.returncode, 0, r.stderr)
            targets = _parse_targets(r.stdout)
            self.assertIn("declared-only", r.stdout)
            self.assertEqual([t["rel"] for t in targets], ["."])

    def test_gitmodules_never_expands_the_target_set(self):
        """1.3 — RED: .gitmodules entries not declared stay out of fan-out."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "packages" / "a").mkdir(parents=True)
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "skills").mkdir()
            (root / "ai-specs" / "commands").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname='x'\nsubrepos=['packages/a']\n\n[agents]\nenabled=['claude']\n"
            )
            (root / ".gitmodules").write_text(
                '[submodule "packages/b"]\n\tpath = packages/b\n\turl = ../b.git\n'
            )
            r = self._invoke(root, "sync")
            self.assertEqual(r.returncode, 0, r.stderr)
            targets = _parse_targets(r.stdout)
            self.assertEqual([t["rel"] for t in targets], [".", "packages/a"])

    def test_plan_topology_reflects_configured_repo_topology(self):
        """1.3 — RED: explicit monorepo-apps stays stable, never reclassified."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "ai-specs").mkdir()
            (root / "ai-specs" / "skills").mkdir()
            (root / "ai-specs" / "commands").mkdir()
            (root / "ai-specs" / "ai-specs.toml").write_text(
                "[project]\nname='apps'\nsubrepos=[]\n\n[agents]\nenabled=['claude']\n"
                "[recipes.worktree-flow]\nenabled = true\n"
                "[recipes.worktree-flow.config]\nrepo_topology = 'monorepo-apps'\n"
            )
            r = self._invoke(root, "sync")
            self.assertEqual(r.returncode, 0, r.stderr)
            m = _TOPOLOGY_RE.search(r.stdout)
            self.assertIsNotNone(m)
            self.assertEqual(m.group(1), "monorepo-apps")
            self.assertEqual(m.group(2), "config")


if __name__ == "__main__":
    unittest.main()
