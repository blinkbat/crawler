package render

import (
	"fmt"
	"image/color"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// partyStatusVisuals is the canonical per-status visual table. Indexed
// by core.PartyStatusKind so render surfaces (party card label, Tome
// Stats badge, panels overlay, etc.) read the color + flicker from a
// single registry instead of each switching on the enum by hand.
// Length-asserted at init against core.PartyStatusCount so a future
// status kind without a row trips the program at startup, not silently
// in a draw call.
//
// Color tokens come from UI_STANDARDS.md "Per-status accents"; the
// flicker bit marks "something is wrong" statuses (Ingested / DoTs /
// lockouts) so the bad news pulses, off for static states (Down /
// Defending / None).
var partyStatusVisuals = [core.PartyStatusCount]struct {
	Col     rl.Color
	Flicker bool
}{
	core.PartyStatusNone:      {rl.NewColor(220, 220, 220, 220), false},
	core.PartyStatusDown:      {statusDown, false},
	core.PartyStatusIngested:  {statusIngested, true},
	core.PartyStatusWebbed:    {statusWebbed, true},
	core.PartyStatusConfused:  {statusConfused, true},
	core.PartyStatusStunned:   {statusStun, true},
	core.PartyStatusAsleep:    {statusSleep, true},
	core.PartyStatusPoisoned:  {statusPoison, true},
	core.PartyStatusDefending: {statusDefending, false},
}

func init() {
	if len(partyStatusVisuals) != int(core.PartyStatusCount) {
		panic(fmt.Sprintf("partyStatusVisuals length %d != PartyStatusCount %d", len(partyStatusVisuals), core.PartyStatusCount))
	}
}

// partyStatusVisual returns the per-status text color and a flicker
// flag for the party card / panels Stats badge. Thin wrapper over the
// partyStatusVisuals table so callers don't dereference the array
// directly. Out-of-range kinds (only possible if a caller forges a
// PartyStatusKind value outside the enum) fall back to the None row.
func partyStatusVisual(kind core.PartyStatusKind) (col rl.Color, flicker bool) {
	if kind < 0 || int(kind) >= len(partyStatusVisuals) {
		v := partyStatusVisuals[core.PartyStatusNone]
		return v.Col, v.Flicker
	}
	v := partyStatusVisuals[kind]
	return v.Col, v.Flicker
}

// partyStatusTurnLabelCache pre-formats every "<LABEL> N" combination
// that the party card row can show — one entry per (kind, turns) pair
// in the small turn-count range that covers realistic durations. The
// card paints this label once per frame per afflicted member; without
// the cache the path runs fmt.Sprintf each time.
var partyStatusTurnLabelCache = func() [core.PartyStatusCount][partyStatusTurnLabelCacheSize]string {
	var out [core.PartyStatusCount][partyStatusTurnLabelCacheSize]string
	for k := core.PartyStatusKind(0); k < core.PartyStatusCount; k++ {
		base := core.PartyStatusLabel(k)
		for n := 0; n < partyStatusTurnLabelCacheSize; n++ {
			out[k][n] = fmt.Sprintf("%s %d", base, n)
		}
	}
	return out
}()

const partyStatusTurnLabelCacheSize = 20

// partyStatusTurnLabel returns "<LABEL>" for boolean statuses (turns == 0)
// or "<LABEL> N" for counted statuses. Reads the precomputed table for
// the common turn range; falls back to fmt.Sprintf only when turns
// exceed the cached window.
func partyStatusTurnLabel(kind core.PartyStatusKind, turns int) string {
	base := core.PartyStatusLabel(kind)
	if turns <= 0 {
		return base
	}
	if int(kind) >= 0 && int(kind) < int(core.PartyStatusCount) && turns < partyStatusTurnLabelCacheSize {
		return partyStatusTurnLabelCache[kind][turns]
	}
	return fmt.Sprintf("%s %d", base, turns)
}

// partyStatusLabelMeasureCache memoizes MeasureTextEx for the small set
// of party-status label strings produced by partyStatusTurnLabel. Each
// card with an active status would otherwise re-measure the same label
// every frame.
var partyStatusLabelMeasureCache measureCache

func measurePartyStatusLabel(font rl.Font, label string) rl.Vector2 {
	return partyStatusLabelMeasureCache.measure(font, label, FontTiny, 1)
}

// partyNamePlusLabels memoizes the "<Name> +" badge string per member
// name so the always-visible ribbon's hot path is a map lookup instead
// of a fresh concat. Font-independent (it's just the string), so unlike
// a measure cache it needs no font-ID invalidation.
var partyNamePlusLabels = make(map[string]string, 8)

// partyNameSpaceWidth measures "<Name> " at FontBody (for positioning the
// "+" overlay), sharing the generic measureCache machinery.
var partyNameSpaceWidth measureCache

func partyNamePlusBadge(name string) string {
	if v, ok := partyNamePlusLabels[name]; ok {
		return v
	}
	v := name + " +"
	partyNamePlusLabels[name] = v
	return v
}

func measurePartyNameWithSpace(font rl.Font, name string) rl.Vector2 {
	return partyNameSpaceWidth.measure(font, name+" ", FontBody, 1)
}

const (
	partyCardW   = float32(184)
	partyCardH   = float32(118)
	partyCardGap = float32(16)
	// activeCardLift raises the active member's card above the ribbon row
	// so "whose turn is it" reads at a glance, on top of the bold halo.
	// Raised from 14 → 24 so the lift is the primary turn cue now that the
	// in-world pyramid + glow were removed (see DrawPartySprites).
	activeCardLift = float32(24)
	// ribbonBottom is the bottom-edge margin for the party ribbon.
	// Routed through hudEdgePad so the bottom margin matches the
	// minimap's top margin (and every other HUD panel's edge
	// distance). Earlier passes used 20 which was four pixels off
	// the rest of the HUD's edge convention.
	ribbonBottom = float32(hudEdgePad)
)

// drawCardScrim paints the shared dim wash (theme's surfaceDimScrim) over a
// card's rounded rect, matching drawCard's corner radius so the dim hugs the
// card body. Used to recede NON-active party cards during a member's turn so
// the lifted active card reads as "whose turn" at a glance; the targeted ally
// is skipped so heal/item targeting stays legible (see DrawPartyRibbon).
func drawCardScrim(x, y, w, h int32) {
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	roundness := fixedRoundnessFor(w, h, cornerRadius)
	rl.DrawRectangleRounded(rect, roundness, 8, surfaceDimScrim)
}

// drawPartyCard renders a single party member card. The class accent stripe
// keeps members recognizable at a glance even when names are short. `dim`
// requests the inactive-member wash (applied last, over the whole card).
func drawPartyCard(font rl.Font, member core.PartyMember, x, y float32, active, selected, down, dim bool) {
	classCol := partyClassPresentationFor(member.Class).turnColor
	accent := classCol
	bg := surfacePrimary
	border := borderSoft
	nameCol := textPrimary

	switch {
	case down:
		bg = surfaceDownTint
		border = borderDim
		accent = accentPartyDown
		nameCol = textDim
	case selected:
		bg = surfaceTargetTint
		border = borderTarget
		accent = borderTarget
	case active:
		bg = selectedGlassTint(surfacePrimary, 0.8)
		border = borderActive
	}

	// Raise the active card above the row so it physically stands out.
	if active && !down {
		y -= activeCardLift
	}

	ix, iy := int32(x), int32(y)
	iw, ih := int32(partyCardW), int32(partyCardH)

	if active && !down {
		// Layered gilt halo — solid inner ring + pulsing wider outer ring — so
		// the active card reads as unmistakably lit. Shared with the battle
		// roster's targeted-row halo via drawSelectionHalo.
		drawSelectionHalo(ix-3, iy-3, iw+6, ih+6, borderActive, pulseActiveActor(), false)
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

	// Class glyph immediately left of the name — the small gilt
	// sigil 90s D&D box-art used as a class shorthand. Drawn in the
	// class accent so it harmonises with the card's left stripe.
	glyphR := float32(8)
	glyphCX := contentX + glyphR
	glyphCY := y + 12 + FontBody/2
	glyphCol := classCol
	if down {
		glyphCol = fadeColor(classCol, 0.45)
	}
	drawClassMedallion(glyphCX, glyphCY, glyphR+4, glyphCol, down)
	drawClassGlyph(glyphCX, glyphCY, glyphR, member.Class, glyphCol)
	nameX := contentX + glyphR*2 + 8

	// Append a soft "+" badge to the name when the member has
	// unspent stat OR skill points. Tinted yellow to draw the eye
	// to the Tome menu — that's where the player goes to allocate.
	// Routes through core.HasUnspentPoints so a future contract
	// change (free respec, refund-on-death, etc.) updates the
	// badge automatically.
	hasPoints := core.HasUnspentPoints(member)
	nameText := member.Name
	if hasPoints {
		nameText = partyNamePlusBadge(member.Name)
	}
	drawTextWithShadow(font, nameText, nameX, y+12, FontBody, nameCol)
	if hasPoints && !down {
		// Re-paint just the "+" in the level-up accent color so the
		// signal pops even when the name itself is in textPrimary.
		// Width of "name " (no plus) only changes when the name does,
		// so route through the per-member measurement cache.
		nameMeasure := measurePartyNameWithSpace(font, member.Name)
		plusX := nameX + nameMeasure.X
		drawTextWithShadow(font, "+", plusX, y+12, FontBody, inkAccent)
	}

	// Status label: walks the canonical PartyStatus priority ladder so
	// the card never disagrees with the Tome's Stats-tab badge. Both
	// surfaces resolve through core.PartyStatus; only the per-status
	// COLOR and flicker live render-side (those are visual choices
	// the core layer shouldn't carry).
	kind, turns := core.PartyStatus(member)
	if kind != core.PartyStatusNone {
		label := partyStatusTurnLabel(kind, turns)
		col, flicker := partyStatusVisual(kind)
		labelSize := FontTiny
		measure := measurePartyStatusLabel(font, label)
		labelX := x + partyCardW - measure.X - 12
		labelCol := col
		if flicker {
			labelCol = fadeColor(col, pulseFlicker())
		}
		drawTextWithShadow(font, label, labelX, y+14, labelSize, labelCol)
	}

	hpFill := hpFillColor(member.HP, member.MaxHP)
	drawBar(font, contentX, y+44, contentW, 30, "HP", member.HP, member.MaxHP, hpFill, down)
	drawBar(font, contentX, y+80, contentW, 30, "MP", member.MP, member.MaxMP, barMP, down)

	// Dim wash over inactive cards (painted last, over everything) so the
	// lifted active card pops. The active and the targeted-ally cards opt
	// out — DrawPartyRibbon only sets dim for the rest.
	if dim {
		drawCardScrim(ix, iy, iw, ih)
	}
}

func drawClassMedallion(cx, cy, r float32, col rl.Color, muted bool) {
	outer := fadeColor(woodDark, 0.88)
	rim := fadeColor(giltDim, 0.78)
	inner := fadeColor(glassWarm, 0.85)
	if muted {
		rim = fadeColor(rim, 0.45)
		inner = fadeColor(inner, 0.45)
	}
	rl.DrawCircleV(rl.NewVector2(cx+1, cy+2), r+1, fadeColor(shadowHeavy, 0.24))
	rl.DrawCircleV(rl.NewVector2(cx, cy), r+1.5, outer)
	rl.DrawCircleV(rl.NewVector2(cx, cy), r, rim)
	rl.DrawCircleV(rl.NewVector2(cx, cy), r-2.5, inner)
	rl.DrawCircleV(rl.NewVector2(cx-r*0.25, cy-r*0.25), 1.6, fadeColor(inkPrimary, 0.32))
	drawDiamondPip(cx+r*0.72, cy-r*0.55, 1.2, fadeColor(col, 0.70))
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
	if startX < float32(hudEdgePad) {
		startX = float32(hudEdgePad)
	}
	y := screenH - partyCardH - ribbonBottom

	activeIdx := core.ActiveActorIndex(&g)
	selectedIdx := -1
	if targetingAlly(g) {
		selectedIdx = core.HighlightedAllyIndex(&g)
	}

	// Dim the OTHER cards only when a party member is actually up
	// (ActiveActorIndex is -1 on enemy turns / between turns, so the ribbon
	// stays at full brightness then rather than greying out wholesale).
	dimOthers := activeIdx >= 0 && activeIdx < len(g.Party) && core.PartyMemberAvailable(g.Party, activeIdx)

	for i, member := range g.Party {
		x := startX + (partyCardW+partyCardGap)*float32(i)
		// Active / selected glow only paints on a member who can act
		// AND be targeted this turn — i.e. not ingested. The turn queue
		// already skips ingested actors and the targeting cyclers route
		// through AvailablePartyTargets, so neither indicator should
		// land on an ingested member organically; this gates defensively
		// in case a stale PartyTarget gets left pointing at someone the
		// mantrap swallowed mid-action.
		available := core.PartyMemberAvailable(g.Party, i)
		active := i == activeIdx && available
		selected := i == selectedIdx && available
		// Dim every non-active card except the one being targeted (its
		// green highlight must stay legible during heal/item targeting).
		dim := dimOthers && !active && !selected
		drawPartyCard(
			assets.hudFont,
			member,
			x, y,
			active,
			selected,
			member.HP <= 0,
			dim,
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
