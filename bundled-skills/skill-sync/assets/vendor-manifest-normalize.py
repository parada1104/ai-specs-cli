#!/usr/bin/env python3
"""
Read vendor.manifest.toml (or pass through legacy pipe rows) and print
pipe-delimited lines for vendor-skills.sh:
  id|source|scope_at_root|auto_invoke_root;;joined|auto_invoke_subrepo;;joined|yes|no|only_subrepos_csv|license|vendor_attribution
  (Trailing fields optional for legacy 6-column rows.)
"""
from __future__ import annotations

import sys
from pathlib import Path

try:
    import tomllib
except ModuleNotFoundError:  # pragma: no cover
    print(
        "Python 3.11+ required (stdlib tomllib), or: pip install tomli",
        file=sys.stderr,
    )
    sys.exit(1)


def _as_phrase_list(v: object) -> list[str]:
    if v is None:
        return []
    if isinstance(v, str):
        return [v.strip()] if v.strip() else []
    if isinstance(v, list):
        out: list[str] = []
        for x in v:
            if isinstance(x, str) and x.strip():
                out.append(x.strip())
        return out
    return []


def _join_phrases(phrases: list[str]) -> str:
    return ";;".join(phrases)


def emit_toml(path: Path) -> None:
    data = tomllib.loads(path.read_text(encoding="utf-8"))
    entries = data.get("skills") or data.get("entries")
    if not isinstance(entries, list):
        print("vendor.manifest.toml: expected top-level [[skills]] or entries: list", file=sys.stderr)
        sys.exit(1)
    for raw in entries:
        if not isinstance(raw, dict):
            continue
        id_ = raw.get("id")
        source = raw.get("source")
        if not id_ or not source:
            print(f"vendor.manifest.toml: skip entry missing id or source: {raw!r}", file=sys.stderr)
            continue
        sr = raw.get("scope_at_monorepo_root", raw.get("scope_at_root", "root"))
        if isinstance(sr, list):
            scope = ",".join(str(x).strip() for x in sr if str(x).strip())
        else:
            scope = str(sr)
        aroot = _as_phrase_list(raw.get("auto_invoke_root"))
        asub = _as_phrase_list(raw.get("auto_invoke_subrepo"))
        if not asub:
            asub = aroot.copy()
        insub = raw.get("install_into_subrepos", False)
        yesno = "yes" if insub in (True, "true", "True", "yes", "YES", 1) else "no"
        only = raw.get("only_subrepos")
        if isinstance(only, list):
            whitelist = ",".join(str(x).strip() for x in only if str(x).strip())
        elif only:
            whitelist = str(only).strip()
        else:
            whitelist = ""
        license_s = str(raw.get("license", "MIT")).strip() or "MIT"
        attr = str(raw.get("vendor_attribution", "")).strip()
        if not attr:
            attr = (
                "obra/superpowers"
                if "obra/superpowers" in str(source)
                else "external-vendored"
            )
        for ch in ("|", "\n", "\r"):
            attr = attr.replace(ch, " ")
        line = "|".join(
            [
                str(id_).strip(),
                str(source).strip(),
                scope,
                _join_phrases(aroot),
                _join_phrases(asub),
                yesno,
                whitelist,
                license_s,
                attr,
            ]
        )
        print(line)


def emit_pipe(path: Path) -> None:
    for line in path.read_text(encoding="utf-8").splitlines():
        s = line.strip()
        if not s or s.startswith("#"):
            continue
        print(line.rstrip("\n"))


def main() -> None:
    if len(sys.argv) != 2:
        print("Usage: vendor-manifest-normalize.py <manifest.toml|manifest>", file=sys.stderr)
        sys.exit(2)
    path = Path(sys.argv[1])
    if not path.is_file():
        print(f"Not found: {path}", file=sys.stderr)
        sys.exit(1)
    suffix = path.suffix.lower()
    if suffix in (".toml",):
        emit_toml(path)
    else:
        emit_pipe(path)


if __name__ == "__main__":
    main()
