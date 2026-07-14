Depth: full

# Tasks: cli-bound-recipes

> Engram-only. Locked: AI_SPECS_HOME cache; resolved-skills in cache; legacy version ignore+WARN; leftover rm; list info-only; fan-out unchanged; keep skills/+recipes/; #104 WARN only; no pin-bump. Supersedes recipe-update-flow.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~900–1400 monolithic; ~200–350 per slice |
| 400-line budget risk | High (mono) / Medium (per slice) |
| Chained PRs recommended | Yes |
| Suggested split | PR1→PR2→PR3→PR4 (design slices) |
| Delivery strategy | ask-on-risk |
| Chain strategy | feature-branch-chain |

Decision needed before apply: Yes
Chained PRs recommended: Yes
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Manifest unpin + WARN + add/init/list | PR 1 | base=`development` (or feature/tracker); tests+docs |
| 2 | Cache module + skill/dep origin + leftover cleanup | PR 2 | base=PR1 branch |
| 3 | Commands merge + flatten + sync-agent + init/gitignore | PR 3 | base=PR2 branch |
| 4 | Doctor/hub/docs/#104 + on-disk spec rewrite | PR 4 | base=PR3 branch |

**Authorization gate:** STOP after tasks. Do not implement until human authorizes slices + chain strategy.

---

## Phase 1 — PR1: Manifest unpin + WARN + add/init/list

- [x] 1.1 RED: tests — sync without `version` succeeds; legacy `version` WARN+succeed; add/init omit `version`; list catalog version info-only (not outdated)
- [x] 1.2 GREEN: delete `validate_version_pin` in `recipe-materialize.py`; `toml-read.py` make `version` optional + expose for WARN
- [x] 1.3 GREEN: `recipe-add.py`, `recipe-init.py`, `recipe-config-write.py`, `init_tui.py` stop writing `version=`
- [x] 1.4 GREEN: `recipe-list.py` catalog version info-only; no outdated/pin-bump path
- [x] 1.5 Docs/tmpl: `templates/ai-specs.toml.tmpl` + brief README/recipe-schema note (no pin ceremony)
- [x] 1.6 Fixture sweep for PR1: drop required `version=` where tests assert pin fail-close
- [x] 1.7 Validate: `./tests/validate.sh` green for PR1 scope

## Phase 2 — PR2: Cache module + skill/dep origin + leftover cleanup

- [ ] 2.1 RED: `project-cache` unit — `cache_key` stable; `meta.toml` sidecar; path helpers; leftover `.recipe`/`.deps` deleted; override migrate before rm
- [ ] 2.2 GREEN: create `lib/_internal/project-cache.py` (`cache_key`, `cache_root`, `ensure_cache`, roots, `remove_legacy_origin`)
- [ ] 2.3 RED: materialize/vendor/skill-resolution resolve under cache; orphans clean via resolver; no skills pollution
- [ ] 2.4 GREEN: `recipe-materialize.py` stage `.recipe` to cache; WARN legacy; call leftover rm + override migrate → `ai-specs/recipes/<id>/overrides/`
- [ ] 2.5 GREEN: `vendor-skills.py` → cache `.deps`; `skill-resolution.py` scan cache tiers + overrides path
- [ ] 2.6 Extend `test_external_dirs` / materialize fixtures for cache paths; validate PR2

## Phase 3 — PR3: Commands merge + flatten + sync-agent + init/gitignore

- [ ] 3.1 RED: command merge — cache cmds + `ai-specs/commands/`; local wins; fan-out targets unchanged
- [ ] 3.2 GREEN: materialize managed cmds → cache `commands/`; merge helper callable from sync
- [ ] 3.3 RED: flatten dest = cache `resolved-skills/`; sync-agent uses cache flatten + merge
- [ ] 3.4 GREEN: `flatten-resolved-skills.py` + `sync-agent.sh` wire cache paths
- [ ] 3.5 GREEN: `init.sh` stop mkdir in-project `.recipe`/`.deps`; `gitignore-render.py` drop those + `.internal/resolved-skills` ignores
- [ ] 3.6 Integration: extend `test_sync_pipeline` full sync→fan-out; validate PR3

## Phase 4 — PR4: Doctor/hub/docs/#104 + on-disk specs

- [ ] 4.1 RED/GREEN: `doctor.py`/`hub.py` — legacy-version WARN / resync note; no outdated-pin UX
- [ ] 4.2 Docs: #104 WARN/note only (no template refresh); troubleshooting + recipes-catalog
- [ ] 4.3 Rewrite on disk: `openspec/specs/external-dirs-layout/spec.md`; delta `recipe-manifest-contract`, `recipe-cli`, `recipe-overrides-runtime` (+ skill-source if present)
- [ ] 4.4 Final fixture sweep + `./tests/validate.sh` full green

## Locked assumptions (do not reopen)

Cache under AI_SPECS_HOME; resolved-skills in cache; legacy ignore+WARN; leftover rm; list info-only; fan-out unchanged; keep skills/+recipes/; #104 WARN only; no pin-bump recipe update.

## Open questions (non-blocking)

- Exact WARN string copy
- Cache key basename sanitization charset
- Whether gitignore keeps temporary `.recipe/` patterns one release (prefer remove in PR3)

## Next

**Stop for human authorization** before `sdd-apply`. Confirm chain strategy + authorize PR1 start.
