// Package title is the launch screen: pick Adventure (which then picks a
// map from disk) or Editor.
package title

import (
	"math"

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
	ActionContinue
	ActionOpenEditor
	ActionQuit
)

// mapChoice pairs a map's on-disk path with its display name, so the picker can't
// desync a name from its path (they were two cursor-indexed slices).
type mapChoice struct {
	Path string
	Name string
}

type State struct {
	mode          titleMode
	cursor        int
	mapChoices    []mapChoice // path + display name, derived once when the picker opens
	chosenMapPath string
	loadError     string
	// hasSave caches SaveExists() once (can't change while the title is up),
	// avoiding a per-frame os.Stat.
	hasSave bool
	// rows: active row set, built once in New(). labels: reused buffer refilled
	// each draw (the Display row's label is dynamic; slice never reallocates).
	rows   []mainMenuRowDef
	labels []string
}

type titleMode int

const (
	modeMain titleMode = iota
	modeMapPicker
)

func New() State {
	hasSave := core.SaveExists()
	rows := mainRows(hasSave)
	return State{
		mode:    modeMain,
		hasSave: hasSave,
		rows:    rows,
		labels:  make([]string, len(rows)),
	}
}

func (s State) ChosenMapPath() string { return s.chosenMapPath }

// SetLoadError surfaces a "couldn't open map" message on the title screen.
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
	default:
		// A new mode that forgets an Update case would be a dead screen.
		panic("title: unhandled mode in Update")
	}
}

// mainMenuRowDef binds a row to its label producer and confirm action.
// mainMenuRows order IS the draw order; index is the cursor position.
type mainMenuRowDef struct {
	Label  func() string
	Action func(s *State) Action
}

// continueRow loads the most recent save. Prepended (default cursor) only when
// a save exists; a fresh install shows Adventure first.
var continueRow = mainMenuRowDef{
	Label:  func() string { return "Continue" },
	Action: func(*State) Action { return ActionContinue },
}

var mainMenuRows = []mainMenuRowDef{
	{
		Label: func() string { return "Adventure" },
		Action: func(s *State) Action {
			paths, _ := mapfile.List(core.MapsDir())
			// Derive path + display name once (the list is fixed while the picker is open).
			s.mapChoices = make([]mapChoice, len(paths))
			for i, p := range paths {
				s.mapChoices[i] = mapChoice{Path: p, Name: core.MapIDFromPath(p)}
			}
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

// mainRows returns the active rows, prepending Continue when hasSave. Both
// updateMain and drawMainMenu read this so cursor→row can't drift.
func mainRows(hasSave bool) []mainMenuRowDef {
	if hasSave {
		return append([]mainMenuRowDef{continueRow}, mainMenuRows...)
	}
	return mainMenuRows
}

// menuLabels refills s.labels (reused buffer) with the current row labels in
// draw order. The Display row's label is fresh each frame; the others constant.
func (s State) menuLabels() []string {
	for i, row := range s.rows {
		s.labels[i] = row.Label()
	}
	return s.labels
}

func updateMain(s *State) Action {
	rows := s.rows
	s.cursor = input.CursorUpDown(s.cursor, len(rows))
	// Quit and Back both exit — nowhere to back up to from the main menu.
	if input.QuitPressed() || input.BackPressed() {
		return ActionQuit
	}
	if input.ConfirmPressed() {
		s.loadError = ""
		if s.cursor >= 0 && s.cursor < len(rows) {
			return rows[s.cursor].Action(s)
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
	if len(s.mapChoices) == 0 {
		return ActionNone
	}
	s.cursor = input.CursorUpDown(s.cursor, len(s.mapChoices))
	if input.ConfirmPressed() {
		s.chosenMapPath = s.mapChoices[s.cursor].Path
		return ActionStartAdventure
	}
	return ActionNone
}

func Draw(s State, assets render.Resources) {
	font := assets.Font()
	theme := assets.Theme()
	screenW, screenH := render.ScreenSize()
	// ClearBackground first — load-bearing depth-buffer wipe on this raylib build
	// (see AGENTS.md). The color is overdrawn by DrawCandlelitBackdrop.
	rl.ClearBackground(rl.NewColor(8, 10, 20, 255))
	render.DrawCandlelitBackdrop(screenW, screenH)

	title := "CRAWLER"
	// Splash sizing is the documented exception to the five-size standard
	// (UI_STANDARDS.md); values live in the const block below, not as literals.
	titleSize := titleSplashSize
	titleSpacing := titleSplashSpacing
	tm := rl.MeasureTextEx(font, title, titleSize, titleSpacing)
	titleX := render.CenterXF(tm.X)
	titleY := float32(screenH) * 0.18
	// Embossed gilt wordmark: cast shadow, gold body, cream speculum up-left.
	rl.DrawTextEx(font, title, rl.NewVector2(titleX+3, titleY+4), titleSize, titleSpacing, rl.NewColor(0, 0, 0, 210))
	rl.DrawTextEx(font, title, rl.NewVector2(titleX, titleY), titleSize, titleSpacing, theme.BorderActive)
	rl.DrawTextEx(font, title, rl.NewVector2(titleX-1, titleY-1), titleSize, titleSpacing, rl.NewColor(255, 246, 220, 130))
	// Shimmer — the wordmark redrawn in cream, scissor-clipped to a band that
	// sweeps every titleSheenPeriod sec; clipping the TEXT lights only the letters.
	sweepSpan := tm.X + 2*titleSheenBandW
	sweepT := float32(math.Mod(rl.GetTime()/titleSheenPeriod, 1))
	bandX := titleX - titleSheenBandW + sweepT*sweepSpan
	rl.BeginScissorMode(int32(bandX), int32(titleY), int32(titleSheenBandW), int32(tm.Y+8))
	rl.DrawTextEx(font, title, rl.NewVector2(titleX, titleY), titleSize, titleSpacing, rl.NewColor(255, 244, 206, 96))
	rl.EndScissorMode()

	// Gilt rule beneath the title, flanked by fleurons. Width = title + slack
	// so the ornament frames the wordmark.
	ruleY := titleY + tm.Y + 14
	ruleW := tm.X + 48
	ruleX := render.CenterXF(ruleW)
	render.DrawTitleRule(ruleX, ruleY, ruleW)

	switch s.mode {
	case modeMain:
		drawMainMenu(s, font, theme, screenH)
	case modeMapPicker:
		drawMapPicker(s, font, theme, screenH)
	default:
		// A mode added to Update but missed here would render a blank screen.
		panic("title: unhandled mode in Draw")
	}

	if s.loadError != "" {
		drawError(font, theme, s.loadError, screenH)
	}
}

func drawMainMenu(s State, font rl.Font, theme render.Theme, screenH int32) {
	drawList(s.menuLabels(), s.cursor, font, theme, screenH, "")
	// Controller-first glyph hints — the first surface shown.
	drawHint(font, []render.HintSeg{
		render.Hint("Navigate", render.GlyphUpDown),
		render.Hint("Select", render.GlyphA),
	}, screenH)
}

func drawMapPicker(s State, font rl.Font, theme render.Theme, screenH int32) {
	header := "Choose a map"
	if len(s.mapChoices) == 0 {
		drawList([]string{"(no maps in maps/ — open the Editor to make one)"}, 0, font, theme, screenH, header)
		drawHint(font, []render.HintSeg{render.Hint("Back", render.GlyphB)}, screenH)
		return
	}
	names := make([]string, len(s.mapChoices))
	for i, c := range s.mapChoices {
		names[i] = c.Name
	}
	drawList(names, s.cursor, font, theme, screenH, header)
	drawHint(font, []render.HintSeg{
		render.Hint("Navigate", render.GlyphUpDown),
		render.Hint("Start", render.GlyphA),
		render.Hint("Back", render.GlyphB),
	}, screenH)
}

// Title-screen layout anchors, so a rebalance touches one block.
const (
	// Wordmark sizing — the documented exception to the five-size standard.
	titleSplashSize        = float32(72)
	titleSplashSpacing     = float32(4)
	titleListAnchorFrac    = float32(0.42) // menu list vertical anchor (frac of screenH)
	titleListRowStride     = float32(44)   // gap between rows
	titleListHeaderOffset  = float32(52)   // list top up to the header
	titleFleuronGap        = float32(22)   // active label edge to flanking fleuron
	titleHintFooterOffset  = float32(36)   // bottom edge up to nav-hint baseline
	titleErrorFooterOffset = float32(60)   // bottom edge up to error baseline
	// Masthead shimmer: a band sweeps the wordmark every titleSheenPeriod sec.
	// A glint, not a marquee.
	titleSheenPeriod = 5.6
	titleSheenBandW  = float32(110)
)

// drawList paints a centered column of menu items (CenterXF, so no screenW);
// screenH sets the vertical anchor. Active row gets flanking fleurons.
func drawList(items []string, cursor int, font rl.Font, theme render.Theme, screenH int32, header string) {
	listY := float32(screenH) * titleListAnchorFrac
	if header != "" {
		sz := render.FontBody
		m := rl.MeasureTextEx(font, header, sz, render.FontSpacingBody)
		render.DrawTextWithShadow(font, header, render.CenterXF(m.X), listY-titleListHeaderOffset, sz, theme.TextLabel)
	}
	for i, label := range items {
		size := render.FontHeading
		col := theme.TextMuted
		active := i == cursor
		if active {
			col = theme.BorderActive
		}
		m := rl.MeasureTextEx(font, label, size, render.FontSpacingHeading)
		y := listY + float32(i)*titleListRowStride
		x := render.CenterXF(m.X)
		// Engraved rows — same top-lit metal-leaf gradient as in-game headings.
		render.DrawEngravedText(font, label, x, y, size, col)
		if active {
			flCY := y + m.Y/2
			render.DrawFleuron(x-titleFleuronGap, flCY, 5, theme.BorderActive)
			render.DrawFleuron(x+m.X+titleFleuronGap, flCY, 5, theme.BorderActive)
		}
	}
}

func drawHint(font rl.Font, segs []render.HintSeg, screenH int32) {
	sw, _ := render.ScreenSizeF()
	render.DrawHintBar(font, segs, sw/2, float32(screenH)-titleHintFooterOffset, render.FontSmall)
}

func drawError(font rl.Font, theme render.Theme, msg string, screenH int32) {
	size := render.FontSmall
	m := rl.MeasureTextEx(font, msg, size, render.FontSpacingBody)
	y := float32(screenH) - titleErrorFooterOffset
	render.DrawTextWithShadow(font, msg, render.CenterXF(m.X), y, size, theme.BorderDanger)
}
