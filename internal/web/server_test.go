package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pb "github.com/dunstorm/pm2-go/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

type fakeProcessSource struct {
	processes []*pb.Process
	err       error
}

func (source fakeProcessSource) ListProcesses(ctx context.Context) ([]*pb.Process, error) {
	return source.processes, source.err
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

func TestDaemonUnavailableReturnsServiceUnavailable(t *testing.T) {
	server := newTestServer(t, fakeProcessSource{err: errors.New("unavailable")})
	cookie := loginCookie(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/processes", nil)
	request.AddCookie(cookie)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", recorder.Code)
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

func loginCookie(t *testing.T, server *Server) *http.Cookie {
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
	return cookies[0]
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
