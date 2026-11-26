package gates

// The plan file (plan-build-openspec-odd-align P1): one ODD-shaped document per
// change, openspec/changes/<change>/tasks.md. The guardian parses it here; the
// criteria and evidence graders read the sections this parser exposes.
//
// The grammar is one compiled regexp per line kind. A line the grammar does not
// know is prose and is ignored; a missing or malformed required piece is a
// blocker, never a panic.

import (
	"fmt"
	"regexp"
	"strings"
)

// PlanSpec is one `## Specs` bullet: a verbatim human requirement.
type PlanSpec struct {
	ID   string // "S1"
	Text string
}

// PlanTask is one `## Tasks` table row.
type PlanTask struct {
	ID     string // "T1"
	Specs  string // raw Spec cell, e.g. "S1, S2" or "—"
	Text   string // the Tarea cell
	Route  string // the Ruta cell
	Commit string // the Commit cell
}

// PlanLogEntry is one `## Log` bullet: the human request or correction.
type PlanLogEntry struct {
	ID   string // "L1"
	Text string
}

// PlanVerification is one `## Verification` row: a verdict per spec.
type PlanVerification struct {
	ID       string // "S1"
	Verdict  string // "PASS", "FAIL" or "N/A"
	Evidence string // text after the dash, empty for a bare verdict
}

// Plan is the parsed plan file.
type Plan struct {
	Specs        []PlanSpec
	Tasks        []PlanTask
	Log          []PlanLogEntry
	Verification []PlanVerification
	// HasVerification reports a `## Verification` section at all, so an empty
	// or unparseable one is a coverage failure rather than a silent fallback.
	HasVerification bool
	// HasLog reports a `## Log` section at all: the tier matrix requires one from
	// standard up, and Light keeps it optional.
	HasLog bool
}

var (
	planSectionRe  = regexp.MustCompile(`^[ ]{0,3}##[ \t]+(\S.*?)[ \t]*$`)
	planSpecRe     = regexp.MustCompile(`^- \*\*(S\d+)\.\*\* +(\S.*)$`)
	planLogRe      = regexp.MustCompile(`^- \*\*(L\d+)\*\* +— +(\S.*)$`)
	planVerifyRe   = regexp.MustCompile(`^- \*\*(S\d+|Criterion \d+): +(PASS|FAIL|N/A)\*\* *(?:— *(.*))?$`)
	planTaskIDRe   = regexp.MustCompile(`^T\d+$`)
	planTableSepRe = regexp.MustCompile(`^:?-+:?$`)
)

// planSectionNames are the recognized sections; every other section is prose.
var planSectionNames = map[string]string{
	"specs":        "Specs",
	"tasks":        "Tasks",
	"log":          "Log",
	"verification": "Verification",
}

// planSection is one recognized section and the content lines under it.
type planSection struct {
	name  string
	lines []string
}

// ParsePlan parses one plan file; the blockers are empty when it is valid.
func ParsePlan(text string) (Plan, []string) {
	plan := Plan{}
	sections, blockers := splitPlanSections(markdownContentLines(splitLines(text)))

	specLines, ok := planSectionLines(sections, "specs")
	if !ok {
		blockers = append(blockers, "`## Specs` section is missing: add the `## Specs` section above `## Tasks`")
	} else {
		plan.Specs, blockers = parsePlanSpecs(specLines, blockers)
	}

	declared := map[string]bool{}
	for _, spec := range plan.Specs {
		declared[spec.ID] = true
	}

	taskLines, ok := planSectionLines(sections, "tasks")
	if !ok {
		blockers = append(blockers, "`## Tasks` section is missing: add the `## Tasks` table")
	} else {
		plan.Tasks, blockers = parsePlanTasks(taskLines, declared, blockers)
	}

	if logLines, ok := planSectionLines(sections, "log"); ok {
		plan.HasLog = true
		plan.Log, blockers = parsePlanLog(logLines, blockers)
	}

	if verifyLines, ok := planSectionLines(sections, "verification"); ok {
		plan.HasVerification = true
		plan.Verification, blockers = parsePlanVerification(verifyLines, blockers)
	}

	return plan, blockers
}

// splitPlanSections cuts the content lines into recognized sections and blocks
// a repeated section header.
func splitPlanSections(lines []string) ([]planSection, []string) {
	var sections []planSection
	var blockers []string
	counts := map[string]int{}
	current := -1
	for _, line := range lines {
		if !sectionHeadingRe.MatchString(line) {
			if current >= 0 {
				sections[current].lines = append(sections[current].lines, line)
			}
			continue
		}
		current = -1
		match := planSectionRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		name := strings.ToLower(match[1])
		if _, known := planSectionNames[name]; !known {
			continue
		}
		counts[name]++
		if counts[name] == 2 {
			blockers = append(blockers, fmt.Sprintf(
				"`## %s` appears more than once: keep a single `## %s` section",
				planSectionNames[name], planSectionNames[name]))
		}
		sections = append(sections, planSection{name: name})
		current = len(sections) - 1
	}
	return sections, blockers
}

// planSectionLines returns the lines of the first section with that name.
func planSectionLines(sections []planSection, name string) ([]string, bool) {
	for _, section := range sections {
		if section.name == name {
			return section.lines, true
		}
	}
	return nil, false
}

// parsePlanSpecs reads the `S#` bullets and enforces unique ids from S1 up.
func parsePlanSpecs(lines []string, blockers []string) ([]PlanSpec, []string) {
	var specs []PlanSpec
	seen := map[string]bool{}
	for _, line := range lines {
		match := planSpecRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		if seen[match[1]] {
			blockers = append(blockers, fmt.Sprintf(
				"`## Specs` has duplicate id %s: keep each `S#` unique", match[1]))
			continue
		}
		seen[match[1]] = true
		specs = append(specs, PlanSpec{ID: match[1], Text: match[2]})
	}
	if len(specs) == 0 {
		return specs, append(blockers, "`## Specs` must declare at least one `S#` bullet")
	}
	for index, spec := range specs {
		want := fmt.Sprintf("S%d", index+1)
		if spec.ID != want {
			return specs, append(blockers, fmt.Sprintf(
				"`## Specs` ids must ascend from S1 without gaps: expected %s but found %s", want, spec.ID))
		}
	}
	return specs, blockers
}

// parsePlanTasks reads the table: the header order is normative, every row
// carries a `T<n>` id, and every cited spec must be declared.
func parsePlanTasks(lines []string, declared map[string]bool, blockers []string) ([]PlanTask, []string) {
	var tasks []PlanTask
	header := true
	row := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := tableCells(trimmed)
		if isTableSeparator(cells) {
			continue
		}
		if header {
			header = false
			if !planTableHeaderOK(cells) {
				blockers = append(blockers, "`## Tasks` header must be | ID | Spec | Tarea | Ruta | Commit | in that order")
			}
			continue
		}
		row++
		if len(cells) != 5 {
			blockers = append(blockers, fmt.Sprintf(
				"`## Tasks` row %d has %d cells: give every row the five columns", row, len(cells)))
			continue
		}
		task := PlanTask{ID: cells[0], Specs: cells[1], Text: cells[2], Route: cells[3], Commit: cells[4]}
		if !planTaskIDRe.MatchString(task.ID) {
			blockers = append(blockers, fmt.Sprintf(
				"`## Tasks` row %d has no `T<n>` id: give every row its `T<n>` id", row))
			continue
		}
		if task.Specs == "" {
			blockers = append(blockers, fmt.Sprintf(
				"`## Tasks` row %s has an empty Spec cell: cite the `S#` it implements", task.ID))
		} else {
			blockers = append(blockers, undeclaredTaskSpecs(task, declared)...)
		}
		tasks = append(tasks, task)
	}
	if header {
		blockers = append(blockers, "`## Tasks` must be a table: start it with | ID | Spec | Tarea | Ruta | Commit |")
	}
	return tasks, blockers
}

// undeclaredTaskSpecs blocks a Spec cell that cites a spec outside `## Specs`;
// a dash or an empty cell means "no spec" and is accepted.
func undeclaredTaskSpecs(task PlanTask, declared map[string]bool) []string {
	cell := strings.TrimSpace(task.Specs)
	if cell == "—" || cell == "-" {
		return nil
	}
	var blockers []string
	for _, token := range strings.Split(cell, ",") {
		spec := strings.TrimSpace(token)
		if spec == "" || spec == "—" || spec == "-" || declared[spec] {
			continue
		}
		blockers = append(blockers, fmt.Sprintf(
			"`## Tasks` row %s cites undeclared spec %s: declare it under `## Specs`", task.ID, spec))
	}
	return blockers
}

// tableCells splits one markdown table row into trimmed cells.
func tableCells(line string) []string {
	inner := strings.Trim(strings.TrimSpace(line), "|")
	cells := strings.Split(inner, "|")
	for index, cell := range cells {
		cells[index] = strings.TrimSpace(cell)
	}
	return cells
}

// isTableSeparator reports the `| --- | --- |` row under a header.
func isTableSeparator(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if !planTableSepRe.MatchString(cell) {
			return false
		}
	}
	return true
}

// planTableHeaderOK checks the five header cells in their normative order.
func planTableHeaderOK(cells []string) bool {
	if len(cells) != 5 {
		return false
	}
	third := strings.ToLower(cells[2])
	return strings.ToLower(cells[0]) == "id" &&
		strings.ToLower(cells[1]) == "spec" &&
		(third == "tarea" || third == "task") &&
		strings.ToLower(cells[3]) == "ruta" &&
		strings.ToLower(cells[4]) == "commit"
}

// parsePlanLog reads the log bullets; a `## Log` section must start at L1.
func parsePlanLog(lines []string, blockers []string) ([]PlanLogEntry, []string) {
	var entries []PlanLogEntry
	hasL1 := false
	for _, line := range lines {
		match := planLogRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		if match[1] == "L1" {
			hasL1 = true
		}
		entries = append(entries, PlanLogEntry{ID: match[1], Text: match[2]})
	}
	if !hasL1 {
		return entries, append(blockers,
			"`## Log` must start at L1: add the `- **L1** — <request>` entry")
	}
	return entries, blockers
}

// parsePlanVerification reads the verdict rows; `S1` and the legacy `Criterion 1`
// label are both accepted, and an N/A verdict needs a reason.
func parsePlanVerification(lines []string, blockers []string) ([]PlanVerification, []string) {
	var rows []PlanVerification
	for _, line := range lines {
		match := planVerifyRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		row := PlanVerification{ID: match[1], Verdict: match[2], Evidence: strings.TrimSpace(match[3])}
		if row.Verdict == "N/A" && row.Evidence == "" {
			blockers = append(blockers, fmt.Sprintf(
				"`## Verification` row %s is N/A without a reason: write `- **%s: N/A** — <why>`",
				row.ID, row.ID))
		}
		rows = append(rows, row)
	}
	return rows, blockers
}
