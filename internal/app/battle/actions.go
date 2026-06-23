package battle

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"fmt"
	"math/rand"
	"reflect"
)

// actionSetup runs validation/cost-payment and returns true when ready for the
// timing minigame; on false it sets a status message and stays in the action menu.
type actionSetup func(g *core.GameState) bool

// actionApply resolves the action at the given timing quality. Returns landed=true
// if it went through; false on a defensive no-op (e.g. target died between confirm
// and apply) so the caller doesn't show a quality popup for a cancelled action.
type actionApply func(g *core.GameState, quality int) (landed bool)

type actionHandlers struct {
	setup actionSetup
	apply actionApply
}

// partyIndexValid reports whether idx is in range. Callers layer their own
// HP / Ingested / amount checks on top.
func partyIndexValid(g *core.GameState, idx int) bool {
	return core.PartyIndexInRange(g.Party, idx)
}

// livingEnemyAt resolves slot to a live enemy pointer, ok=false when out of range
// or dead. The single "is this a living foe" accessor (enemy-side mirror of currentMember).
func livingEnemyAt(g *core.GameState, slot int) (*core.Enemy, bool) {
	enemy := core.BattleMemberAt(g, slot)
	if enemy == nil || !enemy.Alive {
		return nil, false
	}
	return enemy, true
}

// skillActionHandlers is the player-castable skill registry: each entry's
// setup/apply pair drives the timing minigame. Enemy-only skills route through
// resolveEnemySpell instead; an unregistered skill surfaces as "No skill ready."
var skillActionHandlers = map[core.SkillID]actionHandlers{
	core.SkillSwipe:         {setup: setupSwipe, apply: applySwipe},
	core.SkillPrayer:        {setup: targetedSetup(core.SkillPrayer), apply: applyPrayer},
	core.SkillSteal:         {setup: setupTargetedEnemy, apply: applySteal},
	core.SkillScan:          {setup: targetedSetup(core.SkillScan), apply: applyScan},
	core.SkillFirebolt:      {setup: targetedSetup(core.SkillFirebolt), apply: applyFirebolt},
	core.SkillCrushingBlow:  {setup: targetedSetup(core.SkillCrushingBlow), apply: applyCrushingBlow},
	core.SkillWhirlwind:     {setup: payOnlySetup(core.SkillWhirlwind), apply: applyWhirlwind},
	core.SkillMassMend:      {setup: payOnlySetup(core.SkillMassMend), apply: applyMassMend},
	core.SkillSmite:         {setup: targetedSetup(core.SkillSmite), apply: applySmite},
	core.SkillBackstab:      {setup: targetedSetup(core.SkillBackstab), apply: applyBackstab},
	core.SkillVenomStrike:   {setup: targetedSetup(core.SkillVenomStrike), apply: applyVenomStrike},
	core.SkillFrostLance:    {setup: targetedSetup(core.SkillFrostLance), apply: applyFrostLance},
	core.SkillArcBolt:       {setup: payOnlySetup(core.SkillArcBolt), apply: applyArcBolt},
	core.SkillBless:         {setup: payOnlySetup(core.SkillBless), apply: applyBless},
	core.SkillFireball:      {setup: payOnlySetup(core.SkillFireball), apply: applyFireball},
	core.SkillPoisonCloud:   {setup: payOnlySetup(core.SkillPoisonCloud), apply: applyPoisonCloud},
	core.SkillCleanse:       {setup: targetedSetup(core.SkillCleanse), apply: applyCleanse},
	core.SkillSecondWind:    {setup: payOnlySetup(core.SkillSecondWind), apply: applySecondWind},
	core.SkillRenewal:       {setup: targetedSetup(core.SkillRenewal), apply: applyRenewal},
	core.SkillCripple:       {setup: targetedSetup(core.SkillCripple), apply: applyCripple},
	core.SkillFrostbite:     {setup: targetedSetup(core.SkillFrostbite), apply: applyFrostbite},
	core.SkillCorrosiveVial: {setup: targetedSetup(core.SkillCorrosiveVial), apply: applyCorrosiveVial},
	core.SkillConeOfCold:    {setup: payOnlySetup(core.SkillConeOfCold), apply: applyConeOfCold},
	core.SkillSunder:        {setup: targetedSetup(core.SkillSunder), apply: applySunder},
	core.SkillTaunt:         {setup: targetedSetup(core.SkillTaunt), apply: applyTaunt},
	core.SkillWarBanner:     {setup: payOnlySetup(core.SkillWarBanner), apply: applyWarBanner},
	core.SkillStoneSkin:     {setup: targetedSetup(core.SkillStoneSkin), apply: applyStoneSkin},
	core.SkillBlind:         {setup: targetedSetup(core.SkillBlind), apply: applyBlind},
	core.SkillAegis:         {setup: targetedSetup(core.SkillAegis), apply: applyAegis},
	core.SkillSmokeBomb:     {setup: payOnlySetup(core.SkillSmokeBomb), apply: applySmokeBomb},
	core.SkillIceArmor:      {setup: payOnlySetup(core.SkillIceArmor), apply: applyIceArmor},
	core.SkillRend:          {setup: targetedSetup(core.SkillRend), apply: applyRend},
	core.SkillLacerate:      {setup: targetedSetup(core.SkillLacerate), apply: applyLacerate},
}

// init asserts the player-castable contract: every PlayerCastable skill has a
// skillActionHandlers entry (else a missing handler only surfaces at playtest as
// "No skill ready.").
func init() {
	for _, s := range core.PlayerCastableSkills() {
		if _, ok := skillActionHandlers[s]; !ok {
			panic("battle: PlayerCastable skill " + core.SkillName(s) + " has no skillActionHandlers entry")
		}
	}
	// Enemy-castable consistency, both directions: every EnemyCastable skill needs
	// an enemySpellHandlers entry, and the reverse walk below catches stale handlers.
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
	// Every targetedSetupConfig key must be a registered handler skill — a missing
	// config would resolve to the zero value (enemy-target), mislabeling an ally cast.
	for s := range targetedSetupConfig {
		if _, ok := skillActionHandlers[s]; !ok {
			panic("battle: targetedSetupConfig has a row for " + core.SkillName(s) + " with no skillActionHandlers entry")
		}
	}
}

// targetedSetupConfig drives the single-target setup gate for skills whose setup
// is purely "confirm a living enemy/ally, then pay MP." Per-skill data is the gate
// kind (enemy vs ally) and, for ally rows, the downed-target refusal line.
// deadMsg is meaningful only for ally-gated rows; enemy rows leave it "".
type targetedSetupKind struct {
	ally    bool
	deadMsg string
}

var targetedSetupConfig = map[core.SkillID]targetedSetupKind{
	// Enemy-targeted (setup → confirm live enemy → pay MP).
	core.SkillScan:          {},
	core.SkillFirebolt:      {},
	core.SkillCrushingBlow:  {},
	core.SkillSmite:         {},
	core.SkillBackstab:      {},
	core.SkillVenomStrike:   {},
	core.SkillFrostLance:    {},
	core.SkillCripple:       {},
	core.SkillFrostbite:     {},
	core.SkillCorrosiveVial: {},
	core.SkillSunder:        {},
	core.SkillTaunt:         {},
	core.SkillBlind:         {},
	core.SkillRend:          {},
	core.SkillLacerate:      {},
	// Ally-targeted, each with its downed-target refusal line.
	core.SkillPrayer:    {ally: true, deadMsg: "Prayer cannot revive."},
	core.SkillStoneSkin: {ally: true, deadMsg: "Stone Skin can't reach the fallen."},
	core.SkillAegis:     {ally: true, deadMsg: "Aegis can't reach the fallen."},
	core.SkillCleanse:   {ally: true, deadMsg: "Cleanse can't reach the fallen."},
	core.SkillRenewal:   {ally: true, deadMsg: "Renewal can't reach the fallen."},
}

// runTargetedSetup is the table-driven single-target gate: runs the enemy/ally
// check (per targetedSetupConfig) and pays MP. Setup commits the cost; the only
// refund path is target-death before apply, handled by ensureAlive*OrCancel.
func runTargetedSetup(g *core.GameState, skill core.SkillID) bool {
	cfg := targetedSetupConfig[skill]
	if cfg.ally {
		return setupTargetedAllyAndPay(g, skill, cfg.deadMsg)
	}
	return setupTargetedEnemyAndPay(g, skill)
}

// targetedSetup binds runTargetedSetup to one skill, producing its actionSetup.
func targetedSetup(skill core.SkillID) actionSetup {
	return func(g *core.GameState) bool { return runTargetedSetup(g, skill) }
}

// payOnlySetup is the no-target-gate setup: skills that hit the whole party / all
// enemies / the caster need no target pick, so setup just commits MP.
func payOnlySetup(skill core.SkillID) actionSetup {
	return func(g *core.GameState) bool { return chargeMP(g, skill) }
}

// setupPrayer / setupFirebolt / setupCleanse: named entry points for the unit tests;
// each delegates to the shared table gate.
func setupPrayer(g *core.GameState) bool   { return runTargetedSetup(g, core.SkillPrayer) }
func setupFirebolt(g *core.GameState) bool { return runTargetedSetup(g, core.SkillFirebolt) }
func setupCleanse(g *core.GameState) bool  { return runTargetedSetup(g, core.SkillCleanse) }

// enemySpellCtx bundles the pre-resolved state every enemy-spell handler needs.
// Built once by resolveEnemySpell so handlers don't re-derive the lookups.
type enemySpellCtx struct {
	g         *core.GameState
	slot      int
	target    int
	enemy     *core.Enemy
	def       core.EnemyDefinition
	skillName string
	effect    core.SkillEffect
}

// enemySpellHandlers is the dispatch table resolveEnemySpell walks (init guard
// asserts both directions against EnemyCastable). Each handler returns whether the
// cast FIRED — a cancelled cast returns false so it can't burn a PerBattleCastLimit charge.
var enemySpellHandlers = map[core.SkillID]func(enemySpellCtx) bool{
	core.SkillFirebolt:   handleEnemyFirebolt,
	core.SkillIngest:     handleEnemyIngest,
	core.SkillSleep:      handleEnemySleep,
	core.SkillWeb:        handleEnemyWeb,
	core.SkillConfuse:    handleEnemyConfuse,
	core.SkillStoneslam:  handleEnemyStoneslam,
	core.SkillRaiseBones: handleEnemyRaiseBones,
}

// enemySpellLog formats "<Enemy> casts <Skill> — <rest>" for handlers following
// the casts-prefix convention. Handlers with bespoke phrasings call setBattleMessage directly.
func enemySpellLog(ctx enemySpellCtx, rest string, args ...any) {
	tail := fmt.Sprintf(rest, args...)
	setBattleMessage(ctx.g, fmt.Sprintf("%s casts %s — %s", core.TheEnemy(ctx.def), ctx.skillName, tail))
}

// enemySpellDamage is the shared damaging-enemy-spell formula: SpellPower +
// effect.Damage, floored at 1. Used by Firebolt and Stoneslam.
func enemySpellDamage(def core.EnemyDefinition, effect core.SkillEffect) int {
	// Scale by the global difficulty dial (same seam as basic-attack / spawn HP),
	// then floor at 1 — the max forces a >=1 chip when the base is <= 0.
	return max(core.ScaleEnemyDifficulty(def.SpellPower+effect.Damage), 1)
}

// handleEnemyFirebolt applies the ranged magic cast (SpellPower + Effect.Damage).
func handleEnemyFirebolt(ctx enemySpellCtx) bool {
	g := ctx.g
	dealt, killed := damagePartyMemberDefendable(g, ctx.target, enemySpellDamage(ctx.def, ctx.effect), core.SkillTagFor(core.SkillFirebolt))
	core.EnqueuePartyVFX(g, vfxKindFor(core.SkillFirebolt), ctx.target)
	if killed {
		setBattleMessage(g, fmt.Sprintf("%s incinerates %s.", core.TheEnemy(ctx.def), g.Party[ctx.target].Name))
	} else {
		enemySpellLog(ctx, "%s burns for %d.", g.Party[ctx.target].Name, dealt)
	}
	audio.Play(audio.SoundEnemyHit)
	tryRetribution(g, ctx.slot, ctx.target, dealt)
	return true
}

// handleEnemyIngest is the mantrap signature: pulls the target out of combat until
// the mantrap dies. Sleep + Defending clear (the swallow wakes/unbraces); Poison
// persists so ingest isn't a free status escape.
func handleEnemyIngest(ctx enemySpellCtx) bool {
	g := ctx.g
	// Defensive re-check: the target can die between turns; cancel cleanly with a log line.
	picked := ctx.target
	if !core.PartyMemberAvailable(g.Party, picked) {
		picked = core.FirstAvailablePartyMember(g.Party)
	}
	if picked < 0 {
		setBattleMessage(g, fmt.Sprintf("%s lunges, but finds no one to seize.", core.TheEnemy(ctx.def)))
		return false
	}
	m := &g.Party[picked]
	// Webbed targets refuse Ingest — Webbed is "tempo control without removal," so
	// the web shields the prey. The mantrap bites instead this turn.
	if m.WebbedTurns > 0 {
		setBattleMessage(g, fmt.Sprintf("%s lunges, but %s is too tangled to swallow.", core.TheEnemy(ctx.def), m.Name))
		return false
	}
	m.Ingested = true
	m.IngestedBy = ctx.slot
	m.SleepTurns = 0
	m.Defending = false
	// VFX anchors at the MANTRAP (ctx.slot), not the prey: spawnIngest converges
	// motes inward, reading as "prey's essence flowing INTO the swallower."
	core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillIngest), ctx.slot)
	setBattleMessage(g, fmt.Sprintf("%s engulfs %s!", core.TheEnemy(ctx.def), m.Name))
	audio.Play(audio.SoundEnemyHit)
	return true
}

// applyEnemyStatus is the shared mechanical tail of the enemy status casts (Sleep
// / Webbed / Confused): floor the rolled duration, apply WIS-shortened to the
// counter, fire VFX + cue. Returns true when it landed. The validity check (in
// range, available) lives HERE with the deref so a caster can't stamp onto a
// corpse / ingested ally. Callers own per-status guards + log lines.
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

// handleEnemySleep applies the Sleep cast; already-asleep targets short-circuit.
func handleEnemySleep(ctx enemySpellCtx) bool {
	g := ctx.g
	// Bound the index before the deref (match Web/Confuse handlers).
	if !partyIndexValid(g, ctx.target) {
		return false
	}
	m := &g.Party[ctx.target]
	if m.HP <= 0 {
		return false
	}
	if m.SleepTurns > 0 {
		// Fired at a valid target, just already asleep — the enemy spent its action (true).
		enemySpellLog(ctx, "%s is already asleep.", m.Name)
		return true
	}
	if applyEnemyStatus(ctx, &m.SleepTurns, ctx.effect.SleepDuration(g.Rand()), core.SleepMinTurns, core.SkillSleep) {
		enemySpellLog(ctx, "%s falls asleep.", m.Name)
		return true
	}
	return false
}

// handleEnemyWeb applies the Webbed status; already-webbed targets short-circuit
// (no stacking). Index bounded before the deref (mirrors Sleep).
func handleEnemyWeb(ctx enemySpellCtx) bool {
	g := ctx.g
	if !partyIndexValid(g, ctx.target) {
		return false
	}
	m := &g.Party[ctx.target]
	if m.WebbedTurns > 0 {
		setBattleMessage(g, fmt.Sprintf("%s spins a fresh web at %s — already webbed.", core.TheEnemy(ctx.def), m.Name))
		return true
	}
	if applyEnemyStatus(ctx, &m.WebbedTurns, ctx.effect.BindDuration(g.Rand()), core.SpiderWebbedMinTurns, core.SkillWeb) {
		enemySpellLog(ctx, "%s is wrapped in sticky webs.", m.Name)
		return true
	}
	return false
}

// handleEnemyConfuse applies the Confused status; already-confused short-circuits.
// WIS resistance is in ShortenStatusDuration (no separate per-cast roll).
func handleEnemyConfuse(ctx enemySpellCtx) bool {
	g := ctx.g
	if !partyIndexValid(g, ctx.target) {
		return false
	}
	m := &g.Party[ctx.target]
	if m.ConfusedTurns > 0 {
		setBattleMessage(g, fmt.Sprintf("%s flickers at %s — already disoriented.", core.TheEnemy(ctx.def), m.Name))
		return true
	}
	if applyEnemyStatus(ctx, &m.ConfusedTurns, ctx.effect.ConfuseDuration(g.Rand()), core.WispConfuseMinTurns, core.SkillConfuse) {
		enemySpellLog(ctx, "%s grows confused.", m.Name)
		return true
	}
	return false
}

// handleEnemyStoneslam fires the AoE phys cast: hits every available party member
// for SpellPower + Effect.Damage (Phys-tagged, Defend-soakable). Pure damage.
func handleEnemyStoneslam(ctx enemySpellCtx) bool {
	g := ctx.g
	raw := enemySpellDamage(ctx.def, ctx.effect)
	hits := 0
	kills := 0
	for _, i := range core.AvailablePartyTargets(g.Party) {
		dealt, killed := damagePartyMemberDefendable(g, i, raw, core.SkillTagFor(core.SkillStoneslam))
		core.EnqueuePartyVFX(g, vfxKindFor(core.SkillStoneslam), i)
		hits++
		if killed {
			kills++
		}
		// Each warded member reflects its share; a reflect can drop the golem
		// mid-volley, after which tryRetribution no-ops.
		tryRetribution(g, ctx.slot, i, dealt)
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
	// No targets = connected with nothing; don't count it.
	return hits > 0
}

// handleEnemyRaiseBones is the Necromancer's add-summon: inserts a Skeleton into
// the active pack. The per-battle cast limit is enforced upstream by usableEnemySkills.
func handleEnemyRaiseBones(ctx enemySpellCtx) bool {
	g := ctx.g
	pack := core.ActivePack(g)
	if pack == nil {
		setBattleMessage(g, fmt.Sprintf("%s gestures, but the bones refuse to rise.", core.TheEnemy(ctx.def)))
		return false
	}
	skeleton := core.NewEnemy(core.EnemySkeleton)
	pack.Members = append(pack.Members, skeleton)
	// The skeleton enters the queue automatically — beginNewRound rebuilds it from
	// the expanded pack.Members, so it acts the round AFTER this cast.
	//
	// Re-fetch the caster AFTER the append: pack.Members is at exact capacity, so
	// the append reallocates and ctx.enemy now dangles. Re-apply the bump on the
	// LIVE pointer (resolveEnemySpell's pre-dispatch write landed on the orphaned copy).
	if caster := core.BattleMemberAt(g, ctx.slot); caster != nil {
		caster.AttackBump = core.BumpDuration
	}
	setBattleMessage(g, fmt.Sprintf("%s incants — a skeleton claws up from the ground!", core.TheEnemy(ctx.def)))
	audio.Play(audio.SoundInputHit)
	return true
}

// setupTargetedEnemy is the shared live-target check (basic attack + Steal):
// gates on EnemyIndex still pointing at a living pack member.
func setupTargetedEnemy(g *core.GameState) bool {
	if !core.BattleEnemyAlive(g, g.Battle.EnemyIndex) {
		setBattleStatus(g, msgNoTarget)
		return false
	}
	return true
}

// canAffordSkill reports whether actor has enough MP for `skill` — pure predicate.
// Shares the MP-cost rule with chargeMP (the deduct-time path).
func canAffordSkill(actor *core.PartyMember, skill core.SkillID) bool {
	return core.CanAffordSkill(actor, skill)
}

// chargeMP spends the skill's MP cost or refuses, flashing "{skill} needs more MP."
// (label from core.SkillName so it can't drift). The single MP chokepoint.
func chargeMP(g *core.GameState, skill core.SkillID) bool {
	// Debug "all skills" makes every cast free; one bypass covers every skill.
	if g.DebugAllSkills {
		return true
	}
	// Runs in the setup path, before the apply-side ensureAlive* rechecks — guard the
	// index so an actor whose slot went out of range between confirm and the bar
	// refuses the cast instead of panicking.
	if !partyIndexValid(g, g.Battle.CurrentParty) {
		return false
	}
	actor := &g.Party[g.Battle.CurrentParty]
	if !core.SpendSkillMP(actor, skill) {
		setBattleStatus(g, core.SkillName(skill)+" needs more MP.")
		return false
	}
	return true
}

// forEachLivingEnemy invokes fn(slot, enemy) for every alive pack member — the
// shared whole-pack walk. enemy is the write-through pointer.
func forEachLivingEnemy(g *core.GameState, fn func(slot int, enemy *core.Enemy)) {
	// Index, not value-range: Enemy is a large struct and the pointer comes from
	// BattleMemberAt anyway, so the copy would be pure waste.
	members := core.BattleMembers(g)
	for slot := range members {
		if !members[slot].Alive {
			continue
		}
		fn(slot, core.BattleMemberAt(g, slot))
	}
}

// triggerBigShake arms the "costly hit" camera punch (AoE casts, Swipe, crits).
func triggerBigShake(g *core.GameState) {
	core.TriggerCombatShake(&g.Battle, core.CombatShakeBigPeak, core.CombatShakeBigDur)
}

// applyAoEDamage hits every living enemy for `damage` via damageEnemy (SkillTag
// armor rules apply). Returns the hit count.
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
		// Multi-pass callers (Swipe) pass shake=false and fire one shake after all
		// passes so the punch arms once per attack.
		triggerBigShake(g)
	}
	return hits
}

// vfxKindFor maps a skill to the particle effect its apply step queues. Callers
// choose the enqueue direction. Unmapped skills return VFXNone — a SUPPORTED
// outcome (Taunt relies on it; Scan hardcodes its kind; Smoke Bomb has none), so
// deliberately no completeness assert.
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
	case core.SkillVenomStrike, core.SkillPoisonCloud, core.SkillCripple, core.SkillCorrosiveVial:
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

// tryProcStatus is the shared quality-scaled status-proc gate. `defeated` blocks
// a kill-shot from statusing a corpse. minGrade=0 = any quality procs; >0 gates on
// Great/Excellent. durationFn is the pre-bound duration roller; resistWis shortens it.
// Returns true when the counter was just stamped (callers pick their proc copy off it).
func tryProcStatus(rng *rand.Rand, counter *int, defeated bool, baseChance float64, quality, minGrade int, durationFn func(*rand.Rand) int, resistWis int) bool {
	if minGrade > 0 && quality < minGrade {
		return false
	}
	// Scale the base chance by the timing grade; applyStatusRoll owns guard + roll + apply.
	return applyStatusRoll(rng, counter, defeated, core.QualityScaledChance(baseChance, quality), durationFn, resistWis)
}

// applyStatusRoll is the shared status-proc core: refuses on defeated /
// non-positive chance / already-running counter (no-stack), else rolls and stamps
// the WIS-shortened duration. Used by both tryProcStatus and the enemy basic-attack
// proc. Returns true when just stamped.
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

// refundSkillMP returns setup-committed MP when an action is cancelled at apply.
// Skips DebugAllSkills (free casts — refunding would mint MP) and SkillNone.
func refundSkillMP(g *core.GameState, refundSkill core.SkillID) {
	if refundSkill != core.SkillNone && !g.DebugAllSkills {
		if cost := core.SkillCost(refundSkill); cost > 0 {
			actor := &g.Party[g.Battle.CurrentParty]
			core.GainUpTo(&actor.MP, actor.MaxMP, cost)
		}
	}
}

// ensureAliveTargetOrCancel is the apply-side counterpart of setupTargetedEnemy: a
// target can die between confirm and apply, so apply handlers call this first and
// cancel cleanly ("No target.") + refund the committed MP if it's gone.
// refundSkill is the committed skill (SkillNone for Attack/Steal). Returns true when alive.
func ensureAliveTargetOrCancel(g *core.GameState, refundSkill core.SkillID) bool {
	if core.BattleEnemyAlive(g, g.Battle.EnemyIndex) {
		return true
	}
	refundSkillMP(g, refundSkill)
	setBattleStatus(g, msgNoTarget)
	finishActorTurn(g)
	return false
}

// ensureAlivePartyTargetOrCancel is the ally-side mirror for single-ally support
// skills (Aegis / Stone Skin / Cleanse) that bypass healPartyMember's death/ingest
// guard, so they re-check the chosen ally here. Refunds + ends the turn on a gone target.
func ensureAlivePartyTargetOrCancel(g *core.GameState, refundSkill core.SkillID) bool {
	if core.PartyMemberAvailable(g.Party, g.Battle.PartyTarget) {
		return true
	}
	refundSkillMP(g, refundSkill)
	setBattleStatus(g, msgNoTarget)
	finishActorTurn(g)
	return false
}

// beginSingleTargetSkill is the shared head of every single-enemy-target damaging
// skill: refund-on-dead gate, actor lookup, attack-bump, raw-damage roll, and a
// pre-hit target snapshot (message builders want the foe's state before it dies).
// ok=false means cancelled (MP already refunded). rawDamage is pre-armor; callers
// may mutate it (crit doublers) before damageEnemy.
func beginSingleTargetSkill(g *core.GameState, skill core.SkillID, quality int) (actor *core.PartyMember, target core.Enemy, rawDamage, resistWIS int, ok bool) {
	if !ensureAliveTargetOrCancel(g, skill) {
		return nil, core.Enemy{}, 0, 0, false
	}
	actor = &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	rawDamage = applyShadowStep(g, actor, scaleSkillDamage(actor, skill, quality))
	target = *core.BattleMemberAt(g, g.Battle.EnemyIndex)
	// resistWIS hoisted here so status-proc callers don't re-derive it.
	resistWIS = core.EffectiveEnemyStats(&target).WIS
	return actor, target, rawDamage, resistWIS, true
}

// beginPartyAction is the shared head of self / ally / AoE / utility apply
// handlers: resolves the acting member and stamps AttackBump. The slot is always
// valid here (CurrentParty was captured before the bar resolved), so no re-check.
func beginPartyAction(g *core.GameState) *core.PartyMember {
	actor := &g.Party[g.Battle.CurrentParty]
	actor.AttackBump = core.BumpDuration
	return actor
}

// beginPendingAction validates / pays cost once the target is confirmed and, on
// success, arms the timing bar.
func beginPendingAction(g *core.GameState) {
	handler, ok := actionHandlerFor(g.Battle.PendingSkill)
	if !ok {
		resetBattleAction(g)
		setBattleStatus(g, msgNoSkillReady)
		return
	}
	if !handler.setup(g) {
		// setup populates the status message on failure; stay in the menu.
		return
	}
	// Confused retarget BEFORE the bar arms (so the cursor swing sells it). A no-op
	// when not confused; targetless actions skip via the switch.
	maybeConfuseRetarget(g)
	intro := core.AttackTimingIntro
	switch mg := core.SkillMinigameFor(g.Battle.PendingSkill); mg {
	case core.MinigameCharge, core.MinigameOvercharge:
		// Charge-family bars share the hold/release flow; only the constructor and
		// overload band differ. ChargeNeedsRelease blocks the target-confirm Enter
		// from engaging the charge — release once, then a fresh press engages.
		if mg == core.MinigameOvercharge {
			g.Battle.Timing = core.NewOverchargeState(core.ChargeTimingDuration)
		} else {
			g.Battle.Timing = core.NewChargeState(core.ChargeTimingDuration)
		}
		intro = core.ChargeTimingIntro
		g.Battle.ChargeNeedsRelease = true
	case core.MinigameSequence:
		g.Battle.Timing = core.NewSequenceState(g.Rand(), core.SequenceTimingDuration, core.SequenceLength)
		// Clear stick-edge memory so a tilted stick doesn't phantom-input on frame 1.
		input.ResetStickEdges()
	case core.MinigameReels:
		// Slot gamble — press-only, no stick-edge reset.
		g.Battle.Timing = core.NewReelState(g.Rand(), core.ReelTimingDuration)
	case core.MinigameRecall:
		// Memory pattern — directional taps after reveal, so clear stick edges.
		g.Battle.Timing = core.NewRecallState(g.Rand(), core.RecallTimingDuration, core.RecallPatternLength, core.RecallRevealTime)
		input.ResetStickEdges()
	default:
		if g.Battle.PendingSkill == core.SkillSwipe {
			// Swipe is a two-hit tally bar (centers in SwipeHitFracs) — a "wind up,
			// big swing" rhythm; both hits land across the whole formation.
			g.Battle.Timing = core.NewTallyStateAtCenters(core.AttackTimingDuration, core.SwipeHitFracs...)
		} else {
			g.Battle.Timing = core.NewTimingState(g.Rand(), core.AttackTimingDuration)
		}
	}
	g.Battle.TimingFlash = 0
	g.Battle.TimingIntro = intro
	g.Battle.Phase = core.BattleAttackTiming
}

// maybeConfuseRetarget scrambles EnemyIndex / PartyTarget when the actor is
// Confused AND the per-action WispConfuseRetargetRoll succeeds. Retarget stays
// within the action mode (random living enemy, or any living ally incl. the actor).
func maybeConfuseRetarget(g *core.GameState) {
	actor := g.Battle.CurrentParty
	if !partyIndexValid(g, actor) {
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
		// Reach-filtered (battleEnemyTargets): a confused melee fumble re-rolls only
		// among foes the weapon can actually hit, so it can't bypass the front-row
		// reach rule the picker enforces and strike a protected back-row enemy.
		slots := battleEnemyTargets(g)
		if len(slots) == 0 {
			return
		}
		picked := slots[rng.Intn(len(slots))]
		if picked != g.Battle.EnemyIndex {
			g.Battle.EnemyIndex = picked
			// Log it (not setBattleStatus): the bar arms the same frame and would
			// overwrite the transient prompt, so the notice would never show.
			setBattleMessage(g, fmt.Sprintf("%s is confused — wrong target!", g.Party[actor].Name))
		}
	case core.ActionPartyTarget, core.ActionItemTarget:
		// Both ally-target pickers re-roll among the living party on a fumble.
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

// applyPendingAction runs the resolved action at `quality`, stamping the actor's
// quality popup only if it landed. Phase advancement happens in the apply* path.
func applyPendingAction(g *core.GameState, quality int) {
	handler, ok := actionHandlerFor(g.Battle.PendingSkill)
	if !ok {
		setBattleStatus(g, msgNoSkillReady)
		return
	}
	// Capture the actor BEFORE apply: handler.apply runs finishActorTurn →
	// beginPartyTurn synchronously, advancing CurrentParty; reading it after would
	// stamp the popup over the next actor.
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

// recordQuality stamps the floating quality popup over a party slot. isBlock picks
// the defend palette + "BLOCK!" label (in render/timing.go). Single source for both
// attack- and block-side popups. Miss-grade popups ARE stamped — the timing still
// graded the input, acknowledging the player's performance even on an accuracy whiff.
func recordQuality(g *core.GameState, quality, partyIndex int, isBlock bool) {
	g.Battle.LastQuality = quality
	g.Battle.LastQualityTimer = core.QualityResultDuration
	g.Battle.LastQualityIndex = partyIndex
	g.Battle.LastQualityIsBlock = isBlock
}

// --- Basic Attack ---

func applyAttack(g *core.GameState, quality int) bool {
	// No MP cost; SkillNone makes the refund branch a no-op.
	if !ensureAliveTargetOrCancel(g, core.SkillNone) {
		return false
	}
	attacker := &g.Party[g.Battle.CurrentParty]
	// AttackBump fires unconditionally — the swing plays even on a whiff.
	attacker.AttackBump = core.BumpDuration
	target := *core.BattleMemberAt(g, g.Battle.EnemyIndex)
	// Accuracy roll (basic attack only): DEX + timing drive hit chance, clamped past
	// 1.0 so high-DEX/high-grade essentially never whiff. The swing + timing popup
	// still play on a miss; only damage is withheld.
	if !core.MemberAttackHitsTarget(g.Rand(), *attacker, target, quality) {
		// Keep the quality prefix so the whiff reads consistently with hits. A melee
		// swing at a flyer reads "can't reach" so the miss is legible as the flying penalty.
		whiff := fmt.Sprintf("%s%s swings wide.", qualityTag(quality), attacker.Name)
		if core.EnemyInfoFor(target).Flying && !core.MemberMeleeReachesFlyer(*attacker) {
			whiff = fmt.Sprintf("%s%s can't reach the airborne %s.", qualityTag(quality), attacker.Name, core.EnemySingularNoun(target))
		}
		setBattleMessage(g, whiff)
		finishActorTurn(g)
		return true
	}
	// Defender dodge: a connecting swing can still be sidestepped (symmetric with
	// the party-side dodge). Skills are NOT dodgeable.
	if core.RollDodge(g.Rand(), core.EffectiveEnemyStats(&target)) {
		setBattleMessage(g, fmt.Sprintf("%s%s lunges but the %s slips aside.", qualityTag(quality), attacker.Name, core.EnemySingularNoun(target)))
		finishActorTurn(g)
		return true
	}
	// Basic Attack: MemberAttackDamage (STR/DEX per weapon) scaled by quality,
	// Phys-tagged so armor damps it. The log uses damageEnemy's POST-armor figure
	// so it matches the HP delta.
	rawDamage := applyShadowStep(g, attacker, core.ScaleDamage(core.MemberAttackDamage(*attacker, 0), quality))
	crit, _ := rollSkillCrit(g, attacker, core.SkillNone, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	dealt, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagPhys)
	// Glyph keyed to the weapon (blunt/ranged = impact, edged = slash). Basic
	// attacks are the only weapon-driven swing; skills pick VFX via vfxKindFor.
	core.EnqueueEnemyVFX(g, core.WeaponHitVFX(core.EquippedWeapon(*attacker)), g.Battle.EnemyIndex)
	setBattleMessage(g, appendCrit(attackResultMessage(attacker.Name, target, dealt, quality, defeated), crit))
	finishActorTurn(g)
	return true
}

// --- Swipe (Warrior, hits all enemies in the battle group) ---

func setupSwipe(g *core.GameState) bool {
	return chargeMP(g, core.SkillSwipe)
}

func applySwipe(g *core.GameState, quality int) bool {
	actor := beginPartyAction(g)
	// Swipe is Melee-kind (STR + Effect.Damage). Tally mode does one pass per
	// tallied hit; single-press fallback does one pass.
	damage := scaleSkillDamage(actor, core.SkillSwipe, quality)
	// One crit roll for the whole Swipe (the player can't react between sweeps).
	crit, _ := rollSkillCrit(g, actor, core.SkillSwipe, quality)
	damage = applyCritMultiplier(damage, crit, false)
	passes := multiPressPasses(g.Battle.Timing, quality)
	// enemiesHit = distinct foes struck, captured from the FIRST pass (later passes
	// hit fewer as kills accrue; the full set was struck at least once).
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
		// One punch per Swipe, not per tally pass.
		triggerBigShake(g)
	}
	if enemiesHit == 0 || passes == 0 {
		setBattleMessage(g, aoeEmptyMessage(core.SkillName(core.SkillSwipe), "catches only air"))
	} else {
		setBattleMessage(g, appendCrit(swipeMessage(actor.Name, enemiesHit, quality), crit))
	}
	finishActorTurn(g)
	// hits=0 still counts as landed (motion played, MP spent).
	return true
}

// multiPressPasses returns the damage-pass count for a tally-mode skill (one per
// tallied hit). Non-tally bars do 1 pass on any non-Miss grade, 0 on Miss.
func multiPressPasses(t core.TimingState, quality int) int {
	if t.IsTallyMode() {
		return t.Hits
	}
	if quality == core.TimingQualityMiss {
		return 0
	}
	return 1
}

// setupTargetedAllyAndPay confirms a LIVING ally is targeted, then pays MP. A dead
// / unselected ally refuses WITHOUT spending MP. deadMsg is the per-skill downed-target line.
func setupTargetedAllyAndPay(g *core.GameState, skill core.SkillID, deadMsg string) bool {
	if !partyIndexValid(g, g.Battle.PartyTarget) {
		setBattleStatus(g, msgNoAllySelected)
		return false
	}
	if g.Party[g.Battle.PartyTarget].HP <= 0 {
		setBattleStatus(g, deadMsg)
		return false
	}
	return chargeMP(g, skill)
}

// --- Prayer (Cleric, heals an ally) ---

func applyPrayer(g *core.GameState, quality int) bool {
	// The chosen ally can die (or its slot go out of range) between confirm and this
	// apply; re-check — refund + end the turn on a gone target — so we don't index a
	// stale slot or log a heal that never landed. Mirrors the other single-ally casts.
	if !ensureAlivePartyTargetOrCancel(g, core.SkillPrayer) {
		return false
	}
	actor := beginPartyAction(g)
	// Prayer is Heal-kind (WIS + Effect.Heal).
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
	// Steal costs 0 MP; pass the skill anyway in case a cost is added.
	if !ensureAliveTargetOrCancel(g, core.SkillSteal) {
		return false
	}
	actor := beginPartyAction(g)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	if enemy.Item == core.ItemNone {
		setBattleMessage(g, "There is nothing to steal.")
		finishActorTurn(g)
		// The hand still moved — landed.
		return true
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillSteal)
	// Flat base chance (no DEX) with the timing multiplier on top, capped at 1.0.
	chance := core.QualityScaledChance(core.StealChance(effect.StealChance), quality)
	if g.Rand().Float64() < chance {
		kind := enemy.Item
		// Clear the item off the enemy so it can't be looted twice.
		enemy.Item = core.ItemNone
		g.Inventory = core.AddItem(g.Inventory, kind, 1)
		// Steal T3 ("Cuts on lift") deals STR×StealBonusDamage on a landed steal.
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

// applyScan identifies the target's KIND in the bestiary (shortcut to the
// kills-to-identify threshold). No damage / status / roll — the ID always lands;
// timing is cosmetic.
func applyScan(g *core.GameState, quality int) bool {
	// Setup committed MP; the shared head refunds it on a dead target.
	if !ensureAliveTargetOrCancel(g, core.SkillScan) {
		return false
	}
	actor := beginPartyAction(g)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	g.Bestiary.MarkScanned(enemy.Kind)
	core.EnqueueEnemyVFX(g, core.VFXScan, g.Battle.EnemyIndex)
	setBattleMessage(g, fmt.Sprintf("%s scans the %s — %d/%d HP. Identified.",
		actor.Name, core.EnemySingularNoun(*enemy), enemy.HP, enemy.MaxHP))
	finishActorTurn(g)
	return true
}

// applyEnemyDebuffSkill is the shared body for press-class single-enemy debuffs
// (Cripple/Blind/Taunt): refund-on-dead-target gate, pay + begin the turn, and
// resolve the effect, then hand off to stamp so each skill supplies its own
// mutation + log line. VFX (via vfxKindFor) and finishActorTurn are shared.
func applyEnemyDebuffSkill(g *core.GameState, skill core.SkillID, stamp func(actor *core.PartyMember, enemy *core.Enemy, effect core.SkillEffect) string) bool {
	// Setup committed MP; the shared head refunds it on a dead target.
	if !ensureAliveTargetOrCancel(g, skill) {
		return false
	}
	actor := beginPartyAction(g)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	effect := core.EffectiveSkillEffect(actor, skill)
	msg := stamp(actor, enemy, effect)
	core.EnqueueEnemyVFX(g, vfxKindFor(skill), g.Battle.EnemyIndex)
	setBattleMessage(g, msg)
	finishActorTurn(g)
	return true
}

// applyCripple stamps the SPD debuff onto the target via the enemy BuffStats
// mirror (EffectiveEnemyStats folds it into the ATB rate while it runs). No
// damage / proc roll; re-cast overwrites (no-stack).
func applyCripple(g *core.GameState, quality int) bool {
	return applyEnemyDebuffSkill(g, core.SkillCripple, func(actor *core.PartyMember, enemy *core.Enemy, effect core.SkillEffect) string {
		core.StampEnemyDebuff(enemy, core.SkillCripple, effect)
		return fmt.Sprintf("%s cripples the %s — slowed for %d turns.",
			actor.Name, core.EnemySingularNoun(*enemy), effect.BuffTurns)
	})
}

// applyFrostbite deals frost damage and, on a surviving target, ALWAYS chills it
// (SPD debuff, like Cripple — no proc roll, not WIS-resistible). Re-cast overwrites.
func applyFrostbite(g *core.GameState, quality int) bool {
	actor, target, rawDamage, _, ok := beginSingleTargetSkill(g, core.SkillFrostbite, quality)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillFrostbite)
	damage, defeated, crit := strikeWithCrit(g, actor, core.SkillFrostbite, rawDamage, quality)
	core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillFrostbite), g.Battle.EnemyIndex)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	// Chill only on a survivor (no debuffing a corpse); guaranteed when alive.
	chilled := !defeated && core.StampEnemyDebuff(enemy, core.SkillFrostbite, effect)
	setBattleMessage(g, appendCrit(frostbiteMessage(actor.Name, target, damage, quality, defeated, chilled), crit))
	finishActorTurn(g)
	return true
}

func frostbiteMessage(name string, target core.Enemy, damage, quality int, defeated, chilled bool) string {
	return procSkillMessage(frostbiteArms, name, target, damage, quality, defeated, chilled)
}

// applyCorrosiveVial permanently strips the target's Armor (floored at 0) — it
// mutates live Enemy.Armor, not a turn-counted debuff, so re-casts stack down. No damage.
func applyCorrosiveVial(g *core.GameState, quality int) bool {
	// Setup committed MP; the shared head refunds it on a dead target.
	if !ensureAliveTargetOrCancel(g, core.SkillCorrosiveVial) {
		return false
	}
	actor := beginPartyAction(g)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	effect := core.EffectiveSkillEffect(actor, core.SkillCorrosiveVial)
	before := enemy.Armor
	enemy.Armor -= effect.ArmorReduction
	if enemy.Armor < 0 {
		enemy.Armor = 0
	}
	core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillCorrosiveVial), g.Battle.EnemyIndex)
	if stripped := before - enemy.Armor; stripped > 0 {
		setBattleMessage(g, fmt.Sprintf("%s's vial eats %d Armor off the %s.", actor.Name, stripped, core.EnemySingularNoun(*enemy)))
	} else {
		setBattleMessage(g, fmt.Sprintf("%s's vial splashes the %s — no armor left to break.", actor.Name, core.EnemySingularNoun(*enemy)))
	}
	finishActorTurn(g)
	return true
}

// --- Firebolt (Wizard, ramps damage and burn chance with quality) ---

func applyFirebolt(g *core.GameState, quality int) bool {
	actor, target, rawDamage, resistWIS, ok := beginSingleTargetSkill(g, core.SkillFirebolt, quality)
	if !ok {
		return false
	}
	// Overcharge: a release past the peak adds flat bonus damage; the recoil it
	// costs is applied after the hit lands, below.
	overloaded := g.Battle.Timing.Overloaded
	if overloaded {
		rawDamage += core.OverchargeDamageBonus
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillFirebolt)
	damage, defeated, crit := strikeWithCrit(g, actor, core.SkillFirebolt, rawDamage, quality)
	core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillFirebolt), g.Battle.EnemyIndex)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	burned := tryProcStatus(g.Rand(), &enemy.BurnTurns, defeated, effect.BurnChance, quality, 0, effect.BurnDuration, resistWIS)
	setBattleMessage(g, appendCrit(fireboltMessage(actor.Name, target, damage, quality, defeated, burned, enemy.BurnTurns), crit))
	if overloaded {
		// Overcharge recoils on the caster. SkillTagNone so the self-burn bypasses
		// the caster's own armor/MDef.
		recoil, _ := damagePartyMember(g, g.Battle.CurrentParty, core.OverchargeRecoil, core.SkillTagNone)
		core.EnqueuePartyVFX(g, core.VFXEmber, g.Battle.CurrentParty)
		setBattleMessage(g, fmt.Sprintf("%s overcharges the bolt — and is scorched for %d!", actor.Name, recoil))
	}
	finishActorTurn(g)
	return true
}

// --- Crushing Blow (Warrior, charge phys hit with Stun proc on Great+) ---

func applyCrushingBlow(g *core.GameState, quality int) bool {
	actor, target, rawDamage, resistWIS, ok := beginSingleTargetSkill(g, core.SkillCrushingBlow, quality)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillCrushingBlow)
	// Crushing Blow T3 doubles damage on an Excellent timing roll, INDEPENDENT of
	// the universal crit below — both stack (CritMultiplier × 2 = 4×), like Backstab T2.
	if quality == core.TimingQualityExcellent && core.SkillTierMod(actor, core.SkillCrushingBlow).CritDoubleOnExcellent {
		rawDamage *= core.TierDamageDoubler
	}
	damage, defeated, crit := strikeWithCrit(g, actor, core.SkillCrushingBlow, rawDamage, quality)
	core.EnqueueEnemyVFX(g, core.VFXSlash, g.Battle.EnemyIndex)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	stunned := tryProcStatus(g.Rand(), &enemy.StunTurns, defeated, effect.StunChance, quality, core.TimingQualityGreat, effect.StunDuration, resistWIS)
	setBattleMessage(g, appendCrit(crushingBlowMessage(actor.Name, target, damage, quality, defeated, stunned), crit))
	finishActorTurn(g)
	return true
}

// --- Whirlwind (Warrior, charge AoE phys) ---

func applyWhirlwind(g *core.GameState, quality int) bool {
	// No Burn/Poison, so the AoE body's status rolls short-circuit — pure damage.
	return applyAoEStatusSkill(g, core.SkillWhirlwind, "hits", "catches only air", quality)
}

// --- Mass Mend (Cleric, charge AoE heal) ---

func applyMassMend(g *core.GameState, quality int) bool {
	actor := beginPartyAction(g)
	heal := core.ScaleHeal(core.SkillHealFor(actor, core.SkillMassMend), quality)
	// Tally wounds + queue VFX on PRE-heal HP, then heal via core.HealWholeParty
	// (shared with the out-of-battle Mass Mend; no-ops dead/ingested, clamps at MaxHP).
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

// applyBless stamps the tier-folded stat buff on every available member (caster
// included). Always lands (no proc roll); re-cast replaces (no-stack).
func applyBless(g *core.GameState, quality int) bool {
	actor := beginPartyAction(g)
	effect := core.EffectiveSkillEffect(actor, core.SkillBless)
	blessed := stampPartyWideBuff(g, effect, core.SkillBless)
	// Report the EFFECTIVE per-stat boost (all four share one magnitude), not the base.
	setBattleMessage(g, fmt.Sprintf("%s%s blesses %d allies (+%d stats, %d turns).",
		qualityTag(quality), actor.Name, blessed, effect.BuffStats.STR, effect.BuffTurns))
	finishActorTurn(g)
	return true
}

// --- Smite (Cleric, press-tap magic damage) ---

func applySmite(g *core.GameState, quality int) bool {
	actor, target, rawDamage, resistWIS, ok := beginSingleTargetSkill(g, core.SkillSmite, quality)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillSmite)
	damage, defeated, crit := strikeWithCrit(g, actor, core.SkillSmite, rawDamage, quality)
	core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillSmite), g.Battle.EnemyIndex)
	// Smite T3 adds a Great+ stun proc (StunChance is 0 at T0..2, so it short-circuits).
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	stunned := tryProcStatus(g.Rand(), &enemy.StunTurns, defeated, effect.StunChance, quality, core.TimingQualityGreat, effect.StunDuration, resistWIS)
	setBattleMessage(g, appendCrit(smiteMessage(actor.Name, target, damage, quality, defeated, stunned), crit))
	finishActorTurn(g)
	return true
}

// --- Backstab (Thief, charge phys with crit on Excellent) ---

func applyBackstab(g *core.GameState, quality int) bool {
	actor, target, rawDamage, _, ok := beginSingleTargetSkill(g, core.SkillBackstab, quality)
	if !ok {
		return false
	}
	// An Excellent Backstab is a guaranteed crit (rollSkillCrit's special branch);
	// T2's `double` adds a second ×2 (T2+ Excellent = x4). Non-Excellent gets the
	// universal DEX+timing crit chance.
	crit, double := rollSkillCrit(g, actor, core.SkillBackstab, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, double)
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(core.SkillBackstab))
	core.EnqueueEnemyVFX(g, core.VFXSlash, g.Battle.EnemyIndex)
	setBattleMessage(g, backstabMessage(actor.Name, target, damage, quality, defeated, crit))
	finishActorTurn(g)
	return true
}

// dotStrike describes a "phys hit + single-target DoT" skill (Venom Strike,
// Rend, Lacerate). Closures select the counter and chance/duration fields so
// applyDoTStrike stays status-agnostic; `arms` narrates.
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

// bleedDoT builds the Bleed descriptor for Rend / Lacerate (only arms differs).
func bleedDoT(arms procMessageArms) dotStrike {
	return dotStrike{
		counter: func(e *core.Enemy) *int { return &e.BleedTurns },
		chance:  func(eff core.SkillEffect) float64 { return eff.BleedChance },
		dur:     func(eff core.SkillEffect) func(*rand.Rand) int { return eff.BleedDuration },
		arms:    arms,
	}
}

// strikeWithCrit rolls the crit, applies the multiplier (never Backstab's double —
// that lives in applyBackstab), and deals the hit via damageEnemy. Returns
// post-armor damage, defeated, crit. Callers pre-mutate rawDamage before calling.
func strikeWithCrit(g *core.GameState, actor *core.PartyMember, skill core.SkillID, rawDamage, quality int) (damage int, defeated, crit bool) {
	crit, _ = rollSkillCrit(g, actor, skill, quality)
	rawDamage = applyCritMultiplier(rawDamage, crit, false)
	damage, defeated = damageEnemy(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(skill))
	return damage, defeated, crit
}

// applyDoTStrike is the single body for every phys-hit-plus-DoT skill: a hit that,
// on a survivor, rolls the DoT (dot.chance) onto dot.counter via tryProcStatus.
func applyDoTStrike(g *core.GameState, skill core.SkillID, quality int, dot dotStrike) bool {
	actor, target, rawDamage, resistWIS, ok := beginSingleTargetSkill(g, skill, quality)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, skill)
	damage, defeated, crit := strikeWithCrit(g, actor, skill, rawDamage, quality)
	core.EnqueueEnemyVFX(g, vfxKindFor(skill), g.Battle.EnemyIndex)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	procced := tryProcStatus(g.Rand(), dot.counter(enemy), defeated, dot.chance(effect), quality, 0, dot.dur(effect), resistWIS)
	setBattleMessage(g, appendCrit(procSkillMessage(dot.arms, actor.Name, target, damage, quality, defeated, procced), crit))
	finishActorTurn(g)
	return true
}

// --- Venom Strike (Thief, sequence phys + Poison apply) ---

func applyVenomStrike(g *core.GameState, quality int) bool {
	return applyDoTStrike(g, core.SkillVenomStrike, quality, venomStrikeDoT)
}

// --- Rend (Warrior) / Lacerate (Thief): phys hit + Bleed DoT apply ---

func applyRend(g *core.GameState, quality int) bool {
	return applyDoTStrike(g, core.SkillRend, quality, bleedDoT(rendArms))
}

func applyLacerate(g *core.GameState, quality int) bool {
	return applyDoTStrike(g, core.SkillLacerate, quality, bleedDoT(lacerateArms))
}

// --- Frost Lance (Wizard, charge magic with reliable Stun on Great+) ---

func applyFrostLance(g *core.GameState, quality int) bool {
	actor, target, rawDamage, resistWIS, ok := beginSingleTargetSkill(g, core.SkillFrostLance, quality)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillFrostLance)
	damage, defeated, crit := strikeWithCrit(g, actor, core.SkillFrostLance, rawDamage, quality)
	core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillFrostLance), g.Battle.EnemyIndex)
	enemy := core.BattleMemberAt(g, g.Battle.EnemyIndex)
	// FrostLance is "freeze" flavor but uses the canonical StunTurns counter (no
	// separate frozen status); the log keeps "Frozen!" via frostLanceMessage.
	stunned := tryProcStatus(g.Rand(), &enemy.StunTurns, defeated, effect.StunChance, quality, core.TimingQualityGreat, effect.StunDuration, resistWIS)
	setBattleMessage(g, appendCrit(frostLanceMessage(actor.Name, target, damage, quality, defeated, stunned), crit))
	finishActorTurn(g)
	return true
}

// --- Arc Bolt (Wizard, sequence-tap AoE magic) ---

func applyArcBolt(g *core.GameState, quality int) bool {
	// Via applyAoEStatusSkill so Arc Bolt T3's per-target Burn procs (T0-T2 carry
	// BurnChance 0 and short-circuit).
	return applyAoEStatusSkill(g, core.SkillArcBolt, "arcs across", "dissipates with no target", quality)
}

// --- AoE skills (shared body; per-target status when the skill carries it) ---

// applyAoEStatusSkill is the single body for every whole-pack AoE skill: one crit
// roll for the sweep, hits every living enemy, and per target rolls the skill's
// Burn / Poison (chance 0 short-circuits, so a status-free skill is pure damage).
// applyAoEDamage remains for the multi-PASS Swipe.
func applyAoEStatusSkill(g *core.GameState, skill core.SkillID, hitVerb, emptyVerb string, quality int) bool {
	skillNoun := core.SkillName(skill)
	actor := beginPartyAction(g)
	effect := core.EffectiveSkillEffect(actor, skill)
	damage := scaleSkillDamage(actor, skill, quality)
	crit, _ := rollSkillCrit(g, actor, skill, quality)
	damage = applyCritMultiplier(damage, crit, false)
	tag := core.SkillTagFor(skill)
	vfx := vfxKindFor(skill)
	hits := 0
	afflicted := 0
	totalDealt := 0
	forEachLivingEnemy(g, func(slot int, enemy *core.Enemy) {
		dealt, defeated := damageEnemy(g, slot, damage, quality, tag)
		totalDealt += dealt
		core.EnqueueEnemyVFX(g, vfx, slot)
		hits++
		resistWIS := core.EffectiveEnemyStats(enemy).WIS
		// Count each foe afflicted at most once so the tally can't exceed foe count.
		struck := false
		if tryProcStatus(g.Rand(), &enemy.BurnTurns, defeated, effect.BurnChance, quality, 0, effect.BurnDuration, resistWIS) {
			struck = true
		}
		if tryProcStatus(g.Rand(), &enemy.PoisonTurns, defeated, effect.PoisonChance, quality, 0, effect.PoisonDuration, resistWIS) {
			struck = true
		}
		// AoE stat debuff (Cone of Cold's chill) — guaranteed on a survivor.
		// Skills with no buff (BuffTurns 0) skip it.
		if !defeated && core.StampEnemyDebuff(enemy, skill, effect) {
			struck = true
		}
		if struck {
			afflicted++
		}
	})
	if hits == 0 {
		setBattleMessage(g, aoeEmptyMessage(skillNoun, emptyVerb))
		finishActorTurn(g)
		return true
	}
	// AoE casts earn the big camera punch.
	triggerBigShake(g)
	// Report the average post-mitigation hit so the figure tracks real HP loss
	// (armor/MDef varies per foe); hits >= 1 here (the hits == 0 case returned above).
	msg := appendCrit(aoeSkillMessage(actor.Name, skillNoun, hitVerb, hits, totalDealt/hits, quality), crit)
	if afflicted > 0 {
		msg = fmt.Sprintf("%s %d afflicted.", msg, afflicted)
	}
	setBattleMessage(g, msg)
	finishActorTurn(g)
	return true
}

// --- Fireball (Wizard, charge AoE fire + per-target Burn) ---

func applyFireball(g *core.GameState, quality int) bool {
	return applyAoEStatusSkill(g, core.SkillFireball, "engulfs", "fizzles with no target", quality)
}

// applyConeOfCold is the AoE Frostbite: frost sweep + guaranteed per-target SPD
// chill on every survivor (the BuffTurns>0 branch of the shared body).
func applyConeOfCold(g *core.GameState, quality int) bool {
	return applyAoEStatusSkill(g, core.SkillConeOfCold, "sweeps over", "billows with no target", quality)
}

// --- Sunder (Warrior, charge phys + ATB shove) ---

// applySunder deals phys damage and, on a survivor, shoves its ATB gauge back
// (effect.ATBPush) — a one-shot tempo swing, not a status. No shove on a kill.
func applySunder(g *core.GameState, quality int) bool {
	actor, target, rawDamage, _, ok := beginSingleTargetSkill(g, core.SkillSunder, quality)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillSunder)
	damage, defeated, crit := strikeWithCrit(g, actor, core.SkillSunder, rawDamage, quality)
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

// applyTaunt forces the target to attack the caster next turn (TauntedBy /
// TauntTurns, honored by pickEnemyAttackTarget). No damage; always lands; re-cast overwrites.
func applyTaunt(g *core.GameState, quality int) bool {
	return applyEnemyDebuffSkill(g, core.SkillTaunt, func(actor *core.PartyMember, enemy *core.Enemy, _ core.SkillEffect) string {
		enemy.TauntedBy = g.Battle.CurrentParty
		enemy.TauntTurns = core.TauntTurns
		return fmt.Sprintf("%s taunts the %s — it turns its glare on them.",
			actor.Name, core.EnemySingularNoun(*enemy))
	})
}

// --- War Banner (Warrior, press party-wide STR/VIT rally) ---

func applyWarBanner(g *core.GameState, quality int) bool {
	actor := beginPartyAction(g)
	effect := core.EffectiveSkillEffect(actor, core.SkillWarBanner)
	rallied := stampPartyWideBuff(g, effect, core.SkillWarBanner)
	setBattleMessage(g, fmt.Sprintf("%s%s plants a war banner — %d allies rally (+%d STR, +%d Armor, %d turns).",
		qualityTag(quality), actor.Name, rallied, effect.BuffStats.STR, effect.BuffArmor, effect.BuffTurns))
	finishActorTurn(g)
	return true
}

// --- Stone Skin (Warrior, press single-ally Armor/MDef ward) ---

func applyStoneSkin(g *core.GameState, quality int) bool {
	if !ensureAlivePartyTargetOrCancel(g, core.SkillStoneSkin) {
		return false
	}
	actor := beginPartyAction(g)
	effect := core.EffectiveSkillEffect(actor, core.SkillStoneSkin)
	target := &g.Party[g.Battle.PartyTarget]
	eff := effect
	// Self-cast +1 correction (offsets finishActorTurn's immediate tick); see selfCastTurnBonus.
	eff.BuffTurns += selfCastTurnBonus(g, g.Battle.PartyTarget)
	core.StampPartyBuff(target, core.SkillStoneSkin, eff)
	core.EnqueuePartyVFX(g, vfxKindFor(core.SkillStoneSkin), g.Battle.PartyTarget)
	// Report the STAMPED duration (carries the self-cast +1), not the base.
	setBattleMessage(g, fmt.Sprintf("%s%s wards %s in stone (+%d Armor, +%d MDef, %d turns).",
		qualityTag(quality), actor.Name, target.Name, eff.BuffArmor, eff.BuffMDef, eff.BuffTurns))
	finishActorTurn(g)
	return true
}

// --- Blind (Cleric, press enemy accuracy debuff) ---

// applyBlind saps the target's DEX (which EnemyHitChance reads) so it whiffs more
// — the DEX sibling of Cripple. No damage; always lands; re-cast overwrites.
func applyBlind(g *core.GameState, quality int) bool {
	return applyEnemyDebuffSkill(g, core.SkillBlind, func(actor *core.PartyMember, enemy *core.Enemy, effect core.SkillEffect) string {
		core.StampEnemyDebuff(enemy, core.SkillBlind, effect)
		return fmt.Sprintf("%s blinds the %s — its aim falters for %d turns.",
			actor.Name, core.EnemySingularNoun(*enemy), effect.BuffTurns)
	})
}

// --- Aegis (Cleric, press single-ally absorb shield) ---

// applyAegis grants the ally a ShieldHP pool the damage path spends before HP.
// Not turn-counted; re-cast replaces the pool.
func applyAegis(g *core.GameState, quality int) bool {
	if !ensureAlivePartyTargetOrCancel(g, core.SkillAegis) {
		return false
	}
	actor := beginPartyAction(g)
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

// applySmokeBomb buffs the party's DEX (evasion) and saps every enemy's DEX
// (accuracy) by the SAME magnitude. Both overwrite (no-stack).
func applySmokeBomb(g *core.GameState, quality int) bool {
	actor := beginPartyAction(g)
	effect := core.EffectiveSkillEffect(actor, core.SkillSmokeBomb)
	buffed := stampPartyWideBuff(g, effect, core.SkillSmokeBomb)
	enemyDebuff := core.SkillEffect{BuffStats: core.Stats{DEX: -effect.BuffStats.DEX}, BuffTurns: effect.BuffTurns}
	blinded := 0
	if effect.BuffStats.DEX != 0 {
		// Only stamp (and count) when the DEX delta is real — a zero-DEX effect
		// would otherwise report every foe "blinded" while applying a no-op mod.
		forEachLivingEnemy(g, func(_ int, enemy *core.Enemy) {
			if core.StampEnemyDebuff(enemy, core.SkillSmokeBomb, enemyDebuff) {
				blinded++
			}
		})
	}
	setBattleMessage(g, fmt.Sprintf("%s%s drops a smoke bomb — %d allies gain evasion, %d foes lose their aim.",
		qualityTag(quality), actor.Name, buffed, blinded))
	finishActorTurn(g)
	return true
}

// --- Ice Armor (Wizard, charge self frost ward) ---

// applyIceArmor sheathes the caster in frost: while IceArmorTurns runs they gain
// MDef and chill any enemy that lands a basic attack on them. Self-only; +1 turn
// offsets finishActorTurn's immediate tick (Bless rule).
func applyIceArmor(g *core.GameState, quality int) bool {
	actor := beginPartyAction(g)
	effect := core.EffectiveSkillEffect(actor, core.SkillIceArmor)
	// Self-only, so selfCastTurnBonus always returns 1 (the self-cast +1).
	turns := effect.IceArmorTurns + selfCastTurnBonus(g, g.Battle.CurrentParty)
	actor.IceArmorTurns = turns
	core.EnqueuePartyVFX(g, vfxKindFor(core.SkillIceArmor), g.Battle.CurrentParty)
	// Log the stamped duration (incl. self-cast +1), not the base.
	setBattleMessage(g, fmt.Sprintf("%s%s sheathes in ice — +%d MDef, attackers chilled for %d turns.",
		qualityTag(quality), actor.Name, core.IceArmorMDef, turns))
	finishActorTurn(g)
	return true
}

// selfCastTurnBonus returns the +1 duration correction for a buff stamped on the
// CURRENTLY-ACTING member: finishActorTurn ticks their buff down before they act
// again, so +1 nets the full duration. Returns 0 for any other ally. The one place
// this rule lives (Bless / War Banner / Smoke Bomb / Stone Skin / Ice Armor).
func selfCastTurnBonus(g *core.GameState, targetIdx int) int {
	if targetIdx == g.Battle.CurrentParty {
		return 1
	}
	return 0
}

// stampPartyWideBuff stamps the buff on every available member (caster +1 via
// selfCastTurnBonus) and returns the count. Shared by Bless / War Banner / Smoke Bomb.
func stampPartyWideBuff(g *core.GameState, effect core.SkillEffect, skill core.SkillID) int {
	buffed := 0
	for _, i := range core.AvailablePartyTargets(g.Party) {
		eff := effect
		// Re-cast refreshes, never compounds.
		eff.BuffTurns += selfCastTurnBonus(g, i)
		core.StampPartyBuff(&g.Party[i], skill, eff)
		core.EnqueuePartyVFX(g, vfxKindFor(skill), i)
		buffed++
	}
	return buffed
}

// tickIceArmorAfterPartyTurn drains the Ice Armor ward at the warded member's
// turn end (same non-damaging seam as Bless / Renewal).
func tickIceArmorAfterPartyTurn(g *core.GameState, actor core.ActorRef) {
	tickPartyStatusCounter(g, actor, func(m *core.PartyMember) *int { return &m.IceArmorTurns }, "%s's ice armor melts.")
}

// tickEnemyTauntAfterTurn drains a taunt at the taunted enemy's turn end so the
// pull lasts exactly its window. No-ops on party actors / untaunted foes.
func tickEnemyTauntAfterTurn(g *core.GameState, actor core.ActorRef) {
	if actor.IsParty {
		return
	}
	enemy, ok := livingEnemyAt(g, actor.Index)
	if !ok || enemy.TauntTurns <= 0 {
		return
	}
	enemy.TauntTurns--
}

// forcedTauntTarget returns the slot the attacking enemy is taunted onto, if the
// pull is live AND the taunter is still reachable. ok=false falls back to round-robin.
func forcedTauntTarget(g *core.GameState) (int, bool) {
	enemy := core.BattleMemberAt(g, g.Battle.EnemyAttacker)
	if enemy == nil || enemy.TauntTurns <= 0 {
		return -1, false
	}
	t := enemy.TauntedBy
	if !partyIndexValid(g, t) {
		return -1, false
	}
	if g.Party[t].HP <= 0 || g.Party[t].Ingested {
		return -1, false
	}
	return t, true
}

// --- Poison Cloud (Thief, sequence AoE toxin + per-target Poison) ---

func applyPoisonCloud(g *core.GameState, quality int) bool {
	return applyAoEStatusSkill(g, core.SkillPoisonCloud, "blankets", "disperses with no target", quality)
}

// --- Cleanse (Cleric, press single-ally status cure) ---

func applyCleanse(g *core.GameState, quality int) bool {
	if !ensureAlivePartyTargetOrCancel(g, core.SkillCleanse) {
		return false
	}
	actor := beginPartyAction(g)
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

func applySecondWind(g *core.GameState, quality int) bool {
	actor := beginPartyAction(g)
	// Utility-kind: flat Effect.Heal (no WIS), timing-scaled by ScaleHeal.
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

func applyRenewal(g *core.GameState, quality int) bool {
	// Renewal arms on a charge bar (the ally can die mid-charge) and bypasses
	// healPartyMember's guard by stamping the regen counter directly, so re-check here.
	if !ensureAlivePartyTargetOrCancel(g, core.SkillRenewal) {
		return false
	}
	actor := beginPartyAction(g)
	effect := core.EffectiveSkillEffect(actor, core.SkillRenewal)
	// Snapshot the per-turn heal at cast (WIS + timing), floored at 1. Re-cast replaces.
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

// --- Damage / heal helpers ---

// setupTargetedEnemyAndPay confirms a live enemy is targeted AND pays MP
// (setupTargetedEnemy + chargeMP).
func setupTargetedEnemyAndPay(g *core.GameState, skill core.SkillID) bool {
	if !setupTargetedEnemy(g) {
		return false
	}
	return chargeMP(g, skill)
}

// scaleSkillDamage returns the quality-scaled raw damage for `actor` casting
// `skill` (ScaleDamage ∘ SkillDamageFor).
func scaleSkillDamage(actor *core.PartyMember, skill core.SkillID, quality int) int {
	return core.ScaleDamage(core.SkillDamageFor(actor, skill), quality)
}

// rollSkillCrit returns the (crit, double) flags. Standard skills crit via
// probabilistic core.RollCrit; Backstab Excellent = guaranteed crit, and its T2
// tier sets `double` (the only true case) for an extra ×2.
func rollSkillCrit(g *core.GameState, actor *core.PartyMember, skill core.SkillID, quality int) (crit, double bool) {
	if actor == nil {
		return false, false
	}
	if skill == core.SkillBackstab && quality >= core.TimingQualityExcellent {
		crit = true
		if core.SkillTierMod(actor, core.SkillBackstab).CritDoubleOnExcellent {
			double = true
		}
		triggerBigShake(g)
		return
	}
	// MemberRollCrit folds the Thief's Lucky Strike into the DEX/timing curve
	// (no-op without the node).
	crit = core.MemberRollCrit(g.Rand(), actor, quality)
	if crit {
		triggerBigShake(g)
	}
	return
}

// applyCritMultiplier returns the post-crit damage; `double` (Backstab T2)
// multiplies again on top of CritMultiplier.
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

// targetActsLater reports whether the enemy at enemySlot has NOT acted yet this
// round (occupies no queue slot before the cursor). Shadow Step gates its bonus on
// this so it rewards getting the jump, not re-hitting an already-acted foe.
func targetActsLater(g *core.GameState, enemySlot int) bool {
	return !actorAppearsBefore(g.Battle.Queue, g.Battle.QueueCursor, core.ActorRef{IsParty: false, Index: enemySlot})
}

// applyShadowStep folds the Thief's Shadow Step into an outgoing single-target hit:
// +ShadowStepBonusPerRank/rank when striking before the target acts (rank 0 = no-op).
// Applied pre-crit so the bonus rides the crit multiplier.
func applyShadowStep(g *core.GameState, actor *core.PartyMember, raw int) int {
	rank := core.PassiveRank(actor, core.PassiveShadowStep)
	if rank <= 0 || raw <= 0 || !targetActsLater(g, g.Battle.EnemyIndex) {
		return raw
	}
	return raw + int(float64(raw)*float64(rank)*core.ShadowStepBonusPerRank)
}

// appendCrit suffixes " Critical!" when crit landed. Used by every damaging attack
// except Backstab (which encodes the crit in its proc-arm copy).
func appendCrit(msg string, crit bool) string {
	if !crit {
		return msg
	}
	return msg + " Critical!"
}

// mitigateDamage runs the two-stage chain (armor for Phys, MDef for Magic),
// floored at 0; Heal/Buff/None pass through. Shared by both damage helpers so the
// phys-then-magic order can't drift; each side supplies its own armor/mdef.
func mitigateDamage(raw int, tag core.SkillTag, armor, mdef int) int {
	d := core.ApplyArmor(raw, tag, armor)
	d = core.ApplyMagicDefense(d, tag, mdef)
	if d < 0 {
		d = 0
	}
	return d
}

// damageEnemy applies rawDamage to the enemy at slot (mitigated per tag), drives
// the popup color by quality, and returns (postArmorDamage, defeated) — callers log
// the post-armor figure so it matches the HP delta. quality may be Miss for
// non-action damage. Any damage > 0 wakes a sleeping enemy.
func damageEnemy(g *core.GameState, slot, rawDamage, quality int, tag core.SkillTag) (int, bool) {
	enemy, ok := livingEnemyAt(g, slot)
	if !ok {
		return 0, false
	}
	// Effective Armor + MDef with active debuffs folded in (enemy mirror of
	// EffectiveDefenses), so an armor/MDef debuff actually softens mitigation.
	effArmor, effMDef := core.EffectiveEnemyDefenses(enemy)
	damage := mitigateDamage(rawDamage, tag, effArmor, effMDef)
	// Tally phys output this turn for Bloodthirst (finishActorTurn banks it as
	// lifesteal). Off-turn counters (tryRiposte) snapshot/restore around this so
	// their damage stays out of the tally. Non-phys tags don't feed it.
	if tag == core.SkillTagPhys && damage > 0 {
		g.Battle.PhysDamageThisTurn += damage
	}
	// Flash + HP-floor (shared with the party path + poison tick).
	died := core.ApplyFlatDamage(&enemy.HP, &enemy.DamageFlash, damage)
	if damage > 0 {
		enemy.DamagePopup = damage
		enemy.DamagePopupQuality = quality
		enemy.DamagePopupTimer = core.QualityResultDuration
	}
	// Recoil + wake — only on real damage (zero-damage connections don't).
	core.ApplyHitRecoil(&enemy.HitKnockback, &enemy.SleepTurns, damage)
	if !died {
		// Audible thud only on scoring hits.
		if damage > 0 {
			audio.Play(audio.SoundEnemyHit)
		}
		return damage, false
	}
	enemy.Alive = false
	clearEnemyStatusesOnDeath(enemy)
	core.EnqueueEnemyVFX(g, core.VFXDeath, slot)
	// Clear recoil on death so the corpse fades from rest, not a knocked-back
	// offset (HitKnockback was just set above, and the timers would overlap).
	enemy.HitKnockback = 0
	enemy.DeathFade = core.DeathFadeDuration
	audio.Play(audio.SoundEnemyDeath)
	// Release any prey this enemy held so they re-enter the queue next round.
	for _, idx := range core.ReleaseIngestedBy(g.Party, slot) {
		setBattleMessage(g, fmt.Sprintf("%s tumbles free.", g.Party[idx].Name))
	}
	// Repack the front row so a back-row enemy slides up to fill the gap.
	core.ShuntEnemyFormation(core.BattleMembers(g))
	return damage, true
}

// tickPoisonAfterPartyTurn ticks poison on a party member AFTER their action.
// Returns true on the fatal tick (caller's loss check picks it up).
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

// dotTickMessage is the shared DoT-tick line: fatalLine (one %s) on a kill, else
// sufferFmt (%s + %d). Poison and Bleed differ only in their two format strings.
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
	// Guard HP here so the counter only drains on a tick that actually lands:
	// damagePartyMemberPoison no-ops on a dead member, so decrementing first would
	// burn a poison turn with no damage for any caller that skips its own HP gate.
	if member.HP <= 0 || member.PoisonTurns <= 0 {
		return false
	}
	member.PoisonTurns--
	// damagePartyMemberPoison is the authoritative kill signal; it bypasses the
	// ingested lockout so poison keeps ticking on ingested prey (else a free escape).
	dealt, killed := damagePartyMemberPoison(g, index)
	setBattleMessage(g, poisonTickMessage(member.Name, dealt, killed))
	return killed
}

// applyEnemyDoTTick drains the counter and deals tickDamage as a Magic-tagged
// (armor-bypassing), Good-quality (orange popup) hit. Returns (dealt, defeated);
// the caller owns the up-front guard and formats its own line.
func applyEnemyDoTTick(g *core.GameState, index int, counter *int, tickDamage int) (int, bool) {
	*counter--
	return damageEnemy(g, index, tickDamage, core.TimingQualityGood, core.SkillTagMagic)
}

// tickPoisonForIngestedParty ticks poison on every poisoned ingested member —
// they're skipped from the queue, so their normal end-of-turn tick never fires
// (else ingest pauses the DoT). Fired once per round from beginNewRound, before
// the loss gate. No double-tick: the queue is built once and never rebuilt mid-round.
func tickPoisonForIngestedParty(g *core.GameState) {
	for i := range g.Party {
		m := &g.Party[i]
		if !m.Ingested || m.HP <= 0 || m.PoisonTurns <= 0 {
			continue
		}
		applyPartyPoisonTick(g, i)
	}
}

// tickWebbedAfterPartyTurn drains the Webbed counter at the webbed member's turn
// end (party-only today). The slow / Ingest-refusal lives in actorSpeed + handleEnemyIngest.
func tickWebbedAfterPartyTurn(g *core.GameState, actor core.ActorRef) {
	tickPartyStatusCounter(g, actor, func(m *core.PartyMember) *int { return &m.WebbedTurns }, "%s tears free of the webs.")
}

// tickConfusedAfterPartyTurn drains the Confused counter (the retarget roll is
// honored at action time; this just drains).
func tickConfusedAfterPartyTurn(g *core.GameState, actor core.ActorRef) {
	tickPartyStatusCounter(g, actor, func(m *core.PartyMember) *int { return &m.ConfusedTurns }, "%s's head clears.")
}

// tickBlessAfterPartyTurn drains every stackable buff on the member at their turn
// end, dropping expired ones and narrating each fade (so Bless + War Banner + Stone
// Skin tick independently).
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

// tickEnemyBuffAfterTurn drains every stackable debuff on the enemy at its turn
// end — the enemy mirror of tickBlessAfterPartyTurn. No-ops on party actors / dead enemies.
func tickEnemyBuffAfterTurn(g *core.GameState, actor core.ActorRef) {
	if actor.IsParty {
		return
	}
	enemy, ok := livingEnemyAt(g, actor.Index)
	if !ok || len(enemy.Debuffs) == 0 {
		return
	}
	remaining, expired := core.TickStatusMods(enemy.Debuffs)
	enemy.Debuffs = remaining
	// One line per expired source (skill), not per stat — TickStatusMods returns sources.
	for _, s := range expired {
		setBattleMessage(g, fmt.Sprintf("%s's %s wears off.", core.EnemyDisplayName(enemy), core.SkillName(s)))
	}
}

// tickRegenAfterPartyTurn applies one Renewal HoT tick at the member's turn end —
// the positive mirror of tickPoisonAfterPartyTurn. Routes the heal through
// healPartyMember (clamps, flashes, no-ops dead/ingested). Party-only.
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
	// Report HP only when a heal landed (a full-HP tick heals nothing); always
	// announce the fade on the final tick.
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

// tickPartyStatusCounter is the shared body for the non-damaging end-of-turn ticks
// (Webbed, Confused): drains the counterRef counter and, on reaching zero, emits
// clearedFmt ("%s" + name; "" for a silent clear). Damaging ticks use other seams.
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

// tickEnemyDoTAfterTurn is the shared body for enemy end-of-turn DoTs (Poison,
// Bleed): drain the counter, deal tickDamage, narrate via msg. Returns true on a
// kill. Burn is NOT routed here — it ticks at turn-START.
func tickEnemyDoTAfterTurn(g *core.GameState, actor core.ActorRef, counterOf func(*core.Enemy) *int, tickDamage int, msg func(string, int, bool) string) bool {
	if actor.IsParty {
		return false
	}
	enemy, ok := livingEnemyAt(g, actor.Index)
	if !ok {
		return false
	}
	counter := counterOf(enemy)
	if *counter <= 0 {
		return false
	}
	dealt, defeated := applyEnemyDoTTick(g, actor.Index, counter, tickDamage)
	setBattleMessage(g, msg(core.EnemyDisplayName(enemy), dealt, defeated))
	return defeated
}

// tickPoisonAfterEnemyTurn / tickBleedAfterEnemyTurn: per-status wrappers (named
// for tests). PoisonTurns and BleedTurns are separate so both DoTs run at once.
func tickPoisonAfterEnemyTurn(g *core.GameState, actor core.ActorRef) bool {
	return tickEnemyDoTAfterTurn(g, actor, func(e *core.Enemy) *int { return &e.PoisonTurns }, core.PoisonTickDamage, poisonTickMessage)
}

func tickBleedAfterEnemyTurn(g *core.GameState, actor core.ActorRef) bool {
	return tickEnemyDoTAfterTurn(g, actor, func(e *core.Enemy) *int { return &e.BleedTurns }, core.BleedTickDamage, bleedTickMessage)
}

// tickEnemyEndOfTurnDoTs runs all of an enemy's end-of-turn DoTs — the single seam
// both turn-end paths invoke, so a new DoT is added once. Returns true on a kill.
func tickEnemyEndOfTurnDoTs(g *core.GameState, actor core.ActorRef) bool {
	poisonKill := tickPoisonAfterEnemyTurn(g, actor)
	bleedKill := tickBleedAfterEnemyTurn(g, actor)
	return poisonKill || bleedKill
}

// tickBurnAtTurnStart ticks burn on a burning actor at their turn start (only
// enemies burn today; ActorRef keeps it future-proof). Returns true on a kill.
func tickBurnAtTurnStart(g *core.GameState, actor core.ActorRef) bool {
	if actor.IsParty {
		return false
	}
	enemy, ok := livingEnemyAt(g, actor.Index)
	if !ok || enemy.BurnTurns <= 0 {
		return false
	}
	// Log the post-mitigation dealt amount (burn is magic-tagged, MDef clips it) so
	// "burns for N" matches the HP drop — raw BurnTickDamage overstates it on MDef enemies.
	dealt, _ := applyEnemyDoTTick(g, actor.Index, &enemy.BurnTurns, core.BurnTickDamage)
	def := core.EnemyInfoFor(*enemy)
	if !enemy.Alive {
		setBattleMessage(g, fmt.Sprintf("%s succumbs to the flames.", core.TheEnemy(def)))
		// Repoint the cursor if the burn killed the selected enemy.
		repointEnemyCursorIfDead(g)
		return true
	}
	setBattleMessage(g, fmt.Sprintf("%s burns for %d.", core.TheEnemy(def), dealt))
	return false
}

func healPartyMember(g *core.GameState, partyIndex, amount int) bool {
	if !partyIndexValid(g, partyIndex) {
		return false
	}
	// core.HealMember owns the amount / no-revive / ingest-skip guards and returns
	// false when none let the heal land, so we only flash / ping on a real heal.
	member := &g.Party[partyIndex]
	if !core.HealMember(member, amount) {
		return false
	}
	member.DamageFlash = core.FlashDuration
	audio.Play(audio.SoundHeal)
	return true
}

// damagePartyMember applies rawAmount to a member (mitigated per tag); damage > 0
// wakes from Sleep. Returns (dealt, fatal) — callers log the dealt figure so it
// matches the HP delta. Out-of-range or amount<=0 returns (0, false).
func damagePartyMember(g *core.GameState, partyIndex, rawAmount int, tag core.SkillTag) (int, bool) {
	if !partyIndexValid(g, partyIndex) || rawAmount <= 0 {
		return 0, false
	}
	member := &g.Party[partyIndex]
	if member.HP <= 0 {
		return 0, false
	}
	// Ingested prey is sealed off from EXTERNAL damage. The poison DoT is exempt —
	// it ticks via damagePartyMemberPoison (which skips this guard).
	if member.Ingested {
		return 0, false
	}
	return applyPartyDamage(g, member, rawAmount, tag)
}

// applyPartyDamage mitigates + applies rawAmount to an ALREADY-validated living
// member (caller owns the bounds/dead/lockout checks), running the shared
// flash/recoil/wake/rumble/death bookkeeping. Split out so the poison DoT can
// reach it without the ingest lockout.
func applyPartyDamage(g *core.GameState, member *core.PartyMember, rawAmount int, tag core.SkillTag) (int, bool) {
	// EffectiveDefenses folds equipped gear onto the base values.
	armorVal, mdefVal := core.EffectiveDefenses(*member)
	amount := mitigateDamage(rawAmount, tag, armorVal, mdefVal)
	// Aegis shield soaks post-mitigation BEFORE HP; only overflow reaches HP, and
	// the bookkeeping below reads the post-shield amount.
	if amount > 0 && member.ShieldHP > 0 {
		absorbed := amount
		if absorbed > member.ShieldHP {
			absorbed = member.ShieldHP
		}
		member.ShieldHP -= absorbed
		amount -= absorbed
	}
	// Flash + HP-floor (shared with the enemy path + poison tick).
	died := core.ApplyFlatDamage(&member.HP, &member.DamageFlash, amount)
	// Damage popup; incoming hits aren't player-timed so quality is Miss (the draw
	// colors party popups with a fixed hurt tone).
	if amount > 0 {
		member.DamagePopup = amount
		member.DamagePopupQuality = int(core.TimingQualityMiss)
		member.DamagePopupTimer = core.QualityResultDuration
	}
	// Knockback + wake — only on real damage (a soaked 0 doesn't shove).
	core.ApplyHitRecoil(&member.HitKnockback, &member.SleepTurns, amount)
	// Haptic buzz on a landing hit. Taking a hit doesn't shake the camera, so this
	// arms rumble directly (TriggerCombatShake is for offensive impacts).
	if amount > 0 {
		core.TriggerRumble(&g.Battle, core.RumbleHurtStrength, core.RumbleHurtDur)
	}
	if !died {
		return amount, false
	}
	clearPartyStatusesOnDeath(member)
	// A downed member yields the front line: sink it to the back and pull a living
	// backliner up (same column first). Single hook for every death path (melee,
	// cast, DoT all land here). A future Raise re-runs ShuntPartyFormation to restore
	// the revived member to the front when a slot needs manning.
	core.ShuntPartyFormation(g.Party)
	return amount, true
}

// damagePartyMemberPoison applies one poison tick, bypassing the ingested lockout
// (poison ticks on ingested prey) but honoring the bounds/dead guards. Magic-tagged.
func damagePartyMemberPoison(g *core.GameState, partyIndex int) (int, bool) {
	if !partyIndexValid(g, partyIndex) {
		return 0, false
	}
	member := &g.Party[partyIndex]
	if member.HP <= 0 {
		return 0, false
	}
	return applyPartyDamage(g, member, core.PoisonTickDamage, core.SkillTagMagic)
}

// damagePartyMemberDefendable applies an incoming HIT honoring the Defend brace
// BEFORE mitigation (for enemy melee + damaging casts). DoT ticks call
// damagePartyMember directly so the brace doesn't soak them. A positive soak floors at 1.
func damagePartyMemberDefendable(g *core.GameState, partyIndex, rawAmount int, tag core.SkillTag) (int, bool) {
	if rawAmount > 0 && partyIndexValid(g, partyIndex) && g.Party[partyIndex].Defending {
		rawAmount = int(float32(rawAmount) * core.DefendingDamageMult)
		// Floor a positive soak at 1 (a soak, not free immunity).
		if rawAmount < 1 {
			rawAmount = 1
		}
	}
	return damagePartyMember(g, partyIndex, rawAmount, tag)
}

// clearEnemyStatusesOnDeath / clearPartyStatusesOnDeath wipe transient timed
// statuses on death, classified by the descriptors below. Enemy clears all its
// counters except Taunt (moot dead). Party clears Sleep/Webbed/Confused but KEEPS
// Poison + Stun (harmless render hint on a corpse) and the Ice Armor/Regen buffs.
// The init() assert pins each descriptor to the reflected `*Turns` field set so a
// new counter can't silently linger on a corpse.

// enemyStatusCounter is one classified timed counter: field name (for the assert),
// a live-field accessor, and whether it's wiped on death.
type enemyStatusCounter struct {
	field        string
	ptr          func(*core.Enemy) *int
	clearOnDeath bool
}

type partyStatusCounter struct {
	field        string
	ptr          func(*core.PartyMember) *int
	clearOnDeath bool
}

// enemyDeathStatuses / partyDeathStatuses classify every `*Turns` counter;
// clearOnDeath=true rows are zeroed, false rows preserved (see doc above).
var enemyDeathStatuses = []enemyStatusCounter{
	{"BurnTurns", func(e *core.Enemy) *int { return &e.BurnTurns }, true},
	{"SleepTurns", func(e *core.Enemy) *int { return &e.SleepTurns }, true},
	{"StunTurns", func(e *core.Enemy) *int { return &e.StunTurns }, true},
	{"PoisonTurns", func(e *core.Enemy) *int { return &e.PoisonTurns }, true},
	{"BleedTurns", func(e *core.Enemy) *int { return &e.BleedTurns }, true},
	{"TauntTurns", func(e *core.Enemy) *int { return &e.TauntTurns }, false},
}

var partyDeathStatuses = []partyStatusCounter{
	{"SleepTurns", func(m *core.PartyMember) *int { return &m.SleepTurns }, true},
	{"WebbedTurns", func(m *core.PartyMember) *int { return &m.WebbedTurns }, true},
	{"ConfusedTurns", func(m *core.PartyMember) *int { return &m.ConfusedTurns }, true},
	{"PoisonTurns", func(m *core.PartyMember) *int { return &m.PoisonTurns }, false},
	{"StunTurns", func(m *core.PartyMember) *int { return &m.StunTurns }, false},
	{"IceArmorTurns", func(m *core.PartyMember) *int { return &m.IceArmorTurns }, false},
	{"RegenTurns", func(m *core.PartyMember) *int { return &m.RegenTurns }, false},
}

// init asserts the death-clear descriptors are COMPLETE: every `int` field ending
// in "Turns" must be classified, else it panics at startup (can't linger on a corpse).
func init() {
	assertTurnsCountersClassified := func(structName string, sample any, classified map[string]bool) {
		t := reflect.TypeOf(sample)
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Type.Kind() != reflect.Int || len(f.Name) < len("Turns") || f.Name[len(f.Name)-len("Turns"):] != "Turns" {
				continue
			}
			if !classified[f.Name] {
				panic("battle: " + structName + "." + f.Name +
					" is an unclassified timed-status counter — add it to enemyDeathStatuses/partyDeathStatuses (clear or keep) so it can't linger on a corpse")
			}
		}
	}
	enemyClassified := make(map[string]bool, len(enemyDeathStatuses))
	for _, s := range enemyDeathStatuses {
		enemyClassified[s.field] = true
	}
	partyClassified := make(map[string]bool, len(partyDeathStatuses))
	for _, s := range partyDeathStatuses {
		partyClassified[s.field] = true
	}
	assertTurnsCountersClassified("Enemy", core.Enemy{}, enemyClassified)
	assertTurnsCountersClassified("PartyMember", core.PartyMember{}, partyClassified)
}

func clearEnemyStatusesOnDeath(enemy *core.Enemy) {
	for _, s := range enemyDeathStatuses {
		if s.clearOnDeath {
			*s.ptr(enemy) = 0
		}
	}
	enemy.Debuffs = nil
}

func clearPartyStatusesOnDeath(member *core.PartyMember) {
	for _, s := range partyDeathStatuses {
		if s.clearOnDeath {
			*s.ptr(member) = 0
		}
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

// procMessageArms holds the three log variants (defeated / proc / plain). Format
// strings use explicit arg indices (tag=%[1]s, name=%[2]s, noun=%[3]s, damage=%[4]d)
// so an arm can omit verbs it doesn't need.
type procMessageArms struct{ defeated, proc, plain string }

// procSkillMessage selects and formats the right arm — the shared 3-arm helper for
// every single-target proc skill. (fireboltMessage keeps its own 4-arm form.)
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
		// plain is unreachable (chill is guaranteed on survival) — kept for the contract.
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

// aoeSkillMessage / aoeEmptyMessage format the AoE log lines (hit + empty fallback).
func aoeSkillMessage(name, skillNoun, hitVerb string, hits, damage, quality int) string {
	return fmt.Sprintf("%s%s's %s %s %d foes for %d each.", qualityTag(quality), name, skillNoun, hitVerb, hits, damage)
}

func aoeEmptyMessage(skillNoun, emptyVerb string) string {
	return fmt.Sprintf("%s %s.", skillNoun, emptyVerb)
}

// qualityTag returns the leading "Grade! " log prefix on a hit. Miss/Nice return
// "" (Miss has its own whiff copy; Nice reads as baseline). Log text only — the
// popup still shows the full label via TimingQualityLabel.
func qualityTag(quality int) string {
	if quality == core.TimingQualityMiss || quality == core.TimingQualityNice {
		return ""
	}
	return core.TimingQualityLabel(quality) + " "
}

// bloodthirstHeal converts physDamage into Warrior Bloodthirst self-heal
// (BloodthirstHealPerRank/rank, capped at MaxHP). No-op without the node /
// downed / ingested / nothing dealt; logs the ACTUAL HP gained. Shared by the
// end-of-turn tally and the off-turn Riposte counter.
func bloodthirstHeal(g *core.GameState, partyIndex, physDamage int) {
	if !partyIndexValid(g, partyIndex) {
		return
	}
	member := &g.Party[partyIndex]
	rank := core.PassiveRank(member, core.PassiveBloodthirst)
	if rank <= 0 || physDamage <= 0 || member.HP <= 0 || member.Ingested {
		return
	}
	heal := int(float64(physDamage) * float64(rank) * core.BloodthirstHealPerRank)
	if heal <= 0 {
		return
	}
	before := member.HP
	core.GainUpTo(&member.HP, member.MaxHP, heal)
	if gained := member.HP - before; gained > 0 {
		// VFXHeal cue — lifesteal is an HP gain, not a hit.
		core.EnqueuePartyVFX(g, core.VFXHeal, partyIndex)
		setBattleMessage(g, fmt.Sprintf("%s's bloodthirst restores %d HP.", member.Name, gained))
	}
}

// applyBloodthirst banks the turn's PhysDamageThisTurn as Bloodthirst lifesteal
// (once per turn, so an AoE rolls up into one heal). No-op for enemy actors.
func applyBloodthirst(g *core.GameState, actor core.ActorRef) {
	if !actor.ValidPartyIndex(g.Party) {
		return
	}
	bloodthirstHeal(g, actor.Index, g.Battle.PhysDamageThisTurn)
}

// tryRiposte fires the Warrior's Battle Sense counter on a DODGE: an immediate
// phys strike at the attacker for RiposteDamageMult of the dodger's basic damage.
// No-op without the node or on a dead attacker.
func tryRiposte(g *core.GameState, dodger, enemySlot int) {
	if !partyIndexValid(g, dodger) {
		return
	}
	member := &g.Party[dodger]
	if core.PassiveRank(member, core.PassiveRiposte) <= 0 {
		return
	}
	enemy, ok := livingEnemyAt(g, enemySlot)
	if !ok {
		return
	}
	raw := int(float64(core.MemberAttackDamage(*member, 0)) * core.RiposteDamageMult)
	if raw < 1 {
		raw = 1
	}
	noun := core.EnemySingularNoun(*enemy)
	// Keep this counter OUT of the per-turn Bloodthirst tally (it lands on the
	// enemy's turn and lifesteals directly below): snapshot/restore around the strike.
	physTally := g.Battle.PhysDamageThisTurn
	dealt, defeated := damageEnemy(g, enemySlot, raw, core.TimingQualityGood, core.SkillTagPhys)
	g.Battle.PhysDamageThisTurn = physTally
	core.EnqueueEnemyVFX(g, core.WeaponHitVFX(core.EquippedWeapon(*member)), enemySlot)
	if defeated {
		setBattleMessage(g, fmt.Sprintf("%s ripostes — the %s drops!", member.Name, noun))
	} else if dealt > 0 {
		setBattleMessage(g, fmt.Sprintf("%s ripostes the %s for %d.", member.Name, noun, dealt))
	}
	// Feed Bloodthirst directly (the enemy-turn tally would otherwise drop it).
	bloodthirstHeal(g, dodger, dealt)
}

// tryRetribution reflects RetributionReflectPerRank of the damage TAKEN back at the
// attacker — the Cleric's Conviction passive, Magic-tagged (MDef-mitigated, no phys
// tally). No-op without the node / no damage / dead attacker.
func tryRetribution(g *core.GameState, enemySlot, defender, dealt int) {
	if dealt <= 0 || !partyIndexValid(g, defender) {
		return
	}
	// Thorns come from a LIVING ward — a downing hit draws no reflection.
	if g.Party[defender].HP <= 0 {
		return
	}
	rank := core.PassiveRank(&g.Party[defender], core.PassiveRetribution)
	if rank <= 0 {
		return
	}
	enemy, ok := livingEnemyAt(g, enemySlot)
	if !ok {
		return
	}
	reflect := int(float64(dealt) * float64(rank) * core.RetributionReflectPerRank)
	if reflect < 1 {
		return
	}
	noun := core.EnemySingularNoun(*enemy)
	refl, defeated := damageEnemy(g, enemySlot, reflect, core.TimingQualityGood, core.SkillTagMagic)
	core.EnqueueEnemyVFX(g, core.VFXSmite, enemySlot)
	if defeated {
		setBattleMessage(g, fmt.Sprintf("%s's retribution fells the %s!", g.Party[defender].Name, noun))
	} else if refl > 0 {
		setBattleMessage(g, fmt.Sprintf("The %s takes %d from %s's retribution.", noun, refl, g.Party[defender].Name))
	}
}

func resolveEnemyMiss(g *core.GameState, slot int) {
	enemy, ok := livingEnemyAt(g, slot)
	if !ok {
		return
	}
	target := pickEnemyAttackTarget(g, true) // basic attack = melee, front row only
	if target < 0 {
		return
	}
	enemy.AttackBump = core.BumpDuration
	setBattleMessage(g, fmt.Sprintf("The %s's attack misses %s!", core.EnemySingularNoun(*enemy), g.Party[target].Name))
}

// resolveEnemyAttacker applies one enemy's attack against a chosen member, scaled
// by defend quality. Returns true if the hit landed (false if attacker was dead).
func resolveEnemyAttacker(g *core.GameState, slot int, defendQuality int) bool {
	enemy, ok := livingEnemyAt(g, slot)
	if !ok {
		return false
	}
	target := pickEnemyAttackTarget(g, true) // basic attack = melee, front row only
	if target < 0 {
		return false
	}
	enemy.AttackBump = core.BumpDuration
	// Dodge precedes damage: a sidestep eats the whole swing (no damage/proc/
	// lifesteal). The defend quality is still recorded. Skills aren't dodgeable.
	if core.RollDodge(g.Rand(), core.EffectiveStats(g.Party[target])) {
		recordQuality(g, defendQuality, target, true)
		setBattleMessage(g, fmt.Sprintf("%s sidesteps the %s.", g.Party[target].Name, core.EnemySingularNoun(*enemy)))
		// Riposte on dodge (Warrior Battle Sense).
		tryRiposte(g, target, slot)
		return true
	}
	rawDamage := core.EnemyBasicDamage(enemy)
	// Enemy crit: DEX-driven RollCrit at Miss grade (no timing bonus — enemies
	// don't press a bar), keeping them on a flat crit floor.
	enemyCrit := core.RollCrit(g.Rand(), core.EffectiveEnemyStats(enemy), core.TimingQualityMiss)
	rawDamage = applyCritMultiplier(rawDamage, enemyCrit, false)
	damage := core.ScaleIncomingDamage(rawDamage, defendQuality)
	// Phys-tagged so party Armor damps it; the Defend brace (+floor-1 soak) lives in
	// damagePartyMemberDefendable. dealt is the post-armor figure for the message.
	dealt, _ := damagePartyMemberDefendable(g, target, damage, core.SkillTagPhys)
	// Impact VFX only on landed damage — a perfect block clamps to 0, and sparks
	// would undersell it.
	if dealt > 0 {
		core.EnqueuePartyVFX(g, core.VFXImpact, target)
	}
	if defendQuality > core.TimingQualityMiss {
		// A successful block recoils the defender slightly so the impact reads.
		g.Party[target].AttackBump = core.BlockBumpDuration
	}
	recordQuality(g, defendQuality, target, true)
	def := core.EnemyInfoFor(*enemy)
	setBattleMessage(g, appendCrit(enemyHitMessage(*enemy, g.Party[target].Name, dealt, defendQuality, g.Party[target].Defending), enemyCrit))
	// Poison inflict: only on a landed hit (gate on `dealt`, so an armor-soaked 0
	// inflicts no DoT). No-stack like burn.
	if dealt > 0 {
		// Raw-chance proc (no enemy-side minigame) via applyStatusRoll.
		if applyStatusRoll(g.Rand(), &g.Party[target].PoisonTurns, g.Party[target].HP <= 0,
			def.PoisonChance, core.DefaultPoisonEffect.RollDuration, core.EffectiveStats(g.Party[target]).WIS) {
			setBattleMessage(g, fmt.Sprintf("%s is poisoned!", g.Party[target].Name))
		}
	}
	// Lifesteal (Vampire Bat): heals a fraction of `dealt` (post-armor + Defend),
	// so a soaked hit drains little; rounds to 0 = no heal (no free 1-HP leak).
	if def.LifestealPercent > 0 && dealt > 0 && enemy.HP > 0 {
		heal := int(float64(dealt) * def.LifestealPercent)
		if heal > 0 {
			// Cap against the per-instance MaxHP, not the definition's base.
			core.GainUpTo(&enemy.HP, enemy.MaxHP, heal)
			setBattleMessage(g, fmt.Sprintf("%s drains life from %s (+%d HP).", core.TheEnemy(def), g.Party[target].Name, heal))
		}
	}
	// Ice Armor reprisal: a connecting attack on a frost-warded defender chills its
	// attacker (SPD debuff). Lands on contact even if Defended to 0; overwrites (no-stack).
	if g.Party[target].IceArmorTurns > 0 && enemy.HP > 0 {
		if core.StampEnemyDebuff(enemy, core.SkillIceArmor, core.SkillEffect{BuffStats: core.Stats{SPD: -core.IceArmorChillSPD}, BuffTurns: core.IceArmorChillTurns}) {
			setBattleMessage(g, fmt.Sprintf("%s's ice armor chills the %s.", g.Party[target].Name, core.EnemySingularNoun(*enemy)))
		}
	}
	// Retribution LAST (lifesteal + ice-armor chill resolved first), as it may kill the attacker.
	tryRetribution(g, slot, target, dealt)
	return true
}

// pickEnemyAttackTarget chooses (and commits the cursor to) the party member the
// acting enemy hits, round-robin via EnemyAttackCursor (separate from PartyTarget).
// melee=true restricts to the effective front row; a live Taunt overrides any row.
func pickEnemyAttackTarget(g *core.GameState, melee bool) int {
	if forced, ok := forcedTauntTarget(g); ok {
		g.Battle.EnemyAttackCursor = forced
		return forced
	}
	var target int
	if melee {
		target = core.PeekNextMeleeEnemyTarget(g)
	} else {
		target = core.PeekNextEnemyTarget(g)
	}
	if target >= 0 {
		g.Battle.EnemyAttackCursor = target
	}
	return target
}

func enemyHitMessage(enemy core.Enemy, targetName string, damage, defendQuality int, defending bool) string {
	def := core.EnemyInfoFor(enemy)
	if defendQuality > core.TimingQualityMiss {
		if damage <= 0 {
			return fmt.Sprintf("%s blocks the %s!", targetName, core.EnemySingularNoun(enemy))
		}
		return fmt.Sprintf("%s blocks the %s (%d).", targetName, core.EnemySingularNoun(enemy), damage)
	}
	if defending {
		return fmt.Sprintf("%s soaks the %s for %d.", targetName, core.EnemySingularNoun(enemy), damage)
	}
	return fmt.Sprintf("%s %s %s for %d.", core.TheEnemy(def), def.AttackVerbSingular, targetName, damage)
}
