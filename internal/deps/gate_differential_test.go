package deps

// Differential test for Gate: the legacy bytes and the port's, on the same
// scripted terminal. testdata/gate_driver.py drives the REAL
// config_wizard._dep_gate with the probe, questionary, dep_install's
// package-manager calls and the provider install all scripted; this side runs
// Gate with the same script. Both emit one canonical projection — the verdict,
// the two streams, the confirm messages in order and the commands run — and the
// test requires them value for value.
//
// The fixture lines stay under 80 columns on purpose: rich wraps the console at
// its default width off the TTY, and the port emits one unwrapped line per
// result. Every line the gate can print in practice is short (a binary, a
// purpose, the URL), so the constraint belongs to the fixture, not the port.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/provider"
	"ai-specs.dev/ai-specs/internal/schema"
)

// gateDriverDep is one recipe [[deps.cli]] entry as the driver reads it.
type gateDriverDep struct {
	Binary        string `json:"binary"`
	Purpose       string `json:"purpose"`
	Required      bool   `json:"required"`
	InstallURL    string `json:"install_url"`
	Installer     string `json:"installer"`
	Repository    string `json:"repository"`
	ReleasePolicy string `json:"release_policy"`
	MinVersion    string `json:"min_version"`
}

// gateDriverResult is one scripted probe answer.
type gateDriverResult struct {
	Binary     string `json:"binary"`
	Purpose    string `json:"purpose"`
	Detail     string `json:"detail"`
	InstallURL string `json:"install_url"`
	Required   bool   `json:"required"`
	OK         bool   `json:"ok"`
	Found      bool   `json:"found"`
}

// gateDriverInstall scripts the two install arms.
type gateDriverInstall struct {
	Exit          int    `json:"exit"`
	Error         string `json:"error,omitempty"`
	Verified      bool   `json:"verified,omitempty"`
	Version       string `json:"version,omitempty"`
	ProviderError string `json:"provider_error,omitempty"`
}

type gateDifferentialCase struct {
	Name    string
	TTY     bool
	Deps    []gateDriverDep
	Probe   [][]gateDriverResult
	Answers []bool
	Which   []string
	// System is Go's vocabulary ("darwin"/"linux"); the driver payload carries
	// platform.system()'s ("Darwin"/"Linux").
	System  string
	Install gateDriverInstall
}

// projectGate is the canonical projection both sides emit.
type projectGate struct {
	Result   bool       `json:"result"`
	Stdout   string     `json:"stdout"`
	Stderr   string     `json:"stderr"`
	Prompts  []string   `json:"prompts"`
	Commands [][]string `json:"commands"`
}

func (c gateDifferentialCase) payload(home string) map[string]any {
	probe := make([][]gateDriverResult, len(c.Probe))
	copy(probe, c.Probe)
	recipe := map[string]any{"id": "demo", "cli_deps": c.Deps}
	if recipe["cli_deps"] == nil {
		recipe["cli_deps"] = []gateDriverDep{}
	}
	answers := c.Answers
	if answers == nil {
		answers = []bool{}
	}
	which := c.Which
	if which == nil {
		which = []string{}
	}
	payload := map[string]any{
		"home":    home,
		"tty":     c.TTY,
		"recipe":  recipe,
		"probe":   probe,
		"answers": answers,
		"which":   which,
		"install": c.Install,
	}
	if c.System != "" {
		payload["system"] = strings.ToUpper(c.System[:1]) + c.System[1:]
	}
	return payload
}

// runNativeGate is the port's side of the comparison.
func runNativeGate(t *testing.T, c gateDifferentialCase, home string) projectGate {
	t.Helper()
	withPath(t, c.Which...)
	if c.System != "" {
		withSystem(t, c.System)
	}

	probed := 0
	probe := func() []Result {
		if len(c.Probe) == 0 {
			return nil
		}
		index := probed
		if index >= len(c.Probe) {
			index = len(c.Probe) - 1
		}
		probed++
		out := make([]Result, 0, len(c.Probe[index]))
		for _, entry := range c.Probe[index] {
			out = append(out, Result{
				Binary: entry.Binary, Purpose: entry.Purpose, Detail: entry.Detail,
				InstallURL: entry.InstallURL, Required: entry.Required, OK: entry.OK, Found: entry.Found,
			})
		}
		return out
	}

	asked := 0
	prompts := []string{}
	prompt := func(message string, def bool) (bool, bool) {
		prompts = append(prompts, message)
		if asked >= len(c.Answers) {
			return false, false
		}
		answer := c.Answers[asked]
		asked++
		return answer, true
	}

	commands := [][]string{}
	run := func(command []string) (int, error) {
		commands = append(commands, command)
		if c.Install.Error != "" {
			return 0, errors.New(c.Install.Error)
		}
		return c.Install.Exit, nil
	}

	install := func(plan Plan) (provider.Resolution, error) {
		if c.Install.ProviderError != "" {
			return provider.Resolution{}, errors.New(c.Install.ProviderError)
		}
		return provider.Resolution{
			Binary: plan.Binary, Version: c.Install.Version, Verified: c.Install.Verified,
		}, nil
	}

	recipe := buildGateRecipe(c.Deps)
	var console strings.Builder
	ok, err := Gate(GateRequest{
		Recipe:          recipe,
		Home:            home,
		Out:             &console,
		TTY:             c.TTY,
		Prompt:          prompt,
		Probe:           probe,
		Run:             run,
		InstallProvider: install,
	})
	if err != nil {
		t.Fatalf("Gate() error = %v", err)
	}
	return projectGate{Result: ok, Stdout: "", Stderr: console.String(), Prompts: prompts, Commands: commands}
}

// buildGateRecipe converts the case's declared deps into the recipe the
// caller would have loaded.
func buildGateRecipe(deps []gateDriverDep) *schema.Recipe {
	cliDeps := make([]*schema.CliDep, 0, len(deps))
	for _, dep := range deps {
		cliDeps = append(cliDeps, &schema.CliDep{
			Binary: dep.Binary, Purpose: dep.Purpose, Required: dep.Required,
			InstallURL: dep.InstallURL, Installer: dep.Installer,
			Repository: dep.Repository, ReleasePolicy: dep.ReleasePolicy, MinVersion: dep.MinVersion,
		})
	}
	return &schema.Recipe{ID: "demo", CliDeps: cliDeps}
}

func runLegacyGateDriver(t *testing.T, c gateDifferentialCase, home string) projectGate {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	driver := filepath.Join(root, "internal", "deps", "testdata", "gate_driver.py")
	body, err := json.Marshal(c.payload(home))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", driver, root)
	cmd.Stdin = bytes.NewReader(body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("legacy gate driver failed: %v\nstderr: %s", err, stderr.String())
	}
	var out projectGate
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("legacy gate driver emitted non-JSON %q: %v", stdout.String(), err)
	}
	if out.Prompts == nil {
		out.Prompts = []string{}
	}
	if out.Commands == nil {
		out.Commands = [][]string{}
	}
	return out
}

// resolvedHome is the same absolute path on both sides: t.TempDir's macOS
// /var path resolves to /private/var, and the provider cache root embeds it in
// describe_install_plan's destination line.
func resolvedHome(t *testing.T) string {
	t.Helper()
	raw := t.TempDir()
	resolved, err := filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	return resolved
}

func TestGateDifferential(t *testing.T) {
	missingJQ := gateDriverResult{Binary: "jq", Purpose: "JSON processor", Detail: "not found on PATH",
		InstallURL: "https://example.com/jq", Required: true}
	presentJQ := gateDriverResult{Binary: "jq", Purpose: "JSON processor", InstallURL: "https://example.com/jq",
		Required: true, OK: true, Found: true}
	missingNPX := gateDriverResult{Binary: "npx", Purpose: "Node runner", Detail: "not found on PATH", Required: true}
	missingGH := gateDriverResult{Binary: "gh", Purpose: "GitHub CLI", Detail: "not found on PATH",
		InstallURL: "https://cli.github.com", Required: true}
	presentGH := gateDriverResult{Binary: "gh", Purpose: "GitHub CLI", InstallURL: "https://cli.github.com",
		Required: true, OK: true, Found: true}
	missingJinna := gateDriverResult{Binary: "jinna", Purpose: "Jinna MCP", Detail: "no managed version",
		Required: true}
	presentJinna := gateDriverResult{Binary: "jinna", Purpose: "Jinna MCP", Required: true, OK: true, Found: true}

	jqDep := gateDriverDep{Binary: "jq", Purpose: "JSON processor", Required: true, InstallURL: "https://example.com/jq"}
	npxDep := gateDriverDep{Binary: "npx", Purpose: "Node runner", Required: true}
	ghDep := gateDriverDep{Binary: "gh", Purpose: "GitHub CLI", Required: true, InstallURL: "https://cli.github.com"}
	jinnaDep := gateDriverDep{Binary: "jinna", Purpose: "Jinna MCP", Required: true,
		Installer: "github-release", Repository: provider.Repository, ReleasePolicy: "latest-stable"}
	badProviderDep := gateDriverDep{Binary: "jinna", Purpose: "Jinna MCP", Required: true,
		Installer: "github-release", Repository: "evil/repo", ReleasePolicy: "latest-stable"}

	cases := []gateDifferentialCase{
		{Name: "nothing-missing", TTY: true, Deps: []gateDriverDep{jqDep}, Probe: [][]gateDriverResult{{presentJQ}}},
		{Name: "two-missing-declined", TTY: true, Deps: []gateDriverDep{jqDep, npxDep},
			Probe: [][]gateDriverResult{{missingJQ, missingNPX}}, Answers: []bool{false, true},
			Which: []string{"brew"}, System: "darwin"},
		{Name: "package-installed", TTY: true, Deps: []gateDriverDep{ghDep},
			Probe: [][]gateDriverResult{{missingGH}, {presentGH}}, Answers: []bool{true},
			Which: []string{"brew", "gh"}, System: "darwin"},
		{Name: "package-exit-1", TTY: true, Deps: []gateDriverDep{ghDep},
			Probe: [][]gateDriverResult{{missingGH}, {missingGH}}, Answers: []bool{true, false},
			Which: []string{"brew"}, System: "darwin", Install: gateDriverInstall{Exit: 1}},
		{Name: "package-timeout", TTY: true, Deps: []gateDriverDep{ghDep},
			Probe: [][]gateDriverResult{{missingGH}}, Answers: []bool{true, false},
			Which: []string{"brew"}, System: "darwin",
			Install: gateDriverInstall{Error: "Command '['brew', 'install', 'gh']' timed out after 300 seconds"}},
		{Name: "installed-not-on-path", TTY: true, Deps: []gateDriverDep{ghDep},
			Probe: [][]gateDriverResult{{missingGH}, {missingGH}}, Answers: []bool{true, false},
			Which: []string{"brew"}, System: "darwin"},
		{Name: "apt-on-linux", TTY: true, Deps: []gateDriverDep{jqDep},
			Probe: [][]gateDriverResult{{missingJQ}, {presentJQ}}, Answers: []bool{true},
			Which: []string{"apt-get", "jq"}, System: "linux"},
		{Name: "provider-verified", TTY: true, Deps: []gateDriverDep{jinnaDep},
			Probe: [][]gateDriverResult{{missingJinna}, {presentJinna}}, Answers: []bool{true},
			Install: gateDriverInstall{Verified: true, Version: "1.4.2"}},
		{Name: "provider-failure", TTY: true, Deps: []gateDriverDep{jinnaDep},
			Probe: [][]gateDriverResult{{missingJinna}}, Answers: []bool{true, false},
			Install: gateDriverInstall{ProviderError: "provider download URL is not an allowlisted HTTPS URL"}},
		{Name: "plan-resolution-aborts-the-batch", TTY: true, Deps: []gateDriverDep{ghDep, badProviderDep},
			Probe: [][]gateDriverResult{{missingGH, missingJinna}}, Answers: []bool{false}},
		{Name: "off-tty", TTY: false, Deps: []gateDriverDep{jqDep},
			Probe: [][]gateDriverResult{{missingJQ}}, Answers: []bool{false}},
		{Name: "unanswered-confirm", TTY: false, Deps: []gateDriverDep{jqDep},
			Probe: [][]gateDriverResult{{missingJQ}}},
		{Name: "unknown-binary-is-guidance", TTY: true,
			Deps:    []gateDriverDep{{Binary: "custom-tool", Purpose: "Custom", Required: true}},
			Probe:   [][]gateDriverResult{{{Binary: "custom-tool", Purpose: "Custom", Detail: "not found on PATH", Required: true}}},
			Answers: []bool{false}},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%02d-%s", i, c.Name), func(t *testing.T) {
			home := resolvedHome(t)
			legacy := runLegacyGateDriver(t, c, home)
			mine := runNativeGate(t, c, home)
			if !reflect.DeepEqual(legacy, mine) {
				t.Errorf("gate differential mismatch\n legacy: %+v\n    new: %+v", legacy, mine)
			}
		})
	}
}
