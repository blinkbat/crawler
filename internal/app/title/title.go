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
	mode          titleMode
	cursor        int
	mapPaths      []string
	chosenMapPath string
	loadError     string
}

type titleMode int

const (
	modeMain titleMode = iota
	modeMapPicker
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

// mainMenuRowDef binds a title-menu row to its label producer and its
// confirm-press action. The order of mainMenuRows IS the draw order; the
// row index is the cursor position. Mirrors render.pauseMenuRows so both
// menus use the same "label + action" struct pattern instead of one being
// iota+labels-slice and the other being struct-table.
type mainMenuRowDef struct {
	Label  func() string
	Action func(s *State) Action
}

var mainMenuRows = []mainMenuRowDef{
	{
		Label: func() string { return "Adventure" },
		Action: func(s *State) Action {
			paths, _ := mapfile.List(core.MapsDir())
			s.mapPaths = paths
			s.cursor = 0
			s.mode = modeMapPicker
			return ActionNone
		},
	},
	{
		Label:  func() string { return "Editor" },
		Action: func(s *State) Action { return ActionOpenEditor },
	},
	{
		Label: func() string { return render.DisplayMenuRowLabel() },
		Action: func(s *State) Action {
			render.ToggleDisplayMode()
			return ActionNone
		},
	},
	{
		Label:  func() string { return "Quit" },
		Action: func(s *State) Action { return ActionQuit },
	},
}

// mainMenuLabels returns the title menu's row labels in draw order. Used
// by drawMainMenu's drawList call and as the cursor's wrap modulus.
func mainMenuLabels() []string {
	labels := make([]string, len(mainMenuRows))
	for i, row := range mainMenuRows {
		labels[i] = row.Label()
	}
	return labels
}

func updateMain(s *State) Action {
	s.cursor = input.CursorUpDown(s.cursor, len(mainMenuRows))
	// Esc and Q both quit from the main menu — there's nowhere to back up to.
	if input.QuitPressed() || input.BackPressed() {
		return ActionQuit
	}
	if input.ConfirmPressed() {
		s.loadError = ""
		if s.cursor >= 0 && s.cursor < len(mainMenuRows) {
			return mainMenuRows[s.cursor].Action(s)
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
	s.cursor = input.CursorUpDown(s.cursor, len(s.mapPaths))
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
	_, screenH := render.ScreenSize()

	title := "CRAWLER"
	// Game-name splash — the documented exception to the five-size
	// standard (see UI_STANDARDS.md). One-off, single-screen,
	// rendered exactly once at game launch.
	titleSize := float32(72)
	titleSpacing := float32(4)
	tm := rl.MeasureTextEx(font, title, titleSize, titleSpacing)
	titleX := render.CenterXF(tm.X)
	titleY := float32(screenH) * 0.18
	render.DrawTextWithShadow(font, title, titleX, titleY, titleSize, theme.TextPrimary)

	switch s.mode {
	case modeMain:
		drawMainMenu(s, font, theme, screenH)
	case modeMapPicker:
		drawMapPicker(s, font, theme, screenH)
	}

	if s.loadError != "" {
		drawError(font, theme, s.loadError, screenH)
	}
}

func drawMainMenu(s State, font rl.Font, theme render.Theme, screenH int32) {
	drawList(mainMenuLabels(), s.cursor, font, theme, screenH, "")
	drawHint(font, "Up/Down navigate   Enter select   Esc/Q quit", screenH)
}

func drawMapPicker(s State, font rl.Font, theme render.Theme, screenH int32) {
	header := "Choose a map"
	if len(s.mapPaths) == 0 {
		items := []string{"(no maps in maps/ -- press Esc and try Editor first)"}
		drawList(items, 0, font, theme, screenH, header)
		drawHint(font, "Esc to go back", screenH)
		return
	}
	items := make([]string, len(s.mapPaths))
	for i, p := range s.mapPaths {
		items[i] = core.MapIDFromPath(p)
	}
	drawList(items, s.cursor, font, theme, screenH, header)
	drawHint(font, "Up/Down navigate   Enter start   Esc back", screenH)
}

// drawList paints a vertical column of menu items centered horizontally.
// screenH controls the vertical anchor (items start at 42% of screen
// height); horizontal centering is handled by render.CenterXF which
// re-reads the screen width directly, so callers don't pass screenW.
func drawList(items []string, cursor int, font rl.Font, theme render.Theme, screenH int32, header string) {
	listY := float32(screenH) * 0.42
	if header != "" {
		sz := render.FontBody
		m := rl.MeasureTextEx(font, header, sz, 1)
		render.DrawTextWithShadow(font, header, render.CenterXF(m.X), listY-52, sz, theme.TextLabel)
	}
	for i, label := range items {
		size := render.FontHeading
		col := theme.TextMuted
		text := label
		if i == cursor {
			col = theme.BorderActive
			text = "> " + label
		}
		m := rl.MeasureTextEx(font, text, size, render.FontSpacingHeading)
		y := listY + float32(i)*44
		render.DrawTextWithShadow(font, text, render.CenterXF(m.X), y, size, col)
	}
}

func drawHint(font rl.Font, text string, screenH int32) {
	sw, _ := render.ScreenSizeF()
	render.DrawFooterHint(font, text, sw/2, float32(screenH)-36, render.FontSmall)
}

func drawError(font rl.Font, theme render.Theme, msg string, screenH int32) {
	size := render.FontSmall
	m := rl.MeasureTextEx(font, msg, size, 1)
	y := float32(screenH) - 60
	render.DrawTextWithShadow(font, msg, render.CenterXF(m.X), y, size, theme.BorderDanger)
}
