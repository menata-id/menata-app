package web

import (
	"sync"
	"time"
)

// fileDraft is what a Workspace admin pasted for one file and has not applied: their own, per Workspace and file, kept in the
// process and not on disk.
type fileDraft struct {
	yaml string
	at   time.Time
}

type draftKey struct{ workspaceID, userID, path string }

// fileDraftStore keeps the pasted content between the Review and the Apply of a file, so the page that shows what the loader says
// is a page that can be drawn again (a GET) and Apply acts on what was reviewed and not on a second paste. It is deliberately not
// persistent: a draft is minutes of work, a restart discards it, and nothing here is a record. Each entry expires, and the store
// holds a bounded number, evicting the oldest, so a forgotten paste cannot grow without limit.
type fileDraftStore struct {
	mu      sync.Mutex
	entries map[draftKey]fileDraft
	now     func() time.Time
}

const (
	fileDraftTTL = 2 * time.Hour
	fileDraftMax = 200
)

var fileDrafts = &fileDraftStore{entries: map[draftKey]fileDraft{}, now: time.Now}

func (s *fileDraftStore) put(k draftKey, yaml string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for key, d := range s.entries {
		if now.Sub(d.at) > fileDraftTTL {
			delete(s.entries, key)
		}
	}
	for len(s.entries) >= fileDraftMax {
		var oldest draftKey
		var oldestAt time.Time
		for key, d := range s.entries {
			if oldestAt.IsZero() || d.at.Before(oldestAt) {
				oldest, oldestAt = key, d.at
			}
		}
		delete(s.entries, oldest)
	}
	s.entries[k] = fileDraft{yaml: yaml, at: now}
}

func (s *fileDraftStore) get(k draftKey) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.entries[k]
	if !ok || s.now().Sub(d.at) > fileDraftTTL {
		delete(s.entries, k)
		return "", false
	}
	return d.yaml, true
}

func (s *fileDraftStore) drop(k draftKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, k)
}
