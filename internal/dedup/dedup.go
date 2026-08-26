package dedup

import (
	"sync"
	"time"
)

type Entry struct {
	Key       string
	ExpiresAt time.Time
}

type DedupManager struct {
	mu     sync.RWMutex
	ttl    time.Duration
	items  map[string]Entry
	now    func() time.Time
}

func NewDedupManager(ttl time.Duration) *DedupManager {
	return &DedupManager{
		ttl:   ttl,
		items: make(map[string]Entry),
		now:   time.Now,
	}
}

func (m *DedupManager) Set(key string) {
	m.mu.Lock()
	m.items[key] = Entry{Key: key, ExpiresAt: m.now().Add(m.ttl)}
	m.mu.Unlock()
}

func (m *DedupManager) Check(key string) bool {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[key]
	if !ok {
		return false
	}
	if !now.Before(item.ExpiresAt) {
		delete(m.items, key)
		return false
	}
	return true
}

func (m *DedupManager) Clear(key string) {
	m.mu.Lock()
	delete(m.items, key)
	m.mu.Unlock()
}

func (m *DedupManager) Sweep() int {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	for key, item := range m.items {
		if !now.Before(item.ExpiresAt) {
			delete(m.items, key)
			removed++
		}
	}
	return removed
}

func (m *DedupManager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}
