package passage_test

import (
	"testing"

	"afcgatecontrol/internal/gate"
	"afcgatecontrol/internal/passage"
)

func TestPassageRecoveryUsesLatestSnapshot(t *testing.T) {
	ledger := gate.NewSessionLedger()
	ledger.Save("session-1", map[string]int{"G1": 10}, 1000)
	ledger.Save("session-2", map[string]int{"G1": 30}, 2000)
	counter := passage.NewPassageCounter()
	recovery := passage.NewRecovery(ledger, counter)
	if _, err := recovery.Restore(); err != nil {
		t.Fatal(err)
	}
	if got := counter.Total("G1"); got != 30 {
		t.Fatalf("recovery must use the latest session snapshot, got %d want 30", got)
	}
}
