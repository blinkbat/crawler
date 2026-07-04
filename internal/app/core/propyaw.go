package core

import "crawler/internal/app/core/mapfile"

// propyaw.go — the optional per-tile PROP-orientation override. PropYaw is a
// Height×Width char grid: '.' (PropYawAuto) = use the renderer's procedural
// hash-yaw, else a base-36-ish step digit ('0'..'9','a'..) in [0,PropYawSteps).
// Absent/nil = every tile auto, so pre-orientation maps are unchanged. Per-(x,z),
// not per-floor: a stacked column shares one authored facing (a rare case).

const (
	// PropYawAuto marks a tile with no authored facing (procedural hash yaw).
	PropYawAuto = byte('.')
	// PropYawSteps is the authored-orientation resolution — 30° increments, matching
	// the renderer's steppedYaw30 so authored and procedural facings share the grid.
	PropYawSteps = 12
)

// mapfile.validatePropYawGrid derives its accepted char alphabet from its own
// PropYawStepCount (mapfile can't import core), so keep the two in lockstep — else
// the disk validator and the runtime decoder would accept/reject different chars.
func init() {
	if mapfile.PropYawStepCount != PropYawSteps {
		panic("core: mapfile.PropYawStepCount must equal PropYawSteps — the prop_yaw validator and decoder would drift")
	}
}

// PropYawStepChar encodes a yaw step (0..PropYawSteps-1) as one grid char; an
// out-of-range step encodes as PropYawAuto ('.').
func PropYawStepChar(step int) byte {
	if step < 0 || step >= PropYawSteps {
		return PropYawAuto
	}
	if step < 10 {
		return byte('0' + step)
	}
	return byte('a' + step - 10)
}

// propYawStepFromChar decodes a grid char to a step; ok=false for '.'/invalid.
func propYawStepFromChar(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'z':
		if step := int(c-'a') + 10; step < PropYawSteps {
			return step, true
		}
	}
	return 0, false
}

// PropYawDegForStep is the yaw in degrees for a step (30° per step).
func PropYawDegForStep(step int) float32 {
	return float32((((step % PropYawSteps) + PropYawSteps) % PropYawSteps) * (360 / PropYawSteps))
}

// PropYawOverride returns the authored prop yaw (degrees) at (x,z) and whether one is
// set. ok=false means "no override" — the renderer falls back to its procedural yaw.
func (a *AreaDefinition) PropYawOverride(x, z int) (float32, bool) {
	if z < 0 || z >= len(a.PropYaw) {
		return 0, false
	}
	row := a.PropYaw[z]
	if x < 0 || x >= len(row) {
		return 0, false
	}
	step, ok := propYawStepFromChar(row[x])
	if !ok {
		return 0, false
	}
	return PropYawDegForStep(step), true
}

// PropYawStepAt returns the authored yaw STEP at (x,z), or -1 for auto — the editor
// reads this to cycle a tile's facing.
func (a *AreaDefinition) PropYawStepAt(x, z int) int {
	if z < 0 || z >= len(a.PropYaw) {
		return -1
	}
	row := a.PropYaw[z]
	if x < 0 || x >= len(row) {
		return -1
	}
	if step, ok := propYawStepFromChar(row[x]); ok {
		return step
	}
	return -1
}

// trimAllAutoGrid returns nil when every cell of an optional per-tile grid is the
// blank/auto char (or the grid is absent), so an all-auto grid isn't serialized —
// keeping maps without authored facings byte-identical to the pre-feature format.
func trimAllAutoGrid(rows []string, blank byte) []string {
	for _, r := range rows {
		for i := 0; i < len(r); i++ {
			if r[i] != blank {
				return rows
			}
		}
	}
	return nil
}

// SetPropYawStep writes the yaw override at (x,z): step<0 (or out of range) clears it
// to auto. Clearing an already-absent grid stays nil (byte-stable). Materializes the
// grid on first real write.
func (a *AreaDefinition) SetPropYawStep(x, z, step int) {
	if !a.InBounds(x, z) {
		return
	}
	c := PropYawStepChar(step)
	if c == PropYawAuto && len(a.PropYaw) == 0 {
		return
	}
	a.PropYaw = normalizeOptionalLayer(a.PropYaw, a.Width, a.Height, PropYawAuto)
	if z >= len(a.PropYaw) {
		return
	}
	row := []byte(a.PropYaw[z])
	if x >= len(row) {
		return
	}
	row[x] = c
	a.PropYaw[z] = string(row)
}
