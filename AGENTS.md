# Crawler — Agent Guide

Go/raylib dungeon crawler. Runtime in `internal/app`; `main.go` only calls `app.Run()`.
Windows-first (PowerShell default; prefer bash if installed). `raylib.dll` must sit beside the exe or in cwd.

## Commands

- Test: `go test -count=1 ./...`
- Vet: `go vet ./...`
- Build (smoke-check): `go build ./...` or `go build -o .\.build\crawler-3d.exe .`

**Never `go run .` to smoke-test.** It opens a real game window over the user's screen. To verify a change builds: `go build` / `go test` / `go vet`. For UI/render changes, describe what should be visible and let the user open the binary. Only launch when explicitly asked ("run it", "show me") — and prefer the run skill.

## Gamepad-first contract (binding)

The controller is the primary input device; keyboard/mouse are secondary conveniences. Every game UI surface (title, menus, HUD, combat, overlays, modals, panel tabs) MUST be fully operable with a controller alone. Design the pad flow first (D-pad/stick navigate, A confirm, B back), then layer kbd/mouse.

- **Exemption:** the map editor (`internal/app/editor/`) is keyboard+mouse only. Do NOT add pad bindings there or flag it for "missing gamepad support."
- All input reads route through the semantic `input` package (`ConfirmPressed`, `BackPressed`, `CursorUpDown`, `MenuTabPrev/NextPressed`, `LookStick`, …). Do NOT call `rl.IsKeyPressed` / `rl.IsGamepadButton*` / `rl.GetMouse*` at a battle/explore/render call site — add/extend a predicate in `input/input.go` so each binding lives in one remappable place. (Exception: render may read live pointer position at DRAW time for purely visual hover cues.)
- Button vocabulary — don't invent chords: A/Cross = confirm · B/Circle = back · Y/Triangle = panels · Start/Options = pause · Select/Share = quit/debug-flee · L1/L2+R1/R2 = page tabs / cycle target · D-pad+left stick = navigate · right stick = free-look. Square/X now carries Use (in-game) / Erase (editor); **L3/R3 are the only intentionally unbound buttons — claim one before reaching for a combo.**
- On-screen hints read controller-first via the glyph system (`render/glyphs.go`) — never a bare "Press Z…".

## Init-time invariants (parallel tables that panic at startup)

Add a new "kind" of anything and forget a parallel table → the program won't start. The panics are descriptive; when in doubt, run once.

- **Skill handlers:** every `core.PlayerCastableSkills` entry needs a `skillActionHandlers` row (`battle/actions.go` init). Every tree node's `GrantSkill` must be `PlayerCastable` (`core/skilltrees.go` init) → transitively every learnable skill resolves to a handler.
- **Tile labels:** every char in `PropTileChars`/`DecorTileChars`/`floorTileCharList` needs a `tileLabelTable` row (`core/map.go` init).
- **Minimap colors:** every `PropTileChars` char → `minimapPropColors`; every `BlockingFloorChars` → explicit `minimapTileColor` case (`render/minimap.go` init).
- **Decor/Prop/Door models:** every `DecorTileChars`/`PropTileChars` char → `decorModels`/`propModels` OR `inlineDecorHandlers`/`inlinePropHandlers`; door styles need a non-empty `doorProps` slot (`assertDecorCoverage`/`assertPropCoverage`/`assertDoorProps` at `LoadResources`).
- **Enemy gold/drops:** `GoldMin <= GoldMax`, both non-negative; every `Drops` entry `Chance` in [0,1] naming a registered `ItemKind` (`core/enemies.go` init).
- **Enemy visuals:** every `core.EnemyKinds` kind → `enemyVisuals` entry with non-zero texture (end of `loadEnemyVisuals`).
- **Editor entity colors:** every `core.EnemyKinds` kind → `entityBrushColors` (`editor/editor.go` init).
- **Materials:** every `MaterialSet` → one `materialDefs` row (`core/areas.go` init) AND a loaded `worldMaterial` (`assertMaterialCoverage`).
- **Stat table:** `statTable` + `statDescriptions` all length `StatCount` (each `statTable` row carries its own Get/Set/Add + Preview accessors, coverage-asserted); init also calls `StatPreviewLine` for every stat (`core/party.go`).
- **Equip slots:** `core.equipSlotInfo` sized `[EquipSlotCount]` (missing = compile error), init-asserted non-empty (`core/items.go`).
- **Panel-tab drawers/hints:** `panelTabs` is `[PanelTabCount]panelTabInfo` (missing = compile error); init asserts every row has both a `draw` func and a `footer` hint (`render/panels.go`).
- **Timing grades:** `core.timingGrades` + `render.qualityVisuals` + `battle.gradeSounds` all length-checked vs `TimingQualityCount` (four-file edit).
- **Mapfile schema:** `customEnemyEncodeFormat` verb count == `customEnemyFieldCount` (`core/mapfile/mapfile.go` init).
- **Rain kinds:** every `core.RainKind` → non-zero `rainVisuals` row (`render/weather.go` init).

## Gotchas (non-obvious, load-bearing)

- **Save corruption — enums are append-only.** `ItemKind` and `EnemyKind` serialize as their int value in saves. A mid-enum insert renumbers later entries and corrupts existing saves / bestiary maps. ALWAYS append at the end.
- **Sky/depth clear:** `drawAdventureScene` MUST call `rl.ClearBackground(...)` before `DrawSkyBackground` — the color is overdrawn, but the call carries the load-bearing DEPTH clear on this raylib build. Without it, 3D props (trees) flicker behind invisible depth holes.
- **Save = save-POINT:** load rebuilds the area fresh (packs/chests/fog reset); only party progression + gold + quests + position persist. Same semantics as an area transition.
- **Billboard grounding:** sprites are center-anchored; field vs battle use different base heights (`enemyBillboardY` ~0.68 vs `battleFormationCenterY` 1.0). Four eyeballed per-kind `enemyVisual` knobs (`size`/`yOffset`/`shadowRadius`/`markerYOffset`/`tint`) tune placement — expect to retune when swapping art.
- **Blocking floor:** `FloorDeepWater` ('W') is the only blocking floor — renders flat but `BlockedAt` returns true (movement/pack-snap/chest/playtest all refuse it). Shallow water '~' is walkable.
- **Action log lives on `GameState`, not `Battle`** — `g.ActionLog` + `g.StatusMessage` span in and out of combat. Use `g.LogMessage` for real actions (battle's `setBattleMessage` wraps it); `g.SetStatusMessage` for transient prompts that should NOT accrete.
- **New grid layer = one row in `gridLayers()`**, not a hand-listed field sweep (backs editor undo/dirty checks).

## Conventions

- Keep behavior repo-native and procedural unless asked. Visual assets are generated in `textures.go`; the exception is authored billboard PNGs under `maps/sprites/` (procedural fallback retained). Scan→extract→editor-tune workflow for those PNGs: see `ASSET_PIPELINE.md` (`tools/extract_sprite.py`).
- All on-disk assets resolve through `core.ResolveAssetDir(rel)` (cwd-relative first, then beside the exe). Never raw paths; add new asset folders through that helper.
- Respect real Go package boundaries — add behavior in the relevant directory, don't create prefixed files in `internal/app`.
- Reuse existing raylib drawing helpers before introducing a new abstraction. Keep HUD surfaces rounded, slightly translucent, readable borders + text shadows.
- **Auditing/recapping:** present findings in a FLAT, NUMBERED list.
- **Caution:** avoid broad refactors while tuning gameplay feel. Most requests are visual/combat iteration — keep them scoped.

## Where systems live (pointers, not a map)

Quick index: combat state machine `internal/app/battle/`, non-battle input/movement `internal/app/explore/`, drawing/HUD `internal/app/render/`, shared data + pure helpers `internal/app/core/`, map authoring `internal/app/editor/`, audio `internal/app/audio/`. Skill registry + minigames in `core/party.go`; tree node data in `core/skilltrees.go`; on-disk `.map` format in `core/mapfile/mapfile.go` (tile-char reference in its top comment).