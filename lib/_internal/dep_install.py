#!/usr/bin/env python3
"""TTY opt-in install plans for recipe CLI deps. Never silent auto-install."""
from __future__ import annotations

import importlib.util
import platform
import shutil
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path


# binary -> (brew_formula, apt_package). Empty strings mean guidance-only for that side.
_PACKAGE_MAP: dict[str, tuple[str, str]] = {
    "gh": ("gh", "gh"),
    "glab": ("glab", "glab"),
    "jq": ("jq", "jq"),
    "direnv": ("direnv", "direnv"),
    "git": ("git", "git"),
    "bb": ("bb-cli", ""),
}

# Always guidance-only (no blind Node install).
_GUIDANCE_ONLY = frozenset({"npx"})


@dataclass
class InstallPlan:
    binary: str
    command: list[str]
    display: str
    guidance_url: str
    kind: str  # "brew" | "apt" | "guidance" | "github-release"
    installer: str = ""
    repository: str = ""
    release_policy: str = ""
    min_version: str = ""
    provider_plan: object | None = None


def _load_provider_install():
    path = Path(__file__).with_name("provider_install.py")
    spec = importlib.util.spec_from_file_location("provider_install", path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load provider installer at {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def resolve_install_plan(
    binary: str,
    *,
    install_url: str = "",
    installer: str = "",
    repository: str = "",
    release_policy: str = "",
    min_version: str = "",
    ai_specs_home: Path | None = None,
) -> InstallPlan:
    """Resolve a constrained install plan for *binary*."""
    if installer == "github-release":
        provider_install = _load_provider_install()
        dep = type(
            "ProviderDependency",
            (),
            {
                "binary": binary,
                "installer": installer,
                "repository": repository,
                "release_policy": release_policy,
                "min_version": min_version,
                "install_url": install_url,
            },
        )()
        provider_plan = provider_install.build_release_plan(dep, ai_specs_home=ai_specs_home)
        return InstallPlan(
            binary=binary,
            command=[],
            display=(
                f"GitHub Release {repository} ({release_policy}) "
                f"for {provider_plan.goos}/{provider_plan.goarch}"
            ),
            guidance_url=install_url,
            kind="github-release",
            installer=installer,
            repository=repository,
            release_policy=release_policy,
            min_version=min_version,
            provider_plan=provider_plan,
        )

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


def describe_install_plan(plan: InstallPlan) -> str:
    """Human-readable consent preview for one install plan. No side effects.

    Requirement 3: an interactive GitHub Release offer must state the
    allowlisted repository, release policy, host target, expected archive,
    destination, checksum source, and replacement behavior before asking.
    """
    provider_plan = plan.provider_plan
    if plan.kind != "github-release" or provider_plan is None:
        return plan.display

    goos = str(getattr(provider_plan, "goos", ""))
    goarch = str(getattr(provider_plan, "goarch", ""))
    suffix = ".zip" if goos == "windows" else ".tar.gz"
    lines = [
        f"  GitHub Release installation plan for '{plan.binary}':",
        f"    repository:      {plan.repository}",
        f"    release policy:  {plan.release_policy} "
        "(latest non-draft, non-prerelease tag)",
        f"    target:          {goos}/{goarch}",
        f"    expected asset:  jinna_<version>_{goos}_{goarch}{suffix}",
        f"    destination:     {getattr(provider_plan, 'cache_root', '')}",
        "    checksum source: SHA256SUMS from the same release",
        f"    minimum version: {plan.min_version or 'not declared'}",
        "    replacement:     an existing PATH provider is never modified; "
        "a verified managed version is reused",
    ]
    if plan.guidance_url:
        lines.append(f"    manual fallback: {plan.guidance_url}")
    return "\n".join(lines)


def offer_and_install(plans: list[InstallPlan], *, tty: bool) -> list[str]:
    """Prompt per plan on TTY; run confirmed installs. Returns installed binaries."""
    if not tty or not plans:
        return []

    installed: list[str] = []
    try:
        import questionary
    except ImportError:
        return []

    for plan in plans:
        if plan.kind == "guidance":
            print(f"  → {plan.binary}: {plan.display}", file=sys.stderr)
            continue

        if plan.kind != "github-release" and not plan.command:
            # Defensive: a malformed empty-command plan must never reach a
            # subprocess call with an empty argv.
            print(f"  → {plan.binary}: {plan.display}", file=sys.stderr)
            continue

        if plan.kind == "github-release":
            print(describe_install_plan(plan), file=sys.stderr)

        msg = f"Install {plan.binary} now? ({plan.display})"
        try:
            answer = questionary.confirm(msg, default=False).ask()
        except Exception:
            continue
        if not answer:
            continue

        if plan.kind == "github-release":
            try:
                result = _load_provider_install().install_github_release(plan.provider_plan)
            except Exception as exc:  # noqa: BLE001
                print(f"  ! install {plan.binary} failed: {exc}", file=sys.stderr)
                if plan.guidance_url:
                    print(f"    see {plan.guidance_url}", file=sys.stderr)
                continue
            if result.verified:
                installed.append(plan.binary)
                print(f"  ✓ {plan.binary} installed ({result.version})", file=sys.stderr)
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
