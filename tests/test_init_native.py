"""GO-20/U4 T9 + GO-20/U2e T3e: differential black-box pins for the native `init` route.

The native ``init`` (``internal/cli/init.go``) must be observable-equal to the
legacy ``lib/init.sh`` route, byte for byte, except the divergences recorded in
``odd/tasks/go-20-u4-init.md``:

* S8  — ``--name`` substitutes the project name LITERALLY; the legacy
  ``sed "s/{{PROJECT_NAME}}/$PROJECT_NAME/g"`` mangles a name containing ``&``
  (sed replacement syntax), so the legacy output is not the reference there.
* S10 — the legacy route renders the baseline brief through Python, whose
  bridges announce ``GO_*_BRIDGE_FALLBACK`` on stderr; the native route renders
  in process and prints none of those lines.
* the ``RECIPE_MCP_TEMP:`` stdout lines name a fresh mktemp file per run, so
  they cannot be byte-compared either.

Oracle (U2e T3e, odd/tasks/go-20-u2e-flip.md L18): the legacy outputs are frozen
in ``tests/fixtures/init-native/golden.json``, captured from ``bin/ai-specs`` with
the same normalization as this module. The native route is compared to those
goldens. The two cases that prove the legacy shell is effective (the poison
seam and the ``AI_SPECS_INIT_TUI_PY`` hatch) keep their live legacy leg as RED
logic; only their outputs are compared to the golden.

Everything else — exit code, stdout, stderr and the produced relative file
tree with its bytes — must match the golden.
"""
from __future__ import annotations

import difflib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import _go_cli  # noqa: E402
from _blackbox import isolated_home, normalize_output, snapshot  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
LEGACY_CLI = _go_cli.legacy_cli()
GOLDEN_PATH = ROOT / "tests" / "fixtures" / "init-native" / "golden.json"

# Recorded non-contract lines (S10 + random per-run temps).
_RECIPE_MCP_TEMP = re.compile(r"^RECIPE_MCP_TEMP:[^\n]*\n?", re.M)
_GO_BRIDGE_FALLBACK = re.compile(r"^.*GO_[A-Z_]*_BRIDGE_FALLBACK[^\n]*\n?", re.M)
_SHA256_ROW = re.compile(r'^sha256 = "[0-9a-f]*"\n', re.M)

LOCK_REL = "ai-specs/.ai-specs.lock"
GITIGNORE_MARKER = "# --- ai-specs: agent-generated files (managed by ai-specs sync-agent) ---"


def _golden(case: str) -> dict:
    """The frozen legacy output for one case; a missing golden fails loudly."""
    return json.loads(GOLDEN_PATH.read_text(encoding="utf-8"))[case]


def _diff(label: str, golden: str, native: str) -> str:
    """Unified diff for an assert message: golden (legacy) expected vs native observed."""
    return (
        f"{label} differ from the legacy golden (golden=expected, native=observed):\n"
        + "".join(
            difflib.unified_diff(
                golden.splitlines(keepends=True),
                native.splitlines(keepends=True),
                "golden",
                "native",
            )
        )
    )


def _strip_sha256(text: str) -> str:
    """Drop the per-project sha256 rows the task exempts from lock equality."""
    return _SHA256_ROW.sub("", text)


class _Route:
    """One leg of the differential: an isolated CLI home, a scratch HOME and
    TMPDIR, and a target project sharing the basename ``proj`` — all under
    ``<base>/<name>/`` so path normalization maps every leg onto equal text."""

    def __init__(self, base: Path, name: str):
        self.root = base / name
        self.home = isolated_home(self.root)
        self.tmp = self.root / "tmp"
        (self.tmp / "home").mkdir(parents=True)
        self.target = self.root / "proj"
        self.target.mkdir()

    def env(self, extra: dict | None = None) -> dict:
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(self.tmp / "home"),
            "TMPDIR": str(self.tmp),
            "AI_SPECS_HOME": str(self.home),
            "AI_SPECS_NO_NETWORK": "1",
            "LC_ALL": "C",
            "LANG": "C",
            "PYTHONDONTWRITEBYTECODE": "1",
        }
        if extra:
            env.update(extra)
        return env

    def run(self, cli: Path, *args: str, extra_env: dict | None = None,
            cwd: Path | None = None) -> subprocess.CompletedProcess:
        """Run ``<cli> init <args...>``; stdin is an empty pipe, never a TTY.

        ``cwd`` defaults to the repository only for cases that pass an explicit
        target. A case whose argv has no positional makes the CLI read its own
        cwd as the project, so it must pass ``cwd=self.target`` — otherwise the
        run would initialize the checkout the suite runs in.
        """
        return subprocess.run(
            [str(cli), "init", *args],
            cwd=str(cwd if cwd is not None else ROOT),
            env=self.env(extra_env),
            text=True,
            capture_output=True,
            check=False,
            timeout=300,
            input="",
        )

    def stdout(self, proc: subprocess.CompletedProcess) -> str:
        """Normalized stdout minus the random-temp ``RECIPE_MCP_TEMP:`` lines."""
        return _RECIPE_MCP_TEMP.sub("", normalize_output(proc.stdout, (self.root,)))

    def stderr(self, proc: subprocess.CompletedProcess) -> str:
        """Normalized stderr minus the recorded Python-bridge announcements (S10)."""
        return _GO_BRIDGE_FALLBACK.sub("", normalize_output(proc.stderr, (self.root,)))


class _Fixture:
    """One base dir holding both legs (legacy/, native/) for the seam tests that
    still run the legacy shell live."""

    def __init__(self, test: unittest.TestCase, prefix: str):
        self.base = Path(tempfile.mkdtemp(prefix=prefix))
        test.addCleanup(shutil.rmtree, self.base, ignore_errors=True)
        self.legacy = _Route(self.base, "legacy")
        self.native = _Route(self.base, "native")


def _native_route(test: unittest.TestCase, prefix: str) -> _Route:
    """A native-only leg. Its root is ``<base>/native``, so normalization yields
    the same ``<TEMP>/proj`` text the golden was captured with."""
    base = Path(tempfile.mkdtemp(prefix=prefix))
    test.addCleanup(shutil.rmtree, base, ignore_errors=True)
    return _Route(base, "native")


def _write_stub(base: Path, rc: int, write_out: bool = False) -> Path:
    """The wizard-stub pattern from tests/test_init_tui.py (TestInitShellGating).

    A stub that exits 1 writes nothing (the cancel contract); a stub that
    exits 0 stages the manifest plus its JSON sidecar, which init.sh then
    installs with its ``(from TUI)`` marker.
    """
    stub = base / f"stub-tui-{rc}.py"
    body = f"""#!/usr/bin/env python3
import argparse, sys
from pathlib import Path
p = argparse.ArgumentParser()
p.add_argument("--target", required=True)
p.add_argument("--name", default="")
p.add_argument("--out", required=True)
args = p.parse_args()
if {write_out!r}:
    Path(args.out).write_text('[project]\\nname = "stub"\\n\\n[agents]\\nenabled = ["claude"]\\n')
    Path(args.out).with_suffix('.json').write_text('{{"name":"stub","agents":["claude"],"recipes":[]}}\\n')
sys.exit({rc})
"""
    stub.write_text(body, encoding="utf-8")
    stub.chmod(0o755)
    return stub


class InitNativeDifferentialTests(unittest.TestCase):
    def _native_cli(self) -> Path:
        go_cli = _go_cli.cli()
        if go_cli == LEGACY_CLI:
            self.skipTest("the Go build is unavailable, so the golden would test the legacy route")
        return go_cli

    def test_classic_init_is_byte_identical_to_the_legacy_golden(self):
        native_cli = self._native_cli()
        nat = _native_route(self, "ai-specs-init-native-classic-")
        golden = _golden("classic")

        nat_proc = nat.run(native_cli, str(nat.target))

        self.assertEqual(
            nat_proc.returncode, golden["returncode"],
            f"exit code differs from the golden: native={nat_proc.returncode}\n"
            f"native stderr:\n{nat_proc.stderr}",
        )
        self.assertEqual(nat_proc.returncode, 0, nat_proc.stderr)
        nat_out = nat.stdout(nat_proc)
        self.assertEqual(golden["stdout"], nat_out, _diff("stdout", golden["stdout"], nat_out))
        nat_err = nat.stderr(nat_proc)
        self.assertEqual(golden["stderr"], nat_err, _diff("stderr", golden["stderr"], nat_err))

        # Identical relative tree with identical bytes — except the lock, whose
        # sha256 rows cover per-project content and are compared stripped below.
        golden_tree = {rel: tuple(value) for rel, value in golden["tree"].items()}
        nat_tree = snapshot(nat.target)
        self.assertEqual(sorted(golden_tree), sorted(nat_tree), "produced file trees differ from the golden")
        for rel in sorted(golden_tree):
            if rel == LOCK_REL:
                continue
            self.assertEqual(golden_tree[rel], nat_tree[rel], f"{rel} differs from the legacy golden")

        self.assertIn(LOCK_REL, nat_tree, "native route produced no lock")
        nat_lock = (nat.target / LOCK_REL).read_text(encoding="utf-8")
        self.assertIn('[managed."AGENTS.md"]', nat_lock, "lock lost its AGENTS.md key")
        self.assertIn('[managed."AGENTS.md"]', golden["lock"], "golden lock lost its AGENTS.md key")
        self.assertEqual(
            golden["lock"], _strip_sha256(nat_lock),
            _diff("lock (sha256 rows stripped)", golden["lock"], _strip_sha256(nat_lock)),
        )

    def test_help_is_the_same_stdout_and_exit_0_as_the_legacy_golden(self):
        native_cli = self._native_cli()
        nat = _native_route(self, "ai-specs-init-native-help-")
        golden = _golden("help")

        nat_proc = nat.run(native_cli, "--help")

        self.assertEqual(golden["returncode"], 0)
        self.assertEqual(nat_proc.returncode, 0, nat_proc.stderr)
        self.assertIn("Usage: ai-specs init", golden["stdout"])
        self.assertEqual(golden["stdout"], nat_proc.stdout,
                         _diff("help stdout", golden["stdout"], nat_proc.stdout))
        self.assertEqual(golden["stderr"], nat_proc.stderr,
                         _diff("help stderr", golden["stderr"], nat_proc.stderr))

    def test_unknown_flag_and_extra_positional_exit_2_with_the_golden_stderr(self):
        native_cli = self._native_cli()
        nat = _native_route(self, "ai-specs-init-native-usage-")

        flag_golden = _golden("unknown_flag")
        nat_flag = nat.run(native_cli, "--bogus-flag")
        self.assertEqual(flag_golden["returncode"], 2)
        self.assertEqual(nat_flag.returncode, 2, nat_flag.stderr)
        nat_err = nat.stderr(nat_flag)
        self.assertIn("ERROR: unknown flag: --bogus-flag", flag_golden["stderr"])
        self.assertEqual(flag_golden["stderr"], nat_err,
                         _diff("unknown-flag stderr", flag_golden["stderr"], nat_err))
        self.assertEqual(flag_golden["stdout"], "")
        self.assertEqual(nat_flag.stdout, "")

        pos_golden = _golden("extra_positional")
        nat_pos = nat.run(native_cli, str(nat.target), "second-arg")
        self.assertEqual(pos_golden["returncode"], 2)
        self.assertEqual(nat_pos.returncode, 2, nat_pos.stderr)
        nat_err = nat.stderr(nat_pos)
        self.assertIn("ERROR: unexpected positional argument: second-arg", pos_golden["stderr"])
        self.assertEqual(pos_golden["stderr"], nat_err,
                         _diff("extra-positional stderr", pos_golden["stderr"], nat_err))
        self.assertEqual(pos_golden["stdout"], "")
        self.assertEqual(nat_pos.stdout, "")

    def test_missing_target_exits_1_with_the_golden_stderr(self):
        native_cli = self._native_cli()
        nat = _native_route(self, "ai-specs-init-native-missing-")
        golden = _golden("missing_target")

        nat_proc = nat.run(native_cli, str(nat.root / "does-not-exist"))

        self.assertEqual(golden["returncode"], 1)
        self.assertEqual(nat_proc.returncode, 1, nat_proc.stdout)
        nat_err = nat.stderr(nat_proc)
        self.assertIn("ERROR: target path does not exist or is not a directory", golden["stderr"])
        self.assertIn("does-not-exist", golden["stderr"])
        self.assertIn("does-not-exist", nat_err)
        self.assertEqual(golden["stderr"], nat_err,
                         _diff("missing-target stderr", golden["stderr"], nat_err))

    def test_second_run_keeps_stdout_and_user_owned_bytes(self):
        native_cli = self._native_cli()
        nat = _native_route(self, "ai-specs-init-native-second-")
        golden = _golden("second_run")

        first_nat = nat.run(native_cli, str(nat.target))
        self.assertEqual(first_nat.returncode, 0, first_nat.stderr)

        manifest = nat.target / "ai-specs" / "ai-specs.toml"
        root_gitignore = nat.target / ".gitignore"
        manifest_before = manifest.read_bytes()
        gitignore_before = root_gitignore.read_bytes()

        second_nat = nat.run(native_cli, str(nat.target))
        self.assertEqual(second_nat.returncode, golden["returncode"], second_nat.stderr)
        self.assertEqual(second_nat.returncode, 0, second_nat.stderr)

        # The native rerun matches the legacy idempotent rerun line for line...
        nat_out = nat.stdout(second_nat)
        self.assertEqual(golden["stdout"], nat_out, _diff("second-run stdout", golden["stdout"], nat_out))
        # ...and kept the user-owned bytes untouched (S4).
        self.assertIn("keep   ai-specs/ai-specs.toml", second_nat.stdout)
        self.assertEqual(manifest.read_bytes(), manifest_before,
                         "the second native run rewrote ai-specs/ai-specs.toml")
        self.assertEqual(root_gitignore.read_bytes(), gitignore_before,
                         "the second native run rewrote the root .gitignore")

    def test_force_refreshes_the_root_gitignore_block_to_the_golden(self):
        native_cli = self._native_cli()
        nat = _native_route(self, "ai-specs-init-native-force-")
        golden = _golden("force")

        first_nat = nat.run(native_cli, str(nat.target))
        self.assertEqual(first_nat.returncode, 0, first_nat.stderr)

        nat_gitignore = nat.target / ".gitignore"
        fresh_bytes = golden["gitignore"].encode("utf-8")
        self.assertEqual(nat_gitignore.read_bytes(), fresh_bytes,
                         "first-run root .gitignore bytes differ from the golden")

        # Corrupt a line INSIDE the managed block: --force must strip the marked
        # region and re-append the template over it.
        lines = nat_gitignore.read_text(encoding="utf-8").splitlines(keepends=True)
        marker_at = next(i for i, ln in enumerate(lines) if ln.startswith(GITIGNORE_MARKER))
        lines.insert(marker_at + 1, "# injected junk — --force must remove this\n")
        nat_gitignore.write_text("".join(lines), encoding="utf-8")

        force_nat = nat.run(native_cli, "--force", str(nat.target))
        self.assertEqual(force_nat.returncode, golden["returncode"], force_nat.stderr)
        self.assertEqual(force_nat.returncode, 0, force_nat.stderr)

        self.assertIn("✓ refresh .gitignore (agent block)", golden["stdout"])
        self.assertIn("✓ refresh .gitignore (agent block)", force_nat.stdout)
        nat_out = nat.stdout(force_nat)
        self.assertEqual(golden["stdout"], nat_out, _diff("--force stdout", golden["stdout"], nat_out))
        self.assertEqual(nat_gitignore.read_bytes(), fresh_bytes,
                         "native --force did not restore the golden agent block")

    def test_name_with_ampersand_lands_literally_on_the_native_route(self):
        # S8 (recorded divergence): the legacy render is
        # `sed "s/{{PROJECT_NAME}}/$PROJECT_NAME/g"`, and sed treats `&` in the
        # replacement as the whole match — so `R&D` would come out as
        # `R{{PROJECT_NAME}}D`. The native route substitutes literally. The
        # legacy output for this case is deliberately NOT asserted: the port is
        # the documented correct behaviour here, so it has no golden.
        native_cli = self._native_cli()
        nat = _native_route(self, "ai-specs-init-native-ampname-")

        proc = nat.run(native_cli, "--name", "R&D", str(nat.target))
        self.assertEqual(proc.returncode, 0, proc.stderr)

        manifest = (nat.target / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn('name = "R&D"', manifest)
        self.assertNotIn("R{{PROJECT_NAME}}D", manifest,
                         "the native route applied the legacy sed mangling")

    def test_manifest_directory_is_never_destroyed_and_exits_1(self):
        # S4 + S3: when ai-specs/ai-specs.toml is not a regular file, the native
        # route must refuse, exit 1, and leave the path exactly as it found it.
        # stderr text is exempt (bash's own redirection error, S12).
        native_cli = self._native_cli()
        nat = _native_route(self, "ai-specs-init-native-tomldir-")
        golden = _golden("tomldir")
        (nat.target / "ai-specs" / "ai-specs.toml").mkdir(parents=True)

        nat_proc = nat.run(native_cli, str(nat.target))

        self.assertEqual(golden["returncode"], 1)
        self.assertEqual(nat_proc.returncode, 1, nat_proc.stderr)
        toml_path = nat.target / "ai-specs" / "ai-specs.toml"
        self.assertTrue(toml_path.is_dir(), "native: the directory was replaced by a file")
        self.assertEqual([], sorted(p.name for p in toml_path.iterdir()),
                         "native: something was written inside the directory")
        nat_out = nat.stdout(nat_proc)
        self.assertEqual(golden["stdout"], nat_out, _diff("stdout", golden["stdout"], nat_out))

    def test_missing_name_value_refuses_without_writing(self):
        # JD-B-001 (HIGH, converged with JD-A-001): a trailing `--name` aborts the
        # shell verb at `shift 2` under `set -euo pipefail` — exit 1, nothing
        # written. The port keeps the exit code; only the message text differs
        # (bash's own "shift count out of range" is not a designed message, S12).
        native_cli = self._native_cli()
        nat = _native_route(self, "ai-specs-init-native-noname-")
        golden = _golden("noname")

        # No positional: the CLI reads its own cwd as the project, so the cwd
        # is the throwaway project and never the checkout.
        nat_proc = nat.run(native_cli, "--name", cwd=nat.target)

        self.assertEqual(golden["returncode"], 1)
        self.assertEqual(nat_proc.returncode, 1, nat_proc.stderr)
        written = sorted(p.relative_to(nat.target) for p in nat.target.rglob("*"))
        self.assertEqual([], written, "native: the aborted run wrote files")
        self.assertIn("--name requires a value", nat_proc.stderr)

    def test_init_tui_py_env_seam_is_retired_for_the_native_verb(self):
        # U2b (odd/tasks/go-20-u2b-init.md S5): the native verb no longer hands
        # the whole wizard to lib/init.sh when AI_SPECS_INIT_TUI_PY is set. The
        # variable is ignored, so the run with it must match the run without it.
        # The Go test TestInitTUIEnvSeamNoLongerDelegates pins the marker side.
        native_cli = self._native_cli()
        stub_dir = _Fixture(self, "ai-specs-init-native-hatch-stub-")
        stub = _write_stub(stub_dir.base, rc=1, write_out=False)
        extra = {"AI_SPECS_INIT_TUI_PY": str(stub)}
        with_env = _native_route(self, "ai-specs-init-native-hatch-env-")
        without_env = _native_route(self, "ai-specs-init-native-hatch-bare-")
        env_proc = with_env.run(native_cli, "--tui", str(with_env.target), extra_env=extra)
        bare_proc = without_env.run(native_cli, "--tui", str(without_env.target))
        self.assertEqual(env_proc.returncode, bare_proc.returncode, env_proc.stderr)
        env_out, bare_out = with_env.stdout(env_proc), without_env.stdout(bare_proc)
        self.assertEqual(env_out, bare_out, _diff("hatch-retired stdout", bare_out, env_out))
        env_err, bare_err = with_env.stderr(env_proc), without_env.stderr(bare_proc)
        self.assertEqual(env_err, bare_err, _diff("hatch-retired stderr", bare_err, env_err))

    # GO-20 U2e T2 finding (owner: T5, expires when lib/_internal is deleted): after the
    # flip the legacy leg is the native binary, so the poison control below cannot fire.
    # After T5 this test must rebuild the poison control against a Go surface before its
    # native assertions can claim anything. Assertions untouched; expires with T5.
    @unittest.expectedFailure
    def test_native_wizard_never_executes_init_tui_py(self):
        # T4b-1, the nativity pin: the isolated home gets a REAL lib copy whose
        # init_tui.py is a poison that records its own execution and exits 3.
        # The legacy leg stays LIVE here as RED logic: it proves the poison fires
        # (the shell runs the Python wizard). The native leg must leave its marker
        # ABSENT — if `init --tui` still delegated the whole verb to lib/init.sh,
        # the poison would run there too and this assertion fails. Only the
        # outputs are compared to the golden.
        native_cli = self._native_cli()
        fixture = _Fixture(self, "ai-specs-init-native-poison-")
        golden = _golden("tui_poison")

        def poison_home(home: Path, marker: Path) -> None:
            lib = home / "lib"
            lib.unlink()
            shutil.copytree(ROOT / "lib", lib, symlinks=True,
                            ignore=shutil.ignore_patterns("_vendor"))
            (lib / "_vendor").symlink_to(ROOT / "lib" / "_vendor")
            (lib / "_internal" / "init_tui.py").write_text(
                "import sys\n"
                f"open({str(marker)!r}, 'w').write('ran')\n"
                "sys.exit(3)\n",
                encoding="utf-8",
            )

        leg_marker = fixture.base / "poison-ran-legacy"
        nat_marker = fixture.base / "poison-ran-native"
        poison_home(fixture.legacy.home, leg_marker)
        poison_home(fixture.native.home, nat_marker)

        leg_proc = fixture.legacy.run(LEGACY_CLI, "--tui", str(fixture.legacy.target))
        nat_proc = fixture.native.run(native_cli, "--tui", str(fixture.native.target))

        # The poison is effective on the legacy leg: the shell ran init_tui.py,
        # got rc 3 and fell back to the classic path.
        self.assertTrue(leg_marker.exists(),
                        "the poison never ran: the legacy leg did not execute its wizard")
        self.assertEqual(leg_proc.returncode, 0, leg_proc.stderr)
        self.assertIn("falling back to classic init", leg_proc.stderr)

        # The native leg did the same work WITHOUT the Python wizard: no
        # marker, the same fallback line, the same classic result.
        self.assertFalse(nat_marker.exists(),
                         "the native route executed lib/_internal/init_tui.py — the wizard is not native")
        self.assertEqual(nat_proc.returncode, golden["returncode"], nat_proc.stderr)
        self.assertEqual(nat_proc.returncode, 0, nat_proc.stderr)
        self.assertIn("falling back to classic init", nat_proc.stderr)
        self.assertTrue((fixture.native.target / "ai-specs" / "ai-specs.toml").is_file())
        nat_out = fixture.native.stdout(nat_proc)
        self.assertEqual(golden["stdout"], nat_out, _diff("poison stdout", golden["stdout"], nat_out))
        nat_err = fixture.native.stderr(nat_proc)
        self.assertEqual(golden["stderr"], nat_err, _diff("poison stderr", golden["stderr"], nat_err))

    def test_forced_tui_without_a_tty_falls_back_to_classic_as_the_golden(self):
        # S15/S6: --tui with pipes must refuse before any prompt (exit 3 from
        # the wizard) and hand the run to the classic path with the legacy
        # fallback line — identically to the golden.
        native_cli = self._native_cli()
        nat = _native_route(self, "ai-specs-init-native-tui-nontty-")
        golden = _golden("tui_nontty")

        nat_proc = nat.run(native_cli, "--tui", str(nat.target))

        self.assertEqual(nat_proc.returncode, golden["returncode"], nat_proc.stderr)
        self.assertEqual(nat_proc.returncode, 0, nat_proc.stderr)
        self.assertIn("falling back to classic init", golden["stderr"])
        self.assertIn("falling back to classic init", nat_proc.stderr)
        self.assertNotIn("Project name", nat_proc.stdout + nat_proc.stderr)
        nat_out = nat.stdout(nat_proc)
        self.assertEqual(golden["stdout"], nat_out, _diff("tui-nontty stdout", golden["stdout"], nat_out))
        nat_err = nat.stderr(nat_proc)
        self.assertEqual(golden["stderr"], nat_err, _diff("tui-nontty stderr", golden["stderr"], nat_err))
        self.assertTrue((nat.target / "ai-specs" / "ai-specs.toml").is_file())

    def test_tui_over_an_existing_manifest_skips_as_the_golden(self):
        # init.sh's skip branch, at the native verb: --tui over a user-owned
        # manifest prints the skip line before the banner, runs classic with
        # tui: no and keeps the bytes (S4) — identically to the golden.
        native_cli = self._native_cli()
        nat = _native_route(self, "ai-specs-init-native-tui-skip-")
        golden = _golden("tui_skip")
        user = b'# hand-edited\nname = "keepme"\n'
        path = nat.target / "ai-specs" / "ai-specs.toml"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(user)

        nat_proc = nat.run(native_cli, "--tui", str(nat.target))

        self.assertEqual(nat_proc.returncode, golden["returncode"], nat_proc.stderr)
        self.assertEqual(nat_proc.returncode, 0, nat_proc.stderr)
        skip = "  · skip TUI — ai-specs.toml already exists (never overwritten by init)"
        self.assertIn(skip, golden["stdout"])
        self.assertIn(skip, nat_proc.stdout)
        self.assertLess(nat_proc.stdout.index(skip), nat_proc.stdout.index("ai-specs init\n"))
        self.assertIn("tui:     no", nat_proc.stdout)
        self.assertEqual(path.read_bytes(), user, "the user's manifest changed")
        nat_out = nat.stdout(nat_proc)
        self.assertEqual(golden["stdout"], nat_out, _diff("tui-skip stdout", golden["stdout"], nat_out))
        nat_err = nat.stderr(nat_proc)
        self.assertEqual(golden["stderr"], nat_err, _diff("tui-skip stderr", golden["stderr"], nat_err))


if __name__ == "__main__":
    unittest.main()
