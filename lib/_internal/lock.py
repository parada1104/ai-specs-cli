#!/usr/bin/env python3
"""Shared lock file read/write helper for ai-specs.

The lock lives at <project>/ai-specs/.ai-specs.lock and tracks SHA-256 hashes
of managed files so the CLI can detect user customizations.
"""

from __future__ import annotations

import hashlib
import tomllib
from pathlib import Path

LOCK_HEADER = """\
# Managed by ai-specs. Do not edit by hand.
# Tracks SHA-256 of bundled files as last installed by the CLI.
# Used by `ai-specs refresh-bundled` to detect user customizations.
"""


def sha256_of(path: Path) -> str:
    data = path.read_bytes().replace(b"\r\n", b"\n")
    return hashlib.sha256(data).hexdigest()


def load_lock(lock_path: Path) -> dict:
    if not lock_path.is_file():
        return {"skills": {}, "commands": {}, "opted_out": [], "recipes": {}, "deps": {}}
    with lock_path.open("rb") as f:
        data = tomllib.load(f)
    return {
        "skills": {k: dict(v) for k, v in (data.get("skills") or {}).items()},
        "commands": dict(data.get("commands") or {}),
        "opted_out": list(data.get("opted-out", {}).get("files", []) or []),
        "recipes": {k: dict(v) for k, v in (data.get("recipes") or {}).items()},
        "deps": {k: dict(v) for k, v in (data.get("deps") or {}).items()},
    }


def write_lock(lock_path: Path, lock: dict) -> None:
    out = [LOCK_HEADER]

    skills = lock.get("skills") or {}
    for skill in sorted(skills):
        files = skills[skill]
        if not files:
            continue
        out.append(f'[skills."{skill}"]')
        for rel in sorted(files):
            out.append(f'"{rel}" = "{files[rel]}"')
        out.append("")

    commands = lock.get("commands") or {}
    if commands:
        out.append("[commands]")
        for rel in sorted(commands):
            out.append(f'"{rel}" = "{commands[rel]}"')
        out.append("")

    recipes = lock.get("recipes") or {}
    for recipe_id in sorted(recipes):
        recipe_skills = recipes[recipe_id]
        for skill_name in sorted(recipe_skills):
            files = recipe_skills[skill_name]
            if not files:
                continue
            out.append(f'[recipes."{recipe_id}".skills."{skill_name}"]')
            for rel in sorted(files):
                out.append(f'"{rel}" = "{files[rel]}"')
            out.append("")

    deps = lock.get("deps") or {}
    for dep_id in sorted(deps):
        dep_skills = deps[dep_id]
        for skill_name in sorted(dep_skills):
            files = dep_skills[skill_name]
            if not files:
                continue
            out.append(f'[deps."{dep_id}".skills."{skill_name}"]')
            for rel in sorted(files):
                out.append(f'"{rel}" = "{files[rel]}"')
            out.append("")

    opted = sorted(set(lock.get("opted_out") or []))
    if opted:
        out.append("[opted-out]")
        out.append("# Bundled files the user deleted intentionally; the CLI will")
        out.append("# not re-install them. Remove a line here to let the file be")
        out.append("# restored on the next `ai-specs refresh-bundled`.")
        formatted = ", ".join(f'"{name}"' for name in opted)
        out.append(f"files = [{formatted}]")
        out.append("")

    lock_path.parent.mkdir(parents=True, exist_ok=True)
    lock_path.write_text("\n".join(out).rstrip("\n") + "\n")


def set_recipe_skill_hashes(lock: dict, recipe_id: str, skill_name: str, hashes: dict[str, str]) -> None:
    lock.setdefault("recipes", {}).setdefault(recipe_id, {})[skill_name] = dict(hashes)


def set_dep_skill_hashes(lock: dict, dep_id: str, skill_name: str, hashes: dict[str, str]) -> None:
    lock.setdefault("deps", {}).setdefault(dep_id, {})[skill_name] = dict(hashes)
