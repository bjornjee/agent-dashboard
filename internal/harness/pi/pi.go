// Package pi adapts the pi-mono coding agent CLI as an agent-dashboard
// harness, including its OpenAI provider passthrough so users can run
// gpt-5.x / codex models without leaving the dashboard's spawn surface.
//
// Pi-mono provider docs:
// https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/providers.md
//
// TODO(pi-mono#4249): the dashboard does NOT pass any thinking-level flag to
// pi yet because pi-mono's `minimal`/`off` levels are broken for gpt-5.5 as
// of v0.74.0. Defer to pi-mono's default until the upstream fix lands.
package pi

import (
	"strings"

	"github.com/bjornjee/agent-dashboard/internal/domain"
)

// Config is the static configuration the dashboard hands to the pi harness
// at construction time. SessionsDir/ConfigDir are pre-resolved by the
// dashboard's bootstrap; Command lets tests override the binary name.
type Config struct {
	Command     string // typically "pi"
	SessionsDir string // typically ~/.pi/agent/sessions
	ConfigDir   string // typically ~/.pi
}

// Pi implements domain.Harness for the pi-mono CLI.
type Pi struct {
	cfg Config
}

// New constructs a Pi harness from a Config.
func New(cfg Config) *Pi {
	return &Pi{cfg: cfg}
}

// Name implements domain.Harness.
func (p *Pi) Name() string { return "pi" }

// SessionsDir implements domain.Harness.
func (p *Pi) SessionsDir() string { return p.cfg.SessionsDir }

// ConfigDir implements domain.Harness.
func (p *Pi) ConfigDir() string { return p.cfg.ConfigDir }

// SpawnCommand implements domain.Harness. Provider+Model are passed as
// `--provider <name>` and `--model <id>` flags; either may be empty to
// inherit pi-mono's resolution chain (CLI arg > auth.json > env var).
// DefaultEffort is currently ignored (see package doc).
func (p *Pi) SpawnCommand(skill, message string, opts domain.SpawnOpts) string {
	cmd := p.cfg.Command
	// Provider/Model come from settings.toml (user-controlled) — quote them
	// so a value with a space or metacharacter doesn't shatter the tmux
	// command line. The pi binary trims surrounding quotes itself.
	if opts.Provider != "" {
		cmd += " --provider " + shellQuote(opts.Provider)
	}
	if opts.Model != "" {
		cmd += " --model " + shellQuote(opts.Model)
	}
	prompt := buildPrompt(skill, message)
	if prompt != "" {
		cmd += " " + shellQuote(prompt)
	}
	return cmd
}

func buildPrompt(skill, message string) string {
	var parts []string
	if skill != "" {
		parts = append(parts, "/"+skill)
	}
	if message != "" {
		parts = append(parts, message)
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
