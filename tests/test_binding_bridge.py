"""Contract tests for the Go binding bridge in ``recipe-materialize.py`` (T5).

The Go gate binary owns capability binding resolution, capability conflict
grading, and the durable tracker witness write. ``recipe-materialize.py`` is the
bridge only: it hands Go the manifest data Python already read and maps the JSON
envelope back onto the call sites. The Python implementations survive as a
TEMPORARY fail-open fallback (``GO_BINDINGS_BRIDGE_FALLBACK``) and these tests
pin both seams:

* bridge results are identical to the Python authority it replaced,
* the witness states Go writes (bound/ambiguous/declared-not-bound/unbound),
* the degraded path when no verified binary can run.

The Go path needs a built binary (``dist/worktree-gate-current`` or
``$WORKTREE_GATE_BIN``); it skips loudly when none exists. Every fallback test
runs with no binary at all, because failing open is the contract they pin.
"""
from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import os
import re
import subprocess
import sys
import tempfile
import unittest
from datetime import datetime, timezone
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tests"))
from _fixture_catalog import (  # noqa: E402
    allow_internal_test_recipes_env,
    populate_catalog,
)

RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
GATE_BINARY_PY = ROOT / "lib" / "_internal" / "gate_binary.py"
DIST_BINARY = ROOT / "dist" / "worktree-gate-current"

# The witness instant format the Python writer produced and Go reproduces.
WITNESS_STAMP_RE = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$")

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
)


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
        "WORKTREE_GATE_BIN); the Go binding bridge cannot be proven without it"
    )


def write_recipe(catalog: Path, recipe_id: str, capabilities: list[str]) -> None:
    """Write a minimal catalog recipe declaring the given capabilities."""
    recipe_dir = catalog / recipe_id
    recipe_dir.mkdir(parents=True, exist_ok=True)
    caps = "".join(f'[[capabilities]]\nid = "{cap}"\n' for cap in capabilities)
    (recipe_dir / "recipe.toml").write_text(
        f'[recipe]\nid = "{recipe_id}"\nname = "{recipe_id}"\n'
        f'description = "fixture"\nversion = "1.0"\n' + caps
    )


def _git(cwd: Path, *args: str) -> None:
    subprocess.run(
        ["git", "-C", str(cwd), *args], check=True, capture_output=True, text=True
    )


def conflict_shape(conflicts: list) -> list[tuple[str, str, list[str]]]:
    """Comparable projection of a conflict list, whatever produced it."""
    return [
        (c.primitive_type, c.primitive_id, sorted(c.recipes)) for c in conflicts
    ]


class _GoBridgeTestCase(unittest.TestCase):
    """Shared fixture: the module under test plus a pinned Go binary."""

    @classmethod
    def setUpClass(cls):
        cls.binary = gate_binary()
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_binding_bridge"
        )
        cls.gb = load_module(GATE_BINARY_PY, "gate_binary_binding_bridge")

    def setUp(self):
        allow = mock.patch.dict(os.environ, allow_internal_test_recipes_env())
        allow.start()
        self.addCleanup(allow.stop)
        pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(self.binary)})
        pin.start()
        self.addCleanup(pin.stop)

    def _catalog(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        catalog = Path(tmp.name) / "catalog" / "recipes"
        catalog.mkdir(parents=True)
        return catalog

    def _forbid_python_authority(self) -> None:
        """Fail loudly if a bridged call reaches the temporary fallback."""
        for name in ("_python_resolve_bindings", "_python_check_capability_conflicts"):
            patcher = mock.patch.object(
                self.mod,
                name,
                side_effect=AssertionError(f"{name} ran: the bridge fell back"),
            )
            patcher.start()
            self.addCleanup(patcher.stop)


class BindingParityTests(_GoBridgeTestCase):
    """Bridge results equal the Python authority that used to compute them."""

    def test_explicit_binding_selects_the_named_recipe(self):
        catalog = self._catalog()
        write_recipe(catalog, "recipe-a", ["tracker"])
        write_recipe(catalog, "recipe-b", ["tracker"])
        self._forbid_python_authority()
        self.assertEqual(
            self.mod.resolve_bindings(
                catalog,
                ["recipe-a", "recipe-b"],
                [{"capability": "tracker", "recipe": "recipe-b"}],
            ),
            {"tracker": "recipe-b"},
        )

    def test_auto_bind_single_provider(self):
        catalog = self._catalog()
        write_recipe(catalog, "recipe-a", ["tracker", "vcs-pr-flow"])
        write_recipe(catalog, "recipe-b", ["vcs-pr-flow"])
        self._forbid_python_authority()
        self.assertEqual(
            self.mod.resolve_bindings(catalog, ["recipe-a"], []),
            {"tracker": "recipe-a", "vcs-pr-flow": "recipe-a"},
        )

    def test_ambiguity_skips_the_capability(self):
        catalog = self._catalog()
        write_recipe(catalog, "recipe-a", ["tracker"])
        write_recipe(catalog, "recipe-b", ["tracker"])
        self._forbid_python_authority()
        bindings = self.mod.resolve_bindings(catalog, ["recipe-a", "recipe-b"], [])
        self.assertNotIn("tracker", bindings)

    def test_duplicate_explicit_binding_is_a_fatal_error(self):
        catalog = self._catalog()
        write_recipe(catalog, "recipe-a", ["tracker"])
        write_recipe(catalog, "recipe-b", ["tracker"])
        self._forbid_python_authority()
        with self.assertRaises(RuntimeError) as ctx:
            self.mod.resolve_bindings(
                catalog,
                ["recipe-a", "recipe-b"],
                [
                    {"capability": "tracker", "recipe": "recipe-a"},
                    {"capability": "tracker", "recipe": "recipe-b"},
                ],
            )
        self.assertEqual(str(ctx.exception), "duplicate explicit binding for capability 'tracker'")
        self.assertEqual(ctx.exception.code, "duplicate-explicit-binding")

    def test_disabled_or_unknown_recipe_is_an_error(self):
        catalog = self._catalog()
        write_recipe(catalog, "recipe-a", ["tracker"])
        self._forbid_python_authority()
        with self.assertRaises(RuntimeError) as ctx:
            self.mod.resolve_bindings(
                catalog,
                ["recipe-a"],
                [{"capability": "tracker", "recipe": "recipe-b"}],
            )
        self.assertEqual(
            str(ctx.exception),
            "explicit binding for capability 'tracker' references disabled/unknown recipe 'recipe-b'",
        )
        self.assertEqual(ctx.exception.code, "disabled-or-unknown-recipe")

    def test_undeclared_capability_is_an_error(self):
        catalog = self._catalog()
        write_recipe(catalog, "recipe-a", ["vcs-pr-flow"])
        write_recipe(catalog, "recipe-b", ["vcs-pr-flow"])
        self._forbid_python_authority()
        with self.assertRaises(RuntimeError) as ctx:
            self.mod.resolve_bindings(
                catalog,
                ["recipe-a", "recipe-b"],
                [{"capability": "tracker", "recipe": "recipe-b"}],
            )
        self.assertEqual(
            str(ctx.exception),
            "explicit binding for capability 'tracker' references recipe 'recipe-b' "
            "which does not declare that capability",
        )
        self.assertEqual(ctx.exception.code, "undeclared-capability")

    def test_ambiguity_conflict_warning_names_every_provider(self):
        catalog = self._catalog()
        write_recipe(catalog, "recipe-a", ["tracker"])
        write_recipe(catalog, "recipe-b", ["tracker"])
        self._forbid_python_authority()
        conflicts = self.mod.check_capability_conflicts(
            catalog, ["recipe-a", "recipe-b"], []
        )
        self.assertEqual(conflict_shape(conflicts), [("capability", "tracker", ["recipe-a", "recipe-b"])])
        self.assertEqual([c.severity for c in conflicts], ["warning"])

    def test_duplicate_binding_conflict_is_fatal(self):
        catalog = self._catalog()
        write_recipe(catalog, "recipe-a", ["tracker"])
        write_recipe(catalog, "recipe-b", ["tracker"])
        self._forbid_python_authority()
        conflicts = self.mod.check_capability_conflicts(
            catalog,
            ["recipe-a", "recipe-b"],
            [
                {"capability": "tracker", "recipe": "recipe-a"},
                {"capability": "tracker", "recipe": "recipe-b"},
            ],
        )
        self.assertEqual(len(conflicts), 1)
        self.assertEqual(conflicts[0].severity, "fatal")
        self.assertEqual(sorted(conflicts[0].recipes), ["recipe-a", "recipe-b"])

    def test_bridge_matches_the_retained_python_authority_case_by_case(self):
        catalog = self._catalog()
        write_recipe(catalog, "recipe-a", ["tracker"])
        write_recipe(catalog, "recipe-b", ["tracker"])
        write_recipe(catalog, "recipe-c", ["vcs-pr-flow"])

        def outcome(call):
            try:
                return ("ok", call())
            except RuntimeError as exc:
                return ("error", str(exc))

        cases = [
            (["recipe-a"], []),
            (["recipe-a", "recipe-c"], []),
            (["recipe-a", "recipe-b"], []),
            (["recipe-a", "recipe-b"], [{"capability": "tracker", "recipe": "recipe-a"}]),
            (["recipe-a", "recipe-c"], [{"capability": "vcs-pr-flow", "recipe": "recipe-c"}]),
            (["recipe-a"], [{"capability": "tracker", "recipe": "ghost"}]),
            (["recipe-a", "recipe-b"], [{"capability": "tracker", "recipe": "recipe-a"},
                                        {"capability": "tracker", "recipe": "recipe-b"}]),
        ]
        for enabled, bindings in cases:
            with self.subTest(enabled=enabled, bindings=bindings):
                self.assertEqual(
                    outcome(lambda: self.mod.resolve_bindings(catalog, enabled, bindings)),
                    outcome(lambda: self.mod._python_resolve_bindings(catalog, enabled, bindings)),
                )
                self.assertEqual(
                    conflict_shape(
                        self.mod.check_capability_conflicts(catalog, enabled, bindings)
                    ),
                    conflict_shape(
                        self.mod._python_check_capability_conflicts(
                            catalog, enabled, bindings
                        )
                    ),
                )


class WitnessBridgeTests(_GoBridgeTestCase):
    """The durable witness is produced by Go during the same invocation."""

    @classmethod
    def setUpClass(cls):
        super().setUpClass()
        cls._home_tmp = tempfile.TemporaryDirectory()
        cls.home = Path(cls._home_tmp.name)
        populate_catalog(cls.home / "catalog" / "recipes")

    @classmethod
    def tearDownClass(cls):
        cls._home_tmp.cleanup()
        super().tearDownClass()

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
    ) -> Path:
        root = self._tmp() / "repo"
        (root / "ai-specs").mkdir(parents=True)
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'witness'\n\n[agents]\nenabled = ['claude']\n\n" + recipes
        )
        if declaration is not None:
            (root / "openspec").mkdir()
            (root / "openspec" / "config.yaml").write_text(declaration)
        if git:
            _git(root, "init", "-q")
        return root

    def _sync(self, root: Path) -> None:
        out = root / "ai-specs" / ".resolved-config.json"
        self.assertEqual(
            self.mod.materialize_recipes(root, self.home, resolved_config_out=out), 0
        )

    def _witness_path(self, root: Path) -> Path:
        common = self.mod.git_common_dir(root)
        self.assertTrue(common, "expected a git common dir for a git repo")
        return Path(common) / "ai-specs" / "ledger" / "witness.json"

    def _witness(self, root: Path) -> dict:
        return json.loads(self._witness_path(root).read_text())

    def _forbid_python_writer(self) -> None:
        patcher = mock.patch.object(
            self.mod,
            "write_tracker_witness",
            side_effect=AssertionError("the Python writer ran: Go did not own the write"),
        )
        patcher.start()
        self.addCleanup(patcher.stop)

    def test_go_writes_the_bound_witness(self):
        root = self._project(TRACKER_RECIPE)
        self._forbid_python_writer()
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["state"], "bound")
        self.assertEqual(witness["recipe_id"], "test-tracker-ledger")
        self.assertEqual(witness["candidates"], [])

    def test_go_writes_the_ambiguous_witness_without_guessing(self):
        root = self._project(TRACKER_RECIPE + TRACKER_CONFLICT_RECIPE)
        self._forbid_python_writer()
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["state"], "ambiguous")
        self.assertEqual(
            sorted(witness["candidates"]),
            ["test-tracker-ledger", "test-tracker-ledger-conflict"],
        )
        self.assertEqual(witness["recipe_id"], "")

    def test_go_writes_the_declared_not_bound_witness(self):
        root = self._project(NON_TRACKER_RECIPE, declaration=TRACKING_DECLARATION)
        self._forbid_python_writer()
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["state"], "declared-not-bound")
        self.assertEqual(witness["recipe_id"], "")

    def test_go_writes_the_unbound_witness_on_an_empty_repository(self):
        root = self._project("")
        self._forbid_python_writer()
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["state"], "unbound")
        self.assertEqual(witness["candidates"], [])

    def test_no_witness_is_persisted_when_resolution_fails(self):
        """A duplicate binding aborts resolution, so no ledger state is invented."""
        root = self._project(
            TRACKER_RECIPE
            + TRACKER_CONFLICT_RECIPE
            + '[[bindings]]\ncapability = "tracker"\nrecipe = "test-tracker-ledger"\n'
            + '[[bindings]]\ncapability = "tracker"\nrecipe = "test-tracker-ledger-conflict"\n'
        )
        with self.assertRaises(RuntimeError):
            self._sync(root)
        self.assertFalse(self._witness_path(root).exists())

    def test_go_witness_bytes_match_the_python_writer(self):
        """The two authorities persist the same document, key order and all."""
        go_root = self._project(TRACKER_RECIPE + TRACKER_CONFLICT_RECIPE)
        self._sync(go_root)
        with mock.patch.dict(os.environ, {}, clear=False):
            os.environ.pop("WORKTREE_GATE_BIN", None)
            py_root = self._project(TRACKER_RECIPE + TRACKER_CONFLICT_RECIPE)
            self._sync(py_root)
        go_doc = json.loads(self._witness_path(go_root).read_text())
        py_doc = json.loads(self._witness_path(py_root).read_text())
        self.assertEqual(list(go_doc), list(py_doc))
        go_doc["written_at"] = py_doc["written_at"] = "normalized"
        self.assertEqual(go_doc, py_doc)

    def test_witness_written_at_is_utc_second_precision(self):
        root = self._project(TRACKER_RECIPE)
        self._sync(root)
        stamp = self._witness(root)["written_at"]
        self.assertRegex(stamp, WITNESS_STAMP_RE)
        written = datetime.strptime(stamp, "%Y-%m-%dT%H:%M:%SZ").replace(
            tzinfo=timezone.utc
        )
        delta = abs((datetime.now(timezone.utc) - written).total_seconds())
        self.assertLess(delta, 300, f"witness stamp is stale: {stamp}")

    def test_read_only_resolution_never_writes_the_witness(self):
        """The doctor call site resolves bindings without a durable side effect."""
        root = self._project(TRACKER_RECIPE)
        catalog = self.home / "catalog" / "recipes"
        self._forbid_python_authority()
        previous = Path.cwd()
        # Resolve from inside the repository: were Go to default the witness root
        # to the process cwd, the write would land exactly here and be caught.
        os.chdir(root)
        try:
            # Exactly the doctor call shape: no project root, no write opt-in.
            bindings = self.mod.resolve_bindings(catalog, ["test-tracker-ledger"], [])
        finally:
            os.chdir(previous)
        self.assertEqual(bindings, {"tracker": "test-tracker-ledger"})
        self.assertFalse(self._witness_path(root).exists())

    def test_write_opt_in_without_a_project_root_never_defaults_to_cwd(self):
        """A write request without an owning repository must not fall back to cwd."""
        catalog = self.home / "catalog" / "recipes"
        # A decoy repository stands in for the process cwd: were the bridge to let
        # Go default its witness root, the stray ledger would land here.
        decoy = self._project(TRACKER_RECIPE)
        previous = Path.cwd()
        os.chdir(decoy)
        try:
            self.mod.resolve_bindings(
                catalog, ["test-tracker-ledger"], [], write_witness=True
            )
        finally:
            os.chdir(previous)
        self.assertFalse(
            self._witness_path(decoy).exists(),
            "the bridge wrote a witness into the process cwd",
        )


class FailOpenFallbackTests(unittest.TestCase):
    """No verified binary: the temporary Python authority runs, with one warning."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_binding_bridge_fallback"
        )
        cls.gb = load_module(GATE_BINARY_PY, "gate_binary_binding_bridge_fallback")
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
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)
        self.missing = self.tmp / "no-such-worktree-gate"
        self._pin(self.missing)

    def _pin(self, path: Path) -> None:
        pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(path)})
        pin.start()
        self.addCleanup(pin.stop)

    def _stub(self, body: str) -> Path:
        path = self.tmp / "worktree-gate-stub"
        path.write_text(f"#!/bin/sh\n{body}\n")
        path.chmod(0o755)
        return path

    def test_missing_binary_falls_back_to_python_with_one_warning(self):
        catalog = self.home / "catalog" / "recipes"
        captured = io.StringIO()
        with contextlib.redirect_stderr(captured):
            bindings = self.mod.resolve_bindings(catalog, ["test-tracker-ledger"], [])
        self.assertEqual(bindings, {"tracker": "test-tracker-ledger"})
        stderr = captured.getvalue()
        self.assertEqual(stderr.count(self.mod.GO_BINDINGS_BRIDGE_FALLBACK), 1, stderr)
        self.assertIn("temporary", stderr.lower())
        self.assertIn("no verified worktree-gate binary", stderr)

    def test_binary_that_fails_falls_back_with_one_warning(self):
        self._pin(self._stub("exit 2"))
        catalog = self.home / "catalog" / "recipes"
        captured = io.StringIO()
        with contextlib.redirect_stderr(captured):
            bindings = self.mod.resolve_bindings(catalog, ["test-tracker-ledger"], [])
        self.assertEqual(bindings, {"tracker": "test-tracker-ledger"})
        stderr = captured.getvalue()
        self.assertEqual(stderr.count(self.mod.GO_BINDINGS_BRIDGE_FALLBACK), 1, stderr)
        self.assertIn("exited 2", stderr)

    def test_unreadable_binary_output_falls_back_with_one_warning(self):
        self._pin(self._stub("echo not-json"))
        catalog = self.home / "catalog" / "recipes"
        captured = io.StringIO()
        with contextlib.redirect_stderr(captured):
            bindings = self.mod.resolve_bindings(catalog, ["test-tracker-ledger"], [])
        self.assertEqual(bindings, {"tracker": "test-tracker-ledger"})
        stderr = captured.getvalue()
        self.assertEqual(stderr.count(self.mod.GO_BINDINGS_BRIDGE_FALLBACK), 1, stderr)

    def test_python_fallback_still_writes_the_bound_witness(self):
        root = self.tmp / "repo"
        (root / "ai-specs").mkdir(parents=True)
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'fallback'\n\n[agents]\nenabled = ['claude']\n\n"
            + TRACKER_RECIPE
        )
        _git(root, "init", "-q")
        captured = io.StringIO()
        with contextlib.redirect_stderr(captured):
            self.assertEqual(
                self.mod.materialize_recipes(
                    root, self.home, resolved_config_out=root / ".resolved.json"
                ),
                0,
            )
        common = self.mod.git_common_dir(root)
        witness = json.loads(
            (Path(common) / "ai-specs" / "ledger" / "witness.json").read_text()
        )
        self.assertEqual(witness["state"], "bound")
        self.assertEqual(witness["recipe_id"], "test-tracker-ledger")
        self.assertEqual(
            captured.getvalue().count(self.mod.GO_BINDINGS_BRIDGE_FALLBACK), 1
        )

    def test_python_fallback_still_writes_the_unbound_witness(self):
        root = self.tmp / "empty-repo"
        (root / "ai-specs").mkdir(parents=True)
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'fallback'\n\n[agents]\nenabled = ['claude']\n"
        )
        _git(root, "init", "-q")
        captured = io.StringIO()
        with contextlib.redirect_stderr(captured):
            self.assertEqual(self.mod.materialize_recipes(root, self.home), 0)
        common = self.mod.git_common_dir(root)
        witness = json.loads(
            (Path(common) / "ai-specs" / "ledger" / "witness.json").read_text()
        )
        self.assertEqual(witness["state"], "unbound")
        self.assertEqual(
            captured.getvalue().count(self.mod.GO_BINDINGS_BRIDGE_FALLBACK), 1
        )

    def test_fallback_path_is_marked_temporary_in_the_source(self):
        self.assertEqual(
            self.mod.GO_BINDINGS_BRIDGE_FALLBACK, "GO_BINDINGS_BRIDGE_FALLBACK"
        )
        for func in (self.mod._python_resolve_bindings, self.mod.write_tracker_witness):
            doc = func.__doc__ or ""
            self.assertIn("GO_BINDINGS_BRIDGE_FALLBACK", doc, func.__name__)
            self.assertIn("TEMPORARY", doc, func.__name__)

    def test_verified_cache_candidate_requires_its_acquisition_receipt(self):
        pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": ""})
        pin.start()
        self.addCleanup(pin.stop)
        home = self.tmp / "home"
        # The cache layout is version-keyed from the install's VERSION file.
        home.mkdir()
        (home / "VERSION").write_text("test\n")
        goos, goarch = self.gb.detect_platform()
        candidate = self.gb.cache_bin_path(home, goos=goos, goarch=goarch)
        candidate.parent.mkdir(parents=True)
        candidate.write_text("#!/bin/sh\nexit 0\n")
        candidate.chmod(0o755)
        self.assertIsNone(self.gb.resolve_verified_binary(home))
        self.gb.verification_record_path(candidate).write_text("status=verified\n")
        self.assertEqual(self.gb.resolve_verified_binary(home), candidate)


if __name__ == "__main__":
    unittest.main()
