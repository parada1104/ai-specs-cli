#!/usr/bin/env python3
"""Resolve root + declared subrepo sync targets.

Usage:
  target-resolve.py <project_root>

Reads <project_root>/ai-specs/ai-specs.toml, normalizes project.subrepos, and
emits a JSON plan describing the resolved sync targets. Validation rules:
  - entries must be relative paths
  - entries must stay under the root after normalization / symlink resolution
  - entries must exist and be directories
  - duplicate normalized relpaths collapse to the first occurrence

`.gitmodules` is advisory-only in V1 and is not required for resolution.
"""

from __future__ import annotations

import importlib.util
import json
import posixpath
import sys
from pathlib import Path
from typing import Any


DERIVED_ARTIFACTS = [
    "AGENTS.md",
    "ai-specs/.gitignore",
    "ai-specs/skills/**",
    "ai-specs/commands/**",
    "agent-configs",
]
VERSION_POLICY = (
    "derived-only: overwrite from the root manifest on every root sync; "
    "subrepo copies are advisory outputs and must not be hand-edited"
)


class ResolutionError(Exception):
    def __init__(self, rel: str, reason: str):
        self.rel = rel
        self.reason = reason
        super().__init__(f"{rel}: {reason}")

    def as_dict(self) -> dict[str, str]:
        return {"path": self.rel, "reason": self.reason}


def _load_toml_read_module():
    module_path = Path(__file__).with_name("toml-read.py")
    spec = importlib.util.spec_from_file_location("toml_read_internal", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load helper module at {module_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _normalize_declared_relpath(raw: Any) -> str:
    if not isinstance(raw, str):
        raise ResolutionError(repr(raw), "must be a string")

    candidate = raw.strip()
    if not candidate:
        return ""
    if candidate.startswith("/"):
        raise ResolutionError(candidate, "must be relative to the root")

    candidate = candidate.replace("\\", "/")
    normalized = posixpath.normpath(candidate)
    if normalized in (".", ""):
        return "."
    if normalized == ".." or normalized.startswith("../"):
        raise ResolutionError(candidate, "escapes the root")
    return normalized


def _validate_target(root: Path, rel: str) -> Path:
    candidate = (root / rel).resolve()
    try:
        candidate.relative_to(root)
    except ValueError as exc:
        raise ResolutionError(rel, "escapes the root after resolution") from exc

    if not candidate.exists():
        raise ResolutionError(rel, "directory does not exist")
    if not candidate.is_dir():
        raise ResolutionError(rel, "path is not a directory")
    return candidate


def resolve_target_plan(project_root: str | Path) -> dict[str, Any]:
    root = Path(project_root).resolve()
    module = _load_toml_read_module()
    toml_path = root / "ai-specs" / "ai-specs.toml"
    data = module.load_toml(toml_path)
    project = module.read_project(data)
    declared = project["subrepos"]

    targets: list[dict[str, Any]] = []
    seen: set[str] = set()

    def append_target(name: str, kind: str, rel: str, path: Path) -> None:
        targets.append(
            {
                "name": name,
                "kind": kind,
                "path": str(path),
                "rel": rel,
                "derived_ai_specs": str(path / "ai-specs"),
                "manifest_source": str(toml_path),
                "derived_artifacts": list(DERIVED_ARTIFACTS),
                "version_policy": VERSION_POLICY,
            }
        )

    append_target("root", "root", ".", root)
    seen.add(".")

    for raw in declared:
        rel = _normalize_declared_relpath(raw)
        if not rel or rel in seen:
            continue
        path = _validate_target(root, rel)
        seen.add(rel)
        append_target(rel, "subrepo", rel, path)

    return {
        "root": str(root),
        "manifest": str(toml_path),
        "gitmodules": {
            "path": str(root / ".gitmodules"),
            "mode": "advisory-only",
            "present": (root / ".gitmodules").is_file(),
        },
        "targets": targets,
    }


def main() -> int:
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <project_root>", file=sys.stderr)
        return 2

    try:
        plan = resolve_target_plan(sys.argv[1])
    except FileNotFoundError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    except ResolutionError as exc:
        print(json.dumps({"error": exc.as_dict()}), file=sys.stderr)
        return 1
    except Exception as exc:  # pragma: no cover - defensive top-level guard
        print(f"error: {exc}", file=sys.stderr)
        return 1

    print(json.dumps(plan, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
