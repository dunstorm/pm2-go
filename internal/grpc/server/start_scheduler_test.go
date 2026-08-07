package server

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dunstorm/pm2-go/internal/logstore"
	"github.com/dunstorm/pm2-go/internal/utils"
	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/rs/zerolog"
)

func TestBuildLogRotationGroupsDeduplicatesSharedPaths(t *testing.T) {
	dir := t.TempDir()
	sharedOut := filepath.Join(dir, "api-out.log")
	sharedErr := filepath.Join(dir, "api-err.log")
	workerOut := filepath.Join(dir, "worker-out.log")
	workerErr := filepath.Join(dir, "worker-err.log")

	groups := buildLogRotationGroups([]*pb.Process{
		{Id: 1, Name: "API", LogFilePath: sharedOut, ErrFilePath: sharedErr},
		{Id: 2, Name: "api", LogFilePath: sharedOut, ErrFilePath: sharedErr},
		{Id: 3, Name: "worker", LogFilePath: workerOut, ErrFilePath: workerErr},
	})

	if len(groups) != 2 {
		t.Fatalf("expected two log rotation groups, got %d", len(groups))
	}

	sharedGroup := findLogRotationGroup(t, groups, sharedOut, sharedErr)
	if sharedGroup.combinedLogPath != logstore.CombinedPath(sharedOut) {
		t.Fatalf("expected shared combined log path %q, got %q", logstore.CombinedPath(sharedOut), sharedGroup.combinedLogPath)
	}
	if len(sharedGroup.processes) != 2 {
		t.Fatalf("expected two processes in shared group, got %d", len(sharedGroup.processes))
	}

	workerGroup := findLogRotationGroup(t, groups, workerOut, workerErr)
	if len(workerGroup.processes) != 1 {
		t.Fatalf("expected one process in worker group, got %d", len(workerGroup.processes))
	}
}

func TestMaxLogFileCountUsesHighestSharedCount(t *testing.T) {
	got := maxLogFileCount([]*pb.Process{
		{LogFileCount: 2},
		{LogFileCount: 7},
		{LogFileCount: 4},
	})
	if got != 7 {
		t.Fatalf("expected max log file count 7, got %d", got)
	}
}

func TestHandleMaxLogGroupAdvancesCountWhenRotationRecreateFails(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "api-out.log")
	errPath := filepath.Join(dir, "api-err.log")
	combinedPath := logstore.CombinedPath(outPath)
	if err := os.WriteFile(outPath, []byte("large enough"), 0640); err != nil {
		t.Fatalf("write log: %v", err)
	}

	process := &pb.Process{
		Id:           1,
		LogFilePath:  outPath,
		ErrFilePath:  errPath,
		LogFileCount: 4,
	}
	logger := zerolog.New(io.Discard)
	handler := &Handler{
		logger:       &logger,
		databaseById: map[int32]*pb.Process{process.Id: process},
	}

	previousRotateLogFile := rotateLogFile
	rotateLogFile = func(filename, rotatedFilename string) (bool, error) {
		if filename == outPath && rotatedFilename == outPath+".4" {
			return true, errors.New("recreate failed")
		}
		return false, nil
	}
	t.Cleanup(func() {
		rotateLogFile = previousRotateLogFile
	})

	handleMaxLogGroup(handler, &logRotationGroup{
		logFilePath:     outPath,
		errFilePath:     errPath,
		combinedLogPath: combinedPath,
		processes:       []*pb.Process{process},
	}, utils.Config{
		LogRotate:         true,
		LogRotateSize:     1,
		LogRotateMaxFiles: 10,
	})

	if process.LogFileCount != 5 {
		t.Fatalf("expected log file count to advance after partial rotation, got %d", process.LogFileCount)
	}
}

func TestHandleMaxLogGroupKeepsCountMonotonicAfterPrune(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "api-out.log")
	errPath := filepath.Join(dir, "api-err.log")
	combinedPath := logstore.CombinedPath(outPath)
	if err := os.WriteFile(outPath, []byte("large enough"), 0640); err != nil {
		t.Fatalf("write log: %v", err)
	}

	process := &pb.Process{
		Id:           1,
		LogFilePath:  outPath,
		ErrFilePath:  errPath,
		LogFileCount: 3,
	}
	logger := zerolog.New(io.Discard)
	handler := &Handler{
		logger:       &logger,
		databaseById: map[int32]*pb.Process{process.Id: process},
	}

	previousRotateLogFile := rotateLogFile
	rotateLogFile = func(string, string) (bool, error) {
		return true, nil
	}
	t.Cleanup(func() {
		rotateLogFile = previousRotateLogFile
	})

	handleMaxLogGroup(handler, &logRotationGroup{
		logFilePath:     outPath,
		errFilePath:     errPath,
		combinedLogPath: combinedPath,
		processes:       []*pb.Process{process},
	}, utils.Config{
		LogRotate:         true,
		LogRotateSize:     1,
		LogRotateMaxFiles: 3,
	})

	if process.LogFileCount != 4 {
		t.Fatalf("expected monotonic log file count 4 after prune, got %d", process.LogFileCount)
	}
}

func TestHandleMaxLogGroupAdvancesReplacementWithSameLogPaths(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "api-out.log")
	errPath := filepath.Join(dir, "api-err.log")
	combinedPath := logstore.CombinedPath(outPath)
	if err := os.WriteFile(outPath, []byte("large enough"), 0640); err != nil {
		t.Fatalf("write log: %v", err)
	}

	staleProcess := &pb.Process{
		Id:           1,
		LogFilePath:  outPath,
		ErrFilePath:  errPath,
		LogFileCount: 3,
	}
	replacement := &pb.Process{
		Id:           staleProcess.Id,
		LogFilePath:  outPath,
		ErrFilePath:  errPath,
		LogFileCount: staleProcess.LogFileCount,
	}
	logger := zerolog.New(io.Discard)
	handler := &Handler{
		logger:       &logger,
		databaseById: map[int32]*pb.Process{replacement.Id: replacement},
	}

	previousRotateLogFile := rotateLogFile
	rotateLogFile = func(string, string) (bool, error) {
		return true, nil
	}
	t.Cleanup(func() {
		rotateLogFile = previousRotateLogFile
	})

	handleMaxLogGroup(handler, &logRotationGroup{
		logFilePath:     outPath,
		errFilePath:     errPath,
		combinedLogPath: combinedPath,
		processes:       []*pb.Process{staleProcess},
	}, utils.Config{
		LogRotate:         true,
		LogRotateSize:     1,
		LogRotateMaxFiles: 10,
	})

	if replacement.LogFileCount != 4 {
		t.Fatalf("expected replacement log file count to advance, got %d", replacement.LogFileCount)
	}
}

func TestHandleMaxLogGroupPrunesArchivesAfterPartialFailure(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "api-out.log")
	errPath := filepath.Join(dir, "api-err.log")
	combinedPath := logstore.CombinedPath(outPath)
	if err := os.WriteFile(outPath, []byte("large enough"), 0640); err != nil {
		t.Fatalf("write log: %v", err)
	}
	for _, path := range []string{outPath + ".1", errPath + ".1", combinedPath + ".1"} {
		if err := os.WriteFile(path, []byte("archive"), 0640); err != nil {
			t.Fatalf("write archive %s: %v", path, err)
		}
	}

	process := &pb.Process{
		Id:           1,
		LogFilePath:  outPath,
		ErrFilePath:  errPath,
		LogFileCount: 3,
	}
	logger := zerolog.New(io.Discard)
	handler := &Handler{
		logger:       &logger,
		databaseById: map[int32]*pb.Process{process.Id: process},
	}

	previousRotateLogFile := rotateLogFile
	rotateLogFile = func(filename, rotatedFilename string) (bool, error) {
		switch filename {
		case outPath, combinedPath:
			return true, nil
		case errPath:
			return false, errors.New("rotate failed")
		default:
			return false, nil
		}
	}
	t.Cleanup(func() {
		rotateLogFile = previousRotateLogFile
	})

	handleMaxLogGroup(handler, &logRotationGroup{
		logFilePath:     outPath,
		errFilePath:     errPath,
		combinedLogPath: combinedPath,
		processes:       []*pb.Process{process},
	}, utils.Config{
		LogRotate:         true,
		LogRotateSize:     1,
		LogRotateMaxFiles: 3,
	})

	if _, err := os.Stat(outPath + ".1"); !os.IsNotExist(err) {
		t.Fatalf("expected successful stdout archive to be pruned, stat error: %v", err)
	}
	if _, err := os.Stat(combinedPath + ".1"); !os.IsNotExist(err) {
		t.Fatalf("expected successful combined archive to be pruned, stat error: %v", err)
	}
	if _, err := os.Stat(errPath + ".1"); !os.IsNotExist(err) {
		t.Fatalf("expected failed stderr archive to be pruned, stat error: %v", err)
	}
}

func TestHandleMaxLogGroupPrunesCreatedArchiveWhenMaxFilesInvalid(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "api-out.log")
	errPath := filepath.Join(dir, "api-err.log")
	combinedPath := logstore.CombinedPath(outPath)
	if err := os.WriteFile(outPath, []byte("large enough"), 0640); err != nil {
		t.Fatalf("write log: %v", err)
	}
	for _, path := range []string{outPath + ".0", errPath + ".0", combinedPath + ".0"} {
		if err := os.WriteFile(path, []byte("archive"), 0640); err != nil {
			t.Fatalf("write archive %s: %v", path, err)
		}
	}

	process := &pb.Process{
		Id:          1,
		LogFilePath: outPath,
		ErrFilePath: errPath,
	}
	logger := zerolog.New(io.Discard)
	handler := &Handler{
		logger:       &logger,
		databaseById: map[int32]*pb.Process{process.Id: process},
	}

	previousRotateLogFile := rotateLogFile
	rotateLogFile = func(string, string) (bool, error) {
		return true, nil
	}
	t.Cleanup(func() {
		rotateLogFile = previousRotateLogFile
	})

	handleMaxLogGroup(handler, &logRotationGroup{
		logFilePath:     outPath,
		errFilePath:     errPath,
		combinedLogPath: combinedPath,
		processes:       []*pb.Process{process},
	}, utils.Config{
		LogRotate:         true,
		LogRotateSize:     1,
		LogRotateMaxFiles: 0,
	})

	if process.LogFileCount != 1 {
		t.Fatalf("expected log file count to advance, got %d", process.LogFileCount)
	}
	for _, path := range []string{outPath + ".0", errPath + ".0", combinedPath + ".0"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected invalid max-files pruning to remove %s, stat error: %v", path, err)
		}
	}
}

func findLogRotationGroup(t *testing.T, groups []*logRotationGroup, logFilePath, errFilePath string) *logRotationGroup {
	t.Helper()

	for _, group := range groups {
		if group.logFilePath == logFilePath && group.errFilePath == errFilePath {
			return group
		}
	}
	t.Fatalf("missing log rotation group for %s and %s", logFilePath, errFilePath)
	return nil
}
