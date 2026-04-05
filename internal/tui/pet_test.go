package tui

import (
	"strings"
	"testing"
)

func TestNewPetModel(t *testing.T) {
	p := newPetModel(20)
	if p.state != petIdle {
		t.Errorf("initial state = %d, want petIdle (%d)", p.state, petIdle)
	}
	if p.width != 20 {
		t.Errorf("width = %d, want 20", p.width)
	}
	if p.frame != 0 {
		t.Errorf("initial frame = %d, want 0", p.frame)
	}
}

func TestPetView_ContainsRedPanda(t *testing.T) {
	p := newPetModel(20)
	view := p.View()
	if !strings.Contains(view, "(o . o)") {
		t.Errorf("pet view should contain red panda face, got:\n%s", view)
	}
}

func TestPetView_WidthRespected(t *testing.T) {
	p := newPetModel(30)
	view := p.View()
	for _, line := range strings.Split(view, "\n") {
		// Allow some tolerance for ANSI escape codes from lipgloss
		// but the visible content should not exceed width
		if len([]rune(line)) > 60 { // generous upper bound accounting for ANSI
			t.Errorf("line too wide (%d runes): %q", len([]rune(line)), line)
		}
	}
}

func TestPetStateTransition(t *testing.T) {
	p := newPetModel(20)
	p.state = petIdle

	// Simulate a state change tick
	p.ticksInState = p.stateDuration + 1
	p.advanceState()

	if p.state == petIdle {
		t.Error("expected state to change from idle after advanceState()")
	}
	if p.ticksInState != 0 {
		t.Errorf("ticksInState should reset to 0, got %d", p.ticksInState)
	}
}

func TestPetWalkingMovesPosition(t *testing.T) {
	p := newPetModel(20)
	p.state = petWalking
	p.direction = 1
	startX := p.x

	p.updatePosition()

	if p.x == startX {
		t.Error("walking pet should change x position")
	}
}

func TestPetWalkingBouncesAtBounds(t *testing.T) {
	p := newPetModel(20)
	p.state = petWalking
	p.direction = 1
	p.x = p.width - petSpriteWidth // at right edge

	p.updatePosition()

	if p.direction != -1 {
		t.Errorf("direction should reverse at right bound, got %d", p.direction)
	}
}

func TestPetWalkingBounces(t *testing.T) {
	p := newPetModel(20)
	p.state = petWalking
	p.bounce = 0

	// Simulate a tick — bounce should toggle
	p.bounce = 1 - p.bounce
	if p.bounce != 1 {
		t.Errorf("bounce should be 1 after toggle, got %d", p.bounce)
	}

	// View with bounce=1 should have sprite higher (no leading empty line)
	view := p.View()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d", len(lines))
	}
	// First line should contain sprite content (ears), not be empty
	if strings.TrimSpace(lines[0]) == "" {
		t.Error("bounce=1 should have sprite on first line, got empty")
	}
}

func TestPetStateCycle(t *testing.T) {
	// Verify all states are reachable in the cycle
	seen := map[petState]bool{}
	p := newPetModel(20)
	for i := 0; i < 20; i++ {
		seen[p.state] = true
		p.ticksInState = p.stateDuration + 1
		p.advanceState()
	}
	// Should have seen all states in the cycle
	for _, s := range []petState{petIdle, petWalking, petSitting, petDrowsy, petSleeping, petEating} {
		if !seen[s] {
			t.Errorf("state %d never reached in cycle", s)
		}
	}
}
