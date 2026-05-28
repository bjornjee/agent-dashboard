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
	sendKeys func(target, text string) error

	preRoll       time.Duration
	deadline      time.Duration
	sweepInterval time.Duration

	// stop is closed by Start when its ctx is cancelled. Pre-roll
	// goroutines select on it so they bail out on dashboard shutdown
	// instead of typing /plan plan into a pane the dashboard no longer
	// owns. Initialized in NewPlanInjector; never re-assigned.
	stop      chan struct{}
	startOnce sync.Once
}

type pendingPrompt struct {
	target      string
	message     string
	scheduledAt time.Time
	// gen monotonically increments each time MaybeSchedule is called for
	// the same target while a previous entry is still pending. The
	// pre-roll goroutine captures the gen it scheduled with and checks
	// against the current entry before typing — a stale gen means a
	// newer MaybeSchedule has superseded this one, so the goroutine
	// bails out instead of typing a redundant /plan plan.
	gen uint64
}

// NewPlanInjector constructs an injector with production defaults but
// does NOT start the sweeper goroutine — call Start(ctx) when ready.
// Tests configure test-only fields between New and Start to avoid racing
// the sweeper.
func NewPlanInjector() *PlanInjector {
	return &PlanInjector{
		pending:       make(map[string]*pendingPrompt),
		now:           time.Now,
		sendKeys:      tmux.TmuxSendKeys,
		preRoll:       defaultPreRoll,
		deadline:      defaultDeadline,
		sweepInterval: defaultSweepInterval,
		stop:          make(chan struct{}),
	}
}

// Start launches the sweeper goroutine and wires ctx cancellation into
// the injector's stop channel so in-flight pre-roll waits can bail out
// on shutdown. Idempotent — subsequent calls are no-ops.
func (p *PlanInjector) Start(ctx context.Context) {
	p.startOnce.Do(func() {
		go func() {
			<-ctx.Done()
			close(p.stop)
		}()
		go p.sweep(ctx)
	})
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
	gen := uint64(0)
	if existing, ok := p.pending[target]; ok {
		gen = existing.gen + 1
	}
	p.pending[target] = &pendingPrompt{
		target:      target,
		message:     message,
		scheduledAt: p.now(),
		gen:         gen,
	}
	p.mu.Unlock()

	go func() {
		// Wait the pre-roll, but bail immediately if the dashboard is
		// shutting down. p.stop is closed by Start's ctx-cancellation
		// watcher; never re-assigned, so the read is race-free.
		timer := time.NewTimer(p.preRoll)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-p.stop:
			return
		}

		// A newer MaybeSchedule for the same target supersedes us. Bail
		// instead of typing a redundant /plan plan — codex's plan-mode
		// transition is idempotent but a second slash command produces
		// a user-visible "already in plan mode" toast.
		p.mu.Lock()
		cur, ok := p.pending[target]
		stale := !ok || cur.gen != gen
		p.mu.Unlock()
		if stale {
			return
		}

		if err := p.sendKeys(target, codex.PlanModeCommand); err != nil {
			log.Printf("plan injector: send /plan plan to %s: %v", target, err)
		}
	}()
}

// OnStateChange is the watcher observer. For each pending pane that the
// snapshot reports in plan mode (and whose UpdatedAt is not stale
// relative to our scheduledAt), the user's prompt is typed and the
// entry deleted.
//
// The delete happens *inside* the locked queueing phase, before sendKeys
// runs, so a second OnStateChange that arrives while sendKeys is still
// in flight finds no pending entry and does not double-fire. On
// sendKeys failure the entry is re-inserted so the sweeper can expire
// it as a visible timeout — but only if a concurrent MaybeSchedule
// hasn't already replaced it for the same target.
func (p *PlanInjector) OnStateChange(agents []domain.Agent) {
	if p == nil {
		return
	}
	type fire struct {
		target      string
		message     string
		scheduledAt time.Time
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
		toFire = append(toFire, fire{a.Target, pp.message, pp.scheduledAt})
		delete(p.pending, a.Target) // optimistic delete — closes the duplicate race
	}
	p.mu.Unlock()

	for _, f := range toFire {
		if err := p.sendKeys(f.target, f.message); err != nil {
			log.Printf("plan injector: send prompt to %s: %v", f.target, err)
			p.mu.Lock()
			if _, exists := p.pending[f.target]; !exists {
				p.pending[f.target] = &pendingPrompt{
					target:      f.target,
					message:     f.message,
					scheduledAt: f.scheduledAt,
				}
			}
			p.mu.Unlock()
		}
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

// SetSendKeysForTest replaces the tmux send-keys function. Cross-package
// tests use this to observe injector deliveries without stubbing the real
// tmux runner. Must be called before MaybeSchedule and Start — readers at
// lines 133/153/201 access p.sendKeys without holding p.mu, mirroring the
// in-package newTestInjector contract.
func (p *PlanInjector) SetSendKeysForTest(fn func(target, text string) error) {
	p.sendKeys = fn
}

// SetPreRollForTest replaces the /plan plan pre-roll delay. Cross-package
// tests set this large to keep the pre-roll goroutine quiet while they
// drive OnStateChange directly. Must be called before MaybeSchedule —
// the spawned goroutine reads p.preRoll without holding p.mu.
func (p *PlanInjector) SetPreRollForTest(d time.Duration) {
	p.preRoll = d
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
