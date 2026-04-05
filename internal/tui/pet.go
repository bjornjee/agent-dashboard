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
	petDrowsy
	petSleeping
	petEating
)

// petSpriteWidth is the character width of the widest sprite.
const petSpriteWidth = 10

// petHeight is the number of lines the pet panel occupies:
// 1 bounce headroom + 3 sprite lines + 1 trailing newline = 5.
const petHeight = 5

// petTickInterval controls the animation frame rate.
const petTickInterval = 400 * time.Millisecond

// petTickMsg is sent on each animation frame.
type petTickMsg struct{}

// Red panda frames per state.
// All sprites share the same 3-line body shape for visual consistency.
var petFrames = map[petState][][]string{
	petIdle: {
		{" ^   ^", "(o . o)", " (u u)~"},
		{" ^   ^", "(o . o)", " (u u) ~"},
	},
	petWalking: {
		{" ^   ^", "(o . o)", " (u u)~"},
		{" ^   ^", "(o . o)", " (u u) ~"},
	},
	petSitting: {
		{" ^   ^", "(o . o)", " (u u)~"},
		{" ^   ^", "(- . o)", " (u u)~"}, // blink
		{" ^   ^", "(o . o)", " (u u)~"},
		{" ^   ^", "(o . -)", " (u u)~"}, // wink
	},
	petDrowsy: {
		{" ^   ^", "(- . -)", " (u u)~"},
		{" ^   ^", "(- . -)", " (u u) ~"},
	},
	petSleeping: {
		{" ^   ^", "(- . -) z", " (u u)~"},
		{" ^   ^", "(- . -) zZ", " (u u)~"},
	},
	petEating: {
		{" ^   ^", "(o . o)~", " (u u)/|"},
		{" ^   ^", "(o .o )~", " (u u)/|"},
	},
}

// State durations in ticks before transitioning.
var petStateDurations = map[petState]int{
	petIdle:     20, // 8 seconds
	petWalking:  16, // 6.4 seconds
	petSitting:  16, // 6.4 seconds
	petDrowsy:   12, // 4.8 seconds
	petSleeping: 20, // 8 seconds
	petEating:   16, // 6.4 seconds
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
	bounce        int // 0 = on ground, 1 = in air (walking only)
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

	// Update position and bounce for walking state
	if p.state == petWalking {
		p.updatePosition()
		p.bounce = 1 - p.bounce // toggle 0 ↔ 1
	} else {
		p.bounce = 0
	}

	// Check state transition
	p.ticksInState++
	if p.ticksInState > p.stateDuration {
		p.advanceState()
	}

	return p, tea.Tick(petTickInterval, func(time.Time) tea.Msg { return petTickMsg{} })
}

// advanceState moves to the next state in the cycle.
// idle → walk → sit → drowsy → sleep → eating → idle
func (p *petModel) advanceState() {
	switch p.state {
	case petIdle:
		p.state = petWalking
	case petWalking:
		p.state = petSitting
	case petSitting:
		p.state = petDrowsy
	case petDrowsy:
		p.state = petSleeping
	case petSleeping:
		p.state = petEating
	case petEating:
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
// Output is always 4 lines (petHeight - 1 for trailing newline).
// When bouncing, the sprite shifts up and empty space appears below.
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

	const spriteLines = 3
	topPad := 1 - p.bounce // on ground: 1 empty line above; bouncing: 0
	botPad := p.bounce     // on ground: 0 below; bouncing: 1 empty line below
	_ = botPad             // used implicitly: spriteLines + topPad + botPad = 4

	var sb strings.Builder
	for i := 0; i < topPad; i++ {
		sb.WriteString("\n")
	}
	for _, line := range sprite {
		padding := strings.Repeat(" ", p.x)
		sb.WriteString(style.Render(padding + line))
		sb.WriteString("\n")
	}
	for i := 0; i < spriteLines+topPad; i++ {
		// already counted
	}
	// Bottom padding to fill remaining lines
	remaining := (petHeight - 1) - topPad - spriteLines
	for i := 0; i < remaining; i++ {
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
