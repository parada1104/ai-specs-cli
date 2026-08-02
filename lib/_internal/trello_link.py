#!/usr/bin/env python3
"""Canonical ## Tracker link-section parser for active OpenSpec changes.

Shared validity predicate used by doctor and (via an embedded twin) the
tracker-card-gate hook. Validity = non-empty card_id in a ## Tracker section
of proposal.md (fallback tasks.md). card_id shape is not required for
validity; card_id_looks_canonical() is an INFO-nudge helper only.
"""
from __future__ import annotations

import re
from pathlib import Path

_PAIR_RE = re.compile(
    r"^\s*(?:[-*]\s+)?\*{0,2}(?P<key>[A-Za-z_][A-Za-z0-9_]*)\*{0,2}\s*:\s*(?P<value>.*)$"
)
_RECOGNIZED_KEYS = frozenset({"card_id", "shortlink", "url", "list", "pr"})
_CANONICAL_CARD_ID_RE = re.compile(r"^[0-9a-fA-F]{24}$")
_TRACKER_HEADING_RE = re.compile(r"^##\s+Tracker\s*$")
_H2_RE = re.compile(r"^##\s+")


def _clean_value(raw: str) -> str:
    """Strip surrounding backticks/whitespace and a trailing ' #comment'."""
    value = raw.strip()
    # Trailing markdown comment outside backticks: space + hash…
    hash_at = value.find(" #")
    if hash_at != -1:
        # Only strip when the hash is outside a single pair of backticks.
        before = value[:hash_at]
        ticks = before.count("`")
        if ticks % 2 == 0:
            value = before.strip()
    if len(value) >= 2 and value.startswith("`") and value.endswith("`"):
        value = value[1:-1].strip()
    return value


def _extract_tracker_body(text: str) -> str | None:
    """Return body lines after ## Tracker until the next ## heading, or None."""
    lines = text.splitlines()
    start = None
    for i, line in enumerate(lines):
        if _TRACKER_HEADING_RE.match(line):
            start = i + 1
            break
    if start is None:
        return None
    body: list[str] = []
    for line in lines[start:]:
        if _H2_RE.match(line):
            break
        body.append(line)
    return "\n".join(body)


def _parse_section_body(body: str) -> dict[str, str]:
    out: dict[str, str] = {}
    for line in body.splitlines():
        if not line.strip():
            continue
        if line.lstrip().startswith("#"):
            continue
        m = _PAIR_RE.match(line)
        if not m:
            continue
        key = m.group("key").lower()
        if key not in _RECOGNIZED_KEYS:
            continue
        if key in out:
            continue  # first wins
        out[key] = _clean_value(m.group("value"))
    return out


def parse_tracker_section(artifact_paths: list[Path]) -> dict[str, str]:
    """Parse the first ## Tracker section found across artifact_paths.

    Returns {} if none of the paths exist/are readable or none contain a
    ## Tracker section. Keys are lowercased; recognized keys only.
    """
    for path in artifact_paths:
        try:
            if not path.is_file():
                continue
            text = path.read_text(encoding="utf-8")
        except OSError:
            continue
        body = _extract_tracker_body(text)
        if body is None:
            continue
        return _parse_section_body(body)
    return {}


# Tasks.md historical name; design locks parse_tracker_section.
parse_trello_md = parse_tracker_section


def is_valid_link(artifact_paths: list[Path]) -> bool:
    """True iff a ## Tracker section yields a non-empty card_id."""
    data = parse_tracker_section(artifact_paths)
    return bool(data.get("card_id"))


def card_id_looks_canonical(card_id: str) -> bool:
    """True only for a 24-char hex Trello card id (INFO nudge helper)."""
    return bool(card_id) and bool(_CANONICAL_CARD_ID_RE.fullmatch(card_id))
