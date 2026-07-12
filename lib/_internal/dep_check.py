#!/usr/bin/env python3
"""CLI dependency checks for recipe.toml [[deps.cli]] declarations.

Guidance-only: never installs binaries. Existence via shutil.which;
optional version_check runs under a shell with a 5s timeout.
"""
from __future__ import annotations

import importlib.util
import os
import re
import shutil
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

_VERSION_RE = re.compile(r"\d+(?:\.\d+)*")


def _load_recipe_schema():
    path = Path(__file__).with_name("recipe_schema.py")
    spec = importlib.util.spec_from_file_location("recipe_schema", path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load recipe_schema.py at {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


_recipe_schema = _load_recipe_schema()
CliDep = _recipe_schema.CliDep
Recipe = _recipe_schema.Recipe


@dataclass
class DepResult:
    binary: str
    found: bool
    version: str  # "" if unknown/not run
    ok: bool  # found AND (min_version satisfied or no min_version)
    install_url: str
    purpose: str
    required: bool
    recipe_id: str = ""  # populated by check_project_deps
    detail: str = ""  # human note, e.g. "found 1.9.0 < required 2.0.0"


def _load_sibling(name: str):
    """Load a same-directory _internal module by absolute path."""
    path = Path(__file__).with_name(f"{name}.py")
    spec = importlib.util.spec_from_file_location(name.replace("-", "_"), path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load sibling module {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def _catalog_dir() -> Path:
    home = os.environ.get("AI_SPECS_HOME")
    root = Path(home) if home else Path(__file__).resolve().parents[2]
    return root / "catalog" / "recipes"


def _which(binary: str) -> bool:
    return shutil.which(binary) is not None


def _run_version_check(cmd: str) -> str:
    try:
        proc = subprocess.run(
            cmd,
            shell=True,
            capture_output=True,
            text=True,
            timeout=5,
        )
        return (proc.stdout or "") + (proc.stderr or "")
    except Exception:
        return ""


def _parse_version(text: str) -> tuple[int, ...]:
    match = _VERSION_RE.search(text or "")
    if not match:
        return ()
    return tuple(int(part) for part in match.group(0).split("."))


def _version_ge(have: tuple[int, ...], want: tuple[int, ...]) -> bool:
    if not want:
        return True
    if not have:
        return False
    width = max(len(have), len(want))
    have_p = have + (0,) * (width - len(have))
    want_p = want + (0,) * (width - len(want))
    return have_p >= want_p


def _check_one(dep: CliDep, recipe_id: str = "") -> DepResult:
    found = _which(dep.binary)
    if not found:
        return DepResult(
            binary=dep.binary,
            found=False,
            version="",
            ok=False,
            install_url=dep.install_url,
            purpose=dep.purpose,
            required=dep.required,
            recipe_id=recipe_id,
            detail="not found on PATH",
        )

    version = ""
    detail = ""
    if dep.version_check:
        try:
            raw = _run_version_check(dep.version_check)
        except Exception:
            raw = ""
        parsed = _parse_version(raw)
        if parsed:
            version = ".".join(str(p) for p in parsed)
        if dep.min_version:
            want = _parse_version(dep.min_version)
            if not parsed:
                return DepResult(
                    binary=dep.binary,
                    found=True,
                    version=version,
                    ok=True,
                    install_url=dep.install_url,
                    purpose=dep.purpose,
                    required=dep.required,
                    recipe_id=recipe_id,
                    detail="version unknown",
                )
            if _version_ge(parsed, want):
                return DepResult(
                    binary=dep.binary,
                    found=True,
                    version=version,
                    ok=True,
                    install_url=dep.install_url,
                    purpose=dep.purpose,
                    required=dep.required,
                    recipe_id=recipe_id,
                    detail=detail,
                )
            return DepResult(
                binary=dep.binary,
                found=True,
                version=version,
                ok=False,
                install_url=dep.install_url,
                purpose=dep.purpose,
                required=dep.required,
                recipe_id=recipe_id,
                detail=f"found {version} < required {dep.min_version}",
            )

    return DepResult(
        binary=dep.binary,
        found=True,
        version=version,
        ok=True,
        install_url=dep.install_url,
        purpose=dep.purpose,
        required=dep.required,
        recipe_id=recipe_id,
        detail=detail,
    )


def check_cli_deps(recipe: Recipe) -> list[DepResult]:
    """One DepResult per recipe.cli_deps entry. Never raises."""
    results: list[DepResult] = []
    for dep in recipe.cli_deps:
        try:
            results.append(_check_one(dep, recipe_id=getattr(recipe, "id", "") or ""))
        except Exception:
            results.append(
                DepResult(
                    binary=getattr(dep, "binary", "?"),
                    found=False,
                    version="",
                    ok=False,
                    install_url=getattr(dep, "install_url", ""),
                    purpose=getattr(dep, "purpose", ""),
                    required=bool(getattr(dep, "required", True)),
                    recipe_id=getattr(recipe, "id", "") or "",
                    detail="check failed",
                )
            )
    return results


def check_project_deps(project_root: Path) -> list[DepResult]:
    """Aggregate CLI dep checks across enabled recipes in the project manifest."""
    toml_read = _load_sibling("toml-read")
    recipe_read = _load_sibling("recipe-read")
    manifest = project_root / "ai-specs" / "ai-specs.toml"
    if not manifest.is_file():
        return []
    try:
        data = toml_read.load_toml(manifest)
        recipes = toml_read.read_recipes(data)
    except Exception:
        return []

    catalog = _catalog_dir()
    out: list[DepResult] = []
    for recipe_id, entry in recipes.items():
        if not entry.get("enabled"):
            continue
        try:
            recipe = recipe_read.read_recipe(catalog, recipe_id)
        except Exception:
            continue
        for result in check_cli_deps(recipe):
            result.recipe_id = recipe_id
            out.append(result)
    return out
