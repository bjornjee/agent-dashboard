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
