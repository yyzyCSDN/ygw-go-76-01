package rule

import "sync"

type ChangeRecord struct {
	Seq      int
	GateGroup string
	Before   Rule
	After    Rule
	At       int64
}

type RuleAudit struct {
	mu      sync.Mutex
	seq     int
	records []ChangeRecord
}

func NewRuleAudit() *RuleAudit {
	return &RuleAudit{}
}

func (a *RuleAudit) Append(gateGroup string, before Rule, after Rule, at int64) ChangeRecord {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seq++
	record := ChangeRecord{
		Seq:       a.seq,
		GateGroup: gateGroup,
		Before:    before,
		After:     after,
		At:        at,
	}
	a.records = append(a.records, record)
	return record
}

func (a *RuleAudit) List() []ChangeRecord {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]ChangeRecord, len(a.records))
	copy(out, a.records)
	return out
}

func (a *RuleAudit) Count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.records)
}
