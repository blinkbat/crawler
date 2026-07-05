package battle

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
)

// setupKind selects the pre-bar validation/cost flow a skill runs at confirm time
// (resolved by runSkillSetup). An enum, not a per-skill closure: closures in
// skillActionHandlers referencing the gate would make the map's initializer depend
// on itself (an initialization cycle once deadMsg moved into the table).
type setupKind int

const (
	setupInvalid      setupKind = iota // zero value — a row that forgot `setup:` (init-asserted out)
	setupConfirmEnemy                  // confirm a live enemy, NO MP (basic attack, Steal)
	setupTargetedGate                  // confirm live enemy/ally per TargetMode, then pay MP
	setupPayOnly                       // pay MP only (AoE / self / whole-party / Swipe)
	setupReviveAlly                    // require a DOWNED ally exists, then pay MP (Resurrect)
)

// actionApply resolves the action at the given timing quality. Returns landed=true
// if it went through; false on a defensive no-op (e.g. target died between confirm
// and apply) so the caller doesn't show a quality popup for a cancelled action.
type actionApply func(g *core.GameState, quality int) (landed bool)

type actionHandlers struct {
	setup setupKind
	apply actionApply
	// deadMsg is the downed-target refusal line for ally-targeted (ActionPartyTarget)
	// setups; "" for enemy / payOnly / whole-party setups. Folded in here (was the
	// separate targetedSetupConfig map) so each skill's whole behavior is ONE row.
	deadMsg string
}

// runSkillSetup dispatches a skill's confirm-time setup off its kind.
func runSkillSetup(g *core.GameState, skill core.SkillID, kind setupKind) bool {
	switch kind {
	case setupConfirmEnemy:
		return setupTargetedEnemy(g)
	case setupTargetedGate:
		return runTargetedSetup(g, skill)
	case setupPayOnly:
		return chargeMP(g, skill)
	case setupReviveAlly:
		return setupReviveAndPay(g, skill)
	}
	panic("battle: skill " + core.SkillName(skill) + " has an invalid setup kind")
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
	// SkillNone is the basic attack — the implicit "no skill selected" row, in the
	// registry so dispatch is a single lookup (no special-case branch).
	core.SkillNone:            {setup: setupConfirmEnemy, apply: applyAttack},
	core.SkillSwipe:           {setup: setupPayOnly, apply: applySwipe},
	core.SkillPrayer:          {setup: setupTargetedGate, apply: applyPrayer, deadMsg: "Prayer cannot revive."},
	core.SkillSteal:           {setup: setupConfirmEnemy, apply: applySteal},
	core.SkillScan:            {setup: setupTargetedGate, apply: applyScan},
	core.SkillFirebolt:        {setup: setupTargetedGate, apply: applyFirebolt},
	core.SkillCrushingBlow:    {setup: setupTargetedGate, apply: applyCrushingBlow},
	core.SkillWhirlwind:       {setup: setupPayOnly, apply: applyWhirlwind},
	core.SkillMassMend:        {setup: setupPayOnly, apply: applyMassMend},
	core.SkillSmite:           {setup: setupTargetedGate, apply: applySmite},
	core.SkillBackstab:        {setup: setupTargetedGate, apply: applyBackstab},
	core.SkillVenomStrike:     {setup: setupTargetedGate, apply: applyVenomStrike},
	core.SkillFrostLance:      {setup: setupTargetedGate, apply: applyFrostLance},
	core.SkillArcBolt:         {setup: setupPayOnly, apply: applyArcBolt},
	core.SkillBless:           {setup: setupPayOnly, apply: applyBless},
	core.SkillFireball:        {setup: setupPayOnly, apply: applyFireball},
	core.SkillPoisonCloud:     {setup: setupPayOnly, apply: applyPoisonCloud},
	core.SkillCleanse:         {setup: setupTargetedGate, apply: applyCleanse, deadMsg: "Cleanse can't reach the fallen."},
	core.SkillSecondWind:      {setup: setupPayOnly, apply: applySecondWind},
	core.SkillRenewal:         {setup: setupTargetedGate, apply: applyRenewal, deadMsg: "Renewal can't reach the fallen."},
	core.SkillCripple:         {setup: setupTargetedGate, apply: applyCripple},
	core.SkillFrostbite:       {setup: setupTargetedGate, apply: applyFrostbite},
	core.SkillCorrosiveVial:   {setup: setupTargetedGate, apply: applyCorrosiveVial},
	core.SkillConeOfCold:      {setup: setupPayOnly, apply: applyConeOfCold},
	core.SkillSunder:          {setup: setupTargetedGate, apply: applySunder},
	core.SkillTaunt:           {setup: setupTargetedGate, apply: applyTaunt},
	core.SkillWarBanner:       {setup: setupPayOnly, apply: applyWarBanner},
	core.SkillStoneSkin:       {setup: setupTargetedGate, apply: applyStoneSkin, deadMsg: "Stone Skin can't reach the fallen."},
	core.SkillBlind:           {setup: setupTargetedGate, apply: applyBlind},
	core.SkillAegis:           {setup: setupTargetedGate, apply: applyAegis, deadMsg: "Aegis can't reach the fallen."},
	core.SkillSmokeBomb:       {setup: setupPayOnly, apply: applySmokeBomb},
	core.SkillIceArmor:        {setup: setupPayOnly, apply: applyIceArmor},
	core.SkillRend:            {setup: setupTargetedGate, apply: applyRend},
	core.SkillLacerate:        {setup: setupTargetedGate, apply: applyLacerate},
	core.SkillSearingLight:    {setup: setupTargetedGate, apply: applySearingLight},
	core.SkillImmolate:        {setup: setupPayOnly, apply: applyImmolate},
	core.SkillMug:             {setup: setupTargetedGate, apply: applyMug},
	core.SkillChainLightning:  {setup: setupPayOnly, apply: applyChainLightning},
	core.SkillStaticField:     {setup: setupTargetedGate, apply: applyStaticField},
	core.SkillGuard:           {setup: setupTargetedGate, apply: applyGuard, deadMsg: "Guard can't cover the fallen."},
	core.SkillConsecrate:      {setup: setupPayOnly, apply: applyConsecrate},
	core.SkillJudgment:        {setup: setupTargetedGate, apply: applyJudgment},
	core.SkillRecklessSwing:   {setup: setupTargetedGate, apply: applyRecklessSwing},
	core.SkillCombust:         {setup: setupTargetedGate, apply: applyCombust},
	core.SkillBulwark:         {setup: setupPayOnly, apply: applyBulwark},
	core.SkillDispel:          {setup: setupTargetedGate, apply: applyDispel},
	core.SkillVanish:          {setup: setupPayOnly, apply: applyVanish},
	core.SkillMartyrsBond:     {setup: setupTargetedGate, apply: applyMartyrsBond, deadMsg: "Martyr's Bond can't reach the fallen."},
	core.SkillResurrect:       {setup: setupReviveAlly, apply: applyResurrect},
	core.SkillMeteor:          {setup: setupPayOnly, apply: applyMeteor},
	core.SkillAncestralSpirit: {setup: setupPayOnly, apply: applyAncestralSpirit},
}

// setupReviveAndPay refuses (no MP) when no ally is down, else pays — Resurrect's gate.
func setupReviveAndPay(g *core.GameState, skill core.SkillID) bool {
	if core.FirstDownedPartyMember(g.Party) < 0 {
		setBattleStatus(g, "No fallen ally to revive.")
		return false
	}
	return chargeMP(g, skill)
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
	// Reverse walk (mirrors the enemy side): a handler for a skill that isn't
	// PlayerCastable is dead dispatch — catch the stale entry rather than let it rot.
	for s := range skillActionHandlers {
		if s != core.SkillNone && !core.SkillPlayerCastable(s) {
			panic("battle: skillActionHandlers has a handler for " + core.SkillName(s) + " but its registry entry isn't PlayerCastable — remove the handler or flip the flag")
		}
	}
	// Enemy-castable consistency, both directions: every EnemyCastable skill needs
	// an enemySpellHandlers entry, and the reverse walk below catches stale handlers.
	for _, s := range core.EnemyCastableSkills() {
		if _, ok := enemySpellHandlers[s]; !ok {
			panic("battle: EnemyCastable skill " + core.SkillName(s) + " has no enemySpellHandlers entry — register a handler in actions.go")
		}
	}
	for s := range enemySpellHandlers {
		if !core.IsEnemyCastable(s) {
			panic("battle: enemySpellHandlers has a handler for " + core.SkillName(s) + " but its registry entry isn't EnemyCastable — clear the handler or flip the flag")
		}
	}
	// deadMsg contract: every ally-targeted (ActionPartyTarget) row must carry a
	// downed-target refusal line (else setupTargetedAllyAndPay shows ""), and only
	// ally rows may carry one — a deadMsg on a non-ally skill is dead text.
	for s, h := range skillActionHandlers {
		if h.setup == setupInvalid {
			panic("battle: skill " + core.SkillName(s) + " has no setup kind — set one in skillActionHandlers")
		}
		if targetedSetupIsAlly(s) && h.deadMsg == "" {
			panic("battle: ally-targeted skill " + core.SkillName(s) + " needs a downed-target refusal message (deadMsg)")
		}
		if !targetedSetupIsAlly(s) && h.deadMsg != "" {
			panic("battle: non-ally skill " + core.SkillName(s) + " carries a deadMsg but has no ally target to refuse")
		}
	}
}

// targetedSetupIsAlly reports whether a table-gated skill targets an ally, read
// from the skill definition (single source) rather than a parallel flag.
func targetedSetupIsAlly(skill core.SkillID) bool {
	return core.SkillTargetMode(skill) == core.ActionPartyTarget
}

// runTargetedSetup is the table-driven single-target gate: runs the enemy/ally
// check (per the skill's TargetMode) and pays MP. Setup commits the cost; the only
// refund path is target-death before apply, handled by ensureAlive*OrCancel.
func runTargetedSetup(g *core.GameState, skill core.SkillID) bool {
	if targetedSetupIsAlly(skill) {
		return setupTargetedAllyAndPay(g, skill, skillActionHandlers[skill].deadMsg)
	}
	return setupTargetedEnemyAndPay(g, skill)
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

// enemyLine builds "<Enemy><rest>" — the subject-prefixed log line for bespoke
// enemy-spell phrasings that DON'T follow the "casts <Skill> —" convention
// (enemySpellLog). Caller picks the log category (setBattleMessage / …Cat).
func (ctx enemySpellCtx) enemyLine(rest string, args ...any) string {
	return core.TheEnemy(ctx.def) + fmt.Sprintf(rest, args...)
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
	enemySpellLogCat(ctx, core.LogInfo, rest, args...)
}

// enemySpellLogCat is enemySpellLog with an explicit color category (damaging casts pass LogDamageParty).
func enemySpellLogCat(ctx enemySpellCtx, cat core.LogCategory, rest string, args ...any) {
	tail := fmt.Sprintf(rest, args...)
	setBattleMessageCat(ctx.g, fmt.Sprintf("%s casts %s — %s", core.TheEnemy(ctx.def), ctx.skillName, tail), cat)
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
		setBattleMessageCat(g, ctx.enemyLine(" incinerates %s.", g.Party[ctx.target].Name), core.LogDamageParty)
	} else {
		enemySpellLogCat(ctx, core.LogDamageParty, "%s burns for %d.", g.Party[ctx.target].Name, dealt)
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
		setBattleMessage(g, ctx.enemyLine(" lunges, but finds no one to seize."))
		return false
	}
	m := &g.Party[picked]
	// Webbed targets refuse Ingest — Webbed is "tempo control without removal," so
	// the web shields the prey. The mantrap bites instead this turn.
	if m.WebbedTurns > 0 {
		setBattleMessage(g, ctx.enemyLine(" lunges, but %s is too tangled to swallow.", m.Name))
		return false
	}
	m.Ingested = true
	m.IngestedBy = ctx.slot
	m.SleepTurns = 0
	m.Defending = false
	// VFX anchors at the MANTRAP (ctx.slot), not the prey: spawnIngest converges
	// motes inward, reading as "prey's essence flowing INTO the swallower."
	core.EnqueueEnemyVFX(g, vfxKindFor(core.SkillIngest), ctx.slot)
	setBattleMessage(g, ctx.enemyLine(" engulfs %s!", m.Name))
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

// enemyStatusCast parameterizes the shared enemy status-cast skeleton (Sleep /
// Web / Confuse): pick the counter, roll the duration, floor it, fire the VFX, and
// narrate the already-afflicted short-circuit + success. counter is the write-through
// pointer into the target member.
type enemyStatusCast struct {
	counter  func(*core.PartyMember) *int
	duration func(ctx enemySpellCtx) int
	floor    int
	vfxSkill core.SkillID
	already  func(ctx enemySpellCtx, m *core.PartyMember) // already-afflicted log line
	success  func(ctx enemySpellCtx, m *core.PartyMember) // landed log line
}

// handleEnemyStatusCast is the table-driven body for Sleep / Web / Confuse: index
// guard, dead-target bail, already-afflicted short-circuit (the cast still spent the
// turn → true), then applyEnemyStatus + success. Counters are clear-on-death
// (partyDeathStatuses), so a 0-HP target never reaches the already-afflicted branch.
func handleEnemyStatusCast(ctx enemySpellCtx, cast enemyStatusCast) bool {
	g := ctx.g
	if !partyIndexValid(g, ctx.target) {
		return false
	}
	m := &g.Party[ctx.target]
	if m.HP <= 0 {
		return false
	}
	counter := cast.counter(m)
	if *counter > 0 {
		cast.already(ctx, m)
		return true
	}
	// A high-WIS target can shrug a dedicated status cast off entirely — the enemy
	// still spent its action (true). Procs resist silently; a focused cast narrates.
	if core.RollStatusResist(g.Rand(), core.EffectiveStats(*m).WIS) {
		enemySpellLog(ctx, "%s resists!", m.Name)
		return true
	}
	if applyEnemyStatus(ctx, counter, cast.duration(ctx), cast.floor, cast.vfxSkill) {
		cast.success(ctx, m)
		return true
	}
	return false
}

// handleEnemySleep applies the Sleep cast; already-asleep targets short-circuit.
func handleEnemySleep(ctx enemySpellCtx) bool {
	return handleEnemyStatusCast(ctx, enemyStatusCast{
		counter:  func(m *core.PartyMember) *int { return &m.SleepTurns },
		duration: func(ctx enemySpellCtx) int { return ctx.effect.SleepDuration(ctx.g.Rand()) },
		floor:    core.SleepMinTurns,
		vfxSkill: core.SkillSleep,
		// Fired at a valid target, just already asleep — the enemy spent its action (true).
		already: func(ctx enemySpellCtx, m *core.PartyMember) { enemySpellLog(ctx, "%s is already asleep.", m.Name) },
		success: func(ctx enemySpellCtx, m *core.PartyMember) { enemySpellLog(ctx, "%s falls asleep.", m.Name) },
	})
}

// handleEnemyWeb applies the Webbed status; already-webbed targets short-circuit
// (no stacking).
func handleEnemyWeb(ctx enemySpellCtx) bool {
	return handleEnemyStatusCast(ctx, enemyStatusCast{
		counter:  func(m *core.PartyMember) *int { return &m.WebbedTurns },
		duration: func(ctx enemySpellCtx) int { return ctx.effect.BindDuration(ctx.g.Rand()) },
		floor:    core.SpiderWebbedMinTurns,
		vfxSkill: core.SkillWeb,
		already: func(ctx enemySpellCtx, m *core.PartyMember) {
			setBattleMessage(ctx.g, ctx.enemyLine(" spins a fresh web at %s — already webbed.", m.Name))
		},
		success: func(ctx enemySpellCtx, m *core.PartyMember) {
			enemySpellLog(ctx, "%s is wrapped in sticky webs.", m.Name)
		},
	})
}

// handleEnemyConfuse applies the Confused status; already-confused short-circuits.
// WIS resistance (full-resist roll + duration shorten) is in handleEnemyStatusCast.
func handleEnemyConfuse(ctx enemySpellCtx) bool {
	return handleEnemyStatusCast(ctx, enemyStatusCast{
		counter:  func(m *core.PartyMember) *int { return &m.ConfusedTurns },
		duration: func(ctx enemySpellCtx) int { return ctx.effect.ConfuseDuration(ctx.g.Rand()) },
		floor:    core.WispConfuseMinTurns,
		vfxSkill: core.SkillConfuse,
		already: func(ctx enemySpellCtx, m *core.PartyMember) {
			setBattleMessage(ctx.g, ctx.enemyLine(" flickers at %s — already disoriented.", m.Name))
		},
		success: func(ctx enemySpellCtx, m *core.PartyMember) { enemySpellLog(ctx, "%s grows confused.", m.Name) },
	})
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
		setBattleMessage(g, ctx.enemyLine(" raises stone fists, but finds no targets."))
	case kills > 0:
		setBattleMessageCat(g, ctx.enemyLine(" slams the ground — %d crushed.", kills), core.LogDamageParty)
	default:
		setBattleMessageCat(g, ctx.enemyLine(" slams the ground — the whole party staggers."), core.LogDamageParty)
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
		setBattleMessage(g, ctx.enemyLine(" gestures, but the bones refuse to rise."))
		return false
	}
	skeleton := core.NewEnemy(core.EnemySkeleton)
	skeleton.Summoned = true // a Flee drops summons so the pack reverts to its authored roster
	pack.Members = append(pack.Members, skeleton)
	// The skeleton enters the queue automatically — beginNewRound rebuilds it from
	// the expanded pack.Members, so it acts the round AFTER this cast. The append
	// reallocates pack.Members (it's at exact capacity), dangling ctx.enemy — but the
	// AttackBump is applied by resolveEnemySpell on the slot-refetched live caster.
	setBattleMessage(g, ctx.enemyLine(" incants — a skeleton claws up from the ground!"))
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

// mpRefusalMessage is the one "not enough MP" wording, shared by the skill-menu
// pre-gate and chargeMP's deduct-time refusal so the two can't drift.
func mpRefusalMessage(skill core.SkillID) string {
	return fmt.Sprintf("%s needs %d MP.", core.SkillName(skill), core.SkillCost(skill))
}

// chargeMP spends the skill's MP cost or refuses, flashing mpRefusalMessage. The
// single MP chokepoint.
func chargeMP(g *core.GameState, skill core.SkillID) bool {
	// Free casts (debug / Overcharge) must not later REFUND — stamp unpaid first
	// so refundSkillMP can't mint MP that was never spent.
	g.Battle.PendingSkillPaid = false
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
	// Overcharge (Wizard capstone): a chance the cast is free — skip the spend entirely.
	if core.PassiveRank(actor, core.PassiveOvercharge) > 0 && g.Rand().Float64() < core.OverchargeFreeCastChance {
		return true
	}
	if !core.SpendSkillMP(actor, skill) {
		setBattleStatus(g, mpRefusalMessage(skill))
		return false
	}
	g.Battle.PendingSkillPaid = true
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

// forEachTargetableEnemy walks the enemies a whole-pack skill can actually reach:
// a MELEE AoE (Swipe / Whirlwind / Poison Cloud) is front-gated like a melee swing,
// so it hits only the effective front row and skips Flying foes (melee-immune); ranged/
// magic AoE (Fireball / Arc Bolt / Cone of Cold) sweeps the whole pack. One source so
// every sweep respects reach.
func forEachTargetableEnemy(g *core.GameState, skill core.SkillID, fn func(slot int, enemy *core.Enemy)) {
	// Snapshot reachable slots BEFORE any callback runs (see targetableEnemySlots): a
	// melee AoE that kills a front-row foe shunts a back-row foe up to RowFront
	// mid-sweep; re-evaluating per slot would then strike a foe covered at cast start.
	for _, slot := range targetableEnemySlots(g, skill) {
		fn(slot, core.BattleMemberAt(g, slot))
	}
}

// targetableEnemySlots returns the slots a sweep of `skill` reaches at the moment of
// the call: a melee AoE (Swipe/Whirlwind) is front-gated and skips Flyers; ranged/
// magic AoE sweeps the whole pack. Snapshot ONCE and reuse across a multi-pass attack
// (Swipe) so felling the front row can't promote the back row into a later pass.
func targetableEnemySlots(g *core.GameState, skill core.SkillID) []int {
	members := core.BattleMembers(g)
	var slots []int
	for slot := range members {
		if core.AoEReachesEnemy(members, skill, slot) {
			slots = append(slots, slot)
		}
	}
	return slots
}

// autoStrikeQuality is the popup grade for damage that ISN'T player-timed (DoT
// ticks, ancestral shade, off-turn reflect, Meteor) — Good gives the orange popup
// tone without implying the player nailed a press. One home for that convention.
const autoStrikeQuality = core.TimingQualityGood

// triggerBigShake stacks the "costly hit" camera punch (AoE casts, Swipe, crits)
// onto any grade shake already armed this beat, so a crit on a perfect press
// shakes harder than either alone.
func triggerBigShake(g *core.GameState) {
	core.AddCombatShake(&g.Battle, core.CombatShakeBigPeak, core.CombatShakeBigDur)
}

// applyAoEDamageToSlots hits a FIXED slot set for `damage` via damageEnemyCrit
// (SkillTag armor rules apply; the sweep's single crit roll multiplies post-armor
// per target). A slot whose foe an earlier pass already felled is skipped (no-op on
// the dead). Returns the count of foes actually struck this pass. Swipe snapshots its
// front row ONCE and re-hits that same set every tally pass, so a killed front row
// can't bleed the sweep into the back row.
func applyAoEDamageToSlots(g *core.GameState, skill core.SkillID, slots []int, damage, quality int, crit bool) int {
	hits := 0
	tag := core.SkillTagFor(skill)
	vfx := vfxKindFor(skill)
	for _, slot := range slots {
		if _, ok := livingEnemyAt(g, slot); !ok {
			continue
		}
		damageEnemyCrit(g, slot, damage, quality, tag, crit, false)
		core.EnqueueEnemyVFX(g, vfx, slot)
		hits++
	}
	return hits
}

// vfxNoneExempt is the frozen set of castable skills that intentionally queue NO
// particle effect — utility casts with their own feedback (Scan IDs a foe,
// Taunt/SmokeBomb only alter targeting, RaiseBones summons). init() asserts this
// set EXACTLY matches the castable skills vfxKindFor returns VFXNone for, so a new
// damaging/status skill that forgets a vfxKindFor case trips a startup panic
// instead of silently rendering nothing.
var vfxNoneExempt = map[core.SkillID]bool{
	core.SkillScan:       true,
	core.SkillTaunt:      true,
	core.SkillSmokeBomb:  true,
	core.SkillRaiseBones: true,
	core.SkillVanish:     true, // stealth — only alters targeting, no particle
}

func init() {
	for _, s := range append(core.PlayerCastableSkills(), core.EnemyCastableSkills()...) {
		if vfxKindFor(s) == core.VFXNone && !vfxNoneExempt[s] {
			panic("battle: castable skill " + core.SkillName(s) + " maps to VFXNone in vfxKindFor — add a case (or add it to vfxNoneExempt if it intentionally has no effect)")
		}
	}
	for s := range vfxNoneExempt {
		if vfxKindFor(s) != core.VFXNone {
			panic("battle: " + core.SkillName(s) + " is in vfxNoneExempt but vfxKindFor returns a real effect — remove the exemption")
		}
	}
}

// vfxKindFor maps a skill to the particle effect its apply step queues. Callers
// choose the enqueue direction. Unmapped skills return VFXNone, valid only for the
// vfxNoneExempt set (init asserts this — see above).
func vfxKindFor(skill core.SkillID) core.VFXKind {
	switch skill {
	case core.SkillSwipe, core.SkillWhirlwind, core.SkillCrushingBlow, core.SkillBackstab, core.SkillSunder, core.SkillRend, core.SkillLacerate, core.SkillMug, core.SkillRecklessSwing:
		return core.VFXSlash
	case core.SkillArcBolt, core.SkillChainLightning, core.SkillStaticField, core.SkillDispel:
		return core.VFXArc
	case core.SkillFirebolt, core.SkillFireball, core.SkillImmolate, core.SkillCombust, core.SkillMeteor:
		return core.VFXEmber
	case core.SkillSmite, core.SkillBlind, core.SkillSearingLight, core.SkillConsecrate, core.SkillJudgment:
		return core.VFXSmite
	case core.SkillVenomStrike, core.SkillPoisonCloud, core.SkillCripple, core.SkillCorrosiveVial:
		return core.VFXVenom
	case core.SkillFrostLance, core.SkillFrostbite, core.SkillConeOfCold, core.SkillIceArmor:
		return core.VFXFrost
	case core.SkillPrayer, core.SkillMassMend, core.SkillBless, core.SkillCleanse, core.SkillSecondWind, core.SkillRenewal, core.SkillWarBanner, core.SkillStoneSkin, core.SkillAegis, core.SkillGuard, core.SkillBulwark, core.SkillMartyrsBond, core.SkillResurrect, core.SkillAncestralSpirit:
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

// enqueueSkillVFXAtEnemy queues skill's VFX on the current enemy cursor
// (g.Battle.EnemyIndex). The single seam for the hand-rolled single-target
// handlers that don't run through applyProcStrike's shared body.
func enqueueSkillVFXAtEnemy(g *core.GameState, skill core.SkillID) {
	core.EnqueueEnemyVFX(g, vfxKindFor(skill), g.Battle.EnemyIndex)
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
// non-positive chance / already-running counter (no-stack), else rolls the proc and
// stamps the WIS-resisted duration. A high-WIS target may shrug it off entirely
// (ResistStatusDuration → 0), which reads as a silent failed proc — the Amoeba's
// answer to status-spam. Used by tryProcStatus and the enemy basic-attack proc.
// Returns true when just stamped.
func applyStatusRoll(rng *rand.Rand, counter *int, defeated bool, chance float64, durationFn func(*rand.Rand) int, resistWis int) bool {
	if defeated || chance <= 0 || counter == nil || *counter > 0 {
		return false
	}
	if rng.Float64() >= chance {
		return false
	}
	*counter = core.ResistStatusDuration(rng, durationFn(rng), resistWis)
	return *counter > 0
}

// refundSkillMP returns setup-committed MP when an action is cancelled at apply.
// Refunds ONLY what chargeMP actually spent (PendingSkillPaid): a debug or
// Overcharge free cast paid nothing, so refunding it would mint MP.
func refundSkillMP(g *core.GameState, refundSkill core.SkillID) {
	if refundSkill != core.SkillNone && g.Battle.PendingSkillPaid {
		if cost := core.SkillCost(refundSkill); cost > 0 {
			// Mirror chargeMP's setup-side guard: bail if the caster's slot went out
			// of range between confirm and apply rather than panicking on the index.
			if !partyIndexValid(g, g.Battle.CurrentParty) {
				return
			}
			actor := &g.Party[g.Battle.CurrentParty]
			core.GainUpTo(&actor.MP, actor.MaxMP, cost)
			g.Battle.PendingSkillPaid = false
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
	return cancelGoneTarget(g, refundSkill)
}

// cancelGoneTarget is the shared "target gone" tail for the ensureAlive*OrCancel
// guards: refund the committed MP, post "No target.", end the turn, return false.
func cancelGoneTarget(g *core.GameState, refundSkill core.SkillID) bool {
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
	return cancelGoneTarget(g, refundSkill)
}

// beginSingleTargetSkill is the shared head of every single-enemy-target damaging
// skill: refund-on-dead gate, actor lookup, attack-bump, raw-damage roll, and a
// pre-hit target snapshot (message builders want the foe's state before it dies).
// ok=false means cancelled (MP already refunded). `live` is the write-through
// pointer to the targeted enemy (for status procs / armor edits) — valid through
// the handler since damageEnemy never reallocates the pack; `target` is its pre-hit
// snapshot. rawDamage is pre-armor; callers may mutate it (crit doublers) before
// damageEnemy. effect is the tier-folded skill effect, resolved once here.
func beginSingleTargetSkill(g *core.GameState, skill core.SkillID, quality int) (actor *core.PartyMember, live *core.Enemy, target core.Enemy, rawDamage, resistWIS int, effect core.SkillEffect, ok bool) {
	if !ensureAliveTargetOrCancel(g, skill) {
		return nil, nil, core.Enemy{}, 0, 0, core.SkillEffect{}, false
	}
	actor = &g.Party[g.Battle.CurrentParty]
	stampPartyBump(actor)
	live = core.BattleMemberAt(g, g.Battle.EnemyIndex)
	target = *live
	// resistWIS hoisted here so status-proc callers don't re-derive it.
	resistWIS = core.EffectiveEnemyStats(&target).WIS
	effect = core.EffectiveSkillEffect(actor, skill)
	// Outgoing single-target passives fold in pre-crit (so each rides the multiplier),
	// after the target is resolved (Shatter reads its control state).
	rawDamage = applyOutgoingPassives(g, actor, &target, scaleSkillDamage(actor, skill, quality))
	return actor, live, target, rawDamage, resistWIS, effect, true
}

// applyOutgoingPassives folds the single-target damage-out passives into a raw hit:
// Shadow Step (strike-first), Crimson Rampage (low-HP Warrior), Shatter (vs a
// stunned/frozen foe). Each no-ops without its node. Shared by the skill head and
// the basic attack so the three apply from one place.
func applyOutgoingPassives(g *core.GameState, actor *core.PartyMember, target *core.Enemy, raw int) int {
	raw = applyShadowStep(g, actor, raw)
	raw = applyCrimsonRampage(actor, raw)
	raw = applyShatter(actor, target, raw)
	return raw
}

// applyCrimsonRampage folds the Warrior's Fury capstone: +CrimsonRampageMaxBonus ×
// (missing HP fraction) × rank, so a near-dead Warrior hits hardest. rank 0 / full
// HP / no damage = no-op.
func applyCrimsonRampage(actor *core.PartyMember, raw int) int {
	rank := core.PassiveRank(actor, core.PassiveCrimsonRampage)
	if rank <= 0 || raw <= 0 || actor.MaxHP <= 0 {
		return raw
	}
	missing := 1 - float64(actor.HP)/float64(actor.MaxHP)
	if missing <= 0 {
		return raw
	}
	// Cap the bonus at CrimsonRampageMaxBonus regardless of rank: it's a single-rank
	// capstone today, but the ×rank term would silently scale past +100% if MaxRank
	// were ever bumped.
	bonus := missing * core.CrimsonRampageMaxBonus * float64(rank)
	if bonus > core.CrimsonRampageMaxBonus {
		bonus = core.CrimsonRampageMaxBonus
	}
	return raw + int(float64(raw)*bonus)
}

// applyShatter folds the Wizard's Cryomancy capstone: +ShatterBonusVsControlled
// against a Stunned foe (Frost Lance's "frozen" reuses StunTurns). rank 0 / target
// not controlled / no damage = no-op.
func applyShatter(actor *core.PartyMember, target *core.Enemy, raw int) int {
	if raw <= 0 || target == nil || target.StunTurns <= 0 {
		return raw
	}
	rank := core.PassiveRank(actor, core.PassiveShatter)
	if rank <= 0 {
		return raw
	}
	return raw + int(float64(raw)*core.ShatterBonusVsControlled*float64(rank))
}

// beginPartyAction is the shared head of self / ally / AoE / utility apply
// handlers: resolves the acting member and stamps AttackBump. The slot is always
// valid here (CurrentParty was captured before the bar resolved), so no re-check.
func beginPartyAction(g *core.GameState) *core.PartyMember {
	actor := &g.Party[g.Battle.CurrentParty]
	stampPartyBump(actor)
	return actor
}

// beginEnemyTargetAction is the shared opener for single-enemy-target skills that
// pre-roll their own damage (so they bypass beginSingleTargetSkill's fixed rawDamage):
// it cancels + refunds MP on a dead/gone target (ok=false, caller returns false), else
// commits the actor's bump and returns the acting member + the live target foe.
func beginEnemyTargetAction(g *core.GameState, skill core.SkillID) (actor *core.PartyMember, enemy *core.Enemy, ok bool) {
	if !ensureAliveTargetOrCancel(g, skill) {
		return nil, nil, false
	}
	return beginPartyAction(g), core.BattleMemberAt(g, g.Battle.EnemyIndex), true
}

// stampPartyBump / stampEnemyBump arm the attack-lunge offset on a combatant, so
// the bump duration is applied from one place across every attacker path.
func stampPartyBump(m *core.PartyMember) { m.AttackBump = core.BumpDuration }
func stampEnemyBump(e *core.Enemy)       { e.AttackBump = core.BumpDuration }

// beginPendingAction validates / pays cost once the target is confirmed and, on
// success, arms the timing bar.
func beginPendingAction(g *core.GameState) {
	handler, ok := actionHandlerFor(g.Battle.PendingSkill)
	if !ok {
		resetBattleAction(g)
		setBattleStatus(g, msgNoSkillReady)
		return
	}
	if !runSkillSetup(g, g.Battle.PendingSkill, handler.setup) {
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
		// Log it (not setBattleStatus): the bar arms the same frame and would
		// overwrite the transient prompt, so the notice would never show.
		confuseFumbleRetarget(g, rng, battleEnemyTargets(g), &g.Battle.EnemyIndex,
			fmt.Sprintf("%s is confused — wrong target!", g.Party[actor].Name))
	case core.ActionPartyTarget, core.ActionItemTarget:
		// Both ally-target pickers re-roll among the living party on a fumble.
		confuseFumbleRetarget(g, rng, core.AvailablePartyTargets(g.Party), &g.Battle.PartyTarget,
			fmt.Sprintf("%s is confused — wrong ally!", g.Party[actor].Name))
	}
}

// confuseFumbleRetarget re-rolls a confused actor's cursor among slots and, if it
// actually moved, points it at the new pick and logs msg. Shared by the enemy- and
// ally-target arms of maybeConfuseRetarget (they differ only in slots/cursor/msg).
func confuseFumbleRetarget(g *core.GameState, rng *rand.Rand, slots []int, cursor *int, msg string) {
	if len(slots) == 0 {
		return
	}
	picked := slots[rng.Intn(len(slots))]
	if picked != *cursor {
		*cursor = picked
		setBattleMessage(g, msg)
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
	// AttackBump fires unconditionally (beginPartyAction stamps it) — the swing
	// plays even on a whiff.
	attacker := beginPartyAction(g)
	target := *core.BattleMemberAt(g, g.Battle.EnemyIndex)
	// Flying foes are immune to a melee swing — only a ranged weapon connects. Target
	// selection already bars picking one in melee; this is the defense-in-depth hard
	// gate (no damage, turn spent), and the swing still plays via AttackBump.
	if core.EnemyFlying(&target) && !core.MemberMeleeReachesFlyer(*attacker) {
		setBattleMessage(g, fmt.Sprintf("%s%s can't reach the airborne %s.", qualityTag(quality), attacker.Name, core.EnemySingularNoun(&target)))
		finishActorTurn(g)
		return true
	}
	// Accuracy roll (basic attack only): DEX + timing drive hit chance, clamped past
	// 1.0 so high-DEX/high-grade essentially never whiff. The swing + timing popup
	// still play on a miss; only damage is withheld.
	if !core.MemberAttackHits(g.Rand(), *attacker, quality) {
		setBattleMessage(g, fmt.Sprintf("%s%s swings wide.", qualityTag(quality), attacker.Name))
		finishActorTurn(g)
		return true
	}
	// Defender dodge: a connecting swing can still be sidestepped (symmetric with
	// the party-side dodge). Skills are NOT dodgeable.
	if core.RollDodge(g.Rand(), core.EffectiveEnemyStats(&target)) {
		setBattleMessage(g, fmt.Sprintf("%s%s lunges but %s slips aside.", qualityTag(quality), attacker.Name, core.TheFoe(&target)))
		finishActorTurn(g)
		return true
	}
	// Basic Attack: MemberAttackDamage (STR/DEX per weapon) scaled by quality,
	// Phys-tagged so armor damps it (strikeWithCrit's SkillNone fallback). The log
	// uses damageEnemy's POST-armor figure so it matches the HP delta.
	rawDamage := applyOutgoingPassives(g, attacker, &target, core.ScaleDamage(core.MemberAttackDamage(*attacker, 0), quality))
	dealt, defeated, crit := strikeWithCrit(g, attacker, core.SkillNone, rawDamage, quality)
	// Glyph keyed to the weapon (blunt/ranged = impact, edged = slash). Basic
	// attacks are the only weapon-driven swing; skills pick VFX via vfxKindFor.
	core.EnqueueEnemyVFX(g, core.WeaponHitVFX(core.EquippedWeapon(*attacker)), g.Battle.EnemyIndex)
	logFoeHit(g, appendCrit(attackResultMessage(attacker.Name, target, dealt, quality, defeated), crit), defeated)
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
	crit := sweepCrit(g, actor, core.SkillSwipe, quality)
	passes := multiPressPasses(g.Battle.Timing, quality)
	// Lock the reachable front row ONCE for the whole multi-pass sweep. Re-snapshotting
	// per pass would let a pass that fells the front row promote the back row into the
	// next pass — Swipe is front-gated and must keep re-hitting the SAME foes.
	slots := targetableEnemySlots(g, core.SkillSwipe)
	// enemiesHit = distinct foes struck, captured from the FIRST pass (later passes
	// hit fewer as kills accrue; the full set was struck at least once).
	enemiesHit := 0
	anyHit := false
	for p := 0; p < passes; p++ {
		hit := applyAoEDamageToSlots(g, core.SkillSwipe, slots, damage, quality, crit)
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
		setBattleMessageCat(g, appendCrit(swipeMessage(actor.Name, enemiesHit, quality), crit), core.LogDamageFoe)
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

// setupTargetedAllyAndPay confirms an AVAILABLE ally is targeted, then pays MP. A dead/
// ingested/unselected ally refuses WITHOUT spending MP. deadMsg is the per-skill downed-
// target line. Uses the same core.PartyMemberAvailable predicate as the apply-side gate
// (ensureAlivePartyTargetOrCancel) so setup can't pass a target apply would later reject.
func setupTargetedAllyAndPay(g *core.GameState, skill core.SkillID, deadMsg string) bool {
	if !partyIndexValid(g, g.Battle.PartyTarget) {
		setBattleStatus(g, msgNoAllySelected)
		return false
	}
	if !core.PartyMemberAvailable(g.Party, g.Battle.PartyTarget) {
		setBattleStatus(g, deadMsg)
		return false
	}
	return chargeMP(g, skill)
}

// healFailedLine picks the right "the heal mended nothing" line: starving blocks
// all HP recovery (only food helps), otherwise the target was already full. starved
// and full are the skill-specific phrasings. The single home for the starving-vs-full
// branch shared by the single-ally and self heals.
func healFailedLine(target *core.PartyMember, starved, full string) string {
	if core.MemberStarving(*target) {
		return starved
	}
	return full
}

// --- Prayer (Cleric, heals an ally) ---

func applyPrayer(g *core.GameState, quality int) bool {
	// The chosen ally can die (or its slot go out of range) between confirm and this
	// apply; the shared head re-checks — refund + end the turn on a gone target — so we
	// don't index a stale slot or log a heal that never landed.
	return applyAllyTargetSkill(g, core.SkillPrayer, core.LogHeal, func(actor, target *core.PartyMember) string {
		// Prayer is Heal-kind (WIS + Effect.Heal).
		heal := core.ScaleHeal(core.SkillHealFor(actor, core.SkillPrayer), quality)
		before := target.HP
		healPartyMember(g, g.Battle.PartyTarget, heal)
		landed := target.HP - before
		if landed <= 0 {
			// Report honestly when the prayer mends nothing (starved vs already full).
			return healFailedLine(target,
				qualityLine(quality, actor.Name, " prays over %s, but they're too starved to mend.", target.Name),
				qualityLine(quality, actor.Name, " prays over %s, but they're already at full health.", target.Name))
		}
		selfTarget := g.Battle.PartyTarget == g.Battle.CurrentParty
		return prayerMessage(actor.Name, target.Name, landed, quality, selfTarget)
	})
}

// rollSteal attempts a pickpocket on live using the timing-scaled steal chance from
// effect (flat base, no DEX), transferring the looted item into the inventory on a
// landed lift. Returns the kind lifted and whether it landed. Callers own the
// narration + any bonus-damage rider; this is the one home for the chance roll and
// the one-time item transfer, shared by Steal and Mug.
func rollSteal(g *core.GameState, live *core.Enemy, effect core.SkillEffect, quality int) (core.ItemKind, bool) {
	if live.Item == core.ItemNone {
		return core.ItemNone, false
	}
	chance := core.QualityScaledChance(core.StealChance(effect.StealChance), quality)
	if g.Rand().Float64() >= chance {
		return core.ItemNone, false
	}
	kind := live.Item
	// Clear the item off the enemy so it can't be looted twice.
	live.Item = core.ItemNone
	g.Inventory = core.AddItem(g.Inventory, kind, 1)
	return kind, true
}

// --- Steal (Thief, base chance scales with quality) ---

func applySteal(g *core.GameState, quality int) bool {
	// Steal costs 0 MP; pass the skill anyway in case a cost is added.
	actor, enemy, ok := beginEnemyTargetAction(g, core.SkillSteal)
	if !ok {
		return false
	}
	if enemy.Item == core.ItemNone {
		setBattleMessage(g, msgNothingToSteal)
		finishActorTurn(g)
		// The hand still moved — landed.
		return true
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillSteal)
	if kind, stole := rollSteal(g, enemy, effect, quality); stole {
		// Steal T3 ("Cuts on lift") deals STR×StealBonusDamage on a landed steal.
		mod := core.SkillTierMod(actor, core.SkillSteal)
		var bonus int
		var defeated bool
		var critBonus bool
		if mod.StealBonusDamage > 0 {
			rawBonus := core.EffectiveStats(*actor).STR * mod.StealBonusDamage
			critBonus, _ = rollSkillCrit(g, actor, core.SkillSteal, quality)
			bonus, defeated = damageEnemyCrit(g, g.Battle.EnemyIndex, rawBonus, quality, core.SkillTagPhys, critBonus, false)
		}
		enqueueSkillVFXAtEnemy(g, core.SkillSteal)
		msg := stealMessage(actor.Name, kind, quality)
		cat := core.LogInfo // a plain lift is utility; the T3 cut tints it damage/death
		switch {
		case defeated:
			msg = fmt.Sprintf("%s The cut fells %s for %d.", msg, core.TheFoe(enemy), bonus)
			cat = core.LogDeath
		case bonus > 0:
			msg = fmt.Sprintf("%s The cut bleeds for %d.", msg, bonus)
			cat = core.LogDamageFoe
		}
		setBattleMessageCat(g, appendCrit(msg, critBonus), cat)
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
	actor, enemy, ok := beginEnemyTargetAction(g, core.SkillScan)
	if !ok {
		return false
	}
	g.Bestiary.MarkScanned(enemy.Kind)
	core.EnqueueEnemyVFX(g, core.VFXScan, g.Battle.EnemyIndex)
	setBattleMessage(g, fmt.Sprintf("%s scans %s — %d/%d HP. Identified.",
		actor.Name, core.TheFoe(enemy), enemy.HP, enemy.MaxHP))
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
	enqueueSkillVFXAtEnemy(g, skill)
	setBattleMessage(g, msg)
	finishActorTurn(g)
	return true
}

// simpleEnemyDebuff is the body shared by the no-damage stat debuffs (Cripple /
// Blind): stamp the registered effect onto the target's debuff mirror, then narrate
// via msgFmt ("<actor> <verb> the <foe> ... %d turns"). msgFmt takes actor name,
// foe noun, BuffTurns. Re-cast overwrites (no-stack).
func simpleEnemyDebuff(g *core.GameState, skill core.SkillID, msgFmt string) bool {
	return applyEnemyDebuffSkill(g, skill, func(actor *core.PartyMember, enemy *core.Enemy, effect core.SkillEffect) string {
		core.StampEnemyDebuff(enemy, skill, effect)
		return fmt.Sprintf(msgFmt, actor.Name, core.TheFoe(enemy), effect.BuffTurns)
	})
}

// applyAllyTargetSkill is the ally-side mirror of applyEnemyDebuffSkill: the shared
// head of the single-ally supports (Prayer / Cleanse / Renewal / Stone Skin / Aegis)
// that target g.Battle.PartyTarget. Re-checks the chosen ally (refund + end turn on a
// gone target), begins the turn, then hands off to resolve for the skill's unique
// mutation + log line. VFX (vfxKindFor) at PartyTarget and finishActorTurn are shared.
func applyAllyTargetSkill(g *core.GameState, skill core.SkillID, cat core.LogCategory, resolve func(actor, target *core.PartyMember) string) bool {
	if !ensureAlivePartyTargetOrCancel(g, skill) {
		return false
	}
	actor := beginPartyAction(g)
	target := &g.Party[g.Battle.PartyTarget]
	msg := resolve(actor, target)
	core.EnqueuePartyVFX(g, vfxKindFor(skill), g.Battle.PartyTarget)
	setBattleMessageCat(g, msg, cat)
	finishActorTurn(g)
	return true
}

// applyPartyWideBuffSkill is the party-wide-buff sibling of applyAllyTargetSkill /
// applyEnemyDebuffSkill: pay + begin the turn, stamp the tier-folded effect on every
// available member (caster included), then hand off to msg for the skill's log line.
// Always lands (no proc / no target gate); finishActorTurn is shared. Skills that also
// touch enemies (Smoke Bomb) stay bespoke.
func applyPartyWideBuffSkill(g *core.GameState, skill core.SkillID, msg func(actor *core.PartyMember, effect core.SkillEffect, count int) string) bool {
	actor := beginPartyAction(g)
	effect := core.EffectiveSkillEffect(actor, skill)
	count := stampPartyWideBuff(g, effect, skill)
	setBattleMessage(g, msg(actor, effect, count))
	finishActorTurn(g)
	return true
}

// applyCripple stamps the SPD debuff onto the target via the enemy BuffStats
// mirror (EffectiveEnemyStats folds it into the ATB rate while it runs). No
// damage / proc roll; re-cast overwrites (no-stack).
func applyCripple(g *core.GameState, quality int) bool {
	return simpleEnemyDebuff(g, core.SkillCripple, "%s cripples %s — slowed for %d turns.")
}

// applyFrostbite deals frost damage and, on a surviving target, ALWAYS chills it
// (SPD debuff, like Cripple — no proc roll, not WIS-resistible). Re-cast overwrites.
func applyFrostbite(g *core.GameState, quality int) bool {
	actor, enemy, target, rawDamage, _, effect, ok := beginSingleTargetSkill(g, core.SkillFrostbite, quality)
	if !ok {
		return false
	}
	damage, defeated, crit := strikeWithCrit(g, actor, core.SkillFrostbite, rawDamage, quality)
	enqueueSkillVFXAtEnemy(g, core.SkillFrostbite)
	// Chill only on a survivor (no debuffing a corpse); guaranteed when alive.
	chilled := !defeated && core.StampEnemyDebuff(enemy, core.SkillFrostbite, effect)
	logFoeHit(g, appendCrit(frostbiteMessage(actor.Name, target, damage, quality, defeated, chilled), crit), defeated)
	finishActorTurn(g)
	return true
}

func frostbiteMessage(name string, target core.Enemy, damage, quality int, defeated, chilled bool) string {
	return procSkillMessage(frostbiteArms, name, target, damage, quality, defeated, chilled, false)
}

// applyCorrosiveVial permanently strips the target's Armor (floored at 0) — it
// mutates live Enemy.Armor, not a turn-counted debuff, so re-casts stack down. No damage.
func applyCorrosiveVial(g *core.GameState, quality int) bool {
	// Setup committed MP; the shared head refunds it on a dead target.
	actor, enemy, ok := beginEnemyTargetAction(g, core.SkillCorrosiveVial)
	if !ok {
		return false
	}
	effect := core.EffectiveSkillEffect(actor, core.SkillCorrosiveVial)
	before := enemy.Armor
	core.SubFloorZero(&enemy.Armor, effect.ArmorReduction)
	enqueueSkillVFXAtEnemy(g, core.SkillCorrosiveVial)
	if stripped := before - enemy.Armor; stripped > 0 {
		setBattleMessage(g, fmt.Sprintf("%s's vial eats %d Armor off %s.", actor.Name, stripped, core.TheFoe(enemy)))
	} else {
		setBattleMessage(g, fmt.Sprintf("%s's vial splashes %s — no armor left to break.", actor.Name, core.TheFoe(enemy)))
	}
	finishActorTurn(g)
	return true
}

// --- Firebolt (Wizard, ramps damage and burn chance with quality) ---

func applyFirebolt(g *core.GameState, quality int) bool {
	actor, enemy, target, rawDamage, resistWIS, effect, ok := beginSingleTargetSkill(g, core.SkillFirebolt, quality)
	if !ok {
		return false
	}
	// Overcharge: a release past the peak adds flat bonus damage; the recoil it
	// costs is applied after the hit lands, below.
	overloaded := g.Battle.Timing.Overloaded
	if overloaded {
		rawDamage += core.OverchargeDamageBonus
	}
	damage, defeated, crit := strikeWithCrit(g, actor, core.SkillFirebolt, rawDamage, quality)
	enqueueSkillVFXAtEnemy(g, core.SkillFirebolt)
	burned := tryProcStatus(g.Rand(), &enemy.BurnTurns, defeated, effect.BurnChance, quality, 0, effect.BurnDuration, resistWIS)
	logFoeHit(g, appendCrit(fireboltMessage(actor.Name, target, damage, quality, defeated, burned, enemy.BurnTurns), crit), defeated)
	if overloaded {
		// Overcharge recoils on the caster. SkillTagNone so the self-burn bypasses
		// the caster's own armor/MDef.
		recoil, _ := damagePartyMember(g, g.Battle.CurrentParty, core.OverchargeRecoil, core.SkillTagNone)
		core.EnqueuePartyVFX(g, core.VFXEmber, g.Battle.CurrentParty)
		setBattleMessageCat(g, fmt.Sprintf("%s overcharges the bolt — and is scorched for %d!", actor.Name, recoil), core.LogDamageParty)
	}
	finishActorTurn(g)
	return true
}

// --- Crushing Blow (Warrior, charge phys hit with Stun proc on Great+) ---

// Crushing Blow T3 doubles damage on an Excellent timing roll (preStrike,
// INDEPENDENT of the universal crit — both stack, CritMultiplier × 2 = 4×, like
// Backstab T2), then a Great+ stun proc. vfxKindFor(CrushingBlow) is VFXSlash.
func applyCrushingBlow(g *core.GameState, quality int) bool {
	return applyProcStrike(g, core.SkillCrushingBlow, quality, stunProc(crushingBlowArms, applyTierDouble))
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
		// Count only allies HealWholeParty can actually mend (HealMember no-ops on
		// full/starving), so "mends N allies" can't overstate. Shares core's heal guard.
		if core.CanBenefitFromHeal(g.Party[i]) {
			healed++
		}
		core.EnqueuePartyVFX(g, vfxKindFor(core.SkillMassMend), i)
	}
	landed := false
	core.HealWholePartyFunc(g, heal, func(i int) {
		g.Party[i].DamageFlash = core.FlashDuration
		landed = true
	})
	if landed {
		audio.Play(audio.SoundHeal) // one cue for the party heal, matching the out-of-battle path
	}
	if healed == 0 {
		setBattleMessage(g, qualityLine(quality, actor.Name, "'s Mass Mend finds no wounds."))
	} else {
		// No "for %d each": HealWholeParty clamps each member at MaxHP, so a single
		// per-ally figure would overstate a near-full member's actual gain.
		setBattleMessageCat(g, qualityLine(quality, actor.Name, " mends %d allies.", healed), core.LogHeal)
	}
	finishActorTurn(g)
	return true
}

// --- Bless (Cleric, press party-wide stat buff) ---

// applyBless stamps the tier-folded stat buff on every available member (caster
// included). Always lands (no proc roll); re-cast replaces (no-stack).
func applyBless(g *core.GameState, quality int) bool {
	return applyPartyWideBuffSkill(g, core.SkillBless, func(actor *core.PartyMember, effect core.SkillEffect, count int) string {
		// Report the EFFECTIVE per-stat boost (all four share one magnitude), not the base.
		return qualityLine(quality, actor.Name, " blesses %d allies (+%d stats, %d turns).",
			count, effect.BuffStats.STR, effect.BuffTurns)
	})
}

// --- Smite (Cleric, press-tap magic damage) ---

// Smite T3 adds a Great+ stun proc (StunChance is 0 at T0..2, so it short-circuits).
func applySmite(g *core.GameState, quality int) bool {
	return applyProcStrike(g, core.SkillSmite, quality, stunProc(smiteArms, nil))
}

// --- Backstab (Thief, charge phys with crit on Excellent) ---

func applyBackstab(g *core.GameState, quality int) bool {
	actor, _, target, rawDamage, _, _, ok := beginSingleTargetSkill(g, core.SkillBackstab, quality)
	if !ok {
		return false
	}
	// An Excellent Backstab is a guaranteed crit (rollSkillCrit's special branch);
	// T2's `double` adds a second ×2 (T2+ Excellent = x4). Non-Excellent gets the
	// universal DEX+timing crit chance.
	crit, double := rollSkillCrit(g, actor, core.SkillBackstab, quality)
	// Crit (and T2's double) applied post-armor inside damageEnemyCrit.
	damage, defeated := damageEnemyCrit(g, g.Battle.EnemyIndex, rawDamage, quality, core.SkillTagFor(core.SkillBackstab), crit, double)
	core.EnqueueEnemyVFX(g, core.VFXSlash, g.Battle.EnemyIndex)
	logFoeHit(g, backstabMessage(actor.Name, target, damage, quality, defeated, crit), defeated)
	finishActorTurn(g)
	return true
}

// procStrike describes a "hit + single-target status proc" skill (Venom Strike,
// Rend, Lacerate, Smite, Frost Lance, Crushing Blow). Closures select the counter
// and chance/duration fields so applyProcStrike stays status-agnostic; minGrade is
// the timing floor the proc needs (0 = any); preStrike (optional) pre-mutates raw
// damage (Crushing Blow's tier double); `arms` narrates.
type procStrike struct {
	counter   func(*core.Enemy) *int
	chance    func(core.SkillEffect) float64
	dur       func(core.SkillEffect) func(*rand.Rand) int
	minGrade  int
	preStrike func(raw int, actor *core.PartyMember, skill core.SkillID, quality int) int
	arms      procMessageArms
}

var venomStrikeDoT = procStrike{
	counter: func(e *core.Enemy) *int { return &e.PoisonTurns },
	chance:  func(eff core.SkillEffect) float64 { return eff.PoisonChance },
	dur:     func(eff core.SkillEffect) func(*rand.Rand) int { return eff.PoisonDuration },
	arms:    venomStrikeArms,
}

// bleedDoT builds the Bleed descriptor for Rend / Lacerate (only arms differs).
func bleedDoT(arms procMessageArms) procStrike {
	return procStrike{
		counter: func(e *core.Enemy) *int { return &e.BleedTurns },
		chance:  func(eff core.SkillEffect) float64 { return eff.BleedChance },
		dur:     func(eff core.SkillEffect) func(*rand.Rand) int { return eff.BleedDuration },
		arms:    arms,
	}
}

// burnDoT builds the Burn descriptor for a single-target hit + Burn DoT (Searing
// Light); same shape as bleedDoT but onto Enemy.BurnTurns.
func burnDoT(arms procMessageArms) procStrike {
	return procStrike{
		counter: func(e *core.Enemy) *int { return &e.BurnTurns },
		chance:  func(eff core.SkillEffect) float64 { return eff.BurnChance },
		dur:     func(eff core.SkillEffect) func(*rand.Rand) int { return eff.BurnDuration },
		arms:    arms,
	}
}

// stunProc builds the Great+ Stun descriptor for Smite / Frost Lance / Crushing
// Blow (preStrike nil unless the skill pre-mutates raw damage); arms narrates.
func stunProc(arms procMessageArms, preStrike func(int, *core.PartyMember, core.SkillID, int) int) procStrike {
	return procStrike{
		counter:   func(e *core.Enemy) *int { return &e.StunTurns },
		chance:    func(eff core.SkillEffect) float64 { return eff.StunChance },
		dur:       func(eff core.SkillEffect) func(*rand.Rand) int { return eff.StunDuration },
		minGrade:  core.TimingQualityGreat,
		preStrike: preStrike,
		arms:      arms,
	}
}

// strikeWithCrit rolls the crit and deals the hit via damageEnemyCrit, which
// applies the crit multiplier POST-armor (never Backstab's double — that lives in
// applyBackstab). Returns post-armor damage, defeated, crit. Callers pre-mutate
// rawDamage before calling.
func strikeWithCrit(g *core.GameState, actor *core.PartyMember, skill core.SkillID, rawDamage, quality int) (damage int, defeated, crit bool) {
	crit, _ = rollSkillCrit(g, actor, skill, quality)
	// The basic attack (SkillNone) has no registered tag; it's a physical swing, so
	// fall back to Phys (armor-damped, feeds Bloodthirst). Every registered skill
	// caller carries a real tag, so this only affects the basic-attack route.
	tag := core.SkillTagFor(skill)
	if tag == core.SkillTagNone {
		tag = core.SkillTagPhys
	}
	// Crit is applied post-armor inside damageEnemyCrit.
	damage, defeated = damageEnemyCrit(g, g.Battle.EnemyIndex, rawDamage, quality, tag, crit, false)
	return damage, defeated, crit
}

// sweepCrit rolls ONE crit for an AoE sweep. The player can't react between sweep
// targets, so the roll happens once (never per-target); the multiplier itself is
// applied per-target POST-armor in damageEnemyCrit. Returns the crit verdict (for
// the per-slot multiply + the log).
func sweepCrit(g *core.GameState, actor *core.PartyMember, skill core.SkillID, quality int) bool {
	crit, _ := rollSkillCrit(g, actor, skill, quality)
	return crit
}

// applyProcStrike is the single body for every hit-plus-status-proc skill: a hit
// that, on a survivor, rolls the status (ps.chance) onto ps.counter via
// tryProcStatus at ps.minGrade. ps.preStrike (if set) pre-mutates raw damage.
func applyProcStrike(g *core.GameState, skill core.SkillID, quality int, ps procStrike) bool {
	actor, enemy, target, rawDamage, resistWIS, effect, ok := beginSingleTargetSkill(g, skill, quality)
	if !ok {
		return false
	}
	if ps.preStrike != nil {
		rawDamage = ps.preStrike(rawDamage, actor, skill, quality)
	}
	damage, defeated, crit := strikeWithCrit(g, actor, skill, rawDamage, quality)
	enqueueSkillVFXAtEnemy(g, skill)
	// Rider lands only on a hit that dealt damage — an armor-soaked 0 inflicts no
	// status (mirrors the enemy basic attack's dealt>0 gate).
	procced := damage > 0 && tryProcStatus(g.Rand(), ps.counter(enemy), defeated, ps.chance(effect), quality, ps.minGrade, ps.dur(effect), resistWIS)
	logFoeHit(g, appendCrit(procSkillMessage(ps.arms, actor.Name, target, damage, quality, defeated, procced, false), crit), defeated)
	finishActorTurn(g)
	return true
}

// --- Venom Strike (Thief, sequence phys + Poison apply) ---

func applyVenomStrike(g *core.GameState, quality int) bool {
	return applyProcStrike(g, core.SkillVenomStrike, quality, venomStrikeDoT)
}

// --- Rend (Warrior) / Lacerate (Thief): phys hit + Bleed DoT apply ---

func applyRend(g *core.GameState, quality int) bool {
	return applyProcStrike(g, core.SkillRend, quality, bleedDoT(rendArms))
}

func applyLacerate(g *core.GameState, quality int) bool {
	return applyProcStrike(g, core.SkillLacerate, quality, bleedDoT(lacerateArms))
}

// --- Searing Light (Cleric, charge magic + radiant Burn DoT) ---

func applySearingLight(g *core.GameState, quality int) bool {
	return applyProcStrike(g, core.SkillSearingLight, quality, burnDoT(searingLightArms))
}

// --- Immolate (Wizard) / Chain Lightning (Wizard): AoE casts ---

// applyImmolate is the AoE Fireball with a GUARANTEED long Burn on every target
// (BurnChance 1.0 in its def) — the shared body rolls the per-target burn.
func applyImmolate(g *core.GameState, quality int) bool {
	return applyAoEStatusSkill(g, core.SkillImmolate, "immolates", "guts out with no target", quality)
}

// applyChainLightning is AoE lightning + a per-target Stun chance (the shared body's
// StunChance roll, live only for this skill).
func applyChainLightning(g *core.GameState, quality int) bool {
	return applyAoEStatusSkill(g, core.SkillChainLightning, "leaps through", "earths out with no target", quality)
}

// --- Mug (Thief, sequence phys strike + Steal in one hit) ---

// applyMug deals a phys hit and, on a surviving foe with loot, attempts a Steal
// (timing-scaled chance). A kill yields no lift (nothing to pickpocket from a corpse).
func applyMug(g *core.GameState, quality int) bool {
	actor, live, target, rawDamage, _, effect, ok := beginSingleTargetSkill(g, core.SkillMug, quality)
	if !ok {
		return false
	}
	damage, defeated, crit := strikeWithCrit(g, actor, core.SkillMug, rawDamage, quality)
	enqueueSkillVFXAtEnemy(g, core.SkillMug)
	stole := false
	var kind core.ItemKind
	// No pickpocket off a corpse — only a surviving foe can be lifted from.
	if !defeated && live.Alive {
		kind, stole = rollSteal(g, live, effect, quality)
	}
	foe := core.EnemySingularNoun(&target)
	var msg string
	switch {
	case defeated:
		msg = qualityLine(quality, actor.Name, " mugs the %s for %d — it drops.", foe, damage)
	case stole:
		msg = fmt.Sprintf("%s%s mugs the %s for %d and lifts %s!", qualityTag(quality), actor.Name, foe, damage, core.ItemInfo(kind).Name)
	default:
		msg = qualityLine(quality, actor.Name, " mugs the %s for %d.", foe, damage)
	}
	logFoeHit(g, appendCrit(msg, crit), defeated)
	finishActorTurn(g)
	return true
}

// --- Static Field (Wizard, press %-current-HP magic) ---

// applyStaticField deals damage as a SHARE of the target's CURRENT HP (timing-scaled,
// floored at 1), dealt SkillTagNone so it bypasses Armor/MDef — a fixed fraction, not
// a normal hit. Hits hardest on a healthy foe.
func applyStaticField(g *core.GameState, quality int) bool {
	actor, live, ok := beginEnemyTargetAction(g, core.SkillStaticField)
	if !ok {
		return false
	}
	target := *live
	effect := core.EffectiveSkillEffect(actor, core.SkillStaticField)
	raw := max(core.ScaleDamage(int(float64(live.HP)*effect.PercentCurrentHP), quality), 1)
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, raw, quality, core.SkillTagNone)
	enqueueSkillVFXAtEnemy(g, core.SkillStaticField)
	foe := core.EnemySingularNoun(&target)
	msg := killOrPlainLine(quality, actor.Name, " jolts the %s for %d.", " jolts the %s for %d — it drops.", defeated, foe, damage)
	logFoeHit(g, msg, defeated)
	finishActorTurn(g)
	return true
}

// --- Guard (Warrior, press ally cover) ---

// applyGuardCover is the shared ally-cover mechanic behind Guard and Martyr's Bond:
// the ward's incoming hits redirect to the caster (core.SetGuard →
// redirectToGuardian) until the caster's next turn. No damage; always lands.
// Self-target is inert (SetGuard skips it) and reports selfMsg; a real ally target
// reports allyMsg. The two skills differ only in those log lines.
func applyGuardCover(g *core.GameState, skill core.SkillID,
	selfMsg func(actor *core.PartyMember) string,
	allyMsg func(actor, target *core.PartyMember) string) bool {
	return applyAllyTargetSkill(g, skill, core.LogInfo, func(actor, target *core.PartyMember) string {
		core.SetGuard(g.Party, g.Battle.CurrentParty, g.Battle.PartyTarget)
		if g.Battle.PartyTarget == g.Battle.CurrentParty {
			return selfMsg(actor)
		}
		return allyMsg(actor, target)
	})
}

// applyGuard has the Warrior cover the chosen ally via the shared cover mechanic.
func applyGuard(g *core.GameState, quality int) bool {
	return applyGuardCover(g, core.SkillGuard,
		func(a *core.PartyMember) string { return fmt.Sprintf("%s raises a guard — but stands alone.", a.Name) },
		func(a, t *core.PartyMember) string {
			return fmt.Sprintf("%s guards %s — incoming blows fall on them instead.", a.Name, t.Name)
		})
}

// --- Consecrate (Cleric, charge AoE radiant magic) ---

func applyConsecrate(g *core.GameState, quality int) bool {
	// Pure-damage AoE (no status fields) via the shared body.
	return applyAoEStatusSkill(g, core.SkillConsecrate, "scours", "fades with no target", quality)
}

// --- Judgment (Cleric, press execute-or-smite) ---

// applyJudgment executes a foe at/under JudgmentExecuteFraction of its HP (lethal,
// defense-bypassing), else deals heavy WIS-scaled magic. Capstone, no upgrades.
func applyJudgment(g *core.GameState, quality int) bool {
	actor, live, ok := beginEnemyTargetAction(g, core.SkillJudgment)
	if !ok {
		return false
	}
	target := *live
	executed := float64(live.HP) <= float64(max(live.MaxHP, 1))*core.JudgmentExecuteFraction
	raw := scaleSkillDamage(actor, core.SkillJudgment, quality)
	tag := core.SkillTagMagic
	if executed {
		// A true execute bypasses MDef so the kill is guaranteed.
		raw = live.HP
		tag = core.SkillTagNone
	}
	damage, defeated := damageEnemy(g, g.Battle.EnemyIndex, raw, quality, tag)
	enqueueSkillVFXAtEnemy(g, core.SkillJudgment)
	foe := core.EnemySingularNoun(&target)
	var msg string
	switch {
	case executed && defeated:
		msg = qualityLine(quality, actor.Name, " passes judgment — the %s is struck down!", foe)
	case defeated:
		msg = qualityLine(quality, actor.Name, " judges the %s for %d — it falls.", foe, damage)
	default:
		msg = qualityLine(quality, actor.Name, " judges the %s for %d.", foe, damage)
	}
	logFoeHit(g, msg, defeated)
	finishActorTurn(g)
	return true
}

// --- Reckless Swing (Warrior, charge heavy phys + self Armor debuff) ---

// applyRecklessSwing lands a heavy STR hit and sheds the swinger's own Armor for a
// turn (a negative self-buff via the buff bundle — net-negative, so no Blessed pill).
func applyRecklessSwing(g *core.GameState, quality int) bool {
	actor, _, target, rawDamage, _, effect, ok := beginSingleTargetSkill(g, core.SkillRecklessSwing, quality)
	if !ok {
		return false
	}
	damage, defeated, crit := strikeWithCrit(g, actor, core.SkillRecklessSwing, rawDamage, quality)
	enqueueSkillVFXAtEnemy(g, core.SkillRecklessSwing)
	// Self-debuff magnitudes live in the skill's registered Effect (net-negative
	// BuffArmor, so no Blessed pill); stamp them straight off the resolved effect.
	// Self-cast +1 correction (offsets finishActorTurn's immediate tick), else the
	// Armor penalty drains this same turn before any enemy acts — see selfCastTurnBonus.
	effect.BuffTurns += selfCastTurnBonus(g, g.Battle.CurrentParty)
	core.StampPartyBuff(actor, core.SkillRecklessSwing, effect)
	foe := core.EnemySingularNoun(&target)
	msg := killOrPlainLine(quality, actor.Name, " swings wildly at the %s for %d — guard down.", " caves the %s in for %d — guard down.", defeated, foe, damage)
	logFoeHit(g, appendCrit(msg, crit), defeated)
	finishActorTurn(g)
	return true
}

// --- Combust (Wizard, press Burn detonation) ---

// applyCombust adds a per-remaining-Burn-turn spike to the hit and consumes the
// target's Burn. On an unburnt foe it's just the small base magic hit.
func applyCombust(g *core.GameState, quality int) bool {
	actor, live, target, rawDamage, _, _, ok := beginSingleTargetSkill(g, core.SkillCombust, quality)
	if !ok {
		return false
	}
	burn := target.BurnTurns // pre-hit snapshot
	rawDamage += burn * core.CombustDamagePerBurnTurn
	live.BurnTurns = 0 // detonated — consume it (a kill clears it anyway)
	damage, defeated, crit := strikeWithCrit(g, actor, core.SkillCombust, rawDamage, quality)
	enqueueSkillVFXAtEnemy(g, core.SkillCombust)
	foe := core.EnemySingularNoun(&target)
	var msg string
	switch {
	case defeated:
		msg = qualityLine(quality, actor.Name, " combusts the %s for %d — it's ash.", foe, damage)
	case burn > 0:
		msg = qualityLine(quality, actor.Name, " detonates the %s's flames for %d!", foe, damage)
	default:
		msg = qualityLine(quality, actor.Name, " scorches the %s for %d.", foe, damage)
	}
	logFoeHit(g, appendCrit(msg, crit), defeated)
	finishActorTurn(g)
	return true
}

// --- Bulwark of Faith (Cleric, press party-wide Armor/MDef aura) ---

func applyBulwark(g *core.GameState, quality int) bool {
	return applyPartyWideBuffSkill(g, core.SkillBulwark, func(actor *core.PartyMember, effect core.SkillEffect, count int) string {
		return qualityLine(quality, actor.Name, " raises a bulwark of faith — %d allies gain +%d Armor / +%d MDef (%d turns).",
			count, effect.BuffArmor, effect.BuffMDef, effect.BuffTurns)
	})
}

// --- Dispel (Wizard, press enemy-buff strip) ---

func applyDispel(g *core.GameState, quality int) bool {
	return applyEnemyDebuffSkill(g, core.SkillDispel, func(actor *core.PartyMember, enemy *core.Enemy, _ core.SkillEffect) string {
		if removed := core.DispelEnemyBuffs(enemy); removed > 0 {
			return fmt.Sprintf("%s dispels %s — its magic unravels.", actor.Name, core.TheFoe(enemy))
		}
		return fmt.Sprintf("%s dispels %s — but it carries no magic to strip.", actor.Name, core.TheFoe(enemy))
	})
}

// --- Vanish (Thief, press self untargetable) ---

// applyVanish makes the caster untargetable (VanishTurns) until their next turn —
// the enemy target scans skip a vanished member, so aggro drops naturally.
func applyVanish(g *core.GameState, quality int) bool {
	actor := beginPartyAction(g)
	actor.VanishTurns = core.VanishDuration
	setBattleMessage(g, qualityLine(quality, actor.Name, " melts into shadow — untargetable."))
	finishActorTurn(g)
	return true
}

// --- Martyr's Bond (Cleric, press ally cover) ---

// applyMartyrsBond has the Cleric cover an ally via the shared Guard cover mechanic
// (the ward's hits redirect to the Cleric until the Cleric's next turn).
func applyMartyrsBond(g *core.GameState, quality int) bool {
	return applyGuardCover(g, core.SkillMartyrsBond,
		func(a *core.PartyMember) string {
			return fmt.Sprintf("%s forms a martyr's bond — but with no ally to shield.", a.Name)
		},
		func(a, t *core.PartyMember) string {
			return fmt.Sprintf("%s forms a martyr's bond with %s — their wounds become %s's own.", a.Name, t.Name, a.Name)
		})
}

// --- Resurrect (Cleric, press revive) ---

// applyResurrect revives the first downed ally to ResurrectHealPercent of MaxHP and
// pulls them back into the formation. Setup already confirmed a downed ally exists.
func applyResurrect(g *core.GameState, quality int) bool {
	idx := core.FirstDownedPartyMember(g.Party)
	if idx < 0 {
		// Defensive: the only downed ally could have been released/changed between
		// setup and apply. Refund and bail cleanly.
		return cancelGoneTarget(g, core.SkillResurrect)
	}
	actor := beginPartyAction(g)
	m := &g.Party[idx]
	m.HP = max(int(float64(m.MaxHP)*core.ResurrectHealPercent), 1)
	// Pull the revived member back to a manned slot (the anticipated Raise hook).
	core.ShuntPartyFormation(g.Party)
	core.EnqueuePartyVFX(g, vfxKindFor(core.SkillResurrect), idx)
	setBattleMessageCat(g, qualityLine(quality, actor.Name, " calls %s back from the brink (+%d HP)!", m.Name, m.HP), core.LogHeal)
	finishActorTurn(g)
	return true
}

// --- Meteor (Wizard, charge delayed AoE) ---

// applyMeteor arms a pending Meteor: snapshots the quality-scaled per-target damage
// and starts the fuse. It lands in resolveMeteorIfDue when the fuse runs out.
func applyMeteor(g *core.GameState, quality int) bool {
	actor := beginPartyAction(g)
	g.Battle.MeteorDamage = scaleSkillDamage(actor, core.SkillMeteor, quality)
	g.Battle.MeteorFuse = core.MeteorFuseTurns
	setBattleMessage(g, qualityLine(quality, actor.Name, " calls down a meteor — the sky darkens overhead!"))
	finishActorTurn(g)
	return true
}

// --- Ancestral Spirit (Warrior, press summon) ---

// applyAncestralSpirit conjures a shade that strikes alongside the summoner each of
// their turns (tickAncestralSpirit) for AncestralSpiritTurns, starting this turn.
func applyAncestralSpirit(g *core.GameState, quality int) bool {
	actor := beginPartyAction(g)
	actor.SpiritTurns = core.AncestralSpiritTurns
	core.EnqueuePartyVFX(g, vfxKindFor(core.SkillAncestralSpirit), g.Battle.CurrentParty)
	setBattleMessage(g, qualityLine(quality, actor.Name, " summons an ancestral shade to fight at their side!"))
	finishActorTurn(g)
	return true
}

// --- Frost Lance (Wizard, charge magic with reliable Stun on Great+) ---

// FrostLance is "freeze" flavor but uses the canonical StunTurns counter (no
// separate frozen status); the log keeps "Frozen!" via frostLanceArms.
func applyFrostLance(g *core.GameState, quality int) bool {
	return applyProcStrike(g, core.SkillFrostLance, quality, stunProc(frostLanceArms, nil))
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
// applyAoEDamageToSlots remains for the multi-PASS Swipe.
func applyAoEStatusSkill(g *core.GameState, skill core.SkillID, hitVerb, emptyVerb string, quality int) bool {
	skillNoun := core.SkillName(skill)
	actor := beginPartyAction(g)
	effect := core.EffectiveSkillEffect(actor, skill)
	damage := scaleSkillDamage(actor, skill, quality)
	crit := sweepCrit(g, actor, skill, quality)
	tag := core.SkillTagFor(skill)
	vfx := vfxKindFor(skill)
	hits := 0
	afflicted := 0
	totalDealt := 0
	forEachTargetableEnemy(g, skill, func(slot int, enemy *core.Enemy) {
		dealt, defeated := damageEnemyCrit(g, slot, damage, quality, tag, crit, false)
		totalDealt += dealt
		core.EnqueueEnemyVFX(g, vfx, slot)
		hits++
		resistWIS := core.EffectiveEnemyStats(enemy).WIS
		// Count each foe afflicted at most once so the tally can't exceed foe count.
		struck := false
		// Status riders land only on a hit that dealt damage — an armor/MDef-soaked 0
		// inflicts no Burn/Poison/Stun (mirrors the enemy basic attack's dealt>0 gate).
		if dealt > 0 {
			if tryProcStatus(g.Rand(), &enemy.BurnTurns, defeated, effect.BurnChance, quality, 0, effect.BurnDuration, resistWIS) {
				struck = true
			}
			if tryProcStatus(g.Rand(), &enemy.PoisonTurns, defeated, effect.PoisonChance, quality, 0, effect.PoisonDuration, resistWIS) {
				struck = true
			}
			// Per-target Stun (Chain Lightning's shock) — any grade, low chance. Every
			// other AoE skill carries StunChance 0, so this short-circuits for them.
			if tryProcStatus(g.Rand(), &enemy.StunTurns, defeated, effect.StunChance, quality, 0, effect.StunDuration, resistWIS) {
				struck = true
			}
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
	setBattleMessageCat(g, msg, core.LogDamageFoe)
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
	actor, _, target, rawDamage, _, effect, ok := beginSingleTargetSkill(g, core.SkillSunder, quality)
	if !ok {
		return false
	}
	damage, defeated, crit := strikeWithCrit(g, actor, core.SkillSunder, rawDamage, quality)
	enqueueSkillVFXAtEnemy(g, core.SkillSunder)
	shoved := !defeated && pushEnemyReadiness(g, g.Battle.EnemyIndex, effect.ATBPush)
	msg := killOrPlainLine(quality, actor.Name, " sunders %s for %d.", " sunders %s for %d — it falls.", defeated, core.TheFoe(&target), damage)
	if shoved { // shoved implies !defeated, so it can't collide with the kill line
		msg = fmt.Sprintf("%s Its turn is shoved back.", msg)
	}
	logFoeHit(g, appendCrit(msg, crit), defeated)
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
		return fmt.Sprintf("%s taunts %s — it turns its glare on them.",
			actor.Name, core.TheFoe(enemy))
	})
}

// --- War Banner (Warrior, press party-wide STR/VIT rally) ---

func applyWarBanner(g *core.GameState, quality int) bool {
	return applyPartyWideBuffSkill(g, core.SkillWarBanner, func(actor *core.PartyMember, effect core.SkillEffect, count int) string {
		return qualityLine(quality, actor.Name, " plants a war banner — %d allies rally (+%d STR, +%d Armor, %d turns).",
			count, effect.BuffStats.STR, effect.BuffArmor, effect.BuffTurns)
	})
}

// --- Stone Skin (Warrior, press single-ally Armor/MDef ward) ---

func applyStoneSkin(g *core.GameState, quality int) bool {
	return applyAllyTargetSkill(g, core.SkillStoneSkin, core.LogInfo, func(actor, target *core.PartyMember) string {
		eff := core.EffectiveSkillEffect(actor, core.SkillStoneSkin)
		// Self-cast +1 correction (offsets finishActorTurn's immediate tick); see selfCastTurnBonus.
		eff.BuffTurns += selfCastTurnBonus(g, g.Battle.PartyTarget)
		core.StampPartyBuff(target, core.SkillStoneSkin, eff)
		// Report the STAMPED duration (carries the self-cast +1), not the base.
		return qualityLine(quality, actor.Name, " wards %s in stone (+%d Armor, +%d MDef, %d turns).",
			target.Name, eff.BuffArmor, eff.BuffMDef, eff.BuffTurns)
	})
}

// --- Blind (Cleric, press enemy accuracy debuff) ---

// applyBlind saps the target's DEX (which EnemyHitChance reads) so it whiffs more
// — the DEX sibling of Cripple. No damage; always lands; re-cast overwrites.
func applyBlind(g *core.GameState, quality int) bool {
	return simpleEnemyDebuff(g, core.SkillBlind, "%s blinds %s — its aim falters for %d turns.")
}

// --- Aegis (Cleric, press single-ally absorb shield) ---

// applyAegis grants the ally a ShieldHP pool the damage path spends before HP.
// Not turn-counted; re-cast replaces the pool.
func applyAegis(g *core.GameState, quality int) bool {
	return applyAllyTargetSkill(g, core.SkillAegis, core.LogInfo, func(actor, target *core.PartyMember) string {
		effect := core.EffectiveSkillEffect(actor, core.SkillAegis)
		target.ShieldHP = effect.ShieldHP
		return qualityLine(quality, actor.Name, " raises an aegis over %s — absorbs the next %d damage.",
			target.Name, target.ShieldHP)
	})
}

// --- Smoke Bomb (Thief, press party evasion + enemy accuracy loss) ---

// applySmokeBomb buffs the party's DEX (evasion) and saps every enemy's DEX
// (accuracy) by the SAME magnitude. Both overwrite (no-stack).
func applySmokeBomb(g *core.GameState, quality int) bool {
	actor := beginPartyAction(g)
	effect := core.EffectiveSkillEffect(actor, core.SkillSmokeBomb)
	buffed := stampPartyWideBuff(g, effect, core.SkillSmokeBomb)
	enemyDebuff := core.NegStatDebuff(core.Stats{DEX: effect.BuffStats.DEX}, effect.BuffTurns)
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
	setBattleMessage(g, qualityLine(quality, actor.Name, " drops a smoke bomb — %d allies gain evasion, %d foes lose their aim.",
		buffed, blinded))
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
	setBattleMessage(g, qualityLine(quality, actor.Name, " sheathes in ice — +%d MDef, attackers chilled for %d turns.",
		core.IceArmorMDef, turns))
	finishActorTurn(g)
	return true
}

// selfCastTurnBonus returns the +1 duration correction for a buff stamped on the
// CURRENTLY-ACTING member: finishActorTurn ticks their buff down before they act
// again, so +1 nets the full duration. Returns 0 for any other ally. The one place
// this rule lives (Bless / War Banner / Smoke Bomb / Stone Skin / Ice Armor).
func selfCastTurnBonus(g *core.GameState, targetIdx int) int {
	if targetIdx == g.Battle.CurrentParty {
		return core.StatusTurnStep
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

// --- Poison Cloud (Thief, sequence AoE toxin + per-target Poison) ---

func applyPoisonCloud(g *core.GameState, quality int) bool {
	return applyAoEStatusSkill(g, core.SkillPoisonCloud, "blankets", "disperses with no target", quality)
}

// --- Cleanse (Cleric, press single-ally status cure) ---

func applyCleanse(g *core.GameState, quality int) bool {
	return applyAllyTargetSkill(g, core.SkillCleanse, core.LogInfo, func(actor, target *core.PartyMember) string {
		cured := core.CureDebuffs(target)
		if cured == 0 {
			return qualityLine(quality, actor.Name, " cleanses %s — nothing ailed them.", target.Name)
		}
		return qualityLine(quality, actor.Name, " cleanses %s — %d cured.", target.Name, cured)
	})
}

// --- Second Wind (Warrior, charge flat self-heal) ---

func applySecondWind(g *core.GameState, quality int) bool {
	actor := beginPartyAction(g)
	// Utility-kind: flat Effect.Heal (no WIS), timing-scaled by ScaleHeal.
	heal := core.ScaleHeal(core.SkillHealFor(actor, core.SkillSecondWind), quality)
	// Log the LANDED amount, not the intent — HealMember clamps at MaxHP (the
	// Prayer convention: logged figures match the HP delta).
	before := actor.HP
	if healPartyMember(g, g.Battle.CurrentParty, heal) {
		core.EnqueuePartyVFX(g, vfxKindFor(core.SkillSecondWind), g.Battle.CurrentParty)
		setBattleMessageCat(g, qualityLine(quality, actor.Name, " catches a second wind — recovers %d HP.", actor.HP-before), core.LogHeal)
	} else {
		setBattleMessage(g, healFailedLine(actor,
			qualityLine(quality, actor.Name, " is too starved to catch a second wind."),
			qualityLine(quality, actor.Name, " is already at full health.")))
	}
	finishActorTurn(g)
	return true
}

// --- Renewal (Cleric, charge heal-over-time on one ally) ---

func applyRenewal(g *core.GameState, quality int) bool {
	// Renewal arms on a charge bar (the ally can die mid-charge) and bypasses
	// healPartyMember's guard by stamping the regen counter directly, so the shared
	// head re-checks the target.
	return applyAllyTargetSkill(g, core.SkillRenewal, core.LogHeal, func(actor, target *core.PartyMember) string {
		effect := core.EffectiveSkillEffect(actor, core.SkillRenewal)
		// Snapshot the per-turn heal at cast (WIS + timing), floored at 1. Re-cast replaces.
		perTurn := max(core.ScaleHeal(core.SkillHealFor(actor, core.SkillRenewal), quality), 1)
		target.RegenPerTurn = perTurn
		target.RegenTurns = effect.RegenTurns
		return qualityLine(quality, actor.Name, " lays a renewal on %s — +%d HP at the end of their next %d turns.",
			target.Name, perTurn, effect.RegenTurns)
	})
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
		double = excellentTierDoubles(actor, core.SkillBackstab, quality)
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

// excellentTierDoubles reports whether skill's "Excellent doubles damage" tier mod
// (CritDoubleOnExcellent) is live for this timing roll. Single source for the rule;
// Crushing Blow applies it as a flat pre-crit ×2 (applyTierDouble), Backstab folds it
// into rollSkillCrit's guaranteed-crit double channel (the two apply points differ,
// the trigger does not).
func excellentTierDoubles(actor *core.PartyMember, skill core.SkillID, quality int) bool {
	return quality == core.TimingQualityExcellent && core.SkillTierMod(actor, skill).CritDoubleOnExcellent
}

// applyTierDouble applies the "Excellent doubles damage" tier mod as a flat pre-crit
// ×2 — Crushing Blow's path. Named to pair with applyCritMultiplier; the trigger is
// the shared excellentTierDoubles. (Backstab uses rollSkillCrit's double channel
// instead, to stack atop the crit multiplier for the 4× case.)
func applyTierDouble(raw int, actor *core.PartyMember, skill core.SkillID, quality int) int {
	if excellentTierDoubles(actor, skill, quality) {
		return raw * core.TierDamageDoubler
	}
	return raw
}

// applyCritMultiplier returns the post-crit damage, capped at core.MaxHitDamage —
// the sanity ceiling vs extreme authored stats shared by both damage paths.
// `double` (Backstab T2) multiplies again on top of CritMultiplier.
func applyCritMultiplier(raw int, crit, double bool) int {
	if !crit {
		return min(raw, core.MaxHitDamage)
	}
	out := raw * core.CritMultiplier
	if double {
		out *= core.TierDamageDoubler
	}
	return min(out, core.MaxHitDamage)
}

// targetActsLater reports whether the enemy at enemySlot has NOT acted yet this
// round (occupies no queue slot before the cursor). Shadow Step gates its bonus on
// this so it rewards getting the jump, not re-hitting an already-acted foe.
func targetActsLater(g *core.GameState, enemySlot int) bool {
	return !actorAppearsBefore(g.Battle.Queue, g.Battle.QueueCursor, core.ActorRef{IsParty: false, Index: enemySlot})
}

// scaleByRank returns base scaled by rank and a per-rank fraction (rank 0 = 0).
// The shared rank-multiplier math behind Shadow Step / Bloodthirst / Retribution.
func scaleByRank(base, rank int, perRank float64) int {
	return int(float64(base) * float64(rank) * perRank)
}

// scaleFloorOne scales base by a float multiplier, floored at 1 so a derived hit
// always lands for at least 1. One home for the floor rule (Ancestral Spirit /
// Riposte off-turn strikes) so a future floor change is a single edit.
func scaleFloorOne(base int, mult float64) int {
	return max(int(float64(base)*mult), 1)
}

// applyShadowStep folds the Thief's Shadow Step into an outgoing single-target hit:
// +ShadowStepBonusPerRank/rank when striking before the target acts (rank 0 = no-op).
// Applied pre-crit so the bonus rides the crit multiplier.
func applyShadowStep(g *core.GameState, actor *core.PartyMember, raw int) int {
	rank := core.PassiveRank(actor, core.PassiveShadowStep)
	if rank <= 0 || raw <= 0 || !targetActsLater(g, g.Battle.EnemyIndex) {
		return raw
	}
	return raw + scaleByRank(raw, rank, core.ShadowStepBonusPerRank)
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

// damageEnemyUntallied is damageEnemy with the per-turn tallies (Bloodthirst
// lifesteal PhysDamageThisTurn + Killing Spree EnemyKillsThisTurn) snapshotted and
// restored, so an OFF-TURN strike (ancestral shade, riposte/retribution counters)
// never feeds the acting member's own-turn bonuses. Callers that should still
// lifesteal feed the returned amount to bloodthirstHeal explicitly.
func damageEnemyUntallied(g *core.GameState, slot, rawDamage, quality int, tag core.SkillTag) (int, bool) {
	physTally, killTally := g.Battle.PhysDamageThisTurn, g.Battle.EnemyKillsThisTurn
	dealt, defeated := damageEnemy(g, slot, rawDamage, quality, tag)
	g.Battle.PhysDamageThisTurn, g.Battle.EnemyKillsThisTurn = physTally, killTally
	return dealt, defeated
}

// damageEnemy applies a non-crit rawDamage to the enemy at slot. Thin wrapper over
// damageEnemyCrit for the many callers that never crit (DoTs, AoE status passes,
// off-turn counters, tests).
func damageEnemy(g *core.GameState, slot, rawDamage, quality int, tag core.SkillTag) (int, bool) {
	return damageEnemyCrit(g, slot, rawDamage, quality, tag, false, false)
}

// damageEnemyCrit applies rawDamage to the enemy at slot (mitigated per tag), then
// multiplies the POST-armor result on a crit (so crit punches through armor — the
// anti-tank lever), drives the popup color by quality + the "Critical!" prefix, and
// returns (final damage, defeated) — callers log the figure so it matches the HP
// delta. `double` is Backstab T2's extra ×2. quality may be Miss for non-action
// damage. Any damage > 0 wakes a sleeping enemy.
func damageEnemyCrit(g *core.GameState, slot, rawDamage, quality int, tag core.SkillTag, crit, double bool) (int, bool) {
	enemy, ok := livingEnemyAt(g, slot)
	if !ok {
		return 0, false
	}
	// Effective Armor + MDef with active debuffs folded in (enemy mirror of
	// EffectiveDefenses), so an armor/MDef debuff actually softens mitigation.
	effArmor, effMDef := core.EffectiveEnemyDefenses(enemy)
	damage := mitigateDamage(rawDamage, tag, effArmor, effMDef)
	// Crit multiplies the MITIGATED damage (post-armor), so a crit reliably beats a
	// tank's armor floor instead of being swallowed by it.
	damage = applyCritMultiplier(damage, crit, double)

	// Tally phys output this turn for Bloodthirst (finishActorTurn banks it as
	// lifesteal). Off-turn counters (tryRiposte) snapshot/restore around this so
	// their damage stays out of the tally. Non-phys tags don't feed it.
	if tag == core.SkillTagPhys && damage > 0 {
		g.Battle.PhysDamageThisTurn += damage
	}
	// Flash + HP-floor + popup + recoil (shared tail with the party path; popup/recoil
	// only fire on real damage). Death-side handling stays below.
	died := core.ApplyDamageWithPopup(enemy.HitTarget(), damage, quality, crit)
	// Every landing blow buzzes a little; grade/crit shake rumble stacks on top.
	if damage > 0 {
		core.AddRumble(&g.Battle, core.RumbleImpact, core.RumbleImpactDur)
	}
	if !died {
		// Audible thud only on scoring hits.
		if damage > 0 {
			audio.Play(audio.SoundEnemyHit)
		}
		return damage, false
	}
	// Snapshot Plague's trigger BEFORE the death clear wipes the dying foe's poison.
	wasPoisoned := enemy.PoisonTurns > 0
	g.Battle.EnemyKillsThisTurn++ // Killing Spree reads this at end-of-turn
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
	// Plague (Thief): a poisoned foe's death leaps its poison to the rest of the pack.
	if wasPoisoned && core.AnyPartyHasPassive(g.Party, core.PassivePlague) {
		spreadPlague(g, slot)
	}
	// Repack the front row so a back-row enemy slides up to fill the gap.
	core.ShuntEnemyFormation(core.BattleMembers(g))
	return damage, true
}

// spreadPlague stamps Poison on every OTHER living enemy when a poisoned foe dies
// (Thief's Plague capstone). No-stack: only un-poisoned foes catch it; a fixed
// rolled duration. deadSlot is the foe that just died (skipped — already a corpse).
func spreadPlague(g *core.GameState, deadSlot int) {
	dur := core.PlagueSpreadMinTurns
	if span := core.PlagueSpreadMaxTurns - core.PlagueSpreadMinTurns; span > 0 {
		dur += g.Rand().Intn(span + 1)
	}
	spread := 0
	forEachLivingEnemy(g, func(slot int, e *core.Enemy) {
		if slot == deadSlot || e.PoisonTurns > 0 {
			return
		}
		e.PoisonTurns = dur
		core.EnqueueEnemyVFX(g, core.VFXVenom, slot)
		spread++
	})
	if spread > 0 {
		setBattleMessageCat(g, "The plague spreads through the pack!", core.LogDamageFoe)
	}
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
	setBattleMessageCat(g, poisonTickMessage(member.Name, dealt, killed), core.LogDamageParty)
	return killed
}

// applyEnemyDoTTick drains the counter and deals tickDamage as a Magic-tagged
// (armor-bypassing), Good-quality (orange popup) hit. Returns (dealt, defeated);
// the caller owns the up-front guard and formats its own line.
func applyEnemyDoTTick(g *core.GameState, index int, counter *int, tickDamage int) (int, bool) {
	*counter--
	return damageEnemy(g, index, tickDamage, autoStrikeQuality, core.SkillTagMagic)
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
	if m.HP <= 0 {
		return
	}
	m.Buffs = tickStatusModList(g, m.Buffs, m.Name, "%s's %s fades.")
}

// tickEnemyBuffAfterTurn drains every stackable debuff on the enemy at its turn
// end — the enemy mirror of tickBlessAfterPartyTurn. No-ops on party actors / dead enemies.
func tickEnemyBuffAfterTurn(g *core.GameState, actor core.ActorRef) {
	if actor.IsParty {
		return
	}
	enemy, ok := livingEnemyAt(g, actor.Index)
	if !ok {
		return
	}
	enemy.Debuffs = tickStatusModList(g, enemy.Debuffs, core.EnemyDisplayName(enemy), "%s's %s wears off.")
}

// tickStatusModList drains a stackable buff/debuff list one turn, narrating each
// expired source (skill) via fadeFmt(name, skill) and returning the survivors. Shared
// body for tickBlessAfterPartyTurn (party buffs) and tickEnemyBuffAfterTurn (enemy debuffs).
func tickStatusModList(g *core.GameState, mods []core.StatusMod, name, fadeFmt string) []core.StatusMod {
	if len(mods) == 0 {
		return mods
	}
	remaining, expired := core.TickStatusMods(mods)
	for _, s := range expired {
		setBattleMessage(g, fmt.Sprintf(fadeFmt, name, core.SkillName(s)))
	}
	return remaining
}

// tickRegenAfterPartyTurn applies one Renewal HoT tick at the member's turn end —
// the positive mirror of tickPoisonAfterPartyTurn. Routes the heal through
// healPartyMember (clamps, flashes, no-ops dead/ingested). Party-only.
func tickRegenAfterPartyTurn(g *core.GameState, actor core.ActorRef) {
	m, fades, ok := drainPartyCounter(g, actor, func(m *core.PartyMember) *int { return &m.RegenTurns })
	if !ok {
		return
	}
	healed := healPartyMember(g, actor.Index, m.RegenPerTurn)
	if healed {
		core.EnqueuePartyVFX(g, core.VFXHeal, actor.Index)
	}
	// Report HP only when a heal landed (a full-HP tick heals nothing); always
	// announce the fade on the final tick.
	switch {
	case healed && fades:
		setBattleMessageCat(g, fmt.Sprintf("%s renews %d HP — the renewal fades.", m.Name, m.RegenPerTurn), core.LogHeal)
	case healed:
		setBattleMessageCat(g, fmt.Sprintf("%s renews %d HP.", m.Name, m.RegenPerTurn), core.LogHeal)
	case fades:
		setBattleMessage(g, fmt.Sprintf("%s's renewal fades.", m.Name))
	}
}

// drainPartyCounter is the shared head of the end-of-turn party counter ticks
// (Webbed / Confused / Ice Armor / Renewal): validates the actor, guards on
// HP/positive counter, decrements one turn, and returns the member plus whether
// this tick just zeroed it. ok=false means no tick happened (skip the side effect).
func drainPartyCounter(g *core.GameState, actor core.ActorRef, counterRef func(*core.PartyMember) *int) (m *core.PartyMember, cleared, ok bool) {
	if !actor.ValidPartyIndex(g.Party) {
		return nil, false, false
	}
	m = &g.Party[actor.Index]
	if m.HP <= 0 {
		return nil, false, false
	}
	c := counterRef(m)
	if *c <= 0 {
		return nil, false, false
	}
	*c--
	return m, *c == 0, true
}

// tickPartyStatusCounter is the shared body for the non-damaging end-of-turn ticks
// (Webbed, Confused): drains the counterRef counter and, on reaching zero, emits
// clearedFmt ("%s" + name; "" for a silent clear). Damaging / healing ticks reuse
// drainPartyCounter directly for their own side effect.
func tickPartyStatusCounter(g *core.GameState, actor core.ActorRef, counterRef func(*core.PartyMember) *int, clearedFmt string) {
	m, cleared, ok := drainPartyCounter(g, actor, counterRef)
	if !ok {
		return
	}
	if cleared && clearedFmt != "" {
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
	logFoeHit(g, msg(core.EnemyDisplayName(enemy), dealt, defeated), defeated)
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
		setBattleMessageCat(g, fmt.Sprintf("%s succumbs to the flames.", core.TheEnemy(def)), core.LogDeath)
		// Repoint the cursor if the burn killed the selected enemy.
		repointEnemyCursorIfDead(g)
		return true
	}
	setBattleMessageCat(g, fmt.Sprintf("%s burns for %d.", core.TheEnemy(def), dealt), core.LogDamageFoe)
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

// damagePartyMember applies a non-crit rawAmount to a member. Thin wrapper over
// damagePartyMemberCrit for the many non-crit callers (DoTs, spells, recoil, tests).
func damagePartyMember(g *core.GameState, partyIndex, rawAmount int, tag core.SkillTag) (int, bool) {
	return damagePartyMemberCrit(g, partyIndex, rawAmount, tag, false)
}

// damagePartyMemberCrit applies rawAmount to a member (mitigated per tag, crit
// multiplied post-armor); damage > 0 wakes from Sleep. Returns (dealt, fatal) —
// callers log the dealt figure so it matches the HP delta. Out-of-range or
// amount<=0 returns (0, false).
func damagePartyMemberCrit(g *core.GameState, partyIndex, rawAmount int, tag core.SkillTag, crit bool) (int, bool) {
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
	return applyPartyDamage(g, member, rawAmount, tag, crit)
}

// applyPartyDamage mitigates + applies rawAmount to an ALREADY-validated living
// member (caller owns the bounds/dead/lockout checks), running the shared
// flash/recoil/wake/rumble/death bookkeeping. Split out so the poison DoT can
// reach it without the ingest lockout. crit multiplies the POST-armor amount (and
// prefixes the popup) — only the enemy basic attack passes true today.
func applyPartyDamage(g *core.GameState, member *core.PartyMember, rawAmount int, tag core.SkillTag, crit bool) (int, bool) {
	// EffectiveDefenses folds equipped gear onto the base values.
	armorVal, mdefVal := core.EffectiveDefenses(*member)
	amount := mitigateDamage(rawAmount, tag, armorVal, mdefVal)
	// Crit punches post-armor (mirror of the enemy path) before the shield soaks it.
	amount = applyCritMultiplier(amount, crit, false)

	// Aegis shield soaks post-mitigation BEFORE HP; only overflow reaches HP, and
	// the bookkeeping below reads the post-shield amount.
	if amount > 0 && member.ShieldHP > 0 {
		absorbed := min(amount, member.ShieldHP)
		core.SubFloorZero(&member.ShieldHP, absorbed)
		amount -= absorbed
	}
	// Last Stand (Warrior): the first otherwise-lethal blow each battle leaves the
	// member at 1 HP instead of downing them. Post-shield, so a fully-soaked hit
	// doesn't burn the charge.
	if amount >= member.HP && !member.LastStandUsed && core.PassiveRank(member, core.PassiveLastStand) > 0 {
		amount = member.HP - 1
		member.LastStandUsed = true
		setBattleMessage(g, fmt.Sprintf("%s refuses to fall — Last Stand!", member.Name))
	}
	// Flash + HP-floor + popup + recoil (shared tail with the enemy path). Incoming hits
	// aren't player-timed, so quality is Miss (the draw colors party popups a fixed hurt tone).
	died := core.ApplyDamageWithPopup(member.HitTarget(), amount, int(core.TimingQualityMiss), crit)
	// Haptic buzz on a landing hit. Taking a hit doesn't shake the camera, so this
	// arms rumble directly (AddCombatShake is for offensive impacts). Additive so
	// back-to-back hits in one beat stack.
	if amount > 0 {
		core.AddRumble(&g.Battle, core.RumbleHurtStrength, core.RumbleHurtDur)
	}
	if !died {
		return amount, false
	}
	clearPartyStatusesOnDeath(member)
	// Release any Guard cover the downed member anchored: dropping the ward's
	// GuardedBy link here stops a later Resurrect from silently resuming the tank
	// (redirectToGuardian only masks it while the guardian is dead).
	for i := range g.Party {
		if &g.Party[i] == member {
			core.ClearGuardBy(g.Party, i)
			break
		}
	}
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
	return applyPartyDamage(g, member, core.PoisonTickDamage, core.SkillTagMagic, false)
}

// damagePartyMemberDefendable applies a non-crit incoming HIT honoring the Defend
// brace. Thin wrapper; the enemy basic attack uses the Crit variant.
func damagePartyMemberDefendable(g *core.GameState, partyIndex, rawAmount int, tag core.SkillTag) (int, bool) {
	return damagePartyMemberDefendableCrit(g, partyIndex, rawAmount, tag, false)
}

// damagePartyMemberDefendableCrit applies an incoming HIT honoring the Defend brace
// BEFORE mitigation (for enemy melee + damaging casts). DoT ticks call
// damagePartyMember directly so the brace doesn't soak them. A positive soak floors
// at 1. crit multiplies post-armor (enemy basic-attack crit).
func damagePartyMemberDefendableCrit(g *core.GameState, partyIndex, rawAmount int, tag core.SkillTag, crit bool) (int, bool) {
	if rawAmount > 0 && partyIndexValid(g, partyIndex) && g.Party[partyIndex].Defending {
		// Floor a positive soak at 1 (a soak, not free immunity).
		rawAmount = max(int(float32(rawAmount)*core.DefendingDamageMult), 1)
	}
	return damagePartyMemberCrit(g, partyIndex, rawAmount, tag, crit)
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
	{"VanishTurns", func(m *core.PartyMember) *int { return &m.VanishTurns }, true},
	{"SpiritTurns", func(m *core.PartyMember) *int { return &m.SpiritTurns }, true},
}

// init asserts the death-clear descriptors are COMPLETE: every `int` field ending
// in "Turns" must be classified, else it panics at startup (can't linger on a corpse).
func init() {
	assertTurnsCountersClassified := func(structName string, sample any, classified map[string]bool) {
		t := reflect.TypeOf(sample)
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Type.Kind() != reflect.Int || !strings.HasSuffix(f.Name, "Turns") {
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
	// Non-counter statuses the *Turns sweep can't reach (mirrors the enemy path's
	// Debuffs clear): a corpse keeps neither its buff list nor a shield, else a
	// mid-battle Resurrect revives with both live again. Same for its own Guard
	// cover state (the ward-side link is dropped by the caller via ClearGuardBy).
	member.Buffs = nil
	member.ShieldHP = 0
	member.Guarding = false
	member.Guarded = false
	member.GuardedBy = 0
	// Release the ingest link on death: poison ticks CAN kill an ingested member
	// (tickPoisonForIngestedParty bypasses the ingest lockout), and a corpse left
	// Ingested still satisfies MantrapHasPrey (the mantrap can never eat again) and,
	// worse, a mid-battle Resurrect would revive an alive-but-Ingested member —
	// queue-skipped, untargetable, unhealable. Mirrors ReleaseIngestedBy's sentinel.
	member.Ingested = false
	member.IngestedBy = -1
}

// --- Result text ---

func attackResultMessage(name string, target core.Enemy, damage, quality int, defeated bool) string {
	tag := qualityTag(quality)
	if defeated {
		return fmt.Sprintf("%s%s drops a %s for %d.", tag, name, core.EnemySingularNoun(&target), damage)
	}
	return fmt.Sprintf("%s%s hits for %d.", tag, name, damage)
}

func swipeMessage(name string, hits, quality int) string {
	return qualityLine(quality, name, " swipes through %d foes.", hits)
}

func prayerMessage(name, targetName string, heal, quality int, self bool) string {
	if self {
		return qualityLine(quality, name, " prays for themselves (+%d HP).", heal)
	}
	return qualityLine(quality, name, " prays over %s (+%d HP).", targetName, heal)
}

func stealMessage(name string, kind core.ItemKind, quality int) string {
	tag := qualityTag(quality)
	return fmt.Sprintf("%s%s steals %s.", tag, name, core.ItemInfo(kind).Name)
}

var fireboltArms = procMessageArms{
	defeated:      "%[1]s%[2]s's Firebolt drops the %[3]s for %[4]d.",
	proc:          "%[1]s%[2]s scorches the %[3]s for %[4]d. Burning!",
	alreadyActive: "%[1]s%[2]s hits for %[4]d. Burn is already active.",
	plain:         "%[1]s%[2]s hits for %[4]d.",
}

func fireboltMessage(name string, target core.Enemy, damage, quality int, defeated, burned bool, burnTurns int) string {
	return procSkillMessage(fireboltArms, name, target, damage, quality, defeated, burned, burnTurns > 0)
}

// procMessageArms holds the log variants (defeated / proc / alreadyActive / plain).
// Format strings use explicit arg indices (tag=%[1]s, name=%[2]s, noun=%[3]s,
// damage=%[4]d) so an arm can omit verbs it doesn't need. alreadyActive is the
// optional 4th arm for "status was already running" (Firebolt's burn); skills
// without it leave it "" and never pass alreadyActive=true.
type procMessageArms struct{ defeated, proc, alreadyActive, plain string }

// procSkillMessage selects and formats the right arm — the shared helper for every
// single-target proc skill. Precedence: defeated > proc > alreadyActive > plain.
func procSkillMessage(arms procMessageArms, name string, target core.Enemy, damage, quality int, defeated, proc, alreadyActive bool) string {
	f := arms.plain
	switch {
	case defeated:
		f = arms.defeated
	case proc:
		f = arms.proc
	case alreadyActive && arms.alreadyActive != "":
		f = arms.alreadyActive
	}
	if f == "" {
		// A skill may intentionally omit the plain arm (Frostbite — its chill always
		// procs on a survivor). Fall back to a non-empty arm rather than log a blank line.
		if arms.proc != "" {
			f = arms.proc
		} else {
			f = arms.defeated
		}
	}
	return fmt.Sprintf(f, qualityTag(quality), name, core.EnemySingularNoun(&target), damage)
}

var (
	crushingBlowArms = procMessageArms{
		defeated: "%[1]s%[2]s shatters the %[3]s with a Crushing Blow for %[4]d.",
		proc:     "%[1]s%[2]s crushes the %[3]s for %[4]d. Stunned!",
		plain:    "%[1]s%[2]s Crushing Blows for %[4]d.",
	}
	smiteArms = procMessageArms{
		defeated: "%[1]s%[2]s smites the %[3]s down for %[4]d.",
		proc:     "%[1]s%[2]s smites for %[4]d. Stunned!",
		plain:    "%[1]s%[2]s smites for %[4]d.",
	}
	backstabArms = procMessageArms{
		defeated: "%[1]s%[2]s's Backstab fells the %[3]s for %[4]d.",
		proc:     "%[1]s%[2]s lands a clean Backstab for %[4]d!",
		plain:    "%[1]s%[2]s stabs for %[4]d.",
	}
	venomStrikeArms = procMessageArms{
		defeated: "%[1]s%[2]s's Venom Strike fells the %[3]s for %[4]d.",
		proc:     "%[1]s%[2]s envenoms the %[3]s for %[4]d. Poisoned!",
		plain:    "%[1]s%[2]s stings for %[4]d.",
	}
	frostLanceArms = procMessageArms{
		defeated: "%[1]s%[2]s's Frost Lance shatters the %[3]s for %[4]d.",
		proc:     "%[1]s%[2]s freezes the %[3]s for %[4]d. Frozen!",
		plain:    "%[1]s%[2]s lances for %[4]d.",
	}
	frostbiteArms = procMessageArms{
		defeated: "%[1]s%[2]s's Frostbite freezes the %[3]s solid for %[4]d.",
		proc:     "%[1]s%[2]s bites the %[3]s for %[4]d and chills it.",
		// No plain arm: Frostbite's chill is a guaranteed debuff (no chance roll), so a
		// survivor always takes the proc arm and the defeated arm covers the kill. An
		// empty plain would only surface if that invariant broke — left zero deliberately.
	}
	rendArms = procMessageArms{
		defeated: "%[1]s%[2]s's Rend tears the %[3]s apart for %[4]d.",
		proc:     "%[1]s%[2]s rends the %[3]s for %[4]d — it's bleeding.",
		plain:    "%[1]s%[2]s rends the %[3]s for %[4]d.",
	}
	lacerateArms = procMessageArms{
		defeated: "%[1]s%[2]s's Lacerate opens the %[3]s up for good — %[4]d.",
		proc:     "%[1]s%[2]s lacerates the %[3]s for %[4]d — it's bleeding.",
		plain:    "%[1]s%[2]s lacerates the %[3]s for %[4]d.",
	}
	searingLightArms = procMessageArms{
		defeated: "%[1]s%[2]s's Searing Light burns the %[3]s away for %[4]d.",
		proc:     "%[1]s%[2]s sears the %[3]s for %[4]d — it's burning.",
		plain:    "%[1]s%[2]s sears the %[3]s for %[4]d.",
	}
)

func backstabMessage(name string, target core.Enemy, damage, quality int, defeated, crit bool) string {
	return procSkillMessage(backstabArms, name, target, damage, quality, defeated, crit, false)
}

// aoeSkillMessage / aoeEmptyMessage format the AoE log lines (hit + empty fallback).
func aoeSkillMessage(name, skillNoun, hitVerb string, hits, damage, quality int) string {
	return fmt.Sprintf("%s%s's %s %s %d foes for %d each.", qualityTag(quality), name, skillNoun, hitVerb, hits, damage)
}

func aoeEmptyMessage(skillNoun, emptyVerb string) string {
	return fmt.Sprintf("%s %s.", skillNoun, emptyVerb)
}

// qualityLine builds a "<grade><name><rest>" log line: qualityTag(quality) + name,
// then the rest formatted. `rest` is everything AFTER the name (e.g. "'s Mass Mend
// finds no wounds." or " mends %d allies."). The shared head for the buff/utility
// handlers that all open with the quality tag + caster name.
func qualityLine(quality int, name, rest string, args ...any) string {
	return qualityTag(quality) + name + fmt.Sprintf(rest, args...)
}

// killOrPlainLine is the 2-arm (plain / kill) analogue of procSkillMessage for the
// bespoke single-target damagers that format via qualityLine with the SAME args in
// both arms: it picks the defeated wording over the plain one. One home for "the
// kill line beats the plain line" so those sites stop re-spelling the qualityLine call.
func killOrPlainLine(quality int, name, plain, defeated string, killed bool, args ...any) string {
	rest := plain
	if killed {
		rest = defeated
	}
	return qualityLine(quality, name, rest, args...)
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
	if rank <= 0 || physDamage <= 0 || !core.MemberAvailable(*member) || core.MemberStarving(*member) {
		return // starving members can't mend — lifesteal included
	}
	heal := scaleByRank(physDamage, rank, core.BloodthirstHealPerRank)
	if heal <= 0 {
		return
	}
	before := member.HP
	core.GainUpTo(&member.HP, member.MaxHP, heal)
	if gained := member.HP - before; gained > 0 {
		// VFXHeal cue — lifesteal is an HP gain, not a hit.
		core.EnqueuePartyVFX(g, core.VFXHeal, partyIndex)
		setBattleMessageCat(g, fmt.Sprintf("%s's bloodthirst restores %d HP.", member.Name, gained), core.LogHeal)
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

// applyKillingSpree grants the Thief's Cutpurse capstone an ATB burst when their
// turn felled at least one foe (EnemyKillsThisTurn). No-op without the node / no kills.
func applyKillingSpree(g *core.GameState, actor core.ActorRef) {
	if !actor.ValidPartyIndex(g.Party) || g.Battle.EnemyKillsThisTurn <= 0 {
		return
	}
	if core.PassiveRank(&g.Party[actor.Index], core.PassiveKillingSpree) <= 0 {
		return
	}
	gainPartyReadiness(g, actor.Index, core.KillingSpreeReadiness)
	setBattleMessageCat(g, fmt.Sprintf("%s rides the kill — a burst of speed!", g.Party[actor.Index].Name), core.LogHeal)
}

// applyOverchargeRegen trickles MP back to a Wizard holding Overcharge at the end
// of their turn (the passive's sustain half; the free-cast half lives in chargeMP).
func applyOverchargeRegen(g *core.GameState, actor core.ActorRef) {
	if !actor.ValidPartyIndex(g.Party) {
		return
	}
	m := &g.Party[actor.Index]
	if m.HP <= 0 || core.PassiveRank(m, core.PassiveOvercharge) <= 0 {
		return
	}
	core.GainUpTo(&m.MP, m.MaxMP, core.OverchargeMPRegenPerTurn)
}

// tickAncestralSpirit lets the summoner's shade strike a living foe at the end of
// the summoner's turn, then drains one turn. Kept OUT of the phys/kill tallies (the
// shade, not the member). No-op without an active shade / no foe.
func tickAncestralSpirit(g *core.GameState, actor core.ActorRef) {
	if !actor.IsParty || !actor.ValidPartyIndex(g.Party) {
		return
	}
	m := &g.Party[actor.Index]
	if m.HP <= 0 || m.SpiritTurns <= 0 {
		return
	}
	m.SpiritTurns--
	target := core.NextLivingBattleEnemy(g)
	if target < 0 {
		return
	}
	raw := scaleFloorOne(core.MemberAttackDamage(*m, 0), core.AncestralSpiritDamageMult)
	noun := core.EnemySingularNoun(core.BattleMemberAt(g, target))
	dealt, defeated := damageEnemyUntallied(g, target, raw, autoStrikeQuality, core.SkillTagPhys)
	core.EnqueueEnemyVFX(g, core.VFXSlash, target)
	switch {
	case defeated:
		setBattleMessageCat(g, fmt.Sprintf("%s's ancestral shade cuts the %s down for %d!", m.Name, noun, dealt), core.LogDeath)
	case dealt > 0:
		setBattleMessageCat(g, fmt.Sprintf("%s's ancestral shade strikes the %s for %d.", m.Name, noun, dealt), core.LogDamageFoe)
	}
	if m.SpiritTurns == 0 {
		setBattleMessage(g, fmt.Sprintf("%s's ancestral shade fades.", m.Name))
	}
}

// resolveMeteorIfDue counts a pending Meteor's fuse down one turn; at 0 the AoE
// lands on every living enemy for the cast-time damage. No-op when none is in flight.
func resolveMeteorIfDue(g *core.GameState) {
	if g.Battle.MeteorFuse <= 0 {
		return
	}
	g.Battle.MeteorFuse--
	if g.Battle.MeteorFuse > 0 {
		return
	}
	dmg := g.Battle.MeteorDamage
	g.Battle.MeteorDamage = 0
	hits := 0
	forEachLivingEnemy(g, func(slot int, _ *core.Enemy) {
		damageEnemy(g, slot, dmg, autoStrikeQuality, core.SkillTagMagic)
		core.EnqueueEnemyVFX(g, core.VFXEmber, slot)
		hits++
	})
	if hits > 0 {
		triggerBigShake(g)
		setBattleMessageCat(g, "The meteor crashes down across the pack!", core.LogDamageFoe)
	}
}

// gainPartyReadiness adds to a party actor's carry-over ATB gauge (Killing Spree's
// speed burst) — the positive party-side mirror of pushEnemyReadiness.
func gainPartyReadiness(g *core.GameState, partyIndex, amount int) {
	if amount <= 0 || !partyIndexValid(g, partyIndex) {
		return
	}
	ref := core.ActorRef{IsParty: true, Index: partyIndex}
	if g.Battle.Readiness == nil {
		g.Battle.Readiness = map[core.ActorRef]int{}
	}
	g.Battle.Readiness[ref] += amount
	refreshTurnForecast(g) // readiness shifted — re-bake the next-round forecast
}

// offTurnReflect applies an off-turn counter-strike at enemySlot for `raw` damage
// with the given tag/VFX, prints the defeated/dealt messages, and returns the
// damage dealt. The strike is kept OUT of the per-turn Bloodthirst tally
// (snapshot/restore) since it lands outside the actor's own turn; callers that
// should still lifesteal feed the returned amount to bloodthirstHeal explicitly.
// Shared by tryRiposte (phys) and tryRetribution (magic).
func offTurnReflect(g *core.GameState, enemySlot, raw int, tag core.SkillTag, vfx core.VFXKind, defeatedMsg, dealtMsg func(noun string, amount int) string) (dealt int, defeated bool) {
	enemy, ok := livingEnemyAt(g, enemySlot)
	if !ok {
		return 0, false
	}
	noun := core.EnemySingularNoun(enemy)
	dealt, defeated = damageEnemyUntallied(g, enemySlot, raw, autoStrikeQuality, tag)
	core.EnqueueEnemyVFX(g, vfx, enemySlot)
	if defeated {
		setBattleMessageCat(g, defeatedMsg(noun, dealt), core.LogDeath)
	} else if dealt > 0 {
		setBattleMessageCat(g, dealtMsg(noun, dealt), core.LogDamageFoe)
	}
	return dealt, defeated
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
	raw := scaleFloorOne(core.MemberAttackDamage(*member, 0), core.RiposteDamageMult)
	name := member.Name
	dealt, _ := offTurnReflect(g, enemySlot, raw, core.SkillTagPhys, core.WeaponHitVFX(core.EquippedWeapon(*member)),
		func(noun string, _ int) string { return fmt.Sprintf("%s ripostes — the %s drops!", name, noun) },
		func(noun string, amount int) string {
			return fmt.Sprintf("%s ripostes the %s for %d.", name, noun, amount)
		})
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
	reflect := scaleByRank(dealt, rank, core.RetributionReflectPerRank)
	if reflect < 1 {
		return
	}
	name := g.Party[defender].Name
	offTurnReflect(g, enemySlot, reflect, core.SkillTagMagic, core.VFXSmite,
		func(noun string, amount int) string {
			return fmt.Sprintf("%s's retribution fells the %s for %d!", name, noun, amount)
		},
		func(noun string, amount int) string {
			return fmt.Sprintf("The %s takes %d from %s's retribution.", noun, amount, name)
		})
}

func resolveEnemyMiss(g *core.GameState, slot int) {
	enemy, ok := livingEnemyAt(g, slot)
	if !ok {
		return
	}
	target := pickEnemyAttackTarget(g)
	if target < 0 {
		return
	}
	stampEnemyBump(enemy)
	setBattleMessage(g, fmt.Sprintf("%s's attack misses %s!", core.EnemyDisplayName(enemy), g.Party[target].Name))
}

// resolveEnemyAttacker applies one enemy's attack against a chosen member, scaled
// by defend quality. Returns true if the hit landed (false if attacker was dead).
func resolveEnemyAttacker(g *core.GameState, slot int, defendQuality int) bool {
	enemy, ok := livingEnemyAt(g, slot)
	if !ok {
		return false
	}
	target := pickEnemyAttackTarget(g)
	if target < 0 {
		return false
	}
	stampEnemyBump(enemy)
	// Dodge precedes damage: a sidestep eats the whole swing (no damage/proc/
	// lifesteal). The defend quality is still recorded. Skills aren't dodgeable.
	if core.MemberRollDodge(g.Rand(), &g.Party[target]) {
		recordQuality(g, defendQuality, target, true)
		setBattleMessage(g, fmt.Sprintf("%s sidesteps %s.", g.Party[target].Name, core.TheFoe(enemy)))
		// Riposte on dodge (Warrior Battle Sense).
		tryRiposte(g, target, slot)
		return true
	}
	rawDamage := core.EnemyBasicDamage(enemy)
	// Enemy crit: DEX-driven RollCrit at Miss grade (no timing bonus — enemies
	// don't press a bar), keeping them on a flat crit floor.
	enemyCrit := core.RollCrit(g.Rand(), core.EffectiveEnemyStats(enemy), core.TimingQualityMiss)
	damage := core.ScaleIncomingDamage(rawDamage, defendQuality)
	// Phys-tagged so party Armor damps it; the Defend brace (+floor-1 soak) lives in
	// damagePartyMemberDefendableCrit, which also multiplies the crit POST-armor. dealt
	// is the final figure for the message.
	dealt, _ := damagePartyMemberDefendableCrit(g, target, damage, core.SkillTagPhys, enemyCrit)
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
	setBattleMessageCat(g, appendCrit(enemyHitMessage(*enemy, g.Party[target].Name, dealt, defendQuality, g.Party[target].Defending), enemyCrit), core.LogDamageParty)
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
		if core.StampEnemyDebuff(enemy, core.SkillIceArmor, core.NegStatDebuff(core.Stats{SPD: core.IceArmorChillSPD}, core.IceArmorChillTurns)) {
			setBattleMessage(g, fmt.Sprintf("%s's ice armor chills %s.", g.Party[target].Name, core.TheFoe(enemy)))
		}
	}
	// Retribution LAST (lifesteal + ice-armor chill resolved first), as it may kill the attacker.
	tryRetribution(g, slot, target, dealt)
	return true
}

// pickEnemyAttackTarget chooses (and commits the cursor to) the party member the
// acting enemy hits, round-robin via EnemyAttackCursor (separate from PartyTarget).
// The melee/front-row gate and Taunt override live in core.PeekEnemyAttackerTarget
// so the render forecast picks the identical slot.
func pickEnemyAttackTarget(g *core.GameState) int {
	target := core.PeekEnemyAttackerTarget(g)
	if target >= 0 {
		g.Battle.EnemyAttackCursor = target
	}
	return target
}

func enemyHitMessage(enemy core.Enemy, targetName string, damage, defendQuality int, defending bool) string {
	def := core.EnemyInfoFor(enemy)
	if defendQuality > core.TimingQualityMiss {
		if damage <= 0 {
			return fmt.Sprintf("%s blocks %s!", targetName, core.TheFoe(&enemy))
		}
		return fmt.Sprintf("%s blocks %s (%d).", targetName, core.TheFoe(&enemy), damage)
	}
	if defending {
		return fmt.Sprintf("%s soaks %s for %d.", targetName, core.TheFoe(&enemy), damage)
	}
	return fmt.Sprintf("%s %s %s for %d.", core.TheEnemy(def), def.AttackVerbSingular, targetName, damage)
}
