#!/usr/bin/env python3
"""Ledger evidence bridge: acquisition and JSON only, never a grader.

Builds the local-facts half of the ledger's four-side evidence model for the
checkpoint hosts (``tracker-card-gate.sh``, ``tracker_ledger_host.py``). This slice
produces three sides (L3):

  ``local``   always empty from here — the Go ledger uses its own store snapshot
  ``remote``  always empty — no producer in this slice (no tracker MCP read)
  ``code``    the change's ``## Tracker`` ``card_id``, or "" under ``tracker.none``
  ``git``     that same native id only when a ``pr:`` is recorded, else ""

Everything here fails open: a missing artifact, an unreadable file, a non-Git
root, or a malformed section yields an empty side rather than an exception, so a
checkpoint never blocks because acquisition failed. The ``## Tracker`` validity
rule has exactly one authoritative grader — the Go ``--ledger`` predicate — and
``trello_link.parse_tracker_section`` stays the only parser (A8/D5).

The domain is Tracker, not a provider. Provider adaptation is the declarative
``[config.reconcile]`` mapping the Go core reads from the manifest; this bridge
supplies only neutral local facts and never grades or names a provider.

No ``gh``, no MCP, no network: the only subprocess is a local ``git rev-parse``
used to locate the durable witness / store directory.
"""
from __future__ import annotations

import importlib.util
import json
import re
import subprocess
import sys
from pathlib import Path

# The legacy literal: the recipe id every bound project used before the witness
# existed. Used as the fallback so behavior is identical when no witness resolves.
LEGACY_RECIPE_ID = "trello-mcp-workflow"

# The exemption file: the human-authored act of exempting one change (L1/DW1).
TRACKER_NONE = "tracker.none"
TRACKER_NONE_REASON_FALLBACK = "tracker.none"

_ISO_DATE_RE = re.compile(r"^\d{4}-\d{2}-\d{2}$")
# A change slug is one plain, relative path segment — the only shape that may be
# joined to the change root. An absolute path, a separator, a traversal segment, or
# surrounding whitespace would let a caller acquire an artifact outside the planning
# tree and hand it back as this repository's evidence (R3).
_SLUG_SEGMENT_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*$")


def _load_trello_link():
    """Load the sibling ``trello_link.py`` without depending on sys.path.

    Same cold-cache-safe pattern the guardian uses for ``gate_binary.py``: a CLI
    install has no project cache and no guaranteed ``PYTHONPATH``.
    """
    path = Path(__file__).with_name("trello_link.py")
    spec = importlib.util.spec_from_file_location("ledger_bridge_trello_link", path)
    if spec is None or spec.loader is None:
        raise ImportError(f"cannot load {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


parse_tracker_section = _load_trello_link().parse_tracker_section


def _git_common_dir(root: Path | str) -> Path | None:
    """Resolve the owning repository's Git common dir, or ``None``.

    The common dir — not ``<root>/.git`` — is authoritative: a linked worktree
    keeps its witness and store in the main checkout's common dir.
    """
    try:
        proc = subprocess.run(
            ["git", "-C", str(root), "rev-parse", "--path-format=absolute", "--git-common-dir"],
            capture_output=True, text=True, timeout=10,
        )
    except Exception:
        return None
    if proc.returncode != 0:
        return None
    raw = proc.stdout.strip()
    if not raw:
        return None
    candidate = Path(raw)
    if not candidate.is_absolute():
        candidate = Path(root) / candidate
    try:
        return candidate.resolve()
    except OSError:
        return candidate


def recipe_id(root: Path | str) -> str:
    """Read the bound recipe id from the durable witness, with the literal fallback.

    Reading the witness is acquisition, never grading (A8). A missing, corrupt,
    unknown-version, wrong-capability, or id-less witness yields the legacy literal,
    so hosts behave exactly as they do today when no witness resolves.
    """
    common = _git_common_dir(root)
    if common is None:
        return LEGACY_RECIPE_ID
    try:
        raw = (common / "ai-specs" / "ledger" / "witness.json").read_text(encoding="utf-8")
        data = json.loads(raw)
    except Exception:
        return LEGACY_RECIPE_ID
    if not isinstance(data, dict):
        return LEGACY_RECIPE_ID
    if data.get("v") != 1 or data.get("capability") != "tracker":
        return LEGACY_RECIPE_ID
    bound = data.get("recipe_id")
    if isinstance(bound, str) and bound.strip():
        return bound.strip()
    return LEGACY_RECIPE_ID


def change_slug(root: Path | str) -> str:
    """The single active change slug under ``openspec/changes/``, else "".

    Two active changes are ambiguous, so the host must not guess which change a
    ``tracker.none`` file belongs to — exactly the identity rule the Go ledger
    applies (A11).
    """
    try:
        entries = sorted(
            entry.name
            for entry in (Path(root) / "openspec" / "changes").iterdir()
            if entry.is_dir() and entry.name != "archive"
        )
    except OSError:
        return ""
    if len(entries) == 1:
        return entries[0]
    return ""


def change_dir(root: Path | str, slug: str | None) -> Path | None:
    """Resolve a validated change folder active-or-archived, or ``None``.

    Order: active ``changes/<slug>/``, then the newest dated archive entry
    ``<ISO-date>-<slug>/``, then the legacy undated ``archive/<slug>/``, then an
    active-shaped fallback for a change that does not exist yet. Pre-merge grades an
    archived change, so the code side must still resolve there.

    Every join is guarded twice: the slug must be one plain relative segment, and
    each candidate must still resolve inside ``<root>/openspec/changes`` once
    symlinks are followed. A traversal or absolute slug, and a symlink pointing
    outside the planning tree, therefore yield ``None`` instead of a path that would
    borrow an artifact from outside the repository (R3).
    """
    changes = Path(root) / "openspec" / "changes"
    if not valid_slug(slug):
        return None
    # refused tracks a candidate that exists only as a link out of the planning tree:
    # such a slug resolves to nothing rather than falling back to another branch.
    refused = False
    active = changes / slug
    if active.is_dir():
        if _inside_tree(changes, active):
            return active
        refused = True
    archive = changes / "archive"
    dated: list[Path] = []
    try:
        for entry in archive.iterdir():
            suffix = f"-{slug}"
            if not entry.is_dir() or not entry.name.endswith(suffix):
                continue
            date_prefix = entry.name[: -len(suffix)]
            if not _ISO_DATE_RE.match(date_prefix):
                continue
            try:
                from datetime import date as _date

                _date.fromisoformat(date_prefix)
            except ValueError:
                continue
            if not _inside_tree(changes, entry):
                refused = True
                continue
            dated.append(entry)
    except OSError:
        dated = []
    if dated:
        return sorted(dated, key=lambda path: path.name)[-1]
    legacy = archive / slug
    if legacy.is_dir():
        if _inside_tree(changes, legacy):
            return legacy
        refused = True
    if refused:
        # Every candidate that existed escaped the tree: refuse the slug rather than
        # hand a path outside the planning tree back to a caller.
        return None
    return active


def valid_slug(slug: str | None) -> bool:
    """True when ``slug`` is exactly one plain, relative path segment."""
    if not isinstance(slug, str) or not slug or slug != slug.strip():
        return False
    if slug in (".", ".."):
        return False
    return bool(_SLUG_SEGMENT_RE.match(slug))


def _inside_tree(base: Path, candidate: Path) -> bool:
    """True when ``candidate`` resolves to a real path strictly inside ``base``.

    ``resolve()`` follows symlinks, so an in-tree symlink still resolves while a link
    whose target lives outside the planning tree is refused.
    """
    try:
        base_real = base.resolve()
        candidate_real = candidate.resolve()
    except OSError:
        return False
    return candidate_real != base_real and base_real in candidate_real.parents


def tracker_none_reason(root: Path | str, slug: str | None) -> str | None:
    """The persisted exemption reason, or ``None`` when the file is absent.

    ``None`` means "no exemption authored"; a string means the human act is present
    and the evidence side must stay blank. Reading the file is acquisition only: the
    presence of the file never records an exemption by itself, and no host may turn
    it into an exempt write on its own (R1). The file itself is never created,
    modified, or deleted here (DW1).
    """
    folder = change_dir(root, slug)
    if folder is None or not folder.is_dir():
        return None
    path = folder / TRACKER_NONE
    try:
        text = path.read_text(encoding="utf-8")
    except OSError:
        return None
    for line in text.splitlines():
        stripped = line.strip()
        if stripped:
            return stripped
    return TRACKER_NONE_REASON_FALLBACK


def evidence_payload(root: Path | str, slug: str | None) -> dict[str, str]:
    """Build the ``--evidence`` JSON object from local facts only.

    ``local`` and ``remote`` are always empty: the ledger owns its own local
    snapshot and this slice has no remote producer (L3). ``code`` is the change's
    ``## Tracker`` ``card_id``, blanked when ``tracker.none`` exempts the change so
    an exemption cannot invent a conflict. ``git`` carries that same native id when
    a ``pr:`` is recorded — branch names and PR URLs are deliberately NOT used,
    because the Go conflict predicate equality-compares every non-empty side.
    """
    empty = {"local": "", "remote": "", "code": "", "git": ""}
    if not slug:
        return empty
    if tracker_none_reason(root, slug) is not None:
        return empty
    folder = change_dir(root, slug)
    if folder is None:
        return empty
    fields = parse_tracker_section([folder / "proposal.md", folder / "tasks.md"])
    card = (fields.get("card_id") or "").strip()
    if not card:
        return empty
    return {
        "local": "",
        "remote": "",
        "code": card,
        "git": card if (fields.get("pr") or "").strip() else "",
    }
