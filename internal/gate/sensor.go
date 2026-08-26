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

func waitConfirmation(ctx context.Context, sensor Sensor, open bool) error {
	var err error
	if open {
		err = sensor.ConfirmOpen(ctx)
	} else {
		err = sensor.ConfirmClosed(ctx)
	}
	if err == context.DeadlineExceeded {
		return ErrSensorTimeout
	}
	if err == context.Canceled {
		return ErrSensorTimeout
	}
	return err
}

func withTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}
