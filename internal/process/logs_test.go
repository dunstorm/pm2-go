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

func TestProcessStreamLogsDrainsBusyStreams(t *testing.T) {
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
	if err := outFile.Close(); err != nil {
		t.Fatalf("close out log: %v", err)
	}
	if err := errFile.Close(); err != nil {
		t.Fatalf("close err log: %v", err)
	}
	if err := combinedFile.Close(); err != nil {
		t.Fatalf("close combined log: %v", err)
	}

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
	outFile, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open out log: %v", err)
	}
	errPath := filepath.Join(dir, "api-err.log")
	errFile, err := os.OpenFile(errPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open err log: %v", err)
	}

	streams := []*processLogStream{
		{name: logstore.StdoutStream, file: outFile, buffer: []byte(strings.Repeat("out\n", 10))},
		{name: logstore.StderrStream, file: errFile, buffer: []byte(strings.Repeat("err\n", 10))},
	}
	next := 0
	if drained := drainBufferedLines(streams, nil, &next, 6); drained != 6 {
		t.Fatalf("expected six drained lines, got %d", drained)
	}
	if err := outFile.Close(); err != nil {
		t.Fatalf("close out log: %v", err)
	}
	if err := errFile.Close(); err != nil {
		t.Fatalf("close err log: %v", err)
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
