"""Contract tests for the Go primitive-conflict bridge in ``recipe-materialize.py`` (WU2).

``worktree-gate --resolve-primitive-conflicts`` owns the primitive-conflict
DECISION: registering each enabled recipe's skill, command, and MCP primitive
ids in flag order and grading every collision fatal. ``recipe-materialize.py``
keeps the sync blocking messages at the call site and is otherwise a thin
bridge over the JSON envelope.

The Python decision survives as a TEMPORARY fail-open fallback
(``GO_PRIMITIVE_CONFLICTS_BRIDGE_FALLBACK``) and these tests pin both seams:

* the Go path is authoritative whenever a verified binary runs,
* bridge results equal the retained Python authority case by case,
* the flag contract of ``--resolve-primitive-conflicts``,
* the degraded path (missing, failing, non-JSON, malformed) falls back safely,
* an invalid recipe.toml still raises ``RecipeValidationError`` through the
  fallback (Go's lenient grading must never mask a Python schema raise).

The real Go path needs a built binary (``dist/worktree-gate-current`` or
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
RECIPE_CONFLICTS_PATH = ROOT / "lib" / "_internal" / "recipe-conflicts.py"
DIST_BINARY = ROOT / "dist" / "worktree-gate-current"
FIXTURE_CATALOG = ROOT / "tests" / "fixtures" / "recipes"


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
        "WORKTREE_GATE_BIN); the Go primitive-conflict bridge cannot be "
        "proven without it"
    )


def write_recipe(
    catalog: Path,
    directory: str,
    *,
    recipe_id: str | None = None,
    name: str | None = None,
    skills: tuple[str, ...] = (),
    commands: tuple[str, ...] = (),
    mcp: tuple[str, ...] = (),
) -> None:
    """Write a fixture-style catalog recipe providing one primitive kind."""
    recipe_dir = catalog / directory
    recipe_dir.mkdir(parents=True, exist_ok=True)
    rid = recipe_id or directory
    recipe_name = name or rid
    body = (
        f'[recipe]\nid = "{rid}"\nname = "{recipe_name}"\n'
        f'description = "fixture"\nversion = "1.0"\n'
    )
    if skills:
        entries = ",\n".join(
            f'    {{ id = "{s}", source = "bundled" }}' for s in skills
        )
        body += f"[provides]\nskills = [\n{entries},\n]\n"
    elif commands:
        entries = ",\n".join(
            f'    {{ id = "{c}", path = "commands/{c}.md" }}' for c in commands
        )
        body += f"[provides]\ncommands = [\n{entries},\n]\n"
    elif mcp:
        entries = "".join(
            f'[[provides.mcp]]\nid = "{m}"\ncommand = "npx"\n' for m in mcp
        )
        body += entries
    (recipe_dir / "recipe.toml").write_text(body)


def conflict_shape(conflicts: list) -> list[tuple[str, str, list[str], str]]:
    """Comparable projection of a primitive-conflict list, whatever produced it."""
    return [
        (c.primitive_type, c.primitive_id, sorted(c.recipes), c.severity)
        for c in conflicts
    ]


# The shipped test fixtures: one conflicting pair per primitive kind, plus a
# clean recipe. Recipe NAMES (not TOML ids), sorted, severity always fatal.
FIXTURE_ENABLED = [
    "test-conflict-a",
    "test-conflict-b",
    "test-cmd-conflict-a",
    "test-cmd-conflict-b",
    "test-mcp-conflict-a",
    "test-mcp-conflict-b",
]

EXPECTED_FIXTURE_SHAPE = [
    ("skill", "shared-skill", ["Test Conflict A", "Test Conflict B"], "fatal"),
    ("command", "shared-cmd", ["Test Cmd Conflict A", "Test Cmd Conflict B"], "fatal"),
    ("mcp", "shared-mcp", ["Test MCP Conflict A", "Test MCP Conflict B"], "fatal"),
]


def _build_cases(catalog: Path) -> None:
    write_recipe(
        catalog, "alpha", name="Alpha", skills=("skill-x",)
    )
    write_recipe(
        catalog, "beta", name="Beta", skills=("skill-x", "skill-y")
    )


class _PrimitiveConflictBridgeTestCase(unittest.TestCase):
    """Shared fixture: the module under test and its fallback authority."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_primitive_conflict_bridge"
        )

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)

    def _catalog(self) -> Path:
        catalog = self.tmp / "catalog" / "recipes"
        catalog.mkdir(parents=True)
        return catalog

    def _forbid_python_authority(self) -> None:
        """Fail loudly if a bridged call reaches the temporary fallback."""
        patcher = mock.patch.object(
            self.mod,
            "_python_check_recipe_conflicts",
            side_effect=AssertionError(
                "_python_check_recipe_conflicts ran: the bridge fell back"
            ),
        )
        patcher.start()
        self.addCleanup(patcher.stop)

    def _stub(self, body: str) -> Path:
        path = self.tmp / "worktree-gate-stub"
        path.write_text(f"#!/bin/sh\n{body}\n")
        path.chmod(0o755)
        return path


class PrimitiveConflictGoAuthorityTests(_PrimitiveConflictBridgeTestCase):
    """The Go path is authoritative whenever a verified binary runs."""

    def test_go_grades_the_conflicts_and_never_reaches_python(self):
        catalog = self._catalog()
        _build_cases(catalog)
        envelope = {
            "conflicts": [
                {
                    "type": "skill",
                    "id": "skill-x",
                    "recipes": ["Alpha", "Beta"],
                    "severity": "fatal",
                }
            ]
        }
        stub = self._stub(f"printf '%s' '{json.dumps(envelope)}'")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            self._forbid_python_authority()
            conflicts = self.mod.check_conflicts(catalog, ["alpha", "beta"])
        self.assertEqual(
            conflict_shape(conflicts),
            [("skill", "skill-x", ["Alpha", "Beta"], "fatal")],
        )

    def test_bridge_invokes_resolve_primitive_conflicts_with_ordered_recipes(self):
        catalog = self._catalog()
        _build_cases(catalog)
        argv_capture = self.tmp / "argv"
        stub = self._stub(
            f"printf '%s\\n' \"$@\" > '{argv_capture}'\n"
            "printf '%s' '{\"conflicts\": []}'"
        )
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            self.assertEqual(
                self.mod.check_conflicts(catalog, ["beta", "alpha"]), []
            )
        argv = argv_capture.read_text().splitlines()
        self.assertEqual(argv[0], "--resolve-primitive-conflicts")
        self.assertEqual(
            argv[1:],
            [
                "--catalog-dir",
                str(catalog),
                "--recipe",
                "beta",
                "--recipe",
                "alpha",
            ],
        )

    def test_empty_envelope_is_a_valid_clean_result_without_fallback(self):
        catalog = self._catalog()
        _build_cases(catalog)
        stub = self._stub("printf '%s' '{\"conflicts\": []}'")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            self._forbid_python_authority()
            captured = io.StringIO()
            with contextlib.redirect_stderr(captured):
                conflicts = self.mod.check_conflicts(catalog, ["alpha", "beta"])
        self.assertEqual(conflicts, [])
        self.assertNotIn("GO_PRIMITIVE_CONFLICTS_BRIDGE_FALLBACK", captured.getvalue())


class PrimitiveConflictParityTests(_PrimitiveConflictBridgeTestCase):
    """Bridge results equal the retained Python authority case by case."""

    def test_go_and_python_shapes_names_and_order_agree(self):
        expected = conflict_shape(
            self.mod._python_check_recipe_conflicts(FIXTURE_CATALOG, FIXTURE_ENABLED)
        )
        # Guard the fixture itself: one fatal conflict per primitive kind,
        # recipe names (not ids), sorted, skill -> command -> mcp claim order.
        self.assertEqual(expected, EXPECTED_FIXTURE_SHAPE)
        go = gate_binary()
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(go)}):
            self._forbid_python_authority()
            actual = conflict_shape(
                self.mod.check_conflicts(FIXTURE_CATALOG, FIXTURE_ENABLED)
            )
        self.assertEqual(actual, expected)

    def test_clean_catalog_returns_empty_from_go(self):
        go = gate_binary()
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(go)}):
            self._forbid_python_authority()
            actual = conflict_shape(
                self.mod.check_conflicts(FIXTURE_CATALOG, ["test-fixture"])
            )
        self.assertEqual(actual, [])

    def test_invalid_recipe_toml_falls_back_and_still_raises(self):
        """Go exits 2 on a parser failure; Python's own error still surfaces.

        The parse error the retained Python authority raises is unchanged by
        the bridge — Go's lenient grading never masks it.
        """
        import tomllib

        catalog = self._catalog()
        write_recipe(catalog, "ok", skills=("unique-skill",))
        broken = catalog / "broken"
        broken.mkdir()
        (broken / "recipe.toml").write_text('[recipe\nid = "broken"\n')
        go = gate_binary()
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(go)}):
            with self.assertRaises(tomllib.TOMLDecodeError):
                self.mod.check_conflicts(catalog, ["broken", "ok"])

    def test_missing_recipe_dir_falls_back_and_raises_validation_error(self):
        """Go exits 2 for a missing recipe dir; Python raises as it did directly."""
        catalog = self._catalog()
        write_recipe(catalog, "ok", skills=("unique-skill",))
        go = gate_binary()
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(go)}):
            with self.assertRaises(self.mod._load_conflict().RecipeValidationError):
                self.mod.check_conflicts(catalog, ["ghost", "ok"])


class PrimitiveConflictFailOpenFallbackTests(_PrimitiveConflictBridgeTestCase):
    """No usable binary: the legacy Python decision runs, with one warning."""

    def setUp(self):
        super().setUp()
        # No binary anywhere: an empty override short-circuits the cache lookup.
        no_pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": ""})
        no_pin.start()
        self.addCleanup(no_pin.stop)
        self.catalog = self._catalog()
        _build_cases(self.catalog)

    def _run(self, recipe_ids: list[str] | None = None) -> tuple[list, str]:
        captured = io.StringIO()
        with contextlib.redirect_stderr(captured):
            conflicts = self.mod.check_conflicts(
                self.catalog, ["alpha", "beta"] if recipe_ids is None else recipe_ids
            )
        return conflicts, captured.getvalue()

    def _assert_fallback(self, stderr: str) -> None:
        self.assertEqual(
            stderr.count(self.mod.GO_PRIMITIVE_CONFLICTS_BRIDGE_FALLBACK), 1, stderr
        )
        self.assertIn("temporary", stderr.lower())

    def test_missing_binary_falls_back_to_python_with_one_warning(self):
        conflicts, stderr = self._run()
        self._assert_fallback(stderr)
        self.assertIn("no verified worktree-gate binary", stderr)
        self.assertEqual(
            conflict_shape(conflicts),
            [("skill", "skill-x", ["Alpha", "Beta"], "fatal")],
        )

    def test_binary_that_fails_falls_back_with_one_warning(self):
        stub = self._stub("echo boom >&2\nexit 2")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            conflicts, stderr = self._run()
        self._assert_fallback(stderr)
        self.assertIn("exited 2", stderr)
        self.assertTrue(conflicts)

    def test_unreadable_binary_output_falls_back_with_one_warning(self):
        stub = self._stub("echo not-json")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            conflicts, stderr = self._run()
        self._assert_fallback(stderr)
        self.assertIn("was not JSON", stderr)
        self.assertTrue(conflicts)

    def test_malformed_envelope_falls_back_with_one_warning(self):
        stub = self._stub("printf '%s' '{\"conflicts\": [\"nope\"]}'")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            conflicts, stderr = self._run()
        self._assert_fallback(stderr)
        self.assertIn("did not match the primitive-conflict envelope", stderr)
        self.assertTrue(conflicts)

    def test_envelope_item_without_type_defaults_to_capability(self):
        envelope = {
            "conflicts": [
                {"id": "skill-x", "recipes": ["Alpha", "Beta"], "severity": "warning"}
            ]
        }
        stub = self._stub(f"printf '%s' '{json.dumps(envelope)}'")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            conflicts, stderr = self._run()
        self.assertNotIn("GO_PRIMITIVE_CONFLICTS_BRIDGE_FALLBACK", stderr)
        self.assertEqual(
            conflict_shape(conflicts),
            [("capability", "skill-x", ["Alpha", "Beta"], "warning")],
        )

    def test_fallback_path_is_marked_temporary_in_the_source(self):
        self.assertEqual(
            self.mod.GO_PRIMITIVE_CONFLICTS_BRIDGE_FALLBACK,
            "GO_PRIMITIVE_CONFLICTS_BRIDGE_FALLBACK",
        )
        doc = self.mod._python_check_recipe_conflicts.__doc__ or ""
        self.assertIn("TEMPORARY", doc)
        self.assertIn("GO_PRIMITIVE_CONFLICTS_BRIDGE_FALLBACK", doc)


if __name__ == "__main__":
    unittest.main()
