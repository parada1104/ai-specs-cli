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
// T4's Python-oracle differential lives at the bottom of this file.

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
			// TestMaterializePythonDifferential; this fixture-only check
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

// ---------------------------------------------------------------------------
// T4: Python-oracle differential
// ---------------------------------------------------------------------------

// tdOracle drives the REAL Python entry points — _python_materialize_template
// and materialize_doc — on the given scenarios. It patches only the lock and
// copy sinks: load_lock returns the scenario's prepared lock, write_lock
// captures it, go_apply_copy copies through shutil.copy2, and warn/info record
// their exact messages. The outcome is path-independent, byte-comparable to
// the native tdOutcome. GO_*_BRIDGE_FALLBACK stderr noise is discarded.
const tdOracle = `
import contextlib, copy, hashlib, importlib.util, io, json, shutil, sys
from pathlib import Path
from types import SimpleNamespace

spec = importlib.util.spec_from_file_location("materialize_outcome_oracle", sys.argv[1])
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)

scenarios = json.loads(Path(sys.argv[2]).read_text())

def sha(data):
    return hashlib.sha256(data.replace(b"\r\n", b"\n")).hexdigest()

results = []
for sc in scenarios:
    project_root = Path(sc["project_root"])
    recipe_dir = Path(sc["recipe_dir"])
    base = str(project_root.parent)

    def project(s):
        return s.replace("//" + base.lstrip("/"), "<//BASE>").replace(base, "<BASE>")

    dest = project_root / sc["target"]
    before = dest.read_bytes() if dest.is_file() else None
    key = Path(sc["target"]).as_posix()
    lock = {"managed": {}}
    if sc.get("managed_entry"):
        lock["managed"][key] = dict(sc["managed_entry"])
    captured = {}
    lines = []
    mod.load_lock = lambda p, _l=lock: _l
    mod.write_lock = lambda p, l, _c=captured: _c.__setitem__("lock", copy.deepcopy(l))
    mod.warn = lambda msg, _lines=lines: _lines.append("warn:" + msg)
    mod.info = lambda msg, _lines=lines: _lines.append("info:" + msg)
    if sc["kind"] == "doc":
        def _copy(items):
            for it in items:
                d = Path(it["dest"])
                d.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(it["src"], d)
            return [{"id": it["id"], "status": "ok"} for it in items]
        mod.go_apply_copy = _copy
    stdout = io.StringIO()
    error = None
    try:
        with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(io.StringIO()):
            if sc["kind"] == "doc":
                doc = SimpleNamespace(source=sc["source"], target=sc["target"])
                mod.materialize_doc(recipe_dir, doc, project_root, sc.get("recipe_id"))
            else:
                tpl = SimpleNamespace(source=sc["source"], target=sc["target"],
                                      condition=sc.get("condition", "not_exists"),
                                      update_policy=sc.get("update_policy", "auto"))
                mod._python_materialize_template(recipe_dir, tpl, project_root, sc.get("config"), sc.get("recipe_id"))
    except Exception as exc:  # noqa: BLE001
        error = str(exc)
    for line in stdout.getvalue().splitlines():
        if line.startswith("    "):
            lines.append("msg:" + line[4:])
    after = dest.read_bytes() if dest.is_file() else None
    entry = (captured.get("lock") or {}).get("managed", {}).get(key)
    if entry is not None:
        entry = {"target": key, **entry}
        entry = {k: (project(v) if isinstance(v, str) else v) for k, v in entry.items()}
    results.append({
        "wrote": before != after,
        "dest": project(str(dest)) if sc["kind"] == "doc" else "",
        "record": entry,
        "disk_sha": sha(after) if after is not None else "",
        "lines": [project(line) for line in lines],
        "error": error,
    })
print(json.dumps(results, sort_keys=True))
`

// TestMaterializePythonDifferential compares the native outcomes against the
// real Python authority over the full state matrix. Both sides start from
// byte-identical fixtures under different roots and must agree on wrote,
// record fields, bytes on disk, and every emitted line.
func TestMaterializePythonDifferential(t *testing.T) {
	python, authority := syncPythonAuthority(t)
	driver := filepath.Join(t.TempDir(), "td_oracle.py")
	if err := os.WriteFile(driver, []byte(tdOracle), 0o600); err != nil {
		t.Fatal(err)
	}

	nativeBase := t.TempDir()
	pythonBase := t.TempDir()
	nativeScenarios := buildTDScenarios(t, nativeBase)
	pythonScenarios := buildTDScenarios(t, pythonBase)
	if len(nativeScenarios) == 0 || len(nativeScenarios) != len(pythonScenarios) {
		t.Fatalf("scenario builders disagree: %d vs %d", len(nativeScenarios), len(pythonScenarios))
	}

	got := make([]tdOutcome, 0, len(nativeScenarios))
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
	var want []tdOutcome
	if err := json.Unmarshal([]byte(raw), &want); err != nil {
		t.Fatalf("parse oracle JSON: %v\nraw: %s", err, raw)
	}
	if len(want) != len(got) {
		t.Fatalf("oracle returned %d outcome(s), want %d", len(want), len(got))
	}

	// Non-vacuity: both sides really wrote, seeded and warned.
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
			t.Fatalf("scenario %d diverges\n--- go ---\n%#v\n--- python ---\n%#v", i, got[i], want[i])
		}
	}
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

// tdGitOracle drives the REAL Python template authority on a git project root
// and reports the ACTUAL destination it wrote: the realpath-relative location
// under the primary repository, the bytes, the permission bits and the record
// provenance. Only the lock sink and the warn/info sinks are patched.
const tdGitOracle = `
import contextlib, copy, hashlib, importlib.util, io, json, os, stat, sys
from pathlib import Path
from types import SimpleNamespace

spec = importlib.util.spec_from_file_location("git_dest_oracle", sys.argv[1])
mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)

project_root, main_root = Path(sys.argv[2]), Path(sys.argv[3])
recipe_dir, target, src_rel = Path(sys.argv[4]), sys.argv[5], sys.argv[6]

captured, lines = {}, []
mod.load_lock = lambda p: {"managed": {}}
mod.write_lock = lambda p, l: captured.__setitem__("lock", copy.deepcopy(l))
mod.warn = lambda m: lines.append("warn:" + m)
mod.info = lambda m: lines.append("info:" + m)

dest, git_resolved = mod.resolve_template_dest(project_root, target)
stdout = io.StringIO()
error = None
try:
    with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(io.StringIO()):
        tpl = SimpleNamespace(source=src_rel, target=target, condition="not_exists", update_policy="auto")
        mod._python_materialize_template(recipe_dir, tpl, project_root, None, "worktree-flow")
except Exception as exc:  # noqa: BLE001
    error = str(exc)
for line in stdout.getvalue().splitlines():
    if line.startswith("    "):
        lines.append("msg:" + line[4:])
data = dest.read_bytes() if dest.is_file() else b""
key = Path(target).as_posix()
entry = (captured.get("lock") or {}).get("managed", {}).get(key)
print(json.dumps({
    "dest_rel": os.path.relpath(os.path.realpath(dest), os.path.realpath(main_root)),
    "git_resolved": bool(git_resolved),
    "sha": hashlib.sha256(data.replace(b"\r\n", b"\n")).hexdigest() if data else "",
    "mode": format(stat.S_IMODE(dest.stat().st_mode), "o") if dest.exists() else "",
    "record": ({"target": key, **entry} if entry else None),
    "lines": lines,
    "error": error,
}, sort_keys=True))
`

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

// TestMaterializeTemplateGitDestParity verifies a `.git/hooks/...` template
// destination in a NORMAL and a LINKED temporary Git worktree. The root
// authority and the real Python authority must write the SAME file (equal
// realpath-relative identity), with equal bytes, permission bits and record.
// Both fixtures are throwaway repos: the real repository's .git is never
// touched.
func TestMaterializeTemplateGitDestParity(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	python, authority := syncPythonAuthority(t)
	driver := filepath.Join(t.TempDir(), "td_git_oracle.py")
	if err := os.WriteFile(driver, []byte(tdGitOracle), 0o600); err != nil {
		t.Fatal(err)
	}
	const target = ".git/hooks/pre-commit"
	const srcRel = "templates/pre-commit"

	for _, kind := range []string{"normal", "linked"} {
		t.Run(kind, func(t *testing.T) {
			pyMain, pyRoot, pyRecipe := tdBuildGitProject(t, kind, srcRel)
			raw := runSyncPythonOracle(t, python, driver, authority, pyRoot, pyMain, pyRecipe, target, srcRel)
			var want tdGitOutcome
			if err := json.Unmarshal([]byte(raw), &want); err != nil {
				t.Fatalf("parse oracle: %v\nraw: %s", err, raw)
			}
			if want.Error != "" {
				t.Fatalf("python oracle refused the .git target: %s", want.Error)
			}

			goMain, goRoot, goRecipe := tdBuildGitProject(t, kind, srcRel)
			res, err := MaterializeTemplate(&shared.TemplateRequest{
				ProjectRoot: goRoot, RecipeDir: goRecipe, RecipeID: "worktree-flow",
				Source: srcRel, Target: target, Condition: "not_exists", UpdatePolicy: "auto",
			})
			if err != nil {
				t.Fatalf("MaterializeTemplate: %v", err)
			}
			got := tdGitOutcome{DestRel: tdRealRel(t, goMain, res.Dest), Lines: tdLines(res.Warnings, res.Info, res.Message)}
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
			if !reflect.DeepEqual(got, want) {
				t.Errorf("outcome =\n%#v\nwant\n%#v", got, want)
			}
			if _, err := os.Stat(filepath.Join(goMain, ".git", "hooks", "pre-commit")); err != nil {
				t.Errorf("hook not in the primary repo's shared hooks dir: %v", err)
			}
		})
	}
}
