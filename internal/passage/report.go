package passage

type PassageReport struct {
	GateID         string
	Total          int
	Session        int
	ConfirmedAt    int64
	ChannelTotal   int64
	ChannelCounted int64
}

type ReportGenerator struct {
	counter *PassageCounter
	stats   *StatsService
}

func NewReportGenerator(counter *PassageCounter, stats *StatsService) *ReportGenerator {
	return &ReportGenerator{
		counter: counter,
		stats:   stats,
	}
}

func (g *ReportGenerator) Build(gateID string, channelID string) PassageReport {
	return PassageReport{
		GateID:         gateID,
		Total:          g.counter.Total(gateID),
		Session:        g.counter.SessionCount(gateID),
		ConfirmedAt:    g.counter.LastConfirmed(gateID),
		ChannelTotal:   g.stats.Total(channelID),
		ChannelCounted: g.stats.Counted(channelID),
	}
}

func (g *ReportGenerator) BuildAll(pairs map[string]string) []PassageReport {
	keys := make([]string, 0, len(pairs))
	for gateID := range pairs {
		keys = append(keys, gateID)
	}
	sortStrings(keys)
	out := make([]PassageReport, 0, len(keys))
	for _, gateID := range keys {
		out = append(out, g.Build(gateID, pairs[gateID]))
	}
	return out
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
