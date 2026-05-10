// Package title is the launch screen: pick Adventure (which then picks a
// map from disk) or Editor.
package title

import (
	"crawler/internal/app/core"
	"crawler/internal/app/core/mapfile"
	"crawler/internal/app/input"
	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type Action int

const (
	ActionNone Action = iota
	ActionStartAdventure
	ActionOpenEditor
	ActionQuit
)

type State struct {
	mode          int
	cursor        int
	mapPaths      []string
	chosenMapPath string
	loadError     string
}

const (
	modeMain      = 0
	modeMapPicker = 1
)

func New() State { return State{mode: modeMain} }

func (s State) ChosenMapPath() string { return s.chosenMapPath }

// SetLoadError lets the run loop surface a "couldn't open map" message back
// onto the title screen after a failed adventure-start attempt.
func (s *State) SetLoadError(msg string) {
	s.loadError = msg
	s.mode = modeMain
}

func Update(s *State) Action {
	switch s.mode {
	case modeMain:
		return updateMain(s)
	case modeMapPicker:
		return updateMapPicker(s)
	}
	return ActionNone
}

func updateMain(s *State) Action {
	const itemCount = 3
	if input.UpPressed() {
		s.cursor = core.WrapIndex(s.cursor-1, itemCount)
	}
	if input.DownPressed() {
		s.cursor = core.WrapIndex(s.cursor+1, itemCount)
	}
	// Esc and Q both quit from the main menu — there's nowhere to back up to.
	if input.QuitPressed() || input.BackPressed() {
		return ActionQuit
	}
	if input.ConfirmPressed() {
		s.loadError = ""
		switch s.cursor {
		case 0:
			paths, _ := mapfile.List(core.MapsDir())
			s.mapPaths = paths
			s.cursor = 0
			s.mode = modeMapPicker
		case 1:
			return ActionOpenEditor
		case 2:
			return ActionQuit
		}
	}
	return ActionNone
}

func updateMapPicker(s *State) Action {
	if input.BackPressed() {
		s.mode = modeMain
		s.cursor = 0
		return ActionNone
	}
	if len(s.mapPaths) == 0 {
		return ActionNone
	}
	if input.UpPressed() {
		s.cursor = core.WrapIndex(s.cursor-1, len(s.mapPaths))
	}
	if input.DownPressed() {
		s.cursor = core.WrapIndex(s.cursor+1, len(s.mapPaths))
	}
	if input.ConfirmPressed() {
		s.chosenMapPath = s.mapPaths[s.cursor]
		return ActionStartAdventure
	}
	return ActionNone
}

func Draw(s State, assets render.Resources) {
	font := assets.Font()
	theme := assets.Theme()
	rl.ClearBackground(rl.NewColor(8, 12, 24, 255))
	screenW := int32(rl.GetScreenWidth())
	screenH := int32(rl.GetScreenHeight())

	title := "CRAWLER"
	titleSize := float32(72)
	titleSpacing := float32(4)
	tm := rl.MeasureTextEx(font, title, titleSize, titleSpacing)
	titleX := float32(screenW)/2 - tm.X/2
	titleY := float32(screenH) * 0.18
	render.DrawTextWithShadow(font, title, titleX, titleY, titleSize, theme.TextPrimary)

	switch s.mode {
	case modeMain:
		drawMainMenu(s, font, theme, screenW, screenH)
	case modeMapPicker:
		drawMapPicker(s, font, theme, screenW, screenH)
	}

	if s.loadError != "" {
		drawError(font, theme, s.loadError, screenW, screenH)
	}
}

func drawMainMenu(s State, font rl.Font, theme render.Theme, screenW, screenH int32) {
	items := []string{"Adventure", "Editor", "Quit"}
	drawList(items, s.cursor, font, theme, screenW, screenH, "")
	drawHint(font, theme, "Up/Down navigate   Enter select   Esc/Q quit", screenW, screenH)
}

func drawMapPicker(s State, font rl.Font, theme render.Theme, screenW, screenH int32) {
	header := "Choose a map"
	if len(s.mapPaths) == 0 {
		items := []string{"(no maps in maps/ -- press Esc and try Editor first)"}
		drawList(items, 0, font, theme, screenW, screenH, header)
		drawHint(font, theme, "Esc to go back", screenW, screenH)
		return
	}
	items := make([]string, len(s.mapPaths))
	for i, p := range s.mapPaths {
		items[i] = core.MapIDFromPath(p)
	}
	drawList(items, s.cursor, font, theme, screenW, screenH, header)
	drawHint(font, theme, "Up/Down navigate   Enter start   Esc back", screenW, screenH)
}

func drawList(items []string, cursor int, font rl.Font, theme render.Theme, screenW, screenH int32, header string) {
	listY := float32(screenH) * 0.42
	if header != "" {
		sz := float32(20)
		m := rl.MeasureTextEx(font, header, sz, 1.5)
		render.DrawTextWithShadow(font, header, float32(screenW)/2-m.X/2, listY-52, sz, theme.TextLabel)
	}
	for i, label := range items {
		size := float32(28)
		col := theme.TextMuted
		text := label
		if i == cursor {
			col = theme.BorderActive
			text = "> " + label
		}
		m := rl.MeasureTextEx(font, text, size, 1.5)
		x := float32(screenW)/2 - m.X/2
		y := listY + float32(i)*44
		render.DrawTextWithShadow(font, text, x, y, size, col)
	}
}

func drawHint(font rl.Font, theme render.Theme, text string, screenW, screenH int32) {
	size := float32(14)
	m := rl.MeasureTextEx(font, text, size, 1)
	x := float32(screenW)/2 - m.X/2
	y := float32(screenH) - 36
	render.DrawTextWithShadow(font, text, x, y, size, theme.TextHint)
}

func drawError(font rl.Font, theme render.Theme, msg string, screenW, screenH int32) {
	size := float32(16)
	m := rl.MeasureTextEx(font, msg, size, 1)
	x := float32(screenW)/2 - m.X/2
	y := float32(screenH) - 60
	render.DrawTextWithShadow(font, msg, x, y, size, theme.BorderDanger)
}
