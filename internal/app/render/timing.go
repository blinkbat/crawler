package render

import (
	"fmt"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

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
	case core.TimingKindCharge:
		drawChargeBar(timing, g, assets, x, y, barW, barH, flashing)
	case core.TimingKindSequence:
		drawSequenceBar(timing, g, assets, x, y, barW, barH, flashing)
	default:
		drawPressBar(timing, g, assets, x, y, barW, barH, flashing)
	}
}

// timingBarLayout returns the bar's screen-space rectangle. Both kinds share
// the same footprint so switching modes doesn't cause the strip to jump.
func timingBarLayout() (x, y, barW, barH float32) {
	screenW := float32(rl.GetScreenWidth())
	barH = 34
	barW = screenW * 0.62
	if barW < 380 {
		barW = 380
	}
	if barW > screenW-32 {
		barW = screenW - 32
	}
	x = (screenW - barW) / 2
	y = PartyRibbonTopY() - barH - 28
	return
}

// drawTimingHeading paints the centered prompt above the timing bar. Color
// shifts to the quality tint during the flash hold so the prompt itself reads
// the result. (Distinct from theme.go's drawHeading which is a generic panel
// header.)
func drawTimingHeading(font rl.Font, text string, x, barW, y float32, baseCol rl.Color, flashing bool, flashCol rl.Color) {
	size := float32(28)
	col := baseCol
	if flashing {
		col = flashCol
		size = 34
	}
	measure := rl.MeasureTextEx(font, text, size, 1.5)
	hx := x + (barW-measure.X)/2
	hy := y - measure.Y - 6
	rl.DrawTextEx(font, text, rl.NewVector2(hx+2, hy+2), size, 1.5, rl.NewColor(0, 0, 0, 200))
	rl.DrawTextEx(font, text, rl.NewVector2(hx, hy), size, 1.5, col)
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

// drawPressBar is the original press-kind bar: nested quality zones inside
// the acceptance window, sliding cursor, flash on press.
func drawPressBar(timing core.TimingState, g core.GameState, assets Resources, x, y, barW, barH float32, flashing bool) {
	isDefend := g.Battle.Phase == core.BattleEnemyTiming

	heading := "STRIKE!"
	baseCol := rl.NewColor(255, 232, 168, 240)
	if isDefend {
		heading = "DEFEND!"
		baseCol = rl.NewColor(168, 220, 255, 240)
	}
	drawTimingHeading(assets.hudFont, heading, x, barW, y, baseCol, flashing, qualityColor(timing.Quality, isDefend))

	// Track — solid dark fill, no border. During the flash hold the track
	// fades to the quality color so the whole bar pulses with the result.
	trackCol := rl.NewColor(14, 16, 26, 230)
	if flashing {
		trackCol = qualityColor(timing.Quality, isDefend)
		trackCol.A = uint8(220 * flashAlpha(g.Battle.TimingFlash))
	}
	rl.DrawRectangle(int32(x), int32(y), int32(barW), int32(barH), trackCol)

	// Quality zones inside the acceptance window — Nice (outermost) → Good →
	// Great → Excellent (centered on the sweet spot). Each is a full-height
	// solid color stripe; nesting communicates the grading without any lines.
	if !flashing {
		drawWindowZone(timing, x, y, barW, barH, 1.00, qualityColor(core.TimingQualityNice, isDefend))
		drawWindowZone(timing, x, y, barW, barH, 0.60, qualityColor(core.TimingQualityGood, isDefend))
		drawWindowZone(timing, x, y, barW, barH, 0.30, qualityColor(core.TimingQualityGreat, isDefend))
		drawWindowZone(timing, x, y, barW, barH, 0.10, qualityColor(core.TimingQualityExcellent, isDefend))
	}

	// Cursor — a fat vertical block sliding across the bar. Frozen at the
	// press position during the flash hold so the player sees where they hit.
	// During the intro pause (TimingIntro > 0) Tick isn't called, so
	// Progress() naturally stays at 0 — no special-casing needed here.
	curPct := timing.Progress()
	curX := x + curPct*barW
	cursorW := float32(8)
	cursorCol := rl.NewColor(248, 248, 252, 255)
	if flashing {
		cursorW, cursorCol = applyTimingFlashCursor(curX, y, barH, g.Battle.TimingFlash, qualityColor(timing.Quality, isDefend))
	}
	rl.DrawRectangle(int32(curX-cursorW/2), int32(y)-6, int32(cursorW), int32(barH)+12, cursorCol)
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
	heading := "CHARGE!"
	baseCol := rl.NewColor(255, 184, 96, 240) // warm orange
	if timing.Pressed {
		// Past the peak start? Push the prompt toward "release now" feel.
		if timing.Elapsed >= timing.WindowStart {
			heading = "RELEASE!"
			baseCol = rl.NewColor(255, 244, 144, 250)
		}
	}
	drawTimingHeading(assets.hudFont, heading, x, barW, y, baseCol, flashing, qualityColor(timing.Quality, false))

	// Track — dark base fill.
	trackCol := rl.NewColor(14, 16, 26, 230)
	if flashing {
		trackCol = qualityColor(timing.Quality, false)
		trackCol.A = uint8(220 * flashAlpha(g.Battle.TimingFlash))
	}
	rl.DrawRectangle(int32(x), int32(y), int32(barW), int32(barH), trackCol)

	if !flashing {
		// Decay zone (dim warning) — drawn first so the peak overlays it.
		decayStart := timing.WindowEnd
		decayCol := rl.NewColor(184, 96, 80, 220)
		drawTimeRange(decayStart, timing.Duration, timing, x, y, barW, barH, decayCol)

		// Peak window (release zone) — bright Excellent color.
		peakCol := qualityColor(core.TimingQualityExcellent, false)
		drawTimeRange(timing.WindowStart, timing.WindowEnd, timing, x, y, barW, barH, peakCol)

		// Charging fill — snaps forward only when the cursor crosses a tick
		// boundary, since the *grade* counts only fully-completed ticks.
		// A continuous fill would mislead the player into thinking partial
		// progress between ticks scored partial credit. The cursor still
		// glides smoothly through the bar as a separate visual.
		if timing.Pressed {
			fillEnd := chargeFillEnd(timing)
			if fillEnd > 0 {
				chargeCol := rl.NewColor(232, 144, 80, 220)
				drawTimeRange(0, fillEnd, timing, x, y, barW, barH, chargeCol)
			}
		}

		// Tick markers — vertical separators between the three charge segments.
		drawChargeTick(timing, x, y, barW, barH, core.ChargeTick1Pct)
		drawChargeTick(timing, x, y, barW, barH, core.ChargeTick2Pct)
		drawChargeTick(timing, x, y, barW, barH, core.ChargeTick3Pct)
	}

	// Cursor — slides with Elapsed, brightens when held.
	curPct := timing.Progress()
	curX := x + curPct*barW
	cursorW := float32(8)
	cursorCol := rl.NewColor(248, 248, 252, 220)
	if timing.Pressed && !timing.Resolved {
		// Held: punchy cursor with a small halo so the engaged state reads.
		cursorW = 10
		cursorCol = rl.NewColor(255, 244, 144, 255)
		halo := cursorCol
		halo.A = 90
		rl.DrawRectangle(int32(curX-cursorW), int32(y)-6, int32(cursorW*2), int32(barH)+12, halo)
	}
	if flashing {
		cursorW, cursorCol = applyTimingFlashCursor(curX, y, barH, g.Battle.TimingFlash, qualityColor(timing.Quality, false))
	}
	rl.DrawRectangle(int32(curX-cursorW/2), int32(y)-6, int32(cursorW), int32(barH)+12, cursorCol)
}

// drawSequenceBar paints the pickpocket prompt: a row of N arrows the player
// must tap in order before the timer drains. No backing track — the arrows
// float over the 3D scene with drop shadows so they're readable against any
// background. A thin line under the arrows dwindles left-to-right to
// communicate the timer.
func drawSequenceBar(timing core.TimingState, g core.GameState, assets Resources, x, y, barW, barH float32, flashing bool) {
	heading := "PICKPOCKET!"
	baseCol := rl.NewColor(140, 232, 168, 240) // thief green
	drawTimingHeading(assets.hudFont, heading, x, barW, y, baseCol, flashing, qualityColor(timing.Quality, false))

	count := len(timing.SequenceTargets)
	if count == 0 {
		return
	}

	// Arrows are spaced evenly across the row. With no track to constrain
	// them, we can size them larger so they read as the focus of the prompt.
	pad := float32(18)
	available := barW - pad*2
	slotWidth := available / float32(count)
	arrowSize := slotWidth * 0.35
	if arrowSize > barH*0.85 {
		arrowSize = barH * 0.85
	}

	for i, dir := range timing.SequenceTargets {
		cx := x + pad + slotWidth*(float32(i)+0.5)
		cy := y + barH*0.5
		state := timing.SequenceResults[i]

		var col rl.Color
		switch state {
		case core.SeqResultCorrect:
			col = rl.NewColor(140, 232, 168, 255) // green
		case core.SeqResultWrong:
			col = rl.NewColor(228, 96, 96, 255) // red
		default:
			col = rl.NewColor(248, 248, 252, 245) // pending bright white
		}
		if flashing {
			col.A = uint8(float32(col.A) * flashAlpha(g.Battle.TimingFlash))
		}

		// Drop shadow: same triangle drawn 3 px down-right in transparent
		// black so the arrow stays readable over busy 3D backgrounds.
		shadowAlpha := uint8(180)
		if flashing {
			shadowAlpha = uint8(float32(shadowAlpha) * flashAlpha(g.Battle.TimingFlash))
		}
		drawArrow(cx+3, cy+3, arrowSize, dir, rl.NewColor(0, 0, 0, shadowAlpha))
		drawArrow(cx, cy, arrowSize, dir, col)

		// Cursor underline below the next slot.
		if !flashing && i == timing.SequenceCursor {
			ux := cx - arrowSize*0.85
			uw := arrowSize * 1.7
			uy := cy + arrowSize + 8
			rl.DrawRectangle(int32(ux)+2, int32(uy)+2, int32(uw), 4, rl.NewColor(0, 0, 0, 160))
			rl.DrawRectangle(int32(ux), int32(uy), int32(uw), 4, rl.NewColor(255, 244, 144, 245))
		}
	}

	// Dwindling timer — a thin line under the arrow row that shrinks toward
	// the center as time runs out. Red when the clock's almost gone so the
	// urgency reads in peripheral vision.
	if !flashing && timing.Duration > 0 {
		stripH := float32(3)
		stripY := y + barH + 10
		remaining := 1.0 - timing.Progress()
		if remaining < 0 {
			remaining = 0
		}
		stripCol := rl.NewColor(140, 232, 168, 230)
		if remaining < 0.30 {
			stripCol = rl.NewColor(228, 96, 96, 240)
		} else if remaining < 0.55 {
			stripCol = rl.NewColor(232, 196, 92, 235)
		}
		// Center-anchored shrink: the line stays centered as it retracts
		// from both ends, matching how the arrows are centered in their slots.
		visW := barW * remaining
		stripX := x + (barW-visW)*0.5
		rl.DrawRectangle(int32(stripX)+1, int32(stripY)+1, int32(visW), int32(stripH), rl.NewColor(0, 0, 0, 160))
		rl.DrawRectangle(int32(stripX), int32(stripY), int32(visW), int32(stripH), stripCol)
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
	rl.DrawTriangle(
		rl.NewVector2(tipX, tipY),
		rl.NewVector2(headLX, headLY),
		rl.NewVector2(headRX, headRY),
		col,
	)
	// Stem rectangle as two CCW triangles in screen-Y-down. Visualizing
	// the stem as TL/TR/BL/BR corners on a clock face (TL=11, TR=1,
	// BR=5, BL=7), the CCW orderings are TL→BL→BR and TL→BR→TR.
	rl.DrawTriangle(
		rl.NewVector2(stemTLX, stemTLY),
		rl.NewVector2(stemBLX, stemBLY),
		rl.NewVector2(stemBRX, stemBRY),
		col,
	)
	rl.DrawTriangle(
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

// drawTimeRange paints a solid color stripe between two times (in seconds)
// across the bar. Used by the charge bar to paint peak / decay / fill zones.
func drawTimeRange(startSec, endSec float32, timing core.TimingState, barX, barY, barW, barH float32, col rl.Color) {
	if timing.Duration <= 0 || endSec <= startSec {
		return
	}
	drawBarSlice(barX, barY, barW, barH, startSec/timing.Duration, endSec/timing.Duration, col)
}

// chargeFillEnd returns how far (in seconds along the bar) the orange
// charging fill should extend, snapped to the last fully-passed tick. This
// matches resolveCharge's discrete grading: a release between tick N and
// tick N+1 scores N ticks, so the visual should also read as "N ticks
// filled" rather than telegraphing partial progress.
func chargeFillEnd(timing core.TimingState) float32 {
	tick1 := core.ChargeTick1Pct * timing.Duration
	tick2 := core.ChargeTick2Pct * timing.Duration
	tick3 := core.ChargeTick3Pct * timing.Duration
	switch {
	case timing.Elapsed >= tick3:
		return tick3
	case timing.Elapsed >= tick2:
		return tick2
	case timing.Elapsed >= tick1:
		return tick1
	default:
		return 0
	}
}

// drawChargeTick paints a thin vertical separator line at the given fraction
// of the bar — the visible boundary between charge segments.
func drawChargeTick(timing core.TimingState, barX, barY, barW, barH float32, pct float32) {
	tx := barX + pct*barW
	tickCol := rl.NewColor(28, 32, 44, 235)
	rl.DrawRectangle(int32(tx-1), int32(barY)-3, 2, int32(barH)+6, tickCol)
}

// drawWindowZone paints a solid color stripe centered on the sweet spot,
// scaled to a fraction of the acceptance window's full width. Used to nest
// quality bands without drawing any borders.
func drawWindowZone(timing core.TimingState, barX, barY, barW, barH float32, ratio float32, col rl.Color) {
	windowSize := timing.WindowEnd - timing.WindowStart
	if windowSize <= 0 || timing.Duration <= 0 {
		return
	}
	half := windowSize * ratio * 0.5
	startSec := timing.SweetSpot - half
	endSec := timing.SweetSpot + half
	if startSec < timing.WindowStart {
		startSec = timing.WindowStart
	}
	if endSec > timing.WindowEnd {
		endSec = timing.WindowEnd
	}
	drawBarSlice(barX, barY, barW, barH, startSec/timing.Duration, endSec/timing.Duration, col)
}

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

// DrawQualityPopup floats the most recent quality result above the actor for
// QualityResultDuration after each timing resolution. Punches in with a quick
// scale-up for impact, then fades up and out.
func DrawQualityPopup(camera rl.Camera3D, g core.GameState, assets Resources) {
	if g.Battle.LastQualityTimer <= 0 {
		return
	}
	if g.Battle.Phase == core.BattleNone {
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
	if screenPos.X < -200 || screenPos.X > float32(rl.GetScreenWidth())+200 {
		return
	}

	label := core.TimingQualityLabel(g.Battle.LastQuality)
	if g.Battle.LastQualityIsBlock && g.Battle.LastQuality > core.TimingQualityMiss {
		label = "BLOCK!"
	}
	col := qualityColor(g.Battle.LastQuality, g.Battle.LastQualityIsBlock)
	col.A = alpha

	baseSize := float32(34)
	if g.Battle.LastQuality == core.TimingQualityExcellent {
		baseSize = 42
	}
	size := baseSize * scale
	measure := rl.MeasureTextEx(assets.hudFont, label, size, 1.5)
	x := screenPos.X - measure.X/2
	y := screenPos.Y - measure.Y - rise

	shadow := rl.NewColor(0, 0, 0, alpha)
	rl.DrawTextEx(assets.hudFont, label, rl.NewVector2(x+3, y+3), size, 1.5, shadow)
	rl.DrawTextEx(assets.hudFont, label, rl.NewVector2(x, y), size, 1.5, col)
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
	pos := partySpritePosition(camera, idx, g.Party[idx].Class, 0, 0)
	return rl.NewVector3(pos.X, pos.Y, pos.Z), true
}

// DrawDamagePopups floats the most recent damage number above each hit enemy
// for QualityResultDuration. Color matches the timing quality (so a Great
// reads as the same orange as the player's quality popup); an Excellent hit
// gets a trailing "!" so the big damage spike reads at a glance.
func DrawDamagePopups(camera rl.Camera3D, g core.GameState, assets Resources) {
	if g.Battle.Phase == core.BattleNone {
		return
	}
	for i, enemy := range core.BattleMembers(&g) {
		if enemy.DamagePopupTimer <= 0 {
			continue
		}
		// Note: we deliberately don't gate on Alive/DeathFade here. Popup
		// duration (QualityResultDuration ~0.7s) outlasts DeathFade (~0.55s),
		// so for the killing blow we want the number to keep floating after
		// the body fades. enemyDrawPosition still returns a valid spot since
		// the member's slot is stable for the lifetime of the active pack.
		pos := enemyDrawPosition(camera, g, i, enemy)
		pos.Y += 0.6
		screenPos := rl.GetWorldToScreen(pos, camera)
		if screenPos.X < -200 || screenPos.X > float32(rl.GetScreenWidth())+200 {
			continue
		}

		t := enemy.DamagePopupTimer / core.QualityResultDuration
		scale, rise, alpha := popupAnimation(t)

		label := damagePopupLabel(enemy.DamagePopup, enemy.DamagePopupQuality)
		col := qualityColor(enemy.DamagePopupQuality, false)
		col.A = alpha

		baseSize := float32(30)
		if enemy.DamagePopupQuality == core.TimingQualityExcellent {
			baseSize = 38
		}
		size := baseSize * scale
		measure := rl.MeasureTextEx(assets.hudFont, label, size, 1.2)
		x := screenPos.X - measure.X/2
		y := screenPos.Y - measure.Y - rise

		shadow := rl.NewColor(0, 0, 0, alpha)
		rl.DrawTextEx(assets.hudFont, label, rl.NewVector2(x+2, y+2), size, 1.2, shadow)
		rl.DrawTextEx(assets.hudFont, label, rl.NewVector2(x, y), size, 1.2, col)
	}
}

// damagePopupLabel formats the damage value, appending "!" on an Excellent.
func damagePopupLabel(damage, quality int) string {
	if quality == core.TimingQualityExcellent {
		return fmt.Sprintf("%d!", damage)
	}
	return fmt.Sprintf("%d", damage)
}

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
func qualityColor(quality int, isDefend bool) rl.Color {
	switch quality {
	case core.TimingQualityExcellent:
		if isDefend {
			return rl.NewColor(196, 240, 255, 255)
		}
		return rl.NewColor(255, 244, 144, 255)
	case core.TimingQualityGreat:
		if isDefend {
			return rl.NewColor(120, 200, 248, 255)
		}
		return rl.NewColor(255, 188, 88, 255)
	case core.TimingQualityGood:
		if isDefend {
			return rl.NewColor(80, 152, 220, 255)
		}
		return rl.NewColor(232, 144, 80, 255)
	case core.TimingQualityNice:
		if isDefend {
			return rl.NewColor(56, 110, 184, 255)
		}
		return rl.NewColor(184, 96, 80, 255)
	default:
		return rl.NewColor(220, 76, 76, 255)
	}
}
