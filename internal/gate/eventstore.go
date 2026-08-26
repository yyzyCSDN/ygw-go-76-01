package gate

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type EventFileStore struct {
	dir     string
	mu      sync.Mutex
	handles map[string]*os.File
}

func NewEventFileStore(dir string) (*EventFileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create event dir: %w", err)
	}
	return &EventFileStore{
		dir:     dir,
		handles: make(map[string]*os.File),
	}, nil
}

func (s *EventFileStore) openForAppend(gateID string) (*os.File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.dir, gateID+".events")
	handle, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open event file %s: %w", path, err)
	}
	s.handles[gateID] = handle
	return handle, nil
}

func (s *EventFileStore) appendLine(handle *os.File, line string) error {
	if _, err := handle.WriteString(line); err != nil {
		return fmt.Errorf("append event line: %w", err)
	}
	return nil
}

func (s *EventFileStore) ActiveHandles() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.handles)
}

func (s *EventFileStore) Events(gateID string) ([]Event, error) {
	s.mu.Lock()
	handle, ok := s.handles[gateID]
	s.mu.Unlock()
	if ok {
		_ = handle.Sync()
	}
	path := filepath.Join(s.dir, gateID+".events")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read event file %s: %w", path, err)
	}
	out := make([]Event, 0)
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}
		at, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue
		}
		out = append(out, Event{
			GateID:      parts[1],
			Kind:        parts[2],
			Payload:     parts[3],
			Fingerprint: parts[4],
			At:          at,
		})
	}
	return out, nil
}

func (s *EventFileStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for gateID, handle := range s.handles {
		_ = handle.Close()
		delete(s.handles, gateID)
	}
	return nil
}
