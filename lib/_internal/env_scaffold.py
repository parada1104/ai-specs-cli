#!/usr/bin/env python3
"""Harness env scaffolding: root ai-specs.env + merge-safe root .envrc + direnv allow."""
from __future__ import annotations

import argparse
import importlib.util
import os
import re
import sys
from pathlib import Path

MANAGED_START = "# managed-by: ai-specs (do not remove block)"
MANAGED_END = "# end managed-by: ai-specs"
MANAGED_BODY = "dotenv_if_exists .env\ndotenv_if_exists ai-specs.env"

HARNESS_ENV_NAME = "ai-specs.env"
HARNESS_ENV_EXAMPLE_NAME = "ai-specs.env.example"

_EXPORT_RE = re.compile(
    r"^\s*export\s+([A-Za-z_][A-Za-z0-9_]*)=(.*)$"
)
_DOTENV_RE = re.compile(
    r"^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$"
)


def _load_sibling(name: str):
    path = Path(__file__).with_name(f"{name}.py")
    spec = importlib.util.spec_from_file_location(name.replace("-", "_"), path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load sibling module {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


_toml_read = _load_sibling("toml-read")
_recipe_read = _load_sibling("recipe-read")
_recipe_init = _load_sibling("recipe-init")
ENV_REFERENCE_RE = _recipe_init.ENV_REFERENCE_RE

_ENV_EXAMPLE_HEADER = """\
# ai-specs.env.example — committed template (safe to regenerate).
# Copy values into ai-specs.env (gitignored). Root .envrc loads it via direnv.
# Generated from enabled recipes' [[provides.mcp]] env references.
"""

_LEGACY_ENV_EXAMPLE_STUB = """\
# DEPRECATED: use project-root ai-specs.env.example instead.
# Root .envrc is managed by ai-specs (dotenv_if_exists ai-specs.env).
# Regenerate with: ai-specs configure-recipes
"""

_ENVRC_EXAMPLE_STUB = """\
# DEPRECATED: use project-root ai-specs.env.example instead.
# Root .envrc is managed by ai-specs (dotenv_if_exists ai-specs.env).
# Regenerate with: ai-specs configure-recipes
"""

ENV_VAR_HELP: dict[str, str] = {
    "TRELLO_API_KEY": (
        "Trello API key — create at https://trello.com/power-ups/admin"
    ),
    "TRELLO_TOKEN": (
        "Trello token — generate from https://trello.com/power-ups/admin "
        "(Power-Up → API key page → Token)"
    ),
    "CANONICAL_VAULT_PATH": (
        "Absolute path to the project-scoped vault folder (the only env the "
        "vault MCP reads). Example: /Users/you/.../vault/nnodes/proyectos/app. "
        "Must be fully resolved — do not leave nested $OTHER_VAR unexpanded."
    ),
}


def _catalog_dir() -> Path:
    home = os.environ.get("AI_SPECS_HOME")
    root = Path(home) if home else Path(__file__).resolve().parents[2]
    return root / "catalog" / "recipes"


def harness_env_path(project_root: Path) -> Path:
    return project_root / HARNESS_ENV_NAME


def harness_env_example_path(project_root: Path) -> Path:
    return project_root / HARNESS_ENV_EXAMPLE_NAME


def collect_env_vars(project_root: Path) -> dict[str, str]:
    """Collect $VAR references from enabled recipes' MCP env tables.

    Returns {VAR_NAME: purpose}. First declaration wins for purpose text.
    """
    manifest = project_root / "ai-specs" / "ai-specs.toml"
    if not manifest.is_file():
        return {}
    try:
        data = _toml_read.load_toml(manifest)
        recipes = _toml_read.read_recipes(data)
    except Exception:
        return {}

    catalog = _catalog_dir()
    collected: dict[str, str] = {}
    for recipe_id, entry in recipes.items():
        if not entry.get("enabled"):
            continue
        try:
            recipe = _recipe_read.read_recipe(catalog, recipe_id)
        except Exception:
            continue
        for preset in recipe.mcp:
            env = preset.config.get("env")
            if not isinstance(env, dict):
                continue
            for _key, value in env.items():
                if not isinstance(value, str):
                    continue
                match = ENV_REFERENCE_RE.match(value.strip())
                if not match:
                    continue
                var = match.group(1)
                purpose = f"required by {preset.id} ({recipe_id})"
                if var not in collected:
                    collected[var] = purpose
                else:
                    if recipe_id not in collected[var]:
                        collected[var] = (
                            f"{collected[var]}; also {preset.id} ({recipe_id})"
                        )
    return collected


def _unquote(raw: str) -> str:
    raw = raw.strip()
    if len(raw) >= 2 and raw[0] == raw[-1] and raw[0] in ('"', "'"):
        inner = raw[1:-1]
        return inner.replace('\\"', '"').replace("\\\\", "\\")
    # strip inline comments for unquoted values
    if "#" in raw:
        raw = raw.split("#", 1)[0].rstrip()
    return raw


def _quote_dotenv(value: str) -> str:
    if value == "" or any(c in value for c in ' \t\n"\'#$&*|\\'):
        escaped = value.replace("\\", "\\\\").replace('"', '\\"')
        return f'"{escaped}"'
    return value


def _parse_dotenv(text: str) -> dict[str, str]:
    out: dict[str, str] = {}
    for line in text.splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        match = _DOTENV_RE.match(line)
        if not match:
            continue
        out[match.group(1)] = _unquote(match.group(2))
    return out


def _parse_exports(text: str) -> dict[str, str]:
    out: dict[str, str] = {}
    for line in text.splitlines():
        match = _EXPORT_RE.match(line)
        if not match:
            continue
        out[match.group(1)] = _unquote(match.group(2))
    return out


def _format_dotenv(values: dict[str, str], *, header_lines: list[str]) -> str:
    lines = list(header_lines) + [""]
    if values:
        for var in sorted(values):
            lines.append(f"{var}={_quote_dotenv(values[var])}")
    else:
        lines.append("# (no env vars required by enabled recipes)")
    lines.append("")
    return "\n".join(lines)


def load_harness_env(project_root: Path) -> dict[str, str]:
    """Return KEY→value from ai-specs.env (empty dict if missing)."""
    path = harness_env_path(project_root)
    if not path.is_file():
        return {}
    return _parse_dotenv(path.read_text(encoding="utf-8"))


def write_env(project_root: Path, var_values: dict[str, str]) -> Path:
    """Write/merge project-root ai-specs.env. Never touches project-root .env.

    Blank/whitespace values are omitted (same as config_wizard) so a re-prompt
    with empty Enter cannot wipe existing non-empty harness secrets.
    """
    target = harness_env_path(project_root)
    existing: dict[str, str] = {}
    if target.is_file():
        existing = _parse_dotenv(target.read_text(encoding="utf-8"))
    merged = dict(existing)
    for key, value in var_values.items():
        if not str(value).strip():
            continue
        merged[key] = value
    header = [
        "# ai-specs.env — generated by ai-specs configure-recipes",
        "# gitignored. Regenerate values via: ai-specs configure-recipes",
    ]
    target.write_text(_format_dotenv(merged, header_lines=header), encoding="utf-8")
    return target


def _write_deprecation_stub(path: Path, body: str) -> None:
    if path.is_file():
        bak = path.with_name(path.name + ".bak")
        if not bak.is_file():
            bak.write_text(path.read_text(encoding="utf-8"), encoding="utf-8")
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(body, encoding="utf-8")


def generate_env_example(project_root: Path) -> Path:
    """Write root ai-specs.env.example; stub deprecated under-ai-specs templates."""
    target = harness_env_example_path(project_root)
    if target.is_file():
        backup = project_root / f"{HARNESS_ENV_EXAMPLE_NAME}.bak"
        backup.write_text(target.read_text(encoding="utf-8"), encoding="utf-8")

    vars_map = collect_env_vars(project_root)
    lines = [_ENV_EXAMPLE_HEADER.rstrip(), ""]
    if vars_map:
        for var in sorted(vars_map):
            help_bits = [vars_map[var]]
            if var in ENV_VAR_HELP:
                help_bits.append(ENV_VAR_HELP[var])
            lines.append(f"{var}=  # {'; '.join(help_bits)}")
    else:
        lines.append("# (no env vars required by enabled recipes)")
    lines.append("")
    target.write_text("\n".join(lines), encoding="utf-8")

    ai_specs = project_root / "ai-specs"
    _write_deprecation_stub(ai_specs / ".env.example", _LEGACY_ENV_EXAMPLE_STUB)
    _write_deprecation_stub(ai_specs / ".envrc.example", _ENVRC_EXAMPLE_STUB)
    return target


def generate_envrc_example(project_root: Path) -> Path:
    """Deprecated alias — writes ai-specs.env.example (and legacy stubs)."""
    return generate_env_example(project_root)


def managed_block_text() -> str:
    return f"{MANAGED_START}\n{MANAGED_BODY}\n{MANAGED_END}"


def has_managed_block(text: str) -> bool:
    return MANAGED_START in text and MANAGED_END in text


def ensure_root_envrc(project_root: Path) -> Path:
    """Ensure project-root .envrc contains the merge-safe managed block."""
    target = project_root / ".envrc"
    block = managed_block_text()
    if not target.is_file():
        header = (
            "# .envrc — direnv entry for this project\n"
            "# Harness block managed by ai-specs; safe to add custom lines outside it.\n"
        )
        target.write_text(header + "\n" + block + "\n", encoding="utf-8")
        return target

    text = target.read_text(encoding="utf-8")
    if has_managed_block(text):
        pattern = re.compile(
            re.escape(MANAGED_START) + r".*?" + re.escape(MANAGED_END),
            re.DOTALL,
        )
        new_text = pattern.sub(block, text, count=1)
        if new_text != text:
            target.write_text(
                new_text if new_text.endswith("\n") else new_text + "\n",
                encoding="utf-8",
            )
        return target

    sep = "" if text.endswith("\n") else "\n"
    target.write_text(text + sep + "\n" + block + "\n", encoding="utf-8")
    return target


def _merge_into_harness_env(
    project_root: Path,
    incoming: dict[str, str],
    *,
    existing_override: dict[str, str] | None = None,
) -> dict[str, str]:
    """Merge incoming keys into harness env; non-empty existing values win."""
    existing = (
        existing_override
        if existing_override is not None
        else load_harness_env(project_root)
    )
    write_values = dict(existing)
    for key, value in incoming.items():
        if key not in write_values or write_values[key] == "":
            write_values[key] = value
    return write_values


def migrate_nested_harness_env(project_root: Path) -> bool:
    """Migrate ai-specs/.env dotenv into root ai-specs.env. Returns True if migrated."""
    nested = project_root / "ai-specs" / ".env"
    if not nested.is_file():
        return False
    parsed = _parse_dotenv(nested.read_text(encoding="utf-8"))
    existing = load_harness_env(project_root)
    write_values = _merge_into_harness_env(
        project_root, parsed, existing_override=existing
    )
    if write_values or not harness_env_path(project_root).is_file():
        write_env(project_root, write_values)
    bak = project_root / "ai-specs" / ".env.bak"
    if bak.exists():
        bak.unlink()
    nested.rename(bak)
    ensure_root_envrc(project_root)
    return True


def migrate_legacy_envrc(project_root: Path) -> bool:
    """Migrate ai-specs/.envrc exports into root ai-specs.env. Returns True if migrated."""
    legacy = project_root / "ai-specs" / ".envrc"
    if not legacy.is_file():
        return False
    parsed = _parse_exports(legacy.read_text(encoding="utf-8"))
    existing = load_harness_env(project_root)
    write_values = _merge_into_harness_env(
        project_root, parsed, existing_override=existing
    )
    if write_values or not harness_env_path(project_root).is_file():
        write_env(project_root, write_values)
    bak = project_root / "ai-specs" / ".envrc.bak"
    if bak.exists():
        bak.unlink()
    legacy.rename(bak)
    ensure_root_envrc(project_root)
    return True


def migrate_legacy_harness_env(project_root: Path) -> bool:
    """Run nested .env + legacy .envrc migrations. Returns True if any ran."""
    ran = False
    # Nested dotenv first so export migration merges on top of gaps only.
    if migrate_nested_harness_env(project_root):
        ran = True
    if migrate_legacy_envrc(project_root):
        ran = True
    return ran


def _is_secret_var(var: str) -> bool:
    upper = var.upper()
    return any(kw in upper for kw in ["API_KEY", "TOKEN", "SECRET", "PASSWORD", "APIKEY"])


def prompt_env_vars(project_root: Path) -> dict[str, str] | None:
    """Prompt interactively for each MCP env var value.

    Returns {VAR: value} or None if cancelled.
    """
    vars_map = collect_env_vars(project_root)
    if not vars_map:
        return {}

    import questionary
    from rich.console import Console

    console = Console()

    console.print()
    console.print("[bold]Variables de entorno requeridas[/bold]")
    for var, purpose in vars_map.items():
        console.print(f"  [yellow]{var}[/yellow] — {purpose}")
        if var in ENV_VAR_HELP:
            console.print(f"    [dim]ℹ️  {ENV_VAR_HELP[var]}[/]")
    console.print()

    if not questionary.confirm("¿Configurar ahora los valores?", default=True).ask():
        return None

    result: dict[str, str] = {}
    for var in sorted(vars_map):
        if var in ENV_VAR_HELP:
            console.print(f"[dim]ℹ️  {ENV_VAR_HELP[var]}[/]")
        if _is_secret_var(var):
            value = questionary.password(var, instruction="(input oculto)").ask()
        else:
            value = questionary.text(var).ask()
        if value is None:
            return None
        result[var] = value
    return result


def direnv_allow(project_root: Path) -> bool:
    """Run direnv allow for the project root. Returns True if successful."""
    import subprocess

    try:
        result = subprocess.run(
            ["direnv", "allow", str(project_root)],
            capture_output=True,
            text=True,
            timeout=10,
        )
        return result.returncode == 0
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return False


def write_envrc(project_root: Path, var_values: dict[str, str]) -> Path:
    """Deprecated: write harness values to ai-specs.env and ensure root .envrc."""
    path = write_env(project_root, var_values)
    ensure_root_envrc(project_root)
    return path


def offer_harness_env(project_root: Path, *, offer_direnv_install: bool = True) -> None:
    """Migrate, prompt, write ai-specs.env, example, root .envrc, direnv allow. Soft-fails."""
    from rich.console import Console

    console = Console()
    try:
        migrate_legacy_harness_env(project_root)
    except Exception as exc:  # noqa: BLE001
        console.print(f"[yellow]Legacy harness env migration skipped: {exc}[/yellow]")

    try:
        vars_map = collect_env_vars(project_root)
    except Exception:
        return
    if not vars_map:
        return

    try:
        values = prompt_env_vars(project_root)
    except Exception as exc:  # noqa: BLE001
        console.print(f"[yellow]No se pudieron configurar variables de entorno: {exc}[/yellow]")
        return
    if values is None:
        return

    try:
        if values:
            path = write_env(project_root, values)
            console.print(f"[green]✓[/green] escrito {path}")
        generate_env_example(project_root)
        ensure_root_envrc(project_root)
        console.print(f"[green]✓[/green] root .envrc managed block → {project_root / '.envrc'}")
    except Exception as exc:  # noqa: BLE001
        console.print(f"[yellow]No se pudo escribir harness env: {exc}[/yellow]")
        return

    import shutil

    if offer_direnv_install and shutil.which("direnv") is None:
        try:
            dep_install = _load_sibling("dep_install")
            if sys.stdin.isatty() and sys.stdout.isatty():
                plan = dep_install.resolve_install_plan(
                    "direnv",
                    install_url="https://direnv.net/docs/installation.html",
                )
                dep_install.offer_and_install([plan], tty=True)
        except Exception:
            pass

    try:
        if not direnv_allow(project_root):
            print("  ! direnv no está instalado o no se pudo ejecutar.", file=sys.stderr)
            print("    Instalalo con: brew install direnv", file=sys.stderr)
            print("    Despues corre: direnv allow", file=sys.stderr)
        else:
            print("  ✓ direnv allow — las variables quedan activas en esta terminal")
    except Exception as exc:  # noqa: BLE001
        console.print(f"[yellow]direnv allow falló: {exc}[/yellow]")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Generate ai-specs.env.example")
    parser.add_argument("path", nargs="?", default=".", help="Project root")
    args = parser.parse_args(argv)
    root = Path(args.path).resolve()
    if not (root / "ai-specs" / "ai-specs.toml").is_file():
        print(
            f"Proyecto no inicializado: missing ai-specs/ai-specs.toml under {root}",
            file=sys.stderr,
        )
        return 1
    path = generate_env_example(root)
    ensure_root_envrc(root)
    print(f"Wrote {path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
