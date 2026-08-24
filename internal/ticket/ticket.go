package ticket

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"afcgatecontrol/internal/dedup"
	"afcgatecontrol/internal/rule"
)

var (
	ErrTicketNotFound = errors.New("ticket record not found")
	ErrTicketUsed     = errors.New("ticket already used")
	ErrTicketInUse    = errors.New("ticket is still in a journey")
	ErrDuplicateSwipe = errors.New("duplicate swipe rejected")
	ErrRuleDenied     = errors.New("ticket type is not allowed at this gate group")
	ErrStoreClosed    = errors.New("ticket store is closed")
)

type TicketStatus int

const (
	StatusUnused TicketStatus = iota
	StatusEntered
	StatusExited
)

func (s TicketStatus) String() string {
	switch s {
	case StatusEntered:
		return "entered"
	case StatusExited:
		return "exited"
	default:
		return "unused"
	}
}

type TicketRecord struct {
	ID           string
	TicketType   string
	Transferable bool
	EntryStation string
	ExitStation  string
	Status       TicketStatus
	EnteredAt    int64
	ExitedAt     int64
}

type EntryDecision struct {
	TicketID   string
	GateGroup  string
	GateID     string
	RuleVersion int
	EnteredAt  int64
}

type ExitDecision struct {
	TicketID  string
	GateID    string
	ExitedAt  int64
}

type EntryOutcome struct {
	Allowed   bool
	Decision  *EntryDecision
	Reason    string
}

type ExitOutcome struct {
	Completed bool
	Decision  *ExitDecision
	Reason    string
}

type TicketStore struct {
	mu      sync.RWMutex
	dir     string
	closed  bool
	records map[string]*TicketRecord
}

func NewTicketStore(dir string) (*TicketStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create ticket store dir: %w", err)
	}
	store := &TicketStore{
		dir:     dir,
		records: make(map[string]*TicketRecord),
	}
	if err := store.loadAll(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *TicketStore) loadAll() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("read ticket store dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read ticket record %s: %w", path, err)
		}
		var rec TicketRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return fmt.Errorf("decode ticket record %s: %w", path, err)
		}
		s.records[rec.ID] = &rec
	}
	return nil
}

func (s *TicketStore) Lookup(id string) (*TicketRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[id]
	if !ok {
		return nil, false
	}
	copy := *rec
	return &copy, true
}

func (s *TicketStore) Put(rec *TicketRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStoreClosed
	}
	copy := *rec
	s.records[rec.ID] = &copy
	return s.persist(&copy)
}

func (s *TicketStore) MarkEntered(id string, station string, at int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStoreClosed
	}
	rec, ok := s.records[id]
	if !ok {
		return ErrTicketNotFound
	}
	rec.Status = StatusEntered
	rec.EntryStation = station
	rec.EnteredAt = at
	return s.persist(rec)
}

func (s *TicketStore) MarkExited(id string, station string, at int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStoreClosed
	}
	rec, ok := s.records[id]
	if !ok {
		return ErrTicketNotFound
	}
	rec.Status = StatusExited
	rec.ExitStation = station
	rec.ExitedAt = at
	return s.persist(rec)
}

func (s *TicketStore) ResetEntry(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[id]
	if !ok {
		return
	}
	rec.Status = StatusUnused
	rec.EntryStation = ""
	rec.EnteredAt = 0
	if !s.closed {
		_ = s.persist(rec)
	}
}

func (s *TicketStore) persist(rec *TicketRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("encode ticket record: %w", err)
	}
	path := filepath.Join(s.dir, rec.ID+".json")
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return fmt.Errorf("write ticket record: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("commit ticket record: %w", err)
	}
	return nil
}

func (s *TicketStore) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}

func (s *TicketStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

type RuleCache struct {
	mgr      *rule.RuleManager
	mu       sync.RWMutex
	entries  map[string]rule.Rule
	unsub    func()
}

func NewRuleCache(mgr *rule.RuleManager) *RuleCache {
	cache := &RuleCache{
		mgr:     mgr,
		entries: mgr.Snapshot(),
	}
	cache.unsub = mgr.Notifier().Subscribe(cache.onRuleChanged)
	return cache
}

func (c *RuleCache) onRuleChanged(gateGroup string, current rule.Rule) {
	c.mu.Lock()
	c.entries[gateGroup] = current
	c.mu.Unlock()
}

func (c *RuleCache) Get(gateGroup string) (rule.Rule, bool) {
	c.mu.RLock()
	item, ok := c.entries[gateGroup]
	c.mu.RUnlock()
	if !ok {
		live, liveOK := c.mgr.Get(gateGroup)
		if liveOK {
			c.mu.Lock()
			c.entries[gateGroup] = live
			c.mu.Unlock()
			return live, true
		}
	}
	return item, ok
}

func (c *RuleCache) Close() {
	if c.unsub != nil {
		c.unsub()
		c.unsub = nil
	}
}

type Validator struct {
	store *TicketStore
	rules *RuleCache
	dedup *dedup.DedupManager
	now   func() time.Time
}

func NewValidator(store *TicketStore, mgr *rule.RuleManager, dm *dedup.DedupManager) *Validator {
	return &Validator{
		store: store,
		rules: NewRuleCache(mgr),
		dedup: dm,
		now:   time.Now,
	}
}

func (v *Validator) validateEntry(rec *TicketRecord, gateGroup string, gateID string) (*EntryOutcome, error) {
	if rec == nil {
		return &EntryOutcome{Allowed: false, Reason: "ticket record missing"}, ErrTicketNotFound
	}
	if rec.ID == "" {
		return &EntryOutcome{Allowed: false, Reason: "ticket id empty"}, ErrTicketNotFound
	}
	switch rec.Status {
	case StatusExited:
		return nil, ErrTicketUsed
	case StatusEntered:
		return nil, ErrTicketInUse
	}
	key := dedup.SwipeKey(rec.ID)
	if v.dedup.Check(key) {
		return nil, ErrDuplicateSwipe
	}
	current, ok := v.rules.Get(gateGroup)
	if !ok {
		current, _ = v.rules.Get(rule.DefaultGateGroup)
	}
	if !current.Allows(rec.Transferable) {
		return &EntryOutcome{Allowed: false, Reason: "rule denied"}, ErrRuleDenied
	}
	at := v.now().Unix()
	if err := v.store.MarkEntered(rec.ID, gateID, at); err != nil {
		return nil, err
	}
	v.dedup.Set(key)
	decision := &EntryDecision{
		TicketID:    rec.ID,
		GateGroup:   gateGroup,
		GateID:      gateID,
		RuleVersion: current.Version,
		EnteredAt:   at,
	}
	return &EntryOutcome{Allowed: true, Decision: decision}, nil
}

func (v *Validator) validateExit(rec *TicketRecord, gateID string) (*ExitOutcome, error) {
	if rec == nil {
		return nil, ErrTicketNotFound
	}
	if rec.Status != StatusEntered {
		return nil, ErrTicketInUse
	}
	at := v.now().Unix()
	if err := v.store.MarkExited(rec.ID, gateID, at); err != nil {
		v.dedup.Clear(dedup.SwipeKey(rec.ID))
		return nil, err
	}
	confirmed, ok := v.store.Lookup(rec.ID)
	if !ok || confirmed.Status != StatusExited {
		v.dedup.Clear(dedup.SwipeKey(rec.ID))
		return nil, ErrStoreClosed
	}
	decision := &ExitDecision{
		TicketID: rec.ID,
		GateID:   gateID,
		ExitedAt: at,
	}
	return &ExitOutcome{Completed: true, Decision: decision}, nil
}

func (v *Validator) SnapshotRules() []rule.Rule {
	list := make([]rule.Rule, 0, len(v.rules.entries))
	v.rules.mu.RLock()
	for _, item := range v.rules.entries {
		list = append(list, item)
	}
	v.rules.mu.RUnlock()
	sort.Slice(list, func(i, j int) bool {
		return list[i].GateGroup < list[j].GateGroup
	})
	return list
}

func (v *Validator) Close() {
	v.rules.Close()
}
