# tracker-snapshot-on-close (card #131, S1)

Goal: closing a ledger item records the observed provider snapshot instead of persisting a stale link-time snapshot.

## Tasks

- [x] T1 RED: `TestApplyWriteCloseRecordsObservedSnapshot` fails (state stayed "in-progress")
- [x] T1 GREEN: `WriteClose` with payload calls new `Store.RecordSnapshot`; canonical provider normalization reused; bare closes unchanged (`TestApplyWriteCloseWithoutPayloadKeepsSnapshot`)
- [x] T2: explicit close post-grade reports the just-closed snapshot (`ReportItem`/`LatestClosed`) without changing `Primary` or D17; retries work after archive and multiple active folders
- [x] Full ledger package + gate main package + vet green; focused CLI/selector regressions pass
- [x] T3: full validation at deliverable boundary — 2029 tests OK, skipped=2; combined candidate committed

## Notes

- Snapshot update is opt-in via payload; no payload invents or clears nothing.
- No reopen path, no automatic provider writes (D17 unchanged).

## Tracker

- **card_id**: 6aab1f85b36328d49ad56bd5
- **url**: https://trello.com/c/J9aNJFGr

## Combined delivery

This S1 fix is delivered together with cards #130, #132, and the live acceptance card #133 in `feat/tracker-reconcile-adoption`.
