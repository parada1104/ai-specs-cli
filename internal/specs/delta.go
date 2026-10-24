package specs

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// nextTopLevelSection is the offset of the next `## ` heading at or after from,
// or the document length when there is none.
func nextTopLevelSection(markdown string, from int) int {
	if loc := topLevelHeadingRe.FindStringIndex(markdown[from:]); loc != nil {
		return from + loc[0]
	}
	return len(markdown)
}

// cleanRequirementContent trims the block and its optional trailing `---`.
func cleanRequirementContent(content string) string {
	return rstrip(trailingSeparatorRe.ReplaceAllString(rstrip(content), ""))
}

// ParseRequirementBlocks parses `### Requirement:` blocks, keeping their full
// body and offsets.
func ParseRequirementBlocks(markdown string) []RequirementBlock {
	source := normalize(markdown)
	matches := requirementHeadingRe.FindAllStringSubmatchIndex(source, -1)
	blocks := make([]RequirementBlock, 0, len(matches))
	for i, match := range matches {
		end := len(source)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		} else {
			end = nextTopLevelSection(source, match[1])
		}
		blocks = append(blocks, RequirementBlock{
			Name:    strings.TrimSpace(source[match[2]:match[3]]),
			Content: cleanRequirementContent(source[match[0]:end]),
			Start:   match[0],
			End:     end,
		})
	}
	return blocks
}

// ParseDeltaSpec parses a delta spec, rejecting unsupported or contradictory
// operations.
func ParseDeltaSpec(markdown string) (DeltaSpec, error) {
	source := normalize(markdown)
	sections := deltaSectionRe.FindAllStringSubmatchIndex(source, -1)
	var delta DeltaSpec
	for i, match := range sections {
		label := strings.ToUpper(source[match[2]:match[3]])
		if label == "RENAMED" {
			return DeltaSpec{}, errors.New("## RENAMED Requirements is unsupported: the archive " +
				"contract defines ADDED/MODIFIED/REMOVED only — rewrite the delta as REMOVED plus " +
				"ADDED before promoting")
		}
		operation := strings.ToLower(label)
		if operation != "added" && operation != "modified" && operation != "removed" {
			return DeltaSpec{}, fmt.Errorf("## %s Requirements is unsupported", label)
		}
		end := len(source)
		if i+1 < len(sections) {
			end = sections[i+1][0]
		}
		blocks := ParseRequirementBlocks(source[match[1]:end])
		switch operation {
		case "added":
			delta.Added = append(delta.Added, blocks...)
		case "modified":
			delta.Modified = append(delta.Modified, blocks...)
		default:
			delta.Removed = append(delta.Removed, blocks...)
		}
	}

	seen := map[string]string{}
	for _, operation := range deltaOperations {
		for _, block := range delta.blocks(operation) {
			if previous, ok := seen[block.Name]; ok {
				return DeltaSpec{}, fmt.Errorf(
					`duplicate delta operation for requirement "%s" (%s and %s)`,
					block.Name, previous, strings.ToUpper(operation))
			}
			seen[block.Name] = strings.ToUpper(operation)
		}
	}
	return delta, nil
}

// namedSection returns the `## name` section verbatim, without its trailing
// whitespace.
func namedSection(markdown, name string) (string, bool) {
	heading := regexp.MustCompile(`(?m)^##[ \t]+` + regexp.QuoteMeta(name) + `[ \t]*$`)
	loc := heading.FindStringIndex(markdown)
	if loc == nil {
		return "", false
	}
	return rstrip(markdown[loc[0]:nextTopLevelSection(markdown, loc[1])]), true
}

func joinBlockContents(blocks []RequirementBlock) string {
	contents := make([]string, 0, len(blocks))
	for _, block := range blocks {
		contents = append(contents, strings.TrimSpace(block.Content))
	}
	return strings.Join(contents, requirementBlockPadding)
}
