# Shared MCP daemon (`mcp-proxy`)

`ai-specs` can multiplex any MCP server declared with `mode = "shared"` through a single
`mcp-proxy` daemon per git root, instead of letting every agent (Claude, Cursor, OpenCode,
…) spawn its own stdio subprocess for the same MCP across every worktree. The agents that
speak HTTP connect to `http://localhost:<port>/servers/<name>/mcp`; agents without HTTP
(Codex, Gemini) keep the stdio fallback.

## When does the daemon start?

`ai-specs sync` writes `<git-root>/.ai-specs/run/proxy.named-config.json` whenever
materialization resolves at least one MCP with `mode = "shared"`. The presence of that
file is the trigger: `sync.sh` calls `ensure mcp-proxy daemon` immediately after
materialize and before the per-agent fan-out. If no shared MCP is declared (or the merged
manifest demotes the only candidate to `mode = "stdio"`), the run directory is not
created and the daemon step is skipped entirely.

## Runtime dependency: `uvx`

`mcp-proxy` is invoked through `uvx mcp-proxy` so users do not need a global install.
Doctor (`ai-specs doctor`) emits an `ERROR` when shared MCPs are declared but `uvx` is
missing from `PATH`. `ai-specs sync` degrades gracefully in the same scenario: it emits a
`WARN`, demotes the shared MCPs to stdio for the duration of that sync (without touching
the manifest), and completes with exit 0.

Install `uv` (which ships `uvx`):

```bash
curl -LsSf https://astral.sh/uv/install.sh | sh
```

## CLI

```text
ai-specs daemon status     # JSON dump or "no daemon running"
ai-specs daemon stop       # SIGTERM + cleanup state files
ai-specs daemon restart    # stop-and-spawn against the current named-config
```

`ai-specs daemon restart` requires that a prior `ai-specs sync` has produced the
`proxy.named-config.json` (the daemon does not invent its own config). If you have not
synced yet, run `ai-specs sync` instead — it brings the daemon up as part of the
pipeline.

## State files

All daemon state lives under `<git-root>/.ai-specs/run/`:

| File | Contents | Owned by |
| --- | --- | --- |
| `proxy.named-config.json` | The merged MCP set with secrets resolved (chmod 0600). | `recipe-materialize.py` |
| `proxy.pid` | PID of the running `mcp-proxy` process. | `mcp-daemon.py` |
| `proxy.port` | Loopback TCP port the daemon listens on. | `mcp-daemon.py` |
| `proxy.config-hash` | SHA-256 of the canonical named-config, used to detect changes. | `mcp-daemon.py` |
| `proxy.lock` | `fcntl` exclusive lock that serialises `ensure_daemon`. | `mcp-daemon.py` |
| `proxy.log` | stdout/stderr of the daemon (no rotation). | `mcp-daemon.py` |

The pattern `.ai-specs/run/` is included in the gitignore template that `ai-specs init`
appends to the project's root `.gitignore`. Re-run `ai-specs init --force` on existing
projects to refresh the managed block.

## Known mcp-proxy behaviours (verified empirically)

These were the post-design open questions Q1 and Q2; both were validated against a live
`uvx mcp-proxy` during the implementation of this feature.

### Crash semantics — Q1

`mcp-proxy` initialises every named server **once** at startup (it sends a single MCP
initialisation handshake to each stdio child and then exposes the HTTP endpoints).
No restart loop has been observed: if an internal stdio MCP crashes after the proxy is
serving, subsequent client calls receive the upstream error rather than a silently
revived child.

Recovery is manual and cheap:

```bash
ai-specs daemon restart
```

The next `ai-specs sync` would also restart the daemon if the named-config hash differs,
but the explicit `restart` is the right tool when you just want to re-cycle the children
without changing anything else. The `daemon-running` doctor check WARNs when state files
exist but the proxy is unreachable, so a stale daemon shows up loudly during the next
diagnostic pass.

### `/status` payload — Q2

The endpoint returns structured JSON:

```json
{
  "api_last_activity": "2026-05-25T07:07:06.006025+00:00",
  "server_instances": {
    "trello": "configured",
    "github": "configured"
  }
}
```

`ai-specs daemon status` surfaces this directly. The returned dict now includes
`api_last_activity` (proxy-side last-touched timestamp) and `servers` (the
`server_instances` mapping) in addition to the existing `pid`, `port`, and
`uptime_s`. If the daemon is up but the `/status` endpoint is briefly unreachable
(starting up, hung), the base shape is returned unchanged.

The `servers` mapping reports the proxy's view of each hosted MCP. Today every entry is
`"configured"`; future `mcp-proxy` versions may add richer states (e.g. `"initialising"`,
`"failed"`). The caller treats unknown keys/values as opaque.

## Port allocation and the port race

`ensure_daemon` asks the kernel for a free ephemeral port (`socket().bind(('', 0))`),
records it in `proxy.port`, then spawns `mcp-proxy --port <p>`. There is a microscopic
window during which the kernel could hand the same port to another process. This window
is auto-corrected on the **next** sync: the healthcheck (`GET /status` against the port)
returns False, `ensure_daemon` SIGTERMs the dead PID, picks a fresh port, and respawns.
This is the same recovery branch that handles a daemon killed externally; it is exercised
by `tests/test_daemon_dead_pid_recovery.py` and the staleness tests in
`tests/test_mcp_daemon_ensure.py`.

If you ever need to force a fresh port immediately:

```bash
ai-specs daemon restart
```
