package transcript

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

// copyFixture writes a testdata file to path with the given mtime.
func copyFixture(t *testing.T, fixture, path string, mod time.Time) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

// The session Herdr names wins over the newest file of the cwd directory,
// which may belong to another pane in the same directory; a Claude Code
// session is found under any ~/.claude* profile, whatever the cwd.
func TestLastReplyBySession(t *testing.T) {
	home := t.TempDir()
	cwd := "/Users/op/Projects/demo"
	older := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	newer := older.Add(time.Minute)
	r := newReader(func() (string, error) { return home, nil }, func() time.Time { return newer }, slog.New(slog.DiscardHandler))

	claudeFile := filepath.Join(home, ".claude-work", "projects", "-Users-op-Projects-demo-sub", "0158d641-2686-4058-8235-1ae7f1033d36.jsonl")
	copyFixture(t, "simple.jsonl", claudeFile, older)
	copyFixture(t, "noise.jsonl", filepath.Join(home, ".claude", "projects", projectSlug(cwd), "other.jsonl"), newer)
	piFile := filepath.Join(home, ".pi", "agent", "sessions", piSlug(cwd), "2026-09-24T10-00-00-000Z_mine.jsonl")
	copyFixture(t, "pi_simple.jsonl", piFile, older)
	copyFixture(t, "pi_noise.jsonl", filepath.Join(home, ".pi", "agent", "sessions", piSlug(cwd), "2026-09-24T10-01-00-000Z_other.jsonl"), newer)

	tests := []struct {
		name       string
		agent      domain.Agent
		wantText   string
		wantSource string
	}{
		{"claude by id, other profile and directory", domain.Agent{Kind: "claude", Cwd: cwd, SessionKind: "id", SessionValue: "0158d641-2686-4058-8235-1ae7f1033d36"}, "Done: **all good**.", claudeFile},
		{"claude by id without a cwd", domain.Agent{Kind: "claude", SessionKind: "id", SessionValue: "0158d641-2686-4058-8235-1ae7f1033d36"}, "Done: **all good**.", claudeFile},
		{"claude unknown id falls back to the cwd", domain.Agent{Kind: "claude", Cwd: cwd, SessionKind: "id", SessionValue: "ffffffff-0000-0000-0000-000000000000"}, "Noisy final", ""},
		{"claude id with a path in it is ignored", domain.Agent{Kind: "claude", Cwd: cwd, SessionKind: "id", SessionValue: "../../x"}, "Noisy final", ""},
		{"pi by path", domain.Agent{Kind: "pi", Cwd: cwd, SessionKind: "path", SessionValue: piFile}, "Done: **all good**.", piFile},
		{"pi path not written yet falls back to the cwd", domain.Agent{Kind: "pi", Cwd: cwd, SessionKind: "path", SessionValue: piFile + ".missing"}, "Noisy final", ""},
		{"pi relative path is ignored", domain.Agent{Kind: "pi", Cwd: cwd, SessionKind: "path", SessionValue: "s.jsonl"}, "Noisy final", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply, err := r.LastReply(context.Background(), tt.agent)
			if err != nil {
				t.Fatal(err)
			}
			if reply.Text != tt.wantText || (tt.wantSource != "" && reply.Source != tt.wantSource) {
				t.Errorf("reply = %q from %s", reply.Text, reply.Source)
			}
		})
	}
}
