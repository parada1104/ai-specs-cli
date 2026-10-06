package sync

// Contract tests for the native sync-agent fan-out port (epic [Go 07.S15]).
//
// The oracle is lib/sync-agent.sh and lib/_internal/platform.sh at the frozen
// revision: source-structure constants (cd line numbers, the usage heredoc,
// every platform.sh field) are pinned by reading the shell files, and the
// behavioral semantics (symlink kinds, refusals, D3/D22/D24, banner/footer
// framing) are pinned through the same entry points the CLI route uses.
// Python-dependent paths (standalone target resolution, the materialize
// seams) are gated on python3 like the rest of the port's differentials.

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

// ── Source-structure pins ─────────────────────────────────────────────────

func syncAgentScript(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "lib", "sync-agent.sh"))
	if err != nil {
		t.Skipf("lib/sync-agent.sh not available: %v", err)
	}
	return string(data)
}

// TestSyncAgentCDLineMatchesShellSource pins the two cd line numbers bash
// reports in its failing-cd diagnostic (lib/sync-agent.sh:108/:110).
func TestSyncAgentCDLineMatchesShellSource(t *testing.T) {
	src := syncAgentScript(t)
	for _, tc := range []struct {
		name   string
		needle string
		pin    int
	}{
		{"target", `TARGET_PATH="$(cd "$TARGET_PATH" && pwd)"`, syncAgentCDLineTarget},
		{"source", `SOURCE_ROOT="$(cd "$SOURCE_ROOT" && pwd)"`, syncAgentCDLineSource},
	} {
		found := 0
		for i, line := range strings.Split(src, "\n") {
			if strings.Contains(line, tc.needle) {
				found = i + 1
			}
		}
		if found == 0 {
			t.Fatalf("%s: could not locate the cd statement in lib/sync-agent.sh", tc.name)
		}
		if found != tc.pin {
			t.Errorf("%s: pinned line %d, but lib/sync-agent.sh:%d has the statement", tc.name, tc.pin, found)
		}
	}
}

// TestSyncAgentUsageMatchesShellHeredoc pins the byte-exact usage() body to
// the quoted heredoc in lib/sync-agent.sh.
func TestSyncAgentUsageMatchesShellHeredoc(t *testing.T) {
	src := syncAgentScript(t)
	start := strings.Index(src, "<<'EOF'\n")
	if start < 0 {
		t.Fatalf("usage heredoc not found in lib/sync-agent.sh")
	}
	body := src[start+len("<<'EOF'\n"):]
	end := strings.Index(body, "\nEOF\n")
	if end < 0 {
		t.Fatalf("usage heredoc terminator not found in lib/sync-agent.sh")
	}
	want := body[:end+1] // include the trailing newline of the last line
	if syncAgentUsage != want {
		t.Errorf("syncAgentUsage drifts from the lib/sync-agent.sh heredoc\n--- pinned heredoc ---\n%s--- go constant ---\n%s", want, syncAgentUsage)
	}
}

// ── Flag parsing ──────────────────────────────────────────────────────────

func TestParseAgentFlagsHelpPrintsUsageAndStops(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		var out, errOut bytes.Buffer
		_, code, done := parseAgentFlags([]string{flag}, &out, &errOut)
		if !done || code != 0 {
			t.Errorf("%s: done=%v code=%d, want done=true code=0", flag, done, code)
		}
		if out.String() != syncAgentUsage {
			t.Errorf("%s: stdout drifts from usage", flag)
		}
		if errOut.Len() != 0 {
			t.Errorf("%s: stderr = %q, want empty", flag, errOut.String())
		}
	}
}

func TestParseAgentFlagsUnknownFlagIsExit2(t *testing.T) {
	var out, errOut bytes.Buffer
	_, code, done := parseAgentFlags([]string{"--agentx"}, &out, &errOut)
	if !done || code != 2 {
		t.Errorf("done=%v code=%d, want done=true code=2", done, code)
	}
	want := "ERROR: unknown flag: --agentx\nRun 'ai-specs sync-agent --help' for usage.\n"
	if errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}

func TestParseAgentFlagsSecondPositionalIsExit2(t *testing.T) {
	var out, errOut bytes.Buffer
	opts, code, done := parseAgentFlags([]string{"a", "b"}, &out, &errOut)
	if !done || code != 2 {
		t.Errorf("done=%v code=%d, want done=true code=2", done, code)
	}
	if opts.target != "a" {
		t.Errorf("target = %q, want %q", opts.target, "a")
	}
	want := "ERROR: unexpected positional argument: b\n"
	if errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}

func TestParseAgentFlagsAcceptsEveryDocumentedFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	opts, code, done := parseAgentFlags([]string{
		"--source-root", "/r", "--target", "/t", "--recipe-mcp", "/m",
		"--resolved-config", "/c", "--resolved-hooks", "/h", "--adopt-brief",
		"--all", "--cursor", "--omp", "--verbose",
	}, &out, &errOut)
	if done || code != 0 {
		t.Fatalf("done=%v code=%d, want false/0", done, code)
	}
	if opts.sourceRoot != "/r" || !opts.explicitSourceRoot {
		t.Errorf("sourceRoot = %q explicit=%v, want /r true", opts.sourceRoot, opts.explicitSourceRoot)
	}
	if opts.target != "/t" || !opts.explicitTarget {
		t.Errorf("target = %q explicit=%v, want /t true", opts.target, opts.explicitTarget)
	}
	if opts.recipeMCP != "/m" || opts.resolvedConfig != "/c" || opts.resolvedHooks != "/h" {
		t.Errorf("seam paths not recorded: %+v", opts)
	}
	if !opts.adoptBrief || !opts.selectAll || !opts.verbose {
		t.Errorf("boolean flags not recorded: %+v", opts)
	}
	if strings.Join(opts.selected, ",") != "cursor,omp" {
		t.Errorf("selected = %v, want [cursor omp]", opts.selected)
	}
}

// TestParseAgentFlagsPositionalAfterTargetIsExit2 pins the shell's
// second-positional guard: --target already fills TARGET_PATH, so a
// positional after it is rejected (lib/sync-agent.sh:96-101).
func TestParseAgentFlagsPositionalAfterTargetIsExit2(t *testing.T) {
	var out, errOut bytes.Buffer
	opts, code, done := parseAgentFlags([]string{"--target", "/t", "/p"}, &out, &errOut)
	if !done || code != 2 {
		t.Fatalf("done=%v code=%d, want done=true code=2", done, code)
	}
	if opts.target != "/t" {
		t.Errorf("target = %q, want /t", opts.target)
	}
	want := "ERROR: unexpected positional argument: /p\n"
	if errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}

func TestParseAgentFlagsDashDashStopsParsing(t *testing.T) {
	var out, errOut bytes.Buffer
	opts, _, done := parseAgentFlags([]string{"proj", "--", "--all"}, &out, &errOut)
	if done {
		t.Errorf("done = true, want false")
	}
	if opts.target != "proj" || opts.selectAll {
		t.Errorf("opts = %+v, want target=proj selectAll=false", opts)
	}
}

// TestParseAgentFlagsFlagLastDiesSilently pins the shell's shift failure: a
// value-taking flag as the LAST argument makes `shift 2` fail under set -e
// (rc 1, no output, no writes — lib/sync-agent.sh:66-70; measured rc=1 with
// empty stderr). Reproducing it as an empty value instead would trigger
// writes where the oracle refuses (D20 defect class).
func TestParseAgentFlagsFlagLastDiesSilently(t *testing.T) {
	for _, flag := range []string{"--target", "--source-root", "--recipe-mcp", "--resolved-config", "--resolved-hooks"} {
		var out, errOut bytes.Buffer
		_, code, done := parseAgentFlags([]string{flag}, &out, &errOut)
		if !done || code != 1 {
			t.Errorf("%s: done=%v code=%d, want done=true code=1", flag, done, code)
		}
		if out.Len() != 0 || errOut.Len() != 0 {
			t.Errorf("%s: output not empty: stdout=%q stderr=%q", flag, out.String(), errOut.String())
		}
	}
}

// TestReadEnabledAgentsNormalizesLikeTomlRead pins the toml-read.py
// normalization (_normalize_string_list): keep only stripped non-empty
// strings; non-string items are DROPPED, not repr'd (lib/_internal/toml-read.py:47-58).
func TestReadEnabledAgentsNormalizesLikeTomlRead(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "ai-specs", "ai-specs.toml")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte("[project]\nname = 't'\n\n[agents]\nenabled = [' claude', 'pi ', 1, true, '']\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, rc := readEnabledAgents(manifest, io.Discard)
	if rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	want := []string{"claude", "pi"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("enabled = %q, want %q (toml-read drops non-strings and strips)", got, want)
	}
}

// ── Platform matrix ───────────────────────────────────────────────────────

// platformShBlocks splits lib/_internal/platform.sh into per-agent case
// blocks: "        claude)" starts a block that ends at the next agent line.
func platformShBlocks(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "lib", "_internal", "platform.sh"))
	if err != nil {
		t.Skipf("lib/_internal/platform.sh not available: %v", err)
	}
	src := string(data)
	re := regexp.MustCompile(`(?m)^        (\w+)\)$`)
	matches := re.FindAllStringSubmatchIndex(src, -1)
	if len(matches) == 0 {
		t.Fatalf("no agent case blocks found in platform.sh")
	}
	blocks := map[string]string{}
	for i, m := range matches {
		name := src[m[2]:m[3]]
		end := len(src)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		blocks[name] = src[m[1]:end]
	}
	return blocks
}

// TestAgentPlatformMatchesPlatformSh pins every agent × field value against
// the Go table. Drift in the shell matrix must fail here instead of silently
// breaking the fan-out.
func TestAgentPlatformMatchesPlatformSh(t *testing.T) {
	blocks := platformShBlocks(t)
	fields := []string{
		"instructions_path", "skills_dir", "mcp_config_path", "mcp_key",
		"native", "commands_dir", "runtime_hooks_target",
	}
	for agent, block := range blocks {
		for _, field := range fields {
			re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(field) + `\)\s*echo "([^"]*)"`)
			m := re.FindStringSubmatch(block)
			if m == nil {
				t.Errorf("agent %s: field %s not found in its platform.sh block", agent, field)
				continue
			}
			got, ok := platformGet(agent, field)
			if !ok {
				t.Errorf("agent %s: platformGet(%q) unknown", agent, field)
				continue
			}
			if got != m[1] {
				t.Errorf("agent %s field %s: go = %q, platform.sh = %q", agent, field, got, m[1])
			}
		}
	}
}

// TestAgentPlatformCoversEightAgents pins the agent set (selection order
// comes from the manifest/flags, not from the table).
func TestAgentPlatformCoversEightAgents(t *testing.T) {
	want := []string{"claude", "cursor", "opencode", "codex", "copilot", "gemini", "pi", "omp"}
	if len(agentPlatformMatrix) != len(want) {
		t.Fatalf("matrix has %d agents, want %d", len(agentPlatformMatrix), len(want))
	}
	for _, a := range want {
		if _, ok := platformGet(a, "native"); !ok {
			t.Errorf("agent %q missing from the matrix", a)
		}
	}
	if _, ok := platformGet("warp", "native"); ok {
		t.Errorf("unknown agent accepted by the matrix")
	}
}

// ── Symlink helpers ───────────────────────────────────────────────────────

func TestMakeRelativeSymlinkCreatesRelativeLink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(target, []byte("# brief\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".omp", "AGENTS.md")
	var out, errOut bytes.Buffer
	if rc := makeRelativeSymlink(target, link, &out, &errOut); rc != 0 {
		t.Fatalf("rc = %d, stderr=%s", rc, errOut.String())
	}
	if got, err := os.Readlink(link); err != nil || got != "../AGENTS.md" {
		t.Fatalf("link target = %q (%v), want ../AGENTS.md", got, err)
	}
	if out.String() != "    ✓ symlink created "+link+" → ../AGENTS.md\n" {
		t.Errorf("stdout = %q", out.String())
	}
}

// TestMirrorDirectoryReadOnlySource pins cp -R's mode application order: a
// read-only SOURCE directory still mirrors (cp writes children first and
// applies the source mode afterwards; measured rc=0 with dest 0555 and its
// contents present).
func TestMirrorDirectoryReadOnlySource(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "cache", "resolved-skills")
	if err := os.MkdirAll(filepath.Join(src, "skill-a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "skill-a", "SKILL.md"), []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(src, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(src, 0o755) })
	dest := filepath.Join(base, "proj", "ai-specs", "skills")
	// The mirrored destination ends read-only (cp semantics); make it
	// removable again before the TempDir cleanup runs.
	t.Cleanup(func() {
		filepath.WalkDir(dest, func(p string, d os.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				os.Chmod(p, 0o755)
			}
			return nil
		})
	})
	var errOut bytes.Buffer
	if rc := mirrorDirectory(src, dest, &errOut); rc != 0 {
		t.Fatalf("rc = %d, stderr=%s", rc, errOut.String())
	}
	data, err := os.ReadFile(filepath.Join(dest, "skill-a", "SKILL.md"))
	if err != nil || string(data) != "# a\n" {
		t.Fatalf("mirrored content missing: %q %v", data, err)
	}
	if info, err := os.Stat(dest); err != nil || info.Mode().Perm() != 0o555 {
		t.Errorf("dest mode = %v, want 0555 (cp applies the source mode)", info.Mode().Perm())
	}
}

func TestMakeRelativeSymlinkIdempotent(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(target, []byte("# brief\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "CLAUDE.md")
	var out bytes.Buffer
	if rc := makeRelativeSymlink(target, link, &out, io.Discard); rc != 0 {
		t.Fatalf("first rc = %d", rc)
	}
	out.Reset()
	if rc := makeRelativeSymlink(target, link, &out, io.Discard); rc != 0 {
		t.Fatalf("second rc = %d", rc)
	}
	if out.String() != "    · symlink ok      "+link+" → AGENTS.md\n" {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestMakeRelativeSymlinkRefusesNonSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(target, []byte("# brief\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "CLAUDE.md")
	if err := os.WriteFile(link, []byte("manual file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if rc := makeRelativeSymlink(target, link, &out, &errOut); rc != 1 {
		t.Fatalf("rc = %d, want 1", rc)
	}
	if errOut.String() != "    ✗ refuse to overwrite non-symlink: "+link+"\n" {
		t.Errorf("stderr = %q", errOut.String())
	}
	if data, err := os.ReadFile(link); err != nil || string(data) != "manual file\n" {
		t.Errorf("occupied file was modified: %q %v", data, err)
	}
}

// TestMakeRelativeSymlinkOsFailureContinues pins the shell's errexit-off
// behavior inside sync_one_agent: mkdir/ln failures print the coreutils-style
// error, the helper still echoes `✓ symlink created` and returns 0
// (lib/sync-agent.sh:318-355 have no `|| return`; measured rc=0 with the
// failure lines on stderr and the ✓ line on stdout).
// TestMakeRelativeSymlinkOsFailureContinues pins the shell's errexit-off
// behavior inside sync_one_agent: mkdir/ln failures print the coreutils-style
// error, the helper still echoes `✓ symlink created` and returns 0
// (lib/sync-agent.sh:318-355 have no `|| return`; measured rc=0 with the
// failure lines on stderr and the ✓ line on stdout). Both measured mkdir -p
// shapes are pinned: an existing non-directory LEAF is `File exists`
// (measured: `mkdir: o1/file: File exists`), an INTERMEDIATE component is
// `Not a directory` (measured: `mkdir: o1/file: Not a directory` for a deeper
// argument); ln prints only the linkpath (measured: `ln: .pi/skills: Not a
// directory`).
func TestMakeRelativeSymlinkOsFailureContinues(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(target, []byte("# brief\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Leaf shape (omp agent instructions): .omp is a regular file, so
	// mkdir -p <root>/.omp reports File exists and ln -s ... .omp/AGENTS.md
	// reports Not a directory; the helper still echoes ✓ and returns 0.
	if err := os.WriteFile(filepath.Join(root, ".omp"), []byte("occupied\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".omp", "AGENTS.md")
	var out, errOut bytes.Buffer
	if rc := makeRelativeSymlink(target, link, &out, &errOut); rc != 0 {
		t.Fatalf("rc = %d, want 0 (the shell continues after mkdir/ln failures)", rc)
	}
	if !strings.Contains(errOut.String(), "mkdir: "+filepath.Join(root, ".omp")+": File exists\n") {
		t.Errorf("stderr missing coreutils mkdir line: %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "ln: "+link+": Not a directory\n") {
		t.Errorf("stderr missing coreutils ln line: %q", errOut.String())
	}
	if out.String() != "    ✓ symlink created "+link+" → ../AGENTS.md\n" {
		t.Errorf("stdout = %q (the ✓ line must still be emitted)", out.String())
	}

	// Pi-agent skills shape (measured oracle bytes: mkdir -p <root>/.pi —
	// the linkDir IS the occupied leaf — reports File exists; ln reports
	// the linkpath with Not a directory).
	root2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(root2, ".pi"), []byte("occupied\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillsLink := filepath.Join(root2, ".pi", "skills")
	var out2, errOut2 bytes.Buffer
	if rc := makeSkillsSymlink(target, skillsLink, &out2, &errOut2); rc != 0 {
		t.Fatalf("rc = %d, want 0", rc)
	}
	if !strings.Contains(errOut2.String(), "mkdir: "+filepath.Join(root2, ".pi")+": File exists\n") {
		t.Errorf("stderr2 missing mkdir line: %q", errOut2.String())
	}
	if !strings.Contains(errOut2.String(), "ln: "+skillsLink+": Not a directory\n") {
		t.Errorf("stderr2 missing ln line: %q", errOut2.String())
	}
	if !strings.Contains(out2.String(), "    ✓ symlink created "+skillsLink+" → ") {
		t.Errorf("stdout2 = %q", out2.String())
	}
}

// TestMkdirPMirrorShapes pins mkdirPMirror's two measured `mkdir -p` shapes
// directly: an existing non-directory LEAF is File exists, an INTERMEDIATE
// component is Not a directory (measured on this platform's BSD mkdir).
func TestMkdirPMirrorShapes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var errOut bytes.Buffer
	err := mkdirPMirror(filepath.Join(root, "file"))
	if err == nil {
		t.Fatal("leaf case: want an error")
	}
	printChildErr(&errOut, "mkdir", err)
	if !strings.Contains(errOut.String(), "mkdir: "+filepath.Join(root, "file")+": File exists\n") {
		t.Errorf("leaf: %q", errOut.String())
	}
	errOut.Reset()
	err = mkdirPMirror(filepath.Join(root, "file", "sub"))
	if err == nil {
		t.Fatal("intermediate case: want an error")
	}
	printChildErr(&errOut, "mkdir", err)
	if !strings.Contains(errOut.String(), "mkdir: "+filepath.Join(root, "file")+": Not a directory\n") {
		t.Errorf("intermediate: %q", errOut.String())
	}
	// A symlink to a directory counts as a directory, like mkdir -p.
	if err := os.MkdirAll(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := mkdirPMirror(filepath.Join(root, "link", "inside")); err != nil {
		t.Errorf("symlink-to-dir: unexpected error %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "real", "inside")); err != nil {
		t.Errorf("directory not created through the symlink: %v", err)
	}
}

func TestMakeSkillsSymlinkUsesAbsoluteRealpathTarget(t *testing.T) {
	// The cache dir sits behind a symlink to prove the target is realpath'd
	// (os.path.realpath resolves /var → /private/var on this platform).
	base := t.TempDir()
	realCache := filepath.Join(base, "real-cache")
	if err := os.MkdirAll(realCache, 0o755); err != nil {
		t.Fatal(err)
	}
	linkHome := filepath.Join(base, "link-home")
	if err := os.Symlink(realCache, linkHome); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(linkHome, "resolved-skills")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	want := projectcache.ResolvePath(source)
	project := t.TempDir()
	link := filepath.Join(project, ".claude", "skills")
	var out, errOut bytes.Buffer
	if rc := makeSkillsSymlink(source, link, &out, &errOut); rc != 0 {
		t.Fatalf("rc = %d, stderr=%s", rc, errOut.String())
	}
	got, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("link target = %q, want the realpath %q", got, want)
	}
	if out.String() != "    ✓ symlink created "+link+" → "+want+"\n" {
		t.Errorf("stdout = %q", out.String())
	}
}

// ── sync_one_agent semantics ──────────────────────────────────────────────

// newAgentRun builds a minimal single-agent run against a temp project: a
// manifest enabling claude, an AGENTS.md, cache-shaped source dirs, and no
// recipe-mcp/hooks unless the caller overrides them. No python3 is needed:
// every step the body reaches is a native port.
func newAgentRun(t *testing.T, project string) *agentRun {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(project, "ai-specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(project, "ai-specs", "ai-specs.toml")
	if err := os.WriteFile(manifest, []byte("[project]\nname = 't'\n\n[agents]\nenabled = ['claude']\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("# brief\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skills := filepath.Join(project, "cache", "resolved-skills")
	commands := filepath.Join(project, "cache", "merged-commands")
	for _, dir := range []string{filepath.Join(skills, "demo"), commands} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(skills, "demo", "SKILL.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commands, "demo.md"), []byte("# demo command\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &agentRun{
		r:                 &runner{home: project, env: childEnv(project)},
		sourceRoot:        project,
		targetPath:        project,
		tomlPath:          manifest,
		skillsSource:      skills,
		commandsSource:    commands,
		resolvedSkillsDir: skills,
		mergedCommandsDir: commands,
		enabledAgents:     []string{"claude"},
	}
}

func TestSyncOneAgentUnknownAgentIsSuccessNotice(t *testing.T) {
	run := newAgentRun(t, t.TempDir())
	var out, errOut bytes.Buffer
	if rc := run.syncOneAgent("warp", &out, &errOut); rc != 0 {
		t.Fatalf("rc = %d, want 0 (unknown agents never fail the step)", rc)
	}
	if errOut.String() != "  ✗ unknown agent: warp\n" {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestSyncOneAgentNotEnabledWarnsAndSyncs(t *testing.T) {
	run := newAgentRun(t, t.TempDir())
	var out, errOut bytes.Buffer
	if rc := run.syncOneAgent("cursor", &out, &errOut); rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	if !strings.HasPrefix(out.String(), "  ! cursor not in [agents].enabled — syncing anyway\n") {
		t.Errorf("stdout = %q", out.String())
	}
	if _, err := os.Readlink(filepath.Join(run.targetPath, ".cursor", "skills")); err != nil {
		t.Errorf("cursor skills link not created: %v", err)
	}
}

func TestSyncOneAgentClaudeFullFanout(t *testing.T) {
	run := newAgentRun(t, t.TempDir())
	var out, errOut bytes.Buffer
	if rc := run.syncOneAgent("claude", &out, &errOut); rc != 0 {
		t.Fatalf("rc = %d, stderr=%s", rc, errOut.String())
	}
	// Instructions link is RELATIVE.
	if got, _ := os.Readlink(filepath.Join(run.targetPath, "CLAUDE.md")); got != "AGENTS.md" {
		t.Errorf("CLAUDE.md link = %q, want AGENTS.md", got)
	}
	// Skills link is ABSOLUTE into the cache (realpath'd).
	if got, _ := os.Readlink(filepath.Join(run.targetPath, ".claude", "skills")); got != projectcache.ResolvePath(run.skillsSource) {
		t.Errorf("skills link = %q, want %q", got, projectcache.ResolvePath(run.skillsSource))
	}
	// Managed command copied; D3' count line emitted.
	data, err := os.ReadFile(filepath.Join(run.targetPath, ".claude", "commands", "demo.md"))
	if err != nil || string(data) != "# demo command\n" {
		t.Errorf("managed command not copied: %q %v", data, err)
	}
	for _, line := range []string{
		"    ✓ symlink created " + run.targetPath + "/CLAUDE.md → AGENTS.md\n",
		"    ℹ mcp skipped (no [mcp.*] in manifest)\n",
		"    ✓ commands     .claude/commands/ (1 file(s))\n",
	} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("stdout missing %q in:\n%s", line, out.String())
		}
	}
}

func TestSyncOneAgentMcpSkippedNotice(t *testing.T) {
	run := newAgentRun(t, t.TempDir())
	var out, errOut bytes.Buffer
	if rc := run.syncOneAgent("claude", &out, &errOut); rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	if !strings.Contains(out.String(), "    ℹ mcp skipped (no [mcp.*] in manifest)\n") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestSyncOneAgentMcpRendersWhenCountPositive(t *testing.T) {
	run := newAgentRun(t, t.TempDir())
	// The manifest must declare [mcp.*]: MCP_COUNT gates the call, but the
	// renderer itself skips agents when the manifest has no mcp entries.
	if err := os.WriteFile(run.tomlPath, []byte("[project]\nname = 't'\n\n[agents]\nenabled = ['claude']\n\n[mcp.alpha]\ncommand = 'npx'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recipeMCP := filepath.Join(run.targetPath, "recipe-mcp.json")
	if err := os.WriteFile(recipeMCP, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run.mcpCount = 2
	run.recipeMCP = recipeMCP
	var out, errOut bytes.Buffer
	if rc := run.syncOneAgent("claude", &out, &errOut); rc != 0 {
		t.Fatalf("rc = %d, stderr=%s", rc, errOut.String())
	}
	data, err := os.ReadFile(filepath.Join(run.targetPath, ".mcp.json"))
	if err != nil {
		t.Fatalf("mcp file not written: %v", err)
	}
	if !strings.Contains(string(data), "mcpServers") || !strings.Contains(string(data), "alpha") {
		t.Errorf(".mcp.json = %q", data)
	}
}

func TestSyncOneAgentCommandsPreserveNonManagedFiles(t *testing.T) {
	run := newAgentRun(t, t.TempDir())
	cmdDir := filepath.Join(run.targetPath, ".claude", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cmdDir, "user-own.md"), []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if rc := run.syncOneAgent("claude", &out, &errOut); rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	want := "    ! preserved non-managed file .claude/commands/user-own.md (move it to ai-specs/commands/ to manage it)\n"
	if !strings.Contains(errOut.String(), want) {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
	data, err := os.ReadFile(filepath.Join(cmdDir, "user-own.md"))
	if err != nil || string(data) != "# mine\n" {
		t.Errorf("user file not preserved: %q %v", data, err)
	}
	if !strings.Contains(out.String(), "    ✓ commands     .claude/commands/ (1 file(s))\n") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestSyncOneAgentCommandsNeverWriteThroughSymlink(t *testing.T) {
	run := newAgentRun(t, t.TempDir())
	cmdDir := filepath.Join(run.targetPath, ".claude", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(run.targetPath, "outside.md")
	if err := os.WriteFile(outside, []byte("victim\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(cmdDir, "demo.md")); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if rc := run.syncOneAgent("claude", &out, &errOut); rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	info, err := os.Lstat(filepath.Join(cmdDir, "demo.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("managed name still occupied by a symlink")
	}
	if data, err := os.ReadFile(filepath.Join(cmdDir, "demo.md")); err != nil || string(data) != "# demo command\n" {
		t.Errorf("managed file not materialized: %q %v", data, err)
	}
}

func TestSyncOneAgentHooksOnlyWithResolvedHooks(t *testing.T) {
	// D22: without --resolved-hooks nothing hook-related runs, and the step
	// still succeeds.
	run := newAgentRun(t, t.TempDir())
	var out, errOut bytes.Buffer
	if rc := run.syncOneAgent("claude", &out, &errOut); rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	if strings.Contains(out.String(), "runtime hooks") {
		t.Errorf("hooks rendered without --resolved-hooks: %q", out.String())
	}
}

// ── ensure_target_workspace + D24 ─────────────────────────────────────────

func TestEnsureTargetWorkspaceSubrepoReceivesArtifacts(t *testing.T) {
	root := t.TempDir()
	run := newAgentRun(t, root)
	sub := filepath.Join(root, "packages", "a")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	run.targetPath = sub
	var out, errOut bytes.Buffer
	if rc := run.ensureTargetWorkspace(&out, &errOut); rc != 0 {
		t.Fatalf("rc = %d, stderr=%s", rc, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(sub, "ai-specs", ".gitignore")); err != nil {
		t.Errorf("subrepo gitignore missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sub, "ai-specs", "skills", "demo", "SKILL.md")); err != nil {
		t.Errorf("subrepo mirrored skill missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sub, "AGENTS.md")); err != nil {
		t.Errorf("subrepo AGENTS.md missing: %v", err)
	}
}

func TestEnsureTargetWorkspaceSameRootRequiresAgentsMD(t *testing.T) {
	root := t.TempDir()
	run := newAgentRun(t, root)
	if err := os.Remove(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if rc := run.ensureTargetWorkspace(&out, &errOut); rc != 1 {
		t.Fatalf("rc = %d, want 1", rc)
	}
	want := "ERROR: " + filepath.Join(root, "AGENTS.md") + " not found. Run 'ai-specs init " + root + "' first.\n"
	if errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}

func TestEnsureTargetWorkspaceBriefRenderFalseKeepsExistingAgentsMD(t *testing.T) {
	root := t.TempDir()
	run := newAgentRun(t, root)
	manifest := filepath.Join(root, "ai-specs", "ai-specs.toml")
	if err := os.WriteFile(manifest, []byte("[project]\nname = 't'\n\n[agents]\nenabled = ['claude']\n\n[brief]\nrender = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "AGENTS.md"), []byte("# hand written\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run.tomlPath = manifest
	run.targetPath = sub
	var out, errOut bytes.Buffer
	if rc := run.ensureTargetWorkspace(&out, &errOut); rc != 0 {
		t.Fatalf("rc = %d, stderr=%s", rc, errOut.String())
	}
	if !strings.Contains(out.String(), "    ℹ skipped AGENTS.md (brief.render = false)\n") {
		t.Errorf("stdout = %q", out.String())
	}
	data, err := os.ReadFile(filepath.Join(sub, "AGENTS.md"))
	if err != nil || string(data) != "# hand written\n" {
		t.Errorf("existing AGENTS.md was touched: %q %v", data, err)
	}
}

func TestAgentRunD24NoAgentsEnsuresWorkspaceThenExitZero(t *testing.T) {
	root := t.TempDir()
	run := newAgentRun(t, root)
	if err := os.WriteFile(filepath.Join(root, "ai-specs", "ai-specs.toml"), []byte("[project]\nname = 't'\n\n[agents]\nenabled = []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run.enabledAgents = nil
	var out, errOut bytes.Buffer
	run.out, run.errW = &out, &errOut
	if rc := run.runBody(); rc != 0 {
		t.Fatalf("rc = %d, want 0 (D24)", rc)
	}
	if errOut.String() != "WARNING: no agents to sync. Set [agents].enabled in ai-specs.toml.\n" {
		t.Errorf("stderr = %q", errOut.String())
	}
	if strings.Contains(out.String(), "ai-specs sync-agent") {
		t.Errorf("D24 must return before the banner: %q", out.String())
	}
}

// ── Framing (banner/footer) and nested suppression ────────────────────────

func TestAgentRunStandaloneSingleTargetPrintsFraming(t *testing.T) {
	root := t.TempDir()
	run := newAgentRun(t, root)
	var out, errOut bytes.Buffer
	run.out, run.errW = &out, &errOut
	if rc := run.runBody(); rc != 0 {
		t.Fatalf("rc = %d, stderr=%s", rc, errOut.String())
	}
	for _, needle := range []string{
		"\nai-specs sync-agent\n",
		"  source root: " + root + "\n",
		"  target:      " + root + "\n",
		"  agents:      claude\n",
		"  enabled:     claude\n",
		"  mcp:         0 server(s)\n",
		"  derived artifacts: AGENTS.md, ai-specs/.gitignore, ai-specs/skills/**, ai-specs/commands/**, agent-configs\n",
		"\n✓ sync-agent complete\n",
	} {
		if !strings.Contains(out.String(), needle) {
			t.Errorf("framing missing %q in:\n%s", needle, out.String())
		}
	}
}

func TestAgentRunNestedSuppressesFraming(t *testing.T) {
	root := t.TempDir()
	run := newAgentRun(t, root)
	run.nested = true
	var out, errOut bytes.Buffer
	run.out, run.errW = &out, &errOut
	if rc := run.runBody(); rc != 0 {
		t.Fatalf("rc = %d, stderr=%s", rc, errOut.String())
	}
	if strings.Contains(out.String(), "ai-specs sync-agent") ||
		strings.Contains(out.String(), "sync-agent complete") {
		t.Errorf("nested run leaked framing:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "  syncing claude\n") {
		t.Errorf("nested run lost the step label:\n%s", out.String())
	}
}

// ── Cache path oracle ─────────────────────────────────────────────────────

// TestCachePathOracleMatchesProjectCachePy compares the Go cache-path
// derivation with the Python module for the two kinds the fan-out consumes.
func TestCachePathOracleMatchesProjectCachePy(t *testing.T) {
	python3, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("AI_SPECS_HOME", home)
	for _, kind := range []string{"resolved-skills", "root"} {
		cmd := exec.Command(python3, filepath.Join("..", "..", "lib", "_internal", "project-cache.py"), project, "path", kind)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("project-cache.py path %s: %v", kind, err)
		}
		want := strings.TrimSpace(string(out))
		got := ""
		switch kind {
		case "resolved-skills":
			got = projectcache.ResolvedSkillsDir(project, home)
		case "root":
			got = projectcache.CacheRoot(project, home)
		}
		if got != want {
			t.Errorf("kind %s: go = %q, python = %q", kind, got, want)
		}
	}
}

// ── Remediation batch 2 (JD follow-ups, human-authorized) ─────────────────

// TestAgentsReadLoopSplitting pins the full enabled-agents pipeline:
// toml-read strip → print(a) per element → `while IFS= read -r`. Measured
// against the actual bash read loop over the printed bytes: internal blank
// lines are dropped, a CR is retained inside a piece, whitespace-only
// INTERNAL pieces survive, and the information separators strip like space.
func TestAgentsReadLoopSplitting(t *testing.T) {
	raw := []any{
		"a\n\nb", "x\n", "a\r\nb", "a\n  \nb",
		"\x1cclaude", "pad ", "a b", "q\r", "  ",
	}
	want := []string{"a", "b", "x", "a\r", "b", "a", "  ", "b", "claude", "pad", "a b", "q"}
	got := normalizedStringList(raw)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("normalizedStringList = %q, want %q (measured while-read oracle)", got, want)
	}
}

// TestPyStrStripSeparators pins Python str.strip()'s isspace set: the
// information separators U+001C–U+001F strip like space (measured RED:
// "\x1cclaude" synced in legacy but reached platformGet unstripped in Go).
func TestPyStrStripSeparators(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"\x1cclaude", "claude"},
		{"\x1d\x1ex\x1f", "x"},
		{"\u0085x\u00a0", "x"},
		{"a\x1cb", "a\x1cb"}, // internal separator survives (strip trims ends only)
	} {
		if got := pyStrStrip(tc.in); got != tc.want {
			t.Errorf("pyStrStrip(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestMCPCountMatrix pins mcpServerCount's recipe-mcp len/catch behavior to
// the measured oracle (lib/sync-agent.sh:412-425) AND the human-accepted
// portable class diagnostics (final bounded round F3): missing file and
// malformed/empty JSON count as empty; dict/list/string contribute their len
// (string in code points); a non-ENOENT read error (directory) dies with the
// IsADirectoryError class; number/bool/null die with the TypeError class —
// the two death classes are DISTINCT and neither invents a meaningless
// message.
func TestMCPCountMatrix(t *testing.T) {
	run := newAgentRun(t, t.TempDir())
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cases := []struct {
		name      string
		path      string
		want      int
		wantErr   bool
		wantClass string // substring the diagnostic MUST contain
		forbidden string // invented/leaked text the diagnostic MUST NOT contain
	}{
		{"missing", filepath.Join(dir, "nope.json"), 0, false, "", ""},
		{"malformed", write("bad.json", "not json"), 0, false, "", ""},
		{"empty", write("empty.json", ""), 0, false, "", ""},
		{"object", write("obj.json", `{"r":{"s":1}}`), 1, false, "", ""},
		{"list", write("list.json", "[1,2]"), 2, false, "", ""},
		{"string", write("str.json", `"abc"`), 3, false, "", ""},
		{"string-runes", write("runes.json", `"aé"`), 2, false, "", ""},
		{"number", write("num.json", "123"), 0, true, "TypeError", "object of unsized type"},
		{"bool", write("bool.json", "true"), 0, true, "TypeError", ""},
		{"null", write("null.json", "null"), 0, true, "TypeError", ""},
		{"directory", dir, 0, true, "IsADirectoryError", "object of unsized type"},
	}
	seenClasses := map[string]bool{}
	for _, tc := range cases {
		run.recipeMCP = tc.path
		got, err := run.mcpServerCount()
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: want death (uncaught in oracle), got count %d", tc.name, got)
				continue
			}
			if tc.wantClass != "" && !strings.Contains(err.Error(), tc.wantClass) {
				t.Errorf("%s: diagnostic %q lacks the correct class %q", tc.name, err.Error(), tc.wantClass)
			}
			if tc.forbidden != "" && strings.Contains(err.Error(), tc.forbidden) {
				t.Errorf("%s: diagnostic %q contains forbidden text %q", tc.name, err.Error(), tc.forbidden)
			}
			if tc.wantClass != "" {
				seenClasses[tc.wantClass] = true
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: count = %d, want %d", tc.name, got, tc.want)
		}
	}
	if len(seenClasses) < 2 {
		t.Errorf("the IsADirectoryError/read-error and TypeError classes must be DISTINCT; seen: %v", seenClasses)
	}
}

// TestRemoveTreeUnreadableDirSilentSuccess pins the SANCTIONED-MEASURED
// oracle semantics for the unreadable-dir shape (parity fixture
// sync-agent-unreadable-mirror, which the harness measured against the live
// legacy rm): a 0333 EMPTY directory is removed SILENTLY — no stderr, rc 0,
// dir gone (rm attempts the removal without reporting the read failure).
// The manual rm probe was safety-blocked and honestly reported; this unit
// test pins the harness-measured outcome.
func TestRemoveTreeUnreadableDirSilentSuccess(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "skills")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o333); err != nil {
		t.Fatal(err)
	}
	var errOut bytes.Buffer
	if rc := removeTree(target, &errOut); rc != 0 {
		t.Errorf("rc = %d, want 0 (measured: silent success)", rc)
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want empty (measured: no report)", errOut.String())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("dir should be removed, stat err = %v", err)
	}
}

// TestSyncCommandsCpAbortShape pins the measured oracle bytes for the managed
// `cp ... || return $?` (RED probe P5): `cp: <dest>: Permission denied` and
// rc 1 — the cp binary names the DEST when open-for-write fails.
func TestSyncCommandsCpAbortShape(t *testing.T) {
	run := newAgentRun(t, t.TempDir())
	cmdDir := filepath.Join(run.targetPath, ".claude", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cmdDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(cmdDir, 0o755) })
	var out, errOut bytes.Buffer
	if rc := run.syncOneAgent("claude", &out, &errOut); rc != 1 {
		t.Fatalf("rc = %d, want 1", rc)
	}
	want := "cp: " + filepath.Join(cmdDir, "demo.md") + ": Permission denied\n"
	if errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}

// TestMktempFailureReplaysStderr pins the fallback mktemp's death behavior:
// the oracle's `$(mktemp ...)` lets the child's stderr pass through before
// the set -e death (rc 1, nothing else runs). The live trigger is
// unreachable on this platform (macOS `mktemp -t` ignores TMPDIR), so this
// uses the established PATH-stub seam — stub/oracle justification recorded
// here, NOT claimed as a live RED.
func TestMktempFailureReplaysStderr(t *testing.T) {
	bin := t.TempDir()
	stub := filepath.Join(bin, "mktemp")
	script := "#!/bin/sh\necho 'mktemp: failed to create file via template: stub-die' >&2\nexit 1\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ai-specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ai-specs", "ai-specs.toml"), []byte("[project]\nname = 't'\n\n[agents]\nenabled = ['claude']\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// exec.Command resolves via the process PATH, so the stub is seeded with
	// t.Setenv (both LookPath and the child env see it first).
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	r := &runner{home: root, env: childEnv(root)}
	opts := agentOptions{sourceRoot: root, target: root, selectAll: true}
	var out, errOut bytes.Buffer
	if _, rc := buildAgentRun(r, opts, agentStreams{out: &out, errW: &errOut}, false); rc != 1 {
		t.Fatalf("rc = %d, want 1", rc)
	}
	if !strings.Contains(errOut.String(), "mktemp: failed to create file via template: stub-die\n") {
		t.Errorf("stderr = %q (want mktemp's own diagnostic replayed)", errOut.String())
	}
	if strings.Contains(out.String(), "  syncing flatten resolved skills") {
		t.Errorf("death must happen before any step work: %q", out.String())
	}
}

// TestStandalonePlanDiagnostics pins the STABLE PORTABLE diagnostics for the
// standalone plan extraction (PATH-stubbed python3; human decision
// 2026-10-04): a malformed plan body dies with the JSONDecodeError-class
// diagnostic, a plan without "root" dies with the KeyError-class diagnostic,
// both BEFORE any write or banner. The bytes are environment-independent by
// design — the portability guard below rejects any local python install
// path, version string or traceback frame leak.
func TestStandalonePlanDiagnostics(t *testing.T) {
	stubDir := t.TempDir()
	stub := filepath.Join(stubDir, "python3")
	writeStub := func(payload string) {
		script := "#!/bin/sh\nif printf '%s\\n' \"$*\" | grep -q target-resolve.py; then\ncat <<'PYEOF'\n" + payload + "\nPYEOF\nexit 0\nfi\nexit 0\n"
		if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	runWithStub := func(t *testing.T, payload string) (int, string, string) {
		writeStub(payload)
		root := t.TempDir()
		var out, errOut bytes.Buffer
		t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
		rc := RunAgent([]string{"sync-agent", root}, root, nil, &out, &errOut)
		return rc, out.String(), errOut.String()
	}
	assertPortable := func(t *testing.T, stderr string) {
		t.Helper()
		for _, banned := range []string{"/opt/homebrew", "/usr/local", "/Library/", "Cellar", "Frameworks", "Traceback", "3.14", "3.13", "3.9"} {
			if strings.Contains(stderr, banned) {
				t.Errorf("diagnostic leaks environment-specific bytes %q: %q", banned, stderr)
			}
		}
	}

	t.Run("malformed plan body", func(t *testing.T) {
		rc, out, errOut := runWithStub(t, "not json")
		if rc != 1 {
			t.Fatalf("rc = %d, want 1", rc)
		}
		if errOut != pyJSONDecodeErrPlanBytes {
			t.Errorf("stderr = %q\nwant = %q", errOut, pyJSONDecodeErrPlanBytes)
		}
		assertPortable(t, errOut)
		if strings.Contains(out, "ai-specs sync-agent") {
			t.Errorf("banner must not print: %q", out)
		}
	})
	t.Run("missing root", func(t *testing.T) {
		rc, out, errOut := runWithStub(t, `{"targets":[{"path":"x"},{"path":"y"}]}`)
		if rc != 1 {
			t.Fatalf("rc = %d, want 1", rc)
		}
		if errOut != pyKeyErrRootBytes {
			t.Errorf("stderr = %q\nwant = %q", errOut, pyKeyErrRootBytes)
		}
		assertPortable(t, errOut)
		if strings.Contains(out, "syncing") {
			t.Errorf("no writes may happen: %q", out)
		}
	})
	t.Run("classes are distinguishable", func(t *testing.T) {
		if pyJSONDecodeErrPlanBytes == pyKeyErrRootBytes {
			t.Errorf("the two failure classes must stay distinguishable")
		}
	})
}

// TestMarkerTempPathOracleBytes pins markerTempPath against the MEASURED
// bash pipeline `$(grep '^RECIPE_MCP_TEMP:' f | cut -d: -f2-)` (command
// substitution strips trailing newlines only): trailing blank lines vanish,
// an unterminated file still matches, multiple markers join with \n, and a
// trailing space is KEPT (cut only splits on colons).
func TestMarkerTempPathOracleBytes(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"plain", "RECIPE_MCP_TEMP:/x/a.json\n", "/x/a.json"},
		{"trailing blanks", "RECIPE_MCP_TEMP:/x/a.json\n\n\n", "/x/a.json"},
		{"no trailing newline", "RECIPE_MCP_TEMP:/x/a.json", "/x/a.json"},
		{"two markers", "warn\nRECIPE_MCP_TEMP:/x/a.json\nRECIPE_MCP_TEMP:/x/b.json\n", "/x/a.json\n/x/b.json"},
		{"trailing space kept", "RECIPE_MCP_TEMP:/x/a.json \n", "/x/a.json "},
	}
	for _, tc := range cases {
		p := filepath.Join(dir, tc.name)
		if err := os.WriteFile(p, []byte(tc.content), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := markerTempPath(p); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestPrintStepOutputOracleBytes pins printStepOutput and replayRaw against
// the MEASURED bash print_step_output + `[[ -s ]] && cat` oracle over crafted
// capture files (including the judge-B empty/blank-line cases): verbose is
// cat, compact drops blank and ✓·⇢▸ lines, a final unterminated line gains
// its newline, an all-blank capture prints nothing, and the failure replay
// prints empty captures as nothing and blank-only captures as the bare
// newline.
func TestPrintStepOutputOracleBytes(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content string
	}{
		{"label", "  syncing x\n"},
		{"blanks-then-warn", "\n\n  ! warn\n"},
		{"no-trailing-newline", "no-trailing"},
		{"blank-only", "\n"},
		{"empty", ""},
		{"success-detail", "    ✓ ok\n    · noise\n  ✗ keep\n"},
	}
	for _, tc := range cases {
		p := filepath.Join(dir, tc.name)
		if err := os.WriteFile(p, []byte(tc.content), 0o644); err != nil {
			t.Fatal(err)
		}
		var got bytes.Buffer
		printStepOutput(&got, p, true)
		if got.String() != tc.content {
			t.Errorf("%s verbose: got %q, want verbatim %q", tc.name, got.String(), tc.content)
		}
	}
	// Compact: blanks and ✓/· lines dropped, ✗ and content lines kept, a
	// final unterminated line gains its newline.
	p := filepath.Join(dir, "mixed")
	content := "\n  syncing x\n    ✓ ok\n  ✗ keep\n\nno-eol"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	printStepOutput(&got, p, false)
	want := "  syncing x\n  ✗ keep\nno-eol\n"
	if got.String() != want {
		t.Errorf("compact: got %q, want %q", got.String(), want)
	}
	// Failure replay: empty capture prints nothing, blank-only prints "\n".
	var out, errOut bytes.Buffer
	replayRaw(&out, filepath.Join(dir, "empty"))
	if out.Len() != 0 {
		t.Errorf("empty replay: %q", out.String())
	}
	replayRaw(&errOut, filepath.Join(dir, "blank-only"))
	if errOut.String() != "\n" {
		t.Errorf("blank replay: %q", errOut.String())
	}
}

// ── Maintainer closure round (distinct human authorization) ───────────────

// TestMCPReadClass pins the errno→Python-exception-class mapping for the
// recipe-mcp fatal diagnostic (maintainer closure MC1): the OSError
// subclasses open() actually raises — EISDIR → IsADirectoryError, ENOTDIR →
// NotADirectoryError, EACCES and EPERM → PermissionError — with any other
// errno falling back to OSError. RED at closure time: ENOTDIR and EPERM were
// unmapped and fell through to "OSError".
func TestMCPReadClass(t *testing.T) {
	cases := []struct {
		errno syscall.Errno
		want  string
	}{
		{syscall.EISDIR, "IsADirectoryError"},
		{syscall.ENOTDIR, "NotADirectoryError"},
		{syscall.EACCES, "PermissionError"},
		{syscall.EPERM, "PermissionError"},
	}
	for _, tc := range cases {
		if got := mcpReadClass(tc.errno); got != tc.want {
			t.Errorf("mcpReadClass(%s) = %q, want %q", tc.errno, got, tc.want)
		}
	}
	if got := mcpReadClass(syscall.EIO); got != "OSError" {
		t.Errorf("mcpReadClass(EIO) = %q, want OSError (fallback)", got)
	}
}

// TestManifestFatalPathContractPinned pins the manifest fatal-error path
// OUTSIDE the recipe-mcp tolerance (maintainer closure MC2): a malformed
// MANIFEST kills the run rc 1 with the ESTABLISHED `error: …` prefix/format —
// the pre-existing documented deviation for Python tracebacks — before any
// banner or write. This is a CONTRACT PIN of current behavior, not a RED:
// byte-exact reproduction of the manifest traceback format was judged to
// need a new product decision and is reported blocked, not invented.
func TestManifestFatalPathContractPinned(t *testing.T) {
	root := t.TempDir()
	run := newAgentRun(t, root)
	// Overwrite AFTER the constructor so the malformed manifest is what the
	// run actually reads.
	if err := os.WriteFile(filepath.Join(root, "ai-specs", "ai-specs.toml"), []byte("[agents\nenabled = ['claude']\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	run.out, run.errW = &out, &errOut
	if rc := run.runBody(); rc != 1 {
		t.Fatalf("rc = %d, want 1", rc)
	}
	if !strings.HasPrefix(errOut.String(), "error: ") {
		t.Errorf("stderr = %q, want the established `error: ` prefix", errOut.String())
	}
	if strings.Contains(out.String(), "ai-specs sync-agent") {
		t.Errorf("banner must not print: %q", out.String())
	}
	if strings.Contains(out.String(), "  syncing ") {
		t.Errorf("no step work may happen: %q", out.String())
	}
}
