"""Archive-aware resolution of OpenSpec change artifacts.

A change folder lives at ``openspec/changes/<slug>/`` while active and moves to
``openspec/changes/archive/<ISO-date>-<slug>/`` once it is archived, with the
legacy undated form ``openspec/changes/archive/<slug>/`` still supported. A guard
that hardcodes the active path therefore breaks the moment its change is
archived. These helpers resolve a slug across all of those layouts, and fall back
to an active-shaped path so callers keep an informative failure instead of a
crash when nothing exists yet.
"""

from __future__ import annotations

import re
from datetime import date
from pathlib import Path

_ISO_DATE_PREFIX = re.compile(r"^\d{4}-\d{2}-\d{2}$")


def _is_iso_calendar_date(value: str) -> bool:
    """True only for a real ISO calendar date such as ``2026-09-07``.

    Shape alone is not enough: ``2026-99-99`` matches the pattern but sorts above every
    real date, so a malformed folder would shadow the genuine archive entry.
    """
    if not _ISO_DATE_PREFIX.match(value):
        return False
    try:
        date.fromisoformat(value)
    except ValueError:
        return False
    return True


def _dated_archive_entries(archive_root: Path, slug: str) -> list[Path]:
    """Archive entries named ``<ISO-date>-<slug>``, ordered oldest to newest."""
    suffix = f"-{slug}"
    matches: list[Path] = []
    try:
        entries = list(archive_root.iterdir())
    except OSError:
        return matches
    for entry in entries:
        if not entry.is_dir() or not entry.name.endswith(suffix):
            continue
        date_prefix = entry.name[: -len(suffix)]
        if _is_iso_calendar_date(date_prefix):
            matches.append(entry)
    matches.sort(key=lambda path: path.name)
    return matches


def change_dir(root: Path, slug: str) -> Path:
    """Resolve the folder holding a change's artifacts.

    Resolution order: active ``changes/<slug>/``, then the most recent dated
    archive entry, then the legacy undated archive entry, then an active-shaped
    fallback. Never raises when ``openspec/``, ``changes/``, or ``archive/`` is
    missing.
    """
    changes_root = Path(root) / "openspec" / "changes"
    active = changes_root / slug
    if active.is_dir():
        return active

    archive_root = changes_root / "archive"
    dated = _dated_archive_entries(archive_root, slug)
    if dated:
        return dated[-1]

    legacy = archive_root / slug
    if legacy.is_dir():
        return legacy

    return active


def change_artifact(root: Path, slug: str, *parts: str) -> Path:
    """Resolve a specific artifact inside a change folder."""
    return change_dir(root, slug).joinpath(*parts)
