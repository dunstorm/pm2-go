package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dunstorm/pm2-go/internal/logstore"
	pb "github.com/dunstorm/pm2-go/proto"
)

func TestMergedLogEntriesIncludesLegacyWhenCombinedIsShort(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "api-out.log")
	errPath := filepath.Join(dir, "api-err.log")
	combinedPath := logstore.CombinedPath(outPath)
	baseTime := time.Date(2026, 8, 7, 10, 0, 0, 0, time.Local)

	writePlainLog(t, outPath,
		baseTime.Format("2006-01-02 15:04:05")+": boot",
		baseTime.Add(2*time.Second).Format("2006-01-02 15:04:05")+": ready",
	)
	writePlainLog(t, errPath,
		baseTime.Add(time.Second).Format("2006-01-02 15:04:05")+": warning",
	)
	writePlainLog(t, combinedPath,
		`{"timestamp":"`+baseTime.Add(2*time.Second).Format(time.RFC3339Nano)+`","stream":"stdout","line":"ready"}`,
		`{"timestamp":"`+baseTime.Add(3*time.Second).Format(time.RFC3339Nano)+`","stream":"stderr","line":"failed"}`,
	)

	entries, _, err := mergedLogEntries(&pb.Process{
		LogFilePath: outPath,
		ErrFilePath: errPath,
	}, combinedPath, 4)
	if err != nil {
		t.Fatalf("merge log entries: %v", err)
	}

	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Stream+":"+entry.Line)
	}
	want := []string{
		"stdout:boot",
		"stderr:warning",
		"stdout:ready",
		"stderr:failed",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestMergedLogEntriesPreservesDuplicateLegacyOccurrences(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "api-out.log")
	errPath := filepath.Join(dir, "api-err.log")
	combinedPath := logstore.CombinedPath(outPath)
	baseTime := time.Date(2026, 8, 7, 10, 0, 0, 0, time.Local)
	plainTimestamp := baseTime.Format("2006-01-02 15:04:05")

	writePlainLog(t, outPath,
		plainTimestamp+": repeat",
		plainTimestamp+": repeat",
		plainTimestamp+": repeat",
	)
	writePlainLog(t, combinedPath,
		`{"timestamp":"`+baseTime.Add(100*time.Millisecond).Format(time.RFC3339Nano)+`","stream":"stdout","line":"repeat"}`,
	)

	entries, _, err := mergedLogEntries(&pb.Process{
		LogFilePath: outPath,
		ErrFilePath: errPath,
	}, combinedPath, 4)
	if err != nil {
		t.Fatalf("merge log entries: %v", err)
	}

	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Stream+":"+entry.Line)
	}
	want := []string{
		"stdout:repeat",
		"stdout:repeat",
		"stdout:repeat",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestMergedLogEntriesReadsLegacyTailBeyondFixedByteWindow(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "api-out.log")
	errPath := filepath.Join(dir, "api-err.log")
	combinedPath := logstore.CombinedPath(outPath)
	baseTime := time.Date(2026, 8, 7, 10, 0, 0, 0, time.Local)

	lines := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		lines = append(lines, baseTime.Add(time.Duration(i)*time.Second).Format("2006-01-02 15:04:05")+": line-"+strconv.Itoa(i)+" "+strings.Repeat("x", 80))
	}
	writePlainLog(t, outPath, lines...)
	writePlainLog(t, combinedPath)

	entries, _, err := mergedLogEntries(&pb.Process{
		LogFilePath: outPath,
		ErrFilePath: errPath,
	}, combinedPath, 20)
	if err != nil {
		t.Fatalf("merge log entries: %v", err)
	}
	if len(entries) != 20 {
		t.Fatalf("expected 20 legacy entries, got %d", len(entries))
	}
	if !strings.Contains(entries[0].Line, "line-10") || !strings.Contains(entries[19].Line, "line-29") {
		t.Fatalf("expected full legacy tail from line-10 through line-29, got first=%q last=%q", entries[0].Line, entries[19].Line)
	}
}

func TestMergedLogEntriesReturnsCombinedSnapshotOffset(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "api-out.log")
	combinedPath := logstore.CombinedPath(outPath)
	contents := `{"timestamp":"2026-08-07T10:00:00Z","stream":"stdout","line":"ready"}` + "\n"
	writePlainLog(t, combinedPath, strings.TrimRight(contents, "\n"))

	_, cursor, err := mergedLogEntries(&pb.Process{
		LogFilePath: outPath,
		ErrFilePath: filepath.Join(dir, "api-err.log"),
	}, combinedPath, 10)
	if err != nil {
		t.Fatalf("merge log entries: %v", err)
	}
	if cursor.Offset != int64(len(contents)) {
		t.Fatalf("expected snapshot offset %d, got %d", len(contents), cursor.Offset)
	}
}

func writePlainLog(t *testing.T, filename string, lines ...string) {
	t.Helper()

	if err := os.WriteFile(filename, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatalf("write log %s: %v", filename, err)
	}
}
