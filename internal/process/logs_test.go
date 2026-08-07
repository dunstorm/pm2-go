package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dunstorm/pm2-go/internal/logstore"
)

func openManagedTestLogFile(t *testing.T, path string) *managedLogFile {
	t.Helper()

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open log file %s: %v", path, err)
	}
	logFile := registerManagedLogFile(path, file)
	t.Cleanup(logFile.close)
	return logFile
}

func TestProcessStreamLogsWritesPlainAndCombinedLogs(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "api-out.log")
	combinedPath := filepath.Join(dir, "api-combined.jsonl")

	outFile := openManagedTestLogFile(t, outPath)
	errPath := filepath.Join(dir, "api-err.log")
	errFile := openManagedTestLogFile(t, errPath)
	combinedFile := openManagedTestLogFile(t, combinedPath)
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
	outFile := openManagedTestLogFile(t, filepath.Join(dir, "api-out.log"))
	errFile := openManagedTestLogFile(t, filepath.Join(dir, "api-err.log"))
	combinedPath := filepath.Join(dir, "api-combined.jsonl")
	combinedFile := openManagedTestLogFile(t, combinedPath)
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

func TestProcessStreamLogsDrainsBusyStreams(t *testing.T) {
	dir := t.TempDir()
	outFile := openManagedTestLogFile(t, filepath.Join(dir, "api-out.log"))
	errFile := openManagedTestLogFile(t, filepath.Join(dir, "api-err.log"))
	combinedPath := filepath.Join(dir, "api-combined.jsonl")
	combinedFile := openManagedTestLogFile(t, combinedPath)
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

	const lineCount = 1500
	writeDone := make(chan error, 2)
	go writeNumberedLines(stdoutWriter, "stdout", lineCount, writeDone)
	go writeNumberedLines(stderrWriter, "stderr", lineCount, writeDone)
	for i := 0; i < 2; i++ {
		if err := <-writeDone; err != nil {
			t.Fatalf("write busy stream: %v", err)
		}
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("processStreamLogs did not finish while both streams were busy")
	}

	sink.close()

	entries, err := logstore.ReadEntries(combinedPath, lineCount*2)
	if err != nil {
		t.Fatalf("read combined entries: %v", err)
	}
	if len(entries) != lineCount*2 {
		t.Fatalf("expected %d combined entries, got %d", lineCount*2, len(entries))
	}
}

func TestDrainBufferedLinesDrainsFairBoundedBatch(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "api-out.log")
	outFile := openManagedTestLogFile(t, outPath)
	errPath := filepath.Join(dir, "api-err.log")
	errFile := openManagedTestLogFile(t, errPath)

	streams := []*processLogStream{
		{name: logstore.StdoutStream, file: outFile, buffer: []byte(strings.Repeat("out\n", 10))},
		{name: logstore.StderrStream, file: errFile, buffer: []byte(strings.Repeat("err\n", 10))},
	}
	next := 0
	if drained := drainBufferedLines(streams, nil, &next, 6); drained != 6 {
		t.Fatalf("expected six drained lines, got %d", drained)
	}

	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read out log: %v", err)
	}
	stderr, err := os.ReadFile(errPath)
	if err != nil {
		t.Fatalf("read err log: %v", err)
	}
	if strings.Count(string(out), "\n") != 3 {
		t.Fatalf("expected three stdout lines after fair drain, got %q", string(out))
	}
	if strings.Count(string(stderr), "\n") != 3 {
		t.Fatalf("expected three stderr lines after fair drain, got %q", string(stderr))
	}
}

func TestRotateLogFileReopensActiveManagedFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "api-combined.jsonl")
	rotatedPath := logPath + ".0"
	logFile := openManagedTestLogFile(t, logPath)

	logFile.writePlainLine(time.Now(), defaultLogTimestampFormat, "before")
	rotated, err := RotateLogFile(logPath, rotatedPath)
	if err != nil {
		t.Fatalf("rotate log: %v", err)
	}
	if !rotated {
		t.Fatal("expected active log file to rotate")
	}
	logFile.writePlainLine(time.Now(), defaultLogTimestampFormat, "after")

	rotatedContents, err := os.ReadFile(rotatedPath)
	if err != nil {
		t.Fatalf("read rotated log: %v", err)
	}
	if !strings.Contains(string(rotatedContents), "before") {
		t.Fatalf("expected rotated log to contain old line, got %q", string(rotatedContents))
	}
	if strings.Contains(string(rotatedContents), "after") {
		t.Fatalf("expected rotated log not to contain new line, got %q", string(rotatedContents))
	}

	activeContents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read active log: %v", err)
	}
	if !strings.Contains(string(activeContents), "after") {
		t.Fatalf("expected active log to contain new line, got %q", string(activeContents))
	}
	if strings.Contains(string(activeContents), "before") {
		t.Fatalf("expected active log not to contain old line, got %q", string(activeContents))
	}
}

func writeNumberedLines(writer *os.File, prefix string, count int, done chan<- error) {
	var err error
	for i := 0; i < count; i++ {
		if _, err = fmt.Fprintf(writer, "%s-%04d\n", prefix, i); err != nil {
			break
		}
	}
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	done <- err
}
