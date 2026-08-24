package ticket_test

import (
	"testing"
	"time"

	"afcgatecontrol/internal/dedup"
	"afcgatecontrol/internal/rule"
	"afcgatecontrol/internal/ticket"
)

func TestGateRuleRefreshOnTicketUpgrade(t *testing.T) {
	mgr := rule.NewRuleManager(rule.NewRuleStore())
	store, err := ticket.NewTicketStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dm := dedup.NewDedupManager(time.Minute)
	validator := ticket.NewValidator(store, mgr, dm)
	flow := ticket.NewEntryFlow(validator)
	group := rule.TransferGateGroup
	first := &ticket.TicketRecord{ID: "T-UP-1", TicketType: "SINGLE", Transferable: false}
	if err := store.Put(first); err != nil {
		t.Fatal(err)
	}
	if _, err := flow.Run(first, group, "G-EDGE"); err != nil {
		t.Fatalf("single-trip ticket must pass the transfer gate before upgrade: %v", err)
	}
	if _, err := mgr.Upgrade(group, false, true); err != nil {
		t.Fatal(err)
	}
	second := &ticket.TicketRecord{ID: "T-UP-2", TicketType: "SINGLE", Transferable: false}
	if err := store.Put(second); err != nil {
		t.Fatal(err)
	}
	outcome, err := flow.Run(second, group, "G-EDGE")
	if err == nil || (outcome != nil && outcome.Allowed) {
		t.Fatal("single-trip ticket must be rejected at the transfer gate after the rule upgrade")
	}
}
