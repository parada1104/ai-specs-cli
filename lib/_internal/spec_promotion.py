#!/usr/bin/env python3
"""Promote file-backed change delta specs into canonical ``openspec/specs`` specs.

This is the explicit **writer** in the Plan Build archive lifecycle:

```text
verify → promote canonical specs → pre-archive guardian → archive-tail → pre-merge guardian
```

``premerge_guardian.py`` stays read-only and only validates parity; this module
performs the canonical writes. It never calls a network or provider tool, and it
never touches tracker state.

Composition follows the existing SDD archive contract, per domain:

- ``openspec/changes/<slug>/specs/<domain>/spec.md`` → ``openspec/specs/<domain>/spec.md``
- ``## ADDED Requirements`` appends new requirements;
- ``## MODIFIED Requirements`` replaces the full canonical block with the exact
  same requirement name;
- ``## REMOVED Requirements`` deletes the matching block, but only through the
  explicit approval path (``--allow-removed``), because the contract treats a
  destructive removal as requiring recorded human approval;
- ``## RENAMED Requirements`` is unsupported and blocks instead of being improvised;
- unrelated canonical requirements and document sections are preserved.

The operation is idempotent and resume-safe: an already-composed delta is
detected as a no-op, so a rerun after an interruption writes nothing. Writes are
atomic (temp file plus ``os.replace``) and every destination is checked against
the repository-bound ``openspec/specs/`` tree, including symlink escapes.
"""

from __future__ import annotations

import argparse
import os
import re
import sys
import tempfile
from dataclasses import dataclass, field
from pathlib import Path


class PromotionError(Exception):
    """A delta could not be composed safely."""


REQUIREMENT_HEADING_RE = re.compile(r"^###[ \t]+Requirement:[ \t]*(.+?)[ \t]*$", re.MULTILINE)
TOP_LEVEL_HEADING_RE = re.compile(r"^##[ \t]+", re.MULTILINE)
DELTA_SECTION_RE = re.compile(
    r"^##[ \t]+([A-Z][A-Z _-]*)[ \t]+Requirements[ \t]*$",
    re.MULTILINE | re.IGNORECASE,
)
REQUIREMENTS_SECTION_RE = re.compile(r"^##[ \t]+Requirements[ \t]*$", re.MULTILINE)
TRAILING_SEPARATOR_RE = re.compile(r"\n[ \t]*---[ \t]*$")
SLUG_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*$")
OPERATIONS = ("added", "modified", "removed")


@dataclass
class RequirementBlock:
    name: str
    content: str
    start: int
    end: int


@dataclass
class DeltaSpec:
    added: list[RequirementBlock] = field(default_factory=list)
    modified: list[RequirementBlock] = field(default_factory=list)
    removed: list[RequirementBlock] = field(default_factory=list)


@dataclass
class Composition:
    """Result of composing one delta against canonical text."""

    changed: bool
    content: str
    applied: list[str] = field(default_factory=list)
    pending: list[str] = field(default_factory=list)
    blockers: list[str] = field(default_factory=list)


def _normalize(markdown: str) -> str:
    return markdown.replace("\r\n", "\n")


def _next_top_level_section(markdown: str, start: int) -> int:
    match = TOP_LEVEL_HEADING_RE.search(markdown, start)
    return match.start() if match else len(markdown)


def _clean_requirement_content(content: str) -> str:
    return TRAILING_SEPARATOR_RE.sub("", content.rstrip()).rstrip()


def parse_requirement_blocks(markdown: str) -> list[RequirementBlock]:
    """Parse ``### Requirement:`` blocks, keeping their full body and offsets."""
    source = _normalize(markdown)
    matches = list(REQUIREMENT_HEADING_RE.finditer(source))
    blocks: list[RequirementBlock] = []
    for index, match in enumerate(matches):
        start = match.start()
        if index + 1 < len(matches):
            end = matches[index + 1].start()
        else:
            end = _next_top_level_section(source, match.end())
        blocks.append(
            RequirementBlock(
                name=match.group(1).strip(),
                content=_clean_requirement_content(source[start:end]),
                start=start,
                end=end,
            )
        )
    return blocks


def parse_delta_spec(markdown: str) -> DeltaSpec:
    """Parse a delta spec, rejecting unsupported or contradictory operations."""
    source = _normalize(markdown)
    sections = list(DELTA_SECTION_RE.finditer(source))
    delta = DeltaSpec()
    for index, match in enumerate(sections):
        label = match.group(1).upper()
        if label == "RENAMED":
            raise PromotionError(
                "## RENAMED Requirements is unsupported: the archive contract defines "
                "ADDED/MODIFIED/REMOVED only — rewrite the delta as REMOVED plus ADDED "
                "before promoting"
            )
        if label.lower() not in OPERATIONS:
            raise PromotionError(f"## {label} Requirements is unsupported")
        section_start = match.end()
        section_end = (
            sections[index + 1].start() if index + 1 < len(sections) else len(source)
        )
        getattr(delta, label.lower()).extend(
            parse_requirement_blocks(source[section_start:section_end])
        )

    seen: dict[str, str] = {}
    for operation in OPERATIONS:
        for block in getattr(delta, operation):
            previous = seen.get(block.name)
            if previous:
                raise PromotionError(
                    f'duplicate delta operation for requirement "{block.name}" '
                    f"({previous} and {operation.upper()})"
                )
            seen[block.name] = operation.upper()
    return delta


def _named_section(markdown: str, name: str) -> str | None:
    heading = re.compile(rf"^##[ \t]+{re.escape(name)}[ \t]*$", re.MULTILINE)
    match = heading.search(markdown)
    if match is None:
        return None
    end = _next_top_level_section(markdown, match.end())
    return markdown[match.start():end].rstrip()


def _render_new_canonical(domain: str, delta_text: str, added: list[RequirementBlock]) -> str:
    """Build a canonical spec for a brand-new domain from its delta."""
    parts = [f"# {domain} Specification"]
    purpose = _named_section(delta_text, "Purpose")
    if purpose:
        parts.append(purpose)
    parts.append("## Requirements")
    parts.append("\n\n".join(block.content.strip() for block in added))
    return "\n\n".join(parts).rstrip() + "\n"


def _append_added(markdown: str, added: list[RequirementBlock]) -> str:
    if not added:
        return markdown
    addition = "\n\n".join(block.content.strip() for block in added)
    match = REQUIREMENTS_SECTION_RE.search(markdown)
    if match is None:
        return f"{markdown.rstrip()}\n\n## Requirements\n\n{addition}\n"
    section_end = _next_top_level_section(markdown, match.end())
    before = markdown[:section_end].rstrip()
    after = markdown[section_end:].lstrip("\n")
    if after:
        return f"{before}\n\n{addition}\n\n{after}"
    return f"{before}\n\n{addition}\n"


def compose(
    canonical_text: str | None,
    delta_text: str,
    *,
    domain: str = "spec",
    allow_removed: bool = False,
) -> Composition:
    """Compose one delta into canonical text, reporting blockers or a no-op."""
    try:
        delta = parse_delta_spec(delta_text)
    except PromotionError as exc:
        return Composition(changed=False, content=canonical_text or "", blockers=[str(exc)])

    if canonical_text is None:
        blockers: list[str] = []
        if delta.modified:
            names = ", ".join(f'"{block.name}"' for block in delta.modified)
            blockers.append(
                f"MODIFIED requirement {names} has no canonical spec to modify; "
                "MODIFIED requires an existing openspec/specs/<domain>/spec.md"
            )
        if delta.removed:
            names = ", ".join(f'"{block.name}"' for block in delta.removed)
            blockers.append(f"REMOVED requirement {names} has no canonical spec to delete from")
        if blockers:
            return Composition(changed=False, content="", blockers=blockers)
        if not delta.added:
            return Composition(changed=False, content="")
        return Composition(
            changed=True,
            content=_render_new_canonical(domain, delta_text, delta.added),
            pending=[f"ADDED {block.name}" for block in delta.added],
        )

    canonical_map: dict[str, RequirementBlock] = {}
    duplicate_blockers: list[str] = []
    for block in parse_requirement_blocks(canonical_text):
        if block.name in canonical_map:
            duplicate_blockers.append(
                f'canonical spec declares requirement "{block.name}" more than once'
            )
        else:
            canonical_map[block.name] = block
    if duplicate_blockers:
        return Composition(
            changed=False, content=canonical_text, blockers=duplicate_blockers
        )

    applied: list[str] = []
    pending: list[str] = []
    blockers = []
    additions: list[RequirementBlock] = []
    replacements: list[tuple[RequirementBlock, RequirementBlock, str]] = []

    for block in delta.added:
        target = canonical_map.get(block.name)
        if target is None:
            additions.append(block)
            pending.append(f"ADDED {block.name}")
        elif target.content == block.content:
            applied.append(f"ADDED {block.name} (already canonical)")
        else:
            blockers.append(
                f'ADDED requirement "{block.name}" collides with an existing canonical '
                "requirement with different content; use MODIFIED for an intentional "
                "replacement, or rename the requirement"
            )

    for block in delta.modified:
        target = canonical_map.get(block.name)
        if target is None:
            blockers.append(
                f'MODIFIED requirement "{block.name}" does not exist in the canonical '
                "spec; there is nothing safe to replace"
            )
        elif target.content == block.content:
            applied.append(f"MODIFIED {block.name} (already canonical)")
        else:
            replacements.append((target, block, block.content.strip()))
            pending.append(f"MODIFIED {block.name}")

    for block in delta.removed:
        target = canonical_map.get(block.name)
        if target is None:
            applied.append(f"REMOVED {block.name} (already absent)")
        elif not allow_removed:
            blockers.append(
                f'REMOVED requirement "{block.name}" is destructive and needs the explicit '
                "approval path; re-run the promoter with --allow-removed after the removal "
                "was approved"
            )
        else:
            replacements.append((target, block, ""))
            pending.append(f"REMOVED {block.name}")

    if blockers:
        return Composition(
            changed=False, content=canonical_text, applied=applied, blockers=blockers
        )
    if not additions and not replacements:
        return Composition(changed=False, content=canonical_text, applied=applied)

    result = canonical_text
    for target, _block, content in sorted(
        replacements, key=lambda item: item[0].start, reverse=True
    ):
        prefix = result[: target.start].rstrip()
        suffix = result[target.end:].lstrip("\n")
        if content:
            result = f"{prefix}\n\n{content}\n\n{suffix}".rstrip() + "\n"
        else:
            result = f"{prefix}\n\n{suffix}".rstrip() + "\n"
    result = _append_added(result, additions).rstrip() + "\n"
    return Composition(
        changed=result != canonical_text,
        content=result,
        applied=applied,
        pending=pending,
    )


@dataclass
class DomainPlan:
    domain: str
    source: Path
    canonical: Path
    composition: Composition


@dataclass
class PromotionReport:
    root: Path
    slug: str
    folder: Path | None = None
    plans: list[DomainPlan] = field(default_factory=list)
    blockers: list[str] = field(default_factory=list)

    @property
    def changed(self) -> bool:
        return any(plan.composition.changed for plan in self.plans)

    @property
    def pending(self) -> list[str]:
        return [
            f"{plan.domain}: {item}"
            for plan in self.plans
            for item in plan.composition.pending
        ]

    @property
    def applied(self) -> list[str]:
        return [
            f"{plan.domain}: {item}"
            for plan in self.plans
            for item in plan.composition.applied
        ]

    @property
    def ok(self) -> bool:
        return not self.blockers and not self.pending


def _planning_root(root: Path | str) -> Path:
    candidate = Path(root)
    if not (candidate / "openspec").is_dir():
        raise PromotionError(
            f"--root {candidate} is not a planning root: {candidate / 'openspec'} is missing; "
            "pass the resolved planning root, never the process cwd"
        )
    return candidate


def _validate_slug(slug: str) -> str:
    if not SLUG_RE.fullmatch(slug or ""):
        raise PromotionError(
            f"invalid change slug {slug!r}: a slug is one path segment with no separators"
        )
    return slug


def _canonical_target(specs_base: Path, domain: str) -> Path:
    """Resolve a canonical destination inside ``openspec/specs/`` or raise."""
    if not SLUG_RE.fullmatch(domain or ""):
        raise PromotionError(f"invalid spec domain name {domain!r}")
    target = specs_base / domain / "spec.md"
    base = specs_base.resolve()
    resolved = target.resolve()
    if resolved != base and base not in resolved.parents:
        raise PromotionError(
            f"canonical spec path {target} escapes the repository planning tree at {specs_base}"
        )
    for path in (specs_base.parent, specs_base, specs_base / domain, target):
        if path.is_symlink():
            raise PromotionError(f"refusing to promote through symlinked path {path}")
    return target


def _delta_specs(folder: Path) -> tuple[list[tuple[str, Path]], list[str]]:
    """Return ``(domain, delta path)`` pairs and ambiguity blockers."""
    specs = folder / "specs"
    if not specs.is_dir():
        return [], []
    found: dict[str, Path] = {}
    blockers: list[str] = []
    for path in sorted(specs.rglob("spec.md")):
        if path.is_symlink() or not path.is_file():
            continue
        domain = path.parent.name
        if domain in found:
            blockers.append(
                f"ambiguous delta domain '{domain}': {found[domain]} and {path} both "
                "declare a domain spec"
            )
            continue
        found[domain] = path
    return sorted(found.items()), blockers


def _active_domain_collisions(root: Path, slug: str, domains: set[str]) -> list[str]:
    """Name other active changes that declare a delta for the same domains.

    The archive contract stops instead of composing a delta against an ambiguous
    canonical order: when two active changes both touch ``specs/<domain>/spec.md``
    an explicit sync/archive decision is required first. Only real, non-archived
    change folders are scanned, and a symlinked change folder is skipped rather
    than followed outside the planning tree. This is pure discovery: no canonical
    spec is written, and the caller applies the result before any write so a
    collision aborts the whole promotion.
    """
    if not domains:
        return []
    changes = root / "openspec" / "changes"
    candidates = sorted(changes.iterdir(), key=lambda path: path.name) if changes.is_dir() else []
    blockers: list[str] = []
    for child in candidates:
        if child.name == "archive" or child.name == slug:
            continue
        if child.is_symlink() or not child.is_dir():
            continue
        shared, _ = _delta_specs(child)
        for domain, _path in shared:
            if domain in domains:
                blockers.append(
                    f'active change "{child.name}" also declares spec delta '
                    f"specs/{domain}/spec.md — the collision is unresolved; choose an "
                    "explicit sync/archive order or narrow one change before promoting"
                )
    return blockers


def _plan_domains(
    root: Path, folder: Path, *, allow_removed: bool
) -> tuple[list[DomainPlan], list[str]]:
    deltas, blockers = _delta_specs(folder)
    if not deltas:
        return [], blockers
    specs_base = root / "openspec" / "specs"
    plans: list[DomainPlan] = []
    for domain, source in deltas:
        try:
            canonical = _canonical_target(specs_base, domain)
        except PromotionError as exc:
            blockers.append(str(exc))
            continue
        canonical_text: str | None = None
        if canonical.is_file():
            canonical_text = canonical.read_text(encoding="utf-8", errors="replace")
        elif canonical.exists():
            blockers.append(f"canonical spec path {canonical} exists and is not a regular file")
            continue
        composition = compose(
            canonical_text,
            source.read_text(encoding="utf-8", errors="replace"),
            domain=domain,
            allow_removed=allow_removed,
        )
        for blocker in composition.blockers:
            blockers.append(f"specs/{domain}/spec.md: {blocker}")
        plans.append(DomainPlan(domain, source, canonical, composition))
    return plans, blockers


def _atomic_write(path: Path, content: str) -> None:
    """Write ``content`` through a same-directory temp file and ``os.replace``."""
    path.parent.mkdir(parents=True, exist_ok=True)
    handle_fd, temp_name = tempfile.mkstemp(
        dir=str(path.parent), prefix=".spec-promotion-", suffix=".tmp"
    )
    try:
        with os.fdopen(handle_fd, "w", encoding="utf-8", newline="\n") as handle:
            handle.write(content)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temp_name, path)
    except BaseException:
        try:
            os.unlink(temp_name)
        except FileNotFoundError:
            pass
        raise


def promote_change(
    root: Path | str, slug: str, *, allow_removed: bool = False
) -> PromotionReport:
    """Compose the active change's deltas into canonical specs, or report blockers.

    Nothing is written when any domain is blocked or when another active change
    declares a delta for the same domain, so a blocked promotion leaves the
    canonical tree byte-identical.
    """
    try:
        planning = _planning_root(root)
        _validate_slug(slug)
    except PromotionError as exc:
        return PromotionReport(root=Path(root), slug=slug, blockers=[str(exc)])

    folder = planning / "openspec" / "changes" / slug
    report = PromotionReport(root=planning, slug=slug, folder=folder)
    if folder.is_symlink() or not folder.is_dir():
        report.blockers.append(
            f"active change folder missing at openspec/changes/{slug}/ — the promoter runs "
            "before archive-tail while the change folder is still active"
        )
        return report

    plans, blockers = _plan_domains(planning, folder, allow_removed=allow_removed)
    report.plans = plans
    report.blockers = _active_domain_collisions(
        planning, slug, {plan.domain for plan in plans}
    ) + blockers
    if report.blockers:
        return report
    for plan in plans:
        if plan.composition.changed:
            _atomic_write(plan.canonical, plan.composition.content)
    return report


def check_folder_parity(root: Path | str, folder: Path | str) -> list[str]:
    """Read-only parity check for one change folder (active or archived).

    Returns blockers when a delta is unpromoted or cannot be composed, so the
    Plan Build guardian can block Standard/Full without ever writing. A change
    with no ``specs/`` deltas is a no-op.
    """
    root = Path(root)
    folder = Path(folder)
    plans, blockers = _plan_domains(root, folder, allow_removed=False)
    for plan in plans:
        if plan.composition.pending:
            blockers.append(
                f"specs/{plan.domain}/spec.md is not promoted into "
                f"openspec/specs/{plan.domain}/spec.md ({', '.join(plan.composition.pending)}); "
                "run the canonical spec promoter (lib/_internal/spec_promotion.py) before archive"
            )
    return blockers


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Promote change delta specs into canonical openspec/specs/ specs"
    )
    parser.add_argument("slug", help="change slug under openspec/changes/")
    parser.add_argument(
        "--root", type=Path, required=True,
        help="resolved planning root (never the process cwd; a subrepo request "
             "passes the proven superproject root)",
    )
    parser.add_argument(
        "--allow-removed", action="store_true",
        help="execute destructive REMOVED requirements (only after the removal "
             "was explicitly approved)",
    )
    args = parser.parse_args(argv)

    report = promote_change(args.root, args.slug, allow_removed=args.allow_removed)
    if report.blockers:
        print("spec-promotion: BLOCKED", file=sys.stderr)
        for blocker in report.blockers:
            print(f"  - {blocker}", file=sys.stderr)
        return 1
    if not report.pending:
        if report.applied:
            print(f"spec-promotion: OK (already promoted: {', '.join(report.applied)})")
        else:
            print("spec-promotion: OK (nothing to promote)")
        return 0
    print(f"spec-promotion: OK ({len(report.plans)} domain spec(s) composed)")
    for item in report.pending:
        print(f"  - {item}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
