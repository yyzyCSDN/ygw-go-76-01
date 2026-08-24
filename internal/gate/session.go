package gate

import (
	"sync"

	"afcgatecontrol/internal/passage"
)

type SessionLedger struct {
	mu       sync.Mutex
	seq      int64
	sessions []passage.SessionSnapshot
	latest   *passage.SessionSnapshot
}

func NewSessionLedger() *SessionLedger {
	return &SessionLedger{}
}

func (l *SessionLedger) Save(id string, counts map[string]int, endedAt int64) passage.SessionSnapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	copy := make(map[string]int, len(counts))
	for key, value := range counts {
		copy[key] = value
	}
	item := passage.SessionSnapshot{
		ID:      id,
		Seq:     l.seq,
		Counts:  copy,
		EndedAt: endedAt,
	}
	l.sessions = append(l.sessions, item)
	l.latest = &item
	return item
}

func (l *SessionLedger) Sessions() []passage.SessionSnapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]passage.SessionSnapshot, len(l.sessions))
	copy(out, l.sessions)
	return out
}

func (l *SessionLedger) LatestSession() passage.SessionSnapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.latest == nil {
		return passage.SessionSnapshot{}
	}
	item := *l.latest
	counts := make(map[string]int, len(item.Counts))
	for key, value := range item.Counts {
		counts[key] = value
	}
	item.Counts = counts
	return item
}

func (l *SessionLedger) Count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.sessions)
}

var _ passage.SessionSource = (*SessionLedger)(nil)
