package system

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/permgps/herdr-telegram-agents/internal/domain"
)

const (
	// whisperTimeout bounds the conversion plus the transcription of one
	// voice note; a minute of speech takes a few seconds with Metal.
	whisperTimeout = 3 * time.Minute
	// whisperModelEnv names a ggml model file; without it the largest
	// ggml-*.bin in whisperModelDir under the home is used.
	whisperModelEnv = "HERDR_TG_WHISPER_MODEL"
	// whisperLanguageEnv is the spoken language ("it", "en"); the default
	// lets whisper detect it.
	whisperLanguageEnv = "HERDR_TG_WHISPER_LANGUAGE"
)

// whisperModelDir is where the default model is looked up, under the home.
var whisperModelDir = filepath.Join(".local", "share", "whisper.cpp")

// binDirs are tried after PATH: a daemon started by Herdr may not inherit
// the login shell's PATH with Homebrew in it.
var binDirs = []string{"/opt/homebrew/bin", "/usr/local/bin"}

// vadPrefix starts the file name of whisper.cpp's Silero VAD models, which
// sit next to the speech models but are not one.
const vadPrefix = "ggml-silero-"

// Whisper implements domain.Transcriber with whisper.cpp: ffmpeg turns the
// voice note (Ogg/Opus from Telegram) into 16 kHz mono wav, whisper-cli
// transcribes it on this machine, nothing leaves it. With a VAD model the
// silent parts are skipped: on silence alone whisper otherwise invents a
// word ("you", measured 2026-09-24) that would reach the agent as a prompt.
type Whisper struct {
	cli, ffmpeg, model, vad, language string
	timeout                           time.Duration
	log                               *slog.Logger
}

var _ domain.Transcriber = (*Whisper)(nil)

// NewWhisper returns a transcriber when whisper-cli, ffmpeg and a model
// are all found, else nil, logging what is missing once: the inbox then
// prompts with the path as before.
func NewWhisper(log *slog.Logger) *Whisper {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	cli, ffmpeg := findBin("whisper-cli"), findBin("ffmpeg")
	model, vad := os.Getenv(whisperModelEnv), ""
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, whisperModelDir)
		if model == "" {
			model = largestModel(dir)
		}
		vad = vadModel(dir)
	}
	if cli == "" || ffmpeg == "" || model == "" {
		log.Info("voice notes are not transcribed", slog.Bool("whisper_cli", cli != ""), slog.Bool("ffmpeg", ffmpeg != ""), slog.String("model", model))
		return nil
	}
	language := os.Getenv(whisperLanguageEnv)
	if language == "" {
		language = "auto"
	}
	log.Info("voice notes are transcribed", slog.String("model", filepath.Base(model)), slog.String("vad", filepath.Base(vad)), slog.String("language", language))
	return &Whisper{cli: cli, ffmpeg: ffmpeg, model: model, vad: vad, language: language, timeout: whisperTimeout, log: log}
}

// Transcribe implements domain.Transcriber: the words, one space between
// segments, or an error carrying the failing tool's stderr.
func (w *Whisper) Transcribe(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "herdr-tg-voice-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	wav := filepath.Join(dir, "voice.wav")
	if _, err := run(ctx, w.ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-i", path, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wav); err != nil {
		return "", fmt.Errorf("ffmpeg: %w", err)
	}
	args := []string{"-m", w.model, "-f", wav, "-l", w.language, "-nt", "-np"}
	if w.vad != "" {
		args = append(args, "--vad", "-vm", w.vad)
	}
	out, err := run(ctx, w.cli, args...)
	if err != nil {
		return "", fmt.Errorf("whisper-cli: %w", err)
	}
	return strings.Join(strings.Fields(out), " "), nil
}

// run executes bin with args and returns stdout; a failure carries the
// trimmed stderr.
func run(ctx context.Context, bin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return stdout.String(), nil
}

// findBin looks name up on PATH, then in binDirs; "" when absent.
func findBin(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, d := range binDirs {
		p := filepath.Join(d, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

// largestModel returns the biggest ggml-*.bin speech model in dir (the
// most accurate one downloaded), "" when there is none.
func largestModel(dir string) string {
	paths, _ := filepath.Glob(filepath.Join(dir, "ggml-*.bin"))
	best, size := "", int64(-1)
	for _, p := range paths {
		if strings.HasPrefix(filepath.Base(p), vadPrefix) {
			continue
		}
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() && info.Size() > size {
			best, size = p, info.Size()
		}
	}
	return best
}

// vadModel returns the newest-named Silero VAD model in dir, "" without one.
func vadModel(dir string) string {
	paths, _ := filepath.Glob(filepath.Join(dir, vadPrefix+"*.bin"))
	if len(paths) == 0 {
		return ""
	}
	return paths[len(paths)-1] // Glob sorts, so v6 comes after v5
}
