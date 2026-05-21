package battle

import (
	"crawler/internal/app/audio"
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

// skillActionHandlers is the player-castable skill registry. Skills
// registered here are valid choices from the action menu and have a
// setup/apply pair driving the timing minigame. Enemy-only skills
// (SkillSleep, SkillIngest) route through resolveEnemySpell in battle.go
// and deliberately don't appear here — actionHandlerFor returns ok=false
// for any unregistered skill, which beginPendingAction surfaces as "No
// skill ready." A future "player learns Sleep" feature flips
// PlayerCastable on the skill registry, adds a row here, and the init()
// below auto-verifies the wiring.
var skillActionHandlers = map[core.SkillID]actionHandlers{
	core.SkillSwipe:    {setup: setupSwipe, apply: applySwipe},
	core.SkillPrayer:   {setup: setupPrayer, apply: applyPrayer},
	core.SkillSteal:    {setup: setupTargetedEnemy, apply: applySteal},
	core.SkillFirebolt: {setup: setupFirebolt, apply: applyFirebolt},
}

// init asserts the two halves of the player-castable contract stay in
// sync: every party class's Skill must be PlayerCastable AND have an
// entry in skillActionHandlers, and every PlayerCastable skill in the
// registry must have a handler. Without this, a future class that
// pointed at an enemy-only skill (or a registry author who forgot to
// register a handler) would only surface at playtest as "No skill
// ready." — a vague runtime error far from the cause.
func init() {
	for _, def := range core.PartyClasses() {
		if !core.SkillPlayerCastable(def.Skill) {
			panic("battle: class " + def.Name + " skill is not PlayerCastable — flip the flag in core/party.go skillDefinitions")
		}
		if _, ok := skillActionHandlers[def.Skill]; !ok {
			panic("battle: class " + def.Name + " skill has no skillActionHandlers entry — register a setup/apply pair")
		}
	}
	for _, s := range core.PlayerCastableSkills() {
		if _, ok := skillActionHandlers[s]; !ok {
			panic("battle: PlayerCastable skill " + core.SkillName(s) + " has no skillActionHandlers entry")
		}
	}
}

// setupTargetedEnemy is the shared "must have a live target" check used
// by basic attack and Steal — both gate purely on whether g.Battle.EnemyIndex
// still points at a living member of the active pack.
func setupTargetedEnemy(g *core.GameState) bool {
	if !core.BattleEnemyAlive(g, g.Battle.EnemyIndex) {
		setBattleStatus(g, "No target.")
		return false
	}
	return true
}

// chargeMP is the shared "spend the skill's MP cost or refuse" helper
// used by every MP-spending skill's setup function. Returns true and
// deducts when the actor has enough MP; returns false and flashes
// "{label} needs more MP." otherwise. label is the human-readable
// skill name shown in the status (e.g. "Prayer", "Firebolt").
//
// Previously every setup function inlined the same three-line check
// + deduct; routing through here means a future "VIT also affects MP
// pool" or "MP refunds on cancel" change is one helper.
func chargeMP(g *core.GameState, skill core.SkillID, label string) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	cost := core.SkillCost(skill)
	if actor.MP < cost {
		setBattleStatus(g, label+" needs more MP.")
		return false
	}
	actor.MP -= cost
	return true
}

// ensureAliveTargetOrCancel is the apply-side counterpart of
// setupTargetedEnemy. The setup gate runs before the timing minigame, but a
// target can die between confirm and apply (e.g. a faster ally killed it on
// the same round). Apply handlers call this first: if the target's gone, the
// turn cancels cleanly with the same "No target." status that setup uses.
//
// `refundSkill` is the skill whose MP was committed in setup; pass
// core.SkillNone for basic Attack / Steal (no MP cost). When the target
// died between confirm and apply the action literally never happened, so
// the MP is refunded to keep the cost-payment contract honest — a wasted
// turn is enough penalty without also burning a cast.
//
// Returns true when the target is still alive and the apply can proceed.
func ensureAliveTargetOrCancel(g *core.GameState, refundSkill core.SkillID) bool {
	if core.BattleEnemyAlive(g, g.Battle.EnemyIndex) {
		return true
	}
	if refundSkill != core.SkillNone {
		if cost := core.SkillCost(refundSkill); cost > 0 {
			actor := &g.Party[g.Battle.CurrentParty]
			actor.MP += cost
			if actor.MP > actor.MaxMP {
				actor.MP = actor.MaxMP
			}
		}
	}
	setBattleStatus(g, "No target.")
	finishPartyAction(g)
	return false
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
	switch core.SkillMinigameFor(g.Battle.PendingSkill) {
	case core.MinigameCharge:
		g.Battle.Timing = core.NewChargeState(core.ChargeTimingDuration)
		// Charge gets a longer pre-arm pause so the player has time to read
		// the prompt; pressing the input during the intro skips straight
		// into the bar (handled in updateAttackTiming). ChargeNeedsRelease
		// blocks the very same Enter the player used to confirm the
		// target from being read as engaging the charge — they must
		// release once first, then a fresh press engages.
		intro = core.ChargeTimingIntro
		g.Battle.ChargeNeedsRelease = true
	case core.MinigameSequence:
		g.Battle.Timing = core.NewSequenceState(g.Rand(), core.StealTimingDuration, core.StealSequenceLength)
		// Clear analog-stick edge memory so a player whose stick happens to
		// be tilted when the bar arms doesn't get a phantom input on frame 1.
		input.ResetStickEdges()
	default:
		// Swipe gets a two-zone press bar (one hit zone per half of the
		// sweep) so its AoE swing reads as a wider commitment than a
		// single-target Attack. Every other press-minigame skill keeps the
		// classic one-zone bar.
		if g.Battle.PendingSkill == core.SkillSwipe {
			g.Battle.Timing = core.NewDoublePressState(g.Rand(), core.AttackTimingDuration)
		} else {
			g.Battle.Timing = core.NewTimingState(g.Rand(), core.AttackTimingDuration)
		}
	}
	g.Battle.TimingFlash = 0
	g.Battle.TimingIntro = intro
	g.Battle.Phase = core.BattleAttackTiming
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
		recordQuality(g, quality, g.Battle.CurrentParty, false)
	}
}

func actionHandlerFor(skill core.SkillID) (actionHandlers, bool) {
	if skill == core.SkillNone {
		return actionHandlers{setup: setupTargetedEnemy, apply: applyAttack}, true
	}
	handler, ok := skillActionHandlers[skill]
	return handler, ok
}

// recordQuality stamps the floating quality popup over the given party slot
// for QualityResultDuration. isBlock chooses the defend palette (and the
// "BLOCK!" label override) over the attack palette. Single source of truth
// for both attack-side and block-side quality popups so the field set never
// drifts between callers.
//
// NB: Miss-grade popups DO get stamped here (the timing still graded the
// player's input, even though no damage / no whiff message), and render
// reads them back via TimingQualityLabel which returns "Miss..." for that
// row. The popup over a whiffing actor saying "Miss..." is intentional —
// it acknowledges the player's mechanical performance even when the
// accuracy roll ate the swing.
func recordQuality(g *core.GameState, quality, partyIndex int, isBlock bool) {
	g.Battle.LastQuality = quality
	g.Battle.LastQualityTimer = core.QualityResultDuration
	g.Battle.LastQualityIndex = partyIndex
	g.Battle.LastQualityIsBlock = isBlock
}

// --- Basic Attack ---

func applyAttack(g *core.GameState, quality int) bool {
	// Basic attack has no MP cost; pass SkillNone so the refund branch is a no-op.
	if !ensureAliveTargetOrCancel(g, core.SkillNone) {
		return false
	}
	attacker := &g.Party[g.Battle.CurrentParty]
	// AttackBump fires unconditionally — the swing motion plays even on a
	// whiff, which is how a high-quality miss should read ("you swung well,
	// but the target moved"). If the whiff ever needs a different motion
	// (e.g. an over-the-shoulder pass-through), gate this below the
	// accuracy check.
	attacker.AttackBump = core.BumpDuration
	target := *core.BattleMemberAt(g, g.Battle.EnemyIndex)
	// Accuracy roll: basic attack only. DEX + timing quality drive the hit
	// chance; high-DEX classes and high grades push past 1.0 (clamped) so
	// they essentially never whiff. The swing animation still plays and
	// the timing popup still grades — the player's mechanical performance
	// is acknowledged — but no damage lands when the roll fails.
	if !core.AttackHits(g.Rand(), attacker.Stats, quality) {
		// Whiff log keeps the quality prefix so the line reads consistently
		// with hits ("Excellent! Warrior hits for 8." vs "Excellent! Warrior
		// swings wide."). The popup over the actor still says "Excellent!"
		// because the *timing* graded that way — accuracy is a separate roll
		// layered on top.
		setBattleMessage(g, fmt.Sprintf("%s%s swings wide.", qualityTag(quality), attacker.Name))
		finishPartyAction(g)
		return true
	}
	// Basic Attack: STR + 0, scaled by timing quality. Physically tagged
	// so the armor damp applies — basic attacks are the canonical phys
	// swing against an armored foe (amoeba teaches the lesson). The
	// dealt number returned by damageEnemy is the POST-armor figure;
	// the combat log uses it so what the player reads matches the HP
	// delta (an Excellent vs an Amoeba prints "hits for 4", not the
	// 12 we computed before armor clipped it down).
	rawDamage := core.ScaleDamage(core.MeleeDamage(attacker.Stats, 0), quality)
	dealt, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagPhys)
	setBattleMessage(g, attackResultMessage(attacker.Name, target, dealt, quality, defeated))
	finishPartyAction(g)
	return true
}

// --- Swipe (Warrior, hits all enemies in the battle group) ---

func setupSwipe(g *core.GameState) bool {
	return chargeMP(g, core.SkillSwipe, "Swipe")
}

func applySwipe(g *core.GameState, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	// Damage formula is dispatched by skill Kind in core.SkillDamage; Swipe's
	// Kind is Melee so this resolves to STR + Effect.Damage.
	damage := core.ScaleDamage(core.SkillDamage(actor.Stats, core.SkillSwipe), quality)
	hits := 0
	for slot, m := range core.BattleMembers(g) {
		if !m.Alive {
			continue
		}
		damageEnemy(g, slot, damage, quality, core.SkillTagFor(core.SkillSwipe))
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
	// Pre-cost target validation: a dead / unselected ally refuses the
	// cast WITHOUT spending MP, since the player is being asked to pick
	// a different target rather than burning their cast.
	if g.Battle.PartyTarget < 0 || g.Battle.PartyTarget >= len(g.Party) {
		setBattleStatus(g, "No ally selected.")
		return false
	}
	if g.Party[g.Battle.PartyTarget].HP <= 0 {
		setBattleStatus(g, "Prayer cannot revive.")
		return false
	}
	return chargeMP(g, core.SkillPrayer, "Prayer")
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

func applySteal(g *core.GameState, quality int) bool {
	// Steal costs 0 MP; pass the skill anyway so a future cost shows up.
	if !ensureAliveTargetOrCancel(g, core.SkillSteal) {
		return false
	}
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
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
	if g.Rand().Float64() < chance {
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
	if !core.BattleEnemyAlive(g, g.Battle.EnemyIndex) {
		setBattleStatus(g, "No target.")
		return false
	}
	// MP deduction policy: skill setup commits the MP cost here. The apply
	// step is normally guaranteed to run (Miss flashes still call apply
	// with quality=Miss), so there's no "back out" path for whiffs. The
	// one exception is target-death between confirm and apply — that path
	// refunds MP through ensureAliveTargetOrCancel, since the cast literally
	// never happened.
	return chargeMP(g, core.SkillFirebolt, "Firebolt")
}

func applyFirebolt(g *core.GameState, quality int) bool {
	// Firebolt's setup committed MP; refund it if the target died before apply.
	if !ensureAliveTargetOrCancel(g, core.SkillFirebolt) {
		return false
	}
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	effect := core.SkillEffectFor(core.SkillFirebolt)
	// Damage formula is dispatched by skill Kind in core.SkillDamage; Firebolt's
	// Kind is Magic so this resolves to INT + Effect.Damage. We still pull
	// Effect separately for the burn-chance roll below.
	rawDamage := core.ScaleDamage(core.SkillDamage(actor.Stats, core.SkillFirebolt), quality)
	target := *core.BattleMemberAt(g, g.Battle.EnemyIndex)
	// Firebolt is Magic-tagged so dealt == rawDamage in practice;
	// using the return keeps the log honest if a future Tag change
	// brings armor back into play.
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(core.SkillFirebolt))
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	burned := false
	if !defeated && enemy.BurnTurns <= 0 {
		burnChance := effect.BurnChance * float64(core.TimingBonusMult(quality))
		if burnChance > 1 {
			burnChance = 1
		}
		if g.Rand().Float64() < burnChance {
			enemy.BurnTurns = effect.BurnDuration(g.Rand())
			burned = true
		}
	}
	setBattleMessage(g, fireboltMessage(actor.Name, target, damage, quality, defeated, burned, enemy.BurnTurns))
	finishPartyAction(g)
	return true
}

// --- Damage / heal helpers (unchanged from previous behavior) ---

// damageEnemy applies `rawDamage` to the enemy at `slot`, clipped by
// the enemy's Armor when `tag == SkillTagPhys`. Magic / Heal / Buff
// tags bypass armor entirely. quality drives the floating damage popup
// color (set even on a killing blow so a dying enemy still shows the
// number that took it down). Returns (postArmorDamage, defeated): the
// damage number is what callers should put in combat-log messages so
// the log matches the HP delta (otherwise "Warrior hits Amoeba for 12"
// would read as "armor isn't doing anything" when the amoeba's HP only
// dropped by 4).
//
// quality is allowed to be TimingQualityMiss for non-action damage
// (burn ticks pass TimingQualityGood for the orange popup tint; tests
// can pass Miss when they don't care about the popup).
//
// Any damage > 0 wakes a sleeping enemy by zeroing SleepTurns — same
// "violence breaks the spell" rule as the party-side wake.
func damageEnemy(g *core.GameState, slot, rawDamage, quality int, tag core.SkillTag) (int, bool) {
	enemy := core.BattleMemberAt(g, slot)
	if enemy == nil || !enemy.Alive {
		return 0, false
	}
	damage := core.ApplyArmor(rawDamage, tag, enemy.Armor)
	// Clamp negative damage at 0 — every current caller (ScaleDamage,
	// burn ticks with BurnTickDamage>0) already produces non-negative
	// values, but enforcing the contract here keeps a future
	// caller from accidentally healing enemies by passing a signed
	// stat delta.
	if damage < 0 {
		damage = 0
	}
	enemy.DamageFlash = core.FlashDuration
	enemy.HP -= damage
	if damage > 0 {
		enemy.DamagePopup = damage
		enemy.DamagePopupQuality = quality
		enemy.DamagePopupTimer = core.QualityResultDuration
		// Any incoming damage shakes the enemy out of sleep — even an
		// armor-clamped 1, since the contract is "the hit landed."
		if enemy.SleepTurns > 0 {
			enemy.SleepTurns = 0
		}
	}
	if enemy.HP > 0 {
		// Audible "thud" only on hits that actually scored. Zero-damage
		// connections (e.g. Swipe with a 0 base) stay silent so the bar
		// doesn't tick a sound on every empty swing.
		if damage > 0 {
			audio.Play(audio.SoundEnemyHit)
		}
		return damage, false
	}
	enemy.HP = 0
	enemy.Alive = false
	enemy.BurnTurns = 0
	enemy.SleepTurns = 0
	enemy.DeathFade = core.DeathFadeDuration
	audio.Play(audio.SoundEnemyDeath)
	// If this enemy was holding party members ingested, release them now
	// so they re-enter the turn queue on the next round. Logged per-member
	// since freeing a prisoner is meaningful information for the player.
	for _, idx := range core.ReleaseIngestedBy(g.Party, slot) {
		setBattleMessage(g, fmt.Sprintf("%s tumbles free.", g.Party[idx].Name))
	}
	return damage, true
}

// tickPoisonAfterPartyTurn resolves the per-tick poison damage on a poisoned
// party member, fired AFTER their action resolves. Mirrors the burn helper
// but on the party side and with end-of-turn timing — the user still gets to
// act before they bleed. Returns true if the poison reduced the actor's HP
// to 0. Caller's downstream "lose battle" check picks up the kill, so this
// helper doesn't need to short-circuit.
func tickPoisonAfterPartyTurn(g *core.GameState, actor core.ActorRef) bool {
	if !actor.IsParty {
		return false
	}
	if actor.Index < 0 || actor.Index >= len(g.Party) {
		return false
	}
	member := &g.Party[actor.Index]
	if member.HP <= 0 || member.PoisonTurns <= 0 {
		return false
	}
	member.PoisonTurns--
	// damagePartyMember returns true on the fatal hit; use it as the
	// authoritative kill signal so a future "save at 1 HP" mechanic in
	// damagePartyMember can't desync from the message we emit here.
	// Poison is venomous decay — magical in source, so armor doesn't
	// damp it. Pass SkillTagMagic to bypass the armor clip.
	dealt, killed := damagePartyMember(g, actor.Index, core.PoisonTickDamage, core.SkillTagMagic)
	if killed {
		setBattleMessage(g, fmt.Sprintf("%s succumbs to the poison.", member.Name))
		return true
	}
	setBattleMessage(g, fmt.Sprintf("%s suffers %d from poison.", member.Name, dealt))
	return false
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
	enemy := core.BattleMemberAt(g, actor.Index)
	if enemy == nil || !enemy.Alive || enemy.BurnTurns <= 0 {
		return false
	}
	enemy.BurnTurns--
	// Quality=Good gives the orange "fire" tint to the popup that damageEnemy
	// stamps on a damaging hit.
	// Burn ticks are magical (the fire is the spell residue), so armor
	// doesn't damp them — same rule as the original Firebolt impact.
	damageEnemy(g, actor.Index, core.BurnTickDamage, core.TimingQualityGood, core.SkillTagMagic)
	def := core.EnemyInfoFor(*enemy)
	if !enemy.Alive {
		setBattleMessage(g, fmt.Sprintf("%s succumbs to the flames.", core.TheEnemy(def)))
		// Repoint the player's targeting cursor if the burn killed the
		// currently-selected enemy, so the next attack action has a valid one.
		if next := core.NextLivingBattleEnemy(g); next >= 0 && !core.BattleEnemyAlive(g, g.Battle.EnemyIndex) {
			g.Battle.EnemyIndex = next
		}
		return true
	}
	setBattleMessage(g, fmt.Sprintf("%s burns for %d.", core.TheEnemy(def), core.BurnTickDamage))
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
	// Sealed inside a mantrap — heal can't reach. Targeting cyclers
	// (AvailablePartyTargets) already prevent the player from picking
	// an ingested ally; this guards apply-time edge cases (item used
	// via a future macro, etc.).
	if member.Ingested {
		return false
	}
	member.HP += amount
	if member.HP > member.MaxHP {
		member.HP = member.MaxHP
	}
	member.DamageFlash = core.FlashDuration
	audio.Play(audio.SoundHeal)
	return true
}

// damagePartyMember applies `rawAmount` to a party member, armor-clipped
// when `tag == SkillTagPhys`. Magic / Heal / Buff and poison ticks
// bypass armor entirely (poison is the only current non-phys source on
// the party side). Any damage > 0 wakes the member from Sleep — same
// "violence breaks the spell" rule as the enemy side.
//
// Returns (dealtDamage, fatal): callers use the dealt number for the
// combat-log message so what the player reads matches the HP delta —
// once player-side equipment ships and members carry non-zero Armor,
// the log can't drift from the actual hit. partyIndex out-of-range or
// amount<=0 returns (0, false).
func damagePartyMember(g *core.GameState, partyIndex, rawAmount int, tag core.SkillTag) (int, bool) {
	if partyIndex < 0 || partyIndex >= len(g.Party) || rawAmount <= 0 {
		return 0, false
	}
	member := &g.Party[partyIndex]
	if member.HP <= 0 {
		return 0, false
	}
	// Ingested prey is sealed off — no damage reaches them while inside
	// the mantrap. Defense in depth: pickEnemyAttackTarget already routes
	// around ingested members, but any future damage source that picked
	// by index would otherwise bypass the lockout.
	if member.Ingested {
		return 0, false
	}
	amount := core.ApplyArmor(rawAmount, tag, member.Armor)
	member.DamageFlash = core.FlashDuration
	member.HP -= amount
	if amount > 0 && member.SleepTurns > 0 {
		member.SleepTurns = 0
	}
	if member.HP > 0 {
		return amount, false
	}
	member.HP = 0
	member.SleepTurns = 0
	return amount, true
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

// qualityTag returns the leading "Grade! " prefix prepended to battle-log
// messages on a hit ("Excellent! Warrior hits for 8."). Miss and Nice
// grades return "" — Miss is the whiff path which has its own "swings
// wide" copy and shouldn't double up with a grade prefix, and Nice is the
// "barely landed" grade where the unprefixed message reads as the baseline
// outcome. This filter is *only* for log text; the floating popup over the
// actor still shows the full label via TimingQualityLabel (see recordQuality).
func qualityTag(quality int) string {
	if quality == core.TimingQualityMiss || quality == core.TimingQualityNice {
		return ""
	}
	return core.TimingQualityLabel(quality) + " "
}

// resolveEnemyAttacker applies a single enemy's attack against a chosen party
// member, scaled by the player's defend quality. Used by the BattleEnemyTiming
// phase. Returns true if the hit landed (false if attacker was already dead).
func resolveEnemyAttacker(g *core.GameState, slot int, defendQuality int) bool {
	enemy := core.BattleMemberAt(g, slot)
	if enemy == nil || !enemy.Alive {
		return false
	}
	target := pickEnemyAttackTarget(g)
	if target < 0 {
		return false
	}
	enemy.AttackBump = core.BumpDuration
	rawDamage := core.EnemyInfoFor(*enemy).AttackDamage
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
	// Plain enemy melee is physically tagged so the party's Armor field
	// (currently 0 for all members, future equipment) damps the bite.
	// Spell-casting enemies (goblin mage) dispatch through their own
	// resolver and pass SkillTagMagic where appropriate. dealt is the
	// post-armor figure used in the message so once equipment lands
	// the log will reflect "Warrior soaks the goblin for X" with the
	// real number.
	dealt, _ := damagePartyMember(g, target, damage, core.SkillTagPhys)
	if defendQuality > core.TimingQualityMiss {
		// A successful block recoils the defender slightly so the impact reads
		// even though the damage number is small.
		g.Party[target].AttackBump = core.BlockBumpDuration
	}
	recordQuality(g, defendQuality, target, true)
	def := core.EnemyInfoFor(*enemy)
	setBattleMessage(g, enemyHitMessage(*enemy, g.Party[target].Name, dealt, defendQuality, g.Party[target].Defending))
	// Poison inflict: only on damaging hits from a poison-themed attacker
	// against a target that's still alive and not already poisoned. The
	// no-stack rule mirrors burn — re-poisoning a poisoned target on every
	// bite would trivialize the duration roll.
	if damage > 0 && def.PoisonChance > 0 && g.Party[target].HP > 0 && g.Party[target].PoisonTurns <= 0 {
		if g.Rand().Float64() < def.PoisonChance {
			g.Party[target].PoisonTurns = core.DefaultPoisonEffect.RollDuration(g.Rand())
			setBattleMessage(g, fmt.Sprintf("%s is poisoned!", g.Party[target].Name))
		}
	}
	return true
}

// pickEnemyAttackTarget cycles to the next living party member after the last
// one the enemy side targeted. Uses EnemyAttackCursor (separate from
// PartyTarget) so the player's heal/item ally cycling doesn't shift who
// enemies attack next.
func pickEnemyAttackTarget(g *core.GameState) int {
	target := core.WrapNextAvailablePartyMember(g.Party, g.Battle.EnemyAttackCursor+1)
	if target >= 0 {
		g.Battle.EnemyAttackCursor = target
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
	return fmt.Sprintf("%s %s %s for %d.", core.TheEnemy(def), def.AttackVerbSingular, targetName, damage)
}
