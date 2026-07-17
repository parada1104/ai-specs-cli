#!/usr/bin/env python3
"""Refresh the managed agent-generated block in a project root .gitignore.

Usage:
  gitignore-root-refresh.py <project_root> <template_path>
"""

from __future__ import annotations

import sys
from pathlib import Path

MARKER_BEGIN = (
    "# --- ai-specs: agent-generated files (managed by ai-specs sync-agent) ---"
)
MARKER_END = "# --- end ai-specs ---"


def refresh_root_gitignore(project_root: Path, template_path: Path) -> str:
    """Replace or append the managed agent block from *template_path*.

    Returns ``\"refreshed\"`` when an existing block was replaced, or
    ``\"appended\"`` when the block was newly added.
    """
    root = Path(project_root)
    gitignore = root / ".gitignore"
    template = Path(template_path).read_text()
    if not template.endswith("\n"):
        template += "\n"

    text = gitignore.read_text() if gitignore.is_file() else ""

    begin = text.find(MARKER_BEGIN)
    if begin != -1:
        end = text.find(MARKER_END, begin)
        if end == -1:
            raise ValueError(
                f"{gitignore}: found begin marker without matching end marker"
            )
        end += len(MARKER_END)
        after = text[end:]
        if after.startswith("\n"):
            after = after[1:]
        before = text[:begin]
        if before and not before.endswith("\n"):
            before += "\n"
        gitignore.write_text(before + template + after)
        return "refreshed"

    if text and not text.endswith("\n"):
        text += "\n"
    if text and not text.endswith("\n\n"):
        text += "\n"
    gitignore.parent.mkdir(parents=True, exist_ok=True)
    gitignore.write_text(text + template)
    return "appended"


def main() -> int:
    if len(sys.argv) != 3:
        print(
            "Usage: gitignore-root-refresh.py <project_root> <template_path>",
            file=sys.stderr,
        )
        return 2
    project_root = Path(sys.argv[1])
    template_path = Path(sys.argv[2])
    if not template_path.is_file():
        print(f"ERROR: template not found: {template_path}", file=sys.stderr)
        return 1
    action = refresh_root_gitignore(project_root, template_path)
    print(f"  ✓ {action} root .gitignore (agent block)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
