package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"sync"
	"time"
)

type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]session
	ttl      time.Duration
	now      func() time.Time
}

type session struct {
	expiresAt time.Time
	csrfToken string
}

func newSessionStore(ttl time.Duration) *sessionStore {
	return &sessionStore{
		sessions: make(map[string]session),
		ttl:      ttl,
		now:      time.Now,
	}
}

func (store *sessionStore) create() (string, string, time.Time, error) {
	id, err := generateSessionID()
	if err != nil {
		return "", "", time.Time{}, err
	}
	csrfToken, err := generateSessionID()
	if err != nil {
		return "", "", time.Time{}, err
	}
	now := store.now()
	expiresAt := now.Add(store.ttl)

	store.mu.Lock()
	store.pruneExpiredLocked(now)
	store.sessions[id] = session{
		expiresAt: expiresAt,
		csrfToken: csrfToken,
	}
	store.mu.Unlock()

	return id, csrfToken, expiresAt, nil
}

func (store *sessionStore) valid(id string) bool {
	if id == "" {
		return false
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	session, ok := store.sessions[id]
	if !ok {
		return false
	}
	if !session.expiresAt.After(store.now()) {
		delete(store.sessions, id)
		return false
	}
	return true
}

func (store *sessionStore) validCSRF(id string, token string) bool {
	if id == "" || token == "" {
		return false
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	session, ok := store.sessions[id]
	if !ok {
		return false
	}
	if !session.expiresAt.After(store.now()) {
		delete(store.sessions, id)
		return false
	}
	return subtle.ConstantTimeCompare([]byte(session.csrfToken), []byte(token)) == 1
}

func (store *sessionStore) delete(id string) {
	store.mu.Lock()
	delete(store.sessions, id)
	store.mu.Unlock()
}

func (store *sessionStore) pruneExpiredLocked(now time.Time) {
	for id, session := range store.sessions {
		if !session.expiresAt.After(now) {
			delete(store.sessions, id)
		}
	}
}

func generateSessionID() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
