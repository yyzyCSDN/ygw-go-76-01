package gate

import (
	"context"
	"sync"
	"time"
)

type PollManager struct {
	store  *GateStore
	mu     sync.Mutex
	states map[string]bool
}

func NewPollManager(store *GateStore) *PollManager {
	return &PollManager{
		store:  store,
		states: make(map[string]bool),
	}
}

func (p *PollManager) SetDeviceState(gateID string, online bool) {
	p.mu.Lock()
	p.states[gateID] = online
	p.mu.Unlock()
}

func (p *PollManager) PollOnce(at int64) []GateStatus {
	p.mu.Lock()
	ids := make([]string, 0, len(p.states))
	for id := range p.states {
		ids = append(ids, id)
	}
	onlineMap := make(map[string]bool, len(p.states))
	for id, online := range p.states {
		onlineMap[id] = online
	}
	p.mu.Unlock()
	out := make([]GateStatus, 0, len(ids))
	for _, id := range ids {
		out = append(out, p.store.ApplyPoll(id, onlineMap[id], at))
	}
	return out
}

func (p *PollManager) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.PollOnce(time.Now().Unix())
		}
	}
}
