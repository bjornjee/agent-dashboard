// Package dispatch orchestrates post-spawn work that the dashboard owns
// rather than delegating to per-harness hooks. The plan injector is the
// only inhabitant today: it types codex's `/plan plan` slash command and
// the user's prompt into a freshly-spawned codex pane, because codex
// cannot enter plan mode from its model loop and its SessionStart hook
// cannot fire without a first input.
package dispatch

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/harness/codex"
	"github.com/bjornjee/agent-dashboard/internal/tmux"
)

const (
	defaultPreRoll       = 600 * time.Millisecond
	defaultDeadline      = 15 * time.Second
	defaultSweepInterval = 1 * time.Second
)

// PlanInjector schedules and reactively delivers codex prompts that
// require plan-mode bootstrap. One instance is created at dashboard boot
// and shared by the web and TUI dispatch sites.
//
// The lifecycle is: MaybeSchedule registers a pending entry and types
// `/plan plan` after a brief pre-roll. The state watcher then calls
// OnStateChange with the latest agents snapshot; when a pending pane
// reports permission_mode=="plan", the user's prompt is typed and the
// entry deleted. A background sweeper expires entries past the deadline
// so a missing or never-arriving plan-mode event surfaces as a logged
// timeout rather than a silent hang.
type PlanInjector struct {
	mu      sync.Mutex
	pending map[string]*pendingPrompt // key: tmux target like "main:5.1"

	// Injectable for tests.
	now      func() time.Time
	sleep    func(time.Duration)
	sendKeys func(target, text string) error

	preRoll       time.Duration
	deadline      time.Duration
	sweepInterval time.Duration
}

type pendingPrompt struct {
	target      string
	message     string
	scheduledAt time.Time
}

// NewPlanInjector constructs an injector with production defaults but
// does NOT start the sweeper goroutine — call Start(ctx) when ready.
// Tests configure test-only fields between New and Start to avoid racing
// the sweeper.
func NewPlanInjector() *PlanInjector {
	return &PlanInjector{
		pending:       make(map[string]*pendingPrompt),
		now:           time.Now,
		sleep:         time.Sleep,
		sendKeys:      tmux.TmuxSendKeys,
		preRoll:       defaultPreRoll,
		deadline:      defaultDeadline,
		sweepInterval: defaultSweepInterval,
	}
}

// Start launches the sweeper goroutine. It exits when ctx is cancelled.
func (p *PlanInjector) Start(ctx context.Context) {
	go p.sweep(ctx)
}

// MaybeSchedule registers a pending plan-mode injection for the freshly
// spawned codex pane at target. It is a no-op for non-codex harnesses or
// for skills that do not require plan mode (see codex.RequiresPlanMode).
//
// The call returns immediately. A goroutine waits the pre-roll, then
// types codex.PlanModeCommand into the pane; the actual user prompt is
// typed later by OnStateChange when codex reports plan mode.
func (p *PlanInjector) MaybeSchedule(harnessName, skill, target, message string) {
	if p == nil {
		return
	}
	if harnessName != "codex" || !codex.RequiresPlanMode(skill) {
		return
	}
	if target == "" {
		return
	}
	p.mu.Lock()
	p.pending[target] = &pendingPrompt{
		target:      target,
		message:     message,
		scheduledAt: p.now(),
	}
	p.mu.Unlock()

	go func() {
		p.sleep(p.preRoll)
		if err := p.sendKeys(target, codex.PlanModeCommand); err != nil {
			log.Printf("plan injector: send /plan plan to %s: %v", target, err)
		}
	}()
}

// OnStateChange is the watcher observer. For each pending pane that the
// snapshot reports in plan mode (and whose UpdatedAt is not stale
// relative to our scheduledAt), the user's prompt is typed and the
// entry deleted. send-keys failures keep the entry so the sweeper can
// expire it as a visible timeout.
func (p *PlanInjector) OnStateChange(agents []domain.Agent) {
	if p == nil {
		return
	}
	type fire struct {
		target  string
		message string
	}
	p.mu.Lock()
	var toFire []fire
	for _, a := range agents {
		if a.PermissionMode != "plan" {
			continue
		}
		if a.Target == "" {
			continue
		}
		pp, ok := p.pending[a.Target]
		if !ok {
			continue
		}
		if isStaleEvent(a.UpdatedAt, pp.scheduledAt) {
			continue
		}
		toFire = append(toFire, fire{a.Target, pp.message})
	}
	p.mu.Unlock()

	for _, f := range toFire {
		if err := p.sendKeys(f.target, f.message); err != nil {
			log.Printf("plan injector: send prompt to %s: %v", f.target, err)
			continue
		}
		p.mu.Lock()
		delete(p.pending, f.target)
		p.mu.Unlock()
	}
}

// peek is used by tests to inspect pending state. Not exported for
// production use — observers should call OnStateChange.
func (p *PlanInjector) peek(target string) (*pendingPrompt, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pp, ok := p.pending[target]
	return pp, ok
}

func (p *PlanInjector) sweep(ctx context.Context) {
	ticker := time.NewTicker(p.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := p.now()
			p.mu.Lock()
			for target, pp := range p.pending {
				if now.Sub(pp.scheduledAt) > p.deadline {
					log.Printf("plan injector: timeout for %s after %s", target, p.deadline)
					delete(p.pending, target)
				}
			}
			p.mu.Unlock()
		}
	}
}

// isStaleEvent reports whether an agent state event predates the
// pending entry's scheduledAt. A state file from a previous codex
// session at the same target (pane reuse) shows up here. If updatedAt
// cannot be parsed we err on the side of accepting the event — better
// a duplicate prompt than a silent miss.
func isStaleEvent(updatedAt string, scheduledAt time.Time) bool {
	if updatedAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		// fallback for RFC3339 without sub-second precision
		t, err = time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return false
		}
	}
	return t.Before(scheduledAt)
}
