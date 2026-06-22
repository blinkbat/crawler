package explore

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// openSkillTreeFor raises the skill-tree modal for a member, resetting the cursor
// and dismissing any use-target picker so two sub-modals can't co-exist.
func openSkillTreeFor(g *core.GameState, member int) {
	if _, ok := validMember(g, member); !ok {
		return
	}
	g.SkillTreeOpen = true
	g.SkillTreeMember = member
	g.SkillTreeCol = 0
	g.SkillTreeRow = 0
	closeUseTarget(g)
}

// closeSkillTree takes the modal down without touching the rest of the overlay.
func closeSkillTree(g *core.GameState) {
	g.SkillTreeOpen = false
}

// updateSkillTreeModal owns one frame of panel input while the modal is up: Back
// closes it, Left/Right page tree columns, Up/Down walk nodes, Confirm invests.
func updateSkillTreeModal(g *core.GameState) {
	if input.BackPressed() {
		closeSkillTree(g)
		return
	}
	m, ok := validMember(g, g.SkillTreeMember)
	if !ok {
		closeSkillTree(g)
		return
	}
	trees := core.SkillTreesFor(m.Class)
	if len(trees) == 0 {
		closeSkillTree(g)
		return
	}
	g.SkillTreeCol = input.CursorLeftRightWrap(g.SkillTreeCol, len(trees))
	nodes := trees[g.SkillTreeCol].Nodes
	// Clamp the row into the (shorter) column; guard empty so the max can't go negative.
	if len(nodes) > 0 {
		g.SkillTreeRow = core.Clamp(g.SkillTreeRow, 0, len(nodes)-1)
	} else {
		g.SkillTreeRow = 0
	}
	g.SkillTreeRow = input.CursorUpDown(g.SkillTreeRow, len(nodes))
	if input.ConfirmPressed() {
		buySkillNode(g)
	}
}

// buySkillNode invests one SkillPoint into the focused node (miss ping on a
// refusal: locked, maxed, or not enough points).
func buySkillNode(g *core.GameState) {
	m, ok := validMember(g, g.SkillTreeMember)
	if !ok {
		return
	}
	trees := core.SkillTreesFor(m.Class)
	if g.SkillTreeCol < 0 || g.SkillTreeCol >= len(trees) {
		return
	}
	nodes := trees[g.SkillTreeCol].Nodes
	if g.SkillTreeRow < 0 || g.SkillTreeRow >= len(nodes) {
		return
	}
	audio.PlayResult(core.BuySkillNode(m, nodes[g.SkillTreeRow].ID))
}
