package explore

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// openSkillTreeFor raises the Skills-tab skill-tree modal for a party
// member — the Diablo-2-style sub-dialog showing that member's three
// trees. Resets the modal cursor to the first node of the first tree and
// dismisses any open use-target picker so two sub-modals can't co-exist.
func openSkillTreeFor(g *core.GameState, member int) {
	if member < 0 || member >= len(g.Party) {
		return
	}
	g.SkillTreeOpen = true
	g.SkillTreeMember = member
	g.SkillTreeCol = 0
	g.SkillTreeRow = 0
	closeUseTarget(g)
}

// closeSkillTree takes the skill-tree modal down without touching the
// rest of the panels overlay. Safe to call when it's already closed.
func closeSkillTree(g *core.GameState) {
	g.SkillTreeOpen = false
}

// updateSkillTreeModal owns one frame of panel input while the skill-tree
// modal is up. Back closes just the modal; Left/Right page the three tree
// columns (clamping the node cursor into the new column); Up/Down walk the
// nodes; Confirm invests a SkillPoint into the focused node.
func updateSkillTreeModal(g *core.GameState) {
	if input.BackPressed() {
		closeSkillTree(g)
		return
	}
	if g.SkillTreeMember < 0 || g.SkillTreeMember >= len(g.Party) {
		closeSkillTree(g)
		return
	}
	trees := core.SkillTreesFor(g.Party[g.SkillTreeMember].Class)
	if len(trees) == 0 {
		closeSkillTree(g)
		return
	}
	// Shared Left/Right column wrap (also re-clamps a stale cursor into range).
	g.SkillTreeCol = input.CursorLeftRightWrap(g.SkillTreeCol, len(trees))
	nodes := trees[g.SkillTreeCol].Nodes
	// Clamp the row into the (possibly shorter) column after a sideways move.
	// Floor at 0 so an empty column (guarded against at init, but cheap to
	// defend here too) can't leave the cursor at -1.
	if g.SkillTreeRow >= len(nodes) {
		g.SkillTreeRow = len(nodes) - 1
	}
	if g.SkillTreeRow < 0 {
		g.SkillTreeRow = 0
	}
	g.SkillTreeRow = input.CursorUpDown(g.SkillTreeRow, len(nodes))
	if input.ConfirmPressed() {
		buySkillNode(g)
	}
}

// buySkillNode invests one SkillPoint into the focused node, pinging the
// gilt "great" cue on a successful purchase and the miss cue on a refusal
// (locked node, maxed, or not enough points) so the player gets feedback
// either way.
func buySkillNode(g *core.GameState) {
	m := &g.Party[g.SkillTreeMember]
	trees := core.SkillTreesFor(m.Class)
	if g.SkillTreeCol < 0 || g.SkillTreeCol >= len(trees) {
		return
	}
	nodes := trees[g.SkillTreeCol].Nodes
	if g.SkillTreeRow < 0 || g.SkillTreeRow >= len(nodes) {
		return
	}
	if core.BuySkillNode(m, nodes[g.SkillTreeRow].ID) {
		audio.Play(audio.SoundInputGreat)
	} else {
		audio.Play(audio.SoundInputMiss)
	}
}
