#!/usr/bin/env python3
"""Deterministic, non-interactive assisted recipe configuration helper."""
from __future__ import annotations

import argparse
import importlib.util
import json
import os
import re
import shutil
import subprocess
import sys
import tomllib
from pathlib import Path
from typing import Any


class ConfigureError(Exception):
    """A request rejected before any project write."""


def _load_sibling(name: str):
    path = Path(__file__).with_name(f"{name}.py")
    spec = importlib.util.spec_from_file_location(name.replace("-", "_"), path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load sibling module {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules.setdefault(spec.name, module)
    spec.loader.exec_module(module)
    return module


_recipe_schema = _load_sibling("recipe_schema")
_recipe_read = _load_sibling("recipe-read")
_config_write = _load_sibling("recipe-config-write")
_util = _load_sibling("util")
_cli_version = _load_sibling("cli_version")

SCHEMA_VERSION = 1
REPORT_VERSION = 1
SECRET_KEY_RE = re.compile(r"(?:token|password|secret|api[_-]?key)", re.IGNORECASE)
SUMMARY_RE = re.compile(
    r"Summary:\s*(\d+)\s+OK,\s*(\d+)\s+INFO,\s*(\d+)\s+WARN,\s*(\d+)\s+ERROR",
    re.IGNORECASE,
)


def _cli_home() -> Path:
    return Path(os.environ.get("AI_SPECS_HOME", Path(__file__).resolve().parents[2]))


def _catalog_dir() -> Path:
    return _cli_home() / "catalog" / "recipes"


def _manifest_path(project_root: Path) -> Path:
    return project_root / "ai-specs" / "ai-specs.toml"


def _load_manifest(project_root: Path) -> dict[str, Any]:
    path = _manifest_path(project_root)
    if not path.is_file():
        raise ConfigureError(f"manifest not found: {path}")
    try:
        with path.open("rb") as handle:
            return tomllib.load(handle)
    except tomllib.TOMLDecodeError as exc:
        raise ConfigureError(f"invalid manifest: {exc}") from exc


def _schema_for(recipe_id: str):
    try:
        return _recipe_read.read_recipe(_catalog_dir(), recipe_id)
    except Exception as exc:  # noqa: BLE001
        raise ConfigureError(f"unknown recipe '{recipe_id}': {exc}") from exc


def _recipe_state(manifest: dict[str, Any], recipe_id: str) -> tuple[bool, bool, dict[str, Any]]:
    recipes = manifest.get("recipes") or {}
    entry = recipes.get(recipe_id) if isinstance(recipes, dict) else None
    if not isinstance(entry, dict):
        return False, False, {}
    config = entry.get("config")
    return True, entry.get("enabled") is True, dict(config) if isinstance(config, dict) else {}


def _field_type(field: Any) -> str:
    declared = str(getattr(field, "type", "") or "").lower()
    return {"string": "str", "str": "str", "integer": "int", "boolean": "bool"}.get(declared, declared)


def _schema_document(recipe: Any) -> dict[str, Any]:
    fields = []
    for key, field in recipe.config_schema.fields.items():
        fields.append(
            {
                "key": key,
                "type": _field_type(field),
                "required": bool(field.required),
                "enum": list(field.enum) if field.enum is not None else None,
                "default": field.default,
                "help_text": field.help_text or "",
            }
        )
    return {"fields": fields}


def _topology_grounding(project_root: Path, recipe: Any, current: dict[str, Any]) -> tuple[dict[str, Any] | None, list[str]]:
    field = recipe.config_schema.fields.get("repo_topology")
    if field is None:
        return None, []
    configured = str(current.get("repo_topology", field.default or "auto"))
    try:
        resolution = _util.resolve_repo_topology(project_root, configured)
        topology = {
            "resolved": resolution.resolved,
            "configured": resolution.configured,
            "via": resolution.via,
            "submodules": sorted(resolution.submodules),
            "gitmodules_present": bool(resolution.gitmodules_present),
        }
        assumptions: list[str] = []
        if not topology["gitmodules_present"] and configured == "auto":
            assumptions.append(
                "auto detection cannot distinguish monorepo-apps from standalone; ask the user which topology applies"
            )
        if topology["via"] == "auto" and not topology["gitmodules_present"]:
            assumptions.append("topology detection had no .gitmodules signal and may have degraded to standalone")
        return topology, assumptions
    except Exception as exc:  # noqa: BLE001
        return {
            "resolved": None,
            "configured": configured,
            "via": "error",
            "submodules": [],
            "gitmodules_present": False,
        }, [f"topology detection failed: {type(exc).__name__}"]


def _preflight(project_root: Path, manifest: dict[str, Any]) -> dict[str, Any]:
    installed = _cli_version.read_installed_version(_cli_home())
    policy, policy_error = _cli_version.parse_tool_policy(manifest)
    lock_path = project_root / "ai-specs" / ".ai-specs.lock"
    try:
        lock_meta = _cli_version.read_lock_meta(lock_path)
    except Exception:
        lock_meta = {}
    policy_ok = policy_error is None
    reason = policy_error
    pin = policy.version if policy is not None else None
    pin_kind = policy.kind if policy is not None else None
    if policy is not None and policy_error is None:
        policy_ok, reason = _cli_version.check_policy(installed, policy)
    lock_cli = lock_meta.get("cli_version")
    return {
        "cli_version": {
            "installed": installed,
            "pin": pin,
            "pin_kind": pin_kind,
            "policy_ok": bool(policy_ok),
            "lock_cli_version": lock_cli,
            "lock_state": "stale" if lock_cli and lock_cli != installed else ("current" if lock_cli else "unknown"),
        },
        "blocked_reason": reason if not policy_ok else None,
        "ignore_cli_version": False,
    }


def _relative_paths(paths: list[Path], root: Path) -> list[str]:
    out: list[str] = []
    for path in paths:
        try:
            out.append(path.resolve().relative_to(root.resolve()).as_posix())
        except ValueError:
            continue
    return sorted(out)


def inspect_project(project_root: Path, recipe_id: str) -> dict[str, Any]:
    project_root = Path(project_root).resolve()
    manifest = _load_manifest(project_root)
    recipe = _schema_for(recipe_id)
    present, enabled, current = _recipe_state(manifest, recipe_id)
    topology, assumptions = _topology_grounding(project_root, recipe, current)
    manifest_mcp = manifest.get("mcp") if isinstance(manifest.get("mcp"), dict) else {}
    required_mcp = [preset.id for preset in recipe.mcp]
    cli_deps = [
        {"name": dep.binary, "present": shutil.which(dep.binary) is not None}
        for dep in recipe.cli_deps
    ]
    lock_meta = _preflight(project_root, manifest)["cli_version"]
    env_vars: set[str] = set()
    for preset in recipe.mcp:
        env = preset.config.get("env") if isinstance(preset.config, dict) else None
        if isinstance(env, dict):
            env_vars.update(str(name) for name in env)
    grounding: dict[str, Any] = {
        "mcp": {
            "required": bool(required_mcp),
            "present": sorted(name for name in required_mcp if name in manifest_mcp),
            "env_vars": sorted(env_vars),
        },
        "init": {
            "present": recipe.init is not None,
            "needs_manifest": bool(recipe.init.needs_manifest) if recipe.init else False,
            "needs_mcp": sorted(recipe.init.needs_mcp) if recipe.init else [],
        },
        "cli_deps": sorted(cli_deps, key=lambda item: item["name"]),
    }
    if topology is not None:
        grounding["topology"] = topology
    fields = recipe.config_schema.fields
    unknown = sorted(key for key in current if key not in fields)
    return {
        "schema_version": SCHEMA_VERSION,
        "recipe": {"id": recipe_id, "enabled": enabled, "present_in_manifest": present},
        "schema": _schema_document(recipe),
        "current_config": {key: current[key] for key in sorted(current)},
        "grounding": grounding,
        "preflight": {"cli_version": lock_meta},
        "unknown_keys": unknown,
        "assumptions": assumptions,
    }


def _is_secret_literal(key: str, value: Any) -> bool:
    return bool(SECRET_KEY_RE.search(key)) and isinstance(value, str) and not value.startswith("${env:")


def _validate_values(recipe: Any, values: dict[str, Any]) -> None:
    fields = recipe.config_schema.fields
    for key, value in values.items():
        field = fields.get(key)
        if field is None:
            raise ConfigureError(f"unknown config key: {key}")
        if _is_secret_literal(key, value):
            raise ConfigureError(f"secret-shaped literal rejected for config key: {key}")
        kind = _field_type(field)
        if kind == "str" and not isinstance(value, str):
            raise ConfigureError(f"{key} must be a string")
        if kind == "bool" and not isinstance(value, bool):
            raise ConfigureError(f"{key} must be a boolean")
        if kind == "int" and (not isinstance(value, int) or isinstance(value, bool)):
            raise ConfigureError(f"{key} must be an integer")
        if field.enum is not None and value not in field.enum:
            raise ConfigureError(f"{key} must be one of: {', '.join(field.enum)}")
        regex = field.validation.get("regex") if isinstance(field.validation, dict) else None
        if regex and isinstance(value, str) and re.fullmatch(regex, value) is None:
            raise ConfigureError(f"{key} does not match its validation pattern")


def _run_command(argv: list[str], cwd: Path) -> tuple[int, str]:
    env = dict(os.environ)
    env["AI_SPECS_HOME"] = str(_cli_home())
    proc = subprocess.run(argv, cwd=cwd, env=env, capture_output=True, text=True, check=False)
    return proc.returncode, (proc.stdout or "") + (proc.stderr or "")


def parse_doctor_summary(output: str) -> dict[str, Any]:
    matches = list(SUMMARY_RE.finditer(output or ""))
    if not matches:
        return {"doctor_exit": None, "parsed": False, "ok": None, "info": None, "warn": None, "error": None}
    match = matches[-1]
    return {
        "doctor_exit": None,
        "parsed": True,
        "ok": int(match.group(1)),
        "info": int(match.group(2)),
        "warn": int(match.group(3)),
        "error": int(match.group(4)),
    }


def _failed_step(output: str) -> str | None:
    matches = re.findall(r"syncing\s+([^\n\r]+)", output or "", flags=re.IGNORECASE)
    return matches[-1].strip() if matches else None


def _base_report(recipe_id: str, preflight: dict[str, Any], ignore: bool) -> dict[str, Any]:
    preflight = dict(preflight)
    preflight["ignore_cli_version"] = bool(ignore)
    return {
        "report_version": REPORT_VERSION,
        "status": "rejected",
        "recipe": recipe_id,
        "applied": {"changed": [], "unchanged": [], "preserved": []},
        "preflight": preflight,
        "sync": {"ran": False, "exit_code": None, "failed_step": None, "rolled_back": False, "lock_stamped": False},
        "verify": {"doctor_exit": None, "parsed": False, "ok": None, "info": None, "warn": None, "error": None},
        "assumptions": [],
        "drift": [],
        "gaps": [],
    }


def apply_project(
    project_root: Path,
    recipe_id: str,
    values: dict[str, Any],
    *,
    sync: bool = False,
    ignore_cli_version: bool = False,
    dry_run: bool = False,
) -> tuple[dict[str, Any], int]:
    project_root = Path(project_root).resolve()
    try:
        manifest = _load_manifest(project_root)
        recipe = _schema_for(recipe_id)
        _validate_values(recipe, values)
    except ConfigureError as exc:
        report = _base_report(recipe_id, {"cli_version": {}}, ignore_cli_version)
        report["reason"] = str(exc)
        return report, 3

    preflight = _preflight(project_root, manifest)
    report = _base_report(recipe_id, preflight, ignore_cli_version)
    if not preflight["cli_version"]["policy_ok"] and not ignore_cli_version:
        report["status"] = "blocked"
        report["preflight"]["blocked_reason"] = preflight["blocked_reason"]
        return report, 4
    if preflight["cli_version"]["lock_state"] == "stale":
        installed = preflight["cli_version"]["installed"]
        locked = preflight["cli_version"]["lock_cli_version"]
        report["gaps"].append(f"lock cli_version {locked} != installed {installed} (next sync restamps)")

    _present, _enabled, current = _recipe_state(manifest, recipe_id)
    for key in sorted(values):
        if key in current and current[key] == values[key]:
            report["applied"]["unchanged"].append(key)
        else:
            report["applied"]["changed"].append({"key": key, "from": current.get(key), "to": values[key]})
    report["applied"]["preserved"] = sorted(key for key in current if key not in values)
    changed = bool(report["applied"]["changed"])
    report["assumptions"] = inspect_project(project_root, recipe_id).get("assumptions", [])

    if not changed or dry_run:
        report["status"] = "no-op" if not changed else "ok"
        report["dry_run"] = bool(dry_run)
        return report, 0

    try:
        _config_write.update_recipe_config(_manifest_path(project_root), recipe_id, values)
    except Exception as exc:  # noqa: BLE001
        report["status"] = "failed"
        report["reason"] = str(exc)
        return report, 1

    if sync:
        command = [str(_cli_home() / "bin" / "ai-specs"), "sync", str(project_root)]
        if ignore_cli_version:
            command.append("--ignore-cli-version")
        sync_exit, sync_output = _run_command(command, project_root)
        report["sync"].update({"ran": True, "exit_code": sync_exit})
        if sync_exit != 0:
            report["status"] = "partial"
            report["sync"].update({"failed_step": _failed_step(sync_output), "rolled_back": False, "lock_stamped": False})
            report["reason"] = "sync failed; previous writes were not rolled back and lock CLI version was not stamped"
            return report, 1
        report["sync"]["lock_stamped"] = True
        doctor_cmd = [str(_cli_home() / "bin" / "ai-specs"), "doctor", str(project_root)]
        doctor_exit, doctor_output = _run_command(doctor_cmd, project_root)
        verify = parse_doctor_summary(doctor_output)
        verify["doctor_exit"] = doctor_exit
        report["verify"] = verify
        if doctor_exit != 0:
            report["status"] = "failed"
            report["reason"] = "doctor verification failed"
            return report, 1
    report["status"] = "ok"
    return report, 0


def _parse_assignment(recipe: Any, assignment: str) -> tuple[str, Any]:
    if "=" not in assignment:
        raise ConfigureError("--set requires KEY=VALUE")
    key, raw = assignment.split("=", 1)
    key = key.strip()
    raw = raw.strip()
    if not key:
        raise ConfigureError("--set requires a non-empty key")
    field = recipe.config_schema.fields.get(key)
    if field is None:
        raise ConfigureError(f"unknown config key: {key}")
    try:
        value = tomllib.loads(f"value = {raw}\n")["value"]
    except tomllib.TOMLDecodeError:
        if _field_type(field) == "str":
            value = raw
        else:
            raise ConfigureError(f"invalid TOML value for {key}")
    return key, value


def _emit(document: dict[str, Any], as_json: bool) -> None:
    if as_json:
        print(json.dumps(document, ensure_ascii=False, separators=(",", ":"), sort_keys=False))
    else:
        print(json.dumps(document, ensure_ascii=False, indent=2, sort_keys=False))


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="ai-specs recipe configure")
    parser.add_argument("recipe_id")
    parser.add_argument("path", nargs="?", default=".")
    parser.add_argument("--inspect", action="store_true")
    parser.add_argument("--set", dest="assignments", action="append", default=[])
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--sync", action="store_true")
    parser.add_argument("--ignore-cli-version", action="store_true")
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args(argv)
    root = Path(args.path).resolve()
    if args.inspect and args.assignments:
        parser.error("--inspect cannot be combined with --set")
    if not args.inspect and not args.assignments:
        parser.error("one of --inspect or --set is required")
    try:
        recipe = _schema_for(args.recipe_id)
        if args.inspect:
            _emit(inspect_project(root, args.recipe_id), args.json)
            return 0
        values = dict(_parse_assignment(recipe, item) for item in args.assignments)
        report, code = apply_project(
            root,
            args.recipe_id,
            values,
            sync=args.sync,
            ignore_cli_version=args.ignore_cli_version,
            dry_run=args.dry_run,
        )
    except ConfigureError as exc:
        report = _base_report(args.recipe_id, {"cli_version": {}}, args.ignore_cli_version)
        report["reason"] = str(exc)
        code = 3
    _emit(report, args.json)
    return code


if __name__ == "__main__":
    raise SystemExit(main())
