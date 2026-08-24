package gate

import (
	"errors"
	"fmt"
	"sync"

	"afcgatecontrol/internal/passage"
)

type GateState int

const (
	StateClosed GateState = iota
	StateOpening
	StateOpen
	StateClosing
)

func (s GateState) String() string {
	switch s {
	case StateOpening:
		return "opening"
	case StateOpen:
		return "open"
	case StateClosing:
		return "closing"
	default:
		return "closed"
	}
}

var ErrIllegalState = errors.New("illegal gate state transition")

var allowedTransitions = map[GateState]map[GateState]bool{
	StateClosed: {
		StateOpening: true,
	},
	StateOpening: {
		StateOpen: true,
	},
	StateOpen: {
		StateClosing: true,
	},
	StateClosing: {
		StateClosed: true,
	},
}

type GateStatus struct {
	GateID       string
	State        GateState
	Online       bool
	RecordSeq    int64
	UpdatedAt    int64
	OfflineSince int64
	Faults       int
}

type GateStore struct {
	mu       sync.Mutex
	statuses map[string]*GateStatus
	seq      int64
}

func NewGateStore() *GateStore {
	return &GateStore{
		statuses: make(map[string]*GateStatus),
	}
}

func (s *GateStore) ensure(id string) *GateStatus {
	item, ok := s.statuses[id]
	if !ok {
		item = &GateStatus{
			GateID: id,
			State:  StateClosed,
			Online: true,
		}
		s.statuses[id] = item
	}
	return item
}

func (s *GateStore) Status(id string) GateStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.ensure(id)
}

func (s *GateStore) SetState(id string, state GateState, at int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.ensure(id)
	if !allowedTransitions[item.State][state] {
		return fmt.Errorf("%w: %s -> %s", ErrIllegalState, item.State, state)
	}
	item.State = state
	item.UpdatedAt = at
	s.seq++
	item.RecordSeq = s.seq
	return nil
}

// ForceState sets the gate state unconditionally, bypassing the normal
// transition rules. It is reserved for fault recovery: when a sensor
// confirmation times out the gate is stuck in StateOpening/StateClosing,
// which cannot move forward to StateOpen/StateClosed and would block the
// gate indefinitely. Forcing back to StateClosed makes the gate serviceable
// again. Normal callers should use SetState.
func (s *GateStore) ForceState(id string, state GateState, at int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.ensure(id)
	item.State = state
	item.UpdatedAt = at
	s.seq++
	item.RecordSeq = s.seq
	return nil
}

func (s *GateStore) ApplyPoll(id string, online bool, at int64) GateStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.ensure(id)
	if online {
		item.OfflineSince = 0
	} else if item.OfflineSince == 0 {
		item.OfflineSince = at
	}
	item.Online = online
	item.UpdatedAt = at
	s.seq++
	item.RecordSeq = s.seq
	return *item
}

func (s *GateStore) RecordFault(id string, at int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.ensure(id)
	item.Faults++
	item.UpdatedAt = at
	s.seq++
	item.RecordSeq = s.seq
	return nil
}

func (s *GateStore) WriteHeartbeat(id string, online bool, heartbeatSeq int64, at int64) GateStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.ensure(id)
	if at < item.UpdatedAt {
		return *item
	}
	if heartbeatSeq > 0 && item.RecordSeq > heartbeatSeq {
		return *item
	}
	if online {
		item.OfflineSince = 0
	} else if item.OfflineSince == 0 {
		item.OfflineSince = at
	}
	item.Online = online
	item.UpdatedAt = at
	s.seq++
	item.RecordSeq = s.seq
	return *item
}

func (s *GateStore) ApplyOffline(id string, at int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.ensure(id)
	item.Online = false
	if item.OfflineSince == 0 {
		item.OfflineSince = at
	}
	item.UpdatedAt = at
	s.seq++
	item.RecordSeq = s.seq
	return nil
}

func (s *GateStore) List() []GateStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]GateStatus, 0, len(s.statuses))
	for _, item := range s.statuses {
		out = append(out, *item)
	}
	return out
}

func (s *GateStore) IsOpen(id string) bool {
	return s.Status(id).State == StateOpen
}

var _ passage.StateReader = (*GateStore)(nil)
