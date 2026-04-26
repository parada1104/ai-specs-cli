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
import re
import sys
import importlib.util
from pathlib import Path


def load_mcp(toml_path: Path) -> dict:
    module_path = Path(__file__).with_name("toml-read.py")
    spec = importlib.util.spec_from_file_location("toml_read_internal", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load helper module at {module_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    data = module.load_toml(toml_path)
    mcp = module.read_mcp(data)

    # Merge recipe MCP presets if present (recipe values take precedence)
    recipe_mcp_path = toml_path.parent / ".recipe-mcp.json"
    if recipe_mcp_path.is_file():
        try:
            recipe_mcp = json.loads(recipe_mcp_path.read_text())
            if isinstance(recipe_mcp, dict):
                mcp = {**mcp, **recipe_mcp}
        except json.JSONDecodeError:
            pass
    return mcp


# --- Per-agent translators -------------------------------------------------
# Most agents use the generic Claude/Cursor format (command:string, args:array,
# env:dict, $VAR env vars). Agents with native non-generic schemas register a
# translator here.

_ENV_VAR_RE = re.compile(r"^\$([A-Z_][A-Z0-9_]*)$")
# Cursor/Claude JSON use "${env:NAME}" in headers/url; OpenCode remote expects "{env:NAME}".
_CURSOR_ENV_IN_HEADERS = re.compile(r"\$\{env:([A-Za-z_][A-Za-z0-9_]*)\}")


def _headers_for_opencode(headers: dict) -> dict:
    if not isinstance(headers, dict):
        return headers
    out: dict = {}
    for k, v in headers.items():
        if isinstance(v, str):
            out[k] = _CURSOR_ENV_IN_HEADERS.sub(r"{env:\1}", v)
        else:
            out[k] = v
    return out


def _translate_opencode(servers: dict) -> dict:
    """OpenCode native schema:
      - Local: type: "local", command: [cmd, *args], environment: {...}
      - Remote: type: "remote", url: "https://..." (manifest HTTP MCP uses type: "http")
    """
    out = {}
    for name, cfg in servers.items():
        mcp_type = cfg.get("type")
        url = cfg.get("url")
        if mcp_type in ("http", "remote", "sse") and isinstance(url, str) and url:
            new: dict = {"type": "remote", "url": url}
            for passthrough in ("timeout", "enabled", "oauth"):
                if passthrough in cfg:
                    new[passthrough] = cfg[passthrough]
            if "headers" in cfg:
                new["headers"] = _headers_for_opencode(cfg["headers"])
            out[name] = new
            continue

        cmd = cfg.get("command")
        args = cfg.get("args", []) or []
        if isinstance(cmd, str):
            full_cmd = [cmd, *args]
        elif isinstance(cmd, list):
            full_cmd = list(cmd)
        else:
            full_cmd = []

        new = {"type": "local", "command": full_cmd}

        env = cfg.get("env") or {}
        if env:
            new["environment"] = {
                k: _ENV_VAR_RE.sub(r"{env:\1}", v) if isinstance(v, str) else v
                for k, v in env.items()
            }

        for passthrough in ("timeout", "enabled"):
            if passthrough in cfg:
                new[passthrough] = cfg[passthrough]

        out[name] = new
    return out


def _slim_mcp_config_for_write(cfg: dict) -> dict:
    """Omit null command/args for HTTP MCP; otherwise drop only null values."""
    mcp_type = cfg.get("type")
    url = cfg.get("url")
    if mcp_type in ("http", "remote", "sse") and isinstance(url, str) and url:
        slim: dict = {"type": mcp_type, "url": url}
        for k in ("timeout", "enabled", "headers"):
            if k in cfg and cfg[k] is not None:
                slim[k] = cfg[k]
        return slim
    return {k: v for k, v in cfg.items() if v is not None}


_TRANSLATORS = {
    "opencode": _translate_opencode,
}


def translate_servers(agent: str, servers: dict) -> dict:
    fn = _TRANSLATORS.get(agent)
    return fn(servers) if fn else servers


def merge_into_json(target: Path, mcp_key: str, servers: dict, agent: str) -> str:
    """Return new JSON content with `servers` placed under `mcp_key`,
    preserving every other top-level key in the existing file.

    For agent ``opencode``, ensure ``$schema`` is the first top-level key so
    OpenCode recognizes the config file.
    """
    if target.is_file():
        try:
            existing = json.loads(target.read_text())
            if not isinstance(existing, dict):
                existing = {}
        except json.JSONDecodeError:
            existing = {}
    else:
        existing = {}

    if agent == "opencode":
        schema = existing.get("$schema", "https://opencode.ai/config.json")
        existing = {
            "$schema": schema,
            **{k: v for k, v in existing.items() if k != "$schema"},
        }

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

    servers = translate_servers(agent, servers)
    servers = {n: _slim_mcp_config_for_write(c) for n, c in servers.items()}

    if target_path.suffix == ".toml":
        content = merge_into_toml(target_path, mcp_key, servers)
    else:
        content = merge_into_json(target_path, mcp_key, servers, agent)

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
