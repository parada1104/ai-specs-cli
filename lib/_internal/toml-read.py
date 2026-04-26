#!/usr/bin/env python3
"""Read ai-specs.toml and expose sections as JSON.

Usage:
  toml-read.py <toml_path> <section>

Sections:
  project        → JSON object with project.name, project.subrepos
  agents         → JSON object with agents.enabled
  deps           → JSON array of normalized [[deps]] entries
  mcp            → JSON object of normalized MCP servers (key = server name)

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


def _normalize_string_list(raw: Any) -> list[str]:
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


def _normalize_mapping(raw: Any) -> dict[str, Any]:
    return dict(raw) if isinstance(raw, dict) else {}


def read_project(data: dict[str, Any]) -> dict[str, Any]:
    project = data.get("project", {}) or {}
    if not isinstance(project, dict):
        project = {}
    return {
        "name": str(project.get("name") or ""),
        "subrepos": _normalize_subrepos(project.get("subrepos", [])),
    }


def read_agents(data: dict[str, Any]) -> dict[str, list[str]]:
    agents = data.get("agents", {}) or {}
    if not isinstance(agents, dict):
        agents = {}
    return {"enabled": _normalize_string_list(agents.get("enabled", []))}


def read_deps(data: dict[str, Any]) -> list[dict[str, Any]]:
    raw_deps = data.get("deps", []) or []
    if not isinstance(raw_deps, list):
        return []
    return [_normalize_mapping(dep) for dep in raw_deps if isinstance(dep, dict)]


def _normalize_mcp_server(raw: Any) -> dict[str, Any]:
    server = _normalize_mapping(raw)
    normalized: dict[str, Any] = {}

    command = server.get("command")
    if isinstance(command, (str, list)) or command is None:
        normalized["command"] = command
    else:
        normalized["command"] = None

    normalized["args"] = server.get("args") if isinstance(server.get("args"), list) else []

    env = server.get("env")
    if not isinstance(env, dict):
        env = server.get("environment")
    normalized["env"] = dict(env) if isinstance(env, dict) else {}

    timeout = server.get("timeout")
    normalized["timeout"] = timeout if isinstance(timeout, int) and not isinstance(timeout, bool) else None

    enabled = server.get("enabled")
    if isinstance(enabled, bool):
        normalized["enabled"] = enabled

    for key, value in server.items():
        if key in {"command", "args", "env", "environment", "timeout", "enabled"}:
            continue
        normalized[key] = value

    return normalized


def read_mcp(data: dict[str, Any]) -> dict[str, dict[str, Any]]:
    raw_mcp = data.get("mcp", {}) or {}
    if not isinstance(raw_mcp, dict):
        return {}
    return {name: _normalize_mcp_server(cfg) for name, cfg in raw_mcp.items() if isinstance(name, str)}


def read_recipes(data: dict[str, Any]) -> dict[str, dict[str, Any]]:
    """Extract [recipes.<id>] tables. Returns {id: {enabled, version}}."""
    out: dict[str, dict[str, Any]] = {}
    recipes = data.get("recipes", {})
    if not isinstance(recipes, dict):
        return out
    for recipe_id, value in recipes.items():
        if not isinstance(value, dict):
            continue
        enabled = value.get("enabled")
        version = value.get("version")
        out[recipe_id] = {
            "enabled": bool(enabled) if isinstance(enabled, bool) else False,
            "version": str(version) if version is not None else "",
        }
    return out


def read_section(data: dict[str, Any], section: str) -> Any:
    if section == "project":
        return read_project(data)
    if section == "agents":
        return read_agents(data)
    if section == "deps":
        return read_deps(data)
    if section == "mcp":
        return read_mcp(data)
    if section == "recipes":
        return read_recipes(data)
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
