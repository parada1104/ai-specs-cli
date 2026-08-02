# trello-mcp-workflow live scenarios

Notes-file goldens (CI-safe load via harness smoke; live via `EVALS_LIVE=1`):

- `ac_new_change_writes_tracker_section`
- `ac_missing_card_gate_no_bash_skip` (`wire_hooks=true`)
- `ac_phase_transition_state_sync_plan`
- `ac_retro_change_without_card_triggers_link`

Run:

```bash
EVALS_LIVE=1 ./tests/evals/run-live-trello.sh
```

Optional expensive MCP-live board mutation (`ac_mcp_live_card_link`) is not
shipped in v1 — add a disposable-list scenario later with cleanup. Do not wire
into `./tests/validate.sh`.
