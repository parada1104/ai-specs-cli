package deps

// Differential test for Gate against its FROZEN legacy oracle.
// testdata/gate_golden.json holds the canonical projection the REAL legacy
// config_wizard._dep_gate produced for each case below, captured once through
// testdata/gate_driver.py with the probe, questionary, dep_install's
// package-manager calls, the provider install and the isatty answers all
// scripted. Tests never run an interpreter: Gate runs on the same scripted
// terminal (runNativeGate) and its projection must equal the frozen record
// value for value, one record per case.
//
// Host-bound strings were tokenized at capture and are tokenized here by
// gateNormalizeText: the scratch home becomes <HOME>, and the provider target
// becomes <GOOS>/<GOARCH> in its three exact phrases (the capture host was
// darwin/arm64). Every other host-shaped string is compared as written.
//
// The fixture lines stay under 80 columns on purpose: rich wraps the console at
// its default width off the TTY, and the port emits one unwrapped line per
// result. Every line the gate can print in practice is short (a binary, a
// purpose, the URL), so the constraint belongs to the fixture, not the port.
// gate_driver.py stays on disk until the lib/_internal deletion, but no test
// executes it.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/provider"
	"ai-specs.dev/ai-specs/internal/schema"
)

const gateGoldenPath = "testdata/gate_golden.json"

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

// gateNormalizeText tokenizes the exact phrases of the gate projection that
// carry the capture host: the scratch home, and the provider target phrases
// (the capture host was darwin/arm64). Every other host-shaped string is
// compared as written, so a difference there still fails the comparison.
func gateNormalizeText(text, home string) string {
	goos, goarch := runtime.GOOS, runtime.GOARCH
	out := strings.ReplaceAll(text, home, "<HOME>")
	out = strings.ReplaceAll(out, "target:          "+goos+"/"+goarch, "target:          <GOOS>/<GOARCH>")
	out = strings.ReplaceAll(out, "_"+goos+"_"+goarch+".tar.gz", "_<GOOS>_<GOARCH>.tar.gz")
	out = strings.ReplaceAll(out, "for "+goos+"/"+goarch, "for <GOOS>/<GOARCH>")
	return out
}

// normalizeGateProjection applies gateNormalizeText to every string the
// projection carries: both streams, every prompt message and every command
// argument.
func normalizeGateProjection(p projectGate, home string) projectGate {
	p.Stdout = gateNormalizeText(p.Stdout, home)
	p.Stderr = gateNormalizeText(p.Stderr, home)
	for i, message := range p.Prompts {
		p.Prompts[i] = gateNormalizeText(message, home)
	}
	for i, command := range p.Commands {
		for j, arg := range command {
			p.Commands[i][j] = gateNormalizeText(arg, home)
		}
	}
	return p
}

// loadGateGolden returns the frozen gate records keyed by case name.
func loadGateGolden(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(gateGoldenPath)
	if err != nil {
		t.Fatalf("read frozen oracle %s: %v", gateGoldenPath, err)
	}
	var doc struct {
		GateGolden map[string]json.RawMessage `json:"gate_golden"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", gateGoldenPath, err)
	}
	return doc.GateGolden
}

// TestGateNormalizeLeavesOtherHostStringsAlone is the negative control of the
// scoped normalization: a host-shaped string outside the target phrases and
// outside the scratch home must survive normalization unchanged.
func TestGateNormalizeLeavesOtherHostStringsAlone(t *testing.T) {
	goos, goarch := runtime.GOOS, runtime.GOARCH
	text := "check " + goos + "-" + goarch + " tool; see https://example.com/" + goos + "; asset x_" + goarch + ".zip"
	if got := gateNormalizeText(text, "/scratch/home"); got != text {
		t.Errorf("out-of-scope host string was normalized: %q", got)
	}
	if got := gateNormalizeText("dest /private/var/scratch/cache/bin/jinna", "/private/var/scratch"); got != "dest <HOME>/cache/bin/jinna" {
		t.Errorf("home phrase not tokenized: %q", got)
	}
	if got := gateNormalizeText("target:          "+goos+"/"+goarch, "/scratch/home"); got != "target:          <GOOS>/<GOARCH>" {
		t.Errorf("target phrase not tokenized: %q", got)
	}
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

// gateDifferentialCases is the frozen case matrix. Its order defines the
// golden record keys (NN-name); adding, removing or reordering a case is a
// contract change to the frozen oracle.
func gateDifferentialCases() []gateDifferentialCase {
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
	missingCustom := gateDriverResult{Binary: "custom-tool", Purpose: "Custom tool", Detail: "not found on PATH",
		InstallURL: "https://example.com/custom", Required: true}

	jqDep := gateDriverDep{Binary: "jq", Purpose: "JSON processor", Required: true, InstallURL: "https://example.com/jq"}
	npxDep := gateDriverDep{Binary: "npx", Purpose: "Node runner", Required: true}
	ghDep := gateDriverDep{Binary: "gh", Purpose: "GitHub CLI", Required: true, InstallURL: "https://cli.github.com"}
	jinnaDep := gateDriverDep{Binary: "jinna", Purpose: "Jinna MCP", Required: true,
		Installer: "github-release", Repository: provider.Repository, ReleasePolicy: "latest-stable"}
	badProviderDep := gateDriverDep{Binary: "jinna", Purpose: "Jinna MCP", Required: true,
		Installer: "github-release", Repository: "evil/repo", ReleasePolicy: "latest-stable"}
	customDep := gateDriverDep{Binary: "custom-tool", Purpose: "Custom tool", Required: true,
		InstallURL: "https://example.com/custom"}

	return []gateDifferentialCase{
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
		// T2 (S5): the apt failure arms — the exited line WITH its guidance
		// line, and the subprocess.run error line WITHOUT one.
		{Name: "apt-exit-1", TTY: true, Deps: []gateDriverDep{jqDep},
			Probe: [][]gateDriverResult{{missingJQ}, {missingJQ}}, Answers: []bool{true, false},
			Which: []string{"apt-get"}, System: "linux", Install: gateDriverInstall{Exit: 1}},
		{Name: "apt-run-error", TTY: true, Deps: []gateDriverDep{jqDep},
			Probe: [][]gateDriverResult{{missingJQ}}, Answers: []bool{true, false},
			Which: []string{"apt-get"}, System: "linux",
			Install: gateDriverInstall{Error: "Command '['sudo', 'apt-get', 'install', '-y', 'jq']' timed out after 300 seconds"}},
		{Name: "provider-verified", TTY: true, Deps: []gateDriverDep{jinnaDep},
			Probe: [][]gateDriverResult{{missingJinna}, {presentJinna}}, Answers: []bool{true},
			Install: gateDriverInstall{Verified: true, Version: "1.4.2"}},
		{Name: "provider-failure", TTY: true, Deps: []gateDriverDep{jinnaDep},
			Probe: [][]gateDriverResult{{missingJinna}}, Answers: []bool{true, false},
			Install: gateDriverInstall{ProviderError: "provider download URL is not an allowlisted HTTPS URL"}},
		{Name: "plan-resolution-aborts-the-batch", TTY: true, Deps: []gateDriverDep{ghDep, badProviderDep},
			Probe: [][]gateDriverResult{{missingGH, missingJinna}}, Answers: []bool{false}},
		// T1 (S6): one batch whose plans have DIFFERENT kinds — a github-release
		// describe+question, a guidance arrow and a package question — emitted in
		// the probe's own order (jinna, custom-tool, jq: not alphabetical, not
		// grouped by kind), each arm in that single pass.
		{Name: "mixed-kind-loop-order", TTY: true, Deps: []gateDriverDep{customDep, jqDep, jinnaDep},
			Probe:   [][]gateDriverResult{{missingJinna, missingCustom, missingJQ}},
			Answers: []bool{true, true, false},
			Which:   []string{"brew"}, System: "darwin",
			Install: gateDriverInstall{Verified: true, Version: "1.4.2"}},
		{Name: "off-tty", TTY: false, Deps: []gateDriverDep{jqDep},
			Probe: [][]gateDriverResult{{missingJQ}}, Answers: []bool{false}},
		{Name: "unanswered-confirm", TTY: false, Deps: []gateDriverDep{jqDep},
			Probe: [][]gateDriverResult{{missingJQ}}},
		{Name: "unknown-binary-is-guidance", TTY: true,
			Deps:    []gateDriverDep{{Binary: "custom-tool", Purpose: "Custom", Required: true}},
			Probe:   [][]gateDriverResult{{{Binary: "custom-tool", Purpose: "Custom", Detail: "not found on PATH", Required: true}}},
			Answers: []bool{false}},
	}
}

func TestGateDifferential(t *testing.T) {
	golden := loadGateGolden(t)
	if len(golden) != len(gateDifferentialCases()) {
		t.Fatalf("%s holds %d records; the cases need exactly %d", gateGoldenPath, len(golden), len(gateDifferentialCases()))
	}
	for i, c := range gateDifferentialCases() {
		name := fmt.Sprintf("%02d-%s", i, c.Name)
		t.Run(name, func(t *testing.T) {
			record, ok := golden[name]
			if !ok {
				t.Fatalf("%s has no record %q", gateGoldenPath, name)
			}
			var legacy projectGate
			if err := json.Unmarshal(record, &legacy); err != nil {
				t.Fatalf("decode frozen record %s: %v", name, err)
			}
			// Each case runs the port on a fresh home and is compared with its
			// frozen legacy record.
			home := resolvedHome(t)
			mine := normalizeGateProjection(runNativeGate(t, c, home), home)
			if !reflect.DeepEqual(legacy, mine) {
				t.Errorf("gate differential mismatch\n legacy: %+v\n    new: %+v", legacy, mine)
			}
		})
	}
}
