package logstore

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

type TailCursor struct {
	Offset     int64
	FileID     string
	Generation string
	snapshot   bool
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
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := file.Write([]byte(fmt.Sprintf("%d:%d\n", time.Now().UnixNano(), cursorGenerationCounter.Add(1)))); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	removeTemp = false
	return nil
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
	entries, _, err := ReadEntriesWithCursor(filename, tail)
	return entries, err
}

func ReadEntriesWithOffset(filename string, tail int) ([]Entry, int64, error) {
	entries, cursor, err := ReadEntriesWithCursor(filename, tail)
	return entries, cursor.Offset, err
}

func ReadEntriesWithCursor(filename string, tail int) ([]Entry, TailCursor, error) {
	return readEntriesWithGenerationReader(filename, tail, ReadCursorGeneration)
}

func readEntriesWithGenerationReader(filename string, tail int, readGeneration func(string) string) ([]Entry, TailCursor, error) {
	file, info, generation, err := openSnapshotFile(filename, readGeneration)
	if err != nil {
		return nil, TailCursor{}, err
	}
	defer file.Close()

	return readEntriesFromSnapshot(file, info, generation, tail)
}

func readEntriesFromSnapshot(file *os.File, info os.FileInfo, generation string, tail int) ([]Entry, TailCursor, error) {
	cursor := TailCursor{
		Offset:     info.Size(),
		FileID:     fileIdentity(info),
		Generation: generation,
		snapshot:   true,
	}
	if tail <= 0 || info.Size() == 0 {
		return nil, cursor, nil
	}

	physicalTail := tail
	for {
		lines, cursor, err := readTailLinesFromSnapshot(file, info, generation, physicalTail)
		if err != nil {
			return nil, TailCursor{}, err
		}

		entries := parseEntries(lines)
		if len(entries) >= tail {
			return entries[len(entries)-tail:], cursor, nil
		}
		if len(lines) < physicalTail {
			return entries, cursor, nil
		}
		nextPhysicalTail := physicalTail * 2
		if nextPhysicalTail <= physicalTail {
			nextPhysicalTail = len(lines) + 1
		}
		physicalTail = nextPhysicalTail
	}
}

func parseEntries(lines []string) []Entry {
	entries := make([]Entry, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry, err := ParseLine(line)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func ReadLinesWithCursor(filename string, tail int) ([]string, TailCursor, error) {
	return readTailLinesWithGenerationReader(filename, tail, ReadCursorGeneration)
}

type sortableEntry struct {
	entry      Entry
	occurredAt time.Time
	order      int
}

func ReadMergedEntries(combinedLogPath, stdoutLogPath, stderrLogPath string, tail int) ([]Entry, TailCursor, error) {
	if tail <= 0 {
		return ReadEntriesWithCursor(combinedLogPath, tail)
	}

	legacyEntries, err := legacyLogEntries(stdoutLogPath, stderrLogPath, tail)
	if err != nil {
		return nil, TailCursor{}, err
	}

	combinedEntries, tailCursor, err := ReadEntriesWithCursor(combinedLogPath, tail)
	if err != nil {
		return nil, TailCursor{}, err
	}
	if len(combinedEntries) >= tail {
		return combinedEntries, tailCursor, nil
	}

	if len(legacyEntries) == 0 {
		return combinedEntries, tailCursor, nil
	}

	combinedKeys := make(map[string]int, len(combinedEntries))
	for _, entry := range combinedEntries {
		combinedKeys[entryKey(entry)]++
	}

	records := make([]sortableEntry, 0, len(legacyEntries)+len(combinedEntries))
	order := 0
	for _, entry := range legacyEntries {
		key := entryKey(entry)
		if combinedKeys[key] > 0 {
			combinedKeys[key]--
			continue
		}
		records = append(records, sortableEntry{
			entry:      entry,
			occurredAt: entryTime(entry),
			order:      order,
		})
		order++
	}
	for _, entry := range combinedEntries {
		records = append(records, sortableEntry{
			entry:      entry,
			occurredAt: entryTime(entry),
			order:      order,
		})
		order++
	}

	sort.SliceStable(records, func(i, j int) bool {
		left := records[i]
		right := records[j]
		if !left.occurredAt.IsZero() && !right.occurredAt.IsZero() && !left.occurredAt.Equal(right.occurredAt) {
			return left.occurredAt.Before(right.occurredAt)
		}
		return left.order < right.order
	})
	if len(records) > tail {
		records = records[len(records)-tail:]
	}

	entries := make([]Entry, 0, len(records))
	for _, record := range records {
		entries = append(entries, record.entry)
	}
	return entries, tailCursor, nil
}

func legacyLogEntries(stdoutLogPath, stderrLogPath string, tail int) ([]Entry, error) {
	stdoutEntries, err := plainLogEntries(stdoutLogPath, StdoutStream, tail)
	if err != nil {
		return nil, err
	}
	stderrEntries, err := plainLogEntries(stderrLogPath, StderrStream, tail)
	if err != nil {
		return nil, err
	}
	return append(stdoutEntries, stderrEntries...), nil
}

func plainLogEntries(filename, stream string, tail int) ([]Entry, error) {
	if filename == "" {
		return nil, nil
	}
	lines, _, err := ReadLinesWithCursor(filename, tail)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	entries := make([]Entry, 0, len(lines))
	for _, line := range lines {
		timestamp, message, ok := strings.Cut(line, ": ")
		if !ok || len(timestamp) != len("2006-01-02 15:04:05") {
			entries = append(entries, Entry{
				Stream: stream,
				Line:   line,
			})
			continue
		}
		entries = append(entries, Entry{
			Timestamp: timestamp,
			Stream:    stream,
			Line:      message,
		})
	}
	return entries, nil
}

func entryKey(entry Entry) string {
	return entry.Stream + "\x00" + entrySecond(entry) + "\x00" + entry.Line
}

func entrySecond(entry Entry) string {
	if parsed, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
		return parsed.Local().Format("2006-01-02 15:04:05")
	}
	return entry.Timestamp
}

func entryTime(entry Entry) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
		return parsed.Local()
	}
	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", entry.Timestamp, time.Local); err == nil {
		return parsed
	}
	return time.Time{}
}

func TailEntries(filename string, handle func(Entry)) error {
	return TailEntriesFrom(filename, -1, handle)
}

func TailEntriesFrom(filename string, offset int64, handle func(Entry)) error {
	return TailEntriesFromCursor(filename, TailCursor{Offset: offset}, handle)
}

func TailEntriesFromCursor(filename string, cursor TailCursor, handle func(Entry)) error {
	tail, err := openTailedFile(filename, cursor.Offset < 0)
	if err != nil {
		return err
	}
	defer tail.close()
	if cursor.Offset >= 0 {
		offset := cursor.Offset
		if cursor.staleFor(tail) {
			offset = 0
		}
		if offset > tail.size {
			offset = 0
		}
		if _, err := tail.file.Seek(offset, io.SeekStart); err != nil {
			return err
		}
		tail.reader = bufio.NewReader(tail.file)
	}

	for {
		if err := readAvailableTailEntries(tail.reader, handle); err != nil {
			return err
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
				if err := readAvailableTailEntries(tail.reader, handle); err != nil {
					return err
				}
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

func readAvailableTailEntries(reader *bufio.Reader, handle func(Entry)) error {
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			return nil
		}
		if line != "" {
			entry, parseErr := ParseLine(strings.TrimRight(line, "\n"))
			if parseErr == nil {
				handle(entry)
			}
		}
		if err != nil {
			return err
		}
	}
}

func (cursor TailCursor) staleFor(tail *tailedFile) bool {
	if !cursor.snapshot {
		return false
	}
	if cursor.Generation != tail.generation {
		return true
	}
	if cursor.FileID != "" && tail.id != "" && cursor.FileID != tail.id {
		return true
	}
	return false
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

func readTailLinesWithGenerationReader(filename string, tail int, readGeneration func(string) string) ([]string, TailCursor, error) {
	file, info, generation, err := openSnapshotFile(filename, readGeneration)
	if err != nil {
		return nil, TailCursor{}, err
	}
	defer file.Close()

	return readTailLinesFromSnapshot(file, info, generation, tail)
}

func openSnapshotFile(filename string, readGeneration func(string) string) (*os.File, os.FileInfo, string, error) {
	for {
		generationBefore := readGeneration(filename)
		file, err := os.Open(filename)
		if err != nil {
			return nil, nil, "", err
		}

		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, nil, "", err
		}
		generationAfter := readGeneration(filename)
		if generationBefore == generationAfter {
			return file, info, generationAfter, nil
		}
		_ = file.Close()
	}
}

func readTailLinesFromSnapshot(file *os.File, info os.FileInfo, generation string, tail int) ([]string, TailCursor, error) {
	cursor := TailCursor{
		Offset:     info.Size(),
		FileID:     fileIdentity(info),
		Generation: generation,
		snapshot:   true,
	}
	if tail <= 0 || info.Size() == 0 {
		return nil, cursor, nil
	}

	const chunkSize int64 = 256 * 1024
	position := info.Size()
	segments := make([][]byte, 0)
	bufferLen := 0
	newlineCount := 0
	endsWithoutNewline := false
	firstChunk := true
	for position > 0 {
		readSize := minInt64(position, chunkSize)
		position -= readSize

		chunk := make([]byte, readSize)
		n, err := file.ReadAt(chunk, position)
		if err != nil && err != io.EOF {
			return nil, TailCursor{}, err
		}
		if n == 0 {
			continue
		}
		chunk = chunk[:n]
		if firstChunk {
			endsWithoutNewline = chunk[len(chunk)-1] != '\n'
			firstChunk = false
		}
		segments = append(segments, chunk)
		bufferLen += n
		newlineCount += bytes.Count(chunk, []byte{'\n'})
		if tailLineCount(newlineCount, endsWithoutNewline, position == 0) >= tail {
			break
		}
	}

	buffer := make([]byte, 0, bufferLen)
	for i := len(segments) - 1; i >= 0; i-- {
		buffer = append(buffer, segments[i]...)
	}
	lines := splitTailLines(buffer, tail, position == 0)
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return lines, cursor, nil
}

func tailLineCount(newlineCount int, endsWithoutNewline bool, atStart bool) int {
	if newlineCount == 0 && !endsWithoutNewline {
		return 0
	}

	count := newlineCount
	if endsWithoutNewline {
		count++
	}
	if !atStart {
		count--
	}
	if count < 0 {
		return 0
	}
	return count
}

func splitTailLines(buffer []byte, tail int, atStart bool) []string {
	if !atStart {
		newlineIndex := bytes.IndexByte(buffer, '\n')
		if newlineIndex < 0 {
			return nil
		}
		buffer = buffer[newlineIndex+1:]
	}

	lines := strings.Split(string(buffer), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return lines
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
