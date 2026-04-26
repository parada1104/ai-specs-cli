#!/usr/bin/env python3
"""Refresh bundled skills and commands, respecting user customizations.

Algorithm: content-hash + lock file at <project>/ai-specs/.ai-specs.lock.

For each bundled file shipped by the CLI:
  - cli_sha  = sha256 of CLI's current source
  - proj_sha = sha256 of the user's project copy
  - lock_sha = sha256 recorded in the lock file the last time we installed

Decisions (evaluated in order):
  - name in opted_out + proj missing → silent skip (respect opt-out)
  - name in opted_out + proj exists  → user changed their mind; drop from
                                       opted_out and fall through
  - proj missing + no lock           → first install (copy, record)
  - proj missing + lock present      → user deleted; remove from lock,
                                       add to opted_out (permanent)
  - proj == cli                      → nothing to ship; sync lock to cli_sha
                                       if stale (covers clean state, post-mv
                                       resolution, manual match to upstream)
  - no lock entry + proj != cli      → customized baseline:
      * --init: record cli_sha, do not write .new
      * default: save CLI copy as <name>.new and record cli_sha
  - lock present + proj == lock      → user hasn't touched; cli has moved
                                       (proj == cli handled above) → auto-update
  - lock present + proj != lock:
      * cli != lock → customized + upstream moved: save .new
      * cli == lock → customized, no upstream change: nothing

Lock entries for files removed upstream are purged.
Line endings are normalized (CRLF → LF) before hashing.

Usage:
  refresh-bundled.py <project_root> <ai_specs_home> [--init]
"""

from __future__ import annotations

import hashlib
import shutil
import sys
import tomllib
from pathlib import Path
from typing import Iterator, Optional

LOCK_REL = "ai-specs/.ai-specs.lock"

# Preset `openspec`: bundled skills named openspec-* plus catalog policy skills.
PRESET_OPENSPEC_CATALOG_SKILLS = ("openspec-sdd-conventions", "testing-foundation")

LOCK_HEADER = """\
# Managed by ai-specs. Do not edit by hand.
# Tracks SHA-256 of bundled files as last installed by the CLI.
# Used by `ai-specs refresh-bundled` to detect user customizations.
"""


def sha256_of(path: Path) -> str:
    """Hash file content with CRLF → LF normalization."""
    data = path.read_bytes().replace(b"\r\n", b"\n")
    return hashlib.sha256(data).hexdigest()


def iter_bundled(cli_source: Path) -> Iterator[tuple[str, Optional[str], str, Path]]:
    """Yield (kind, owner, rel, abs_path) for each bundled file.

    kind in {"skill", "command"}; owner is skill name for skills, None for commands.
    rel is path relative to the skill dir (for skills) or to bundled-commands (for commands).
    """
    skills_dir = cli_source / "bundled-skills"
    if skills_dir.is_dir():
        for skill_dir in sorted(p for p in skills_dir.iterdir() if p.is_dir()):
            for p in sorted(skill_dir.rglob("*")):
                if p.is_file():
                    yield "skill", skill_dir.name, str(p.relative_to(skill_dir)), p

    commands_dir = cli_source / "bundled-commands"
    if commands_dir.is_dir():
        for p in sorted(commands_dir.rglob("*")):
            if p.is_file():
                yield "command", None, str(p.relative_to(commands_dir)), p


def iter_catalog_skill(cli_source: Path, skill_name: str) -> Iterator[tuple[str, str, str, Path]]:
    base = cli_source / "catalog" / "skills" / skill_name
    if not base.is_dir():
        return
    for p in sorted(base.rglob("*")):
        if p.is_file():
            yield "skill", skill_name, str(p.relative_to(base)), p


def preset_match(preset: Optional[str], kind: str, owner: Optional[str], rel: str) -> bool:
    if preset is None:
        return True
    if preset != "openspec":
        raise ValueError(f"unknown preset: {preset!r}")
    if kind == "command":
        return True
    assert owner is not None
    if owner.startswith("openspec-"):
        return True
    if owner in PRESET_OPENSPEC_CATALOG_SKILLS:
        return True
    return False


def iter_preset_sources(cli_source: Path, preset: Optional[str]) -> Iterator[tuple[str, Optional[str], str, Path]]:
    for kind, owner, rel, cli_path in iter_bundled(cli_source):
        if preset_match(preset, kind, owner, rel):
            yield kind, owner, rel, cli_path
    if preset == "openspec":
        for name in PRESET_OPENSPEC_CATALOG_SKILLS:
            yield from iter_catalog_skill(cli_source, name)


def project_path_for(project: Path, kind: str, owner: Optional[str], rel: str) -> Path:
    base = project / "ai-specs"
    if kind == "skill":
        return base / "skills" / owner / rel
    if kind == "command":
        return base / "commands" / rel
    raise ValueError(f"unknown kind: {kind}")


def display_name(kind: str, owner: Optional[str], rel: str) -> str:
    if kind == "skill":
        return f"skills/{owner}/{rel}"
    return f"commands/{rel}"


def load_lock(lock_path: Path) -> dict:
    if not lock_path.is_file():
        return {"skills": {}, "commands": {}, "opted_out": []}
    with lock_path.open("rb") as f:
        data = tomllib.load(f)
    return {
        "skills": {k: dict(v) for k, v in (data.get("skills") or {}).items()},
        "commands": dict(data.get("commands") or {}),
        "opted_out": list(data.get("opted-out", {}).get("files", []) or []),
    }


def write_lock(lock_path: Path, lock: dict) -> None:
    """Write lock file as TOML (handmade; stdlib tomllib is read-only)."""
    out = [LOCK_HEADER]
    skills = lock.get("skills") or {}
    for skill in sorted(skills):
        files = skills[skill]
        if not files:
            continue
        out.append(f'[skills."{skill}"]')
        for rel in sorted(files):
            out.append(f'"{rel}" = "{files[rel]}"')
        out.append("")
    commands = lock.get("commands") or {}
    if commands:
        out.append("[commands]")
        for rel in sorted(commands):
            out.append(f'"{rel}" = "{commands[rel]}"')
        out.append("")
    opted = sorted(set(lock.get("opted_out") or []))
    if opted:
        out.append("[opted-out]")
        out.append("# Bundled files the user deleted intentionally; the CLI will")
        out.append("# not re-install them. Remove a line here to let the file be")
        out.append("# restored on the next `ai-specs refresh-bundled`.")
        formatted = ", ".join(f'"{name}"' for name in opted)
        out.append(f"files = [{formatted}]")
        out.append("")
    lock_path.parent.mkdir(parents=True, exist_ok=True)
    lock_path.write_text("\n".join(out).rstrip("\n") + "\n")


def lock_get(lock: dict, kind: str, owner: Optional[str], rel: str) -> Optional[str]:
    if kind == "skill":
        return (lock["skills"].get(owner) or {}).get(rel)
    return lock["commands"].get(rel)


def lock_set(lock: dict, kind: str, owner: Optional[str], rel: str, sha: str) -> None:
    if kind == "skill":
        lock["skills"].setdefault(owner, {})[rel] = sha
    else:
        lock["commands"][rel] = sha


def lock_del(lock: dict, kind: str, owner: Optional[str], rel: str) -> None:
    if kind == "skill":
        files = lock["skills"].get(owner) or {}
        files.pop(rel, None)
        if not files:
            lock["skills"].pop(owner, None)
    else:
        lock["commands"].pop(rel, None)


def save_new_sidecar(cli_path: Path, proj_path: Path) -> Path:
    sidecar = proj_path.with_name(proj_path.name + ".new")
    shutil.copy2(cli_path, sidecar)
    return sidecar


def refresh(
    project: Path, cli_source: Path, init_mode: bool = False, preset: Optional[str] = None
) -> int:
    lock_path = project / LOCK_REL
    lock = load_lock(lock_path)

    touched: list[tuple[str, str, str]] = []
    seen: set[tuple[str, Optional[str], str]] = set()
    opted_out: set[str] = set(lock.get("opted_out") or [])

    for kind, owner, rel, cli_path in iter_preset_sources(cli_source, preset):
        key = (kind, owner, rel)
        seen.add(key)

        cli_sha = sha256_of(cli_path)
        proj_path = project_path_for(project, kind, owner, rel)
        lock_sha = lock_get(lock, kind, owner, rel)
        name = display_name(kind, owner, rel)

        if name in opted_out:
            if proj_path.exists():
                # User changed their mind (re-created the file) — drop opt-out.
                opted_out.discard(name)
                touched.append(("+", name, "opt-out lifted (file restored)"))
                # Fall through to normal processing below.
            else:
                # Silently skip — respect permanent opt-out.
                continue

        if not proj_path.exists():
            if lock_sha is None:
                # First install (fresh project or CLI added a new bundled file).
                proj_path.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(cli_path, proj_path)
                lock_set(lock, kind, owner, rel, cli_sha)
                touched.append(("+", name, "installed"))
            else:
                # User deleted a previously-tracked file → record opt-out.
                lock_del(lock, kind, owner, rel)
                opted_out.add(name)
                touched.append(("-", name, "opted-out (deleted by user)"))
            continue

        proj_sha = sha256_of(proj_path)

        # Short-circuit: project already matches CLI exactly. This covers the
        # post-`mv .new SKILL.md` resolution case where the user accepted
        # upstream and the lock is now stale against reality.
        if proj_sha == cli_sha:
            if lock_sha != cli_sha:
                lock_set(lock, kind, owner, rel, cli_sha)
                if lock_sha is None:
                    touched.append(("=", name, "tracked"))
                else:
                    touched.append(("✓", name, "accepted upstream"))
            # else: already in sync, silent.
            continue

        # From here: proj_sha != cli_sha.
        if lock_sha is None:
            if init_mode:
                # Pre-lock customization — record CLI baseline without overwriting.
                lock_set(lock, kind, owner, rel, cli_sha)
                touched.append(("~", name, "tracked (customized)"))
            else:
                sidecar = save_new_sidecar(cli_path, proj_path)
                lock_set(lock, kind, owner, rel, cli_sha)
                touched.append(("~", name, f"customized → saved {sidecar.name}"))
            continue

        if proj_sha == lock_sha:
            # User hasn't touched; CLI has moved (proj == cli was handled above).
            shutil.copy2(cli_path, proj_path)
            lock_set(lock, kind, owner, rel, cli_sha)
            touched.append(("✓", name, "updated"))
            continue

        # proj_sha != lock_sha → user customized since last install.
        if cli_sha != lock_sha:
            sidecar = save_new_sidecar(cli_path, proj_path)
            touched.append(("~", name, f"customized → saved {sidecar.name}"))
        # else: user customized, CLI unchanged → nothing to do.

    # Purge lock entries for files the CLI no longer ships (full refresh only).
    if preset is None:
        stale: list[tuple[str, Optional[str], str]] = []
        for skill, files in lock["skills"].items():
            for rel in files:
                if ("skill", skill, rel) not in seen:
                    stale.append(("skill", skill, rel))
        for rel in lock["commands"]:
            if ("command", None, rel) not in seen:
                stale.append(("command", None, rel))
        for kind, owner, rel in stale:
            lock_del(lock, kind, owner, rel)
            touched.append(("-", display_name(kind, owner, rel), "untracked (removed upstream)"))

        shipped_names = {
            display_name(k, o, r) for k, o, r, _cli in iter_bundled(cli_source)
        }
        opted_out &= shipped_names
    lock["opted_out"] = sorted(opted_out)

    write_lock(lock_path, lock)

    if touched:
        for sym, name, msg in touched:
            print(f"  {sym} {name}  {msg}")
    else:
        print("  = all bundled up-to-date")
    return 0


def main() -> int:
    args = list(sys.argv[1:])
    init_mode = False
    if "--init" in args:
        args.remove("--init")
        init_mode = True
    preset: Optional[str] = None
    if "--preset" in args:
        i = args.index("--preset")
        try:
            preset = args[i + 1]
        except IndexError:
            print("ERROR: --preset requires a value", file=sys.stderr)
            return 2
        args = args[:i] + args[i + 2 :]
    if len(args) != 2:
        print(
            "Usage: refresh-bundled.py <project_root> <ai_specs_home> [--init] [--preset NAME]",
            file=sys.stderr,
        )
        return 2

    project = Path(args[0]).resolve()
    cli_source = Path(args[1]).resolve()

    if not (project / "ai-specs").is_dir():
        print(
            f"ERROR: {project}/ai-specs not found. Run `ai-specs init` first.",
            file=sys.stderr,
        )
        return 1

    return refresh(project, cli_source, init_mode=init_mode, preset=preset)


if __name__ == "__main__":
    sys.exit(main())
