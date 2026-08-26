package ticket

import (
	"sort"
)

type TypeSummary struct {
	TicketType string
	Total      int
	Entered    int
	Exited     int
}

type StoreReport struct {
	Total    int
	ByStatus map[string]int
	ByType   []TypeSummary
}

func (s *TicketStore) ListByStatus(status TicketStatus) []TicketRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TicketRecord, 0)
	for _, rec := range s.records {
		if rec.Status != status {
			continue
		}
		out = append(out, *rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func (s *TicketStore) Recent(limit int) []TicketRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := make([]TicketRecord, 0, len(s.records))
	for _, rec := range s.records {
		all = append(all, *rec)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].EnteredAt != all[j].EnteredAt {
			return all[i].EnteredAt > all[j].EnteredAt
		}
		return all[i].ID < all[j].ID
	})
	if limit <= 0 || limit > len(all) {
		limit = len(all)
	}
	return all[:limit]
}

func (s *TicketStore) Report() StoreReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	report := StoreReport{
		Total:    len(s.records),
		ByStatus: map[string]int{"unused": 0, "entered": 0, "exited": 0},
	}
	byType := make(map[string]*TypeSummary)
	for _, rec := range s.records {
		switch rec.Status {
		case StatusEntered:
			report.ByStatus["entered"]++
		case StatusExited:
			report.ByStatus["exited"]++
		default:
			report.ByStatus["unused"]++
		}
		summary, ok := byType[rec.TicketType]
		if !ok {
			summary = &TypeSummary{TicketType: rec.TicketType}
			byType[rec.TicketType] = summary
		}
		summary.Total++
		switch rec.Status {
		case StatusEntered:
			summary.Entered++
		case StatusExited:
			summary.Exited++
		}
	}
	for _, summary := range byType {
		report.ByType = append(report.ByType, *summary)
	}
	sort.Slice(report.ByType, func(i, j int) bool {
		return report.ByType[i].TicketType < report.ByType[j].TicketType
	})
	return report
}
