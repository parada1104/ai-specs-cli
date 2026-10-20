package deps

// Differential test against the legacy implementation
// (lib/_internal/dep_install.py). The Python side runs via
// testdata/driver.py with shutil.which and platform.system scripted per case;
// both sides emit the same canonical projection of the resolved plan (or the
// exact error string) and the test requires equality of the decoded JSON — the
// key ORDER is the encoder's, and the projection's VALUES are the contract.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/provider"
)

type differentialCase struct {
	Binary        string   `json:"binary"`
	InstallURL    string   `json:"install_url,omitempty"`
	Installer     string   `json:"installer,omitempty"`
	Repository    string   `json:"repository,omitempty"`
	ReleasePolicy string   `json:"release_policy,omitempty"`
	MinVersion    string   `json:"min_version,omitempty"`
	Which         []string `json:"which,omitempty"`
	System        string   `json:"system,omitempty"`
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
			ReleasePolicy: "latest-stable",
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

	for i, c := range cases {
		// Each case is resolved by the legacy driver and by the port, on the
		// same home, and the projections must match value for value.
		t.Run(fmt.Sprintf("%02d-%s-%s", i, c.Binary, c.System), func(t *testing.T) {
			home := t.TempDir()
			legacy := decodeProjection(t, "legacy", runLegacyPlanDriver(t, home, c))
			mine := decodeProjection(t, "native", resolveForDifferential(t, c, home))
			if !reflect.DeepEqual(legacy, mine) {
				t.Errorf("plan differential mismatch\n legacy: %v\n    new: %v", legacy, mine)
			}
		})
	}
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

func runLegacyPlanDriver(t *testing.T, home string, c differentialCase) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	driver := filepath.Join(root, "internal", "deps", "testdata", "driver.py")
	payload, err := json.Marshal(map[string]any{"home": home, "cases": []differentialCase{c}})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", driver, root)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("legacy driver failed: %v\nstderr: %s", err, stderr.String())
	}
	return strings.TrimSpace(stdout.String())
}
