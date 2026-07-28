#!/usr/bin/env python3
"""TTY opt-in install plans for recipe CLI deps. Never silent auto-install."""
from __future__ import annotations

import platform
import shutil
import subprocess
import sys
from dataclasses import dataclass


# binary -> (brew_formula, apt_package). Empty strings mean guidance-only for that side.
_PACKAGE_MAP: dict[str, tuple[str, str]] = {
    "gh": ("gh", "gh"),
    "glab": ("glab", "glab"),
    "jq": ("jq", "jq"),
    "direnv": ("direnv", "direnv"),
    "git": ("git", "git"),
}

# Always guidance-only (no blind Node / bb installs).
_GUIDANCE_ONLY = frozenset({"npx", "bb"})


@dataclass
class InstallPlan:
    binary: str
    command: list[str]
    display: str
    guidance_url: str
    kind: str  # "brew" | "apt" | "guidance"


def resolve_install_plan(binary: str, *, install_url: str = "") -> InstallPlan:
    """Resolve a constrained install plan for *binary*."""
    if binary in _GUIDANCE_ONLY or binary not in _PACKAGE_MAP:
        return InstallPlan(
            binary=binary,
            command=[],
            display=install_url or f"install '{binary}' manually",
            guidance_url=install_url,
            kind="guidance",
        )

    brew_formula, apt_pkg = _PACKAGE_MAP[binary]
    brew = shutil.which("brew")
    apt = shutil.which("apt-get")
    system = platform.system()

    if brew and brew_formula and (system == "Darwin" or system == "Linux"):
        cmd = ["brew", "install", brew_formula]
        return InstallPlan(
            binary=binary,
            command=cmd,
            display=" ".join(cmd),
            guidance_url=install_url,
            kind="brew",
        )

    if apt and apt_pkg and system == "Linux":
        cmd = ["sudo", "apt-get", "install", "-y", apt_pkg]
        return InstallPlan(
            binary=binary,
            command=cmd,
            display=" ".join(cmd),
            guidance_url=install_url,
            kind="apt",
        )

    return InstallPlan(
        binary=binary,
        command=[],
        display=install_url or f"install '{binary}' manually",
        guidance_url=install_url,
        kind="guidance",
    )


def offer_and_install(plans: list[InstallPlan], *, tty: bool) -> list[str]:
    """Prompt per plan on TTY; run confirmed installs. Returns binaries that exist after."""
    if not tty or not plans:
        return []

    installed: list[str] = []
    try:
        import questionary
    except ImportError:
        return []

    for plan in plans:
        if plan.kind == "guidance" or not plan.command:
            print(f"  → {plan.binary}: {plan.display}", file=sys.stderr)
            continue

        msg = f"Install {plan.binary} now? ({plan.display})"
        try:
            answer = questionary.confirm(msg, default=False).ask()
        except Exception:
            continue
        if not answer:
            continue

        try:
            proc = subprocess.run(plan.command, check=False, timeout=300)
        except (OSError, subprocess.TimeoutExpired) as exc:
            print(f"  ! install {plan.binary} failed: {exc}", file=sys.stderr)
            continue
        if proc.returncode != 0:
            print(
                f"  ! install {plan.binary} exited {proc.returncode}",
                file=sys.stderr,
            )
            if plan.guidance_url:
                print(f"    see {plan.guidance_url}", file=sys.stderr)
            continue

        if shutil.which(plan.binary):
            installed.append(plan.binary)
            print(f"  ✓ {plan.binary} installed", file=sys.stderr)
        else:
            print(
                f"  ! {plan.binary} still not on PATH after install",
                file=sys.stderr,
            )
            if plan.guidance_url:
                print(f"    see {plan.guidance_url}", file=sys.stderr)
    return installed
