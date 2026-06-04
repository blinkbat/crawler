package core

// TimeOfDay names a phase in the day/night cycle. Six phases of StepsPerPhase
// player tile-steps each (see config.go) make up one full loop. The order
// matches the natural progression of a day starting at dawn.
type TimeOfDay int

const (
	Dawn TimeOfDay = iota
	Morning
	Afternoon
	Dusk
	Evening
	Midnight
)

// TimeOfDayCount is the wrap modulus for cycling through phases — matches the
// pattern used by PauseMenuCount / ActionRowCount in config.go. Bump by adding
// a TimeOfDay constant above this line; neither caller hard-codes "6" anywhere.
const TimeOfDayCount = int(Midnight) + 1

// PhaseAtStep returns the current phase and the intra-phase progress in
// [0,1). progress=0 means the player just entered the phase; progress
// approaches 1 right before the next phase starts. Used by the renderer
// to interpolate lighting smoothly across the boundary instead of
// snapping every 25 steps.
func PhaseAtStep(steps int) (phase TimeOfDay, progress float32) {
	if steps < 0 {
		steps = 0
	}
	cycle := steps % StepsPerCycle
	phase = TimeOfDay(cycle / StepsPerPhase)
	progress = float32(cycle%StepsPerPhase) / float32(StepsPerPhase)
	return phase, progress
}

// phaseNames is the human-readable HUD label per phase, indexed by TimeOfDay.
// Table (not a switch) so it parallels the render-side timeProfiles[TimeOfDayCount]
// array and is guarded by the same kind of init length/coverage assert the
// other enum→string tables use (packAINameTable, doorStyleNameTable). A new
// phase that forgets a label leaves a "" entry the init() below catches at
// startup, rather than panicking only when that phase is reached at runtime.
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

// PhaseName returns the human-readable label rendered in the HUD. An
// out-of-range phase index panics (array bounds) — like the prior switch's
// default case, invalid input fails loud rather than rendering a blank.
func PhaseName(p TimeOfDay) string {
	return phaseNames[p]
}
