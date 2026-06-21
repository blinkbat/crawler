# Asset Pipeline — hand-drawn sprite → in-game billboard

How a drawing on paper becomes a tuned enemy/party billboard. The engine art is
otherwise procedural (`render/textures.go`); this pipeline is the exception for
authored PNGs under `maps/sprites/` (see AGENTS.md "Conventions").

## The flow

1. **Draw + scan.** Draw the subject with a dark outline (and/or saturated fill)
   on near-neutral paper (white/grey). Scan or photograph it. The outline +
   chroma is what the extractor keys on — see the script header for when this
   does and doesn't work (greyscale/pencil art has no chroma signal and won't
   extract cleanly).

2. **Extract a transparent PNG.** Run `tools/extract_sprite.py` to chroma-key the
   paper away and tight-crop to a transparent PNG.

   ```
   pip install opencv-python-headless numpy
   python tools/extract_sprite.py drawing.png --inspect      # read S/V stats first
   python tools/extract_sprite.py drawing.png -o rat.png     # then extract
   python tools/extract_sprite.py scans/*.png --batch out/   # or batch a sheet
   ```

   Always `--inspect` a new source first and set `--sat` / `--val-guard` from the
   printed distribution — the defaults were tuned for one specific marker-on-grey
   source. The script's docstring explains every algorithm step; don't drop one
   without reading why it's there.

3. **Place it as the sprite.** The sprite for a foe/class loads from
   `maps/sprites/<slug>.png` at boot, where `<slug>` is `core.EnemySlug(kind)` for
   foes or `core.PartyClassSlug(class)` for party members (e.g. `feral_rat.png`,
   `warrior.png`). Two ways to get the PNG there:

   - **Editor import (preferred):** open the **Foe Visualizer** or **Party
     Visualizer** in the map editor, cycle to the kind, and **drag-drop the PNG
     onto the window**. The editor writes `<slug>.png`, backs up any prior art to
     `<slug>.png.bak`, runs a transparency-matte safety net (re-keys an opaque
     background to alpha if the PNG lost its transparency on export), and reloads
     the texture live. A procedural-only kind is promoted to an authored PNG on
     first import.
   - **By hand:** drop the file at `maps/sprites/<slug>.png` directly. Picked up
     on the next launch.

4. **Tune in the editor.** In the Visualizer:
   - **Layout tab** — placement/size/shadow/cursor/glyph/particle offsets + tint
     (`EnemyVisualOverride`). Applied at draw time, so changes are live.
   - **Asset tab** — non-destructive image **FX**: Pixelate, Brightness,
     Contrast, Posterize, Saturate, Dither, GameBoy. These are *not* baked into
     the PNG; they're stored as the override and re-baked onto the pristine art
     at texture-build time. **Revert** clears them; a dropped PNG replaces the art.

   **Save** writes the tuning to `maps/sprites/visuals.json` (foes) or
   `partyvisuals.json` (party), keyed by slug, **and** re-bakes the FX into the
   live display texture — so the change applies in-session (editor world + the
   in-process playtest) without a restart. A *separately* launched game `.exe`
   reads the JSON at its own boot, so that one needs its own restart.

## Where it lives

- `tools/extract_sprite.py` — the background-removal script (chroma key + flood
  fill + component filter + erode-inside-outline + crop).
- `maps/sprites/<slug>.png` — the authored art (`<slug>.png.bak` = one-step undo
  for the last bake/import/restore).
- `maps/sprites/visuals.json` / `partyvisuals.json` — per-kind override tuning
  (placement, tint, FX). Code defaults stand for any kind absent from the file.
- Editor authoring: `internal/app/editor/foeview.go` (Foe), `partyview.go`
  (Party). Image edits + texture (re)derivation: `internal/app/render/spriteedit.go`.

## Notes / gotchas

- **Extraction is chroma-based.** A low-chroma subject (pencil/greyscale,
  white-on-white) can't be separated from the paper — use the layered source or a
  hand mask instead. No threshold tuning fixes a missing chroma signal.
- **Transparency must survive export.** A PNG flattened with an opaque matte
  ("export lost transparency") would tint as a solid rectangle in-engine. The
  editor import re-keys an all-opaque background to alpha as a safety net, but
  `extract_sprite.py` output already carries real alpha, so it's left untouched.
- **FX vs. the retro screen filter are separate.** The Asset-tab FX are per-asset
  and baked into the sprite texture. The Debug ▸ Retro Filters screen pass is a
  global post-process that, by default, hits **only the environment** — sprites
  stay crisp so their authored FX read through (toggle "Filter Sprites" to crunch
  them with the world).
- **Sprite size/grounding is per-kind.** After importing new art, expect to
  retune Size/Y-Offset/Shadow in the Layout tab — billboards are center-anchored
  and field vs. battle use different base heights (see AGENTS.md "Billboard
  grounding").
