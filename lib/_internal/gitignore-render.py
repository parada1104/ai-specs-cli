#!/usr/bin/env python3
"""Render <project>/ai-specs/.gitignore from ai-specs.toml.

Ignored (CLI-owned / regenerable materialization):
  - .internal/            transient internal state
  - .deps/                toml-dep materialization (regenerable from git source)
  - recipes/**            recipe docs/hooks/templates are CLI-owned

Committed (project governance):
  - recipes/*/overrides/  declared per-recipe override surface

Usage:
  gitignore-render.py <toml_path> <output_path>
"""

import sys
import tomllib
from pathlib import Path


HEADER = """\
# --- ai-specs: managed by `ai-specs init`/`sync` — do not edit ---
"""

FOOTER = """\
# --- end ai-specs ---
"""


def render(deps: list[dict]) -> str:
    lines = [HEADER]
    lines.append(".internal/")
    lines.append(".deps/")
    lines.append("")
    lines.append("# Recipe materialization is CLI-owned; only declared overrides are committed.")
    lines.append("recipes/**")
    lines.append("!recipes/*/")
    lines.append("!recipes/*/overrides/")
    lines.append("!recipes/*/overrides/**")
    lines.append("")
    lines.append(FOOTER)
    return "\n".join(lines)


def main() -> int:
    if len(sys.argv) != 3:
        print("Usage: gitignore-render.py <toml_path> <output_path>", file=sys.stderr)
        return 2

    toml_path = Path(sys.argv[1])
    output_path = Path(sys.argv[2])

    with toml_path.open("rb") as f:
        data = tomllib.load(f)

    deps = data.get("deps", []) or []
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(render(deps))
    print(f"  ✓ wrote {output_path} ({len(deps)} dep(s))")
    return 0


if __name__ == "__main__":
    sys.exit(main())
