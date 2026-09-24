package telegram

import (
	"strings"
	"unicode/utf8"
)

// gridMaxWidth is the widest table, borders included, still drawn as a
// grid: about what a phone shows of a monospace line without wrapping.
// A wider table becomes one card per row.
const gridMaxWidth = 40

// gridMarkup is the inline Markdown dropped from a grid cell, since the
// grid is a code block and would show it raw.
var gridMarkup = strings.NewReplacer("`", "", "**", "")

// telegramTables rewrites the pipe tables of a Markdown reply for
// Telegram, which has no table markup: a table that fits gridMaxWidth
// becomes a fenced block with the columns aligned and box-drawing
// borders; a wider one becomes a list with one entry per row, every cell
// on its own line after its header in bold, so long cells wrap as prose
// on a phone. It runs before splitMarkdown, so the parts are budgeted on
// the text that is sent. Tables inside code fences are left alone.
// ponytail: widths count runes, so a column with wide glyphs (CJK, emoji)
// is misaligned by one cell per glyph; a width table fixes that if it
// shows up.
func telegramTables(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	fence := ""
	for i := 0; i < len(lines); {
		line := lines[i]
		if m := mdFence.FindStringSubmatch(line); m != nil && (fence == "" || strings.HasPrefix(strings.TrimSpace(line), fence)) {
			if fence == "" {
				fence = m[1]
			} else {
				fence = ""
			}
			out = append(out, line)
			i++
			continue
		}
		if fence == "" && strings.Contains(line, "|") && i+1 < len(lines) && strings.Contains(lines[i+1], "|") && mdTableRule.MatchString(lines[i+1]) {
			header := tableCells(line)
			var rows [][]string
			for i += 2; i < len(lines) && strings.Contains(lines[i], "|"); i++ {
				rows = append(rows, fitCells(tableCells(lines[i]), len(header)))
			}
			out = append(out, renderTable(header, rows))
			continue
		}
		out = append(out, line)
		i++
	}
	return strings.Join(out, "\n")
}

// tableCells splits one table row into trimmed cells, outer pipes optional;
// an escaped pipe (\|) is cell text and comes out as a plain |.
func tableCells(line string) []string {
	line = strings.ReplaceAll(strings.TrimSpace(line), `\|`, "\x00")
	line = strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|")
	cells := strings.Split(line, "|")
	for i := range cells {
		cells[i] = strings.ReplaceAll(strings.TrimSpace(cells[i]), "\x00", "|")
	}
	return cells
}

// fitCells pads or cuts a row to n cells.
func fitCells(cells []string, n int) []string {
	for len(cells) < n {
		cells = append(cells, "")
	}
	return cells[:n]
}

// renderTable picks the grid or the cards for one table.
func renderTable(header []string, rows [][]string) string {
	widths := make([]int, len(header))
	for _, row := range append([][]string{header}, rows...) {
		for c, cell := range row {
			widths[c] = max(widths[c], utf8.RuneCountInString(gridMarkup.Replace(cell)))
		}
	}
	total := 3*len(widths) + 1
	for _, w := range widths {
		total += w
	}
	if total <= gridMaxWidth {
		return renderGrid(header, rows, widths)
	}
	return renderCards(header, rows)
}

func renderGrid(header []string, rows [][]string, widths []int) string {
	rule := func(left, mid, right string) string {
		segs := make([]string, len(widths))
		for c, w := range widths {
			segs[c] = strings.Repeat("─", w+2)
		}
		return left + strings.Join(segs, mid) + right
	}
	row := func(cells []string) string {
		segs := make([]string, len(widths))
		for c, w := range widths {
			cell := gridMarkup.Replace(cells[c])
			segs[c] = cell + strings.Repeat(" ", w-utf8.RuneCountInString(cell))
		}
		return "│ " + strings.Join(segs, " │ ") + " │"
	}
	lines := []string{"```", rule("┌", "┬", "┐"), row(header), rule("├", "┼", "┤")}
	for _, r := range rows {
		lines = append(lines, row(r))
	}
	return strings.Join(append(lines, rule("└", "┴", "┘"), "```"), "\n")
}

func renderCards(header []string, rows [][]string) string {
	var cards []string
	for _, r := range rows {
		var lines []string
		for c, cell := range r {
			if cell == "" {
				continue
			}
			if header[c] != "" {
				cell = "**" + header[c] + "**: " + cell
			}
			prefix := "  "
			if len(lines) == 0 {
				prefix = "- "
			}
			lines = append(lines, prefix+cell)
		}
		if len(lines) > 0 {
			cards = append(cards, strings.Join(lines, "\n"))
		}
	}
	return strings.Join(cards, "\n\n")
}
