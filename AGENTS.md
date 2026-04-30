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
- `internal/app/run.go`: window setup, borderless windowed fullscreen boot, resource lifetime, main loop.
- `internal/app/core/`: shared data and pure-ish helpers.
- `internal/app/core/config.go`: global constants, direction/action enums, RNG.
- `internal/app/core/types.go`: shared structs for player, map state, battle state, enemies, and party members.
- `internal/app/core/state.go`: initial game state, party members, and rat factory.
- `internal/app/core/map.go`: dungeon layout, map queries, rat placement helpers.
- `internal/app/core/util.go`: math, easing, direction, color, clamp, flash, and bump helpers.
- `internal/app/explore/`: non-battle game input and movement.
- `internal/app/explore/movement.go`: pause menu input, tile movement, free-look snapback, step/turn animations, adjacent encounter checks.
- `internal/app/battle/`: turn-based combat state machine and rules.
- `internal/app/battle/battle.go`: battle lifecycle, phase transitions, battle log updates, transient combat effects.
- `internal/app/battle/menu.go`: combat menu input, enemy/party target cycling, class skill metadata, confirm/back controls.
- `internal/app/battle/actions.go`: Attack, Swipe, Prayer, Steal, Firebolt, burn ticks, damage resolution, rat attacks.
- `internal/app/battle/helpers.go`: living-count helpers, party helpers, turn forecast generation.
- `internal/app/render/`: raylib drawing, procedural assets, and HUD.
- `internal/app/render/world.go`: camera, screen-filling sky background, world drawing, enemy/party billboards, target markers, battle formation positioning.
- `internal/app/render/hud.go`: top-level HUD routing and exploration party totals.
- `internal/app/render/battle.go`: battle panel, combat log, action menu, target tooltip, battle splash.
- `internal/app/render/party.go`: bottom party stat cards and HP/MP bars.
- `internal/app/render/turns.go`: color-coded turn order panel.
- `internal/app/render/minimap.go`: auto-scrolling minimap and facing arrow.
- `internal/app/render/menu.go`: pause menu.
- `internal/app/render/resources.go`: procedural resource loading, font loading, HUD text helpers, rounded panel helpers.
- `internal/app/render/textures.go`: procedural wall/floor/sky textures and rat/party sprite pixels.

## Gameplay Notes

- Movement is tile-based with short animation. `W/S` step, `A/D` strafe, `Q/E` or arrows turn.
- Right-click drag free-look snaps back on release.
- Battles start when the player is adjacent to a live rat; if needed, the player rotates to face it first.
- Battle input:
  - Confirm: `Space`, `Enter`, or `Z`
  - Back: `Esc` or `X`
  - Target/menu movement: arrows, `W/S`, `A/D`, `Tab` where applicable
- Party classes are intentionally named by class only: `Warrior`, `Cleric`, `Thief`, `Wizard`.
- Current class skills:
  - Warrior: `Swipe`, AoE damage.
  - Cleric: `Prayer`, party-targeted single heal.
  - Thief: `Steal`, 70% chance from enemies with an item. Rats carry `Morsel of Cheese`.
  - Wizard: `Firebolt`, single-target damage with a high burn chance for 3-5 turns. Burns do not stack.

## Implementation Notes

- Keep behavior repo-native and procedural unless asked otherwise. Most visual assets are generated in `textures.go`.
- Package boundaries are real Go package boundaries now. Prefer adding behavior in the relevant directory over creating prefixed files in `internal/app`.
- Use the existing raylib drawing style and helper functions before introducing a new rendering abstraction.
- Keep HUD surfaces rounded and slightly translucent; preserve readable borders and text shadows.
- Use `setBattleMessage` for real combat log events. Use `setBattleStatus` for transient prompts like target selection.
- Enemy death uses `deathFade`; do not immediately remove dead enemies from battle visuals if the fade is still active.
- Target tooltip text is centered, with wound-state colors from `enemyHealthColor`.
- Party stat cards are pinned near the screen bottom and horizontally follow the projected party sprite positions.

## Caution

- Avoid broad refactors while tuning gameplay feel. Most requests are visual/combat iteration and should stay scoped.
