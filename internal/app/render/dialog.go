package render

import (
	"crawler/internal/app/core"
	"image/color"
	"strconv"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// dialog modal geometry. Wider than the chest card so a paragraph of
// conversation text wraps to a readable measure.
const (
	dialogCardWidth   = int32(600)
	dialogTextPadX    = int32(24)
	dialogLineH       = int32(26)
	dialogChoiceRowH  = int32(34)
	dialogMinBodyH    = int32(52)
	dialogSpeakerBand = int32(40)
	dialogBodyGap     = int32(16)  // gap between the body text block and the row list
	dialogBottomPad   = int32(40)  // padding below the row list down to the card's bottom edge
	dialogMinCardH    = modalMinCardH // floor so a short one-line node still reads as a card
	// dialogMaxBodyLines bounds how many wrapped text lines the card shows so
	// a pathologically long node body can't grow the card past the screen.
	// Authored lines are short; a longer beat should be split across nodes.
	dialogMaxBodyLines = 10
)

// dialogModalCache memoizes the per-node wrapped body lines, choice views, and
// formatted choice-row labels. DrawDialogModal paints over the live exploration
// render EVERY frame, so without this it would re-run wrapTextLines' per-word
// rl.MeasureTextEx cgo calls (plus the overflow ellipsis-trim loop) and rebuild
// the choice slice + concat each row label 60×/sec for fixed text. The node body
// and its choice set are stable for the life of a node — the player is locked in
// the modal, so gating inputs can't change between frames — so the cache rebuilds
// only when the node (id + body text) or inner width changes. Same rebuild-on-
// change pattern as actionLogCache.
var dialogModalCache struct {
	nodeID    string
	text      string
	innerW    float32
	bodyLines []string
	views     []core.DialogChoiceView
	labels    []string
}

// dialogModalContent returns the wrapped body lines, choice views, and
// preformatted row labels for the current node, rebuilding the cache only when
// the node or width changes (see dialogModalCache).
func dialogModalContent(g *core.GameState, font rl.Font, node core.DialogNode, innerW float32) (bodyLines []string, views []core.DialogChoiceView, labels []string) {
	c := &dialogModalCache
	if c.nodeID == g.Dialog.NodeID && c.text == node.Text && c.innerW == innerW &&
		len(c.views) == len(node.Choices) {
		return c.bodyLines, c.views, c.labels
	}

	bodyLines = wrapTextLines(font, node.Text, FontBody, innerW)
	if len(bodyLines) == 0 {
		bodyLines = []string{""}
	}
	if len(bodyLines) > dialogMaxBodyLines {
		bodyLines = bodyLines[:dialogMaxBodyLines]
		// Trim runes off the last kept line until it + the ellipsis fits the
		// card's inner width — appending " …" to an already-full wrapped line
		// would otherwise clip past the right edge.
		const ellipsis = " …"
		spacing := canonicalSpacing(FontBody)
		last := bodyLines[dialogMaxBodyLines-1]
		for last != "" && rl.MeasureTextEx(font, last+ellipsis, FontBody, spacing).X > innerW {
			r := []rune(last)
			last = strings.TrimRight(string(r[:len(r)-1]), " ")
		}
		bodyLines[dialogMaxBodyLines-1] = last + ellipsis
	}

	views = core.DialogChoiceViews(g)
	labels = make([]string, len(views))
	for i, v := range views {
		label := strconv.Itoa(i+1) + ". " + v.Choice.Label
		if v.Disabled && v.Reason != "" {
			label += "  (" + v.Reason + ")"
		}
		labels[i] = label
	}

	c.nodeID = g.Dialog.NodeID
	c.text = node.Text
	c.innerW = innerW
	c.bodyLines = bodyLines
	c.views = views
	c.labels = labels
	return bodyLines, views, labels
}

// DrawDialogModal paints the branching-conversation overlay: the current
// speaker's name (in their nameplate tint), the wrapped line of text, and
// either the selectable choice rows or a Continue affordance. Gamepad-first:
// Up/Down move the choice cursor, A confirms (pick a choice / continue), B
// skips the whole conversation. Rendered after the world like the other
// explore modals; no-ops when no dialog is open.
func DrawDialogModal(g *core.GameState, assets Resources) {
	if !g.DialogOpen {
		return
	}
	node, ok := g.Dialog.Def.NodeByID(g.Dialog.NodeID)
	if !ok {
		return
	}
	font := assets.Font()

	// Body text wrapped to the card's inner width + the choice rows; memoized
	// per node so this per-frame overlay doesn't re-wrap / re-measure / rebuild
	// the choice slice every frame (see dialogModalCache).
	innerW := float32(dialogCardWidth - 2*dialogTextPadX)
	bodyLines, views, labels := dialogModalContent(g, font, node, innerW)
	bodyH := int32(len(bodyLines)) * dialogLineH
	if bodyH < dialogMinBodyH {
		bodyH = dialogMinBodyH
	}

	// A node either presents choices or a single Continue row.
	rowCount := len(views)
	if rowCount == 0 {
		rowCount = 1 // the Continue row
	}
	rowsH := int32(rowCount) * dialogChoiceRowH

	cardH := dialogSpeakerBand + bodyH + dialogBodyGap + rowsH + dialogBottomPad
	if cardH < dialogMinCardH {
		cardH = dialogMinCardH
	}
	card := drawModalScaffold(font, dialogCardWidth, cardH, "")
	cardX, cardY := int32(card.X), int32(card.Y)
	cardW := int32(card.Width)

	// Speaker nameplate.
	speakerName := core.DialogSpeakerName(node.SpeakerID)
	drawHeading(font, speakerName, cardX+int32(dialogTextPadX), cardY+modalHeadingInsetY, dialogSpeakerColor(node.SpeakerID))

	// Body text.
	textX := float32(cardX + dialogTextPadX)
	y := cardY + dialogSpeakerBand
	for _, line := range bodyLines {
		drawTextWithShadow(font, line, textX, float32(y), FontBody, textPrimary)
		y += dialogLineH
	}
	y += dialogBodyGap

	rowX := cardX + dialogTextPadX
	rowW := cardW - 2*dialogTextPadX
	rowRect := func(ry int32) rl.Rectangle { return SelectionRowRect(rowX, ry, rowW, dialogChoiceRowH) }

	if len(views) == 0 {
		// No-choice node: a single Continue row (always focused).
		label := node.ContinueLabel
		if label == "" {
			if node.NextNodeID == "" {
				label = "(End)"
			} else {
				label = "Continue"
			}
		}
		DrawSelectedRow(rowRect(y))
		drawTextWithShadow(font, label, float32(rowX), float32(y), FontBody, textPrimary)
	} else {
		for i, v := range views {
			focused := g.Dialog.ChoiceCursor == i
			if focused && !v.Disabled {
				DrawSelectedRow(rowRect(y))
			}
			col := rowTextColor(focused, v.Disabled, textDim)
			drawTextWithShadow(font, labels[i], float32(rowX), float32(y), FontBody, col)
			y += dialogChoiceRowH
		}
	}

	hints := []HintSeg{}
	if len(views) > 1 {
		hints = append(hints, Hint("Move", GlyphUpDown))
	}
	hints = append(hints, Hint("Select", GlyphA), Hint("Skip", GlyphB))
	drawModalFooterGlyphs(font, card, hints)
}

// dialogSpeakerColor resolves a speaker id to its nameplate color, falling
// back to the gilt heading color when the id is unregistered or untinted.
func dialogSpeakerColor(id core.DialogSpeakerID) color.RGBA {
	if sp, ok := core.DialogSpeakerByID(id); ok && sp.TintA != 0 {
		return color.RGBA{R: sp.TintR, G: sp.TintG, B: sp.TintB, A: sp.TintA}
	}
	return borderActive
}
