#!/usr/bin/env python3
"""Differential driver for internal/deps.Gate (T3).

Drives the REAL legacy ``config_wizard._dep_gate`` with a scripted terminal.
Only the OUTSIDE WORLD is faked, at the seams the frozen module itself uses:

  * ``config_wizard._dep_check.check_cli_deps`` — the probe, fed the scripted
    result sets (the probe's own parity is T1's differential);
  * ``questionary`` in ``sys.modules`` — every ``confirm(...).ask()`` records
    its message and answers from the script;
  * ``dep_install``'s ``platform.system``, ``shutil.which``,
    ``subprocess.run`` and ``_load_provider_install`` — the plan matrix, the
    package-manager calls and the provider download;
  * ``sys.stdin.isatty``/``sys.stdout.isatty`` — the payload's ``tty``, because
    the driver runs on pipes and _dep_gate's offer is guarded by that check.

Usage: gate_driver.py <repo-root>   with the payload on stdin.
"""
from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import os
import shutil
import subprocess
import sys
import types
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
    home = Path(payload["home"]).resolve()
    os.environ["AI_SPECS_HOME"] = str(home)

    # The legacy modules import their vendors from lib/_vendor.
    sys.path.insert(0, str(root / "lib" / "_vendor"))
    sys.path.insert(0, str(root / "lib" / "_internal"))

    # rich wraps console output at the terminal width, and off a TTY it reads
    # COLUMNS. The comparison pins CONTENT, not rich's wrapping: a wide COLUMNS
    # keeps every fixture line on one line on both sides. The port's own
    # deviation (one unwrapped line per result) is recorded in the unit doc.
    os.environ["COLUMNS"] = "1000"
    config_wizard = load_module("config_wizard", root / "lib" / "_internal" / "config_wizard.py")
    dep_check = config_wizard._dep_check

    install = payload.get("install") or {}
    which = set(payload.get("which") or [])
    system = payload.get("system") or ""
    answers = list(payload.get("answers") or [])
    prompts: list[str] = []
    commands: list[list[str]] = []

    # ── the probe: scripted result sets, the LAST one repeating ──
    probe_sets = [
        [
            dep_check.DepResult(
                binary=entry["binary"],
                found=entry.get("found", False),
                version=entry.get("version", ""),
                ok=entry.get("ok", False),
                install_url=entry.get("install_url", ""),
                purpose=entry.get("purpose", ""),
                required=entry.get("required", False),
                recipe_id=entry.get("recipe_id", ""),
                detail=entry.get("detail", ""),
                source=entry.get("source", ""),
                resolved_path=entry.get("resolved_path", ""),
            )
            for entry in result_set
        ]
        for result_set in payload.get("probe") or []
    ]
    probed = {"n": 0}

    def scripted_check_cli_deps(recipe, ai_specs_home=None):  # noqa: ANN001, ARG001
        if not probe_sets:
            return []
        index = probed["n"] if probed["n"] < len(probe_sets) else len(probe_sets) - 1
        probed["n"] += 1
        return probe_sets[index]

    dep_check.check_cli_deps = scripted_check_cli_deps

    # ── questionary: record the message, answer from the script ──
    def scripted_confirm(message: str, default: bool = False):  # noqa: ARG001
        prompts.append(message)

        def ask():  # noqa: ANN202
            if not answers:
                return None  # questionary's "no answer" → bool(None) is False
            return answers.pop(0)

        return types.SimpleNamespace(ask=ask)

    sys.modules["questionary"] = types.SimpleNamespace(confirm=scripted_confirm)

    # ── the outside world inside dep_install ──
    def scripted_which(name: str, *args, **kwargs):  # noqa: ARG001
        return "/fake/" + name if name in which else None

    def scripted_run(command, check=False, timeout=None):  # noqa: ANN001, ARG001
        commands.append(list(command))
        if install.get("error"):
            raise OSError(install["error"])
        return types.SimpleNamespace(returncode=install.get("exit", 0))

    def scripted_install_github_release(plan):  # noqa: ANN001, ARG001
        if install.get("provider_error"):
            raise Exception(install["provider_error"])
        return types.SimpleNamespace(
            verified=install.get("verified", False),
            version=install.get("version", ""),
        )

    def patch_dep_install(module):
        real_load_provider_install = module._load_provider_install
        if system:
            module.platform.system = lambda _system=system: _system
        module.shutil.which = scripted_which
        module.subprocess.run = scripted_run
        # The stub keeps the REAL provider_install module: resolve_install_plan
        # calls build_release_plan on it (the rejection paths are the plan
        # half's contract, already differentially pinned), and only the network
        # install is scripted.
        def load_provider_install():
            module_ = real_load_provider_install()
            module_.install_github_release = scripted_install_github_release
            return module_

        module._load_provider_install = load_provider_install

    # _dep_gate loads dep_install through _load_sibling on every call, so the
    # patch has to ride the loader rather than a preloaded instance.
    real_load_sibling = config_wizard._load_sibling

    def patched_load_sibling(name: str):
        module = real_load_sibling(name)
        if name == "dep_install":
            patch_dep_install(module)
        return module

    config_wizard._load_sibling = patched_load_sibling

    # ── run ──
    recipe = types.SimpleNamespace(
        id=payload["recipe"].get("id", ""),
        cli_deps=[types.SimpleNamespace(**dep) for dep in payload["recipe"].get("cli_deps") or []],
    )
    tty = bool(payload.get("tty"))
    stdin_tty, stdout_tty = sys.stdin.isatty, sys.stdout.isatty
    sys.stdin.isatty = lambda: tty
    sys.stdout.isatty = lambda: tty
    # redirect_stdout REPLACES sys.stdout, so the TTY answer has to live on the
    # replacement: _dep_gate asks sys.stdout.isatty(), not the original's.
    class TTYStringIO(io.StringIO):
        def isatty(self):
            return tty

    out, err = TTYStringIO(), io.StringIO()
    try:
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            from rich.console import Console

            ok = config_wizard._dep_gate(recipe, Console(stderr=True))
    finally:
        sys.stdin.isatty, sys.stdout.isatty = stdin_tty, stdout_tty

    sys.stdout.write(
        json.dumps(
            {
                "result": bool(ok),
                "stdout": out.getvalue(),
                "stderr": err.getvalue(),
                "prompts": prompts,
                "commands": commands,
            }
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
