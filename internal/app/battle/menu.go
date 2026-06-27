package battle

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"fmt"
	"log"
	"slices"
)

func updateActionMenu(g *core.GameState) {
	// Debug easy-quit, gated on the toggle so normal play has no flee shortcut.
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
		enterEnemyTargeting(g, msgChooseTarget) // gates back-row melee; snaps cursor to a reachable foe
		return
	case core.ActionRowDefend:
		performDefend(g)
		return
	case core.ActionRowSwap:
		enterSwapTargeting(g)
		return
	case core.ActionRowFlee:
		// Yes/no gate so a stray Confirm doesn't burn the turn fleeing.
		g.Battle.ActionMode = core.ActionFleeConfirm
		setBattleStatus(g, "Flee this battle?")
		return
	case core.ActionRowItem:
		if !core.HasConsumable(g.Inventory) {
			setBattleStatus(g, msgNoItems)
			return
		}
		g.Battle.ActionMode = core.ActionItemMenu
		g.Battle.ItemMenuIndex = 0
		refreshItemMenuBuf(g)
		setBattleStatus(g, msgChooseItem)
		return
	case core.ActionRowSkill:
		openSkillMenu(g)
		return
	default:
		// Surface an unhandled ActionRow rather than swallow the press as a no-op.
		setBattleStatus(g, msgChooseAction)
	}
}

// updateFleeConfirm runs the Flee yes/no gate: Confirm commits, Back returns.
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

// currentMember returns the acting party member (CurrentParty slot) and whether
// the index is in range. Callers handle !ok their own way.
func currentMember(g *core.GameState) (*core.PartyMember, bool) {
	if !partyIndexValid(g, g.Battle.CurrentParty) {
		return nil, false
	}
	return &g.Party[g.Battle.CurrentParty], true
}

// refreshSkillMenuBuf repopulates the skill-menu buffer for `member`: learned
// skills normally, or every player-castable skill under DebugAllSkills. Both the
// open and per-frame update paths call it so they can't diverge on the list.
func refreshSkillMenuBuf(g *core.GameState, member *core.PartyMember) {
	if g.DebugAllSkills {
		g.Battle.SkillMenuList = core.PlayerCastableSkillsInto(g.Battle.SkillMenuList)
		return
	}
	g.Battle.SkillMenuList = core.LearnedSkillsInto(member, g.Battle.SkillMenuList)
}

// refreshItemMenuBuf repopulates the item-menu list with live consumable stacks.
// Called on the open frame (renderer needs the list before updateItemMenu runs)
// and each frame it stays open.
func refreshItemMenuBuf(g *core.GameState) {
	g.Battle.ItemMenuList = core.LiveConsumablesInto(g.Inventory, g.Battle.ItemMenuList)
}

func openSkillMenu(g *core.GameState) {
	member, ok := currentMember(g)
	if !ok {
		return
	}
	idx := member.SkillCursor
	refreshSkillMenuBuf(g, member)
	if idx < 0 || idx >= len(g.Battle.SkillMenuList) {
		idx = 0
	}
	g.Battle.SkillMenuIndex = idx
	g.Battle.ActionMode = core.ActionSkillMenu
	setBattleStatus(g, msgChooseSkill)
}

// updateSkillMenu drives the skill-picker: Confirm arms the skill and enters its
// target mode (or arms the timing bar directly for self-cast / AoE).
func updateSkillMenu(g *core.GameState) {
	member, ok := currentMember(g)
	if !ok {
		resetBattleAction(g)
		return
	}
	refreshSkillMenuBuf(g, member)
	skills := g.Battle.SkillMenuList
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
	// SkillMenuIndex is already in range: CursorUpDown re-clamps to [0,len-1] every
	// frame with this same count (len>0 ensured by the empty-list bail above).
	skill := skills[g.Battle.SkillMenuIndex]
	if skill == core.SkillNone {
		setBattleStatus(g, msgNoSkillReady)
		return
	}
	// MP gate via canAffordSkill so it matches chargeMP's deduct-time check; shares
	// mpRefusalMessage so the pre-gate and chargeMP can't word the refusal differently.
	if !g.DebugAllSkills && !canAffordSkill(member, skill) {
		setBattleStatus(g, mpRefusalMessage(skill))
		return
	}
	// Melee (single-target or AoE cleave) reaches nothing — gate before any target
	// selection / AoE dispatch, same buzz+log refusal as the greyed Attack row. Submenu
	// stays open to re-pick.
	if core.SkillAttackClassFor(skill).IsMelee() && refuseMeleeAction(g) {
		return
	}
	// Persist the choice so next turn's submenu opens on this skill.
	member.SkillCursor = g.Battle.SkillMenuIndex
	g.Battle.PendingSkill = skill
	switch core.SkillTargetMode(skill) {
	case core.ActionPartyTarget:
		g.Battle.ActionMode = core.ActionPartyTarget
		g.Battle.PartyTarget = g.Battle.CurrentParty
		setBattleStatus(g, fmt.Sprintf("Choose who receives %s.", core.SkillName(skill)))
	case core.ActionEnemyTarget:
		// Snaps the cursor to a reachable foe (front for melee, any for ranged/magic).
		enterEnemyTargeting(g, fmt.Sprintf("Choose a target for %s.", core.SkillName(skill)))
	default:
		beginPendingAction(g)
	}
}

// battlePendingAttackMelee reports whether the pending action is a melee attack
// (front-gated): basic attack keys off the equipped weapon, a skill off its reach
// class. Thin alias for core.BattlePendingAttackIsMelee (the shared body the
// renderer also uses); kept so battle call sites read locally.
func battlePendingAttackMelee(g *core.GameState) bool {
	return core.BattlePendingAttackIsMelee(g)
}

// battleEnemyTargets returns selectable enemy slots: effective front row for melee,
// every living enemy for ranged/magic.
func battleEnemyTargets(g *core.GameState) []int {
	if battlePendingAttackMelee(g) {
		return core.MeleeReachableBattleEnemyIndices(g)
	}
	return core.LivingBattleEnemyIndices(g)
}

// refuseMeleeAction handles a greyed melee ACTION (Attack row or melee skill) picked
// anyway: buzz + log the reason — stuck in the back row, else up front but every foe
// flying — leaving the turn unspent. The single source so the Attack-row and skill-
// submenu refusals stay one pattern (same buzz, same log surface, same wording).
// Returns true when refused; callers guard with their own melee check:
// `if isMelee && refuseMeleeAction(g) { return }`. A no-op (returns false) when reachable.
func refuseMeleeAction(g *core.GameState) bool {
	backRow := !core.PartyInEffectiveFront(g.Party, g.Battle.CurrentParty)
	if !backRow && !core.NoMeleeReachableEnemy(g) {
		return false // a foe is in reach — not refused
	}
	audio.Play(audio.SoundInputMiss)
	msg := msgMeleeFlyingFmt
	if backRow {
		msg = msgMeleeBackRowFmt
	}
	if m, ok := currentMember(g); ok {
		setBattleMessage(g, fmt.Sprintf(msg, m.Name))
	}
	return true
}

// enterEnemyTargeting opens the enemy target picker, enforcing melee reach (a
// back-row melee attacker is refused) and snapping the cursor to a reachable foe.
// Returns false (menu state intact) when barred or no target is reachable.
func enterEnemyTargeting(g *core.GameState, prompt string) bool {
	if battlePendingAttackMelee(g) && refuseMeleeAction(g) {
		return false
	}
	targets := battleEnemyTargets(g)
	if len(targets) == 0 {
		setBattleStatus(g, "No reachable target.")
		return false
	}
	if !slices.Contains(targets, g.Battle.EnemyIndex) {
		g.Battle.EnemyIndex = targets[0]
	}
	g.Battle.ActionMode = core.ActionEnemyTarget
	setBattleStatus(g, prompt)
	return true
}

// updateItemMenu drives the inventory picker; Confirm moves to ally-target
// selection. Items only heal allies for now, so target mode is always party.
func updateItemMenu(g *core.GameState) {
	refreshItemMenuBuf(g)
	living := g.Battle.ItemMenuList
	count := len(living)
	if count == 0 {
		// Inventory ran dry since open — not reachable today, but bail defensively.
		resetBattleAction(g)
		setBattleStatus(g, msgNoItems)
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
	// ItemMenuIndex is already in range: CursorUpDown re-clamps to [0,count-1] every
	// frame with this same count (count>0 ensured by the empty-list bail above).
	picked := living[g.Battle.ItemMenuIndex].Kind
	g.Battle.PendingItem = picked
	g.Battle.ActionMode = core.ActionItemTarget
	g.Battle.PartyTarget = g.Battle.CurrentParty
	setBattleStatus(g, fmt.Sprintf("Use %s on whom?", core.ItemInfo(picked).Name))
}

// updateTargetPicker is the shared input loop for every target-selection mode.
// Hooks take *core.GameState directly so callers pass non-allocating top-level
// functions rather than fresh per-frame closures capturing g.
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

// updateItemTarget picks the ally to receive the pending item, routing through applyItem.
func updateItemTarget(g *core.GameState) {
	updateTargetPicker(g, cyclePartyTarget, cancelTargetToItemMenu, applyItem)
}

// cancelTargetToItemMenu steps back to the item picker, not the action menu —
// target-back cancels the target selection, not the whole action.
func cancelTargetToItemMenu(g *core.GameState) {
	g.Battle.ActionMode = core.ActionItemMenu
	refreshItemMenuBuf(g)
	setBattleStatus(g, msgChooseItem)
}

// applyItem consumes the pending item, heals the targeted ally, and ends the
// turn. No timing minigame — the turn + finite resource are demand enough.
func applyItem(g *core.GameState) {
	kind := g.Battle.PendingItem
	if kind == core.ItemNone {
		resetBattleAction(g)
		return
	}
	if _, ok := currentMember(g); !ok {
		// Defensive: don't index g.Party with a stale CurrentParty mid-transition.
		resetBattleAction(g)
		return
	}
	target := g.Battle.PartyTarget
	// Re-check Ingested (a mantrap can swallow the chosen ally between target-select
	// and confirm): without it the stack is consumed and the heal no-ops — item wasted.
	if !partyIndexValid(g, target) || !core.MemberAvailable(g.Party[target]) {
		setBattleStatus(g, msgInvalidTarget)
		return
	}
	def := core.ItemInfo(kind)
	// Don't spend a restorative on a target it can't help (core.ItemHelpsTarget
	// refuses only when full on every axis the item restores).
	if !core.ItemHelpsTarget(def, g.Party[target]) {
		setBattleStatus(g, g.Party[target].Name+" doesn't need that right now.")
		return
	}
	// Confused fumble onto a random living ally. Deferred until AFTER the
	// validity/help gates so it fires once on the turn-committing path — running it
	// first let a re-confirm (after a non-turn-ending bounce) re-roll and spam the log.
	maybeConfuseRetarget(g)
	target = g.Battle.PartyTarget
	tgt := &g.Party[target]
	// Consume first; bail without spending the turn if the stack vanished (defensive).
	updated, ok := core.ConsumeItem(g.Inventory, kind)
	if !ok {
		setBattleStatus(g, "Item not in inventory.")
		resetBattleAction(g)
		return
	}
	g.Inventory = updated
	// Confusion may have redirected the item onto an ally it can't help — the top gate
	// only vetted the player's CHOSEN target, not the confused re-roll. The stack is
	// still spent (the fumble is the confusion penalty), but report it honestly instead
	// of a generic "uses" that implies it did something.
	if !core.ItemHelpsTarget(def, *tgt) {
		caster := &g.Party[g.Battle.CurrentParty]
		stampPartyBump(caster)
		setBattleMessage(g, fmt.Sprintf("Confused, %s fumbles %s onto %s.", caster.Name, def.Name, tgt.Name))
		g.Battle.PendingItem = core.ItemNone
		finishActorTurn(g)
		return
	}
	// core.ApplyRestorative owns the feed→heal→restore order (shared with explore).
	res := core.ApplyRestorative(tgt, def)
	if res.HP > 0 {
		tgt.DamageFlash = core.FlashDuration // battle-only VFX: flash + cue on a real HP heal
		audio.Play(audio.SoundHeal)
	}
	actor := &g.Party[g.Battle.CurrentParty]
	stampPartyBump(actor)
	setBattleMessageCat(g, core.ItemUseMessage(tgt.Name, def, res), core.RestorativeUseCategory(res))
	g.Battle.PendingItem = core.ItemNone
	finishActorTurn(g)
}

// performDefend marks the current member defending and ends their turn. No timing
// minigame — the boost is the whole reward, a bar would imply a nonexistent better outcome.
func performDefend(g *core.GameState) {
	member, ok := currentMember(g)
	if !ok {
		return
	}
	member.Defending = true
	setBattleMessage(g, fmt.Sprintf("%s braces for impact.", member.Name))
	finishActorTurn(g)
}

// Battle Swap is a tactical, live-only move: the actor can trade slots with ANY
// other LIVING member (not just an orthogonal neighbour). Downed members are
// shunted to the back and excluded (not swappable). nextLivingSwapTarget cycles
// the roster; defaultSwapPartner picks the picker's opening highlight.

// nextLivingSwapTarget steps from `from` in dir (+1 forward / -1 back) to the next
// living member that isn't the acting member, wrapping. ok=false if none exists.
func nextLivingSwapTarget(g *core.GameState, from, dir int) (int, bool) {
	n := len(g.Party)
	if n == 0 || dir == 0 {
		return 0, false
	}
	actor := g.Battle.CurrentParty
	for step := 1; step <= n; step++ {
		i := core.WrapIndex(from+dir*step, n)
		if i != actor && g.Party[i].HP > 0 {
			return i, true
		}
	}
	return 0, false
}

// defaultSwapPartner is the member the Swap picker opens on: the first living
// member after the actor, or -1 if the actor is the last one standing.
func defaultSwapPartner(g *core.GameState) int {
	actor := g.Battle.CurrentParty
	if !core.PartyIndexInRange(g.Party, actor) {
		return -1
	}
	if idx, ok := nextLivingSwapTarget(g, actor, +1); ok {
		return idx
	}
	return -1
}

// enterSwapTargeting opens the Swap picker on the first living partner; the actor
// stays the source and the D-pad cycles the cursor across all living members.
func enterSwapTargeting(g *core.GameState) {
	if _, ok := currentMember(g); !ok {
		return
	}
	partner := defaultSwapPartner(g)
	if partner < 0 {
		setBattleStatus(g, msgChooseAction)
		return
	}
	g.Battle.PartyTarget = partner
	g.Battle.ActionMode = core.ActionSwapTarget
	setBattleStatus(g, fmt.Sprintf("Swap with %s?", g.Party[partner].Name))
}

// updateSwapTarget drives the Swap picker: the D-pad cycles the cursor across all
// living members, Confirm trades slots.
func updateSwapTarget(g *core.GameState) {
	if input.BackPressed() {
		cancelTargetToActionMenu(g)
		return
	}
	if idx, ok := swapTargetForDirection(g); ok {
		g.Battle.PartyTarget = idx
		setBattleStatus(g, fmt.Sprintf("Swap with %s?", g.Party[idx].Name))
	}
	if input.ConfirmPressed() {
		performSwap(g)
	}
}

// swapTargetForDirection cycles the Swap cursor across every OTHER living member —
// Down/Right step forward, Up/Left step back (any slot; swap is no longer limited to
// an orthogonal neighbour). ok=false if no direction was pressed.
func swapTargetForDirection(g *core.GameState) (int, bool) {
	return nextLivingSwapTarget(g, g.Battle.PartyTarget, input.CursorStep())
}

// performSwap exchanges the actor's formation slot with the cursored partner's and
// ends the turn. Any living partner is valid (a downed member or the actor isn't).
func performSwap(g *core.GameState) {
	actor, ok := currentMember(g)
	if !ok {
		resetBattleAction(g)
		return
	}
	partner := g.Battle.PartyTarget
	a := g.Battle.CurrentParty
	if partner == a || !partyIndexValid(g, partner) {
		setBattleStatus(g, msgInvalidTarget)
		return
	}
	if g.Party[partner].HP <= 0 {
		setBattleStatus(g, msgInvalidTarget) // can't swap with a downed member
		return
	}
	actorName, partnerName := actor.Name, g.Party[partner].Name
	// Capture each member's pre-swap slot so the sprites GLIDE between positions
	// (StampSwapSlide) instead of teleporting once the live slots flip.
	fromRowA, fromColA := g.Party[a].Row, g.Party[a].Col
	fromRowB, fromColB := g.Party[partner].Row, g.Party[partner].Col
	// Tactical, live-only: trade the on-screen (live) slots for this fight. The
	// preferred Home formation is untouched and restored next battle.
	core.SwapLiveSlots(g.Party, a, partner)
	core.StampSwapSlide(&g.Party[a], fromRowA, fromColA)
	core.StampSwapSlide(&g.Party[partner], fromRowB, fromColB)
	setBattleMessage(g, core.SwapPlacesMessage(actorName, partnerName))
	finishActorTurn(g)
}

// performFlee attempts escape (core.FleeChance scales party vs pack level). On
// success the party retreats to the pre-combat tile and the pack STAYS on the
// field; on failure the attempt burns the turn.
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
	// Escaped: snap back to the pre-combat tile. That tile is always walkable; the
	// only hazard is another pack having ambush-stepped onto it — only reposition
	// when no pack occupies it, else escape in place rather than land on a pack.
	if core.PackIndexAtTile(g.Packs, g.Battle.FleeReturnX, g.Battle.FleeReturnZ) < 0 {
		g.Player.TileX = g.Battle.FleeReturnX
		g.Player.TileZ = g.Battle.FleeReturnZ
		// Re-seat the standing level: keep the carried level if still standable
		// (flee back onto the bridge deck, not the ground below), else snap to the
		// lowest standable. Same rule the save loader uses; no-op on a heightfield.
		if !g.Area.Standable(g.Player.TileX, g.Player.Level, g.Player.TileZ) {
			if lo := g.Area.LowestStandableLevel(g.Player.TileX, g.Player.TileZ); lo >= 0 {
				g.Player.Level = lo
			}
		}
		core.SnapPlayerToTile(&g.Player)
		g.Player.Anim = core.Animation{}
	}
	// Fleeing forfeits all progress against the pack: fully heal + revive its
	// members so re-engaging starts fresh (escape from death, not attrition).
	core.RestorePackFull(pack)
	leaveBattle(g, fmt.Sprintf("%s leads the party in a hasty retreat!", member.Name))
}

func updateEnemyTargeting(g *core.GameState) {
	updateTargetPicker(g, cycleBattleTarget, cancelTargetToActionMenu, confirmEnemyTarget)
}

// confirmEnemyTarget commits the pending action on the cursor's foe — an
// unreachable one (greyed) buzzes + logs the reason rather than burning the turn.
// Top-level (not a closure) so updateTargetPicker stays alloc-free.
func confirmEnemyTarget(g *core.GameState) {
	slot := g.Battle.EnemyIndex
	if !core.BattleEnemyTargetReachable(g, slot) {
		audio.Play(audio.SoundInputMiss)
		setBattleMessage(g, unreachableMeleeTargetMsg(g, slot))
		return
	}
	beginPendingAction(g)
}

// unreachableMeleeTargetMsg explains WHY a melee attack can't hit the cursor's foe:
// a flying foe is melee-immune (needs a ranged weapon), else it's a covered back-row foe.
func unreachableMeleeTargetMsg(g *core.GameState, slot int) string {
	members := core.BattleMembers(g)
	if slot >= 0 && slot < len(members) && core.EnemyFlying(&members[slot]) {
		return msgFlyingMeleeTarget
	}
	return msgBackRowMeleeTarget
}

func updatePartyTargeting(g *core.GameState) {
	updateTargetPicker(g, cyclePartyTarget, cancelTargetToActionMenu, beginPendingAction)
}

// cancelTargetToActionMenu is the shared Back handler for the target pickers —
// drop the pending action and return to the action menu.
func cancelTargetToActionMenu(g *core.GameState) {
	resetBattleAction(g)
	setBattleStatus(g, msgChooseAction)
}

// cycleTargetSelection is the shared body for the enemy / party target cyclers:
// fetch live indices, wrap the cursor by delta, commit + announce the new pick.
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
	// Cycle EVERY living foe, including greyed back-row ones a melee weapon can't
	// reach — the cursor can land on one to aim; confirmEnemyTarget gates the hit.
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

// cycleTarget returns the targets-slot the cursor moves to on a left/right press.
// `current` is expected to be in `targets`; if not, it falls back to slot 0 and
// logs the broken selection invariant rather than failing silently.
func cycleTarget(current int, targets []int, delta int) int {
	currentSlot := slices.Index(targets, current)
	if currentSlot < 0 {
		log.Printf("battle.cycleTarget: current=%d not in targets=%v (selection invariant broken)", current, targets)
		currentSlot = 0
	}
	return core.WrapIndex(currentSlot+delta, len(targets))
}
