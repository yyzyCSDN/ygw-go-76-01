package passage

import (
	"sync"
	"time"
)

type StateReader interface {
	IsOpen(gateID string) bool
}

type SessionSnapshot struct {
	ID      string
	Seq     int64
	Counts  map[string]int
	EndedAt int64
}

type SessionSource interface {
	Sessions() []SessionSnapshot
}

type ChannelSnapshot struct {
	ChannelID string
	Seq       int64
	Counted   int64
	Total     int64
	TakenAt   int64
}

type ChannelSource interface {
	ChannelSnapshots() []ChannelSnapshot
	LatestChannelSnapshot() ChannelSnapshot
	SnapshotOf(channelID string) (ChannelSnapshot, bool)
}

type PassageCounter struct {
	mu          sync.Mutex
	totals      map[string]int
	sessions    map[string]int
	confirmedAt map[string]int64
	reader      StateReader
}

func NewPassageCounter() *PassageCounter {
	return &PassageCounter{
		totals:      make(map[string]int),
		sessions:    make(map[string]int),
		confirmedAt: make(map[string]int64),
	}
}

func (c *PassageCounter) SetStateReader(reader StateReader) {
	c.mu.Lock()
	c.reader = reader
	c.mu.Unlock()
}

func (c *PassageCounter) Count(gateID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reader != nil && !c.reader.IsOpen(gateID) {
		return c.totals[gateID]
	}
	c.totals[gateID]++
	c.sessions[gateID]++
	return c.totals[gateID]
}

func (c *PassageCounter) Confirm(gateID string) {
	c.mu.Lock()
	c.confirmedAt[gateID] = time.Now().Unix()
	c.mu.Unlock()
}

func (c *PassageCounter) LastConfirmed(gateID string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.confirmedAt[gateID]
}

func (c *PassageCounter) Total(gateID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.totals[gateID]
}

func (c *PassageCounter) SessionCount(gateID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessions[gateID]
}

func (c *PassageCounter) Snapshot() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]int, len(c.totals))
	for key, value := range c.totals {
		out[key] = value
	}
	return out
}

func (c *PassageCounter) Restore(counts map[string]int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, value := range counts {
		c.totals[key] = value
	}
}

func (c *PassageCounter) ResetSession() {
	c.mu.Lock()
	c.sessions = make(map[string]int)
	c.mu.Unlock()
}

type Recovery struct {
	source          SessionSource
	counter         *PassageCounter
	lastRestoredID  string
	lastRestoredSeq int64
}

func NewRecovery(source SessionSource, counter *PassageCounter) *Recovery {
	return &Recovery{
		source:  source,
		counter: counter,
	}
}

func (r *Recovery) Restore() (SessionSnapshot, error) {
	sessions := r.source.Sessions()
	if len(sessions) == 0 {
		return SessionSnapshot{}, nil
	}
	// Recover from the newest session snapshot. The ledger appends snapshots
	// in save order, so picking sessions[0] restores the oldest cumulative
	// counts and leaves the totals lagging behind actual gate passage after a
	// restart. Select by Seq to always restore the latest state regardless of
	// slice ordering.
	latest := sessions[0]
	for _, snap := range sessions[1:] {
		if snap.Seq > latest.Seq {
			latest = snap
		}
	}
	r.counter.Restore(latest.Counts)
	r.lastRestoredID = latest.ID
	r.lastRestoredSeq = latest.Seq
	return latest, nil
}

func (r *Recovery) LastRestored() (string, int64) {
	return r.lastRestoredID, r.lastRestoredSeq
}

type StatsService struct {
	source           ChannelSource
	mu               sync.Mutex
	counted          map[string]int64
	totals           map[string]int64
	pending          map[string]int64
	lastRecoveredSeq int64
}

func NewStatsService(source ChannelSource) *StatsService {
	return &StatsService{
		source:  source,
		counted: make(map[string]int64),
		totals:  make(map[string]int64),
		pending: make(map[string]int64),
	}
}

func (s *StatsService) Recover() error {
	snap := s.source.LatestChannelSnapshot()
	if snap.ChannelID == "" {
		all := s.source.ChannelSnapshots()
		if len(all) == 0 {
			return nil
		}
		snap = all[len(all)-1]
	}
	s.mu.Lock()
	s.counted[snap.ChannelID] = snap.Counted
	s.totals[snap.ChannelID] = snap.Total
	s.lastRecoveredSeq = snap.Seq
	delete(s.pending, snap.ChannelID)
	s.mu.Unlock()
	return nil
}

func (s *StatsService) Record(channelID string, amount int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[channelID] += amount
}

func (s *StatsService) Replay(channelID string, amount int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.counted[channelID] > 0 {
		return
	}
	s.totals[channelID] += amount
}

func (s *StatsService) FlushPending() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for channelID, amount := range s.pending {
		if s.counted[channelID] > 0 {
			continue
		}
		s.totals[channelID] += amount
	}
	s.pending = make(map[string]int64)
}

func (s *StatsService) Total(channelID string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totals[channelID]
}

func (s *StatsService) Counted(channelID string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counted[channelID]
}

func (s *StatsService) LastRecoveredSeq() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastRecoveredSeq
}
