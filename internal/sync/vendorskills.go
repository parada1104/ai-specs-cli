package sync

// Native port of lib/_internal/vendor-skills.py (U2a T5): the root [[deps]]
// route (sync_vendored_skills) and the recipe-dep delegation
// (sync_dep_target with in_project=False). The Python module stays under
// lib/_internal/ as the differential oracle until U2e.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/ai-specs/internal/projectcache"
	"ai-specs.dev/ai-specs/internal/schema"
	"ai-specs.dev/ai-specs/internal/skills"
	"ai-specs.dev/ai-specs/internal/toml"
)

var (
	vendorAncillaryDirs = [...]string{"assets", "references", "scripts"}
	vendorShaRE         = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
)

// vendorDepSpec is one dependency to materialize. table carries the full
// [[deps]] entry for rendering; a recipe dep has only id/source/path.
type vendorDepSpec struct {
	table  *toml.Table
	id     string
	source string
	path   string
	ref    string
}

func vendorDepFromTable(table *toml.Table) vendorDepSpec {
	get := func(k string) any { v, _ := table.Get(k); return v }
	asString := func(v any) string {
		if s, ok := v.(string); ok {
			return s
		}
		return ""
	}
	ref := asString(get("ref"))
	if !config.Truthy(ref) {
		ref = asString(get("rev"))
	}
	return vendorDepSpec{
		table:  table,
		id:     asString(get("id")),
		source: asString(get("source")),
		path:   strings.Trim(asString(get("path")), "/"),
		ref:    strings.TrimSpace(ref),
	}
}

// SyncVendoredSkills mirrors vendor-skills.py's main(): load [[deps]], vendor
// each one into the project's ai-specs/.deps tree, then record the file hashes
// in ai-specs/.ai-specs.lock. Exit 0 on success (including no deps); 1 with a
// `  ✗ ` line on stderr when loading or a dep fails (Python prints the same for
// its fail() paths; an uncaught traceback only differs in wording).
func SyncVendoredSkills(projectRoot, cliHome string, stdout, stderr io.Writer) int {
	root := projectcache.ResolvePath(projectRoot)
	tomlPath := filepath.Join(root, "ai-specs", "ai-specs.toml")
	data, err := config.LoadManifest(tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "  ✗ %s\n", err)
		return 1
	}
	section, err := config.ReadSection(data, "deps")
	if err != nil {
		fmt.Fprintf(stderr, "  ✗ %s\n", err)
		return 1
	}
	var deps []*toml.Table
	switch typed := section.(type) {
	case []any:
		for _, item := range typed {
			if table, ok := item.(*toml.Table); ok {
				deps = append(deps, table)
			}
		}
	case []*toml.Table:
		deps = append(deps, typed...)
	}
	if len(deps) == 0 {
		fmt.Fprintln(stdout, "  (no [[deps]] declared — nothing to vendor)")
		return 0
	}

	lockPath := filepath.Join(root, "ai-specs", ".ai-specs.lock")
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		fmt.Fprintf(stderr, "  ✗ %s\n", err)
		return 1
	}
	for _, table := range deps {
		dep := vendorDepFromTable(table)
		targetDir, err := syncDepTarget(dep, root, cliHome, true, stdout, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "  ✗ %s\n", err)
			return 1
		}
		hashes, err := vendorFileHashes(targetDir)
		if err != nil {
			fmt.Fprintf(stderr, "  ✗ %s\n", err)
			return 1
		}
		lock.SetDepSkillHashes(lk, dep.id, dep.id, hashes)
	}
	if err := lock.WriteLock(lockPath, lk); err != nil {
		fmt.Fprintf(stderr, "  ✗ %s\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "  ✓ vendored %d dep(s)\n", len(deps))
	return 0
}

// materializeDepSkill is the recipe-dep delegation: sync_dep_target with
// in_project=False, staging under the CLI cache. The progress line is the
// Python module's; the caller prints the `✓ dep skill <id>` line.
func materializeDepSkill(projectRoot, cliHome string, skill *schema.SkillRef, stdout, stderr io.Writer) error {
	dep := vendorDepSpec{
		id:     skill.ID,
		source: skill.URL,
		path:   strings.Trim(skill.Path, "/"),
	}
	_, err := syncDepTarget(dep, projectRoot, cliHome, false, stdout, stderr)
	return err
}

// syncDepTarget mirrors vendor-skills.py's sync_dep_target: clone the source,
// locate SKILL.md, replace the target after validation, render the standardized
// frontmatter and copy the ancillary dirs. It returns the vendored target dir.
func syncDepTarget(dep vendorDepSpec, projectRoot, cliHome string, inProject bool, stdout, stderr io.Writer) (string, error) {
	if dep.id == "" || dep.source == "" {
		return "", fmt.Errorf("dep missing id/source: %s", vendorDepRepr(dep))
	}

	var depsRoot string
	if inProject {
		depsRoot = projectcache.InprojectDepsRoot(projectRoot)
	} else {
		if _, err := projectcache.EnsureCache(projectRoot, cliHome); err != nil {
			return "", err
		}
		depsRoot = projectcache.DepsSkillsRoot(projectRoot, cliHome)
	}
	targetDir := filepath.Join(depsRoot, dep.id, "skills", dep.id)

	fmt.Fprintf(stdout, "  ▸ %s  ←  %s", dep.id, dep.source)
	if dep.path != "" {
		fmt.Fprintf(stdout, "  (path: %s)", dep.path)
	}
	if dep.ref != "" {
		fmt.Fprintf(stdout, "  (ref: %s)", dep.ref)
	}
	fmt.Fprintln(stdout)

	tmp, err := os.MkdirTemp("", "ai-specs-vendor-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	if err := cloneVendorSource(dep.source, tmp, dep.ref, stderr); err != nil {
		return "", err
	}

	srcDir := tmp
	if dep.path != "" {
		srcDir = filepath.Join(tmp, filepath.FromSlash(dep.path))
	}
	skillMD := filepath.Join(srcDir, "SKILL.md")
	info, err := os.Stat(skillMD)
	if err != nil || !info.Mode().IsRegular() {
		rel, relErr := filepath.Rel(tmp, skillMD)
		if relErr != nil {
			rel = skillMD
		}
		return "", fmt.Errorf("%s: SKILL.md not found at %s", dep.id, filepath.ToSlash(rel))
	}
	upstream, err := readVendorText(skillMD)
	if err != nil {
		return "", err
	}
	rendered, err := vendorRenderMarkdown(dep, upstream)
	if err != nil {
		return "", err
	}

	// Partial-state behaviour (S6): the target is replaced after the clone and
	// the SKILL.md validation, before the copy, so a late failure leaves it.
	if err := os.RemoveAll(targetDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(targetDir, 0o777); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(targetDir, "SKILL.md"), []byte(rendered), 0o666); err != nil {
		return "", err
	}
	for _, ancillary := range vendorAncillaryDirs {
		aSrc := filepath.Join(srcDir, ancillary)
		if info, err := os.Stat(aSrc); err == nil && info.IsDir() {
			if err := projectcache.CopyTree(aSrc, filepath.Join(targetDir, ancillary)); err != nil {
				return "", err
			}
		}
	}
	return targetDir, nil
}

func vendorRenderMarkdown(dep vendorDepSpec, upstream string) (string, error) {
	if dep.table != nil {
		return skills.VendoredSkillMarkdown(dep.table, upstream)
	}
	return skills.VendoredSkillMarkdownFromMap(map[string]any{
		"id":     dep.id,
		"source": dep.source,
	}, upstream)
}

// resolveVendorCloneSource mirrors _resolve_clone_source: the offline/CI
// override only applies to the kepano obsidian-skills URL.
func resolveVendorCloneSource(source string) string {
	fixture := strings.TrimSpace(os.Getenv("AI_SPECS_VENDOR_FIXTURE_ROOT"))
	if fixture == "" || !strings.Contains(source, "kepano/obsidian-skills") {
		return source
	}
	return fixture
}

// cloneVendorSource mirrors vendor-skills.py's clone(): a plain local tree is
// copied (the tests' fixtures have no .git); otherwise git clones, fetching a
// raw SHA directly so a shallow clone can pin it.
func cloneVendorSource(source, dest, ref string, stderr io.Writer) error {
	resolved := resolveVendorCloneSource(source)
	if info, err := os.Stat(resolved); err == nil && info.IsDir() {
		if _, err := os.Stat(filepath.Join(resolved, ".git")); err != nil {
			if _, err := os.Stat(dest); err == nil {
				if err := os.RemoveAll(dest); err != nil {
					return err
				}
			}
			return projectcache.CopyTree(resolved, dest)
		}
	}
	if ref != "" && vendorShaRE.MatchString(ref) {
		if err := runVendorGit(stderr, "init", "--quiet", dest); err != nil {
			return err
		}
		if err := runVendorGit(stderr, "-C", dest, "remote", "add", "origin", resolved); err != nil {
			return err
		}
		if err := runVendorGit(stderr, "-C", dest, "fetch", "--depth", "1", "--quiet", "origin", ref); err != nil {
			return err
		}
		return runVendorGit(stderr, "-C", dest, "checkout", "--quiet", "FETCH_HEAD")
	}
	if ref != "" {
		return runVendorGit(stderr, "clone", "--depth", "1", "--quiet", "--branch", ref, resolved, dest)
	}
	return runVendorGit(stderr, "clone", "--depth", "1", "--quiet", resolved, dest)
}

func runVendorGit(stderr io.Writer, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stderr = stderr
	return cmd.Run()
}

// readVendorText mirrors Path.read_text(): universal newlines, so CRLF and
// lone CR arrive as \n before the frontmatter is parsed and re-rendered.
func readVendorText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n"), nil
}

// vendorFileHashes mirrors the lock's hash set: every regular file under the
// target, keyed by its slash-separated path relative to the target.
func vendorFileHashes(targetDir string) (map[string]string, error) {
	hashes := map[string]string{}
	err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(targetDir, path)
		if err != nil {
			return err
		}
		sum, err := lock.Sha256OfFile(path)
		if err != nil {
			return err
		}
		hashes[filepath.ToSlash(rel)] = sum
		return nil
	})
	if err != nil {
		return nil, err
	}
	return hashes, nil
}

func vendorDepRepr(dep vendorDepSpec) string {
	if dep.table != nil {
		return fmt.Sprintf("%v", dep.table.Any())
	}
	return fmt.Sprintf("{'id': %q, 'source': %q}", dep.id, dep.source)
}
