#!/usr/bin/env python3
"""Render MCP servers from ai-specs.toml into per-agent config files.

Strategy (chai-inspired):
  - We OWN the MCP key (e.g. "mcpServers"). On write, we replace it entirely.
  - We PRESERVE every other key in the target file.
  - Missing target file → create with just our key.

Usage:
  mcp-render.py <toml_path> <agent> <target_path> <mcp_key> [--dry-run]

Examples:
  mcp-render.py ai-specs/ai-specs.toml claude   .mcp.json          mcpServers
  mcp-render.py ai-specs/ai-specs.toml cursor   .cursor/mcp.json   mcpServers
  mcp-render.py ai-specs/ai-specs.toml opencode opencode.json      mcp
  mcp-render.py ai-specs/ai-specs.toml codex    .codex/config.toml mcp_servers

Format detection: by target file extension (.json vs .toml).
"""

import json
import sys
import tomllib
from pathlib import Path


def load_mcp(toml_path: Path) -> dict:
    with toml_path.open("rb") as f:
        data = tomllib.load(f)
    return data.get("mcp", {})


def merge_into_json(target: Path, mcp_key: str, servers: dict) -> str:
    """Return new JSON content with `servers` placed under `mcp_key`,
    preserving every other top-level key in the existing file."""
    if target.is_file():
        try:
            existing = json.loads(target.read_text())
            if not isinstance(existing, dict):
                existing = {}
        except json.JSONDecodeError:
            existing = {}
    else:
        existing = {}

    existing[mcp_key] = servers
    return json.dumps(existing, indent=2) + "\n"


def merge_into_toml(target: Path, mcp_key: str, servers: dict) -> str:
    """Codex-style: write a [mcp_servers.<name>] table per server.

    This is a minimal TOML writer (we don't depend on a 3rd-party lib).
    Existing content is preserved by reading the file and rewriting only
    the [<mcp_key>.*] section blocks.
    """
    existing_lines: list[str] = []
    if target.is_file():
        existing_lines = target.read_text().splitlines()

    # Strip out previous [<mcp_key>.*] sections (we own that key namespace).
    out_lines: list[str] = []
    skip = False
    section_prefix = f"[{mcp_key}."
    for line in existing_lines:
        stripped = line.strip()
        if stripped.startswith(section_prefix):
            skip = True
            continue
        if skip and stripped.startswith("[") and not stripped.startswith(section_prefix):
            skip = False
        if skip:
            continue
        out_lines.append(line)

    # Trim trailing blanks.
    while out_lines and out_lines[-1].strip() == "":
        out_lines.pop()

    # Append our sections.
    if out_lines:
        out_lines.append("")
    for name, cfg in servers.items():
        out_lines.append(f"[{mcp_key}.{name}]")
        for k, v in cfg.items():
            out_lines.append(f"{k} = {_toml_value(v)}")
        out_lines.append("")

    return "\n".join(out_lines).rstrip() + "\n"


def _toml_value(v) -> str:
    """Minimal TOML serializer for str/int/float/bool/list/dict-of-str."""
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, (int, float)):
        return str(v)
    if isinstance(v, str):
        return json.dumps(v)  # JSON string ≅ TOML basic string
    if isinstance(v, list):
        return "[" + ", ".join(_toml_value(x) for x in v) + "]"
    if isinstance(v, dict):
        inner = ", ".join(f"{k} = {_toml_value(val)}" for k, val in v.items())
        return "{ " + inner + " }"
    raise TypeError(f"cannot serialize {type(v).__name__} to TOML")


def main() -> int:
    args = sys.argv[1:]
    dry_run = False
    if "--dry-run" in args:
        dry_run = True
        args = [a for a in args if a != "--dry-run"]

    if len(args) != 4:
        print(
            "Usage: mcp-render.py <toml_path> <agent> <target_path> <mcp_key> [--dry-run]",
            file=sys.stderr,
        )
        return 2

    toml_path = Path(args[0])
    agent = args[1]
    target_path = Path(args[2])
    mcp_key = args[3]

    if not toml_path.is_file():
        print(f"error: {toml_path} not found", file=sys.stderr)
        return 1

    servers = load_mcp(toml_path)
    if not servers:
        print(f"info: no [mcp.*] entries — skipping {agent}", file=sys.stderr)
        return 0

    if target_path.suffix == ".toml":
        content = merge_into_toml(target_path, mcp_key, servers)
    else:
        content = merge_into_json(target_path, mcp_key, servers)

    if dry_run:
        print(f"--- {target_path} (dry-run) ---")
        print(content)
        return 0

    target_path.parent.mkdir(parents=True, exist_ok=True)
    target_path.write_text(content)
    print(f"  ✓ {agent} → {target_path} ({len(servers)} server(s))")
    return 0


if __name__ == "__main__":
    sys.exit(main())
