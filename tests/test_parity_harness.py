"""Self-tests for the differential parity harness (card [Go 03]).

Includes negative mutation tests: the RED evidence that the harness FAILS
when the two legs genuinely differ. Stdlib only.
"""
from __future__ import annotations

import contextlib
import hashlib
import io
import json
import shutil
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent / "parity"))
sys.path.insert(0, str(Path(__file__).resolve().parent))
import parity  # noqa: E402
import parity.parity as parity_impl  # noqa: E402  (implementation module: the
# package __init__ re-exports from it, so patch internal calls here)
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


class GateModeTests(unittest.TestCase):
    """The gate mode is a first-class parameter pinned for BOTH legs."""

    def test_run_leg_pins_worktree_gate_bin_when_provided(self):
        captured = {}

        def fake_run(argv, **kwargs):
            captured["env"] = kwargs.get("env", {})
            return mock.Mock(returncode=0, stdout="", stderr="")

        with tempfile.TemporaryDirectory(prefix="parity-gate-") as td, \
                mock.patch.object(parity.subprocess, "run", side_effect=fake_run):
            parity.run_leg("legacy", Path("/bin/true"), parity.CORPUS[0],
                           Path(td), gate_bin=Path("/gate/worktree-gate"))
        self.assertEqual(captured["env"]["WORKTREE_GATE_BIN"],
                         "/gate/worktree-gate")

    def test_run_leg_omits_worktree_gate_bin_when_absent(self):
        captured = {}

        def fake_run(argv, **kwargs):
            captured["env"] = kwargs.get("env", {})
            return mock.Mock(returncode=0, stdout="", stderr="")

        with tempfile.TemporaryDirectory(prefix="parity-gate-") as td, \
                mock.patch.object(parity.subprocess, "run", side_effect=fake_run):
            parity.run_leg("legacy", Path("/bin/true"), parity.CORPUS[0],
                           Path(td))
        self.assertNotIn("WORKTREE_GATE_BIN", captured["env"])

    def test_run_comparison_forwards_gate_bin_to_both_legs_and_labels_mode(self):
        seen = []

        def fake_run_leg(label, cli_path, fixture, scratch, gate_bin=None):
            seen.append(gate_bin)
            return parity.LegResult(label=label, cli_path=str(cli_path))

        gate = Path("/gate/worktree-gate")
        with tempfile.TemporaryDirectory(prefix="parity-gate-") as td, \
                mock.patch.object(parity_impl, "run_leg", side_effect=fake_run_leg):
            deltas, mode = parity.run_comparison(
                parity.CORPUS[0], go_cli=None, workdir=Path(td),
                gate_mode=parity.GATE_PRESENT, gate_bin=gate)
        self.assertEqual(seen, [gate, gate])
        self.assertIn(parity.GATE_PRESENT, mode)
        self.assertEqual(deltas, [])

    def test_run_corpus_reports_and_forwards_the_gate_mode(self):
        seen = []

        def fake_run_comparison(fixture, go_cli, workdir, mutate=None,
                                gate_mode=parity.GATE_ABSENT, gate_bin=None,
                                live_legacy=None):
            seen.append((gate_mode, gate_bin))
            return [], f"legacy-vs-go | {gate_mode}"

        gate = Path("/gate/worktree-gate")
        with tempfile.TemporaryDirectory(prefix="parity-gate-") as td, \
                mock.patch.object(parity_impl, "run_comparison",
                                  side_effect=fake_run_comparison):
            code, report = parity.run_corpus(
                go_cli=Path("/go"), workdir=Path(td),
                fixtures=(parity.CORPUS[0],), gate_mode=parity.GATE_PRESENT,
                gate_bin=gate)
        self.assertEqual(code, 0)
        self.assertIn("gate mode: gate-present", report)
        self.assertIn("[legacy-vs-go | gate-present]", report)
        self.assertEqual(seen, [(parity.GATE_PRESENT, gate)])


class GateBuildTests(unittest.TestCase):
    """The gate-present builder degrades to None; the caller decides policy."""

    def test_none_when_go_is_unavailable(self):
        with tempfile.TemporaryDirectory(prefix="parity-gate-") as td, \
                mock.patch.object(parity.shutil, "which", return_value=None):
            self.assertIsNone(parity.build_gate_binary(Path(td)))

    def test_returns_dest_path_on_success(self):
        ok = mock.Mock(returncode=0, stdout="", stderr="")
        with tempfile.TemporaryDirectory(prefix="parity-gate-") as td, \
                mock.patch.object(parity.shutil, "which", return_value="/go"), \
                mock.patch.object(parity.subprocess, "run", return_value=ok):
            dest = parity.build_gate_binary(Path(td))
            self.assertEqual(dest, Path(td) / "worktree-gate")

    def test_none_and_loud_stderr_on_build_failure(self):
        bad = mock.Mock(returncode=1, stdout="", stderr="boom")
        err = io.StringIO()
        with tempfile.TemporaryDirectory(prefix="parity-gate-") as td, \
                mock.patch.object(parity.shutil, "which", return_value="/go"), \
                mock.patch.object(parity.subprocess, "run", return_value=bad), \
                contextlib.redirect_stderr(err):
            self.assertIsNone(parity.build_gate_binary(Path(td)))
        self.assertIn("worktree-gate build failed", err.getvalue())


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


class GoldenCorpusTests(unittest.TestCase):
    """GO-20 U2e T2a: the legacy leg is frozen in versioned goldens."""

    def _captured_fixtures(self):
        return [f for f in parity.CORPUS
                if f.name not in parity.GOLDEN_EXCLUDED]

    def test_every_fixture_has_a_gate_absent_golden(self):
        for fixture in self._captured_fixtures():
            with self.subTest(fixture=fixture.name):
                path = parity.golden_path(fixture.name, parity.GATE_ABSENT)
                self.assertTrue(path.is_file(), f"missing golden {path}")

    def test_gate_present_goldens_exist_when_captured(self):
        gdir = parity.GOLDENS_DIR / parity.GATE_PRESENT
        self.assertTrue(gdir.is_dir(), f"missing goldens dir {gdir}")
        unavailable = gdir / "NOT_CAPTURED.txt"
        if unavailable.is_file():
            self.assertTrue(unavailable.read_text().strip())
            return
        for fixture in self._captured_fixtures():
            with self.subTest(fixture=fixture.name):
                path = parity.golden_path(fixture.name, parity.GATE_PRESENT)
                self.assertTrue(path.is_file(), f"missing golden {path}")

    def test_goldens_round_trip_to_step_results(self):
        fixture = self._captured_fixtures()[0]
        path = parity.golden_path(fixture.name, parity.GATE_ABSENT)
        raw = json.loads(path.read_text(encoding="utf-8"))
        leg = parity.load_golden_leg(fixture, parity.GATE_ABSENT)
        self.assertEqual(raw["fixture"], fixture.name)
        self.assertEqual(raw["gate_mode"], parity.GATE_ABSENT)
        self.assertTrue(raw["legacy_launcher"])
        self.assertEqual(len(leg.steps), len(raw["steps"]))
        for step, item in zip(leg.steps, raw["steps"]):
            self.assertEqual(list(step.argv), item["argv"])
            self.assertEqual(step.rc, item["rc"])
            self.assertEqual(step.stdout, item["stdout"])
            self.assertEqual(step.stderr, item["stderr"])
            self.assertEqual(parity.tree_to_json(step.tree), item["tree"])
            self.assertEqual(step.tree, parity.tree_from_json(item["tree"]))

    def test_missing_golden_fails_loudly_naming_the_fixture(self):
        fixture = self._captured_fixtures()[0]
        with tempfile.TemporaryDirectory(prefix="parity-golden-") as td, \
                mock.patch.object(parity_impl, "GOLDENS_DIR", Path(td)):
            with self.assertRaises(RuntimeError) as raised:
                parity.load_golden_leg(fixture, parity.GATE_ABSENT)
        self.assertIn(fixture.name, str(raised.exception))

    def test_mutated_golden_stdout_reports_a_delta(self):
        fixture = self._captured_fixtures()[0]
        real = parity.load_golden_leg(fixture, parity.GATE_ABSENT)
        source = parity.golden_path(fixture.name, parity.GATE_ABSENT)
        raw = json.loads(source.read_text(encoding="utf-8"))
        # Flip one byte of the first non-empty stdout (the negative control).
        index = next(i for i, item in enumerate(raw["steps"])
                     if item["stdout"])
        stdout = raw["steps"][index]["stdout"]
        flipped = ("X" if stdout[-1] != "X" else "Y")
        raw["steps"][index]["stdout"] = stdout[:-1] + flipped
        with tempfile.TemporaryDirectory(prefix="parity-golden-") as td:
            target = Path(td) / parity.GATE_ABSENT / f"{fixture.name}.json"
            target.parent.mkdir(parents=True)
            target.write_text(json.dumps(raw, sort_keys=True, indent=1) + "\n",
                              encoding="utf-8")
            with mock.patch.object(parity_impl, "GOLDENS_DIR", Path(td)):
                mutated = parity.load_golden_leg(fixture, parity.GATE_ABSENT)
        deltas = parity.diff_legs(real, mutated)
        self.assertTrue(deltas)
        self.assertTrue(any("stdout" in delta for delta in deltas))

    def test_mutated_live_go_stdout_reports_a_delta(self):
        fixture = parity.CORPUS[0]
        with tempfile.TemporaryDirectory(prefix="parity-golden-") as td:
            deltas, _mode = parity.run_comparison(
                fixture, go_cli=None, workdir=Path(td),
                mutate=_mutate_stdout, live_legacy=True)
        self.assertTrue(deltas)
        self.assertTrue(any("stdout" in delta for delta in deltas))

    def test_default_mode_runs_only_the_go_leg_live(self):
        seen = []

        def fake_run_leg(label, cli_path, fixture, scratch, gate_bin=None):
            seen.append(label)
            return parity.LegResult(label=label, cli_path=str(cli_path))

        fixture = self._captured_fixtures()[0]
        with tempfile.TemporaryDirectory(prefix="parity-golden-") as td, \
                mock.patch.object(parity_impl, "run_leg", side_effect=fake_run_leg):
            parity.run_comparison(fixture, go_cli=Path("/go/ai-specs-go"),
                                  workdir=Path(td))
        self.assertEqual(seen, ["other"])

    def test_self_test_mode_still_runs_the_legacy_launcher_live(self):
        seen = []

        def fake_run_leg(label, cli_path, fixture, scratch, gate_bin=None):
            seen.append((label, cli_path))
            return parity.LegResult(label=label, cli_path=str(cli_path))

        with tempfile.TemporaryDirectory(prefix="parity-golden-") as td, \
                mock.patch.object(parity_impl, "run_leg", side_effect=fake_run_leg):
            parity.run_comparison(parity.CORPUS[0], go_cli=None,
                                  workdir=Path(td))
        self.assertEqual([label for label, _ in seen], ["legacy", "other"])
        self.assertTrue(all(path == parity.ROOT / "bin" / "ai-specs"
                            for _label, path in seen))

    @unittest.skipUnless(shutil.which("go"),
                         "go toolchain unavailable: structured golden-vs-Go "
                         "comparison skipped (mock-based controls above still "
                         "run)")
    def test_golden_vs_live_go_reports_zero_deltas(self):
        fixture = parity.CORPUS[0]
        with tempfile.TemporaryDirectory(prefix="parity-golden-") as td:
            go_cli = parity.build_go_binary(Path(td))
            if go_cli is None:
                self.skipTest("Go build failed in this environment")
            deltas, mode = parity.run_comparison(
                fixture, go_cli=go_cli, workdir=Path(td))
        self.assertEqual(deltas, [], f"{fixture.name}: {mode}")


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

    def test_default_mode_fails_loudly_when_gate_build_unavailable(self):
        err, out = io.StringIO(), io.StringIO()
        with mock.patch.object(parity, "build_go_binary",
                               return_value=Path("/go/ai-specs-go")), \
                mock.patch.object(parity, "build_gate_binary", return_value=None), \
                contextlib.redirect_stderr(err), contextlib.redirect_stdout(out):
            rc = parity_run.main([])
        self.assertEqual(rc, 2)
        self.assertIn("worktree-gate build", err.getvalue())
        self.assertIn("gate-present", err.getvalue())

    def test_self_test_mode_is_explicit_legacy_vs_legacy(self):
        out, err = io.StringIO(), io.StringIO()
        with mock.patch.object(parity, "build_go_binary", return_value=None), \
                mock.patch.object(parity, "build_gate_binary", return_value=None), \
                contextlib.redirect_stderr(err), \
                contextlib.redirect_stdout(out):
            rc = parity_run.main(["--self-test"])
        self.assertEqual(rc, 0)
        self.assertIn("legacy-vs-legacy", out.getvalue())
        self.assertIn("gate-present: UNAVAILABLE", err.getvalue())
        self.assertIn("gate-present NOT MEASURED", out.getvalue())

    def test_self_test_runs_both_gate_modes_when_gate_buildable(self):
        calls = []

        def fake_run_corpus(go_cli, workdir, fixtures=parity.CORPUS,
                            gate_mode=parity.GATE_ABSENT, gate_bin=None):
            calls.append(gate_mode)
            return 0, f"report {gate_mode}\nfixtures: 1, failing: 0"

        out = io.StringIO()
        with mock.patch.object(parity, "build_go_binary",
                               return_value=Path("/go/ai-specs-go")), \
                mock.patch.object(parity, "build_gate_binary",
                                  return_value=Path("/gate/worktree-gate")), \
                mock.patch.object(parity, "run_corpus", side_effect=fake_run_corpus), \
                contextlib.redirect_stdout(out), \
                contextlib.redirect_stderr(io.StringIO()):
            rc = parity_run.main(["--self-test"])
        self.assertEqual(rc, 0)
        self.assertEqual(calls, [parity.GATE_ABSENT, parity.GATE_PRESENT])
        self.assertIn("parity summary", out.getvalue())
        self.assertIn("gate-present failing=0", out.getvalue())


class VendorAddCloneNormalizationTests(unittest.TestCase):
    """GO-20 U2e T4: N11 removes the vendor-skills.py clone-failure wrapper
    for `add-dep` and `skills add` only, and still lets a real divergence fail."""

    LEGACY = (
        "fatal: repository 'A' does not exist\n"
        "Traceback (most recent call last):\n"
        '  File "vendor-skills.py", line 114, in clone\n'
        "    subprocess.run(\n"
        "subprocess.CalledProcessError: Command '['git', 'clone', '--depth', "
        "'1', 'A']' returned non-zero exit status 128.\n"
    )
    GO = "fatal: repository 'A' does not exist\n  ✗ exit status 128\n"
    EXPECTED = "fatal: repository 'A' does not exist\n"

    def test_add_dep_traceback_and_status_normalize_to_fatal_line(self):
        ctx = {"argv": ("add-dep", "/x")}
        self.assertEqual(
            parity.apply_normalizations("stderr", self.LEGACY, ctx),
            self.EXPECTED)
        self.assertEqual(
            parity.apply_normalizations("stderr", self.GO, ctx),
            self.EXPECTED)

    def test_skills_add_is_normalized_too(self):
        ctx = {"argv": ("skills", "add", "/x")}
        self.assertEqual(
            parity.apply_normalizations("stderr", self.LEGACY, ctx),
            self.EXPECTED)
        self.assertEqual(
            parity.apply_normalizations("stderr", self.GO, ctx),
            self.EXPECTED)

    def test_other_verbs_are_not_touched(self):
        ctx = {"argv": ("doctor",)}
        self.assertEqual(
            parity.apply_normalizations("stderr", self.LEGACY, ctx),
            self.LEGACY)
        self.assertEqual(
            parity.apply_normalizations("stderr", self.GO, ctx), self.GO)

    def test_different_fatal_lines_still_diverge(self):
        ctx = {"argv": ("add-dep", "/x")}
        legacy = self.LEGACY
        go = self.GO.replace("'A'", "'B'")
        self.assertNotEqual(
            parity.apply_normalizations("stderr", legacy, ctx),
            parity.apply_normalizations("stderr", go, ctx))

    def test_n11_records_its_expiry(self):
        entry = {n.rule_id: n for n in parity.NORMALIZATIONS}[
            "N11-vendor-add-clone-failure"]
        self.assertIn("T5", entry.description)


class S3GapCoverageTests(unittest.TestCase):
    """U2 S7 gap rule (GO-20 U2e T4): every named verb is covered by a corpus
    step or recorded with an explicit reason. One subtest per gap."""

    GAPS = ("hub", "upgrade", "skills", "add-dep", "configure-recipes",
            "refresh-bundled", "recipe remove", "recipe configure")

    def test_every_s3_gap_is_registered(self):
        self.assertEqual(sorted(parity.S3_GAP_COVERAGE), sorted(self.GAPS))

    def test_each_gap_is_covered_or_recorded(self):
        names = {f.name: f for f in parity.CORPUS}
        for gap in self.GAPS:
            with self.subTest(gap=gap):
                entry = parity.S3_GAP_COVERAGE[gap]
                covers = entry.get("covers", ())
                recorded = entry.get("recorded", ())
                equivalent = entry.get("equivalent", ())
                self.assertTrue(covers or recorded or equivalent,
                                f"{gap}: neither covered, equivalent nor recorded")
                for fixture_name, prefix in covers:
                    if fixture_name not in names:
                        self.fail(f"{gap}: fixture {fixture_name} missing")
                    argvs = [s.argv for s in names[fixture_name].steps]
                    self.assertTrue(
                        any(a[:len(prefix)] == prefix for a in argvs),
                        f"{gap}: {fixture_name} has no step {prefix}")
                for reason in recorded:
                    self.assertTrue(reason.strip())
                for note in entry.get("equivalent", ()):
                    self.assertTrue(note.strip())


if __name__ == "__main__":
    unittest.main()
