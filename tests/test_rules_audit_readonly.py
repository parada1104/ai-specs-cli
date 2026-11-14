"""Filesystem-mutation test for the native `rules-audit` port (card [Go 08]).

Sibling of ``tests/test_doctor_readonly.py``: ``rules-audit`` is the second
native verb whose contract says "never modifies project files". This test
proves it at the filesystem boundary: it snapshots the project AND the isolated
install home, runs the built Go binary, and asserts both snapshots are
byte-identical afterwards (paths, modes, contents and symlink targets), the
exit code is 0, and stdout parses as the schema_version 1 JSON inventory.
"""
from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent / "parity"))
import parity  # noqa: E402


def _write_project(project: Path) -> None:
    """Isolated fixture project with a rich legacy surface so the scan reads
    .cursor rules, .cursorrules, AGENTS.md, the manifest and a local skill."""
    (project / ".cursor" / "rules").mkdir(parents=True)
    (project / ".cursor" / "rules" / "rules.mdc").write_text(
        "---\n"
        "description: workflow rules\n"
        "alwaysApply: true\n"
        "---\n"
        "# Workflow\n"
        "Run tests first.\n",
        encoding="utf-8")
    (project / ".cursorrules").write_text(
        "Use the vault and open a pull request.\n", encoding="utf-8")
    (project / "AGENTS.md").write_text(
        "# Overview\n"
        "See the `tdd-flow` skill.\n"
        "\n"
        "## Runtime notes\n"
        "The Trello board is the source of truth.\n",
        encoding="utf-8")
    ai = project / "ai-specs"
    (ai / "skills" / "local-skill").mkdir(parents=True)
    (ai / "skills" / "local-skill" / "SKILL.md").write_text(
        "# local skill\n", encoding="utf-8")
    (ai / "ai-specs.toml").write_text(
        "[project]\n"
        "name = 'rules-audit-readonly'\n"
        "\n"
        "[agents]\n"
        "enabled = ['claude', 'pi']\n"
        "\n"
        "[recipes.tdd-flow]\n"
        "enabled = true\n",
        encoding="utf-8")


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


class RulesAuditReadOnlyTests(unittest.TestCase):
    def test_rules_audit_writes_nothing_to_the_project_or_the_install(self):
        with tempfile.TemporaryDirectory(prefix="ai-specs-rules-audit-ro-") as td:
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
                [str(go_cli), "rules-audit", str(project)], cwd=project, env=env,
                text=True, capture_output=True, check=False, timeout=600)

            after_project = parity.snapshot_tree(project, ctx)
            after_home = parity.snapshot_tree(home, ctx)

            project_delta = _delta_lines(before_project, after_project)
            self.assertEqual(
                project_delta, [],
                "rules-audit modified the project tree:\n"
                + "\n".join(project_delta) + f"\nstderr={proc.stderr!r}")
            home_delta = _delta_lines(before_home, after_home)
            self.assertEqual(
                home_delta, [],
                "rules-audit modified the install home:\n"
                + "\n".join(home_delta) + f"\nstderr={proc.stderr!r}")

            # Frozen contract: exit 0 and the schema_version 1 JSON inventory.
            self.assertEqual(
                proc.returncode, 0,
                f"exit = {proc.returncode}, want 0 (stderr={proc.stderr!r})")
            payload = json.loads(proc.stdout)
            self.assertEqual(payload.get("schema_version"), 1)


if __name__ == "__main__":
    unittest.main()
