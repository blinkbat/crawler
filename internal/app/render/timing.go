package render

import (
	"math"
	"strconv"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Per-bar heading text. Lifted to module-level constants so the press /
// charge / sequence draw paths read from one source instead of inlining
// the string at each draw call site (the press bar also flips between
// STRIKE / DEFEND depending on whose timing phase is active). Any future
// "translate the bar text" pass lands here once.
const (
	timingHeadingStrike       = "STRIKE!"
	timingHeadingDefend       = "DEFEND!"
	timingHeadingCharge       = "CHARGE!"
	timingHeadingPickpocket   = "PICKPOCKET!"
	timingHeadingReels        = "STOP THE REELS!"
	timingHeadingRecallMemo   = "MEMORIZE!"
	timingHeadingRecallRecall = "RECALL!"
)

// Per-mode heading + base-fill tints, paired with the timingHeading*
// strings above. Named so the bar's signature hue lives next to its
// label instead of as a bare rl.NewColor literal inside each draw
// function — a palette pass touches one block. (Pickpocket derives its
// tint from seqOkColor and stays at its call site.)
var (
	timingHeadingStrikeColor       = mute(rl.NewColor(255, 232, 168, 240)) // warm gold
	timingHeadingDefendColor       = mute(rl.NewColor(168, 220, 255, 240)) // cool blue
	timingHeadingChargeColor       = mute(rl.NewColor(255, 184, 96, 240))  // warm orange
	timingHeadingReelsColor        = mute(rl.NewColor(255, 244, 144, 240)) // gold-yellow gamble
	timingHeadingRecallMemoColor   = mute(rl.NewColor(255, 244, 144, 240)) // memorize: held-yellow
	timingHeadingRecallRecallColor = mute(rl.NewColor(140, 232, 168, 240)) // recall: thief green
)

// Shared timing-bar layout + alpha tunables. Every bar that lays out an arrow
// row (sequence / recall) or a dwindling timer strip (sequence / recall) reads
// these, so a tweak can't drift the bars apart — the cause of the copy-paste
// the bar helpers below were extracted to kill.
const (
	barRowPadPx          = float32(18)   // horizontal padding inside a bar row
	barCellGapPx         = float32(12)   // gap between reel cells
	arrowSizeSlotFrac    = float32(0.35) // arrow half-extent as a fraction of slot width
	arrowSizeBarHCap     = float32(0.85) // arrow size ceiling as a fraction of bar height
	timerStripUrgentFrac = float32(0.30) // remaining-time fraction below which the strip reads red
	timerStripWarnFrac   = float32(0.55) // ...below which it reads warn-amber
	barHighlightAlpha    = uint8(245)    // fully-lit element (pending arrow, locked reel frame, cursor underline)
	iconShadowAlpha      = uint8(180)    // arrow drop-shadow alpha
	reelRimAlpha         = uint8(150)    // dark rim behind a reel symbol (definition over glass)
)

// fadeForFlash scales a color's alpha by the flash-hold envelope when the bar
// is in its resolved flash, else returns it unchanged. Replaces the
// `if flashing { col.A = uint8(float32(col.A) * flashAlpha(flashTimer)) }`
// block the press / sequence / reel / recall draws each open-coded.
func fadeForFlash(col rl.Color, flashing bool, flashTimer float32) rl.Color {
	if flashing {
		col.A = uint8(float32(col.A) * flashAlpha(flashTimer))
	}
	return col
}

// arrowRowLayout returns the shared geometry for an evenly-spaced arrow row
// (sequence + recall bars). ok=false when the bar is too narrow or empty, so
// callers bail rather than draw flipped-sign geometry. The per-slot center is
// drawX + pad + slotWidth*(i+0.5).
func arrowRowLayout(barW, barH float32, count int) (pad, slotWidth, arrowSize float32, ok bool) {
	if count <= 0 {
		return 0, 0, 0, false
	}
	pad = barRowPadPx
	available := barW - pad*2
	if available <= 0 {
		return 0, 0, 0, false
	}
	slotWidth = available / float32(count)
	arrowSize = slotWidth * arrowSizeSlotFrac
	if arrowSize > barH*arrowSizeBarHCap {
		arrowSize = barH * arrowSizeBarHCap
	}
	return pad, slotWidth, arrowSize, true
}

// drawSequenceCursorUnderline paints the bright "next slot" underline beneath
// the arrow at (cx, cy). Shared by the sequence and recall bars.
func drawSequenceCursorUnderline(cx, cy, arrowSize float32) {
	ux := cx - arrowSize*0.85
	uw := arrowSize * 1.7
	uy := cy + arrowSize + 8
	rl.DrawRectangle(int32(ux)+2, int32(uy)+2, int32(uw), 4, shadowLight)
	rl.DrawRectangle(int32(ux), int32(uy), int32(uw), 4, colorWithAlpha(timingHeldColor, barHighlightAlpha))
}

// drawDwindlingTimerStrip paints the thin center-anchored line under a bar's
// content that shrinks toward the center as `remaining` (1 → 0) drains,
// reddening as the clock runs out. Shared by the sequence and recall bars;
// callers gate on `!flashing && Duration > 0` and pass 1-Progress().
func drawDwindlingTimerStrip(drawX, y, barW, barH, remaining float32) {
	if remaining < 0 {
		remaining = 0
	}
	stripH := float32(3)
	stripY := y + barH + 10
	stripCol := colorWithAlpha(seqOkColor, 230)
	if remaining < timerStripUrgentFrac {
		stripCol = colorWithAlpha(seqFailColor, 240)
	} else if remaining < timerStripWarnFrac {
		stripCol = timingWarnColor
	}
	visW := barW * remaining
	stripX := drawX + (barW-visW)*0.5
	rl.DrawRectangle(int32(stripX)+1, int32(stripY)+1, int32(visW), int32(stripH), shadowLight)
	rl.DrawRectangle(int32(stripX), int32(stripY), int32(visW), int32(stripH), stripCol)
}

// --- Bar juice helpers ----------------------------------------------------
//
// barShake, barThrob, and tickFreshness are the three reusable feedback
// curves the press / charge / sequence bars all share. Each is keyed off
// the bar's existing TimingFlash hold so the juice fades out as the flash
// resolves — no extra state, no new timers, no per-frame plumbing.

// blendTowardWhite mixes col with pure white by `whiteAmount` ∈ [0, 1].
// At 0 the original color is returned untouched; at 1 the result is white.
// Used to keep grade-tinted UI elements distinct from grade-tinted
// backgrounds (most importantly the press cursor sitting on its own
// preview zone — a pure-color cursor would visually disappear).
func blendTowardWhite(col rl.Color, whiteAmount float32) rl.Color {
	out := core.MixColor(col, rl.White, float64(core.Clamp(whiteAmount, 0, 1)))
	out.A = col.A // preserve the source alpha; rl.White would drag it to 255
	return out
}

// Miss-flash bar-shake tunables. shakeAmplitudePx is the peak horizontal
// offset right after a missed press; shakeCyclesPerFlash is how many
// full sine cycles play over the flash hold (a higher number reads as
// more "rejection" energy). Named so a balance pass touches one line
// instead of two embedded magic numbers.
const (
	shakeAmplitudePx    = float32(6.5)
	shakeCyclesPerFlash = float32(9)
)

// barShake returns a horizontal pixel offset for the bar during a Miss
// flash. Damped sinusoid: peaks right after the missed press, decays out
// over the flash hold. Zero for any other grade so the bar only "rejects"
// the player's input when they fully whiffed.
func barShake(timing core.TimingState, flashTimer float32) float32 {
	if timing.Quality != core.TimingQualityMiss || flashTimer <= 0 {
		return 0
	}
	if core.TimingFlashDuration <= 0 {
		return 0
	}
	age := 1 - flashTimer/core.TimingFlashDuration
	if age < 0 {
		age = 0
	}
	if age > 1 {
		age = 1
	}
	amp := (1 - age) * shakeAmplitudePx
	return float32(math.Sin(float64(age)*math.Pi*float64(shakeCyclesPerFlash))) * amp
}

// qualityVisuals is the render-side per-grade attribute table. Color and
// throb intensity both vary by quality, and both used to live in parallel
// switches — collapsed here so a balance / palette pass touches one row.
// Indexed by core.TimingQualityMiss..Excellent.
var qualityVisuals = [...]struct {
	ThrobIntensity float32
	AttackColor    rl.Color
	DefendColor    rl.Color
}{
	core.TimingQualityMiss:      {ThrobIntensity: 0, AttackColor: rl.NewColor(220, 76, 76, 255), DefendColor: rl.NewColor(220, 76, 76, 255)},
	core.TimingQualityNice:      {ThrobIntensity: 0.08, AttackColor: rl.NewColor(184, 96, 80, 255), DefendColor: rl.NewColor(56, 110, 184, 255)},
	core.TimingQualityGood:      {ThrobIntensity: 0.12, AttackColor: rl.NewColor(232, 144, 80, 255), DefendColor: rl.NewColor(80, 152, 220, 255)},
	core.TimingQualityGreat:     {ThrobIntensity: 0.16, AttackColor: rl.NewColor(255, 188, 88, 255), DefendColor: rl.NewColor(120, 200, 248, 255)},
	core.TimingQualityExcellent: {ThrobIntensity: 0.22, AttackColor: rl.NewColor(255, 244, 144, 255), DefendColor: rl.NewColor(196, 240, 255, 255)},
}

// init asserts qualityVisuals covers every timing grade. Pairs with
// the analogous init() in core/config.go (timingGrades) and battle.go
// (gradeSounds) so the three parallel tables stay in sync. The fixed-
// size array form means a missing row produces a zero-valued entry,
// not a length mismatch — so the assert pivots on TimingQualityCount.
func init() {
	if len(qualityVisuals) != int(core.TimingQualityCount) {
		panic("render/timing: qualityVisuals length must match core.TimingQualityCount")
	}
}

// barThrob returns the height scale multiplier for a bar during a graded
// flash. Higher grades pulse harder — visual confirmation that the player
// hit *something*. Miss stays at 1.0 so the bar shakes instead (see
// barShake). The bar's centerline stays fixed; the multiplier grows
// height around it.
func barThrob(timing core.TimingState, flashTimer float32) float32 {
	if flashTimer <= 0 {
		return 1
	}
	intensity := float32(0)
	if timing.Quality >= 0 && timing.Quality < len(qualityVisuals) {
		intensity = qualityVisuals[timing.Quality].ThrobIntensity
	}
	return 1 + intensity*flashAlpha(flashTimer)
}

// tickFreshness returns a [0, 1] strength for a charge bar's tick marker
// based on how recently the cursor crossed it. The tick flashes brighter +
// wider while fresh, then settles back to its baseline. tickFlashDuration
// is local to this helper since it's the only caller. tickPct is the
// tick's *visual* position on the bar — we invert through the cursor
// curve to find the elapsed time the cursor actually reached it.
func tickFreshness(timing core.TimingState, tickPct float32) float32 {
	const tickFlashDuration = float32(0.22)
	tickTime := core.ChargeElapsedForVisual(tickPct, timing.Duration)
	age := timing.Elapsed - tickTime
	if age < 0 || age > tickFlashDuration {
		return 0
	}
	return 1 - age/tickFlashDuration
}

// applyBarMotion shifts (x, y, h) by the throb/shake feedback so the bar's
// body, window zones, and cursor stay in sync. Returns the adjusted
// (xOffset, yOffset, scaledH). The caller adds xOffset/yOffset to its draw
// positions and uses scaledH in place of barH.
func applyBarMotion(timing core.TimingState, flashTimer, barH float32) (xOffset, yOffset, scaledH float32) {
	shake := barShake(timing, flashTimer)
	throb := barThrob(timing, flashTimer)
	scaledH = barH * throb
	yOffset = -(scaledH - barH) / 2
	xOffset = shake
	return
}

// drawTimingBar paints the active timed-hit bar above the party ribbon. The
// bar dispatches by Kind: press-mode shows a sliding cursor over nested
// quality zones; charge-mode shows three filling segments + peak + decay.
func drawTimingBar(g core.GameState, assets Resources) {
	if !g.Battle.Timing.Active {
		return
	}
	timing := g.Battle.Timing
	flashing := timing.Resolved && g.Battle.TimingFlash > 0
	if timing.Resolved && !flashing {
		return
	}

	x, y, barW, barH := timingBarLayout()

	switch timing.Kind {
	case core.TimingKindCharge, core.TimingKindOvercharge:
		// Overcharge reuses the charge bar's visuals; its post-peak decay
		// band already reads as a danger zone (where the overload lives).
		drawChargeBar(timing, g, assets, x, y, barW, barH, flashing)
	case core.TimingKindSequence:
		drawSequenceBar(timing, g, assets, x, y, barW, barH, flashing)
	case core.TimingKindRecall:
		drawRecallBar(timing, g, assets, x, y, barW, barH, flashing)
	case core.TimingKindReels:
		drawReelBar(timing, g, assets, x, y, barW, barH, flashing)
	default:
		drawPressBar(timing, g, assets, x, y, barW, barH, flashing)
	}
}

// timingBarLayout returns the bar's screen-space rectangle. Both kinds share
// the same footprint so switching modes doesn't cause the strip to jump.
func timingBarLayout() (x, y, barW, barH float32) {
	screenW, _ := screenSizeF()
	barH = 34
	barW = screenW * 0.62
	if barW < 380 {
		barW = 380
	}
	if barW > screenW-32 {
		barW = screenW - 32
	}
	x = centerXF(barW)
	y = PartyRibbonTopY() - barH - 28
	return
}

// drawTimingHeading paints the centered prompt above the timing bar. Color
// shifts to the quality tint during the flash hold so the prompt itself reads
// the result. (Distinct from theme.go's drawHeading which is a generic panel
// header.)
func drawTimingHeading(font rl.Font, text string, x, barW, y float32, baseCol rl.Color, flashing bool, flashCol rl.Color) {
	size := FontHeading
	col := baseCol
	if flashing {
		col = flashCol
		// Flash punch — was a bespoke 34px; FontTitle (36) is the
		// next size up on the locked 5-size scale (UI_STANDARDS.md).
		size = FontTitle
	}
	measure := measureTimingHeading(font, text, size)
	hx := x + (barW-measure.X)/2
	hy := y - measure.Y - 6
	drawTextWithShadowStyle(font, text, hx, hy, size, 1.5, col, shadowStrong, 2, 2)
}

// timingHeadingMeasureCache memoizes drawTimingHeading's MeasureTextEx;
// the size flips between FontHeading and FontTitle during the flash hold,
// which the shared measureCache keys on.
var timingHeadingMeasureCache measureCache

func measureTimingHeading(font rl.Font, text string, size float32) rl.Vector2 {
	return timingHeadingMeasureCache.measure(font, text, size, 1.5)
}

// applyTimingFlashCursor draws the bright halo around the frozen cursor
// during the flash hold and returns the (width, color) overrides the
// caller's final cursor draw should pick up. Shared by press and charge
// bars — both handle flashing identically.
func applyTimingFlashCursor(curX, y, barH, flashTimer float32, base rl.Color) (float32, rl.Color) {
	const cursorW = float32(12)
	flashCol := base
	flashCol.A = 255
	halo := flashCol
	halo.A = uint8(180 * flashAlpha(flashTimer))
	rl.DrawRectangle(int32(curX-cursorW*2), int32(y)-8, int32(cursorW*4), int32(barH)+16, halo)
	return cursorW, flashCol
}

// timingTrackColor is the dark base fill behind every timing bar (press
// and charge). Named so a palette pass touches one line instead of the
// per-function literals it replaced.
var timingTrackColor = rl.NewColor(14, 16, 26, 230)

// drawTimingTrack paints the dark base fill behind a timing bar. During
// the flash hold the track fades to the grade color so the whole bar
// pulses with the result. Shared by the press and charge bars so the
// flash-fade math (and its 220 peak-alpha literal) lives in one place.
func drawTimingTrack(drawX, drawY, barW, drawnH float32, quality int, isDefend, flashing bool, timingFlash float32) {
	trackCol := timingTrackColor
	if flashing {
		trackCol = qualityColor(quality, isDefend)
		trackCol.A = uint8(220 * flashAlpha(timingFlash))
	}
	rl.DrawRectangle(int32(drawX), int32(drawY), int32(barW), int32(drawnH), trackCol)
}

// drawExcellentShockwave paints the expanding ring that pops from the
// frozen cursor on an Excellent timing resolution, fading as the flash
// hold (flashTimer) drains. Shared verbatim by the press and charge bars
// so the flourish reads identically; isDefend picks the grade hue.
func drawExcellentShockwave(curX, drawY, drawnH, flashTimer float32, isDefend bool) {
	phase := 1 - flashAlpha(flashTimer) // 0 fresh → 1 done
	radius := 14 + phase*72
	ringCol := qualityColor(core.TimingQualityExcellent, isDefend)
	ringCol.A = uint8(220 * (1 - phase))
	cy := drawY + drawnH*0.5
	rl.DrawCircleLines(int32(curX), int32(cy), radius, ringCol)
	rl.DrawCircleLines(int32(curX), int32(cy), radius+2, ringCol)
}

// drawPressBar is the original press-kind bar: nested quality zones inside
// the acceptance window, sliding cursor, flash on press.
//
// Juice layers stacked on top of the base bar:
//   - barThrob: bar height pulses on graded flashes (bigger on Excellent)
//   - barShake: bar shudders horizontally on a Miss flash
//   - cursor preview color: cursor tints to the grade you'd score right now
//     while you're inside the acceptance window, so Excellent shimmers up to
//     you before you commit
//   - shockwave ring: Excellent flashes spawn an expanding ring from the
//     frozen cursor position
func drawPressBar(timing core.TimingState, g core.GameState, assets Resources, x, y, barW, barH float32, flashing bool) {
	isDefend := g.Battle.Phase == core.BattleEnemyTiming

	heading := timingHeadingStrike
	baseCol := timingHeadingStrikeColor
	if isDefend {
		heading = timingHeadingDefend
		baseCol = timingHeadingDefendColor
	}

	xOff, yOff, drawnH := applyBarMotion(timing, g.Battle.TimingFlash, barH)
	drawX := x + xOff
	drawY := y + yOff

	drawTimingHeading(assets.hudFont, heading, drawX, barW, drawY, baseCol, flashing, qualityColor(timing.Quality, isDefend))

	// Track — solid dark fill, no border. During the flash hold the track
	// fades to the quality color so the whole bar pulses with the result.
	drawTimingTrack(drawX, drawY, barW, drawnH, timing.Quality, isDefend, flashing, g.Battle.TimingFlash)

	// Quality zones inside the acceptance window — Nice (outermost) → Good →
	// Great → Excellent (centered on the sweet spot). Each is a full-height
	// solid color stripe; nesting communicates the grading without any lines.
	// Two-zone press bars (Swipe) paint the nested bands for both windows so
	// each hit zone reads with its own gradient.
	if timing.IsTallyMode() {
		// Tally bars always render their per-window pips, even
		// during the resolution flash. Without this, the final
		// hit's FlashTimer is set but never seen — pressing the
		// last accept window auto-resolves the bar (Hits == count),
		// which flips `flashing` true on the next frame and the
		// player loses the per-window confirmation pop the earlier
		// hits got. Drawing under the flash also gives the
		// resolution moment a richer texture: the bar-wide quality
		// wash sits behind, the individual windows still telegraph
		// what landed.
		drawTallyBar(timing, drawX, drawY, barW, drawnH, isDefend)
	} else if !flashing {
		drawPressWindowZones(timing.WindowStart, timing.WindowEnd, timing.SweetSpot, timing.Duration, drawX, drawY, barW, drawnH, isDefend)
	}

	// Cursor — a fat vertical block sliding across the bar. Frozen at the
	// press position during the flash hold so the player sees where they hit.
	// During the intro pause (TimingIntro > 0) Tick isn't called, so
	// Progress() naturally stays at 0 — no special-casing needed here.
	curPct := timing.Progress()
	curX := drawX + curPct*barW
	cursorW := float32(8)
	cursorCol := timingCursorColor
	// Live grade preview: while the cursor's inside the acceptance window
	// and the player hasn't pressed yet, tint it toward the grade it would
	// land. We blend 35% toward the grade color from white instead of
	// taking the grade color pure — pure grade-color would let the cursor
	// disappear when sitting *on top of* its own grade zone (both bright
	// yellow at Excellent → invisible cursor). The blended tint keeps the
	// cursor distinct from the zone behind it while still communicating
	// "press NOW for X."
	if !flashing && !timing.Resolved {
		if preview := timing.PreviewQuality(); preview > core.TimingQualityMiss {
			cursorCol = blendTowardWhite(qualityColor(preview, isDefend), 0.55)
		}
	}
	if flashing {
		cursorW, cursorCol = applyTimingFlashCursor(curX, drawY, drawnH, g.Battle.TimingFlash, qualityColor(timing.Quality, isDefend))
	}
	rl.DrawRectangle(int32(curX-cursorW/2), int32(drawY)-6, int32(cursorW), int32(drawnH)+12, cursorCol)

	// Excellent shockwave — an expanding ring from the frozen cursor
	// position during the flash hold. Only on Excellent so the moment
	// reads as special; lesser grades stay quiet.
	if flashing && timing.Quality == core.TimingQualityExcellent {
		drawExcellentShockwave(curX, drawY, drawnH, g.Battle.TimingFlash, isDefend)
	}
}

// drawChargeBar paints the charge-and-release bar. Layout from left to right:
//   - segments 1, 2, 3: separated by tick lines, fill with charge color as
//     the cursor crosses them while the player is holding the button
//   - peak window: bright "release now" zone right after segment 3
//   - decay zone: dim warning fade until 100%
//
// The cursor sweeps regardless of hold state — Elapsed counts up always — so
// the player sees how close they are to the peak window even before pressing.
func drawChargeBar(timing core.TimingState, g core.GameState, assets Resources, x, y, barW, barH float32, flashing bool) {
	heading := timingHeadingCharge
	baseCol := timingHeadingChargeColor
	// Intro-pause heading: while the player hasn't engaged yet (intro
	// counter > 0), swap to the "Press to start" prompt in the hint
	// tone so they see the bar is waiting on input rather than already
	// running. Replaces the prior overlay-stamp from drawTimingBar
	// that fought the orange CHARGE prompt for the same heading slot.
	if !flashing && g.Battle.TimingIntro > 0 {
		heading = "Press to start"
		baseCol = textHint
	} else if timing.Pressed {
		// Past the peak start? Push the prompt toward "release now" feel.
		if timing.Elapsed >= timing.WindowStart {
			heading = "RELEASE!"
			baseCol = colorWithAlpha(timingHeldColor, 250)
		}
	}

	xOff, yOff, drawnH := applyBarMotion(timing, g.Battle.TimingFlash, barH)
	drawX := x + xOff
	drawY := y + yOff

	drawTimingHeading(assets.hudFont, heading, drawX, barW, drawY, baseCol, flashing, qualityColor(timing.Quality, false))

	// Track — dark base fill.
	drawTimingTrack(drawX, drawY, barW, drawnH, timing.Quality, false, flashing, g.Battle.TimingFlash)

	if !flashing {
		// Bar layout below works in visual-fraction space directly, since
		// tick lines and peak band sit at constant bar positions
		// (ChargeTickNPct / ChargePeakStart / ChargePeakEnd). The cursor
		// is the only thing that moves non-linearly across them.

		// Decay zone (dim warning) — drawn first so the peak overlays it.
		// Uses the Nice grade's attack tone (a muted brick) at reduced
		// alpha so a palette retune of the grades carries the bar along.
		decayCol := colorWithAlpha(qualityColor(core.TimingQualityNice, false), 220)
		drawBarSlice(drawX, drawY, barW, drawnH, core.ChargePeakEnd, 1.0, decayCol)

		// Peak window (release zone) — bright Excellent color.
		peakCol := qualityColor(core.TimingQualityExcellent, false)
		drawBarSlice(drawX, drawY, barW, drawnH, core.ChargePeakStart, core.ChargePeakEnd, peakCol)

		// Charging fill — snaps forward only when the cursor crosses a tick
		// boundary, since the *grade* counts only fully-completed ticks.
		// A continuous fill would mislead the player into thinking partial
		// progress between ticks scored partial credit. The cursor still
		// glides smoothly through the bar as a separate visual.
		if timing.Pressed {
			fillEnd := chargeFillEnd(timing)
			if fillEnd > 0 {
				// Good grade's attack tone at reduced alpha — same source
				// as the grade swatches so the bar never drifts from them.
				chargeCol := colorWithAlpha(qualityColor(core.TimingQualityGood, false), 220)
				drawBarSlice(drawX, drawY, barW, drawnH, 0, fillEnd, chargeCol)
			}
		}

		// Tick markers — vertical separators between the three charge segments.
		// Each tick gets a freshness flash for ~220ms after the cursor crosses
		// it, signalling "you just earned a grade tier" without changing the
		// underlying line drawing.
		drawChargeTickWithFlash(timing, drawX, drawY, barW, drawnH, core.ChargeTick1Pct)
		drawChargeTickWithFlash(timing, drawX, drawY, barW, drawnH, core.ChargeTick2Pct)
		drawChargeTickWithFlash(timing, drawX, drawY, barW, drawnH, core.ChargeTick3Pct)
	}

	// Cursor — slides with Elapsed, brightens when held.
	curPct := timing.Progress()
	curX := drawX + curPct*barW
	cursorW := float32(8)
	cursorCol := colorWithAlpha(timingCursorColor, 220)
	if timing.Pressed && !timing.Resolved {
		// Held: punchy cursor with a small halo so the engaged state reads.
		cursorW = 10
		cursorCol = timingHeldColor
		halo := cursorCol
		halo.A = 90
		rl.DrawRectangle(int32(curX-cursorW), int32(drawY)-6, int32(cursorW*2), int32(drawnH)+12, halo)
	}
	if flashing {
		cursorW, cursorCol = applyTimingFlashCursor(curX, drawY, drawnH, g.Battle.TimingFlash, qualityColor(timing.Quality, false))
	}
	rl.DrawRectangle(int32(curX-cursorW/2), int32(drawY)-6, int32(cursorW), int32(drawnH)+12, cursorCol)

	// Excellent shockwave on release — same treatment as the press bar so
	// charge-graded Excellents read with the same flourish.
	if flashing && timing.Quality == core.TimingQualityExcellent {
		drawExcellentShockwave(curX, drawY, drawnH, g.Battle.TimingFlash, false)
	}
}

// drawChargeTickWithFlash paints a tick marker plus a freshness overlay so
// the line briefly glows brighter and a touch wider in the ~220ms after
// the cursor crosses it. Drives off tickFreshness — no extra state needed.
func drawChargeTickWithFlash(timing core.TimingState, barX, barY, barW, barH float32, pct float32) {
	drawChargeTick(timing, barX, barY, barW, barH, pct)
	fresh := tickFreshness(timing, pct)
	if fresh <= 0 {
		return
	}
	tx := barX + pct*barW
	col := qualityColor(core.TimingQualityExcellent, false)
	col.A = uint8(220 * fresh)
	width := 2 + fresh*4
	rl.DrawRectangle(int32(tx-width/2), int32(barY)-5, int32(width), int32(barH)+10, col)
}

// drawSequenceBar paints the pickpocket prompt: a row of N arrows the player
// must tap in order before the timer drains. No backing track — the arrows
// float over the 3D scene with drop shadows so they're readable against any
// background. A thin line under the arrows dwindles left-to-right to
// communicate the timer.
func drawSequenceBar(timing core.TimingState, g core.GameState, assets Resources, x, y, barW, barH float32, flashing bool) {
	heading := timingHeadingPickpocket
	baseCol := colorWithAlpha(seqOkColor, 240) // thief green

	xOff, _, _ := applyBarMotion(timing, g.Battle.TimingFlash, barH)
	drawX := x + xOff

	drawTimingHeading(assets.hudFont, heading, drawX, barW, y, baseCol, flashing, qualityColor(timing.Quality, false))

	count := len(timing.SequenceTargets)
	pad, slotWidth, arrowSize, ok := arrowRowLayout(barW, barH, count)
	if !ok {
		return
	}

	for i, dir := range timing.SequenceTargets {
		cx := drawX + pad + slotWidth*(float32(i)+0.5)
		cy := y + barH*0.5
		state := timing.SequenceResults[i]

		var col rl.Color
		switch state {
		case core.SeqResultCorrect:
			col = seqOkColor // green
		case core.SeqResultWrong:
			col = seqFailColor // red
		default:
			col = colorWithAlpha(timingCursorColor, barHighlightAlpha) // pending bright white
		}
		col = fadeForFlash(col, flashing, g.Battle.TimingFlash)

		// Per-slot pulse: the just-landed correct slot scales up briefly so
		// each tap reads as a discrete win. Drives off SequencePulseTimer
		// set by the battle update when SequenceInput resolves Correct.
		slotSize := arrowSize
		if g.Battle.SequencePulseIndex == i && g.Battle.SequencePulseTimer > 0 {
			phase := g.Battle.SequencePulseTimer / core.SequencePulseDuration
			if phase > 1 {
				phase = 1
			}
			slotSize = arrowSize * (1 + 0.55*phase*phase)
		}

		// Drop shadow: same triangle drawn 3 px down-right in transparent
		// black so the arrow stays readable over busy 3D backgrounds.
		shadow := fadeForFlash(rl.NewColor(0, 0, 0, iconShadowAlpha), flashing, g.Battle.TimingFlash)
		drawArrow(cx+3, cy+3, slotSize, dir, shadow)
		drawArrow(cx, cy, slotSize, dir, col)

		if !flashing && i == timing.SequenceCursor {
			drawSequenceCursorUnderline(cx, cy, arrowSize)
		}
	}

	if !flashing && timing.Duration > 0 {
		drawDwindlingTimerStrip(drawX, y, barW, barH, 1.0-timing.Progress())
	}
}

// reelSymbolColors are the slot symbols' fill hues — well-separated colors so
// a "match" reads at a glance. Indexed by symbol modulo its length, so it
// stays safe if core.ReelSymbolCount ever changes (symbols would just share
// hues rather than index out of range).
var reelSymbolColors = []rl.Color{
	mute(rl.NewColor(255, 206, 84, 255)),  // gold
	mute(rl.NewColor(96, 208, 255, 255)),  // cyan
	mute(rl.NewColor(236, 120, 200, 255)), // magenta
	mute(rl.NewColor(140, 232, 168, 255)), // green
}

// drawReelBar paints Steal's slot-machine gamble: one framed cell per reel,
// each showing its current symbol. A spinning reel's symbol is dimmed; a
// stopped reel gilds its frame and shows the locked symbol solid. On the
// resolve flash the symbols fade with the grade tint. Mirrors the other bars'
// heading + bar-motion handling.
func drawReelBar(timing core.TimingState, g core.GameState, assets Resources, x, y, barW, barH float32, flashing bool) {
	xOff, _, _ := applyBarMotion(timing, g.Battle.TimingFlash, barH)
	drawX := x + xOff
	drawTimingHeading(assets.hudFont, timingHeadingReels, drawX, barW, y, timingHeadingReelsColor, flashing, qualityColor(timing.Quality, false))

	n := len(timing.Reels)
	if n == 0 {
		return
	}
	available := barW - barRowPadPx*2 - barCellGapPx*float32(n-1)
	if available <= 0 {
		return
	}
	cellW := available / float32(n)
	cy := y + barH*0.5
	for i := 0; i < n; i++ {
		cellX := drawX + barRowPadPx + (cellW+barCellGapPx)*float32(i)
		stopped := timing.Reels[i].Stop >= 0

		rl.DrawRectangle(int32(cellX), int32(y), int32(cellW), int32(barH), timingTrackColor)
		frame := colorWithAlpha(timingCursorColor, 110) // dim frame while spinning
		if stopped {
			frame = colorWithAlpha(timingHeldColor, barHighlightAlpha)
		}
		rl.DrawRectangleLinesEx(rl.NewRectangle(cellX, y, cellW, barH), 2, frame)

		sym := timing.ReelSymbolAt(i)
		col := reelSymbolColors[sym%len(reelSymbolColors)]
		if !stopped {
			col = colorWithAlpha(col, 140) // dim while spinning
		}
		col = fadeForFlash(col, flashing, g.Battle.TimingFlash)
		// Dark rim behind the symbol so it reads crisply (etched) over the
		// translucent glass cell — same "definition over glass" logic as the
		// mandatory text drop-shadow (UI_STANDARDS.md). Matters more now that
		// the symbol fills are muted toward parchment tones.
		sx := cellX + cellW*0.5
		r := barH * 0.30
		rl.DrawCircleV(rl.NewVector2(sx, cy), r+2, fadeForFlash(rl.NewColor(0, 0, 0, reelRimAlpha), flashing, g.Battle.TimingFlash))
		rl.DrawCircleV(rl.NewVector2(sx, cy), r, col)
	}
}

// drawRecallBar paints Arc Bolt's memory minigame. During the reveal phase it
// shows the full directional pattern lit ("MEMORIZE!"); once the pattern
// hides ("RECALL!") the arrows go face-down — pending slots are dim dots,
// landed slots reveal their answer tinted green (correct) or red (wrong) so
// the player gets feedback as they go. Reuses the sequence bar's arrow icons,
// cursor underline, and dwindling timer strip.
func drawRecallBar(timing core.TimingState, g core.GameState, assets Resources, x, y, barW, barH float32, flashing bool) {
	hidden := timing.RecallHidden()
	heading := timingHeadingRecallMemo
	baseCol := timingHeadingRecallMemoColor
	if hidden {
		heading = timingHeadingRecallRecall
		baseCol = timingHeadingRecallRecallColor
	}
	xOff, _, _ := applyBarMotion(timing, g.Battle.TimingFlash, barH)
	drawX := x + xOff
	drawTimingHeading(assets.hudFont, heading, drawX, barW, y, baseCol, flashing, qualityColor(timing.Quality, false))

	count := len(timing.SequenceTargets)
	pad, slotWidth, arrowSize, ok := arrowRowLayout(barW, barH, count)
	if !ok {
		return
	}
	for i := 0; i < count; i++ {
		cx := drawX + pad + slotWidth*(float32(i)+0.5)
		cy := y + barH*0.5
		if !hidden {
			// Memorize phase: show every pattern arrow lit, with a drop shadow
			// (matches the sequence bar) so it reads over busy 3D backgrounds.
			drawArrow(cx+3, cy+3, arrowSize, timing.SequenceTargets[i], rl.NewColor(0, 0, 0, iconShadowAlpha))
			drawArrow(cx, cy, arrowSize, timing.SequenceTargets[i], colorWithAlpha(timingCursorColor, barHighlightAlpha))
			continue
		}
		// Recall phase: arrows hidden. Landed slots reveal their answer tinted
		// by correctness; pending slots stay a dim face-down dot.
		switch timing.SequenceResults[i] {
		case core.SeqResultCorrect:
			drawArrow(cx, cy, arrowSize, timing.SequenceTargets[i], fadeForFlash(seqOkColor, flashing, g.Battle.TimingFlash))
		case core.SeqResultWrong:
			drawArrow(cx, cy, arrowSize, timing.SequenceTargets[i], fadeForFlash(seqFailColor, flashing, g.Battle.TimingFlash))
		default:
			rl.DrawCircleV(rl.NewVector2(cx, cy), arrowSize*0.5, colorWithAlpha(timingCursorColor, 90))
		}

		if !flashing && i == timing.SequenceCursor {
			drawSequenceCursorUnderline(cx, cy, arrowSize)
		}
	}

	if !flashing && timing.Duration > 0 {
		drawDwindlingTimerStrip(drawX, y, barW, barH, 1.0-timing.Progress())
	}
}

// drawArrow paints a chunky arrow icon (pointed head + thick stem) pointing
// in the given SeqDir direction, centered at (cx, cy). `size` is the arrow's
// half-extent along its axis, so the full icon spans 2*size.
//
// raylib's rl.DrawTriangle is sensitive to winding: with the 2D pipeline's
// GL_CULL_FACE enabled (which it is on at least some drivers), back-faces
// silently render as nothing. The first take here had the head and both
// stem triangles wound CW in screen-Y-down space and tried to compensate
// with rl.DisableBackfaceCulling — which, on the user's machine, doesn't
// take effect in time for the immediate-mode triangles. Result: blank
// arrows during the pickpocket prompt. The fix is to wind every triangle
// CCW in screen-Y-down (matches the working minimap arrow in minimap.go)
// so visibility doesn't depend on the cull state at all.
func drawArrow(cx, cy, size float32, dir int, col rl.Color) {
	// axis points along the arrow's tip; perp is 90° clockwise from it.
	axisX, axisY := arrowAxisVec(dir)
	perpX, perpY := -axisY, axisX

	tipX := cx + axisX*size
	tipY := cy + axisY*size

	headBaseX := cx + axisX*size*0.05
	headBaseY := cy + axisY*size*0.05

	headLX := headBaseX - perpX*size*0.85
	headLY := headBaseY - perpY*size*0.85
	headRX := headBaseX + perpX*size*0.85
	headRY := headBaseY + perpY*size*0.85

	stemTLX := headBaseX - perpX*size*0.40
	stemTLY := headBaseY - perpY*size*0.40
	stemTRX := headBaseX + perpX*size*0.40
	stemTRY := headBaseY + perpY*size*0.40
	stemBLX := cx - axisX*size*0.85 - perpX*size*0.40
	stemBLY := cy - axisY*size*0.85 - perpY*size*0.40
	stemBRX := cx - axisX*size*0.85 + perpX*size*0.40
	stemBRY := cy - axisY*size*0.85 + perpY*size*0.40

	// Head: tip → L → R is CCW in screen-Y-down (matches drawMinimapArrow).
	drawTriangleCCW(
		rl.NewVector2(tipX, tipY),
		rl.NewVector2(headLX, headLY),
		rl.NewVector2(headRX, headRY),
		col,
	)
	// Stem rectangle as two CCW triangles in screen-Y-down. Visualizing
	// the stem as TL/TR/BL/BR corners on a clock face (TL=11, TR=1,
	// BR=5, BL=7), the CCW orderings are TL→BL→BR and TL→BR→TR.
	drawTriangleCCW(
		rl.NewVector2(stemTLX, stemTLY),
		rl.NewVector2(stemBLX, stemBLY),
		rl.NewVector2(stemBRX, stemBRY),
		col,
	)
	drawTriangleCCW(
		rl.NewVector2(stemTLX, stemTLY),
		rl.NewVector2(stemBRX, stemBRY),
		rl.NewVector2(stemTRX, stemTRY),
		col,
	)
}

// arrowAxisVec returns the unit "tip-direction" vector for a SeqDir, in
// screen-Y-down coordinates. Up = (0,-1), Right = (1,0), Down = (0,1),
// Left = (-1,0).
func arrowAxisVec(dir int) (float32, float32) {
	switch dir {
	case core.SeqDirUp:
		return 0, -1
	case core.SeqDirRight:
		return 1, 0
	case core.SeqDirDown:
		return 0, 1
	case core.SeqDirLeft:
		return -1, 0
	}
	return 0, -1
}

// drawBarSlice paints a stripe across the bar between two normalized
// fractions. Clamps to [0,1] and skips zero-or-negative widths so callers
// don't have to repeat the bookkeeping.
func drawBarSlice(barX, barY, barW, barH, startPct, endPct float32, col rl.Color) {
	if startPct < 0 {
		startPct = 0
	}
	if endPct > 1 {
		endPct = 1
	}
	if endPct <= startPct {
		return
	}
	zx := barX + startPct*barW
	zw := (endPct - startPct) * barW
	rl.DrawRectangle(int32(zx), int32(barY), int32(zw), int32(barH), col)
}

// chargeFillEnd returns how far (as a visual bar fraction in [0, 1]) the
// orange charging fill should extend, snapped to the last fully-passed
// tick. This matches resolveCharge's discrete grading: a release between
// tick N and tick N+1 scores N ticks, so the visual should also read as
// "N ticks filled" rather than telegraphing partial progress. Pulls
// cursor visual progress so the fill snap fires the same instant the
// cursor crosses the tick line on screen.
func chargeFillEnd(timing core.TimingState) float32 {
	p := core.ChargeCursorProgress(timing.Elapsed, timing.Duration)
	switch {
	case p >= core.ChargeTick3Pct:
		return core.ChargeTick3Pct
	case p >= core.ChargeTick2Pct:
		return core.ChargeTick2Pct
	case p >= core.ChargeTick1Pct:
		return core.ChargeTick1Pct
	default:
		return 0
	}
}

// drawChargeTick paints a thin vertical separator line at the given fraction
// of the bar — the visible boundary between charge segments.
func drawChargeTick(timing core.TimingState, barX, barY, barW, barH float32, pct float32) {
	tx := barX + pct*barW
	tickCol := timingTickColor
	rl.DrawRectangle(int32(tx-1), int32(barY)-3, 2, int32(barH)+6, tickCol)
}

// drawWindowZone paints a solid color stripe centered on `sweet`, scaled
// to a fraction of the acceptance window's (`end` - `start`) width. Used
// to nest quality bands without drawing any borders. Takes the window
// scalars explicitly so callers can paint either the primary or the
// secondary window of a two-zone press bar through the same helper.
func drawWindowZone(start, end, sweet, duration, barX, barY, barW, barH, ratio float32, col rl.Color) {
	windowSize := end - start
	if windowSize <= 0 || duration <= 0 {
		return
	}
	half := windowSize * ratio * 0.5
	startSec := sweet - half
	endSec := sweet + half
	if startSec < start {
		startSec = start
	}
	if endSec > end {
		endSec = end
	}
	drawBarSlice(barX, barY, barW, barH, startSec/duration, endSec/duration, col)
}

// drawPressWindowZones paints the full Nice → Good → Great → Excellent
// nested-band stack for one acceptance window of a press bar. Called once
// per window so single-zone and double-zone (Swipe) press bars share the
// same gradient look without the caller having to repeat four lines per
// window.
func drawPressWindowZones(start, end, sweet, duration, barX, barY, barW, barH float32, isDefend bool) {
	drawWindowZone(start, end, sweet, duration, barX, barY, barW, barH, 1.00, qualityColor(core.TimingQualityNice, isDefend))
	drawWindowZone(start, end, sweet, duration, barX, barY, barW, barH, 0.60, qualityColor(core.TimingQualityGood, isDefend))
	drawWindowZone(start, end, sweet, duration, barX, barY, barW, barH, 0.30, qualityColor(core.TimingQualityGreat, isDefend))
	drawWindowZone(start, end, sweet, duration, barX, barY, barW, barH, 0.10, qualityColor(core.TimingQualityExcellent, isDefend))
}

// drawTallyBar paints the multi-press tally layout: one flat hit-zone
// stripe per accept window (filled solid for unhit, dimmed checker
// for already-consumed) + a late "COMMIT" zone painted in orange at
// the bar's tail. The number of stripes communicates "how many hits
// are possible"; the dimming communicates "you've already got this
// one." A small tally counter overlay would be nice on top but is
// left to the caller's HUD layer — drawing it inside the world's
// 3D pass would require re-projecting screen coords.
// tallyConsumedAttack / tallyConsumedDefend are the dimmed "this
// window's already been hit" tints used by the multi-press tally bar.
// Derived once from the attack / defend "Good"-grade colors at init
// rather than rebuilt every frame inside drawTallyBar — the inputs
// (qualityVisuals row colors) are constants.
var (
	tallyConsumedAttack = makeTallyConsumedColor(false)
	tallyConsumedDefend = makeTallyConsumedColor(true)
)

func makeTallyConsumedColor(isDefend bool) rl.Color {
	c := qualityColor(core.TimingQualityGood, isDefend)
	return rl.NewColor(c.R/3, c.G/3, c.B/3, 200)
}

func drawTallyBar(t core.TimingState, barX, barY, barW, barH float32, isDefend bool) {
	if t.Duration <= 0 {
		return
	}
	hitCol := qualityColor(core.TimingQualityGood, isDefend)
	excellentCol := qualityColor(core.TimingQualityExcellent, isDefend)
	consumedCol := tallyConsumedAttack
	if isDefend {
		consumedCol = tallyConsumedDefend
	}
	cursorElapsed := t.Elapsed
	// Indexed access (not range-by-value) so a future per-frame
	// render-state addition on TallyWindow (e.g., a screen-space
	// pulse anchor) can be mutated through &t.Windows[i] without
	// a copy-and-write-back dance. Today the loop is read-only;
	// the indexed form documents the intent.
	for i := range t.Windows {
		w := &t.Windows[i]
		startX := barX + (w.Start/t.Duration)*barW
		width := ((w.End - w.Start) / t.Duration) * barW
		// Pick the window's base color based on state:
		// - Just-hit (FlashTimer > 0): bright "Excellent" tint that
		//   fades back to consumed as the timer drains. This is the
		//   per-press feedback flash the player wanted.
		// - Already hit (no flash): dim consumed color.
		// - Unhit + cursor inside: pulse alpha-bright so the player
		//   sees "press NOW" while the cursor is in-zone.
		// - Unhit + cursor outside: resting hit color.
		var col rl.Color
		switch {
		case w.Hit && w.FlashTimer > 0:
			flashT := w.FlashTimer / core.TallyHitFlashDuration
			if flashT > 1 {
				flashT = 1
			}
			col = core.MixColor(consumedCol, excellentCol, float64(flashT))
		case w.Hit:
			col = consumedCol
		case cursorElapsed >= w.Start && cursorElapsed <= w.End:
			// Live-preview throb — the cursor is currently inside
			// this window; pulse the alpha so the player reads
			// "tap to score." pulse() returns 0..1 sinusoidally.
			throb := 0.75 + 0.25*pulse(2.4)
			col = fadeColor(blendTowardWhite(hitCol, 0.45), throb)
		default:
			col = hitCol
		}
		rl.DrawRectangle(int32(startX), int32(barY), int32(width), int32(barH), col)
		// Sweet-spot pip — a brighter notch in the centre of each
		// unhit window so the eye reads "tap the bright dot."
		if !w.Hit {
			pipX := barX + (w.Sweet/t.Duration)*barW
			pipW := width * 0.18
			pip := blendTowardWhite(hitCol, 0.60)
			rl.DrawRectangle(int32(pipX-pipW*0.5), int32(barY), int32(pipW), int32(barH), pip)
		}
	}
	// Commit zone — orange tail. Pressing here ends the bar with
	// whatever tally you've got. Visible from CommitStart through
	// the end of the duration so the player sees their "exit" gate
	// approaching as the bar nears its end.
	if t.CommitStart < t.Duration {
		commitX := barX + (t.CommitStart/t.Duration)*barW
		commitW := barX + barW - commitX
		commitCol := timingCommitColor
		// If the cursor's inside the commit zone, throb it too —
		// matches the unhit-window preview so the player sees the
		// "exit gate is live" without reading a separate visual
		// vocabulary.
		if cursorElapsed >= t.CommitStart {
			throb := 0.78 + 0.22*pulse(2.6)
			commitCol = fadeColor(commitCol, throb)
		}
		rl.DrawRectangle(int32(commitX), int32(barY), int32(commitW), int32(barH), commitCol)
	}
}

// (lerpRGBA removed — callers route through core.MixColor with a
// float64(t) cast. rl.Color is a type alias of color.RGBA so the
// signatures unify cleanly.)

// flashAlpha returns the [0,1] strength of the flash, peaking right after the
// press and decaying as the hold timer counts down. Squared falloff so it
// stays bright at first and fades fast at the end.
func flashAlpha(remaining float32) float32 {
	if core.TimingFlashDuration <= 0 {
		return 0
	}
	t := remaining / core.TimingFlashDuration
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return t * t
}

// popupOffScreenX reports whether a world-anchored popup's projected screen X
// has drifted far enough past the viewport edges (beyond offscreenPopupSlack)
// that it should be culled. Shared by the quality + damage popup draws.
func popupOffScreenX(screenX, screenW float32) bool {
	return screenX < -offscreenPopupSlack || screenX > screenW+offscreenPopupSlack
}

// DrawQualityPopup floats the most recent quality result above the actor for
// QualityResultDuration after each timing resolution. Punches in with a quick
// scale-up for impact, then fades up and out.
func DrawQualityPopup(camera rl.Camera3D, g core.GameState, assets Resources) {
	if g.Battle.LastQualityTimer <= 0 {
		return
	}
	if !g.Battle.Active() {
		return
	}

	t := g.Battle.LastQualityTimer / core.QualityResultDuration
	scale, rise, alpha := popupAnimation(t)

	worldPos, ok := qualityPopupAnchor(camera, g)
	if !ok {
		return
	}
	worldPos.Y += 0.6
	screenPos := rl.GetWorldToScreen(worldPos, camera)
	sw, _ := screenSizeF()
	if popupOffScreenX(screenPos.X, sw) {
		return
	}

	label := core.TimingQualityLabel(g.Battle.LastQuality)
	if g.Battle.LastQualityIsBlock && g.Battle.LastQuality > core.TimingQualityMiss {
		label = "BLOCK!"
	}
	col := qualityColor(g.Battle.LastQuality, g.Battle.LastQualityIsBlock)
	col.A = alpha

	// Quality popup uses FontTitle; an Excellent landing keeps the
	// same size but the post-scale `scale` factor (driven by the
	// throb curve) gives Excellent extra visual punch via geometry
	// rather than a hand-baked larger size literal.
	baseSize := FontTitle
	size := baseSize * scale
	measure := rl.MeasureTextEx(assets.hudFont, label, size, 1.5)
	x := screenPos.X - measure.X/2
	y := screenPos.Y - measure.Y - rise

	shadow := rl.NewColor(0, 0, 0, alpha)
	drawTextWithShadowStyle(assets.hudFont, label, x, y, size, 1.5, col, shadow, 3, 3)
}

// qualityPopupAnchor returns the 3D world position the floating quality text
// should hover above — the actor for attacks, the defender for blocks. Both
// cases anchor to a party-member sprite, since enemies don't author quality
// popups (only the damage-number popups, which use a separate path).
func qualityPopupAnchor(camera rl.Camera3D, g core.GameState) (rl.Vector3, bool) {
	idx := g.Battle.LastQualityIndex
	if idx < 0 || idx >= len(g.Party) {
		return rl.Vector3{}, false
	}
	pos := partySpritePosition(camera, idx, g.Party[idx].Class, 0, 0, 0)
	return rl.NewVector3(pos.X, pos.Y, pos.Z), true
}

// DrawDamagePopups floats the most recent damage number above each hit enemy
// for QualityResultDuration. Color matches the timing quality (so a Great
// reads as the same orange as the player's quality popup); an Excellent hit
// gets a trailing "!" so the big damage spike reads at a glance.
func DrawDamagePopups(camera rl.Camera3D, g core.GameState, assets Resources) {
	if !g.Battle.Active() {
		return
	}
	members := core.BattleMembers(&g)
	for i := range members {
		enemy := &members[i] // index-range + pointer: no ~496-byte Enemy copy per member
		if enemy.DamagePopupTimer <= 0 {
			continue
		}
		// Note: we deliberately don't gate on Alive/DeathFade here. Popup
		// duration (QualityResultDuration ~0.7s) outlasts DeathFade (~0.55s),
		// so for the killing blow we want the number to keep floating after
		// the body fades. enemyDrawPosition still returns a valid spot since
		// the member's slot is stable for the lifetime of the active pack.
		pos := enemyDrawPosition(camera, &g, i, enemy)
		pos.Y += 0.6
		screenPos := rl.GetWorldToScreen(pos, camera)
		sw, _ := screenSizeF()
		if popupOffScreenX(screenPos.X, sw) {
			continue
		}

		t := enemy.DamagePopupTimer / core.QualityResultDuration
		scale, rise, alpha := popupAnimation(t)

		label := damagePopupLabel(enemy.DamagePopup, enemy.DamagePopupQuality)
		col := qualityColor(enemy.DamagePopupQuality, false)
		col.A = alpha

		// Damage popup uses FontHeading at the base size; Excellent
		// gets a stronger throb via the `scale` factor rather than
		// a separate larger size literal (so all popup text stays
		// on the standardized size scale).
		baseSize := FontHeading
		size := baseSize * scale
		measure := rl.MeasureTextEx(assets.hudFont, label, size, 1.2)
		x := screenPos.X - measure.X/2
		y := screenPos.Y - measure.Y - rise

		shadow := rl.NewColor(0, 0, 0, alpha)
		drawTextWithShadowStyle(assets.hudFont, label, x, y, size, 1.2, col, shadow, 2, 2)
	}
}

// damagePopupLabel formats the damage value, appending "!" on an
// Excellent. The single-digit and small two-digit cases dominate
// normal play; pre-format 0..199 × {plain, excellent} so the per-
// frame popup paint is a slice index rather than a fmt.Sprintf alloc.
// Anything past the cache window falls back to strconv concat —
// still cheaper than fmt.Sprintf.
func damagePopupLabel(damage, quality int) string {
	if damage >= 0 && damage < len(damagePopupLabelCache) {
		if quality == core.TimingQualityExcellent {
			return damagePopupLabelCache[damage].excellent
		}
		return damagePopupLabelCache[damage].plain
	}
	if quality == core.TimingQualityExcellent {
		return strconv.Itoa(damage) + "!"
	}
	return strconv.Itoa(damage)
}

var damagePopupLabelCache = func() [200]struct{ plain, excellent string } {
	var out [200]struct{ plain, excellent string }
	for i := range out {
		out[i].plain = strconv.Itoa(i)
		out[i].excellent = out[i].plain + "!"
	}
	return out
}()

// popupAnimation returns the scale/rise/alpha for a popup whose remaining
// life ratio is t (1.0 = just spawned, 0.0 = expired). Punches in with a
// 0.6→2.15→1.0 scale curve, rises 36 px, fades alpha via Smoothstep.
func popupAnimation(t float32) (scale, rise float32, alpha uint8) {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	age := 1 - t
	scale = 1
	switch {
	case age < 0.12:
		scale = 0.6 + 1.55*(age/0.12)
		if scale > 2.15 {
			scale = 2.15
		}
	case age < 0.22:
		scale = 2.15 - 1.15*((age-0.12)/0.10)
	}
	rise = (1 - t) * 36
	alpha = uint8(255 * core.Smoothstep(t))
	return
}

// qualityColor returns the bar/popup tint for a quality grade. Defend mode
// shifts the palette toward cool blues so attack vs block reads at a glance.
// Reads from qualityVisuals; out-of-range qualities fall through to the
// Miss color (shared between attack and defend rows).
func qualityColor(quality int, isDefend bool) rl.Color {
	if quality < 0 || quality >= len(qualityVisuals) {
		return qualityVisuals[core.TimingQualityMiss].AttackColor
	}
	if isDefend {
		return qualityVisuals[quality].DefendColor
	}
	return qualityVisuals[quality].AttackColor
}
