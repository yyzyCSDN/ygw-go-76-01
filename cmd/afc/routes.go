package main

import "net/http"

func NewMux(app *Application) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", app.handlers.Health)
	mux.HandleFunc("/api/gates", app.handlers.ListGates)
	mux.HandleFunc("/api/swipe", app.handlers.Swipe)
	mux.HandleFunc("/api/rules/upgrade", app.handlers.UpgradeRule)
	mux.HandleFunc("/api/rules", app.handlers.ListRules)
	mux.HandleFunc("/api/schedule", app.handlers.ListSchedule)
	mux.HandleFunc("/api/schedule/add", app.handlers.AddSchedule)
	mux.HandleFunc("/api/heartbeat", app.handlers.IngestHeartbeat)
	mux.HandleFunc("/api/alarms", app.handlers.ListAlarms)
	mux.HandleFunc("/api/stats", app.handlers.ListStats)
	mux.HandleFunc("/api/tickets", app.handlers.ListTickets)
	mux.HandleFunc("/api/reports", app.handlers.ListReports)
	mux.HandleFunc("/", app.handlers.Console)
	return mux
}
