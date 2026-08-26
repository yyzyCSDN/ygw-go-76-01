package alarm

type SeverityCount struct {
	Info     int
	Warning  int
	Critical int
	Total    int
}

func (m *Manager) CountBySeverity() SeverityCount {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := SeverityCount{}
	for _, item := range m.alarms {
		if !item.Active {
			continue
		}
		count.Total++
		switch item.Severity {
		case SeverityWarning:
			count.Warning++
		case SeverityCritical:
			count.Critical++
		default:
			count.Info++
		}
	}
	return count
}
