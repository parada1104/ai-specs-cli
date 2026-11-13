package sync

// T2 contract tests for the native root template + docs authority (GO-07 S12.2).
//
// MaterializeTemplate reuses the shared actuator core
// (ai-specs.dev/worktree-gate/shared.MaterializeTemplate) the gate binary also
// runs, so there is one decision shell, never a second port. MaterializeDoc is
// the native docs-flow policy shell over the S12.1 copy authority. Both mirror
// lib/_internal/recipe-materialize.py (_python_materialize_template :1430,
// materialize_doc :1551) and return the managed-override record payload for the
// caller — S14 owns the lock write.
//
// The frozen record of that authority lives at the bottom of this file and in
// testdata/templatedocs_golden.json.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/worktree-gate/shared"
)

const (
	tdTemplateTarget = "ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh"
	tdDocBody        = "doc body\n"
)

// Exact Python reference strings (recipe-materialize.py :1495-1530, :1583-1612).
func tdWarnUntracked(target string) string {
	return "override metadata missing for " + target + "; preserving existing file without assigning ownership. " +
		"To preserve this local file, leave it unchanged. To replace it with the current recipe version, " +
		"remove it and run sync again:\n  rm " + target + " && ai-specs sync"
}
func tdWarnUserModified(target string) string {
	return "override user-modified: " + target + " was not refreshed. Refresh with:\n  rm " + target + " && ai-specs sync"
}
func tdWarnStaleTemplate(policy, target string) string {
	return fmt.Sprintf("override managed-stale (%s-required): %s was not refreshed. Refresh with:\n  rm %s && ai-specs sync", policy, target, target)
}
func tdWarnStaleDoc(target string) string {
	return "override managed-stale (confirm-required): " + target + " was not refreshed. Refresh with:\n  rm " + target + " && ai-specs sync"
}

// tdOutcome is the scenario outcome both the native side and the Python oracle
// produce; it is path-independent (relative targets, content hashes) so the two
// fixtures can live under different temp roots.
type tdOutcome struct {
	Wrote   bool              `json:"wrote"`
	Dest    string            `json:"dest"`
	Record  map[string]string `json:"record"`
	DiskSHA string            `json:"disk_sha"`
	Lines   []string          `json:"lines"`
	Error   string            `json:"error"`
}

// tdScenario is one materialize scenario. The json-tagged fields go to the
// Python oracle; the unexported fields drive the native fixture.
type tdScenario struct {
	Kind         string         `json:"kind"`
	ProjectRoot  string         `json:"project_root"`
	RecipeDir    string         `json:"recipe_dir"`
	RecipeID     string         `json:"recipe_id"`
	Source       string         `json:"source"`
	Target       string         `json:"target"`
	Condition    string         `json:"condition,omitempty"`
	UpdatePolicy string         `json:"update_policy,omitempty"`
	Config       map[string]any `json:"config,omitempty"`
	ManagedEntry map[string]any `json:"managed_entry,omitempty"`

	want     *tdOutcome
	destSeed []byte
	destKind string // "", "dir", "symlink"
}

func (sc *tdScenario) destPath() string {
	return filepath.Join(sc.ProjectRoot, filepath.FromSlash(sc.Target))
}

func tdRecord(target, sha, source, kind, policy string) map[string]string {
	return map[string]string{
		"target": target, "sha256": sha, "recipe": "worktree-flow",
		"source": source, "kind": kind, "policy": policy,
	}
}

func tdReadRegular(path string) []byte {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

// tdProjectText replaces each side's temp root with a fixed token so outcomes
// from two different fixtures are comparable. The doubled-leading-slash form
// maps to a DISTINCT token, so the POSIX two-slash case stays honest. Applied
// to the destination, the emitted lines and the record source/target (an
// absolute doc target or source carries the temp root).
func tdProjectText(base, s string) string {
	s = strings.ReplaceAll(s, "//"+strings.TrimPrefix(base, "/"), "<//BASE>")
	return strings.ReplaceAll(s, base, "<BASE>")
}

func writeTDFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

// prepare seeds the fixture on disk: the pre-existing destination for the
// classification scenarios and the directory/symlink pre-existence cases.
func (sc *tdScenario) prepare(t *testing.T) {
	t.Helper()
	dest := sc.destPath()
	switch sc.destKind {
	case "dir":
		if err := os.MkdirAll(dest, 0o755); err != nil {
			t.Fatal(err)
		}
	case "symlink":
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(sc.ProjectRoot, "..", "linked-file"), dest); err != nil {
			t.Fatal(err)
		}
	case "dangling":
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(sc.ProjectRoot, "..", "dangling-target"), dest); err != nil {
			t.Fatal(err)
		}
	default:
		if sc.destSeed != nil {
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dest, sc.destSeed, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// run executes one scenario through the native authority and projects the
// outcome into tdOutcome.
func (sc *tdScenario) run(t *testing.T) tdOutcome {
	t.Helper()
	sc.prepare(t)
	base := filepath.Dir(sc.ProjectRoot)
	probeDest := sc.destPath()

	out := tdOutcome{DiskSHA: ""}
	entrySHA := ""
	if sc.ManagedEntry != nil {
		entrySHA, _ = sc.ManagedEntry["sha256"].(string)
	}

	if sc.Kind == "doc" {
		req := &DocRequest{
			ProjectRoot: sc.ProjectRoot, RecipeDir: sc.RecipeDir, RecipeID: sc.RecipeID,
			Source: sc.Source, Target: sc.Target,
		}
		if sc.ManagedEntry != nil {
			entry := &DocManagedEntry{SHA256: entrySHA}
			if policy, ok := sc.ManagedEntry["policy"]; ok {
				entry.HasPolicy = true
				entry.Policy, _ = policy.(string)
			}
			req.ManagedEntry = entry
		}
		res, err := MaterializeDoc(req)
		if err != nil {
			out.Error = err.Error()
		} else {
			out.Wrote = res.Wrote
			out.Dest = res.Dest
			probeDest = res.Dest
			out.Lines = tdLines(res.Warnings, res.Info, res.Message)
			if res.Record != nil {
				out.Record = map[string]string{
					"target": res.Record.Target, "sha256": res.Record.SHA256,
					"recipe": res.Record.Recipe, "source": res.Record.Source,
					"kind": res.Record.Kind, "policy": res.Record.Policy,
				}
			}
		}
	} else {
		req := &shared.TemplateRequest{
			ProjectRoot: sc.ProjectRoot, RecipeDir: sc.RecipeDir, RecipeID: sc.RecipeID,
			Source: sc.Source, Target: sc.Target, Condition: sc.Condition,
			UpdatePolicy: sc.UpdatePolicy, Config: sc.Config,
		}
		if sc.ManagedEntry != nil {
			req.ManagedEntry = &shared.ClassifyManagedEntry{SHA256: entrySHA}
		}
		res, err := MaterializeTemplate(req)
		if err != nil {
			out.Error = err.Error()
		} else {
			out.Wrote = res.Wrote
			out.Lines = tdLines(res.Warnings, res.Info, res.Message)
			if res.Record != nil {
				out.Record = map[string]string{
					"target": res.Record.Target, "sha256": res.Record.SHA256,
					"recipe": res.Record.Recipe, "source": res.Record.Source,
					"kind": res.Record.Kind, "policy": res.Record.Policy,
				}
			}
		}
	}
	// Project every path-bearing field with the SAME token map the oracle uses,
	// then hash the file at the AUTHORITY's own destination (not a recomputed
	// join) so a write to the wrong path cannot pass.
	out.Dest = tdProjectText(base, out.Dest)
	for i := range out.Lines {
		out.Lines[i] = tdProjectText(base, out.Lines[i])
	}
	if out.Record != nil {
		out.Record["target"] = tdProjectText(base, out.Record["target"])
		out.Record["source"] = tdProjectText(base, out.Record["source"])
	}
	if disk := tdReadRegular(probeDest); disk != nil {
		out.DiskSHA = shared.Sha256Bytes(disk)
	}
	return out
}

// tdLines projects an actuator result onto the emission order Python uses:
// warnings, then info, then the detail line.
func tdLines(warnings []string, info, message string) []string {
	lines := []string{}
	for _, warning := range warnings {
		lines = append(lines, "warn:"+warning)
	}
	if info != "" {
		lines = append(lines, "info:"+info)
	}
	if message != "" {
		lines = append(lines, "msg:"+message)
	}
	return lines
}

// buildTDScenarios creates the fixture files under base and returns the state
// scenarios with their expected outcomes. Both sides of the differential call
// it with their own base; identical content makes the outcomes comparable.
func buildTDScenarios(t *testing.T, base string) []tdScenario {
	t.Helper()
	recipeDir := filepath.Join(base, "catalog", "worktree-flow")
	writeTDFile(t, filepath.Join(recipeDir, "templates", "run.sh"), "mode=__WORKTREE_REPO_TOPOLOGY__\n", 0o755)
	writeTDFile(t, filepath.Join(recipeDir, "README.md"), tdDocBody, 0o644)
	writeTDFile(t, filepath.Join(base, "linked-file"), "linked\n", 0o644)
	projectRoot := filepath.Join(base, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	const tplSrc, docSrc, rid = "templates/run.sh", "README.md", "worktree-flow"
	rendered := "mode=auto\n"
	raw := "mode=__WORKTREE_REPO_TOPOLOGY__\n"
	old, local, other := "old\n", "local\n", "other\n"
	sha := func(s string) string { return shared.Sha256Bytes([]byte(s)) }
	tdSeed := func(s string) []byte {
		if s == "" {
			return nil
		}
		return []byte(s)
	}
	skip := func(target string) string { return "· template skipped (exists) " + target }
	skipDoc := func(target string) string { return "· doc skipped (exists) " + target }
	cfg := map[string]any{"repo_topology": "auto"}

	tpl := func(name string) string { return "ai-specs/recipes/worktree-flow/overrides/bin/" + name }
	doc := func(name string) string { return "ai-specs/recipes/worktree-flow/docs/" + name }

	out := []tdScenario{}
	// addDoc appends one standalone doc scenario. target defaults to the
	// canonical doc(name) spelling; a noncanonical raw target exercises the
	// Path.as_posix lock-key normalization while the diagnostics stay raw.
	addDoc := func(name, target, seed string, entry map[string]any, want tdOutcome) {
		if target == "" {
			target = doc(name)
		}
		out = append(out, tdScenario{
			Kind: "doc", ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: rid,
			Source: docSrc, Target: target, ManagedEntry: entry, want: &want, destSeed: tdSeed(seed),
		})
	}
	for _, tc := range []struct {
		name     string
		disk     string
		entry    string
		policy   string
		want     tdOutcome
		hasEntry bool
	}{
		{"t1.sh", "", "", "auto", tdOutcome{Wrote: true, Record: tdRecord(tpl("t1.sh"), sha(rendered), tplSrc, "template", "auto"), DiskSHA: sha(rendered), Lines: []string{"msg:✓ template " + tpl("t1.sh")}}, false},
		{"t2.sh", rendered, "", "auto", tdOutcome{Record: tdRecord(tpl("t2.sh"), sha(rendered), tplSrc, "template", "auto"), DiskSHA: sha(rendered), Lines: []string{"msg:" + skip(tpl("t2.sh"))}}, false},
		{"t3.sh", raw, "", "auto", tdOutcome{Record: tdRecord(tpl("t3.sh"), sha(raw), tplSrc, "template", "auto"), DiskSHA: sha(raw), Lines: []string{"msg:" + skip(tpl("t3.sh"))}}, false},
		{"t4.sh", local, "", "auto", tdOutcome{DiskSHA: sha(local), Lines: []string{"warn:" + tdWarnUntracked(tpl("t4.sh")), "msg:" + skip(tpl("t4.sh"))}}, false},
		{"t5.sh", rendered, sha(rendered), "auto", tdOutcome{Record: tdRecord(tpl("t5.sh"), sha(rendered), tplSrc, "template", "auto"), DiskSHA: sha(rendered), Lines: []string{"msg:" + skip(tpl("t5.sh"))}}, true},
		{"t6.sh", old, sha(old), "auto", tdOutcome{Wrote: true, Record: tdRecord(tpl("t6.sh"), sha(rendered), tplSrc, "template", "auto"), DiskSHA: sha(rendered), Lines: []string{"info:refreshed managed template " + tpl("t6.sh"), "msg:" + skip(tpl("t6.sh"))}}, true},
		{"t7.sh", old, sha(old), "confirm", tdOutcome{DiskSHA: sha(old), Lines: []string{"warn:" + tdWarnStaleTemplate("confirm", tpl("t7.sh")), "msg:" + skip(tpl("t7.sh"))}}, true},
		{"t8.sh", local, sha(other), "auto", tdOutcome{DiskSHA: sha(local), Lines: []string{"warn:" + tdWarnUserModified(tpl("t8.sh")), "msg:" + skip(tpl("t8.sh"))}}, true},
	} {
		sc := tdScenario{
			Kind: "template", ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: rid,
			Source: tplSrc, Target: tpl(tc.name), Condition: "not_exists", UpdatePolicy: tc.policy,
			Config: cfg, want: &tc.want, destSeed: tdSeed(tc.disk),
		}
		if tc.hasEntry {
			sc.ManagedEntry = map[string]any{"sha256": tc.entry}
		}
		out = append(out, sc)
	}

	for _, tc := range []struct {
		name     string
		disk     string
		entry    string
		policy   string
		hasEntry bool
		want     tdOutcome
	}{
		{"d1.md", "", "", "auto", false, tdOutcome{Wrote: true, Record: tdRecord(doc("d1.md"), sha(tdDocBody), docSrc, "doc", "auto"), DiskSHA: sha(tdDocBody), Lines: []string{"msg:✓ doc " + doc("d1.md")}}},
		{"d2.md", tdDocBody, "", "auto", false, tdOutcome{Record: tdRecord(doc("d2.md"), sha(tdDocBody), docSrc, "doc", "auto"), DiskSHA: sha(tdDocBody), Lines: []string{"msg:" + skipDoc(doc("d2.md"))}}},
		{"d3.md", local, "", "auto", false, tdOutcome{DiskSHA: sha(local), Lines: []string{"warn:" + tdWarnUntracked(doc("d3.md")), "msg:" + skipDoc(doc("d3.md"))}}},
		{"d4.md", tdDocBody, sha(tdDocBody), "auto", true, tdOutcome{Record: tdRecord(doc("d4.md"), sha(tdDocBody), docSrc, "doc", "auto"), DiskSHA: sha(tdDocBody), Lines: []string{"msg:" + skipDoc(doc("d4.md"))}}},
		{"d5.md", old, sha(old), "auto", true, tdOutcome{Wrote: true, Record: tdRecord(doc("d5.md"), sha(tdDocBody), docSrc, "doc", "auto"), DiskSHA: sha(tdDocBody), Lines: []string{"info:refreshed managed doc " + doc("d5.md"), "msg:" + skipDoc(doc("d5.md"))}}},
		{"d6.md", old, sha(old), "confirm", true, tdOutcome{DiskSHA: sha(old), Lines: []string{"warn:" + tdWarnStaleDoc(doc("d6.md")), "msg:" + skipDoc(doc("d6.md"))}}},
		{"d7.md", local, sha(other), "auto", true, tdOutcome{DiskSHA: sha(local), Lines: []string{"warn:" + tdWarnUserModified(doc("d7.md")), "msg:" + skipDoc(doc("d7.md"))}}},
	} {
		sc := tdScenario{
			Kind: "doc", ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: rid,
			Source: docSrc, Target: doc(tc.name), want: &tc.want, destSeed: tdSeed(tc.disk),
		}
		if tc.hasEntry {
			sc.ManagedEntry = map[string]any{"sha256": tc.entry, "policy": tc.policy}
		}
		out = append(out, sc)
	}

	dirTarget := doc("d8-dir")
	out = append(out, tdScenario{
		Kind: "doc", ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: rid,
		Source: docSrc, Target: dirTarget, destKind: "dir",
		want: &tdOutcome{Lines: []string{"warn:override metadata missing for " + dirTarget + "; preserving existing directory without assigning ownership."}},
	})
	linkTarget := doc("d9-link")
	out = append(out, tdScenario{
		Kind: "doc", ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: rid,
		Source: docSrc, Target: linkTarget, destKind: "symlink",
		want: &tdOutcome{DiskSHA: sha("linked\n"), Lines: []string{"warn:override metadata missing for " + linkTarget + "; preserving existing symlink without assigning ownership."}},
	})

	// d10: managed entry with an EXPLICIT empty policy. Python's
	// (entry or {}).get("policy", "auto") != "auto", so the doc is preserved
	// with the confirm-required warning. d11 is the control: the same entry
	// WITHOUT a policy key means auto, so the stale doc refreshes.
	addDoc("d10.md", "", old, map[string]any{"sha256": sha(old), "policy": ""},
		tdOutcome{DiskSHA: sha(old), Lines: []string{"warn:" + tdWarnStaleDoc(doc("d10.md")), "msg:" + skipDoc(doc("d10.md"))}})
	addDoc("d11.md", "", old, map[string]any{"sha256": sha(old)},
		tdOutcome{Wrote: true, Record: tdRecord(doc("d11.md"), sha(tdDocBody), docSrc, "doc", "auto"), DiskSHA: sha(tdDocBody), Lines: []string{"info:refreshed managed doc " + doc("d11.md"), "msg:" + skipDoc(doc("d11.md"))}})

	// d12-d14: noncanonical targets. The lock key is Path(target).as_posix()
	// (./ and // collapse, a leading separator is kept) while every diagnostic
	// and the copy id stay the RAW target. d14 keeps a '..' segment: as_posix
	// PRESERVES it, so a path.Clean-style normalizer would wrongly rewrite the
	// key and change which lock entry is read and written.
	d12 := "./" + doc("d12.md")
	addDoc("d12.md", d12, old, map[string]any{"sha256": sha(old), "policy": "auto"},
		tdOutcome{Wrote: true, Record: tdRecord(doc("d12.md"), sha(tdDocBody), docSrc, "doc", "auto"), DiskSHA: sha(tdDocBody), Lines: []string{"info:refreshed managed doc " + d12, "msg:" + skipDoc(d12)}})
	d13 := "ai-specs/recipes//worktree-flow/docs/d13.md"
	addDoc("d13.md", d13, old, map[string]any{"sha256": sha(old), "policy": "auto"},
		tdOutcome{Wrote: true, Record: tdRecord(doc("d13.md"), sha(tdDocBody), docSrc, "doc", "auto"), DiskSHA: sha(tdDocBody), Lines: []string{"info:refreshed managed doc " + d13, "msg:" + skipDoc(d13)}})
	d14 := "ai-specs/recipes/worktree-flow/docs/../docs/d14.md"
	addDoc("d14.md", d14, old, map[string]any{"sha256": sha(old), "policy": "auto"},
		tdOutcome{Wrote: true, Record: tdRecord(d14, sha(tdDocBody), docSrc, "doc", "auto"), DiskSHA: sha(tdDocBody), Lines: []string{"info:refreshed managed doc " + d14, "msg:" + skipDoc(d14)}})

	// d15: a DANGLING doc symlink. Plain Path.exists() is false, so Python
	// skips the symlink-preservation branch, copies THROUGH the link and
	// records provenance — the actual bytes and record, not just the line.
	danglingTarget := doc("d15-dangling")
	out = append(out, tdScenario{
		Kind: "doc", ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: rid,
		Source: docSrc, Target: danglingTarget, destKind: "dangling",
		want: &tdOutcome{Wrote: true, Record: tdRecord(danglingTarget, sha(tdDocBody), docSrc, "doc", "auto"), DiskSHA: sha(tdDocBody), Lines: []string{"msg:✓ doc " + danglingTarget}},
	})
	// t9: a noncanonical TEMPLATE target. The shared gate actuator keys the
	// record and the diagnostics on one spelling; the root wrapper must restore
	// Path(target).as_posix() for the record while every message keeps the raw
	// target, exactly like _python_materialize_template.
	t9raw := "./" + tpl("t9.sh")
	out = append(out, tdScenario{
		Kind: "template", ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: rid,
		Source: tplSrc, Target: t9raw, Condition: "not_exists", UpdatePolicy: "auto",
		Config: cfg, destSeed: []byte(old), ManagedEntry: map[string]any{"sha256": sha(old)},
		want: &tdOutcome{Wrote: true, Record: tdRecord(tpl("t9.sh"), sha(rendered), tplSrc, "template", "auto"), DiskSHA: sha(rendered), Lines: []string{"info:refreshed managed template " + t9raw, "msg:" + skip(t9raw)}},
	})

	// d16/d17/d18: recipe_schema.py accepts ANY doc target/source string.
	// pathlib's `root / part` REPLACES the root for an absolute part (native
	// filepath.Join concatenated it), and Path.as_posix PRESERVES exactly two
	// leading slashes (three or more collapse). Fresh writes inside each side's
	// own temp sandbox; '<BASE>'/'<//BASE>' are the temp-root tokens. d18 pins
	// the SOURCE join (an absolute doc.source).
	absDst := filepath.ToSlash(filepath.Join(base, "outside", "d16.md"))
	out = append(out, tdScenario{
		Kind: "doc", ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: rid,
		Source: docSrc, Target: absDst,
		want: &tdOutcome{Dest: "<BASE>/outside/d16.md", Wrote: true, Record: tdRecord("<BASE>/outside/d16.md", sha(tdDocBody), docSrc, "doc", "auto"), DiskSHA: sha(tdDocBody), Lines: []string{"msg:✓ doc <BASE>/outside/d16.md"}},
	})
	d17 := "/" + filepath.ToSlash(filepath.Join(base, "project")) + "/" + doc("d17.md")
	out = append(out, tdScenario{
		Kind: "doc", ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: rid,
		Source: docSrc, Target: d17,
		want: &tdOutcome{Dest: "<//BASE>/project/" + doc("d17.md"), Wrote: true, Record: tdRecord("<//BASE>/project/"+doc("d17.md"), sha(tdDocBody), docSrc, "doc", "auto"), DiskSHA: sha(tdDocBody), Lines: []string{"msg:✓ doc <//BASE>/project/" + doc("d17.md")}},
	})
	absSrc := filepath.ToSlash(filepath.Join(recipeDir, "d18.md"))
	writeTDFile(t, absSrc, tdDocBody, 0o644)
	out = append(out, tdScenario{
		Kind: "doc", ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: rid,
		Source: absSrc, Target: doc("d18.md"),
		want: &tdOutcome{Dest: "<BASE>/project/" + doc("d18.md"), Wrote: true, Record: tdRecord(doc("d18.md"), sha(tdDocBody), "<BASE>/catalog/worktree-flow/d18.md", "doc", "auto"), DiskSHA: sha(tdDocBody), Lines: []string{"msg:✓ doc " + doc("d18.md")}},
	})
	return out
}

// TestMaterializeStateParity pins every classification branch: write, seed,
// skip, user-modified warn, managed-stale refresh, and the stale non-auto
// warn, for both templates and docs, against the exact Python outcomes.
func TestMaterializeStateParity(t *testing.T) {
	for _, sc := range buildTDScenarios(t, t.TempDir()) {
		t.Run(sc.Kind+"_"+filepath.Base(sc.Target), func(t *testing.T) {
			got := sc.run(t)
			want := *sc.want
			// Dest identity is asserted against the real Python authority by
			// TestMaterializeFrozenDifferential; this fixture-only check
			// compares the decision fields (the token is fixture-specific).
			got.Dest, want.Dest = "", ""
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("outcome =\n%#v\nwant\n%#v", got, want)
			}
		})
	}
}

// TestMaterializeRefusalsAndBoundary pins the exact refusal strings and the
// record-only boundary: no lock or manifest is written.
func TestMaterializeRefusalsAndBoundary(t *testing.T) {
	newFixture := func(t *testing.T) (projectRoot, recipeDir string) {
		base := t.TempDir()
		recipeDir = filepath.Join(base, "catalog", "worktree-flow")
		projectRoot = filepath.Join(base, "project")
		writeTDFile(t, filepath.Join(recipeDir, "templates", "run.sh"), "body\n", 0o644)
		writeTDFile(t, filepath.Join(recipeDir, "README.md"), tdDocBody, 0o644)
		if err := os.MkdirAll(projectRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		return projectRoot, recipeDir
	}
	t.Run("template source missing", func(t *testing.T) {
		projectRoot, recipeDir := newFixture(t)
		_, err := MaterializeTemplate(&shared.TemplateRequest{
			ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: "worktree-flow",
			Source: "templates/missing.sh", Target: tdTemplateTarget,
		})
		want := "template source not found: " + filepath.Join(recipeDir, "templates", "missing.sh")
		if err == nil || err.Error() != want {
			t.Fatalf("error = %v, want %q", err, want)
		}
	})
	t.Run("template invalid policy", func(t *testing.T) {
		projectRoot, recipeDir := newFixture(t)
		_, err := MaterializeTemplate(&shared.TemplateRequest{
			ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: "worktree-flow",
			Source: "templates/run.sh", Target: tdTemplateTarget, UpdatePolicy: "bogus",
		})
		want := "invalid update policy 'bogus' for template '" + tdTemplateTarget + "'; expected auto | confirm | never-force"
		if err == nil || err.Error() != want {
			t.Fatalf("error = %v, want %q", err, want)
		}
	})
	t.Run("template escaping target", func(t *testing.T) {
		projectRoot, recipeDir := newFixture(t)
		_, err := MaterializeTemplate(&shared.TemplateRequest{
			ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: "worktree-flow",
			Source: "templates/run.sh", Target: "../escape.txt",
		})
		want := "template target ../escape.txt escapes the project root; refusing to write outside the project. Fix the recipe target and run sync again"
		if err == nil || err.Error() != want {
			t.Fatalf("error = %v, want %q", err, want)
		}
	})
	t.Run("template ancestor symlink", func(t *testing.T) {
		projectRoot, recipeDir := newFixture(t)
		if err := os.Symlink(filepath.Join(projectRoot, "..", "elsewhere"), filepath.Join(projectRoot, "evil")); err != nil {
			t.Fatal(err)
		}
		_, err := MaterializeTemplate(&shared.TemplateRequest{
			ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: "worktree-flow",
			Source: "templates/run.sh", Target: "evil/run.sh",
		})
		want := "ancestor path of evil/run.sh is a symlink; refusing to write through it. Replace it with a real directory and run sync again"
		if err == nil || err.Error() != want {
			t.Fatalf("error = %v, want %q", err, want)
		}
	})
	t.Run("template destination symlink", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			condition string
			regular   bool
		}{
			{"link to a regular file", "always", true},
			{"dangling link", "not_exists", false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				projectRoot, recipeDir := newFixture(t)
				dest := filepath.Join(projectRoot, filepath.FromSlash(tdTemplateTarget))
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
					t.Fatal(err)
				}
				victim := filepath.Join(projectRoot, "victim.sh")
				if tc.regular {
					writeTDFile(t, victim, "echo victim\n", 0o644)
				}
				if err := os.Symlink(victim, dest); err != nil {
					t.Fatal(err)
				}
				_, err := MaterializeTemplate(&shared.TemplateRequest{
					ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: "worktree-flow",
					Source: "templates/run.sh", Target: tdTemplateTarget,
					Condition: tc.condition, UpdatePolicy: "auto",
				})
				want := shared.TemplateSymlinkRefusal(tdTemplateTarget)
				if err == nil || err.Error() != want {
					t.Fatalf("error = %v, want %q", err, want)
				}
				if tc.regular {
					if got := string(tdReadRegular(victim)); got != "echo victim\n" {
						t.Errorf("victim bytes = %q, want them unchanged", got)
					}
				} else if _, statErr := os.Stat(victim); !os.IsNotExist(statErr) {
					t.Errorf("a dangling link target must never be created (stat err=%v)", statErr)
				}
				if info, lstatErr := os.Lstat(dest); lstatErr != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Errorf("the destination symlink must survive: %v, %v", info, lstatErr)
				}
			})
		}
	})
	t.Run("template git parent escape", func(t *testing.T) {
		projectRoot, recipeDir := newFixture(t)
		outside := filepath.Join(filepath.Dir(projectRoot), "escape.sh")
		_, err := MaterializeTemplate(&shared.TemplateRequest{
			ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: "worktree-flow",
			Source: "templates/run.sh", Target: ".git/../../escape.sh",
		})
		want := "template target .git/../../escape.sh escapes the project root; refusing to write outside the project. Fix the recipe target and run sync again"
		if err == nil || err.Error() != want {
			t.Fatalf("error = %v, want %q", err, want)
		}
		if _, statErr := os.Stat(outside); !os.IsNotExist(statErr) {
			t.Fatalf("nothing may be written outside the root: %v", statErr)
		}
		if _, statErr := os.Stat(filepath.Join(projectRoot, ".git")); !os.IsNotExist(statErr) {
			t.Fatalf("no .git tree may be created: %v", statErr)
		}
	})
	t.Run("doc source missing", func(t *testing.T) {
		projectRoot, recipeDir := newFixture(t)
		_, err := MaterializeDoc(&DocRequest{
			ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: "worktree-flow",
			Source: "missing.md", Target: "ai-specs/recipes/worktree-flow/missing.md",
		})
		want := "doc source not found: " + filepath.Join(recipeDir, "missing.md")
		if err == nil || err.Error() != want {
			t.Fatalf("error = %v, want %q", err, want)
		}
	})
	t.Run("no lock or manifest write", func(t *testing.T) {
		projectRoot, recipeDir := newFixture(t)
		if _, err := MaterializeTemplate(&shared.TemplateRequest{
			ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: "worktree-flow",
			Source: "templates/run.sh", Target: tdTemplateTarget,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := MaterializeDoc(&DocRequest{
			ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: "worktree-flow",
			Source: "README.md", Target: "ai-specs/recipes/worktree-flow/README.md",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(projectRoot, "ai-specs", ".ai-specs.lock")); !os.IsNotExist(err) {
			t.Fatalf("the authority must not write a lock (stat err = %v)", err)
		}
	})
}

func tdCount(outcomes []tdOutcome, pred func(tdOutcome) bool) int {
	n := 0
	for _, o := range outcomes {
		if pred(o) {
			n++
		}
	}
	return n
}

func tdHasPrefix(lines []string, prefix string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// .git template destinations in normal and linked temporary Git worktrees
// ---------------------------------------------------------------------------

// tdGitOutcome is the path-independent projection of one .git-destination
// materialization: identity by realpath so a lexical spelling difference (the
// macOS /private canonicalization of a linked worktree's git path) is not
// silently treated as equality.
type tdGitOutcome struct {
	DestRel string            `json:"dest_rel"`
	SHA     string            `json:"sha"`
	Mode    string            `json:"mode"`
	Record  map[string]string `json:"record"`
	Lines   []string          `json:"lines"`
	Error   string            `json:"error"`
}

func tdGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// tdBuildGitProject creates a throwaway Git repository (plus, for "linked", a
// linked worktree) under a fresh temp dir. It never touches the real
// repository's .git.
func tdBuildGitProject(t *testing.T, kind, srcRel string) (mainRoot, projectRoot, recipeDir string) {
	t.Helper()
	base := t.TempDir()
	mainRoot = filepath.Join(base, "main")
	if err := os.MkdirAll(mainRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	tdGit(t, mainRoot, "init", "-q")
	tdGit(t, mainRoot, "config", "user.email", "t@example.com")
	tdGit(t, mainRoot, "config", "user.name", "t")
	writeTDFile(t, filepath.Join(mainRoot, "seed.txt"), "seed\n", 0o644)
	tdGit(t, mainRoot, "add", "seed.txt")
	tdGit(t, mainRoot, "commit", "-qm", "init")
	projectRoot = mainRoot
	if kind == "linked" {
		projectRoot = filepath.Join(base, "wt")
		tdGit(t, mainRoot, "worktree", "add", "-q", "-b", "wt", projectRoot)
	}
	recipeDir = filepath.Join(projectRoot, "recipe")
	writeTDFile(t, filepath.Join(recipeDir, srcRel), "#!/bin/sh\nexit 0\n", 0o755)
	// Neutralize the process umask: the authority chmods the destination to the
	// SOURCE mode, so the hook's frozen 0755 must not follow the umask.
	if err := os.Chmod(filepath.Join(recipeDir, srcRel), 0o755); err != nil {
		t.Fatal(err)
	}
	return mainRoot, projectRoot, recipeDir
}

// tdRealRel is the realpath-relative identity of path under root.
func tdRealRel(t *testing.T, root, path string) string {
	t.Helper()
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("eval root: %v", err)
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("eval path %s: %v", path, err)
	}
	rel, err := filepath.Rel(realRoot, realPath)
	if err != nil {
		t.Fatalf("rel: %v", err)
	}
	return rel
}

// ---------------------------------------------------------------------------
// Frozen Python-oracle payload
// ---------------------------------------------------------------------------

// The legacy live legs drove the REAL Python authority — _python_materialize_
// template (recipe-materialize.py:1430) and materialize_doc (:1551) — patching
// only the lock, copy and print sinks: load_lock returned the scenario's
// prepared lock, write_lock captured it, go_apply_copy copied through
// shutil.copy2 and warn/info recorded the exact messages. The matrix leg ran the
// state matrix below; the git leg ran the same template authority through
// resolve_template_dest on a throwaway normal and linked Git worktree and
// reported the ACTUAL destination as a realpath-relative path under the primary
// repository, with its bytes, permission bits and record.
//
// That output is frozen in testdata/templatedocs_golden.json: per matrix case
// the input projection the fixture drove and the outcome Python produced, per
// git kind the throwaway repo layout and the same outcome fields.
//
// The payload is host-independent: the legacy drivers replaced each fixture's
// temp root with <BASE> (and the POSIX doubled-slash spelling with <//BASE>) on
// both sides, so no host path is frozen. The matrix leg freezes no mode at all
// and the git leg only the written hook's 0755, which the authority chmods from
// the fixture's explicitly-set source mode, so no frozen mode follows the
// process umask. A run of these tests under `umask 077` proves it.
//
// The tests never exec python3 or read lib/. The golden is a snapshot:
// regenerating it needs a human decision.
const templateDocsGoldenFile = "templatedocs_golden.json"

// templateDocsMatrixCases is the hard-coded identity inventory of the frozen
// matrix: the exact case names in capture order. It is an independent oracle of
// coverage — kept in the test, never derived from the bundle — so a bundle that
// is missing, renamed, duplicated or reordered cannot validate silently.
var templateDocsMatrixCases = []string{
	"template_t1.sh", "template_t2.sh", "template_t3.sh", "template_t4.sh",
	"template_t5.sh", "template_t6.sh", "template_t7.sh", "template_t8.sh",
	"doc_d1.md", "doc_d2.md", "doc_d3.md", "doc_d4.md", "doc_d5.md", "doc_d6.md", "doc_d7.md",
	"doc_d8-dir", "doc_d9-link",
	"doc_d10.md", "doc_d11.md", "doc_d12.md", "doc_d13.md", "doc_d14.md",
	"doc_d15-dangling", "template_t9.sh", "doc_d16.md", "doc_d17.md", "doc_d18.md",
}

// templateDocsGitCases is the same inventory for the .git-destination leg.
var templateDocsGitCases = []string{"normal", "linked"}

// The two token spellings every host-dependent path segment is frozen as, and
// the .git leg's fixed request.
const (
	tdGoldenBaseToken = "<BASE>"
	tdGitTarget       = ".git/hooks/pre-commit"
	tdGitSrcRel       = "templates/pre-commit"
)

// tdFrozenInput is the frozen input projection of one matrix case: the request
// fields the authority reads plus the fixture state the case seeded. A drifted
// fixture fails on this, before any outcome is compared, instead of appearing as
// an opaque payload mismatch.
type tdFrozenInput struct {
	Kind         string            `json:"kind"`
	Target       string            `json:"target"`
	Source       string            `json:"source"`
	RecipeID     string            `json:"recipe_id"`
	Condition    string            `json:"condition,omitempty"`
	UpdatePolicy string            `json:"update_policy,omitempty"`
	Config       map[string]string `json:"config,omitempty"`
	ManagedEntry map[string]string `json:"managed_entry,omitempty"`
	DestKind     string            `json:"dest_kind,omitempty"`
	SeedSHA256   string            `json:"seed_sha256,omitempty"`
}

// tdFrozenCase is the frozen record of one matrix case.
type tdFrozenCase struct {
	Name    string        `json:"name"`
	Input   tdFrozenInput `json:"input"`
	Outcome tdOutcome     `json:"outcome"`
}

// tdGitFrozenInput is the frozen input projection of one .git-destination case.
// The throwaway repo layout rides here per case, not per leg, because a linked
// worktree's project root lives beside the primary repo instead of inside it.
type tdGitFrozenInput struct {
	Kind         string `json:"kind"`
	Target       string `json:"target"`
	SrcRel       string `json:"src_rel"`
	SourceSHA256 string `json:"source_sha256"`
	Main         string `json:"main"`
	ProjectRoot  string `json:"project_root"`
	RecipeDir    string `json:"recipe_dir"`
}

// tdGitFrozenCase is the frozen record of one .git-destination case.
type tdGitFrozenCase struct {
	Name    string           `json:"name"`
	Input   tdGitFrozenInput `json:"input"`
	Outcome tdGitOutcome     `json:"outcome"`
}

// templateDocsGolden is the frozen bundle: oracle provenance, the matrix leg and
// the .git-destination leg, each with its own inventory.
type templateDocsGolden struct {
	Oracle map[string]string `json:"oracle"`
	Matrix struct {
		Fixture map[string]string `json:"fixture"`
		Cases   []tdFrozenCase    `json:"cases"`
	} `json:"matrix"`
	GitDest struct {
		Cases []tdGitFrozenCase `json:"cases"`
	} `json:"gitdest"`
}

// tdCaseName is the frozen identity of one matrix scenario: the same kind and
// destination basename the fixture-only parity test uses as its subtest name.
func tdCaseName(sc *tdScenario) string {
	return sc.Kind + "_" + filepath.Base(sc.Target)
}

// tdStringMap narrows a scenario's string-valued map field for the freeze.
func tdStringMap(in map[string]any) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		text, _ := value.(string)
		out[key] = text
	}
	return out
}

// tdInputOf projects one matrix scenario onto the frozen input surface.
func tdInputOf(sc *tdScenario) tdFrozenInput {
	base := filepath.Dir(sc.ProjectRoot)
	in := tdFrozenInput{
		Kind: sc.Kind, Target: tdProjectText(base, sc.Target), Source: tdProjectText(base, sc.Source),
		RecipeID: sc.RecipeID, Condition: sc.Condition, UpdatePolicy: sc.UpdatePolicy,
		Config: tdStringMap(sc.Config), ManagedEntry: tdStringMap(sc.ManagedEntry),
		DestKind: sc.destKind,
	}
	if sc.destSeed != nil {
		in.SeedSHA256 = shared.Sha256Bytes(sc.destSeed)
	}
	return in
}

// tdMatrixFixture projects the matrix fixture onto the frozen layout surface:
// the two roots relative to the temp base and the bytes hash of each source
// family the cases read.
func tdMatrixFixture(t *testing.T, base string, sc *tdScenario) map[string]string {
	t.Helper()
	rel := func(path string) string {
		s, err := filepath.Rel(base, path)
		if err != nil {
			t.Fatalf("fixture path %s is not under the base: %v", path, err)
		}
		return filepath.ToSlash(s)
	}
	sourceSHA := ""
	if data := tdReadRegular(filepath.Join(sc.RecipeDir, "templates", "run.sh")); data != nil {
		sourceSHA = shared.Sha256Bytes(data)
	}
	return map[string]string{
		"recipe_dir": rel(sc.RecipeDir), "project_root": rel(sc.ProjectRoot),
		"template_sha256": sourceSHA, "doc_sha256": shared.Sha256Bytes([]byte(tdDocBody)),
	}
}

// tdGitInputAndFixture builds the frozen input projection of one .git case from
// the throwaway repo the builder created: the three roots relative to the temp
// base and the hook source hash.
func tdGitInputAndFixture(t *testing.T, kind, mainRoot, projectRoot, recipeDir string) (tdGitFrozenInput, map[string]string) {
	t.Helper()
	base := filepath.Dir(mainRoot)
	rel := func(path string) string {
		s, err := filepath.Rel(base, path)
		if err != nil {
			t.Fatalf("fixture path %s is not under the base: %v", path, err)
		}
		return filepath.ToSlash(s)
	}
	source := ""
	if data := tdReadRegular(filepath.Join(recipeDir, filepath.FromSlash(tdGitSrcRel))); data != nil {
		source = shared.Sha256Bytes(data)
	}
	in := tdGitFrozenInput{
		Kind: kind, Target: tdGitTarget, SrcRel: tdGitSrcRel, SourceSHA256: source,
		Main: rel(mainRoot), ProjectRoot: rel(projectRoot), RecipeDir: rel(recipeDir),
	}
	return in, map[string]string{kind: filepath.ToSlash(rel(mainRoot)) + "/" + tdGitTarget}
}

// loadTemplateDocsGolden reads the frozen bundle and validates both identity
// inventories, the oracle metadata, the fixture layout and the tokenized shape
// of every frozen path before any comparison runs.
func loadTemplateDocsGolden(t *testing.T) templateDocsGolden {
	t.Helper()
	raw := readGoldenBundle(t, templateDocsGoldenFile)
	// A capture that leaked a host path cannot validate here.
	for _, leak := range []string{"/Users/", "/private/", "/var/folders"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("frozen oracle %s leaks the host path segment %q", templateDocsGoldenFile, leak)
		}
	}
	var golden templateDocsGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("parse frozen oracle %s: %v", templateDocsGoldenFile, err)
	}
	for _, key := range []string{"python", "script", "driver"} {
		if golden.Oracle[key] == "" {
			t.Fatalf("frozen oracle %s carries no %s metadata", templateDocsGoldenFile, key)
		}
	}
	for _, key := range []string{"recipe_dir", "project_root", "template_sha256", "doc_sha256"} {
		if golden.Matrix.Fixture[key] == "" {
			t.Fatalf("frozen oracle %s carries no fixture %s", templateDocsGoldenFile, key)
		}
	}
	for _, key := range []string{"recipe_dir", "project_root"} {
		if path := golden.Matrix.Fixture[key]; filepath.IsAbs(path) || strings.HasPrefix(path, "..") {
			t.Fatalf("frozen fixture %s %q must be root-relative", key, path)
		}
	}
	// A path that is absolute but not tokenized would carry the host: every
	// host-dependent segment is <BASE> or <//BASE>.
	tokenized := func(path string) bool {
		return !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "/"+tdGoldenBaseToken+"/")
	}
	names := make([]string, len(golden.Matrix.Cases))
	for i, rec := range golden.Matrix.Cases {
		names[i] = rec.Name
		if rec.Input.Kind == "" || rec.Input.Target == "" || rec.Input.RecipeID == "" {
			t.Fatalf("frozen case %q carries an incomplete input projection", rec.Name)
		}
		for _, path := range []string{rec.Input.Target, rec.Input.Source, rec.Outcome.Dest} {
			if !tokenized(path) {
				t.Fatalf("frozen case %q path %q must be tokenized or project-relative", rec.Name, path)
			}
		}
	}
	validateGoldenIdentity(t, "templatedocs matrix", names, templateDocsMatrixCases)
	gitNames := make([]string, len(golden.GitDest.Cases))
	for i, rec := range golden.GitDest.Cases {
		gitNames[i] = rec.Name
		if rec.Input.Kind != rec.Name || rec.Input.Target == "" || rec.Input.SrcRel == "" || rec.Input.SourceSHA256 == "" {
			t.Fatalf("frozen git case %q carries an incomplete input projection", rec.Name)
		}
		for _, path := range []string{rec.Input.Main, rec.Input.ProjectRoot, rec.Input.RecipeDir, rec.Outcome.DestRel} {
			if filepath.IsAbs(path) || strings.HasPrefix(path, "..") {
				t.Fatalf("frozen git case %q path %q must be root-relative", rec.Name, path)
			}
		}
		if rec.Outcome.DestRel != rec.Input.Target {
			t.Fatalf("frozen git case %q wrote %q, want the target %q in the primary repo",
				rec.Name, rec.Outcome.DestRel, rec.Input.Target)
		}
	}
	validateGoldenIdentity(t, "templatedocs gitdest", gitNames, templateDocsGitCases)
	return golden
}

// TestMaterializeFrozenDifferential pins the native outcomes against the frozen
// record of the real Python authority over the full state matrix: templates
// t1-t9 and docs d1-d18 — write, seed, skip, user-modified warn, managed-stale
// refresh, stale non-auto warn, directory and symlink preservation, the dangling
// symlink copy-through, noncanonical lock keys, out-of-project and
// doubled-slash destinations and an absolute doc source. Both sides must agree
// on wrote, destination identity, record fields, bytes on disk, every emitted
// line and the error.
func TestMaterializeFrozenDifferential(t *testing.T) {
	golden := loadTemplateDocsGolden(t)

	base := t.TempDir()
	scenarios := buildTDScenarios(t, base)
	byName := make(map[string]*tdScenario, len(scenarios))
	for i := range scenarios {
		sc := &scenarios[i]
		name := tdCaseName(sc)
		if _, seen := byName[name]; seen {
			t.Fatalf("the fixture builder produced the case %q twice", name)
		}
		byName[name] = sc
	}
	if layout := tdMatrixFixture(t, base, &scenarios[0]); !reflect.DeepEqual(layout, golden.Matrix.Fixture) {
		t.Fatalf("fixture layout diverges\n--- go ---\n%+v\n--- frozen ---\n%+v", layout, golden.Matrix.Fixture)
	}
	if len(byName) != len(golden.Matrix.Cases) {
		t.Fatalf("the fixture builder produced %d case(s), the frozen bundle records %d", len(byName), len(golden.Matrix.Cases))
	}

	got := make([]tdOutcome, 0, len(golden.Matrix.Cases))
	for _, rec := range golden.Matrix.Cases {
		sc, ok := byName[rec.Name]
		if !ok {
			t.Fatalf("frozen case %q has no native scenario", rec.Name)
		}
		// The frozen inputs are an input of the payload: a drifted fixture
		// fails here, not as an opaque outcome mismatch.
		if input := tdInputOf(sc); !reflect.DeepEqual(input, rec.Input) {
			t.Fatalf("case %q input diverges\n--- go ---\n%+v\n--- frozen ---\n%+v", rec.Name, input, rec.Input)
		}
		got = append(got, sc.run(t))
	}

	// Non-vacuity: both sides really wrote, recorded, warned, refreshed a
	// managed override and kept the doubled-slash destination distinct.
	for _, marker := range []struct {
		name string
		any  func([]tdOutcome) bool
	}{
		{"a write", func(os []tdOutcome) bool { return tdCount(os, func(o tdOutcome) bool { return o.Wrote }) > 0 }},
		{"a record", func(os []tdOutcome) bool { return tdCount(os, func(o tdOutcome) bool { return o.Record != nil }) > 0 }},
		{"a warning", func(os []tdOutcome) bool {
			return tdCount(os, func(o tdOutcome) bool { return tdHasPrefix(o.Lines, "warn:") }) > 0
		}},
		{"an info refresh", func(os []tdOutcome) bool {
			return tdCount(os, func(o tdOutcome) bool { return tdHasPrefix(o.Lines, "info:") }) > 0
		}},
		{"a doubled-slash destination", func(os []tdOutcome) bool {
			return tdCount(os, func(o tdOutcome) bool { return strings.HasPrefix(o.Dest, "<//") }) > 0
		}},
	} {
		frozen := make([]tdOutcome, 0, len(golden.Matrix.Cases))
		for _, rec := range golden.Matrix.Cases {
			frozen = append(frozen, rec.Outcome)
		}
		if !marker.any(frozen) {
			t.Fatalf("frozen outcomes are missing %s: %#v", marker.name, frozen)
		}
		if !marker.any(got) {
			t.Fatalf("native outcomes are missing %s: %#v", marker.name, got)
		}
	}

	for i, rec := range golden.Matrix.Cases {
		t.Run(rec.Name, func(t *testing.T) {
			if !reflect.DeepEqual(got[i], rec.Outcome) {
				t.Fatalf("outcome diverges\n--- go ---\n%#v\n--- frozen ---\n%#v", got[i], rec.Outcome)
			}
		})
	}
}

// TestMaterializeTemplateGitDestParity verifies a `.git/hooks/...` template
// destination in a NORMAL and a LINKED temporary Git worktree against the frozen
// record of the real Python authority: the identical realpath-relative
// destination under the primary repository, the same bytes, permission bits and
// record. Both fixtures are throwaway repos: the real repository's .git is never
// touched.
func TestMaterializeTemplateGitDestParity(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	golden := loadTemplateDocsGolden(t)
	byKind := make(map[string]tdGitFrozenCase, len(golden.GitDest.Cases))
	for _, rec := range golden.GitDest.Cases {
		byKind[rec.Name] = rec
	}

	for _, kind := range templateDocsGitCases {
		t.Run(kind, func(t *testing.T) {
			rec, ok := byKind[kind]
			if !ok {
				t.Fatalf("frozen git case %q is missing", kind)
			}
			mainRoot, projectRoot, recipeDir := tdBuildGitProject(t, kind, tdGitSrcRel)
			if input, _ := tdGitInputAndFixture(t, kind, mainRoot, projectRoot, recipeDir); !reflect.DeepEqual(input, rec.Input) {
				t.Fatalf("case %q input diverges\n--- go ---\n%+v\n--- frozen ---\n%+v", kind, input, rec.Input)
			}
			res, err := MaterializeTemplate(&shared.TemplateRequest{
				ProjectRoot: projectRoot, RecipeDir: recipeDir, RecipeID: "worktree-flow",
				Source: tdGitSrcRel, Target: tdGitTarget, Condition: "not_exists", UpdatePolicy: "auto",
			})
			if err != nil {
				t.Fatalf("MaterializeTemplate: %v", err)
			}
			got := tdGitOutcome{DestRel: tdRealRel(t, mainRoot, res.Dest), Lines: tdLines(res.Warnings, res.Info, res.Message)}
			if data := tdReadRegular(res.Dest); data != nil {
				got.SHA = shared.Sha256Bytes(data)
			}
			if info, statErr := os.Stat(res.Dest); statErr == nil {
				got.Mode = fmt.Sprintf("%o", info.Mode().Perm())
			}
			if res.Record != nil {
				got.Record = map[string]string{
					"target": res.Record.Target, "sha256": res.Record.SHA256,
					"recipe": res.Record.Recipe, "source": res.Record.Source,
					"kind": res.Record.Kind, "policy": res.Record.Policy,
				}
			}
			if !res.Wrote {
				t.Errorf("native did not write %s", res.Dest)
			}
			if !reflect.DeepEqual(got, rec.Outcome) {
				t.Errorf("outcome =\n%#v\nwant\n%#v", got, rec.Outcome)
			}
			if _, err := os.Stat(filepath.Join(mainRoot, ".git", "hooks", "pre-commit")); err != nil {
				t.Errorf("hook not in the primary repo's shared hooks dir: %v", err)
			}
		})
	}
}
