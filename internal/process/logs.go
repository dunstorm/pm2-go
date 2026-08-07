package process

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dunstorm/pm2-go/internal/logstore"
	"golang.org/x/sys/unix"
)

const defaultLogTimestampFormat = "2006-01-02 15:04:05"

type combinedLogSink struct {
	file    *os.File
	entries chan logstore.Entry
	done    chan struct{}
}

type processLogStream struct {
	name   string
	reader *os.File
	file   *os.File
	buffer []byte
	closed bool
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

func processStreamLogs(stdoutReader, stderrReader *os.File, stdoutFile, stderrFile *os.File, sink *combinedLogSink) {
	streams := []*processLogStream{
		{name: logstore.StdoutStream, reader: stdoutReader, file: stdoutFile},
		{name: logstore.StderrStream, reader: stderrReader, file: stderrFile},
	}
	for _, stream := range streams {
		_ = unix.SetNonblock(int(stream.reader.Fd()), true)
	}

	buffer := make([]byte, 32*1024)
	active := len(streams)
	next := 0
	for active > 0 {
		progressed := false
		pollFDs := make([]unix.PollFd, 0, active)
		pollStreams := make([]*processLogStream, 0, active)

		for _, stream := range streams {
			if stream.closed {
				continue
			}
			pollFDs = append(pollFDs, unix.PollFd{
				Fd:     int32(stream.reader.Fd()),
				Events: unix.POLLIN | unix.POLLHUP | unix.POLLERR,
			})
			pollStreams = append(pollStreams, stream)
		}

		timeout := 10
		if hasAnyBufferedLine(streams) {
			timeout = 0
		}
		_, err := unix.Poll(pollFDs, timeout)
		if err != nil && err != unix.EINTR {
			for _, stream := range pollStreams {
				stream.close(sink)
			}
			break
		}

		for index, pollFD := range pollFDs {
			if pollFD.Revents&(unix.POLLIN|unix.POLLHUP|unix.POLLERR) == 0 {
				continue
			}
			stream := pollStreams[index]

			n, err := unix.Read(int(stream.reader.Fd()), buffer)
			if n > 0 {
				stream.buffer = append(stream.buffer, buffer[:n]...)
				progressed = true
			}
			if err != nil {
				if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
					continue
				}
				stream.close(sink)
				active--
				progressed = true
				continue
			}
			if n == 0 {
				stream.close(sink)
				active--
				progressed = true
			}
		}

		for i := 0; i < len(streams); i++ {
			index := (next + i) % len(streams)
			stream := streams[index]
			if stream.closed || !stream.hasBufferedLine() {
				continue
			}
			stream.emitBufferedLine(sink)
			for !hasOtherBufferedLine(streams, index) && stream.emitBufferedLine(sink) {
			}
			progressed = true
			next = (index + 1) % len(streams)
			break
		}

		if !progressed {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func hasAnyBufferedLine(streams []*processLogStream) bool {
	for _, stream := range streams {
		if !stream.closed && stream.hasBufferedLine() {
			return true
		}
	}
	return false
}

func hasOtherBufferedLine(streams []*processLogStream, current int) bool {
	for index, stream := range streams {
		if index == current || stream.closed {
			continue
		}
		if stream.hasBufferedLine() {
			return true
		}
	}
	return false
}

func (stream *processLogStream) hasBufferedLine() bool {
	return bytes.IndexByte(stream.buffer, '\n') >= 0
}

func (stream *processLogStream) emitBufferedLine(sink *combinedLogSink) bool {
	index := bytes.IndexByte(stream.buffer, '\n')
	if index < 0 {
		return false
	}
	line := string(stream.buffer[:index])
	stream.buffer = stream.buffer[index+1:]
	stream.writeLine(strings.TrimSuffix(line, "\r"), sink)
	return true
}

func (stream *processLogStream) close(sink *combinedLogSink) {
	for stream.emitBufferedLine(sink) {
	}
	if len(stream.buffer) > 0 {
		line := string(stream.buffer)
		stream.buffer = nil
		stream.writeLine(strings.TrimSuffix(line, "\r"), sink)
	}
	stream.closed = true
}

func (stream *processLogStream) writeLine(line string, sink *combinedLogSink) {
	now := time.Now()
	fmt.Fprintf(stream.file, "%s: %s\n", now.Format(defaultLogTimestampFormat), line)
	sink.write(logstore.Entry{
		Timestamp: now.UTC().Format(time.RFC3339Nano),
		Stream:    stream.name,
		Line:      line,
	})
}

func createPipe() (*os.File, *os.File, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	return reader, writer, nil
}
