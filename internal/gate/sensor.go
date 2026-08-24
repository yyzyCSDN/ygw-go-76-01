package gate

import (
	"context"
	"errors"
	"time"
)

var ErrSensorTimeout = errors.New("sensor confirmation timed out")

type Sensor interface {
	Open() error
	Close() error
	ConfirmOpen(ctx context.Context) error
	ConfirmClosed(ctx context.Context) error
}

type DirectSensor struct{}

func NewDirectSensor() *DirectSensor {
	return &DirectSensor{}
}

func (s *DirectSensor) Open() error {
	return nil
}

func (s *DirectSensor) Close() error {
	return nil
}

func (s *DirectSensor) ConfirmOpen(ctx context.Context) error {
	return ctx.Err()
}

func (s *DirectSensor) ConfirmClosed(ctx context.Context) error {
	return ctx.Err()
}

// waitConfirmation waits for the sensor to confirm the gate reached the
// requested position. The sensor call is run in a separate goroutine and
// the caller's context is observed via a select, so a sensor that ignores
// its context (e.g. a hardware driver that blocks indefinitely) still
// returns when the deadline fires. The goroutine is abandoned if it is
// still running, but the caller never blocks past the timeout.
func waitConfirmation(ctx context.Context, sensor Sensor, open bool) error {
	type result struct{ err error }
	done := make(chan result, 1)
	go func() {
		var err error
		if open {
			err = sensor.ConfirmOpen(ctx)
		} else {
			err = sensor.ConfirmClosed(ctx)
		}
		done <- result{err}
	}()
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled {
			return ErrSensorTimeout
		}
		return ctx.Err()
	case r := <-done:
		switch r.err {
		case context.DeadlineExceeded, context.Canceled:
			return ErrSensorTimeout
		default:
			return r.err
		}
	}
}

func withTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}
