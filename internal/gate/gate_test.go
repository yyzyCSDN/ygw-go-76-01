package gate

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"afcgatecontrol/internal/alarm"
	"afcgatecontrol/internal/passage"
)

// mockSensor is a controllable Sensor for testing the open/close recovery
// path. Its ConfirmOpen/ConfirmClosed behavior is configurable so we can
// simulate a timed-out confirmation and — critically — a sensor that
// ignores its context (the "goroutine keeps running" failure mode).
type mockSensor struct {
	mu sync.Mutex

	openCalls  int32
	closeCalls int32

	// confirm blocks until released or the deadline fires.
	openRelease   chan struct{}
	closedRelease chan struct{}

	// ignoreCtx makes Confirm* block on the release channel regardless of
	// ctx, emulating a hardware driver that never checks its context.
	ignoreCtx bool

	// err returned by Confirm* when it returns normally (not on block).
	openErr   error
	closedErr error

	// actuatorErr, if non-nil, is returned by Open()/Close().
	actuatorErr error
}

func newMockSensor() *mockSensor {
	return &mockSensor{
		openRelease:   make(chan struct{}, 1),
		closedRelease: make(chan struct{}, 1),
	}
}

func (s *mockSensor) Open() error {
	atomic.AddInt32(&s.openCalls, 1)
	return s.actuatorErr
}

func (s *mockSensor) Close() error {
	atomic.AddInt32(&s.closeCalls, 1)
	return s.actuatorErr
}

func (s *mockSensor) ConfirmOpen(ctx context.Context) error {
	if s.ignoreCtx {
		<-s.openRelease
		return s.openErr
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.openRelease:
		return s.openErr
	}
}

func (s *mockSensor) ConfirmClosed(ctx context.Context) error {
	if s.ignoreCtx {
		<-s.closedRelease
		return s.closedErr
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closedRelease:
		return s.closedErr
	}
}

// blockingSensor never resolves Confirm* on its own and never checks ctx.
// It is only safe because waitConfirmation abandons its goroutine.
type blockingSensor struct {
	startedOpen   chan struct{}
	startedClosed chan struct{}
}

func newBlockingSensor() *blockingSensor {
	return &blockingSensor{
		startedOpen:   make(chan struct{}, 1),
		startedClosed: make(chan struct{}, 1),
	}
}

func (s *blockingSensor) Open() error  { return nil }
func (s *blockingSensor) Close() error { return nil }

func (s *blockingSensor) ConfirmOpen(ctx context.Context) error {
	select {
	case s.startedOpen <- struct{}{}:
	default:
	}
	select {} // block forever, ignore ctx
}

func (s *blockingSensor) ConfirmClosed(ctx context.Context) error {
	select {
	case s.startedClosed <- struct{}{}:
	default:
	}
	select {}
}

func newTestController(sensor Sensor, openTimeout time.Duration) (*Controller, *GateStore, *EventRecorder) {
	gates := NewGateStore()
	rec := NewEventRecorder(nil) // nil store => Record is a no-op
	sessions := NewSessionLedger()
	channels := NewChannelRegistry()
	var alarms *alarm.Manager
	counter := passage.NewPassageCounter()
	counter.SetStateReader(gates)
	c := &Controller{
		gates:      gates,
		recorder:   rec,
		sessions:   sessions,
		channels:   channels,
		alarms:     alarms,
		counter:    counter,
		sensors:    func(string) Sensor { return sensor },
		openTimeout: openTimeout,
	}
	return c, gates, rec
}

// TestOpenTimeoutRecoversToClosed reproduces the reported bug: a sensor that
// times out during confirmation leaves the gate wedged in StateOpening. After
// the fix the gate must return to StateClosed so a subsequent Open succeeds.
func TestOpenTimeoutRecoversToClosed(t *testing.T) {
	sensor := newMockSensor()
	c, gates, _ := newTestController(sensor, 20*time.Millisecond)

	// ConfirmOpen is never released, so it times out.
	err := c.Open("G-1")
	if !errors.Is(err, ErrSensorTimeout) {
		t.Fatalf("Open: want ErrSensorTimeout, got %v", err)
	}

	if got := gates.Status("G-1").State; got != StateClosed {
		t.Fatalf("after timeout: state = %s, want closed", got)
	}

	// Recovery: a second Open must not be rejected by an illegal
	// transition from a wedged opening state.
	sensor.openRelease <- struct{}{}
	if err := c.Open("G-1"); err != nil {
		t.Fatalf("recovery Open: %v", err)
	}
	if got := gates.Status("G-1").State; got != StateOpen {
		t.Fatalf("after recovery: state = %s, want open", got)
	}
}

// TestOpenTimeoutWithBlockingSensor covers the "goroutine keeps running"
// failure mode: a sensor that ignores its context entirely.
// waitConfirmation must still return on the deadline, and the gate must
// recover to closed.
func TestOpenTimeoutWithBlockingSensor(t *testing.T) {
	sensor := newBlockingSensor()
	c, gates, _ := newTestController(sensor, 20*time.Millisecond)

	errCh := make(chan error, 1)
	go func() { errCh <- c.Open("G-1") }()

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrSensorTimeout) {
			t.Fatalf("Open: want ErrSensorTimeout, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Open blocked past timeout; context cancellation leaked")
	}

	if got := gates.Status("G-1").State; got != StateClosed {
		t.Fatalf("after timeout: state = %s, want closed", got)
	}

	// Fault must be recorded for the recovery.
	if got := gates.Status("G-1").Faults; got != 1 {
		t.Fatalf("faults = %d, want 1", got)
	}
}

// TestCloseTimeoutRecoversToClosed mirrors the open path for Close.
func TestCloseTimeoutRecoversToClosed(t *testing.T) {
	sensor := newMockSensor()
	c, gates, _ := newTestController(sensor, 20*time.Millisecond)

	// Reach StateOpen first.
	sensor.openRelease <- struct{}{}
	if err := c.Open("G-1"); err != nil {
		t.Fatalf("setup Open: %v", err)
	}

	// ConfirmClosed is never released, so Close times out.
	err := c.Close("G-1")
	if !errors.Is(err, ErrSensorTimeout) {
		t.Fatalf("Close: want ErrSensorTimeout, got %v", err)
	}

	if got := gates.Status("G-1").State; got != StateClosed {
		t.Fatalf("after timeout: state = %s, want closed", got)
	}
}

// TestOpenActuatorFailureRecoversToClosed ensures an actuator error also
// unwedges the gate rather than leaving it in StateOpening.
func TestOpenActuatorFailureRecoversToClosed(t *testing.T) {
	sensor := newMockSensor()
	sensor.actuatorErr = errors.New("actuator offline")
	c, gates, _ := newTestController(sensor, 20*time.Millisecond)

	err := c.Open("G-1")
	if err == nil {
		t.Fatal("Open: want error, got nil")
	}

	if got := gates.Status("G-1").State; got != StateClosed {
		t.Fatalf("after actuator failure: state = %s, want closed", got)
	}
}

// TestNormalOpenCloseStillWorks is a regression guard: the happy path must
// remain unaffected by the recovery plumbing.
func TestNormalOpenCloseStillWorks(t *testing.T) {
	sensor := newMockSensor()
	c, gates, _ := newTestController(sensor, time.Second)

	sensor.openRelease <- struct{}{}
	if err := c.Open("G-1"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := gates.Status("G-1").State; got != StateOpen {
		t.Fatalf("state = %s, want open", got)
	}

	sensor.closedRelease <- struct{}{}
	if err := c.Close("G-1"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := gates.Status("G-1").State; got != StateClosed {
		t.Fatalf("state = %s, want closed", got)
	}
}
