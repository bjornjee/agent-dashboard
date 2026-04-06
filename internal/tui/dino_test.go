package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func keyMsg(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text}
}

func spaceKey() tea.KeyPressMsg { return keyMsg(' ', " ") }
func downKey() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyDown} }
func escKey() tea.KeyPressMsg   { return tea.KeyPressMsg{Code: tea.KeyEscape} }

func TestNewDinoGameModel(t *testing.T) {
	d := newDinoGameModel(80, 24)
	if d.state != dinoWaiting {
		t.Errorf("initial state = %d, want dinoWaiting (%d)", d.state, dinoWaiting)
	}
	if d.score != 0 {
		t.Errorf("initial score = %d, want 0", d.score)
	}
	if d.speed != baseSpeed {
		t.Errorf("initial speed = %d, want %d", d.speed, baseSpeed)
	}
	if d.width != 80 || d.height != 24 {
		t.Errorf("dimensions = %dx%d, want 80x24", d.width, d.height)
	}
}

func TestDinoStartOnSpace(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d, cmd := d.handleKey(spaceKey())
	if d.state != dinoPlaying {
		t.Errorf("state after space = %d, want dinoPlaying", d.state)
	}
	if cmd == nil {
		t.Error("expected tick command after starting game")
	}
}

func TestDinoExitOnEsc(t *testing.T) {
	d := newDinoGameModel(80, 24)
	_, cmd := d.handleKey(escKey())
	if cmd == nil {
		t.Error("expected exit command on esc in waiting state")
	}
	msg := cmd()
	if _, ok := msg.(dinoExitMsg); !ok {
		t.Errorf("expected dinoExitMsg, got %T", msg)
	}
}

func TestDinoJumpPhysics(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.speed = baseSpeed
	d.spawnTimer = 999

	// Jump
	d, _ = d.handleKey(spaceKey())
	if d.dinoY == 0 {
		t.Error("dino should be above ground after jump")
	}
	if d.pose != dinoJumping {
		t.Errorf("pose = %d, want dinoJumping", d.pose)
	}
	peakY := d.dinoY

	// Tick until back on ground
	for i := 0; i < 50; i++ {
		d, _ = d.tick()
		if d.state != dinoPlaying {
			break
		}
		if d.dinoY > peakY {
			peakY = d.dinoY
		}
		if d.dinoY == 0 {
			break
		}
	}

	if d.dinoY != 0 {
		t.Errorf("dino should land back on ground, dinoY = %d", d.dinoY)
	}
	if peakY <= 0 {
		t.Error("dino should have reached a peak height > 0")
	}
	if d.pose == dinoJumping {
		t.Error("pose should not be jumping after landing")
	}
}

func TestDinoDuckChangesPose(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying

	d, _ = d.handleKey(downKey())
	if d.pose != dinoDucking {
		t.Errorf("pose = %d, want dinoDucking", d.pose)
	}
	if !d.isDucking() {
		t.Error("expected isDucking() = true after down key")
	}
}

func TestDinoCannotJumpWhileDucking(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying

	d, _ = d.handleKey(downKey())
	d, _ = d.handleKey(spaceKey())
	if d.dinoY != 0 {
		t.Error("should not be able to jump while ducking")
	}
}

func TestObstacleScrollsLeft(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.speed = 2
	d.spawnTimer = 999

	startX := 60
	d.obstacles = []obstacle{{x: startX, kind: obstSmallCactus, width: 3, height: 1}}

	d, _ = d.tick()

	if d.obstacles[0].x >= startX {
		t.Errorf("obstacle should move left: x = %d, was %d", d.obstacles[0].x, startX)
	}
	if d.obstacles[0].x != startX-2 {
		t.Errorf("obstacle x = %d, want %d", d.obstacles[0].x, startX-2)
	}
}

func TestObstacleRemoval(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.speed = 2
	d.spawnTimer = 999

	d.obstacles = []obstacle{{x: -5, kind: obstSmallCactus, width: 3, height: 1}}

	d, _ = d.tick()

	for _, o := range d.obstacles {
		if o.x+o.width <= 0 {
			t.Error("off-screen obstacle should have been removed")
		}
	}
}

func TestCollisionTriggersGameOver(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.speed = 1
	d.spawnTimer = 999

	d.obstacles = []obstacle{{
		x:      dinoPosX,
		kind:   obstSmallCactus,
		width:  3,
		height: 1,
	}}

	d, _ = d.tick()

	if d.state != dinoGameOver {
		t.Errorf("state = %d, want dinoGameOver after collision", d.state)
	}
}

func TestNoCollisionWhenJumpingOver(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.speed = 1
	d.spawnTimer = 999
	d.dinoY = 10

	d.obstacles = []obstacle{{
		x:      dinoPosX,
		kind:   obstSmallCactus,
		width:  3,
		height: 1,
	}}

	if d.checkCollision() {
		t.Error("should not collide when jumping high over obstacle")
	}
}

func TestScoreIncrementsEachTick(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.speed = 1
	d.spawnTimer = 999

	startScore := d.score
	d, _ = d.tick()

	if d.score != startScore+1 {
		t.Errorf("score = %d, want %d", d.score, startScore+1)
	}
}

func TestSpeedIncreasesOverTime(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.speed = baseSpeed
	d.spawnTimer = 999

	startSpeed := d.speed

	for i := 0; i < speedUpInterval+1; i++ {
		d, _ = d.tick()
		if d.state != dinoPlaying {
			d.state = dinoPlaying
			d.obstacles = nil
		}
	}

	if d.speed <= startSpeed {
		t.Errorf("speed should increase after %d ticks: speed = %d, start = %d",
			speedUpInterval, d.speed, startSpeed)
	}
}

func TestSpeedCapsAtMax(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.speed = maxSpeed
	d.tickCount = speedUpInterval - 1
	d.spawnTimer = 999

	d, _ = d.tick()

	if d.speed > maxSpeed {
		t.Errorf("speed = %d should not exceed maxSpeed %d", d.speed, maxSpeed)
	}
}

func TestGameResetFromGameOver(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoGameOver
	d.score = 42
	d.obstacles = []obstacle{{x: 10, kind: obstSmallCactus, width: 3, height: 1}}

	d, _ = d.handleKey(spaceKey())

	if d.state != dinoWaiting {
		t.Errorf("state = %d, want dinoWaiting after reset", d.state)
	}
	if d.score != 0 {
		t.Errorf("score = %d, want 0 after reset", d.score)
	}
	if len(d.obstacles) != 0 {
		t.Errorf("obstacles should be cleared, got %d", len(d.obstacles))
	}
}

func TestDinoViewWaiting(t *testing.T) {
	d := newDinoGameModel(80, 24)
	view := d.View()
	if len(view) == 0 {
		t.Error("expected non-empty view in waiting state")
	}
}

func TestDinoViewTooSmall(t *testing.T) {
	d := newDinoGameModel(5, 2)
	view := d.View()
	if view == "" {
		t.Error("expected a message for too-small terminal")
	}
}

func TestDinoSetSize(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d = d.withSize(120, 40)
	if d.width != 120 || d.height != 40 {
		t.Errorf("dimensions = %dx%d, want 120x40", d.width, d.height)
	}
}
