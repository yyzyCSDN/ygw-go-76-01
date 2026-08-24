package passage

import "testing"

// stubSessionSource implements SessionSource for tests.
type stubSessionSource struct {
	sessions []SessionSnapshot
}

func (s stubSessionSource) Sessions() []SessionSnapshot { return s.sessions }

func TestRecovery_RestoreUsesLatestSession(t *testing.T) {
	// The ledger appends snapshots in save order, so the newest snapshot is
	// the one with the highest Seq. Recovery must restore from the newest
	// cumulative counts, not the first slice element.
	source := stubSessionSource{sessions: []SessionSnapshot{
		{ID: "session-1", Seq: 1, Counts: map[string]int{"G-NORTH": 5}},
		{ID: "session-2", Seq: 2, Counts: map[string]int{"G-NORTH": 12}},
		{ID: "session-3", Seq: 3, Counts: map[string]int{"G-NORTH": 20}},
	}}
	counter := NewPassageCounter()
	recovery := NewRecovery(source, counter)

	restored, err := recovery.Restore()
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if restored.ID != "session-3" {
		t.Fatalf("restored latest = %q, want %q", restored.ID, "session-3")
	}
	if got := counter.Total("G-NORTH"); got != 20 {
		t.Fatalf("counter.Total = %d, want 20", got)
	}

	// LastRestored must reflect the snapshot actually used so reports expose
	// the correct recovery marker.
	gotID, gotSeq := recovery.LastRestored()
	if gotID != "session-3" || gotSeq != 3 {
		t.Fatalf("LastRestored = (%q, %d), want (%q, %d)", gotID, gotSeq, "session-3", 3)
	}
}

func TestRecovery_RestorePicksHighestSeqRegardlessOfOrder(t *testing.T) {
	// Ordering is not guaranteed; the newest snapshot is identified by Seq.
	source := stubSessionSource{sessions: []SessionSnapshot{
		{ID: "session-3", Seq: 3, Counts: map[string]int{"G-SOUTH": 7}},
		{ID: "session-1", Seq: 1, Counts: map[string]int{"G-SOUTH": 1}},
		{ID: "session-2", Seq: 2, Counts: map[string]int{"G-SOUTH": 3}},
	}}
	counter := NewPassageCounter()
	recovery := NewRecovery(source, counter)

	restored, _ := recovery.Restore()
	if restored.Seq != 3 {
		t.Fatalf("restored.Seq = %d, want 3", restored.Seq)
	}
	if got := counter.Total("G-SOUTH"); got != 7 {
		t.Fatalf("counter.Total = %d, want 7", got)
	}
}

func TestRecovery_RestoreEmpty(t *testing.T) {
	recovery := NewRecovery(stubSessionSource{}, NewPassageCounter())
	restored, err := recovery.Restore()
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if restored.ID != "" {
		t.Fatalf("restored.ID = %q, want empty", restored.ID)
	}
}
