package battle

import (
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"fmt"
)

// actionSetup runs validation/cost-payment for the player's chosen action and
// returns true if the action is ready to enter the timed-hit minigame. The
// caller transitions to BattleAttackTiming on success; on failure a status
// message is set and the player stays in the action menu.
type actionSetup func(g *core.GameState) bool

// actionApply resolves the effect of the action with the given timing quality.
// It writes the combat-log message, applies damage/heal/etc., and finishes by
// returning to the caller — phase advancement is handled by the battle loop.
// Returns landed=true if the action actually went through (so the caller can
// gate the actor's quality popup); landed=false on a defensive no-op (e.g.
// the target died between confirm and apply) so we don't show "Excellent!"
// over the actor for a cancelled action.
type actionApply func(g *core.GameState, quality int) (landed bool)

type actionHandlers struct {
	setup actionSetup
	apply actionApply
}

var skillActionHandlers = map[int]actionHandlers{
	core.SkillSwipe:    {setup: setupSwipe, apply: applySwipe},
	core.SkillPrayer:   {setup: setupPrayer, apply: applyPrayer},
	core.SkillSteal:    {setup: setupSteal, apply: applySteal},
	core.SkillFirebolt: {setup: setupFirebolt, apply: applyFirebolt},
}

// beginPendingAction is invoked once the player has confirmed their target
// (or their no-target action). It validates / pays cost and, on success,
// arms the timing bar.
func beginPendingAction(g *core.GameState) {
	handler, ok := actionHandlerFor(g.Battle.PendingSkill)
	if !ok {
		resetBattleAction(g)
		setBattleStatus(g, "No skill ready.")
		return
	}
	if !handler.setup(g) {
		// setup populates the status message on failure; stay in the menu.
		return
	}
	intro := core.AttackTimingIntro
	switch {
	case usesChargeMinigame(g.Battle.PendingSkill):
		g.Battle.Timing = core.NewChargeState(core.ChargeTimingDuration)
		// Charge gets a longer pre-arm pause so the player has time to read
		// the prompt; pressing/holding the input during the intro skips
		// straight into the bar (handled in updateAttackTiming).
		intro = core.ChargeTimingIntro
	case usesSequenceMinigame(g.Battle.PendingSkill):
		g.Battle.Timing = core.NewSequenceState(core.StealTimingDuration, core.StealSequenceLength)
		// Clear analog-stick edge memory so a player whose stick happens to
		// be tilted when the bar arms doesn't get a phantom input on frame 1.
		input.ResetStickEdges()
	default:
		g.Battle.Timing = core.NewTimingState(core.AttackTimingDuration)
	}
	g.Battle.TimingFlash = 0
	g.Battle.TimingIntro = intro
	g.Battle.Phase = core.BattleAttackTiming
}

// usesChargeMinigame is the per-skill switch that picks the charge timing
// kind. Firebolt and Prayer are hold-and-release.
func usesChargeMinigame(skill int) bool {
	return skill == core.SkillFirebolt || skill == core.SkillPrayer
}

// usesSequenceMinigame picks the pickpocket sequence kind. Steal needs a
// thief-y rhythm input, not a single press.
func usesSequenceMinigame(skill int) bool {
	return skill == core.SkillSteal
}

// applyPendingAction is invoked once the timing bar resolves. It runs the
// recorded action's effect with the resulting quality and records the actor's
// quality popup if (and only if) the action actually landed. Phase advancement
// happens inside the apply* path via finishPartyAction.
func applyPendingAction(g *core.GameState, quality int) {
	handler, ok := actionHandlerFor(g.Battle.PendingSkill)
	if !ok {
		setBattleStatus(g, "No skill ready.")
		return
	}
	if landed := handler.apply(g, quality); landed {
		recordAttackQuality(g, quality)
	}
}

func actionHandlerFor(skill int) (actionHandlers, bool) {
	if skill == core.SkillNone {
		return actionHandlers{setup: setupAttack, apply: applyAttack}, true
	}
	handler, ok := skillActionHandlers[skill]
	return handler, ok
}

// recordEnemyDamagePopup stamps a floating-number popup on the given enemy
// so the renderer can draw the damage value above its sprite. Should be
// called immediately after damageEnemy whenever a player action lands.
func recordEnemyDamagePopup(g *core.GameState, enemyIndex, damage, quality int) {
	if enemyIndex < 0 || enemyIndex >= len(g.Enemies) || damage <= 0 {
		return
	}
	g.Enemies[enemyIndex].DamagePopup = damage
	g.Enemies[enemyIndex].DamagePopupQuality = quality
	g.Enemies[enemyIndex].DamagePopupTimer = core.QualityResultDuration
}

// recordAttackQuality stores the quality + actor for the floating popup that
// renders for QualityResultDuration after the bar resolves.
func recordAttackQuality(g *core.GameState, quality int) {
	g.Battle.LastQuality = quality
	g.Battle.LastQualityTimer = core.QualityResultDuration
	g.Battle.LastQualityIndex = g.Battle.CurrentParty
	g.Battle.LastQualityIsBlock = false
}

// recordBlockQuality is the defend-side counterpart, used when an enemy
// attack resolves and the player's defend timing produced a quality grade.
func recordBlockQuality(g *core.GameState, quality int, partyIndex int) {
	g.Battle.LastQuality = quality
	g.Battle.LastQualityTimer = core.QualityResultDuration
	g.Battle.LastQualityIndex = partyIndex
	g.Battle.LastQualityIsBlock = true
}

// --- Basic Attack ---

func setupAttack(g *core.GameState) bool {
	if !core.EnemyAlive(g.Enemies, g.Battle.EnemyIndex) {
		setBattleStatus(g, "No target.")
		return false
	}
	return true
}

func applyAttack(g *core.GameState, quality int) bool {
	if !core.EnemyAlive(g.Enemies, g.Battle.EnemyIndex) {
		setBattleStatus(g, "No target.")
		finishPartyAction(g)
		return false
	}
	attacker := &g.Party[g.Battle.CurrentParty]
	attacker.AttackBump = core.BumpDuration
	target := g.Enemies[g.Battle.EnemyIndex]
	// Basic Attack: STR + 0, scaled by timing quality.
	damage := core.ScaleDamage(core.MeleeDamage(attacker.Stats, 0), quality)
	defeated := damageEnemy(g, g.Battle.EnemyIndex, damage)
	recordEnemyDamagePopup(g, g.Battle.EnemyIndex, damage, quality)
	setBattleMessage(g, attackResultMessage(attacker.Name, target, damage, quality, defeated))
	finishPartyAction(g)
	return true
}

// --- Swipe (Warrior, hits all enemies in the battle group) ---

func setupSwipe(g *core.GameState) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	cost := core.SkillCost(core.SkillSwipe)
	if actor.MP < cost {
		setBattleStatus(g, "Swipe needs more MP.")
		return false
	}
	actor.MP -= cost
	return true
}

func applySwipe(g *core.GameState, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	// Damage formula is dispatched by skill Kind in core.SkillDamage; Swipe's
	// Kind is Melee so this resolves to STR + Effect.Damage.
	damage := core.ScaleDamage(core.SkillDamage(actor.Stats, core.SkillSwipe), quality)
	hits := 0
	for _, index := range g.Battle.EnemyGroup {
		if !core.EnemyAlive(g.Enemies, index) {
			continue
		}
		damageEnemy(g, index, damage)
		recordEnemyDamagePopup(g, index, damage, quality)
		hits++
	}
	if hits == 0 {
		setBattleMessage(g, "Swipe catches only air.")
	} else {
		setBattleMessage(g, swipeMessage(actor.Name, hits, quality))
	}
	finishPartyAction(g)
	// Even if hits=0, the attack motion played and MP was spent — landed.
	return true
}

// --- Prayer (Cleric, heals an ally) ---

func setupPrayer(g *core.GameState) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	cost := core.SkillCost(core.SkillPrayer)
	if actor.MP < cost {
		setBattleStatus(g, "Prayer needs more MP.")
		return false
	}
	if g.Battle.PartyTarget < 0 || g.Battle.PartyTarget >= len(g.Party) {
		setBattleStatus(g, "No ally selected.")
		return false
	}
	if g.Party[g.Battle.PartyTarget].HP <= 0 {
		setBattleStatus(g, "Prayer cannot revive.")
		return false
	}
	actor.MP -= cost
	return true
}

func applyPrayer(g *core.GameState, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	// Heal formula is dispatched by skill Kind in core.SkillHeal; Prayer's
	// Kind is Heal so this resolves to WIS + Effect.Heal.
	heal := core.ScaleHeal(core.SkillHeal(actor.Stats, core.SkillPrayer), quality)
	target := &g.Party[g.Battle.PartyTarget]
	healPartyMember(g, g.Battle.PartyTarget, heal)
	selfTarget := g.Battle.PartyTarget == g.Battle.CurrentParty
	setBattleMessage(g, prayerMessage(actor.Name, target.Name, heal, quality, selfTarget))
	finishPartyAction(g)
	return true
}

// --- Steal (Thief, base chance scales with quality) ---

func setupSteal(g *core.GameState) bool {
	if !core.EnemyAlive(g.Enemies, g.Battle.EnemyIndex) {
		setBattleStatus(g, "No target.")
		return false
	}
	return true
}

func applySteal(g *core.GameState, quality int) bool {
	if !core.EnemyAlive(g.Enemies, g.Battle.EnemyIndex) {
		setBattleStatus(g, "No target.")
		finishPartyAction(g)
		return false
	}
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	enemy := &g.Enemies[g.Battle.EnemyIndex]
	if enemy.Item == "" {
		setBattleMessage(g, "There is nothing to steal.")
		finishPartyAction(g)
		// The thief's hand still moved — popup with the quality reads as
		// "graded an empty grab" which is fine.
		return true
	}
	effect := core.SkillEffectFor(core.SkillSteal)
	// Steal chance: base × (1 + DEX/20), then quality multiplier on top.
	// Capped at 1.0 so a perfect-Excellent high-DEX thief still rolls.
	chance := core.StealChance(actor.Stats, effect.StealChance) * float64(core.TimingBonusMult(quality))
	if chance > 1 {
		chance = 1
	}
	if core.GameRNG.Float64() < chance {
		item := enemy.Item
		enemy.Item = ""
		// Drop the loot into shared inventory so the Item action can use it
		// later. Unknown item names (none today, but defensive) silently
		// don't add — better than panicking.
		if kind := core.ItemKindByName(item); kind != core.ItemNone {
			g.Inventory = core.AddItem(g.Inventory, kind, 1)
		}
		setBattleMessage(g, stealMessage(actor.Name, item, quality))
	} else {
		setBattleMessage(g, fmt.Sprintf("%s fails to steal.", actor.Name))
	}
	finishPartyAction(g)
	return true
}

// --- Firebolt (Wizard, ramps damage and burn chance with quality) ---

func setupFirebolt(g *core.GameState) bool {
	if !core.EnemyAlive(g.Enemies, g.Battle.EnemyIndex) {
		setBattleStatus(g, "No target.")
		return false
	}
	actor := &g.Party[g.Battle.CurrentParty]
	cost := core.SkillCost(core.SkillFirebolt)
	if actor.MP < cost {
		setBattleStatus(g, "Firebolt needs more MP.")
		return false
	}
	// MP isn't deducted here. The cost is paid in apply() once the bar
	// resolves AND the target is still valid, so a target dying between
	// confirm and apply doesn't burn MP for nothing.
	return true
}

func applyFirebolt(g *core.GameState, quality int) bool {
	if !core.EnemyAlive(g.Enemies, g.Battle.EnemyIndex) {
		setBattleStatus(g, "No target.")
		finishPartyAction(g)
		return false
	}
	actor := &g.Party[g.Battle.CurrentParty]
	cost := core.SkillCost(core.SkillFirebolt)
	if actor.MP < cost {
		// Defensive: setupFirebolt already checked MP, but if anything between
		// then and now spent it (no current code path does, but keep the check
		// honest) we bail without finishing — the action menu will repaint.
		setBattleStatus(g, "Firebolt needs more MP.")
		finishPartyAction(g)
		return false
	}
	actor.MP -= cost
	actor.AttackBump = core.BumpDuration
	effect := core.SkillEffectFor(core.SkillFirebolt)
	// Damage formula is dispatched by skill Kind in core.SkillDamage; Firebolt's
	// Kind is Magic so this resolves to INT + Effect.Damage. We still pull
	// Effect separately for the burn-chance roll below.
	damage := core.ScaleDamage(core.SkillDamage(actor.Stats, core.SkillFirebolt), quality)
	target := g.Enemies[g.Battle.EnemyIndex]
	defeated := damageEnemy(g, g.Battle.EnemyIndex, damage)
	recordEnemyDamagePopup(g, g.Battle.EnemyIndex, damage, quality)
	enemy := &g.Enemies[g.Battle.EnemyIndex]
	burned := false
	if !defeated && enemy.BurnTurns <= 0 {
		burnChance := effect.BurnChance * float64(core.TimingBonusMult(quality))
		if burnChance > 1 {
			burnChance = 1
		}
		if core.GameRNG.Float64() < burnChance {
			enemy.BurnTurns = effect.BurnDuration()
			burned = true
		}
	}
	setBattleMessage(g, fireboltMessage(actor.Name, target, damage, quality, defeated, burned, enemy.BurnTurns))
	finishPartyAction(g)
	return true
}

// --- Damage / heal helpers (unchanged from previous behavior) ---

func damageEnemy(g *core.GameState, enemyIndex, damage int) bool {
	if !core.EnemyAlive(g.Enemies, enemyIndex) {
		return false
	}
	enemy := &g.Enemies[enemyIndex]
	enemy.DamageFlash = core.FlashDuration
	enemy.HP -= damage
	if enemy.HP > 0 {
		return false
	}
	enemy.HP = 0
	enemy.Alive = false
	enemy.BurnTurns = 0
	enemy.DeathFade = core.DeathFadeDuration
	return true
}

// tickBurnAtTurnStart resolves the per-tick burn damage on a burning actor at
// the start of their turn. Currently only enemies can burn, but the helper
// takes an ActorRef so a future "party can be burned" effect can hook in
// without reshaping the scheduler. Returns true if the burn killed the actor
// (caller should skip their turn).
func tickBurnAtTurnStart(g *core.GameState, actor core.ActorRef) bool {
	if actor.IsParty {
		return false
	}
	if actor.Index < 0 || actor.Index >= len(g.Battle.EnemyGroup) {
		return false
	}
	enemyIdx := g.Battle.EnemyGroup[actor.Index]
	if enemyIdx < 0 || enemyIdx >= len(g.Enemies) {
		return false
	}
	enemy := &g.Enemies[enemyIdx]
	if !enemy.Alive || enemy.BurnTurns <= 0 {
		return false
	}
	enemy.BurnTurns--
	damageEnemy(g, enemyIdx, core.BurnTickDamage)
	// Quality=Good gives the orange "fire" tint to the floating popup.
	recordEnemyDamagePopup(g, enemyIdx, core.BurnTickDamage, core.TimingQualityGood)
	def := core.EnemyInfoFor(*enemy)
	if !enemy.Alive {
		setBattleMessage(g, fmt.Sprintf("The %s succumbs to the flames.", def.SingularNoun))
		// Repoint the player's targeting cursor if the burn killed the
		// currently-selected enemy, so the next attack action has a valid one.
		if next := core.NextLivingBattleEnemy(g); next >= 0 && !core.EnemyAlive(g.Enemies, g.Battle.EnemyIndex) {
			g.Battle.EnemyIndex = next
		}
		return true
	}
	setBattleMessage(g, fmt.Sprintf("The %s burns for %d.", def.SingularNoun, core.BurnTickDamage))
	return false
}

func healPartyMember(g *core.GameState, partyIndex, amount int) bool {
	if partyIndex < 0 || partyIndex >= len(g.Party) || amount <= 0 {
		return false
	}
	member := &g.Party[partyIndex]
	if member.HP <= 0 {
		return false
	}
	member.HP += amount
	if member.HP > member.MaxHP {
		member.HP = member.MaxHP
	}
	member.DamageFlash = core.FlashDuration
	return true
}

func damagePartyMember(g *core.GameState, partyIndex, amount int) bool {
	if partyIndex < 0 || partyIndex >= len(g.Party) || amount <= 0 {
		return false
	}
	member := &g.Party[partyIndex]
	if member.HP <= 0 {
		return false
	}
	member.DamageFlash = core.FlashDuration
	member.HP -= amount
	if member.HP > 0 {
		return false
	}
	member.HP = 0
	return true
}

// finishPartyAction is the apply* hand-off — kept as a thin wrapper so the
// per-skill apply functions keep their old names. Mixed-initiative scheduling
// lives in finishActorTurn (battle.go), which advances the round queue.
func finishPartyAction(g *core.GameState) {
	finishActorTurn(g)
}

// --- Result text ---

func attackResultMessage(name string, target core.Enemy, damage, quality int, defeated bool) string {
	tag := qualityTag(quality)
	if defeated {
		return fmt.Sprintf("%s%s drops a %s for %d.", tag, name, core.EnemySingularNoun(target), damage)
	}
	return fmt.Sprintf("%s%s hits for %d.", tag, name, damage)
}

func swipeMessage(name string, hits, quality int) string {
	tag := qualityTag(quality)
	return fmt.Sprintf("%s%s swipes through %d foes.", tag, name, hits)
}

func prayerMessage(name, targetName string, heal, quality int, self bool) string {
	tag := qualityTag(quality)
	if self {
		return fmt.Sprintf("%s%s prays for themselves (+%d HP).", tag, name, heal)
	}
	return fmt.Sprintf("%s%s prays over %s (+%d HP).", tag, name, targetName, heal)
}

func stealMessage(name, item string, quality int) string {
	tag := qualityTag(quality)
	return fmt.Sprintf("%s%s steals %s.", tag, name, item)
}

func fireboltMessage(name string, target core.Enemy, damage, quality int, defeated, burned bool, burnTurns int) string {
	tag := qualityTag(quality)
	switch {
	case defeated:
		return fmt.Sprintf("%s%s's Firebolt drops the %s.", tag, name, core.EnemySingularNoun(target))
	case burned:
		return fmt.Sprintf("%s%s scorches the %s for %d. Burning!", tag, name, core.EnemySingularNoun(target), damage)
	case burnTurns > 0:
		return fmt.Sprintf("%s%s hits for %d. Burn is already active.", tag, name, damage)
	default:
		return fmt.Sprintf("%s%s hits for %d.", tag, name, damage)
	}
}

func qualityTag(quality int) string {
	if quality == core.TimingQualityMiss || quality == core.TimingQualityNice {
		return ""
	}
	return core.TimingQualityLabel(quality) + " "
}

// resolveEnemyAttacker applies a single enemy's attack against a chosen party
// member, scaled by the player's defend quality. Used by the BattleEnemyTiming
// phase. Returns true if the hit landed (false if attacker was already dead).
func resolveEnemyAttacker(g *core.GameState, enemyIndex int, defendQuality int) bool {
	if enemyIndex < 0 || enemyIndex >= len(g.Enemies) || !g.Enemies[enemyIndex].Alive {
		return false
	}
	target := pickEnemyAttackTarget(g)
	if target < 0 {
		return false
	}
	g.Enemies[enemyIndex].AttackBump = core.BumpDuration
	rawDamage := core.EnemyInfoFor(g.Enemies[enemyIndex]).AttackDamage
	damage := core.ScaleIncomingDamage(rawDamage, defendQuality)
	if g.Party[target].Defending {
		scaled := int(float32(damage) * core.DefendingDamageMult)
		// Don't let Defending fully zero a hit unless the timing block already
		// did — that way Defend is a meaningful soak, not a free immunity.
		if scaled <= 0 && damage > 0 {
			scaled = 1
		}
		damage = scaled
	}
	damagePartyMember(g, target, damage)
	if defendQuality > core.TimingQualityMiss {
		// A successful block recoils the defender slightly so the impact reads
		// even though the damage number is small.
		g.Party[target].AttackBump = core.BlockBumpDuration
	}
	recordBlockQuality(g, defendQuality, target)
	setBattleMessage(g, enemyHitMessage(g.Enemies[enemyIndex], g.Party[target].Name, damage, defendQuality, g.Party[target].Defending))
	return true
}

// pickEnemyAttackTarget cycles to the next living party member after the last
// one targeted, mirroring the previous round-robin behavior.
func pickEnemyAttackTarget(g *core.GameState) int {
	cursor := g.Battle.PartyTarget + 1
	if cursor < 0 {
		cursor = 0
	}
	target := core.NextLivingPartyMember(g.Party, cursor)
	if target < 0 {
		target = core.FirstLivingPartyMember(g.Party)
	}
	if target >= 0 {
		g.Battle.PartyTarget = target
	}
	return target
}

func enemyHitMessage(enemy core.Enemy, targetName string, damage, defendQuality int, defending bool) string {
	def := core.EnemyInfoFor(enemy)
	if defendQuality > core.TimingQualityMiss {
		if damage <= 0 {
			return fmt.Sprintf("%s blocks the %s!", targetName, def.SingularNoun)
		}
		return fmt.Sprintf("%s blocks the %s (%d).", targetName, def.SingularNoun, damage)
	}
	if defending {
		return fmt.Sprintf("%s soaks the %s for %d.", targetName, def.SingularNoun, damage)
	}
	return fmt.Sprintf("The %s %s %s for %d.", def.SingularNoun, def.AttackVerbSingular, targetName, damage)
}
