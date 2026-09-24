package transcript

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

// claudeSessionID is the shape of a Claude Code session id Herdr reports;
// anything else (a path, a glob character) is not looked up.
var claudeSessionID = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// sessionTranscript returns the transcript Herdr names for the agent and
// its modification time, or "" when Herdr names none or it is not on disk
// (Pi writes its file with the first reply), so the caller guesses from
// the working directory as before. Pi reports its session file. Claude
// Code reports its session id, whose <id>.jsonl is looked up in every
// project of every ~/.claude* directory, since a CLAUDE_CONFIG_DIR such as
// ~/.claude-work moves the transcripts and the pane's cwd may not be the
// directory Claude Code runs in.
func sessionTranscript(home string, agent domain.Agent) (string, time.Time) {
	var paths []string
	switch {
	case agent.Kind == kindPi && agent.SessionKind == "path" && filepath.IsAbs(agent.SessionValue) && strings.HasSuffix(agent.SessionValue, transcriptSuffix):
		paths = []string{agent.SessionValue}
	case agent.Kind == kindClaude && agent.SessionKind == "id" && claudeSessionID.MatchString(agent.SessionValue):
		paths, _ = filepath.Glob(filepath.Join(home, ".claude*", "projects", "*", agent.SessionValue+transcriptSuffix))
	}
	var best string
	var modTime time.Time
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if best == "" || info.ModTime().After(modTime) {
			best, modTime = p, info.ModTime()
		}
	}
	return best, modTime
}
