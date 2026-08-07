package logstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCombinedPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "standard stdout log",
			path: filepath.Join("logs", "api-out.log"),
			want: filepath.Join("logs", "api-combined.jsonl"),
		},
		{
			name: "custom log path",
			path: filepath.Join("logs", "api.log"),
			want: filepath.Join("logs", "api-combined.jsonl"),
		},
		{
			name: "extensionless path",
			path: filepath.Join("logs", "api"),
			want: filepath.Join("logs", "api-combined.jsonl"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CombinedPath(tt.path); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestReadEntriesPreservesFileOrder(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "api-combined.jsonl")
	contents := strings.Join([]string{
		`{"timestamp":"2026-08-07T10:00:00Z","stream":"stdout","line":"first"}`,
		`{"timestamp":"2026-08-07T10:00:01Z","stream":"stderr","line":"second"}`,
		`{"timestamp":"2026-08-07T10:00:02Z","stream":"stdout","line":"third"}`,
		"",
	}, "\n")
	if err := os.WriteFile(filePath, []byte(contents), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	entries, err := ReadEntries(filePath, 2)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected two entries, got %d", len(entries))
	}
	if entries[0].Line != "second" || entries[1].Line != "third" {
		t.Fatalf("expected ordered tail, got %#v", entries)
	}
}

func TestReadEntriesSkipsMalformedRecords(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "api-combined.jsonl")
	contents := strings.Join([]string{
		`{"stream":"stdout","line":"before"}`,
		`{"stream":"stdout","line":`,
		`{"stream":"stderr","line":"after"}`,
		"",
	}, "\n")
	if err := os.WriteFile(filePath, []byte(contents), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	entries, err := ReadEntries(filePath, 10)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected two valid entries, got %d", len(entries))
	}
	if entries[0].Line != "before" || entries[1].Line != "after" {
		t.Fatalf("expected valid entries around malformed record, got %#v", entries)
	}
}

func TestReadEntriesReadsLargeFinalRecordBeyondInitialWindow(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "api-combined.jsonl")
	largeLine := strings.Repeat("x", 300*1024)
	contents := strings.Join([]string{
		`{"stream":"stdout","line":"before"}`,
		`{"stream":"stderr","line":"` + largeLine + `"}`,
		"",
	}, "\n")
	if err := os.WriteFile(filePath, []byte(contents), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	entries, err := ReadEntries(filePath, 1)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}
	if entries[0].Stream != StderrStream || entries[0].Line != largeLine {
		t.Fatalf("expected large final stderr entry, got stream=%q line length=%d", entries[0].Stream, len(entries[0].Line))
	}
}

func TestReadEntriesWithOffsetReturnsConsumedOffset(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "api-combined.jsonl")
	contents := `{"stream":"stdout","line":"ready"}` + "\n"
	if err := os.WriteFile(filePath, []byte(contents), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	_, offset, err := ReadEntriesWithOffset(filePath, 10)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if offset != int64(len(contents)) {
		t.Fatalf("expected consumed offset %d, got %d", len(contents), offset)
	}
}

func TestReadEntriesWithOffsetReturnsFileSizeWhenTailDisabled(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "api-combined.jsonl")
	contents := `{"stream":"stdout","line":"ready"}` + "\n"
	if err := os.WriteFile(filePath, []byte(contents), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	entries, offset, err := ReadEntriesWithOffset(filePath, 0)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no history entries, got %#v", entries)
	}
	if offset != int64(len(contents)) {
		t.Fatalf("expected consumed offset %d, got %d", len(contents), offset)
	}
}

func TestReadEntriesWithCursorReturnsSnapshotIdentity(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "api-combined.jsonl")
	contents := `{"stream":"stdout","line":"ready"}` + "\n"
	if err := os.WriteFile(filePath, []byte(contents), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	if err := BumpCursorGeneration(filePath); err != nil {
		t.Fatalf("bump cursor generation: %v", err)
	}

	_, cursor, err := ReadEntriesWithCursor(filePath, 10)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if cursor.Offset != int64(len(contents)) {
		t.Fatalf("expected consumed offset %d, got %d", len(contents), cursor.Offset)
	}
	if cursor.FileID == "" {
		t.Fatal("expected snapshot file identity")
	}
	if cursor.Generation == "" {
		t.Fatal("expected snapshot cursor generation")
	}
}

func TestReadLinesWithCursorRetriesWhenGenerationChangesAroundSnapshot(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "api-combined.jsonl")
	oldContents := `{"stream":"stdout","line":"old"}` + "\n"
	if err := os.WriteFile(filePath, []byte(oldContents), 0600); err != nil {
		t.Fatalf("write old log: %v", err)
	}

	newContents := `{"stream":"stderr","line":"new"}` + "\n"
	generationReads := 0
	readGeneration := func(string) string {
		generationReads++
		if generationReads == 2 {
			if err := os.WriteFile(filePath, []byte(newContents), 0600); err != nil {
				t.Fatalf("write regenerated log: %v", err)
			}
			return "new-generation"
		}
		if generationReads > 2 {
			return "new-generation"
		}
		return "old-generation"
	}

	lines, cursor, err := readTailLinesWithGenerationReader(filePath, 1, readGeneration)
	if err != nil {
		t.Fatalf("read lines: %v", err)
	}
	if len(lines) != 1 || lines[0] != strings.TrimRight(newContents, "\n") {
		t.Fatalf("expected regenerated line, got %#v", lines)
	}
	if cursor.Offset != int64(len(newContents)) {
		t.Fatalf("expected regenerated offset %d, got %d", len(newContents), cursor.Offset)
	}
	if cursor.Generation != "new-generation" {
		t.Fatalf("expected new generation cursor, got %q", cursor.Generation)
	}
}

func TestFormatEntry(t *testing.T) {
	line := FormatEntry(Entry{
		Timestamp: "2026-08-07T10:00:00Z",
		Stream:    StderrStream,
		Line:      "failed",
	})
	if !strings.Contains(line, "[stderr]") || !strings.Contains(line, "failed") {
		t.Fatalf("unexpected formatted line: %q", line)
	}
}

func TestTailEntriesFromCursorResetsOnCursorGenerationChange(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "api-combined.jsonl")
	oldLine := `{"stream":"stdout","line":"old"}` + "\n"
	if err := os.WriteFile(filePath, []byte(oldLine), 0600); err != nil {
		t.Fatalf("write initial log: %v", err)
	}

	_, cursor, err := ReadEntriesWithCursor(filePath, 0)
	if err != nil {
		t.Fatalf("read cursor: %v", err)
	}

	newLine := `{"stream":"stderr","line":"` + strings.Repeat("new", 64) + `"}`
	if err := os.Truncate(filePath, 0); err != nil {
		t.Fatalf("truncate log: %v", err)
	}
	if err := BumpCursorGeneration(filePath); err != nil {
		t.Fatalf("bump cursor generation: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(newLine+"\n"), 0600); err != nil {
		t.Fatalf("write regenerated log: %v", err)
	}

	entries := make(chan Entry, 1)
	errs := make(chan error, 1)
	go func() {
		errs <- TailEntriesFromCursor(filePath, cursor, func(entry Entry) {
			entries <- entry
		})
	}()

	select {
	case entry := <-entries:
		if entry.Stream != StderrStream || entry.Line != strings.Repeat("new", 64) {
			t.Fatalf("expected regenerated entry, got %#v", entry)
		}
	case err := <-errs:
		t.Fatalf("tail failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for regenerated entry")
	}
}

func TestTailEntriesFromCursorResetsOnFileIdentityChange(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "api-combined.jsonl")
	oldLine := `{"stream":"stdout","line":"old"}` + "\n"
	if err := os.WriteFile(filePath, []byte(oldLine), 0600); err != nil {
		t.Fatalf("write initial log: %v", err)
	}

	_, cursor, err := ReadEntriesWithCursor(filePath, 0)
	if err != nil {
		t.Fatalf("read cursor: %v", err)
	}
	if cursor.FileID == "" {
		t.Skip("file identity is unavailable on this platform")
	}

	newLine := `{"stream":"stderr","line":"` + strings.Repeat("rotated", 32) + `"}`
	if err := os.Rename(filePath, filePath+".0"); err != nil {
		t.Fatalf("rotate log: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(newLine+"\n"), 0600); err != nil {
		t.Fatalf("write replacement log: %v", err)
	}

	entries := make(chan Entry, 1)
	errs := make(chan error, 1)
	go func() {
		errs <- TailEntriesFromCursor(filePath, cursor, func(entry Entry) {
			entries <- entry
		})
	}()

	select {
	case entry := <-entries:
		if entry.Stream != StderrStream || entry.Line != strings.Repeat("rotated", 32) {
			t.Fatalf("expected replacement entry, got %#v", entry)
		}
	case err := <-errs:
		t.Fatalf("tail failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for replacement entry")
	}
}

func TestTailEntriesFromCursorDrainsRotatedDescriptorBeforeReopen(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "api-combined.jsonl")
	firstLine := `{"stream":"stdout","line":"first"}` + "\n"
	if err := os.WriteFile(filePath, []byte(firstLine), 0600); err != nil {
		t.Fatalf("write initial log: %v", err)
	}

	_, cursor, err := ReadEntriesWithCursor(filePath, 0)
	if err != nil {
		t.Fatalf("read cursor: %v", err)
	}
	if cursor.FileID == "" {
		t.Skip("file identity is unavailable on this platform")
	}

	entries := make(chan Entry, 2)
	errs := make(chan error, 1)
	go func() {
		errs <- TailEntriesFromCursor(filePath, cursor, func(entry Entry) {
			entries <- entry
		})
	}()

	time.Sleep(250 * time.Millisecond)
	secondLine := `{"stream":"stdout","line":"second"}`
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open log for append: %v", err)
	}
	if _, err := file.WriteString(secondLine + "\n"); err != nil {
		_ = file.Close()
		t.Fatalf("append pending line: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close appended log: %v", err)
	}
	if err := os.Rename(filePath, filePath+".0"); err != nil {
		t.Fatalf("rotate log: %v", err)
	}
	thirdLine := `{"stream":"stderr","line":"third"}`
	if err := os.WriteFile(filePath, []byte(thirdLine+"\n"), 0600); err != nil {
		t.Fatalf("write replacement log: %v", err)
	}

	select {
	case entry := <-entries:
		if entry.Stream != StdoutStream || entry.Line != "second" {
			t.Fatalf("expected drained rotated entry, got %#v", entry)
		}
	case err := <-errs:
		t.Fatalf("tail failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for drained rotated entry")
	}
}

func TestTailEntriesFromStartsAtOffset(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "api-combined.jsonl")
	firstLine := `{"stream":"stdout","line":"first"}` + "\n"
	if err := os.WriteFile(filePath, []byte(firstLine), 0600); err != nil {
		t.Fatalf("write initial log: %v", err)
	}

	entries := make(chan Entry, 1)
	errs := make(chan error, 1)
	go func() {
		errs <- TailEntriesFrom(filePath, int64(len(firstLine)), func(entry Entry) {
			entries <- entry
		})
	}()

	time.Sleep(300 * time.Millisecond)
	if err := appendLogLine(filePath, `{"stream":"stderr","line":"second"}`); err != nil {
		t.Fatalf("append log: %v", err)
	}

	select {
	case entry := <-entries:
		if entry.Stream != StderrStream || entry.Line != "second" {
			t.Fatalf("expected second entry, got %#v", entry)
		}
	case err := <-errs:
		t.Fatalf("tail failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tailed entry")
	}
}

func TestTailEntriesReopensAfterRotation(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "api-combined.jsonl")
	if err := os.WriteFile(filePath, []byte(`{"stream":"stdout","line":"before"}`+"\n"), 0600); err != nil {
		t.Fatalf("write initial log: %v", err)
	}

	entries := make(chan Entry, 1)
	errs := make(chan error, 1)
	go func() {
		errs <- TailEntries(filePath, func(entry Entry) {
			entries <- entry
		})
	}()

	time.Sleep(300 * time.Millisecond)
	if err := os.Rename(filePath, filePath+".0"); err != nil {
		t.Fatalf("rotate log: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(`{"stream":"stderr","line":"after"}`+"\n"), 0600); err != nil {
		t.Fatalf("write replacement log: %v", err)
	}

	select {
	case entry := <-entries:
		if entry.Stream != StderrStream || entry.Line != "after" {
			t.Fatalf("expected replacement entry, got %#v", entry)
		}
	case err := <-errs:
		t.Fatalf("tail failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tailed replacement entry")
	}
}

func appendLogLine(filePath, line string) error {
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(line + "\n")
	return err
}

func TestTailEntriesReopensAfterCursorGenerationChanges(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "api-combined.jsonl")
	if err := os.WriteFile(filePath, []byte(`{"stream":"stdout","line":"before"}`+"\n"), 0600); err != nil {
		t.Fatalf("write initial log: %v", err)
	}

	entries := make(chan Entry, 1)
	errs := make(chan error, 1)
	go func() {
		errs <- TailEntries(filePath, func(entry Entry) {
			entries <- entry
		})
	}()

	time.Sleep(300 * time.Millisecond)
	if err := os.Truncate(filePath, 0); err != nil {
		t.Fatalf("truncate log: %v", err)
	}
	if err := BumpCursorGeneration(filePath); err != nil {
		t.Fatalf("bump cursor generation: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(`{"stream":"stderr","line":"after-after-after-after"}`+"\n"), 0600); err != nil {
		t.Fatalf("write regenerated log: %v", err)
	}

	select {
	case entry := <-entries:
		if entry.Stream != StderrStream || entry.Line != "after-after-after-after" {
			t.Fatalf("expected regenerated entry, got %#v", entry)
		}
	case err := <-errs:
		t.Fatalf("tail failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tailed regenerated entry")
	}
}
