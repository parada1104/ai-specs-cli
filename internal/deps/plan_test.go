package deps

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/provider"
)

// withPath scripts shutil.which's authority: only the named binaries exist.
func withPath(t *testing.T, present ...string) {
	t.Helper()
	old := lookPath
	lookPath = func(name string) (string, error) {
		for _, candidate := range present {
			if candidate == name {
				return "/fake/" + name, nil
			}
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { lookPath = old })
}

// withSystem scripts platform.system()'s answer in the resolver's own terms.
func withSystem(t *testing.T, name string) {
	t.Helper()
	old := hostSystem
	hostSystem = func() string { return name }
	t.Cleanup(func() { hostSystem = old })
}

func planRequest(binary string) PlanRequest {
	return PlanRequest{Binary: binary, InstallURL: "https://example.com/" + binary}
}

// TestResolvePlanPackageKinds pins dep_install.resolve_install_plan's branch
// order: brew first on Darwin AND Linux, apt only on Linux, guidance when
// neither package manager is present.
func TestResolvePlanPackageKinds(t *testing.T) {
	cases := []struct {
		name    string
		binary  string
		which   []string
		system  string
		want    string
		kind    string
		command []string
	}{
		{
			name: "brew on Darwin", binary: "gh", which: []string{"brew"}, system: "darwin",
			want: "brew install gh", kind: KindBrew, command: []string{"brew", "install", "gh"},
		},
		{
			name: "apt on Linux", binary: "jq", which: []string{"apt-get"}, system: "linux",
			want: "sudo apt-get install -y jq", kind: KindApt,
			command: []string{"sudo", "apt-get", "install", "-y", "jq"},
		},
		{
			// The legacy branch tests brew first, so brew wins where both exist.
			name: "brew wins on Linux with both", binary: "gh", which: []string{"brew", "apt-get"}, system: "linux",
			want: "brew install gh", kind: KindBrew, command: []string{"brew", "install", "gh"},
		},
		{
			// apt only ever answers on Linux.
			name: "apt is never offered on Darwin", binary: "jq", which: []string{"apt-get"}, system: "darwin",
			want: "https://example.com/jq", kind: KindGuidance,
		},
		{
			name: "a mapped binary with no package manager is guidance", binary: "gh", which: nil, system: "darwin",
			want: "https://example.com/gh", kind: KindGuidance,
		},
	}
	for _, tc := range cases {
		withPath(t, tc.which...)
		withSystem(t, tc.system)
		plan, err := ResolvePlan(planRequest(tc.binary), "")
		if err != nil {
			t.Fatalf("%s: ResolvePlan error: %v", tc.name, err)
		}
		if plan.Kind != tc.kind {
			t.Errorf("%s: Kind = %q, want %q", tc.name, plan.Kind, tc.kind)
		}
		if plan.Display != tc.want {
			t.Errorf("%s: Display = %q, want %q", tc.name, plan.Display, tc.want)
		}
		if strings.Join(plan.Command, " ") != strings.Join(tc.command, " ") {
			t.Errorf("%s: Command = %v, want %v", tc.name, plan.Command, tc.command)
		}
		if plan.Binary != tc.binary {
			t.Errorf("%s: Binary = %q, want %q", tc.name, plan.Binary, tc.binary)
		}
	}
}

// TestResolvePlanGuidanceOnly pins the two guidance arms the legacy module
// hard-codes: "install '<binary>' manually" without a URL, the URL with one,
// and npx never getting a package plan even where brew exists.
func TestResolvePlanGuidanceOnly(t *testing.T) {
	withPath(t, "brew")
	withSystem(t, "darwin")

	plan, err := ResolvePlan(PlanRequest{Binary: "custom-tool"}, "")
	if err != nil {
		t.Fatalf("ResolvePlan: %v", err)
	}
	if plan.Kind != KindGuidance || plan.Display != "install 'custom-tool' manually" {
		t.Errorf("plan = %+v, want guidance with the manual wording", plan)
	}

	plan, err = ResolvePlan(PlanRequest{Binary: "npx", InstallURL: "https://nodejs.org/"}, "")
	if err != nil {
		t.Fatalf("ResolvePlan: %v", err)
	}
	if plan.Kind != KindGuidance || plan.Display != "https://nodejs.org/" {
		t.Errorf("plan = %+v, want guidance that shows the URL", plan)
	}
	if len(plan.Command) != 0 {
		t.Errorf("Command = %v, want empty", plan.Command)
	}
}

func providerRequest() PlanRequest {
	return PlanRequest{
		Binary:        "jinna",
		InstallURL:    "https://example.com/jinna",
		Installer:     "github-release",
		Repository:    provider.Repository,
		ReleasePolicy: "latest-stable",
		MinVersion:    "0.4.0",
	}
}

// TestResolvePlanProvider pins the provider arm's plan shape.
func TestResolvePlanProvider(t *testing.T) {
	goos, goarch := provider.DetectPlatform(nil, nil)
	if goos == "" || goarch == "" {
		t.Skipf("unsupported provider platform %q/%q", goos, goarch)
	}
	home := t.TempDir()
	plan, err := ResolvePlan(providerRequest(), home)
	if err != nil {
		t.Fatalf("ResolvePlan: %v", err)
	}
	if plan.Kind != KindGitHubRelease {
		t.Fatalf("Kind = %q, want %q", plan.Kind, KindGitHubRelease)
	}
	wantDisplay := "GitHub Release " + provider.Repository + " (latest-stable) for " + goos + "/" + goarch
	if plan.Display != wantDisplay {
		t.Errorf("Display = %q, want %q", plan.Display, wantDisplay)
	}
	if len(plan.Command) != 0 {
		t.Errorf("Command = %v, want empty", plan.Command)
	}
	if plan.ProviderPlan == nil {
		t.Fatalf("ProviderPlan = nil, want the release plan")
	}
	if plan.ProviderPlan.Goos != goos || plan.ProviderPlan.Goarch != goarch {
		t.Errorf("ProviderPlan target = %s/%s, want %s/%s",
			plan.ProviderPlan.Goos, plan.ProviderPlan.Goarch, goos, goarch)
	}
	if want := provider.CacheRoot(&home); plan.ProviderPlan.CacheRoot != want {
		t.Errorf("ProviderPlan.CacheRoot = %q, want %q", plan.ProviderPlan.CacheRoot, want)
	}
	if plan.GuidanceURL != providerRequest().InstallURL {
		t.Errorf("GuidanceURL = %q, want the declared install_url", plan.GuidanceURL)
	}
}

// TestResolvePlanProviderRejects pins build_release_plan's validation errors
// verbatim: the gate prints them inside `install offer skipped: <exc>`.
func TestResolvePlanProviderRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*PlanRequest)
		want   string
	}{
		{
			name:   "not the allowlisted repository",
			mutate: func(r *PlanRequest) { r.Repository = "someone/else" },
			want:   "provider dependency is not the allowlisted GitHub Release installer",
		},
		{
			name:   "policy other than latest-stable",
			mutate: func(r *PlanRequest) { r.ReleasePolicy = "latest" },
			want:   "provider release policy must be latest-stable",
		},
	}
	for _, tc := range cases {
		req := providerRequest()
		tc.mutate(&req)
		_, err := ResolvePlan(req, t.TempDir())
		if err == nil || err.Error() != tc.want {
			t.Errorf("%s: error = %v, want %q", tc.name, err, tc.want)
		}
	}
}

// TestResolvePlanNonProviderInstallerFallsThrough pins the branch order: the
// provider arm is entered ONLY by installer == "github-release", so a dep that
// names another installer resolves exactly like a plain binary would.
func TestResolvePlanNonProviderInstallerFallsThrough(t *testing.T) {
	withPath(t, "brew")
	withSystem(t, "darwin")
	req := planRequest("gh")
	req.Installer = "curl-bash"
	req.Repository = "someone/else"
	plan, err := ResolvePlan(req, "")
	if err != nil {
		t.Fatalf("ResolvePlan: %v", err)
	}
	if plan.Kind != KindBrew || plan.ProviderPlan != nil {
		t.Errorf("plan = %+v, want the brew plan with no provider plan", plan)
	}
}

// TestDescribeProviderPlan pins describe_install_plan's consent text: the
// eleven-line plan a user is shown before an install, with the manual fallback
// only when a URL is declared.
func TestDescribeProviderPlan(t *testing.T) {
	goos, goarch := provider.DetectPlatform(nil, nil)
	if goos == "" || goarch == "" {
		t.Skipf("unsupported provider platform %q/%q", goos, goarch)
	}
	home := t.TempDir()
	plan, err := ResolvePlan(providerRequest(), home)
	if err != nil {
		t.Fatalf("ResolvePlan: %v", err)
	}
	suffix := ".tar.gz"
	if goos == "windows" {
		suffix = ".zip"
	}
	want := "  GitHub Release installation plan for 'jinna':\n" +
		"    repository:      " + provider.Repository + "\n" +
		"    release policy:  latest-stable (latest non-draft, non-prerelease tag)\n" +
		"    target:          " + goos + "/" + goarch + "\n" +
		"    expected asset:  jinna_<version>_" + goos + "_" + goarch + suffix + "\n" +
		"    destination:     " + provider.CacheRoot(&home) + "\n" +
		"    checksum source: SHA256SUMS from the same release\n" +
		"    minimum version: 0.4.0\n" +
		"    replacement:     an existing PATH provider is never modified; a verified managed version is reused\n" +
		"    manual fallback: https://example.com/jinna"
	if got := Describe(plan); got != want {
		t.Errorf("Describe = %q, want %q", got, want)
	}
}

// TestDescribeGuidanceAndPackagePlans pins the non-provider arm: the display
// line is the whole consent text.
func TestDescribeGuidanceAndPackagePlans(t *testing.T) {
	withPath(t, "brew")
	withSystem(t, "darwin")
	plan, err := ResolvePlan(planRequest("gh"), "")
	if err != nil {
		t.Fatalf("ResolvePlan: %v", err)
	}
	if got := Describe(plan); got != "brew install gh" {
		t.Errorf("Describe = %q, want the display line", got)
	}
}

// TestResolvePlanKeepsTheRequestFields pins what the gate's offer passes on:
// binary, installer, repository, policy and min_version travel with the plan
// (the provider arm), and stay empty otherwise, exactly as InstallPlan's
// defaults do.
func TestResolvePlanKeepsTheRequestFields(t *testing.T) {
	goos, goarch := provider.DetectPlatform(nil, nil)
	if goos != "" && goarch != "" {
		plan, err := ResolvePlan(providerRequest(), t.TempDir())
		if err != nil {
			t.Fatalf("ResolvePlan: %v", err)
		}
		if plan.Installer != "github-release" || plan.Repository != provider.Repository ||
			plan.ReleasePolicy != "latest-stable" || plan.MinVersion != "0.4.0" {
			t.Errorf("plan lost its request fields: %+v", plan)
		}
	}

	withPath(t, "brew")
	withSystem(t, "darwin")
	plain, err := ResolvePlan(planRequest("gh"), "")
	if err != nil {
		t.Fatalf("ResolvePlan: %v", err)
	}
	if plain.Installer != "" || plain.Repository != "" || plain.ReleasePolicy != "" || plain.MinVersion != "" {
		t.Errorf("non-provider plan carries provider fields: %+v", plain)
	}
}

// TestResolvePlanBoundsCompile keeps the error type honest: the provider
// rejection reaches the caller as provider.InstallError, which is what the
// gate's `install offer skipped:` line renders.
func TestResolvePlanProviderErrorType(t *testing.T) {
	req := providerRequest()
	req.Repository = "someone/else"
	_, err := ResolvePlan(req, t.TempDir())
	var installErr *provider.InstallError
	if !errors.As(err, &installErr) {
		t.Fatalf("error %v is not a *provider.InstallError", err)
	}
}

// TestPanelLines pins config_wizard._render_dep_panel's line byte for byte:
// f"{status:4s}  {r.binary:12s}  {r.purpose}{detail}{hint}", where detail is
// " (<detail>)" and hint is " → <install_url>" only for a non-ok dep that
// declares a URL.
func TestPanelLines(t *testing.T) {
	cases := []struct {
		name   string
		result Result
		want   string
	}{
		{
			name:   "ok dep",
			result: Result{Binary: "git", OK: true, Required: true, Purpose: "p"},
			want:   "OK    git           p",
		},
		{
			name: "required, missing, with guidance",
			result: Result{Binary: "npx", OK: false, Required: true,
				Purpose: "Run the MCP server", InstallURL: "https://example.com/install"},
			want: "WARN  npx           Run the MCP server → https://example.com/install",
		},
		{
			name: "required, missing, detail only",
			result: Result{Binary: "missing", OK: false, Required: true,
				Purpose: "do things", Detail: "not found on PATH"},
			want: "WARN  missing       do things (not found on PATH)",
		},
		{
			name:   "optional, missing",
			result: Result{Binary: "opt", OK: false, Required: false, Purpose: "optional thing"},
			want:   "INFO  opt           optional thing",
		},
		{
			name: "ok dep keeps its detail and drops the hint",
			result: Result{Binary: "gh", OK: true, Required: true,
				Purpose: "PRs", Detail: "version unknown", InstallURL: "https://x.y"},
			want: "OK    gh            PRs (version unknown)",
		},
		{
			name: "optional, missing, with guidance",
			result: Result{Binary: "opt2", OK: false, Required: false,
				Purpose: "optional thing", InstallURL: "https://example.com/opt"},
			want: "INFO  opt2          optional thing → https://example.com/opt",
		},
	}
	for _, tc := range cases {
		got := PanelLines([]Result{tc.result})
		if len(got) != 1 {
			t.Fatalf("%s: PanelLines returned %d lines, want 1", tc.name, len(got))
		}
		if got[0] != tc.want {
			t.Errorf("%s: line = %q, want %q", tc.name, got[0], tc.want)
		}
	}
}

func TestPanelLinesEmpty(t *testing.T) {
	if got := PanelLines(nil); len(got) != 0 {
		t.Errorf("PanelLines(nil) = %q, want no lines", got)
	}
}

// TestMissingRequired pins _dep_gate's filter: required AND not ok.
func TestMissingRequired(t *testing.T) {
	results := []Result{
		{Binary: "required-ok", OK: true, Required: true},
		{Binary: "required-missing", OK: false, Required: true},
		{Binary: "optional-missing", OK: false, Required: false},
	}
	got := MissingRequired(results)
	if len(got) != 1 || got[0].Binary != "required-missing" {
		t.Fatalf("MissingRequired = %+v, want only required-missing", got)
	}
}
