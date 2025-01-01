package shared

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"time"
)

type timestampWriter struct {
	file            *os.File
	timestampFormat string
}

func newTimestampWriter(file *os.File, format string) *timestampWriter {
	if format == "" {
		format = "2006-01-02 15:04:05"
	}
	w := &timestampWriter{
		file:            file,
		timestampFormat: format,
	}
	return w
}

func (w *timestampWriter) Write(p []byte) (n int, err error) {
	return w.file.Write(p)
}

func (w *timestampWriter) processLogs(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		timestamp := time.Now().Format(w.timestampFormat)
		text := scanner.Text()
		fmt.Fprintf(w.file, "%s: %s\n", timestamp, text)
	}
}

func createPipe() (*os.File, *os.File, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	return reader, writer, nil
}
