#!/usr/bin/env python3
"""Differential reference driver for the Go AGENTS.md write-governance port.

Card [Go 07.S3b]. Reads ONE JSON object from stdin:

    {"manifest_toml_b64": <b64 of a TOML manifest>,
     "existing_agents_b64": <b64> | null   # AGENTS.md content (null = absent)
     "lock_toml_b64": <b64> | null         # <root>/ai-specs/.ai-specs.lock
     "resolved_json_b64": <b64> | null     # resolved-config JSON
     "adopt_brief": bool,
     "preserve_flag": bool,
     "policy_validate": bool}

It materializes the real project layout in a fresh temp root

    <root>/ai-specs/ai-specs.toml
    <root>/AGENTS.md                        (only when existing_agents_b64)
    <root>/ai-specs/.ai-specs.lock          (only when lock_toml_b64)
    <root>/resolved.json                    (only when resolved_json_b64)

and drives the REAL ``lib/_internal/agents-render.py`` and the REAL
``lib/_internal/brief-render-policy.py`` (imported, not reimplemented).

The classify bridge (``util.classify_managed_override`` -> ``worktree-gate
--plan-classify``) is redirected to its documented Python fallback
(``util._python_classify_managed_override``): this slice pins the historical
Python decision the Go state machine reproduces, not a gate binary that may or
may not be present. The fallback is the exact code path util.py keeps as the
temporary authority, and the redirection also suppresses the one degraded-run
warning the bridge would otherwise emit into the render stderr.

Emits the policy leg, the render leg's observable surface (stdout/stderr/rc,
returned action, whether write_bytes ran), the raw classify state, the
effective state sync would act on, and the final AGENTS.md / lock sha256.
"""
from __future__ import annotations

import base64
import contextlib
import hashlib
import importlib.util
import io
import json
import sys
import tempfile
import traceback
from pathlib import Path

# testdata/ -> internal/sync/ -> internal/ -> repo root
REPO_ROOT = Path(__file__).resolve().parents[3]
RENDER_PATH = REPO_ROOT / "lib" / "_internal" / "agents-render.py"
POLICY_PATH = REPO_ROOT / "lib" / "_internal" / "brief-render-policy.py"

_RENDER = None
_POLICY = None


def _load(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load {path}")
    mod = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = mod
    spec.loader.exec_module(mod)
    return mod


def load_render():
    global _RENDER
    if _RENDER is None:
        _RENDER = _load(RENDER_PATH, "agents_render_briefgov_ref")
        # Pin the historical Python classify decision (see module docstring).
        util = _RENDER._load_util()
        util.classify_managed_override = util._python_classify_managed_override
        # Pin the pure-Python lock writer (the Go --write-lock bridge would
        # otherwise emit its own degraded-run warning into the render stderr).
        lock_mod = _RENDER._load_lock()
        lock_mod.write_lock = lock_mod._write_lock_python
    return _RENDER


def load_policy():
    global _POLICY
    if _POLICY is None:
        _POLICY = _load(POLICY_PATH, "brief_render_policy_ref")
    return _POLICY


def _write(path: Path, data: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)


def run_policy(toml_path: Path, validate: bool):
    """Call the real brief-render-policy.py main() with a pinned argv."""
    mod = load_policy()
    argv = [str(POLICY_PATH), str(toml_path)] + (["--validate"] if validate else [])
    out_buf, err_buf = io.StringIO(), io.StringIO()
    old_argv = sys.argv
    sys.argv = argv
    rc = 0
    try:
        with contextlib.redirect_stdout(out_buf), contextlib.redirect_stderr(err_buf):
            rc = mod.main()
    except SystemExit as exc:  # argparse errors
        rc = exc.code if isinstance(exc.code, int) else 2
    finally:
        sys.argv = old_argv
    return {"stdout": out_buf.getvalue(), "stderr": err_buf.getvalue(), "rc": rc}


def _sha256_or_null(path: Path):
    if not path.is_file():
        return None
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main() -> int:
    case = json.loads(sys.stdin.read())
    manifest_bytes = base64.b64decode(case["manifest_toml_b64"])

    with tempfile.TemporaryDirectory(prefix="briefgov-ref-") as td:
        root = Path(td)
        toml_path = root / "ai-specs" / "ai-specs.toml"
        output_path = root / "AGENTS.md"
        lock_path = root / "ai-specs" / ".ai-specs.lock"
        resolved_path = None

        _write(toml_path, manifest_bytes)
        if case.get("existing_agents_b64") is not None:
            _write(output_path, base64.b64decode(case["existing_agents_b64"]))
        if case.get("lock_toml_b64") is not None:
            _write(lock_path, base64.b64decode(case["lock_toml_b64"]))
        if case.get("resolved_json_b64") is not None:
            resolved_path = root / "resolved.json"
            _write(resolved_path, base64.b64decode(case["resolved_json_b64"]))

        policy = run_policy(toml_path, bool(case.get("policy_validate")))

        mod = load_render()

        # Reproduce render()'s resolved-config coercion for the state probe.
        import tomllib

        with open(toml_path, "rb") as fh:
            manifest = tomllib.load(fh)
        resolved = {}
        if resolved_path is not None:
            with open(resolved_path) as fh:
                data = json.load(fh)
            if not isinstance(data, dict):
                data = {}
            if not isinstance(data.get("bindings"), dict):
                data["bindings"] = {}
            if not isinstance(data.get("recipes"), dict):
                data["recipes"] = {}
            if not isinstance(data.get("enabled"), list):
                data["enabled"] = []
            resolved = data
        resolved.setdefault("project_root", str(toml_path.resolve().parent.parent))

        result = {
            "policy": policy,
            "render_stdout": "",
            "render_stderr": "",
            "render_rc": 0,
            "action": None,
            "classify_state": None,
            "effective_state": None,
            "wrote": False,
            "agents_sha256": None,
            "lock_sha256": None,
        }

        try:
            # Compute the would-write bytes for the state probe. _render_lines
            # emits the unknown-VCS warning to stderr; discard it here so the
            # render-leg stderr stays single-warning (the Go port warns once).
            with contextlib.redirect_stderr(io.StringIO()):
                content = "\n".join(mod._render_lines(manifest, resolved)).encode("utf-8")
            result["classify_state"] = mod.classify_brief(
                toml_path, output_path, content, lock_path=lock_path)
            result["effective_state"] = mod.brief_effective_state(
                toml_path, output_path, content, lock_path=lock_path)

            out_buf, err_buf = io.StringIO(), io.StringIO()
            with contextlib.redirect_stdout(out_buf), contextlib.redirect_stderr(err_buf):
                action = mod.render(
                    toml_path,
                    output_path,
                    preserve_if_marker=bool(case.get("preserve_flag")),
                    resolved_config_path=resolved_path,
                    adopt_brief=bool(case.get("adopt_brief")),
                )
            result["render_stdout"] = out_buf.getvalue()
            result["render_stderr"] = err_buf.getvalue()
            result["action"] = action
            result["wrote"] = action == "written"
        except Exception:
            result["render_stdout"] = ""
            result["render_stderr"] = traceback.format_exc()
            result["render_rc"] = 1

        result["agents_sha256"] = _sha256_or_null(output_path)
        result["lock_sha256"] = _sha256_or_null(lock_path)

        sys.stdout.write(json.dumps(result, ensure_ascii=False))
        sys.stdout.write("\n")
        return 0


if __name__ == "__main__":
    sys.exit(main())
