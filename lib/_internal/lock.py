#!/usr/bin/env python3
"""Shared lock file read/write helper for ai-specs."""

from __future__ import annotations

import hashlib
import os
import tempfile
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

    text = "\n".join(out).rstrip("\n") + "\n"
    lock_path.parent.mkdir(parents=True, exist_ok=True)
    # Atomic replace: a failed write never leaves a partially updated lock, so
    # refresh rollback can promise all-or-nothing lock state.
    fd, tmp = tempfile.mkstemp(
        dir=str(lock_path.parent), prefix=".ai-specs.lock.", suffix=".tmp"
    )
    try:
        with os.fdopen(fd, "w") as fh:
            fh.write(text)
        os.replace(tmp, lock_path)
    except BaseException:
        try:
            os.unlink(tmp)
        except OSError:
            pass
        raise


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


def set_gate_baseline(
    lock: dict,
    path: str,
    sha256: str,
    *,
    recipe: str | None = None,
    source: str | None = None,
) -> None:
    """Record the last CLI-rendered bytes for a generated runtime hook (gate).

    Gates follow the ``auto`` update policy: a baseline matching current bytes
    is treated as unmodified and may be force-updated; a mismatch or missing
    baseline is preserved with a warning.
    """
    set_managed_override(
        lock,
        path,
        sha256,
        recipe=recipe,
        source=source,
        kind="gate",
        policy="auto",
    )


def set_brief_baseline(lock: dict, path: str, sha256: str) -> None:
    """Record the last CLI-rendered bytes for the runtime brief.

    Runtime briefs are never force-refreshed after a user edit: the baseline
    is provenance only, and the renderer uses the ``never-force`` policy.
    """
    set_managed_override(
        lock,
        path,
        sha256,
        kind="runtime-brief",
        policy="never-force",
    )


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
