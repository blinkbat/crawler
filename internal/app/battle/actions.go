package battle

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"fmt"
	"math/rand"
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
	core.SkillSwipe:        {setup: setupSwipe, apply: applySwipe},
	core.SkillPrayer:       {setup: setupPrayer, apply: applyPrayer},
	core.SkillSteal:        {setup: setupTargetedEnemy, apply: applySteal},
	core.SkillFirebolt:     {setup: setupFirebolt, apply: applyFirebolt},
	core.SkillCrushingBlow: {setup: setupCrushingBlow, apply: applyCrushingBlow},
	core.SkillWhirlwind:    {setup: setupWhirlwind, apply: applyWhirlwind},
	core.SkillMassMend:     {setup: setupMassMend, apply: applyMassMend},
	core.SkillSmite:        {setup: setupSmite, apply: applySmite},
	core.SkillBackstab:     {setup: setupBackstab, apply: applyBackstab},
	core.SkillVenomStrike:  {setup: setupVenomStrike, apply: applyVenomStrike},
	core.SkillFrostLance:   {setup: setupFrostLance, apply: applyFrostLance},
	core.SkillArcBolt:      {setup: setupArcBolt, apply: applyArcBolt},
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
		// Each class learns SkillsPerClass skills; every slot must be
		// player-castable AND have a handler. Looping the array means
		// adding a new universal skill (or per-class slot) only needs
		// the registry rows — this assert picks it up automatically.
		for _, s := range def.Skills {
			if s == core.SkillNone {
				continue
			}
			if !core.SkillPlayerCastable(s) {
				panic("battle: class " + def.Name + " skill " + core.SkillName(s) + " is not PlayerCastable — flip the flag in core/party.go skillDefinitions")
			}
			if _, ok := skillActionHandlers[s]; !ok {
				panic("battle: class " + def.Name + " skill " + core.SkillName(s) + " has no skillActionHandlers entry — register a setup/apply pair")
			}
		}
	}
	for _, s := range core.PlayerCastableSkills() {
		if _, ok := skillActionHandlers[s]; !ok {
			panic("battle: PlayerCastable skill " + core.SkillName(s) + " has no skillActionHandlers entry")
		}
	}
	// Enemy-castable consistency: every skill flagged EnemyCastable in
	// the registry must appear in enemySpellHandlers (the actual
	// dispatch table). This catches both directions of drift at
	// startup: a new EnemyCastable skill with no handler panics, and
	// a stale handler whose registry flag got removed shows up in the
	// reverse walk below.
	for _, s := range core.EnemyCastableSkills() {
		if _, ok := enemySpellHandlers[s]; !ok {
			panic("battle: EnemyCastable skill " + core.SkillName(s) + " has no enemySpellHandlers entry — register a handler in battle.go")
		}
	}
	for s := range enemySpellHandlers {
		if !core.IsEnemyCastable(s) {
			panic("battle: enemySpellHandlers has a handler for " + core.SkillName(s) + " but its registry entry isn't EnemyCastable — clear the handler or flip the flag")
		}
	}
}

// enemySpellCtx bundles the pre-resolved state every enemy-spell
// handler needs: the caster's battle slot, the picked party target,
// the enemy's definition (for damage formulas, log text), the
// skill's effect block (for damage / sleep duration / etc.), and the
// localized skill name. Built once by resolveEnemySpell so each
// handler doesn't re-derive the same lookups.
type enemySpellCtx struct {
	g         *core.GameState
	slot      int
	target    int
	enemy     *core.Enemy
	def       core.EnemyDefinition
	skillName string
	effect    core.SkillEffect
}

// enemySpellHandlers is the dispatch table resolveEnemySpell walks.
// Every entry's key must have EnemyCastable=true in the skill
// registry; the init guard above asserts both directions of the
// invariant at startup so a stale handler or a missing one is a
// panic at process start, not a silent fizzle mid-encounter.
var enemySpellHandlers = map[core.SkillID]func(enemySpellCtx){
	core.SkillFirebolt:   handleEnemyFirebolt,
	core.SkillIngest:     handleEnemyIngest,
	core.SkillSleep:      handleEnemySleep,
	core.SkillWeb:        handleEnemyWeb,
	core.SkillConfuse:    handleEnemyConfuse,
	core.SkillStoneslam:  handleEnemyStoneslam,
	core.SkillRaiseBones: handleEnemyRaiseBones,
}

// enemySpellLog formats the canonical "<Enemy> casts <Skill> — <rest>"
// combat-log line for every enemy spell handler that follows the
// casts-prefix convention. `rest` is the tail format (verb + targets +
// numbers); enemy + skill name are auto-prefixed from ctx. Routes
// through setBattleMessage so a future color / channel routing change
// lands once.
//
// Handlers with bespoke phrasings (Ingest's "lunges/engulfs", Web's
// "spins a fresh web at", Stoneslam's "slams the ground") still call
// setBattleMessage directly — this helper is for the prefixed format
// only.
func enemySpellLog(ctx enemySpellCtx, rest string, args ...any) {
	tail := fmt.Sprintf(rest, args...)
	setBattleMessage(ctx.g, fmt.Sprintf("%s casts %s — %s", core.TheEnemy(ctx.def), ctx.skillName, tail))
}

// enemySpellDamage is the shared damage formula for every damaging
// enemy spell: SpellPower (per-kind magic stat) + the skill effect's
// Damage base, clipped to a minimum of 1 so a poorly-stat'd caster
// can't deal 0 or negative damage. Used by Firebolt (single-target
// magic) and Stoneslam (AoE phys); future damage spells slot in here
// instead of re-typing the floor-1 clamp.
func enemySpellDamage(def core.EnemyDefinition, effect core.SkillEffect) int {
	raw := def.SpellPower + effect.Damage
	if raw < 1 {
		raw = 1
	}
	return raw
}

// handleEnemyFirebolt applies the goblin-mage style ranged magic
// damage cast. Damage = SpellPower (per-kind magic stat) + the
// skill's Effect.Damage base — SpellPower defaults to 0 so a
// non-caster enemy that somehow rolled into this branch can't deal
// huge damage by accident.
func handleEnemyFirebolt(ctx enemySpellCtx) {
	g := ctx.g
	dealt, killed := damagePartyMember(g, ctx.target, enemySpellDamage(ctx.def, ctx.effect), core.SkillTagMagic)
	core.EnqueuePartyVFX(g, core.VFXEmber, ctx.target)
	if killed {
		setBattleMessage(g, fmt.Sprintf("%s incinerates %s.", core.TheEnemy(ctx.def), g.Party[ctx.target].Name))
	} else {
		enemySpellLog(ctx, "%s burns for %d.", g.Party[ctx.target].Name, dealt)
	}
	audio.Play(audio.SoundInputGreat)
}

// handleEnemyIngest is the mantrap signature: pulls the target out of
// combat until the mantrap dies (or until ingested-by-dead-mantrap
// cleanup releases them). Sleep + Defending are cleared because the
// swallow is violent enough to wake / unbrace; Poison persists so
// ingest isn't a free status-effect escape.
func handleEnemyIngest(ctx enemySpellCtx) {
	g := ctx.g
	// Defensive re-check: enemyAIPickSkill won't route here without an
	// available target, but the world can shift between turns (e.g. a
	// fast ally killed the only viable target). Cancel cleanly with a
	// log line so the combat log doesn't go silent on the cast.
	picked := ctx.target
	if !core.PartyMemberAvailable(g.Party, picked) {
		picked = core.FirstAvailablePartyMember(g.Party)
	}
	if picked < 0 {
		setBattleMessage(g, fmt.Sprintf("%s lunges, but finds no one to seize.", core.TheEnemy(ctx.def)))
		return
	}
	m := &g.Party[picked]
	// Bound targets refuse Ingest — the design contract for Bound is
	// "tempo control without removal," so the spider's web should
	// shield the prey from the mantrap. The mantrap just bites
	// instead this turn (caller falls back to plain melee on the
	// next round when usableEnemySkills sees Ingest still pending).
	if m.BoundTurns > 0 {
		setBattleMessage(g, fmt.Sprintf("%s lunges, but %s is too tangled to swallow.", core.TheEnemy(ctx.def), m.Name))
		return
	}
	m.Ingested = true
	m.IngestedBy = ctx.slot
	m.SleepTurns = 0
	m.Defending = false
	// VFX anchors at the MANTRAP (ctx.slot), not the prey (picked).
	// spawnIngest's pattern converges motes inward toward origin —
	// anchoring at the mantrap reads visually as "prey's essence
	// flowing INTO the swallower." Anchoring at the prey reverses
	// the meaning (motes converge on the prey, which reads as
	// "something attacking the prey" not "being absorbed").
	core.EnqueueEnemyVFX(g, core.VFXIngest, ctx.slot)
	setBattleMessage(g, fmt.Sprintf("%s engulfs %s!", core.TheEnemy(ctx.def), m.Name))
	audio.Play(audio.SoundEnemyHit)
}

// handleEnemySleep applies the goblin-mage Sleep cast. Already-asleep
// targets short-circuit with a flavor line; otherwise the duration
// rolls from the skill's effect block and lands on the target.
func handleEnemySleep(ctx enemySpellCtx) {
	g := ctx.g
	m := &g.Party[ctx.target]
	// Defense-in-depth: pickEnemyAttackTarget only returns living
	// members today, but a future code path that lets a corpse
	// through would silently land sleep on a dead body.
	if m.HP <= 0 {
		return
	}
	if m.SleepTurns > 0 {
		enemySpellLog(ctx, "%s is already asleep.", m.Name)
		return
	}
	duration := ctx.effect.SleepDuration(g.Rand())
	if duration <= 0 {
		duration = core.SleepMinTurns
	}
	m.SleepTurns = core.ShortenStatusDuration(duration, core.EffectiveStats(*m).WIS)
	core.EnqueuePartyVFX(g, core.VFXSleep, ctx.target)
	enemySpellLog(ctx, "%s falls asleep.", m.Name)
	audio.Play(audio.SoundInputHit)
}

// handleEnemyWeb applies the Cave Spider's Bound status. Already-bound
// targets short-circuit with a flavor line (no stacking); otherwise
// the duration rolls from BindMin/Max and lands on the target. The
// target is guaranteed alive by pickEnemyAttackTarget's living-filter
// upstream — no HP<=0 guard needed here.
func handleEnemyWeb(ctx enemySpellCtx) {
	g := ctx.g
	m := &g.Party[ctx.target]
	if m.BoundTurns > 0 {
		setBattleMessage(g, fmt.Sprintf("%s spins a fresh web at %s — already bound.", core.TheEnemy(ctx.def), m.Name))
		return
	}
	duration := ctx.effect.BindDuration(g.Rand())
	if duration <= 0 {
		duration = core.SpiderWebBoundMinTurns
	}
	m.BoundTurns = core.ShortenStatusDuration(duration, core.EffectiveStats(*m).WIS)
	core.EnqueuePartyVFX(g, core.VFXWeb, ctx.target)
	enemySpellLog(ctx, "%s is bound in sticky webs.", m.Name)
	audio.Play(audio.SoundInputHit)
}

// handleEnemyConfuse applies the Will-o'-Wisp's Confused status.
// WIS resistance lives in the universal ShortenStatusDuration path
// (mirrors Sleep / Bound / Poison applies on the party side) — high
// WIS cuts the duration; no separate per-cast resist roll.
// Already-confused targets short-circuit (no stacking). Duration
// rolls from ConfuseMin/Max. Target is living by upstream filter —
// no HP<=0 guard.
func handleEnemyConfuse(ctx enemySpellCtx) {
	g := ctx.g
	m := &g.Party[ctx.target]
	if m.ConfusedTurns > 0 {
		setBattleMessage(g, fmt.Sprintf("%s flickers at %s — already disoriented.", core.TheEnemy(ctx.def), m.Name))
		return
	}
	rng := g.Rand()
	duration := ctx.effect.ConfuseDuration(rng)
	if duration <= 0 {
		duration = core.WispConfuseMinTurns
	}
	m.ConfusedTurns = core.ShortenStatusDuration(duration, core.EffectiveStats(*m).WIS)
	core.EnqueuePartyVFX(g, core.VFXConfuse, ctx.target)
	enemySpellLog(ctx, "%s grows confused.", m.Name)
	audio.Play(audio.SoundInputHit)
}

// handleEnemyStoneslam fires the Stone Golem's AoE phys cast. Hits
// every living party member (skipping ingested ones — they're
// untargetable while inside their swallower) with damage = SpellPower
// + Effect.Damage, tagged Phys so per-target Armor / Defending
// applies. No status component — the slam is pure damage.
func handleEnemyStoneslam(ctx enemySpellCtx) {
	g := ctx.g
	raw := enemySpellDamage(ctx.def, ctx.effect)
	hits := 0
	kills := 0
	for i := range g.Party {
		m := &g.Party[i]
		if m.HP <= 0 || m.Ingested {
			continue
		}
		_, killed := damagePartyMember(g, i, raw, core.SkillTagPhys)
		core.EnqueuePartyVFX(g, core.VFXStoneslam, i)
		hits++
		if killed {
			kills++
		}
	}
	switch {
	case hits == 0:
		setBattleMessage(g, fmt.Sprintf("%s raises stone fists, but finds no targets.", core.TheEnemy(ctx.def)))
	case kills > 0:
		setBattleMessage(g, fmt.Sprintf("%s slams the ground — %d crushed.", core.TheEnemy(ctx.def), kills))
	default:
		setBattleMessage(g, fmt.Sprintf("%s slams the ground — the whole party staggers.", core.TheEnemy(ctx.def)))
	}
	audio.Play(audio.SoundEnemyHit)
}

// handleEnemyRaiseBones is the Necromancer's signature add-summon.
// Inserts one Skeleton Enemy into the active pack and queues an
// initiative slot so the new fighter takes a turn this round if its
// SPD slot hasn't passed yet. The per-battle cast limit is enforced
// by usableEnemySkills (drops the skill from the pick list once
// SkillCastCount[SkillRaiseBones] hits PerBattleCastLimit) so by the
// time we get here, a cast is legal.
func handleEnemyRaiseBones(ctx enemySpellCtx) {
	g := ctx.g
	if g.Battle.ActivePack < 0 || g.Battle.ActivePack >= len(g.Packs) {
		setBattleMessage(g, fmt.Sprintf("%s gestures, but the bones refuse to rise.", core.TheEnemy(ctx.def)))
		return
	}
	pack := &g.Packs[g.Battle.ActivePack]
	skeleton := core.NewEnemy(core.EnemySkeleton)
	pack.Members = append(pack.Members, skeleton)
	// The skeleton enters the turn queue automatically: beginNewRound
	// rebuilds NextRoundQueue from scratch via buildTurnQueue, which
	// walks the (now-expanded) pack.Members list. So the skeleton
	// acts starting the round AFTER this cast — no manual queue
	// insertion needed (an earlier pass appended to NextRoundQueue
	// here, but the rebuild discarded the append, making the line
	// dead code).
	//
	// Re-fetch the caster AFTER the append. pack.Members is allocated at
	// exact capacity (placePacks: make([]Enemy, 0, len)), so appending a
	// skeleton always reallocates the backing array — ctx.enemy now
	// dangles at the old array. Stamping the cast counter through ctx.enemy
	// would land on the orphaned copy, leaving the live necromancer's
	// SkillCastCount nil so usableEnemySkills never sees the cap and it
	// summons every eligible turn. BattleMemberAt re-derives the live
	// pointer by slot. (NewEnemy leaves SkillCastCount nil by design —
	// nil-map reads return zero — so the guard below still allocates it.)
	caster := core.BattleMemberAt(g, ctx.slot)
	if caster != nil {
		if caster.SkillCastCount == nil {
			caster.SkillCastCount = map[core.SkillID]int{}
		}
		caster.SkillCastCount[core.SkillRaiseBones]++
	}
	setBattleMessage(g, fmt.Sprintf("%s incants — a skeleton claws up from the ground!", core.TheEnemy(ctx.def)))
	audio.Play(audio.SoundInputHit)
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

// canAffordSkill reports whether the given party member has enough MP
// to cast `skill` — pure predicate, no state mutation. Used by the
// skill-picker menu to preview-gate a selection before the player
// confirms (where chargeMP would deduct). Sharing the MP-cost rule
// with chargeMP via this helper means a future "VIT raises MP cap"
// or "potion grants free cast" change lands in one place rather
// than the two places that previously inlined `actor.MP < cost`.
func canAffordSkill(actor core.PartyMember, skill core.SkillID) bool {
	return actor.MP >= core.SkillCost(skill)
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

// applyAoEDamage hits every living enemy in the active pack with the
// given damage amount, routing through damageEnemy so the skill's
// SkillTag-driven armor rules apply. Returns the hit count for the
// log message — three apply handlers (Swipe / Whirlwind / Arc Bolt)
// used to inline the same `for slot, m := range core.BattleMembers
// { if !m.Alive continue; damageEnemy }` loop.
func applyAoEDamage(g *core.GameState, skill core.SkillID, damage, quality int) int {
	hits := 0
	tag := core.SkillTagFor(skill)
	vfx := aoeVFXFor(skill)
	for slot, m := range core.BattleMembers(g) {
		if !m.Alive {
			continue
		}
		damageEnemy(g, slot, damage, quality, tag)
		core.EnqueueEnemyVFX(g, vfx, slot)
		hits++
	}
	return hits
}

// aoeVFXFor picks the per-skill VFX kind for an AoE damage skill.
// Centralised so the table-style mapping lives in one place — adding
// a new AoE skill is one row here, not a new switch inline at each
// apply* site.
func aoeVFXFor(skill core.SkillID) core.VFXKind {
	switch skill {
	case core.SkillSwipe, core.SkillWhirlwind:
		return core.VFXSlash
	case core.SkillArcBolt:
		return core.VFXArc
	}
	return core.VFXNone
}

// tryProcStatus is the shared "roll a quality-scaled status proc and
// stamp the counter on success" gate every status-inflict skill used
// to inline. Four apply handlers (Firebolt → BurnTurns, CrushingBlow
// + FrostLance → StunTurns, VenomStrike → PoisonTurns) repeated the
// same structure: skip if already procced, optional minimum-grade
// gate, scale the base chance by TimingBonusMult, clamp to 1, roll.
//
// Caller passes the pre-computed defeated flag so a kill-shot can't
// silently inflict a status on a corpse. minGrade = 0 means "any
// quality can proc" (Firebolt / Venom Strike); >0 gates the proc on
// Great / Excellent (Crushing Blow / Frost Lance). durationFn is the
// SkillEffect.{X}Duration roller — pre-bound by the caller so the
// helper doesn't need a switch on which counter to roll. resistWis
// is the target's WIS, which shortens the rolled duration via
// core.ShortenStatusDuration (mirrors the party-side enemy → party
// status apply path).
//
// Returns true when the counter was just stamped — callers use it to
// pick the "you stunned/burned/poisoned them" copy in their log line.
func tryProcStatus(rng *rand.Rand, counter *int, defeated bool, baseChance float64, quality, minGrade int, durationFn func(*rand.Rand) int, resistWis int) bool {
	if defeated || baseChance <= 0 || counter == nil || *counter > 0 {
		return false
	}
	if minGrade > 0 && quality < minGrade {
		return false
	}
	chance := baseChance * float64(core.TimingBonusMult(quality))
	if chance > 1 {
		chance = 1
	}
	if rng.Float64() >= chance {
		return false
	}
	*counter = core.ShortenStatusDuration(durationFn(rng), resistWis)
	return *counter > 0
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

// beginSingleTargetSkill is the shared head of every single-enemy-target
// damaging skill (Firebolt / CrushingBlow / Smite / Backstab / VenomStrike
// / FrostLance): the refund-on-dead-target gate, actor lookup, attack-bump,
// raw-damage roll, and a pre-hit snapshot of the target (the message
// builders want the foe's name/state from BEFORE it possibly dies). Returns
// ok=false when the cast was cancelled — ensureAliveTargetOrCancel already
// refunded MP and ended the turn, so callers just `return false`. The
// returned rawDamage is the pre-armor figure; callers may still mutate it
// (crit doublers) before handing it to damageEnemy.
func beginSingleTargetSkill(g *core.GameState, skill core.SkillID, quality int) (actor *core.PartyMember, target core.Enemy, rawDamage int, ok bool) {
	if !ensureAliveTargetOrCancel(g, skill) {
		return nil, core.Enemy{}, 0, false
	}
	actor = &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	rawDamage = scaleSkillDamage(actor, skill, quality)
	target = *core.BattleMemberAt(g, g.Battle.EnemyIndex)
	return actor, target, rawDamage, true
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
	// Confused retarget: if the acting party member is afflicted with
	// the Will-o'-Wisp's Confused status, roll
	// WispConfuseRetargetRoll. On a success, scramble the target —
	// enemy-target actions pick a random LIVING enemy (could be the
	// "wrong" one), party-target actions pick a random LIVING party
	// slot including the actor. The retarget happens BEFORE the
	// timing bar arms so the player sees what their character is
	// actually about to do (the camera/cursor swing sells the
	// "wait, that's not who I picked" beat). Out-of-band (e.g.
	// SkillNone with no target) skips automatically because the
	// switch only handles ActionMode values that have targets.
	maybeConfuseRetarget(g)
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
			// Swipe is the canonical two-hit tally bar — both window
			// hits land twice across the AoE formation. Other multi-
			// hit skills, when they ship, swap `2` for their target
			// hit count.
			g.Battle.Timing = core.NewMultiPressState(g.Rand(), core.AttackTimingDuration, 2)
		} else {
			g.Battle.Timing = core.NewTimingState(g.Rand(), core.AttackTimingDuration)
		}
	}
	g.Battle.TimingFlash = 0
	g.Battle.TimingIntro = intro
	g.Battle.Phase = core.BattleAttackTiming
}

// maybeConfuseRetarget scrambles g.Battle.EnemyIndex /
// g.Battle.PartyTarget when the acting party member is afflicted with
// the Will-o'-Wisp's Confused status AND a per-action retarget roll
// (WispConfuseRetargetRoll) succeeds. Retarget stays within the
// action mode — an enemy-target action picks a random LIVING enemy
// (could be the originally-picked one, that's fine; the chaos comes
// from the chance that it ISN'T), a party-target action picks any
// living party slot including the actor (so a Cleric mid-Confuse
// might Prayer themselves instead of the bleeding warrior).
//
// SkillSteal's TargetMode is ActionEnemyTarget so its retarget is
// covered by the enemy branch — the Thief might pickpocket the
// wrong enemy under Confused, which is the right flavor for the
// "Steal pickpockets a friend" line in the wisp's design note (the
// codebase doesn't currently have a friend-side steal target, but
// the disorientation reads correctly).
func maybeConfuseRetarget(g *core.GameState) {
	actor := g.Battle.CurrentParty
	if actor < 0 || actor >= len(g.Party) {
		return
	}
	if g.Party[actor].ConfusedTurns <= 0 {
		return
	}
	rng := g.Rand()
	if rng.Float64() >= core.WispConfuseRetargetRoll {
		return
	}
	switch g.Battle.ActionMode {
	case core.ActionEnemyTarget:
		slots := core.LivingBattleEnemyIndices(g)
		if len(slots) == 0 {
			return
		}
		picked := slots[rng.Intn(len(slots))]
		if picked != g.Battle.EnemyIndex {
			g.Battle.EnemyIndex = picked
			setBattleStatus(g, fmt.Sprintf("%s is confused — wrong target!", g.Party[actor].Name))
		}
	case core.ActionPartyTarget:
		slots := core.AvailablePartyTargets(g.Party)
		if len(slots) == 0 {
			return
		}
		picked := slots[rng.Intn(len(slots))]
		if picked != g.Battle.PartyTarget {
			g.Battle.PartyTarget = picked
			setBattleStatus(g, fmt.Sprintf("%s is confused — wrong ally!", g.Party[actor].Name))
		}
	}
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
	if !core.AttackHits(g.Rand(), core.EffectiveStats(*attacker), quality) {
		// Whiff log keeps the quality prefix so the line reads consistently
		// with hits ("Excellent! Warrior hits for 8." vs "Excellent! Warrior
		// swings wide."). The popup over the actor still says "Excellent!"
		// because the *timing* graded that way — accuracy is a separate roll
		// layered on top.
		setBattleMessage(g, fmt.Sprintf("%s%s swings wide.", qualityTag(quality), attacker.Name))
		finishPartyAction(g)
		return true
	}
	// Defender dodge: a connecting swing can still be sidestepped by a
	// nimble enemy. Symmetric with the party-side dodge in
	// resolveEnemyAttacker. Skills are NOT dodgeable (mirrors
	// AttackAccuracy's basic-attack-only gate).
	if core.RollDodge(g.Rand(), core.EnemyInfoFor(target).Stats) {
		setBattleMessage(g, fmt.Sprintf("%s%s lunges but the %s slips aside.", qualityTag(quality), attacker.Name, core.EnemySingularNoun(target)))
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
	rawDamage := core.ScaleDamage(core.MeleeDamage(core.EffectiveStats(*attacker), 0), quality)
	crit, _ := rollSkillCrit(g, attacker, core.SkillNone, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	dealt, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagPhys)
	core.EnqueueEnemyVFX(g, core.VFXSlash, g.Battle.EnemyIndex)
	setBattleMessage(g, appendCrit(attackResultMessage(attacker.Name, target, dealt, quality, defeated), crit))
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
	// Kind is Melee so this resolves to STR + Effect.Damage. Tally mode
	// multiplies the per-pass damage by the number of tallied hits —
	// each window the player landed inside is one extra sweep across
	// the formation. Single-press fallback (Quality > Miss but tally
	// mode never armed) still does one pass.
	damage := scaleSkillDamage(actor, core.SkillSwipe, quality)
	// Single crit roll for the whole Swipe — a lucky swing doubles the
	// damage on every pass that lands. Per-pass rolls would feel noisier
	// in the log without offering more decision depth, since the player
	// can't react between sweeps.
	crit, _ := rollSkillCrit(g, actor, core.SkillSwipe, quality)
	damage = applyCritMultiplier(damage, crit, false)
	passes := multiPressPasses(g.Battle.Timing, quality)
	// `enemiesHit` is the COUNT OF DISTINCT FOES STRUCK — captured
	// from the first pass only. Later passes may hit fewer enemies
	// because earlier passes killed some, but the player struck the
	// full set at least once; the log line should reflect that.
	// Earlier code captured the LAST pass's count, which undercounted
	// the swing whenever the first sweep got a kill.
	enemiesHit := 0
	for p := 0; p < passes; p++ {
		hit := applyAoEDamage(g, core.SkillSwipe, damage, quality)
		if p == 0 {
			enemiesHit = hit
		}
	}
	if enemiesHit == 0 || passes == 0 {
		setBattleMessage(g, aoeEmptyMessage("Swipe", "catches only air"))
	} else {
		setBattleMessage(g, appendCrit(swipeMessage(actor.Name, enemiesHit, quality), crit))
	}
	finishPartyAction(g)
	// Even if hits=0, the attack motion played and MP was spent — landed.
	return true
}

// multiPressPasses returns the number of damage passes a tally-mode
// skill should make: one per hit tallied during the press bar. Non-
// tally bars fall back to 1 pass on any non-Miss grade (the original
// single-press behaviour), 0 on Miss. Centralised so future multi-
// hit skills (Whirlwind variants, Backstab combos) read the same
// rule instead of duplicating the timing-state inspection.
func multiPressPasses(t core.TimingState, quality int) int {
	if t.IsTallyMode() {
		return t.Hits
	}
	if quality == core.TimingQualityMiss {
		return 0
	}
	return 1
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
	heal := core.ScaleHeal(core.SkillHealFor(actor, core.SkillPrayer), quality)
	target := &g.Party[g.Battle.PartyTarget]
	healPartyMember(g, g.Battle.PartyTarget, heal)
	core.EnqueuePartyVFX(g, core.VFXHeal, g.Battle.PartyTarget)
	// Prayer T3 cleanses Poison + Sleep on the target. The cleanses
	// happen AFTER the heal so the player sees the heal pop even if
	// the status would have otherwise ticked the same turn.
	mod := core.SkillTierMod(actor, core.SkillPrayer)
	cleansed := applyStatusCleanses(target, mod)
	selfTarget := g.Battle.PartyTarget == g.Battle.CurrentParty
	msg := prayerMessage(actor.Name, target.Name, heal, quality, selfTarget)
	if cleansed != "" {
		msg = fmt.Sprintf("%s %s", msg, cleansed)
	}
	setBattleMessage(g, msg)
	finishPartyAction(g)
	return true
}

// applyStatusCleanses clears Poison/Sleep on the given party member
// per the tier-effective mod bits and returns a human-readable
// suffix describing what was cleared (empty string if nothing
// happened). Single seam so a future cleanse-skill (Cleric universal
// Purify, paladin Cure) wires in via the same flag-and-suffix path
// instead of inlining the same status-zero loop.
func applyStatusCleanses(m *core.PartyMember, mod core.SkillEffectDelta) string {
	cleared := []string{}
	if mod.CleansesPoison && m.PoisonTurns > 0 {
		m.PoisonTurns = 0
		cleared = append(cleared, "Poison")
	}
	if mod.CleansesSleep && m.SleepTurns > 0 {
		m.SleepTurns = 0
		cleared = append(cleared, "Sleep")
	}
	switch len(cleared) {
	case 0:
		return ""
	case 1:
		return "(Cleansed " + cleared[0] + ".)"
	default:
		return "(Cleansed " + cleared[0] + " + " + cleared[1] + ".)"
	}
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
	effect := core.EffectiveSkillEffect(actor, core.SkillSteal)
	// Steal chance: base × (1 + DEX/20), then quality multiplier on top.
	// Capped at 1.0 so a perfect-Excellent high-DEX thief still rolls.
	chance := core.StealChance(core.EffectiveStats(*actor), effect.StealChance) * float64(core.TimingBonusMult(quality))
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
		// Steal T3 ("Cuts on lift") deals STR damage on a landed
		// steal. The multiplier-style StealBonusDamage (current
		// table value 1) scales STR linearly so future tunes ("T4
		// adds 2×STR") plug in as a numeric bump.
		mod := core.SkillTierMod(actor, core.SkillSteal)
		var bonus int
		var defeated bool
		var critBonus bool
		if mod.StealBonusDamage > 0 {
			rawBonus := core.EffectiveStats(*actor).STR * mod.StealBonusDamage
			critBonus, _ = rollSkillCrit(g, actor, core.SkillSteal, quality)
			rawBonus = applyCritMultiplier(rawBonus, critBonus, false)
			bonus, defeated = damageEnemy(g, g.Battle.EnemyIndex, rawBonus, quality, core.SkillTagPhys)
		}
		core.EnqueueEnemyVFX(g, core.VFXSteal, g.Battle.EnemyIndex)
		msg := stealMessage(actor.Name, item, quality)
		switch {
		case defeated:
			msg = fmt.Sprintf("%s The cut fells the %s.", msg, core.EnemySingularNoun(*enemy))
		case bonus > 0:
			msg = fmt.Sprintf("%s The cut bleeds for %d.", msg, bonus)
		}
		setBattleMessage(g, appendCrit(msg, critBonus))
	} else {
		setBattleMessage(g, fmt.Sprintf("%s fails to steal.", actor.Name))
	}
	finishPartyAction(g)
	return true
}

// --- Firebolt (Wizard, ramps damage and burn chance with quality) ---

func setupFirebolt(g *core.GameState) bool {
	// MP deduction policy: skill setup commits the MP cost here. The apply
	// step is normally guaranteed to run (Miss flashes still call apply
	// with quality=Miss), so there's no "back out" path for whiffs. The
	// one exception is target-death between confirm and apply — that path
	// refunds MP through ensureAliveTargetOrCancel, since the cast literally
	// never happened.
	return setupTargetedEnemyAndPay(g, core.SkillFirebolt, "Firebolt")
}

func applyFirebolt(g *core.GameState, quality int) bool {
	// Firebolt's setup committed MP; the shared head refunds it if the
	// target died before apply.
	actor, target, rawDamage, ok := beginSingleTargetSkill(g, core.SkillFirebolt, quality)
	if !ok {
		return false
	}
	// Effect is pulled separately for the burn-chance roll below.
	effect := core.EffectiveSkillEffect(actor, core.SkillFirebolt)
	crit, _ := rollSkillCrit(g, actor, core.SkillFirebolt, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	// Firebolt is Magic-tagged so dealt == rawDamage in practice;
	// using the return keeps the log honest if a future Tag change
	// brings armor back into play.
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(core.SkillFirebolt))
	core.EnqueueEnemyVFX(g, core.VFXEmber, g.Battle.EnemyIndex)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	burned := tryProcStatus(g.Rand(), &enemy.BurnTurns, defeated, effect.BurnChance, quality, 0, effect.BurnDuration, core.EnemyInfoFor(*enemy).Stats.WIS)
	setBattleMessage(g, appendCrit(fireboltMessage(actor.Name, target, damage, quality, defeated, burned, enemy.BurnTurns), crit))
	finishPartyAction(g)
	return true
}

// --- Crushing Blow (Warrior, charge phys hit with Stun proc on Great+) ---

func setupCrushingBlow(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillCrushingBlow, "Crushing Blow")
}

func applyCrushingBlow(g *core.GameState, quality int) bool {
	actor, target, rawDamage, ok := beginSingleTargetSkill(g, core.SkillCrushingBlow, quality)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillCrushingBlow)
	// Crushing Blow T3 ("Excellent crits") doubles damage on a
	// landed Excellent timing roll. CritDoubleOnExcellent is the
	// tier-only mod the apply path consults — bool flag so future
	// "T4 triples" etc. would extend the mod struct, not this site.
	// This tier doubling is INDEPENDENT of the universal crit roll
	// below; an Excellent T3 swing that also wins the RollCrit dice
	// stacks both multipliers (CritMultiplier × 2 = 4×) — same
	// shape as Backstab T2's double-crit.
	if quality == core.TimingQualityExcellent && core.SkillTierMod(actor, core.SkillCrushingBlow).CritDoubleOnExcellent {
		rawDamage *= 2
	}
	crit, _ := rollSkillCrit(g, actor, core.SkillCrushingBlow, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(core.SkillCrushingBlow))
	core.EnqueueEnemyVFX(g, core.VFXSlash, g.Battle.EnemyIndex)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	stunned := tryProcStatus(g.Rand(), &enemy.StunTurns, defeated, effect.StunChance, quality, core.TimingQualityGreat, effect.StunDuration, core.EnemyInfoFor(*enemy).Stats.WIS)
	setBattleMessage(g, appendCrit(crushingBlowMessage(actor.Name, target, damage, quality, defeated, stunned), crit))
	finishPartyAction(g)
	return true
}

// --- Whirlwind (Warrior, charge AoE phys) ---

func setupWhirlwind(g *core.GameState) bool {
	return chargeMP(g, core.SkillWhirlwind, "Whirlwind")
}

// applyAoESkill is the shared body for the charge/sequence AoE damage
// skills (Whirlwind, Arc Bolt): bump the actor, quality-scale the
// damage, hit every living enemy, and log the "landed on N" or "caught
// only air" line. The two handlers used to be byte-for-byte identical
// apart from the skill id and the two verbs.
func applyAoESkill(g *core.GameState, skill core.SkillID, skillNoun, hitVerb, emptyVerb string, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	damage := scaleSkillDamage(actor, skill, quality)
	// Single crit roll for the whole sweep — when the dice come up
	// every enemy caught in the AoE eats the doubled tick.
	crit, _ := rollSkillCrit(g, actor, skill, quality)
	damage = applyCritMultiplier(damage, crit, false)
	hits := applyAoEDamage(g, skill, damage, quality)
	if hits == 0 {
		setBattleMessage(g, aoeEmptyMessage(skillNoun, emptyVerb))
	} else {
		setBattleMessage(g, appendCrit(aoeSkillMessage(actor.Name, skillNoun, hitVerb, hits, damage, quality), crit))
	}
	finishPartyAction(g)
	return true
}

func applyWhirlwind(g *core.GameState, quality int) bool {
	return applyAoESkill(g, core.SkillWhirlwind, "Whirlwind", "hits", "catches only air", quality)
}

// --- Mass Mend (Cleric, charge AoE heal) ---

func setupMassMend(g *core.GameState) bool {
	return chargeMP(g, core.SkillMassMend, "Mass Mend")
}

func applyMassMend(g *core.GameState, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	heal := core.ScaleHeal(core.SkillHealFor(actor, core.SkillMassMend), quality)
	mod := core.SkillTierMod(actor, core.SkillMassMend)
	healed := 0
	cleansedTotal := 0
	for i := range g.Party {
		m := &g.Party[i]
		if m.HP <= 0 || m.Ingested {
			continue
		}
		if m.HP < m.MaxHP {
			m.HP += heal
			if m.HP > m.MaxHP {
				m.HP = m.MaxHP
			}
			healed++
		}
		// Tier-3 cleanse applies to EVERY alive member touched, not
		// only those that needed healing — a full-HP poisoned ally
		// still benefits from the wash. Counted separately so the
		// log can call it out even if the heal portion was a no-op.
		if applyStatusCleanses(m, mod) != "" {
			cleansedTotal++
		}
		core.EnqueuePartyVFX(g, core.VFXHeal, i)
	}
	switch {
	case healed == 0 && cleansedTotal == 0:
		setBattleMessage(g, fmt.Sprintf("%s%s's Mass Mend finds no wounds.", qualityTag(quality), actor.Name))
	case healed == 0:
		setBattleMessage(g, fmt.Sprintf("%s%s's Mass Mend cleanses %d allies.", qualityTag(quality), actor.Name, cleansedTotal))
	case cleansedTotal == 0:
		setBattleMessage(g, fmt.Sprintf("%s%s mends %d allies for %d each.", qualityTag(quality), actor.Name, healed, heal))
	default:
		setBattleMessage(g, fmt.Sprintf("%s%s mends %d allies for %d each, cleansing %d.", qualityTag(quality), actor.Name, healed, heal, cleansedTotal))
	}
	finishPartyAction(g)
	return true
}

// --- Smite (Cleric, press-tap magic damage) ---

func setupSmite(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillSmite, "Smite")
}

func applySmite(g *core.GameState, quality int) bool {
	actor, target, rawDamage, ok := beginSingleTargetSkill(g, core.SkillSmite, quality)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillSmite)
	crit, _ := rollSkillCrit(g, actor, core.SkillSmite, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(core.SkillSmite))
	core.EnqueueEnemyVFX(g, core.VFXSmite, g.Battle.EnemyIndex)
	// Smite T3 ("+25% stun") gives the base-stun-less skill a stun
	// proc on Great+ timing. effect.StunChance is 0 at tier 0..2,
	// so tryProcStatus short-circuits cleanly until the tier is
	// purchased — no behavior change for un-upgraded clerics.
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	stunned := tryProcStatus(g.Rand(), &enemy.StunTurns, defeated, effect.StunChance, quality, core.TimingQualityGreat, effect.StunDuration, core.EnemyInfoFor(*enemy).Stats.WIS)
	setBattleMessage(g, appendCrit(smiteMessage(actor.Name, target, damage, quality, defeated, stunned), crit))
	finishPartyAction(g)
	return true
}

// --- Backstab (Thief, charge phys with crit on Excellent) ---

func setupBackstab(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillBackstab, "Backstab")
}

func applyBackstab(g *core.GameState, quality int) bool {
	actor, target, rawDamage, ok := beginSingleTargetSkill(g, core.SkillBackstab, quality)
	if !ok {
		return false
	}
	// Timing-gated crit: an Excellent Backstab is a guaranteed crit
	// (rollSkillCrit's Backstab-special branch). Phys-tagged, so
	// amoebas still chew most of it; the multiplier is the thief's
	// reward for nailing the charge. Backstab T2 ("Excellent crits
	// harder") flags the `double` return, which applyCritMultiplier
	// turns into the second ×2 — perfectly-timed Backstab at T2+ is
	// x4 damage. Non-Excellent presses still get the universal
	// DEX+timing crit chance via the same helper's RollCrit branch.
	crit, double := rollSkillCrit(g, actor, core.SkillBackstab, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, double)
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(core.SkillBackstab))
	core.EnqueueEnemyVFX(g, core.VFXSlash, g.Battle.EnemyIndex)
	setBattleMessage(g, backstabMessage(actor.Name, target, damage, quality, defeated, crit))
	finishPartyAction(g)
	return true
}

// --- Venom Strike (Thief, sequence phys + Poison apply) ---

func setupVenomStrike(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillVenomStrike, "Venom Strike")
}

func applyVenomStrike(g *core.GameState, quality int) bool {
	actor, target, rawDamage, ok := beginSingleTargetSkill(g, core.SkillVenomStrike, quality)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillVenomStrike)
	crit, _ := rollSkillCrit(g, actor, core.SkillVenomStrike, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(core.SkillVenomStrike))
	core.EnqueueEnemyVFX(g, core.VFXVenom, g.Battle.EnemyIndex)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	poisoned := tryProcStatus(g.Rand(), &enemy.PoisonTurns, defeated, effect.PoisonChance, quality, 0, effect.PoisonDuration, core.EnemyInfoFor(*enemy).Stats.WIS)
	setBattleMessage(g, appendCrit(venomStrikeMessage(actor.Name, target, damage, quality, defeated, poisoned), crit))
	finishPartyAction(g)
	return true
}

// --- Frost Lance (Wizard, charge magic with reliable Stun on Great+) ---

func setupFrostLance(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillFrostLance, "Frost Lance")
}

func applyFrostLance(g *core.GameState, quality int) bool {
	actor, target, rawDamage, ok := beginSingleTargetSkill(g, core.SkillFrostLance, quality)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillFrostLance)
	crit, _ := rollSkillCrit(g, actor, core.SkillFrostLance, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(core.SkillFrostLance))
	core.EnqueueEnemyVFX(g, core.VFXFrost, g.Battle.EnemyIndex)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	// FrostLance is flavored as a freeze but reads from the canonical
	// StunTurns counter — there's no separate "frozen" status today,
	// only the timing-gate that turns Stun-on-Great into a near-
	// guaranteed lock. The variable is `stunned` to match the field
	// it queries (any future grep for StunTurns lands here cleanly);
	// the player-facing log line keeps the "Frozen!" flavor via
	// frostLanceMessage.
	stunned := tryProcStatus(g.Rand(), &enemy.StunTurns, defeated, effect.StunChance, quality, core.TimingQualityGreat, effect.StunDuration, core.EnemyInfoFor(*enemy).Stats.WIS)
	setBattleMessage(g, appendCrit(frostLanceMessage(actor.Name, target, damage, quality, defeated, stunned), crit))
	finishPartyAction(g)
	return true
}

// --- Arc Bolt (Wizard, sequence-tap AoE magic) ---

func setupArcBolt(g *core.GameState) bool {
	return chargeMP(g, core.SkillArcBolt, "Arc Bolt")
}

func applyArcBolt(g *core.GameState, quality int) bool {
	return applyAoESkill(g, core.SkillArcBolt, "Arc Bolt", "arcs across", "dissipates with no target", quality)
}

// --- Damage / heal helpers (unchanged from previous behavior) ---

// setupTargetedEnemyAndPay is the standard setup gate for a
// single-target damaging skill: confirm a living enemy is targeted
// AND deduct the skill's MP cost. Combines setupTargetedEnemy +
// chargeMP — six setup functions (Firebolt / Crushing Blow / Smite /
// Backstab / Venom Strike / Frost Lance) used to inline the same
// "if !alive { No target.; return false }" then "if !chargeMP { ...
// return false }" pair. Bundled so a future "wakefulness check" or
// "concentration roll" lands in one helper.
//
// label is the human name passed through to chargeMP's status
// message ("Firebolt needs more MP." etc.).
func setupTargetedEnemyAndPay(g *core.GameState, skill core.SkillID, label string) bool {
	if !setupTargetedEnemy(g) {
		return false
	}
	return chargeMP(g, skill, label)
}

// scaleSkillDamage returns the quality-scaled raw damage figure for
// `actor` casting `skill`. Wraps `core.ScaleDamage(core.SkillDamageFor(...))`
// — the exact two-call chain every damaging apply function used to
// open-code. Centralising means a future "STR-magic hybrid" base
// formula or a global damage multiplier lands in one place; today's
// nine call sites pull the same two helpers off in lockstep, which
// already drifted once on the AoE side when applyArcBolt was added.
func scaleSkillDamage(actor *core.PartyMember, skill core.SkillID, quality int) int {
	return core.ScaleDamage(core.SkillDamageFor(actor, skill), quality)
}

// rollSkillCrit returns the (crit, double) flags for `actor` using
// `skill` at the given timing `quality`. Standard skills crit via the
// probabilistic core.RollCrit (DEX + per-grade bonus from
// timingGrades.CritBonus). Backstab keeps its signature "Excellent
// timing = guaranteed crit" — and the T2 CritDoubleOnExcellent tier
// stacks an additional doubling on that specific path. The `double`
// flag is only ever true on the deterministic Backstab path; the
// probabilistic crit just multiplies by core.CritMultiplier once.
func rollSkillCrit(g *core.GameState, actor *core.PartyMember, skill core.SkillID, quality int) (crit, double bool) {
	if actor == nil {
		return false, false
	}
	if skill == core.SkillBackstab && quality >= core.TimingQualityExcellent {
		crit = true
		if core.SkillTierMod(actor, core.SkillBackstab).CritDoubleOnExcellent {
			double = true
		}
		return
	}
	crit = core.RollCrit(g.Rand(), core.EffectiveStats(*actor), quality)
	return
}

// applyCritMultiplier returns the post-crit damage. Pass through
// unchanged when neither flag is set so a no-crit hit stays a single
// arithmetic op. The double flag is the Backstab-T2 stacker; it
// multiplies AGAIN on top of the standard CritMultiplier.
func applyCritMultiplier(raw int, crit, double bool) int {
	if !crit {
		return raw
	}
	out := raw * core.CritMultiplier
	if double {
		out *= 2
	}
	return out
}

// appendCrit suffixes the combat-log message with " Critical!" when
// crit landed. Used by every damaging player skill EXCEPT Backstab,
// which already encodes the crit in its proc-arm copy ("lands a clean
// Backstab for X!"). Centralised so a future "crit color code" pass
// touches one place instead of seven.
func appendCrit(msg string, crit bool) string {
	if !crit {
		return msg
	}
	return msg + " Critical!"
}

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
	// Magic-tagged damage clips through the enemy's MDef (symmetric
	// with the party-side ApplyMagicDefense path). Most enemies carry
	// MDef 0 today — only the wizard-flavored kinds (Wisp, Goblin
	// Mage, Necromancer) and the Stone Golem authored a non-zero
	// value, so player Firebolt against unarmored grunts still feels
	// the same. The future enemy-magic-resist pass tunes the field.
	damage = core.ApplyMagicDefense(damage, tag, core.EnemyInfoFor(*enemy).MDef)
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
		// Receiver recoil — the enemy flinches backward away from
		// the camera. Only fires on real damage (armor-shrugged 1s
		// still recoil; pure zero-damage connections don't).
		enemy.HitKnockback = core.HitKnockbackDuration
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
	clearEnemyStatusesOnDeath(enemy)
	core.EnqueueEnemyVFX(g, core.VFXDeath, slot)
	// Clear the recoil timer on death so the corpse fades from
	// its resting position rather than displaying a knocked-back
	// offset while DeathFade plays out — the two timers overlap
	// otherwise (HitKnockback was just set six lines above the
	// HP<=0 branch we're in) and the body reads as drifting away
	// during the fade.
	enemy.HitKnockback = 0
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
	if !actor.ValidPartyIndex(g.Party) {
		return false
	}
	member := &g.Party[actor.Index]
	if member.HP <= 0 || member.PoisonTurns <= 0 {
		return false
	}
	return applyPartyPoisonTick(g, actor.Index)
}

// applyPartyPoisonTick decrements one party member's Poison counter,
// deals PoisonTickDamage (Magic-tagged so armor doesn't damp the DoT),
// and logs the suffers / succumbs line. Returns true on the fatal tick.
// Shared by the end-of-turn tick and the ingested-member round tick,
// which used to inline this damage-and-message body verbatim. Callers
// guard HP/PoisonTurns before calling.
func applyPartyPoisonTick(g *core.GameState, index int) bool {
	member := &g.Party[index]
	member.PoisonTurns--
	// damagePartyMember returns true on the fatal hit; use it as the
	// authoritative kill signal so a future "save at 1 HP" mechanic in
	// damagePartyMember can't desync from the message we emit here.
	dealt, killed := damagePartyMember(g, index, core.PoisonTickDamage, core.SkillTagMagic)
	if killed {
		setBattleMessage(g, fmt.Sprintf("%s succumbs to the poison.", member.Name))
		return true
	}
	setBattleMessage(g, fmt.Sprintf("%s suffers %d from poison.", member.Name, dealt))
	return false
}

// tickPoisonForIngestedParty applies one tick of poison to every ingested
// party member whose PoisonTurns counter is still active. Ingested members
// are skipped from the per-turn queue (buildTurnQueue), so their normal
// end-of-turn Poison tick never fires — which would silently turn ingest
// into a free pause of the DoT. Fire this once per round from beginNewRound
// (before the loss gate) so a poison kill while ingested still routes
// through ActivePartyCount and triggers the loss check.
func tickPoisonForIngestedParty(g *core.GameState) {
	for i := range g.Party {
		m := &g.Party[i]
		if !m.Ingested || m.HP <= 0 || m.PoisonTurns <= 0 {
			continue
		}
		applyPartyPoisonTick(g, i)
	}
}

// tickBoundAfterPartyTurn drains the Bound counter at the end of the
// bound member's own turn. Same shape as the Poison tick — actor-kind
// dispatch up front, party-only today (no party skill applies Bound
// to enemies). Emits a short log line when the status wears off so
// the player sees the counter clear. No damage tied to Bound;
// the slow / Ingest-refusal effect lives in actorSpeed +
// handleEnemyIngest.
func tickBoundAfterPartyTurn(g *core.GameState, actor core.ActorRef) {
	tickPartyStatusCounter(g, actor, func(m *core.PartyMember) *int { return &m.BoundTurns }, "%s tears free of the webs.")
}

// tickConfusedAfterPartyTurn mirrors tickBoundAfterPartyTurn for the
// Confused status. The per-action retarget roll is honored at action
// resolution time (see action handlers' confuse-retarget path); this
// helper just drains the counter.
func tickConfusedAfterPartyTurn(g *core.GameState, actor core.ActorRef) {
	tickPartyStatusCounter(g, actor, func(m *core.PartyMember) *int { return &m.ConfusedTurns }, "%s's head clears.")
}

// tickPartyStatusCounter is the shared body the non-damaging
// end-of-party-turn status ticks (Bound, Confused) walk. Each ticker
// used to inline the same actor-kind dispatch + index bounds +
// HP/counter guard. counterRef returns a pointer into the member's
// field so the helper can both read and decrement without a
// type-specific switch. clearedFmt is a "%s" template — when the
// counter hits zero, the helper formats with the member's name and
// emits the cleared message. Pass "" for a silent clear.
//
// Poison's tick stays separate because it also deals damage and
// returns a kill signal — the shape doesn't collapse cleanly into
// this helper without piling the heal/death machinery into the
// signature. Burn likewise sits in its own function for the same
// reason (damage + start-of-turn semantics, different timing seam).
func tickPartyStatusCounter(g *core.GameState, actor core.ActorRef, counterRef func(*core.PartyMember) *int, clearedFmt string) {
	if !actor.ValidPartyIndex(g.Party) {
		return
	}
	m := &g.Party[actor.Index]
	if m.HP <= 0 {
		return
	}
	c := counterRef(m)
	if *c <= 0 {
		return
	}
	*c--
	if *c == 0 && clearedFmt != "" {
		setBattleMessage(g, fmt.Sprintf(clearedFmt, m.Name))
	}
}

// tickPoisonAfterEnemyTurn is the enemy-side mirror of
// tickPoisonAfterPartyTurn. The Thief's Venom Strike applies
// Enemy.PoisonTurns; this helper drains the counter after the enemy's
// own turn lands and deals PoisonTickDamage. Magic-tagged so armor
// doesn't damp the DoT — same rule the party-side tick uses.
func tickPoisonAfterEnemyTurn(g *core.GameState, actor core.ActorRef) bool {
	if actor.IsParty {
		return false
	}
	enemy := core.BattleMemberAt(g, actor.Index)
	if enemy == nil || !enemy.Alive || enemy.PoisonTurns <= 0 {
		return false
	}
	enemy.PoisonTurns--
	dealt, defeated := damageEnemy(g, actor.Index, core.PoisonTickDamage, core.TimingQualityGood, core.SkillTagMagic)
	noun := core.EnemySingularNoun(*enemy)
	if defeated {
		setBattleMessage(g, fmt.Sprintf("The %s succumbs to the poison.", noun))
		return true
	}
	setBattleMessage(g, fmt.Sprintf("The %s suffers %d from poison.", noun, dealt))
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
// when `tag == SkillTagPhys` and WIS-clipped when `tag == SkillTagMagic`
// via ApplyMagicDefense. Heal / Buff still bypass mitigation entirely.
// Any damage > 0 wakes the member from Sleep — same "violence breaks
// the spell" rule as the enemy side.
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
	// Mitigation reads through EffectiveArmor / EffectiveMDef so any
	// equipped gear bonuses stack on top of the base values. Base
	// member.Armor stays authored (0 today on the party side); items
	// add to it via ArmorBonus / MDefBonus on their ItemDefinition.
	amount := core.ApplyArmor(rawAmount, tag, core.EffectiveArmor(*member))
	amount = core.ApplyMagicDefense(amount, tag, core.EffectiveMDef(*member))
	member.DamageFlash = core.FlashDuration
	member.HP -= amount
	if amount > 0 {
		// Reactionary knockback — only on real damage so a fully-
		// soaked hit doesn't visually shove a tank who took 0. The
		// renderer pushes the member toward the camera (away from
		// the attacking enemy formation) for HitKnockbackDuration.
		member.HitKnockback = core.HitKnockbackDuration
		if member.SleepTurns > 0 {
			member.SleepTurns = 0
		}
	}
	if member.HP > 0 {
		return amount, false
	}
	member.HP = 0
	clearPartyStatusesOnDeath(member)
	return amount, true
}

// clearEnemyStatusesOnDeath / clearPartyStatusesOnDeath are the two
// canonical "wipe transient timed statuses now that this actor is
// dead" hooks. Centralizing them means a future timed status (Curse,
// Charm, …) lands as one row in the matching helper instead of
// silently lingering on a corpse — and the asymmetry between the two
// actor types lives in one place so it's reviewable rather than
// scattered across multiple kill sites.
//
// Enemy side clears Burn + Sleep + Poison + Stun. Party side clears the
// statuses a member can actually carry into death today: Sleep (goblin
// mage), Bound (cave spider web), and Confused (will-o'-wisp). Poison is
// intentionally left so the corpse keeps its poison render hint while it
// fades; Burn has no player-applicable source yet. Add new timed statuses
// to whichever side can carry them so they can't linger on a corpse.
func clearEnemyStatusesOnDeath(enemy *core.Enemy) {
	enemy.BurnTurns = 0
	enemy.SleepTurns = 0
	enemy.PoisonTurns = 0
	enemy.StunTurns = 0
}

func clearPartyStatusesOnDeath(member *core.PartyMember) {
	member.SleepTurns = 0
	member.BoundTurns = 0
	member.ConfusedTurns = 0
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

// procMessageArms holds the three combat-log copy variants a
// single-target damaging skill picks between: the defeated kill line,
// the status-proc landing line, and the plain hit. Format strings use
// explicit argument indices over the fixed arg tuple
// (tag=%[1]s, name=%[2]s, noun=%[3]s, damage=%[4]d) so an arm can omit
// whichever verbs it doesn't need without a "too many args" error.
type procMessageArms struct{ defeated, proc, plain string }

// procSkillMessage selects and formats the right arm. Collapses the five
// near-identical 3-arm switch helpers (Crushing Blow / Smite / Backstab /
// Venom Strike / Frost Lance) into one; each skill now supplies only its
// copy via a procMessageArms table. A future "battle-log color codes"
// pass lands here once instead of five times. (fireboltMessage keeps its
// own 4-arm form — it has an extra "already burning" line.)
func procSkillMessage(arms procMessageArms, name string, target core.Enemy, damage, quality int, defeated, proc bool) string {
	f := arms.plain
	switch {
	case defeated:
		f = arms.defeated
	case proc:
		f = arms.proc
	}
	return fmt.Sprintf(f, qualityTag(quality), name, core.EnemySingularNoun(target), damage)
}

var (
	crushingBlowArms = procMessageArms{
		defeated: "%[1]s%[2]s shatters the %[3]s with a Crushing Blow.",
		proc:     "%[1]s%[2]s crushes the %[3]s for %[4]d. Stunned!",
		plain:    "%[1]s%[2]s Crushing Blows for %[4]d.",
	}
	smiteArms = procMessageArms{
		defeated: "%[1]s%[2]s smites the %[3]s down.",
		proc:     "%[1]s%[2]s smites for %[4]d. Stunned!",
		plain:    "%[1]s%[2]s smites for %[4]d.",
	}
	backstabArms = procMessageArms{
		defeated: "%[1]s%[2]s's Backstab fells the %[3]s.",
		proc:     "%[1]s%[2]s lands a clean Backstab for %[4]d!",
		plain:    "%[1]s%[2]s stabs for %[4]d.",
	}
	venomStrikeArms = procMessageArms{
		defeated: "%[1]s%[2]s's Venom Strike fells the %[3]s.",
		proc:     "%[1]s%[2]s envenoms the %[3]s for %[4]d. Poisoned!",
		plain:    "%[1]s%[2]s stings for %[4]d.",
	}
	frostLanceArms = procMessageArms{
		defeated: "%[1]s%[2]s's Frost Lance shatters the %[3]s.",
		proc:     "%[1]s%[2]s freezes the %[3]s for %[4]d. Frozen!",
		plain:    "%[1]s%[2]s lances for %[4]d.",
	}
)

func crushingBlowMessage(name string, target core.Enemy, damage, quality int, defeated, stunned bool) string {
	return procSkillMessage(crushingBlowArms, name, target, damage, quality, defeated, stunned)
}

func smiteMessage(name string, target core.Enemy, damage, quality int, defeated, stunned bool) string {
	return procSkillMessage(smiteArms, name, target, damage, quality, defeated, stunned)
}

func backstabMessage(name string, target core.Enemy, damage, quality int, defeated, crit bool) string {
	return procSkillMessage(backstabArms, name, target, damage, quality, defeated, crit)
}

func venomStrikeMessage(name string, target core.Enemy, damage, quality int, defeated, poisoned bool) string {
	return procSkillMessage(venomStrikeArms, name, target, damage, quality, defeated, poisoned)
}

func frostLanceMessage(name string, target core.Enemy, damage, quality int, defeated, stunned bool) string {
	return procSkillMessage(frostLanceArms, name, target, damage, quality, defeated, stunned)
}

// aoeSkillMessage / aoeEmptyMessage format the canonical AoE log
// lines. Hit message: "<grade>! <actor>'s <Skill> <verb> <hits> foes
// for <damage> each." Empty fallback: "<Skill> <emptyVerb>." Used by
// every AoE handler (Swipe / Whirlwind / Arc Bolt) to keep the
// "missed everyone" and "landed on N" shapes consistent.
func aoeSkillMessage(name, skillNoun, hitVerb string, hits, damage, quality int) string {
	return fmt.Sprintf("%s%s's %s %s %d foes for %d each.", qualityTag(quality), name, skillNoun, hitVerb, hits, damage)
}

func aoeEmptyMessage(skillNoun, emptyVerb string) string {
	return fmt.Sprintf("%s %s.", skillNoun, emptyVerb)
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
	// Dodge precedes damage math: a DEX-driven sidestep eats the whole
	// swing — no damage, no poison proc, no lifesteal — even on a hit
	// the player blocked poorly. The defend-timing quality the player
	// pressed is still recorded so the grade-quality history tracks
	// what they did, not just what landed. Skills (goblin mage casts,
	// stone golem slam, etc.) go through their own resolver and are
	// NOT dodgeable today — mirrors AttackAccuracy, which only gates
	// basic attacks.
	if core.RollDodge(g.Rand(), core.EffectiveStats(g.Party[target])) {
		recordQuality(g, defendQuality, target, true)
		setBattleMessage(g, fmt.Sprintf("%s sidesteps the %s.", g.Party[target].Name, core.EnemySingularNoun(*enemy)))
		return true
	}
	rawDamage := core.EnemyInfoFor(*enemy).AttackDamage
	// Enemy crit on basic attacks — symmetric with the player side, but
	// no timing-grade bonus (enemies don't press a bar). Pure
	// DEX-driven CritChance via core.RollCrit at the Miss grade
	// keeps enemies on a flat ~5-10% crit floor where the player can
	// push 30%+ on Excellent.
	enemyCrit := core.RollCrit(g.Rand(), core.EnemyInfoFor(*enemy).Stats, core.TimingQualityMiss)
	if enemyCrit {
		rawDamage *= core.CritMultiplier
	}
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
	// Slash VFX only when damage actually landed — an Excellent
	// defend can clamp damage to 0, and the player just performed a
	// successful block. Spawning impact sparks on a perfect parry
	// would visually undersell the block.
	if dealt > 0 {
		core.EnqueuePartyVFX(g, core.VFXSlash, target)
	}
	if defendQuality > core.TimingQualityMiss {
		// A successful block recoils the defender slightly so the impact reads
		// even though the damage number is small.
		g.Party[target].AttackBump = core.BlockBumpDuration
	}
	recordQuality(g, defendQuality, target, true)
	def := core.EnemyInfoFor(*enemy)
	setBattleMessage(g, appendCrit(enemyHitMessage(*enemy, g.Party[target].Name, dealt, defendQuality, g.Party[target].Defending), enemyCrit))
	// Poison inflict: only on damaging hits from a poison-themed attacker
	// against a target that's still alive and not already poisoned. The
	// no-stack rule mirrors burn — re-poisoning a poisoned target on every
	// bite would trivialize the duration roll.
	if damage > 0 && def.PoisonChance > 0 && g.Party[target].HP > 0 && g.Party[target].PoisonTurns <= 0 {
		if g.Rand().Float64() < def.PoisonChance {
			rawPoison := core.DefaultPoisonEffect.RollDuration(g.Rand())
			g.Party[target].PoisonTurns = core.ShortenStatusDuration(rawPoison, core.EffectiveStats(g.Party[target]).WIS)
			setBattleMessage(g, fmt.Sprintf("%s is poisoned!", g.Party[target].Name))
		}
	}
	// Lifesteal: the Vampire Bat (and any future LifestealPercent kind)
	// heals for a fraction of the post-armor damage dealt. Reads `dealt`
	// (already armor-clipped AND Defending-reduced) so a soaked or
	// blocked hit produces a proportionally small drain — when the
	// multiplier rounds to zero, the bat heals nothing (earlier passes
	// floored at 1, which leaked a free 1-HP heal off 1-damage hits
	// the player had just Defended down to a chip). Capped at MaxHP.
	if def.LifestealPercent > 0 && dealt > 0 && enemy.HP > 0 {
		heal := int(float64(dealt) * def.LifestealPercent)
		if heal > 0 {
			enemy.HP += heal
			// Cap against the per-instance MaxHP, not the definition's
			// base MaxHP — a future raised/scaled enemy with a non-
			// default per-instance ceiling would otherwise overheal
			// past its real cap or undercap a buffed one. Today both
			// values match because NewEnemy seeds from the same def.
			if enemy.HP > enemy.MaxHP {
				enemy.HP = enemy.MaxHP
			}
			setBattleMessage(g, fmt.Sprintf("%s drains life from %s (+%d HP).", core.TheEnemy(def), g.Party[target].Name, heal))
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
