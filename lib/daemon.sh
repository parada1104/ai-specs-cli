#!/usr/bin/env bash
# daemon.sh — control the per-git-root mcp-proxy daemon.
#
# Usage:
#   ai-specs daemon stop      Send SIGTERM to the running daemon (if any).
#   ai-specs daemon status    Print pid/port/uptime JSON or "no daemon running".
#   ai-specs daemon restart   Stop the daemon (if any) then start a new one
#                             using the named-config under the git root.
#
# The daemon identity is the git toplevel of the current working directory,
# so every worktree of the same repository shares a single mcp-proxy.
#
# The Python module owns the lifecycle; this wrapper only resolves git_root
# and delegates. The module filename contains a dash, so direct-path
# invocation is used (NOT `python3 -m`).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="${AI_SPECS_HOME:-$(cd "$SCRIPT_DIR/.." && pwd)}"
DAEMON_PY="$AI_SPECS_HOME/lib/_internal/mcp-daemon.py"

usage() {
    cat <<'EOF'
Usage: ai-specs daemon <subcommand>
Subcommands:
  stop      Stop the running mcp-proxy daemon (if any).
  status    Print daemon pid/port/uptime, or exit 1 if no daemon is running.
  restart   Stop the current daemon (if any) and start a new one.
EOF
}

subcmd="${1:-}"
shift || true

case "$subcmd" in
    --help|-h|help) usage; exit 0 ;;
    "") usage >&2; exit 2 ;;
    stop|status|restart) ;;
    *) echo "ai-specs daemon: unknown subcommand '$subcmd'" >&2
       usage >&2
       exit 2 ;;
esac

if ! GIT_ROOT="$(git rev-parse --show-toplevel 2>/dev/null)"; then
    echo "ERROR: ai-specs daemon must run inside a git repository." >&2
    echo "       Run 'git init' in the project root first." >&2
    exit 1
fi

case "$subcmd" in
    stop)
        exec python3 "$DAEMON_PY" stop "$GIT_ROOT"
        ;;
    status)
        exec python3 "$DAEMON_PY" status "$GIT_ROOT"
        ;;
    restart)
        NAMED_CONFIG="$GIT_ROOT/.ai-specs/run/proxy.named-config.json"
        if [[ ! -f "$NAMED_CONFIG" ]]; then
            echo "ERROR: missing named-config at $NAMED_CONFIG." >&2
            echo "       Run 'ai-specs sync' first to materialize the shared MCP config." >&2
            exit 1
        fi
        exec python3 "$DAEMON_PY" restart "$GIT_ROOT" --named-config "$NAMED_CONFIG"
        ;;
esac
