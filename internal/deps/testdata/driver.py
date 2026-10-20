#!/usr/bin/env python3
"""Differential driver for internal/deps' plan resolution.

Loads the LEGACY modules (lib/_internal/dep_install.py and, through it,
provider_install.py) by absolute path, scripts shutil.which and
platform.system per case exactly as the Go test scripts them, and prints one
canonical JSON projection per case (or the exception's own string).

Usage: driver.py <repo-root>   with {"home": ..., "cases": [...]} on stdin.
"""
from __future__ import annotations

import importlib.util
import json
import platform
import shutil
import sys
from pathlib import Path


def load_module(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


def main() -> int:
    root = Path(sys.argv[1])
    payload = json.load(sys.stdin)
    home = Path(payload["home"])
    dep_install = load_module("dep_install", root / "lib" / "_internal" / "dep_install.py")

    real_which = shutil.which
    real_system = platform.system
    try:
        for case in payload["cases"]:
            # Every case starts from the host's own answers: a case that
            # scripts nothing (the provider plans) must not inherit the
            # previous case's patch.
            shutil.which = real_which
            platform.system = real_system
            names = set(case.get("which") or [])
            system = case.get("system") or ""
            if system:
                shutil.which = lambda name, _names=names: (
                    "/fake/" + name if name in _names else None
                )
                platform.system = lambda _system=system: _system
            try:
                plan = dep_install.resolve_install_plan(
                    case["binary"],
                    install_url=case.get("install_url", ""),
                    installer=case.get("installer", ""),
                    repository=case.get("repository", ""),
                    release_policy=case.get("release_policy", ""),
                    min_version=case.get("min_version", ""),
                    ai_specs_home=home,
                )
                provider_plan = None
                if plan.provider_plan is not None:
                    provider_plan = {
                        "goos": plan.provider_plan.goos,
                        "goarch": plan.provider_plan.goarch,
                        "cache_root": str(plan.provider_plan.cache_root),
                    }
                out = {
                    "ok": True,
                    "binary": plan.binary,
                    "command": list(plan.command),
                    "display": plan.display,
                    "guidance_url": plan.guidance_url,
                    "kind": plan.kind,
                    "installer": plan.installer,
                    "repository": plan.repository,
                    "release_policy": plan.release_policy,
                    "min_version": plan.min_version,
                    "described": dep_install.describe_install_plan(plan),
                    "provider": provider_plan,
                }
            except Exception as exc:  # noqa: BLE001 — the rejection IS the contract
                out = {"ok": False, "error": str(exc)}
            print(json.dumps(out, sort_keys=True))
    finally:
        shutil.which = real_which
        platform.system = real_system
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
