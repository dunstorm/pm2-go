package web

import (
	"sync"
	"time"
)

const maxEvents = 200

type eventView struct {
	Timestamp   string `json:"timestamp"`
	ProcessID   int32  `json:"process_id,omitempty"`
	ProcessName string `json:"process_name,omitempty"`
	Type        string `json:"type"`
	Message     string `json:"message"`
}

type eventStore struct {
	mu     sync.Mutex
	events []eventView
	now    func() time.Time
}

func newEventStore() *eventStore {
	return &eventStore{now: time.Now}
}

func (store *eventStore) add(event eventView) {
	if event.Timestamp == "" {
		event.Timestamp = store.now().UTC().Format(time.RFC3339)
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	store.events = append(store.events, event)
	if len(store.events) > maxEvents {
		store.events = append([]eventView(nil), store.events[len(store.events)-maxEvents:]...)
	}
}

func (store *eventStore) list() []eventView {
	store.mu.Lock()
	defer store.mu.Unlock()

	events := make([]eventView, len(store.events))
	for i := range store.events {
		events[len(store.events)-1-i] = store.events[i]
	}
	return events
}
