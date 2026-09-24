package domain

import (
	"regexp"
	"strings"
)

// groundItem matches one "N. Label" or "N) Label" row, with the optional
// cursor Claude Code or Pi draws before the selected option. Rows are also
// matched inside Pi's overlay table cells: the caller splits lines on the
// box edge before matching.
var groundItem = regexp.MustCompile(`^\s*(?:[❯→]\s*)?([1-9])[\.\)]\s+(\S.*)$`)

// groundTailLines is how far from the bottom the last option may sit: a
// numbered list higher up belongs to the transcript, not the question, so
// it must not lend its rows to LLM-proposed buttons.
const groundTailLines = 8

// GroundChoices validates LLM-proposed options against the screen and
// reports whether they may become buttons. Every option needs its number
// (1, 2, 3, … without gaps, from minChoices to MaxChoiceButtons) on a
// screen row "N. Label" or "N) Label" whose label matches after whitespace
// normalisation, the rows in screen order, the last one among the last
// groundTailLines non-empty rows. Anything else (an invented label, a gap,
// a list in the transcript above the question) grounds nothing and the
// post goes out text-only. Labels come back whitespace-normalised.
func GroundChoices(screen string, choices []Choice) ([]Choice, bool) {
	filtered := make([]Choice, 0, len(choices))
	for _, c := range choices {
		if serviceRank(strings.TrimSuffix(stripCheckbox(normLabel(c.Label)), ".")) < 0 {
			filtered = append(filtered, c)
		}
	}
	choices = filtered
	if len(choices) < minChoices || len(choices) > MaxChoiceButtons {
		return nil, false
	}
	want := make([]string, len(choices))
	for i, c := range choices {
		if c.Number != i+1 {
			return nil, false
		}
		label := normLabel(c.Label)
		if label == "" {
			return nil, false
		}
		want[i] = label
	}
	lines := strings.Split(strings.ReplaceAll(screen, "\r\n", "\n"), "\n")
	// after[i] counts the non-empty lines below line i.
	after := make([]int, len(lines)+1)
	for i := len(lines) - 1; i >= 0; i-- {
		after[i] = after[i+1]
		if strings.TrimSpace(lines[i]) != "" {
			after[i]++
		}
	}
	type match struct {
		line   int
		number int
		label  string
	}
	var matches []match
	for i, line := range lines {
		cells := strings.Split(line, "│")
		for _, cell := range cells {
			m := groundItem.FindStringSubmatch(strings.TrimRight(cell, " \t\r"))
			if m == nil {
				continue
			}
			matches = append(matches, match{line: i, number: int(m[1][0] - '0'), label: normLabel(m[2])})
		}
	}
	// Walk from the bottom so a transcript list above the question cannot
	// steal the grounding: the last option must sit near the bottom, and
	// past it only dialog footers may follow (ParseDialog's filler rule),
	// so a list closed by transcript prose grounds nothing.
	picked := make([]Choice, len(choices))
	next, bound, end := len(choices), len(lines), -1
	for i := len(matches) - 1; i >= 0 && next > 0; i-- {
		m := matches[i]
		if m.line >= bound || m.number != next || m.label != want[next-1] {
			continue
		}
		if next == len(choices) {
			if after[m.line] >= groundTailLines {
				return nil, false
			}
			end = m.line
		}
		picked[next-1] = Choice{Number: m.number, Label: m.label}
		next, bound = next-1, m.line
	}
	if next > 0 {
		return nil, false
	}
	for _, line := range lines[end+1:] {
		if strings.TrimSpace(line) == "" || isChoiceFiller(line) || hasGroundRow(line) {
			continue
		}
		return nil, false
	}
	return picked, true
}

// hasGroundRow reports whether any table cell of the line holds an option
// row; trailing rows of a longer list still belong to a dialog.
func hasGroundRow(line string) bool {
	for _, cell := range strings.Split(line, "│") {
		if groundItem.FindStringSubmatch(strings.TrimRight(cell, " \t\r")) != nil {
			return true
		}
	}
	return false
}

// normLabel collapses every whitespace run to one space for comparison.
func normLabel(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// PressConfirm maps an agent kind to its answer keystroke: false sends the
// digit alone (Claude Code answers on the digit), true sends digit plus
// enter (Pi's ask_user only moves the selection). Measured 2026-09-24 for
// claude and pi; a kind outside the table (Codex until its keys are
// measured on a live pane, anything new) reports ok false and gets text
// only, never LLM-proposed buttons.
func PressConfirm(kind string) (confirm, ok bool) {
	switch kind {
	case "claude":
		return false, true
	case "pi":
		return true, true
	default:
		return false, false
	}
}
