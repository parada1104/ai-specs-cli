#!/usr/bin/env python3
"""Pre-merge guardian: archive + tier-minimum planning artifacts.

Used by agents (and tests) before merging a PR/MR. Hard-stops when the change
folder is still active or the archived folder lacks the tier minimum files.
"""

from __future__ import annotations

import argparse
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Literal


Tier = Literal["light", "standard", "full"]

DEPTH_RE = re.compile(r"(?im)^\s*Depth:\s*(light|standard|full)\s*$")


@dataclass
class GuardianResult:
    ok: bool
    blockers: list[str] = field(default_factory=list)
    tier: str | None = None
    archive_path: Path | None = None


def infer_tier(tasks_text: str, fallback: Tier = "standard") -> Tier:
    match = DEPTH_RE.search(tasks_text)
    if not match:
        return fallback
    return match.group(1).lower()  # type: ignore[return-value]


def _has_spec_delta(archive: Path) -> bool:
    specs = archive / "specs"
    if not specs.is_dir():
        return False
    return any(specs.rglob("*.md"))


def check_premerge(
    repo_root: Path | str,
    slug: str,
    *,
    tier: Tier | str | None = "standard",
) -> GuardianResult:
    """Return ok=False with blockers when merge must not proceed."""
    root = Path(repo_root)
    changes = root / "openspec" / "changes"
    active = changes / slug
    archive = changes / "archive" / slug
    blockers: list[str] = []

    if active.is_dir():
        blockers.append(
            f"active change folder still present at openspec/changes/{slug}/ — "
            "run archive-tail on the review branch before merge"
        )

    if not archive.is_dir():
        blockers.append(
            f"missing archive at openspec/changes/archive/{slug}/ — "
            "archive planning artifacts on the review branch before merge"
        )
        return GuardianResult(ok=False, blockers=blockers, tier=str(tier) if tier else None)

    tasks = archive / "tasks.md"
    if not tasks.is_file():
        blockers.append(f"archive/{slug}/tasks.md is required for all tiers")
        resolved: Tier | None = None
    else:
        tasks_text = tasks.read_text(encoding="utf-8", errors="replace")
        if tier is None:
            resolved = infer_tier(tasks_text)
        else:
            resolved = str(tier).lower()  # type: ignore[assignment]
            if resolved not in {"light", "standard", "full"}:
                blockers.append(f"unknown tier '{tier}' (expected light|standard|full)")
                resolved = infer_tier(tasks_text)

        if resolved in {"standard", "full"} and not _has_spec_delta(archive):
            blockers.append(
                f"archive/{slug}/specs/ must contain at least one *.md for tier {resolved}"
            )

        if resolved == "full":
            if not (archive / "proposal.md").is_file() and not (archive / "design.md").is_file():
                blockers.append(
                    f"archive/{slug}/ must include proposal.md or design.md for tier full"
                )

    return GuardianResult(
        ok=not blockers,
        blockers=blockers,
        tier=resolved if tasks.is_file() else (str(tier) if tier else None),
        archive_path=archive,
    )


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Check pre-merge archive/artifact gates")
    parser.add_argument("slug", help="change slug under openspec/changes/")
    parser.add_argument(
        "--root",
        type=Path,
        default=Path.cwd(),
        help="repository root (default: cwd)",
    )
    parser.add_argument(
        "--tier",
        choices=["light", "standard", "full"],
        default=None,
        help="planning depth (default: infer from tasks.md Depth: line)",
    )
    args = parser.parse_args(argv)
    result = check_premerge(args.root, args.slug, tier=args.tier)
    if result.ok:
        print(f"premerge-guardian: OK ({result.tier})")
        return 0
    print("premerge-guardian: BLOCKED", file=sys.stderr)
    for blocker in result.blockers:
        print(f"  - {blocker}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
