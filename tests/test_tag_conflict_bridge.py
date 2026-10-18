"""Contract tests for the Go tag-conflict bridge in ``recipe-materialize.py`` (T2).

``worktree-gate --resolve-tag-conflicts`` owns the advisory tag-conflict
DECISION: grouping enabled recipes by first-seen tag, deduplicating recipe ids,
and grading each overlap warning/fatal. ``recipe-materialize.py`` keeps the
warning text and the advisory exit behavior at the call site, and is otherwise a
thin bridge over the JSON envelope.

The Python decision survives as a TEMPORARY fail-open fallback
(``GO_TAG_CONFLICTS_BRIDGE_FALLBACK``) and these tests pin both seams:

* the Go path is authoritative whenever a verified binary runs,
* bridge results equal the retained Python authority case by case,
* the flag contract of ``--resolve-tag-conflicts``,
* the degraded path (missing, failing, non-JSON, malformed) falls back safely.

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
        "WORKTREE_GATE_BIN); the Go tag-conflict bridge cannot be proven without it"
    )


def write_tag_recipe(
    catalog: Path,
    directory: str,
    *,
    recipe_id: str | None = None,
    tags: tuple[str, ...] = (),
    conflicts_with: tuple[str, ...] = (),
) -> None:
    """Write a catalog recipe's top-level [recipe] tag/conflict declaration.

    ``directory`` names the catalog folder; ``recipe_id`` lets a case prove the
    grader reads the TOML id, never the directory name.
    """
    recipe_dir = catalog / directory
    recipe_dir.mkdir(parents=True, exist_ok=True)
    rid = recipe_id or directory
    tags_toml = json.dumps(list(tags))
    conflicts_toml = json.dumps(list(conflicts_with))
    (recipe_dir / "recipe.toml").write_text(
        f'[recipe]\nid = "{rid}"\nname = "{rid}"\ndescription = "fixture"\n'
        f'version = "1.0"\n'
        f"tags = {tags_toml}\n"
        f"conflicts_with = {conflicts_toml}\n"
    )


def conflict_shape(conflicts: list) -> list[tuple[str, list[str], str]]:
    """Comparable projection of a tag-conflict list, whatever produced it."""
    return [
        (c.tag, sorted(c.recipes), getattr(c, "severity", "warning"))
        for c in conflicts
    ]


# Cases every path must agree on: the TOML id wins over the directory name,
# warning vs fatal, and first-seen tag order. Alpha declares conflicts_with beta,
# so both tags the pair shares grade fatal; pair-x/pair-y share a plain tag.
def _build_cases(catalog: Path) -> None:
    write_tag_recipe(
        catalog,
        "dir-alpha",
        recipe_id="alpha",
        tags=("vcs", "flow"),
        conflicts_with=("beta",),
    )
    write_tag_recipe(catalog, "beta", tags=("flow", "vcs"))
    write_tag_recipe(catalog, "pair-x", tags=("color",))
    write_tag_recipe(catalog, "pair-y", tags=("color",))
    write_tag_recipe(catalog, "solo", tags=("solo-tag",))
    write_tag_recipe(catalog, "dup", tags=("self-tag", "self-tag"))
    write_tag_recipe(catalog, "dup-peer", tags=("self-tag", "self-tag"))


ENABLED = ["dir-alpha", "beta", "pair-x", "pair-y", "solo", "dup", "dup-peer"]

EXPECTED_SHAPE = [
    ("vcs", ["alpha", "beta"], "fatal"),
    ("flow", ["alpha", "beta"], "fatal"),
    ("color", ["pair-x", "pair-y"], "warning"),
    ("self-tag", ["dup", "dup-peer"], "warning"),
]


class _TagConflictBridgeTestCase(unittest.TestCase):
    """Shared fixture: the module under test and its fallback authority."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_tag_conflict_bridge"
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
            "_python_check_tag_conflicts",
            side_effect=AssertionError(
                "_python_check_tag_conflicts ran: the bridge fell back"
            ),
        )
        patcher.start()
        self.addCleanup(patcher.stop)

    def _stub(self, body: str) -> Path:
        path = self.tmp / "worktree-gate-stub"
        path.write_text(f"#!/bin/sh\n{body}\n")
        path.chmod(0o755)
        return path


class TagConflictGoAuthorityTests(_TagConflictBridgeTestCase):
    """The Go path is authoritative whenever a verified binary runs."""

    def test_go_grades_the_conflicts_and_never_reaches_python(self):
        catalog = self._catalog()
        _build_cases(catalog)
        envelope = {
            "conflicts": [
                {
                    "type": "tag_conflict",
                    "tag": "vcs",
                    "recipes": ["alpha", "beta"],
                    "severity": "fatal",
                }
            ]
        }
        stub = self._stub(f"printf '%s' '{json.dumps(envelope)}'")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            self._forbid_python_authority()
            conflicts = self.mod.check_tag_conflicts(catalog, ENABLED)
        self.assertEqual(
            conflict_shape(conflicts), [("vcs", ["alpha", "beta"], "fatal")]
        )

    def test_bridge_invokes_resolve_tag_conflicts_with_ordered_recipes(self):
        catalog = self._catalog()
        _build_cases(catalog)
        argv_capture = self.tmp / "argv"
        stub = self._stub(
            f"printf '%s\\n' \"$@\" > '{argv_capture}'\n"
            "printf '%s' '{\"conflicts\": []}'"
        )
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            self.assertEqual(self.mod.check_tag_conflicts(catalog, ["beta", "solo"]), [])
        argv = argv_capture.read_text().splitlines()
        self.assertEqual(argv[0], "--resolve-tag-conflicts")
        self.assertEqual(
            argv[1:],
            [
                "--catalog-dir",
                str(catalog),
                "--recipe",
                "beta",
                "--recipe",
                "solo",
            ],
        )


class TagConflictParityTests(_TagConflictBridgeTestCase):
    """Bridge results equal the retained Python authority case by case."""

    def test_go_and_python_shapes_order_and_severity_agree(self):
        catalog = self._catalog()
        _build_cases(catalog)
        expected = conflict_shape(self.mod._python_check_tag_conflicts(catalog, ENABLED))
        # Guard the fixture itself: warning + fatal both present, first-seen
        # tag order (vcs before flow before color), self-tag not a conflict.
        self.assertEqual(expected, EXPECTED_SHAPE)
        go = gate_binary()
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(go)}):
            self._forbid_python_authority()
            actual = conflict_shape(self.mod.check_tag_conflicts(catalog, ENABLED))
        self.assertEqual(actual, expected)

    def test_go_reads_the_toml_id_not_the_directory_name(self):
        catalog = self._catalog()
        write_tag_recipe(
            catalog, "dir-only", recipe_id="real-id", tags=("shared",)
        )
        write_tag_recipe(catalog, "other", tags=("shared",))
        expected = conflict_shape(
            self.mod._python_check_tag_conflicts(catalog, ["dir-only", "other"])
        )
        self.assertEqual(expected, [("shared", ["other", "real-id"], "warning")])
        go = gate_binary()
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(go)}):
            self._forbid_python_authority()
            actual = conflict_shape(
                self.mod.check_tag_conflicts(catalog, ["dir-only", "other"])
            )
        self.assertEqual(actual, expected)


class TagConflictFailOpenFallbackTests(_TagConflictBridgeTestCase):
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
            conflicts = self.mod.check_tag_conflicts(
                self.catalog, ENABLED if recipe_ids is None else recipe_ids
            )
        return conflicts, captured.getvalue()

    def _assert_fallback(self, stderr: str) -> None:
        self.assertEqual(
            stderr.count(self.mod.GO_TAG_CONFLICTS_BRIDGE_FALLBACK), 1, stderr
        )
        self.assertIn("temporary", stderr.lower())

    def test_missing_binary_falls_back_to_python_with_one_warning(self):
        conflicts, stderr = self._run()
        self._assert_fallback(stderr)
        self.assertIn("no verified worktree-gate binary", stderr)
        self.assertEqual(conflict_shape(conflicts), EXPECTED_SHAPE)

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
        stub = self._stub("printf '%s' '{\"conflicts\": \"nope\"}'")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            conflicts, stderr = self._run()
        self._assert_fallback(stderr)
        self.assertIn("did not match the tag-conflict envelope", stderr)
        self.assertTrue(conflicts)

    def test_fallback_path_is_marked_temporary_in_the_source(self):
        self.assertEqual(
            self.mod.GO_TAG_CONFLICTS_BRIDGE_FALLBACK,
            "GO_TAG_CONFLICTS_BRIDGE_FALLBACK",
        )
        doc = self.mod._python_check_tag_conflicts.__doc__ or ""
        self.assertIn("TEMPORARY", doc)
        self.assertIn("GO_TAG_CONFLICTS_BRIDGE_FALLBACK", doc)


if __name__ == "__main__":
    unittest.main()
