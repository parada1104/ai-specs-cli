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
import subprocess
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


APPLY_PLAN_PAYLOAD = json.dumps(
    {
        "orphaned_recipes": ["gone"],
        "orphaned_deps": [],
        "orphaned_inproject_deps": [],
        "stale_lock_recipes": ["old"],
    }
)

# The gate's --write-lock branch: the real Go writer (lockwrite.go renderLock)
# renders the stdin envelope byte-exactly -- fixed header, [meta], sorted
# [managed."<path>"] with empty values skipped, sorted [agents."<harness>"] --
# then prints {"written": true} with exit 0. The envelope carries no
# recipes/skills/deps sections, so the rewritten lock drops them, same as the
# real gate. Kept free of single quotes so it can be embedded in the sh stub.
_WRITE_LOCK_STUB_PY = r"""
import json, sys
env = json.load(sys.stdin)
def s(v):
    return "\"" + v.replace("\\", "\\\\").replace("\"", "\\\\\"") + "\""
out = [
    "# Managed by ai-specs. Do not edit by hand.",
    "# Provenance stamp: [meta] records the CLI version and timestamp of the last",
    "# sync. [managed.*] records integrity only for CLI-owned override targets;",
    "# it is not a general content-integrity manifest. git covers the committed",
    "# project surface; skill/recipe/dep content hashes are not tracked.",
]
meta = env.get("meta") or {}
if meta:
    out.append("[meta]")
    if meta.get("cli_version"):
        out.append("cli_version = " + s(meta["cli_version"]))
    if meta.get("synced_at"):
        out.append("synced_at = " + s(meta["synced_at"]))
    out.append("")
managed = env.get("managed") or {}
for p in sorted(managed):
    e = managed[p]
    if not e.get("sha256"):
        continue
    out.append("[managed." + s(p) + "]")
    for k in ("sha256", "recipe", "source", "kind", "policy"):
        if e.get(k):
            out.append(k + " = " + s(e[k]))
    out.append("")
agents = env.get("agents") or {}
for h in sorted(agents):
    files = agents[h]
    if not files:
        continue
    out.append("[agents." + s(h) + "]")
    for n in sorted(files):
        out.append(s(n) + " = " + s(files[n]))
    out.append("")
open(env["lock_path"], "w").write("\n".join(out).rstrip("\n") + "\n")
print(json.dumps({"written": True}))
"""

APPLIED_PAYLOAD = json.dumps(
    {
        "status": "applied",
        "removed": [{"scope": "recipe_skills", "name": "gone"}],
        "remaining": [],
        "error": "",
    }
)


class OrphanApplyBridgeTests(unittest.TestCase):
    """WU2: clean_orphans delegates deletion to ``--apply-orphans``.

    Every test stubs the gate binary, so no built Go artifact is required. The
    stub answers ``--plan-orphans`` with a valid plan envelope and
    ``--apply-orphans`` with the scenario payload. The contract pinned here:

    * the apply call receives the plan input plus the three resolved roots,
    * an ``applied`` outcome prints today's messages and prunes the lock,
    * fail-open (legacy Python deletion) happens ONLY when Go provably never
      ran or reported a clean pre-apply failure,
    * partial, uncertain, malformed, timeout, and exit-2 outcomes fail CLOSED:
      no re-deletion, no lock prune, one greppable fail-closed warning.
    """

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_orphan_apply"
        )
        cls.pc = cls.mod._load_project_cache()

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)
        self.home = self.tmp / "home"
        self.home.mkdir()
        self.root = self.tmp / "repo"
        (self.root / "ai-specs").mkdir(parents=True)

    def _tree(self):
        """A project whose recipe scope holds one orphan and one kept child."""
        recipe_dir = self.pc.recipe_skills_root(self.root, cli_home=self.home)
        deps_dir = self.pc.deps_skills_root(self.root, cli_home=self.home)
        inproject_dir = self.pc.inproject_deps_root(self.root)
        recipe_dir.mkdir(parents=True, exist_ok=True)
        for name in ("a", "gone"):
            (recipe_dir / name).mkdir()
        (self.root / "ai-specs" / ".ai-specs.lock").write_text(
            lock_text("a", "old")
        )
        return {"recipe_dir": recipe_dir, "deps_dir": deps_dir,
                "inproject_dir": inproject_dir}

    def _stub(self, apply_body: str) -> Path:
        path = self.tmp / "gate-stub"
        path.write_text(
            "#!/bin/sh\n"
            "if [ \"$1\" = \"--apply-orphans\" ]; then\n"
            f"{apply_body}\n"
            "elif [ \"$1\" = \"--write-lock\" ]; then\n"
            "python3 -c '" + _WRITE_LOCK_STUB_PY + "'\n"
            "else\n"
            f"printf '%s' '{APPLY_PLAN_PAYLOAD}'\n"
            "fi\n"
        )
        path.chmod(0o755)
        return path

    def _run(self) -> tuple[str, str]:
        out, err = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            self.mod.clean_orphans(self.root, {"a"}, {"x"}, cli_home=self.home)
        return out.getvalue(), err.getvalue()

    def _orphan_still_there(self, tree: dict) -> bool:
        return (tree["recipe_dir"] / "gone").is_dir()

    def test_apply_orphans_receives_plan_input_plus_roots(self):
        tree = self._tree()
        argv_capture = self.tmp / "argv"
        stdin_capture = self.tmp / "stdin"
        stub = self._stub(
            f"printf '%s' \"$1\" > '{argv_capture}'\n"
            f"cat > '{stdin_capture}'\n"
            f"printf '%s' '{APPLIED_PAYLOAD}'\n"
        )
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            out, err = self._run()
        self.assertEqual(argv_capture.read_text(), "--apply-orphans")
        self.assertEqual(
            json.loads(stdin_capture.read_text()),
            {
                "recipe_skills": ["a", "gone"],
                "deps_skills": [],
                "inproject_deps": [],
                "lock_recipes": ["a", "old"],
                "enabled_recipe_ids": ["a"],
                "expected_dep_ids": ["x"],
                "roots": {
                    "recipe_skills": str(tree["recipe_dir"]),
                    "deps_skills": str(tree["deps_dir"]),
                    "inproject_deps": str(tree["inproject_dir"]),
                },
            },
        )

    def test_applied_outcome_prints_todays_messages_and_prunes_lock(self):
        self._tree()
        stub = self._stub(f"printf '%s' '{APPLIED_PAYLOAD}'\n")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            out, err = self._run()
        self.assertIn("  ✓ removed orphaned cache .recipe/gone", out)
        self.assertIn("  ✓ removed stale lock entries for recipe 'old'", out)
        self.assertNotIn("'a'", out)
        self.assertEqual(err, "")
        lock = self.mod.load_lock(self.root / "ai-specs" / ".ai-specs.lock")
        self.assertNotIn("old", lock["recipes"])

    def test_partial_outcome_fails_closed(self):
        tree = self._tree()
        payload = json.dumps(
            {
                "status": "partial",
                "removed": [{"scope": "recipe_skills", "name": "gone",
                             "uncertain": True}],
                "remaining": [],
                "error": "remove failed",
            }
        )
        stub = self._stub(f"printf '%s' '{payload}'\nexit 3\n")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            out, err = self._run()
        self.assertTrue(self._orphan_still_there(tree), "legacy must not rerun")
        self.assertNotIn("removed orphaned cache", out)
        self.assertNotIn(self.mod.GO_ORPHANS_BRIDGE_FALLBACK, err)
        self.assertEqual(err.count(self.mod.GO_ORPHANS_APPLY_FAIL_CLOSED), 1, err)
        lock = self.mod.load_lock(self.root / "ai-specs" / ".ai-specs.lock")
        self.assertIn("old", lock["recipes"], "lock must not be pruned")

    def test_malformed_apply_output_fails_closed(self):
        tree = self._tree()
        stub = self._stub("echo not-json\n")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            out, err = self._run()
        self.assertTrue(self._orphan_still_there(tree))
        self.assertNotIn(self.mod.GO_ORPHANS_BRIDGE_FALLBACK, err)
        self.assertEqual(err.count(self.mod.GO_ORPHANS_APPLY_FAIL_CLOSED), 1, err)

    def test_applied_with_remaining_fails_closed(self):
        tree = self._tree()
        payload = json.dumps(
            {
                "status": "applied",
                "removed": [],
                "remaining": [{"scope": "recipe_skills", "name": "gone"}],
                "error": "",
            }
        )
        stub = self._stub(f"printf '%s' '{payload}'\n")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            out, err = self._run()
        self.assertTrue(self._orphan_still_there(tree))
        self.assertNotIn("removed orphaned cache", out)
        self.assertNotIn(self.mod.GO_ORPHANS_BRIDGE_FALLBACK, err)
        self.assertEqual(err.count(self.mod.GO_ORPHANS_APPLY_FAIL_CLOSED), 1, err)
        lock = self.mod.load_lock(self.root / "ai-specs" / ".ai-specs.lock")
        self.assertIn("old", lock["recipes"], "lock must not be pruned")

    def test_applied_with_uncertain_entry_fails_closed(self):
        tree = self._tree()
        payload = json.dumps(
            {
                "status": "applied",
                "removed": [
                    {"scope": "recipe_skills", "name": "gone", "uncertain": True}
                ],
                "remaining": [],
                "error": "",
            }
        )
        stub = self._stub(f"printf '%s' '{payload}'\n")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            out, err = self._run()
        self.assertTrue(self._orphan_still_there(tree))
        self.assertNotIn("removed orphaned cache", out)
        self.assertNotIn(self.mod.GO_ORPHANS_BRIDGE_FALLBACK, err)
        self.assertEqual(err.count(self.mod.GO_ORPHANS_APPLY_FAIL_CLOSED), 1, err)
        lock = self.mod.load_lock(self.root / "ai-specs" / ".ai-specs.lock")
        self.assertIn("old", lock["recipes"], "lock must not be pruned")

    def test_undecodable_apply_output_fails_closed(self):
        tree = self._tree()
        stub = self._stub("printf '\\xff\\xfe not utf-8'\n")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            out, err = self._run()
        self.assertTrue(self._orphan_still_there(tree))
        self.assertNotIn("removed orphaned cache", out)
        self.assertNotIn(self.mod.GO_ORPHANS_BRIDGE_FALLBACK, err)
        self.assertEqual(err.count(self.mod.GO_ORPHANS_APPLY_FAIL_CLOSED), 1, err)

    def test_exit_2_after_invocation_fails_closed(self):
        tree = self._tree()
        stub = self._stub("echo bad input >&2\nexit 2\n")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            out, err = self._run()
        self.assertTrue(self._orphan_still_there(tree))
        self.assertNotIn(self.mod.GO_ORPHANS_BRIDGE_FALLBACK, err)
        self.assertEqual(err.count(self.mod.GO_ORPHANS_APPLY_FAIL_CLOSED), 1, err)

    def test_exit_0_with_non_applied_status_fails_closed(self):
        tree = self._tree()
        payload = json.dumps(
            {
                "status": "pre_apply_failed",
                "removed": [],
                "remaining": [{"scope": "recipe_skills", "name": "gone"}],
                "error": "x",
            }
        )
        stub = self._stub(f"printf '%s' '{payload}'\n")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            out, err = self._run()
        self.assertTrue(self._orphan_still_there(tree))
        self.assertEqual(err.count(self.mod.GO_ORPHANS_APPLY_FAIL_CLOSED), 1, err)

    def test_clean_pre_apply_failed_falls_back_to_legacy_with_one_warning(self):
        tree = self._tree()
        payload = json.dumps(
            {
                "status": "pre_apply_failed",
                "removed": [],
                "remaining": [{"scope": "recipe_skills", "name": "gone"}],
                "error": "root not absolute",
            }
        )
        stub = self._stub(f"printf '%s' '{payload}'\nexit 3\n")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            out, err = self._run()
        self.assertFalse(self._orphan_still_there(tree), "legacy must run")
        self.assertIn("  ✓ removed orphaned cache .recipe/gone", out)
        self.assertIn("  ✓ removed stale lock entries for recipe 'old'", out)
        self.assertEqual(err.count(self.mod.GO_ORPHANS_BRIDGE_FALLBACK), 1, err)
        self.assertNotIn(self.mod.GO_ORPHANS_APPLY_FAIL_CLOSED, err)

    def test_uncertain_pre_apply_failed_fails_closed(self):
        tree = self._tree()
        payload = json.dumps(
            {
                "status": "pre_apply_failed",
                "removed": [],
                "remaining": [
                    {"scope": "recipe_skills", "name": "gone",
                     "uncertain": True}
                ],
                "error": "stat failed",
            }
        )
        stub = self._stub(f"printf '%s' '{payload}'\nexit 3\n")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            out, err = self._run()
        self.assertTrue(self._orphan_still_there(tree))
        self.assertNotIn(self.mod.GO_ORPHANS_BRIDGE_FALLBACK, err)
        self.assertEqual(err.count(self.mod.GO_ORPHANS_APPLY_FAIL_CLOSED), 1, err)

    def _fake_gate(self) -> None:
        """Make binary resolution succeed without a real gate artifact."""

        class _FakeGB:
            @staticmethod
            def resolve_verified_binary(home: Path) -> Path:
                return self.tmp / "fake-gate"

        patcher = mock.patch.object(
            self.mod, "_load_gate_binary", return_value=_FakeGB
        )
        patcher.start()
        self.addCleanup(patcher.stop)

    def _fake_run(self, apply_effect):
        """Patch the gate resolution and subprocess.run for one scenario."""
        self._fake_gate()
        plan_proc = subprocess.CompletedProcess(
            [], 0, stdout=APPLY_PLAN_PAYLOAD, stderr=""
        )

        def fake_run(cmd, **kwargs):
            if cmd[-1] == "--apply-orphans":
                return apply_effect(cmd, kwargs)
            return plan_proc

        patcher = mock.patch.object(self.mod.subprocess, "run", side_effect=fake_run)
        patcher.start()
        self.addCleanup(patcher.stop)

    def test_spawn_failure_before_apply_falls_back_to_legacy_with_one_warning(self):
        tree = self._tree()

        def apply_effect(cmd, kwargs):
            raise FileNotFoundError(2, "No such file or directory")

        self._fake_run(apply_effect)
        out, err = self._run()
        self.assertFalse(self._orphan_still_there(tree), "legacy must run")
        self.assertIn("  ✓ removed orphaned cache .recipe/gone", out)
        self.assertEqual(err.count(self.mod.GO_ORPHANS_BRIDGE_FALLBACK), 1, err)
        self.assertNotIn(self.mod.GO_ORPHANS_APPLY_FAIL_CLOSED, err)

    def test_timeout_after_invocation_fails_closed(self):
        tree = self._tree()

        def apply_effect(cmd, kwargs):
            raise subprocess.TimeoutExpired(cmd="--apply-orphans", timeout=60)

        self._fake_run(apply_effect)
        out, err = self._run()
        self.assertTrue(self._orphan_still_there(tree))
        self.assertNotIn("removed orphaned cache", out)
        self.assertNotIn(self.mod.GO_ORPHANS_BRIDGE_FALLBACK, err)
        self.assertEqual(err.count(self.mod.GO_ORPHANS_APPLY_FAIL_CLOSED), 1, err)


if __name__ == "__main__":
    unittest.main()
