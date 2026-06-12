// Package timer implements the timer plugin — cron-like scheduler
// that submits ProactiveIntents on a recurring schedule.
package timer

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/shirohania/sion/plugin/sdk"
)

// Plugin implements sdk.Plugin for the timer module.
type Plugin struct {
	sdk.BasePlugin
	pctx     *sdk.PluginContext
	mu       sync.Mutex
	stopCh   chan struct{}
	schedules []Schedule
}

// Schedule defines a recurring timer event.
type Schedule struct {
	Name     string        `json:"name"`
	CronExpr string        `json:"cron_expr"` // simple: "every 30m", "at 09:00", "every 2h"
	Interval time.Duration `json:"interval"`  // parsed interval
	Action   string        `json:"action"`    // action name from ActionDef
	Message  string        `json:"message"`   // prompt for the LLM
	Priority int           `json:"priority"`
}

func New() *Plugin {
	return &Plugin{
		BasePlugin: sdk.NewBasePlugin(sdk.PluginInfo{
			Name:        "timer",
			Version:     "1.0.0",
			Description: "Cron-like scheduler for recurring reminders and check-ins",
			Author:      "Sion",
		}),
		stopCh: make(chan struct{}),
		schedules: []Schedule{
			{Name: "hydration", Interval: 2 * time.Hour, Action: "care_hydration",
				Message: "Remind the user to drink water.", Priority: 3},
			{Name: "stretch", Interval: 1*time.Hour + 30*time.Minute, Action: "care_rest",
				Message: "Suggest the user take a short break and stretch.", Priority: 3},
		},
	}
}

func (p *Plugin) AddSchedule(s Schedule) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.schedules = append(p.schedules, s)
}

func (p *Plugin) Init(ctx context.Context, pctx *sdk.PluginContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pctx = pctx
	return nil
}

func (p *Plugin) Start(ctx context.Context) error {
	go p.loop(ctx)
	log.Println("[timer] plugin started")
	return nil
}

func (p *Plugin) loop(ctx context.Context) {
	p.mu.Lock()
	schedules := make([]Schedule, len(p.schedules))
	copy(schedules, p.schedules)
	p.mu.Unlock()

	// Create a ticker per schedule
	timers := make(map[string]*time.Ticker)
	for _, s := range schedules {
		timers[s.Name] = time.NewTicker(s.Interval)
	}
	defer func() {
		for _, t := range timers {
			t.Stop()
		}
	}()

	// Build a single select loop
	for {
		for name, ticker := range timers {
			select {
			case <-p.stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.fire(ctx, name)
			default:
			}
		}
		time.Sleep(1 * time.Second)
	}
}

func (p *Plugin) fire(ctx context.Context, name string) {
	p.mu.Lock()
	var s *Schedule
	for i := range p.schedules {
		if p.schedules[i].Name == name {
			s = &p.schedules[i]
			break
		}
	}
	p.mu.Unlock()

	if s == nil || p.pctx.IntentSubmitter == nil {
		return
	}

	intent := sdk.ProactiveIntent{
		Source:      "plugin:timer",
		Action:      s.Action,
		Message:     s.Message,
		Priority:    s.Priority,
		CoalesceKey: "timer:" + s.Name,
	}
	if err := p.pctx.IntentSubmitter.Submit(intent); err != nil {
		log.Printf("[timer] submit %s: %v", s.Name, err)
	}
}

func (p *Plugin) Stop(ctx context.Context) error {
	close(p.stopCh)
	return nil
}

var _ sdk.Plugin = (*Plugin)(nil)
