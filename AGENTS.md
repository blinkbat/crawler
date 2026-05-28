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

### Smoke-checking changes — DO NOT `go run`

**`go run .` actually opens the game window on the user's screen.** It is not a headless smoke test. Running it during the user's working hours pops a real window over whatever they're doing and is genuinely disruptive.

For verifying that a change builds and links cleanly:

- Use `go build ./...` (or the named build above) — that exercises compile + link without launching anything.
- Run `go test -count=1 ./...` and `go vet ./...` for correctness.
- For UI / rendering changes, REPORT the change and let the user open the binary themselves. Do not "just check it boots." Describe what should be visible and stop.

Only run the game when the user has explicitly asked you to (e.g. "launch it", "run it", "show me"). Even then, prefer the `run` skill which goes through the project-aware launcher.

## File Layout

- `main.go`: tiny entrypoint; calls `app.Run()`.
- `internal/app/run.go`: window setup, borderless windowed fullscreen boot, resource lifetime, scene routing (title / adventure / editor), main loop.
- `internal/app/core/`: shared data and pure-ish helpers.
- `internal/app/core/config.go`: global constants, direction/action/phase enums, timing-bar tuning, RNG.
- `internal/app/core/types.go`: shared structs for player, map state, battle state, enemies, party members (with `Armor`, `SleepTurns`, `BoundTurns`, `ConfusedTurns`, `Ingested` + `IngestedBy`, `Level`, `XP`, `PendingLevelUps`, `SkillPoints`, `SkillTiers`), and chests (`Chest` runtime, `ChestSpawn` authored). `GameState` tracks `LevelUpOpen`/`LevelUpMember`/`LevelUpPending`/`LevelUpRowCursor` for the stat-spend modal (3 stat points per level, staged then committed via `CommitLevelUp`). Skill points (1 per level) sit on the member and are spent later via the Skills panel; the Party Stats surface is now a tab of the panels overlay rather than a separate overlay.
- `internal/app/core/state.go`: initial game state, party members (Level seeded to `BaseLevel`), runtime chest placement (`placeChests`).
- `internal/app/core/areas.go`: area / map-file conversion (5 grid layers + pack + chest + door spawns), material + facing names, on-disk path helpers.
- `internal/app/core/map.go`: area layout queries (`WallAt`, `CeilingAt`, `BlockedAt`, `FloorAt`, `TileAt`, `InBounds`), chest tile lookups (`ChestIndexAt`, `AdjacentChestIndex`, `AdjacentInteractableChestIndex`), door lookups (`DoorIndexAt`, `DoorByName`), enemy placement helpers, table-driven `tileLabelTable` powering `TileLabel`, and the canonical char registries (`PropTileChars`, `DecorTileChars`, `FloorTileChars`, `BlockingFloorChars`) feeding init-time coverage asserts.
- `internal/app/core/mapfile/`: on-disk `.map` format — five grid layers (`walls`/`floor`/`decor`/`props`/`ceiling`), enemy + chest + door spawn sections (`doors:` is optional; absent sections back-compat as empty), parse/encode/load/save, layer slot dispatch, blank-layer seeding. Older files without `ceiling:` parse fine and get a blank ceiling layer.
- `internal/app/core/party.go`: party class definitions, skill registry (now with `SkillTag` for Phys/Magic/Heal/Buff, `PlayerCastable` flag separating menu-eligible skills from enemy-only ones, and `SkillEffect.AppliesIngest` carrying the mantrap's swallow status), damage / heal / burn / sleep-duration formulas, `ApplyArmor` damp, `XPForLevel` / `AddXP` / `SpendStatPoint` / `FirstPendingLevelUp` for the leveling system, the table-driven `statTable` powering `StatLabel`/`StatValue`, and `PlayerCastableSkills` (used by battle's init to assert handler coverage).
- `internal/app/core/selectors.go`: read-only party/battle selectors, turn forecast generation, wrap-aware living-member walk, `PackXPValue` and `AwardBattleXP`, plus the "available" variants (`PartyMemberAvailable`, `ActivePartyCount`, `WrapNextAvailablePartyMember`, `AvailablePartyTargets`, `FirstAvailablePartyMember`) that exclude ingested members and `MantrapHasPrey` / `ReleaseIngestedBy` / `ReleaseAllIngested` for the Ingest status lifecycle.
- `internal/app/core/condition.go`: enemy wound-state thresholds and labels.
- `internal/app/core/enemies.go`: enemy kind enum (13 kinds today: Rat, Bat, DiseasedRat, Goblin, GoblinMage, Amoeba, VenusMantrap, CaveSpider, VampireBat, Wisp, StoneGolem, Necromancer, Skeleton — the const block is the source of truth), per-kind definitions (stats, tier, carried item, `Armor`, `XPValue`, `Skills`, `SkillCastChance`, `SpellPower`, `LifestealPercent`), and lookup helpers.
- `internal/app/core/items.go`: item registry, inventory add/consume/empty helpers, kind-by-name lookup.
- `internal/app/core/timing.go`: timed-hit minigame state machine (press / charge / sequence / multi-press tally), grade dispatch, damage/heal/defense scaling. `NewMultiPressState(rng, duration, count)` builds tally-mode bars: N accept windows + a late commit zone, each press in an unconsumed window scores a hit, commit-zone press or timeout resolves with the tally. Single-window bars stay on `NewTimingState`.
- `internal/app/core/vfx.go`: VFX intent layer — battle/explore code pushes `VFXRequest` values onto `g.VFXQueue` (via `EnqueueEnemyVFX` / `EnqueuePartyVFX` / `EnqueueTileVFX`), the render layer drains them each frame and materialises particles. Avoids battle → render imports. `RequestVFXReset` / `TakeVFXResetRequest` is the pool-clear signal pair (battle exit, area transition, return-to-title).
- `internal/app/core/daycycle.go`: time-of-day phase enum, phase-at-step math, phase labels.
- `internal/app/core/util.go`: math, easing, direction, color, clamp, flash, and bump helpers.
- `internal/app/input/`: shared key semantics for confirm/back/menu/targeting/exploration controls; analog-stick edge detection.
- `internal/app/explore/`: non-battle game input and movement.
- `internal/app/explore/movement.go`: pause menu input (P / Esc / gamepad Start), tile movement (props + chests + deep water block; doors trigger area transitions on step-land), pack-AI tick (junkyard-dog wander/chase + pack animation), step-into-pack engagement, smooth free-look return, step/turn animations, `core.RevealRadius` 3×3 fog-of-war marking on step. The Update gate routes level-up modal -> game panels overlay -> chest modal -> pause menu -> battle / explore.
- `internal/app/explore/chest.go`: chest-modal input loop — Up/Down picks a row, Confirm takes one or all, Esc closes; emptied chests mark `Looted` so the lid renders open.
- `internal/app/explore/levelup.go`: stat-spend modal input loop. Modal is NO LONGER auto-opened after battle — XP awards accrue PendingLevelUps and SkillPoints on the member, the party card paints a "+" badge, and the player allocates from the Tome menu when ready. The modal supports Back (X / B / Esc) on a staged row to decrement that row, or on an empty row to close and revert; Confirm on the Apply row commits via `core.CommitLevelUp`.
- `internal/app/explore/panels.go`: game panels overlay input loop — open/close on big-start (gamepad middle or keyboard I), L1/R1 cycle tabs, Up/Down moves per-tab cursor or zooms the Map tab. `PanelsOpen` shadows chest/pause priorities; the toggle refuses to open during battle.
- `internal/app/battle/`: turn-based combat state machine and rules.
- `internal/app/battle/battle.go`: battle lifecycle, phase transitions, mixed-initiative round queue, battle log updates, transient combat effects. `tickSleepAtTurnStart` skips the sleeper's turn; `enemyAIPickSkill` rolls `SkillCastChance` to pick between melee and a Skill from `EnemyDefinition.Skills` (filtered by `usableEnemySkills`, which respects `PerBattleCastLimit` for capped skills like Raise Bones); `resolveEnemySpell` dispatches via the `enemySpellHandlers` table — single-target casts gate on a living party target, AoE / summon casts (`AppliesAOEParty` / `AppliesSummonSkeleton`) bypass that gate. XP awards fire in `winBattle` / `leaveBattle`; the level-up modal is no longer auto-opened — points accrue on the member for later allocation.
- `internal/app/battle/menu.go`: combat menu input, action / item / target cycling.
- `internal/app/battle/actions.go`: Attack, Swipe, Prayer, Steal, Firebolt, burn ticks, damage resolution (armor-aware via `core.ApplyArmor`; any damage > 0 wakes a sleeping target), enemy attack target round-robin. Per-skill VFX queued via `core.EnqueueEnemyVFX` / `core.EnqueuePartyVFX`; multi-hit Swipe scales damage passes by `multiPressPasses` (reads the tally bar's `Hits` count).
- `internal/app/render/`: raylib drawing, procedural assets, and HUD.
- `internal/app/render/world.go`: camera, screen-filling sky background, world drawing (5 grid layers including ceiling slabs), enemy/party billboards, target markers, battle formation positioning.
- `internal/app/render/chest.go`: chest billboards (body + lid + lockplate), adjacent-chest prompt (`DrawChestPrompt`), and the chest-open modal (`DrawChestModal`).
- `internal/app/render/doors.go`: door-prop billboards (`DrawDoors`) at each `g.Doors` tile, rotated by the door's `Facing` so the opening points the right way.
- `internal/app/render/panels.go`: game panels overlay (`DrawPanelsOverlay`) — five tabs (Stats / Equipment / Items / Skills / zoomable Map). Map tab honors `g.Visited` for fog-of-war; pack/chest/door markers only paint on revealed tiles.
- `internal/app/render/levelup.go`: level-up modal (`DrawLevelUpModal`) — staged stat-point spend, reads `core.StatLabel` / `core.StatValue`. No longer auto-opens; triggered explicitly when the player chooses to allocate. Party Stats live in the panels overlay's Stats / Character tab.
- `internal/app/render/hud.go`: top-level HUD routing and exploration party totals.
- `internal/app/render/battle.go`: enemy roster, combat log, action menu, item picker, battle splash.
- `internal/app/render/party.go`: bottom party stat cards and HP/MP bars.
- `internal/app/render/turns.go`: color-coded turn order panel.
- `internal/app/render/minimap.go`: auto-scrolling minimap, facing arrow, day/night strip.
- `internal/app/render/menu.go`: pause menu overlay.
- `internal/app/render/classes.go`: per-party-class presentation (turn color, victory dance motion).
- `internal/app/render/daycycle.go`: time-of-day lighting profiles + interpolation, sky tinting.
- `internal/app/render/lighting.go`: lighting shader load / uniforms / per-area profiles. The `fogCeiling` constant in `distancefog.go` is hand-synced with this file's GLSL `clamp(fog, 0.0, 0.85)` — touch both halves together.
- `internal/app/render/distancefog.go`: owns the per-frame lighting-profile cache (`cacheLightingProfile` / `resolvedLightingProfile`) that DrawWorld populates and the billboard draws read. The actual billboard fog wash runs through the `billboardFogShader` shader pair in `lighting.go` — multiplicative tint can't lerp toward fog color, so each of the three billboard draws (drawFieldPacks / drawBattlePack / DrawPartySprites) wraps its loop in `BeginShaderMode(assets.billboardFog.shader)`. The shader uses the same `1 - exp(-density · dist)` curve and `fogCeiling` clamp as the world fog shader, so billboards recede in lockstep with the lit geometry around them.
- `internal/app/render/vfx.go`: particle pool + `TickAndDrawVFX` (drains `g.VFXQueue` → seeds per-kind spawn patterns → ticks → draws). 14 spawn patterns covering melee impact, ember, heal, smite pillar, frost shards, venom mist, arc flashes, steal pop, death dust, stoneslam ground burst, sleep/web/confuse/ingest status FX. Bounded by `particleHardCap = 2048`; `ResetParticles` is called from battle exit + area transition + return-to-title via the `core.VFXResetRequested` flag pair.
- `internal/app/render/timing.go`: timed-hit bar rendering (press / charge / sequence / multi-press tally bars, flash hold, quality popup). `qualityVisuals` is the per-grade color + throb-intensity table. `drawTallyBar` paints N accept windows + a late commit zone for multi-press skills.
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
- `internal/app/editor/editor.go`: state, 6-layer enum (walls/floor/decor/props/ceiling/entities), brush palette (`layerBrushes`), modal kinds including `modalChestEdit` / `modalDoorEdit` / `modalValidate`.
- `internal/app/editor/input.go`: hotkeys, mouse handling, modal updaters table (`modalUpdaters`), text input pump (`pumpPrintableASCII` with accept-rune filter), `chestAddRules` add-key table for the chest-edit modal. Topbar Validate button opens `modalValidate` via `openValidateModal`.
- `internal/app/editor/ops.go`: brush apply / erase, undo/redo, multi-tile footprint validation, pack + chest + door edit ops (`placeChestAt`, `placeDoorAt`, plus `removeChestSpawnAt` / `removeDoorAt` / `removePackAt` via `slices.DeleteFunc`), resize, reachability warnings, `rewriteLayerRows` row-mutation helper shared by `floodFill` / `fillEntireLayer`, `centerViewOnTile`.
- `internal/app/editor/draw.go`: topbar / palette / metadata / grid / status drawing, modal renderers table (`modalDrawers`), `scrollWindow` helper, sentinel-brush hatching, ceiling-hash overlay (`drawCeilingHash`), chest + door spawn markers, hover tooltip card, axis tick labels. Entity-marker colors live in `render`'s theme as `render.Marker*` exports.
- `internal/app/editor/sounds.go`: in-editor sound creator modal — synth-param sliders, saved-sounds list with Play/Delete, built-in-cue assignments column.
- `internal/app/editor/palette.go`: editor chrome colors (panels, borders, buttons). Map-content brush colors live in `editor.go`'s `layerBrushes`.
- `internal/app/title/`: launch screen — pick Adventure (map picker) / Editor / Display / Quit.
- `internal/app/audio/`: sound bank. Procedural defaults synthesized at startup; user `.wav` overrides live under `maps/sounds/` and rebind built-in cues via `maps/sounds/assignments.txt`.
- `internal/app/audio/audio.go`: device init / teardown, bank slot loading via `loadBank` / `loadCueFromDisk` / `pcmToSound`, `Play`, and the `soundCues` table (display label + canonical slug + procedural PCM closure per `Sound` enum entry; `soundIDByName` is derived from it).
- `internal/app/audio/user.go`: raylib-side preview ring (`PreviewPCM`/`PreviewFile`), bank-overlay reload (`ReloadUserAssignments`, `AssignUserSound`), thin wrappers around the userconfig helpers for backwards-compat.
- `internal/app/audio/userconfig/`: pure (non-raylib) filesystem + parsing helpers — `SoundsDir`, `SanitizeName`, `ListSounds`, `WriteWAV`, `DeleteSound`, `LoadAssignments`, `SaveAssignments`. Tested via `go test`.
- `internal/app/audio/wavsynth/`: pure synthesis primitives — `SynthSweep`, `SynthChord`, `SynthChime`, `BuildWAV`, `SampleRate`, plus the richer `SynthShape` (selectable wave shape, noise mix, vibrato rate + depth) which `SynthSweep` now wraps. `WaveShape` enum (`WaveSine` / `WaveSquare` / `WaveTriangle` / `WaveSaw`) drives the timbre picker in the editor's sound modal. Tested via `go test`.

## Gameplay Notes

- Movement is tile-based with short animation. `W/S` step, `A/D` strafe, `Q/E` or arrows turn. Walls and prop tiles (trees / boulders / large bushes) both block.
- Right-click drag free-look recenters smoothly on release.
- Pause menu ("small start"): `P`, `Esc` (only outside battle, so Esc keeps working as "back / cancel target" in combat), or gamepad Options/Start button (`GamepadButtonMiddleRight`).
- Game panels overlay ("big start"): keyboard `I` or gamepad middle button (`GamepadButtonMiddle` — PS button on DualSense, guide button on Xbox, closest raylib exposes to the PS5 touchpad click). Out-of-battle only. Five tabs (Stats / Equipment / Items / Skills / Map) cycle with L1/R1 or Tab/arrows. Up/Down moves the per-tab cursor or zooms the Map. Pressing the same button again — or Esc / B / Circle — closes the overlay. Map tab honors `g.Visited` for fog-of-war: every successful step marks the destination tile; pack/chest/door markers only show on revealed tiles.
- Battles start when the player is adjacent to a live enemy pack; if needed, the player rotates to face it first. The engaged pack IS the encounter — packs are authored on the map (PackSpawns) and there's no spatial clustering at runtime.
- Chests block movement onto their tile; press Confirm (Space/Enter/Z) while adjacent to open the loot modal. Take one item with Confirm, or land the cursor on "Take All" to drain the chest. Esc closes. Looted chests render with an open lid and ignore further interactions.
- Battle input:
  - Confirm: `Space`, `Enter`, or `Z`
  - Back: `Esc` or `X`
  - Target/menu movement: arrows, `W/S`, `A/D`, `Tab` where applicable
- Mixed initiative: party + enemies are sorted by SPD into a single per-round queue. Burn ticks fire at the start of the burning actor's own turn; Poison ticks at the END of the poisoned actor's turn (after their action lands).
- Basic-attack accuracy: `core.AttackAccuracy(stats, quality)` rolls per swing. DEX-driven baseline (0.55 + 0.04·DEX) plus a timing bonus (Miss=+0 → Excellent=+0.45), clamped to [0, 1]. Skills are NOT gated — they pay MP and shouldn't be double-jeopardied. Excellent timing functionally guarantees the hit for every class.
- Timing-bar JUICE: graded flashes throb (height pulse, scaled by grade); Miss flashes shake horizontally. Cursor color-previews the live grade while inside the press window. Excellent flashes spawn an expanding shockwave ring. Charge ticks freshness-flash on crossing. Sequence arrows pulse on correct land. Hit-stop pauses the world for 100ms (Great) or 160ms (Excellent) between the bar flash and the action's apply step.
- Charge bars (Prayer, Firebolt) arm with a `ChargeTimingIntro` pre-arm pause (3s) during which the bar shows a "Press to start" prompt instead of "CHARGE!". A fresh edge press during the pause skips it; otherwise the bar auto-arms when the pause elapses. `Battle.ChargeNeedsRelease` gates the engage check so the same Enter the player used to confirm the target can't bleed into the bar's held-state read — they must release once first, then a fresh press engages. The gate clears the frame after `AttackTimingHeld()` goes false. Combat log narrows during the timing bar to dodge the bar's left edge ([drawCombatLogPanel in render/battle.go](internal/app/render/battle.go#L229)).
- "Targeting" indicators (in-world yellow chevron, enemy-roster row highlight, friendly-target pyramid) all gate on the same render-package predicates so they never drift: `targetingEnemy(g)` for both enemy indicators (`Phase == BattlePlayer && ActionMode == ActionEnemyTarget`), `targetingAlly(g)` (plus an explicit `Phase == BattlePlayer` at the call site) for the friendly marker. They drop the moment the timing bar arms, so the cursor only shows while the player is actually picking a target, not while they're pressing the bar.
- Tile-character reference for `.map` files lives in the top comment of `internal/app/core/mapfile/mapfile.go`. New tile types are added there + `internal/app/core/map.go`'s const blocks + the `tileLabelTable` in `map.go` (init asserts coverage) + the canonical list (`propTileCharList` / `decorTileCharList` / `floorTileCharList`) so the renderer's coverage asserts pass + the editor's brush palette in `internal/app/editor/editor.go` (`layerBrushes` — drives both palette UI and grid-cell colors via `tileColorByChar`) + the renderer (`r.specialFloors` for universal floors, `r.decorModels` for decor, `r.propModels` for props, or an inline case in `world.go`'s `drawWorld` / `drawDecor` for hand-tuned variants — both inline sets are documented in `inlineDecorChars` / `inlinePropChars` in `render/resources.go`).
- Floor layer: `FloorDeepWater` ('W') is the sole *blocking* floor — renders flat (camera can see across), but `BlockedAt` reports true so movement / pack snapping / chest placement / canPlaytest all refuse it. Shallow water ('~') is walkable. The lilypad decor ('y') is pure visual and pairs with water tiles for the swamp aesthetic.
- Per-grade balance tunables live in `internal/app/core/config.go`: the `timingGrades` table (label / atk mult / def mult / accuracy bonus per Miss..Excellent) is the single source of truth; render-side color + throb intensity live in `qualityVisuals` (render/timing.go); audio cue per grade lives in `gradeSounds` (battle/battle.go).
- Enemies: `rat`, `bat`, `diseased_rat` (tier-3 rat variant, 60% chance to inflict Poison on a landed bite; carries no loot), `goblin` (tier-3 grunt), `goblin_mage` (tier-4 caster — `SkillCastChance` controls its mixed Firebolt / Sleep loadout), `amoeba` (tier-3 tank with `Armor: 8` — phys whiffs to 1 dmg, magic shreds it), `venus_mantrap` (tier-4 plant lurker — slow, beefy bite + signature `SkillIngest` that pulls a party member out of combat until the mantrap dies, max one prey per mantrap, see Ingested below).
- Status effects: Burn (enemy-side, from Firebolt — flat tick at start of turn, 2–3 turns), Poison (party-side, from Diseased Rat — flat tick at END of turn, 3–5 turns), Sleep (from the goblin mage's `SkillSleep` — 2–5 turns, skips the sleeper's turn at the start, any damage > 0 wakes them), and Ingested (from the mantrap's `SkillIngest` — party member is removed from the turn queue and untargetable by friend or foe until the mantrap that swallowed them dies; the mantrap can still bite-attack but can't ingest a second prey; PoisonTurns survives the lockout, Sleep + Defending clear). None of the four stack onto an already-affected target. If every living party member is ingested at once the battle counts as lost (`core.ActivePartyCount == 0`).
- Armor: a per-actor field outside `Stats` (not buyable / not a level-up spend). `core.ApplyArmor` clips phys-tagged damage to `max(dmg - armor, 1)`; magic / heal / buff bypass entirely. Skill tags live on `SkillDefinition.Tag` (`SkillTagPhys`/`Magic`/`Heal`/`Buff`/`None`).
- Per-character XP / levels. Living party members earn the pack's `PackXPValue` on victory. Geometric cost curve: `XPForLevel(N) = LevelXPBase × LevelXPRatio^(N-1)` (100, 200, 400, …). Each level grants `LevelStatPoints` stat points (spent in the level-up modal — VIT spends auto-raise MaxHP and heal the difference) PLUS `LevelSkillPoints` skill points (saved on the member, spent later via the Skills panel's tier-purchase UI). Unspent points surface as a "+" badge on the party card name via `core.HasUnspentPoints`. The level-up modal is opened on demand from the panels overlay's Character / Stats tab — it no longer auto-fires post-battle.
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
