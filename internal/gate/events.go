package gate

import (
	"fmt"
	"time"

	"afcgatecontrol/internal/dedup"
)

type Event struct {
	GateID      string
	Kind        string
	Payload     string
	At          int64
	Fingerprint string
}

type EventRecorder struct {
	store *EventFileStore
}

func NewEventRecorder(store *EventFileStore) *EventRecorder {
	return &EventRecorder{store: store}
}

func (r *EventRecorder) Record(gateID string, kind string, payload string) error {
	if r.store == nil {
		return nil
	}
	at := time.Now().Unix()
	event := Event{
		GateID:      gateID,
		Kind:        kind,
		Payload:     payload,
		At:          at,
		Fingerprint: dedup.EventFingerprint(gateID, kind, time.Unix(at, 0)),
	}
	line := fmt.Sprintf("%d\t%s\t%s\t%s\t%s\n", event.At, event.GateID, event.Kind, event.Payload, event.Fingerprint)
	handle, err := r.store.openForAppend(event.GateID)
	if err != nil {
		return err
	}
	return r.store.appendLine(handle, line)
}

func (r *EventRecorder) ActiveHandles() int {
	if r.store == nil {
		return 0
	}
	return r.store.ActiveHandles()
}

func (r *EventRecorder) Close() error {
	if r.store == nil {
		return nil
	}
	return r.store.Close()
}
