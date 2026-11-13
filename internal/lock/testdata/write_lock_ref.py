#!/usr/bin/env python3
"""Differential reference driver for the Go internal/lock port (GO-05).

Runs the pure-Python lock authority in lib/_internal/lock.py DIRECTLY,
bypassing the Go write bridge (go_write_lock) entirely, so the Go test
compares the emission authority against the Go port byte for byte.

Modes:
  write <spec.json> <lock_path>
      Build a lock dict from the JSON spec and call _write_lock_python.
      Prints "OK" on success; on any exception prints "ERROR: <message>"
      and exits 1 (refusal cases compare this message against the Go error).
  roundtrip <src> <dst>
      load_lock(src) then _write_lock_python(dst, lock) — used for the
      LoadLock/WriteLock byte round-trip differential.
"""
from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path

# testdata/ -> internal/lock/ -> internal/ -> repo root
REPO_ROOT = Path(__file__).resolve().parents[3]


def load_lock_module():
    path = REPO_ROOT / "lib" / "_internal" / "lock.py"
    spec = importlib.util.spec_from_file_location("lock_ref", path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def build_lock(spec: dict) -> dict:
    return {
        "skills": spec.get("skills") or {},
        "meta": spec.get("meta") or {},
        "recipes": spec.get("recipes") or {},
        "deps": spec.get("deps") or {},
        "agents": spec.get("agents") or {},
        "managed": spec.get("managed") or {},
    }


def main() -> int:
    mode = sys.argv[1]
    mod = load_lock_module()
    try:
        if mode == "write":
            spec = json.loads(Path(sys.argv[2]).read_text(encoding="utf-8"))
            mod._write_lock_python(Path(sys.argv[3]), build_lock(spec))
        elif mode == "roundtrip":
            lock = mod.load_lock(Path(sys.argv[2]))
            mod._write_lock_python(Path(sys.argv[3]), lock)
        else:
            raise RuntimeError(f"unknown mode {mode!r}")
    except Exception as exc:  # noqa: BLE001 - the message IS the test fixture
        print(f"ERROR: {exc}")
        return 1
    print("OK")
    return 0


if __name__ == "__main__":
    sys.exit(main())
