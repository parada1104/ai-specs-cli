"""Helpers for locating / assembling recipe fixtures outside the shipped catalog.

Internal ``test-*`` recipes live under ``tests/fixtures/recipes/`` and MUST NOT
appear in ``catalog/recipes/``. Tests that need them as a CLI catalog should call
``cli_home_with_fixtures``.
"""

from __future__ import annotations

import os
import shutil
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PUBLIC_RECIPES = ROOT / "catalog" / "recipes"
FIXTURE_RECIPES = ROOT / "tests" / "fixtures" / "recipes"

# Opt-in for materialize tests that intentionally enable test-* fixtures.
ALLOW_INTERNAL_TEST_RECIPES_ENV = "AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES"


def unit_catalog() -> Path:
    """Catalog path for unit tests that only need fixture recipes."""
    return FIXTURE_RECIPES


def _link_or_copy(src: Path, dest: Path) -> None:
    dest.parent.mkdir(parents=True, exist_ok=True)
    if dest.exists() or dest.is_symlink():
        return
    try:
        os.symlink(src, dest, target_is_directory=src.is_dir())
    except OSError:
        if src.is_dir():
            shutil.copytree(src, dest)
        else:
            shutil.copy2(src, dest)


def populate_catalog(catalog_dir: Path, *, include_public: bool = True, include_fixtures: bool = True) -> None:
    """Populate ``catalog_dir`` with public and/or fixture recipe directories."""
    catalog_dir.mkdir(parents=True, exist_ok=True)
    sources: list[Path] = []
    if include_public and PUBLIC_RECIPES.is_dir():
        sources.append(PUBLIC_RECIPES)
    if include_fixtures and FIXTURE_RECIPES.is_dir():
        sources.append(FIXTURE_RECIPES)
    for src_root in sources:
        for child in sorted(src_root.iterdir()):
            if child.is_dir() and not child.name.startswith("."):
                _link_or_copy(child, catalog_dir / child.name)


def cli_home_with_fixtures(
    *,
    include_public: bool = True,
    include_fixtures: bool = True,
    cleanup_register=None,
) -> Path:
    """Temp AI_SPECS_HOME whose ``catalog/recipes`` merges public + fixture recipes.

    If ``cleanup_register`` is a callable (e.g. ``TestCase.addCleanup``), the
    temporary directory is registered for cleanup. Otherwise the caller owns it.
    """
    tmp = tempfile.TemporaryDirectory()
    if cleanup_register is not None:
        cleanup_register(tmp.cleanup)
    home = Path(tmp.name)
    # Keep TemporaryDirectory alive when not using cleanup_register by attaching
    # the object to the returned path.
    home._fixture_tmpdir = tmp  # type: ignore[attr-defined]
    populate_catalog(
        home / "catalog" / "recipes",
        include_public=include_public,
        include_fixtures=include_fixtures,
    )
    return home


def allow_internal_test_recipes_env() -> dict[str, str]:
    """Env fragment for subprocess/materialize calls that need test-* fixtures."""
    return {ALLOW_INTERNAL_TEST_RECIPES_ENV: "1"}
