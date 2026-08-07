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
)

const maxIncrementalLogReadBytes = 1024 * 1024
const maxIncrementalLogRecordBytes = 2 * 1024 * 1024

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

	lines, consumedOffset, err := readIncrementalLogLines(reader, readStart, size)
	if err != nil {
		return logResponse{}, err
	}
	return logResponse{
		FileID: currentFileID,
		Offset: consumedOffset,
		Size:   consumedOffset,
		Lines:  lines,
	}, nil
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
