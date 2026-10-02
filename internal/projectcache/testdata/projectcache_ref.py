#!/usr/bin/env python3
"""Differential reference driver for the Go project-cache port (GO-07.S6).

The sandbox is created and pre-seeded by the Go test and its absolute paths are
passed in, so both legs run against the SAME directory tree: the FROZEN cache
key (sha256 of realpath) therefore matches byte-for-byte without any path
normalisation. The driver runs ONE invocation against that tree, then reports
the resulting projection:

    {"stdout": ..., "stderr": ..., "rc": ...,
     "files": {rel: b64}, "modes": {rel: int},
     "dirs": [rel, ...], "links": {rel: target}, "result": <json>}

Two invocation modes:

  cli  -- runs the REAL `lib/_internal/project-cache.py <root> <action> [arg]`
          as a subprocess (the Bash spine's actual entry point).
  lib  -- importlib-loads the same module and calls one library function
          directly (the surface only other Python modules reach).

`created_at` timestamps differ between the two runs by wall clock, so every
text file has its `created_at = "..."` value collapsed to `<TIME>`. `.git/` is
excluded: it is git's own bookkeeping, not CLI behaviour.
"""
from __future__ import annotations

import base64
import contextlib
import importlib.util
import io
import json
import os
import re
import subprocess
import sys
import traceback
from pathlib import Path

sys.dont_write_bytecode = True

REPO_ROOT = Path(__file__).resolve().parents[3]
MODULE_PATH = REPO_ROOT / "lib" / "_internal" / "project-cache.py"

_MODULE = None


def load_module():
    global _MODULE
    if _MODULE is not None:
        return _MODULE
    spec = importlib.util.spec_from_file_location("project_cache_ref", MODULE_PATH)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load project-cache.py at {MODULE_PATH}")
    mod = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = mod
    spec.loader.exec_module(mod)
    _MODULE = mod
    return mod


_CREATED_AT = re.compile(rb'created_at = "[^"]*"')


def _norm_bytes(data: bytes) -> bytes:
    return _CREATED_AT.sub(b'created_at = "<TIME>"', data)


def snapshot(root: Path) -> dict:
    files: dict[str, str] = {}
    modes: dict[str, int] = {}
    dirs: list[str] = []
    links: dict[str, str] = {}
    for p in sorted(root.rglob("*")):
        rel = p.relative_to(root).as_posix()
        if ".git" in rel.split("/"):
            continue
        if p.is_symlink():
            links[rel] = os.readlink(p)
        elif p.is_dir():
            dirs.append(rel)
        else:
            try:
                data = _norm_bytes(p.read_bytes())
            except OSError:
                # Permission-denied cases keep the file on disk but unreadable;
                # both snapshots mark it identically instead of aborting.
                files[rel] = "<unreadable>"
            else:
                files[rel] = base64.b64encode(data).decode("ascii")
            modes[rel] = p.stat().st_mode & 0o7777
    return {"files": files, "modes": modes, "dirs": dirs, "links": links}


def _jsonable(v):
    if isinstance(v, Path):
        return str(v)
    if isinstance(v, (list, tuple)):
        return [_jsonable(x) for x in v]
    if isinstance(v, dict):
        return {str(k): _jsonable(x) for k, x in v.items()}
    return v


def _lock_skills(mod, args, root: Path):
    val = args.get("lock_skills", "disk")
    if val == "disk" or val is None:
        return None
    return val


def _lock_commands(mod, args, root: Path):
    val = args.get("lock_commands", "disk")
    if val == "disk" or val is None:
        return None
    return val


def _recipe_sources(args, root: Path):
    srcs = args.get("recipe_sources")
    if not srcs:
        return None
    return {name: (root / rel) for name, rel in srcs.items()}


def run_lib(spec: dict, root: Path, home: Path):
    mod = load_module()
    fn = spec["fn"]
    args = spec.get("args", {})
    cli_home = spec.get("cli_home")

    if fn == "remove_bundled_skill_leftovers":
        mod.remove_bundled_skill_leftovers(
            root / args["ai_specs"], cli_home,
            lock_skills=_lock_skills(mod, args, root))
        return None
    if fn == "remove_bundled_command_leftovers":
        mod.remove_bundled_command_leftovers(
            root / args["ai_specs"], cli_home,
            lock_commands=_lock_commands(mod, args, root))
        return None
    if fn == "remove_recipe_command_leftovers":
        mod.remove_recipe_command_leftovers(
            root / args.get("project_root", "project"), cli_home=cli_home,
            lock_commands=_lock_commands(mod, args, root),
            recipe_sources=_recipe_sources(args, root))
        return None
    if fn == "bundled_skill_ids":
        return mod.bundled_skill_ids(cli_home)
    if fn == "bundled_command_ids":
        return mod.bundled_command_ids(cli_home)
    if fn == "_is_git_work_tree":
        return mod._is_git_work_tree(root / args["project_root"])
    if fn == "_git_ls_files":
        return mod._git_ls_files(root / args["project_root"], args["pathspec"])
    if fn == "tracked_bundled_skill_leftovers":
        return mod.tracked_bundled_skill_leftovers(root / args["project_root"], cli_home)
    if fn == "tracked_bundled_command_leftovers":
        return mod.tracked_bundled_command_leftovers(root / args["project_root"], cli_home)
    if fn == "format_tracked_bundled_remediation":
        return mod.format_tracked_bundled_remediation(
            args["bundled_ids"], kind=args.get("kind", "skill"),
            path_template=args.get("path_template", "ai-specs/skills/{name}"),
            recursive=args.get("recursive", True))
    if fn == "remove_legacy_origin":
        mod.remove_legacy_origin(root / args["project_root"], cli_home)
        return None
    if fn == "merge_commands":
        return mod.merge_commands(root / args["project_root"], root / args["dest"], cli_home)
    if fn == "gate_backup_path":
        return mod.gate_backup_path(
            root / args["project_root"], args["rel_path"], args["content_sha"], cli_home)
    raise RuntimeError(f"unknown lib function: {fn}")


def run_cli(spec: dict, root: Path):
    argv = [sys.executable, str(MODULE_PATH), str(root)]
    action = spec["cli_action"]
    argv.append(action)
    if action == "path" and spec.get("cli_kind") is not None:
        argv.append(spec["cli_kind"])
    elif action == "merge-commands" and spec.get("cli_dest"):
        argv.append(str(root / spec["cli_dest"]))
    env = dict(os.environ)
    env["PYTHONDONTWRITEBYTECODE"] = "1"
    home = spec.get("ai_specs_home")
    if home:
        env["AI_SPECS_HOME"] = home
    else:
        env.pop("AI_SPECS_HOME", None)
    proc = subprocess.run(argv, capture_output=True, text=True, env=env,
                          cwd=str(REPO_ROOT))
    return proc.stdout, proc.stderr, proc.returncode, None


def main() -> int:
    spec = json.load(sys.stdin)
    root = Path(spec["root"])
    sandbox = Path(spec["sandbox"])
    home = Path(spec["home"]) if spec.get("home") else None
    out = io.StringIO()
    err = io.StringIO()
    result = None
    rc = 0
    try:
        if spec["mode"] == "cli":
            stdout, stderr, rc, result = run_cli(spec, root)
            out.write(stdout)
            err.write(stderr)
        else:
            with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
                result = run_lib(spec, root, home)
    except Exception:
        # Uncaught: model the process traceback surface (rc 1) the same way the
        # hooks/mcp drivers do.
        tb = traceback.format_exc()
        err.write(tb)
        rc = 1

    snap = snapshot(sandbox)
    payload = {
        "stdout": out.getvalue(),
        "stderr": err.getvalue(),
        "rc": rc,
        "result": _jsonable(result),
        # Interpreter identity of the oracle leg, reported by the Go
        # divergence pins (R3-001); never used to branch behavior.
        "python_version": list(sys.version_info[:2]),
        **snap,
    }
    sys.stdout.write(json.dumps(payload))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
