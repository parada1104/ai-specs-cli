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
# The separator may be an em dash or a hyphen; the date is optional.
_HEADING = re.compile(
    r"^##\s+\[(?P<version>\d+\.\d+\.\d+)\]"
    r"(?:\s*[—-]\s*(?P<date>\S+))?\s*$"
)
_ANY_H2 = re.compile(r"^##\s+")
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
    starts: list[tuple[int, str, str | None]] = []
    for index, line in enumerate(lines):
        match = _HEADING.match(line)
        if match:
            starts.append((index, match.group("version"), match.group("date")))

    sections: list[Section] = []
    for position, (index, version, date) in enumerate(starts):
        # The body ends at the next H2 of any kind, not merely the next
        # released one, so an "[Unreleased]" heading cannot absorb a body.
        end = len(lines)
        for cursor in range(index + 1, len(lines)):
            if _ANY_H2.match(lines[cursor]):
                end = cursor
                break
        body = "\n".join(lines[index + 1 : end]).strip("\n")
        sections.append(Section(version=version, date=date, body=body))

    sections.sort(key=lambda section: section.key, reverse=True)
    return sections


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
    start = None
    for index, line in enumerate(lines):
        if _NOTICE_HEADING.match(line):
            start = index + 1
            break
    if start is None:
        return None

    end = len(lines)
    for cursor in range(start, len(lines)):
        if _ANY_H3.match(lines[cursor]):
            end = cursor
            break

    notice = "\n".join(lines[start:end]).strip()
    return notice or None


def crossed_notices(text: str, current: str, new: str) -> list[tuple[str, str]]:
    """(version, notice) for each crossed version declaring one, oldest first.

    Oldest first is deliberate and differs from the summary order: notices are
    instructions, and instructions apply in release order.
    """
    pairs: list[tuple[str, str]] = []
    for section in reversed(crossed_versions(text, current, new)):
        notice = upgrade_notice(section)
        if notice:
            pairs.append((section.version, notice))
    return pairs


def _emit_summary(sections: list[Section]) -> None:
    for section in sections:
        header = section.version
        if section.date:
            header = f"{header} — {section.date}"
        print(f"  {header}")


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
        pairs: list[tuple[str, str]] = []
        for section in reversed(selected):
            notice = upgrade_notice(section)
            if notice:
                pairs.append((section.version, notice))
        _emit_notices(pairs)
    else:
        _emit_summary(selected)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
