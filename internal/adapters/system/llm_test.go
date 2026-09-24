package system

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

// llmTestServer answers every completion with content and records the
// request for header assertions.
func llmTestServer(t *testing.T, status int, content string, seen *http.Request) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			*seen = *r
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, content)
	}))
}

func llmEnvelope(content string) string {
	return fmt.Sprintf(`{"choices": [{"message": {"content": %q}}]}`, content)
}

func newTestLLM(url string) *LLM {
	l := &LLM{url: url, model: "test-model", client: &http.Client{Timeout: 2 * time.Second}}
	return l
}

func TestLLMRenderOK(t *testing.T) {
	var seen http.Request
	content := `{"text": "# Question\n\n**Do it?**", "options": [{"number": 1, "label": "Yes"}, {"number": 2, "label": "No"}]}`
	srv := llmTestServer(t, http.StatusOK, llmEnvelope(content), &seen)
	defer srv.Close()
	l := newTestLLM(srv.URL)
	got, err := l.Render(context.Background(), "claude", "Do it?\n1. Yes\n2. No", true)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Rendered{Text: "# Question\n\n**Do it?**",
		Choices: []domain.Choice{{Number: 1, Label: "Yes"}, {Number: 2, Label: "No"}}}
	if got.Text != want.Text || len(got.Choices) != 2 || got.Choices[0] != want.Choices[0] || got.Choices[1] != want.Choices[1] {
		t.Fatalf("Render() = %+v, want %+v", got, want)
	}
	if seen.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("content-type = %q", seen.Header.Get("Content-Type"))
	}
	if got := seen.URL.Path; got != "/v1/chat/completions" {
		t.Fatalf("path = %q", got)
	}
}

func TestLLMStructuredRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization = %q", got)
		}
		var body struct {
			Provider struct {
				RequireParameters bool `json:"require_parameters"`
			} `json:"provider"`
			ResponseFormat struct {
				JSONSchema struct {
					Strict bool `json:"strict"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if !body.Provider.RequireParameters || !body.ResponseFormat.JSONSchema.Strict {
			t.Errorf("structured output not required: %+v", body)
		}
		fmt.Fprint(w, llmEnvelope(`{"text":"ok","options":[]}`))
	}))
	defer srv.Close()
	l := newTestLLM(srv.URL)
	l.key = "test-key"
	if _, err := l.Render(context.Background(), "claude", "screen", true); err != nil {
		t.Fatal(err)
	}
}

func TestLLMRenderSkipsBadOptions(t *testing.T) {
	content := `{"text": "hi", "options": [{"number": 0, "label": "Zero"}, {"number": 3, "label": "  "}, {"number": 1, "label": " One "}]}`
	srv := llmTestServer(t, http.StatusOK, llmEnvelope(content), nil)
	defer srv.Close()
	got, err := newTestLLM(srv.URL).Render(context.Background(), "pi", "screen", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "hi" || len(got.Choices) != 1 || got.Choices[0] != (domain.Choice{Number: 1, Label: "One"}) {
		t.Fatalf("Render() = %+v", got)
	}
}

func TestLLMRenderEmptyText(t *testing.T) {
	srv := llmTestServer(t, http.StatusOK, llmEnvelope(`{"text": "  ", "options": []}`), nil)
	defer srv.Close()
	got, err := newTestLLM(srv.URL).Render(context.Background(), "claude", "screen", true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "" {
		t.Fatalf("Render() = %+v, want empty text", got)
	}
}

func TestLLMRenderFailures(t *testing.T) {
	broken := llmTestServer(t, http.StatusOK, `{"choices": [`, nil)
	defer broken.Close()
	err500 := llmTestServer(t, http.StatusInternalServerError, "boom", nil)
	defer err500.Close()
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer slow.Close()
	slowLLM := &LLM{url: slow.URL, model: "m", client: &http.Client{Timeout: 50 * time.Millisecond}}

	for _, tc := range []struct {
		name string
		llm  *LLM
		url  string
	}{
		{"broken envelope", newTestLLM(broken.URL), ""},
		{"status 500", newTestLLM(err500.URL), ""},
		{"bad option json", func() *LLM {
			s := llmTestServer(t, http.StatusOK, llmEnvelope(`not json`), nil)
			t.Cleanup(s.Close)
			return newTestLLM(s.URL)
		}(), ""},
		{"timeout", slowLLM, ""},
		{"connection refused", newTestLLM("http://127.0.0.1:1"), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.llm.Render(context.Background(), "claude", "screen", true); err == nil {
				t.Fatal("Render() = nil error")
			}
		})
	}
}

func TestNewLLMNeedsConfig(t *testing.T) {
	dir := t.TempDir()
	if NewLLM(dir, nil) != nil {
		t.Fatal("NewLLM() without config, want nil")
	}
	path := filepath.Join(dir, llmConfigFile)
	for _, tc := range []struct {
		content string
		want    bool
	}{
		{`{`, false},
		{`{"url":"https://openrouter.ai/api","model":"mistralai/ministral-14b-2512"}`, false},
		{`{"url":"https://openrouter.ai/api","model":"mistralai/ministral-14b-2512","key":"test-key"}`, true},
	} {
		if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
			t.Fatal(err)
		}
		got := NewLLM(dir, nil)
		if (got != nil) != tc.want {
			t.Fatalf("NewLLM() configured = %v for %q", got != nil, tc.content)
		}
	}
}
