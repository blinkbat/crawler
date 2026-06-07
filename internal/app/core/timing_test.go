package core

import (
	"math/rand"
	"testing"
)

// Tests use a deterministic RNG so press-window placement, sequence targets,
// and burn rolls become reproducible. The closure receives the seeded RNG;
// callers pass it through to NewTimingState / NewSequenceState / MemberAttackHits.
func withSeededRNG(t *testing.T, seed int64, fn func(rng *rand.Rand)) {
	t.Helper()
	fn(rand.New(rand.NewSource(seed)))
}

// TestResolveSequenceGrades locks the pickpocket grading after the dead-code
// fix. A flawless run reaches Excellent ONLY when finished under
// SequenceFastThreshold; a clean-but-slow run caps at Great (speed is the
// deciding edge), and each wrong slot drops one grade from Excellent.
func TestResolveSequenceGrades(t *testing.T) {
	correct := func(n int) []int {
		r := make([]int, n)
		for i := range r {
			r[i] = SeqResultCorrect
		}
		return r
	}
	cases := []struct {
		name    string
		results []int
		elapsed float32
		want    int
	}{
		{"clean and fast", correct(3), SequenceFastThreshold - 0.1, TimingQualityExcellent},
		{"clean but slow", correct(3), SequenceFastThreshold + 0.1, TimingQualityGreat},
		{"one wrong (fast)", []int{SeqResultCorrect, 0, SeqResultCorrect}, SequenceFastThreshold - 0.1, TimingQualityGreat},
		{"two wrong", []int{SeqResultCorrect, 0, 0}, 0, TimingQualityGood},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := &TimingState{
				// Sequence kind: the fast-threshold speed demotion is
				// Sequence-only (Recall is excluded), so the grading this
				// case checks only fires for TimingKindSequence.
				Kind:            TimingKindSequence,
				SequenceTargets: make([]int, len(tc.results)),
				SequenceResults: tc.results,
				Elapsed:         tc.elapsed,
			}
			ts.resolveSequence()
			if ts.Quality != tc.want {
				t.Fatalf("grade = %d, want %d", ts.Quality, tc.want)
			}
		})
	}
}

func TestNewTimingState_ZeroDurationDefaults(t *testing.T) {
	withSeededRNG(t, 1, func(rng *rand.Rand) {
		s := NewTimingState(rng, 0)
		if s.Duration != AttackTimingDuration {
			t.Fatalf("zero duration should default to AttackTimingDuration, got %v", s.Duration)
		}
		if s.Kind != TimingKindPress {
			t.Fatalf("expected press kind, got %v", s.Kind)
		}
		if !s.Active || s.Resolved {
			t.Fatalf("freshly armed bar should be active and unresolved")
		}
	})
}

func TestNewTimingState_WindowWithinBounds(t *testing.T) {
	withSeededRNG(t, 42, func(rng *rand.Rand) {
		for i := 0; i < 50; i++ {
			s := NewTimingState(rng, 2.0)
			startPct := s.WindowStart / s.Duration
			endPct := s.WindowEnd / s.Duration
			if startPct < PressWindow.MinStart-0.001 {
				t.Fatalf("window opened before PressWindow.MinStart: %v", startPct)
			}
			if endPct > PressWindow.MaxEnd+0.001 {
				t.Fatalf("window crossed tail: %v", endPct)
			}
			width := endPct - startPct
			if width < PressWindow.Width-0.001 || width > PressWindow.Width+0.001 {
				t.Fatalf("window width drifted from PressWindow.Width: %v", width)
			}
		}
	})
}

func TestPress_InWindowExcellentAtSweetSpot(t *testing.T) {
	withSeededRNG(t, 7, func(rng *rand.Rand) {
		s := NewTimingState(rng, 2.0)
		s.Elapsed = s.SweetSpot
		if !s.Press() {
			t.Fatalf("press at sweet spot should resolve the bar")
		}
		if s.Quality != TimingQualityExcellent {
			t.Fatalf("sweet-spot press should be Excellent, got %v", s.Quality)
		}
	})
}

func TestPress_OutsideWindowIsMiss(t *testing.T) {
	withSeededRNG(t, 7, func(rng *rand.Rand) {
		s := NewTimingState(rng, 2.0)
		s.Elapsed = 0
		s.Press()
		if s.Quality != TimingQualityMiss {
			t.Fatalf("press before window should be Miss, got %v", s.Quality)
		}
	})
}

func TestPress_OnlyResolvesOnce(t *testing.T) {
	withSeededRNG(t, 7, func(rng *rand.Rand) {
		s := NewTimingState(rng, 2.0)
		s.Elapsed = s.SweetSpot
		if !s.Press() {
			t.Fatalf("first press should resolve")
		}
		if s.Press() {
			t.Fatalf("second press should be ignored")
		}
	})
}

func TestPress_WrongKindNoOp(t *testing.T) {
	s := NewChargeState(1.0)
	if s.Press() {
		t.Fatalf("Press on charge-kind should be a no-op")
	}
	if s.Resolved {
		t.Fatalf("Press on charge-kind shouldn't resolve it")
	}
}

func TestTick_PressAutoMissesAtTimeout(t *testing.T) {
	withSeededRNG(t, 11, func(rng *rand.Rand) {
		s := NewTimingState(rng, 1.0)
		s.Tick(2.0)
		if !s.Resolved {
			t.Fatalf("press bar should resolve after timeout")
		}
		if s.Quality != TimingQualityMiss {
			t.Fatalf("untouched press bar should Miss, got %v", s.Quality)
		}
	})
}

func TestCharge_ReleaseAtSweetSpotIsExcellent(t *testing.T) {
	s := NewChargeState(2.0)
	s.Hold()
	s.Elapsed = s.SweetSpot
	if !s.Release() {
		t.Fatalf("release should resolve charge bar")
	}
	if s.Quality != TimingQualityExcellent {
		t.Fatalf("release at sweet spot should be Excellent, got %v", s.Quality)
	}
}

func TestCharge_GradeDispatch(t *testing.T) {
	// ChargeTickNPct / ChargePeakEnd are visual positions on the bar; we
	// pick a target cursor visual position per case and invert through
	// ChargeElapsedForVisual to set Elapsed. This keeps the test
	// expressed in the same coordinate system as the rendered bar.
	cases := []struct {
		name   string
		visual float32
		want   int
	}{
		{"before tick1", ChargeTick1Pct - 0.01, TimingQualityMiss},
		{"between tick1 and tick2", (ChargeTick1Pct + ChargeTick2Pct) / 2, TimingQualityNice},
		{"between tick2 and tick3", (ChargeTick2Pct + ChargeTick3Pct) / 2, TimingQualityGood},
		{"past peak window", ChargePeakEnd + 0.05, TimingQualityMiss},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewChargeState(2.0)
			s.Hold()
			s.Elapsed = ChargeElapsedForVisual(tc.visual, s.Duration)
			s.Release()
			if s.Quality != tc.want {
				t.Fatalf("got quality %v, want %v", s.Quality, tc.want)
			}
		})
	}
}

func TestCharge_ReleaseWithoutHoldIsNoOp(t *testing.T) {
	s := NewChargeState(2.0)
	if s.Release() {
		t.Fatalf("Release without Hold should not resolve")
	}
}

func TestCharge_TimeoutAfterHoldGrades(t *testing.T) {
	s := NewChargeState(1.0)
	s.Hold()
	// Tick past the peak window — held too long, should Miss.
	s.Tick(2.0)
	if !s.Resolved {
		t.Fatalf("charge bar should resolve at timeout")
	}
	if s.Quality != TimingQualityMiss {
		t.Fatalf("over-charged release should Miss, got %v", s.Quality)
	}
}

func TestCharge_TimeoutWithoutHoldIsMiss(t *testing.T) {
	s := NewChargeState(1.0)
	s.Tick(2.0)
	if s.Quality != TimingQualityMiss {
		t.Fatalf("idle charge bar should Miss, got %v", s.Quality)
	}
}

func TestSequence_AllCorrectFastIsExcellent(t *testing.T) {
	withSeededRNG(t, 99, func(rng *rand.Rand) {
		s := NewSequenceState(rng, 2.0, 4)
		s.Elapsed = SequenceFastThreshold - 0.1
		for _, dir := range s.SequenceTargets {
			s.SequenceInput(dir)
		}
		if !s.Resolved {
			t.Fatalf("sequence should resolve after all inputs")
		}
		if s.Quality != TimingQualityExcellent {
			t.Fatalf("all-correct under threshold should be Excellent, got %v", s.Quality)
		}
	})
}

func TestSequence_OneWrongDropsOneGrade(t *testing.T) {
	withSeededRNG(t, 99, func(rng *rand.Rand) {
		s := NewSequenceState(rng, 2.0, 4)
		s.Elapsed = 0 // skip the fast-bonus path entirely
		for i, dir := range s.SequenceTargets {
			if i == 0 {
				s.SequenceInput((dir + 1) % 4) // intentionally wrong
				continue
			}
			s.SequenceInput(dir)
		}
		if s.Quality != TimingQualityGreat {
			t.Fatalf("3/4 correct with no fast bonus should be Great, got %v", s.Quality)
		}
	})
}

func TestSequence_TimeoutMarksPendingAsWrong(t *testing.T) {
	withSeededRNG(t, 99, func(rng *rand.Rand) {
		// 1 correct + 3 pending == 3 dropped grades: Excellent - 3 = Nice.
		s := NewSequenceState(rng, 2.0, 4)
		s.SequenceInput(s.SequenceTargets[0])
		s.Tick(3.0)
		if !s.Resolved {
			t.Fatalf("sequence should resolve at timeout")
		}
		if s.Quality != TimingQualityNice {
			t.Fatalf("1/4 correct with 3 pending should be Nice, got %v", s.Quality)
		}
	})
}

func TestSequence_NoneCorrectIsMiss(t *testing.T) {
	withSeededRNG(t, 99, func(rng *rand.Rand) {
		s := NewSequenceState(rng, 2.0, 4)
		s.Tick(3.0)
		if s.Quality != TimingQualityMiss {
			t.Fatalf("0/4 correct should be Miss, got %v", s.Quality)
		}
	})
}

func TestSequence_InputAfterResolveIsNoOp(t *testing.T) {
	withSeededRNG(t, 99, func(rng *rand.Rand) {
		s := NewSequenceState(rng, 2.0, 2)
		s.SequenceInput(s.SequenceTargets[0])
		s.SequenceInput(s.SequenceTargets[1])
		if !s.Resolved {
			t.Fatalf("expected resolve after final input")
		}
		if s.SequenceInput(0) {
			t.Fatalf("input after resolve should be no-op")
		}
	})
}

func TestProgress_ClampsToUnitInterval(t *testing.T) {
	s := TimingState{Duration: 1.0, Elapsed: 0.5}
	if got := s.Progress(); got != 0.5 {
		t.Fatalf("progress should reflect Elapsed/Duration, got %v", got)
	}
	s.Elapsed = -1
	if got := s.Progress(); got != 0 {
		t.Fatalf("negative elapsed should clamp to 0, got %v", got)
	}
	s.Elapsed = 2
	if got := s.Progress(); got != 1 {
		t.Fatalf("over-duration should clamp to 1, got %v", got)
	}
	zero := TimingState{}
	if got := zero.Progress(); got != 0 {
		t.Fatalf("zero duration should return 0, got %v", got)
	}
}

func TestTimingBonusMult_Table(t *testing.T) {
	cases := []struct {
		quality int
		want    float32
	}{
		{TimingQualityMiss, timingGrades[TimingQualityMiss].Atk},
		{TimingQualityNice, timingGrades[TimingQualityNice].Atk},
		{TimingQualityGood, timingGrades[TimingQualityGood].Atk},
		{TimingQualityGreat, timingGrades[TimingQualityGreat].Atk},
		{TimingQualityExcellent, timingGrades[TimingQualityExcellent].Atk},
	}
	for _, tc := range cases {
		if got := TimingBonusMult(tc.quality); got != tc.want {
			t.Errorf("TimingBonusMult(%v) = %v, want %v", tc.quality, got, tc.want)
		}
	}
}

func TestTimingDefenseMult_Table(t *testing.T) {
	cases := []struct {
		quality int
		want    float32
	}{
		{TimingQualityMiss, timingGrades[TimingQualityMiss].Def},
		{TimingQualityNice, timingGrades[TimingQualityNice].Def},
		{TimingQualityGood, timingGrades[TimingQualityGood].Def},
		{TimingQualityGreat, timingGrades[TimingQualityGreat].Def},
		{TimingQualityExcellent, timingGrades[TimingQualityExcellent].Def},
	}
	for _, tc := range cases {
		if got := TimingDefenseMult(tc.quality); got != tc.want {
			t.Errorf("TimingDefenseMult(%v) = %v, want %v", tc.quality, got, tc.want)
		}
	}
}

func TestScaleDamage_ExcellentAlwaysGains(t *testing.T) {
	// On a 0 base, Excellent should still land 1 damage.
	if got := ScaleDamage(0, TimingQualityExcellent); got != 1 {
		t.Fatalf("ScaleDamage(0, Excellent) = %d, want >=1", got)
	}
	// On a non-zero base, Excellent doubles.
	if got := ScaleDamage(5, TimingQualityExcellent); got != 10 {
		t.Fatalf("ScaleDamage(5, Excellent) = %d, want 10", got)
	}
	// Miss leaves base alone.
	if got := ScaleDamage(5, TimingQualityMiss); got != 5 {
		t.Fatalf("ScaleDamage(5, Miss) = %d, want 5", got)
	}
}

func TestScaleHeal_ClampsAtZero(t *testing.T) {
	if got := ScaleHeal(0, TimingQualityExcellent); got != 0 {
		t.Fatalf("ScaleHeal(0, Excellent) should clamp to 0, got %d", got)
	}
	if got := ScaleHeal(4, TimingQualityGood); got != 6 {
		t.Fatalf("ScaleHeal(4, Good) = %d, want 6", got)
	}
}

func TestScaleIncomingDamage_DefendedHitNeverHeals(t *testing.T) {
	if got := ScaleIncomingDamage(10, TimingQualityExcellent); got != 2 {
		t.Fatalf("ScaleIncomingDamage(10, Excellent) = %d, want 2", got)
	}
	if got := ScaleIncomingDamage(0, TimingQualityExcellent); got != 0 {
		t.Fatalf("ScaleIncomingDamage(0, Excellent) should stay at 0, got %d", got)
	}
}

func TestTimingQualityLabel_AllGrades(t *testing.T) {
	cases := map[int]string{
		TimingQualityMiss:      "Miss...",
		TimingQualityNice:      "Nice!",
		TimingQualityGood:      "Good!",
		TimingQualityGreat:     "Great!",
		TimingQualityExcellent: "Excellent!",
	}
	for q, want := range cases {
		if got := TimingQualityLabel(q); got != want {
			t.Errorf("TimingQualityLabel(%v) = %q, want %q", q, got, want)
		}
	}
}

// TestPreviewQuality_MatchesResolve sweeps the bar's Elapsed across the full
// duration in fine steps; at each step we compare PreviewQuality (the live
// cursor read) against the grade the bar would actually score if Press()
// fired right now. They must agree everywhere — otherwise the cursor color
// lies about the player's pending grade. Guards against the resolve() /
// PreviewQuality threshold drift the original code had as two duplicated
// switch tables.
func TestPreviewQuality_MatchesResolve(t *testing.T) {
	withSeededRNG(t, 13, func(rng *rand.Rand) {
		base := NewTimingState(rng, 2.0)
		steps := 400
		for i := 0; i <= steps; i++ {
			elapsed := float32(i) / float32(steps) * base.Duration
			preview := base
			preview.Elapsed = elapsed
			expected := preview.PreviewQuality()

			scored := base
			scored.Elapsed = elapsed
			scored.Press()

			if expected != scored.Quality {
				t.Fatalf("at elapsed=%v: preview=%v but Press graded %v",
					elapsed, expected, scored.Quality)
			}
		}
	})
}

// TestHitStopFor_OnlyOnHighGrades pins the fast-path: Miss / Nice / Good get
// no freeze (HitStopFor returns 0, so tickFlashHold fires onResolve the
// moment flash hits zero). Great / Excellent each get their own pause so
// the contrast between "fine hit" and "great hit" reads on impact.
func TestHitStopFor_OnlyOnHighGrades(t *testing.T) {
	cases := []struct {
		quality int
		want    float32
	}{
		{TimingQualityMiss, 0},
		{TimingQualityNice, 0},
		{TimingQualityGood, 0},
		{TimingQualityGreat, HitStopGreat},
		{TimingQualityExcellent, HitStopExcellent},
	}
	for _, tc := range cases {
		if got := HitStopFor(tc.quality); got != tc.want {
			t.Errorf("HitStopFor(%v) = %v, want %v", tc.quality, got, tc.want)
		}
	}
	if HitStopExcellent <= HitStopGreat {
		t.Fatalf("Excellent should freeze longer than Great (got %v vs %v)",
			HitStopExcellent, HitStopGreat)
	}
}

// TestMeleeAccuracy_Curve checks the headline guarantees of the basic
// (melee) attack's hit curve, now STR-driven:
//
//   - low-STR (STR 2) on Miss timing lands in the 0.55–0.70 band
//   - Excellent timing always pushes accuracy to a clamped 1.0
//   - any out-of-table quality (negative, unknown) falls back to base
//   - accuracy stays inside [0, 1]
func TestMeleeAccuracy_Curve(t *testing.T) {
	low := Stats{STR: 2}
	high := Stats{STR: 6}

	if got := MeleeAccuracy(low, TimingQualityMiss); got < 0.55 || got > 0.70 {
		t.Errorf("low-STR Miss should sit ~0.63, got %v", got)
	}
	if got := MeleeAccuracy(low, TimingQualityExcellent); got != 1.0 {
		t.Errorf("low-STR Excellent should clamp to 1.0, got %v", got)
	}
	if got := MeleeAccuracy(high, TimingQualityExcellent); got != 1.0 {
		t.Errorf("high-STR Excellent should clamp to 1.0, got %v", got)
	}
	if got := MeleeAccuracy(high, TimingQualityMiss); got <= MeleeAccuracy(low, TimingQualityMiss) {
		t.Errorf("higher STR should out-accuracy lower STR on the same grade")
	}
	if got := MeleeAccuracy(low, -42); got != MeleeAccuracy(low, TimingQualityMiss) {
		t.Errorf("unknown quality should fall back to Miss baseline, got %v", got)
	}
	// RangedAccuracy mirrors the same curve off DEX (the seam for ranged
	// attacks); confirm it tracks DEX, not STR.
	if got := RangedAccuracy(Stats{DEX: 6}, TimingQualityMiss); got <= RangedAccuracy(Stats{DEX: 2}, TimingQualityMiss) {
		t.Errorf("RangedAccuracy should scale with DEX")
	}
}

// TestMemberAttackHits_StatisticsRoughlyMatch fires MemberAttackHits many
// times against a known seed and asserts the hit rate is close to the
// member's accuracy curve. The member is unarmed (no equipped weapon), so
// the basic attack is STR-governed and the expected rate is
// MeleeAccuracy(STR). Not a precise distribution check — just a sanity
// guard that the dice are actually rolled against the curve, not a constant.
func TestMemberAttackHits_StatisticsRoughlyMatch(t *testing.T) {
	withSeededRNG(t, 2024, func(rng *rand.Rand) {
		m := PartyMember{Stats: Stats{STR: 2}} // unarmed → STR governs
		quality := TimingQualityNice
		expected := MeleeAccuracy(m.Stats, quality)
		hits := 0
		const trials = 4000
		for i := 0; i < trials; i++ {
			if MemberAttackHits(rng, m, quality) {
				hits++
			}
		}
		rate := float64(hits) / float64(trials)
		if rate < expected-0.04 || rate > expected+0.04 {
			t.Fatalf("hit rate %v drifted from expected %v over %d trials", rate, expected, trials)
		}
	})
}

// TestMemberAttackHits_ExcellentNeverWhiffs locks the contract that any
// stat + Excellent timing combination always lands. The accuracy clamps to
// 1.0 past that point, so the RNG roll should never come back false. The
// member is unarmed, so STR is the governing stat being swept.
func TestMemberAttackHits_ExcellentNeverWhiffs(t *testing.T) {
	withSeededRNG(t, 9, func(rng *rand.Rand) {
		for str := 0; str <= 10; str++ {
			m := PartyMember{Stats: Stats{STR: str}} // unarmed → STR governs
			for i := 0; i < 200; i++ {
				if !MemberAttackHits(rng, m, TimingQualityExcellent) {
					t.Fatalf("Excellent should always hit (STR=%d, trial=%d)", str, i)
				}
			}
		}
	})
}

// --- Reels (slot) minigame -------------------------------------------------

// reelsFromStops builds a []Reel with the given locked Stop values (Speed /
// Offset unused once stopped) — for grading tests that set the result directly.
func reelsFromStops(stops ...int) []Reel {
	reels := make([]Reel, len(stops))
	for i, s := range stops {
		reels[i] = Reel{Stop: s}
	}
	return reels
}

// TestReels_MatchTiers pins the payout grading directly via the stop values:
// three matching = Excellent (jackpot), exactly two = Good, all distinct =
// Miss (a real whiff).
func TestReels_MatchTiers(t *testing.T) {
	cases := []struct {
		name  string
		stops []int
		want  int
	}{
		{"three of a kind", []int{2, 2, 2}, TimingQualityExcellent},
		{"a pair", []int{1, 3, 1}, TimingQualityGood},
		{"all distinct", []int{0, 1, 2}, TimingQualityMiss},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Pressed: true — grade the match tiers as a PLAYED bar; a no-play
			// (!Pressed) reels bar is Miss regardless of symbols (see
			// TestReels_NoPlayIsMiss).
			s := &TimingState{Kind: TimingKindReels, Active: true, Pressed: true, Reels: reelsFromStops(tc.stops...)}
			s.resolveReels()
			if !s.Resolved || s.Quality != tc.want {
				t.Fatalf("stops %v graded %v, want %v", tc.stops, s.Quality, tc.want)
			}
		})
	}
}

// TestReels_StopLocksAndResolves checks each press locks the next spinning
// reel and the final stop resolves the bar.
func TestReels_StopLocksAndResolves(t *testing.T) {
	withSeededRNG(t, 5, func(rng *rand.Rand) {
		s := NewReelState(rng, 2.0)
		if got := len(s.Reels); got != ReelCount {
			t.Fatalf("expected %d reels, got %d", ReelCount, got)
		}
		for i := 0; i < ReelCount; i++ {
			last := s.StopNextReel()
			if s.Reels[i].Stop < 0 {
				t.Fatalf("reel %d should be locked after a stop", i)
			}
			wantLast := i == ReelCount-1
			if last != wantLast {
				t.Fatalf("StopNextReel resolve=%v at reel %d, want %v", last, i, wantLast)
			}
		}
		if !s.Resolved {
			t.Fatalf("bar should resolve once every reel is stopped")
		}
		// A press past the last reel is a no-op.
		if s.StopNextReel() {
			t.Fatalf("stop after resolve should be a no-op")
		}
	})
}

// TestReels_TimeoutLocksSpinningReels checks an un-played bar still resolves
// (locking whatever's showing) and reads as a no-play Miss-ish result.
func TestReels_TimeoutLocksSpinning(t *testing.T) {
	withSeededRNG(t, 6, func(rng *rand.Rand) {
		s := NewReelState(rng, 1.0)
		s.Tick(2.0)
		if !s.Resolved {
			t.Fatalf("reel bar should resolve at timeout")
		}
		for i := range s.Reels {
			if s.Reels[i].Stop < 0 {
				t.Fatalf("reel %d still spinning after timeout", i)
			}
		}
		if s.Pressed {
			t.Fatalf("a timed-out bar with no stop should not read as Pressed")
		}
	})
}

// --- Recall (memory) minigame ----------------------------------------------

// TestRecall_RevealGate confirms the pattern stays hidden until RevealTime
// elapses, then opens — and that recall input grades through the shared
// sequence resolve.
func TestRecall_RevealGateAndGrade(t *testing.T) {
	withSeededRNG(t, 8, func(rng *rand.Rand) {
		s := NewRecallState(rng, 3.0, 4, 1.2)
		if s.Kind != TimingKindRecall {
			t.Fatalf("expected recall kind")
		}
		if s.RecallHidden() {
			t.Fatalf("pattern should be visible (not hidden) at elapsed 0")
		}
		s.Elapsed = s.RevealTime + 0.01
		if !s.RecallHidden() {
			t.Fatalf("pattern should hide once past RevealTime")
		}
		// Reproduce it perfectly during the hidden (input) phase. Recall input
		// can only ever land at Elapsed >= RevealTime (gated by the battle
		// loop), which is past SequenceFastThreshold — so this asserts the
		// Recall-specific rule that a flawless recall reaches Excellent
		// regardless of clock (the speed demotion is Sequence-only). A
		// realistic in-phase Elapsed is used, not the impossible sub-reveal
		// value the earlier test set.
		s.Elapsed = s.RevealTime + 0.5
		if s.Elapsed < SequenceFastThreshold {
			t.Fatalf("test premise broken: recall input must land past SequenceFastThreshold")
		}
		for _, dir := range s.SequenceTargets {
			s.SequenceInput(dir)
		}
		if !s.Resolved || s.Quality != TimingQualityExcellent {
			t.Fatalf("perfect recall should be Excellent regardless of clock, got resolved=%v q=%v", s.Resolved, s.Quality)
		}
	})
}

// TestRecall_NoSpeedDemotion pins that the Sequence-only fast-threshold
// demotion does NOT apply to Recall: a flawless recall finished well after
// SequenceFastThreshold still grades Excellent (regression guard for the bug
// where Recall could never reach Excellent).
func TestRecall_NoSpeedDemotion(t *testing.T) {
	s := &TimingState{
		Kind:            TimingKindRecall,
		SequenceTargets: []int{SeqDirUp, SeqDirDown},
		SequenceResults: []int{SeqResultCorrect, SeqResultCorrect},
		Elapsed:         SequenceFastThreshold + 5, // deliberately "slow"
	}
	s.resolveSequence()
	if s.Quality != TimingQualityExcellent {
		t.Fatalf("flawless recall should stay Excellent past the fast threshold, got %v", s.Quality)
	}
	// The same results on a Sequence bar DO get demoted to Great.
	seq := &TimingState{
		Kind:            TimingKindSequence,
		SequenceTargets: []int{SeqDirUp, SeqDirDown},
		SequenceResults: []int{SeqResultCorrect, SeqResultCorrect},
		Elapsed:         SequenceFastThreshold + 5,
	}
	seq.resolveSequence()
	if seq.Quality != TimingQualityGreat {
		t.Fatalf("flawless-but-slow Sequence should demote to Great, got %v", seq.Quality)
	}
}

// TestReels_NoPlayIsMiss pins finding #1's fix: a reels bar that times out
// with no reel ever stopped grades Miss outright, even if the random
// auto-locked symbols happen to match — a walk-away can't earn a steal.
func TestReels_NoPlayIsMiss(t *testing.T) {
	// Hand-set three matching stops but leave Pressed=false (no player stop).
	s := &TimingState{Kind: TimingKindReels, Active: true, Reels: reelsFromStops(2, 2, 2)}
	s.resolveReels()
	if s.Quality != TimingQualityMiss {
		t.Fatalf("no-play reels (even with matching symbols) should be Miss, got %v", s.Quality)
	}
	// With a player stop recorded, the same triple grades Excellent.
	s2 := &TimingState{Kind: TimingKindReels, Active: true, Reels: reelsFromStops(2, 2, 2), Pressed: true}
	s2.resolveReels()
	if s2.Quality != TimingQualityExcellent {
		t.Fatalf("played triple should be Excellent, got %v", s2.Quality)
	}
}

// --- Overcharge minigame ---------------------------------------------------

// TestOvercharge_GradesAndOverloadBand confirms the pre-peak/peak grades match
// a normal charge, and that releasing PAST the peak overloads (Excellent +
// Overloaded) instead of the normal charge's decay-to-Miss.
func TestOvercharge_GradesAndOverloadBand(t *testing.T) {
	mk := func(visual float32) *TimingState {
		s := NewOverchargeState(2.0)
		s.Hold()
		s.Elapsed = ChargeElapsedForVisual(visual, s.Duration)
		return &s
	}
	// Sweet-spot release: Excellent, NOT overloaded.
	s := mk((ChargePeakStart + ChargePeakEnd) / 2)
	s.Release()
	if s.Quality != TimingQualityExcellent || s.Overloaded {
		t.Fatalf("peak release should be clean Excellent, got q=%v overloaded=%v", s.Quality, s.Overloaded)
	}
	// Before tick1: Miss, not overloaded.
	s = mk(ChargeTick1Pct - 0.01)
	s.Release()
	if s.Quality != TimingQualityMiss || s.Overloaded {
		t.Fatalf("early release should Miss without overload, got q=%v overloaded=%v", s.Quality, s.Overloaded)
	}
	// Past the peak: OVERLOAD (Excellent + Overloaded) where a normal charge
	// would have decayed to Miss.
	s = mk(ChargePeakEnd + 0.05)
	s.Release()
	if !s.Overloaded || s.Quality != TimingQualityExcellent {
		t.Fatalf("past-peak release should overload to Excellent, got q=%v overloaded=%v", s.Quality, s.Overloaded)
	}
	// A normal charge at the same spot must still Miss (no overload field).
	nc := NewChargeState(2.0)
	nc.Hold()
	nc.Elapsed = ChargeElapsedForVisual(ChargePeakEnd+0.05, nc.Duration)
	nc.Release()
	if nc.Quality != TimingQualityMiss || nc.Overloaded {
		t.Fatalf("normal charge past peak should Miss without overload, got q=%v overloaded=%v", nc.Quality, nc.Overloaded)
	}
}
