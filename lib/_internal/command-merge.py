#!/usr/bin/env python3
"""Merge managed (cache) commands with hand-authored project commands.

Usage:
  command-merge.py <project_root> <dest_dir> [--cli-home <path>]

Copies cache-managed commands first, then overlays ``ai-specs/commands/``
so hand-authored files win on id collision. Fan-out targets consume ``dest_dir``.
"""

from __future__ import annotations

import argparse
import importlib.util
import shutil
import sys
from pathlib import Path


def _load_project_cache():
    module_path = Path(__file__).with_name("project-cache.py")
    spec = importlib.util.spec_from_file_location("project_cache_internal", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load project-cache.py at {module_path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def merge_commands(
    project_root: Path,
    dest_dir: Path,
    cli_home: Path | None = None,
) -> int:
    """Merge cache-managed + local commands into dest_dir. Returns file count."""
    pc = _load_project_cache()
    pc.ensure_cache(project_root, cli_home=cli_home)
    managed = pc.commands_dir(project_root, cli_home=cli_home)
    local = Path(project_root) / "ai-specs" / "commands"

    if dest_dir.exists():
        shutil.rmtree(dest_dir)
    dest_dir.mkdir(parents=True, exist_ok=True)

    copied = 0
    if managed.is_dir():
        for src in sorted(managed.iterdir()):
            if src.is_file() and src.suffix == ".md":
                shutil.copy2(src, dest_dir / src.name)
                copied += 1

    if local.is_dir():
        for src in sorted(local.iterdir()):
            if src.is_file() and src.suffix == ".md":
                dest = dest_dir / src.name
                if dest.exists():
                    print(
                        f"  ! local command '{src.name}' overrides managed command",
                        file=sys.stderr,
                    )
                shutil.copy2(src, dest)
                if not (managed / src.name).is_file():
                    copied += 1

    print(f"  ✓ merged {copied} command(s) → {dest_dir}")
    return copied


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("project_root", type=Path)
    parser.add_argument("dest_dir", type=Path)
    parser.add_argument("--cli-home", type=Path, default=None)
    args = parser.parse_args()
    merge_commands(args.project_root.resolve(), args.dest_dir.resolve(), cli_home=args.cli_home)
    return 0


if __name__ == "__main__":
    sys.exit(main())
