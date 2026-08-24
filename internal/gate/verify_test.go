package gate_test

import (
	"context"
	"testing"
	"time"

	"afcgatecontrol/internal/alarm"
	"afcgatecontrol/internal/dedup"
	"afcgatecontrol/internal/gate"
	"afcgatecontrol/internal/passage"
	"afcgatecontrol/internal/rule"
	"afcgatecontrol/internal/ticket"
)

type stuckOpenSensor struct{}

func (s *stuckOpenSensor) Open() error {
	return nil
}

func (s *stuckOpenSensor) Close() error {
	return nil
}

func (s *stuckOpenSensor) ConfirmOpen(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (s *stuckOpenSensor) ConfirmClosed(ctx context.Context) error {
	return nil
}

func newTimeoutController(t *testing.T, timeout time.Duration) *gate.Controller {
	t.Helper()
	store, err := ticket.NewTicketStore(t.TempDir())
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
	return gate.NewController(
		gate.NewGateStore(),
		store,
		validator,
		counter,
		recorder,
		sessions,
		channels,
		dm,
		alarms,
		func(string) gate.Sensor { return &stuckOpenSensor{} },
		timeout,
	)
}

func TestGateOpenTimeoutRecovers(t *testing.T) {
	ctrl := newTimeoutController(t, 30*time.Millisecond)
	if err := ctrl.Open("G1"); err == nil {
		t.Fatal("sensor confirmation timeout must be reported")
	}
	if state := ctrl.State("G1"); state != gate.StateClosed {
		t.Fatalf("gate must recover to closed after a timeout, state=%s", state)
	}
}
