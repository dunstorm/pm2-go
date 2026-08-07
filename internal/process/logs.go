package process

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/dunstorm/pm2-go/internal/logstore"
)

const defaultLogTimestampFormat = "2006-01-02 15:04:05"

type combinedLogSink struct {
	file    *os.File
	entries chan logstore.Entry
	done    chan struct{}
}

func newCombinedLogSink(file *os.File) *combinedLogSink {
	sink := &combinedLogSink{
		file:    file,
		entries: make(chan logstore.Entry, 256),
		done:    make(chan struct{}),
	}

	go func() {
		defer close(sink.done)
		encoder := json.NewEncoder(sink.file)
		for entry := range sink.entries {
			_ = encoder.Encode(entry)
		}
	}()

	return sink
}

func (sink *combinedLogSink) write(entry logstore.Entry) {
	if sink == nil {
		return
	}
	sink.entries <- entry
}

func (sink *combinedLogSink) close() {
	if sink == nil {
		return
	}
	close(sink.entries)
	<-sink.done
}

func processStreamLogs(reader io.Reader, file *os.File, stream string, sink *combinedLogSink) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		now := time.Now()
		text := scanner.Text()
		fmt.Fprintf(file, "%s: %s\n", now.Format(defaultLogTimestampFormat), text)
		sink.write(logstore.Entry{
			Timestamp: now.UTC().Format(time.RFC3339Nano),
			Stream:    stream,
			Line:      text,
		})
	}
}

func createPipe() (*os.File, *os.File, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	return reader, writer, nil
}
