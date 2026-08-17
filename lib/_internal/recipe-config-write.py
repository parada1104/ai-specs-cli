#!/usr/bin/env python3
"""Surgical, comment-preserving updater for [recipes.<id>.config] blocks.

Does NOT round-trip through a TOML library. Validates with tomllib.loads and
restores the original bytes on failure (same guarantee as recipe-add.py).
"""
from __future__ import annotations

import importlib.util
import json
import re
import sys
import tomllib
from pathlib import Path


class RecipeConfigWriteError(Exception):
    """Raised when a config write would produce invalid TOML (original restored)."""


def _load_sibling(name: str):
    path = Path(__file__).with_name(f"{name}.py")
    spec = importlib.util.spec_from_file_location(name.replace("-", "_"), path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load sibling module {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


_toml_write = _load_sibling("toml_write")


def _toml_key(key: str) -> str:
    if re.match(r"^[A-Za-z0-9_-]+$", key):
        return key
    return json.dumps(key)


def _header_for(recipe_id: str, suffix: str = "") -> str:
    key = _toml_key(recipe_id)
    if suffix:
        return f"[recipes.{key}.{suffix}]"
    return f"[recipes.{key}]"


def _is_header(line: str) -> bool:
    return line.lstrip().startswith("[")


def _header_name(line: str) -> str:
    return line.strip()


def _find_recipe_header(lines: list[str], recipe_id: str) -> int | None:
    want = _header_for(recipe_id)
    for idx, line in enumerate(lines):
        if line.strip() == want:
            return idx
    return None


def _recipe_region_end(lines: list[str], start: int, recipe_id: str) -> int:
    """End index (exclusive) of the recipe region starting at header `start`."""
    prefix = f"[recipes.{_toml_key(recipe_id)}"
    for idx in range(start + 1, len(lines)):
        stripped = lines[idx].strip()
        if stripped.startswith("[") and not stripped.startswith(prefix):
            return idx
    return len(lines)


def _find_config_header(lines: list[str], start: int, end: int, recipe_id: str) -> int | None:
    want = _header_for(recipe_id, "config")
    for idx in range(start, end):
        if lines[idx].strip() == want:
            return idx
    return None


def _config_block_end(lines: list[str], start: int, region_end: int) -> int:
    for idx in range(start + 1, region_end):
        if _is_header(lines[idx]):
            return idx
    return region_end


def _key_line_re(key: str) -> re.Pattern[str]:
    # Match bare key or quoted key at start of line (allow leading whitespace).
    bare = re.escape(key)
    quoted = re.escape(json.dumps(key))
    return re.compile(rf"^(\s*)(?:{bare}|{quoted})\s*=")

def _split_inline_comment(line: str) -> tuple[str, str]:
    """Return (value prefix, comment suffix) with TOML string awareness."""
    quote = ""
    triple = False
    escaped = False
    idx = 0
    while idx < len(line):
        if triple:
            marker = quote * 3
            if line.startswith(marker, idx):
                triple = False
                idx += 3
                continue
            idx += 1
            continue
        char = line[idx]
        if quote == '"':
            if escaped:
                escaped = False
            elif char == "\\":
                escaped = True
            elif char == '"':
                quote = ""
            idx += 1
            continue
        if quote == "'":
            if char == "'":
                quote = ""
            idx += 1
            continue
        if line.startswith('"""', idx) or line.startswith("'''", idx):
            quote = char
            triple = True
            idx += 3
            continue
        if char in ("'", '"'):
            quote = char
        elif char == "#":
            start = idx
            while start > 0 and line[start - 1] in " \t":
                start -= 1
            return line[:start], line[start:]
        idx += 1
    return line, ""


def _value_is_multiline(line: str) -> bool:
    """Detect an unfinished array/table/string value on one source line."""
    _value, _comment = _split_inline_comment(line)
    equals = _value.find("=")
    if equals < 0:
        return False
    text = _value[equals + 1:]
    quote = ""
    triple = False
    escaped = False
    depth = 0
    idx = 0
    while idx < len(text):
        char = text[idx]
        if triple:
            if text.startswith(quote * 3, idx):
                triple = False
                idx += 3
                continue
            idx += 1
            continue
        if quote:
            if quote == '"' and escaped:
                escaped = False
            elif quote == '"' and char == "\\":
                escaped = True
            elif char == quote:
                quote = ""
            idx += 1
            continue
        if text.startswith('"""', idx) or text.startswith("'''", idx):
            quote = text[idx]
            triple = True
            idx += 3
            continue
        if char in ("'", '"'):
            quote = char
        elif char in "[{":
            depth += 1
        elif char in "]}":
            depth = max(0, depth - 1)
        idx += 1
    return bool(quote or triple or depth)


def _existing_config(manifest_path: Path, recipe_id: str) -> dict:
    try:
        parsed = tomllib.loads(manifest_path.read_text(encoding="utf-8"))
    except tomllib.TOMLDecodeError:
        return {}
    recipes = parsed.get("recipes") or {}
    recipe = recipes.get(recipe_id) if isinstance(recipes, dict) else None
    config = recipe.get("config") if isinstance(recipe, dict) else None
    return dict(config) if isinstance(config, dict) else {}


def update_recipe_config(manifest_path: Path, recipe_id: str, values: dict) -> None:
    """Write values into [recipes.<id>.config], preserving comments and bytes."""
    if not values:
        return

    original = manifest_path.read_text(encoding="utf-8")
    lines = original.splitlines(keepends=True)
    current = _existing_config(manifest_path, recipe_id)
    pending_values = {
        key: value for key, value in values.items()
        if key not in current or current[key] != value
    }
    if not pending_values:
        return

    recipe_idx = _find_recipe_header(lines, recipe_id)
    if recipe_idx is None:
        block_lines = ["\n", f"{_header_for(recipe_id)}\n", "enabled = true\n", "\n"]
        block_lines.append(f"{_header_for(recipe_id, 'config')}\n")
        for key in sorted(pending_values):
            block_lines.append(
                f"{_toml_key(key)} = {_toml_write.toml_value(pending_values[key])}\n"
            )
        new_text = original if original.endswith("\n") or original == "" else original + "\n"
        new_text += "".join(block_lines)
    else:
        region_end = _recipe_region_end(lines, recipe_idx, recipe_id)
        config_idx = _find_config_header(lines, recipe_idx, region_end, recipe_id)
        if config_idx is None:
            insert_at = region_end
            lines[insert_at:insert_at] = ["\n", f"{_header_for(recipe_id, 'config')}\n"]
            config_idx = insert_at + 1
            region_end = _recipe_region_end(lines, recipe_idx, recipe_id)

        block_end = _config_block_end(lines, config_idx, region_end)
        pending: list[tuple[str, object]] = []
        for key in sorted(pending_values):
            pattern = _key_line_re(key)
            replaced = False
            for idx in range(config_idx + 1, block_end):
                match = pattern.match(lines[idx])
                if not match:
                    continue
                if _value_is_multiline(lines[idx]):
                    raise RecipeConfigWriteError(
                        f"cannot replace multiline value for key '{key}'"
                    )
                indent = match.group(1)
                _value_part, comment = _split_inline_comment(lines[idx])
                newline = "\n" if lines[idx].endswith("\n") else ""
                lines[idx] = (
                    f"{indent}{_toml_key(key)} = "
                    f"{_toml_write.toml_value(pending_values[key])}"
                    f"{comment.rstrip(chr(10) + chr(13))}{newline}"
                )
                replaced = True
                break
            if not replaced:
                pending.append((key, pending_values[key]))

        if pending:
            new_lines = [
                f"{_toml_key(key)} = {_toml_write.toml_value(value)}\n"
                for key, value in pending
            ]
            lines[block_end:block_end] = new_lines
        new_text = "".join(lines)

    try:
        tomllib.loads(new_text)
    except tomllib.TOMLDecodeError as exc:
        manifest_path.write_text(original, encoding="utf-8")
        raise RecipeConfigWriteError(f"invalid TOML after config write: {exc}") from exc

    if new_text != original:
        manifest_path.write_text(new_text, encoding="utf-8")


def main(argv: list[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    if len(args) != 1:
        print(f"Usage: {Path(__file__).name} <manifest-path>", file=sys.stderr)
        return 2
    print("recipe-config-write.py is a library module; use config_wizard.py", file=sys.stderr)
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
