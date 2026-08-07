package process

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	errPath := filepath.Join(dir, "api-err.log")
	errFile, err := os.OpenFile(errPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open err log: %v", err)
	}
	combinedFile, err := os.OpenFile(combinedPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open combined log: %v", err)
	}
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("open stdout pipe: %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("open stderr pipe: %v", err)
	}
	if _, err := stdoutWriter.WriteString("one\ntwo\n"); err != nil {
		t.Fatalf("write stdout pipe: %v", err)
	}
	if err := stdoutWriter.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	if err := stderrWriter.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}

	sink := newCombinedLogSink(combinedFile)
	processStreamLogs(stdoutReader, stderrReader, outFile, errFile, sink)
	sink.close()
	if err := outFile.Close(); err != nil {
		t.Fatalf("close out log: %v", err)
	}
	if err := errFile.Close(); err != nil {
		t.Fatalf("close err log: %v", err)
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

func TestProcessStreamLogsMultiplexesStreamsBetweenBufferedLines(t *testing.T) {
	dir := t.TempDir()
	outFile, err := os.OpenFile(filepath.Join(dir, "api-out.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open out log: %v", err)
	}
	errFile, err := os.OpenFile(filepath.Join(dir, "api-err.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open err log: %v", err)
	}
	combinedPath := filepath.Join(dir, "api-combined.jsonl")
	combinedFile, err := os.OpenFile(combinedPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open combined log: %v", err)
	}
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("open stdout pipe: %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("open stderr pipe: %v", err)
	}

	sink := newCombinedLogSink(combinedFile)
	done := make(chan struct{})
	go func() {
		defer close(done)
		processStreamLogs(stdoutReader, stderrReader, outFile, errFile, sink)
	}()
	if _, err := stdoutWriter.WriteString("stdout-one\n"); err != nil {
		t.Fatalf("write first stdout: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := stderrWriter.WriteString("stderr-one\n"); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := stdoutWriter.WriteString("stdout-two\n"); err != nil {
		t.Fatalf("write second stdout: %v", err)
	}
	if err := stdoutWriter.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	if err := stderrWriter.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	<-done
	sink.close()
	if err := outFile.Close(); err != nil {
		t.Fatalf("close out log: %v", err)
	}
	if err := errFile.Close(); err != nil {
		t.Fatalf("close err log: %v", err)
	}
	if err := combinedFile.Close(); err != nil {
		t.Fatalf("close combined log: %v", err)
	}

	entries, err := logstore.ReadEntries(combinedPath, 10)
	if err != nil {
		t.Fatalf("read combined entries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected three entries, got %d", len(entries))
	}
	got := []string{
		entries[0].Stream + ":" + entries[0].Line,
		entries[1].Stream + ":" + entries[1].Line,
		entries[2].Stream + ":" + entries[2].Line,
	}
	want := []string{
		"stdout:stdout-one",
		"stderr:stderr-one",
		"stdout:stdout-two",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
