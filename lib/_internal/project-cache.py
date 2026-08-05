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
import subprocess
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


def bundled_commands_root(project_root: Path, cli_home: Path | None = None) -> Path:
    """CLI-bundled commands flattened into the cache (recipe-independent)."""
    return bundled_skills_root(project_root, cli_home=cli_home) / "commands"


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

    # Disk cleanup does not touch the git index. Guide the developer when
    # removed bundled skills are still tracked (never run git rm here).
    leftovers = tracked_bundled_skill_leftovers(ai_specs.parent, cli_home=home)
    if leftovers:
        print(format_tracked_bundled_remediation(leftovers))


def _legacy_lock_command_hashes(ai_specs: Path) -> dict[str, str]:
    """Read the legacy [commands] hash table from an old lock (best-effort).

    Used only as a migration signal: a committed bundled-command copy whose
    content matches its recorded lock hash was installed by the CLI and never
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
    commands = data.get("commands")
    if not isinstance(commands, dict):
        return {}
    return {name: sha for name, sha in commands.items() if isinstance(sha, str)}


def remove_bundled_command_leftovers(
    ai_specs: Path,
    cli_home: Path | None,
    lock_commands: dict | None = None,
) -> None:
    """Delete in-project copies of CLI-bundled commands (now cache-resolved).

    A ``*.md`` file directly under ``ai-specs/commands/`` is removed only when
    a bundled command of the same name ships under
    ``{cli_home}/bundled-commands/`` AND the project copy is untouched — i.e.
    byte-identical (CRLF-normalized) to EITHER the current bundled source OR
    the hash the legacy lock recorded for it (migration signal for copies from
    an older CLI version). Genuine local commands (no bundled counterpart) and
    user-edited copies are preserved.

    ``lock_commands`` may be supplied by a caller that still holds the legacy
    lock in memory (e.g. refresh-bundled, before it normalizes the lock away);
    when omitted the hashes are read from the on-disk lock best-effort.
    """
    home = Path(cli_home) if cli_home is not None else _ai_specs_home()
    bundled_src = home / "bundled-commands"
    local_commands_dir = ai_specs / "commands"
    if not bundled_src.is_dir() or not local_commands_dir.is_dir():
        return
    if lock_commands is None:
        lock_commands = _legacy_lock_command_hashes(ai_specs)
    for child in sorted(local_commands_dir.iterdir()):
        if not child.is_file() or child.suffix != ".md":
            continue
        src_cmd = bundled_src / child.name
        if not src_cmd.is_file():
            continue
        proj_norm = _normalized_bytes(child)
        matches_source = proj_norm == _normalized_bytes(src_cmd)
        lock_hash = lock_commands.get(child.name)
        matches_lock = bool(lock_hash) and hashlib.sha256(proj_norm).hexdigest() == lock_hash
        if not (matches_source or matches_lock):
            _warn(
                f"keeping customized ai-specs/commands/{child.name} "
                "(differs from CLI-bundled source and lock; resolve manually)"
            )
            continue
        try:
            child.unlink()
            print(f"  ✓ removed leftover bundled command ai-specs/commands/{child.name}")
        except OSError as exc:
            _warn(f"failed to remove leftover ai-specs/commands/{child.name}: {exc}")

    # Disk cleanup does not touch the git index. Guide the developer when
    # removed bundled commands are still tracked (never run git rm here).
    leftovers = tracked_bundled_command_leftovers(ai_specs.parent, cli_home=home)
    if leftovers:
        print(
            format_tracked_bundled_remediation(
                leftovers,
                kind="command",
                path_template="ai-specs/commands/{name}.md",
                recursive=False,
            )
        )


def remove_recipe_command_leftovers(
    project_root: Path,
    cli_home: Path | None = None,
    lock_commands: dict[str, str] | None = None,
    recipe_sources: dict[str, Path] | None = None,
) -> None:
    """Delete untouched recipe-managed commands left in ``ai-specs/commands``.

    Recipe commands now materialize in the per-project cache. A project copy is
    removed only when it matches the currently cached recipe command, a current
    catalog recipe source, or its recorded hash in the legacy ``[commands]``
    lock table. Files that differ from all available provenance are treated as
    local/customized commands and are preserved.
    """
    root = Path(project_root)
    ai_specs = root / "ai-specs"
    local_commands_dir = ai_specs / "commands"
    managed_commands_dir = commands_dir(root, cli_home=cli_home)
    if not local_commands_dir.is_dir():
        return
    if lock_commands is None:
        lock_commands = _legacy_lock_command_hashes(ai_specs)
    recipe_sources = recipe_sources or {}

    for child in sorted(local_commands_dir.iterdir()):
        if not child.is_file() or child.suffix != ".md":
            continue
        managed = managed_commands_dir / child.name
        source = recipe_sources.get(child.name)
        lock_hash = lock_commands.get(child.name)
        if not managed.is_file() and not (source and source.is_file()) and not lock_hash:
            continue

        project_bytes = _normalized_bytes(child)
        matches_managed = managed.is_file() and project_bytes == _normalized_bytes(managed)
        matches_source = source is not None and source.is_file() and project_bytes == _normalized_bytes(source)
        matches_lock = bool(lock_hash) and hashlib.sha256(project_bytes).hexdigest() == lock_hash
        if not (matches_managed or matches_source or matches_lock):
            _warn(
                f"keeping local/customized ai-specs/commands/{child.name} "
                "(differs from recipe-managed cache/source and legacy provenance; resolve manually)"
            )
            continue
        try:
            child.unlink()
            print(f"  ✓ removed leftover recipe command ai-specs/commands/{child.name}")
        except OSError as exc:
            _warn(f"failed to remove leftover ai-specs/commands/{child.name}: {exc}")


def bundled_skill_ids(cli_home: Path | None = None) -> list[str]:
    """Directory names under ``bundled-skills/`` that ship a ``SKILL.md``."""
    home = Path(cli_home) if cli_home is not None else _ai_specs_home()
    root = home / "bundled-skills"
    if not root.is_dir():
        return []
    return sorted(
        p.name
        for p in root.iterdir()
        if p.is_dir() and (p / "SKILL.md").is_file()
    )


def bundled_command_ids(cli_home: Path | None = None) -> list[str]:
    """``.md`` stems shipped under ``bundled-commands/``."""
    home = Path(cli_home) if cli_home is not None else _ai_specs_home()
    root = home / "bundled-commands"
    if not root.is_dir():
        return []
    return sorted(p.stem for p in root.glob("*.md") if p.is_file())


def _is_git_work_tree(project_root: Path) -> bool:
    try:
        result = subprocess.run(
            ["git", "-C", str(project_root), "rev-parse", "--is-inside-work-tree"],
            capture_output=True,
            text=True,
            check=False,
        )
    except OSError:
        return False
    return result.returncode == 0 and result.stdout.strip() == "true"


def _git_ls_files(project_root: Path, pathspec: str) -> list[str]:
    try:
        result = subprocess.run(
            ["git", "-C", str(project_root), "ls-files", "--", pathspec],
            capture_output=True,
            text=True,
            check=False,
        )
    except OSError:
        return []
    if result.returncode != 0:
        return []
    return [ln.strip() for ln in result.stdout.splitlines() if ln.strip()]


def _tracked_bundled_leftovers(
    project_root: Path, bundled_ids: list[str], path_template: str
) -> list[str]:
    """Bundled ids still tracked in git after their working-tree copy is gone.

    ``path_template`` is formatted with ``name=<id>`` to build both the
    on-disk existence check and the ``git ls-files`` pathspec. Does not
    mutate the index. Empty when not a git work tree.
    """
    root = Path(project_root)
    if not _is_git_work_tree(root):
        return []
    leftover_ids: list[str] = []
    for bundled_id in bundled_ids:
        rel = path_template.format(name=bundled_id)
        if (root / rel).exists():
            continue
        if _git_ls_files(root, rel):
            leftover_ids.append(bundled_id)
    return leftover_ids


def tracked_bundled_skill_leftovers(
    project_root: Path, cli_home: Path | None = None
) -> list[str]:
    """Bundled skill ids still tracked in git after the working-tree copy is gone.

    Does not mutate the index. Empty when not a git work tree.
    """
    return _tracked_bundled_leftovers(
        project_root, bundled_skill_ids(cli_home), "ai-specs/skills/{name}"
    )


def tracked_bundled_command_leftovers(
    project_root: Path, cli_home: Path | None = None
) -> list[str]:
    """Bundled command ids still tracked in git after the working-tree copy is gone.

    Does not mutate the index. Empty when not a git work tree.
    """
    return _tracked_bundled_leftovers(
        project_root, bundled_command_ids(cli_home), "ai-specs/commands/{name}.md"
    )


def format_tracked_bundled_remediation(
    bundled_ids: list[str],
    *,
    kind: str = "skill",
    path_template: str = "ai-specs/skills/{name}",
    recursive: bool = True,
) -> str:
    """Human-readable remediation; never executes git."""
    paths = " ".join(path_template.format(name=bid) for bid in bundled_ids)
    names = ", ".join(bundled_ids)
    flag = "-r --cached" if recursive else "--cached"
    return (
        f"  ℹ git still tracks removed CLI-bundled {kind}(s): {names}\n"
        f"    To stop committing them (ai-specs will not modify the index):\n"
        f"    git rm {flag} {paths}\n"
        f"    # then commit when ready"
    )


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
    """Merge bundled, recipe-managed, and hand-authored commands.

    Ascending precedence copy order: CLI-bundled (``{cache}/.bundled/commands``)
    -> recipe-managed (``{cache}/commands``) -> local hand-authored
    (``ai-specs/commands/``). Recipe-managed silently overrides bundled (both
    CLI-driven tiers, no user-facing signal); local hand-authored wins on id
    conflict with either lower tier and warns. Returns file count in dest.
    """
    dest_dir = Path(dest_dir)
    if dest_dir.exists():
        shutil.rmtree(dest_dir)
    dest_dir.mkdir(parents=True, exist_ok=True)

    count = 0
    seen: set[str] = set()

    bundled = bundled_commands_root(project_root, cli_home=cli_home)
    if bundled.is_dir():
        for src in sorted(bundled.glob("*.md")):
            shutil.copy2(src, dest_dir / src.name)
            seen.add(src.name)
            count += 1

    managed = commands_dir(project_root, cli_home=cli_home)
    if managed.is_dir():
        for src in sorted(managed.glob("*.md")):
            shutil.copy2(src, dest_dir / src.name)
            if src.name not in seen:
                count += 1
            seen.add(src.name)

    local = Path(project_root) / "ai-specs" / "commands"
    if local.is_dir():
        for src in sorted(local.glob("*.md")):
            target = dest_dir / src.name
            if src.name in seen:
                _warn(
                    f"command '{src.stem}' present in cache and ai-specs/commands/; "
                    f"local hand-authored wins"
                )
            shutil.copy2(src, target)
            if src.name not in seen:
                count += 1
            seen.add(src.name)

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
