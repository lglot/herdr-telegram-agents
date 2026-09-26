package transcript

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

// Bound both what the cloud receives and the work needed to read the tail.
const (
	recentMaxRunes    = 8000
	recentMaxMessages = 24
)

// recentIn walks the current session backwards, keeps only human and
// assistant text, then returns it in chronological order. Pi's parentId
// chain excludes messages on abandoned branches.
func recentIn(ctx context.Context, path, kind string, budget int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("%w: open transcript: %v", domain.ErrNoReply, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("%w: stat transcript: %v", domain.ErrNoReply, err)
	}
	var newestFirst []string
	runes := 0
	truncated := false
	piFirst, piParent := true, ""
	visit := func(line []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			return nil
		}
		role, text := "", ""
		if kind == kindPi {
			var rec piRecord
			if json.Unmarshal(line, &rec) != nil || rec.ID == "" {
				return nil
			}
			if piFirst {
				piFirst = false
			} else if rec.ID != piParent {
				return nil
			}
			piParent = rec.ParentID
			if rec.Type != "message" {
				return nil
			}
			role = rec.Message.Role
			text = recapText(rec.Message.Content)
		} else {
			var rec record
			if json.Unmarshal(line, &rec) != nil || rec.IsSidechain {
				return nil
			}
			if rec.Type == "user" && isPrompt(rec) {
				role = "user"
			} else if rec.Type == "assistant" {
				role = "assistant"
			}
			text = recapText(rec.Message.Content)
		}
		if (role != "user" && role != "assistant") || text == "" {
			return nil
		}
		chars := []rune(text)
		if len(chars) > recentMaxRunes {
			chars = chars[len(chars)-recentMaxRunes:]
			truncated = true
		}
		if len(newestFirst) >= recentMaxMessages || runes+len(chars) > recentMaxRunes {
			truncated = true
			return errStop
		}
		newestFirst = append(newestFirst, role+": "+string(chars))
		runes += len(chars)
		return nil
	}
	read, err := walkBack(f, info.Size(), budget, visit)
	if err != nil && !errors.Is(err, errStop) {
		return "", err
	}
	if len(newestFirst) == 0 {
		return "", fmt.Errorf("%w: session has no human or assistant text", domain.ErrNoReply)
	}
	if read < info.Size() {
		truncated = true
	}
	var out strings.Builder
	if truncated {
		out.WriteString("[Older session messages omitted]\n\n")
	}
	for i := len(newestFirst) - 1; i >= 0; i-- {
		out.WriteString(newestFirst[i])
		out.WriteString("\n\n")
	}
	return strings.TrimSpace(out.String()), nil
}

// recapText keeps every text part while leaving thinking and tool parts out.
func recapText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '[' {
		text, _ := lastText(raw)
		return text
	}
	var parts []contentPart
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var texts []string
	for _, part := range parts {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			texts = append(texts, strings.TrimSpace(part.Text))
		}
	}
	return strings.Join(texts, "\n")
}
