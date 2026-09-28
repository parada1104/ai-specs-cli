#!/usr/bin/env python3
"""Suite entry for the differential parity harness (card [Go 03]).

Default mode: builds the Go root-module binary (card 04, cmd/ai-specs) into a
temp dir (CGO_ENABLED=0) and runs the fixture corpus legacy-vs-Go. The Go
build is REQUIRED: when go is absent or the build fails, the entry FAILS
LOUDLY with exit 2 — the wired CI phase must never pass silently without
measuring (reliability finding: the previous silent identical-by-shim
fallback defeated acceptance (d)).

`--self-test` is the EXPLICIT legacy-vs-legacy identical-by-shim mode
(acceptance (a)): both legs run the legacy launcher and the corpus must
report zero deltas; it is never a silent fallback of the default mode.

Exit codes: 0 = no unexplained delta; 1 = at least one delta (fails the
suite phase wired in tests/run.sh; validate.sh picks the phase up);
2 = Go build unavailable or failed (fail-loudly).
"""
from __future__ import annotations

import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import parity


def main(argv: list[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    self_test = "--self-test" in args
    with tempfile.TemporaryDirectory(prefix="ai-specs-parity-") as td:
        go_cli = None if self_test else parity.build_go_binary(Path(td))
        if go_cli is None and not self_test:
            sys.stderr.write(
                "parity: FAIL — the Go build (cmd/ai-specs) is REQUIRED for the "
                "wired parity phase but is unavailable (no go toolchain or the "
                "build failed). The harness must not pass silently without "
                "measuring. Re-run with --self-test for the explicit "
                "legacy-vs-legacy self-test (acceptance-a mode), or fix the Go "
                "build.\n")
            return 2
        code, report = parity.run_corpus(go_cli, Path(td))
        print(report)
        return code


if __name__ == "__main__":
    sys.exit(main())
