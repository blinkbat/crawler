package core

// TimeOfDay names a phase in the day/night cycle: 6 phases of StepsPerPhase
// steps each, ordered dawn-first.
type TimeOfDay int

const (
	Dawn TimeOfDay = iota
	Morning
	Afternoon
	Dusk
	Evening
	Midnight
)

// TimeOfDayCount is the wrap modulus for cycling phases; bump by adding a
// TimeOfDay constant above this line.
const TimeOfDayCount = int(Midnight) + 1

// PhaseAtStep returns the current phase and intra-phase progress in [0,1)
// (0 = just entered, →1 before the next phase), for smooth lighting interp.
func PhaseAtStep(steps int) (phase TimeOfDay, progress float32) {
	if steps < 0 {
		steps = 0
	}
	cycle := steps % StepsPerCycle
	phase = TimeOfDay(cycle / StepsPerPhase)
	progress = float32(cycle%StepsPerPhase) / float32(StepsPerPhase)
	return phase, progress
}

// phaseNames is the HUD label per phase, indexed by TimeOfDay. A missing label
// leaves a "" entry the init() below catches at startup.
var phaseNames = [TimeOfDayCount]string{
	Dawn:      "Dawn",
	Morning:   "Morning",
	Afternoon: "Afternoon",
	Dusk:      "Dusk",
	Evening:   "Evening",
	Midnight:  "Midnight",
}

func init() {
	for _, n := range phaseNames {
		if n == "" {
			panic("core: phaseNames is missing a label for a TimeOfDay phase — add a row to the keyed array")
		}
	}
}

// PhaseName returns the HUD label, or "" for an out-of-range phase (defensive:
// in-package callers feed 0..TimeOfDayCount-1, but guard a stray external value).
func PhaseName(p TimeOfDay) string {
	if p < 0 || int(p) >= TimeOfDayCount {
		return ""
	}
	return phaseNames[p]
}
