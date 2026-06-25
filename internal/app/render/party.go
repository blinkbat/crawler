package render

import (
	"fmt"
	"image/color"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// sharedStatusVisuals: per-status accent + glyph painted identically by party cards
// and enemy pills (battle.go). Only color + glyph are shared; flicker is not
// (party cards pulse, enemy pills don't). Burn/Bleed are enemy-only.
var sharedStatusVisuals = map[core.PartyStatusKind]struct {
	Col   rl.Color
	Glyph func(cx, cy, r float32, col rl.Color)
}{
	core.PartyStatusStunned:  {Col: statusStun, Glyph: drawStatusGlyphStunned},
	core.PartyStatusAsleep:   {Col: statusSleep, Glyph: drawStatusGlyphAsleep},
	core.PartyStatusPoisoned: {Col: statusPoison, Glyph: drawStatusGlyphPoisoned},
}

// partyStatusVisuals is the canonical per-status visual table (indexed by
// core.PartyStatusKind). init asserts every non-None kind carries a Glyph. Colors:
// UI_STANDARDS.md "Per-status accents"; flicker marks "something is wrong".
var partyStatusVisuals = [core.PartyStatusCount]struct {
	Col     rl.Color
	Flicker bool
	// Glyph paints at (cx,cy) radius r; every non-None kind has one (init-asserted).
	Glyph func(cx, cy, r float32, col rl.Color)
}{
	core.PartyStatusNone:      {Col: statusNoneAccent},
	core.PartyStatusDown:      {Col: statusDown, Glyph: drawStatusGlyphDown},
	core.PartyStatusIngested:  {Col: statusIngested, Flicker: true, Glyph: drawStatusGlyphIngested},
	core.PartyStatusWebbed:    {Col: statusWebbed, Flicker: true, Glyph: drawStatusGlyphWebbed},
	core.PartyStatusConfused:  {Col: statusConfused, Flicker: true, Glyph: drawStatusGlyphConfused},
	core.PartyStatusStunned:   {Col: sharedStatusVisuals[core.PartyStatusStunned].Col, Flicker: true, Glyph: sharedStatusVisuals[core.PartyStatusStunned].Glyph},
	core.PartyStatusAsleep:    {Col: sharedStatusVisuals[core.PartyStatusAsleep].Col, Flicker: true, Glyph: sharedStatusVisuals[core.PartyStatusAsleep].Glyph},
	core.PartyStatusPoisoned:  {Col: sharedStatusVisuals[core.PartyStatusPoisoned].Col, Flicker: true, Glyph: sharedStatusVisuals[core.PartyStatusPoisoned].Glyph},
	core.PartyStatusBlessed:   {Col: statusBlessed, Glyph: drawStatusGlyphBlessed},
	core.PartyStatusRegen:     {Col: statusRegen, Glyph: drawStatusGlyphRegen},
	core.PartyStatusShielded:  {Col: statusShielded, Glyph: drawStatusGlyphShielded},
	core.PartyStatusIceArmor:  {Col: statusIceArmor, Glyph: drawStatusGlyphIceArmor},
	core.PartyStatusDefending: {Col: statusDefending, Glyph: drawStatusGlyphDefending},
}

func init() {
	// Every non-None kind must carry a Glyph; caught at startup, not as a bare disc.
	assertTableComplete("partyStatusVisuals", int(core.PartyStatusCount), func(i int) bool {
		k := core.PartyStatusKind(i)
		return k != core.PartyStatusNone && partyStatusVisuals[k].Glyph == nil
	})
}

// partyStatusVisual returns the per-status color + flicker flag (out-of-range → None).
func partyStatusVisual(kind core.PartyStatusKind) (col rl.Color, flicker bool) {
	if kind < 0 || int(kind) >= len(partyStatusVisuals) {
		v := partyStatusVisuals[core.PartyStatusNone]
		return v.Col, v.Flicker
	}
	v := partyStatusVisuals[kind]
	return v.Col, v.Flicker
}

// partyStatusTurnLabelCache pre-formats every "<LABEL> N" so the draw isn't a Sprintf.
var partyStatusTurnLabelCache = func() [core.PartyStatusCount][statusTurnCacheMax]string {
	var out [core.PartyStatusCount][statusTurnCacheMax]string
	for k := core.PartyStatusKind(0); k < core.PartyStatusCount; k++ {
		base := core.PartyStatusLabel(k)
		for n := 0; n < statusTurnCacheMax; n++ {
			out[k][n] = fmt.Sprintf("%s %d", base, n)
		}
	}
	return out
}()

// statusTurnCacheMax: shared turn-count ceiling for all three status-turn caches
// (so they can't size-drift); past it they fall back to fmt.Sprintf.
const statusTurnCacheMax = 20

// partyStatusTurnLabel returns "<LABEL>" (turns==0) or "<LABEL> N" from the cache,
// falling back to fmt.Sprintf past the window.
func partyStatusTurnLabel(kind core.PartyStatusKind, turns int) string {
	base := core.PartyStatusLabel(kind)
	if turns <= 0 {
		return base
	}
	if int(kind) >= 0 && int(kind) < int(core.PartyStatusCount) && turns < statusTurnCacheMax {
		return partyStatusTurnLabelCache[kind][turns]
	}
	return fmt.Sprintf("%s %d", base, turns)
}

// partyStatusLabelMeasureCache memoizes MeasureTextEx for party-status labels.
var partyStatusLabelMeasureCache measureCache

func measurePartyStatusLabel(font rl.Font, label string) rl.Vector2 {
	return partyStatusLabelMeasureCache.measure(font, label, FontTiny, 1)
}

// statusTurnDigits caches the bare turns-remaining numerals (no per-frame alloc).
// Shared by party card + enemy pill.
var statusTurnDigits = func() [statusTurnCacheMax]string {
	var d [statusTurnCacheMax]string
	for i := range d {
		d[i] = fmt.Sprintf("%d", i)
	}
	return d
}()

// statusTurnDigit returns the cached bare numeral for n, falling back to
// fmt.Sprintf past the cached window.
func statusTurnDigit(n int) string {
	if n >= 0 && n < statusTurnCacheMax {
		return statusTurnDigits[n]
	}
	return fmt.Sprintf("%d", n)
}

// partyNamePlusLabels memoizes the "<Name> +" badge so the ribbon path is a map
// lookup, not a concat.
var partyNamePlusLabels = make(map[string]string, 8)

// partyNameSpaceWidth measures "<Name> " at FontBody (positions the "+" overlay).
var partyNameSpaceWidth measureCache

func partyNamePlusBadge(name string) string {
	if v, ok := partyNamePlusLabels[name]; ok {
		return v
	}
	v := name + " +"
	partyNamePlusLabels[name] = v
	return v
}

// partyHPBarKeys memoizes the "hp:<Name>" bar-ghost key so drawBarLive's
// per-frame ribbon path is a map lookup, not a concat.
var partyHPBarKeys = make(map[string]string, 8)

func partyHPBarKey(name string) string {
	if v, ok := partyHPBarKeys[name]; ok {
		return v
	}
	v := "hp:" + name
	partyHPBarKeys[name] = v
	return v
}

func measurePartyNameWithSpace(font rl.Font, name string) rl.Vector2 {
	return partyNameSpaceWidth.measure(font, name+" ", FontBody, 1)
}

const (
	// Wide short cards in a 2×2 grid: left = sigil + name + status, right = HP/MP bars.
	partyCardW    = float32(300)
	partyCardH    = float32(72)
	partyCardGap  = float32(14)
	partyRowGap   = float32(10) // vertical gap between the two card rows
	partyCardCols = 2
	partyCardBarH = float32(22) // shorter gauge so HP+MP stack in the right column
	// cardGlowMargin: per-side inset shared by active halo + selected outline.
	cardGlowMargin = int32(3)
	// activeCardJut nudges the active card OUTWARD by column so "whose turn" reads
	// without overlapping neighbours.
	activeCardJut = float32(28)
	// ribbonBottom is the ribbon's bottom margin (hudEdgePad, matching other panels).
	ribbonBottom = float32(hudEdgePad)
)

// drawCardScrim paints the dim wash over a card's rounded rect (matching
// drawCard's radius) to recede non-active cards during a member's turn.
func drawCardScrim(x, y, w, h int32) {
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	roundness := fixedRoundnessFor(w, h, cornerRadius)
	rl.DrawRectangleRounded(rect, roundness, 8, surfaceDimScrim)
}

// drawPartyCard renders one party member card. `dim` requests the inactive-member
// wash (applied last, over the whole card).
func drawPartyCard(font rl.Font, member *core.PartyMember, x, y float32, active, selected, down, dim bool) {
	classCol := classAccent(member.Class)
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

	// Jut the active card outward by column so it stands out without colliding
	// with the row above or the neighbouring card.
	if active && !down {
		if member.HomeCol == core.ColRight {
			x += activeCardJut
		} else {
			x -= activeCardJut
		}
	}

	ix, iy := int32(x), int32(y)
	iw, ih := int32(partyCardW), int32(partyCardH)

	if active && !down {
		// Layered gilt halo (solid inner + pulsing outer ring), shared with the
		// battle roster's targeted-row halo via drawSelectionHalo.
		drawSelectionHalo(ix-cardGlowMargin, iy-cardGlowMargin, iw+2*cardGlowMargin, ih+2*cardGlowMargin, borderActive, pulseActiveActor(), false)
	}
	if selected {
		drawPanelOutline(ix-cardGlowMargin, iy-cardGlowMargin, iw+2*cardGlowMargin, ih+2*cardGlowMargin, borderTarget)
	}

	drawCard(ix, iy, iw, ih, bg, border, accent)

	if selected {
		centerX := x + partyCardW/2
		drawArrowMarker(rl.NewVector2(centerX, y+2), 0, -12, 10, borderTarget)
	}

	// LEFT column: class sigil + name on top, status icon (+ turns) below.
	leftX := x + 14
	glyphR := float32(9)
	glyphCX := leftX + glyphR
	glyphCY := y + 18
	glyphCol := classCol
	if down {
		glyphCol = fadeColor(classCol, 0.45)
	}
	drawClassMedallion(glyphCX, glyphCY, glyphR+4, glyphCol, down)
	drawClassGlyph(glyphCX, glyphCY, glyphR, member.Class, glyphCol)
	nameX := glyphCX + glyphR + 10

	// Soft "+" badge on the name when the member has unspent stat/skill points.
	hasPoints := core.HasUnspentPoints(member)
	nameText := member.Name
	if hasPoints {
		nameText = partyNamePlusBadge(member.Name)
	}
	drawTextWithShadow(font, nameText, nameX, y+10, FontBody, nameCol)
	if hasPoints && !down {
		nameMeasure := measurePartyNameWithSpace(font, member.Name)
		drawTextWithShadow(font, "+", nameX+nameMeasure.X, y+10, FontBody, inkAccent)
	}

	// Status icon + turn count below the name. Resolves through core.PartyStatus
	// so the card agrees with the Tome badge; only color/flicker are render-side.
	kind, turns := core.PartyStatus(member)
	if kind != core.PartyStatusNone {
		col, flicker := partyStatusVisual(kind)
		iconCol := col
		if flicker {
			iconCol = fadeColor(col, pulseFlicker())
		}
		const statusIconR = float32(7)
		icx := nameX + statusIconR
		icy := y + partyCardH - 16
		drawPartyStatusIcon(icx, icy, statusIconR, kind, iconCol)
		if turns > 0 {
			num := statusTurnDigit(turns)
			drawTextWithShadow(font, num, icx+statusIconR+4, icy-FontTiny/2, FontTiny, iconCol)
		}
	}

	// RIGHT column: HP over MP. HP rides the live gauge (hit-ghost + low-HP
	// breathing); MP is static.
	barX := x + partyCardW*0.5
	barW := x + partyCardW - 14 - barX
	hpFill := hpFillColor(member.HP, member.MaxHP)
	drawBarLive(font, partyHPBarKey(member.Name), barX, y+10, barW, partyCardBarH, "HP", member.HP, member.MaxHP, hpFill, down)
	drawBar(font, barX, y+partyCardH-10-partyCardBarH, barW, partyCardBarH, "MP", member.MP, member.MaxMP, barMP, down)

	// Dim wash over inactive cards, painted last (active + targeted-ally opt out).
	if dim {
		drawCardScrim(ix, iy, iw, ih)
	}
}

// statusWebSpokes are the unit directions for the Webbed icon's radial strands
// (6 spokes, 60° apart) — precomputed so the per-frame draw needs no trig.
var statusWebSpokes = [6][2]float32{
	{1, 0}, {0.5, 0.866}, {-0.5, 0.866}, {-1, 0}, {-0.5, -0.866}, {0.5, -0.866},
}

// statusDizzyDots are the offsets (×r) for the Stunned icon's orbiting star-dots.
var statusDizzyDots = [3][2]float32{{-0.62, -0.5}, {0, -0.78}, {0.62, -0.5}}

// drawPartyStatusIcon draws the status glyph at (cx,cy) radius r over a dark
// backing disc. col is the per-status accent (already flickered by the caller).
// The symbol dispatches from partyStatusVisuals.Glyph (init-asserted) not a
// switch, so a new status can't be missed.
func drawPartyStatusIcon(cx, cy, r float32, kind core.PartyStatusKind, col rl.Color) {
	rl.DrawCircleV(rl.NewVector2(cx, cy+1), r+3, fadeColor(shadowHeavy, 0.30))
	rl.DrawCircleV(rl.NewVector2(cx, cy), r+2, statusIconBacking)
	if kind < 0 || int(kind) >= len(partyStatusVisuals) {
		return
	}
	if glyph := partyStatusVisuals[kind].Glyph; glyph != nil {
		glyph(cx, cy, r, col)
	}
}

// drawStatusGlyph* paint one status symbol each (the rows of partyStatusVisuals.Glyph).

func drawStatusGlyphPoisoned(cx, cy, r float32, col rl.Color) {
	// Toxic droplet: round body + pointed tip + a highlight bubble.
	rl.DrawCircleV(rl.NewVector2(cx, cy+r*0.28), r*0.55, col)
	rl.DrawPoly(rl.NewVector2(cx, cy-r*0.12), 3, r*0.5, -90, col)
	rl.DrawCircleV(rl.NewVector2(cx-r*0.16, cy+r*0.2), r*0.16, fadeColor(rl.White, 0.7))
}

func drawStatusGlyphAsleep(cx, cy, r float32, col rl.Color) {
	// A drawn "Z" (three strokes) — sleep mark, no font.
	t := float32(2)
	rl.DrawLineEx(rl.NewVector2(cx-r*0.5, cy-r*0.5), rl.NewVector2(cx+r*0.5, cy-r*0.5), t, col)
	rl.DrawLineEx(rl.NewVector2(cx+r*0.5, cy-r*0.5), rl.NewVector2(cx-r*0.5, cy+r*0.5), t, col)
	rl.DrawLineEx(rl.NewVector2(cx-r*0.5, cy+r*0.5), rl.NewVector2(cx+r*0.5, cy+r*0.5), t, col)
}

func drawStatusGlyphStunned(cx, cy, r float32, col rl.Color) {
	// Dizzy: orbiting star-dots above the head.
	for _, d := range statusDizzyDots {
		rl.DrawPoly(rl.NewVector2(cx+r*d[0], cy+r*d[1]), 4, r*0.26, 45, col)
	}
}

func drawStatusGlyphWebbed(cx, cy, r float32, col rl.Color) {
	// Spider web: two rings + radial strands.
	c := rl.NewVector2(cx, cy)
	rl.DrawCircleLines(int32(cx), int32(cy), r*0.8, col)
	rl.DrawCircleLines(int32(cx), int32(cy), r*0.42, col)
	for _, d := range statusWebSpokes {
		rl.DrawLineEx(c, rl.NewVector2(cx+r*0.8*d[0], cy+r*0.8*d[1]), 1, col)
	}
}

func drawStatusGlyphConfused(cx, cy, r float32, col rl.Color) {
	// Confusion swirl: a near-closed ring with a gap.
	rl.DrawRing(rl.NewVector2(cx, cy), r*0.42, r*0.64, 20, 320, 20, col)
}

func drawStatusGlyphIngested(cx, cy, r float32, col rl.Color) {
	// Maw mid-swallow: open-mouth disc facing a small prey dot.
	rl.DrawCircleSector(rl.NewVector2(cx, cy), r*0.82, 35, 325, 20, col)
	rl.DrawCircleV(rl.NewVector2(cx+r*0.95, cy), r*0.22, col)
	rl.DrawCircleV(rl.NewVector2(cx-r*0.18, cy-r*0.22), r*0.14, statusGlyphDark)
}

func drawStatusGlyphBlessed(cx, cy, r float32, col rl.Color) {
	// Blessing: two rising chevrons — the universal "buff / stats up" mark.
	t := float32(2)
	rl.DrawLineEx(rl.NewVector2(cx-r*0.55, cy+r*0.12), rl.NewVector2(cx, cy-r*0.38), t, col)
	rl.DrawLineEx(rl.NewVector2(cx, cy-r*0.38), rl.NewVector2(cx+r*0.55, cy+r*0.12), t, col)
	rl.DrawLineEx(rl.NewVector2(cx-r*0.55, cy+r*0.62), rl.NewVector2(cx, cy+r*0.12), t, col)
	rl.DrawLineEx(rl.NewVector2(cx, cy+r*0.12), rl.NewVector2(cx+r*0.55, cy+r*0.62), t, col)
}

func drawStatusGlyphRegen(cx, cy, r float32, col rl.Color) {
	// Healing cross (a "+") — the universal restore mark, in mint green.
	rl.DrawRectangleRounded(rl.NewRectangle(cx-r*0.2, cy-r*0.62, r*0.4, r*1.24), 0.4, 4, col)
	rl.DrawRectangleRounded(rl.NewRectangle(cx-r*0.62, cy-r*0.2, r*1.24, r*0.4), 0.4, 4, col)
}

func drawStatusGlyphDefending(cx, cy, r float32, col rl.Color) {
	// Shield: rounded badge body + a heraldic center spine.
	rl.DrawRectangleRounded(rl.NewRectangle(cx-r*0.62, cy-r*0.7, r*1.24, r*1.5), 0.45, 6, col)
	rl.DrawLineEx(rl.NewVector2(cx, cy-r*0.55), rl.NewVector2(cx, cy+r*0.55), 1.5, fadeColor(statusGlyphDark, 0.6))
}

func drawStatusGlyphShielded(cx, cy, r float32, col rl.Color) {
	// Aegis ward: glowing energy bubble (ring + soft fill), distinct from the
	// Defending solid-shield badge so magical ward reads apart from manual block.
	rl.DrawCircleV(rl.NewVector2(cx, cy), r*0.7, fadeColor(col, 0.28))
	rl.DrawCircleLines(int32(cx), int32(cy), r*0.7, col)
	rl.DrawCircleLines(int32(cx), int32(cy), r*0.46, fadeColor(col, 0.7))
}

func drawStatusGlyphIceArmor(cx, cy, r float32, col rl.Color) {
	// Frost ward: a six-spoke snowflake/crystal — the universal ice mark.
	radialSpokes(6, 0, func(_ int, dx, dy float32) {
		rl.DrawLineEx(rl.NewVector2(cx, cy), rl.NewVector2(cx+dx*r*0.68, cy+dy*r*0.68), 1.5, col)
	})
}

func drawStatusGlyphDown(cx, cy, r float32, col rl.Color) {
	// Skull: dome + jaw + eye sockets.
	rl.DrawCircleV(rl.NewVector2(cx, cy-r*0.12), r*0.78, col)
	rl.DrawRectangleRounded(rl.NewRectangle(cx-r*0.42, cy+r*0.32, r*0.84, r*0.42), 0.4, 4, col)
	rl.DrawCircleV(rl.NewVector2(cx-r*0.3, cy-r*0.16), r*0.2, statusGlyphDark)
	rl.DrawCircleV(rl.NewVector2(cx+r*0.3, cy-r*0.16), r*0.2, statusGlyphDark)
	rl.DrawLineEx(rl.NewVector2(cx, cy+r*0.34), rl.NewVector2(cx, cy+r*0.72), 1, statusGlyphDark)
}

func drawStatusGlyphBurn(cx, cy, r float32, col rl.Color) {
	// Flame: rounded base, upward tip, bright inner core — reads as fire, not the
	// poison drop (which points up but carries a highlight bubble instead).
	rl.DrawCircleV(rl.NewVector2(cx, cy+r*0.3), r*0.52, col)
	rl.DrawPoly(rl.NewVector2(cx, cy-r*0.18), 3, r*0.6, -90, col)
	rl.DrawCircleV(rl.NewVector2(cx, cy+r*0.34), r*0.2, fadeColor(rl.White, 0.45))
}

func drawStatusGlyphBleed(cx, cy, r float32, col rl.Color) {
	// Blood drop: tip at the BOTTOM (falling) — inverse of the poison droplet's
	// tip-up, so the two DoT marks read apart before pill colors differ.
	rl.DrawCircleV(rl.NewVector2(cx, cy-r*0.05), r*0.52, col)
	rl.DrawPoly(rl.NewVector2(cx, cy+r*0.4), 3, r*0.5, 90, col)
}

func drawClassMedallion(cx, cy, r float32, col rl.Color, muted bool) {
	rim := fadeColor(giltDim, 0.78)
	inner := fadeColor(glassWarm, 0.85)
	if muted {
		rim = fadeColor(rim, 0.45)
		inner = fadeColor(inner, 0.45)
	}
	// Shared medallion stack; the pip callback adds the class-badge glint + a
	// class-tinted corner diamond.
	drawMedallion(cx, cy, r+1.5, r, r-2.5,
		fadeColor(woodDark, 0.88), rim, inner, r+1, func() {
			rl.DrawCircleV(rl.NewVector2(cx-r*0.25, cy-r*0.25), 1.6, fadeColor(inkPrimary, 0.32))
			drawDiamondPip(cx+r*0.72, cy-r*0.55, 1.2, fadeColor(col, 0.70))
		})
}

// DrawPartyRibbon renders the always-visible bottom party ribbon. Cards are
// pinned at fixed positions so they stay readable through attack bumps/dances.
func DrawPartyRibbon(g *core.GameState, assets Resources) {
	if len(g.Party) == 0 {
		return
	}
	_, screenH := screenSizeF()
	totalW := partyCardW*partyCardCols + partyCardGap*(partyCardCols-1)
	startX := centerXF(totalW)
	if startX < float32(hudEdgePad) {
		startX = float32(hudEdgePad)
	}
	topY := screenH - partyRibbonHeight(len(g.Party)) - ribbonBottom

	activeIdx := core.ActiveActorIndex(g)
	selectedIdx := -1
	if targetingAlly(g) {
		selectedIdx = core.HighlightedAllyIndex(g)
	}

	// Dim other cards only when a member is actually up (ActiveActorIndex is -1 on
	// enemy/between turns, so the ribbon stays full-bright then).
	dimOthers := core.PartyIndexInRange(g.Party, activeIdx) && core.PartyMemberAvailable(g.Party, activeIdx)

	for i := range g.Party {
		member := &g.Party[i]
		// Tile by FORMATION slot, not array index, so the ribbon mirrors the 2×2 the
		// party fights in and a Swap visibly moves a card. In battle, use the LIVE slot
		// so an ambush rotation or a death-driven shunt shuffles the cards in lockstep
		// with the 3D sprites; out of battle, the stable Home slot (the live slot is
		// stale until the next battle seats it). Both are a clean 2×2 by invariant.
		col, row := int(member.HomeCol), int(member.HomeRow)
		if g.Battle.Active() {
			col, row = int(member.Col), int(member.Row)
		}
		x := startX + (partyCardW+partyCardGap)*float32(col)
		y := topY + (partyCardH+partyRowGap)*float32(row)
		// Active/selected glow only on a member who can act AND be targeted (not
		// ingested) — defensive against a stale PartyTarget on a swallowed member.
		available := core.PartyMemberAvailable(g.Party, i)
		active := i == activeIdx && available
		selected := i == selectedIdx && available
		// Dim every non-active card except the targeted one (its green highlight
		// must stay legible during heal/item targeting).
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

// partyRibbonHeight is the total height of the tiled ribbon for memberCount, in
// 2-wide rows. Single source so DrawPartyRibbon and PartyRibbonTopY can't drift.
func partyRibbonHeight(memberCount int) float32 {
	rows := (memberCount + partyCardCols - 1) / partyCardCols
	if rows < 1 {
		rows = 1
	}
	return partyCardH*float32(rows) + partyRowGap*float32(rows-1)
}

// PartyRibbonTopY reports the screen Y of the ribbon's top so other panels can
// stack above it.
func PartyRibbonTopY() float32 {
	_, h := screenSizeF()
	return h - partyRibbonHeight(core.PartyMemberCount) - ribbonBottom
}

// centeredMeasureCache memoizes the MeasureTextEx backing drawTextCentered, whose
// callers re-draw stable strings every frame.
var centeredMeasureCache measureCache

func drawTextCentered(font rl.Font, text string, centerX, y, size float32, col color.RGBA) {
	// canonicalSpacing matches drawTextWithShadow so heading text centers on its
	// true tracked width.
	measure := centeredMeasureCache.measure(font, text, size, canonicalSpacing(size))
	drawTextWithShadow(font, text, centerX-measure.X/2, y, size, col)
}

// rightAlignMeasureCache memoizes the MeasureTextEx backing drawTextRightAligned,
// whose right-aligned readouts are stable strings re-drawn every frame.
var rightAlignMeasureCache measureCache

// drawTextRightAligned draws text so its RIGHT edge sits at rightX. The mirror of
// drawTextCentered, consolidating the open-coded "edge - measure.X - pad" sites.
func drawTextRightAligned(font rl.Font, text string, rightX, y, size float32, col color.RGBA) {
	// canonicalSpacing as in drawTextCentered, or the measured width misses the
	// heading tracking and the right edge drifts.
	measure := rightAlignMeasureCache.measure(font, text, size, canonicalSpacing(size))
	drawTextWithShadow(font, text, rightX-measure.X, y, size, col)
}
