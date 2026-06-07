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
- **`glassDeep`** — `rgb(14, 12, 18) α=210` — the canonical pane tint.
  Used as the body fill of every persistent HUD panel and every modal.
  Dark enough to read parchment text against; transparent enough that
  the world bleeds through faintly.
- **`glassMid`** — `rgb(22, 18, 24) α=200` — used as the INNER section
  fill of a panel (inset rectangles, list rows). Reads as a slightly
  inset second pane behind the outer one.
- **`glassWarm`** — `rgb(28, 22, 16) α=200` — for the active row / the
  currently-selected actor's card. Same alpha family as `glassDeep`
  but tilted toward amber so the eye drifts to the cursor.
- **`glassDanger`** — `rgb(36, 16, 18) α=200` — for the threatened
  party member during enemy attack timing, and for danger modals
  (confirm-discard, etc.).
- **`veil`** — `rgb(0, 0, 0) α=140` — full-screen modal scrim.

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
- **`statusBurn`** — `rgb(240, 144, 72)` — *enemy-side only (not a `PartyStatusKind`)*

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
| **`FontTiny`** | 13 | Footers, axis tick labels, status pill counters, debug overlay subtleties |
| **`FontSmall`** | 16 | List rows, bar value text, body copy in panels, modal body text |
| **`FontBody`** | 20 | Panel titles, party-card name, action menu rows, level-up stat names |
| **`FontHeading`** | 26 | Modal headers (e.g. "PACK AT (3, 7)", "EDITOR MENU"), pause menu rows |
| **`FontTitle`** | 36 | Battle splash, victory / loss banner, title-screen options |

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

### Drop shadow
Every text call routes through `drawTextWithShadow` (1px offset,
`shadowStrong`) or `drawTextWithShadowStyle` for ad-hoc. Never inline
two `rl.DrawTextEx` calls. The shadow is non-negotiable: glass is
translucent; without a shadow, parchment text disappears over a
lit-floor tile.

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
- **Resting row**: 36 px tall, glassMid inset (4 px from panel edge),
  no border.
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
- `Square` / `X` and `L3` / `R3` are intentionally unbound.

Never invent a new keybinding or button combo for a new surface — route
through `input.ConfirmPressed` / `input.BackPressed` / `input.CursorUpDown`
(add a predicate in `input/input.go` if the gesture is genuinely new).

### Modal scaffolding
Every modal calls `drawModalScaffold(font, w, h, heading)`:
- Paints the full-screen `veil`.
- Draws the wood-framed panel centered.
- Renders the heading using `drawPanelHeading`.
- Returns the body rect for the caller to paint into.
- Reserves the bottom 28 px for the footer hint, drawn via
  `DrawFooterHint`.

### Footer hint
Every modal has a footer line such as
`Z confirm   X cancel   Up/Down navigate`.
Painted in `FontTiny` `inkDim`, centred, never customised per modal.
Owners call `DrawFooterHint(font, text, cx, y, FontTiny)`.

### Pulse / breathing
- Active-actor pulse: `0.70 + 0.30·sin(t·π·1.4)` on alpha.
- Cursor / selection halo: `0.60 + 0.40·sin(t·π·2.0)` on alpha.
- Status flicker: `0.65 + 0.35·sin(t·π·2.6)` on alpha.
- Damage flash: handled by `core.FlashTint`, peaks at `0.86` for
  one `FlashDuration` then decays.
- Hit knockback: handled by `core.KnockbackOffset`, peak
  `HitKnockbackDist` over `HitKnockbackDuration`.

Never invent a new pulse frequency for a one-off panel.

## How to extend this document

When you add a new surface, FIRST write its row in here. The MD
proves the system; the code is the implementation. If the MD says
something the code doesn't, fix the code. If the code says
something the MD doesn't, write it down here before the next pass.
