package gate

import (
	"context"
	"sort"
	"sync"
	"time"
)

type ScheduleEntry struct {
	GateID  string
	OpenAt  int64
	CloseAt int64
	Enabled bool
}

type Scheduler struct {
	mu      sync.Mutex
	store   *GateStore
	entries map[string]ScheduleEntry
}

func NewScheduler(store *GateStore) *Scheduler {
	return &Scheduler{
		store:   store,
		entries: make(map[string]ScheduleEntry),
	}
}

func (s *Scheduler) Add(entry ScheduleEntry) {
	s.mu.Lock()
	entry.Enabled = true
	s.entries[entry.GateID] = entry
	s.mu.Unlock()
}

func (s *Scheduler) Entries() []ScheduleEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ScheduleEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].GateID < out[j].GateID
	})
	return out
}

func (s *Scheduler) RunOnce(now int64, open func(string) error, closeFn func(string) error) []string {
	s.mu.Lock()
	entries := make([]ScheduleEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		entries = append(entries, entry)
	}
	s.mu.Unlock()
	actions := make([]string, 0)
	for _, entry := range entries {
		if !entry.Enabled {
			continue
		}
		current := s.store.Status(entry.GateID)
		if entry.OpenAt > 0 && now >= entry.OpenAt && current.State != StateOpen {
			if err := open(entry.GateID); err == nil {
				actions = append(actions, "open:"+entry.GateID)
			}
		}
		if entry.CloseAt > 0 && now >= entry.CloseAt && current.State == StateOpen {
			if err := closeFn(entry.GateID); err == nil {
				actions = append(actions, "close:"+entry.GateID)
			}
		}
	}
	return actions
}

func (s *Scheduler) Run(ctx context.Context, interval time.Duration, open func(string) error, closeFn func(string) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.RunOnce(now.Unix(), open, closeFn)
		}
	}
}
