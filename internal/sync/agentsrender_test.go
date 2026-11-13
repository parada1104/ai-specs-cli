package sync

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
)

// agentsRenderRefError is the {"error": {...}} payload the frozen
// agentsrender bundle carries when the Python authority raised (the
// brief-modes validation path).
type agentsRenderRefError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// agentsRenderGoldenCase is one frozen record of
// testdata/agentsrender_golden.json (PR2-B S4): the render-core authority for
// one agentsRenderCases entry. Exactly one of (bytes/sha) and (error) is set.
type agentsRenderGoldenCase struct {
	Name     string                `json:"name"`
	BytesB64 *string               `json:"bytes_b64"`
	SHA256   *string               `json:"sha256"`
	Error    *agentsRenderRefError `json:"error"`
}

// goldenB64Artifact freezes both the presence and the exact bytes of one
// output file (PR2-B S4 step bundle).
type goldenB64Artifact struct {
	Present bool   `json:"present"`
	B64     string `json:"b64"`
}

// agentsStepGoldenCase is one frozen record of
// testdata/agentsstep_golden.json: the observable surface of the real
// two-module Python sequence (brief-render-policy.py + agents-render.py) that
// sync ran before T3, over one briefGovCases entry.
type agentsStepGoldenCase struct {
	Name   string            `json:"name"`
	Stdout string            `json:"stdout"`
	Stderr string            `json:"stderr"`
	RC     int               `json:"rc"`
	Agents goldenB64Artifact `json:"agents"`
	Lock   goldenB64Artifact `json:"lock"`
}

// agentsRenderGoldenNames is the hard-coded identity inventory of the frozen
// agentsrender bundle: exact case names in capture order (13 = 11 render +
// 2 error). Kept in the test, never derived from the bundle.
var agentsRenderGoldenNames = []string{
	"minimal manifest project only",
	"full manifest with every brief section",
	"deps and recipe brief fragments",
	"config substitution edge cases",
	"mcp env redaction",
	"missing optional sections",
	"blank optional sections",
	"repo topology from project field",
	"unknown vcs recipe uses generic label",
	"vcs sibling filtering keeps only the bound provider",
	"replace modes suppress recipe fragments per section",
	"invalid brief mode raises",
	"invalid brief mode date raises",
}

// briefGovGoldenNames is the hard-coded identity inventory of the frozen
// briefgov bundle (13 governance cases, capture order).
var briefGovGoldenNames = []string{
	"missing writes",
	"managed_stale writes",
	"marker preserves",
	"marker preserves with inert preserve flag",
	"untracked preserves",
	"untracked adopts with adopt_brief",
	"user_modified preserves",
	"user_modified adopts with adopt_brief",
	"corrupt lock is undetermined and preserves",
	"brief render false disables the policy gate",
	"brief render non-boolean string fails safe under validate",
	"brief render non-boolean int fails safe under validate",
	"unknown vcs recipe warns on stderr",
}

// agentsStepGoldenNames is the hard-coded identity inventory of the frozen
// composed-step bundle (11 = briefGov order minus the two --validate cases,
// which are off the sync path).
var agentsStepGoldenNames = []string{
	"missing writes",
	"managed_stale writes",
	"marker preserves",
	"marker preserves with inert preserve flag",
	"untracked preserves",
	"untracked adopts with adopt_brief",
	"user_modified preserves",
	"user_modified adopts with adopt_brief",
	"corrupt lock is undetermined and preserves",
	"brief render false disables the policy gate",
	"unknown vcs recipe warns on stderr",
}

// validateGoldenIdentity checks one bundle's recorded names against the
// hard-coded inventory: exact order, no duplicates, exact length.
func validateGoldenIdentity(t *testing.T, bundle string, gotNames, wantNames []string) {
	t.Helper()
	if len(gotNames) != len(wantNames) {
		t.Fatalf("%s: %d records, want %d", bundle, len(gotNames), len(wantNames))
	}
	seen := make(map[string]bool, len(wantNames))
	for i, got := range gotNames {
		if got != wantNames[i] {
			t.Fatalf("%s: record %d is %q, want %q (identity mismatch)", bundle, i, got, wantNames[i])
		}
		if seen[got] {
			t.Fatalf("%s: duplicate record %q", bundle, got)
		}
		seen[got] = true
	}
}

func readGoldenBundle(t *testing.T, filename string) json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		t.Fatalf("read frozen oracle: %v", err)
	}
	return raw
}

// loadAgentsRenderGolden validates and indexes testdata/agentsrender_golden.json.
func loadAgentsRenderGolden(t *testing.T) map[string]agentsRenderGoldenCase {
	t.Helper()
	var bundle struct {
		Cases []agentsRenderGoldenCase `json:"cases"`
	}
	if err := json.Unmarshal(readGoldenBundle(t, "agentsrender_golden.json"), &bundle); err != nil {
		t.Fatalf("parse frozen oracle: %v", err)
	}
	names := make([]string, len(bundle.Cases))
	for i, rec := range bundle.Cases {
		names[i] = rec.Name
	}
	validateGoldenIdentity(t, "agentsrender", names, agentsRenderGoldenNames)
	errors := 0
	byName := make(map[string]agentsRenderGoldenCase, len(bundle.Cases))
	for _, rec := range bundle.Cases {
		if rec.Error != nil {
			errors++
			if rec.BytesB64 != nil || rec.SHA256 != nil {
				t.Fatalf("frozen record %q mixes bytes and error", rec.Name)
			}
		} else if rec.BytesB64 == nil || rec.SHA256 == nil {
			t.Fatalf("frozen record %q has neither bytes nor error", rec.Name)
		}
		byName[rec.Name] = rec
	}
	if errors != 2 {
		t.Fatalf("agentsrender bundle has %d error records, want 2", errors)
	}
	return byName
}

// loadBriefGovGolden validates and indexes testdata/briefgov_golden.json.
func loadBriefGovGolden(t *testing.T) map[string]briefGovRefResult {
	t.Helper()
	var bundle struct {
		Names []string            `json:"names"`
		Cases []briefGovRefResult `json:"cases"`
	}
	if err := json.Unmarshal(readGoldenBundle(t, "briefgov_golden.json"), &bundle); err != nil {
		t.Fatalf("parse frozen oracle: %v", err)
	}
	validateGoldenIdentity(t, "briefgov", bundle.Names, briefGovGoldenNames)
	if len(bundle.Cases) != len(bundle.Names) {
		t.Fatalf("briefgov: %d records for %d names", len(bundle.Cases), len(bundle.Names))
	}
	wrote := 0
	byName := make(map[string]briefGovRefResult, len(bundle.Cases))
	for i, rec := range bundle.Cases {
		if rec.Wrote {
			wrote++
		}
		byName[bundle.Names[i]] = rec
	}
	if wrote != 6 {
		t.Fatalf("briefgov bundle has %d wrote=true records, want 6", wrote)
	}
	return byName
}

// loadAgentsStepGolden validates and indexes testdata/agentsstep_golden.json.
func loadAgentsStepGolden(t *testing.T) map[string]agentsStepGoldenCase {
	t.Helper()
	var bundle struct {
		Names []string               `json:"names"`
		Cases []agentsStepGoldenCase `json:"cases"`
	}
	if err := json.Unmarshal(readGoldenBundle(t, "agentsstep_golden.json"), &bundle); err != nil {
		t.Fatalf("parse frozen oracle: %v", err)
	}
	validateGoldenIdentity(t, "agentsstep", bundle.Names, agentsStepGoldenNames)
	if len(bundle.Cases) != len(bundle.Names) {
		t.Fatalf("agentsstep: %d records for %d names", len(bundle.Cases), len(bundle.Names))
	}
	present := 0
	byName := make(map[string]agentsStepGoldenCase, len(bundle.Cases))
	for i, rec := range bundle.Cases {
		if rec.Agents.Present {
			present++
		}
		byName[bundle.Names[i]] = rec
	}
	if present != 10 {
		t.Fatalf("agentsstep bundle has %d AGENTS.md artifacts, want 10", present)
	}
	return byName
}

// agentsRenderCase is one differential scenario. resolved builds a fresh
// resolved-config map for a leg's own project root (so the two legs never
// share mutable state and topology detection sees a real, non-git dir).
type agentsRenderCase struct {
	name     string
	manifest string
	resolved func(projectRoot string) map[string]any
	wantErr  bool
}

// emptyResolved builds a resolved-config with no bindings/recipes/enabled.
func emptyResolved(projectRoot string) map[string]any {
	return map[string]any{
		"bindings":     map[string]any{},
		"recipes":      map[string]any{},
		"enabled":      []any{},
		"project_root": projectRoot,
	}
}

// TestAgentsRenderDifferential runs every case through the Go rendering core
// and requires byte equality against the frozen authority record
// (PR2-B S4: the live python3 leg is replaced by
// testdata/agentsrender_golden.json).
func TestAgentsRenderDifferential(t *testing.T) {
	golden := loadAgentsRenderGolden(t)

	for _, tc := range agentsRenderCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec, ok := golden[tc.name]
			if !ok {
				t.Fatalf("frozen record %q missing", tc.name)
			}

			manifestTable, err := toml.Parse([]byte(tc.manifest))
			if err != nil {
				t.Fatalf("parse manifest with internal/toml: %v", err)
			}

			goDir := t.TempDir()
			got := RenderAgentsMarkdown(manifestTable, tc.resolved(goDir))

			if tc.wantErr {
				if rec.Error == nil {
					t.Fatalf("frozen authority did not raise; got %+v", rec)
				}
				if rec.Error.Type != "ValueError" {
					t.Errorf("frozen error type = %q, want ValueError", rec.Error.Type)
				}
				if got != nil {
					t.Errorf("RenderAgentsMarkdown returned %d byte(s), want nil on invalid brief mode", len(got))
				}
				brief, _ := manifestTable.Table("brief")
				verr := ValidateBriefModes(brief)
				if verr == nil {
					t.Fatalf("ValidateBriefModes returned nil, want %q", rec.Error.Message)
				}
				if verr.Error() != rec.Error.Message {
					t.Errorf("ValidateBriefModes message:\n  go:     %q\n  frozen: %q", verr.Error(), rec.Error.Message)
				}
				return
			}

			want, err := base64.StdEncoding.DecodeString(*rec.BytesB64)
			if err != nil {
				t.Fatalf("decode frozen bytes: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("would-write bytes differ\n--- go ---\n%s\n--- frozen ---\n%s", got, want)
			}
			if gotSHA := sha256Hex(got); gotSHA != *rec.SHA256 {
				t.Errorf("sha256: go=%s frozen=%s", gotSHA, *rec.SHA256)
			}
		})
	}
}

func agentsRenderCases() []agentsRenderCase {
	return []agentsRenderCase{
		{
			name: "minimal manifest project only",
			manifest: `[project]
name = 'minimal'

[agents]
enabled = ['claude']
`,
			resolved: emptyResolved,
		},
		{
			name: "full manifest with every brief section",
			manifest: `[project]
name = 'full'

[agents]
enabled = ['claude', 'pi']

[brief]
intro = """
First line.

Second line.
"""
purpose = 'Do great things.'
runtime_flow = ['Flow one.', 'Flow two.']
context_sources = ['Source one.']
conflict_policy = ['Policy one.']
workflow_rules = ['Rule one.']
useful_commands = ['Run the focused suite.']

[brief.mcp_descriptions]
trello = 'Project tracking.'
vault = 'Canonical notes.'

[mcp.trello]
command = 'npx'
args = ['-y', '@trello/mcp']
env = { TOKEN = '$TRELLO_TOKEN' }

[mcp.engram]
command = 'engram'
env = { API_KEY = 'sk-super-secret-value-1234567890', HOME = '${HOME}' }
`,
			resolved: func(projectRoot string) map[string]any {
				return map[string]any{
					"bindings": map[string]any{
						"tracker":     "trello-mcp-workflow",
						"vcs-pr-flow": "git-pr-flow",
						"test-runner": "tdd-flow",
					},
					"recipes": map[string]any{
						"trello-mcp-workflow": map[string]any{"board_id": "69ec097f13e2d38ecd89a557"},
						"git-pr-flow":         map[string]any{"base_branch": "development"},
						"tdd-flow":            map[string]any{"test_command": "./tests/validate.sh"},
						"worktree-flow":       map[string]any{"integration_branch": "development"},
					},
					"enabled": []any{
						"git-pr-flow", "tdd-flow", "worktree-flow", "trello-mcp-workflow",
					},
					"project_root": projectRoot,
				}
			},
		},
		{
			name: "deps and recipe brief fragments",
			manifest: `[project]
name = 'fragments'

[agents]
enabled = ['claude']

[[deps]]
id = 'some-dep'
source = '../some-dep'
path = 'skills/some'

[recipes.wf]
enabled = true

[recipes.wf.config]
integration_branch = 'main'

[recipes.tdd]
enabled = true
`,
			resolved: func(projectRoot string) map[string]any {
				return map[string]any{
					"bindings": map[string]any{},
					"recipes": map[string]any{
						"wf": map[string]any{
							"integration_branch": "main",
							"brief_fragments": map[string]any{
								"workflow_rules": []any{
									map[string]any{"key": "wt", "text": "Create a worktree on `{config.integration_branch}`."},
								},
								"context_sources": []any{
									map[string]any{"text": "Trello is the source of truth."},
								},
							},
						},
						"tdd": map[string]any{
							"test_command": "./tests/run.sh",
							"brief_fragments": map[string]any{
								"workflow_rules": []any{
									map[string]any{"text": "Write failing tests first."},
								},
								"useful_commands": []any{
									map[string]any{"text": "Run tests: `{config.test_command}`"},
								},
							},
						},
					},
					"enabled":      []any{"wf", "tdd"},
					"project_root": projectRoot,
				}
			},
		},
		{
			name: "config substitution edge cases",
			manifest: `[project]
name = 'sub'

[agents]
enabled = ['claude']

[recipes.sub]
enabled = true
`,
			resolved: func(projectRoot string) map[string]any {
				return map[string]any{
					"bindings": map[string]any{},
					"recipes": map[string]any{
						"sub": map[string]any{
							"known": "VALUE",
							"flag":  true,
							"count": 3,
							"scope": []any{"root", "skill"},
							"brief_fragments": map[string]any{
								"workflow_rules": []any{
									map[string]any{"text": "A {config.known} B {config.missing} C {bare} D {{esc}} E {config.flag} F {config.count} G {config.scope} H { lone"},
								},
							},
						},
					},
					"enabled":      []any{"sub"},
					"project_root": projectRoot,
				}
			},
		},
		{
			name: "mcp env redaction",
			manifest: `[project]
name = 'envs'

[agents]
enabled = ['claude']

[mcp.alpha]
command = 'alpha'
env = ['PLAIN_VAR', 'OTHER_VAR']

[mcp.beta]
command = 'beta'
environment = { A = '$A_VAR', B = '${B_VAR}', C = 'plain-secret-value', D = '  $PADDED  ' }
`,
			resolved: emptyResolved,
		},
		{
			name: "missing optional sections",
			manifest: `[project]
name = 'empty'
`,
			resolved: emptyResolved,
		},
		{
			name: "blank optional sections",
			manifest: `[project]
name = 'blank'

[agents]
enabled = ['pi']

[brief]
intro = '   '
purpose = ''
`,
			resolved: emptyResolved,
		},
		{
			name: "repo topology from project field",
			manifest: `[project]
name = 'topo'
repo_topology = 'monorepo-apps'

[agents]
enabled = ['claude']
`,
			resolved: emptyResolved,
		},
		{
			name: "unknown vcs recipe uses generic label",
			manifest: `[project]
name = 'vcs'

[agents]
enabled = ['claude']

[recipes.custom-vcs]
enabled = true

[recipes.custom-vcs.config]
base_branch = 'trunk'
`,
			resolved: func(projectRoot string) map[string]any {
				return map[string]any{
					"bindings": map[string]any{"vcs-pr-flow": "custom-vcs"},
					"recipes": map[string]any{
						"custom-vcs": map[string]any{"base_branch": "trunk"},
					},
					"enabled":      []any{"custom-vcs"},
					"project_root": projectRoot,
				}
			},
		},
		{
			name: "vcs sibling filtering keeps only the bound provider",
			manifest: `[project]
name = 'vcsfilter'

[agents]
enabled = ['claude']

[recipes.git-pr-flow]
enabled = true

[recipes.gitlab-mr-flow]
enabled = true

[recipes.bitbucket-pr-flow]
enabled = true

[recipes.worktree-flow]
enabled = true
`,
			resolved: func(projectRoot string) map[string]any {
				frag := func(text string) map[string]any {
					return map[string]any{
						"brief_fragments": map[string]any{
							"workflow_rules": []any{map[string]any{"text": text}},
						},
					}
				}
				return map[string]any{
					"bindings": map[string]any{"vcs-pr-flow": "gitlab-mr-flow"},
					"recipes": map[string]any{
						"git-pr-flow":       frag("Use GitHub PRs to merge."),
						"gitlab-mr-flow":    frag("Use GitLab MRs to merge."),
						"bitbucket-pr-flow": frag("Use Bitbucket PRs to merge."),
						"worktree-flow":     frag("Create a worktree for every change."),
					},
					"enabled": []any{
						"git-pr-flow", "gitlab-mr-flow", "bitbucket-pr-flow", "worktree-flow",
					},
					"project_root": projectRoot,
				}
			},
		},
		{
			name: "replace modes suppress recipe fragments per section",
			manifest: `[project]
name = 'replace'

[agents]
enabled = ['claude']

[brief]
workflow_rules_mode = 'replace'
workflow_rules = ['Only this rule.']

[recipes.wf]
enabled = true
`,
			resolved: func(projectRoot string) map[string]any {
				return map[string]any{
					"bindings": map[string]any{},
					"recipes": map[string]any{
						"wf": map[string]any{
							"brief_fragments": map[string]any{
								"workflow_rules":  []any{map[string]any{"text": "Recipe rule should not appear."}},
								"context_sources": []any{map[string]any{"text": "Recipe context."}},
							},
						},
					},
					"enabled":      []any{"wf"},
					"project_root": projectRoot,
				}
			},
		},
		{
			name: "invalid brief mode raises",
			manifest: `[project]
name = 'bad'

[brief]
workflow_rules_mode = 'merge'
`,
			resolved: emptyResolved,
			wantErr:  true,
		},
		{
			// A date/time mode value drives pyRepr through the ValueError
			// message (agents-render.py:180 uses {val!r}); the differential
			// pins Go's repr to the real Python authority.
			name: "invalid brief mode date raises",
			manifest: `[project]
name = 'bad-date'

[brief]
workflow_rules_mode = 1979-05-27
`,
			resolved: emptyResolved,
			wantErr:  true,
		},
	}
}

// ---------------------------------------------------------------------------
// S3b: write-governance differential against the real Python authority
// ---------------------------------------------------------------------------

// briefGovRefInput mirrors testdata/briefgov_ref.py's stdin JSON.
// briefGovRefPolicy mirrors the policy leg of the frozen briefgov bundle.
type briefGovRefPolicy struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	RC     int    `json:"rc"`
}

// briefGovRefResult mirrors one frozen record of
// testdata/briefgov_golden.json (PR2-B S4).
type briefGovRefResult struct {
	Policy         briefGovRefPolicy `json:"policy"`
	RenderStdout   string            `json:"render_stdout"`
	RenderStderr   string            `json:"render_stderr"`
	RenderRC       int               `json:"render_rc"`
	Action         string            `json:"action"`
	ClassifyState  string            `json:"classify_state"`
	EffectiveState string            `json:"effective_state"`
	Wrote          bool              `json:"wrote"`
	AgentsSHA256   *string           `json:"agents_sha256"`
	LockSHA256     *string           `json:"lock_sha256"`
}

// briefGovCase is one governance scenario. Pointers keep "absent" distinct
// from an empty file; resolved nil means render() runs without a
// --resolved-config file.
type briefGovCase struct {
	name           string
	manifest       string
	existingAgents *string
	lockTOML       *string
	resolved       map[string]any
	adoptBrief     bool
	preserveFlag   bool
	policyValidate bool
}

func govPtr(s string) *string { return &s }

// briefLockTOML builds the [managed."AGENTS.md"] baseline entry lock.py/Go's
// lock package both read.
func briefLockTOML(sha string) string {
	return "[managed.\"AGENTS.md\"]\nsha256 = \"" + sha + "\"\nkind = \"runtime-brief\"\npolicy = \"never-force\"\n"
}

func briefGovCases() []briefGovCase {
	const manifest = "[project]\nname = 'gov'\n\n[agents]\nenabled = ['claude']\n"
	const stale = "# stale brief\n"
	const mine = "# mine\n"
	const withMarker = "# mine\n<!-- ai-specs:runtime-brief -->\n"
	otherBaseline := sha256Hex([]byte("# original\n"))
	return []briefGovCase{
		{name: "missing writes", manifest: manifest},
		{
			name:           "managed_stale writes",
			manifest:       manifest,
			existingAgents: govPtr(stale),
			lockTOML:       govPtr(briefLockTOML(sha256Hex([]byte(stale)))),
		},
		{name: "marker preserves", manifest: manifest, existingAgents: govPtr(withMarker)},
		{
			name:           "marker preserves with inert preserve flag",
			manifest:       manifest,
			existingAgents: govPtr(withMarker),
			preserveFlag:   true,
		},
		{name: "untracked preserves", manifest: manifest, existingAgents: govPtr(mine)},
		{
			name:           "untracked adopts with adopt_brief",
			manifest:       manifest,
			existingAgents: govPtr(mine),
			adoptBrief:     true,
		},
		{
			name:           "user_modified preserves",
			manifest:       manifest,
			existingAgents: govPtr(mine),
			lockTOML:       govPtr(briefLockTOML(otherBaseline)),
		},
		{
			name:           "user_modified adopts with adopt_brief",
			manifest:       manifest,
			existingAgents: govPtr(mine),
			lockTOML:       govPtr(briefLockTOML(otherBaseline)),
			adoptBrief:     true,
		},
		{
			name:           "corrupt lock is undetermined and preserves",
			manifest:       manifest,
			existingAgents: govPtr(mine),
			lockTOML:       govPtr("managed = = broken\n"),
		},
		{
			name:     "brief render false disables the policy gate",
			manifest: "[project]\nname = 'x'\n\n[brief]\nrender = false\n",
		},
		{
			name:           "brief render non-boolean string fails safe under validate",
			manifest:       "[project]\nname = 'x'\n\n[brief]\nrender = 'yes'\n",
			policyValidate: true,
		},
		{
			name:           "brief render non-boolean int fails safe under validate",
			manifest:       "[project]\nname = 'x'\n\n[brief]\nrender = 1\n",
			policyValidate: true,
		},
		{
			name:     "unknown vcs recipe warns on stderr",
			manifest: manifest,
			resolved: map[string]any{
				"bindings": map[string]any{"vcs-pr-flow": "custom-vcs"},
				"recipes":  map[string]any{"custom-vcs": map[string]any{"base_branch": "trunk"}},
				"enabled":  []any{},
			},
		},
	}
}

// setupBriefGovDir materializes the real project layout for one leg.
func setupBriefGovDir(t *testing.T, dir string, tc briefGovCase) (tomlPath, outPath, lockPath, resolvedPath string) {
	t.Helper()
	tomlPath = filepath.Join(dir, "ai-specs", "ai-specs.toml")
	outPath = filepath.Join(dir, "AGENTS.md")
	lockPath = filepath.Join(dir, "ai-specs", ".ai-specs.lock")
	if err := os.MkdirAll(filepath.Dir(tomlPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(tomlPath, []byte(tc.manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if tc.existingAgents != nil {
		if err := os.WriteFile(outPath, []byte(*tc.existingAgents), 0o644); err != nil {
			t.Fatalf("write AGENTS.md: %v", err)
		}
	}
	if tc.lockTOML != nil {
		if err := os.WriteFile(lockPath, []byte(*tc.lockTOML), 0o644); err != nil {
			t.Fatalf("write lock: %v", err)
		}
	}
	if tc.resolved != nil {
		data, err := json.Marshal(tc.resolved)
		if err != nil {
			t.Fatalf("marshal resolved: %v", err)
		}
		resolvedPath = filepath.Join(dir, "resolved.json")
		if err := os.WriteFile(resolvedPath, data, 0o644); err != nil {
			t.Fatalf("write resolved: %v", err)
		}
	}
	return tomlPath, outPath, lockPath, resolvedPath
}

// goBriefPolicy reproduces brief-render-policy.py main()'s observable surface.
func goBriefPolicy(t *testing.T, tomlPath string, validate bool) (string, string, int) {
	t.Helper()
	enabled, evalErr := EvaluateBriefRender(tomlPath)
	var validateErr error
	if validate {
		validateErr = ValidateBriefRender(tomlPath)
	}
	if validateErr != nil {
		return "", "error: " + validateErr.Error() + "\n", 1
	}
	if evalErr != nil {
		return "", "error: " + evalErr.Error() + "\n", 1
	}
	return fmt.Sprintf("%t\n", enabled), "", 0
}

func fileSHA256OrNull(path string) *string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	s := sha256Hex(data)
	return &s
}

func govPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// TestAgentsRenderGovernanceDifferential runs every governance case through
// the Go port and requires byte equality against the frozen authority record
// (PR2-B S4: the live briefgov_ref.py leg is replaced by
// testdata/briefgov_golden.json): the policy surface, the render surface, the
// raw state decision, and the written AGENTS.md / lock bytes.
func TestAgentsRenderGovernanceDifferential(t *testing.T) {
	golden := loadBriefGovGolden(t)

	for _, tc := range briefGovCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec, ok := golden[tc.name]
			if !ok {
				t.Fatalf("frozen record %q missing", tc.name)
			}

			goDir := t.TempDir()
			tomlPath, outPath, lockPath, resolvedPath := setupBriefGovDir(t, goDir, tc)

			pOut, pErr, pRC := goBriefPolicy(t, tomlPath, tc.policyValidate)
			if pOut != rec.Policy.Stdout {
				t.Errorf("policy stdout:\n  go:     %q\n  frozen: %q", pOut, rec.Policy.Stdout)
			}
			if pErr != rec.Policy.Stderr {
				t.Errorf("policy stderr:\n  go:     %q\n  frozen: %q", pErr, rec.Policy.Stderr)
			}
			if pRC != rec.Policy.RC {
				t.Errorf("policy rc: go=%d frozen=%d", pRC, rec.Policy.RC)
			}

			manifest, resolved, err := loadAgentsRenderInputs(tomlPath, resolvedPath)
			if err != nil {
				t.Fatalf("load inputs: %v", err)
			}
			content := RenderAgentsMarkdown(manifest, resolved)
			if got := classifyBriefState(tomlPath, outPath, content, lockPath, nil); got != rec.ClassifyState {
				t.Errorf("classify state: go=%q frozen=%q", got, rec.ClassifyState)
			}
			if got := briefEffectiveState(tomlPath, outPath, content, lockPath, nil); got != rec.EffectiveState {
				t.Errorf("effective state: go=%q frozen=%q", got, rec.EffectiveState)
			}

			var rOut, rErr bytes.Buffer
			action, wrote, rc := RenderAgentsFile(tomlPath, outPath, RenderAgentsOptions{
				PreserveIfRuntimeBrief: tc.preserveFlag,
				AdoptBrief:             tc.adoptBrief,
				ResolvedConfigPath:     resolvedPath,
			}, &rOut, &rErr)
			if action != rec.Action {
				t.Errorf("action: go=%q frozen=%q", action, rec.Action)
			}
			if wrote != rec.Wrote {
				t.Errorf("wrote: go=%v frozen=%v", wrote, rec.Wrote)
			}
			if rc != rec.RenderRC {
				t.Errorf("render rc: go=%d frozen=%d", rc, rec.RenderRC)
			}
			if rOut.String() != rec.RenderStdout {
				t.Errorf("render stdout:\n  go:     %q\n  frozen: %q", rOut.String(), rec.RenderStdout)
			}
			if rErr.String() != rec.RenderStderr {
				t.Errorf("render stderr:\n  go:     %q\n  frozen: %q", rErr.String(), rec.RenderStderr)
			}
			if got := fileSHA256OrNull(outPath); !govPtrEqual(got, rec.AgentsSHA256) {
				t.Errorf("AGENTS.md sha: go=%v frozen=%v", derefString(got), derefString(rec.AgentsSHA256))
			}
			if got := fileSHA256OrNull(lockPath); !govPtrEqual(got, rec.LockSHA256) {
				t.Errorf("lock sha: go=%v frozen=%v", derefString(got), derefString(rec.LockSHA256))
			}
		})
	}
}

// TestAgentsRenderStepDifferential drives the composed native step
// (policy gate + renderer) against the frozen record of the real
// two-Python-module sequence sync ran before T3 (PR2-B S4:
// testdata/agentsstep_golden.json), over every governance fixture plus a
// preserve-if-runtime-brief case. It compares the exit code, both streams and
// the exact AGENTS.md / lock bytes.
func TestAgentsRenderStepDifferential(t *testing.T) {
	golden := loadAgentsStepGolden(t)

	for _, tc := range briefGovCases() {
		if tc.policyValidate {
			// --validate is not on the sync path; the strict gate belongs to
			// doctor and is covered by TestBriefRenderPolicy.
			continue
		}
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rec, ok := golden[tc.name]
			if !ok {
				t.Fatalf("frozen record %q missing", tc.name)
			}

			goDir := t.TempDir()
			goTOML, goOut, goLock, goResolved := setupBriefGovDir(t, goDir, tc)
			if goResolved == "" {
				goResolved = writeEmptyResolved(t, goDir)
			}
			var out, errW bytes.Buffer
			gotRC := renderAgentsStep(goTOML, goOut, goResolved, tc.adoptBrief, &out, &errW)

			if gotRC != rec.RC {
				t.Errorf("rc: go=%d frozen=%d (go stderr=%q)", gotRC, rec.RC, errW.String())
			}
			if out.String() != rec.Stdout {
				t.Errorf("stdout:\n  go:     %q\n  frozen: %q", out.String(), rec.Stdout)
			}
			if errW.String() != rec.Stderr {
				t.Errorf("stderr:\n  go:     %q\n  frozen: %q", errW.String(), rec.Stderr)
			}
			assertFrozenArtifact(t, "AGENTS.md", goOut, rec.Agents)
			assertFrozenArtifact(t, "lock", goLock, rec.Lock)
		})
	}
}

// assertFrozenArtifact requires the file at path to be present exactly when
// the frozen record says so, with byte equality when present.
func assertFrozenArtifact(t *testing.T, label, path string, want goldenB64Artifact) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if want.Present {
				t.Errorf("%s: file absent, frozen authority wrote it", label)
			}
			return
		}
		t.Fatalf("read %s: %v", path, err)
	}
	if !want.Present {
		t.Errorf("%s: file present, frozen authority left none", label)
		return
	}
	wantBytes, err := base64.StdEncoding.DecodeString(want.B64)
	if err != nil {
		t.Fatalf("decode frozen %s bytes: %v", label, err)
	}
	if !bytes.Equal(data, wantBytes) {
		t.Errorf("%s bytes differ from the frozen authority", label)
	}
}

// writeEmptyResolved writes the resolved-config JSON of a project with no
// bindings or recipes into dir and returns its path.
func writeEmptyResolved(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "resolved.json")
	data, err := json.Marshal(map[string]any{
		"bindings": map[string]any{},
		"recipes":  map[string]any{},
		"enabled":  []any{},
	})
	if err != nil {
		t.Fatalf("marshal resolved: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write resolved: %v", err)
	}
	return path
}

// TestRenderAgentsStepPolicyFailurePreservesTheSilentSkip pins D3 on the
// composed step: when the policy cannot reach a verdict (here, a manifest the
// parser rejects) its failure is ignored, the gate is not satisfied, and sync
// skips the brief with exit 0 — the same silent skip sync.go produced when the
// policy was a child process. Only the error wording differs, since the two
// parsers describe the same defect differently (L6's recorded divergence).
func TestRenderAgentsStepPolicyFailurePreservesTheSilentSkip(t *testing.T) {
	const broken = "[project\nname = 'x'\n"

	tomlPath := filepath.Join(t.TempDir(), "ai-specs", "ai-specs.toml")
	if err := os.MkdirAll(filepath.Dir(tomlPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(tomlPath, []byte(broken), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	outPath := filepath.Join(filepath.Dir(filepath.Dir(tomlPath)), "AGENTS.md")

	var out, errW bytes.Buffer
	if rc := renderAgentsStep(tomlPath, outPath, "", false, &out, &errW); rc != 0 {
		t.Errorf("renderAgentsStep rc = %d, want 0 (the policy's failure is ignored)", rc)
	}
	if want := "  \u2139 skipped AGENTS.md (brief.render = false)\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if !strings.HasPrefix(errW.String(), "error: ") {
		t.Errorf("stderr = %q, want one `error: ` line", errW.String())
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Errorf("AGENTS.md written despite no policy verdict (err=%v)", err)
	}
}

// TestRenderAgentsStepPolicyGate pins the sync wiring's two-step contract:
// the policy gate runs first, and a false gate skips the renderer entirely.
func TestRenderAgentsStepPolicyGate(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "ai-specs", "ai-specs.toml")
	outPath := filepath.Join(dir, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(tomlPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(tomlPath, []byte("[project]\nname = 'gate'\n\n[brief]\nrender = false\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	var out, errW bytes.Buffer
	if rc := renderAgentsStep(tomlPath, outPath, "", false, &out, &errW); rc != 0 {
		t.Fatalf("renderAgentsStep rc = %d, want 0", rc)
	}
	if want := "  ℹ skipped AGENTS.md (brief.render = false)\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Errorf("AGENTS.md exists despite brief.render = false (err=%v)", err)
	}

	if err := os.WriteFile(tomlPath, []byte("[project]\nname = 'gate'\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	out.Reset()
	errW.Reset()
	if rc := renderAgentsStep(tomlPath, outPath, "", false, &out, &errW); rc != 0 {
		t.Fatalf("renderAgentsStep rc = %d, want 0; stderr=%s", rc, errW.String())
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("AGENTS.md not written: %v", err)
	}
	if !strings.Contains(string(data), "# gate Runtime Brief") {
		t.Errorf("AGENTS.md missing the rendered heading:\n%s", data)
	}
}

// TestAgentsRenderWidenedValueKinds pins the D6 value kinds in the Python
// formatting mirrors: pyRepr renders the datetime constructor form and pyStr
// the ISO str() form (agents-render.py interpolates with f-strings), while
// arbitrary-precision ints render as their exact decimal digits. The
// already-working scalar kinds are asserted unchanged.
func TestAgentsRenderWidenedValueKinds(t *testing.T) {
	const big = toml.BigInt("123456789012345678901234567890123456789")
	date := toml.DateTime{Kind: toml.DateLocal, Year: 1979, Month: 5, Day: 27}
	dt := toml.DateTime{Kind: toml.DateTimeOffset, Year: 1979, Month: 5, Day: 27, Hour: 7, Minute: 32, HasOffset: true}

	cases := []struct {
		name     string
		value    any
		wantRepr string
		wantStr  string
	}{
		{"bigint", big, string(big), string(big)},
		{"date", date, "datetime.date(1979, 5, 27)", "1979-05-27"},
		{"datetime", dt, "datetime.datetime(1979, 5, 27, 7, 32, tzinfo=datetime.timezone.utc)", "1979-05-27 07:32:00+00:00"},
		{"int64-unchanged", int64(5), "5", "5"},
		{"string-unchanged", "x", "'x'", "x"},
		{"bool-unchanged", true, "True", "True"},
	}
	for _, tc := range cases {
		if got := pyRepr(tc.value); got != tc.wantRepr {
			t.Errorf("%s: pyRepr = %q, want %q", tc.name, got, tc.wantRepr)
		}
		if got := pyStr(tc.value); got != tc.wantStr {
			t.Errorf("%s: pyStr = %q, want %q", tc.name, got, tc.wantStr)
		}
	}
}

// TestCollectRecipeBriefFragmentsFirstWinsKeyAndExactText pins the two dedupe
// rules of collect_recipe_brief_fragments: a keyed fragment belongs to the
// FIRST enabled recipe that declares that key, and an identical unkeyed text is
// emitted once. The retired Python suite reached this through the renderer's
// --resolved-config process boundary with a hand-crafted resolved document;
// the native rules are asserted on the collector that owns them.
func TestCollectRecipeBriefFragmentsFirstWinsKeyAndExactText(t *testing.T) {
	const sharedRule = "A session works on one explicit user request or tracker card; resolve focus from memory and tracker before starting."
	resolved := map[string]any{
		"enabled": []any{"session-context", "recipe-extra"},
		"recipes": map[string]any{
			"session-context": map[string]any{
				"brief_fragments": map[string]any{
					"conflict_policy": []any{
						map[string]any{
							"key":  "conflict-policy-source-authority",
							"text": "Current explicit human instruction wins.",
						},
					},
					"workflow_rules": []any{
						map[string]any{"key": nil, "text": sharedRule},
					},
				},
			},
			"recipe-extra": map[string]any{
				"brief_fragments": map[string]any{
					"conflict_policy": []any{
						map[string]any{
							"key":  "conflict-policy-source-authority",
							"text": "Extra recipe override must not appear.",
						},
					},
					"workflow_rules": []any{
						map[string]any{"key": nil, "text": sharedRule},
					},
				},
			},
		},
	}

	got := collectRecipeBriefFragments(resolved, "conflict_policy", nil)
	if len(got) != 1 {
		t.Fatalf("conflict_policy fragments = %d, want 1: %#v", len(got), got)
	}
	if got[0].text != "Current explicit human instruction wins." {
		t.Fatalf("conflict_policy[0].text = %q, want the first recipe text", got[0].text)
	}
	if got[0].key == nil || *got[0].key != "conflict-policy-source-authority" {
		t.Fatalf("conflict_policy[0].key = %v, want conflict-policy-source-authority", got[0].key)
	}
	if strings.Contains(got[0].text, "must not appear") {
		t.Fatalf("the second recipe override survived: %q", got[0].text)
	}

	rules := collectRecipeBriefFragments(resolved, "workflow_rules", nil)
	if len(rules) != 1 {
		t.Fatalf("workflow_rules fragments = %d, want 1 (exact-text dedupe): %#v", len(rules), rules)
	}
	if rules[0].key != nil {
		t.Fatalf("an unkeyed fragment must stay unkeyed, got %q", *rules[0].key)
	}
	if rules[0].text != sharedRule {
		t.Fatalf("workflow_rules[0].text = %q", rules[0].text)
	}
}

// TestRenderAgentsMarkdownResolvesStructuredFieldsThroughTheBinding pins the
// four structured fields at the binding seam: test_command, integration_branch
// and vault_scope come from the recipe the CAPABILITY is bound to, never from a
// hard-coded recipe id. The retired Python suite reached this through the
// renderer's --resolved-config process boundary with a hand-crafted resolved
// document (no CLI verb can produce a binding to a recipe outside the catalog);
// the native rules are asserted on the renderer that owns them.
func TestRenderAgentsMarkdownResolvesStructuredFieldsThroughTheBinding(t *testing.T) {
	manifest, err := toml.Parse([]byte("[project]\nname = 'binding-substitution'\n\n[agents]\nenabled = ['claude']\n"))
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	resolved := map[string]any{
		"bindings": map[string]any{
			"test-runner":        "my-custom-runner",
			"worktree-isolation": "my-worktree",
			"canonical-store":    "my-vault",
		},
		"recipes": map[string]any{
			"my-custom-runner":      map[string]any{"test_command": "./custom-tests.sh"},
			"tdd-flow":              map[string]any{"test_command": "./tests/run.sh"},
			"my-worktree":           map[string]any{"integration_branch": "staging"},
			"worktree-flow":         map[string]any{"integration_branch": "main"},
			"my-vault":              map[string]any{"vault_scope": "my/vault/path"},
			"vault-canonical-store": map[string]any{"vault_scope": "other/path"},
		},
		"enabled": []any{"my-custom-runner", "my-worktree", "my-vault"},
	}

	got := string(RenderAgentsMarkdown(manifest, resolved))
	for _, want := range []string{
		"- Focused tests: `./custom-tests.sh`",
		"- **Integration branch**: `staging`",
		"- **Vault scope**: `my/vault/path`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("render lacks the BOUND field %q:\n%s", want, got)
		}
	}
	// The un-bound recipes' values must not appear: resolution follows the
	// binding, never a hard-coded recipe id.
	for _, unwanted := range []string{"`./tests/run.sh`", "`main`", "other/path"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("un-bound recipe value %q leaked into the render:\n%s", unwanted, got)
		}
	}
}

// TestLoadResolvedConfigDegradesMalformedDocumentsAndStillRenders pins the two
// payload shapes no CLI verb can produce (malformed JSON and a non-dict
// document) plus the wrong-typed inner fields the Python suite fed the
// renderer: the reader degrades to the empty document, the inner fields are
// coerced, and the render still carries the manifest's identity and prose.
func TestLoadResolvedConfigDegradesMalformedDocumentsAndStillRenders(t *testing.T) {
	manifest, err := toml.Parse([]byte(
		"[project]\nname = 'tolerant-render'\n\n[agents]\nenabled = ['claude']\n\n" +
			"[brief]\nworkflow_rules = ['No merges without review.']\n"))
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}

	cases := []struct {
		name     string
		payload  string
		wantKeys map[string]any
	}{
		{
			name:    "malformed json degrades to the empty document",
			payload: "this is not json {{{",
		},
		{
			name:    "non-dict document degrades to the empty document",
			payload: `["a", "b", "c"]`,
		},
		{
			name:    "wrong-typed inner fields are coerced",
			payload: `{"bindings": ["tracker"], "recipes": "should-be-a-dict", "enabled": {"tdd-flow": true}}`,
			wantKeys: map[string]any{
				"bindings": map[string]any{},
				"recipes":  map[string]any{},
				"enabled":  []any{},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "resolved.json")
			if err := os.WriteFile(path, []byte(tc.payload), 0o644); err != nil {
				t.Fatalf("write payload: %v", err)
			}
			got := loadResolvedConfig(path)
			for key, want := range tc.wantKeys {
				if !reflect.DeepEqual(got[key], want) {
					t.Errorf("%s = %#v, want %#v", key, got[key], want)
				}
			}
			if tc.wantKeys == nil && len(got) != 0 {
				t.Errorf("an unusable document must degrade to the empty document: %#v", got)
			}

			rendered := string(RenderAgentsMarkdown(manifest, got))
			for _, want := range []string{"tolerant-render", "No merges without review."} {
				if !strings.Contains(rendered, want) {
					t.Errorf("render lacks %q:\n%s", want, rendered)
				}
			}
		})
	}
}

// TestRenderAgentsMarkdownToleratesAnEnabledRecipeWithoutConfig pins the
// no-crash contract for an enabled id the resolved document carries no recipe
// entry for. The Python suite reached it through the renderer's
// --resolved-config boundary because the CLI derives the enabled list from the
// manifest; the native accessors own the tolerance, so it is asserted here.
func TestRenderAgentsMarkdownToleratesAnEnabledRecipeWithoutConfig(t *testing.T) {
	manifest, err := toml.Parse([]byte("[project]\nname = 'missing-recipe-config'\n\n[agents]\nenabled = ['claude']\n"))
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	resolved := map[string]any{
		"enabled":  []any{"missing-recipe"},
		"recipes":  map[string]any{},
		"bindings": map[string]any{},
	}
	got := string(RenderAgentsMarkdown(manifest, resolved))
	if !strings.Contains(got, "missing-recipe-config") {
		t.Errorf("render lost the project identity:\n%s", got)
	}
	if strings.Count(got, "## ") != strings.Count(string(RenderAgentsMarkdown(manifest, map[string]any{})), "## ") {
		t.Errorf("an absent recipe entry changed the section set:\n%s", got)
	}
}
