package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dunstorm/pm2-go/internal/logstore"
	pb "github.com/dunstorm/pm2-go/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

type fakeProcessSource struct {
	processes []*pb.Process
	err       error
	actionErr error
}

func (source fakeProcessSource) ListProcesses(ctx context.Context) ([]*pb.Process, error) {
	return source.processes, source.err
}

func (source fakeProcessSource) FindProcess(ctx context.Context, id int32) (*pb.Process, error) {
	if source.err != nil {
		return nil, source.err
	}
	for _, process := range source.processes {
		if process != nil && process.Id == id {
			return process, nil
		}
	}
	return nil, errors.New("not found")
}

func (source fakeProcessSource) StopProcess(ctx context.Context, id int32) (bool, error) {
	return source.actionResult()
}

func (source fakeProcessSource) RestartProcess(ctx context.Context, process *pb.Process, graceful bool) (*pb.Process, error) {
	if source.actionErr != nil {
		return nil, source.actionErr
	}
	return process, nil
}

func (source fakeProcessSource) DeleteProcess(ctx context.Context, id int32) (bool, error) {
	return source.actionResult()
}

func (source fakeProcessSource) actionResult() (bool, error) {
	if source.actionErr != nil {
		return false, source.actionErr
	}
	return true, nil
}

func TestAPIRequiresAuth(t *testing.T) {
	server := newTestServer(t, fakeProcessSource{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/processes", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}

func TestHTTPServerSetsTimeouts(t *testing.T) {
	server := newTestServer(t, fakeProcessSource{})
	httpServer := server.HTTPServer()

	if httpServer.Addr != server.Addr() {
		t.Fatalf("expected addr %q, got %q", server.Addr(), httpServer.Addr)
	}
	if httpServer.Handler == nil {
		t.Fatal("expected handler")
	}
	if httpServer.ReadHeaderTimeout <= 0 {
		t.Fatal("expected read header timeout")
	}
	if httpServer.ReadTimeout <= 0 {
		t.Fatal("expected read timeout")
	}
	if httpServer.WriteTimeout <= 0 {
		t.Fatal("expected write timeout")
	}
	if httpServer.IdleTimeout <= 0 {
		t.Fatal("expected idle timeout")
	}
}

func TestLoginSetsSessionAndListsProcesses(t *testing.T) {
	server := newTestServer(t, fakeProcessSource{processes: []*pb.Process{testProcess()}})

	login := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader("token=secret"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server.Handler().ServeHTTP(login, request)

	if login.Code != http.StatusSeeOther {
		t.Fatalf("expected login redirect, got %d", login.Code)
	}
	cookies := login.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}

	api := httptest.NewRecorder()
	apiRequest := httptest.NewRequest(http.MethodGet, "/api/processes", nil)
	apiRequest.AddCookie(cookies[0])
	server.Handler().ServeHTTP(api, apiRequest)

	if api.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", api.Code, api.Body.String())
	}
	if strings.Contains(api.Body.String(), "SHOULD_NOT_LEAK") {
		t.Fatal("expected process environment to be redacted")
	}

	var response struct {
		Processes []processView `json:"processes"`
	}
	if err := json.Unmarshal(api.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Processes) != 1 {
		t.Fatalf("expected one process, got %d", len(response.Processes))
	}
	if response.Processes[0].Name != "api" {
		t.Fatalf("expected process name api, got %q", response.Processes[0].Name)
	}
}

func TestLoginRejectsInvalidToken(t *testing.T) {
	server := newTestServer(t, fakeProcessSource{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader("token=wrong"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
	if len(recorder.Result().Cookies()) != 0 {
		t.Fatal("expected no session cookie")
	}
}

func TestSecureCookiesConfigForcesSecureCookies(t *testing.T) {
	server, err := NewServer(Config{
		Host:          DefaultHost,
		Port:          DefaultPort,
		Token:         "secret",
		SecureCookies: true,
	}, fakeProcessSource{})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	cookies := loginCookies(t, server)
	seen := make(map[string]bool, len(cookies))
	for _, cookie := range cookies {
		seen[cookie.Name] = true
		if !cookie.Secure {
			t.Fatalf("expected %s cookie to be Secure", cookie.Name)
		}
	}
	if !seen[sessionCookieName] || !seen[csrfCookieName] {
		t.Fatalf("expected session and csrf cookies, got %#v", cookies)
	}
}

func TestDaemonUnavailableReturnsServiceUnavailable(t *testing.T) {
	server := newTestServer(t, fakeProcessSource{err: errors.New("unavailable")})
	cookies := loginCookies(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/processes", nil)
	addCookies(request, cookies)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", recorder.Code)
	}
}

func TestActionRequiresCSRF(t *testing.T) {
	server := newTestServer(t, fakeProcessSource{processes: []*pb.Process{testProcess()}})
	cookies := loginCookies(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/processes/1/actions", strings.NewReader(`{"action":"stop"}`))
	request.Header.Set("Content-Type", "application/json")
	addCookies(request, cookies)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", recorder.Code)
	}
}

func TestActionWithCSRF(t *testing.T) {
	server := newTestServer(t, fakeProcessSource{processes: []*pb.Process{testProcess()}})
	cookies := loginCookies(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/processes/1/actions", strings.NewReader(`{"action":"stop"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrfFromCookies(t, cookies))
	addCookies(request, cookies)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"success":true`) {
		t.Fatalf("expected success response, got %s", recorder.Body.String())
	}
}

func TestReadOnlyRejectsAction(t *testing.T) {
	server, err := NewServer(Config{
		Host:     DefaultHost,
		Port:     DefaultPort,
		Token:    "secret",
		ReadOnly: true,
	}, fakeProcessSource{processes: []*pb.Process{testProcess()}})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	cookies := loginCookies(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/processes/1/actions", strings.NewReader(`{"action":"stop"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrfFromCookies(t, cookies))
	addCookies(request, cookies)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", recorder.Code)
	}
}

func TestProcessMetrics(t *testing.T) {
	server := newTestServer(t, fakeProcessSource{processes: []*pb.Process{testProcess()}})
	cookies := loginCookies(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/processes/1/metrics", nil)
	addCookies(request, cookies)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"cpu":1.5`) {
		t.Fatalf("expected CPU metric, got %s", recorder.Body.String())
	}
}

func TestProcessLogs(t *testing.T) {
	logFile := t.TempDir() + "/out.log"
	if err := os.WriteFile(logFile, []byte("one\ntwo\nthree\n"), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	process := testProcess()
	process.LogFilePath = logFile
	server := newTestServer(t, fakeProcessSource{processes: []*pb.Process{process}})
	cookies := loginCookies(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/processes/1/logs?stream=out&tail=2", nil)
	addCookies(request, cookies)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "one") || !strings.Contains(recorder.Body.String(), "three") {
		t.Fatalf("expected last two log lines, got %s", recorder.Body.String())
	}
}

func TestReadLogHonorsTailAfterBoundedInitialRead(t *testing.T) {
	logFile := t.TempDir() + "/large.log"
	var builder strings.Builder
	for i := 0; i < 400; i++ {
		builder.WriteString(strings.Repeat("x", 1024))
		builder.WriteString(" line-")
		builder.WriteString(strconv.Itoa(i))
		builder.WriteString("\n")
	}
	if err := os.WriteFile(logFile, []byte(builder.String()), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	logs, err := readLog(logFile, 0, "", 5)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(logs.Lines) != 5 {
		t.Fatalf("expected five lines, got %d", len(logs.Lines))
	}
	if !strings.Contains(logs.Lines[4], "line-399") {
		t.Fatalf("expected final line, got %#v", logs.Lines)
	}
}

func TestReadLogIncludesLargeFinalLineAfterBoundedInitialRead(t *testing.T) {
	logFile := t.TempDir() + "/large-final.log"
	largeLine := strings.Repeat("x", 300*1024)
	contents := "before\n" + largeLine + "\n"
	if err := os.WriteFile(logFile, []byte(contents), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	logs, err := readLog(logFile, 0, "", 1)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if logs.Offset != int64(len(contents)) {
		t.Fatalf("expected consumed offset %d, got %d", len(contents), logs.Offset)
	}
	if len(logs.Lines) != 1 || logs.Lines[0] != largeLine {
		t.Fatalf("expected large final line, got %d lines with final length %d", len(logs.Lines), len(strings.Join(logs.Lines, "")))
	}
}

func TestReadLogReturnsConsumedOffset(t *testing.T) {
	logFile := t.TempDir() + "/incremental.log"
	contents := "first\nsecond\n"
	if err := os.WriteFile(logFile, []byte(contents), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	logs, err := readLog(logFile, int64(len("first\n")), "", 10)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if logs.Offset != int64(len(contents)) {
		t.Fatalf("expected consumed offset %d, got %d", len(contents), logs.Offset)
	}
	if logs.Size != logs.Offset {
		t.Fatalf("expected size to match consumed offset, got size=%d offset=%d", logs.Size, logs.Offset)
	}
	if len(logs.Lines) != 1 || logs.Lines[0] != "second" {
		t.Fatalf("expected incremental line, got %#v", logs.Lines)
	}
}

func TestReadLogBoundsFarBehindIncrementalRead(t *testing.T) {
	logFile := t.TempDir() + "/incremental-large.log"
	prefix := "first\n"
	largeLine := strings.Repeat("x", maxIncrementalLogReadBytes+1024)
	contents := prefix + largeLine + "\nsecond\n"
	if err := os.WriteFile(logFile, []byte(contents), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	logs, err := readLog(logFile, int64(len(prefix)), "", 10)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	expectedOffset := int64(len(prefix) + len(largeLine) + 1)
	if logs.Offset != expectedOffset {
		t.Fatalf("expected bounded consumed offset %d, got %d", expectedOffset, logs.Offset)
	}
	if logs.Size != expectedOffset {
		t.Fatalf("expected bounded response size %d, got %d", expectedOffset, logs.Size)
	}
	if len(logs.Lines) != 1 || logs.Lines[0] != largeLine {
		t.Fatalf("expected oversized complete line, got %d lines with length %d", len(logs.Lines), len(strings.Join(logs.Lines, "")))
	}
}

func TestReadLogRetainsIncompleteOversizedIncrementalRecord(t *testing.T) {
	logFile := t.TempDir() + "/incremental-incomplete.log"
	prefix := "first\n"
	contents := prefix + strings.Repeat("x", maxIncrementalLogRecordBytes+1024)
	if err := os.WriteFile(logFile, []byte(contents), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	logs, err := readLog(logFile, int64(len(prefix)), "", 10)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if logs.Offset != int64(len(prefix)) {
		t.Fatalf("expected cursor to retain incomplete record at %d, got %d", len(prefix), logs.Offset)
	}
	if logs.Size != logs.Offset {
		t.Fatalf("expected size to match retained offset, got size=%d offset=%d", logs.Size, logs.Offset)
	}
	if len(logs.Lines) != 0 {
		t.Fatalf("expected no lines for incomplete oversized record, got %#v", logs.Lines)
	}
}

func TestReadLogConsumesJSONExpandedIncrementalRecord(t *testing.T) {
	logFile := t.TempDir() + "/incremental-json-expanded.log"
	prefix := "first\n"
	expandedLine := strings.Repeat(`\u003c`, maxIncrementalLogReadBytes+512)
	contents := prefix + expandedLine + "\nsecond\n"
	if err := os.WriteFile(logFile, []byte(contents), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	logs, err := readLog(logFile, int64(len(prefix)), "", 10)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	expectedOffset := int64(len(prefix) + len(expandedLine) + 1)
	if logs.Offset != expectedOffset {
		t.Fatalf("expected expanded record offset %d, got %d", expectedOffset, logs.Offset)
	}
	if len(logs.Lines) != 1 || logs.Lines[0] != expandedLine {
		t.Fatalf("expected expanded record, got %d lines with length %d", len(logs.Lines), len(strings.Join(logs.Lines, "")))
	}
}

func TestReadLogRetriesWhenCursorGenerationChangesDuringOpen(t *testing.T) {
	logFile := t.TempDir() + "/incremental-generation.log"
	prefix := "first\n"
	if err := os.WriteFile(logFile, []byte(prefix+"second\n"), 0600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	previousReadLogCursorGeneration := readLogCursorGeneration
	calls := 0
	readLogCursorGeneration = func(string) string {
		calls++
		if calls == 1 {
			return "old"
		}
		return "new"
	}
	t.Cleanup(func() {
		readLogCursorGeneration = previousReadLogCursorGeneration
	})

	logs, err := readLog(logFile, int64(len(prefix)), "", 10)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if calls < 4 {
		t.Fatalf("expected generation change retry, got %d generation reads", calls)
	}
	if logs.FileID != "new" && !strings.HasSuffix(logs.FileID, ":new") {
		t.Fatalf("expected stable new generation file id, got %q", logs.FileID)
	}
	if len(logs.Lines) != 1 || logs.Lines[0] != "second" {
		t.Fatalf("expected incremental line after retry, got %#v", logs.Lines)
	}
}

func TestReadLogResetsOffsetWhenFileChanges(t *testing.T) {
	dir := t.TempDir()
	logFile := dir + "/rotated.log"
	if err := os.WriteFile(logFile, []byte("first\n"), 0600); err != nil {
		t.Fatalf("write original log: %v", err)
	}

	first, err := readLog(logFile, 0, "", 10)
	if err != nil {
		t.Fatalf("read original log: %v", err)
	}
	if first.FileID == "" {
		t.Fatal("expected file id")
	}
	if err := os.Rename(logFile, logFile+".1"); err != nil {
		t.Fatalf("rotate log: %v", err)
	}
	if err := os.WriteFile(logFile, []byte("second\n"), 0600); err != nil {
		t.Fatalf("write replacement log: %v", err)
	}

	logs, err := readLog(logFile, first.Offset, first.FileID, 10)
	if err != nil {
		t.Fatalf("read replacement log: %v", err)
	}
	if logs.FileID == first.FileID {
		t.Fatal("expected replacement log to have a different file id")
	}
	if logs.Offset != int64(len("second\n")) {
		t.Fatalf("expected replacement offset, got %d", logs.Offset)
	}
	if len(logs.Lines) != 1 || logs.Lines[0] != "second" {
		t.Fatalf("expected replacement line, got %#v", logs.Lines)
	}
}

func TestReadLogDrainsOpenedDescriptorAfterRotation(t *testing.T) {
	dir := t.TempDir()
	logFile := dir + "/drain-rotated.log"
	if err := os.WriteFile(logFile, []byte("first\n"), 0600); err != nil {
		t.Fatalf("write original log: %v", err)
	}

	first, err := readLog(logFile, 0, "", 10)
	if err != nil {
		t.Fatalf("read original log: %v", err)
	}

	previousAfterStableLogSnapshot := afterStableLogSnapshot
	rotated := false
	afterStableLogSnapshot = func(path string) {
		if rotated || path != logFile {
			return
		}
		rotated = true
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			t.Fatalf("open original for append: %v", err)
		}
		if _, err := file.WriteString("second\n"); err != nil {
			_ = file.Close()
			t.Fatalf("append original log: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close original append: %v", err)
		}
		if err := os.Rename(path, path+".1"); err != nil {
			t.Fatalf("rotate log: %v", err)
		}
		if err := os.WriteFile(path, []byte("third\n"), 0600); err != nil {
			t.Fatalf("write replacement log: %v", err)
		}
	}
	t.Cleanup(func() {
		afterStableLogSnapshot = previousAfterStableLogSnapshot
	})

	logs, err := readLog(logFile, first.Offset, first.FileID, 10)
	if err != nil {
		t.Fatalf("read rotated descriptor: %v", err)
	}
	if !rotated {
		t.Fatal("expected rotation hook to run")
	}
	if logs.FileID != first.FileID {
		t.Fatalf("expected old file cursor before replacement poll, got %q want %q", logs.FileID, first.FileID)
	}
	if logs.Offset != int64(len("first\nsecond\n")) {
		t.Fatalf("expected drained offset, got %d", logs.Offset)
	}
	if strings.Join(logs.Lines, ",") != "second" {
		t.Fatalf("expected drained line from rotated descriptor, got %#v", logs.Lines)
	}

	next, err := readLog(logFile, logs.Offset, logs.FileID, 10)
	if err != nil {
		t.Fatalf("read replacement log: %v", err)
	}
	if next.FileID == logs.FileID {
		t.Fatal("expected next poll to switch to replacement file")
	}
	if strings.Join(next.Lines, ",") != "third" {
		t.Fatalf("expected replacement line, got %#v", next.Lines)
	}
}

func TestReadLogResetsOffsetWhenCursorGenerationChanges(t *testing.T) {
	dir := t.TempDir()
	logFile := dir + "/flushed.log"
	if err := os.WriteFile(logFile, []byte("first\n"), 0600); err != nil {
		t.Fatalf("write original log: %v", err)
	}

	first, err := readLog(logFile, 0, "", 10)
	if err != nil {
		t.Fatalf("read original log: %v", err)
	}
	if err := os.Truncate(logFile, 0); err != nil {
		t.Fatalf("truncate log: %v", err)
	}
	if err := logstore.BumpCursorGeneration(logFile); err != nil {
		t.Fatalf("bump cursor generation: %v", err)
	}
	if err := os.WriteFile(logFile, []byte("second\nthird\n"), 0600); err != nil {
		t.Fatalf("write regrown log: %v", err)
	}

	logs, err := readLog(logFile, first.Offset, first.FileID, 10)
	if err != nil {
		t.Fatalf("read regenerated log: %v", err)
	}
	if logs.FileID == first.FileID {
		t.Fatal("expected cursor generation change to alter file id")
	}
	if logs.Offset != int64(len("second\nthird\n")) {
		t.Fatalf("expected regenerated offset, got %d", logs.Offset)
	}
	if strings.Join(logs.Lines, ",") != "second,third" {
		t.Fatalf("expected regenerated lines, got %#v", logs.Lines)
	}
}

func TestProcessCombinedLogs(t *testing.T) {
	dir := t.TempDir()
	logFile := dir + "/api-out.log"
	combinedLogFile := logstore.CombinedPath(logFile)
	contents := strings.Join([]string{
		`{"timestamp":"2026-08-07T10:00:00Z","stream":"stdout","line":"ready"}`,
		`{"timestamp":"2026-08-07T10:00:01Z","stream":"stderr","line":"warning"}`,
		`{"timestamp":"2026-08-07T10:00:02Z","stream":"stdout","line":"done"}`,
		"",
	}, "\n")
	if err := os.WriteFile(combinedLogFile, []byte(contents), 0600); err != nil {
		t.Fatalf("write combined log: %v", err)
	}

	process := testProcess()
	process.LogFilePath = logFile
	server := newTestServer(t, fakeProcessSource{processes: []*pb.Process{process}})
	cookies := loginCookies(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/processes/1/logs?stream=both&tail=3", nil)
	addCookies(request, cookies)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	body := recorder.Body.String()
	stdoutFirst := strings.Index(body, "[stdout]")
	stderrSecond := strings.Index(body, "[stderr]")
	stdoutThird := strings.LastIndex(body, "[stdout]")
	if stdoutFirst < 0 || stderrSecond < 0 || stdoutThird <= stdoutFirst {
		t.Fatalf("expected formatted combined streams, got %s", body)
	}
	if !(stdoutFirst < stderrSecond && stderrSecond < stdoutThird) {
		t.Fatalf("expected file order to be preserved, got %s", body)
	}
}

func TestProcessCombinedLogsBackfillsLegacyHistory(t *testing.T) {
	dir := t.TempDir()
	outLogFile := dir + "/api-out.log"
	errLogFile := dir + "/api-err.log"
	combinedLogFile := logstore.CombinedPath(outLogFile)
	if err := os.WriteFile(outLogFile, []byte("2026-08-07 10:00:00: legacy-out\n"), 0600); err != nil {
		t.Fatalf("write stdout log: %v", err)
	}
	if err := os.WriteFile(errLogFile, []byte("2026-08-07 10:00:01: legacy-err\n"), 0600); err != nil {
		t.Fatalf("write stderr log: %v", err)
	}
	contents := strings.Join([]string{
		`{"timestamp":"2026-08-07T10:00:02Z","stream":"stdout","line":"combined-new"}`,
		"",
	}, "\n")
	if err := os.WriteFile(combinedLogFile, []byte(contents), 0600); err != nil {
		t.Fatalf("write combined log: %v", err)
	}

	process := testProcess()
	process.LogFilePath = outLogFile
	process.ErrFilePath = errLogFile
	server := newTestServer(t, fakeProcessSource{processes: []*pb.Process{process}})
	cookies := loginCookies(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/processes/1/logs?stream=both&tail=3", nil)
	addCookies(request, cookies)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	body := recorder.Body.String()
	legacyOut := strings.Index(body, "legacy-out")
	legacyErr := strings.Index(body, "legacy-err")
	combined := strings.Index(body, "combined-new")
	if legacyOut < 0 || legacyErr < 0 || combined < 0 {
		t.Fatalf("expected legacy and combined log history, got %s", body)
	}
	if !(legacyOut < legacyErr && legacyErr < combined) {
		t.Fatalf("expected merged log history ordered by timestamp, got %s", body)
	}
}

func TestProcessCombinedLogsDoesNotRepeatLegacyBackfillForEmptyCombinedCursor(t *testing.T) {
	dir := t.TempDir()
	outLogFile := dir + "/api-out.log"
	errLogFile := dir + "/api-err.log"
	combinedLogFile := logstore.CombinedPath(outLogFile)
	if err := os.WriteFile(outLogFile, []byte("2026-08-07 10:00:00: legacy-out\n"), 0600); err != nil {
		t.Fatalf("write stdout log: %v", err)
	}
	if err := os.WriteFile(errLogFile, []byte("2026-08-07 10:00:01: legacy-err\n"), 0600); err != nil {
		t.Fatalf("write stderr log: %v", err)
	}
	if err := os.WriteFile(combinedLogFile, nil, 0600); err != nil {
		t.Fatalf("write empty combined log: %v", err)
	}

	process := testProcess()
	process.LogFilePath = outLogFile
	process.ErrFilePath = errLogFile
	server := newTestServer(t, fakeProcessSource{processes: []*pb.Process{process}})
	cookies := loginCookies(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/processes/1/logs?stream=both&tail=2", nil)
	addCookies(request, cookies)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var first logResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if first.Offset != 0 || first.FileID == "" {
		t.Fatalf("expected empty combined cursor with file id, got offset=%d fileID=%q", first.Offset, first.FileID)
	}
	if len(first.Lines) != 2 {
		t.Fatalf("expected legacy backfill lines, got %#v", first.Lines)
	}

	recorder = httptest.NewRecorder()
	query := url.Values{
		"stream": {"both"},
		"tail":   {"2"},
		"offset": {strconv.FormatInt(first.Offset, 10)},
		"fileId": {first.FileID},
	}
	request = httptest.NewRequest(http.MethodGet, "/api/processes/1/logs?"+query.Encode(), nil)
	addCookies(request, cookies)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var second logResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if len(second.Lines) != 0 {
		t.Fatalf("expected no repeated legacy backfill lines, got %#v", second.Lines)
	}
}

func TestReadCombinedLogUsesKnownZeroOffsetAsIncrementalCursor(t *testing.T) {
	dir := t.TempDir()
	combinedLogFile := logstore.CombinedPath(dir + "/api-out.log")
	if err := os.WriteFile(combinedLogFile, nil, 0600); err != nil {
		t.Fatalf("write empty combined log: %v", err)
	}

	first, err := readCombinedLog(combinedLogFile, 0, "", 2)
	if err != nil {
		t.Fatalf("read empty combined log: %v", err)
	}
	if first.Offset != 0 || first.FileID == "" {
		t.Fatalf("expected empty combined cursor with file id, got offset=%d fileID=%q", first.Offset, first.FileID)
	}

	records := []string{
		`{"timestamp":"2026-08-07T10:00:00Z","stream":"stdout","line":"one"}`,
		`{"timestamp":"2026-08-07T10:00:01Z","stream":"stdout","line":"two"}`,
		`{"timestamp":"2026-08-07T10:00:02Z","stream":"stdout","line":"three"}`,
	}
	if err := os.WriteFile(combinedLogFile, []byte(strings.Join(records, "\n")+"\n"), 0600); err != nil {
		t.Fatalf("append combined records: %v", err)
	}

	second, err := readCombinedLog(combinedLogFile, first.Offset, first.FileID, 2)
	if err != nil {
		t.Fatalf("read combined log from known zero cursor: %v", err)
	}
	if len(second.Lines) != 3 {
		t.Fatalf("expected all records after known zero cursor, got %#v", second.Lines)
	}
	got := strings.Join(second.Lines, "\n")
	for _, line := range []string{"one", "two", "three"} {
		if !strings.Contains(got, line) {
			t.Fatalf("expected incremental lines to contain %q, got %#v", line, second.Lines)
		}
	}
}

func TestReadCombinedLogBackfillsMalformedInitialTail(t *testing.T) {
	dir := t.TempDir()
	logFile := dir + "/api-out.log"
	combinedLogFile := logstore.CombinedPath(logFile)
	contents := strings.Join([]string{
		`{"timestamp":"2026-08-07T10:00:00Z","stream":"stdout","line":"ready"}`,
		`{"timestamp":"2026-08-07T10:00:01Z","stream":"stderr","line":"warning"`,
		"",
	}, "\n")
	if err := os.WriteFile(combinedLogFile, []byte(contents), 0600); err != nil {
		t.Fatalf("write combined log: %v", err)
	}

	logs, err := readCombinedLog(combinedLogFile, 0, "", 1)
	if err != nil {
		t.Fatalf("read combined log: %v", err)
	}
	if len(logs.Lines) != 1 || !strings.Contains(logs.Lines[0], "ready") {
		t.Fatalf("expected older valid entry to backfill malformed tail, got %#v", logs.Lines)
	}
}

func TestNonLoopbackHostRequiresAllowRemote(t *testing.T) {
	_, err := NewServer(Config{
		Host:  "0.0.0.0",
		Port:  DefaultPort,
		Token: "secret",
	}, fakeProcessSource{})
	if err == nil {
		t.Fatal("expected non-loopback host to be rejected")
	}
}

func TestAllowRemoteAcceptsNonLoopbackHost(t *testing.T) {
	server, err := NewServer(Config{
		Host:        "0.0.0.0",
		Port:        DefaultPort,
		Token:       "secret",
		AllowRemote: true,
	}, fakeProcessSource{})
	if err != nil {
		t.Fatalf("expected remote host with allow-remote to be accepted: %v", err)
	}
	if server.Addr() != "0.0.0.0:9615" {
		t.Fatalf("unexpected addr %q", server.Addr())
	}
}

func TestAddrFormatsIPv6LoopbackHost(t *testing.T) {
	for _, host := range []string{"::1", "[::1]"} {
		server, err := NewServer(Config{
			Host:  host,
			Port:  DefaultPort,
			Token: "secret",
		}, fakeProcessSource{})
		if err != nil {
			t.Fatalf("expected IPv6 loopback host %q to be accepted: %v", host, err)
		}
		if server.Addr() != "[::1]:9615" {
			t.Fatalf("unexpected addr for host %q: %q", host, server.Addr())
		}
	}
}

func TestGeneratedToken(t *testing.T) {
	server, err := NewServer(Config{Port: DefaultPort}, fakeProcessSource{})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	if !server.TokenGenerated() {
		t.Fatal("expected token to be generated")
	}
	if server.Token() == "" {
		t.Fatal("expected generated token value")
	}
}

func TestDevAssetsInjectReloadScript(t *testing.T) {
	assetDir := writeTestAssets(t, "<!doctype html><html><body>login</body></html>")
	server, err := NewServer(Config{
		Host:      DefaultHost,
		Port:      DefaultPort,
		Token:     "secret",
		AssetDir:  assetDir,
		DevReload: true,
	}, fakeProcessSource{})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), devReloadScriptPath) {
		t.Fatalf("expected reload script to be injected, got %s", recorder.Body.String())
	}

	script := httptest.NewRecorder()
	scriptRequest := httptest.NewRequest(http.MethodGet, devReloadScriptPath, nil)
	server.Handler().ServeHTTP(script, scriptRequest)
	if script.Code != http.StatusOK {
		t.Fatalf("expected reload script response, got %d", script.Code)
	}
	if !strings.Contains(script.Body.String(), "/dev/reload/version") {
		t.Fatalf("expected reload script endpoint, got %s", script.Body.String())
	}
}

func TestDevReloadVersionChangesWhenAssetChanges(t *testing.T) {
	assetDir := writeTestAssets(t, "<!doctype html><html><body>login</body></html>")
	server, err := NewServer(Config{
		Host:      DefaultHost,
		Port:      DefaultPort,
		Token:     "secret",
		AssetDir:  assetDir,
		DevReload: true,
	}, fakeProcessSource{})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	first := devReloadVersion(t, server)
	if err := os.WriteFile(assetDir+"/assets/app.css", []byte("body { color: red; }\n"), 0600); err != nil {
		t.Fatalf("write css: %v", err)
	}
	second := devReloadVersion(t, server)
	if first == second {
		t.Fatalf("expected version to change after asset edit")
	}
}

func newTestServer(t *testing.T, source fakeProcessSource) *Server {
	t.Helper()
	server, err := NewServer(Config{
		Host:  DefaultHost,
		Port:  DefaultPort,
		Token: "secret",
	}, source)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	return server
}

func writeTestAssets(t *testing.T, loginHTML string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(root+"/assets", 0700); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	files := map[string]string{
		"assets/index.html": "<!doctype html><html><body>index</body></html>",
		"assets/login.html": loginHTML,
		"assets/app.css":    "body { color: black; }\n",
		"assets/app.js":     "console.log('dev');\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(root+"/"+name, []byte(contents), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

func devReloadVersion(t *testing.T, server *Server) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/dev/reload/version", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	return strings.TrimSpace(recorder.Body.String())
}

func loginCookies(t *testing.T, server *Server) []*http.Cookie {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader("token=secret"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("login failed: %d", recorder.Code)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}
	return cookies
}

func addCookies(request *http.Request, cookies []*http.Cookie) {
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
}

func csrfFromCookies(t *testing.T, cookies []*http.Cookie) string {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == csrfCookieName {
			return cookie.Value
		}
	}
	t.Fatal("expected csrf cookie")
	return ""
}

func testProcess() *pb.Process {
	return &pb.Process{
		Id:             1,
		Name:           "api",
		Pid:            123,
		ExecutablePath: "/usr/bin/python3",
		Args:           []string{"app.py"},
		Cwd:            "/srv/api",
		AutoRestart:    true,
		Env: map[string]string{
			"SECRET": "SHOULD_NOT_LEAK",
		},
		HealthCheckUrl: "http://localhost:3000/health",
		Watch:          true,
		ProcStatus: &pb.ProcStatus{
			Status:    "online",
			Restarts:  2,
			Cpu:       "1.5%",
			Memory:    "42.0MB",
			Uptime:    durationpb.New(3 * time.Minute),
			ParentPid: 44,
		},
	}
}
