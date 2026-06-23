package core

// BattleWipeKind selects the screen-wipe FX played entering battle (and previewable
// in Debug ▸ Screen Wipe FX). Each manipulates the captured scene image as it settles
// into the fight — render owns the per-kind blit. Append-only ordering isn't required
// (not serialized to saves), but keep WipeNone first as the zero value / "off".
type BattleWipeKind int

const (
	WipeNone     BattleWipeKind = iota // instant, no FX
	WipeZoom                           // camera punch: zooms in then settles
	WipeSpin                           // camera roll: spins upright into place
	WipeWobble                         // camera wobble that damps out
	WipeTint                           // warm color wash that clears
	WipeFlash                          // white flash that fades
	WipeVignette                       // dark iris that opens from the edges
	WipePixelate                       // scene snapshot mosaics chunky→fine, then clears
	wipeKindCount
)

// BattleWipeKindCount is the number of wipe kinds.
const BattleWipeKindCount = int(wipeKindCount)

// BattleWipePreviewSeconds is how long a Debug ▸ Screen Wipe FX preview plays (and
// the battle-entry wipe window). Shared by the render timing + the menu's arm.
const BattleWipePreviewSeconds = float32(0.5)

var battleWipeNames = [BattleWipeKindCount]string{
	WipeNone:     "None",
	WipeZoom:     "Zoom",
	WipeSpin:     "Spin",
	WipeWobble:   "Wobble",
	WipeTint:     "Tint Wash",
	WipeFlash:    "Flash",
	WipeVignette: "Vignette",
	WipePixelate: "Pixel Blur",
}

func init() {
	for _, n := range battleWipeNames {
		if n == "" {
			panic("core: battleWipeNames is missing a label for a BattleWipeKind — add a row to the keyed array")
		}
	}
}

// BattleWipeName is the menu label for a wipe kind.
func BattleWipeName(k BattleWipeKind) string {
	if k < 0 || int(k) >= len(battleWipeNames) {
		return "?"
	}
	return battleWipeNames[k]
}

// Screen Wipe FX submenu rows: one per kind, then Close.
func BattleWipeCloseRow() int  { return BattleWipeKindCount }
func BattleWipeMenuCount() int { return BattleWipeKindCount + 1 }
