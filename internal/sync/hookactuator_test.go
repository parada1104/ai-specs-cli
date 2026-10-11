package sync

// T2 contract tests for the native root hook actuation authority (GO-07 S12.3
// delivery B).
//
// MaterializeHook routes the root request into the shared actuator core
// (ai-specs.dev/worktree-gate/shared.MaterializeHook) the gate binary's
// --materialize-hook surface also runs, so the root single binary and the gate
// share one decision + execution shell — never a second port. It mirrors
// Python's hook authority: the Go-bridge dispatcher materialize_hook_script
// (lib/_internal/recipe-materialize.py:2085) and the historical body it falls
// back to, _python_materialize_hook_script (:2233). Every call returns the
// gate-baseline record payload for the caller; S14 owns the lock write and the
// caller diagnostics.
//
// T4's Python-oracle differential lives at the bottom of this file.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/worktree-gate/shared"
)

const (
	hkRel    = "ai-specs/recipes/worktree-flow/hooks/gate.sh"
	hkScript = "hooks/gate.sh"
)

// hkHookSource carries all eight hook placeholders, so one render exercises
// every substitution arm. The fixture config omits gate_mode, so the two
// gate-mode tokens take their historical defaults ("always"/"warn"); the cli
// home is empty, so the tracker bridge tokens render empty (fail open) and the
// gate version is the Python-resolved "dev" the envelope carries.
const hkHookSource = "#!/bin/sh\n" +
	"# mode=__WORKTREE_GATE_MODE__\n" +
	"# card=__TRACKER_CARD_GATE_MODE__\n" +
	"# scope=__WORKTREE_GATE_SCOPE__\n" +
	"# topology=__WORKTREE_REPO_TOPOLOGY__\n" +
	"# impl=__WORKTREE_GATE_IMPL__\n" +
	"# version=__WORKTREE_GATE_VERSION__\n" +
	"# home=__TRACKER_CLI_HOME__\n" +
	"# internal=__TRACKER_LIB_INTERNAL__\n"

// hkRender reproduces the render for the fixture config/version/home so the
// expected bytes stay derived from the source instead of duplicated by hand.
func hkRender(body string) string {
	return strings.NewReplacer(
		"__WORKTREE_GATE_MODE__", "always",
		"__TRACKER_CARD_GATE_MODE__", "warn",
		"__WORKTREE_GATE_SCOPE__", "auto",
		"__WORKTREE_REPO_TOPOLOGY__", "auto",
		"__WORKTREE_GATE_IMPL__", "auto",
		"__WORKTREE_GATE_VERSION__", "dev",
		"__TRACKER_CLI_HOME__", "",
		"__TRACKER_LIB_INTERNAL__", "",
	).Replace(body)
}

// hkRecord is the gate-baseline payload the caller hands to the lock:
// target is the project-relative rel path, kind "gate", policy "auto".
func hkRecord(sc hkScenario, sha string) map[string]string {
	return map[string]string{
		"target": sc.Rel, "sha256": sha, "recipe": sc.RecipeID,
		"source": sc.Script, "kind": "gate", "policy": "auto",
	}
}

// Exact Python reference warn strings (recipe-materialize.py :2355, :2362).
func hkWarnNoProvenance(rel string) string {
	return fmt.Sprintf("hook %s has no recorded provenance; preserving existing bytes. "+
		"A baseline is recorded only when the CLI renders the gate. Refresh with:\n"+
		"  rm %s && ai-specs sync  (or: ai-specs sync --refresh-gates)", rel, rel)
}
func hkWarnUserModified(rel string) string {
	return fmt.Sprintf("hook %s is user-modified; preserving existing bytes. Refresh with:\n"+
		"  rm %s && ai-specs sync  (or: ai-specs sync --refresh-gates)", rel, rel)
}

// hkOutcome is the scenario outcome both the native side and the Python oracle
// produce; it is path-independent (relative target, projected temp roots,
// content hashes) so the two fixtures can live under different temp roots.
type hkOutcome struct {
	Wrote     bool              `json:"wrote"`
	Dest      string            `json:"dest"`
	Record    map[string]string `json:"record"`
	DiskSHA   string            `json:"disk_sha"`
	Mode      string            `json:"mode"`
	Backup    string            `json:"backup"`
	BackupSHA string            `json:"backup_sha"`
	Lines     []string          `json:"lines"`
	Refused   bool              `json:"refused"`
}

// hkScenario is one hook actuation scenario. The json-tagged fields go to the
// Python oracle; the unexported fields drive the native fixture.
type hkScenario struct {
	name        string
	Rel         string         `json:"rel"`
	ProjectRoot string         `json:"project_root"`
	RecipeDir   string         `json:"recipe_dir"`
	RecipeID    string         `json:"recipe_id"`
	Script      string         `json:"script"`
	Config      map[string]any `json:"config,omitempty"`
	Refresh     bool           `json:"refresh,omitempty"`
	ManagedSHA  string         `json:"managed_sha,omitempty"`
	BackupPath  string         `json:"backup_path,omitempty"`

	want     *hkOutcome
	destSeed []byte
	destMode os.FileMode
	destKind string // "", "symlink" (dangling)
}

func (sc *hkScenario) destPath() string {
	return filepath.Join(sc.ProjectRoot, filepath.FromSlash(sc.Rel))
}

// prepare seeds the fixture on disk: the pre-existing destination for the
// classification scenarios, the dangling symlink for the never-written guard,
// and the read-only destination for the rollback scenario.
func (sc *hkScenario) prepare(t *testing.T) {
	t.Helper()
	dest := sc.destPath()
	if sc.destKind == "symlink" {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			t.Fatal(err)
		}
		// Defensive: each guard scenario owns a distinct rel path, so this
		// normally creates the link; an already-created dangling link is reused
		// rather than re-created.
		if info, lstatErr := os.Lstat(dest); lstatErr == nil {
			if info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("symlink scenario destination already exists as a non-symlink: %v", info.Mode())
			}
			return
		}
		if err := os.Symlink(filepath.Join(sc.ProjectRoot, "..", "dangling-hook-target"), dest); err != nil {
			t.Fatal(err)
		}
		return
	}
	if sc.destSeed == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	mode := sc.destMode
	if mode == 0 {
		mode = 0o644
	}
	if err := os.WriteFile(dest, sc.destSeed, mode); err != nil {
		t.Fatal(err)
	}
	// Neutralize the process umask so the fixture mode is exactly what the
	// scenario declares (the rollback case needs a strictly read-only file).
	if err := os.Chmod(dest, mode); err != nil {
		t.Fatal(err)
	}
}

// run executes one scenario through the native authority and projects the
// outcome into hkOutcome. The destination bytes are read at the authority's
// own destination (res.Dest) when it reports one.
func (sc *hkScenario) run(t *testing.T) hkOutcome {
	t.Helper()
	sc.prepare(t)
	base := filepath.Dir(sc.ProjectRoot)
	dest := sc.destPath()

	req := &HookRequest{
		ProjectRoot: sc.ProjectRoot, RecipeDir: sc.RecipeDir, RecipeID: sc.RecipeID,
		Script: sc.Script, Config: sc.Config, GateVersion: "dev",
		Refresh: sc.Refresh, BackupPath: sc.BackupPath,
	}
	if sc.ManagedSHA != "" {
		req.ManagedEntry = &shared.ClassifyManagedEntry{SHA256: sc.ManagedSHA}
	}

	out := hkOutcome{Lines: []string{}}
	res, err := MaterializeHook(req)
	if err != nil {
		out.Refused = true
	} else {
		out.Wrote = res.Wrote
		out.Dest = res.Dest
		if res.Dest != "" {
			dest = res.Dest
		}
		out.Lines = hkLines(res.Warnings, res.Message)
		out.Backup = res.Backup
		if res.Record != nil {
			out.Record = map[string]string{
				"target": res.Record.Target, "sha256": res.Record.SHA256,
				"recipe": res.Record.Recipe, "source": res.Record.Source,
				"kind": res.Record.Kind, "policy": res.Record.Policy,
			}
		}
	}

	out.Dest = tdProjectText(base, out.Dest)
	out.Backup = tdProjectText(base, out.Backup)
	for i := range out.Lines {
		out.Lines[i] = tdProjectText(base, out.Lines[i])
	}
	if out.Record != nil {
		out.Record["target"] = tdProjectText(base, out.Record["target"])
		out.Record["source"] = tdProjectText(base, out.Record["source"])
	}
	// Hash the file at the AUTHORITY's own destination (not a recomputed
	// join) so a write to the wrong path cannot pass. The symlink target is
	// dangling in the guard scenarios: no regular file, no hash, no mode.
	if data := tdReadRegular(dest); data != nil {
		out.DiskSHA = shared.Sha256Bytes(data)
	}
	if info, statErr := os.Stat(dest); statErr == nil {
		out.Mode = fmt.Sprintf("%o", info.Mode().Perm())
	}
	if out.Backup != "" {
		if data := tdReadRegular(sc.BackupPath); data != nil {
			out.BackupSHA = shared.Sha256Bytes(data)
		}
	}
	return out
}

// hkLines projects an actuator result onto the emission order Python uses:
// warnings, then the detail line.
func hkLines(warnings []string, message string) []string {
	lines := []string{}
	for _, warning := range warnings {
		lines = append(lines, "warn:"+warning)
	}
	if message != "" {
		lines = append(lines, "msg:"+message)
	}
	return lines
}

// hkBuildScenarios creates the fixture files under base and returns the hook
// scenarios with their expected outcomes. Both sides of the differential call
// it with their own base; identical content makes the outcomes comparable.
func hkBuildScenarios(t *testing.T, base string) []hkScenario {
	t.Helper()
	recipeDir := filepath.Join(base, "catalog", "worktree-flow")
	projectRoot := filepath.Join(base, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	rendered := hkRender(hkHookSource)
	sha := func(s string) string { return shared.Sha256Bytes([]byte(s)) }
	const (
		local = "#!/bin/sh\n# local customization\n"
		stale = "#!/bin/sh\n# mode=warn\n"
		prior = "#!/bin/sh\n# user customization\n"
		other = "#!/bin/sh\n# other baseline\n"
	)
	seed := func(s string) []byte {
		if s == "" {
			return nil
		}
		return []byte(s)
	}
	cfg := map[string]any{"gate_scope": "auto", "repo_topology": "auto", "gate_impl": "auto"}
	backup := func(content string) string {
		return filepath.Join(base, "cache", "backups", "gate", sha(content)+".sh")
	}

	out := []hkScenario{}
	// Each scenario owns a distinct script basename (and therefore a distinct
	// rel path and destination), so a state seeded by one scenario can never
	// leak into the next.
	mk := func(name, seedStr, managedSHA string, refresh bool, backupContent, kind string) hkScenario {
		sc := hkScenario{
			name: name, Rel: "ai-specs/recipes/worktree-flow/hooks/" + name + ".sh",
			Script: "hooks/" + name + ".sh", RecipeID: "worktree-flow",
			ProjectRoot: projectRoot, RecipeDir: recipeDir, Config: cfg,
			ManagedSHA: managedSHA, Refresh: refresh,
			destSeed: seed(seedStr), destKind: kind,
		}
		writeTDFile(t, filepath.Join(recipeDir, filepath.FromSlash(sc.Script)), hkHookSource, 0o644)
		if backupContent != "" {
			sc.BackupPath = backup(backupContent)
		}
		return sc
	}
	add := func(sc hkScenario, want hkOutcome) {
		sc.want = &want
		out = append(out, sc)
	}
	empty := []string{}

	// h1: fresh write into a missing destination: rendered bytes, mode 0755
	// (the source mode is never copied), and the gate record payload.
	h1 := mk("h1-fresh-write", "", "", false, "", "")
	add(h1, hkOutcome{
		Wrote: true, Record: hkRecord(h1, sha(rendered)), DiskSHA: sha(rendered),
		Mode: "755", Lines: []string{"msg:✓ hook script " + h1.Rel},
	})
	// h2: managed_current — bytes match both baseline and render, so the pair
	// backfills provenance without a rewrite.
	h2 := mk("h2-managed-current", rendered, sha(rendered), false, "", "")
	add(h2, hkOutcome{
		Record: hkRecord(h2, sha(rendered)), DiskSHA: sha(rendered), Mode: "644",
		Lines: []string{"msg:· hook skipped (current) " + h2.Rel},
	})
	// h3: no provenance — an existing destination without a baseline is NEVER
	// seeded (unlike the template actuator): preserve with the exact warning.
	h3 := mk("h3-no-provenance", rendered, "", false, "", "")
	add(h3, hkOutcome{
		DiskSHA: sha(rendered), Mode: "644",
		Lines: []string{"warn:" + hkWarnNoProvenance(h3.Rel), "msg:· hook skipped (no provenance) " + h3.Rel},
	})
	// h4: user-modified — the baseline does not match the disk bytes.
	h4 := mk("h4-user-modified", local, sha(other), false, "", "")
	add(h4, hkOutcome{
		DiskSHA: sha(local), Mode: "644",
		Lines: []string{"warn:" + hkWarnUserModified(h4.Rel), "msg:· hook skipped (user-modified) " + h4.Rel},
	})
	// h5: managed_stale on an ordinary sync — the baseline matches the disk
	// bytes, so the CLI force-updates the gate and re-records the baseline.
	h5 := mk("h5-managed-stale-refresh", stale, sha(stale), false, "", "")
	add(h5, hkOutcome{
		Wrote: true, Record: hkRecord(h5, sha(rendered)), DiskSHA: sha(rendered), Mode: "755",
		Lines: []string{"msg:✓ hook refreshed (baseline matched) " + h5.Rel},
	})
	// h6: --refresh-gates — the exact prior bytes land in the immutable
	// content-hash backup before the gate is rewritten.
	sc6 := mk("h6-refresh-backup", prior, "", true, prior, "")
	add(sc6, hkOutcome{
		Wrote: true, Record: hkRecord(sc6, sha(rendered)), DiskSHA: sha(rendered), Mode: "755",
		Backup: sc6.BackupPath, BackupSHA: sha(prior),
		Lines: []string{"msg:✓ hook refreshed " + sc6.Rel},
	})
	// h7: a refresh that cannot complete the write is all-or-nothing: the
	// prior bytes survive, the backup it created is removed, and no record is
	// returned. A root euid cannot be denied by file mode, so it is skipped.
	if os.Geteuid() != 0 {
		sc7 := mk("h7-refresh-rollback", prior, "", true, prior, "")
		sc7.destMode = 0o400
		add(sc7, hkOutcome{Refused: true, DiskSHA: sha(prior), Mode: "400", Lines: empty})
	}
	// h8: a dangling symlink destination is refused, never written through.
	add(mk("h8-symlink-guard", "", "", false, "", "symlink"), hkOutcome{Refused: true, Lines: empty})
	// h9: the refresh path refuses a symlinked destination before any read of
	// the prior bytes or any backup write.
	add(mk("h9-refresh-symlink-guard", "", "", true, prior, "symlink"), hkOutcome{Refused: true, Lines: empty})

	return out
}

// TestMaterializeHookStateParity pins every classification branch against the
// exact Python outcomes: fresh write, managed_current backfill, no-provenance
// preserve, user-modified preserve, managed-stale refresh, refresh backup,
// refresh rollback, and both symlink refusals.
func TestMaterializeHookStateParity(t *testing.T) {
	for _, sc := range hkBuildScenarios(t, t.TempDir()) {
		t.Run(sc.name, func(t *testing.T) {
			got := sc.run(t)
			want := *sc.want
			// Dest and Backup identity are asserted against the real Python
			// authority by TestMaterializeHookPythonDifferential; this
			// fixture-only check compares the decision fields and bytes.
			got.Dest, want.Dest = "", ""
			got.Backup, want.Backup = "", ""
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("outcome =\n%#v\nwant\n%#v", got, want)
			}
		})
	}
}

// TestMaterializeHookRefusalsAndBoundary pins the exact refusal strings and
// the record-only boundary: the authority writes no lock or manifest. The
// rel-path traversal guard is a Go hardening (R1-hook-rel-path-unsanitized):
// the Python body has no equivalent check, so it is asserted natively here,
// not in the differential.
func TestMaterializeHookRefusalsAndBoundary(t *testing.T) {
	newFixture := func(t *testing.T) (projectRoot, recipeDir string) {
		base := t.TempDir()
		recipeDir = filepath.Join(base, "catalog", "worktree-flow")
		projectRoot = filepath.Join(base, "project")
		writeTDFile(t, filepath.Join(recipeDir, "hooks", "gate.sh"), hkHookSource, 0o644)
		if err := os.MkdirAll(projectRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		return projectRoot, recipeDir
	}
	t.Run("source missing", func(t *testing.T) {
		projectRoot, recipeDir := newFixture(t)
		_, err := MaterializeHook(&HookRequest{
			ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: "worktree-flow",
			Script: "hooks/missing.sh",
		})
		want := "hook script not found: " + filepath.Join(recipeDir, "hooks", "missing.sh")
		if err == nil || err.Error() != want {
			t.Fatalf("error = %v, want %q", err, want)
		}
	})
	t.Run("escaping rel path", func(t *testing.T) {
		for _, tc := range []struct{ recipeID, script, want string }{
			{"../escape", hkScript, `hook rel path would escape ai-specs/recipes/: recipe_id "../escape", script "hooks/gate.sh"`},
			{"sub/dir", hkScript, `hook rel path would escape ai-specs/recipes/: recipe_id "sub/dir", script "hooks/gate.sh"`},
			{"worktree-flow", "sub/..", `hook rel path would escape ai-specs/recipes/: recipe_id "worktree-flow", script "sub/.."`},
		} {
			projectRoot, recipeDir := newFixture(t)
			_, err := MaterializeHook(&HookRequest{
				ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: tc.recipeID,
				Script: tc.script, GateVersion: "dev",
			})
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			// The guard fires before any read or write: nothing may appear
			// under a traversal recipe id.
			escaped := filepath.Join(projectRoot, "ai-specs", "recipes", tc.recipeID)
			if _, statErr := os.Stat(escaped); !os.IsNotExist(statErr) {
				t.Fatalf("escaped destination %s created despite refusal: %v", escaped, statErr)
			}
		}
	})
	t.Run("symlink never written through", func(t *testing.T) {
		projectRoot, recipeDir := newFixture(t)
		dest := filepath.Join(projectRoot, filepath.FromSlash(hkRel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(projectRoot, "bystander.sh")
		if err := os.Symlink(target, dest); err != nil {
			t.Fatal(err)
		}
		want := "destination " + hkRel + " is a symlink; refusing to write through it. " +
			"Replace it with a regular file and run sync again:\n  rm " + hkRel + " && ai-specs sync"
		for _, refresh := range []bool{false, true} {
			_, err := MaterializeHook(&HookRequest{
				ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: "worktree-flow",
				Script: hkScript, GateVersion: "dev", Refresh: refresh,
				BackupPath: filepath.Join(t.TempDir(), "backup.sh"),
			})
			if err == nil || err.Error() != want {
				t.Fatalf("refresh=%v error = %v, want %q", refresh, err, want)
			}
			if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
				t.Fatalf("the symlink target was written despite the guard: %v", statErr)
			}
			if info, lstatErr := os.Lstat(dest); lstatErr != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("the destination symlink was replaced: %v, %v", info, lstatErr)
			}
		}
	})
	t.Run("no lock or manifest write", func(t *testing.T) {
		projectRoot, recipeDir := newFixture(t)
		if _, err := MaterializeHook(&HookRequest{
			ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: "worktree-flow",
			Script: hkScript, GateVersion: "dev",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(projectRoot, "ai-specs", ".ai-specs.lock")); !os.IsNotExist(err) {
			t.Fatalf("the authority must not write a lock (stat err = %v)", err)
		}
	})
}

// TestMaterializeHookRefreshRollback pins the root-level all-or-nothing
// refresh: a write that cannot complete restores the prior bytes, removes the
// backup it created, and returns a refusal naming the destination. The
// differential also covers this scenario against Python; this test pins the
// native refusal shape independently of the oracle.
func TestMaterializeHookRefreshRollback(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("write refusal via file mode does not work as root")
	}
	base := t.TempDir()
	recipeDir := filepath.Join(base, "catalog", "worktree-flow")
	projectRoot := filepath.Join(base, "project")
	writeTDFile(t, filepath.Join(recipeDir, "hooks", "gate.sh"), hkHookSource, 0o644)
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(projectRoot, filepath.FromSlash(hkRel))
	prior := "#!/bin/sh\n# user customization\n"
	writeTDFile(t, dest, prior, 0o400)
	backupPath := filepath.Join(base, "cache", "backups", "gate", shared.Sha256Bytes([]byte(prior))+".sh")

	_, err := MaterializeHook(&HookRequest{
		ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: "worktree-flow",
		Script: hkScript, GateVersion: "dev", Refresh: true, BackupPath: backupPath,
	})
	if err == nil {
		t.Fatal("refresh into a read-only destination must refuse")
	}
	if !strings.Contains(err.Error(), dest) {
		t.Fatalf("error = %q, want a write-failure message naming %s", err, dest)
	}
	if _, statErr := os.Stat(backupPath); !os.IsNotExist(statErr) {
		t.Fatalf("backup still exists after rollback: %v", statErr)
	}
	if got, readErr := os.ReadFile(dest); readErr != nil || string(got) != prior {
		t.Fatalf("dest bytes = %q (%v), want restored prior %q", got, readErr, prior)
	}
}

// ---------------------------------------------------------------------------
// T4: Python-oracle differential
// ---------------------------------------------------------------------------

// hkOracle drives the REAL Python hook authority — the historical body
// _python_materialize_hook_script that the Go-bridge dispatcher falls back to
// (recipe-materialize.py:2233). It patches only the lock and print sinks:
// load_lock returns the scenario's prepared lock, write_lock captures it (after
// the real set_gate_baseline upserts it), warn records the exact warning
// strings, and the project-cache stub returns the harness-computed immutable
// backup path so no real CLI cache is touched. The outcome is path-independent
// and byte-comparable to the native hkOutcome.
const hkOracle = `
import contextlib, copy, hashlib, importlib.util, io, json, stat, sys
from pathlib import Path
from types import SimpleNamespace

spec = importlib.util.spec_from_file_location("hook_outcome_oracle", sys.argv[1])
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)

scenarios = json.loads(Path(sys.argv[2]).read_text())

def sha(data):
    return hashlib.sha256(data.replace(b"\r\n", b"\n")).hexdigest()

class CacheStub:
    def __init__(self, path):
        self.path = path
    def gate_backup_path(self, project_root, rel, digest, cli_home=None):
        return Path(self.path)

results = []
for sc in scenarios:
    project_root = Path(sc["project_root"])
    recipe_dir = Path(sc["recipe_dir"])
    base = str(project_root.parent)

    def project(s):
        return s.replace("//" + base.lstrip("/"), "<//BASE>").replace(base, "<BASE>")

    dest = project_root / sc["rel"]
    before = dest.read_bytes() if dest.is_file() else None
    key = sc["rel"]
    lock = {"managed": {}}
    if sc.get("managed_sha"):
        lock["managed"][key] = {"sha256": sc["managed_sha"]}
    captured, lines = {}, []
    mod.load_lock = lambda p, _l=lock: _l
    mod.write_lock = lambda p, l, _c=captured: _c.__setitem__("lock", copy.deepcopy(l))
    mod.warn = lambda msg, _lines=lines: _lines.append("warn:" + msg)
    mod.info = lambda msg, _lines=lines: _lines.append("info:" + msg)
    mod._load_project_cache = lambda _p=sc.get("backup_path", ""): CacheStub(_p)
    backup = Path(sc["backup_path"]) if sc.get("backup_path") else None
    stdout, error = io.StringIO(), None
    try:
        with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(io.StringIO()):
            hook = SimpleNamespace(script=sc["script"])
            mod._python_materialize_hook_script(
                recipe_dir, hook, project_root, sc["recipe_id"],
                sc.get("config"), None, sc.get("refresh", False),
            )
    except Exception as exc:  # noqa: BLE001
        error = str(exc)
    for line in stdout.getvalue().splitlines():
        if line.startswith("    "):
            lines.append("msg:" + line[4:])
    after = dest.read_bytes() if dest.is_file() else None
    entry = (captured.get("lock") or {}).get("managed", {}).get(key)
    if entry is not None:
        entry = {"target": key, **entry}
    backup_sha = ""
    backup_present = backup is not None and backup.is_file()
    if backup_present:
        backup_sha = sha(backup.read_bytes())
    results.append({
        "wrote": before != after,
        "dest": project(str(dest)) if error is None else "",
        "record": entry,
        "disk_sha": sha(after) if after is not None else "",
        "mode": format(stat.S_IMODE(dest.stat().st_mode), "o") if dest.exists() else "",
        "backup": project(str(backup)) if backup_present else "",
        "backup_sha": backup_sha,
        "lines": [project(line) for line in lines],
        "refused": error is not None,
    })
print(json.dumps(results, sort_keys=True))
`

// TestMaterializeHookPythonDifferential compares the native outcomes against
// the real Python hook authority over the full state matrix. Both sides start
// from byte-identical fixtures under different roots and must agree on wrote,
// destination identity, record fields, bytes and mode on disk, the refresh
// backup, every emitted line, and whether the call refused.
func TestMaterializeHookPythonDifferential(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := filepath.Join(t.TempDir(), "hk_oracle.py")
	if err := os.WriteFile(driver, []byte(hkOracle), 0o600); err != nil {
		t.Fatal(err)
	}

	nativeBase := t.TempDir()
	pythonBase := t.TempDir()
	nativeScenarios := hkBuildScenarios(t, nativeBase)
	pythonScenarios := hkBuildScenarios(t, pythonBase)
	if len(nativeScenarios) == 0 || len(nativeScenarios) != len(pythonScenarios) {
		t.Fatalf("scenario builders disagree: %d vs %d", len(nativeScenarios), len(pythonScenarios))
	}

	got := make([]hkOutcome, 0, len(nativeScenarios))
	for i := range nativeScenarios {
		got = append(got, nativeScenarios[i].run(t))
	}
	for i := range pythonScenarios {
		pythonScenarios[i].prepare(t)
	}

	specPath := filepath.Join(t.TempDir(), "scenarios.json")
	spec, err := json.Marshal(pythonScenarios)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, spec, 0o600); err != nil {
		t.Fatal(err)
	}
	raw := runSyncPythonOracle(t, python, driver, authority, specPath)
	var want []hkOutcome
	if err := json.Unmarshal([]byte(raw), &want); err != nil {
		t.Fatalf("parse oracle JSON: %v\nraw: %s", err, raw)
	}
	if len(want) != len(got) {
		t.Fatalf("oracle returned %d outcome(s), want %d", len(want), len(got))
	}

	// Non-vacuity: both sides really wrote, recorded, warned, refused and
	// snapshotted a refresh backup.
	for _, marker := range []struct {
		name string
		any  func([]hkOutcome) bool
	}{
		{"a write", func(os []hkOutcome) bool { return tdCountHK(os, func(o hkOutcome) bool { return o.Wrote }) > 0 }},
		{"a record", func(os []hkOutcome) bool { return tdCountHK(os, func(o hkOutcome) bool { return o.Record != nil }) > 0 }},
		{"a warning", func(os []hkOutcome) bool {
			return tdCountHK(os, func(o hkOutcome) bool { return tdHasPrefix(o.Lines, "warn:") }) > 0
		}},
		{"a refusal", func(os []hkOutcome) bool { return tdCountHK(os, func(o hkOutcome) bool { return o.Refused }) > 0 }},
		{"a refresh backup", func(os []hkOutcome) bool {
			return tdCountHK(os, func(o hkOutcome) bool { return o.BackupSHA != "" }) > 0
		}},
	} {
		if !marker.any(want) {
			t.Fatalf("oracle outcomes are missing %s: %#v", marker.name, want)
		}
		if !marker.any(got) {
			t.Fatalf("native outcomes are missing %s: %#v", marker.name, got)
		}
	}

	for i := range got {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Fatalf("scenario %d (%s) diverges\n--- go ---\n%#v\n--- python ---\n%#v",
				i, nativeScenarios[i].name, got[i], want[i])
		}
	}
}

func tdCountHK(outcomes []hkOutcome, pred func(hkOutcome) bool) int {
	n := 0
	for _, o := range outcomes {
		if pred(o) {
			n++
		}
	}
	return n
}
