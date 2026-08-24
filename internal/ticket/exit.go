package ticket

type ExitFlow struct {
	validator *Validator
}

func NewExitFlow(validator *Validator) *ExitFlow {
	return &ExitFlow{validator: validator}
}

func (f *ExitFlow) Run(rec *TicketRecord, gateID string) (*ExitOutcome, error) {
	return f.validator.validateExit(rec, gateID)
}
