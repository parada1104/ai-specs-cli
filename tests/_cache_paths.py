"""Shared helpers for locating recipe origin under the CLI project cache."""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def _load_project_cache():
    path = ROOT / "lib" / "_internal" / "project-cache.py"
    name = "project_cache_test_helpers"
    if name in sys.modules:
        return sys.modules[name]
    internal = str(path.parent)
    if internal not in sys.path:
        sys.path.insert(0, internal)
    spec = importlib.util.spec_from_file_location(name, path)
    mod = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = mod
    spec.loader.exec_module(mod)
    return mod


def recipe_skill_dir(
    project_root: Path,
    recipe_id: str,
    skill_id: str,
    cli_home: Path | None = None,
) -> Path:
    home = ROOT if cli_home is None else cli_home
    pc = _load_project_cache()
    return pc.recipe_skills_root(project_root, cli_home=home) / recipe_id / "skills" / skill_id


def recipe_root(
    project_root: Path,
    recipe_id: str,
    cli_home: Path | None = None,
) -> Path:
    home = ROOT if cli_home is None else cli_home
    pc = _load_project_cache()
    return pc.recipe_skills_root(project_root, cli_home=home) / recipe_id


def cache_command(
    project_root: Path,
    cmd_id: str,
    cli_home: Path | None = None,
) -> Path:
    home = ROOT if cli_home is None else cli_home
    pc = _load_project_cache()
    return pc.commands_dir(project_root, cli_home=home) / f"{cmd_id}.md"


def resolved_skills_dir(project_root: Path, cli_home: Path | None = None) -> Path:
    home = ROOT if cli_home is None else cli_home
    pc = _load_project_cache()
    return pc.resolved_skills_dir(project_root, cli_home=home)


def deps_skill_dir(
    project_root: Path,
    dep_id: str,
    skill_id: str | None = None,
    cli_home: Path | None = None,
) -> Path:
    home = ROOT if cli_home is None else cli_home
    sid = dep_id if skill_id is None else skill_id
    pc = _load_project_cache()
    return pc.deps_skills_root(project_root, cli_home=home) / dep_id / "skills" / sid
