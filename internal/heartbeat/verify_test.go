package heartbeat_test

import (
	"testing"

	"afcgatecontrol/internal/gate"
	"afcgatecontrol/internal/heartbeat"
)

func TestHeartbeatTimeoutNotSwallowed(t *testing.T) {
	store := gate.NewGateStore()
	monitor := heartbeat.NewMonitor(store, nil, 60)
	monitor.Ingest("G1", 1000)
	if err := monitor.Check("G1", 2000); err == nil {
		t.Fatal("heartbeat timeout must be reported")
	}
	if status := store.Status("G1"); status.Online {
		t.Fatal("a timed-out gate must not remain online")
	}
}
