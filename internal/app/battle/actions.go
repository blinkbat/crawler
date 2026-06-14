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
	core.SkillSwipe:         {setup: setupSwipe, apply: applySwipe},
	core.SkillPrayer:        {setup: setupPrayer, apply: applyPrayer},
	core.SkillSteal:         {setup: setupTargetedEnemy, apply: applySteal},
	core.SkillScan:          {setup: setupScan, apply: applyScan},
	core.SkillFirebolt:      {setup: setupFirebolt, apply: applyFirebolt},
	core.SkillCrushingBlow:  {setup: setupCrushingBlow, apply: applyCrushingBlow},
	core.SkillWhirlwind:     {setup: setupWhirlwind, apply: applyWhirlwind},
	core.SkillMassMend:      {setup: setupMassMend, apply: applyMassMend},
	core.SkillSmite:         {setup: setupSmite, apply: applySmite},
	core.SkillBackstab:      {setup: setupBackstab, apply: applyBackstab},
	core.SkillVenomStrike:   {setup: setupVenomStrike, apply: applyVenomStrike},
	core.SkillFrostLance:    {setup: setupFrostLance, apply: applyFrostLance},
	core.SkillArcBolt:       {setup: setupArcBolt, apply: applyArcBolt},
	core.SkillBless:         {setup: setupBless, apply: applyBless},
	core.SkillFireball:      {setup: setupFireball, apply: applyFireball},
	core.SkillPoisonCloud:   {setup: setupPoisonCloud, apply: applyPoisonCloud},
	core.SkillCleanse:       {setup: setupCleanse, apply: applyCleanse},
	core.SkillSecondWind:    {setup: setupSecondWind, apply: applySecondWind},
	core.SkillRenewal:       {setup: setupRenewal, apply: applyRenewal},
	core.SkillCripple:       {setup: setupCripple, apply: applyCripple},
	core.SkillFrostbite:     {setup: setupFrostbite, apply: applyFrostbite},
	core.SkillCorrosiveVial: {setup: setupCorrosiveVial, apply: applyCorrosiveVial},
	core.SkillConeOfCold:    {setup: setupConeOfCold, apply: applyConeOfCold},
	core.SkillSunder:        {setup: setupSunder, apply: applySunder},
	core.SkillTaunt:         {setup: setupTaunt, apply: applyTaunt},
	core.SkillWarBanner:     {setup: setupWarBanner, apply: applyWarBanner},
	core.SkillStoneSkin:     {setup: setupStoneSkin, apply: applyStoneSkin},
	core.SkillBlind:         {setup: setupBlind, apply: applyBlind},
	core.SkillAegis:         {setup: setupAegis, apply: applyAegis},
	core.SkillSmokeBomb:     {setup: setupSmokeBomb, apply: applySmokeBomb},
	core.SkillIceArmor:      {setup: setupIceArmor, apply: applyIceArmor},
	core.SkillRend:          {setup: setupRend, apply: applyRend},
	core.SkillLacerate:      {setup: setupLacerate, apply: applyLacerate},
}

// init asserts the player-castable contract: every PlayerCastable skill
// in the registry must have an entry in skillActionHandlers. Skill
// progression now flows through the skill trees (core.LearnedSkills),
// and core asserts every tree node's GrantSkill is PlayerCastable — so
// "every granted skill resolves to a handler" follows transitively from
// this single loop. Without it, a registry author who forgot to register
// a handler would only surface at playtest as "No skill ready." — a
// vague runtime error far from the cause.
func init() {
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
	// Scale by the global enemy-difficulty dial (same seam as basic-attack /
	// spawn HP) so casters and AoE foes get harder in lockstep, then floor at 1.
	raw := core.ScaleEnemyDifficulty(def.SpellPower + effect.Damage)
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
	dealt, killed := damagePartyMemberDefendable(g, ctx.target, enemySpellDamage(ctx.def, ctx.effect), core.SkillTagFor(core.SkillFirebolt))
	core.EnqueuePartyVFX(g, vfxKindFor(core.SkillFirebolt), ctx.target)
	if killed {
		setBattleMessage(g, fmt.Sprintf("%s incinerates %s.", core.TheEnemy(ctx.def), g.Party[ctx.target].Name))
	} else {
		enemySpellLog(ctx, "%s burns for %d.", g.Party[ctx.target].Name, dealt)
	}
	audio.Play(audio.SoundEnemyHit)
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
	// Webbed targets refuse Ingest — the design contract for Webbed is
	// "tempo control without removal," so the spider's web should
	// shield the prey from the mantrap. The mantrap just bites
	// instead this turn (caller falls back to plain melee on the
	// next round when usableEnemySkills sees Ingest still pending).
	if m.WebbedTurns > 0 {
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
	core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillIngest), ctx.slot)
	setBattleMessage(g, fmt.Sprintf("%s engulfs %s!", core.TheEnemy(ctx.def), m.Name))
	audio.Play(audio.SoundEnemyHit)
}

// applyEnemyStatus is the shared mechanical tail of the enemy status casts
// (Sleep / Webbed / Confused): floor a non-positive rolled duration, apply it
// to the target's counter shortened by WIS (the universal resist path), fire
// the status VFX, and play the land cue. The caller owns the per-status guards
// and log lines — those vary in wording and in whether they name the caster —
// so only the identical apply mechanics live here. The enemy→party mirror of
// the player→enemy tryProcStatus tail (which the latter's comment references).
// Returns true when the status actually landed. The validity invariant (target
// in range, alive, not ingested) lives HERE with the dereference rather than
// being owned by each caller / leaned on the upstream pickEnemyAttackTarget
// filter — so a future caster that forgets its own guard can't stamp a status
// onto a corpse or an ingested ally, and the bool lets the caller skip its
// success log so it can't claim a status that didn't land. No behavior change
// today: the current callers always pass a living, non-ingested target.
func applyEnemyStatus(ctx enemySpellCtx, counter *int, duration, floor int, vfxSkill core.SkillID) bool {
	if !core.PartyMemberAvailable(ctx.g.Party, ctx.target) {
		return false
	}
	if duration <= 0 {
		duration = floor
	}
	m := &ctx.g.Party[ctx.target]
	*counter = core.ShortenStatusDuration(duration, core.EffectiveStats(*m).WIS)
	core.EnqueuePartyVFX(ctx.g, vfxKindFor(vfxSkill), ctx.target)
	audio.Play(audio.SoundInputHit)
	return true
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
	if applyEnemyStatus(ctx, &m.SleepTurns, ctx.effect.SleepDuration(g.Rand()), core.SleepMinTurns, core.SkillSleep) {
		enemySpellLog(ctx, "%s falls asleep.", m.Name)
	}
}

// handleEnemyWeb applies the Cave Spider's Webbed status. Already-webbed
// targets short-circuit with a flavor line (no stacking); otherwise
// the duration rolls from BindMin/Max and lands on the target. The
// target is guaranteed alive by pickEnemyAttackTarget's living-filter
// upstream — no HP<=0 guard needed here.
func handleEnemyWeb(ctx enemySpellCtx) {
	g := ctx.g
	m := &g.Party[ctx.target]
	if m.WebbedTurns > 0 {
		setBattleMessage(g, fmt.Sprintf("%s spins a fresh web at %s — already webbed.", core.TheEnemy(ctx.def), m.Name))
		return
	}
	if applyEnemyStatus(ctx, &m.WebbedTurns, ctx.effect.BindDuration(g.Rand()), core.SpiderWebbedMinTurns, core.SkillWeb) {
		enemySpellLog(ctx, "%s is wrapped in sticky webs.", m.Name)
	}
}

// handleEnemyConfuse applies the Will-o'-Wisp's Confused status.
// WIS resistance lives in the universal ShortenStatusDuration path
// (mirrors Sleep / Webbed / Poison applies on the party side) — high
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
	if applyEnemyStatus(ctx, &m.ConfusedTurns, ctx.effect.ConfuseDuration(g.Rand()), core.WispConfuseMinTurns, core.SkillConfuse) {
		enemySpellLog(ctx, "%s grows confused.", m.Name)
	}
}

// handleEnemyStoneslam fires the Stone Golem's AoE phys cast. Hits
// every living party member (skipping ingested ones — they're
// untargetable while inside their swallower) with damage = SpellPower
// + Effect.Damage, tagged Phys so per-target Armor applies. Routed
// through damagePartyMemberDefendable so a braced member soaks the slam
// the same as a basic melee hit. No status component — pure damage.
func handleEnemyStoneslam(ctx enemySpellCtx) {
	g := ctx.g
	raw := enemySpellDamage(ctx.def, ctx.effect)
	hits := 0
	kills := 0
	for _, i := range core.AvailablePartyTargets(g.Party) {
		_, killed := damagePartyMemberDefendable(g, i, raw, core.SkillTagFor(core.SkillStoneslam))
		core.EnqueuePartyVFX(g, vfxKindFor(core.SkillStoneslam), i)
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
	pack := core.ActivePack(g)
	if pack == nil {
		setBattleMessage(g, fmt.Sprintf("%s gestures, but the bones refuse to rise.", core.TheEnemy(ctx.def)))
		return
	}
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
	return core.CanAffordSkill(actor, skill)
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
	// Debug "all skills" makes every cast free — pairs with the skill menu
	// listing skills the member never paid the MP to learn. This is the single
	// MP chokepoint (setupTargetedEnemyAndPay routes through here too), so the
	// one bypass covers every skill.
	if g.DebugAllSkills {
		return true
	}
	actor := &g.Party[g.Battle.CurrentParty]
	if !core.SpendSkillMP(actor, skill) {
		setBattleStatus(g, label+" needs more MP.")
		return false
	}
	return true
}

// forEachLivingEnemy invokes fn(slot, enemy) for every alive member of the
// active pack. The single living-enemy walk the whole-pack apply paths share
// (applyAoEDamage / applyAoEStatusSkill / applySmokeBomb) so "who does a
// pack-wide effect touch?" lives in one loop. enemy is the write-through
// pointer (core.BattleMemberAt) so callers can mutate it directly.
func forEachLivingEnemy(g *core.GameState, fn func(slot int, enemy *core.Enemy)) {
	for slot, m := range core.BattleMembers(g) {
		if !m.Alive {
			continue
		}
		fn(slot, core.BattleMemberAt(g, slot))
	}
}

// applyAoEDamage hits every living enemy in the active pack with the
// given damage amount, routing through damageEnemy so the skill's
// SkillTag-driven armor rules apply. Returns the hit count for the
// log message.
func applyAoEDamage(g *core.GameState, skill core.SkillID, damage, quality int, shake bool) int {
	hits := 0
	tag := core.SkillTagFor(skill)
	vfx := vfxKindFor(skill)
	forEachLivingEnemy(g, func(slot int, _ *core.Enemy) {
		damageEnemy(g, slot, damage, quality, tag)
		core.EnqueueEnemyVFX(g, vfx, slot)
		hits++
	})
	if hits > 0 && shake {
		// AoE casts are the "costly" hits that earn the big camera punch
		// (overrides the subtle base shake from the timing grade). Callers
		// that loop this (multi-pass Swipe) pass shake=false and fire one
		// shake after all passes so the punch arms once per attack.
		core.TriggerCombatShake(&g.Battle, core.CombatShakeBigPeak, core.CombatShakeBigDur)
	}
	return hits
}

// vfxKindFor maps a skill to the particle effect its apply step queues.
// Centralised so the skill→VFX mapping is one source of truth, mirroring
// core.SkillTagFor (each apply site already pulls its SkillTag through a
// helper rather than hardcoding it — the VFX kind is the same kind of
// skill-derived property). Adding a new skill's effect is one row here
// instead of an inline core.VFX<Kind> literal scattered across the apply
// functions. Callers still choose the enqueue direction (EnqueueEnemyVFX
// for hits, EnqueuePartyVFX for heals / enemy casts landing on the party).
// Unmapped skills return core.VFXNone (no particles).
func vfxKindFor(skill core.SkillID) core.VFXKind {
	switch skill {
	case core.SkillSwipe, core.SkillWhirlwind, core.SkillCrushingBlow, core.SkillBackstab, core.SkillSunder, core.SkillRend, core.SkillLacerate:
		return core.VFXSlash
	case core.SkillArcBolt:
		return core.VFXArc
	case core.SkillFirebolt, core.SkillFireball:
		return core.VFXEmber
	case core.SkillSmite, core.SkillBlind:
		return core.VFXSmite
	case core.SkillVenomStrike, core.SkillPoisonCloud:
		return core.VFXVenom
	case core.SkillFrostLance, core.SkillFrostbite, core.SkillConeOfCold, core.SkillIceArmor:
		return core.VFXFrost
	case core.SkillPrayer, core.SkillMassMend, core.SkillBless, core.SkillCleanse, core.SkillSecondWind, core.SkillRenewal, core.SkillWarBanner, core.SkillStoneSkin, core.SkillAegis:
		return core.VFXHeal
	case core.SkillSteal:
		return core.VFXSteal
	case core.SkillSleep:
		return core.VFXSleep
	case core.SkillWeb:
		return core.VFXWeb
	case core.SkillConfuse:
		return core.VFXConfuse
	case core.SkillStoneslam:
		return core.VFXStoneslam
	case core.SkillIngest:
		return core.VFXIngest
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
	if minGrade > 0 && quality < minGrade {
		return false
	}
	// Player-side procs scale the base chance by the timing grade; the shared
	// applyStatusRoll owns the guard + roll + WIS-shorten apply below.
	return applyStatusRoll(rng, counter, defeated, core.QualityScaledChance(baseChance, quality), durationFn, resistWis)
}

// applyStatusRoll is the shared status-proc core: it refuses when the target is
// defeated, the chance is non-positive, or the counter is already running (the
// no-stack rule), then rolls `chance` and on success stamps the WIS-shortened
// duration onto the counter. Both tryProcStatus (player-side, which pre-scales
// chance by the timing grade) and the enemy basic-attack proc (raw chance, no
// timing minigame) route through it so the "guard → roll → WIS-shorten apply"
// rule lives in ONE place instead of being hand-rolled per side. Returns true
// when the counter was just stamped.
func applyStatusRoll(rng *rand.Rand, counter *int, defeated bool, chance float64, durationFn func(*rand.Rand) int, resistWis int) bool {
	if defeated || chance <= 0 || counter == nil || *counter > 0 {
		return false
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
			core.GainUpTo(&actor.MP, actor.MaxMP, cost)
		}
	}
	setBattleStatus(g, "No target.")
	finishActorTurn(g)
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
func beginSingleTargetSkill(g *core.GameState, skill core.SkillID, quality int) (actor *core.PartyMember, target core.Enemy, rawDamage, resistWIS int, ok bool) {
	if !ensureAliveTargetOrCancel(g, skill) {
		return nil, core.Enemy{}, 0, 0, false
	}
	actor = &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	rawDamage = scaleSkillDamage(actor, skill, quality)
	target = *core.BattleMemberAt(g, g.Battle.EnemyIndex)
	// resistWIS is the target's WIS, hoisted here so the status-proc callers
	// don't each re-derive core.EnemyInfoFor(*enemy).Stats.WIS (it doesn't
	// change mid-action).
	resistWIS = core.EffectiveEnemyStats(target).WIS
	return actor, target, rawDamage, resistWIS, true
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
	switch mg := core.SkillMinigameFor(g.Battle.PendingSkill); mg {
	case core.MinigameCharge, core.MinigameOvercharge:
		// Both charge-family bars share the hold/release flow and pre-arm
		// gating; only the constructor (and its resolve/overload band) differ.
		// Charge gets a longer pre-arm pause so the player has time to read
		// the prompt; pressing the input during the intro skips straight
		// into the bar (handled in updateAttackTiming). ChargeNeedsRelease
		// blocks the very same Enter the player used to confirm the
		// target from being read as engaging the charge — they must
		// release once first, then a fresh press engages.
		if mg == core.MinigameOvercharge {
			g.Battle.Timing = core.NewOverchargeState(core.ChargeTimingDuration)
		} else {
			g.Battle.Timing = core.NewChargeState(core.ChargeTimingDuration)
		}
		intro = core.ChargeTimingIntro
		g.Battle.ChargeNeedsRelease = true
	case core.MinigameSequence:
		g.Battle.Timing = core.NewSequenceState(g.Rand(), core.SequenceTimingDuration, core.SequenceLength)
		// Clear analog-stick edge memory so a player whose stick happens to
		// be tilted when the bar arms doesn't get a phantom input on frame 1.
		input.ResetStickEdges()
	case core.MinigameReels:
		// Slot gamble — stop each reel with a press (no directional input,
		// so no stick-edge reset needed).
		g.Battle.Timing = core.NewReelState(g.Rand(), core.ReelTimingDuration)
	case core.MinigameRecall:
		// Memory pattern — directional taps after the reveal hides, same as
		// the sequence bar, so clear stick edges too.
		g.Battle.Timing = core.NewRecallState(g.Rand(), core.RecallTimingDuration, core.RecallPatternLength, core.RecallRevealTime)
		input.ResetStickEdges()
	default:
		// Swipe gets a two-zone press bar (one hit zone per half of the
		// sweep) so its AoE swing reads as a wider commitment than a
		// single-target Attack. Every other press-minigame skill keeps the
		// classic one-zone bar.
		if g.Battle.PendingSkill == core.SkillSwipe {
			// Swipe is a two-hit tally bar: one window around the middle
			// and one just before the commit tail — a "wind up, then the
			// big swing" rhythm rather than two evenly-spread beats (the
			// centers live in core.SwipeHitFracs). Both hits land across
			// the whole AoE formation.
			g.Battle.Timing = core.NewTallyStateAtCenters(core.AttackTimingDuration, core.SwipeHitFracs...)
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
			// Log it (not setBattleStatus) — the timing bar arms the same
			// frame and would overwrite the transient prompt slot, so the
			// player would never see the "wrong target" notice on a
			// charge/sequence skill. The combat log persists.
			setBattleMessage(g, fmt.Sprintf("%s is confused — wrong target!", g.Party[actor].Name))
		}
	case core.ActionPartyTarget, core.ActionItemTarget:
		// Heal-skill target picker AND the item target picker both land on an
		// ally, so a confused fumble re-rolls among the living party for both.
		slots := core.AvailablePartyTargets(g.Party)
		if len(slots) == 0 {
			return
		}
		picked := slots[rng.Intn(len(slots))]
		if picked != g.Battle.PartyTarget {
			g.Battle.PartyTarget = picked
			setBattleMessage(g, fmt.Sprintf("%s is confused — wrong ally!", g.Party[actor].Name))
		}
	}
}

// applyPendingAction is invoked once the timing bar resolves. It runs the
// recorded action's effect with the resulting quality and records the actor's
// quality popup if (and only if) the action actually landed. Phase advancement
// happens inside the apply* path via finishActorTurn.
func applyPendingAction(g *core.GameState, quality int) {
	handler, ok := actionHandlerFor(g.Battle.PendingSkill)
	if !ok {
		setBattleStatus(g, "No skill ready.")
		return
	}
	// Capture the acting member BEFORE apply: handler.apply runs
	// finishActorTurn → beginPartyTurn synchronously,
	// which advances g.Battle.CurrentParty to the NEXT actor. Reading it
	// after apply would stamp the quality popup over whoever acts next
	// (when two party members are back-to-back in the queue) instead of
	// the member whose timing this popup describes.
	actor := g.Battle.CurrentParty
	if landed := handler.apply(g, quality); landed {
		recordQuality(g, quality, actor, false)
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
// for QualityResultDuration. isBlock sets g.Battle.LastQualityIsBlock, which
// the renderer reads to pick the defend palette + the "BLOCK!" label (that
// label/palette override lives in render/timing.go, not here). Single source
// of truth for both attack- and block-side quality popups so the field set
// never drifts between callers.
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
	if !core.MemberAttackHitsTarget(g.Rand(), *attacker, target, quality) {
		// Whiff log keeps the quality prefix so the line reads consistently
		// with hits ("Excellent! Warrior hits for 8." vs "Excellent! Warrior
		// swings wide."). The popup over the actor still says "Excellent!"
		// because the *timing* graded that way — accuracy is a separate roll
		// layered on top. A melee swing at a flyer reads "can't reach" so the
		// elevated miss rate is legible as the flying penalty, not bad luck.
		whiff := fmt.Sprintf("%s%s swings wide.", qualityTag(quality), attacker.Name)
		if core.EnemyInfoFor(target).Flying && !core.MemberMeleeReachesFlyer(*attacker) {
			whiff = fmt.Sprintf("%s%s can't reach the airborne %s.", qualityTag(quality), attacker.Name, core.EnemySingularNoun(target))
		}
		setBattleMessage(g, whiff)
		finishActorTurn(g)
		return true
	}
	// Defender dodge: a connecting swing can still be sidestepped by a
	// nimble enemy. Symmetric with the party-side dodge in
	// resolveEnemyAttacker. Skills are NOT dodgeable (mirrors
	// MeleeAccuracy's basic-attack-only gate).
	if core.RollDodge(g.Rand(), core.EffectiveEnemyStats(target)) {
		setBattleMessage(g, fmt.Sprintf("%s%s lunges but the %s slips aside.", qualityTag(quality), attacker.Name, core.EnemySingularNoun(target)))
		finishActorTurn(g)
		return true
	}
	// Basic Attack: weapon-stat + 0, scaled by timing quality. The
	// governing stat tracks the equipped weapon (STR heavy / DEX light +
	// ranged) via MemberAttackDamage. Physically tagged so the armor damp
	// applies — basic attacks are the canonical phys swing against an
	// armored foe (amoeba teaches the lesson). The dealt number returned
	// by damageEnemy is the POST-armor figure; the combat log uses it so
	// what the player reads matches the HP delta (an Excellent vs an
	// Amoeba prints "hits for 4", not the 12 we computed before armor).
	rawDamage := core.ScaleDamage(core.MemberAttackDamage(*attacker, 0), quality)
	crit, _ := rollSkillCrit(g, attacker, core.SkillNone, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	dealt, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagPhys)
	// Glyph/particle keyed to the weapon: an unarmed fist or a blunt club/hammer
	// (and ranged strikes) reads as a percussive impact; an edged weapon reads as
	// a slash. Basic attacks are the only weapon-driven swing — skills pick their
	// VFX via vfxKindFor.
	core.EnqueueEnemyVFX(g, core.WeaponHitVFX(core.EquippedWeapon(*attacker)), g.Battle.EnemyIndex)
	setBattleMessage(g, appendCrit(attackResultMessage(attacker.Name, target, dealt, quality, defeated), crit))
	finishActorTurn(g)
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
	anyHit := false
	for p := 0; p < passes; p++ {
		hit := applyAoEDamage(g, core.SkillSwipe, damage, quality, false)
		if hit > 0 {
			anyHit = true
		}
		if p == 0 {
			enemiesHit = hit
		}
	}
	if anyHit {
		// One big camera punch per Swipe, not one per tally pass.
		core.TriggerCombatShake(&g.Battle, core.CombatShakeBigPeak, core.CombatShakeBigDur)
	}
	if enemiesHit == 0 || passes == 0 {
		setBattleMessage(g, aoeEmptyMessage("Swipe", "catches only air"))
	} else {
		setBattleMessage(g, appendCrit(swipeMessage(actor.Name, enemiesHit, quality), crit))
	}
	finishActorTurn(g)
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

// setupTargetedAllyAndPay is the single-ally counterpart of
// setupTargetedEnemyAndPay: confirm a LIVING party member is targeted, then
// deduct the skill's MP. A dead / unselected ally refuses WITHOUT spending MP
// (the player is being asked to re-pick, not to burn the cast). The
// no-selection message is shared; deadMsg is the per-skill refusal for a downed
// target (Prayer "cannot revive", Cleanse/Renewal "can't reach the fallen") so
// a future single-ally heal can't drift on the validation shape.
func setupTargetedAllyAndPay(g *core.GameState, skill core.SkillID, label, deadMsg string) bool {
	if g.Battle.PartyTarget < 0 || g.Battle.PartyTarget >= len(g.Party) {
		setBattleStatus(g, "No ally selected.")
		return false
	}
	if g.Party[g.Battle.PartyTarget].HP <= 0 {
		setBattleStatus(g, deadMsg)
		return false
	}
	return chargeMP(g, skill, label)
}

// --- Prayer (Cleric, heals an ally) ---

func setupPrayer(g *core.GameState) bool {
	return setupTargetedAllyAndPay(g, core.SkillPrayer, "Prayer", "Prayer cannot revive.")
}

func applyPrayer(g *core.GameState, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	// Heal formula is dispatched by skill Kind in core.SkillHeal; Prayer's
	// Kind is Heal so this resolves to WIS + Effect.Heal.
	heal := core.ScaleHeal(core.SkillHealFor(actor, core.SkillPrayer), quality)
	target := &g.Party[g.Battle.PartyTarget]
	healPartyMember(g, g.Battle.PartyTarget, heal)
	core.EnqueuePartyVFX(g, vfxKindFor(core.SkillPrayer), g.Battle.PartyTarget)
	selfTarget := g.Battle.PartyTarget == g.Battle.CurrentParty
	setBattleMessage(g, prayerMessage(actor.Name, target.Name, heal, quality, selfTarget))
	finishActorTurn(g)
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
	if enemy.Item == core.ItemNone {
		setBattleMessage(g, "There is nothing to steal.")
		finishActorTurn(g)
		// The thief's hand still moved — popup with the quality reads as
		// "graded an empty grab" which is fine.
		return true
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillSteal)
	// Steal chance: flat base (no DEX scaling) with the timing-quality
	// multiplier on top, capped at 1.0 so a perfect Excellent still rolls.
	chance := core.QualityScaledChance(core.StealChance(effect.StealChance), quality)
	if g.Rand().Float64() < chance {
		kind := enemy.Item
		// Add the stolen kind to the inventory and clear it off the enemy
		// so the same foe can't be looted twice. Guarded by the
		// ItemNone check above, so kind is always a real item here.
		enemy.Item = core.ItemNone
		g.Inventory = core.AddItem(g.Inventory, kind, 1)
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
		core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillSteal), g.Battle.EnemyIndex)
		msg := stealMessage(actor.Name, kind, quality)
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
	finishActorTurn(g)
	return true
}

// setupScan validates the enemy target and commits Scan's MP cost (mirrors
// setupFirebolt's setup→pay flow). A target that dies between confirm and
// apply is refunded by ensureAliveTargetOrCancel in applyScan.
func setupScan(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillScan, "Scan")
}

// applyScan identifies the target's KIND in the bestiary
// (Bestiary.MarkScanned) — the shortcut to the 5-kills-to-identify
// threshold. Once identified, that kind's exact HP renders in the battle
// roster (here and in every future encounter) and the bestiary tab marks
// it known. No damage, no status, and no chance roll — the ID always
// lands; the timing grade is cosmetic.
func applyScan(g *core.GameState, quality int) bool {
	// setupScan committed the MP; the shared head refunds it if the target
	// died before apply.
	if !ensureAliveTargetOrCancel(g, core.SkillScan) {
		return false
	}
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	g.Bestiary.MarkScanned(enemy.Kind)
	core.EnqueueEnemyVFX(g, core.VFXScan, g.Battle.EnemyIndex)
	setBattleMessage(g, fmt.Sprintf("%s scans the %s — %d/%d HP. Identified.",
		actor.Name, core.EnemySingularNoun(*enemy), enemy.HP, enemy.MaxHP))
	finishActorTurn(g)
	return true
}

func setupCripple(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillCripple, "Cripple")
}

// applyCripple stamps the SPD debuff onto the targeted enemy — the first
// enemy-side debuff. No damage and (like Bless / Scan) the timing grade is
// cosmetic, so there's no proc roll: the negative-SPD BuffStats and BuffTurns
// (tier-folded via EffectiveSkillEffect) land on the enemy's BuffStats /
// BuffTurns, where EffectiveEnemyStats folds them into its ATB turn-rate while
// the counter runs. Re-casting overwrites rather than stacking, matching the
// no-stack rule every other status follows.
func applyCripple(g *core.GameState, quality int) bool {
	// setupCripple committed the MP; the shared head refunds it if the target
	// died before apply.
	if !ensureAliveTargetOrCancel(g, core.SkillCripple) {
		return false
	}
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	effect := core.EffectiveSkillEffect(actor, core.SkillCripple)
	core.StampEnemyDebuff(enemy, core.SkillCripple, effect)
	core.EnqueueEnemyVFX(g, core.VFXVenom, g.Battle.EnemyIndex)
	setBattleMessage(g, fmt.Sprintf("%s cripples the %s — slowed for %d turns.",
		actor.Name, core.EnemySingularNoun(*enemy), effect.BuffTurns))
	finishActorTurn(g)
	return true
}

func setupFrostbite(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillFrostbite, "Frostbite")
}

// applyFrostbite deals INT-scaled frost damage and, on a surviving target,
// ALWAYS chills it — stamping the SPD debuff (the enemy BuffStats mirror) the
// same way Cripple does. The chill is guaranteed (no proc roll; timing only
// scales the damage), so it's the "damage + debuff" counterpart to Cripple's
// pure-utility slow. Re-cast overwrites per the no-stack rule. resistWIS is
// unused — the chill isn't WIS-resistible (matches Cripple/Bless).
func applyFrostbite(g *core.GameState, quality int) bool {
	actor, target, rawDamage, _, ok := beginSingleTargetSkill(g, core.SkillFrostbite, quality)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillFrostbite)
	crit, _ := rollSkillCrit(g, actor, core.SkillFrostbite, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(core.SkillFrostbite))
	core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillFrostbite), g.Battle.EnemyIndex)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	// The chill lands only on a surviving target — no point debuffing a corpse
	// (BuffTurns would dangle until clearEnemyStatusesOnDeath). Guaranteed when
	// alive, so `chilled` == survived for the message arm.
	chilled := !defeated && core.StampEnemyDebuff(enemy, core.SkillFrostbite, effect)
	setBattleMessage(g, appendCrit(frostbiteMessage(actor.Name, target, damage, quality, defeated, chilled), crit))
	finishActorTurn(g)
	return true
}

func frostbiteMessage(name string, target core.Enemy, damage, quality int, defeated, chilled bool) string {
	return procSkillMessage(frostbiteArms, name, target, damage, quality, defeated, chilled)
}

func setupCorrosiveVial(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillCorrosiveVial, "Corrosive Vial")
}

// applyCorrosiveVial breaks the target enemy's Armor for the rest of the battle
// — a permanent strip (floored at 0), distinct from the turn-counted BuffStats
// debuffs: it mutates the live per-instance Enemy.Armor that the damageEnemy
// phys-mitigation chain reads, so every subsequent phys hit lands harder.
// Re-casting strips further (stacking down to 0). No damage; timing is cosmetic.
func applyCorrosiveVial(g *core.GameState, quality int) bool {
	// setupCorrosiveVial committed the MP; the shared head refunds it if the
	// target died before apply.
	if !ensureAliveTargetOrCancel(g, core.SkillCorrosiveVial) {
		return false
	}
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	effect := core.EffectiveSkillEffect(actor, core.SkillCorrosiveVial)
	before := enemy.Armor
	enemy.Armor -= effect.ArmorReduction
	if enemy.Armor < 0 {
		enemy.Armor = 0
	}
	core.EnqueueEnemyVFX(g, core.VFXVenom, g.Battle.EnemyIndex)
	if stripped := before - enemy.Armor; stripped > 0 {
		setBattleMessage(g, fmt.Sprintf("%s's vial eats %d Armor off the %s.", actor.Name, stripped, core.EnemySingularNoun(*enemy)))
	} else {
		setBattleMessage(g, fmt.Sprintf("%s's vial splashes the %s — no armor left to break.", actor.Name, core.EnemySingularNoun(*enemy)))
	}
	finishActorTurn(g)
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
	actor, target, rawDamage, resistWIS, ok := beginSingleTargetSkill(g, core.SkillFirebolt, quality)
	if !ok {
		return false
	}
	// Overcharge: a release past the peak (Overcharge minigame) overloads the
	// bolt — flat bonus damage on top of the guaranteed Excellent grade. The
	// recoil it costs is applied after the hit lands, below.
	overloaded := g.Battle.Timing.Overloaded
	if overloaded {
		rawDamage += core.OverchargeDamageBonus
	}
	// Effect is pulled separately for the burn-chance roll below.
	effect := core.EffectiveSkillEffect(actor, core.SkillFirebolt)
	crit, _ := rollSkillCrit(g, actor, core.SkillFirebolt, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	// Firebolt is Magic-tagged so dealt == rawDamage in practice;
	// using the return keeps the log honest if a future Tag change
	// brings armor back into play.
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(core.SkillFirebolt))
	core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillFirebolt), g.Battle.EnemyIndex)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	burned := tryProcStatus(g.Rand(), &enemy.BurnTurns, defeated, effect.BurnChance, quality, 0, effect.BurnDuration, resistWIS)
	setBattleMessage(g, appendCrit(fireboltMessage(actor.Name, target, damage, quality, defeated, burned, enemy.BurnTurns), crit))
	if overloaded {
		// The overcharged bolt recoils on the caster. SkillTagNone so the
		// self-burn bypasses the caster's own armor/MDef (it's not an attack
		// to be mitigated), and damagePartyMember handles the death/status
		// cleanup if it drops them.
		recoil, _ := damagePartyMember(g, g.Battle.CurrentParty, core.OverchargeRecoil, core.SkillTagNone)
		core.EnqueuePartyVFX(g, core.VFXEmber, g.Battle.CurrentParty)
		setBattleMessage(g, fmt.Sprintf("%s overcharges the bolt — and is scorched for %d!", actor.Name, recoil))
	}
	finishActorTurn(g)
	return true
}

// --- Crushing Blow (Warrior, charge phys hit with Stun proc on Great+) ---

func setupCrushingBlow(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillCrushingBlow, "Crushing Blow")
}

func applyCrushingBlow(g *core.GameState, quality int) bool {
	actor, target, rawDamage, resistWIS, ok := beginSingleTargetSkill(g, core.SkillCrushingBlow, quality)
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
		rawDamage *= core.TierDamageDoubler
	}
	crit, _ := rollSkillCrit(g, actor, core.SkillCrushingBlow, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(core.SkillCrushingBlow))
	core.EnqueueEnemyVFX(g, core.VFXSlash, g.Battle.EnemyIndex)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	stunned := tryProcStatus(g.Rand(), &enemy.StunTurns, defeated, effect.StunChance, quality, core.TimingQualityGreat, effect.StunDuration, resistWIS)
	setBattleMessage(g, appendCrit(crushingBlowMessage(actor.Name, target, damage, quality, defeated, stunned), crit))
	finishActorTurn(g)
	return true
}

// --- Whirlwind (Warrior, charge AoE phys) ---

func setupWhirlwind(g *core.GameState) bool {
	return chargeMP(g, core.SkillWhirlwind, "Whirlwind")
}

func applyWhirlwind(g *core.GameState, quality int) bool {
	// SkillWhirlwind carries no Burn/Poison, so applyAoEStatusSkill's per-target
	// status rolls short-circuit (chance 0) — a pure AoE damage cast. Shares the
	// one AoE body so the damage/crit/shake/log path can't drift.
	return applyAoEStatusSkill(g, core.SkillWhirlwind, "Whirlwind", "hits", "catches only air", quality)
}

// --- Mass Mend (Cleric, charge AoE heal) ---

func setupMassMend(g *core.GameState) bool {
	return chargeMP(g, core.SkillMassMend, "Mass Mend")
}

func applyMassMend(g *core.GameState, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	heal := core.ScaleHeal(core.SkillHealFor(actor, core.SkillMassMend), quality)
	// Tally the wounds + queue per-member VFX on the PRE-heal HP, then apply
	// the heal through the shared core.HealWholeParty so this and the
	// out-of-battle Mass Mend (explore/panels_use.go) can't drift on the
	// heal rule. HealWholeParty/HealMember no-op the dead/ingested and clamp
	// at MaxHP; the HP<MaxHP check here is only the "counts as a mend" tally.
	healed := 0
	for _, i := range core.AvailablePartyTargets(g.Party) {
		if g.Party[i].HP < g.Party[i].MaxHP {
			healed++
		}
		core.EnqueuePartyVFX(g, vfxKindFor(core.SkillMassMend), i)
	}
	core.HealWholeParty(g, heal)
	if healed == 0 {
		setBattleMessage(g, fmt.Sprintf("%s%s's Mass Mend finds no wounds.", qualityTag(quality), actor.Name))
	} else {
		setBattleMessage(g, fmt.Sprintf("%s%s mends %d allies for %d each.", qualityTag(quality), actor.Name, healed, heal))
	}
	finishActorTurn(g)
	return true
}

// --- Bless (Cleric, press party-wide stat buff) ---

func setupBless(g *core.GameState) bool {
	// No target gate: Bless always hits the whole living party, so the only
	// setup step is committing the MP (mirrors Mass Mend's chargeMP setup).
	return chargeMP(g, core.SkillBless, "Bless")
}

// applyBless stamps the tier-folded stat buff on every living, non-ingested
// party member (the caster included). The buff always lands — like Scan, the
// timing grade is cosmetic — so there's no proc roll. EffectiveSkillEffect
// folds the Conviction tree's purchased tiers (magnitude + duration) into the
// base BuffStats / BuffTurns before they're written, and a re-cast replaces
// the existing buff rather than stacking (matching the no-stack rule every
// other status follows).
func applyBless(g *core.GameState, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	effect := core.EffectiveSkillEffect(actor, core.SkillBless)
	blessed := stampPartyWideBuff(g, effect, core.SkillBless)
	// Report the EFFECTIVE per-stat boost (effect.BuffStats.STR — all four
	// buffed stats share the same magnitude), not the base constant, so the
	// log line stays honest once Conviction-tree tiers raise it.
	setBattleMessage(g, fmt.Sprintf("%s%s blesses %d allies (+%d stats, %d turns).",
		qualityTag(quality), actor.Name, blessed, effect.BuffStats.STR, effect.BuffTurns))
	finishActorTurn(g)
	return true
}

// --- Smite (Cleric, press-tap magic damage) ---

func setupSmite(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillSmite, "Smite")
}

func applySmite(g *core.GameState, quality int) bool {
	actor, target, rawDamage, resistWIS, ok := beginSingleTargetSkill(g, core.SkillSmite, quality)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillSmite)
	crit, _ := rollSkillCrit(g, actor, core.SkillSmite, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(core.SkillSmite))
	core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillSmite), g.Battle.EnemyIndex)
	// Smite T3 ("+25% stun") gives the base-stun-less skill a stun
	// proc on Great+ timing. effect.StunChance is 0 at tier 0..2,
	// so tryProcStatus short-circuits cleanly until the tier is
	// purchased — no behavior change for un-upgraded clerics.
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	stunned := tryProcStatus(g.Rand(), &enemy.StunTurns, defeated, effect.StunChance, quality, core.TimingQualityGreat, effect.StunDuration, resistWIS)
	setBattleMessage(g, appendCrit(smiteMessage(actor.Name, target, damage, quality, defeated, stunned), crit))
	finishActorTurn(g)
	return true
}

// --- Backstab (Thief, charge phys with crit on Excellent) ---

func setupBackstab(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillBackstab, "Backstab")
}

func applyBackstab(g *core.GameState, quality int) bool {
	actor, target, rawDamage, _, ok := beginSingleTargetSkill(g, core.SkillBackstab, quality)
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
	finishActorTurn(g)
	return true
}

// dotStrike describes a "phys hit that procs a single-target DoT" skill — the
// shared shape behind Venom Strike (Poison), Rend, and Lacerate (Bleed). The
// closures select which enemy counter the DoT lands on and which SkillEffect
// fields drive its chance / duration, so applyDoTStrike stays status-agnostic;
// `arms` narrates the hit. Mirrors the table-driven style of enemyStatusPillVisuals.
type dotStrike struct {
	counter func(*core.Enemy) *int
	chance  func(core.SkillEffect) float64
	dur     func(core.SkillEffect) func(*rand.Rand) int
	arms    procMessageArms
}

var venomStrikeDoT = dotStrike{
	counter: func(e *core.Enemy) *int { return &e.PoisonTurns },
	chance:  func(eff core.SkillEffect) float64 { return eff.PoisonChance },
	dur:     func(eff core.SkillEffect) func(*rand.Rand) int { return eff.PoisonDuration },
	arms:    venomStrikeArms,
}

// bleedDoT builds the Bleed descriptor for Rend / Lacerate — same counter and
// effect fields, only the narration (arms) differs per skill.
func bleedDoT(arms procMessageArms) dotStrike {
	return dotStrike{
		counter: func(e *core.Enemy) *int { return &e.BleedTurns },
		chance:  func(eff core.SkillEffect) float64 { return eff.BleedChance },
		dur:     func(eff core.SkillEffect) func(*rand.Rand) int { return eff.BleedDuration },
		arms:    arms,
	}
}

// applyDoTStrike is the single body for every phys-hit-plus-DoT skill: a
// stat-scaled hit that, on a surviving target, rolls the DoT apply (dot.chance,
// quality-scaled) onto dot.counter via the shared tryProcStatus (inheriting the
// no-stack + WIS-shorten rules), then narrates via dot.arms.
func applyDoTStrike(g *core.GameState, skill core.SkillID, quality int, dot dotStrike) bool {
	actor, target, rawDamage, resistWIS, ok := beginSingleTargetSkill(g, skill, quality)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, skill)
	crit, _ := rollSkillCrit(g, actor, skill, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(skill))
	core.EnqueueEnemyVFX(g, vfxKindFor(skill), g.Battle.EnemyIndex)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	procced := tryProcStatus(g.Rand(), dot.counter(enemy), defeated, dot.chance(effect), quality, 0, dot.dur(effect), resistWIS)
	setBattleMessage(g, appendCrit(procSkillMessage(dot.arms, actor.Name, target, damage, quality, defeated, procced), crit))
	finishActorTurn(g)
	return true
}

// --- Venom Strike (Thief, sequence phys + Poison apply) ---

func setupVenomStrike(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillVenomStrike, "Venom Strike")
}

func applyVenomStrike(g *core.GameState, quality int) bool {
	return applyDoTStrike(g, core.SkillVenomStrike, quality, venomStrikeDoT)
}

// --- Rend (Warrior) / Lacerate (Thief): phys hit + Bleed DoT apply ---

func setupRend(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillRend, "Rend")
}

func applyRend(g *core.GameState, quality int) bool {
	return applyDoTStrike(g, core.SkillRend, quality, bleedDoT(rendArms))
}

func setupLacerate(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillLacerate, "Lacerate")
}

func applyLacerate(g *core.GameState, quality int) bool {
	return applyDoTStrike(g, core.SkillLacerate, quality, bleedDoT(lacerateArms))
}

// --- Frost Lance (Wizard, charge magic with reliable Stun on Great+) ---

// --- Frost Lance (Wizard, charge magic with reliable Stun on Great+) ---

func setupFrostLance(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillFrostLance, "Frost Lance")
}

func applyFrostLance(g *core.GameState, quality int) bool {
	actor, target, rawDamage, resistWIS, ok := beginSingleTargetSkill(g, core.SkillFrostLance, quality)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillFrostLance)
	crit, _ := rollSkillCrit(g, actor, core.SkillFrostLance, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(core.SkillFrostLance))
	core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillFrostLance), g.Battle.EnemyIndex)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	// FrostLance is flavored as a freeze but reads from the canonical
	// StunTurns counter — there's no separate "frozen" status today,
	// only the timing-gate that turns Stun-on-Great into a near-
	// guaranteed lock. The variable is `stunned` to match the field
	// it queries (any future grep for StunTurns lands here cleanly);
	// the player-facing log line keeps the "Frozen!" flavor via
	// frostLanceMessage.
	stunned := tryProcStatus(g.Rand(), &enemy.StunTurns, defeated, effect.StunChance, quality, core.TimingQualityGreat, effect.StunDuration, resistWIS)
	setBattleMessage(g, appendCrit(frostLanceMessage(actor.Name, target, damage, quality, defeated, stunned), crit))
	finishActorTurn(g)
	return true
}

// --- Arc Bolt (Wizard, sequence-tap AoE magic) ---

func setupArcBolt(g *core.GameState) bool {
	return chargeMP(g, core.SkillArcBolt, "Arc Bolt")
}

func applyArcBolt(g *core.GameState, quality int) bool {
	// Routed through applyAoEStatusSkill (not the old status-free AoE body) so
	// Arc Bolt's T3 "+15% Burn" delta actually procs per arc target — the
	// previous applyAoEDamage path silently dropped it. T0-T2 carry BurnChance
	// 0, so the per-target roll short-circuits and they're unchanged.
	return applyAoEStatusSkill(g, core.SkillArcBolt, "Arc Bolt", "arcs across", "dissipates with no target", quality)
}

// --- AoE skills (shared body; per-target status when the skill carries it) ---

// applyAoEStatusSkill is the single body for every whole-pack AoE skill
// (Whirlwind, Arc Bolt, Fireball, Poison Cloud). It bumps the actor,
// quality-scales the damage with one crit roll for the whole sweep, hits every
// living enemy, and — per target — rolls the skill's Burn / Poison from the
// tier-augmented effect against that enemy's WIS (the same tryProcStatus the
// single-target casts use). A skill with no Burn/Poison (Whirlwind) has
// chance 0, so the roll short-circuits and it's pure damage; only the status
// the skill actually carries can proc. The lower-level applyAoEDamage remains
// for the multi-PASS Swipe (which loops it per press).
func applyAoEStatusSkill(g *core.GameState, skill core.SkillID, skillNoun, hitVerb, emptyVerb string, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	effect := core.EffectiveSkillEffect(actor, skill)
	damage := scaleSkillDamage(actor, skill, quality)
	crit, _ := rollSkillCrit(g, actor, skill, quality)
	damage = applyCritMultiplier(damage, crit, false)
	tag := core.SkillTagFor(skill)
	vfx := vfxKindFor(skill)
	hits := 0
	afflicted := 0
	forEachLivingEnemy(g, func(slot int, enemy *core.Enemy) {
		_, defeated := damageEnemy(g, slot, damage, quality, tag)
		core.EnqueueEnemyVFX(g, vfx, slot)
		hits++
		resistWIS := core.EffectiveEnemyStats(*enemy).WIS
		if tryProcStatus(g.Rand(), &enemy.BurnTurns, defeated, effect.BurnChance, quality, 0, effect.BurnDuration, resistWIS) {
			afflicted++
		}
		if tryProcStatus(g.Rand(), &enemy.PoisonTurns, defeated, effect.PoisonChance, quality, 0, effect.PoisonDuration, resistWIS) {
			afflicted++
		}
		// AoE stat debuff (Cone of Cold's chill) — the multi-target mirror of
		// Frostbite's single-target chill. Guaranteed on a surviving target (no
		// proc roll), stamped via the enemy BuffStats debuff. Skills carrying no
		// buff (Whirlwind / Arc Bolt / Fireball / Poison Cloud) have BuffTurns 0
		// and skip it, so this stays inert for them.
		if !defeated && core.StampEnemyDebuff(enemy, skill, effect) {
			afflicted++
		}
	})
	if hits == 0 {
		setBattleMessage(g, aoeEmptyMessage(skillNoun, emptyVerb))
		finishActorTurn(g)
		return true
	}
	// AoE casts earn the big camera punch (mirrors applyAoEDamage's shake=true).
	core.TriggerCombatShake(&g.Battle, core.CombatShakeBigPeak, core.CombatShakeBigDur)
	msg := appendCrit(aoeSkillMessage(actor.Name, skillNoun, hitVerb, hits, damage, quality), crit)
	if afflicted > 0 {
		msg = fmt.Sprintf("%s %d afflicted.", msg, afflicted)
	}
	setBattleMessage(g, msg)
	finishActorTurn(g)
	return true
}

// --- Fireball (Wizard, charge AoE fire + per-target Burn) ---

func setupFireball(g *core.GameState) bool {
	return chargeMP(g, core.SkillFireball, "Fireball")
}

func applyFireball(g *core.GameState, quality int) bool {
	return applyAoEStatusSkill(g, core.SkillFireball, "Fireball", "engulfs", "fizzles with no target", quality)
}

func setupConeOfCold(g *core.GameState) bool {
	return chargeMP(g, core.SkillConeOfCold, "Cone of Cold")
}

// applyConeOfCold is the AoE Frostbite: applyAoEStatusSkill deals the frost
// sweep and stamps the guaranteed per-target SPD chill (the BuffTurns>0 branch
// in that shared body) on every surviving enemy.
func applyConeOfCold(g *core.GameState, quality int) bool {
	return applyAoEStatusSkill(g, core.SkillConeOfCold, "Cone of Cold", "sweeps over", "billows with no target", quality)
}

// --- Sunder (Warrior, charge phys + ATB shove) ---

func setupSunder(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillSunder, "Sunder")
}

// applySunder deals STR-scaled phys damage and, on a surviving target, shoves its
// ATB readiness gauge back (effect.ATBPush) so its next turn lands later — the
// offensive counterpart to Cripple's persistent SPD debuff: a one-shot tempo
// swing, not a status. No shove on a kill (the dead slot's gauge is moot).
func applySunder(g *core.GameState, quality int) bool {
	actor, target, rawDamage, _, ok := beginSingleTargetSkill(g, core.SkillSunder, quality)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillSunder)
	crit, _ := rollSkillCrit(g, actor, core.SkillSunder, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(core.SkillSunder))
	core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillSunder), g.Battle.EnemyIndex)
	shoved := !defeated && pushEnemyReadiness(g, g.Battle.EnemyIndex, effect.ATBPush)
	msg := fmt.Sprintf("%s%s sunders the %s for %d.", qualityTag(quality), actor.Name, core.EnemySingularNoun(target), damage)
	switch {
	case defeated:
		msg = fmt.Sprintf("%s%s sunders the %s for %d — it falls.", qualityTag(quality), actor.Name, core.EnemySingularNoun(target), damage)
	case shoved:
		msg = fmt.Sprintf("%s Its turn is shoved back.", msg)
	}
	setBattleMessage(g, appendCrit(msg, crit))
	finishActorTurn(g)
	return true
}

// --- Taunt (Warrior, press forced-target pull) ---

func setupTaunt(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillTaunt, "Taunt")
}

// applyTaunt forces the target enemy to attack the casting Warrior on its next
// turn (Enemy.TauntedBy / TauntTurns, honored by pickEnemyAttackTarget). No
// damage; the pull always lands (timing grade cosmetic). Re-casting overwrites.
func applyTaunt(g *core.GameState, quality int) bool {
	if !ensureAliveTargetOrCancel(g, core.SkillTaunt) {
		return false
	}
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	enemy.TauntedBy = g.Battle.CurrentParty
	enemy.TauntTurns = core.TauntTurns
	core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillTaunt), g.Battle.EnemyIndex)
	setBattleMessage(g, fmt.Sprintf("%s taunts the %s — it turns its glare on them.",
		actor.Name, core.EnemySingularNoun(*enemy)))
	finishActorTurn(g)
	return true
}

// --- War Banner (Warrior, press party-wide STR/VIT rally) ---

func setupWarBanner(g *core.GameState) bool {
	return chargeMP(g, core.SkillWarBanner, "War Banner")
}

func applyWarBanner(g *core.GameState, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	effect := core.EffectiveSkillEffect(actor, core.SkillWarBanner)
	rallied := stampPartyWideBuff(g, effect, core.SkillWarBanner)
	setBattleMessage(g, fmt.Sprintf("%s%s plants a war banner — %d allies rally (+%d STR, +%d Armor, %d turns).",
		qualityTag(quality), actor.Name, rallied, effect.BuffStats.STR, effect.BuffArmor, effect.BuffTurns))
	finishActorTurn(g)
	return true
}

// --- Stone Skin (Warrior, press single-ally Armor/MDef ward) ---

func setupStoneSkin(g *core.GameState) bool {
	return setupTargetedAllyAndPay(g, core.SkillStoneSkin, "Stone Skin", "Stone Skin can't reach the fallen.")
}

func applyStoneSkin(g *core.GameState, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	effect := core.EffectiveSkillEffect(actor, core.SkillStoneSkin)
	target := &g.Party[g.Battle.PartyTarget]
	eff := effect
	// If the caster warded themselves, finishActorTurn ticks their buff down
	// immediately — grant one extra so they get the full intended duration (same
	// correction the party-wide case documents).
	if g.Battle.PartyTarget == g.Battle.CurrentParty {
		eff.BuffTurns++
	}
	core.StampPartyBuff(target, core.SkillStoneSkin, eff)
	core.EnqueuePartyVFX(g, vfxKindFor(core.SkillStoneSkin), g.Battle.PartyTarget)
	setBattleMessage(g, fmt.Sprintf("%s%s wards %s in stone (+%d Armor, +%d MDef, %d turns).",
		qualityTag(quality), actor.Name, target.Name, effect.BuffArmor, effect.BuffMDef, effect.BuffTurns))
	finishActorTurn(g)
	return true
}

// --- Blind (Cleric, press enemy accuracy debuff) ---

func setupBlind(g *core.GameState) bool {
	return setupTargetedEnemyAndPay(g, core.SkillBlind, "Blind")
}

// applyBlind saps the target's DEX (the stat EnemyHitChance reads) via the enemy
// debuff mirror so it whiffs more often — the DEX-flavored sibling of Cripple. No
// damage; the debuff always lands (timing cosmetic); re-casting overwrites.
func applyBlind(g *core.GameState, quality int) bool {
	if !ensureAliveTargetOrCancel(g, core.SkillBlind) {
		return false
	}
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	effect := core.EffectiveSkillEffect(actor, core.SkillBlind)
	core.StampEnemyDebuff(enemy, core.SkillBlind, effect)
	core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillBlind), g.Battle.EnemyIndex)
	setBattleMessage(g, fmt.Sprintf("%s blinds the %s — its aim falters for %d turns.",
		actor.Name, core.EnemySingularNoun(*enemy), effect.BuffTurns))
	finishActorTurn(g)
	return true
}

// --- Aegis (Cleric, press single-ally absorb shield) ---

func setupAegis(g *core.GameState) bool {
	return setupTargetedAllyAndPay(g, core.SkillAegis, "Aegis", "Aegis can't reach the fallen.")
}

// applyAegis grants the ally a damage-absorbing shield (ShieldHP) that the party
// damage path spends before HP. Not turn-counted (no caster-tick correction
// needed); re-casting replaces the pool rather than stacking.
func applyAegis(g *core.GameState, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	effect := core.EffectiveSkillEffect(actor, core.SkillAegis)
	target := &g.Party[g.Battle.PartyTarget]
	target.ShieldHP = effect.ShieldHP
	core.EnqueuePartyVFX(g, vfxKindFor(core.SkillAegis), g.Battle.PartyTarget)
	setBattleMessage(g, fmt.Sprintf("%s%s raises an aegis over %s — absorbs the next %d damage.",
		qualityTag(quality), actor.Name, target.Name, target.ShieldHP))
	finishActorTurn(g)
	return true
}

// --- Smoke Bomb (Thief, press party evasion + enemy accuracy loss) ---

func setupSmokeBomb(g *core.GameState) bool {
	return chargeMP(g, core.SkillSmokeBomb, "Smoke Bomb")
}

// applySmokeBomb buffs the whole party's DEX (evasion) and saps every living
// enemy's DEX (accuracy) by the SAME magnitude — one number drives both sides.
// The party side rides the shared buff bundle; the enemy side mirrors it (negated)
// onto each foe's debuff. Both overwrite any existing buff/debuff (no-stack rule).
func applySmokeBomb(g *core.GameState, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	effect := core.EffectiveSkillEffect(actor, core.SkillSmokeBomb)
	buffed := stampPartyWideBuff(g, effect, core.SkillSmokeBomb)
	enemyDebuff := core.SkillEffect{BuffStats: core.Stats{DEX: -effect.BuffStats.DEX}, BuffTurns: effect.BuffTurns}
	blinded := 0
	forEachLivingEnemy(g, func(_ int, enemy *core.Enemy) {
		if core.StampEnemyDebuff(enemy, core.SkillSmokeBomb, enemyDebuff) {
			blinded++
		}
	})
	setBattleMessage(g, fmt.Sprintf("%s%s drops a smoke bomb — %d allies gain evasion, %d foes lose their aim.",
		qualityTag(quality), actor.Name, buffed, blinded))
	finishActorTurn(g)
	return true
}

// --- Ice Armor (Wizard, charge self frost ward) ---

func setupIceArmor(g *core.GameState) bool {
	return chargeMP(g, core.SkillIceArmor, "Ice Armor")
}

// applyIceArmor sheathes the caster in frost: while IceArmorTurns runs they gain
// MDef (EffectiveMDef) and chill any enemy that lands a basic attack on them
// (resolveEnemyAttacker). Self-only; grant +1 turn to offset finishActorTurn's
// immediate end-of-turn tick so the caster nets the full duration (Bless rule).
func applyIceArmor(g *core.GameState, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	effect := core.EffectiveSkillEffect(actor, core.SkillIceArmor)
	actor.IceArmorTurns = effect.IceArmorTurns + 1
	core.EnqueuePartyVFX(g, vfxKindFor(core.SkillIceArmor), g.Battle.CurrentParty)
	setBattleMessage(g, fmt.Sprintf("%s%s sheathes in ice — +%d MDef, attackers chilled for %d turns.",
		qualityTag(quality), actor.Name, core.IceArmorMDef, effect.IceArmorTurns))
	finishActorTurn(g)
	return true
}

// stampPartyWideBuff stamps the tier-folded party buff on every living,
// non-ingested member (caster included) via core.StampPartyBuff, granting the
// caster one extra turn of duration to offset finishActorTurn's immediate
// end-of-turn tick (the correction Bless documents). Queues the buff VFX per
// member and returns the count buffed. Shared by Bless / War Banner / Smoke Bomb.
func stampPartyWideBuff(g *core.GameState, effect core.SkillEffect, skill core.SkillID) int {
	buffed := 0
	for _, i := range core.AvailablePartyTargets(g.Party) {
		eff := effect
		// The caster acts NOW, so finishActorTurn's end-of-turn drain ticks THEIR
		// buff down one before they act again — grant them one extra turn so every
		// member nets the full intended duration. Re-casting refreshes, never compounds.
		if i == g.Battle.CurrentParty {
			eff.BuffTurns++
		}
		core.StampPartyBuff(&g.Party[i], skill, eff)
		core.EnqueuePartyVFX(g, vfxKindFor(skill), i)
		buffed++
	}
	return buffed
}

// tickIceArmorAfterPartyTurn drains the Ice Armor ward at the end of the warded
// member's own turn — same non-damaging seam as Bless / Renewal. The MDef bonus
// and chill-on-hit stay live while IceArmorTurns > 0; this only counts it down.
func tickIceArmorAfterPartyTurn(g *core.GameState, actor core.ActorRef) {
	tickPartyStatusCounter(g, actor, func(m *core.PartyMember) *int { return &m.IceArmorTurns }, "%s's ice armor melts.")
}

// tickEnemyTauntAfterTurn drains a taunt at the end of the taunted enemy's own
// turn — so a "attack the Warrior next turn" pull lasts exactly its window. The
// enemy-side mirror of the buff drains; no-ops on party actors and untaunted foes.
func tickEnemyTauntAfterTurn(g *core.GameState, actor core.ActorRef) {
	if actor.IsParty {
		return
	}
	enemy := core.BattleMemberAt(g, actor.Index)
	if enemy == nil || !enemy.Alive || enemy.TauntTurns <= 0 {
		return
	}
	enemy.TauntTurns--
}

// forcedTauntTarget returns the party slot the CURRENTLY-ATTACKING enemy
// (g.Battle.EnemyAttacker) is taunted onto, if the pull is live AND the taunter is
// still a reachable target (alive, not ingested). ok=false falls the caller back
// to normal round-robin targeting — a dead / swallowed / lapsed taunt is ignored.
func forcedTauntTarget(g *core.GameState) (int, bool) {
	enemy := core.BattleMemberAt(g, g.Battle.EnemyAttacker)
	if enemy == nil || enemy.TauntTurns <= 0 {
		return -1, false
	}
	t := enemy.TauntedBy
	if t < 0 || t >= len(g.Party) {
		return -1, false
	}
	if g.Party[t].HP <= 0 || g.Party[t].Ingested {
		return -1, false
	}
	return t, true
}

// --- Poison Cloud (Thief, sequence AoE toxin + per-target Poison) ---

func setupPoisonCloud(g *core.GameState) bool {
	return chargeMP(g, core.SkillPoisonCloud, "Poison Cloud")
}

func applyPoisonCloud(g *core.GameState, quality int) bool {
	return applyAoEStatusSkill(g, core.SkillPoisonCloud, "Poison Cloud", "blankets", "disperses with no target", quality)
}

// --- Cleanse (Cleric, press single-ally status cure) ---

func setupCleanse(g *core.GameState) bool {
	return setupTargetedAllyAndPay(g, core.SkillCleanse, "Cleanse", "Cleanse can't reach the fallen.")
}

func applyCleanse(g *core.GameState, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	target := &g.Party[g.Battle.PartyTarget]
	cured := core.CureDebuffs(target)
	core.EnqueuePartyVFX(g, vfxKindFor(core.SkillCleanse), g.Battle.PartyTarget)
	if cured == 0 {
		setBattleMessage(g, fmt.Sprintf("%s%s cleanses %s — nothing ailed them.", qualityTag(quality), actor.Name, target.Name))
	} else {
		setBattleMessage(g, fmt.Sprintf("%s%s cleanses %s — %d cured.", qualityTag(quality), actor.Name, target.Name, cured))
	}
	finishActorTurn(g)
	return true
}

// --- Second Wind (Warrior, charge flat self-heal) ---

func setupSecondWind(g *core.GameState) bool {
	// No target pick (ActionMenu, self-heal) — just commit the MP.
	return chargeMP(g, core.SkillSecondWind, "Second Wind")
}

func applySecondWind(g *core.GameState, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	// SkillHealFor on a Utility-kind skill returns the flat Effect.Heal (no WIS
	// scaling); ScaleHeal applies the timing quality on top.
	heal := core.ScaleHeal(core.SkillHealFor(actor, core.SkillSecondWind), quality)
	if healPartyMember(g, g.Battle.CurrentParty, heal) {
		core.EnqueuePartyVFX(g, vfxKindFor(core.SkillSecondWind), g.Battle.CurrentParty)
		setBattleMessage(g, fmt.Sprintf("%s%s catches a second wind — recovers %d HP.", qualityTag(quality), actor.Name, heal))
	} else {
		setBattleMessage(g, fmt.Sprintf("%s%s is already at full health.", qualityTag(quality), actor.Name))
	}
	finishActorTurn(g)
	return true
}

// --- Renewal (Cleric, charge heal-over-time on one ally) ---

func setupRenewal(g *core.GameState) bool {
	return setupTargetedAllyAndPay(g, core.SkillRenewal, "Renewal", "Renewal can't reach the fallen.")
}

func applyRenewal(g *core.GameState, quality int) bool {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	effect := core.EffectiveSkillEffect(actor, core.SkillRenewal)
	// Snapshot the per-turn heal at cast: Renewal is Heal-kind so SkillHealFor
	// folds the caster's WIS, and ScaleHeal folds the timing quality. Floored
	// at 1 so a regen always does something. RegenTurns is the (tier-augmented)
	// fixed duration. Re-casting replaces the existing regen (no stacking).
	perTurn := core.ScaleHeal(core.SkillHealFor(actor, core.SkillRenewal), quality)
	if perTurn < 1 {
		perTurn = 1
	}
	target := &g.Party[g.Battle.PartyTarget]
	target.RegenPerTurn = perTurn
	target.RegenTurns = effect.RegenTurns
	core.EnqueuePartyVFX(g, vfxKindFor(core.SkillRenewal), g.Battle.PartyTarget)
	setBattleMessage(g, fmt.Sprintf("%s%s lays a renewal on %s — +%d HP at the end of their next %d turns.",
		qualityTag(quality), actor.Name, target.Name, perTurn, effect.RegenTurns))
	finishActorTurn(g)
	return true
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
		core.TriggerCombatShake(&g.Battle, core.CombatShakeBigPeak, core.CombatShakeBigDur)
		return
	}
	crit = core.RollCrit(g.Rand(), core.EffectiveStats(*actor), quality)
	if crit {
		// Crits are the "big hit" moment — punch the camera harder than a
		// plain well-timed press (overrides the subtle base shake).
		core.TriggerCombatShake(&g.Battle, core.CombatShakeBigPeak, core.CombatShakeBigDur)
	}
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
		out *= core.TierDamageDoubler
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

// mitigateDamage runs the canonical two-stage mitigation chain — armor
// clip for phys-tagged hits, then magic-defense clip for magic-tagged
// hits — and clamps the result at 0. Heal / Buff / None tags pass
// straight through both stages. Both damage helpers (damageEnemy and
// damagePartyMember) share it so the phys-then-magic order and the
// floor-at-0 contract can't drift between the enemy and party sides;
// each side still supplies its own armor / mdef (enemy reads the raw
// EnemyDefinition fields, party reads the Effective* gear-folded values).
func mitigateDamage(raw int, tag core.SkillTag, armor, mdef int) int {
	d := core.ApplyArmor(raw, tag, armor)
	d = core.ApplyMagicDefense(d, tag, mdef)
	if d < 0 {
		d = 0
	}
	return d
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
	// Phys clips through Armor, magic clips through MDef (symmetric with
	// the party-side path). Most enemies carry MDef 0 today — only the
	// wizard-flavored kinds (Wisp, Goblin Mage, Necromancer) and the
	// Stone Golem authored a non-zero value, so player Firebolt against
	// unarmored grunts still feels the same. The clamp-at-0 inside
	// mitigateDamage keeps a future caller from accidentally healing an
	// enemy by passing a signed stat delta.
	damage := mitigateDamage(rawDamage, tag, enemy.Armor, core.EnemyInfoFor(*enemy).MDef)
	// Flash + HP-floor (shared with the party path + the poison tick) and
	// the real-hit recoil/wake reaction (shared with the party path).
	died := core.ApplyFlatDamage(&enemy.HP, &enemy.DamageFlash, damage)
	if damage > 0 {
		enemy.DamagePopup = damage
		enemy.DamagePopupQuality = quality
		enemy.DamagePopupTimer = core.QualityResultDuration
	}
	// Receiver recoil + wake-from-sleep — only on real damage (armor-
	// shrugged 1s still recoil; pure zero-damage connections don't).
	core.ApplyHitRecoil(&enemy.HitKnockback, &enemy.SleepTurns, damage)
	if !died {
		// Audible "thud" only on hits that actually scored. Zero-damage
		// connections (e.g. Swipe with a 0 base) stay silent so the bar
		// doesn't tick a sound on every empty swing.
		if damage > 0 {
			audio.Play(audio.SoundEnemyHit)
		}
		return damage, false
	}
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
// poisonTickMessage formats the end-of-turn poison-tick combat-log line
// for either side — `subject` is the pre-resolved actor name (party) or
// "The <noun>" (enemy), so the two ticks share one format instead of
// two near-identical fmt.Sprintf pairs.
// dotTickMessage is the shared "<subject> took a DoT tick" line: the fatalLine
// (one %s) when the tick killed, else the sufferFmt (%s + %d dealt). Poison and
// Bleed differ only in their two format strings.
func dotTickMessage(subject string, dealt int, fatal bool, fatalLine, sufferFmt string) string {
	if fatal {
		return fmt.Sprintf(fatalLine, subject)
	}
	return fmt.Sprintf(sufferFmt, subject, dealt)
}

func poisonTickMessage(subject string, dealt int, fatal bool) string {
	return dotTickMessage(subject, dealt, fatal, "%s succumbs to the poison.", "%s suffers %d from poison.")
}

func bleedTickMessage(subject string, dealt int, fatal bool) string {
	return dotTickMessage(subject, dealt, fatal, "%s bleeds out.", "%s bleeds for %d.")
}

func applyPartyPoisonTick(g *core.GameState, index int) bool {
	member := &g.Party[index]
	member.PoisonTurns--
	// damagePartyMemberPoison returns true on the fatal hit; use it as the
	// authoritative kill signal so a future "save at 1 HP" mechanic can't desync
	// from the message we emit here. NOTE: it deliberately bypasses the ingested
	// lockout in damagePartyMember — tickPoisonForIngestedParty fires this for
	// ingested members, and poison is designed to keep ticking on ingested prey
	// (otherwise ingest would silently zero the DoT and become a free escape).
	dealt, killed := damagePartyMemberPoison(g, index)
	setBattleMessage(g, poisonTickMessage(member.Name, dealt, killed))
	return killed
}

// applyEnemyDoTTick is the enemy-side damaging-tick seam: drain the given
// status counter by one and deal tickDamage as a Magic-tagged,
// Good-quality hit (Magic so armor doesn't damp the DoT; Good gives the
// orange "fire/venom" popup tint damageEnemy stamps). Returns
// (dealt, defeated) so each DoT caller formats its own succumbs / suffers
// line and does any post-kill bookkeeping (Burn repoints the player's
// target cursor). Mirrors applyPartyPoisonTick on the enemy side; the
// caller owns the up-front guard since it reads a status-specific counter.
func applyEnemyDoTTick(g *core.GameState, index int, counter *int, tickDamage int) (int, bool) {
	*counter--
	return damageEnemy(g, index, tickDamage, core.TimingQualityGood, core.SkillTagMagic)
}

// tickPoisonForIngestedParty applies one tick of poison to every ingested
// party member whose PoisonTurns counter is still active. Ingested members
// are skipped from the per-turn queue (buildTurnQueue), so their normal
// end-of-turn Poison tick never fires — which would silently turn ingest
// into a free pause of the DoT. Fire this once per round from beginNewRound
// (before the loss gate) so a poison kill while ingested still routes
// through ActivePartyCount and triggers the loss check.
//
// No double-tick: the round's turn queue is built once at beginNewRound
// (skipping ingested members) and never rebuilt mid-round, and
// ReleaseIngestedBy only clears the Ingested flag — it does not re-queue the
// freed member. So a member ingested at round start is drained here exactly
// once and can't also reach an end-of-turn tick the same round; if released
// mid-round it rejoins the queue (and the end-of-turn tick) only at the NEXT
// beginNewRound, where it's no longer ingested and so isn't drained here.
func tickPoisonForIngestedParty(g *core.GameState) {
	for i := range g.Party {
		m := &g.Party[i]
		if !m.Ingested || m.HP <= 0 || m.PoisonTurns <= 0 {
			continue
		}
		applyPartyPoisonTick(g, i)
	}
}

// tickWebbedAfterPartyTurn drains the Webbed counter at the end of the
// webbed member's own turn. Same shape as the Poison tick — actor-kind
// dispatch up front, party-only today (no party skill applies Webbed
// to enemies). Emits a short log line when the status wears off so
// the player sees the counter clear. No damage tied to Webbed;
// the slow / Ingest-refusal effect lives in actorSpeed +
// handleEnemyIngest.
func tickWebbedAfterPartyTurn(g *core.GameState, actor core.ActorRef) {
	tickPartyStatusCounter(g, actor, func(m *core.PartyMember) *int { return &m.WebbedTurns }, "%s tears free of the webs.")
}

// tickConfusedAfterPartyTurn mirrors tickWebbedAfterPartyTurn for the
// Confused status. The per-action retarget roll is honored at action
// resolution time (see action handlers' confuse-retarget path); this
// helper just drains the counter.
func tickConfusedAfterPartyTurn(g *core.GameState, actor core.ActorRef) {
	tickPartyStatusCounter(g, actor, func(m *core.PartyMember) *int { return &m.ConfusedTurns }, "%s's head clears.")
}

// tickBlessAfterPartyTurn drains the Bless buff counter at the end of the
// blessed member's own turn — same non-damaging seam as Webbed / Confused, so
// a buff lasts the recipient's BuffTurns turns. The stat boost stays live
// while BuffTurns > 0 (EffectiveStats reads it); this only counts it down. The
// per-stat magnitude (BuffStats) is left intact while the counter runs and
// zeroed wholesale on battle exit by ClearPartyTransientStatuses — leaving the
// stale magnitude here is harmless because EffectiveStats gates entirely on
// BuffTurns > 0.
// tickBlessAfterPartyTurn drains every stackable buff on the member at the end
// of their own turn, dropping the expired ones and narrating each fade — the
// stackable replacement for the old single Bless counter (so a member holding
// Bless + War Banner + Stone Skin ticks all three independently).
func tickBlessAfterPartyTurn(g *core.GameState, actor core.ActorRef) {
	if !actor.ValidPartyIndex(g.Party) {
		return
	}
	m := &g.Party[actor.Index]
	if m.HP <= 0 || len(m.Buffs) == 0 {
		return
	}
	remaining, expired := core.TickStatusMods(m.Buffs)
	m.Buffs = remaining
	for _, s := range expired {
		setBattleMessage(g, fmt.Sprintf("%s's %s fades.", m.Name, core.SkillName(s)))
	}
}

// tickEnemyBuffAfterTurn drains every stackable debuff on the enemy at the end
// of its own turn, dropping the expired ones — the enemy-side mirror of
// tickBlessAfterPartyTurn (so an enemy under Cripple + Blind ticks both
// independently). EffectiveEnemyStats folds the summed deltas while they run.
// No-ops on party actors and on a dead enemy (its turn-end housekeeping may run
// after a poison tick killed it this same step). One "shakes off" line when the
// last debuff clears keeps the log from spamming a fade per stat.
func tickEnemyBuffAfterTurn(g *core.GameState, actor core.ActorRef) {
	if actor.IsParty {
		return
	}
	enemy := core.BattleMemberAt(g, actor.Index)
	if enemy == nil || !enemy.Alive || len(enemy.Debuffs) == 0 {
		return
	}
	remaining, expired := core.TickStatusMods(enemy.Debuffs)
	enemy.Debuffs = remaining
	// Narrate per expired SOURCE (one line per skill), mirroring the party side's
	// tickBlessAfterPartyTurn. TickStatusMods returns expired sources (skills),
	// not per-stat rows, so this is one line per debuff — not spam. The old
	// "shakes off the affliction" line only fired when the LAST debuff cleared,
	// silently dropping feedback when one of several stacked debuffs expired.
	for _, s := range expired {
		setBattleMessage(g, fmt.Sprintf("%s's %s wears off.", core.TheEnemy(core.EnemyInfoFor(*enemy)), core.SkillName(s)))
	}
}

// tickRegenAfterPartyTurn applies one tick of the Renewal heal-over-time at the
// END of the regenerating member's own turn — the positive mirror of
// tickPoisonAfterPartyTurn. It can't use tickPartyStatusCounter (that seam only
// drains a counter, it doesn't heal), so it inlines the same actor-kind / HP /
// counter guards and routes the heal through healPartyMember (which clamps at
// MaxHP, flashes, and no-ops a dead/ingested member). Party-only — no enemy
// applies regen.
func tickRegenAfterPartyTurn(g *core.GameState, actor core.ActorRef) {
	if !actor.ValidPartyIndex(g.Party) {
		return
	}
	m := &g.Party[actor.Index]
	if m.HP <= 0 || m.RegenTurns <= 0 {
		return
	}
	m.RegenTurns--
	healed := healPartyMember(g, actor.Index, m.RegenPerTurn)
	if healed {
		core.EnqueuePartyVFX(g, core.VFXHeal, actor.Index)
	}
	// Log honestly: report the HP only when a heal actually landed (a tick on a
	// full-HP member heals nothing), and always announce the fade on the final
	// tick — including when that last tick itself healed.
	fades := m.RegenTurns == 0
	switch {
	case healed && fades:
		setBattleMessage(g, fmt.Sprintf("%s renews %d HP — the renewal fades.", m.Name, m.RegenPerTurn))
	case healed:
		setBattleMessage(g, fmt.Sprintf("%s renews %d HP.", m.Name, m.RegenPerTurn))
	case fades:
		setBattleMessage(g, fmt.Sprintf("%s's renewal fades.", m.Name))
	}
}

// tickPartyStatusCounter is the shared body the non-damaging
// end-of-party-turn status ticks (Webbed, Confused) walk. Each ticker
// used to inline the same actor-kind dispatch + index bounds +
// HP/counter guard. counterRef returns a pointer into the member's
// field so the helper can both read and decrement without a
// type-specific switch. clearedFmt is a "%s" template — when the
// counter hits zero, the helper formats with the member's name and
// emits the cleared message. Pass "" for a silent clear.
//
// This is the NON-damaging seam (counter drain + optional cleared
// message). The damaging ticks (Poison, Burn) keep their own functions
// because they also deal damage and return a kill signal, and they share
// a different seam for that: applyPartyPoisonTick on the party side and
// applyEnemyDoTTick on the enemy side. Folding damage into this helper
// would pile the death/kill-signal machinery into a signature meant for
// the no-damage Webbed / Confused drains.
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

// tickEnemyDoTAfterTurn is the shared body for every enemy end-of-turn damaging
// DoT (Poison, Bleed): no-op on party actors / dead / un-afflicted enemies, else
// drain the selected counter by one, deal tickDamage (Magic-tagged via
// applyEnemyDoTTick so armor doesn't damp it), and narrate via msg. Returns true
// if the tick killed the enemy. Burn is NOT routed here — it ticks at turn-START.
func tickEnemyDoTAfterTurn(g *core.GameState, actor core.ActorRef, counterOf func(*core.Enemy) *int, tickDamage int, msg func(string, int, bool) string) bool {
	if actor.IsParty {
		return false
	}
	enemy := core.BattleMemberAt(g, actor.Index)
	if enemy == nil || !enemy.Alive {
		return false
	}
	counter := counterOf(enemy)
	if *counter <= 0 {
		return false
	}
	dealt, defeated := applyEnemyDoTTick(g, actor.Index, counter, tickDamage)
	setBattleMessage(g, msg(core.TheEnemy(core.EnemyInfoFor(*enemy)), dealt, defeated))
	return defeated
}

// tickPoisonAfterEnemyTurn / tickBleedAfterEnemyTurn are the per-status wrappers
// (kept as named entry points for tests). The Thief's Venom Strike applies
// Enemy.PoisonTurns; Rend / Lacerate apply the SEPARATE Enemy.BleedTurns so the
// two DoTs run at once.
func tickPoisonAfterEnemyTurn(g *core.GameState, actor core.ActorRef) bool {
	return tickEnemyDoTAfterTurn(g, actor, func(e *core.Enemy) *int { return &e.PoisonTurns }, core.PoisonTickDamage, poisonTickMessage)
}

func tickBleedAfterEnemyTurn(g *core.GameState, actor core.ActorRef) bool {
	return tickEnemyDoTAfterTurn(g, actor, func(e *core.Enemy) *int { return &e.BleedTurns }, core.BleedTickDamage, bleedTickMessage)
}

// tickEnemyEndOfTurnDoTs runs ALL of an enemy's end-of-turn damaging DoTs in one
// call — the single seam both turn-end paths (advanceSkippedTurn + finishActorTurn)
// invoke, so a new enemy DoT is added here ONCE instead of at both sites (where a
// missed site silently freezes the DoT during Sleep/Stun skips). Returns true if
// any tick killed the enemy; callers re-check win conditions regardless.
func tickEnemyEndOfTurnDoTs(g *core.GameState, actor core.ActorRef) bool {
	poisonKill := tickPoisonAfterEnemyTurn(g, actor)
	bleedKill := tickBleedAfterEnemyTurn(g, actor)
	return poisonKill || bleedKill
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
	// Use the post-mitigation dealt amount (burn is magic-tagged, so an
	// enemy's MDef clips it) for the log, so the "burns for N" line matches
	// the HP drop + damage popup — same contract the poison tick honors.
	// Logging the raw BurnTickDamage overstated the hit on MDef enemies
	// (Goblin Mage / Wisp / Necromancer / Stone Golem).
	dealt, _ := applyEnemyDoTTick(g, actor.Index, &enemy.BurnTurns, core.BurnTickDamage)
	def := core.EnemyInfoFor(*enemy)
	if !enemy.Alive {
		setBattleMessage(g, fmt.Sprintf("%s succumbs to the flames.", core.TheEnemy(def)))
		// Repoint the player's targeting cursor if the burn killed the
		// currently-selected enemy, so the next attack action has a valid one.
		repointEnemyCursorIfDead(g)
		return true
	}
	setBattleMessage(g, fmt.Sprintf("%s burns for %d.", core.TheEnemy(def), dealt))
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
	core.GainUpTo(&member.HP, member.MaxHP, amount)
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
	// Ingested prey is sealed off — no EXTERNAL damage reaches them while inside
	// the mantrap. Defense in depth: pickEnemyAttackTarget already routes around
	// ingested members, but any future damage source that picked by index would
	// otherwise bypass the lockout. The internal poison DoT is exempt — it ticks
	// on ingested prey via damagePartyMemberPoison (which skips this guard), so
	// ingest can't silently zero a standing poison.
	if member.Ingested {
		return 0, false
	}
	return applyPartyDamage(g, member, rawAmount, tag)
}

// applyPartyDamage mitigates + applies rawAmount to an ALREADY-validated, living
// party member (the caller owns the bounds / dead / lockout checks), running the
// shared flash / recoil / wake / rumble / death bookkeeping. Split out of
// damagePartyMember so the poison DoT — which by design keeps ticking on
// ingested prey — can reach this same bookkeeping without the ingest lockout.
func applyPartyDamage(g *core.GameState, member *core.PartyMember, rawAmount int, tag core.SkillTag) (int, bool) {
	// Mitigation reads through EffectiveDefenses (one equipment + buff walk for
	// both Armor and MDef) so any equipped gear bonuses stack on top of the base
	// values. Base member.Armor stays authored (0 today on the party side); items
	// add to it via ArmorBonus / MDefBonus on their ItemDefinition.
	armorVal, mdefVal := core.EffectiveDefenses(*member)
	amount := mitigateDamage(rawAmount, tag, armorVal, mdefVal)
	// Aegis shield soaks post-mitigation damage BEFORE HP: spend the pool first,
	// only the overflow reaches HP. A fully-absorbed hit reaches HP as 0 — so the
	// recoil / wake / DoT-proc bookkeeping below all read the post-shield `amount`
	// and a turned-aside blow flinches nothing.
	if amount > 0 && member.ShieldHP > 0 {
		absorbed := amount
		if absorbed > member.ShieldHP {
			absorbed = member.ShieldHP
		}
		member.ShieldHP -= absorbed
		amount -= absorbed
	}
	// Flash + HP-floor (shared with the enemy path + the poison tick).
	died := core.ApplyFlatDamage(&member.HP, &member.DamageFlash, amount)
	// Reactionary knockback + wake — only on real damage so a fully-
	// soaked hit doesn't visually shove a tank who took 0. The renderer
	// pushes the member toward the camera (away from the attacking enemy
	// formation) for HitKnockbackDuration.
	core.ApplyHitRecoil(&member.HitKnockback, &member.SleepTurns, amount)
	// Haptic "ouch" — buzz the pad when a hit actually lands. Gated on amount>0
	// like the recoil/wake above (a fully-soaked 0 doesn't buzz). Taking a hit
	// doesn't shake the camera, so this arms rumble directly (TriggerCombatShake
	// is for the player's own offensive impacts).
	if amount > 0 {
		core.TriggerRumble(&g.Battle, core.RumbleHurtStrength, core.RumbleHurtDur)
	}
	if !died {
		return amount, false
	}
	clearPartyStatusesOnDeath(member)
	return amount, true
}

// damagePartyMemberPoison applies one poison tick, bypassing the ingested
// lockout in damagePartyMember (poison keeps ticking on ingested prey —
// tickPoisonForIngestedParty) while still honoring the bounds + already-dead
// guards. Magic-tagged so MDef mitigates it, identical to the non-ingested
// end-of-turn tick that previously routed through damagePartyMember.
func damagePartyMemberPoison(g *core.GameState, partyIndex int) (int, bool) {
	if partyIndex < 0 || partyIndex >= len(g.Party) {
		return 0, false
	}
	member := &g.Party[partyIndex]
	if member.HP <= 0 {
		return 0, false
	}
	return applyPartyDamage(g, member, core.PoisonTickDamage, core.SkillTagMagic)
}

// damagePartyMemberDefendable applies an incoming HIT to a party member,
// honoring their Defend brace BEFORE mitigation — for enemy basic attacks
// and damaging casts (Firebolt, Stoneslam), anything the player can
// reasonably brace against. DoT ticks (poison) call damagePartyMember
// directly so the one-turn brace doesn't soak a status that ticks on the
// member's own turn. The brace floors at 1 when the pre-Defend amount was
// positive (a soak, not free immunity); a hit already zeroed upstream by a
// perfect timing block arrives as rawAmount<=0 and stays 0.
func damagePartyMemberDefendable(g *core.GameState, partyIndex, rawAmount int, tag core.SkillTag) (int, bool) {
	if rawAmount > 0 && partyIndex >= 0 && partyIndex < len(g.Party) && g.Party[partyIndex].Defending {
		rawAmount = int(float32(rawAmount) * core.DefendingDamageMult)
		if rawAmount < 1 {
			rawAmount = 1
		}
	}
	return damagePartyMember(g, partyIndex, rawAmount, tag)
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
// mage), Webbed (cave spider web), and Confused (will-o'-wisp). Poison is
// intentionally left so the corpse keeps its poison render hint while it
// fades; Burn has no player-applicable source yet. Add new timed statuses
// to whichever side can carry them so they can't linger on a corpse.
// Each side's death-clear set is ONE declared list of the timed-status counters
// that side carries — adding a new timed status is appending one pointer here,
// not hunting for scattered field assignments. (The sets differ by design: see
// the doc comment above for which statuses each actor can carry into death.)
func clearEnemyStatusesOnDeath(enemy *core.Enemy) {
	for _, c := range []*int{&enemy.BurnTurns, &enemy.SleepTurns, &enemy.PoisonTurns, &enemy.BleedTurns, &enemy.StunTurns} {
		*c = 0
	}
	enemy.Debuffs = nil
}

func clearPartyStatusesOnDeath(member *core.PartyMember) {
	for _, c := range []*int{&member.SleepTurns, &member.WebbedTurns, &member.ConfusedTurns} {
		*c = 0
	}
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

func stealMessage(name string, kind core.ItemKind, quality int) string {
	tag := qualityTag(quality)
	return fmt.Sprintf("%s%s steals %s.", tag, name, core.ItemInfo(kind).Name)
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
	frostbiteArms = procMessageArms{
		defeated: "%[1]s%[2]s's Frostbite freezes the %[3]s solid.",
		proc:     "%[1]s%[2]s bites the %[3]s for %[4]d and chills it.",
		// plain (alive, not chilled) is effectively unreachable — the chill is
		// guaranteed on survival — but kept for the procSkillMessage contract.
		plain: "%[1]s%[2]s bites the %[3]s for %[4]d.",
	}
	rendArms = procMessageArms{
		defeated: "%[1]s%[2]s's Rend tears the %[3]s apart.",
		proc:     "%[1]s%[2]s rends the %[3]s for %[4]d — it's bleeding.",
		plain:    "%[1]s%[2]s rends the %[3]s for %[4]d.",
	}
	lacerateArms = procMessageArms{
		defeated: "%[1]s%[2]s's Lacerate opens the %[3]s up for good.",
		proc:     "%[1]s%[2]s lacerates the %[3]s for %[4]d — it's bleeding.",
		plain:    "%[1]s%[2]s lacerates the %[3]s for %[4]d.",
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

// resolveEnemyMiss narrates a clean whiff — the accuracy roll in
// beginEnemyAttack failed, so no defend bar armed and no damage lands. It still
// commits the attack target through pickEnemyAttackTarget (advancing the
// round-robin cursor exactly as a connecting swing would, so targeting can't
// drift based on whether the enemy hit) and lunges the sprite so the miss
// reads. No damage, no status proc, no lifesteal.
func resolveEnemyMiss(g *core.GameState, slot int) {
	enemy := core.BattleMemberAt(g, slot)
	if enemy == nil || !enemy.Alive {
		return
	}
	target := pickEnemyAttackTarget(g)
	if target < 0 {
		return
	}
	enemy.AttackBump = core.BumpDuration
	setBattleMessage(g, fmt.Sprintf("The %s's attack misses %s!", core.EnemySingularNoun(*enemy), g.Party[target].Name))
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
	// NOT dodgeable today — mirrors MeleeAccuracy, which only gates
	// basic attacks.
	if core.RollDodge(g.Rand(), core.EffectiveStats(g.Party[target])) {
		recordQuality(g, defendQuality, target, true)
		setBattleMessage(g, fmt.Sprintf("%s sidesteps the %s.", g.Party[target].Name, core.EnemySingularNoun(*enemy)))
		return true
	}
	rawDamage := core.EnemyBasicDamage(*enemy)
	// Enemy crit on basic attacks — symmetric with the player side, but
	// no timing-grade bonus (enemies don't press a bar). Pure
	// DEX-driven CritChance via core.RollCrit at the Miss grade
	// keeps enemies on a flat ~5-10% crit floor where the player can
	// push 30%+ on Excellent.
	enemyCrit := core.RollCrit(g.Rand(), core.EffectiveEnemyStats(*enemy), core.TimingQualityMiss)
	rawDamage = applyCritMultiplier(rawDamage, enemyCrit, false)
	damage := core.ScaleIncomingDamage(rawDamage, defendQuality)
	// Plain enemy melee is physically tagged so the party's Armor field
	// (currently 0 for all members, future equipment) damps the bite.
	// Spell-casting enemies (goblin mage) dispatch through their own
	// resolver and pass SkillTagMagic where appropriate. The Defend brace
	// (and its floor-at-1 soak) is applied inside damagePartyMemberDefendable
	// — shared with the damaging enemy casts so Defend soaks every incoming
	// hit, not just melee. dealt is the post-armor figure used in the message.
	dealt, _ := damagePartyMemberDefendable(g, target, damage, core.SkillTagPhys)
	// Impact VFX only when damage actually landed — an Excellent
	// defend can clamp damage to 0, and the player just performed a
	// successful block. Spawning impact sparks on a perfect parry
	// would visually undersell the block. Enemy melee (claw/bite/slam)
	// reads as a percussive impact, not a bladed slash.
	if dealt > 0 {
		core.EnqueuePartyVFX(g, core.VFXImpact, target)
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
	// bite would trivialize the duration roll. Gate on `dealt` (the
	// post-armor/post-MDef figure), NOT the pre-mitigation `damage`, so a
	// bite fully soaked to 0 by armor inflicts no DoT — matching the
	// lifesteal block below which also reads `dealt`.
	if dealt > 0 {
		// Raw-chance proc (no timing minigame on the enemy side) through the
		// shared applyStatusRoll, which owns the alive / not-already-poisoned
		// guard and the WIS-shortened stamp — the same core the player-side
		// tryProcStatus uses with a quality-scaled chance.
		if applyStatusRoll(g.Rand(), &g.Party[target].PoisonTurns, g.Party[target].HP <= 0,
			def.PoisonChance, core.DefaultPoisonEffect.RollDuration, core.EffectiveStats(g.Party[target]).WIS) {
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
			// Cap against the per-instance MaxHP, not the definition's
			// base MaxHP — a future raised/scaled enemy with a non-
			// default per-instance ceiling would otherwise overheal
			// past its real cap or undercap a buffed one. Today both
			// values match because NewEnemy seeds from the same def.
			core.GainUpTo(&enemy.HP, enemy.MaxHP, heal)
			setBattleMessage(g, fmt.Sprintf("%s drains life from %s (+%d HP).", core.TheEnemy(def), g.Party[target].Name, heal))
		}
	}
	// Ice Armor reprisal: a connecting basic attack chills its attacker when the
	// struck defender is frost-warded (IceArmorTurns > 0) — an SPD debuff via the
	// enemy debuff mirror, the reactive sibling of Frostbite. Lands on contact
	// regardless of how much got through (even a Defended-to-0 hit "struck" the
	// ward); overwrites any existing enemy debuff (no-stack, like the chills).
	if g.Party[target].IceArmorTurns > 0 && enemy.HP > 0 {
		if core.StampEnemyDebuff(enemy, core.SkillIceArmor, core.SkillEffect{BuffStats: core.Stats{SPD: -core.IceArmorChillSPD}, BuffTurns: core.IceArmorChillTurns}) {
			setBattleMessage(g, fmt.Sprintf("%s's ice armor chills the %s.", g.Party[target].Name, core.EnemySingularNoun(*enemy)))
		}
	}
	return true
}

// pickEnemyAttackTarget cycles to the next living party member after the last
// one the enemy side targeted. Uses EnemyAttackCursor (separate from
// PartyTarget) so the player's heal/item ally cycling doesn't shift who
// enemies attack next.
func pickEnemyAttackTarget(g *core.GameState) int {
	// Taunt override: a live pull forces the acting enemy onto the taunter, as
	// long as that ally is still reachable. Falls through to the round-robin
	// otherwise (taunter died / got ingested / the taunt lapsed). The cursor is
	// still advanced to the forced slot so post-taunt targeting resumes from there.
	if forced, ok := forcedTauntTarget(g); ok {
		g.Battle.EnemyAttackCursor = forced
		return forced
	}
	target := core.PeekNextEnemyTarget(g)
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
