#!/usr/bin/env python3
"""Tracker-domain ledger host: acquisition + JSON bridge to the Go predicate.

This module owns the tracker-ledger checkpoint lifecycle (``pre-merge`` and
``archive-close``). It is deliberately independent of Plan Build/OpenSpec: a
project with a bound tracker recipe can grade its checkpoints without an
``openspec/`` tree, and the Plan Build artifact guardian never invokes it. The
CLI names the wire checkpoint directly (``--checkpoint pre-merge|archive-close``)
and keeps ``--stage pre-merge|pre-archive`` as a compatibility alias.

``archive-close`` is *tracker item closure*, never an OpenSpec archive: no
planning tree is required, and an existing ``openspec/changes/archive/`` entry is
never read to infer or select a tracker checkpoint.

The Go ``--ledger`` predicate stays the single authoritative grader. Everything
here is acquisition and transport: resolve the effective ledger mode, locate the
verified gate binary, build the evidence file from local facts, invoke the
binary, and translate its verdict into blockers. Fail-open is preserved: a
missing binary, evidence, or verdict yields no blocker rather than an exception.

The domain is Tracker, not a provider. A provider recipe extends it by declaring a
``[config.reconcile]`` adapter mapping (native value -> neutral property) that the
Go core reads from the project manifest; ``ledger_mode`` / ``gate_mode`` stay
Tracker-domain policy. Nothing here names a provider, grades, or writes tracker
state.

No tracker state is written here and no ``gh``/MCP/network surface exists.
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path


def resolve_ledger_mode(root: Path | str, gate_hint: str = "") -> str:
    """Map project config to the effective ledger mode (A9).

    ``ledger_mode`` wins whenever it is set; otherwise the legacy tracker
    ``gate_mode`` maps ``off``→skip, ``warn``→``warn``, ``always``→``always``.
    The worktree gate mode is never read. ``TRACKER_LEDGER_MODE`` overrides both.
    """
    env_mode = os.environ.get("TRACKER_LEDGER_MODE", "")
    ledger = gate = ""
    bridge = _ledger_bridge()
    if bridge is not None:
        try:
            import tomllib
            manifest = Path(root) / "ai-specs" / "ai-specs.toml"
            data = tomllib.loads(manifest.read_text(encoding="utf-8"))
            # The bound recipe id comes from the durable witness, never from a
            # hardcoded literal (task 3.2). Reading the witness is acquisition.
            recipe = bridge.recipe_id(root)
            cfg = ((data.get("recipes") or {}).get(recipe) or {}).get("config") or {}
            ledger = str(cfg.get("ledger_mode") or "")
            gate = str(cfg.get("gate_mode") or "")
        except Exception:
            pass
    if gate_hint in ("off", "warn", "always") and not gate:
        # The bound recipe's own gate_mode wins; the caller's legacy hint fills in
        # when that config section declares none (the pre-witness behavior).
        gate = gate_hint
    if env_mode in ("always", "ask", "warn"):
        return env_mode
    if ledger in ("always", "ask", "warn"):
        return ledger
    if gate == "off":
        return "off"
    if gate == "always":
        return "always"
    return "warn"


def _ledger_binary() -> Path | None:
    """Resolve a verified worktree-gate binary, or ``None`` (fail open).

    ``WORKTREE_GATE_BIN`` is the debugging/test pin; otherwise the version-keyed
    cache layout from the sibling ``gate_binary`` helper is reused, and the
    executable is accepted only with its ``.verified`` receipt. A cold
    ``AI_SPECS_HOME`` therefore fails open rather than blocking a merge.
    """
    override = os.environ.get("WORKTREE_GATE_BIN", "")
    if override:
        candidate = Path(override)
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return candidate
        return None
    home = Path(os.environ.get("AI_SPECS_HOME") or (Path.home() / ".ai-specs"))
    try:
        import importlib.util
        spec = importlib.util.spec_from_file_location(
            "gate_binary_guardian", Path(__file__).with_name("gate_binary.py")
        )
        if spec is None or spec.loader is None:
            return None
        module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = module
        spec.loader.exec_module(module)
        goos, goarch = module.detect_platform()
        candidate = module.cache_bin_path(home, goos=goos, goarch=goarch)
    except Exception:
        return None
    receipt = candidate.with_name(candidate.name + ".verified")
    if candidate.is_file() and os.access(candidate, os.X_OK) and receipt.is_file():
        return candidate
    return None


def _ledger_ask(binary: Path, checkpoint: str, mode: str, root: Path | str,
                verdict: dict) -> list[str]:
    """Prompt for a checkpoint-scoped opt-out, then persist it via ``--decide``."""
    prompt = verdict.get("prompt") or {}
    evidence = prompt.get("evidence") or {}
    for side in ("local", "remote", "code", "git"):
        print(f"  {side}: {evidence.get(side) or '(unavailable)'}", file=sys.stderr)
    print(
        "  choices: " + ", ".join(prompt.get("choices") or []),
        file=sys.stderr,
    )
    try:
        with open("/dev/tty") as tty:
            print(f"opt out of the {checkpoint} checkpoint? [y/N] ", end="",
                  file=sys.stderr, flush=True)
            answer = tty.readline().strip()
    except OSError:
        print(
            f"tracker-ledger-host: tracker-ledger {checkpoint} needs a decision but "
            "no terminal is available; no opt-out was recorded",
            file=sys.stderr,
        )
        return [f"tracker-ledger {checkpoint}: no opt-out recorded (no terminal available)"]
    if answer.lower() not in ("y", "yes"):
        return [f"tracker-ledger {checkpoint}: no opt-out recorded"]
    payload = json.dumps(
        {"checkpoint": checkpoint, "kind": "opt-out", "choice": "continue"}
    )
    try:
        proc = subprocess.run(
            [str(binary), "--ledger", "--checkpoint", checkpoint,
             "--ledger-mode", mode, "--project-root", str(root), "--decide", payload],
            capture_output=True, text=True, timeout=30,
        )
    except Exception:
        return [f"tracker-ledger {checkpoint}: failed to record opt-out"]
    if proc.returncode == 2:
        return [f"tracker-ledger {checkpoint}: failed to record opt-out"]
    return []


def _ledger_bridge():
    """Sibling-load ``lib/_internal/ledger_bridge.py`` (cold-cache safe), or None.

    The bridge adds no project-cache or ``AI_SPECS_HOME`` dependency: it is loaded
    from beside this module exactly like ``gate_binary.py``, so a cold CLI install
    still acquires evidence.
    """
    try:
        import importlib.util
        spec = importlib.util.spec_from_file_location(
            "ledger_bridge_guardian", Path(__file__).with_name("ledger_bridge.py")
        )
        if spec is None or spec.loader is None:
            return None
        module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = module
        spec.loader.exec_module(module)
        return module
    except Exception:
        return None


def _ledger_evidence_args(root: Path | str, slug: str | None) -> tuple[list[str], str | None]:
    """Build the ``--evidence`` file from the bridge's local facts.

    Returns ``(extra_argv, cleanup_path)``. Acquisition only: the host never
    derives a ledger write from a planning-tree artifact (R1), so a tracker.none file
    contributes nothing here but the blank evidence side it already implies. The file
    is never created, modified, or deleted.
    """
    bridge = _ledger_bridge()
    if bridge is None:
        return [], None
    resolved_slug = slug or bridge.change_slug(root)
    try:
        fd, evidence_path = tempfile.mkstemp(prefix="tracker-ledger-evidence-", suffix=".json")
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            json.dump(bridge.evidence_payload(root, resolved_slug), handle)
    except Exception:
        return [], None
    return ["--evidence", evidence_path], evidence_path


def ledger_blockers(root: Path | str, checkpoint: str, slug: str | None = None) -> list[str]:
    """Grade one ledger checkpoint through the verified binary (JSON bridge).

    ``slug`` is the change being graded when the caller knows it (archive-close and
    pre-merge do); otherwise the single active change is used, and an ambiguous
    planning tree simply contributes no evidence.
    """
    mode = resolve_ledger_mode(root)
    if mode == "off":
        return []
    binary = _ledger_binary()
    if binary is None:
        print(
            "tracker-ledger-host: tracker-ledger: no verified gate binary resolved; "
            "failing open (run ai-specs sync / ai-specs doctor)",
            file=sys.stderr,
        )
        return []
    extra, evidence_path = _ledger_evidence_args(root, slug)
    try:
        proc = subprocess.run(
            [str(binary), "--ledger", "--checkpoint", checkpoint,
             "--ledger-mode", mode, "--project-root", str(root), *extra],
            capture_output=True, text=True, timeout=30,
        )
    except Exception:
        return []
    finally:
        if evidence_path:
            try:
                os.unlink(evidence_path)
            except OSError:
                pass
    if not proc.stdout.strip():
        return []
    try:
        verdict = json.loads(proc.stdout)
    except Exception:
        return []
    decision = str(verdict.get("decision") or "")
    reason = str(verdict.get("reason") or "")
    if proc.returncode == 2 or decision == "block":
        return [f"tracker-ledger {checkpoint}: blocked — {reason or 'missing tracked item'}"]
    if decision == "ask":
        if reason == "identity_unavailable":
            # No identity means no durable key for a checkpoint-scoped answer
            # (A2/A5), so ask cannot collect a recordable decision here: report
            # and proceed without inferring one (the spec's non-blocking rule).
            print(
                f"tracker-ledger-host: tracker-ledger {checkpoint}: identity_unavailable; "
                "reporting and proceeding without recording a decision.",
                file=sys.stderr,
            )
            return []
        return _ledger_ask(binary, checkpoint, mode, root, verdict)
    if decision == "unevaluable":
        print(
            f"tracker-ledger-host: tracker-ledger {checkpoint}: {reason or 'unevaluable'}; "
            "failing open",
            file=sys.stderr,
        )
    return []


def _resolve_checkpoint(parser: argparse.ArgumentParser, checkpoint: str | None,
                        stage: str | None) -> str:
    """Resolve the wire checkpoint from the direct surface or the legacy alias.

    ``--checkpoint pre-merge|archive-close`` is the direct lifecycle surface;
    ``--stage pre-merge|pre-archive`` remains the compatibility alias. The two
    must not disagree: a contradictory request grades nothing.
    """
    from_stage = None
    if stage is not None:
        from_stage = "archive-close" if stage == "pre-archive" else "pre-merge"
    if checkpoint is not None and from_stage is not None and checkpoint != from_stage:
        parser.error(f"--checkpoint {checkpoint} conflicts with --stage {stage}")
    return checkpoint or from_stage or "pre-merge"


def main(argv: list[str] | None = None) -> int:
    """Grade one tracker-ledger checkpoint without Plan Build or OpenSpec.

    ``--checkpoint pre-merge|archive-close`` names the wire checkpoint directly.
    ``archive-close`` is tracker item closure, not an OpenSpec archive: no
    ``openspec/`` tree is required and archive state never selects the
    checkpoint. ``--stage pre-merge|pre-archive`` stays as a compatibility alias
    (``pre-archive`` maps to ``archive-close``). ``--root`` is required; the slug
    is optional, because a tracker-only project has no change tree and the
    binding witness supplies the identity.
    """
    parser = argparse.ArgumentParser(
        description="Grade a tracker-ledger checkpoint (acquisition/JSON host)"
    )
    parser.add_argument(
        "slug", nargs="?", default=None,
        help="change slug (default: the single active change, or none)",
    )
    parser.add_argument(
        "--root", type=Path, required=True,
        help="resolved project root (never the process cwd)",
    )
    parser.add_argument(
        "--checkpoint", choices=["pre-merge", "archive-close"], default=None,
        help="tracker-ledger wire checkpoint (default: pre-merge)",
    )
    parser.add_argument(
        "--stage", choices=["pre-merge", "pre-archive"], default=None,
        help="compatibility alias for --checkpoint (pre-archive -> archive-close)",
    )
    args = parser.parse_args(argv)
    checkpoint = _resolve_checkpoint(parser, args.checkpoint, args.stage)
    blockers = ledger_blockers(args.root, checkpoint, args.slug)
    if blockers:
        print("tracker-ledger-host: BLOCKED", file=sys.stderr)
        for blocker in blockers:
            print(f"  - {blocker}", file=sys.stderr)
        return 1
    print(f"tracker-ledger-host: OK ({checkpoint})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
