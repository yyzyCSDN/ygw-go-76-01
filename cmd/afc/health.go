package main

import (
	"fmt"
	"time"
)

type HealthReport struct {
	Status         string
	GateCount      int
	ActiveAlarms   int
	AlarmSeverity  string
	MonitorOnline  int
	MonitorOffline int
	MonitorStale   int
	MonitorTimeout int
	OpenHandles    int
	TicketCount    int
	DedupEntries   int
	SessionCount   int
	ChannelCount   int
	CheckedAt      string
}

func (a *Application) Health() HealthReport {
	report := HealthReport{
		Status:         "ok",
		GateCount:      len(a.gates.List()),
		ActiveAlarms:   a.alarms.ActiveCount(),
		OpenHandles:    a.recorder.ActiveHandles(),
		TicketCount:    a.tickets.Count(),
		DedupEntries:   a.dedups.ActiveCount(),
		SessionCount:   a.sessions.Count(),
		ChannelCount:   a.channels.Count(),
		CheckedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	severity := a.alarms.CountBySeverity()
	report.AlarmSeverity = fmt.Sprintf("info=%d,warning=%d,critical=%d", severity.Info, severity.Warning, severity.Critical)
	summary := a.monitor.Summary(a.registry.EnabledIDs(), time.Now().Unix())
	report.MonitorOnline = summary.Online
	report.MonitorOffline = summary.Offline
	report.MonitorStale = summary.Stale
	report.MonitorTimeout = summary.TimeoutEvents
	if report.OpenHandles > 32 {
		report.Status = "degraded"
	}
	if report.ActiveAlarms > 8 {
		report.Status = "degraded"
	}
	return report
}

func (a *Application) HealthLine() string {
	report := a.Health()
	return fmt.Sprintf(
		"status=%s gates=%d alarms=%d handles=%d tickets=%d sessions=%d channels=%d",
		report.Status,
		report.GateCount,
		report.ActiveAlarms,
		report.OpenHandles,
		report.TicketCount,
		report.SessionCount,
		report.ChannelCount,
	)
}
