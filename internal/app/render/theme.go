package render

import (
	"image/color"
	"math"
	"strconv"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// sqrt2Inv is 1/√2 — the unit-diagonal component shared by every glyph
// drawn on a 45° axis (DEX arrow, warrior cross-swords, skill icons, the
// compass rose). One package-level const instead of a per-function local
// in four draw helpers.
const sqrt2Inv = float32(0.7071)

// tau is one full turn in radians (2π) — the magnitude a "spread phase / angle
// evenly over a full circle" computation multiplies a normalized [0,1) value
// by. Named so the full-turn constant isn't a bare 6.2831853 / 2*math.Pi
// scattered across the torch-phase and particle-angle code.
const tau = 6.2831853

// hashPhase maps a 32-bit hash to a stable phase angle in [0, tau): the low 16
// bits scaled across a full turn. Used to give each torch flame (and any other
// hash-seeded oscillator) its own fixed starting phase so a field of them
// flickers out of sync. Replaces the verbatim
// float32(h&0xFFFF)/65535.0*6.2831853 idiom.
func hashPhase(h uint32) float32 {
	return float32(h&0xFFFF) / 65535.0 * tau
}

// paletteSaturationCut pulls every BRIGHT accent token a fraction of the way
// toward its own luminance gray, taming the palette toward the muted
// "library" look (parchment + waxed wood, not neon). It's applied via mute()
// to the saturated accents only — the HP/MP/enemy bars, status pills + their
// outlines, the turn-order enemy red, the timing-bar accents, and the
// sequence pass/fail greens. The earthy base tokens (glass / wood / ink /
// veil) are already low-saturation and are NOT routed through mute(), so the
// frame-and-parchment identity is preserved while the colorful bits calm
// down. One knob: raise toward 1 for grayer, lower toward 0 for punchier.
const paletteSaturationCut = 0.30

// mapRGB applies f to each of c's R/G/B channels independently, preserving
// alpha. The single seam for the "transform RGB, keep A" pattern that the
// color derivers (mute below, shadeColor, adjust) each used to re-spell. f
// owns any clamping/rounding so each caller keeps its exact prior math.
func mapRGB(c rl.Color, f func(uint8) uint8) rl.Color {
	return rl.NewColor(f(c.R), f(c.G), f(c.B), c.A)
}

// mute desaturates c toward its perceptual-luminance gray by
// paletteSaturationCut, preserving alpha. Used on the bright accent tokens in
// the palette below so the whole accent set tones down from a single knob.
func mute(c rl.Color) rl.Color {
	lum := float32(c.R)*0.30 + float32(c.G)*0.59 + float32(c.B)*0.11
	return mapRGB(c, func(v uint8) uint8 {
		return uint8(float32(v) + (lum-float32(v))*paletteSaturationCut)
	})
}

// The Library palette. See UI_STANDARDS.md for the full rationale and
// the rule "no new rl.NewColor literals for any surface that already
// has a token." Every persistent HUD pane is dark glass framed in
// hardwood; every selection highlight is gilt.
//
// Two parallel naming families exist for historical reasons:
//   - The UI_STANDARDS.md tokens (`glassDeep` / `woodMid` /
//     `inkPrimary` / etc.) are the canonical names — use these in
//     new code.
//   - Older semantic aliases (`surfacePrimary` / `borderSoft` /
//     `textPrimary` / etc., grouped below) resolve to the same
//     RGB values but predate the library aesthetic doc. They're
//     treated as first-class — neither set is "deprecated" —
//     because the codebase reads cleanly with either, and a full
//     rename would touch 200+ call sites without changing pixels.
//     New uses are fine either way; pick the name that reads best
//     in context (e.g. `textPrimary` for body copy, `inkPrimary`
//     when the parchment metaphor is load-bearing).
var (
	// ----- Glass surfaces (panel fills) -----
	// Two-layer composition: drawCard paints `glassBaseWash` (a
	// dark wash that anchors the pane as dark glass) first, then
	// the glass tint over it. Both layers are translucent — the
	// world composes through both. Cumulative effective opacity is
	// roughly 1 - (1-wash)(1-tint), which lands the pane near
	// 55-62 % apparent opacity at the alphas below — thinned from an
	// earlier ~70 % so the dungeon genuinely glows through every pane
	// (the BG-era "UI floats over the world" read). The mandatory text
	// drop shadows are what keep parchment ink legible at this
	// thinness; if a future pass thins further, check the action log
	// over a lit floor first.
	glassBaseWash = rl.NewColor(8, 6, 10, 88)
	glassDeep     = rl.NewColor(14, 12, 18, 100)
	glassMid      = rl.NewColor(22, 18, 24, 84)
	glassWarm     = rl.NewColor(28, 22, 16, 118)
	glassDanger   = rl.NewColor(36, 16, 18, 112)
	veil          = rl.NewColor(0, 0, 0, 130)
	// surfaceCardBackdrop is an OPAQUE dark panel fill (glass-family tone at
	// full alpha) for modals that must fully hide the world behind them — it's
	// painted UNDER the glass body so the frame + filigree still composite on
	// top. The skill-tree modal uses it; reuse it for any future opaque modal.
	surfaceCardBackdrop = rl.NewColor(24, 20, 29, 255)
	// surfaceDimScrim is the translucent dark wash laid over inactive elements
	// to recede them beneath a lit selection (the non-active party ribbon
	// cards). Close to glassBaseWash but its own alpha so the two stay tunable.
	surfaceDimScrim = rl.NewColor(6, 8, 14, 105)

	// ----- Wood frames -----
	woodDark   = rl.NewColor(48, 30, 18, 255)
	woodMid    = rl.NewColor(96, 62, 36, 255)
	woodLight  = rl.NewColor(150, 104, 64, 255)
	woodAccent = rl.NewColor(184, 140, 92, 255)
	woodInlay  = rl.NewColor(34, 22, 14, 175)

	// ----- Gilt accents (selection / focus) -----
	giltDim    = rl.NewColor(160, 124, 64, 255)
	giltBright = rl.NewColor(232, 196, 112, 255)
	// Coin sigil tones (the gold HUD coin glyph): a brighter face and a darker
	// inner shade than the gilt selection accents. Named here with the rest of
	// the gilt family so a palette retune reaches the coin glyph too, instead
	// of leaving two bare literals in hud.go's drawCoinGlyph.
	coinFace  = rl.NewColor(218, 168, 78, 255)
	coinShade = rl.NewColor(152, 104, 42, 255)

	// ----- Controller glyphs (on-screen button icons) -----
	// Modern-game face-button hues, drawn as the letter + ring on a dark
	// button body (the dark-button / colored-letter style that reads cleanest
	// over the library's dark glass). mute()-wrapped like every other bright
	// accent so the icons calm to the library palette while staying instantly
	// recognizable (A green / B red / X blue / Y amber). See render/glyphs.go.
	glyphAColor = mute(rl.NewColor(60, 168, 74, 255))  // confirm
	glyphBColor = mute(rl.NewColor(206, 66, 58, 255))  // back / cancel
	glyphXColor = mute(rl.NewColor(58, 124, 206, 255)) // use
	glyphYColor = mute(rl.NewColor(228, 178, 52, 255)) // panels / Tome
	// glyphBody is the dark raised button fill behind every glyph; glyphRim is
	// the light bevel stroke around the non-face buttons (shoulders / start /
	// select / d-pad); glyphInk is the neutral label/icon ink on those.
	glyphBody = rl.NewColor(34, 30, 40, 235)
	glyphRim  = rl.NewColor(150, 140, 150, 220)
	glyphInk  = rl.NewColor(232, 226, 214, 255)

	// ----- Parchment ink (text) -----
	inkPrimary = rl.NewColor(232, 222, 196, 255)
	inkMuted   = rl.NewColor(184, 172, 144, 240)
	inkDim     = rl.NewColor(132, 122, 100, 220)
	inkAccent  = rl.NewColor(232, 196, 112, 255)
	// statusGlyphDark is the near-black used for cut-out details inside a few
	// status glyphs (the skull's sockets/jaw, the ingest prey dot, the shield's
	// center spine) AND for the enemy roster pill's glyph + turn count (drawn
	// dark on the bright pill fill). Cross-file (party.go glyphs + battle.go
	// pills), so it lives here with the ink tokens.
	statusGlyphDark = rl.NewColor(12, 10, 15, 255)
	// statusIconBacking is the dark, slightly purple disc drawn behind a party
	// status icon so its colored glyph reads against the HUD glass. Sits with
	// statusGlyphDark since both are the status-icon ink tokens.
	statusIconBacking = rl.NewColor(22, 19, 26, 235)
	// statusNoneAccent is the neutral light-grey the PartyStatusNone row carries
	// in partyStatusVisuals — the "no status" placeholder accent (no glyph). A
	// named token so the table doesn't open-code a bare grey literal.
	statusNoneAccent = rl.NewColor(220, 220, 220, 220)

	// ----- Semantic aliases of the library palette (use freely) -----
	// These names predate the wood/glass/gilt nomenclature but resolve
	// to the same RGB values — pick whichever reads best in context.
	surfacePrimary    = glassDeep
	surfaceLog        = glassMid
	surfaceVeil       = veil
	surfaceActiveTint = glassWarm
	surfaceTargetTint = rl.NewColor(20, 38, 32, 140)    // faint emerald glass for friendly target
	surfaceDownTint   = rl.NewColor(28, 22, 28, 115)    // knocked down — dim grey wash
	accentPartyDown   = rl.NewColor(120, 110, 116, 200) // knocked-down party card accent (name tick / edge)
	surfaceEnemyTint  = glassDanger

	// Enemy-roster row tints (drawEnemyRosterRow): the live row's glass
	// fill + warm border, and the dimmed pair used while a defeated enemy
	// fades out. Named here so the row chrome shares the palette's source
	// of truth instead of carrying its own NewColor literals.
	surfaceRosterRow      = rl.NewColor(20, 14, 22, 130)
	borderRosterRow       = rl.NewColor(96, 60, 64, 140)
	surfaceRosterRowFaded = rl.NewColor(28, 20, 24, 95)

	// Border aliases — used by drawCard as the OUTERMOST frame
	// stroke. Default panels use woodDark (deepest band); active /
	// danger panels swap in a saturated tint so the focused surface
	// reads first.
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
	textDim     = inkDim
	textHint    = inkDim

	barHPHigh = mute(rl.NewColor(116, 200, 132, 255))
	// xpGainColor fills the victory spoils screen's XP bars — a warm
	// coin-gold that reads as treasure/reward (not the green of an HP gauge).
	// Shares coinFace so the XP fill and the gold tally sit in one palette.
	xpGainColor = coinFace
	barHPMid    = mute(rl.NewColor(224, 184, 88, 255))
	barHPLow    = mute(rl.NewColor(220, 88, 88, 255))
	barMP       = mute(rl.NewColor(104, 152, 224, 255))
	barEnemyHP  = mute(rl.NewColor(204, 76, 76, 255))
	// Enemy wound-state ramp — the per-condition tint the roster's health
	// label reads as, from unharmed green through to the near-death red.
	// Pulled out of battle.go's enemyConditionColors table so the accent ramp
	// lives in the palette beside the other bars instead of as bare NewColor
	// literals. The near-death tone (condEnemyNearDeath) shares the "enemy is
	// bleeding out" red intent with barEnemyHP / barHPLow; kept a touch hotter
	// than the muted static bar so a small roster label still reads.
	condEnemyUnharmed     = rl.NewColor(126, 231, 170, 255)
	condEnemyScuffed      = rl.NewColor(208, 226, 128, 255)
	condEnemyInjured      = rl.NewColor(246, 196, 91, 255)
	condEnemyBadlyWounded = rl.NewColor(244, 126, 75, 255)
	condEnemyNearDeath    = rl.NewColor(255, 78, 88, 255)
	// partyDamagePopupColor is the floating-number tone for damage the PARTY
	// takes. Brighter/hotter than barEnemyHP (which is muted for a static bar)
	// so a fast-fading popup still reads against the formation; distinct from
	// the timing-grade ramp the player's outgoing hits use.
	partyDamagePopupColor = rl.NewColor(255, 100, 88, 255)
	barTrack              = rl.NewColor(8, 12, 22, 140) // near-black track, already muted
	// barMutedFill is the desaturated plum fill drawBar swaps in when a
	// bar is muted (e.g. a downed member's gauges).
	barMutedFill = rl.NewColor(96, 84, 92, 230)
	// barGhostHot is the trailing "damage ghost" segment a live gauge leaves
	// behind when its value drops (see barghost.go) — hot parchment-gold so
	// the just-lost slice reads as a burn against any fill tone. Gilt-family
	// transient, so NOT routed through mute() (matches the gilt accents).
	barGhostHot = rl.NewColor(255, 226, 168, 235)

	// ----- Per-status accents (UI_STANDARDS.md "Per-status accents") -----
	// Indexed by core.PartyStatusKind via partyStatusVisuals below; the
	// raw tokens are exported here so non-party surfaces (enemy pills,
	// future field-status overlays) can pull the same hue without
	// re-typing the RGBs.
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
	// statusBlessed is a POSITIVE buff accent — a warm holy gilt that reads as
	// a blessing rather than the green/amber threat hues. Distinct from
	// statusStun's flatter yellow by its warmer, gilt-leaning tone (and it
	// never flickers).
	statusBlessed = mute(rl.NewColor(244, 212, 128, 240))
	// statusRegen is the POSITIVE heal-over-time accent (Renewal) — a fresh
	// mint green that reads as "healing," kept cleaner/brighter than
	// statusPoison's sickly olive so the two greens don't confuse. Never
	// flickers (it's good news).
	statusRegen = mute(rl.NewColor(120, 224, 150, 240))
	// statusShielded is the POSITIVE Aegis-ward accent — a luminous teal/cyan
	// energy bubble, kept off the crowded light-blues (statusSleep / statusDefending)
	// so the magic ward reads apart from the manual block. Never flickers.
	statusShielded = mute(rl.NewColor(96, 222, 214, 240))
	// statusIceArmor is the POSITIVE frost-ward accent — a pale icy blue-white,
	// cooler/lighter than statusSleep so the two don't confuse. Never flickers.
	statusIceArmor = mute(rl.NewColor(186, 226, 248, 240))
	// Outline tints paired with the fills above for the enemy-pill
	// silhouette. Lighter / more saturated than the fill so the pill
	// reads as a "glow with a hard rim" against the panel.
	statusBurnOutline   = mute(rl.NewColor(255, 200, 120, 220))
	statusSleepOutline  = mute(rl.NewColor(190, 220, 244, 220))
	statusPoisonOutline = mute(rl.NewColor(180, 232, 132, 220))
	statusBleedOutline  = mute(rl.NewColor(248, 120, 120, 220))
	statusStunOutline   = mute(rl.NewColor(248, 232, 160, 230))

	// Turn-order panel: the danger red an enemy row reads as. Named so
	// it isn't a bare literal buried in turnEntryColor's getter.
	turnEnemyColor = mute(rl.NewColor(245, 100, 92, 255))

	// Timing-bar accent tokens. The bright cursor white, the "held /
	// RELEASE" gold, and the sequence-bar pass/fail greens & reds used
	// to recur as bare NewColor literals at drifting alphas across the
	// press / charge / sequence draws. Defined once at full alpha; the
	// few sites that want a softer alpha wrap these in colorWithAlpha.
	timingCursorColor = rl.NewColor(248, 248, 252, 255)
	timingHeldColor   = rl.NewColor(255, 244, 144, 255)
	seqOkColor        = mute(rl.NewColor(140, 232, 168, 255))
	seqFailColor      = mute(rl.NewColor(228, 96, 96, 255))
	// timingWarnColor is the "running low on time" amber the sequence
	// strip fades through before it goes red. timingCommitColor is the
	// multi-press bar's late commit-zone orange. timingTickColor is the
	// near-black vertical separator between charge-bar segments. Named
	// here with the other timing accents so the bars don't carry bare
	// NewColor literals at drifting alphas.
	timingWarnColor   = rl.NewColor(232, 196, 92, 235)
	timingCommitColor = rl.NewColor(255, 168, 96, 200)
	timingTickColor   = rl.NewColor(28, 32, 44, 235)

	// Timing-grade accent ramps — the per-quality attack (warm: red→amber→
	// cream) and defend (cool: red→blue→pale ice) tones the bar flashes when a
	// hit lands, indexed Miss..Excellent by timing.go's qualityVisuals table.
	// Pulled out of that table so the grade ramp lives with the other timing
	// accents instead of as bare NewColor literals mid-file. Defined at full
	// alpha and full saturation: this is a fast transient confirmation flash,
	// so (like the gilt-family transients) it's NOT routed through mute() —
	// the punch is the point.
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

	// Per-mode timing-bar heading + base-fill tints, paired with the
	// timingHeading* label strings in timing.go. Each bar's signature hue lives
	// here with the other timing accents instead of as a bare NewColor literal
	// inside its draw function. (The Combo bar derives its tint from seqOkColor
	// and stays at its call site.)
	timingHeadingStrikeColor       = mute(rl.NewColor(255, 232, 168, 240)) // warm gold
	timingHeadingDefendColor       = mute(rl.NewColor(168, 220, 255, 240)) // cool blue
	timingHeadingChargeColor       = mute(rl.NewColor(255, 184, 96, 240))  // warm orange
	timingHeadingReelsColor        = mute(rl.NewColor(255, 244, 144, 240)) // gold-yellow gamble
	timingHeadingRecallMemoColor   = mute(rl.NewColor(255, 244, 144, 240)) // memorize: held-yellow
	timingHeadingRecallRecallColor = mute(rl.NewColor(140, 232, 168, 240)) // recall: thief green

	// reelSymbolColors are Steal's slot-symbol fill hues — well-separated colors
	// so a "match" reads at a glance. Indexed by symbol modulo its length so it
	// stays safe if core.ReelSymbolCount ever changes (symbols would just share
	// hues rather than index out of range). Lives here with the other timing
	// accents rather than as bare NewColor literals in timing.go.
	reelSymbolColors = []rl.Color{
		mute(rl.NewColor(255, 206, 84, 255)),  // gold
		mute(rl.NewColor(96, 208, 255, 255)),  // cyan
		mute(rl.NewColor(236, 120, 200, 255)), // magenta
		mute(rl.NewColor(140, 232, 168, 255)), // green
	}

	// Battle-splash banner tones: splashBgColor is the near-black panel fill
	// behind the banner; splashTitleColor is the warm cream the encounter title
	// is engraved in. Both are defined at full alpha here — the splash applies
	// its fade-driven alpha per-frame via colorWithAlpha so the banner can ease
	// in / out. Lives here with the other battle accents rather than as bare
	// NewColor literals in battle.go.
	splashBgColor    = rl.NewColor(8, 10, 16, 255)
	splashTitleColor = rl.NewColor(248, 232, 198, 255)

	// Billboard tints for the in-world combatant markers — the warm
	// off-white the player's target reads as, and the slightly redder
	// pulse the currently-attacking enemy reads as. Pulled out here so
	// future palette passes don't have to chase NewColor literals across
	// world.go's draw loop.
	tintEnemyTargeted = rl.NewColor(255, 228, 190, 255)
	tintEnemyAttacker = rl.NewColor(255, 196, 156, 255)
	// Selector-pyramid colors for the same three combat states the billboard
	// tints above cover — the bright marker over the targeted enemy, the
	// friendly heal/item target, and the enemy's attack target. Kept in this
	// block so the "what color is targeted" decision lives beside the tints
	// (the marker is brighter / more saturated than the soft sprite wash by
	// design — they're paired, not identical). world.go's markerStyle rows
	// read these instead of inlining NewColor literals.
	selectorEnemyTargetColor    = rl.NewColor(255, 224, 80, 255)
	selectorFriendlyTargetColor = rl.NewColor(118, 235, 136, 245)
	selectorEnemyAttackColor    = rl.NewColor(255, 96, 96, 245)
	// Party-side billboard tints, the mirror of the enemy pair above: the
	// desaturated gray a knocked-out member fades to, and the warm wash on
	// the member whose turn it is. Named here so DrawPartySprites pulls from
	// the same palette block instead of inlining NewColor literals.
	tintPartyDown   = rl.NewColor(110, 110, 120, 190)
	tintPartyActive = rl.NewColor(255, 245, 204, 255)

	// Editor + minimap entity-marker colors. Centralised here so the
	// in-grid markers (editor's draw.go), the on-disk-list swatches
	// (editor's brush palette), and the minimap dots all share one
	// source of truth — moving the door tone or chest tone is now one
	// edit instead of three NewColor literals. Exported via theme.go
	// as theme.MarkerXxx fields for external scenes.
	markerStart    = rl.NewColor(255, 220, 124, 255)
	markerChest    = rl.NewColor(232, 180, 92, 255)
	markerChestDim = rl.NewColor(160, 132, 78, 255)
	markerDoor     = rl.NewColor(176, 132, 86, 255)
	markerPack     = rl.NewColor(220, 76, 70, 255)
	markerPlayer   = rl.NewColor(132, 240, 148, 255) // player facing arrow (minimap + panels Map tab)
	// crystalCyanBase is the shared charged-crystal cyan — the single source of
	// truth for both the editor/minimap marker (markerCrystal) and the in-world
	// gem body (crystal.go's crystalColor pulses this base's R/G channels). Keeps
	// the marker and the entity the player sees in lockstep instead of two
	// hand-synced literals.
	crystalCyanBase = rl.NewColor(96, 214, 232, 255)
	// Companion crystal tints — the dormant gem body and the charged/dormant
	// faceted-edge wires. Live beside crystalCyanBase so the whole gem palette
	// (charged body + dormant body + both edge states) tunes in one place
	// instead of as scattered literals in crystal.go.
	crystalDormantBody = rl.NewColor(70, 92, 110, 190)   // dormant gem body (flat slate)
	crystalEdgeCharged = rl.NewColor(210, 250, 255, 220) // charged faceted wire
	crystalEdgeDormant = rl.NewColor(110, 130, 150, 150) // dormant faceted wire
	// crystalChargedBody is the full charged gem-body tint: R/G ride
	// crystalCyanBase (so the gem and editor marker stay in lockstep) with the
	// blue pinned full and a fixed alpha for the bright cut-crystal read.
	// crystalColor pulses only the R/G channels of this at render time.
	crystalChargedBody = rl.NewColor(crystalCyanBase.R, crystalCyanBase.G, 255, 235)
	// markerCrystal reads as the same charged-crystal cyan the player sees, and
	// stands clear of the amber chest / brown door / red pack / yellow start
	// swatches.
	markerCrystal = crystalCyanBase
	markerOutline = rl.NewColor(0, 0, 0, 220)

	// Hit-glyph clarity colors (render/hitglyph.go) — the signature hue of
	// each attack glyph. Named here so the hues can be matched to their VFX
	// particle bursts instead of living as scattered rl.NewColor literals per
	// painter. Painters override the alpha per frame via
	// colorWithAlpha(_, glyphFade(t)); the alpha below is a placeholder.
	glyphSlashColor  = rl.NewColor(245, 248, 255, 255)
	glyphImpactColor = rl.NewColor(255, 236, 150, 255)
	glyphFrostColor  = rl.NewColor(170, 224, 255, 255)
	glyphSparkBolt   = rl.NewColor(150, 205, 255, 255)
	glyphSparkCore   = rl.NewColor(225, 242, 255, 255)
	glyphFireOuter   = rl.NewColor(255, 150, 60, 255)
	glyphFireInner   = rl.NewColor(255, 222, 130, 255)
	glyphHolyColor   = rl.NewColor(255, 232, 150, 255)
	glyphVenomColor  = rl.NewColor(150, 230, 110, 255)

	// mapTileFogColor is the dim fill drawn for cells that fall outside
	// the area's bounds — the "fog" beyond the walkable map. Shared by
	// both map surfaces (the corner minimap window and the panels-overlay
	// Map tab) so the out-of-bounds tone is one source of truth instead
	// of two near-identical literals that drift by a couple RGB units.
	mapTileFogColor = rl.NewColor(8, 10, 14, 235)

	// chestColors govern the chest billboard — body color and lid color.
	// Pulled out here rather than open-coded in DrawChests so the palette
	// can be tuned without hunting through world-render code. The interact
	// prompt reuses borderActive (the global "draw the player's eye here"
	// yellow) so adding a new prompt color isn't needed.
	chestBodyColor = rl.NewColor(168, 116, 70, 255)
	chestLidColor  = rl.NewColor(196, 148, 92, 255)

	// Shadow tints for drop-shadowed text and overlay scrims. Pre-named so
	// callers don't open-code rl.NewColor(0,0,0,…) with a drifting alpha.
	// Strength runs Light (background hints) → Mid (HUD body) → Strong
	// (large titles / debug pills) → Heavy (top-of-stack labels).
	shadowLight  = rl.NewColor(0, 0, 0, 160)
	shadowMid    = rl.NewColor(0, 0, 0, 180)
	shadowStrong = rl.NewColor(0, 0, 0, 200)
	shadowHeavy  = rl.NewColor(0, 0, 0, 220)
	// shadowBase is the fully-transparent black drop-shadow base — the source
	// color callers fade per-frame when the shadow alpha is dynamic (the splash
	// title shadow and the timing-bar icon shadows track an animation). Named
	// so those sites don't open-code rl.NewColor(0,0,0,…) with a per-call alpha.
	shadowBase = rl.NewColor(0, 0, 0, 0)
	// noAccent is the zero-alpha sentinel passed to drawCard's `accent`
	// parameter to skip the optional left-spine stripe. Named so the intent
	// ("this card has no accent stripe") reads at the call site instead of a
	// bare transparent-black literal.
	noAccent = rl.NewColor(0, 0, 0, 0)
)

const (
	// hudEdgePad is the canonical distance every always-on HUD panel
	// keeps from the screen edges. Pulled into theme so the minimap,
	// turn panel, action log, action menu, and party ribbon all
	// honour the same margin without each one picking its own.
	// 16 reads "comfortable margin" at 1080p without wasting too much
	// real estate on smaller windows.
	hudEdgePad = int32(16)
	// hudColumnGap is the vertical spacing between stacked HUD
	// panels in the left/right column (minimap → turn panel →
	// action log). Smaller than hudEdgePad so adjacent panels feel
	// grouped rather than scattered.
	hudColumnGap = int32(10)
	// hudPanelMinH is the height floor a short-window collision guard shrinks a
	// pinned HUD pane to (the action log + the action menu) — below this the
	// pane's rows stop being readable, so it stays this tall even if that means
	// overlapping a neighbor on a very small window.
	hudPanelMinH = int32(160)

	// Enemy roster card (battle.go) — the top-center pane listing the foes.
	// Named here with the other HUD geometry so a "rows too cramped" retune is
	// one edit rather than bare literals at the draw site.
	rosterRowH      = int32(60)  // per-enemy row height
	rosterTopPad    = int32(18)  // inset above the first row
	rosterBottomPad = int32(18)  // inset below the last row
	rosterW         = int32(560) // multi-enemy width
	rosterWSingle   = int32(440) // single-enemy width (narrower)

	// Combat HUD panes (battle.go) — the bottom-left action log and the
	// bottom-right action menu. Both share the hudPanelMinH collision floor.
	actionLogW  = int32(320)
	actionLogH  = int32(300)
	actionMenuW = int32(340)
	actionMenuH = int32(312)
	// hudContentInsetX is the left gutter from a combat HUD pane's edge to its
	// content (enemy-card name, action-menu header/rows). Named so the roster
	// and action-menu surfaces share one inset instead of a bare 22 at each.
	hudContentInsetX = int32(22)
	// actionMenuHintMinH is the panel-height floor below which the action menu
	// drops its confirm/back hint footer (it would collide with the rows on a
	// short window). Sits above the hudPanelMinH row-readability floor.
	actionMenuHintMinH = int32(260)

	// Corner radii. Smaller than the previous pass (10/6 → 4/3) so the
	// frame reads as a hardwood mitre joint rather than a modern UI
	// rounded tile. See UI_STANDARDS.md "Panel" section.
	cornerRadius      = float32(4)
	smallCornerRadius = float32(3)
	stripeWidth       = int32(3)

	// Font sizes — the FIVE permitted text sizes across the whole HUD,
	// editor, and modal surface. See UI_STANDARDS.md "Type" section.
	// Anything else is a bug. All downsample from the high-res atlas bake
	// (hudFontBake, resources.go) with mipmaps + trilinear, so each renders
	// sharp without subpixel sludge. Bumped well up from the original
	// 13/16/20/26/36 ladder (~30% larger) for the Della Respira face —
	// bigger, more readable UI text; the bake is sized so even Title stays
	// crisp, and the optional faux-bold (drawBoldText) has room when on.
	FontTiny    = float32(17)
	FontSmall   = float32(21)
	FontBody    = float32(26)
	FontHeading = float32(36)
	FontTitle   = float32(48)

	// Letter spacing per size. Wider tracking on titles to sell the
	// "engraved on hardwood" feel. drawTextWithShadow (and the centered /
	// right-aligned / footer helpers built on it) applies these
	// AUTOMATICALLY via canonicalSpacing(size) — heading-size text tracks
	// at 2, title-size at 3, everything smaller at 1 — so plain call sites
	// conform without remembering the Style variant. Pass them explicitly
	// only through drawTextWithShadowStyle when pairing with a manual
	// MeasureTextEx. (FontTiny/FontSmall/FontBody all track at 1 —
	// FontSpacingBody covers the trio.)
	FontSpacingBody    = float32(1)
	FontSpacingHeading = float32(2)
	FontSpacingTitle   = float32(3)

	// woodFrameOuter / woodFrameInner are the stroke widths of the
	// outer dark band and the inner highlight pinstripe inside the
	// wood-panel border. Tuned so the frame reads at 1080p without
	// dominating; modest panels still feel substantial.
	woodFrameOuter = int32(2)
	woodFrameBand  = int32(3)
	woodFrameInner = int32(1)
	// cardFrameThick is the total wood-frame band width (outer + band + inner).
	// drawCard's stroke width, drawCardFiligree's corner inset (+5), and
	// drawCardInlay's inset (+4) all derive from this so bumping any frame band
	// keeps the three frame decorations aligned.
	cardFrameThick = woodFrameOuter + woodFrameBand + woodFrameInner

	// Focus-plate insets: how far the gilt selection plate (DrawSelectedRowI)
	// bleeds out past the row's text origin so the plate frames the row rather
	// than sitting flush behind it. focusPlateInsetX/Y are the standard pair the
	// shop + journal rows share; the pause menu sits its larger heading-tier rows
	// on a deeper plate (menuRowInsetX/Y) so the wider gilt margin matches the
	// bigger type. Named so none of the three carries a bare -12/-2 / -18/-6.
	focusPlateInsetX = int32(12)
	focusPlateInsetY = int32(2)
	menuRowInsetX    = int32(18)
	menuRowInsetY    = int32(6)

	// Heading tick markers (drawHeading underline) have a minimum width so
	// short headings still read as labelled. Bar value text inset is the
	// constant pad on the right edge of drawBar.
	headingTickMinWidth = int32(28)
	barValuePadRight    = float32(10)
	barLabelPadLeft     = float32(8)

	// Canonical HP/MP gauge heights for the three contexts drawBar / drawBarLive
	// render into: the full-height party-ribbon card gauge, the slightly shorter
	// gauge on the panels Character tab, and the mini gauge on the dense item /
	// target picker rows. Named here so the call sites stop carrying bare 32 / 28
	// / 18 height literals and a "make all the bars a touch slimmer" pass is one
	// edit per tier.
	barHeightFull    = float32(32)
	barHeightCompact = float32(28)
	barHeightMini    = float32(18)

	// World-popup horizontal slack: how many pixels past the screen edges a
	// 3D-to-2D projected popup can drift before we cull it. Larger than zero
	// so a popup whose anchor moves slightly off-screen still fades cleanly
	// instead of snapping to invisible mid-animation.
	offscreenPopupSlack = float32(200)

	// Overlay (modal card) dimensions. Each modal surface used to
	// hard-code its own cardW / cardH literals; centralizing them keeps
	// a future "shrink all modals on small screens" pass in one edit.
	// The dimensions are sized to the modal's CONTENT — a chest with
	// fewer items renders shorter via cardH expansion at the call site,
	// but the WIDTH stays standardized.
	overlayCardWidthSmall = int32(360) // chest modal (item list)
	// modalMinCardH floors a content-sized modal card's height so a short
	// node/list still reads as a card. Shared by the dialog and chest modals
	// (both previously floored at an independent bare 200).
	modalMinCardH = int32(200)
	// The level-up modal and game-panels overlay size themselves
	// screen-relative (drawScreenFractionScaffold) rather than off fixed
	// widths, so the character menus scale with the window and stay
	// readable. Clamped to the screen by overlayCardMarginScreen below.
	// The panels overlay used to run nearly edge-to-edge (0.95×0.93);
	// pulled in so the Tome reads as a framed dashboard with breathing
	// room around it rather than a full-screen takeover.
	panelsOverlayWidthFrac  = float32(0.80)
	panelsOverlayHeightFrac = float32(0.80)
	levelUpModalWidthFrac   = float32(0.60)
	levelUpModalHeightFrac  = float32(0.85)
	// victoryWidthFrac is the post-battle spoils card's screen-relative WIDTH
	// (its height is content-sized; see DrawVictorySpoils). Lives here with the
	// other modal-width fractions rather than in victory.go.
	victoryWidthFrac = float32(0.5)

	overlayCardMarginScreen = int32(40) // minimum margin between card and screen edges

	// Panels-overlay tab strip geometry. Shared by the panels surface
	// only today — moved here so future tab-strip surfaces (a future
	// equipment swap modal, a settings panel) can reuse the heights.
	overlayTabHeight  = int32(46)
	overlayTabPadding = int32(12)

	// overlayFooterReserve is the vertical band at the bottom of every
	// overlay card reserved for the "Esc close / L1 R1 tabs" hint
	// footer rendered by DrawFooterHint. Body rect = card minus this
	// band minus the heading band at the top.
	overlayFooterReserve = int32(28)
)

// drawModalScaffold paints the shared screen-veil + centered card +
// heading band for every modal overlay (chest, level-up, the panels
// overlay — Party Stats now lives in the panels Character tab, not a
// standalone modal). Returns the card rect so the caller can lay out its
// body inside without redoing the centering math. Pass an empty heading
// to skip the header band — the caller still gets the right card
// rect.
//
// The older overlays (chest / levelup) used to open-code
// rl.DrawRectangle(0,0,…,surfaceVeil) + drawCard + drawHeading each in
// slightly different orders. This helper plus the
// overlayCardWidth* / overlayCardHeight* constants in this file are
// the seam where future "shrink for small screens" or "fade-in"
// behaviour lands once.
// modalHeadingInsetY is the Y offset from a modal card's top edge to its
// heading baseline. Shared by drawModalScaffold's built-in heading and the
// dialog overlay's hand-drawn speaker nameplate (which passes an empty heading
// to the scaffold and stamps its own at the same band) so the two stay aligned.
const modalHeadingInsetY = int32(14)

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
		drawHeading(font, heading, int32(rect.X)+28, int32(rect.Y)+modalHeadingInsetY, borderActive)
	}
	return rect
}

// drawVeiledCard paints the full-screen veil, a centered wood-framed
// card at the given size, and its gilt corner filigree, returning the
// card rect for the caller to title + fill. The bare composition shared
// by drawModalScaffold (left-aligned heading) and the centered-title
// overlays (drawTitledMenuCard, DrawDoorPrompt) so the veil+card+filigree
// triple isn't maintained in three places. No screen clamp here —
// drawModalScaffold clamps before calling; the fixed-size menu/door
// cards don't need it. `outline`/`accent` are the drawCard strokes and
// `filigree` the corner-bracket tone.
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
	// Warm candlelight pool blooming from the center where the card will sit —
	// a soft radial wash that breathes with the flame. Darkens the periphery by
	// contrast so the eye is drawn inward to the lit dialog.
	poolR := float32(min(int(screenW), int(screenH))) * 0.66
	rl.DrawCircleGradient(cx, cy, poolR,
		fadeColor(rl.NewColor(78, 50, 24, 255), 0.11*flick),
		rl.NewColor(0, 0, 0, 0))
	// Drifting dust motes catching the candlelight — stateless, hash-seeded,
	// slowly falling and wrapping. Few enough (per drawDustMotes) to be free.
	drawDustMotes(screenW, screenH, flick)
}

// drawDustMotes scatters a handful of faint warm motes that drift slowly
// downward (wrapping at the bottom) with a gentle horizontal sway — the
// floating dust you only notice in a shaft of candlelight. Stateless: position
// and twinkle derive from a per-mote hash plus rl.GetTime(), so no pool to
// retain and nothing to reset between modals.
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
		// Twinkle inline from the already-captured t instead of pulse() (which
		// would re-cross cgo for rl.GetTime() once per mote).
		tw := 0.5 + 0.5*float32(math.Sin(t*(0.25+float64(i)*0.04)*math.Pi*2))
		a := (0.06 + 0.07*tw) * flick
		rl.DrawCircleV(rl.NewVector2(mx, my), 1.2, fadeColor(warm, a))
	}
}

// DrawCandlelitBackdrop paints a full-screen candlelit background: a deep
// vertical gradient (near-black indigo at the top settling to warm brown at the
// base), a wide radial pool of candlelight blooming from the upper third and
// breathing with the flame, drifting dust motes, and a faint material grain
// over everything. The title screen uses it so the launch screen reads as a
// grimoire opened by candlelight instead of a flat fill. Exported because the
// title package lives outside render.
func DrawCandlelitBackdrop(screenW, screenH int32) {
	if screenW <= 0 || screenH <= 0 {
		return
	}
	rl.DrawRectangleGradientV(0, 0, screenW, screenH,
		rl.NewColor(8, 10, 20, 255), rl.NewColor(22, 15, 12, 255))
	flick := candleFlicker()
	poolY := int32(float32(screenH) * 0.34)
	poolR := float32(min(int(screenW), int(screenH))) * 0.72
	rl.DrawCircleGradient(screenW/2, poolY, poolR,
		fadeColor(rl.NewColor(96, 62, 28, 255), 0.30*flick),
		rl.NewColor(0, 0, 0, 0))
	drawDustMotes(screenW, screenH, flick)
	drawHudGrain(0, 0, screenW, screenH, 0.5)
}

// drawScreenFractionScaffold sizes a modal card as a fraction of the
// current screen (wFrac × hFrac), then defers to drawModalScaffold for
// centering + clamping. The two screen-relative overlays (game panels,
// level-up) share it so the screenSize() read + multiply + int32-cast
// boilerplate lives in one place.
func drawScreenFractionScaffold(font rl.Font, wFrac, hFrac float32, heading string) rl.Rectangle {
	sw, sh := screenSize()
	return drawModalScaffold(font, int32(float32(sw)*wFrac), int32(float32(sh)*hFrac), heading)
}

// drawCardFiligree paints multi-stroke gilt corner brackets on a
// wood-framed card — the illuminated-manuscript bezel 90s AD&D PC
// RPGs used to dress dialog boxes. Each corner gets:
//
//   - outer L bracket (14 px arms, 2 px thick)
//   - inner secondary L bracket (8 px arms, 1 px thick, ~4 px inset)
//     in a softer gilt tone so it reads as a parallel cast line
//   - joint diamond pip (3 px) at the meeting point of the outer arms
//   - tip diamond pips (2 px) at the far ends of the outer arms
//   - centre highlight pip inside the joint diamond (1 px in bright
//     gilt) — the cast-metal speculum
//
// Skipped on panes too small to receive the ornament cleanly
// (< 80×80) so the corner minimap / turn panel stay simple.
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
		// Outer L — 14 px arms, 2 px thick. A 1px dark offset copy is laid
		// first so the bracket reads as cast metal RAISED off the frame
		// (the BG-era corner brace), not gilt paint flush with the wood.
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
		// Inner L — shorter and thinner, offset diagonally inward
		// from the outer bracket. Reads as the second cast line
		// of a Gothic frame.
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

// statIconDrawers dispatches each Stat to its sigil drawer. A fixed
// [core.StatCount] array (not a switch) so a new Stat forces a slot and
// the init below panics on a nil entry at startup — the same coverage
// contract the other render registries use, instead of a switch that
// silently draws nothing for an unmapped Stat.
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

// drawStatIcon dispatches to the per-stat sigil drawer. One small glyph
// per Stat enum value, used on the level-up modal's stat rows and the
// panels overlay's Stats tab so each row reads at a glance without
// leaning on the 3-letter label.
func drawStatIcon(s core.Stat, cx, cy, r float32, col color.RGBA) {
	if int(s) < 0 || int(s) >= len(statIconDrawers) {
		return
	}
	statIconDrawers[s](cx, cy, r, col)
}

// STR — short-shafted hammer: rectangular head on top, narrow
// handle hanging below, with a thin gilt band where head meets
// handle. The "strength of arms" sigil.
func drawStatIconSTR(cx, cy, r float32, col color.RGBA) {
	headHalfW := r * 0.85
	headH := r * 0.55
	rl.DrawRectangle(int32(cx-headHalfW), int32(cy-r), int32(headHalfW*2), int32(headH), col)
	// Head face band — a brighter inner stripe along the bottom of
	// the head reads as a steel rim against the haft.
	rl.DrawRectangle(int32(cx-headHalfW), int32(cy-r+headH-2), int32(headHalfW*2), 1, giltBright)
	// Haft running down from the head.
	haftHalfW := r * 0.16
	rl.DrawRectangle(int32(cx-haftHalfW), int32(cy-r+headH), int32(haftHalfW*2), int32(r*1.35), col)
	// Pommel knob at the haft's foot.
	rl.DrawCircleV(rl.NewVector2(cx, cy+r*0.45), haftHalfW*1.4, fadeColor(col, 0.85))
}

// DEX — arrow pointing up-right: tip triangle, shaft rectangle,
// fletching V at the tail. The classic "agility / precision" sigil.
func drawStatIconDEX(cx, cy, r float32, col color.RGBA) {
	// Arrow axis points NE. Direction unit vector + perpendicular.
	ax, ay := sqrt2Inv, -sqrt2Inv
	px, py := sqrt2Inv, sqrt2Inv
	// Tip triangle.
	tip := rl.NewVector2(cx+ax*r, cy+ay*r)
	tipBaseW := r * 0.45
	tipBack := rl.NewVector2(cx+ax*r*0.55, cy+ay*r*0.55)
	tipL := rl.NewVector2(tipBack.X+px*tipBaseW, tipBack.Y+py*tipBaseW)
	tipR := rl.NewVector2(tipBack.X-px*tipBaseW, tipBack.Y-py*tipBaseW)
	drawTriangleCCW(tip, tipR, tipL, col)
	// Shaft.
	shaftHalfW := r * 0.12
	for t := float32(-r * 0.85); t < r*0.55; t += 1 {
		px2 := cx + ax*t
		py2 := cy + ay*t
		// Tiny disc per sample — produces a rotated rectangle look
		// without depending on a rotated-rect primitive. Cheap
		// enough at r ~ 10 (samples ~25).
		rl.DrawCircleV(rl.NewVector2(px2, py2), shaftHalfW, col)
	}
	// Fletching V at the tail — two short diagonal strokes.
	tail := rl.NewVector2(cx-ax*r, cy-ay*r)
	fl1 := rl.NewVector2(tail.X+px*r*0.45, tail.Y+py*r*0.45)
	fl2 := rl.NewVector2(tail.X-px*r*0.45, tail.Y-py*r*0.45)
	drawTriangleCCW(tail, fl1, rl.NewVector2(tail.X+ax*r*0.35, tail.Y+ay*r*0.35), col)
	drawTriangleCCW(tail, rl.NewVector2(tail.X+ax*r*0.35, tail.Y+ay*r*0.35), fl2, col)
}

// INT — open book: two rectangular leaves meeting at a centre
// spine, with a small bookmark hanging off the side. The "lore /
// arcane study" sigil.
func drawStatIconINT(cx, cy, r float32, col color.RGBA) {
	pageHalfW := r * 0.7
	pageH := r * 1.3
	// Spine in centre.
	rl.DrawRectangle(int32(cx)-1, int32(cy-pageH/2), 2, int32(pageH), fadeColor(col, 0.7))
	// Left page (slight tilt via two triangles for the top-edge
	// curl, but flat-rect body for legibility).
	rl.DrawRectangle(int32(cx-pageHalfW), int32(cy-pageH/2), int32(pageHalfW)-1, int32(pageH), col)
	// Right page.
	rl.DrawRectangle(int32(cx)+1, int32(cy-pageH/2), int32(pageHalfW)-1, int32(pageH), col)
	// Page-line hatching — two thin horizontal pencil marks on
	// each page so the book reads as written-in, not blank.
	hatch := fadeColor(col, 0.4)
	rl.DrawRectangle(int32(cx-pageHalfW+2), int32(cy-pageH/4), int32(pageHalfW-4), 1, hatch)
	rl.DrawRectangle(int32(cx-pageHalfW+2), int32(cy+1), int32(pageHalfW-4), 1, hatch)
	rl.DrawRectangle(int32(cx+3), int32(cy-pageH/4), int32(pageHalfW-4), 1, hatch)
	rl.DrawRectangle(int32(cx+3), int32(cy+1), int32(pageHalfW-4), 1, hatch)
	// Bookmark — a thin gilt ribbon hanging off the right page.
	bmHalfW := float32(1)
	rl.DrawRectangle(int32(cx+pageHalfW*0.55), int32(cy-pageH/2), int32(bmHalfW*2), int32(pageH+3), giltBright)
}

// WIS — eye with iris: lens-shaped outline + centre pupil + bright
// catchlight. The "perception / divine sight" sigil.
func drawStatIconWIS(cx, cy, r float32, col color.RGBA) {
	// Lens: two triangles meeting along the horizontal axis make
	// a diamond, then we round it visually with a smaller circle
	// sitting inside.
	lensHalfW := r * 0.95
	lensHalfH := r * 0.55
	// Top half — triangle from left point through top arc to right
	// point. Approximate the arc with a single triangle (eye reads
	// at small size).
	left := rl.NewVector2(cx-lensHalfW, cy)
	right := rl.NewVector2(cx+lensHalfW, cy)
	top := rl.NewVector2(cx, cy-lensHalfH)
	bot := rl.NewVector2(cx, cy+lensHalfH)
	drawTriangleCCW(left, top, right, col)
	drawTriangleCCW(left, right, bot, col)
	// Iris — filled inner disc.
	rl.DrawCircleV(rl.NewVector2(cx, cy), lensHalfH*0.7, fadeColor(col, 0.55))
	// Pupil — dark centre dot.
	rl.DrawCircleV(rl.NewVector2(cx, cy), lensHalfH*0.3, fadeColor(col, 0.25))
	// Catchlight — bright pip slightly off-centre, like the gleam
	// in a painted portrait.
	rl.DrawCircleV(rl.NewVector2(cx-lensHalfH*0.18, cy-lensHalfH*0.18), 1.4, giltBright)
}

// VIT — heart shape: two lobes (filled discs) up top, a V-point at
// the bottom (triangle), with a bright inner pip. The "vitality"
// sigil.
func drawStatIconVIT(cx, cy, r float32, col color.RGBA) {
	lobeR := r * 0.42
	lobeY := cy - r*0.2
	lobeOffset := lobeR * 0.85
	rl.DrawCircleV(rl.NewVector2(cx-lobeOffset, lobeY), lobeR, col)
	rl.DrawCircleV(rl.NewVector2(cx+lobeOffset, lobeY), lobeR, col)
	// V-point — triangle from each lobe's outer edge down to the
	// chin.
	leftAnchor := rl.NewVector2(cx-lobeOffset-lobeR*0.85, lobeY+lobeR*0.25)
	rightAnchor := rl.NewVector2(cx+lobeOffset+lobeR*0.85, lobeY+lobeR*0.25)
	chin := rl.NewVector2(cx, cy+r*0.95)
	drawTriangleCCW(leftAnchor, chin, rightAnchor, col)
	// Inner highlight — bright pip slightly above centre for the
	// "alive and beating" feel.
	rl.DrawCircleV(rl.NewVector2(cx-lobeOffset*0.4, lobeY-lobeR*0.2), 1.4, giltBright)
}

// SPD — lightning bolt: a zigzag polygon drawn as a strip of
// triangles. Reads as "speed / initiative" without needing a label.
func drawStatIconSPD(cx, cy, r float32, col color.RGBA) {
	// Define the bolt as a closed 6-vertex polygon, then triangle-
	// fan from the first vertex. Vertices walk top-down across the
	// zigzag.
	verts := []rl.Vector2{
		{X: cx - r*0.05, Y: cy - r},     // top spike
		{X: cx + r*0.5, Y: cy - r*0.1},  // upper right notch
		{X: cx + r*0.05, Y: cy - r*0.1}, // inner step
		{X: cx + r*0.4, Y: cy + r},      // bottom spike
		{X: cx - r*0.5, Y: cy + r*0.1},  // lower left notch
		{X: cx - r*0.05, Y: cy + r*0.1}, // inner step
	}
	// Fan: (v0, v1, v2), (v0, v2, v3), (v0, v3, v4), (v0, v4, v5).
	for i := 1; i < len(verts)-1; i++ {
		drawTriangleCCW(verts[0], verts[i+1], verts[i], col)
	}
	// Bright inner highlight along the bolt's mid-axis — a tiny
	// gilt pip at the kink so the bolt has a "live" centre.
	rl.DrawCircleV(rl.NewVector2(cx, cy), 1.4, giltBright)
}

// drawEmptyLedgerNote is the standard "this page is blank" treatment for an
// empty list surface (no quests, no recorded foes, an empty bag): a dim gilt
// fleuron with short flanking hairlines — the unfilled-entry mark a scribe
// rules into a ledger — over the centered message in muted ink. Sits in the
// upper third of the body rect so it reads as the page's letterhead, not a
// modal. One composable so every empty state speaks the same dialect instead
// of each tab left-aligning bare text at its own inset.
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
	// Optional second line — the "what fills this page" hint, a step dimmer
	// and smaller so the note keeps one clear voice.
	if sub != "" {
		drawTextCentered(font, sub, cx, ornY+18+FontBody+10, FontSmall, textHint)
	}
}

// starVertsBuf backs starVerts so the per-frame star draws (the Wizard class
// glyph on the always-visible party ribbon, the ★ rich-text glyph) reuse one
// slice instead of escaping a fresh one to the heap each call. The render path
// is single-threaded, so sharing the buffer is safe — same scratch pattern as
// the status-glyph spoke tables. Sized for the 5-point star (10 verts); grows
// if a caller ever asks for more points.
var starVertsBuf = make([]rl.Vector2, 0, 10)

// starVerts returns the 2×points alternating outer/inner vertices of a regular
// star centered at (cx, cy), the first vertex pointing straight up (-90°) and
// stepping by π/points. Shared by the Wizard class glyph and the ★ rich-text
// glyph so the two star silhouettes are built from one place. The returned
// slice aliases a reused package buffer — copy it if it must outlive the next
// call.
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

// drawDiamondPip paints a small filled diamond centered on (cx, cy)
// with half-extent r. Used at filigree corner joints and as the
// fleuron sigil flanking the pause-menu title.
func drawDiamondPip(cx, cy, r float32, col color.RGBA) {
	top := rl.NewVector2(cx, cy-r)
	right := rl.NewVector2(cx+r, cy)
	bottom := rl.NewVector2(cx, cy+r)
	left := rl.NewVector2(cx-r, cy)
	drawTriangleCCW(top, left, right, col)
	drawTriangleCCW(right, left, bottom, col)
}

// drawFleuron paints a four-direction gilt fleuron — a centre
// diamond flanked by teardrop leaves on all four compass points, with
// a bright inner pip at the heart. The classic "chapter divider"
// sigil illuminated manuscripts and 90s PC RPGs used as ornamental
// punctuation. Sized by `r` (the centre diamond's half-extent);
// flanking leaves scale to ~0.85 r so the whole motif fits in a
// roughly 6r square.
//
// The bright inner pip uses giltBright regardless of the caller's
// requested colour so the centre catches a highlight against a
// duller bracket tone — gives the fleuron a tiny "cast metal"
// reflection instead of reading as a flat silhouette.
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
	// Bright speculum at the heart of the diamond.
	if r >= 3 {
		drawDiamondPip(cx, cy, r*0.35, giltBright)
	}
}

// drawFleuronsFlanking paints a gilt fleuron `gap` px outside each end of
// a centered label — the ◆ label ◆ motif shared by the menu titles and
// the level-up Apply gate. leftX is the label's left edge, w its measured
// width, cy the vertical midline.
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

// drawGiltFocusRing paints the bold 3px gilt "you're here" frame around a
// focused/active rounded surface, with corners matching the glass body radius.
// Shared by the active party member card (panels.go) and the focused skill-tree
// node plate (skilltree.go) — both want the identical giltBright ring, so the
// roundness + thickness + color triple lives in one place.
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

// drawAccentStripe paints a card's left-edge class rail, inset slightly so it
// reads as part of the card rather than its border. Thin wrapper over the
// shared drawClassRail (the one embellished-rail look) — just the card inset
// geometry.
func drawAccentStripe(panelX, panelY, panelH int32, col color.RGBA) {
	if panelH < 16 {
		return
	}
	drawClassRail(panelX+5, panelY+8, stripeWidth, panelH-16, col)
}

// drawGiltRule paints a thin horizontal gilt separator — the hairline a
// panel heading or tab strip draws under itself. One primitive so the
// "thin brass divider" stamp lives in a single place; callers pass the
// rect and the alpha to fade giltBright by. (The battle splash's
// fleuron-flanked divider is a distinct ornament and stays bespoke.)
func drawGiltRule(x, y, w, h int32, alpha float32) {
	rl.DrawRectangle(x, y, w, h, fadeColor(giltBright, alpha))
}

// drawPipCappedRule paints a 1px horizontal rule from x to x+w in ruleCol,
// flanked by a diamond pip of radius pipR (in pipCol) seated on each end. This
// is the "ruled divider with filigree termini" stamp the action-menu header and
// the panels overlay's info-strip both draw; centralizing the rule + two end
// pips keeps the two from drifting. (drawPanelHeading paints a richer multi-layer
// rule with an extra brass glint and a trailing fleuron, so it stays bespoke.)
func drawPipCappedRule(x, y, w int32, ruleCol color.RGBA, pipR float32, pipCol color.RGBA) {
	rl.DrawRectangle(x, y, w, 1, ruleCol)
	drawDiamondPip(float32(x), float32(y), pipR, pipCol)
	drawDiamondPip(float32(x+w), float32(y), pipR, pipCol)
}

// drawSplitRule draws a 1px horizontal rule from leftX to rightX broken by a
// `gap` on each side of cx (to seat a centre fleuron). The two-segment line is
// shared by the title banner divider (DrawTitleRule, which adds end-cap +
// centre fleurons over it) and the battle-splash divider (which fades it with
// the splash) so the segment math isn't hand-written in both.
func drawSplitRule(leftX, rightX, cx, y, gap float32, col color.RGBA) {
	rl.DrawRectangle(int32(leftX), int32(y), int32(cx-gap-leftX), 1, col)
	rl.DrawRectangle(int32(cx+gap), int32(y), int32(rightX-(cx+gap)), 1, col)
}

// drawCard renders a wood-framed glass pane — the library aesthetic
// every panel-shaped surface uses. Owns the four-layer composition
// from UI_STANDARDS.md "Panel": outer woodDark stroke, woodMid band,
// woodLight inner highlight, glass tint body. The `accent` parameter
// is preserved as the optional left-spine stripe (for class-tinted
// active actor panels, etc.); pass a zero-alpha color to skip.
//
// `fill` should be one of the glass tokens (glassDeep / glassMid /
// glassWarm / glassDanger). `outline` is used as the OUTERMOST
// stroke; callers can pass `woodDark` for the standard frame or
// borderActive / borderDanger to tint the frame for state. The
// woodMid band + woodLight highlight are always painted between
// the outline and the glass body — the structural feel of the
// frame doesn't degrade for state changes.
func drawCard(x, y, w, h int32, fill, outline, accent color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	// Outer wood-framed card composition:
	//   1. Translucent glass body filling the whole pane (via
	//      drawGlassPane — the shared dark-wash + tint pair every
	//      translucent surface in the HUD uses).
	//   2. Three concentric hardwood frame strokes painted ON TOP
	//      as outlines, so the body underneath stays glass.
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

// drawPanelCard is the standard HUD panel: a drawCard with the default
// surfacePrimary glass body and borderSoft frame, no accent spine. The
// "neutral panel" most static surfaces (minimap frame, turn order, battle
// log) draw, factored out so the (surfacePrimary, borderSoft, borderSoft)
// triple lives in one place instead of being re-typed per call site.
func drawPanelCard(x, y, w, h int32) {
	drawCard(x, y, w, h, surfacePrimary, borderSoft, borderSoft)
}

// drawCardBevel carves the flat three-stroke frame into raised molding — the
// chunky directionally-lit bevel 90s CRPG dialog frames wore. Two opposing
// light reads, hairlines only (the strokes stay the structure):
//
//   - RAISED outer edge: a warm highlight along the frame's top (and a dimmer
//     one down the left) where overhead candlelight catches the molding, and
//     a deep shadow along the bottom (dimmer up the right) where it falls
//     away — the frame stands proud of the world.
//   - RECESSED inner lip: the opposite pair where the frame meets the glass —
//     shadow under the top lip (the molding overhangs the pane) and a faint
//     light at the bottom lip (light pooling on the sill) — the glass sits
//     sunk INTO the frame.
//
// Lines stop short of the corners (cornerClear) so they never fight the
// rounded mitres; small chips skip the treatment entirely.
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
	hi := fadeColor(woodAccent, 0.80)
	hiSide := fadeColor(woodAccent, 0.36)
	lo := fadeColor(shadowHeavy, 0.62)
	loSide := fadeColor(shadowHeavy, 0.34)
	// Raised outer edge.
	rl.DrawRectangle(hx, y+1, hw, 1, hi)
	rl.DrawRectangle(hx, y+h-2, hw, 1, lo)
	rl.DrawRectangle(x+1, vy, 1, vh, hiSide)
	rl.DrawRectangle(x+w-2, vy, 1, vh, loSide)
	// Recessed inner lip (only when the pane is tall enough that the two
	// reads don't merge into stripes).
	if h >= 56 {
		rl.DrawRectangle(hx, y+ft, hw, 1, fadeColor(shadowHeavy, 0.42))
		rl.DrawRectangle(hx, y+h-ft-1, hw, 1, fadeColor(woodAccent, 0.22))
	}
}

func drawCardDropShadow(x, y, w, h int32) {
	if w < 32 || h < 24 {
		return
	}
	roundness := fixedRoundnessFor(w, h, cornerRadius)
	// Three stacked, offset copies — widest+faintest first — so the card casts
	// a soft graduated shadow that lifts it convincingly off the world behind
	// it rather than a single hard drop.
	wide := rl.NewRectangle(float32(x+10), float32(y+14), float32(w), float32(h))
	soft := rl.NewRectangle(float32(x+6), float32(y+8), float32(w), float32(h))
	near := rl.NewRectangle(float32(x+2), float32(y+3), float32(w), float32(h))
	rl.DrawRectangleRounded(wide, roundness, 8, fadeColor(shadowHeavy, 0.13))
	rl.DrawRectangleRounded(soft, roundness, 8, fadeColor(shadowHeavy, 0.22))
	rl.DrawRectangleRounded(near, roundness, 8, fadeColor(shadowStrong, 0.28))
}

// drawCardInlay adds the small carved details that make a HUD pane read
// like old CRPG cabinetry rather than a flat overlay: an inner dark groove,
// a soft gilt hairline just inside it, and tiny brass pips at the mitered
// corners. Skips narrow/tiny panes so chips and small row highlights stay
// crisp instead of over-ornamented.
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

	// A partial gilt line near the BOTTOM reads like a recessed brass wire
	// inlay without boxing in the content. The former TOP hairline sat at the
	// content's top edge and cut straight through panel/card titles (party-card
	// class names, modal headings, the minimap area name) — it was removed; the
	// bottom rule alone keeps the carved-cabinetry read without crossing text.
	lineInset := int32(18)
	if innerW > lineInset*2 {
		botY := y + h - inset - 3
		lineX := x + inset + lineInset
		lineW := innerW - lineInset*2
		rl.DrawRectangle(lineX, botY, lineW, 1, fadeColor(giltDim, 0.28))
	}

	corners := [4]rl.Vector2{
		rl.NewVector2(float32(x+inset), float32(y+inset)),
		rl.NewVector2(float32(x+w-inset), float32(y+inset)),
		rl.NewVector2(float32(x+inset), float32(y+h-inset)),
		rl.NewVector2(float32(x+w-inset), float32(y+h-inset)),
	}
	// Domed brass rivets at the inner corners — the carved-cabinet hardware
	// detail. Replaces the old flat diamond pips with lit half-spheres.
	for _, c := range corners {
		drawBrassStud(c.X, c.Y, 2.3)
	}
}

// drawGlassPane paints the canonical translucent glass body — a dark
// `glassBaseWash` underlay plus the family `fill` tint on top — with
// no wood frame. Use this for nested sub-panels inside a card (member
// cards, equipment slot bezels, skill rows, items detail card, tab
// tiles) so every translucent surface in the UI composites the same
// way against the world content behind it.
//
// drawCard internally calls this for its body. Callers that need a
// framed pane reach for drawCard; callers that need a frame-less
// translucent body (sub-panes inside a card) reach for this directly.
// drawSmallPanel — single-layer opaque-ish fill — stays the right
// choice for actual small chrome (status pills, chips, gilt rails).
func drawGlassPane(x, y, w, h int32, fill color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	rect := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	// cornerRadius (4) for big card bodies; small sub-panes still
	// look round at this radius and the unified curvature makes
	// nested panes harmonise with their parent's frame.
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
// (The "wet dream by candlelight" pass — real material tooth, recessed glass,
// lit hardwood, and a living flame breathing over the gilt.)

// hudGrainTex is the tileable transparent grain overlay (specks + fibers) minted
// once in NewResources and tiled over every glass body by drawGlassRelief. A
// package singleton because the theme draw helpers are free functions with no
// Resources handle (same reason groundShadowModel is). hudGrainReady guards the
// pre-init / headless window (tests) so the draw is a clean no-op.
var (
	hudGrainTex   rl.Texture2D
	hudGrainReady bool
)

// hudGrainAlpha is the white-tint alpha the grain overlay is drawn at. The
// texels are already low-alpha, so this is a master "tooth" knob — raise for a
// rougher, more antique surface, lower toward 0 to silence it.
const hudGrainAlpha = float32(0.85)

// drawHudGrain tiles hudGrainTex across (x,y,w,h) at the given alpha. WrapRepeat
// on the texture means a source rect the size of the destination tiles it in a
// single draw. No-op until the texture exists.
func drawHudGrain(x, y, w, h int32, alpha float32) {
	if !hudGrainReady || w <= 0 || h <= 0 {
		return
	}
	src := rl.NewRectangle(0, 0, float32(w), float32(h))
	dst := rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
	rl.DrawTexturePro(hudGrainTex, src, dst, rl.NewVector2(0, 0), 0, fadeColor(rl.White, alpha))
}

// candleFlicker returns a slow, organic 0.86..1.0 multiplier — the breath of a
// candle flame. Three incommensurate sines summed so it never reads as a clean
// loop; multiply gilt accent brightness/alpha by it on the ornaments that
// should feel lit (frame speculum, brass studs, heading rule) for the
// "by candlelight" shimmer. Deliberately shallow — it should register
// subliminally, never strobe.
// flickerCache memoizes candleFlicker within a frame. rl.GetTime() is constant
// across a single frame, so the (3-sin) flicker value is identical for every
// ornament that asks for it that frame — it was being recomputed dozens of
// times per frame (4 brass studs per card, frame sheen, heading, etc.). Keyed
// on the frame's time so it auto-refreshes next frame.
var flickerCache struct {
	t      float64
	value  float32
	primed bool
}

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

// drawGlassRelief layers the depth cues over a translucent glass body: the
// material grain, a soft inner shadow bleeding down from the top inner edge
// (the pane sits recessed behind its frame), and a thin warm rim-light along
// the bottom inner edge (light pooling at the base of lit cabinet glass). Kept
// faint so the world still reads through. Called at the tail of drawGlassPane,
// so EVERY glass surface in the UI — cards, sub-panes, focusable rows — gains
// the same physically-consistent depth at once.
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
	// Just the material grain — the glass sheen comes from drawGlassGradientWash
	// and the lift from drawCardDropShadow. The earlier inner-shadow + rim-light
	// hairlines read as ambiguous "lines on glass" noise, so they're gone.
	drawHudGrain(x+inset, y+inset, iw, ih, hudGrainAlpha)
}

// drawBrassStud paints a small domed rivet — the cabinet-hardware detail at
// frame corners: a dark seat ring, a gilt dome, and a bright upper-left
// speculum (candle-modulated) so it reads as a polished metal half-sphere
// catching the light rather than a flat dot.
func drawBrassStud(cx, cy, r float32) {
	rl.DrawCircleV(rl.NewVector2(cx, cy), r+1, fadeColor(woodDark, 0.9))
	rl.DrawCircleV(rl.NewVector2(cx, cy), r, giltDim)
	rl.DrawCircleV(rl.NewVector2(cx-r*0.32, cy-r*0.32), r*0.42,
		fadeColor(giltBright, 0.92*candleFlicker()))
}

// drawFocusableRow paints a selectable list row: a glass body that
// warm-tints when focused, plus a gilt selection outline on the focused
// row. One definition of the "cursored row" look shared by the panels
// overlay's Skills list, Equipment slot rows, and the equip-slot
// picker, so the three can't drift on fill tone or outline weight.
func drawFocusableRow(rect rl.Rectangle, focused bool) {
	bg := fadeColor(glassDeep, 0.55)
	if focused {
		bg = fadeColor(glassWarm, 0.85)
	}
	drawGlassPane(int32(rect.X), int32(rect.Y), int32(rect.Width), int32(rect.Height), bg)
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

// drawSelectionHalo paints the shared "this is the live selection" emphasis: a
// solid inner ring around (x,y,w,h) plus a wider, pulsing outer ring in the
// same tint. `pulseV` is the caller's breathing-curve sample (pulseActiveActor
// for the party ribbon's active card, pulseHalo for the battle roster's
// targeted row) so each surface keeps its own cadence; `small` picks the
// small-radius outline for compact surfaces. Both call sites route through this
// so their halo geometry + pulse→alpha mapping can't drift.
func drawSelectionHalo(x, y, w, h int32, tint color.RGBA, pulseV float32, small bool) {
	outline := drawPanelOutline
	if small {
		outline = drawSmallPanelOutline
	}
	outline(x, y, w, h, tint)
	outline(x-3, y-3, w+6, h+6, fadeColor(tint, 0.30+0.55*pulseV))
}

// drawPaneDropShadow stamps the cheap offset drop shadow under a selectable
// glass pane (the menu/list selected row + the skill-tree node plate) — one
// offset + alpha so the two can't drift apart by a couple alpha points.
func drawPaneDropShadow(r rl.Rectangle) {
	rl.DrawRectangle(int32(r.X+2), int32(r.Y+3), int32(r.Width), int32(r.Height), fadeColor(shadowHeavy, 0.20))
}

// drawPanelHeading paints a FontHeading title with the standard
// wood-accent tick mark underline. Replaces the older drawHeading
// helper (kept as an alias below). Use this for every persistent
// HUD panel title and every modal heading.
//
// `accent` is the underline color — pass woodAccent for resting
// panels, borderActive for the focused / active panel, borderDanger
// for danger modals, etc. Header text is always inkPrimary.
func drawPanelHeading(font rl.Font, text string, x, y float32, accent color.RGBA) {
	// Engraved (top-lit gradient) lettering — the heading tier wears the
	// 90s-CRPG metal-leaf treatment everywhere via this one seam.
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
	// Polished-brass speculum: a short bright glint riding the left of the rule,
	// breathing with the candle so the heading underline reads as lit metal.
	if ruleW > 28 {
		rl.DrawRectangle(ruleX+6, ruleY, 12, 1, fadeColor(giltBright, 0.6*flick))
	}
	drawDiamondPip(float32(ruleX+2), float32(ruleY+1), 2.4, fadeColor(accent, 0.85))
	drawDiamondPip(float32(ruleX+ruleW-2), float32(ruleY+1), 2.4, fadeColor(accent, 0.85))
	drawFleuron(float32(ruleX+ruleW+8), float32(ruleY+1), 3.2, fadeColor(accent, 0.65*flick))
}

// measureKey identifies a cached text measurement: the string plus the
// size + spacing it was shaped at. (The same text can be measured at
// different sizes — e.g. the panels Stats tab uses both FontBody and
// FontSmall, the timing heading flips FontHeading↔FontTitle.)
type measureKey struct {
	text          string
	size, spacing float32
}

// measureCache memoizes rl.MeasureTextEx, flushing when the font atlas
// changes (font.Texture.ID shifts after a settings flip / font reload).
// MeasureTextEx is a cgo round-trip that re-shapes the string; the HUD
// re-measures the same labels every frame, so every per-frame measure
// call site shares this ONE implementation instead of re-hand-rolling
// the map + font-ID guard (there were ~11 near-identical copies). The
// zero value is ready to use.
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
	// measureRichText so a string carrying a procedurally-drawn symbol
	// (richtext.go) measures to its real on-screen width — otherwise centered
	// / right-aligned labels with a symbol in them would drift by the missing
	// glyph's (zero/notdef) advance. Symbol-free strings fall straight through
	// to rl.MeasureTextEx.
	v := measureRichText(font, text, size, spacing)
	c.entries[key] = v
	return v
}

// qualityPopupMeasureCache / damagePopupMeasureCache back the floating combat
// popups, which animate their font size via a throb `scale`. The draw paths
// measure at the FIXED base size (cache-friendly — few distinct labels) and
// scale the result linearly, so the per-frame cgo MeasureTextEx round-trip a
// size-keyed cache would never hit is avoided. The spacing term doesn't scale
// with size, but for the short popup strings the error is sub-pixel and the
// text is centered, so it's invisible.
var (
	qualityPopupMeasureCache measureCache
	damagePopupMeasureCache  measureCache
)

// panelHeadingMeasureCache backs drawPanelHeading, which runs every frame
// for every visible HUD panel ("COMBAT LOG", "TURN ORDER", "PAUSED", …).
var panelHeadingMeasureCache measureCache

func measurePanelHeading(font rl.Font, text string) rl.Vector2 {
	return panelHeadingMeasureCache.measure(font, text, FontHeading, FontSpacingHeading)
}

// pulse oscillates 0..1 at the given frequency in Hz.
func pulse(speed float64) float32 {
	return 0.5 + 0.5*float32(math.Sin(rl.GetTime()*speed*math.Pi*2))
}

// rowSheenPeriod is the seconds one full sheen sweep takes to cross a
// selected row (drawRowSheen). Slow — the band drifts like candlelight
// caught on lacquer, it doesn't "scan."
const rowSheenPeriod = 3.8

// drawRowSheen sweeps a soft gilt light-band across a selection plate — the
// candle catching the polished brass as it breathes. Scissor-clipped to the
// rect so the band never paints outside the row; the band position derives
// from wall-clock time, so every selected row in the UI shares one drifting
// light source (they don't strobe independently). Skipped on rows too small
// to read the gradient. Layered ABOVE the warm glass body and BELOW the
// spine/pips/underline so the ornaments stay crisp under the moving light.
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
	// 0..1 sweep phase; the band starts fully off the left edge and exits
	// fully off the right so there's a beat of "no sheen" between passes.
	t, _ := math.Modf(rl.GetTime() / rowSheenPeriod)
	x := r.X - band + float32(t)*(r.Width+2*band)
	peak := fadeColor(giltBright, 0.13*flick)
	clear := fadeColor(giltBright, 0)
	rl.BeginScissorMode(int32(r.X), int32(r.Y), int32(r.Width), int32(r.Height))
	rl.DrawRectangleGradientH(int32(x), int32(r.Y), int32(band/2), int32(r.Height), clear, peak)
	rl.DrawRectangleGradientH(int32(x+band/2), int32(r.Y), int32(band/2), int32(r.Height), peak, clear)
	rl.EndScissorMode()
}

// pulseActiveActor / pulseHalo / pulseFlicker are the three canonical
// breathing curves from UI_STANDARDS.md ("Pulse / breathing"). They are
// the single source of truth so the active-actor frame, the selection
// halo (cursor / target chevron), and the status flicker can't drift
// apart into bespoke per-call-site amplitudes + frequencies. Each
// re-expresses the documented `base + amp·sin(t·π·f)` in terms of
// pulse(speed) (= 0.5 + 0.5·sin(t·speed·2π)), so speed = f/2 and the
// offset/scale fold the documented base/amp:
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

// colorWithAlpha replaces col's alpha channel with `byteAlpha` (0-255).
// Differs from fadeColor which multiplies the existing alpha by a
// normalized 0..1 factor — colorWithAlpha is the "I know exactly what
// alpha I want, regardless of the source color's alpha" form. Used by
// turn-order panel tints (per-class color at varying transparencies
// based on row state) where the source colors already encode the hue
// and the alpha is a UI-state knob.
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

// barLabelMeasureCache backs the short constant bar labels ("HP"/"MP");
// barValueMeasureCache backs the "10/20" value strings on the right edge.
// drawBar runs ~16×/frame across the party ribbon + enemy roster.
var barLabelMeasureCache measureCache
var barValueMeasureCache measureCache

func measureBarLabel(font rl.Font, label string) rl.Vector2 {
	return barLabelMeasureCache.measure(font, label, FontTiny, 1)
}

func measureBarValue(font rl.Font, valText string) rl.Vector2 {
	return barValueMeasureCache.measure(font, valText, FontSmall, 1)
}

// clampBarPct clips a fill fraction to [0, 1] — shared by the gauge body and
// the live-state wrappers so over/underflowing values (temp HP, debug boosts)
// can't draw outside the track.
func clampBarPct(pct float32) float32 {
	if pct < 0 {
		return 0
	}
	if pct > 1 {
		return 1
	}
	return pct
}

// drawBar renders a track + filled portion + thin outline, all rounded.
// label is drawn as a small uppercase tag at the bar's left, value text on right.
// Static form — no trailing damage ghost, no low-value heartbeat. Dashboard
// surfaces (the Tome's Stats tab) use this; live combat gauges go through
// drawBarLive so the juice only plays where the stakes are.
func drawBar(font rl.Font, x, y, width, height float32, label string, value, maxValue int, fill color.RGBA, muted bool) {
	mv := maxValue
	if mv <= 0 {
		mv = 1
	}
	drawBarState(font, x, y, width, height, label, clampBarPct(float32(value)/float32(mv)), formatBarValue(value, maxValue), fill, muted, -1, false, false)
}

// drawBarFraction draws a gauge at a CONTINUOUS fill fraction with a
// caller-supplied label + value readout — for animated bars that shouldn't
// step per integer unit (the victory XP gauge fills through it, smoothly).
// No damage ghost or low-value heartbeat (those are HP-gauge treatments).
func drawBarFraction(font rl.Font, x, y, width, height float32, label, valText string, frac float32, fill color.RGBA, muted bool) {
	// gradientFill=true: the XP gauge fills left-to-right faint→rich and gains
	// overall opacity as it nears full, so climbing toward the next level reads
	// as building intensity.
	drawBarState(font, x, y, width, height, label, frac, valText, fill, muted, -1, false, true)
}

// drawBarLive is drawBar plus the living-gauge treatments, keyed by a stable
// identity string (e.g. "hp:Warrior") that owns the trailing state:
//   - damage ghost: a hot gilt segment marks the just-lost slice, holding a
//     beat then draining into the fill edge (barghost.go) — the size of a hit
//     reads from the ribbon even when the eye was on the timing bar;
//   - heartbeat: when the value sits at or under a quarter, the fill breathes
//     at the status-flicker rate and the value text turns danger-red, so a
//     critical member reads peripherally without a popup.
//
// The party ribbon's HP gauges use both; its MP gauge stays static (spends
// are deliberate, not threats). Muted (downed) gauges suppress both.
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

// drawBarState is the shared gauge body behind drawBar / drawBarLive.
// ghostPct >= 0 draws the trailing damage segment from the fill edge out to
// that level; heartbeat enables the low-value breathing treatment.
func drawBarState(font rl.Font, x, y, width, height float32, label string, pct float32, valText string, fill color.RGBA, muted bool, ghostPct float32, heartbeat, gradientFill bool) {
	// pct arrives as a CONTINUOUS fraction (drawBar derives it from value/max;
	// drawBarFraction passes a smoothly-animated one for the XP gauge), so the
	// fill width moves sub-integer-smoothly rather than stepping per unit.
	pct = clampBarPct(pct)
	track := barTrack
	outline := borderDim
	if muted {
		fill = barMutedFill
	}
	// Low-value heartbeat: the fill itself breathes at the canonical status-
	// flicker rate. Gated on heartbeat (live gauges only) so dashboard bars
	// and muted (downed) gauges stay still.
	lowPulse := heartbeat && pct > 0 && pct <= 0.25
	if lowPulse {
		fill = fadeColor(fill, 0.70+0.30*pulseFlicker())
	}
	ix, iy, iw, ih := int32(x), int32(y), int32(width), int32(height)
	drawGaugeWell(ix, iy, iw, ih)
	drawSmallPanel(ix, iy, iw, ih, track)
	// Trailing damage ghost — painted UNDER the live fill, spanning from the
	// fill edge out to the held level, so the drain visibly sweeps toward the
	// real value. Slightly translucent: the track grain reads through, which
	// keeps it "afterimage," not "second fill."
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
				// Left-to-right faint→rich opacity, and the whole fill gains
				// opacity as pct climbs — so the gauge "builds" toward the
				// leading edge / the next level (the right end). The leading
				// edge reaches full opacity only at a full bar.
				lead := fadeColor(fill, core.Lerp(0.45, 1.0, pct))
				rl.DrawRectangleGradientH(ix+1, iy+1, fillW, ih-2, fadeColor(fill, 0.32), lead)
			} else {
				drawSmallPanel(ix+1, iy+1, fillW, ih-2, fill)
			}
			drawGaugeFillDepth(ix+1, iy+1, fillW, ih-2, muted)
			// Liquid meniscus — a bright hairline riding the fill's leading
			// edge so the level reads as the surface of liquid in a glass
			// tube, and any motion (drain, regen tick, ghost catch-up)
			// glints. Skipped at the extremes (nothing to mark) and on
			// muted gauges.
			if !muted && pct < 1 && fillW >= 6 && ih > 8 {
				menX := ix + 1 + fillW - 1
				menCol := fadeColor(core.MixColor(fill, inkPrimary, 0.65), 0.85)
				rl.DrawRectangle(menX, iy+3, 2, ih-6, menCol)
			}
		}
	}
	drawSmallPanelOutline(ix, iy, iw, ih, outline)
	drawGaugeBezel(ix, iy, iw, ih, muted)

	// Tape-measure ticks — small triangular notches biting in from
	// the top and bottom edges of the bar at the 25/50/75 % marks.
	// The 50 % mark gets a deeper notch (the cardinal index on a
	// brass scale); the 25 / 75 marks are shorter (secondary
	// graduations). Notches read as cast-metal cutouts against
	// both the fill and the empty track because they're triangles
	// pointing inward in a wood-accent tone, distinct from both
	// glass and any HP/MP tint. Skipped on tiny bars (<80 px wide
	// or <12 px tall) so compact UI surfaces don't look busy.
	if iw >= 80 && ih >= 12 && !muted {
		tickCol := fadeColor(woodAccent, 0.85)
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
			// Top notch — triangle pointing DOWN into the bar.
			topApex := rl.NewVector2(tx, float32(iy)+tk.depth)
			topL := rl.NewVector2(tx-tk.width/2, float32(iy))
			topR := rl.NewVector2(tx+tk.width/2, float32(iy))
			drawTriangleCCW(topL, topApex, topR, tickCol)
			// Bottom notch — triangle pointing UP into the bar.
			botApex := rl.NewVector2(tx, float32(iy+ih)-tk.depth)
			botL := rl.NewVector2(tx-tk.width/2, float32(iy+ih))
			botR := rl.NewVector2(tx+tk.width/2, float32(iy+ih))
			drawTriangleCCW(botL, botR, botApex, tickCol)
		}
	}

	// Bar labels (HP / MP / etc.) — always FontTiny per UI_STANDARDS.md
	// (the bar IS small, the value text is what reads at a glance).
	// Cream-bright color + heavy shadow so the tag pops on any fill.
	labelSize := FontTiny
	labelColor := inkPrimary
	if muted {
		labelColor = textDim
	}
	// labelSize stays FontTiny throughout; the measurement is keyed on
	// it inside measureBarLabel for cgo-call avoidance.
	labelMeasure := measureBarLabel(font, label)
	labelY := y + (float32(ih)-labelMeasure.Y)/2 - 1
	labelX := x + barLabelPadLeft
	rl.DrawTextEx(font, label, rl.NewVector2(labelX+2, labelY+2), labelSize, 1, shadowHeavy)
	rl.DrawTextEx(font, label, rl.NewVector2(labelX+1, labelY+1), labelSize, 1, shadowHeavy)
	rl.DrawTextEx(font, label, rl.NewVector2(labelX, labelY), labelSize, 1, labelColor)

	if valText != "" {
		// Value text is always FontSmall per UI_STANDARDS.md — the
		// number is the bar's readable content and stays consistent
		// regardless of bar height. Bright by default; faded only
		// when muted. Double-offset drop shadow for contrast on
		// any fill.
		valSize := FontSmall
		valColor := textPrimary
		if muted {
			valColor = textDim
		}
		// Critical gauge: the number itself turns danger-red alongside the
		// breathing fill, so "someone's about to drop" reads even at a
		// glance that only catches the text.
		if lowPulse {
			valColor = barHPLow
		}
		// Value text size is locked at FontSmall inside measureBarValue;
		// valSize stays in scope below for the draw calls.
		valMeasure := measureBarValue(font, valText)
		valY := y + (float32(ih)-valMeasure.Y)/2 - 1
		valX := x + width - valMeasure.X - barValuePadRight
		rl.DrawTextEx(font, valText, rl.NewVector2(valX+2, valY+2), valSize, 1, shadowHeavy)
		rl.DrawTextEx(font, valText, rl.NewVector2(valX+1, valY+1), valSize, 1, shadowHeavy)
		rl.DrawTextEx(font, valText, rl.NewVector2(valX, valY), valSize, 1, valColor)
	}
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
		// Glass-tube specular cap — a bright hairline riding the top of the
		// fill so the gauge reads as a curved, light-catching tube of liquid
		// rather than a flat bar. Brighter when not muted.
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

// drawClassRail paints THE shared embellished 3-D character-color rail — a
// jeweled inlay sunk into the surface, not a flat bar. Layers back-to-front:
// a recessed dark channel (reads inlaid); the rounded class-color core; a lit
// left edge + shadowed right edge (rounded bevel); and a gilt diamond pip
// capping each end like a brass terminal, breathing with the candlelight.
// This is the ONE per-character colored tick/stripe look across the whole
// UI — card accents (drawAccentStripe: party ribbon, action menu), the
// panels member columns, the turn-order rows, the victory spoils rows — so
// none of them drift. Callers pass explicit geometry (insets differ per
// surface). `col` carries the intended alpha.
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

// barValueLabelCache memoizes the "value/max" bar readout per (value, max)
// pair. drawBar/drawBarLive call formatBarValue once per visible bar per frame
// (HP + MP on every party card, plus enemy roster bars — up to ~8 persistent
// bars), so without the cache every one of those allocated a fresh string each
// frame. Bounded: keys only span (value, max) pairs actually seen in play.
// Mirrors enemyHPLabelCache in battle.go.
var barValueLabelCache = map[[2]int]string{}

func formatBarValue(value, maxValue int) string {
	k := [2]int{value, maxValue}
	if s, ok := barValueLabelCache[k]; ok {
		return s
	}
	// Direct strconv concat avoids the fmt formatter machinery: "%d/%d" via
	// fmt.Sprintf allocates ~3× the bytes of the result. Only runs on a cache
	// miss now (first time a given value/max pair is shown).
	s := strconv.Itoa(value) + "/" + strconv.Itoa(maxValue)
	barValueLabelCache[k] = s
	return s
}

// drawTriangleCCW wraps rl.DrawTriangle with an explicit "vertices are in
// counter-clockwise order in screen-Y-down coords" contract. raylib's 2D
// pipeline has GL_CULL_FACE enabled on some drivers (Intel/AMD have been
// observed culling CW-wound triangles silently — the triangle just doesn't
// appear). Use this helper at every 2D triangle call site so the winding
// requirement is named at the call, not buried in a comment somewhere.
//
// "CCW in screen-Y-down" means the signed cross product of (b-a)×(c-b) is
// NEGATIVE — that's the inverted convention vs the y-up math-textbook one
// because screen Y increases downward.
func drawTriangleCCW(a, b, c rl.Vector2, col color.RGBA) {
	rl.DrawTriangle(a, b, c, col)
}

// drawArrowMarker paints a small triangle chevron. The base sits at `center`
// perpendicular to the direction; the apex is `center + (tipDx, tipDy)`.
// Base width is 2*halfWidth. Used by HUD selection / target / active-actor
// indicators where a tiny arrow reads better than a label — saves party,
// battle, and item-target panels from each computing their own three
// rl.Vector2 corners by hand. Goes through drawTriangleCCW so the winding
// constraint stays visible.
func drawArrowMarker(center rl.Vector2, tipDx, tipDy, halfWidth float32, col color.RGBA) {
	tipLen := float32(math.Sqrt(float64(tipDx*tipDx + tipDy*tipDy)))
	if tipLen == 0 {
		return
	}
	px := -tipDy / tipLen * halfWidth
	py := tipDx / tipLen * halfWidth
	// Apex → base1 → base2 is CCW in screen-Y-down for any tipDx/tipDy
	// because px/py is the (tipDx,tipDy) vector rotated +90° in screen
	// space (which is -90° in math space → CCW).
	drawTriangleCCW(
		rl.NewVector2(center.X+tipDx, center.Y+tipDy),
		rl.NewVector2(center.X-px, center.Y-py),
		rl.NewVector2(center.X+px, center.Y+py),
		col,
	)
}

// canonicalSpacing maps a font size to its standard letter spacing from
// UI_STANDARDS.md "Type": Tiny/Small/Body track at 1, Heading at 2
// (FontSpacingHeading), Title and up at 3 (FontSpacingTitle). The single
// source the plain text helpers (drawTextWithShadow, drawTextCentered,
// drawTextRightAligned, DrawFooterHint, the wrap/fit helpers) consult, so
// heading-size text drawn through the convenience path gets its engraved
// tracking automatically and draw + measure can never disagree on width.
// Sites with a genuinely load-bearing ad-hoc spacing still use
// drawTextWithShadowStyle, which takes spacing explicitly.
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

// drawTextWithShadow paints text twice: once offset by (1,1) at shadowStrong,
// once at the requested color. The single +1 offset reads as a clean drop
// shadow under most HUD sizes; callers that want a heavier shadow for large
// titles (menu rows, debug pills) go through drawTextWithShadowStyle. Letter
// spacing follows canonicalSpacing(size) — the UI_STANDARDS tracking ladder —
// so FontHeading text through this path engraves at spacing 2 without every
// call site having to remember the Style variant. Lives here alongside the
// shadowLight/Mid/Strong/Heavy palette it consumes.
func drawTextWithShadow(font rl.Font, text string, x, y, size float32, col color.RGBA) {
	drawTextWithShadowStyle(font, text, x, y, size, canonicalSpacing(size), col, shadowStrong, 1, 1)
}

// rowTextColor centralizes the focus/disable palette for list-row labels so the
// {textPrimary, textMuted, disabled} tones aren't hand-picked (and prone to
// drift) at each list. A disabled row takes disabledCol; otherwise a focused
// row reads textPrimary and an unfocused one textMuted. Two-state lists that
// convey focus with a selection plate rather than the text color (shop rows,
// skill-tree node names) pass focused=enabled with their own disabled tone.
func rowTextColor(focused, disabled bool, disabledCol color.RGBA) color.RGBA {
	switch {
	case disabled:
		return disabledCol
	case focused:
		return textPrimary
	}
	return textMuted
}

// tabLabelMeasurer returns the FontBody label-width closure drawTextTabStrip
// expects, backed by the given measure cache — the shared shape behind the
// shop's and Journal's tab strips, instead of each hand-rolling the same
// `func(s string) float32 { return cache.measure(font, s, FontBody, …).X }`.
func tabLabelMeasurer(cache *measureCache, font rl.Font) func(string) float32 {
	return func(s string) float32 { return cache.measure(font, s, FontBody, FontSpacingBody).X }
}

// drawTextTabStrip paints a left-anchored row of simple FontBody text tabs
// starting at (x, y): the active tab in activeCol (with an inkAccent underline
// when underline is set), the rest textMuted, advancing by measured width +
// gap. `label` formats tab i; `measure` returns a label's width so callers can
// supply a cache-backed measurer where it matters (the Journal) or a direct one
// (the shop). Returns the x just past the last tab. This is the shared rhythm
// behind the shop's Buy/Sell header and the Journal's Quests/Bestiary sub-tabs;
// the panels overlay's MAIN tab strip is a different visual (even-width glass
// tiles, centered labels) and intentionally does not route through here.
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

// engravedMeasureCache backs drawEngravedText's band math — engraved labels
// (panel headings, action verbs, menu titles) are stable strings redrawn
// every frame their surface is up.
var engravedMeasureCache measureCache

// drawEngravedText paints large text as top-lit engraved metal — the gradient
// lettering 90s CRPG dialog headers wore. Four passes over one string:
//
//	heavy drop shadow (+2,+2) → full-height body in `base` → a bright
//	top-light band (upper ~45 %, base mixed toward parchment-white) → a
//	deep band at the baseline (lower ~28 %, base mixed toward black)
//
// The two bands are scissor-clipped re-draws of the SAME string, so the
// gradient rides the letterforms exactly (counters and serifs shade
// correctly) — no texture, no per-glyph work. Spacing follows
// canonicalSpacing(size), same as drawTextWithShadow, so engraved and plain
// text of the same size always measure identically — callers can center
// against a plain MeasureTextEx at the canonical spacing.
//
// Reserve it for the heading tier and up (panel/modal headings, action
// verbs, menu titles): at body sizes the bands collapse into noise, and the
// four passes are priced for the handful of large labels on screen, not for
// list rows.
func drawEngravedText(font rl.Font, text string, x, y, size float32, base color.RGBA) {
	drawEngravedTextSpaced(font, text, x, y, size, canonicalSpacing(size), base)
}

// drawEngravedTextSpaced is the explicit-spacing core of drawEngravedText,
// for the few engraved surfaces whose tracking is load-bearing (the
// timing-bar prompt's 1.5). Pair with a MeasureTextEx at the same spacing.
func drawEngravedTextSpaced(font rl.Font, text string, x, y, size, spacing float32, base color.RGBA) {
	m := engravedMeasureCache.measure(font, text, size, spacing)
	// drawRichTextKnown (not raw DrawTextEx) so engraved labels carrying a
	// symbol the font lacks — e.g. the pause-menu "Options ▸" / "Debug ▸" rows —
	// get the procedural glyph (richtext.go) in every pass. The scissor bands
	// clip the symbol shapes too, so they pick up the same top-light /
	// deep-shade gradient as the letters. The has-symbol scan runs ONCE here,
	// not per pass (this draws the string four times); symbol-free headings
	// then take the fast path in all four.
	hasSym := containsSymGlyph(text)
	drawRichTextKnown(font, text, hasSym, x+2, y+2, size, spacing, shadowHeavy)
	drawRichTextKnown(font, text, hasSym, x, y, size, spacing, base)
	// Band rects pad ±2 px horizontally so antialiased glyph edges aren't
	// shaved at the scissor boundary.
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

// drawTextWithShadowStyle is the parametric form of drawTextWithShadow.
// shadowCol picks the drop color (shadowLight/Mid/Strong/Heavy above);
// offX/offY pick the drop offset in pixels; `spacing` is the letter
// spacing passed through to rl.DrawTextEx. Use this when an ad-hoc
// shadow alpha, offset, or letter spacing is actually load-bearing
// (splash titles, menu rows, the debug overlay's 1.2 spacing); prefer
// the non-styled drawTextWithShadow for everything else so HUD shadows
// stay consistent.
func drawTextWithShadowStyle(font rl.Font, text string, x, y, size, spacing float32, col, shadowCol color.RGBA, offX, offY float32) {
	// Route through drawRichTextKnown so any geometric/arrow/relational symbols
	// the font lacks (▶ → ↑↓ ● ★ ✓ ≤ ≥ …) are drawn procedurally (richtext.go)
	// instead of baking as missing-glyph boxes. The has-symbol scan runs ONCE
	// here and is shared by the shadow + main pass — symbol-free strings (the
	// vast majority) then take the single-DrawTextEx fast path in both, so this
	// per-frame hot path adds only one cheap ASCII byte-scan per label.
	hasSym := containsSymGlyph(text)
	drawRichTextKnown(font, text, hasSym, x+offX, y+offY, size, spacing, shadowCol)
	drawRichTextKnown(font, text, hasSym, x, y, size, spacing, col)
}

// drawHeading is the legacy panel-heading helper. Routes through
// drawPanelHeading so the heading size is the standardized FontHeading
// across every call site — the old hand-tuned 20pt is gone. Kept as
// an alias because dozens of call sites pass int32 coords; over time
// they should adopt drawPanelHeading directly.
func drawHeading(font rl.Font, text string, x, y int32, accent color.RGBA) {
	drawPanelHeading(font, text, float32(x), float32(y), accent)
}
