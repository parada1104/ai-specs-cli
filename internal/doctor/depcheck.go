package doctor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/toml"
)

// Native port of lib/_internal/dep_check.py's doctor-facing surface
// (check_project_deps and the per-dep probe). Every recipe dep the enabled
// recipes declare is probed with the same semantics as the legacy module:
// existence via PATH, optional version_check under a shell with a 5s timeout,
// and an optional min_version gate. The probe is read-only; any failure is
// "not ok", never an abort.

// depResult mirrors dep_check.DepResult.
type depResult struct {
	Binary       string
	Found        bool
	Version      string
	OK           bool
	InstallURL   string
	Purpose      string
	Required     bool
	RecipeID     string
	Detail       string
	Source       string
	ResolvedPath string
}

// checkRecipeCLIDeps is doctor._check_recipe_cli_deps.
func (d *Doctor) checkRecipeCLIDeps() {
	d.loadManifest()
	var recipes *toml.Table
	if d.manifest.data != nil {
		recipes, _ = d.manifest.data.Table("recipes")
	}
	if recipes == nil || len(recipes.Keys()) == 0 {
		return
	}
	d.emitRecipeDepChecks(d.collectRecipeDepResults())
}

// collectRecipeDepResults is doctor._collect_recipe_dep_results: the aggregate
// of dep_check.check_project_deps for the enabled recipes.
func (d *Doctor) collectRecipeDepResults() []depResult {
	return checkProjectDeps(d.Root, d.Home)
}

// emitRecipeDepChecks maps each DepResult onto its frozen check. Extracted so
// the mapping is testable without a real PATH or catalog.
func (d *Doctor) emitRecipeDepChecks(results []depResult) {
	for _, r := range results {
		switch {
		case r.OK:
			d.add(OK, "recipe-dep", r.Binary+" available for "+r.RecipeID)
		case r.Required:
			guidance := r.InstallURL
			if guidance == "" {
				guidance = "install the required CLI"
			}
			d.add(WARN, "recipe-dep",
				r.Binary+" missing/unusable for "+r.RecipeID+": "+r.Purpose, guidance)
		default:
			d.add(INFO, "recipe-dep",
				"optional "+r.Binary+" not found for "+r.RecipeID+": "+r.Purpose, r.InstallURL)
		}
	}
}

// checkProjectDeps is dep_check.check_project_deps: one DepResult per cli_dep
// of every enabled recipe, in manifest order. A missing / unreadable manifest
// yields no results (the legacy `return []`).
func checkProjectDeps(projectRoot, home string) []depResult {
	manifestPath := filepath.Join(projectRoot, "ai-specs", "ai-specs.toml")
	if !isFile(manifestPath) {
		return nil
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil
	}
	data, err := toml.Parse(raw)
	if err != nil {
		return nil
	}
	recipes, ok := data.Table("recipes")
	if !ok || recipes == nil {
		return nil
	}
	var out []depResult
	for _, recipeID := range recipes.Keys() {
		cfg, ok := recipes.Table(recipeID)
		if !ok {
			continue
		}
		if enabled, isBool := cfg.Bool("enabled"); !isBool || !enabled {
			continue
		}
		recipe, err := readCatalogRecipe(home, recipeID)
		if err != nil {
			continue
		}
		for _, dep := range recipe.CliDeps {
			out = append(out, checkOne(dep, recipeID, home))
		}
	}
	return out
}

// readCatalogRecipe is recipe-read.read_recipe: read
// <home>/catalog/recipes/<recipe_id>/recipe.toml through LoadRecipeToml (the
// load_recipe_toml port), which validates runtime-hook and init-prompt paths
// relative to the recipe directory. Any missing directory/file or validation
// error is the caller's legacy `except Exception: continue`, so the caller
// skips that recipe rather than aborting the whole list.
func readCatalogRecipe(home, recipeID string) (*schema.Recipe, error) {
	return schema.LoadRecipeToml(filepath.Join(home, "catalog", "recipes", recipeID, "recipe.toml"))
}

// checkOne is dep_check._check_one for a non-provider dep.
func checkOne(dep *schema.CliDep, recipeID, home string) depResult {
	if dep.Installer == "github-release" {
		return resolveProviderDep(dep, recipeID, home)
	}
	res := depResult{
		Binary:     dep.Binary,
		InstallURL: dep.InstallURL,
		Purpose:    dep.Purpose,
		Required:   dep.Required,
		RecipeID:   recipeID,
	}
	if !whichBinary(dep.Binary) {
		res.Detail = "not found on PATH"
		return res
	}
	res.Found = true
	version := ""
	if dep.VersionCheck != "" {
		parsed := parseVersion(runVersionCheck(dep.VersionCheck))
		if len(parsed) > 0 {
			version = joinVersion(parsed)
		}
		res.Version = version
		if dep.MinVersion != "" {
			want := parseVersion(dep.MinVersion)
			if len(parsed) == 0 {
				res.OK = true
				res.Detail = "version unknown"
				return res
			}
			if versionGE(parsed, want) {
				res.OK = true
				return res
			}
			res.Detail = "found " + version + " < required " + dep.MinVersion
			return res
		}
	}
	res.Version = version
	res.OK = true
	return res
}

// whichBinary mirrors shutil.which(binary) is not None.
func whichBinary(binary string) bool {
	if binary == "" {
		return false
	}
	_, err := exec.LookPath(binary)
	return err == nil
}

// runVersionCheck is dep_check._run_version_check: shell=True, capture both
// streams, 5s timeout, and "" on any failure (timeout included). stdout is
// concatenated before stderr, exactly as the legacy
// `(proc.stdout or "") + (proc.stderr or "")` did. A non-zero exit is NOT a
// failure here; its output is still returned, exactly as the legacy
// `subprocess.run(check=False)` did.
func runVersionCheck(cmd string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	proc := exec.CommandContext(ctx, "sh", "-c", cmd)
	var stdout, stderr strings.Builder
	proc.Stdout = &stdout
	proc.Stderr = &stderr
	err := proc.Run()
	if ctx.Err() != nil {
		return ""
	}
	if err != nil {
		if _, isExit := err.(*exec.ExitError); !isExit {
			return ""
		}
	}
	return stdout.String() + stderr.String()
}

var depVersionRe = regexp.MustCompile(`\d+(?:\.\d+)*`)

// parseVersion mirrors dep_check._parse_version: the first version-like run of
// digits, split on '.'. Empty when there is no match.
func parseVersion(text string) []int {
	match := depVersionRe.FindString(text)
	if match == "" {
		return nil
	}
	parts := strings.Split(match, ".")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		n, _ := strconv.Atoi(part)
		out = append(out, n)
	}
	return out
}

func joinVersion(parts []int) string {
	out := make([]string, len(parts))
	for i, part := range parts {
		out[i] = strconv.Itoa(part)
	}
	return strings.Join(out, ".")
}

// versionGE mirrors dep_check._version_ge: absent want -> true, absent have ->
// false, otherwise zero-padded lexicographic comparison.
func versionGE(have, want []int) bool {
	if len(want) == 0 {
		return true
	}
	if len(have) == 0 {
		return false
	}
	width := len(have)
	if len(want) > width {
		width = len(want)
	}
	hc := make([]int, width)
	wc := make([]int, width)
	copy(hc, have)
	copy(wc, want)
	return intTupleCompare(hc, wc) >= 0
}

// intTupleCompare mirrors Python's tuple comparison for int tuples (shorter
// equal-prefix tuple is smaller).
func intTupleCompare(a, b []int) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

// --- github-release provider resolution ------------------------------------
//
// The doctor-facing subset of provider_install.resolve_provider, which the
// github-release installer branch of _check_one calls. Read-only and offline:
// PATH probe, then a trust-checked scan of the managed cache. It exists here
// because provider_install.py has no Go port yet; a future port should
// replace this helper with a shared implementation.

const providerRepository = "parada1104/jinna-provider"

// providerVersionRe mirrors provider_install.VERSION_RE's capture, without the
// lookaround RE2 rejects; providerVersionTuple enforces the digit boundaries.
var providerVersionRe = regexp.MustCompile(`\d+(?:\.\d+)+`)

type providerResolution struct {
	version  string
	source   string
	path     string
	hasPath  bool
	target   [2]string
	verified bool
}

var providerPlatforms = map[[2]string]bool{
	{"darwin", "arm64"}:  true,
	{"darwin", "amd64"}:  true,
	{"linux", "arm64"}:   true,
	{"linux", "amd64"}:   true,
	{"windows", "amd64"}: true,
}

// detectProviderPlatform mirrors provider_install.detect_platform, mapping the
// Go runtime's OS/arch onto the supported release targets.
func detectProviderPlatform() [2]string {
	goos := map[string]string{"darwin": "darwin", "linux": "linux", "windows": "windows"}[runtime.GOOS]
	goarch := map[string]string{"arm64": "arm64", "amd64": "amd64"}[runtime.GOARCH]
	target := [2]string{goos, goarch}
	if !providerPlatforms[target] {
		return [2]string{}
	}
	return target
}

func expectedProviderBinary(goos string) string {
	if goos == "windows" {
		return "jinna.exe"
	}
	return "jinna"
}

// providerVersionTuple mirrors provider_install._version_tuple (requires at
// least one '.', and rejects a version whose digits are part of a longer run).
func providerVersionTuple(text string) []int {
	for _, loc := range providerVersionRe.FindAllStringIndex(text, -1) {
		start, end := loc[0], loc[1]
		if start > 0 && text[start-1] >= '0' && text[start-1] <= '9' {
			continue
		}
		if end < len(text) && text[end] >= '0' && text[end] <= '9' {
			continue
		}
		parts := strings.Split(text[start:end], ".")
		out := make([]int, 0, len(parts))
		for _, part := range parts {
			n, _ := strconv.Atoi(part)
			out = append(out, n)
		}
		return out
	}
	return nil
}

// runProviderVersion mirrors provider_install._run_version: run
// `<path> version`, 5s timeout, parse the version, and fail on a non-zero exit
// or an unparseable output.
func runProviderVersion(binaryPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, "version")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("provider version check timed out")
		}
		return "", fmt.Errorf("provider version check failed: %w", err)
	}
	version := providerVersionTuple(stdout.String() + stderr.String())
	if len(version) == 0 {
		return "", fmt.Errorf("provider version check returned no usable version")
	}
	return joinVersion(version), nil
}

func providerVersionAtLeast(have, minimum string) bool {
	want := providerVersionTuple(minimum)
	current := providerVersionTuple(have)
	if len(want) == 0 {
		return true
	}
	if len(current) == 0 {
		return false
	}
	return intTupleCompare(current, want) >= 0
}

// readProviderReceipt mirrors provider_install._read_receipt.
func readProviderReceipt(candidate string) map[string]any {
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(candidate), "install.json"))
	if err != nil {
		return nil
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil
	}
	return data
}

// managedProviderCandidates mirrors provider_install._managed_candidates.
func managedProviderCandidates(root, goos, goarch string) []string {
	if !isDir(root) {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	target := goos + "-" + goarch
	expected := expectedProviderBinary(goos)
	var paths []string
	for _, entry := range entries {
		candidate := filepath.Join(root, entry.Name(), target, expected)
		if isFile(candidate) {
			paths = append(paths, candidate)
		}
	}
	return paths
}

func fileSHA256(filePath string) (string, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// resolveProvider mirrors provider_install.resolve_provider: PATH first, then
// the newest trust-checked managed candidate.
func resolveProvider(dep *schema.CliDep, home string) (providerResolution, error) {
	target := detectProviderPlatform()
	binary := dep.Binary
	if binary == "" {
		binary = "jinna"
	}
	if candidate, err := exec.LookPath(binary); err == nil {
		if version, verr := runProviderVersion(candidate); verr == nil &&
			providerVersionAtLeast(version, dep.MinVersion) {
			return providerResolution{
				version: version, source: "path", path: candidate,
				hasPath: true, target: target, verified: true,
			}, nil
		}
	}
	root := filepath.Join(home, "cache", "bin", "jinna")
	type managedCandidate struct {
		version []int
		path    string
	}
	var best *managedCandidate
	for _, candidate := range managedProviderCandidates(root, target[0], target[1]) {
		receipt := readProviderReceipt(candidate)
		if status, _ := receipt["status"].(string); status != "verified" {
			continue
		}
		if repository, _ := receipt["repository"].(string); repository != providerRepository {
			continue
		}
		if receiptTarget, _ := receipt["target"].(string); receiptTarget != target[0]+"-"+target[1] {
			continue
		}
		tag, _ := receipt["release_tag"].(string)
		tagVersion := providerVersionTuple(tag)
		if len(tagVersion) == 0 {
			continue
		}
		digest, _ := receipt["binary_sha256"].(string)
		if digest == "" {
			continue
		}
		sum, err := fileSHA256(candidate)
		if err != nil || sum != digest {
			continue
		}
		version, err := runProviderVersion(candidate)
		if err != nil || !providerVersionAtLeast(version, dep.MinVersion) {
			continue
		}
		if intTupleCompare(providerVersionTuple(version), tagVersion) != 0 {
			continue
		}
		v := providerVersionTuple(version)
		if best == nil || intTupleCompare(v, best.version) > 0 {
			best = &managedCandidate{version: v, path: candidate}
		}
	}
	if best != nil {
		version, err := runProviderVersion(best.path)
		if err != nil {
			return providerResolution{target: target, source: "unresolved"}, nil
		}
		return providerResolution{
			version: version, source: "managed", path: best.path,
			hasPath: true, target: target, verified: true,
		}, nil
	}
	return providerResolution{target: target, source: "unresolved"}, nil
}

// resolveProviderDep maps a provider resolution onto a DepResult.
func resolveProviderDep(dep *schema.CliDep, recipeID, home string) depResult {
	res := depResult{
		Binary:     dep.Binary,
		InstallURL: dep.InstallURL,
		Purpose:    dep.Purpose,
		Required:   dep.Required,
		RecipeID:   recipeID,
	}
	resolution, err := resolveProvider(dep, home)
	if err != nil {
		res.Detail = "provider check failed: " + err.Error()
		res.Source = "unresolved"
		return res
	}
	if resolution.verified && resolution.hasPath {
		res.Found = true
		res.Version = resolution.version
		res.OK = true
		res.Detail = "using " + resolution.source + " provider"
		res.Source = resolution.source
		res.ResolvedPath = resolution.path
		return res
	}
	res.Version = resolution.version
	res.Source = "unresolved"
	res.Detail = "not found for " + orQuestion(resolution.target[0]) + "/" + orQuestion(resolution.target[1])
	return res
}

func orQuestion(s string) string {
	if s == "" {
		return "?"
	}
	return s
}
