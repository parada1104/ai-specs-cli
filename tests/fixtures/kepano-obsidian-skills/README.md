# kepano/obsidian-skills offline fixture

Stands in for `https://github.com/kepano/obsidian-skills.git` when
`AI_SPECS_VENDOR_FIXTURE_ROOT` is set, so vendor tests never hit the network.

Two layouts on purpose:

- `plugins/obsidian/skills/<id>` mirrors upstream (the recipe's declared paths
  since upstream moved its skills there).
- `skills/<id>` is the older upstream layout. It stays because the frozen
  vendor goldens (`internal/sync/testdata/vendorskills_golden.json`,
  `internal/cli/testdata/skills_add_golden.json`) reference a generic
  `path = 'skills/obsidian-markdown'` / `--subdir skills/defuddle` dep, and
  regenerating those snapshots needs a human decision.
