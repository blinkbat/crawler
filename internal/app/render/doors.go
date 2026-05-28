package render

import (
	"strings"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// DrawDoors paints every authored door in the current area as a wooden
// doorframe at its tile center, rotated to face the player's
// post-transition direction. The lighting shader is bound by DrawWorld
// before this call so the door picks up the same per-area profile;
// callers must invoke DrawDoors inside the same rl.BeginMode3D pass.
//
// Doors don't have a "looted" or "open" state today — they render
// identically before and after the player steps through. A future
// "locked" state can add a closed-panel variant or recolor the brass
// keystone.
func DrawDoors(camera rl.Camera3D, g core.GameState, assets Resources) {
	for _, d := range g.Doors {
		center := tileWorldPos(d.TileX, d.TileZ, 0)
		yaw := doorYawDeg(d.Facing)
		style := d.Style
		if style < 0 || int(style) >= len(assets.doorProps) {
			style = core.DoorStyleBuilding
		}
		assets.doorProps[style].draw(center, 1.0, yaw)
	}
}

// DrawDoorPrompt paints the "enter this doorway?" confirm modal that opens
// when the player steps onto a door (g.DoorPrompt >= 0). Centered glass
// card with the destination name and Enter/Cancel hints. No-op when no
// prompt is active. Drawn as a 2D overlay after the world pass.
func DrawDoorPrompt(g core.GameState, assets Resources) {
	if g.DoorPrompt < 0 || g.DoorPrompt >= len(g.Doors) {
		return
	}
	dest := humanizeMapID(g.Doors[g.DoorPrompt].TargetMap)
	screenW, screenH := screenSize()
	panelW := int32(440)
	panelH := int32(168)
	panelX := centerX(panelW)
	panelY := screenH/2 - panelH/2

	rl.DrawRectangle(0, 0, screenW, screenH, surfaceVeil)
	drawCard(panelX, panelY, panelW, panelH, surfacePrimary, borderSoft, borderSoft)
	drawCardFiligree(panelX, panelY, panelW, panelH, giltDim)

	title := "DOORWAY"
	tm := rl.MeasureTextEx(assets.hudFont, title, FontHeading, FontSpacingHeading)
	drawTextWithShadowStyle(assets.hudFont, title,
		float32(panelX)+float32(panelW)/2-tm.X/2, float32(panelY+22),
		FontHeading, FontSpacingHeading, textPrimary, shadowStrong, 1, 1)

	cardCenterX := float32(panelX) + float32(panelW)/2
	drawTextCentered(assets.hudFont, "Enter "+dest+"?",
		cardCenterX, float32(panelY+78), FontBody, textMuted)
	drawTextCentered(assets.hudFont, "Enter — go through      Esc — stay",
		cardCenterX, float32(panelY+panelH-40), FontSmall, textHint)
}

// humanizeMapID turns a bare map id ("forgotten_plaza") into a display
// label ("Forgotten Plaza") for the door prompt. Underscores become
// spaces and each word is title-cased; an empty id falls back to "the
// next area" so the prompt always reads as a sentence.
func humanizeMapID(id string) string {
	if id == "" {
		return "the next area"
	}
	words := strings.FieldsFunc(id, func(r rune) bool { return r == '_' || r == '-' })
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// doorYawDeg maps a core.Facing direction to the degree rotation the
// doorframe's mesh needs so the opening points in that direction. The
// mesh is authored facing +Z (south), so North needs a 180° flip,
// East/West get 90°/270°.
func doorYawDeg(facing int) float32 {
	switch core.NormalizeFacing(facing) {
	case core.North:
		return 180
	case core.East:
		return 90
	case core.South:
		return 0
	case core.West:
		return 270
	}
	return 0
}
