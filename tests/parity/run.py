#!/usr/bin/env python3
"""Suite entry for the differential parity harness (card [Go 03]).

Default mode: builds the Go root-module binary (card 04, cmd/ai-specs) into a
temp dir (CGO_ENABLED=0) and runs the fixture corpus legacy-vs-Go. The Go
build is REQUIRED: when go is absent or the build fails, the entry FAILS
LOUDLY with exit 2 — the wired CI phase must never pass silently without
measuring (reliability finding: the previous silent identical-by-shim
fallback defeated acceptance (d)).

The corpus runs in TWO gate modes so a zero-delta result names the gate
authority it measured (plan finding F3 / risk R7):

  * `gate-absent` — the historical behavior: isolated home, empty cache/, no
    verified gate binary, so every bridge degrades to the Python fallback;
  * `gate-present` — the nested worktree-gate module is built once into the
    temp dir and WORKTREE_GATE_BIN is pinned to it for BOTH legs, exercising
    the real Go-authority bridge path.

`AI_SPECS_GATE_OFFLINE=1` stays set in both modes (it only prevents network
acquisition). The built gate binary lives in the temp dir and is never
committed. In the default path the gate build is REQUIRED (exit 2 when
unavailable); `--self-test` states gate-present as unavailable instead.

`--self-test` is the EXPLICIT legacy-vs-legacy identical-by-shim mode
(acceptance (a)): both legs run the legacy launcher and the corpus must
report zero deltas; it is never a silent fallback of the default mode. It
runs both gate modes when a gate binary is buildable and says so explicitly
when it is not.

Exit codes: 0 = no unexplained delta; 1 = at least one delta (fails the
suite phase wired in tests/run.sh; validate.sh picks the phase up);
2 = required Go root build OR required gate build unavailable or failed
(fail-loudly).
"""
from __future__ import annotations

import re
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import parity

_GO_REQUIRED_MSG = (
    "parity: FAIL — the Go build (cmd/ai-specs) is REQUIRED for the "
    "wired parity phase but is unavailable (no go toolchain or the "
    "build failed). The harness must not pass silently without "
    "measuring. Re-run with --self-test for the explicit "
    "legacy-vs-legacy self-test (acceptance-a mode), or fix the Go "
    "build.\n")

_GATE_REQUIRED_MSG = (
    "parity: FAIL — the worktree-gate build "
    "(catalog/recipes/worktree-flow/gate) is REQUIRED for gate-present "
    "mode but is unavailable (no go toolchain or the build failed). The "
    "corpus must not pass without measuring the Go-authority bridge "
    "path (finding F3). Fix the gate build, or re-run with --self-test "
    "to measure only gate-absent explicitly.\n")

_GATE_UNAVAILABLE_NOTE = (
    "gate-present: UNAVAILABLE — the worktree-gate binary could not be "
    "built (no go toolchain or the build failed); this run measured "
    "gate-absent only.\n")


def _failing_count(report: str) -> int:
    """Read the `fixtures: N, failing: M` anchor of one mode report."""
    match = re.search(r"failing: (\d+)", report)
    return int(match.group(1)) if match else -1


def main(argv: list[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    self_test = "--self-test" in args
    with tempfile.TemporaryDirectory(prefix="ai-specs-parity-") as td:
        root = Path(td)
        go_cli = None if self_test else parity.build_go_binary(root)
        if go_cli is None and not self_test:
            sys.stderr.write(_GO_REQUIRED_MSG)
            return 2
        gate_bin = parity.build_gate_binary(root)
        if gate_bin is None and not self_test:
            sys.stderr.write(_GATE_REQUIRED_MSG)
            return 2
        modes = [(parity.GATE_ABSENT, None)]
        if gate_bin is not None:
            modes.append((parity.GATE_PRESENT, gate_bin))
        results: list[tuple[str, int, str]] = []
        exit_code = 0
        for gate_mode, gbin in modes:
            code, report = parity.run_corpus(
                go_cli, root / gate_mode, gate_mode=gate_mode, gate_bin=gbin)
            results.append((gate_mode, code, report))
            exit_code = exit_code or code
        for _gate_mode, _code, report in results:
            print(report)
        if gate_bin is None:
            sys.stderr.write(_GATE_UNAVAILABLE_NOTE)
        summary = ", ".join(
            f"{gm} failing={_failing_count(report)}"
            for gm, _code, report in results)
        if gate_bin is None:
            summary += ", gate-present NOT MEASURED"
        verdict = "PASS" if exit_code == 0 else "FAIL"
        print(f"parity summary: {summary} — {verdict}")
        return exit_code


if __name__ == "__main__":
    sys.exit(main())
