package render

import (
	"crawler/internal/app/core"
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	partyCardW    = float32(184)
	partyCardH    = float32(118)
	partyCardGap  = float32(16)
	ribbonBottom  = float32(20)
	ribbonTopRoom = float32(0)
)

// drawPartyCard renders a single party member card. The class accent stripe
// keeps members recognizable at a glance even when names are short.
func drawPartyCard(font rl.Font, member core.PartyMember, x, y float32, active, selected, down bool) {
	classCol := partyClassPresentationFor(member.Class).turnColor
	accent := classCol
	bg := surfacePrimary
	border := borderSoft
	nameCol := textPrimary

	switch {
	case down:
		bg = surfaceDownTint
		border = borderDim
		accent = rl.NewColor(120, 110, 116, 200)
		nameCol = textDim
	case selected:
		bg = surfaceTargetTint
		border = borderTarget
		accent = borderTarget
	case active:
		bg = core.MixColor(surfacePrimary, surfaceActiveTint, 0.55)
		border = borderActive
	}

	ix, iy := int32(x), int32(y)
	iw, ih := int32(partyCardW), int32(partyCardH)

	if active && !down {
		halo := fadeColor(borderActive, 0.32+0.32*pulse(1.4))
		drawPanelOutline(ix-3, iy-3, iw+6, ih+6, halo)
	}
	if selected {
		drawPanelOutline(ix-3, iy-3, iw+6, ih+6, borderTarget)
	}

	drawCard(ix, iy, iw, ih, bg, border, accent)

	if selected {
		centerX := x + partyCardW/2
		drawArrowMarker(rl.NewVector2(centerX, y+2), 0, -12, 10, borderTarget)
	}
	if active && !down {
		cx := x + partyCardW - 16
		cy := y + 12
		drawArrowMarker(rl.NewVector2(cx, cy), 0, 10, 7, borderActive)
	}

	contentX := x + 16
	contentW := partyCardW - 28

	drawTextWithShadow(font, member.Name, contentX, y+12, 21, nameCol)

	if down {
		drawTextWithShadow(font, "DOWN", x+partyCardW-58, y+14, 14, rl.NewColor(220, 102, 102, 235))
	} else if member.PoisonTurns > 0 {
		// Poison takes priority over the Defending label since it's the
		// shorter-lived, more actionable status (heal vs ride it out).
		flicker := 0.65 + 0.35*pulse(2.6)
		col := rl.NewColor(160, 220, 100, 240)
		drawTextWithShadow(font, "POISONED", x+partyCardW-88, y+14, 14, fadeColor(col, flicker))
	} else if member.Defending {
		drawTextWithShadow(font, "DEFENDING", x+partyCardW-94, y+14, 14, rl.NewColor(132, 196, 255, 240))
	}

	hpFill := hpFillColor(member.HP, member.MaxHP)
	drawBar(font, contentX, y+44, contentW, 30, "HP", member.HP, member.MaxHP, hpFill, down)
	drawBar(font, contentX, y+80, contentW, 30, "MP", member.MP, member.MaxMP, barMP, down)
}

// DrawPartyRibbon renders the always-visible bottom party ribbon. Cards are
// pinned at fixed positions so they stay readable through attack bumps and
// victory dances. Active and selected states are surfaced from battle state.
func DrawPartyRibbon(g core.GameState, assets Resources) {
	if len(g.Party) == 0 {
		return
	}
	_, screenH := screenSizeF()
	count := float32(len(g.Party))

	totalW := partyCardW*count + partyCardGap*(count-1)
	startX := centerXF(totalW)
	if startX < 16 {
		startX = 16
	}
	y := screenH - partyCardH - ribbonBottom

	activeIdx := core.ActiveActorIndex(&g)
	selectedIdx := -1
	if targetingAlly(g) {
		selectedIdx = core.HighlightedAllyIndex(&g)
	}

	for i, member := range g.Party {
		x := startX + (partyCardW+partyCardGap)*float32(i)
		drawPartyCard(
			assets.hudFont,
			member,
			x, y,
			i == activeIdx && member.HP > 0,
			i == selectedIdx && member.HP > 0,
			member.HP <= 0,
		)
	}
}

// PartyRibbonTopY reports the screen Y coordinate of the top of the party
// ribbon, so other panels can stack cleanly above it.
func PartyRibbonTopY() float32 {
	_, h := screenSizeF()
	return h - partyCardH - ribbonBottom
}

func drawTextCentered(font rl.Font, text string, centerX, y, size float32, col color.RGBA) {
	measure := rl.MeasureTextEx(font, text, size, 1)
	drawTextWithShadow(font, text, centerX-measure.X/2, y, size, col)
}

