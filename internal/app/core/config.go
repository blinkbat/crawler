package core

const (
	// InitialWindowWidth/Height seed rl.InitWindow at startup. The window is
	// immediately resized to the monitor by render.SetDisplayMode(DisplayFullscreen),
	// so these are NOT the runtime screen dimensions — never use them for
	// HUD layout (render code reads rl.GetScreenWidth/Height via screenSize()
	// in the render package). They also double as the default size the
	// Windowed display mode snaps to on the first Fullscreen→Windowed toggle.
	InitialWindowWidth  = 1180
	InitialWindowHeight = 820

	// TargetFPS is the render frame cap fed to rl.SetTargetFPS at boot.
	// Co-located with the window-size seeds so all startup tuning lives
	// in one place rather than as a bare literal in run.go.
	TargetFPS = 120

	// MaxFrameStep clamps the per-frame delta time the update loops act
	// on, so a stall (alt-tab, breakpoint, GC pause) can't teleport the
	// player through a wall or fast-forward animations. Both explore.Update
	// and battle.Update pass their dt through ClampFrameTime. 1/15s = a
	// 15 FPS floor on simulation stepping.
	MaxFrameStep = float32(1.0 / 15.0)

	TileSize      = 2.05
	WallHeight    = 2.25
	EyeHeight     = 1.32
	StepDuration  = 0.18
	TurnDuration  = 0.14
	BumpDuration  = 0.18
	FlashDuration = 0.16
	// HitKnockbackDuration is how long the receiver's recoil offset
	// lasts after taking damage. A touch longer than BumpDuration so
	// the impact reads as "the hit shoved them" — the attacker has
	// already pulled their swing back by the time the receiver
	// finishes flinching. Magnitude is governed by HitKnockbackDist
	// + the BumpOffset sine curve, same shape as the lunge bump.
	HitKnockbackDuration = float32(0.24)
	// HitKnockbackDist is the peak world-units displacement during
	// the recoil. Smaller than BumpOffset's lunge distance (0.20-
	// 0.22) so the receiver doesn't moonwalk halfway across the
	// arena on every hit — just a clear "they felt that" shove.
	HitKnockbackDist     = float32(0.14)
	DeathFadeDuration    = 0.55
	VictoryDanceDuration = 3.0
	MouseSense           = 0.0024
	// StickLookSense scales right-stick free-look. Unlike MouseSense
	// (applied per raw mouse-delta) this is dt-scaled in updateFreeLook
	// since an analog stick is a sustained hold — the unit is roughly
	// "radians per second at full tilt." Tuned so a full push reaches
	// the yaw clamp in ~0.35s, matching the mouse path's feel.
	StickLookSense      = 2.2
	MaxLookYaw          = 0.78
	MaxLookPitch        = 0.62
	FreeLookReturnSpeed = 3.4
)

// AnimKind tags Player.Anim so the renderer/updater can dispatch by kind
// instead of poking at duration/elapsed alone.
type AnimKind int

const (
	AnimNone AnimKind = iota
	AnimStep
	AnimTurn
)

// BattlePhase is the top-level state of the battle FSM. Values are
// internal-only (no save file format depends on the integers), so the
// historical gaps are gone and the enum walks via iota.
type BattlePhase int

const (
	BattleNone BattlePhase = iota
	BattlePlayer
	BattleWon
	BattleLost
	BattleAttackTiming
	BattleEnemyTiming
)

// ActionMode is the sub-state of BattlePlayer — which input mode the action
// menu is currently in (action picker, target picker, item picker, etc.).
type ActionMode int

const (
	ActionMenu ActionMode = iota
	ActionEnemyTarget
	ActionPartyTarget
	ActionItemMenu
	ActionItemTarget
	// ActionSkillMenu is the skill-selection submenu — opens when the
	// player picks the "Skill" row, lets them choose which of the
	// class's 3 learned skills to cast. Confirming a row in this menu
	// arms the chosen skill and transitions into its target mode
	// (party / enemy / no-target) the same way performSkill used to.
	ActionSkillMenu
)

// ActionRow enumerates the in-battle action menu rows. The integer values
// double as g.Battle.MenuIndex cursor positions; reordering this enum
// reorders the menu. Typed so a free `int` can't accidentally stand in
// where a row label is expected.
type ActionRow int

const (
	ActionRowAttack ActionRow = iota
	ActionRowSkill
	ActionRowItem
	ActionRowDefend
)

// ActionRowCount is the wrap modulus for the action-menu cursor.
const ActionRowCount = int(ActionRowDefend) + 1

const (
	// SightRadius is the Chebyshev fog-of-war reveal radius around
	// the player. 1 = the 3×3 window centered on the player tile is
	// marked Visited on every successful step. The corner minimap and
	// the panels Map tab both read Visited to fade unexplored tiles
	// (panels also hides entity markers on fogged tiles).
	SightRadius = 1

	// --- Panels Map-tab zoom (cells-on-screen) ----------------------------
	//
	// The zoomable Map tab measures zoom as "how many tiles fit across the
	// view." Lives in config alongside the other UI tunables so the explore
	// input handler (clamp + step) and the render draw (default fallback for
	// struct-literal GameStates that didn't seed PanelsMapZoom) read one
	// source instead of each package re-declaring the literal 14.
	//
	//   PanelMapZoomDefault — initial cells-on-screen (fits a ~16-wide map).
	//   PanelMapZoomMin/Max — soft-clamp bounds for the Up/Down zoom.
	//   PanelMapZoomStep    — cells-on-screen change per zoom press.
	//
	// Step is deliberately coarse so the range resolves to just a handful
	// of distinct stops (8 / 16 / 24 / 32 / 40 / 48) rather than the old
	// 22-stop crawl — a couple of presses takes you from "room" to "whole
	// map." The default + min/max are all multiples of the step so every
	// stop lands cleanly and the clamp can't strand the player on a
	// fractional zoom.
	PanelMapZoomDefault = 16
	PanelMapZoomMin     = 8
	PanelMapZoomMax     = 48
	PanelMapZoomStep    = 8

	// --- Pack AI (junkyard-dog leash) -------------------------------------
	//
	// Packs wander when the player steps. They never leave their leash
	// (Chebyshev distance from Pack.HomeX/HomeZ ≤ PackLeashRadius), they
	// don't move every step (only PackStepChance of the time, rolled per
	// pack independently), and they pick a direction by a simple rule:
	// if the player is inside the leash radius, step closer; otherwise
	// pick a random cardinal that stays inside the leash.
	//
	//   PackLeashRadius  — Chebyshev radius around the spawn tile.
	//   PackStepChance   — 0..1 chance to even attempt a move per player
	//                      step. Lower = sleepier dogs.
	//   PackChaseRadius  — Chebyshev distance at which a pack abandons
	//                      wander for the "lazy chase" branch. Smaller
	//                      than PackLeashRadius so far-away wanderers
	//                      stay random until the player draws close.
	PackLeashRadius = 4
	PackStepChance  = 0.35
	PackChaseRadius = 3

	// PackAIPatrol tuning. A patrolling pack paces along the X axis out
	// to PatrolRadius tiles from its home, bouncing at the ends and at
	// walls. PatrolStepChance is higher than PackStepChance so a sentry
	// keeps a steady, readable beat rather than idling like a dog.
	PatrolRadius     = 4
	PatrolStepChance = 0.6

	// PackAISkittish tuning. A skittish pack flees when the player is
	// within SkittishFleeRadius (Chebyshev), stepping directly away; past
	// that it wanders its leash. Matched to PackChaseRadius so prey starts
	// running at the same range a junkyard dog would start chasing.
	SkittishFleeRadius = 3

	// BattleSplashDuration is how long the encounter banner sits on screen at
	// the start of a battle. The battle code seeds Battle.Splash with this and
	// the renderer uses it for ease-in/ease-out math, so they stay in sync.
	BattleSplashDuration = float32(1.15)

	AttackTimingDuration = float32(1.4)
	DefendTimingDuration = float32(1.3)

	TimingFlashDuration = float32(0.32)
	// TallyHitFlashDuration is the per-window feedback hold on a
	// multi-press tally bar. Shorter than TimingFlashDuration so
	// each hit in a rapid-fire sequence reads as a distinct pop
	// instead of running together into one long bloom.
	TallyHitFlashDuration = float32(0.18)
	QualityResultDuration = float32(0.70)

	// Hit-stop is the brief world-pause inserted between the timing flash and
	// the action's apply step on Great/Excellent grades. The bar's already
	// frozen at the press; this freezes EVERYTHING else (sprite bumps, popup
	// floats, enemy bars) so the moment punctuates. Tuned short — under 200ms
	// — to feel like a satisfying punch, not a stutter.
	HitStopGreat     = float32(0.10)
	HitStopExcellent = float32(0.16)

	// Combat screen shake — the camera-punch juice on a well-timed hit.
	// Set as a countdown on Battle.ShakeTimer when a Great/Excellent press
	// resolves (CombatShakeFor); the render camera offsets by
	// CombatShakeMagnitude world units, scaled by ShakeTimer/CombatShakeMax,
	// with the oscillation driven off the wall clock so the screen visibly
	// shakes even through the impact freeze (hit-stop). Excellent shakes
	// harder + longer than Great. Tunable; CombatShakeMagnitude=0 disables.
	CombatShakeGreat     = float32(0.12)
	CombatShakeExcellent = float32(0.22)
	CombatShakeMax       = CombatShakeExcellent // normalizer for the intensity ramp
	CombatShakeMagnitude = float32(0.045)       // peak camera offset, world units

	// Sequence arrow pulse: how long an arrow scales up after landing a
	// correct tap. Slightly less than the flash duration so the pulse decays
	// before the bar fades, keeping each tap visually punctuated.
	SequencePulseDuration = float32(0.22)
	EnemyTurnIntro        = float32(0.85)
	// Charge bars get a longer pre-arm pause so the player has time to read
	// the prompt. The bar arms early if the player presses/holds the input,
	// see updateAttackTiming for the skip logic.
	AttackTimingIntro = float32(0.35)
	// ChargeTimingIntro is the auto-arm timeout for charge bars: the
	// bar waits this long for the player to engage their input before
	// arming on its own. Pressing/holding during the wait skips
	// straight into the bar (see updateAttackTiming). 3s is the
	// "wait for the player, but don't softlock if they're afk" sweet
	// spot — long enough to read the prompt and breathe, short enough
	// that a missed cue doesn't burn the whole turn.
	ChargeTimingIntro = float32(3.0)
	// BlockBumpDuration is intentionally LONGER than BumpDuration (0.22 vs
	// 0.18). The hit landing on an enemy gets its own DamageFlash + popup
	// to read the impact; a successful block has none of that — the defender
	// just doesn't lose much HP. The longer recoil sells the block visually
	// when the damage number is "1" or "0."
	BlockBumpDuration = float32(0.22)

	// Charge minigame: hold the input through three ticks, then release at
	// the peak. ChargeTick1Pct..ChargeTick3Pct / ChargePeakStart / ChargePeakEnd
	// are VISUAL positions on the bar — tick lines are evenly spaced at
	// quarters, with a 10%-wide peak band tucked in after the third tick.
	//
	// The cursor's elapsed→visual mapping is non-linear (see
	// chargeSegments in timing.go). Each tick segment runs at a strictly
	// faster slope than the previous one, so the cursor visibly
	// accelerates with every notch it crosses — start slow, fast by the
	// peak. resolveCharge grades off the cursor's visual position; the
	// segment table is the single source of truth for both render and
	// grade so they can't drift.
	ChargeTimingDuration = float32(1.8)
	ChargeTick1Pct       = float32(0.25)
	ChargeTick2Pct       = float32(0.50)
	ChargeTick3Pct       = float32(0.75)
	// ChargePeakStart MUST equal ChargeTick3Pct: resolveCharge treats
	// "past tick 3" and "entering the peak window" as the same boundary,
	// so the charge-grade bands are only contiguous when they match.
	// Tied to the same literal so a balance edit can't open a grading gap.
	ChargePeakStart = ChargeTick3Pct
	ChargePeakEnd   = float32(0.85)

	// Pickpocket sequence minigame: tap a randomized run of N directions in
	// order before time runs out. Each correct tap holds the grade; each
	// miss (wrong key OR a slot that timed out unfilled) drops it one notch.
	// Finishing all-correct under StealFastThreshold bumps grade by one
	// (capped at Excellent).
	StealTimingDuration = float32(1.8)
	StealSequenceLength = 5
	StealFastThreshold  = float32(1.0)

	// DefendingDamageMult scales incoming damage when the target picked the
	// Defend action on their previous turn. Stacks multiplicatively with the
	// defend timing quality, so a perfectly-blocked hit on a defending member
	// is reduced even further.
	DefendingDamageMult = float32(0.5)

	// Attack accuracy curve. MeleeAccuracy / RangedAccuracy in types.go
	// compute per-swing hit chance as:
	//     AccuracyBaseline + AccuracyPerStat*stat + timingBonus
	// then clamp to [0, 1]. The governing stat depends on the attack:
	// MELEE accuracy is driven by STR, RANGED by DEX. timingBonus comes
	// from timingGrades.AccuracyBonus below. With this pair, a STR-6
	// Warrior melee-hits 0.79 on a Miss timing; a STR-2 caster hits 0.63.
	AccuracyBaseline = 0.55
	AccuracyPerStat  = 0.04

	// FlyingMeleeAccuracyPenalty is subtracted from a basic attack's hit
	// chance when the target is Flying and the wielder's weapon strikes in
	// melee (not WeaponIsRanged). The scaffold for "bring a bow to fight
	// bats" — a ranged weapon shrugs the penalty entirely. Applied to the
	// post-clamp accuracy, so unlike the stat/timing curve it can pull even
	// an Excellent press below a guaranteed hit: melee-vs-flyer is meant to
	// be unreliable. Tunable; 0 disables the scaffold. Only the basic attack
	// reads this (skills aren't accuracy-gated).
	FlyingMeleeAccuracyPenalty = 0.30

	// DodgeChance curve. A party member rolls a dodge against every
	// incoming enemy basic attack: dodge succeeds → no damage, no status
	// proc. DEX is the only driver, scaled linearly with a saturating
	// cap so a future high-DEX rogue can't ever be untouchable.
	// Warrior/Cleric/Wizard at DEX 2 dodge ~4%; Thief at DEX 6 dodges
	// ~12%; cap kicks in at DEX 15.
	DodgePerDEX = 0.02
	DodgeCap    = 0.30

	// Crit curve. Every connecting damage roll has a chance to crit;
	// crits multiply the post-armor damage by CritMultiplier. Base
	// chance + DEX scaling + a per-grade bonus pulled from
	// timingGrades.CritBonus. CritPerDEX is intentionally LOW so timing
	// (the player skill) is the dominant lever — DEX is a sweetener,
	// not the main dial. Warrior (DEX 2) at Good ≈ 12%; Thief (DEX 6)
	// at Excellent ≈ 36%. Doubling DEX from 2 → 4 lifts crit by ~2pp;
	// chasing an Excellent press lifts it by ~20pp.
	CritBaseline   = 0.05
	CritPerDEX     = 0.008
	CritCap        = 0.6
	CritMultiplier = 2

	// StatusShortenDivisor controls how much WIS shaves off the rolled
	// duration of an enemy-applied status (Sleep / Poison / Webbed /
	// Confuse). Each StatusShortenDivisor points of WIS removes one
	// turn from the roll. Floor is 1 — high WIS shortens but doesn't
	// outright skip. Cleric (WIS 6) loses 2 turns; everyone else (WIS
	// 1-2) loses nothing or one turn.
	StatusShortenDivisor = 3

	// ATBReadyThreshold is the readiness gate an actor must cross to
	// take a turn under the tick-based scheduler. Each tick every alive
	// actor's readiness gains its SPD; whoever crosses first acts (then
	// keeps the overflow). Higher SPD → reaches the gate sooner AND
	// more often. This is a continuous weight, not a per-round bonus —
	// a SPD 6 actor takes ~2 turns for every 1 turn a SPD 3 actor
	// takes, with the slots naturally interleaved by who hits the gate
	// next instead of clumped at round boundaries.
	ATBReadyThreshold = 100

	// ATBQueueSlotMultiplier caps the per-round queue length at
	// target × this multiplier — the runaway-fast actor safety net.
	// With target = count of alive actors with SPD>0, a value of 4
	// means a single 8× faster actor still can't act more than 4
	// times per round and the slow actors all get their one turn.
	// Sibling of ATBReadyThreshold so both ATB tuning knobs sit
	// together.
	ATBQueueSlotMultiplier = 4

	// BurnTickDamage is the per-turn damage applied to a burning actor at
	// the start of their own turn. Flat so the strategic value of burn
	// stays predictable across enemy HP scales.
	BurnTickDamage = 2

	// PoisonTickDamage is the per-turn damage applied to a poisoned party
	// member, ticked immediately AFTER their action resolves (vs. burn,
	// which ticks at the start of the actor's turn). The "after the act"
	// timing means the player still gets their action in, but bleeds out
	// faster than they can heal it back without dedicated cleansing.
	PoisonTickDamage = 1

	// Status duration bounds. Every (Poison / Sleep / Stun / Webbed /
	// Confuse / Burn) status rolls a uniform duration in
	// [Min, Max] inclusive when it lands. Co-located so a balance
	// pass touches one block — earlier passes had Poison here,
	// Sleep + Stun in their own blocks lower in the file, and the
	// new Webbed / Confuse in the roster-expansion section, which
	// made "how long does X last?" a three-place search.
	PoisonMinTurns       = 3
	PoisonMaxTurns       = 5
	SleepMinTurns        = 2
	SleepMaxTurns        = 5
	StunMinTurns         = 1
	StunMaxTurns         = 2
	SpiderWebbedMinTurns = 3
	SpiderWebbedMaxTurns = 3
	WispConfuseMinTurns  = 2
	WispConfuseMaxTurns  = 2
	// (Burn min/max travel on SkillEffect.Burn fields per skill —
	// no global default since only the Wizard's Firebolt sets it
	// today, and the registry value is the canonical source.)

	// Skill / enemy proc chances. Lifted out of the per-entry registry
	// literals (party.go skillDefinitions, enemies.go enemyDefinitions) so
	// a balance pass touches one file. The registry still owns the
	// per-entry binding; these constants are the values it cites.
	StealBaseChance          = 0.40 // Thief: Steal base success before timing-quality scaling.
	FireboltBurnChance       = 0.45 // Wizard: Firebolt burn inflict before quality scaling.
	DiseasedRatPoisonChance  = 0.60 // Diseased Rat: per-bite poison inflict.
	GoblinMageCastChance     = 0.50 // Goblin Mage: per-turn roll into Firebolt / Sleep vs plain melee.
	SpiderWebCastChance      = 0.45 // Cave Spider: roll-to-Web vs plain bite per turn.
	VampireBatLifesteal      = 0.60 // Vampire Bat: fraction of post-armor damage healed back per bite.
	WispConfuseCastChance    = 0.50 // Wisp: roll-to-Confuse vs flicker-bite per turn.
	WispConfuseRetargetRoll  = 0.50 // Wisp: per-action chance a Confused member retargets randomly.
	StoneGolemSlamCastChance = 0.40 // Stone Golem: roll-to-Stoneslam vs single-target smash per turn.
	NecromancerCastChance    = 0.55 // Necromancer: combined roll into Raise / Firebolt vs incant-melee.
	NecromancerRaiseLimit    = 2    // Necromancer: hard cap on RaiseBones casts per battle.

	// Day/night cycle tuning. Six phases of StepsPerPhase player tile-steps
	// make up one full loop (StepsPerCycle). Only landed exploration steps
	// advance the cycle (battles don't tick it), so combat preserves the
	// phase the player walked into.
	StepsPerPhase = 25
	StepsPerCycle = StepsPerPhase * TimeOfDayCount

	// OutdoorCeilingThreshold is the ceiling-coverage fraction above which
	// an area counts as an enclosed interior (roofed, no sky) rather than
	// outdoor. One definition of "has a roof" shared by the spooky-dungeon
	// lighting override (render) and the rain gate (core.AreaIsOutdoor) so
	// the two can't drift. A field or roofless forest scores near 0; a real
	// dungeon roofs most of its tiles.
	OutdoorCeilingThreshold = 0.30

	// Ambient rain (outdoor weather) tuning — purely atmospheric, see
	// core/weather.go. A bluegray wash eases in and darkens the open-sky
	// view, then rain falls for a spell before lifting. The state machine
	// advances on landed player steps (trigger roll / downpour length /
	// cooldown); the tint Intensity eases per frame so it fades rather than
	// snaps. Rain only happens outdoors and never indoors.
	//
	// Durations are framed against the day/night cycle (StepsPerCycle = 150
	// steps = one full day, StepsPerPhase = 25): a storm is short — under a
	// phase to a couple phases — and the cooldown spans roughly half a day to
	// ~1.2 days so rain is a recurring event without being constant.
	RainStartChance       = 0.012 // per outdoor step (once off cooldown): chance a storm begins
	RainMinSteps          = 18    // shortest downpour, in player steps (~0.7 phases)
	RainMaxSteps          = 50    // longest downpour, in player steps (~2 phases)
	RainCooldownMin       = 70    // min clear steps after a storm (~half a day) before rain may roll again
	RainCooldownMax       = 180   // max of that random cooldown span (~1.2 days)
	WeatherRampSpeed      = 0.40  // Intensity (0..1) eased per second — full tint ramp ≈ 2.5s
	WeatherRainStartLevel = 0.85  // Intensity the darkening must reach before the rain actually falls

	// Lightning (heavy storms only). A bolt blanks the world view bright
	// for a blink, then the flash decays; bolts are scheduled at random
	// gaps (one RNG draw per bolt, never per frame). All in seconds.
	LightningIntervalMin = 4.0  // shortest gap between bolts
	LightningIntervalMax = 13.0 // longest gap between bolts
	LightningDecayPerSec = 3.6  // flash brightness lost per second (≈0.28s blink)

	// BattleLogMaxLines caps the rolling combat log buffer so a long fight
	// doesn't grow it unbounded. The renderer reads len(Log) to draw the
	// last-N visible lines; this cap is the ceiling for any scrollback
	// feature that might land later.
	BattleLogMaxLines = 40

	// MaxMapDimension caps editor map width/height. Used by both the typed
	// numeric input and the +/- resize buttons so they share one ceiling.
	MaxMapDimension = 200

	// MinMapDimension is the smallest playable map. Border walls take 2
	// cells in each axis; 4×4 leaves a 2×2 interior — the tightest you
	// can put a player start and one pack on without overlap.
	MinMapDimension = 4
	// DefaultNewMapDimension is the seed width / height the editor's
	// "New" modal arms with. Shared so the title-screen editor.New()
	// path and the in-editor New modal can't drift apart on what a
	// "fresh map" looks like.
	DefaultNewMapDimension = 16

	// LevelUpApplyRowIndex is the cursor slot of the "Apply changes"
	// row in the level-up modal — sits one past the last stat row
	// (StatSTR..StatSPD). LevelUpRowCount is the total row count
	// (StatCount stat rows + 1 Apply row). Owned by core so the input
	// handler in explore/levelup.go and the renderer in
	// render/levelup.go share one truth; the modal's "row 6 is Apply"
	// rule used to live as a private constant in explore and a magic
	// `int(core.StatCount)` literal in render, which drifted twice
	// during the skill-point row's removal.
	LevelUpApplyRowIndex = int(StatCount)
	LevelUpRowCount      = int(StatCount) + 1
)

// PressWindow groups the press-bar window geometry. Values are fractions
// of the bar's duration. At construction time the window's position is
// randomized within [Min, Max] so two consecutive bars don't land in the
// same place — but it never starts before Min so the player can't get hit
// with a window that opens immediately. Width is fixed; MaxEnd clamps the
// tail so a window can't run into the last sliver of the bar (slides back
// to fit, see NewTimingState).
var PressWindow = struct {
	MinStart float32
	MaxStart float32
	Width    float32
	MaxEnd   float32
}{
	MinStart: 0.38,
	MaxStart: 0.62,
	Width:    0.18,
	MaxEnd:   0.96,
}

// MultiPressWindow is the per-fraction geometry config for tally-mode
// press bars (Swipe today, future N-hit skills). Layout is
// derived from the hit count rather than a fixed zone block:
// LeadInFrac is where the first window opens, WindowWidthFrac is
// each accept zone's width, CommitZoneFrac is the late "press
// here to end" tail. NewMultiPressState reads these values and
// distributes `count` windows evenly across the gap between the
// lead-in and the commit zone. Sibling of PressWindow above so a
// balance pass on either bar lands in one file.
var MultiPressWindow = struct {
	LeadInFrac      float32
	WindowWidthFrac float32
	CommitZoneFrac  float32
}{
	LeadInFrac:      0.20,
	WindowWidthFrac: 0.08,
	CommitZoneFrac:  0.15,
}

// SwipeHitFracs are the two hand-placed tally-window centers for
// Swipe's press bar (fractions of the bar duration): one around the
// middle and one just before the commit tail — a "wind up, then the
// big swing" rhythm rather than two evenly-spread beats. Passed to
// core.NewTallyStateAtCenters; kept here beside MultiPressWindow so a
// Swipe-feel balance pass lands in one file.
var SwipeHitFracs = []float32{0.5, 0.78}

// timingGrades is the single per-grade attribute table for the timed-hit
// minigame. Every core-side function that varies by TimingQuality reads
// from this slice — Label, attack/defense multipliers, and the
// accuracy-roll bonus all live in one row per grade so a balance pass
// touches one place instead of four parallel switches.
//
// Render-side (color, throb intensity) and battle-side (audio cue) attrs
// live in their own per-package tables (qualityVisuals in render/timing.go,
// gradeSounds in battle/battle.go) — package layering keeps audio out of
// core and rl.Color out of core, so we get one table per layer.
//
// Defense multipliers are <1 (lower incoming damage); attack multipliers
// are >=1 (higher outgoing damage); the accuracy bonus is added to the
// stat-driven baseline and clamped at 1.0 (see MeleeAccuracy / RangedAccuracy).
var timingGrades = []struct {
	Label         string
	Atk           float32
	Def           float32
	AccuracyBonus float64
	CritBonus     float64
}{
	TimingQualityMiss:      {Label: "Miss...", Atk: 1.0, Def: 1.0, AccuracyBonus: 0.0, CritBonus: 0.0},
	TimingQualityNice:      {Label: "Nice!", Atk: 1.25, Def: 0.75, AccuracyBonus: 0.10, CritBonus: 0.02},
	TimingQualityGood:      {Label: "Good!", Atk: 1.5, Def: 0.5, AccuracyBonus: 0.20, CritBonus: 0.05},
	TimingQualityGreat:     {Label: "Great!", Atk: 1.75, Def: 0.35, AccuracyBonus: 0.30, CritBonus: 0.12},
	TimingQualityExcellent: {Label: "Excellent!", Atk: 2.0, Def: 0.25, AccuracyBonus: 0.45, CritBonus: 0.25},
}

// init asserts timingGrades covers every TimingQualityXxx grade. The
// other two parallel tables (qualityVisuals in render, gradeSounds in
// battle) carry their own length-check inits against TimingQualityCount.
// Adding a new grade is now: extend the iota in timing.go, add a row
// here, add a row in qualityVisuals, add a row in gradeSounds — any
// one missing panics at startup.
func init() {
	if len(timingGrades) != int(TimingQualityCount) {
		panic("core: timingGrades length must match TimingQualityCount — add a row when extending the grade enum")
	}
}

// SkillID identifies a learned skill. Stored on Battle.PendingSkill and
// used as the map key for action handlers.
type SkillID int

const (
	SkillNone SkillID = iota
	SkillSwipe
	SkillPrayer
	SkillSteal
	SkillFirebolt
	// Class-thematic skills. Each class learns its signature (above)
	// plus two unique entries here; in-battle Tab cycles SkillCursor
	// across all three. See skillDefinitions for handler-shaping
	// notes and skillActionHandlers for the setup/apply registrations.
	//
	// Warrior: Crushing Blow (charge phys, stun proc), Whirlwind
	// (charge AoE phys).
	SkillCrushingBlow
	SkillWhirlwind
	// Cleric: Mass Mend (charge AoE heal), Smite (press magic damage).
	SkillMassMend
	SkillSmite
	// Thief: Backstab (charge phys, double damage on Excellent),
	// Venom Strike (sequence phys + Poison apply).
	SkillBackstab
	SkillVenomStrike
	// Wizard: Frost Lance (charge magic, stun on Great+), Arc Bolt
	// (sequence multi-target magic).
	SkillFrostLance
	SkillArcBolt
	// SkillSleep is the goblin-mage's status-inflict cast — single
	// target, puts a party member to sleep for SleepMin..SleepMaxTurns.
	// Wakes on any incoming damage. Tagged Magic so it bypasses armor.
	SkillSleep
	// SkillIngest is the Venus Mantrap's signature swallow — single
	// target, removes the party member from combat (skipped in the turn
	// queue, untargetable by friend or foe) until the mantrap that
	// swallowed them is defeated. Each mantrap holds at most one
	// prisoner; a mantrap with prey can still bite-attack but won't
	// ingest a second target. Tagged Magic so the cast itself bypasses
	// armor (it doesn't deal damage anyway).
	SkillIngest
	// SkillWeb is the Cave Spider's tempo-control cast — applies the
	// Webbed status (half-SPD, can't be ingested while webbed) for
	// SpiderWebbedTurns turns. Tagged Magic so the apply bypasses
	// armor; no damage component.
	SkillWeb
	// SkillConfuse is the Will-o'-Wisp's status cast — applies the
	// Confused status (a 50/50 retarget roll on the afflicted
	// member's next two turns, friend or foe at random). Tagged
	// Magic, no damage component, WIS-resistible at apply time.
	SkillConfuse
	// SkillStoneslam is the Stone Golem's AoE phys cast — hits every
	// living party member for STR + SpellPower scaled by quality.
	// Phys-tagged so the player's Armor / Defending applies; the
	// Wizard takes the full slap, the Warrior eats it well.
	SkillStoneslam
	// SkillRaiseBones is the Necromancer's signature add-summon —
	// inserts one Skeleton into the active pack mid-battle. Capped
	// per battle via the skill definition's PerBattleCastLimit field
	// (not a per-cast counter on the enemy). Tagged Magic; no
	// targeting (the summon lands in the necromancer's own pack).
	SkillRaiseBones
)

// SkillTag classifies a skill for damage-type interactions (armor,
// future elemental resists) and for HUD color-coding. Phys damage
// clips against the target's Armor; Magic / Heal / Buff bypass it.
// Independent of SkillKind: Kind controls the stat-scaling formula
// (STR vs INT vs WIS), Tag controls the defensive interaction.
type SkillTag int

const (
	SkillTagNone SkillTag = iota
	SkillTagPhys
	SkillTagMagic
	SkillTagHeal
	SkillTagBuff
)

// Stun-related cast chances. The Sleep/Stun duration constants moved
// up to the "Status duration bounds" block earlier in this file so
// every status-duration tunable sits together; only the proc-chance
// gates remain here. Stun is the "skip your next turn" status:
// unlike Sleep, it does NOT clear on incoming damage — the target
// is locked for the full rolled duration. Tuned short (1–2 turns)
// because it's strictly upside for the player; the proc gate
// (quality) controls frequency, not the duration.
const (
	// CrushingBlowStunChance gates the Warrior's signature heavy hit.
	// Tuned high because the cost (3 MP) and damage (+4 base) are both
	// aggressive — a Great-or-better landing should reliably lock the
	// target down.
	CrushingBlowStunChance = 0.50
	// FrostLanceStunChance gates the Wizard's freeze. ALWAYS lands on
	// Great/Excellent (1.0 base) but the apply handler still goes
	// through the same probability seam so a future "magic resist"
	// stat can plug in at one place.
	FrostLanceStunChance = 1.0
	// FrostLanceStunTurns is Frost Lance's fixed 1-turn freeze (min ==
	// max, a hard lock rather than the variable StunMinTurns..MaxTurns
	// window Crushing Blow rolls). Named so the duration lives with the
	// other status-duration tunables instead of as a bare literal in the
	// skill definition.
	FrostLanceStunTurns = 1
	// VenomStrikePoisonChance gates the Thief's Poison apply. Tuned
	// high so a clean sequence reliably lands the DoT; a Miss timing
	// scales it down through the standard TimingBonusMult curve.
	VenomStrikePoisonChance = 0.75
)

// XP / level constants. Per-character XP and levels (one pool + one
// counter per PartyMember) with a geometric per-level cost: each level
// costs LevelXPBase × LevelXPRatio^(level-1) — 100, 200, 400, 800.
// LevelStatPoints is the number of points the player allocates on each
// level-up — small enough that one level is a noticeable bump, not a
// respec. BaseLevel is the level every member starts at (1, not 0, so
// the cost formula works out).
const (
	LevelXPBase  = 100
	LevelXPRatio = 2.0
	// MaxLevelXPCost saturates the geometric per-level cost so XPForLevel
	// always returns a sane positive int instead of overflowing to +Inf
	// (and an unspecified int conversion) at absurd levels. 1<<30 (~1.07e9
	// XP) is far past any reachable total, so it acts as an effective soft
	// level cap without ever producing a garbage cost.
	MaxLevelXPCost  = 1 << 30
	LevelStatPoints = 3
	// LevelSkillPoints is the number of skill points granted per
	// level-up. Land on PartyMember.SkillPoints; the player spends
	// them later from the Skills panel's tree UI via SpendSkillTier.
	// Default 1 — each level reliably unlocks one tier somewhere in
	// the tree, with no pressure to spend it immediately.
	LevelSkillPoints = 1
	BaseLevel        = 1

	// MPPerINT is the MaxMP gained per point of INT spent at level-up.
	// INT thus feeds the MP pool (casters who invest in INT both hit
	// harder AND cast more often), mirroring how VIT spends grow MaxHP.
	// The class's starting MaxMP is the authored base; INT grows it from
	// there. Spending INT tops off current MP by the same delta so the
	// bump feels immediately usable, the same way a VIT spend heals.
	MPPerINT = 2
)

const (
	North = 0
	East  = 1
	South = 2
	West  = 3
	// FacingCount is the number of cardinal facings — the wrap modulus for
	// NormalizeFacing and the size of facingTable. Named so the bare `4`
	// doesn't recur at each rotation site. (Facing is still a bare int
	// rather than a typed enum: Player.Facing and the +1 turn arithmetic
	// thread raw ints through too many call sites to retype safely here.)
	FacingCount = 4
)

// PauseMenuItem enumerates the rows in the top-level pause menu. The
// integer values double as menu cursor positions (g.MenuIndex), so
// reordering this enum reorders the menu. The top level is intentionally
// minimal — Options and Debug each descend into their own submenu, Quit
// exits. Single source of truth shared by explore (cursor dispatch) and
// render (row drawing) — neither side reinvents the count.
type PauseMenuItem int

const (
	PauseMenuOptions PauseMenuItem = iota // ▸ Options submenu (display, party stats, restart)
	PauseMenuDebug                        // ▸ Debug submenu (toggles + audio tools)
	PauseMenuQuit
)

// PauseMenuCount is the wrap modulus for the pause menu cursor. Bump by
// adding a PauseMenuItem enum constant above this line.
const PauseMenuCount = int(PauseMenuQuit) + 1

// OptionsMenuItem enumerates the rows in the Options submenu (opened from
// the pause menu's Options row). Player-facing settings/actions live here:
// the display-mode toggle, a jump to the party-stats dashboard, and a
// run restart. Integer values double as the cursor (g.OptionsMenuIndex).
type OptionsMenuItem int

const (
	OptionsMenuDisplay OptionsMenuItem = iota // Fullscreen / Windowed toggle
	OptionsMenuStats                          // open the Tome on the Stats tab
	OptionsMenuQuests                         // open the quest-journal overlay
	OptionsMenuSave                           // write the run to the save file
	OptionsMenuRestart
	OptionsMenuClose
)

// OptionsMenuCount is the wrap modulus for the Options submenu cursor.
const OptionsMenuCount = int(OptionsMenuClose) + 1

// DebugMenuItem enumerates the rows in the Debug submenu (opened from the
// pause menu's Debug row — now always reachable; the master "Debug Mode"
// on/off toggle lives INSIDE the submenu rather than gating access to it).
// Audio tools (the jukebox sound tester) live here too. Integer values
// double as the cursor position (g.DebugMenuIndex).
type DebugMenuItem int

const (
	// DebugMenuToggle flips DebugOverlay ("debug mode" — in-world tile
	// labels + coord readout). The submenu itself is always reachable now,
	// so this is an in-place toggle, not an access gate.
	DebugMenuToggle DebugMenuItem = iota
	DebugMenuEnemies
	DebugMenuAdvanceTime
	DebugMenuEasyQuit
	// DebugMenuRenderLog toggles the render-pass diagnostics log file
	// (crawler-render.log). When on, each DrawWorld writes a one-line
	// snapshot of camera + tile counts + shader IDs to disk so a
	// flicker/invisibility issue can be inspected from the resulting
	// log even when reproducing the bug from outside the editor.
	DebugMenuRenderLog
	// DebugMenuJukebox is the audio sound-tester: confirm cycles through
	// and plays the sound bank. Moved here from the top-level pause menu.
	DebugMenuJukebox
	DebugMenuClose
)

// DebugMenuCount is the wrap modulus for the debug submenu cursor. Bump by
// adding a DebugMenuItem constant above this line.
const DebugMenuCount = int(DebugMenuClose) + 1
