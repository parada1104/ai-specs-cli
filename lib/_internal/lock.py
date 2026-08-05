#!/usr/bin/env python3
"""Shared lock file read/write helper for ai-specs."""

from __future__ import annotations

import hashlib
import tomllib
from pathlib import Path

LOCK_HEADER = """\
# Managed by ai-specs. Do not edit by hand.
# Provenance stamp: [meta] records the CLI version and timestamp of the last
# sync. [managed.*] records integrity only for CLI-owned override targets;
# it is not a general content-integrity manifest. git covers the committed
# project surface; skill/recipe/dep content hashes are not tracked.
"""


def sha256_bytes(data: bytes) -> str:
    """Hash normalized file bytes so CRLF/LF line endings are equivalent."""
    return hashlib.sha256(data.replace(b"\r\n", b"\n")).hexdigest()


def sha256_of(path: Path) -> str:
    return sha256_bytes(path.read_bytes())


def _load_skill_groups(section: dict | None) -> dict:
    """Normalize recipes/deps to their legacy in-memory shape."""
    result: dict = {}
    for owner_id, owner in (section or {}).items():
        skills = (owner or {}).get("skills") or {}
        result[owner_id] = {
            skill_name: dict(files) for skill_name, files in skills.items()
        }
    return result


def load_lock(lock_path: Path) -> dict:
    if not lock_path.is_file():
        return {
            "skills": {},
            "meta": {},
            "recipes": {},
            "deps": {},
            "agents": {},
            "managed": {},
        }
    with lock_path.open("rb") as f:
        data = tomllib.load(f)
    meta_raw = data.get("meta") or {}
    meta = {}
    if isinstance(meta_raw, dict):
        for key in ("cli_version", "synced_at"):
            value = meta_raw.get(key)
            if isinstance(value, str) and value.strip():
                meta[key] = value.strip()

    managed: dict[str, dict] = {}
    raw_managed = data.get("managed") or {}
    if isinstance(raw_managed, dict):
        for path, entry in raw_managed.items():
            if not isinstance(path, str) or not isinstance(entry, dict):
                continue
            normalized = dict(entry)
            if isinstance(normalized.get("sha256"), str) and normalized["sha256"].strip():
                normalized["sha256"] = normalized["sha256"].strip()
                managed[path] = normalized

    return {
        "skills": {k: dict(v) for k, v in (data.get("skills") or {}).items()},
        "meta": meta,
        "recipes": _load_skill_groups(data.get("recipes")),
        "deps": _load_skill_groups(data.get("deps")),
        "agents": {k: dict(v) for k, v in (data.get("agents") or {}).items()},
        "managed": managed,
    }


def _toml_string(value: str) -> str:
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'


def write_lock(lock_path: Path, lock: dict) -> None:
    out = [LOCK_HEADER]

    meta = lock.get("meta") or {}
    if meta:
        out.append("[meta]")
        if meta.get("cli_version"):
            out.append(f"cli_version = {_toml_string(str(meta['cli_version']))}")
        if meta.get("synced_at"):
            out.append(f"synced_at = {_toml_string(str(meta['synced_at']))}")
        out.append("")

    managed = lock.get("managed") or {}
    for path in sorted(managed):
        entry = managed[path]
        if not isinstance(entry, dict) or not entry.get("sha256"):
            continue
        out.append(f"[managed.{_toml_string(path)}]")
        for key in ("sha256", "recipe", "source", "kind", "policy"):
            value = entry.get(key)
            if value is not None and value != "":
                out.append(f"{key} = {_toml_string(str(value))}")
        out.append("")

    agents = lock.get("agents") or {}
    for harness in sorted(agents):
        files = agents[harness]
        if not files:
            continue
        out.append(f"[agents.{_toml_string(harness)}]")
        for name in sorted(files):
            out.append(f"{_toml_string(name)} = {_toml_string(str(files[name]))}")
        out.append("")

    lock_path.parent.mkdir(parents=True, exist_ok=True)
    lock_path.write_text("\n".join(out).rstrip("\n") + "\n")


def set_managed_override(
    lock: dict,
    path: str,
    sha256: str,
    *,
    recipe: str | None = None,
    source: str | None = None,
    kind: str | None = None,
    policy: str | None = None,
) -> None:
    """Upsert the last CLI-written bytes for one governed override target."""
    managed = lock.setdefault("managed", {})
    entry = dict(managed.get(path) or {})
    entry["sha256"] = sha256
    for key, value in (("recipe", recipe), ("source", source), ("kind", kind), ("policy", policy)):
        if value is not None:
            entry[key] = value
    managed[path] = entry


def set_recipe_skill_hashes(lock: dict, recipe_id: str, skill_name: str, hashes: dict[str, str]) -> None:
    lock.setdefault("recipes", {}).setdefault(recipe_id, {})[skill_name] = dict(hashes)


def set_dep_skill_hashes(lock: dict, dep_id: str, skill_name: str, hashes: dict[str, str]) -> None:
    lock.setdefault("deps", {}).setdefault(dep_id, {})[skill_name] = dict(hashes)


def remove_recipe_lock_entries(lock: dict, recipe_id: str) -> bool:
    """Remove all lock entries for a recipe. Returns True if anything was removed."""
    recipes = lock.get("recipes") or {}
    if recipe_id in recipes:
        del recipes[recipe_id]
        return True
    return False
