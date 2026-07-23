#!/usr/bin/env python3
"""Per-project CLI cache for recipe origin staging.

Cache root: ``$AI_SPECS_HOME/cache/projects/<key>/``
Key: ``sha256(realpath(project_root))[:12]-<sanitized-basename>``

Layout under the cache root:
  meta.toml
  .recipe/<recipe-id>/skills/...
  .deps/<dep-id>/skills/...
  commands/<cmd-id>.md
  resolved-skills/<skill-id>/
"""

from __future__ import annotations

import hashlib
import os
import re
import shutil
import sys
import tomllib
from datetime import datetime, timezone
from pathlib import Path

_BASENAME_SAFE = re.compile(r"[^A-Za-z0-9._-]+")


def _ai_specs_home() -> Path:
    env = os.environ.get("AI_SPECS_HOME")
    if env:
        return Path(env).resolve()
    return Path(__file__).resolve().parents[2]


def _sanitize_basename(name: str) -> str:
    cleaned = _BASENAME_SAFE.sub("-", name).strip("-._")
    return cleaned or "project"


def cache_key(project_root: Path) -> str:
    """Stable short key: 12-char sha256 of realpath + sanitized basename."""
    resolved = Path(project_root).resolve()
    digest = hashlib.sha256(str(resolved).encode("utf-8")).hexdigest()[:12]
    return f"{digest}-{_sanitize_basename(resolved.name)}"


def cache_root(project_root: Path, cli_home: Path | None = None) -> Path:
    home = Path(cli_home).resolve() if cli_home is not None else _ai_specs_home()
    return (home / "cache" / "projects" / cache_key(project_root)).resolve()


def ensure_cache(project_root: Path, cli_home: Path | None = None) -> Path:
    """Create cache root and write/refresh meta.toml sidecar."""
    root = Path(project_root).resolve()
    cache = cache_root(root, cli_home=cli_home)
    try:
        cache.mkdir(parents=True, exist_ok=True)
    except OSError as exc:
        raise RuntimeError(f"cache not writable at {cache}: {exc}") from exc

    meta = cache / "meta.toml"
    if not meta.is_file():
        created_at = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        meta.write_text(
            f'project_root = "{root}"\ncreated_at = "{created_at}"\n',
            encoding="utf-8",
        )
    else:
        # Keep project_root current (worktree moves / renames).
        text = meta.read_text(encoding="utf-8")
        lines = []
        saw_root = False
        for line in text.splitlines():
            if line.strip().startswith("project_root"):
                lines.append(f'project_root = "{root}"')
                saw_root = True
            else:
                lines.append(line)
        if not saw_root:
            lines.insert(0, f'project_root = "{root}"')
        meta.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return cache


def recipe_skills_root(project_root: Path, cli_home: Path | None = None) -> Path:
    return cache_root(project_root, cli_home=cli_home) / ".recipe"


def deps_skills_root(project_root: Path, cli_home: Path | None = None) -> Path:
    return cache_root(project_root, cli_home=cli_home) / ".deps"


def bundled_skills_root(project_root: Path, cli_home: Path | None = None) -> Path:
    """CLI-bundled skills flattened into the cache (recipe-independent)."""
    return cache_root(project_root, cli_home=cli_home) / ".bundled"


def inproject_deps_root(project_root: Path) -> Path:
    """toml-declared deps ([[deps]]) materialize in-project (gitignored).

    These are project governance (the user chose them via add-dep), so unlike
    recipe-deps (which stay in the cache) their skills live under the project
    tree — but gitignored, since they are regenerable from the declared source.
    """
    return Path(project_root) / "ai-specs" / ".deps"


def commands_dir(project_root: Path, cli_home: Path | None = None) -> Path:
    return cache_root(project_root, cli_home=cli_home) / "commands"


def resolved_skills_dir(project_root: Path, cli_home: Path | None = None) -> Path:
    return cache_root(project_root, cli_home=cli_home) / "resolved-skills"


def _warn(msg: str) -> None:
    print(f"  ! {msg}", file=sys.stderr)


def _normalized_bytes(path: Path) -> bytes:
    return path.read_bytes().replace(b"\r\n", b"\n")


def _legacy_lock_skill_hashes(ai_specs: Path) -> dict[str, dict]:
    """Read the legacy [skills.*] hash table from an old lock (best-effort).

    Used only as a migration signal: a committed bundled-skill copy whose
    SKILL.md matches its recorded lock hash was installed by the CLI and never
    edited, so it is safe to remove even across a version bump.
    """
    lock_path = ai_specs / ".ai-specs.lock"
    if not lock_path.is_file():
        return {}
    try:
        with lock_path.open("rb") as f:
            data = tomllib.load(f)
    except (OSError, tomllib.TOMLDecodeError):
        return {}
    skills = data.get("skills")
    if not isinstance(skills, dict):
        return {}
    return {sid: files for sid, files in skills.items() if isinstance(files, dict)}


def remove_bundled_skill_leftovers(
    ai_specs: Path,
    cli_home: Path | None,
    lock_skills: dict | None = None,
) -> None:
    """Delete in-project copies of CLI-bundled skills (now cache-resolved).

    A directory under ``ai-specs/skills/`` is removed only when a bundled skill
    of the same id ships under ``{cli_home}/bundled-skills/`` AND the project
    copy is untouched — i.e. its ``SKILL.md`` is byte-identical (CRLF-normalized)
    to EITHER the current bundled source OR the hash the legacy lock recorded for
    it (migration signal for copies from an older CLI version). Genuine local
    skills (no bundled counterpart) and user-edited copies are preserved.

    ``lock_skills`` may be supplied by a caller that still holds the legacy lock
    in memory (e.g. refresh-bundled, before it normalizes the lock away); when
    omitted the hashes are read from the on-disk lock best-effort.
    """
    home = Path(cli_home) if cli_home is not None else _ai_specs_home()
    bundled_src = home / "bundled-skills"
    skills_dir = ai_specs / "skills"
    if not bundled_src.is_dir() or not skills_dir.is_dir():
        return
    if lock_skills is None:
        lock_skills = _legacy_lock_skill_hashes(ai_specs)
    for child in sorted(skills_dir.iterdir()):
        if not child.is_dir():
            continue
        src_skill = bundled_src / child.name / "SKILL.md"
        proj_skill = child / "SKILL.md"
        if not src_skill.is_file() or not proj_skill.is_file():
            continue
        proj_norm = _normalized_bytes(proj_skill)
        matches_source = proj_norm == _normalized_bytes(src_skill)
        lock_hash = (lock_skills.get(child.name) or {}).get("SKILL.md")
        matches_lock = bool(lock_hash) and hashlib.sha256(proj_norm).hexdigest() == lock_hash
        if not (matches_source or matches_lock):
            _warn(
                f"keeping customized ai-specs/skills/{child.name}/ "
                "(differs from CLI-bundled source and lock; resolve manually)"
            )
            continue
        try:
            shutil.rmtree(child)
            print(f"  ✓ removed leftover bundled skill ai-specs/skills/{child.name}/")
        except OSError as exc:
            _warn(f"failed to remove leftover ai-specs/skills/{child.name}/: {exc}")


def remove_legacy_origin(project_root: Path, cli_home: Path | None = None) -> None:
    """Migrate leftover overrides, then delete in-project origin leftovers.

    Removes legacy origin trees (``.recipe``, ``.deps``), obsolete skill-cache
    dirs (``.resolved-skills``, ``.internal``), the stale shared helper formerly
    staged at ``ai-specs/bin/premerge_guardian.py``, and in-project copies of
    CLI-bundled skills (which now resolve from the cache).
    """
    root = Path(project_root)
    ai_specs = root / "ai-specs"
    legacy_recipe = ai_specs / ".recipe"

    migration_failed = False
    if legacy_recipe.is_dir():
        for recipe_child in sorted(legacy_recipe.iterdir()):
            if not recipe_child.is_dir():
                continue
            legacy_overrides = recipe_child / "overrides"
            if not legacy_overrides.is_dir():
                continue
            dest = ai_specs / "recipes" / recipe_child.name / "overrides"
            if dest.exists():
                continue
            try:
                dest.parent.mkdir(parents=True, exist_ok=True)
                shutil.copytree(legacy_overrides, dest)
                print(
                    f"  ✓ migrated overrides {recipe_child.name} → "
                    f"ai-specs/recipes/{recipe_child.name}/overrides/"
                )
            except OSError as exc:
                _warn(
                    f"failed to migrate overrides for '{recipe_child.name}': {exc}"
                )
                migration_failed = True

    # Only .recipe/ is legacy origin. ai-specs/.deps/ is now the toml-dep home
    # (gitignored) and must NOT be deleted.
    if legacy_recipe.exists():
        if migration_failed:
            _warn(
                "skipping removal of ai-specs/.recipe/ — "
                "one or more override migrations failed; re-run ai-specs sync to retry"
            )
        else:
            try:
                shutil.rmtree(legacy_recipe)
                print("  ✓ removed leftover ai-specs/.recipe/")
            except OSError as exc:
                _warn(f"failed to remove leftover ai-specs/.recipe/: {exc}")

    for path, label in (
        (ai_specs / ".resolved-skills", ".resolved-skills"),
        (ai_specs / ".internal", ".internal"),
    ):
        if not path.exists():
            continue
        try:
            shutil.rmtree(path)
            print(f"  ✓ removed leftover ai-specs/{label}/")
        except OSError as exc:
            _warn(f"failed to remove leftover ai-specs/{label}/: {exc}")

    guardian = ai_specs / "bin" / "premerge_guardian.py"
    if guardian.is_file() or guardian.is_symlink():
        try:
            guardian.unlink()
            print("  ✓ removed leftover ai-specs/bin/premerge_guardian.py")
        except OSError as exc:
            _warn(f"failed to remove leftover ai-specs/bin/premerge_guardian.py: {exc}")

    bin_dir = ai_specs / "bin"
    if bin_dir.is_dir():
        try:
            if not any(bin_dir.iterdir()):
                bin_dir.rmdir()
                print("  ✓ removed empty ai-specs/bin/")
        except OSError as exc:
            _warn(f"failed to remove empty ai-specs/bin/: {exc}")

    remove_bundled_skill_leftovers(ai_specs, cli_home)


def merge_commands(
    project_root: Path,
    dest_dir: Path,
    cli_home: Path | None = None,
) -> int:
    """Merge cache-managed commands with hand-authored ai-specs/commands/.

    Local hand-authored commands win on id conflict. Returns file count in dest.
    """
    dest_dir = Path(dest_dir)
    if dest_dir.exists():
        shutil.rmtree(dest_dir)
    dest_dir.mkdir(parents=True, exist_ok=True)

    count = 0
    managed = commands_dir(project_root, cli_home=cli_home)
    managed_names: set[str] = set()
    if managed.is_dir():
        for src in sorted(managed.glob("*.md")):
            shutil.copy2(src, dest_dir / src.name)
            managed_names.add(src.name)
            count += 1

    local = Path(project_root) / "ai-specs" / "commands"
    if local.is_dir():
        for src in sorted(local.glob("*.md")):
            target = dest_dir / src.name
            if src.name in managed_names:
                _warn(
                    f"command '{src.stem}' present in cache and ai-specs/commands/; "
                    f"local hand-authored wins"
                )
            shutil.copy2(src, target)
            if src.name not in managed_names:
                count += 1

    return count


def main() -> int:
    """CLI helpers for shell callers.

    Usage:
      project-cache.py <project_root> path <resolved-skills|commands|recipe|deps|bundled|root>
      project-cache.py <project_root> merge-commands <dest_dir>
      project-cache.py <project_root> ensure
    """
    if len(sys.argv) < 3:
        print(
            f"Usage: {sys.argv[0]} <project_root> "
            "path <resolved-skills|commands|recipe|deps|bundled|root> | "
            "merge-commands <dest> | ensure",
            file=sys.stderr,
        )
        return 2

    project_root = Path(sys.argv[1]).resolve()
    action = sys.argv[2]

    if action == "ensure":
        print(ensure_cache(project_root))
        return 0

    if action == "path":
        if len(sys.argv) != 4:
            print("Usage: project-cache.py <project_root> path <kind>", file=sys.stderr)
            return 2
        kind = sys.argv[3]
        mapping = {
            "root": cache_root,
            "resolved-skills": resolved_skills_dir,
            "commands": commands_dir,
            "recipe": recipe_skills_root,
            "deps": deps_skills_root,
            "bundled": bundled_skills_root,
        }
        if kind not in mapping:
            print(f"unknown path kind: {kind}", file=sys.stderr)
            return 2
        ensure_cache(project_root)
        print(mapping[kind](project_root))
        return 0

    if action == "merge-commands":
        if len(sys.argv) != 4:
            print(
                "Usage: project-cache.py <project_root> merge-commands <dest>",
                file=sys.stderr,
            )
            return 2
        ensure_cache(project_root)
        dest = Path(sys.argv[3])
        n = merge_commands(project_root, dest)
        print(f"  ✓ merged {n} command file(s) → {dest}")
        return 0

    print(f"unknown action: {action}", file=sys.stderr)
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
