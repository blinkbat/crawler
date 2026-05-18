package battle

import (
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"fmt"
	"log"
)

func updateActionMenu(g *core.GameState) {
	if input.UpPressed() {
		g.Battle.MenuIndex = core.WrapIndex(g.Battle.MenuIndex-1, core.ActionRowCount)
	}
	if input.DownPressed() {
		g.Battle.MenuIndex = core.WrapIndex(g.Battle.MenuIndex+1, core.ActionRowCount)
	}
	if input.BackPressed() {
		setBattleStatus(g, "Choose an action.")
		return
	}
	if !input.ConfirmPressed() {
		return
	}
	switch core.ActionRow(g.Battle.MenuIndex) {
	case core.ActionRowAttack:
		g.Battle.PendingSkill = core.SkillNone
		g.Battle.ActionMode = core.ActionEnemyTarget
		setBattleStatus(g, "Choose a target.")
		return
	case core.ActionRowDefend:
		performDefend(g)
		return
	case core.ActionRowItem:
		if core.InventoryEmpty(g.Inventory) {
			setBattleStatus(g, "No items.")
			return
		}
		g.Battle.ActionMode = core.ActionItemMenu
		g.Battle.ItemMenuIndex = 0
		setBattleStatus(g, "Choose an item.")
		return
	case core.ActionRowSkill:
		performSkill(g)
		return
	}
}

// performSkill is the body of the "Skill" row's confirm — split out so the
// action-menu switch reads as one row per case and actionRowSkill isn't a
// silent fall-through.
func performSkill(g *core.GameState) {
	skill := core.PartySkill(g.Party[g.Battle.CurrentParty])
	if skill == core.SkillNone {
		setBattleStatus(g, "No skill ready.")
		return
	}
	cost := core.SkillCost(skill)
	if g.Party[g.Battle.CurrentParty].MP < cost {
		setBattleStatus(g, fmt.Sprintf("%s needs %d MP.", core.SkillName(skill), cost))
		return
	}
	g.Battle.PendingSkill = skill
	switch core.SkillTargetMode(skill) {
	case core.ActionPartyTarget:
		g.Battle.ActionMode = core.ActionPartyTarget
		g.Battle.PartyTarget = g.Battle.CurrentParty
		setBattleStatus(g, fmt.Sprintf("Choose who receives %s.", core.SkillName(skill)))
	case core.ActionEnemyTarget:
		g.Battle.ActionMode = core.ActionEnemyTarget
		setBattleStatus(g, fmt.Sprintf("Choose a target for %s.", core.SkillName(skill)))
	default:
		beginPendingAction(g)
	}
}

// updateItemMenu drives the inventory picker. Up/Down cycles entries; Back
// returns to the action menu; Confirm picks the highlighted item and moves
// to ally-target selection. Items only heal allies for now, so target mode
// is always party.
func updateItemMenu(g *core.GameState) {
	living := core.LiveStacks(g.Inventory)
	count := len(living)
	if count == 0 {
		// Inventory ran dry between opening the menu and now — not actually
		// reachable today (use is the only consumer), but defensively bail.
		resetBattleAction(g)
		setBattleStatus(g, "No items.")
		return
	}
	if input.UpPressed() {
		g.Battle.ItemMenuIndex = core.WrapIndex(g.Battle.ItemMenuIndex-1, count)
	}
	if input.DownPressed() {
		g.Battle.ItemMenuIndex = core.WrapIndex(g.Battle.ItemMenuIndex+1, count)
	}
	if input.BackPressed() {
		resetBattleAction(g)
		setBattleStatus(g, "Choose an action.")
		return
	}
	if !input.ConfirmPressed() {
		return
	}
	if g.Battle.ItemMenuIndex < 0 || g.Battle.ItemMenuIndex >= count {
		g.Battle.ItemMenuIndex = 0
	}
	picked := living[g.Battle.ItemMenuIndex].Kind
	g.Battle.PendingItem = picked
	g.Battle.ActionMode = core.ActionItemTarget
	g.Battle.PartyTarget = g.Battle.CurrentParty
	setBattleStatus(g, fmt.Sprintf("Use %s on whom?", core.ItemInfo(picked).Name))
}

// updateItemTarget picks the ally to receive the pending item. Mirrors
// updatePartyTargeting but routes through applyItem.
func updateItemTarget(g *core.GameState) {
	if input.TargetNextPressed() {
		cyclePartyTarget(g, 1)
	}
	if input.TargetPreviousPressed() {
		cyclePartyTarget(g, -1)
	}
	if input.BackPressed() {
		// Step back to the item picker, NOT all the way to the action menu —
		// matches the pattern where target-back cancels the target selection
		// rather than the whole action.
		g.Battle.ActionMode = core.ActionItemMenu
		setBattleStatus(g, "Choose an item.")
		return
	}
	if !input.ConfirmPressed() {
		return
	}
	applyItem(g)
}

// applyItem consumes the pending item, heals the targeted ally by the
// item's HealAmount, and ends the actor's turn. The item action doesn't
// run a timing minigame — the player already invested a turn and used a
// finite resource, no need to extract a third demand on top.
func applyItem(g *core.GameState) {
	kind := g.Battle.PendingItem
	target := g.Battle.PartyTarget
	if kind == core.ItemNone {
		resetBattleAction(g)
		return
	}
	if target < 0 || target >= len(g.Party) || g.Party[target].HP <= 0 {
		setBattleStatus(g, "Invalid target.")
		return
	}
	// Try to consume from inventory first — bail without using the action's
	// turn if the stack disappeared (defensive; shouldn't happen).
	updated, ok := core.ConsumeItem(g.Inventory, kind)
	if !ok {
		setBattleStatus(g, "Item not in inventory.")
		resetBattleAction(g)
		return
	}
	g.Inventory = updated
	def := core.ItemInfo(kind)
	healed := healPartyMember(g, target, def.HealAmount)
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	if healed && def.HealAmount > 0 {
		setBattleMessage(g, fmt.Sprintf("%s eats %s (+%d HP).", g.Party[target].Name, def.Name, def.HealAmount))
	} else {
		setBattleMessage(g, fmt.Sprintf("%s uses %s.", actor.Name, def.Name))
	}
	g.Battle.PendingItem = core.ItemNone
	finishPartyAction(g)
}

// performDefend marks the current member as defending and ends their turn.
// No timing minigame — the boost is the whole reward, and showing a bar would
// imply an opportunity for a better outcome that doesn't exist.
func performDefend(g *core.GameState) {
	if g.Battle.CurrentParty < 0 || g.Battle.CurrentParty >= len(g.Party) {
		return
	}
	member := &g.Party[g.Battle.CurrentParty]
	member.Defending = true
	setBattleMessage(g, fmt.Sprintf("%s braces for impact.", member.Name))
	finishPartyAction(g)
}

func updateEnemyTargeting(g *core.GameState) {
	if input.TargetNextPressed() {
		cycleBattleTarget(g, 1)
	}
	if input.TargetPreviousPressed() {
		cycleBattleTarget(g, -1)
	}
	if input.BackPressed() {
		resetBattleAction(g)
		setBattleStatus(g, "Choose an action.")
		return
	}
	if !input.ConfirmPressed() {
		return
	}
	beginPendingAction(g)
}

func updatePartyTargeting(g *core.GameState) {
	if input.TargetNextPressed() {
		cyclePartyTarget(g, 1)
	}
	if input.TargetPreviousPressed() {
		cyclePartyTarget(g, -1)
	}
	if input.BackPressed() {
		resetBattleAction(g)
		setBattleStatus(g, "Choose an action.")
		return
	}
	if !input.ConfirmPressed() {
		return
	}
	beginPendingAction(g)
}

func cycleBattleTarget(g *core.GameState, delta int) {
	living := core.LivingBattleEnemyIndices(g)
	if len(living) == 0 {
		return
	}
	next := cycleTarget(g.Battle.EnemyIndex, living, delta)
	g.Battle.EnemyIndex = living[next]
	setBattleStatus(g, core.BattleEnemyTargetStatus(*g, next+1, len(living)))
}

func cyclePartyTarget(g *core.GameState, delta int) {
	living := core.LivingPartyTargets(g.Party)
	if len(living) == 0 {
		return
	}
	next := cycleTarget(g.Battle.PartyTarget, living, delta)
	g.Battle.PartyTarget = living[next]
	setBattleStatus(g, fmt.Sprintf("Targeting %s.", g.Party[g.Battle.PartyTarget].Name))
}

// cycleTarget returns the targets-slot the cursor should move to when the
// player presses left/right on a target picker. `current` is expected to be
// one of the values in `targets` — callers (cycleBattleTarget,
// cyclePartyTarget) maintain that invariant by sourcing both the current
// selection AND the living-target list from the same selectors.
//
// If `current` isn't found in `targets`, the wrap falls back to "slot 0 +
// delta" so the game keeps running, but the invariant break is logged so
// the regression surfaces to whoever is debugging. Previously this was a
// silent fallback — if a future steal removes the targeted enemy mid-list
// or a heal target dies between frames, the cursor would jump to the
// front with no diagnostic trail.
func cycleTarget(current int, targets []int, delta int) int {
	currentSlot := 0
	found := false
	for i, target := range targets {
		if target == current {
			currentSlot = i
			found = true
			break
		}
	}
	if !found {
		log.Printf("battle.cycleTarget: current=%d not in targets=%v (selection invariant broken)", current, targets)
	}
	return core.WrapIndex(currentSlot+delta, len(targets))
}
