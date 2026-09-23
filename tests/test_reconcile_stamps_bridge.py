"""Contract tests for the Go reconcile-stamps bridge in ``recipe-materialize.py`` (WU2).

``worktree-gate --plan-reconcile-stamps`` owns the per-recipe reconcile stamp
DECISION: the stamp dict each enabled recipe's declared ``[config.reconcile]``
table and config-field defaults select. Python keeps only the manifest write
(``update_recipe_config``, whose existing behavior — add missing values and
replace existing values when they differ — is unchanged by this change). The
Python decision survives as a
TEMPORARY fail-open fallback (``GO_RECONCILE_STAMPS_BRIDGE_FALLBACK``) and
these tests pin both seams:

* the bridge-applied manifest equals the retained Python authority's manifest,
* the stdout envelope contract of ``--plan-reconcile-stamps`` (enabled order,
  silent skips of unreadable or reconcile-less recipes),
* the degraded path when no verified binary can run or the envelope is wrong,
* the verbatim per-recipe writer-failure warning on both paths.

The Go path needs a built binary (``dist/worktree-gate-current`` or
``$WORKTREE_GATE_BIN``); it skips loudly when none exists. Every fallback test
runs with no usable binary at all, because failing open is the contract they pin.
"""
from __future__ import annotations

import contextlib
import importlib.util
import io
import os
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]

RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
DIST_BINARY = ROOT / "dist" / "worktree-gate-current"

# Enabled order pins the envelope order; no-reconcile/broken/ghost are the
# silent-skip shapes both authorities must omit without a warning.
ENABLED = ["stamp-min", "stamp-full", "no-reconcile", "broken", "ghost"]


def recipe_toml(recipe_id: str, name: str, config: str) -> str:
    return (
        "[recipe]\n"
        f'id = "{recipe_id}"\n'
        f'name = "{name}"\n'
        'description = "reconcile-stamps bridge fixture"\n'
        'version = "1.0.0"\n'
        "\n"
        "[config]\n"
        + config
    )


# scope_field + two expectations: one plain config_field, one with a
# config_field_when_set and a referenced field that declares no default. Pins
# the present-but-falsy defaults (false, 0), the absent default, and the
# unreferenced field.
STAMP_FULL_CONFIG = (
    "[config.reconcile]\n"
    'scope_field = "workflow"\n'
    "\n"
    "[[config.reconcile.expectations]]\n"
    'event = "pr_opened"\n'
    'property = "branch"\n'
    'config_field = "base_branch"\n'
    "\n"
    "[[config.reconcile.expectations]]\n"
    'event = "pr_merged"\n'
    'property = "state"\n'
    'config_field = "no_default_field"\n'
    'config_field_when_set = "protect_base"\n'
    "\n"
    "[config.workflow]\n"
    "required = true\n"
    'default = "feature"\n'
    "\n"
    "[config.base_branch]\n"
    "required = true\n"
    "default = false\n"
    "\n"
    "[config.protect_base]\n"
    "required = true\n"
    "default = 0\n"
    "\n"
    "[config.no_default_field]\n"
    "required = true\n"
    "\n"
    "[config.unused_field]\n"
    "required = true\n"
    'default = "never referenced"\n'
)

# Reconcile table with an empty used set: only the raw table is stamped.
STAMP_MIN_CONFIG = "[config.reconcile]\nmax_age_seconds = 3600\n"

NO_RECONCILE_CONFIG = (
    "[config.other]\n"
    "required = true\n"
    'default = "no reconcile table"\n'
)

EXPECTED_STAMP_FULL = {
    "reconcile": {
        "scope_field": "workflow",
        "expectations": [
            {"event": "pr_opened", "property": "branch", "config_field": "base_branch"},
            {
                "event": "pr_merged",
                "property": "state",
                "config_field": "no_default_field",
                "config_field_when_set": "protect_base",
            },
        ],
    },
    "workflow": "feature",
    "base_branch": False,
    "protect_base": 0,
}

EXPECTED_STAMP_MIN = {"reconcile": {"max_age_seconds": 3600}}

# A pre-existing unrelated project key the stamp must not touch; absent keys
# (reconcile, base_branch, protect_base) are the ones stamping adds.
MANIFEST = (
    "[recipes.stamp-full]\n"
    "enabled = true\n"
    "\n"
    "[recipes.stamp-full.config]\n"
    'unrelated = "kept"\n'
    "\n"
    "[recipes.stamp-min]\n"
    "enabled = true\n"
)

EXPECTED_CONFIGS = {
    "stamp-full": {**EXPECTED_STAMP_FULL, "unrelated": "kept"},
    "stamp-min": EXPECTED_STAMP_MIN,
}


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
        "WORKTREE_GATE_BIN); the Go reconcile-stamps bridge cannot be proven "
        "without it"
    )


def write_catalog(catalog: Path, recipe_ids=()) -> None:
    """A temporary recipe catalog; ``recipe_ids`` restricts which fixtures land."""
    wanted = set(recipe_ids) if recipe_ids else {"stamp-min", "stamp-full", "no-reconcile", "broken"}
    catalog.mkdir(parents=True, exist_ok=True)
    for recipe_id, name, config in (
        ("stamp-min", "stamp-min", STAMP_MIN_CONFIG),
        ("stamp-full", "Stamp Full Fixture", STAMP_FULL_CONFIG),
        ("no-reconcile", "no-reconcile", NO_RECONCILE_CONFIG),
        ("broken", "broken", "not = [valid\n"),
    ):
        if recipe_id not in wanted:
            continue
        recipe_dir = catalog / recipe_id
        recipe_dir.mkdir(parents=True, exist_ok=True)
        (recipe_dir / "recipe.toml").write_text(recipe_toml(recipe_id, name, config))
    # "ghost" has no directory at all.


def write_manifest(root: Path, body: str) -> Path:
    (root / "ai-specs").mkdir(parents=True, exist_ok=True)
    (root / "ai-specs" / "ai-specs.toml").write_text(body)
    return root


def read_configs(root: Path) -> dict:
    """The stamped ``[recipes.<id>.config]`` tables of a manifest."""
    with (root / "ai-specs" / "ai-specs.toml").open("rb") as fh:
        data = tomllib.load(fh)
    return {
        rid: (recipe or {}).get("config", {})
        for rid, recipe in (data.get("recipes") or {}).items()
    }


class _GoBridgeTestCase(unittest.TestCase):
    """Shared fixture for tests that pin the Go path as the authority."""

    @classmethod
    def setUpClass(cls):
        cls.binary = gate_binary()
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_reconcile_stamps"
        )

    def setUp(self):
        pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(self.binary)})
        pin.start()
        self.addCleanup(pin.stop)
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)
        self.catalog = self.tmp / "catalog" / "recipes"
        write_catalog(self.catalog)
        self.root = write_manifest(self.tmp / "repo", MANIFEST)

    def _forbid_python_authority(self):
        """Fails loudly if a bridged call reaches the temporary fallback."""
        return mock.patch.object(
            self.mod,
            "_stamp_recipe_reconcile_defaults_python",
            side_effect=AssertionError(
                "_stamp_recipe_reconcile_defaults_python ran: the bridge fell back"
            ),
        )

    def _stamp_at(self, root: Path) -> None:
        self.mod.stamp_recipe_reconcile_defaults(root, self.catalog, ENABLED)

    def _stamp(self) -> None:
        self._stamp_at(self.root)


class GoReconcileStampsAuthorityTests(_GoBridgeTestCase):
    """The Go path is authoritative whenever a verified binary runs."""

    def test_go_authority_stamps_exactly_like_the_python_authority(self):
        py_root = write_manifest(self.tmp / "py-repo", MANIFEST)
        with self._forbid_python_authority():
            self._stamp()
        with mock.patch.object(self.mod, "go_reconcile_stamps", return_value=None):
            self.mod.stamp_recipe_reconcile_defaults(py_root, self.catalog, ENABLED)
        self.assertEqual(read_configs(self.root), EXPECTED_CONFIGS)
        self.assertEqual(read_configs(py_root), EXPECTED_CONFIGS)

    def test_envelope_keeps_enabled_order_and_silently_skips(self):
        stamps = self.mod.go_reconcile_stamps(self.catalog, ENABLED)
        self.assertEqual([entry["id"] for entry in stamps], ["stamp-min", "stamp-full"])
        self.assertEqual(stamps[1]["stamp"], EXPECTED_STAMP_FULL)
        self.assertEqual(stamps[0]["stamp"], EXPECTED_STAMP_MIN)
        self.assertEqual(
            self.mod.GO_RECONCILE_STAMPS_BRIDGE_TIMEOUT_SECONDS, 60
        )

    def test_fully_skipped_catalog_writes_nothing_and_warns_nothing(self):
        quiet_catalog = self.tmp / "quiet" / "recipes"
        write_catalog(quiet_catalog, recipe_ids=("no-reconcile", "broken"))
        stderr = io.StringIO()
        with self._forbid_python_authority():
            with contextlib.redirect_stderr(stderr):
                self.mod.stamp_recipe_reconcile_defaults(
                    self.root, quiet_catalog, ENABLED
                )
        self.assertEqual(stderr.getvalue(), "")
        self.assertEqual(
            read_configs(self.root),
            {"stamp-full": {"unrelated": "kept"}, "stamp-min": {}},
        )


class ReconcileStampsFallbackTests(unittest.TestCase):
    """No usable Go authority: the legacy Python decision runs, with one warning."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_reconcile_stamps_fallback"
        )

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)
        self.catalog = self.tmp / "catalog" / "recipes"
        write_catalog(self.catalog)
        # WORKTREE_GATE_BIN="" is falsy, so resolution falls through to the
        # temporary catalog home's cache path, which has no binary in this test.
        no_pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": ""})
        no_pin.start()
        self.addCleanup(no_pin.stop)

    def _stamp(self, root: Path) -> str:
        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr):
            self.mod.stamp_recipe_reconcile_defaults(root, self.catalog, ENABLED)
        return stderr.getvalue()

    def _stub(self, body: str) -> Path:
        path = self.tmp / "worktree-gate-stub"
        path.write_text(f"#!/bin/sh\n{body}\n")
        path.chmod(0o755)
        return path

    def test_unusable_go_authority_falls_back_with_one_warning(self):
        stubs = {
            "missing-binary": self.tmp / "no-such-gate",
            "nonzero-exit": self._stub("exit 2"),
            "non-json": self._stub('echo "not json"'),
            "wrong-envelope": self._stub("printf '%s' '{\"stamps\": {\"id\": \"x\"}}'"),
        }
        for case, stub in stubs.items():
            with self.subTest(case=case):
                root = write_manifest(self.tmp / f"repo-{case}", MANIFEST)
                with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
                    stderr = self._stamp(root)
                self.assertEqual(
                    stderr.count(self.mod.GO_RECONCILE_STAMPS_BRIDGE_FALLBACK),
                    1,
                    stderr,
                )
                self.assertEqual(read_configs(root), EXPECTED_CONFIGS)
    def test_invalid_utf8_output_falls_back_with_one_warning(self):
        # text=True decodes stdout strictly; invalid bytes must not escape the
        # bridge as an uncaught UnicodeDecodeError — one fallback warning, then
        # the Python authority still stamps.
        stub = self._stub("printf '\\377\\376not utf8'")
        root = write_manifest(self.tmp / "repo-invalid-utf8", MANIFEST)
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            stderr = self._stamp(root)
        self.assertEqual(
            stderr.count(self.mod.GO_RECONCILE_STAMPS_BRIDGE_FALLBACK),
            1,
            stderr,
        )
        self.assertEqual(read_configs(root), EXPECTED_CONFIGS)


class ReconcileStampsWriterFailureTests(_GoBridgeTestCase):
    """A failing manifest write warns once per recipe, verbatim, on both paths."""

    def test_writer_failure_warns_verbatim_and_keeps_other_stamps(self):
        writer_mod = self.mod._load_recipe_config_write()
        real_write = writer_mod.update_recipe_config

        def flaky(manifest_path, recipe_id, values):
            if recipe_id == "stamp-full":
                raise OSError("disk full")
            real_write(manifest_path, recipe_id, values)

        for go_enabled in (True, False):
            with self.subTest(go_authority=go_enabled):
                root = write_manifest(self.tmp / f"flaky-{go_enabled}", MANIFEST)
                stderr = io.StringIO()
                with mock.patch.object(
                    writer_mod, "update_recipe_config", side_effect=flaky
                ):
                    with contextlib.redirect_stderr(stderr):
                        if not go_enabled:
                            with mock.patch.object(
                                self.mod, "go_reconcile_stamps", return_value=None
                            ):
                                self._stamp_at(root)
                        else:
                            with self._forbid_python_authority():
                                self._stamp_at(root)
                self.assertEqual(
                    stderr.getvalue(),
                    "  ! recipe 'Stamp Full Fixture': reconcile defaults not stamped "
                    "(OSError: disk full)\n",
                )
                # The surviving recipe is still stamped; only the failed write is lost.
                self.assertEqual(
                    read_configs(root),
                    {
                        "stamp-full": {"unrelated": "kept"},
                        "stamp-min": EXPECTED_STAMP_MIN,
                    },
                )


if __name__ == "__main__":
    unittest.main()
