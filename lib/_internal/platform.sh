#!/usr/bin/env bash
# platform.sh — paths/keys por agente.
# Source: source "ai-specs/cli/lib/platform.sh"
#
# Cada agente está descripto por 5 valores: instructions_path, skills_dir,
# agents_dir, mcp_config_path, mcp_key. Acceso vía función `platform_get`.
#
# Inspirado en charliesbot/chai internal/platform/platform.go pero adaptado
# a per-project (paths relativos al REPO_ROOT, no a $HOME).

# platform_get <agent> <field>
#   agent ∈ claude|cursor|opencode|codex|copilot|gemini
#   field ∈ instructions_path|skills_dir|agents_dir|mcp_config_path|mcp_key|native
#         | commands_dir
#
# `commands_dir` is the directory where slash-command files (like
# `/skills-as-rules`) get written. Empty string means the agent has no native
# slash-command UX and we skip the fan-out for it.
#
# Imprime el valor en stdout. Exit 1 si agent/field desconocidos.
platform_get() {
    local agent="$1"
    local field="$2"

    case "$agent" in
        claude)
            case "$field" in
                instructions_path) echo "CLAUDE.md" ;;
                skills_dir)        echo ".claude/skills" ;;
                agents_dir)        echo ".claude/agents" ;;
                mcp_config_path)   echo ".mcp.json" ;;
                mcp_key)           echo "mcpServers" ;;
                native)            echo "false" ;;
                commands_dir)      echo ".claude/commands" ;;
                *) return 1 ;;
            esac
            ;;
        cursor)
            # Cursor reads AGENTS.md natively at root and has no native
            # skill auto-invocation (skills are referenced via AGENTS.md table).
            case "$field" in
                instructions_path) echo "" ;;
                skills_dir)        echo "" ;;
                agents_dir)        echo "" ;;
                mcp_config_path)   echo ".cursor/mcp.json" ;;
                mcp_key)           echo "mcpServers" ;;
                native)            echo "true" ;;
                commands_dir)      echo ".cursor/commands" ;;
                *) return 1 ;;
            esac
            ;;
        opencode)
            # OpenCode reads AGENTS.md natively at root; no native skill dir.
            case "$field" in
                instructions_path) echo "" ;;
                skills_dir)        echo "" ;;
                agents_dir)        echo "" ;;
                mcp_config_path)   echo "opencode.json" ;;
                mcp_key)           echo "mcp" ;;
                native)            echo "true" ;;
                commands_dir)      echo ".opencode/command" ;;
                *) return 1 ;;
            esac
            ;;
        codex)
            # Codex reads AGENTS.md natively at root; no native skill dir.
            case "$field" in
                instructions_path) echo "" ;;
                skills_dir)        echo "" ;;
                agents_dir)        echo "" ;;
                mcp_config_path)   echo ".codex/config.toml" ;;
                mcp_key)           echo "mcp_servers" ;;
                native)            echo "true" ;;
                commands_dir)      echo "" ;;  # codex has no slash commands
                *) return 1 ;;
            esac
            ;;
        copilot)
            case "$field" in
                instructions_path) echo ".github/copilot-instructions.md" ;;
                skills_dir)        echo "" ;;  # no skills dir nativo
                agents_dir)        echo "" ;;
                mcp_config_path)   echo "" ;;  # no MCP nativo
                mcp_key)           echo "" ;;
                native)            echo "true" ;;
                commands_dir)      echo "" ;;  # copilot has no slash commands
                *) return 1 ;;
            esac
            ;;
        gemini)
            case "$field" in
                instructions_path) echo "GEMINI.md" ;;
                skills_dir)        echo ".gemini/skills" ;;
                agents_dir)        echo ".gemini/agents" ;;
                mcp_config_path)   echo ".gemini/settings.json" ;;
                mcp_key)           echo "mcpServers" ;;
                native)            echo "false" ;;
                commands_dir)      echo "" ;;  # gemini has no slash commands
                *) return 1 ;;
            esac
            ;;
        *)
            return 1
            ;;
    esac
}
