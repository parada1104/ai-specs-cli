"""Contract tests for the Go orphan bridge in ``recipe-materialize.py`` (T4).

``worktree-gate --plan-orphans`` owns the orphan DECISION: which materialized
recipe/dep names and which lock recipe ids the manifest no longer expects.
``recipe-materialize.py`` keeps ACQUISITION (directory listing through the
project cache roots, lock reading) and EXECUTION (``shutil.rmtree``, lock
pruning) and is otherwise a thin bridge over the JSON envelope.

The Python decision survives as a TEMPORARY fail-open fallback
(``GO_ORPHANS_BRIDGE_FALLBACK``) and these tests pin both seams:

* bridge results equal the retained Python authority case by case,
* the stdin/stdout envelope contract of ``--plan-orphans``,
* the degraded path when no verified binary can run.

The Go path needs a built binary (``dist/worktree-gate-current`` or
``$WORKTREE_GATE_BIN``); it skips loudly when none exists. Every fallback test
runs with no usable binary at all, because failing open is the contract they pin.
"""
from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]

RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
DIST_BINARY = ROOT / "dist" / "worktree-gate-current"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def gate_binary() -> Path:
    """The built Go binary the bridge is proven against, or a loud skip."""
    pinned = os.environ.get("WORKTREE_GATE_BIN", "")
    candidate = Path(pinned) if pinned else DIST_BINARY
    if candidate.is_file() and os.access(candidate, os.X_OK):
        return candidate
    raise unittest.SkipTest(
        "no worktree-gate binary (run scripts/build-gate.sh or set "
        "WORKTREE_GATE_BIN); the Go orphan bridge cannot be proven without it"
    )


def lock_text(*recipe_ids: str) -> str:
    """A lock file whose recipe sections ``load_lock`` reads back."""
    out = ["# Managed by ai-specs. Do not edit by hand.", ""]
    for rid in recipe_ids:
        out.append(f"[recipes.{rid}.skills.some-skill]")
        out.append('SKILL.md = "deadbeef"')
        out.append("")
    return "\n".join(out) + "\n"


class _OrphanBridgeTestCase(unittest.TestCase):
    """Shared fixture: the module under test, its cache helper, a pinned binary."""

    @classmethod
    def setUpClass(cls):
        cls.binary = gate_binary()
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_orphan_bridge"
        )
        cls.pc = cls.mod._load_project_cache()

    def setUp(self):
        pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(self.binary)})
        pin.start()
        self.addCleanup(pin.stop)
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)
        self.home = self.tmp / "home"
        self.home.mkdir()

    def _make_tree(
        self,
        name: str,
        *,
        recipes: tuple[str, ...] = (),
        deps: tuple[str, ...] = (),
        inproject: tuple[str, ...] = (),
        lock_recipes: tuple[str, ...] | None = None,
    ) -> dict:
        """Build a project with cache/.deps children and an optional lock.

        Every materialized scope also gets one stray *file* child: it must be
        ignored on both paths, proving the bridge lists directories only.
        """
        root = self.tmp / name / "repo"
        (root / "ai-specs").mkdir(parents=True)
        recipe_dir = self.pc.recipe_skills_root(root, cli_home=self.home)
        deps_dir = self.pc.deps_skills_root(root, cli_home=self.home)
        inproject_dir = self.pc.inproject_deps_root(root)
        for directory, names in (
            (recipe_dir, recipes),
            (deps_dir, deps),
            (inproject_dir, inproject),
        ):
            directory.mkdir(parents=True, exist_ok=True)
            (directory / "stray.txt").write_text("ignore me")
            for child in names:
                (directory / child).mkdir()
                (directory / child / "SKILL.md").write_text("x")
        if lock_recipes is not None:
            lock_path = root / "ai-specs" / ".ai-specs.lock"
            lock_path.write_text(lock_text(*lock_recipes))
        return {
            "root": root,
            "recipe_dir": recipe_dir,
            "deps_dir": deps_dir,
            "inproject_dir": inproject_dir,
        }

    def _state(self, tree: dict) -> dict:
        def names(directory: Path) -> list[str]:
            if not directory.is_dir():
                return []
            return sorted(child.name for child in directory.iterdir())

        lock_path = tree["root"] / "ai-specs" / ".ai-specs.lock"
        return {
            "recipes": names(tree["recipe_dir"]),
            "deps": names(tree["deps_dir"]),
            "inproject": names(tree["inproject_dir"]),
            "lock": lock_path.read_text() if lock_path.is_file() else None,
        }

    def _run(
        self,
        tree: dict,
        enabled: set[str],
        expected: set[str],
        *,
        forbid_python: bool = False,
    ) -> str:
        if forbid_python:
            self._forbid_python_authority()
        captured = io.StringIO()
        with contextlib.redirect_stdout(captured):
            self.mod.clean_orphans(
                tree["root"], enabled, expected, cli_home=self.home
            )
        return captured.getvalue()

    def _forbid_python_authority(self) -> None:
        """Fail loudly if a bridged call reaches the temporary fallback."""
        patcher = mock.patch.object(
            self.mod,
            "_python_orphan_plan",
            side_effect=AssertionError("_python_orphan_plan ran: the bridge fell back"),
        )
        patcher.start()
        self.addCleanup(patcher.stop)


class OrphanParityTests(_OrphanBridgeTestCase):
    """Bridge results equal the Python authority that used to compute them."""

    def _case(
        self,
        name: str,
        specs: dict,
        enabled: set[str],
        expected: set[str],
    ) -> None:
        go_tree = self._make_tree(f"go-{name}", **specs)
        py_tree = self._make_tree(f"py-{name}", **specs)
        go_out = self._run(go_tree, enabled, expected, forbid_python=True)
        py_out = self._run(py_tree, enabled, expected)
        self.assertEqual(go_out, py_out, "print output differs")
        self.assertEqual(self._state(go_tree), self._state(py_tree), "post-state differs")

    def test_matches_the_retained_python_authority_case_by_case(self):
        cases = [
            ("recipe-orphan", {"recipes": ("a", "b"), "deps": ("x",)}, {"a"}, {"x"}),
            ("deps-orphan", {"recipes": ("a",), "deps": ("x", "y")}, {"a"}, {"x"}),
            (
                "inproject-orphan",
                {"recipes": ("a",), "inproject": ("x", "y")},
                {"a"},
                {"x"},
            ),
            (
                "nothing-orphaned",
                {"recipes": ("a",), "deps": ("x",), "inproject": ("x",)},
                {"a"},
                {"x"},
            ),
            (
                "all-scopes-orphaned",
                {"recipes": ("a",), "deps": ("x",), "inproject": ("x",)},
                set(),
                set(),
            ),
            (
                "stale-lock",
                {"recipes": ("a",), "lock_recipes": ("a", "old")},
                {"a"},
                set(),
            ),
            (
                "empty-dirs",
                {"recipes": (), "deps": (), "inproject": ()},
                {"a"},
                {"x"},
            ),
        ]
        for name, specs, enabled, expected in cases:
            with self.subTest(case=name):
                self._case(name, specs, enabled, expected)

    def test_orphan_in_each_scope_is_removed_with_todays_messages(self):
        tree = self._make_tree(
            "scopes",
            recipes=("a", "gone"),
            deps=("x", "gone-dep"),
            inproject=("gone-inproj",),
        )
        out = self._run(tree, {"a"}, {"x"}, forbid_python=True)
        state = self._state(tree)
        self.assertEqual(state["recipes"], ["a", "stray.txt"])
        self.assertEqual(state["deps"], ["stray.txt", "x"])
        self.assertEqual(state["inproject"], ["stray.txt"])
        self.assertIn("  ✓ removed orphaned cache .recipe/gone", out)
        self.assertIn("  ✓ removed orphaned cache .deps/gone-dep", out)
        self.assertIn("  ✓ removed orphaned ai-specs/.deps/gone-inproj", out)

    def test_nothing_orphaned_is_a_silent_no_op(self):
        tree = self._make_tree("quiet", recipes=("a",), deps=("x",), inproject=("x",))
        out = self._run(tree, {"a"}, {"x"}, forbid_python=True)
        self.assertEqual(out, "")
        state = self._state(tree)
        self.assertEqual(state["recipes"], ["a", "stray.txt"])
        self.assertEqual(state["deps"], ["stray.txt", "x"])
        self.assertEqual(state["inproject"], ["stray.txt", "x"])

    def test_stale_lock_entries_are_pruned_on_the_go_path(self):
        tree = self._make_tree("lock", recipes=("a",), lock_recipes=("a", "old"))
        out = self._run(tree, {"a"}, set(), forbid_python=True)
        self.assertIn("  ✓ removed stale lock entries for recipe 'old'", out)
        self.assertNotIn("'a'", out)
        # The shared writer owns the bytes, so pruning drops the section exactly
        # as the Python path did.
        lock = self.mod.load_lock(tree["root"] / "ai-specs" / ".ai-specs.lock")
        self.assertNotIn("old", lock["recipes"])

    def test_non_directory_children_are_never_listed_as_orphans(self):
        tree = self._make_tree("stray", recipes=("a",))
        out = self._run(tree, set(), set(), forbid_python=True)
        self.assertNotIn("stray.txt", out)
        self.assertTrue((tree["recipe_dir"] / "stray.txt").is_file())


class OrphanEnvelopeContractTests(_OrphanBridgeTestCase):
    """The stdin/stdout contract of ``worktree-gate --plan-orphans``."""

    def test_go_envelope_round_trips_and_sorts(self):
        plan = self.mod.go_orphan_plan(
            self.home,
            {
                "recipe_skills": ["b", "a"],
                "deps_skills": ["z"],
                "inproject_deps": ["y"],
                "lock_recipes": ["old", "a"],
                "enabled_recipe_ids": ["a"],
                "expected_dep_ids": ["z"],
            },
        )
        self.assertEqual(
            plan,
            {
                "orphaned_recipes": ["b"],
                "orphaned_deps": [],
                "orphaned_inproject_deps": ["y"],
                "stale_lock_recipes": ["old"],
            },
        )

    def test_bridge_invokes_plan_orphans_with_json_stdin(self):
        argv_capture = self.tmp / "argv"
        stdin_capture = self.tmp / "stdin"
        payload = json.dumps(
            {
                "orphaned_recipes": [],
                "orphaned_deps": [],
                "orphaned_inproject_deps": [],
                "stale_lock_recipes": [],
            }
        )
        stub = self.tmp / "worktree-gate-stub"
        stub.write_text(
            "#!/bin/sh\n"
            f"printf '%s' \"$1\" > '{argv_capture}'\n"
            f"cat > '{stdin_capture}'\n"
            f"printf '%s' '{payload}'\n"
        )
        stub.chmod(0o755)
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            plan = self.mod.go_orphan_plan(
                self.home,
                {
                    "recipe_skills": ["b"],
                    "deps_skills": [],
                    "inproject_deps": [],
                    "lock_recipes": [],
                    "enabled_recipe_ids": ["a"],
                    "expected_dep_ids": [],
                },
            )
        self.assertEqual(plan["orphaned_recipes"], [])
        self.assertEqual(argv_capture.read_text(), "--plan-orphans")
        self.assertEqual(
            json.loads(stdin_capture.read_text()),
            {
                "recipe_skills": ["b"],
                "deps_skills": [],
                "inproject_deps": [],
                "lock_recipes": [],
                "enabled_recipe_ids": ["a"],
                "expected_dep_ids": [],
            },
        )


class OrphanFailOpenFallbackTests(unittest.TestCase):
    """No usable binary: the legacy Python decision runs, with one warning."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_orphan_bridge_fallback"
        )
        cls.pc = cls.mod._load_project_cache()

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)
        self.home = self.tmp / "home"
        # No binary anywhere: an empty override short-circuits the cache lookup.
        no_pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": ""})
        no_pin.start()
        self.addCleanup(no_pin.stop)
        self.root = self.tmp / "repo"
        (self.root / "ai-specs").mkdir(parents=True)

    def _tree(self, recipes: tuple[str, ...] = ()) -> None:
        recipe_dir = self.pc.recipe_skills_root(self.root, cli_home=self.home)
        recipe_dir.mkdir(parents=True, exist_ok=True)
        for name in recipes:
            (recipe_dir / name).mkdir()

    def _run(self, enabled: set[str], expected: set[str]) -> str:
        captured = io.StringIO()
        with contextlib.redirect_stderr(captured):
            self.mod.clean_orphans(self.root, enabled, expected, cli_home=self.home)
        return captured.getvalue()

    def _stub(self, body: str) -> Path:
        path = self.tmp / "worktree-gate-stub"
        path.write_text(f"#!/bin/sh\n{body}\n")
        path.chmod(0o755)
        return path

    def test_missing_binary_falls_back_to_python_with_one_warning(self):
        self._tree(("a", "gone"))
        stderr = self._run({"a"}, set())
        self.assertEqual(stderr.count(self.mod.GO_ORPHANS_BRIDGE_FALLBACK), 1, stderr)
        self.assertIn("temporary", stderr.lower())
        self.assertIn("no verified worktree-gate binary", stderr)
        self.assertFalse(
            (self.pc.recipe_skills_root(self.root, cli_home=self.home) / "gone").exists()
        )

    def test_binary_that_fails_falls_back_with_one_warning(self):
        self._tree(("a", "gone"))
        stub = self._stub("exit 2")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            stderr = self._run({"a"}, set())
        self.assertEqual(stderr.count(self.mod.GO_ORPHANS_BRIDGE_FALLBACK), 1, stderr)
        self.assertIn("exited 2", stderr)
        self.assertFalse(
            (self.pc.recipe_skills_root(self.root, cli_home=self.home) / "gone").exists()
        )

    def test_unreadable_binary_output_falls_back_with_one_warning(self):
        self._tree(("a", "gone"))
        stub = self._stub("echo not-json")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            stderr = self._run({"a"}, set())
        self.assertEqual(stderr.count(self.mod.GO_ORPHANS_BRIDGE_FALLBACK), 1, stderr)

    def test_malformed_envelope_falls_back_with_one_warning(self):
        self._tree(("a", "gone"))
        stub = self._stub("echo '{\"orphaned_recipes\": \"nope\"}'")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            stderr = self._run({"a"}, set())
        self.assertEqual(stderr.count(self.mod.GO_ORPHANS_BRIDGE_FALLBACK), 1, stderr)

    def test_fallback_path_is_marked_temporary_in_the_source(self):
        self.assertEqual(
            self.mod.GO_ORPHANS_BRIDGE_FALLBACK, "GO_ORPHANS_BRIDGE_FALLBACK"
        )
        doc = self.mod._python_orphan_plan.__doc__ or ""
        self.assertIn("GO_ORPHANS_BRIDGE_FALLBACK", doc)
        self.assertIn("TEMPORARY", doc)


if __name__ == "__main__":
    unittest.main()
