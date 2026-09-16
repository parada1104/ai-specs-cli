# tracker-snapshot-on-close (card #131, S1)

Goal: closing a ledger item records the observed provider snapshot instead of persisting a stale link-time snapshot.

## Tasks

- [x] T1 RED: `TestApplyWriteCloseRecordsObservedSnapshot` fails (state stayed "in-progress")
- [x] T1 GREEN: `WriteClose` with payload calls new `Store.RecordSnapshot`; canonical provider normalization reused; bare closes unchanged (`TestApplyWriteCloseWithoutPayloadKeepsSnapshot`)
- [x] Full ledger package + gate main package + vet green
- [ ] T2: full validation at deliverable boundary + commit

## Notes

- Snapshot update is opt-in via payload; no payload invents or clears nothing.
- No reopen path, no automatic provider writes (D17 unchanged).

## Tracker

- **card_id**: 6aab1f85b36328d49ad56bd5
- **url**: https://trello.com/c/J9aNJFGr
