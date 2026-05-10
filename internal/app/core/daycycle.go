package core

// TimeOfDay names a phase in the day/night cycle. Six phases of 25 steps
// each (StepsPerPhase) make up one full loop (StepsPerCycle = 150). The
// order matches the natural progression of a day starting at dawn.
type TimeOfDay int

const (
	Dawn TimeOfDay = iota
	Morning
	Afternoon
	Dusk
	Evening
	Midnight

	StepsPerPhase = 25
	StepsPerCycle = StepsPerPhase * 6
)

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

// PhaseName returns the human-readable label rendered in the HUD.
func PhaseName(p TimeOfDay) string {
	switch p {
	case Dawn:
		return "Dawn"
	case Morning:
		return "Morning"
	case Afternoon:
		return "Afternoon"
	case Dusk:
		return "Dusk"
	case Evening:
		return "Evening"
	case Midnight:
		return "Midnight"
	}
	return "?"
}
