# Auditing

When auditing OR recapping, always, ALWAYS present findings in a FLAT, NUMBERED list for easy discussion.

# Crawler Codebase Map

This is a Go/raylib dungeon crawler prototype. The runtime package is `internal/app`, with `main.go` only calling `app.Run()`.

## Commands

- Test: `go test ./...`
- Build: `go build -o .\.codex-build\crawler-3d.exe .`
- Run from source: `go run .`
- The project currently relies on `raylib.dll` being available beside the executable or in the working directory.

This workspace is Windows-first. Prefer bash if it is installed, but do not block on it; PowerShell is the default shell here.

## File Layout

- `main.go`: tiny entrypoint; calls `app.Run()`.
- `internal/app/run.go`: window setup, borderless windowed fullscreen boot, resource lifetime, scene routing (title / adventure / editor), main loop.
- `internal/app/core/`: shared data and pure-ish helpers.
- `internal/app/core/config.go`: global constants, direction/action/phase enums, timing-bar tuning, RNG.
- `internal/app/core/types.go`: shared structs for player, map state, battle state, enemies, and party members.
- `internal/app/core/state.go`: initial game state and party members.
- `internal/app/core/areas.go`: area / map-file conversion, material + facing names, on-disk path helpers.
- `internal/app/core/map.go`: area layout queries (`WallAt`, `BlockedAt`, `FloorAt`, `TileAt`, `InBounds`) and enemy placement helpers.
- `internal/app/core/mapfile/`: on-disk `.map` format — parse/encode/load/save, layer slot dispatch, blank-layer seeding.
- `internal/app/core/party.go`: party class definitions, skill registry, damage / heal / burn-duration formulas.
- `internal/app/core/selectors.go`: read-only party/battle selectors, turn forecast generation, wrap-aware living-member walk.
- `internal/app/core/condition.go`: enemy wound-state thresholds and labels.
- `internal/app/core/enemies.go`: enemy kind enum, per-kind definitions (stats, tier, carried item), and lookup helpers.
- `internal/app/core/items.go`: item registry, inventory add/consume/empty helpers, kind-by-name lookup.
- `internal/app/core/timing.go`: timed-hit minigame state machine (press / charge / sequence), grade dispatch, damage/heal/defense scaling.
- `internal/app/core/daycycle.go`: time-of-day phase enum, phase-at-step math, phase labels.
- `internal/app/core/util.go`: math, easing, direction, color, clamp, flash, and bump helpers.
- `internal/app/input/`: shared key semantics for confirm/back/menu/targeting/exploration controls; analog-stick edge detection.
- `internal/app/explore/`: non-battle game input and movement.
- `internal/app/explore/movement.go`: pause menu input (P / Esc / gamepad Start), tile movement (props block), smooth free-look return, step/turn animations, adjacent encounter checks.
- `internal/app/battle/`: turn-based combat state machine and rules.
- `internal/app/battle/battle.go`: battle lifecycle, phase transitions, mixed-initiative round queue, battle log updates, transient combat effects.
- `internal/app/battle/menu.go`: combat menu input, action / item / target cycling.
- `internal/app/battle/actions.go`: Attack, Swipe, Prayer, Steal, Firebolt, burn ticks, damage resolution, enemy attack target round-robin.
- `internal/app/battle/helpers.go`: battle encounter group selection (Manhattan + diagonal LOS).
- `internal/app/render/`: raylib drawing, procedural assets, and HUD.
- `internal/app/render/world.go`: camera, screen-filling sky background, world drawing, enemy/party billboards, target markers, battle formation positioning.
- `internal/app/render/hud.go`: top-level HUD routing and exploration party totals.
- `internal/app/render/battle.go`: enemy roster, combat log, action menu, item picker, battle splash.
- `internal/app/render/party.go`: bottom party stat cards and HP/MP bars.
- `internal/app/render/turns.go`: color-coded turn order panel.
- `internal/app/render/minimap.go`: auto-scrolling minimap, facing arrow, day/night strip.
- `internal/app/render/menu.go`: pause menu overlay.
- `internal/app/render/classes.go`: per-party-class presentation (turn color, victory dance motion).
- `internal/app/render/daycycle.go`: time-of-day lighting profiles + interpolation, sky tinting.
- `internal/app/render/lighting.go`: lighting shader load / uniforms / per-area profiles.
- `internal/app/render/timing.go`: timed-hit bar rendering (press / charge / sequence bars, flash hold, quality popup).
- `internal/app/render/resources.go`: procedural resource loading, area material models, font loading.
- `internal/app/render/theme.go`: HUD color tokens, rounded panel helpers, shared text shadows.
- `internal/app/render/theme_export.go`: theme accessor for non-render packages (editor, title).
- `internal/app/render/textures.go`: procedural area wall/floor/sky textures and rat/bat/party sprite pixels.
- `internal/app/render/models.go`: procedural mesh construction for trees, boulders, bushes, and mushroom props.
- `internal/app/editor/`: in-game map authoring tool (Walls / Floor / Decor / Props / Entities layers).
- `internal/app/title/`: launch screen — pick Adventure (map picker) / Editor / Quit.

## Gameplay Notes

- Movement is tile-based with short animation. `W/S` step, `A/D` strafe, `Q/E` or arrows turn. Walls and prop tiles (trees / boulders / large bushes) both block.
- Right-click drag free-look recenters smoothly on release.
- Pause menu: `P`, `Esc` (only outside battle, so Esc keeps working as "back / cancel target" in combat), or gamepad Start.
- Battles start when the player is adjacent to a live enemy (rats or bats); if needed, the player rotates to face it first. The encounter pulls in any other enemies within Manhattan distance 2 that have a clear LOS.
- Battle input:
  - Confirm: `Space`, `Enter`, or `Z`
  - Back: `Esc` or `X`
  - Target/menu movement: arrows, `W/S`, `A/D`, `Tab` where applicable
- Mixed initiative: party + enemies are sorted by SPD into a single per-round queue. Burn ticks fire at the start of the burning actor's own turn; Poison ticks at the END of the poisoned actor's turn (after their action lands).
- Enemies: `rat`, `bat`, `diseased_rat` (tier-3 rat variant, 60% chance to inflict Poison on a landed bite; carries no loot).
- Status effects: Burn (enemy-side, from Firebolt — flat tick at start of turn, 2–3 turns) and Poison (party-side, from Diseased Rat — flat tick at END of turn, 3–5 turns). Neither stacks onto an already-affected target.
- Party classes are intentionally named by class only: `Warrior`, `Cleric`, `Thief`, `Wizard`.
- Current class skills:
  - Warrior: `Swipe`, AoE damage.
  - Cleric: `Prayer`, party-targeted single heal.
  - Thief: `Steal`, base 40% chance scaled by DEX and timing quality. Rats carry `Morsel of Cheese`; bats carry `Bat Jerky`.
  - Wizard: `Firebolt`, single-target damage with a 45% base burn chance for 2-3 turns. Burns do not stack on an already-burning target.

## Implementation Notes

- Keep behavior repo-native and procedural unless asked otherwise. Most visual assets are generated in `textures.go`.
- Package boundaries are real Go package boundaries now. Prefer adding behavior in the relevant directory over creating prefixed files in `internal/app`.
- Use the existing raylib drawing style and helper functions before introducing a new rendering abstraction.
- Keep HUD surfaces rounded and slightly translucent; preserve readable borders and text shadows.
- Use `setBattleMessage` for real combat log events. Use `setBattleStatus` for transient prompts like target selection.
- Enemy death uses `deathFade`; do not immediately remove dead enemies from battle visuals if the fade is still active.
- Target tooltip text is centered, with wound-state labels from `core.EnemyConditionFor`.
- Party stat cards are pinned near the screen bottom and horizontally follow the projected party sprite positions.

## Caution

- Avoid broad refactors while tuning gameplay feel. Most requests are visual/combat iteration and should stay scoped.
