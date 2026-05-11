package core

import (
	"math/rand"
	"testing"
)

// Tests use a deterministic RNG so press-window placement, sequence targets,
// and burn rolls become reproducible. NewTimingState / NewSequenceState read
// GameRNG, so we swap it for the duration of the test and restore on exit.
func withSeededRNG(t *testing.T, seed int64, fn func()) {
	t.Helper()
	saved := GameRNG
	GameRNG = rand.New(rand.NewSource(seed))
	defer func() { GameRNG = saved }()
	fn()
}

func TestNewTimingState_ZeroDurationDefaults(t *testing.T) {
	withSeededRNG(t, 1, func() {
		s := NewTimingState(0)
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
	withSeededRNG(t, 42, func() {
		for i := 0; i < 50; i++ {
			s := NewTimingState(2.0)
			startPct := s.WindowStart / s.Duration
			endPct := s.WindowEnd / s.Duration
			if startPct < PressWindowMinStart-0.001 {
				t.Fatalf("window opened before PressWindowMinStart: %v", startPct)
			}
			if endPct > 0.961 {
				t.Fatalf("window crossed tail: %v", endPct)
			}
			width := endPct - startPct
			if width < PressWindowWidth-0.001 || width > PressWindowWidth+0.001 {
				t.Fatalf("window width drifted from PressWindowWidth: %v", width)
			}
		}
	})
}

func TestPress_InWindowExcellentAtSweetSpot(t *testing.T) {
	withSeededRNG(t, 7, func() {
		s := NewTimingState(2.0)
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
	withSeededRNG(t, 7, func() {
		s := NewTimingState(2.0)
		s.Elapsed = 0
		s.Press()
		if s.Quality != TimingQualityMiss {
			t.Fatalf("press before window should be Miss, got %v", s.Quality)
		}
	})
}

func TestPress_OnlyResolvesOnce(t *testing.T) {
	withSeededRNG(t, 7, func() {
		s := NewTimingState(2.0)
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
	withSeededRNG(t, 11, func() {
		s := NewTimingState(1.0)
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
	cases := []struct {
		name    string
		elapsed float32
		want    int
	}{
		{"before tick1", ChargeTick1Pct - 0.01, TimingQualityMiss},
		{"between tick1 and tick2", (ChargeTick1Pct + ChargeTick2Pct) / 2, TimingQualityNice},
		{"between tick2 and tick3", (ChargeTick2Pct + ChargeTick3Pct) / 2, TimingQualityGood},
		{"past peak window", 0.95, TimingQualityMiss},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewChargeState(2.0)
			s.Hold()
			s.Elapsed = tc.elapsed * s.Duration
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
	withSeededRNG(t, 99, func() {
		s := NewSequenceState(2.0, 4)
		s.Elapsed = StealFastThreshold - 0.1
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
	withSeededRNG(t, 99, func() {
		s := NewSequenceState(2.0, 4)
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
	withSeededRNG(t, 99, func() {
		// 1 correct + 3 pending == 3 dropped grades: Excellent - 3 = Nice.
		s := NewSequenceState(2.0, 4)
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
	withSeededRNG(t, 99, func() {
		s := NewSequenceState(2.0, 4)
		s.Tick(3.0)
		if s.Quality != TimingQualityMiss {
			t.Fatalf("0/4 correct should be Miss, got %v", s.Quality)
		}
	})
}

func TestSequence_InputAfterResolveIsNoOp(t *testing.T) {
	withSeededRNG(t, 99, func() {
		s := NewSequenceState(2.0, 2)
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
		{TimingQualityMiss, TimingMultMiss},
		{TimingQualityNice, TimingMultNice},
		{TimingQualityGood, TimingMultGood},
		{TimingQualityGreat, TimingMultGreat},
		{TimingQualityExcellent, TimingMultExcellent},
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
		{TimingQualityMiss, TimingDefMiss},
		{TimingQualityNice, TimingDefNice},
		{TimingQualityGood, TimingDefGood},
		{TimingQualityGreat, TimingDefGreat},
		{TimingQualityExcellent, TimingDefExcellent},
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
