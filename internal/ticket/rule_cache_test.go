package ticket

import (
	"errors"
	"testing"
	"time"

	"afcgatecontrol/internal/dedup"
	"afcgatecontrol/internal/rule"
)

// newTestValidator wires a Validator to a fresh rule manager exactly as the
// application does, so the test exercises the real cache-subscription path.
func newTestValidator(t *testing.T) (*Validator, *rule.RuleManager) {
	t.Helper()
	dir := t.TempDir()
	store, err := NewTicketStore(dir)
	if err != nil {
		t.Fatalf("create ticket store: %v", err)
	}
	if err := store.Put(&TicketRecord{ID: "T-SINGLE", TicketType: "SINGLE", Transferable: false}); err != nil {
		t.Fatalf("seed single ticket: %v", err)
	}
	if err := store.Put(&TicketRecord{ID: "T-XFER", TicketType: "TRANSFER", Transferable: true}); err != nil {
		t.Fatalf("seed transfer ticket: %v", err)
	}
	mgr := rule.NewRuleManager(rule.NewRuleStore())
	dm := dedup.NewDedupManager(5 * time.Minute)
	v := NewValidator(store, mgr, dm)
	t.Cleanup(v.Close)
	return v, mgr
}

// TestRuleUpgradeTakesEffectImmediately reproduces the reported bug: after a
// gate group is upgraded so that single-journey tickets are no longer allowed,
// the validator must reject the very next swipe instead of still honoring the
// stale startup snapshot.
func TestRuleUpgradeTakesEffectImmediately(t *testing.T) {
	v, mgr := newTestValidator(t)
	const group = rule.DefaultGateGroup

	singleRec, _ := v.store.Lookup("T-SINGLE")

	// Baseline: single ticket is allowed under the default rule set.
	if _, err := v.validateEntry(singleRec, group, "G-1"); err != nil {
		t.Fatalf("expected single ticket to be allowed before upgrade, got %v", err)
	}

	// Simulate the station denying single-journey tickets at this group.
	if _, err := mgr.Upgrade(group, false /* single */, true /* transfer */); err != nil {
		t.Fatalf("upgrade rule: %v", err)
	}

	// A fresh single ticket must now be rejected at the gate immediately,
	// without a restart and without a manual cache refresh.
	// The previous entry marked it entered; reset it and clear its dedup key
	// so it can be re-evaluated under the new rule.
	v.dedup.Clear(dedup.SwipeKey("T-SINGLE"))
	v.store.ResetEntry("T-SINGLE")
	freshSingle, _ := v.store.Lookup("T-SINGLE")

	_, err := v.validateEntry(freshSingle, group, "G-1")
	if !errors.Is(err, ErrRuleDenied) {
		t.Fatalf("expected ErrRuleDenied after upgrade, got %v", err)
	}

	// Transfer ticket still passes — the new rule applies, not the old one.
	v.dedup.Clear(dedup.SwipeKey("T-XFER"))
	xferRec, _ := v.store.Lookup("T-XFER")
	if _, err := v.validateEntry(xferRec, group, "G-1"); err != nil {
		t.Fatalf("expected transfer ticket to still be allowed after upgrade, got %v", err)
	}
}

// TestRuleCacheCloseUnsubscribes ensures the notifier no longer holds a
// reference to a closed validator's cache.
func TestRuleCacheCloseUnsubscribes(t *testing.T) {
	dir := t.TempDir()
	store, err := NewTicketStore(dir)
	if err != nil {
		t.Fatalf("create ticket store: %v", err)
	}
	mgr := rule.NewRuleManager(rule.NewRuleStore())
	dm := dedup.NewDedupManager(5 * time.Minute)
	v := NewValidator(store, mgr, dm)

	before := len(mgr.Notifier().SubIDs())
	v.Close()
	after := len(mgr.Notifier().SubIDs())
	if after != before-1 {
		t.Fatalf("expected subscriber count to drop by 1 on close, before=%d after=%d", before, after)
	}
}
