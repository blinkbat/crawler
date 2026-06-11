package render

import rl "github.com/gen2brain/raylib-go/raylib"

// Damage ghost-drain for the party ribbon's HP gauges — the classic fighting-
// game "white-hot trailing bar." When a member takes a hit, the lost segment
// doesn't vanish: it holds for a beat at the old level (barGhostHold), then
// drains down to the real value (barGhostDrainPerSec), so the SIZE of the bite
// reads at a glance even when the eye was on the timing bar, not the ribbon.
//
// Presentation-only render state (like the hit-glyph pool, NOT on GameState),
// keyed by a caller-supplied string identity ("hp:Warrior"). Heals and
// loads/restarts snap the ghost up instantly — only losses trail. The pool is
// bounded by the number of distinct keys (4 party HP bars today), so there's
// no growth concern; it's still cleared alongside the particle pool on a VFX
// reset (battle exit / area transition) so a mid-drain ghost can't carry a
// stale read into the next scene.

const (
	// barGhostHold is the "freeze at the old level" beat after a hit, long
	// enough to register the bite but short enough that rapid multi-hits
	// (Swipe passes) chain into one read.
	barGhostHold = float32(0.35)
	// barGhostDrainPerSec is the drain rate in bar-fractions per second once
	// the hold elapses — a full-bar loss sweeps away in ~0.8 s.
	barGhostDrainPerSec = float32(1.25)
	// barGhostMin is the smallest ghost-vs-actual gap worth drawing; below it
	// the ghost snaps closed instead of rendering a sub-pixel sliver.
	barGhostMin = float32(0.004)
)

type barGhost struct {
	ghost float32 // displayed trailing level (>= last)
	last  float32 // actual level seen last frame
	hold  float32 // remaining freeze before the drain starts
}

var barGhosts = make(map[string]*barGhost, 8)

// resetBarGhosts drops all trailing state — called with the particle/glyph
// reset so battle exit or an area transition never carries a mid-drain ghost
// into the next scene.
func resetBarGhosts() {
	for k := range barGhosts {
		delete(barGhosts, k)
	}
}

// ghostPctFor advances the trailing level for `key` toward the actual `pct`
// and returns the ghost level to draw, or -1 when no ghost should render
// (steady, healing, or fully drained). Ticked from the draw path with
// raylib's frame dt — same clamp discipline as the VFX pools so a debugger
// stall can't fast-forward a drain into a teleport.
func ghostPctFor(key string, pct float32) float32 {
	g, ok := barGhosts[key]
	if !ok {
		// First sighting: seed at the current level, no ghost. A bar's first
		// frame (new game, load, scene return) never shows a phantom drain.
		barGhosts[key] = &barGhost{ghost: pct, last: pct}
		return -1
	}
	if pct < g.last-barGhostMin {
		// Fresh damage this frame: freeze the trail at the higher of (old
		// trail, old actual) and restart the hold beat. Chained hits extend
		// the hold so a flurry reads as one accumulating bite.
		if g.last > g.ghost {
			g.ghost = g.last
		}
		g.hold = barGhostHold
	} else if pct > g.ghost {
		// Heal / restore past the trail: snap shut. Only losses trail.
		g.ghost = pct
		g.hold = 0
	}
	g.last = pct

	if g.ghost > pct {
		dt := rl.GetFrameTime()
		if dt > 1.0/15.0 {
			dt = 1.0 / 15.0
		}
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
