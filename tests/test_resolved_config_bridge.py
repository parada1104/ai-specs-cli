"""Contract tests for the Go resolved-config bridge in ``recipe-materialize.py`` (T4).

``worktree-gate --plan-resolved-config`` owns the resolved-config PROJECTION: the
capability bindings, the per-recipe config (flat + ``[recipes.<id>.config]``),
the enabled id list, the resolved project root, and the resolved topology. The
Python authority survives as a TEMPORARY fail-open fallback
(``GO_RESOLVED_CONFIG_BRIDGE_FALLBACK``) and these tests pin both seams:

* the bridge result equals the retained Python authority case by case,
* the stdout envelope contract of ``--plan-resolved-config``,
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

# One manifest per projection concern. They stay small on purpose: the Go
# projection must reproduce the Python authority's byte-for-byte shape, not just
# a happy path.
FIXTURES = {
    "flat-and-config-recipes": (
        "[recipes.flat-recipe]\n"
        "enabled = true\n"
        'version = "1.0.0"\n'
        'flat_setting = "on"\n'
        "another = 7\n"
        "\n"
        "[recipes.config-recipe]\n"
        "enabled = false\n"
        'version = "2.0.0"\n'
        "\n"
        "[recipes.config-recipe.config]\n"
        'nested_setting = "yes"\n'
        "count = 3\n"
    ),
    "explicit-bindings": (
        "[project]\n"
        'name = "bindings"\n'
        "\n"
        "[recipes.recipe-a]\n"
        "enabled = true\n"
        "\n"
        "[recipes.recipe-b]\n"
        "enabled = true\n"
        "\n"
        "[[bindings]]\n"
        'capability = "tracker"\n'
        'recipe = "recipe-a"\n'
        "\n"
        "[[bindings]]\n"
        'capability = "vcs"\n'
        'recipe = "recipe-b"\n'
    ),
    "project-topology": (
        "[project]\n"
        'name = "topology"\n'
        'repo_topology = "standalone"\n'
        "\n"
        "[recipes.recipe-a]\n"
        "enabled = true\n"
    ),
    "legacy-recipe-topology": (
        "[recipes.worktree-flow]\n"
        "enabled = true\n"
        'version = "1.0.0"\n'
        "\n"
        "[recipes.worktree-flow.config]\n"
        'repo_topology = "monorepo-apps"\n'
    ),
    "empty-manifest": "# no recipes table\n",
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
        "WORKTREE_GATE_BIN); the Go resolved-config bridge cannot be proven "
        "without it"
    )


def write_manifest(root: Path, body: str) -> Path:
    """Write ``<root>/ai-specs/ai-specs.toml`` and return the project root."""
    ai_specs = root / "ai-specs"
    ai_specs.mkdir(parents=True, exist_ok=True)
    (ai_specs / "ai-specs.toml").write_text(body)
    return root


def without_project_root(result: dict) -> dict:
    """The two parity trees differ only by their own absolute path."""
    return {key: value for key, value in result.items() if key != "project_root"}


class _ResolvedBridgeTestCase(unittest.TestCase):
    """Shared fixture: the module under test, a pinned binary, an empty home."""

    @classmethod
    def setUpClass(cls):
        cls.binary = gate_binary()
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_resolved_bridge"
        )

    def setUp(self):
        pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(self.binary)})
        pin.start()
        self.addCleanup(pin.stop)
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)
        self.home = self.tmp / "home"
        self.home.mkdir()

    def _make_tree(self, name: str, manifest: str) -> Path:
        return write_manifest(self.tmp / name / "repo", manifest)

    def _forbid_python_authority(self):
        """Fails loudly if a bridged call reaches the temporary fallback."""
        return mock.patch.object(
            self.mod,
            "_python_build_resolved_config",
            side_effect=AssertionError(
                "_python_build_resolved_config ran: the bridge fell back"
            ),
        )

    def _go_result(self, root: Path) -> dict:
        with self._forbid_python_authority():
            return self.mod.build_resolved_config(root, ai_specs_home=self.home)

    def _python_result(self, root: Path) -> dict:
        with mock.patch.object(self.mod, "go_resolved_config", return_value=None):
            return self.mod.build_resolved_config(root, ai_specs_home=self.home)


class ResolvedConfigParityTests(_ResolvedBridgeTestCase):
    """Bridge results equal the Python authority that used to compute them."""

    def _assert_parity(self, name: str) -> None:
        manifest = FIXTURES[name]
        go_root = self._make_tree(f"go-{name}", manifest)
        py_root = self._make_tree(f"py-{name}", manifest)
        go_result = self._go_result(go_root)
        py_result = self._python_result(py_root)
        self.assertEqual(
            without_project_root(go_result),
            without_project_root(py_result),
            "the Go and Python projections differ",
        )
        self.assertEqual(go_result["project_root"], str(go_root.resolve()))
        self.assertEqual(py_result["project_root"], str(py_root.resolve()))

    def test_matches_the_retained_python_authority_case_by_case(self):
        for name in FIXTURES:
            with self.subTest(case=name):
                self._assert_parity(name)


class ResolvedConfigFallbackTests(unittest.TestCase):
    """No usable binary: the legacy Python projection runs, with one warning."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_resolved_bridge_fallback"
        )

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)
        self.home = self.tmp / "home"
        self.home.mkdir()
        # No binary anywhere: an empty override short-circuits the cache lookup.
        no_pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": ""})
        no_pin.start()
        self.addCleanup(no_pin.stop)
        self.root = write_manifest(
            self.tmp / "repo", FIXTURES["flat-and-config-recipes"]
        )

    def _run(self) -> tuple[dict, str]:
        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr):
            result = self.mod.build_resolved_config(self.root, ai_specs_home=self.home)
        return result, stderr.getvalue()

    def _expected_python_projection(self) -> dict:
        return self.mod._python_build_resolved_config(self.root)

    def _stub(self, body: str) -> Path:
        path = self.tmp / "worktree-gate-stub"
        path.write_text(f"#!/bin/sh\n{body}\n")
        path.chmod(0o755)
        return path

    def test_missing_binary_falls_back_to_python_with_one_warning(self):
        with mock.patch.dict(
            os.environ, {"WORKTREE_GATE_BIN": str(self.tmp / "no-such-gate")}
        ):
            result, stderr = self._run()
        self.assertEqual(
            stderr.count(self.mod.GO_RESOLVED_CONFIG_BRIDGE_FALLBACK), 1, stderr
        )
        self.assertIn("temporary", stderr.lower())
        self.assertIn("no verified worktree-gate binary", stderr)
        self.assertEqual(result, self._expected_python_projection())

    def test_non_json_output_falls_back_with_one_warning(self):
        stub = self._stub('echo "not json"')
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            result, stderr = self._run()
        self.assertEqual(
            stderr.count(self.mod.GO_RESOLVED_CONFIG_BRIDGE_FALLBACK), 1, stderr
        )
        self.assertEqual(result, self._expected_python_projection())

    def test_failing_binary_falls_back_with_one_warning(self):
        stub = self._stub("exit 3")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            result, stderr = self._run()
        self.assertEqual(
            stderr.count(self.mod.GO_RESOLVED_CONFIG_BRIDGE_FALLBACK), 1, stderr
        )
        self.assertIn("exited 3", stderr)
        self.assertEqual(result, self._expected_python_projection())


class EnvelopeContractTests(_ResolvedBridgeTestCase):
    """The stdout envelope contract of ``worktree-gate --plan-resolved-config``."""

    def test_go_envelope_carries_the_documented_types(self):
        root = self._make_tree("envelope", FIXTURES["explicit-bindings"])
        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr):
            envelope = self.mod.go_resolved_config(root, ai_specs_home=self.home)
        self.assertIsInstance(envelope, dict)
        self.assertEqual(
            set(envelope),
            {"bindings", "recipes", "enabled", "project_root", "topology"},
        )
        self.assertEqual(stderr.getvalue(), "")

        self.assertIsInstance(envelope["bindings"], dict)
        for capability, recipe in envelope["bindings"].items():
            self.assertIsInstance(capability, str)
            self.assertIsInstance(recipe, str)

        self.assertIsInstance(envelope["recipes"], dict)
        for recipe_id, config in envelope["recipes"].items():
            self.assertIsInstance(recipe_id, str)
            self.assertIsInstance(config, dict)

        self.assertIsInstance(envelope["enabled"], list)
        self.assertIsInstance(envelope["project_root"], str)

        topology = envelope["topology"]
        self.assertIsInstance(topology, dict)
        self.assertEqual(
            set(topology),
            {
                "resolved",
                "configured",
                "via",
                "source",
                "submodules",
                "gitmodules_present",
            },
        )
        for key in ("resolved", "configured", "via", "source"):
            self.assertIsInstance(topology[key], str)
        self.assertIsInstance(topology["submodules"], list)
        self.assertIsInstance(topology["gitmodules_present"], bool)


if __name__ == "__main__":
    unittest.main()
