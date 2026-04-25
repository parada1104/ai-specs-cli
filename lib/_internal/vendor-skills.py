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
import importlib.util
from pathlib import Path

from skill_contract import from_dep, render_skill_markdown


ANCILLARY_DIRS = ("assets", "references", "scripts")


def fail(msg: str) -> None:
    print(f"  ✗ {msg}", file=sys.stderr)
    sys.exit(1)

def clone(source: str, dest: Path) -> None:
    subprocess.run(
        ["git", "clone", "--depth", "1", "--quiet", source, str(dest)],
        check=True,
    )


def _load_toml_read_module():
    module_path = Path(__file__).with_name("toml-read.py")
    spec = importlib.util.spec_from_file_location("toml_read_internal", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load helper module at {module_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def load_deps(project_root: Path) -> list[dict]:
    toml_path = project_root / "ai-specs" / "ai-specs.toml"
    module = _load_toml_read_module()
    try:
        data = module.load_toml(toml_path)
    except FileNotFoundError:
        fail(f"{toml_path} not found")
    return module.read_deps(data)


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

        if target_dir.exists():
            shutil.rmtree(target_dir)
        target_dir.mkdir(parents=True)

        (target_dir / "SKILL.md").write_text(render_skill_markdown(from_dep(dep, upstream_text)))

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
