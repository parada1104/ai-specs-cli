"""Self-tests for the differential parity harness (card [Go 03]).

Includes negative mutation tests: the RED evidence that the harness FAILS
when the two legs genuinely differ. Stdlib only.
"""
from __future__ import annotations

import contextlib
import hashlib
import io
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent / "parity"))
sys.path.insert(0, str(Path(__file__).resolve().parent))
import parity  # noqa: E402
import run as parity_run  # noqa: E402  (tests/parity/run.py; no stdlib collision)


def _help_legacy() -> str:
    # Simulates the legacy heredoc quirk: the backticked `ai-specs` was
    # command-substituted at runtime with a multi-line hub status dump.
    return ("ai-specs — declarative per-project AI agent config\n"
            "  hub [path] Interactive status + command menu (also bare "
            "some injected\nmulti-line hub status output\n"
            "\n  help Show this help\nRepo: x\n")


def _help_go() -> str:
    # Go (card 04) prints the literal backticked name plus its own wrapping.
    return ("ai-specs — declarative per-project AI agent config\n"
            "  hub [path] Interactive status + command menu (also bare `ai-specs`;\n"
            "               can rewrite ai-specs.toml and run commands)\n"
            "  help Show this help\nRepo: x\n")


class NormalizationTests(unittest.TestCase):
    def test_n2_help_cmdsubst_collapses_volatile_region(self):
        legacy = parity.apply_normalizations("stdout", _help_legacy(), {})
        go = parity.apply_normalizations("stdout", _help_go(), {})
        self.assertEqual(legacy, go)
        self.assertIn("<HELP_CMD_SUBST>", legacy)

    def test_n2_leaves_unanchored_text_unchanged(self):
        text = "regular output\nwithout the hub prefix\n"
        self.assertEqual(parity.apply_normalizations("stdout", text, {}), text)

    def test_n1_temp_paths_replaced(self):
        project_root = "/tmp/parity-xyz/project"
        home = "/tmp/parity-xyz/home"
        scratch = "/tmp/parity-xyz"
        text = (f"root={project_root} home={home} scratch={scratch} "
                "and a /private/var/folders/xx/x alias\nkeep: ai-specs.toml\n")
        out = parity.apply_normalizations("stdout", text, {
            "project_root": project_root, "home": home, "scratch": scratch})
        self.assertEqual(
            out, "root=<TEMP> home=<TEMP> scratch=<TEMP> and a <TEMP> alias\n"
                 "keep: ai-specs.toml\n")

    def test_n6_help_stderr_replaced_unconditionally(self):
        for text in ("  ! GO_X_FALLBACK: blah\n", ""):
            with self.subTest(text=text):
                self.assertEqual(
                    parity.apply_normalizations(
                        "stderr", text, {"argv": ("help",)}),
                    "<HELP_CMD_SUBST_STDERR>\n")

    def test_n6_other_verbs_and_no_argv_pass_through(self):
        text = "  ! GO_X_FALLBACK: blah\n"
        self.assertEqual(
            parity.apply_normalizations("stderr", text, {"argv": ("doctor",)}),
            text)
        self.assertEqual(
            parity.apply_normalizations("stderr", text, {}), text)


class CorpusTests(unittest.TestCase):
    def test_fixture_names_unique(self):
        names = [f.name for f in parity.CORPUS]
        self.assertEqual(len(names), len(set(names)))

    def test_every_fixture_has_steps_and_setup(self):
        for fixture in parity.CORPUS:
            with self.subTest(fixture=fixture.name):
                self.assertTrue(fixture.steps)
                self.assertTrue(callable(fixture.setup))

    def test_every_step_has_argv(self):
        for fixture in parity.CORPUS:
            for step in fixture.steps:
                with self.subTest(fixture=fixture.name, argv=step.argv):
                    self.assertTrue(step.argv)


def _mutate_stdout(leg_b, scratch_b):
    for step in leg_b.steps:
        step.stdout += "\nINJECTED STDOUT LINE\n"
    return leg_b


def _mutate_tree(leg_b, scratch_b):
    leg_b.steps[-1].tree["injected.txt"] = (
        "file", "0o644", hashlib.sha256(b"injected").hexdigest())
    return leg_b


def _mutate_rc(leg_b, scratch_b):
    leg_b.steps[-1].rc += 1
    return leg_b


class NegativeMutationTests(unittest.TestCase):
    """RED evidence: the harness must FAIL when the legs genuinely differ."""

    def _baseline_deltas(self):
        with tempfile.TemporaryDirectory(prefix="parity-baseline-") as td:
            deltas, _mode = parity.run_comparison(
                parity.CORPUS[0], go_cli=None, workdir=Path(td))
            return deltas

    def _mutation_deltas(self, mutate):
        with tempfile.TemporaryDirectory(prefix="parity-mutate-") as td:
            deltas, _mode = parity.run_comparison(
                parity.CORPUS[0], go_cli=None, workdir=Path(td), mutate=mutate)
            return deltas

    def test_baseline_identical_by_shim_has_zero_deltas(self):
        self.assertEqual(self._baseline_deltas(), [])

    def test_mutate_stdout_is_flagged(self):
        self.assertEqual(self._baseline_deltas(), [])
        deltas = self._mutation_deltas(_mutate_stdout)
        self.assertTrue(deltas)
        self.assertTrue(any("stdout" in d for d in deltas))

    def test_mutate_tree_is_flagged(self):
        self.assertEqual(self._baseline_deltas(), [])
        deltas = self._mutation_deltas(_mutate_tree)
        self.assertTrue(deltas)
        self.assertTrue(any("injected.txt" in d for d in deltas))

    def test_mutate_rc_is_flagged(self):
        self.assertEqual(self._baseline_deltas(), [])
        deltas = self._mutation_deltas(_mutate_rc)
        self.assertTrue(deltas)
        self.assertTrue(any("exit code" in d for d in deltas))


class RunEntrypointTests(unittest.TestCase):
    """Entrypoint contract: default mode FAILS LOUDLY without a Go build;
    legacy-vs-legacy is only reachable via the EXPLICIT --self-test flag
    (reliability fix: the old silent identical-by-shim fallback defeated
    acceptance (d))."""

    def test_default_mode_fails_loudly_when_go_build_unavailable(self):
        err, out = io.StringIO(), io.StringIO()
        with mock.patch.object(parity, "build_go_binary", return_value=None), \
                contextlib.redirect_stderr(err), contextlib.redirect_stdout(out):
            rc = parity_run.main([])
        self.assertEqual(rc, 2)
        self.assertIn("Go build (cmd/ai-specs) is REQUIRED", err.getvalue())
        self.assertIn("--self-test", err.getvalue())

    def test_self_test_mode_is_explicit_legacy_vs_legacy(self):
        out = io.StringIO()
        with mock.patch.object(parity, "build_go_binary", return_value=None), \
                contextlib.redirect_stderr(io.StringIO()), \
                contextlib.redirect_stdout(out):
            rc = parity_run.main(["--self-test"])
        self.assertEqual(rc, 0)
        self.assertIn("legacy-vs-legacy", out.getvalue())


if __name__ == "__main__":
    unittest.main()
