#!/usr/bin/env bash
# Download external SKILL.md (e.g. obra/superpowers), inject melon frontmatter, write skills/<id>/SKILL.md
# Usage: see --help

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$(dirname "$(dirname "$(dirname "$SCRIPT_DIR")")")")"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

DRY_RUN=false
ALL_SUBREPOS=false
MANIFEST=""
TARGET_SKILLS_DIR=""
SCOPE_OVERRIDE=""

show_help() {
    echo "Usage: $0 [--dry-run] [--all-subrepos] [--manifest PATH] [--target-skills-dir DIR] [--scope SCOPE]"
    echo ""
    echo "Default manifest: skills/vendor.manifest.toml (human-editable TOML)."
    echo "  Override with --manifest; legacy pipe file vendor.manifest still supported."
    echo "  TOML schema: see comments in vendor.manifest.toml (Python 3.11+ tomllib)."
    echo ""
    echo "Standalone repo (no .melon-monorepo): default --target-skills-dir is ./skills;"
    echo "  scope from skills/.scope if --scope not set."
    exit 0
}

while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run) DRY_RUN=true; shift ;;
        --all-subrepos) ALL_SUBREPOS=true; shift ;;
        --manifest) MANIFEST="$2"; shift 2 ;;
        --target-skills-dir) TARGET_SKILLS_DIR="$2"; shift 2 ;;
        --scope) SCOPE_OVERRIDE="$2"; shift 2 ;;
        --help|-h) show_help ;;
        *) echo -e "${RED}Unknown option: $1${NC}"; exit 1 ;;
    esac
done

if [ -z "${MANIFEST:-}" ]; then
    if [ -f "$REPO_ROOT/ai-specs/skills/vendor.manifest.toml" ]; then
        MANIFEST="$REPO_ROOT/ai-specs/skills/vendor.manifest.toml"
    else
        MANIFEST="$REPO_ROOT/ai-specs/skills/vendor.manifest"
    fi
fi
SUBREPOS_FILE="$REPO_ROOT/ai-specs/skills/subrepos.txt"

if [ ! -f "$MANIFEST" ]; then
    echo -e "${RED}Manifest not found: $MANIFEST${NC}"
    exit 1
fi

NORMALIZE_PY="$SCRIPT_DIR/vendor-manifest-normalize.py"
MANIFEST_PIPE=$(mktemp)
trap 'rm -f "$MANIFEST_PIPE"' EXIT
if ! python3 "$NORMALIZE_PY" "$MANIFEST" > "$MANIFEST_PIPE"; then
    echo -e "${RED}Failed to read manifest (need Python 3.11+ for .toml)${NC}"
    exit 1
fi

# Strip YAML comment lines from subrepos file for parsing
subrepos_entries() {
    grep -v '^#' "$SUBREPOS_FILE" | grep -v '^[[:space:]]*$' || true
}

auto_invoke_yaml() {
    local csv="$1"
    echo "$csv" | awk -F';;' '{
        for (i = 1; i <= NF; i++) {
            t = $i
            gsub(/^[ \t]+|[ \t]+$/, "", t)
            if (t != "") {
                gsub(/"/, "\\\"", t)
                printf "    - \"%s\"\n", t
            }
        }
    }'
}

subrepos_yes() {
    case "$1" in yes|Yes|YES) return 0 ;; *) return 1 ;; esac
}

row_allowed_for_subrepo() {
    local subdir="$1"
    local whitelist="$2"
    [ -z "$whitelist" ] && return 0
    local IFS=',' item
    for item in $whitelist; do
        item="${item//[[:space:]]/}"
        [ -z "$item" ] && continue
        [ "$item" = "$subdir" ] && return 0
    done
    return 1
}

write_scope_yaml() {
    local s="$1"
    if [[ "$s" == *","* ]]; then
        printf '  scope: ['
        local first=1
        local oIFS=$IFS
        IFS=','
        read -ra PARTS <<< "$s"
        IFS=$oIFS
        local p
        for p in "${PARTS[@]}"; do
            p="${p//[[:space:]]/}"
            [ -z "$p" ] && continue
            [ "$first" -eq 0 ] && printf ', '
            printf '%s' "$p"
            first=0
        done
        printf ']\n'
    else
        echo "  scope: [$s]"
    fi
}

install_skill() {
    local id="$1"
    local url="$2"
    local scope="$3"
    local auto_csv="$4"
    local dest_root="$5"
    local lic="${6:-MIT}"
    local author="${7:-external-vendored}"

    local dest="$dest_root/$id"
    if $DRY_RUN; then
        echo -e "${YELLOW}[DRY RUN]${NC} $id -> $dest/SKILL.md (scope=$scope, lic=$lic)"
        return 0
    fi

    mkdir -p "$dest"
    local tmp bodyf
    tmp=$(mktemp)
    bodyf=$(mktemp)
    if ! curl -fsSL "$url" -o "$tmp"; then
        echo -e "${RED}curl failed: $url${NC}"
        rm -f "$tmp"
        return 1
    fi

    awk '/^---$/ { c++; if (c == 2) { b = 1; next } } b { print }' "$tmp" > "$bodyf"
    if [ ! -s "$bodyf" ]; then
        echo -e "${YELLOW}Warning: no body after frontmatter for $id; using full file${NC}" >&2
        cp "$tmp" "$bodyf"
    fi

    local auto_block
    auto_block=$(auto_invoke_yaml "$auto_csv")

    {
        echo "---"
        echo "name: $id"
        echo "description: >"
        echo "  Vendored copy (${author}). License: ${lic}. Upstream: $url"
        echo "license: $lic"
        echo "metadata:"
        echo "  author: vendored-${id//[^a-zA-Z0-9_-]/-}"
        echo "  version: \"1.0\""
        write_scope_yaml "$scope"
        echo "  auto_invoke:"
        if [ -n "$auto_block" ]; then
            echo "$auto_block"
        else
            echo "    - \"Load this skill when its topic matches the task\""
        fi
        echo "---"
        cat "$bodyf"
    } > "$dest/SKILL.md"

    rm -f "$tmp" "$bodyf"
    echo -e "${GREEN}  ✓${NC} $dest/SKILL.md"
}

echo -e "${BLUE}Vendor skills${NC}"
echo "============="

if $ALL_SUBREPOS; then
    if [ ! -f "$REPO_ROOT/.melon-monorepo" ]; then
        echo -e "${RED}--all-subrepos requires .melon-monorepo at $REPO_ROOT${NC}"
        exit 1
    fi
    echo -e "${BLUE}Monorepo root skills${NC}"
    root_skills="$REPO_ROOT/ai-specs/skills"
    while IFS= read -r line || [ -n "$line" ]; do
        [[ "$line" =~ ^# ]] || [ -z "${line// }" ] && continue
        IFS='|' read -r id url sroot aroot asub insub onlysub vlic vattr <<< "$line"
        [ -z "$id" ] && continue
        install_skill "$id" "$url" "$sroot" "$aroot" "$root_skills" "${vlic:-MIT}" "${vattr:-external-vendored}"
    done < "$MANIFEST_PIPE"

    echo -e "${BLUE}Per-subrepo skills${NC}"
    while IFS= read -r subline; do
        [ -z "$subline" ] && continue
        subdir=$(echo "$subline" | awk -F'\t' '{print $1}')
        sc=$(echo "$subline" | awk -F'\t' '{print $2}')
        [ -z "$subdir" ] || [ -z "$sc" ] && continue
        dest="$REPO_ROOT/$subdir/ai-specs/skills"
        [ -d "$dest" ] || { mkdir -p "$dest"; }
        echo -e "${BLUE}  -> $subdir (scope=$sc)${NC}"
        while IFS= read -r line || [ -n "$line" ]; do
            [[ "$line" =~ ^# ]] || [ -z "${line// }" ] && continue
            IFS='|' read -r id url sroot aroot asub insub onlysub vlic vattr <<< "$line"
            [ -z "$id" ] && continue
            subrepos_yes "$insub" || continue
            row_allowed_for_subrepo "$subdir" "$onlysub" || continue
            auto="$asub"
            [ -z "$auto" ] && auto="$aroot"
            install_skill "$id" "$url" "$sc" "$auto" "$dest" "${vlic:-MIT}" "${vattr:-external-vendored}"
        done < "$MANIFEST_PIPE"
    done < <(subrepos_entries)
else
    skills_root="${TARGET_SKILLS_DIR:-$REPO_ROOT/ai-specs/skills}"
    scope_eff="$SCOPE_OVERRIDE"
    if [ -z "$scope_eff" ]; then
        if [ -f "$REPO_ROOT/.melon-monorepo" ]; then
            scope_eff="root"
        elif [ -f "$skills_root/.scope" ]; then
            scope_eff=$(head -1 "$skills_root/.scope" | tr -d '[:space:]')
        else
            echo -e "${RED}Standalone repo: set skills/.scope or pass --scope${NC}"
            exit 1
        fi
    fi

    if [ -f "$REPO_ROOT/.melon-monorepo" ]; then
        while IFS= read -r line || [ -n "$line" ]; do
            [[ "$line" =~ ^# ]] || [ -z "${line// }" ] && continue
            IFS='|' read -r id url sroot aroot asub insub onlysub vlic vattr <<< "$line"
            [ -z "$id" ] && continue
            install_skill "$id" "$url" "$sroot" "$aroot" "$skills_root" "${vlic:-MIT}" "${vattr:-external-vendored}"
        done < "$MANIFEST_PIPE"
    else
        STAND_SUB=$(basename "$REPO_ROOT")
        while IFS= read -r line || [ -n "$line" ]; do
            [[ "$line" =~ ^# ]] || [ -z "${line// }" ] && continue
            IFS='|' read -r id url sroot aroot asub insub onlysub vlic vattr <<< "$line"
            [ -z "$id" ] && continue
            subrepos_yes "$insub" || continue
            row_allowed_for_subrepo "$STAND_SUB" "$onlysub" || continue
            auto="$asub"
            [ -z "$auto" ] && auto="$aroot"
            install_skill "$id" "$url" "$scope_eff" "$auto" "$skills_root" "${vlic:-MIT}" "${vattr:-external-vendored}"
        done < "$MANIFEST_PIPE"
    fi
fi

echo -e "${GREEN}Done.${NC}"
