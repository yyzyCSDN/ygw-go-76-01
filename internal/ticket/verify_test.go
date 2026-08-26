package ticket_test

import (
	"path/filepath"
	"testing"
	"time"

	"afcgatecontrol/internal/alarm"
	"afcgatecontrol/internal/dedup"
	"afcgatecontrol/internal/gate"
	"afcgatecontrol/internal/passage"
	"afcgatecontrol/internal/rule"
	"afcgatecontrol/internal/ticket"
)

func TestExitWritebackErrorNotSwallowed(t *testing.T) {
	store, err := ticket.NewTicketStore(filepath.Join(t.TempDir(), "tickets"))
	if err != nil {
		t.Fatal(err)
	}
	mgr := rule.NewRuleManager(rule.NewRuleStore())
	dm := dedup.NewDedupManager(time.Minute)
	validator := ticket.NewValidator(store, mgr, dm)
	counter := passage.NewPassageCounter()
	recorder := gate.NewEventRecorder(nil)
	sessions := gate.NewSessionLedger()
	channels := gate.NewChannelRegistry()
	alarms := alarm.NewManager()
	ctrl := gate.NewController(
		gate.NewGateStore(),
		store,
		validator,
		counter,
		recorder,
		sessions,
		channels,
		dm,
		alarms,
		nil,
		time.Second,
	)
	rec := &ticket.TicketRecord{ID: "T-EXIT", TicketType: "MULTI", Transferable: true}
	if err := store.Put(rec); err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.HandleEntry("T-EXIT", "NORMAL", "G1"); err != nil {
		t.Fatal(err)
	}
	key := dedup.SwipeKey("T-EXIT")
	if !dm.Check(key) {
		t.Fatal("entry must set the dedup key")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.HandleExit("T-EXIT", "G2"); err == nil {
		t.Fatal("exit writeback failure must be reported")
	}
	if dm.Check(key) {
		t.Fatal("failed exit writeback must clear the dedup key")
	}
	after, ok := store.Lookup("T-EXIT")
	if !ok || after.Status != ticket.StatusUnused {
		t.Fatal("failed exit writeback must not leave the stale entered state")
	}
}
