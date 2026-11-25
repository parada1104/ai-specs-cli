#!/usr/bin/env python3
"""Compare (or regenerate) the plan-build behavior goldens.

    python3 tests/behavior/run.py            # compare every case
    python3 tests/behavior/run.py --update   # regenerate the goldens on purpose
    python3 tests/behavior/run.py <case>...  # one or more cases

Exit codes: 0 = every case matches, 1 = at least one delta, 2 = the environment
could not run (no Go toolchain, so the binary under test is unavailable).
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))

from tests._go_cli import LEGACY_CLI, cli  # noqa: E402
from tests.behavior.cases import CASES  # noqa: E402
from tests.behavior.harness import Sandbox  # noqa: E402

GOLDENS = Path(__file__).resolve().parent / "goldens"


def run_case(name: str, factory, mode: str) -> dict:
    sandbox = Sandbox(name, mode=mode)
    try:
        extra = factory(sandbox) or {}
        return sandbox.snapshot(files=extra.get("files"))
    finally:
        sandbox.cleanup()


def diff(name: str, expected: dict, actual: dict) -> list[str]:
    """Report the first difference per evidence layer, not a whole JSON dump."""
    problems: list[str] = []
    exp_steps, act_steps = expected.get("steps", []), actual.get("steps", [])
    for index, exp in enumerate(exp_steps):
        act = act_steps[index] if index < len(act_steps) else None
        if act is None:
            problems.append(f"{name}: step {index} ({exp['label']}) did not run")
            continue
        for field in ("exit", "stdout", "stderr"):
            if exp.get(field) != act.get(field):
                problems.append(f"{name}: step {index} ({exp['label']}) {field} differs")
    if len(act_steps) > len(exp_steps):
        problems.append(f"{name}: {len(act_steps) - len(exp_steps)} unexpected extra step(s)")
    exp_tree, act_tree = expected.get("tree", {}), actual.get("tree", {})
    for path in sorted(set(exp_tree) | set(act_tree)):
        if exp_tree.get(path) != act_tree.get(path):
            problems.append(f"{name}: tree entry {path} differs")
    exp_files, act_files = expected.get("files", {}), actual.get("files", {})
    for path in sorted(set(exp_files) | set(act_files)):
        if exp_files.get(path) != act_files.get(path):
            problems.append(f"{name}: content of {path} differs")
    return problems


def main(argv: list[str]) -> int:
    update = "--update" in argv
    selected = [a for a in argv if not a.startswith("-")]
    if cli() == LEGACY_CLI:
        print("behavior goldens: the Go binary is unavailable; nothing was measured", file=sys.stderr)
        return 2

    GOLDENS.mkdir(exist_ok=True)
    failures: list[str] = []
    for name, factory, mode in CASES:
        if selected and name not in selected:
            continue
        actual = run_case(name, factory, mode)
        path = GOLDENS / f"{name}.json"
        if update:
            path.write_text(json.dumps(actual, indent=1, sort_keys=True) + "\n", encoding="utf-8")
            print(f"updated {path.relative_to(ROOT)}")
            continue
        if not path.exists():
            failures.append(f"{name}: golden missing at {path.relative_to(ROOT)} (run --update)")
            continue
        expected = json.loads(path.read_text(encoding="utf-8"))
        problems = diff(name, expected, actual)
        if problems:
            failures.extend(problems)
            print(f"FAIL {name}")
        else:
            print(f"ok   {name}")

    if failures:
        print(f"\nbehavior goldens: {len(failures)} delta(s)", file=sys.stderr)
        for problem in failures:
            print(f"  {problem}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
