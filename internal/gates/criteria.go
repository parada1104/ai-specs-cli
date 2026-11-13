package gates

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	successCriteriaHeadingRe = regexp.MustCompile(`(?i)^[ ]{0,3}##[ \t]+Success criteria[ \t]*$`)
	mappingHeadingRe         = regexp.MustCompile(`(?i)^[ ]{0,3}##[ \t]+Success-criteria mapping[ \t]*$`)
	criterionItemRe          = regexp.MustCompile(`^(?:[-*+]|\d+[.)])[ \t]+(?:\[[ xX]\][ \t]*)?\S`)
	criterionMappingRe       = regexp.MustCompile(
		`(?i)^[ ]{0,3}[-*][ \t]+Criterion[ \t]+(\d+)[ \t]*:[ \t]*([^\s—–-]+)(?:[ \t]+[—–-].*)?[ \t]*$`)
)

// criteria is the authoritative Success Criteria declaration of one change.
type criteria struct {
	count  int
	source string
}

// parseSuccessCriteria parses criteria from exactly one authoritative planning
// artifact. proposal.md is authoritative whenever it exists; design.md is a
// fallback only when proposal.md is absent, never when proposal criteria are
// missing or empty. Duplicate headings are rejected because their meaning cannot
// be selected safely or deterministically.
func parseSuccessCriteria(folder string) (criteria, string) {
	source := filepath.Join(folder, "proposal.md")
	if !isFile(source) {
		design := filepath.Join(folder, "design.md")
		if !isFile(design) {
			return criteria{}, "verify-report.md Full mapping requires a non-empty ## Success Criteria " +
				"section in proposal.md or design.md"
		}
		source = design
	}

	filename := filepath.Base(source)
	lines := markdownContentLines(splitLines(readText(source)))
	var headings []int
	for index, line := range lines {
		if successCriteriaHeadingRe.MatchString(line) {
			headings = append(headings, index)
		}
	}
	if len(headings) > 1 {
		return criteria{}, "verify-report.md has duplicate ## Success Criteria headings in " + filename
	}
	if len(headings) == 0 {
		return criteria{}, "verify-report.md requires a ## Success Criteria section in " + filename
	}

	heading := headings[0]
	end := len(lines)
	for index := heading + 1; index < len(lines); index++ {
		if sectionHeadingRe.MatchString(lines[index]) {
			end = index
			break
		}
	}
	count := 0
	for _, line := range lines[heading+1 : end] {
		if criterionItemRe.MatchString(line) {
			count++
		}
	}
	if count == 0 {
		return criteria{}, "verify-report.md requires a non-empty ## Success Criteria section in " + filename
	}
	return criteria{count: count, source: filename}, ""
}

// checkFullCriteriaMapping requires one strict PASS mapping row for every
// declared success criterion.
func checkFullCriteriaMapping(folder, reportText string) []string {
	declared, issue := parseSuccessCriteria(folder)
	if declared.count == 0 {
		if issue == "" {
			issue = "verify-report.md could not parse Success Criteria"
		}
		return []string{issue}
	}

	lines := markdownContentLines(splitLines(reportText))
	var headings []int
	for index, line := range lines {
		if mappingHeadingRe.MatchString(line) {
			headings = append(headings, index)
		}
	}
	if len(headings) == 0 {
		return []string{"verify-report.md Full evidence requires canonical ## Success-criteria mapping block"}
	}
	if len(headings) > 1 {
		return []string{"verify-report.md has duplicate canonical ## Success-criteria mapping blocks"}
	}

	start := headings[0] + 1
	end := len(lines)
	for index := start; index < len(lines); index++ {
		if sectionHeadingRe.MatchString(lines[index]) {
			end = index
			break
		}
	}

	var blockers []string
	seen := map[int]bool{}
	for _, line := range lines[start:end] {
		match := criterionMappingRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		number, _ := strconv.Atoi(match[1])
		status := strings.ToUpper(match[2])
		if seen[number] {
			blockers = append(blockers, "verify-report.md has duplicate success-criteria mapping for criterion "+
				strconv.Itoa(number))
			continue
		}
		seen[number] = true
		if number < 1 || number > declared.count {
			blockers = append(blockers, "verify-report.md maps unknown success criterion "+strconv.Itoa(number)+
				" (expected 1-"+strconv.Itoa(declared.count)+" from "+declared.source+")")
		} else if status != "PASS" {
			blockers = append(blockers, "verify-report.md criterion "+strconv.Itoa(number)+
				" mapping must be strict PASS for tier full")
		}
	}

	for number := 1; number <= declared.count; number++ {
		if !seen[number] {
			blockers = append(blockers, "verify-report.md is missing success-criteria mapping for criterion "+
				strconv.Itoa(number))
		}
	}
	return blockers
}
