"""Reproducible project sandboxes for the plan-build behavior goldens.

Every case gets its own temporary directory holding a real git repository with a
`development` branch, an isolated `HOME` (so the global install and the lock are
never touched) and an `ai-specs/ai-specs.toml` manifest that enables the recipe
in the mode the case asks for. The binary under test is the Go CLI built once
from this checkout by `tests/_go_cli.py`; nothing here talks to the network or
to a model.

Captured evidence per step is the exit code, stdout, stderr and the resulting
tree. Paths, dates and commit ids are normalized so a golden compares equal on
macOS and Linux.
"""
from __future__ import annotations

import hashlib
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))

from tests._go_cli import cli  # noqa: E402

# A sandbox tree is the whole project except the git object store.
_SKIP_DIRS = {".git"}

_DATE = re.compile(r"\b\d{4}-\d{2}-\d{2}\b")
_TIMESTAMP = re.compile(r"\b\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z\b")
_SHA = re.compile(r"\b[0-9a-f]{40}\b")

# The binary resolves its assets from the install root, exactly like a real
# install (~/.ai-specs/dist/ai-specs beside templates/, catalog/, ...). A binary
# built straight into a bare temp dir would look for assets that are not there.
ASSET_DIRS = ("templates", "catalog", "bundled-skills", "bundled-commands")
_install: Path | None = None


def install_root() -> Path:
    """Build the throwaway install root once per process and return it."""
    global _install
    if _install is not None:
        return _install
    root = Path(tempfile.mkdtemp(prefix="ai-specs-behavior-install-"))
    dist = root / "dist"
    dist.mkdir()
    shutil.copy2(cli(), dist / "ai-specs")
    (dist / "ai-specs").chmod(0o755)
    for name in ASSET_DIRS:
        source = ROOT / name
        if source.is_dir():
            shutil.copytree(source, root / name)
    _install = root
    return root


def installed_cli() -> Path:
    return install_root() / "dist" / "ai-specs"


def normalize(text: str, root: Path) -> str:
    """Make a sandbox path, install path, date or commit id portable."""
    # The generated hook command bakes in the install root the CLI resolved from,
    # so it has to fold to a placeholder along with the sandbox root.
    if _install is not None:
        text = text.replace(f"/private{_install}", "<INSTALL>")
        text = text.replace(str(_install), "<INSTALL>")
    text = text.replace(f"/private{root}", str(root))
    text = text.replace(str(root), "<ROOT>")
    text = text.replace("/private<ROOT>", "<ROOT>")
    text = _TIMESTAMP.sub("<TIMESTAMP>", text)
    text = _SHA.sub("<SHA>", text)
    text = _DATE.sub("<DATE>", text)
    return text


def _digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


class Sandbox:
    """One isolated project the cases drive through the compiled CLI."""

    def __init__(self, name: str, mode: str = "openspec", base: Path | None = None):
        self.name = name
        self.mode = mode
        holder = Path(tempfile.mkdtemp(prefix=f"ai-specs-behavior-{name}-", dir=base))
        self.holder = holder
        self.root = holder / "repo"
        self.home = holder / "home"
        self.root.mkdir()
        self.home.mkdir()
        self.steps: list[dict] = []
        self._git_init()

    # -- lifecycle ---------------------------------------------------------
    def _git_init(self) -> None:
        self.git("init", "-q", "-b", "development")
        self.git("config", "user.email", "eval@example.invalid")
        self.git("config", "user.name", "behavior golden")
        self.git("config", "commit.gpgsign", "false")
        self.write("README.md", "# fixture\n")
        self.commit("initial")

    def cleanup(self) -> None:
        shutil.rmtree(self.holder, ignore_errors=True)

    # -- environment -------------------------------------------------------
    def env(self) -> dict[str, str]:
        return {
            **os.environ,
            "HOME": str(self.home),
            "AI_SPECS_HOME": str(install_root()),
            "AI_SPECS_GATE_OFFLINE": "1",
            "GIT_CONFIG_NOSYSTEM": "1",
            "TZ": "UTC",
            "LANG": "C",
        }

    # -- files -------------------------------------------------------------
    def write(self, rel: str, content: str) -> Path:
        path = self.root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")
        return path

    def manifest(self, extra: str = "") -> None:
        """Enable the recipe in this project, in the requested mode."""
        self.write(
            "ai-specs/ai-specs.toml",
            "[project]\n"
            f'name = "{self.name}"\n'
            "\n"
            "[agents]\n"
            'enabled = ["claude"]\n'
            "\n"
            "[recipes.plan-build-flow]\n"
            "enabled = true\n"
            "\n"
            "[recipes.plan-build-flow.config]\n"
            f'mode = "{self.mode}"\n'
            f"{extra}",
        )

    # -- git ---------------------------------------------------------------
    def git(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["git", "-C", str(self.root), *args],
            env=self.env(),
            text=True,
            capture_output=True,
            check=False,
        )

    def commit(self, message: str) -> str:
        self.git("add", "-A")
        self.git("commit", "-q", "-m", message)
        out = self.git("rev-parse", "HEAD")
        return out.stdout.strip()

    def branch(self, name: str) -> None:
        self.git("checkout", "-q", "-b", name)

    # -- the binary under test --------------------------------------------
    def run(self, *argv: str, stdin: str | None = None, label: str | None = None) -> dict:
        """Run the compiled CLI inside the sandbox and record the evidence."""
        proc = subprocess.run(
            [str(installed_cli()), *argv],
            cwd=str(self.root),
            env=self.env(),
            input=stdin,
            text=True,
            capture_output=True,
            check=False,
            timeout=180,
        )
        step = {
            "label": label or " ".join(argv),
            "argv": ["<CLI>", *argv],
            "exit": proc.returncode,
            "stdout": normalize(proc.stdout, self.root),
            "stderr": normalize(proc.stderr, self.root),
        }
        self.steps.append(step)
        return step

    def run_stdin(self, argv: list[str], payload: str, label: str) -> dict:
        """Run the CLI feeding a pre-tool-use hook payload on stdin."""
        return self.run(*argv, stdin=payload, label=label)

    # -- resulting tree ----------------------------------------------------
    def tree(self) -> dict[str, dict]:
        entries: dict[str, dict] = {}
        for path in sorted(self.root.rglob("*")):
            rel = path.relative_to(self.root)
            if any(part in _SKIP_DIRS for part in rel.parts):
                continue
            if path.is_dir():
                continue
            body = path.read_bytes()
            try:
                text = body.decode("utf-8")
            except UnicodeDecodeError:
                entries[str(rel)] = {"sha256": hashlib.sha256(body).hexdigest()}
                continue
            entries[str(rel)] = {"sha256": hashlib.sha256(normalize(text, self.root).encode()).hexdigest()}
        return entries

    def contents(self, *globs: str) -> dict[str, str]:
        """Normalized bodies of the files a case wants legible in its golden."""
        bodies: dict[str, str] = {}
        for pattern in globs:
            for path in sorted(self.root.glob(pattern)):
                if not path.is_file():
                    continue
                rel = str(path.relative_to(self.root))
                try:
                    bodies[rel] = normalize(path.read_text(encoding="utf-8"), self.root)
                except UnicodeDecodeError:
                    bodies[rel] = "<binary>"
        return bodies

    def snapshot(self, files: dict[str, str] | None = None) -> dict:
        snap = {"case": self.name, "mode": self.mode, "steps": self.steps, "tree": self.tree()}
        if files:
            snap["files"] = files
        return snap
