package render

import (
	"image/color"
	"math"
	"strconv"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// sqrt2Inv is 1/√2 — the unit-diagonal component shared by every 45°-axis glyph.
const sqrt2Inv = float32(0.7071)

// tau is one full turn in radians (2π).
const tau = 6.2831853

// degToRad converts degrees to radians (π/180).
const degToRad = float32(math.Pi / 180)

// hashSalt is Knuth's multiplicative-hash constant (golden-ratio odd multiplier,
// 0x9E3779B9) for spreading seeds across the 32-bit hash space.
const hashSalt = uint32(2654435761)

// fractSinHashA / fractSinHashB are the classic GLSL fract(sin(x)·k) constants,
// used by glyphJitter for a cheap hash of a CONTINUOUS float seed.
const (
	fractSinHashA = 12.9898
	fractSinHashB = 43758.5453
)

// hashPhase maps a 32-bit hash to a stable phase in [0,tau) (low 16 bits) so
// hash-seeded oscillators (torch flames) flicker out of sync.
func hashPhase(h uint32) float32 {
	return float32(h&0xFFFF) / 65535.0 * tau
}

// paletteSaturationCut pulls every BRIGHT accent (via mute()) toward its
// luminance gray for the muted library look; earthy base tokens skip mute().
// One knob: toward 1 = grayer, toward 0 = punchier.
const paletteSaturationCut = 0.30

// mapRGB applies f to c's R/G/B, preserving alpha — the single "transform RGB,
// keep A" seam (f owns any clamping).
func mapRGB(c rl.Color, f func(uint8) uint8) rl.Color {
	return rl.NewColor(f(c.R), f(c.G), f(c.B), c.A)
}

// mute desaturates c toward its luminance gray by paletteSaturationCut.
func mute(c rl.Color) rl.Color {
	lum := float32(c.R)*0.30 + float32(c.G)*0.59 + float32(c.B)*0.11
	return mapRGB(c, func(v uint8) uint8 {
		return uint8(float32(v) + (lum-float32(v))*paletteSaturationCut)
	})
}

// The Library palette (see UI_STANDARDS.md; "no new rl.NewColor literals for a
// surface that already has a token"). Dark glass framed in hardwood, gilt
// highlights. Two naming families coexist (canonical glassDeep/woodMid/inkPrimary
// + older surfacePrimary/borderSoft/textPrimary aliases below, same RGBs); both
// first-class — pick whichever reads best.
var (
	// ----- Glass surfaces (panel fills) -----
	// Two-layer: drawCard paints glassBaseWash then the glass tint, both
	// translucent (world composes through). Lands ~55-62% apparent opacity; the
	// mandatory text drop shadows keep ink legible at this thinness.
	glassBaseWash = rl.NewColor(8, 6, 10, 88)
	glassDeep     = rl.NewColor(14, 12, 18, 100)
	glassMid      = rl.NewColor(22, 18, 24, 84)
	glassWarm     = rl.NewColor(28, 22, 16, 118)
	glassDanger   = rl.NewColor(36, 16, 18, 112)
	veil          = rl.NewColor(0, 0, 0, 130)
	// surfaceCardBackdrop is an OPAQUE dark panel fill for modals that must hide
	// the world; painted UNDER the glass body so frame + filigree composite on top.
	surfaceCardBackdrop = rl.NewColor(24, 20, 29, 255)
	// surfaceDimScrim is the dark wash receding inactive elements (non-active party
	// cards). Own alpha so it stays tunable apart from glassBaseWash.
	surfaceDimScrim = rl.NewColor(6, 8, 14, 105)

	// ----- Wood frames -----
	woodDark   = rl.NewColor(48, 30, 18, 255)
	woodMid    = rl.NewColor(96, 62, 36, 255)
	woodLight  = rl.NewColor(150, 104, 64, 255)
	woodAccent = rl.NewColor(184, 140, 92, 255)
	woodInlay  = rl.NewColor(34, 22, 14, 175)

	// ----- Wood-accent fades -----
	// Single source for the wood rule/outline/icon fade tones (were scattered
	// fadeColor(woodAccent, X) literals).
	woodAccentGrid       = fadeColor(woodAccent, 0.16) // map-tab grid lines
	woodAccentInlayLip   = fadeColor(woodAccent, 0.22) // card inlay bottom lip
	woodAccentBevelSide  = fadeColor(woodAccent, 0.36) // card bevel side face
	woodAccentRule       = fadeColor(woodAccent, 0.38) // info-strip hairline rule
	woodAccentOutline    = fadeColor(woodAccent, 0.42) // glass-pane outline
	woodAccentSeam       = fadeColor(woodAccent, 0.55) // small-panel outline
	woodAccentIconSoft   = fadeColor(woodAccent, 0.70) // equipment-row icon
	woodAccentBevelTop   = fadeColor(woodAccent, 0.80) // card bevel top face
	woodAccentFrame      = fadeColor(woodAccent, 0.82) // minimap frame border
	woodAccentIcon       = fadeColor(woodAccent, 0.85) // panel / tick icon
	woodAccentIconBright = fadeColor(woodAccent, 0.90) // stat icon

	// ----- Gilt accents (selection / focus) -----
	giltDim    = rl.NewColor(160, 124, 64, 255)
	giltBright = rl.NewColor(232, 196, 112, 255)
	// Coin sigil tones (gold HUD coin glyph): brighter face, darker inner shade.
	coinFace  = rl.NewColor(218, 168, 78, 255)
	coinShade = rl.NewColor(152, 104, 42, 255)

	// ----- Controller glyphs (on-screen button icons) -----
	// Face-button hues (letter + ring on a dark body), mute()-wrapped like every
	// bright accent but still recognizable (A green/B red/X blue/Y amber).
	glyphAColor = mute(rl.NewColor(60, 168, 74, 255))  // confirm
	glyphBColor = mute(rl.NewColor(206, 66, 58, 255))  // back / cancel
	glyphXColor = mute(rl.NewColor(58, 124, 206, 255)) // use
	glyphYColor = mute(rl.NewColor(228, 178, 52, 255)) // panels / Tome
	// glyphBody: dark raised button fill; glyphRim: light bevel on non-face
	// buttons; glyphInk: neutral label ink.
	glyphBody = rl.NewColor(34, 30, 40, 235)
	glyphRim  = rl.NewColor(150, 140, 150, 220)
	glyphInk  = rl.NewColor(232, 226, 214, 255)

	// ----- Parchment ink (text) -----
	inkPrimary = rl.NewColor(232, 222, 196, 255)
	inkMuted   = rl.NewColor(184, 172, 144, 240)
	inkDim     = rl.NewColor(132, 122, 100, 220)
	inkAccent  = rl.NewColor(232, 196, 112, 255)
	// statusGlyphDark is the near-black for cut-out details in status glyphs and
	// the enemy roster pill's glyph/turn count (dark on the bright fill).
	statusGlyphDark = rl.NewColor(12, 10, 15, 255)
	// statusIconBacking is the dark disc behind a party status icon so its glyph
	// reads against the glass.
	statusIconBacking = rl.NewColor(22, 19, 26, 235)
	// statusNoneAccent is the neutral grey the PartyStatusNone row carries.
	statusNoneAccent = rl.NewColor(220, 220, 220, 220)

	// ----- Semantic aliases (same RGBs, predate the wood/glass/gilt names) -----
	surfacePrimary    = glassDeep
	surfaceLog        = glassMid
	surfaceVeil       = veil
	surfaceActiveTint = glassWarm
	surfaceTargetTint = rl.NewColor(20, 38, 32, 140)    // faint emerald glass for friendly target
	surfaceDownTint   = rl.NewColor(28, 22, 28, 115)    // knocked down — dim grey wash
	accentPartyDown   = rl.NewColor(120, 110, 116, 200) // knocked-down party card accent (name tick / edge)
	surfaceEnemyTint  = glassDanger

	// Enemy-roster row tints (drawEnemyRosterRow): live row fill + border, plus
	// the dimmed pair used while a defeated enemy fades.
	surfaceRosterRow      = rl.NewColor(20, 14, 22, 130)
	borderRosterRow       = rl.NewColor(96, 60, 64, 140)
	surfaceRosterRowFaded = rl.NewColor(28, 20, 24, 95)

	// Border aliases — drawCard's OUTERMOST stroke. Default = woodDark; active/
	// danger panels swap in a saturated tint so the focused surface reads first.
	borderDim    = rl.NewColor(60, 48, 36, 130) // dim hardwood, for disabled panels
	borderSoft   = woodDark                     // default outer stroke
	borderStrong = woodAccent                   // heading underline accent
	borderActive = giltBright                   // active actor panel frame
	borderTarget = mute(rl.NewColor(118, 200, 132, 235))
	borderEnemy  = mute(rl.NewColor(212, 120, 80, 235))
	borderDanger = mute(rl.NewColor(220, 88, 88, 235))

	textPrimary = inkPrimary
	textMuted   = inkMuted
	textLabel   = inkAccent
	// textDim is the faded-calligraphy ink (disabled text, footer hints, blurbs).
	textDim = inkDim

	// ----- Action-log category tints (logCategoryColor, by core.LogCategory) -----
	// Reuse existing semantic tones where the meaning lines up (heal = HP green,
	// foe death = gilt gold, party damage = the floating-popup red); only white and
	// the pale-blue info tone are log-specific.
	logDamageFoe   = rl.NewColor(244, 244, 248, 255) // party→foe damage — white
	logInfo        = rl.NewColor(150, 198, 238, 255) // neutral flavor / prompts — pale blue
	logDamageParty = partyDamagePopupColor           // party takes damage — popup red
	logHeal        = barHPHigh                       // party healed — HP green
	logDeath       = giltBright                      // a foe falls — gilt gold

	barHPHigh = mute(rl.NewColor(116, 200, 132, 255))
	// xpGainColor fills the victory XP bars — coin-gold (reward, not HP green).
	xpGainColor = coinFace
	barHPMid    = mute(rl.NewColor(224, 184, 88, 255))
	barHPLow    = mute(rl.NewColor(220, 88, 88, 255))
	barMP       = mute(rl.NewColor(104, 152, 224, 255))
	barEnemyHP  = mute(rl.NewColor(204, 76, 76, 255))
	// Enemy wound-state ramp — the roster health-label tint from unharmed green to
	// near-death red (near-death hotter than the muted bar so a small label reads).
	condEnemyUnharmed     = rl.NewColor(126, 231, 170, 255)
	condEnemyScuffed      = rl.NewColor(208, 226, 128, 255)
	condEnemyInjured      = rl.NewColor(246, 196, 91, 255)
	condEnemyBadlyWounded = rl.NewColor(244, 126, 75, 255)
	condEnemyNearDeath    = rl.NewColor(255, 78, 88, 255)
	// partyDamagePopupColor is the floating-number tone for PARTY damage — hotter
	// than barEnemyHP so a fast popup reads against the formation.
	partyDamagePopupColor = rl.NewColor(255, 100, 88, 255)
	barTrack              = rl.NewColor(8, 12, 22, 140) // near-black track, already muted
	// barMutedFill is the plum fill drawBar swaps in for a muted (downed) gauge.
	barMutedFill = rl.NewColor(96, 84, 92, 230)
	// barGhostHot is the trailing "damage ghost" segment (drawBarState) — hot
	// parchment-gold; gilt-family transient, NOT muted.
	barGhostHot = rl.NewColor(255, 226, 168, 235)

	// ----- Per-status accents (UI_STANDARDS.md) -----
	// Indexed by core.PartyStatusKind via partyStatusVisuals; exported so non-party
	// surfaces (enemy pills) can pull the same hue.
	statusPoison    = mute(rl.NewColor(148, 200, 96, 240))
	statusBleed     = mute(rl.NewColor(200, 56, 56, 240))
	statusBurn      = mute(rl.NewColor(240, 144, 72, 240))
	statusSleep     = mute(rl.NewColor(132, 196, 232, 240))
	statusStun      = mute(rl.NewColor(232, 220, 120, 240))
	statusWebbed    = mute(rl.NewColor(180, 140, 220, 240))
	statusConfused  = mute(rl.NewColor(220, 188, 96, 240))
	statusIngested  = mute(rl.NewColor(200, 132, 220, 240))
	statusDefending = mute(rl.NewColor(132, 196, 255, 240))
	statusDown      = mute(rl.NewColor(220, 102, 102, 235))
	// statusBlessed: POSITIVE buff — warm holy gilt, distinct from statusStun's
	// flatter yellow. Never flickers.
	statusBlessed = mute(rl.NewColor(244, 212, 128, 240))
	// statusRegen: POSITIVE heal-over-time (Renewal) — mint green, cleaner than
	// statusPoison's olive. Never flickers.
	statusRegen = mute(rl.NewColor(120, 224, 150, 240))
	// statusShielded: POSITIVE Aegis ward — teal/cyan, off the light-blues
	// (statusSleep/statusDefending). Never flickers.
	statusShielded = mute(rl.NewColor(96, 222, 214, 240))
	// statusIceArmor: POSITIVE frost ward — pale icy blue-white, cooler than
	// statusSleep. Never flickers.
	statusIceArmor = mute(rl.NewColor(186, 226, 248, 240))
	// Outline tints paired with the fills above for the enemy-pill silhouette
	// (lighter/more saturated = "glow with a hard rim").
	statusBurnOutline   = mute(rl.NewColor(255, 200, 120, 220))
	statusSleepOutline  = mute(rl.NewColor(190, 220, 244, 220))
	statusPoisonOutline = mute(rl.NewColor(180, 232, 132, 220))
	statusBleedOutline  = mute(rl.NewColor(248, 120, 120, 220))
	statusStunOutline   = mute(rl.NewColor(248, 232, 160, 230))

	// Turn-order panel: the danger red an enemy row reads as.
	turnEnemyColor = mute(rl.NewColor(245, 100, 92, 255))

	// Timing-bar accents (cursor white, held/RELEASE gold, sequence pass/fail).
	// Full alpha; softer sites wrap in colorWithAlpha.
	timingCursorColor = rl.NewColor(248, 248, 252, 255)
	timingHeldColor   = rl.NewColor(255, 244, 144, 255)
	seqOkColor        = mute(rl.NewColor(140, 232, 168, 255))
	seqFailColor      = mute(rl.NewColor(228, 96, 96, 255))
	// timingWarnColor: low-time amber before red; timingTickColor: charge-segment separator.
	timingWarnColor = rl.NewColor(232, 196, 92, 235)
	timingTickColor = rl.NewColor(28, 32, 44, 235)

	// Timing-grade ramps — attack (warm red→cream) and defend (cool red→ice)
	// flash tones, indexed Miss..Excellent by qualityVisuals. NOT muted (the
	// punch is the point).
	timingGradeAtkMiss      = rl.NewColor(220, 76, 76, 255)
	timingGradeAtkNice      = rl.NewColor(184, 96, 80, 255)
	timingGradeAtkGood      = rl.NewColor(232, 144, 80, 255)
	timingGradeAtkGreat     = rl.NewColor(255, 188, 88, 255)
	timingGradeAtkExcellent = rl.NewColor(255, 244, 144, 255)
	timingGradeDefMiss      = rl.NewColor(220, 76, 76, 255)
	timingGradeDefNice      = rl.NewColor(56, 110, 184, 255)
	timingGradeDefGood      = rl.NewColor(80, 152, 220, 255)
	timingGradeDefGreat     = rl.NewColor(120, 200, 248, 255)
	timingGradeDefExcellent = rl.NewColor(196, 240, 255, 255)

	// Per-mode timing-bar heading tints, paired with the timingHeading* labels.
	// (The Combo bar derives its from seqOkColor at its call site.)
	timingHeadingStrikeColor       = mute(rl.NewColor(255, 232, 168, 240)) // warm gold
	timingHeadingDefendColor       = mute(rl.NewColor(168, 220, 255, 240)) // cool blue
	timingHeadingChargeColor       = mute(rl.NewColor(255, 184, 96, 240))  // warm orange
	timingHeadingReelsColor        = mute(rl.NewColor(255, 244, 144, 240)) // gold-yellow gamble
	timingHeadingRecallMemoColor   = mute(rl.NewColor(255, 244, 144, 240)) // memorize: held-yellow
	timingHeadingRecallRecallColor = mute(rl.NewColor(140, 232, 168, 240)) // recall: thief green

	// reelSymbolColors are Steal's slot-symbol fill hues (well-separated so a match
	// reads). Indexed by symbol modulo length, so safe if ReelSymbolCount changes.
	reelSymbolColors = []rl.Color{
		mute(rl.NewColor(255, 206, 84, 255)),  // gold
		mute(rl.NewColor(96, 208, 255, 255)),  // cyan
		mute(rl.NewColor(236, 120, 200, 255)), // magenta
		mute(rl.NewColor(140, 232, 168, 255)), // green
	}

	// Battle-splash banner tones (near-black fill + warm cream title). Full alpha;
	// the splash applies its fade per-frame via colorWithAlpha.
	splashBgColor    = rl.NewColor(8, 10, 16, 255)
	splashTitleColor = rl.NewColor(248, 232, 198, 255)

	// Billboard tints for in-world combatant markers: warm off-white target,
	// redder attacker pulse.
	tintEnemyTargeted = rl.NewColor(255, 228, 190, 255)
	tintEnemyAttacker = rl.NewColor(255, 196, 156, 255)
	// Selector-pyramid colors for the same three states (brighter than the soft
	// sprite wash by design — paired, not identical).
	selectorEnemyTargetColor    = rl.NewColor(255, 224, 80, 255)
	selectorFriendlyTargetColor = rl.NewColor(118, 235, 136, 245)
	selectorEnemyAttackColor    = rl.NewColor(255, 96, 96, 245)
	// Party-side billboard tints (mirror of the enemy pair): downed gray + your-
	// turn warm wash.
	tintPartyDown   = rl.NewColor(110, 110, 120, 190)
	tintPartyActive = rl.NewColor(255, 245, 204, 255)

	// Editor + minimap entity-marker colors, shared by the editor markers/swatches
	// and the minimap dots. Exported via theme.MarkerXxx.
	markerStart    = rl.NewColor(255, 220, 124, 255)
	markerChest    = rl.NewColor(232, 180, 92, 255)
	markerChestDim = rl.NewColor(160, 132, 78, 255)
	markerDoor     = rl.NewColor(176, 132, 86, 255)
	markerPack     = rl.NewColor(220, 76, 70, 255)
	markerPlayer   = rl.NewColor(132, 240, 148, 255) // player facing arrow (minimap + panels Map tab)
	// crystalCyanBase is the shared charged-crystal cyan for both the editor/minimap
	// marker (markerCrystal) and the in-world gem (crystal.go pulses its R/G).
	crystalCyanBase = rl.NewColor(96, 214, 232, 255)
	// Companion crystal tints (dormant body + both edge-wire states), beside
	// crystalCyanBase so the whole gem palette tunes in one place.
	crystalDormantBody = rl.NewColor(70, 92, 110, 190)   // dormant gem body (flat slate)
	crystalEdgeCharged = rl.NewColor(210, 250, 255, 220) // charged faceted wire
	crystalEdgeDormant = rl.NewColor(110, 130, 150, 150) // dormant faceted wire
	// crystalChargedBody: charged gem body — R/G ride crystalCyanBase (lockstep),
	// blue pinned full. crystalColor pulses only R/G at render time.
	crystalChargedBody = rl.NewColor(crystalCyanBase.R, crystalCyanBase.G, 255, 235)
	// crystalChargedCore: bright near-white inner/tip glint that reads as "shiny".
	// (The old larger-than-gem "halo" aura was dropped for a real point light — see
	// crystalLightColor / collectTorches.)
	crystalChargedCore = rl.NewColor(225, 252, 255, 240)
	// markerCrystal reads as the charged cyan, clear of the chest/door/pack/start
	// swatches.
	markerCrystal = crystalCyanBase
	markerOutline = rl.NewColor(0, 0, 0, 220)
	// Debug-overlay text tints (coord heading vs in-world tile labels) — kept in the
	// palette so debug chrome tunes alongside the rest, not as one-off literals.
	debugHeadingColor = rl.NewColor(186, 240, 186, 245)
	debugLabelColor   = rl.NewColor(220, 240, 220, 245)

	// Hit-glyph clarity colors (hitglyph.go) — each attack glyph's signature hue.
	// Painters override alpha per frame; the alpha below is a placeholder.
	glyphSlashColor  = rl.NewColor(245, 248, 255, 255)
	glyphImpactColor = rl.NewColor(255, 236, 150, 255)
	glyphFrostColor  = rl.NewColor(170, 224, 255, 255)
	glyphSparkBolt   = rl.NewColor(150, 205, 255, 255)
	glyphSparkCore   = rl.NewColor(225, 242, 255, 255)
	glyphFireOuter   = rl.NewColor(255, 150, 60, 255)
	glyphFireInner   = rl.NewColor(255, 222, 130, 255)
	glyphHolyColor   = rl.NewColor(255, 232, 150, 255)
	glyphVenomColor  = rl.NewColor(150, 230, 110, 255)

	// mapTileFogColor is the dim out-of-bounds fill, shared by both map surfaces
	// (corner minimap + panels Map tab).
	mapTileFogColor = rl.NewColor(8, 10, 14, 235)

	// chestColors — chest billboard body + lid. (The interact prompt reuses
	// borderActive.)
	chestBodyColor = rl.NewColor(168, 116, 70, 255)
	chestLidColor  = rl.NewColor(196, 148, 92, 255)
	// chestInteriorColor — the dark "hole" inside an opened/looted chest (no lid).
	chestInteriorColor = rl.NewColor(26, 18, 12, 255)
	// chestMetalDark/Bright — the chest's band + lockplate tints (3D prop). Body and
	// lid MUST share them, so the literals live here beside the chest body/lid tokens.
	chestMetalDark   = rl.NewColor(140, 108, 64, 255)
	chestMetalBright = rl.NewColor(182, 148, 86, 255)

	// Wood-grain tints shared by the timber props (well, scarecrow, table, etc.).
	// Distinct from the UI woodDark/woodMid/... frame tokens (those frame glass; these
	// tint 3D meshes).
	woodPaletteWarm = rl.NewColor(110, 78, 50, 255) // warm timber brown
	woodPaletteDark = rl.NewColor(72, 52, 32, 255)  // dark grain / bark

	// Per-class accent ("slot color") — single source for every HUD/UI/log tint
	// keyed to a class (classes.go turnColor + classAccent). Tuned distinct on dark glass.
	classAccentWarrior = rl.NewColor(232, 184, 82, 255)  // gold
	classAccentCleric  = rl.NewColor(238, 236, 226, 255) // off-white
	classAccentThief   = rl.NewColor(182, 132, 236, 255) // purple
	classAccentWizard  = rl.NewColor(148, 198, 244, 255) // pale blue
	// classRobeGold is the gilt trim on party robe sprites (Cleric sash, Wizard trim).
	// (Warrior's gold is its HUD slot accent classAccentWarrior, a separate source by design.)
	classRobeGold = rl.NewColor(226, 196, 93, 255)

	// Shadow tints for text + scrims, Light → Mid → Strong → Heavy.
	shadowLight  = rl.NewColor(0, 0, 0, 160)
	shadowMid    = rl.NewColor(0, 0, 0, 180)
	shadowStrong = rl.NewColor(0, 0, 0, 200)
	shadowHeavy  = rl.NewColor(0, 0, 0, 220)
	// shadowBase is the transparent-black base callers fade per-frame for dynamic
	// shadow alpha (splash title, timing-bar icons).
	shadowBase = rl.NewColor(0, 0, 0, 0)
	// noAccent is the zero-alpha sentinel skipping drawCard's left-spine stripe.
	noAccent = rl.NewColor(0, 0, 0, 0)
)

const (
	// hudEdgePad is the canonical screen-edge margin every always-on HUD panel
	// keeps; 16 reads comfortable at 1080p.
	hudEdgePad = int32(16)
	// hudColumnGap is the vertical spacing between stacked HUD panels (smaller than
	// hudEdgePad so they feel grouped).
	hudColumnGap = int32(10)
	// hudPanelMinH is the height floor the short-window collision guard shrinks a
	// pinned pane to (below this rows stop being readable).
	hudPanelMinH = int32(160)

	// Enemy roster card (battle.go) — the top-center foe-list pane, laid out as a
	// formation grid: back rank (up to EnemyBackRowCap) on top, front rank (up to
	// EnemyFrontRowCap) below.
	rosterTopPad    = int32(16)  // inset above the first rank
	rosterBottomPad = int32(16)  // inset below the last rank
	rosterCellW     = int32(152) // per-foe cell width
	rosterCellH     = int32(60)  // per-foe cell height (name over condition, smaller fonts)
	rosterCellGap   = int32(10)  // gap between cells in a rank
	rosterGridInset = int32(16)  // card edge → cell grid
	rosterRankGap   = int32(10)  // vertical gap between the back and front ranks
	// Status-pill geometry (anchored to a cell's top-right corner, stacking left).
	rosterStatusPillW = float32(28) // status pill width
	rosterStatusPillH = float32(20) // status pill height

	// Combat HUD panes (battle.go) — bottom-left action log + bottom-right action
	// menu. Both share the hudPanelMinH floor.
	actionLogW  = int32(320)
	actionLogH  = int32(300)
	actionMenuW = int32(360)
	actionMenuH = int32(404)
	// hudContentInsetX is the left gutter from a combat pane edge to its content;
	// also the spacing system's canonical window-padding token.
	hudContentInsetX = int32(22)
	// modalContentInsetX is the canonical side gutter for modal-card content
	// (dialog/chest/victory/level-up). Aliases hudContentInsetX(22) so combat panes
	// and modals share one window-padding value.
	modalContentInsetX = hudContentInsetX
	// modalGutterWide / modalGutterTight are the two intentional deviations from
	// modalContentInsetX(22): wide(24) for big screen-relative cards that want a
	// touch more side air (dialog/level-up/info-strip), tight(20) for narrow cards
	// (chest). Named here so the family lives in one place, not re-derived per file.
	modalGutterWide  = int32(24)
	modalGutterTight = int32(20)

	// --- Spacing system (see UI_STANDARDS.md "Spacing") -----------------
	// Shared gaps so headings/rows/footers line up the same everywhere. Header→
	// body and footer gaps are font-aware via layout.go (bodyBelowHeading /
	// footerBaselineY) since they must clear a text line whose height scales.
	uiGapAfterTitle = int32(12)         // breathing space below a heading's TEXT before its body
	uiRowH          = int32(32)         // standard interactive row-plate height
	uiRowGap        = int32(10)         // vertical gap between stacked row plates
	uiRowPitch      = uiRowH + uiRowGap // row center-to-center pitch (42)
	// modalListRowH is the shared row height for the simple modal list overlays
	// (dialog choices, chest items, shop rows). Taller than uiRowH(32) — these
	// FontBody rows want a bit more air than the dense combat menus.
	modalListRowH = int32(34)
	// modalValueInsetX pulls a right-aligned value/price column in from the card's
	// right content edge (shop prices, level-up stat values).
	modalValueInsetX = int32(12)
	uiFooterMargin   = int32(14) // visual gap below a footer hint's glyphs/text to the card bottom edge
	// actionMenuHintMinH is the height floor below which the action menu drops its
	// hint footer (it would collide with rows). Above the hudPanelMinH floor.
	actionMenuHintMinH = int32(260)

	// Corner radii — small (4/3) so the frame reads as a hardwood mitre, not a
	// modern rounded tile. See UI_STANDARDS.md "Panel".
	cornerRadius      = float32(4)
	smallCornerRadius = float32(3)
	stripeWidth       = int32(3)

	// Font sizes — the FIVE permitted text sizes (UI_STANDARDS.md "Type"); anything
	// else is a bug. All downsample from the high-res atlas bake with mipmaps.
	FontTiny    = float32(17)
	FontSmall   = float32(21)
	FontBody    = float32(26)
	FontHeading = float32(36)
	FontTitle   = float32(48)

	// Letter spacing per size (wider on titles). Applied automatically via
	// canonicalSpacing(size): heading=2, title=3, smaller=1. Pass explicitly only
	// through drawTextWithShadowStyle.
	FontSpacingBody    = float32(1)
	FontSpacingHeading = float32(2)
	FontSpacingTitle   = float32(3)

	// woodFrame* are the stroke widths of the wood-panel border bands.
	woodFrameOuter = int32(2)
	woodFrameBand  = int32(3)
	woodFrameInner = int32(1)
	// cardFrameThick is the total band width; drawCard's stroke + drawCardFiligree
	// (+5) + drawCardInlay (+4) insets all derive from it so they stay aligned.
	cardFrameThick = woodFrameOuter + woodFrameBand + woodFrameInner

	// Focus-plate insets: how far the gilt selection plate bleeds past the row's
	// text origin. focusPlateInsetX/Y for shop/journal rows; menuRowInsetX/Y for
	// the pause menu's larger heading-tier rows.
	focusPlateInsetX = int32(12)
	focusPlateInsetY = int32(2)
	menuRowInsetX    = int32(18)
	menuRowInsetY    = int32(6)

	// Heading underline minimum width (so short headings still read as labelled)
	// + drawBar value/label pads.
	headingTickMinWidth = int32(28)
	barValuePadRight    = float32(10)
	barLabelPadLeft     = float32(8)

	// Canonical HP/MP gauge heights: Character tab (compact) and the use-item
	// picker row (mini). The party ribbon cards size their own gauge (partyCardBarH).
	barHeightCompact = float32(28)
	barHeightMini    = float32(18)

	// World-popup horizontal slack: pixels past the screen edge a projected popup
	// can drift before culling, so it fades cleanly instead of snapping off.
	offscreenPopupSlack = float32(200)

	// Overlay (modal card) dimensions, centralized for a future "shrink on small
	// screens" pass. Sized to content (height expands at the call site, WIDTH
	// standardized).
	overlayCardWidthSmall = int32(360) // chest modal (item list)
	// modalMinCardH floors a content-sized modal height so a short list still
	// reads as a card. Shared by the dialog + chest modals.
	modalMinCardH = int32(200)
	// The level-up modal + game-panels overlay size screen-relative
	// (drawScreenFractionScaffold), clamped by overlayCardMarginScreen.
	panelsOverlayWidthFrac  = float32(0.80)
	panelsOverlayHeightFrac = float32(0.80)
	levelUpModalWidthFrac   = float32(0.60)
	levelUpModalHeightFrac  = float32(0.85)
	// victoryWidthFrac is the spoils card's screen-relative WIDTH (height is
	// content-sized; see DrawVictorySpoils).
	victoryWidthFrac = float32(0.5)

	overlayCardMarginScreen = int32(40) // minimum margin between card and screen edges

	// Panels-overlay tab strip geometry.
	overlayTabHeight  = int32(46)
	overlayTabPadding = int32(12)

	// overlayFooterReserve is the bottom band reserved for the hint footer
	// (DrawHintBar / drawModalFooterGlyphs). Body = card minus this band minus the heading band.
	overlayFooterReserve = int32(38)
	// panelsBodyBottomReserve is the extra gap below the panels tab body (above the
	// footer band) so the body never butts the card's bottom edge.
	panelsBodyBottomReserve = int32(26)
)

// modalHeadingInsetY is the Y offset from a modal card's top edge to its heading
// baseline. Shared by drawModalScaffold and the dialog overlay's nameplate so the
// two align.
const modalHeadingInsetY = int32(14)

// modalHeadingGutterX is the heading's X inset from the card's left mitre — wider
// than the body's hudContentInsetX(22) because the engraved FontHeading title wants
// more breathing room from the wood frame than body content does.
const modalHeadingGutterX = int32(28)

// drawModalScaffold paints the shared veil + centered card + heading band for
// every modal overlay, returning the card rect. Empty heading skips the band.
func drawModalScaffold(font rl.Font, cardW, cardH int32, heading string) rl.Rectangle {
	screenW, screenH := screenSize()
	// Soft clamp so a tiny window doesn't push the card off-screen.
	if cardW > screenW-overlayCardMarginScreen {
		cardW = screenW - overlayCardMarginScreen
	}
	if cardH > screenH-overlayCardMarginScreen {
		cardH = screenH - overlayCardMarginScreen
	}
	rect := drawVeiledCard(cardW, cardH, borderSoft, borderActive, giltDim)
	if heading != "" {
		drawHeading(font, heading, int32(rect.X)+modalHeadingGutterX, int32(rect.Y)+modalHeadingInsetY, borderActive)
	}
	return rect
}

// Confirm-modal geometry — the shared shape behind DrawDoorPrompt / DrawQuitConfirm:
// a fleuron-free FontHeading title card, one centered body line, an A/B hint bar.
const (
	confirmCardW        = int32(440)
	confirmCardH        = int32(168)
	confirmHeaderInsetY = float32(22)
	confirmBodyInsetY   = float32(78)
	confirmFooterInsetY = float32(40)
)

// drawConfirmModal paints a centered yes/no confirm card: engraved title, one muted
// body line, and a controller-first hint bar. Single source for the door/quit prompts
// so their geometry can't drift apart.
func drawConfirmModal(font rl.Font, title, body string, hints []HintSeg) {
	rect, _ := drawCenteredTitleCard(font, confirmCardW, confirmCardH, title, FontHeading, confirmHeaderInsetY, false)
	cx := rect.X + rect.Width/2
	drawTextCentered(font, body, cx, rect.Y+confirmBodyInsetY, FontBody, textMuted)
	DrawHintBar(font, hints, cx, rect.Y+rect.Height-confirmFooterInsetY, FontSmall)
}

// drawVeiledCard paints the veil + centered wood-framed card + gilt filigree,
// returning the card rect. Shared by drawModalScaffold and the centered-title
// overlays. No clamp here (callers that need it clamp first).
func drawVeiledCard(cardW, cardH int32, outline, accent, filigree color.RGBA) rl.Rectangle {
	screenW, screenH := screenSize()
	cardX := centerX(cardW)
	cardY := screenH/2 - cardH/2
	drawCandleVeil(screenW, screenH)
	drawCard(cardX, cardY, cardW, cardH, surfacePrimary, outline, accent)
	drawCardFiligree(cardX, cardY, cardW, cardH, filigree)
	return rl.NewRectangle(float32(cardX), float32(cardY), float32(cardW), float32(cardH))
}

func drawCandleVeil(screenW, screenH int32) {
	rl.DrawRectangle(0, 0, screenW, screenH, surfaceVeil)
	if screenW <= 0 || screenH <= 0 {
		return
	}
	rl.DrawRectangleGradientEx(
		rl.NewRectangle(0, 0, float32(screenW), float32(screenH)),
		rl.NewColor(56, 34, 18, 52),
		rl.NewColor(0, 0, 0, 36),
		rl.NewColor(18, 22, 44, 30),
		rl.NewColor(0, 0, 0, 56),
	)
	flick := candleFlicker()
	cx := screenW / 2
	cy := screenH / 2
	// Warm candlelight pool blooming from center, breathing with the flame.
	poolR := float32(min(int(screenW), int(screenH))) * 0.66
	rl.DrawCircleGradient(cx, cy, poolR,
		fadeColor(rl.NewColor(78, 50, 24, 255), 0.11*flick),
		rl.NewColor(0, 0, 0, 0))
	// Drifting dust motes catching the candlelight (stateless, hash-seeded).
	drawDustMotes(screenW, screenH, flick)
}

// drawDustMotes scatters faint warm motes drifting down with a gentle sway.
// Stateless: position/twinkle derive from a per-mote hash + rl.GetTime().
func drawDustMotes(screenW, screenH int32, flick float32) {
	const motes = 16
	t := rl.GetTime()
	fw := float32(screenW)
	fh := float32(screenH)
	warm := rl.NewColor(255, 230, 184, 255)
	for i := 0; i < motes; i++ {
		hx := hash01(uint32(i*2654435761 + 1))
		hy := hash01(uint32(i*40503 + 7))
		fall := float32(math.Mod(t*9+float64(hy)*float64(fh), float64(fh)))
		sway := float32(math.Sin(t*0.35+float64(i)*1.7)) * 22
		mx := hx*fw + sway
		my := fall
		// Twinkle inline from the captured t (avoids a per-mote pulse() cgo call).
		tw := 0.5 + 0.5*float32(math.Sin(t*(0.25+float64(i)*0.04)*math.Pi*2))
		a := (0.06 + 0.07*tw) * flick
		rl.DrawCircleV(rl.NewVector2(mx, my), 1.2, fadeColor(warm, a))
	}
}

// BackdropTopColor is the candlelit backdrop's top gradient stop. Exported so the
// title's load-bearing ClearBackground uses the SAME value the gradient overdraws,
// instead of re-spelling the literal in the title package.
var BackdropTopColor = rl.NewColor(8, 10, 20, 255)

// DrawCandlelitBackdrop paints a full-screen candlelit background (gradient +
// radial pool + dust motes + grain) for the title screen. Exported for the title
// package.
func DrawCandlelitBackdrop(screenW, screenH int32) {
	if screenW <= 0 || screenH <= 0 {
		return
	}
	rl.DrawRectangleGradientV(0, 0, screenW, screenH,
		BackdropTopColor, rl.NewColor(22, 15, 12, 255))
	flick := candleFlicker()
	poolY := int32(float32(screenH) * 0.34)
	poolR := float32(min(int(screenW), int(screenH))) * 0.72
	rl.DrawCircleGradient(screenW/2, poolY, poolR,
		fadeColor(rl.NewColor(96, 62, 28, 255), 0.30*flick),
		rl.NewColor(0, 0, 0, 0))
	drawDustMotes(screenW, screenH, flick)
	drawHudGrain(0, 0, screenW, screenH, 0.5)
}

// drawScreenFractionScaffold sizes a modal card as a screen fraction (wFrac×hFrac),
// then defers to drawModalScaffold. Shared by the game-panels + level-up overlays.
func drawScreenFractionScaffold(font rl.Font, wFrac, hFrac float32, heading string) rl.Rectangle {
	sw, sh := screenSize()
	return drawModalScaffold(font, int32(float32(sw)*wFrac), int32(float32(sh)*hFrac), heading)
}

// drawCardFiligree paints gilt corner brackets (outer + inner L, joint/tip pips,
// a bright speculum) on a wood-framed card. Skipped under 80×80.
func drawCardFiligree(x, y, w, h int32, col color.RGBA) {
	if w < 80 || h < 80 {
		return
	}
	inset := int32(cardFrameThick + 5)
	outerArm := int32(14)
	innerArm := int32(8)
	innerInset := int32(4)
	corners := [4][2]int32{
		{x + inset, y + inset},
		{x + w - inset, y + inset},
		{x + inset, y + h - inset},
		{x + w - inset, y + h - inset},
	}
	softGilt := fadeColor(col, 0.55)
	for i, c := range corners {
		dx := int32(1)
		dy := int32(1)
		if i == 1 || i == 3 {
			dx = -1
		}
		if i == 2 || i == 3 {
			dy = -1
		}
		// Outer L — 14px arms; a 1px dark offset copy first so it reads as raised
		// cast metal, not flush paint.
		outerHX := c[0]
		if dx < 0 {
			outerHX = c[0] - outerArm
		}
		outerVY := c[1]
		if dy < 0 {
			outerVY = c[1] - outerArm
		}
		braceShadow := fadeColor(shadowHeavy, 0.70)
		rl.DrawRectangle(outerHX+1, c[1]+1, outerArm, 2, braceShadow)
		rl.DrawRectangle(c[0]+1, outerVY+1, 2, outerArm, braceShadow)
		rl.DrawRectangle(outerHX, c[1], outerArm, 2, col)
		rl.DrawRectangle(c[0], outerVY, 2, outerArm, col)
		// Inner L — shorter/thinner, offset diagonally inward.
		innerOriginX := c[0] + dx*innerInset
		innerOriginY := c[1] + dy*innerInset
		innerHX := innerOriginX
		if dx < 0 {
			innerHX = innerOriginX - innerArm
		}
		rl.DrawRectangle(innerHX, innerOriginY, innerArm, 1, softGilt)
		innerVY := innerOriginY
		if dy < 0 {
			innerVY = innerOriginY - innerArm
		}
		rl.DrawRectangle(innerOriginX, innerVY, 1, innerArm, softGilt)
		// Joint pip + bright speculum inside it.
		jointX := float32(c[0]) + float32(dx)*1
		jointY := float32(c[1]) + float32(dy)*1
		drawDiamondPip(jointX, jointY, 3, col)
		drawDiamondPip(jointX, jointY, 1, giltBright)
		// Tip pips at the far ends of the outer arms.
		drawDiamondPip(float32(c[0]+dx*outerArm), float32(c[1]), 1.5, col)
		drawDiamondPip(float32(c[0]), float32(c[1]+dy*outerArm), 1.5, col)
	}
}

// statIconDrawers dispatches each Stat to its sigil drawer. A fixed array (not a
// switch) so a new Stat forces a slot and the init below panics on a nil entry.
var statIconDrawers = [core.StatCount]func(cx, cy, r float32, col color.RGBA){
	core.StatSTR: drawStatIconSTR,
	core.StatDEX: drawStatIconDEX,
	core.StatINT: drawStatIconINT,
	core.StatWIS: drawStatIconWIS,
	core.StatVIT: drawStatIconVIT,
	core.StatSPD: drawStatIconSPD,
}

func init() {
	for s := core.Stat(0); s < core.StatCount; s++ {
		if statIconDrawers[s] == nil {
			panic("render: Stat " + strconv.Itoa(int(s)) + " has no statIconDrawers entry")
		}
	}
}

// drawStatIcon dispatches to the per-stat sigil drawer (level-up + Stats tab rows).
// Fails loud on an out-of-range Stat to match the package's other icon dispatchers
// (slotIconForKind, partyClassPresentationFor) — the init above guarantees coverage.
func drawStatIcon(s core.Stat, cx, cy, r float32, col color.RGBA) {
	if int(s) < 0 || int(s) >= len(statIconDrawers) {
		panic("render: drawStatIcon called with out-of-range Stat " + strconv.Itoa(int(s)))
	}
	statIconDrawers[s](cx, cy, r, col)
}

// STR — hammer sigil (head + haft + gilt band).
func drawStatIconSTR(cx, cy, r float32, col color.RGBA) {
	headHalfW := r * 0.85
	headH := r * 0.55
	rl.DrawRectangle(int32(cx-headHalfW), int32(cy-r), int32(headHalfW*2), int32(headH), col)
	// Head face band — brighter inner stripe reading as a steel rim.
	rl.DrawRectangle(int32(cx-headHalfW), int32(cy-r+headH-2), int32(headHalfW*2), 1, giltBright)
	// Haft.
	haftHalfW := r * 0.16
	rl.DrawRectangle(int32(cx-haftHalfW), int32(cy-r+headH), int32(haftHalfW*2), int32(r*1.35), col)
	// Pommel knob.
	rl.DrawCircleV(rl.NewVector2(cx, cy+r*0.45), haftHalfW*1.4, fadeColor(col, 0.85))
}

// DEX — NE arrow sigil (tip + shaft + fletching).
func drawStatIconDEX(cx, cy, r float32, col color.RGBA) {
	// Arrow axis NE: direction unit vector + perpendicular.
	ax, ay := sqrt2Inv, -sqrt2Inv
	px, py := sqrt2Inv, sqrt2Inv
	// Tip triangle.
	tip := rl.NewVector2(cx+ax*r, cy+ay*r)
	tipBaseW := r * 0.45
	tipBack := rl.NewVector2(cx+ax*r*0.55, cy+ay*r*0.55)
	tipL := rl.NewVector2(tipBack.X+px*tipBaseW, tipBack.Y+py*tipBaseW)
	tipR := rl.NewVector2(tipBack.X-px*tipBaseW, tipBack.Y-py*tipBaseW)
	drawTriangleCCW(tip, tipR, tipL, col)
	// Shaft — tiny discs per sample give a rotated-rect look without the primitive.
	shaftHalfW := r * 0.12
	for t := float32(-r * 0.85); t < r*0.55; t += 1 {
		px2 := cx + ax*t
		py2 := cy + ay*t
		rl.DrawCircleV(rl.NewVector2(px2, py2), shaftHalfW, col)
	}
	// Fletching V at the tail.
	tail := rl.NewVector2(cx-ax*r, cy-ay*r)
	fl1 := rl.NewVector2(tail.X+px*r*0.45, tail.Y+py*r*0.45)
	fl2 := rl.NewVector2(tail.X-px*r*0.45, tail.Y-py*r*0.45)
	drawTriangleCCW(tail, fl1, rl.NewVector2(tail.X+ax*r*0.35, tail.Y+ay*r*0.35), col)
	drawTriangleCCW(tail, rl.NewVector2(tail.X+ax*r*0.35, tail.Y+ay*r*0.35), fl2, col)
}

// INT — open-book sigil (two leaves + spine + bookmark).
func drawStatIconINT(cx, cy, r float32, col color.RGBA) {
	pageHalfW := r * 0.7
	pageH := r * 1.3
	// Spine.
	rl.DrawRectangle(int32(cx)-1, int32(cy-pageH/2), 2, int32(pageH), fadeColor(col, 0.7))
	// Left + right page.
	rl.DrawRectangle(int32(cx-pageHalfW), int32(cy-pageH/2), int32(pageHalfW)-1, int32(pageH), col)
	rl.DrawRectangle(int32(cx)+1, int32(cy-pageH/2), int32(pageHalfW)-1, int32(pageH), col)
	// Page-line hatching so the book reads as written-in.
	hatch := fadeColor(col, 0.4)
	rl.DrawRectangle(int32(cx-pageHalfW+2), int32(cy-pageH/4), int32(pageHalfW-4), 1, hatch)
	rl.DrawRectangle(int32(cx-pageHalfW+2), int32(cy+1), int32(pageHalfW-4), 1, hatch)
	rl.DrawRectangle(int32(cx+3), int32(cy-pageH/4), int32(pageHalfW-4), 1, hatch)
	rl.DrawRectangle(int32(cx+3), int32(cy+1), int32(pageHalfW-4), 1, hatch)
	// Bookmark ribbon.
	bmHalfW := float32(1)
	rl.DrawRectangle(int32(cx+pageHalfW*0.55), int32(cy-pageH/2), int32(bmHalfW*2), int32(pageH+3), giltBright)
}

// WIS — eye sigil (lens + iris + pupil + catchlight).
func drawStatIconWIS(cx, cy, r float32, col color.RGBA) {
	// Lens: two triangles forming a diamond.
	lensHalfW := r * 0.95
	lensHalfH := r * 0.55
	left := rl.NewVector2(cx-lensHalfW, cy)
	right := rl.NewVector2(cx+lensHalfW, cy)
	top := rl.NewVector2(cx, cy-lensHalfH)
	bot := rl.NewVector2(cx, cy+lensHalfH)
	drawTriangleCCW(left, top, right, col)
	drawTriangleCCW(left, right, bot, col)
	// Iris.
	rl.DrawCircleV(rl.NewVector2(cx, cy), lensHalfH*0.7, fadeColor(col, 0.55))
	// Pupil.
	rl.DrawCircleV(rl.NewVector2(cx, cy), lensHalfH*0.3, fadeColor(col, 0.25))
	// Catchlight.
	rl.DrawCircleV(rl.NewVector2(cx-lensHalfH*0.18, cy-lensHalfH*0.18), 1.4, giltBright)
}

// VIT — heart sigil (two lobes + V-point + pip).
func drawStatIconVIT(cx, cy, r float32, col color.RGBA) {
	lobeR := r * 0.42
	lobeY := cy - r*0.2
	lobeOffset := lobeR * 0.85
	rl.DrawCircleV(rl.NewVector2(cx-lobeOffset, lobeY), lobeR, col)
	rl.DrawCircleV(rl.NewVector2(cx+lobeOffset, lobeY), lobeR, col)
	// V-point down to the chin.
	leftAnchor := rl.NewVector2(cx-lobeOffset-lobeR*0.85, lobeY+lobeR*0.25)
	rightAnchor := rl.NewVector2(cx+lobeOffset+lobeR*0.85, lobeY+lobeR*0.25)
	chin := rl.NewVector2(cx, cy+r*0.95)
	drawTriangleCCW(leftAnchor, chin, rightAnchor, col)
	// Inner highlight.
	rl.DrawCircleV(rl.NewVector2(cx-lobeOffset*0.4, lobeY-lobeR*0.2), 1.4, giltBright)
}

// SPD — lightning-bolt sigil (zigzag polygon, triangle-fanned).
func drawStatIconSPD(cx, cy, r float32, col color.RGBA) {
	verts := []rl.Vector2{
		{X: cx - r*0.05, Y: cy - r},     // top spike
		{X: cx + r*0.5, Y: cy - r*0.1},  // upper right notch
		{X: cx + r*0.05, Y: cy - r*0.1}, // inner step
		{X: cx + r*0.4, Y: cy + r},      // bottom spike
		{X: cx - r*0.5, Y: cy + r*0.1},  // lower left notch
		{X: cx - r*0.05, Y: cy + r*0.1}, // inner step
	}
	// Triangle-fan from v0.
	for i := 1; i < len(verts)-1; i++ {
		drawTriangleCCW(verts[0], verts[i+1], verts[i], col)
	}
	// Gilt pip at the kink for a "live" centre.
	rl.DrawCircleV(rl.NewVector2(cx, cy), 1.4, giltBright)
}

// drawEmptyLedgerNote is the standard empty-list treatment: a dim gilt fleuron
// with flanking hairlines over a centered muted message, in the body's upper
// third. Shared so every empty state matches.
func drawEmptyLedgerNote(font rl.Font, body rl.Rectangle, text, sub string) {
	cx := body.X + body.Width/2
	ornY := body.Y + body.Height*0.26
	drawFleuron(cx, ornY, 4, fadeColor(giltDim, 0.55))
	lineW := float32(46)
	gap := float32(16)
	lineCol := fadeColor(giltDim, 0.35)
	rl.DrawRectangle(int32(cx-gap-lineW), int32(ornY), int32(lineW), 1, lineCol)
	rl.DrawRectangle(int32(cx+gap), int32(ornY), int32(lineW), 1, lineCol)
	drawDiamondPip(cx-gap-lineW, ornY, 1.5, lineCol)
	drawDiamondPip(cx+gap+lineW, ornY, 1.5, lineCol)
	drawTextCentered(font, text, cx, ornY+18, FontBody, textMuted)
	// Optional second hint line, dimmer + smaller.
	if sub != "" {
		drawTextCentered(font, sub, cx, ornY+18+FontBody+10, FontSmall, textDim)
	}
}

// starVertsBuf backs starVerts (reused, single-threaded path). Sized for the
// 5-point star; grows on demand.
var starVertsBuf = make([]rl.Vector2, 0, 10)

// starVerts returns the 2×points alternating outer/inner vertices of a star at
// (cx,cy), first vertex up (-90°). The returned slice aliases a reused buffer —
// copy it if it must outlive the next call.
func starVerts(cx, cy, outer, inner float32, points int) []rl.Vector2 {
	n := points * 2
	if cap(starVertsBuf) < n {
		starVertsBuf = make([]rl.Vector2, 0, n)
	}
	verts := starVertsBuf[:0]
	angleStart := -math.Pi / 2 // start at top
	for i := 0; i < n; i++ {
		angle := angleStart + float64(i)*math.Pi/float64(points)
		radius := outer
		if i%2 == 1 {
			radius = inner
		}
		verts = append(verts, rl.NewVector2(
			cx+float32(math.Cos(angle))*radius,
			cy+float32(math.Sin(angle))*radius,
		))
	}
	starVertsBuf = verts
	return verts
}

// drawDiamondPip paints a small filled diamond centered on (cx,cy), half-extent r.
func drawDiamondPip(cx, cy, r float32, col color.RGBA) {
	top := rl.NewVector2(cx, cy-r)
	right := rl.NewVector2(cx+r, cy)
	bottom := rl.NewVector2(cx, cy+r)
	left := rl.NewVector2(cx-r, cy)
	drawTriangleCCW(top, left, right, col)
	drawTriangleCCW(right, left, bottom, col)
}

// drawFleuron paints a gilt fleuron — a centre diamond flanked by teardrop leaves
// on the four compass points, with a bright inner pip. Sized by `r`. The pip uses
// giltBright regardless of `col` so the centre catches a highlight.
func drawFleuron(cx, cy, r float32, col color.RGBA) {
	drawDiamondPip(cx, cy, r, col)
	leafR := r * 0.85
	leafOffset := r + 2
	// East leaf.
	eTip := rl.NewVector2(cx+leafOffset+leafR, cy)
	eTop := rl.NewVector2(cx+leafOffset, cy-leafR*0.55)
	eBot := rl.NewVector2(cx+leafOffset, cy+leafR*0.55)
	drawTriangleCCW(eTip, eTop, eBot, col)
	// West leaf.
	wTip := rl.NewVector2(cx-leafOffset-leafR, cy)
	wTop := rl.NewVector2(cx-leafOffset, cy-leafR*0.55)
	wBot := rl.NewVector2(cx-leafOffset, cy+leafR*0.55)
	drawTriangleCCW(wTip, wBot, wTop, col)
	// North leaf.
	nTip := rl.NewVector2(cx, cy-leafOffset-leafR)
	nLeft := rl.NewVector2(cx-leafR*0.55, cy-leafOffset)
	nRight := rl.NewVector2(cx+leafR*0.55, cy-leafOffset)
	drawTriangleCCW(nTip, nLeft, nRight, col)
	// South leaf.
	sTip := rl.NewVector2(cx, cy+leafOffset+leafR)
	sLeft := rl.NewVector2(cx-leafR*0.55, cy+leafOffset)
	sRight := rl.NewVector2(cx+leafR*0.55, cy+leafOffset)
	drawTriangleCCW(sTip, sRight, sLeft, col)
	// Bright speculum at the heart.
	if r >= 3 {
		drawDiamondPip(cx, cy, r*0.35, giltBright)
	}
}

// drawFleuronsFlanking paints a fleuron `gap` px outside each end of a centered
// label (the ◆ label ◆ motif). leftX = label left edge, w = width, cy = midline.
func drawFleuronsFlanking(leftX, w, gap, cy, r float32, col color.RGBA) {
	drawFleuron(leftX-gap, cy, r, col)
	drawFleuron(leftX+w+gap, cy, r, col)
}

// drawPanel fills a rounded rect at a fixed pixel corner radius.
func drawPanel(x, y, w, h int32, fill color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawRectangleRounded(rect, fixedRoundnessFor(w, h, cornerRadius), 8, fill)
	drawSoftPanelBevel(x, y, w, h, false)
}

func drawPanelOutline(x, y, w, h int32, col color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawRectangleRoundedLinesEx(rect, fixedRoundnessFor(w, h, cornerRadius), 8, 1, col)
}

func drawSmallPanel(x, y, w, h int32, fill color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawRectangleRounded(rect, fixedRoundnessFor(w, h, smallCornerRadius), 6, fill)
	drawSoftPanelBevel(x, y, w, h, true)
}

func drawSoftPanelBevel(x, y, w, h int32, small bool) {
	if w < 12 || h < 10 {
		return
	}
	inset := int32(3)
	if small {
		inset = 2
	}
	iw := w - inset*2
	ih := h - inset*2
	if iw <= 0 || ih <= 0 {
		return
	}
	rl.DrawRectangleGradientV(x+inset, y+inset, iw, ih,
		fadeColor(inkPrimary, 0.08),
		fadeColor(shadowHeavy, 0.10))
}

func drawSmallPanelOutline(x, y, w, h int32, col color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawRectangleRoundedLinesEx(rect, fixedRoundnessFor(w, h, smallCornerRadius), 6, 1, col)
}

// drawIntensityGauge paints the retro-slider chrome — barTrack well, an inner gilt
// fill (caller supplies fillW + fillCol; width/color differ per surface), and the
// woodLight outline. Shared by the Combat Tuning + Retro Filters slider rows; the
// per-row adjust arrows stay at each call site (they differ in size/gap/pulse).
func drawIntensityGauge(x, y, w, h, fillW int32, fillCol color.RGBA) {
	drawSmallPanel(x, y, w, h, barTrack)
	if fillW > 0 {
		rl.DrawRectangle(x+1, y+1, fillW, h-2, fillCol)
	}
	drawSmallPanelOutline(x, y, w, h, fadeColor(woodLight, 0.6))
}

// drawGiltFocusRing paints the bold 3px gilt focus frame, corners matching the
// glass body radius. Shared by the active party card + focused skill-tree node.
func drawGiltFocusRing(rect rl.Rectangle) {
	roundness := fixedRoundnessFor(int32(rect.Width), int32(rect.Height), cornerRadius)
	rl.DrawRectangleRoundedLinesEx(rect, roundness, 8, 3, giltBright)
}

func fixedRoundnessFor(w, h int32, target float32) float32 {
	minDim := float32(min(int(w), int(h)))
	if minDim <= 0 {
		return 0
	}
	r := 2 * target / minDim
	if r > 1 {
		r = 1
	}
	return r
}

// drawAccentStripe paints a card's left-edge class rail, slightly inset. Thin
// wrapper over drawClassRail with the card inset geometry.
func drawAccentStripe(panelX, panelY, panelH int32, col color.RGBA) {
	if panelH < 16 {
		return
	}
	drawClassRail(panelX+5, panelY+8, stripeWidth, panelH-16, col)
}

// drawGiltRule paints a thin horizontal gilt separator (giltBright faded by
// alpha) — the brass-divider stamp under a heading/tab strip.
func drawGiltRule(x, y, w, h int32, alpha float32) {
	speckleHairline(x, y, w, h, fadeColor(giltBright, alpha))
}

// drawPipCappedRule paints a 1px rule from x to x+w with a diamond pip on each
// end — the divider-with-termini stamp for the action-menu header + info-strip.
func drawPipCappedRule(x, y, w int32, ruleCol color.RGBA, pipR float32, pipCol color.RGBA) {
	speckleHairline(x, y, w, 1, ruleCol)
	drawDiamondPip(float32(x), float32(y), pipR, pipCol)
	drawDiamondPip(float32(x+w), float32(y), pipR, pipCol)
}

// drawSplitRule draws a 1px rule from leftX to rightX broken by `gap` around cx
// (to seat a centre fleuron). Shared by the title + battle-splash dividers.
func drawSplitRule(leftX, rightX, cx, y, gap float32, col color.RGBA) {
	speckleHairline(int32(leftX), int32(y), int32(cx-gap-leftX), 1, col)
	speckleHairline(int32(cx+gap), int32(y), int32(rightX-(cx+gap)), 1, col)
}

// drawCard renders a wood-framed glass pane (UI_STANDARDS.md "Panel"): outer
// stroke, woodMid band, woodLight highlight, glass body. `accent` is the optional
// left-spine stripe (zero-alpha skips). `fill` = a glass token; `outline` = the
// outermost stroke (woodDark default, borderActive/Danger for state).
func drawCard(x, y, w, h int32, fill, outline, accent color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	// Glass body first, then three concentric hardwood frame strokes on top.
	drawCardDropShadow(x, y, w, h)
	drawGlassPane(x, y, w, h, fill)
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	roundness := fixedRoundnessFor(w, h, cornerRadius)
	frameThick := float32(cardFrameThick)
	rl.DrawRectangleRoundedLinesEx(rect, roundness, 8, frameThick, woodLight)
	rl.DrawRectangleRoundedLinesEx(rect, roundness, 8, float32(woodFrameOuter+woodFrameBand), woodMid)
	rl.DrawRectangleRoundedLinesEx(rect, roundness, 8, float32(woodFrameOuter), outline)
	drawCardBevel(x, y, w, h)
	drawCardInlay(x, y, w, h)
	if accent.A > 0 {
		drawAccentStripe(x, y, h, accent)
	}
}

// drawPanelCard is the standard neutral HUD panel: drawCard with surfacePrimary +
// borderSoft, no accent.
func drawPanelCard(x, y, w, h int32) {
	drawCard(x, y, w, h, surfacePrimary, borderSoft, borderSoft)
}

// speckleHairlineDrop is the max fraction a hairline pixel's alpha is dithered
// down by the painterly speckle (modest so the line still reads continuous).
const speckleHairlineDrop = float32(0.4)

// speckleAlpha scales base by a deterministic [1-drop, 1] factor hashed from the
// pixel position, so the per-pixel variance is stable frame-to-frame.
func speckleAlpha(px, py int32, base uint8) uint8 {
	h := uint32(px)*73856093 ^ uint32(py)*19349663
	h ^= h >> 13
	h *= 0x5bd1e995
	frac := float32(h&1023) / 1023 // 0..1
	return uint8(float32(base) * (1 - speckleHairlineDrop*frac))
}

// speckleHairline draws a 1px axis-aligned line (one of w/h must be 1), dithering
// alpha via speckleAlpha. Geometry stays EXACTLY straight — only alpha varies
// (opacity speckle, NOT waver). Non-1px/degenerate rects fall back to a fill.
func speckleHairline(x, y, w, h int32, col color.RGBA) {
	n, horizontal := w, true
	if h != 1 {
		n, horizontal = h, false
	}
	if (w != 1 && h != 1) || n <= 0 {
		rl.DrawRectangle(x, y, w, h, col)
		return
	}
	for i := int32(0); i < n; i++ {
		px, py := x, y
		if horizontal {
			px += i
		} else {
			py += i
		}
		c := col
		c.A = speckleAlpha(px, py, col.A)
		rl.DrawRectangle(px, py, 1, 1, c)
	}
}

// drawCardBevel carves the flat frame into raised molding: a lit top/left
// highlight + shadowed bottom/right outer edge, and (on tall panes) a recessed
// inner lip. Hairlines only; stop short of the corners so they don't fight the mitres.
func drawCardBevel(x, y, w, h int32) {
	if w < 56 || h < 34 {
		return
	}
	const cornerClear = int32(9)
	ft := cardFrameThick // inner lip sits where the frame band ends
	hx := x + cornerClear
	hw := w - cornerClear*2
	vy := y + cornerClear
	vh := h - cornerClear*2
	if hw <= 0 || vh <= 0 {
		return
	}
	hi := woodAccentBevelTop
	hiSide := woodAccentBevelSide
	lo := fadeColor(shadowHeavy, 0.62)
	loSide := fadeColor(shadowHeavy, 0.34)
	// Raised outer edge — speckled so it reads hand-laid, not mechanical.
	speckleHairline(hx, y+1, hw, 1, hi)
	speckleHairline(hx, y+h-2, hw, 1, lo)
	speckleHairline(x+1, vy, 1, vh, hiSide)
	speckleHairline(x+w-2, vy, 1, vh, loSide)
	// Recessed inner lip, only on tall panes (else the reads merge into stripes).
	if h >= 56 {
		speckleHairline(hx, y+ft, hw, 1, fadeColor(shadowHeavy, 0.42))
		speckleHairline(hx, y+h-ft-1, hw, 1, woodAccentInlayLip)
	}
}

func drawCardDropShadow(x, y, w, h int32) {
	if w < 32 || h < 24 {
		return
	}
	roundness := fixedRoundnessFor(w, h, cornerRadius)
	// Three stacked offset copies (widest+faintest first) for a soft graduated lift.
	wide := rl.NewRectangle(float32(x+10), float32(y+14), float32(w), float32(h))
	soft := rl.NewRectangle(float32(x+6), float32(y+8), float32(w), float32(h))
	near := rl.NewRectangle(float32(x+2), float32(y+3), float32(w), float32(h))
	rl.DrawRectangleRounded(wide, roundness, 8, fadeColor(shadowHeavy, 0.13))
	rl.DrawRectangleRounded(soft, roundness, 8, fadeColor(shadowHeavy, 0.22))
	rl.DrawRectangleRounded(near, roundness, 8, fadeColor(shadowStrong, 0.28))
}

// drawCardInlay adds carved cabinetry details: an inner groove, a gilt hairline,
// and brass corner pips. Skips narrow/tiny panes.
func drawCardInlay(x, y, w, h int32) {
	if w < 96 || h < 52 {
		return
	}
	inset := int32(cardFrameThick + 4)
	innerW := w - inset*2
	innerH := h - inset*2
	if innerW <= 0 || innerH <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x+inset), float32(y+inset), float32(innerW), float32(innerH))
	roundness := fixedRoundnessFor(innerW, innerH, smallCornerRadius)
	rl.DrawRectangleRoundedLinesEx(rect, roundness, 6, 1, woodInlay)

	// A partial gilt line near the BOTTOM only — a top hairline cut through panel
	// titles, so it's gone.
	lineInset := int32(18)
	if innerW > lineInset*2 {
		botY := y + h - inset - 3
		lineX := x + inset + lineInset
		lineW := innerW - lineInset*2
		speckleHairline(lineX, botY, lineW, 1, fadeColor(giltDim, 0.28))
	}

	corners := [4]rl.Vector2{
		rl.NewVector2(float32(x+inset), float32(y+inset)),
		rl.NewVector2(float32(x+w-inset), float32(y+inset)),
		rl.NewVector2(float32(x+inset), float32(y+h-inset)),
		rl.NewVector2(float32(x+w-inset), float32(y+h-inset)),
	}
	// Domed brass rivets at the inner corners.
	for _, c := range corners {
		drawBrassStud(c.X, c.Y, 2.3)
	}
}

// drawGlassPaneRect is the rl.Rectangle form of drawGlassPane.
func drawGlassPaneRect(r rl.Rectangle, fill color.RGBA) {
	drawGlassPane(int32(r.X), int32(r.Y), int32(r.Width), int32(r.Height), fill)
}

// drawGlassPane paints the frame-less translucent glass body (glassBaseWash + fill
// tint) for nested sub-panels; drawCard calls it for its own body.
func drawGlassPane(x, y, w, h int32, fill color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	// One cornerRadius for all so nested panes harmonise with their frame.
	roundness := fixedRoundnessFor(w, h, cornerRadius)
	rl.DrawRectangleRounded(rect, roundness, 8, glassBaseWash)
	rl.DrawRectangleRounded(rect, roundness, 8, fill)
	drawGlassGradientWash(x, y, w, h)
	drawGlassRelief(x, y, w, h)
}

func drawGlassGradientWash(x, y, w, h int32) {
	if w < 24 || h < 20 {
		return
	}
	inset := int32(4)
	if w < 80 || h < 44 {
		inset = 3
	}
	innerW := w - inset*2
	innerH := h - inset*2
	if innerW <= 0 || innerH <= 0 {
		return
	}
	r := rl.NewRectangle(float32(x+inset), float32(y+inset), float32(innerW), float32(innerH))
	topLeft := rl.NewColor(255, 226, 166, 20)
	topRight := rl.NewColor(255, 226, 166, 10)
	bottomLeft := rl.NewColor(0, 0, 0, 22)
	bottomRight := rl.NewColor(0, 0, 0, 42)
	rl.DrawRectangleGradientEx(r, topLeft, bottomLeft, topRight, bottomRight)
}

// ---- Skeuomorphic relief: grain, candlelight, glass depth -----------------

// hudGrainTex is the tileable grain overlay tiled over every glass body by
// drawGlassRelief. A package singleton (free-function helpers, no Resources);
// hudGrainReady guards the pre-init/headless window.
var (
	hudGrainTex   rl.Texture2D
	hudGrainReady bool
)

// hudGrainAlpha is the master "tooth" knob for the grain overlay.
const hudGrainAlpha = float32(0.85)

// drawHudGrain tiles hudGrainTex across (x,y,w,h) (WrapRepeat → one draw). No-op
// until the texture exists.
func drawHudGrain(x, y, w, h int32, alpha float32) {
	if !hudGrainReady || w <= 0 || h <= 0 {
		return
	}
	src := rl.NewRectangle(0, 0, float32(w), float32(h))
	dst := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawTexturePro(hudGrainTex, src, dst, rl.NewVector2(0, 0), 0, fadeColor(rl.White, alpha))
}

// flickerCache memoizes candleFlicker within a frame (rl.GetTime() is frame-
// constant, but ornaments ask for it dozens of times). Keyed on time so it
// refreshes next frame.
var flickerCache struct {
	t      float64
	value  float32
	primed bool
}

// candleFlicker returns a slow organic ~0.86..1.0 multiplier (three summed sines)
// to ride gilt-ornament brightness for a "by candlelight" shimmer.
func candleFlicker() float32 {
	t := rl.GetTime()
	if flickerCache.primed && t == flickerCache.t {
		return flickerCache.value
	}
	n := 0.5*math.Sin(t*3.1) + 0.3*math.Sin(t*7.7+1.3) + 0.2*math.Sin(t*17.3+2.6)
	v := float32(0.93 + 0.07*n)
	flickerCache.t, flickerCache.value, flickerCache.primed = t, v, true
	return v
}

// drawGlassRelief lays the material grain over a glass body (sheen comes from
// drawGlassGradientWash, lift from drawCardDropShadow). Called at the tail of
// drawGlassPane so every glass surface gains it.
func drawGlassRelief(x, y, w, h int32) {
	if w < 22 || h < 16 {
		return
	}
	inset := int32(3)
	iw := w - inset*2
	ih := h - inset*2
	if iw <= 0 || ih <= 0 {
		return
	}
	drawHudGrain(x+inset, y+inset, iw, ih, hudGrainAlpha)
}

// drawBrassStud paints a domed rivet (dark seat, gilt dome, candle-modulated
// upper-left speculum).
func drawBrassStud(cx, cy, r float32) {
	rl.DrawCircleV(rl.NewVector2(cx, cy), r+1, fadeColor(woodDark, 0.9))
	rl.DrawCircleV(rl.NewVector2(cx, cy), r, giltDim)
	rl.DrawCircleV(rl.NewVector2(cx-r*0.32, cy-r*0.32), r*0.42,
		fadeColor(giltBright, 0.92*candleFlicker()))
}

// drawFocusableRow paints a selectable list row (glass body, warm-tinting + gilt
// outline when focused). The shared "cursored row" look.
func drawFocusableRow(rect rl.Rectangle, focused bool) {
	bg := fadeColor(glassDeep, 0.55)
	if focused {
		bg = fadeColor(glassWarm, 0.85)
	}
	drawGlassPaneRect(rect, bg)
	if !focused {
		return
	}
	ix, iy := int32(rect.X), int32(rect.Y)
	ih := int32(rect.Height)
	flick := candleFlicker()
	// Gilt selection frame, breathing with the flame, plus a leading gilt spine.
	rl.DrawRectangleLinesEx(rect, 2, fadeColor(giltBright, 0.72+0.28*flick))
	if ih > 8 {
		rl.DrawRectangle(ix+2, iy+3, 2, ih-6, fadeColor(giltBright, 0.8*flick))
	}
}

// drawSelectionHalo paints the live-selection emphasis: a solid inner ring + a
// wider pulsing outer ring. pulseV is the caller's breathing sample; small picks
// the compact outline.
func drawSelectionHalo(x, y, w, h int32, tint color.RGBA, pulseV float32, small bool) {
	outline := drawPanelOutline
	if small {
		outline = drawSmallPanelOutline
	}
	outline(x, y, w, h, tint)
	outline(x-3, y-3, w+6, h+6, fadeColor(tint, 0.30+0.55*pulseV))
}

// drawPaneDropShadow stamps the cheap offset drop shadow under a selectable pane.
func drawPaneDropShadow(r rl.Rectangle) {
	rl.DrawRectangle(int32(r.X+2), int32(r.Y+3), int32(r.Width), int32(r.Height), fadeColor(shadowHeavy, 0.20))
}

// drawPanelHeading paints a FontHeading title (engraved) with the wood-accent
// underline. `accent` is the underline color (woodAccent resting, borderActive/
// Danger for state); text is always inkPrimary.
func drawPanelHeading(font rl.Font, text string, x, y float32, accent color.RGBA) {
	drawEngravedText(font, text, x, y, FontHeading, inkPrimary)
	measure := measurePanelHeading(font, text)
	tickW := int32(measure.X)
	if tickW < headingTickMinWidth {
		tickW = headingTickMinWidth
	}
	ruleY := int32(y + measure.Y + 3)
	ruleX := int32(x)
	ruleW := tickW + 18
	flick := candleFlicker()
	rl.DrawRectangle(ruleX+6, ruleY, ruleW-12, 2, fadeColor(accent, 0.85))
	rl.DrawRectangle(ruleX+18, ruleY+4, ruleW-36, 1, fadeColor(accent, 0.35))
	// Brass speculum: a short glint on the left of the rule, candle-breathing.
	if ruleW > 28 {
		rl.DrawRectangle(ruleX+6, ruleY, 12, 1, fadeColor(giltBright, 0.6*flick))
	}
	drawDiamondPip(float32(ruleX+2), float32(ruleY+1), 2.4, fadeColor(accent, 0.85))
	drawDiamondPip(float32(ruleX+ruleW-2), float32(ruleY+1), 2.4, fadeColor(accent, 0.85))
	drawFleuron(float32(ruleX+ruleW+8), float32(ruleY+1), 3.2, fadeColor(accent, 0.65*flick))
}

// measureKey identifies a cached measurement: the string + the size/spacing it
// was shaped at (the same text can be measured at different sizes).
type measureKey struct {
	text          string
	size, spacing float32
}

// measureCache memoizes rl.MeasureTextEx (a cgo round-trip), flushing when the
// font atlas changes (font.Texture.ID shifts). Zero value ready to use.
type measureCache struct {
	entries map[measureKey]rl.Vector2
	fontID  uint32
}

func (c *measureCache) measure(font rl.Font, text string, size, spacing float32) rl.Vector2 {
	if c.entries == nil || font.Texture.ID != c.fontID {
		c.entries = make(map[measureKey]rl.Vector2, 32)
		c.fontID = font.Texture.ID
	}
	key := measureKey{text: text, size: size, spacing: spacing}
	if v, ok := c.entries[key]; ok {
		return v
	}
	// measureRichText so a string with a procedural symbol (richtext.go) measures
	// to its real width; symbol-free strings fall through to rl.MeasureTextEx.
	v := measureRichText(font, text, size, spacing)
	c.entries[key] = v
	return v
}

// qualityPopupMeasureCache / damagePopupMeasureCache back the throbbing combat
// popups: they measure at the FIXED base size and scale the result, so the
// size-keyed cache stays useful.
var (
	qualityPopupMeasureCache measureCache
	damagePopupMeasureCache  measureCache
)

// panelHeadingMeasureCache backs drawPanelHeading (every visible HUD panel/frame).
var panelHeadingMeasureCache measureCache

func measurePanelHeading(font rl.Font, text string) rl.Vector2 {
	return panelHeadingMeasureCache.measure(font, text, FontHeading, FontSpacingHeading)
}

// pulse oscillates 0..1 at the given frequency in Hz. Reads the frame-fixed clock
// (BeginFrame) so the many per-card / per-row pulse callers share one time sample
// instead of each making a GetTime cgo call.
func pulse(speed float64) float32 {
	return 0.5 + 0.5*float32(math.Sin(frameTime()*speed*math.Pi*2))
}

// rowSheenPeriod is the seconds for one sheen sweep across a selected row (slow,
// drifting — it doesn't "scan").
const rowSheenPeriod = 3.8

// drawRowSheen sweeps a soft gilt band across a selection plate, scissor-clipped
// to the rect and wall-clock-driven (every row shares one light source). Skipped
// on tiny rows.
func drawRowSheen(r rl.Rectangle, flick float32) {
	if r.Width < 48 || r.Height < 12 {
		return
	}
	band := r.Width * 0.30
	if band < 36 {
		band = 36
	}
	if band > 110 {
		band = 110
	}
	// Sweep phase, starting/ending fully off-edge so there's a beat between passes.
	_, t := math.Modf(frameTime() / rowSheenPeriod)
	x := r.X - band + float32(t)*(r.Width+2*band)
	peak := fadeColor(giltBright, 0.13*flick)
	clear := fadeColor(giltBright, 0)
	rl.BeginScissorMode(int32(r.X), int32(r.Y), int32(r.Width), int32(r.Height))
	rl.DrawRectangleGradientH(int32(x), int32(r.Y), int32(band/2), int32(r.Height), clear, peak)
	rl.DrawRectangleGradientH(int32(x+band/2), int32(r.Y), int32(band/2), int32(r.Height), peak, clear)
	rl.EndScissorMode()
}

// pulseActiveActor / pulseHalo / pulseFlicker are the canonical breathing curves
// (UI_STANDARDS.md "Pulse"), the single source for the active-actor frame, the
// selection halo, and the status flicker. Each re-expresses base+amp·sin(t·π·f)
// via pulse(speed=f/2):
//
//	active-actor: 0.70 + 0.30·sin(t·π·1.4)
//	halo:         0.60 + 0.40·sin(t·π·2.0)
//	flicker:      0.65 + 0.35·sin(t·π·2.6)
func pulseActiveActor() float32 { return 0.40 + 0.60*pulse(0.7) }
func pulseHalo() float32        { return 0.20 + 0.80*pulse(1.0) }
func pulseFlicker() float32     { return 0.30 + 0.70*pulse(1.3) }

// fadeColor returns col scaled by alpha multiplier in 0..1.
func fadeColor(col color.RGBA, alpha float32) color.RGBA {
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	col.A = uint8(float32(col.A) * alpha)
	return col
}

// colorWithAlpha replaces col's alpha with byteAlpha (0-255) — the "exact alpha"
// form, vs fadeColor which multiplies the existing alpha.
func colorWithAlpha(col color.RGBA, byteAlpha uint8) color.RGBA {
	col.A = byteAlpha
	return col
}

// hpFillColor selects a tier color based on remaining HP percent.
func hpFillColor(value, maxValue int) color.RGBA {
	if maxValue <= 0 {
		return barHPLow
	}
	p := float32(value) / float32(maxValue)
	switch {
	case p > 0.6:
		return barHPHigh
	case p > 0.3:
		return barHPMid
	default:
		return barHPLow
	}
}

// barLabelMeasureCache backs the bar labels ("HP"/"MP"); barValueMeasureCache the
// "10/20" value strings. drawBar runs ~16×/frame.
var barLabelMeasureCache measureCache
var barValueMeasureCache measureCache

func measureBarLabel(font rl.Font, label string) rl.Vector2 {
	return barLabelMeasureCache.measure(font, label, FontTiny, 1)
}

func measureBarValue(font rl.Font, valText string) rl.Vector2 {
	return barValueMeasureCache.measure(font, valText, FontSmall, 1)
}

// clampBarPct clips a fill fraction to [0,1] so over/underflow can't draw outside
// the track.
func clampBarPct(pct float32) float32 {
	return core.Clamp(pct, 0, 1)
}

// drawBar renders the static gauge (track + fill + outline + label/value). No
// damage ghost or heartbeat — dashboard surfaces use this; live gauges use
// drawBarLive.
func drawBar(font rl.Font, x, y, width, height float32, label string, value, maxValue int, fill color.RGBA, muted bool) {
	mv := maxValue
	if mv <= 0 {
		mv = 1
	}
	drawBarState(font, x, y, width, height, label, clampBarPct(float32(value)/float32(mv)), formatBarValue(value, maxValue), fill, muted, -1, false, false)
}

// drawBarFraction draws a gauge at a CONTINUOUS fill with a caller-supplied label
// + value (animated bars like the victory XP gauge). No ghost/heartbeat.
func drawBarFraction(font rl.Font, x, y, width, height float32, label, valText string, frac float32, fill color.RGBA, muted bool) {
	// gradientFill: fills faint→rich and gains opacity toward full (building read).
	drawBarState(font, x, y, width, height, label, frac, valText, fill, muted, -1, false, true)
}

// drawBarLive is drawBar plus living-gauge treatments, keyed by a stable identity
// string (e.g. "hp:Warrior"): a damage ghost marking the just-lost slice
// (barghost.go), and a low-value heartbeat (fill breathes + value reddens at ≤¼).
// HP gauges use both; MP stays static; muted gauges suppress both.
func drawBarLive(font rl.Font, key string, x, y, width, height float32, label string, value, maxValue int, fill color.RGBA, muted bool) {
	if maxValue <= 0 {
		maxValue = 1
	}
	pct := clampBarPct(float32(value) / float32(maxValue))
	ghost := float32(-1)
	if !muted {
		ghost = ghostPctFor(key, pct)
	}
	drawBarState(font, x, y, width, height, label, pct, formatBarValue(value, maxValue), fill, muted, ghost, !muted, false)
}

// drawBarState is the shared gauge body. ghostPct >= 0 draws the trailing damage
// segment; heartbeat enables the low-value breathing.
func drawBarState(font rl.Font, x, y, width, height float32, label string, pct float32, valText string, fill color.RGBA, muted bool, ghostPct float32, heartbeat, gradientFill bool) {
	pct = clampBarPct(pct)
	track := barTrack
	outline := borderDim
	if muted {
		fill = barMutedFill
	}
	// Low-value heartbeat: fill breathes at the status-flicker rate (live gauges
	// only).
	lowPulse := heartbeat && pct > 0 && pct <= 0.25
	if lowPulse {
		fill = fadeColor(fill, 0.70+0.30*pulseFlicker())
	}
	ix, iy, iw, ih := int32(x), int32(y), int32(width), int32(height)
	drawGaugeWell(ix, iy, iw, ih)
	drawSmallPanel(ix, iy, iw, ih, track)
	// Trailing damage ghost UNDER the live fill, from the edge out to the held
	// level. Translucent so it reads as afterimage, not a second fill.
	if ghostPct > pct {
		gp := clampBarPct(ghostPct)
		ghostW := int32(float32(iw-2) * gp)
		if ghostW > 0 {
			drawSmallPanel(ix+1, iy+1, ghostW, ih-2, fadeColor(barGhostHot, 0.80))
		}
	}
	if pct > 0 {
		fillW := int32(float32(iw-2) * pct)
		if fillW > 0 {
			if gradientFill && !muted {
				// Faint→rich left-to-right, whole fill gaining opacity toward full.
				lead := fadeColor(fill, core.Lerp(0.45, 1.0, pct))
				rl.DrawRectangleGradientH(ix+1, iy+1, fillW, ih-2, fadeColor(fill, 0.32), lead)
			} else {
				drawSmallPanel(ix+1, iy+1, fillW, ih-2, fill)
			}
			drawGaugeFillDepth(ix+1, iy+1, fillW, ih-2, muted)
			// Liquid meniscus — bright hairline on the leading edge (glints on
			// motion). Skipped at the extremes and on muted gauges.
			if !muted && pct < 1 && fillW >= 6 && ih > 8 {
				menX := ix + 1 + fillW - 1
				menCol := fadeColor(core.MixColor(fill, inkPrimary, 0.65), 0.85)
				rl.DrawRectangle(menX, iy+3, 2, ih-6, menCol)
			}
		}
	}
	drawSmallPanelOutline(ix, iy, iw, ih, outline)
	drawGaugeBezel(ix, iy, iw, ih, muted)

	// Tape-measure ticks — inward triangular notches at 25/50/75% (50% deeper).
	// Skipped on tiny bars so compact surfaces don't look busy.
	if iw >= 80 && ih >= 12 && !muted {
		tickCol := woodAccentIcon
		ticks := [3]struct {
			t     float32
			depth float32
			width float32
		}{
			{t: 0.25, depth: 2.5, width: 3},
			{t: 0.5, depth: 4, width: 4},
			{t: 0.75, depth: 2.5, width: 3},
		}
		for _, tk := range ticks {
			tx := float32(ix) + 1 + float32(iw-2)*tk.t
			// Top notch (points down).
			topApex := rl.NewVector2(tx, float32(iy)+tk.depth)
			topL := rl.NewVector2(tx-tk.width/2, float32(iy))
			topR := rl.NewVector2(tx+tk.width/2, float32(iy))
			drawTriangleCCW(topL, topApex, topR, tickCol)
			// Bottom notch (points up).
			botApex := rl.NewVector2(tx, float32(iy+ih)-tk.depth)
			botL := rl.NewVector2(tx-tk.width/2, float32(iy+ih))
			botR := rl.NewVector2(tx+tk.width/2, float32(iy+ih))
			drawTriangleCCW(botL, botR, botApex, tickCol)
		}
	}

	// Bar labels — always FontTiny (UI_STANDARDS.md); the value text reads at a
	// glance. Cream + heavy shadow so the tag pops on any fill.
	labelSize := FontTiny
	labelColor := inkPrimary
	if muted {
		labelColor = textDim
	}
	labelMeasure := measureBarLabel(font, label)
	labelY := y + (float32(ih)-labelMeasure.Y)/2 - 1
	labelX := x + barLabelPadLeft
	drawBarText(font, label, labelX, labelY, labelSize, labelColor)

	if valText != "" {
		// Value text always FontSmall (UI_STANDARDS.md). Bright, faded when muted.
		valSize := FontSmall
		valColor := textPrimary
		if muted {
			valColor = textDim
		}
		// Critical gauge: the number turns danger-red with the breathing fill.
		if lowPulse {
			valColor = barHPLow
		}
		valMeasure := measureBarValue(font, valText)
		valY := y + (float32(ih)-valMeasure.Y)/2 - 1
		valX := x + width - valMeasure.X - barValuePadRight
		drawBarText(font, valText, valX, valY, valSize, valColor)
	}
}

// drawBarText paints a gauge label/value with a two-layer drop shadow (+2 then +1,
// heavier than drawTextWithShadow's single drop) so it reads on any fill. Plain text
// at spacing 1 — local to the gauge, no rich-glyph or canonical-spacing path.
func drawBarText(font rl.Font, text string, x, y, size float32, col color.RGBA) {
	rl.DrawTextEx(font, text, rl.NewVector2(x+2, y+2), size, 1, shadowHeavy)
	rl.DrawTextEx(font, text, rl.NewVector2(x+1, y+1), size, 1, shadowHeavy)
	rl.DrawTextEx(font, text, rl.NewVector2(x, y), size, 1, col)
}

func drawGaugeWell(x, y, w, h int32) {
	if w <= 0 || h <= 0 {
		return
	}
	rl.DrawRectangle(x+2, y+2, w, h, fadeColor(shadowHeavy, 0.26))
	rl.DrawRectangle(x+1, y+1, w, h, fadeColor(woodDark, 0.36))
}

func drawGaugeFillDepth(x, y, w, h int32, muted bool) {
	if w <= 2 || h <= 4 {
		return
	}
	hi := fadeColor(inkPrimary, 0.18)
	lo := fadeColor(shadowHeavy, 0.20)
	if muted {
		hi = fadeColor(inkDim, 0.12)
		lo = fadeColor(shadowHeavy, 0.16)
	}
	if w > 4 && h > 5 {
		rl.DrawRectangleGradientV(x+2, y+2, w-4, h-4, hi, lo)
		// Glass-tube specular cap — bright hairline on top of the fill.
		spec := fadeColor(inkPrimary, 0.34)
		if muted {
			spec = fadeColor(inkDim, 0.16)
		}
		rl.DrawRectangle(x+2, y+2, w-4, 1, spec)
	}
}

func drawGaugeBezel(x, y, w, h int32, muted bool) {
	if w < 28 || h < 10 {
		return
	}
	top := fadeColor(woodLight, 0.70)
	bot := fadeColor(woodDark, 0.78)
	if muted {
		top = fadeColor(woodMid, 0.35)
		bot = fadeColor(woodDark, 0.55)
	}
	rl.DrawRectangle(x+3, y, w-6, 1, top)
	rl.DrawRectangle(x+3, y+h-1, w-6, 1, bot)
	rl.DrawRectangle(x, y+3, 1, h-6, bot)
	rl.DrawRectangle(x+w-1, y+3, 1, h-6, top)
	if w >= 80 {
		capCol := fadeColor(giltDim, 0.55)
		drawDiamondPip(float32(x+6), float32(y+h/2), 2, capCol)
		drawDiamondPip(float32(x+w-6), float32(y+h/2), 2, capCol)
	}
}

// drawClassRail paints THE shared embellished 3-D class-color rail (recessed
// channel + color core + beveled edges + gilt end pips). The one per-character
// tick/stripe look across the UI. Callers pass geometry; `col` carries the alpha.
func drawClassRail(x, y, w, h int32, col color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	chan_ := rl.NewRectangle(float32(x-1), float32(y-2), float32(w+2), float32(h+4))
	rl.DrawRectangleRounded(chan_, 0.7, 5, fadeColor(shadowHeavy, 0.55))
	core := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawRectangleRounded(core, 0.7, 5, col)
	rl.DrawRectangle(x, y+1, 1, h-2, fadeColor(rl.White, 0.32))        // lit left edge
	rl.DrawRectangle(x+w-1, y+1, 1, h-2, fadeColor(shadowHeavy, 0.45)) // shadowed right edge
	cx := float32(x) + float32(w)/2
	g := candleFlicker() // brass terminals breathe with the candlelight
	drawDiamondPip(cx, float32(y-1), 2.3, fadeColor(giltBright, 0.85*g))
	drawDiamondPip(cx, float32(y+h+1), 2.1, fadeColor(giltDim, 0.7*g))
}

// barValueLabelCache memoizes the "value/max" readout per pair so the ~8
// per-frame bars don't each allocate a fresh string. Mirrors enemyHPLabelCache.
var barValueLabelCache = map[[2]int]string{}

func formatBarValue(value, maxValue int) string {
	k := [2]int{value, maxValue}
	if s, ok := barValueLabelCache[k]; ok {
		return s
	}
	// strconv concat over fmt.Sprintf (which allocates ~3×); only on a cache miss.
	s := strconv.Itoa(value) + "/" + strconv.Itoa(maxValue)
	barValueLabelCache[k] = s
	return s
}

// drawTriangleCCW wraps rl.DrawTriangle with the "CCW in screen-Y-down" winding
// contract: some drivers silently cull CW-wound 2D triangles. CCW here means the
// (b-a)×(c-b) cross product is NEGATIVE (screen Y points down).
func drawTriangleCCW(a, b, c rl.Vector2, col color.RGBA) {
	rl.DrawTriangle(a, b, c, col)
}

// drawArrowMarker paints a small chevron: base at `center` perpendicular to the
// direction, apex at center+(tipDx,tipDy), base width 2*halfWidth. Via
// drawTriangleCCW for the winding contract.
func drawArrowMarker(center rl.Vector2, tipDx, tipDy, halfWidth float32, col color.RGBA) {
	tipLen := float32(math.Sqrt(float64(tipDx*tipDx + tipDy*tipDy)))
	if tipLen == 0 {
		return
	}
	px := -tipDy / tipLen * halfWidth
	py := tipDx / tipLen * halfWidth
	// Apex → base1 → base2 is CCW in screen-Y-down (px/py is the dir rotated +90°).
	drawTriangleCCW(
		rl.NewVector2(center.X+tipDx, center.Y+tipDy),
		rl.NewVector2(center.X-px, center.Y-py),
		rl.NewVector2(center.X+px, center.Y+py),
		col,
	)
}

// canonicalSpacing maps a font size to its standard letter spacing
// (UI_STANDARDS.md "Type"): Tiny/Small/Body=1, Heading=2, Title+=3. The single
// source the plain text helpers consult so draw + measure can't disagree on width.
func canonicalSpacing(size float32) float32 {
	switch {
	case size >= FontTitle:
		return FontSpacingTitle
	case size >= FontHeading:
		return FontSpacingHeading
	default:
		return FontSpacingBody
	}
}

// drawTextWithShadow paints text twice: a (1,1) shadowStrong drop, then the
// color. Letter spacing follows canonicalSpacing(size). Heavier shadows go through
// drawTextWithShadowStyle.
func drawTextWithShadow(font rl.Font, text string, x, y, size float32, col color.RGBA) {
	drawTextWithShadowStyle(font, text, x, y, size, canonicalSpacing(size), col, shadowStrong, 1, 1)
}

// rowTextColor picks the list-row label tone: disabled→disabledCol, else
// focused→textPrimary, else textMuted.
func rowTextColor(focused, disabled bool, disabledCol color.RGBA) color.RGBA {
	switch {
	case disabled:
		return disabledCol
	case focused:
		return textPrimary
	}
	return textMuted
}

// accentIfPositive tints a balance readout: accent when there's something to spend
// or invest, textMuted at zero. Shared by the SkillPoints + tree-ratio headers.
func accentIfPositive(n int, accent color.RGBA) color.RGBA {
	if n > 0 {
		return accent
	}
	return textMuted
}

// tabLabelMeasurer returns the FontBody label-width closure drawTextTabStrip
// expects, backed by the given cache.
func tabLabelMeasurer(cache *measureCache, font rl.Font) func(string) float32 {
	return func(s string) float32 { return cache.measure(font, s, FontBody, FontSpacingBody).X }
}

// drawTextTabStrip paints a left-anchored row of FontBody text tabs: active in
// activeCol (inkAccent underline when set), rest textMuted, advancing by measured
// width + gap. Returns the x past the last tab. Shared by the shop + Journal
// sub-tabs (the panels overlay's main strip is a different visual).
func drawTextTabStrip(font rl.Font, x, y float32, count, active int, label func(int) string, measure func(string) float32, activeCol color.RGBA, gap float32, underline bool) float32 {
	cursorX := x
	for i := 0; i < count; i++ {
		txt := label(i)
		col := textMuted
		if i == active {
			col = activeCol
		}
		drawTextWithShadow(font, txt, cursorX, y, FontBody, col)
		w := measure(txt)
		if underline && i == active {
			rl.DrawRectangle(int32(cursorX), int32(y+FontBody+2), int32(w), 2, inkAccent)
		}
		cursorX += w + gap
	}
	return cursorX
}

// engravedMeasureCache backs drawEngravedText's band math.
var engravedMeasureCache measureCache

// drawEngravedText paints large text as top-lit engraved metal in four passes:
// shadow → body → a scissor-clipped bright top band → a scissor-clipped deep
// baseline band (bands re-draw the same string so the gradient rides the glyphs).
// Spacing follows canonicalSpacing(size). Heading tier and up only.
func drawEngravedText(font rl.Font, text string, x, y, size float32, base color.RGBA) {
	drawEngravedTextSpaced(font, text, x, y, size, canonicalSpacing(size), base)
}

// drawEngravedTextSpaced is the explicit-spacing core of drawEngravedText, for
// surfaces whose tracking is load-bearing. Pair with a MeasureTextEx at the same
// spacing.
func drawEngravedTextSpaced(font rl.Font, text string, x, y, size, spacing float32, base color.RGBA) {
	m := engravedMeasureCache.measure(font, text, size, spacing)
	// drawRichTextKnown so symbol-bearing labels get the procedural glyph in every
	// pass; the has-symbol scan runs once (this draws four times).
	hasSym := containsSymGlyph(text)
	drawRichTextKnown(font, text, hasSym, x+2, y+2, size, spacing, shadowHeavy)
	drawRichTextKnown(font, text, hasSym, x, y, size, spacing, base)
	// Band rects pad ±2px so antialiased glyph edges aren't shaved at the scissor.
	lit := core.MixColor(base, inkPrimary, 0.55)
	lit.A = base.A
	rl.BeginScissorMode(int32(x-2), int32(y), int32(m.X+4), int32(m.Y*0.45))
	drawRichTextKnown(font, text, hasSym, x, y, size, spacing, lit)
	rl.EndScissorMode()
	deep := core.MixColor(base, rl.NewColor(0, 0, 0, base.A), 0.34)
	deep.A = base.A
	deepTop := y + m.Y*0.72
	rl.BeginScissorMode(int32(x-2), int32(deepTop), int32(m.X+4), int32(m.Y-m.Y*0.72+2))
	drawRichTextKnown(font, text, hasSym, x, y, size, spacing, deep)
	rl.EndScissorMode()
}

// drawTextWithShadowStyle is the parametric form of drawTextWithShadow: shadowCol
// + offX/offY pick the drop, `spacing` the letter spacing. Use when an ad-hoc
// shadow/offset/spacing is load-bearing; else use drawTextWithShadow.
func drawTextWithShadowStyle(font rl.Font, text string, x, y, size, spacing float32, col, shadowCol color.RGBA, offX, offY float32) {
	// drawRichTextKnown so missing-font symbols draw procedurally; the scan runs
	// once and is shared by both passes (symbol-free strings take the fast path).
	hasSym := containsSymGlyph(text)
	drawRichTextKnown(font, text, hasSym, x+offX, y+offY, size, spacing, shadowCol)
	drawRichTextKnown(font, text, hasSym, x, y, size, spacing, col)
}

// drawHeading is the legacy int32-coord alias for drawPanelHeading.
func drawHeading(font rl.Font, text string, x, y int32, accent color.RGBA) {
	drawPanelHeading(font, text, float32(x), float32(y), accent)
}
