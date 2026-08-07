package process

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dunstorm/pm2-go/internal/logstore"
	"golang.org/x/sys/unix"
)

const defaultLogTimestampFormat = "2006-01-02 15:04:05"

const (
	logReadBufferSize       = 32 * 1024
	maxLogDrainLinesPerPass = 1024
	maxStreamLogBufferBytes = 1024 * 1024
	logPollEvents           = unix.POLLIN | unix.POLLHUP | unix.POLLERR
	logTerminalPollEvents   = unix.POLLHUP | unix.POLLERR | unix.POLLNVAL
)

type managedLogFile struct {
	path string
	mu   sync.Mutex
	file *os.File
}

type activeManagedLogFiles struct {
	files map[*managedLogFile]struct{}
}

type combinedLogSink struct {
	file    *managedLogFile
	entries chan logstore.Entry
	done    chan struct{}
}

type processLogStream struct {
	name   string
	reader *os.File
	file   *managedLogFile
	buffer []byte
	closed bool
}

var activeLogFiles sync.Map
var logFilePathLocks sync.Map

func lockLogFilePath(path string) func() {
	if path == "" {
		return func() {}
	}
	value, _ := logFilePathLocks.LoadOrStore(path, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func openManagedLogFile(path string) (*managedLogFile, error) {
	unlock := lockLogFilePath(path)
	defer unlock()

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0640)
	if err != nil {
		return nil, err
	}
	return registerManagedLogFileLocked(path, file), nil
}

func registerManagedLogFile(path string, file *os.File) *managedLogFile {
	unlock := lockLogFilePath(path)
	defer unlock()

	return registerManagedLogFileLocked(path, file)
}

func registerManagedLogFileLocked(path string, file *os.File) *managedLogFile {
	logFile := &managedLogFile{path: path, file: file}
	value, _ := activeLogFiles.LoadOrStore(path, &activeManagedLogFiles{files: make(map[*managedLogFile]struct{})})
	active := value.(*activeManagedLogFiles)
	active.files[logFile] = struct{}{}
	return logFile
}

func RotateLogFile(filename, rotatedFilename string) (bool, error) {
	if filename == "" || rotatedFilename == "" {
		return false, nil
	}

	unlock := lockLogFilePath(filename)
	defer unlock()

	files := activeManagedLogFileList(filename)
	if len(files) > 0 {
		return rotateManagedLogFiles(filename, rotatedFilename, files)
	}
	return rotateInactiveLogFile(filename, rotatedFilename)
}

func activeManagedLogFileList(filename string) []*managedLogFile {
	value, ok := activeLogFiles.Load(filename)
	if !ok {
		return nil
	}
	active := value.(*activeManagedLogFiles)
	files := make([]*managedLogFile, 0, len(active.files))
	for logFile := range active.files {
		files = append(files, logFile)
	}
	return files
}

func rotateLogPath(filename, rotatedFilename string) (bool, error) {
	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := os.Rename(filename, rotatedFilename); err != nil {
		return false, err
	}
	return true, nil
}

func rotateInactiveLogFile(filename, rotatedFilename string) (bool, error) {
	rotated, err := rotateLogPath(filename, rotatedFilename)
	if err != nil || !rotated {
		return rotated, err
	}
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0640)
	if err != nil {
		return true, err
	}
	if err := file.Close(); err != nil {
		return true, err
	}
	return true, nil
}

func rotateManagedLogFiles(filename, rotatedFilename string, files []*managedLogFile) (bool, error) {
	for _, logFile := range files {
		logFile.mu.Lock()
	}
	defer func() {
		for i := len(files) - 1; i >= 0; i-- {
			files[i].mu.Unlock()
		}
	}()

	for _, logFile := range files {
		if logFile.file != nil {
			_ = logFile.file.Close()
			logFile.file = nil
		}
	}

	rotated, rotateErr := rotateLogPath(filename, rotatedFilename)
	var openErr error
	for _, logFile := range files {
		file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0640)
		if err != nil {
			if openErr == nil {
				openErr = err
			}
			continue
		}
		logFile.file = file
	}

	if rotateErr != nil {
		return rotated, rotateErr
	}
	if openErr != nil {
		return rotated, openErr
	}
	return rotated, rotateErr
}

func (logFile *managedLogFile) close() {
	if logFile == nil {
		return
	}

	unlock := lockLogFilePath(logFile.path)
	defer unlock()

	logFile.mu.Lock()
	if logFile.file != nil {
		_ = logFile.file.Close()
		logFile.file = nil
	}
	logFile.mu.Unlock()

	if value, ok := activeLogFiles.Load(logFile.path); ok {
		active := value.(*activeManagedLogFiles)
		delete(active.files, logFile)
		if len(active.files) == 0 {
			activeLogFiles.Delete(logFile.path)
		}
	}
}

func (logFile *managedLogFile) writePlainLine(timestamp time.Time, timestampFormat, line string) {
	if logFile == nil {
		return
	}
	logFile.mu.Lock()
	defer logFile.mu.Unlock()
	if logFile.file == nil {
		return
	}
	fmt.Fprintf(logFile.file, "%s: %s\n", timestamp.Format(timestampFormat), line)
}

func (logFile *managedLogFile) writeEntry(entry logstore.Entry) {
	if logFile == nil {
		return
	}
	logFile.mu.Lock()
	defer logFile.mu.Unlock()
	if logFile.file == nil {
		return
	}
	_ = json.NewEncoder(logFile.file).Encode(entry)
}

func newCombinedLogSink(file *managedLogFile) *combinedLogSink {
	sink := &combinedLogSink{
		file:    file,
		entries: make(chan logstore.Entry, 256),
		done:    make(chan struct{}),
	}

	go func() {
		defer close(sink.done)
		for entry := range sink.entries {
			sink.file.writeEntry(entry)
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

func processStreamLogs(stdoutReader, stderrReader *os.File, stdoutFile, stderrFile *managedLogFile, sink *combinedLogSink) {
	streams := []*processLogStream{
		{name: logstore.StdoutStream, reader: stdoutReader, file: stdoutFile},
		{name: logstore.StderrStream, reader: stderrReader, file: stderrFile},
	}
	active := 0
	for _, stream := range streams {
		fd, ok := stream.fileDescriptor()
		if !ok {
			stream.closed = true
			continue
		}
		_ = unix.SetNonblock(fd, true)
		active++
	}

	buffer := make([]byte, logReadBufferSize)
	next := 0
	for active > 0 {
		progressed := false

		if drainBufferedLines(streams, sink, &next, maxLogDrainLinesPerPass) > 0 {
			continue
		}
		if flushOversizedBuffers(streams, sink) {
			continue
		}

		pollFDs := make([]unix.PollFd, 0, active)
		pollStreams := make([]*processLogStream, 0, active)

		for _, stream := range streams {
			if stream.closed {
				continue
			}
			fd, ok := stream.fileDescriptor()
			if !ok {
				closeProcessLogStream(stream, sink, &active)
				progressed = true
				continue
			}
			pollFDs = append(pollFDs, unix.PollFd{
				Fd:     int32(fd),
				Events: logPollEvents,
			})
			pollStreams = append(pollStreams, stream)
		}
		if len(pollFDs) == 0 {
			continue
		}

		_, err := unix.Poll(pollFDs, 10)
		if err != nil && err != unix.EINTR {
			for _, stream := range pollStreams {
				closeProcessLogStream(stream, sink, &active)
			}
			break
		}

		for index, pollFD := range pollFDs {
			if pollFD.Revents&unix.POLLNVAL != 0 {
				closeProcessLogStream(pollStreams[index], sink, &active)
				progressed = true
				continue
			}
			if pollFD.Revents&(unix.POLLIN|logTerminalPollEvents) == 0 {
				continue
			}
			stream := pollStreams[index]
			fd, ok := stream.fileDescriptor()
			if !ok {
				closeProcessLogStream(stream, sink, &active)
				progressed = true
				continue
			}

			n, err := unix.Read(fd, buffer)
			if n > 0 {
				stream.buffer = append(stream.buffer, buffer[:n]...)
				progressed = true
			}
			if err != nil {
				if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
					continue
				}
				closeProcessLogStream(stream, sink, &active)
				progressed = true
				continue
			}
			if n == 0 {
				closeProcessLogStream(stream, sink, &active)
				progressed = true
			}
		}

		if !progressed {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (stream *processLogStream) fileDescriptor() (int, bool) {
	if stream.reader == nil {
		return -1, false
	}
	fd := int(stream.reader.Fd())
	return fd, fd >= 0
}

func closeProcessLogStream(stream *processLogStream, sink *combinedLogSink, active *int) {
	if stream.closed {
		return
	}
	stream.close(sink)
	*active = *active - 1
}

func (stream *processLogStream) hasBufferedLine() bool {
	return bytes.IndexByte(stream.buffer, '\n') >= 0
}

func drainBufferedLines(streams []*processLogStream, sink *combinedLogSink, next *int, limit int) int {
	drained := 0
	for drained < limit {
		emitted := false
		for i := 0; i < len(streams); i++ {
			index := (*next + i) % len(streams)
			stream := streams[index]
			if stream.closed || !stream.hasBufferedLine() {
				continue
			}
			stream.emitBufferedLine(sink)
			*next = (index + 1) % len(streams)
			drained++
			emitted = true
			break
		}
		if !emitted {
			break
		}
	}
	return drained
}

func flushOversizedBuffers(streams []*processLogStream, sink *combinedLogSink) bool {
	flushed := false
	for _, stream := range streams {
		if stream.closed || len(stream.buffer) <= maxStreamLogBufferBytes {
			continue
		}
		stream.emitBufferedChunk(maxStreamLogBufferBytes, sink)
		flushed = true
	}
	return flushed
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

func (stream *processLogStream) emitBufferedChunk(size int, sink *combinedLogSink) {
	if size > len(stream.buffer) {
		size = len(stream.buffer)
	}
	line := string(stream.buffer[:size])
	stream.buffer = stream.buffer[size:]
	stream.writeLine(strings.TrimSuffix(line, "\r"), sink)
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
	stream.file.writePlainLine(now, defaultLogTimestampFormat, line)
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
