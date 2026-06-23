package core

import (
	"math"
	"math/rand"
)

const (
	TimingQualityMiss = iota
	TimingQualityNice
	TimingQualityGood
	TimingQualityGreat
	TimingQualityExcellent
	// TimingQualityCount: parallel grade tables (timingGrades config.go,
	// qualityVisuals render/timing.go, gradeSounds battle/battle.go) assert
	// length parity at init against this — extend all three or panic.
	TimingQualityCount
)

// TimingKind picks how the bar grades input.
//
//	Press:      hit the input once during a window.
//	Charge:     hold through three ticks, release at the peak.
//	Sequence:   tap a randomized run of directions in order before time's up.
//	Reels:      slot-machine — stop each spinning reel; matches pay off.
//	Recall:     memory — a directional pattern shows then hides; reproduce it.
//	Overcharge: a charge bar with a post-peak OVERLOAD band (bonus + recoil).
const (
	TimingKindPress = iota
	TimingKindCharge
	TimingKindSequence
	TimingKindReels
	TimingKindRecall
	TimingKindOvercharge
)

func init() {
	// SkillMinigame (party.go) and TimingKind are mapped 1:1 in
	// battle.beginPendingAction; assert value-alignment so a reorder panics.
	if int(MinigamePress) != TimingKindPress ||
		int(MinigameCharge) != TimingKindCharge ||
		int(MinigameSequence) != TimingKindSequence ||
		int(MinigameReels) != TimingKindReels ||
		int(MinigameRecall) != TimingKindRecall ||
		int(MinigameOvercharge) != TimingKindOvercharge {
		panic("core: SkillMinigame and TimingKind enums have drifted out of lockstep")
	}
}

// Sequence-minigame direction codes, stored in TimingState.SequenceTargets.
const (
	SeqDirUp    = 0
	SeqDirRight = 1
	SeqDirDown  = 2
	SeqDirLeft  = 3
	SeqDirCount = 4 // range NewSequenceState draws from
)

func init() {
	// SeqDir* and the cardinal facings (config.go) MUST share order + int
	// values so directional code reads either set; assert the parity.
	if int(SeqDirCount) != int(FacingCount) ||
		int(SeqDirUp) != int(North) ||
		int(SeqDirRight) != int(East) ||
		int(SeqDirDown) != int(South) ||
		int(SeqDirLeft) != int(West) {
		panic("core: SeqDir* and cardinal facing enums have drifted out of lockstep")
	}
}

// Per-slot result for the sequence minigame (parallel to SequenceTargets).
const (
	SeqResultPending = iota
	SeqResultCorrect
	SeqResultWrong
)

// TimingState is the pure state of one "timed hit" minigame instance (input
// + rendering are external), so the same type powers attack/defend/charge.
// Field semantics by kind:
//
//	Press:  WindowStart..WindowEnd = accept window; SweetSpot = centered
//	        "Excellent" tick; Pressed = input fired.
//	Charge: WindowStart..WindowEnd = peak release window (after 3rd tick);
//	        SweetSpot = centered Excellent; Pressed = ever held;
//	        Released = let go (or timed out while held).

// TallyWindow is one accept zone in a multi-press tally bar. Hit marks the
// zone consumed (only the first press in an unconsumed window scores). Sweet
// is the in-window peak for the renderer (tally mode grades equally per hit).
// FlashTimer = per-window hit-feedback hold, set to TallyHitFlashDuration on
// the landing press, decayed each Tick.
type TallyWindow struct {
	Start      float32
	End        float32
	Sweet      float32
	Hit        bool
	FlashTimer float32
}

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

	// Multi-press tally mode (Windows non-nil): each press in an unconsumed
	// accept window increments Hits + consumes it; a press in the commit zone
	// (CommitStart..Duration) or timeout resolves with the tally. Quality
	// maps from Hits (0=Miss, partial=Good, all=Excellent); callers wanting
	// per-hit scaling read Hits directly. Windows are Start-ascending and
	// never overlap each other or the commit zone (see NewMultiPressState).
	Windows     []TallyWindow
	Hits        int
	CommitStart float32

	// Sequence-kind state (also reused by Recall). Targets = random direction
	// run; Cursor = next slot to fill; Results parallel to Targets.
	SequenceTargets []int
	SequenceResults []int
	SequenceCursor  int

	// Reels-kind state. A press locks the next still-spinning reel on its
	// current symbol; resolveReels grades by the largest matching set.
	Reels []Reel

	// Recall-kind: how long the pattern stays face-up before input opens.
	RevealTime float32

	// Overcharge-kind: set by resolveOvercharge on a past-peak release; the
	// apply path reads it to grant the bonus + recoil.
	Overloaded bool
}

// IsTallyMode reports whether this is a multi-press tally bar.
func (t TimingState) IsTallyMode() bool {
	return t.Kind == TimingKindPress && len(t.Windows) > 0
}

// NewTimingState builds a freshly-armed press-kind bar. Window start is
// randomized in [PressWindow.MinStart, MaxStart] (fraction of duration) so the
// press point can't be muscle-memoried; width fixed at PressWindow.Width.
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

// NewMultiPressState builds a tally-mode press bar with `count` evenly-spaced
// accept windows + a late commit zone. count <= 0 falls back to one window.
func NewMultiPressState(rng *rand.Rand, duration float32, count int) TimingState {
	if duration <= 0 {
		duration = AttackTimingDuration
	}
	if count < 1 {
		count = 1
	}
	commitZoneFrac := MultiPressWindow.CommitZoneFrac
	commitStart := 1.0 - commitZoneFrac
	// Windows distributed across [leadIn, commitStart - gap]; geometry config
	// lives in MultiPressWindow (config.go).
	leadIn := MultiPressWindow.LeadInFrac
	winWidth := MultiPressWindow.WindowWidthFrac
	span := commitStart - leadIn - MultiPressWindow.CommitGapFrac // breathing room before commit
	if span < winWidth {
		span = winWidth
	}
	windows := make([]TallyWindow, count)
	usable := span - winWidth
	for i := 0; i < count; i++ {
		var center float32
		if count == 1 {
			center = leadIn + span*0.5
		} else {
			center = leadIn + winWidth*0.5 + usable*(float32(i)/float32(count-1))
		}
		// Small jitter so consecutive bars aren't identically placed.
		jitter := (float32(rng.Float64())*2 - 1) * (winWidth * MultiPressWindow.JitterFrac)
		center += jitter
		// Clamp so a jittered window never goes negative or crosses CommitStart
		// (a commit-zone press resolves the bar early, making it unreachable).
		if lo := winWidth * 0.5; center < lo {
			center = lo
		}
		if hi := commitStart - winWidth*0.5; center > hi {
			center = hi
		}
		start := center - winWidth*0.5
		end := center + winWidth*0.5
		windows[i] = TallyWindow{
			Start: start * duration,
			End:   end * duration,
			Sweet: center * duration,
		}
	}
	return TimingState{
		Kind:        TimingKindPress,
		Active:      true,
		Duration:    duration,
		Windows:     windows,
		CommitStart: commitStart * duration,
	}
}

// NewTallyStateAtCenters builds a tally-mode press bar with windows hand-placed
// at the given fractional centers (0..1). Each keeps WindowWidthFrac width and
// is clamped to [winWidth/2, commitStart-winWidth/2] so it stays pressable.
func NewTallyStateAtCenters(duration float32, centers ...float32) TimingState {
	if duration <= 0 {
		duration = AttackTimingDuration
	}
	commitStart := 1.0 - MultiPressWindow.CommitZoneFrac
	winWidth := MultiPressWindow.WindowWidthFrac
	lo := winWidth * 0.5
	hi := commitStart - winWidth*0.5
	if hi < lo {
		hi = lo
	}
	windows := make([]TallyWindow, 0, len(centers))
	for _, c := range centers {
		if c < lo {
			c = lo
		}
		if c > hi {
			c = hi
		}
		windows = append(windows, TallyWindow{
			Start: (c - winWidth*0.5) * duration,
			End:   (c + winWidth*0.5) * duration,
			Sweet: c * duration,
		})
	}
	return TimingState{
		Kind:        TimingKindPress,
		Active:      true,
		Duration:    duration,
		Windows:     windows,
		CommitStart: commitStart * duration,
	}
}

// randomizedPressWindow returns (start, end, sweet) fractions for a width-`width`
// window in [minStart, maxStart], slid back so it never exceeds maxEnd.
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

// chargePeakSweet is the peak window midpoint (Excellent sweet spot), shared by
// NewChargeState and chargeGradeUpToPeak so arm + grade can't drift.
const chargePeakSweet = (ChargePeakStart + ChargePeakEnd) * 0.5

// chargeSegments is the piecewise-linear curve mapping charge elapsed fraction
// [0,1] to visual cursor fraction [0,1]. Each row is the (visual, elapsed)
// breakpoint ending a segment; visual slope strictly increases (cursor
// accelerates). Single source for both render and grade.
var chargeSegments = [...]struct {
	Visual, Elapsed float32
}{
	{0.00, 0.00},
	{ChargeTick1Pct, 0.45},
	{ChargeTick2Pct, 0.70},
	{ChargeTick3Pct, 0.88},
	{ChargePeakEnd, 0.94}, // peak window
	{1.00, 1.00},          // decay, past the player's reach
}

// ChargeCursorProgress maps charge elapsed time to visual cursor position [0,1]
// (non-linear via chargeSegments). Source of truth for render + grade.
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

// ChargeElapsedForVisual is the inverse of ChargeCursorProgress: the elapsed
// time at which the cursor reaches visual position [0,1].
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

// NewChargeState builds a freshly-armed charge-kind bar. WindowStart/End/
// SweetSpot are stored as elapsed times (inverted through chargeSegments) so
// comparisons against TimingState.Elapsed stay honest under the non-linear curve.
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
		SweetSpot:   ChargeElapsedForVisual(chargePeakSweet, duration),
	}
}

// randomDirectionRun builds a length-N slice of random directions ([0,SeqDirCount)).
// Shared by the Sequence and Recall constructors.
func randomDirectionRun(rng *rand.Rand, length int) []int {
	targets := make([]int, length)
	for i := range targets {
		targets[i] = rng.Intn(SeqDirCount)
	}
	return targets
}

// NewSequenceState builds a sequence-kind bar with `length` random arrows to
// tap in order; pending/wrong slots drop the grade.
func NewSequenceState(rng *rand.Rand, duration float32, length int) TimingState {
	if duration <= 0 {
		duration = SequenceTimingDuration
	}
	if length <= 0 {
		length = SequenceLength
	}
	targets := randomDirectionRun(rng, length)
	return TimingState{
		Kind:            TimingKindSequence,
		Active:          true,
		Duration:        duration,
		SequenceTargets: targets,
		SequenceResults: make([]int, length),
	}
}

// Reel is one spinner in the Reels minigame. Speed = symbol cadence
// (symbols/sec), Offset = starting symbol, Stop = locked symbol (-1 while spinning).
type Reel struct {
	Speed  float32
	Offset int
	Stop   int
}

// NewReelState builds a slot-machine bar: ReelCount reels cycling
// ReelSymbolCount symbols at desynced speeds; resolves once all stopped.
func NewReelState(rng *rand.Rand, duration float32) TimingState {
	if duration <= 0 {
		duration = ReelTimingDuration
	}
	reels := make([]Reel, ReelCount)
	for i := range reels {
		reels[i] = Reel{
			// Desynced speeds so no single beat jackpots every reel.
			Speed:  ReelSpinMin + float32(rng.Float64())*(ReelSpinMax-ReelSpinMin),
			Offset: rng.Intn(ReelSymbolCount),
			Stop:   -1,
		}
	}
	return TimingState{
		Kind:     TimingKindReels,
		Active:   true,
		Duration: duration,
		Reels:    reels,
	}
}

// ReelSymbolAt returns reel i's current symbol: its locked value if stopped,
// else the spinning value from elapsed/speed/offset.
func (t TimingState) ReelSymbolAt(i int) int {
	if i < 0 || i >= len(t.Reels) {
		return 0
	}
	r := t.Reels[i]
	if r.Stop >= 0 {
		return r.Stop
	}
	// Round (not floor) so the locked symbol is the one centred on the
	// pay-line. Euclidean mod guards a negative phase from out-of-range indexing.
	sym := int(math.Round(float64(t.reelPhase(r))))
	return WrapIndex(sym, ReelSymbolCount)
}

// reelPhase is reel r's continuous scroll position in symbols (offset + elapsed
// spin distance, floored at 0).
func (t TimingState) reelPhase(r Reel) float32 {
	p := float32(r.Offset) + t.Elapsed*r.Speed
	if p < 0 {
		return 0
	}
	return p
}

// ReelPhaseAt returns reel i's scroll phase for the render strip draw.
// Meaningless for a stopped reel — callers gate on Reels[i].Stop < 0.
func (t TimingState) ReelPhaseAt(i int) float32 {
	if i < 0 || i >= len(t.Reels) {
		return 0
	}
	return t.reelPhase(t.Reels[i])
}

// StopNextReel locks the next still-spinning reel onto its current symbol.
// Returns true if this stop resolved the bar. Reels-kind only.
func (t *TimingState) StopNextReel() bool {
	if !t.Active || t.Resolved || t.Kind != TimingKindReels {
		return false
	}
	for i := range t.Reels {
		if t.Reels[i].Stop >= 0 {
			continue
		}
		t.Reels[i].Stop = t.ReelSymbolAt(i)
		t.Pressed = true
		if t.allReelsStopped() {
			t.resolveReels()
			return true
		}
		return false
	}
	return false
}

func (t TimingState) allReelsStopped() bool {
	for i := range t.Reels {
		if t.Reels[i].Stop < 0 {
			return false
		}
	}
	return true
}

// resolveReels finalises the slot result. 3 match = Excellent, 2 = Good, all
// distinct = Miss. A no-play timeout (!Pressed) is Miss outright — else the
// random auto-locked symbols could hand a walk-away player a boosted result.
func (t *TimingState) resolveReels() {
	// Only player-stopped reels count toward the match; snapshot them before
	// auto-locking the rest (which lock for display only).
	stopped := make([]Reel, 0, len(t.Reels))
	for i := range t.Reels {
		if t.Reels[i].Stop >= 0 {
			stopped = append(stopped, t.Reels[i])
		}
	}
	for i := range t.Reels {
		if t.Reels[i].Stop < 0 {
			t.Reels[i].Stop = t.ReelSymbolAt(i)
		}
	}
	t.Resolved = true
	if !t.Pressed {
		t.Quality = TimingQualityMiss
		return
	}
	switch largestReelMatch(stopped) {
	case 3:
		t.Quality = TimingQualityExcellent
	case 2:
		t.Quality = TimingQualityGood
	default:
		t.Quality = TimingQualityMiss
	}
}

// largestReelMatch returns the size of the most-repeated locked symbol.
func largestReelMatch(reels []Reel) int {
	best := 0
	for i := range reels {
		c := 0
		for j := range reels {
			if reels[j].Stop == reels[i].Stop {
				c++
			}
		}
		if c > best {
			best = c
		}
	}
	return best
}

// NewRecallState builds a memory-pattern bar: a random run shows for `reveal`
// seconds then hides; the player reproduces it. Reuses Sequence* fields +
// resolveSequence; differs only in the reveal-then-hide phase (RecallHidden).
func NewRecallState(rng *rand.Rand, duration float32, length int, reveal float32) TimingState {
	if duration <= 0 {
		duration = RecallTimingDuration
	}
	if length <= 0 {
		length = RecallPatternLength
	}
	if reveal <= 0 {
		reveal = RecallRevealTime
	}
	// Reveal MUST end before the bar does or there's no input window; cap it.
	if maxReveal := duration * RecallMaxRevealFrac; reveal > maxReveal {
		reveal = maxReveal
	}
	targets := randomDirectionRun(rng, length)
	return TimingState{
		Kind:            TimingKindRecall,
		Active:          true,
		Duration:        duration,
		RevealTime:      reveal,
		SequenceTargets: targets,
		SequenceResults: make([]int, length),
	}
}

// RecallHidden reports whether a recall pattern has hidden (input phase open).
func (t TimingState) RecallHidden() bool {
	return t.Kind == TimingKindRecall && t.Elapsed >= t.RevealTime
}

// NewOverchargeState builds a charge bar with a post-peak OVERLOAD band:
// releasing past the peak overcharges (bonus + recoil) instead of Miss.
// Identical curve to a normal charge; only resolveOvercharge differs.
func NewOverchargeState(duration float32) TimingState {
	t := NewChargeState(duration)
	t.Kind = TimingKindOvercharge
	return t
}

// Tick advances the bar by dt, auto-resolving at timeout (press → Miss,
// charge → graded by where Elapsed lands).
func (t *TimingState) Tick(dt float32) {
	if !t.Active || t.Resolved {
		return
	}
	t.Elapsed += dt
	// Decay per-window hit-flash timers on tally bars.
	if t.IsTallyMode() {
		for i := range t.Windows {
			if t.Windows[i].FlashTimer > 0 {
				t.Windows[i].FlashTimer -= dt
				if t.Windows[i].FlashTimer < 0 {
					t.Windows[i].FlashTimer = 0
				}
			}
		}
	}
	if t.Elapsed < t.Duration {
		return
	}
	switch t.Kind {
	case TimingKindCharge, TimingKindOvercharge:
		// If engaged, treat as released-late (overcharge → overload); else Miss.
		if t.Pressed {
			t.Released = true
			t.resolveChargeKind()
		} else {
			t.Quality = TimingQualityMiss
			t.Resolved = true
		}
	case TimingKindSequence, TimingKindRecall:
		t.resolveSequence()
	case TimingKindReels:
		t.resolveReels()
	default:
		// Press timeout: tally bars resolve with accumulated hits; single Miss.
		if t.IsTallyMode() {
			t.resolveTally()
		} else {
			t.Resolved = true
			t.Quality = TimingQualityMiss
		}
	}
}

// Press records a press-kind input; returns true if it resolved the bar.
// Single-window bars resolve on the first press (in-window graded, else Miss).
// Tally bars accumulate hits via pressTally. Charge bars use Hold/Release.
func (t *TimingState) Press() bool {
	if !t.Active || t.Resolved {
		return false
	}
	if t.Kind != TimingKindPress {
		return false
	}
	if t.IsTallyMode() {
		return t.pressTally()
	}
	if t.Pressed {
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

// pressTally handles one input on a multi-press tally bar: an unconsumed-window
// press adds a hit, a commit-zone press resolves, any other press is a no-op.
func (t *TimingState) pressTally() bool {
	// Commit-zone press resolves early. Mark Pressed so a deliberate commit
	// counts as engagement even at zero hits (only a walk-away stays !Pressed).
	if t.Elapsed >= t.CommitStart {
		t.Pressed = true
		t.resolveTally()
		return true
	}
	// Accept-window press: consume the window the cursor is in.
	for i := range t.Windows {
		w := &t.Windows[i]
		if w.Hit {
			continue
		}
		if t.Elapsed >= w.Start && t.Elapsed <= w.End {
			w.Hit = true
			w.FlashTimer = TallyHitFlashDuration
			t.Hits++
			// All windows landed — resolve now, nothing left to score.
			if t.Hits >= len(t.Windows) {
				t.resolveTally()
				return true
			}
			return false
		}
	}
	return false // outside any window, before commit — no-op
}

// resolveTally finalises a multi-press bar. Quality from Hits: 0 = Miss,
// all = Excellent, one short = Great, fewer = Good (Good only reachable for
// 3+-window bars). Apply paths also read Hits directly for per-hit scaling.
func (t *TimingState) resolveTally() {
	t.Resolved = true
	// Don't clobber a commit-zone press's Pressed; else infer from hits so a
	// no-input timeout stays !Pressed.
	t.Pressed = t.Pressed || t.Hits > 0
	switch {
	case t.Hits == 0:
		t.Quality = TimingQualityMiss
	case t.Hits >= len(t.Windows):
		t.Quality = TimingQualityExcellent
	case t.Hits >= len(t.Windows)-1:
		t.Quality = TimingQualityGreat
	default:
		t.Quality = TimingQualityGood
	}
}

// activePressWindow returns the single acceptance window the cursor is inside
// (ok=false if outside). Single-window bars only.
func (t TimingState) activePressWindow() (start, end, sweet float32, ok bool) {
	if t.Elapsed >= t.WindowStart && t.Elapsed <= t.WindowEnd {
		return t.WindowStart, t.WindowEnd, t.SweetSpot, true
	}
	return 0, 0, 0, false
}

// Hold marks the charge bar engaged (idempotent). No-op for press-kind bars.
func (t *TimingState) Hold() {
	if !t.Active || t.Resolved {
		return
	}
	if !t.isChargeFamily() {
		return
	}
	t.Pressed = true
}

// isChargeFamily reports whether this bar uses the hold/release flow (Charge
// or Overcharge); they differ only in the resolve grader.
func (t TimingState) isChargeFamily() bool {
	return t.Kind == TimingKindCharge || t.Kind == TimingKindOvercharge
}

// Release closes a charge bar; returns true if it resolved. No-op without a
// prior Hold, or for press-kind bars.
func (t *TimingState) Release() bool {
	if !t.Active || t.Resolved {
		return false
	}
	if !t.isChargeFamily() || !t.Pressed {
		return false
	}
	t.Released = true
	t.resolveChargeKind()
	return true
}

// SequenceInput records a directional press at the cursor slot, advancing it
// and resolving when the last slot fills. Returns true if it resolved the bar.
func (t *TimingState) SequenceInput(dir int) bool {
	if !t.Active || t.Resolved {
		return false
	}
	if t.Kind != TimingKindSequence && t.Kind != TimingKindRecall {
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

// resolveSequence grades the pattern: each non-correct slot (wrong or pending)
// drops one grade from Excellent. For TimingKindSequence a flawless run only
// reaches Excellent under SequenceFastThreshold (slow caps at Great). Recall is
// EXCLUDED from that demotion (its input is gated past the threshold, so the
// speed clause would make Excellent unreachable).
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
	if t.Kind == TimingKindSequence && wrongCount == 0 && t.Elapsed >= SequenceFastThreshold {
		grade = TimingQualityGreat // flawless but slow
	}
	if grade < TimingQualityMiss {
		grade = TimingQualityMiss
	}
	if grade > TimingQualityExcellent {
		grade = TimingQualityExcellent
	}
	t.Quality = grade
}

// resolveCharge grades a charge release by ticks crossed (visual position via
// ChargeCursorProgress, not raw Elapsed). Past the peak decays to Miss.
func (t *TimingState) resolveCharge() {
	t.Resolved = true
	p := ChargeCursorProgress(t.Elapsed, t.Duration)
	t.Quality, _ = chargeGradeUpToPeak(p)
}

// chargeGradeUpToPeak grades a cursor at visual progress p, returning
// (grade, pastPeak). pastPeak is true ONLY past the peak window (a pre-peak
// early Miss returns false). Shared by both charge resolvers; resolveOvercharge
// reinterprets pastPeak as an overload.
func chargeGradeUpToPeak(p float32) (grade int, pastPeak bool) {
	switch {
	case p < ChargeTick1Pct:
		return TimingQualityMiss, false
	case p < ChargeTick2Pct:
		return TimingQualityNice, false
	case p < ChargeTick3Pct:
		return TimingQualityGood, false
	case p <= ChargePeakEnd:
		// Peak window — split Great vs Excellent on sweet-spot proximity.
		distance := AbsF(p - chargePeakSweet)
		windowSize := ChargePeakEnd - ChargePeakStart
		if windowSize <= 0 || distance/windowSize <= ChargeExcellentBandFrac {
			return TimingQualityExcellent, false
		}
		return TimingQualityGreat, false
	default:
		return TimingQualityMiss, true
	}
}

// resolveOvercharge grades like resolveCharge through the peak, but past the
// peak it OVERLOADS (Excellent + Overloaded) instead of Miss. A pre-tick1
// release still misses.
func (t *TimingState) resolveOvercharge() {
	t.Resolved = true
	p := ChargeCursorProgress(t.Elapsed, t.Duration)
	grade, pastPeak := chargeGradeUpToPeak(p)
	if pastPeak {
		t.Quality = TimingQualityExcellent
		t.Overloaded = true
		return
	}
	t.Quality = grade
}

// resolveChargeKind dispatches a charge resolve to the normal or overload
// grader. Both Release and Tick timeout route through here so they can't diverge.
func (t *TimingState) resolveChargeKind() {
	if t.Kind == TimingKindOvercharge {
		t.resolveOvercharge()
	} else {
		t.resolveCharge()
	}
}

// resolveInWindow grades a press against an explicit window. Callers must
// verify the cursor is inside it first (a zero-width window returns Nice).
func (t *TimingState) resolveInWindow(start, end, sweet float32) {
	t.Resolved = true
	t.Quality = gradeWindow(t.Elapsed, start, end, sweet)
}

// gradeWindow grades a cursor at `elapsed` against window [start,end] with sweet
// spot `sweet`: sweet-distance / width picks a band off pressGradeBands.
// Non-positive width returns Nice. Shared by resolveInWindow + PreviewQuality.
func gradeWindow(elapsed, start, end, sweet float32) int {
	windowSize := end - start
	if windowSize <= 0 {
		return TimingQualityNice
	}
	return gradeFromRatio(AbsF(elapsed-sweet) / windowSize)
}

// Progress returns the sweep position in [0,1]: ChargeCursorProgress for charge
// bars (accelerating), else clamped Elapsed/Duration.
func (t TimingState) Progress() float32 {
	if t.Duration <= 0 {
		return 0
	}
	if t.isChargeFamily() {
		return ChargeCursorProgress(t.Elapsed, t.Duration)
	}
	return Clamp(t.Elapsed/t.Duration, 0, 1)
}

// timingGradeAt returns the valid timingGrades index for a quality, falling
// back to Miss when out of range. Shared bounds guard for all grade readers.
func timingGradeAt(quality int) int {
	if quality < 0 || quality >= len(timingGrades) {
		return TimingQualityMiss
	}
	return quality
}

// TimingBonusMult is the offensive damage multiplier for an attack quality.
func TimingBonusMult(quality int) float32 {
	return timingGrades[timingGradeAt(quality)].Atk
}

// QualityScaledChance scales a base probability by TimingBonusMult, clamped to
// [_,1]. Shared by status-proc and steal-success rolls.
func QualityScaledChance(base float64, quality int) float64 {
	c := base * float64(TimingBonusMult(quality))
	return Clamp(c, 0, 1)
}

// TimingDefenseMult is the incoming damage multiplier for a defend quality
// (lower is better; Excellent quarters, Miss takes full).
func TimingDefenseMult(quality int) float32 {
	return timingGrades[timingGradeAt(quality)].Def
}

// pressGradeBands maps sweet-distance ratio (units of window width) to a grade.
// Single source for resolve + PreviewQuality so they can't desync.
var pressGradeBands = []struct {
	maxRatio float32
	grade    int
}{
	{PressExcellentBandFrac, TimingQualityExcellent},
	{PressGreatBandFrac, TimingQualityGreat},
	{PressGoodBandFrac, TimingQualityGood},
}

// gradeFromRatio looks up the grade for a normalized sweet-distance ratio;
// past the largest band falls through to Nice (caller gates window membership).
func gradeFromRatio(ratio float32) int {
	for _, b := range pressGradeBands {
		if ratio <= b.maxRatio {
			return b.grade
		}
	}
	return TimingQualityNice
}

// PreviewQuality returns the grade a press-kind bar would score right now (for
// the renderer's live cursor color). Miss when outside every window or non-press.
func (t TimingState) PreviewQuality() int {
	if t.Kind != TimingKindPress {
		return TimingQualityMiss
	}
	start, end, sweet, ok := t.activePressWindow()
	if !ok {
		return TimingQualityMiss
	}
	return gradeWindow(t.Elapsed, start, end, sweet)
}

// HitStopFor returns the freeze-frame duration after a graded press: 0 below
// Great, short for Great, longer for Excellent.
func HitStopFor(quality int) float32 {
	return timingGrades[timingGradeAt(quality)].HitStop
}

// CombatShakeFor returns the base screen-shake (peak + duration) for a graded
// hit — only Great/Excellent shake. Big crit/AoE shake is armed separately.
func CombatShakeFor(quality int) (peak, dur float32) {
	g := timingGrades[timingGradeAt(quality)]
	return g.ShakePeak, g.ShakeDur
}

// TriggerCombatShake arms the camera shake (peak world units, dur seconds).
// A stronger in-flight shake is not stomped by a weaker one. No-op for nil
// battle or non-positive peak/dur.
func TriggerCombatShake(b *Battle, peak, dur float32) {
	if b == nil || peak <= 0 || dur <= 0 {
		return
	}
	if b.ShakeTimer > 0 && b.ShakePeak > peak {
		return
	}
	b.ShakePeak = peak
	b.ShakeDur = dur
	b.ShakeTimer = dur
	// Buzz the pad proportionally so every shake site rumbles off one knob.
	// (Taking a hit doesn't shake, so that path calls TriggerRumble directly.)
	TriggerRumble(b, peak*RumblePerShakePeak, dur)
}

// TriggerRumble arms the rumble envelope (strength clamped [0,1], dur seconds,
// decayed by TickRumble). Keep-the-stronger like TriggerCombatShake. No-op on
// nil battle or non-positive args. Intent layer; input.ApplyRumble drives the motor.
func TriggerRumble(b *Battle, strength, dur float32) {
	if b == nil || strength <= 0 || dur <= 0 {
		return
	}
	strength = Clamp(strength, 0, 1)
	if b.RumbleTimer > 0 && b.RumbleStrength > strength {
		return
	}
	b.RumbleStrength = strength
	b.RumbleDur = dur
	b.RumbleTimer = dur
}

// TickRumble decays the rumble envelope by dt and returns the motor level
// [0,1] (or 0 when inactive). Must be called every frame (scene-independent) so
// a rumble armed near battle exit still eases to 0 instead of sticking on.
func TickRumble(b *Battle, dt float32) float32 {
	if b == nil || b.RumbleTimer <= 0 || b.RumbleDur <= 0 {
		return 0
	}
	b.RumbleTimer -= dt
	if b.RumbleTimer <= 0 {
		b.RumbleTimer = 0
		return 0
	}
	level := b.RumbleStrength * (b.RumbleTimer / b.RumbleDur)
	return Clamp(level, 0, 1)
}

// TimingQualityLabel returns the popup text for a quality grade.
func TimingQualityLabel(quality int) string {
	return timingGrades[timingGradeAt(quality)].Label
}

// ScaleDamage applies an attack quality's multiplier to base damage. Excellent
// is guaranteed > base (base+1 floor); other grades may round to 0 on tiny bases.
func ScaleDamage(base int, quality int) int {
	scaled := scaleByMult(base, TimingBonusMult(quality))
	if quality == TimingQualityExcellent && scaled <= base {
		scaled = base + 1
	}
	return scaled
}

// ScaleHeal applies an attack quality's multiplier to a base heal amount.
func ScaleHeal(base int, quality int) int {
	return scaleByMult(base, TimingBonusMult(quality))
}

// ScaleIncomingDamage applies a defend quality's multiplier to incoming damage,
// clamped at 0 (a strong block on a small hit can truncate to 0 — intentional).
func ScaleIncomingDamage(base int, quality int) int {
	return scaleByMult(base, TimingDefenseMult(quality))
}

// scaleByMult truncates base*mult to an int, clamped at 0. Shared core of every
// quality-scaled amount; wrappers layer special cases on top.
func scaleByMult(base int, mult float32) int {
	scaled := int(float32(base) * mult)
	if scaled < 0 {
		return 0
	}
	return scaled
}
