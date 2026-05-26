package explore

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// updateChestModal runs the chest-open dialog. Esc closes; Up/Down picks
// a row; Confirm on an item row takes one of that item; Confirm on the
// Take-All row drains everything. Closing on an emptied chest flips the
// chest's Looted flag so the lid renders open from now on.
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

	stacks := core.LiveStacks(chest.Items)
	// Empty-chest shortcut: no rows to pick, only Take All (no-op) — just
	// mark looted and close so the player isn't stuck in an empty dialog.
	if len(stacks) == 0 {
		closeChest(g, chest)
		return
	}

	rowCount := len(stacks) + 1 // items + "Take All"
	g.ChestMenuIndex = input.CursorUpDown(g.ChestMenuIndex, rowCount)

	if !input.ConfirmPressed() {
		return
	}
	if g.ChestMenuIndex == core.ChestTakeAllRow(len(stacks)) {
		for _, st := range stacks {
			g.Inventory = core.AddItem(g.Inventory, st.Kind, st.Count)
		}
		chest.Items = nil
		audio.Play(audio.SoundInputHit)
		closeChest(g, chest)
		return
	}
	if g.ChestMenuIndex < 0 || g.ChestMenuIndex >= len(stacks) {
		return
	}
	picked := stacks[g.ChestMenuIndex]
	chest.Items, _ = core.ConsumeItem(chest.Items, picked.Kind)
	g.Inventory = core.AddItem(g.Inventory, picked.Kind, 1)
	audio.Play(audio.SoundInputHit)
	// After taking, if no items remain, close + mark looted. Otherwise
	// re-clamp the cursor so it doesn't point past the shrunken list.
	remaining := core.LiveStacks(chest.Items)
	if len(remaining) == 0 {
		closeChest(g, chest)
		return
	}
	if g.ChestMenuIndex >= len(remaining) {
		g.ChestMenuIndex = len(remaining) - 1
	}
}

// closeChest dismisses the modal and marks the chest looted if its
// stacks are now empty. Centralised so every exit path (Esc, Take All,
// last-item Take) goes through the same logic — delegates the
// looted-detection rule to core.MarkChestLootedIfEmpty so the rule
// itself isn't duplicated alongside the modal-state reset.
func closeChest(g *core.GameState, chest *core.Chest) {
	core.MarkChestLootedIfEmpty(chest)
	g.ChestOpen = -1
	g.ChestMenuIndex = 0
}
