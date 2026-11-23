package planbuild

// Plan Build execution modes (plan-build-odd-companion S1/S2/S10): exactly one
// mode per repository decides which planning tree the gate reads. The mode is
// declared in the project manifest and never waives the plan precondition.

import (
	"os"
	"path/filepath"

	"ai-specs.dev/ai-specs/internal/config"
)

const (
	// ModeAuto infers the mode from the Gentle AI state directory.
	ModeAuto = "auto"
	// ModeOdd keeps planning artifacts under odd/.
	ModeOdd = "odd"
	// ModeOpenspec is the OpenSpec-centered default flow.
	ModeOpenspec = "openspec"
	// RecipeID is the recipe whose config carries the mode.
	RecipeID = "plan-build-flow"
	// ConfigKey is the manifest key holding the declared mode.
	ConfigKey = "mode"
)

// ManifestPath is the repository-wide project manifest convention.
func ManifestPath(root string) string {
	return filepath.Join(root, "ai-specs", "ai-specs.toml")
}

// Decide resolves the pure part of the mode: an explicit odd/openspec wins,
// anything else is auto, and auto follows the Gentle AI state probe.
func Decide(declared string, gentleAIState bool) string {
	switch declared {
	case ModeOdd, ModeOpenspec:
		return declared
	}
	if gentleAIState {
		return ModeOdd
	}
	return ModeOpenspec
}

// Resolve reads the declared mode from the manifest and infers auto from the
// Gentle AI state directory inside the Git common directory. Every unusable
// input (missing manifest, load error, absent key, non-string value, empty
// common dir) falls back to auto.
func Resolve(root, gitCommonDir string) string {
	declared := ""
	if manifest, err := config.LoadManifest(ManifestPath(root)); err == nil {
		if _, _, cfg := config.RecipeConfigState(manifest, RecipeID); cfg != nil {
			if value, ok := config.LookupNestedConfig(cfg, ConfigKey); ok {
				if s, isString := value.(string); isString {
					declared = s
				}
			}
		}
	}
	return Decide(declared, gentleAIState(gitCommonDir))
}

// gentleAIState reports whether the Gentle AI state directory exists: the
// project-scoped signal the shell and the CLI both create.
func gentleAIState(gitCommonDir string) bool {
	if gitCommonDir == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(gitCommonDir, "gentle-ai"))
	return err == nil && info.IsDir()
}
