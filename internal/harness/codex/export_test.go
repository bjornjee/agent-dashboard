package codex

import "time"

// SetSleep swaps the package settle-delay function for tests and returns a
// restore func. Tests pass a no-op to avoid the real 100ms wait on every
// queue-prone state without changing production behavior.
func SetSleep(f func(time.Duration)) func() {
	orig := sleep
	sleep = f
	return func() { sleep = orig }
}

// SetBootReadyBudget swaps the composer-readiness polling budget used by
// BootstrapPlanMode and returns a restore func.
func SetBootReadyBudget(d time.Duration) func() {
	orig := bootReadyBudget
	bootReadyBudget = d
	return func() { bootReadyBudget = orig }
}

// BootstrapPlanModeForTest exposes the result-returning bootstrap so tests
// can assert the injected/timed-out outcome.
func BootstrapPlanModeForTest(target, prompt string) planbootResult {
	return bootstrapPlanMode(target, prompt)
}

// Planboot result values for black-box assertions.
var (
	PlanbootInjectedForTest = planbootInjected
	PlanbootTimedOutForTest = planbootTimedOut
)
