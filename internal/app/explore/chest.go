package explore

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// updateChestModal runs the chest-open dialog: Esc closes, Up/Down picks a row,
// Confirm takes one item (or Take-All drains everything). Closing an emptied
// chest flips its Looted flag so the lid renders open.
func updateChestModal(g *core.GameState) {
	if g.ChestOpen < 0 || g.ChestOpen >= len(g.Chests) {
		g.ChestOpen = -1
		return
	}
	chest := &g.Chests[g.ChestOpen]

	if input.BackPressed() {
		closeChest(g, chest)
		return
	}

	// No-alloc count; the slice is materialized only when a Confirm lands (below).
	stackCount := core.LiveStackCount(chest.Items)
	// Empty chest: mark looted and close so the player isn't stuck.
	if stackCount == 0 {
		closeChest(g, chest)
		return
	}

	rowCount := stackCount + 1 // items + "Take All"
	g.ChestMenuIndex = input.CursorUpDown(g.ChestMenuIndex, rowCount)

	if !input.ConfirmPressed() {
		return
	}
	stacks := core.LiveStacks(chest.Items)
	if g.ChestMenuIndex == core.ChestTakeAllRow(len(stacks)) {
		for _, st := range stacks {
			g.Inventory = core.AddItem(g.Inventory, st.Kind, st.Count)
		}
		chest.Items = nil
		audio.Play(audio.SoundInputHit)
		closeChest(g, chest)
		return
	}
	picked, ok := stackAtCursor(stacks, g.ChestMenuIndex)
	if !ok {
		return
	}
	updated, ok := core.ConsumeItem(chest.Items, picked.Kind)
	if !ok {
		// Defensive: `picked` has positive count, so this should succeed; bail
		// without handing out a free item if it ever diverges.
		return
	}
	chest.Items = updated
	g.Inventory = core.AddItem(g.Inventory, picked.Kind, 1)
	audio.Play(audio.SoundInputHit)
	// Empty now → close + mark looted; else re-clamp the cursor.
	remaining := core.LiveStacks(chest.Items)
	if len(remaining) == 0 {
		closeChest(g, chest)
		return
	}
	// Took from an item row: clamp so emptying the bottom stack pulls the cursor
	// back to the new last item instead of onto Take-All.
	g.ChestMenuIndex = clampCursorToLen(g.ChestMenuIndex, len(remaining))
}

// closeChest dismisses the modal and marks the chest looted if empty (looted
// rule delegated to core.MarkChestLootedIfEmpty).
func closeChest(g *core.GameState, chest *core.Chest) {
	core.MarkChestLootedIfEmpty(chest)
	g.ChestOpen = -1
	g.ChestMenuIndex = 0
}
