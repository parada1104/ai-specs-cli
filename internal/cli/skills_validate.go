package cli

// Native `ai-specs skills validate <skill_md_path>` (GO-20 U2e, unit U-D): the
// replacement for `python3 lib/_internal/skill_contract.py sync-metadata
// <path>` (bundled-skills/skill-sync/assets/sync.sh:15). The library lives in
// internal/skills (FromLocalSkillFile + ValidateSyncMetadata), reused as-is.
//
// Exit codes and streams match the oracle: one JSON line on stdout and 0 on
// success, one JSON error line on stderr and 1 on failure, usage and 2 on a
// bad argument list. Documented divergences: `-h|--help` prints usage and
// exits 0 (Python treats it as a path and dies), and an unreadable/invalid
// file words a single `ERROR: …` line instead of a Python traceback.

import (
	"fmt"
	"io"
	"strings"

	"ai-specs.dev/ai-specs/internal/skills"
)

const skillsValidateUsage = `Usage: ai-specs skills validate <skill_md_path>

Validate one SKILL.md's sync metadata and print the payload as one JSON line.

Arguments:
  <skill_md_path>     Path to a SKILL.md file

Options:
  --help, -h          Show this help
`

func runSkillsValidate(args []string, stdout, stderr io.Writer) int {
	var positional []string
	for _, arg := range args {
		switch {
		case arg == "--help" || arg == "-h":
			fmt.Fprint(stdout, skillsValidateUsage)
			return 0
		case strings.HasPrefix(arg, "-"):
			fmt.Fprint(stderr, skillsValidateUsage)
			return 2
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) != 1 {
		fmt.Fprint(stderr, skillsValidateUsage)
		return 2
	}
	path := positional[0]
	skill, err := skills.FromLocalSkillFile(path)
	if err == nil {
		payload, validateErr := skills.ValidateSyncMetadata(skill, path)
		if validateErr == nil {
			fmt.Fprintln(stdout, skills.SyncMetadataJSON(payload))
			return 0
		}
		err = validateErr
	}
	if contract, ok := err.(*skills.SkillContractError); ok {
		fmt.Fprintln(stderr, skills.SkillContractErrorJSON(contract))
		return 1
	}
	fmt.Fprintf(stderr, "ERROR: %v\n", err)
	return 1
}
