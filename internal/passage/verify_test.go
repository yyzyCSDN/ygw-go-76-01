package passage_test

import (
	"testing"

	"afcgatecontrol/internal/passage"
)

type fakeChannelSource struct {
	snapshots []passage.ChannelSnapshot
	latest    passage.ChannelSnapshot
	byID      map[string]passage.ChannelSnapshot
}

func (f *fakeChannelSource) ChannelSnapshots() []passage.ChannelSnapshot {
	return f.snapshots
}

func (f *fakeChannelSource) LatestChannelSnapshot() passage.ChannelSnapshot {
	return f.latest
}

func (f *fakeChannelSource) SnapshotOf(channelID string) (passage.ChannelSnapshot, bool) {
	snap, ok := f.byID[channelID]
	return snap, ok
}

func TestPassageStatsRecoveryUsesLatestSnapshot(t *testing.T) {
	source := &fakeChannelSource{
		snapshots: []passage.ChannelSnapshot{
			{ChannelID: "CH-3", Seq: 1, Counted: 5, Total: 100, TakenAt: 1000},
			{ChannelID: "CH-3", Seq: 2, Counted: 8, Total: 160, TakenAt: 2000},
		},
		latest: passage.ChannelSnapshot{ChannelID: "CH-3", Seq: 2, Counted: 8, Total: 160, TakenAt: 2000},
	}
	stats := passage.NewStatsService(source)
	if err := stats.Recover(); err != nil {
		t.Fatal(err)
	}
	stats.Replay("CH-3", 3)
	if got := stats.Total("CH-3"); got != 160 {
		t.Fatalf("stats recovery must use the latest channel snapshot and skip already counted channels, got %d", got)
	}
}
