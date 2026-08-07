package web

import (
	"testing"
	"time"
)

func TestSessionCreatePrunesExpiredSessions(t *testing.T) {
	now := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	store := newSessionStore(time.Hour)
	store.now = func() time.Time {
		return now
	}
	store.sessions["expired"] = session{expiresAt: now.Add(-time.Second)}
	store.sessions["active"] = session{expiresAt: now.Add(time.Minute)}

	if _, _, _, err := store.create(); err != nil {
		t.Fatalf("create session: %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.sessions["expired"]; ok {
		t.Fatal("expected expired session to be pruned")
	}
	if _, ok := store.sessions["active"]; !ok {
		t.Fatal("expected active session to remain")
	}
	if len(store.sessions) != 2 {
		t.Fatalf("expected active plus new session, got %d sessions", len(store.sessions))
	}
}
