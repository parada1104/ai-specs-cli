#!/usr/bin/env bash
# platform.sh — paths/keys por agente.
# Source: source "ai-specs/cli/lib/platform.sh"
#
# Cada agente está descripto por sus paths/capacidades nativas. Acceso vía
# función `platform_get`.
#
# Inspirado en charliesbot/chai internal/platform/platform.go pero adaptado
# a per-project (paths relativos al REPO_ROOT, no a $HOME).

# platform_get <agent> <field>
#   agent ∈ claude|cursor|opencode|codex|copilot|gemini|pi|omp
#   field ∈ instructions_path|skills_dir|agents_dir|mcp_config_path|mcp_key|native
#         | commands_dir|runtime_hooks_target
#
# `commands_dir` is the directory where slash-command files (like
# `/skills-as-rules`) get written. Empty string means the agent has no native
# slash-command UX and we skip the fan-out for it.
#
# `runtime_hooks_target` is a short descriptor of where this agent's
# recipe-declared runtime hooks ([[provides.hooks]]) are rendered. The value is
# the project-relative file or dir owned by hooks-render.py:
#   claude   → .claude/settings.json   (managed block, script wired directly)
#   cursor   → .cursor/hooks.json      (managed entry + generated wrapper in .cursor/hooks/)
#   opencode → .opencode/plugin        (generated TS plugin per hook)
#   pi       → .pi/extensions          (generated TS extension per hook)
# Empty string means the agent has no runtime-hook target and hooks-render.py
# skips it.
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
                runtime_hooks_target) echo ".claude/settings.json" ;;
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
                runtime_hooks_target) echo ".cursor/hooks.json" ;;
                *) return 1 ;;
            esac
            ;;
        opencode)
            # OpenCode reads AGENTS.md natively and supports project-local
            # skills/commands under .opencode/.
            case "$field" in
                instructions_path) echo "" ;;
                skills_dir)        echo ".opencode/skills" ;;
                agents_dir)        echo "" ;;
                mcp_config_path)   echo "opencode.json" ;;
                mcp_key)           echo "mcp" ;;
                native)            echo "true" ;;
                commands_dir)      echo ".opencode/commands" ;;
                runtime_hooks_target) echo ".opencode/plugin" ;;
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
                runtime_hooks_target) echo "" ;;
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
                runtime_hooks_target) echo "" ;;
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
                runtime_hooks_target) echo "" ;;
                *) return 1 ;;
            esac
            ;;
        pi)
            # Pi (pi.dev) — reads AGENTS.md natively, uses .mcp.json with mcpServers.
            case "$field" in
                instructions_path) echo "" ;;
                skills_dir)        echo ".pi/skills" ;;
                agents_dir)        echo "" ;;
                mcp_config_path)   echo ".mcp.json" ;;
                mcp_key)           echo "mcpServers" ;;
                native)            echo "true" ;;
                commands_dir)      echo "" ;;  # pi has no slash commands
                runtime_hooks_target) echo ".pi/extensions" ;;
                *) return 1 ;;
            esac
            ;;
        omp)
            # Oh My Pi (can1357/oh-my-pi) — Rust pi fork; AGENTS.md native, .omp/ root.
            # KEY DELTA vs pi: commands_dir is populated (.omp/commands); dedicated mcp path.
            case "$field" in
                instructions_path)    echo "" ;;
                skills_dir)           echo ".omp/skills" ;;
                agents_dir)           echo "" ;;
                mcp_config_path)      echo ".omp/mcp.json" ;;   # dedicated; avoids .mcp.json clash with pi
                mcp_key)              echo "mcpServers" ;;
                native)               echo "true" ;;
                commands_dir)         echo ".omp/commands" ;;   # KEY DELTA vs pi ("")
                runtime_hooks_target) echo ".omp/extensions" ;;
                *) return 1 ;;
            esac
            ;;
        *)
            return 1
            ;;
    esac
}
