package sync

// Differential contract tests for the native materializer spine (GO-07 S14a
// WU1). The Go authority is unwired; these tests pin it against the frozen
// oracle bundle testdata/materialize_golden.json captured once from the REAL
// Python orchestration (testdata/materialize_ref.py, PR2-B S10) and require
// byte equality of the emitted lines and documents.
//
// The capture pinned the Python leg to the freshly built worktree-gate binary
// through WORKTREE_GATE_BIN so the fail-open GO_*_BRIDGE_FALLBACK warnings never
// masked the orchestration under test. Both legs ran against the SAME project
// and home directories, so any absolute path embedded in a document is
// identical; the frozen records tokenize those roots (<HOME>, <PROJECT>) and
// this test normalizes the Go side symmetrically.
//
// This file also owns the stamp-step manifest differential: the reconcile
// defaults the gate plans are written into [recipes.<id>.config] before any
// artifact actuation. Those cases freeze the manifest bytes the authority left
// behind, plus the authority-stamped input of the no-op case.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

// --- frozen oracle ----------------------------------------------------------

// materializeGoldenCase is one frozen record of testdata/materialize_golden.json.
// The kind selects which fields the owning family compares:
//   - spine: rc/stdout/stderr plus the requested documents (and the marker temp
//     document when the case wrote no explicit recipe-MCP path);
//   - stamp: the seed manifest and the post-run manifest bytes;
//   - loop: rc/stdout/stderr plus the documents, the whole project+cache tree
//     and the lock bytes inside it.
type materializeGoldenCase struct {
	Kind           string                    `json:"kind"` // spine | stamp | loop
	RC             int                       `json:"rc"`
	Stdout         string                    `json:"stdout"`
	Stderr         string                    `json:"stderr"`
	RecipeMCP      string                    `json:"recipe_mcp,omitempty"`
	ResolvedConfig string                    `json:"resolved_config,omitempty"`
	ResolvedHooks  string                    `json:"resolved_hooks,omitempty"`
	TempDocument   string                    `json:"temp_document,omitempty"`
	Manifest       string                    `json:"manifest,omitempty"`
	SeedManifest   string                    `json:"seed_manifest,omitempty"`
	Tree           map[string]string         `json:"tree,omitempty"`
	Tokens         map[string]map[string]int `json:"tokens,omitempty"`
}

// materializeGoldenIdentity is one entry of the hard-coded identity inventory.
type materializeGoldenIdentity struct {
	Kind, Name string
}

// materializeGoldenInventory is the hard-coded identity inventory of the frozen
// bundle: the exact case names in capture order and their kind. It is an
// independent oracle of coverage, kept in the test and never derived from the
// bundle, so a missing, renamed, duplicated or reordered record fails before
// any case loop runs.
var materializeGoldenInventory = []materializeGoldenIdentity{
	{"spine", "no-enabled branch writes both temp documents"},
	{"spine", "single enabled recipe and the empty resolved-hooks dump"},
	{"spine", "several enabled recipes in manifest order with an explicit recipe-MCP path"},
	{"spine", "recipe-MCP temp write without an output path prints the marker"},
	{"spine", "fatal primitive conflict"},
	{"spine", "schema-invalid primitive-coherent recipes report the same fatal conflict"},
	{"spine", "schema-invalid tagged recipes report the same tag warning and fatal conflict"},
	{"spine", "resolved-config carries catalog defaults and brief fragments"},
	{"spine", "resolved-config renders catalog default value types"},
	{"spine", "no-enabled resolved-config is not enriched"},
	{"spine", "advisory tag conflict"},
	{"stamp", "added recipe writes new config values"},
	{"stamp", "updated value rewrites the stale config"},
	{"stamp", "no-op run leaves the already-stamped manifest untouched"},
	{"stamp", "sorted-key reconcile order matches the bridge"},
	{"loop", "bundled skill and command with an overwrite warning and the lock bytes"},
	{"loop", "templates fresh managed-current and user-modified"},
	{"loop", "docs fresh managed-current and user-modified"},
	{"loop", "runtime hook fresh collects the resolved entry"},
	{"loop", "runtime hook managed-current is skipped and baselined"},
	{"loop", "runtime hook user-modified is preserved"},
	{"loop", "runtime hook --refresh-gates backs up and rewrites"},
	{"loop", "resolved-hooks env exports only the UPPER env-shaped keys"},
	{"loop", "dep skill through the documented delegation"},
	{"loop", "execute_hooks failure fails the sync with the fatal line"},
	{"loop", "worktree-flow gate acquisition degrades offline"},
}

// materializeGoldenKindCounts is the per-kind count of the inventory above.
var materializeGoldenKindCounts = map[string]int{"spine": 11, "stamp": 4, "loop": 11}

const (
	// materializeGoldenHomeToken is the tokenized ai-specs home fixture root.
	materializeGoldenHomeToken = "<HOME>"
	// materializeGoldenProjectToken is the tokenized project fixture root.
	materializeGoldenProjectToken = "<PROJECT>"
	// materializeGoldenCacheToken is the tokenized per-project CLI cache
	// directory basename (<hash>-<project>); its hash derives from the fixture
	// project path, so it differs on every run.
	materializeGoldenCacheToken = "<CACHE_PROJECT>"
)

// materializeGoldenTokenInventory is the hard-coded host-path token inventory
// of the frozen bundle: which record carries which token, in which field, and
// how often. Only these positions may carry a host path; anything else is a
// bundle mutation, not a host-path occurrence. "tree" counts occurrences across
// every value of the loop record's project+cache tree.
var materializeGoldenTokenInventory = map[string]map[string]map[string]int{
	"no-enabled branch writes both temp documents":                 {"resolved_config": {"<PROJECT>": 1}},
	"resolved-config carries catalog defaults and brief fragments": {"resolved_config": {"<PROJECT>": 1}},
	"resolved-config renders catalog default value types":          {"resolved_config": {"<PROJECT>": 1}},
	"no-enabled resolved-config is not enriched":                   {"resolved_config": {"<PROJECT>": 1}},
	"bundled skill and command with an overwrite warning and the lock bytes": {
		"stderr": {"<CACHE_PROJECT>": 1, "<HOME>": 1},
		"tree":   {"<CACHE_PROJECT>": 3},
	},
	"runtime hook --refresh-gates backs up and rewrites": {"tree": {"<CACHE_PROJECT>": 1}},
	"dep skill through the documented delegation": {
		"stdout": {"<HOME>": 1},
		"tree":   {"<CACHE_PROJECT>": 2, "<HOME>": 1},
	},
	"worktree-flow gate acquisition degrades offline": {"stderr": {"<HOME>": 1}},
}

// checkMaterializeGoldenInventory validates one loaded bundle against the
// hard-coded identity, per-kind and token inventories. It is pure so the
// focused control can run it against a mutated in-memory copy.
func checkMaterializeGoldenInventory(names []string, cases []materializeGoldenCase) error {
	if len(names) != len(materializeGoldenInventory) {
		return fmt.Errorf("frozen oracle has %d names, want %d", len(names), len(materializeGoldenInventory))
	}
	if len(cases) != len(names) {
		return fmt.Errorf("frozen oracle has %d records for %d names", len(cases), len(names))
	}
	seen := make(map[string]bool, len(names))
	kinds := map[string]int{}
	for i, tc := range cases {
		want := materializeGoldenInventory[i]
		if names[i] != want.Name {
			return fmt.Errorf("frozen oracle record %d is %q, want %q (identity mismatch)", i, names[i], want.Name)
		}
		if seen[names[i]] {
			return fmt.Errorf("frozen oracle duplicates record %q", names[i])
		}
		seen[names[i]] = true
		if tc.Kind != want.Kind {
			return fmt.Errorf("frozen oracle record %q has kind %q, want %q", names[i], tc.Kind, want.Kind)
		}
		kinds[tc.Kind]++
		wantTokens, hasTokens := materializeGoldenTokenInventory[names[i]]
		if !hasTokens && len(tc.Tokens) != 0 {
			return fmt.Errorf("frozen oracle record %q carries a non-inventoried token %v", names[i], tc.Tokens)
		}
		if hasTokens && len(tc.Tokens) != len(wantTokens) {
			return fmt.Errorf("frozen oracle record %q tokenizes %d fields, want %d", names[i], len(tc.Tokens), len(wantTokens))
		}
		for field, tokens := range tc.Tokens {
			switch field {
			case "stdout", "stderr", "recipe_mcp", "resolved_config", "resolved_hooks", "temp_document", "manifest", "tree":
			default:
				return fmt.Errorf("frozen oracle record %q tokenizes unknown field %q", names[i], field)
			}
			wantField, ok := wantTokens[field]
			if !ok || len(tokens) != len(wantField) {
				return fmt.Errorf("frozen oracle record %q tokenizes %v in %s, want %v", names[i], tokens, field, wantField)
			}
			for token, n := range tokens {
				if wantField[token] != n {
					return fmt.Errorf("frozen oracle record %q has %d %s tokens in %s, want %d", names[i], n, token, field, wantField[token])
				}
			}
		}
	}
	for kind, want := range materializeGoldenKindCounts {
		if kinds[kind] != want {
			return fmt.Errorf("frozen oracle has %d %s records, want %d", kinds[kind], kind, want)
		}
	}
	return nil
}

// loadMaterializeGolden validates and indexes testdata/materialize_golden.json.
// The inventory is asserted before any case loop runs.
func loadMaterializeGolden(t *testing.T) map[string]materializeGoldenCase {
	t.Helper()
	var bundle struct {
		Names []string                `json:"names"`
		Cases []materializeGoldenCase `json:"cases"`
	}
	if err := json.Unmarshal(readGoldenBundle(t, "materialize_golden.json"), &bundle); err != nil {
		t.Fatalf("parse frozen oracle: %v", err)
	}
	if err := checkMaterializeGoldenInventory(bundle.Names, bundle.Cases); err != nil {
		t.Fatalf("frozen materialize oracle: %v", err)
	}
	golden := make(map[string]materializeGoldenCase, len(bundle.Cases))
	for i, rec := range bundle.Cases {
		golden[bundle.Names[i]] = rec
	}
	return golden
}

// frozenMaterializeRecord looks up one frozen record by its identity name.
func frozenMaterializeRecord(t *testing.T, golden map[string]materializeGoldenCase, name string) materializeGoldenCase {
	t.Helper()
	rec, ok := golden[name]
	if !ok {
		t.Fatalf("frozen record %q missing", name)
	}
	return rec
}

// TestMaterializeFrozenInventoryControl proves the inventory validation bites:
// the intact bundle validates, while renaming, removing, duplicating or
// mutating a record (all in memory, never on disk) fails loudly.
func TestMaterializeFrozenInventoryControl(t *testing.T) {
	var bundle struct {
		Names []string                `json:"names"`
		Cases []materializeGoldenCase `json:"cases"`
	}
	if err := json.Unmarshal(readGoldenBundle(t, "materialize_golden.json"), &bundle); err != nil {
		t.Fatalf("parse frozen oracle: %v", err)
	}
	if err := checkMaterializeGoldenInventory(bundle.Names, bundle.Cases); err != nil {
		t.Fatalf("intact inventory must validate: %v", err)
	}

	renamed := append([]string{}, bundle.Names...)
	renamed[0] = "renamed record"
	if err := checkMaterializeGoldenInventory(renamed, bundle.Cases); err == nil {
		t.Fatal("a renamed record must fail the inventory validation")
	}

	missing := append(append([]materializeGoldenCase{}, bundle.Cases[:1]...), bundle.Cases[2:]...)
	if err := checkMaterializeGoldenInventory(bundle.Names, missing); err == nil {
		t.Fatal("a removed record must fail the inventory validation")
	}

	duplicated := append(append([]materializeGoldenCase{}, bundle.Cases...), bundle.Cases[0])
	doubledNames := append(append([]string{}, bundle.Names...), bundle.Names[0])
	if err := checkMaterializeGoldenInventory(doubledNames, duplicated); err == nil {
		t.Fatal("a duplicated record must fail the inventory validation")
	}

	mutated := append([]materializeGoldenCase{}, bundle.Cases...)
	mutated[0].Kind = "mutated"
	if err := checkMaterializeGoldenInventory(bundle.Names, mutated); err == nil {
		t.Fatal("a mutated kind must fail the inventory validation")
	}
}

// freezeMaterializePaths replaces both spellings (raw and
// EvalSymlinks-resolved) of the home and project fixture roots with their
// tokens, symmetric with the capture-time tokenization.
func freezeMaterializePaths(s, home, project string) string {
	pairs := [][2]string{
		{materializeGoldenHomeToken, home},
		{materializeGoldenProjectToken, project},
	}
	if home != "" && project != "" {
		pairs = append(pairs, [2]string{materializeGoldenCacheToken, filepath.Base(projectcache.CacheRoot(project, home))})
	}
	for _, pair := range pairs {
		token, path := pair[0], pair[1]
		if path == "" {
			continue
		}
		// The fixture root may carry an unclean spelling (a TMPDIR with a
		// trailing slash yields "T//x"), while the emitted value is Cleaned.
		spellings := []string{path, filepath.Clean(path)}
		if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
			spellings = append([]string{resolved}, spellings...)
		}
		for _, spelling := range spellings {
			s = strings.ReplaceAll(s, spelling, token)
		}
	}
	return s
}

// freezeMaterializeTree tokenizes every key and value of a project+cache tree.
func freezeMaterializeTree(t *testing.T, tree map[string]string, home, project string) map[string]string {
	t.Helper()
	out := make(map[string]string, len(tree))
	for key, value := range tree {
		out[freezeMaterializePaths(key, home, project)] = freezeMaterializePaths(value, home, project)
	}
	return out
}

// --- harness ----------------------------------------------------------------

var materializeGateDir string

// materializeGateBinary builds the worktree-gate binary once per test run and
// returns its path. The Python bridges require a verified binary; without it
// every bridge announces a GO_*_BRIDGE_FALLBACK warning that is not part of the
// orchestration under test.
func materializeGateBinary(t *testing.T) string {
	t.Helper()
	if materializeGateDir != "" {
		return filepath.Join(materializeGateDir, "worktree-gate")
	}
	module, err := filepath.Abs(filepath.Join("..", "..", "catalog", "recipes", "worktree-flow", "gate"))
	if err != nil {
		t.Fatalf("resolve gate module: %v", err)
	}
	dir, err := os.MkdirTemp("", "materialize-gate-")
	if err != nil {
		t.Fatalf("temp gate dir: %v", err)
	}
	out := filepath.Join(dir, "worktree-gate")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = module
	cmd.Env = os.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Skipf("worktree-gate build unavailable: %v: %s", err, stderr.String())
	}
	materializeGateDir = dir
	return out
}

// normalizeRecipeMCPPath replaces the nondeterministic mkstemp path on the
// RECIPE_MCP_TEMP marker so the two legs' marker lines compare equal.
func normalizeRecipeMCPPath(stdout string) string {
	lines := strings.Split(stdout, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "RECIPE_MCP_TEMP:") {
			lines[i] = "RECIPE_MCP_TEMP:<TEMP>"
		}
	}
	return strings.Join(lines, "\n")
}

// markerPath extracts the path the leg printed on the RECIPE_MCP_TEMP marker.
func markerPath(stdout string) string {
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "RECIPE_MCP_TEMP:") {
			return strings.TrimPrefix(line, "RECIPE_MCP_TEMP:")
		}
	}
	return ""
}

// --- fixture ----------------------------------------------------------------

// materializeFixture writes one project manifest and its catalog recipes. The
// returned paths are the SAME for both legs.
type materializeFixture struct {
	home    string
	project string
}

func writeMaterializeFixture(t *testing.T, recipes map[string]string, manifest string) materializeFixture {
	t.Helper()
	home := t.TempDir()
	project := t.TempDir()
	catalog := filepath.Join(home, "catalog", "recipes")
	for id, body := range recipes {
		dir := filepath.Join(catalog, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir recipe %s: %v", id, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "recipe.toml"), []byte(body), 0o644); err != nil {
			t.Fatalf("write recipe %s: %v", id, err)
		}
	}
	aiSpecs := filepath.Join(project, "ai-specs")
	if err := os.MkdirAll(aiSpecs, 0o755); err != nil {
		t.Fatalf("mkdir ai-specs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(aiSpecs, "ai-specs.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return materializeFixture{home: home, project: project}
}

// --- scenarios --------------------------------------------------------------

type materializeScenario struct {
	name              string
	recipes           map[string]string
	manifest          string
	recipeMCPDoc      bool
	recipeMCPOut      bool
	resolvedConfigOut bool
	resolvedHooksOut  bool
}

func materializeRecipeBody(id, name string) string {
	return syncRecipeHeader(id, name)
}

func materializeRecipeBodyWith(text string) string {
	return text
}

// materializeSpineScenarios is the shared scenario set of the spine
// differential: the frozen capture and the frozen test both drive it.
func materializeSpineScenarios() []materializeScenario {
	return []materializeScenario{
		{
			name: "no-enabled branch writes both temp documents",
			recipes: map[string]string{
				"alpha": materializeRecipeBody("alpha", "Alpha"),
			},
			manifest:          "[recipes.alpha]\nenabled = false\n",
			resolvedConfigOut: true,
			resolvedHooksOut:  true,
		},
		{
			name: "single enabled recipe and the empty resolved-hooks dump",
			recipes: map[string]string{
				"alpha": materializeRecipeBody("alpha", "Alpha"),
			},
			manifest:         "[agents]\nenabled = [\"claude\"]\n\n[recipes.alpha]\nenabled = true\n",
			recipeMCPDoc:     true,
			resolvedHooksOut: true,
		},
		{
			name: "several enabled recipes in manifest order with an explicit recipe-MCP path",
			recipes: map[string]string{
				"alpha": materializeRecipeBody("alpha", "Alpha"),
				"beta":  materializeRecipeBody("beta", "Beta"),
				"gamma": materializeRecipeBody("gamma", "Gamma"),
			},
			manifest:     "[recipes.alpha]\nenabled = true\n\n[recipes.beta]\nenabled = true\n\n[recipes.gamma]\nenabled = true\n",
			recipeMCPDoc: true,
			recipeMCPOut: true,
		},
		{
			name: "recipe-MCP temp write without an output path prints the marker",
			recipes: map[string]string{
				"alpha": materializeRecipeBody("alpha", "Alpha"),
			},
			manifest:     "[recipes.alpha]\nenabled = true\n",
			recipeMCPDoc: true,
		},
		{
			name: "fatal primitive conflict",
			recipes: map[string]string{
				"alpha": materializeRecipeBodyWith(syncRecipeHeader("alpha", "Alpha") + "[[provides.skills]]\nid = \"dup\"\nsource = \"bundled\"\npath = \"skills/dup\"\n"),
				"beta":  materializeRecipeBodyWith(syncRecipeHeader("beta", "Beta") + "[[provides.skills]]\nid = \"dup\"\nsource = \"bundled\"\npath = \"skills/dup\"\n"),
			},
			manifest:     "[recipes.alpha]\nenabled = true\n\n[recipes.beta]\nenabled = true\n",
			recipeMCPDoc: false,
		},
		{
			name: "schema-invalid primitive-coherent recipes report the same fatal conflict",
			recipes: map[string]string{
				"alpha": "[recipe]\nid = \"alpha\"\nname = \"Alpha\"\n\n[[provides.skills]]\nid = \"dup\"\n",
				"beta":  "[recipe]\nid = \"beta\"\nname = \"Beta\"\n\n[[provides.skills]]\nid = \"dup\"\n",
			},
			manifest:     "[recipes.alpha]\nenabled = true\n\n[recipes.beta]\nenabled = true\n",
			recipeMCPDoc: false,
		},
		{
			name: "schema-invalid tagged recipes report the same tag warning and fatal conflict",
			recipes: map[string]string{
				"alpha": "[recipe]\nid = \"alpha\"\nname = \"Alpha\"\ntags = [\"vcs\"]\nconflicts_with = [\"beta\"]\n\n[[provides.skills]]\nid = \"dup\"\n",
				"beta":  "[recipe]\nid = \"beta\"\nname = \"Beta\"\ntags = [\"vcs\"]\n\n[[provides.skills]]\nid = \"dup\"\n",
			},
			manifest:     "[recipes.alpha]\nenabled = true\n\n[recipes.beta]\nenabled = true\n",
			recipeMCPDoc: false,
		},
		{
			// S14b: catalog [config] defaults fill absent keys (manifest wins)
			// and [provides.brief] fragments attach per enabled recipe; a recipe
			// without a brief block gets {}.
			name: "resolved-config carries catalog defaults and brief fragments",
			recipes: map[string]string{
				"alpha": materializeRecipeBodyWith(syncRecipeHeader("alpha", "Alpha") +
					"\n[config.base_branch]\nrequired = false\ntype = \"string\"\ndefault = \"development\"\n" +
					"\n[config.mode]\nrequired = false\ntype = \"string\"\ndefault = \"strict\"\n" +
					"\n[config.ratio]\nrequired = false\ntype = \"float\"\ndefault = 2.0\n" +
					"\n[config.label]\nrequired = false\ntype = \"string\"\n" +
					"\n[provides.brief]\nworkflow_rules = [\"Rule one\", \"Rule two\"]\nuseful_commands = [{key = \"run\", text = \"make run\"}]\n"),
				"beta": materializeRecipeBody("beta", "Beta"),
			},
			manifest:          "[recipes.alpha]\nenabled = true\n\n[recipes.alpha.config]\nmode = \"loose\"\n\n[recipes.beta]\nenabled = true\n",
			recipeMCPDoc:      true,
			resolvedConfigOut: true,
		},
		{
			// S14b value fidelity at this writer: the projected manifest values
			// keep the Go envelope's rendering (Python decodes that envelope, so a
			// manifest 1.0 is 1 on both legs), while catalog defaults keep Python's
			// own tomllib types (a default 2.0 stays 2.0 and a wide integer keeps
			// its digits).
			name: "resolved-config renders catalog default value types",
			recipes: map[string]string{
				"alpha": materializeRecipeBodyWith(syncRecipeHeader("alpha", "Alpha") +
					"\n[config.ratio]\nrequired = false\ntype = \"float\"\ndefault = 2.0\n" +
					"\n[config.huge]\nrequired = false\ntype = \"int\"\ndefault = 123456789012345678901234567890\n"),
			},
			manifest:          "[recipes.alpha]\nenabled = true\nweight = 1.0\n",
			recipeMCPDoc:      true,
			resolvedConfigOut: true,
		},
		{
			// S14b: the no-enabled branch stays the raw projection (Python does
			// not enrich it), even when the disabled recipe declares defaults.
			name: "no-enabled resolved-config is not enriched",
			recipes: map[string]string{
				"alpha": materializeRecipeBodyWith(syncRecipeHeader("alpha", "Alpha") +
					"\n[config.base_branch]\nrequired = false\ntype = \"string\"\ndefault = \"development\"\n" +
					"\n[provides.brief]\nworkflow_rules = [\"Rule one\"]\n"),
			},
			manifest:          "[recipes.alpha]\nenabled = false\nweight = 1.0\n",
			resolvedConfigOut: true,
		},
		{
			name: "advisory tag conflict",
			recipes: map[string]string{
				"alpha": materializeRecipeBodyWith(syncRecipeHeader("alpha", "Alpha") + "tags = [\"vcs\"]\n"),
				"beta":  materializeRecipeBodyWith(syncRecipeHeader("beta", "Beta") + "tags = [\"vcs\"]\n"),
			},
			manifest:     "[recipes.alpha]\nenabled = true\n\n[recipes.beta]\nenabled = true\n",
			recipeMCPDoc: true,
		},
	}
}

// TestMaterializeSpineDifferentialFrozen pins the native spine against the
// frozen records captured once from the real Python orchestration (PR2-B S10):
// rc, the emitted lines, and each requested document must agree scenario by
// scenario.
func TestMaterializeSpineDifferentialFrozen(t *testing.T) {
	golden := loadMaterializeGolden(t)

	for _, scenario := range materializeSpineScenarios() {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			ref := frozenMaterializeRecord(t, golden, scenario.name)
			fixture := writeMaterializeFixture(t, scenario.recipes, scenario.manifest)
			goDir := t.TempDir()

			goMCP := "-"
			if scenario.recipeMCPOut {
				goMCP = filepath.Join(goDir, "recipe-mcp.json")
			}
			goConfig := "-"
			if scenario.resolvedConfigOut {
				goConfig = filepath.Join(goDir, "resolved-config.json")
			}
			goHooks := "-"
			if scenario.resolvedHooksOut {
				goHooks = filepath.Join(goDir, "resolved-hooks.json")
			}

			var mcpOut, configOut, hooksOut *string
			if scenario.recipeMCPOut {
				mcpOut = &goMCP
			}
			if scenario.resolvedConfigOut {
				configOut = &goConfig
			}
			if scenario.resolvedHooksOut {
				hooksOut = &goHooks
			}
			var stdout, stderr bytes.Buffer
			result, goErr := MaterializeRecipes(MaterializeRequest{
				ProjectRoot:       fixture.project,
				AISpecsHome:       fixture.home,
				RecipeMCPOut:      mcpOut,
				ResolvedConfigOut: configOut,
				ResolvedHooksOut:  hooksOut,
				Stdout:            &stdout,
				Stderr:            &stderr,
			})
			goRC := 0
			if goErr != nil {
				goRC = 1
			}
			if goRC != ref.RC {
				t.Fatalf("rc: go=%d frozen=%d\n--- go stderr ---\n%s\n--- frozen stderr ---\n%s", goRC, ref.RC, stderr.String(), ref.Stderr)
			}
			if got := freezeMaterializePaths(normalizeRecipeMCPPath(stdout.String()), fixture.home, fixture.project); got != ref.Stdout {
				t.Fatalf("stdout:\n--- go ---\n%q\n--- frozen ---\n%q", got, ref.Stdout)
			}
			if got := freezeMaterializePaths(stderr.String(), fixture.home, fixture.project); got != ref.Stderr {
				t.Fatalf("stderr:\n--- go ---\n%q\n--- frozen ---\n%q", got, ref.Stderr)
			}

			// recipe-MCP document: explicit path or the marker's temp file.
			switch {
			case scenario.recipeMCPOut:
				got := freezeMaterializePaths(result.RecipeMCPDocument, fixture.home, fixture.project)
				assertDocumentEqual(t, "recipe-mcp", got, ref.RecipeMCP)
			case scenario.recipeMCPDoc:
				goTemp := markerPath(stdout.String())
				if goTemp == "" {
					t.Fatalf("RECIPE_MCP_TEMP marker missing: %q", stdout.String())
				}
				goDoc, err := os.ReadFile(goTemp)
				if err != nil {
					t.Fatalf("read go temp: %v", err)
				}
				got := freezeMaterializePaths(string(goDoc), fixture.home, fixture.project)
				assertDocumentEqual(t, "recipe-mcp", got, ref.TempDocument)
				if result.RecipeMCPPath != goTemp {
					t.Fatalf("result path = %q, want the marker path %q", result.RecipeMCPPath, goTemp)
				}
			}

			if scenario.resolvedConfigOut {
				got := freezeMaterializePaths(result.ResolvedConfigDocument, fixture.home, fixture.project)
				assertDocumentEqual(t, "resolved-config", got, ref.ResolvedConfig)
			}
			if scenario.resolvedHooksOut {
				got := freezeMaterializePaths(result.ResolvedHooksDocument, fixture.home, fixture.project)
				assertDocumentEqual(t, "resolved-hooks", got, ref.ResolvedHooks)
			}
		})
	}
}

func assertDocumentEqual(t *testing.T, label, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s document:\n--- go ---\n%q\n--- frozen ---\n%q", label, got, want)
	}
}

// --- stamp-step manifest differential (GO-07 S14a follow-up) -----------------
//
// The stamp step MUTATES the user's manifest: the reconcile defaults the gate
// plans are written into [recipes.<id>.config] before any artifact actuation.
// These scenarios froze the manifest bytes the Python authority left behind on
// two byte-identical project copies. The Python leg reached the same shared
// writer through the gate's --write-recipe-config bridge (the reconcile-stamp
// decision and the write decision are both Go), so this is the strongest
// available check that the in-process PlanReconcileStamps ->
// materializeStampOrdered -> shared.ApplyRecipeConfigWrite path is
// byte-faithful.

// materializeReconcileRecipe is a schema-valid recipe declaring a reconcile
// mapping that selects two config-field defaults. Both the whole reconcile table
// and the two scalar defaults are stamped into the manifest.
func materializeReconcileRecipe(id, name string) string {
	return syncRecipeHeader(id, name) + `
[config.reconcile]
scope_field = "workflow"

[[config.reconcile.expectations]]
event = "pr_opened"
config_field = "base_branch"

[config.workflow]
required = true
default = "feature"

[config.base_branch]
required = true
default = "main"
`
}

// materializeSortedStampRecipe declares its [config.reconcile] keys out of
// alphabetical order and selects three defaults, so the inline-table key order
// of the written reconcile value (and of every nested expectation) is
// observable. The Go-side PlanReconcileStamps returns a map whose iteration
// order is random; only the JSON round-trip through the bridge's order makes the
// bytes deterministic and Python-identical.
func materializeSortedStampRecipe(id, name string) string {
	return syncRecipeHeader(id, name) + `
[config.reconcile]
scope_field = "workflow"
max_age_seconds = 3600

[[config.reconcile.expectations]]
event = "pr_opened"
config_field = "base_branch"
config_field_when_set = "sync_state"

[config.workflow]
required = true
default = "feature"

[config.base_branch]
required = true
default = "main"

[config.sync_state]
required = true
default = "open"
`
}

// writeMaterializeManifest writes one manifest into <project>/ai-specs.
func writeMaterializeManifest(t *testing.T, project string, data []byte) {
	t.Helper()
	dir := filepath.Join(project, "ai-specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir ai-specs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ai-specs.toml"), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// materializeStampScenario is one stamp-step differential case.
type materializeStampScenario struct {
	name      string
	recipes   map[string]string
	manifest  string // literal seed; empty means the authority-stamped output
	untouched bool
}

// materializeStampScenarios is the shared scenario set of the stamp
// differential: the frozen capture and the frozen test both drive it.
func materializeStampScenarios() []materializeStampScenario {
	reconcileRecipes := map[string]string{"alpha": materializeReconcileRecipe("alpha", "Alpha")}
	sortedRecipes := map[string]string{"alpha": materializeSortedStampRecipe("alpha", "Alpha")}
	return []materializeStampScenario{
		{
			name:     "added recipe writes new config values",
			recipes:  reconcileRecipes,
			manifest: "[recipes.alpha]\nenabled = true\n",
		},
		{
			name:     "updated value rewrites the stale config",
			recipes:  reconcileRecipes,
			manifest: "[recipes.alpha]\nenabled = true\n\n[recipes.alpha.config]\nworkflow = \"stale\"\n",
		},
		{
			name:      "no-op run leaves the already-stamped manifest untouched",
			recipes:   reconcileRecipes,
			manifest:  "",
			untouched: true,
		},
		{
			name:     "sorted-key reconcile order matches the bridge",
			recipes:  sortedRecipes,
			manifest: "[recipes.alpha]\nenabled = true\n",
		},
	}
}

// TestMaterializeSpineStampManifestDifferentialFrozen compares the manifest
// bytes the native stamp step leaves behind against the frozen records for an
// added recipe (new values), an updated value, a no-op run, and a reconcile
// table whose declaration order differs from the sorted inline-table order the
// bridge emits. The no-op input comes from the frozen authority-stamped
// manifest.
func TestMaterializeSpineStampManifestDifferentialFrozen(t *testing.T) {
	golden := loadMaterializeGolden(t)

	for _, scenario := range materializeStampScenarios() {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			ref := frozenMaterializeRecord(t, golden, scenario.name)
			seed := scenario.manifest
			if seed == "" {
				seed = ref.SeedManifest
			}
			fixture := writeMaterializeFixture(t, scenario.recipes, seed)
			goManifest := filepath.Join(fixture.project, "ai-specs", "ai-specs.toml")
			goBefore, err := os.Stat(goManifest)
			if err != nil {
				t.Fatalf("stat go manifest: %v", err)
			}

			var stdout, stderr bytes.Buffer
			if _, err := MaterializeRecipes(MaterializeRequest{
				ProjectRoot: fixture.project,
				AISpecsHome: fixture.home,
				Stdout:      &stdout,
				Stderr:      &stderr,
			}); err != nil {
				t.Fatalf("go leg failed: %v\nstderr: %s", err, stderr.String())
			}

			goBytes, err := os.ReadFile(goManifest)
			if err != nil {
				t.Fatalf("read go manifest: %v", err)
			}
			if got := freezeMaterializePaths(string(goBytes), fixture.home, fixture.project); got != ref.Manifest {
				t.Fatalf("manifest bytes diverge\n--- go ---\n%q\n--- frozen ---\n%q", got, ref.Manifest)
			}
			if scenario.untouched {
				goAfter, err := os.Stat(goManifest)
				if err != nil {
					t.Fatalf("stat go manifest after: %v", err)
				}
				if !os.SameFile(goBefore, goAfter) {
					t.Fatalf("a no-op run rewrote the manifest (inode changed): %q", goBytes)
				}
			}
		})
	}
}
