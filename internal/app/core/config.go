package core

const (
	// Window seeds for rl.InitWindow only — NOT runtime screen dims (the window
	// resizes to the monitor at startup; HUD layout reads rl.GetScreenWidth/Height).
	// Also the default size Windowed mode snaps to on first toggle.
	InitialWindowWidth  = 1180
	InitialWindowHeight = 820

	TargetFPS = 120

	// MaxFrameStep clamps per-frame dt (via ClampFrameTime) so a stall can't
	// teleport the player through walls / fast-forward animations. 1/15s floor.
	MaxFrameStep = float32(1.0 / 15.0)

	TileSize  = 2.05
	EyeHeight = 1.32
	// LevelStep is the world height of one elevation level — the sole vertical
	// unit. A wall IS the rendered face of an elevation step (no separate wall
	// height). 2.4 over a 2.05 tile is a ~49° ramp.
	LevelStep     = float32(2.4)
	StepDuration  = 0.18
	TurnDuration  = 0.14
	BumpDuration  = 0.18
	FlashDuration = 0.16
	// TurnRepeatDelay paces HELD turn AUTO-REPEAT only (additive rest between
	// turns, not during the turn animation); a single tap/press is unaffected.
	TurnRepeatDelay = float32(0.25)
	// FlashTintStrength is the peak fraction a flash washes a sprite toward white.
	FlashTintStrength = 0.86
	// HitKnockbackDuration: receiver recoil after taking damage. Longer than
	// BumpDuration so the hit reads as a shove. Magnitude = HitKnockbackDist + BumpOffset curve.
	HitKnockbackDuration = float32(0.24)
	// HitKnockbackDist is the peak recoil displacement (world units); smaller
	// than BumpOffset's lunge so the receiver doesn't moonwalk on every hit.
	HitKnockbackDist  = float32(0.14)
	DeathFadeDuration = 0.55
	// VictoryDanceDuration is the FULL victory-pose length; NOT VictoryDanceBeat
	// (the shorter spoils-card hold) — don't conflate them.
	VictoryDanceDuration = 3.0
	// Victory spoils-screen pacing, driven off Battle.VictoryElapsed: solo-pose
	// hold, XP-bar fill sweep, per-row slide-in stagger, per-row fade window.
	VictoryDanceBeat        = float32(0.9)
	VictoryBarFillDuration  = float32(1.0)
	VictoryLootStagger      = float32(0.15)
	VictoryLootFadeDuration = float32(0.22)
	// VictoryXPPerTick: XP per count-up blip (SoundXPTick) on the spoils screen.
	VictoryXPPerTick = 8
	MouseSense       = 0.0024
	// StickLookSense scales right-stick free-look; dt-scaled (unlike per-delta
	// MouseSense). Roughly radians/sec at full tilt.
	StickLookSense      = 2.2
	MaxLookYaw          = 0.78
	MaxLookPitch        = 0.62
	FreeLookReturnSpeed = 3.4
)

// AnimKind tags Player.Anim for dispatch by kind.
type AnimKind int

const (
	AnimNone AnimKind = iota
	AnimStep
	AnimTurn
)

// BattlePhase is the top-level battle FSM state. Internal-only (no save depends
// on the integers).
type BattlePhase int

const (
	BattleNone BattlePhase = iota
	BattlePlayer
	BattleWon
	BattleLost
	BattleAttackTiming
	BattleEnemyTiming
)

// ActionMode is the BattlePlayer sub-state — which input mode the action menu is in.
type ActionMode int

const (
	ActionMenu ActionMode = iota
	ActionEnemyTarget
	ActionPartyTarget
	ActionItemMenu
	ActionItemTarget
	// ActionSkillMenu is the skill-selection submenu; confirming arms the skill
	// and transitions to its target mode (see battle.updateSkillMenu).
	ActionSkillMenu
	// ActionFleeConfirm is the yes/no gate so a stray Confirm can't burn the turn fleeing.
	ActionFleeConfirm
	// ActionSwapTarget: pick another party tile to trade formation slots with;
	// Confirm swaps and ends the turn.
	ActionSwapTarget
)

// ActionRow enumerates the in-battle action menu rows; values double as the
// g.Battle.MenuIndex cursor, so reordering reorders the menu.
type ActionRow int

const (
	ActionRowAttack ActionRow = iota
	ActionRowSkill
	ActionRowItem
	ActionRowDefend
	// ActionRowSwap trades the actor's formation slot with another member's,
	// spending the turn. Only way to rearrange formation; keeps the 2-front/2-back grid.
	ActionRowSwap
	// ActionRowFlee is the LAST row: roll to escape and retreat to the pre-combat
	// tile. See performFlee / FleeChance.
	ActionRowFlee
)

// ActionRowCount is the wrap modulus for the action-menu cursor.
const ActionRowCount = int(ActionRowFlee) + 1

const (
	// SightRadius is the Chebyshev fog-of-war reveal radius. 1 = the 3×3 window
	// around the player is marked Visited per step. NOTE: this is the CONSTANT;
	// RevealRadius (packai.go) is the FUNCTION that consumes it.
	SightRadius = 1

	// Panels Map-tab zoom, measured in cells-on-screen. Step is coarse (stops at
	// 8/16/.../48); default + min/max are step multiples so the clamp lands cleanly.
	PanelMapZoomDefault = 48
	PanelMapZoomMin     = 8
	PanelMapZoomMax     = 64
	PanelMapZoomStep    = 8
	// Pan step scales with zoom (PanelsMapZoom / PanelMapPanDivisor), floored at
	// PanelMapPanStepMin cells.
	PanelMapPanDivisor = 6
	PanelMapPanStepMin = 2

	// Pack AI (junkyard-dog leash). Packs wander on player steps, never leaving
	// their leash; step closer if the player is inside it, else a random in-leash cardinal.
	//   PackLeashRadius — Chebyshev radius around the spawn tile.
	//   PackStepChance  — chance to attempt a move per player step (lower = sleepier).
	//   PackChaseRadius — range at which wander gives way to lazy chase (< leash).
	PackLeashRadius = 4
	PackStepChance  = 0.35
	PackChaseRadius = 3

	// Patrol AI: paces the X axis out to PatrolRadius, bouncing at ends/walls.
	// PatrolStepChance > PackStepChance for a steady sentry beat.
	PatrolRadius     = 4
	PatrolStepChance = 0.6

	// Skittish AI: flees directly away within SkittishFleeRadius, else wanders.
	// Matched to PackChaseRadius.
	SkittishFleeRadius = 3

	// BattleSplashDuration: how long the encounter banner shows. Seeds
	// Battle.Splash; the renderer reads it for ease math.
	BattleSplashDuration = float32(1.15)

	AttackTimingDuration = float32(1.4)
	DefendTimingDuration = float32(1.3)

	TimingFlashDuration = float32(0.32)
	// TallyHitFlashDuration: per-window hold on a multi-press tally bar. Shorter
	// than TimingFlashDuration so rapid hits read as distinct pops.
	TallyHitFlashDuration = float32(0.18)
	QualityResultDuration = float32(0.70)

	// Hit-stop: brief world-pause between the timing flash and apply on
	// Great/Excellent. Freezes everything except the already-frozen bar. <200ms.
	HitStopGreat     = float32(0.10)
	HitStopExcellent = float32(0.16)

	// Combat screen shake (camera punch on a well-timed hit). Each tier has a
	// PEAK (world-unit offset) and DURATION; oscillation runs off the wall clock
	// so it shakes through hit-stop. Armed via TriggerCombatShake. Big shake is
	// reserved for crits/AoE (armed explicitly, overrides the grade base); peak 0 mutes a tier.
	CombatShakeGreatPeak     = float32(0.016) // subtle normal Great
	CombatShakeGreatDur      = float32(0.10)
	CombatShakeExcellentPeak = float32(0.026) // a touch more for Excellent
	CombatShakeExcellentDur  = float32(0.14)
	CombatShakeBigPeak       = float32(0.055) // crits + AoE casts: the real punch
	CombatShakeBigDur        = float32(0.30)

	// Controller rumble — haptic half of impact feedback, armed with the shake.
	// RumblePerShakePeak maps a shake's world-unit peak to motor strength [0,1],
	// so rumble grades with the shake automatically.
	RumblePerShakePeak = float32(15.0)
	// Taking a hit buzzes too (no camera shake — armed directly in damagePartyMember).
	RumbleHurtStrength = float32(0.45)
	RumbleHurtDur      = float32(0.18)
	// Debug menu "Test Rumble" — strong + long enough to be unmistakable.
	RumbleTestStrength = float32(0.8)
	RumbleTestDur      = float32(0.5)

	// SequencePulseDuration: arrow scale-up after a correct tap. Slightly less
	// than the flash duration so the pulse decays before the bar fades.
	SequencePulseDuration = float32(0.22)
	EnemyTurnIntro        = float32(0.85)
	// AttackTimingIntro: longer pre-arm pause so the player can read the prompt;
	// arms early on press/hold (see updateAttackTiming).
	AttackTimingIntro = float32(0.35)
	// ChargeTimingIntro: auto-arm timeout for charge bars — waits this long for
	// the player to engage before arming itself (press/hold skips straight in).
	ChargeTimingIntro = float32(3.0)
	// BlockBumpDuration is LONGER than BumpDuration: a block has no DamageFlash/popup,
	// so the longer recoil sells it when the damage number is 1 or 0.
	BlockBumpDuration = float32(0.22)

	// Charge minigame: hold through three ticks, release at the peak.
	// ChargeTick*Pct / ChargePeak* are VISUAL bar positions (ticks at quarters,
	// a 10%-wide peak band after tick 3). The elapsed→visual mapping is non-linear
	// and accelerates per tick (chargeSegments, timing.go — the single source for
	// both render and grade).
	ChargeTimingDuration = float32(1.8)
	ChargeTick1Pct       = float32(0.25)
	ChargeTick2Pct       = float32(0.50)
	ChargeTick3Pct       = float32(0.75)
	// ChargePeakStart MUST equal ChargeTick3Pct: resolveCharge treats "past tick 3"
	// and "entering the peak window" as one boundary, so the grade bands stay contiguous.
	ChargePeakStart = ChargeTick3Pct
	ChargePeakEnd   = float32(0.85)
	// ChargeExcellentBandFrac: half-width (fraction of the peak window) of the
	// dead-center Excellent zone; outside it grades Great.
	ChargeExcellentBandFrac = float32(0.30)

	// Directional-sequence minigame: tap a random run of N directions in order;
	// each correct tap holds the grade, each miss drops it one notch. All-correct
	// under SequenceFastThreshold keeps the top grade. Used by Venom Strike (and
	// the Recall bar, which opts out of the speed clause).
	SequenceTimingDuration = float32(1.8)
	SequenceLength         = 5
	SequenceFastThreshold  = float32(1.0)

	// Reels (slot) minigame — Steal's gamble. Stop ReelCount reels; 3 match =
	// jackpot, 2 = Good, all distinct = Miss. Spin at ReelSpinMin..Max symbols/sec,
	// rolled per-reel so they desync (no guaranteed jackpot, but symbols stay readable).
	ReelTimingDuration = float32(4.5)
	ReelCount          = 3
	// 3 symbols (was 4) so matches land far more often while a real whiff stays possible.
	ReelSymbolCount = 3
	ReelSpinMin     = float32(3.25)
	ReelSpinMax     = float32(5.0)

	// Recall (memory) minigame — Arc Bolt's pattern. RecallPatternLength
	// directions show for RecallRevealTime, then hide; reproduce before
	// RecallTimingDuration. Duration MUST exceed the reveal so there's an input window.
	RecallTimingDuration = float32(3.6)
	RecallPatternLength  = 4
	RecallRevealTime     = float32(1.8)
	// RecallMaxRevealFrac caps the reveal to this fraction of the bar so the
	// pattern always hides with input time left (NewRecallState clamps to it).
	RecallMaxRevealFrac = float32(0.8)

	// Overcharge backfire — Firebolt's risk band. Releasing PAST the peak: a
	// guaranteed Excellent hit + OverchargeDamageBonus extra damage, but
	// OverchargeRecoil self-damage onto the caster.
	OverchargeDamageBonus = 4
	OverchargeRecoil      = 3

	// DefendingDamageMult scales incoming damage when the target Defended last
	// turn. Stacks multiplicatively with defend timing quality.
	DefendingDamageMult = float32(0.5)

	// Attack accuracy curve: AccuracyBaseline + AccuracyPerStat*stat + timingBonus,
	// clamped [0,1]. Governing stat is STR (melee) / DEX (ranged); timingBonus from
	// timingGrades.AccuracyBonus.
	AccuracyBaseline = 0.55
	AccuracyPerStat  = 0.04

	// FlyingMeleeAccuracyPenalty: subtracted from a basic MELEE attack's hit chance
	// vs a Flying target (a ranged weapon shrugs it). Applied post-clamp, so it can
	// pull even an Excellent press below a sure hit. 0 disables; skills aren't gated.
	FlyingMeleeAccuracyPenalty = 0.30

	// DodgeChance curve: DEX-driven, linear with a saturating cap (cap bites at DEX 15)
	// so no one is ever untouchable. Dodge = no damage, no status proc.
	DodgePerDEX = 0.02
	DodgeCap    = 0.30

	// Enemy basic-attack accuracy: rolled BEFORE the player's defend bar arms (a
	// miss skips the minigame). DEX-driven via EffectiveEnemyStats (so Blind etc.
	// lowers it), clamped [Floor, Cap]. Enemy SKILLS are NOT gated.
	EnemyAccuracyBaseline = 0.80
	EnemyAccuracyPerDEX   = 0.02
	EnemyAccuracyFloor    = 0.30
	EnemyAccuracyCap      = 0.95

	// EnemyDifficulty{Num,Den}: global "harder foes" dial as a rational (integer-exact,
	// no float import). 7/5 = 1.4×. ScaleEnemyDifficulty applies it at three seams —
	// spawn HP, basic-attack damage, enemy spell damage. Leaves accuracy/crit/XP/gold alone.
	EnemyDifficultyNum = 7
	EnemyDifficultyDen = 5

	// ShopSellDivisor: resale ratio (catalog Price / this, floored at 1 by ShopSellPrice).
	ShopSellDivisor = 2

	// Flee chance: party avg living level vs pack's — even ≈ BaseFleeChance, each
	// level of advantage shifts by FleePerLevelStep, clamped [FleeFloor, FleeCap].
	BaseFleeChance   = 0.50
	FleePerLevelStep = 0.10
	FleeFloor        = 0.10
	FleeCap          = 0.95

	// DefaultEnemyLevel: level a foe reads when its definition authors none.
	// Used only by the flee math (no XP/scaling wiring yet).
	DefaultEnemyLevel = 1

	// Crit curve: CritBaseline + CritPerDEX*DEX + per-grade timingGrades.CritBonus,
	// capped at CritCap; crit multiplies post-armor damage by CritMultiplier.
	// CritPerDEX is LOW so timing is the dominant lever, DEX a sweetener.
	CritBaseline   = 0.05
	CritPerDEX     = 0.008
	CritCap        = 0.6
	CritMultiplier = 2

	// TierDamageDoubler: the ×N a "doubles damage" skill tier applies (Backstab T2,
	// Crushing Blow T3). Stacks on top of CritMultiplier where both fire.
	TierDamageDoubler = 2

	// Passive skill-tree per-rank magnitudes (GrantSkill==SkillNone nodes; applied
	// at the matching battle hook). All are shares of an existing figure so they
	// scale with combat:
	//   LuckyStrikeCritPerRank    — Thief Cutpurse: +crit/rank (re-clamped at CritCap).
	//   BloodthirstHealPerRank    — Warrior Fury: heal this share of phys damage dealt/turn.
	//   RetributionReflectPerRank — Cleric Conviction: reflect this share of damage taken.
	//   ShadowStepBonusPerRank    — Thief Shadow Arts: +single-target damage when striking first.
	//   RiposteDamageMult         — Warrior Battle Sense: dodge counter damage (single rank).
	LuckyStrikeCritPerRank    = 0.05
	BloodthirstHealPerRank    = 0.10
	RetributionReflectPerRank = 0.20
	ShadowStepBonusPerRank    = 0.15
	RiposteDamageMult         = 0.75

	// StatusShortenDivisor: each this-many WIS shaves one turn off a rolled
	// enemy-applied status duration. Floor 1 (shortens, never skips).
	StatusShortenDivisor = 3

	// ATBReadyThreshold: readiness gate to take a turn. Each tick an alive actor's
	// readiness gains its SPD; first to cross acts and keeps the overflow. Higher
	// SPD = sooner and more often (continuous weight, not a per-round bonus).
	ATBReadyThreshold = 100

	// ATBQueueSlotMultiplier caps the per-round queue at target × this — the
	// runaway-fast-actor safety net (slow actors still get their turn).
	ATBQueueSlotMultiplier = 4

	// WebbedSpeedDivisor: a Webbed actor's effective SPD is divided by this for the
	// turn-queue sort. Untyped so it divides an int SPD cleanly.
	WebbedSpeedDivisor = 2

	// BurnTickDamage: per-turn damage to a burning actor at the START of its turn. Flat.
	BurnTickDamage = 2

	// PoisonTickDamage: per-turn damage to a poisoned member, ticked AFTER its
	// action (vs Burn at the start) — the act lands but bleeds out faster than self-heal.
	PoisonTickDamage = 1

	// BleedTickDamage: per-turn damage to a bleeding enemy at END of its turn.
	// Player-applied only (Rend/Lacerate); own counter, so it runs alongside Poison.
	BleedTickDamage = 2

	// Status duration bounds — each status rolls uniform in [Min, Max] inclusive.
	// Co-located so a balance pass touches one block.
	PoisonMinTurns = 3
	PoisonMaxTurns = 5
	SleepMinTurns  = 2
	SleepMaxTurns  = 5
	StunMinTurns   = 1
	StunMaxTurns   = 2
	// FrostLanceStunTurns: Frost Lance's fixed 1-turn freeze (min==max hard lock).
	// Its proc gate (FrostLanceStunChance) is with the cast chances below.
	FrostLanceStunTurns = 1
	// StunTurnStep: one turn of stun — the unit a skill tier grants/extends (Smite T3, Frost Lance T3).
	StunTurnStep         = 1
	SpiderWebbedMinTurns = 3
	SpiderWebbedMaxTurns = 3
	WispConfuseMinTurns  = 2
	WispConfuseMaxTurns  = 2
	// Bleed duration bounds (Rend/Lacerate DoT on Enemy.BleedTurns); proc gates
	// (Rend/LacerateBleedChance) are with the cast chances.
	BleedMinTurns = 3
	BleedMaxTurns = 4
	// (Burn min/max travel on SkillEffect.Burn per skill — no global default; only Firebolt sets it.)

	// Skill / enemy proc chances — lifted out of the registry literals so a balance
	// pass touches one file. The registry still owns the per-entry binding.
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

	// Guaranteed-apply gates: read 1.0 today but go through a named seam so a
	// future "resist" stat can plug in at one place.
	MantrapIngestCastChance = 1.0 // Venus Mantrap: always attempts Ingest when it casts (one prey, MantrapHasPrey).
	WebBindChance           = 1.0 // Cave Spider: Web always binds once cast (cast gate is SpiderWebCastChance).
	WispConfuseApplyChance  = 1.0 // Wisp: Confuse always lands once cast (cast gate is WispConfuseCastChance).

	// Day/night cycle: TimeOfDayCount phases of StepsPerPhase steps per loop.
	// Only landed exploration steps advance it (battles preserve the phase).
	StepsPerPhase = 25
	StepsPerCycle = StepsPerPhase * TimeOfDayCount

	// CrystalRechargeSteps: landed steps a dormant healing crystal needs to re-arm
	// (Charge climbs 1/step). Long enough that the heal+autosave is a deliberate resource.
	CrystalRechargeSteps = 60

	// OutdoorCeilingThreshold: ceiling-coverage fraction above which an area counts
	// as roofed interior (no sky). Shared by the dungeon lighting override and the
	// rain gate (core.AreaIsOutdoor) so they can't drift.
	OutdoorCeilingThreshold = 0.30

	// Ambient rain tuning (outdoor only, atmospheric — see core/weather.go). State
	// machine advances on landed steps; tint Intensity eases per frame.
	RainStartChance          = 0.012 // per outdoor step off cooldown: chance a storm begins
	RainMinSteps             = 18    // shortest downpour, in player steps
	RainMaxSteps             = 50    // longest downpour, in player steps
	RainCooldownMin          = 70    // min clear steps after a storm before rain may roll again
	RainCooldownMax          = 180   // max of that random cooldown span
	WeatherRampSpeed         = 0.40  // Intensity (0..1) eased per second — full ramp ≈ 2.5s
	WeatherRainStartLevel    = 0.85  // Intensity the darkening must reach before rain falls
	WeatherIntensityNearZero = 0.01  // Intensity at/below which a Clearing storm snaps to Clear

	// Lightning (heavy storms only). Bolts scheduled at random gaps (one RNG draw
	// per bolt). All in seconds.
	LightningIntervalMin = 4.0  // shortest gap between bolts
	LightningIntervalMax = 13.0 // longest gap between bolts
	LightningDecayPerSec = 3.6  // flash brightness lost per second (≈0.28s blink)

	// ActionLogMaxLines caps the rolling action log buffer.
	ActionLogMaxLines = 40

	// MaxMapDimension caps editor map width/height (shared by typed input + resize buttons).
	MaxMapDimension = 200

	// MinMapDimension: smallest playable map. Border walls take 2 cells/axis; 4×4
	// leaves a 2×2 interior — the tightest fitting a player start + one pack.
	MinMapDimension = 4
	// DefaultNewMapDimension: seed width/height for the editor's New modal (shared
	// by editor.New() and the in-editor modal).
	DefaultNewMapDimension = 16

	// LevelUpApplyRowIndex: cursor slot of the "Apply changes" row (one past the
	// last stat row). LevelUpRowCount = StatCount stat rows + 1. Shared by explore
	// input and render so the row index can't drift.
	LevelUpApplyRowIndex = int(StatCount)
	LevelUpRowCount      = int(StatCount) + 1
)

// PressWindow: press-bar window geometry, as fractions of the bar duration.
// Start is randomized in [MinStart, MaxStart] (never before MinStart). Width is
// fixed; MaxEnd clamps the tail (slides back to fit, see NewTimingState).
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

// MultiPressWindow: per-fraction geometry for tally-mode press bars. Layout is
// derived from the hit count: LeadInFrac (first window), WindowWidthFrac (each
// accept zone), CommitZoneFrac (late commit tail). NewMultiPressState distributes
// `count` windows evenly between the lead-in and the commit zone.
var MultiPressWindow = struct {
	LeadInFrac      float32
	WindowWidthFrac float32
	CommitZoneFrac  float32
	// CommitGapFrac: breathing room reserved before the commit zone (subtracted
	// from the window span). JitterFrac: per-window placement jitter as a fraction
	// of WindowWidthFrac.
	CommitGapFrac float32
	JitterFrac    float32
}{
	LeadInFrac:      0.20,
	WindowWidthFrac: 0.08,
	CommitZoneFrac:  0.15,
	CommitGapFrac:   0.02,
	JitterFrac:      0.20,
}

// SwipeHitFracs: Swipe's two hand-placed tally-window centers (bar fractions) —
// a "wind up, then big swing" rhythm. Passed to core.NewTallyStateAtCenters.
var SwipeHitFracs = []float32{0.5, 0.78}

// timingGrades is the single per-grade attribute table for the timed-hit minigame
// (Label, atk/def multipliers, accuracy & crit bonuses, impact knobs). Render
// (color/throb) and battle (audio) keep their own tables for package layering.
// Def < 1 (less incoming), Atk >= 1 (more outgoing); AccuracyBonus is added then
// clamped at 1.0. Only Great/Excellent get a non-zero HitStop/ShakePeak/ShakeDur.
var timingGrades = []struct {
	Label         string
	Atk           float32
	Def           float32
	AccuracyBonus float64
	CritBonus     float64
	HitStop       float32
	ShakePeak     float32
	ShakeDur      float32
}{
	TimingQualityMiss:      {Label: "Miss...", Atk: 1.0, Def: 1.0, AccuracyBonus: 0.0, CritBonus: 0.0},
	TimingQualityNice:      {Label: "Nice!", Atk: 1.25, Def: 0.75, AccuracyBonus: 0.10, CritBonus: 0.02},
	TimingQualityGood:      {Label: "Good!", Atk: 1.5, Def: 0.5, AccuracyBonus: 0.20, CritBonus: 0.05},
	TimingQualityGreat:     {Label: "Great!", Atk: 1.75, Def: 0.35, AccuracyBonus: 0.30, CritBonus: 0.12, HitStop: HitStopGreat, ShakePeak: CombatShakeGreatPeak, ShakeDur: CombatShakeGreatDur},
	TimingQualityExcellent: {Label: "Excellent!", Atk: 2.0, Def: 0.25, AccuracyBonus: 0.45, CritBonus: 0.25, HitStop: HitStopExcellent, ShakePeak: CombatShakeExcellentPeak, ShakeDur: CombatShakeExcellentDur},
}

// init asserts timingGrades covers every grade (qualityVisuals + gradeSounds
// carry their own length-check inits).
func init() {
	if len(timingGrades) != int(TimingQualityCount) {
		panic("core: timingGrades length must match TimingQualityCount — add a row when extending the grade enum")
	}
}

// SkillID identifies a learned skill (Battle.PendingSkill; action-handler map key).
type SkillID int

const (
	SkillNone SkillID = iota
	SkillSwipe
	SkillPrayer
	SkillSteal
	SkillFirebolt
	// Player-castable skills, grouped by class. No fixed per-class loadout: members
	// learn skills by ranking tree nodes (GrantSkill, core/skilltrees.go). See
	// skillDefinitions for handler notes. NOTE: all entries below SkillArcBolt are
	// APPENDED in serialization order — SkillID is a saved-map key (SkillTiers), so
	// a mid-enum insert renumbers later skills (same contract as ItemKind/EnemyKind).
	//
	// Warrior: Crushing Blow (charge phys, stun proc), Whirlwind (charge AoE phys).
	SkillCrushingBlow
	SkillWhirlwind
	// Cleric: Mass Mend (charge AoE heal), Smite (press magic damage).
	SkillMassMend
	SkillSmite
	// Thief: Backstab (charge phys, double on Excellent), Venom Strike (sequence phys + Poison).
	SkillBackstab
	SkillVenomStrike
	// Wizard: Frost Lance (charge magic, stun on Great+), Arc Bolt (sequence multi-target magic).
	SkillFrostLance
	SkillArcBolt
	// SkillSleep (Goblin Mage): single-target sleep for SleepMin..MaxTurns; wakes on
	// any damage. Magic (bypasses armor).
	SkillSleep
	// SkillIngest (Venus Mantrap): swallows one target out of combat until the
	// mantrap dies; one prey per mantrap. Magic.
	SkillIngest
	// SkillWeb (Cave Spider): Webbed (half-SPD, can't be ingested) for SpiderWebbedTurns. Magic, no damage.
	SkillWeb
	// SkillConfuse (Will-o'-Wisp): Confused (50/50 retarget over the next two turns).
	// Magic, no damage, WIS-resistible.
	SkillConfuse
	// SkillStoneslam (Stone Golem): AoE phys to every member (STR+SpellPower);
	// Phys-tagged so Armor/Defending applies.
	SkillStoneslam
	// SkillRaiseBones (Necromancer): summons a Skeleton into the active pack;
	// capped via PerBattleCastLimit. Magic, no targeting.
	SkillRaiseBones
	// SkillScan: single enemy, no damage/status — marks the KIND scanned in the
	// persistent bestiary (reveals exact HP). Kind-level, survives the battle.
	SkillScan
	// SkillBless (Cleric, Conviction): party-wide temporary STR/DEX/INT/WIS boost
	// for BlessBuffTurns. First SkillTagBuff user.
	SkillBless
	// SkillFireball (Wizard, Pyromancy): INT-scaled magic to every enemy + per-target
	// Burn roll — pack counterpart to Firebolt.
	SkillFireball
	// SkillPoisonCloud (Thief, Venomancy): light STR-scaled damage to every enemy +
	// per-target Poison — pack counterpart to Venom Strike.
	SkillPoisonCloud
	// SkillCleanse (Cleric, Mercy): clears curable debuffs (Poison/Sleep/Stun/Webbed/
	// Confused) via CureDebuffs, leaving Bless + Defending. NoUpgrades.
	SkillCleanse
	// SkillSecondWind (Warrior, Ancestral Call): flat self-heal, Utility-kind (not
	// WIS-gated). The Warrior's only heal; battle-only.
	SkillSecondWind
	// SkillRenewal (Cleric, Mercy): stamps a regen (RegenTurns/RegenPerTurn) ticking
	// at the ally's end-of-turn. The first HoT.
	SkillRenewal
	// SkillCripple (Thief, Subterfuge): FIRST enemy-side debuff — negative SPD on the
	// enemy BuffStats/BuffTurns mirror (folded by EffectiveEnemyStats), no damage.
	SkillCripple
	// SkillFrostbite (Wizard, Cryomancy): INT-scaled magic that ALWAYS chills (same
	// SPD debuff as Cripple) — the "damage + debuff" counterpart.
	SkillFrostbite
	// SkillCorrosiveVial (Thief, Subterfuge): strips the target's Armor (floored 0)
	// for the battle by mutating Enemy.Armor — permanent, not turn-counted.
	SkillCorrosiveVial
	// SkillConeOfCold (Wizard, Cryomancy): INT-scaled frost to every enemy + guaranteed
	// per-target chill — pack counterpart to Frostbite, via applyAoEStatusSkill.
	SkillConeOfCold
	// SkillSunder (Warrior, Battle Sense): STR-scaled phys that also shoves the target's
	// ATB readiness back (effect.ATBPush) — the offensive Cripple.
	SkillSunder
	// SkillTaunt (Warrior, Battle Sense): forces the enemy to attack the caster next
	// turn (Enemy.TauntedBy/TauntTurns). Single-rank utility.
	SkillTaunt
	// SkillWarBanner (Warrior, Ancestral Call): party-wide STR + Armor buff via the
	// shared party buff bundle — martial mirror of Bless.
	SkillWarBanner
	// SkillStoneSkin (Warrior, Ancestral Call): temporary Armor + MDef on one ally via
	// the buff bundle (BuffArmor/BuffMDef).
	SkillStoneSkin
	// SkillBlind (Cleric, Radiance): saps the enemy's DEX (the stat EnemyHitChance
	// reads) via the BuffStats mirror — the DEX sibling of Cripple.
	SkillBlind
	// SkillAegis (Cleric, Conviction): damage-absorbing shield (PartyMember.ShieldHP)
	// that soaks hits before HP until spent or battle end.
	SkillAegis
	// SkillSmokeBomb (Thief, Shadow Arts): one symmetric DEX magnitude — +DEX party
	// (evasion), -DEX every enemy (accuracy).
	SkillSmokeBomb
	// SkillIceArmor (Wizard, Cryomancy): self-buff (PartyMember.IceArmorTurns) granting
	// MDef + chilling any enemy that basic-attacks the caster.
	SkillIceArmor
	// SkillRend (Warrior, Fury): STR-scaled phys applying Bleed — the third DoT, on its
	// own Enemy.BleedTurns counter so it stacks with Poison.
	SkillRend
	// SkillLacerate (Thief, Venomancy): same Bleed DoT as Rend, flavored to stack with
	// the tree's Poison (separate counters).
	SkillLacerate
)

// init pins every SkillID's serialized value (SkillTiers is a saved map[SkillID]int).
// A mid-enum insert renumbers later skills and silently misattributes saved tiers;
// this panics at startup instead. APPEND only, then add one pinned line here.
func init() {
	assertAppendOnly("SkillID (renumbers saved SkillTiers keys)",
		SkillNone, SkillSwipe, SkillPrayer, SkillSteal,
		SkillFirebolt, SkillCrushingBlow, SkillWhirlwind,
		SkillMassMend, SkillSmite, SkillBackstab,
		SkillVenomStrike, SkillFrostLance, SkillArcBolt,
		SkillSleep, SkillIngest, SkillWeb, SkillConfuse,
		SkillStoneslam, SkillRaiseBones, SkillScan,
		SkillBless, SkillFireball, SkillPoisonCloud,
		SkillCleanse, SkillSecondWind, SkillRenewal,
		SkillCripple, SkillFrostbite, SkillCorrosiveVial,
		SkillConeOfCold, SkillSunder, SkillTaunt,
		SkillWarBanner, SkillStoneSkin, SkillBlind,
		SkillAegis, SkillSmokeBomb, SkillIceArmor,
		SkillRend, SkillLacerate,
	)
}

// SkillTag classifies a skill for damage-type interaction + HUD color. Phys clips
// against Armor; Magic/Heal/Buff bypass it. Independent of SkillKind (Kind = stat
// scaling, Tag = defensive interaction).
type SkillTag int

const (
	SkillTagNone SkillTag = iota
	SkillTagPhys
	SkillTagMagic
	SkillTagHeal
	SkillTagBuff
)

// Proc-chance gates for status/DoT skills. (Durations live in the "Status duration
// bounds" block.) Stun does NOT clear on damage (unlike Sleep).
const (
	// CrushingBlowStunChance: Warrior's heavy hit. High — its MP/damage cost is aggressive.
	CrushingBlowStunChance = 0.50
	// FrostLanceStunChance: Wizard freeze, always lands on Great/Excellent, but
	// still routes through this seam for a future magic-resist stat.
	FrostLanceStunChance = 1.0
	// VenomStrikePoisonChance: Thief Poison apply (Miss timing scales it via TimingBonusMult).
	VenomStrikePoisonChance = 0.75
	// FireballBurnChance: per-target Burn for the AoE; lower than single-target FireboltBurnChance.
	FireballBurnChance = 0.30
	// FireBurnMin/MaxTurns: Burn duration shared by Firebolt + Fireball.
	FireBurnMinTurns = 2
	FireBurnMaxTurns = 3
	// PoisonCloudPoisonChance: per-target Poison for the AoE; lower than single-target VenomStrikePoisonChance.
	PoisonCloudPoisonChance = 0.45
)

// Bless (Cleric) tuning — party-wide, always lands (timing cosmetic). Lifts
// STR/DEX/INT/WIS; VIT/SPD excluded (VIT desyncs MaxHP, SPD perturbs ATB order).
const (
	BlessBuffPerStat = 1 // base per-stat boost (tier 0)
	BlessBuffTurns   = 3 // base duration (fixed, not rolled)
)

// Cripple (Thief) tuning — first enemy-side debuff, always lands. Negative
// BuffStats.SPD folded by EffectiveEnemyStats (which floors effective SPD at 1).
const (
	CrippleSPDReduction = 2 // base SPD sapped (tier 0), as a negative BuffStats.SPD
	CrippleTurns        = 3 // base duration (fixed)
)

// Frostbite (Wizard) tuning — INT-scaled magic that always chills (Cripple's
// SPD debuff delivered with damage; timing scales only the damage).
const (
	FrostbiteDamageBase   = 2 // tier-0 base damage (INT-scaled)
	FrostbiteSPDReduction = 2 // SPD sapped by the chill (negative BuffStats.SPD)
	FrostbiteChillTurns   = 3 // base chill duration
)

// Corrosive Vial (Thief) tuning — permanent (battle-duration) armor break:
// mutates Enemy.Armor directly (floored 0), re-casting strips further.
const (
	CorrosiveArmorReduction = 4 // tier-0 Armor stripped per cast
)

// Cone of Cold (Wizard) tuning — AoE counterpart to Frostbite (lower damage +
// shorter chill, guaranteed per-target).
const (
	ConeOfColdDamageBase   = 1 // tier-0 per-target damage (INT-scaled)
	ConeOfColdSPDReduction = 1 // per-target SPD sapped
	ConeOfColdChillTurns   = 2 // per-target chill duration
)

// Sunder (Warrior) tuning — STR-scaled phys + a one-shot ATB readiness push
// (g.Battle.Readiness), distinct from Cripple's persistent SPD debuff.
const (
	SunderDamageBase     = 1                     // tier-0 base damage (STR-scaled)
	SunderATBPush        = ATBReadyThreshold / 2 // readiness knocked off (~half a turn; retuning the threshold keeps the intent)
	SunderATBPushPerTier = 25                    // extra push from the T3 upgrade
)

// Taunt (Warrior) tuning — forces the target to attack the caster. Single-rank.
const (
	TauntTurns = 1 // taunted-enemy turns the forced-target holds; drains at end-of-turn
)

// War Banner (Warrior) tuning — party-wide STR + Armor rally via the shared buff
// bundle (no-stack). VIT unused (buffs don't re-derive MaxHP); Armor stands in for toughness.
const (
	WarBannerPerStat = 1 // base STR boost (tier 0)
	WarBannerArmor   = 2 // base flat Armor per ally (tier 0)
	WarBannerTurns   = 4 // base duration (tier 0)
)

// Stone Skin (Warrior) tuning — temporary Armor + MDef on one ally via the buff
// bundle's BuffArmor/BuffMDef.
const (
	StoneSkinArmor = 3 // tier-0 flat Armor
	StoneSkinMDef  = 2 // tier-0 flat MDef
	StoneSkinTurns = 3 // base duration
)

// Blind (Cleric) tuning — saps the enemy's DEX (the stat EnemyHitChance reads)
// via the BuffStats mirror; DEX sibling of Cripple.
const (
	BlindDEXReduction = 3 // base DEX sapped (tier 0), a negative BuffStats.DEX
	BlindTurns        = 3 // base duration
)

// Aegis (Cleric) tuning — absorb pool that soaks post-mitigation damage before HP.
const (
	AegisShieldBase = 8 // tier-0 absorb pool (HP-equivalent points)
)

// Smoke Bomb (Thief) tuning — one symmetric DEX magnitude: +DEX party (evasion),
// -DEX every enemy (accuracy).
const (
	SmokeBombDEX   = 2 // base DEX swing (tier 0)
	SmokeBombTurns = 2 // base duration (each side ticks on its own turns)
)

// Ice Armor (Wizard) tuning — self-buff (PartyMember.IceArmorTurns) granting MDef
// + chilling any enemy that basic-attacks the caster.
const (
	IceArmorMDef      = 3 // flat MDef while warded
	IceArmorTurnsBase = 3 // base duration (tier 0)
	// Chill stamped on an attacker (enemy BuffStats mirror, like Cone of Cold's).
	IceArmorChillSPD   = 1
	IceArmorChillTurns = 2
)

// Rend (Warrior) + Lacerate (Thief) tuning — phys hits applying the Bleed DoT
// (BleedTickDamage/turn for BleedMin..MaxTurns on Enemy.BleedTurns). Quality-scaled
// proc via *BleedChance, no-stack + WIS-shorten through tryProcStatus.
const (
	RendDamageBase     = 2 // tier-0 base damage (SkillKindMelee-scaled)
	LacerateDamageBase = 1
	// Gate the Bleed apply before timing scaling — high, so it reads reliable.
	RendBleedChance     = 0.85
	LacerateBleedChance = 0.85
)

// Second Wind (Warrior) + Renewal (Cleric) heal tuning (tier ladder via skillTierTable).
const (
	SecondWindHealBase = 6 // flat self-heal (tier 0), Utility-kind so NOT WIS-scaled
	// Renewal is Heal-kind: the per-turn amount snapshots WIS-scaled value at cast.
	RenewalRegenBase  = 2 // base per-turn heal (tier 0)
	RenewalRegenTurns = 3 // base duration (fixed)
)

// XP / level constants. Per-character, geometric per-level cost: LevelXPBase ×
// LevelXPRatio^(level-1) — 100, 200, 400, 800. BaseLevel is 1 (not 0) so the formula works.
const (
	LevelXPBase  = 100
	LevelXPRatio = 2.0
	// MaxLevelXPCost saturates the geometric cost so XPForLevel can't overflow at
	// absurd levels (~1.07e9 XP — an effective soft cap, past any reachable total).
	MaxLevelXPCost  = 1 << 30
	LevelStatPoints = 3
	// LevelSkillPoints granted per level-up (PartyMember.SkillPoints, spent via BuySkillNode).
	LevelSkillPoints = 1
	BaseLevel        = 1

	// MPPerINT: MaxMP gained per INT spent at level-up (mirrors VIT→MaxHP).
	// Spending INT also tops off current MP by the same delta.
	MPPerINT = 2
)

const (
	North = 0
	East  = 1
	South = 2
	West  = 3
	// FacingCount: number of cardinal facings — wrap modulus for NormalizeFacing,
	// size of facingTable. (Facing stays a bare int, not a typed enum.)
	FacingCount = 4
)

// PauseMenuItem enumerates the top-level pause menu rows; values double as the
// g.MenuIndex cursor, so reordering reorders the menu.
type PauseMenuItem int

const (
	PauseMenuOptions PauseMenuItem = iota // ▸ Options submenu (display, party stats, restart)
	PauseMenuDebug                        // ▸ Debug submenu (toggles + audio tools)
	PauseMenuQuit
)

// PauseMenuCount is the wrap modulus for the pause menu cursor.
const PauseMenuCount = int(PauseMenuQuit) + 1

// OptionsMenuItem enumerates the Options submenu rows; values double as the
// g.OptionsMenuIndex cursor.
type OptionsMenuItem int

const (
	OptionsMenuDisplay   OptionsMenuItem = iota // Fullscreen / Windowed toggle
	OptionsMenuVibration                        // controller rumble On / Off
	OptionsMenuSave                             // write the run to the save file
	OptionsMenuRestart
	OptionsMenuClose
)

// OptionsMenuCount is the wrap modulus for the Options submenu cursor.
const OptionsMenuCount = int(OptionsMenuClose) + 1

// DebugMenuItem enumerates the Debug submenu rows; values double as the
// g.DebugMenuIndex cursor.
type DebugMenuItem int

const (
	// DebugMenuToggle flips DebugOverlay (in-world tile labels + coord readout).
	DebugMenuToggle DebugMenuItem = iota
	DebugMenuEnemies
	DebugMenuAdvanceTime
	DebugMenuEasyQuit
	// DebugMenuRenderLog toggles crawler-render.log — a per-DrawWorld snapshot of
	// camera + tile counts + shader IDs for diagnosing flicker/invisibility.
	DebugMenuRenderLog
	// DebugMenuJukebox: audio sound-tester (confirm cycles the sound bank).
	DebugMenuJukebox
	// DebugMenuAllSkills toggles g.DebugAllSkills: skill menu lists every skill, casts are free.
	DebugMenuAllSkills
	// DebugMenuBoostStats: one-shot ACTION — adds DebugStatBoost to every stat and
	// refreshes HP/MP (stacks on repeat).
	DebugMenuBoostStats
	// DebugMenuSkipBattles toggles g.DebugSkipBattles: engaging a pack instant-wins
	// (kills + XP + loot). Distinct from DebugMenuEnemies (no encounters, no reward).
	DebugMenuSkipBattles
	// DebugMenuTestRumble: one-shot rumble pulse (raylib's GLFW has no vibration; runs via XInput on Windows).
	DebugMenuTestRumble
	// DebugMenuRetro opens the Retro Filters sub-submenu (per-filter intensity sliders).
	DebugMenuRetro
	// DebugMenuStartDialog: test launcher — starts the area's first authored conversation.
	DebugMenuStartDialog
	// DebugMenuCombatTune opens the Combat Tuning sub-submenu (camera/foe/party
	// placement sliders).
	DebugMenuCombatTune
	// DebugMenuWipe opens the Screen Wipe FX sub-submenu (battle-entry transitions).
	DebugMenuWipe
	DebugMenuClose
)

// DebugMenuCount is the wrap modulus for the debug submenu cursor.
const DebugMenuCount = int(DebugMenuClose) + 1

// DebugStatBoost: amount DebugMenuBoostStats adds to every base stat per activation.
const DebugStatBoost = 100

// RetroFilterKind enumerates the 3D-world post-process filters. Each has an
// independent 0..1 intensity on GameState.RetroFilters; every non-zero filter
// applies in ONE shader pass in this fixed pipeline order.
type RetroFilterKind int

const (
	RetroFilterPixelate  RetroFilterKind = iota // chunky low-res UV quantize
	RetroFilterChroma                           // RGB fringing (VHS / worn composite)
	RetroFilterPosterize                        // color-level crush (8/16-bit feel)
	RetroFilterDither                           // 4×4 Bayer ordered dithering
	RetroFilterGameBoy                          // 4-shade green LCD palette remap
	RetroFilterScanlines                        // CRT horizontal line darkening
	RetroFilterPalette                          // snap each pixel to the nearest color in a fixed limited palette
	RetroFilterCount
)

// retroFilterNames: display labels, indexed by RetroFilterKind, length-locked to the enum.
var retroFilterNames = [RetroFilterCount]string{
	RetroFilterPixelate:  "Pixelate",
	RetroFilterChroma:    "Chroma Fringe",
	RetroFilterPosterize: "Posterize",
	RetroFilterDither:    "Dither",
	RetroFilterGameBoy:   "Game Boy",
	RetroFilterScanlines: "Scanlines",
	RetroFilterPalette:   "Palette",
}

func init() {
	for k := RetroFilterKind(0); k < RetroFilterCount; k++ {
		if retroFilterNames[k] == "" {
			panic("core: retroFilterNames has an empty entry — label every RetroFilterKind")
		}
	}
}

// validRetroFilter reports whether k indexes a filter slot (shared bounds rule).
func validRetroFilter(k RetroFilterKind) bool {
	return k >= 0 && k < RetroFilterCount
}

// RetroFilterName returns the display label for a filter kind ("" out of range).
func RetroFilterName(k RetroFilterKind) string {
	if !validRetroFilter(k) {
		return ""
	}
	return retroFilterNames[k]
}

// Retro Filters submenu rows: first RetroFilterCount positions ARE the filter
// kinds, then the toggles, Reset, All Off, Close. iota-seeded at RetroFilterCount
// so the ladder stays contiguous without hand-counting offsets.
const (
	RetroMenuSkyToggle = iota + int(RetroFilterCount)
	RetroMenuSpriteToggle
	RetroMenuResetAll
	RetroMenuAllOff
	RetroMenuClose
	RetroMenuCount
)

// RetroFilterStep: Left/Right intensity increment. RetroFilterToggleDefault: the
// ON level a Confirm-toggle uses when the filter's authored default is zero.
const (
	RetroFilterStep          = 0.1
	RetroFilterToggleDefault = 0.7
)

// DefaultRetroFilters is the out-of-the-box filter mix ("90s CD-ROM FMV" wash).
// New games start here; Reset to Default restores it.
func DefaultRetroFilters() [RetroFilterCount]float64 {
	var f [RetroFilterCount]float64
	f[RetroFilterPixelate] = 0.1
	f[RetroFilterChroma] = 0.2
	f[RetroFilterPosterize] = 0.1
	f[RetroFilterDither] = 0.4
	return f
}

// DefaultRetroFilterSky: skybox NOT filtered by default (clean sky behind the crunched world).
const DefaultRetroFilterSky = false

// DefaultRetroFilterSprites: billboards NOT filtered by default (FX stays crisp).
const DefaultRetroFilterSprites = false

// AdjustRetroFilter nudges an intensity by dir (±1) steps, clamped to [0, 1].
func AdjustRetroFilter(v *float64, dir int) {
	*v = Clamp(*v+float64(dir)*RetroFilterStep, 0, 1)
}

// ToggleRetroFilter flips filters[k] between off and its "on" level: its
// DefaultRetroFilters intensity if it has one, else RetroFilterToggleDefault.
func ToggleRetroFilter(filters *[RetroFilterCount]float64, k RetroFilterKind) {
	if !validRetroFilter(k) {
		return
	}
	if filters[k] > 0 {
		filters[k] = 0
		return
	}
	if def := DefaultRetroFilters()[k]; def > 0 {
		filters[k] = def
		return
	}
	filters[k] = RetroFilterToggleDefault
}

// AnyRetroFilterActive reports whether at least one filter has a non-zero
// intensity — the render side's gate for the whole post-process pass.
func AnyRetroFilterActive(filters *[RetroFilterCount]float64) bool {
	for _, v := range filters {
		if v > 0 {
			return true
		}
	}
	return false
}
