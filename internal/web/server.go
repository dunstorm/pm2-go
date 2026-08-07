package web

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dunstorm/pm2-go/internal/logstore"
	pb "github.com/dunstorm/pm2-go/proto"
)

const sessionCookieName = "pm2_go_web_session"
const csrfCookieName = "pm2_go_web_csrf"
const devReloadScriptPath = "/assets/dev-reload.js"

//go:embed assets/*
var embeddedAssets embed.FS

type Server struct {
	config   runtimeConfig
	source   ProcessSource
	sessions *sessionStore
	metrics  *metricsStore
	events   *eventStore
	assets   fs.FS
}

func NewServer(config Config, source ProcessSource) (*Server, error) {
	runtime, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	if source == nil {
		source = NewGRPCProcessSource(DefaultDaemonPort)
	}

	assets := fs.FS(embeddedAssets)
	if runtime.AssetDir != "" {
		assets = os.DirFS(runtime.AssetDir)
		if _, err := fs.Stat(assets, "assets/index.html"); err != nil {
			return nil, fmt.Errorf("invalid web asset dir %q: %w", runtime.AssetDir, err)
		}
	}

	events := newEventStore()
	return &Server{
		config:   runtime,
		source:   source,
		sessions: newSessionStore(runtime.SessionTTL),
		metrics:  newMetricsStore(events),
		events:   events,
		assets:   assets,
	}, nil
}

func (server *Server) Addr() string {
	return fmt.Sprintf("%s:%d", server.config.Host, server.config.Port)
}

func (server *Server) Token() string {
	return server.config.Token
}

func (server *Server) TokenGenerated() bool {
	return server.config.tokenGenerated
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	if server.config.DevReload {
		mux.HandleFunc(devReloadScriptPath, server.handleDevReloadScript)
		mux.HandleFunc("/dev/reload/version", server.handleDevReloadVersion)
	}
	mux.Handle("/assets/", http.FileServer(http.FS(server.assets)))
	mux.HandleFunc("/login", server.handleLoginPage)
	mux.HandleFunc("/auth/login", server.handleLogin)
	mux.HandleFunc("/auth/logout", server.requireAuth(server.handleLogout))
	mux.HandleFunc("/api/session", server.requireAuth(server.handleSession))
	mux.HandleFunc("/api/events", server.requireAuth(server.handleEvents))
	mux.HandleFunc("/api/processes/", server.requireAuth(server.handleProcessRoute))
	mux.HandleFunc("/api/processes", server.requireAuth(server.handleProcesses))
	mux.HandleFunc("/", server.requireAuth(server.handleIndex))
	return securityHeaders(mux)
}

func (server *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	server.serveAsset(w, r, "assets/index.html")
}

func (server *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if server.isAuthenticated(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	server.serveAsset(w, r, "assets/login.html")
}

func (server *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	if !server.validToken(r.FormValue("token")) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	sessionID, csrfToken, expiresAt, err := server.sessions.create()
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    csrfToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: false,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (server *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		server.sessions.delete(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (server *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"allow_remote":        server.config.AllowRemote,
		"dev_assets":          server.config.AssetDir != "",
		"dev_reload":          server.config.DevReload,
		"host":                server.config.Host,
		"port":                server.config.Port,
		"read_only":           server.config.ReadOnly,
		"session_ttl_seconds": int(server.config.SessionTTL.Seconds()),
		"token_generated":     server.config.tokenGenerated,
	})
}

func (server *Server) handleProcesses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	processes, err := server.source.ListProcesses(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "pm2-go daemon is unavailable")
		return
	}
	server.metrics.observeSnapshot(processes)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"processes": processViews(processes),
		"events":    server.events.list(),
	})
}

func (server *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"events": server.events.list(),
	})
}

func (server *Server) handleProcessRoute(w http.ResponseWriter, r *http.Request) {
	id, suffix, ok := processRoute(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch suffix {
	case "":
		server.handleProcessDetail(w, r, id)
	case "metrics":
		server.handleProcessMetrics(w, r, id)
	case "logs":
		server.handleProcessLogs(w, r, id)
	case "actions":
		server.handleProcessAction(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (server *Server) handleProcessDetail(w http.ResponseWriter, r *http.Request, id int32) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	process, err := server.source.FindProcess(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "process not found")
		return
	}
	server.metrics.observePartial([]*pb.Process{process})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"process": newProcessView(process),
		"metrics": server.metrics.history(id),
	})
}

func (server *Server) handleProcessMetrics(w http.ResponseWriter, r *http.Request, id int32) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if process, err := server.source.FindProcess(r.Context(), id); err == nil {
		server.metrics.observePartial([]*pb.Process{process})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"points": server.metrics.history(id),
	})
}

func (server *Server) handleProcessLogs(w http.ResponseWriter, r *http.Request, id int32) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	process, err := server.source.FindProcess(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "process not found")
		return
	}

	stream := r.URL.Query().Get("stream")
	var filePath string
	read := readLog
	switch stream {
	case "", "out", "stdout":
		filePath = process.LogFilePath
	case "err", "stderr":
		filePath = process.ErrFilePath
	case "both", "combined":
		filePath = logstore.CombinedPath(process.LogFilePath)
		read = readCombinedLog
	default:
		writeJSONError(w, http.StatusBadRequest, "invalid log stream")
		return
	}
	if filePath == "" {
		writeJSONError(w, http.StatusNotFound, "log file not available")
		return
	}

	offset := parseInt64Query(r, "offset", 0)
	tail := parseIntQuery(r, "tail", 200)
	if tail > 1000 {
		tail = 1000
	}
	logs, err := read(filePath, offset, tail)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "failed to read log file")
		return
	}
	writeJSON(w, http.StatusOK, logs)
}

func (server *Server) handleProcessAction(w http.ResponseWriter, r *http.Request, id int32) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if server.config.ReadOnly {
		writeJSONError(w, http.StatusForbidden, "web dashboard is read-only")
		return
	}
	if !server.validCSRF(r) {
		writeJSONError(w, http.StatusForbidden, "invalid csrf token")
		return
	}

	var request struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid action request")
		return
	}
	action := strings.ToLower(strings.TrimSpace(request.Action))

	process, _ := server.source.FindProcess(r.Context(), id)
	processName := ""
	if process != nil {
		processName = process.Name
	}

	var (
		updated *pb.Process
		success bool
		err     error
	)
	switch action {
	case "stop":
		success, err = server.source.StopProcess(r.Context(), id)
	case "start", "restart":
		if process == nil {
			writeJSONError(w, http.StatusNotFound, "process not found")
			return
		}
		updated, err = server.source.RestartProcess(r.Context(), process, false)
		success = err == nil && updated != nil
	case "reload":
		if process == nil {
			writeJSONError(w, http.StatusNotFound, "process not found")
			return
		}
		updated, err = server.source.RestartProcess(r.Context(), process, true)
		success = err == nil && updated != nil
	case "delete":
		success, err = server.source.DeleteProcess(r.Context(), id)
	default:
		writeJSONError(w, http.StatusBadRequest, "unsupported action")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "action failed")
		return
	}
	if !success {
		writeJSONError(w, http.StatusNotFound, "process not found")
		return
	}

	server.events.add(eventView{
		ProcessID:   id,
		ProcessName: processName,
		Type:        "action",
		Message:     action + " requested from web dashboard",
	})

	response := map[string]interface{}{
		"success": true,
		"action":  action,
	}
	if updated != nil {
		server.metrics.observePartial([]*pb.Process{updated})
		response["process"] = newProcessView(updated)
	}
	writeJSON(w, http.StatusOK, response)
}

func (server *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if server.isAuthenticated(r) {
			next(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSONError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func (server *Server) isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	return err == nil && server.sessions.valid(cookie.Value)
}

func (server *Server) validCSRF(r *http.Request) bool {
	sessionCookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	token := r.Header.Get("X-CSRF-Token")
	if token == "" {
		token = r.URL.Query().Get("csrf")
	}
	return server.sessions.validCSRF(sessionCookie.Value, token)
}

func (server *Server) validToken(token string) bool {
	candidateHash := sha256.Sum256([]byte(strings.TrimSpace(token)))
	expectedHash := sha256.Sum256([]byte(server.config.Token))
	return subtle.ConstantTimeCompare(candidateHash[:], expectedHash[:]) == 1
}

func (server *Server) serveAsset(w http.ResponseWriter, r *http.Request, name string) {
	contents, err := fs.ReadFile(server.assets, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if server.config.DevReload && strings.HasSuffix(name, ".html") {
		contents = injectDevReloadScript(contents)
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(contents))
}

func (server *Server) handleDevReloadScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(devReloadScript))
}

func (server *Server) handleDevReloadVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	version, err := assetFingerprint(server.assets)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to fingerprint assets")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(version))
}

func injectDevReloadScript(contents []byte) []byte {
	tag := []byte(`<script src="` + devReloadScriptPath + `" defer></script>`)
	if bytes.Contains(contents, tag) {
		return contents
	}
	if bytes.Contains(contents, []byte("</body>")) {
		return bytes.Replace(contents, []byte("</body>"), append(tag, []byte("\n  </body>")...), 1)
	}
	return append(contents, tag...)
}

func assetFingerprint(assets fs.FS) (string, error) {
	hash := sha256.New()
	err := fs.WalkDir(assets, "assets", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		switch {
		case strings.HasSuffix(path, ".css"),
			strings.HasSuffix(path, ".html"),
			strings.HasSuffix(path, ".js"),
			strings.HasSuffix(path, ".svg"):
		default:
			return nil
		}
		contents, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		_, _ = hash.Write([]byte(path))
		_, _ = hash.Write(contents)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

const devReloadScript = `(() => {
  const endpoint = "/dev/reload/version";
  let current = "";

  async function check() {
    try {
      const response = await fetch(endpoint, { cache: "no-store" });
      if (!response.ok) return;
      const next = (await response.text()).trim();
      if (!current) {
        current = next;
        return;
      }
      if (next && next !== current) {
        window.location.reload();
      }
    } catch (_) {
      // Keep the dashboard usable if the dev server is stopped.
    }
  }

  window.setInterval(check, 700);
  check();
})();`

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		if !strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func processRoute(path string) (int32, string, bool) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/processes/"), "/")
	if trimmed == "" {
		return 0, "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) > 2 {
		return 0, "", false
	}
	id, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return 0, "", false
	}
	suffix := ""
	if len(parts) == 2 {
		suffix = parts[1]
	}
	return int32(id), suffix, true
}

func parseIntQuery(r *http.Request, name string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return fallback
	}
	return value
}

func parseInt64Query(r *http.Request, name string, fallback int64) int64 {
	value, err := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	if err != nil {
		return fallback
	}
	return value
}
