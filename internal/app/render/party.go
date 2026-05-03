package render

import (
	"crawler/internal/app/core"
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	partyCardW    = float32(176)
	partyCardH    = float32(102)
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
		bg = mixColor(surfacePrimary, surfaceActiveTint, 0.55)
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
		rl.DrawTriangle(
			rl.NewVector2(centerX, y-10),
			rl.NewVector2(centerX-10, y+2),
			rl.NewVector2(centerX+10, y+2),
			borderTarget,
		)
	}
	if active && !down {
		cx := x + partyCardW - 16
		cy := y + 12
		rl.DrawTriangle(
			rl.NewVector2(cx-7, cy),
			rl.NewVector2(cx+7, cy),
			rl.NewVector2(cx, cy+10),
			borderActive,
		)
	}

	contentX := x + 16
	contentW := partyCardW - 26

	drawTextWithShadow(font, member.Name, contentX, y+10, 20, nameCol)

	if down {
		drawTextWithShadow(font, "DOWN", x+partyCardW-58, y+12, 14, rl.NewColor(220, 102, 102, 235))
	}

	hpFill := hpFillColor(member.HP, member.MaxHP)
	drawBar(font, contentX, y+42, contentW, 22, "HP", member.HP, member.MaxHP, hpFill, down)
	drawBar(font, contentX, y+70, contentW, 22, "MP", member.MP, member.MaxMP, barMP, down)
}

// DrawPartyRibbon renders the always-visible bottom party ribbon. Cards are
// pinned at fixed positions so they stay readable through attack bumps and
// victory dances. Active and selected states are surfaced from battle state.
func DrawPartyRibbon(g core.GameState, assets Resources) {
	if len(g.Party) == 0 {
		return
	}
	screenW := float32(rl.GetScreenWidth())
	screenH := float32(rl.GetScreenHeight())
	count := float32(len(g.Party))

	totalW := partyCardW*count + partyCardGap*(count-1)
	startX := (screenW - totalW) / 2
	if startX < 16 {
		startX = 16
	}
	y := screenH - partyCardH - ribbonBottom

	activeIdx := -1
	selectedIdx := -1
	if g.Battle.Phase == core.BattlePlayer {
		activeIdx = g.Battle.CurrentParty
		if g.Battle.ActionMode == core.ActionPartyTarget {
			selectedIdx = g.Battle.PartyTarget
		}
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
	return float32(rl.GetScreenHeight()) - partyCardH - ribbonBottom
}

func drawTextCentered(font rl.Font, text string, centerX, y, size float32, col color.RGBA) {
	measure := rl.MeasureTextEx(font, text, size, 1)
	pos := rl.NewVector2(centerX-measure.X/2, y)
	shadow := rl.NewVector2(pos.X+1, pos.Y+1)
	rl.DrawTextEx(font, text, shadow, size, 1, rl.NewColor(0, 0, 0, 200))
	rl.DrawTextEx(font, text, pos, size, 1, col)
}

func drawTextWithShadow(font rl.Font, text string, x, y, size float32, col color.RGBA) {
	pos := rl.NewVector2(x, y)
	shadow := rl.NewVector2(x+1, y+1)
	rl.DrawTextEx(font, text, shadow, size, 1, rl.NewColor(0, 0, 0, 200))
	rl.DrawTextEx(font, text, pos, size, 1, col)
}

func mixColor(a, b color.RGBA, t float32) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return color.RGBA{
		R: uint8(float32(a.R)*(1-t) + float32(b.R)*t),
		G: uint8(float32(a.G)*(1-t) + float32(b.G)*t),
		B: uint8(float32(a.B)*(1-t) + float32(b.B)*t),
		A: uint8(float32(a.A)*(1-t) + float32(b.A)*t),
	}
}
