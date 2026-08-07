package server

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dunstorm/pm2-go/internal/logstore"
	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/rs/zerolog"
)

func TestFlushProcessFlushesDaemonLogFiles(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "api-out.log")
	errPath := filepath.Join(dir, "api-err.log")
	combinedPath := logstore.CombinedPath(outPath)
	for _, path := range []string{outPath, errPath, combinedPath} {
		if err := os.WriteFile(path, []byte("before\n"), 0600); err != nil {
			t.Fatalf("write log %s: %v", path, err)
		}
	}

	logger := zerolog.New(io.Discard)
	handler := &Handler{
		logger: &logger,
		databaseById: map[int32]*pb.Process{
			1: {
				Id:          1,
				Name:        "api",
				LogFilePath: outPath,
				ErrFilePath: errPath,
			},
		},
	}

	response, err := handler.FlushProcess(context.Background(), &pb.FlushProcessRequest{Id: 1})
	if err != nil {
		t.Fatalf("flush process: %v", err)
	}
	if !response.GetSuccess() {
		t.Fatal("expected flush to succeed")
	}

	wantPaths := map[string]bool{
		outPath:      true,
		errPath:      true,
		combinedPath: true,
	}
	if len(response.GetLogFilePaths()) != len(wantPaths) {
		t.Fatalf("expected %d flushed paths, got %d: %v", len(wantPaths), len(response.GetLogFilePaths()), response.GetLogFilePaths())
	}
	seenPaths := make(map[string]bool, len(response.GetLogFilePaths()))
	for _, path := range response.GetLogFilePaths() {
		if !wantPaths[path] {
			t.Fatalf("unexpected flushed path %s", path)
		}
		if seenPaths[path] {
			t.Fatalf("duplicate flushed path %s", path)
		}
		seenPaths[path] = true

		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read flushed log %s: %v", path, err)
		}
		if len(contents) != 0 {
			t.Fatalf("expected flushed log %s to be empty, got %q", path, string(contents))
		}
		if generation := logstore.ReadCursorGeneration(path); generation == "" {
			t.Fatalf("expected cursor generation for %s to be bumped", path)
		}
	}
}
