package core

import "math/rand"

const (
	TimingQualityMiss = iota
	TimingQualityNice
	TimingQualityGood
	TimingQualityGreat
	TimingQualityExcellent
	// TimingQualityCount is the number of timing grades. Used by the
	// three parallel grade tables (timingGrades in config.go,
	// qualityVisuals in render/timing.go, gradeSounds in
	// battle/battle.go) to assert length parity at init — adding a new
	// grade requires extending all three or a startup panic catches the
	// drift.
	TimingQualityCount
)

// TimingKind picks how the bar grades input.
//
//	Press:    hit the input once during a window.
//	Charge:   hold through three ticks, release at the peak.
//	Sequence: tap a randomized run of directions in order before time's up.
const (
	TimingKindPress    = 0
	TimingKindCharge   = 1
	TimingKindSequence = 2
)

// Sequence-minigame direction codes. Stored in TimingState.SequenceTargets
// at NewSequenceState time; matched against player input at runtime.
const (
	SeqDirUp    = 0
	SeqDirRight = 1
	SeqDirDown  = 2
	SeqDirLeft  = 3
)

// Per-slot result for the sequence minigame. SequenceResults is a slice
// parallel to SequenceTargets — one entry per arrow.
const (
	SeqResultPending = 0
	SeqResultCorrect = 1
	SeqResultWrong   = 2
)

// TimingState drives a single instance of the "timed hit" minigame. It owns
// the elapsed time, the acceptance window, and the resolved quality. Input
// handling and rendering are external — this struct is pure state so the same
// type powers attack, defend, and charge bars alike.
//
// Field semantics by kind:
//
//	Press:
//	  WindowStart..WindowEnd is the green acceptance window.
//	  SweetSpot is the centered "Excellent" tick.
//	  Pressed is true once the input fired.
//
//	Charge:
//	  WindowStart..WindowEnd is the peak release window (right after the 3rd
//	  tick lands). SweetSpot is the centered "Excellent" point inside it.
//	  Pressed is true once the player ever held the input.
//	  Released is true once they let go (or the bar timed out while held).
type TimingState struct {
	Kind        int
	Active      bool
	Resolved    bool
	Pressed     bool
	Released    bool
	Duration    float32
	Elapsed     float32
	WindowStart float32
	WindowEnd   float32
	SweetSpot   float32
	Quality     int

	// Window2Start / Window2End / SweetSpot2 are the optional SECOND press
	// acceptance window for multi-zone press bars (e.g. Swipe — two hit
	// zones across the same sweep). Window2End == 0 means "single-window
	// press bar", which is the default for NewTimingState. NewDoublePressState
	// is the constructor that populates these. Charge / sequence bars
	// ignore them.
	Window2Start float32
	Window2End   float32
	SweetSpot2   float32

	// Sequence-kind state (unused for press/charge). Targets is the random
	// run of directions; Cursor points at the next slot to fill; Results is
	// parallel to Targets and holds Pending / Correct / Wrong per slot.
	SequenceTargets []int
	SequenceResults []int
	SequenceCursor  int
}

// NewTimingState builds a freshly-armed press-kind bar. The bar sweeps for
// the given duration. Window position is randomized each time the bar arms
// (in [PressWindowMinStart, PressWindowMaxStart] of bar duration) so the
// player can't muscle-memory the press point — but it never opens before
// PressWindowMinStart, so they always get a moment of "approaching, not
// yet" before a hit is possible. Width is fixed at PressWindowWidth and
// the sweet spot sits in the center.
func NewTimingState(rng *rand.Rand, duration float32) TimingState {
	if duration <= 0 {
		duration = AttackTimingDuration
	}
	start, end, sweet := randomizedPressWindow(rng, PressWindow.MinStart, PressWindow.MaxStart, PressWindow.Width, PressWindow.MaxEnd)
	return TimingState{
		Kind:        TimingKindPress,
		Active:      true,
		Duration:    duration,
		WindowStart: start * duration,
		WindowEnd:   end * duration,
		SweetSpot:   sweet * duration,
	}
}

// NewDoublePressState builds a press-kind bar with TWO acceptance windows
// — one in each half of the sweep. Used by skills like Swipe where the
// player sees two distinct hit zones and presses inside either one. Each
// window grades independently (full Miss → Excellent ladder) so picking
// the closer zone never penalizes the player.
//
// Window 1 randomizes inside DoublePressWindow.Window1, window 2 inside
// DoublePressWindow.Window2 — never overlapping, with a guaranteed gap
// between them so the player reads "two zones" instead of "wide zone".
func NewDoublePressState(rng *rand.Rand, duration float32) TimingState {
	if duration <= 0 {
		duration = AttackTimingDuration
	}
	w1Start, w1End, w1Sweet := randomizedPressWindow(rng, DoublePressWindow.Window1MinStart, DoublePressWindow.Window1MaxStart, DoublePressWindow.Width, DoublePressWindow.Window1MaxEnd)
	w2Start, w2End, w2Sweet := randomizedPressWindow(rng, DoublePressWindow.Window2MinStart, DoublePressWindow.Window2MaxStart, DoublePressWindow.Width, DoublePressWindow.Window2MaxEnd)
	return TimingState{
		Kind:         TimingKindPress,
		Active:       true,
		Duration:     duration,
		WindowStart:  w1Start * duration,
		WindowEnd:    w1End * duration,
		SweetSpot:    w1Sweet * duration,
		Window2Start: w2Start * duration,
		Window2End:   w2End * duration,
		SweetSpot2:   w2Sweet * duration,
	}
}

// randomizedPressWindow returns (start, end, sweet) fractions for a press
// window placed inside [minStart, maxStart] with the fixed `width`. If the
// roll would push the window past `maxEnd` it slides back so the full
// width still fits. Shared by NewTimingState and NewDoublePressState so
// the slide-to-fit rule lives in one place.
func randomizedPressWindow(rng *rand.Rand, minStart, maxStart, width, maxEnd float32) (start, end, sweet float32) {
	span := maxStart - minStart
	if span < 0 {
		span = 0
	}
	start = minStart + float32(rng.Float64())*span
	end = start + width
	if end > maxEnd {
		end = maxEnd
		start = end - width
	}
	sweet = (start + end) * 0.5
	return
}

// chargeSegments is the piecewise-linear curve mapping a charge bar's
// elapsed-time fraction [0, 1] to its visual cursor fraction [0, 1].
// Each row is the (visual, elapsed) breakpoint at the end of a segment;
// the cursor interpolates linearly between adjacent rows. Visual slope
// (visual_span / elapsed_span) strictly increases through the three
// tick segments and into the peak — that's the cursor accelerating with
// every notch the player picks up.
//
// Tick lines and peak band sit at constant visual quarters (0.25 / 0.50
// / 0.75 / 0.85 — see ChargeTickNPct in config.go); the *elapsed* values
// here are what stretch and squeeze the cursor's speed across them.
// Touch one row and both render and grade follow.
var chargeSegments = [...]struct {
	Visual, Elapsed float32
}{
	{0.00, 0.00},
	{ChargeTick1Pct, 0.45},  // segment 0 — slow lead-in (slope ≈ 0.56)
	{ChargeTick2Pct, 0.70},  // segment 1 — speeds up (slope ≈ 1.00)
	{ChargeTick3Pct, 0.88},  // segment 2 — faster yet (slope ≈ 1.39)
	{ChargePeakEnd, 0.94},   // segment 3 — peak window (slope ≈ 1.67)
	{1.00, 1.00},            // segment 4 — decay, past the player's reach
}

// ChargeCursorProgress maps a charge bar's elapsed time to its visual
// cursor position [0, 1]. Non-linear via chargeSegments — the cursor
// accelerates through each segment. Single source of truth for both
// rendering (cursor X, tick line / peak / decay band positions) and
// grading (resolveCharge compares visual progress to the visual tick
// constants).
func ChargeCursorProgress(elapsed, duration float32) float32 {
	if duration <= 0 || elapsed <= 0 {
		return 0
	}
	t := elapsed / duration
	if t >= 1 {
		return 1
	}
	for i := 1; i < len(chargeSegments); i++ {
		cur := chargeSegments[i]
		if t <= cur.Elapsed {
			prev := chargeSegments[i-1]
			span := cur.Elapsed - prev.Elapsed
			if span <= 0 {
				return cur.Visual
			}
			frac := (t - prev.Elapsed) / span
			return prev.Visual + frac*(cur.Visual-prev.Visual)
		}
	}
	return 1
}

// ChargeElapsedForVisual is the inverse of ChargeCursorProgress: given a
// target visual cursor position [0, 1], returns the elapsed time at which
// the cursor reaches that visual position. Render code uses this to ask
// "when did the cursor cross this tick" without re-solving the curve.
func ChargeElapsedForVisual(visual, duration float32) float32 {
	if duration <= 0 || visual <= 0 {
		return 0
	}
	if visual >= 1 {
		return duration
	}
	for i := 1; i < len(chargeSegments); i++ {
		cur := chargeSegments[i]
		if visual <= cur.Visual {
			prev := chargeSegments[i-1]
			span := cur.Visual - prev.Visual
			if span <= 0 {
				return cur.Elapsed * duration
			}
			frac := (visual - prev.Visual) / span
			return (prev.Elapsed + frac*(cur.Elapsed-prev.Elapsed)) * duration
		}
	}
	return duration
}

// NewChargeState builds a freshly-armed charge-kind bar. The bar runs for
// `duration` seconds; three tick markers land before the peak window opens
// at ChargePeakStart and closes at ChargePeakEnd. The sweet spot is centered
// in the peak window. WindowStart/End/SweetSpot are stored as elapsed times
// (inverted through chargeSegments) so callers reading TimingState.Elapsed
// against them — e.g., the renderer's "RELEASE!" heading flip — keep their
// linear-elapsed comparisons honest under the non-linear cursor curve.
func NewChargeState(duration float32) TimingState {
	if duration <= 0 {
		duration = ChargeTimingDuration
	}
	return TimingState{
		Kind:        TimingKindCharge,
		Active:      true,
		Duration:    duration,
		WindowStart: ChargeElapsedForVisual(ChargePeakStart, duration),
		WindowEnd:   ChargeElapsedForVisual(ChargePeakEnd, duration),
		SweetSpot:   ChargeElapsedForVisual((ChargePeakStart+ChargePeakEnd)*0.5, duration),
	}
}

// NewSequenceState builds a freshly-armed sequence-kind bar with `length`
// random directional arrows. Player has `duration` seconds to tap them all
// in order; pending/wrong slots drop the grade.
func NewSequenceState(rng *rand.Rand, duration float32, length int) TimingState {
	if duration <= 0 {
		duration = StealTimingDuration
	}
	if length <= 0 {
		length = StealSequenceLength
	}
	targets := make([]int, length)
	for i := range targets {
		targets[i] = rng.Intn(4)
	}
	return TimingState{
		Kind:            TimingKindSequence,
		Active:          true,
		Duration:        duration,
		SequenceTargets: targets,
		SequenceResults: make([]int, length),
	}
}

// Tick advances the bar by dt. For press bars, auto-resolves a Miss if the
// window passes without a press. For charge bars, auto-resolves at duration
// (graded by where Elapsed lands) — held-too-long produces a Miss/Nice.
func (t *TimingState) Tick(dt float32) {
	if !t.Active || t.Resolved {
		return
	}
	t.Elapsed += dt
	if t.Elapsed < t.Duration {
		return
	}
	switch t.Kind {
	case TimingKindCharge:
		// Bar timed out. If they engaged at all, treat as released-late;
		// otherwise it's just a Miss.
		if t.Pressed {
			t.Released = true
			t.resolveCharge()
		} else {
			t.Quality = TimingQualityMiss
			t.Resolved = true
		}
	case TimingKindSequence:
		// Time's up — pending slots count as wrong, grade what we have.
		t.resolveSequence()
	default:
		// Press bar timed out without a press — auto-Miss, regardless of
		// how many windows the bar carried.
		t.Resolved = true
		t.Quality = TimingQualityMiss
	}
}

// Press records a press-kind input. Returns true if this press resolved the
// bar. For charge-kind bars, use Hold/Release instead.
//
// For multi-zone press bars (Window2End > 0), the press grades against
// whichever acceptance window the cursor is currently in — Miss if it's
// outside both. Each window has its own sweet spot and gradient bands;
// the player isn't penalized for aiming at the closer zone.
func (t *TimingState) Press() bool {
	if !t.Active || t.Resolved || t.Pressed {
		return false
	}
	if t.Kind != TimingKindPress {
		return false
	}
	t.Pressed = true
	if start, end, sweet, ok := t.activePressWindow(); ok {
		t.resolveInWindow(start, end, sweet)
	} else {
		t.Resolved = true
		t.Quality = TimingQualityMiss
	}
	return true
}

// activePressWindow returns the (start, end, sweet) of the press window
// the cursor is currently inside, with ok=false if neither window
// contains the cursor. For single-zone bars, only the primary window is
// considered; for double-zone bars, both are checked in left-to-right
// order so a window that happened to overlap (shouldn't, per config
// guards) still grades against the first one the cursor entered.
func (t TimingState) activePressWindow() (start, end, sweet float32, ok bool) {
	if t.Elapsed >= t.WindowStart && t.Elapsed <= t.WindowEnd {
		return t.WindowStart, t.WindowEnd, t.SweetSpot, true
	}
	if t.Window2End > 0 && t.Elapsed >= t.Window2Start && t.Elapsed <= t.Window2End {
		return t.Window2Start, t.Window2End, t.SweetSpot2, true
	}
	return 0, 0, 0, false
}

// Hold marks the charge bar as engaged. Idempotent — sets Pressed=true on
// the first held frame and stays. No-op for press-kind bars.
func (t *TimingState) Hold() {
	if !t.Active || t.Resolved {
		return
	}
	if t.Kind != TimingKindCharge {
		return
	}
	t.Pressed = true
}

// Release closes a charge bar. Returns true if this release resolved the bar.
// Releasing without ever having Held does nothing — you can't release what
// you never picked up. No-op for press-kind bars.
func (t *TimingState) Release() bool {
	if !t.Active || t.Resolved {
		return false
	}
	if t.Kind != TimingKindCharge || !t.Pressed {
		return false
	}
	t.Released = true
	t.resolveCharge()
	return true
}

// SequenceInput records a directional press at the current cursor slot.
// Marks that slot Correct or Wrong, advances the cursor, and resolves when
// the last slot is filled. Returns true if this input resolved the bar.
func (t *TimingState) SequenceInput(dir int) bool {
	if !t.Active || t.Resolved {
		return false
	}
	if t.Kind != TimingKindSequence {
		return false
	}
	if t.SequenceCursor >= len(t.SequenceTargets) {
		return false
	}
	t.Pressed = true
	if dir == t.SequenceTargets[t.SequenceCursor] {
		t.SequenceResults[t.SequenceCursor] = SeqResultCorrect
	} else {
		t.SequenceResults[t.SequenceCursor] = SeqResultWrong
	}
	t.SequenceCursor++
	if t.SequenceCursor >= len(t.SequenceTargets) {
		t.resolveSequence()
		return true
	}
	return false
}

// resolveSequence grades the pickpocket pattern. Baseline is Excellent; each
// non-correct slot (wrong key OR pending/timed-out) drops one grade. Finishing
// all-correct under StealFastThreshold bumps grade by one (capped Excellent).
func (t *TimingState) resolveSequence() {
	t.Resolved = true
	correctCount := 0
	for _, r := range t.SequenceResults {
		if r == SeqResultCorrect {
			correctCount++
		}
	}
	wrongCount := len(t.SequenceTargets) - correctCount
	grade := TimingQualityExcellent - wrongCount
	if wrongCount == 0 && t.Elapsed < StealFastThreshold {
		grade++
	}
	if grade < TimingQualityMiss {
		grade = TimingQualityMiss
	}
	if grade > TimingQualityExcellent {
		grade = TimingQualityExcellent
	}
	t.Quality = grade
}

// resolveCharge grades a charge release by counting how many tick markers
// the cursor crossed (visually) before the player let go. Each tick is a
// discrete grade jump, so a release that lands one frame before the third
// tick scores the same as one that lands halfway between the second and
// third. The peak window (3 ticks completed) is split into Great vs
// Excellent based on closeness to the sweet spot. Past the peak window
// the release decays straight to Miss — over-charging isn't rewarded.
//
// Grading reads the cursor's *visual* position (via ChargeCursorProgress),
// not raw Elapsed, so the non-linear acceleration curve drives both what
// the player sees and what they're scored against — no drift between the
// cursor crossing a tick line on screen and the bar awarding that tick.
//
// Grade dispatch (p = cursor visual progress):
//
//	0 ticks crossed (p < ChargeTick1Pct):                          Miss
//	1 tick crossed  (ChargeTick1Pct <= p < ChargeTick2Pct):        Nice
//	2 ticks crossed (ChargeTick2Pct <= p < ChargeTick3Pct):        Good
//	3 ticks crossed in peak window (p <= ChargePeakEnd):           Great or Excellent
//	past peak window:                                              Miss
func (t *TimingState) resolveCharge() {
	t.Resolved = true
	p := ChargeCursorProgress(t.Elapsed, t.Duration)

	switch {
	case p < ChargeTick1Pct:
		t.Quality = TimingQualityMiss
	case p < ChargeTick2Pct:
		t.Quality = TimingQualityNice
	case p < ChargeTick3Pct:
		t.Quality = TimingQualityGood
	case p <= ChargePeakEnd:
		// In the peak window — split Great vs Excellent on sweet-spot proximity.
		sweet := (ChargePeakStart + ChargePeakEnd) * 0.5
		distance := p - sweet
		if distance < 0 {
			distance = -distance
		}
		windowSize := ChargePeakEnd - ChargePeakStart
		if windowSize <= 0 || distance/windowSize <= 0.30 {
			t.Quality = TimingQualityExcellent
		} else {
			t.Quality = TimingQualityGreat
		}
	default:
		// Past the peak — they held too long.
		t.Quality = TimingQualityMiss
	}
}

// resolveInWindow grades a press against an explicit (start, end, sweet)
// window. Distance from the sweet spot, normalized by the window's
// half-width, picks a grade off pressGradeBands. Callers must verify the
// cursor is inside the window first — an out-of-window press should be
// resolved as Miss by the caller, not by passing a zero-width window
// (that path quietly returns Nice).
func (t *TimingState) resolveInWindow(start, end, sweet float32) {
	t.Resolved = true
	distance := t.Elapsed - sweet
	if distance < 0 {
		distance = -distance
	}
	windowSize := end - start
	if windowSize <= 0 {
		t.Quality = TimingQualityNice
		return
	}
	t.Quality = gradeFromRatio(distance / windowSize)
}

// Progress returns the current sweep position in [0, 1]. For charge bars
// this runs through ChargeCursorProgress so the cursor accelerates with
// each notch; for press / sequence bars it's a straight Elapsed/Duration.
func (t TimingState) Progress() float32 {
	if t.Duration <= 0 {
		return 0
	}
	if t.Kind == TimingKindCharge {
		return ChargeCursorProgress(t.Elapsed, t.Duration)
	}
	p := t.Elapsed / t.Duration
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}

// TimingBonusMult is the offensive damage multiplier for an attack quality.
// Indexes into timingGrades (config.go); out-of-range qualities fall back
// to the Miss multiplier (1.0×).
func TimingBonusMult(quality int) float32 {
	if quality < 0 || quality >= len(timingGrades) {
		return timingGrades[TimingQualityMiss].Atk
	}
	return timingGrades[quality].Atk
}

// TimingDefenseMult is the incoming damage multiplier for a defend quality.
// Lower is better; Excellent quarters incoming damage, Miss takes the full
// hit. Indexes into timingGrades (config.go); out-of-range qualities fall
// back to the Miss multiplier (1.0×).
func TimingDefenseMult(quality int) float32 {
	if quality < 0 || quality >= len(timingGrades) {
		return timingGrades[TimingQualityMiss].Def
	}
	return timingGrades[quality].Def
}

// pressGradeBands maps the sweet-spot-distance ratio (in units of the
// acceptance window half-width) onto a grade. Single source of truth for
// both resolve() and PreviewQuality so the cursor's live preview and the
// actually-scored grade never desync. A balance pass that wants different
// bands changes this table and both call sites follow.
var pressGradeBands = []struct {
	maxRatio float32
	grade    int
}{
	{0.05, TimingQualityExcellent},
	{0.15, TimingQualityGreat},
	{0.30, TimingQualityGood},
}

// gradeFromRatio looks up the press-bar grade for a normalized
// sweet-spot-distance ratio. Anything past the largest band falls
// through to Nice (still inside the window) — the caller is responsible
// for testing window membership first; a ratio passed in for an
// out-of-window press still grades as a Miss by the caller's gate.
func gradeFromRatio(ratio float32) int {
	for _, b := range pressGradeBands {
		if ratio <= b.maxRatio {
			return b.grade
		}
	}
	return TimingQualityNice
}

// PreviewQuality returns the grade a press-kind bar would score if the player
// pressed right now. Used by the renderer to live-color the cursor so the
// player sees their potential grade approaching — Excellent shimmers as they
// near the sweet spot, slips to Great, then Good, etc. Returns Miss when the
// cursor is outside every acceptance window (or the bar isn't a press kind).
// Routes through activePressWindow so single-zone and double-zone bars share
// one grading path.
func (t TimingState) PreviewQuality() int {
	if t.Kind != TimingKindPress {
		return TimingQualityMiss
	}
	start, end, sweet, ok := t.activePressWindow()
	if !ok {
		return TimingQualityMiss
	}
	windowSize := end - start
	if windowSize <= 0 {
		return TimingQualityNice
	}
	distance := t.Elapsed - sweet
	if distance < 0 {
		distance = -distance
	}
	return gradeFromRatio(distance / windowSize)
}

// HitStopFor returns the freeze-frame duration after a graded press lands.
// Misses / Nice / Good get no hit-stop — the action just flows. Great gets a
// short punch, Excellent gets a longer one. Caller (battle/battle.go's
// tickFlashHold) uses this to delay the apply step so the bar's flash hold
// chains into a true world-pause before damage lands.
func HitStopFor(quality int) float32 {
	switch quality {
	case TimingQualityExcellent:
		return HitStopExcellent
	case TimingQualityGreat:
		return HitStopGreat
	}
	return 0
}

// TimingQualityLabel returns the popup text for a quality grade. Reads
// from timingGrades (config.go); out-of-range values fall back to the
// Miss label so a future grade addition is one table-row edit.
func TimingQualityLabel(quality int) string {
	if quality < 0 || quality >= len(timingGrades) {
		return timingGrades[TimingQualityMiss].Label
	}
	return timingGrades[quality].Label
}

// ScaleDamage applies an attack quality's multiplier to a base damage amount.
// On Excellent, the result is guaranteed strictly greater than the base
// (base+1 if the multiplier would otherwise round down to <=base), so an
// "Excellent" never reads as a non-improvement over a Miss. Other grades
// can round to 0 on tiny bases — that's by design (a Nice on a 1-damage
// swing shouldn't print "1 damage" same as a Miss).
func ScaleDamage(base int, quality int) int {
	scaled := int(float32(base) * TimingBonusMult(quality))
	if quality == TimingQualityExcellent && scaled <= base {
		scaled = base + 1
	}
	if scaled < 0 {
		scaled = 0
	}
	return scaled
}

// ScaleHeal applies an attack quality's multiplier to a base heal amount.
func ScaleHeal(base int, quality int) int {
	scaled := int(float32(base) * TimingBonusMult(quality))
	if scaled < 0 {
		scaled = 0
	}
	return scaled
}

// ScaleIncomingDamage applies a defend quality's multiplier to incoming damage.
// A defended hit cannot become heal — clamps at 0. Integer truncation means
// a Great/Excellent block on a small hit (e.g. base 3 × 0.25 = 0.75 → 0) can
// fully zero the damage; that's intentional, matches how a successful
// timed block reads in-game ("0 damage" with the block flourish).
func ScaleIncomingDamage(base int, quality int) int {
	scaled := int(float32(base) * TimingDefenseMult(quality))
	if scaled < 0 {
		scaled = 0
	}
	return scaled
}
