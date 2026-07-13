#!/usr/bin/env python3
"""Interactive front door for bare `ai-specs` (status + command menu).

Import-time contract: pure stdlib. rich/questionary are imported lazily only
after the deps gate passes. Sibling modules (util, doctor) load via absolute path.
"""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
from dataclasses import dataclass
from enum import Enum
from pathlib import Path


def _load_sibling(name: str):
    """Load a same-directory _internal module by absolute path (sys.path-independent)."""
    import importlib.util

    path = Path(__file__).with_name(f"{name}.py")
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load sibling module {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


_util = _load_sibling("util")
_doctor = _load_sibling("doctor")


class Mode(Enum):
    INTERACTIVE_HUB = "interactive-hub"
    NONINTERACTIVE_STATUS = "noninteractive-status"
    OFFER_INIT = "offer-init"
    ERROR_UNINITIALIZED = "error-uninitialized"


def decide_mode(*, initialized: bool, tty: bool) -> Mode:
    if initialized:
        return Mode.INTERACTIVE_HUB if tty else Mode.NONINTERACTIVE_STATUS
    return Mode.OFFER_INIT if tty else Mode.ERROR_UNINITIALIZED


class Action(Enum):
    SYNC = "sync"
    DOCTOR = "doctor"
    SKILLS = "skills"
    RECIPES = "recipe"
    CONFIGURE_RECIPES = "configure-recipes"
    RULES_AUDIT = "rules-audit"
    UPGRADE = "upgrade"
    VERSION = "version"
    HELP = "help"
    INIT = "init"
    QUIT = "quit"
_MENU: list[tuple[Action, str, str]] = [
    (Action.SYNC, "Sync", "Reconcile manifest → bundled + vendor + AGENTS.md + agents"),
    (Action.DOCTOR, "Doctor", "Full project health report (read-only)"),
    (Action.SKILLS, "Skills", "List / add / remove vendored skills"),
    (Action.RECIPES, "Recipes", "List / add / remove / configure catalog recipes"),
    (Action.RULES_AUDIT, "Rules audit", "Inventory legacy rules for migration"),
    (Action.UPGRADE, "Upgrade", "Upgrade the global ai-specs installation"),
    (Action.VERSION, "Version", "Print the CLI version"),
    (Action.HELP, "Help", "Show ai-specs command help"),
    (Action.INIT, "Init wizard", "Re-run interactive onboarding"),
    (Action.QUIT, "Quit", "Exit the hub"),
]
@dataclass(frozen=True)
class StatusSummary:
    root: Path
    ok: int
    info: int
    warn: int
    error: int
    exit_code: int
    headline: str
    checks: list


def status_summary(root: Path) -> StatusSummary:
    doc = _doctor.Doctor(root)
    exit_code = doc.run()
    Sev = _doctor.Severity
    counts = {s: sum(1 for c in doc.checks if c.severity == s) for s in Sev}
    error = counts[Sev.ERROR]
    warn = counts[Sev.WARN]
    if error:
        headline = f"{error} error(s)"
    elif warn:
        headline = f"{warn} warning{'s' if warn != 1 else ''}"
    else:
        headline = "healthy"
    return StatusSummary(
        root=root,
        ok=counts[Sev.OK],
        info=counts[Sev.INFO],
        warn=warn,
        error=error,
        exit_code=exit_code,
        headline=headline,
        checks=list(doc.checks),
    )


def _print_uninit_error(target: Path) -> None:
    print(f"ERROR: no ai-specs project at {target}", file=sys.stderr)
    print("Run 'ai-specs init' to create one.", file=sys.stderr)


def _run_noninteractive(target: Path) -> int:
    summary = status_summary(target)
    print(f"ai-specs status — {summary.headline}")
    print(f"  target: {summary.root}")
    print(
        f"  Summary: {summary.ok} OK, {summary.info} INFO, "
        f"{summary.warn} WARN, {summary.error} ERROR"
    )
    print()
    print("Commands:")
    for _act, title, desc in _MENU:
        print(f"  {title:12s}  {desc}")
    return 0


@dataclass
class CommandMenu:
    def prompt(self) -> Action:
        import questionary

        # questionary<2.1 has no Choice.description — embed in title.
        choices = [
            questionary.Choice(title=f"{title} — {desc}", value=act)
            for act, title, desc in _MENU
        ]
        answer = questionary.select("What do you want to do?", choices=choices).ask()
        return answer if answer is not None else Action.QUIT


@dataclass
class DelegateRunner:
    cli: Path
    target: Path

    def run(self, action: Action, extra: list[str] | None = None) -> int:
        argv = [str(self.cli), action.value]
        if extra:
            # Subcommand-based dispatchers (recipe, skills) want subcommand before target
            argv.extend(extra)
            argv.append(str(self.target))
        else:
            argv.append(str(self.target))
        return subprocess.run(argv).returncode


@dataclass
class StatusPanel:
    summary: StatusSummary

    def render(self):
        from rich.panel import Panel
        from rich.table import Table

        table = Table.grid(padding=(0, 1))
        table.add_column(style="bold")
        table.add_column()
        table.add_row("target", str(self.summary.root))
        table.add_row(
            "Summary:",
            f"{self.summary.ok} OK, {self.summary.info} INFO, "
            f"{self.summary.warn} WARN, {self.summary.error} ERROR "
            f"({self.summary.headline})",
        )
        Sev = _doctor.Severity
        for check in self.summary.checks:
            if check.severity is Sev.OK:
                continue
            table.add_row(check.severity.value, f"{check.name}: {check.message}")

        if self.summary.error:
            border = "red"
        elif self.summary.warn:
            border = "yellow"
        else:
            border = "green"
        return Panel(table, title="ai-specs", border_style=border)


def _print_version() -> None:
    version_path = _util.ai_specs_home() / "VERSION"
    if version_path.is_file():
        print(version_path.read_text(encoding="utf-8").strip())
    else:
        print("unknown")


_SUB_ARGS: dict[Action, list[str]] = {
    Action.SKILLS: ["list"],
}


def _run_interactive_hub(target: Path) -> int:
    from rich.console import Console


    console = Console()
    cli = _util.ai_specs_home() / "bin" / "ai-specs"
    runner = DelegateRunner(cli=cli, target=target)

    while True:
        summary = status_summary(target)
        console.print(StatusPanel(summary).render())
        action = CommandMenu().prompt()

        if action is Action.QUIT:
            return 0

        if action is Action.RECIPES:
            import questionary
            sub = questionary.select(
                "Recipes:",
                choices=[
                    questionary.Choice(title="List recipes", value="list"),
                    questionary.Choice(title="Add recipe", value="add"),
                    questionary.Choice(title="Remove recipe", value="remove"),
                    questionary.Choice(title="Configure recipe", value="configure"),
                    questionary.Choice(title="Back", value="back"),
                ],
            ).ask()
            if sub is None or sub == "back":
                continue
            if sub == "list":
                rc = runner.run(Action.RECIPES, extra=["list"])
            elif sub == "add":
                recipe_id = questionary.text(
                    "Recipe id:",
                    instruction="(e.g. trello-mcp-workflow, git-pr-flow)",
                ).ask()
                if not recipe_id:
                    continue
                rc = runner.run(Action.RECIPES, extra=["add", recipe_id])
            elif sub == "remove":
                recipe_id = questionary.text(
                    "Recipe id to remove:",
                    instruction="(e.g. git-pr-flow, trello-mcp-workflow)",
                ).ask()
                if not recipe_id:
                    continue
                rc = runner.run(Action.RECIPES, extra=["remove", recipe_id])
            elif sub == "configure":
                rc = runner.run(Action.CONFIGURE_RECIPES)
            else:
                continue
            print("✓ done" if rc == 0 else f"✗ exited {rc}")
            try:
                input("Press Enter to return…")
            except EOFError:
                return 0
            continue
        if action is Action.VERSION:
            _print_version()
            continue

        _extra = _SUB_ARGS.get(action, [])
        rc = runner.run(action, extra=_extra)
        if rc == 0:
            print("✓ done")
        else:
            print(f"✗ exited {rc}")
        try:
            input("Press Enter to return…")
        except EOFError:
            return 0


def _offer_init(target: Path) -> bool:
    import questionary

    print(f"No ai-specs project found at {target}.")
    answer = questionary.confirm("Run the init wizard now?", default=True).ask()
    if not answer:
        return False
    cli = _util.ai_specs_home() / "bin" / "ai-specs"
    rc = DelegateRunner(cli=cli, target=target).run(Action.INIT)
    if rc != 0 or not _util.is_initialized(target):
        return False
    # After a successful init, auto-run sync so the hub shows a clean status
    # instead of doctor errors about missing agent configs.
    print("✓ init complete — running sync to generate agent configs…")
    rc = DelegateRunner(cli=cli, target=target).run(Action.SYNC)
    return rc == 0


def _parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="ai-specs hub — status + command menu")
    parser.add_argument(
        "target",
        nargs="?",
        default=os.getcwd(),
        help="Target project root (default: current directory)",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    target = Path(args.target).resolve()
    if not target.is_dir():
        print(f"ERROR: target is not a directory: {target}", file=sys.stderr)
        return 2

    tty = sys.stdin.isatty() and sys.stdout.isatty()
    mode = decide_mode(initialized=_util.is_initialized(target), tty=tty)

    if mode is Mode.ERROR_UNINITIALIZED:
        _print_uninit_error(target)
        return 2
    if mode is Mode.NONINTERACTIVE_STATUS:
        return _run_noninteractive(target)

    err = _util.ensure_deps(_util.vendor_dir())
    if err is not None:
        return err

    if mode is Mode.OFFER_INIT:
        if not _offer_init(target):
            return 0

    return _run_interactive_hub(target)


if __name__ == "__main__":
    sys.exit(main())
