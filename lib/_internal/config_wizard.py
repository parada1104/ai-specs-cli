#!/usr/bin/env python3
"""Questionary-driven per-recipe config wizard.

Reuses ConfigField metadata for prompt type / validation / defaults.
Writes back via recipe-config-write.update_recipe_config.
"""
from __future__ import annotations

import argparse
import importlib.util
import re
import sys
from collections.abc import Callable
from pathlib import Path
from typing import Any


def _load_sibling(name: str):
    path = Path(__file__).with_name(f"{name}.py")
    spec = importlib.util.spec_from_file_location(name.replace("-", "_"), path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load sibling module {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


_util = _load_sibling("util")
_recipe_schema = _load_sibling("recipe_schema")
_dep_check = _load_sibling("dep_check")
_recipe_read = _load_sibling("recipe-read")
_toml_read = _load_sibling("toml-read")
_config_write = _load_sibling("recipe-config-write")

Recipe = _recipe_schema.Recipe


def _required_validator(value: str) -> bool | str:
    return True if value.strip() else "This field is required."


def _regex_validator(pattern: str) -> Callable[[str], bool | str]:
    rx = re.compile(pattern)

    def _v(value: str) -> bool | str:
        if not value:
            return True
        return True if rx.match(value) else f"Must match {pattern}"

    return _v


def _compose_required_regex(pattern: str) -> Callable[[str], bool | str]:
    rx_v = _regex_validator(pattern)

    def _v(value: str) -> bool | str:
        required = _required_validator(value)
        if required is not True:
            return required
        return rx_v(value)

    return _v


def _field_default(field, existing_config: dict, key: str):
    if key in existing_config:
        return existing_config[key]
    if field.default is not None:
        return field.default
    if field.type == "bool":
        return False
    return ""


def _prompt_message(recipe_id: str, key: str, field) -> str:
    req = "required" if field.required else "optional"
    kind = field.type or ("enum" if field.enum else "string")
    return f"{recipe_id}.{key} ({req}, {kind})"


def run_config_wizard(recipe: Recipe, existing_config: dict) -> dict:
    """Prompt for each ConfigField in recipe.config_schema.fields (never .extra).

    Keys left blank/default are omitted so write-back keeps existing/default.
    Cancel (None from .ask) returns {}.
    """
    import questionary
    from rich.console import Console
    _console = Console(force_terminal=True)

    result: dict[str, Any] = {}
    fields = recipe.config_schema.fields
    for key in sorted(fields):
        field = fields[key]
        if field.help_text:
            _console.print(f"[dim]ℹ️  {field.help_text}[/]")
        msg = _prompt_message(recipe.id, key, field)
        default = _field_default(field, existing_config, key)

        if field.enum:
            choices = list(field.enum)
            default_choice = default if default in choices else (choices[0] if choices else None)
            answer = questionary.select(msg, choices=choices, default=default_choice).ask()
            if answer is None:
                return {}
            result[key] = answer
            continue

        if field.type == "bool":
            answer = questionary.confirm(msg, default=bool(default)).ask()
            if answer is None:
                return {}
            result[key] = bool(answer)
            continue

        regex = ""
        if isinstance(field.validation, dict):
            regex = str(field.validation.get("regex") or "")

        validate = None
        if regex and field.required:
            validate = _compose_required_regex(regex)
        elif regex:
            validate = _regex_validator(regex)
        elif field.required:
            validate = _required_validator

        answer = questionary.text(
            msg,
            default="" if default is None else str(default),
            validate=validate,
        ).ask()
        if answer is None:
            return {}
        if not str(answer).strip():
            # Blank → omit (keep existing/default).
            continue
        result[key] = answer
    return result


def _render_dep_panel(results, console) -> None:
    if not results:
        return
    lines = []
    for r in results:
        if r.ok:
            status = "OK"
        elif r.required:
            status = "WARN"
        else:
            status = "INFO"
        hint = f" → {r.install_url}" if r.install_url and not r.ok else ""
        detail = f" ({r.detail})" if r.detail else ""
        lines.append(f"{status:4s}  {r.binary:12s}  {r.purpose}{detail}{hint}")
    console.print("\n".join(lines))


def _dep_gate(recipe: Recipe, console) -> bool:
    import questionary

    results = _dep_check.check_cli_deps(recipe)
    _render_dep_panel(results, console)
    missing_required = [r for r in results if r.required and not r.ok]
    if not missing_required:
        return True
    answer = questionary.confirm(
        f"{len(missing_required)} required CLI tool(s) missing. Configure anyway?",
        default=False,
    ).ask()
    return bool(answer)


def configure_selected_recipes(
    project_root: Path,
    recipe_ids: list[str],
    existing_manifest: Path,
) -> dict:
    """Configure each recipe: dep gate → wizard → surgical write-back."""
    from rich.console import Console

    console = Console(stderr=True)
    catalog = _util.ai_specs_home() / "catalog" / "recipes"
    configured: dict[str, dict] = {}

    # Existing config values from the manifest (if any).
    existing_by_id: dict[str, dict] = {}
    try:
        data = _toml_read.load_toml(existing_manifest)
        for rid, entry in _toml_read.read_recipes(data).items():
            existing_by_id[rid] = dict(entry.get("config") or {})
    except Exception:
        existing_by_id = {}

    for rid in recipe_ids:
        try:
            recipe = _recipe_read.read_recipe(catalog, rid)
        except Exception as exc:  # noqa: BLE001
            console.print(f"[yellow]skip {rid}: {exc}[/yellow]")
            continue

        if recipe.cli_deps and not _dep_gate(recipe, console):
            console.print(f"[yellow]Skipped {rid} (missing CLI deps)[/yellow]")
            continue

        if not recipe.config_schema.fields:
            continue

        values = run_config_wizard(recipe, existing_by_id.get(rid, {}))
        if not values:
            continue
        _config_write.update_recipe_config(existing_manifest, rid, values)
        configured[rid] = values
    return configured


def _enabled_recipe_ids(project_root: Path) -> list[str]:
    manifest = project_root / "ai-specs" / "ai-specs.toml"
    if not manifest.is_file():
        return []
    data = _toml_read.load_toml(manifest)
    recipes = _toml_read.read_recipes(data)
    return [rid for rid, entry in recipes.items() if entry.get("enabled")]


def _offer_envrc(project_root: Path) -> None:
    """Prompt for MCP env vars, write .envrc, run direnv allow."""
    try:
        envrc = _load_sibling("envrc-scaffold")
    except Exception:
        return
    import questionary
    from rich.console import Console

    try:
        vars_map = envrc.collect_env_vars(project_root)
    except Exception:
        return
    if not vars_map:
        return

    console = Console()
    values = envrc.prompt_env_vars(project_root)
    if values is None:
        return
    if not values:
        return

    path = envrc.write_envrc(project_root, values)
    console.print(f"[green]✓[/green] escrito {path}")

    if not envrc.direnv_allow(project_root):
        print("  ! direnv no está instalado o no se pudo ejecutar.", file=sys.stderr)
        print("    Instalalo con: brew install direnv", file=sys.stderr)
        print("    Despues corre: direnv allow", file=sys.stderr)
    else:
        print("  ✓ direnv allow — las variables están activas en esta terminal")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Configure recipe config + CLI deps")
    parser.add_argument("path", nargs="?", default=".", help="Project root")
    args = parser.parse_args(argv)

    project_root = Path(args.path).resolve()
    manifest = project_root / "ai-specs" / "ai-specs.toml"
    if not manifest.is_file():
        print(f"Proyecto no inicializado: missing {manifest}", file=sys.stderr)
        return 1

    err = _util.ensure_deps(_util.vendor_dir())
    if err is not None:
        return err
    if not sys.stdin.isatty() or not sys.stdout.isatty():
        print("configure-recipes requires an interactive TTY", file=sys.stderr)
        return 3

    recipe_ids = _enabled_recipe_ids(project_root)
    if not recipe_ids:
        print("No enabled recipes found in the manifest.")
        return 0

    configure_selected_recipes(project_root, recipe_ids, manifest)
    _offer_envrc(project_root)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
