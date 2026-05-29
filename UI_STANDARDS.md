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
- **`barTrack`** — `rgb(10, 8, 14) α=220` — the empty bar track. Dark
  enough that the fill colour pops at any width.

### Per-status accents
Match the status priority ladder in `core.PartyStatus`. All semi-glow:
- **`statusPoison`** — `rgb(148, 200, 96)`
- **`statusBurn`** — `rgb(240, 144, 72)`
- **`statusSleep`** — `rgb(132, 196, 232)`
- **`statusStun`** — `rgb(232, 220, 120)`
- **`statusBound`** — `rgb(180, 140, 220)`
- **`statusConfused`** — `rgb(220, 188, 96)`
- **`statusIngested`** — `rgb(200, 132, 220)`
- **`statusDefending`** — `rgb(132, 196, 255)`

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

## Behaviour standards

### Input convention (already locked)
- **Confirm**: keyboard `Z` / `Space` / `Enter`, gamepad `A`
  (Xbox) / `Cross` (PS).
- **Back**: keyboard `X` / `Esc`, gamepad `B` / `Circle`.
- **Cycle prev/next list / target**: shoulder buttons or
  `Q`/`E` on keyboard.
- **Open Tome (panels)**: `I` / middle gamepad button.

Never invent a new keybinding for a new modal — route through
`input.ConfirmPressed` / `input.BackPressed` / `input.CursorUpDown`.

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
