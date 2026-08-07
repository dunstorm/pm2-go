package web

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/dunstorm/pm2-go/internal/logstore"
	pb "github.com/dunstorm/pm2-go/proto"
)

const maxIncrementalLogReadBytes = 1024 * 1024
const maxIncrementalLogRecordBytes = 8 * 1024 * 1024

var readLogCursorGeneration = logstore.ReadCursorGeneration
var afterStableLogSnapshot = func(string) {}

type logResponse struct {
	FileID string   `json:"fileId"`
	Offset int64    `json:"offset"`
	Size   int64    `json:"size"`
	Lines  []string `json:"lines"`
}

func readLog(filePath string, offset int64, fileID string, tailLines int) (logResponse, error) {
	file, info, generation, err := openStableLogSnapshot(filePath)
	if err != nil {
		return logResponse{}, err
	}
	defer file.Close()

	if info.IsDir() {
		return logResponse{}, errors.New("log path is a directory")
	}
	size := info.Size()
	currentFileID := logFileID(info, generation)
	currentPhysicalFileID := logPhysicalFileID(info)
	afterStableLogSnapshot(filePath)
	offsetReset := false
	if fileID != "" && fileID != currentFileID {
		offset = 0
		offsetReset = true
	}
	if offset < 0 || offset > size {
		offset = 0
		offsetReset = true
	}
	if tailLines <= 0 {
		tailLines = 200
	}

	initialRead := offset == 0 && (fileID == "" || offsetReset)
	if initialRead {
		return readInitialLogFromSnapshot(filePath, file, info, generation, currentPhysicalFileID, tailLines)
	}

	readStart := offset
	var reader io.Reader = file
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return logResponse{}, err
	}

	lines, consumedOffset, err := readIncrementalLogLines(reader, readStart, size)
	if err != nil {
		return logResponse{}, err
	}
	if logCursorGenerationChangedSinceSnapshot(filePath, generation) {
		return readInitialLog(filePath, tailLines)
	}
	if logPathRotatedSinceSnapshot(filePath, currentPhysicalFileID) {
		var drainedLines []string
		drainedLines, _, err = drainRotatedLogDescriptor(file, consumedOffset)
		if err != nil {
			return logResponse{}, err
		}
		lines = append(lines, drainedLines...)

		replacementLogs, err := readReplacementLogAfterRotation(filePath)
		if err != nil {
			return logResponse{}, err
		}
		replacementLogs.Lines = append(lines, replacementLogs.Lines...)
		return replacementLogs, nil
	}
	return logResponse{
		FileID: currentFileID,
		Offset: consumedOffset,
		Size:   consumedOffset,
		Lines:  lines,
	}, nil
}

func readReplacementLogAfterRotation(filePath string) (logResponse, error) {
	file, info, generation, err := openStableLogSnapshot(filePath)
	if err != nil {
		return logResponse{}, err
	}
	defer file.Close()

	if info.IsDir() {
		return logResponse{}, errors.New("log path is a directory")
	}

	lines, consumedOffset, err := readIncrementalLogLines(file, 0, info.Size())
	if err != nil {
		return logResponse{}, err
	}
	return logResponse{
		FileID: logFileID(info, generation),
		Offset: consumedOffset,
		Size:   consumedOffset,
		Lines:  lines,
	}, nil
}

func readInitialLog(filePath string, tailLines int) (logResponse, error) {
	file, info, generation, err := openStableLogSnapshot(filePath)
	if err != nil {
		return logResponse{}, err
	}
	defer file.Close()

	if info.IsDir() {
		return logResponse{}, errors.New("log path is a directory")
	}

	return readInitialLogFromSnapshot(filePath, file, info, generation, logPhysicalFileID(info), tailLines)
}

func readInitialLogFromSnapshot(filePath string, file *os.File, info os.FileInfo, generation, physicalFileID string, tailLines int) (logResponse, error) {
	lines, cursor, err := logstore.ReadLinesWithCursorFromSnapshot(file, info, generation, tailLines)
	if err != nil {
		return logResponse{}, err
	}
	if logCursorGenerationChangedSinceSnapshot(filePath, generation) {
		return readInitialLog(filePath, tailLines)
	}
	if logPathRotatedSinceSnapshot(filePath, physicalFileID) {
		drainedLines, _, err := drainRotatedLogDescriptor(file, cursor.Offset)
		if err != nil {
			return logResponse{}, err
		}
		lines = append(lines, drainedLines...)

		replacementLogs, err := readReplacementLogAfterRotation(filePath)
		if err != nil {
			return logResponse{}, err
		}
		replacementLogs.Lines = append(lines, replacementLogs.Lines...)
		return replacementLogs, nil
	}

	return logResponse{
		FileID: logFileIDFromParts(cursor.FileID, cursor.Generation),
		Offset: cursor.Offset,
		Size:   cursor.Offset,
		Lines:  lines,
	}, nil
}

func openStableLogSnapshot(filePath string) (*os.File, os.FileInfo, string, error) {
	for attempt := 0; attempt < 3; attempt++ {
		generationBefore := readLogCursorGeneration(filePath)
		file, err := os.Open(filePath)
		if err != nil {
			return nil, nil, "", err
		}
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, nil, "", err
		}
		generationAfter := readLogCursorGeneration(filePath)
		if generationBefore != generationAfter {
			_ = file.Close()
			continue
		}
		return file, info, generationAfter, nil
	}
	return nil, nil, "", errors.New("log changed while opening")
}

func readIncrementalLogLines(reader io.Reader, readStart, fileSize int64) ([]string, int64, error) {
	remaining := fileSize - readStart
	if remaining <= 0 {
		return nil, readStart, nil
	}
	limit := remaining
	if limit > maxIncrementalLogReadBytes {
		limit = maxIncrementalLogReadBytes
	}

	contents, err := io.ReadAll(io.LimitReader(reader, limit))
	if err != nil {
		return nil, 0, err
	}
	consumedBytes := len(contents)
	if int64(consumedBytes) < remaining {
		lastNewline := bytes.LastIndexByte(contents, '\n')
		if lastNewline >= 0 {
			contents = contents[:lastNewline+1]
			consumedBytes = lastNewline + 1
		} else {
			extraLimit := maxIncrementalLogRecordBytes - int64(consumedBytes)
			if extraLimit > remaining-int64(consumedBytes) {
				extraLimit = remaining - int64(consumedBytes)
			}
			if extraLimit > 0 {
				extra, err := io.ReadAll(io.LimitReader(reader, extraLimit))
				if err != nil {
					return nil, 0, err
				}
				contents = append(contents, extra...)
				consumedBytes = len(contents)
			}
			firstNewline := bytes.IndexByte(contents, '\n')
			if firstNewline >= 0 {
				contents = contents[:firstNewline+1]
				consumedBytes = firstNewline + 1
			} else if int64(consumedBytes) < remaining {
				return nil, readStart, nil
			}
		}
	}

	return splitLogLines(string(contents)), readStart + int64(consumedBytes), nil
}

func drainRotatedLogDescriptor(file *os.File, offset int64) ([]string, int64, error) {
	lines := []string{}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}
	for {
		info, err := file.Stat()
		if err != nil {
			return nil, offset, err
		}
		if offset >= info.Size() {
			return lines, offset, nil
		}

		nextLines, nextOffset, err := readIncrementalLogLines(file, offset, info.Size())
		if err != nil {
			return nil, offset, err
		}
		lines = append(lines, nextLines...)
		if nextOffset <= offset {
			return lines, offset, nil
		}
		offset = nextOffset
	}
}

func readCombinedLog(filePath string, offset int64, fileID string, tailLines int) (logResponse, error) {
	if offset == 0 && fileID == "" {
		return readInitialCombinedLog(filePath, tailLines)
	}

	logs, err := readLog(filePath, offset, fileID, tailLines)
	if err != nil {
		return logResponse{}, err
	}

	logs.Lines = validCombinedLogLines(logs.Lines)
	return logs, nil
}

func readProcessCombinedLog(process *pb.Process, offset int64, fileID string, tailLines int) (logResponse, error) {
	combinedLogPath := logstore.CombinedPath(process.LogFilePath)
	if offset != 0 || fileID != "" {
		return readCombinedLog(combinedLogPath, offset, fileID, tailLines)
	}
	if tailLines <= 0 {
		tailLines = 200
	}

	entries, cursor, err := logstore.ReadMergedEntries(combinedLogPath, process.LogFilePath, process.ErrFilePath, tailLines)
	if err != nil {
		return logResponse{}, err
	}
	return logResponse{
		FileID: logFileIDFromParts(cursor.FileID, cursor.Generation),
		Offset: cursor.Offset,
		Size:   cursor.Offset,
		Lines:  formatLogEntries(entries),
	}, nil
}

func readInitialCombinedLog(filePath string, tailLines int) (logResponse, error) {
	if tailLines <= 0 {
		tailLines = 200
	}

	physicalTail := tailLines
	for {
		lines, cursor, err := logstore.ReadLinesWithCursor(filePath, physicalTail)
		if err != nil {
			return logResponse{}, err
		}
		validLines := validCombinedLogLines(lines)
		if len(validLines) > tailLines {
			validLines = validLines[len(validLines)-tailLines:]
		}
		if len(validLines) >= tailLines || len(lines) < physicalTail {
			return logResponse{
				FileID: logFileIDFromParts(cursor.FileID, cursor.Generation),
				Offset: cursor.Offset,
				Size:   cursor.Offset,
				Lines:  validLines,
			}, nil
		}
		physicalTail *= 2
	}
}

func validCombinedLogLines(lines []string) []string {
	validLines := make([]string, 0, len(lines))
	for _, line := range lines {
		entry, err := logstore.ParseLine(line)
		if err != nil {
			continue
		}
		validLines = append(validLines, logstore.FormatEntry(entry))
	}
	return validLines
}

func formatLogEntries(entries []logstore.Entry) []string {
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, logstore.FormatEntry(entry))
	}
	return lines
}

func logFileID(info os.FileInfo, generation string) string {
	return logFileIDFromParts(logPhysicalFileID(info), generation)
}

func logPhysicalFileID(info os.FileInfo) string {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino)
	}
	return ""
}

func logPathRotatedSinceSnapshot(filePath, physicalFileID string) bool {
	if physicalFileID == "" {
		return false
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return true
	}
	return logPhysicalFileID(info) != physicalFileID
}

func logCursorGenerationChangedSinceSnapshot(filePath, generation string) bool {
	return readLogCursorGeneration(filePath) != generation
}

func logFileIDFromParts(fileID, generation string) string {
	if fileID != "" && generation != "" {
		return fileID + ":" + generation
	}
	if fileID != "" {
		return fileID
	}
	return generation
}

func splitLogLines(contents string) []string {
	contents = strings.TrimRight(contents, "\n")
	if contents == "" {
		return nil
	}
	return strings.Split(contents, "\n")
}
