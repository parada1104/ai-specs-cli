#!/usr/bin/env python3
"""Differential reference driver for the Go internal/cliversion port (GO-12).

Reads a JSON corpus and prints one line per case: "<index>\t<canonical result>".
The Go test builds the same corpus and compares its own lines byte for byte.
"""
from __future__ import annotations

import importlib.util
import json
import sys
import tomllib
from pathlib import Path

# testdata/ -> internal/cliversion/ -> internal/ -> repo root
REPO_ROOT = Path(__file__).resolve().parents[3]


def load_module():
    path = REPO_ROOT / "lib" / "_internal" / "cli_version.py"
    spec = importlib.util.spec_from_file_location("cli_version_ref", path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def fmt_parse(t):
    if t is None:
        return "None"
    major, minor, patch, pre = t
    return f"({major}, {minor}, {patch}, {pre!r})"


def main() -> int:
    mod = load_module()
    corpus = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
    for index, case in enumerate(corpus):
        kind = case["kind"]
        if kind == "compare":
            result = str(mod.compare_versions(case["left"], case["right"]))
        elif kind == "parse":
            result = fmt_parse(mod._parse_version_tuple(case["version"]))
        elif kind == "policy":
            manifest = tomllib.loads(case.get("toml", ""))
            policy, err = mod.parse_tool_policy(manifest)
            if policy is not None:
                result = f"{policy.kind}|{policy.version}"
            elif err:
                result = "error|" + err
            else:
                result = "none"
        elif kind == "checkpolicy":
            policy = mod.ToolPolicy(kind=case["kind_"], version=case["version"])
            ok, reason = mod.check_policy(case["installed"], policy)
            result = "ok" if ok else "fail|" + reason
        elif kind == "evaluate":
            manifest = tomllib.loads(case.get("manifest", ""))
            lock = {}
            if case.get("lock"):
                import tomllib as _t  # noqa: F401
                with open(case["lock"], "rb") as fh:
                    lock = _t.load(fh).get("meta") or {}
            severity, name, message = mod.evaluate_cli_version(
                installed=case["installed"], manifest=manifest, lock_meta=lock
            )
            result = f"{severity}|{name}|{message}"
        elif kind == "installed":
            result = mod.read_installed_version(Path(case["home"]))
        elif kind == "lockmeta":
            meta = mod.read_lock_meta(Path(case["lock"]))
            result = ";".join(f"{k}={meta[k]}" for k in sorted(meta))
        else:
            raise SystemExit(f"unknown kind {kind!r}")
        print(f"{index}\t{result}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
