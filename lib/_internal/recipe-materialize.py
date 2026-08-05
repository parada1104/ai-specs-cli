#!/usr/bin/env python3
"""Orchestrate recipe materialization during ai-specs sync.

Usage:
  recipe-materialize.py <project_root> <ai_specs_home>

Reads [recipes.*] from ai-specs.toml, validates, detects conflicts,
materializes bundled assets, vendors dep skills, applies templates,
and writes recipe MCP presets to a temp file for downstream mcp-render.py.

Exit 0 on success, 1 on validation/conflict error.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any

import importlib.util


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

_project_cache_module = None

def _load_project_cache():
    global _project_cache_module
    if _project_cache_module is None:
        module_path = Path(__file__).with_name("project-cache.py")
        spec = importlib.util.spec_from_file_location("project_cache_internal", module_path)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"unable to load project-cache.py at {module_path}")
        _project_cache_module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = _project_cache_module
        spec.loader.exec_module(_project_cache_module)
    return _project_cache_module

load_lock = _lock_mod.load_lock
write_lock = _lock_mod.write_lock
set_recipe_skill_hashes = _lock_mod.set_recipe_skill_hashes
set_dep_skill_hashes = _lock_mod.set_dep_skill_hashes
set_managed_override = _lock_mod.set_managed_override
sha256_of = _lock_mod.sha256_of
remove_recipe_lock_entries = _lock_mod.remove_recipe_lock_entries

# Load toml-read helper
_toml_read_module = None

def _load_toml_read() -> Any:
    global _toml_read_module
    if _toml_read_module is None:
        module_path = Path(__file__).with_name("toml-read.py")
        spec = importlib.util.spec_from_file_location("toml_read_internal", module_path)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"unable to load toml-read.py at {module_path}")
        _toml_read_module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(_toml_read_module)
    return _toml_read_module


_util_module = None


def _load_util() -> Any:
    global _util_module
    if _util_module is None:
        module_path = Path(__file__).with_name("util.py")
        spec = importlib.util.spec_from_file_location("util_internal", module_path)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"unable to load util.py at {module_path}")
        _util_module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = _util_module
        spec.loader.exec_module(_util_module)
    return _util_module


def load_recipes_from_manifest(project_root: Path) -> dict[str, dict[str, Any]]:
    mod = _load_toml_read()
    toml_path = project_root / "ai-specs" / "ai-specs.toml"
    data = mod.load_toml(toml_path)
    return mod.read_recipes(data)


def load_bindings_from_manifest(project_root: Path) -> list[dict[str, str]]:
    mod = _load_toml_read()
    toml_path = project_root / "ai-specs" / "ai-specs.toml"
    data = mod.load_toml(toml_path)
    return mod.read_bindings(data)


def fail(msg: str) -> None:
    print(f"  ✗ {msg}", file=sys.stderr)
    sys.exit(1)


def warn(msg: str) -> None:
    print(f"  ! {msg}", file=sys.stderr)


def info(msg: str) -> None:
    print(f"  ℹ {msg}")


# --- Load recipe schema helper ------------------------------------------------
_recipe_schema_module = None

def _load_recipe_schema() -> Any:
    global _recipe_schema_module
    if _recipe_schema_module is None:
        module_path = Path(__file__).with_name("recipe_schema.py")
        spec = importlib.util.spec_from_file_location("recipe_schema_internal", module_path)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"unable to load recipe_schema.py at {module_path}")
        _recipe_schema_module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = _recipe_schema_module
        spec.loader.exec_module(_recipe_schema_module)
    return _recipe_schema_module


def _fragments_to_json(bf: Any) -> dict[str, list[dict]]:
    """Convert a BriefFragments object to a plain JSON-serialisable dict.

    Returns {} when bf is None. Sections with value None are omitted.
    Each fragment becomes {"key": <str|null>, "text": <str>}.
    """
    if bf is None:
        return {}
    result: dict[str, list[dict]] = {}
    for section in ("runtime_flow", "context_sources", "conflict_policy",
                    "workflow_rules", "useful_commands", "mcp_descriptions"):
        frags = getattr(bf, section, None)
        if frags is None:
            continue
        result[section] = [{"key": f.key, "text": f.text} for f in frags]
    return result


def read_recipe(catalog_dir: Path, recipe_id: str) -> Any:
    schema = _load_recipe_schema()
    recipe_dir = catalog_dir / recipe_id
    if not recipe_dir.is_dir():
        raise schema.RecipeValidationError(f"recipe directory not found: {recipe_dir}")
    return schema.load_recipe_toml(recipe_dir / "recipe.toml")


def attach_brief_fragments_to_resolved(
    resolved: dict[str, Any],
    ai_specs_home: Path | None = None,
) -> None:
    """In-place: attach catalog [provides.brief] fragments for enabled recipes."""
    try:
        home = ai_specs_home if ai_specs_home is not None else Path(__file__).resolve().parents[2]
        catalog_dir = home / "catalog" / "recipes"
        recipes = resolved.setdefault("recipes", {})
        for rid in resolved.get("enabled", []):
            try:
                recipe = read_recipe(catalog_dir, rid)
                recipes.setdefault(rid, {})["brief_fragments"] = _fragments_to_json(
                    getattr(recipe, "brief_fragments", None)
                )
            except Exception:
                pass
    except Exception:
        pass


def merge_catalog_defaults_into_resolved(
    resolved: dict[str, Any],
    ai_specs_home: Path | None = None,
) -> None:
    """In-place: merge catalog [config] defaults into resolved recipes.

    For each enabled recipe, read its catalog recipe.toml [config] block and
    merge default values into the resolved config. Manifest overrides already
    present in resolved["recipes"][rid] take precedence over catalog defaults.

    This ensures that recipes enabled without explicit config overrides still
    get their catalog defaults (e.g. base_branch = "development") propagated
    into the resolved-config JSON for downstream rendering.
    """
    try:
        home = ai_specs_home if ai_specs_home is not None else Path(__file__).resolve().parents[2]
        catalog_dir = home / "catalog" / "recipes"
        recipes = resolved.setdefault("recipes", {})
        for rid in resolved.get("enabled", []):
            try:
                recipe = read_recipe(catalog_dir, rid)
                schema_fields = (
                    recipe.config_schema.fields
                    if hasattr(recipe, "config_schema")
                    else {}
                )
                if not schema_fields:
                    continue
                existing = recipes.setdefault(rid, {})
                for key, field in schema_fields.items():
                    if field.default is not None and key not in existing:
                        existing[key] = field.default
            except Exception:
                pass
    except Exception:
        pass


# --- Conflict detection -------------------------------------------------------
_conflict_module = None

def _load_conflict() -> Any:
    global _conflict_module
    if _conflict_module is None:
        module_path = Path(__file__).with_name("recipe-conflicts.py")
        spec = importlib.util.spec_from_file_location("recipe_conflicts_internal", module_path)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"unable to load recipe-conflicts.py at {module_path}")
        _conflict_module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = _conflict_module
        spec.loader.exec_module(_conflict_module)
    return _conflict_module


def check_conflicts(catalog_dir: Path, recipe_ids: list[str]) -> list[Any]:
    mod = _load_conflict()
    return mod.check_recipe_conflicts(catalog_dir, recipe_ids)


def check_capability_conflicts(
    catalog_dir: Path, recipe_ids: list[str], manifest_bindings: list[dict[str, str]]
) -> list[Any]:
    mod = _load_conflict()
    return mod.check_capability_conflicts(catalog_dir, recipe_ids, manifest_bindings)


def check_tag_conflicts(catalog_dir: Path, recipe_ids: list[str]) -> list[Any]:
    """Load enabled recipes and detect tag-based conflicts between them."""
    mod = _load_conflict()
    recipes = []
    for rid in recipe_ids:
        recipe_toml = catalog_dir / rid / "recipe.toml"
        if not recipe_toml.is_file():
            continue
        recipes.append(mod.load_recipe_toml(recipe_toml))
    return mod.check_tag_conflicts(recipes)


# --- Legacy version key handling ---------------------------------------------
def warn_legacy_version(recipe_id: str, manifest_version: str) -> None:
    """Emit a non-blocking WARN when a legacy manifest version= key is present."""
    if not manifest_version:
        return
    warn(
        f"recipe '{recipe_id}' has legacy version='{manifest_version}' in "
        "ai-specs.toml; pins are ignored — sync uses the installed CLI catalog"
    )


# --- Materialize helpers ------------------------------------------------------
def materialize_bundled_skill(recipe_dir: Path, skill_id: str, project_root: Path, recipe_id: str, cli_home: Path | None = None) -> None:
    src = recipe_dir / "skills" / skill_id
    pc = _load_project_cache()
    dest = pc.recipe_skills_root(project_root, cli_home=cli_home) / recipe_id / "skills" / skill_id
    if not src.is_dir():
        raise RuntimeError(f"bundled skill not found: {src}")
    if dest.exists():
        shutil.rmtree(dest)
    dest.parent.mkdir(parents=True, exist_ok=True)
    shutil.copytree(src, dest)
    print(f"    ✓ bundled skill {skill_id}")
    # Track hashes in lock
    lock_path = project_root / "ai-specs" / ".ai-specs.lock"
    lock = load_lock(lock_path)
    hashes = {str(p.relative_to(dest)): sha256_of(p) for p in dest.rglob("*") if p.is_file()}
    set_recipe_skill_hashes(lock, recipe_id, skill_id, hashes)
    write_lock(lock_path, lock)


def materialize_dep_skill(skill: Any, project_root: Path, cli_home: Path | None = None) -> None:
    # Reuse vendor-skills.py logic via import
    vendor_path = Path(__file__).with_name("vendor-skills.py")
    # vendor-skills imports sibling modules (skill_contract); ensure _internal is on path
    # when this file was loaded via importlib from tests (sys.path[0] is not _internal).
    internal_dir = str(vendor_path.parent)
    if internal_dir not in sys.path:
        sys.path.insert(0, internal_dir)
    spec = importlib.util.spec_from_file_location("vendor_skills_internal", vendor_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load vendor-skills.py at {vendor_path}")
    vendor_mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(vendor_mod)

    dep = {
        "id": skill.id,
        "source": skill.url,
    }
    if skill.path:
        dep["path"] = skill.path
    vendor_mod.sync_dep_target(dep, project_root, cli_home=cli_home)
    print(f"    ✓ dep skill {skill.id}")


def materialize_command(
    recipe_dir: Path,
    cmd: Any,
    project_root: Path,
    cli_home: Path | None = None,
) -> None:
    src = recipe_dir / cmd.path
    pc = _load_project_cache()
    dest = pc.commands_dir(project_root, cli_home=cli_home) / f"{cmd.id}.md"
    if not src.is_file():
        raise RuntimeError(f"command source not found: {src}")
    if dest.exists() and (not dest.is_file() or dest.read_bytes() != src.read_bytes()):
        warn(f"recipe command '{cmd.id}' overwrites existing managed command at {dest}")
    dest.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(src, dest)
    print(f"    ✓ command {cmd.id}")


def materialize_template(
    recipe_dir: Path,
    tpl: Any,
    project_root: Path,
    merged_cfg: dict[str, Any] | None = None,
    recipe_id: str | None = None,
) -> None:
    util = _load_util()
    src = recipe_dir / tpl.source
    dest = project_root / tpl.target
    if not src.is_file():
        raise RuntimeError(f"template source not found: {src}")

    content = util.render_override_bytes(src, merged_cfg)
    lock_path = project_root / "ai-specs" / ".ai-specs.lock"
    lock = load_lock(lock_path)
    target = Path(tpl.target).as_posix()
    policy = getattr(tpl, "update_policy", "auto") or "auto"
    if policy not in util.OVERRIDE_POLICIES:
        raise RuntimeError(
            f"invalid update policy '{policy}' for template '{tpl.target}'; "
            "expected auto | confirm | never-force"
        )

    def record(written: bytes = content) -> None:
        set_managed_override(
            lock,
            target,
            util.sha256_bytes(written),
            recipe=recipe_id,
            source=tpl.source,
            kind="template",
            policy=policy,
        )
        write_lock(lock_path, lock)

    def write_content() -> None:
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_bytes(content)
        os.chmod(dest, src.stat().st_mode)

    if tpl.condition == "not_exists" and dest.exists():
        entry = (lock.get("managed") or {}).get(target)
        state = util.classify_managed_override(dest, entry, would_write=content)
        if state == "untracked":
            disk_sha = util.sha256_bytes(dest.read_bytes())
            rendered_sha = util.sha256_bytes(content)
            legacy_catalog_sha = util.sha256_bytes(src.read_bytes())
            if disk_sha in (rendered_sha, legacy_catalog_sha):
                # Existing projects may contain a pre-render placeholder copy;
                # seed its actual bytes and let the next sync reconcile it.
                record(dest.read_bytes())
            else:
                warn(
                    f"declared recipe override has no ownership metadata: {tpl.target}; "
                    "preserving it because the CLI cannot prove it is unmodified; "
                    "the override was not refreshed. This is user-managed override "
                    "content, not a failed sync. To keep this customized override, "
                    "do nothing. To adopt the current recipe version, remove it and "
                    "sync again:\n"
                    f"  rm {tpl.target} && ai-specs sync"
                )
        elif state == "managed_stale" and policy == "auto":
            write_content()
            record()
            info(f"refreshed managed template {tpl.target}")
        elif state in ("user_modified", "managed_stale"):
            label = "user-modified" if state == "user_modified" else f"managed-stale ({policy}-required)"
            warn(
                f"override {label}: {tpl.target} was not refreshed. "
                "Refresh with:\n"
                f"  rm {tpl.target} && ai-specs sync"
            )
        elif state == "managed_current":
            # Backfill provenance fields without rewriting the target.
            record()
        # Idempotent existing-template detail is intentional compact noise.
        # Noise (keep ·): filtered in compact mode via print_step_output.
        print(f"    · template skipped (exists) {tpl.target}")
        return

    write_content()
    record()
    print(f"    ✓ template {tpl.target}")


def materialize_doc(recipe_dir: Path, doc: Any, project_root: Path) -> None:
    src = recipe_dir / doc.source
    dest = project_root / doc.target
    if not src.is_file():
        raise RuntimeError(f"doc source not found: {src}")
    dest.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(src, dest)
    print(f"    ✓ doc {doc.target}")


def hook_script_rel_path(recipe_id: str, hook: Any) -> str:
    """Harness-neutral materialized path for a hook script (project-relative)."""
    return f"ai-specs/recipes/{recipe_id}/hooks/{Path(hook.script).name}"


GATE_MODE_PLACEHOLDERS = {
    "__WORKTREE_GATE_MODE__": "always",
    "__TRACKER_CARD_GATE_MODE__": "warn",
}
# Backward-compatible alias for older call sites / tests.
GATE_MODE_PLACEHOLDER = "__WORKTREE_GATE_MODE__"
REPO_TOPOLOGY_PLACEHOLDER = "__WORKTREE_REPO_TOPOLOGY__"
TRACKER_CLI_HOME_PLACEHOLDER = "__TRACKER_CLI_HOME__"


def materialize_hook_script(
    recipe_dir: Path,
    hook: Any,
    project_root: Path,
    recipe_id: str,
    merged_cfg: dict[str, Any] | None = None,
    cli_home: Path | None = None,
) -> str:
    """Copy a recipe hook script to the harness-neutral path and chmod +x.

    Returns the project-relative materialized path so every harness's wiring
    can reference the single copy.
    """
    src = recipe_dir / hook.script
    if not src.is_file():
        raise RuntimeError(f"hook script not found: {src}")
    rel = hook_script_rel_path(recipe_id, hook)
    dest = project_root / rel
    dest.parent.mkdir(parents=True, exist_ok=True)
    content = src.read_text()
    for token, default in GATE_MODE_PLACEHOLDERS.items():
        if token in content:
            mode = default
            if merged_cfg is not None:
                mode = str(merged_cfg.get("gate_mode", default))
            content = content.replace(token, mode)
    if TRACKER_CLI_HOME_PLACEHOLDER in content:
        home_val = str(Path(cli_home).resolve()) if cli_home is not None else ""
        content = content.replace(TRACKER_CLI_HOME_PLACEHOLDER, home_val)
    dest.write_text(content)
    os.chmod(dest, 0o755)
    print(f"    ✓ hook script {rel}")
    return rel


# --- Binding resolution -------------------------------------------------------
def resolve_bindings(
    catalog_dir: Path, enabled_ids: list[str], manifest_bindings: list[dict[str, str]]
) -> dict[str, str]:
    """Resolve capability-to-recipe bindings.

    Step 1: Validate explicit bindings (recipe enabled, recipe declares capability).
    Step 2: Auto-bind capabilities declared by exactly one enabled recipe.
    Returns map: capability_id -> recipe_id.
    """
    enabled_set = set(enabled_ids)
    binding_map: dict[str, str] = {}

    # Load enabled recipes and their capabilities
    recipe_caps: dict[str, list[str]] = {}
    cap_providers: dict[str, list[str]] = {}
    for rid in enabled_ids:
        try:
            recipe = read_recipe(catalog_dir, rid)
        except Exception:
            continue
        caps = [c.id for c in recipe.capabilities]
        recipe_caps[rid] = caps
        for cap in caps:
            cap_providers.setdefault(cap, []).append(rid)

    # Step 1: explicit bindings
    seen_caps: set[str] = set()
    for binding in manifest_bindings:
        cap = binding.get("capability", "")
        rec = binding.get("recipe", "")
        if cap in seen_caps:
            raise RuntimeError(f"duplicate explicit binding for capability '{cap}'")
        seen_caps.add(cap)
        if rec not in enabled_set:
            raise RuntimeError(f"explicit binding for capability '{cap}' references disabled/unknown recipe '{rec}'")
        if cap not in recipe_caps.get(rec, []):
            raise RuntimeError(f"explicit binding for capability '{cap}' references recipe '{rec}' which does not declare that capability")
        binding_map[cap] = rec

    # Step 2: auto-bind
    for cap, providers in cap_providers.items():
        if cap in binding_map:
            continue
        if len(providers) == 1:
            binding_map[cap] = providers[0]

    return binding_map


# --- Config merge -------------------------------------------------------------
def merge_config(recipe: Any, manifest_config: dict[str, Any]) -> dict[str, Any]:
    """Merge recipe config schema defaults with manifest overrides.

    Fails if any required=True field is missing in the final dict.
    Warns if manifest provides keys not in the schema.
    """
    result: dict[str, Any] = {}
    schema_fields = recipe.config_schema.fields if hasattr(recipe, "config_schema") else {}

    # Start with defaults
    for key, field in schema_fields.items():
        if field.default is not None:
            result[key] = field.default

    # Overlay manifest values
    for key, value in manifest_config.items():
        if key not in schema_fields:
            warn(f"recipe '{recipe.name}': unknown config key '{key}' in manifest (ignored)")
            continue
        result[key] = value

    # Validate required
    for key, field in schema_fields.items():
        if field.required and key not in result:
            raise RuntimeError(f"recipe '{recipe.name}': missing required config field '{key}'")

    # Validate enum constraints
    for key, field in schema_fields.items():
        if key not in result:
            continue
        enum_values = getattr(field, "enum", None)
        if not enum_values:
            continue
        value_str = str(result[key])
        if value_str not in enum_values:
            allowed = " | ".join(enum_values)
            raise RuntimeError(
                f"recipe '{recipe.name}': config field '{key}' value '{value_str}' "
                f"is invalid; allowed: {allowed}"
            )

    return result


# --- Hook execution -----------------------------------------------------------
def execute_hooks(
    recipe: Any,
    merged_config: dict[str, Any],
    project_root: Path,
    cli_home: Path | None = None,
) -> None:
    """Execute recipe hooks in declaration order.

    Unknown actions emit a warning and are skipped.
    Any exception causes sync to fail.
    """
    for hook in recipe.hooks:
        if hook.action == "validate-config":
            # validate-config: ensure all required fields are present
            schema_fields = recipe.config_schema.fields if hasattr(recipe, "config_schema") else {}
            for key, field in schema_fields.items():
                if field.required and key not in merged_config:
                    raise RuntimeError(
                        f"recipe '{recipe.name}': hook 'validate-config' failed: "
                        f"missing required config field '{key}'"
                    )
                # Skip regex validation if the field is not in the merged config
                if key not in merged_config:
                    continue
                value = merged_config[key]
                value_str = str(value)
                # Check for Trello shortLink on board_id fields
                if key == "board_id" and len(value_str) == 8 and value_str.isalnum():
                    raise RuntimeError(
                        f"recipe '{recipe.name}': hook 'validate-config' failed: "
                        f"field 'board_id' value '{value_str}' looks like a Trello shortLink; "
                        f"the real board ID is 24 hex characters (e.g., '69ec0a2099ea20956e371d62')"
                    )
                # Regex validation from field.validation.regex
                validation = getattr(field, "validation", {}) or {}
                pattern = validation.get("regex", "")
                if pattern:
                    if not re.fullmatch(pattern, value_str):
                        raise RuntimeError(
                            f"recipe '{recipe.name}': hook 'validate-config' failed: "
                            f"field '{key}' value '{value_str}' does not match required pattern '{pattern}'"
                        )
        elif hook.action == "bootstrap-board":
            pc = _load_project_cache()
            marker_dir = pc.recipe_skills_root(project_root, cli_home=cli_home) / recipe.id
            marker_dir.mkdir(parents=True, exist_ok=True)
            (marker_dir / "bootstrap-ready").write_text(
                f"board_id={merged_config.get('board_id', '')}\n"
                f"default_list={merged_config.get('default_list', 'In Progress')}\n"
                f"epic_list={merged_config.get('epic_list', 'Epic')}\n"
            )
        elif hook.action == "link-trello-card":
            info(f"recipe '{recipe.name}': hook 'link-trello-card' deferred to agent runtime")
        elif hook.action == "sync-card-state":
            info(f"recipe '{recipe.name}': hook 'sync-card-state' deferred to agent runtime")
        elif hook.action == "comment-verification":
            info(f"recipe '{recipe.name}': hook 'comment-verification' deferred to agent runtime")
        else:
            warn(f"recipe '{recipe.name}': unknown hook action '{hook.action}' (skipped)")


# --- MCP merge ---------------------------------------------------------------
def build_recipe_mcp(catalog_dir: Path, recipe_ids: list[str], manifest_mcp: dict[str, Any]) -> dict[str, Any]:
    """Merge recipe MCP presets with manifest precedence (shallow merge).

    Project manifest keys always win over recipe defaults. Conflicting keys
    emit a warning and are skipped.
    """
    merged: dict[str, Any] = {sid: dict(cfg) for sid, cfg in manifest_mcp.items()}
    for rid in recipe_ids:
        recipe = read_recipe(catalog_dir, rid)
        for mcp in recipe.mcp:
            if mcp.id not in merged:
                merged[mcp.id] = dict(mcp.config)
                continue
            manifest_cfg = merged[mcp.id]
            for key, value in mcp.config.items():
                if key in manifest_cfg:
                    warn(
                        f"recipe '{recipe.name}' mcp.id='{mcp.id}' key '{key}' "
                        f"conflicts with project manifest (manifest wins)"
                    )
                else:
                    manifest_cfg[key] = value
    return merged


def clean_orphans(
    project_root: Path,
    enabled_recipe_ids: set[str],
    expected_dep_ids: set[str],
    cli_home: Path | None = None,
) -> None:
    pc = _load_project_cache()
    recipe_dir = pc.recipe_skills_root(project_root, cli_home=cli_home)
    if recipe_dir.is_dir():
        for child in recipe_dir.iterdir():
            if child.is_dir() and child.name not in enabled_recipe_ids:
                shutil.rmtree(child)
                print(f"  ✓ removed orphaned cache .recipe/{child.name}")

    deps_dir = pc.deps_skills_root(project_root, cli_home=cli_home)
    if deps_dir.is_dir():
        for child in deps_dir.iterdir():
            if child.is_dir() and child.name not in expected_dep_ids:
                shutil.rmtree(child)
                print(f"  ✓ removed orphaned cache .deps/{child.name}")

    # Prune in-project toml-dep materialization (ai-specs/.deps/) for deps no
    # longer declared in the manifest.
    inproject_deps = pc.inproject_deps_root(project_root)
    if inproject_deps.is_dir():
        for child in inproject_deps.iterdir():
            if child.is_dir() and child.name not in expected_dep_ids:
                shutil.rmtree(child)
                print(f"  ✓ removed orphaned ai-specs/.deps/{child.name}")

    # Clean up stale lock entries for recipes no longer in the manifest
    lock_path = project_root / "ai-specs" / ".ai-specs.lock"
    if lock_path.is_file():
        lock = load_lock(lock_path)
        removed_any = False
        for rid in list(lock.get("recipes", {})):
            if rid not in enabled_recipe_ids:
                remove_recipe_lock_entries(lock, rid)
                removed_any = True
                print(f"  ✓ removed stale lock entries for recipe '{rid}'")
        if removed_any:
            write_lock(lock_path, lock)


# --- Main ---------------------------------------------------------------------
def build_resolved_config(project_root: Path) -> dict[str, Any]:
    """Build a resolved-config JSON blob from raw manifest data.

    Reads [recipes.*] sub-tables directly (no catalog lookup), plus [[bindings]].
    Returns: {bindings: {capability→recipe}, recipes: {id→{raw config keys}}, enabled: [id...]}
    """
    mod = _load_toml_read()
    toml_path = project_root / "ai-specs" / "ai-specs.toml"
    manifest_data = mod.load_toml(toml_path)

    # Raw recipes dict: {id: {all keys except enabled/version}}
    raw_recipes = manifest_data.get("recipes", {}) or {}
    recipes_out: dict[str, dict[str, Any]] = {}
    enabled_ids: list[str] = []
    if isinstance(raw_recipes, dict):
        for rid, val in raw_recipes.items():
            if not isinstance(val, dict):
                continue
            # Config = merged catalog-schema defaults + manifest overrides
            # For raw-manifest mode (no catalog), collect all non-meta keys
            config: dict[str, Any] = {}
            for k, v in val.items():
                if k in ("enabled", "version"):
                    continue
                if k == "config" and isinstance(v, dict):
                    # Real manifest style: [recipes.<id>.config]
                    config.update(v)
                else:
                    # Flat style: key=value directly in [recipes.<id>]
                    config[k] = v
            recipes_out[rid] = config
            if val.get("enabled") is True:
                enabled_ids.append(rid)

    # Bindings: explicit [[bindings]] → {capability: recipe}
    raw_bindings = manifest_data.get("bindings", []) or []
    bindings_out: dict[str, str] = {}
    if isinstance(raw_bindings, list):
        for b in raw_bindings:
            if isinstance(b, dict):
                cap = b.get("capability", "")
                rec = b.get("recipe", "")
                if cap and rec:
                    bindings_out[cap] = rec

    return {
        "bindings": bindings_out,
        "recipes": recipes_out,
        "enabled": enabled_ids,
    }


def _enabled_agents(project_root: Path) -> list[str]:
    """Read [agents].enabled from the project manifest (best-effort)."""
    mod = _load_toml_read()
    toml_path = project_root / "ai-specs" / "ai-specs.toml"
    try:
        data = mod.load_toml(toml_path)
    except Exception:
        return []
    agents = data.get("agents", {}) or {}
    enabled = agents.get("enabled", []) or []
    return [str(a) for a in enabled if a]


def materialize_recipes(project_root: Path, ai_specs_home: Path, recipe_mcp_out: Path | None = None, resolved_config_out: Path | None = None, resolved_hooks_out: Path | None = None) -> int:
    catalog_dir = ai_specs_home / "catalog" / "recipes"
    toml_path = project_root / "ai-specs" / "ai-specs.toml"
    cli_home = Path(ai_specs_home)
    pc = _load_project_cache()
    pc.ensure_cache(project_root, cli_home=cli_home)

    recipes = load_recipes_from_manifest(project_root)
    enabled = {rid: cfg for rid, cfg in recipes.items() if cfg.get("enabled")}
    # Legacy-origin cleanup must run after the manifest is loaded so recipe
    # command cleanup can compare against current catalog sources.
    pc.remove_legacy_origin(project_root, cli_home=cli_home)

    util = _load_util()
    allow_internal = os.environ.get("AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES") == "1"
    blocked = sorted(rid for rid in enabled if util.is_internal_test_recipe(rid))
    if blocked and not allow_internal:
        # fail() exits the process (same pattern as other materialize guards).
        fail(util.internal_test_recipe_message(blocked[0]))

    # Collect resolved runtime hooks across enabled recipes (for hooks-render.py).
    resolved_hooks: list[dict[str, Any]] = []

    # Collect expected dep IDs from manifest [[deps]]
    mod = _load_toml_read()
    manifest_data = mod.load_toml(project_root / "ai-specs" / "ai-specs.toml")
    manifest_deps = mod.read_deps(manifest_data)
    expected_dep_ids: set[str] = {d.get("id", "") for d in manifest_deps if d.get("id")}

    if not enabled:
        pc.remove_recipe_command_leftovers(project_root, cli_home=cli_home)
        # Still clean up orphaned recipes (none expected) and deps not in manifest
        clean_orphans(project_root, set(), expected_dep_ids, cli_home=cli_home)
        print("  (no [recipes.*] enabled — skipping)")
        # Still write resolved-config if requested (even with no enabled recipes)
        if resolved_config_out is not None:
            resolved = build_resolved_config(project_root)
            with open(resolved_config_out, "w") as f:
                json.dump(resolved, f, indent=2, sort_keys=True)
                f.write("\n")
            print(f"  ✓ wrote resolved-config (0 enabled recipe(s))")
        if resolved_hooks_out is not None:
            with open(resolved_hooks_out, "w") as f:
                json.dump(
                    {"enabled_agents": _enabled_agents(project_root), "hooks": []},
                    f, indent=2, sort_keys=True,
                )
                f.write("\n")
            print(f"  ✓ wrote resolved-hooks (0 hook(s))")
        return 0

    manifest_bindings = load_bindings_from_manifest(project_root)

    # Binding resolution (NEW)
    resolved_bindings = resolve_bindings(catalog_dir, list(enabled.keys()), manifest_bindings)

    # Capability conflict check (NEW)
    cap_conflicts = check_capability_conflicts(catalog_dir, list(enabled.keys()), manifest_bindings)
    for c in cap_conflicts:
        if getattr(c, "severity", "fatal") == "fatal":
            fail(
                f"capability conflict: {c.primitive_type}.id='{c.primitive_id}' "
                f"claimed by {', '.join(sorted(c.recipes))}. "
                f"Resolve manually in ai-specs.toml."
            )
            return 1
        else:
            warn(
                f"capability ambiguity: {c.primitive_type}.id='{c.primitive_id}' "
                f"declared by {', '.join(sorted(c.recipes))}. "
                f"Add an explicit [[bindings]] entry to resolve."
            )

    # Tag conflict check (NEW): advisory only. Tags are metadata and MUST NOT
    # block materialization — the capability-binding layer owns blocking
    # decisions about competing providers. We surface overlaps as warnings so a
    # developer notices two same-category recipes (e.g. two VCS flows), and flag
    # explicit conflicts_with more loudly, but never change the exit code.
    for c in check_tag_conflicts(catalog_dir, list(enabled.keys())):
        recipes = ", ".join(sorted(c.recipes))
        if getattr(c, "severity", "warning") == "fatal":
            warn(
                f"tag conflict: recipes {recipes} share tag '{c.tag}' and declare "
                f"an explicit conflicts_with. Review whether both should be enabled."
            )
        else:
            warn(
                f"tag overlap: recipes {recipes} share tag '{c.tag}' "
                f"(same capability category)."
            )

    # Primitive conflict detection across recipes
    conflicts = check_conflicts(catalog_dir, list(enabled.keys()))
    if conflicts:
        for c in conflicts:
            fail(
                f"recipe conflict: {c.primitive_type}.id='{c.primitive_id}' "
                f"claimed by {', '.join(sorted(c.recipes))}. "
                f"Resolve manually in ai-specs.toml."
            )
        return 1

    # Load manifest MCP for merge
    mod = _load_toml_read()
    manifest_data = mod.load_toml(toml_path)
    manifest_mcp = mod.read_mcp(manifest_data)

    recipe_mcp: dict[str, Any] = {sid: dict(cfg) for sid, cfg in manifest_mcp.items()}

    # Build source provenance before materializing so first-time upgrades can
    # remove an untouched project copy even when cache/commands is empty.
    recipe_command_sources: dict[str, Path] = {}
    for rid, cfg in enabled.items():
        recipe = read_recipe(catalog_dir, rid)
        recipe_dir = catalog_dir / rid
        for skill in recipe.skills:
            if skill.source == "dep":
                expected_dep_ids.add(skill.id)
        for cmd in recipe.commands:
            recipe_command_sources[f"{cmd.id}.md"] = recipe_dir / cmd.path

    clean_orphans(project_root, set(enabled.keys()), expected_dep_ids, cli_home=cli_home)
    pc.remove_recipe_command_leftovers(
        project_root, cli_home=cli_home, recipe_sources=recipe_command_sources
    )

    for rid, cfg in enabled.items():
        print(f"  ▸ recipe {rid}")
        recipe = read_recipe(catalog_dir, rid)
        warn_legacy_version(rid, cfg.get("version", ""))

        # Config merge (NEW)
        manifest_config = cfg.get("config", {})
        try:
            merged_cfg = merge_config(recipe, manifest_config)
        except RuntimeError as exc:
            fail(str(exc))

        recipe_dir = catalog_dir / rid

        # Skills (bundled then deps)
        for skill in recipe.skills:
            if skill.source == "bundled":
                materialize_bundled_skill(recipe_dir, skill.id, project_root, rid, cli_home=cli_home)
            elif skill.source == "dep":
                materialize_dep_skill(skill, project_root, cli_home=cli_home)
            else:
                raise RuntimeError(f"unknown skill source '{skill.source}' for skill '{skill.id}'")

        # Commands
        for cmd in recipe.commands:
            materialize_command(recipe_dir, cmd, project_root, cli_home=cli_home)

        # MCP presets (shallow merge with manifest precedence)
        for mcp in recipe.mcp:
            if mcp.id not in recipe_mcp:
                recipe_mcp[mcp.id] = dict(mcp.config)
                continue
            manifest_cfg = recipe_mcp[mcp.id]
            for key, value in mcp.config.items():
                if key in manifest_cfg:
                    warn(
                        f"recipe '{recipe.name}' mcp.id='{mcp.id}' key '{key}' "
                        f"conflicts with project manifest (manifest wins)"
                    )
                else:
                    manifest_cfg[key] = value

        # Templates
        for tpl in recipe.templates:
            materialize_template(recipe_dir, tpl, project_root, merged_cfg, recipe_id=rid)

        # Docs
        for doc in recipe.docs:
            materialize_doc(recipe_dir, doc, project_root)

        # Hook execution (sync-time [[hooks]])
        execute_hooks(recipe, merged_cfg, project_root, cli_home=cli_home)

        # Runtime hooks ([[provides.hooks]]): materialize the script once and
        # collect a resolved entry for downstream hooks-render.py. Tunable
        # config values ride along as env (resolved [config.*] overrides).
        for rhook in getattr(recipe, "runtime_hooks", []) or []:
            script_path = materialize_hook_script(recipe_dir, rhook, project_root, rid, merged_cfg, cli_home=cli_home)
            # Pass tunables to the hook as env vars. Only ENV-shaped config keys
            # (UPPER_SNAKE_CASE) are exported, so hook scripts can read them as
            # environment variables; other config keys (e.g. worktrees_dir) are
            # recipe-internal and not exposed to the runtime hook.
            hook_env = {
                k: str(v)
                for k, v in merged_cfg.items()
                if k.isupper() and "-" not in k and k.replace("_", "").isalnum()
            }
            resolved_hooks.append({
                "recipe": rid,
                "id": rhook.id,
                "event": rhook.event,
                "matcher": rhook.matcher,
                "blocking": rhook.blocking,
                "script_path": script_path,
                "env": hook_env,
            })

    # Write merged MCP to a temp file for downstream mcp-render.py
    if recipe_mcp_out is not None:
        temp_path = recipe_mcp_out
    else:
        import tempfile as _tempfile
        fd, temp_path = _tempfile.mkstemp(prefix="ai-specs-recipe-mcp-", suffix=".json")
        os.close(fd)
    with open(temp_path, "w") as f:
        json.dump(recipe_mcp, f, indent=2)
        f.write("\n")
    print(f"  ✓ wrote recipe MCP temp ({len(recipe_mcp)} server(s))")
    if recipe_mcp_out is None:
        print(f"RECIPE_MCP_TEMP:{temp_path}")

    # Write resolved-config JSON for downstream agents-render.py
    # IMPORTANT: use the catalog-aware resolved_bindings (auto-bind included) computed
    # above by resolve_bindings(), rather than re-deriving from explicit [[bindings]] only.
    # build_resolved_config() provides the recipes/enabled structure; we override the
    # bindings key with the full auto-bound map so downstream renderers see auto-bindings.
    if resolved_config_out is not None:
        resolved = build_resolved_config(project_root)
        resolved["bindings"] = resolved_bindings  # replace explicit-only with auto-bound
        merge_catalog_defaults_into_resolved(resolved, ai_specs_home)
        attach_brief_fragments_to_resolved(resolved, ai_specs_home)
        with open(resolved_config_out, "w") as f:
            json.dump(resolved, f, indent=2, sort_keys=True)
            f.write("\n")
        print(f"  ✓ wrote resolved-config ({len(resolved['recipes'])} recipe(s))")

    # Write resolved-hooks JSON for downstream hooks-render.py
    if resolved_hooks_out is not None:
        with open(resolved_hooks_out, "w") as f:
            json.dump(
                {"enabled_agents": _enabled_agents(project_root), "hooks": resolved_hooks},
                f, indent=2, sort_keys=True,
            )
            f.write("\n")
        print(f"  ✓ wrote resolved-hooks ({len(resolved_hooks)} hook(s))")

    return 0


def build_resolved_config_only(project_root: Path, resolved_config_out: Path, ai_specs_home: Path | None = None) -> int:
    """Lightweight mode: build and write ONLY the resolved-config JSON.

    No skill copying, no hooks, no lock writes, no orphan cleanup, no
    recipe-mcp temp file. Used by sync-agent standalone to avoid side effects
    and leaked temp files.

    ai_specs_home: explicit home dir to locate the catalog. When None, falls
    back to resolving relative to __file__ (legacy behaviour, may diverge for
    custom/symlinked installs).
    """
    try:
        resolved = build_resolved_config(project_root)

        # Attempt catalog-aware auto-binding (same as the full materialize path)
        # so standalone sync-agent forwards the same enriched bindings as sync.sh.
        # Distinguish two failure modes:
        #   - RuntimeError from resolve_bindings: a manifest validation error
        #     (duplicate binding, unknown recipe, capability mismatch, etc.).
        #     These are FATAL — surface to stderr and return non-zero, matching
        #     the full materialize_recipes path.
        #   - Any other exception (catalog absent, version mismatch, TOML parse
        #     failure, etc.): degrade silently to explicit-only bindings already
        #     in resolved; do NOT swallow RuntimeError validation errors.
        try:
            mod = _load_toml_read()
            toml_path = project_root / "ai-specs" / "ai-specs.toml"
            manifest_data = mod.load_toml(toml_path)
            raw_recipes = manifest_data.get("recipes", {}) or {}
            enabled_ids = [
                rid for rid, val in raw_recipes.items()
                if isinstance(val, dict) and val.get("enabled") is True
            ]
            if enabled_ids:
                # Use the caller-supplied home so symlinked/custom installs locate
                # the catalog correctly.  Fall back to __file__-relative only when
                # no home was passed (backward-compat for direct script invocations
                # that pre-date the ai_specs_home parameter).
                _home = ai_specs_home if ai_specs_home is not None else Path(__file__).resolve().parents[2]
                catalog_dir = _home / "catalog" / "recipes"
                manifest_bindings = load_bindings_from_manifest(project_root)
                auto_bindings = resolve_bindings(catalog_dir, enabled_ids, manifest_bindings)
                if auto_bindings:
                    resolved["bindings"] = auto_bindings
        except RuntimeError as exc:
            # Manifest validation error from resolve_bindings — fatal, not benign.
            print(f"ERROR: binding validation failed: {exc}", file=sys.stderr)
            return 1
        except Exception:
            pass  # catalog absent or unreadable — degrade to explicit-only bindings

        merge_catalog_defaults_into_resolved(resolved, ai_specs_home)
        attach_brief_fragments_to_resolved(resolved, ai_specs_home)

        with open(resolved_config_out, "w") as f:
            json.dump(resolved, f, indent=2, sort_keys=True)
            f.write("\n")
        return 0
    except Exception as exc:
        print(f"WARNING: resolved-config generation failed: {exc}", file=sys.stderr)
        return 1


def main() -> int:
    args = sys.argv[1:]
    recipe_mcp_out = None
    resolved_config_out = None
    resolved_hooks_out = None
    resolved_config_only = False
    if "--recipe-mcp-out" in args:
        idx = args.index("--recipe-mcp-out")
        if idx + 1 < len(args):
            recipe_mcp_out = Path(args[idx + 1])
            args = args[:idx] + args[idx + 2:]
    if "--resolved-config-out" in args:
        idx = args.index("--resolved-config-out")
        if idx + 1 < len(args):
            resolved_config_out = Path(args[idx + 1])
            args = args[:idx] + args[idx + 2:]
    if "--resolved-hooks-out" in args:
        idx = args.index("--resolved-hooks-out")
        if idx + 1 < len(args):
            resolved_hooks_out = Path(args[idx + 1])
            args = args[:idx] + args[idx + 2:]
    if "--resolved-config-only" in args:
        idx = args.index("--resolved-config-only")
        resolved_config_only = True
        args = args[:idx] + args[idx + 1:]
    if len(args) != 2:
        print(
            f"Usage: {sys.argv[0]} <project_root> <ai_specs_home>"
            " [--recipe-mcp-out <path>] [--resolved-config-out <path>]"
            " [--resolved-hooks-out <path>] [--resolved-config-only]",
            file=sys.stderr,
        )
        return 2

    project_root = Path(args[0]).resolve()
    ai_specs_home = Path(args[1]).resolve()

    if resolved_config_only:
        if resolved_config_out is None:
            print("ERROR: --resolved-config-only requires --resolved-config-out <path>", file=sys.stderr)
            return 2
        return build_resolved_config_only(project_root, resolved_config_out, ai_specs_home)

    try:
        return materialize_recipes(project_root, ai_specs_home, recipe_mcp_out, resolved_config_out, resolved_hooks_out)
    except Exception as exc:
        fail(str(exc))
        return 1


if __name__ == "__main__":
    sys.exit(main())