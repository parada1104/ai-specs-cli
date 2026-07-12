[spec.md#DE38]
1:[spec.md#F967]
2:1:# vcs-pr-flow Specification: Multi-Provider VCS Flow
3:2:
4:3:## Purpose
5:4:
6:5:Provide provider-backed `vcs-pr-flow` recipes that mirror the same semantics across GitHub,
7:6:GitLab, and Bitbucket: explicit branch pushes, review-gated merging, and worktree cleanup.
8:7:The bound recipe id is the provider identity; only `base_branch` is configurable per project.
9:8:
10:9:## Requirements
11:10:
12:11:### Requirement: VCS Sibling Recipe Manifests
13:12:
14:13:Each VCS sibling recipe (`git-pr-flow`, `gitlab-mr-flow`, `bitbucket-pr-flow`) MUST declare
15:14:`vcs-pr-flow`, an `on-sync` `validate-config` hook, a bundled host-specific merge workflow
16:15:skill, a host-specific create command, README doc provision, and **only** `base_branch` as
17:16:config (no `provider` key).
18:17:
19:18:#### Scenario: GitHub manifest validates
20:19:- GIVEN the `git-pr-flow` catalog recipe is loaded
21:20:- WHEN recipe schema validation runs
22:21:- THEN the recipe is valid and declares `vcs-pr-flow`
23:22:- AND `base_branch` defaults to `main`
24:23:- AND no `provider` field exists in `[config]`
25:24:
26:25:#### Scenario: GitLab manifest validates
27:26:- GIVEN the `gitlab-mr-flow` catalog recipe is loaded
28:27:- WHEN recipe schema validation runs
29:28:- THEN the recipe is valid and declares `vcs-pr-flow`
30:29:- AND `base_branch` defaults to `development`
31:30:- AND no `provider` field exists in `[config]`
32:31:
33:32:#### Scenario: Bitbucket manifest validates
34:33:- GIVEN the `bitbucket-pr-flow` catalog recipe is loaded
35:34:- WHEN recipe schema validation runs
36:35:- THEN the recipe is valid and declares `vcs-pr-flow`
37:36:- AND `base_branch` defaults to `development`
38:37:- AND no `provider` field exists in `[config]`
39:38:
40:39:### Requirement: Materialized Assets
41:40:
42:41:Sync MUST materialize provider assets without changing sibling provider recipe assets when
43:42:only one provider recipe is enabled.
44:43:
45:44:#### Scenario: GitLab sync provisions assets
46:45:- GIVEN `gitlab-mr-flow` is enabled
47:46:- WHEN `ai-specs sync` runs
48:47:- THEN the GitLab skill, command, and README exist in generated locations
49:48:
50:49:#### Scenario: Bitbucket sync provisions assets
51:50:- GIVEN `bitbucket-pr-flow` is enabled
52:51:- WHEN `ai-specs sync` runs
53:52:- THEN the Bitbucket skill, command, and README exist in generated locations
54:53:
55:54:### Requirement: Provider Binding Semantics
56:55:
57:56:When multiple recipes provide `vcs-pr-flow`, the system MUST require an explicit
58:57:`[[bindings]]` selection; without it, sync MUST warn and leave `vcs-pr-flow` unbound.
59:58:The bound **recipe id** is the provider identity; there is no separate `provider` config.
60:59:
61:60:#### Scenario: Ambiguous providers stay unbound
62:61:- GIVEN multiple VCS provider recipes are enabled without `[[bindings]]`
63:62:- WHEN sync resolves capabilities
64:63:- THEN a warning names the ambiguity
65:64:- AND no implicit `vcs-pr-flow` binding is selected
66:65:
67:66:#### Scenario: Explicit binding selects host
68:67:- GIVEN multiple VCS provider recipes are enabled with a binding to `bitbucket-pr-flow`
69:68:- WHEN sync resolves capabilities
70:69:- THEN `vcs-pr-flow` is bound to Bitbucket assets and brief rules
71:70:
72:71:### Requirement: Runtime Brief VCS Bullet
73:72:
74:73:The renderer MUST derive the Runtime Flow VCS provider bullet from the bound `vcs-pr-flow` recipe id, not from a `provider` config value.
75:74:If the bound recipe id is unknown to the VCS label table, it MUST emit a `⚠ ai-specs:` warning to stderr and render `VCS PR (custom)`.
76:75:It MUST append `base branch: \`<base_branch>\`` when `base_branch` is configured or defaulted.
77:76:(Previously: The bullet only mapped known recipe ids and appended base branch.)
78:77:
79:78:#### Scenario: GitHub binding renders gh hint
80:79:- GIVEN `bindings.vcs-pr-flow = "git-pr-flow"` and `base_branch = "development"`
81:80:- WHEN the brief is rendered
82:81:- THEN the Runtime Flow section includes `VCS/PR provider: GitHub` and `gh` CLI
83:82:- AND includes `base branch: \`development\``
84:83:
85:84:#### Scenario: Unknown recipe id warns and falls back
86:85:- GIVEN `bindings.vcs-pr-flow = "custom-pr-flow"`
87:86:- WHEN the brief is rendered
88:87:- THEN stderr includes `⚠ ai-specs:`
89:88:- AND the Runtime Flow section uses `VCS PR (custom)`
90:89:
91:90:#### Scenario: Multiple unknown ids each warn
92:91:- GIVEN two render passes bind different unknown `vcs-pr-flow` ids
93:92:- WHEN each brief is rendered
94:93:- THEN each pass emits one `⚠ ai-specs:` warning
95:94:- AND each pass uses `VCS PR (custom)`
96:95:
97:96:#### Scenario: Stale provider config ignored
98:97:- GIVEN a manifest still sets `[recipes.gitlab-mr-flow.config] provider = "github"`
99:98:- WHEN sync validates and renders
100:99:- THEN sync warns that `provider` is an unknown config key
101:100:- AND the rendered brief still identifies GitLab from the binding recipe id
102:101:
103:102:### Requirement: Runtime Checks and Docs
104:103:
105:104:Provider skills and commands MUST check CLI install/auth before PR/MR creation, stop with
106:105:actionable blockers on failure, and README MUST document enablement, config (`base_branch`
107:106:only), explicit bindings, runtime prerequisites, explicit push behavior, and no auto-merge
108:107:policy.
109:108:
110:109:### Requirement: Bound VCS Workflow Rules Stay Isolated
111:110:
112:111:The system MUST emit `workflow_rules` brief fragments only from the recipe bound to `vcs-pr-flow`.
113:112:Fragments from other enabled VCS sibling recipes MUST NOT appear when a binding exists.
114:113:
115:114:#### Scenario: One bound recipe among three enabled
116:115:- GIVEN `git-pr-flow`, `gitlab-mr-flow`, and `bitbucket-pr-flow` are enabled
117:116:- AND `vcs-pr-flow` is bound to `gitlab-mr-flow`
118:117:- WHEN the brief is rendered
119:118:- THEN only GitLab workflow rules appear
120:119:- AND GitHub and Bitbucket workflow rules do not appear
121:120:
122:121:#### Scenario: Single enabled bound recipe
123:122:- GIVEN only `git-pr-flow` is enabled and bound
124:123:- WHEN the brief is rendered
125:124:- THEN the GitHub workflow rules appear
126:125:- AND no other VCS workflow rules are added
127:126:
128:127:#### Scenario: No VCS binding exists
129:128:- GIVEN VCS sibling recipes are enabled
130:129:- AND `vcs-pr-flow` is unbound
131:130:- WHEN the brief is rendered
132:131:- THEN no VCS workflow rule fragments are emitted
133:132:
134:133:### Requirement: Git PR Flow Docs Omit Provider
135:134:
136:135:The `git-pr-flow` README and `docs/recipes-catalog.md` section for `git-pr-flow` MUST document `base_branch` only for config.
137:136:Neither document MAY include a `provider` config row.
138:137:
139:138:#### Scenario: README contract
140:139:- GIVEN `catalog/recipes/git-pr-flow/README.md`
141:140:- WHEN the docs contract is checked
142:141:- THEN the config table includes `base_branch`
143:142:- AND it does not include `provider`
144:143:
145:144:#### Scenario: Catalog contract
146:145:- GIVEN `docs/recipes-catalog.md`
147:146:- WHEN the `## git-pr-flow` section is checked
148:147:- THEN the config table includes `base_branch`
149:148:- AND it does not include `provider`
150:149:
151:150:### Requirement: Pre-merge archive artifacts
152:
153:The system MUST archive and record SDD/OpenSpec artifacts before a VCS PR/MR is merged. The archive boundary MUST occur while the change is still on the review branch, not after the merge commit lands on the base branch.
154:
155:#### Scenario: Archive runs before merge
156:
157:- GIVEN a provider-backed PR/MR is ready to merge
158:- WHEN the archive step runs for the change
159:- THEN the change artifacts are persisted before merge completes
160:- AND the archive records the pre-merge state as the source of truth
161:
162:#### Scenario: Post-merge archive is rejected
163:
164:- GIVEN a PR/MR has already been merged into the base branch
165:- WHEN the archive step tries to treat the merged state as the archive boundary
166:- THEN the system rejects that interpretation
167:- AND the archive must reference the pre-merge branch state instead
168:
169:#### Scenario: Provider behavior stays aligned
170:
171:- GIVEN GitHub, GitLab, or Bitbucket provider flows are enabled
172:- WHEN the pre-merge archive rule is rendered into workflow guidance
173:- THEN the provider guidance matches the same archive-before-merge contract
174:- AND no provider introduces a different timing rule
175:
176:#### Scenario: Hidden ceremony remains hidden
177:
178:- GIVEN the user follows the normal plan/build flow
179:- WHEN the archive rule is applied
180:- THEN no new slash command or extra user-facing mode is introduced
181:- AND the archive step remains part of the existing invisible workflow
182:
183:### Requirement: Post-merge branch and worktree cleanup

After a VCS PR/MR is merged, provider merge-workflow skills MUST instruct the
agent to remove the feature worktree and delete the local feature branch when
the user requests merge cleanup. Squash and rebase merges MUST use force-delete
(`git branch -D`) because feature tips are not ancestors of the base branch.

#### Scenario: Squash merge allows local branch cleanup

- GIVEN a feature branch was merged with squash
- WHEN post-merge cleanup runs
- THEN the skill uses `git branch -D` (not `-d`) for the local branch
- AND removes the worktree from outside the worktree directory

#### Scenario: Provider skills stay aligned on cleanup

- GIVEN GitHub, GitLab, or Bitbucket provider flows are enabled
- WHEN post-merge cleanup guidance is rendered
- THEN each provider skill documents worktree removal and local branch deletion
- AND no provider omits cleanup as an optional afterthought

### Requirement: Test and Validation Commands Pass
184:151:
185:152:The implementation MUST pass `./tests/run.sh` and `./tests/validate.sh` with the change applied.
186:153:
187:154:#### Scenario: Focused run passes
188:155:- GIVEN the change is applied
189:156:- WHEN `./tests/run.sh` runs
190:157:- THEN it exits successfully
191:158:
192:159:#### Scenario: Full validation passes
193:160:- GIVEN the change is applied
194:161:- WHEN `./tests/validate.sh` runs
195:162:- THEN it exits successfully
196:163: