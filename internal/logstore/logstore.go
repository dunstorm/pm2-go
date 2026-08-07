package logstore

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	StdoutStream = "stdout"
	StderrStream = "stderr"
)

var cursorGenerationCounter atomic.Int64

type Entry struct {
	Timestamp string `json:"timestamp"`
	Stream    string `json:"stream"`
	Line      string `json:"line"`
}

type tailedFile struct {
	file       *os.File
	reader     *bufio.Reader
	id         string
	generation string
	size       int64
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

func CursorGenerationPath(logPath string) string {
	if logPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(logPath), "."+filepath.Base(logPath)+".cursor")
}

func ReadCursorGeneration(logPath string) string {
	path := CursorGenerationPath(logPath)
	if path == "" {
		return ""
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(contents))
}

func BumpCursorGeneration(logPath string) error {
	path := CursorGenerationPath(logPath)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(fmt.Sprintf("%d:%d\n", time.Now().UnixNano(), cursorGenerationCounter.Add(1))), 0600)
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
	tail, err := openTailedFile(filename, true)
	if err != nil {
		return err
	}
	defer tail.close()

	for {
		for line, err := tail.reader.ReadString('\n'); err != io.EOF; line, err = tail.reader.ReadString('\n') {
			entry, parseErr := ParseLine(strings.TrimRight(line, "\n"))
			if parseErr == nil {
				handle(entry)
			}
			if err != nil {
				break
			}
		}
		pos, err := tail.file.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		for {
			time.Sleep(200 * time.Millisecond)
			info, err := os.Stat(filename)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return err
			}
			nextGeneration := ReadCursorGeneration(filename)
			if tail.generation != nextGeneration {
				if err := tail.reopen(filename); err != nil {
					return err
				}
				break
			}

			nextID := fileIdentity(info)
			if tail.id != "" && nextID != "" && tail.id != nextID {
				if err := tail.reopen(filename); err != nil {
					return err
				}
				break
			}

			newSize := info.Size()
			if newSize != tail.size {
				if newSize < tail.size {
					if _, err := tail.file.Seek(0, io.SeekStart); err != nil {
						return err
					}
				} else if _, err := tail.file.Seek(pos, io.SeekStart); err != nil {
					return err
				}
				tail.reader = bufio.NewReader(tail.file)
				tail.size = newSize
				break
			}
		}
	}
}

func openTailedFile(filename string, seekEnd bool) (*tailedFile, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if seekEnd {
		if _, err := file.Seek(0, io.SeekEnd); err != nil {
			_ = file.Close()
			return nil, err
		}
	}

	return &tailedFile{
		file:       file,
		reader:     bufio.NewReader(file),
		id:         fileIdentity(info),
		generation: ReadCursorGeneration(filename),
		size:       info.Size(),
	}, nil
}

func (tail *tailedFile) reopen(filename string) error {
	tail.close()
	next, err := openTailedFile(filename, false)
	if err != nil {
		return err
	}
	*tail = *next
	return nil
}

func (tail *tailedFile) close() {
	if tail == nil || tail.file == nil {
		return
	}
	_ = tail.file.Close()
	tail.file = nil
}

func fileIdentity(info os.FileInfo) string {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino)
	}
	return ""
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
