#!/usr/bin/env python3
"""Build an agent-readable initialization brief for a recipe.

Usage:
  recipe-init.py <project_root> <recipe_id>

The command is intentionally read-only. It inspects the manifest, recipe
metadata, MCP declarations, and template targets, then prints reviewable setup
guidance for an agent or human.
"""

from __future__ import annotations

import importlib.util
import os
import re
import sys
from pathlib import Path
from typing import Any


class RecipeInitError(Exception):
    pass


def _load_module(filename: str, module_name: str) -> Any:
    module_path = Path(__file__).with_name(filename)
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load helper module at {module_path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def _load_toml_read() -> Any:
    return _load_module("toml-read.py", "toml_read_internal")


def _load_recipe_read() -> Any:
    return _load_module("recipe-read.py", "recipe_read_internal")


def _resolve_catalog_dir(project_root: Path) -> Path:
    local_catalog = project_root / "catalog" / "recipes"
    if local_catalog.is_dir():
        return local_catalog

    ai_specs_home = os.environ.get("AI_SPECS_HOME")
    if ai_specs_home:
        home_catalog = Path(ai_specs_home) / "catalog" / "recipes"
        if home_catalog.is_dir():
            return home_catalog

    return Path(__file__).resolve().parents[2] / "catalog" / "recipes"


ENV_REFERENCE_RE = re.compile(r"^\$(?:\{env:)?([A-Za-z_][A-Za-z0-9_]*)\}?$")
SECRET_KEY_RE = re.compile(r"(key|token|secret|password|api|auth)", re.IGNORECASE)


def _redact_value(key: str, value: Any) -> Any:
    if isinstance(value, dict):
        return {k: _redact_value(k, v) for k, v in value.items()}
    if isinstance(value, list):
        return [_redact_value(key, item) for item in value]
    if isinstance(value, str):
        match = ENV_REFERENCE_RE.match(value.strip())
        if match:
            return f"${{{match.group(1)}}}"
        if SECRET_KEY_RE.search(key):
            return "***"
        return value
    return value


def _format_mapping(data: dict[str, Any], indent: str = "  ") -> list[str]:
    lines: list[str] = []
    for key in sorted(data):
        value = data[key]
        if isinstance(value, dict):
            lines.append(f"{indent}{key}:")
            lines.extend(_format_mapping(value, indent + "  "))
        else:
            lines.append(f"{indent}{key}: {value}")
    return lines


def _load_manifest(project_root: Path) -> tuple[Any, dict[str, Any]]:
    manifest_path = project_root / "ai-specs" / "ai-specs.toml"
    if not manifest_path.is_file():
        raise RecipeInitError("Proyecto no inicializado. Ejecuta: ai-specs init")
    toml_read = _load_toml_read()
    return toml_read, toml_read.load_toml(manifest_path)


def _load_recipe(project_root: Path, recipe_id: str) -> tuple[Any, Any, Path]:
    catalog_dir = _resolve_catalog_dir(project_root)
    recipe_dir = catalog_dir / recipe_id
    if not recipe_dir.is_dir():
        raise RecipeInitError(f"Recipe '{recipe_id}' no encontrada en catalog/recipes/")
    recipe_read = _load_recipe_read()
    try:
        recipe = recipe_read.read_recipe(catalog_dir, recipe_id)
    except Exception as exc:
        raise RecipeInitError(str(exc)) from exc
    if recipe.init is None:
        raise RecipeInitError(f"Recipe '{recipe_id}' has no init workflow")
    return recipe_read, recipe, recipe_dir


def _install_state(recipe_id: str, manifest_recipes: dict[str, dict[str, Any]]) -> str:
    if recipe_id not in manifest_recipes:
        return "available (not installed)"
    if manifest_recipes[recipe_id].get("enabled"):
        return "installed"
    return "disabled"


def _config_lines(recipe: Any, recipe_id: str, manifest_recipes: dict[str, dict[str, Any]]) -> list[str]:
    config = manifest_recipes.get(recipe_id, {}).get("config", {})
    fields = recipe.config_schema.fields
    existing = sorted(config.keys())
    lines = ["## Config Guidance", ""]
    lines.append("Existing config keys: " + (", ".join(existing) if existing else "(none)"))

    if recipe_id not in manifest_recipes:
        lines.append("Reviewable manifest addition:")
        lines.append(f"```toml\n[recipes.{recipe_id}]\nenabled = true\nversion = \"{recipe.version}\"\n```")

    if fields:
        lines.append("Schema-aligned config targets:")
        for key in sorted(fields):
            field = fields[key]
            if key in config:
                lines.append(f"- Update existing key `{key}` if needed; do not append a duplicate `{key}` key.")
            elif field.required:
                lines.append(f"- Add required `{key}` under `[recipes.{recipe_id}.config]`; sync still validates it later.")
            elif field.default is not None:
                lines.append(f"- `{key}` is optional and defaults to `{field.default}`.")
            else:
                lines.append(f"- `{key}` is optional.")
    else:
        lines.append("Recipe declares no config schema.")

    unknown = sorted(k for k in config if k not in fields)
    if unknown:
        lines.append("Unknown config keys: " + ", ".join(unknown) + "; sync still validates recipe config later.")

    return lines + [""]


def _mcp_lines(recipe: Any, manifest_mcp: dict[str, dict[str, Any]]) -> list[str]:
    recipe_presets = {m.id: dict(m.config) for m in recipe.mcp}
    needed = list(dict.fromkeys([*recipe.init.needs_mcp, *recipe_presets.keys()]))
    lines = ["## MCP Discovery", ""]
    lines.append("Reminder: project manifest values take precedence during sync-time MCP merge.")
    if not needed:
        lines.append("No MCP servers declared by this init workflow or recipe presets.")
        return lines + [""]

    for mcp_id in needed:
        configured = mcp_id in manifest_mcp
        preset = mcp_id in recipe_presets
        status = "configured" if configured else "missing"
        if preset:
            status += "; recipe preset available"
        lines.append(f"- {mcp_id}: {status}")
        if configured:
            redacted = _redact_value(mcp_id, manifest_mcp[mcp_id])
            lines.append("  project manifest:")
            lines.extend(_format_mapping(redacted, "    "))
        if preset:
            redacted = _redact_value(mcp_id, recipe_presets[mcp_id])
            lines.append("  recipe preset:")
            lines.extend(_format_mapping(redacted, "    "))
    return lines + [""]


def _template_lines(recipe: Any, project_root: Path) -> list[str]:
    lines = ["## Template/Override Preview", ""]
    if not recipe.templates:
        lines.append("No template/override targets declared.")
        return lines + [""]
    for tpl in recipe.templates:
        target = project_root / tpl.target
        exists = target.exists()
        action = "review update/skip/diff" if exists else "review create"
        state = "exists" if exists else "missing"
        lines.append(f"- {tpl.target}: {state}; condition `{tpl.condition}`; action: {action}.")
    return lines + [""]


def build_init_brief(project_root: Path, recipe_id: str) -> str:
    project_root = project_root.resolve()
    toml_read, data = _load_manifest(project_root)
    _recipe_read, recipe, recipe_dir = _load_recipe(project_root, recipe_id)

    project = toml_read.read_project(data)
    agents = toml_read.read_agents(data)
    manifest_recipes = toml_read.read_recipes(data)
    manifest_mcp = toml_read.read_mcp(data)
    bindings = toml_read.read_bindings(data)
    state = _install_state(recipe_id, manifest_recipes)

    prompt_path = recipe_dir / recipe.init.prompt
    prompt_text = prompt_path.read_text(encoding="utf-8")

    lines = [
        "# Recipe Init Brief",
        "",
        "## Recipe",
        "",
        f"- ID: {recipe.id}",
        f"- Name: {recipe.name}",
        f"- Version: {recipe.version}",
        f"- Description: {recipe.description}",
        f"- Install state: {state}",
        "",
        "## Init Workflow",
        "",
        f"- Prompt: {recipe.init.prompt}",
        f"- Description: {recipe.init.description or '(none)'}",
        f"- Needs manifest: {str(recipe.init.needs_manifest).lower()}",
        f"- Needs MCP: {', '.join(recipe.init.needs_mcp) if recipe.init.needs_mcp else '(none)'}",
        "",
        "## Prompt Content",
        "",
        prompt_text.rstrip(),
        "",
        "## Manifest Context",
        "",
        f"- Project: {project.get('name') or '(unnamed)'}",
        f"- Enabled agents: {', '.join(agents.get('enabled', [])) if agents.get('enabled') else '(none)'}",
        f"- Active bindings: {len(bindings)}",
        "",
    ]
    lines.extend(_config_lines(recipe, recipe_id, manifest_recipes))
    lines.extend(_mcp_lines(recipe, manifest_mcp))
    lines.extend(_template_lines(recipe, project_root))
    lines.extend([
        "## Reviewable Next Actions",
        "",
        "- Ask the project-specific setup questions from the prompt.",
        f"- Review any proposed `[recipes.{recipe_id}.config]` changes before editing the manifest.",
        "- Run `ai-specs sync` only after humans approve durable configuration changes.",
        "- No files were changed. Init does not run sync or materialize primitives.",
        "",
    ])
    return "\n".join(lines)


def init_recipe(project_root: Path, recipe_id: str) -> int:
    try:
        print(build_init_brief(project_root, recipe_id))
    except RecipeInitError as exc:
        print(str(exc), file=sys.stderr)
        return 1
    return 0


def main() -> int:
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <project_root> <recipe_id>", file=sys.stderr)
        return 2
    return init_recipe(Path(sys.argv[1]), sys.argv[2])


if __name__ == "__main__":
    sys.exit(main())
