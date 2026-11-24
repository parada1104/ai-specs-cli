package gates

// The project config (plan-build-openspec-odd-align P5/S11): openspec/config.yaml
// (odd/config.yaml in odd mode) configures the project, not the change. The CLI
// validates only the keys it consumes -- containment, a non-empty `schema:`, a
// `testing:` block with a non-empty `runner:`, and a boolean `strict_tdd:` --
// and then keeps the runner honest against the change's verification rows.
//
// Everything else (`context`, `rules.*`, `ai_specs_guidance`) is opaque prose for
// the agent and is never validated: a strict schema would repeat the frozen
// schema oracle trap of internal/schema that already blocked declaring
// [config.mode] in #589. The file is line-scanned on purpose; the freshness loop
// below, not a YAML dependency, is what keeps it true.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/planbuild"
)

// ProjectConfig is the consumed part of the project configuration.
type ProjectConfig struct {
	Schema    string
	Runner    string
	StrictTDD *bool // nil when the key is absent
}

// ReadProjectConfig reads and validates one mode's project config. Every error is
// a ready blocker in the house voice `config.yaml: <what is wrong>: <what to
// write>`.
func ReadProjectConfig(root, mode string) (ProjectConfig, error) {
	path := planbuild.ConfigPath(root, mode)
	if !containedRegularFile(root, path) {
		return ProjectConfig{}, fmt.Errorf(
			"config.yaml: the project config is missing or is not a regular file inside the "+
				"repository: write %s as a file of this repository", repoRelative(root, path))
	}
	return parseProjectConfig(readText(path))
}

// parseProjectConfig line-scans the consumed keys and blocks the invalid shapes.
func parseProjectConfig(text string) (ProjectConfig, error) {
	cfg := ProjectConfig{}
	strictSeen, strictInvalid := false, false
	open := false // inside the top-level `testing:` block
	for _, raw := range splitLines(text) {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		key, value, ok := splitYAMLKey(line)
		if !ok {
			continue
		}
		value = strings.Trim(value, `"'`)
		if indent > 0 {
			if open && key == "runner" {
				cfg.Runner = value
			}
			continue
		}
		open = key == "testing"
		switch key {
		case "schema":
			cfg.Schema = value
		case "strict_tdd":
			strictSeen = true
			switch strings.ToLower(value) {
			case "true":
				truth := true
				cfg.StrictTDD = &truth
			case "false":
				truth := false
				cfg.StrictTDD = &truth
			default:
				strictInvalid = true
			}
		}
	}
	switch {
	case cfg.Schema == "":
		return cfg, fmt.Errorf("config.yaml: schema is missing: declare the project schema with schema: spec-driven")
	case cfg.Runner == "":
		return cfg, fmt.Errorf("config.yaml: testing.runner is missing: declare the command that runs the project tests")
	case strictSeen && strictInvalid:
		return cfg, fmt.Errorf("config.yaml: strict_tdd must be true or false: write strict_tdd: true or strict_tdd: false")
	}
	return cfg, nil
}

// splitYAMLKey splits `key: value` on the first top-level colon.
func splitYAMLKey(line string) (key, value string, ok bool) {
	index := strings.Index(line, ":")
	if index <= 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:index]), strings.TrimSpace(line[index+1:]), true
}

// containedRegularFile is planbuild's plan predicate shape applied to any path: a
// regular file whose real path stays inside root.
func containedRegularFile(root, path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		base = root
	}
	return real == base || strings.HasPrefix(real, base+string(filepath.Separator))
}

// repoRelative is the repository-relative path the blockers name.
func repoRelative(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return relative
}

// checkProjectConfig is the standard/full config layer: the config must exist and
// validate, and its declared runner must stay fresh against the plan's
// verification rows. Light is exempt, and so is a change with no rows.
func checkProjectConfig(repoRoot, mode, planPath, tier string) []string {
	if tier != TierStandard && tier != TierFull {
		return nil
	}
	path := planbuild.ConfigPath(repoRoot, mode)
	cfg, err := ReadProjectConfig(repoRoot, mode)
	if err != nil {
		return []string{err.Error()}
	}
	plan, _ := ParsePlan(readText(planPath))
	return checkConfigFreshness(repoRoot, repoRelative(repoRoot, path), cfg, plan)
}

// checkConfigFreshness enforces P5's two freshness rules over one plan:
//
//	(a) every backticked command in a `## Verification` row must resolve here --
//	    a relative or absolute path must exist on disk, a bare word must be on PATH;
//	(b) the configured testing.runner must match the first word of at least one row.
//
// A change with no rows, or with no backticked command in any row, is not subject
// to either rule: there is nothing to keep fresh (S12 ceremony reduction).
// commandHead is the program plus its subcommand ("go test", "./tests/run.sh").
// Freshness compares this, not the whole command and not the first word: a
// narrower run of the same runner still counts as running it, while moving from
// ./tests/run.sh to "go test" is exactly the staleness this rule must catch.
func commandHead(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	if len(fields) == 1 {
		return fields[0]
	}
	return fields[0] + " " + fields[1]
}

func checkConfigFreshness(root, configPath string, cfg ProjectConfig, plan Plan) []string {
	var blockers, commands []string
	for _, row := range plan.Verification {
		command, ok := backtickedCommand(row.Evidence)
		if !ok {
			continue
		}
		commands = append(commands, command)
		if word := firstWord(command); !commandResolves(root, word) {
			blockers = append(blockers, configPath+`: verification for `+row.ID+
				` names command "`+command+`" which does not exist: `+
				"run the configured tests and record the real command")
		}
	}
	if len(commands) == 0 {
		return blockers
	}
	runner := commandHead(cfg.Runner)
	for _, command := range commands {
		if commandHead(command) == runner {
			return blockers
		}
	}
	return append(blockers, configPath+`: verification never runs the configured testing.runner "`+
		cfg.Runner+`": name it in at least one row or update config.yaml`)
}

// backtickedCommand reads the command a row's evidence starts with: the
// `\x60<command>\x60, exit 0` shape the SKILL shows.
func backtickedCommand(evidence string) (string, bool) {
	if !strings.HasPrefix(evidence, "`") {
		return "", false
	}
	rest := evidence[1:]
	end := strings.Index(rest, "`")
	if end <= 0 {
		return "", false
	}
	command := strings.TrimSpace(rest[:end])
	return command, command != ""
}

// firstWord is the shell word that decides whether a command can run.
func firstWord(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// commandResolves reports whether the first shell word of a recorded command can
// run here: a path is stat'ed inside the repository, a bare word is looked up.
func commandResolves(root, word string) bool {
	if word == "" {
		return false
	}
	if strings.HasPrefix(word, "./") || strings.HasPrefix(word, "/") {
		path := word
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, word)
		}
		info, err := os.Stat(path)
		return err == nil && info.Mode().IsRegular()
	}
	_, err := exec.LookPath(word)
	return err == nil
}
