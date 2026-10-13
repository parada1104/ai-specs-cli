#!/usr/bin/env python3
"""Differential reference driver for the native provider resolver (GO-07 S13 PR4b-1).

Imports the REAL authority ``lib/_internal/provider_install.py`` and runs
``resolve_provider`` / ``_run_version`` in a subprocess so
``internal/provider/provider_test.go`` can compare the native ``internal/provider``
port field for field (and error string for error string).

Modes:
  resolve <authority> <binary> <min_version> <home|->
      ``resolve_provider`` -> compact JSON of every ProviderResolution field, or
      ``{"error": str(exc)}`` when the authority raises (only the winner
      re-check does).
  runversion <authority> <path> [timeout]
      ``_run_version`` -> ``{"version": ...}`` or ``{"error": str(exc)}``.
  candidates <authority> <root> <goos> <goarch>
      ``_managed_candidates`` -> ``{"candidates": [str(path), ...]}`` in order.
"""
from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path

AUTHORITY = sys.argv[2]
spec = importlib.util.spec_from_file_location("provider_install_oracle", AUTHORITY)
assert spec is not None and spec.loader is not None
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)


class Dep:
    """The duck-typed dep resolve_provider reads (binary, min_version)."""

    def __init__(self, binary: str, min_version: str) -> None:
        self.binary = binary
        self.min_version = min_version


def dump(payload: dict) -> int:
    sys.stdout.write(json.dumps(payload, sort_keys=True, separators=(",", ":")))
    return 0


def main() -> int:
    mode = sys.argv[1]
    if mode == "resolve":
        dep = Dep(sys.argv[3], sys.argv[4])
        home = None if sys.argv[5] == "-" else Path(sys.argv[5])
        try:
            r = mod.resolve_provider(dep, ai_specs_home=home)
        except Exception as exc:  # noqa: BLE001 - the oracle reports the message
            return dump({"error": str(exc)})
        return dump(
            {
                "binary": r.binary,
                "command": r.command,
                "version": r.version,
                "source": r.source,
                "path": str(r.path) if r.path is not None else None,
                "repository": r.repository,
                "release_tag": r.release_tag,
                "target": list(r.target),
                "verified": r.verified,
            }
        )
    if mode == "runversion":
        timeout = float(sys.argv[4]) if len(sys.argv) > 4 else 5.0
        try:
            version = mod._run_version(Path(sys.argv[3]), timeout=timeout)
        except Exception as exc:  # noqa: BLE001 - the oracle reports the message
            return dump({"error": str(exc)})
        return dump({"version": version})
    if mode == "candidates":
        paths = [str(p) for p in mod._managed_candidates(Path(sys.argv[3]), sys.argv[4], sys.argv[5])]
        return dump({"candidates": paths})
    print(f"unknown mode {mode!r}", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
