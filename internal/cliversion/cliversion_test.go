package cliversion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
	}{
		{"0.12.2", "0.12.3", -1},
		{"0.12.3", "0.12.2", 1},
		{"0.12.2", "0.12.2", 0},
		{"0.12.2-rc1", "0.12.2", -1},
		{"0.12.2", "0.12.2-rc1", 1},
		{"0.12.2+build", "0.12.2", 0},
		{"1.0.0", "0.9.9", 1},
		{"unknown", "0.12.2", -1},
		{"0.12.2", "unknown", 1},
		{"bad", "bad", 0},
	}
	for _, c := range cases {
		if got := CompareVersions(c.left, c.right); got != c.want {
			t.Errorf("CompareVersions(%q,%q) = %d, want %d", c.left, c.right, got, c.want)
		}
	}
}

func TestParseToolPolicy(t *testing.T) {
	cases := []struct {
		toml    string
		kind    string
		version string
		err     string
	}{
		{"[tool]\nversion='0.12.2'\n", "exact", "0.12.2", ""},
		{"[tool]\nmin_version='0.11.0'\n", "min", "0.11.0", ""},
		{"[tool]\nversion='0.12.2'\npolicy='exact'\n", "exact", "0.12.2", ""},
		{"[tool]\nversion='0.12.2'\nmin_version='0.11.0'\n", "", "", "cannot set both [tool].version and [tool].min_version"},
		{"[tool]\nversion='0.12.2'\npolicy='bogus'\n", "", "", "unknown [tool].policy: 'bogus'"},
		{"[tool]\npolicy='exact'\n", "", "", "[tool].policy requires [tool].version or [tool].min_version"},
		{"[tool]\nversion=''\n", "", "", "[tool].version must be a non-empty string"},
		{"", "", "", ""},
		{"[tool]\n", "", "", ""},
	}
	for _, c := range cases {
		tbl, err := toml.Parse([]byte(c.toml))
		if err != nil {
			t.Fatalf("toml.Parse(%q): %v", c.toml, err)
		}
		policy, perr := ParseToolPolicy(tbl)
		if perr != c.err {
			t.Errorf("%q: err = %q, want %q", c.toml, perr, c.err)
			continue
		}
		if c.err != "" {
			if policy != nil {
				t.Errorf("%q: expected nil policy", c.toml)
			}
			continue
		}
		if c.kind == "" {
			if policy != nil {
				t.Errorf("%q: expected no policy, got %+v", c.toml, policy)
			}
			continue
		}
		if policy == nil || policy.Kind != c.kind || policy.Version != c.version {
			t.Errorf("%q: policy = %+v, want %s|%s", c.toml, policy, c.kind, c.version)
		}
	}
}

func TestCheckPolicy(t *testing.T) {
	if ok, reason := CheckPolicy("unknown", &ToolPolicy{Kind: "exact", Version: "0.1.0"}); ok || reason != "installed CLI version is unknown" {
		t.Errorf("unknown: %v %q", ok, reason)
	}
	if ok, _ := CheckPolicy("0.12.2", &ToolPolicy{Kind: "exact", Version: "0.12.2"}); !ok {
		t.Error("exact match should pass")
	}
	if ok, reason := CheckPolicy("0.11.0", &ToolPolicy{Kind: "exact", Version: "0.12.2"}); ok || reason != "installed CLI 0.11.0 does not match pinned 0.12.2" {
		t.Errorf("exact mismatch: %v %q", ok, reason)
	}
	if ok, reason := CheckPolicy("0.10.0", &ToolPolicy{Kind: "min", Version: "0.11.0"}); ok || reason != "installed CLI 0.10.0 is below minimum 0.11.0" {
		t.Errorf("min violation: %v %q", ok, reason)
	}
}

func TestEvaluateCLIVersion(t *testing.T) {
	tbl, err := toml.Parse([]byte("[tool]\nversion='0.12.2'\n"))
	if err != nil {
		t.Fatal(err)
	}
	sev, name, msg := EvaluateCLIVersion("0.12.2", tbl, map[string]string{"cli_version": "0.12.2"})
	if sev != "OK" || name != "cli-version" || msg != "installed 0.12.2, pinned 0.12.2, last sync 0.12.2" {
		t.Errorf("got %s|%s|%s", sev, name, msg)
	}
	sev, _, _ = EvaluateCLIVersion("0.10.0", tbl, nil)
	if sev != "ERROR" {
		t.Errorf("policy violation severity = %s, want ERROR", sev)
	}
	none, err := toml.Parse([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	sev, _, msg = EvaluateCLIVersion("0.12.2", none, nil)
	if sev != "INFO" || !strings.Contains(msg, "no [tool] pin") {
		t.Errorf("no-pin: %s %q", sev, msg)
	}
}

func TestReadInstalledVersionAndLockMeta(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ReadInstalledVersion(home); got != "unknown" {
		t.Errorf("missing VERSION = %q", got)
	}
	if err := os.WriteFile(filepath.Join(home, "VERSION"), []byte("0.12.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ReadInstalledVersion(home); got != "0.12.2" {
		t.Errorf("VERSION = %q", got)
	}
	lock := filepath.Join(tmp, "a.lock")
	if err := os.WriteFile(lock, []byte("[meta]\ncli_version=\"0.12.2\"\nsynced_at=\"2026-06-23T12:00:00Z\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := ReadLockMeta(lock)
	if meta["cli_version"] != "0.12.2" || meta["synced_at"] != "2026-06-23T12:00:00Z" {
		t.Errorf("meta = %v", meta)
	}
}

type corpusCase struct {
	Kind      string `json:"kind"`
	Left      string `json:"left,omitempty"`
	Right     string `json:"right,omitempty"`
	Version   string `json:"version,omitempty"`
	KindStr   string `json:"kind_,omitempty"`
	Installed string `json:"installed,omitempty"`
	TOML      string `json:"toml,omitempty"`
	Manifest  string `json:"manifest,omitempty"`
	Lock      string `json:"lock,omitempty"`
	Home      string `json:"home,omitempty"`
}

func goResult(c corpusCase) string {
	switch c.Kind {
	case "compare":
		return fmt.Sprintf("%d", CompareVersions(c.Left, c.Right))
	case "parse":
		v, ok := ParseVersionTuple(c.Version)
		if !ok {
			return "None"
		}
		pre := "None"
		if v.Pre != "" {
			pre = pyReprString(v.Pre)
		}
		return fmt.Sprintf("(%d, %d, %d, %s)", v.Major, v.Minor, v.Patch, pre)
	case "policy":
		tbl, err := toml.Parse([]byte(c.TOML))
		if err != nil {
			return "parse-error"
		}
		policy, perr := ParseToolPolicy(tbl)
		if policy != nil {
			return policy.Kind + "|" + policy.Version
		}
		if perr != "" {
			return "error|" + perr
		}
		return "none"
	case "checkpolicy":
		ok, reason := CheckPolicy(c.Installed, &ToolPolicy{Kind: c.KindStr, Version: c.Version})
		if ok {
			return "ok"
		}
		return "fail|" + reason
	case "evaluate":
		tbl, err := toml.Parse([]byte(c.Manifest))
		if err != nil {
			return "parse-error"
		}
		lock := map[string]string{}
		if c.Lock != "" {
			lock = ReadLockMeta(c.Lock)
		}
		sev, name, msg := EvaluateCLIVersion(c.Installed, tbl, lock)
		return sev + "|" + name + "|" + msg
	case "installed":
		return ReadInstalledVersion(c.Home)
	case "lockmeta":
		meta := ReadLockMeta(c.Lock)
		keys := SortedStrings(meta)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = k + "=" + meta[k]
		}
		return strings.Join(parts, ";")
	}
	return "unknown"
}

func TestDifferentialReference(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "lib", "_internal", "cli_version.py")); err != nil {
		t.Fatalf("python reference missing: %v", err)
	}

	tmp := t.TempDir()
	homeA := filepath.Join(tmp, "home-a")
	if err := os.MkdirAll(homeA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeA, "VERSION"), []byte("0.12.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	homeBlank := filepath.Join(tmp, "home-blank")
	if err := os.MkdirAll(homeBlank, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeBlank, "VERSION"), []byte("  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	homeMissing := filepath.Join(tmp, "home-missing")

	lockA := filepath.Join(tmp, "a.lock")
	if err := os.WriteFile(lockA, []byte("[meta]\ncli_version=\"0.12.2\"\nsynced_at=\"2026-06-23T12:00:00Z\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lockB := filepath.Join(tmp, "b.lock")
	if err := os.WriteFile(lockB, []byte("[meta]\ncli_version=\"0.11.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var corpus []corpusCase
	add := func(c corpusCase) { corpus = append(corpus, c) }

	for _, p := range [][2]string{
		{"0.12.2", "0.12.3"}, {"0.12.3", "0.12.2"}, {"0.12.2", "0.12.2"},
		{"0.12.2-rc1", "0.12.2"}, {"0.12.2", "0.12.2-rc1"}, {"0.12.2+build", "0.12.2"},
		{"1.0.0", "0.9.9"}, {"unknown", "0.12.2"}, {"0.12.2", "unknown"},
		{"bad", "bad"}, {"bad", "0.1.0"}, {"0.1.0", "bad"},
	} {
		add(corpusCase{Kind: "compare", Left: p[0], Right: p[1]})
	}
	for _, v := range []string{"0.12.2", "1.2.3-rc1", "1.2.3+build", "1.2.3-rc1+build", "unknown", "bad", "1.2", " 0.12.2 "} {
		add(corpusCase{Kind: "parse", Version: v})
	}

	manifests := []string{
		"[tool]\nversion = '0.12.2'\n",
		"[tool]\nmin_version = '0.11.0'\n",
		"[tool]\nversion = '0.12.2'\npolicy = 'exact'\n",
		"[tool]\nmin_version = '0.11.0'\npolicy = 'min'\n",
		"[tool]\nversion = '0.12.2'\nmin_version = '0.11.0'\n",
		"[tool]\nversion = '0.12.2'\npolicy = 'bogus'\n",
		"[tool]\npolicy = 'exact'\n",
		"[tool]\nversion = ''\n",
		"[tool]\nmin_version = ''\n",
		"version = '0.12.2'\n",
		"[tool]\n",
		"",
		"[project]\nname = 'x'\n",
	}
	for _, m := range manifests {
		add(corpusCase{Kind: "policy", TOML: m})
		add(corpusCase{Kind: "evaluate", Installed: "0.12.2", Manifest: m, Lock: lockA})
		add(corpusCase{Kind: "evaluate", Installed: "0.11.0", Manifest: m, Lock: lockB})
		add(corpusCase{Kind: "evaluate", Installed: "unknown", Manifest: m})
	}
	for _, cp := range []struct{ inst, kind, ver string }{
		{"0.12.2", "exact", "0.12.2"}, {"0.11.0", "exact", "0.12.2"},
		{"0.12.2", "exact", "0.12.2-rc1"}, {"0.12.3", "min", "0.12.2"},
		{"0.12.2", "min", "0.12.3"}, {"unknown", "min", "0.1.0"}, {"0.12.2", "min", "0.12.2"},
	} {
		add(corpusCase{Kind: "checkpolicy", Installed: cp.inst, KindStr: cp.kind, Version: cp.ver})
	}
	add(corpusCase{Kind: "installed", Home: homeA})
	add(corpusCase{Kind: "installed", Home: homeMissing})
	add(corpusCase{Kind: "installed", Home: homeBlank})
	add(corpusCase{Kind: "lockmeta", Lock: lockA})
	add(corpusCase{Kind: "lockmeta", Lock: lockB})
	add(corpusCase{Kind: "lockmeta", Lock: filepath.Join(tmp, "absent.lock")})

	data, err := json.Marshal(corpus)
	if err != nil {
		t.Fatal(err)
	}
	corpusPath := filepath.Join(tmp, "corpus.json")
	if err := os.WriteFile(corpusPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("python3", filepath.Join(root, "internal", "cliversion", "testdata", "version_ref.py"), corpusPath)
	cmd.Dir = root
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("python driver failed: %v\n%s", err, errBuf.String())
	}
	pyLines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(pyLines) != len(corpus) {
		t.Fatalf("driver produced %d lines for %d cases", len(pyLines), len(corpus))
	}
	compared := 0
	for i, c := range corpus {
		got := fmt.Sprintf("%d\t%s", i, goResult(c))
		if got != pyLines[i] {
			t.Errorf("case %d %+v:\n  go:     %q\n  python: %q", i, c, got, pyLines[i])
			continue
		}
		compared++
	}
	t.Logf("differential cliversion: compared %d/%d cases", compared, len(corpus))
	if compared < 60 {
		t.Fatalf("only %d cases compared — corpus unexpectedly small", compared)
	}
}
