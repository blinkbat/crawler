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
	// SkillMinigame (party.go) and these TimingKind codes are parallel
	// enumerations mapped 1:1 in battle.beginPendingAction. Assert they stay
	// value-aligned so reordering or inserting a value in one without the
	// other fails loudly at startup instead of silently arming the wrong bar.
	if int(MinigamePress) != TimingKindPress ||
		int(MinigameCharge) != TimingKindCharge ||
		int(MinigameSequence) != TimingKindSequence ||
		int(MinigameReels) != TimingKindReels ||
		int(MinigameRecall) != TimingKindRecall ||
		int(MinigameOvercharge) != TimingKindOvercharge {
		panic("core: SkillMinigame and TimingKind enums have drifted out of lockstep")
	}
}

// Sequence-minigame direction codes. Stored in TimingState.SequenceTargets
// at NewSequenceState time; matched against player input at runtime.
const (
	SeqDirUp    = 0
	SeqDirRight = 1
	SeqDirDown  = 2
	SeqDirLeft  = 3
	// SeqDirCount is the number of directions — the range NewSequenceState
	// draws random targets from. Named so the random draw can't drift out
	// of lockstep with the four SeqDir* codes above.
	SeqDirCount = 4
)

func init() {
	// SeqDir* (above) and the cardinal facings North/East/South/West
	// (config.go) are intentionally the SAME clockwise order with the same
	// int values, so directional code can read either set interchangeably.
	// Nothing else pins them together, so assert the parity here — the same
	// MinigamePress==TimingKindPress style guard above — so a reorder or an
	// inserted value in one without the other fails loudly at startup.
	if int(SeqDirCount) != int(FacingCount) ||
		int(SeqDirUp) != int(North) ||
		int(SeqDirRight) != int(East) ||
		int(SeqDirDown) != int(South) ||
		int(SeqDirLeft) != int(West) {
		panic("core: SeqDir* and cardinal facing enums have drifted out of lockstep")
	}
}

// Per-slot result for the sequence minigame. SequenceResults is a slice
// parallel to SequenceTargets — one entry per arrow.
const (
	SeqResultPending = iota
	SeqResultCorrect
	SeqResultWrong
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
//
// TallyWindow is one accept zone in a multi-press tally bar. Hit
// marks the zone as already consumed so a player can't repeatedly
// press inside the same window for free hits — only the first press
// inside an unconsumed window scores. Sweet is the in-window peak
// position used by the renderer to highlight the bullseye; tally
// mode doesn't grade per-window (every hit counts equally) but the
// visual still nudges the player toward the centre. FlashTimer is
// the per-window feedback hold — set to TallyHitFlashDuration on
// the press that lands the hit, decays each Tick, drives the
// render-side "this window just got hit" pulse so the player gets
// visual confirmation without waiting for the bar to resolve.
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

	// Multi-press tally mode. When Windows is non-nil, the bar treats
	// each press as a per-window hit: a press inside an unconsumed
	// accept window increments Hits and marks that window consumed
	// (without resolving the bar). A press inside the late "commit"
	// zone (CommitStart..Duration) resolves the bar early with the
	// current tally. Bar timeout also resolves with the tally. Quality
	// maps from Hits at resolve time; for an N-window bar, 0 hits =
	// Miss, partial = Good, all-windows-cleared = Excellent. Callers
	// that want per-hit damage scaling read Hits directly.
	//
	// Windows are placed in time order (Start ascending) and never
	// overlap each other or the commit zone — see NewMultiPressState
	// for the geometry rule.
	Windows     []TallyWindow
	Hits        int
	CommitStart float32

	// Sequence-kind state (unused for press/charge). Targets is the random
	// run of directions; Cursor points at the next slot to fill; Results is
	// parallel to Targets and holds Pending / Correct / Wrong per slot.
	// Recall-kind reuses these three for its pattern + per-slot grading.
	SequenceTargets []int
	SequenceResults []int
	SequenceCursor  int

	// Reels-kind state (TimingKindReels). One Reel per spinner. A press locks
	// the next still-spinning reel on whatever symbol it's showing;
	// resolveReels grades by the largest matching set across the locked reels.
	Reels []Reel

	// Recall-kind state (TimingKindRecall). RevealTime is how long the
	// pattern (in SequenceTargets) stays face-up before it hides and input
	// opens — the "memorize, then reproduce from memory" phase split.
	RevealTime float32

	// Overcharge-kind state (TimingKindOvercharge). Set true by
	// resolveOvercharge when the player releases past the peak window (into
	// the overload band): the skill's apply path reads it to grant the bonus
	// effect and apply recoil.
	Overloaded bool
}

// IsTallyMode reports whether this is a multi-press tally bar. Render
// and apply paths gate per-window draws + per-hit damage on this.
func (t TimingState) IsTallyMode() bool {
	return t.Kind == TimingKindPress && len(t.Windows) > 0
}

// NewTimingState builds a freshly-armed press-kind bar. The bar sweeps for
// the given duration. Window position is randomized each time the bar arms
// (in [PressWindow.MinStart, PressWindow.MaxStart] of bar duration) so the
// player can't muscle-memory the press point — but it never opens before
// PressWindow.MinStart, so they always get a moment of "approaching, not
// yet" before a hit is possible. Width is fixed at PressWindow.Width and
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

// NewMultiPressState builds a tally-mode press bar with `count`
// acceptance windows + a late commit zone. Each press inside an
// unconsumed accept window scores one hit; pressing inside the
// commit zone (or letting the bar elapse) resolves with the
// current tally. Used by Swipe and any other multi-hit skill where
// the player should be able to chain presses across the bar
// instead of resolving on the first input.
//
// Windows are spaced evenly across the sweep so the player sees a
// rhythmic row of "hit zones" rather than a single wide blob. The
// commit zone sits in the final 15% of the duration so the late
// portion of the bar reliably ends the tally — pressing there
// fires the attack with whatever hits landed so far.
//
// count <= 0 falls back to a single accept window so a caller can't
// accidentally arm a never-resolvable bar.
func NewMultiPressState(rng *rand.Rand, duration float32, count int) TimingState {
	if duration <= 0 {
		duration = AttackTimingDuration
	}
	if count < 1 {
		count = 1
	}
	commitZoneFrac := MultiPressWindow.CommitZoneFrac
	commitStart := 1.0 - commitZoneFrac
	// Accept windows are distributed across [leadIn, commitStart -
	// small gap]. Each window is winWidth wide; gaps between windows
	// are computed from the remaining span so all N windows fit even
	// at higher counts. Geometry config lives in MultiPressWindow
	// (config.go) so a balance pass touches one file.
	leadIn := MultiPressWindow.LeadInFrac
	winWidth := MultiPressWindow.WindowWidthFrac
	span := commitStart - leadIn - 0.02 // small breathing-room gap before commit
	if span < winWidth {
		span = winWidth
	}
	windows := make([]TallyWindow, count)
	// stride is the distance between window CENTERS; place the
	// centers so the row fills [leadIn + winWidth/2, leadIn + span - winWidth/2].
	usable := span - winWidth
	for i := 0; i < count; i++ {
		var center float32
		if count == 1 {
			center = leadIn + span*0.5
		} else {
			center = leadIn + winWidth*0.5 + usable*(float32(i)/float32(count-1))
		}
		// Jitter the center slightly so two consecutive bars don't
		// have identical zone placement, but only within a tiny
		// range so the rhythm reads as predictable.
		jitter := (float32(rng.Float64())*2 - 1) * (winWidth * 0.20)
		center += jitter
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

// NewTallyStateAtCenters builds a tally-mode press bar with accept
// windows hand-placed at the given fractional centers (0..1 of the bar
// duration) rather than NewMultiPressState's even spread. Used where a
// skill wants a specific rhythm: Swipe's two hits sit around the middle
// and just before the commit tail (a "wind up, then the big swing"
// beat) instead of bunched at the two ends. Each window keeps
// MultiPressWindow.WindowWidthFrac width and is clamped so it stays
// fully inside [winWidth/2, commitStart-winWidth/2] — a center pushed
// into the commit tail would otherwise become unpressable.
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

// randomizedPressWindow returns (start, end, sweet) fractions for a press
// window placed inside [minStart, maxStart] with the fixed `width`. If the
// roll would push the window past `maxEnd` it slides back so the full
// width still fits. Used by NewTimingState's single-window placement.
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
// / 0.75 / 0.85 — see ChargeTick1Pct..ChargeTick3Pct in config.go); the *elapsed* values
// here are what stretch and squeeze the cursor's speed across them.
// Touch one row and both render and grade follow.
var chargeSegments = [...]struct {
	Visual, Elapsed float32
}{
	{0.00, 0.00},
	{ChargeTick1Pct, 0.45}, // segment 0 — slow lead-in (slope ≈ 0.56)
	{ChargeTick2Pct, 0.70}, // segment 1 — speeds up (slope ≈ 1.00)
	{ChargeTick3Pct, 0.88}, // segment 2 — faster yet (slope ≈ 1.39)
	{ChargePeakEnd, 0.94},  // segment 3 — peak window (slope ≈ 1.67)
	{1.00, 1.00},           // segment 4 — decay, past the player's reach
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

// randomDirectionRun builds a length-N slice of random directional indices
// (each in [0, SeqDirCount)). The shared pattern generator for the Sequence
// and Recall minigame constructors, which otherwise hand-rolled the identical
// make+fill loop in lockstep.
func randomDirectionRun(rng *rand.Rand, length int) []int {
	targets := make([]int, length)
	for i := range targets {
		targets[i] = rng.Intn(SeqDirCount)
	}
	return targets
}

// NewSequenceState builds a freshly-armed sequence-kind bar with `length`
// random directional arrows. Player has `duration` seconds to tap them all
// in order; pending/wrong slots drop the grade.
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

// Reel is one spinner in the slot (Reels) minigame. Speed is its symbol
// cadence (symbols/sec), Offset its starting symbol, and Stop the locked
// symbol once the player stops it (-1 while still spinning). Bundling the
// three into one struct (vs the old parallel ReelSpeeds/Offsets/Stops slices)
// keeps them in lockstep by construction — no per-field bounds-checking.
type Reel struct {
	Speed  float32
	Offset int
	Stop   int
}

// NewReelState builds a slot-machine bar: ReelCount reels each cycling
// ReelSymbolCount symbols at its own (RNG-varied, desynced) speed. A press
// locks the next still-spinning reel onto whatever symbol it's showing; the
// bar resolves once every reel is stopped (or the duration elapses, locking
// the rest). resolveReels grades by the largest matching set.
func NewReelState(rng *rand.Rand, duration float32) TimingState {
	if duration <= 0 {
		duration = ReelTimingDuration
	}
	reels := make([]Reel, ReelCount)
	for i := range reels {
		reels[i] = Reel{
			// Desynced speeds so the player can't stop all reels on one beat
			// for a guaranteed jackpot — matching three takes skill plus luck.
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

// ReelSymbolAt returns the symbol currently shown on reel i: its locked value
// if stopped, else the spinning value derived from elapsed time and the
// reel's speed/offset. Render and resolve both read this so a stopped reel
// always shows exactly what it locked.
func (t TimingState) ReelSymbolAt(i int) int {
	if i < 0 || i >= len(t.Reels) {
		return 0
	}
	r := t.Reels[i]
	if r.Stop >= 0 {
		return r.Stop
	}
	steps := int(t.Elapsed * r.Speed)
	if steps < 0 {
		// Elapsed is only ever advanced by a non-negative dt today, but guard
		// against a malformed/replayed negative so the modulo can't return a
		// negative index that a symbol-art lookup would treat as out-of-range.
		steps = 0
	}
	return (r.Offset + steps) % ReelSymbolCount
}

// StopNextReel locks the next still-spinning reel onto its current symbol.
// Returns true if this stop resolved the bar (the last reel landed). No-op
// when every reel is already stopped. Reels-kind only.
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

// resolveReels finalises the slot result. Any reel still spinning at timeout
// locks on its current symbol first. Three matching symbols = Excellent
// (jackpot), exactly two = Good, all distinct = Miss — a real gamble where
// the player can whiff. A pure timeout where the player never stopped a reel
// (!Pressed) is a no-PLAY, graded Miss outright: otherwise the random
// auto-locked symbols would frequently match and hand a walk-away player a
// boosted Steal chance (the apply path reads Quality regardless of Pressed).
func (t *TimingState) resolveReels() {
	// Snapshot the reels the player actually stopped BEFORE auto-locking the
	// rest. Only player-stopped reels count toward the match — a reel left
	// spinning still locks on its current symbol for display, but letting it
	// time out must not luck into a jackpot from a randomly auto-locked symbol
	// (the apply path reads Quality regardless of how the reel stopped).
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

// largestReelMatch returns the size of the most-repeated locked symbol across
// the reels (called after every reel is stopped).
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

// NewRecallState builds a memory-pattern bar: a random directional run shows
// face-up for `reveal` seconds, then hides — the player reproduces it from
// memory before the bar elapses. Reuses the Sequence* fields and
// resolveSequence grading; the only differences from a sequence bar are the
// reveal-then-hide phase (RecallHidden) and that input is ignored until the
// pattern hides (gated at the call site).
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
	// Reveal MUST end before the bar does, or RecallHidden never flips: there'd
	// be no input window and the resolve-flash would paint the memorize branch
	// (answer lit). Cap it to leave at least a fraction of the bar for input.
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

// RecallHidden reports whether a recall bar's pattern has hidden — i.e. the
// reveal phase is over and the input phase is open. Render gates "show the
// arrows" on this; the battle input loop gates "accept directional taps" on it.
func (t TimingState) RecallHidden() bool {
	return t.Kind == TimingKindRecall && t.Elapsed >= t.RevealTime
}

// NewOverchargeState builds a charge bar with a post-peak OVERLOAD band:
// releasing past the peak (or holding to the end) overcharges the spell —
// bonus effect at the cost of recoil — instead of decaying straight to Miss.
// Identical cursor/curve to a normal charge; only the resolve differs
// (resolveOvercharge).
func NewOverchargeState(duration float32) TimingState {
	t := NewChargeState(duration)
	t.Kind = TimingKindOvercharge
	return t
}

// Tick advances the bar by dt. For press bars, auto-resolves a Miss if the
// window passes without a press. For charge bars, auto-resolves at duration
// (graded by where Elapsed lands) — held-too-long produces a Miss/Nice.
func (t *TimingState) Tick(dt float32) {
	if !t.Active || t.Resolved {
		return
	}
	t.Elapsed += dt
	// Per-window flash decay on tally bars. The render reads
	// FlashTimer to drive a brief brightening on each freshly-hit
	// window; tick it down here so the feedback decays even while
	// the bar continues sweeping toward more accept zones.
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
		// Bar timed out. If they engaged at all, treat as released-late
		// (overcharge grades this as an overload); otherwise it's just a Miss.
		if t.Pressed {
			t.Released = true
			t.resolveChargeKind()
		} else {
			t.Quality = TimingQualityMiss
			t.Resolved = true
		}
	case TimingKindSequence, TimingKindRecall:
		// Time's up — pending slots count as wrong, grade what we have.
		// Recall shares the sequence grading (it's a hidden-pattern sequence).
		t.resolveSequence()
	case TimingKindReels:
		// Time's up — lock any reel still spinning and grade the result.
		t.resolveReels()
	default:
		// Press bar timed out. Tally-mode bars resolve with the
		// hits they accumulated (could be partial); single-window
		// bars auto-Miss for failure to press.
		if t.IsTallyMode() {
			t.resolveTally()
		} else {
			t.Resolved = true
			t.Quality = TimingQualityMiss
		}
	}
}

// Press records a press-kind input. Returns true if this press
// resolved the bar.
//
// Single-window bars resolve on the first press: in-window grades
// against the gradient bands; out-of-window stamps a Miss.
//
// Tally-mode bars (Windows non-nil) accumulate hits without
// resolving — each press inside an unconsumed accept window
// increments Hits and marks the window consumed. Pressing inside
// the late commit zone (>= CommitStart) resolves with the current
// tally; pressing outside any window or zone is a no-op (the bar
// continues sweeping). The bar also resolves on Tick timeout.
//
// For charge-kind bars, use Hold/Release instead.
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

// pressTally handles one input on a multi-press tally bar. Each
// press inside an unconsumed accept window adds a hit; pressing in
// the commit zone resolves the bar with whatever's tallied; any
// press outside both zones is a silent no-op (no penalty — the
// rhythm-game vibe is "tap to the beat," not "punish off-beat").
func (t *TimingState) pressTally() bool {
	// Commit-zone press: end the bar early with the current tally. Mark
	// Pressed so a deliberate early commit registers as engagement even with
	// zero hits — a walk-away (Tick timeout, no press) is the only !Pressed case.
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
			// All windows landed? Resolve immediately — there's
			// nothing else to score; making the player wait would
			// just feel like dead time.
			if t.Hits >= len(t.Windows) {
				t.resolveTally()
				return true
			}
			return false
		}
	}
	// Press outside any window AND before commit — no-op. Player
	// can keep tapping; the bar continues.
	return false
}

// resolveTally finalises a multi-press bar. Quality maps from Hits:
// zero = Miss, all-windows = Excellent (perfect run), one short of all
// = Great, any fewer than that = Good. For a 2-window bar the Good band
// is unreachable (1-of-2 lands in the Great band) — intended: missing
// half a two-hit tally is still a solid partial. The Good band is for
// 3+-window bars, where landing e.g. 1 of 3 grades below Great. Most
// apply paths also read Hits directly so the per-hit damage scaling
// lands without depending on Quality.
func (t *TimingState) resolveTally() {
	t.Resolved = true
	// Don't clobber an already-set Pressed (a commit-zone press marks it true
	// before calling here); otherwise infer engagement from landed hits so a
	// Tick-timeout with no input stays !Pressed.
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

// activePressWindow returns the (start, end, sweet) of the single
// acceptance window the cursor is currently inside, with ok=false
// if the cursor is outside it. Single-window bars only — multi-
// window tally bars route through pressTally and never call this.
func (t TimingState) activePressWindow() (start, end, sweet float32, ok bool) {
	if t.Elapsed >= t.WindowStart && t.Elapsed <= t.WindowEnd {
		return t.WindowStart, t.WindowEnd, t.SweetSpot, true
	}
	return 0, 0, 0, false
}

// Hold marks the charge bar as engaged. Idempotent — sets Pressed=true on
// the first held frame and stays. No-op for press-kind bars.
func (t *TimingState) Hold() {
	if !t.Active || t.Resolved {
		return
	}
	if !t.isChargeFamily() {
		return
	}
	t.Pressed = true
}

// isChargeFamily reports whether this bar uses the hold-and-release charge
// flow (normal Charge or its Overcharge variant). Both share Hold / Release
// / Tick-timeout handling — only the resolve grader differs.
func (t TimingState) isChargeFamily() bool {
	return t.Kind == TimingKindCharge || t.Kind == TimingKindOvercharge
}

// Release closes a charge bar. Returns true if this release resolved the bar.
// Releasing without ever having Held does nothing — you can't release what
// you never picked up. No-op for press-kind bars.
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

// SequenceInput records a directional press at the current cursor slot.
// Marks that slot Correct or Wrong, advances the cursor, and resolves when
// the last slot is filled. Returns true if this input resolved the bar.
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

// resolveSequence grades the directional-sequence pattern. Each non-correct
// slot (wrong key OR pending/timed-out) drops one grade from Excellent. For
// the plain sequence bar (TimingKindSequence — Venom Strike) a flawless run
// only reaches Excellent when finished under SequenceFastThreshold — a
// clean-but-slow run caps at Great, so speed is the deciding edge. The shared
// Recall bar (TimingKindRecall)
// is EXCLUDED from that demotion: its input is gated until the pattern hides
// (Elapsed >= RevealTime > SequenceFastThreshold), so the speed clause would make
// Excellent structurally unreachable — the memory itself is its skill test, so
// a flawless recall earns Excellent regardless of clock.
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
		// Flawless but slow — the top grade is reserved for a fast clean run.
		grade = TimingQualityGreat
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
	// chargeGradeUpToPeak returns pastPeak=true PAST the peak window, where the
	// returned grade is Miss — exactly the normal charge's held-too-long penalty.
	t.Quality, _ = chargeGradeUpToPeak(p)
}

// chargeGradeUpToPeak grades a charge cursor at visual progress p across the
// pre-peak ticks and the peak window, returning (grade, pastPeak). pastPeak is
// true ONLY when p is past the peak window (the late-release case); the
// pre-peak early-release Miss returns pastPeak=false so it isn't mistaken for
// an overload. The caller decides what pastPeak means: resolveCharge takes the
// returned Miss either way (held too long / released too early), while
// resolveOvercharge reinterprets pastPeak as an OVERLOAD (bonus + recoil) but
// still misses the pre-peak early release. Sharing this keeps the two charge
// resolvers' tick/peak bands from drifting.
func chargeGradeUpToPeak(p float32) (grade int, pastPeak bool) {
	switch {
	case p < ChargeTick1Pct:
		return TimingQualityMiss, false
	case p < ChargeTick2Pct:
		return TimingQualityNice, false
	case p < ChargeTick3Pct:
		return TimingQualityGood, false
	case p <= ChargePeakEnd:
		// In the peak window — split Great vs Excellent on sweet-spot proximity.
		sweet := (ChargePeakStart + ChargePeakEnd) * 0.5
		distance := p - sweet
		if distance < 0 {
			distance = -distance
		}
		windowSize := ChargePeakEnd - ChargePeakStart
		if windowSize <= 0 || distance/windowSize <= ChargeExcellentBandFrac {
			return TimingQualityExcellent, false
		}
		return TimingQualityGreat, false
	default:
		return TimingQualityMiss, true
	}
}

// resolveOvercharge grades like resolveCharge through the peak, but PAST the
// peak — where a normal charge decays to Miss — it OVERLOADS: the release
// still counts as a top-tier (Excellent) hit and sets Overloaded so the
// skill's apply path grants the bonus and applies recoil. Releasing before
// the first tick still misses; the brave-but-greedy late release pays off
// with a cost.
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
// grader. Both charge-family kinds (Charge / Overcharge) route through here
// from Release and the Tick timeout so the late-release path can't diverge.
func (t *TimingState) resolveChargeKind() {
	if t.Kind == TimingKindOvercharge {
		t.resolveOvercharge()
	} else {
		t.resolveCharge()
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
	if t.isChargeFamily() {
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

// QualityScaledChance scales a base probability by the timing-quality
// offensive multiplier and clamps to [_, 1]. Single home for the
// "base * TimingBonusMult(quality), cap at 1.0" pattern shared by
// status-proc and steal-success rolls.
func QualityScaledChance(base float64, quality int) float64 {
	c := base * float64(TimingBonusMult(quality))
	if c > 1 {
		c = 1
	}
	return c
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
	if quality < 0 || quality >= len(timingGrades) {
		return 0
	}
	return timingGrades[quality].HitStop
}

// CombatShakeFor returns the BASE screen-shake (peak amplitude + duration) a
// graded hit earns — only Great / Excellent presses shake (the timed-hit
// payoff), Excellent a touch harder + longer. This is deliberately subtle;
// the big shake (crits / AoE) is armed separately via TriggerCombatShake.
// Mirrors HitStopFor so the two impact knobs sit together.
func CombatShakeFor(quality int) (peak, dur float32) {
	if quality < 0 || quality >= len(timingGrades) {
		return 0, 0
	}
	g := timingGrades[quality]
	return g.ShakePeak, g.ShakeDur
}

// TriggerCombatShake arms the camera shake with an explicit peak amplitude
// (world units) and duration (seconds). A stronger shake already in flight is
// NOT stomped by a weaker one — so the big crit/AoE shake survives the small
// grade-based base that gets armed a frame earlier in the same resolve. No-op
// for a nil battle or a non-positive peak/duration (e.g. a Miss/Nice/Good).
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
	// Impact feedback has two halves: the camera shake (above) and a
	// proportional controller rumble. Arming both here means every shake site
	// (good press / crit / AoE) buzzes the pad too, graded by the same peak —
	// one knob, no scattered TriggerRumble calls. Taking a hit doesn't shake,
	// so that path arms TriggerRumble directly.
	TriggerRumble(b, peak*RumblePerShakePeak, dur)
}

// TriggerRumble arms the controller-rumble envelope: peak motor strength
// (clamped to [0,1]) for `dur` seconds, decayed by TickRumble. Keep-the-stronger
// like TriggerCombatShake — a weaker buzz won't stomp a stronger one still in
// flight. No-op on a nil battle or non-positive strength/dur. This is the
// raylib-free intent layer; input.ApplyRumble drives the actual motor.
func TriggerRumble(b *Battle, strength, dur float32) {
	if b == nil || strength <= 0 || dur <= 0 {
		return
	}
	if strength > 1 {
		strength = 1
	}
	if b.RumbleTimer > 0 && b.RumbleStrength > strength {
		return
	}
	b.RumbleStrength = strength
	b.RumbleDur = dur
	b.RumbleTimer = dur
}

// TickRumble decays the rumble envelope by dt and returns the current motor
// level in [0,1] (RumbleStrength scaled by the remaining fraction), or 0 when
// no rumble is active. Pure / raylib-free — the run loop calls it EVERY frame
// (scene-independently) and hands the level to input.ApplyRumble. Decaying in
// the main loop (not a battle-only update) is what guarantees a rumble armed
// just before battle exit still eases to 0 instead of sticking the motor on.
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
	if level < 0 {
		return 0
	}
	if level > 1 {
		return 1
	}
	return level
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

// ScaleIncomingDamage applies a defend quality's multiplier to incoming damage.
// A defended hit cannot become heal — clamps at 0. Integer truncation means
// a Great/Excellent block on a small hit (e.g. base 3 × 0.25 = 0.75 → 0) can
// fully zero the damage; that's intentional, matches how a successful
// timed block reads in-game ("0 damage" with the block flourish).
func ScaleIncomingDamage(base int, quality int) int {
	return scaleByMult(base, TimingDefenseMult(quality))
}

// scaleByMult truncates base*mult to an int, clamped at 0 — the shared
// core of every quality-scaled amount (damage, heal, incoming). Each
// public wrapper layers its own special case (the Excellent damage
// floor) on top.
func scaleByMult(base int, mult float32) int {
	scaled := int(float32(base) * mult)
	if scaled < 0 {
		return 0
	}
	return scaled
}
