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


def update_recipe_config(manifest_path: Path, recipe_id: str, values: dict) -> None:
    """Write values into [recipes.<id>.config], preserving comments.

    No-op when values is empty. Restores original text and raises
    RecipeConfigWriteError if the result is not valid TOML.
    """
    if not values:
        return

    original = manifest_path.read_text(encoding="utf-8")
    lines = original.splitlines(keepends=True)

    recipe_idx = _find_recipe_header(lines, recipe_id)
    if recipe_idx is None:
        # APPEND full recipe + config block.
        block_lines = ["\n", f"{_header_for(recipe_id)}\n", "enabled = true\n", 'version = "0.0.0"\n', "\n"]
        block_lines.append(f"{_header_for(recipe_id, 'config')}\n")
        for key in sorted(values):
            block_lines.append(f"{_toml_key(key)} = {_toml_write.toml_value(values[key])}\n")
        new_text = original if original.endswith("\n") or original == "" else original + "\n"
        new_text = new_text + "".join(block_lines)
    else:
        region_end = _recipe_region_end(lines, recipe_idx, recipe_id)
        config_idx = _find_config_header(lines, recipe_idx, region_end, recipe_id)
        if config_idx is None:
            # Insert config header at end of recipe region.
            insert_at = region_end
            # Prefer inserting before trailing blank that precedes next section.
            header_line = f"{_header_for(recipe_id, 'config')}\n"
            lines[insert_at:insert_at] = ["\n", header_line]
            config_idx = insert_at + 1
            region_end = _recipe_region_end(lines, recipe_idx, recipe_id)

        block_end = _config_block_end(lines, config_idx, region_end)
        pending: list[tuple[str, object]] = []
        for key in sorted(values):
            pattern = _key_line_re(key)
            replaced = False
            for idx in range(config_idx + 1, block_end):
                match = pattern.match(lines[idx])
                if match:
                    indent = match.group(1)
                    lines[idx] = f"{indent}{_toml_key(key)} = {_toml_write.toml_value(values[key])}\n"
                    replaced = True
                    break
            if not replaced:
                pending.append((key, values[key]))

        if pending:
            insert_at = block_end
            new_lines = [
                f"{_toml_key(key)} = {_toml_write.toml_value(val)}\n" for key, val in pending
            ]
            lines[insert_at:insert_at] = new_lines
        new_text = "".join(lines)

    try:
        tomllib.loads(new_text)
    except tomllib.TOMLDecodeError as exc:
        manifest_path.write_text(original, encoding="utf-8")
        raise RecipeConfigWriteError(f"invalid TOML after config write: {exc}") from exc

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
