# UI Standards — The Library Aesthetic

> Read this before touching any rendering code. The HUD, modals, editor
> chrome, and battle overlays all conform to this language. When you add
> a new surface, you reference these constants, not new literals.

The visual idea is a sorcerer's reading room. Panels are panes of dark
tinted glass framed in waxed hardwood; text is parchment ink, etched
crisply against the glass. Selection highlights are gilt — gold ink
underlining the current line in a ledger. Decoration is restrained:
the elegance comes from typography, frame, and tint, not ornaments.

## Color tokens

All colors live in `render/theme.go` as package variables. Never
inline an `rl.NewColor(...)` literal for any surface that already has a
token — extend the palette instead.

**Saturation knob.** The bright accent tokens (HP/MP/enemy bars, status
pills + outlines, the turn-order enemy red, sequence pass/fail, the
timing-bar heading tints, and the reel-symbol hues) are wrapped in
`mute(...)`, which pulls each toward its luminance gray by
`paletteSaturationCut` (currently `0.30`). One knob tones the whole
accent set toward the muted "library" look — raise toward 1 for grayer,
lower for punchier. The earthy base tokens (glass / wood / ink / veil)
are already low-saturation and are NOT routed through `mute` — the
frame-and-parchment identity stays put while the colorful bits calm
down. New bright accents should be `mute`-wrapped too.

### Glass surfaces
Panes composite as `glassBaseWash` (α=88) + the family tint below —
roughly 55-62 % apparent opacity. Deliberately thin: the dungeon glows
through every pane (the BG-era "UI floats over the world" read); the
mandatory text drop shadows are what keep ink legible at this thinness.
- **`glassDeep`** — `rgb(14, 12, 18) α=100` — the canonical pane tint.
  Used as the body fill of every persistent HUD panel and every modal.
- **`glassMid`** — `rgb(22, 18, 24) α=84` — used as the INNER section
  fill of a panel (inset rectangles, list rows). Reads as a slightly
  inset second pane behind the outer one.
- **`glassWarm`** — `rgb(28, 22, 16) α=118` — for the active row / the
  currently-selected actor's card. Same family as `glassDeep` but
  tilted toward amber so the eye drifts to the cursor.
- **`glassDanger`** — `rgb(36, 16, 18) α=112` — for the threatened
  party member during enemy attack timing, and for danger modals
  (confirm-discard, etc.).
- **`veil`** — `rgb(0, 0, 0) α=130` — full-screen modal scrim (NOT
  thinned with the panes — modals still need separation).

### Wood frames
- **`woodDark`** — `rgb(48, 30, 18) α=255` — the outer-edge frame stroke.
  2px stroke, always painted first.
- **`woodMid`** — `rgb(96, 62, 36) α=255` — the body of the frame.
  3-4px band between `woodDark` and `woodLight`.
- **`woodLight`** — `rgb(150, 104, 64) α=255` — the inner highlight, 1px
  sitting against the glass to suggest a varnished bevel.
- **`woodAccent`** — `rgb(184, 140, 92) α=255` — header underline + the
  decorative tick beside panel titles. Same warmth as `woodLight`,
  slightly more saturated.

### Gilt accents (selection / focus)
- **`giltDim`** — `rgb(160, 124, 64) α=255` — resting state of any
  cursor line. Visible but doesn't pulse.
- **`giltBright`** — `rgb(232, 196, 112) α=255` — pulse peak of the
  active actor's frame; gilt edge of a selected target marker.

### Parchment text
- **`inkPrimary`** — `rgb(232, 222, 196) α=255` — body text, panel
  labels, the value text inside HP/MP bars.
- **`inkMuted`** — `rgb(184, 172, 144) α=240` — secondary copy
  (descriptions next to a label, footer hints).
- **`inkDim`** — `rgb(132, 122, 100) α=220` — disabled / unavailable
  text. Reads as faded calligraphy.
- **`inkAccent`** — `rgb(232, 196, 112) α=255` — column headings,
  numeric callouts, "Press Z to..." prompts. Pairs with `giltBright`.

### Health / resource bars
- **`barHPHigh`** — `rgb(116, 200, 132) α=255` — green ink (lawn or
  apothecary).
- **`barHPMid`** — `rgb(224, 184, 88) α=255` — caution amber.
- **`barHPLow`** — `rgb(220, 88, 88) α=255` — danger ink.
- **`barMP`** — `rgb(104, 152, 224) α=255` — sapphire ink.
- **`barEnemyHP`** — `rgb(204, 76, 76) α=255` — claret.
- **`barTrack`** — `rgb(8, 12, 22) α=140` — the empty bar track. Dark
  enough that the fill colour pops at any width.

### Per-status accents
Party-card / status-pill tints, ordered by the priority ladder in
`core.PartyStatus`. Note `statusBurn` is **enemy-side only** (enemies
can burn; party members can't today), so it is not a `PartyStatusKind`.
All semi-glow:
- **`statusDown`** — `rgb(220, 102, 102)` — knocked-out member (top of the ladder)
- **`statusIngested`** — `rgb(200, 132, 220)`
- **`statusWebbed`** — `rgb(180, 140, 220)`
- **`statusConfused`** — `rgb(220, 188, 96)`
- **`statusStun`** — `rgb(232, 220, 120)`
- **`statusSleep`** — `rgb(132, 196, 232)`
- **`statusPoison`** — `rgb(148, 200, 96)`
- **`statusDefending`** — `rgb(132, 196, 255)`
- **`statusStarving`** — `rgb(198, 130, 84)` — famine brown; the food system's only
  mechanical status, and the one that *flickers* (browner than `statusBurn`).
- **`statusBurn`** — `rgb(240, 144, 72)` — *enemy-side only (not a `PartyStatusKind`)*
- **`statusBleed`** — `rgb(200, 56, 56)` — bleed DoT accent (enemy-pill fill/outline)

**Positive-buff accents** (never flicker — a buff isn't a threat):
- **`statusBlessed`** — `rgb(244, 212, 128)` — warm holy gilt (Bless), off `statusStun`'s flatter yellow
- **`statusRegen`** — `rgb(120, 224, 150)` — mint heal-over-time (Renewal), cleaner than `statusPoison`'s olive
- **`statusShielded`** — `rgb(96, 222, 214)` — teal Aegis ward, off the light-blues
- **`statusIceArmor`** — `rgb(186, 226, 248)` — pale icy frost ward, cooler than `statusSleep`
- **`statusGuarding`** — `rgb(150, 172, 214)` — steely azure cover (Warrior's Guard), deeper than `statusDefending`

## Type

### Font
Serif. The HUD font loads the first available of: Constantia, Cambria,
Georgia, Palatino, then falls back to whatever DejaVu/Liberation Serif
the platform exposes. The atlas is baked at **64 pt** for sharp
down-rendering at every standard size below.

If a system has none of these, the engine falls back to raylib's
default bitmap font — readable, but the wood-and-glass feel breaks.
Document this as a known limitation; don't paper over it.

### Sizes — exactly five
| Token | Pixels | Use |
| --- | --- | --- |
| **`FontTiny`** | 17 | Footers, axis tick labels, status pill counters, debug overlay subtleties |
| **`FontSmall`** | 21 | List rows, bar value text, body copy in panels, modal body text |
| **`FontBody`** | 26 | Panel titles, party-card name, action menu rows, level-up stat names |
| **`FontHeading`** | 36 | Modal headers (e.g. "PACK AT (3, 7)", "EDITOR MENU"), pause menu rows |
| **`FontTitle`** | 48 | Battle splash, victory / loss banner, title-screen options |

That's it. **No other sizes are permitted in rendered text.** When you
need a "label between body and heading," choose Body. The atlas is
sharp at every one of these because all five are even-numbered
divisions of the 64 pt bake.

**Documented exception**: the title screen's "CRAWLER" game-name
splash renders at 72 pt. It's the only rendered-once, single-line,
no-competing-content surface in the project; the size is a deliberate
break from the body scale so the game name reads as a typeset masthead
rather than another menu heading. Anything else at a non-standard
size is a bug.

### Letter spacing
- `FontTiny` / `FontSmall`: spacing **1**
- `FontBody`: spacing **1**
- `FontHeading`: spacing **2** (the wider tracking sells the
  "engraved title" feel against a wood frame)
- `FontTitle`: spacing **3**

This ladder is applied **automatically**: `drawTextWithShadow` (and the
helpers built on it — `drawTextCentered`, `drawTextRightAligned`,
`DrawHintBar`, the wrap/fit helpers) resolves spacing from the size
via `canonicalSpacing(size)`, so a plain call site conforms by
construction and draw + measure can never disagree on width. Only
`drawTextWithShadowStyle` takes spacing explicitly — use it when an
ad-hoc tracking is genuinely load-bearing (the timing-bar prompt's 1.5,
the debug overlay's 1.2, animation-scaled splash text) and pair it with
a `MeasureTextEx` at the same spacing.

### Engraved lettering (heading tier and up)
Large text wears a top-lit metal-leaf gradient via
`drawEngravedText(font, text, x, y, size, base)` (exported as
`DrawEngravedText`): heavy +2 drop shadow, full body in `base`, a
bright scissored band over the upper ~45 % (base mixed toward
parchment), a deep band under the lower ~28 % (base mixed toward
black). The bands are re-draws of the same string, so the gradient
rides the letterforms exactly. It tracks at `canonicalSpacing(size)` —
identical measure to plain text — so existing centering math holds;
`drawEngravedTextSpaced` exists for the one load-bearing ad-hoc
tracking (the timing prompt's 1.5).

WHERE: panel/modal headings (`drawPanelHeading`), menu titles + rows
(`drawTitledMenuCard`/`drawMenuRow`, pause/debug rows, title-screen rows), combat verbs
(action-menu header + Attack/Items/Skills), roster enemy names,
level-up stat labels, the timing-bar prompt, Tome member names +
picker titles + item detail names, the door prompt. NOT for FontBody
or smaller (bands collapse into noise), and not for high-count list
rows — it's four passes per string, priced for the heading tier.

### Drop shadow
Every text call routes through `drawTextWithShadow` (1px offset,
`shadowStrong`) or `drawTextWithShadowStyle` for ad-hoc. Never inline
two `rl.DrawTextEx` calls. The shadow is non-negotiable: glass is
translucent; without a shadow, parchment text disappears over a
lit-floor tile.

## Spacing

> The gaps. One source for "how far is content from the edge / from a
> heading / between rows / above the footer," so every surface breathes the
> same instead of each draw site hand-tuning an offset (which is exactly how
> the roster name overlapped its condition line, the battle submenus packed
> their rows edge-to-edge, and three footers drifted to `-30 / -28 / -26`).

### Tokens (`render/theme.go`)
| Token | Value | Meaning |
| --- | --- | --- |
| **`hudContentInsetX`** | 22 | Window padding — edge of a card/panel to its content, both sides. The canonical inset; right edges mirror it. |
| **`uiGapAfterTitle`** | 12 | Breathing space below a heading's **text** before its body begins (added on top of the heading's line height — see helper). |
| **`uiRowH`** | 32 | Height of one interactive row plate. |
| **`uiRowGap`** | 10 | Vertical gap between stacked row plates. |
| **`uiRowPitch`** | 42 | Row center-to-center pitch = `uiRowH + uiRowGap`. Stacked lists step by this. |
| **`uiFooterMargin`** | 14 | Visual gap below a footer hint's glyphs/text to the card's bottom edge. |

### Helpers (`render/layout.go`) — use these, don't hand-roll offsets
- **`bodyBelowHeading(headingTop, fontSize) int32`** — the Y where body
  content starts under a heading. Returns `headingTop + lineHeight(fontSize)
  + uiGapAfterTitle`, so the gap is correct whether the heading is
  `FontHeading` or `FontSmall`. A bare constant can't track the font height —
  that's what made the roster name and the action-menu heading graze their
  bodies.
- **`footerBaselineY(cardBottom, fontSize) int32`** — the Y (glyph/text TOP)
  for a footer hint sitting `uiFooterMargin` above the card's bottom edge.
  Subtracts the font's line height so a `FontTiny` centered footer and a
  `FontSmall` left/picker footer keep the **same** visual gap off the bottom.
  The `drawModalFooterGlyphs` / `…Left` wrappers and the battle action-menu
  footer all route through it.

### Rules
- A card's content rect insets by `hudContentInsetX` on the left **and** right
  (symmetric margins). Row plates that should reach the edge stretch to
  `panelX + width - hudContentInsetX`, not a fixed plate width — a fixed width
  leaves a dead gap when rows start at different x (the icon-gutter main menu
  vs the flush-left submenu list).
- Content under a heading starts at `bodyBelowHeading(...)`. Never a bare
  `headingY + 34`.
- Stacked rows step by `uiRowPitch`. Never a pitch equal to the row height
  (plates touch).
- Footers sit at `footerBaselineY(...)`. Never a per-surface bottom offset.
- A fixed-height panel that hosts a variable-length list (the action menu's
  skill/item submenus) must be tall enough for the realistic max, or scroll.
  When you bound coverage, say so — don't silently clip.

## Surface composition

### Panel (the bread-and-butter wood-framed pane)
```
+--------------------------------------------+ <- woodDark stroke (2px)
| +----------------------------------------+ |
| | +------------------------------------+ | | <- woodMid band (3px)
| | |                                    | | |
| | |  CONTENTS                          | | | <- glassDeep body
| | |                                    | | |
| | +------------------------------------+ | |
| +----------------------------------------+ | <- woodLight inner highlight (1px)
+--------------------------------------------+
```

Owners call `drawCard(x, y, w, h, fill, outline, accent)` — never
`rl.DrawRectangle` for a panel body. The helper handles all four
strokes and the rounded corner radius (4 px — small, so the frame
reads as a hardwood mitre joint, not a modern UI tile). `fill` is one
of the glass tokens (`glassDeep` / `glassMid` / `glassWarm` /
`glassDanger`), `outline` is the outermost stroke color (`woodDark`
for resting, `borderActive` / `borderDanger` to tint the frame for
state), and `accent` is the optional left-spine stripe (pass a
zero-alpha color to skip).

**Carved bevel.** `drawCard` finishes the frame with `drawCardBevel` —
directional light that turns the flat strokes into raised molding:
a warm highlight along the frame's top edge (dimmer down the left),
deep shadow along the bottom (dimmer up the right), and on panes ≥ 56 px
tall the opposite pair at the inner lip (shadow under the top lip,
faint light at the sill) so the glass reads recessed INTO the frame.
Hairlines stop short of the corners; chips below 56×34 skip it. The
corner filigree's outer brackets also carry a 1 px dark offset so they
read as cast-metal braces raised off the frame, not flush gilt paint.

### Heading
```
TITLE GOES HERE
═══════════
```

A small `FontHeading` title, all-caps, followed by a 28-px-wide
`woodAccent` underline 2 px below the baseline. Owners call
`drawPanelHeading(font, text, x, y, accent)` — the helper handles
the tick mark.

### Row (list entry)
- **Resting row**: one `uiRowH` (32 px) tall, glassMid inset (4 px from panel edge),
  no border. (Modal cursor lists use `modalListRowH` = 34; see the Spacing tokens.)
- **Hovered / cursor row**: same panel, but with a `giltDim` left
  spine (3 px × row height) and the row label promoted to
  `inkPrimary` from `inkMuted`.
- **Selected / committed**: `giltBright` left spine + a 1px
  `giltDim` underline along the bottom of the row + the label in
  `inkAccent`. Subtle pulse on the spine alpha (`0.7 → 1.0` over
  ~1.4 s) for the active actor's turn.

Today list owners paint their own row chrome inline (see
`drawActionRow` in battle.go, Items / Skills rows in panels.go); the
modal-cursor highlight goes through `DrawSelectedRow` in
theme_export.go. (A central `drawListRow(rect, state)` helper lived
here briefly and was removed during the audit-perf pass — the
remaining call sites already drift on small details that a single
helper would have flattened poorly.)

### Selection chevron (in-world)
Same `>` ASCII chevron, painted in `giltBright`, drawn via
`drawArrowMarker`. No special chevron-style proliferation.

### Bar (HP / MP / etc.)
- Track: `barTrack`, rounded corners (radius matches half-height).
- Fill: per-resource tone (`barHPHigh` etc.) over the leftmost `pct`
  of the inner width.
- Outline: 1 px `woodLight`.
- Label inside the bar: `FontTiny` at fixed `inkPrimary`, drop shadow.
- Value text on the right edge: `FontSmall` `inkPrimary`, drop
  shadow, padded `barValuePadRight` from the right edge.

Owners call `drawBar(font, x, y, w, h, label, value, max, fill, muted)`.

**Live gauges.** Combat-facing bars (the party ribbon's HP) route through
`drawBarLive(font, key, ...)` instead — same body plus three treatments:
- **Damage ghost** — when the value drops, the lost slice holds for a beat
  in `barGhostHot` (hot parchment-gold) then drains into the fill edge
  (`render/barghost.go`; render-side state keyed by the caller's string
  identity, cleared with the VFX pools on scene reset).
- **Heartbeat** — at ≤ 25 % the fill breathes at the status-flicker rate
  and the value text turns `barHPLow`.
- **Meniscus** — a bright hairline rides the fill's leading edge so the
  level reads as liquid in the glass tube.

Dashboard bars (the Tome's Stats tab) stay on plain `drawBar` — the juice
plays only where the stakes are. MP bars stay static everywhere (spends
are deliberate, not threats).

### Timing-bar minigames (combat)
The timed-hit bar above the party ribbon is its own minimal HUD family
(NOT a wood-framed panel — a transient strip). All variants share the
`timing*` accent tokens (`timingTrackColor` fill, `timingCursorColor`,
`timingHeldColor`, `seqOkColor` / `seqFailColor`) and a `FontHeading`
prompt drawn via `drawTimingHeading` (flips to `FontTitle` + the grade
tint during the resolve flash). Six kinds dispatch in `drawTimingBar`:
Press, Charge, Sequence, **Reels** (one framed-glass cell per spinner;
the locked cell gilds; symbols are `mute`-d hues with a dark rim so they
read etched over the translucent glass), **Recall** (sequence arrows
shown then hidden), and **Overcharge** (the Charge bar; its post-peak
band is the overload zone).

When you add a bar variant, reuse the shared row helpers
(`arrowRowLayout`, `drawSequenceCursorUnderline`,
`drawDwindlingTimerStrip`, `fadeForFlash`) and the `bar*` / `arrowSize*`
/ `timerStrip*` consts — don't re-derive the geometry or re-spell the
alphas (the sequence/recall bars drifted on copied literals until those
were extracted).

## Behaviour standards

### Input convention — gamepad-first (locked)
This is a **gamepad-first game**: the controller is the primary input;
keyboard/mouse are secondary. Every surface you add MUST be fully
operable by controller alone — nothing reachable only by mouse or by an
unbound key. All reads route through the `input` package; never inline
`rl.IsKeyPressed` / `rl.IsGamepadButton*` / `rl.GetMouse*` at a call
site.

- **Confirm**: gamepad `A` / `Cross`; keyboard `Z` / `Space` / `Enter`.
- **Back / cancel**: gamepad `B` / `Circle`; keyboard `X` / `Esc`.
- **Navigate**: D-pad / left stick (keyboard arrows / `WASD`).
- **Open Tome (panels)**: `Y` / `Triangle` or the middle button;
  keyboard `I` (or the per-tab letters `C`/`E`/`I`/`K`/`M`).
- **Page tabs / cycle prev-next target**: `L1`/`L2` back, `R1`/`R2`
  forward (keyboard `Tab` / `Shift+Tab` for tabs; arrows for targets).
- **Pause**: `Start` / `Options` (keyboard `P` / `Esc`).
- **Free-look**: right stick, or right-mouse drag.
- **Use / cast** (panels): `Square` / `X` (keyboard `F`); in the editor
  `Square` / `X` is **erase**. `L3` / `R3` are the only intentionally
  unbound buttons.

Never invent a new keybinding or button combo for a new surface — route
through `input.ConfirmPressed` / `input.BackPressed` / `input.CursorUpDown`
(add a predicate in `input/input.go` if the gesture is genuinely new).

### Modal scaffolding
Every modal calls `drawModalScaffold(font, w, h, heading)`:
- Paints the full-screen `veil`.
- Draws the wood-framed panel centered (soft-clamped to the window).
- Renders the heading (`drawHeading`).
- Returns the card rect for the caller to paint into.

The footer hint is drawn separately by the owner via `drawModalFooterGlyphs`
at `footerBaselineY` (see Spacing and Footer hint below); the scaffold does
not carve out footer space.

### Footer hint
Every modal has a footer line of control affordances. The game is
**gamepad-first**, so hints render as **controller button GLYPHS**, never
spelled-out keys — `[A] Confirm   [B] Back   [↕] Move`, not
`Z confirm   X cancel`. (Keyboard glyphs are a later device-switch pass;
today we draw the controller set only.)

Owners build a `[]render.HintSeg` — each seg is one glyph (or two, e.g.
`[LB][RB]`) plus its action word — and call `DrawHintBar(font, segs, cx,
y, size)` (centred, with the same diamond termini the old text footer
stitched) or `DrawHintBarLeft(font, segs, x, y, size)`. Modal owners use
the `drawModalFooterGlyphs` / `drawModalFooterGlyphsLeft` wrappers, which
own the card-geometry math. (The pure-text footer helpers were removed —
all footers are glyph footers now.) In-world
press cues (chest / crystal / door) use `drawGlyphPrompt(font, glyph,
verb, cx, y, size)` — brighter, no termini. Build segs with the terse
`render.Hint("Verb", render.GlyphA)` constructor.

### Controller glyphs
`render/glyphs.go` draws the on-screen button icons procedurally (no PNG
assets) in the library palette — the glyph hues + body/rim/ink tokens
live in `theme.go` (`glyphAColor` … `glyphInk`). Style is **dark raised
button + colored letter** (reads cleanest over dark glass): face buttons
A/B/X/Y are a dark disc + colored ring + the letter in the button's hue
(A green / B red / X blue / Y amber, all `mute()`-wrapped); LB/RB are
bumper pills; Start/Select are the menu / view pictograms; the d-pad is a
rounded tile with the active direction(s) chevroned in `giltBright`. The
`InputGlyph` enum + `GlyphUpDown` / `GlyphLeftRight` paired directions are
the vocabulary — extend the enum + the `drawInputGlyph` switch together.
Glyphs keep full color (the icon is the point); only the trailing label
rides the footer's dim ink.

### Static ornament dialect
The carved-cabinet details share a small vocabulary — reuse these, don't
invent siblings:
- **Ruled page** — the combat log paints a whisper hairline
  (`inkDim` ≤ 0.13) at the bottom of every line slot, bottom-anchored to
  the same footing as the text so entries sit ON their rule.
- **Watermark** — a surface owned by an actor may ghost that actor's
  sigil LARGE into its lower glass (`drawClassGlyph` at α ≈ 0.11,
  fainter when downed). Content always layers over it; it reads as
  depth in the glass, never as information.
- **Sequence thread** — an ordered forecast (turn panel) stitches its
  row markers with a 1 px `inkDim` strand ending in a pip, so the list
  reads as one lineage rather than disconnected chips.
- **Header rule** — a chrome strip that owns the body below it (the
  Tome's location/gold band) underlines itself with a `woodAccent`
  hairline ending in diamond pips, matching the panel-heading underline.
- **Blank ledger page** — empty list states route through
  `drawEmptyLedgerNote(font, body, text, sub)`: a dim gilt fleuron with
  flanking hairlines over the centred message (+ optional dimmer hint
  line). No more bare left-aligned "nothing here" text.

### Pulse / breathing
- Active-actor pulse: `0.70 + 0.30·sin(t·π·1.4)` on alpha.
- Cursor / selection halo: `0.60 + 0.40·sin(t·π·2.0)` on alpha.
- Status flicker: `0.65 + 0.35·sin(t·π·2.6)` on alpha.
- Damage flash: handled by `core.FlashTint`, peaks at `0.86` for
  one `FlashDuration` then decays.
- Hit knockback: handled by `core.KnockbackOffset`, peak
  `HitKnockbackDist` over `HitKnockbackDuration`.

Never invent a new pulse frequency for a one-off panel.

### Light sweeps (position, not alpha)
Two sanctioned wall-clock sweeps — candlelight moving across a surface,
not a pulse. Both are slow, shared-clock (every instance on screen rides
the same light), and have a dark beat between passes:
- **Selection-plate sheen** — `drawRowSheen`, every `DrawSelectedRow`
  plate, one pass per `rowSheenPeriod` (3.8 s), gilt at ≤ 0.13 alpha,
  scissor-clipped to the plate.
- **Masthead shimmer** — the title-screen wordmark redrawn in cream
  through a sweeping scissor band, one pass per `titleSheenPeriod`
  (5.6 s). The glint rides the letterforms, never a rectangle over them.

### Danger vignette (combat)
While an enemy is mid-swing (`BattleEnemyTiming`), the screen edges
breathe claret: four shallow `borderDanger` gradients
(`render/vignette.go`, depth 16 % of the short dimension, peak alpha 34,
status-flicker rhythm), drawn over the world and under every HUD pane.
Peripheral pressure for the defend window — the same beat the rumble and
the red incoming-attack marker mark. No other phase tints the frame.

## How to extend this document

When you add a new surface, FIRST write its row in here. The MD
proves the system; the code is the implementation. If the MD says
something the code doesn't, fix the code. If the code says
something the MD doesn't, write it down here before the next pass.
