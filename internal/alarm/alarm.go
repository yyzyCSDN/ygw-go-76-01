package alarm

import (
	"fmt"
	"sort"
	"sync"
)

type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityWarning:
		return "warning"
	case SeverityCritical:
		return "critical"
	default:
		return "info"
	}
}

type Alarm struct {
	ID         string
	GateID     string
	Kind       string
	Severity   Severity
	Active     bool
	RaisedAt   int64
	ResolvedAt int64
}

type Manager struct {
	mu      sync.Mutex
	seq     int64
	alarms  map[string]*Alarm
	history []Alarm
}

func NewManager() *Manager {
	return &Manager{
		alarms: make(map[string]*Alarm),
	}
}

func alarmKey(gateID string, kind string) string {
	return gateID + "|" + kind
}

func (m *Manager) Raise(gateID string, kind string, severity Severity) Alarm {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := alarmKey(gateID, kind)
	item, ok := m.alarms[key]
	if !ok {
		m.seq++
		item = &Alarm{
			ID:       fmt.Sprintf("AL-%d", m.seq),
			GateID:   gateID,
			Kind:     kind,
			Severity: severity,
			RaisedAt: m.seq,
		}
		m.alarms[key] = item
	}
	item.Active = true
	item.Severity = severity
	return *item
}

func (m *Manager) Resolve(gateID string, kind string) (Alarm, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := alarmKey(gateID, kind)
	item, ok := m.alarms[key]
	if !ok {
		return Alarm{}, false
	}
	if !item.Active {
		return *item, false
	}
	item.Active = false
	item.ResolvedAt = m.seq
	m.history = append(m.history, *item)
	return *item, true
}

func (m *Manager) Active(gateID string) []Alarm {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Alarm, 0)
	for key, item := range m.alarms {
		if !item.Active {
			continue
		}
		if gateID != "" && item.GateID != gateID {
			continue
		}
		out = append(out, *item)
		_ = key
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func (m *Manager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, item := range m.alarms {
		if item.Active {
			count++
		}
	}
	return count
}
