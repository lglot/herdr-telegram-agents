package transcript

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

func TestPiSlug(t *testing.T) {
	tests := map[string]string{
		"/Users/op/Projects/demo": "--Users-op-Projects-demo--",
		"/Users/op/my_dir.v2":     "--Users-op-my_dir.v2--",
		`C:\Users\op\demo`:        "--C--Users-op-demo--",
		"/":                       "----",
	}
	for cwd, want := range tests {
		if got := piSlug(cwd); got != want {
			t.Errorf("piSlug(%q) = %q, want %q", cwd, got, want)
		}
	}
}

func TestLastPiReplyInFixtures(t *testing.T) {
	tests := []struct {
		file, want string
		skipped    int
		wantErr    string
	}{
		{file: "pi_simple.jsonl", want: "Done: **all good**."},
		{file: "pi_tool_last.jsonl", want: "Working on it."}, // the turn ended with a tool: the narration before it is the last text
		{file: "pi_noise.jsonl", want: "Noisy final", skipped: 1},
		{file: "pi_empty.jsonl", wantErr: "no reply text after the last prompt"},
		{file: "empty.jsonl", wantErr: "transcript has no reply"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			text, turn, stats, err := lastPiReplyIn(filepath.Join("testdata", tt.file), defaultMaxScan)
			if tt.wantErr != "" {
				if !errors.Is(err, domain.ErrNoReply) || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want ErrNoReply with %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if text != tt.want {
				t.Errorf("text = %q, want %q", text, tt.want)
			}
			if stats.skipped != tt.skipped || stats.lines == 0 || stats.bytes == 0 {
				t.Errorf("stats = %+v", stats)
			}
			if !turn.complete {
				t.Errorf("turn = %+v", turn)
			}
		})
	}
}

func TestLastReplyPi(t *testing.T) {
	home := t.TempDir()
	cwd := "/Users/op/Projects/demo"
	dir := filepath.Join(home, ".pi", "agent", "sessions", piSlug(cwd))
	// A subagent run lives in a subdirectory and must not be picked.
	if err := os.MkdirAll(filepath.Join(dir, "2026-09-18T00-24-08-750Z_abc", "run-0"), 0o700); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join("testdata", "pi_simple.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "2026-09-18T00-24-08-750Z_abc.jsonl")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	mod := time.Date(2026, 9, 18, 0, 25, 5, 0, time.UTC)
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
	now := mod.Add(2 * time.Second)
	r := newReader(func() (string, error) { return home, nil }, func() time.Time { return now }, slog.New(slog.DiscardHandler))
	agent := domain.Agent{Key: domain.Key{PaneID: "p1", TerminalID: "t1"}, Kind: "pi", Cwd: cwd}

	reply, err := r.LastReply(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Text != "Done: **all good**." || reply.Source != path || reply.Age != 2*time.Second || !reply.Written.Equal(mod) {
		t.Errorf("reply = %+v", reply)
	}
	if reply.Meta.Line() != "" {
		t.Errorf("meta line = %q, want none for Pi", reply.Meta.Line())
	}

	if _, err := r.LastReply(context.Background(), domain.Agent{Kind: "pi", Cwd: "/elsewhere"}); !errors.Is(err, domain.ErrNoReply) || !strings.Contains(err.Error(), "no transcript directory") {
		t.Errorf("missing dir err = %v", err)
	}
	if _, err := r.LastReply(context.Background(), domain.Agent{Kind: "pi"}); !errors.Is(err, domain.ErrNoReply) || !strings.Contains(err.Error(), "no working directory") {
		t.Errorf("no cwd err = %v", err)
	}
}
