package server

import (
	"path/filepath"
	"testing"

	"github.com/dunstorm/pm2-go/internal/logstore"
	pb "github.com/dunstorm/pm2-go/proto"
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
