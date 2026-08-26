package gate

import (
	"sync"

	"afcgatecontrol/internal/passage"
)

type ChannelRegistry struct {
	mu      sync.Mutex
	seq     int64
	latest  map[string]*passage.ChannelSnapshot
	history []passage.ChannelSnapshot
}

func NewChannelRegistry() *ChannelRegistry {
	return &ChannelRegistry{
		latest: make(map[string]*passage.ChannelSnapshot),
	}
}

func (r *ChannelRegistry) SnapshotChannel(channelID string, counted int64, total int64, takenAt int64) passage.ChannelSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	item := passage.ChannelSnapshot{
		ChannelID: channelID,
		Seq:       r.seq,
		Counted:   counted,
		Total:     total,
		TakenAt:   takenAt,
	}
	r.latest[channelID] = &item
	r.history = append(r.history, item)
	return item
}

func (r *ChannelRegistry) ChannelSnapshots() []passage.ChannelSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]passage.ChannelSnapshot, len(r.history))
	copy(out, r.history)
	return out
}

func (r *ChannelRegistry) SnapshotOf(channelID string) (passage.ChannelSnapshot, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.latest[channelID]
	if !ok {
		return passage.ChannelSnapshot{}, false
	}
	return *item, true
}

func (r *ChannelRegistry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.latest)
}

var _ passage.ChannelSource = (*ChannelRegistry)(nil)
