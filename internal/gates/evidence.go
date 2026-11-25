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
	// exitCodeRe finds the exit code an evidence row records, so a row that
	// records a non-zero one blocks (R6).
	exitCodeRe = regexp.MustCompile(`(?i)\bexit(?:ed)?\s*(?:code|status)?\s*[:=]?\s*(-?\d+)\b`)
	// failedWordRe finds a failing word a row records (R6).
	failedWordRe = regexp.MustCompile(`(?i)\b(failed|fail)\b`)
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
// advisory. Evidence is read from the plan's `## Verification` when the plan has
// that section, otherwise from the legacy verify-report.md.
func checkVerifyEvidence(changeDir, tier string) []string {
	if tier == TierLight {
		return nil
	}
	plan, _ := ParsePlan(readText(filepath.Join(changeDir, "tasks.md")))
	if plan.HasVerification {
		return checkPlanVerification(filepath.Join(changeDir, "tasks.md"))
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
		if failureVerdicts[token] {
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
	return blockers
}

// checkPlanVerification grades the plan's `## Verification` rows: coverage of
// every declared requirement, a single row per requirement, exactly one
// backticked command per graded row with no prose around or inside it, and no
// recorded failing run. An `N/A` row carries its reason and is exempt from the
// command requirement (C').
func checkPlanVerification(planPath string) []string {
	plan, _ := ParsePlan(readText(planPath))
	declared, issue := parseRequirementsFor(filepath.Dir(planPath), readText(planPath))
	if len(declared.ids) == 0 {
		return []string{issue}
	}
	var blockers []string
	covered := make([]bool, len(declared.ids))
	seen := map[int]bool{}
	for _, row := range plan.Verification {
		number, err := requirementNumber(row.ID)
		if err != nil || number < 1 || number > len(declared.ids) {
			blockers = append(blockers, "verification has a row for "+row.ID+
				" but the declared requirements are "+declared.rangeLabel()+
				": remove the row or declare "+row.ID)
			continue
		}
		id := declared.ids[number-1]
		if seen[number] {
			blockers = append(blockers, "verification has more than one row for "+id+
				": keep a single verdict per requirement")
			continue
		}
		seen[number] = true
		covered[number-1] = true
		if row.Verdict == "N/A" {
			continue // an N/A row carries its reason and records no run
		}
		// Prose is not evidence: a row that does not record exactly one command,
		// opening the evidence and with no other backticked value beside it, is
		// not a run (R6).
		if _, ok := verificationCommand(row.Evidence); !ok {
			blockers = append(blockers, verificationCommandBlocker(id, row.Evidence))
		}
		if failed := recordedFailure(row.Evidence); failed != "" {
			blockers = append(blockers, "verification for "+id+" records a failing run ("+failed+
				"): record a passing run or write \"- **"+id+": N/A** — <why>\"")
		}
		switch row.Verdict {
		case "FAIL":
			blockers = append(blockers, `verification for `+id+` is FAIL: fix it or record "- **`+
				id+`: N/A** — <why>"`)
		}
	}
	for index, ok := range covered {
		if ok {
			continue
		}
		id := declared.ids[index]
		blockers = append(blockers, `verification is missing a row for `+id+`: add "- **`+
			id+`: PASS** — <evidence>"`)
	}
	return blockers
}

// verificationCommand reads the single command a row records: the evidence opens
// with one backticked span holding a command, and a second backticked value is
// accepted only after trailing notes — a bare `, `next“ is a second command, not
// a note.
func verificationCommand(evidence string) (string, bool) {
	if !strings.HasPrefix(evidence, "`") {
		return "", false
	}
	rest := evidence[1:]
	end := strings.Index(rest, "`")
	if end < 0 {
		return "", false
	}
	command := strings.TrimSpace(rest[:end])
	if command == "" || strings.ContainsAny(command, "\r\n") {
		return "", false
	}
	tail := rest[end+1:]
	if next := strings.Index(tail, "`"); next >= 0 {
		between := strings.TrimSpace(tail[:next])
		if strings.Trim(between, ",;:—-– ") == "" {
			return "", false
		}
	}
	return command, true
}

// verificationCommandBlocker says what the row must record instead.
func verificationCommandBlocker(id, evidence string) string {
	if !strings.Contains(evidence, "`") {
		return "verification for " + id +
			` must name the command it ran in backticks: record "- **` + id +
			": PASS** — `go test ./...`\""
	}
	return "verification for " + id +
		` must record exactly one command in backticks, with no prose around or inside it: ` +
		`record "- **` + id + ": PASS** — `go test ./...`, exit 0\""
}

// recordedFailure names the failing token a row's evidence records, or "" when it
// records none.
func recordedFailure(evidence string) string {
	for _, match := range exitCodeRe.FindAllStringSubmatch(evidence, -1) {
		if strings.TrimLeft(match[1], "+") != "0" {
			return match[0]
		}
	}
	return failedWordRe.FindString(evidence)
}

// verdictToken is the comparable first word of a verdict value.
func verdictToken(verdict string) string {
	fields := strings.Fields(verdict)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(strings.ToUpper(fields[0]), "`.,;:()")
}
