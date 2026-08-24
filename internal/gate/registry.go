package gate

import (
	"sort"
	"sync"

	"afcgatecontrol/internal/rule"
)

type GateInfo struct {
	ID        string
	Name      string
	Location  string
	ChannelID string
	Group     string
	Enabled   bool
}

type GateRegistry struct {
	mu    sync.RWMutex
	gates map[string]GateInfo
}

func NewGateRegistry() *GateRegistry {
	return &GateRegistry{
		gates: make(map[string]GateInfo),
	}
}

func (r *GateRegistry) Register(info GateInfo) {
	r.mu.Lock()
	if info.ChannelID == "" {
		info.ChannelID = "CH-" + info.ID
	}
	if info.Group == "" {
		info.Group = rule.DefaultGateGroup
	}
	r.gates[info.ID] = info
	r.mu.Unlock()
}

func (r *GateRegistry) Get(id string) (GateInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.gates[id]
	return info, ok
}

func (r *GateRegistry) List() []GateInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]GateInfo, 0, len(r.gates))
	for _, info := range r.gates {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func (r *GateRegistry) GroupOf(id string, fallback string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.gates[id]
	if !ok {
		return fallback
	}
	return info.Group
}

func (r *GateRegistry) ChannelOf(id string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.gates[id]
	if !ok {
		return "CH-" + id
	}
	return info.ChannelID
}

func (r *GateRegistry) EnabledIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0)
	for id, info := range r.gates {
		if info.Enabled {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
