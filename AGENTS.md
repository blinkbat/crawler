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
- `internal/app/core/types.go`: shared structs for player, map state, battle state, enemies, party members (with `Armor`, `SleepTurns`, `Ingested` + `IngestedBy`, `Level`, `XP`, `PendingLevelUps`), and chests (`Chest` runtime, `ChestSpawn` authored). `GameState` also tracks `LevelUpOpen`/`LevelUpMember`/`LevelUpStat` for the post-battle stat-spend modal and `StatsScreenOpen` for the read-only Party Stats overlay.
- `internal/app/core/state.go`: initial game state, party members (Level seeded to `BaseLevel`), runtime chest placement (`placeChests`).
- `internal/app/core/areas.go`: area / map-file conversion (5 grid layers + pack + chest spawns), material + facing names, on-disk path helpers.
- `internal/app/core/map.go`: area layout queries (`WallAt`, `CeilingAt`, `BlockedAt`, `FloorAt`, `TileAt`, `InBounds`), chest tile lookups (`ChestIndexAt`, `AdjacentChestIndex`, `AdjacentInteractableChestIndex`), and enemy placement helpers.
- `internal/app/core/mapfile/`: on-disk `.map` format — five grid layers (`walls`/`floor`/`decor`/`props`/`ceiling`), enemy + chest spawn sections, parse/encode/load/save, layer slot dispatch, blank-layer seeding. Older files without `ceiling:` parse fine and get a blank ceiling layer.
- `internal/app/core/party.go`: party class definitions, skill registry (now with `SkillTag` for Phys/Magic/Heal/Buff, `PlayerCastable` flag separating menu-eligible skills from enemy-only ones, and `SkillEffect.AppliesIngest` carrying the mantrap's swallow status), damage / heal / burn / sleep-duration formulas, `ApplyArmor` damp, `XPForLevel` / `AddXP` / `SpendStatPoint` / `FirstPendingLevelUp` for the leveling system, the table-driven `statTable` powering `StatLabel`/`StatValue`, and `PlayerCastableSkills` (used by battle's init to assert handler coverage).
- `internal/app/core/selectors.go`: read-only party/battle selectors, turn forecast generation, wrap-aware living-member walk, `PackXPValue` and `AwardBattleXP`, plus the "available" variants (`PartyMemberAvailable`, `ActivePartyCount`, `WrapNextAvailablePartyMember`, `AvailablePartyTargets`, `FirstAvailablePartyMember`) that exclude ingested members and `MantrapHasPrey` / `ReleaseIngestedBy` / `ReleaseAllIngested` for the Ingest status lifecycle.
- `internal/app/core/condition.go`: enemy wound-state thresholds and labels.
- `internal/app/core/enemies.go`: enemy kind enum (Rat, Bat, DiseasedRat, Goblin, GoblinMage, Amoeba, VenusMantrap), per-kind definitions (stats, tier, carried item, `Armor`, `XPValue`, `Skills`, `SkillCastChance`, `SpellPower`), and lookup helpers.
- `internal/app/core/items.go`: item registry, inventory add/consume/empty helpers, kind-by-name lookup.
- `internal/app/core/timing.go`: timed-hit minigame state machine (press / charge / sequence), grade dispatch, damage/heal/defense scaling.
- `internal/app/core/daycycle.go`: time-of-day phase enum, phase-at-step math, phase labels.
- `internal/app/core/util.go`: math, easing, direction, color, clamp, flash, and bump helpers.
- `internal/app/input/`: shared key semantics for confirm/back/menu/targeting/exploration controls; analog-stick edge detection.
- `internal/app/explore/`: non-battle game input and movement.
- `internal/app/explore/movement.go`: pause menu input (P / Esc / gamepad Start), tile movement (props + chests block), smooth free-look return, step/turn animations, adjacent encounter + chest checks (`tryOpenAdjacentChest`). The Update gate routes level-up modal → Party Stats overlay → chest modal → pause menu → battle / explore.
- `internal/app/explore/chest.go`: chest-modal input loop — Up/Down picks a row, Confirm takes one or all, Esc closes; emptied chests mark `Looted` so the lid renders open.
- `internal/app/explore/levelup.go`: post-battle stat-spend modal input loop. Walks members with pending points via `advanceLevelUpMember`; no Esc-out (every point must be allocated before exploration resumes).
- `internal/app/battle/`: turn-based combat state machine and rules.
- `internal/app/battle/battle.go`: battle lifecycle, phase transitions, mixed-initiative round queue, battle log updates, transient combat effects. `tickSleepAtTurnStart` skips the sleeper's turn; `enemyAIPickSkill` rolls `SkillCastChance` to pick between melee and a Skill from `EnemyDefinition.Skills`; `resolveEnemySpell` dispatches enemy Firebolt / Sleep (magic-tagged so armor is bypassed). XP award + level-up modal trigger fire in `winBattle` / `leaveBattle`.
- `internal/app/battle/menu.go`: combat menu input, action / item / target cycling.
- `internal/app/battle/actions.go`: Attack, Swipe, Prayer, Steal, Firebolt, burn ticks, damage resolution (armor-aware via `core.ApplyArmor`; any damage > 0 wakes a sleeping target), enemy attack target round-robin.
- `internal/app/render/`: raylib drawing, procedural assets, and HUD.
- `internal/app/render/world.go`: camera, screen-filling sky background, world drawing (5 grid layers including ceiling slabs), enemy/party billboards, target markers, battle formation positioning.
- `internal/app/render/chest.go`: chest billboards (body + lid + lockplate), adjacent-chest prompt (`DrawChestPrompt`), and the chest-open modal (`DrawChestModal`).
- `internal/app/render/levelup.go`: post-battle level-up modal (`DrawLevelUpModal`) and the read-only Party Stats overlay (`DrawPartyStatsScreen`) — both read the table-driven `core.StatLabel` / `core.StatValue`.
- `internal/app/render/hud.go`: top-level HUD routing and exploration party totals.
- `internal/app/render/battle.go`: enemy roster, combat log, action menu, item picker, battle splash.
- `internal/app/render/party.go`: bottom party stat cards and HP/MP bars.
- `internal/app/render/turns.go`: color-coded turn order panel.
- `internal/app/render/minimap.go`: auto-scrolling minimap, facing arrow, day/night strip.
- `internal/app/render/menu.go`: pause menu overlay.
- `internal/app/render/classes.go`: per-party-class presentation (turn color, victory dance motion).
- `internal/app/render/daycycle.go`: time-of-day lighting profiles + interpolation, sky tinting.
- `internal/app/render/lighting.go`: lighting shader load / uniforms / per-area profiles.
- `internal/app/render/timing.go`: timed-hit bar rendering (press / charge / sequence bars, flash hold, quality popup). `qualityVisuals` is the per-grade color + throb-intensity table.
- `internal/app/render/resources.go`: procedural resource loading, area material models, font loading.
- `internal/app/render/theme.go`: HUD color tokens, rounded panel helpers, shared text shadows, `drawArrowMarker` / `drawTextWithShadow` helpers.
- `internal/app/render/theme_export.go`: theme accessor for non-render packages (editor, title).
- `internal/app/render/textures.go`: procedural area wall/floor/sky textures and rat/bat/party sprite pixels.
- `internal/app/render/models.go`: procedural mesh construction for trees, boulders, bushes, and mushroom props.
- `internal/app/render/layout.go`: `screenSize`/`screenSizeF`/`centerX`/`centerXF` (and exported `CenterXF`) helpers, `behindCamera` cull for in-world prompts, `tileWorldPos` tile→world coord conversion, `DrawFooterHint` centered modal-footer text. Shared by every HUD panel and modal.
- `internal/app/render/display.go`: Fullscreen/Windowed display-mode helpers (`SetDisplayMode`, `ToggleDisplayMode`, `CurrentDisplayMode`, `DisplayMenuRowLabel`) shared by the pause menu and title menu.
- `internal/app/render/jukebox.go`: Pause-menu sound-tester state and helpers (`PlayJukebox`, `JukeboxRowLabel`) — cycles through `audio.Sound` entries on confirm.
- `internal/app/render/debug.go`: in-world tile labels and player-coord readout for the pause-menu Debug Overlay toggle. Always-built; no overhead when `g.DebugOverlay` is false.
- `internal/app/editor/`: in-game map authoring tool.
- `internal/app/editor/editor.go`: state, 6-layer enum (walls/floor/decor/props/ceiling/entities), brush palette (`layerBrushes`), modal kinds including `modalChestEdit`.
- `internal/app/editor/input.go`: hotkeys, mouse handling, modal updaters table (`modalUpdaters`), text input pump (`pumpPrintableASCII` with accept-rune filter), `chestAddRules` add-key table for the chest-edit modal.
- `internal/app/editor/ops.go`: brush apply / erase, undo/redo, multi-tile footprint validation, pack + chest edit ops (`placeChestAt`, `filterChests`), resize, reachability warnings.
- `internal/app/editor/draw.go`: topbar / palette / metadata / grid / status drawing, modal renderers table (`modalDrawers`), `scrollWindow` helper, sentinel-brush hatching, ceiling-hash overlay (`drawCeilingHash`), chest spawn markers.
- `internal/app/editor/sounds.go`: in-editor sound creator modal — synth-param sliders, saved-sounds list with Play/Delete, built-in-cue assignments column.
- `internal/app/editor/palette.go`: editor chrome colors (panels, borders, buttons). Map-content brush colors live in `editor.go`'s `layerBrushes`.
- `internal/app/title/`: launch screen — pick Adventure (map picker) / Editor / Display / Quit.
- `internal/app/audio/`: sound bank. Procedural defaults synthesized at startup; user `.wav` overrides live under `maps/sounds/` and rebind built-in cues via `maps/sounds/assignments.txt`.
- `internal/app/audio/audio.go`: device init / teardown, bank slot loading via `pcmToSound`, `Play`, `soundMeta` (display label + canonical slug per `Sound` enum entry).
- `internal/app/audio/user.go`: raylib-side preview ring (`PreviewPCM`/`PreviewFile`), bank-overlay reload (`ReloadUserAssignments`, `AssignUserSound`), thin wrappers around the userconfig helpers for backwards-compat.
- `internal/app/audio/userconfig/`: pure (non-raylib) filesystem + parsing helpers — `SoundsDir`, `SanitizeName`, `ListSounds`, `WriteWAV`, `DeleteSound`, `LoadAssignments`, `SaveAssignments`. Tested via `go test`.
- `internal/app/audio/wavsynth/`: pure synthesis primitives — `SynthSweep`, `SynthChord`, `SynthChime`, `BuildWAV`, `SampleRate`. Tested via `go test`.

## Gameplay Notes

- Movement is tile-based with short animation. `W/S` step, `A/D` strafe, `Q/E` or arrows turn. Walls and prop tiles (trees / boulders / large bushes) both block.
- Right-click drag free-look recenters smoothly on release.
- Pause menu: `P`, `Esc` (only outside battle, so Esc keeps working as "back / cancel target" in combat), or gamepad Start.
- Battles start when the player is adjacent to a live enemy pack; if needed, the player rotates to face it first. The engaged pack IS the encounter — packs are authored on the map (PackSpawns) and there's no spatial clustering at runtime.
- Chests block movement onto their tile; press Confirm (Space/Enter/Z) while adjacent to open the loot modal. Take one item with Confirm, or land the cursor on "Take All" to drain the chest. Esc closes. Looted chests render with an open lid and ignore further interactions.
- Battle input:
  - Confirm: `Space`, `Enter`, or `Z`
  - Back: `Esc` or `X`
  - Target/menu movement: arrows, `W/S`, `A/D`, `Tab` where applicable
- Mixed initiative: party + enemies are sorted by SPD into a single per-round queue. Burn ticks fire at the start of the burning actor's own turn; Poison ticks at the END of the poisoned actor's turn (after their action lands).
- Basic-attack accuracy: `core.AttackAccuracy(stats, quality)` rolls per swing. DEX-driven baseline (0.55 + 0.04·DEX) plus a timing bonus (Miss=+0 → Excellent=+0.45), clamped to [0, 1]. Skills are NOT gated — they pay MP and shouldn't be double-jeopardied. Excellent timing functionally guarantees the hit for every class.
- Timing-bar JUICE: graded flashes throb (height pulse, scaled by grade); Miss flashes shake horizontally. Cursor color-previews the live grade while inside the press window. Excellent flashes spawn an expanding shockwave ring. Charge ticks freshness-flash on crossing. Sequence arrows pulse on correct land. Hit-stop pauses the world for 100ms (Great) or 160ms (Excellent) between the bar flash and the action's apply step.
- Charge bars (Prayer, Firebolt) arm with a `ChargeTimingIntro` pre-arm pause (3s) during which the bar shows a "Press to start" prompt instead of "CHARGE!". A fresh edge press during the pause skips it; otherwise the bar auto-arms when the pause elapses. `Battle.ChargeNeedsRelease` gates the engage check so the same Enter the player used to confirm the target can't bleed into the bar's held-state read — they must release once first, then a fresh press engages. The gate clears the frame after `AttackTimingHeld()` goes false. Combat log narrows during the timing bar to dodge the bar's left edge ([render/battle.go drawCombatLogPanel](internal/app/render/battle.go)).
- "Targeting" indicators (in-world yellow chevron, enemy-roster row highlight, friendly-target pyramid) all gate on the same render-package predicates so they never drift: `targetingEnemy(g)` for both enemy indicators (`Phase == BattlePlayer && ActionMode == ActionEnemyTarget`), `targetingAlly(g)` (plus an explicit `Phase == BattlePlayer` at the call site) for the friendly marker. They drop the moment the timing bar arms, so the cursor only shows while the player is actually picking a target, not while they're pressing the bar.
- Tile-character reference for `.map` files lives in the top comment of `internal/app/core/mapfile/mapfile.go`. New tile types are added there + `internal/app/core/map.go`'s const blocks + the `tileLabelTable` in `map.go` (init asserts coverage) + the canonical list (`propTileCharList` / `decorTileCharList` / `floorTileCharList`) so the renderer's coverage asserts pass + the editor's brush palette in `internal/app/editor/editor.go` (`layerBrushes` — drives both palette UI and grid-cell colors via `tileColorByChar`) + the renderer (`r.specialFloors` for universal floors, `r.decorModels` for decor, `r.propModels` for props, or an inline case in `world.go`'s `drawWorld` / `drawDecor` for hand-tuned variants — both inline sets are documented in `inlineDecorChars` / `inlinePropChars` in `render/resources.go`).
- Floor layer: `FloorDeepWater` ('W') is the sole *blocking* floor — renders flat (camera can see across), but `BlockedAt` reports true so movement / pack snapping / chest placement / canPlaytest all refuse it. Shallow water ('~') is walkable. The lilypad decor ('y') is pure visual and pairs with water tiles for the swamp aesthetic.
- Per-grade balance tunables live in `internal/app/core/config.go`: the `timingGrades` table (label / atk mult / def mult / accuracy bonus per Miss..Excellent) is the single source of truth; render-side color + throb intensity live in `qualityVisuals` (render/timing.go); audio cue per grade lives in `gradeSounds` (battle/battle.go).
- Enemies: `rat`, `bat`, `diseased_rat` (tier-3 rat variant, 60% chance to inflict Poison on a landed bite; carries no loot), `goblin` (tier-3 grunt), `goblin_mage` (tier-4 caster — `SkillCastChance` controls its mixed Firebolt / Sleep loadout), `amoeba` (tier-3 tank with `Armor: 8` — phys whiffs to 1 dmg, magic shreds it), `venus_mantrap` (tier-4 plant lurker — slow, beefy bite + signature `SkillIngest` that pulls a party member out of combat until the mantrap dies, max one prey per mantrap, see Ingested below).
- Status effects: Burn (enemy-side, from Firebolt — flat tick at start of turn, 2–3 turns), Poison (party-side, from Diseased Rat — flat tick at END of turn, 3–5 turns), Sleep (from the goblin mage's `SkillSleep` — 2–5 turns, skips the sleeper's turn at the start, any damage > 0 wakes them), and Ingested (from the mantrap's `SkillIngest` — party member is removed from the turn queue and untargetable by friend or foe until the mantrap that swallowed them dies; the mantrap can still bite-attack but can't ingest a second prey; PoisonTurns survives the lockout, Sleep + Defending clear). None of the four stack onto an already-affected target. If every living party member is ingested at once the battle counts as lost (`core.ActivePartyCount == 0`).
- Armor: a per-actor field outside `Stats` (not buyable / not a level-up spend). `core.ApplyArmor` clips phys-tagged damage to `max(dmg - armor, 1)`; magic / heal / buff bypass entirely. Skill tags live on `SkillDefinition.Tag` (`SkillTagPhys`/`Magic`/`Heal`/`Buff`/`None`).
- Per-character XP / levels. Living party members earn the pack's `PackXPValue` on victory. Geometric cost curve: `XPForLevel(N) = LevelXPBase × LevelXPRatio^(N-1)` (100, 200, 400, …). Each level grants `LevelStatPoints` to spend in the level-up modal — VIT spends auto-raise MaxHP and heal the difference. The "Party Stats" pause-menu entry opens a read-only overlay of every member's stats / armor / XP-to-next.
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

## Init-time Invariants

The codebase enforces several data-consistency contracts at package init via `panic()` — if you add a new tile / enemy / skill / grade and forget one of the parallel tables, the program won't start. Read this list before adding a new "kind" of anything:

- **Skill handlers**: every `PartyClassDefinition.Skill` must be `PlayerCastable` AND have an entry in `battle/actions.go`'s `skillActionHandlers`. Every `PlayerCastable` skill in `core.PlayerCastableSkills` must have a handler. Asserted in `battle/actions.go` init.
- **Tile labels**: every char in `core.PropTileChars` / `DecorTileChars` / `floorTileCharList` must have a row in `tileLabelTable`. Asserted in `core/map.go` init.
- **Minimap colors**: every char in `core.PropTileChars` must have an entry in `render/minimap.go`'s `minimapPropColors`. Every blocking floor (`core.BlockingFloorChars`) must have an explicit case in `minimapTileColor` distinct from the open-tile fallback. Asserted in `render/minimap.go` init.
- **Decor / Prop models**: every char in `core.DecorTileChars` / `core.PropTileChars` must either have an entry in `decorModels` / `propModels` or be listed in `render/resources.go`'s `inlineDecorChars` / `inlinePropChars` set (for the hand-tuned bush/mushroom/tree/etc. cases drawn directly in `world.go`). Asserted in `assertDecorCoverage` / `assertPropCoverage` at `NewResources` time.
- **Enemy visuals**: every kind in `core.EnemyKinds` must have an `enemyVisuals` entry with a non-zero texture. Asserted at the end of `loadEnemyVisuals`.
- **Editor entity colors**: every kind in `core.EnemyKinds` must have an `entityBrushColors` entry. Asserted in `editor/editor.go` init.
- **Stat table**: `core.statTable` length must equal `core.StatCount`. Asserted in `core/party.go` init.
- **Timing grade tables**: `core.timingGrades`, `render.qualityVisuals`, and `battle.gradeSounds` all carry length-checks against `core.TimingQualityCount`. Adding a new grade is a four-file edit (timing.go iota, config.go row, timing.go row, battle.go row) and any one missing panics.

When in doubt, run the program once — the panics are descriptive and point at the missing wiring.

## Caution

- Avoid broad refactors while tuning gameplay feel. Most requests are visual/combat iteration and should stay scoped.
