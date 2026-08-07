package process

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dunstorm/pm2-go/internal/logstore"
)

func TestProcessStreamLogsWritesPlainAndCombinedLogs(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "api-out.log")
	combinedPath := filepath.Join(dir, "api-combined.jsonl")

	outFile, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open out log: %v", err)
	}
	combinedFile, err := os.OpenFile(combinedPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open combined log: %v", err)
	}

	sink := newCombinedLogSink(combinedFile)
	processStreamLogs(strings.NewReader("one\ntwo\n"), outFile, logstore.StdoutStream, sink)
	sink.close()
	if err := outFile.Close(); err != nil {
		t.Fatalf("close out log: %v", err)
	}
	if err := combinedFile.Close(); err != nil {
		t.Fatalf("close combined log: %v", err)
	}

	plain, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read plain log: %v", err)
	}
	if !strings.Contains(string(plain), ": one\n") || !strings.Contains(string(plain), ": two\n") {
		t.Fatalf("expected timestamped plain lines, got %q", string(plain))
	}

	entries, err := logstore.ReadEntries(combinedPath, 10)
	if err != nil {
		t.Fatalf("read combined entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected two entries, got %d", len(entries))
	}
	if entries[0].Stream != logstore.StdoutStream || entries[0].Line != "one" {
		t.Fatalf("unexpected first entry: %#v", entries[0])
	}
	if entries[1].Stream != logstore.StdoutStream || entries[1].Line != "two" {
		t.Fatalf("unexpected second entry: %#v", entries[1])
	}
}
