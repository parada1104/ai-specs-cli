import os
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "catalog/recipes/worktree-flow/hooks/worktree-gate.sh"


class OverrideVerification(unittest.TestCase):
    def _launcher(self, d: Path) -> Path:
        text = SOURCE.read_text()
        for token, value in {"MODE": "always", "SCOPE": "auto",
                             "IMPL": "go", "VERSION": "test"}.items():
            text = text.replace("__WORKTREE_GATE_" + token + "__", value)
        launcher = d / "launcher"
        launcher.write_text(text.replace("__WORKTREE_REPO_TOPOLOGY__", "auto"))
        return launcher

    def _override(self, d: Path, selftest_rc: int) -> Path:
        override = d / "override"
        override.write_text(
            '#!/bin/sh\nprintf "%s\\n" "$*" >> "$TRACE"\n'
            f'[ "$1" = "--selftest" ] && exit {selftest_rc}\nexit 0\n'
        )
        override.chmod(0o755)
        return override

    def _run(self, d: Path, selftest_rc: int):
        env = dict(os.environ, WORKTREE_GATE_BIN=str(self._override(d, selftest_rc)),
                   WORKTREE_GATE_VERIFY="1", TRACE=str(d / "trace"),
                   WORKTREE_GATE_PROTECTED="main development", AI_SPECS_HOME=str(d / "home"))
        return subprocess.run(["bash", str(self._launcher(d))], input="{}",
                              text=True, capture_output=True, env=env)

    def test_explicit_override_still_gets_requested_selftest(self):
        with tempfile.TemporaryDirectory() as tmp:
            d = Path(tmp)
            p = self._run(d, 0)
            self.assertEqual((p.returncode, p.stdout, p.stderr), (0, "", ""))
            self.assertEqual((d / "trace").read_text().splitlines(), [
                "--selftest",
                "--gate-mode always --gate-scope auto "
                "--repo-topology auto --protected main development",
            ])

    def test_failing_override_selftest_is_ignored_and_never_runs_the_gate(self):
        with tempfile.TemporaryDirectory() as tmp:
            d = Path(tmp)
            p = self._run(d, 1)
            self.assertEqual((p.returncode, p.stdout), (0, ""))
            self.assertIn("WORKTREE_GATE_BIN failed --selftest", p.stderr)
            self.assertIn("not enforcing", p.stderr)  # no CLI to fall through to
            self.assertEqual((d / "trace").read_text().splitlines(), ["--selftest"])

    def test_failing_override_selftest_falls_back_to_cli_enforcement(self):
        with tempfile.TemporaryDirectory() as tmp:
            d = Path(tmp)
            cli = d / "home" / "dist" / "ai-specs"
            cli.parent.mkdir(parents=True)
            cli.write_text(
                '#!/bin/sh\n[ "$1" = worktree-gate ] || exit 1\n'
                'case "$2" in --version|--selftest) exit 0 ;; esac\n'
                'echo "blocked by CLI" >&2\nexit 2\n'
            )
            cli.chmod(0o755)

            p = self._run(d, 1)
            self.assertEqual((p.returncode, p.stdout), (2, ""))
            self.assertEqual(p.stderr, f"worktree-gate: WORKTREE_GATE_BIN failed --selftest: "
                                       f"{d / 'override'}; ignoring.\nblocked by CLI\n")
            self.assertEqual((d / "trace").read_text().splitlines(), ["--selftest"])


if __name__ == "__main__":
    unittest.main()
