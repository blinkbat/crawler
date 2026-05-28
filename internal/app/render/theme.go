package render

import (
	"image/color"
	"math"
	"strconv"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// sqrt2Inv is 1/√2 — the unit-diagonal component shared by every glyph
// drawn on a 45° axis (DEX arrow, warrior cross-swords, skill icons, the
// compass rose). One package-level const instead of a per-function local
// in four draw helpers.
const sqrt2Inv = float32(0.7071)

// The Library palette. See UI_STANDARDS.md for the full rationale and
// the rule "no new rl.NewColor literals for any surface that already
// has a token." Every persistent HUD pane is dark glass framed in
// hardwood; every selection highlight is gilt.
//
// Two parallel naming families exist for historical reasons:
//   - The UI_STANDARDS.md tokens (`glassDeep` / `woodMid` /
//     `inkPrimary` / etc.) are the canonical names — use these in
//     new code.
//   - Older semantic aliases (`surfacePrimary` / `borderSoft` /
//     `textPrimary` / etc., grouped below) resolve to the same
//     RGB values but predate the library aesthetic doc. They're
//     treated as first-class — neither set is "deprecated" —
//     because the codebase reads cleanly with either, and a full
//     rename would touch 200+ call sites without changing pixels.
//     New uses are fine either way; pick the name that reads best
//     in context (e.g. `textPrimary` for body copy, `inkPrimary`
//     when the parchment metaphor is load-bearing).
var (
	// ----- Glass surfaces (panel fills) -----
	// Two-layer composition: drawCard paints `glassBaseWash` (a
	// dark wash that anchors the pane as dark glass) first, then
	// the glass tint over it. Both layers are translucent — the
	// world composes through both. Cumulative effective opacity is
	// roughly 1 - (1-wash)(1-tint), which lands the pane near
	// 60-70 % apparent opacity at the alphas below: dark enough to
	// read as glass, translucent enough that the world hints
	// through.
	glassBaseWash = rl.NewColor(8, 6, 10, 115)
	glassDeep     = rl.NewColor(14, 12, 18, 130)
	glassMid      = rl.NewColor(22, 18, 24, 105)
	glassWarm     = rl.NewColor(28, 22, 16, 145)
	glassDanger   = rl.NewColor(36, 16, 18, 135)
	veil          = rl.NewColor(0, 0, 0, 130)

	// ----- Wood frames -----
	woodDark   = rl.NewColor(48, 30, 18, 255)
	woodMid    = rl.NewColor(96, 62, 36, 255)
	woodLight  = rl.NewColor(150, 104, 64, 255)
	woodAccent = rl.NewColor(184, 140, 92, 255)

	// ----- Gilt accents (selection / focus) -----
	giltDim    = rl.NewColor(160, 124, 64, 255)
	giltBright = rl.NewColor(232, 196, 112, 255)
	giltGlow   = rl.NewColor(255, 232, 168, 210)

	// ----- Parchment ink (text) -----
	inkPrimary = rl.NewColor(232, 222, 196, 255)
	inkMuted   = rl.NewColor(184, 172, 144, 240)
	inkDim     = rl.NewColor(132, 122, 100, 220)
	inkAccent  = rl.NewColor(232, 196, 112, 255)

	// ----- Semantic aliases of the library palette (use freely) -----
	// These names predate the wood/glass/gilt nomenclature but resolve
	// to the same RGB values — pick whichever reads best in context.
	surfacePrimary    = glassDeep
	surfaceLog        = glassMid
	surfaceVeil       = veil
	surfaceActiveTint = glassWarm
	surfaceTargetTint = rl.NewColor(20, 38, 32, 140) // faint emerald glass for friendly target
	surfaceDownTint   = rl.NewColor(28, 22, 28, 115) // knocked down — dim grey wash
	surfaceEnemyTint  = glassDanger

	// Enemy-roster row tints (drawEnemyRosterRow): the live row's glass
	// fill + warm border, and the dimmed pair used while a defeated enemy
	// fades out. Named here so the row chrome shares the palette's source
	// of truth instead of carrying its own NewColor literals.
	surfaceRosterRow      = rl.NewColor(20, 14, 22, 130)
	borderRosterRow       = rl.NewColor(96, 60, 64, 140)
	surfaceRosterRowFaded = rl.NewColor(28, 20, 24, 95)

	// Border aliases — used by drawCard as the OUTERMOST frame
	// stroke. Default panels use woodDark (deepest band); active /
	// danger panels swap in a saturated tint so the focused surface
	// reads first.
	borderDim    = rl.NewColor(60, 48, 36, 130) // dim hardwood, for disabled panels
	borderSoft   = woodDark                     // default outer stroke
	borderStrong = woodAccent                   // heading underline accent
	borderActive = giltBright                   // active actor panel frame
	borderTarget = rl.NewColor(118, 200, 132, 235)
	borderEnemy  = rl.NewColor(212, 120, 80, 235)
	borderDanger = rl.NewColor(220, 88, 88, 235)

	textPrimary = inkPrimary
	textMuted   = inkMuted
	textLabel   = inkAccent
	textDim     = inkDim
	textHint    = inkDim

	barHPHigh  = rl.NewColor(116, 200, 132, 255)
	barHPMid   = rl.NewColor(224, 184, 88, 255)
	barHPLow   = rl.NewColor(220, 88, 88, 255)
	barMP      = rl.NewColor(104, 152, 224, 255)
	barEnemyHP = rl.NewColor(204, 76, 76, 255)
	barTrack   = rl.NewColor(10, 8, 14, 140)

	// ----- Per-status accents (UI_STANDARDS.md "Per-status accents") -----
	// Indexed by core.PartyStatusKind via partyStatusVisuals below; the
	// raw tokens are exported here so non-party surfaces (enemy pills,
	// future field-status overlays) can pull the same hue without
	// re-typing the RGBs.
	statusPoison    = rl.NewColor(148, 200, 96, 240)
	statusBurn      = rl.NewColor(240, 144, 72, 240)
	statusSleep     = rl.NewColor(132, 196, 232, 240)
	statusStun      = rl.NewColor(232, 220, 120, 240)
	statusBound     = rl.NewColor(180, 140, 220, 240)
	statusConfused  = rl.NewColor(220, 188, 96, 240)
	statusIngested  = rl.NewColor(200, 132, 220, 240)
	statusDefending = rl.NewColor(132, 196, 255, 240)
	statusDown      = rl.NewColor(220, 102, 102, 235)
	// Outline tints paired with the fills above for the enemy-pill
	// silhouette. Lighter / more saturated than the fill so the pill
	// reads as a "glow with a hard rim" against the panel.
	statusBurnOutline   = rl.NewColor(255, 200, 120, 220)
	statusSleepOutline  = barSleepOutline
	statusPoisonOutline = rl.NewColor(180, 232, 132, 220)
	statusStunOutline   = rl.NewColor(248, 232, 160, 230)
	// barSleep is the indigo-blue used for the sleep-status indicator
	// (Z-counter beside enemy HP bars). Shares the barMP RGB but with
	// reduced alpha so the panel reads as "soft glow" rather than the
	// solid MP-bar tone.
	barSleep        = rl.NewColor(96, 162, 232, 200)
	barSleepOutline = rl.NewColor(190, 220, 244, 220)

	// Turn-order panel: the danger red an enemy row reads as. Named so
	// it isn't a bare literal buried in turnEntryColor's getter.
	turnEnemyColor = rl.NewColor(245, 100, 92, 255)

	// Timing-bar accent tokens. The bright cursor white, the "held /
	// RELEASE" gold, and the sequence-bar pass/fail greens & reds used
	// to recur as bare NewColor literals at drifting alphas across the
	// press / charge / sequence draws. Defined once at full alpha; the
	// few sites that want a softer alpha wrap these in colorWithAlpha.
	timingCursorColor = rl.NewColor(248, 248, 252, 255)
	timingHeldColor   = rl.NewColor(255, 244, 144, 255)
	seqOkColor        = rl.NewColor(140, 232, 168, 255)
	seqFailColor      = rl.NewColor(228, 96, 96, 255)

	// Billboard tints for the in-world combatant markers — the warm
	// off-white the player's target reads as, and the slightly redder
	// pulse the currently-attacking enemy reads as. Pulled out here so
	// future palette passes don't have to chase NewColor literals across
	// world.go's draw loop.
	tintEnemyTargeted = rl.NewColor(255, 228, 190, 255)
	tintEnemyAttacker = rl.NewColor(255, 196, 156, 255)

	// Editor + minimap entity-marker colors. Centralised here so the
	// in-grid markers (editor's draw.go), the on-disk-list swatches
	// (editor's brush palette), and the minimap dots all share one
	// source of truth — moving the door tone or chest tone is now one
	// edit instead of three NewColor literals. Exported via theme.go
	// as theme.MarkerXxx fields for external scenes.
	markerStart    = rl.NewColor(255, 220, 124, 255)
	markerChest    = rl.NewColor(232, 180, 92, 255)
	markerChestDim = rl.NewColor(160, 132, 78, 255)
	markerDoor     = rl.NewColor(176, 132, 86, 255)
	markerPack     = rl.NewColor(220, 76, 70, 255)
	markerPackEdge = rl.NewColor(255, 200, 200, 220)
	markerOutline  = rl.NewColor(0, 0, 0, 220)

	// chestColors govern the chest billboard — body color, lid color,
	// and the deeper tone for an emptied/looted chest. Pulled out here
	// rather than open-coded in DrawChests so the palette can be tuned
	// without hunting through world-render code. The interact prompt
	// reuses borderActive (the global "draw the player's eye here"
	// yellow) so adding a new prompt color isn't needed.
	chestBodyColor  = rl.NewColor(168, 116, 70, 255)
	chestBodyLooted = rl.NewColor(98, 76, 56, 255)
	chestLidColor   = rl.NewColor(196, 148, 92, 255)
	chestMetalColor = rl.NewColor(220, 192, 102, 255)

	// Shadow tints for drop-shadowed text and overlay scrims. Pre-named so
	// callers don't open-code rl.NewColor(0,0,0,…) with a drifting alpha.
	// Strength runs Light (background hints) → Mid (HUD body) → Strong
	// (large titles / debug pills) → Heavy (top-of-stack labels).
	shadowLight  = rl.NewColor(0, 0, 0, 160)
	shadowMid    = rl.NewColor(0, 0, 0, 180)
	shadowStrong = rl.NewColor(0, 0, 0, 200)
	shadowHeavy  = rl.NewColor(0, 0, 0, 220)
)

const (
	// hudEdgePad is the canonical distance every always-on HUD panel
	// keeps from the screen edges. Pulled into theme so the minimap,
	// turn panel, combat log, action menu, and party ribbon all
	// honour the same margin without each one picking its own.
	// 16 reads "comfortable margin" at 1080p without wasting too much
	// real estate on smaller windows.
	hudEdgePad = int32(16)
	// hudColumnGap is the vertical spacing between stacked HUD
	// panels in the left/right column (minimap → turn panel →
	// combat log). Smaller than hudEdgePad so adjacent panels feel
	// grouped rather than scattered.
	hudColumnGap = int32(10)

	// Corner radii. Smaller than the previous pass (10/6 → 4/3) so the
	// frame reads as a hardwood mitre joint rather than a modern UI
	// rounded tile. See UI_STANDARDS.md "Panel" section.
	cornerRadius      = float32(4)
	smallCornerRadius = float32(3)
	stripeWidth       = int32(3)

	// Font sizes — the FIVE permitted text sizes across the whole HUD,
	// editor, and modal surface. See UI_STANDARDS.md "Type" section.
	// Anything else is a bug. Picked as even divisions of the 64 pt
	// atlas bake so every size renders sharp without subpixel sludge.
	FontTiny    = float32(13)
	FontSmall   = float32(16)
	FontBody    = float32(20)
	FontHeading = float32(26)
	FontTitle   = float32(36)

	// Letter spacing per size. Wider tracking on titles to sell the
	// "engraved on hardwood" feel. Use these via the
	// drawTextWithShadowStyle path; drawTextWithShadow defaults to 1.
	FontSpacingTiny    = float32(1)
	FontSpacingSmall   = float32(1)
	FontSpacingBody    = float32(1)
	FontSpacingHeading = float32(2)
	FontSpacingTitle   = float32(3)

	// woodFrameOuter / woodFrameInner are the stroke widths of the
	// outer dark band and the inner highlight pinstripe inside the
	// wood-panel border. Tuned so the frame reads at 1080p without
	// dominating; modest panels still feel substantial.
	woodFrameOuter = int32(2)
	woodFrameBand  = int32(3)
	woodFrameInner = int32(1)

	// Heading tick markers (drawHeading underline) have a minimum width so
	// short headings still read as labelled. Bar value text inset is the
	// constant pad on the right edge of drawBar.
	headingTickMinWidth = int32(28)
	barValuePadRight    = float32(10)
	barLabelPadLeft     = float32(8)

	// World-popup horizontal slack: how many pixels past the screen edges a
	// 3D-to-2D projected popup can drift before we cull it. Larger than zero
	// so a popup whose anchor moves slightly off-screen still fades cleanly
	// instead of snapping to invisible mid-animation.
	offscreenPopupSlack = float32(200)

	// Overlay (modal card) dimensions. Each modal surface used to
	// hard-code its own cardW / cardH literals; centralizing them keeps
	// a future "shrink all modals on small screens" pass in one edit.
	// The dimensions are sized to the modal's CONTENT — a chest with
	// fewer items renders shorter via cardH expansion at the call site,
	// but the WIDTH stays standardized.
	overlayCardWidthSmall  = int32(360) // chest modal (item list)
	overlayCardWidthMedium = int32(420) // level-up modal
	overlayCardWidthLarge  = int32(680) // party stats overlay
	overlayCardWidthHuge   = int32(820) // game panels overlay

	overlayCardHeightSmall  = int32(380) // level-up / party stats
	overlayCardHeightLarge  = int32(520) // game panels overlay
	overlayCardMarginScreen = int32(40)  // minimum margin between card and screen edges

	// Panels-overlay tab strip geometry. Shared by the panels surface
	// only today — moved here so future tab-strip surfaces (a future
	// equipment swap modal, a settings panel) can reuse the heights.
	overlayTabHeight  = int32(34)
	overlayTabPadding = int32(12)

	// overlayFooterReserve is the vertical band at the bottom of every
	// overlay card reserved for the "Esc close / L1 R1 tabs" hint
	// footer rendered by DrawFooterHint. Body rect = card minus this
	// band minus the heading band at the top.
	overlayFooterReserve = int32(28)
	overlayHeaderReserve = int32(40)
)

// drawModalScaffold paints the shared screen-veil + centered card +
// heading band for every modal overlay (chest, level-up, party stats,
// panels). Returns the card rect so the caller can lay out its body
// inside without redoing the centering math. Pass an empty heading
// to skip the header band — the caller still gets the right card
// rect.
//
// The three older overlays (chest / levelup / party stats) used to
// open-code rl.DrawRectangle(0,0,…,surfaceVeil) + drawCard + drawHeading
// each in slightly different orders. This helper plus the
// overlayCardWidth* / overlayCardHeight* constants in this file are
// the seam where future "shrink for small screens" or "fade-in"
// behaviour lands once.
func drawModalScaffold(font rl.Font, cardW, cardH int32, heading string) rl.Rectangle {
	screenW, screenH := screenSize()
	// Soft clamp so a tiny window doesn't push the card off-screen.
	if cardW > screenW-overlayCardMarginScreen {
		cardW = screenW - overlayCardMarginScreen
	}
	if cardH > screenH-overlayCardMarginScreen {
		cardH = screenH - overlayCardMarginScreen
	}
	cardX := centerX(cardW)
	cardY := screenH/2 - cardH/2

	rl.DrawRectangle(0, 0, screenW, screenH, surfaceVeil)
	drawCard(cardX, cardY, cardW, cardH, surfacePrimary, borderSoft, borderActive)
	drawCardFiligree(cardX, cardY, cardW, cardH, giltDim)
	if heading != "" {
		drawHeading(font, heading, cardX+28, cardY+14, borderActive)
	}
	return rl.NewRectangle(float32(cardX), float32(cardY), float32(cardW), float32(cardH))
}

// drawCardFiligree paints multi-stroke gilt corner brackets on a
// wood-framed card — the illuminated-manuscript bezel 90s AD&D PC
// RPGs used to dress dialog boxes. Each corner gets:
//
//   - outer L bracket (14 px arms, 2 px thick)
//   - inner secondary L bracket (8 px arms, 1 px thick, ~4 px inset)
//     in a softer gilt tone so it reads as a parallel cast line
//   - joint diamond pip (3 px) at the meeting point of the outer arms
//   - tip diamond pips (2 px) at the far ends of the outer arms
//   - centre highlight pip inside the joint diamond (1 px in bright
//     gilt) — the cast-metal speculum
//
// Skipped on panes too small to receive the ornament cleanly
// (< 80×80) so the corner minimap / turn panel stay simple.
func drawCardFiligree(x, y, w, h int32, col color.RGBA) {
	if w < 80 || h < 80 {
		return
	}
	inset := int32(woodFrameOuter + woodFrameBand + woodFrameInner + 5)
	outerArm := int32(14)
	innerArm := int32(8)
	innerInset := int32(4)
	corners := [4][2]int32{
		{x + inset, y + inset},
		{x + w - inset, y + inset},
		{x + inset, y + h - inset},
		{x + w - inset, y + h - inset},
	}
	softGilt := fadeColor(col, 0.55)
	for i, c := range corners {
		dx := int32(1)
		dy := int32(1)
		if i == 1 || i == 3 {
			dx = -1
		}
		if i == 2 || i == 3 {
			dy = -1
		}
		// Outer L — 14 px arms, 2 px thick.
		outerHX := c[0]
		if dx < 0 {
			outerHX = c[0] - outerArm
		}
		rl.DrawRectangle(outerHX, c[1], outerArm, 2, col)
		outerVY := c[1]
		if dy < 0 {
			outerVY = c[1] - outerArm
		}
		rl.DrawRectangle(c[0], outerVY, 2, outerArm, col)
		// Inner L — shorter and thinner, offset diagonally inward
		// from the outer bracket. Reads as the second cast line
		// of a Gothic frame.
		innerOriginX := c[0] + dx*innerInset
		innerOriginY := c[1] + dy*innerInset
		innerHX := innerOriginX
		if dx < 0 {
			innerHX = innerOriginX - innerArm
		}
		rl.DrawRectangle(innerHX, innerOriginY, innerArm, 1, softGilt)
		innerVY := innerOriginY
		if dy < 0 {
			innerVY = innerOriginY - innerArm
		}
		rl.DrawRectangle(innerOriginX, innerVY, 1, innerArm, softGilt)
		// Joint pip + bright speculum inside it.
		jointX := float32(c[0]) + float32(dx)*1
		jointY := float32(c[1]) + float32(dy)*1
		drawDiamondPip(jointX, jointY, 3, col)
		drawDiamondPip(jointX, jointY, 1, giltBright)
		// Tip pips at the far ends of the outer arms.
		drawDiamondPip(float32(c[0]+dx*outerArm), float32(c[1]), 1.5, col)
		drawDiamondPip(float32(c[0]), float32(c[1]+dy*outerArm), 1.5, col)
	}
}

// drawStatIcon dispatches to the per-stat sigil drawer. One small
// glyph per Stat enum value, used on the level-up modal's stat rows
// and the panels overlay's Stats tab so each row reads at a glance
// without needing to lean on the 3-letter label.
func drawStatIcon(s core.Stat, cx, cy, r float32, col color.RGBA) {
	switch s {
	case core.StatSTR:
		drawStatIconSTR(cx, cy, r, col)
	case core.StatDEX:
		drawStatIconDEX(cx, cy, r, col)
	case core.StatINT:
		drawStatIconINT(cx, cy, r, col)
	case core.StatWIS:
		drawStatIconWIS(cx, cy, r, col)
	case core.StatVIT:
		drawStatIconVIT(cx, cy, r, col)
	case core.StatSPD:
		drawStatIconSPD(cx, cy, r, col)
	}
}

// STR — short-shafted hammer: rectangular head on top, narrow
// handle hanging below, with a thin gilt band where head meets
// handle. The "strength of arms" sigil.
func drawStatIconSTR(cx, cy, r float32, col color.RGBA) {
	headHalfW := r * 0.85
	headH := r * 0.55
	rl.DrawRectangle(int32(cx-headHalfW), int32(cy-r), int32(headHalfW*2), int32(headH), col)
	// Head face band — a brighter inner stripe along the bottom of
	// the head reads as a steel rim against the haft.
	rl.DrawRectangle(int32(cx-headHalfW), int32(cy-r+headH-2), int32(headHalfW*2), 1, giltBright)
	// Haft running down from the head.
	haftHalfW := r * 0.16
	rl.DrawRectangle(int32(cx-haftHalfW), int32(cy-r+headH), int32(haftHalfW*2), int32(r*1.35), col)
	// Pommel knob at the haft's foot.
	rl.DrawCircleV(rl.NewVector2(cx, cy+r*0.45), haftHalfW*1.4, fadeColor(col, 0.85))
}

// DEX — arrow pointing up-right: tip triangle, shaft rectangle,
// fletching V at the tail. The classic "agility / precision" sigil.
func drawStatIconDEX(cx, cy, r float32, col color.RGBA) {
	// Arrow axis points NE. Direction unit vector + perpendicular.
	ax, ay := sqrt2Inv, -sqrt2Inv
	px, py := sqrt2Inv, sqrt2Inv
	// Tip triangle.
	tip := rl.NewVector2(cx+ax*r, cy+ay*r)
	tipBaseW := r * 0.45
	tipBack := rl.NewVector2(cx+ax*r*0.55, cy+ay*r*0.55)
	tipL := rl.NewVector2(tipBack.X+px*tipBaseW, tipBack.Y+py*tipBaseW)
	tipR := rl.NewVector2(tipBack.X-px*tipBaseW, tipBack.Y-py*tipBaseW)
	drawTriangleCCW(tip, tipR, tipL, col)
	// Shaft.
	shaftHalfW := r * 0.12
	for t := float32(-r * 0.85); t < r*0.55; t += 1 {
		px2 := cx + ax*t
		py2 := cy + ay*t
		// Tiny disc per sample — produces a rotated rectangle look
		// without depending on a rotated-rect primitive. Cheap
		// enough at r ~ 10 (samples ~25).
		rl.DrawCircleV(rl.NewVector2(px2, py2), shaftHalfW, col)
	}
	// Fletching V at the tail — two short diagonal strokes.
	tail := rl.NewVector2(cx-ax*r, cy-ay*r)
	fl1 := rl.NewVector2(tail.X+px*r*0.45, tail.Y+py*r*0.45)
	fl2 := rl.NewVector2(tail.X-px*r*0.45, tail.Y-py*r*0.45)
	drawTriangleCCW(tail, fl1, rl.NewVector2(tail.X+ax*r*0.35, tail.Y+ay*r*0.35), col)
	drawTriangleCCW(tail, rl.NewVector2(tail.X+ax*r*0.35, tail.Y+ay*r*0.35), fl2, col)
}

// INT — open book: two rectangular leaves meeting at a centre
// spine, with a small bookmark hanging off the side. The "lore /
// arcane study" sigil.
func drawStatIconINT(cx, cy, r float32, col color.RGBA) {
	pageHalfW := r * 0.7
	pageH := r * 1.3
	// Spine in centre.
	rl.DrawRectangle(int32(cx)-1, int32(cy-pageH/2), 2, int32(pageH), fadeColor(col, 0.7))
	// Left page (slight tilt via two triangles for the top-edge
	// curl, but flat-rect body for legibility).
	rl.DrawRectangle(int32(cx-pageHalfW), int32(cy-pageH/2), int32(pageHalfW)-1, int32(pageH), col)
	// Right page.
	rl.DrawRectangle(int32(cx)+1, int32(cy-pageH/2), int32(pageHalfW)-1, int32(pageH), col)
	// Page-line hatching — two thin horizontal pencil marks on
	// each page so the book reads as written-in, not blank.
	hatch := fadeColor(col, 0.4)
	rl.DrawRectangle(int32(cx-pageHalfW+2), int32(cy-pageH/4), int32(pageHalfW-4), 1, hatch)
	rl.DrawRectangle(int32(cx-pageHalfW+2), int32(cy+1), int32(pageHalfW-4), 1, hatch)
	rl.DrawRectangle(int32(cx+3), int32(cy-pageH/4), int32(pageHalfW-4), 1, hatch)
	rl.DrawRectangle(int32(cx+3), int32(cy+1), int32(pageHalfW-4), 1, hatch)
	// Bookmark — a thin gilt ribbon hanging off the right page.
	bmHalfW := float32(1)
	rl.DrawRectangle(int32(cx+pageHalfW*0.55), int32(cy-pageH/2), int32(bmHalfW*2), int32(pageH+3), giltBright)
}

// WIS — eye with iris: lens-shaped outline + centre pupil + bright
// catchlight. The "perception / divine sight" sigil.
func drawStatIconWIS(cx, cy, r float32, col color.RGBA) {
	// Lens: two triangles meeting along the horizontal axis make
	// a diamond, then we round it visually with a smaller circle
	// sitting inside.
	lensHalfW := r * 0.95
	lensHalfH := r * 0.55
	// Top half — triangle from left point through top arc to right
	// point. Approximate the arc with a single triangle (eye reads
	// at small size).
	left := rl.NewVector2(cx-lensHalfW, cy)
	right := rl.NewVector2(cx+lensHalfW, cy)
	top := rl.NewVector2(cx, cy-lensHalfH)
	bot := rl.NewVector2(cx, cy+lensHalfH)
	drawTriangleCCW(left, top, right, col)
	drawTriangleCCW(left, right, bot, col)
	// Iris — filled inner disc.
	rl.DrawCircleV(rl.NewVector2(cx, cy), lensHalfH*0.7, fadeColor(col, 0.55))
	// Pupil — dark centre dot.
	rl.DrawCircleV(rl.NewVector2(cx, cy), lensHalfH*0.3, fadeColor(col, 0.25))
	// Catchlight — bright pip slightly off-centre, like the gleam
	// in a painted portrait.
	rl.DrawCircleV(rl.NewVector2(cx-lensHalfH*0.18, cy-lensHalfH*0.18), 1.4, giltBright)
}

// VIT — heart shape: two lobes (filled discs) up top, a V-point at
// the bottom (triangle), with a bright inner pip. The "vitality"
// sigil.
func drawStatIconVIT(cx, cy, r float32, col color.RGBA) {
	lobeR := r * 0.42
	lobeY := cy - r*0.2
	lobeOffset := lobeR * 0.85
	rl.DrawCircleV(rl.NewVector2(cx-lobeOffset, lobeY), lobeR, col)
	rl.DrawCircleV(rl.NewVector2(cx+lobeOffset, lobeY), lobeR, col)
	// V-point — triangle from each lobe's outer edge down to the
	// chin.
	leftAnchor := rl.NewVector2(cx-lobeOffset-lobeR*0.85, lobeY+lobeR*0.25)
	rightAnchor := rl.NewVector2(cx+lobeOffset+lobeR*0.85, lobeY+lobeR*0.25)
	chin := rl.NewVector2(cx, cy+r*0.95)
	drawTriangleCCW(leftAnchor, chin, rightAnchor, col)
	// Inner highlight — bright pip slightly above centre for the
	// "alive and beating" feel.
	rl.DrawCircleV(rl.NewVector2(cx-lobeOffset*0.4, lobeY-lobeR*0.2), 1.4, giltBright)
}

// SPD — lightning bolt: a zigzag polygon drawn as a strip of
// triangles. Reads as "speed / initiative" without needing a label.
func drawStatIconSPD(cx, cy, r float32, col color.RGBA) {
	// Define the bolt as a closed 6-vertex polygon, then triangle-
	// fan from the first vertex. Vertices walk top-down across the
	// zigzag.
	verts := []rl.Vector2{
		{X: cx - r*0.05, Y: cy - r},          // top spike
		{X: cx + r*0.5, Y: cy - r*0.1},       // upper right notch
		{X: cx + r*0.05, Y: cy - r*0.1},      // inner step
		{X: cx + r*0.4, Y: cy + r},           // bottom spike
		{X: cx - r*0.5, Y: cy + r*0.1},       // lower left notch
		{X: cx - r*0.05, Y: cy + r*0.1},      // inner step
	}
	// Fan: (v0, v1, v2), (v0, v2, v3), (v0, v3, v4), (v0, v4, v5).
	for i := 1; i < len(verts)-1; i++ {
		drawTriangleCCW(verts[0], verts[i+1], verts[i], col)
	}
	// Bright inner highlight along the bolt's mid-axis — a tiny
	// gilt pip at the kink so the bolt has a "live" centre.
	rl.DrawCircleV(rl.NewVector2(cx, cy), 1.4, giltBright)
}

// drawDiamondPip paints a small filled diamond centered on (cx, cy)
// with half-extent r. Used at filigree corner joints and as the
// fleuron sigil flanking the pause-menu title.
func drawDiamondPip(cx, cy, r float32, col color.RGBA) {
	top := rl.NewVector2(cx, cy-r)
	right := rl.NewVector2(cx+r, cy)
	bottom := rl.NewVector2(cx, cy+r)
	left := rl.NewVector2(cx-r, cy)
	drawTriangleCCW(top, left, right, col)
	drawTriangleCCW(right, left, bottom, col)
}

// drawFleuron paints a four-direction gilt fleuron — a centre
// diamond flanked by teardrop leaves on all four compass points, with
// a bright inner pip at the heart. The classic "chapter divider"
// sigil illuminated manuscripts and 90s PC RPGs used as ornamental
// punctuation. Sized by `r` (the centre diamond's half-extent);
// flanking leaves scale to ~0.85 r so the whole motif fits in a
// roughly 6r square.
//
// The bright inner pip uses giltBright regardless of the caller's
// requested colour so the centre catches a highlight against a
// duller bracket tone — gives the fleuron a tiny "cast metal"
// reflection instead of reading as a flat silhouette.
func drawFleuron(cx, cy, r float32, col color.RGBA) {
	drawDiamondPip(cx, cy, r, col)
	leafR := r * 0.85
	leafOffset := r + 2
	// East leaf.
	eTip := rl.NewVector2(cx+leafOffset+leafR, cy)
	eTop := rl.NewVector2(cx+leafOffset, cy-leafR*0.55)
	eBot := rl.NewVector2(cx+leafOffset, cy+leafR*0.55)
	drawTriangleCCW(eTip, eTop, eBot, col)
	// West leaf.
	wTip := rl.NewVector2(cx-leafOffset-leafR, cy)
	wTop := rl.NewVector2(cx-leafOffset, cy-leafR*0.55)
	wBot := rl.NewVector2(cx-leafOffset, cy+leafR*0.55)
	drawTriangleCCW(wTip, wBot, wTop, col)
	// North leaf.
	nTip := rl.NewVector2(cx, cy-leafOffset-leafR)
	nLeft := rl.NewVector2(cx-leafR*0.55, cy-leafOffset)
	nRight := rl.NewVector2(cx+leafR*0.55, cy-leafOffset)
	drawTriangleCCW(nTip, nLeft, nRight, col)
	// South leaf.
	sTip := rl.NewVector2(cx, cy+leafOffset+leafR)
	sLeft := rl.NewVector2(cx-leafR*0.55, cy+leafOffset)
	sRight := rl.NewVector2(cx+leafR*0.55, cy+leafOffset)
	drawTriangleCCW(sTip, sRight, sLeft, col)
	// Bright speculum at the heart of the diamond.
	if r >= 3 {
		drawDiamondPip(cx, cy, r*0.35, giltBright)
	}
}

// drawFleuronsFlanking paints a gilt fleuron `gap` px outside each end of
// a centered label — the ◆ label ◆ motif shared by the menu titles and
// the level-up Apply gate. leftX is the label's left edge, w its measured
// width, cy the vertical midline.
func drawFleuronsFlanking(leftX, w, gap, cy, r float32, col color.RGBA) {
	drawFleuron(leftX-gap, cy, r, col)
	drawFleuron(leftX+w+gap, cy, r, col)
}

// drawPanel fills a rounded rect at a fixed pixel corner radius.
func drawPanel(x, y, w, h int32, fill color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawRectangleRounded(rect, fixedRoundnessFor(w, h, cornerRadius), 8, fill)
}

func drawPanelOutline(x, y, w, h int32, col color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawRectangleRoundedLinesEx(rect, fixedRoundnessFor(w, h, cornerRadius), 8, 1, col)
}

func drawSmallPanel(x, y, w, h int32, fill color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawRectangleRounded(rect, fixedRoundnessFor(w, h, smallCornerRadius), 6, fill)
}

func drawSmallPanelOutline(x, y, w, h int32, col color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawRectangleRoundedLinesEx(rect, fixedRoundnessFor(w, h, smallCornerRadius), 6, 1, col)
}

func fixedRoundnessFor(w, h int32, target float32) float32 {
	minDim := float32(core.MinInt(int(w), int(h)))
	if minDim <= 0 {
		return 0
	}
	r := 2 * target / minDim
	if r > 1 {
		r = 1
	}
	return r
}

// drawAccentStripe paints a thin colored bar inside a panel's left edge,
// inset slightly so it reads as part of the card rather than its border.
func drawAccentStripe(panelX, panelY, panelH int32, col color.RGBA) {
	if panelH < 16 {
		return
	}
	rl.DrawRectangle(panelX+5, panelY+8, stripeWidth, panelH-16, col)
}

// drawCard renders a wood-framed glass pane — the library aesthetic
// every panel-shaped surface uses. Owns the four-layer composition
// from UI_STANDARDS.md "Panel": outer woodDark stroke, woodMid band,
// woodLight inner highlight, glass tint body. The `accent` parameter
// is preserved as the optional left-spine stripe (for class-tinted
// active actor panels, etc.); pass a zero-alpha color to skip.
//
// `fill` should be one of the glass tokens (glassDeep / glassMid /
// glassWarm / glassDanger). `outline` is used as the OUTERMOST
// stroke; callers can pass `woodDark` for the standard frame or
// borderActive / borderDanger to tint the frame for state. The
// woodMid band + woodLight highlight are always painted between
// the outline and the glass body — the structural feel of the
// frame doesn't degrade for state changes.
func drawCard(x, y, w, h int32, fill, outline, accent color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	// Outer wood-framed card composition:
	//   1. Translucent glass body filling the whole pane (via
	//      drawGlassPane — the shared dark-wash + tint pair every
	//      translucent surface in the HUD uses).
	//   2. Three concentric hardwood frame strokes painted ON TOP
	//      as outlines, so the body underneath stays glass.
	drawGlassPane(x, y, w, h, fill)
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	roundness := fixedRoundnessFor(w, h, cornerRadius)
	frameThick := float32(woodFrameOuter + woodFrameBand + woodFrameInner)
	rl.DrawRectangleRoundedLinesEx(rect, roundness, 8, frameThick, woodLight)
	rl.DrawRectangleRoundedLinesEx(rect, roundness, 8, float32(woodFrameOuter+woodFrameBand), woodMid)
	rl.DrawRectangleRoundedLinesEx(rect, roundness, 8, float32(woodFrameOuter), outline)
	if accent.A > 0 {
		drawAccentStripe(x, y, h, accent)
	}
}

// drawGlassPane paints the canonical translucent glass body — a dark
// `glassBaseWash` underlay plus the family `fill` tint on top — with
// no wood frame. Use this for nested sub-panels inside a card (member
// cards, equipment slot bezels, skill rows, items detail card, tab
// tiles) so every translucent surface in the UI composites the same
// way against the world content behind it.
//
// drawCard internally calls this for its body. Callers that need a
// framed pane reach for drawCard; callers that need a frame-less
// translucent body (sub-panes inside a card) reach for this directly.
// drawSmallPanel — single-layer opaque-ish fill — stays the right
// choice for actual small chrome (status pills, chips, gilt rails).
func drawGlassPane(x, y, w, h int32, fill color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	// cornerRadius (4) for big card bodies; small sub-panes still
	// look round at this radius and the unified curvature makes
	// nested panes harmonise with their parent's frame.
	roundness := fixedRoundnessFor(w, h, cornerRadius)
	rl.DrawRectangleRounded(rect, roundness, 8, glassBaseWash)
	rl.DrawRectangleRounded(rect, roundness, 8, fill)
}

// ListRowState enumerates the visual states a single row in a panel
// list can take. UI_STANDARDS.md "Row" defines what each state
// renders — Rest is bare, Hover gilds the spine + promotes text,
// Selected pulses + underlines + uses inkAccent, Disabled mutes
// every layer.
type ListRowState int

const (
	ListRowRest ListRowState = iota
	ListRowHover
	ListRowSelected
	ListRowDisabled
)

// drawListRow paints the panel-row chrome for one of the four
// canonical row states. Single source of truth for "what does a
// list row look like?" — owners pass the row rect, the helper
// handles the inset fill, gilt spine, underline, and inset
// padding. Returns the text rect (panel rect minus the spine
// width + a small padding) so the caller can paint the row's
// content without recomputing the inset.
//
// Owners draw row text via drawTextWithShadow at FontBody (or
// FontSmall for dense lists). Active actor's spine is animated by
// the caller via fadeColor + the standard pulse frequency.
func drawListRow(rect rl.Rectangle, state ListRowState) rl.Rectangle {
	if rect.Width <= 0 || rect.Height <= 0 {
		return rect
	}
	// Inset glass plate, slightly darker than the panel body.
	body := glassMid
	if state == ListRowSelected {
		body = glassWarm
	}
	if state == ListRowDisabled {
		body = rl.NewColor(18, 14, 18, 130)
	}
	drawGlassPane(int32(rect.X), int32(rect.Y), int32(rect.Width), int32(rect.Height), body)

	// Gilt left spine on Hover / Selected.
	spineW := int32(3)
	switch state {
	case ListRowHover:
		rl.DrawRectangle(int32(rect.X)+4, int32(rect.Y)+5, spineW, int32(rect.Height)-10, giltDim)
	case ListRowSelected:
		rl.DrawRectangle(int32(rect.X)+4, int32(rect.Y)+5, spineW, int32(rect.Height)-10, giltBright)
		// Underline along the bottom edge — sells the "current row
		// in a ledger" feel.
		rl.DrawRectangle(int32(rect.X)+8, int32(rect.Y)+int32(rect.Height)-3, int32(rect.Width)-16, 1, giltDim)
	}

	// Text rect: padded past the spine on the left, breathing room
	// on the right.
	textPadL := float32(14)
	textPadR := float32(8)
	textRect := rl.NewRectangle(rect.X+textPadL, rect.Y+4, rect.Width-textPadL-textPadR, rect.Height-8)
	return textRect
}

// drawPanelHeading paints a FontHeading title with the standard
// wood-accent tick mark underline. Replaces the older drawHeading
// helper (kept as an alias below). Use this for every persistent
// HUD panel title and every modal heading.
//
// `accent` is the underline color — pass woodAccent for resting
// panels, borderActive for the focused / active panel, borderDanger
// for danger modals, etc. Header text is always inkPrimary.
func drawPanelHeading(font rl.Font, text string, x, y float32, accent color.RGBA) {
	drawTextWithShadowStyle(font, text, x, y, FontHeading, FontSpacingHeading, inkPrimary, shadowStrong, 1, 1)
	measure := measurePanelHeading(font, text)
	tickW := int32(measure.X)
	if tickW < headingTickMinWidth {
		tickW = headingTickMinWidth
	}
	rl.DrawRectangle(int32(x), int32(y+measure.Y+2), tickW, 2, accent)
}

// panelHeadingMeasureCache memoizes rl.MeasureTextEx for panel-heading
// strings. drawPanelHeading runs every frame for every visible HUD
// panel ("COMBAT LOG", "TURN ORDER", "AREA", "PAUSED", the action-
// menu header, etc.). All callers use FontHeading + FontSpacingHeading
// so the cache is keyed solely on the text and the font texture ID.
var panelHeadingMeasureCache = make(map[string]rl.Vector2, 16)
var panelHeadingMeasureCacheFontID uint32

func measurePanelHeading(font rl.Font, text string) rl.Vector2 {
	if font.Texture.ID != panelHeadingMeasureCacheFontID {
		for k := range panelHeadingMeasureCache {
			delete(panelHeadingMeasureCache, k)
		}
		panelHeadingMeasureCacheFontID = font.Texture.ID
	}
	if v, ok := panelHeadingMeasureCache[text]; ok {
		return v
	}
	v := rl.MeasureTextEx(font, text, FontHeading, FontSpacingHeading)
	panelHeadingMeasureCache[text] = v
	return v
}

// pulse oscillates 0..1 at the given frequency in Hz.
func pulse(speed float64) float32 {
	return 0.5 + 0.5*float32(math.Sin(rl.GetTime()*speed*math.Pi*2))
}

// fadeColor returns col scaled by alpha multiplier in 0..1.
func fadeColor(col color.RGBA, alpha float32) color.RGBA {
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	col.A = uint8(float32(col.A) * alpha)
	return col
}

// colorWithAlpha replaces col's alpha channel with `byteAlpha` (0-255).
// Differs from fadeColor which multiplies the existing alpha by a
// normalized 0..1 factor — colorWithAlpha is the "I know exactly what
// alpha I want, regardless of the source color's alpha" form. Used by
// turn-order panel tints (per-class color at varying transparencies
// based on row state) where the source colors already encode the hue
// and the alpha is a UI-state knob.
func colorWithAlpha(col color.RGBA, byteAlpha uint8) color.RGBA {
	col.A = byteAlpha
	return col
}

// hpFillColor selects a tier color based on remaining HP percent.
func hpFillColor(value, maxValue int) color.RGBA {
	if maxValue <= 0 {
		return barHPLow
	}
	p := float32(value) / float32(maxValue)
	switch {
	case p > 0.6:
		return barHPHigh
	case p > 0.3:
		return barHPMid
	default:
		return barHPLow
	}
}

// barTrackColor is the dim glass tint behind every HP/MP bar. Hoisted
// from drawBar's body so the per-call construction of the same color
// literal lives once at package scope, matching the pattern used by
// minimapOutOfBoundsColor and panelsMapOutOfBoundsColor.
var barTrackColor = rl.NewColor(8, 12, 22, 140)

// barLabelMeasureCache memoizes rl.MeasureTextEx for short, constant
// bar labels like "HP" and "MP". drawBar runs ~16 times per frame
// across the party ribbon and enemy roster; the label measurement
// is a cgo round-trip that returns the same value every time for a
// given (font.Texture.ID, label) pair. Cleared whenever the active
// font changes (font reload after a setting flip, etc.) via
// resetBarLabelMeasureCache from the font-swap path.
var barLabelMeasureCache = make(map[string]rl.Vector2, 8)

// barLabelMeasureCacheFontID tracks the font the cache was built
// against. raylib's rl.Font carries a Texture2D ID; if it shifts,
// the cached pixel widths are stale and the map is cleared.
var barLabelMeasureCacheFontID uint32

func measureBarLabel(font rl.Font, label string) rl.Vector2 {
	if font.Texture.ID != barLabelMeasureCacheFontID {
		for k := range barLabelMeasureCache {
			delete(barLabelMeasureCache, k)
		}
		barLabelMeasureCacheFontID = font.Texture.ID
	}
	if v, ok := barLabelMeasureCache[label]; ok {
		return v
	}
	v := rl.MeasureTextEx(font, label, FontTiny, 1)
	barLabelMeasureCache[label] = v
	return v
}

// barValueMeasureCache memoizes rl.MeasureTextEx for value strings
// like "10/20" rendered to the right of each HP/MP bar. drawBar
// produces ~14 of these per frame (party HP+MP + enemy roster); the
// string changes only on HP/MP mutation, not 60 Hz, so caching by
// the value text catches a long run of frames where it's stable.
// The cache grows by one entry per unique value pair seen — small,
// since most bars hover at a handful of common HP/MP pairs.
var barValueMeasureCache = make(map[string]rl.Vector2, 32)
var barValueMeasureCacheFontID uint32

func measureBarValue(font rl.Font, valText string) rl.Vector2 {
	if font.Texture.ID != barValueMeasureCacheFontID {
		for k := range barValueMeasureCache {
			delete(barValueMeasureCache, k)
		}
		barValueMeasureCacheFontID = font.Texture.ID
	}
	if v, ok := barValueMeasureCache[valText]; ok {
		return v
	}
	v := rl.MeasureTextEx(font, valText, FontSmall, 1)
	barValueMeasureCache[valText] = v
	return v
}

// drawBar renders a track + filled portion + thin outline, all rounded.
// label is drawn as a small uppercase tag at the bar's left, value text on right.
func drawBar(font rl.Font, x, y, width, height float32, label string, value, maxValue int, fill color.RGBA, muted bool) {
	if maxValue <= 0 {
		maxValue = 1
	}
	pct := float32(value) / float32(maxValue)
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	track := barTrackColor
	outline := borderDim
	if muted {
		fill = rl.NewColor(96, 84, 92, 230)
	}
	ix, iy, iw, ih := int32(x), int32(y), int32(width), int32(height)
	drawSmallPanel(ix, iy, iw, ih, track)
	if pct > 0 {
		fillW := int32(float32(iw-2) * pct)
		if fillW > 0 {
			drawSmallPanel(ix+1, iy+1, fillW, ih-2, fill)
		}
	}
	drawSmallPanelOutline(ix, iy, iw, ih, outline)

	// Tape-measure ticks — small triangular notches biting in from
	// the top and bottom edges of the bar at the 25/50/75 % marks.
	// The 50 % mark gets a deeper notch (the cardinal index on a
	// brass scale); the 25 / 75 marks are shorter (secondary
	// graduations). Notches read as cast-metal cutouts against
	// both the fill and the empty track because they're triangles
	// pointing inward in a wood-accent tone, distinct from both
	// glass and any HP/MP tint. Skipped on tiny bars (<80 px wide
	// or <12 px tall) so compact UI surfaces don't look busy.
	if iw >= 80 && ih >= 12 && !muted {
		tickCol := fadeColor(woodAccent, 0.85)
		ticks := [3]struct {
			t     float32
			depth float32
			width float32
		}{
			{t: 0.25, depth: 2.5, width: 3},
			{t: 0.5, depth: 4, width: 4},
			{t: 0.75, depth: 2.5, width: 3},
		}
		for _, tk := range ticks {
			tx := float32(ix) + 1 + float32(iw-2)*tk.t
			// Top notch — triangle pointing DOWN into the bar.
			topApex := rl.NewVector2(tx, float32(iy)+tk.depth)
			topL := rl.NewVector2(tx-tk.width/2, float32(iy))
			topR := rl.NewVector2(tx+tk.width/2, float32(iy))
			drawTriangleCCW(topL, topApex, topR, tickCol)
			// Bottom notch — triangle pointing UP into the bar.
			botApex := rl.NewVector2(tx, float32(iy+ih)-tk.depth)
			botL := rl.NewVector2(tx-tk.width/2, float32(iy+ih))
			botR := rl.NewVector2(tx+tk.width/2, float32(iy+ih))
			drawTriangleCCW(botL, botR, botApex, tickCol)
		}
	}

	// Bar labels (HP / MP / etc.) — always FontTiny per UI_STANDARDS.md
	// (the bar IS small, the value text is what reads at a glance).
	// Cream-bright color + heavy shadow so the tag pops on any fill.
	labelSize := FontTiny
	labelColor := inkPrimary
	if muted {
		labelColor = textDim
	}
	// labelSize stays FontTiny throughout; the measurement is keyed on
	// it inside measureBarLabel for cgo-call avoidance.
	labelMeasure := measureBarLabel(font, label)
	labelY := y + (float32(ih)-labelMeasure.Y)/2 - 1
	labelX := x + barLabelPadLeft
	rl.DrawTextEx(font, label, rl.NewVector2(labelX+2, labelY+2), labelSize, 1, shadowHeavy)
	rl.DrawTextEx(font, label, rl.NewVector2(labelX+1, labelY+1), labelSize, 1, shadowHeavy)
	rl.DrawTextEx(font, label, rl.NewVector2(labelX, labelY), labelSize, 1, labelColor)

	valText := ""
	if maxValue > 0 {
		valText = formatBarValue(value, maxValue)
	}
	if valText != "" {
		// Value text is always FontSmall per UI_STANDARDS.md — the
		// number is the bar's readable content and stays consistent
		// regardless of bar height. Bright by default; faded only
		// when muted. Double-offset drop shadow for contrast on
		// any fill.
		valSize := FontSmall
		valColor := textPrimary
		if muted {
			valColor = textDim
		}
		// Value text size is locked at FontSmall inside measureBarValue;
		// valSize stays in scope below for the draw calls.
		valMeasure := measureBarValue(font, valText)
		valY := y + (float32(ih)-valMeasure.Y)/2 - 1
		valX := x + width - valMeasure.X - barValuePadRight
		rl.DrawTextEx(font, valText, rl.NewVector2(valX+2, valY+2), valSize, 1, shadowHeavy)
		rl.DrawTextEx(font, valText, rl.NewVector2(valX+1, valY+1), valSize, 1, shadowHeavy)
		rl.DrawTextEx(font, valText, rl.NewVector2(valX, valY), valSize, 1, valColor)
	}
}

func formatBarValue(value, maxValue int) string {
	// Direct strconv concat avoids the fmt formatter machinery on a
	// path that runs once per visible bar per frame (HP + MP on every
	// party card, plus enemy roster bars). "%d/%d" via fmt.Sprintf
	// allocates ~3× the bytes of the result; this routes to the
	// minimal strconv.Itoa + concat.
	return strconv.Itoa(value) + "/" + strconv.Itoa(maxValue)
}

// drawTriangleCCW wraps rl.DrawTriangle with an explicit "vertices are in
// counter-clockwise order in screen-Y-down coords" contract. raylib's 2D
// pipeline has GL_CULL_FACE enabled on some drivers (Intel/AMD have been
// observed culling CW-wound triangles silently — the triangle just doesn't
// appear). Use this helper at every 2D triangle call site so the winding
// requirement is named at the call, not buried in a comment somewhere.
//
// "CCW in screen-Y-down" means the signed cross product of (b-a)×(c-b) is
// NEGATIVE — that's the inverted convention vs the y-up math-textbook one
// because screen Y increases downward.
func drawTriangleCCW(a, b, c rl.Vector2, col color.RGBA) {
	rl.DrawTriangle(a, b, c, col)
}

// drawArrowMarker paints a small triangle chevron. The base sits at `center`
// perpendicular to the direction; the apex is `center + (tipDx, tipDy)`.
// Base width is 2*halfWidth. Used by HUD selection / target / active-actor
// indicators where a tiny arrow reads better than a label — saves party,
// battle, and item-target panels from each computing their own three
// rl.Vector2 corners by hand. Goes through drawTriangleCCW so the winding
// constraint stays visible.
func drawArrowMarker(center rl.Vector2, tipDx, tipDy, halfWidth float32, col color.RGBA) {
	tipLen := float32(math.Sqrt(float64(tipDx*tipDx + tipDy*tipDy)))
	if tipLen == 0 {
		return
	}
	px := -tipDy / tipLen * halfWidth
	py := tipDx / tipLen * halfWidth
	// Apex → base1 → base2 is CCW in screen-Y-down for any tipDx/tipDy
	// because px/py is the (tipDx,tipDy) vector rotated +90° in screen
	// space (which is -90° in math space → CCW).
	drawTriangleCCW(
		rl.NewVector2(center.X+tipDx, center.Y+tipDy),
		rl.NewVector2(center.X-px, center.Y-py),
		rl.NewVector2(center.X+px, center.Y+py),
		col,
	)
}

// drawTextWithShadow paints text twice: once offset by (1,1) at shadowStrong,
// once at the requested color. The single +1 offset reads as a clean drop
// shadow under most HUD sizes; callers that want a heavier shadow for large
// titles (menu rows, debug pills) go through drawTextWithShadowStyle. Lives
// here alongside the shadowLight/Mid/Strong/Heavy palette it consumes.
func drawTextWithShadow(font rl.Font, text string, x, y, size float32, col color.RGBA) {
	drawTextWithShadowStyle(font, text, x, y, size, 1, col, shadowStrong, 1, 1)
}

// drawTextWithShadowStyle is the parametric form of drawTextWithShadow.
// shadowCol picks the drop color (shadowLight/Mid/Strong/Heavy above);
// offX/offY pick the drop offset in pixels; `spacing` is the letter
// spacing passed through to rl.DrawTextEx. Use this when an ad-hoc
// shadow alpha, offset, or letter spacing is actually load-bearing
// (splash titles, menu rows, the debug overlay's 1.2 spacing); prefer
// the non-styled drawTextWithShadow for everything else so HUD shadows
// stay consistent.
func drawTextWithShadowStyle(font rl.Font, text string, x, y, size, spacing float32, col, shadowCol color.RGBA, offX, offY float32) {
	rl.DrawTextEx(font, text, rl.NewVector2(x+offX, y+offY), size, spacing, shadowCol)
	rl.DrawTextEx(font, text, rl.NewVector2(x, y), size, spacing, col)
}

// drawHeading is the legacy panel-heading helper. Routes through
// drawPanelHeading so the heading size is the standardized FontHeading
// across every call site — the old hand-tuned 20pt is gone. Kept as
// an alias because dozens of call sites pass int32 coords; over time
// they should adopt drawPanelHeading directly.
func drawHeading(font rl.Font, text string, x, y int32, accent color.RGBA) {
	drawPanelHeading(font, text, float32(x), float32(y), accent)
}
