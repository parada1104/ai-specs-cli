#!/usr/bin/env python3
"""Flatten resolved skills into a single directory.

Usage:
  flatten-resolved-skills.py <project_root> <dest_dir>

Reads the multi-source resolved skill dict and copies each skill directory
into <dest_dir>/{skill-id}/. Existing contents of <dest_dir> are removed.
"""

from __future__ import annotations

import shutil
import sys
from pathlib import Path

import importlib.util


def _load_skill_resolution():
    module_path = Path(__file__).with_name("skill-resolution.py")
    spec = importlib.util.spec_from_file_location("skill_resolution_internal", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load skill-resolution.py at {module_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def main() -> int:
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <project_root> <dest_dir>", file=sys.stderr)
        return 2

    project_root = Path(sys.argv[1]).resolve()
    dest_dir = Path(sys.argv[2]).resolve()

    mod = _load_skill_resolution()
    resolved = mod.collect_skills(project_root)

    if dest_dir.exists():
        shutil.rmtree(dest_dir)
    dest_dir.mkdir(parents=True, exist_ok=True)

    for skill_id, (source_type, src_path) in sorted(resolved.items()):
        target = dest_dir / skill_id
        if target.exists():
            shutil.rmtree(target)
        shutil.copytree(src_path, target)

    print(f"  ✓ flattened {len(resolved)} skill(s) to {dest_dir}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
