#!/usr/bin/env python3
"""Read ai-specs.toml and expose sections as JSON.

Usage:
  toml-read.py <toml_path> <section>

Sections:
  project        → JSON object with project.name, project.subrepos
  agents         → JSON array of enabled agents
  deps           → JSON array of [[deps]] entries
  mcp            → JSON object of MCP servers (key = server name)

Output: JSON to stdout. Exit 1 on missing file or unknown section.
"""

import json
import sys
import tomllib
from pathlib import Path


def main() -> int:
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <toml_path> <section>", file=sys.stderr)
        return 2

    toml_path = Path(sys.argv[1])
    section = sys.argv[2]

    if not toml_path.is_file():
        print(f"error: {toml_path} not found", file=sys.stderr)
        return 1

    with toml_path.open("rb") as f:
        data = tomllib.load(f)

    if section == "project":
        print(json.dumps(data.get("project", {})))
    elif section == "agents":
        print(json.dumps(data.get("agents", {}).get("enabled", [])))
    elif section == "deps":
        print(json.dumps(data.get("deps", [])))
    elif section == "mcp":
        print(json.dumps(data.get("mcp", {})))
    else:
        print(f"error: unknown section '{section}'", file=sys.stderr)
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
