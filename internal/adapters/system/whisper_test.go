package system

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeBin writes an executable shell script and returns its path.
func fakeBin(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWhisperTranscribe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fakes")
	}
	dir := t.TempDir()
	argsLog := filepath.Join(dir, "args")
	// ffmpeg writes its last argument, the wav; whisper-cli logs its argv
	// and prints two segments the way -nt -np does.
	ffmpeg := fakeBin(t, dir, "ffmpeg", `for a; do last=$a; done; echo wav > "$last"`)
	cli := fakeBin(t, dir, "whisper-cli", `echo "$@" >> "`+argsLog+`"; printf ' Ciao, controlla la release\n e dimmi se i test sono verdi.\n'`)
	w := &Whisper{cli: cli, ffmpeg: ffmpeg, model: "/models/ggml-x.bin", vad: "/models/ggml-silero-v6.2.0.bin", language: "it", timeout: whisperTimeout}

	text, err := w.Transcribe(context.Background(), filepath.Join(dir, "voice.ogg"))
	if err != nil {
		t.Fatal(err)
	}
	if text != "Ciao, controlla la release e dimmi se i test sono verdi." {
		t.Errorf("text = %q", text)
	}
	args, _ := os.ReadFile(argsLog)
	if got := string(args); !strings.Contains(got, "-m /models/ggml-x.bin") || !strings.Contains(got, "-l it") || !strings.Contains(got, "-nt -np") ||
		!strings.Contains(got, "--vad -vm /models/ggml-silero-v6.2.0.bin") {
		t.Errorf("whisper-cli args = %q", got)
	}

	failing := &Whisper{cli: fakeBin(t, dir, "broken", `echo "model not found" >&2; exit 1`), ffmpeg: ffmpeg, model: "m", language: "auto", timeout: whisperTimeout}
	if _, err := failing.Transcribe(context.Background(), filepath.Join(dir, "voice.ogg")); err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Errorf("failing err = %v", err)
	}
}

func TestWhisperModelPicksTheLargest(t *testing.T) {
	dir := t.TempDir()
	if got := largestModel(dir); got != "" {
		t.Fatalf("empty dir model = %q", got)
	}
	// The Silero VAD model is never the speech model, even when alone.
	if err := os.WriteFile(filepath.Join(dir, "ggml-silero-v6.2.0.bin"), make([]byte, 50), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := largestModel(dir); got != "" {
		t.Fatalf("vad-only dir model = %q", got)
	}
	if got := vadModel(dir); got != filepath.Join(dir, "ggml-silero-v6.2.0.bin") {
		t.Errorf("vad = %q", got)
	}
	for name, size := range map[string]int{"ggml-base.bin": 10, "ggml-large-v3-turbo-q5_0.bin": 30, "notes.txt": 99} {
		if err := os.WriteFile(filepath.Join(dir, name), make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := largestModel(dir); got != filepath.Join(dir, "ggml-large-v3-turbo-q5_0.bin") {
		t.Errorf("model = %q", got)
	}
}
