package server

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

func TestHandleMaxLogGroupInitializesCountFromExistingArchives(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "api-out.log")
	errPath := filepath.Join(dir, "api-err.log")
	combinedPath := logstore.CombinedPath(outPath)
	if err := os.WriteFile(outPath, []byte("large enough"), 0640); err != nil {
		t.Fatalf("write log: %v", err)
	}
	for _, path := range []string{outPath + ".7", errPath + ".6", combinedPath + ".5"} {
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

	rotatedPaths := make(map[string]bool)
	previousRotateLogFile := rotateLogFile
	rotateLogFile = func(_, rotatedFilename string) (bool, error) {
		rotatedPaths[rotatedFilename] = true
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
		LogRotateMaxFiles: 20,
	})

	for _, path := range []string{outPath + ".8", errPath + ".8", combinedPath + ".8"} {
		if !rotatedPaths[path] {
			t.Fatalf("expected rotation to use archive path %s, got %#v", path, rotatedPaths)
		}
	}
	if process.LogFileCount != 9 {
		t.Fatalf("expected log file count to continue at 9, got %d", process.LogFileCount)
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

	if process.LogFileCount != 2 {
		t.Fatalf("expected log file count to advance, got %d", process.LogFileCount)
	}
	for _, path := range []string{outPath + ".0", errPath + ".0", combinedPath + ".0", outPath + ".1", errPath + ".1", combinedPath + ".1"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected invalid max-files pruning to remove %s, stat error: %v", path, err)
		}
	}
}

func TestRestartLiveProcessReleasesLockBeforeWaitingForExit(t *testing.T) {
	command := exec.Command("sleep", "10")
	if err := command.Start(); err != nil {
		t.Fatalf("start test process: %v", err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
	})

	logger := zerolog.New(io.Discard)
	process := &pb.Process{
		Id:             1,
		Name:           "live-restart",
		Pid:            int32(command.Process.Pid),
		ExecutablePath: "definitely-not-a-real-command",
		ProcStatus: &pb.ProcStatus{
			Status: "online",
			Cpu:    "2.0%",
			Memory: "8.0MB",
		},
	}
	handler := &Handler{
		logger:           &logger,
		databaseById:     map[int32]*pb.Process{process.Id: process},
		databaseByName:   map[string]*pb.Process{process.Name: process},
		processes:        map[int32]*os.Process{process.Id: command.Process},
		metricsUpdatedAt: make(map[int32]time.Time),
	}

	previousWaitForLiveRestartProcessExit := waitForLiveRestartProcessExit
	waitStarted := make(chan struct{})
	releaseWait := make(chan struct{})
	var waitStartedOnce sync.Once
	var releaseWaitOnce sync.Once
	waitForLiveRestartProcessExit = func(pid int32, timeout time.Duration) bool {
		waitStartedOnce.Do(func() {
			close(waitStarted)
		})
		<-releaseWait
		return true
	}
	t.Cleanup(func() {
		releaseWaitOnce.Do(func() {
			close(releaseWait)
		})
		waitForLiveRestartProcessExit = previousWaitForLiveRestartProcessExit
	})

	restartDone := make(chan struct{})
	go func() {
		handler.mu.Lock()
		restartLiveProcess(handler, process)
		handler.mu.Unlock()
		close(restartDone)
	}()

	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("expected live restart to enter exit wait")
	}

	listDone := make(chan error, 1)
	go func() {
		_, err := handler.ListProcess(context.Background(), &pb.ListProcessRequest{})
		listDone <- err
	}()

	select {
	case err := <-listDone:
		if err != nil {
			t.Fatalf("list process while live restart waits: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected list process not to block while live restart waits")
	}

	releaseWaitOnce.Do(func() {
		close(releaseWait)
	})
	select {
	case <-restartDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live restart to return")
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
