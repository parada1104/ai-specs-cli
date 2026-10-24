#!/usr/bin/env python3
"""Capture the frozen legacy parity corpus as versioned goldens (GO-20 U2e T2a).

The legacy launcher (this tree's ``bin/ai-specs``) is executed ONLY here, at
capture time. Every capture is run TWICE into two independent scratch roots
and the serialized JSON must be byte-identical for every fixture and gate
mode; a mismatch aborts the write instead of freezing nondeterministic output.

For each fixture in ``parity.CORPUS`` (minus ``parity.GOLDEN_EXCLUDED``) and
each gate mode (gate-absent always; gate-present when the worktree-gate build
succeeds) this writes::

    tests/parity/goldens/<gate-mode>/<fixture-name>.json

with the fixture name, gate mode, the legacy launcher identity (``git rev-parse
HEAD`` of the worktree, as a string) and one entry per step carrying the
already-normalized argv/rc/stdout/stderr plus the tree snapshot.

When the gate binary cannot be built, ``tests/parity/goldens/gate-present/
NOT_CAPTURED.txt`` records that gate-present was not captured; the suite's
default mode already fails loudly in that case.

Usage: python3 tests/parity/capture_goldens.py
"""
from __future__ import annotations

import json
import subprocess
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import parity  # noqa: E402


def _git_head() -> str:
    proc = subprocess.run(
        ["git", "-C", str(parity.ROOT), "rev-parse", "HEAD"],
        text=True, capture_output=True, check=True)
    return proc.stdout.strip()


def _payload(fixture, gate_mode: str, head: str, scratch: Path,
             gate_bin: Path | None) -> dict:
    leg = parity.run_leg("legacy", parity.ROOT / "bin" / "ai-specs", fixture,
                         scratch, gate_bin=gate_bin)
    return {
        "fixture": fixture.name,
        "gate_mode": gate_mode,
        "legacy_launcher": head,
        "steps": [
            {
                "argv": list(step.argv),
                "rc": step.rc,
                "stdout": step.stdout,
                "stderr": step.stderr,
                "tree": parity.tree_to_json(step.tree),
            }
            for step in leg.steps
        ],
    }


def _dumps(payload: dict) -> str:
    return json.dumps(payload, sort_keys=True, indent=1) + "\n"


def main() -> int:
    head = _git_head()
    fixtures = [f for f in parity.CORPUS
                if f.name not in parity.GOLDEN_EXCLUDED]
    with tempfile.TemporaryDirectory(prefix="ai-specs-goldens-") as td:
        root = Path(td)
        gate_bin = parity.build_gate_binary(root / "build")
        modes: list[tuple[str, Path | None]] = [(parity.GATE_ABSENT, None)]
        if gate_bin is not None:
            modes.append((parity.GATE_PRESENT, gate_bin))

        captured: dict[tuple[str, str], str] = {}
        mismatches: list[str] = []
        for run_name in ("a", "b"):
            for gate_mode, gbin in modes:
                for fixture in fixtures:
                    scratch = root / run_name / gate_mode / fixture.name
                    serialized = _dumps(_payload(
                        fixture, gate_mode, head, scratch, gbin))
                    key = (gate_mode, fixture.name)
                    if run_name == "a":
                        captured[key] = serialized
                    elif serialized != captured[key]:
                        mismatches.append(f"{gate_mode}/{fixture.name}")

        if mismatches:
            sys.stderr.write(
                "capture: ABORT — non-deterministic legacy output on two "
                "captures: " + ", ".join(mismatches) + "\n")
            return 1

        parity.GOLDENS_DIR.mkdir(parents=True, exist_ok=True)
        for (gate_mode, name), serialized in captured.items():
            target = parity.golden_path(name, gate_mode)
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(serialized, encoding="utf-8")

        present_dir = parity.GOLDENS_DIR / parity.GATE_PRESENT
        present_dir.mkdir(parents=True, exist_ok=True)
        marker = present_dir / "NOT_CAPTURED.txt"
        if gate_bin is None:
            marker.write_text(
                "gate-present was NOT captured: the worktree-gate binary "
                "could not be built (no go toolchain or the build failed).\n",
                encoding="utf-8")
            sys.stderr.write(
                "capture: gate-present NOT CAPTURED (worktree-gate build "
                "unavailable)\n")
        elif marker.is_file():
            # A prior unavailable capture left a marker; this capture owns the
            # directory now. Overwrite it (never delete) with an explicit note.
            marker.write_text(
                "gate-present captured in this directory; this marker is "
                "superseded.\n", encoding="utf-8")

    total = len(captured)
    print(f"captured: {len(fixtures)} fixtures x {len(modes)} gate mode(s) "
          f"= {total} goldens")
    print(f"determinism: {total} fixture/mode pairs byte-identical on two "
          f"independent captures")
    print(f"legacy launcher: bin/ai-specs @ {head}")
    if parity.GOLDEN_EXCLUDED:
        for name, reason in sorted(parity.GOLDEN_EXCLUDED.items()):
            print(f"excluded: {name} — {reason}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
