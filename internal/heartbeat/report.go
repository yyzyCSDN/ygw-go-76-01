package heartbeat

type MonitorSummary struct {
	Total         int
	Online        int
	Offline       int
	Stale         int
	Unknown       int
	TimeoutEvents int
}

func (m *Monitor) Summary(gateIDs []string, now int64) MonitorSummary {
	summary := MonitorSummary{Total: len(gateIDs)}
	m.mu.Lock()
	timeout := m.timeout
	lastSeen := make(map[string]int64, len(m.lastSeen))
	timeouts := make(map[string]int, len(m.timeouts))
	for gateID, last := range m.lastSeen {
		lastSeen[gateID] = last
	}
	for gateID, count := range m.timeouts {
		timeouts[gateID] = count
	}
	m.mu.Unlock()
	for _, count := range timeouts {
		summary.TimeoutEvents += count
	}
	for _, gateID := range gateIDs {
		last, ok := lastSeen[gateID]
		if !ok {
			summary.Unknown++
			continue
		}
		if now-last > timeout {
			summary.Stale++
			continue
		}
		if m.store.Status(gateID).Online {
			summary.Online++
		} else {
			summary.Offline++
		}
	}
	return summary
}
