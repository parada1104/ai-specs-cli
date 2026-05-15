#!/usr/bin/env python3
"""Materialize SDD subagent files into harness-specific directories.

Usage:
  agents-render.py <project_root> <ai_specs_home>

Reads <project_root>/ai-specs/ai-specs.toml. When ``[sdd].sub_agents = true``,
materializes ``<ai_specs_home>/bundled-agents/<harness>/*.md`` into the
harness-specific destination for every harness in ``[agents].enabled`` that
supports subagents natively. Records hashes in ``ai-specs/.ai-specs.lock``
under ``[agents."<harness>"]`` and writes ``.new`` sidecars for user-modified
files (the same pattern used by ``refresh-bundled.py``).

Behavior summary:
- ``sub_agents`` false/absent: detect orphan ``.claude/agents/sdd-*.md`` and
  warn (never delete). Lock state is left intact so a later re-enable does not
  trigger sidecars unnecessarily.
- ``sub_agents`` true:
  * For each supported harness in ``[agents].enabled``: materialize.
  * For each unsupported harness (opencode, cursor, …): log fallback to inline
    orchestration; no files written.
- Re-running with the same configuration is idempotent.

Reuses ``lock.py`` helpers via ``importlib`` so the same lock file is shared
with bundled skills/commands.
"""

from __future__ import annotations

import hashlib
import importlib.util
import shutil
import sys
import tomllib
from pathlib import Path
from typing import Iterator

LOCK_REL = "ai-specs/.ai-specs.lock"

# Harness → destination directory for materialized subagent files.
# Adding a harness here is the only enabling switch needed once bundled files
# exist under ``bundled-agents/<harness>/``.
SUPPORTED_HARNESSES = {
    "claude": ".claude/agents",
}

# Canonical SDD subagent slugs. Used by orphan detection when the feature is
# turned off but previously-materialized files may still be on disk.
SDD_SUBAGENT_NAMES: tuple[str, ...] = (
    "sdd-explore",
    "sdd-proposal",
    "sdd-artifacts",
    "sdd-apply",
    "sdd-verify",
    "sdd-archive",
)


def _load_lock_module():
    """Import ``lock.py`` as a sibling module without triggering package install."""
    module_path = Path(__file__).with_name("lock.py")
    spec = importlib.util.spec_from_file_location("lock_internal_agents", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load lock.py at {module_path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


_lock_mod = _load_lock_module()
load_lock = _lock_mod.load_lock
write_lock = _lock_mod.write_lock


def sha256_of(path: Path) -> str:
    """Hash file content with CRLF → LF normalization (matches refresh-bundled)."""
    data = path.read_bytes().replace(b"\r\n", b"\n")
    return hashlib.sha256(data).hexdigest()


def _load_manifest(toml_path: Path) -> dict:
    if not toml_path.is_file():
        return {}
    with toml_path.open("rb") as f:
        return tomllib.load(f)


def read_sub_agents_flag(data: dict) -> bool:
    """Return ``[sdd].sub_agents`` strictly as a boolean.

    Raises ``ValueError`` with an explicit message when the value is present
    but not a boolean. Missing ``[sdd]`` or missing ``sub_agents`` returns
    ``False`` (the documented default).
    """
    sdd = data.get("sdd")
    if not isinstance(sdd, dict):
        return False
    value = sdd.get("sub_agents", False)
    if not isinstance(value, bool):
        raise ValueError(
            "[sdd].sub_agents must be a boolean "
            f"(got {type(value).__name__}: {value!r})"
        )
    return value


def read_enabled_agents(data: dict) -> list[str]:
    agents = data.get("agents")
    if not isinstance(agents, dict):
        return []
    enabled = agents.get("enabled", [])
    if not isinstance(enabled, list):
        return []
    return [a for a in enabled if isinstance(a, str)]


def iter_bundled_for_harness(cli_source: Path, harness: str) -> Iterator[tuple[str, Path]]:
    base = cli_source / "bundled-agents" / harness
    if not base.is_dir():
        return
    for path in sorted(base.glob("*.md")):
        if path.is_file():
            yield path.name, path


def materialize_for_harness(
    project: Path,
    cli_source: Path,
    harness: str,
    lock: dict,
) -> list[tuple[str, str, str]]:
    """Materialize ``bundled-agents/<harness>/*.md`` into the project.

    Mirrors the decision table in ``refresh-bundled.py`` so behavior is
    consistent: first install copies, byte-match updates the lock, divergence
    yields ``.new`` sidecars, upstream-removed files are untracked.
    """
    dest_rel = SUPPORTED_HARNESSES[harness]
    dest_dir = project / dest_rel
    lock_agents = lock.setdefault("agents", {}).setdefault(harness, {})

    touched: list[tuple[str, str, str]] = []
    seen: set[str] = set()

    for name, cli_path in iter_bundled_for_harness(cli_source, harness):
        seen.add(name)
        cli_sha = sha256_of(cli_path)
        proj_path = dest_dir / name
        lock_sha = lock_agents.get(name)
        display = f"{dest_rel}/{name}"

        if not proj_path.exists():
            if lock_sha is None:
                # First install for this project.
                proj_path.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(cli_path, proj_path)
                lock_agents[name] = cli_sha
                touched.append(("+", display, "installed"))
            else:
                # User deleted a previously-tracked file. Untrack rather than
                # re-install — they will get a clean install if they re-enable.
                del lock_agents[name]
                touched.append(("-", display, "untracked (deleted by user)"))
            continue

        proj_sha = sha256_of(proj_path)

        if proj_sha == cli_sha:
            if lock_sha != cli_sha:
                lock_agents[name] = cli_sha
                if lock_sha is None:
                    touched.append(("=", display, "tracked"))
                else:
                    touched.append(("✓", display, "accepted upstream"))
            continue

        # proj_sha != cli_sha — divergence requires user attention.
        if lock_sha is None:
            sidecar = _save_new_sidecar(cli_path, proj_path)
            lock_agents[name] = cli_sha
            touched.append(("~", display, f"customized → saved {sidecar.name}"))
            continue

        if proj_sha == lock_sha:
            # User hasn't touched the file; CLI moved → safe to update.
            shutil.copy2(cli_path, proj_path)
            lock_agents[name] = cli_sha
            touched.append(("✓", display, "updated"))
            continue

        # Both moved: user customized AND upstream changed.
        if cli_sha != lock_sha:
            sidecar = _save_new_sidecar(cli_path, proj_path)
            touched.append(("~", display, f"customized → saved {sidecar.name}"))
        # else: user customized, upstream unchanged → leave as-is.

    # Purge lock entries for files we no longer ship from this harness.
    for stale_name in sorted(set(lock_agents) - seen):
        del lock_agents[stale_name]
        touched.append(
            ("-", f"{dest_rel}/{stale_name}", "untracked (removed upstream)")
        )

    return touched


def _save_new_sidecar(cli_path: Path, proj_path: Path) -> Path:
    sidecar = proj_path.with_name(proj_path.name + ".new")
    shutil.copy2(cli_path, sidecar)
    return sidecar


def detect_orphans(project: Path) -> list[Path]:
    """Return orphan SDD subagent files when ``sub_agents`` is off."""
    orphans: list[Path] = []
    for harness, dest_rel in SUPPORTED_HARNESSES.items():
        agents_dir = project / dest_rel
        if not agents_dir.is_dir():
            continue
        for name in SDD_SUBAGENT_NAMES:
            candidate = agents_dir / f"{name}.md"
            if candidate.is_file():
                orphans.append(candidate)
    return orphans


def render(project: Path, cli_source: Path) -> int:
    toml_path = project / "ai-specs" / "ai-specs.toml"
    data = _load_manifest(toml_path)

    try:
        flag = read_sub_agents_flag(data)
    except ValueError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1

    lock_path = project / LOCK_REL

    if not flag:
        # Feature off → no writes, no lock mutations. Warn on orphans so users
        # know the materialized copies will not receive upstream updates.
        orphans = detect_orphans(project)
        if orphans:
            print(
                "  ⚠ [sdd].sub_agents is off but SDD subagent files exist:"
            )
            for orphan in orphans:
                try:
                    rel = orphan.relative_to(project)
                except ValueError:
                    rel = orphan
                print(f"    - {rel}")
            print(
                "    These files will not be updated. "
                "Remove them manually if no longer needed."
            )
        return 0

    lock = load_lock(lock_path)
    enabled = read_enabled_agents(data)

    all_touched: list[tuple[str, str, str]] = []
    supported_present = False

    for harness in enabled:
        if harness in SUPPORTED_HARNESSES:
            supported_present = True
            touched = materialize_for_harness(project, cli_source, harness, lock)
            all_touched.extend(touched)
        else:
            print(
                f"  i {harness}: sub_agents active, no native subagent support "
                "— SDD phases will run inline via the orchestrator"
            )

    # Only write the lock if we made changes. ``load_lock`` returned defaults
    # for a missing file, so persist only when we actually own state for this
    # harness section.
    if supported_present:
        write_lock(lock_path, lock)

    if all_touched:
        for sym, name, msg in all_touched:
            print(f"  {sym} {name}  {msg}")
    elif supported_present:
        print("  = all subagent files up-to-date")

    return 0


def main() -> int:
    if len(sys.argv) != 3:
        print(
            "Usage: agents-render.py <project_root> <ai_specs_home>",
            file=sys.stderr,
        )
        return 2

    project = Path(sys.argv[1]).resolve()
    cli_source = Path(sys.argv[2]).resolve()

    if not (project / "ai-specs").is_dir():
        print(
            f"ERROR: {project}/ai-specs not found. Run `ai-specs init` first.",
            file=sys.stderr,
        )
        return 1

    return render(project, cli_source)


if __name__ == "__main__":
    sys.exit(main())
