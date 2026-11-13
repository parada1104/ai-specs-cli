# Tasks: go-parity-contract

Depth: light

Requested depth: tasks-only. Signal depth: light. Decided depth: light.

Rationale: the entire deliverable is one committed document. No production code,
no behavior change, no new runtime surface. A spec or design would restate the
document itself, so the tier minimum (proposal + tasks) is the correct depth.

## Tracker

- **card_id**: `6a84e758a52c749278855a14`
- **shortLink**: `wUldHID4`
- **url**: https://trello.com/c/wUldHID4
- **epic**: https://trello.com/c/qwlHQ7Xa

## Work units

- [x] **1. Enumerate the command surface.** All 14 verbs dispatched by
      `bin/ai-specs`: aliases, entry point, whether the verb writes, and whether
      it branches on TTY. Recorded as §1.
- [x] **2. Record the exit-code contracts.** Per-command code maps for `doctor`,
      `upgrade`, `recipe configure`, and `hub`, plus the cross-cutting `2` for
      unknown flags. Recorded as §2, classified FROZEN.
- [x] **3. Analyze the manifest write paths.** Five independent write paths, all
      text-based. Concluded that a whole-document TOML marshaller is not viable
      and a line/segment editor is required. Recorded as §3.
- [x] **4. Record the filesystem contracts.** Cache key derivation, ownership
      and preservation rules, symlink kinds and the non-symlink refusal.
      Recorded as §4, classified FROZEN.
- [x] **5. Map inter-module coupling.** `sync.sh` greps `recipe-materialize.py`
      stdout for structured data; the four compact-filter glyphs are the display
      contract. Recorded as §5.
- [x] **6. Audit read-only claims.** `doctor`, `refresh-bundled`, and `hub` are
      documented read-only and are not. Recorded as §6.
- [x] **7. Record TTY branching, the network/external-process surface, ordering
      guarantees, and environment variables.** Recorded as §7, §8, §10, §11.
- [x] **8. Classify every surface FROZEN / TOLERANT / FREE** with a rationale,
      and summarize what the port must reproduce exactly. Recorded as §12.
- [x] **9. File behavioral inconsistencies as defects, never normalized.**
      35 defects recorded (D1–D35), four of them data-loss class. Recorded as §9.
- [x] **10. Conclude with a written go/no-go.** **GO**, with three findings that
      materially change the epic plan.

## Verification

Documentation-only change. No code, no test surface, no behavior touched.

- `./tests/validate.sh` — unaffected; the change adds two Markdown files and
  modifies no executable path.
- Structural readback of `docs/go-migration-parity-contract.md` is the
  proportional check for a passive document at this tier.

## Outcome

Delivered as `docs/go-migration-parity-contract.md`. Four of the recorded
defects were filed as cards for tranche 2 of the epic; D1 and D5 are marked as
blocking [Go 03] so the parity harness does not freeze them as correct baseline
behavior.

## Branch and merge policy

Per the epic contract: branched from `epic/go-single-binary`, PR targets
`epic/go-single-binary`, never `development` or `main`.
