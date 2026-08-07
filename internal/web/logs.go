package web

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/dunstorm/pm2-go/internal/logstore"
)

const maxInitialLogBytes = 256 * 1024

type logResponse struct {
	Offset int64    `json:"offset"`
	Size   int64    `json:"size"`
	Lines  []string `json:"lines"`
}

func readLog(filePath string, offset int64, tailLines int) (logResponse, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return logResponse{}, err
	}
	if info.IsDir() {
		return logResponse{}, errors.New("log path is a directory")
	}
	size := info.Size()
	if offset < 0 || offset > size {
		offset = 0
	}
	if tailLines <= 0 {
		tailLines = 200
	}

	file, err := os.Open(filePath)
	if err != nil {
		return logResponse{}, err
	}
	defer file.Close()

	initialRead := offset == 0
	var reader io.Reader = file
	if offset == 0 && size > maxInitialLogBytes {
		offset = size - maxInitialLogBytes
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return logResponse{}, err
		}
		buffered := bufio.NewReader(file)
		_, _ = buffered.ReadString('\n')
		reader = buffered
	} else if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return logResponse{}, err
	}

	contents, err := io.ReadAll(reader)
	if err != nil {
		return logResponse{}, err
	}
	lines := splitLogLines(string(contents))
	if initialRead && len(lines) > tailLines {
		lines = lines[len(lines)-tailLines:]
	}
	return logResponse{
		Offset: size,
		Size:   size,
		Lines:  lines,
	}, nil
}

func readCombinedLog(filePath string, offset int64, tailLines int) (logResponse, error) {
	logs, err := readLog(filePath, offset, tailLines)
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

func splitLogLines(contents string) []string {
	contents = strings.TrimRight(contents, "\n")
	if contents == "" {
		return nil
	}
	return strings.Split(contents, "\n")
}
