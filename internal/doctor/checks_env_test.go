package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

// setDirenvOnPath puts an executable `direnv` on PATH for one test.
func setDirenvOnPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "direnv")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write direnv stub: %v", err)
	}
	t.Setenv("PATH", dir)
}

// setNoDirenvOnPath points PATH at an empty directory (shutil.which -> None).
func setNoDirenvOnPath(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

func canonicalEnvrc() string {
	return "# .envrc — direnv entry for this project\n\n" +
		managedStart + "\n" + managedBody + "\n" + managedEnd + "\n"
}

func TestUnquoteEnvValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"\"quoted\"", "quoted"},
		{"'single'", "single"},
		{"\"with \\\" esc\"", "with \" esc"},
		{"abc # comment", "abc"},
		{"abc#comment", "abc"},
		{"\"abc#def\"", "abc#def"},
		{"  spaced  ", "spaced"},
		{"\"back\\\\slash\"", "back\\slash"},
	}
	for _, tc := range cases {
		if got := unquoteEnvValue(tc.in); got != tc.want {
			t.Errorf("unquoteEnvValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseDotenv(t *testing.T) {
	text := "# comment\n" +
		"\n" +
		"  KEY1=value1  \n" +
		"KEY2=\"quoted value\"\n" +
		"KEY3=abc # trailing\n" +
		"# KEY4=skip\n" +
		"BAD LINE\n" +
		"KEY5='single'\n" +
		"KEY1=last\n"
	got := parseDotenv(text)
	want := map[string]string{
		"KEY1": "last",
		"KEY2": "quoted value",
		"KEY3": "abc",
		"KEY5": "single",
	}
	if len(got) != len(want) {
		t.Fatalf("parseDotenv = %+v, want %+v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("parseDotenv[%q] = %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got["KEY4"]; ok {
		t.Errorf("parseDotenv kept a commented key: %+v", got)
	}
}

func TestParseExports(t *testing.T) {
	text := "export FOO=bar\n  export BAZ=\"qux\"\nnotexport=1\nexport EMPTY=\n"
	got := parseExports(text)
	want := map[string]string{"FOO": "bar", "BAZ": "qux", "EMPTY": ""}
	if len(got) != len(want) {
		t.Fatalf("parseExports = %+v, want %+v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("parseExports[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestLoadHarnessEnv(t *testing.T) {
	root := t.TempDir()
	if got := loadHarnessEnv(root); len(got) != 0 {
		t.Fatalf("missing ai-specs.env: got %+v, want empty", got)
	}
	writeFile(t, filepath.Join(root, harnessEnvName), "TOKEN=abc\n")
	got := loadHarnessEnv(root)
	if got["TOKEN"] != "abc" || len(got) != 1 {
		t.Fatalf("loadHarnessEnv = %+v, want TOKEN=abc", got)
	}
}

func TestManagedBlock(t *testing.T) {
	canonical := canonicalEnvrc()
	if !hasManagedBlock(canonical) || !managedBlockIsCurrent(canonical) {
		t.Errorf("canonical .envrc: has=%v current=%v, want true/true",
			hasManagedBlock(canonical), managedBlockIsCurrent(canonical))
	}

	stale := managedStart + "\nold body\n" + managedEnd + "\n"
	if !hasManagedBlock(stale) || managedBlockIsCurrent(stale) {
		t.Errorf("stale .envrc: has=%v current=%v, want true/false",
			hasManagedBlock(stale), managedBlockIsCurrent(stale))
	}

	none := "just some text\n"
	if hasManagedBlock(none) || managedBlockIsCurrent(none) {
		t.Errorf("blockless .envrc: has=%v current=%v, want false/false",
			hasManagedBlock(none), managedBlockIsCurrent(none))
	}
}

// TestCollectEnvVarsPurposeAndAllowed pins the first-declaration purpose text
// and the env_allowed intersection rule.
func TestCollectEnvVarsPurposeAndAllowed(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeManifest(t, root, "[recipes.demo-a]\nenabled = true\n[recipes.demo-b]\nenabled = true\n")
	writeCatalogRecipe(t, home, "demo-a", catalogRecipe("demo-a",
		"\n[[provides.mcp]]\nid = \"preset-a\"\n"+
			"env = { SHARED = \"$SHARED\", ONLY_A = \"$ONLY_A\" }\n"+
			"env_allowed = { ONLY_A = [\"x\", \"y\"] }\n"))
	writeCatalogRecipe(t, home, "demo-b", catalogRecipe("demo-b",
		"\n[[provides.mcp]]\nid = \"preset-b\"\nenv = { SHARED = \"$SHARED\" }\n"))

	vars := collectEnvVars(root, home)
	if vars["SHARED"] != "required by preset-a (demo-a); also preset-b (demo-b)" {
		t.Errorf("SHARED purpose = %q", vars["SHARED"])
	}
	if vars["ONLY_A"] != "required by preset-a (demo-a)" {
		t.Errorf("ONLY_A purpose = %q", vars["ONLY_A"])
	}
	allowed := collectEnvAllowed(root, home)
	if len(allowed["ONLY_A"]) != 2 || allowed["ONLY_A"][0] != "x" || allowed["ONLY_A"][1] != "y" {
		t.Errorf("ONLY_A allowed = %+v, want [x y]", allowed["ONLY_A"])
	}
	if _, ok := allowed["SHARED"]; ok {
		t.Errorf("SHARED must have no allowed values, got %+v", allowed["SHARED"])
	}
}

func TestCollectEnvVarsNonReferencesSkipped(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeManifest(t, root, "[recipes.demo]\nenabled = true\n")
	writeCatalogRecipe(t, home, "demo", catalogRecipe("demo",
		"\n[[provides.mcp]]\nid = \"preset\"\n"+
			"env = { LITERAL = \"not-a-ref\", EMPTY = \"\", GOOD = \"$GOOD\" }\n"))
	vars := collectEnvVars(root, home)
	if len(vars) != 1 || vars["GOOD"] == "" {
		t.Fatalf("collectEnvVars = %+v, want only GOOD", vars)
	}
}

// TestCheckHarnessEnvLayoutEmptyDeclarations pins the legacy early return: no
// enabled recipe MCP presets means none of the three checks run.
func TestCheckHarnessEnvLayoutEmptyDeclarations(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "[project]\nname = \"x\"\n")
	d := New(root, t.TempDir())
	d.checkHarnessEnvLayout()
	assertChecks(t, d, []Check{})
}

func TestCheckHarnessEnvLayoutMissingKeysAndEnvrc(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeManifest(t, root, "[recipes.demo]\nenabled = true\n")
	writeCatalogRecipe(t, home, "demo", catalogRecipe("demo",
		"\n[[provides.mcp]]\nid = \"demo-preset\"\nenv = { DEMO_KEY = \"$DEMO_KEY\", DEMO_TOKEN = \"$DEMO_TOKEN\" }\n"))
	setNoDirenvOnPath(t)

	d := New(root, home)
	d.checkHarnessEnvLayout()
	assertChecks(t, d, []Check{
		{WARN, "direnv", "direnv not on PATH (needed to load harness MCP env for shells)",
			"brew install direnv && direnv allow  # or see https://direnv.net"},
		{WARN, "envrc-managed", "project-root .envrc missing ai-specs managed block",
			"run ai-specs configure-recipes to ensure root .envrc"},
		{WARN, "harness-env", "missing/empty in ai-specs.env: DEMO_KEY, DEMO_TOKEN",
			"run ai-specs configure-recipes to set harness env values"},
	})
}

func TestCheckHarnessEnvLayoutOK(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeManifest(t, root, "[recipes.demo]\nenabled = true\n")
	writeCatalogRecipe(t, home, "demo", catalogRecipe("demo",
		"\n[[provides.mcp]]\nid = \"demo-preset\"\nenv = { DEMO_KEY = \"$DEMO_KEY\", DEMO_TOKEN = \"$DEMO_TOKEN\" }\n"))
	writeFile(t, filepath.Join(root, ".envrc"), canonicalEnvrc())
	writeFile(t, filepath.Join(root, harnessEnvName), "DEMO_KEY=aaa\nDEMO_TOKEN=bbb\n")
	setDirenvOnPath(t)

	d := New(root, home)
	d.checkHarnessEnvLayout()
	assertChecks(t, d, []Check{
		{OK, "direnv", "direnv available on PATH", ""},
		{OK, "envrc-managed", "project-root .envrc has ai-specs managed block", ""},
		{OK, "harness-env", "ai-specs.env has 2 required MCP env key(s)", ""},
	})
}

func TestCheckHarnessEnvLayoutStaleEnvrc(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeManifest(t, root, "[recipes.demo]\nenabled = true\n")
	writeCatalogRecipe(t, home, "demo", catalogRecipe("demo",
		"\n[[provides.mcp]]\nid = \"demo-preset\"\nenv = { DEMO_KEY = \"$DEMO_KEY\" }\n"))
	writeFile(t, filepath.Join(root, ".envrc"), managedStart+"\nold body\n"+managedEnd+"\n")
	writeFile(t, filepath.Join(root, harnessEnvName), "DEMO_KEY=aaa\n")
	setDirenvOnPath(t)

	d := New(root, home)
	d.checkHarnessEnvLayout()
	assertChecks(t, d, []Check{
		{OK, "direnv", "direnv available on PATH", ""},
		{WARN, "envrc-managed", "project-root .envrc has stale ai-specs managed block",
			"run ai-specs configure-recipes to ensure root .envrc"},
		{OK, "harness-env", "ai-specs.env has 1 required MCP env key(s)", ""},
	})
}

func TestCheckHarnessEnvLayoutInvalidValue(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeManifest(t, root, "[recipes.demo]\nenabled = true\n")
	writeCatalogRecipe(t, home, "demo", catalogRecipe("demo",
		"\n[[provides.mcp]]\nid = \"demo-preset\"\n"+
			"env = { DEMO_MODE = \"$DEMO_MODE\" }\nenv_allowed = { DEMO_MODE = [\"on\", \"off\"] }\n"))
	writeFile(t, filepath.Join(root, ".envrc"), canonicalEnvrc())
	writeFile(t, filepath.Join(root, harnessEnvName), "DEMO_MODE=maybe\n")
	setDirenvOnPath(t)

	d := New(root, home)
	d.checkHarnessEnvLayout()
	assertChecks(t, d, []Check{
		{OK, "direnv", "direnv available on PATH", ""},
		{OK, "envrc-managed", "project-root .envrc has ai-specs managed block", ""},
		{OK, "harness-env", "ai-specs.env has 1 required MCP env key(s)", ""},
		{WARN, "harness-env-value",
			"invalid value for DEMO_MODE in ai-specs.env (allowed: on, off)",
			"run ai-specs configure-recipes to pick a valid value"},
	})
}
