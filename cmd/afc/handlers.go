package main

import (
	"encoding/json"
	"net/http"
	"time"

	"afcgatecontrol/internal/alarm"
	"afcgatecontrol/internal/gate"
	"afcgatecontrol/internal/heartbeat"
	"afcgatecontrol/internal/ticket"
)

type Handlers struct {
	app *Application
}

func NewHandlers(app *Application) *Handlers {
	return &Handlers{app: app}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func readJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return false
	}
	return true
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.app.Health())
}

type GateView struct {
	ID           string
	Name         string
	Location     string
	ChannelID    string
	State        string
	Online       bool
	RecordSeq    int64
	OfflineSince int64
	Faults       int
	SessionPassed int
	TotalPassed  int
}

func (h *Handlers) ListGates(w http.ResponseWriter, r *http.Request) {
	statuses := h.app.gates.List()
	views := make([]GateView, 0, len(statuses))
	for _, status := range statuses {
		info, _ := h.app.registry.Get(status.GateID)
		views = append(views, GateView{
			ID:            status.GateID,
			Name:          info.Name,
			Location:      info.Location,
			ChannelID:     info.ChannelID,
			State:         h.app.controller.State(status.GateID).String(),
			Online:        status.Online,
			RecordSeq:     status.RecordSeq,
			OfflineSince:  status.OfflineSince,
			Faults:        status.Faults,
			SessionPassed: h.app.controller.Counter().SessionCount(status.GateID),
			TotalPassed:   h.app.controller.Counter().Total(status.GateID),
		})
	}
	writeJSON(w, http.StatusOK, views)
}

type SwipeRequest struct {
	TicketNo  string
	Action    string
	GateID    string
	GateGroup string
}

func (h *Handlers) Swipe(w http.ResponseWriter, r *http.Request) {
	var request SwipeRequest
	if !readJSON(w, r, &request) {
		return
	}
	if request.TicketNo == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ticketNo is required"})
		return
	}
	if request.GateID == "" {
		request.GateID = "G-DEFAULT"
	}
	gateGroup := request.GateGroup
	if gateGroup == "" {
		gateGroup = h.app.registry.GroupOf(request.GateID, h.app.controller.DefaultGroup())
	}
	switch request.Action {
	case "entry":
		outcome, err := h.app.controller.HandleEntry(request.TicketNo, gateGroup, request.GateID)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		if !outcome.Allowed {
			writeJSON(w, http.StatusConflict, map[string]string{"error": outcome.Reason})
			return
		}
		if err := h.app.controller.Open(request.GateID); err != nil {
			h.app.alarms.Raise(request.GateID, "open-failed", alarm.SeverityWarning)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}
		h.app.stats.Record("CH-"+request.GateID, 1)
		writeJSON(w, http.StatusOK, map[string]string{"message": "entry allowed", "ticketNo": request.TicketNo})
	case "exit":
		outcome, err := h.app.controller.HandleExit(request.TicketNo, request.GateID)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		if !outcome.Completed {
			writeJSON(w, http.StatusConflict, map[string]string{"error": outcome.Reason})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "exit completed", "ticketNo": request.TicketNo})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be entry or exit"})
	}
}

type UpgradeRequest struct {
	GateGroup        string
	SingleAllowed    bool
	TransferAllowed  bool
}

func (h *Handlers) UpgradeRule(w http.ResponseWriter, r *http.Request) {
	var request UpgradeRequest
	if !readJSON(w, r, &request) {
		return
	}
	if request.GateGroup == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "gateGroup is required"})
		return
	}
	stored, err := h.app.rules.Upgrade(request.GateGroup, request.SingleAllowed, request.TransferAllowed)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	before, _ := h.app.rules.Get(request.GateGroup)
	h.app.audit.Append(request.GateGroup, before, stored, time.Now().Unix())
	writeJSON(w, http.StatusOK, map[string]string{
		"message":   "rule upgraded",
		"gateGroup": stored.GateGroup,
		"version":   itoa(stored.Version),
	})
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

type HeartbeatRequest struct {
	GateID string
	Online bool
	Seq    int64
	At     int64
}

func (h *Handlers) IngestHeartbeat(w http.ResponseWriter, r *http.Request) {
	var request HeartbeatRequest
	if !readJSON(w, r, &request) {
		return
	}
	if request.GateID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "gateId is required"})
		return
	}
	if request.At == 0 {
		request.At = time.Now().Unix()
	}
	h.app.monitor.Ingest(request.GateID, request.At)
	status := h.app.monitor.ApplyHeartbeat(heartbeat.HeartbeatSnapshot{
		GateID: request.GateID,
		Online: request.Online,
		Seq:    request.Seq,
		At:     request.At,
	})
	if status.Online {
		_, _ = h.app.alarms.Resolve(request.GateID, "heartbeat-timeout")
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"message":  "heartbeat accepted",
		"gateId":   request.GateID,
		"online":   boolString(status.Online),
		"recordSeq": int64String(status.RecordSeq),
	})
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func int64String(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	digits := make([]byte, 0, 20)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

func (h *Handlers) ListAlarms(w http.ResponseWriter, r *http.Request) {
	type alarmView struct {
		ID         string
		GateID     string
		Kind       string
		Severity   string
		Active     bool
		RaisedAt   int64
		ResolvedAt int64
	}
	active := h.app.alarms.Active("")
	views := make([]alarmView, 0, len(active))
	for _, item := range active {
		views = append(views, alarmView{
			ID:         item.ID,
			GateID:     item.GateID,
			Kind:       item.Kind,
			Severity:   item.Severity.String(),
			Active:     item.Active,
			RaisedAt:   item.RaisedAt,
			ResolvedAt: item.ResolvedAt,
		})
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handlers) ListRules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"rules":          h.app.rules.List(),
		"audit":          h.app.audit.List(),
		"validatorRules": h.app.validator.SnapshotRules(),
	})
}

type ScheduleRequest struct {
	GateID  string
	OpenAt  int64
	CloseAt int64
}

func (h *Handlers) ListSchedule(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.app.scheduler.Entries())
}

func (h *Handlers) AddSchedule(w http.ResponseWriter, r *http.Request) {
	var request ScheduleRequest
	if !readJSON(w, r, &request) {
		return
	}
	if request.GateID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "gateId is required"})
		return
	}
	h.app.scheduler.Add(gate.ScheduleEntry{
		GateID:  request.GateID,
		OpenAt:  request.OpenAt,
		CloseAt: request.CloseAt,
	})
	writeJSON(w, http.StatusOK, map[string]string{"message": "schedule added", "gateId": request.GateID})
}

func (h *Handlers) ListTickets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"report":  h.app.tickets.Report(),
		"entered": h.app.tickets.ListByStatus(ticket.StatusEntered),
		"recent":  h.app.tickets.Recent(5),
	})
}

func (h *Handlers) ListReports(w http.ResponseWriter, r *http.Request) {
	pairs := make(map[string]string)
	for _, info := range h.app.registry.List() {
		pairs[info.ID] = info.ChannelID
	}
	restoredID, restoredSeq := h.app.recovery.LastRestored()
	writeJSON(w, http.StatusOK, map[string]any{
		"reports": h.app.reports.BuildAll(pairs),
		"lastRestored": map[string]any{
			"id":  restoredID,
			"seq": restoredSeq,
		},
	})
}

func (h *Handlers) ListStats(w http.ResponseWriter, r *http.Request) {
	type channelStat struct {
		ChannelID      string
		Total          int64
		Counted        int64
		SnapshotSeq    int64
		SnapshotTotal  int64
		SnapshotCounted int64
		LastRecovered  int64
	}
	lastRecovered := h.app.stats.LastRecoveredSeq()
	rows := make([]channelStat, 0)
	for _, info := range h.app.registry.List() {
		channelID := h.app.registry.ChannelOf(info.ID)
		row := channelStat{
			ChannelID:     channelID,
			Total:         h.app.stats.Total(channelID),
			Counted:       h.app.stats.Counted(channelID),
			LastRecovered: lastRecovered,
		}
		if snap, ok := h.app.channels.SnapshotOf(channelID); ok {
			row.SnapshotSeq = snap.Seq
			row.SnapshotTotal = snap.Total
			row.SnapshotCounted = snap.Counted
		}
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handlers) Console(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(h.app.console)
}
