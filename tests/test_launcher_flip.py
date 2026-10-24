"""GO-20 U2e T2: bin/ai-specs execs the native binary for every verb.

The launcher resolves the binary from AI_SPECS_GO_BIN, then from
$AI_SPECS_HOME/dist/ai-specs. A missing binary is a loud failure (exit 1),
never a silent fallback to the legacy Bash/Python route.

A fake native binary records its argv, so these tests prove the dispatch
without building Go: every verb must reach the binary with its arguments.
"""
from __future__ import annotations

import os
import shutil
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
LAUNCHER = ROOT / "bin" / "ai-specs"

VERBS = ("init", "sync", "sync-agent", "refresh-bundled", "add-dep", "skills",
         "doctor", "rules-audit", "recipe", "configure-recipes", "upgrade",
         "version", "hub", "help")

_FAKE = """#!/bin/sh
printf 'NATIVE-DISPATCH'
for a in "$@"; do printf ' [%s]' "$a"; done
printf '\\n'
exit 0
"""


class LauncherDispatchTests(unittest.TestCase):
    def setUp(self) -> None:
        self._tmp = Path(tempfile.mkdtemp(prefix="launcher-flip-"))
        self.fake = self._tmp / "ai-specs-fake"
        self.fake.write_text(_FAKE, encoding="utf-8")
        self.fake.chmod(self.fake.stat().st_mode | stat.S_IEXEC)
        self.home = self._tmp / "home"
        self.home.mkdir()

    def tearDown(self) -> None:
        shutil.rmtree(self._tmp, ignore_errors=True)

    def _run(self, *args: str, env_extra: dict | None = None):
        env = {"PATH": os.environ.get("PATH", ""),
               "HOME": str(self.home),
               "AI_SPECS_GO_BIN": str(self.fake),
               "LC_ALL": "C", "LANG": "C"}
        if env_extra is not None:
            env = env_extra
        return subprocess.run([str(LAUNCHER), *args], env=env, text=True,
                              capture_output=True, check=False, timeout=60)

    def test_every_verb_reaches_the_native_binary_with_its_arguments(self):
        for verb in VERBS:
            with self.subTest(verb=verb):
                proc = self._run(verb, "a1", "b2")
                self.assertEqual(proc.returncode, 0, proc.stderr)
                self.assertEqual(
                    proc.stdout,
                    f"NATIVE-DISPATCH [{verb}] [a1] [b2]\n")

    def test_bare_invocation_passes_no_verb_to_the_binary(self):
        proc = self._run()
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertEqual(proc.stdout, "NATIVE-DISPATCH\n")

    def test_missing_binary_fails_loudly_and_never_runs_legacy_bash(self):
        empty_home = self._tmp / "no-dist"
        empty_home.mkdir()
        env = {"PATH": os.environ.get("PATH", ""), "HOME": str(self.home),
               "AI_SPECS_HOME": str(empty_home),
               "LC_ALL": "C", "LANG": "C"}
        proc = subprocess.run([str(LAUNCHER), "doctor"], env=env, text=True,
                              capture_output=True, check=False, timeout=60)
        self.assertEqual(proc.returncode, 1)
        self.assertIn("native binary not found", proc.stderr)
        self.assertIn("scripts/build-cli.sh", proc.stderr)
        self.assertEqual(proc.stdout, "")


if __name__ == "__main__":
    unittest.main()
