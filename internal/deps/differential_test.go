package deps

// Frozen oracle for the plan differential. testdata/deps_plan_golden.json holds
// the canonical projection the legacy resolver (lib/_internal/dep_install.py,
// run once through testdata/driver.py with shutil.which and platform.system
// scripted per case) produced for each case below. Tests never run an
// interpreter: the port's projection must equal the frozen record value for
// value. Host-bound strings are tokenized on both sides: the scratch home
// becomes <HOME>, and the provider target becomes <GOOS>/<GOARCH> (the capture
// host was darwin/arm64). The key ORDER is the encoder's; the VALUES are the
// contract.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/provider"
)

const depsPlanGoldenPath = "testdata/deps_plan_golden.json"

type differentialCase struct {
	Binary        string   `json:"binary"`
	InstallURL    string   `json:"install_url,omitempty"`
	Installer     string   `json:"installer,omitempty"`
	Repository    string   `json:"repository,omitempty"`
	ReleasePolicy string   `json:"release_policy,omitempty"`
	MinVersion    string   `json:"min_version,omitempty"`
	Which         []string `json:"which,omitempty"`
	System        string   `json:"system,omitempty"`
	// PathStub puts a non-provider executable named Binary on the case's PATH
	// (the capture host had gh 2.102.0 installed). It is never an input of the
	// plan; the frozen record must hold with or without it.
	PathStub bool `json:"-"`
}

// isolateProviderPath gives every github-release case its own PATH, so no host
// binary can reach a provider case; the stub keeps the captured precondition.
func isolateProviderPath(t *testing.T, c differentialCase) {
	t.Helper()
	if c.Installer != KindGitHubRelease {
		return
	}
	dir := t.TempDir()
	if c.PathStub {
		stub := "#!/bin/sh\necho \"" + c.Binary + " version 2.102.0\"\n"
		if err := os.WriteFile(filepath.Join(dir, c.Binary), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

func requestOf(c differentialCase) PlanRequest {
	return PlanRequest{
		Binary:        c.Binary,
		InstallURL:    c.InstallURL,
		Installer:     c.Installer,
		Repository:    c.Repository,
		ReleasePolicy: c.ReleasePolicy,
		MinVersion:    c.MinVersion,
	}
}

// projectPlan is the canonical projection both sides emit.
func projectPlan(plan Plan) map[string]any {
	command := make([]any, 0, len(plan.Command))
	for _, arg := range plan.Command {
		command = append(command, arg)
	}
	var providerPlan any
	if plan.ProviderPlan != nil {
		providerPlan = map[string]any{
			"goos":       plan.ProviderPlan.Goos,
			"goarch":     plan.ProviderPlan.Goarch,
			"cache_root": plan.ProviderPlan.CacheRoot,
		}
	}
	return map[string]any{
		"ok":             true,
		"binary":         plan.Binary,
		"command":        command,
		"display":        plan.Display,
		"guidance_url":   plan.GuidanceURL,
		"kind":           plan.Kind,
		"installer":      plan.Installer,
		"repository":     plan.Repository,
		"release_policy": plan.ReleasePolicy,
		"min_version":    plan.MinVersion,
		"described":      Describe(plan),
		"provider":       providerPlan,
	}
}

func TestResolvePlanDifferential(t *testing.T) {
	cases := []differentialCase{
		// Package plans: brew, apt, brew-before-apt, and both absences.
		{Binary: "gh", InstallURL: "https://cli.github.com/", Which: []string{"brew"}, System: "Darwin"},
		{Binary: "jq", InstallURL: "https://jqlang.github.io/jq/", Which: []string{"apt-get"}, System: "Linux"},
		{Binary: "gh", InstallURL: "https://cli.github.com/", Which: []string{"brew", "apt-get"}, System: "Linux"},
		{Binary: "jq", InstallURL: "https://jqlang.github.io/jq/", Which: []string{"apt-get"}, System: "Darwin"},
		{Binary: "gh", InstallURL: "https://cli.github.com/", Which: []string{}, System: "Darwin"},
		// A mapped binary whose apt side is empty ("bb-cli", "").
		{Binary: "bb", InstallURL: "https://bitbucket.org/product/features/bb-cli", Which: []string{"apt-get"}, System: "Linux"},
		// Guidance: guidance-only npx, a binary outside the map, and an
		// installer hint that is not the provider's (the branch order pin:
		// the provider arm is entered by `installer == github-release` ONLY).
		{Binary: "npx", InstallURL: "https://nodejs.org/", Which: []string{"brew"}, System: "Darwin"},
		{Binary: "custom-tool", Which: []string{}, System: "Darwin"},
		{Binary: "gh", InstallURL: "https://cli.github.com/", Installer: "curl-bash", Which: []string{"brew"}, System: "Darwin"},
		// The provider plan and its two rejection paths. The system stays
		// unscripted here: provider_install.detect_platform reads the host, and
		// internal/provider reads the Go runtime — the same machine, by design.
		{
			Binary: "jinna", InstallURL: "https://example.com/jinna",
			Installer: "github-release", Repository: provider.Repository,
			ReleasePolicy: "latest-stable", MinVersion: "0.4.0",
		},
		{
			Binary: "gh", InstallURL: "https://cli.github.com/",
			Installer: "github-release", Repository: provider.Repository,
			ReleasePolicy: "latest-stable", PathStub: true,
		},
		{
			Binary: "jinna", Installer: "github-release", Repository: "someone/else",
			ReleasePolicy: "latest-stable",
		},
		{
			Binary: "jinna", Installer: "github-release", Repository: provider.Repository,
			ReleasePolicy: "latest",
		},
	}

	golden := loadPlanGolden(t)
	if len(golden) != len(cases) {
		t.Fatalf("%s holds %d records; the cases need exactly %d", depsPlanGoldenPath, len(golden), len(cases))
	}
	for i, c := range cases {
		// Each case is resolved by the port on a fresh home and compared with
		// its frozen legacy record.
		name := fmt.Sprintf("%02d-%s-%s", i, c.Binary, c.System)
		t.Run(name, func(t *testing.T) {
			record, ok := golden[name]
			if !ok {
				t.Fatalf("%s has no record %q", depsPlanGoldenPath, name)
			}
			isolateProviderPath(t, c)
			home := t.TempDir()
			legacy := decodeProjection(t, "frozen", record)
			mine := decodeProjection(t, "native", hostBoundTokens(resolveForDifferential(t, c, home), home))
			if !reflect.DeepEqual(legacy, mine) {
				t.Errorf("plan differential mismatch\n legacy: %v\n    new: %v", legacy, mine)
			}
		})
	}
}

// loadPlanGolden returns the frozen plan records keyed by case name.
func loadPlanGolden(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(depsPlanGoldenPath)
	if err != nil {
		t.Fatalf("read frozen oracle %s: %v", depsPlanGoldenPath, err)
	}
	var doc struct {
		PlanGolden map[string]json.RawMessage `json:"plan_golden"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", depsPlanGoldenPath, err)
	}
	out := make(map[string]string, len(doc.PlanGolden))
	for name, record := range doc.PlanGolden {
		out[name] = string(record)
	}
	return out
}

// hostBoundTokens applies the capture's normalization to the port's output.
func hostBoundTokens(text, home string) string {
	text = strings.ReplaceAll(text, home, "<HOME>")
	text = strings.ReplaceAll(text, runtime.GOOS, "<GOOS>")
	return strings.ReplaceAll(text, runtime.GOARCH, "<GOARCH>")
}

func decodeProjection(t *testing.T, label, text string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("%s emitted non-JSON output %q: %v", label, text, err)
	}
	return out
}

func resolveForDifferential(t *testing.T, c differentialCase, home string) string {
	t.Helper()
	if c.System != "" {
		withSystem(t, strings.ToLower(c.System))
		withPath(t, c.Which...)
	}
	plan, err := ResolvePlan(requestOf(c), home)
	var payload map[string]any
	if err != nil {
		payload = map[string]any{"ok": false, "error": err.Error()}
	} else {
		payload = projectPlan(plan)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}
	return string(encoded)
}
