package dispatch

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bjornjee/agent-dashboard/internal/domain"
	"github.com/bjornjee/agent-dashboard/internal/harness/codex"
)

// fakeSendKeys records every (target, text) pair sent and lets the test
// inject errors. It replaces tmux.TmuxSendKeys via the injector's sendKeys
// field — avoids touching the real tmux runner.
type fakeSendKeys struct {
	mu    sync.Mutex
	calls []sendCall
	fail  map[string]error // target -> error
}

type sendCall struct {
	target string
	text   string
}

func (f *fakeSendKeys) send(target, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, sendCall{target, text})
	if err, ok := f.fail[target]; ok {
		return err
	}
	return nil
}

func (f *fakeSendKeys) snapshot() []sendCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]sendCall, len(f.calls))
	copy(out, f.calls)
	return out
}

func newTestInjector(t *testing.T, sk *fakeSendKeys, now time.Time) *PlanInjector {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	inj := NewPlanInjector()
	inj.sendKeys = sk.send
	inj.now = func() time.Time { return now }
	inj.sleep = func(time.Duration) {}
	inj.preRoll = time.Millisecond
	inj.deadline = 100 * time.Millisecond
	inj.sweepInterval = 5 * time.Millisecond
	inj.Start(ctx)
	return inj
}

func waitUntil(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timeout waiting: %s", msg)
}

func TestNilReceiver_NoOp(t *testing.T) {
	// Both methods must tolerate nil receivers so callers that don't
	// instantiate a PlanInjector (most tests) don't panic.
	var p *PlanInjector
	p.MaybeSchedule("codex", "feature", "main:5.1", "msg")
	p.OnStateChange([]domain.Agent{{Target: "main:5.1", PermissionMode: "plan"}})
}

func TestMaybeSchedule_NonCodexHarness_NoOp(t *testing.T) {
	sk := &fakeSendKeys{}
	inj := newTestInjector(t, sk, time.Now())

	inj.MaybeSchedule("claude", "feature", "main:5.1", "msg")

	time.Sleep(10 * time.Millisecond)
	if got := sk.snapshot(); len(got) != 0 {
		t.Fatalf("expected no send-keys for claude harness, got %v", got)
	}
	if _, ok := inj.peek("main:5.1"); ok {
		t.Fatal("expected no pending entry for claude harness")
	}
}

func TestMaybeSchedule_CodexNonPlanModeSkill_NoOp(t *testing.T) {
	sk := &fakeSendKeys{}
	inj := newTestInjector(t, sk, time.Now())

	inj.MaybeSchedule("codex", "fix", "main:5.1", "msg")

	time.Sleep(10 * time.Millisecond)
	if got := sk.snapshot(); len(got) != 0 {
		t.Fatalf("expected no send-keys for non-plan-mode skill, got %v", got)
	}
}

func TestMaybeSchedule_CodexPlanModeSkill_TypesPlanCommand(t *testing.T) {
	sk := &fakeSendKeys{}
	inj := newTestInjector(t, sk, time.Now())

	inj.MaybeSchedule("codex", "feature", "main:5.1", "user prompt")

	waitUntil(t, func() bool {
		for _, c := range sk.snapshot() {
			if c.target == "main:5.1" && c.text == "/plan plan" {
				return true
			}
		}
		return false
	}, "/plan plan typed")

	pp, ok := inj.peek("main:5.1")
	if !ok {
		t.Fatal("expected pending entry after schedule")
	}
	if pp.message != "user prompt" {
		t.Errorf("pending message = %q, want %q", pp.message, "user prompt")
	}
}

func TestOnStateChange_PlanMode_TypesPrompt(t *testing.T) {
	sk := &fakeSendKeys{}
	scheduledAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	inj := newTestInjector(t, sk, scheduledAt)

	inj.MaybeSchedule("codex", "feature", "main:5.1", "user prompt")
	waitUntil(t, func() bool { return len(sk.snapshot()) >= 1 }, "/plan plan typed")

	// simulate the state watcher firing with the pane now in plan mode
	updated := scheduledAt.Add(2 * time.Second).Format(time.RFC3339Nano)
	inj.OnStateChange([]domain.Agent{{
		Target:         "main:5.1",
		TmuxPaneID:     "%124",
		PermissionMode: "plan",
		UpdatedAt:      updated,
	}})

	calls := sk.snapshot()
	var sawPrompt bool
	for _, c := range calls {
		if c.target == "main:5.1" && c.text == "user prompt" {
			sawPrompt = true
		}
	}
	if !sawPrompt {
		t.Fatalf("expected prompt send-keys after plan-mode event, got %v", calls)
	}
	if _, ok := inj.peek("main:5.1"); ok {
		t.Fatal("expected pending entry deleted after successful inject")
	}
}

func TestOnStateChange_NonPlanMode_NoInject(t *testing.T) {
	sk := &fakeSendKeys{}
	scheduledAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	inj := newTestInjector(t, sk, scheduledAt)

	inj.MaybeSchedule("codex", "feature", "main:5.1", "user prompt")
	waitUntil(t, func() bool { return len(sk.snapshot()) >= 1 }, "/plan plan typed")

	pre := len(sk.snapshot())
	inj.OnStateChange([]domain.Agent{{
		Target:         "main:5.1",
		PermissionMode: "default",
		UpdatedAt:      scheduledAt.Add(time.Second).Format(time.RFC3339Nano),
	}})
	if got := len(sk.snapshot()); got != pre {
		t.Fatalf("expected no new send-keys for non-plan mode, before=%d after=%d", pre, got)
	}
	if _, ok := inj.peek("main:5.1"); !ok {
		t.Fatal("pending entry should remain when mode is not plan")
	}
}

func TestOnStateChange_UnknownTarget_NoInject(t *testing.T) {
	sk := &fakeSendKeys{}
	inj := newTestInjector(t, sk, time.Now())

	// no MaybeSchedule call — pending is empty
	inj.OnStateChange([]domain.Agent{{
		Target:         "main:9.9",
		PermissionMode: "plan",
		UpdatedAt:      time.Now().Format(time.RFC3339Nano),
	}})
	if got := sk.snapshot(); len(got) != 0 {
		t.Fatalf("expected no send-keys when target not pending, got %v", got)
	}
}

func TestOnStateChange_StaleEvent_NoInject(t *testing.T) {
	sk := &fakeSendKeys{}
	scheduledAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	inj := newTestInjector(t, sk, scheduledAt)

	inj.MaybeSchedule("codex", "feature", "main:5.1", "user prompt")
	waitUntil(t, func() bool { return len(sk.snapshot()) >= 1 }, "/plan plan typed")

	pre := len(sk.snapshot())
	// state event with timestamp BEFORE our scheduledAt — stale (pane reuse)
	stale := scheduledAt.Add(-10 * time.Second).Format(time.RFC3339Nano)
	inj.OnStateChange([]domain.Agent{{
		Target:         "main:5.1",
		PermissionMode: "plan",
		UpdatedAt:      stale,
	}})
	if got := len(sk.snapshot()); got != pre {
		t.Fatalf("expected stale event ignored, before=%d after=%d", pre, got)
	}
	if _, ok := inj.peek("main:5.1"); !ok {
		t.Fatal("pending entry should remain after stale-event rejection")
	}
}

func TestOnStateChange_SendKeysError_KeepsPending(t *testing.T) {
	sk := &fakeSendKeys{
		fail: map[string]error{"main:5.1": errors.New("tmux dead")},
	}
	scheduledAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	inj := newTestInjector(t, sk, scheduledAt)

	inj.MaybeSchedule("codex", "feature", "main:5.1", "user prompt")
	// the failing /plan plan send-keys still counts as a call; wait for it
	waitUntil(t, func() bool { return len(sk.snapshot()) >= 1 }, "/plan plan attempted")

	// State event arrives; the prompt send-keys also fails. Pending should
	// remain so the sweeper can expire it.
	inj.OnStateChange([]domain.Agent{{
		Target:         "main:5.1",
		PermissionMode: "plan",
		UpdatedAt:      scheduledAt.Add(time.Second).Format(time.RFC3339Nano),
	}})
	if _, ok := inj.peek("main:5.1"); !ok {
		t.Fatal("pending entry should remain when send-keys fails")
	}
}

func TestSweeper_ExpiresStalePending(t *testing.T) {
	sk := &fakeSendKeys{}
	scheduledAt := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	inj := NewPlanInjector()
	inj.sendKeys = sk.send
	inj.sleep = func(time.Duration) {}
	inj.preRoll = time.Millisecond
	inj.deadline = 50 * time.Millisecond
	inj.sweepInterval = 5 * time.Millisecond
	// advance "now" past the deadline on later reads. Use a mutex-guarded
	// counter so the sweeper's reads don't race with the test goroutine.
	var mu sync.Mutex
	var calls int
	inj.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == 1 {
			return scheduledAt // MaybeSchedule
		}
		return scheduledAt.Add(time.Second) // far past deadline
	}
	inj.Start(ctx)

	inj.MaybeSchedule("codex", "feature", "main:5.1", "user prompt")
	waitUntil(t, func() bool {
		_, ok := inj.peek("main:5.1")
		return !ok
	}, "sweeper should have expired pending entry")
}

// blockingSendKeys wraps fakeSendKeys with a gate that blocks any send-keys
// matching blockText until the test closes release. Used to deterministically
// hold a send-keys call in-flight while another goroutine runs OnStateChange.
type blockingSendKeys struct {
	fakeSendKeys
	blockText string
	entered   chan struct{}
	release   chan struct{}
	once      sync.Once
}

func (b *blockingSendKeys) send(target, text string) error {
	b.mu.Lock()
	b.calls = append(b.calls, sendCall{target, text})
	b.mu.Unlock()
	if text == b.blockText {
		b.once.Do(func() { close(b.entered) })
		<-b.release
	}
	return nil
}

// Regression test for the OnStateChange duplicate-fire race. If a second
// OnStateChange arrives while the first one's send-keys is still running,
// the pending entry must not be visible to the second call — otherwise the
// user's prompt is typed into the codex pane twice. The fix is optimistic
// delete inside the locked queueing phase (with re-insert on send-keys
// failure).
func TestOnStateChange_NoDuplicateFire_WhenSendKeysInFlight(t *testing.T) {
	bk := &blockingSendKeys{
		blockText: "user prompt",
		entered:   make(chan struct{}),
		release:   make(chan struct{}),
	}
	scheduledAt := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	inj := NewPlanInjector()
	inj.sendKeys = bk.send
	inj.now = func() time.Time { return scheduledAt }
	inj.sleep = func(time.Duration) {}
	inj.preRoll = time.Millisecond
	inj.deadline = 5 * time.Second
	inj.sweepInterval = 50 * time.Millisecond
	inj.Start(ctx)

	inj.MaybeSchedule("codex", "feature", "main:5.1", "user prompt")
	// Wait for the pre-roll /plan plan to be recorded (it doesn't block —
	// only the user prompt does). Otherwise the in-flight assertion is racy.
	waitUntil(t, func() bool {
		for _, c := range bk.snapshot() {
			if c.text == codex.PlanModeCommand {
				return true
			}
		}
		return false
	}, "/plan plan pre-roll recorded")

	agent := domain.Agent{
		Target:         "main:5.1",
		PermissionMode: "plan",
		UpdatedAt:      scheduledAt.Add(time.Second).Format(time.RFC3339Nano),
	}

	// OnStateChange #1 — runs in goroutine because the prompt send-keys
	// will block until we close bk.release.
	done1 := make(chan struct{})
	go func() {
		inj.OnStateChange([]domain.Agent{agent})
		close(done1)
	}()
	<-bk.entered // confirm prompt send-keys is in-flight

	// OnStateChange #2 — also in a goroutine because, in the unfixed
	// code, it would queue a duplicate fire and block on sendKeys; the
	// fixed code skips it (pending already deleted) and returns immediately.
	done2 := make(chan struct{})
	go func() {
		inj.OnStateChange([]domain.Agent{agent})
		close(done2)
	}()

	// Give #2 a tick to either return immediately (fixed) or block in
	// sendKeys (buggy). Either outcome is observable post-release.
	time.Sleep(50 * time.Millisecond)

	close(bk.release) // release any in-flight send(s)
	<-done1
	<-done2

	promptSends := 0
	for _, c := range bk.snapshot() {
		if c.text == "user prompt" {
			promptSends++
		}
	}
	if promptSends != 1 {
		t.Fatalf("expected exactly 1 prompt send, got %d (calls=%v)", promptSends, bk.snapshot())
	}
}

func TestMaybeSchedule_PreRollErrorLogged(t *testing.T) {
	// /plan plan send-keys fails; we just need to confirm the injector
	// doesn't panic and the pending entry remains for the sweeper.
	sk := &fakeSendKeys{
		fail: map[string]error{"main:5.1": errors.New("boom")},
	}
	inj := newTestInjector(t, sk, time.Now())

	inj.MaybeSchedule("codex", "feature", "main:5.1", "user prompt")
	waitUntil(t, func() bool {
		for _, c := range sk.snapshot() {
			if strings.Contains(c.text, "/plan") {
				return true
			}
		}
		return false
	}, "/plan plan attempt recorded")

	if _, ok := inj.peek("main:5.1"); !ok {
		t.Fatal("pending entry should remain even if /plan plan send-keys errored")
	}
}
