package gate

import (
	"context"
	"fmt"
	"time"

	"afcgatecontrol/internal/alarm"
	"afcgatecontrol/internal/dedup"
	"afcgatecontrol/internal/passage"
	"afcgatecontrol/internal/ticket"
)

type Controller struct {
	gates        *GateStore
	tickets      *ticket.TicketStore
	validator    *ticket.Validator
	entryFlow    *ticket.EntryFlow
	exitFlow     *ticket.ExitFlow
	counter      *passage.PassageCounter
	recorder     *EventRecorder
	sessions     *SessionLedger
	channels     *ChannelRegistry
	dedups       *dedup.DedupManager
	alarms       *alarm.Manager
	sensors      SensorProvider
	openTimeout  time.Duration
	defaultGroup string
}

type SensorProvider func(gateID string) Sensor

func NewController(
	gates *GateStore,
	tickets *ticket.TicketStore,
	validator *ticket.Validator,
	counter *passage.PassageCounter,
	recorder *EventRecorder,
	sessions *SessionLedger,
	channels *ChannelRegistry,
	dedups *dedup.DedupManager,
	alarms *alarm.Manager,
	sensors SensorProvider,
	openTimeout time.Duration,
) *Controller {
	counter.SetStateReader(gates)
	return &Controller{
		gates:        gates,
		tickets:      tickets,
		validator:    validator,
		entryFlow:    ticket.NewEntryFlow(validator),
		exitFlow:     ticket.NewExitFlow(validator),
		counter:      counter,
		recorder:     recorder,
		sessions:     sessions,
		channels:     channels,
		dedups:       dedups,
		alarms:       alarms,
		sensors:      sensors,
		openTimeout:  openTimeout,
		defaultGroup: "NORMAL",
	}
}

func (c *Controller) sensorFor(gateID string) Sensor {
	if c.sensors != nil {
		sensor := c.sensors(gateID)
		if sensor != nil {
			return sensor
		}
	}
	return NewDirectSensor()
}

func (c *Controller) HandleEntry(ticketID string, gateGroup string, gateID string) (*ticket.EntryOutcome, error) {
	rec, ok := c.tickets.Lookup(ticketID)
	if !ok {
		return nil, ticket.ErrTicketNotFound
	}
	outcome, err := c.entryFlow.Run(rec, gateGroup, gateID)
	if err != nil {
		c.alarms.Raise(gateID, "entry-rejected", alarm.SeverityInfo)
		return nil, err
	}
	_ = c.recorder.Record(gateID, "entry", ticketID)
	return outcome, nil
}

func (c *Controller) HandleExit(ticketID string, gateID string) (*ticket.ExitOutcome, error) {
	rec, ok := c.tickets.Lookup(ticketID)
	if !ok {
		return nil, ticket.ErrTicketNotFound
	}
	outcome, err := c.exitFlow.Run(rec, gateID)
	if err != nil {
		return nil, err
	}
	_ = c.recorder.Record(gateID, "exit", ticketID)
	return outcome, nil
}

func (c *Controller) Open(gateID string) error {
	sensor := c.sensorFor(gateID)
	at := time.Now().Unix()
	if err := c.gates.SetState(gateID, StateOpening, at); err != nil {
		return err
	}
	ctx, cancel := withTimeout(context.Background(), c.openTimeout)
	defer cancel()
	if err := sensor.Open(); err != nil {
		_ = c.gates.RecoverToClosed(gateID, time.Now().Unix())
		_ = c.gates.RecordFault(gateID, time.Now().Unix())
		c.alarms.Raise(gateID, "open-failed", alarm.SeverityWarning)
		_ = c.recorder.Record(gateID, "open-failed", err.Error())
		return fmt.Errorf("open actuator failed: %w", err)
	}
	if err := waitConfirmation(ctx, sensor, true); err != nil {
		_ = c.gates.RecoverToClosed(gateID, time.Now().Unix())
		_ = c.gates.RecordFault(gateID, time.Now().Unix())
		c.alarms.Raise(gateID, "open-timeout", alarm.SeverityWarning)
		return err
	}
	if err := c.gates.SetState(gateID, StateOpen, time.Now().Unix()); err != nil {
		return err
	}
	if !c.gates.IsOpen(gateID) {
		_ = c.gates.RecoverToClosed(gateID, time.Now().Unix())
		return fmt.Errorf("gate %s did not reach the open state", gateID)
	}
	c.counter.Count(gateID)
	c.counter.Confirm(gateID)
	_ = c.recorder.Record(gateID, "open", "")
	return nil
}

func (c *Controller) Close(gateID string) error {
	sensor := c.sensorFor(gateID)
	at := time.Now().Unix()
	if err := c.gates.SetState(gateID, StateClosing, at); err != nil {
		return err
	}
	ctx, cancel := withTimeout(context.Background(), c.openTimeout)
	defer cancel()
	if err := sensor.Close(); err != nil {
		_ = c.gates.RecoverToClosed(gateID, time.Now().Unix())
		_ = c.gates.RecordFault(gateID, time.Now().Unix())
		c.alarms.Raise(gateID, "close-failed", alarm.SeverityWarning)
		_ = c.recorder.Record(gateID, "close-failed", err.Error())
		return fmt.Errorf("close actuator failed: %w", err)
	}
	if err := waitConfirmation(ctx, sensor, false); err != nil {
		_ = c.gates.RecoverToClosed(gateID, time.Now().Unix())
		_ = c.gates.RecordFault(gateID, time.Now().Unix())
		c.alarms.Raise(gateID, "close-timeout", alarm.SeverityWarning)
		return err
	}
	if err := c.gates.SetState(gateID, StateClosed, time.Now().Unix()); err != nil {
		return err
	}
	_ = c.recorder.Record(gateID, "close", "")
	return nil
}

func (c *Controller) State(gateID string) GateState {
	return c.gates.Status(gateID).State
}

func (c *Controller) Counter() *passage.PassageCounter {
	return c.counter
}

func (c *Controller) DefaultGroup() string {
	return c.defaultGroup
}
