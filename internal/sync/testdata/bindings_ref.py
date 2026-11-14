#!/usr/bin/env python3
"""Differential reference driver for the native bindings authority (GO-07 S13 PR1).

Runs the REAL Python fallback authority in ``lib/_internal/recipe-materialize.py``
in a subprocess and emits canonical output so ``internal/sync/bindings_test.go``
can require equality against the Go native authority
(``internal/sync.ResolveBindings`` / the shared witness producer).

Modes:
  resolve <authority> <catalog> <enabled_json> <bindings_json>
      ``_python_resolve_bindings`` + ``_python_check_capability_conflicts``
      -> compact JSON ``{bindings?, conflicts?, error?, conflicts_error?}``
  witness <authority> <catalog> <enabled_json> <resolved_json> <declared:0|1> <written_at>
      ``tracker_witness_payload`` serialized exactly as ``write_tracker_witness``
      does (``json.dumps(indent=2, sort_keys=True)`` plus one trailing newline)
      -> the raw bytes, with no wrapper, so the Go side can compare bytes.
"""
from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path

AUTHORITY = sys.argv[2]
spec = importlib.util.spec_from_file_location("materialize_bindings_oracle", AUTHORITY)
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)


def main() -> int:
    mode = sys.argv[1]
    if mode == "resolve":
        catalog = Path(sys.argv[3])
        enabled = json.loads(sys.argv[4])
        bindings = json.loads(sys.argv[5])
        out = {}
        try:
            out["bindings"] = mod._python_resolve_bindings(catalog, enabled, bindings)
        except Exception as exc:  # noqa: BLE001 - the oracle reports the message
            out["error"] = str(exc)
        try:
            out["conflicts"] = [
                {
                    "type": c.primitive_type,
                    "id": c.primitive_id,
                    "recipes": sorted(c.recipes),
                    "severity": c.severity,
                }
                for c in mod._python_check_capability_conflicts(catalog, enabled, bindings)
            ]
        except Exception as exc:  # noqa: BLE001 - the oracle reports the message
            out["conflicts_error"] = str(exc)
        sys.stdout.write(json.dumps(out, sort_keys=True, separators=(",", ":")))
        return 0
    if mode == "witness":
        catalog = Path(sys.argv[3])
        enabled = json.loads(sys.argv[4])
        resolved = json.loads(sys.argv[5])
        declared = sys.argv[6] == "1"
        written_at = sys.argv[7]
        payload = mod.tracker_witness_payload(
            catalog, enabled, resolved, declared=declared, written_at=written_at
        )
        sys.stdout.write(json.dumps(payload, indent=2, sort_keys=True))
        sys.stdout.write("\n")
        return 0
    print(f"unknown mode {mode!r}", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
