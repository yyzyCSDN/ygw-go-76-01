package gate_test

import (
	"fmt"
	"testing"

	"afcgatecontrol/internal/gate"
)

func TestGateEventHandleClosed(t *testing.T) {
	store, err := gate.NewEventFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	recorder := gate.NewEventRecorder(store)
	for i := 0; i < 5; i++ {
		if err := recorder.Record("G1", "pass", fmt.Sprintf("n%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if handles := store.ActiveHandles(); handles != 0 {
		t.Fatalf("event file handles must be closed after each write, open=%d", handles)
	}
}
