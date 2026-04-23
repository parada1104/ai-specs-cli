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

from __future__ import annotations

import json
import sys
import tomllib
from pathlib import Path
from typing import Any


def load_toml(toml_path: str | Path) -> dict[str, Any]:
    path = Path(toml_path)
    if not path.is_file():
        raise FileNotFoundError(f"{path} not found")
    with path.open("rb") as f:
        return tomllib.load(f)


def _normalize_subrepos(raw: Any) -> list[str]:
    if not isinstance(raw, list):
        return []

    out: list[str] = []
    for item in raw:
        if not isinstance(item, str):
            continue
        value = item.strip()
        if value:
            out.append(value)
    return out


def read_project(data: dict[str, Any]) -> dict[str, Any]:
    project = data.get("project", {}) or {}
    if not isinstance(project, dict):
        project = {}
    return {
        **project,
        "name": str(project.get("name") or ""),
        "subrepos": _normalize_subrepos(project.get("subrepos", [])),
    }


def read_section(data: dict[str, Any], section: str) -> Any:
    if section == "project":
        return read_project(data)
    if section == "agents":
        return data.get("agents", {}).get("enabled", [])
    if section == "deps":
        return data.get("deps", [])
    if section == "mcp":
        return data.get("mcp", {})
    raise KeyError(section)


def main() -> int:
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <toml_path> <section>", file=sys.stderr)
        return 2

    toml_path = Path(sys.argv[1])
    section = sys.argv[2]

    try:
        data = load_toml(toml_path)
        payload = read_section(data, section)
    except FileNotFoundError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    except KeyError:
        print(f"error: unknown section '{section}'", file=sys.stderr)
        return 1

    print(json.dumps(payload))
    return 0


if __name__ == "__main__":
    sys.exit(main())
