#!/usr/bin/env bash
# Sync skill metadata to AGENTS.md Auto-invoke sections.
# Vendoring of external skills is NOT done here — that lives in the CLI
# (`ai-specs sync` → lib/_internal/vendor-skills.py).
#
# Usage: ./sync.sh [--dry-run] [--scope <scope>]

set -e
shopt -s nullglob

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$(dirname "$(dirname "$(dirname "$SCRIPT_DIR")")")")"
SKILLS_DIR="$REPO_ROOT/ai-specs/skills"
SKILL_CONTRACT_HOME="${AI_SPECS_HOME:-$REPO_ROOT}"
SKILL_CONTRACT_PY="$SKILL_CONTRACT_HOME/lib/_internal/skill_contract.py"

if [ ! -f "$SKILL_CONTRACT_PY" ]; then
    echo -e "${RED}Missing skill contract helper: $SKILL_CONTRACT_PY${NC}" >&2
    exit 1
fi

# List every SKILL.md under .../skills/<name>/SKILL.md (monorepo root and each subrepo).
# Prunes generated dependency/agent directories for speed and to avoid false matches.
list_all_skill_md() {
    find "$REPO_ROOT" \
        \( -name node_modules -o -name .git -o -name .worktrees -o -name .opencode -o -name .claude -o -name .cursor -o -name .gemini -o -name .codex \) -prune -o \
        -type f -path "*/skills/*/SKILL.md" -print 2>/dev/null | LC_ALL=C sort
}

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Options
DRY_RUN=false
FILTER_SCOPE=""

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --scope)
            FILTER_SCOPE="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [--dry-run] [--scope <scope>]"
            echo ""
            echo "Discovers SKILL.md under:"
            echo "  \$REPO_ROOT/skills/<name>/SKILL.md"
            echo "  \$REPO_ROOT/<subrepo>/skills/<name>/SKILL.md"
            echo "(excludes node_modules, .git, .worktrees, and generated agent dirs)"
            echo ""
            echo "If \$REPO_ROOT/.melon-monorepo exists: map each scope to that subfolder's AGENTS.md."
            echo "Otherwise (standalone repo): all scopes update \$REPO_ROOT/AGENTS.md only."
            echo ""
            echo "Options:"
            echo "  --dry-run    Show what would change without modifying files"
            echo "  --scope      Only sync specific scope"
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

# Map scope to AGENTS.md path (melon-alquimia monorepo vs standalone service repo)
get_agents_path() {
    local scope="$1"
    if [ -f "$REPO_ROOT/.melon-monorepo" ]; then
        case "$scope" in
            root)            echo "$REPO_ROOT/AGENTS.md" ;;
            front_web)       echo "$REPO_ROOT/alquimia-front-web/AGENTS.md" ;;
            back_web)        echo "$REPO_ROOT/alquimia-back-web/AGENTS.md" ;;
            apis_design)     echo "$REPO_ROOT/alquimia-apis-design/AGENTS.md" ;;
            apis_designv2)   echo "$REPO_ROOT/apis-designv2/AGENTS.md" ;;
            ml)              echo "$REPO_ROOT/alquimia-ml/AGENTS.md" ;;
            ms_auth)         echo "$REPO_ROOT/alquimia-ms-auth/AGENTS.md" ;;
            ms_memories)     echo "$REPO_ROOT/alquimia-ms-memories/AGENTS.md" ;;
            ms_products)     echo "$REPO_ROOT/alquimia-ms-products/AGENTS.md" ;;
            ms_recomendations) echo "$REPO_ROOT/alquimia-ms-recomendations/AGENTS.md" ;;
            *)               echo "" ;;
        esac
    else
        echo "$REPO_ROOT/AGENTS.md"
    fi
}

read_skill_sync_metadata() {
    local file="$1"
    python3 "$SKILL_CONTRACT_PY" sync-metadata "$file"
}

json_field() {
    local json="$1"
    local field="$2"
    python3 -c 'import json,sys; data=json.loads(sys.argv[1]); value=data.get(sys.argv[2], ""); print(value if isinstance(value, str) else "")' "$json" "$field"
}

json_joined_list() {
    local json="$1"
    local field="$2"
    python3 -c 'import json,sys; data=json.loads(sys.argv[1]); print("|".join(data.get(sys.argv[2], [])))' "$json" "$field"
}

echo -e "${BLUE}Skill Sync - Updating AGENTS.md Auto-invoke sections${NC}"
echo "========================================================"
echo ""

# Collect skills by scope using temp files (Bash 3 compatible)
SCOPE_TMPDIR=$(mktemp -d)
trap 'rm -rf "$SCOPE_TMPDIR"' EXIT

# Deterministic iteration order (stable diffs)
# Note: macOS ships BSD find; avoid GNU-only flags.
while IFS= read -r skill_file; do
    [ -f "$skill_file" ] || continue

    if ! metadata_json=$(read_skill_sync_metadata "$skill_file"); then
        echo -e "${RED}Invalid skill metadata in $skill_file${NC}" >&2
        exit 1
    fi

    skill_name=$(json_field "$metadata_json" "name")
    enabled=$(python3 -c 'import json,sys; print("true" if json.loads(sys.argv[1]).get("enabled") else "false")' "$metadata_json")
    [ "$enabled" = "true" ] || continue

    scope_raw=$(json_joined_list "$metadata_json" "scope")
    auto_invoke_raw=$(json_joined_list "$metadata_json" "auto_invoke")
    # extract_metadata() returns:
    # - single action: "Action"
    # - multiple actions: "Action A|Action B" (pipe-delimited)
    # We use ';;' as separator to avoid conflicts with '|' used between entries.
    auto_invoke=$(echo "$auto_invoke_raw" | sed 's/|/;;/g')

    # Skip if no scope or auto_invoke defined
    [ -z "$scope_raw" ] || [ -z "$auto_invoke" ] && continue

    # Parse scope (JSON helper joins lists with '|'; compatibility inputs may still
    # show comma/space-separated values, so split all of them here).
    # Bash 3 compatible: use tr + read instead of read -ra with <<<
    echo "$scope_raw" | tr '|, ' '\n\n\n' | while read -r scope; do
        scope=$(echo "$scope" | tr -d '[:space:]')
        [ -z "$scope" ] && continue

        # Filter by scope if specified
        [ -n "$FILTER_SCOPE" ] && [ "$scope" != "$FILTER_SCOPE" ] && continue

        # Append to scope's skill file
        echo "$skill_name:$auto_invoke" >> "$SCOPE_TMPDIR/$scope"
    done
done < <(list_all_skill_md)

# Standalone repo: merge all scope buckets into one (same AGENTS.md for every scope)
if [ ! -f "$REPO_ROOT/.melon-monorepo" ] && [ -d "$SCOPE_TMPDIR" ]; then
    merged="$SCOPE_TMPDIR/_merged"
    found_any=false
    for f in "$SCOPE_TMPDIR"/*; do
        [ -f "$f" ] || continue
        case "$(basename "$f")" in _*) continue ;; esac
        found_any=true
        cat "$f" >> "$merged"
        rm -f "$f"
    done
    if $found_any && [ -f "$merged" ]; then
        mv "$merged" "$SCOPE_TMPDIR/root"
    else
        rm -f "$merged"
    fi
fi

# Generate Auto-invoke section for each scope
# Deterministic scope order (stable diffs)
for scope_file in "$SCOPE_TMPDIR"/*; do
    [ -f "$scope_file" ] || continue
    scope=$(basename "$scope_file")
    agents_path=$(get_agents_path "$scope")

    if [ -z "$agents_path" ] || [ ! -f "$agents_path" ]; then
        echo -e "${YELLOW}Warning: No AGENTS.md found for scope '$scope'${NC}"
        continue
    fi

    echo -e "${BLUE}Processing: $scope -> $(basename "$(dirname "$agents_path")")/AGENTS.md${NC}"

    # Build the Auto-invoke table
    auto_invoke_section="### Auto-invoke Skills

When performing these actions, ALWAYS invoke the corresponding skill FIRST:

| Action | Skill |
|--------|-------|"

    # Expand into sortable rows: "action<TAB>skill"
    rows_file=$(mktemp)

    while IFS= read -r entry; do
        [ -z "$entry" ] && continue
        skill_name="${entry%%:*}"
        actions_raw="${entry#*:}"

        # Restore '|' from ';;' and split actions
        actions_raw=$(echo "$actions_raw" | sed 's/;;/|/g')
        echo "$actions_raw" | tr '|' '\n' | while read -r action; do
            action=$(echo "$action" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')
            [ -z "$action" ] && continue
            printf "%s\t%s\n" "$action" "$skill_name" >> "$rows_file"
        done
    done < "$scope_file"

    # Deterministic row order: Action then Skill
    while IFS=$'\t' read -r action skill_name; do
        [ -z "$action" ] && continue
        auto_invoke_section="$auto_invoke_section
| $action | \`$skill_name\` |"
    done < <(LC_ALL=C sort -t $'\t' -k1,1 -k2,2 "$rows_file")

    rm -f "$rows_file"

    if $DRY_RUN; then
        echo -e "${YELLOW}[DRY RUN] Would update $agents_path with:${NC}"
        echo "$auto_invoke_section"
        echo ""
    else
        # Write new section to temp file (avoids awk multi-line string issues on macOS)
        section_file=$(mktemp)
        echo "$auto_invoke_section" > "$section_file"

        # Check if Auto-invoke section exists
        if grep -q "### Auto-invoke Skills" "$agents_path"; then
            # Replace existing section (up to next --- or ## heading)
            awk '
                /^### Auto-invoke Skills/ {
                    while ((getline line < "'"$section_file"'") > 0) print line
                    close("'"$section_file"'")
                    skip = 1
                    next
                }
                skip && /^(---|## )/ {
                    skip = 0
                    print ""
                }
                !skip { print }
            ' "$agents_path" > "$agents_path.tmp"
            mv "$agents_path.tmp" "$agents_path"
            echo -e "${GREEN}  ✓ Updated Auto-invoke section${NC}"
        else
            # Insert after Skills Reference blockquote
            awk '
                /^>.*SKILL\.md\)$/ && !inserted {
                    print
                    getline
                    if (/^$/) {
                        print ""
                        while ((getline line < "'"$section_file"'") > 0) print line
                        close("'"$section_file"'")
                        print ""
                        inserted = 1
                        next
                    }
                }
                { print }
            ' "$agents_path" > "$agents_path.tmp"
            mv "$agents_path.tmp" "$agents_path"
            echo -e "${GREEN}  ✓ Inserted Auto-invoke section${NC}"
        fi

        rm -f "$section_file"
    fi
done

echo ""
echo -e "${GREEN}Done!${NC}"

# Show skills without metadata
echo ""
echo -e "${BLUE}Skills missing sync metadata:${NC}"
missing=0
while IFS= read -r skill_file; do
    [ -f "$skill_file" ] || continue

    if ! metadata_json=$(read_skill_sync_metadata "$skill_file"); then
        echo -e "  ${RED}invalid${NC} - $skill_file"
        missing=$((missing + 1))
        continue
    fi

    skill_name=$(json_field "$metadata_json" "name")
    scope_raw=$(json_joined_list "$metadata_json" "scope")
    auto_invoke_raw=$(json_joined_list "$metadata_json" "auto_invoke")
    auto_invoke=$(echo "$auto_invoke_raw" | sed 's/|/;;/g')

    if [ -z "$scope_raw" ] || [ -z "$auto_invoke" ]; then
        echo -e "  ${YELLOW}$skill_name${NC} - missing: ${scope_raw:+}${scope_raw:-scope} ${auto_invoke:+}${auto_invoke:-auto_invoke}"
        missing=$((missing + 1))
    fi
done < <(list_all_skill_md)

if [ $missing -eq 0 ]; then
    echo -e "  ${GREEN}All skills have sync metadata${NC}"
fi
