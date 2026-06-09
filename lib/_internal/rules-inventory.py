#!/usr/bin/env python3
"""Read-only inventory of legacy Cursor rules for ai-specs migration planning.

Scans .cursor/rules, .cursorrules, AGENTS.md, manifest, skills, and recipe
catalog. Emits JSON to stdout. MUST NOT write any files.
"""
from __future__ import annotations

import importlib.util
import json
import re
import sys
import tomllib
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

SCHEMA_VERSION = 1
BODY_EXCERPT_LEN = 400

RECIPE_CATALOG = [
    "worktree-flow",
    "git-pr-flow",
    "session-context",
    "tdd-flow",
    "trello-mcp-workflow",
    "vault-canonical-store",
]

RECIPE_KEYWORDS: dict[str, tuple[str, ...]] = {
    "worktree-flow": ("worktree",),
    "git-pr-flow": ("pull request", "git pr", "open pr"),
    "tdd-flow": ("tdd", "test-driven", "red-green", "run tests first"),
    "trello-mcp-workflow": ("trello", "trello board", "trello card"),
    "vault-canonical-store": ("vault", "obsidian", "canonical vault"),
    "session-context": ("session context", "session bootstrap", "bootstrap session"),
}

CLASSIFICATION_BUCKETS = (
    "keep_in_brief",
    "enable_recipe",
    "use_catalog_dep",
    "create_local_skill",
    "merge_into_skill",
    "already_in_atl",
    "deprecate_rule_file",
)

LOCKFILE_HINTS: dict[str, str] = {
    "package-lock.json": "node",
    "yarn.lock": "node",
    "pnpm-lock.yaml": "node",
    "pyproject.toml": "python",
    "requirements.txt": "python",
    "Pipfile": "python",
    "go.mod": "go",
    "Cargo.toml": "rust",
}

HEADING_RE = re.compile(r"^#{1,3}\s+(.+)$", re.MULTILINE)
SKILL_ID_RE = re.compile(r"`([a-z0-9][a-z0-9-]*)`")
_WORD_KEYWORD_RE_CACHE: dict[str, re.Pattern[str]] = {}
_MDC_META_FALLBACK_RE = {
    "description": re.compile(r"^description:\s*(.+)$", re.IGNORECASE | re.MULTILINE),
    "globs": re.compile(r"^globs:\s*(\[[^\]]*\]|.+)$", re.IGNORECASE | re.MULTILINE),
    "alwaysApply": re.compile(
        r"^alwaysApply:\s*(.+)$", re.IGNORECASE | re.MULTILINE
    ),
    "always_apply": re.compile(
        r"^always_apply:\s*(.+)$", re.IGNORECASE | re.MULTILINE
    ),
}


def _load_module(filename: str, module_name: str):
    module_path = Path(__file__).with_name(filename)
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load {filename}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


_skill_contract = _load_module("skill_contract.py", "skill_contract_internal")
_skill_resolution = _load_module("skill-resolution.py", "skill_resolution_internal")

split_frontmatter = _skill_contract.split_frontmatter
parse_frontmatter = _skill_contract.parse_frontmatter
_strip_quotes = _skill_contract._strip_quotes
collect_skills = _skill_resolution.collect_skills


def _coerce_bool(value: Any) -> bool:
    if value is None:
        return False
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return bool(value)
    if isinstance(value, str):
        return value.strip().lower() in {"true", "1", "yes"}
    return bool(value)


def _keyword_in_haystack(keyword: str, haystack: str) -> bool:
    if " " in keyword:
        return keyword in haystack
    pattern = _WORD_KEYWORD_RE_CACHE.get(keyword)
    if pattern is None:
        pattern = re.compile(rf"\b{re.escape(keyword)}\b")
        _WORD_KEYWORD_RE_CACHE[keyword] = pattern
    return pattern.search(haystack) is not None


def _parse_mdc_meta(frontmatter: str) -> dict[str, Any]:
    if not frontmatter:
        return {}
    try:
        return parse_frontmatter(frontmatter)
    except Exception:
        pass

    meta: dict[str, Any] = {}
    for line in frontmatter.splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        if line.startswith(" "):
            continue
        if ":" not in stripped:
            continue
        key, _, rest = stripped.partition(":")
        key = key.strip()
        rest = rest.strip()
        if key in {"description", "globs", "alwaysApply", "always_apply"}:
            if rest.startswith("[") and rest.endswith("]"):
                inner = rest[1:-1].strip()
                meta[key] = (
                    [_strip_quotes(part.strip()) for part in inner.split(",") if part.strip()]
                    if inner
                    else []
                )
            else:
                meta[key] = _strip_quotes(rest)

    for key, pattern in _MDC_META_FALLBACK_RE.items():
        if key in meta:
            continue
        match = pattern.search(frontmatter)
        if not match:
            continue
        value = match.group(1).strip()
        if key == "globs" and value.startswith("[") and value.endswith("]"):
            inner = value[1:-1].strip()
            meta[key] = (
                [_strip_quotes(part.strip()) for part in inner.split(",") if part.strip()]
                if inner
                else []
            )
        else:
            meta[key] = _strip_quotes(value)
    return meta


@dataclass
class InventoryItem:
    path: str
    body_excerpt: str = ""
    description: str = ""
    globs: list[str] = field(default_factory=list)
    always_apply: bool = False
    heading: str = ""
    candidate_recipes: list[str] = field(default_factory=list)
    already_resolved: bool = False
    classification: str = "create_local_skill"


@dataclass
class RulesInventory:
    root: Path

    def scan(self) -> dict[str, Any]:
        resolved = self._resolved_skills()
        resolved_ids = {item["id"] for item in resolved}
        atl = self._scan_atl_registry()
        atl_ids = set(atl.get("skill_ids", []))

        cursor_rules = self._scan_cursor_rules(resolved_ids, atl_ids)
        cursorrules = self._scan_cursorrules(resolved_ids, atl_ids)
        agents_md_sections = self._scan_agents_md_sections(resolved_ids, atl_ids)
        manifest = self._scan_manifest()
        mode = self._detect_mode(cursor_rules, cursorrules)

        payload: dict[str, Any] = {
            "schema_version": SCHEMA_VERSION,
            "mode": mode,
            "target": str(self.root.resolve()),
            "classification_is_suggestion": True,
            "sources": {
                "cursor_rules": [self._item_dict(item) for item in cursor_rules],
                "cursorrules": cursorrules,
                "agents_md_sections": [self._item_dict(item) for item in agents_md_sections],
                "agents_md_present": (self.root / "AGENTS.md").is_file(),
                "manifest": manifest,
                "resolved_skills": resolved,
                "recipe_catalog": list(RECIPE_CATALOG),
                "atl_registry": atl,
            },
            "summary": {
                "cursor_rules": len(cursor_rules),
                "cursorrules": 0 if self._cursorrules_absent(cursorrules) else 1,
                "agents_md_sections": len(agents_md_sections),
            },
        }

        if mode == "B":
            payload["stack_hints"] = self._stack_hints()
            payload["recommendations"] = {
                "init": "ai-specs init",
                "default_recipes": self._default_recipes_for_stack(payload["stack_hints"]),
                "brief_hint": "Draft a runtime-brief [brief] section in AGENTS.md after init.",
            }

        return payload

    def _item_dict(self, item: InventoryItem) -> dict[str, Any]:
        data: dict[str, Any] = {
            "path": item.path,
            "body_excerpt": item.body_excerpt,
            "candidate_recipes": item.candidate_recipes,
            "already_resolved": item.already_resolved,
            "classification": item.classification,
        }
        if item.description:
            data["description"] = item.description
        if item.globs:
            data["globs"] = item.globs
        if item.heading:
            data["heading"] = item.heading
        if item.path.endswith(".mdc"):
            data["always_apply"] = item.always_apply
        return data

    def _cursorrules_absent(self, cursorrules: dict[str, Any] | list[Any]) -> bool:
        if isinstance(cursorrules, dict):
            return cursorrules.get("status") == "absent"
        return not cursorrules

    def _detect_mode(
        self,
        cursor_rules: list[InventoryItem],
        cursorrules: dict[str, Any] | list[Any],
    ) -> str:
        if cursor_rules or not self._cursorrules_absent(cursorrules):
            return "A"
        if (self.root / "AGENTS.md").is_file():
            return "A"
        return "B"

    def _stack_hints(self) -> list[str]:
        hints: list[str] = []
        for name, stack in LOCKFILE_HINTS.items():
            if (self.root / name).is_file() and stack not in hints:
                hints.append(stack)
        return hints

    def _default_recipes_for_stack(self, stack_hints: list[str]) -> list[str]:
        recipes = ["session-context"]
        if "python" in stack_hints or "node" in stack_hints:
            recipes.extend(["worktree-flow", "tdd-flow"])
        if "node" in stack_hints:
            recipes.append("git-pr-flow")
        seen: set[str] = set()
        ordered: list[str] = []
        for recipe in recipes:
            if recipe in RECIPE_CATALOG and recipe not in seen:
                seen.add(recipe)
                ordered.append(recipe)
        return ordered

    def _excerpt(self, text: str) -> str:
        compact = text.strip()
        if len(compact) <= BODY_EXCERPT_LEN:
            return compact
        return compact[:BODY_EXCERPT_LEN]

    def _match_recipes(self, *parts: str) -> list[str]:
        haystack = " ".join(p for p in parts if p).lower()
        matches: list[str] = []
        for recipe_id, keywords in RECIPE_KEYWORDS.items():
            if any(_keyword_in_haystack(keyword, haystack) for keyword in keywords):
                matches.append(recipe_id)
        return matches

    def _classify(
        self,
        *,
        candidate_recipes: list[str],
        already_resolved: bool,
        heading: str = "",
        body: str = "",
    ) -> str:
        if already_resolved:
            return "already_in_atl"
        heading_lower = heading.lower()
        body_lower = body.lower()
        if heading_lower and any(
            _keyword_in_haystack(token, heading_lower)
            for token in ("workflow", "project", "conflict", "runtime")
        ):
            return "keep_in_brief"
        if candidate_recipes:
            return "enable_recipe"
        if "deprecate" in body_lower or "obsolete" in body_lower:
            return "deprecate_rule_file"
        if "merge into" in body_lower:
            return "merge_into_skill"
        if "catalog dep" in body_lower or "vendored skill" in body_lower:
            return "use_catalog_dep"
        return "create_local_skill"

    def _apply_classification(
        self,
        item: InventoryItem,
        resolved_ids: set[str],
        atl_ids: set[str],
    ) -> None:
        item.candidate_recipes = self._match_recipes(
            item.description,
            item.heading,
            item.body_excerpt,
        )
        item.already_resolved = any(
            recipe.replace("-", "_") in resolved_ids or recipe in resolved_ids
            for recipe in item.candidate_recipes
        ) or any(token in atl_ids for token in item.candidate_recipes)
        item.classification = self._classify(
            candidate_recipes=item.candidate_recipes,
            already_resolved=item.already_resolved,
            heading=item.heading,
            body=item.body_excerpt,
        )

    def _scan_cursor_rules(
        self,
        resolved_ids: set[str],
        atl_ids: set[str],
    ) -> list[InventoryItem]:
        rules_dir = self.root / ".cursor" / "rules"
        if not rules_dir.is_dir():
            return []

        items: list[InventoryItem] = []
        for path in sorted(rules_dir.rglob("*.mdc")):
            text = path.read_text(encoding="utf-8", errors="replace")
            frontmatter, body = split_frontmatter(text)
            meta = _parse_mdc_meta(frontmatter)
            globs_raw = meta.get("globs", [])
            if isinstance(globs_raw, str):
                globs = [globs_raw]
            elif isinstance(globs_raw, list):
                globs = [str(g) for g in globs_raw]
            else:
                globs = []
            always_apply = _coerce_bool(
                meta.get("alwaysApply") if "alwaysApply" in meta else meta.get("always_apply")
            )
            item = InventoryItem(
                path=str(path.relative_to(self.root)),
                description=str(meta.get("description", "")),
                globs=globs,
                always_apply=always_apply,
                body_excerpt=self._excerpt(body),
            )
            self._apply_classification(item, resolved_ids, atl_ids)
            items.append(item)
        return items

    def _scan_cursorrules(
        self,
        resolved_ids: set[str],
        atl_ids: set[str],
    ) -> dict[str, Any] | list[dict[str, Any]]:
        path = self.root / ".cursorrules"
        if not path.is_file():
            return {"status": "absent"}

        body = path.read_text(encoding="utf-8", errors="replace")
        item = InventoryItem(
            path=".cursorrules",
            body_excerpt=self._excerpt(body),
        )
        self._apply_classification(item, resolved_ids, atl_ids)
        return [self._item_dict(item)]

    def _scan_agents_md_sections(
        self,
        resolved_ids: set[str],
        atl_ids: set[str],
    ) -> list[InventoryItem]:
        path = self.root / "AGENTS.md"
        if not path.is_file():
            return []

        text = path.read_text(encoding="utf-8", errors="replace")

        # Strip fenced code-block regions before scanning for headings so that
        # # lines inside ``` or ~~~ fences are not captured as spurious sections.
        #
        # FENCE_RE matches both backtick and tilde fences (closing fence must use
        # the same character as the opening fence).  Unterminated fences are
        # handled by a second pass that blanks from an opening marker to EOF.
        #
        # Length-preserving substitution: replace every non-newline character in
        # the fenced block with a space so len(text_no_fences) == len(text) and
        # heading offsets computed on text_no_fences align exactly with text.
        FENCE_RE = re.compile(
            r"^(?P<fence>`{3,}|~{3,})[^\n]*\n.*?^(?P=fence)[ \t]*$",
            re.MULTILINE | re.DOTALL,
        )
        _blank = lambda m: re.sub(r"[^\n]", " ", m.group(0))  # noqa: E731
        text_no_fences = FENCE_RE.sub(_blank, text)
        # Unterminated fence: opening marker with no matching close — blank to EOF.
        OPEN_FENCE_RE = re.compile(r"^(?:`{3,}|~{3,})[^\n]*\n", re.MULTILINE)
        for open_m in OPEN_FENCE_RE.finditer(text_no_fences):
            # Only act if the opening marker itself was not already blanked
            # (i.e. it starts with a fence char, not a space).
            if text_no_fences[open_m.start()] in ("`", "~"):
                rest_start = open_m.start()
                replacement = re.sub(r"[^\n]", " ", text_no_fences[rest_start:])
                text_no_fences = text_no_fences[:rest_start] + replacement
                break  # at most one unterminated fence can remain

        matches = list(HEADING_RE.finditer(text_no_fences))
        if not matches:
            # N1: monolithic AGENTS.md with no headings — emit a single section
            # built from the full body so the file is not silently dropped.
            body = text.strip()
            if not body:
                return []
            item = InventoryItem(
                path="AGENTS.md",
                heading="",
                body_excerpt=self._excerpt(body),
            )
            self._apply_classification(item, resolved_ids, atl_ids)
            return [item]

        items: list[InventoryItem] = []
        preamble = text[: matches[0].start()].strip()
        if preamble:
            item = InventoryItem(
                path="AGENTS.md",
                heading="",
                body_excerpt=self._excerpt(preamble),
            )
            self._apply_classification(item, resolved_ids, atl_ids)
            items.append(item)

        for index, match in enumerate(matches):
            heading = match.group(1).strip()
            start = match.end()
            end = matches[index + 1].start() if index + 1 < len(matches) else len(text)
            body = text[start:end].strip()
            item = InventoryItem(
                path="AGENTS.md",
                heading=heading,
                body_excerpt=self._excerpt(body),
            )
            self._apply_classification(item, resolved_ids, atl_ids)
            items.append(item)
        return items

    def _scan_manifest(self) -> dict[str, Any]:
        toml_path = self.root / "ai-specs" / "ai-specs.toml"
        if not toml_path.is_file():
            return {"present": False}

        try:
            with toml_path.open("rb") as handle:
                data = tomllib.load(handle)
        except tomllib.TOMLDecodeError:
            return {"present": True, "parse_error": True}

        agents = data.get("agents", {})
        enabled = agents.get("enabled", []) if isinstance(agents, dict) else []
        recipes_raw = data.get("recipes", {})
        if isinstance(recipes_raw, dict):
            recipes = [
                recipe_id
                for recipe_id, cfg in recipes_raw.items()
                if isinstance(cfg, dict) and _coerce_bool(cfg.get("enabled"))
            ]
        elif isinstance(recipes_raw, list):
            recipes = [
                block.get("id")
                for block in recipes_raw
                if isinstance(block, dict)
                and block.get("id")
                and _coerce_bool(block.get("enabled", True))
            ]
        else:
            recipes = []
        agents_md = self.root / "AGENTS.md"
        has_runtime_brief = False
        if agents_md.is_file():
            content = agents_md.read_text(encoding="utf-8", errors="replace").lower()
            has_runtime_brief = "runtime brief" in content or "director de orquesta" in content

        return {
            "present": True,
            "enabled_agents": list(enabled) if isinstance(enabled, list) else [],
            "recipes": recipes,
            "has_runtime_brief": has_runtime_brief,
        }

    def _resolved_skills(self) -> list[dict[str, str]]:
        resolved = collect_skills(self.root)
        return [
            {"id": skill_id, "source": source}
            for skill_id, (source, _path) in sorted(resolved.items())
        ]

    def _scan_atl_registry(self) -> dict[str, Any]:
        registry_path = self.root / ".atl" / "skill-registry.md"
        if not registry_path.is_file():
            return {"present": False, "skill_ids": []}

        text = registry_path.read_text(encoding="utf-8", errors="replace")
        skill_ids = sorted(set(SKILL_ID_RE.findall(text)))
        return {"present": True, "skill_ids": skill_ids}


def main() -> int:
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <project-path>", file=sys.stderr)
        return 2

    root = Path(sys.argv[1]).resolve()
    if not root.is_dir():
        print(f"ERROR: not a directory: {root}", file=sys.stderr)
        return 2

    try:
        inventory = RulesInventory(root)
        print(json.dumps(inventory.scan(), indent=2))
    except Exception as exc:  # noqa: BLE001 — surface scan failures without writing files
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
