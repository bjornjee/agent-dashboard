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

// -- Tiny sprites for pet panel (3 play rows, ~40 cols) --

// Dino standing: 2 lines × 4 chars
var dinoRunFrame0 = []string{
	"▄█▀▄",
	"▀ ▀ ",
}

var dinoRunFrame1 = []string{
	"▄█▀▄",
	" ▀▀ ",
}

// Dino ducking: 1 line × 5 chars
var dinoDuckFrame = []string{
	"▄▄█▀▄",
}

// Small cactus: 1 line × 3 chars
var spriteSmallCactus = []string{
	"▀█▀",
}

// Large cactus: 2 lines × 3 chars
var spriteLargeCactus = []string{
	"█▄█",
	" █ ",
}

// Bird: 1 line × 3 chars
var spriteBirdFrame0 = []string{
	"▀▄▀",
}

var spriteBirdFrame1 = []string{
	"▄▀▄",
}

// Sprite dimensions for collision detection.
const (
	dinoStandWidth  = 4
	dinoStandHeight = 2
	dinoDuckWidth   = 5
	dinoDuckHeight  = 1
	dinoPosX        = 2 // fixed horizontal position of the dino
)

// -- Physics constants (tuned for 3-row play area) --

const (
	jumpVelocity    = 2  // jump 2 rows then fall (peaks at row 2, lands after 4 ticks)
	gravity         = 1  // 1 row per tick deceleration
	maxSpeed        = 4  // max scroll speed
	baseSpeed       = 1  // starting scroll speed
	speedUpInterval = 80 // ticks between speed increases
	minSpawnGap     = 12 // minimum ticks between spawns
	maxSpawnGap     = 25 // maximum ticks between spawns
)

// -- Game model --

type dinoGameModel struct {
	state   dinoGameState
	width   int
	height  int
	pose    dinoPose
	dinoY   int // vertical offset from ground (0 = ground, positive = up)
	jumpVel int
	frame   int // animation frame counter

	obstacles  []obstacle
	groundOff  int
	speed      int
	score      int
	tickCount  int
	spawnTimer int
}

func newDinoGameModel(w, h int) dinoGameModel {
	return dinoGameModel{
		state:      dinoWaiting,
		width:      w,
		height:     h,
		speed:      baseSpeed,
		spawnTimer: minSpawnGap,
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
			d.speed = baseSpeed
			d.obstacles = nil
			d.spawnTimer = minSpawnGap
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
				d.pose = dinoDucking
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

	// Move obstacles left
	for i := range d.obstacles {
		d.obstacles[i].x -= d.speed
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
	d.spawnTimer -= d.speed
	if d.spawnTimer <= 0 {
		d.obstacles = append(d.obstacles, d.spawnObstacle())
		d.spawnTimer = minSpawnGap + rand.IntN(maxSpawnGap-minSpawnGap)
	}

	// Collision detection
	if d.checkCollision() {
		d.state = dinoGameOver
		return d, nil
	}

	d.score++

	// Speed ramp
	if d.tickCount%speedUpInterval == 0 && d.speed < maxSpeed {
		d.speed++
	}

	// Scroll ground
	if d.width > 0 {
		d.groundOff = (d.groundOff + d.speed) % d.width
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
			width:  3,
			height: 1,
			birdY:  1 + rand.IntN(2), // fly at row 1 or 2
		}
	case obstLargeCactus:
		return obstacle{
			x:      d.width,
			kind:   obstLargeCactus,
			width:  3,
			height: 2,
		}
	default:
		return obstacle{
			x:      d.width,
			kind:   obstSmallCactus,
			width:  3,
			height: 1,
		}
	}
}

func (d dinoGameModel) checkCollision() bool {
	var dw, dh int
	if d.isDucking() {
		dw, dh = dinoDuckWidth, dinoDuckHeight
	} else {
		dw, dh = dinoStandWidth, dinoStandHeight
	}
	dx1 := dinoPosX
	dx2 := dinoPosX + dw
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
	if d.width < 10 || d.height < 4 {
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
	// Line 0: title
	sb.WriteString(dinoCenterText(dinoTitleStyle.Render("DINO"), d.width) + "\n")
	// Line 1: dino sprite
	sb.WriteString(dinoCenterText(dinoSpriteStyle.Render(dinoRunFrame0[0]), d.width) + "\n")
	sb.WriteString(dinoCenterText(dinoSpriteStyle.Render(dinoRunFrame0[1]), d.width) + "\n")
	// Line 3: prompt
	sb.WriteString(dinoCenterText(dinoTitleStyle.Render("SPACE start"), d.width) + "\n")
	// Line 4: ground
	sb.WriteString(dinoGroundStyle.Render(d.renderGround()))
	return sb.String()
}

func (d dinoGameModel) renderPlaying() string {
	// 3 rows of play area (rows 0=sky, 1=mid, 2=ground level)
	const playRows = 3
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
	speedText := lipgloss.NewStyle().Foreground(themeOverlay1).Render(fmt.Sprintf("s%d", d.speed))
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
	// Line 0: GAME OVER
	sb.WriteString(dinoCenterText(dinoGameOverStyle.Render("GAME OVER"), d.width) + "\n")
	// Line 1: empty
	sb.WriteString("\n")
	// Line 2: score
	sb.WriteString(dinoCenterText(dinoScoreStyle.Render(fmt.Sprintf("Score: %04d", d.score)), d.width) + "\n")
	// Line 3: prompt
	sb.WriteString(dinoCenterText(dinoTitleStyle.Render("SPACE retry"), d.width) + "\n")
	// Line 4: ground
	sb.WriteString(dinoGroundStyle.Render(d.renderGround()))
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
