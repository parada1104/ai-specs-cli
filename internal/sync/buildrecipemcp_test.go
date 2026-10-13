package sync

// Contract tests for the native recipe-MCP build authority (GO-07 S13 PR4b-2).
//
// BuildRecipeMCP mirrors the Python authority build_recipe_mcp
// (lib/_internal/recipe-materialize.py:3179) and resolveProviderMarkers mirrors
// _resolve_provider_markers (:3131). The authority is added, not wired; S14
// owns the sync-pipeline wiring. The recipe-mcp JSON serialization
// (json.dump(indent=2) plus a trailing newline, :4108-4110) belongs to the
// caller and reuses mcpJSONDumpsIndent2, so the differential below compares the
// serialized document bytes.
//
// The differential shells out to the real Python module through
// testdata/buildrecipemcp_ref.py (the hooksrunner_ref.py / stamps_test.go
// shape) scenario by scenario: manifest precedence, recipe append order, the
// conflict warning, the four provider-marker warnings, path/managed provider
// resolution, resolution omission, a non-jinna cli_dep, and a command that is
// not the marker literal.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/provider"
)

// --- differential harness ---------------------------------------------------

// buildRecipeMCPRefPath resolves the reference driver under this package (go
// test cwd is internal/sync).
func buildRecipeMCPRefPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "buildrecipemcp_ref.py"))
	if err != nil {
		t.Fatalf("resolve ref driver: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("ref driver %s: %v", path, err)
	}
	return path
}

// buildRecipeMCPRefOut is the canonical JSON testdata/buildrecipemcp_ref.py
// emits.
type buildRecipeMCPRefOut struct {
	Error    string `json:"error"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Document string `json:"document"`
}

// runBuildRecipeMCPRef runs the reference driver once and decodes its envelope.
func runBuildRecipeMCPRef(t *testing.T, python, driver string, args ...string) buildRecipeMCPRefOut {
	t.Helper()
	cmd := exec.Command(python, append([]string{driver}, args...)...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("python oracle failed: %v\nstderr: %s", err, stderr.String())
	}
	var out buildRecipeMCPRefOut
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode ref %s: %v", stdout.String(), err)
	}
	return out
}

// mustOrderedMCP decodes a JSON object into the ordered mcp model both legs
// consume.
func mustOrderedMCP(t *testing.T, raw string) *mcpObj {
	t.Helper()
	v, err := mcpParseJSONOrdered([]byte(raw))
	if err != nil {
		t.Fatalf("decode ordered mcp %q: %v", raw, err)
	}
	obj, ok := v.(*mcpObj)
	if !ok {
		t.Fatalf("decode ordered mcp %q: got %T", raw, v)
	}
	return obj
}

// nativeMCPDocument serializes the merged mapping exactly as the caller does.
func nativeMCPDocument(merged *mcpObj) string {
	return mcpJSONDumpsIndent2(merged) + "\n"
}

func mustJSONArg(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// compareBuildRecipeMCP requires the native leg to match the Python reference
// byte for byte: error string, warning stream, and serialized document.
func compareBuildRecipeMCP(t *testing.T, ref buildRecipeMCPRefOut, goErr, goStderr, goDoc string) {
	t.Helper()
	if goErr != ref.Error {
		t.Fatalf("error: go=%q python=%q", goErr, ref.Error)
	}
	if goStderr != ref.Stderr {
		t.Fatalf("stderr:\n--- go ---\n%q\n--- python ---\n%q", goStderr, ref.Stderr)
	}
	if goDoc != ref.Document {
		t.Fatalf("document:\n--- go ---\n%q\n--- python ---\n%q", goDoc, ref.Document)
	}
}

// --- fixture catalog --------------------------------------------------------

// buildRecipeMCPCatalog is the shared fixture catalog read by both legs.
func buildRecipeMCPCatalog(t *testing.T) string {
	t.Helper()
	catalog := t.TempDir()
	header := syncRecipeHeader

	// Manifest precedence, append order, the conflict warning, and a preset
	// whose command is not a provider marker.
	writeSyncCatalogRecipe(t, catalog, "mcpmerge", header("mcpmerge", "MCP Merge")+
		"[[deps.cli]]\nbinary = \"jinna\"\npurpose = \"p\"\nmin_version = \"0.1.0\"\n"+
		"[[provides.mcp]]\nid = \"alpha\"\ncommand = \"npx\"\nargs = [\"-y\", \"pkg\"]\ntimeout = 5000\n"+
		"[[provides.mcp]]\nid = \"beta\"\ncommand = \"other\"\n"+
		"[[provides.mcp]]\nid = \"gamma\"\ncommand = \"{dep:jinna}\"\n")

	// A second recipe pins cross-recipe append order.
	writeSyncCatalogRecipe(t, catalog, "mcpsecond", header("mcpsecond", "MCP Second")+
		"[[provides.mcp]]\nid = \"epsilon\"\ncommand = \"e\"\n")

	// A resolvable jinna marker.
	writeSyncCatalogRecipe(t, catalog, "mcpjinna", header("mcpjinna", "Jinna")+
		"[[deps.cli]]\nbinary = \"jinna\"\npurpose = \"p\"\nmin_version = \"0.1.0\"\n"+
		"[[provides.mcp]]\nid = \"jinna\"\ncommand = \"{dep:jinna}\"\nargs = [\"mcp\"]\ntimeout = 30000\n")

	// A cli_deps entry whose binary is not jinna: the jinna declaration is
	// absent even though the marker is present.
	writeSyncCatalogRecipe(t, catalog, "mcpnodep", header("mcpnodep", "No Dep")+
		"[[deps.cli]]\nbinary = \"other\"\npurpose = \"p\"\n"+
		"[[provides.mcp]]\nid = \"jinna\"\ncommand = \"{dep:jinna}\"\n")
	return catalog
}

// --- fake providers ---------------------------------------------------------

func writeFakeJinna(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "jinna"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func sha256HexFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// writeManagedJinna materializes a verified managed provider under
// <home>/cache/bin/jinna/<versionDir>/<target>/jinna plus its install.json
// receipt, with scriptBody as the executable.
func writeManagedJinna(t *testing.T, home, versionDir, scriptBody string) {
	t.Helper()
	goos, goarch := provider.DetectPlatform(nil, nil)
	if goos == "" || goarch == "" {
		t.Skipf("unsupported provider platform %q/%q", goos, goarch)
	}
	root := filepath.Join(home, "cache", "bin", "jinna")
	bin := provider.ManagedBinaryPath(root, versionDir, goos, goarch)
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte(scriptBody), 0o755); err != nil {
		t.Fatal(err)
	}
	receipt := map[string]any{
		"status":        "verified",
		"repository":    provider.Repository,
		"target":        goos + "-" + goarch,
		"release_tag":   "v" + versionDir,
		"binary_sha256": sha256HexFile(t, bin),
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(bin), "install.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// counterJinnaBody succeeds on the first "version" invocation and fails on the
// second, so the winner re-check (the only unguarded version run) fails. It
// uses shell builtins only, because the scenario strips PATH to a dir with no
// jinna on it.
func counterJinnaBody(home string) string {
	flag := filepath.Join(home, "called-once")
	return "#!/bin/sh\n" +
		"if [ -f '" + flag + "' ]; then echo boom; exit 1; fi\n" +
		": > '" + flag + "'\n" +
		"echo jinna 0.1.0\n"
}

// emptyPath returns a PATH with no jinna on it, so only the managed cache can
// satisfy a resolution.
func emptyPath(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// --- build differential -----------------------------------------------------

// TestBuildRecipeMCPDifferentialAgainstPython pins BuildRecipeMCP against the
// real Python build_recipe_mcp: the serialized document, the warning stream,
// and the raised error string must agree scenario by scenario.
func TestBuildRecipeMCPDifferentialAgainstPython(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := buildRecipeMCPRefPath(t)
	catalog := buildRecipeMCPCatalog(t)

	cases := []struct {
		name         string
		recipeIDs    []string
		manifestJSON string
	}{
		{"manifest precedence and conflict warning", []string{"mcpmerge"},
			`{"alpha":{"command":"m","timeout":1},"delta":{"command":"d"}}`},
		{"manifest order first then recipe presets", []string{"mcpsecond"},
			`{"zeta":{"command":"z"},"alpha":{"command":"a"}}`},
		{"recipe append order across recipes", []string{"mcpmerge", "mcpsecond"}, `{}`},
		{"recipe-only append preserves declaration order", []string{"mcpsecond"}, `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref := runBuildRecipeMCPRef(t, python, driver, "build", authority, catalog,
				mustJSONArg(t, tc.recipeIDs), tc.manifestJSON, "-")
			manifest := mustOrderedMCP(t, tc.manifestJSON)
			var goErrBuf bytes.Buffer
			merged, err := BuildRecipeMCP(catalog, tc.recipeIDs, manifest, nil, &goErrBuf)
			goErr, goDoc := "", ""
			if err != nil {
				goErr = err.Error()
			} else {
				goDoc = nativeMCPDocument(merged)
			}
			compareBuildRecipeMCP(t, ref, goErr, goErrBuf.String(), goDoc)
		})
	}
}

// TestBuildRecipeMCPErrorPropagates pins that a read_recipe failure inside the
// public build loop propagates out of BuildRecipeMCP (unlike inside the
// resolver, which catches it).
func TestBuildRecipeMCPErrorPropagates(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := buildRecipeMCPRefPath(t)
	catalog := buildRecipeMCPCatalog(t)

	ref := runBuildRecipeMCPRef(t, python, driver, "build", authority, catalog,
		mustJSONArg(t, []string{"ghost"}), `{}`, "-")
	var goErrBuf bytes.Buffer
	_, err := BuildRecipeMCP(catalog, []string{"ghost"}, newMCPObj(), nil, &goErrBuf)
	if err == nil {
		t.Fatalf("expected an error for a missing recipe directory")
	}
	want := "recipe directory not found: " + filepath.Join(catalog, "ghost")
	if err.Error() != want {
		t.Fatalf("go error = %q, want %q", err.Error(), want)
	}
	if ref.Error != want {
		t.Fatalf("python error = %q, want %q", ref.Error, want)
	}
	if ref.Document != "" {
		t.Fatalf("python document = %q, want empty on error", ref.Document)
	}
}

// --- resolver differential --------------------------------------------------

// TestResolveProviderMarkersDifferentialAgainstPython pins resolveProviderMarkers
// against the real Python _resolve_provider_markers scenario by scenario.
func TestResolveProviderMarkersDifferentialAgainstPython(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := buildRecipeMCPRefPath(t)
	catalog := buildRecipeMCPCatalog(t)

	t.Run("cannot read recipe for provider resolution", func(t *testing.T) {
		home := t.TempDir()
		mergedJSON := `{}`
		ref := runBuildRecipeMCPRef(t, python, driver, "resolve", authority, catalog,
			mustJSONArg(t, []string{"ghost"}), mergedJSON, home)
		merged := mustOrderedMCP(t, mergedJSON)
		var buf bytes.Buffer
		resolveProviderMarkers(merged, catalog, []string{"ghost"}, home, &buf)
		compareBuildRecipeMCP(t, ref, "", buf.String(), nativeMCPDocument(merged))
	})

	t.Run("missing jinna dependency declaration", func(t *testing.T) {
		home := t.TempDir()
		mergedJSON := `{"jinna":{"command":"{dep:jinna}"}}`
		t.Setenv("PATH", emptyPath(t))
		ref := runBuildRecipeMCPRef(t, python, driver, "resolve", authority, catalog,
			mustJSONArg(t, []string{"mcpnodep"}), mergedJSON, home)
		merged := mustOrderedMCP(t, mergedJSON)
		var buf bytes.Buffer
		resolveProviderMarkers(merged, catalog, []string{"mcpnodep"}, home, &buf)
		compareBuildRecipeMCP(t, ref, "", buf.String(), nativeMCPDocument(merged))
	})

	t.Run("command not the marker literal is skipped", func(t *testing.T) {
		home := t.TempDir()
		mergedJSON := `{"jinna":{"command":"npx"}}`
		t.Setenv("PATH", emptyPath(t))
		ref := runBuildRecipeMCPRef(t, python, driver, "resolve", authority, catalog,
			mustJSONArg(t, []string{"mcpjinna"}), mergedJSON, home)
		merged := mustOrderedMCP(t, mergedJSON)
		var buf bytes.Buffer
		resolveProviderMarkers(merged, catalog, []string{"mcpjinna"}, home, &buf)
		compareBuildRecipeMCP(t, ref, "", buf.String(), nativeMCPDocument(merged))
	})

	t.Run("provider unresolved omits the server", func(t *testing.T) {
		home := t.TempDir()
		mergedJSON := `{"jinna":{"command":"{dep:jinna}"}}`
		t.Setenv("PATH", emptyPath(t))
		ref := runBuildRecipeMCPRef(t, python, driver, "resolve", authority, catalog,
			mustJSONArg(t, []string{"mcpjinna"}), mergedJSON, home)
		merged := mustOrderedMCP(t, mergedJSON)
		var buf bytes.Buffer
		resolveProviderMarkers(merged, catalog, []string{"mcpjinna"}, home, &buf)
		compareBuildRecipeMCP(t, ref, "", buf.String(), nativeMCPDocument(merged))
	})

	t.Run("path source resolves to the bare binary", func(t *testing.T) {
		fakeDir := t.TempDir()
		writeFakeJinna(t, fakeDir, "#!/bin/sh\necho jinna 1.2.3\n")
		t.Setenv("PATH", fakeDir)
		home := t.TempDir()
		mergedJSON := `{"jinna":{"command":"{dep:jinna}"}}`
		ref := runBuildRecipeMCPRef(t, python, driver, "resolve", authority, catalog,
			mustJSONArg(t, []string{"mcpjinna"}), mergedJSON, home)
		merged := mustOrderedMCP(t, mergedJSON)
		var buf bytes.Buffer
		resolveProviderMarkers(merged, catalog, []string{"mcpjinna"}, home, &buf)
		compareBuildRecipeMCP(t, ref, "", buf.String(), nativeMCPDocument(merged))
	})

	t.Run("managed source resolves to the absolute command", func(t *testing.T) {
		// A shared home is required: the absolute command path is observable.
		home := t.TempDir()
		writeManagedJinna(t, home, "0.1.0", "#!/bin/sh\necho jinna 0.1.0\n")
		t.Setenv("PATH", emptyPath(t))
		mergedJSON := `{"jinna":{"command":"{dep:jinna}"}}`
		ref := runBuildRecipeMCPRef(t, python, driver, "resolve", authority, catalog,
			mustJSONArg(t, []string{"mcpjinna"}), mergedJSON, home)
		merged := mustOrderedMCP(t, mergedJSON)
		var buf bytes.Buffer
		resolveProviderMarkers(merged, catalog, []string{"mcpjinna"}, home, &buf)
		compareBuildRecipeMCP(t, ref, "", buf.String(), nativeMCPDocument(merged))
	})

	t.Run("resolution failure degrades to a warning", func(t *testing.T) {
		t.Setenv("PATH", emptyPath(t))
		// Separate homes so each leg's counter starts at zero; the server is
		// dropped on both sides, so the absolute path never reaches the document.
		pyHome := t.TempDir()
		goHome := t.TempDir()
		writeManagedJinna(t, pyHome, "0.1.0", counterJinnaBody(pyHome))
		writeManagedJinna(t, goHome, "0.1.0", counterJinnaBody(goHome))
		mergedJSON := `{"jinna":{"command":"{dep:jinna}"}}`
		ref := runBuildRecipeMCPRef(t, python, driver, "resolve", authority, catalog,
			mustJSONArg(t, []string{"mcpjinna"}), mergedJSON, pyHome)
		merged := mustOrderedMCP(t, mergedJSON)
		var buf bytes.Buffer
		resolveProviderMarkers(merged, catalog, []string{"mcpjinna"}, goHome, &buf)
		compareBuildRecipeMCP(t, ref, "", buf.String(), nativeMCPDocument(merged))
	})
}

// --- recorded divergences (not parity) --------------------------------------

// buildRecipeMCPDivergenceBigInteger records the internal/toml value-subset
// divergence on an integer outside int64. Python's tomllib accepts the literal
// and build_recipe_mcp serializes it in full; internal/toml refuses the whole
// document. In the resolver leg Python therefore resolves the marker while the
// native authority warns and leaves the literal. This is an OUTCOME divergence
// on a recipe Python accepts, not a message-text difference: it is the
// pre-existing internal/toml subset limit, NOT something this delivery
// introduced and NOT a parity claim. The test below pins both observed sides so
// a future internal/toml change must update it deliberately.
const buildRecipeMCPDivergenceBigInteger = "big integer: tomllib accepts it and Python resolves the marker, while internal/toml refuses the document and the native resolver warns and leaves the literal (recorded divergence, NOT parity)"

// buildRecipeMCPDivergenceDatetime records the same subset divergence on a TOML
// datetime. Python's tomllib reads the value, the resolver leg resolves the
// marker, and the build leg only fails later at json.dump ("Object of type
// datetime is not JSON serializable"); internal/toml refuses the document at
// parse time. This is a pre-existing internal/toml subset limit, NOT something
// this delivery introduced and NOT a parity claim. The test below pins both
// observed sides so a future internal/toml change must update it deliberately.
const buildRecipeMCPDivergenceDatetime = "datetime: tomllib reads it and Python resolves the marker (build leg then fails at json.dump), while internal/toml refuses at parse time and the native resolver warns and leaves the literal (recorded divergence, NOT parity)"

// TestBuildRecipeMCPDivergenceBigInteger pins buildRecipeMCPDivergenceBigInteger.
// It asserts both observed sides directly instead of claiming parity: Python
// resolves the marker, while Go warns on the unreadable recipe and keeps the
// {dep:jinna} literal. The build leg additionally pins that Python serializes
// the full integer while Go refuses the document.
func TestBuildRecipeMCPDivergenceBigInteger(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := buildRecipeMCPRefPath(t)
	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "bigint", syncRecipeHeader("bigint", "Big Int")+
		"[[deps.cli]]\nbinary = \"jinna\"\npurpose = \"p\"\nmin_version = \"0.1.0\"\n"+
		"[[provides.mcp]]\nid = \"jinna\"\ncommand = \"{dep:jinna}\"\ntimeout = 123456789012345678901234567890\n")

	// Build leg: Python serializes the full integer; Go cannot read the recipe.
	buildRef := runBuildRecipeMCPRef(t, python, driver, "build", authority, catalog,
		mustJSONArg(t, []string{"bigint"}), `{}`, "-")
	if buildRef.Error != "" || buildRef.Stderr != "" {
		t.Fatalf("python build raised or warned: error=%q stderr=%q", buildRef.Error, buildRef.Stderr)
	}
	wantPyBuild := "{\n  \"jinna\": {\n    \"command\": \"{dep:jinna}\",\n    \"timeout\": 123456789012345678901234567890\n  }\n}\n"
	if buildRef.Document != wantPyBuild {
		t.Fatalf("python build document = %q, want the full integer %q", buildRef.Document, wantPyBuild)
	}
	if _, goBuildErr := BuildRecipeMCP(catalog, []string{"bigint"}, newMCPObj(), nil, io.Discard); goBuildErr == nil {
		t.Fatalf("Go build accepted a big integer; want the internal/toml refusal")
	}

	// Resolver leg: Python resolves the marker; Go warns and leaves the literal.
	fakeDir := t.TempDir()
	writeFakeJinna(t, fakeDir, "#!/bin/sh\necho jinna 1.2.3\n")
	t.Setenv("PATH", fakeDir)
	home := t.TempDir()
	mergedJSON := `{"jinna":{"command":"{dep:jinna}"}}`
	resolveRef := runBuildRecipeMCPRef(t, python, driver, "resolve", authority, catalog,
		mustJSONArg(t, []string{"bigint"}), mergedJSON, home)
	if resolveRef.Error != "" || resolveRef.Stderr != "" {
		t.Fatalf("python resolve raised or warned: error=%q stderr=%q", resolveRef.Error, resolveRef.Stderr)
	}
	wantPyResolve := "{\n  \"jinna\": {\n    \"command\": \"jinna\"\n  }\n}\n"
	if resolveRef.Document != wantPyResolve {
		t.Fatalf("python resolve document = %q, want the resolved marker %q", resolveRef.Document, wantPyResolve)
	}

	merged := mustOrderedMCP(t, mergedJSON)
	var buf bytes.Buffer
	resolveProviderMarkers(merged, catalog, []string{"bigint"}, home, &buf)
	wantWarn := "  ! recipe 'bigint': cannot read recipe for provider resolution (toml: line 13: invalid integer \"123456789012345678901234567890\")\n"
	if buf.String() != wantWarn {
		t.Fatalf("%s: go stderr = %q, want %q", buildRecipeMCPDivergenceBigInteger, buf.String(), wantWarn)
	}
	wantGoLiteral := "{\n  \"jinna\": {\n    \"command\": \"{dep:jinna}\"\n  }\n}\n"
	if got := nativeMCPDocument(merged); got != wantGoLiteral {
		t.Fatalf("%s: go resolve document = %q, want the unresolved literal %q", buildRecipeMCPDivergenceBigInteger, got, wantGoLiteral)
	}
}

// TestBuildRecipeMCPDivergenceDatetime pins buildRecipeMCPDivergenceDatetime.
// It asserts both observed sides directly instead of claiming parity: Python
// resolves the marker (and its build leg fails only at json.dump), while Go
// warns on the unreadable recipe and keeps the {dep:jinna} literal.
func TestBuildRecipeMCPDivergenceDatetime(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := buildRecipeMCPRefPath(t)
	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "datetime", syncRecipeHeader("datetime", "DateTime")+
		"[[deps.cli]]\nbinary = \"jinna\"\npurpose = \"p\"\nmin_version = \"0.1.0\"\n"+
		"[[provides.mcp]]\nid = \"jinna\"\ncommand = \"{dep:jinna}\"\nrelease = 1979-05-27T07:32:00Z\n")

	// Build leg: Python parses the datetime but cannot serialize it; Go cannot
	// read the recipe at all.
	buildRef := runBuildRecipeMCPRef(t, python, driver, "build", authority, catalog,
		mustJSONArg(t, []string{"datetime"}), `{}`, "-")
	if buildRef.Error != "Object of type datetime is not JSON serializable" || buildRef.Document != "" {
		t.Fatalf("python build = error %q document %q, want the json.dump TypeError", buildRef.Error, buildRef.Document)
	}
	if _, goBuildErr := BuildRecipeMCP(catalog, []string{"datetime"}, newMCPObj(), nil, io.Discard); goBuildErr == nil {
		t.Fatalf("Go build accepted a datetime; want the internal/toml refusal")
	}

	// Resolver leg: Python resolves the marker; Go warns and leaves the literal.
	fakeDir := t.TempDir()
	writeFakeJinna(t, fakeDir, "#!/bin/sh\necho jinna 1.2.3\n")
	t.Setenv("PATH", fakeDir)
	home := t.TempDir()
	mergedJSON := `{"jinna":{"command":"{dep:jinna}"}}`
	resolveRef := runBuildRecipeMCPRef(t, python, driver, "resolve", authority, catalog,
		mustJSONArg(t, []string{"datetime"}), mergedJSON, home)
	if resolveRef.Error != "" || resolveRef.Stderr != "" {
		t.Fatalf("python resolve raised or warned: error=%q stderr=%q", resolveRef.Error, resolveRef.Stderr)
	}
	wantPyResolve := "{\n  \"jinna\": {\n    \"command\": \"jinna\"\n  }\n}\n"
	if resolveRef.Document != wantPyResolve {
		t.Fatalf("python resolve document = %q, want the resolved marker %q", resolveRef.Document, wantPyResolve)
	}

	merged := mustOrderedMCP(t, mergedJSON)
	var buf bytes.Buffer
	resolveProviderMarkers(merged, catalog, []string{"datetime"}, home, &buf)
	wantWarn := "  ! recipe 'datetime': cannot read recipe for provider resolution (toml: line 13: datetime values (\"1979-05-27T07:32:00Z\") are not supported)\n"
	if buf.String() != wantWarn {
		t.Fatalf("%s: go stderr = %q, want %q", buildRecipeMCPDivergenceDatetime, buf.String(), wantWarn)
	}
	wantGoLiteral := "{\n  \"jinna\": {\n    \"command\": \"{dep:jinna}\"\n  }\n}\n"
	if got := nativeMCPDocument(merged); got != wantGoLiteral {
		t.Fatalf("%s: go resolve document = %q, want the unresolved literal %q", buildRecipeMCPDivergenceDatetime, got, wantGoLiteral)
	}
}

// --- literal pins -----------------------------------------------------------

// TestBuildRecipeMCPWarningStrings pins the warning bytes and streams literally,
// independent of the Python oracle.
func TestBuildRecipeMCPWarningStrings(t *testing.T) {
	catalog := buildRecipeMCPCatalog(t)

	t.Run("conflict warning", func(t *testing.T) {
		var buf bytes.Buffer
		manifest := mustOrderedMCP(t, `{"alpha":{"command":"m","timeout":1}}`)
		merged, err := BuildRecipeMCP(catalog, []string{"mcpmerge"}, manifest, nil, &buf)
		if err != nil {
			t.Fatal(err)
		}
		wantErr := "  ! recipe 'MCP Merge' mcp.id='alpha' key 'command' conflicts with project manifest (manifest wins)\n" +
			"  ! recipe 'MCP Merge' mcp.id='alpha' key 'timeout' conflicts with project manifest (manifest wins)\n"
		if buf.String() != wantErr {
			t.Fatalf("stderr = %q, want %q", buf.String(), wantErr)
		}
		// Manifest values win; the recipe-only key is appended.
		alpha, _ := merged.get("alpha")
		got := mcpJSONDumpsIndent2(alpha)
		want := "{\n  \"command\": \"m\",\n  \"timeout\": 1,\n  \"args\": [\n    \"-y\",\n    \"pkg\"\n  ]\n}"
		if got != want {
			t.Fatalf("alpha = %s, want %s", got, want)
		}
	})

	t.Run("resolver warnings", func(t *testing.T) {
		cases := []struct {
			name      string
			recipeIDs []string
			merged    string
			home      func(t *testing.T) string
			want      string
		}{
			{"cannot read", []string{"ghost"}, `{}`, func(t *testing.T) string { return t.TempDir() },
				"  ! recipe 'ghost': cannot read recipe for provider resolution (recipe directory not found: " + filepath.Join(catalog, "ghost") + ")\n"},
			{"missing declaration", []string{"mcpnodep"}, `{"jinna":{"command":"{dep:jinna}"}}`,
				func(t *testing.T) string { return t.TempDir() },
				"  ! recipe 'No Dep': missing jinna dependency declaration\n"},
			{"unresolved", []string{"mcpjinna"}, `{"jinna":{"command":"{dep:jinna}"}}`,
				func(t *testing.T) string { return t.TempDir() },
				"  ! recipe 'Jinna': provider jinna is unresolved; run ai-specs configure-recipes interactively to install it\n"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Setenv("PATH", emptyPath(t))
				merged := mustOrderedMCP(t, tc.merged)
				var buf bytes.Buffer
				resolveProviderMarkers(merged, catalog, tc.recipeIDs, tc.home(t), &buf)
				if buf.String() != tc.want {
					t.Fatalf("stderr = %q, want %q", buf.String(), tc.want)
				}
			})
		}
	})

	t.Run("resolution failure warning", func(t *testing.T) {
		t.Setenv("PATH", emptyPath(t))
		home := t.TempDir()
		writeManagedJinna(t, home, "0.1.0", counterJinnaBody(home))
		merged := mustOrderedMCP(t, `{"jinna":{"command":"{dep:jinna}"}}`)
		var buf bytes.Buffer
		resolveProviderMarkers(merged, catalog, []string{"mcpjinna"}, home, &buf)
		want := "  ! recipe 'Jinna': provider jinna could not be resolved (provider version check returned no usable version); run ai-specs configure-recipes interactively to install it\n"
		if buf.String() != want {
			t.Fatalf("stderr = %q, want %q", buf.String(), want)
		}
	})
}
