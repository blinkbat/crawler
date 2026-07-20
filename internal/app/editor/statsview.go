package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// statsview.go — the read-only Map Stats report (modalStats). Quantitative feedback
// the metadata panel and Validate report don't give: tile mix, content counts, and a
// rough encounter budget for balancing.

const (
	statsModalW = modalCardNarrowW
	statsRowH   = float32(20)
)

// mapStatsLines builds the report body (label : value pairs) for the current area.
func mapStatsLines(s *State) []string {
	a := &s.area
	var walls, walkable, water, ramps int
	for z := 0; z < a.Height; z++ {
		for x := 0; x < a.Width; x++ {
			if a.WallAt(x, z) {
				walls++
			}
			if !a.BlockedAt(x, z) {
				walkable++
			}
			if fc, ok := cellAt(a.Floor, x, z); ok {
				if fc == core.FloorWater || fc == core.FloorDeepWater {
					water++
				}
				if core.IsRampChar(fc) {
					ramps++
				}
			}
		}
	}
	// A materialized per-floor stack is authoritative and the legacy grid is frozen
	// but still populated (EnsurePropStack copies from it without clearing), so summing
	// both double-counts every ground-floor item. Count the stack when present, else the grid.
	props := countCharCells(a.Props, core.IsPropChar)
	if len(a.PropStack) > 0 {
		props = countStackCells(a.PropStack, core.IsPropChar)
	}
	decor := countCharCells(a.Decor, isAuthoredDecor)
	if len(a.DecorStack) > 0 {
		decor = countStackCells(a.DecorStack, isAuthoredDecor)
	}

	enemies, tierSum, goldMin, goldMax := 0, 0, 0, 0
	for _, p := range a.PackSpawns {
		for i := range p.Members {
			def := core.PackMemberDefinition(p, i)
			enemies++
			tierSum += def.Tier
			goldMin += def.GoldMin
			goldMax += def.GoldMax
		}
	}

	levelSpan := "flat"
	if lo, hi, found := areaLevelSpan(a); found && hi != lo {
		levelSpan = fmt.Sprintf("%s … %s", signedLevelLabel(clampLevel(lo)), signedLevelLabel(clampLevel(hi)))
	}

	reach := "OK"
	if n := len(s.ReachabilityWarnings()); n > 0 {
		reach = fmt.Sprintf("%d issue(s) — see Validate", n)
	}

	return []string{
		fmt.Sprintf("Dimensions:  %d × %d  (%d tiles)", a.Width, a.Height, a.Width*a.Height),
		fmt.Sprintf("Elevation levels:  %s", levelSpan),
		"",
		fmt.Sprintf("Walkable tiles:  %d", walkable),
		fmt.Sprintf("Wall tiles:  %d", walls),
		fmt.Sprintf("Water tiles:  %d", water),
		fmt.Sprintf("Ramps:  %d", ramps),
		fmt.Sprintf("Prop tiles:  %d", props),
		fmt.Sprintf("Decor tiles:  %d", decor),
		"",
		fmt.Sprintf("Packs:  %d   ·   Enemies:  %d", len(a.PackSpawns), enemies),
		fmt.Sprintf("Encounter budget:  Σtier %d   ·   gold %d–%d", tierSum, goldMin, goldMax),
		fmt.Sprintf("Chests:  %d   ·   Doors:  %d   ·   Crystals:  %d", len(a.ChestSpawns), len(a.DoorSpawns), len(a.CrystalSpawns)),
		fmt.Sprintf("Locations:  %d   ·   Dialogs:  %d   ·   Triggers:  %d", len(a.Locations), len(a.Dialogs), len(a.Triggers)),
		"",
		fmt.Sprintf("Reachability:  %s", reach),
	}
}

// countCharCells counts grid cells whose char satisfies pred.
func countCharCells(rows []string, pred func(byte) bool) int {
	n := 0
	for _, row := range rows {
		for i := 0; i < len(row); i++ {
			if pred(row[i]) {
				n++
			}
		}
	}
	return n
}

// countStackCells sums countCharCells across a per-floor scatter stack's planes.
func countStackCells(stack [][]string, pred func(byte) bool) int {
	n := 0
	for _, plane := range stack {
		n += countCharCells(plane, pred)
	}
	return n
}

// isAuthoredDecor reports whether a decor char is a real placement (not auto/empty).
func isAuthoredDecor(c byte) bool { return c != core.DecorAuto && c != core.DecorEmpty }

func openStatsModal(s *State) { s.modal = modalStats }

// updateStatsModal: read-only viewer — any dismiss closes.
func updateStatsModal(s *State) Action {
	if anyDismissPressed() {
		closeModal(s)
	}
	return ActionNone
}

func drawStatsModal(s *State, font rl.Font, theme render.Theme) {
	lines := mapStatsLines(s)
	ph := 64 + float32(len(lines))*statsRowH + 24
	_, sh := render.ScreenSizeF()
	if ph > sh-40 {
		ph = sh - 40
	}
	r := drawModalHeader(font, theme, statsModalW, ph, "MAP STATS", theme.BorderActive)
	y := r.Y + 48
	for _, line := range lines {
		render.DrawRichText(font, line, rl.NewVector2(r.X+modalContentInset, y), editorFontBody, 1, theme.TextPrimary)
		y += statsRowH
	}
	drawModalFooterHint(font, r, "Esc / Enter / click   close", theme)
}
