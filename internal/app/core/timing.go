package core

const (
	TimingQualityMiss = iota
	TimingQualityNice
	TimingQualityGood
	TimingQualityGreat
	TimingQualityExcellent
)

// TimingKind picks how the bar grades input.
//   Press:    hit the input once during a window.
//   Charge:   hold through three ticks, release at the peak.
//   Sequence: tap a randomized run of directions in order before time's up.
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
//   Press:
//     WindowStart..WindowEnd is the green acceptance window.
//     SweetSpot is the centered "Excellent" tick.
//     Pressed is true once the input fired.
//
//   Charge:
//     WindowStart..WindowEnd is the peak release window (right after the 3rd
//     tick lands). SweetSpot is the centered "Excellent" point inside it.
//     Pressed is true once the player ever held the input.
//     Released is true once they let go (or the bar timed out while held).
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
func NewTimingState(duration float32) TimingState {
	if duration <= 0 {
		duration = AttackTimingDuration
	}
	span := PressWindowMaxStart - PressWindowMinStart
	startPct := PressWindowMinStart + float32(GameRNG.Float64())*span
	endPct := startPct + PressWindowWidth
	if endPct > 0.96 {
		// Clamp so the window never bumps against the bar's tail. Slide it
		// back so the full width fits.
		endPct = 0.96
		startPct = endPct - PressWindowWidth
	}
	sweet := (startPct + endPct) * 0.5
	return TimingState{
		Kind:        TimingKindPress,
		Active:      true,
		Duration:    duration,
		WindowStart: startPct * duration,
		WindowEnd:   endPct * duration,
		SweetSpot:   sweet * duration,
	}
}

// NewChargeState builds a freshly-armed charge-kind bar. The bar runs for
// `duration` seconds; three tick markers land before the peak window opens
// at ChargePeakStart and closes at ChargePeakEnd. The sweet spot is centered
// in the peak window.
func NewChargeState(duration float32) TimingState {
	if duration <= 0 {
		duration = ChargeTimingDuration
	}
	return TimingState{
		Kind:        TimingKindCharge,
		Active:      true,
		Duration:    duration,
		WindowStart: ChargePeakStart * duration,
		WindowEnd:   ChargePeakEnd * duration,
		SweetSpot:   (ChargePeakStart + ChargePeakEnd) * 0.5 * duration,
	}
}

// NewSequenceState builds a freshly-armed sequence-kind bar with `length`
// random directional arrows. Player has `duration` seconds to tap them all
// in order; pending/wrong slots drop the grade.
func NewSequenceState(duration float32, length int) TimingState {
	if duration <= 0 {
		duration = StealTimingDuration
	}
	if length <= 0 {
		length = StealSequenceLength
	}
	targets := make([]int, length)
	for i := range targets {
		targets[i] = GameRNG.Intn(4)
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
		t.resolve(false)
	}
}

// Press records a press-kind input. Returns true if this press resolved the
// bar. For charge-kind bars, use Hold/Release instead.
func (t *TimingState) Press() bool {
	if !t.Active || t.Resolved || t.Pressed {
		return false
	}
	if t.Kind != TimingKindPress {
		return false
	}
	t.Pressed = true
	inWindow := t.Elapsed >= t.WindowStart && t.Elapsed <= t.WindowEnd
	t.resolve(inWindow)
	return true
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
// the player crossed PAST before letting go. Each tick is a discrete grade
// jump, so a release that lands one frame before the third tick scores the
// same as one that lands halfway between the second and third. The peak
// window (3 ticks completed) is split into Great vs Excellent based on
// closeness to the sweet spot. Past the peak window the release decays
// straight to Miss — over-charging isn't rewarded.
//
// Grade dispatch:
//
//	0 ticks crossed (Elapsed < tick1):           Miss
//	1 tick crossed  (tick1 <= Elapsed < tick2):  Nice
//	2 ticks crossed (tick2 <= Elapsed < tick3):  Good
//	3 ticks crossed in peak window:              Great or Excellent
//	past peak window:                            Miss
func (t *TimingState) resolveCharge() {
	t.Resolved = true
	tick1 := ChargeTick1Pct * t.Duration
	tick2 := ChargeTick2Pct * t.Duration
	tick3 := ChargeTick3Pct * t.Duration

	switch {
	case t.Elapsed < tick1:
		t.Quality = TimingQualityMiss
	case t.Elapsed < tick2:
		t.Quality = TimingQualityNice
	case t.Elapsed < tick3:
		t.Quality = TimingQualityGood
	case t.Elapsed <= t.WindowEnd:
		// In the peak window — split Great vs Excellent on sweet-spot proximity.
		distance := t.Elapsed - t.SweetSpot
		if distance < 0 {
			distance = -distance
		}
		windowSize := t.WindowEnd - t.WindowStart
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

func (t *TimingState) resolve(inWindow bool) {
	t.Resolved = true
	if !inWindow {
		t.Quality = TimingQualityMiss
		return
	}
	distance := t.Elapsed - t.SweetSpot
	if distance < 0 {
		distance = -distance
	}
	windowSize := t.WindowEnd - t.WindowStart
	if windowSize <= 0 {
		t.Quality = TimingQualityNice
		return
	}
	ratio := distance / windowSize
	switch {
	case ratio <= 0.05:
		t.Quality = TimingQualityExcellent
	case ratio <= 0.15:
		t.Quality = TimingQualityGreat
	case ratio <= 0.30:
		t.Quality = TimingQualityGood
	default:
		t.Quality = TimingQualityNice
	}
}

// Progress returns the current sweep position in [0, 1].
func (t TimingState) Progress() float32 {
	if t.Duration <= 0 {
		return 0
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
// Multipliers live in config.go so balance tuning is centralized.
func TimingBonusMult(quality int) float32 {
	switch quality {
	case TimingQualityNice:
		return TimingMultNice
	case TimingQualityGood:
		return TimingMultGood
	case TimingQualityGreat:
		return TimingMultGreat
	case TimingQualityExcellent:
		return TimingMultExcellent
	default:
		return TimingMultMiss
	}
}

// TimingDefenseMult is the incoming damage multiplier for a defend quality.
// Lower is better; Excellent quarters incoming damage, Miss takes the full hit.
// Multipliers live in config.go.
func TimingDefenseMult(quality int) float32 {
	switch quality {
	case TimingQualityNice:
		return TimingDefNice
	case TimingQualityGood:
		return TimingDefGood
	case TimingQualityGreat:
		return TimingDefGreat
	case TimingQualityExcellent:
		return TimingDefExcellent
	default:
		return TimingDefMiss
	}
}

// TimingQualityLabel returns the popup text for a quality grade.
func TimingQualityLabel(quality int) string {
	switch quality {
	case TimingQualityNice:
		return "Nice!"
	case TimingQualityGood:
		return "Good!"
	case TimingQualityGreat:
		return "Great!"
	case TimingQualityExcellent:
		return "Excellent!"
	default:
		return "Miss..."
	}
}

// ScaleDamage applies an attack quality's multiplier to a base damage amount.
// Excellent always lands at least 1 point even on a 0 base.
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
// A defended hit cannot become heal — clamps at 0.
func ScaleIncomingDamage(base int, quality int) int {
	scaled := int(float32(base) * TimingDefenseMult(quality))
	if scaled < 0 {
		scaled = 0
	}
	return scaled
}
