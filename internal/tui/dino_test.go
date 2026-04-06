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
	if d.speed10 != baseSpeed10 {
		t.Errorf("initial speed10 = %d, want %d", d.speed10, baseSpeed10)
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
	d.speed10 = baseSpeed10
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

func TestDuckReleasedOnKeyRelease(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.hasKeyRelease = true

	d, _ = d.handleKey(downKey())
	if d.pose != dinoDucking {
		t.Fatalf("pose = %d, want dinoDucking", d.pose)
	}

	// Release down — should immediately stand
	d, _ = d.handleKeyRelease(tea.KeyReleaseMsg(tea.Key{Code: tea.KeyDown}))
	if d.pose != dinoRunning {
		t.Errorf("pose = %d after down release, want dinoRunning", d.pose)
	}
}

func TestDuckPersistsWithKeyRelease(t *testing.T) {
	// With key release support, duck persists indefinitely (no fallback timer)
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.hasKeyRelease = true
	d.spawnTimer = 999

	d, _ = d.handleKey(downKey())

	for i := 0; i < 20; i++ {
		d, _ = d.tick()
		if d.state != dinoPlaying {
			d.state = dinoPlaying
			d.obstacles = nil
		}
	}
	if d.pose != dinoDucking {
		t.Errorf("pose = %d after 20 ticks, want dinoDucking (should persist until release)", d.pose)
	}
}

func TestDuckFallbackTimerExpires(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.hasKeyRelease = false
	d.spawnTimer = 999

	d, _ = d.handleKey(downKey())
	if d.duckTicks != duckFallbackTicks {
		t.Fatalf("duckTicks = %d, want %d", d.duckTicks, duckFallbackTicks)
	}

	for i := 0; i < duckFallbackTicks+1; i++ {
		d, _ = d.tick()
		if d.state != dinoPlaying {
			d.state = dinoPlaying
			d.obstacles = nil
		}
	}
	if d.pose != dinoRunning {
		t.Errorf("pose = %d, want dinoRunning after fallback timer", d.pose)
	}
}

func TestDuckFallbackResetOnRepeat(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.hasKeyRelease = false
	d.spawnTimer = 999

	d, _ = d.handleKey(downKey())

	// Tick partway
	for i := 0; i < duckFallbackTicks-1; i++ {
		d, _ = d.tick()
		if d.state != dinoPlaying {
			d.state = dinoPlaying
			d.obstacles = nil
		}
	}
	if d.pose != dinoDucking {
		t.Fatalf("should still be ducking")
	}

	// Repeat resets timer
	d, _ = d.handleKey(downKey())
	if d.duckTicks != duckFallbackTicks {
		t.Errorf("duckTicks = %d after repeat, want %d", d.duckTicks, duckFallbackTicks)
	}
}

func TestKeyboardEnhancementsMsg(t *testing.T) {
	d := newDinoGameModel(80, 24)
	if d.hasKeyRelease {
		t.Error("hasKeyRelease should be false by default")
	}

	d, _ = d.Update(tea.KeyboardEnhancementsMsg{Flags: 0x2}) // KittyReportEventTypes = 0x2
	if !d.hasKeyRelease {
		t.Error("hasKeyRelease should be true after KeyboardEnhancementsMsg with event types")
	}
}

func TestSpaceCancelsDuckAndJumps(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying

	d, _ = d.handleKey(downKey())
	d, _ = d.handleKey(spaceKey())
	if d.pose == dinoDucking {
		t.Error("space should cancel duck")
	}
	if d.dinoY == 0 {
		t.Error("space should trigger jump after cancelling duck")
	}
}

func TestObstacleScrollsLeft(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.speed10 = 20 // 2.0 col/tick
	d.spawnTimer = 999

	startX := 60
	d.obstacles = []obstacle{{x: startX, kind: obstSmallCactus, width: 6, height: 3}}

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
	d.speed10 = 20
	d.spawnTimer = 999

	d.obstacles = []obstacle{{x: -5, kind: obstSmallCactus, width: 6, height: 3}}

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
	d.speed10 = 20
	d.spawnTimer = 999

	d.obstacles = []obstacle{{
		x:      dinoPosX,
		kind:   obstSmallCactus,
		width:  6,
		height: 3,
	}}

	d, _ = d.tick()

	if d.state != dinoGameOver {
		t.Errorf("state = %d, want dinoGameOver after collision", d.state)
	}
}

func TestNoCollisionWhenJumpingOver(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.speed10 = 20
	d.spawnTimer = 999
	d.dinoY = 10

	d.obstacles = []obstacle{{
		x:      dinoPosX,
		kind:   obstSmallCactus,
		width:  6,
		height: 3,
	}}

	if d.checkCollision() {
		t.Error("should not collide when jumping high over obstacle")
	}
}

func TestScoreIncrementsEachTick(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.speed10 = 20
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
	d.speed10 = baseSpeed10
	d.spawnTimer = 999

	startSpeed := d.speed10

	for i := 0; i < speedUpEvery+1; i++ {
		d, _ = d.tick()
		if d.state != dinoPlaying {
			d.state = dinoPlaying
			d.obstacles = nil
		}
	}

	if d.speed10 <= startSpeed {
		t.Errorf("speed10 should increase after %d ticks: speed10 = %d, start = %d",
			speedUpEvery, d.speed10, startSpeed)
	}
}

func TestSpeedGrowsBeyondOldCap(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.speed10 = 45 // old cap
	d.tickCount = speedUpEvery - 1
	d.spawnTimer = 999

	d, _ = d.tick()

	if d.speed10 <= 45 {
		t.Errorf("speed10 = %d, should grow beyond old cap of 45", d.speed10)
	}
}

func TestDifficultyIncreases(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.tickCount = 0
	d0 := d.difficulty()
	d.tickCount = 1000
	d1 := d.difficulty()
	if d1 <= d0 {
		t.Errorf("difficulty should increase: d(0)=%f, d(1000)=%f", d0, d1)
	}
}

func TestSpawnRangeDecreases(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.tickCount = 0
	min0, max0 := d.spawnRange()

	d.tickCount = 5000
	min1, max1 := d.spawnRange()

	if min1 >= min0 {
		t.Errorf("min spawn should decrease: %d -> %d", min0, min1)
	}
	if max1 >= max0 {
		t.Errorf("max spawn should decrease: %d -> %d", max0, max1)
	}
}

func TestSpawnFloor(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.tickCount = 1_000_000 // extreme difficulty
	minT, maxT := d.spawnRange()
	if minT < spawnFloor {
		t.Errorf("minSpawn = %d, should not go below %d", minT, spawnFloor)
	}
	if maxT <= minT {
		t.Errorf("maxSpawn = %d should be > minSpawn = %d", maxT, minT)
	}
}

func TestCollisionForgivenessDecreases(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.tickCount = 0
	w0, off0 := d.collisionParams(10)

	d.tickCount = 1500 // diff=3, off=2, w=5 — clearly shrinking but not saturated
	w1, off1 := d.collisionParams(10)

	if w1 <= w0 {
		t.Errorf("collision width should grow: %d -> %d", w0, w1)
	}
	if off1 >= off0 {
		t.Errorf("collision offset should shrink: %d -> %d", off0, off1)
	}
}

func upKey() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyUp} }

func TestDuckCancelledByUpKey(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying

	// Start ducking
	d, _ = d.handleKey(downKey())
	if d.pose != dinoDucking {
		t.Fatalf("pose = %d, want dinoDucking", d.pose)
	}

	// Press up — should immediately cancel duck and stand
	d, _ = d.handleKey(upKey())
	if d.pose == dinoDucking {
		t.Error("pressing up while ducking should cancel duck immediately")
	}
}

func TestDuckCancelledBySpaceThenJumps(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying

	// Start ducking
	d, _ = d.handleKey(downKey())
	if d.pose != dinoDucking {
		t.Fatalf("pose = %d, want dinoDucking", d.pose)
	}

	// Press space — should cancel duck and jump
	d, _ = d.handleKey(spaceKey())
	if d.pose == dinoDucking {
		t.Error("pressing space while ducking should cancel duck")
	}
	if d.dinoY == 0 {
		t.Error("pressing space while ducking should trigger a jump")
	}
}

func TestBirdCannotBeJumpedOver(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.spawnTimer = 999

	// Spawn a bird via the game's spawner to get realistic values
	bird := obstacle{
		x:      dinoPosX,
		kind:   obstBird,
		width:  5,
		height: dinoGameHeight, // tall hitbox as spawned
		birdY:  dinoDuckHeight,
	}
	d.obstacles = []obstacle{bird}

	// Simulate jump at peak height
	d.dinoY = 10 // jump peak
	d.pose = dinoJumping

	if !d.checkCollision() {
		t.Error("bird should collide with dino even at jump peak — birds must not be jumpable")
	}
}

func TestBirdAvoidableByDucking(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoPlaying
	d.spawnTimer = 999

	bird := obstacle{
		x:      dinoPosX,
		kind:   obstBird,
		width:  5,
		height: dinoGameHeight,
		birdY:  dinoDuckHeight,
	}
	d.obstacles = []obstacle{bird}

	// Ducking dino on the ground
	d.dinoY = 0
	d.pose = dinoDucking

	if d.checkCollision() {
		t.Error("ducking dino should avoid bird")
	}
}

func TestGameResetFromGameOver(t *testing.T) {
	d := newDinoGameModel(80, 24)
	d.state = dinoGameOver
	d.score = 42
	d.obstacles = []obstacle{{x: 10, kind: obstSmallCactus, width: 6, height: 3}}

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
	d := newDinoGameModel(10, 5)
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
