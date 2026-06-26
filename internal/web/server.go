package web

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

const sessionCookieName = "pm2_go_web_session"

//go:embed assets/*
var embeddedAssets embed.FS

type Server struct {
	config   runtimeConfig
	source   ProcessSource
	sessions *sessionStore
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

	return &Server{
		config:   runtime,
		source:   source,
		sessions: newSessionStore(runtime.SessionTTL),
		assets:   embeddedAssets,
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
	mux.Handle("/assets/", http.FileServer(http.FS(server.assets)))
	mux.HandleFunc("/login", server.handleLoginPage)
	mux.HandleFunc("/auth/login", server.handleLogin)
	mux.HandleFunc("/auth/logout", server.requireAuth(server.handleLogout))
	mux.HandleFunc("/api/processes", server.requireAuth(server.handleProcesses))
	mux.HandleFunc("/", server.requireAuth(server.handleIndex))
	return securityHeaders(mux)
}

func (server *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	serveAsset(w, r, server.assets, "assets/index.html")
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
	serveAsset(w, r, server.assets, "assets/login.html")
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

	sessionID, expiresAt, err := server.sessions.create()
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
	http.Redirect(w, r, "/login", http.StatusSeeOther)
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
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"processes": processViews(processes),
	})
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

func (server *Server) validToken(token string) bool {
	candidateHash := sha256.Sum256([]byte(strings.TrimSpace(token)))
	expectedHash := sha256.Sum256([]byte(server.config.Token))
	return subtle.ConstantTimeCompare(candidateHash[:], expectedHash[:]) == 1
}

func serveAsset(w http.ResponseWriter, r *http.Request, assets fs.FS, name string) {
	contents, err := fs.ReadFile(assets, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(contents))
}

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
