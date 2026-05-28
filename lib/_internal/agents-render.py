#!/usr/bin/env python3
"""Generate or update AGENTS.md from the ai-specs manifest.

Usage:
    agents-render.py <toml_path> <output_path> [--preserve-if-runtime-brief]

Rules:
  - With --preserve-if-runtime-brief: if output_path exists and contains
    '<!-- ai-specs:runtime-brief -->', leave it untouched (user-managed brief).
  - Otherwise: write a generated AGENTS.md with project name and MCP summary
    (literal env values are redacted; $VAR references are normalised to ${VAR}).
"""

from __future__ import annotations

import argparse
import sys
import tomllib
from pathlib import Path

RUNTIME_BRIEF_MARKER = "<!-- ai-specs:runtime-brief -->"


def _redact_env_value(value: str) -> str:
    """Return a safe representation of an MCP env value.

    $VAR or ${VAR}  →  ${VAR}
    literal secret  →  ***
    """
    stripped = str(value).strip()
    if stripped.startswith("$"):
        var = stripped[1:].strip("{}")
        return f"${{{var}}}"
    return "***"


def _render_lines(manifest: dict) -> list[str]:
    project = manifest.get("project", {}) or {}
    name = project.get("name", "")
    mcp: dict = manifest.get("mcp", {}) or {}

    lines: list[str] = ["# AGENTS.md - Runtime context", ""]

    if name:
        lines += [f"## Project: {name}", ""]

    if mcp:
        lines += ["## How AI tooling is wired", "", "### MCP Servers", ""]
        for server_name, cfg in mcp.items():
            cfg = cfg or {}
            lines.append(f"**{server_name}**")
            if "command" in cfg:
                lines.append(f"- command: {cfg['command']}")
            args = cfg.get("args", []) or []
            if args:
                lines.append(f"- args: {' '.join(str(a) for a in args)}")
            env = cfg.get("env") or cfg.get("environment") or {}
            if env:
                lines.append("- env:")
                if isinstance(env, list):
                    for var in env:
                        lines.append(f"  - {var}: ${{{var}}}")
                else:
                    for k, v in env.items():
                        lines.append(f"  - {k}: {_redact_env_value(str(v))}")
            lines.append("")

    return lines


def render(toml_path: Path, output_path: Path, *, preserve_if_marker: bool) -> None:
    if preserve_if_marker and output_path.exists():
        if RUNTIME_BRIEF_MARKER in output_path.read_text():
            return  # user-managed runtime brief — leave alone

    with open(toml_path, "rb") as fh:
        manifest = tomllib.load(fh)

    content = "\n".join(_render_lines(manifest))
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(content)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("toml_path", help="Path to ai-specs.toml")
    parser.add_argument("output_path", help="Path to write AGENTS.md")
    parser.add_argument(
        "--preserve-if-runtime-brief",
        action="store_true",
        help="Leave output_path unchanged if it contains the runtime-brief marker",
    )
    args = parser.parse_args()

    render(
        Path(args.toml_path),
        Path(args.output_path),
        preserve_if_marker=args.preserve_if_runtime_brief,
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
