package codex

// PlanModeCommand is the slash command typed into codex's TUI input box to
// transition the session into plan mode. The dashboard's plan injector
// types this command before delivering the user's prompt for skills that
// require plan mode (see RequiresPlanMode).
//
// Single source of truth: codex 0.133.0's `/plan plan` syntax. If codex
// renames or removes this command in a future version, update here.
const PlanModeCommand = "/plan plan"

// planModeSkills lists the dashboard skills that must launch codex into
// plan mode before the user's prompt is delivered. Codex cannot enter
// plan mode from the model loop (no model-invocable equivalent of
// claude's EnterPlanMode tool), so the dashboard's plan injector types
// PlanModeCommand into the pane on the skill's behalf.
//
// Mirrors the shape of effortOptedSkills in codex.go.
var planModeSkills = map[string]struct{}{
	"feature": {},
}

// RequiresPlanMode reports whether a codex spawn for the given skill
// must be bootstrapped into plan mode by the dashboard's plan injector.
func RequiresPlanMode(skill string) bool {
	_, ok := planModeSkills[skill]
	return ok
}
