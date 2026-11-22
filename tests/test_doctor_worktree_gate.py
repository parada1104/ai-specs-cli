"""Doctor worktree-gate check tests (Phase 3, task 3.14), black-box via the CLI.

Severity table (design §6.5 / spec "Diagnostics for gate implementation
health"):
  OK    installed CLI serves worktree-gate and selftest passes
  INFO  leftover legacy launcher file
  WARN  materialized launcher still resolves the retired separate binary
  ERROR retired gate_impl=bash or missing/failing installed CLI (failing open)

Every test drives `bin/ai-specs doctor` and asserts on its rendered output.
"""
from __future__ import annotations

import re
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import (  # noqa: E402
    cache_project_dir,
    invoke,
    isolated_home,
)

ROOT = Path(__file__).resolve().parents[1]

BUNDLED_SKILLS = (
    "harness-lifecycle",
    "harness-recipes",
    "harness-skills-deps",
    "skill-creator",
    "skill-sync",
)
BUNDLED_COMMANDS = ("rules-audit", "skills-as-rules")

_LINE_RE = re.compile(r"^\s*(OK|INFO|WARN|ERROR)\s+(?P<name>\S+)\s+(?P<body>.*)$")


def _make_home(base: Path) -> Path:
    """Isolated CLI home with a real lib copy and an empty catalog.

    ``lib/`` is not installed: doctor is the native binary, and the empty
    catalog keeps unrelated bundled-cache checks from affecting its exit code.
    """
    return isolated_home(base, catalog=False)


def _seed_clean_cache(root: Path, home: Path) -> None:
    """Pre-seed the per-project bundled cache so only the worktree-gate check
    can influence the frozen exit-code contract (exit 1 iff any ERROR)."""
    bundled = cache_project_dir(root, home) / ".bundled"
    for skill in BUNDLED_SKILLS:
        (bundled / "skills" / skill).mkdir(parents=True, exist_ok=True)
    for command in BUNDLED_COMMANDS:
        path = bundled / "commands" / f"{command}.md"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("# bundled\n", encoding="utf-8")


def _stage_gate_cli(home: Path, *, working: bool = True) -> None:
    """Install a CLI fixture whose worktree-gate selftest can pass or fail."""
    dist = home / "dist"
    dist.mkdir(parents=True, exist_ok=True)
    cli = dist / "ai-specs"
    result = "echo ok\n  exit 0" if working else "echo selftest exploded >&2\n  exit 1"
    cli.write_text(
        "#!/bin/sh\n"
        'if [ "$1" = "worktree-gate" ] && [ "$2" = "--selftest" ]; then\n'
        f"  {result}\n"
        "fi\nexit 1\n",
        encoding="utf-8",
    )
    cli.chmod(0o755)


def _named_checks(stdout: str, name: str) -> list[tuple[str, str]]:
    """The rendered (severity, message+guidance) lines for one check name."""
    found = []
    for line in stdout.splitlines():
        match = _LINE_RE.match(line)
        if match and match.group("name") == name:
            found.append((match.group(1), match.group("body")))
    return found


class WorktreeGateDoctorTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.home = _make_home(self.base)

    def _project(self, *, gate_impl: str | None = None, enabled: bool = True) -> Path:
        root = self.base / "prj"
        root.mkdir(exist_ok=True)
        ai = root / "ai-specs"
        ai.mkdir()
        cfg = ""
        if gate_impl:
            cfg = f"[recipes.worktree-flow.config]\ngate_impl = '{gate_impl}'\n"
        (ai / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = []\n\n"
            f"[recipes.worktree-flow]\nenabled = {'true' if enabled else 'false'}\n"
            + cfg
        )
        (root / "AGENTS.md").write_text("# agents\n")
        _seed_clean_cache(root, self.home)
        return root

    def _write_launcher(self, root: Path, body: str) -> None:
        launcher = root / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
        launcher.parent.mkdir(parents=True, exist_ok=True)
        launcher.write_text(body, encoding="utf-8")

    def _checks(self, root: Path) -> tuple[list[tuple[str, str]], int]:
        result = invoke(root, "doctor", cli_home=self.home)
        return _named_checks(result.stdout, "worktree-gate"), result.returncode

    def test_recipe_disabled_skips_check(self):
        root = self._project(enabled=False)
        checks, code = self._checks(root)
        self.assertEqual(checks, [])
        self.assertEqual(code, 0)

    def test_gate_impl_bash_reports_retired_error(self):
        root = self._project(gate_impl="bash")
        checks, code = self._checks(root)
        errors = [body for sev, body in checks if sev == "ERROR"]
        self.assertEqual(len(errors), 1)
        self.assertIn("retired", errors[0])
        self.assertIn("auto", errors[0])
        self.assertIn("go", errors[0])
        self.assertIn("sync", errors[0])
        blob = " ".join(f"{sev} {body}" for sev, body in checks)
        self.assertNotIn("rollback lever", blob)
        # Frozen exit contract: exit 1 iff at least one ERROR was rendered.
        self.assertEqual(code, 1)

    def test_stamped_bash_reports_retired_error(self):
        root = self._project(gate_impl="auto")
        self._write_launcher(
            root, 'stamped_gate_impl="bash"\nstamped_gate_version="9.9.9"\n')
        checks, code = self._checks(root)
        errors = [body for sev, body in checks if sev == "ERROR"]
        self.assertTrue(errors)
        self.assertIn("retired", errors[0])
        self.assertEqual(code, 1)

    def test_leftover_legacy_file_reports_info_with_rm_hint(self):
        root = self._project(gate_impl="auto")
        leftover = (
            root / "ai-specs" / "recipes" / "worktree-flow"
            / "hooks" / "worktree-gate-legacy.sh"
        )
        leftover.parent.mkdir(parents=True)
        leftover.write_text("inert leftover\n")
        _stage_gate_cli(self.home)
        checks, code = self._checks(root)
        infos = [body for sev, body in checks if sev == "INFO"]
        self.assertEqual(len(infos), 1)
        self.assertIn("leftover", infos[0])
        self.assertIn("rm ai-specs/recipes/worktree-flow/hooks/worktree-gate-legacy.sh",
                      infos[0])
        self.assertNotIn("stale", infos[0].lower())
        self.assertEqual(code, 0)

    def test_go_without_binary_reports_error_failing_open(self):
        root = self._project(gate_impl="go")
        checks, code = self._checks(root)
        self.assertEqual(checks, [("ERROR", "ai-specs CLI is missing or not executable at <TEMP>/dist/ai-specs; "
                                            "the gate is failing open  (reinstall ai-specs via install.sh)")])
        self.assertEqual(code, 1)

    def test_auto_without_binary_reports_error_failing_open(self):
        root = self._project(gate_impl="auto")
        checks, code = self._checks(root)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0][0], "ERROR")
        self.assertIn("failing open", checks[0][1])
        self.assertNotIn("Bash", checks[0][1])
        self.assertNotIn("rollback lever", checks[0][1])
        self.assertEqual(code, 1)

    def test_healthy_cli_reports_ok(self):
        root = self._project()
        _stage_gate_cli(self.home)
        checks, code = self._checks(root)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0][0], "OK")
        self.assertIn("serves worktree-gate; selftest passed", checks[0][1])
        self.assertEqual(code, 0)

    def test_cli_selftest_failure_reports_error_failing_open(self):
        root = self._project()
        _stage_gate_cli(self.home, working=False)
        checks, code = self._checks(root)
        self.assertEqual(checks, [("ERROR", "ai-specs CLI at <TEMP>/dist/ai-specs failed worktree-gate --selftest: "
                                            "selftest exploded; the gate is failing open  (run ai-specs upgrade)")])
        self.assertEqual(code, 1)


if __name__ == "__main__":
    unittest.main()
