package rule

import (
	"fmt"
	"sort"
	"sync"
)

const (
	DefaultGateGroup = "NORMAL"
	TransferGateGroup = "TRANSFER"
)

type Rule struct {
	GateGroup        string
	SingleAllowed    bool
	TransferAllowed  bool
	Version          int
}

func (r Rule) Allows(transferable bool) bool {
	if transferable {
		return r.TransferAllowed
	}
	return r.SingleAllowed
}

type RuleStore struct {
	mu    sync.RWMutex
	rules map[string]Rule
	seq   int
}

func NewRuleStore() *RuleStore {
	return &RuleStore{
		rules: map[string]Rule{
			DefaultGateGroup: {
				GateGroup:       DefaultGateGroup,
				SingleAllowed:   true,
				TransferAllowed: true,
				Version:         1,
			},
			TransferGateGroup: {
				GateGroup:       TransferGateGroup,
				SingleAllowed:   true,
				TransferAllowed: true,
				Version:         1,
			},
		},
		seq: 2,
	}
}

func (s *RuleStore) Get(gateGroup string) (Rule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.rules[gateGroup]
	return item, ok
}

func (s *RuleStore) Set(rule Rule) Rule {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	rule.Version = s.seq
	s.rules[rule.GateGroup] = rule
	return rule
}

func (s *RuleStore) Snapshot() map[string]Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Rule, len(s.rules))
	for key, value := range s.rules {
		out[key] = value
	}
	return out
}

func (s *RuleStore) List() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Rule, 0, len(s.rules))
	for _, value := range s.rules {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].GateGroup < out[j].GateGroup
	})
	return out
}

type RuleManager struct {
	store    *RuleStore
	notifier *ChangeNotifier
}

func NewRuleManager(store *RuleStore) *RuleManager {
	return &RuleManager{
		store:    store,
		notifier: NewChangeNotifier(),
	}
}

func (m *RuleManager) Notifier() *ChangeNotifier {
	return m.notifier
}

func (m *RuleManager) Get(gateGroup string) (Rule, bool) {
	return m.store.Get(gateGroup)
}

func (m *RuleManager) Snapshot() map[string]Rule {
	return m.store.Snapshot()
}

func (m *RuleManager) List() []Rule {
	return m.store.List()
}

func (m *RuleManager) Upgrade(gateGroup string, singleAllowed bool, transferAllowed bool) (Rule, error) {
	if _, ok := m.store.Get(gateGroup); !ok {
		return Rule{}, fmt.Errorf("unknown gate group %q", gateGroup)
	}
	next := Rule{
		GateGroup:       gateGroup,
		SingleAllowed:   singleAllowed,
		TransferAllowed: transferAllowed,
	}
	stored := m.store.Set(next)
	m.notifier.Broadcast(stored.GateGroup, stored)
	return stored, nil
}

func (m *RuleManager) UpgradeRule(gateGroup string, next Rule) (Rule, error) {
	if _, ok := m.store.Get(gateGroup); !ok {
		return Rule{}, fmt.Errorf("unknown gate group %q", gateGroup)
	}
	next.GateGroup = gateGroup
	stored := m.store.Set(next)
	m.notifier.Broadcast(stored.GateGroup, stored)
	return stored, nil
}
