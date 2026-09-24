package telegram

import "testing"

func TestTelegramTables(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{
			name: "narrow table becomes an aligned grid",
			in:   "Stato:\n\n| Env | Stato |\n|---|:---:|\n| dev | `ok` |\n| prod | **down** |\n\nFine.",
			want: "Stato:\n\n```\n┌──────┬───────┐\n│ Env  │ Stato │\n├──────┼───────┤\n│ dev  │ ok    │\n│ prod │ down  │\n└──────┴───────┘\n```\n\nFine.",
		},
		{
			// The reply of 2026-09-24 that read badly as a raw pipe table.
			name: "wide table becomes one card per row",
			in: "Fatto.\n\n| Richiesta | Comportamento ora |\n|---|---|\n" +
				"| Il pane non si chiude | Default: locale invariato, pane e transcript restano aperti. Resta `--close-source`. |\n" +
				"| Workspace e tab identici | Lo script legge le etichette della sorgente |\n\nProve eseguite.",
			want: "Fatto.\n\n" +
				"- **Richiesta**: Il pane non si chiude\n  **Comportamento ora**: Default: locale invariato, pane e transcript restano aperti. Resta `--close-source`.\n\n" +
				"- **Richiesta**: Workspace e tab identici\n  **Comportamento ora**: Lo script legge le etichette della sorgente\n\nProve eseguite.",
		},
		{
			name: "empty cells are left out of a card",
			in:   "| Nome | Nota | Esito |\n|---|---|---|\n| una riga abbastanza lunga da non entrare nella griglia |  | ok |",
			want: "- **Nome**: una riga abbastanza lunga da non entrare nella griglia\n  **Esito**: ok",
		},
		{
			// Seen in a real reply on 2026-09-24.
			name: "an escaped pipe stays inside its cell",
			in:   "| Passo | Prova |\n|---|---|\n| 1 | `chezmoi managed \\| grep -c move-to-agent` = 8, piu' altro testo |",
			want: "- **Passo**: 1\n  **Prova**: `chezmoi managed | grep -c move-to-agent` = 8, piu' altro testo",
		},
		{
			name: "rows without the outer pipes",
			in:   "a | b\n--|--\n1 | 2",
			want: "```\n┌───┬───┐\n│ a │ b │\n├───┼───┤\n│ 1 │ 2 │\n└───┴───┘\n```",
		},
		{
			name: "a table inside a code fence is left alone",
			in:   "```\n| a | b |\n|---|---|\n| 1 | 2 |\n```",
			want: "```\n| a | b |\n|---|---|\n| 1 | 2 |\n```",
		},
		{
			name: "pipes without a rule row are prose",
			in:   "a | b\nc | d",
			want: "a | b\nc | d",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := telegramTables(tt.in); got != tt.want {
				t.Errorf("telegramTables() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}
