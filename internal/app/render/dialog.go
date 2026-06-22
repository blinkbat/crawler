package render

import (
	"crawler/internal/app/core"
	"image/color"
	"strconv"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// dialog modal geometry. Wider than the chest card for a readable text measure.
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
	// dialogMaxBodyLines caps wrapped lines so a long body can't grow the card past the screen.
	dialogMaxBodyLines = 10
)

// dialogModalCache memoizes the per-node wrapped body lines so DrawDialogModal doesn't re-run
// wrapTextLines' per-word MeasureTextEx every frame; rebuilt only when the node id/text or width changes.
// Choice views/labels are NOT cached — DialogChoiceViews derives Disabled/Reason from live state
// that can cross a threshold on the same node, so they're rebuilt every frame.
var dialogModalCache struct {
	nodeID    string
	text      string
	innerW    float32
	bodyLines []string
}

// dialogModalContent returns wrapped body lines (cached), choice views, and row labels (rebuilt
// every call, live state). See dialogModalCache.
func dialogModalContent(g *core.GameState, font rl.Font, node core.DialogNode, innerW float32) (bodyLines []string, views []core.DialogChoiceView, labels []string) {
	c := &dialogModalCache
	if c.nodeID == g.Dialog.NodeID && c.text == node.Text && c.innerW == innerW {
		bodyLines = c.bodyLines
	} else {
		bodyLines = wrapTextLines(font, node.Text, FontBody, innerW)
		if len(bodyLines) == 0 {
			bodyLines = []string{""}
		}
		if len(bodyLines) > dialogMaxBodyLines {
			bodyLines = bodyLines[:dialogMaxBodyLines]
			// Trim the last kept line until it + ellipsis fits, else " …" clips past the right edge.
			const ellipsis = " …"
			spacing := canonicalSpacing(FontBody)
			last := bodyLines[dialogMaxBodyLines-1]
			for last != "" && rl.MeasureTextEx(font, last+ellipsis, FontBody, spacing).X > innerW {
				r := []rune(last)
				last = strings.TrimRight(string(r[:len(r)-1]), " ")
			}
			bodyLines[dialogMaxBodyLines-1] = last + ellipsis
		}
		c.nodeID = g.Dialog.NodeID
		c.text = node.Text
		c.innerW = innerW
		c.bodyLines = bodyLines
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
	return bodyLines, views, labels
}

// DrawDialogModal paints the branching-conversation overlay: speaker nameplate, wrapped text, and
// either choice rows or a Continue affordance. A confirms, B skips. No-op when no dialog is open.
func DrawDialogModal(g *core.GameState, assets Resources) {
	if !g.DialogOpen {
		return
	}
	node, ok := g.Dialog.Def.NodeByID(g.Dialog.NodeID)
	if !ok {
		return
	}
	font := assets.Font()

	// Body lines memoized per node (see dialogModalCache).
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
		drawModalListRow(rowX, y, rowW, dialogChoiceRowH, true, func() {
			drawTextWithShadow(font, label, float32(rowX), float32(y), FontBody, textPrimary)
		})
	} else {
		for i, v := range views {
			focused := g.Dialog.ChoiceCursor == i
			col := rowTextColor(focused, v.Disabled, textDim)
			ry := y
			drawModalListRow(rowX, ry, rowW, dialogChoiceRowH, focused && !v.Disabled, func() {
				drawTextWithShadow(font, labels[i], float32(rowX), float32(ry), FontBody, col)
			})
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
