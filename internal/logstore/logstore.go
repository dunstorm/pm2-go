package logstore

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	StdoutStream = "stdout"
	StderrStream = "stderr"
)

type Entry struct {
	Timestamp string `json:"timestamp"`
	Stream    string `json:"stream"`
	Line      string `json:"line"`
}

func CombinedPath(stdoutLogPath string) string {
	if stdoutLogPath == "" {
		return ""
	}

	dir := filepath.Dir(stdoutLogPath)
	base := filepath.Base(stdoutLogPath)
	switch {
	case strings.HasSuffix(base, "-out.log"):
		base = strings.TrimSuffix(base, "-out.log") + "-combined.jsonl"
	case strings.HasSuffix(base, ".log"):
		base = strings.TrimSuffix(base, ".log") + "-combined.jsonl"
	default:
		base += "-combined.jsonl"
	}
	return filepath.Join(dir, base)
}

func ParseLine(line string) (Entry, error) {
	var entry Entry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return Entry{}, err
	}
	if entry.Stream == "" {
		entry.Stream = StdoutStream
	}
	return entry, nil
}

func FormatEntry(entry Entry) string {
	stream := entry.Stream
	if stream == "" {
		stream = StdoutStream
	}

	if entry.Timestamp == "" {
		return fmt.Sprintf("[%s] %s", stream, entry.Line)
	}

	timestamp := entry.Timestamp
	if parsed, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
		timestamp = parsed.Local().Format("2006-01-02 15:04:05")
	}
	return fmt.Sprintf("[%s] %s: %s", stream, timestamp, entry.Line)
}

func ReadEntries(filename string, tail int) ([]Entry, error) {
	lines, err := readTailLines(filename, tail)
	if err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry, err := ParseLine(line)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func TailEntries(filename string, handle func(Entry)) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return err
	}

	reader := bufio.NewReader(file)
	info, err := file.Stat()
	if err != nil {
		return err
	}
	oldSize := info.Size()
	for {
		for line, err := reader.ReadString('\n'); err != io.EOF; line, err = reader.ReadString('\n') {
			entry, parseErr := ParseLine(strings.TrimRight(line, "\n"))
			if parseErr == nil {
				handle(entry)
			}
			if err != nil {
				break
			}
		}
		pos, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		for {
			time.Sleep(200 * time.Millisecond)
			info, err := file.Stat()
			if err != nil {
				return err
			}
			newSize := info.Size()
			if newSize != oldSize {
				if newSize < oldSize {
					if _, err := file.Seek(0, io.SeekStart); err != nil {
						return err
					}
				} else if _, err := file.Seek(pos, io.SeekStart); err != nil {
					return err
				}
				reader = bufio.NewReader(file)
				oldSize = newSize
				break
			}
		}
	}
}

func readTailLines(filename string, tail int) ([]string, error) {
	if tail <= 0 {
		return nil, nil
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	offset := info.Size() - 256*1024
	if offset < 0 {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(file)
	if offset > 0 {
		_, _ = reader.ReadString('\n')
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lines := make([]string, 0, tail)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return lines, nil
}
