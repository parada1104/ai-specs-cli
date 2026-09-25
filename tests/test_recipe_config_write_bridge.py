"""Contract tests for the Go config-write bridge in ``recipe-config-write.py`` (GO-07 WU2).

``worktree-gate --write-recipe-config`` owns the surgical ``[recipes.<id>.config]``
WRITE: the Go port of ``update_recipe_config`` performs the line surgery and the
manifest write. Python keeps the retained surgical writer as a TEMPORARY
fail-open fallback (``GO_RECIPE_CONFIG_BRIDGE_FALLBACK``) and these tests pin
both seams:

* the bridge-applied manifest equals the retained Python writer's manifest,
* the stdout envelope contract (``{"applied": true|false}`` on exit 0,
  ``{"error": "<exact refusal string>"}`` on exit 2),
* fail closed on a Go refusal: ``RecipeConfigWriteError`` with the exact Go
  string and NO fallback run,
* fail open on infrastructure failures only: one
  ``GO_RECIPE_CONFIG_BRIDGE_FALLBACK`` warning, then the retained Python writer
  produces byte-identical output,
* the serialization TypeError of the values is NOT a bridge failure: it falls
  back silently so the original ``toml_value`` TypeError surfaces unchanged.

The Go path needs a built binary (``dist/worktree-gate-current`` or
``$WORKTREE_GATE_BIN``); it skips loudly when none exists. Every fallback test
runs with no usable binary at all, because failing open is the contract they pin.
"""
from __future__ import annotations

import contextlib
import importlib.util
import io
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]

WRITE_PATH = ROOT / "lib" / "_internal" / "recipe-config-write.py"
DIST_BINARY = ROOT / "dist" / "worktree-gate-current"

MANIFEST = (
    "[recipes.trello-mcp-workflow]\n"
    "enabled = true\n"
    "\n"
    "[recipes.trello-mcp-workflow.config]\n"
    'default_list = "To Do"  # keep-me\n'
    "\n"
    "[recipes.other]\n"
    "enabled = false\n"
)

VALUES = {"default_list": "In Progress"}
EXPECTED_MANIFEST = MANIFEST.replace('default_list = "To Do"', 'default_list = "In Progress"')

# A whole-table and a dotted update for the same root: the exact refusal both
# authorities must raise verbatim (fail closed, no fallback).
CONFLICT_VALUES = {"reconcile": {"scope_field": "workflow"}, "reconcile.max_age_seconds": 5}
CONFLICT_REFUSAL = "cannot combine a whole-table and a dotted update for key(s): reconcile"


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
        "WORKTREE_GATE_BIN); the Go config-write bridge cannot be proven "
        "without it"
    )


class _BridgeTestCase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(WRITE_PATH, "recipe_config_write_bridge")

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)

    def manifest(self, text: str, name: str = "ai-specs.toml") -> Path:
        path = self.tmp / name
        path.write_text(text, encoding="utf-8")
        return path

    def stub(self, body: str) -> Path:
        path = self.tmp / f"worktree-gate-stub-{abs(hash(body))}"
        path.write_text(f"#!/bin/sh\n{body}\n")
        path.chmod(0o755)
        return path

    def pin_binary(self, path: Path) -> None:
        pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(path)})
        pin.start()
        self.addCleanup(pin.stop)

    def run_write(self, manifest_path: Path, values=VALUES) -> tuple[str, object]:
        """Run update_recipe_config capturing stderr; return (stderr, exc_info)."""
        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr):
            try:
                self.mod.update_recipe_config(manifest_path, "trello-mcp-workflow", values)
                error = None
            except Exception as exc:  # noqa: BLE001 - the test asserts on the type
                error = exc
        return stderr.getvalue(), error

    def forbid_python_writer(self):
        """Fails loudly if a bridged call reaches the temporary fallback."""
        return mock.patch.object(
            self.mod,
            "_update_recipe_config_python",
            side_effect=AssertionError(
                "_update_recipe_config_python ran: the bridge fell back"
            ),
        )

    def reference_bytes(self, values=VALUES) -> bytes:
        """What the retained pure Python writer alone produces for a fresh manifest."""
        path = self.manifest(MANIFEST, name="reference.toml")
        self.mod._update_recipe_config_python(path, "trello-mcp-workflow", values)
        return path.read_bytes()


class GoConfigWriteAuthorityTests(_BridgeTestCase):
    """The Go path is authoritative whenever a verified binary runs."""

    @classmethod
    def setUpClass(cls):
        super().setUpClass()
        cls.binary = gate_binary()

    def setUp(self):
        super().setUp()
        self.pin_binary(self.binary)

    def test_go_bridge_applies_values_without_the_python_writer(self):
        path = self.manifest(MANIFEST)
        with self.forbid_python_writer():
            stderr, error = self.run_write(path)
        self.assertIsNone(error)
        self.assertEqual(stderr, "")
        self.assertEqual(path.read_text(encoding="utf-8"), EXPECTED_MANIFEST)
        self.assertEqual(self.mod.GO_RECIPE_CONFIG_BRIDGE_TIMEOUT_SECONDS, 60)

    def test_go_bridge_noop_writes_nothing_without_the_python_writer(self):
        path = self.manifest(MANIFEST)
        before = path.read_bytes()
        with self.forbid_python_writer():
            stderr, error = self.run_write(path, values={"default_list": "To Do"})
        self.assertIsNone(error)
        self.assertEqual(stderr, "")
        self.assertEqual(path.read_bytes(), before)

    def test_go_bridge_refusal_fails_closed_with_the_exact_string(self):
        path = self.manifest(MANIFEST)
        stderr, error = self.run_write(path, values=CONFLICT_VALUES)
        self.assertIsInstance(error, self.mod.RecipeConfigWriteError)
        self.assertEqual(str(error), CONFLICT_REFUSAL)
        self.assertNotIn(self.mod.GO_RECIPE_CONFIG_BRIDGE_FALLBACK, stderr)
        # A refused write must leave the manifest untouched.
        self.assertEqual(path.read_text(encoding="utf-8"), MANIFEST)

    def test_empty_values_short_circuits_before_any_authority(self):
        path = self.manifest(MANIFEST)
        before = path.read_bytes()
        with self.forbid_python_writer():
            with mock.patch.object(
                self.mod,
                "go_update_recipe_config",
                side_effect=AssertionError("the bridge ran for an empty no-op"),
            ):
                stderr, error = self.run_write(path, values={})
        self.assertIsNone(error)
        self.assertEqual(stderr, "")
        self.assertEqual(path.read_bytes(), before)


class ConfigWriteRefusalEnvelopeTests(_BridgeTestCase):
    """Any exit-2 stdout error envelope is a Go refusal: fail closed, no fallback."""

    def test_exit_two_error_envelope_raises_without_fallback(self):
        self.pin_binary(self.stub("printf '%s' '{\"error\": \"boom refusal\"}'; exit 2"))
        path = self.manifest(MANIFEST)
        stderr, error = self.run_write(path)
        self.assertIsInstance(error, self.mod.RecipeConfigWriteError)
        self.assertEqual(str(error), "boom refusal")
        self.assertNotIn(self.mod.GO_RECIPE_CONFIG_BRIDGE_FALLBACK, stderr)
        self.assertEqual(path.read_text(encoding="utf-8"), MANIFEST)


class ConfigWriteFallbackTests(_BridgeTestCase):
    """No usable Go authority: the retained Python writer runs, with one warning."""

    INFRASTRUCTURE_CASES = {
        # A pinned-but-missing binary: resolution step 1 yields None.
        "missing-binary": lambda self: self.pin_binary(self.tmp / "no-such-gate"),
        # An unloadable gate_binary helper: resolution raises.
        "resolution-raises": lambda self: mock.patch.object(
            self.mod, "_load_gate_binary", side_effect=RuntimeError("unloadable")
        ),
        # The process never starts.
        "oserror": lambda self: mock.patch.object(
            self.mod.subprocess, "run", side_effect=OSError("no exec")
        ),
        # The process outlives the bridge timeout.
        "subprocess-timeout": lambda self: mock.patch.object(
            self.mod.subprocess,
            "run",
            side_effect=subprocess.TimeoutExpired(cmd="gate", timeout=60),
        ),
        # text=True decodes stdout strictly; invalid bytes must not escape.
        "invalid-utf8": lambda self: self.pin_binary(
            self.stub("printf '\\377\\376not utf8'")
        ),
        # Nonzero exit with no stdout error envelope: an infra diagnostic path.
        "exit-two-no-envelope": lambda self: self.pin_binary(
            self.stub("echo 'infra diagnostic' >&2; exit 2")
        ),
        # Non-JSON stdout on a successful exit.
        "non-json": lambda self: self.pin_binary(self.stub("echo 'not json'")),
        # The documented applied envelope shape, violated.
        "wrong-envelope": lambda self: self.pin_binary(
            self.stub("printf '%s' '{\"applied\": \"yes\"}'")
        ),
    }

    def test_infrastructure_failures_fall_back_with_one_warning(self):
        for case, prepare in self.INFRASTRUCTURE_CASES.items():
            with self.subTest(case=case):
                prepare(self)
                path = self.manifest(MANIFEST, name=f"{case}.toml")
                stderr, error = self.run_write(path)
                self.assertIsNone(error)
                self.assertEqual(
                    stderr.count(self.mod.GO_RECIPE_CONFIG_BRIDGE_FALLBACK),
                    1,
                    stderr,
                )
                self.assertEqual(path.read_bytes(), self.reference_bytes())

    def test_fallback_warning_matches_the_bridge_family_format(self):
        self.pin_binary(self.tmp / "no-such-gate")
        path = self.manifest(MANIFEST)
        stderr, _error = self.run_write(path)
        (line,) = [ln for ln in stderr.splitlines() if self.mod.GO_RECIPE_CONFIG_BRIDGE_FALLBACK in ln]
        self.assertTrue(
            line.startswith(f"  ! {self.mod.GO_RECIPE_CONFIG_BRIDGE_FALLBACK}: "),
            line,
        )
        self.assertTrue(
            line.endswith("; using the temporary Python config-write authority"),
            line,
        )

    def test_unserializable_values_fall_back_silently_to_the_python_typeerror(self):
        # json.dumps TypeError is NOT a bridge failure: no warning, and the
        # retained writer's own toml_value TypeError surfaces unchanged.
        path = self.manifest(MANIFEST)
        stderr, error = self.run_write(path, values={"default_list": {1, 2}})
        self.assertIsInstance(error, TypeError)
        self.assertEqual(stderr, "")

    def test_fallback_bytes_equal_the_pure_python_writer(self):
        self.pin_binary(self.tmp / "no-such-gate")
        path = self.manifest(MANIFEST)
        stderr, _error = self.run_write(path)
        self.assertEqual(stderr.count(self.mod.GO_RECIPE_CONFIG_BRIDGE_FALLBACK), 1)
        self.assertEqual(path.read_bytes(), self.reference_bytes())


if __name__ == "__main__":
    unittest.main()
