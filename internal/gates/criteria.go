package gates

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	successCriteriaHeadingRe = regexp.MustCompile(`(?i)^[ ]{0,3}##[ \t]+Success criteria[ \t]*$`)
	criterionItemRe          = regexp.MustCompile(`^(?:[-*+]|\d+[.)])[ \t]+(?:\[[ xX]\][ \t]*)?\S`)
	criterionRowRe           = regexp.MustCompile(`(?i)^Criterion[ \t]+(\d+)$`)
	planSpecIDRe             = regexp.MustCompile(`^S(\d+)$`)
)

// criteria is the authoritative requirement declaration of one change: the
// identifiers in document order and the file they came from.
type criteria struct {
	ids    []string
	source string
}

// rangeLabel names the declared identifiers for an undeclared-row blocker.
func (c criteria) rangeLabel() string {
	if len(c.ids) == 0 {
		return "none"
	}
	return c.ids[0] + "-" + c.ids[len(c.ids)-1]
}

// parseRequirementsFor resolves the requirement source chain (P2) from a plan
// that does not live inside the folder, as the ODD plan at odd/tasks/<change>.md
// does (C3): the plan's `## Specs` when it declares at least one `S#`, otherwise
// the legacy `## Success Criteria` of proposal.md, otherwise of design.md.
func parseRequirementsFor(folder, planText string) (criteria, string) {
	plan, _ := ParsePlan(planText)
	if len(plan.Specs) > 0 {
		ids := make([]string, len(plan.Specs))
		for index, spec := range plan.Specs {
			ids[index] = spec.ID
		}
		return criteria{ids: ids, source: "tasks.md"}, ""
	}
	return parseSuccessCriteria(folder)
}

// parseSuccessCriteria parses the legacy criteria source. proposal.md is
// authoritative whenever it exists; design.md is a fallback only when
// proposal.md is absent, never when proposal criteria are missing or empty.
// Duplicate headings are rejected because their meaning cannot be selected
// safely or deterministically.
func parseSuccessCriteria(folder string) (criteria, string) {
	source := filepath.Join(folder, "proposal.md")
	if !isFile(source) {
		design := filepath.Join(folder, "design.md")
		if !isFile(design) {
			return criteria{}, "requirements are missing: add at least one `S#` under `## Specs` in " +
				"tasks.md, or a non-empty ## Success Criteria section in proposal.md or design.md"
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
	ids := make([]string, count)
	for number := 1; number <= count; number++ {
		ids[number-1] = "Criterion " + strconv.Itoa(number)
	}
	return criteria{ids: ids, source: filename}, ""
}

// requirementNumber is the 1-based position a verification or mapping label
// denotes; `S1` and the legacy `Criterion 1` are interchangeable.
func requirementNumber(label string) (int, error) {
	label = strings.TrimSpace(label)
	if match := planSpecIDRe.FindStringSubmatch(label); match != nil {
		number, err := strconv.Atoi(match[1])
		return number, err
	}
	if match := criterionRowRe.FindStringSubmatch(label); match != nil {
		number, err := strconv.Atoi(match[1])
		return number, err
	}
	return 0, strconv.ErrSyntax
}
