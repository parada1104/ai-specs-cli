#!/usr/bin/env python3
"""Vendor external skills declared in [[deps]] of ai-specs.toml.

For each entry:
  - shallow-clone `source` into a tempdir
  - locate SKILL.md at repo root or under optional `path`
  - copy SKILL.md + ancillary dirs (assets/, references/, scripts/) to
    <project>/ai-specs/skills/<id>/
  - replace the upstream YAML frontmatter with a standardized block
    (preserves the upstream body verbatim)

Root sync remains the ONLY place that vendors external skills. Multi-target
fan-out mirrors the already-vendored root ai-specs/skills tree into subrepos.

Usage:
  vendor-skills.py <project_root>
"""

from __future__ import annotations

import shutil
import subprocess
import sys
import tempfile
import tomllib
from pathlib import Path


ANCILLARY_DIRS = ("assets", "references", "scripts")


def fail(msg: str) -> None:
    print(f"  ✗ {msg}", file=sys.stderr)
    sys.exit(1)


def strip_frontmatter(text: str) -> tuple[str, str]:
    if not text.startswith("---"):
        return "", text
    rest = text[3:]
    end = rest.find("\n---")
    if end < 0:
        return "", text
    fm = rest[:end].lstrip("\n")
    after = rest[end + 4 :]
    if after.startswith("\n"):
        after = after[1:]
    return fm, after


def extract_description(fm: str) -> str:
    lines = fm.splitlines()
    i = 0
    while i < len(lines):
        line = lines[i]
        if line.startswith("description:"):
            rest = line[len("description:") :].strip()
            if rest and rest not in (">", "|", ">-", "|-"):
                return rest.strip("\"'")
            buf: list[str] = []
            j = i + 1
            while j < len(lines):
                nxt = lines[j]
                if not nxt.strip():
                    j += 1
                    continue
                if nxt and not nxt[0].isspace():
                    break
                buf.append(nxt.strip())
                j += 1
            return " ".join(buf)
        i += 1
    return ""


def render_frontmatter(dep: dict, upstream_desc: str) -> str:
    name = dep["id"]
    attribution = dep.get("vendor_attribution", "")
    license_id = dep.get("license", "Unknown")
    scope = dep.get("scope") or ["root"]
    auto_invoke = dep.get("auto_invoke") or []

    desc_parts: list[str] = []
    if upstream_desc:
        desc_parts.append(upstream_desc.rstrip(". "))
    if attribution:
        desc_parts.append(f"Vendored from {attribution} (see metadata.source)")
    description = ". ".join(desc_parts) or f"Vendored skill: {name}"

    out = ["---", f"name: {name}", "description: >", f" {description}.", f"license: {license_id}", "metadata:"]
    if "source" in dep:
        out.append(f" source: {dep['source']}")
    if attribution:
        out.append(f" vendor_attribution: {attribution}")
    out.append(f" scope: [{', '.join(scope)}]")
    if auto_invoke:
        out.append(" auto_invoke:")
        for phrase in auto_invoke:
            escaped = phrase.replace('"', '\\"')
            out.append(f'   - "{escaped}"')
    out.append("---")
    return "\n".join(out) + "\n"


def clone(source: str, dest: Path) -> None:
    subprocess.run(
        ["git", "clone", "--depth", "1", "--quiet", source, str(dest)],
        check=True,
    )


def load_deps(project_root: Path) -> list[dict]:
    toml_path = project_root / "ai-specs" / "ai-specs.toml"
    if not toml_path.is_file():
        fail(f"{toml_path} not found")
    with toml_path.open("rb") as f:
        data = tomllib.load(f)
    return data.get("deps", []) or []


def sync_dep_target(dep: dict, project_root: Path) -> None:
    dep_id = dep.get("id")
    source = dep.get("source")
    if not dep_id or not source:
        fail(f"dep missing id/source: {dep!r}")

    skill_subpath = dep.get("path", "").strip("/")
    target_dir = project_root / "ai-specs" / "skills" / dep_id

    print(f"  ▸ {dep_id}  ←  {source}" + (f"  (path: {skill_subpath})" if skill_subpath else ""))

    with tempfile.TemporaryDirectory(prefix="ai-specs-vendor-") as tmp:
        tmp_path = Path(tmp)
        clone(source, tmp_path)

        src_dir = tmp_path / skill_subpath if skill_subpath else tmp_path
        skill_md = src_dir / "SKILL.md"
        if not skill_md.is_file():
            fail(f"{dep_id}: SKILL.md not found at {skill_md.relative_to(tmp_path)}")

        upstream_text = skill_md.read_text()
        upstream_fm, upstream_body = strip_frontmatter(upstream_text)
        upstream_desc = extract_description(upstream_fm)

        if target_dir.exists():
            shutil.rmtree(target_dir)
        target_dir.mkdir(parents=True)

        new_frontmatter = render_frontmatter(dep, upstream_desc)
        (target_dir / "SKILL.md").write_text(new_frontmatter + upstream_body)

        for ancillary in ANCILLARY_DIRS:
            a_src = src_dir / ancillary
            if a_src.is_dir():
                shutil.copytree(a_src, target_dir / ancillary, dirs_exist_ok=False)


def sync_vendored_skills(project_root: Path, deps: list[dict]) -> int:
    if not deps:
        print("  (no [[deps]] declared — nothing to vendor)")
        return 0

    for dep in deps:
        sync_dep_target(dep, project_root)

    print(f"  ✓ vendored {len(deps)} dep(s)")
    return 0


def main() -> int:
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <project_root>", file=sys.stderr)
        return 2

    project_root = Path(sys.argv[1]).resolve()
    deps = load_deps(project_root)
    return sync_vendored_skills(project_root, deps)


if __name__ == "__main__":
    sys.exit(main())
