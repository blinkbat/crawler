package render

import (
	"image/color"
	"math"
	"strconv"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// noPromptGlyph marks a timing heading whose minigame shows directional arrows in
// the bar itself (the button is self-evident) — so no single-button glyph is
// appended. Distinct from hitglyph.go's glyphNone (a different type).
const noPromptGlyph = InputGlyph(-1)

// timingHeadingStyle pairs a bar heading's text with its base-fill tint and the
// button glyph the minigame wants, so a dispatch site can't mismatch them. The
// tints live in theme.go's palette block.
type timingHeadingStyle struct {
	text  string
	color rl.Color
	glyph InputGlyph // button to press; noPromptGlyph when the bar shows arrows
}

// Per-bar headings (the press bar flips Strike/Defend by phase; recall flips Memo/
// Recall). glyph: Strike/Charge/Reels = Confirm (A), Defend = Back (B); the arrow
// minigames (Memorize/Recall, and the Combo sequence below) carry no button glyph.
var (
	headStrike       = timingHeadingStyle{"STRIKE!", timingHeadingStrikeColor, GlyphA}
	headDefend       = timingHeadingStyle{"DEFEND!", timingHeadingDefendColor, GlyphB}
	headCharge       = timingHeadingStyle{"CHARGE!", timingHeadingChargeColor, GlyphA}
	headReels        = timingHeadingStyle{"STOP THE REELS!", timingHeadingReelsColor, GlyphA}
	headRecallMemo   = timingHeadingStyle{"MEMORIZE!", timingHeadingRecallMemoColor, noPromptGlyph}
	headRecallRecall = timingHeadingStyle{"RECALL!", timingHeadingRecallRecallColor, noPromptGlyph}
)

// timingHeadingCombo's tint is the thief-green sequence color (seqOkColor), not a
// paired palette entry, so it stays a bare string.
const timingHeadingCombo = "COMBO!"

// Shared timing-bar layout + alpha tunables, read by all bars so they can't drift.
const (
	barRowPadPx           = float32(18)   // horizontal padding inside a bar row
	barCellGapPx          = float32(12)   // gap between reel cells
	arrowSizeSlotFrac     = float32(0.35) // arrow half-extent as a fraction of slot width
	arrowSizeBarHCap      = float32(0.85) // arrow size ceiling as a fraction of bar height
	timerStripUrgentFrac  = float32(0.30) // remaining-time fraction below which the strip reads red
	timerStripWarnFrac    = float32(0.55) // ...below which it reads warn-amber
	barHighlightAlpha     = uint8(245)    // fully-lit element (pending arrow, locked reel frame, cursor underline)
	iconShadowAlpha       = uint8(180)    // arrow drop-shadow alpha
	reelRimAlpha          = uint8(150)    // dark rim behind a reel symbol (definition over glass)
	flashHaloAlpha        = uint8(180)    // frozen-cursor halo peak alpha during the flash hold
	timingStrongFillAlpha = uint8(220)    // strong fill alpha (track flood, shockwave ring, charge/decay slices, tick flash)
)

// Shared timing-bar footprint (timingBarLayout): height, min width, side edge
// margin, and gap above the party ribbon. One place so the modes share a footprint.
const (
	timingBarHeight     = float32(34)
	timingBarMinW       = float32(380)
	timingBarEdgeMargin = float32(32) // clamp so the bar never reaches the screen edges
	timingBarRibbonGap  = float32(28) // gap between the bar's bottom and the party ribbon top
)

// fadeForFlash scales col's alpha by the flash-hold envelope when flashing, else unchanged.
func fadeForFlash(col rl.Color, flashing bool, flashTimer float32) rl.Color {
	if flashing {
		col.A = uint8(float32(col.A) * flashAlpha(flashTimer))
	}
	return col
}

// arrowRowLayout returns the shared evenly-spaced arrow-row geometry (sequence + recall).
// ok=false when too narrow/empty. Per-slot center is drawX + pad + slotWidth*(i+0.5).
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

// drawArrowWithShadow stamps an arrow plus its 3px down-right drop shadow so it stays
// readable over busy backgrounds (mirrors drawTextWithShadow). Sequence + recall bars.
func drawArrowWithShadow(cx, cy, size float32, dir int, col, shadowCol rl.Color) {
	drawArrow(cx+3, cy+3, size, dir, shadowCol)
	drawArrow(cx, cy, size, dir, col)
}

// seqResultColor maps a sequence-slot result to its arrow tint: green correct, red
// wrong, bright-white pending. Shared by the sequence + recall (revealed) slots.
func seqResultColor(state int) rl.Color {
	switch state {
	case core.SeqResultCorrect:
		return seqOkColor // green
	case core.SeqResultWrong:
		return seqFailColor // red
	default:
		return colorWithAlpha(timingCursorColor, barHighlightAlpha) // pending bright white
	}
}

// drawSeqArrowSlot renders one sequence-bar arrow slot at (cx,cy): result-tinted
// arrow (with flash fade + the just-landed pulse) under a drop shadow, plus the
// "next slot" cursor underline. Extracted so the sequence + recall bars share the
// per-slot geometry/coloring instead of duplicating the loop body.
func drawSeqArrowSlot(g *core.GameState, cx, cy, arrowSize float32, dir int, i int, state int, isCursor, flashing bool) {
	col := fadeForFlash(seqResultColor(state), flashing, g.Battle.TimingFlash)

	// Per-slot pulse: the just-landed correct slot scales up briefly (SequencePulseTimer).
	slotSize := arrowSize
	if g.Battle.SequencePulseIndex == i && g.Battle.SequencePulseTimer > 0 {
		phase := clampBarPct(g.Battle.SequencePulseTimer / core.SequencePulseDuration)
		slotSize = arrowSize * (1 + 0.55*phase*phase)
	}

	shadow := fadeForFlash(colorWithAlpha(shadowBase, iconShadowAlpha), flashing, g.Battle.TimingFlash)
	drawArrowWithShadow(cx, cy, slotSize, dir, col, shadow)

	if !flashing && isCursor {
		drawSequenceCursorUnderline(cx, cy, arrowSize)
	}
}

// drawShadowedRect paints a drop-shadow rect (shadowLight, offset by off) then the
// bright rect on top — the "shadow then fill" idiom the underline + timer strip share.
func drawShadowedRect(x, y, w, h, off float32, col rl.Color) {
	rl.DrawRectangle(int32(x)+int32(off), int32(y)+int32(off), int32(w), int32(h), shadowLight)
	rl.DrawRectangle(int32(x), int32(y), int32(w), int32(h), col)
}

// drawSequenceCursorUnderline paints the "next slot" underline (sequence + recall bars).
func drawSequenceCursorUnderline(cx, cy, arrowSize float32) {
	ux := cx - arrowSize*0.85
	uw := arrowSize * 1.7
	uy := cy + arrowSize + 8
	drawShadowedRect(ux, uy, uw, 4, 2, colorWithAlpha(timingHeldColor, barHighlightAlpha))
}

// drawDwindlingTimerStrip paints the center-anchored timer line that shrinks toward center
// as remaining (1→0) drains, reddening near zero. Sequence + recall bars; pass 1-Progress().
func drawDwindlingTimerStrip(drawX, y, barW, barH, remaining float32) {
	// Upper clamp guards against a negative Progress() (intro) overrunning the bar.
	remaining = clampBarPct(remaining)
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
	drawShadowedRect(stripX, stripY, visW, stripH, 1, stripCol)
}

// --- Bar juice helpers ----------------------------------------------------
// barShake / barThrob / tickFreshness: feedback curves keyed off the bar's TimingFlash hold.

// blendTowardWhite mixes col toward white by whiteAmount ∈ [0,1], preserving source alpha.
// Keeps grade-tinted elements visible over grade-tinted backgrounds (e.g. cursor on its zone).
func blendTowardWhite(col rl.Color, whiteAmount float32) rl.Color {
	out := core.MixColor(col, rl.White, float64(core.Clamp(whiteAmount, 0, 1)))
	out.A = col.A // preserve source alpha; rl.White would drag it to 255
	return out
}

// Miss-flash bar-shake tunables: peak horizontal offset, and full sine cycles over the flash hold.
const (
	shakeAmplitudePx    = float32(6.5)
	shakeCyclesPerFlash = float32(9)
)

// barShake returns the bar's horizontal offset during a Miss flash (damped sinusoid);
// zero for any other grade.
func barShake(timing core.TimingState, flashTimer float32) float32 {
	if timing.Quality != core.TimingQualityMiss || flashTimer <= 0 {
		return 0
	}
	if core.TimingFlashDuration <= 0 {
		return 0
	}
	age := core.Clamp(1-flashTimer/core.TimingFlashDuration, 0, 1)
	amp := (1 - age) * shakeAmplitudePx
	return float32(math.Sin(float64(age)*math.Pi*float64(shakeCyclesPerFlash))) * amp
}

// qualityVisuals is the per-grade color + throb table, indexed by core.TimingQualityMiss..Excellent.
var qualityVisuals = [core.TimingQualityCount]struct {
	ThrobIntensity float32
	AttackColor    rl.Color
	DefendColor    rl.Color
}{
	core.TimingQualityMiss:      {ThrobIntensity: 0, AttackColor: timingGradeAtkMiss, DefendColor: timingGradeDefMiss},
	core.TimingQualityNice:      {ThrobIntensity: 0.08, AttackColor: timingGradeAtkNice, DefendColor: timingGradeDefNice},
	core.TimingQualityGood:      {ThrobIntensity: 0.12, AttackColor: timingGradeAtkGood, DefendColor: timingGradeDefGood},
	core.TimingQualityGreat:     {ThrobIntensity: 0.16, AttackColor: timingGradeAtkGreat, DefendColor: timingGradeDefGreat},
	core.TimingQualityExcellent: {ThrobIntensity: 0.22, AttackColor: timingGradeAtkExcellent, DefendColor: timingGradeDefExcellent},
}

// init asserts qualityVisuals covers every grade (a zero-alpha slot trips at startup).
func init() {
	for q := 0; q < int(core.TimingQualityCount); q++ {
		if qualityVisuals[q].AttackColor.A == 0 {
			panic("render/timing: qualityVisuals missing a row for a timing grade")
		}
	}
}

// barThrob returns the bar's height-scale multiplier during a graded flash (higher grades
// pulse harder; Miss stays 1.0 and shakes instead). Centerline fixed; height grows around it.
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

// tickFreshness returns [0,1] for how recently the cursor crossed a charge tick (fresh = bright).
// tickPct is the tick's visual position, inverted through the cursor curve to its elapsed time.
func tickFreshness(timing core.TimingState, tickPct float32) float32 {
	const tickFlashDuration = float32(0.22)
	tickTime := core.ChargeElapsedForVisual(tickPct, timing.Duration)
	age := timing.Elapsed - tickTime
	if age < 0 || age > tickFlashDuration {
		return 0
	}
	return 1 - age/tickFlashDuration
}

// applyBarMotion returns the throb/shake (xOffset, yOffset, scaledH); callers add the offsets
// to their draw positions and use scaledH in place of barH.
func applyBarMotion(timing core.TimingState, flashTimer, barH float32) (xOffset, yOffset, scaledH float32) {
	shake := barShake(timing, flashTimer)
	throb := barThrob(timing, flashTimer)
	scaledH = barH * throb
	yOffset = -(scaledH - barH) / 2
	xOffset = shake
	return
}

// drawTimingBar paints the active timed-hit bar above the party ribbon, dispatching by Kind.
func drawTimingBar(g *core.GameState, assets Resources) {
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
		// Overcharge reuses the charge bar; its decay band already reads as the danger zone.
		drawChargeBar(timing, g, assets, x, y, barW, barH, flashing)
	case core.TimingKindSequence:
		drawSequenceBar(timing, g, assets, x, y, barW, barH, flashing)
	case core.TimingKindRecall:
		drawRecallBar(timing, g, assets, x, y, barW, barH, flashing)
	case core.TimingKindReels:
		drawReelBar(timing, g, assets, x, y, barW, barH, flashing)
	case core.TimingKindPress:
		drawPressBar(timing, g, assets, x, y, barW, barH, flashing)
	default:
		// Unknown kind: log loudly and fall back to the Press bar.
		LogRenderError("timing: no bar for TimingKind %d; drew Press fallback", int(timing.Kind))
		drawPressBar(timing, g, assets, x, y, barW, barH, flashing)
	}
}

// timingBarLayout returns the bar's screen-space rect (shared footprint so modes don't jump).
func timingBarLayout() (x, y, barW, barH float32) {
	screenW, _ := screenSizeF()
	barH = timingBarHeight
	barW = screenW * 0.62
	if barW < timingBarMinW {
		barW = timingBarMinW
	}
	if barW > screenW-timingBarEdgeMargin {
		barW = screenW - timingBarEdgeMargin
	}
	x = centerXF(barW)
	y = PartyRibbonTopY() - barH - timingBarRibbonGap
	return
}

// timingHeadingGlyphGap separates the button glyph from the heading word ("[B] DEFEND!").
const timingHeadingGlyphGap = float32(10)

// timingHeadingGlyphScale shrinks the button glyph relative to the heading text so the
// badge reads as a small chip leading the word rather than matching its cap height.
const timingHeadingGlyphScale = float32(0.7)

// drawTimingHeading paints the centered prompt above the bar, shifting to the quality tint
// during the flash hold. When glyph != noPromptGlyph it prepends the button to press
// (e.g. "[A] STRIKE!") so a non-obvious minigame's input reads at a glance.
func drawTimingHeading(font rl.Font, text string, glyph InputGlyph, x, barW, y float32, baseCol rl.Color, flashing bool, flashCol rl.Color) {
	size := FontHeading
	col := baseCol
	if flashing {
		col = flashCol
		size = FontTitle // flash punch, next size up on the locked scale
	}
	measure := measureTimingHeading(font, text, size)
	gap, glyphW, glyphSize := float32(0), float32(0), float32(0)
	if glyph != noPromptGlyph {
		glyphSize = size * timingHeadingGlyphScale
		glyphW = glyphWidth(glyph, glyphSize)
		gap = timingHeadingGlyphGap
	}
	// Glyph leads the word, centered as one group above the bar.
	hx := x + (barW-measure.X-gap-glyphW)/2
	hy := y - measure.Y - 6
	if glyph != noPromptGlyph {
		// Center the smaller glyph on the heading's vertical midline (drawInputGlyph
		// centers on yPassed + size/2, so offset by the half-size difference).
		drawInputGlyph(font, glyph, hx, hy+(size-glyphSize)/2, glyphSize, 1)
	}
	drawEngravedTextSpaced(font, text, hx+glyphW+gap, hy, size, 1.5, col)
}

// timingHeadingMeasureCache memoizes drawTimingHeading's measure (keyed on the size flip).
var timingHeadingMeasureCache measureCache

func measureTimingHeading(font rl.Font, text string, size float32) rl.Vector2 {
	return timingHeadingMeasureCache.measure(font, text, size, 1.5)
}

// applyTimingFlashCursor draws the frozen-cursor halo during the flash hold and returns the
// (width, color) overrides for the caller's cursor draw. Shared by press + charge bars.
func applyTimingFlashCursor(curX, y, barH, flashTimer float32, base rl.Color) (float32, rl.Color) {
	cursorW := timingCursorWidthFlash
	flashCol := base
	flashCol.A = 255
	halo := flashCol
	halo.A = uint8(float32(flashHaloAlpha) * flashAlpha(flashTimer))
	rl.DrawRectangle(int32(curX-cursorW*2), int32(y)-8, int32(cursorW*4), int32(barH)+16, halo)
	return cursorW, flashCol
}

const (
	// timingCursorWidth is the resting cursor-block width (px) on press + charge bars.
	timingCursorWidth = float32(8)
	// timingCursorWidthHeld is the slightly fatter cursor while a charge bar is held.
	timingCursorWidthHeld = float32(10)
	// timingCursorWidthFlash is the cursor width during the frozen-result flash hold.
	timingCursorWidthFlash = float32(12)
	// timingCursorBleed is the cursor's top/bottom overhang so it reads as a slider, not a fill.
	timingCursorBleed = int32(6)
	// timingTickBleed / timingTickFlashBleed: charge-segment separators straddle the bar
	// by this overhang — the plain tick (drawChargeTick) and the wider fresh-flash tick.
	timingTickBleed      = int32(3)
	timingTickFlashBleed = int32(5)
)

// drawTimingCursor paints the vertical cursor block at curX (press + charge bars).
func drawTimingCursor(curX, drawY, drawnH, cursorW float32, col rl.Color) {
	rl.DrawRectangle(int32(curX-cursorW/2), int32(drawY)-timingCursorBleed, int32(cursorW), int32(drawnH)+timingCursorBleed*2, col)
}

// timingTrackColor is the dark base fill behind every timing bar, derived from the shared
// barTrack hue (only alpha differs: 230 vs 140 so the tube reads solid). Retune base hue in barTrack.
var timingTrackColor = colorWithAlpha(barTrack, 230)

// drawTimingTrackBody stamps the recessed glass timing-track body (gauge well + dark
// glass fill) behind a bar or reel cell. One place so every track shares the look.
func drawTimingTrackBody(ix, iy, iw, ih int32) {
	drawGaugeWell(ix, iy, iw, ih)
	drawSmallPanel(ix, iy, iw, ih, timingTrackColor)
}

// drawTimingTrack paints the recessed gauge body (well + dark glass track) behind a timing bar,
// flooding with the grade color during the flash hold. Frame/studs go on top via drawTimingFrameOverlay.
func drawTimingTrack(drawX, drawY, barW, drawnH float32, quality int, isDefend, flashing bool, timingFlash float32) {
	ix, iy, iw, ih := int32(drawX), int32(drawY), int32(barW), int32(drawnH)
	drawTimingTrackBody(ix, iy, iw, ih)
	if flashing {
		flood := qualityColor(quality, isDefend)
		flood.A = uint8(float32(timingStrongFillAlpha) * flashAlpha(timingFlash))
		drawSmallPanel(ix, iy, iw, ih, flood)
	}
}

// Brass-stud geometry: studR/studInset for the full frame overlay; reelStudR/reelStudInset
// for the narrower per-reel cells (kept separate so the reel look stays pixel-identical).
const (
	studR         = float32(3)
	studInset     = float32(7)
	reelStudR     = float32(2.5)
	reelStudInset = float32(6)
)

// drawTimingFrame seats the candlelit cabinet chrome (wood bezel, gilt outline, four
// corner studs) over a bar/cell interior. studRadius/studIns size the rivets; outline
// is the breathing frame color. Shared by the press/charge overlay and locked reel cells.
func drawTimingFrame(drawX, drawY, barW, drawnH, studRadius, studIns float32, outline color.RGBA) {
	ix, iy, iw, ih := int32(drawX), int32(drawY), int32(barW), int32(drawnH)
	drawGaugeBezel(ix, iy, iw, ih, false)
	drawSmallPanelOutline(ix, iy, iw, ih, outline)
	drawBrassStud(drawX+studIns, drawY+studIns, studRadius)
	drawBrassStud(drawX+barW-studIns, drawY+studIns, studRadius)
	drawBrassStud(drawX+studIns, drawY+drawnH-studIns, studRadius)
	drawBrassStud(drawX+barW-studIns, drawY+drawnH-studIns, studRadius)
}

// drawTimingFrameOverlay caps a press/charge bar with the full-size cabinet chrome.
// Drawn AFTER the interior content so it seats over the edges.
func drawTimingFrameOverlay(drawX, drawY, barW, drawnH float32) {
	flick := candleFlicker()
	drawTimingFrame(drawX, drawY, barW, drawnH, studR, studInset, fadeColor(giltBright, 0.55+0.3*flick))
}

// drawExcellentShockwave paints the expanding ring from the frozen cursor on an Excellent
// resolution, fading with the flash hold. Press + charge bars; isDefend picks the hue.
func drawExcellentShockwave(curX, drawY, drawnH, flashTimer float32, isDefend bool) {
	phase := 1 - flashAlpha(flashTimer) // 0 fresh → 1 done
	radius := 14 + phase*72
	ringCol := qualityColor(core.TimingQualityExcellent, isDefend)
	ringCol.A = uint8(float32(timingStrongFillAlpha) * (1 - phase))
	cy := drawY + drawnH*0.5
	rl.DrawCircleLines(int32(curX), int32(cy), radius, ringCol)
	rl.DrawCircleLines(int32(curX), int32(cy), radius+2, ringCol)
}

// drawPressBar is the press-kind bar: nested quality zones, sliding cursor, flash on press.
// Juice: barThrob (graded flash), barShake (Miss), live grade-preview cursor tint, Excellent shockwave.
func drawPressBar(timing core.TimingState, g *core.GameState, assets Resources, x, y, barW, barH float32, flashing bool) {
	isDefend := g.Battle.Phase == core.BattleEnemyTiming

	hs := headStrike
	if isDefend {
		hs = headDefend
	}
	heading, baseCol := hs.text, hs.color

	xOff, yOff, drawnH := applyBarMotion(timing, g.Battle.TimingFlash, barH)
	drawX := x + xOff
	drawY := y + yOff

	drawTimingHeading(assets.hudFont, heading, hs.glyph, drawX, barW, drawY, baseCol, flashing, qualityColor(timing.Quality, isDefend))

	// Track (flash hold fades it to the quality color).
	drawTimingTrack(drawX, drawY, barW, drawnH, timing.Quality, isDefend, flashing, g.Battle.TimingFlash)

	// Nested quality zones (Nice→Good→Great→Excellent) inside the acceptance window;
	// two-zone Swipe bars paint both windows.
	if timing.IsTallyMode() {
		// Tally bars render per-window pips even under the flash: the final hit auto-resolves
		// the bar (flips flashing true next frame), so without this the last hit loses its pop.
		drawTallyBar(timing, drawX, drawY, barW, drawnH, isDefend)
	} else if !flashing {
		drawPressWindowZones(timing.WindowStart, timing.WindowEnd, timing.SweetSpot, timing.Duration, drawX, drawY, barW, drawnH, isDefend)
	}

	// Cursor — frozen at the press position during the flash hold. (Intro pause leaves Progress() at 0.)
	curPct := timing.Progress()
	curX := drawX + curPct*barW
	cursorW := timingCursorWidth
	cursorCol := timingCursorColor
	// Live grade preview: tint toward the grade it would land, blended from white so the
	// cursor stays visible when sitting on top of its own (same-color) zone.
	if !flashing && !timing.Resolved {
		if preview := timing.PreviewQuality(); preview > core.TimingQualityMiss {
			cursorCol = blendTowardWhite(qualityColor(preview, isDefend), 0.55)
		}
	}
	if flashing {
		cursorW, cursorCol = applyTimingFlashCursor(curX, drawY, drawnH, g.Battle.TimingFlash, qualityColor(timing.Quality, isDefend))
	}
	drawTimingCursor(curX, drawY, drawnH, cursorW, cursorCol)

	// Excellent shockwave during the flash hold (Excellent only).
	if flashing && timing.Quality == core.TimingQualityExcellent {
		drawExcellentShockwave(curX, drawY, drawnH, g.Battle.TimingFlash, isDefend)
	}

	drawTimingFrameOverlay(drawX, drawY, barW, drawnH)
}

// drawChargeBar paints the charge-and-release bar: three tick-separated segments that fill as the
// cursor crosses them, a bright peak window, then a dim decay zone. The cursor sweeps regardless
// of hold state (Elapsed always counts up) so the peak window is visible before pressing.
func drawChargeBar(timing core.TimingState, g *core.GameState, assets Resources, x, y, barW, barH float32, flashing bool) {
	heading, baseCol := headCharge.text, headCharge.color
	// Intro-pause: show "Press to start" so the bar reads as waiting on input.
	if !flashing && g.Battle.TimingIntro > 0 {
		heading = "Press to start"
		baseCol = textDim
	} else if timing.Pressed {
		// Past the peak start? Push toward "release now."
		if timing.Elapsed >= timing.WindowStart {
			heading = "RELEASE!"
			baseCol = colorWithAlpha(timingHeldColor, 250)
		}
	}

	xOff, yOff, drawnH := applyBarMotion(timing, g.Battle.TimingFlash, barH)
	drawX := x + xOff
	drawY := y + yOff

	drawTimingHeading(assets.hudFont, heading, headCharge.glyph, drawX, barW, drawY, baseCol, flashing, qualityColor(timing.Quality, false))

	// Track.
	drawTimingTrack(drawX, drawY, barW, drawnH, timing.Quality, false, flashing, g.Battle.TimingFlash)

	if !flashing {
		// Decay zone first (peak overlays it); Nice grade tone so a palette retune carries along.
		decayCol := colorWithAlpha(qualityColor(core.TimingQualityNice, false), timingStrongFillAlpha)
		drawBarSlice(drawX, drawY, barW, drawnH, core.ChargePeakEnd, 1.0, decayCol)

		// Peak window (release zone).
		peakCol := qualityColor(core.TimingQualityExcellent, false)
		drawBarSlice(drawX, drawY, barW, drawnH, core.ChargePeakStart, core.ChargePeakEnd, peakCol)

		// Charging fill snaps to the last crossed tick — the grade counts only completed ticks,
		// so a continuous fill would imply partial credit. The cursor glides smoothly as a separate visual.
		if timing.Pressed {
			fillEnd := chargeFillEnd(timing)
			if fillEnd > 0 {
				chargeCol := colorWithAlpha(qualityColor(core.TimingQualityGood, false), timingStrongFillAlpha)
				drawBarSlice(drawX, drawY, barW, drawnH, 0, fillEnd, chargeCol)
			}
		}

		// Tick markers, each with a ~220ms freshness flash as the cursor crosses it.
		drawChargeTickWithFlash(timing, drawX, drawY, barW, drawnH, core.ChargeTick1Pct)
		drawChargeTickWithFlash(timing, drawX, drawY, barW, drawnH, core.ChargeTick2Pct)
		drawChargeTickWithFlash(timing, drawX, drawY, barW, drawnH, core.ChargeTick3Pct)
	}

	// Cursor — slides with Elapsed, brightens when held.
	curPct := timing.Progress()
	curX := drawX + curPct*barW
	cursorW := timingCursorWidth
	cursorCol := colorWithAlpha(timingCursorColor, timingStrongFillAlpha)
	if timing.Pressed && !timing.Resolved {
		// Held: punchier cursor + halo.
		cursorW = timingCursorWidthHeld
		cursorCol = timingHeldColor
		halo := cursorCol
		halo.A = 90
		drawTimingCursor(curX, drawY, drawnH, cursorW*2, halo)
	}
	if flashing {
		cursorW, cursorCol = applyTimingFlashCursor(curX, drawY, drawnH, g.Battle.TimingFlash, qualityColor(timing.Quality, false))
	}
	drawTimingCursor(curX, drawY, drawnH, cursorW, cursorCol)

	// Excellent shockwave on release (same as the press bar).
	if flashing && timing.Quality == core.TimingQualityExcellent {
		drawExcellentShockwave(curX, drawY, drawnH, g.Battle.TimingFlash, false)
	}

	drawTimingFrameOverlay(drawX, drawY, barW, drawnH)
}

// drawChargeTickWithFlash paints a tick marker plus a ~220ms freshness glow (via tickFreshness).
func drawChargeTickWithFlash(timing core.TimingState, barX, barY, barW, barH float32, pct float32) {
	drawChargeTick(timing, barX, barY, barW, barH, pct)
	fresh := tickFreshness(timing, pct)
	if fresh <= 0 {
		return
	}
	tx := barX + pct*barW
	col := qualityColor(core.TimingQualityExcellent, false)
	col.A = uint8(float32(timingStrongFillAlpha) * fresh)
	width := 2 + fresh*4
	rl.DrawRectangle(int32(tx-width/2), int32(barY)-timingTickFlashBleed, int32(width), int32(barH)+timingTickFlashBleed*2, col)
}

// drawSequenceBar paints the pickpocket prompt: N arrows tapped in order before the timer drains.
// No backing track; arrows float over the scene with drop shadows, a dwindling line shows the timer.
func drawSequenceBar(timing core.TimingState, g *core.GameState, assets Resources, x, y, barW, barH float32, flashing bool) {
	heading := timingHeadingCombo
	baseCol := colorWithAlpha(seqOkColor, 240) // thief green

	xOff, _, _ := applyBarMotion(timing, g.Battle.TimingFlash, barH)
	drawX := x + xOff

	// noPromptGlyph: the arrow slots below already show which directions to press.
	drawTimingHeading(assets.hudFont, heading, noPromptGlyph, drawX, barW, y, baseCol, flashing, qualityColor(timing.Quality, false))

	count := len(timing.SequenceTargets)
	pad, slotWidth, arrowSize, ok := arrowRowLayout(barW, barH, count)
	if !ok {
		return
	}

	for i, dir := range timing.SequenceTargets {
		cx := drawX + pad + slotWidth*(float32(i)+0.5)
		cy := y + barH*0.5
		// Guard the index so a length drift degrades to "pending" rather than panicking.
		state := core.SeqResultPending
		if i < len(timing.SequenceResults) {
			state = timing.SequenceResults[i]
		}
		drawSeqArrowSlot(g, cx, cy, arrowSize, dir, i, state, i == timing.SequenceCursor, flashing)
	}

	if !flashing && timing.Duration > 0 {
		drawDwindlingTimerStrip(drawX, y, barW, barH, 1.0-timing.Progress())
	}
}

// drawReelBar paints Steal's slot-machine gamble: one framed cell per reel. Spinning reels dim;
// stopped reels gild their frame and show the locked symbol solid; resolve flash tints them.
func drawReelBar(timing core.TimingState, g *core.GameState, assets Resources, x, y, barW, barH float32, flashing bool) {
	// Reels stand taller than the shared gauge so the spinning drum reads clearly;
	// grow upward from the shared baseline so the bottom edge stays aligned.
	const reelBarHeightScale = float32(1.6)
	y += barH - barH*reelBarHeightScale
	barH *= reelBarHeightScale
	xOff, _, _ := applyBarMotion(timing, g.Battle.TimingFlash, barH)
	drawX := x + xOff
	drawTimingHeading(assets.hudFont, headReels.text, headReels.glyph, drawX, barW, y, headReels.color, flashing, qualityColor(timing.Quality, false))

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
	// symStep: vertical gap between symbols (~cell half-height) so the centre sits on the pay-line
	// and neighbours show above/below — the read of a reel drum mid-spin.
	symStep := barH * 0.5
	r := barH * 0.2
	ncol := len(reelSymbolColors)
	flash := g.Battle.TimingFlash
	for i := 0; i < n; i++ {
		cellX := drawX + barRowPadPx + (cellW+barCellGapPx)*float32(i)
		stopped := timing.Reels[i].Stop >= 0
		ix, iy, iw, ih := int32(cellX), int32(y), int32(cellW), int32(barH)
		sx := cellX + cellW*0.5

		// Recessed glass window per reel (same gauge body as the press/charge tracks).
		drawTimingTrackBody(ix, iy, iw, ih)

		// Clip the scrolling strip to the reel window so symbols slide in/out, not spill.
		rl.BeginScissorMode(ix, iy, iw, ih)
		if stopped {
			// Reduce to a symbol identity (mod ReelSymbolCount) THEN to a hue (mod
			// ncol), matching the scrolling path so the locked colour can't diverge
			// from the spinning preview if the palette length and symbol count desync.
			sym := ((timing.Reels[i].Stop % core.ReelSymbolCount) + core.ReelSymbolCount) % core.ReelSymbolCount
			col := reelSymbolColors[sym%ncol]
			drawReelSymbol(sx, cy, r, col, flashing, flash)
		} else {
			// Continuous scroll: the symbol nearest the pay-line is the one a press locks
			// (ReelSymbolAt rounds to it), so it's brightest; the rest fade with distance.
			phase := timing.ReelPhaseAt(i)
			center := int(math.Round(float64(phase)))
			for d := -2; d <= 2; d++ {
				k := center + d
				sy := cy + (float32(k)-phase)*symStep
				if sy < y-r || sy > y+barH+r {
					continue
				}
				// Softer falloff (~2-symbol denominator) so neighbours stay visible.
				a := core.Clamp(1-float32(math.Abs(float64(sy-cy)))/(symStep*2.1), 0, 1)
				// Reduce to a symbol identity (mod ReelSymbolCount) before picking the hue, so
				// the centred symbol's colour matches the one it locks to (ReelSymbolAt's mod).
				sym := core.WrapIndex(k, core.ReelSymbolCount)
				col := colorWithAlpha(reelSymbolColors[sym%ncol], uint8(90+165*a))
				drawReelSymbol(sx, sy, r, col, flashing, flash)
			}
			// Faint gilt pay-line across the window centre — where a stop locks.
			rl.DrawRectangle(ix+4, int32(cy)-1, iw-8, 2, fadeForFlash(colorWithAlpha(giltBright, 70), flashing, flash))
		}
		rl.EndScissorMode()

		// Frame: dim wood rail while spinning; breathing gilt frame + corner studs once locked.
		if stopped {
			flick := candleFlicker()
			drawTimingFrame(cellX, y, cellW, barH, reelStudR, reelStudInset,
				fadeForFlash(fadeColor(giltBright, 0.6+0.3*flick), flashing, flash))
		} else {
			drawGaugeBezel(ix, iy, iw, ih, true)
			drawSmallPanelOutline(ix, iy, iw, ih, woodAccentSeam)
		}
	}
}

// drawReelSymbol paints one slot symbol: a dark rim under the colored fill. The rim alpha
// tracks the fill's so faded scrolling symbols don't carry a full-strength halo.
func drawReelSymbol(sx, sy, r float32, col rl.Color, flashing bool, flash float32) {
	rimA := uint8(int(col.A) * int(reelRimAlpha) / 255)
	rl.DrawCircleV(rl.NewVector2(sx, sy), r+2, fadeForFlash(colorWithAlpha(shadowBase, rimA), flashing, flash))
	rl.DrawCircleV(rl.NewVector2(sx, sy), r, fadeForFlash(col, flashing, flash))
}

// drawRecallBar paints Arc Bolt's memory minigame: pattern lit during MEMORIZE, face-down during
// RECALL (pending = dim dots, landed = answer tinted green/red). Reuses the sequence bar's arrows.
func drawRecallBar(timing core.TimingState, g *core.GameState, assets Resources, x, y, barW, barH float32, flashing bool) {
	hidden := timing.RecallHidden()
	hs := headRecallMemo
	if hidden {
		hs = headRecallRecall
	}
	heading, baseCol := hs.text, hs.color
	xOff, _, _ := applyBarMotion(timing, g.Battle.TimingFlash, barH)
	drawX := x + xOff
	drawTimingHeading(assets.hudFont, heading, hs.glyph, drawX, barW, y, baseCol, flashing, qualityColor(timing.Quality, false))

	count := len(timing.SequenceTargets)
	pad, slotWidth, arrowSize, ok := arrowRowLayout(barW, barH, count)
	if !ok {
		return
	}
	for i := 0; i < count; i++ {
		cx := drawX + pad + slotWidth*(float32(i)+0.5)
		cy := y + barH*0.5
		if !hidden {
			// Memorize phase: every pattern arrow lit, with a drop shadow.
			drawArrowWithShadow(cx, cy, arrowSize, timing.SequenceTargets[i],
				colorWithAlpha(timingCursorColor, barHighlightAlpha),
				colorWithAlpha(shadowBase, iconShadowAlpha))
			continue
		}
		// Recall phase: landed slots reveal their answer tinted by correctness; pending = dim dot.
		result := core.SeqResultPending
		if i < len(timing.SequenceResults) {
			result = timing.SequenceResults[i]
		}
		switch result {
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

// drawArrow paints a chunky arrow (head + stem) pointing in dir (SeqDir), centered at (cx,cy);
// size is the half-extent. All triangles wound CCW in screen-Y-down — back-face culling on some
// drivers silently dropped CW triangles, so visibility must not depend on cull state.
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
	// Stem as two CCW triangles (TL→BL→BR and TL→BR→TR in screen-Y-down).
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

// arrowAxisVec returns the tip-direction unit vector for a SeqDir (screen-Y-down).
// SeqDir* share int values with the cardinal facings (asserted in core/timing.go
// init), so the direction map is sourced once from core.FacingVector.
func arrowAxisVec(dir int) (float32, float32) {
	dx, dz := core.FacingVector(dir)
	return float32(dx), float32(dz)
}

// drawBarSlice paints a stripe between two normalized fractions (clamped to [0,1], skips zero/negative).
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

// chargeFillEnd returns the charging fill's extent ([0,1]), snapped to the last passed tick
// to match resolveCharge's discrete grading (release between ticks N and N+1 scores N).
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

// drawChargeTick paints the vertical separator between charge segments at the given fraction.
func drawChargeTick(timing core.TimingState, barX, barY, barW, barH float32, pct float32) {
	tx := barX + pct*barW
	tickCol := timingTickColor
	rl.DrawRectangle(int32(tx-1), int32(barY)-timingTickBleed, 2, int32(barH)+timingTickBleed*2, tickCol)
}

// drawWindowZone paints a stripe centered on sweet, scaled to `ratio` of the window width.
// Window scalars are explicit so either window of a two-zone press bar can route through it.
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

// drawPressWindowZones paints the nested Nice→Good→Great→Excellent stack for one press window.
func drawPressWindowZones(start, end, sweet, duration, barX, barY, barW, barH float32, isDefend bool) {
	drawWindowZone(start, end, sweet, duration, barX, barY, barW, barH, 1.00, qualityColor(core.TimingQualityNice, isDefend))
	drawWindowZone(start, end, sweet, duration, barX, barY, barW, barH, 0.60, qualityColor(core.TimingQualityGood, isDefend))
	drawWindowZone(start, end, sweet, duration, barX, barY, barW, barH, 0.30, qualityColor(core.TimingQualityGreat, isDefend))
	drawWindowZone(start, end, sweet, duration, barX, barY, barW, barH, 0.10, qualityColor(core.TimingQualityExcellent, isDefend))
}

// drawTallyBar paints the multi-press tally: one hit-zone stripe per accept window (solid unhit,
// dimmed consumed) plus an orange COMMIT zone at the tail.
// tallyConsumedAttack / tallyConsumedDefend are the dimmed "already hit" tints, derived once at init.
var (
	tallyConsumedAttack = makeTallyConsumedColor(false)
	tallyConsumedDefend = makeTallyConsumedColor(true)
)

func makeTallyConsumedColor(isDefend bool) rl.Color {
	c := qualityColor(core.TimingQualityGood, isDefend)
	// Dim to a third, then pin alpha to the tally's fixed 200.
	return colorWithAlpha(shadeColor(c, 0.33), 200)
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
	for i := range t.Windows {
		w := &t.Windows[i]
		startX := barX + (w.Start/t.Duration)*barW
		width := ((w.End - w.Start) / t.Duration) * barW
		// Base color by state: just-hit (FlashTimer) bright Excellent fading to consumed;
		// already-hit dim consumed; unhit cursor-inside pulses bright; else resting hit color.
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
			// Live-preview throb while the cursor is in-zone.
			throb := 0.75 + 0.25*pulseAttention()
			col = fadeColor(blendTowardWhite(hitCol, 0.45), throb)
		default:
			col = hitCol
		}
		rl.DrawRectangle(int32(startX), int32(barY), int32(width), int32(barH), col)
		// Sweet-spot pip — brighter notch at each unhit window's centre.
		if !w.Hit {
			pipX := barX + (w.Sweet/t.Duration)*barW
			pipW := width * 0.18
			pip := blendTowardWhite(hitCol, 0.60)
			rl.DrawRectangle(int32(pipX-pipW*0.5), int32(barY), int32(pipW), int32(barH), pip)
		}
	}
	// The commit zone (CommitStart..Duration) stays UNDRAWN: a press there still
	// resolves early in pressTally, but rendering its orange tail read as a third
	// hit hotspot. The bar shows exactly its scoring windows.
}

// flashAlpha returns the flash strength ([0,1]), peaking after the press and decaying
// with the hold timer (squared falloff: bright first, fades fast).
func flashAlpha(remaining float32) float32 {
	if core.TimingFlashDuration <= 0 {
		return 0
	}
	t := core.Clamp(remaining/core.TimingFlashDuration, 0, 1)
	return t * t
}

// popupOffScreenX reports whether a popup's screen X has drifted past the viewport edges
// (beyond offscreenPopupSlack) and should be culled. Shared by the quality + damage popups.
func popupOffScreenX(screenX, screenW float32) bool {
	return screenX < -offscreenPopupSlack || screenX > screenW+offscreenPopupSlack
}

// DrawQualityPopup floats the most recent quality result above the actor for QualityResultDuration,
// punching in with a scale-up then fading up and out.
func DrawQualityPopup(camera rl.Camera3D, g *core.GameState, assets Resources) {
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
	worldPos.Y += popupWorldRise
	// A behind-camera anchor projects mirrored into the visible range (raylib quirk);
	// skip it like the hit-glyph / interact-prompt paths do.
	if behindCamera(camera, worldPos) {
		return
	}
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

	// FontTitle; Excellent's extra punch comes from the throb-driven scale, not a larger size.
	baseSize := FontTitle
	size := baseSize * scale
	// Measure at the fixed base size (cache hits) and scale; the animating size would miss the cache.
	base := qualityPopupMeasureCache.measure(assets.hudFont, label, baseSize, 1.5)
	measure := rl.NewVector2(base.X*scale, base.Y*scale)
	x := screenPos.X - measure.X/2
	y := screenPos.Y - measure.Y - rise

	shadow := colorWithAlpha(shadowBase, alpha)
	drawTextWithShadowStyle(assets.hudFont, label, x, y, size, 1.5, col, shadow, 3, 3)
}

// qualityPopupAnchor returns the world position the quality text hovers above (a party
// sprite — actor for attacks, defender for blocks; enemies don't author quality popups).
func qualityPopupAnchor(camera rl.Camera3D, g *core.GameState) (rl.Vector3, bool) {
	idx := g.Battle.LastQualityIndex
	if idx < 0 || idx >= len(g.Party) {
		return rl.Vector3{}, false
	}
	pos := partySpritePosition(camera, g.Party, idx, 0, 0, 0)
	return rl.NewVector3(pos.X, pos.Y, pos.Z), true
}

// DrawDamagePopups floats the most recent damage number above each hit enemy for
// QualityResultDuration, colored by timing grade (Excellent gets a trailing "!").
func DrawDamagePopups(camera rl.Camera3D, g *core.GameState, assets Resources) {
	if !g.Battle.Active() {
		return
	}
	members := core.BattleMembers(g)
	for i := range members {
		enemy := &members[i] // pointer: no ~496-byte Enemy copy per member
		if enemy.DamagePopupTimer <= 0 {
			continue
		}
		// No Alive/DeathFade gate: the popup (~0.7s) outlasts DeathFade (~0.55s), so the
		// killing-blow number keeps floating after the body fades.
		pos := enemyDrawPosition(camera, g, i, enemy)
		// Per-kind number-spawn nudge (Num X/Y in the Foe Visualizer).
		if v, ok := enemyVisualFor(assets, enemy.Kind); ok {
			pos = cameraRelativeOffset(camera, pos, v.popupXOffset, v.popupYOffset, 0)
		}
		drawFloatingDamage(camera, assets, pos, enemy.DamagePopup, enemy.DamagePopupQuality,
			enemy.DamagePopupCrit, enemy.DamagePopupTimer, qualityColor(enemy.DamagePopupQuality, false))
	}
	// Party side: every hit a member takes floats a number too. Incoming hits aren't graded,
	// so they use the fixed hurt tone, not the timing-grade ramp.
	for i := range g.Party {
		m := &g.Party[i]
		if m.DamagePopupTimer <= 0 {
			continue
		}
		pos := partySpritePosition(camera, g.Party, i, 0, 0, 0)
		worldPos := rl.NewVector3(pos.X, pos.Y, pos.Z)
		// Per-class number-spawn nudge (Num X/Y in the Party Visualizer).
		if v, ok := partyVisualFor(assets, m.Class); ok {
			worldPos = cameraRelativeOffset(camera, worldPos, v.popupXOffset, v.popupYOffset, 0)
		}
		drawFloatingDamage(camera, assets, worldPos,
			m.DamagePopup, m.DamagePopupQuality, m.DamagePopupCrit, m.DamagePopupTimer, partyDamagePopupColor)
	}
}

// drawFloatingDamage renders one floating damage number above worldPos, shared by the enemy
// and party loops. col is the base tint (grade ramp outgoing, hurt tone incoming); alpha applied here.
// A crit prefixes the number with "Critical!".
func drawFloatingDamage(camera rl.Camera3D, assets Resources, worldPos rl.Vector3, value, quality int, crit bool, timer float32, col rl.Color) {
	worldPos.Y += popupWorldRise
	// A behind-camera anchor projects mirrored into view (raylib quirk); skip it.
	if behindCamera(camera, worldPos) {
		return
	}
	screenPos := rl.GetWorldToScreen(worldPos, camera)
	sw, _ := screenSizeF()
	if popupOffScreenX(screenPos.X, sw) {
		return
	}

	t := timer / core.QualityResultDuration
	scale, rise, alpha := popupAnimation(t)

	label := damagePopupLabel(value, quality, crit)
	col.A = alpha

	// FontHeading; Excellent's stronger throb comes from the scale factor, not a larger size.
	baseSize := FontHeading
	size := baseSize * scale
	// Measure at the fixed base size (cache hits) and scale; see DrawQualityPopup.
	base := damagePopupMeasureCache.measure(assets.hudFont, label, baseSize, 1.2)
	measure := rl.NewVector2(base.X*scale, base.Y*scale)
	x := screenPos.X - measure.X/2
	y := screenPos.Y - measure.Y - rise

	shadow := colorWithAlpha(shadowBase, alpha)
	drawTextWithShadowStyle(assets.hudFont, label, x, y, size, 1.2, col, shadow, 2, 2)
}

// damagePopupLabel formats the damage value (appending "!" on Excellent), from a 0..199
// {plain,excellent} LUT; past that, strconv concat. A crit prefixes "Critical! "
// (and skips the LUT — crit popups are comparatively rare).
func damagePopupLabel(damage, quality int, crit bool) string {
	if crit {
		return "Critical! " + strconv.Itoa(damage)
	}
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

// popupWorldRise lifts a popup's anchor to torso height before projection. The Layout-tab
// "Num" gizmo anchors add the same lift — keep all four sites on this const.
const popupWorldRise = float32(0.6)

// Popup punch-in curve breakpoints (peak scale + phase durations); popupPeakEnd = grow + shrink.
const (
	popupPeakScale   = 2.15
	popupStartScale  = 0.6 // scale at spawn; grows to peak then settles to 1.0
	popupGrowPhase   = 0.12
	popupShrinkPhase = 0.10
	popupPeakEnd     = 0.22
)

// popupAnimation returns scale/rise/alpha for life ratio t (1=spawned, 0=expired):
// 0.6→2.15→1.0 scale, 36px rise, Smoothstep alpha fade.
func popupAnimation(t float32) (scale, rise float32, alpha uint8) {
	t = core.Clamp(t, 0, 1)
	age := 1 - t
	scale = 1
	switch {
	case age < popupGrowPhase:
		scale = popupStartScale + (popupPeakScale-popupStartScale)*(age/popupGrowPhase)
		if scale > popupPeakScale {
			scale = popupPeakScale
		}
	case age < popupPeakEnd:
		scale = popupPeakScale - (popupPeakScale-1)*((age-popupGrowPhase)/popupShrinkPhase)
	}
	rise = (1 - t) * 36
	alpha = uint8(255 * core.Smoothstep(t))
	return
}

// qualityColor returns the bar/popup tint for a grade (defend shifts toward cool blues);
// out-of-range falls through to the Miss color.
func qualityColor(quality int, isDefend bool) rl.Color {
	if quality < 0 || quality >= len(qualityVisuals) {
		return qualityVisuals[core.TimingQualityMiss].AttackColor
	}
	if isDefend {
		return qualityVisuals[quality].DefendColor
	}
	return qualityVisuals[quality].AttackColor
}
