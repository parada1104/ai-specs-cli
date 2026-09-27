"""Doctor worktree-gate check tests (Phase 3, task 3.14), black-box via the CLI.

Severity table (design §6.5 / spec "Diagnostics for gate implementation
health"):
  OK    Go binary resolved, version matches stamp, selftest passes
  INFO  gate_impl=bash configured explicitly
  WARN  gate_impl=auto falling back to Bash / version mismatch
  ERROR gate_impl=go with no usable binary (failing open)
  ERROR digest mismatch recorded at last acquisition

Every test drives `bin/ai-specs doctor` and asserts on its rendered output.
"""
from __future__ import annotations

import platform
import re
import shutil
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


def _platform() -> tuple[str, str]:
    """Mirror gate_binary.detect_platform for staging cache paths in tests."""
    goos = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system(), "")
    machine = platform.machine()
    goarch = "arm64" if machine in ("arm64", "aarch64") else (
        "amd64" if machine in ("x86_64", "amd64") else "")
    return goos, goarch


def _make_home(base: Path) -> Path:
    """Isolated CLI home with a REAL lib copy and an empty catalog.

    doctor.py derives its cache root from its own realpath, so a symlinked
    lib would resolve back into the repository and make the gate check read
    (never write) repo cache state. A real copy keeps every cache lookup
    (binary, verification receipt, digest-mismatch record) inside temp; the
    empty catalog removes the SHA256SUMS trust root so a receipt-backed stub
    binary is accepted exactly like an acquired one.
    """
    home = isolated_home(base, catalog=False)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor"),
    )
    return home


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


def _gate_cache_dir(home: Path) -> Path:
    version = (home / "VERSION").read_text(encoding="utf-8").strip()
    goos, goarch = _platform()
    return home / "cache" / "bin" / "worktree-gate" / version / f"{goos}-{goarch}"


def _stage_gate_stub(home: Path, *, version: str) -> None:
    """Install a receipt-backed stub binary answering --version.

    doctor runs `--version` and `--selftest` against the resolved binary; the
    stub reports the given version and passes everything else.
    """
    bindir = _gate_cache_dir(home)
    bindir.mkdir(parents=True, exist_ok=True)
    stub = bindir / "worktree-gate"
    stub.write_text(
        "#!/bin/sh\n"
        'if [ "$1" = "--version" ]; then\n'
        f'  printf \'%s\\n\' "{version}"\n'
        "  exit 0\n"
        "fi\n"
        "exit 0\n",
        encoding="utf-8",
    )
    stub.chmod(0o755)
    (bindir / "worktree-gate.verified").write_text("status=verified\n", encoding="utf-8")


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
        self._write_launcher(root, 'stamped_gate_version="9.9.9"\n')
        _stage_gate_stub(self.home, version="9.9.9")
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
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0][0], "ERROR")
        self.assertIn("failing open", checks[0][1])
        version = (self.home / "VERSION").read_text(encoding="utf-8").strip()
        goos, goarch = _platform()
        expected_suffix = f"cache/bin/worktree-gate/{version}/{goos}-{goarch}/worktree-gate"
        self.assertIn(expected_suffix, checks[0][1])
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

    def test_healthy_binary_reports_ok(self):
        root = self._project()
        self._write_launcher(root, 'stamped_gate_version="9.9.9"\n')
        _stage_gate_stub(self.home, version="9.9.9")
        checks, code = self._checks(root)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0][0], "OK")
        self.assertIn("9.9.9", checks[0][1])
        self.assertEqual(code, 0)

    def test_version_mismatch_reports_warn(self):
        root = self._project()
        self._write_launcher(root, 'stamped_gate_version="8.8.8"\n')
        _stage_gate_stub(self.home, version="9.9.9")
        checks, code = self._checks(root)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0][0], "WARN")
        self.assertIn("8.8.8", checks[0][1])
        # Frozen exit contract: WARN never affects the exit code.
        self.assertEqual(code, 0)

    def test_digest_mismatch_record_reports_error(self):
        root = self._project()
        version = (self.home / "VERSION").read_text(encoding="utf-8").strip()
        goos, goarch = _platform()
        mismatch = (
            self.home / "cache" / "bin" / "worktree-gate" / version
            / "last-digest-mismatch.txt"
        )
        mismatch.parent.mkdir(parents=True, exist_ok=True)
        mismatch.write_text(
            f"worktree-gate: digest mismatch for worktree-gate-{goos}-{goarch}; "
            "artifact deleted and never executed",
            encoding="utf-8",
        )
        checks, code = self._checks(root)
        self.assertEqual(len(checks), 1)
        self.assertEqual(checks[0][0], "ERROR")
        self.assertIn("digest mismatch", checks[0][1])
        self.assertIn("never executed", checks[0][1])
        self.assertEqual(code, 1)


if __name__ == "__main__":
    unittest.main()
