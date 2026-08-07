package logstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
