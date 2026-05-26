package harness

import "strings"

const dashboardPluginNamespace = "agent-dashboard"

// InitialPrompt returns the text the dashboard should submit after a harness
// has reached an input-ready state.
func InitialPrompt(harnessName, skill, message string) string {
	var parts []string
	if skill != "" {
		switch harnessName {
		case "codex":
			parts = append(parts, "$"+dashboardPluginNamespace+":"+skill)
		default:
			parts = append(parts, "/"+skill)
		}
	}
	if message != "" {
		parts = append(parts, message)
	}
	return strings.Join(parts, " ")
}
