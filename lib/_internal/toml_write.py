#!/usr/bin/env python3
"""Shared TOML value serialization for hand-built manifest writers.

Several internal scripts append sections to TOML manifests by string
concatenation rather than through a full TOML library. They MUST route
scalar/collection values through `toml_value()` so that Python types map to
valid TOML literals — in particular `True`/`False` -> `true`/`false` and
lists/dicts use double-quoted strings.
"""

from __future__ import annotations

import json
from typing import Any


def toml_value(v: Any) -> str:
    """Serialize a Python value to a valid TOML inline literal.

    Supports str, bool, int, float, list, and dict-of-serializable. Raises
    TypeError for anything else so malformed defaults fail loudly at the
    write site instead of producing invalid TOML downstream.
    """
    # bool must precede int: bool is a subclass of int in Python.
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, (int, float)):
        return str(v)
    if isinstance(v, str):
        return json.dumps(v)  # JSON string ≅ TOML basic string
    if isinstance(v, list):
        return "[" + ", ".join(toml_value(x) for x in v) + "]"
    if isinstance(v, dict):
        inner = ", ".join(f"{k} = {toml_value(val)}" for k, val in v.items())
        return "{ " + inner + " }"
    raise TypeError(f"cannot serialize {type(v).__name__} to TOML")
