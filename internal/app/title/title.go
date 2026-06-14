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

type State struct {
	mode          titleMode
	cursor        int
	mapPaths      []string
	mapNames      []string // display names for mapPaths, derived once when mapPaths is set (#6)
	chosenMapPath string
	loadError     string
	// hasSave caches core.SaveExists() once at construction. The save
	// file can't change while the title screen is up (you can only save
	// in-game), and New() runs on every return to title, so this avoids
	// an os.Stat syscall every frame from updateMain + drawMainMenu.
	hasSave bool
	// rows is the active main-menu row set, built once in New() (its
	// membership only depends on hasSave, which is fixed for the title's
	// lifetime). labels is a reusable buffer refilled in place each draw —
	// the Display row's label is dynamic, but the slice never reallocates.
	// Both replace per-frame allocations that updateMain + drawMainMenu used
	// to make every frame.
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
	default:
		// Mirrors run.go's scene-dispatch panic — a new title mode that
		// forgets an Update case would otherwise be a dead screen that
		// silently swallows all input.
		panic("title: unhandled mode in Update")
	}
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

// continueRow loads the most recent save. Prepended to the menu (so it's
// the default cursor position) only when a save file exists — a fresh
// install shows Adventure first. The run loop consumes ActionContinue by
// reading + applying the save.
var continueRow = mainMenuRowDef{
	Label:  func() string { return "Continue" },
	Action: func(*State) Action { return ActionContinue },
}

var mainMenuRows = []mainMenuRowDef{
	{
		Label: func() string { return "Adventure" },
		Action: func(s *State) Action {
			paths, _ := mapfile.List(core.MapsDir())
			s.mapPaths = paths
			// Derive the display names once here, not every frame in
			// drawMapPicker — the path list is fixed for as long as the
			// picker is open.
			s.mapNames = make([]string, len(paths))
			for i, p := range paths {
				s.mapNames[i] = core.MapIDFromPath(p)
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

// mainRows returns the active main-menu rows, prepending Continue when a
// save file is present (hasSave, cached in State). Both updateMain (cursor +
// confirm dispatch) and drawMainMenu (labels) read this so the row a cursor
// index resolves to can't drift between input and render.
func mainRows(hasSave bool) []mainMenuRowDef {
	if hasSave {
		return append([]mainMenuRowDef{continueRow}, mainMenuRows...)
	}
	return mainMenuRows
}

// menuLabels refills s.labels (a reused buffer, never reallocated) with the
// current row labels in draw order and returns it. The Display row's label is
// produced fresh each frame so the live mode shows; the others are constant.
func (s State) menuLabels() []string {
	for i, row := range s.rows {
		s.labels[i] = row.Label()
	}
	return s.labels
}

func updateMain(s *State) Action {
	rows := s.rows
	s.cursor = input.CursorUpDown(s.cursor, len(rows))
	// Quit (Q / Select) or Back (Esc / X / Circle) both exit from the main
	// menu — there's nowhere to back up to, so "cancel" and "quit" collapse
	// to the same action here.
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
	screenW, screenH := render.ScreenSize()
	// ClearBackground first — load-bearing for the depth-buffer wipe on this
	// raylib build (see AGENTS.md), independent of the opaque backdrop painted
	// over it. The color is immediately overdrawn by DrawCandlelitBackdrop.
	rl.ClearBackground(rl.NewColor(8, 10, 20, 255))
	// Candlelit backdrop — warm radial pool + drifting dust + grain over a deep
	// gradient — so the launch screen opens like a tome by candlelight rather
	// than a flat fill.
	render.DrawCandlelitBackdrop(screenW, screenH)

	title := "CRAWLER"
	// Game-name splash — the documented exception to the five-size
	// standard (see UI_STANDARDS.md). One-off, single-screen,
	// rendered exactly once at game launch.
	titleSize := float32(72)
	titleSpacing := float32(4)
	tm := rl.MeasureTextEx(font, title, titleSize, titleSpacing)
	titleX := render.CenterXF(tm.X)
	titleY := float32(screenH) * 0.18
	// Embossed gilt wordmark: deep cast shadow, gold body, then a fine cream
	// speculum nudged up-left so the letters read as raised, candle-struck
	// gold leaf rather than flat text.
	rl.DrawTextEx(font, title, rl.NewVector2(titleX+3, titleY+4), titleSize, titleSpacing, rl.NewColor(0, 0, 0, 210))
	rl.DrawTextEx(font, title, rl.NewVector2(titleX, titleY), titleSize, titleSpacing, theme.BorderActive)
	rl.DrawTextEx(font, title, rl.NewVector2(titleX-1, titleY-1), titleSize, titleSpacing, rl.NewColor(255, 246, 220, 130))
	// Shimmer pass — the wordmark redrawn in bright cream, scissor-clipped to
	// a narrow band that sweeps the title every titleSheenPeriod seconds (with
	// a dark beat between passes, since the band starts and ends fully
	// off-glyph). Clipping the TEXT redraw (not drawing a band over it) means
	// only the letterforms light up — the glint rides the leaf itself.
	sweepSpan := tm.X + 2*titleSheenBandW
	sweepT := float32(math.Mod(rl.GetTime()/titleSheenPeriod, 1))
	bandX := titleX - titleSheenBandW + sweepT*sweepSpan
	rl.BeginScissorMode(int32(bandX), int32(titleY), int32(titleSheenBandW), int32(tm.Y+8))
	rl.DrawTextEx(font, title, rl.NewVector2(titleX, titleY), titleSize, titleSpacing, rl.NewColor(255, 244, 206, 96))
	rl.EndScissorMode()

	// Gilt rule beneath the title, flanked by fleurons — the heraldic
	// banner divider 90s D&D box art used between a game title and
	// its menu. Width = title width + 24 px slack so the ornament
	// frames the wordmark.
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
		// Mirrors Update's (and run.go's) unhandled-mode panic — a new title
		// mode added to Update but missed here would render a blank screen.
		panic("title: unhandled mode in Draw")
	}

	if s.loadError != "" {
		drawError(font, theme, s.loadError, screenH)
	}
}

func drawMainMenu(s State, font rl.Font, theme render.Theme, screenH int32) {
	drawList(s.menuLabels(), s.cursor, font, theme, screenH, "")
	// Controller-first glyph hints (gamepad-first) — the first surface shown,
	// so it must read as a controller prompt, not a keyboard one.
	drawHint(font, []render.HintSeg{
		render.Hint("Navigate", render.GlyphUpDown),
		render.Hint("Select", render.GlyphA),
	}, screenH)
}

func drawMapPicker(s State, font rl.Font, theme render.Theme, screenH int32) {
	header := "Choose a map"
	if len(s.mapPaths) == 0 {
		items := []string{"(no maps in maps/ — open the Editor to make one)"}
		drawList(items, 0, font, theme, screenH, header)
		drawHint(font, []render.HintSeg{render.Hint("Back", render.GlyphB)}, screenH)
		return
	}
	drawList(s.mapNames, s.cursor, font, theme, screenH, header)
	drawHint(font, []render.HintSeg{
		render.Hint("Navigate", render.GlyphUpDown),
		render.Hint("Start", render.GlyphA),
		render.Hint("Back", render.GlyphB),
	}, screenH)
}

// Title-screen layout anchors. Pulled out of the inline literals
// drawList / drawHint / drawError / drawMainMenu used to repeat so a
// title-screen rebalance touches one block instead of half a dozen.
const (
	titleListAnchorFrac    = float32(0.42) // vertical anchor for the menu list (fraction of screenH)
	titleListRowStride     = float32(44)   // gap between menu rows
	titleListHeaderOffset  = float32(52)   // distance from list top up to the "Map:" header
	titleFleuronGap        = float32(22)   // horizontal gap from active label edge to flanking fleuron centre
	titleHintFooterOffset  = float32(36)   // distance from bottom edge up to the nav-hint baseline
	titleErrorFooterOffset = float32(60)   // distance from bottom edge up to the error-message baseline
	// Masthead shimmer: every titleSheenPeriod seconds a narrow bright band
	// sweeps across the gold-leaf wordmark (a scissored re-draw of the title
	// in cream), so the leaf catches the candle the way the gilt ornaments
	// already do. Slow + occasional — a glint, not a marquee.
	titleSheenPeriod = 5.6
	titleSheenBandW  = float32(110)
)

// drawList paints a vertical column of menu items centered horizontally.
// screenH controls the vertical anchor (titleListAnchorFrac of the
// screen); horizontal centering is handled by render.CenterXF which
// re-reads the screen width directly, so callers don't pass screenW.
//
// Active row gets the full heraldic treatment: text in inkAccent
// flanked by gilt fleurons on each side, like a banner herald
// announcing the selected option. Inactive rows render plain in
// muted text so the cursor pops without a hard chevron.
func drawList(items []string, cursor int, font rl.Font, theme render.Theme, screenH int32, header string) {
	listY := float32(screenH) * titleListAnchorFrac
	if header != "" {
		sz := render.FontBody
		m := rl.MeasureTextEx(font, header, sz, 1)
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
		// Engraved menu rows — the launch options wear the same top-lit
		// metal-leaf gradient the in-game headings do.
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
	m := rl.MeasureTextEx(font, msg, size, 1)
	y := float32(screenH) - titleErrorFooterOffset
	render.DrawTextWithShadow(font, msg, render.CenterXF(m.X), y, size, theme.BorderDanger)
}
