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

// -- Sprites (half-block art) --

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

// Sprite dimensions for collision detection.
const (
	dinoStandWidth  = 10
	dinoStandHeight = 5
	dinoDuckWidth   = 9
	dinoDuckHeight  = 3
	dinoPosX        = 4 // fixed horizontal position of the dino
)

// -- Physics constants --

const (
	jumpVelocity    = 5
	gravity         = 1
	maxSpeed        = 6
	baseSpeed       = 1
	speedUpInterval = 60
	minSpawnGap     = 15
	maxSpawnGap     = 40
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
			width:  5,
			height: 2,
			birdY:  2 + rand.IntN(3),
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
	dinoObstacleStyle = lipgloss.NewStyle().
				Foreground(themeOverlay1)
	dinoGameOverStyle = lipgloss.NewStyle().
				Foreground(themeRed).
				Bold(true)
)

func (d dinoGameModel) View() string {
	if d.width < 30 || d.height < 10 {
		return dinoTitleStyle.Render("Terminal too small for dino game")
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
	topPad := (d.height - 8) / 2
	for i := 0; i < topPad; i++ {
		sb.WriteString("\n")
	}

	title := dinoTitleStyle.Render("DINO GAME")
	sb.WriteString(dinoCenterText(title, d.width) + "\n\n")

	for _, line := range dinoRunFrame0 {
		sb.WriteString(dinoCenterText(dinoSpriteStyle.Render(line), d.width) + "\n")
	}

	sb.WriteString("\n")
	prompt := dinoTitleStyle.Render("Press SPACE to start · ESC to exit")
	sb.WriteString(dinoCenterText(prompt, d.width))

	return sb.String()
}

func (d dinoGameModel) renderPlaying() string {
	// Use a rune grid for the play field.
	// groundRow is the line index where the ground sits (near bottom).
	playH := d.height - 2 // leave room for score line + ground line
	groundRow := playH    // ground is rendered separately after the grid

	// Build rune grid (rows × cols)
	grid := make([][]rune, playH)
	for i := range grid {
		grid[i] = make([]rune, d.width)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	// Place dino sprite
	d.blitSprite(grid, d.dinoSprite(), dinoPosX, groundRow, d.dinoY)

	// Place obstacles
	for _, o := range d.obstacles {
		sprite := d.obstacleSprite(o)
		d.blitSprite(grid, sprite, o.x, groundRow, o.birdY)
	}

	// Render to string
	var sb strings.Builder

	// Score line
	scoreText := dinoScoreStyle.Render(fmt.Sprintf("Score: %04d", d.score))
	speedText := lipgloss.NewStyle().Foreground(themeOverlay1).Render(fmt.Sprintf("Speed: %d", d.speed))
	pad := d.width - lipgloss.Width(scoreText) - lipgloss.Width(speedText) - 2
	if pad < 1 {
		pad = 1
	}
	sb.WriteString(speedText + strings.Repeat(" ", pad) + scoreText + "\n")

	// Play field
	for _, row := range grid {
		sb.WriteString(dinoSpriteStyle.Render(string(row)) + "\n")
	}

	// Ground
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
// x is the left column, groundRow is the grid row representing ground level,
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
	topPad := (d.height - 6) / 2
	for i := 0; i < topPad; i++ {
		sb.WriteString("\n")
	}

	title := dinoGameOverStyle.Render("G A M E   O V E R")
	sb.WriteString(dinoCenterText(title, d.width) + "\n\n")

	score := dinoScoreStyle.Render(fmt.Sprintf("Score: %04d", d.score))
	sb.WriteString(dinoCenterText(score, d.width) + "\n\n")

	prompt := dinoTitleStyle.Render("SPACE to retry · ESC to exit")
	sb.WriteString(dinoCenterText(prompt, d.width))

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
