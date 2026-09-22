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
from datetime import datetime, timezone
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
_provider_install_module = None


def _load_provider_install():
    global _provider_install_module
    if _provider_install_module is None:
        module_path = Path(__file__).with_name("provider_install.py")
        spec = importlib.util.spec_from_file_location("provider_install", module_path)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"unable to load provider_install.py at {module_path}")
        module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = module
        spec.loader.exec_module(module)
        _provider_install_module = module
    return _provider_install_module


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
set_gate_baseline = _lock_mod.set_gate_baseline
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


_cli_version_module = None


def _load_cli_version() -> Any:
    """Load the sibling cli_version.py (lazy, cached)."""
    global _cli_version_module
    if _cli_version_module is None:
        module_path = Path(__file__).with_name("cli_version.py")
        spec = importlib.util.spec_from_file_location("cli_version_internal", module_path)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"unable to load cli_version.py at {module_path}")
        _cli_version_module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = _cli_version_module
        spec.loader.exec_module(_cli_version_module)
    return _cli_version_module


_gate_binary_module = None


def _load_gate_binary() -> Any:
    """Load the sibling gate_binary.py acquisition module (lazy, cached)."""
    global _gate_binary_module
    if _gate_binary_module is None:
        module_path = Path(__file__).with_name("gate_binary.py")
        spec = importlib.util.spec_from_file_location("gate_binary_internal", module_path)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"unable to load gate_binary.py at {module_path}")
        _gate_binary_module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = _gate_binary_module
        spec.loader.exec_module(_gate_binary_module)
    return _gate_binary_module


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


# --- Recipe-declared reconcile stamping ---------------------------------------
def _load_recipe_config_write() -> Any:
    global _recipe_config_write_module
    if _recipe_config_write_module is None:
        module_path = Path(__file__).with_name("recipe-config-write.py")
        spec = importlib.util.spec_from_file_location("recipe_config_write_internal", module_path)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"unable to load recipe-config-write.py at {module_path}")
        _recipe_config_write_module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = _recipe_config_write_module
        spec.loader.exec_module(_recipe_config_write_module)
    return _recipe_config_write_module


_recipe_config_write_module: Any = None


def stamp_recipe_reconcile_defaults(project_root: Path, catalog_dir: Path, enabled_ids: list[str]) -> None:
    """Stamp recipe-declared reconcile mapping and the config values it selects
    into the project manifest, absent keys only.

    The gate reads only the manifest, so the recipe-owned lifecycle mapping
    (delivery/review/merge) must reach ``[recipes.<id>.config]`` for
    reconciliation to work out of the box. Explicit project values always win;
    this helper never overwrites an existing key. Optional fields stamp only
    when the declared mapping references them, so unrelated defaults keep the
    old resolved-render behavior.
    """
    schema = _load_recipe_schema()
    manifest = project_root / "ai-specs" / "ai-specs.toml"
    writer = _load_recipe_config_write()
    for rid in enabled_ids:
        try:
            recipe = read_recipe(catalog_dir, rid)
        except Exception:
            continue  # validation failures surface through the normal sync path
        tables = recipe.config_schema.tables
        declared = tables.get("reconcile")
        if declared is None:
            continue
        values = declared.values or {}
        stamp: dict[str, Any] = {"reconcile": values}
        used: set[str] = {values.get("scope_field") or ""}
        for expectation in values.get("expectations", []) or []:
            if isinstance(expectation, dict):
                used.add(expectation.get("config_field") or "")
                used.add(expectation.get("config_field_when_set") or "")
        used.discard("")
        for field_name in used:
            field = recipe.config_schema.fields.get(field_name)
            if field is not None and field.default is not None:
                stamp[field_name] = field.default
        try:
            writer.update_recipe_config(manifest, rid, stamp)
        except Exception as exc:
            warn(f"recipe '{recipe.name}': reconcile defaults not stamped ({type(exc).__name__}: {exc})")


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
    """Grade capability conflicts through the Go authority (read-only).

    Fatal (duplicate explicit binding) and warning (ambiguous provider) grades
    come from the same ``--resolve-bindings`` envelope as the bindings, in the
    conflict shape the call sites already read. A resolution error reported in
    the same envelope is ignored here: the conflict grader is independent of the
    binding grader in both authorities, so a duplicate binding is graded fatal
    even though resolution aborts. Never writes the witness and never acquires a
    binary: the read-only bridge call leaves acquisition opt-in off.
    """
    envelope = go_binding_resolution(catalog_dir, recipe_ids, manifest_bindings)
    if envelope is not None:
        return _conflicts_from_envelope(envelope)
    return _python_check_capability_conflicts(catalog_dir, recipe_ids, manifest_bindings)


def _python_check_capability_conflicts(
    catalog_dir: Path, recipe_ids: list[str], manifest_bindings: list[dict[str, str]]
) -> list[Any]:
    """Grade capability conflicts (TEMPORARY Python authority).

    Kept only as the fail-open fallback announced by
    ``GO_BINDINGS_BRIDGE_FALLBACK``; the Go grader is the primary authority.
    """
    mod = _load_conflict()
    return mod.check_capability_conflicts(catalog_dir, recipe_ids, manifest_bindings)


# --- Tag conflict grading (Go authority, temporary Python fallback) -----------
#
# The Go gate binary (``--resolve-tag-conflicts``) owns the tag-conflict
# DECISION: grouping enabled recipes by first-seen tag, deduplicating recipe
# ids, and grading each overlap warning/fatal. ``recipe-materialize.py`` keeps
# the warning text and the advisory exit behavior at the call site. The Python
# decision survives as a TEMPORARY fail-open fallback
# (``GO_TAG_CONFLICTS_BRIDGE_FALLBACK``) announced by one warning line whenever
# the bridge cannot run.
GO_TAG_CONFLICTS_BRIDGE_FALLBACK = "GO_TAG_CONFLICTS_BRIDGE_FALLBACK"
GO_TAG_CONFLICTS_BRIDGE_TIMEOUT_SECONDS = 60


def _warn_tag_conflicts_bridge_fallback(reason: str) -> None:
    """One greppable warning line for a degraded tag-conflict authority."""
    warn(
        f"{GO_TAG_CONFLICTS_BRIDGE_FALLBACK}: {reason}; "
        "using the temporary Python tag-conflict authority"
    )


def _tag_conflicts_from_envelope(envelope: dict[str, Any]) -> list[Any]:
    """Adapt the Go envelope's conflicts onto the existing ``TagConflict`` type.

    Call sites read ``tag`` / ``recipes`` / ``severity``, so reusing the
    dataclass keeps the warning and fatal messages byte-identical to the ones
    the Python grader produced. Go already sorts recipe ids and preserves
    first-seen tag order, so the mapped list needs no extra ordering.
    """
    conflict_cls = _load_conflict().TagConflict
    return [
        conflict_cls(
            tag=str(item.get("tag", "")),
            recipes=set(item.get("recipes") or []),
            severity=str(item.get("severity") or "warning"),
        )
        for item in envelope.get("conflicts") or []
    ]


def go_tag_conflicts(catalog_dir: Path, recipe_ids: list[str]) -> list[Any] | None:
    """Run ``worktree-gate --resolve-tag-conflicts`` and return its conflicts, or None.

    None means the bridge could not run: no verified binary, the process failed,
    the output was not JSON, or it was not the documented envelope. The caller
    then falls back to the temporary Python authority. This function emits the
    single ``GO_TAG_CONFLICTS_BRIDGE_FALLBACK`` warning naming the reason, so a
    degraded run is never silent and never needs a second warning.
    """
    try:
        gb = _load_gate_binary()
        binary = gb.resolve_verified_binary(_bridge_home(catalog_dir))
    except Exception as exc:  # noqa: BLE001 - an unloadable helper is "no binary"
        _warn_tag_conflicts_bridge_fallback(
            f"the gate binary could not be resolved ({type(exc).__name__}: {exc})"
        )
        return None
    if binary is None:
        _warn_tag_conflicts_bridge_fallback("no verified worktree-gate binary")
        return None

    command = [str(binary), "--resolve-tag-conflicts", "--catalog-dir", str(catalog_dir)]
    for rid in recipe_ids:
        command += ["--recipe", str(rid)]
    try:
        proc = subprocess.run(
            command,
            capture_output=True,
            text=True,
            timeout=GO_TAG_CONFLICTS_BRIDGE_TIMEOUT_SECONDS,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        _warn_tag_conflicts_bridge_fallback(
            f"worktree-gate did not run ({type(exc).__name__}: {exc})"
        )
        return None
    if proc.returncode != 0:
        detail = (proc.stderr or "").strip() or "no stderr"
        _warn_tag_conflicts_bridge_fallback(
            f"worktree-gate exited {proc.returncode} ({detail})"
        )
        return None
    try:
        envelope = json.loads(proc.stdout)
    except ValueError as exc:
        _warn_tag_conflicts_bridge_fallback(
            f"worktree-gate output was not JSON ({exc})"
        )
        return None
    if (
        not isinstance(envelope, dict)
        or not isinstance(envelope.get("conflicts"), list)
        or not all(isinstance(item, dict) for item in envelope["conflicts"])
    ):
        _warn_tag_conflicts_bridge_fallback(
            "worktree-gate output did not match the tag-conflict envelope"
        )
        return None
    return _tag_conflicts_from_envelope(envelope)


def check_tag_conflicts(catalog_dir: Path, recipe_ids: list[str]) -> list[Any]:
    """Grade advisory tag conflicts through the Go authority.

    ``worktree-gate --resolve-tag-conflicts`` is the primary authority; the
    retained Python grader runs only when the bridge cannot. Tag conflicts are
    advisory, so neither path changes the materialization exit code.
    """
    conflicts = go_tag_conflicts(catalog_dir, recipe_ids)
    if conflicts is not None:
        return conflicts
    return _python_check_tag_conflicts(catalog_dir, recipe_ids)


def _python_check_tag_conflicts(catalog_dir: Path, recipe_ids: list[str]) -> list[Any]:
    """Detect tag-based conflicts (TEMPORARY Python authority).

    Kept only as the fail-open fallback announced by
    ``GO_TAG_CONFLICTS_BRIDGE_FALLBACK``; ``worktree-gate
    --resolve-tag-conflicts`` is the primary authority.
    """
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


def resolve_template_dest(project_root: Path, target: str) -> Path:
    """Resolve a governed template target to its real path.

    A ``.git/...`` target (a Git hook) is resolved through Git's own
    ``rev-parse --git-path``, because in a linked worktree ``.git`` is a gitfile,
    not a directory: the naive ``project_root/.git/hooks`` join raises
    ``NotADirectoryError``. Git resolves hooks to the shared hooks directory, so
    the hook lands where Git will actually run it. Anything else stays
    project-relative, and a missing/unavailable Git falls back to the literal
    project-relative path (fixtures outside a repository still materialize).
    """
    literal = project_root / target
    if not target.startswith(".git/"):
        return literal
    remainder = target[len(".git/") :]
    try:
        proc = subprocess.run(
            ["git", "-C", str(project_root), "rev-parse", "--git-path", remainder],
            capture_output=True,
            text=True,
            check=False,
        )
    except OSError:
        return literal
    if proc.returncode != 0:
        return literal
    resolved = proc.stdout.strip()
    if not resolved:
        return literal
    path = Path(resolved)
    if not path.is_absolute():
        # Git emits a repo-relative path for the main worktree; we invoked it
        # with `-C project_root`, so anchor there.
        path = project_root / path
    return path


def materialize_template(
    recipe_dir: Path,
    tpl: Any,
    project_root: Path,
    merged_cfg: dict[str, Any] | None = None,
    recipe_id: str | None = None,
) -> None:
    util = _load_util()
    src = recipe_dir / tpl.source
    dest = resolve_template_dest(project_root, tpl.target)
    if not src.is_file():
        raise RuntimeError(f"template source not found: {src}")

    content = render_template_bytes(src, merged_cfg)
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
                    f"override metadata missing for {tpl.target}; preserving existing file without assigning ownership. "
                    "To preserve this local file, leave it unchanged. To replace it with the current recipe version, "
                    "remove it and run sync again:\n"
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
GATE_SCOPE_PLACEHOLDER = "__WORKTREE_GATE_SCOPE__"
GATE_SCOPE_VALUES = ("auto", "superrepo", "subrepo")
REPO_TOPOLOGY_VALUES = ("auto", "standalone", "monorepo-apps", "monorepo-submodules")
# Backward-compatible alias for older call sites / tests.
GATE_MODE_PLACEHOLDER = "__WORKTREE_GATE_MODE__"
REPO_TOPOLOGY_PLACEHOLDER = "__WORKTREE_REPO_TOPOLOGY__"
TRACKER_CLI_HOME_PLACEHOLDER = "__TRACKER_CLI_HOME__"
# The tracker gate's evidence-bridge directory: $AI_SPECS_HOME/lib/_internal, where
# ledger_bridge.py ships. Empty when no CLI home resolves, which makes the host
# skip evidence acquisition and the tracker.none write (fail open).
TRACKER_LIB_INTERNAL_PLACEHOLDER = "__TRACKER_LIB_INTERNAL__"
GATE_IMPL_PLACEHOLDER = "__WORKTREE_GATE_IMPL__"
GATE_IMPL_VALUES = ("auto", "go")
GATE_VERSION_PLACEHOLDER = "__WORKTREE_GATE_VERSION__"
# Narrow per-recipe template stamps for worktree-flow's managed post-merge hook.
# The hook must carry the project's configured cleanup inputs, or cleanup
# silently falls back to `.worktrees` / the current HEAD for customized projects.
WORKTREES_DIR_PLACEHOLDER = "__WORKTREE_WORKTREES_DIR__"
INTEGRATION_BRANCH_PLACEHOLDER = "__WORKTREE_INTEGRATION_BRANCH__"


def render_template_bytes(src: Path, merged_cfg: dict[str, Any] | None) -> bytes:
    """Render one governed template with the shared topology token plus the
    narrow cleanup-config stamps the managed post-merge hook needs.

    ``util.render_override_bytes`` owns the shared ``__WORKTREE_REPO_TOPOLOGY__``
    token; this adds only ``worktrees_dir`` and ``integration_branch`` so the
    rendered bytes are the exact content sync writes (and the lock compares).
    """
    util = _load_util()
    data = util.render_override_bytes(src, merged_cfg)
    if merged_cfg is None:
        return data
    for token, key, default in (
        (WORKTREES_DIR_PLACEHOLDER, "worktrees_dir", ".worktrees"),
        (INTEGRATION_BRANCH_PLACEHOLDER, "integration_branch", "main"),
    ):
        token_bytes = token.encode()
        if token_bytes in data:
            value = str(merged_cfg.get(key) or default)
            data = data.replace(token_bytes, value.encode())
    return data


def _invalid_gate_impl_error(impl: str) -> RuntimeError:
    return RuntimeError(
        f"invalid gate_impl '{impl}'; bash has been removed; allowed: auto | go"
    )


def _write_gate_backup(
    project_root: Path,
    rel: str,
    prior_bytes: bytes,
    cli_home: Path | None,
) -> Path:
    """Persist one immutable pre-refresh snapshot in the CLI cache.

    The backup path is keyed by the project-relative target and the exact
    content hash, so repeated refreshes never overwrite an earlier snapshot.
    """
    util = _load_util()
    pc = _load_project_cache()
    path = pc.gate_backup_path(
        project_root, rel, util.sha256_bytes(prior_bytes), cli_home=cli_home
    )
    path.parent.mkdir(parents=True, exist_ok=True)
    if not path.exists():
        path.write_bytes(prior_bytes)
    return path


def _refresh_gate(
    project_root: Path,
    dest: Path,
    rel: str,
    content: str,
    lock: dict,
    lock_path: Path,
    recipe_id: str,
    hook: Any,
    cli_home: Path | None,
) -> None:
    """Explicit gate refresh: cache backup → gate write → lock (all-or-nothing).

    On any failure the new backup is deleted and the gate is restored to its
    prior bytes; the lock is never partially updated (atomic write_lock).
    """
    util = _load_util()
    prior = dest.read_bytes() if dest.exists() else None
    created_backup: Path | None = None
    try:
        if prior is not None:
            created_backup = _write_gate_backup(project_root, rel, prior, cli_home)
        dest.write_text(content)
        os.chmod(dest, 0o755)
        set_gate_baseline(
            lock, rel, util.sha256_bytes(content.encode()),
            recipe=recipe_id, source=hook.script,
        )
        write_lock(lock_path, lock)
    except BaseException:
        if prior is not None:
            try:
                dest.write_bytes(prior)
                os.chmod(dest, 0o755)
            except OSError:
                pass
        if created_backup is not None and created_backup.exists():
            try:
                created_backup.unlink()
            except OSError:
                pass
        raise


def materialize_hook_script(
    recipe_dir: Path,
    hook: Any,
    project_root: Path,
    recipe_id: str,
    merged_cfg: dict[str, Any] | None = None,
    cli_home: Path | None = None,
    refresh: bool = False,
) -> str:
    """Materialize a generated runtime hook script with gate provenance.

    Records a lock baseline of the exact bytes the CLI last rendered
    (``kind="gate"``, ``policy="auto"``) and classifies before writing:

    - baseline match + catalog drift → refresh and re-record (unmodified gate);
    - byte mismatch (user-modified) → preserve + warn with refresh guidance;
    - no baseline (unknown provenance) → preserve + warn, never seed.

    ``refresh=True`` (the ``--refresh-gates`` flag, never set by ordinary sync)
    replaces a customized gate only after its exact pre-refresh bytes are saved
    to the cache-only immutable backup. Returns the project-relative path.
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
    if GATE_SCOPE_PLACEHOLDER in content:
        scope = "auto"
        if merged_cfg is not None:
            scope = str(merged_cfg.get("gate_scope") or "auto")
        if scope not in GATE_SCOPE_VALUES:
            raise RuntimeError(
                f"invalid gate_scope '{scope}'; allowed: auto | superrepo | subrepo"
            )
        content = content.replace(GATE_SCOPE_PLACEHOLDER, scope)
    if REPO_TOPOLOGY_PLACEHOLDER in content:
        topology = "auto"
        if merged_cfg is not None:
            topology = str(merged_cfg.get("repo_topology") or "auto")
        if topology not in REPO_TOPOLOGY_VALUES:
            raise RuntimeError(
                f"invalid repo_topology '{topology}'; allowed: auto | standalone | monorepo-apps | monorepo-submodules"
            )
        content = content.replace(REPO_TOPOLOGY_PLACEHOLDER, topology)
    if GATE_IMPL_PLACEHOLDER in content:
        impl = "auto"
        if merged_cfg is not None:
            impl = str(merged_cfg.get("gate_impl") or "auto")
        if impl not in GATE_IMPL_VALUES:
            raise _invalid_gate_impl_error(impl)
        content = content.replace(GATE_IMPL_PLACEHOLDER, impl)
    if GATE_VERSION_PLACEHOLDER in content:
        version = "dev"
        if cli_home is not None:
            try:
                version = _load_cli_version().read_installed_version(Path(cli_home))
            except Exception:
                version = "dev"
        content = content.replace(GATE_VERSION_PLACEHOLDER, version)
    if TRACKER_CLI_HOME_PLACEHOLDER in content:
        home_val = str(Path(cli_home).resolve()) if cli_home is not None else ""
        content = content.replace(TRACKER_CLI_HOME_PLACEHOLDER, home_val)
    if TRACKER_LIB_INTERNAL_PLACEHOLDER in content:
        internal = (
            str((Path(cli_home) / "lib" / "_internal").resolve())
            if cli_home is not None
            else ""
        )
        content = content.replace(TRACKER_LIB_INTERNAL_PLACEHOLDER, internal)

    util = _load_util()
    lock_path = project_root / "ai-specs" / ".ai-specs.lock"
    lock = load_lock(lock_path)
    target = rel
    entry = (lock.get("managed") or {}).get(target)

    def record(written: bytes = content.encode()) -> None:
        set_gate_baseline(
            lock, target, util.sha256_bytes(written),
            recipe=recipe_id, source=hook.script,
        )
        write_lock(lock_path, lock)

    if refresh:
        _refresh_gate(
            project_root, dest, target, content, lock, lock_path,
            recipe_id, hook, cli_home,
        )
        print(f"    ✓ hook refreshed {rel}")
        return rel

    state = util.classify_managed_override(dest, entry, would_write=content)
    if state == "missing":
        dest.write_text(content)
        os.chmod(dest, 0o755)
        record()
        print(f"    ✓ hook script {rel}")
        return rel
    if state == "managed_current":
        record()
        print(f"    · hook skipped (current) {rel}")
        return rel
    if state == "managed_stale":
        # Baseline matches current bytes: the CLI rendered this gate, so an
        # ordinary sync may force-update it and re-record the baseline.
        dest.write_text(content)
        os.chmod(dest, 0o755)
        record()
        print(f"    ✓ hook refreshed (baseline matched) {rel}")
        return rel
    if state == "user_modified":
        warn(
            f"hook {rel} is user-modified; preserving existing bytes. Refresh with:\n"
            f"  rm {rel} && ai-specs sync  (or: ai-specs sync --refresh-gates)"
        )
        print(f"    · hook skipped (user-modified) {rel}")
        return rel
    warn(
        f"hook {rel} has no recorded provenance; preserving existing bytes. "
        "A baseline is recorded only when the CLI renders the gate. Refresh with:\n"
        f"  rm {rel} && ai-specs sync  (or: ai-specs sync --refresh-gates)"
    )
    print(f"    · hook skipped (no provenance) {rel}")
    return rel


# --- Binding resolution (Go authority, temporary Python fallback) --------------
#
# The Go gate binary (``--resolve-bindings``) owns capability binding
# resolution, capability conflict grading, and the durable tracker witness
# write. Python keeps only what is not the decision: TOML acquisition it already
# performs for other purposes, and the conflict -> error message formatting sync
# users see. This section is the bridge between them.
#
# The Python implementations below are a TEMPORARY fail-open fallback
# (``GO_BINDINGS_BRIDGE_FALLBACK``): they run only when the binary cannot be
# acquired, verified, executed, or parsed, and the strangler deletes them once
# the bridge is proven in the field. Tests pin their contract; they are never
# the primary authority.
GO_BINDINGS_BRIDGE_FALLBACK = "GO_BINDINGS_BRIDGE_FALLBACK"
GO_BINDINGS_BRIDGE_TIMEOUT_SECONDS = 60

# Stable machine-readable classification of the Go resolution errors. Go emits
# one deterministic message per failure and aborts step 1 on the first one; the
# prefix is that message's stable contract, so a caller branches on a code
# instead of matching a whole sentence. An unrecognized error keeps its Go text
# and classifies as "binding-error" rather than being dropped.
GO_BINDING_ERROR_CODES = {
    "duplicate explicit binding for capability": "duplicate-explicit-binding",
    "references disabled/unknown recipe": "disabled-or-unknown-recipe",
    "does not declare that capability": "undeclared-capability",
}


class BindingResolutionError(RuntimeError):
    """Invalid manifest binding, carrying the stable bridge error code.

    Subclasses RuntimeError so every existing ``except RuntimeError`` call site
    (sync, sync-agent, tests) keeps working unchanged; ``code`` is the
    machine-readable classification, and ``str(exc)`` stays the exact message
    the Python authority raised.
    """

    def __init__(self, message: str, code: str) -> None:
        super().__init__(message)
        self.code = code


def binding_error_code(message: str) -> str:
    """Classify one Go resolution error message into its stable bridge code."""
    for prefix, code in GO_BINDING_ERROR_CODES.items():
        if prefix in message:
            return code
    return "binding-error"


def _bridge_home(catalog_dir: Path) -> Path:
    """The AI_SPECS_HOME owning a ``<home>/catalog/recipes`` directory."""
    return Path(catalog_dir).resolve().parents[1]


def _warn_bridge_fallback(reason: str) -> None:
    """One greppable warning line for a degraded binding authority."""
    warn(
        f"{GO_BINDINGS_BRIDGE_FALLBACK}: {reason}; "
        "using the temporary Python binding authority"
    )


def _acquire_gate_binary(gb: Any, home: Path) -> Path | None:
    """Acquire the gate binary for the binding step, then re-resolve.

    Acquisition is opt-in (``go_binding_resolution(acquire_if_missing=True)``),
    only ever set by the sync binding step: it reuses the exact
    ``gate_binary.acquire`` the worktree-flow distribution step calls and the
    same ``AI_SPECS_GATE_OFFLINE`` signal, so there is one acquisition authority.
    The acquisition warning is deliberately NOT re-emitted here: the sync
    pipeline already reports acquisition degradation, and this bridge keeps its
    single ``GO_BINDINGS_BRIDGE_FALLBACK`` line. Never raises: a failed
    acquisition is a fallback, not a sync error.
    """
    try:
        gb.acquire(
            gate_impl="auto",
            ai_specs_home=home,
            offline=os.environ.get("AI_SPECS_GATE_OFFLINE") == "1",
        )
    except Exception:  # noqa: BLE001 - a failed acquisition degrades to fallback
        return None
    try:
        return gb.resolve_verified_binary(home)
    except Exception:  # noqa: BLE001 - an unloadable helper is "no binary"
        return None


def go_binding_resolution(
    catalog_dir: Path,
    enabled_ids: list[str],
    manifest_bindings: list[dict[str, str]],
    *,
    project_root: Path | None = None,
    write_witness: bool = False,
    acquire_if_missing: bool = False,
) -> dict[str, Any] | None:
    """Run the Go binding authority and return its JSON envelope, or None.

    None means the bridge could not run: no verified binary, the process failed,
    or the output was not the documented envelope. The caller then falls back to
    the temporary Python authority. This function emits the single
    ``GO_BINDINGS_BRIDGE_FALLBACK`` warning naming the reason, so a degraded run
    is never silent and never needs a second warning.

    ``write_witness`` lets Go own the durable witness in the same invocation.
    It never applies without ``project_root``: Go defaults the witness root to
    the process cwd, and a read-only caller must not inherit a write there.

    ``acquire_if_missing`` is the sync binding step's acquisition opt-in: when
    no verified binary resolves, the bridge acquires through the canonical
    ``gate_binary`` path (mirroring ``AI_SPECS_GATE_OFFLINE``) and re-resolves,
    so a fresh project's first sync does not fall back just because the
    worktree-flow recipe is disabled. Read-only callers (doctor, conflict
    grading) leave it False and never touch the network.
    """
    if project_root is None:
        write_witness = False
    try:
        gb = _load_gate_binary()
        home = _bridge_home(catalog_dir)
        binary = gb.resolve_verified_binary(home)
        if binary is None and acquire_if_missing:
            binary = _acquire_gate_binary(gb, home)
    except Exception as exc:  # noqa: BLE001 - an unloadable helper is "no binary"
        _warn_bridge_fallback(f"the gate binary could not be resolved ({type(exc).__name__}: {exc})")
        return None
    if binary is None:
        _warn_bridge_fallback("no verified worktree-gate binary")
        return None

    command = [str(binary), "--resolve-bindings", "--catalog-dir", str(catalog_dir)]
    for rid in enabled_ids:
        command += ["--recipe", str(rid)]
    command += ["--bindings", json.dumps(manifest_bindings)]
    if project_root is not None:
        command += ["--project-root", str(project_root)]
    # One token, not ``--write-witness false``: Go's flag package treats a
    # boolean flag as valueless, so a separate "false" argument would leave the
    # write ENABLED and demote the value to an ignored positional argument.
    write_flag = "true" if write_witness else "false"
    command += [f"--write-witness={write_flag}"]

    try:
        proc = subprocess.run(
            command,
            capture_output=True,
            text=True,
            timeout=GO_BINDINGS_BRIDGE_TIMEOUT_SECONDS,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        _warn_bridge_fallback(f"worktree-gate did not run ({type(exc).__name__}: {exc})")
        return None
    if proc.returncode != 0:
        detail = (proc.stderr or "").strip() or "no stderr"
        _warn_bridge_fallback(f"worktree-gate exited {proc.returncode} ({detail})")
        return None
    try:
        envelope = json.loads(proc.stdout)
    except ValueError as exc:
        _warn_bridge_fallback(f"worktree-gate output was not JSON ({exc})")
        return None
    if (
        not isinstance(envelope, dict)
        or not isinstance(envelope.get("bindings"), dict)
        or not isinstance(envelope.get("conflicts"), list)
        or not isinstance(envelope.get("errors"), list)
    ):
        _warn_bridge_fallback("worktree-gate output did not match the binding envelope")
        return None
    for warning in envelope.get("warnings") or []:
        # Go's best-effort side effects (a failed witness write) are reported
        # without demoting a resolution error that is already in ``errors``.
        warn(str(warning))
    return envelope


def _bindings_from_envelope(envelope: dict[str, Any]) -> dict[str, str]:
    """Adapt the Go envelope's bindings, raising exactly what Python raised.

    Go aborts step 1 on the first invalid explicit binding with the same
    deterministic message the Python authority raised, so parity is exact; the
    classification rides along as ``BindingResolutionError.code``.
    """
    errors = [str(error) for error in envelope.get("errors") or []]
    if errors:
        raise BindingResolutionError(errors[0], binding_error_code(errors[0]))
    return {str(cap): str(rid) for cap, rid in envelope["bindings"].items()}


def _conflicts_from_envelope(envelope: dict[str, Any]) -> list[Any]:
    """Adapt the Go envelope's conflicts onto the existing ``Conflict`` type.

    Call sites read ``primitive_type`` / ``primitive_id`` / ``recipes`` /
    ``severity``, so reusing the dataclass keeps the sync warning and blocking
    messages byte-identical to the ones the Python grader produced.
    """
    conflict_cls = _load_conflict().Conflict
    return [
        conflict_cls(
            primitive_type=str(item.get("type", "capability")),
            primitive_id=str(item.get("id", "")),
            recipes=set(item.get("recipes") or []),
            severity=str(item.get("severity") or "fatal"),
        )
        for item in envelope.get("conflicts") or []
    ]


def binding_resolution(
    catalog_dir: Path,
    enabled_ids: list[str],
    manifest_bindings: list[dict[str, str]],
    *,
    project_root: Path | None = None,
    write_witness: bool = False,
    acquire_if_missing: bool = False,
) -> tuple[dict[str, str], list[Any]]:
    """Resolve bindings and grade capability conflicts, Go first.

    One invocation returns both graders and, when ``write_witness`` is set, the
    durable tracker witness Go writes itself. When the bridge cannot run, the
    temporary Python authority computes both and writes the witness with the
    Python writer, so a degraded run still leaves a truthful ledger.

    Go persists the witness before the caller acts on a fatal conflict, exactly
    as the Python path would have: a fatal conflict is always a duplicate
    explicit binding, which is also a resolution error, so an unresolved
    binding is never persisted. Raises ``RuntimeError`` for an invalid manifest
    binding, exactly as ``resolve_bindings`` does.
    """
    envelope = go_binding_resolution(
        catalog_dir,
        enabled_ids,
        manifest_bindings,
        project_root=project_root,
        write_witness=write_witness,
        acquire_if_missing=acquire_if_missing,
    )
    if envelope is not None:
        return _bindings_from_envelope(envelope), _conflicts_from_envelope(envelope)
    bindings = _python_resolve_bindings(catalog_dir, enabled_ids, manifest_bindings)
    conflicts = _python_check_capability_conflicts(
        catalog_dir, enabled_ids, manifest_bindings
    )
    if write_witness:
        write_tracker_witness(project_root, catalog_dir, enabled_ids, bindings)
    return bindings, conflicts


def resolve_bindings(
    catalog_dir: Path,
    enabled_ids: list[str],
    manifest_bindings: list[dict[str, str]],
    *,
    project_root: Path | None = None,
    write_witness: bool = False,
    acquire_if_missing: bool = False,
) -> dict[str, str]:
    """Resolve capability-to-recipe bindings through the Go authority.

    Step 1 (validate explicit bindings) and step 2 (auto-bind a capability
    exactly one enabled recipe declares) run in ``worktree-gate
    --resolve-bindings``. Read-only callers (doctor, sync-agent) keep the
    defaults ``write_witness=False`` and ``acquire_if_missing=False``: they
    pass ``--write-witness=false`` so no caller other than sync can touch the
    ledger, and they never acquire or download a binary. Raises
    ``RuntimeError`` for an invalid manifest binding.
    """
    return binding_resolution(
        catalog_dir,
        enabled_ids,
        manifest_bindings,
        project_root=project_root,
        write_witness=write_witness,
        acquire_if_missing=acquire_if_missing,
    )[0]


# --- Binding resolution: TEMPORARY Python authority (GO_BINDINGS_BRIDGE_FALLBACK)
#
# Everything below is the fail-open fallback the strangler deletes once the Go
# bridge is proven: it runs only when the gate binary cannot run, and the run is
# always announced by a GO_BINDINGS_BRIDGE_FALLBACK warning line.
#
# --- Binding resolution -------------------------------------------------------
def _python_resolve_bindings(
    catalog_dir: Path, enabled_ids: list[str], manifest_bindings: list[dict[str, str]]
) -> dict[str, str]:
    """Resolve capability-to-recipe bindings (TEMPORARY Python authority).

    Kept only as the fail-open fallback announced by
    ``GO_BINDINGS_BRIDGE_FALLBACK``; ``worktree-gate --resolve-bindings`` is the
    primary authority.

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


# --- Binding witness (A4) -----------------------------------------------------
LEDGER_CAPABILITY = "tracker"
LEDGER_WITNESS_VERSION = 1
LEDGER_WITNESS_RELPATH = Path("ai-specs") / "ledger" / "witness.json"
WITNESS_BOUND = "bound"
WITNESS_AMBIGUOUS = "ambiguous"
WITNESS_UNBOUND = "unbound"
WITNESS_DECLARED_NOT_BOUND = "declared-not-bound"


def git_common_dir(project_root: Path) -> str:
    """Absolute Git common dir for project_root, or "" outside a repository.

    Mirrors the Go reader (``catalog/recipes/worktree-flow/gate/gitfacts.go``):
    prefer the absolute form, fall back to the relative one, then realpath so a
    linked worktree and its main checkout resolve to one shared directory (A2).
    """
    def run(*args: str) -> str:
        try:
            proc = subprocess.run(
                ["git", "-C", str(project_root), *args],
                capture_output=True, text=True, check=False,
            )
        except OSError:
            return ""
        return proc.stdout.strip() if proc.returncode == 0 else ""

    value = run("rev-parse", "--path-format=absolute", "--git-common-dir")
    if not value:
        value = run("rev-parse", "--git-common-dir")
    if not value:
        return ""
    path = Path(value)
    if not path.is_absolute():
        path = Path(project_root) / path
    return str(path.resolve())


def tracking_declared(project_root: Path) -> bool:
    """True when openspec/config.yaml declares a top-level ``tracking:`` block.

    The declaration is supply, never activation (D6); it only distinguishes the
    witness's ``declared-not-bound`` state from plain ``unbound``.
    """
    config = Path(project_root) / "openspec" / "config.yaml"
    if not config.is_file():
        return False
    try:
        lines = config.read_text().splitlines()
    except OSError:
        return False
    return any(line.startswith("tracking:") for line in lines)


def tracker_witness_payload(
    catalog_dir: Path,
    enabled_ids: list[str],
    resolved_bindings: dict[str, str],
    *,
    declared: bool,
    written_at: str,
) -> dict[str, Any]:
    """Describe the already-resolved tracker binding (A4).

    There is no verdict logic here: ``resolve_bindings`` stays the only binding
    producer, and an unresolved capability records candidate recipe ids instead
    of guessing a provider (D6).
    """
    recipe_id = resolved_bindings.get(LEDGER_CAPABILITY, "")
    candidates: list[str] = []
    if recipe_id:
        state = WITNESS_BOUND
    else:
        for rid in enabled_ids:
            try:
                recipe = read_recipe(catalog_dir, rid)
            except Exception:
                continue
            if any(cap.id == LEDGER_CAPABILITY for cap in recipe.capabilities):
                candidates.append(rid)
        if len(candidates) > 1:
            state = WITNESS_AMBIGUOUS
        elif declared:
            state = WITNESS_DECLARED_NOT_BOUND
        else:
            state = WITNESS_UNBOUND
    return {
        "v": LEDGER_WITNESS_VERSION,
        "capability": LEDGER_CAPABILITY,
        "state": state,
        "recipe_id": recipe_id,
        "candidates": candidates,
        "written_at": written_at,
    }


def write_tracker_witness(
    project_root: Path,
    catalog_dir: Path,
    enabled_ids: list[str],
    resolved_bindings: dict[str, str],
) -> Path | None:
    """Atomically persist the resolved tracker binding under the common dir (A4).

    TEMPORARY fail-open fallback (``GO_BINDINGS_BRIDGE_FALLBACK``):
    ``worktree-gate --resolve-bindings`` normally writes this witness itself in
    the invocation that resolves the bindings, and the strangler deletes this
    writer with the rest of the Python binding authority. It stays reachable for
    the degraded path (no verified binary) and for tests that pin its contract.

    ``<git-common-dir>/ai-specs/ledger/witness.json`` — deliberately outside
    ``RESOLVED_CONFIG_TEMP`` so ``lib/sync.sh``'s EXIT trap cannot delete it.
    Best-effort: outside a repository there is no common dir, and a write error
    leaves the ledger dormant (missing witness) instead of aborting sync. The Go
    reader treats a missing or unreadable witness as dormant, never bound.
    """
    common = git_common_dir(project_root)
    if not common:
        return None
    payload = tracker_witness_payload(
        catalog_dir,
        enabled_ids,
        resolved_bindings,
        declared=tracking_declared(project_root),
        written_at=datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    )
    target = Path(common) / LEDGER_WITNESS_RELPATH
    try:
        target.parent.mkdir(parents=True, exist_ok=True)
        fd, tmp = tempfile.mkstemp(
            dir=target.parent, prefix="witness.json.tmp.", suffix=".json"
        )
        try:
            with os.fdopen(fd, "w") as f:
                json.dump(payload, f, indent=2, sort_keys=True)
                f.write("\n")
                f.flush()
                os.fsync(f.fileno())
            os.replace(tmp, target)
        except BaseException:
            try:
                os.unlink(tmp)
            except OSError:
                pass
            raise
    except OSError as exc:
        warn(
            f"tracker witness not written ({type(exc).__name__}: {exc}); "
            "ledger stays dormant"
        )
        return None
    return target


# --- Config merge (Go --plan-merge-config) ------------------------------------
#
# Go owns the merge_config DECISION: schema defaults + manifest overrides +
# structured-config validation. Python keeps ACQUISITION of the already-loaded
# Recipe and serializes its schema into the Go stdin envelope. The Python
# decision below is a TEMPORARY fail-open fallback
# (``GO_MERGE_CONFIG_BRIDGE_FALLBACK``) the strangler deletes once the bridge is
# proven: it runs only when the binary cannot be acquired, executed, or parsed,
# and the run is always announced by exactly one warning line.
GO_MERGE_CONFIG_BRIDGE_FALLBACK = "GO_MERGE_CONFIG_BRIDGE_FALLBACK"
GO_MERGE_CONFIG_BRIDGE_TIMEOUT_SECONDS = 60


def _warn_merge_bridge_fallback(reason: str) -> None:
    """One greppable warning line for a degraded merge authority."""
    warn(
        f"{GO_MERGE_CONFIG_BRIDGE_FALLBACK}: {reason}; "
        "using the temporary Python merge authority"
    )


def _merge_config_request_envelope(
    recipe: Any, manifest_config: dict[str, Any]
) -> dict[str, Any]:
    """Serialize the already-loaded Recipe schema into the Go stdin envelope.

    Ordered surfaces (schema fields, declared tables, manifest pairs) cross as
    arrays so Go never depends on map iteration. ``has_default`` transports
    Python's "default is not None", so defaults of false, 0 and "" apply.
    Values cross verbatim: anything JSON cannot carry (TOML date/datetime/time,
    nonfinite floats) fails envelope serialization and the whole merge falls
    back once to the Python authority, preserving its typed values and
    validation failures. No coercion happens at this boundary.
    """
    schema = getattr(recipe, "config_schema", None)
    schema_fields = schema.fields if schema is not None else {}
    schema_tables = getattr(schema, "tables", {}) or {}
    fields = []
    for key, field in schema_fields.items():
        has_default = field.default is not None
        fields.append(
            {
                "key": key,
                "required": bool(field.required),
                "has_default": has_default,
                "default": field.default if has_default else None,
                "enum": list(field.enum) if field.enum else [],
            }
        )
    tables = [
        {"key": key, "shape": table.shape} for key, table in schema_tables.items()
    ]
    manifest = [
        {"key": key, "value": value} for key, value in manifest_config.items()
    ]
    return {
        "recipe_name": recipe.name,
        "fields": fields,
        "tables": tables,
        "manifest": manifest,
    }


def _is_merge_config_result(result: Any) -> bool:
    """True when the decoded stdout is the documented merge-config envelope."""
    if not isinstance(result, dict):
        return False
    warnings = result.get("warnings")
    return (
        isinstance(result.get("config"), dict)
        and isinstance(warnings, list)
        and all(isinstance(item, str) for item in warnings)
        and isinstance(result.get("error"), str)
    )


def go_merge_config(
    recipe: Any,
    manifest_config: dict[str, Any],
    *,
    home: Path | None = None,
) -> dict[str, Any] | None:
    """Run ``worktree-gate --plan-merge-config`` and return its result, or None.

    ``home`` is the active CLI home owning the version-keyed gate binary cache
    (mirroring the sibling bridges); None keeps the historical package-root
    resolution.

    None means the bridge could not run: no verified binary, the request could
    not be serialized, the process failed, or the output was not the documented
    envelope. The caller then falls back to the temporary Python authority. This
    function emits the single ``GO_MERGE_CONFIG_BRIDGE_FALLBACK`` warning naming
    the reason, so a degraded run is never silent and never needs a second
    warning.

    A semantic validation error is NOT a transport failure: Go reports it in
    the envelope's ``error`` field with exit 0, so the caller raises it without
    falling back.
    """
    try:
        gb = _load_gate_binary()
        binary = gb.resolve_verified_binary(_orphans_bridge_home(home))
    except Exception as exc:  # noqa: BLE001 - an unloadable helper is "no binary"
        _warn_merge_bridge_fallback(
            f"the gate binary could not be resolved ({type(exc).__name__}: {exc})"
        )
        return None
    if binary is None:
        _warn_merge_bridge_fallback("no verified worktree-gate binary")
        return None
    try:
        request = json.dumps(
            _merge_config_request_envelope(recipe, manifest_config),
            allow_nan=False,
        )
    except (TypeError, ValueError) as exc:
        _warn_merge_bridge_fallback(
            f"the config envelope could not be serialized "
            f"({type(exc).__name__}: {exc})"
        )
        return None
    try:
        # Locale-proof decode: undecodable Go bytes become U+FFFD (never an
        # exception), so the JSON parse below degrades to the one fallback.
        proc = subprocess.run(
            [str(binary), "--plan-merge-config"],
            input=request,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
            timeout=GO_MERGE_CONFIG_BRIDGE_TIMEOUT_SECONDS,
        )
    except (OSError, subprocess.SubprocessError, UnicodeDecodeError) as exc:
        _warn_merge_bridge_fallback(
            f"worktree-gate did not run ({type(exc).__name__}: {exc})"
        )
        return None
    if proc.returncode != 0:
        detail = (proc.stderr or "").strip() or "no stderr"
        _warn_merge_bridge_fallback(
            f"worktree-gate exited {proc.returncode} ({detail})"
        )
        return None
    try:
        result = json.loads(proc.stdout)
    except ValueError as exc:
        _warn_merge_bridge_fallback(f"worktree-gate output was not JSON ({exc})")
        return None
    if not _is_merge_config_result(result):
        _warn_merge_bridge_fallback(
            "worktree-gate output did not match the merge-config envelope"
        )
        return None
    return result


def merge_config(
    recipe: Any,
    manifest_config: dict[str, Any],
    *,
    home: Path | None = None,
) -> dict[str, Any]:
    """Merge recipe config schema defaults with manifest overrides, Go first.

    Fails if any required=True field is missing in the final dict. Carries a
    declared structured (table) section such as ``reconcile`` through after
    validating it against the recipe's declarative shape. Warns for any other
    manifest key not in the schema.

    The Go gate binary (``--plan-merge-config``) is the primary authority; the
    whole response is validated before any warning is emitted or any result is
    returned, so a degraded run never produces partial output. When the bridge
    cannot run, ``_python_merge_config`` computes the same decision.
    """
    result = go_merge_config(recipe, manifest_config, home=home)
    if result is not None:
        for warning in result["warnings"]:
            warn(warning)
        if result["error"]:
            raise RuntimeError(result["error"])
        return result["config"]
    return _python_merge_config(recipe, manifest_config)


def _python_merge_config(
    recipe: Any, manifest_config: dict[str, Any]
) -> dict[str, Any]:
    """Compute the config merge (TEMPORARY Python authority).

    Kept only as the fail-open fallback announced by
    ``GO_MERGE_CONFIG_BRIDGE_FALLBACK``; ``worktree-gate --plan-merge-config``
    is the primary authority.
    """
    result: dict[str, Any] = {}
    schema = getattr(recipe, "config_schema", None)
    schema_fields = schema.fields if schema is not None else {}
    schema_tables = getattr(schema, "tables", {}) or {}

    # Start with defaults
    for key, field in schema_fields.items():
        if field.default is not None:
            result[key] = field.default

    # Overlay manifest values
    for key, value in manifest_config.items():
        if key in schema_fields:
            result[key] = value
            continue
        if key in schema_tables:
            try:
                _load_recipe_schema().validate_structured_config(key, value)
            except _load_recipe_schema().RecipeValidationError as exc:
                raise RuntimeError(
                    f"recipe '{recipe.name}': invalid config field '{key}': {exc}"
                ) from exc
            result[key] = value
            continue
        warn(f"recipe '{recipe.name}': unknown config key '{key}' in manifest (ignored)")
    if "gate_scope" in schema_fields and not str(result.get("gate_scope") or "").strip():
        result["gate_scope"] = "auto"

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
            if key == "gate_impl":
                raise _invalid_gate_impl_error(value_str)
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
def _resolve_provider_markers(
    merged: dict[str, Any],
    catalog_dir: Path,
    recipe_ids: list[str],
    ai_specs_home: Path,
) -> None:
    provider_install = _load_provider_install()
    for rid in recipe_ids:
        try:
            recipe = read_recipe(catalog_dir, rid)
        except Exception as exc:  # noqa: BLE001
            warn(f"recipe '{rid}': cannot read recipe for provider resolution ({exc})")
            continue
        dependencies = {dep.binary: dep for dep in recipe.cli_deps}
        for preset in recipe.mcp:
            config = merged.get(preset.id)
            if not isinstance(config, dict) or config.get("command") != "{dep:jinna}":
                continue
            dep = dependencies.get("jinna")
            if dep is None:
                merged.pop(preset.id, None)
                warn(f"recipe '{recipe.name}': missing jinna dependency declaration")
                continue
            try:
                resolution = provider_install.resolve_provider(
                    dep, ai_specs_home=ai_specs_home
                )
            except Exception as exc:  # noqa: BLE001
                # A malformed/unreadable managed cache must degrade to
                # unresolved, never abort sync with a traceback.
                merged.pop(preset.id, None)
                warn(
                    f"recipe '{recipe.name}': provider jinna could not be resolved "
                    f"({exc}); run ai-specs configure-recipes interactively to install it"
                )
                continue
            if resolution.verified:
                resolved = dict(config)
                resolved["command"] = resolution.command
                merged[preset.id] = resolved
                continue
            merged.pop(preset.id, None)
            warn(
                f"recipe '{recipe.name}': provider jinna is unresolved; "
                "run ai-specs configure-recipes interactively to install it"
            )


def build_recipe_mcp(
    catalog_dir: Path,
    recipe_ids: list[str],
    manifest_mcp: dict[str, Any],
    ai_specs_home: Path | None = None,
) -> dict[str, Any]:
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
    if ai_specs_home is not None:
        _resolve_provider_markers(merged, catalog_dir, recipe_ids, Path(ai_specs_home))
    return merged


# --- Orphan cleanup bridge (Go --plan-orphans) --------------------------------
#
# Go owns the orphan DECISION: which materialized recipe/dep names and which
# lock recipe ids the manifest no longer expects. Python keeps ACQUISITION
# (listing the project cache roots and reading the lock) and EXECUTION
# (``shutil.rmtree``, ``remove_recipe_lock_entries`` + ``write_lock``). The
# Python decision below is a TEMPORARY fail-open fallback
# (``GO_ORPHANS_BRIDGE_FALLBACK``) the strangler deletes once the bridge is
# proven: it runs only when the binary cannot be acquired, executed, or parsed,
# and the run is always announced by exactly one warning line.
GO_ORPHANS_BRIDGE_FALLBACK = "GO_ORPHANS_BRIDGE_FALLBACK"
GO_ORPHANS_BRIDGE_TIMEOUT_SECONDS = 60

_ORPHAN_PLAN_KEYS = (
    "orphaned_recipes",
    "orphaned_deps",
    "orphaned_inproject_deps",
    "stale_lock_recipes",
)


def _warn_orphans_bridge_fallback(reason: str) -> None:
    """One greppable warning line for a degraded orphan authority."""
    warn(
        f"{GO_ORPHANS_BRIDGE_FALLBACK}: {reason}; "
        "using the temporary Python orphan authority"
    )


def _orphans_bridge_home(cli_home: Path | None) -> Path:
    """The AI_SPECS_HOME owning the version-keyed gate binary cache."""
    if cli_home is not None:
        return Path(cli_home).resolve()
    return Path(__file__).resolve().parents[2]


def _is_orphan_envelope(envelope: Any) -> bool:
    """True when the decoded stdout is the documented sorted-list envelope."""
    if not isinstance(envelope, dict):
        return False
    return all(isinstance(envelope.get(key), list) for key in _ORPHAN_PLAN_KEYS)


def go_orphan_plan(home: Path, plan_input: dict[str, Any]) -> dict[str, Any] | None:
    """Run ``worktree-gate --plan-orphans`` and return its JSON envelope, or None.

    None means the bridge could not run: no verified binary, the process failed,
    or the output was not the documented envelope. The caller then falls back to
    the temporary Python authority. This function emits the single
    ``GO_ORPHANS_BRIDGE_FALLBACK`` warning naming the reason, so a degraded run
    is never silent and never needs a second warning.
    """
    try:
        gb = _load_gate_binary()
        binary = gb.resolve_verified_binary(home)
    except Exception as exc:  # noqa: BLE001 - an unloadable helper is "no binary"
        _warn_orphans_bridge_fallback(
            f"the gate binary could not be resolved ({type(exc).__name__}: {exc})"
        )
        return None
    if binary is None:
        _warn_orphans_bridge_fallback("no verified worktree-gate binary")
        return None

    try:
        proc = subprocess.run(
            [str(binary), "--plan-orphans"],
            input=json.dumps(plan_input),
            capture_output=True,
            text=True,
            timeout=GO_ORPHANS_BRIDGE_TIMEOUT_SECONDS,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        _warn_orphans_bridge_fallback(
            f"worktree-gate did not run ({type(exc).__name__}: {exc})"
        )
        return None
    if proc.returncode != 0:
        detail = (proc.stderr or "").strip() or "no stderr"
        _warn_orphans_bridge_fallback(
            f"worktree-gate exited {proc.returncode} ({detail})"
        )
        return None
    try:
        envelope = json.loads(proc.stdout)
    except ValueError as exc:
        _warn_orphans_bridge_fallback(f"worktree-gate output was not JSON ({exc})")
        return None
    if not _is_orphan_envelope(envelope):
        _warn_orphans_bridge_fallback(
            "worktree-gate output did not match the orphan envelope"
        )
        return None
    return envelope


def _dir_child_names(directory: Path) -> list[str]:
    """Directory child names of one materialized scope, sorted; files ignored."""
    if not directory.is_dir():
        return []
    return sorted(child.name for child in directory.iterdir() if child.is_dir())


def _orphan_plan_input(
    recipe_dir: Path,
    deps_dir: Path,
    inproject_deps: Path,
    lock: dict[str, Any],
    enabled_recipe_ids: set[str],
    expected_dep_ids: set[str],
) -> dict[str, list[str]]:
    """The stdin envelope: the same names the Python authority used to compare."""
    return {
        "recipe_skills": _dir_child_names(recipe_dir),
        "deps_skills": _dir_child_names(deps_dir),
        "inproject_deps": _dir_child_names(inproject_deps),
        "lock_recipes": sorted(lock.get("recipes") or {}),
        "enabled_recipe_ids": sorted(enabled_recipe_ids),
        "expected_dep_ids": sorted(expected_dep_ids),
    }


def _python_orphan_plan(
    recipe_dir: Path,
    deps_dir: Path,
    inproject_deps: Path,
    lock: dict[str, Any],
    enabled_recipe_ids: set[str],
    expected_dep_ids: set[str],
) -> dict[str, list[str]]:
    """Compute the orphan plan (TEMPORARY Python authority).

    Kept only as the fail-open fallback announced by
    ``GO_ORPHANS_BRIDGE_FALLBACK``; ``worktree-gate --plan-orphans`` is the
    primary authority. Sorted so both paths produce byte-identical output.
    """
    return {
        "orphaned_recipes": [
            name
            for name in _dir_child_names(recipe_dir)
            if name not in enabled_recipe_ids
        ],
        "orphaned_deps": [
            name
            for name in _dir_child_names(deps_dir)
            if name not in expected_dep_ids
        ],
        "orphaned_inproject_deps": [
            name
            for name in _dir_child_names(inproject_deps)
            if name not in expected_dep_ids
        ],
        "stale_lock_recipes": sorted(
            rid
            for rid in (lock.get("recipes") or {})
            if rid not in enabled_recipe_ids
        ),
    }


def clean_orphans(
    project_root: Path,
    enabled_recipe_ids: set[str],
    expected_dep_ids: set[str],
    cli_home: Path | None = None,
) -> None:
    pc = _load_project_cache()
    recipe_dir = pc.recipe_skills_root(project_root, cli_home=cli_home)
    deps_dir = pc.deps_skills_root(project_root, cli_home=cli_home)
    # In-project toml-dep materialization (ai-specs/.deps/) is pruned for deps
    # no longer declared in the manifest.
    inproject_deps = pc.inproject_deps_root(project_root)
    lock_path = project_root / "ai-specs" / ".ai-specs.lock"
    lock = load_lock(lock_path) if lock_path.is_file() else {}

    plan = go_orphan_plan(
        _orphans_bridge_home(cli_home),
        _orphan_plan_input(
            recipe_dir, deps_dir, inproject_deps, lock,
            enabled_recipe_ids, expected_dep_ids,
        ),
    )
    if plan is None:
        plan = _python_orphan_plan(
            recipe_dir, deps_dir, inproject_deps, lock,
            enabled_recipe_ids, expected_dep_ids,
        )

    for name in plan["orphaned_recipes"]:
        child = recipe_dir / name
        if child.is_dir():
            shutil.rmtree(child)
            print(f"  ✓ removed orphaned cache .recipe/{name}")

    for name in plan["orphaned_deps"]:
        child = deps_dir / name
        if child.is_dir():
            shutil.rmtree(child)
            print(f"  ✓ removed orphaned cache .deps/{name}")

    for name in plan["orphaned_inproject_deps"]:
        child = inproject_deps / name
        if child.is_dir():
            shutil.rmtree(child)
            print(f"  ✓ removed orphaned ai-specs/.deps/{name}")

    # Clean up stale lock entries for recipes no longer in the manifest
    if lock_path.is_file():
        removed_any = False
        for rid in plan["stale_lock_recipes"]:
            if remove_recipe_lock_entries(lock, rid):
                removed_any = True
                print(f"  ✓ removed stale lock entries for recipe '{rid}'")
        if removed_any:
            write_lock(lock_path, lock)


# --- Main ---------------------------------------------------------------------
# --- Resolved-config projection: Go authority (GO_RESOLVED_CONFIG_BRIDGE_FALLBACK)
#
# The Go gate binary (``--plan-resolved-config``) owns the resolved-config
# PROJECTION: capability bindings, the per-recipe config (flat + config
# sub-table), the enabled id list, the resolved project root, and the resolved
# topology. Python keeps only the manifest acquisition it already performs and
# the TEMPORARY fail-open fallback below
# (``GO_RESOLVED_CONFIG_BRIDGE_FALLBACK``). This bridge is read-only: it never
# acquires a binary, so doctor and sync only degrade when no verified binary is
# already available.
GO_RESOLVED_CONFIG_BRIDGE_FALLBACK = "GO_RESOLVED_CONFIG_BRIDGE_FALLBACK"
GO_RESOLVED_CONFIG_BRIDGE_TIMEOUT_SECONDS = 60

_RESOLVED_TOPOLOGY_STRING_KEYS = ("resolved", "configured", "via", "source")


def _warn_resolved_bridge_fallback(reason: str) -> None:
    """One greppable warning line for a degraded resolved-config authority."""
    warn(
        f"{GO_RESOLVED_CONFIG_BRIDGE_FALLBACK}: {reason}; "
        "using the temporary Python projection authority"
    )


def _is_str_keyed_mapping(value: Any, value_type: type) -> bool:
    """True for a ``{str: value_type}`` mapping (empty mappings included)."""
    if not isinstance(value, dict):
        return False
    return all(
        isinstance(key, str) and isinstance(item, value_type)
        for key, item in value.items()
    )


def _is_resolved_config_envelope(envelope: Any) -> bool:
    """True when the decoded stdout is the documented resolved-config envelope."""
    if not isinstance(envelope, dict):
        return False
    topology = envelope.get("topology")
    if not isinstance(topology, dict):
        return False
    return (
        _is_str_keyed_mapping(envelope.get("bindings"), str)
        and _is_str_keyed_mapping(envelope.get("recipes"), dict)
        and isinstance(envelope.get("enabled"), list)
        and isinstance(envelope.get("project_root"), str)
        and all(
            isinstance(topology.get(key), str)
            for key in _RESOLVED_TOPOLOGY_STRING_KEYS
        )
        and isinstance(topology.get("submodules"), list)
        and isinstance(topology.get("gitmodules_present"), bool)
    )


def go_resolved_config(
    project_root: Path, *, ai_specs_home: Path | None = None
) -> dict[str, Any] | None:
    """Run ``worktree-gate --plan-resolved-config`` and return its JSON envelope, or None.

    None means the bridge could not run: no verified binary, the process failed,
    or the output was not the documented envelope. The caller then falls back to
    the temporary Python authority. This function emits the single
    ``GO_RESOLVED_CONFIG_BRIDGE_FALLBACK`` warning naming the reason, so a
    degraded run is never silent and never needs a second warning.

    This bridge is read-only and never acquires a binary: doctor and sync both
    either have a verified binary or degrade to the Python projection.
    """
    home = (
        ai_specs_home
        if ai_specs_home is not None
        else Path(__file__).resolve().parents[2]
    )
    try:
        gb = _load_gate_binary()
        binary = gb.resolve_verified_binary(home)
    except Exception as exc:  # noqa: BLE001 - an unloadable helper is "no binary"
        _warn_resolved_bridge_fallback(
            f"the gate binary could not be resolved ({type(exc).__name__}: {exc})"
        )
        return None
    if binary is None:
        _warn_resolved_bridge_fallback("no verified worktree-gate binary")
        return None

    try:
        proc = subprocess.run(
            [str(binary), "--plan-resolved-config", "--project", str(project_root)],
            capture_output=True,
            text=True,
            timeout=GO_RESOLVED_CONFIG_BRIDGE_TIMEOUT_SECONDS,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        _warn_resolved_bridge_fallback(
            f"worktree-gate did not run ({type(exc).__name__}: {exc})"
        )
        return None
    if proc.returncode != 0:
        detail = (proc.stderr or "").strip() or "no stderr"
        _warn_resolved_bridge_fallback(
            f"worktree-gate exited {proc.returncode} ({detail})"
        )
        return None
    try:
        envelope = json.loads(proc.stdout)
    except ValueError as exc:
        _warn_resolved_bridge_fallback(f"worktree-gate output was not JSON ({exc})")
        return None
    if not _is_resolved_config_envelope(envelope):
        _warn_resolved_bridge_fallback(
            "worktree-gate output did not match the resolved-config envelope"
        )
        return None
    return envelope


def _python_build_resolved_config(project_root: Path) -> dict[str, Any]:
    """Build a resolved-config JSON blob from raw manifest data (TEMPORARY authority).

    Kept only as the fail-open fallback announced by
    ``GO_RESOLVED_CONFIG_BRIDGE_FALLBACK``; ``worktree-gate
    --plan-resolved-config`` is the primary authority.

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

    # Request-context propagation: the root manifest project is the canonical
    # planning root for every fan-out target, and the resolved topology is
    # stamped so downstream renderers never re-derive it from a subrepo cwd.
    resolved: dict[str, Any] = {
        "bindings": bindings_out,
        "recipes": recipes_out,
        "enabled": enabled_ids,
        "project_root": str(Path(project_root).resolve()),
    }
    # Topology is project-owned (CLI-resolved), never re-derived per recipe.
    try:
        topo = _load_util().project_repo_topology(project_root, manifest_data)
        resolved["topology"] = topo.as_dict()
    except Exception:
        resolved["topology"] = {
            "resolved": "standalone",
            "configured": "auto",
            "via": "auto",
            "source": "default",
        }
    return resolved


def build_resolved_config(
    project_root: Path, ai_specs_home: Path | None = None
) -> dict[str, Any]:
    """Build the resolved-config blob, Go first with a fail-open Python fallback."""
    envelope = go_resolved_config(project_root, ai_specs_home=ai_specs_home)
    if envelope is not None:
        return envelope
    return _python_build_resolved_config(project_root)


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


def materialize_recipes(project_root: Path, ai_specs_home: Path, recipe_mcp_out: Path | None = None, resolved_config_out: Path | None = None, resolved_hooks_out: Path | None = None, refresh_gates: bool = False) -> int:
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
        # Recover the witness ownership rule for a shrinking enabled set through
        # the same Go authority as the enabled path, so a disabled provider never
        # stays active. The fallback writes it in Python when Go cannot run.
        binding_resolution(
            catalog_dir, [], [],
            project_root=project_root, write_witness=True, acquire_if_missing=True,
        )
        print("  (no [recipes.*] enabled — skipping)")
        # Still write resolved-config if requested (even with no enabled recipes)
        if resolved_config_out is not None:
            resolved = build_resolved_config(project_root, ai_specs_home=ai_specs_home)
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

    # Binding resolution, capability conflict grading, and the durable tracker
    # witness are ONE Go invocation (single authority, single durable write).
    # resolve_bindings just computed the binding the Go ledger reads; nothing
    # downstream re-derives it.
    resolved_bindings, cap_conflicts = binding_resolution(
        catalog_dir,
        list(enabled.keys()),
        manifest_bindings,
        project_root=project_root,
        write_witness=True,
        # The sync binding step owns acquisition, independent of worktree-flow
        # enablement, so the first sync of a fresh project does not fall back.
        acquire_if_missing=True,
    )
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

    # Durable tracker binding witness (A4): Go persisted the binding outcome
    # during the invocation above, outside RESOLVED_CONFIG_TEMP, so the EXIT trap
    # cannot delete it.

    # Recipe-owned reconcile mapping reaches the manifest here: the gate reads
    # only the manifest, so declared defaults must be stamped during sync.
    stamp_recipe_reconcile_defaults(project_root, catalog_dir, list(enabled.keys()))

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

    recipe_mcp = build_recipe_mcp(
        catalog_dir,
        list(enabled.keys()),
        manifest_mcp,
        ai_specs_home=cli_home,
    )

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
            merged_cfg = merge_config(recipe, manifest_config, home=cli_home)
        except RuntimeError as exc:
            fail(str(exc))
        # Project-owned keys win over their legacy recipe alias so every stamp
        # and rendered template carries one resolved value.
        merged_cfg = util.project_owned_recipe_config(
            project_root, None, rid, merged_cfg
        )

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
            script_path = materialize_hook_script(recipe_dir, rhook, project_root, rid, merged_cfg, cli_home=cli_home, refresh=refresh_gates)
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

        # Phase 3 distribution (worktree-flow only): acquire the Go binary
        # when gate_impl is auto or go. Acquisition never fails sync.
        if rid == "worktree-flow":
            impl = str(merged_cfg.get("gate_impl") or "auto")
            if impl in ("auto", "go"):
                try:
                    gb = _load_gate_binary()
                    status = gb.acquire(
                        gate_impl=impl,
                        ai_specs_home=Path(ai_specs_home),
                        offline=os.environ.get("AI_SPECS_GATE_OFFLINE") == "1",
                    )
                    if status.get("warn"):
                        warn(f"worktree-flow: {status['warn']}")
                except Exception as exc:  # noqa: BLE001
                    warn(
                        f"worktree-flow: gate binary acquisition failed "
                        f"({type(exc).__name__}: {exc}); "
                        "gate degrades per gate_impl (see 'ai-specs doctor')"
                    )

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
        resolved = build_resolved_config(project_root, ai_specs_home=ai_specs_home)
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
        resolved = build_resolved_config(project_root, ai_specs_home=ai_specs_home)

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
    refresh_gates = False
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
    if "--refresh-gates" in args:
        idx = args.index("--refresh-gates")
        refresh_gates = True
        args = args[:idx] + args[idx + 1:]
    if len(args) != 2:
        print(
            f"Usage: {sys.argv[0]} <project_root> <ai_specs_home>"
            " [--recipe-mcp-out <path>] [--resolved-config-out <path>]"
            " [--resolved-hooks-out <path>] [--resolved-config-only]"
            " [--refresh-gates]",
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
        return materialize_recipes(
            project_root, ai_specs_home, recipe_mcp_out, resolved_config_out,
            resolved_hooks_out, refresh_gates=refresh_gates,
        )
    except Exception as exc:
        fail(str(exc))
        return 1


if __name__ == "__main__":
    sys.exit(main())