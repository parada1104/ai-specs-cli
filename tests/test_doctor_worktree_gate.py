"""Doctor worktree-gate check tests (Phase 3, task 3.14).

Severity table (design §6.5 / spec "Diagnostics for gate implementation
health"):
  OK    Go binary resolved, version matches stamp, selftest passes
  INFO  gate_impl=bash configured explicitly
  WARN  gate_impl=auto falling back to Bash / version mismatch
  ERROR gate_impl=go with no usable binary (failing open)
  ERROR digest mismatch recorded at last acquisition
"""
from __future__ import annotations

import os
import tempfile
import unittest
from pathlib import Path

from _blackbox import isolated_home, invoke

ROOT = Path(__file__).resolve().parents[1]


def _uname_platform() -> tuple[str, str]:
    sysname = os.uname().sysname
    machine = os.uname().machine
    goos = "darwin" if sysname == "Darwin" else "linux" if sysname == "Linux" else ""
    if machine in ("arm64", "aarch64"):
        goarch = "arm64"
    elif machine in ("x86_64", "amd64"):
        goarch = "amd64"
    else:
        goarch = ""
    return goos, goarch


def _cli_version() -> str:
    text = (ROOT / "VERSION").read_text(encoding="utf-8").strip()
    return text or "dev"


def _gate_bin_path() -> Path:
    goos, goarch = _uname_platform()
    return (
        ROOT / "cache" / "bin" / "worktree-gate" / _cli_version()
        / f"{goos}-{goarch}" / "worktree-gate"
    )


def _mismatch_path() -> Path:
    return ROOT / "cache" / "bin" / "worktree-gate" / _cli_version() / "last-digest-mismatch.txt"


class WorktreeGateDoctorTests(unittest.TestCase):
    def _project(self, *, gate_impl: str | None = None, enabled: bool = True) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name) / "prj"
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
        return root

    def _doctor(self, root: Path):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        home = isolated_home(Path(td.name))
        invoke(root, "refresh-bundled", cli_home=home)
        return invoke(root, "doctor", cli_home=home)

    def _checks(self, root: Path) -> list[str]:
        result = self._doctor(root)
        return [
            ln for ln in result.stdout.splitlines()
            if "worktree-gate  " in ln
        ]

    def _plant_fake_binary(self, version: str = "9.9.9") -> Path:
        path = _gate_bin_path()
        path.parent.mkdir(parents=True, exist_ok=True)
        existed = path.exists()
        previous = path.read_bytes() if existed else None
        previous_mode = path.stat().st_mode if existed else None

        def _restore() -> None:
            if previous is None:
                path.unlink(missing_ok=True)
            else:
                path.write_bytes(previous)
                if previous_mode is not None:
                    os.chmod(path, previous_mode)

        self.addCleanup(_restore)
        path.write_text(
            "#!/bin/sh\n"
            f'case "$1" in --version) echo {version};; --selftest) exit 0;; esac\n'
            "exit 0\n"
        )
        os.chmod(path, 0o755)
        return path

    def _hide_gate_binary(self) -> None:
        """Reconstruct the no-usable-binary state against the live CLI cache."""
        path = _gate_bin_path()
        if not path.exists():
            return
        hidden = path.with_name(path.name + ".hidden-by-bb-doctor-test")
        path.rename(hidden)
        self.addCleanup(lambda: hidden.rename(path) if hidden.exists() else None)

    def test_recipe_disabled_skips_check(self):
        root = self._project(enabled=False)
        self.assertEqual(self._checks(root), [])

    def test_gate_impl_bash_reports_info(self):
        root = self._project(gate_impl="bash")
        checks = self._checks(root)
        self.assertEqual(len(checks), 1)
        self.assertIn("INFO", checks[0])
        self.assertIn("bash", checks[0])

    def test_go_without_binary_reports_error_failing_open(self):
        root = self._project(gate_impl="go")
        self._hide_gate_binary()
        expected = _gate_bin_path()
        checks = self._checks(root)
        self.assertEqual(len(checks), 1)
        self.assertIn("ERROR", checks[0])
        self.assertIn("failing open", checks[0])
        self.assertIn(str(expected), checks[0])

    def test_auto_without_binary_reports_warn_fallback(self):
        root = self._project(gate_impl="auto")
        self._hide_gate_binary()
        checks = self._checks(root)
        self.assertEqual(len(checks), 1)
        self.assertIn("WARN", checks[0])
        self.assertIn("Bash", checks[0])

    def test_healthy_binary_reports_ok(self):
        root = self._project()
        launcher = root / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
        launcher.parent.mkdir(parents=True)
        launcher.write_text('stamped_gate_version="9.9.9"\n')
        self._plant_fake_binary("9.9.9")
        checks = self._checks(root)
        self.assertEqual(len(checks), 1)
        self.assertIn("OK", checks[0])
        self.assertIn("9.9.9", checks[0])

    def test_version_mismatch_reports_warn(self):
        root = self._project()
        launcher = root / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
        launcher.parent.mkdir(parents=True)
        launcher.write_text('stamped_gate_version="8.8.8"\n')
        self._plant_fake_binary("9.9.9")
        checks = self._checks(root)
        self.assertEqual(len(checks), 1)
        self.assertIn("WARN", checks[0])
        self.assertIn("8.8.8", checks[0])

    def test_digest_mismatch_record_reports_error(self):
        root = self._project()
        mismatch = _mismatch_path()
        mismatch.parent.mkdir(parents=True, exist_ok=True)
        existed = mismatch.exists()
        previous = mismatch.read_text(encoding="utf-8") if existed else None

        def _restore() -> None:
            if previous is None:
                mismatch.unlink(missing_ok=True)
            else:
                mismatch.write_text(previous, encoding="utf-8")

        self.addCleanup(_restore)
        mismatch.write_text(
            "worktree-gate: digest mismatch for worktree-gate-darwin-arm64; "
            "artifact deleted and never executed"
        )
        checks = self._checks(root)
        self.assertEqual(len(checks), 1)
        self.assertIn("ERROR", checks[0])
        self.assertIn("digest mismatch", checks[0])
        self.assertIn("never executed", checks[0])


if __name__ == "__main__":
    unittest.main()
