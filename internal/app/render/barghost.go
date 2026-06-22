package render

import rl "github.com/gen2brain/raylib-go/raylib"

// Damage ghost-drain for the party ribbon's HP gauges: a hit's lost segment
// holds at the old level (barGhostHold) then drains to the real value
// (barGhostDrainPerSec) so the bite's size reads at a glance.
//
// Presentation-only state (NOT on GameState), keyed by string identity
// ("hp:Warrior"). Only losses trail — heals/loads snap up. Cleared with the
// particle pool on VFX reset so a mid-drain ghost can't carry into the next scene.

const (
	// barGhostHold: freeze beat after a hit; short enough that rapid multi-hits chain into one read.
	barGhostHold = float32(0.35)
	// barGhostDrainPerSec: drain rate in bar-fractions/sec; full-bar loss sweeps in ~0.8s.
	barGhostDrainPerSec = float32(1.25)
	// barGhostMin: smallest ghost-vs-actual gap worth drawing; below it the ghost snaps closed.
	barGhostMin = float32(0.004)
)

type barGhost struct {
	ghost float32 // displayed trailing level (>= last)
	last  float32 // actual level seen last frame
	hold  float32 // remaining freeze before the drain starts
}

var barGhosts = make(map[string]*barGhost, 8)

// resetBarGhosts drops all trailing state; called with the particle/glyph reset.
func resetBarGhosts() {
	for k := range barGhosts {
		delete(barGhosts, k)
	}
}

// ghostPctFor advances the trailing level for key toward pct and returns the
// ghost level to draw, or -1 when none should render. Ticked with clamped frame
// dt so a debugger stall can't fast-forward a drain.
func ghostPctFor(key string, pct float32) float32 {
	g, ok := barGhosts[key]
	if !ok {
		// First sighting: seed at current level, no ghost (no phantom drain on first frame).
		barGhosts[key] = &barGhost{ghost: pct, last: pct}
		return -1
	}
	if pct < g.last-barGhostMin {
		// Fresh damage: freeze trail at max(old trail, old actual), restart hold; chained hits extend it.
		if g.last > g.ghost {
			g.ghost = g.last
		}
		g.hold = barGhostHold
	} else if pct > g.ghost {
		// Heal past the trail: snap shut. Only losses trail.
		g.ghost = pct
		g.hold = 0
	}
	g.last = pct

	if g.ghost > pct {
		dt := clampFrameDelta(rl.GetFrameTime())
		if g.hold > 0 {
			g.hold -= dt
		} else {
			g.ghost -= barGhostDrainPerSec * dt
			if g.ghost < pct {
				g.ghost = pct
			}
		}
	}
	if g.ghost > pct+barGhostMin {
		return g.ghost
	}
	return -1
}
