#!/usr/bin/env python3
"""Dataclasses and validation for recipe.toml schema.

See specs/recipe-schema/spec.md for canonical contract.
"""

from __future__ import annotations

import tomllib
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any


class RecipeValidationError(Exception):
    pass


CEREMONY_LEVELS = frozenset({"trivial", "local_fix", "behavior_change", "domain_change"})


@dataclass
class SkillRef:
    id: str
    source: str  # "bundled" or "dep"
    url: str = ""
    path: str = ""


@dataclass
class CommandRef:
    id: str
    path: str


@dataclass
class McpPreset:
    id: str
    config: dict[str, Any] = field(default_factory=dict)


@dataclass
class TemplateRef:
    source: str
    target: str
    condition: str = "not_exists"


@dataclass
class DocRef:
    source: str
    target: str


@dataclass
class Capability:
    id: str


@dataclass
class Hook:
    event: str
    action: str


@dataclass
class ConfigField:
    required: bool
    type: str = ""
    default: Any = None


@dataclass
class ConfigSchema:
    fields: dict[str, ConfigField] = field(default_factory=dict)


@dataclass
class SddConfig:
    threshold: str = ""


@dataclass
class Recipe:
    id: str
    name: str
    description: str
    version: str
    author: str = ""
    license: str = ""
    skills: list[SkillRef] = field(default_factory=list)
    commands: list[CommandRef] = field(default_factory=list)
    mcp: list[McpPreset] = field(default_factory=list)
    templates: list[TemplateRef] = field(default_factory=list)
    docs: list[DocRef] = field(default_factory=list)
    capabilities: list[Capability] = field(default_factory=list)
    hooks: list[Hook] = field(default_factory=list)
    config_schema: ConfigSchema = field(default_factory=ConfigSchema)
    sdd: SddConfig = field(default_factory=SddConfig)


def _require_string(data: dict[str, Any], key: str, context: str) -> str:
    value = data.get(key)
    if not isinstance(value, str) or not value.strip():
        raise RecipeValidationError(f"{context}: missing or invalid required field '{key}'")
    return value.strip()


def _parse_skills(raw: Any, context: str) -> list[SkillRef]:
    if not isinstance(raw, list):
        return []
    out: list[SkillRef] = []
    for idx, item in enumerate(raw):
        if not isinstance(item, dict):
            raise RecipeValidationError(f"{context}.skills[{idx}]: expected object, got {type(item).__name__}")
        skill_id = _require_string(item, "id", f"{context}.skills[{idx}]")
        source = _require_string(item, "source", f"{context}.skills[{idx}]")
        url = str(item.get("url", ""))
        path = str(item.get("path", ""))
        if source == "dep" and not url:
            raise RecipeValidationError(f"{context}.skills[{idx}]: source='dep' requires 'url'")
        out.append(SkillRef(id=skill_id, source=source, url=url, path=path))
    return out


def _parse_commands(raw: Any, context: str) -> list[CommandRef]:
    if not isinstance(raw, list):
        return []
    out: list[CommandRef] = []
    for idx, item in enumerate(raw):
        if not isinstance(item, dict):
            raise RecipeValidationError(f"{context}.commands[{idx}]: expected object, got {type(item).__name__}")
        cmd_id = _require_string(item, "id", f"{context}.commands[{idx}]")
        path = _require_string(item, "path", f"{context}.commands[{idx}]")
        out.append(CommandRef(id=cmd_id, path=path))
    return out


def _parse_mcp(raw: Any, context: str) -> list[McpPreset]:
    if not isinstance(raw, list):
        return []
    out: list[McpPreset] = []
    for idx, item in enumerate(raw):
        if not isinstance(item, dict):
            raise RecipeValidationError(f"{context}.mcp[{idx}]: expected object, got {type(item).__name__}")
        mcp_id = _require_string(item, "id", f"{context}.mcp[{idx}]")
        config = {k: v for k, v in item.items() if k != "id"}
        out.append(McpPreset(id=mcp_id, config=config))
    return out


def _parse_templates(raw: Any, context: str) -> list[TemplateRef]:
    if not isinstance(raw, list):
        return []
    out: list[TemplateRef] = []
    for idx, item in enumerate(raw):
        if not isinstance(item, dict):
            raise RecipeValidationError(f"{context}.templates[{idx}]: expected object, got {type(item).__name__}")
        source = _require_string(item, "source", f"{context}.templates[{idx}]")
        target = _require_string(item, "target", f"{context}.templates[{idx}]")
        condition = str(item.get("condition", "not_exists"))
        out.append(TemplateRef(source=source, target=target, condition=condition))
    return out


def _parse_docs(raw: Any, context: str) -> list[DocRef]:
    if not isinstance(raw, list):
        return []
    out: list[DocRef] = []
    for idx, item in enumerate(raw):
        if not isinstance(item, dict):
            raise RecipeValidationError(f"{context}.docs[{idx}]: expected object, got {type(item).__name__}")
        source = _require_string(item, "source", f"{context}.docs[{idx}]")
        target = _require_string(item, "target", f"{context}.docs[{idx}]")
        out.append(DocRef(source=source, target=target))
    return out


def _parse_capabilities(raw: Any, context: str) -> list[Capability]:
    if not isinstance(raw, list):
        return []
    out: list[Capability] = []
    seen: set[str] = set()
    for idx, item in enumerate(raw):
        if not isinstance(item, dict):
            raise RecipeValidationError(f"{context}.capabilities[{idx}]: expected object, got {type(item).__name__}")
        cap_id = _require_string(item, "id", f"{context}.capabilities[{idx}]")
        if cap_id in seen:
            raise RecipeValidationError(f"{context}.capabilities[{idx}]: duplicate capability id '{cap_id}'")
        seen.add(cap_id)
        out.append(Capability(id=cap_id))
    return out


def _parse_hooks(raw: Any, context: str) -> list[Hook]:
    if not isinstance(raw, list):
        return []
    out: list[Hook] = []
    for idx, item in enumerate(raw):
        if not isinstance(item, dict):
            raise RecipeValidationError(f"{context}.hooks[{idx}]: expected object, got {type(item).__name__}")
        event = _require_string(item, "event", f"{context}.hooks[{idx}]")
        action = _require_string(item, "action", f"{context}.hooks[{idx}]")
        out.append(Hook(event=event, action=action))
    return out


def _parse_config(raw: Any, context: str) -> ConfigSchema:
    if not isinstance(raw, dict):
        return ConfigSchema()
    fields: dict[str, ConfigField] = {}
    for key, value in raw.items():
        if not isinstance(value, dict):
            raise RecipeValidationError(f"{context}.config.{key}: expected table, got {type(value).__name__}")
        required = value.get("required")
        if not isinstance(required, bool):
            raise RecipeValidationError(f"{context}.config.{key}: missing or invalid 'required' (must be boolean)")
        field_type = str(value.get("type", ""))
        default = value.get("default")
        fields[key] = ConfigField(required=required, type=field_type, default=default)
    return ConfigSchema(fields=fields)


def _parse_sdd(raw: Any, context: str) -> SddConfig:
    if not isinstance(raw, dict):
        return SddConfig()
    threshold = str(raw.get("threshold", "")).strip()
    if threshold and threshold not in CEREMONY_LEVELS:
        raise RecipeValidationError(
            f"{context}.sdd.threshold: invalid value '{threshold}' (allowed: {', '.join(sorted(CEREMONY_LEVELS))})"
        )
    return SddConfig(threshold=threshold)


def validate_recipe_toml(data: dict[str, Any]) -> Recipe:
    """Validate a raw dict loaded from recipe.toml and return a Recipe dataclass."""
    recipe_table = data.get("recipe", {})
    if not isinstance(recipe_table, dict):
        raise RecipeValidationError("[recipe] must be a table")

    ctx = "[recipe]"
    recipe_id = _require_string(recipe_table, "id", ctx)
    name = _require_string(recipe_table, "name", ctx)
    description = _require_string(recipe_table, "description", ctx)
    version = _require_string(recipe_table, "version", ctx)
    author = str(recipe_table.get("author", ""))
    license_ = str(recipe_table.get("license", ""))

    provides = data.get("provides", {})
    if not isinstance(provides, dict):
        provides = {}

    ctx_prov = "[provides]"
    return Recipe(
        id=recipe_id,
        name=name,
        description=description,
        version=version,
        author=author,
        license=license_,
        skills=_parse_skills(provides.get("skills"), ctx_prov),
        commands=_parse_commands(provides.get("commands"), ctx_prov),
        mcp=_parse_mcp(provides.get("mcp"), ctx_prov),
        templates=_parse_templates(provides.get("templates"), ctx_prov),
        docs=_parse_docs(provides.get("docs"), ctx_prov),
        capabilities=_parse_capabilities(data.get("capabilities"), ""),
        hooks=_parse_hooks(data.get("hooks"), ""),
        config_schema=_parse_config(data.get("config"), ""),
        sdd=_parse_sdd(data.get("sdd"), "[sdd]"),
    )


def load_recipe_toml(path: Path) -> Recipe:
    if not path.is_file():
        raise RecipeValidationError(f"recipe.toml not found: {path}")
    with path.open("rb") as f:
        data = tomllib.load(f)
    return validate_recipe_toml(data)
