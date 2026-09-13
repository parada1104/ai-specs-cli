"""Tests for the durable tracker binding witness written by sync (Unit 4).

Covers ``lib/_internal/recipe-materialize.py``: the witness lands at
``<git-common-dir>/ai-specs/ledger/witness.json`` with exactly one of the four
design states, records ambiguous candidates without ever guessing a provider,
is written atomically with no temp residue, survives the ``RESOLVED_CONFIG_TEMP``
EXIT trap, and is shared between a main checkout and its linked worktrees.
"""

from __future__ import annotations

import importlib.util
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tests"))
from _fixture_catalog import (  # noqa: E402
    allow_internal_test_recipes_env,
    populate_catalog,
)

RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
SYNC_SH = ROOT / "lib" / "sync.sh"
CLI = ROOT / "bin" / "ai-specs"
GATE_BINARY = ROOT / "dist" / "worktree-gate-current"

TRACKER_RECIPE = '[recipes.test-tracker-ledger]\nenabled = true\nversion = "1.0.0"\n'
TRACKER_CONFLICT_RECIPE = (
    '[recipes.test-tracker-ledger-conflict]\nenabled = true\nversion = "1.0.0"\n'
)
NON_TRACKER_RECIPE = '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
TRACKING_DECLARATION = (
    "schema: spec-driven\n"
    "\n"
    "tracking:\n"
    "  tracker: trello\n"
    '  board_id: "69ec097f13e2d38ecd89a557"\n'
)


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def _git(cwd: Path, *args: str) -> None:
    subprocess.run(
        ["git", "-C", str(cwd), *args], check=True, capture_output=True, text=True
    )


class TrackerLedgerWitnessTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_witness")
        cls._home_tmp = tempfile.TemporaryDirectory()
        cls.home = Path(cls._home_tmp.name)
        populate_catalog(cls.home / "catalog" / "recipes")

    @classmethod
    def tearDownClass(cls):
        cls._home_tmp.cleanup()

    def setUp(self):
        allow = mock.patch.dict(os.environ, allow_internal_test_recipes_env())
        allow.start()
        self.addCleanup(allow.stop)

    # --- helpers ---------------------------------------------------------------

    def _tmp(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def _project(
        self,
        recipes: str,
        *,
        declaration: str | None = None,
        git: bool = True,
        base: Path | None = None,
    ) -> Path:
        root = (base or self._tmp()) / "repo"
        (root / "ai-specs").mkdir(parents=True)
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'witness'\n\n[agents]\nenabled = ['claude']\n\n"
            + recipes
        )
        if declaration is not None:
            (root / "openspec").mkdir()
            (root / "openspec" / "config.yaml").write_text(declaration)
        if git:
            _git(root, "init", "-q")
        return root

    def _sync(self, root: Path) -> Path:
        out = root / "ai-specs" / ".resolved-config.json"
        self.assertEqual(
            self.mod.materialize_recipes(root, self.home, resolved_config_out=out), 0
        )
        return out

    def _witness_path(self, root: Path) -> Path:
        common = self.mod.git_common_dir(root)
        self.assertTrue(common, "expected a git common dir for a git repo")
        return Path(common) / "ai-specs" / "ledger" / "witness.json"

    def _witness(self, root: Path) -> dict:
        return json.loads(self._witness_path(root).read_text())

    # --- 4.1 RED: four states --------------------------------------------------

    def test_bound_witness_names_the_resolved_recipe(self):
        root = self._project(TRACKER_RECIPE)
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["v"], 1)
        self.assertEqual(witness["capability"], "tracker")
        self.assertEqual(witness["state"], "bound")
        self.assertEqual(witness["recipe_id"], "test-tracker-ledger")
        self.assertEqual(witness["candidates"], [])
        self.assertTrue(witness["written_at"].endswith("Z"))

    def test_ambiguous_witness_records_candidates_without_guessing(self):
        root = self._project(TRACKER_RECIPE + TRACKER_CONFLICT_RECIPE)
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["state"], "ambiguous")
        self.assertEqual(
            sorted(witness["candidates"]),
            ["test-tracker-ledger", "test-tracker-ledger-conflict"],
        )
        # No provider is guessed or selected (D6): no bound recipe id at all.
        self.assertEqual(witness["recipe_id"], "")

    def test_unbound_witness_when_no_recipe_declares_tracker(self):
        root = self._project("")
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["state"], "unbound")
        self.assertEqual(witness["recipe_id"], "")
        self.assertEqual(witness["candidates"], [])

    def test_declared_not_bound_witness_when_only_config_declares_tracking(self):
        root = self._project(NON_TRACKER_RECIPE, declaration=TRACKING_DECLARATION)
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["state"], "declared-not-bound")
        self.assertEqual(witness["recipe_id"], "")
        self.assertEqual(witness["candidates"], [])

    def test_witness_has_no_provider_vocabulary(self):
        root = self._project(TRACKER_RECIPE)
        self._sync(root)
        witness = self._witness(root)
        for banned in ("board_id", "board", "list", "trello", "provider"):
            self.assertNotIn(banned, witness)

    # --- 4.1 RED: trap survival + atomic write ---------------------------------

    def test_witness_survives_resolved_config_temp_deletion(self):
        root = self._project(TRACKER_RECIPE)
        resolved_config = self._sync(root)
        # sync.sh's EXIT trap removes RESOLVED_CONFIG_TEMP only.
        resolved_config.unlink()
        witness = self._witness(root)
        self.assertEqual(witness["state"], "bound")
        self.assertEqual(witness["recipe_id"], "test-tracker-ledger")

    def test_atomic_write_leaves_no_temp_residue(self):
        root = self._project(TRACKER_RECIPE)
        self._sync(root)
        ledger_dir = self._witness_path(root).parent
        residue = list(ledger_dir.glob("witness.json.tmp.*"))
        self.assertEqual(residue, [])

    def test_no_witness_outside_a_git_repo(self):
        root = self._project(TRACKER_RECIPE, git=False)
        self._sync(root)
        self.assertEqual(self.mod.git_common_dir(root), "")

    def test_sync_sh_trap_never_names_the_witness(self):
        text = SYNC_SH.read_text()
        trap_line = next(
            line for line in text.splitlines() if "RESOLVED_CONFIG_TEMP:-" in line
        )
        self.assertIn("rm -f", trap_line)
        self.assertNotIn("witness", trap_line.lower())
        self.assertEqual(text.count("RESOLVED_CONFIG_TEMP="), 1)

    # --- 4.5 TRIANGULATE: linked worktree shares the common-dir witness --------

    def test_linked_worktree_reads_main_checkout_witness(self):
        base = self._tmp()
        root = self._project(TRACKER_RECIPE, base=base)
        _git(root, "config", "user.email", "t@t.t")
        _git(root, "config", "user.name", "t")
        (root / "README.md").write_text("x\n")
        _git(root, "add", "-A")
        _git(root, "commit", "-qm", "init")
        linked = base / "linked"
        _git(root, "worktree", "add", "-q", str(linked), "-b", "linked-branch")

        self._sync(root)

        # The linked worktree has its own .git file but one shared common dir.
        self.assertTrue((linked / ".git").is_file())
        self.assertEqual(
            self.mod.git_common_dir(linked), self.mod.git_common_dir(root)
        )
        witness = json.loads(self._witness_path(linked).read_text())
        self.assertEqual(witness["state"], "bound")
        self.assertFalse((linked / ".git" / "ai-specs").exists())

    # --- 4.5 TRIANGULATE: explicit binding + deactivation -----------------------

    def test_explicit_binding_wins_over_two_declarers(self):
        root = self._project(
            TRACKER_RECIPE
            + TRACKER_CONFLICT_RECIPE
            + '[[bindings]]\ncapability = "tracker"\nrecipe = "test-tracker-ledger-conflict"\n'
        )
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["state"], "bound")
        self.assertEqual(witness["recipe_id"], "test-tracker-ledger-conflict")

    def test_disabling_the_bound_recipe_overwrites_the_witness(self):
        root = self._project(TRACKER_RECIPE)
        self._sync(root)
        self.assertEqual(self._witness(root)["state"], "bound")
        # Recipe list shrinks to none: the next sync must deactivate the witness,
        # never leave a disabled provider active.
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'witness'\n\n[agents]\nenabled = ['claude']\n"
        )
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["state"], "unbound")
        self.assertEqual(witness["recipe_id"], "")

    @unittest.skipUnless(GATE_BINARY.is_file(), "worktree-gate binary not built")
    def test_go_ledger_reads_the_python_witness_from_a_linked_worktree(self):
        base = self._tmp()
        root = self._project(TRACKER_RECIPE, base=base)
        _git(root, "config", "user.email", "t@t.t")
        _git(root, "config", "user.name", "t")
        (root / "README.md").write_text("x\n")
        _git(root, "add", "-A")
        _git(root, "commit", "-qm", "init")
        linked = base / "linked"
        _git(root, "worktree", "add", "-q", str(linked), "-b", "linked-branch")

        self._sync(root)

        proc = subprocess.run(
            [
                str(GATE_BINARY),
                "--ledger",
                "--checkpoint",
                "apply-start",
                "--ledger-mode",
                "warn",
                "--project-root",
                str(linked),
            ],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        verdict = json.loads(proc.stdout)
        self.assertTrue(verdict["active"])
        self.assertEqual(verdict["decision"], "allow")
        self.assertEqual(verdict["identity"]["branch"], "linked-branch")


class SyncTrapIntegrationTests(unittest.TestCase):
    """End-to-end: a real ``ai-specs sync`` leaves the witness after its EXIT trap."""

    def test_real_sync_leaves_witness_after_trap(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        workspace = Path(tmp.name) / "workspace"
        workspace.mkdir()
        subprocess.run([str(CLI), "init", str(workspace)], check=True, capture_output=True)
        (workspace / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'witness-e2e'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = '69ec097f13e2d38ecd89a557'\n"
        )
        _git(workspace, "init", "-q")
        subprocess.run(
            [str(CLI), "sync", str(workspace)],
            check=True,
            capture_output=True,
            text=True,
        )
        common = subprocess.run(
            ["git", "-C", str(workspace), "rev-parse", "--path-format=absolute", "--git-common-dir"],
            check=True,
            capture_output=True,
            text=True,
        ).stdout.strip()
        witness_path = Path(common) / "ai-specs" / "ledger" / "witness.json"
        self.assertTrue(witness_path.is_file(), "witness must survive the sync EXIT trap")
        witness = json.loads(witness_path.read_text())
        self.assertEqual(witness["state"], "bound")
        self.assertEqual(witness["recipe_id"], "trello-mcp-workflow")


if __name__ == "__main__":
    unittest.main()
