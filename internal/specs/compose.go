package specs

import (
	"sort"
	"strings"
)

// renderNewCanonical builds a canonical spec for a brand-new domain.
func renderNewCanonical(domain, deltaText string, added []RequirementBlock) string {
	parts := []string{"# " + domain + " Specification"}
	if purpose, ok := namedSection(deltaText, "Purpose"); ok && purpose != "" {
		parts = append(parts, purpose)
	}
	parts = append(parts, "## Requirements")
	parts = append(parts, joinBlockContents(added))
	return rstrip(strings.Join(parts, requirementBlockPadding)) + "\n"
}

// appendAdded inserts the new requirements at the end of `## Requirements`.
func appendAdded(markdown string, added []RequirementBlock) string {
	if len(added) == 0 {
		return markdown
	}
	addition := joinBlockContents(added)
	loc := requirementsSectionRe.FindStringIndex(markdown)
	if loc == nil {
		return rstrip(markdown) + "\n\n## Requirements\n\n" + addition + "\n"
	}
	sectionEnd := nextTopLevelSection(markdown, loc[1])
	before := rstrip(markdown[:sectionEnd])
	after := strings.TrimLeft(markdown[sectionEnd:], "\n")
	if after != "" {
		return before + "\n\n" + addition + "\n\n" + after
	}
	return before + "\n\n" + addition + "\n"
}

// Compose composes one delta into canonical text, reporting blockers or a no-op.
// A nil canonical means the domain has no canonical spec yet.
func Compose(canonical *string, deltaText, domain string, allowRemoved bool) Composition {
	delta, err := ParseDeltaSpec(deltaText)
	if err != nil {
		content := ""
		if canonical != nil {
			content = *canonical
		}
		return Composition{Content: content, Blockers: []string{err.Error()}}
	}

	if canonical == nil {
		var blockers []string
		if len(delta.Modified) > 0 {
			blockers = append(blockers, "MODIFIED requirement "+quotedNames(delta.Modified)+
				" has no canonical spec to modify; MODIFIED requires an existing "+
				"openspec/specs/<domain>/spec.md")
		}
		if len(delta.Removed) > 0 {
			blockers = append(blockers, "REMOVED requirement "+quotedNames(delta.Removed)+
				" has no canonical spec to delete from")
		}
		if len(blockers) > 0 {
			return Composition{Content: "", Blockers: blockers}
		}
		if len(delta.Added) == 0 {
			return Composition{Content: ""}
		}
		pending := make([]string, 0, len(delta.Added))
		for _, block := range delta.Added {
			pending = append(pending, "ADDED "+block.Name)
		}
		return Composition{
			Changed: true,
			Content: renderNewCanonical(domain, deltaText, delta.Added),
			Pending: pending,
		}
	}

	canonicalText := *canonical
	canonicalMap := map[string]RequirementBlock{}
	var duplicateBlockers []string
	for _, block := range ParseRequirementBlocks(canonicalText) {
		if _, ok := canonicalMap[block.Name]; ok {
			duplicateBlockers = append(duplicateBlockers,
				`canonical spec declares requirement "`+block.Name+`" more than once`)
			continue
		}
		canonicalMap[block.Name] = block
	}
	if len(duplicateBlockers) > 0 {
		return Composition{Content: canonicalText, Blockers: duplicateBlockers}
	}

	var applied, pending, blockers []string
	var additions []RequirementBlock
	type replacement struct {
		target  RequirementBlock
		content string
	}
	var replacements []replacement

	for _, block := range delta.Added {
		target, ok := canonicalMap[block.Name]
		switch {
		case !ok:
			additions = append(additions, block)
			pending = append(pending, "ADDED "+block.Name)
		case target.Content == block.Content:
			applied = append(applied, "ADDED "+block.Name+" (already canonical)")
		default:
			blockers = append(blockers, `ADDED requirement "`+block.Name+`" collides with an `+
				"existing canonical requirement with different content; use MODIFIED for an "+
				"intentional replacement, or rename the requirement")
		}
	}

	for _, block := range delta.Modified {
		target, ok := canonicalMap[block.Name]
		switch {
		case !ok:
			blockers = append(blockers, `MODIFIED requirement "`+block.Name+`" does not exist in `+
				"the canonical spec; there is nothing safe to replace")
		case target.Content == block.Content:
			applied = append(applied, "MODIFIED "+block.Name+" (already canonical)")
		default:
			replacements = append(replacements, replacement{target: target, content: strings.TrimSpace(block.Content)})
			pending = append(pending, "MODIFIED "+block.Name)
		}
	}

	for _, block := range delta.Removed {
		target, ok := canonicalMap[block.Name]
		switch {
		case !ok:
			applied = append(applied, "REMOVED "+block.Name+" (already absent)")
		case !allowRemoved:
			blockers = append(blockers, `REMOVED requirement "`+block.Name+`" is destructive and `+
				"needs the explicit approval path; re-run the promoter with --allow-removed after "+
				"the removal was approved")
		default:
			replacements = append(replacements, replacement{target: target, content: ""})
			pending = append(pending, "REMOVED "+block.Name)
		}
	}

	if len(blockers) > 0 {
		return Composition{Content: canonicalText, Applied: applied, Blockers: blockers}
	}
	if len(additions) == 0 && len(replacements) == 0 {
		return Composition{Content: canonicalText, Applied: applied}
	}

	// Descending offsets: every edit keeps the earlier offsets valid.
	sort.SliceStable(replacements, func(i, j int) bool {
		return replacements[i].target.Start > replacements[j].target.Start
	})
	result := canonicalText
	for _, repl := range replacements {
		prefix := rstrip(result[:repl.target.Start])
		suffix := strings.TrimLeft(result[repl.target.End:], "\n")
		if repl.content != "" {
			result = rstrip(prefix+"\n\n"+repl.content+"\n\n"+suffix) + "\n"
			continue
		}
		result = rstrip(prefix+"\n\n"+suffix) + "\n"
	}
	result = rstrip(appendAdded(result, additions)) + "\n"
	return Composition{
		Changed: result != canonicalText,
		Content: result,
		Applied: applied,
		Pending: pending,
	}
}

func quotedNames(blocks []RequirementBlock) string {
	names := make([]string, 0, len(blocks))
	for _, block := range blocks {
		names = append(names, `"`+block.Name+`"`)
	}
	return strings.Join(names, ", ")
}
