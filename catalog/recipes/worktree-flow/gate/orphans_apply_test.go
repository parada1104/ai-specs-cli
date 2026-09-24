package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// applyEnvelope builds the --apply-orphans stdin envelope: the plan input
// fields of --plan-orphans plus the three resolved cache roots.
func applyEnvelope(t *testing.T, recipeRoot, depsRoot, inprojectRoot string,
	recipeSkills, depsSkills, inprojectDeps, lockRecipes, enabled, expected []string) string {
	t.Helper()
	envelope := map[string]any{
		"recipe_skills":      recipeSkills,
		"deps_skills":        depsSkills,
		"inproject_deps":     inprojectDeps,
		"lock_recipes":       lockRecipes,
		"enabled_recipe_ids": enabled,
		"expected_dep_ids":   expected,
		"roots": map[string]string{
			"recipe_skills":  recipeRoot,
			"deps_skills":    depsRoot,
			"inproject_deps": inprojectRoot,
		},
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return string(raw)
}

// runApplyCLI invokes run() with --apply-orphans and returns the exit code,
// stdout and stderr, mirroring runPlanCLI in orphans_plan_test.go.
func runApplyCLI(t *testing.T, stdin string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run([]string{"--apply-orphans"}, strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// decodeApplyOutcome parses the structured outcome JSON from stdout.
func decodeApplyOutcome(t *testing.T, stdout string) orphanApplyOutcome {
	t.Helper()
	var outcome orphanApplyOutcome
	if err := json.Unmarshal([]byte(stdout), &outcome); err != nil {
		t.Fatalf("stdout is not a valid apply outcome: %v\nraw: %s", err, stdout)
	}
	return outcome
}

// mustMkdir creates a directory and fails the test on error.
func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func TestApplyOrphansRemovesOrphanedDirectoriesAcrossScopes(t *testing.T) {
	tmp := t.TempDir()
	recipeRoot := filepath.Join(tmp, "recipe")
	depsRoot := filepath.Join(tmp, "deps")
	inprojectRoot := filepath.Join(tmp, "inproject")
	mustMkdir(t, filepath.Join(recipeRoot, "orphan-a"))
	mustMkdir(t, filepath.Join(recipeRoot, "keep-recipe"))
	mustMkdir(t, filepath.Join(depsRoot, "orphan-b"))
	mustMkdir(t, filepath.Join(depsRoot, "keep-dep"))
	mustMkdir(t, filepath.Join(inprojectRoot, "orphan-c"))

	stdin := applyEnvelope(t, recipeRoot, depsRoot, inprojectRoot,
		[]string{"orphan-a", "keep-recipe"}, []string{"orphan-b", "keep-dep"},
		[]string{"orphan-c"}, []string{"ghost-lock"}, []string{"keep-recipe"},
		[]string{"keep-dep"})
	code, stdout, stderr := runApplyCLI(t, stdin)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	outcome := decodeApplyOutcome(t, stdout)
	wantRemoved := []orphanApplyEntry{
		{Scope: "recipe_skills", Name: "orphan-a"},
		{Scope: "deps_skills", Name: "orphan-b"},
		{Scope: "inproject_deps", Name: "orphan-c"},
	}
	if outcome.Status != statusApplied {
		t.Errorf("status = %q, want %q (error: %s)", outcome.Status, statusApplied, outcome.Error)
	}
	if !reflect.DeepEqual(outcome.Removed, wantRemoved) {
		t.Errorf("removed = %+v, want %+v", outcome.Removed, wantRemoved)
	}
	if len(outcome.Remaining) != 0 {
		t.Errorf("remaining = %+v, want empty", outcome.Remaining)
	}
	for _, gone := range []string{
		filepath.Join(recipeRoot, "orphan-a"),
		filepath.Join(depsRoot, "orphan-b"),
		filepath.Join(inprojectRoot, "orphan-c"),
	} {
		if _, err := os.Lstat(gone); !os.IsNotExist(err) {
			t.Errorf("orphan %s still exists (lstat err: %v)", gone, err)
		}
	}
	for _, kept := range []string{
		filepath.Join(recipeRoot, "keep-recipe"),
		filepath.Join(depsRoot, "keep-dep"),
	} {
		if _, err := os.Lstat(kept); err != nil {
			t.Errorf("expected child %s to survive: %v", kept, err)
		}
	}
}

func TestApplyOrphansPreservesNonDirectoryChildren(t *testing.T) {
	tmp := t.TempDir()
	recipeRoot := filepath.Join(tmp, "recipe")
	depsRoot := filepath.Join(tmp, "deps")
	mustMkdir(t, recipeRoot)
	mustMkdir(t, depsRoot)
	// A regular file planned as an orphan: preserved.
	strayFile := filepath.Join(recipeRoot, "orphan-file")
	if err := os.WriteFile(strayFile, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("write stray file: %v", err)
	}
	// A symlink to a directory planned as an orphan: preserved, never traversed.
	symlinkTarget := filepath.Join(tmp, "real-target")
	mustMkdir(t, symlinkTarget)
	symlink := filepath.Join(depsRoot, "orphan-symlink")
	if err := os.Symlink(symlinkTarget, symlink); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	stdin := applyEnvelope(t, recipeRoot, depsRoot, "",
		[]string{"orphan-file"}, []string{"orphan-symlink"}, nil, nil, nil, nil)
	code, stdout, stderr := runApplyCLI(t, stdin)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	outcome := decodeApplyOutcome(t, stdout)
	if outcome.Status != statusApplied {
		t.Errorf("status = %q, want %q (error: %s)", outcome.Status, statusApplied, outcome.Error)
	}
	if len(outcome.Removed) != 0 {
		t.Errorf("removed = %+v, want empty (non-directory children are preserved)", outcome.Removed)
	}
	data, err := os.ReadFile(strayFile)
	if err != nil || string(data) != "keep me" {
		t.Errorf("stray file was altered or removed: %v (%q)", err, data)
	}
	info, err := os.Lstat(symlink)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink was removed or replaced: %v", err)
	}
	if _, err := os.Lstat(symlinkTarget); err != nil {
		t.Errorf("symlink target was removed through traversal: %v", err)
	}
}

func TestApplyOrphansTreatsMissingChildrenAsApplied(t *testing.T) {
	tmp := t.TempDir()
	recipeRoot := filepath.Join(tmp, "missing-root")
	depsRoot := filepath.Join(tmp, "deps")
	mustMkdir(t, depsRoot)

	stdin := applyEnvelope(t, recipeRoot, depsRoot, "",
		[]string{"already-gone"}, []string{"also-gone"}, nil, nil, nil, nil)
	code, stdout, stderr := runApplyCLI(t, stdin)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	outcome := decodeApplyOutcome(t, stdout)
	if outcome.Status != statusApplied {
		t.Errorf("status = %q, want %q (error: %s)", outcome.Status, statusApplied, outcome.Error)
	}
	if len(outcome.Removed) != 0 || len(outcome.Remaining) != 0 {
		t.Errorf("removed/remaining = %+v/%+v, want both empty", outcome.Removed, outcome.Remaining)
	}
}

func TestApplyOrphansRejectsPathTraversalWithoutSideEffects(t *testing.T) {
	tmp := t.TempDir()
	recipeRoot := filepath.Join(tmp, "roots", "recipe")
	mustMkdir(t, recipeRoot)
	escaped := filepath.Join(tmp, "evil")
	if err := os.WriteFile(escaped, []byte("do not delete"), 0o644); err != nil {
		t.Fatalf("write escape target: %v", err)
	}

	stdin := applyEnvelope(t, recipeRoot, "", "",
		[]string{"../../evil"}, nil, nil, nil, nil, nil)
	code, stdout, _ := runApplyCLI(t, stdin)
	if code != 3 {
		t.Fatalf("exit = %d, want 3", code)
	}
	outcome := decodeApplyOutcome(t, stdout)
	if outcome.Status != statusPreApplyFailed {
		t.Errorf("status = %q, want %q", outcome.Status, statusPreApplyFailed)
	}
	if outcome.Error == "" {
		t.Errorf("error = empty, want a validation reason")
	}
	if len(outcome.Removed) != 0 {
		t.Errorf("removed = %+v, want empty (nothing may be attempted)", outcome.Removed)
	}
	data, err := os.ReadFile(escaped)
	if err != nil || string(data) != "do not delete" {
		t.Errorf("path escaped the root: target altered/removed: %v (%q)", err, data)
	}
}

func TestApplyOrphansStopsOnFirstRemovalFailure(t *testing.T) {
	tmp := t.TempDir()
	recipeRoot := filepath.Join(tmp, "recipe")
	depsRoot := filepath.Join(tmp, "deps")
	for _, dir := range []string{
		filepath.Join(recipeRoot, "a-first"),
		filepath.Join(recipeRoot, "b-fail"),
		filepath.Join(recipeRoot, "c-after"),
		filepath.Join(depsRoot, "d-deps"),
	} {
		mustMkdir(t, dir)
	}
	in := applyOrphansInput{
		Roots: orphanApplyRoots{RecipeSkills: recipeRoot, DepsSkills: depsRoot},
		orphanPlanInput: orphanPlanInput{
			RecipeSkills:     []string{"a-first", "b-fail", "c-after"},
			DepsSkills:       []string{"d-deps"},
			EnabledRecipeIDs: []string{},
			ExpectedDepIDs:   []string{},
		},
	}
	// The injected remover succeeds everywhere except the second planned
	// target, so the failure must stop the run before c-after and d-deps.
	remove := func(path string) error {
		if strings.Contains(path, "b-fail") {
			return os.ErrPermission
		}
		return os.RemoveAll(path)
	}
	outcome := applyOrphans(in, remove)
	if outcome.Status != statusPartial {
		t.Errorf("status = %q, want %q (error: %s)", outcome.Status, statusPartial, outcome.Error)
	}
	wantRemoved := []orphanApplyEntry{{Scope: "recipe_skills", Name: "a-first"}}
	if !reflect.DeepEqual(outcome.Removed, wantRemoved) {
		t.Errorf("removed = %+v, want %+v", outcome.Removed, wantRemoved)
	}
	wantRemaining := []orphanApplyEntry{
		{Scope: "recipe_skills", Name: "b-fail", Uncertain: true},
		{Scope: "recipe_skills", Name: "c-after"},
		{Scope: "deps_skills", Name: "d-deps"},
	}
	if !reflect.DeepEqual(outcome.Remaining, wantRemaining) {
		t.Errorf("remaining = %+v, want %+v", outcome.Remaining, wantRemaining)
	}
	if _, err := os.Lstat(filepath.Join(recipeRoot, "c-after")); err != nil {
		t.Errorf("first-error stop violated: c-after was touched: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(depsRoot, "d-deps")); err != nil {
		t.Errorf("first-error stop violated: d-deps was touched: %v", err)
	}
}

func TestApplyOrphansValidationFailureIsPreApply(t *testing.T) {
	tmp := t.TempDir()
	// A relative root with a planned orphan is a pre-apply contract failure:
	// nothing may be attempted and every planned entry stays in remaining.
	in := applyOrphansInput{
		Roots: orphanApplyRoots{RecipeSkills: filepath.Join(tmp, "recipe")},
		orphanPlanInput: orphanPlanInput{
			RecipeSkills:     []string{"orphan"},
			EnabledRecipeIDs: []string{},
		},
	}
	in.Roots.RecipeSkills = filepath.Join("relative", "root")
	remove := func(path string) error {
		t.Errorf("remover must not be called on a validation failure; got %s", path)
		return nil
	}
	outcome := applyOrphans(in, remove)
	if outcome.Status != statusPreApplyFailed {
		t.Errorf("status = %q, want %q", outcome.Status, statusPreApplyFailed)
	}
	wantRemaining := []orphanApplyEntry{{Scope: "recipe_skills", Name: "orphan"}}
	if !reflect.DeepEqual(outcome.Remaining, wantRemaining) {
		t.Errorf("remaining = %+v, want %+v", outcome.Remaining, wantRemaining)
	}
	if len(outcome.Removed) != 0 {
		t.Errorf("removed = %+v, want empty", outcome.Removed)
	}
}

func TestApplyOrphansCLIMalformedInput(t *testing.T) {
	code, stdout, stderr := runApplyCLI(t, "{not json")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty on malformed input", stdout)
	}
	if !strings.Contains(stderr, "apply-orphans") {
		t.Errorf("stderr = %q, want an --apply-orphans diagnostic", stderr)
	}
}
