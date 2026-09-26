package transcript

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

func TestRecentClaudeExcludesTools(t *testing.T) {
	got, err := recentIn(context.Background(), filepath.Join("testdata", "simple.jsonl"), kindClaude, defaultMaxScan)
	if err != nil {
		t.Fatal(err)
	}
	want := "user: do it\n\nassistant: Working on it.\n\nassistant: Done: **all good**."
	if got != want || strings.Contains(got, "tool_result") || strings.Contains(got, "out") {
		t.Fatalf("Recent = %q, want %q", got, want)
	}
}

func TestRecentPiFollowsCurrentBranch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	lines := []string{
		`{"type":"message","id":"u1","parentId":null,"message":{"role":"user","content":"start"}}`,
		`{"type":"message","id":"a1","parentId":"u1","message":{"role":"assistant","content":"base"}}`,
		`{"type":"message","id":"u2a","parentId":"a1","message":{"role":"user","content":"abandoned"}}`,
		`{"type":"message","id":"a2a","parentId":"u2a","message":{"role":"assistant","content":"old answer"}}`,
		`{"type":"model_change","id":"m2","parentId":"a1"}`,
		`{"type":"message","id":"u2b","parentId":"m2","message":{"role":"user","content":"current"}}`,
		`{"type":"message","id":"a2b","parentId":"u2b","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hidden"},{"type":"text","text":"new answer"},{"type":"text","text":"with detail"}]}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := recentIn(context.Background(), path, kindPi, defaultMaxScan)
	if err != nil {
		t.Fatal(err)
	}
	want := "user: start\n\nassistant: base\n\nuser: current\n\nassistant: new answer\nwith detail"
	if got != want {
		t.Fatalf("Recent = %q, want %q", got, want)
	}
}

func TestRecentRequiresIdentifiedSession(t *testing.T) {
	home := t.TempDir()
	r := newReader(func() (string, error) { return home, nil }, time.Now, nil)
	if _, err := r.Recent(context.Background(), domain.Agent{Kind: kindPi, Cwd: "/shared"}); err == nil || !strings.Contains(err.Error(), "did not identify") {
		t.Fatalf("Recent without session = %v", err)
	}
	path := filepath.Join(home, "pi.jsonl")
	data, err := os.ReadFile(filepath.Join("testdata", "pi_simple.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := r.Recent(context.Background(), domain.Agent{Kind: kindPi, SessionKind: "path", SessionValue: path})
	if err != nil || !strings.Contains(got, "user: first") || !strings.Contains(got, "assistant: Done: **all good**.") {
		t.Fatalf("Recent = %q, %v", got, err)
	}
}

func TestRecentMarksTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	line := `{"type":"user","message":{"content":"` + strings.Repeat("x", recentMaxRunes+1) + `"}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := recentIn(context.Background(), path, kindClaude, defaultMaxScan)
	if err != nil || !strings.HasPrefix(got, "[Older session messages omitted]") {
		t.Fatalf("Recent = %q, %v", got, err)
	}
}
