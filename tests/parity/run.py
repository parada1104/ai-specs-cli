#!/usr/bin/env python3
"""Suite entry for the differential parity harness (card [Go 03]).

Builds the Go root-module binary (card 04, cmd/ai-specs) into a temp dir
(CGO_ENABLED=0) and runs the fixture corpus legacy-vs-Go. When go is absent
or the build fails, falls back to the identical-by-shim self-test mode
(both legs legacy) — that mode MUST report zero deltas.

Exit codes: 0 = no unexplained delta; 1 = at least one delta (fails the
suite phase wired in tests/run.sh; validate.sh picks the phase up).
"""
from __future__ import annotations

import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import parity


def main() -> int:
    with tempfile.TemporaryDirectory(prefix="ai-specs-parity-") as td:
        go_cli = parity.build_go_binary(Path(td))
        code, report = parity.run_corpus(go_cli, Path(td))
        print(report)
        return code


if __name__ == "__main__":
    sys.exit(main())
