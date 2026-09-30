"""Filesystem-mutation test for the native `doctor` port (card [Go 08]).

`doctor` is the first fully native verb whose contract says "never modifies
project files". This test proves it at the filesystem boundary: it snapshots
the project AND the isolated install home, runs the built Go binary, and
asserts both snapshots are byte-identical afterwards (paths, modes, contents
and symlink targets). The legacy implementation wrote Python bytecode caches
into the install home (parity defect D26); the port must not.

This covers `doctor` only. `rules-audit` is the other read-only native verb
and has its own sibling test (`tests/test_rules_audit_readonly.py`), so the
pair covers both read-only commands.
"""
from __future__ import annotations

import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent / "parity"))
import parity  # noqa: E402

_SUMMARY_RE = re.compile(
    r"^Summary: \d+ OK, \d+ INFO, \d+ WARN, (?P<errors>\d+) ERROR$", re.MULTILINE)


def _write_project(project: Path) -> None:
    """Isolated fixture project: two agents, tdd-flow + worktree-flow, empty
    generated-surface dirs (no sync, so doctor reads an unsynced state)."""
    manifest = (
        "[project]\n"
        "name = 'doctor-readonly'\n"
        "\n"
        "[agents]\n"
        "enabled = ['claude', 'pi']\n"
        "\n"
        "[recipes.tdd-flow]\n"
        "enabled = true\n"
        "\n"
        "[recipes.worktree-flow]\n"
        "enabled = true\n"
    )
    ai = project / "ai-specs"
    (ai / "skills").mkdir(parents=True)
    (ai / "commands").mkdir()
    (ai / "ai-specs.toml").write_text(manifest, encoding="utf-8")


def _delta_lines(before: dict, after: dict) -> list[str]:
    """Added / removed / changed paths between two snapshots, readable."""
    lines: list[str] = []
    for rel in sorted(set(before) | set(after)):
        a, b = before.get(rel), after.get(rel)
        if a is None:
            lines.append(f"  added:   {rel} -> {b}")
        elif b is None:
            lines.append(f"  removed: {rel} (was {a})")
        elif a != b:
            lines.append(f"  changed: {rel}\n    before={a}\n    after ={b}")
    return lines


class DoctorReadOnlyTests(unittest.TestCase):
    def test_doctor_writes_nothing_to_the_project_or_the_install(self):
        with tempfile.TemporaryDirectory(prefix="ai-specs-doctor-readonly-") as td:
            scratch = Path(td)
            go_cli = parity.build_go_binary(scratch)
            if go_cli is None:
                self.skipTest("go toolchain unavailable or the Go build failed")

            home = parity.make_home(scratch / "home")
            project = scratch / "project"
            project.mkdir()
            _write_project(project)
            (scratch / "user-home").mkdir()

            ctx = {"project_root": str(project), "home": str(home),
                   "scratch": str(scratch)}
            before_project = parity.snapshot_tree(project, ctx)
            before_home = parity.snapshot_tree(home, ctx)

            env = {**parity.BASE_ENV, "HOME": str(scratch / "user-home"),
                   "TMPDIR": str(scratch), "AI_SPECS_HOME": str(home)}
            proc = subprocess.run(
                [str(go_cli), "doctor", str(project)], cwd=project, env=env,
                text=True, capture_output=True, check=False, timeout=600)

            after_project = parity.snapshot_tree(project, ctx)
            after_home = parity.snapshot_tree(home, ctx)

            project_delta = _delta_lines(before_project, after_project)
            self.assertEqual(
                project_delta, [],
                "doctor modified the project tree:\n"
                + "\n".join(project_delta) + f"\nstderr={proc.stderr!r}")
            home_delta = _delta_lines(before_home, after_home)
            self.assertEqual(
                home_delta, [],
                "doctor modified the install home:\n"
                + "\n".join(home_delta) + f"\nstderr={proc.stderr!r}")

            # Frozen framing.
            self.assertIn("ai-specs doctor", proc.stdout)
            self.assertIn("  target: ", proc.stdout)
            self.assertIn("Summary: ", proc.stdout)

            # Frozen exit rule: 1 iff the report contains an ERROR check.
            match = _SUMMARY_RE.search(proc.stdout)
            self.assertIsNotNone(match, f"no Summary line in stdout: {proc.stdout!r}")
            expected = 1 if int(match.group("errors")) > 0 else 0
            self.assertEqual(
                proc.returncode, expected,
                f"exit = {proc.returncode}, want {expected} for {match.group(0)!r}")


if __name__ == "__main__":
    unittest.main()
