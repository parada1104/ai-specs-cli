"""Tokenizer differential: Go tokenizer vs python3 shlex.split.

Pins the shlex-equivalent Go tokenizer
(catalog/recipes/worktree-flow/gate/tokenize.go) token-for-token against
python3 shlex.split(cmd, posix=True) over the shared corpus
tests/fixtures/worktree-gate-tokenizer-corpus.json. The retired Bash
tokenizer is not an oracle.

Every corpus entry is either {"cmd": ..., "tokens": [...]} or
{"cmd": ..., "error": true} — the pinned answers produced by the real shlex.
Three assertions run per case:

1. The corpus pin is still true: the local python3 shlex.split still yields
   the pinned tokens / still raises ValueError. This is the drift guard on
   the reference itself (design D9: the failure mode is part of the contract).
2. The Go binary's --tokenize diagnostic (JSON {"tokens": [...], "error": bool})
   matches the same pinned answer, token-for-token.
3. ValueError in the reference maps to error=true in the Go binary (the
   fail-open verdict pass1 depends on), and a clean parse maps to error=false.

The Go half skips loudly when no binary is built yet (task 1.17 / 1.22); the
Python pinning half always runs, so the corpus keeps guarding the reference
on machines without Go. Build with scripts/build-gate.sh, which emits
dist/worktree-gate-<goos>-<goarch>.
"""
from __future__ import annotations

import json
import platform
import shlex
import subprocess
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CORPUS = ROOT / "tests/fixtures/worktree-gate-tokenizer-corpus.json"
BINARY_GLOBS = sorted(ROOT.glob("dist/worktree-gate-*"))


def find_binary() -> Path | None:
    """Locate a built Go gate binary, preferring the current platform.

    scripts/build-gate.sh emits dist/worktree-gate-<goos>-<goarch>; the
    differential runner keys off those real build outputs so the Go half
    activates exactly when a binary exists.
    """
    system = platform.system().lower()
    machine = platform.machine().lower()
    want = f"worktree-gate-{system}-{machine}"
    for binary in BINARY_GLOBS:
        if binary.name == want:
            return binary
    return BINARY_GLOBS[0] if BINARY_GLOBS else None


def reference_tokens(cmd: str) -> tuple[list[str], bool]:
    """Run the real reference tokenizer; returns (tokens, ok)."""
    try:
        return shlex.split(cmd, posix=True), True
    except ValueError:
        return [], False


def go_tokens(binary: Path, cmd: str) -> tuple[list[str], bool]:
    """Run the Go binary's --tokenize diagnostic; returns (tokens, ok)."""
    result = subprocess.run(
        [str(binary), "--tokenize"],
        input=cmd,
        capture_output=True, text=True, timeout=60,
    )
    if result.returncode != 0:
        raise AssertionError(
            f"--tokenize exited {result.returncode}: {result.stderr!r}")
    diag = json.loads(result.stdout)
    return diag["tokens"], not diag.get("error", False)


def load_corpus() -> list[dict]:
    raw = CORPUS.read_text(encoding="utf-8")
    cases = json.loads(raw)
    if not cases:
        raise AssertionError("tokenizer corpus is empty")
    return cases


class TokenizerDifferentialTests(unittest.TestCase):
    def test_corpus_is_pinned_against_reference(self):
        # The corpus is the specification; the local reference must still
        # produce it. A drift in either direction (shlex semantics changed
        # under a Python upgrade, or the corpus was edited by hand) breaks
        # here first.
        for case in load_corpus():
            with self.subTest(cmd=case["cmd"]):
                tokens, ok = reference_tokens(case["cmd"])
                if case.get("error"):
                    self.assertFalse(ok, f"corpus says error but shlex accepted {case['cmd']!r}")
                else:
                    self.assertTrue(ok, f"corpus says tokens but shlex raised on {case['cmd']!r}")
                    self.assertEqual(tokens, case["tokens"],
                                     f"pinned tokens stale for {case['cmd']!r}")

    def test_go_tokenizer_matches_reference_token_for_token(self):
        binary = find_binary()
        if binary is None:
            self.skipTest(
                "no Go gate binary in dist/ (run scripts/build-gate.sh); "
                "Python-vs-corpus half still ran")
        for case in load_corpus():
            with self.subTest(cmd=case["cmd"]):
                tokens, ok = reference_tokens(case["cmd"])
                got_tokens, got_ok = go_tokens(binary, case["cmd"])
                self.assertEqual(got_ok, ok,
                                 f"error verdict mismatch for {case['cmd']!r}: "
                                 f"reference ok={ok}, Go ok={got_ok}")
                self.assertEqual(got_tokens, tokens,
                                 f"tokens mismatch for {case['cmd']!r}: "
                                 f"reference {tokens!r}, Go {got_tokens!r}")

    def test_error_maps_to_fail_open_verdict(self):
        # The whole point of the port (D9): a ValueError in shlex must be
        # exactly as invisible to the gate as an empty candidate list. The
        # --tokenize diagnostic must surface it the same way pass1 consumes
        # it — an explicit error verdict, never a corrupted token stream.
        binary = find_binary()
        if binary is None:
            self.skipTest("no Go gate binary in dist/")
        for case in load_corpus():
            if not case.get("error"):
                continue
            with self.subTest(cmd=case["cmd"]):
                result = subprocess.run(
                    [str(binary), "--tokenize"],
                    input=case["cmd"], capture_output=True, text=True, timeout=60,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                diag = json.loads(result.stdout)
                self.assertTrue(diag.get("error"), f"error case reported clean: {case['cmd']!r}")
                self.assertEqual(diag["tokens"], [],
                                 f"error case must yield no tokens: {case['cmd']!r}")


if __name__ == "__main__":
    unittest.main()
