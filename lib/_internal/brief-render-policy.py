#!/usr/bin/env python3
"""Read [brief].render from ai-specs.toml — managed AGENTS.md opt-out policy.

Usage:
  brief-render-policy.py <toml_path>           → prints true/false, exit 0.
    Non-boolean render values are treated as the default (enabled); this is
    fail-safe so a typo never silently drops the brief for bash callers.
  brief-render-policy.py <toml_path> --validate  → exit 1 on invalid render type
"""

from __future__ import annotations

import argparse
import sys
import tomllib
from pathlib import Path
from typing import Any

PLACEHOLDER_LINE = "# AGENTS.md - Runtime context"


def brief_render_enabled(manifest: dict[str, Any]) -> bool:
    """Return False only when [brief].render is explicitly false."""
    brief = manifest.get("brief")
    if not isinstance(brief, dict):
        return True
    render = brief.get("render", True)
    if render is False:
        return False
    if render is True:
        return True
    raise ValueError(
        "[brief].render must be a boolean (true or false); "
        f"got {type(render).__name__}"
    )


def load_brief_render_enabled(toml_path: Path) -> bool:
    with toml_path.open("rb") as fh:
        data = tomllib.load(fh)
    return brief_render_enabled(data)


def has_dead_recipe_fragments(resolved: dict[str, Any]) -> bool:
    """True if any enabled recipe has non-empty brief_fragments."""
    for rid in resolved.get("enabled", []):
        rcfg = resolved.get("recipes", {}).get(rid, {}) or {}
        frags = rcfg.get("brief_fragments") or {}
        if not isinstance(frags, dict):
            continue
        for section_fragments in frags.values():
            if section_fragments:
                return True
    return False


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("toml_path", type=Path, help="Path to ai-specs.toml")
    parser.add_argument(
        "--validate",
        action="store_true",
        help="Exit 1 when [brief].render is not a boolean",
    )
    args = parser.parse_args()

    try:
        enabled = load_brief_render_enabled(args.toml_path)
    except FileNotFoundError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    except tomllib.TOMLDecodeError as exc:
        # Must be caught before ValueError because TOMLDecodeError inherits
        # from ValueError in Python's tomllib implementation.
        print(f"error: {exc}", file=sys.stderr)
        return 1
    except ValueError as exc:
        if args.validate:
            print(f"error: {exc}", file=sys.stderr)
            return 1
        # Non-validate mode: treat an invalid render value as the default
        # (render enabled). This is fail-safe — a typo must not silently
        # suppress AGENTS.md for bash callers. The strict gate lives in
        # --validate (and doctor reports it).
        enabled = True

    if args.validate:
        # load_brief_render_enabled already validated boolean when present
        pass

    print("true" if enabled else "false")
    return 0


if __name__ == "__main__":
    sys.exit(main())
