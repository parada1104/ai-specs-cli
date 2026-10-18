# MCP Distribution

How `[mcp.<name>]` entries in `ai-specs/ai-specs.toml` are rendered into
each agent's native config format via a merge-safe strategy.

`ai-specs` owns the MCP key (e.g. `mcpServers`) in the target file and
preserves every other top-level key the file already contains.

| Agent    | Target file              | Key            | Format | Notes |
|----------|--------------------------|----------------|--------|-------|
| Claude   | `.mcp.json`              | `mcpServers`   | JSON   | per-project |
| Cursor   | `.cursor/mcp.json`       | `mcpServers`   | JSON   | merge preserves other keys |
| OpenCode | `opencode.json`          | `mcp`          | JSON   | translated to OpenCode native schema (`type:"local"`, `command:[…]`, `environment:{…}`, `{env:VAR}`) |
| Codex    | `.codex/config.toml`     | `mcp_servers`  | TOML   | rewrites `[mcp_servers.*]` blocks only |
| Gemini   | `.gemini/settings.json`  | `mcpServers`   | JSON   | |
| Pi       | `.pi/mcp.json`           | `mcpServers`   | JSON   | Pi resolves project servers from `.pi/mcp.json` only; the root `.mcp.json` is Claude Code's file and pi ignores it |
| Oh My Pi | `.omp/mcp.json`          | `mcpServers`   | JSON   | dedicated file, same reason as pi |
| Copilot  | (no MCP support)         | —              | —      | reads AGENTS.md only |
