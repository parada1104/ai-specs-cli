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
// The differential covers the same scenarios the Python module was driven
// through by testdata/buildrecipemcp_ref.py (the hooksrunner_ref.py /
// stamps_test.go shape): manifest precedence, recipe append order, the
// conflict warning, the four provider-marker warnings, path/managed provider
// resolution, resolution omission, a non-jinna cli_dep, and a command that is
// not the marker literal.
//
// PR2-B S7: the differential tests consume testdata/buildrecipemcp_golden.json,
// the golden bundle captured once from the live Python 3.14.7 oracle through the
// same in-package fixture helpers below; no test execs Python anymore. Host
// paths in the captured strings are tokenized (<CATALOG>, <HOME>) and expanded
// with this run's fixtures.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/provider"
	"ai-specs.dev/ai-specs/internal/toml"
)

// --- frozen oracle ----------------------------------------------------------

// buildRecipeMCPGoldenCase is one frozen record of
// testdata/buildrecipemcp_golden.json: the canonical envelope
// testdata/buildrecipemcp_ref.py emitted for one scenario, plus the per-field
// host-path token inventory of that record.
type buildRecipeMCPGoldenCase struct {
	Kind     string         `json:"kind"` // build | resolve
	Error    string         `json:"error"`
	Stdout   string         `json:"stdout"`
	Stderr   string         `json:"stderr"`
	Document string         `json:"document"`
	Tokens   map[string]int `json:"tokens,omitempty"`
}

// buildRecipeMCPGoldenInventory is the hard-coded identity inventory of the
// frozen bundle: exact case names in capture order and their kind. Kept in the
// test, never derived from the bundle. A missing, renamed, duplicated or
// reordered record fails before any case loop runs.
var buildRecipeMCPGoldenInventory = []struct{ Kind, Name string }{
	{"build", "manifest precedence and conflict warning"},
	{"build", "manifest order first then recipe presets"},
	{"build", "recipe append order across recipes"},
	{"build", "recipe-only append preserves declaration order"},
	{"build", "missing recipe error propagates"},
	{"resolve", "cannot read recipe for provider resolution"},
	{"resolve", "missing jinna dependency declaration"},
	{"resolve", "command not the marker literal is skipped"},
	{"resolve", "provider unresolved omits the server"},
	{"resolve", "path source resolves to the bare binary"},
	{"resolve", "managed source resolves to the absolute command"},
	{"resolve", "resolution failure degrades to a warning"},
	{"build", "big integer build parity"},
	{"resolve", "big integer resolve parity"},
	{"build", "datetime build parity"},
	{"resolve", "datetime resolve parity"},
}

// buildRecipeMCPGoldenKindCounts is the per-kind count of the inventory above.
var buildRecipeMCPGoldenKindCounts = map[string]int{"build": 7, "resolve": 9}

const (
	// buildRecipeMCPCatalogToken is the tokenized fixture catalog root.
	buildRecipeMCPCatalogToken = "<CATALOG>"
	// buildRecipeMCPHomeToken is the tokenized ai-specs home of one case.
	buildRecipeMCPHomeToken = "<HOME>"
)

// buildRecipeMCPGoldenTokenInventory is the hard-coded host-path token
// inventory of the frozen bundle: which record carries a token, in which field,
// and how often. Only these three positions may carry a host path; anything
// else is a bundle mutation, not a host-path occurrence.
var buildRecipeMCPGoldenTokenInventory = map[string]map[string]int{
	"missing recipe error propagates":                 {"error": 1},
	"cannot read recipe for provider resolution":      {"stderr": 1},
	"managed source resolves to the absolute command": {"document": 1},
}

// buildRecipeMCPRefOut is the oracle envelope the frozen records carry.
type buildRecipeMCPRefOut struct {
	Error    string `json:"error"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Document string `json:"document"`
}

// checkBuildRecipeMCPGoldenInventory validates one loaded bundle against the
// hard-coded identity and token inventories. It is pure so the focused control
// can run it against a mutated in-memory copy.
func checkBuildRecipeMCPGoldenInventory(names []string, cases []buildRecipeMCPGoldenCase) error {
	if len(names) != len(buildRecipeMCPGoldenInventory) {
		return fmt.Errorf("frozen oracle has %d names, want %d", len(names), len(buildRecipeMCPGoldenInventory))
	}
	if len(cases) != len(names) {
		return fmt.Errorf("frozen oracle has %d records for %d names", len(cases), len(names))
	}
	seen := make(map[string]bool, len(names))
	kinds := map[string]int{}
	for i, tc := range cases {
		want := buildRecipeMCPGoldenInventory[i]
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
		wantTokens, hasTokens := buildRecipeMCPGoldenTokenInventory[names[i]]
		if !hasTokens && len(tc.Tokens) != 0 {
			return fmt.Errorf("frozen oracle record %q carries a non-inventoried token %v", names[i], tc.Tokens)
		}
		for field, n := range tc.Tokens {
			switch field {
			case "error", "stdout", "stderr", "document":
			default:
				return fmt.Errorf("frozen oracle record %q tokenizes unknown field %q", names[i], field)
			}
			if wantTokens[field] != n {
				return fmt.Errorf("frozen oracle record %q has %d host tokens in %s, want %d", names[i], n, field, wantTokens[field])
			}
		}
		if hasTokens && len(tc.Tokens) != len(wantTokens) {
			return fmt.Errorf("frozen oracle record %q tokenizes %d fields, want %d", names[i], len(tc.Tokens), len(wantTokens))
		}
	}
	for kind, want := range buildRecipeMCPGoldenKindCounts {
		if kinds[kind] != want {
			return fmt.Errorf("frozen oracle has %d %s records, want %d", kinds[kind], kind, want)
		}
	}
	return nil
}

// loadBuildRecipeMCPGolden validates and indexes
// testdata/buildrecipemcp_golden.json. The inventories are asserted before any
// case loop runs.
func loadBuildRecipeMCPGolden(t *testing.T) map[string]buildRecipeMCPGoldenCase {
	t.Helper()
	var bundle struct {
		Names []string                   `json:"names"`
		Cases []buildRecipeMCPGoldenCase `json:"cases"`
	}
	if err := json.Unmarshal(readGoldenBundle(t, "buildrecipemcp_golden.json"), &bundle); err != nil {
		t.Fatalf("parse frozen oracle: %v", err)
	}
	if err := checkBuildRecipeMCPGoldenInventory(bundle.Names, bundle.Cases); err != nil {
		t.Fatalf("frozen buildrecipe-MCP oracle: %v", err)
	}
	golden := make(map[string]buildRecipeMCPGoldenCase, len(bundle.Cases))
	for i, rec := range bundle.Cases {
		golden[bundle.Names[i]] = rec
	}
	return golden
}

// TestBuildRecipeMCPFrozenInventoryControl proves the inventory validation
// bites: the intact bundle validates, while renaming, removing or duplicating a
// record (all in memory, never on disk) fails loudly.
func TestBuildRecipeMCPFrozenInventoryControl(t *testing.T) {
	var bundle struct {
		Names []string                   `json:"names"`
		Cases []buildRecipeMCPGoldenCase `json:"cases"`
	}
	if err := json.Unmarshal(readGoldenBundle(t, "buildrecipemcp_golden.json"), &bundle); err != nil {
		t.Fatalf("parse frozen oracle: %v", err)
	}
	if err := checkBuildRecipeMCPGoldenInventory(bundle.Names, bundle.Cases); err != nil {
		t.Fatalf("intact inventory must validate: %v", err)
	}

	renamed := append([]string{}, bundle.Names...)
	renamed[0] = "renamed record"
	if err := checkBuildRecipeMCPGoldenInventory(renamed, bundle.Cases); err == nil {
		t.Fatal("a renamed record must fail the inventory validation")
	}

	missing := append(append([]buildRecipeMCPGoldenCase{}, bundle.Cases[:1]...), bundle.Cases[2:]...)
	if err := checkBuildRecipeMCPGoldenInventory(bundle.Names, missing); err == nil {
		t.Fatal("a removed record must fail the inventory validation")
	}

	duplicated := append(append([]buildRecipeMCPGoldenCase{}, bundle.Cases...), bundle.Cases[0])
	doubledNames := append(append([]string{}, bundle.Names...), bundle.Names[0])
	if err := checkBuildRecipeMCPGoldenInventory(doubledNames, duplicated); err == nil {
		t.Fatal("a duplicated record must fail the inventory validation")
	}
}

// frozenBuildRecipeMCP returns the named frozen record unchanged, after
// checking that every host-path token it carries has a path supplied. The
// comparison normalizes the GO side instead (mcpFreezeHostPath), symmetric
// with the capture-time tokenization: Go resolves symlinks when it emits the
// managed command (macOS /var -> /private/var) while the captured Python
// printed the raw form, so the raw-vs-resolved spellings must never be
// compared literally.
func frozenBuildRecipeMCP(t *testing.T, golden map[string]buildRecipeMCPGoldenCase, name, catalog, home string) buildRecipeMCPRefOut {
	t.Helper()
	rec, ok := golden[name]
	if !ok {
		t.Fatalf("frozen record %q missing", name)
	}
	check := func(value, token, supplied string) {
		if strings.Contains(value, token) && supplied == "" {
			t.Fatalf("frozen record %q carries %s but no path was supplied", name, token)
		}
	}
	for _, value := range []string{rec.Error, rec.Stdout, rec.Stderr, rec.Document} {
		check(value, buildRecipeMCPCatalogToken, catalog)
		check(value, buildRecipeMCPHomeToken, home)
	}
	return buildRecipeMCPRefOut{
		Error:    rec.Error,
		Stdout:   rec.Stdout,
		Stderr:   rec.Stderr,
		Document: rec.Document,
	}
}

// mcpFreezeHostPath replaces both spellings (raw and EvalSymlinks-resolved)
// of the catalog and home fixture roots with their tokens, symmetric with the
// capture-time tokenization.
func mcpFreezeHostPath(t *testing.T, s, catalog, home string) string {
	t.Helper()
	for _, pair := range [][2]string{
		{buildRecipeMCPCatalogToken, catalog},
		{buildRecipeMCPHomeToken, home},
	} {
		token, path := pair[0], pair[1]
		if path == "" {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
			s = strings.ReplaceAll(s, resolved, token)
		}
		s = strings.ReplaceAll(s, path, token)
	}
	return s
}

// mcpExpandTokens is the inverse of mcpFreezeHostPath: it restores this run's
// fixture paths into a frozen record value, for assertions that pin the
// literal Go message built from the same paths.
func mcpExpandTokens(t *testing.T, s, catalog, home string) string {
	t.Helper()
	for _, pair := range [][2]string{
		{buildRecipeMCPCatalogToken, catalog},
		{buildRecipeMCPHomeToken, home},
	} {
		token, path := pair[0], pair[1]
		if strings.Contains(s, token) {
			if path == "" {
				t.Fatalf("frozen value carries %s but no path was supplied", token)
			}
			s = strings.ReplaceAll(s, token, path)
		}
	}
	return s
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
// A serialization failure (a TOML date/time) is surfaced as an ERROR sentinel
// so a test comparing it against the Python document fails loudly; the
// dedicated boundary test asserts the error text directly.
func nativeMCPDocument(merged *mcpObj) string {
	document, err := mcpJSONDumpsIndent2(merged)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	return document + "\n"
}

// compareBuildRecipeMCP requires the native leg to match the Python reference
// byte for byte: error string, warning stream, and serialized document.
// compareBuildRecipeMCP requires byte equality between the Go surface and the
// frozen record, with the Go side normalized to the frozen host-path tokens
// (both raw and symlink-resolved spellings of the catalog and home roots).
func compareBuildRecipeMCP(t *testing.T, ref buildRecipeMCPRefOut, catalog, home, goErr, goStderr, goDoc string) {
	t.Helper()
	goErr = mcpFreezeHostPath(t, goErr, catalog, home)
	goStderr = mcpFreezeHostPath(t, goStderr, catalog, home)
	goDoc = mcpFreezeHostPath(t, goDoc, catalog, home)
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

// TestBuildRecipeMCPDifferentialAgainstFrozenOracle pins BuildRecipeMCP against
// the frozen Python build_recipe_mcp record: the serialized document, the
// warning stream, and the raised error string must agree scenario by scenario.
func TestBuildRecipeMCPDifferentialAgainstFrozenOracle(t *testing.T) {
	golden := loadBuildRecipeMCPGolden(t)
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
			ref := frozenBuildRecipeMCP(t, golden, tc.name, catalog, "")
			manifest := mustOrderedMCP(t, tc.manifestJSON)
			var goErrBuf bytes.Buffer
			merged, err := BuildRecipeMCP(catalog, tc.recipeIDs, manifest, nil, &goErrBuf)
			goErr, goDoc := "", ""
			if err != nil {
				goErr = err.Error()
			} else {
				goDoc = nativeMCPDocument(merged)
			}
			compareBuildRecipeMCP(t, ref, catalog, "", goErr, goErrBuf.String(), goDoc)
		})
	}
}

// TestBuildRecipeMCPErrorPropagates pins that a read_recipe failure inside the
// public build loop propagates out of BuildRecipeMCP (unlike inside the
// resolver, which catches it).
func TestBuildRecipeMCPErrorPropagates(t *testing.T) {
	golden := loadBuildRecipeMCPGolden(t)
	catalog := buildRecipeMCPCatalog(t)

	ref := frozenBuildRecipeMCP(t, golden, "missing recipe error propagates", catalog, "")
	var goErrBuf bytes.Buffer
	_, err := BuildRecipeMCP(catalog, []string{"ghost"}, newMCPObj(), nil, &goErrBuf)
	if err == nil {
		t.Fatalf("expected an error for a missing recipe directory")
	}
	want := "recipe directory not found: " + filepath.Join(catalog, "ghost")
	if err.Error() != want {
		t.Fatalf("go error = %q, want %q", err.Error(), want)
	}
	if got := mcpExpandTokens(t, ref.Error, catalog, ""); got != want {
		t.Fatalf("frozen oracle error = %q, want %q", ref.Error, want)
	}
	if ref.Document != "" {
		t.Fatalf("frozen oracle document = %q, want empty on error", ref.Document)
	}
}

// --- resolver differential --------------------------------------------------

// TestResolveProviderMarkersDifferentialAgainstFrozenOracle pins
// resolveProviderMarkers against the frozen Python _resolve_provider_markers
// record scenario by scenario.
func TestResolveProviderMarkersDifferentialAgainstFrozenOracle(t *testing.T) {
	golden := loadBuildRecipeMCPGolden(t)
	catalog := buildRecipeMCPCatalog(t)

	t.Run("cannot read recipe for provider resolution", func(t *testing.T) {
		home := t.TempDir()
		mergedJSON := `{}`
		ref := frozenBuildRecipeMCP(t, golden, "cannot read recipe for provider resolution", catalog, home)
		merged := mustOrderedMCP(t, mergedJSON)
		var buf bytes.Buffer
		resolveProviderMarkers(merged, catalog, []string{"ghost"}, home, &buf)
		compareBuildRecipeMCP(t, ref, catalog, home, "", buf.String(), nativeMCPDocument(merged))
	})

	t.Run("missing jinna dependency declaration", func(t *testing.T) {
		home := t.TempDir()
		mergedJSON := `{"jinna":{"command":"{dep:jinna}"}}`
		t.Setenv("PATH", emptyPath(t))
		ref := frozenBuildRecipeMCP(t, golden, "missing jinna dependency declaration", catalog, home)
		merged := mustOrderedMCP(t, mergedJSON)
		var buf bytes.Buffer
		resolveProviderMarkers(merged, catalog, []string{"mcpnodep"}, home, &buf)
		compareBuildRecipeMCP(t, ref, catalog, home, "", buf.String(), nativeMCPDocument(merged))
	})

	t.Run("command not the marker literal is skipped", func(t *testing.T) {
		home := t.TempDir()
		mergedJSON := `{"jinna":{"command":"npx"}}`
		t.Setenv("PATH", emptyPath(t))
		ref := frozenBuildRecipeMCP(t, golden, "command not the marker literal is skipped", catalog, home)
		merged := mustOrderedMCP(t, mergedJSON)
		var buf bytes.Buffer
		resolveProviderMarkers(merged, catalog, []string{"mcpjinna"}, home, &buf)
		compareBuildRecipeMCP(t, ref, catalog, home, "", buf.String(), nativeMCPDocument(merged))
	})

	t.Run("provider unresolved omits the server", func(t *testing.T) {
		home := t.TempDir()
		mergedJSON := `{"jinna":{"command":"{dep:jinna}"}}`
		t.Setenv("PATH", emptyPath(t))
		ref := frozenBuildRecipeMCP(t, golden, "provider unresolved omits the server", catalog, home)
		merged := mustOrderedMCP(t, mergedJSON)
		var buf bytes.Buffer
		resolveProviderMarkers(merged, catalog, []string{"mcpjinna"}, home, &buf)
		compareBuildRecipeMCP(t, ref, catalog, home, "", buf.String(), nativeMCPDocument(merged))
	})

	t.Run("path source resolves to the bare binary", func(t *testing.T) {
		fakeDir := t.TempDir()
		writeFakeJinna(t, fakeDir, "#!/bin/sh\necho jinna 1.2.3\n")
		t.Setenv("PATH", fakeDir)
		home := t.TempDir()
		mergedJSON := `{"jinna":{"command":"{dep:jinna}"}}`
		ref := frozenBuildRecipeMCP(t, golden, "path source resolves to the bare binary", catalog, home)
		merged := mustOrderedMCP(t, mergedJSON)
		var buf bytes.Buffer
		resolveProviderMarkers(merged, catalog, []string{"mcpjinna"}, home, &buf)
		compareBuildRecipeMCP(t, ref, catalog, home, "", buf.String(), nativeMCPDocument(merged))
	})

	t.Run("managed source resolves to the absolute command", func(t *testing.T) {
		// A shared home is required: the absolute command path is observable.
		home := t.TempDir()
		writeManagedJinna(t, home, "0.1.0", "#!/bin/sh\necho jinna 0.1.0\n")
		t.Setenv("PATH", emptyPath(t))
		mergedJSON := `{"jinna":{"command":"{dep:jinna}"}}`
		ref := frozenBuildRecipeMCP(t, golden, "managed source resolves to the absolute command", catalog, home)
		merged := mustOrderedMCP(t, mergedJSON)
		var buf bytes.Buffer
		resolveProviderMarkers(merged, catalog, []string{"mcpjinna"}, home, &buf)
		compareBuildRecipeMCP(t, ref, catalog, home, "", buf.String(), nativeMCPDocument(merged))
	})

	t.Run("resolution failure degrades to a warning", func(t *testing.T) {
		t.Setenv("PATH", emptyPath(t))
		home := t.TempDir()
		writeManagedJinna(t, home, "0.1.0", counterJinnaBody(home))
		mergedJSON := `{"jinna":{"command":"{dep:jinna}"}}`
		ref := frozenBuildRecipeMCP(t, golden, "resolution failure degrades to a warning", catalog, home)
		merged := mustOrderedMCP(t, mergedJSON)
		var buf bytes.Buffer
		resolveProviderMarkers(merged, catalog, []string{"mcpjinna"}, home, &buf)
		compareBuildRecipeMCP(t, ref, catalog, home, "", buf.String(), nativeMCPDocument(merged))
	})
}

// --- widened internal/toml value subset (parity) ----------------------------

// TestBuildRecipeMCPBigIntegerParity pins that an integer outside int64 is a
// parity case now that internal/toml reads arbitrary-precision integers: both
// legs read the recipe and serialize the exact decimal digits, and both
// resolve the provider marker. The build leg compares the serialized document.
func TestBuildRecipeMCPBigIntegerParity(t *testing.T) {
	golden := loadBuildRecipeMCPGolden(t)
	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "bigint", syncRecipeHeader("bigint", "Big Int")+
		"[[deps.cli]]\nbinary = \"jinna\"\npurpose = \"p\"\nmin_version = \"0.1.0\"\n"+
		"[[provides.mcp]]\nid = \"jinna\"\ncommand = \"{dep:jinna}\"\ntimeout = 123456789012345678901234567890\n")

	// Build leg: both sides read the recipe and emit the full integer.
	buildRef := frozenBuildRecipeMCP(t, golden, "big integer build parity", catalog, "")
	if buildRef.Error != "" || buildRef.Stderr != "" {
		t.Fatalf("python build raised or warned: error=%q stderr=%q", buildRef.Error, buildRef.Stderr)
	}
	merged, err := BuildRecipeMCP(catalog, []string{"bigint"}, newMCPObj(), nil, io.Discard)
	if err != nil {
		t.Fatalf("Go build failed to read a big integer: %v", err)
	}
	if goDoc := nativeMCPDocument(merged); goDoc != buildRef.Document {
		t.Fatalf("big integer document:\n go=%q\n py=%q", goDoc, buildRef.Document)
	}

	// Resolver leg: both sides resolve the marker, no warning either side.
	fakeDir := t.TempDir()
	writeFakeJinna(t, fakeDir, "#!/bin/sh\necho jinna 1.2.3\n")
	t.Setenv("PATH", fakeDir)
	home := t.TempDir()
	mergedJSON := `{"jinna":{"command":"{dep:jinna}"}}`
	resolveRef := frozenBuildRecipeMCP(t, golden, "big integer resolve parity", catalog, home)
	if resolveRef.Error != "" || resolveRef.Stderr != "" {
		t.Fatalf("python resolve raised or warned: error=%q stderr=%q", resolveRef.Error, resolveRef.Stderr)
	}
	resolved := mustOrderedMCP(t, mergedJSON)
	var buf bytes.Buffer
	resolveProviderMarkers(resolved, catalog, []string{"bigint"}, home, &buf)
	if buf.String() != "" {
		t.Fatalf("Go resolver warned on a readable recipe: %q", buf.String())
	}
	if goDoc := nativeMCPDocument(resolved); goDoc != resolveRef.Document {
		t.Fatalf("big integer resolve document:\n go=%q\n py=%q", goDoc, resolveRef.Document)
	}
}

// TestBuildRecipeMCPDatetimeParity pins that a TOML date/time is a parity case:
// both legs read it, and both fail at the JSON boundary with Python's exact
// TypeError text. The resolver leg never serializes the recipe value, so both
// sides resolve the marker.
func TestBuildRecipeMCPDatetimeParity(t *testing.T) {
	golden := loadBuildRecipeMCPGolden(t)
	catalog := t.TempDir()
	writeSyncCatalogRecipe(t, catalog, "datetime", syncRecipeHeader("datetime", "DateTime")+
		"[[deps.cli]]\nbinary = \"jinna\"\npurpose = \"p\"\nmin_version = \"0.1.0\"\n"+
		"[[provides.mcp]]\nid = \"jinna\"\ncommand = \"{dep:jinna}\"\nrelease = 1979-05-27T07:32:00Z\n")
	const wantMsg = "Object of type datetime is not JSON serializable"

	// Build leg: both sides read the datetime, then both fail at json.dumps.
	buildRef := frozenBuildRecipeMCP(t, golden, "datetime build parity", catalog, "")
	if buildRef.Error != wantMsg || buildRef.Document != "" {
		t.Fatalf("python build = error %q document %q, want the json.dump TypeError", buildRef.Error, buildRef.Document)
	}
	merged, err := BuildRecipeMCP(catalog, []string{"datetime"}, newMCPObj(), nil, io.Discard)
	if err != nil {
		t.Fatalf("Go build failed to read the datetime: %v", err)
	}
	if _, serr := mcpJSONDumpsIndent2(merged); serr == nil || serr.Error() != wantMsg {
		t.Fatalf("Go JSON boundary = %v, want %q", serr, wantMsg)
	}

	// Resolver leg: no serialization, so both sides resolve the marker.
	fakeDir := t.TempDir()
	writeFakeJinna(t, fakeDir, "#!/bin/sh\necho jinna 1.2.3\n")
	t.Setenv("PATH", fakeDir)
	home := t.TempDir()
	mergedJSON := `{"jinna":{"command":"{dep:jinna}"}}`
	resolveRef := frozenBuildRecipeMCP(t, golden, "datetime resolve parity", catalog, home)
	if resolveRef.Error != "" || resolveRef.Stderr != "" {
		t.Fatalf("python resolve raised or warned: error=%q stderr=%q", resolveRef.Error, resolveRef.Stderr)
	}
	resolved := mustOrderedMCP(t, mergedJSON)
	var buf bytes.Buffer
	resolveProviderMarkers(resolved, catalog, []string{"datetime"}, home, &buf)
	if buf.String() != "" {
		t.Fatalf("Go resolver warned on a readable recipe: %q", buf.String())
	}
	if goDoc := nativeMCPDocument(resolved); goDoc != resolveRef.Document {
		t.Fatalf("datetime resolve document:\n go=%q\n py=%q", goDoc, resolveRef.Document)
	}
}

// TestMCPJSONBoundaryRejectsDatetime is the focused JSON-boundary pin: a TOML
// date/time now reaches mcpJSONDumpsIndent2 (formerly it could not leave
// internal/toml at all), and it must fail with the Python TypeError naming its
// own kind instead of rendering null.
func TestMCPJSONBoundaryRejectsDatetime(t *testing.T) {
	cases := []struct {
		name string
		dt   toml.DateTime
		want string
	}{
		{"offset datetime", toml.DateTime{Kind: toml.DateTimeOffset}, "Object of type datetime is not JSON serializable"},
		{"local datetime", toml.DateTime{Kind: toml.DateTimeLocal}, "Object of type datetime is not JSON serializable"},
		{"local date", toml.DateTime{Kind: toml.DateLocal}, "Object of type date is not JSON serializable"},
		{"local time", toml.DateTime{Kind: toml.TimeLocal}, "Object of type time is not JSON serializable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			merged := newMCPObj()
			inner := newMCPObj()
			inner.set("when", tc.dt)
			merged.set("srv", inner)
			if _, err := mcpJSONDumpsIndent2(merged); err == nil || err.Error() != tc.want {
				t.Fatalf("mcpJSONDumpsIndent2 = %v, want %q", err, tc.want)
			}
		})
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
		got, err := mcpJSONDumpsIndent2(alpha)
		if err != nil {
			t.Fatalf("mcpJSONDumpsIndent2(alpha): %v", err)
		}
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
