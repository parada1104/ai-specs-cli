# Tasks: materialization follow-up guidance

Depth: standard (spec + tasks)

Authorized: yes (user "ok demosle" after ownership analysis — CLI literacy + tracked-leftover WARN; no silent git rm).

Trello: https://trello.com/c/AfRD6P6O (#49)

## Implementation

- [x] RED/GREEN: agents-render harness pointer does not claim skills live under `ai-specs/skills/`
- [x] RED/GREEN: harness-lifecycle bundled skill documents cache flatten; no `.new` sidecar story
- [x] RED/GREEN: doctor WARNs when git still tracks removed CLI-bundled skill paths; guidance is `git rm -r --cached …`; never runs git rm
- [x] RED/GREEN: sync/refresh prints the same remediation after leftover removal when tracked paths remain
- [x] `./tests/validate.sh` green (1023 tests)

## Out of scope

- Auto `git rm` / commit
- Dogfood `[brief].context_sources` rewrite (manual)
- recipes/ untrack automation beyond the same WARN pattern if cheap; prefer skill leftovers first
