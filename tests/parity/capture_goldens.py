#!/usr/bin/env python3
"""Traces of the frozen legacy parity corpus (GO-20 U2e T2a) — capture RETIRED.

What the goldens are
--------------------
``tests/parity/goldens/<gate-mode>/<fixture>.json`` freeze the LEGACY
Bash/Python implementation's output for every fixture in ``parity.CORPUS``
(minus ``parity.GOLDEN_EXCLUDED``) in both gate modes. Each entry carries the
fixture name, gate mode, the launcher identity it was captured from
(``git rev-parse HEAD`` as a string) and one entry per step with the
already-normalized argv/rc/stdout/stderr plus the tree snapshot.

Those files are the FIXED legacy side of ``tests/parity/run.py``: the harness
compares them against the live Go binary and never runs a legacy
implementation.

Why this script cannot capture any more
--------------------------------------
Capture used to run ``bin/ai-specs`` as its leg. Since the GO-20 U2e flip
(T2) that launcher only resolves and execs the native binary, and an isolated
capture home has no ``dist/ai-specs``, so a capture today would freeze
"ai-specs: native binary not found" as the legacy golden. There is no legacy
implementation left in this tree to capture from.

Therefore this script REFUSES to write. The corpus is never regenerated in
place: deleting it (or recapturing it against the Go binary) is GO-20 U2e
PR4b's call, together with the normalizations N10/N11 that only exist because
the frozen bytes still carry the legacy shapes (see parity.NORMALIZATIONS).
"""
from __future__ import annotations

import sys

_REFUSAL = (
    "capture: REFUSED — the legacy corpus cannot be captured any more: "
    "bin/ai-specs is the NATIVE launcher since the GO-20 U2e flip and an "
    "isolated capture home has no dist/ai-specs, so a capture would overwrite "
    "the frozen legacy goldens with 'native binary not found'. "
    "tests/parity/goldens/ is the fixed legacy side of the comparison and is "
    "never regenerated in place (GO-20 U2e T5 PR4a S7b).\n"
)


def main() -> int:
    sys.stderr.write(_REFUSAL)
    return 2


if __name__ == "__main__":
    sys.exit(main())
