#!/usr/bin/env python3
"""Pre-merge guardian for planning artifacts and staged verification evidence."""

from __future__ import annotations

import argparse
import re
import sys
from dataclasses import dataclass, field
from datetime import date
from pathlib import Path
from typing import Literal


Tier = Literal["light", "standard", "full"]
Stage = Literal["pre-archive", "pre-merge"]

# Keep this exact standalone-line contract: plan annotations must not suffix it.
DEPTH_RE = re.compile(r"(?im)^\s*Depth:\s*(light|standard|full)\s*$")
DATE_RE = re.compile(r"^\d{4}-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12]\d|3[01])$")
DATED_ARCHIVE_RE = re.compile(r"^(?P<date>\d{4}-\d{2}-\d{2})-(?P<suffix>.+)$")
SHA_RE = re.compile(r"^[0-9a-fA-F]{7,40}$")
FAILURE_VERDICTS = {"FAIL", "FAILED", "BLOCKED", "ERROR", "FAILURE"}

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


def _resolve_tier(tasks_text: str, tier: Tier | str | None, blockers: list[str]) -> Tier:
    if tier is None:
        return infer_tier(tasks_text)
    resolved = str(tier).lower()
    if resolved not in {"light", "standard", "full"}:
        blockers.append(f"unknown tier '{tier}' (expected light|standard|full)")
        return infer_tier(tasks_text)
    return resolved  # type: ignore[return-value]


def _has_spec_delta(folder: Path) -> bool:
    specs = folder / "specs"
    return specs.is_dir() and any(specs.rglob("*.md"))


def _check_minimums(folder: Path, tier: Tier, location: str) -> list[str]:
    blockers: list[str] = []
    if not (folder / "tasks.md").is_file():
        blockers.append(f"{location}/tasks.md is required for all tiers")
    if tier == "light":
        if not (folder / "proposal.md").is_file():
            blockers.append(f"{location}/proposal.md is required for tier light")
    elif tier == "standard":
        if not (folder / "proposal.md").is_file():
            blockers.append(f"{location}/proposal.md is required for tier standard")
        if not _has_spec_delta(folder):
            blockers.append(f"{location}/specs/ must contain at least one *.md for tier standard")
    else:
        if not (folder / "proposal.md").is_file() and not (folder / "design.md").is_file():
            blockers.append(f"{location}/proposal.md or design.md is required for tier full")
        if not _has_spec_delta(folder):
            blockers.append(f"{location}/specs/ must contain at least one *.md for tier full")
    return blockers


EVIDENCE_HEADING_RE = re.compile(r"^[ ]{0,3}##[ \t]+Verify evidence[ \t]*$", re.IGNORECASE)
SECTION_HEADING_RE = re.compile(r"^[ ]{0,3}#{1,6}(?:[ \t]+|$)")
MAPPING_HEADING_RE = re.compile(
    r"^[ ]{0,3}##[ \t]+Success-criteria mapping[ \t]*$", re.IGNORECASE
)
SUCCESS_CRITERIA_HEADING_RE = re.compile(
    r"^[ ]{0,3}##[ \t]+Success criteria[ \t]*$", re.IGNORECASE
)
CRITERION_ITEM_RE = re.compile(
    r"^(?:[-*+]|\d+[.)])[ \t]+(?:\[[ xX]\][ \t]*)?\S"
)
CRITERION_MAPPING_RE = re.compile(
    r"^[ ]{0,3}[-*][ \t]+Criterion[ \t]+(\d+)[ \t]*:[ \t]*([^\s—-]+)"
    r"(?:[ \t]+[—-].*)?[ \t]*$",
    re.IGNORECASE,
)
EVIDENCE_ALIASES = {
    "verdict": "verdict",
    "status": "verdict",
    "overall": "verdict",
    "command": "command",
    "exit": "exit",
    "exit code": "exit",
    "exit status": "exit",
    "date": "date",
    "commit": "commit",
    "sha": "commit",
    "revision": "commit",
    "ready_for_archive": "ready_for_archive",
}


def _parse_evidence_fields(lines: list[str]) -> tuple[dict[str, str], set[str]]:
    """Parse canonical evidence rows and report repeated field identities."""
    values: dict[str, str] = {}
    duplicates: set[str] = set()
    seen: set[str] = set()
    for raw_line in lines:
        line = raw_line.strip().strip("|").strip()
        line = re.sub(r"^[-*]\s+", "", line)
        line = line.replace("**", "").strip()
        if "|" in line:
            parts = [part.strip() for part in line.split("|")]
            if len(parts) < 2:
                continue
            label, value = parts[0], parts[1]
        elif ":" in line:
            label, value = line.split(":", 1)
            label, value = label.strip(), value.strip()
        else:
            continue
        key = EVIDENCE_ALIASES.get(label.lower().rstrip(";"))
        if not key:
            continue
        if key in seen:
            duplicates.add(key)
            continue
        seen.add(key)
        if value:
            values[key] = value.strip().strip("`").strip()
    return values, duplicates


def _markdown_content_lines(lines: list[str]) -> list[str]:
    """Return non-code Markdown lines so code blocks cannot forge evidence."""
    content: list[str] = []
    fence: tuple[str, int] | None = None
    for line in lines:
        if fence is not None:
            marker, width = fence
            if re.fullmatch(rf"[ ]{{0,3}}{re.escape(marker)}{{{width},}}[ \t]*", line):
                fence = None
            continue
        opening = re.match(r"^[ ]{0,3}(`{3,}|~{3,})", line)
        if opening:
            token = opening.group(1)
            fence = (token[0], len(token))
            continue
        if re.match(r"^(?: {4,}|\t)", line):
            continue
        content.append(line)
    return content


def _extract_evidence_fields(text: str) -> tuple[dict[str, str], list[str]]:
    """Extract fields only from one exact ``## Verify evidence`` section."""
    lines = _markdown_content_lines(text.splitlines())
    headings = [index for index, line in enumerate(lines) if EVIDENCE_HEADING_RE.fullmatch(line)]
    if not headings:
        return {}, ["verify-report.md is missing canonical ## Verify evidence block"]
    if len(headings) > 1:
        return {}, ["verify-report.md has duplicate canonical ## Verify evidence blocks"]

    start = headings[0] + 1
    end = len(lines)
    for index in range(start, len(lines)):
        if SECTION_HEADING_RE.match(lines[index]):
            end = index
            break
    values, duplicates = _parse_evidence_fields(lines[start:end])
    blockers = [
        f"verify-report.md has duplicate evidence label: {key}"
        for key in sorted(duplicates)
    ]
    return values, blockers
def _valid_date(value: str) -> bool:
    """Validate both the YYYY-MM-DD shape and the calendar date."""
    value = value.strip("`")
    if not DATE_RE.fullmatch(value):
        return False
    try:
        date.fromisoformat(value)
    except ValueError:
        return False
    return True


def _resolve_archive(archive_root: Path, slug: str) -> tuple[Path | None, list[str]]:
    """Resolve one exact dated archive, or the exact undated legacy fallback.

    Symlinked roots and candidates are rejected outright: ``is_dir()`` follows
    symlinks, so a dated archive symlink to content outside the planning tree
    could otherwise have its external files satisfy artifact/verification gates.
    """
    missing = (
        f"missing archive at openspec/changes/archive/{slug}/ or a valid dated "
        f"archive named YYYY-MM-DD-{slug}/"
    )
    symlink_blockers: list[str] = []
    if archive_root.is_symlink():
        symlink_blockers.append(
            f"archive root {archive_root} is a symlink and is rejected — "
            "the archive tree must be a real directory inside the planning tree"
        )
    if not archive_root.is_dir():
        return None, symlink_blockers or [missing]

    dated: list[Path] = []
    invalid_dates: list[Path] = []
    near_matches: list[Path] = []
    legacy = archive_root / slug

    if legacy.is_symlink():
        symlink_blockers.append(
            f"archive candidate {legacy} is a symlink and is rejected — "
            "an archive must be a real directory inside the planning tree"
        )

    for child in sorted(archive_root.iterdir(), key=lambda path: path.name):
        if child.name == slug:
            continue
        if child.is_symlink():
            match = DATED_ARCHIVE_RE.fullmatch(child.name)
            if match:
                suffix = match.group("suffix")
                # Scope dated symlink blockers to the requested slug, mirroring the
                # real-directory candidates: an unrelated dated symlink must not
                # poison resolution for this slug.
                if suffix == slug or suffix.startswith(f"{slug}-"):
                    symlink_blockers.append(
                        f"archive candidate {child} is a symlink and is rejected — "
                        "an archive must be a real directory inside the planning tree"
                    )
            continue
        if not child.is_dir():
            continue
        match = DATED_ARCHIVE_RE.fullmatch(child.name)
        if not match:
            continue
        suffix = match.group("suffix")
        if suffix == slug:
            if _valid_date(match.group("date")):
                dated.append(child)
            else:
                invalid_dates.append(child)
        elif suffix.startswith(f"{slug}-"):
            near_matches.append(child)

    blockers: list[str] = []
    for candidate in sorted(invalid_dates, key=lambda path: path.name):
        blockers.append(
            f"invalid dated archive candidate {candidate} — date prefix must be "
            "a valid ISO calendar date"
        )
    for candidate in sorted(near_matches, key=lambda path: path.name):
        blockers.append(
            f"near-match archive candidate {candidate} does not exactly match "
            f"YYYY-MM-DD-{slug}"
        )

    dated = sorted(dated, key=lambda path: path.name)
    if len(dated) > 1 or (dated and legacy.is_dir()):
        candidates = dated + ([legacy] if legacy.is_dir() else [])
        blockers.append(
            f"ambiguous archives for slug '{slug}': "
            f"{', '.join(str(path) for path in candidates)}"
        )

    if symlink_blockers:
        return None, symlink_blockers + blockers
    if blockers:
        return None, blockers
    if len(dated) == 1:
        return dated[0], []
    if legacy.is_dir():
        return legacy, []
    return None, [missing]


def _parse_success_criteria(folder: Path) -> tuple[tuple[int, str] | None, str | None]:
    """Parse criteria from exactly one authoritative planning artifact.

    ``proposal.md`` is authoritative whenever it exists.  ``design.md`` is a
    fallback only when proposal.md is absent, never when proposal criteria are
    missing or empty.  Duplicate headings are rejected because their meaning
    cannot be selected safely or deterministically.
    """
    proposal = folder / "proposal.md"
    if proposal.is_file():
        source = proposal
    else:
        design = folder / "design.md"
        if not design.is_file():
            return None, (
                "verify-report.md Full mapping requires a non-empty ## Success Criteria "
                "section in proposal.md or design.md"
            )
        source = design

    filename = source.name
    lines = _markdown_content_lines(
        source.read_text(encoding="utf-8", errors="replace").splitlines()
    )
    headings = [
        index for index, line in enumerate(lines)
        if SUCCESS_CRITERIA_HEADING_RE.fullmatch(line)
    ]
    if len(headings) > 1:
        return None, f"verify-report.md has duplicate ## Success Criteria headings in {filename}"
    if not headings:
        return None, f"verify-report.md requires a ## Success Criteria section in {filename}"

    heading = headings[0]
    end = len(lines)
    for index in range(heading + 1, len(lines)):
        if SECTION_HEADING_RE.match(lines[index]):
            end = index
            break
    count = sum(1 for line in lines[heading + 1:end] if CRITERION_ITEM_RE.match(line))
    if not count:
        return None, f"verify-report.md requires a non-empty ## Success Criteria section in {filename}"
    return (count, filename), None


def _success_criteria(folder: Path) -> tuple[int, str] | None:
    """Return the count of top-level criteria from the authoritative artifact."""
    criteria, _ = _parse_success_criteria(folder)
    return criteria


def _check_full_criteria_mapping(folder: Path, report_text: str) -> list[str]:
    """Require one strict PASS mapping row for every declared success criterion."""
    criteria, issue = _parse_success_criteria(folder)
    if criteria is None:
        return [issue or "verify-report.md could not parse Success Criteria"]
    expected_count, source = criteria
    lines = _markdown_content_lines(report_text.splitlines())
    headings = [index for index, line in enumerate(lines) if MAPPING_HEADING_RE.fullmatch(line)]
    if not headings:
        return ["verify-report.md Full evidence requires canonical ## Success-criteria mapping block"]
    if len(headings) > 1:
        return ["verify-report.md has duplicate canonical ## Success-criteria mapping blocks"]

    start = headings[0] + 1
    end = len(lines)
    for index in range(start, len(lines)):
        if SECTION_HEADING_RE.match(lines[index]):
            end = index
            break

    blockers: list[str] = []
    seen: set[int] = set()
    for line in lines[start:end]:
        match = CRITERION_MAPPING_RE.fullmatch(line)
        if not match:
            continue
        number = int(match.group(1))
        status = match.group(2).upper()
        if number in seen:
            blockers.append(f"verify-report.md has duplicate success-criteria mapping for criterion {number}")
            continue
        seen.add(number)
        if number < 1 or number > expected_count:
            blockers.append(
                f"verify-report.md maps unknown success criterion {number} "
                f"(expected 1-{expected_count} from {source})"
            )
        elif status != "PASS":
            blockers.append(f"verify-report.md criterion {number} mapping must be strict PASS for tier full")

    for number in range(1, expected_count + 1):
        if number not in seen:
            blockers.append(f"verify-report.md is missing success-criteria mapping for criterion {number}")
    return blockers



def _field_values(text: str) -> dict[str, str]:
    """Read labelled evidence fields from the canonical evidence block only."""
    values, _ = _extract_evidence_fields(text)
    return values


def check_verify_evidence(change_dir: Path, tier: Tier | str) -> list[str]:
    """Return staged verify blockers; Light is always advisory."""
    resolved = str(tier).lower()
    if resolved == "light":
        return []
    report = Path(change_dir) / "verify-report.md"
    if not report.is_file():
        return [f"{resolved} verify evidence requires dedicated verify-report.md"]

    values, blockers = _extract_evidence_fields(report.read_text(encoding="utf-8", errors="replace"))
    verdict = values.get("verdict", "")
    verdict_token = verdict.split()[0].upper().strip("`.,;:()") if verdict else ""
    if not verdict:
        blockers.append("verify-report.md is missing Verdict/Status/Overall")
    elif resolved == "full" and verdict_token != "PASS":
        blockers.append("verify-report.md Verdict must be strict PASS for tier full")
    elif resolved == "standard" and verdict_token in FAILURE_VERDICTS:
        blockers.append("verify-report.md Verdict must not be FAIL or BLOCKED for tier standard")

    if not values.get("command"):
        blockers.append("verify-report.md is missing Command")
    exit_value = values.get("exit", "").split()
    if not exit_value or exit_value[0].strip("`.,;:") != "0":
        blockers.append("verify-report.md Exit/Exit code/Exit status must be 0")
    if not _valid_date(values.get("date", "")):
        blockers.append("verify-report.md is missing Date in valid YYYY-MM-DD format")
    if not SHA_RE.fullmatch(values.get("commit", "").strip("`")):
        blockers.append("verify-report.md is missing Commit/SHA/Revision (7-40 hex)")
    if resolved == "full" and values.get("ready_for_archive", "").lower() != "true":
        blockers.append("verify-report.md requires ready_for_archive: true for tier full")
    if resolved == "full":
        blockers.extend(
            _check_full_criteria_mapping(
                Path(change_dir), report.read_text(encoding="utf-8", errors="replace")
            )
        )
    return blockers


def _inspect_folder(folder: Path, tier: Tier | str | None, location: str) -> GuardianResult:
    blockers: list[str] = []
    tasks = folder / "tasks.md"
    tasks_text = tasks.read_text(encoding="utf-8", errors="replace") if tasks.is_file() else ""
    resolved = _resolve_tier(tasks_text, tier, blockers)
    blockers.extend(_check_minimums(folder, resolved, location))
    blockers.extend(check_verify_evidence(folder, resolved))
    return GuardianResult(ok=not blockers, blockers=blockers, tier=resolved, archive_path=folder)


def check_prearchive(
    repo_root: Path | str,
    slug: str,
    *,
    tier: Tier | str | None = None,
) -> GuardianResult:
    """Inspect active ``openspec/changes/<slug>/`` before archive-tail."""
    active = Path(repo_root) / "openspec" / "changes" / slug
    if not active.is_dir():
        return GuardianResult(
            ok=False,
            blockers=[f"active change folder missing at openspec/changes/{slug}/"],
            tier=str(tier) if tier else None,
            archive_path=active,
        )
    return _inspect_folder(active, tier, f"changes/{slug}")


def check_premerge(
    repo_root: Path | str,
    slug: str,
    *,
    tier: Tier | str | None = None,
) -> GuardianResult:
    """Inspect archived artifacts and staged evidence before merge."""
    root = Path(repo_root)
    changes = root / "openspec" / "changes"
    active = changes / slug
    archive_root = changes / "archive"
    blockers: list[str] = []

    if active.is_dir():
        blockers.append(
            f"active change folder still present at openspec/changes/{slug}/ — "
            "run archive-tail on the review branch before merge"
        )
    archive, archive_blockers = _resolve_archive(archive_root, slug)
    blockers.extend(archive_blockers)
    if archive is None:
        return GuardianResult(ok=False, blockers=blockers, tier=str(tier) if tier else None)

    result = _inspect_folder(archive, tier, f"archive/{archive.name}")
    result.blockers = blockers + result.blockers
    result.ok = not result.blockers
    result.archive_path = archive
    return result


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Check pre-merge archive/artifact gates")
    parser.add_argument("slug", help="change slug under openspec/changes/")
    parser.add_argument(
        "--root", type=Path, required=True,
        help="resolved planning root (never the process cwd; a subrepo request "
             "passes the proven superproject root)",
    )
    parser.add_argument(
        "--tier", choices=["light", "standard", "full"], default=None,
        help="planning depth (default: infer from tasks.md Depth: line)",
    )
    parser.add_argument(
        "--stage", choices=["pre-merge", "pre-archive"], default="pre-merge",
        help="enforcement stage (default: pre-merge)",
    )
    args = parser.parse_args(argv)
    if args.stage == "pre-archive":
        result = check_prearchive(args.root, args.slug, tier=args.tier)
    else:
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
