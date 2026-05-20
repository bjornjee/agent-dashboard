package skills

// codexBlockedSkills lists dashboard skills that still rely on Claude-only
// orchestration primitives. New/custom skills are allowed by default because
// the dashboard cannot infer their internals from a name.
var codexBlockedSkills = map[string]struct{}{
	"implement": {},
	"rca":       {},
}

// SupportsHarness reports whether a skill can be spawned for a harness.
// Empty skill names are free-prompt spawns and are always supported.
func SupportsHarness(skill, harness string) bool {
	if skill == "" || harness != "codex" {
		return true
	}
	_, blocked := codexBlockedSkills[skill]
	return !blocked
}
