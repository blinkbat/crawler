package editor

import (
	"fmt"

	"crawler/internal/app/render"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// help.go — the read-only keyboard-shortcut reference (modalHelp, ? key). The
// scrolling palette footer (paletteHints) is the at-a-glance subset; this is the
// exhaustive two-column list so no binding is undiscoverable.

// helpRow is one reference line. An empty key marks a section header (desc is the
// header text).
type helpRow struct{ key, desc string }

func helpHeader(title string) helpRow { return helpRow{desc: title} }

// init guards against menu-binding rot: every editorMenus hotkey must be
// documented in helpColumns (the reverse — gesture-only rows — is intentionally
// not asserted). A rebind that skips the help now panics at startup instead of
// silently leaving the reference stale.
func init() {
	documented := map[string]bool{}
	for _, col := range helpColumns {
		for _, row := range col {
			if row.key != "" {
				documented[row.key] = true
			}
		}
	}
	for _, g := range editorMenus {
		for _, it := range g.items {
			if it.hotkey != "" && !documented[it.hotkey] {
				panic(fmt.Sprintf("editor: menu %q command %q binds %q but help.go helpColumns doesn't list it — document the binding there", g.label, it.label, it.hotkey))
			}
		}
	}
}

// helpColumns is the two-column reference body. Grouped by section. The init
// above asserts every menu hotkey appears here; gesture-only rows are hand-kept.
var helpColumns = [2][]helpRow{
	{
		helpHeader("Paint"),
		{"L-drag", "Paint with the current brush"},
		{"Alt+click", "Eyedropper — sample the tile"},
		{"Shift+drag", "Filled rectangle"},
		{"Ctrl+click", "Flood-fill the region"},
		{"Ctrl+Shift+F", "Fill the whole layer"},
		{"[  ]", "Brush size"},
		{"1..9", "Pick brush (Sh+1..9 for 10-18)"},
		{"R-click", "Tile menu (edit/erase/faces)"},
		helpHeader("Layers & levels"),
		{"Tab / Shift+Tab", "Next / previous layer"},
		{"Alt+1..6", "Jump to a layer"},
		{"PgUp / PgDn", "Active elevation level"},
		{"Alt (tap)", "Toggle tile-glyph overlay"},
		helpHeader("Selection"),
		{"Select tool", "Drag a marquee region"},
		{"Ctrl+A", "Select the whole map"},
		{"Ctrl+C", "Copy region (+entities)"},
		{"Ctrl+X", "Cut region (+entities)"},
		{"Ctrl+V", "Paste at the cursor"},
		{"Ctrl+D", "Duplicate the selection"},
		{"drag inside", "Move the selection's contents"},
		{"Esc", "Clear the selection"},
	},
	{
		helpHeader("View"),
		{"I", "3D / top-down view"},
		{"Q / E", "Turn the 3D camera"},
		{"wheel  or  + / -", "Zoom"},
		{"arrows  or  R-drag", "Pan"},
		{"Ctrl+0", "Zoom to fit"},
		{"Home", "Reset view"},
		{"G", "Center on player start"},
		{"T", "Cycle day/night preview"},
		{"L", "Recent-messages panel"},
		helpHeader("File"),
		{"Ctrl+S", "Save"},
		{"Ctrl+O", "Open"},
		{"Ctrl+N", "New map"},
		{"Ctrl+Z", "Undo"},
		{"Ctrl+Y", "Redo"},
		helpHeader("Test & entities"),
		{"F5", "Playtest from the start tile"},
		{"Ctrl+F5", "Playtest from the cursor"},
		{"R", "Rotate prop facing / player start"},
		helpHeader("Help"),
		{"?", "This reference"},
		{"Esc / click", "Close"},
	},
}

func openHelpModal(s *State) { openModal(s, modalHelp) }

// updateHelpModal: read-only viewer, so any key/click dismisses.
func updateHelpModal(s *State) Action {
	if anyDismissPressed() {
		closeModal(s)
	}
	return ActionNone
}

const (
	helpModalW     = float32(720)
	helpColW       = float32(340)
	helpRowH       = float32(20)
	helpHeaderH    = float32(58) // title band above the columns
	helpFooterH    = float32(34)
	helpKeyColW    = float32(126) // key text column width within a help column
	helpSectionGap = float32(6)   // extra space before a section header
)

// helpColumnHeight is a column's body pixel height — section headers each cost an
// extra helpSectionGap on top of a row, so counting entries alone undersizes the card.
func helpColumnHeight(col []helpRow) float32 {
	var h float32
	for _, row := range col {
		if row.key == "" {
			h += helpSectionGap
		}
		h += helpRowH
	}
	return h
}

func drawHelpModal(s *State, font rl.Font, theme render.Theme) {
	bodyH := helpColumnHeight(helpColumns[0])
	if h := helpColumnHeight(helpColumns[1]); h > bodyH {
		bodyH = h
	}
	ph := helpHeaderH + bodyH + helpFooterH
	r := drawModalHeader(font, theme, helpModalW, ph, "KEYBOARD SHORTCUTS", theme.BorderActive)

	colGap := (r.Width - 2*helpColW) / 3
	for c := 0; c < 2; c++ {
		x := r.X + colGap + float32(c)*(helpColW+colGap)
		y := r.Y + helpHeaderH
		for _, row := range helpColumns[c] {
			if row.key == "" {
				y += helpSectionGap
				render.DrawHeading(font, row.desc, int32(x), int32(y), theme.BorderStrong)
				y += helpRowH
				continue
			}
			render.DrawRichText(font, row.key, rl.NewVector2(x, y), editorFontLabel, 1, editorGold)
			render.DrawRichText(font, row.desc, rl.NewVector2(x+helpKeyColW, y), editorFontLabel, 1, theme.TextPrimary)
			y += helpRowH
		}
	}

	drawModalFooterHint(font, r, "Esc / Enter / click   close", theme)
}
