# tracker-recipe-default-mapping (card #130)

Goal: recipe-owned default event mapping so reconciliation works out of the box after one sync; project config is override-only.

## Tasks

- [x] T1 RED: recipe tests demand review_list/done_list defaults + reconcile expectations for review/merge events
- [x] T1 GREEN: recipe.toml extended; ConfigTable retains declared values; sync stamps recipe-declared reconcile + referenced fields into manifest (absent keys only)
- [x] T2: SKILL documents the recipe-supported events (review, merge, delivery) and override-only config
- [x] T3: dogfood — sync stamped reconcile block + review_list/done_list/default_list; epic_list and unrelated defaults untouched; manifest reverted per dogfood-verification-isolation (project state is evidence, not deliverable)
- [x] T4: full suite at deliverable boundary — 2029 tests OK, skipped=2; combined candidate committed after the post-close fix

## Tracker

- **card_id**: 6aab1f85f9522bc30e37f201
- **url**: https://trello.com/c/vm5gVI9P
