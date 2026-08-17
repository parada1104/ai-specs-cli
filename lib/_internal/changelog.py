#!/usr/bin/env python3
"""Parse CHANGELOG.md sections for `ai-specs upgrade`.

Two surfaces read the same version-keyed sections:

  - the version-crossing summary ("what did I just get"), newest first;
  - version-keyed upgrade notices ("what must I do now"), oldest first, so
    instructions apply in release order.

Every entry point degrades to empty data rather than raising. By the time this
parser runs the fast-forward has already landed, so a malformed changelog must
never turn a successful upgrade into a failed one.

Notices are prose. `upgrade` operates on the global installation and has no
consumer project in scope, so a notice is never evaluated, templated, or
executed — it is displayed, and it names the command that does have project
state (`ai-specs sync`, `ai-specs doctor`).
"""

from __future__ import annotations

import re
import sys
from dataclasses import dataclass
from pathlib import Path

# "## [0.22.0] — 2026-08-17", "## [0.22.0] - 2026-08-17", or "## [0.22.0]".
# The separator may be an em dash (U+2014), an en dash (U+2013), or a plain
# hyphen; the date is optional. The en dash was originally missing, which made
# such a heading fail the match and silently drop that version's whole section.
_HEADING = re.compile(
    r"^##\s+\[(?P<version>\d+\.\d+\.\d+)\]"
    r"(?:\s*[—–-]\s*(?P<date>\S+))?\s*$"
)
_ANY_H2 = re.compile(r"^##\s+")
_FENCE = re.compile(r"^\s*(?:```|~~~)")
_NOTICE_HEADING = re.compile(r"^###\s+Upgrade notes\s*$", re.IGNORECASE)
_ANY_H3 = re.compile(r"^###\s+")
_SEMVER = re.compile(r"^(\d+)\.(\d+)\.(\d+)$")

NOTICE_HEADING = "### Upgrade notes"


@dataclass(frozen=True)
class Section:
    """One released version's changelog entry."""

    version: str
    date: str | None
    body: str

    @property
    def key(self) -> tuple[int, int, int]:
        return version_key(self.version)


def version_key(version: str) -> tuple[int, int, int] | None:
    """Semver sort key, or None when `version` is not a release version.

    String ordering is wrong here: "0.9.0" sorts after "0.22.0" as text but
    before it as a version.
    """
    match = _SEMVER.match((version or "").strip())
    if not match:
        return None
    return tuple(int(part) for part in match.groups())  # type: ignore[return-value]


def parse_sections(text: str) -> list[Section]:
    """Return released sections, newest first.

    `[Unreleased]` and any other non-semver heading are skipped: they are not
    something a user can cross.
    """
    if not text:
        return []

    lines = text.splitlines()
    # A '##' or '###' line inside a fenced code block is sample content, not a
    # heading. Without this, a changelog documenting markdown would truncate
    # its own section.
    fenced = _fenced_lines(lines)

    starts: list[tuple[int, str, str | None]] = []
    for index, line in enumerate(lines):
        if fenced[index]:
            continue
        match = _HEADING.match(line)
        if match:
            starts.append((index, match.group("version"), match.group("date")))

    sections: list[Section] = []
    for index, version, date in starts:
        # The body ends at the next H2 of any kind, not merely the next
        # released one, so an "[Unreleased]" heading cannot absorb a body.
        end = len(lines)
        for cursor in range(index + 1, len(lines)):
            if not fenced[cursor] and _ANY_H2.match(lines[cursor]):
                end = cursor
                break
        body = "\n".join(lines[index + 1 : end]).strip("\n")
        sections.append(Section(version=version, date=date, body=body))

    sections.sort(key=lambda section: section.key, reverse=True)

    # Collapse duplicate version headings, keeping the first (newest-first
    # order means that is the topmost occurrence). The repository's own
    # CHANGELOG carries a duplicated 0.12.4 entry, which would otherwise render
    # that version twice in the upgrade summary.
    deduped: list[Section] = []
    seen: set[str] = set()
    for section in sections:
        if section.version in seen:
            continue
        seen.add(section.version)
        deduped.append(section)
    return deduped


def _fenced_lines(lines: list[str]) -> list[bool]:
    """Mark which lines sit inside a fenced code block."""
    inside = False
    flags: list[bool] = []
    for line in lines:
        if _FENCE.match(line):
            # The fence delimiter itself is never a heading either way.
            flags.append(True)
            inside = not inside
            continue
        flags.append(inside)
    return flags


def read_sections(path: Path | str) -> list[Section]:
    """Parse a changelog file, returning [] when it cannot be read."""
    try:
        text = Path(path).read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return []
    return parse_sections(text)


def select_range(
    sections: list[Section], current: str, new: str
) -> list[Section]:
    """Sections in the crossed range (current, new], newest first.

    An unparseable `current` degrades to "report the target only" rather than
    dropping the summary: a hand-edited VERSION should still tell the user
    what they landed on.
    """
    new_key = version_key(new)
    if new_key is None:
        return []

    current_key = version_key(current)
    if current_key is None:
        return [s for s in sections if s.key == new_key]

    if current_key >= new_key:
        return []

    return [s for s in sections if current_key < s.key <= new_key]


def crossed_versions(text: str, current: str, new: str) -> list[Section]:
    """Convenience wrapper over parse_sections + select_range."""
    return select_range(parse_sections(text), current, new)


def upgrade_notice(section: Section) -> str | None:
    """The section's `### Upgrade notes` prose, or None when absent.

    The notice ends at the next H3 so a following subsection cannot bleed in.
    """
    lines = section.body.splitlines()
    fenced = _fenced_lines(lines)

    start = None
    for index, line in enumerate(lines):
        if not fenced[index] and _NOTICE_HEADING.match(line):
            start = index + 1
            break
    if start is None:
        return None

    end = len(lines)
    for cursor in range(start, len(lines)):
        if not fenced[cursor] and _ANY_H3.match(lines[cursor]):
            end = cursor
            break

    notice = "\n".join(lines[start:end]).strip()
    return notice or None


def _summary_bullets_all(section: Section) -> list[str]:
    """Every bullet under a summary-worthy subsection, as plain text.

    `### Upgrade notes` is excluded: it is an instruction, not a change, and it
    is rendered separately with more prominence.
    """
    lines = section.body.splitlines()
    bullets: list[str] = []
    in_summary = False
    current: list[str] = []

    def flush() -> None:
        if current:
            bullets.append(_condense(_plain(" ".join(current))))
            current.clear()

    for line in lines:
        if _ANY_H3.match(line):
            flush()
            in_summary = not _NOTICE_HEADING.match(line)
            continue
        if not in_summary:
            continue
        stripped = line.strip()
        if stripped.startswith(("- ", "* ")):
            flush()
            current.append(stripped[2:].strip())
        elif current and stripped:
            # A wrapped continuation line belongs to the bullet above it.
            current.append(stripped)
        elif not stripped:
            flush()
    flush()
    return [bullet for bullet in bullets if bullet]


MAX_BULLET = 100


def _plain(text: str) -> str:
    """Strip the inline markup that reads as noise in a terminal."""
    text = re.sub(r"\*\*(.+?)\*\*", r"\1", text)
    text = re.sub(r"`([^`]+)`", r"\1", text)
    return re.sub(r"\s+", " ", text).strip()


def _condense(text: str, limit: int = MAX_BULLET) -> str:
    """Reduce a changelog bullet to one scannable line.

    Changelog entries are written for readers who want the full story; an
    upgrade summary is read while waiting for a prompt. Prefer the first
    sentence, and truncate on a word boundary when there is no sentence break.
    """
    first = re.split(r"(?<=[.!?])\s+", text, maxsplit=1)[0].strip()
    if first and len(first) <= limit:
        return first

    candidate = first or text
    if len(candidate) <= limit:
        return candidate

    clipped = candidate[: limit - 1]
    if " " in clipped:
        clipped = clipped[: clipped.rindex(" ")]
    return clipped.rstrip(" ,;:.") + "…"


def summary_bullets(section: Section, limit: int = 3) -> list[str]:
    """Up to `limit` plain-text bullets describing what changed."""
    return _summary_bullets_all(section)[:limit]


def remaining_count(section: Section, limit: int = 3) -> int:
    """How many bullets `summary_bullets` dropped.

    Reported rather than silently truncated: a summary that hides how much it
    hid is worse than one that admits it.
    """
    return max(0, len(_summary_bullets_all(section)) - limit)


def _notices_for(sections: list[Section]) -> list[tuple[str, str]]:
    """(version, notice) pairs, oldest first, for already-selected sections."""
    pairs: list[tuple[str, str]] = []
    for section in reversed(sections):
        notice = upgrade_notice(section)
        if notice:
            pairs.append((section.version, notice))
    return pairs


def crossed_notices(text: str, current: str, new: str) -> list[tuple[str, str]]:
    """(version, notice) for each crossed version declaring one, oldest first.

    Oldest first is deliberate and differs from the summary order: notices are
    instructions, and instructions apply in release order.
    """
    return _notices_for(crossed_versions(text, current, new))


def _emit_summary(sections: list[Section], limit: int = 3) -> None:
    for section in sections:
        header = section.version
        if section.date:
            header = f"{header} — {section.date}"
        print(f"  {header}")
        for bullet in summary_bullets(section, limit=limit):
            print(f"    · {bullet}")
        dropped = remaining_count(section, limit=limit)
        if dropped:
            print(f"    · and {dropped} more")


def _emit_notices(pairs: list[tuple[str, str]]) -> None:
    for version, notice in pairs:
        print(f"  {version}")
        for line in notice.splitlines():
            print(f"  {line}" if line.strip() else "")


def main(argv: list[str]) -> int:
    """CLI shim: `changelog.py <path> <current> <new> [--notices]`.

    Prints nothing and exits 0 when there is nothing to report, so the caller
    can pipe unconditionally.
    """
    if len(argv) < 4:
        print(
            "usage: changelog.py <changelog-path> <current> <new> [--notices]",
            file=sys.stderr,
        )
        return 2

    path, current, new = argv[1], argv[2], argv[3]
    want_notices = "--notices" in argv[4:]

    sections = read_sections(path)
    if not sections:
        return 0

    selected = select_range(sections, current, new)
    if not selected:
        return 0

    if want_notices:
        # Reuse the unit-tested helper rather than re-deriving the pairs here;
        # the two implementations would otherwise be free to drift.
        _emit_notices(_notices_for(selected))
    else:
        _emit_summary(selected)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
