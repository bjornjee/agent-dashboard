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

func TestPetView_ContainsStickFigure(t *testing.T) {
	p := newPetModel(20)
	view := p.View()
	// The stick figure should contain the head "O"
	if !strings.Contains(view, "O") {
		t.Errorf("pet view should contain stick figure head 'O', got:\n%s", view)
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

func TestPetStateCycle(t *testing.T) {
	// Verify all states are reachable in the cycle
	seen := map[petState]bool{}
	p := newPetModel(20)
	for i := 0; i < 20; i++ {
		seen[p.state] = true
		p.ticksInState = p.stateDuration + 1
		p.advanceState()
	}
	// Should have seen at least idle, walking, sitting
	for _, s := range []petState{petIdle, petWalking, petSitting} {
		if !seen[s] {
			t.Errorf("state %d never reached in cycle", s)
		}
	}
}
