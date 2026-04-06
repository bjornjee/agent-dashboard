package tui

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// -- Game states --

type dinoGameState int

const (
	dinoWaiting  dinoGameState = iota // "Press SPACE to start"
	dinoPlaying                       // active gameplay
	dinoGameOver                      // "Game Over"
)

// -- Dino posture --

type dinoPose int

const (
	dinoRunning dinoPose = iota
	dinoJumping
	dinoDucking
)

// -- Obstacle types --

const (
	obstSmallCactus = iota
	obstLargeCactus
	obstBird
)

// obstacle represents a single obstacle on the game field.
type obstacle struct {
	x      int // horizontal position (column, decreases each tick)
	kind   int
	width  int
	height int // lines tall (from ground)
	birdY  int // vertical offset for birds (0 = ground level)
}

// -- Tick messages --

const dinoTickInterval = 50 * time.Millisecond

type dinoTickMsg struct{}
type dinoExitMsg struct{}

// -- Sprites (half-block art, matching approved plan) --

var dinoRunFrame0 = []string{
	"    ▄█▀██▄",
	"    ███▀▀▀",
	"▄█▄████▀  ",
	" ▀▀██▀    ",
	"   ▀  ▀   ",
}

var dinoRunFrame1 = []string{
	"    ▄█▀██▄",
	"    ███▀▀▀",
	"▄█▄████▀  ",
	" ▀▀██▀    ",
	"    ▀▀    ",
}

var dinoDuckFrame = []string{
	"▄▄▄▄█▀██",
	"▀▀████▀  ",
	"   ▀ ▀   ",
}

var spriteSmallCactus = []string{
	"  ██  ",
	"█▄██▄█",
	" ▀██▀ ",
}

var spriteLargeCactus = []string{
	"  ██  ",
	"█ ██ █",
	"▀████▀",
	"  ██  ",
}

var spriteBirdFrame0 = []string{
	"▀▄ ▄▀",
	"  █  ",
}

var spriteBirdFrame1 = []string{
	"▄▀ ▀▄",
	"  █  ",
}

// Sprite visual position (for rendering).
const dinoPosX = 4

// Collision hitbox — smaller than visual sprites for forgiving gameplay.
// Visual dino is 10 wide, but collision uses 4 (trimmed 3 each side).
const (
	dinoCollisionW   = 4
	dinoStandHeight  = 5
	dinoDuckCollW    = 4
	dinoDuckHeight   = 3
	dinoCollisionOff = 3 // offset from dinoPosX to start of collision box
)

// dinoGameHeight is the maximum lines the game needs:
// 1 score + playRows + 1 ground.
// playRows = dinoStandHeight + jumpPeak.
//
// Jump math (v=4, g=1, discrete):
//
//	Y: 4→7→9→10→10→9→7→4→0  (8 ticks airborne, Y≥4)
//	peak = 10
//
// Reaction zone math:
//
//	danger zone = dinoCollisionW + maxObstWidth = 4+6 = 10 cols
//	base speed 2.0: 10/2 = 5 ticks to cross, 8 airborne → 6 col buffer ✓
//	max speed 4.5:  10/4.5 ≈ 2 ticks to cross, 8 airborne → 27 col buffer ✓
//
// Recovery: spawn timer is tick-based (not column-based), so the player
// always gets ≥1.25s (25 ticks) between obstacles regardless of speed.
//
// playRows = 5 + 10 = 15, total = 17.
const dinoGameHeight = 17

// -- Physics constants --

const (
	jumpVelocity     = 4
	gravity          = 1  // tuned so 8-tick airtime clears danger zone at all speeds
	duckDefaultTicks = 12 // default duck expiry in ticks (~600ms) to cover initial key repeat delay

	// Speed is stored in tenths of a column per tick (fixed-point ×10).
	baseSpeed10    = 20 // 2.0 col/tick — tuned so jump clears 10-col danger zone
	maxSpeed10     = 45 // 4.5 col/tick
	speedIncrement = 1  // +0.1 col/tick each interval
	speedUpEvery   = 50 // ticks between speed bumps

	// Spawn timer is tick-based so recovery time is constant regardless of speed.
	minSpawnTicks = 25 // 1.25s minimum between obstacles
	maxSpawnTicks = 45 // 2.25s maximum
)

// -- Game model --

type dinoGameModel struct {
	state        dinoGameState
	width        int
	height       int
	pose         dinoPose
	dinoY        int // vertical offset from ground (0 = ground, positive = up)
	jumpVel      int
	frame        int       // animation frame counter
	duckTicks    int       // ticks remaining in duck; 0 = not ducking
	lastDownTime time.Time // timestamp of last "down" key press
	duckExpiry   int       // adaptive expiry in ticks (3× measured repeat gap)

	obstacles  []obstacle
	groundOff  int
	speed10    int // speed in tenths of col/tick (fixed-point ×10)
	subPixel   int // accumulator for sub-pixel movement (0-9)
	score      int
	tickCount  int
	spawnTimer int
}

func newDinoGameModel(w, h int) dinoGameModel {
	return dinoGameModel{
		state:      dinoWaiting,
		width:      w,
		height:     h,
		speed10:    baseSpeed10,
		spawnTimer: minSpawnTicks,
	}
}

func (d dinoGameModel) isDucking() bool { return d.pose == dinoDucking }

func (d dinoGameModel) Init() tea.Cmd {
	return nil
}

func (d dinoGameModel) Update(msg tea.Msg) (dinoGameModel, tea.Cmd) {
	switch msg := msg.(type) {
	case dinoTickMsg:
		return d.tick()
	case tea.KeyPressMsg:
		return d.handleKey(msg)
	}
	return d, nil
}

func (d dinoGameModel) handleKey(msg tea.KeyPressMsg) (dinoGameModel, tea.Cmd) {
	key := msg.String()

	switch d.state {
	case dinoWaiting:
		if key == " " || key == "space" {
			d.state = dinoPlaying
			d.score = 0
			d.tickCount = 0
			d.speed10 = baseSpeed10
			d.subPixel = 0
			d.obstacles = nil
			d.spawnTimer = minSpawnTicks
			d.dinoY = 0
			d.jumpVel = 0
			d.pose = dinoRunning
			d.frame = 0
			return d, tea.Tick(dinoTickInterval, func(time.Time) tea.Msg { return dinoTickMsg{} })
		}
		if key == "esc" || key == "q" {
			return d, func() tea.Msg { return dinoExitMsg{} }
		}

	case dinoPlaying:
		switch key {
		case " ", "space", "up":
			if d.dinoY == 0 && !d.isDucking() {
				d.jumpVel = jumpVelocity
				d.dinoY = d.jumpVel
				d.pose = dinoJumping
			}
		case "down":
			if d.dinoY == 0 {
				now := time.Now()
				gap := now.Sub(d.lastDownTime)
				d.lastDownTime = now
				if gap > 0 && gap < 500*time.Millisecond {
					// Adaptive: 3× the measured repeat gap, converted to ticks
					ticks := int((3 * gap) / dinoTickInterval)
					if ticks < 2 {
						ticks = 2
					}
					d.duckExpiry = ticks
				} else {
					d.duckExpiry = duckDefaultTicks
				}
				d.pose = dinoDucking
				d.duckTicks = d.duckExpiry
			}
		case "esc", "q":
			return d, func() tea.Msg { return dinoExitMsg{} }
		}

	case dinoGameOver:
		if key == " " || key == "space" {
			d.state = dinoWaiting
			d.obstacles = nil
			d.score = 0
			d.dinoY = 0
			d.pose = dinoRunning
			return d, nil
		}
		if key == "esc" || key == "q" {
			return d, func() tea.Msg { return dinoExitMsg{} }
		}
	}

	return d, nil
}

func (d dinoGameModel) tick() (dinoGameModel, tea.Cmd) {
	if d.state != dinoPlaying {
		return d, nil
	}

	d.frame++
	d.tickCount++

	// Gravity / jump physics
	if d.dinoY > 0 {
		d.jumpVel -= gravity
		d.dinoY += d.jumpVel
		if d.dinoY <= 0 {
			d.dinoY = 0
			d.jumpVel = 0
			d.pose = dinoRunning
		}
	}

	// Auto-release duck after timer expires
	if d.isDucking() {
		d.duckTicks--
		if d.duckTicks <= 0 {
			d.pose = dinoRunning
			d.duckTicks = 0
		}
	}

	// Calculate how many whole columns to move this tick (sub-pixel accumulation).
	d.subPixel += d.speed10
	move := d.subPixel / 10
	d.subPixel = d.subPixel % 10

	// Move obstacles left
	for i := range d.obstacles {
		d.obstacles[i].x -= move
	}

	// Remove off-screen obstacles
	alive := d.obstacles[:0]
	for _, o := range d.obstacles {
		if o.x+o.width > 0 {
			alive = append(alive, o)
		}
	}
	d.obstacles = alive

	// Spawn new obstacles
	// Tick-based spawn timer — guarantees constant recovery time regardless of speed.
	d.spawnTimer--
	if d.spawnTimer <= 0 {
		d.obstacles = append(d.obstacles, d.spawnObstacle())
		d.spawnTimer = minSpawnTicks + rand.IntN(maxSpawnTicks-minSpawnTicks)
	}

	// Collision detection
	if d.checkCollision() {
		d.state = dinoGameOver
		return d, nil
	}

	d.score++

	// Gradual speed ramp
	if d.tickCount%speedUpEvery == 0 && d.speed10 < maxSpeed10 {
		d.speed10 += speedIncrement
	}

	// Scroll ground
	if d.width > 0 {
		d.groundOff = (d.groundOff + move) % d.width
	}

	return d, tea.Tick(dinoTickInterval, func(time.Time) tea.Msg { return dinoTickMsg{} })
}

func (d dinoGameModel) spawnObstacle() obstacle {
	kind := rand.IntN(3)
	switch kind {
	case obstBird:
		return obstacle{
			x:      d.width,
			kind:   obstBird,
			width:  5,
			height: 2,
			birdY:  dinoDuckHeight + rand.IntN(2), // fly above ducking dino (rows 3-4)
		}
	case obstLargeCactus:
		return obstacle{
			x:      d.width,
			kind:   obstLargeCactus,
			width:  6,
			height: 4,
		}
	default:
		return obstacle{
			x:      d.width,
			kind:   obstSmallCactus,
			width:  6,
			height: 3,
		}
	}
}

func (d dinoGameModel) checkCollision() bool {
	// Use forgiving collision box (smaller than visual sprite).
	var dw, dh int
	if d.isDucking() {
		dw, dh = dinoDuckCollW, dinoDuckHeight
	} else {
		dw, dh = dinoCollisionW, dinoStandHeight
	}
	dx1 := dinoPosX + dinoCollisionOff
	dx2 := dx1 + dw
	dy1 := d.dinoY
	dy2 := d.dinoY + dh

	for _, o := range d.obstacles {
		ox1 := o.x
		ox2 := o.x + o.width
		oy1 := o.birdY
		oy2 := o.birdY + o.height

		if dx1 < ox2 && dx2 > ox1 && dy1 < oy2 && dy2 > oy1 {
			return true
		}
	}
	return false
}

func (d dinoGameModel) withSize(w, h int) dinoGameModel {
	d.width = w
	d.height = h
	return d
}

// -- Rendering --

var (
	dinoScoreStyle = lipgloss.NewStyle().
			Foreground(themeYellow).
			Bold(true)
	dinoTitleStyle = lipgloss.NewStyle().
			Foreground(themeSapphire).
			Bold(true)
	dinoGroundStyle = lipgloss.NewStyle().
			Foreground(themeOverlay0)
	dinoSpriteStyle = lipgloss.NewStyle().
			Foreground(themeText)
	dinoGameOverStyle = lipgloss.NewStyle().
				Foreground(themeRed).
				Bold(true)
)

// View renders the dino game within the pet panel area.
// Layout (petHeight = 5 lines):
//
//	line 0: score / status
//	line 1: sky (jump row 2)
//	line 2: play row 1 (dino top when standing)
//	line 3: ground level (dino bottom when standing)
//	line 4: ground decoration
func (d dinoGameModel) View() string {
	if d.width < 20 || d.height < 8 {
		return dinoTitleStyle.Render("too small")
	}

	switch d.state {
	case dinoWaiting:
		return d.renderWaiting()
	case dinoPlaying:
		return d.renderPlaying()
	case dinoGameOver:
		return d.renderGameOver()
	}
	return ""
}

func (d dinoGameModel) renderWaiting() string {
	var sb strings.Builder

	// Vertically center the content
	spriteLines := len(dinoRunFrame0)
	contentLines := 1 + spriteLines + 2 // title + sprite + blank + prompt
	topPad := (d.height - contentLines) / 2
	for i := 0; i < topPad; i++ {
		sb.WriteString("\n")
	}

	sb.WriteString(dinoCenterText(dinoTitleStyle.Render("DINO GAME"), d.width) + "\n")
	for _, line := range dinoRunFrame0 {
		sb.WriteString(dinoCenterText(dinoSpriteStyle.Render(line), d.width) + "\n")
	}
	sb.WriteString("\n")
	sb.WriteString(dinoCenterText(dinoTitleStyle.Render("SPACE to start · ESC to exit"), d.width))

	return sb.String()
}

func (d dinoGameModel) renderPlaying() string {
	// Play area: total height minus score line and ground line
	playRows := d.height - 2
	if playRows < 3 {
		playRows = 3
	}
	grid := make([][]rune, playRows)
	for i := range grid {
		grid[i] = make([]rune, d.width)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	// Place dino sprite — groundRow is just past the bottom of the grid
	d.blitSprite(grid, d.dinoSprite(), dinoPosX, playRows, d.dinoY)

	// Place obstacles
	for _, o := range d.obstacles {
		sprite := d.obstacleSprite(o)
		d.blitSprite(grid, sprite, o.x, playRows, o.birdY)
	}

	var sb strings.Builder

	// Line 0: score
	scoreText := dinoScoreStyle.Render(fmt.Sprintf("%04d", d.score))
	speedText := lipgloss.NewStyle().Foreground(themeOverlay1).Render(fmt.Sprintf("s%.1f", float64(d.speed10)/10))
	pad := d.width - lipgloss.Width(scoreText) - lipgloss.Width(speedText)
	if pad < 1 {
		pad = 1
	}
	sb.WriteString(speedText + strings.Repeat(" ", pad) + scoreText + "\n")

	// Lines 1-3: play field
	for _, row := range grid {
		sb.WriteString(dinoSpriteStyle.Render(string(row)) + "\n")
	}

	// Line 4: ground
	sb.WriteString(dinoGroundStyle.Render(d.renderGround()))

	return sb.String()
}

// dinoSprite returns the current dino sprite based on pose and frame.
func (d dinoGameModel) dinoSprite() []string {
	if d.isDucking() {
		return dinoDuckFrame
	}
	if d.frame%6 < 3 {
		return dinoRunFrame0
	}
	return dinoRunFrame1
}

// obstacleSprite returns the sprite for an obstacle.
func (d dinoGameModel) obstacleSprite(o obstacle) []string {
	switch o.kind {
	case obstSmallCactus:
		return spriteSmallCactus
	case obstLargeCactus:
		return spriteLargeCactus
	case obstBird:
		if d.frame%8 < 4 {
			return spriteBirdFrame0
		}
		return spriteBirdFrame1
	default:
		return spriteSmallCactus
	}
}

// blitSprite writes a sprite onto the rune grid.
// x is the left column, groundRow is the row index just below the grid,
// yOff is the vertical offset above ground (0 = feet on ground).
func (d dinoGameModel) blitSprite(grid [][]rune, sprite []string, x, groundRow, yOff int) {
	spriteH := len(sprite)
	// Sprite bottom sits at groundRow - 1 - yOff
	bottomRow := groundRow - 1 - yOff

	for si, line := range sprite {
		gridRow := bottomRow - (spriteH - 1 - si)
		if gridRow < 0 || gridRow >= len(grid) {
			continue
		}
		col := x
		for _, ch := range line {
			if col >= 0 && col < len(grid[gridRow]) && ch != ' ' {
				grid[gridRow][col] = ch
			}
			col++
		}
	}
}

func (d dinoGameModel) renderGround() string {
	groundRunes := []rune("▁▁▁▁▁ ▁▁▁ ▁▁▁▁▁▁ ▁▁ ▁▁▁▁")
	patLen := len(groundRunes)
	result := make([]rune, d.width)
	for i := range result {
		idx := (i + d.groundOff) % patLen
		result[i] = groundRunes[idx]
	}
	return string(result)
}

func (d dinoGameModel) renderGameOver() string {
	var sb strings.Builder

	contentLines := 5 // title + blank + score + blank + prompt
	topPad := (d.height - contentLines) / 2
	for i := 0; i < topPad; i++ {
		sb.WriteString("\n")
	}

	sb.WriteString(dinoCenterText(dinoGameOverStyle.Render("GAME OVER"), d.width) + "\n")
	sb.WriteString("\n")
	sb.WriteString(dinoCenterText(dinoScoreStyle.Render(fmt.Sprintf("Score: %04d", d.score)), d.width) + "\n")
	sb.WriteString("\n")
	sb.WriteString(dinoCenterText(dinoTitleStyle.Render("SPACE retry · ESC exit"), d.width))

	return sb.String()
}

func dinoCenterText(s string, width int) string {
	visLen := lipgloss.Width(s)
	if visLen >= width {
		return s
	}
	pad := (width - visLen) / 2
	return strings.Repeat(" ", pad) + s
}
