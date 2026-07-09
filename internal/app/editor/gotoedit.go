package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"
	"strconv"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// gotoedit.go — the jump-to-tile dialog (modalGoto): type an X,Z to recenter the
// view on a large map, or save/recall session view bookmarks. Bookmarks are
// in-session only (cleared on New/Open, not persisted to disk).

// gotoBookmark is one saved view mark (a tile the view recenters on) with an optional
// name. Persisted per-map in editorprefs (see bookmarksForMap / saveBookmarksForMap).
type gotoBookmark struct {
	x, z int
	name string
}

const (
	gotoModalW     = float32(360)
	gotoRowH       = float32(26)
	gotoMaxBmShown = 8 // bookmark rows shown at once; more scroll (mouse wheel)
)

// gotoLayout holds the modalGoto rects (draw + hit-test share it). bmRows covers
// only the on-screen window [top,end); absolute index = top + local.
type gotoLayout struct {
	card, xField, zField, nameField, goBtn, markBtn rl.Rectangle
	bmRows                                          []gotoRowRects
	top, end                                        int
}

type gotoRowRects struct{ row, del rl.Rectangle }

// gotoNameRowH is the extra height the optional-name field row adds (field + its label).
const gotoNameRowH = textFieldH + 22

func gotoModalH(bmCount int) float32 {
	return listModalHeight(min(bmCount, gotoMaxBmShown), gotoRowH) + gotoNameRowH
}

func gotoLayoutFor(s *State) gotoLayout {
	card := centeredCardRect(gotoModalW, gotoModalH(len(s.bookmarks)))
	x, w := cardContentBox(card)
	half := (w - 10) / 2
	y := modalBodyTop(card)
	l := gotoLayout{card: card}
	l.xField = rl.NewRectangle(x, y, half, textFieldH)
	l.zField = rl.NewRectangle(x+half+10, y, half, textFieldH)
	y += gotoNameRowH
	l.nameField = rl.NewRectangle(x, y, w, textFieldH)
	y += textFieldH + 8
	l.goBtn = rl.NewRectangle(x, y, half, textFieldH)
	l.markBtn = rl.NewRectangle(x+half+10, y, half, textFieldH)
	y += textFieldH + 10
	shown := min(len(s.bookmarks), gotoMaxBmShown)
	l.top, l.end = scrollWindow(s.bookmarkCursor, len(s.bookmarks), shown)
	for i := l.top; i < l.end; i++ {
		row := rl.NewRectangle(x, y+float32(i-l.top)*gotoRowH, w, gotoRowH-4)
		del := rl.NewRectangle(row.X+row.Width-24, row.Y, 24, row.Height)
		l.bmRows = append(l.bmRows, gotoRowRects{row: row, del: del})
	}
	return l
}

// openGotoModal opens the jump-to-tile dialog, seeding the fields with the last
// hovered tile (else the player start).
func openGotoModal(s *State) {
	s.modal = modalGoto
	s.gotoField = 0
	s.bookmarkCursor = 0
	s.gotoName = ""
	tx, tz := s.hoverX, s.hoverZ
	if !s.area.InBounds(tx, tz) {
		tx, tz = s.area.StartTileX, s.area.StartTileZ
	}
	s.gotoX = strconv.Itoa(tx)
	s.gotoZ = strconv.Itoa(tz)
}

func updateGotoModal(s *State) Action {
	l := gotoLayoutFor(s)
	switch s.gotoField {
	case 0:
		pumpPrintableASCII(&s.gotoX, 5, acceptDigit, nil)
	case 1:
		pumpPrintableASCII(&s.gotoZ, 5, acceptDigit, nil)
	default:
		pumpPrintableASCII(&s.gotoName, defaultTextFieldMaxLen, acceptPrintable, nil)
	}
	if editorTabPressed() {
		s.gotoField = (s.gotoField + 1) % 3 // X → Z → Name → X
		return ActionNone
	}
	if editorCommitPressed() {
		gotoConfirm(s)
		return ActionNone
	}
	if editorCancelPressed() {
		closeModal(s)
		return ActionNone
	}
	if w := rl.GetMouseWheelMove(); w != 0 && len(s.bookmarks) > gotoMaxBmShown {
		s.bookmarkCursor = clampCursor(s.bookmarkCursor-int(w), len(s.bookmarks))
	}
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		switch {
		case pointIn(mp, l.xField):
			s.gotoField = 0
		case pointIn(mp, l.zField):
			s.gotoField = 1
		case pointIn(mp, l.nameField):
			s.gotoField = 2
		case pointIn(mp, l.goBtn):
			gotoConfirm(s)
		case pointIn(mp, l.markBtn):
			bookmarkTypedTile(s)
		case !pointIn(mp, l.card):
			closeModal(s)
		default:
			for i := l.top; i < l.end; i++ {
				r := l.bmRows[i-l.top]
				if pointIn(mp, r.del) {
					s.bookmarks = removeModalListItem(s.bookmarks, i)
					s.bookmarkCursor = clampCursor(s.bookmarkCursor, len(s.bookmarks))
					saveBookmarksForMap(s.area.Path, s.bookmarks)
					return ActionNone
				}
				if pointIn(mp, r.row) {
					jumpToBookmark(s, i)
					return ActionNone
				}
			}
		}
	}
	return ActionNone
}

// gotoTypedTile parses the X/Z fields, clamped in-bounds; ok=false when empty/invalid.
func gotoTypedTile(s *State) (x, z int, ok bool) {
	xv, ex := strconv.Atoi(s.gotoX)
	zv, ez := strconv.Atoi(s.gotoZ)
	if ex != nil || ez != nil {
		return 0, 0, false
	}
	return core.Clamp(xv, 0, s.area.Width-1), core.Clamp(zv, 0, s.area.Height-1), true
}

func gotoConfirm(s *State) {
	x, z, ok := gotoTypedTile(s)
	if !ok {
		s.flash("Enter a tile X and Z")
		return
	}
	centerViewOnTile(s, x, z)
	closeModal(s)
}

// bookmarkTypedTile saves the field coordinate as a session view bookmark (deduped).
func bookmarkTypedTile(s *State) {
	x, z, ok := gotoTypedTile(s)
	if !ok {
		s.flash("Enter a tile X and Z to bookmark")
		return
	}
	for _, b := range s.bookmarks {
		if b.x == x && b.z == z {
			s.flash("Already bookmarked " + core.TileCoord(x, z))
			return
		}
	}
	s.bookmarks = append(s.bookmarks, gotoBookmark{x: x, z: z, name: strings.TrimSpace(s.gotoName)})
	saveBookmarksForMap(s.area.Path, s.bookmarks) // persist per-map (no-op for an unsaved map)
	s.gotoName = ""
	s.flash("Bookmarked " + core.TileCoord(x, z))
}

func jumpToBookmark(s *State, i int) {
	if i < 0 || i >= len(s.bookmarks) {
		return
	}
	b := s.bookmarks[i]
	centerViewOnTile(s, b.x, b.z)
	closeModal(s)
}

func drawGotoModal(s *State, font rl.Font, theme render.Theme) {
	l := gotoLayoutFor(s)
	drawModalHeaderAt(font, theme, l.card, "GO TO TILE", theme.BorderActive)

	drawLabel(font, "X", labelAbove(l.xField))
	drawTextField(font, l.xField, s.gotoX, s.gotoField == 0)
	drawLabel(font, "Z", labelAbove(l.zField))
	drawTextField(font, l.zField, s.gotoZ, s.gotoField == 1)
	drawLabel(font, "Bookmark name (optional)", labelAbove(l.nameField))
	drawTextField(font, l.nameField, s.gotoName, s.gotoField == 2)

	drawButton(font, l.goBtn, "Go (Enter)", false)
	drawButton(font, l.markBtn, "★ Bookmark", false)

	if len(s.bookmarks) == 0 {
		render.DrawRichText(font, "No bookmarks yet — Bookmark a tile to jump back later.",
			rl.NewVector2(l.card.X+modalContentInset, l.goBtn.Y+textFieldH+14), editorFontHint, 1, theme.TextHint)
	}
	for i := l.top; i < l.end; i++ {
		r := l.bmRows[i-l.top]
		b := s.bookmarks[i]
		label := "Jump to " + core.TileCoord(b.x, b.z)
		if b.name != "" {
			label = b.name + "  —  " + core.TileCoord(b.x, b.z)
		}
		render.DrawRichText(font, label,
			rl.NewVector2(r.row.X+6, r.row.Y+4), editorFontBody, 1, theme.TextPrimary)
		drawButton(font, r.del, "X", false)
	}
}
