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
# Provenance stamp: [meta] records the CLI version and timestamp of the last
# sync. It is the CLI-provenance signal that travels with a fresh clone (the
# machine-local cache meta.toml does not). git covers integrity/diff of the
# committed project surface; skill/recipe/dep content hashes are not tracked.
"""


def sha256_of(path: Path) -> str:
    data = path.read_bytes().replace(b"\r\n", b"\n")
    return hashlib.sha256(data).hexdigest()


def load_lock(lock_path: Path) -> dict:
    if not lock_path.is_file():
        return {
            "skills": {},
            "meta": {},
            "recipes": {},
            "deps": {},
            "agents": {},
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

    return {
        "skills": {k: dict(v) for k, v in (data.get("skills") or {}).items()},
        "meta": meta,
        # On disk recipe/dep skills live under a `.skills.` sub-table
        # (`[recipes."<id>".skills."<skill>"]`). Unwrap that level so the
        # in-memory shape is recipes[<id>][<skill>] = {rel: sha}, matching
        # write_lock and set_recipe_skill_hashes. Reading it one level too
        # shallow leaves a stray "skills" key that corrupts the next write.
        "recipes": _load_skill_groups(data.get("recipes")),
        "deps": _load_skill_groups(data.get("deps")),
        "agents": {k: dict(v) for k, v in (data.get("agents") or {}).items()},
    }


def _load_skill_groups(section: dict | None) -> dict:
    """Normalize a recipes/deps section to {<id>: {<skill>: {rel: sha}}}."""
    result: dict = {}
    for owner_id, owner in (section or {}).items():
        skills = (owner or {}).get("skills") or {}
        result[owner_id] = {
            skill_name: dict(files) for skill_name, files in skills.items()
        }
    return result


def write_lock(lock_path: Path, lock: dict) -> None:
    out = [LOCK_HEADER]

    meta = lock.get("meta") or {}
    if meta:
        out.append("[meta]")
        if meta.get("cli_version"):
            out.append(f'cli_version = "{meta["cli_version"]}"')
        if meta.get("synced_at"):
            out.append(f'synced_at = "{meta["synced_at"]}"')
        out.append("")

    # Skill/recipe/dep content hashes are intentionally NOT serialized: the lock
    # is a provenance stamp (see LOCK_HEADER), not an integrity manifest. git
    # covers integrity/diff for the committed surface, and bundled/recipe skills
    # resolve from the CLI cache. Any legacy [skills.*]/[recipes.*]/[deps.*]/
    # [commands]/[opted-out] sections loaded from an older lock are dropped
    # here on the next write — commands no longer materialize in-project, so
    # no per-file hash or delete-memory bookkeeping is needed for them either.
    agents = lock.get("agents") or {}
    for harness in sorted(agents):
        files = agents[harness]
        if not files:
            continue
        out.append(f'[agents."{harness}"]')
        for name in sorted(files):
            out.append(f'"{name}" = "{files[name]}"')
        out.append("")

    lock_path.parent.mkdir(parents=True, exist_ok=True)
    lock_path.write_text("\n".join(out).rstrip("\n") + "\n")


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
