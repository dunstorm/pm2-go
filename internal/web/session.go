package web

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]time.Time
	ttl      time.Duration
	now      func() time.Time
}

func newSessionStore(ttl time.Duration) *sessionStore {
	return &sessionStore{
		sessions: make(map[string]time.Time),
		ttl:      ttl,
		now:      time.Now,
	}
}

func (store *sessionStore) create() (string, time.Time, error) {
	id, err := generateSessionID()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := store.now().Add(store.ttl)

	store.mu.Lock()
	store.sessions[id] = expiresAt
	store.mu.Unlock()

	return id, expiresAt, nil
}

func (store *sessionStore) valid(id string) bool {
	if id == "" {
		return false
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	expiresAt, ok := store.sessions[id]
	if !ok {
		return false
	}
	if !expiresAt.After(store.now()) {
		delete(store.sessions, id)
		return false
	}
	return true
}

func (store *sessionStore) delete(id string) {
	store.mu.Lock()
	delete(store.sessions, id)
	store.mu.Unlock()
}

func generateSessionID() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
