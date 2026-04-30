#!/usr/bin/env python3
"""Render AGENTS.md runtime brief from a root manifest.

Usage:
  agents-md-render.py <manifest_root> <output_path> [--skills-dir <path>]

The optional --skills-dir flag is accepted for backward compatibility but
ignored; the runtime brief no longer contains a skills catalog.
"""

from __future__ import annotations

import re
import sys
import tomllib
from pathlib import Path

RUNTIME_BRIEF_MARKER = "<!-- ai-specs:runtime-brief -->"
GENERATED_MARKER = "<!-- ai-specs:generated-runtime-brief -->"

ENV_REFERENCE_RE = re.compile(
    r"^\$(?:\{env:)?([A-Za-z_][A-Za-z0-9_]*)\}?$"
)

SECRET_KEY_RE = re.compile(
    r"(key|token|secret|password|api|auth)", re.IGNORECASE
)


def parse_args(argv: list[str]) -> tuple[Path, Path, Path]:
    if len(argv) < 3:
        raise ValueError(
            "Usage: agents-md-render.py <manifest_root> <output_path> [--skills-dir <path>]"
        )

    manifest_root = Path(argv[1])
    output_path = Path(argv[2])
    skills_dir = manifest_root / "ai-specs" / "skills"

    i = 3
    while i < len(argv):
        if argv[i] == "--skills-dir":
            skills_dir = Path(argv[i + 1])
            i += 2
            continue
        raise ValueError(f"unknown argument: {argv[i]}")

    return manifest_root, output_path, skills_dir


def redact_mcp_value(key: str, value: str) -> str:
    """Redact an MCP env or header value.

    Env variable references ($VAR, ${VAR}, {env:VAR}) are shown in a
    normalised form.  All other literal values are replaced with ***.
    """
    match = ENV_REFERENCE_RE.match(value.strip())
    if match:
        return f"${{{match.group(1)}}}"
    return "***"


def render_mcp_section(mcp_data: dict) -> str:
    if not mcp_data:
        return ""

    lines = ["## Runtime MCPs", ""]
    for mcp_name in sorted(mcp_data.keys()):
        config = mcp_data[mcp_name]
        parts: list[str] = []

        if config.get("type") == "http":
            url = config.get("url", "")
            parts.append(f"url `{url}`")
        elif config.get("command"):
            cmd = config["command"]
            args = config.get("args", [])
            if isinstance(args, list):
                cmd_str = " ".join([cmd] + args)
            else:
                cmd_str = str(cmd)
            parts.append(f"command `{cmd_str}`")

        env = config.get("env") or config.get("environment")
        if isinstance(env, list):
            env = {k: f"${k}" for k in env}
        if env and isinstance(env, dict):
            redacted = []
            for k, v in sorted(env.items()):
                redacted.append(f"{k}: {redact_mcp_value(k, str(v))}")
            parts.append("env: { " + "; ".join(redacted) + " }")

        headers = config.get("headers")
        if headers and isinstance(headers, dict):
            redacted = []
            for k, v in sorted(headers.items()):
                redacted.append(f"{k}: {redact_mcp_value(k, str(v))}")
            parts.append("headers: { " + "; ".join(redacted) + " }")

        lines.append(f"- `{mcp_name}`: " + " ".join(parts))

    lines.append("")
    return "\n".join(lines)


def render_project_section(project: dict, agents: dict) -> str:
    name = project.get("name") or "Project"
    description = project.get("description", "")
    enabled = agents.get("enabled", [])
    integration_branch = project.get("integration_branch", "")

    lines = ["## Project", ""]
    lines.append(f"- Project: `{name}`")
    lines.append("- Manifest: `ai-specs/ai-specs.toml`")
    if description:
        lines.append(f"- Purpose: {description}")
    if enabled:
        lines.append(
            f"- Enabled runtimes: {', '.join(f'`{a}`' for a in enabled)}"
        )
    if integration_branch:
        lines.append(f"- Integration branch: `{integration_branch}`")
    lines.append("")
    return "\n".join(lines)


def render_recipes_section(recipes: dict) -> str:
    if not recipes:
        return ""

    lines = ["## Active Recipes / Bindings / Capabilities", ""]
    for recipe_id in sorted(recipes.keys()):
        config = recipes[recipe_id]
        if config.get("enabled"):
            lines.append(f"- `{recipe_id}`: enabled")
    lines.append("")
    return "\n".join(lines)


def render_optional_section(data: dict, title: str, key: str) -> str:
    value = data.get(key)
    if not value:
        return ""

    lines = [f"## {title}", ""]
    if isinstance(value, list):
        for item in value:
            lines.append(f"- {item}")
    elif isinstance(value, str):
        lines.append(value)
    elif isinstance(value, dict):
        for k, v in sorted(value.items()):
            lines.append(f"- {k}: {v}")
    lines.append("")
    return "\n".join(lines)


def main() -> int:
    try:
        manifest_root, output_path, _skills_dir = parse_args(sys.argv)
    except (ValueError, IndexError) as exc:
        print(str(exc), file=sys.stderr)
        return 2

    # Respect manual runtime-brief marker
    if output_path.is_file():
        content = output_path.read_text()
        if RUNTIME_BRIEF_MARKER in content:
            print(f"Skipping {output_path} — runtime-brief marker detected")
            return 0

    toml_path = manifest_root / "ai-specs" / "ai-specs.toml"
    if not toml_path.is_file():
        print(f"error: {toml_path} not found", file=sys.stderr)
        return 1

    with toml_path.open("rb") as f:
        data = tomllib.load(f)

    project = data.get("project", {}) or {}
    agents = data.get("agents", {}) or {}
    mcp = data.get("mcp", {}) or {}
    recipes = data.get("recipes", {}) or {}

    name = project.get("name") or manifest_root.name

    parts = [
        f"# {name} — Agent Instructions",
        "",
        GENERATED_MARKER,
        "",
        "> **Auto-generated by `ai-specs sync`.** Source of truth: [`ai-specs/`](ai-specs/).",
        "",
        render_project_section(project, agents),
        render_mcp_section(mcp),
        render_recipes_section(recipes),
        render_optional_section(data, "Safety Rules", "safety"),
        render_optional_section(data, "Context Sources", "context_sources"),
        render_optional_section(data, "Conflict Policy", "conflict_policy"),
        render_optional_section(data, "Workflow Rules", "workflow"),
        render_optional_section(data, "Useful Commands", "useful_commands"),
    ]

    output_path.parent.mkdir(parents=True, exist_ok=True)
    text = "\n".join(part for part in parts if part).rstrip() + "\n"
    output_path.write_text(text)
    print(f"  ✓ wrote {output_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
