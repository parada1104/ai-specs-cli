package gates

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	evidenceHeadingRe = regexp.MustCompile(`(?i)^[ ]{0,3}##[ \t]+Verify evidence[ \t]*$`)
	sectionHeadingRe  = regexp.MustCompile(`^[ ]{0,3}#{1,6}(?:[ \t]+|$)`)
	bulletPrefixRe    = regexp.MustCompile(`^[-*]\s+`)
	dateRe            = regexp.MustCompile(`^\d{4}-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12]\d|3[01])$`)
	shaRe             = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
)

// failureVerdicts are the verdict tokens Standard refuses.
var failureVerdicts = map[string]bool{
	"FAIL": true, "FAILED": true, "BLOCKED": true, "ERROR": true, "FAILURE": true,
}

// evidenceAliases maps every accepted label to its canonical field.
var evidenceAliases = map[string]string{
	"verdict":           "verdict",
	"status":            "verdict",
	"overall":           "verdict",
	"command":           "command",
	"exit":              "exit",
	"exit code":         "exit",
	"exit status":       "exit",
	"date":              "date",
	"commit":            "commit",
	"sha":               "commit",
	"revision":          "commit",
	"ready_for_archive": "ready_for_archive",
}

// parseEvidenceFields parses canonical evidence rows and reports repeated field
// identities.
func parseEvidenceFields(lines []string) (map[string]string, []string) {
	values := map[string]string{}
	seen := map[string]bool{}
	var duplicates []string
	for _, rawLine := range lines {
		line := strings.TrimSpace(strings.Trim(strings.TrimSpace(rawLine), "|"))
		line = bulletPrefixRe.ReplaceAllString(line, "")
		line = strings.TrimSpace(strings.ReplaceAll(line, "**", ""))

		var label, value string
		if strings.Contains(line, "|") {
			parts := strings.Split(line, "|")
			label = strings.TrimSpace(parts[0])
			value = strings.TrimSpace(parts[1])
		} else if strings.Contains(line, ":") {
			halves := strings.SplitN(line, ":", 2)
			label = strings.TrimSpace(halves[0])
			value = strings.TrimSpace(halves[1])
		} else {
			continue
		}
		key, ok := evidenceAliases[strings.TrimRight(strings.ToLower(label), ";")]
		if !ok {
			continue
		}
		if seen[key] {
			duplicates = append(duplicates, key)
			continue
		}
		seen[key] = true
		if value != "" {
			values[key] = strings.TrimSpace(strings.Trim(strings.TrimSpace(value), "`"))
		}
	}
	sort.Strings(duplicates)
	return values, duplicates
}

// extractEvidenceFields reads fields only from one exact `## Verify evidence`
// section.
func extractEvidenceFields(text string) (map[string]string, []string) {
	lines := markdownContentLines(splitLines(text))
	var headings []int
	for index, line := range lines {
		if evidenceHeadingRe.MatchString(line) {
			headings = append(headings, index)
		}
	}
	if len(headings) == 0 {
		return map[string]string{}, []string{
			"verify-report.md is missing canonical ## Verify evidence block"}
	}
	if len(headings) > 1 {
		return map[string]string{}, []string{
			"verify-report.md has duplicate canonical ## Verify evidence blocks"}
	}

	start := headings[0] + 1
	end := len(lines)
	for index := start; index < len(lines); index++ {
		if sectionHeadingRe.MatchString(lines[index]) {
			end = index
			break
		}
	}
	values, duplicates := parseEvidenceFields(lines[start:end])
	blockers := make([]string, 0, len(duplicates))
	for _, key := range duplicates {
		blockers = append(blockers, "verify-report.md has duplicate evidence label: "+key)
	}
	return values, blockers
}

// validDate checks both the YYYY-MM-DD shape and the calendar date.
func validDate(value string) bool {
	value = strings.Trim(value, "`")
	if !dateRe.MatchString(value) {
		return false
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

// checkVerifyEvidence returns the staged verify blockers; Light is always
// advisory.
func checkVerifyEvidence(changeDir, tier string) []string {
	if tier == TierLight {
		return nil
	}
	report := filepath.Join(changeDir, "verify-report.md")
	if !isFile(report) {
		return []string{tier + " verify evidence requires dedicated verify-report.md"}
	}

	text := readText(report)
	values, blockers := extractEvidenceFields(text)
	verdict := values["verdict"]
	if verdict == "" {
		blockers = append(blockers, "verify-report.md is missing Verdict/Status/Overall")
	} else {
		token := verdictToken(verdict)
		if tier == TierFull && token != "PASS" {
			blockers = append(blockers, "verify-report.md Verdict must be strict PASS for tier full")
		} else if tier == TierStandard && failureVerdicts[token] {
			blockers = append(blockers, "verify-report.md Verdict must not be FAIL or BLOCKED for tier standard")
		}
	}

	if values["command"] == "" {
		blockers = append(blockers, "verify-report.md is missing Command")
	}
	exit := strings.Fields(values["exit"])
	if len(exit) == 0 || strings.Trim(exit[0], "`.,;:") != "0" {
		blockers = append(blockers, "verify-report.md Exit/Exit code/Exit status must be 0")
	}
	if !validDate(values["date"]) {
		blockers = append(blockers, "verify-report.md is missing Date in valid YYYY-MM-DD format")
	}
	if !shaRe.MatchString(strings.Trim(values["commit"], "`")) {
		blockers = append(blockers, "verify-report.md is missing Commit/SHA/Revision (7-40 hex)")
	}
	if tier == TierFull && strings.ToLower(values["ready_for_archive"]) != "true" {
		blockers = append(blockers, "verify-report.md requires ready_for_archive: true for tier full")
	}
	if tier == TierFull {
		blockers = append(blockers, checkFullCriteriaMapping(changeDir, text)...)
	}
	return blockers
}

// verdictToken is the comparable first word of a verdict value.
func verdictToken(verdict string) string {
	fields := strings.Fields(verdict)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(strings.ToUpper(fields[0]), "`.,;:()")
}
