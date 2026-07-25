# Tasks: harness-hook-tool-name-matching

Depth: full

Trello: [#53](https://trello.com/c/mMtm3KhA)

## Planning

- [x] exploration.md
- [x] proposal.md
- [x] design.md (includes D ground-truth finding)
- [x] specs/runtime-hook-distribution/spec.md
- [x] specs/worktree-flow/spec.md

## Implementation (red-green-refactor)

- [ ] RED: extend `tests/test_hooks_render.py` so OpenCode shim asserts
      `new RegExp(\`^(?:${MATCHER})$\`, "i")` (expect fail before fix)
- [ ] GREEN: add `"i"` flag + comment in `render_opencode` (`hooks-render.py`)
- [ ] Docs: update `docs/runtime-hooks.md` (omp rows, honest pi/omp status,
      known-gaps subprocess bullet)
- [ ] Brief: add pre-delegation `workflow_rules` entry in
      `catalog/recipes/worktree-flow/recipe.toml`
- [ ] Skill + README: mirror the pre-delegation guidance in
      `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` and
      `catalog/recipes/worktree-flow/README.md`
- [ ] Promote spec deltas into `openspec/specs/runtime-hook-distribution/spec.md`
      and `openspec/specs/worktree-flow/spec.md`
- [ ] CHANGELOG Unreleased entry (Fixed / Changed as appropriate)

## Validation

- [ ] Focused: `python3 -m unittest tests.test_hooks_render -v`
- [ ] Full: `./tests/validate.sh`
- [ ] verify-report.md with RED/GREEN evidence

## Close

- [x] Archive change folder on review branch
- [ ] Open PR → development (link Trello #53)
- [ ] Judgment Day
