package heartbeat

import (
	"fmt"
	"sync"

	"afcgatecontrol/internal/alarm"
	"afcgatecontrol/internal/gate"
)

type HeartbeatSnapshot struct {
	GateID string
	Online bool
	Seq    int64
	At     int64
}

type Monitor struct {
	store    *gate.GateStore
	alarms   *alarm.Manager
	timeout  int64
	mu       sync.Mutex
	lastSeen map[string]int64
	applied  map[string]int64
	timeouts map[string]int
}

func NewMonitor(store *gate.GateStore, alarms *alarm.Manager, timeoutSeconds int64) *Monitor {
	return &Monitor{
		store:    store,
		alarms:   alarms,
		timeout:  timeoutSeconds,
		lastSeen: make(map[string]int64),
		applied:  make(map[string]int64),
		timeouts: make(map[string]int),
	}
}

func (m *Monitor) Ingest(gateID string, at int64) {
	m.mu.Lock()
	m.lastSeen[gateID] = at
	m.mu.Unlock()
}

func (m *Monitor) Check(gateID string, now int64) error {
	m.mu.Lock()
	last, ok := m.lastSeen[gateID]
	m.mu.Unlock()
	if !ok {
		last = now
	}
	if now-last > m.timeout {
		m.mu.Lock()
		m.timeouts[gateID]++
		timeoutCount := m.timeouts[gateID]
		m.mu.Unlock()
		_ = m.store.ApplyOffline(gateID, now)
		severity := alarm.SeverityWarning
		if timeoutCount >= 2 {
			severity = alarm.SeverityCritical
		}
		if m.alarms != nil {
			m.alarms.Raise(gateID, "heartbeat-timeout", severity)
		}
		return fmt.Errorf(
			"heartbeat timeout for gate %s: last=%d now=%d timeouts=%d",
			gateID,
			last,
			now,
			timeoutCount,
		)
	}
	return nil
}

func (m *Monitor) ApplyHeartbeat(snapshot HeartbeatSnapshot) gate.GateStatus {
	m.mu.Lock()
	last, ok := m.applied[snapshot.GateID]
	if ok && snapshot.At < last {
		m.mu.Unlock()
		return m.store.Status(snapshot.GateID)
	}
	m.applied[snapshot.GateID] = snapshot.At
	m.mu.Unlock()
	current := m.store.Status(snapshot.GateID)
	if snapshot.At < current.UpdatedAt {
		return current
	}
	return m.store.WriteHeartbeat(snapshot.GateID, snapshot.Online, snapshot.Seq, snapshot.At)
}
