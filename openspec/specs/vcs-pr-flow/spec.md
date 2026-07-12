[spec.md#F967]
1:# vcs-pr-flow Specification: Multi-Provider VCS Flow
2:
3:## Purpose
4:
5:Provide provider-backed `vcs-pr-flow` recipes that mirror the same semantics across GitHub,
6:GitLab, and Bitbucket: explicit branch pushes, review-gated merging, and worktree cleanup.
7:The bound recipe id is the provider identity; only `base_branch` is configurable per project.
8:
9:## Requirements
10:
11:### Requirement: VCS Sibling Recipe Manifests
12:
13:Each VCS sibling recipe (`git-pr-flow`, `gitlab-mr-flow`, `bitbucket-pr-flow`) MUST declare
14:`vcs-pr-flow`, an `on-sync` `validate-config` hook, a bundled host-specific merge workflow
15:skill, a host-specific create command, README doc provision, and **only** `base_branch` as
16:config (no `provider` key).
17:
18:#### Scenario: GitHub manifest validates
19:- GIVEN the `git-pr-flow` catalog recipe is loaded
20:- WHEN recipe schema validation runs
21:- THEN the recipe is valid and declares `vcs-pr-flow`
22:- AND `base_branch` defaults to `main`
23:- AND no `provider` field exists in `[config]`
24:
25:#### Scenario: GitLab manifest validates
26:- GIVEN the `gitlab-mr-flow` catalog recipe is loaded
27:- WHEN recipe schema validation runs
28:- THEN the recipe is valid and declares `vcs-pr-flow`
29:- AND `base_branch` defaults to `development`
30:- AND no `provider` field exists in `[config]`
31:
32:#### Scenario: Bitbucket manifest validates
33:- GIVEN the `bitbucket-pr-flow` catalog recipe is loaded
34:- WHEN recipe schema validation runs
35:- THEN the recipe is valid and declares `vcs-pr-flow`
36:- AND `base_branch` defaults to `development`
37:- AND no `provider` field exists in `[config]`
38:
39:### Requirement: Materialized Assets
40:
41:Sync MUST materialize provider assets without changing sibling provider recipe assets when
42:only one provider recipe is enabled.
43:
44:#### Scenario: GitLab sync provisions assets
45:- GIVEN `gitlab-mr-flow` is enabled
46:- WHEN `ai-specs sync` runs
47:- THEN the GitLab skill, command, and README exist in generated locations
48:
49:#### Scenario: Bitbucket sync provisions assets
50:- GIVEN `bitbucket-pr-flow` is enabled
51:- WHEN `ai-specs sync` runs
52:- THEN the Bitbucket skill, command, and README exist in generated locations
53:
54:### Requirement: Provider Binding Semantics
55:
56:When multiple recipes provide `vcs-pr-flow`, the system MUST require an explicit
57:`[[bindings]]` selection; without it, sync MUST warn and leave `vcs-pr-flow` unbound.
58:The bound **recipe id** is the provider identity; there is no separate `provider` config.
59:
60:#### Scenario: Ambiguous providers stay unbound
61:- GIVEN multiple VCS provider recipes are enabled without `[[bindings]]`
62:- WHEN sync resolves capabilities
63:- THEN a warning names the ambiguity
64:- AND no implicit `vcs-pr-flow` binding is selected
65:
66:#### Scenario: Explicit binding selects host
67:- GIVEN multiple VCS provider recipes are enabled with a binding to `bitbucket-pr-flow`
68:- WHEN sync resolves capabilities
69:- THEN `vcs-pr-flow` is bound to Bitbucket assets and brief rules
70:
71:### Requirement: Runtime Brief VCS Bullet
72:
73:The renderer MUST derive the Runtime Flow VCS provider bullet from the bound `vcs-pr-flow` recipe id, not from a `provider` config value.
74:If the bound recipe id is unknown to the VCS label table, it MUST emit a `⚠ ai-specs:` warning to stderr and render `VCS PR (custom)`.
75:It MUST append `base branch: \`<base_branch>\`` when `base_branch` is configured or defaulted.
76:(Previously: The bullet only mapped known recipe ids and appended base branch.)
77:
78:#### Scenario: GitHub binding renders gh hint
79:- GIVEN `bindings.vcs-pr-flow = "git-pr-flow"` and `base_branch = "development"`
80:- WHEN the brief is rendered
81:- THEN the Runtime Flow section includes `VCS/PR provider: GitHub` and `gh` CLI
82:- AND includes `base branch: \`development\``
83:
84:#### Scenario: Unknown recipe id warns and falls back
85:- GIVEN `bindings.vcs-pr-flow = "custom-pr-flow"`
86:- WHEN the brief is rendered
87:- THEN stderr includes `⚠ ai-specs:`
88:- AND the Runtime Flow section uses `VCS PR (custom)`
89:
90:#### Scenario: Multiple unknown ids each warn
91:- GIVEN two render passes bind different unknown `vcs-pr-flow` ids
92:- WHEN each brief is rendered
93:- THEN each pass emits one `⚠ ai-specs:` warning
94:- AND each pass uses `VCS PR (custom)`
95:
96:#### Scenario: Stale provider config ignored
97:- GIVEN a manifest still sets `[recipes.gitlab-mr-flow.config] provider = "github"`
98:- WHEN sync validates and renders
99:- THEN sync warns that `provider` is an unknown config key
100:- AND the rendered brief still identifies GitLab from the binding recipe id
101:
102:### Requirement: Runtime Checks and Docs
103:
104:Provider skills and commands MUST check CLI install/auth before PR/MR creation, stop with
105:actionable blockers on failure, and README MUST document enablement, config (`base_branch`
106:only), explicit bindings, runtime prerequisites, explicit push behavior, and no auto-merge
107:policy.
108:
109:### Requirement: Bound VCS Workflow Rules Stay Isolated
110:
111:The system MUST emit `workflow_rules` brief fragments only from the recipe bound to `vcs-pr-flow`.
112:Fragments from other enabled VCS sibling recipes MUST NOT appear when a binding exists.
113:
114:#### Scenario: One bound recipe among three enabled
115:- GIVEN `git-pr-flow`, `gitlab-mr-flow`, and `bitbucket-pr-flow` are enabled
116:- AND `vcs-pr-flow` is bound to `gitlab-mr-flow`
117:- WHEN the brief is rendered
118:- THEN only GitLab workflow rules appear
119:- AND GitHub and Bitbucket workflow rules do not appear
120:
121:#### Scenario: Single enabled bound recipe
122:- GIVEN only `git-pr-flow` is enabled and bound
123:- WHEN the brief is rendered
124:- THEN the GitHub workflow rules appear
125:- AND no other VCS workflow rules are added
126:
127:#### Scenario: No VCS binding exists
128:- GIVEN VCS sibling recipes are enabled
129:- AND `vcs-pr-flow` is unbound
130:- WHEN the brief is rendered
131:- THEN no VCS workflow rule fragments are emitted
132:
133:### Requirement: Git PR Flow Docs Omit Provider
134:
135:The `git-pr-flow` README and `docs/recipes-catalog.md` section for `git-pr-flow` MUST document `base_branch` only for config.
136:Neither document MAY include a `provider` config row.
137:
138:#### Scenario: README contract
139:- GIVEN `catalog/recipes/git-pr-flow/README.md`
140:- WHEN the docs contract is checked
141:- THEN the config table includes `base_branch`
142:- AND it does not include `provider`
143:
144:#### Scenario: Catalog contract
145:- GIVEN `docs/recipes-catalog.md`
146:- WHEN the `## git-pr-flow` section is checked
147:- THEN the config table includes `base_branch`
148:- AND it does not include `provider`
149:
150:### Requirement: Pre-merge archive artifacts

The system MUST archive and record SDD/OpenSpec artifacts before a VCS PR/MR is merged. The archive boundary MUST occur while the change is still on the review branch, not after the merge commit lands on the base branch.

#### Scenario: Archive runs before merge

- GIVEN a provider-backed PR/MR is ready to merge
- WHEN the archive step runs for the change
- THEN the change artifacts are persisted before merge completes
- AND the archive records the pre-merge state as the source of truth

#### Scenario: Post-merge archive is rejected

- GIVEN a PR/MR has already been merged into the base branch
- WHEN the archive step tries to treat the merged state as the archive boundary
- THEN the system rejects that interpretation
- AND the archive must reference the pre-merge branch state instead

#### Scenario: Provider behavior stays aligned

- GIVEN GitHub, GitLab, or Bitbucket provider flows are enabled
- WHEN the pre-merge archive rule is rendered into workflow guidance
- THEN the provider guidance matches the same archive-before-merge contract
- AND no provider introduces a different timing rule

#### Scenario: Hidden ceremony remains hidden

- GIVEN the user follows the normal plan/build flow
- WHEN the archive rule is applied
- THEN no new slash command or extra user-facing mode is introduced
- AND the archive step remains part of the existing invisible workflow

### Requirement: Test and Validation Commands Pass
151:
152:The implementation MUST pass `./tests/run.sh` and `./tests/validate.sh` with the change applied.
153:
154:#### Scenario: Focused run passes
155:- GIVEN the change is applied
156:- WHEN `./tests/run.sh` runs
157:- THEN it exits successfully
158:
159:#### Scenario: Full validation passes
160:- GIVEN the change is applied
161:- WHEN `./tests/validate.sh` runs
162:- THEN it exits successfully
163: