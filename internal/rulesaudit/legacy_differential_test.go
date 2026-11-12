package rulesaudit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

// Full-payload oracle: the legacy rules-inventory.py output over a set of
// fabricated trees was captured once into testdata/rules_inventory_golden.json.
// The test never execs python3 or reads lib/. The golden is a snapshot:
// regenerating it needs a human decision.

const rulesInventoryGoldenPath = "testdata/rules_inventory_golden.json"

// Tokens replace the host-dependent absolute directories in the frozen payload
// (the only host-dependent field is sources.target, the resolved scanned root)
// and in the frozen tree manifest. The cache root is tokenized first: it lives
// under the CLI home and its own key hashes the TempDir path.
const (
	rootToken  = "<ROOT>"
	homeToken  = "<HOME>"
	cacheToken = "<CACHE>"
)

type treeCase struct {
	name       string
	compareErr bool
	build      func(t *testing.T, root, home string)
}

type frozenRulesInventoryCase struct {
	Name          string   `json:"name"`
	CompareStderr bool     `json:"compare_stderr"`
	Exit          int      `json:"exit"`
	Tree          []string `json:"tree"`
	Stdout        string   `json:"stdout"`
	Stderr        string   `json:"stderr"`
}

type frozenRulesInventory struct {
	Oracle map[string]any             `json:"oracle"`
	Cases  []frozenRulesInventoryCase `json:"cases"`
}

// expectedRulesInventoryGoldenCases is the independent inventory of the frozen
// bundle: deleting, duplicating or reordering a golden case fails before any
// comparison runs.
var expectedRulesInventoryGoldenCases = []string{
	"empty",
	"cursor-agents-manifest",
	"manifest-widened-bigint",
	"manifest-float-widened",
	"manifest-datetime",
	"mode-b-stack-hints",
	"manifest-parse-error",
	"recipes-list-and-monolithic-agents",
	"agents-heading-depths",
	"mdc-scalar-globs-and-snake-always",
	"skills-cache-atl",
}

func differentialTrees() []treeCase {
	writeSkill := func(t *testing.T, dir string) {
		writeFileT(t, filepath.Join(dir, "SKILL.md"), "---\nname: sample\nversion: 1.0.0\n---\nbody\n")
	}
	return []treeCase{
		{
			name: "empty",
			build: func(t *testing.T, root, home string) {
			},
		},
		{
			name: "cursor-agents-manifest",
			build: func(t *testing.T, root, home string) {
				writeFileT(t, filepath.Join(root, ".cursor", "rules", "alpha.mdc"),
					"---\ndescription: Deploy workflow rules\nglobs: [\"**/*.ts\"]\nalwaysApply: true\n---\n# Heading\nBody about pull request and worktree.\n")
				writeFileT(t, filepath.Join(root, ".cursor", "rules", "nested", "beta.mdc"),
					"---\ndescription: Legacy notes\n# not yaml comment\nalwaysApply: false\n---\nold project rules to deprecate\n")
				writeFileT(t, filepath.Join(root, ".cursorrules"),
					"Always use the trello board and the vault.\n")
				writeFileT(t, filepath.Join(root, "AGENTS.md"),
					"# Runtime Brief\n\nIntro paragraph.\n\n## Build\n\nRun tests with worktree skill here.\n\n~~~sh\n# not a heading\necho hi\n~~~\n\n### Deploy\n\nShip the pull request.\n")
				writeFileT(t, filepath.Join(root, "ai-specs", "ai-specs.toml"),
					"[agents]\nenabled = [\"claude\", \"pi\"]\n\n[recipes.worktree-flow]\nenabled = true\n\n[recipes.tdd-flow]\nenabled = false\n")
			},
		},
		{
			name: "manifest-widened-bigint",
			build: func(t *testing.T, root, home string) {
				writeFileT(t, filepath.Join(root, "ai-specs", "ai-specs.toml"),
					"[agents]\nenabled = [\"claude\", 123456789012345678901234567890]\n")
			},
		},
		{
			// Floats in a mirrored raw section: finite values keep their
			// Python repr (shortest round-trip, exponent thresholds) and the
			// TOML inf/-inf/nan literals render Infinity/-Infinity/NaN under
			// json.dumps' default allow_nan=True rather than a silent null.
			name: "manifest-float-widened",
			build: func(t *testing.T, root, home string) {
				writeFileT(t, filepath.Join(root, "ai-specs", "ai-specs.toml"),
					"[agents]\nenabled = [2.5, 2.0, 0.0001, 1e-5, 1e16, 1e22, inf, -inf, nan]\n")
			},
		},
		{
			// A raw TOML date/time in a mirrored raw section: json.dumps raises
			// TypeError and rules-inventory's main prints ERROR: to stderr and
			// exits 1, so exit code, empty stdout, and stderr must all match.
			name:       "manifest-datetime",
			compareErr: true,
			build: func(t *testing.T, root, home string) {
				writeFileT(t, filepath.Join(root, "ai-specs", "ai-specs.toml"),
					"[agents]\nenabled = [\"claude\", 1979-05-27T07:32:00Z]\n")
			},
		},
		{
			name: "mode-b-stack-hints",
			build: func(t *testing.T, root, home string) {
				writeFileT(t, filepath.Join(root, "package-lock.json"), "{}\n")
				writeFileT(t, filepath.Join(root, "pyproject.toml"), "[project]\nname = \"x\"\n")
				writeFileT(t, filepath.Join(root, "go.mod"), "module x\n")
				writeFileT(t, filepath.Join(root, ".atl", "skill-registry.md"),
					"See `atl-only` and `worktree-flow`.\n")
			},
		},
		{
			name: "manifest-parse-error",
			build: func(t *testing.T, root, home string) {
				writeFileT(t, filepath.Join(root, "ai-specs", "ai-specs.toml"), "key =\n")
				writeFileT(t, filepath.Join(root, "AGENTS.md"),
					"## Project Rules\n\nkeep this project rule.\n")
			},
		},
		{
			name: "recipes-list-and-monolithic-agents",
			build: func(t *testing.T, root, home string) {
				writeFileT(t, filepath.Join(root, "ai-specs", "ai-specs.toml"),
					"[[recipes]]\nid = \"worktree-flow\"\nenabled = true\n\n[[recipes]]\nid = \"tdd-flow\"\n")
				writeFileT(t, filepath.Join(root, "AGENTS.md"),
					"Just a simple runtime brief paragraph with no headings.\n")
			},
		},
		{
			name: "agents-heading-depths",
			build: func(t *testing.T, root, home string) {
				writeFileT(t, filepath.Join(root, "AGENTS.md"),
					"# One\nbody one\n## Two\nbody two\n### Three\nbody three\n#### Four\nnot a heading\n")
			},
		},
		{
			name: "mdc-scalar-globs-and-snake-always",
			build: func(t *testing.T, root, home string) {
				writeFileT(t, filepath.Join(root, ".cursor", "rules", "x.mdc"),
					"---\nglobs: **/*.py\nalways_apply: 1\n---\nbody x\n")
				writeFileT(t, filepath.Join(root, ".cursor", "rules", "y.mdc"),
					"---\ndescription: Empty globs\nglobs:\n---\nbody y\n")
			},
		},
		{
			name:       "skills-cache-atl",
			compareErr: true,
			build: func(t *testing.T, root, home string) {
				cache := projectcache.CacheRoot(root, home)
				writeSkill(t, filepath.Join(root, "ai-specs", "skills", "local-skill"))
				writeSkill(t, filepath.Join(root, "ai-specs", "skills", "worktree-flow"))
				writeSkill(t, filepath.Join(cache, ".recipe", "rec-a", "skills", "recipe-skill"))
				writeSkill(t, filepath.Join(cache, ".recipe", "rec-b", "skills", "recipe-skill"))
				writeSkill(t, filepath.Join(cache, ".deps", "dep-a", "skills", "dep-skill"))
				writeSkill(t, filepath.Join(cache, ".bundled", "skills", "bundled-skill"))
				writeSkill(t, filepath.Join(cache, ".bundled", "skills", "local-skill"))
				writeFileT(t, filepath.Join(root, ".cursor", "rules", "atl.mdc"),
					"---\ndescription: Trello sync rules\n---\nSync trello cards in a worktree.\n")
				writeFileT(t, filepath.Join(root, ".atl", "skill-registry.md"),
					"`trello-mcp-workflow`, `recipe-skill`\n")
			},
		},
	}
}

type pathToken struct{ path, token string }

// frozenTokens lists the host-dependent directories of one case, longest-lived
// prefix first: the cache root lives under the CLI home.
func frozenTokens(root, home string) []pathToken {
	return []pathToken{
		{projectcache.CacheRoot(root, home), cacheToken},
		{projectcache.ResolvePath(root), rootToken},
		{projectcache.ResolvePath(home), homeToken},
	}
}

func applyTokens(s string, tokens []pathToken) string {
	for _, tk := range tokens {
		s = strings.ReplaceAll(s, tk.path, tk.token)
	}
	return s
}

// treeManifest describes the tree a case builds as "sha256  <tokenized path>"
// lines sorted by path, so the frozen golden pins exactly the inputs the oracle
// was captured from.
func treeManifest(t *testing.T, root, home string) []string {
	t.Helper()
	tokens := frozenTokens(root, home)
	type entry struct{ path, line string }
	var entries []entry
	for _, dir := range []string{root, home} {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			tokenized := applyTokens(projectcache.ResolvePath(path), tokens)
			entries = append(entries, entry{tokenized, hex.EncodeToString(sum[:]) + "  " + tokenized})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = e.line
	}
	return lines
}

func loadRulesInventoryGolden(t *testing.T) frozenRulesInventory {
	t.Helper()
	raw, err := os.ReadFile(rulesInventoryGoldenPath)
	if err != nil {
		t.Fatalf("read frozen oracle %s: %v", rulesInventoryGoldenPath, err)
	}
	var golden frozenRulesInventory
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode frozen oracle %s: %v", rulesInventoryGoldenPath, err)
	}
	return golden
}

// TestDifferentialRulesInventory compares the native scanner with the frozen
// legacy oracle across every fabricated tree: exit code, stdout and stderr.
func TestDifferentialRulesInventory(t *testing.T) {
	golden := loadRulesInventoryGolden(t)
	corpus := differentialTrees()

	if len(corpus) != len(expectedRulesInventoryGoldenCases) {
		t.Fatalf("in-test corpus has %d cases, inventory %d", len(corpus), len(expectedRulesInventoryGoldenCases))
	}
	if len(golden.Cases) != len(expectedRulesInventoryGoldenCases) {
		t.Fatalf("frozen oracle has %d cases, inventory %d", len(golden.Cases), len(expectedRulesInventoryGoldenCases))
	}
	seen := map[string]bool{}
	for i, want := range expectedRulesInventoryGoldenCases {
		if seen[want] {
			t.Fatalf("inventory lists %q twice", want)
		}
		seen[want] = true
		if golden.Cases[i].Name != want || corpus[i].name != want {
			t.Fatalf("case %d: golden %q, corpus %q, inventory %q", i, golden.Cases[i].Name, corpus[i].name, want)
		}
	}

	for i, tc := range corpus {
		t.Run(tc.name, func(t *testing.T) {
			frozen := golden.Cases[i]
			if frozen.CompareStderr != tc.compareErr {
				t.Fatalf("%s: frozen compare_stderr = %v, corpus %v", tc.name, frozen.CompareStderr, tc.compareErr)
			}
			root := t.TempDir()
			home := t.TempDir()
			tc.build(t, root, home)

			gotTree := treeManifest(t, root, home)
			if strings.Join(gotTree, "\n") != strings.Join(frozen.Tree, "\n") {
				t.Fatalf("%s: frozen inputs differ from the corpus tree:\n--- frozen ---\n%s\n--- corpus ---\n%s",
					tc.name, strings.Join(frozen.Tree, "\n"), strings.Join(gotTree, "\n"))
			}

			var gout, gerr bytes.Buffer
			gcode := Run(root, home, &gout, &gerr)
			tokens := frozenTokens(root, home)

			if gcode != frozen.Exit {
				t.Errorf("exit = %d, legacy %d\nport stderr: %s\nlegacy stderr: %s",
					gcode, frozen.Exit, gerr.String(), frozen.Stderr)
			}
			if got := applyTokens(gout.String(), tokens); got != frozen.Stdout {
				t.Errorf("stdout mismatch:\n--- port ---\n%s\n--- legacy ---\n%s", got, frozen.Stdout)
			}
			if got := applyTokens(gerr.String(), tokens); got != frozen.Stderr {
				t.Errorf("stderr mismatch:\n--- port ---\n%q\n--- legacy ---\n%q", got, frozen.Stderr)
			}
		})
	}
}
