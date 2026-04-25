#!/usr/bin/env python3
"""Shared parsing, normalization, validation, and rendering for skill metadata."""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path
from typing import Any


BLOCK_SCALARS = {">", "|", ">-", "|-"}
NAME_RE = re.compile(r"^[a-z0-9][a-z0-9-]*$")
VERSION_RE = re.compile(r"^[0-9]+\.[0-9]+(?:\.[0-9]+)?(?:[-+][A-Za-z0-9.-]+)?$")


class SkillContractError(ValueError):
    def __init__(self, field: str, message: str, *, path: str | Path | None = None):
        location = f"{path}: " if path else ""
        super().__init__(f"{location}{field}: {message}")
        self.field = field
        self.message = message
        self.path = str(path) if path else None

    def as_dict(self) -> dict[str, str | None]:
        return {"field": self.field, "message": self.message, "path": self.path}


def split_frontmatter(text: str) -> tuple[str, str]:
    if not text.startswith("---"):
        return "", text
    rest = text[3:]
    end = rest.find("\n---")
    if end < 0:
        return "", text
    frontmatter = rest[:end].lstrip("\n")
    body = rest[end + 4 :]
    if body.startswith("\n"):
        body = body[1:]
    return frontmatter, body


def _strip_quotes(value: str) -> str:
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {'"', "'"}:
        return value[1:-1]
    return value


def _split_inline_list(value: str) -> list[str]:
    inner = value.strip()[1:-1].strip()
    if not inner:
        return []
    return [_strip_quotes(part.strip()) for part in inner.split(",") if part.strip()]


def _consume_block(lines: list[str], start: int, indent: int) -> tuple[str, int]:
    parts: list[str] = []
    i = start
    while i < len(lines):
        line = lines[i]
        if not line.strip():
            parts.append("")
            i += 1
            continue
        current_indent = len(line) - len(line.lstrip(" "))
        if current_indent < indent:
            break
        parts.append(line[indent:])
        i += 1
    folded = " ".join(part.strip() for part in parts if part.strip())
    return folded.strip(), i


def _consume_list(lines: list[str], start: int, indent: int) -> tuple[list[str], int]:
    items: list[str] = []
    i = start
    while i < len(lines):
        line = lines[i]
        if not line.strip():
            i += 1
            continue
        current_indent = len(line) - len(line.lstrip(" "))
        if current_indent < indent:
            break
        stripped = line[indent:]
        if not stripped.startswith("- "):
            break
        items.append(_strip_quotes(stripped[2:].strip()))
        i += 1
    return items, i


def _next_content_indent(lines: list[str], start: int) -> int | None:
    i = start
    while i < len(lines):
        line = lines[i]
        if line.strip():
            return len(line) - len(line.lstrip(" "))
        i += 1
    return None


def parse_frontmatter(frontmatter: str) -> dict[str, Any]:
    data: dict[str, Any] = {}
    lines = frontmatter.splitlines()
    i = 0

    while i < len(lines):
        line = lines[i]
        if not line.strip():
            i += 1
            continue
        if line.startswith(" "):
            raise SkillContractError("frontmatter", "unexpected indentation")

        if not line.endswith(":") and ":" not in line:
            raise SkillContractError("frontmatter", f"unsupported line: {line}")

        key, _, rest = line.partition(":")
        key = key.strip()
        rest = rest.strip()

        if key == "metadata":
            metadata: dict[str, Any] = {}
            i += 1
            metadata_indent: int | None = None
            while i < len(lines):
                nested = lines[i]
                if not nested.strip():
                    i += 1
                    continue
                indent = len(nested) - len(nested.lstrip(" "))
                if indent == 0:
                    break
                if metadata_indent is None:
                    metadata_indent = indent
                if indent != metadata_indent:
                    raise SkillContractError("metadata", f"unsupported indentation: {nested}")
                subkey, _, subrest = nested.strip().partition(":")
                subrest = subrest.strip()
                i += 1
                child_indent = _next_content_indent(lines, i) or (metadata_indent + 2)

                if subrest in BLOCK_SCALARS:
                    value, i = _consume_block(lines, i, child_indent)
                elif subrest.startswith("[") and subrest.endswith("]"):
                    value = _split_inline_list(subrest)
                elif subrest:
                    value = _strip_quotes(subrest)
                else:
                    value, i = _consume_list(lines, i, child_indent)
                    if not value:
                        value, i = _consume_block(lines, i, child_indent)
                metadata[subkey] = value
            data[key] = metadata
            continue

        i += 1
        child_indent = _next_content_indent(lines, i) or 1
        if rest in BLOCK_SCALARS:
            value, i = _consume_block(lines, i, child_indent)
        elif rest.startswith("[") and rest.endswith("]"):
            value = _split_inline_list(rest)
        else:
            value = _strip_quotes(rest)
        data[key] = value

    return data


def read_skill_text(text: str) -> tuple[dict[str, Any], str]:
    frontmatter, body = split_frontmatter(text)
    if not frontmatter:
        raise SkillContractError("frontmatter", "missing YAML frontmatter")
    return parse_frontmatter(frontmatter), body


def _require_string(value: Any, field: str, *, path: str | Path | None = None, default: str | None = None, warnings: list[str] | None = None, compatibility_label: str | None = None) -> str:
    if value is None or (isinstance(value, str) and not value.strip()):
        if default is not None:
            if warnings is not None and compatibility_label:
                warnings.append(f"{compatibility_label}; add `{field}` explicitly")
            return default
        raise SkillContractError(field, "is required", path=path)
    if not isinstance(value, str):
        raise SkillContractError(field, "must be a string", path=path)
    stripped = value.strip()
    if not stripped:
        raise SkillContractError(field, "must not be empty", path=path)
    return stripped


def _normalize_string_list(value: Any, field: str, *, path: str | Path | None = None, warnings: list[str] | None = None, compatibility: bool = False) -> list[str]:
    if value in (None, ""):
        return []
    if isinstance(value, str):
        stripped = value.strip()
        if not stripped:
            return []
        if not compatibility:
            raise SkillContractError(field, "must be a YAML list of strings", path=path)
        if warnings is not None:
            warnings.append(f"converted scalar `{field}` to a one-item list")
        return [stripped]
    if not isinstance(value, list):
        raise SkillContractError(field, "must be a YAML list of strings", path=path)

    normalized: list[str] = []
    for item in value:
        if not isinstance(item, str):
            raise SkillContractError(field, "must contain only strings", path=path)
        stripped = item.strip()
        if not stripped:
            raise SkillContractError(field, "must not contain empty items", path=path)
        normalized.append(stripped)
    return normalized


def canonical_description(description: str) -> str:
    trimmed = description.strip()
    trigger_idx = trimmed.find(" Trigger:")
    if trigger_idx >= 0:
        trimmed = trimmed[:trigger_idx]
    return trimmed.strip().rstrip(".")


def _validate_name(name: str, *, path: str | Path | None = None) -> str:
    if not NAME_RE.fullmatch(name):
        raise SkillContractError("name", "must be lowercase kebab-case", path=path)
    return name


def _validate_version(version: str, *, path: str | Path | None = None) -> str:
    if not VERSION_RE.fullmatch(version):
        raise SkillContractError("metadata.version", "must look like a semantic version (for example `1.0` or `1.0.0`)", path=path)
    return version


def normalize_local_skill(raw: dict[str, Any], body: str, *, path: str | Path | None = None, compatibility: bool = True) -> dict[str, Any]:
    warnings: list[str] = []
    metadata = raw.get("metadata") or {}
    if metadata and not isinstance(metadata, dict):
        raise SkillContractError("metadata", "must be a mapping", path=path)

    name = _validate_name(_require_string(raw.get("name"), "name", path=path), path=path)
    description = _require_string(raw.get("description"), "description", path=path)
    license_id = _require_string(
        raw.get("license"),
        "license",
        path=path,
        default="Apache-2.0" if compatibility else None,
        warnings=warnings,
        compatibility_label="using compatibility default `Apache-2.0`",
    )
    author = _require_string(
        metadata.get("author"),
        "metadata.author",
        path=path,
        default="unknown" if compatibility else None,
        warnings=warnings,
        compatibility_label="using compatibility default `unknown` author",
    )
    version = _validate_version(
        _require_string(
            metadata.get("version"),
            "metadata.version",
            path=path,
            default="1.0" if compatibility else None,
            warnings=warnings,
            compatibility_label="using compatibility default `1.0` version",
        ),
        path=path,
    )

    scope = _normalize_string_list(metadata.get("scope"), "metadata.scope", path=path, warnings=warnings, compatibility=compatibility)
    auto_invoke = _normalize_string_list(metadata.get("auto_invoke"), "metadata.auto_invoke", path=path, warnings=warnings, compatibility=compatibility)

    normalized = {
        "name": name,
        "description": description.strip(),
        "description_summary": canonical_description(description),
        "license": license_id,
        "metadata": {
            "author": author,
            "version": version,
        },
        "warnings": warnings,
        "body": body,
    }
    if scope:
        normalized["metadata"]["scope"] = scope
    if auto_invoke:
        normalized["metadata"]["auto_invoke"] = auto_invoke
    return normalized


def from_local_skill(path: str | Path, *, compatibility: bool = True) -> dict[str, Any]:
    skill_path = Path(path)
    raw, body = read_skill_text(skill_path.read_text())
    return normalize_local_skill(raw, body, path=skill_path, compatibility=compatibility)


def from_dep(dep: dict[str, Any], upstream_text: str) -> dict[str, Any]:
    dep_id = _validate_name(_require_string(dep.get("id"), "id"))
    source = _require_string(dep.get("source"), "source")
    frontmatter, body = split_frontmatter(upstream_text)
    raw = parse_frontmatter(frontmatter) if frontmatter else {}
    upstream_description = canonical_description(str(raw.get("description") or ""))

    attribution = str(dep.get("vendor_attribution") or "").strip()
    description_parts: list[str] = []
    if upstream_description:
        description_parts.append(upstream_description.rstrip(". "))
    if attribution:
        description_parts.append(f"Vendored from {attribution} (see metadata.source)")
    description = ". ".join(description_parts) or f"Vendored skill: {dep_id}"

    scope = dep.get("scope") or ["root"]
    auto_invoke = dep.get("auto_invoke") or []

    metadata: dict[str, Any] = {
        "author": attribution or "upstream",
        "version": _validate_version(str(dep.get("version") or "1.0")),
        "source": source,
    }
    if attribution:
        metadata["vendor_attribution"] = attribution

    normalized = {
        "name": dep_id,
        "description": description,
        "description_summary": canonical_description(description),
        "license": str(dep.get("license") or "Unknown"),
        "metadata": metadata,
        "warnings": [],
        "body": body,
    }

    scope_list = _normalize_string_list(scope, "metadata.scope", compatibility=isinstance(scope, str))
    auto_list = _normalize_string_list(auto_invoke, "metadata.auto_invoke", compatibility=isinstance(auto_invoke, str))
    if scope_list:
        normalized["metadata"]["scope"] = scope_list
    if auto_list:
        normalized["metadata"]["auto_invoke"] = auto_list
    return normalized


def validate_sync_metadata(skill: dict[str, Any], *, path: str | Path | None = None) -> dict[str, Any]:
    metadata = skill.get("metadata") or {}
    scope = metadata.get("scope") or []
    auto_invoke = metadata.get("auto_invoke") or []
    if not scope and not auto_invoke:
        return {"enabled": False, "name": skill["name"], "scope": [], "auto_invoke": []}
    if not scope:
        raise SkillContractError("metadata.scope", "is required when metadata.auto_invoke is set", path=path)
    if not auto_invoke:
        raise SkillContractError("metadata.auto_invoke", "is required when metadata.scope is set", path=path)
    return {"enabled": True, "name": skill["name"], "scope": list(scope), "auto_invoke": list(auto_invoke)}


def _yaml_quote(value: str) -> str:
    escaped = value.replace("\\", "\\\\").replace('"', '\\"')
    return f'"{escaped}"'


def render_frontmatter(skill: dict[str, Any]) -> str:
    metadata = skill["metadata"]
    out = [
        "---",
        f"name: {skill['name']}",
        "description: >",
        f"  {skill['description'].strip()}",
        f"license: {skill['license']}",
        "metadata:",
        f"  author: {_yaml_quote(str(metadata['author']))}",
        f"  version: {_yaml_quote(str(metadata['version']))}",
    ]
    if metadata.get("source"):
        out.append(f"  source: {_yaml_quote(str(metadata['source']))}")
    if metadata.get("vendor_attribution"):
        out.append(f"  vendor_attribution: {_yaml_quote(str(metadata['vendor_attribution']))}")
    if metadata.get("scope"):
        scopes = ", ".join(str(scope) for scope in metadata["scope"])
        out.append(f"  scope: [{scopes}]")
    if metadata.get("auto_invoke"):
        out.append("  auto_invoke:")
        for phrase in metadata["auto_invoke"]:
            out.append(f"    - {_yaml_quote(str(phrase))}")
    out.append("---")
    return "\n".join(out) + "\n"


def render_skill_markdown(skill: dict[str, Any]) -> str:
    body = skill.get("body", "")
    rendered = render_frontmatter(skill)
    if body:
        return rendered + "\n" + body.lstrip("\n")
    return rendered


def _cli_sync_metadata(path_arg: str) -> int:
    try:
        skill = from_local_skill(path_arg, compatibility=True)
        payload = validate_sync_metadata(skill, path=path_arg)
        payload["warnings"] = skill["warnings"]
        print(json.dumps(payload))
        return 0
    except SkillContractError as exc:
        print(json.dumps({"error": exc.as_dict()}), file=sys.stderr)
        return 1


def main(argv: list[str]) -> int:
    if len(argv) != 3 or argv[1] != "sync-metadata":
        print(f"Usage: {argv[0]} sync-metadata <skill_md_path>", file=sys.stderr)
        return 2
    return _cli_sync_metadata(argv[2])


if __name__ == "__main__":
    sys.exit(main(sys.argv))
