package domain_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

func TestGroundChoices(t *testing.T) {
	question := "What now?\n\n1. Red\n2. Green\n3. Blue\n"
	cases := []struct {
		name    string
		screen  string
		choices []domain.Choice
		want    []domain.Choice
		ok      bool
	}{
		{"plain question", question,
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 2, Label: "Green"}, {Number: 3, Label: "Blue"}},
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 2, Label: "Green"}, {Number: 3, Label: "Blue"}}, true},
		{"paren style", "Pick:\n1) Red\n2) Green\n",
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 2, Label: "Green"}},
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 2, Label: "Green"}}, true},
		{"whitespace normalised", "Pick:\n  1.   Red   color\n  2. Green\n",
			[]domain.Choice{{Number: 1, Label: "Red color"}, {Number: 2, Label: "Green"}},
			[]domain.Choice{{Number: 1, Label: "Red color"}, {Number: 2, Label: "Green"}}, true},
		{"cursor row", "Pick:\n❯ 1. Red\n  2. Green\n",
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 2, Label: "Green"}},
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 2, Label: "Green"}}, true},
		{"invented label", question,
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 2, Label: "Yellow"}, {Number: 3, Label: "Blue"}}, nil, false},
		{"gap in numbering", question,
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 3, Label: "Blue"}}, nil, false},
		{"numbers not from one", question,
			[]domain.Choice{{Number: 2, Label: "Green"}, {Number: 3, Label: "Blue"}}, nil, false},
		{"one option", "Pick:\n1. Only\n",
			[]domain.Choice{{Number: 1, Label: "Only"}}, nil, false},
		{"six options", "1. A\n2. B\n3. C\n4. D\n5. E\n6. F\n",
			[]domain.Choice{{Number: 1, Label: "A"}, {Number: 2, Label: "B"}, {Number: 3, Label: "C"}, {Number: 4, Label: "D"}, {Number: 5, Label: "E"}, {Number: 6, Label: "F"}}, nil, false},
		{"empty label", question,
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 2, Label: "  "}}, nil, false},
		{"no options", question, nil, nil, false},
		{"list in the transcript above the question",
			"Plan:\n1. Add the import\n2. Run the tests\n\nQuestion?\n1. Red\n2. Green\n",
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 2, Label: "Green"}},
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 2, Label: "Green"}}, true},
		{"transcript list closed by prose does not ground",
			"Plan:\n1. Add the import\n2. Run the tests\n\nContinue? (y/n)\n",
			[]domain.Choice{{Number: 1, Label: "Add the import"}, {Number: 2, Label: "Run the tests"}}, nil, false},
		{"trailing footer allowed",
			question + "\nEnter to select · ↑/↓ to navigate · Esc to cancel",
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 2, Label: "Green"}, {Number: 3, Label: "Blue"}},
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 2, Label: "Green"}, {Number: 3, Label: "Blue"}}, true},
		{"question far above the bottom",
			"1. Red\n2. Green\n" + strings.Repeat("log line\n", 10),
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 2, Label: "Green"}}, nil, false},
		{"measured dialog", measuredDialog,
			[]domain.Choice{{Number: 1, Label: "Красный"}, {Number: 2, Label: "Зелёный"}, {Number: 3, Label: "Синий"}},
			[]domain.Choice{{Number: 1, Label: "Красный"}, {Number: 2, Label: "Зелёный"}, {Number: 3, Label: "Синий"}}, true},
		{"service rows proposed by LLM", measuredDialog,
			[]domain.Choice{{Number: 1, Label: "Красный"}, {Number: 2, Label: "Зелёный"}, {Number: 3, Label: "Синий"}, {Number: 4, Label: "Type something."}, {Number: 5, Label: "Chat about this"}},
			[]domain.Choice{{Number: 1, Label: "Красный"}, {Number: 2, Label: "Зелёный"}, {Number: 3, Label: "Синий"}}, true},
		{"measured dialog with an invented label", measuredDialog,
			[]domain.Choice{{Number: 1, Label: "Красный"}, {Number: 2, Label: "Жёлтый"}, {Number: 3, Label: "Синий"}}, nil, false},
		// The Pi overlay sits above the transcript, never at the bottom:
		// its layouts ground through the ParseDialog overlay parser,
		// never through LLM options.
		{"measured pi ask_user has no bottom dialog", measuredPiDialog,
			[]domain.Choice{{Number: 1, Label: "Lo screen, come oggi"}, {Number: 2, Label: "Screen con nota di ripiego"}, {Number: 3, Label: "Niente, salta il post"}, {Number: 4, Label: "Testo segnaposto"}},
			nil, false},
		{"crlf screen", "Pick:\r\n1. Red\r\n2. Green\r\n",
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 2, Label: "Green"}},
			[]domain.Choice{{Number: 1, Label: "Red"}, {Number: 2, Label: "Green"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := domain.GroundChoices(tc.screen, tc.choices)
			if ok != tc.ok || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("GroundChoices() = %v, %v, want %v, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestPressConfirm(t *testing.T) {
	for _, tc := range []struct {
		kind        string
		confirm, ok bool
	}{
		{"claude", false, true},
		{"pi", true, true},
		{"codex", false, false},
		{"", false, false},
		{"gemini", false, false},
	} {
		t.Run("kind="+tc.kind, func(t *testing.T) {
			if confirm, ok := domain.PressConfirm(tc.kind); confirm != tc.confirm || ok != tc.ok {
				t.Fatalf("PressConfirm(%q) = %v, %v, want %v, %v", tc.kind, confirm, ok, tc.confirm, tc.ok)
			}
		})
	}
}
