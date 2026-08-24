package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"afcgatecontrol/internal/alarm"
	"afcgatecontrol/internal/dedup"
	"afcgatecontrol/internal/gate"
	"afcgatecontrol/internal/heartbeat"
	"afcgatecontrol/internal/passage"
	"afcgatecontrol/internal/rule"
	"afcgatecontrol/internal/ticket"
)

type Application struct {
	cfg        Config
	rules      *rule.RuleManager
	audit      *rule.RuleAudit
	tickets    *ticket.TicketStore
	validator  *ticket.Validator
	dedups     *dedup.DedupManager
	gates      *gate.GateStore
	registry   *gate.GateRegistry
	scheduler  *gate.Scheduler
	counter    *passage.PassageCounter
	recovery   *passage.Recovery
	stats      *passage.StatsService
	reports    *passage.ReportGenerator
	sessions   *gate.SessionLedger
	channels   *gate.ChannelRegistry
	recorder   *gate.EventRecorder
	controller *gate.Controller
	poller     *gate.PollManager
	monitor    *heartbeat.Monitor
	alarms     *alarm.Manager
	handlers   *Handlers
	server     *http.Server
	console    []byte
}

func BuildApplication(cfg Config) (*Application, error) {
	rules := rule.NewRuleManager(rule.NewRuleStore())
	audit := rule.NewRuleAudit()
	tickets, err := ticket.NewTicketStore(filepath.Join(cfg.DataDir, "tickets"))
	if err != nil {
		return nil, err
	}
	events, err := gate.NewEventFileStore(filepath.Join(cfg.DataDir, "events"))
	if err != nil {
		return nil, err
	}
	dedups := dedup.NewDedupManager(5 * time.Minute)
	validator := ticket.NewValidator(tickets, rules, dedups)
	counter := passage.NewPassageCounter()
	recorder := gate.NewEventRecorder(events)
	sessions := gate.NewSessionLedger()
	channels := gate.NewChannelRegistry()
	alarms := alarm.NewManager()
	gates := gate.NewGateStore()
	registry := gate.NewGateRegistry()
	seedGates(registry)
	controller := gate.NewController(
		gates,
		tickets,
		validator,
		counter,
		recorder,
		sessions,
		channels,
		dedups,
		alarms,
		func(string) gate.Sensor {
			return gate.NewDirectSensor()
		},
		cfg.OpenTimeout,
	)
	poller := gate.NewPollManager(gates)
	monitor := heartbeat.NewMonitor(gates, alarms, cfg.HeartbeatTimeout)
	for _, info := range registry.List() {
		poller.SetDeviceState(info.ID, true)
		monitor.Ingest(info.ID, time.Now().Unix())
	}
	recovery := passage.NewRecovery(sessions, counter)
	stats := passage.NewStatsService(channels)
	reports := passage.NewReportGenerator(counter, stats)
	scheduler := gate.NewScheduler(gates)
	_, _ = recovery.Restore()
	_ = stats.Recover()
	for _, info := range registry.List() {
		channelID := registry.ChannelOf(info.ID)
		evs, _ := events.Events(info.ID)
		for _, ev := range evs {
			if ev.Kind == "entry" {
				stats.Replay(channelID, 1)
			}
		}
	}
	seedTickets(tickets)
	console, err := os.ReadFile(filepath.Join(cfg.WebDir, "console.html"))
	if err != nil {
		return nil, fmt.Errorf("read console page: %w", err)
	}
	app := &Application{
		cfg:        cfg,
		rules:      rules,
		audit:      audit,
		tickets:    tickets,
		validator:  validator,
		dedups:     dedups,
		gates:      gates,
		registry:   registry,
		scheduler:  scheduler,
		counter:    counter,
		recovery:   recovery,
		stats:      stats,
		reports:    reports,
		sessions:   sessions,
		channels:   channels,
		recorder:   recorder,
		controller: controller,
		poller:     poller,
		monitor:    monitor,
		alarms:     alarms,
		console:    console,
	}
	app.handlers = NewHandlers(app)
	app.server = &http.Server{
		Addr:              cfg.Listen,
		Handler:           NewMux(app),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return app, nil
}

func (a *Application) Server() *http.Server {
	return a.server
}

func (a *Application) RunBackground(ctx context.Context) {
	go a.poller.Run(ctx, a.cfg.PollInterval)
	go a.snapshotLoop(ctx)
	go a.heartbeatSweep(ctx)
	go a.dedupSweep(ctx)
	go a.scheduler.Run(ctx, a.cfg.PollInterval, a.controller.Open, a.controller.Close)
}

func seedGates(registry *gate.GateRegistry) {
	registry.Register(gate.GateInfo{ID: "G-NORTH", Name: "北进站闸机", Location: "北厅", Enabled: true})
	registry.Register(gate.GateInfo{ID: "G-SOUTH", Name: "南进站闸机", Location: "南厅", Enabled: true})
	registry.Register(gate.GateInfo{ID: "G-EDGE", Name: "边界换乘闸机", Location: "换乘口", Group: rule.TransferGateGroup, Enabled: true})
	registry.Register(gate.GateInfo{ID: "G-EAST", Name: "东出站闸机", Location: "东厅", Enabled: true})
}

func seedTickets(store *ticket.TicketStore) {
	_ = store.Put(&ticket.TicketRecord{ID: "T-0001", TicketType: "SINGLE", Transferable: false})
	_ = store.Put(&ticket.TicketRecord{ID: "T-0002", TicketType: "TRANSFER", Transferable: true})
	_ = store.Put(&ticket.TicketRecord{ID: "T-0003", TicketType: "SINGLE", Transferable: false})
}

func (a *Application) snapshotLoop(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.SnapshotInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			stamp := now.Unix()
			a.stats.FlushPending()
			a.sessions.Save(fmt.Sprintf("session-%d", stamp), a.counter.Snapshot(), stamp)
			a.counter.ResetSession()
			for _, status := range a.gates.List() {
				channelID := "CH-" + status.GateID
				a.channels.SnapshotChannel(channelID, a.stats.Counted(channelID), a.stats.Total(channelID), stamp)
			}
		}
	}
}

func (a *Application) heartbeatSweep(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			for _, status := range a.gates.List() {
				_ = a.monitor.Check(status.GateID, now.Unix())
			}
		}
	}
}

func (a *Application) dedupSweep(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.dedups.Sweep()
		}
	}
}

func (a *Application) Shutdown(ctx context.Context) error {
	if err := a.server.Shutdown(ctx); err != nil {
		return err
	}
	a.validator.Close()
	_ = a.tickets.Close()
	_ = a.recorder.Close()
	return nil
}
