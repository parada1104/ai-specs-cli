"""GO-20/U4 T9: differential black-box pins for the native `init` route.

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

Everything else — exit code, stdout, stderr and the produced relative file
tree with its bytes — must match between the two routes. Both legs run in
throwaway projects that share the basename ``proj`` under one base dir, each
with its own isolated CLI home, HOME and TMPDIR, so normalizing each leg's
root maps both onto the same ``<TEMP>/...`` text.
"""
from __future__ import annotations

import difflib
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

# Recorded non-contract lines (S10 + random per-run temps).
_RECIPE_MCP_TEMP = re.compile(r"^RECIPE_MCP_TEMP:[^\n]*\n?", re.M)
_GO_BRIDGE_FALLBACK = re.compile(r"^.*GO_[A-Z_]*_BRIDGE_FALLBACK[^\n]*\n?", re.M)
_SHA256_ROW = re.compile(r'^sha256 = "[0-9a-f]*"\n', re.M)

LOCK_REL = "ai-specs/.ai-specs.lock"
GITIGNORE_MARKER = "# --- ai-specs: agent-generated files (managed by ai-specs sync-agent) ---"


def _diff(label: str, legacy: str, native: str) -> str:
    """Unified diff for an assert message: legacy reference vs native observed."""
    return (
        f"{label} differ between routes (legacy=expected, native=observed):\n"
        + "".join(
            difflib.unified_diff(
                legacy.splitlines(keepends=True),
                native.splitlines(keepends=True),
                "legacy",
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
    ``<base>/<name>/`` so path normalization maps both legs onto equal text."""

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
        legacy route would initialize the checkout the suite runs in.
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
    """One base dir holding both legs (legacy/, native/) — the shape of
    tests/test_init_brief_route.py, doubled so one test can diff the routes."""

    def __init__(self, test: unittest.TestCase, prefix: str):
        self.base = Path(tempfile.mkdtemp(prefix=prefix))
        test.addCleanup(shutil.rmtree, self.base, ignore_errors=True)
        self.legacy = _Route(self.base, "legacy")
        self.native = _Route(self.base, "native")


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
            self.skipTest("the Go build is unavailable, so the two routes would be one program")
        return go_cli

    def test_classic_init_is_byte_identical_to_the_legacy_route(self):
        native_cli = self._native_cli()
        fixture = _Fixture(self, "ai-specs-init-native-classic-")
        leg, nat = fixture.legacy, fixture.native

        leg_proc = leg.run(LEGACY_CLI, str(leg.target))
        nat_proc = nat.run(native_cli, str(nat.target))

        self.assertEqual(
            leg_proc.returncode, nat_proc.returncode,
            f"exit codes differ: legacy={leg_proc.returncode} native={nat_proc.returncode}\n"
            f"legacy stderr:\n{leg_proc.stderr}\nnative stderr:\n{nat_proc.stderr}",
        )
        self.assertEqual(leg_proc.returncode, 0, leg_proc.stderr)
        leg_out, nat_out = leg.stdout(leg_proc), nat.stdout(nat_proc)
        self.assertEqual(leg_out, nat_out, _diff("stdout", leg_out, nat_out))
        leg_err, nat_err = leg.stderr(leg_proc), nat.stderr(nat_proc)
        self.assertEqual(leg_err, nat_err, _diff("stderr", leg_err, nat_err))

        # Identical relative tree with identical bytes — except the lock, whose
        # sha256 rows cover per-project content and may differ by project name.
        leg_tree, nat_tree = snapshot(leg.target), snapshot(nat.target)
        self.assertEqual(sorted(leg_tree), sorted(nat_tree), "produced file trees differ")
        for rel in sorted(leg_tree):
            if rel == LOCK_REL:
                continue
            self.assertEqual(leg_tree[rel], nat_tree[rel], f"{rel} differs between routes")

        self.assertIn(LOCK_REL, leg_tree, "legacy route produced no lock")
        self.assertIn(LOCK_REL, nat_tree, "native route produced no lock")
        leg_lock = (leg.target / LOCK_REL).read_text(encoding="utf-8")
        nat_lock = (nat.target / LOCK_REL).read_text(encoding="utf-8")
        for lock in (leg_lock, nat_lock):
            self.assertIn('[managed."AGENTS.md"]', lock, "lock lost its AGENTS.md key")
        self.assertEqual(
            _strip_sha256(leg_lock), _strip_sha256(nat_lock),
            _diff("lock (sha256 rows stripped)", _strip_sha256(leg_lock),
                  _strip_sha256(nat_lock)),
        )

    def test_help_is_the_same_stdout_and_exit_0_on_both_routes(self):
        native_cli = self._native_cli()
        fixture = _Fixture(self, "ai-specs-init-native-help-")

        leg_proc = fixture.legacy.run(LEGACY_CLI, "--help")
        nat_proc = fixture.native.run(native_cli, "--help")

        self.assertEqual(leg_proc.returncode, 0, leg_proc.stderr)
        self.assertEqual(nat_proc.returncode, 0, nat_proc.stderr)
        self.assertIn("Usage: ai-specs init", leg_proc.stdout)
        self.assertEqual(leg_proc.stdout, nat_proc.stdout,
                         _diff("help stdout", leg_proc.stdout, nat_proc.stdout))
        self.assertEqual(leg_proc.stderr, nat_proc.stderr,
                         _diff("help stderr", leg_proc.stderr, nat_proc.stderr))

    def test_unknown_flag_and_extra_positional_exit_2_with_same_stderr(self):
        native_cli = self._native_cli()
        fixture = _Fixture(self, "ai-specs-init-native-usage-")
        leg, nat = fixture.legacy, fixture.native

        leg_flag = leg.run(LEGACY_CLI, "--bogus-flag")
        nat_flag = nat.run(native_cli, "--bogus-flag")
        self.assertEqual(leg_flag.returncode, 2, leg_flag.stderr)
        self.assertEqual(nat_flag.returncode, 2, nat_flag.stderr)
        leg_err, nat_err = leg.stderr(leg_flag), nat.stderr(nat_flag)
        self.assertIn("ERROR: unknown flag: --bogus-flag", leg_err)
        self.assertEqual(leg_err, nat_err, _diff("unknown-flag stderr", leg_err, nat_err))
        self.assertEqual(leg_flag.stdout, "")
        self.assertEqual(nat_flag.stdout, "")

        leg_pos = leg.run(LEGACY_CLI, str(leg.target), "second-arg")
        nat_pos = nat.run(native_cli, str(nat.target), "second-arg")
        self.assertEqual(leg_pos.returncode, 2, leg_pos.stderr)
        self.assertEqual(nat_pos.returncode, 2, nat_pos.stderr)
        leg_err, nat_err = leg.stderr(leg_pos), nat.stderr(nat_pos)
        self.assertIn("ERROR: unexpected positional argument: second-arg", leg_err)
        self.assertEqual(leg_err, nat_err, _diff("extra-positional stderr", leg_err, nat_err))
        self.assertEqual(leg_pos.stdout, "")
        self.assertEqual(nat_pos.stdout, "")

    def test_missing_target_exits_1_with_same_stderr(self):
        native_cli = self._native_cli()
        fixture = _Fixture(self, "ai-specs-init-native-missing-")
        leg, nat = fixture.legacy, fixture.native

        leg_proc = leg.run(LEGACY_CLI, str(leg.root / "does-not-exist"))
        nat_proc = nat.run(native_cli, str(nat.root / "does-not-exist"))

        self.assertEqual(leg_proc.returncode, 1, leg_proc.stdout)
        self.assertEqual(nat_proc.returncode, 1, nat_proc.stdout)
        leg_err, nat_err = leg.stderr(leg_proc), nat.stderr(nat_proc)
        self.assertIn("ERROR: target path does not exist or is not a directory", leg_err)
        self.assertIn("does-not-exist", leg_err)
        self.assertIn("does-not-exist", nat_err)
        self.assertEqual(leg_err, nat_err, _diff("missing-target stderr", leg_err, nat_err))

    def test_second_run_keeps_stdout_and_user_owned_bytes(self):
        native_cli = self._native_cli()
        fixture = _Fixture(self, "ai-specs-init-native-second-")
        leg, nat = fixture.legacy, fixture.native

        first_leg = leg.run(LEGACY_CLI, str(leg.target))
        first_nat = nat.run(native_cli, str(nat.target))
        self.assertEqual(first_leg.returncode, 0, first_leg.stderr)
        self.assertEqual(first_nat.returncode, 0, first_nat.stderr)

        manifest = nat.target / "ai-specs" / "ai-specs.toml"
        root_gitignore = nat.target / ".gitignore"
        manifest_before = manifest.read_bytes()
        gitignore_before = root_gitignore.read_bytes()

        second_leg = leg.run(LEGACY_CLI, str(leg.target))
        second_nat = nat.run(native_cli, str(nat.target))
        self.assertEqual(second_leg.returncode, 0, second_leg.stderr)
        self.assertEqual(second_nat.returncode, 0, second_nat.stderr)

        # The two routes still agree line for line on the idempotent rerun...
        leg_out, nat_out = leg.stdout(second_leg), nat.stdout(second_nat)
        self.assertEqual(leg_out, nat_out, _diff("second-run stdout", leg_out, nat_out))
        # ...and the native rerun kept the user-owned bytes untouched (S4).
        self.assertIn("keep   ai-specs/ai-specs.toml", second_nat.stdout)
        self.assertEqual(manifest.read_bytes(), manifest_before,
                         "the second native run rewrote ai-specs/ai-specs.toml")
        self.assertEqual(root_gitignore.read_bytes(), gitignore_before,
                         "the second native run rewrote the root .gitignore")

    def test_force_refreshes_the_root_gitignore_block_on_both_routes(self):
        native_cli = self._native_cli()
        fixture = _Fixture(self, "ai-specs-init-native-force-")
        leg, nat = fixture.legacy, fixture.native

        first_leg = leg.run(LEGACY_CLI, str(leg.target))
        first_nat = nat.run(native_cli, str(nat.target))
        self.assertEqual(first_leg.returncode, 0, first_leg.stderr)
        self.assertEqual(first_nat.returncode, 0, first_nat.stderr)

        leg_gitignore = leg.target / ".gitignore"
        nat_gitignore = nat.target / ".gitignore"
        fresh_bytes = leg_gitignore.read_bytes()
        self.assertEqual(nat_gitignore.read_bytes(), fresh_bytes,
                         "first-run root .gitignore bytes differ between routes")

        # Corrupt a line INSIDE the managed block on both legs: --force must
        # strip the marked region and re-append the template over it.
        for path in (leg_gitignore, nat_gitignore):
            lines = path.read_text(encoding="utf-8").splitlines(keepends=True)
            marker_at = next(i for i, ln in enumerate(lines)
                             if ln.startswith(GITIGNORE_MARKER))
            lines.insert(marker_at + 1, "# injected junk — --force must remove this\n")
            path.write_text("".join(lines), encoding="utf-8")

        force_leg = leg.run(LEGACY_CLI, "--force", str(leg.target))
        force_nat = nat.run(native_cli, "--force", str(nat.target))
        self.assertEqual(force_leg.returncode, 0, force_leg.stderr)
        self.assertEqual(force_nat.returncode, 0, force_nat.stderr)

        self.assertIn("✓ refresh .gitignore (agent block)", force_leg.stdout)
        self.assertIn("✓ refresh .gitignore (agent block)", force_nat.stdout)
        leg_out, nat_out = leg.stdout(force_leg), nat.stdout(force_nat)
        self.assertEqual(leg_out, nat_out, _diff("--force stdout", leg_out, nat_out))
        self.assertEqual(leg_gitignore.read_bytes(), fresh_bytes,
                         "legacy --force did not restore the fresh agent block")
        self.assertEqual(nat_gitignore.read_bytes(), fresh_bytes,
                         "native --force did not restore the fresh agent block")

    def test_name_with_ampersand_lands_literally_on_the_native_route(self):
        # S8 (recorded divergence): the legacy render is
        # `sed "s/{{PROJECT_NAME}}/$PROJECT_NAME/g"`, and sed treats `&` in the
        # replacement as the whole match — so `R&D` would come out as
        # `R{{PROJECT_NAME}}D`. The native route substitutes literally. The
        # legacy output for this case is deliberately NOT asserted: the port is
        # the documented correct behaviour here.
        native_cli = self._native_cli()
        fixture = _Fixture(self, "ai-specs-init-native-ampname-")
        nat = fixture.native

        proc = nat.run(native_cli, "--name", "R&D", str(nat.target))
        self.assertEqual(proc.returncode, 0, proc.stderr)

        manifest = (nat.target / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        self.assertIn('name = "R&D"', manifest)
        self.assertNotIn("R{{PROJECT_NAME}}D", manifest,
                         "the native route applied the legacy sed mangling")

    def test_manifest_directory_is_never_destroyed_and_exits_1(self):
        # S4 + S3, from the verifier's probe: when ai-specs/ai-specs.toml is not
        # a regular file, BOTH routes must refuse, exit 1, and leave the path
        # exactly as they found it. stderr text is exempt (the shell's message is
        # bash's own redirection error, not a designed diagnostic — S12).
        native_cli = self._native_cli()
        fixture = _Fixture(self, "ai-specs-init-native-tomldir-")
        leg, nat = fixture.legacy, fixture.native

        for route in (leg, nat):
            (route.target / "ai-specs" / "ai-specs.toml").mkdir(parents=True)

        leg_proc = leg.run(LEGACY_CLI, str(leg.target))
        nat_proc = nat.run(native_cli, str(nat.target))

        self.assertEqual(leg_proc.returncode, 1, leg_proc.stderr)
        self.assertEqual(nat_proc.returncode, 1, nat_proc.stderr)
        for route, label in ((leg, "legacy"), (nat, "native")):
            toml_path = route.target / "ai-specs" / "ai-specs.toml"
            self.assertTrue(toml_path.is_dir(), f"{label}: the directory was replaced by a file")
            self.assertEqual([], sorted(p.name for p in toml_path.iterdir()),
                             f"{label}: something was written inside the directory")
        self.assertEqual(leg.stdout(leg_proc), nat.stdout(nat_proc),
                         _diff("stdout", leg.stdout(leg_proc), nat.stdout(nat_proc)))

    def test_missing_name_value_refuses_without_writing(self):
        # JD-B-001 (HIGH, converged with JD-A-001): a trailing `--name` aborts the
        # shell verb at `shift 2` under `set -euo pipefail` — exit 1, nothing
        # written. The port keeps the exit code; only the message text differs
        # (bash's own "shift count out of range" is not a designed message, S12).
        native_cli = self._native_cli()
        fixture = _Fixture(self, "ai-specs-init-native-noname-")
        leg, nat = fixture.legacy, fixture.native

        # No positional: each route reads its own cwd as the project, so the cwd
        # is the throwaway project and never the checkout.
        leg_proc = leg.run(LEGACY_CLI, "--name", cwd=leg.target)
        nat_proc = nat.run(native_cli, "--name", cwd=nat.target)

        self.assertEqual(leg_proc.returncode, 1, leg_proc.stderr)
        self.assertEqual(nat_proc.returncode, 1, nat_proc.stderr)
        for route, label in ((leg, "legacy"), (nat, "native")):
            written = sorted(p.relative_to(route.target) for p in route.target.rglob("*"))
            self.assertEqual([], written, f"{label}: the aborted run wrote files")
        self.assertIn("--name requires a value", nat_proc.stderr)

    def test_tui_gate_delegates_the_wizard_to_the_legacy_script(self):
        native_cli = self._native_cli()

        # A wizard that cancels (rc 1) without writing: both routes must exit 1
        # with the legacy cancel message and leave no manifest behind.
        cancel = _Fixture(self, "ai-specs-init-native-tui-cancel-")
        stub_cancel = _write_stub(cancel.base, rc=1, write_out=False)
        extra = {"AI_SPECS_INIT_TUI_PY": str(stub_cancel)}
        leg = cancel.legacy
        nat = cancel.native
        leg_proc = leg.run(LEGACY_CLI, "--tui", str(leg.target), extra_env=extra)
        nat_proc = nat.run(native_cli, "--tui", str(nat.target), extra_env=extra)
        self.assertEqual(leg_proc.returncode, 1, leg_proc.stderr)
        self.assertEqual(nat_proc.returncode, 1, nat_proc.stderr)
        self.assertIn("cancelled", leg_proc.stderr.lower())
        self.assertIn("cancelled", nat_proc.stderr.lower())
        self.assertFalse((leg.target / "ai-specs" / "ai-specs.toml").exists())
        self.assertFalse((nat.target / "ai-specs" / "ai-specs.toml").exists())
        leg_out, nat_out = leg.stdout(leg_proc), nat.stdout(nat_proc)
        self.assertEqual(leg_out, nat_out, _diff("tui-cancel stdout", leg_out, nat_out))
        leg_err, nat_err = leg.stderr(leg_proc), nat.stderr(nat_proc)
        self.assertEqual(leg_err, nat_err, _diff("tui-cancel stderr", leg_err, nat_err))

        # A wizard that stages the manifest (rc 0): both routes install it with
        # the legacy "(from TUI)" marker.
        ok = _Fixture(self, "ai-specs-init-native-tui-ok-")
        stub_ok = _write_stub(ok.base, rc=0, write_out=True)
        extra = {"AI_SPECS_INIT_TUI_PY": str(stub_ok)}
        leg = ok.legacy
        nat = ok.native
        leg_proc = leg.run(LEGACY_CLI, "--tui", str(leg.target), extra_env=extra)
        nat_proc = nat.run(native_cli, "--tui", str(nat.target), extra_env=extra)
        self.assertEqual(leg_proc.returncode, 0, leg_proc.stderr)
        self.assertEqual(nat_proc.returncode, 0, nat_proc.stderr)
        self.assertIn("(from TUI)", leg_proc.stdout)
        self.assertIn("(from TUI)", nat_proc.stdout)
        self.assertIn('name = "stub"',
                      (leg.target / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))
        self.assertIn('name = "stub"',
                      (nat.target / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))
        leg_out, nat_out = leg.stdout(leg_proc), nat.stdout(nat_proc)
        self.assertEqual(leg_out, nat_out, _diff("tui-success stdout", leg_out, nat_out))
        leg_err, nat_err = leg.stderr(leg_proc), nat.stderr(nat_proc)
        self.assertEqual(leg_err, nat_err, _diff("tui-success stderr", leg_err, nat_err))


if __name__ == "__main__":
    unittest.main()
