package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// petState represents the current behavior of the ASCII pet.
type petState int

const (
	petIdle petState = iota
	petWalking
	petSitting
	petSleeping
)

// petSpriteWidth is the character width of the widest stick figure sprite.
const petSpriteWidth = 6

// petHeight is the number of lines the pet panel occupies:
// 1 separator + 3 sprite lines + 1 trailing newline from View() = 5.
const petHeight = 5

// petTickInterval controls the animation frame rate.
const petTickInterval = 250 * time.Millisecond

// petTickMsg is sent on each animation frame.
type petTickMsg struct{}

// Stick figure frames per state.
var petFrames = map[petState][][]string{
	petIdle: {
		{" O ", "/|\\", "/ \\"},
		{" O ", "/|\\", " | "},
	},
	petWalking: {
		{" O ", "/|\\", "/ \\"},
		{" O ", "/|\\", " |\\"},
	},
	petSitting: {
		{" O ", "/|\\", "/  "},
	},
	petSleeping: {
		{" O  ", "/|\\ z", "/  "},
		{" O   ", "/|\\ zZ", "/  "},
	},
}

// State durations in ticks before transitioning.
var petStateDurations = map[petState]int{
	petIdle:     20, // 5 seconds
	petWalking:  16, // 4 seconds
	petSitting:  12, // 3 seconds
	petSleeping: 24, // 6 seconds
}

// petModel is a self-contained Bubble Tea sub-model for the ASCII pet.
type petModel struct {
	state         petState
	frame         int
	ticksInState  int
	stateDuration int
	x             int
	direction     int // 1 = right, -1 = left
	width         int
}

// newPetModel creates a pet model for the given panel width.
func newPetModel(width int) petModel {
	return petModel{
		state:         petIdle,
		frame:         0,
		ticksInState:  0,
		stateDuration: petStateDurations[petIdle],
		x:             0,
		direction:     1,
		width:         width,
	}
}

// Init returns the initial tick command.
func (p petModel) Init() tea.Cmd {
	return tea.Tick(petTickInterval, func(time.Time) tea.Msg { return petTickMsg{} })
}

// Update handles tick messages for animation.
func (p petModel) Update(msg tea.Msg) (petModel, tea.Cmd) {
	if _, ok := msg.(petTickMsg); !ok {
		return p, nil
	}

	// Advance frame
	frames := petFrames[p.state]
	if len(frames) > 1 {
		p.frame = (p.frame + 1) % len(frames)
	}

	// Update position for walking state
	if p.state == petWalking {
		p.updatePosition()
	}

	// Check state transition
	p.ticksInState++
	if p.ticksInState > p.stateDuration {
		p.advanceState()
	}

	return p, tea.Tick(petTickInterval, func(time.Time) tea.Msg { return petTickMsg{} })
}

// advanceState moves to the next state in the cycle.
func (p *petModel) advanceState() {
	switch p.state {
	case petIdle:
		p.state = petWalking
	case petWalking:
		p.state = petSitting
	case petSitting:
		p.state = petSleeping
	case petSleeping:
		p.state = petIdle
	}
	p.frame = 0
	p.ticksInState = 0
	p.stateDuration = petStateDurations[p.state]
}

// updatePosition shifts the pet's x position during walking.
func (p *petModel) updatePosition() {
	p.x += p.direction
	maxX := p.width - petSpriteWidth
	if maxX < 0 {
		maxX = 0
	}
	if p.x >= maxX {
		p.x = maxX
		p.direction = -1
	}
	if p.x <= 0 {
		p.x = 0
		p.direction = 1
	}
}

// View renders the pet as a string.
func (p petModel) View() string {
	frames := petFrames[p.state]
	if len(frames) == 0 {
		return ""
	}
	frameIdx := p.frame % len(frames)
	sprite := frames[frameIdx]

	style := lipgloss.NewStyle().
		Foreground(themeOverlay1).
		Faint(true)

	var sb strings.Builder
	for _, line := range sprite {
		padding := strings.Repeat(" ", p.x)
		sb.WriteString(style.Render(padding + line))
		sb.WriteString("\n")
	}
	return sb.String()
}

// setWidth updates the pet's available width.
func (p *petModel) setWidth(w int) {
	p.width = w
	// Clamp position
	maxX := w - petSpriteWidth
	if maxX < 0 {
		maxX = 0
	}
	if p.x > maxX {
		p.x = maxX
	}
}
