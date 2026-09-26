package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ai-specs.dev/worktree-gate/ledger"
)

// gitRepo creates an empty Git repository in a temp dir and returns its resolved
// path. The Git common dir is what the witness hangs off, so a real repository is
// the smallest honest fixture for the durable write.
func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available for the durable witness fixture")
	}
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init", "-q").Run(); err != nil {
		t.Fatalf("git init %s: %v", dir, err)
	}
	return RealPath(dir)
}

// wantCommonDir resolves the fixture repository's Git common dir the same way the
// writer does: absolute path-format output, then realpath.
func wantCommonDir(t *testing.T, root string) string {
	t.Helper()
	common := git(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if common == "" {
		t.Fatalf("git common dir of %s is empty", root)
	}
	return RealPath(common)
}

const witnessWhen = "2026-01-02T03:04:05Z"

// TestTrackerWitnessPayloadStates mirrors tracker_witness_payload from the Python
// authority: a resolved tracker binding is bound and names its recipe, otherwise
// every enabled tracker declarer becomes a candidate, in enabled order, and only
// more than one candidate is ambiguous. A declaration distinguishes
// declared-not-bound from unbound and never activates anything (D6).
func TestTrackerWitnessPayloadStates(t *testing.T) {
	cases := []struct {
		name     string
		enabled  []string
		caps     map[string][]string
		resolved map[string]string
		declared bool
		want     witnessPayload
	}{
		{
			name:     "bound names the resolved recipe and records no candidate",
			enabled:  []string{"alpha"},
			caps:     map[string][]string{"alpha": {"tracker"}},
			resolved: map[string]string{"tracker": "alpha"},
			want:     witnessPayload{V: 1, Capability: "tracker", State: ledger.WitnessBound, RecipeID: "alpha", Candidates: []string{}, WrittenAt: witnessWhen},
		},
		{
			name:     "bound wins over a competing declarer and still records no candidate",
			enabled:  []string{"alpha", "beta"},
			caps:     map[string][]string{"alpha": {"tracker"}, "beta": {"tracker"}},
			resolved: map[string]string{"tracker": "beta"},
			want:     witnessPayload{V: 1, Capability: "tracker", State: ledger.WitnessBound, RecipeID: "beta", Candidates: []string{}, WrittenAt: witnessWhen},
		},
		{
			name:     "more than one declarer is ambiguous and records every candidate in enabled order",
			enabled:  []string{"beta", "alpha", "gamma"},
			caps:     map[string][]string{"alpha": {"tracker"}, "beta": {"tracker"}, "gamma": {"vcs"}},
			resolved: map[string]string{},
			want:     witnessPayload{V: 1, Capability: "tracker", State: ledger.WitnessAmbiguous, RecipeID: "", Candidates: []string{"beta", "alpha"}, WrittenAt: witnessWhen},
		},
		{
			name:     "a declaration with no declarer is declared-not-bound",
			enabled:  []string{"gamma"},
			caps:     map[string][]string{"gamma": {"vcs"}},
			resolved: map[string]string{},
			declared: true,
			want:     witnessPayload{V: 1, Capability: "tracker", State: ledger.WitnessDeclaredNotBound, RecipeID: "", Candidates: []string{}, WrittenAt: witnessWhen},
		},
		{
			name:     "no declarer and no declaration is unbound",
			enabled:  []string{"gamma"},
			caps:     map[string][]string{"gamma": {"vcs"}},
			resolved: map[string]string{},
			want:     witnessPayload{V: 1, Capability: "tracker", State: ledger.WitnessUnbound, RecipeID: "", Candidates: []string{}, WrittenAt: witnessWhen},
		},
		{
			name:     "exactly one candidate with a declaration is declared-not-bound and records it",
			enabled:  []string{"alpha"},
			caps:     map[string][]string{"alpha": {"tracker"}},
			resolved: map[string]string{},
			declared: true,
			want:     witnessPayload{V: 1, Capability: "tracker", State: ledger.WitnessDeclaredNotBound, RecipeID: "", Candidates: []string{"alpha"}, WrittenAt: witnessWhen},
		},
		{
			name:     "an unreadable enabled recipe declares nothing",
			enabled:  []string{"ghost", "alpha"},
			caps:     map[string][]string{"alpha": {}},
			resolved: map[string]string{},
			want:     witnessPayload{V: 1, Capability: "tracker", State: ledger.WitnessUnbound, RecipeID: "", Candidates: []string{}, WrittenAt: witnessWhen},
		},
		{
			name:     "a non-tracker binding never produces a tracker state",
			enabled:  []string{"alpha"},
			caps:     map[string][]string{"alpha": {"vcs"}},
			resolved: map[string]string{"vcs": "alpha"},
			want:     witnessPayload{V: 1, Capability: "tracker", State: ledger.WitnessUnbound, RecipeID: "", Candidates: []string{}, WrittenAt: witnessWhen},
		},
		{
			name: "empty inputs are unbound",
			want: witnessPayload{V: 1, Capability: "tracker", State: ledger.WitnessUnbound, RecipeID: "", Candidates: []string{}, WrittenAt: witnessWhen},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := trackerWitnessPayload(tc.enabled, tc.caps, tc.resolved, tc.declared, witnessWhen)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("trackerWitnessPayload() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestWitnessWrittenAtIsUTCSecondPrecision pins the timestamp format the Python
// writer produces with strftime("%Y-%m-%dT%H:%M:%SZ"): UTC, second precision.
func TestWitnessWrittenAtIsUTCSecondPrecision(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		{name: "utc", now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), want: "2026-01-02T03:04:05Z"},
		{name: "offset is converted to utc", now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("minus3", -3*3600)), want: "2026-01-02T06:04:05Z"},
		{name: "sub-second precision is truncated", now: time.Date(2026, 1, 2, 3, 4, 5, 999999999, time.UTC), want: "2026-01-02T03:04:05Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := witnessWrittenAt(tc.now); got != tc.want {
				t.Fatalf("witnessWrittenAt(%v) = %q, want %q", tc.now, got, tc.want)
			}
		})
	}
}

// TestMarshalTrackerWitnessByteShape pins the on-disk bytes to the Python writer's
// json.dump(indent=2, sort_keys=True) plus a trailing newline, so the Go reader
// and the Python bridge read the same file. Keys are alphabetically ordered and
// an empty candidate set stays a list, never null.
func TestMarshalTrackerWitnessByteShape(t *testing.T) {
	cases := []struct {
		name    string
		payload witnessPayload
		want    string
	}{
		{
			name: "ambiguous with candidates",
			payload: witnessPayload{
				Candidates: []string{"alpha", "beta"}, Capability: "tracker",
				RecipeID: "", State: ledger.WitnessAmbiguous, V: 1, WrittenAt: witnessWhen,
			},
			want: "{\n" +
				"  \"candidates\": [\n    \"alpha\",\n    \"beta\"\n  ],\n" +
				"  \"capability\": \"tracker\",\n" +
				"  \"recipe_id\": \"\",\n" +
				"  \"state\": \"ambiguous\",\n" +
				"  \"v\": 1,\n" +
				"  \"written_at\": \"2026-01-02T03:04:05Z\"\n" +
				"}\n",
		},
		{
			name: "bound with no candidates",
			payload: witnessPayload{
				Candidates: []string{}, Capability: "tracker",
				RecipeID: "alpha", State: ledger.WitnessBound, V: 1, WrittenAt: witnessWhen,
			},
			want: "{\n" +
				"  \"candidates\": [],\n" +
				"  \"capability\": \"tracker\",\n" +
				"  \"recipe_id\": \"alpha\",\n" +
				"  \"state\": \"bound\",\n" +
				"  \"v\": 1,\n" +
				"  \"written_at\": \"2026-01-02T03:04:05Z\"\n" +
				"}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := marshalTrackerWitness(tc.payload)
			if err != nil {
				t.Fatalf("marshalTrackerWitness: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("bytes = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestWriteTrackerWitnessIsAtomicAndDurable writes through a temp file in the
// ledger directory into <git-common-dir>/ai-specs/ledger/witness.json, leaves no
// temp residue, overwrites in place, and produces bytes the ledger reader
// accepts as active.
func TestWriteTrackerWitnessIsAtomicAndDurable(t *testing.T) {
	root := gitRepo(t)
	target := ledger.WitnessPath(wantCommonDir(t, root))

	first := trackerWitnessPayload([]string{"alpha"}, map[string][]string{"alpha": {"tracker"}}, map[string]string{"tracker": "alpha"}, false, witnessWhen)
	outcome := writeTrackerWitness(root, first)
	if outcome.Warning != "" {
		t.Fatalf("writeTrackerWitness warning = %q, want none", outcome.Warning)
	}
	if outcome.Path != target {
		t.Fatalf("writeTrackerWitness path = %q, want %q", outcome.Path, target)
	}

	stored := readWitness(t, target)
	if !reflect.DeepEqual(stored, first) {
		t.Fatalf("stored witness = %#v, want %#v", stored, first)
	}
	if binding := ledger.ReadBinding(target); !binding.Active() || binding.RecipeID != "alpha" {
		t.Fatalf("ledger.ReadBinding = %#v, want the bound alpha witness", binding)
	}
	assertNoTempResidue(t, filepath.Dir(target))

	// A second write overwrites the durable state rather than appending beside it
	// (deactivation must be able to clear a previously bound provider).
	second := trackerWitnessPayload(nil, nil, map[string]string{}, false, witnessWhen)
	if outcome := writeTrackerWitness(root, second); outcome.Warning != "" || outcome.Path != target {
		t.Fatalf("second writeTrackerWitness = %#v, want the same path with no warning", outcome)
	}
	if stored := readWitness(t, target); stored.State != ledger.WitnessUnbound || stored.RecipeID != "" {
		t.Fatalf("stored witness = %#v, want the unbound overwrite", stored)
	}
	assertNoTempResidue(t, filepath.Dir(target))
}

// TestWriteTrackerWitnessSkipsOutsideRepository mirrors the Python best-effort
// contract: without a Git common dir there is no witness and no warning.
func TestWriteTrackerWitnessSkipsOutsideRepository(t *testing.T) {
	root := RealPath(t.TempDir())
	if gitCommon(root) != "" {
		t.Skip("temp dir is inside a Git repository; the outside-repository case is unreachable here")
	}
	payload := trackerWitnessPayload(nil, nil, map[string]string{}, false, witnessWhen)
	outcome := writeTrackerWitness(root, payload)
	if outcome.Path != "" || outcome.Warning != "" {
		t.Fatalf("writeTrackerWitness = %#v, want a silent skip", outcome)
	}
	if _, err := os.Stat(filepath.Join(root, "ai-specs")); !os.IsNotExist(err) {
		t.Fatalf("ledger directory created outside a repository: %v", err)
	}
}

// TestWriteTrackerWitnessWarnsAndStaysDormant makes the durable write impossible
// (the ledger path is a regular file) and requires a warning instead of a
// failure: the ledger stays dormant and the caller keeps running.
func TestWriteTrackerWitnessWarnsAndStaysDormant(t *testing.T) {
	root := gitRepo(t)
	blocker := filepath.Join(wantCommonDir(t, root), "ai-specs", "ledger")
	if err := os.MkdirAll(filepath.Dir(blocker), 0o755); err != nil {
		t.Fatalf("mkdir blocker parent: %v", err)
	}
	if err := os.WriteFile(blocker, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	payload := trackerWitnessPayload([]string{"alpha"}, map[string][]string{"alpha": {"tracker"}}, map[string]string{"tracker": "alpha"}, false, witnessWhen)
	outcome := writeTrackerWitness(root, payload)
	if outcome.Path != "" {
		t.Fatalf("writeTrackerWitness path = %q, want no path on failure", outcome.Path)
	}
	if !strings.Contains(outcome.Warning, "tracker witness not written") || !strings.Contains(outcome.Warning, "ledger stays dormant") {
		t.Fatalf("writeTrackerWitness warning = %q, want the dormant-ledger warning", outcome.Warning)
	}
	if info, err := os.Stat(blocker); err != nil || info.IsDir() {
		t.Fatalf("blocker changed: info = %v, err = %v", info, err)
	}
	assertNoTempResidue(t, filepath.Dir(blocker))
}

// TestRunResolveBindingsWritesWitness exercises the flag wiring end to end: one
// invocation resolves bindings and persists the durable witness relative to
// --project-root, the write is best-effort, and it never turns into a process
// failure. It runs the real TOML acquisition subprocess, so it is skipped in
// short mode and without python3.
func TestRunResolveBindingsWritesWitness(t *testing.T) {
	if testing.Short() {
		t.Skip("command test runs the standard TOML parser subprocess")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for the TOML acquisition seam")
	}

	catalogDir := t.TempDir()
	writeRecipeToml(t, catalogDir, "alpha", "\n[[capabilities]]\nid = \"tracker\"\n")

	t.Run("writes the bound witness by default", func(t *testing.T) {
		root := gitRepo(t)
		code, stdout, stderr := runCLI(t, "--resolve-bindings", "--catalog-dir", catalogDir, "--recipe", "alpha", "--project-root", root)
		if code != 0 {
			t.Fatalf("code = %d stderr = %q, want 0", code, stderr)
		}
		if got := decodeResolution(t, stdout); len(got.Warnings) != 0 {
			t.Fatalf("warnings = %#v, want none on a successful write", got.Warnings)
		}
		binding := ledger.ReadBinding(ledger.WitnessPath(wantCommonDir(t, root)))
		if !binding.Active() || binding.RecipeID != "alpha" {
			t.Fatalf("ledger.ReadBinding = %#v, want the bound alpha witness", binding)
		}
	})

	t.Run("write-witness=false skips the durable write", func(t *testing.T) {
		root := gitRepo(t)
		code, stdout, stderr := runCLI(t, "--resolve-bindings", "--catalog-dir", catalogDir, "--recipe", "alpha", "--project-root", root, "--write-witness=false")
		if code != 0 {
			t.Fatalf("code = %d stderr = %q, want 0", code, stderr)
		}
		if got := decodeResolution(t, stdout); len(got.Warnings) != 0 {
			t.Fatalf("warnings = %#v, want none", got.Warnings)
		}
		target := ledger.WitnessPath(wantCommonDir(t, root))
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("witness written despite --write-witness=false: %v", err)
		}
	})

	t.Run("a resolution error never writes a guessed witness", func(t *testing.T) {
		root := gitRepo(t)
		code, stdout, stderr := runCLI(t, "--resolve-bindings", "--catalog-dir", catalogDir, "--recipe", "alpha", "--project-root", root, "--bindings", `[{"capability":"tracker","recipe":"ghost"}]`)
		if code != 0 {
			t.Fatalf("code = %d stderr = %q, want 0", code, stderr)
		}
		if got := decodeResolution(t, stdout); len(got.Errors) != 1 {
			t.Fatalf("errors = %#v, want the resolution error", got.Errors)
		}
		target := ledger.WitnessPath(wantCommonDir(t, root))
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("witness written after a resolution error: %v", err)
		}
	})

	t.Run("a write failure is a warning, never a process failure", func(t *testing.T) {
		root := gitRepo(t)
		blocker := filepath.Join(wantCommonDir(t, root), "ai-specs", "ledger")
		if err := os.MkdirAll(filepath.Dir(blocker), 0o755); err != nil {
			t.Fatalf("mkdir blocker parent: %v", err)
		}
		if err := os.WriteFile(blocker, []byte("not a directory\n"), 0o644); err != nil {
			t.Fatalf("write blocker: %v", err)
		}
		code, stdout, stderr := runCLI(t, "--resolve-bindings", "--catalog-dir", catalogDir, "--recipe", "alpha", "--project-root", root)
		if code != 0 {
			t.Fatalf("code = %d stderr = %q, want 0", code, stderr)
		}
		got := decodeResolution(t, stdout)
		if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "ledger stays dormant") {
			t.Fatalf("warnings = %#v, want one dormant-ledger warning", got.Warnings)
		}
	})
}

// decodeResolution decodes the stdout result contract and fails the test when it
// is not the expected JSON object.
func decodeResolution(t *testing.T, stdout string) bindingResolution {
	t.Helper()
	var got bindingResolution
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout %q is not the result JSON: %v", stdout, err)
	}
	return got
}

// readWitness decodes the on-disk witness payload.
func readWitness(t *testing.T, path string) witnessPayload {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read witness %s: %v", path, err)
	}
	var got witnessPayload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode witness %s: %v", path, err)
	}
	return got
}

// assertNoTempResidue requires the atomic write to leave no temporary sibling.
func assertNoTempResidue(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp.") {
			t.Fatalf("temp residue %q left in %s", entry.Name(), dir)
		}
	}
}
