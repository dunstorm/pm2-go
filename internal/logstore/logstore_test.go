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
