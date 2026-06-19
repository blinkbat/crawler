package battle

import (
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"fmt"
	"log"
	"slices"
)

// skillMenuBuf / itemMenuBuf are reused scratch slices for the skill and
// item submenus, refilled each frame the menu is open (via the *Into
// buffer-reusing helpers) instead of allocating a fresh list — and a map,
// in the skill case — every frame. Mirror render's itemMenuStacksBuf. Only
// valid for the duration of the frame they're filled; never aliased.
var (
	skillMenuBuf []core.SkillID
	itemMenuBuf  []core.ItemStack
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
		setBattleStatus(g, msgChooseAction)
		return
	}
	if !input.ConfirmPressed() {
		return
	}
	switch core.ActionRow(g.Battle.MenuIndex) {
	case core.ActionRowAttack:
		g.Battle.PendingSkill = core.SkillNone
		g.Battle.ActionMode = core.ActionEnemyTarget
		setBattleStatus(g, msgChooseTarget)
		return
	case core.ActionRowDefend:
		performDefend(g)
		return
	case core.ActionRowFlee:
		// Gate the retreat behind a yes/no — a stray Confirm on this row
		// shouldn't burn the turn fleeing by accident.
		g.Battle.ActionMode = core.ActionFleeConfirm
		setBattleStatus(g, "Flee this battle?")
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
	default:
		// A new ActionRow added to the enum without a case here would
		// otherwise make Confirm on that row a silent no-op. Surface it
		// instead of swallowing the press.
		setBattleStatus(g, msgChooseAction)
	}
}

// updateFleeConfirm runs the Flee yes/no gate (ActionFleeConfirm): Confirm
// commits the retreat, Back returns to the action menu. Keeps an accidental
// Confirm on the Flee row from spending the turn fleeing.
func updateFleeConfirm(g *core.GameState) {
	if input.BackPressed() {
		g.Battle.ActionMode = core.ActionMenu
		setBattleStatus(g, msgChooseAction)
		return
	}
	if input.ConfirmPressed() {
		performFlee(g)
	}
}

// currentMember returns the acting party member (the CurrentParty slot) and
// whether the slot index is valid. The "is CurrentParty in range?" guard was
// open-coded at every action entry point; this is the single accessor so the
// bounds rule lives in one place. Callers handle the !ok case their own way
// (some resetBattleAction, some just return).
func currentMember(g *core.GameState) (*core.PartyMember, bool) {
	if !partyIndexValid(g, g.Battle.CurrentParty) {
		return nil, false
	}
	return &g.Party[g.Battle.CurrentParty], true
}

// openSkillMenu transitions the action menu into the skill-picker
// submenu. Seeds the cursor from the member's persisted SkillCursor so
// the submenu opens on their last-used skill instead of always jumping
// back to slot 0.
// refreshSkillMenuBuf repopulates the shared skill-menu buffer for `member`:
// the learned-skill list normally, or EVERY player-castable skill when the
// debug "all skills" toggle (g.DebugAllSkills) is on, so any skill can be
// tested without learning it. Both the open and the per-frame update path call
// this so the two can't diverge on which list they show.
func refreshSkillMenuBuf(g *core.GameState, member *core.PartyMember) {
	if g.DebugAllSkills {
		skillMenuBuf = core.PlayerCastableSkillsInto(skillMenuBuf)
		return
	}
	skillMenuBuf = core.LearnedSkillsInto(member, skillMenuBuf)
}

func openSkillMenu(g *core.GameState) {
	member, ok := currentMember(g)
	if !ok {
		return
	}
	idx := member.SkillCursor
	refreshSkillMenuBuf(g, member)
	if idx < 0 || idx >= len(skillMenuBuf) {
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
	member, ok := currentMember(g)
	if !ok {
		resetBattleAction(g)
		return
	}
	refreshSkillMenuBuf(g, member)
	skills := skillMenuBuf
	if len(skills) == 0 {
		resetBattleAction(g)
		setBattleStatus(g, msgNoSkillReady)
		return
	}
	g.Battle.SkillMenuIndex = input.CursorUpDown(g.Battle.SkillMenuIndex, len(skills))
	if input.BackPressed() {
		resetBattleAction(g)
		setBattleStatus(g, msgChooseAction)
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
		setBattleStatus(g, msgNoSkillReady)
		return
	}
	// MP gate routed through the shared canAffordSkill predicate so
	// the rule matches chargeMP's deduct-time check. Two separate
	// inlines of `actor.MP < cost` previously made a "potion of
	// free cast" or "VIT-raises-MP-cap" feature a two-place edit.
	if !g.DebugAllSkills && !canAffordSkill(g.Party[g.Battle.CurrentParty], skill) {
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
	itemMenuBuf = core.LiveConsumablesInto(g.Inventory, itemMenuBuf)
	living := itemMenuBuf
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
		setBattleStatus(g, msgChooseAction)
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
// mode: Next/Previous cycle the cursor via cycle, Back runs onBack (and
// stops), Confirm runs onConfirm. updateEnemyTargeting /
// updatePartyTargeting / updateItemTarget used to repeat this exact block,
// differing only in which cycle / back / confirm functions they bound.
//
// The hooks take *core.GameState directly (rather than pre-bound zero-arg
// closures) so the three callers can pass top-level function values — which are
// static and DON'T allocate — instead of fresh closures capturing g every
// frame the picker is up. This runs per frame while the player hovers a target,
// so the closures were pure per-frame garbage.
func updateTargetPicker(g *core.GameState, cycle func(*core.GameState, int), onBack, onConfirm func(*core.GameState)) {
	if input.TargetNextPressed() {
		cycle(g, 1)
	}
	if input.TargetPreviousPressed() {
		cycle(g, -1)
	}
	if input.BackPressed() {
		onBack(g)
		return
	}
	if input.ConfirmPressed() {
		onConfirm(g)
	}
}

// updateItemTarget picks the ally to receive the pending item. Mirrors
// updatePartyTargeting but routes through applyItem.
func updateItemTarget(g *core.GameState) {
	updateTargetPicker(g, cyclePartyTarget, cancelTargetToItemMenu, applyItem)
}

// cancelTargetToItemMenu steps back to the item picker, NOT all the way to the
// action menu — target-back cancels the target selection, not the whole action.
func cancelTargetToItemMenu(g *core.GameState) {
	g.Battle.ActionMode = core.ActionItemMenu
	setBattleStatus(g, "Choose an item.")
}

// applyItem consumes the pending item, heals the targeted ally by the
// item's HealAmount, and ends the actor's turn. The item action doesn't
// run a timing minigame — the player already invested a turn and used a
// finite resource, no need to extract a third demand on top.
func applyItem(g *core.GameState) {
	kind := g.Battle.PendingItem
	if kind == core.ItemNone {
		resetBattleAction(g)
		return
	}
	if _, ok := currentMember(g); !ok {
		// Defensive: the live caller (updatePlayerBattle) guards this, but
		// don't index g.Party with a stale CurrentParty if a future path
		// reaches an item-target confirm mid-transition. Mirrors performDefend.
		resetBattleAction(g)
		return
	}
	target := g.Battle.PartyTarget
	// Ingested is checked alongside HP<=0: the target picker excludes ingested
	// allies, but a mantrap can swallow the chosen ally between target-select
	// and this confirm (mixed-initiative). Without this guard the stack is
	// consumed and healPartyMember no-ops on the ingested member — item wasted.
	if !partyIndexValid(g, target) || g.Party[target].HP <= 0 || g.Party[target].Ingested {
		setBattleStatus(g, "Invalid target.")
		return
	}
	def := core.ItemInfo(kind)
	// Don't spend a restorative on a target it can't help — the shared rule
	// (refuse only when full on every axis the item restores) lives in
	// core.ItemHelpsTarget, used by the out-of-battle path too.
	if !core.ItemHelpsTarget(def, g.Party[target]) {
		setBattleStatus(g, g.Party[target].Name+" doesn't need that right now.")
		return
	}
	// A confused actor may fumble the item onto a random living ally — the same
	// per-action retarget a confused heal cast gets (AGENTS: "Confused =
	// per-action random retarget"). Deferred until AFTER the validity/help gates
	// above so it fires exactly once on the path that commits the turn: running
	// it first let a re-confirm (after an "invalid"/"doesn't need that" bounce,
	// neither of which ends the turn) re-roll the fumble on every press and spam
	// the log. Mirrors beginPendingAction, which retargets only after setup
	// succeeds. A no-op when the actor isn't confused.
	maybeConfuseRetarget(g)
	target = g.Battle.PartyTarget
	tgt := &g.Party[target]
	// Try to consume from inventory first — bail without using the action's
	// turn if the stack disappeared (defensive; shouldn't happen).
	updated, ok := core.ConsumeItem(g.Inventory, kind)
	if !ok {
		setBattleStatus(g, "Item not in inventory.")
		resetBattleAction(g)
		return
	}
	g.Inventory = updated
	healedHP := 0
	if def.HealAmount > 0 {
		before := tgt.HP
		if healPartyMember(g, target, def.HealAmount) {
			healedHP = tgt.HP - before
		}
	}
	restoredMP := 0
	if def.MPAmount > 0 {
		restoredMP = core.RestoreMP(tgt, def.MPAmount)
	}
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	setBattleMessage(g, itemUseMessage(actor.Name, tgt.Name, def, healedHP > 0, healedHP, restoredMP))
	g.Battle.PendingItem = core.ItemNone
	finishActorTurn(g)
}

// itemUseMessage formats the combat-log line for a consumed item by what it
// actually restored: HP (food — "eats"), MP (phial — "drinks"), both, or a
// plain "uses" for a no-restore item. Both hp and mp are the ACTUAL amounts
// gained (post-clamp), so the log can't claim "+8 HP" when only 1 landed.
func itemUseMessage(actorName, targetName string, def core.ItemDefinition, healed bool, hp, mp int) string {
	switch {
	case healed && hp > 0 && mp > 0:
		return fmt.Sprintf("%s uses %s (+%d HP, +%d MP).", targetName, def.Name, hp, mp)
	case healed && hp > 0:
		return fmt.Sprintf("%s eats %s (+%d HP).", targetName, def.Name, hp)
	case mp > 0:
		return fmt.Sprintf("%s drinks %s (+%d MP).", targetName, def.Name, mp)
	default:
		return fmt.Sprintf("%s uses %s.", actorName, def.Name)
	}
}

// performDefend marks the current member as defending and ends their turn.
// No timing minigame — the boost is the whole reward, and showing a bar would
// imply an opportunity for a better outcome that doesn't exist.
func performDefend(g *core.GameState) {
	member, ok := currentMember(g)
	if !ok {
		return
	}
	member.Defending = true
	setBattleMessage(g, fmt.Sprintf("%s braces for impact.", member.Name))
	finishActorTurn(g)
}

// performFlee attempts to escape the fight. The chance scales the party's
// average living level against the pack's (core.FleeChance). On success the
// battle ends and the party retreats to the pre-combat tile (Battle.FleeReturn)
// — the pack STAYS on the field (you fled, you didn't kill it; clearBattleResidual
// keeps an alive pack). On failure the attempt burns the actor's turn, so the
// enemies get their swing.
func performFlee(g *core.GameState) {
	member, ok := currentMember(g)
	if !ok {
		return
	}
	pack := core.ActivePack(g)
	if pack == nil {
		return
	}
	chance := core.FleeChance(core.PartyAverageLevel(g.Party), core.PackAverageLevel(*pack))
	if !core.RollChance(g.Rand(), chance) {
		setBattleMessage(g, fmt.Sprintf("%s tries to flee — and can't break away!", member.Name))
		finishActorTurn(g)
		return
	}
	// Escaped: snap the party back to the pre-combat tile (clearing any step
	// animation) and leave combat. The pack survives, so the player lands where
	// they started and is free to walk off. The retreat tile is always walkable
	// (the player legitimately stood on it and terrain can't change mid-battle),
	// so the only dynamic hazard is a pack having moved onto it — in a
	// pack-into-player ambush, the engaging tick can (rarely) step ANOTHER pack
	// onto the just-vacated tile. Guard against that: only reposition when no
	// pack occupies the tile, otherwise escape combat in place rather than
	// teleporting on top of a pack.
	if core.PackIndexAtTile(g.Packs, g.Battle.FleeReturnX, g.Battle.FleeReturnZ) < 0 {
		g.Player.TileX = g.Battle.FleeReturnX
		g.Player.TileZ = g.Battle.FleeReturnZ
		// Re-seat the standing level on the return tile: keep the level carried
		// out of the pre-combat step when it's still a standable surface (so a
		// player who fought on a bridge deck flees back onto the deck, not the
		// ground beneath it), and only snap to the lowest standable surface when
		// it isn't. Same rule the save loader uses. No-op on a heightfield, where
		// the single surface is both the carried level and the lowest standable.
		if !g.Area.Standable(g.Player.TileX, g.Player.Level, g.Player.TileZ) {
			if lo := g.Area.LowestStandableLevel(g.Player.TileX, g.Player.TileZ); lo >= 0 {
				g.Player.Level = lo
			}
		}
		core.SnapPlayerToTile(&g.Player)
		g.Player.Anim = core.Animation{}
	}
	leaveBattle(g, fmt.Sprintf("%s leads the party in a hasty retreat!", member.Name))
}

func updateEnemyTargeting(g *core.GameState) {
	updateTargetPicker(g, cycleBattleTarget, cancelTargetToActionMenu, beginPendingAction)
}

func updatePartyTargeting(g *core.GameState) {
	updateTargetPicker(g, cyclePartyTarget, cancelTargetToActionMenu, beginPendingAction)
}

// cancelTargetToActionMenu is the shared Back handler for the
// enemy/party target pickers — drop the pending action and return to the
// action menu prompt.
func cancelTargetToActionMenu(g *core.GameState) {
	resetBattleAction(g)
	setBattleStatus(g, msgChooseAction)
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
	currentSlot := slices.Index(targets, current)
	if currentSlot < 0 {
		log.Printf("battle.cycleTarget: current=%d not in targets=%v (selection invariant broken)", current, targets)
		currentSlot = 0
	}
	return core.WrapIndex(currentSlot+delta, len(targets))
}
