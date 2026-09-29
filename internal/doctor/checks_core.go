package doctor

import (
	"fmt"
	"path/filepath"
	"strings"
)

// checkManifest is doctor._check_manifest.
func (d *Doctor) checkManifest() {
	tomlPath := d.manifestPath()
	rel := relTo(d.Root, tomlPath)
	if !isFile(tomlPath) {
		d.add(ERROR, "manifest", "ai-specs/ai-specs.toml missing", "run ai-specs init")
		return
	}
	d.add(OK, "manifest", rel+" found")
	d.loadManifest()
	if d.manifest.data != nil {
		return
	}
	// The guidance renders tomllib's own diagnostic, so it is asked from that
	// same authority instead of being approximated.
	guidance := "manifest is not valid TOML"
	var res tomlErrorResult
	if err := d.op("toml_error", &res); err == nil && res.Error != nil {
		guidance = *res.Error
	}
	d.add(ERROR, "manifest", rel+" is not parseable", guidance)
}

// checkCLIVersion is doctor._check_cli_version.
func (d *Doctor) checkCLIVersion() {
	// The legacy `cli_version.py is a file` guard cannot fail for a compiled
	// binary, so the port always evaluates the policy (a documented deviation).
	d.loadManifest()
	if d.manifest.exists && d.manifest.data == nil {
		// Unparseable manifest: the legacy branch returns without a check.
		return
	}
	installed := readInstalledVersion(d.Home)
	lockMeta := readLockMeta(filepath.Join(d.Root, "ai-specs", ".ai-specs.lock"))
	severity, name, message := evaluateCLIVersion(installed, d.manifest.data, lockMeta)
	d.add(severity, name, message)
}

// checkLegacyRecipeVersions is doctor._check_legacy_recipe_versions.
func (d *Doctor) checkLegacyRecipeVersions() {
	d.loadManifest()
	if !d.manifest.exists || d.manifest.data == nil {
		return
	}
	legacy := legacyRecipeVersions(d.manifest.data)
	if len(legacy) == 0 {
		return
	}
	sample := strings.Join(legacy[:min(5, len(legacy))], ", ")
	more := ""
	if len(legacy) > 5 {
		more = fmt.Sprintf(" (+%d more)", len(legacy)-5)
	}
	d.add(WARN, "recipe-version",
		"legacy recipe version= keys present ("+sample+more+"); ignored — sync uses CLI catalog",
		"optional: remove version= from [recipes.*]; after ai-specs upgrade run ai-specs sync")
}

// checkAgentsMD is doctor._check_agents_md.
func (d *Doctor) checkAgentsMD() {
	agents := filepath.Join(d.Root, "AGENTS.md")
	switch {
	case isFile(agents):
		d.add(OK, "agents-md", "AGENTS.md found")
	case d.briefRenderDisabled():
		d.add(ERROR, "agents-md", "AGENTS.md missing; brief.render = false",
			"create a manual AGENTS.md or set [brief].render = true")
	default:
		d.add(ERROR, "agents-md", "AGENTS.md missing; run ai-specs sync",
			"ai-specs init or ai-specs sync")
	}
}
