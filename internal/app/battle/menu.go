package battle

import (
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"fmt"
	"log"
)

func updateActionMenu(g *core.GameState) {
	// Debug easy-quit: bail out of the fight entirely. Only live when the
	// debug toggle is on, so normal play has no flee shortcut here.
	if g.EasyBattleQuit && input.DebugFleePressed() {
		fleeBattle(g)
		return
	}
	g.Battle.MenuIndex = input.CursorUpDown(g.Battle.MenuIndex, core.ActionRowCount)
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
		if !core.HasConsumable(g.Inventory) {
			setBattleStatus(g, "No items.")
			return
		}
		g.Battle.ActionMode = core.ActionItemMenu
		g.Battle.ItemMenuIndex = 0
		setBattleStatus(g, "Choose an item.")
		return
	case core.ActionRowSkill:
		openSkillMenu(g)
		return
	}
}

// openSkillMenu transitions the action menu into the skill-picker
// submenu. Seeds the cursor from the member's persisted SkillCursor so
// the submenu opens on their last-used skill instead of always jumping
// back to slot 0.
func openSkillMenu(g *core.GameState) {
	if g.Battle.CurrentParty < 0 || g.Battle.CurrentParty >= len(g.Party) {
		return
	}
	idx := g.Party[g.Battle.CurrentParty].SkillCursor
	if idx < 0 || idx >= core.SkillsPerClass {
		idx = 0
	}
	g.Battle.SkillMenuIndex = idx
	g.Battle.ActionMode = core.ActionSkillMenu
	setBattleStatus(g, "Choose a skill.")
}

// updateSkillMenu drives the skill-picker submenu. Up/Down cycles the
// learned-skill list; Back returns to the action menu; Confirm arms
// the chosen skill and transitions into its target mode (or arms the
// timing bar directly when the skill is self-cast / AoE).
func updateSkillMenu(g *core.GameState) {
	if g.Battle.CurrentParty < 0 || g.Battle.CurrentParty >= len(g.Party) {
		resetBattleAction(g)
		return
	}
	skills := core.PartySkills(g.Party[g.Battle.CurrentParty])
	if len(skills) == 0 {
		resetBattleAction(g)
		setBattleStatus(g, "No skill ready.")
		return
	}
	g.Battle.SkillMenuIndex = input.CursorUpDown(g.Battle.SkillMenuIndex, len(skills))
	if input.BackPressed() {
		resetBattleAction(g)
		setBattleStatus(g, "Choose an action.")
		return
	}
	if !input.ConfirmPressed() {
		return
	}
	if g.Battle.SkillMenuIndex < 0 || g.Battle.SkillMenuIndex >= len(skills) {
		g.Battle.SkillMenuIndex = 0
	}
	skill := skills[g.Battle.SkillMenuIndex]
	if skill == core.SkillNone {
		setBattleStatus(g, "No skill ready.")
		return
	}
	// MP gate routed through the shared canAffordSkill predicate so
	// the rule matches chargeMP's deduct-time check. Two separate
	// inlines of `actor.MP < cost` previously made a "potion of
	// free cast" or "VIT-raises-MP-cap" feature a two-place edit.
	if !canAffordSkill(g.Party[g.Battle.CurrentParty], skill) {
		setBattleStatus(g, fmt.Sprintf("%s needs %d MP.", core.SkillName(skill), core.SkillCost(skill)))
		return
	}
	// Persist the choice so next turn's submenu opens on this skill.
	g.Party[g.Battle.CurrentParty].SkillCursor = g.Battle.SkillMenuIndex
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
	living := core.LiveConsumables(g.Inventory)
	count := len(living)
	if count == 0 {
		// Inventory ran dry between opening the menu and now — not actually
		// reachable today (use is the only consumer), but defensively bail.
		resetBattleAction(g)
		setBattleStatus(g, "No items.")
		return
	}
	g.Battle.ItemMenuIndex = input.CursorUpDown(g.Battle.ItemMenuIndex, count)
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

// updateTargetPicker is the shared input loop for every target-selection
// mode: Next/Previous cycle the cursor via cycleFn, Back runs onBack (and
// stops), Confirm runs onConfirm. updateEnemyTargeting /
// updatePartyTargeting / updateItemTarget used to repeat this exact block,
// differing only in which cycle / back / confirm closures they bound.
func updateTargetPicker(cycleFn func(int), onBack, onConfirm func()) {
	if input.TargetNextPressed() {
		cycleFn(1)
	}
	if input.TargetPreviousPressed() {
		cycleFn(-1)
	}
	if input.BackPressed() {
		onBack()
		return
	}
	if input.ConfirmPressed() {
		onConfirm()
	}
}

// updateItemTarget picks the ally to receive the pending item. Mirrors
// updatePartyTargeting but routes through applyItem.
func updateItemTarget(g *core.GameState) {
	updateTargetPicker(
		func(d int) { cyclePartyTarget(g, d) },
		func() {
			// Step back to the item picker, NOT all the way to the action
			// menu — target-back cancels the target selection, not the
			// whole action.
			g.Battle.ActionMode = core.ActionItemMenu
			setBattleStatus(g, "Choose an item.")
		},
		func() { applyItem(g) },
	)
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
	updateTargetPicker(
		func(d int) { cycleBattleTarget(g, d) },
		func() { cancelTargetToActionMenu(g) },
		func() { beginPendingAction(g) },
	)
}

func updatePartyTargeting(g *core.GameState) {
	updateTargetPicker(
		func(d int) { cyclePartyTarget(g, d) },
		func() { cancelTargetToActionMenu(g) },
		func() { beginPendingAction(g) },
	)
}

// cancelTargetToActionMenu is the shared Back handler for the
// enemy/party target pickers — drop the pending action and return to the
// action menu prompt.
func cancelTargetToActionMenu(g *core.GameState) {
	resetBattleAction(g)
	setBattleStatus(g, "Choose an action.")
}

// cycleTargetSelection is the shared body for the enemy / party target
// cyclers: fetch the live target indices, wrap the cursor by delta, and
// commit + announce the new pick. The two cycle helpers used to be
// near-identical apart from selector, state field, and status formatter.
func cycleTargetSelection(
	g *core.GameState,
	current *int,
	delta int,
	living func() []int,
	status func(g *core.GameState, slot, total int) string,
) {
	targets := living()
	if len(targets) == 0 {
		return
	}
	next := cycleTarget(*current, targets, delta)
	*current = targets[next]
	setBattleStatus(g, status(g, next+1, len(targets)))
}

func cycleBattleTarget(g *core.GameState, delta int) {
	cycleTargetSelection(g, &g.Battle.EnemyIndex, delta,
		func() []int { return core.LivingBattleEnemyIndices(g) },
		func(g *core.GameState, slot, total int) string {
			return core.BattleEnemyTargetStatus(g, slot, total)
		})
}

func cyclePartyTarget(g *core.GameState, delta int) {
	cycleTargetSelection(g, &g.Battle.PartyTarget, delta,
		func() []int { return core.AvailablePartyTargets(g.Party) },
		func(g *core.GameState, _, _ int) string {
			return fmt.Sprintf("Targeting %s.", g.Party[g.Battle.PartyTarget].Name)
		})
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
