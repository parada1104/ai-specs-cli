#!/usr/bin/env python3
"""Go->Python strangle bridge for the ai-specs doctor port (card [Go 08]).

Read-only. One op remains interpreter-owned since GO-07 S18: ``toml_error``
renders tomllib's own manifest diagnostic, whose text belongs to the
interpreter version. The brief probes (``brief_state``,
``brief_dead_fragments``) are native in the Go doctor now, so this bridge no
longer imports recipe-materialize.py, agents-render.py or
brief-render-policy.py.

Invoked as: python3 -c "<this file>" <ai-specs-home> <request-json>
"""
from __future__ import annotations

import json
import sys
import tomllib
from pathlib import Path

REQUEST = json.loads(sys.argv[2])
ROOT = Path(REQUEST["root"]).resolve()


def op_toml_error() -> dict:
    """doctor._check_manifest: the guidance renders tomllib's own diagnostic.

    That text belongs to the interpreter, and Python versions differ in it, so
    the port asks the same authority the legacy check did instead of inventing
    a lookalike.
    """
    try:
        with (ROOT / "ai-specs" / "ai-specs.toml").open("rb") as fh:
            tomllib.load(fh)
    except Exception as exc:
        return {"error": f"{type(exc).__name__}: {exc}"}
    return {"error": None}


OPS = {
    "toml_error": op_toml_error,
}


def main() -> int:
    results = {}
    for name in REQUEST.get("ops") or []:
        fn = OPS.get(name)
        if fn is None:
            results[name] = {"error": f"unknown bridge op: {name}"}
            continue
        try:
            results[name] = fn()
        except Exception as exc:  # each op degrades on its own, never the run
            results[name] = {"error": f"{type(exc).__name__}: {exc}"}
    print(json.dumps({"ops": results}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
