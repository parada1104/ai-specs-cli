"""Shared helpers for locating recipe origin under the CLI project cache."""

from __future__ import annotations

import hashlib
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
_BASENAME_SAFE = re.compile(r"[^A-Za-z0-9._-]+")


def _cache_key(project_root: Path) -> str:
    resolved = project_root.resolve()
    basename = _BASENAME_SAFE.sub("-", resolved.name).strip("-._") or "project"
    digest = hashlib.sha256(str(resolved).encode("utf-8")).hexdigest()[:12]
    return f"{digest}-{basename}"


def _cache_root(project_root: Path, cli_home: Path) -> Path:
    return (cli_home.resolve() / "cache" / "projects" / _cache_key(project_root)).resolve()


def recipe_skill_dir(project_root: Path, recipe_id: str, skill_id: str, cli_home: Path | None = None) -> Path:
    home = ROOT if cli_home is None else cli_home
    return _cache_root(project_root, home) / ".recipe" / recipe_id / "skills" / skill_id


def recipe_root(project_root: Path, recipe_id: str, cli_home: Path | None = None) -> Path:
    home = ROOT if cli_home is None else cli_home
    return _cache_root(project_root, home) / ".recipe" / recipe_id


def cache_command(project_root: Path, cmd_id: str, cli_home: Path | None = None) -> Path:
    home = ROOT if cli_home is None else cli_home
    return _cache_root(project_root, home) / "commands" / f"{cmd_id}.md"


def resolved_skills_dir(project_root: Path, cli_home: Path | None = None) -> Path:
    home = ROOT if cli_home is None else cli_home
    return _cache_root(project_root, home) / "resolved-skills"


def deps_skill_dir(project_root: Path, dep_id: str, skill_id: str | None = None, cli_home: Path | None = None) -> Path:
    home = ROOT if cli_home is None else cli_home
    sid = dep_id if skill_id is None else skill_id
    return _cache_root(project_root, home) / ".deps" / dep_id / "skills" / sid


def inproject_deps_skill_dir(project_root: Path, dep_id: str, skill_id: str | None = None) -> Path:
    sid = dep_id if skill_id is None else skill_id
    return project_root / "ai-specs" / ".deps" / dep_id / "skills" / sid
