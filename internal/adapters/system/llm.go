package system

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

const (
	// llmTimeout bounds one screen rewrite below bridgeCallTimeout (15s).
	llmTimeout    = 5 * time.Second
	llmConfigFile = "llm.json"
)

// LLM implements domain.Renderer through an OpenAI-compatible chat API:
// temperature 0 and one JSON object per screen. It is nil when unconfigured.
type LLM struct {
	url, model, key string
	client          *http.Client
}

var _ domain.Renderer = (*LLM)(nil)

// NewLLM loads the optional private config once at daemon start. Without a
// complete config, outbound posts screens as before.
func NewLLM(configDir string, log *slog.Logger) *LLM {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	path := filepath.Join(configDir, llmConfigFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		log.Warn("llm config unreadable", slog.String("err", err.Error()))
		return nil
	}
	var cfg struct {
		URL   string `json:"url"`
		Model string `json:"model"`
		Key   string `json:"key"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Warn("llm config invalid", slog.String("err", err.Error()))
		return nil
	}
	url, model, key := strings.TrimSpace(cfg.URL), strings.TrimSpace(cfg.Model), strings.TrimSpace(cfg.Key)
	if url == "" || model == "" || key == "" {
		log.Info("screens are not rewritten", slog.Bool("llm_url", url != ""), slog.Bool("llm_model", model != ""), slog.Bool("llm_key", key != ""))
		return nil
	}
	log.Info("screens are rewritten", slog.String("model", model))
	return &LLM{url: strings.TrimRight(url, "/"), model: model, key: key,
		client: &http.Client{Timeout: llmTimeout}}
}

// llmSystem tells the model what Telegram accepts: the subset the
// Markdown renderer in internal/adapters/telegram understands (bold,
// italic, strike, inline code and fences, links, bullets, quotes, rules,
// pipe tables). Anything else arrives as escaped text, so restraint in
// the prompt is what keeps posts readable.
const llmSystem = `Format the entire coding-agent terminal screen as Telegram Markdown. JSON only: text string and options array with number and label. Keep all substantive content in original order and language, including the earlier transcript. Remove TUI chrome, rules and key hints. Keep numbered answer labels and descriptions. For options, copy only real answer rows of the final active question; exclude Type something and Chat about this. If unsure, use []. Never summarize.`

// Render implements domain.Renderer: the rewritten text with the options
// read on the screen. An empty text is a usable miss (the screen is
// posted as usual); transport and protocol failures are errors.
func (l *LLM) Render(ctx context.Context, kind, screen string, blocked bool) (domain.Rendered, error) {
	prompt := fmt.Sprintf("agent kind: %s. blocked: %v.\nscreen:\n%s", kind, blocked, screen)
	body, err := json.Marshal(map[string]any{
		"model":       l.model,
		"temperature": 0,
		"provider":    map[string]any{"require_parameters": true},
		"messages": []map[string]string{
			{"role": "system", "content": llmSystem},
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "screen_render",
				"strict": true,
				"schema": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"text", "options"},
					"properties": map[string]any{
						"text": map[string]any{"type": "string"},
						"options": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type":                 "object",
								"additionalProperties": false,
								"required":             []string{"number", "label"},
								"properties": map[string]any{
									"number": map[string]any{"type": "integer"},
									"label":  map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return domain.Rendered{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.url+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return domain.Rendered{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if l.key != "" {
		req.Header.Set("Authorization", "Bearer "+l.key)
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return domain.Rendered{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return domain.Rendered{}, fmt.Errorf("llm: status %d", resp.StatusCode)
	}
	var env struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return domain.Rendered{}, fmt.Errorf("llm: bad envelope: %w", err)
	}
	if len(env.Choices) == 0 {
		return domain.Rendered{}, fmt.Errorf("llm: no choices")
	}
	var payload struct {
		Text    string `json:"text"`
		Options []struct {
			Number int    `json:"number"`
			Label  string `json:"label"`
		} `json:"options"`
	}
	if err := json.Unmarshal([]byte(env.Choices[0].Message.Content), &payload); err != nil {
		return domain.Rendered{}, fmt.Errorf("llm: bad json: %w", err)
	}
	out := domain.Rendered{Text: strings.TrimSpace(payload.Text)}
	for _, o := range payload.Options {
		if o.Number >= 1 && strings.TrimSpace(o.Label) != "" {
			out.Choices = append(out.Choices, domain.Choice{Number: o.Number, Label: strings.TrimSpace(o.Label)})
		}
	}
	return out, nil
}
