# Proposal: sync-env-scaffold

- **Tier**: standard (design N/A per supervisor-approved standard depth)
- **Status**: implemented and verified (see verify-report.md)

## What changes

1. Wire `env_scaffold` into the sync pipeline (after vendored skills, before
   AGENTS.md render): regenerate the root `ai-specs.env.example` from enabled
   recipes with required env values; ensure a managed `.envrc` block
   idempotently; warn non-fatally on missing required values.
2. Remove the deprecated `ai-specs/.env` stub path and nested examples — sync
   never creates them; the interactive offer path was already retired.
3. Amend the canonical `openspec/specs/harness-env-scaffold/spec.md`:
   SHALL-NOT-create wording replaces the deprecated-stub sentence; a new
   requirement section mirrors the delta.

## Exit criteria

- Resync idempotency holds (ResyncIdempotencyTests green).
- Sync never writes secrets or a project `.env`; no example files under `ai-specs/`.
- Focused suites green (env_scaffold 50 tests) and full validate green
  (1876 tests attested; exit 0 on the final tree).

## Tracker

- card_id: — (pre-dates the current tracker gate; tracked under the follow-up
  session that closed it: card #112 workflow, 2026-08-29)
