package web

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"
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

	if offset == 0 && size > maxInitialLogBytes {
		offset = size - maxInitialLogBytes
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return logResponse{}, err
		}
		_, _ = bufio.NewReader(file).ReadString('\n')
	} else if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return logResponse{}, err
	}

	contents, err := io.ReadAll(file)
	if err != nil {
		return logResponse{}, err
	}
	lines := splitLogLines(string(contents))
	if offset == 0 && len(lines) > tailLines {
		lines = lines[len(lines)-tailLines:]
	}
	return logResponse{
		Offset: size,
		Size:   size,
		Lines:  lines,
	}, nil
}

func splitLogLines(contents string) []string {
	contents = strings.TrimRight(contents, "\n")
	if contents == "" {
		return nil
	}
	return strings.Split(contents, "\n")
}
