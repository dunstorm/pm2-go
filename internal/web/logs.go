package web

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/dunstorm/pm2-go/internal/logstore"
)

type logResponse struct {
	FileID string   `json:"fileId"`
	Offset int64    `json:"offset"`
	Size   int64    `json:"size"`
	Lines  []string `json:"lines"`
}

func readLog(filePath string, offset int64, fileID string, tailLines int) (logResponse, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return logResponse{}, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return logResponse{}, err
	}
	if info.IsDir() {
		return logResponse{}, errors.New("log path is a directory")
	}
	size := info.Size()
	currentFileID := logFileID(filePath, info)
	if fileID != "" && fileID != currentFileID {
		offset = 0
	}
	if offset < 0 || offset > size {
		offset = 0
	}
	if tailLines <= 0 {
		tailLines = 200
	}

	initialRead := offset == 0
	if initialRead {
		lines, cursor, err := logstore.ReadLinesWithCursor(filePath, tailLines)
		if err != nil {
			return logResponse{}, err
		}
		return logResponse{
			FileID: logFileIDFromParts(cursor.FileID, cursor.Generation),
			Offset: cursor.Offset,
			Size:   cursor.Offset,
			Lines:  lines,
		}, nil
	}

	readStart := offset
	var reader io.Reader = file
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return logResponse{}, err
	}

	contents, err := io.ReadAll(reader)
	if err != nil {
		return logResponse{}, err
	}
	lines := splitLogLines(string(contents))
	consumedOffset := readStart + int64(len(contents))
	return logResponse{
		FileID: currentFileID,
		Offset: consumedOffset,
		Size:   consumedOffset,
		Lines:  lines,
	}, nil
}

func readCombinedLog(filePath string, offset int64, fileID string, tailLines int) (logResponse, error) {
	logs, err := readLog(filePath, offset, fileID, tailLines)
	if err != nil {
		return logResponse{}, err
	}

	lines := make([]string, 0, len(logs.Lines))
	for _, line := range logs.Lines {
		entry, err := logstore.ParseLine(line)
		if err != nil {
			continue
		}
		lines = append(lines, logstore.FormatEntry(entry))
	}
	logs.Lines = lines
	return logs, nil
}

func logFileID(filePath string, info os.FileInfo) string {
	generation := logstore.ReadCursorGeneration(filePath)
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return logFileIDFromParts(fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), generation)
	}
	return generation
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
