package ticket

type EntryFlow struct {
	validator *Validator
}

func NewEntryFlow(validator *Validator) *EntryFlow {
	return &EntryFlow{validator: validator}
}

func (f *EntryFlow) Run(rec *TicketRecord, gateGroup string, gateID string) (*EntryOutcome, error) {
	return f.validator.validateEntry(rec, gateGroup, gateID)
}
