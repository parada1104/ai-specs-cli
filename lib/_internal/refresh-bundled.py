#!/usr/bin/env python3
"""Refresh bundled skills and commands.

CLI-bundled SKILLS and COMMANDS both flatten into the cache
(``{cache}/.bundled/skills/`` and ``{cache}/.bundled/commands/`` respectively —
see ``flatten_bundled_skills``/``flatten_bundled_commands``) and are NEVER
written into the project surface. refresh-bundled is a pure cache-repair verb:
it flattens both asset kinds, migrates away any leftover in-project copies
from a project that predates this relocation
(``remove_bundled_skill_leftovers``/``remove_bundled_command_leftovers``), and
stamps the lock (``[meta]`` + ``[agents.*]`` only — see lock.py). There is no
per-file content-hash diffing and no ``.new`` sidecar is ever written.

Usage:
  refresh-bundled.py <project_root> <ai_specs_home> [--init]

``--init`` is accepted for backward-compatible invocation (e.g. from
``init.sh``) but no longer changes behavior — flatten-only has no
customization-detection step to skip.
"""

from __future__ import annotations

import importlib.util
import shutil
import sys
from pathlib import Path

LOCK_REL = "ai-specs/.ai-specs.lock"


def _load_lock_module():
    module_path = Path(__file__).with_name("lock.py")
    spec = importlib.util.spec_from_file_location("lock_internal", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load lock.py at {module_path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module

_lock_mod = _load_lock_module()
load_lock = _lock_mod.load_lock
write_lock = _lock_mod.write_lock


def _load_project_cache():
    module_path = Path(__file__).with_name("project-cache.py")
    spec = importlib.util.spec_from_file_location("project_cache_refresh", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load project-cache.py at {module_path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def flatten_bundled_skills(cli_source: Path, project: Path, cli_home: Path) -> int:
    """Flatten CLI-bundled skills into the cache at {cache}/.bundled/skills/.

    CLI-bundled skills are recipe-independent and NEVER written into the project
    surface. The destination is rebuilt each run so upstream removals propagate.
    """
    pc = _load_project_cache()
    dest = pc.bundled_skills_root(project, cli_home=cli_home) / "skills"
    if dest.exists():
        shutil.rmtree(dest)
    src = cli_source / "bundled-skills"
    count = 0
    if src.is_dir():
        dest.mkdir(parents=True, exist_ok=True)
        for skill_dir in sorted(p for p in src.iterdir() if p.is_dir()):
            shutil.copytree(skill_dir, dest / skill_dir.name)
            count += 1
    return count


def flatten_bundled_commands(cli_source: Path, project: Path, cli_home: Path) -> int:
    """Flatten CLI-bundled commands into the cache at {cache}/.bundled/commands/.

    CLI-bundled commands are recipe-independent and NEVER written into the
    project surface. Every ``*.md`` directly under ``bundled-commands/`` is
    copied flat (no subdirectories, unlike skills). The destination is rebuilt
    each run so upstream removals propagate.
    """
    pc = _load_project_cache()
    dest = pc.bundled_commands_root(project, cli_home=cli_home)
    if dest.exists():
        shutil.rmtree(dest)
    src = cli_source / "bundled-commands"
    count = 0
    if src.is_dir():
        dest.mkdir(parents=True, exist_ok=True)
        for cmd_file in sorted(p for p in src.glob("*.md") if p.is_file()):
            shutil.copy2(cmd_file, dest / cmd_file.name)
            count += 1
    return count


def refresh(
    project: Path, cli_source: Path, init_mode: bool = False
) -> int:
    # CLI-bundled skills and commands flatten into the cache — never into the
    # project surface.
    n_skills = flatten_bundled_skills(cli_source, project, cli_source)
    n_commands = flatten_bundled_commands(cli_source, project, cli_source)

    lock_path = project / LOCK_REL
    lock = load_lock(lock_path)

    # Migrate away any in-project bundled copies while the legacy lock (with
    # [skills.*] hashes) is still in memory — write_lock below drops that
    # section, so this must happen first for the lock-hash migration signal
    # (mirrors the pre-existing skill migration ordering). ``load_lock`` no
    # longer exposes a legacy ``[commands]`` key (dropped in Phase 4), so the
    # command variant self-reads the raw on-disk lock (still present at this
    # point, before write_lock below normalizes it away).
    pc = _load_project_cache()
    pc.remove_bundled_skill_leftovers(
        project / "ai-specs", cli_source, lock.get("skills") or {}
    )
    pc.remove_bundled_command_leftovers(project / "ai-specs", cli_source)

    write_lock(lock_path, lock)

    if n_skills:
        print(f"  ⇢ flattened {n_skills} bundled skill(s) → cache/.bundled/skills")
    if n_commands:
        print(f"  ⇢ flattened {n_commands} bundled command(s) → cache/.bundled/commands")
    if not n_skills and not n_commands:
        print("  = all bundled up-to-date")
    return 0


def main() -> int:
    args = list(sys.argv[1:])
    init_mode = False
    if "--init" in args:
        args.remove("--init")
        init_mode = True
    if len(args) != 2:
        print(
            "Usage: refresh-bundled.py <project_root> <ai_specs_home> [--init]",
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

    return refresh(project, cli_source, init_mode=init_mode)


if __name__ == "__main__":
    sys.exit(main())
