// Package deps is the single authority for recipe CLI-dependency checks and
// the installation plans offered on top of them. It is the native port of
// lib/_internal/dep_check.py's probe (check_cli_deps / check_project_deps /
// _check_one and their helpers) plus dep_install.py's read-only resolution
// half (resolve_install_plan / describe_install_plan) and
// config_wizard._render_dep_panel's line format.
//
// It exists as its own package because TWO callers need the same answers:
// `internal/doctor` (whose probe this used to be, unexported, and which now
// delegates here — one authoritative grader per behaviour) and the CLI-dep
// gate (ported next, which needs the per-recipe form `check_cli_deps` that
// doctor's manifest-wide walk never exposed).
//
// What is NOT here yet, by design: the TTY offer, the installs and the
// "Configure anyway?" confirm. `_dep_gate` itself still runs through the
// legacy seam (S37) until the gate slice replaces it.
package deps

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ai-specs.dev/ai-specs/internal/provider"
	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/toml"
)

// Result mirrors dep_check.DepResult field for field.
type Result struct {
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

// lookPath is shutil.which: the only authority for "is this binary reachable".
// It is a variable so the plan tests can script the same matrix the legacy
// module's differential scripts (which binaries the host ships).
var lookPath = exec.LookPath

// Which mirrors shutil.which(binary) is not None.
func Which(binary string) bool {
	if binary == "" {
		return false
	}
	_, err := lookPath(binary)
	return err == nil
}

// CheckRecipe is dep_check.check_cli_deps: one Result per declared CLI dep, in
// declaration order, carrying the recipe id. The legacy module's `except
// Exception` fallback ("check failed") has no Go analogue because neither arm
// below can fail without a panic: the provider arm returns its error, and the
// PATH arm is read-only.
func CheckRecipe(recipe *schema.Recipe, home string) []Result {
	out := make([]Result, 0, len(recipe.CliDeps))
	for _, dep := range recipe.CliDeps {
		out = append(out, CheckOne(dep, recipe.ID, home))
	}
	return out
}

// CheckOne is dep_check._check_one.
func CheckOne(dep *schema.CliDep, recipeID, home string) Result {
	if dep.Installer == "github-release" {
		return resolveProviderDep(dep, recipeID, home)
	}
	res := Result{
		Binary:     dep.Binary,
		InstallURL: dep.InstallURL,
		Purpose:    dep.Purpose,
		Required:   dep.Required,
		RecipeID:   recipeID,
	}
	if !Which(dep.Binary) {
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

// CheckProject is dep_check.check_project_deps: one Result per cli_dep of
// every enabled recipe, in manifest order. A missing or unreadable manifest
// yields no results (the legacy `return []`).
func CheckProject(projectRoot, home string) []Result {
	manifestPath := filepath.Join(projectRoot, "ai-specs", "ai-specs.toml")
	info, err := os.Stat(manifestPath)
	if err != nil || info.IsDir() {
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
	var out []Result
	for _, recipeID := range recipes.Keys() {
		cfg, ok := recipes.Table(recipeID)
		if !ok {
			continue
		}
		if enabled, isBool := cfg.Bool("enabled"); !isBool || !enabled {
			continue
		}
		recipe, err := ReadCatalogRecipe(home, recipeID)
		if err != nil {
			continue
		}
		// check_project_deps overwrites the recipe id with the MANIFEST key
		// after probing, so a recipe.toml whose declared id differs from its
		// directory still reports the key the manifest (and doctor) names.
		results := CheckRecipe(recipe, home)
		for i := range results {
			results[i].RecipeID = recipeID
		}
		out = append(out, results...)
	}
	return out
}

// ReadCatalogRecipe is recipe-read.read_recipe: read
// <home>/catalog/recipes/<recipe_id>/recipe.toml through LoadRecipeToml (the
// load_recipe_toml port), which validates runtime-hook and init-prompt paths
// relative to the recipe directory. Any missing directory/file or validation
// error is the caller's legacy `except Exception: continue`, so the caller
// skips that recipe rather than aborting the whole list.
func ReadCatalogRecipe(home, recipeID string) (*schema.Recipe, error) {
	return schema.LoadRecipeToml(filepath.Join(home, "catalog", "recipes", recipeID, "recipe.toml"))
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

var versionRe = regexp.MustCompile(`\d+(?:\.\d+)*`)

// parseVersion mirrors dep_check._parse_version: the first version-like run of
// digits, split on '.'. Empty when there is no match.
func parseVersion(text string) []int {
	match := versionRe.FindString(text)
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

// resolveProviderDep maps a provider resolution onto a Result, mirroring
// dep_check._check_one's github-release branch: an error becomes
// "provider check failed: <exc>" with source "unresolved"; a verified
// resolution carries its source and resolved path; otherwise it stays
// unresolved with the "not found for goos/goarch" detail.
func resolveProviderDep(dep *schema.CliDep, recipeID, home string) Result {
	res := Result{
		Binary:     dep.Binary,
		InstallURL: dep.InstallURL,
		Purpose:    dep.Purpose,
		Required:   dep.Required,
		RecipeID:   recipeID,
	}
	resolution, err := provider.Resolve(dep, installHome(home))
	if err != nil {
		res.Detail = "provider check failed: " + err.Error()
		res.Source = "unresolved"
		return res
	}
	if resolution.Verified && resolution.Path != nil {
		res.Found = true
		res.Version = resolution.Version
		res.OK = true
		res.Detail = "using " + resolution.Source + " provider"
		res.Source = resolution.Source
		res.ResolvedPath = *resolution.Path
		return res
	}
	res.Version = resolution.Version
	res.Source = "unresolved"
	res.Detail = "not found for " + orQuestion(resolution.Target[0]) + "/" + orQuestion(resolution.Target[1])
	return res
}

func orQuestion(s string) string {
	if s == "" {
		return "?"
	}
	return s
}

// installHome maps an explicit install root onto provider.CacheRoot's contract:
// the empty string means "resolve it the legacy way" (env, then the user's
// cache), never a relative path.
func installHome(home string) *string {
	if home == "" {
		return nil
	}
	return &home
}
