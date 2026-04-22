#!/usr/bin/env bash
# Sync skill metadata to AGENTS.md Auto-invoke sections
# Usage: ./sync.sh [--dry-run] [--scope <scope>]

set -e
shopt -s nullglob

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$(dirname "$(dirname "$(dirname "$SCRIPT_DIR")")")")"
SKILLS_DIR="$REPO_ROOT/ai-specs/skills"

# List every SKILL.md under .../skills/<name>/SKILL.md (monorepo root and each subrepo).
# Prunes node_modules, .git, and .worktrees for speed and to avoid false matches.
list_all_skill_md() {
    find "$REPO_ROOT" \
        \( -name node_modules -o -name .git -o -name .worktrees \) -prune -o \
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
RUN_VENDOR=false

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
        --vendor)
            RUN_VENDOR=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [--dry-run] [--scope <scope>] [--vendor]"
            echo ""
            echo "Discovers SKILL.md under:"
            echo "  \$REPO_ROOT/skills/<name>/SKILL.md"
            echo "  \$REPO_ROOT/<subrepo>/skills/<name>/SKILL.md"
            echo "(excludes node_modules, .git, .worktrees)"
            echo ""
            echo "If \$REPO_ROOT/.melon-monorepo exists: map each scope to that subfolder's AGENTS.md."
            echo "Otherwise (standalone repo): all scopes update \$REPO_ROOT/AGENTS.md only."
            echo ""
            echo "Options:"
            echo "  --dry-run    Show what would change without modifying files"
            echo "  --scope      Only sync specific scope"
            echo "  --vendor     Run vendor-skills.sh for this repo first (needs network)"
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

if $RUN_VENDOR; then
    VENDOR_SH="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/vendor-skills.sh"
    if [ -f "$VENDOR_SH" ]; then
        echo -e "${BLUE}Running vendor-skills.sh...${NC}"
        bash "$VENDOR_SH" --target-skills-dir "$REPO_ROOT/ai-specs/skills" || exit 1
    else
        echo -e "${YELLOW}vendor-skills.sh not found; skipping --vendor${NC}"
    fi
fi

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

# Extract YAML frontmatter field using awk
extract_field() {
    local file="$1"
    local field="$2"
    awk -v field="$field" '
        /^---$/ { in_frontmatter = !in_frontmatter; next }
        in_frontmatter && $1 == field":" {
            # Handle single line value
            sub(/^[^:]+:[[:space:]]*/, "")
            if ($0 != "" && $0 != ">") {
                gsub(/^["'\'']|["'\'']$/, "")  # Remove quotes
                print
                exit
            }
            # Handle multi-line value
            getline
            while (/^[[:space:]]/ && !/^---$/) {
                sub(/^[[:space:]]+/, "")
                printf "%s ", $0
                if (!getline) break
            }
            print ""
            exit
        }
    ' "$file" | sed 's/[[:space:]]*$//'
}

# Extract nested metadata field
#
# Supports either:
#   auto_invoke: "Single Action"
# or:
#   auto_invoke:
#     - "Action A"
#     - "Action B"
#
# For list values, this returns a pipe-delimited string: "Action A|Action B"
extract_metadata() {
    local file="$1"
    local field="$2"

    awk -v field="$field" '
        function trim(s) {
            sub(/^[[:space:]]+/, "", s)
            sub(/[[:space:]]+$/, "", s)
            return s
        }

        /^---$/ { in_frontmatter = !in_frontmatter; next }

        in_frontmatter && /^metadata:/ { in_metadata = 1; next }
        in_frontmatter && in_metadata && /^[a-z]/ && !/^[[:space:]]/ { in_metadata = 0 }

        in_frontmatter && in_metadata && $1 == field":" {
            # Remove "field:" prefix
            sub(/^[^:]+:[[:space:]]*/, "")

            # Single-line scalar: auto_invoke: "Action"
            if ($0 != "") {
                v = $0
                gsub(/^["'\'']|["'\'']$/, "", v)
                gsub(/^\[|\]$/, "", v)  # legacy: allow inline [a, b]
                print trim(v)
                exit
            }

            # Multi-line list:
            # auto_invoke:
            #   - "Action A"
            #   - "Action B"
            out = ""
            while (getline) {
                # Stop when leaving metadata block
                if (!in_frontmatter) break
                if (!in_metadata) break
                if ($0 ~ /^[a-z]/ && $0 !~ /^[[:space:]]/) break

                # On multi-line list, only accept "- item" lines. Anything else ends the list.
                line = $0
                # Stop at frontmatter delimiter (getline bypasses pattern matching)
                if (line ~ /^---$/) break
                if (line ~ /^[[:space:]]*-[[:space:]]*/) {
                    sub(/^[[:space:]]*-[[:space:]]*/, "", line)
                    line = trim(line)
                    gsub(/^["'\'']|["'\'']$/, "", line)
                    if (line != "") {
                        if (out == "") out = line
                        else out = out "|" line
                    }
                } else {
                    break
                }
            }

            if (out != "") print out
            exit
        }
    ' "$file"
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

    skill_name=$(extract_field "$skill_file" "name")
    scope_raw=$(extract_metadata "$skill_file" "scope")

    auto_invoke_raw=$(extract_metadata "$skill_file" "auto_invoke")
    # extract_metadata() returns:
    # - single action: "Action"
    # - multiple actions: "Action A|Action B" (pipe-delimited)
    # We use ';;' as separator to avoid conflicts with '|' used between entries.
    auto_invoke=$(echo "$auto_invoke_raw" | sed 's/|/;;/g')

    # Skip if no scope or auto_invoke defined
    [ -z "$scope_raw" ] || [ -z "$auto_invoke" ] && continue

    # Parse scope (can be comma-separated or space-separated)
    # Bash 3 compatible: use tr + read instead of read -ra with <<<
    echo "$scope_raw" | tr ', ' '\n' | while read -r scope; do
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
    skill_name=$(extract_field "$skill_file" "name")
    scope_raw=$(extract_metadata "$skill_file" "scope")
    auto_invoke_raw=$(extract_metadata "$skill_file" "auto_invoke")
    auto_invoke=$(echo "$auto_invoke_raw" | sed 's/|/;;/g')

    if [ -z "$scope_raw" ] || [ -z "$auto_invoke" ]; then
        echo -e "  ${YELLOW}$skill_name${NC} - missing: ${scope_raw:+}${scope_raw:-scope} ${auto_invoke:+}${auto_invoke:-auto_invoke}"
        missing=$((missing + 1))
    fi
done < <(list_all_skill_md)

if [ $missing -eq 0 ]; then
    echo -e "  ${GREEN}All skills have sync metadata${NC}"
fi
