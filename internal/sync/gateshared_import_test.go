package sync

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/worktree-gate/shared"
)

// This file is the SX0a in-process import evidence: the root single binary
// reaches the gate's authoritative orphans decision through the importable
// shared package (ai-specs.dev/worktree-gate/shared) instead of shelling out
// to the gate binary. The go.mod require/replace wiring makes the nested
// gate module resolvable without a published release.

// TestSharedOrphansPlanContract pins the plan envelope contract from the
// root side: same decision, same JSON field names, lists never null.
func TestSharedOrphansPlanContract(t *testing.T) {
	in := shared.OrphanPlanInput{
		RecipeSkills:     []string{"c", "a", "b", "a"},
		DepsSkills:       []string{"z", "m", "m"},
		InprojectDeps:    []string{"q", "n"},
		LockRecipes:      []string{"r", "d"},
		EnabledRecipeIDs: nil,
		ExpectedDepIDs:   nil,
	}
	got := shared.PlanOrphans(in)
	want := shared.OrphanPlan{
		OrphanedRecipes:       []string{"a", "b", "c"},
		OrphanedDeps:          []string{"m", "z"},
		OrphanedInprojectDeps: []string{"n", "q"},
		StaleLockRecipes:      []string{"d", "r"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PlanOrphans() = %#v, want %#v", got, want)
	}
}

// TestSharedRunPlanOrphansEnvelope drives the full command surface
// in-process: stdin envelope in, one JSON object out, exit 0/2 semantics.
func TestSharedRunPlanOrphansEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	stdin := `{"recipe_skills":["a","b"],"enabled_recipe_ids":["a"]}`
	code := shared.RunPlanOrphans(strings.NewReader(stdin), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunPlanOrphans exit = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("RunPlanOrphans stderr = %q, want empty", stderr.String())
	}
	out := stdout.String()
	for _, field := range []string{
		"orphaned_recipes", "orphaned_deps",
		"orphaned_inproject_deps", "stale_lock_recipes",
	} {
		if !strings.Contains(out, `"`+field+`":`) {
			t.Fatalf("RunPlanOrphans stdout = %q, want field %q", out, field)
		}
	}
	if strings.Contains(out, "null") {
		t.Fatalf("RunPlanOrphans emitted a null list: %q", out)
	}
}

// TestSharedRunPlanOrphansMalformedInput pins the process-level failure
// contract: malformed JSON is exit 2 with a diagnostic on stderr and an
// empty stdout.
func TestSharedRunPlanOrphansMalformedInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := shared.RunPlanOrphans(strings.NewReader("{not json"), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("RunPlanOrphans malformed input exit = %d, want 2", code)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty on malformed input", stdout.String())
	}
	if !strings.Contains(stderr.String(), "plan-orphans") {
		t.Fatalf("stderr = %q, want a plan-orphans diagnostic", stderr.String())
	}
}

// TestSharedSortedUniqueSemantics pins the shared helper the root side now
// consumes: sorted, deduplicated, stable order.
func TestSharedSortedUniqueSemantics(t *testing.T) {
	got := shared.SortedUnique([]string{"b", "a", "a", "c"})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SortedUnique = %#v, want %#v", got, want)
	}
	if got := shared.SortedUnique(nil); got == nil || len(got) != 0 {
		t.Fatalf("SortedUnique(nil) = %#v, want non-nil empty slice", got)
	}
}

// TestSharedRunWriteLockContract drives the lock-writer command surface
// in-process (SX0b): one JSON envelope on stdin, {"written":true} on stdout,
// exit 0, byte-exact emitted lock, mode 0600 (mkstemp parity).
func TestSharedRunWriteLockContract(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), ".ai-specs.lock")
	envelope := `{"lock_path": "` + lockPath + `", "meta": {"cli_version": "0.24.0", "synced_at": "2026-07-14T00:00:00Z"}, "managed": {"AGENTS.md": {"sha256": "abc", "recipe": "worktree-flow", "source": "tpl.md", "kind": "template", "policy": "auto"}}, "agents": {"claude": {"AGENTS.md": "agenthash"}}}`
	var stdout, stderr bytes.Buffer
	code := shared.RunWriteLock(strings.NewReader(envelope), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunWriteLock exit = %d, want 0; stderr: %s; stdout: %s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), `"written"`) {
		t.Fatalf("RunWriteLock stdout = %q, want a written envelope", stdout.String())
	}
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("lock file was not written: %v", err)
	}
	want := shared.LockHeader +
		"\n[meta]\n" +
		"cli_version = \"0.24.0\"\n" +
		"synced_at = \"2026-07-14T00:00:00Z\"\n" +
		"\n[managed.\"AGENTS.md\"]\n" +
		"sha256 = \"abc\"\n" +
		"recipe = \"worktree-flow\"\n" +
		"source = \"tpl.md\"\n" +
		"kind = \"template\"\n" +
		"policy = \"auto\"\n" +
		"\n[agents.\"claude\"]\n" +
		"\"AGENTS.md\" = \"agenthash\"\n"
	if string(data) != want {
		t.Fatalf("emitted lock bytes diverge\n--- got ---\n%q\n--- want ---\n%q", data, want)
	}
	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("stat lock: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("lock mode = %o, want 600 (tempfile.mkstemp parity)", got)
	}
}

// TestSharedRunWriteLockRefusesControlCharacters pins the envelope-boundary
// refusal from the root side: a control character in any key or value is a
// structured {"error": ...} refusal with exit 2 and no bytes written.
func TestSharedRunWriteLockRefusesControlCharacters(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), ".ai-specs.lock")
	envelope := `{"lock_path": "` + lockPath + `", "meta": {"cli_version": "0.1\n0"}}`
	var stdout, stderr bytes.Buffer
	code := shared.RunWriteLock(strings.NewReader(envelope), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("RunWriteLock exit = %d, want 2", code)
	}
	if !strings.Contains(stdout.String(), "contains a control character") {
		t.Fatalf("stdout = %q, want a control-character refusal", stdout.String())
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("a refused envelope must not write the lock")
	}
}

// TestSharedPyJSONStringSemantics pins the relocated JSON string helper the
// root side now consumes: CPython json.dumps(ensure_ascii=True) semantics —
// named escapes, lowercase 4-digit hex for control chars and >=0x7f,
// surrogate pairs above the BMP.
func TestSharedPyJSONStringSemantics(t *testing.T) {
	cases := []struct{ in, want string }{
		{`plain`, `"plain"`},
		{"a\nb", `"a\nb"`},
		{"café", `"caf\u00e9"`},
		{"\x7f", `"\u007f"`},
		{"€", `"\u20ac"`},
	}
	for _, tc := range cases {
		if got := shared.PyJSONString(tc.in); got != tc.want {
			t.Errorf("PyJSONString(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
