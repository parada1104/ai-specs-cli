package recipe

// Port of lib/_internal/recipe-init.py (GO-20 U7-2). Every test names the S#
// it pins; the fixtures mirror tests/test_recipe_init.py's black-box trees.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const initTrackerRecipeBody = `[recipe]
id = "tracker"
name = "Tracker"
description = "Tracker setup"
version = "1.0"

[init]
prompt = "init.md"
description = "Configure tracker"
needs_manifest = true
needs_mcp = ["trello", "missing-mcp"]

[config.board_id]
required = true
type = "string"

[config.timeout]
required = false
type = "integer"
default = 30

[[provides.mcp]]
id = "trello"
command = "npx"
args = ["-y", "@recipe/trello"]
env = { API_TOKEN = "recipe-secret" }

[[provides.templates]]
source = "templates/mapping.toml"
target = "ai-specs/trello-mapping.toml"
condition = "not_exists"
`

const initTrackerPrompt = "# Tracker init\nChoose a board and list mapping.\n"

// initProject builds a project tree plus a catalogue home carrying the
// tracker recipe, mirroring _make_project/_make_cli_home.
func initProject(t *testing.T, installed bool, configBlock, recipeBody string) (root, home string) {
	t.Helper()
	root, home = t.TempDir(), t.TempDir()
	manifest := "[project]\nname = \"fixture\"\n\n" +
		"[agents]\nenabled = [\"claude\", \"opencode\"]\n\n" +
		"[mcp.trello]\ncommand = \"npx\"\nargs = [\"-y\", \"@trello/mcp\"]\n" +
		"env = { TRELLO_TOKEN = \"$TRELLO_TOKEN\", literal_secret = \"super-secret\" }\n" +
		"headers = { Authorization = \"literal-auth\" }\n\n"
	if installed {
		manifest += "[recipes.tracker]\nenabled = true\nversion = \"1.0\"\n"
		if configBlock != "" {
			manifest += "\n[recipes.tracker.config]\n" + configBlock + "\n"
		}
	}
	write(t, filepath.Join(root, "ai-specs", "ai-specs.toml"), manifest)

	recipeDir := filepath.Join(home, "catalog", "recipes", "tracker")
	write(t, filepath.Join(recipeDir, "recipe.toml"), recipeBody)
	write(t, filepath.Join(recipeDir, "init.md"), initTrackerPrompt)
	write(t, filepath.Join(recipeDir, "templates", "mapping.toml"), "[mapping]\n")
	return root, home
}

func initOpts(home string) InitOptions {
	return InitOptions{Home: home}
}

func mustBrief(t *testing.T, root, id string, opts InitOptions) string {
	t.Helper()
	brief, err := BuildInitBrief(root, id, opts)
	if err != nil {
		t.Fatalf("BuildInitBrief: %v", err)
	}
	return brief
}

func TestInitBriefBannerAndSectionOrder(t *testing.T) {
	root, home := initProject(t, true, `board_id = "abc123"`+"\n", initTrackerRecipeBody)
	brief := mustBrief(t, root, "tracker", initOpts(home))

	if !strings.HasPrefix(brief, "# Recipe Init Brief") {
		t.Fatalf("brief does not start with the banner: %q", brief[:min(40, len(brief))])
	}
	wantOrder := []string{
		"## Recipe",
		"- ID: tracker",
		"- Name: Tracker",
		"- Version: 1.0",
		"- Description: Tracker setup",
		"- Install state: installed",
		"## Init Workflow",
		"- Prompt: init.md",
		"- Description: Configure tracker",
		"- Needs manifest: true",
		"- Needs MCP: trello, missing-mcp",
		"## Prompt Content",
		strings.TrimRight(initTrackerPrompt, "\n"),
		"## Manifest Context",
		"- Project: fixture",
		"- Enabled agents: claude, opencode",
		"- Active bindings: 0",
		"## Config Guidance",
		"## MCP Discovery",
		"## Template/Override Preview",
		"## Reviewable Next Actions",
	}
	at := 0
	for _, want := range wantOrder {
		idx := strings.Index(brief[at:], want)
		if idx < 0 {
			t.Fatalf("missing or out of order %q after offset %d in:\n%s", want, at, brief)
		}
		at += idx + len(want)
	}
}

// TestInitBriefCountsBindings pins read_bindings' filter: only a top-level
// [[bindings]] entry with BOTH keys as strings counts (S1's `Active bindings`).
func TestInitBriefCountsBindings(t *testing.T) {
	root, home := initProject(t, true, `board_id = "abc123"`+"\n", initTrackerRecipeBody)
	manifest := filepath.Join(root, "ai-specs", "ai-specs.toml")
	existing, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	write(t, manifest, string(existing)+
		"\n[[bindings]]\ncapability = \"tracker\"\nrecipe = \"tracker\"\n"+
		"\n[[bindings]]\ncapability = \"tracker\"\n")
	if brief := mustBrief(t, root, "tracker", initOpts(home)); !strings.Contains(brief, "- Active bindings: 1") {
		t.Errorf("binding filter not applied:\n%s", brief)
	}
}

func TestInitBriefInstallState(t *testing.T) {
	t.Run("available", func(t *testing.T) {
		root, home := initProject(t, false, "", initTrackerRecipeBody)
		brief := mustBrief(t, root, "tracker", initOpts(home))
		if !strings.Contains(brief, "- Install state: available (not installed)") {
			t.Errorf("missing available state:\n%s", brief)
		}
	})
	t.Run("disabled", func(t *testing.T) {
		root, home := t.TempDir(), t.TempDir()
		write(t, filepath.Join(root, "ai-specs", "ai-specs.toml"), "[recipes.tracker]\nenabled = false\n")
		write(t, filepath.Join(home, "catalog", "recipes", "tracker", "recipe.toml"), initTrackerRecipeBody)
		write(t, filepath.Join(home, "catalog", "recipes", "tracker", "init.md"), initTrackerPrompt)
		brief := mustBrief(t, root, "tracker", initOpts(home))
		if !strings.Contains(brief, "- Install state: disabled") {
			t.Errorf("missing disabled state:\n%s", brief)
		}
	})
}

func TestInitBriefConfigGuidance(t *testing.T) {
	t.Run("existing key is never printed as an assignment", func(t *testing.T) {
		root, home := initProject(t, true, `board_id = "abc123"`+"\n", initTrackerRecipeBody)
		brief := mustBrief(t, root, "tracker", initOpts(home))
		for _, want := range []string{
			"Existing config keys: board_id",
			"Update existing key `board_id`",
			"`timeout` is optional and defaults to `30`",
		} {
			if !strings.Contains(brief, want) {
				t.Errorf("missing %q in:\n%s", want, brief)
			}
		}
		if strings.Contains(brief, "board_id =") {
			t.Errorf("brief printed a real config assignment:\n%s", brief)
		}
		if strings.Contains(brief, "[recipes.tracker]\nenabled = true") {
			t.Errorf("installed recipe emitted the manifest addition:\n%s", brief)
		}
	})
	t.Run("absent recipe emits the reviewable addition", func(t *testing.T) {
		root, home := initProject(t, false, "", initTrackerRecipeBody)
		brief := mustBrief(t, root, "tracker", initOpts(home))
		for _, want := range []string{
			"Existing config keys: (none)",
			"Reviewable manifest addition:",
			"```toml\n[recipes.tracker]\nenabled = true\n```",
			"Add required `board_id` under `[recipes.tracker.config]`",
		} {
			if !strings.Contains(brief, want) {
				t.Errorf("missing %q in:\n%s", want, brief)
			}
		}
	})
	t.Run("unknown keys", func(t *testing.T) {
		root, home := initProject(t, true, "board_id = \"abc123\"\nunknown = \"value\"\n", initTrackerRecipeBody)
		brief := mustBrief(t, root, "tracker", initOpts(home))
		if !strings.Contains(brief, "Unknown config keys: unknown; sync still validates recipe config later.") {
			t.Errorf("missing unknown-key line:\n%s", brief)
		}
	})
	t.Run("no schema", func(t *testing.T) {
		body := strings.Replace(initTrackerRecipeBody,
			"[config.board_id]\nrequired = true\ntype = \"string\"\n\n[config.timeout]\nrequired = false\ntype = \"integer\"\ndefault = 30\n\n", "", 1)
		root, home := initProject(t, false, "", body)
		brief := mustBrief(t, root, "tracker", initOpts(home))
		if !strings.Contains(brief, "Recipe declares no config schema.") {
			t.Errorf("missing no-schema line:\n%s", brief)
		}
	})
}

func TestInitBriefMCPDiscoveryAndRedaction(t *testing.T) {
	root, home := initProject(t, true, `board_id = "abc123"`+"\n", initTrackerRecipeBody)
	brief := mustBrief(t, root, "tracker", initOpts(home))
	for _, want := range []string{
		"project manifest values take precedence",
		"- trello: configured; recipe preset available",
		"- missing-mcp: missing",
		"${TRELLO_TOKEN}",
		"literal_secret: ***",
		"Authorization: ***",
		"API_TOKEN: ***",
	} {
		if !strings.Contains(brief, want) {
			t.Errorf("missing %q in:\n%s", want, brief)
		}
	}
	for _, leak := range []string{"super-secret", "literal-auth", "recipe-secret"} {
		if strings.Contains(brief, leak) {
			t.Errorf("secret %q leaked:\n%s", leak, brief)
		}
	}
}

func TestInitBriefNoMCPDeclared(t *testing.T) {
	body := strings.Replace(initTrackerRecipeBody,
		"needs_mcp = [\"trello\", \"missing-mcp\"]", "needs_mcp = []", 1)
	body = body[:strings.Index(body, "[[provides.mcp]]")]
	root, home := initProject(t, false, "", body)
	brief := mustBrief(t, root, "tracker", initOpts(home))
	if !strings.Contains(brief, "No MCP servers declared by this init workflow or recipe presets.") {
		t.Errorf("missing empty-MCP line:\n%s", brief)
	}
}

func TestInitBriefTemplatePreviewIsReadOnly(t *testing.T) {
	root, home := initProject(t, false, "", initTrackerRecipeBody)
	target := filepath.Join(root, "ai-specs", "trello-mapping.toml")
	write(t, target, "existing")

	brief := mustBrief(t, root, "tracker", initOpts(home))
	for _, want := range []string{
		"## Template/Override Preview",
		"- ai-specs/trello-mapping.toml: exists; condition `not_exists`; action: review update/skip/diff.",
	} {
		if !strings.Contains(brief, want) {
			t.Errorf("missing %q in:\n%s", want, brief)
		}
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "existing" {
		t.Errorf("existing target was overwritten: %q", got)
	}

	os.Remove(target)
	brief = mustBrief(t, root, "tracker", initOpts(home))
	if !strings.Contains(brief, "- ai-specs/trello-mapping.toml: missing; condition `not_exists`; action: review create.") {
		t.Errorf("missing create line:\n%s", brief)
	}
	if !strings.Contains(brief, "No files were changed. Init does not run sync or materialize primitives.") {
		t.Errorf("missing read-only footer:\n%s", brief)
	}
	for _, absent := range []string{
		filepath.Join(root, "ai-specs", ".recipe"),
		filepath.Join(root, "ai-specs", ".tmp", "recipe-mcp.json"),
	} {
		if _, err := os.Stat(absent); !os.IsNotExist(err) {
			t.Errorf("init materialized %s", absent)
		}
	}
}

func TestInitErrors(t *testing.T) {
	t.Run("uninitialized project", func(t *testing.T) {
		root, home := t.TempDir(), t.TempDir()
		_, err := BuildInitBrief(root, "tracker", initOpts(home))
		if err == nil || err.Error() != "Project not initialized. Run: ai-specs init" {
			t.Fatalf("err = %v", err)
		}
		entries, _ := os.ReadDir(root)
		if len(entries) != 0 {
			t.Errorf("uninitialized project was mutated: %v", entries)
		}
	})
	t.Run("unknown id", func(t *testing.T) {
		root, home := initProject(t, false, "", initTrackerRecipeBody)
		_, err := BuildInitBrief(root, "missing", initOpts(home))
		if err == nil || err.Error() != "Recipe 'missing' not found in catalog/recipes/" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("no init workflow", func(t *testing.T) {
		body := initTrackerRecipeBody[:strings.Index(initTrackerRecipeBody, "[init]")] +
			initTrackerRecipeBody[strings.Index(initTrackerRecipeBody, "[config.board_id]"):]
		root, home := initProject(t, false, "", body)
		_, err := BuildInitBrief(root, "tracker", initOpts(home))
		if err == nil || err.Error() != "Recipe 'tracker' has no init workflow" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("internal test recipe", func(t *testing.T) {
		root, home := initProject(t, false, "", initTrackerRecipeBody)
		_, err := BuildInitBrief(root, "test-fixture", initOpts(home))
		if err == nil || !strings.Contains(err.Error(), "internal test fixture") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("missing id exits 2 with usage", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := RunInit(nil, initOpts(t.TempDir()), &stdout, &stderr)
		if code != 2 {
			t.Fatalf("exit = %d, want 2", code)
		}
		if !strings.Contains(stderr.String(), "missing recipe id") {
			t.Errorf("stderr = %q", stderr.String())
		}
		if !strings.Contains(stderr.String(), "Usage: ai-specs recipe init <id> [path] [--help]") {
			t.Errorf("stderr missing usage: %q", stderr.String())
		}
	})
}

func TestInitCatalogResolution(t *testing.T) {
	t.Run("home catalog wins over a project-local catalog", func(t *testing.T) {
		root, home := initProject(t, true, `board_id = "abc123"`+"\n", initTrackerRecipeBody)
		local := filepath.Join(root, "catalog", "recipes", "tracker", "recipe.toml")
		write(t, local, strings.ReplaceAll(
			strings.ReplaceAll(initTrackerRecipeBody, `name = "Tracker"`, `name = "Local Tracker"`),
			`version = "1.0"`, `version = "9.9"`))
		brief := mustBrief(t, root, "tracker", initOpts(home))
		if !strings.Contains(brief, "- Name: Tracker") || !strings.Contains(brief, "- Version: 1.0") {
			t.Errorf("home catalog not used:\n%s", brief)
		}
		if strings.Contains(brief, "Local Tracker") || strings.Contains(brief, "9.9") {
			t.Errorf("project-local catalog leaked:\n%s", brief)
		}
	})
	t.Run("isolated home with one recipe", func(t *testing.T) {
		root := t.TempDir()
		write(t, filepath.Join(root, "ai-specs", "ai-specs.toml"), "[project]\nname = \"fixture\"\n")
		home := t.TempDir()
		write(t, filepath.Join(home, "catalog", "recipes", "tracker", "recipe.toml"), initTrackerRecipeBody)
		write(t, filepath.Join(home, "catalog", "recipes", "tracker", "init.md"), initTrackerPrompt)
		brief := mustBrief(t, root, "tracker", initOpts(home))
		if !strings.Contains(brief, "- ID: tracker") {
			t.Errorf("isolated home recipe not resolved:\n%s", brief)
		}
	})
	t.Run("falls back to the CLI catalog", func(t *testing.T) {
		root := t.TempDir()
		write(t, filepath.Join(root, "ai-specs", "ai-specs.toml"),
			"[project]\nname = \"fixture\"\n\n[agents]\nenabled = [\"claude\"]\n\n"+
				"[mcp.trello]\ncommand = \"npx\"\nargs = [\"-y\", \"@trello/mcp\"]\n")
		installRoot, err := filepath.Abs("../..")
		if err != nil {
			t.Fatal(err)
		}
		brief := mustBrief(t, root, "trello-mcp-workflow", InitOptions{InstallRoot: installRoot})
		for _, want := range []string{
			"- ID: trello-mcp-workflow",
			"- Install state: available (not installed)",
			"- trello: configured",
			"# Recipe Init Contract",
		} {
			if !strings.Contains(brief, want) {
				t.Errorf("missing %q in:\n%s", want, brief)
			}
		}
	})
}

func TestInitWrapperGrammar(t *testing.T) {
	root, home := initProject(t, true, `board_id = "abc123"`+"\n", initTrackerRecipeBody)
	opts := initOpts(home)

	t.Run("help flag wins", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := RunInit([]string{"--help"}, opts, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if !strings.HasPrefix(stdout.String(), "Usage: ai-specs recipe init <id> [path] [--help]") {
			t.Errorf("stdout = %q", stdout.String())
		}
	})
	t.Run("-- makes the rest positional", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := RunInit([]string{"--", "tracker", root}, opts, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
		}
		if !strings.HasPrefix(stdout.String(), "# Recipe Init Brief") {
			t.Errorf("stdout = %q", stdout.String())
		}
	})
	t.Run("unknown flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := RunInit([]string{"--bogus"}, opts, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("exit = %d, want 2", code)
		}
		want := "ERROR: unknown flag: --bogus\nRun 'ai-specs recipe init --help' for usage.\n"
		if stderr.String() != want {
			t.Errorf("stderr = %q, want %q", stderr.String(), want)
		}
	})
	t.Run("a lone dash is an unknown flag", func(t *testing.T) {
		// lib/recipe-init.sh matches `-*` and `-` matches it, so the wrapper
		// never reads a bare dash as the recipe id.
		var stdout, stderr bytes.Buffer
		code := RunInit([]string{"-"}, opts, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("exit = %d, want 2", code)
		}
		want := "ERROR: unknown flag: -\nRun 'ai-specs recipe init --help' for usage.\n"
		if stderr.String() != want {
			t.Errorf("stderr = %q, want %q", stderr.String(), want)
		}
	})
	t.Run("third positional", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := RunInit([]string{"tracker", root, "extra"}, opts, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("exit = %d, want 2", code)
		}
		if stderr.String() != "ERROR: unexpected positional argument: extra\n" {
			t.Errorf("stderr = %q", stderr.String())
		}
	})
}

func TestInitBriefPathDefaultsToCwd(t *testing.T) {
	root, home := initProject(t, true, `board_id = "abc123"`+"\n", initTrackerRecipeBody)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	var stdout, stderr bytes.Buffer
	code := RunInit([]string{"tracker"}, initOpts(home), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "- Project: fixture") {
		t.Errorf("cwd was not used as the project root:\n%s", stdout.String())
	}
}
