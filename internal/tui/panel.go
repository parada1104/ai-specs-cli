package tui

import (
	"fmt"
	"strings"
)

// Counts is the doctor's tally as both the summary and the panel display it.
type Counts struct {
	OK    int
	Info  int
	Warn  int
	Error int
}

// PanelCheck is one non-OK check line: the doctor's severity label and its text.
type PanelCheck struct {
	Label string
	Text  string
}

// PanelData is everything the panel shows. Topology is already formatted by the
// caller and an empty string omits the row entirely, which is hub.py's conditional.
type PanelData struct {
	Version  string
	Target   string
	Topology string
	Counts   Counts
	Checks   []PanelCheck
}

// Headline is the frozen headline vocabulary, shared by the summary and the panel
// because both print it: errors say `error(s)` LITERALLY and win over warnings, while
// warnings pluralize properly.
func Headline(c Counts) string {
	switch {
	case c.Error > 0:
		return fmt.Sprintf("%d error(s)", c.Error)
	case c.Warn == 1:
		return "1 warning"
	case c.Warn > 1:
		return fmt.Sprintf("%d warnings", c.Warn)
	default:
		return "healthy"
	}
}

// RenderPanel draws what rich drew: a rounded box titled `ai-specs` whose rows are
// version, target, an optional topology, the summary, and one row per non-OK check.
// The border is red with any error, yellow with any warning, green otherwise. With
// noColor the box carries no escapes at all, which is what keeps the panel readable
// in a redirected log.
func RenderPanel(d PanelData, noColor bool) string {
	rows := [][2]string{
		{"version", d.Version},
		{"target", d.Target},
	}
	if d.Topology != "" {
		rows = append(rows, [2]string{"topology", d.Topology})
	}
	rows = append(rows, [2]string{"Summary:", fmt.Sprintf("%d OK, %d INFO, %d WARN, %d ERROR (%s)",
		d.Counts.OK, d.Counts.Info, d.Counts.Warn, d.Counts.Error, Headline(d.Counts))})
	for _, check := range d.Checks {
		rows = append(rows, [2]string{check.Label, check.Text})
	}

	labelWidth, textWidth := 0, 0
	for _, row := range rows {
		if len(row[0]) > labelWidth {
			labelWidth = len(row[0])
		}
		if len(row[1]) > textWidth {
			textWidth = len(row[1])
		}
	}
	inner := labelWidth + 2 + textWidth

	colour, reset := borderColour(d.Counts), ""
	if !noColor && colour != "" {
		reset = "\x1b[0m"
	} else {
		colour = ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s╭─ ai-specs %s╮%s\n", colour, strings.Repeat("─", max(inner-12, 0)), reset)
	for _, row := range rows {
		fmt.Fprintf(&b, "%s│%s %-*s  %-*s %s│%s\n", colour, reset, labelWidth, row[0], textWidth, row[1], colour, reset)
	}
	fmt.Fprintf(&b, "%s╰%s╯%s\n", colour, strings.Repeat("─", inner+2), reset)
	return b.String()
}

func borderColour(c Counts) string {
	switch {
	case c.Error > 0:
		return "\x1b[31m"
	case c.Warn > 0:
		return "\x1b[33m"
	default:
		return "\x1b[32m"
	}
}
