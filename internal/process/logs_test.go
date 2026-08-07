package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dunstorm/pm2-go/internal/logstore"
	"golang.org/x/sys/unix"
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

func TestProcessStreamLogsTreatsInvalidPollDescriptorAsClosed(t *testing.T) {
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
	defer stdoutWriter.Close()
	defer stderrWriter.Close()

	sink := newCombinedLogSink(combinedFile)
	done := make(chan struct{})
	go func() {
		defer close(done)
		processStreamLogs(stdoutReader, stderrReader, outFile, errFile, sink)
	}()

	time.Sleep(20 * time.Millisecond)
	_ = stdoutReader.Close()
	_ = stderrReader.Close()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("processStreamLogs did not finish after readers were externally closed")
	}
	sink.close()
}

func TestLogTerminalPollEventsIncludesInvalidDescriptor(t *testing.T) {
	if logTerminalPollEvents&unix.POLLNVAL == 0 {
		t.Fatal("expected invalid poll descriptors to be terminal")
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

func TestRotateLogFileKeepsActiveWriterWhenReopenFails(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "api-combined.jsonl")
	rotatedPath := logPath + ".0"
	logFile := openManagedTestLogFile(t, logPath)

	now := time.Now()
	logFile.writePlainLine(now, defaultLogTimestampFormat, "before")

	previousOpenLogFile := openLogFile
	openLogFile = func(string) (*os.File, error) {
		return nil, errors.New("open failed")
	}
	t.Cleanup(func() {
		openLogFile = previousOpenLogFile
	})

	rotated, err := RotateLogFile(logPath, rotatedPath)
	if err == nil {
		t.Fatal("expected reopen error")
	}
	if rotated {
		t.Fatal("expected rotation to roll back")
	}
	if logFile.file == nil {
		t.Fatal("expected active writer to remain usable")
	}

	logFile.writePlainLine(now, defaultLogTimestampFormat, "after")
	activeContents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read active log: %v", err)
	}
	for _, line := range []string{"before", "after"} {
		if !strings.Contains(string(activeContents), line) {
			t.Fatalf("expected active log to contain %q, got %q", line, string(activeContents))
		}
	}
	if _, err := os.Stat(rotatedPath); !os.IsNotExist(err) {
		t.Fatalf("expected rolled-back archive to be absent, got err=%v", err)
	}
}

func TestRotateLogFileReopensAllActiveManagedFilesForPath(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "api-combined.jsonl")
	rotatedPath := logPath + ".0"
	firstLogFile := openManagedTestLogFile(t, logPath)
	secondLogFile := openManagedTestLogFile(t, logPath)

	now := time.Now()
	firstLogFile.writePlainLine(now, defaultLogTimestampFormat, "before-a")
	secondLogFile.writePlainLine(now, defaultLogTimestampFormat, "before-b")
	rotated, err := RotateLogFile(logPath, rotatedPath)
	if err != nil {
		t.Fatalf("rotate log: %v", err)
	}
	if !rotated {
		t.Fatal("expected active log file to rotate")
	}
	firstLogFile.writePlainLine(now, defaultLogTimestampFormat, "after-a")
	secondLogFile.writePlainLine(now, defaultLogTimestampFormat, "after-b")

	rotatedContents, err := os.ReadFile(rotatedPath)
	if err != nil {
		t.Fatalf("read rotated log: %v", err)
	}
	for _, line := range []string{"before-a", "before-b"} {
		if !strings.Contains(string(rotatedContents), line) {
			t.Fatalf("expected rotated log to contain %q, got %q", line, string(rotatedContents))
		}
	}
	for _, line := range []string{"after-a", "after-b"} {
		if strings.Contains(string(rotatedContents), line) {
			t.Fatalf("expected rotated log not to contain %q, got %q", line, string(rotatedContents))
		}
	}

	activeContents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read active log: %v", err)
	}
	for _, line := range []string{"after-a", "after-b"} {
		if !strings.Contains(string(activeContents), line) {
			t.Fatalf("expected active log to contain %q, got %q", line, string(activeContents))
		}
	}
	for _, line := range []string{"before-a", "before-b"} {
		if strings.Contains(string(activeContents), line) {
			t.Fatalf("expected active log not to contain %q, got %q", line, string(activeContents))
		}
	}
}

func TestRotateLogFileWaitsForPathLock(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "api-combined.jsonl")
	unlock := lockLogFilePath(logPath)

	done := make(chan error, 1)
	go func() {
		_, err := RotateLogFile(logPath, logPath+".0")
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("expected rotation to wait for path lock, got err=%v", err)
	case <-time.After(100 * time.Millisecond):
	}

	unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("rotate after unlock: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for rotation after path unlock")
	}
}

func TestLogFilePathLocksAreReclaimedAfterUse(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "api-combined.jsonl")
	unlock := lockLogFilePath(logPath)

	secondDone := make(chan struct{})
	go func() {
		secondUnlock := lockLogFilePath(logPath)
		secondUnlock()
		close(secondDone)
	}()

	waitForLogFilePathLockRefs(t, logPath, 2)
	unlock()

	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second path lock holder")
	}
	waitForLogFilePathLockRefs(t, logPath, 0)
}

func TestWaitForLogCaptureClosesReadersAfterTimeout(t *testing.T) {
	previousTimeout := logCaptureDrainTimeout
	logCaptureDrainTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		logCaptureDrainTimeout = previousTimeout
	})

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("open pipe: %v", err)
	}
	defer writer.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(io.Discard, reader)
	}()

	started := time.Now()
	waitForLogCapture(&wg, reader)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("expected bounded log capture wait, took %s", elapsed)
	}
}

func waitForLogFilePathLockRefs(t *testing.T, path string, want int) {
	t.Helper()

	for i := 0; i < 100; i++ {
		if got := logFilePathLockRefs(path); got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected path lock refs for %s to become %d, got %d", path, want, logFilePathLockRefs(path))
}

func logFilePathLockRefs(path string) int {
	logFilePathLocksMu.Lock()
	defer logFilePathLocksMu.Unlock()

	lock := logFilePathLocks[path]
	if lock == nil {
		return 0
	}
	return lock.refs
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
