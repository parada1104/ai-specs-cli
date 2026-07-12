#!/usr/bin/env python3
"""CLI version policy: read installed version, parse [tool], compare semver."""

from __future__ import annotations

import re
import sys
import tomllib
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

VERSION_RE = re.compile(
    r"^([0-9]+)\.([0-9]+)\.([0-9]+)(?:-([0-9A-Za-z.-]+))?(?:\+([0-9A-Za-z.-]+))?$"
)


@dataclass(frozen=True)
class ToolPolicy:
    kind: str  # "exact" | "min"
    version: str


def read_installed_version(cli_home: Path) -> str:
    version_path = cli_home / "VERSION"
    if not version_path.is_file():
        return "unknown"
    text = version_path.read_text(encoding="utf-8").strip()
    return text or "unknown"


def _parse_version_tuple(version: str) -> tuple[int, int, int, str | None] | None:
    if version == "unknown":
        return None
    match = VERSION_RE.fullmatch(version.strip())
    if not match:
        return None
    major, minor, patch, prerelease = match.groups()[:4]
    return int(major), int(minor), int(patch), prerelease


def compare_versions(left: str, right: str) -> int:
    """Return -1 if left < right, 0 if equal, 1 if left > right."""
    left_t = _parse_version_tuple(left)
    right_t = _parse_version_tuple(right)
    if left_t is None or right_t is None:
        if left == right:
            return 0
        if left_t is None:
            return -1
        if right_t is None:
            return 1
        return 0

    left_core = left_t[:3]
    right_core = right_t[:3]
    if left_core != right_core:
        return -1 if left_core < right_core else 1

    left_pre = left_t[3]
    right_pre = right_t[3]
    if left_pre == right_pre:
        return 0
    if left_pre is None:
        return 1
    if right_pre is None:
        return -1
    if left_pre == right_pre:
        return 0
    return -1 if left_pre < right_pre else 1


def parse_tool_policy(manifest: dict[str, Any]) -> tuple[ToolPolicy | None, str | None]:
    tool = manifest.get("tool")
    if not tool:
        return None, None
    if not isinstance(tool, dict):
        return None, "invalid [tool] table"

    version = tool.get("version")
    min_version = tool.get("min_version")
    policy = tool.get("policy")

    if version is not None and min_version is not None:
        return None, "cannot set both [tool].version and [tool].min_version"

    if version is not None:
        if not isinstance(version, str) or not version.strip():
            return None, "[tool].version must be a non-empty string"
        effective_policy = policy if policy is not None else "exact"
        if effective_policy != "exact":
            return None, f"unknown [tool].policy: {policy!r}"
        return ToolPolicy(kind="exact", version=version.strip()), None

    if min_version is not None:
        if not isinstance(min_version, str) or not min_version.strip():
            return None, "[tool].min_version must be a non-empty string"
        effective_policy = policy if policy is not None else "min"
        if effective_policy != "min":
            return None, f"unknown [tool].policy: {policy!r}"
        return ToolPolicy(kind="min", version=min_version.strip()), None

    if policy is not None:
        return None, "[tool].policy requires [tool].version or [tool].min_version"

    return None, None


def check_policy(installed: str, policy: ToolPolicy) -> tuple[bool, str]:
    if installed == "unknown":
        return False, "installed CLI version is unknown"

    if policy.kind == "exact":
        if compare_versions(installed, policy.version) == 0:
            return True, ""
        return (
            False,
            f"installed CLI {installed} does not match pinned {policy.version}",
        )

    if compare_versions(installed, policy.version) >= 0:
        return True, ""
    return (
        False,
        f"installed CLI {installed} is below minimum {policy.version}",
    )


def read_lock_meta(lock_path: Path) -> dict[str, str]:
    if not lock_path.is_file():
        return {}
    with lock_path.open("rb") as fh:
        data = tomllib.load(fh)
    meta = data.get("meta") or {}
    if not isinstance(meta, dict):
        return {}
    out: dict[str, str] = {}
    for key in ("cli_version", "synced_at"):
        value = meta.get(key)
        if isinstance(value, str) and value.strip():
            out[key] = value.strip()
    return out


def load_manifest(manifest_path: Path) -> dict[str, Any]:
    with manifest_path.open("rb") as fh:
        return tomllib.load(fh)


def evaluate_cli_version(
    *,
    installed: str,
    manifest: dict[str, Any],
    lock_meta: dict[str, str],
) -> tuple[str, str, str]:
    """Return (severity, check_name, message) for doctor."""
    policy, err = parse_tool_policy(manifest)
    if err:
        return "ERROR", "cli-version", err

    last_synced = lock_meta.get("cli_version", "")

    if policy is not None:
        ok, reason = check_policy(installed, policy)
        if not ok:
            guidance = "run ai-specs upgrade or adjust [tool] in ai-specs.toml"
            return "ERROR", "cli-version", f"{reason} ({guidance})"
        if last_synced and last_synced != installed:
            return (
                "WARN",
                "cli-version",
                f"installed {installed}, pinned {policy.version}, last sync {last_synced}",
            )
        if last_synced == installed:
            return (
                "OK",
                "cli-version",
                f"installed {installed}, pinned {policy.version}, last sync {last_synced}",
            )
        return (
            "OK",
            "cli-version",
            f"installed {installed}, pinned {policy.version}, last sync unknown",
        )

    if not last_synced:
        return (
            "INFO",
            "cli-version",
            f"installed {installed}, no [tool] pin, last sync unknown — run ai-specs sync",
        )
    if last_synced == installed:
        return (
            "OK",
            "cli-version",
            f"installed {installed}, no [tool] pin, last sync {last_synced}",
        )
    return (
        "WARN",
        "cli-version",
        f"installed {installed}, no [tool] pin, last sync {last_synced} — run ai-specs sync",
    )


def stamp_lock_meta(project_root: Path, cli_home: Path) -> None:
    lock_mod = _load_lock_module()
    lock_path = project_root / "ai-specs" / ".ai-specs.lock"
    lock = lock_mod.load_lock(lock_path)
    installed = read_installed_version(cli_home)
    existing_meta = lock.get("meta") or {}
    if (
        existing_meta.get("cli_version") == installed
        and existing_meta.get("synced_at")
    ):
        synced_at = existing_meta["synced_at"]
    else:
        synced_at = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    lock["meta"] = {
        "cli_version": installed,
        "synced_at": synced_at,
    }
    lock_mod.write_lock(lock_path, lock)


def _load_lock_module():
    import importlib.util

    module_path = Path(__file__).with_name("lock.py")
    spec = importlib.util.spec_from_file_location("lock_internal_cli_version", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load lock.py at {module_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _cli_check_sync(argv: list[str]) -> int:
    if len(argv) < 2:
        print("Usage: cli_version.py check-sync <project_root> <cli_home> [--ignore-cli-version]", file=sys.stderr)
        return 2

    project_root = Path(argv[0]).resolve()
    cli_home = Path(argv[1]).resolve()
    ignore = "--ignore-cli-version" in argv[2:]

    manifest_path = project_root / "ai-specs" / "ai-specs.toml"
    if not manifest_path.is_file():
        print(f"ERROR: {manifest_path} not found.", file=sys.stderr)
        return 1

    manifest = load_manifest(manifest_path)
    policy, err = parse_tool_policy(manifest)
    if err:
        print(f"ERROR: {err}", file=sys.stderr)
        return 1

    if policy is None:
        return 0

    installed = read_installed_version(cli_home)
    ok, reason = check_policy(installed, policy)
    if ok:
        return 0

    if ignore:
        print(
            f"⚠ ai-specs: ignoring CLI version policy ({reason})",
            file=sys.stderr,
        )
        return 0

    print(f"ERROR: {reason}.", file=sys.stderr)
    print("       Run `ai-specs upgrade` or adjust [tool] in ai-specs.toml.", file=sys.stderr)
    print("       Pass --ignore-cli-version to sync anyway.", file=sys.stderr)
    return 1


def _cli_stamp_meta(argv: list[str]) -> int:
    if len(argv) != 2:
        print("Usage: cli_version.py stamp-meta <project_root> <cli_home>", file=sys.stderr)
        return 2
    stamp_lock_meta(Path(argv[0]).resolve(), Path(argv[1]).resolve())
    return 0


def main() -> int:
    if len(sys.argv) < 2:
        print("Usage: cli_version.py <check-sync|stamp-meta> ...", file=sys.stderr)
        return 2
    cmd = sys.argv[1]
    args = sys.argv[2:]
    if cmd == "check-sync":
        return _cli_check_sync(args)
    if cmd == "stamp-meta":
        return _cli_stamp_meta(args)
    print(f"ERROR: unknown command: {cmd}", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
