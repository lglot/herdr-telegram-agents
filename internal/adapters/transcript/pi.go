package transcript

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

// kindPi is the Herdr agent kind of the Pi coding agent.
const kindPi = "pi"

// piSessionsDir is Pi's session root, relative to the home. Pi keeps one
// directory per working directory and one .jsonl per session; subagent
// runs live in subdirectories, which newestTranscript skips.
var piSessionsDir = []string{".pi", "agent", "sessions"}

// piSlugReplacer turns every path separator and the drive colon into "-".
var piSlugReplacer = strings.NewReplacer("/", "-", `\`, "-", ":", "-")

// piSlug is the directory name Pi derives from a working directory: the
// first separator is dropped, every "/", "\" and ":" becomes "-", and the
// result is wrapped in "--", so "/Users/op/demo" is "--Users-op-demo--".
// Mirrors getDefaultSessionDirPath in Pi's session-manager (Pi 0.87.0).
func piSlug(cwd string) string {
	if strings.HasPrefix(cwd, "/") || strings.HasPrefix(cwd, `\`) {
		cwd = cwd[1:]
	}
	return "--" + piSlugReplacer.Replace(cwd) + "--"
}

// piRecord is the slice of a Pi session line the reader needs. Pi writes
// a session header and many entry types (model_change, compaction,
// custom, custom_message, ...); only message entries are read, and among
// them only the user and assistant roles (tool results are their own
// role, subagent notifications are custom_message entries).
type piRecord struct {
	Type    string `json:"type"`
	Message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// lastPiReplyIn walks a Pi session backwards and returns the first
// non-empty text of an assistant message met before the operator's last
// prompt. Pi branches are appended at the end of the file, so the newest
// lines are the current branch. No turn stats are gathered.
func lastPiReplyIn(path string, budget int64) (string, turnStats, scanStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", turnStats{}, scanStats{}, fmt.Errorf("%w: open transcript: %v", domain.ErrNoReply, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", turnStats{}, scanStats{}, fmt.Errorf("%w: stat transcript: %v", domain.ErrNoReply, err)
	}
	return lastPiReplyFrom(f, info.Size(), budget)
}

// lastPiReplyFrom is lastPiReplyIn over an open session of size bytes.
func lastPiReplyFrom(f io.ReaderAt, size, budget int64) (string, turnStats, scanStats, error) {
	var stats scanStats
	var turn turnStats
	var found string
	var haveText bool
	visit := func(line []byte) error {
		stats.lines++
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			return nil
		}
		var rec piRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			stats.skipped++
			return nil
		}
		if rec.Type != "message" {
			return nil
		}
		switch rec.Message.Role {
		case "user":
			if raw := bytes.TrimSpace(rec.Message.Content); len(raw) == 0 || bytes.Equal(raw, []byte(`""`)) {
				return nil
			}
			turn.complete = true
			if !haveText {
				return fmt.Errorf("%w: no reply text after the last prompt", domain.ErrNoReply)
			}
			return errStop
		case "assistant":
			if haveText {
				return nil
			}
			if text, ok := lastText(rec.Message.Content); ok {
				found, haveText = text, true
			}
		}
		return nil
	}
	bytesRead, err := walkBack(f, size, budget, visit)
	stats.bytes = bytesRead
	switch {
	case errors.Is(err, errStop):
		return found, turn, stats, nil
	case haveText:
		stats.readErr = err
		return found, turn, stats, nil
	case err != nil:
		return "", turnStats{}, stats, err
	case bytesRead >= budget && size > budget:
		return "", turnStats{}, stats, fmt.Errorf("%w: no prompt within the last %d bytes", domain.ErrNoReply, budget)
	}
	return "", turnStats{}, stats, fmt.Errorf("%w: transcript has no reply", domain.ErrNoReply)
}
