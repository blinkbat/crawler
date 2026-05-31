package render

import (
	"crawler/internal/app/core"
	"fmt"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// panelStatMeasureCache memoizes rl.MeasureTextEx for the Stats-tab's
// right-aligned value strings (stat values, ARM, XP, status chip). These
// change only on level-up / HP-spend / status change — not 60 Hz — but
// drawPanelsStats was re-shaping ~8 strings per member every frame the
// Stats tab is open (~32 cgo calls for a 4-member party). Keyed by
// the tab mixes FontBody and FontSmall values (the shared measureCache
// keys on size, so both coexist).
var panelStatMeasureCache measureCache

func measurePanelStatValue(font rl.Font, text string, size float32) rl.Vector2 {
	return panelStatMeasureCache.measure(font, text, size, 1)
}

// panelsMapFooterCache memoizes the "AreaName   zoom: N cells   ..."
// footer string drawn on the Map tab. Both inputs change only on
// user action (open the overlay, change zoom, transition area), so
// rebuilding via fmt.Sprintf every frame is pure waste.
var panelsMapFooterCache struct {
	areaName string
	zoom     int
	text     string
}

func panelsMapFooterText(areaName string, zoom int) string {
	if panelsMapFooterCache.areaName == areaName && panelsMapFooterCache.zoom == zoom {
		return panelsMapFooterCache.text
	}
	panelsMapFooterCache.text = fmt.Sprintf("%s   zoom: %d cells   Up/Down to zoom", areaName, zoom)
	panelsMapFooterCache.areaName = areaName
	panelsMapFooterCache.zoom = zoom
	return panelsMapFooterCache.text
}

// panelTabDrawers dispatches by tab index to the per-tab body drawer.
// Indexed array (not a map) so the literal length-checks at compile
// time against PanelTabCount — adding a 6th tab is a single new
// PanelTab const + a new drawer + adding its slot here; forgetting
// one is a compile error, not a silent no-op like the old switch.
var panelTabDrawers = [core.PanelTabCount]func(core.GameState, Resources, rl.Rectangle){
	core.PanelTabStats:     drawPanelsStats,
	core.PanelTabEquipment: drawPanelsEquipment,
	core.PanelTabItems:     drawPanelsItems,
	core.PanelTabSkills:    drawPanelsSkills,
	core.PanelTabMap:       drawPanelsMap,
}

// DrawPanelsOverlay paints the game-panels modal — the five-tab overlay
// raised by the gamepad middle button / keyboard I. Routes by
// g.PanelsTab to the per-tab body drawer; the tab strip + footer hint
// are drawn once around all of them so the chrome stays consistent.
// No-op when the overlay isn't open.
func DrawPanelsOverlay(g core.GameState, assets Resources) {
	if !g.PanelsOpen {
		return
	}
	font := assets.Font()
	// Panels overlay skips drawModalScaffold's heading band — the tab
	// strip IS the heading. We reuse the veil + centered card, sized
	// screen-relative so the character menus fill most of the display and
	// each per-member column has room for body-size text.
	card := drawScreenFractionScaffold(font, panelsOverlayWidthFrac, panelsOverlayHeightFrac, "")
	cardX, cardY := int32(card.X), int32(card.Y)
	cardW, cardH := int32(card.Width), int32(card.Height)

	// Tab strip across the top. Each tab is a flat label on a soft
	// glass tile; the active tab gets a brighter glass + a thick gilt
	// underline so the eye lands on it before the body content. No
	// wood frame per tab — they read as ledger dividers, not nested
	// panels-in-a-panel.
	tabH := overlayTabHeight + 4
	tabRowY := cardY + 14
	tabPad := overlayTabPadding
	tabW := (cardW - 24 - tabPad*int32(core.PanelTabCount-1)) / int32(core.PanelTabCount)
	for t := core.PanelTab(0); t < core.PanelTabCount; t++ {
		tx := cardX + 12 + int32(t)*(tabW+tabPad)
		active := t == g.PanelsTab
		bg := glassMid
		txt := textMuted
		if active {
			bg = core.MixColor(glassMid, glassWarm, 0.65)
			txt = textPrimary
		}
		drawGlassPane(tx, tabRowY, tabW, tabH, bg)
		if active {
			// Gilt underline strip at the bottom of the active tab —
			// the same "you're here" mark the list rows use, scaled
			// to the tab width.
			rl.DrawRectangle(tx+8, tabRowY+tabH-3, tabW-16, 2, giltBright)
		}
		label := core.PanelTabLabel(t)
		m := measureTabLabel(font, label)
		drawTextWithShadow(font, label,
			float32(tx)+float32(tabW)/2-m.X/2,
			float32(tabRowY)+float32(tabH)/2-m.Y/2-1,
			FontBody, txt)
	}

	bodyY := tabRowY + tabH + 18
	bodyRect := rl.NewRectangle(float32(cardX+22), float32(bodyY),
		float32(cardW-44), float32(cardY+cardH-26-bodyY-overlayFooterReserve))

	if int(g.PanelsTab) >= 0 && int(g.PanelsTab) < len(panelTabDrawers) {
		panelTabDrawers[g.PanelsTab](g, assets, bodyRect)
	}

	drawModalFooter(font, card, "L1/R1 tabs   Left/Right pick member   X close")
}

// tabLabelMeasureCache memoizes panel-tab label measurements (fixed core
// registry, fills once and stays warm).
var tabLabelMeasureCache measureCache

func measureTabLabel(font rl.Font, label string) rl.Vector2 {
	return tabLabelMeasureCache.measure(font, label, FontBody, 1)
}

// memberColumnLayout returns the per-member column rectangles for any
// tab that paints one card per party member (Stats / Equipment /
// Skills). Equal-width columns with a small gap so the grid reads as
// "ledger of party members" rather than a single dense slab.
func memberColumnLayout(body rl.Rectangle, count int) []rl.Rectangle {
	const gap = float32(14)
	if count <= 0 {
		return nil
	}
	total := body.Width - gap*float32(count-1)
	colW := total / float32(count)
	cols := make([]rl.Rectangle, count)
	for i := 0; i < count; i++ {
		cols[i] = rl.NewRectangle(body.X+float32(i)*(colW+gap), body.Y, colW, body.Height)
	}
	return cols
}

// memberCardInner returns the inner content inset (X origin + width) for
// a per-member card column — the writable region inside the gutter the
// Stats / Equipment / Skills body drawers paint into. Centralizes the
// 14px gutter the three used to hardcode as `col.X + 14` / `colW - 28`.
func memberCardInner(col rl.Rectangle) (innerX, innerW float32) {
	const gutter = 14
	return col.X + gutter, col.Width - 2*gutter
}

// drawPartyMemberCardHeader paints the shared header chrome every
// per-member tab card uses: class accent stripe on the left edge, the
// member's name, a "Class · Lv N" sub-label, and the HP+MP bars. The
// returned float32 is the Y coordinate immediately below the bars —
// where tab-specific content (stat grid, equipment slots, skill list)
// should start drawing.
//
// `highlight` is true for the currently-cursored column; it brightens
// the name and tints the card body so the active member pops without
// adding a heavy second selection chrome.
func drawPartyMemberCardHeader(font rl.Font, m core.PartyMember, col rl.Rectangle, highlight bool) float32 {
	classCol := partyClassPresentationFor(m.Class).turnColor

	// Soft inset glass body. We don't paint a full wood-framed card per
	// member (would compete with the outer modal's frame); a small
	// rounded panel + a class stripe gives enough separation.
	cardBG := glassMid
	if highlight {
		cardBG = core.MixColor(glassMid, glassWarm, 0.55)
	}
	drawGlassPane(int32(col.X), int32(col.Y), int32(col.Width), int32(col.Height), cardBG)
	rl.DrawRectangle(int32(col.X), int32(col.Y)+6, 3, int32(col.Height)-12, classCol)

	innerX := col.X + 14
	innerW := col.Width - 24

	y := col.Y + 16
	nameCol := textPrimary
	if !highlight {
		nameCol = textMuted
	}
	// Class sigil flanks the name — same iconography as the party
	// ribbon's card, but at a larger radius so it reads as a banner
	// crest in this fuller pane.
	glyphR := float32(12)
	glyphCX := innerX + glyphR
	glyphCY := y + FontHeading/2
	drawClassGlyph(glyphCX, glyphCY, glyphR, m.Class, classCol)
	nameOffset := glyphR*2 + 12
	drawTextWithShadow(font, m.Name, innerX+nameOffset, y, FontHeading, nameCol)
	y += 36

	// PartyMember.Name doubles as the class label in this build, so the
	// sub-line just carries the level — no need to repeat the class.
	sub := "Lv " + strconv.Itoa(m.Level)
	drawTextWithShadow(font, sub, innerX, y, FontBody, textMuted)
	y += 30

	hpFill := hpFillColor(m.HP, m.MaxHP)
	drawBar(font, innerX, y, innerW, 28, "HP", m.HP, m.MaxHP, hpFill, m.HP <= 0)
	y += 36
	drawBar(font, innerX, y, innerW, 28, "MP", m.MP, m.MaxMP, barMP, m.HP <= 0)
	y += 42

	return y
}

// drawPanelsStats renders the Stats tab as one card per party member.
// Each card stacks: class accent + name + level + HP/MP bars (shared
// header) → 2-column stat grid (STR/DEX/INT/WIS/VIT/SPD) → armor / XP
// row → status pill chip → allocate hints for the cursored member.
func drawPanelsStats(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	if len(g.Party) == 0 {
		return
	}
	cols := memberColumnLayout(body, len(g.Party))
	for i, m := range g.Party {
		highlight := i == g.PanelsRowCursor
		contentY := drawPartyMemberCardHeader(font, m, cols[i], highlight)
		innerX, innerW := memberCardInner(cols[i])

		// Stat grid: 2 columns, ceil(StatCount/2) rows. Each cell paints
		// "[icon] LBL  value" with the icon in soft gilt to the left of
		// the label, the label muted, and the value bright so the eye
		// scans the numbers without losing the sigil row anchor.
		statColW := innerW / 2
		rowH := float32(30)
		statRows := (core.StatCount + 1) / 2
		statIconCol := fadeColor(woodAccent, 0.9)
		for s := core.Stat(0); s < core.StatCount; s++ {
			row := int(s) / 2
			col := int(s) % 2
			cellX := innerX + float32(col)*statColW
			cellY := contentY + float32(row)*rowH
			label := core.StatLabel(s)
			value := strconv.Itoa(core.StatValue(m.Stats, s))
			drawStatIcon(s, cellX+9, cellY+13, 9, statIconCol)
			drawTextWithShadow(font, label, cellX+24, cellY, FontBody, textMuted)
			// Value right-aligned within the cell so the column
			// of numbers lines up no matter the label width.
			vm := measurePanelStatValue(font, value, FontBody)
			drawTextWithShadow(font, value, cellX+statColW-vm.X-14, cellY, FontBody, textPrimary)
		}
		contentY += float32(statRows) * rowH

		// Armor + XP secondary row, slightly muted so they
		// don't compete with the stat grid above.
		contentY += 8
		drawTextWithShadow(font, "ARM", innerX, contentY, FontSmall, textMuted)
		armVal := strconv.Itoa(m.Armor)
		am := measurePanelStatValue(font, armVal, FontSmall)
		drawTextWithShadow(font, armVal, innerX+statColW-am.X-14, contentY, FontSmall, textPrimary)

		nextXP := core.XPForLevel(m.Level)
		xpText := strconv.Itoa(m.XP) + " / " + strconv.Itoa(nextXP)
		drawTextWithShadow(font, "XP", innerX+statColW, contentY, FontSmall, textMuted)
		xm := measurePanelStatValue(font, xpText, FontSmall)
		drawTextWithShadow(font, xpText, innerX+innerW-xm.X, contentY, FontSmall, textPrimary)
		contentY += 28

		// Status chip — bright pill in the per-status accent
		// color so afflicted members read at a glance.
		if kind, turns := core.PartyStatus(m); kind != core.PartyStatusNone {
			label := partyStatusTurnLabel(kind, turns)
			lm := measurePanelStatValue(font, label, FontSmall)
			chipW := lm.X + 20
			chipH := float32(26)
			chipX := innerX
			col, _ := partyStatusVisual(kind)
			drawSmallPanel(int32(chipX), int32(contentY), int32(chipW), int32(chipH), fadeColor(col, 0.28))
			drawSmallPanelOutline(int32(chipX), int32(contentY), int32(chipW), int32(chipH), fadeColor(col, 0.85))
			drawTextWithShadow(font, label, chipX+10, contentY+4, FontSmall, col)
			contentY += chipH + 8
		}

		// Allocate hint: only on the cursored member, only when
		// there's something to spend. Painted near the bottom of
		// the card so it reads as a call-to-action footer.
		if highlight && (m.PendingLevelUps > 0 || m.SkillPoints > 0) {
			hintY := cols[i].Y + cols[i].Height - 60
			if m.PendingLevelUps > 0 {
				hint := "Z   allocate " + strconv.Itoa(m.PendingLevelUps) + " stat pt" + plural(m.PendingLevelUps)
				drawTextWithShadow(font, hint, innerX, hintY, FontSmall, inkAccent)
				hintY += 24
			}
			if m.SkillPoints > 0 {
				hint := strconv.Itoa(m.SkillPoints) + " skill pt" + plural(m.SkillPoints) + "  (Skills tab)"
				drawTextWithShadow(font, hint, innerX, hintY, FontSmall, inkAccent)
			}
		}
	}
}

// plural returns the "s" suffix when n != 1 so labels like "1 pt"
// / "2 pts" don't grow special-case branches. Tiny helper kept
// next to the only caller for now.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// drawPanelsEquipment renders the Equipment tab: a per-member column
// of five drop targets (R.HAND, L.HAND, ARMOR, ACC1, ACC2) above a
// shared inventory strip of every equippable item the party owns.
// The strip is the drag source; the slots are the drop targets. The
// input layer (explore/panels_drag.go) reads back the hit rects
// stored on lastEquipLayout to route mouse events.
//
// While a drag is in progress (g.EquipDrag.Source != None), the
// dragged item paints near the cursor and the compatible drop
// targets gain a gilt outline so the player sees where it can land.

// Equipment-tab layout constants. Co-located so tuning the panel
// shape is a single-file edit; the drag-ghost width is locked to
// the inventory-tile width on purpose (a hovering ghost the same
// size as its source tile reads cleaner than a free-floating one),
// so changing equipInventoryTileW automatically follows on the
// ghost. equipMinColRegion is derived from the per-slot row height
// × EquipSlotCount so the "is there room for the strip?" rescue
// branch can never out-shrink the slot column underneath it.
const (
	equipSlotRowHeight    = float32(56)
	equipStripGap         = float32(8)
	equipStripHeight      = float32(140)
	equipInventoryTileW   = float32(190)
	equipInventoryTileH   = float32(80)
	equipInventoryTileGap = float32(10)
	equipDragGhostW       = equipInventoryTileW
	equipDragGhostH       = float32(48)
)

// equipMinColRegion is the minimum height left for the per-member
// slot column before the inventory strip starts auto-shrinking.
// Derived from the slot row count so a new equip slot lifts the
// floor automatically.
var equipMinColRegion = equipSlotRowHeight * float32(core.EquipSlotCount)

// slotIconForType returns the icon-draw function for an EquipSlotIndex
// — the per-slot row variant.
func slotIconForType(slot core.EquipSlotIndex) func(cx, cy, r float32, col rl.Color) {
	return slotIconForKind(core.SlotIndexType(slot))
}

// slotIconForKind returns the icon-draw function for an
// EquipmentSlotType — the inventory-tile / drag-ghost variant.
// One mapping, one place; the historical sword/shield/ring icons
// stay if the slot type set ever expands (e.g. SlotConsumable).
func slotIconForKind(t core.EquipmentSlotType) func(cx, cy, r float32, col rl.Color) {
	switch t {
	case core.SlotHand:
		return drawSlotIconSword
	case core.SlotArmor:
		return drawSlotIconShield
	case core.SlotAccessory:
		return drawSlotIconRing
	}
	return drawSlotIconRing
}

// equipPanelLayout caches the hit-test rectangles laid down each
// frame by drawPanelsEquipment so the input layer can read them
// without re-running the layout math. SlotRects is flattened
// [partyIndex][slotIndex] in row-major order; InventoryRects mirrors
// the order of equippableInventory. PartyCount and InvCount let the
// input layer iterate without re-deriving sizes.
type equipPanelLayout struct {
	SlotRects        []rl.Rectangle // len = PartyCount * EquipSlotCount
	SlotPartyIdx     []int          // parallel: which member owns each rect
	SlotIdx          []core.EquipSlotIndex
	InventoryRects   []rl.Rectangle
	InventoryEntries []equipInventoryEntry
	DragCursor       rl.Vector2
}

type equipInventoryEntry struct {
	Kind  core.ItemKind
	Count int
}

// lastEquipLayout is the most recently drawn panel layout. Read by the
// input layer in the same frame; render writes it AFTER drawing so
// hit-test geometry matches what was painted. Single-threaded
// renderer + single-threaded input means no synchronisation needed.
var lastEquipLayout equipPanelLayout

// ResetEquipPanelLayout zeroes the cached hit-rect layout. Called
// from the input layer on overlay close / tab switch so the first
// frame after a transition can't observe stale rects from a previous
// Equipment-tab visit — without this, an LMB-down on the frame
// EQUIPMENT regains focus could route to coordinates that no longer
// describe the layout the player sees.
func ResetEquipPanelLayout() { lastEquipLayout = equipPanelLayout{} }

// EquipPanelInventoryVisibleCount returns how many inventory tiles the
// Equipment strip actually painted last frame (after overflow
// truncation). The keyboard/controller cursor clamps to this so it
// never focuses an off-screen tile.
func EquipPanelInventoryVisibleCount() int { return len(lastEquipLayout.InventoryRects) }

// EquipPanelInventoryEntryKind returns the ItemKind of the visible
// inventory tile at index i (ok=false when out of range). Reads the
// same per-frame entry list the strip drew, so a controller "lift"
// targets exactly the tile the player sees.
func EquipPanelInventoryEntryKind(i int) (core.ItemKind, bool) {
	// Bound by the VISIBLE tile count (InventoryRects), not the full
	// entries list, so this can never return a kind for an off-screen
	// (overflow-truncated) tile — keeps it in lockstep with
	// EquipPanelInventoryVisibleCount, which the cursor clamps against.
	// InventoryEntries[i] is safe because rects are a prefix of entries.
	if i < 0 || i >= len(lastEquipLayout.InventoryRects) {
		return core.ItemNone, false
	}
	return lastEquipLayout.InventoryEntries[i].Kind, true
}

// EquipPanelSlotHit returns (partyIndex, slot, true) if `pt` is inside
// any slot rect, else (-1, 0, false). Mouse hit test for the input
// layer.
func EquipPanelSlotHit(pt rl.Vector2) (int, core.EquipSlotIndex, bool) {
	for i, r := range lastEquipLayout.SlotRects {
		if rl.CheckCollisionPointRec(pt, r) {
			return lastEquipLayout.SlotPartyIdx[i], lastEquipLayout.SlotIdx[i], true
		}
	}
	return -1, 0, false
}

// inventoryHitRect returns the index of the inventory tile under
// `pt`, or -1. Shared by EquipPanelInventoryHit and
// EquipPanelInventoryAreaHit so the iterate-rects loop lives in one
// place — a future per-tile bounds check (padding, dead zone) can't
// drift between the two callers.
func inventoryHitRect(pt rl.Vector2) int {
	for i, r := range lastEquipLayout.InventoryRects {
		if rl.CheckCollisionPointRec(pt, r) {
			return i
		}
	}
	return -1
}

// EquipPanelInventoryHit returns (inventoryIndex into shared
// inventory, ItemKind, true) when `pt` is over an inventory tile, else
// (-1, ItemNone, false). InventoryIndex is the index in
// GameState.Inventory (not the filtered equippable list), so the
// input layer can ConsumeItem against it directly.
func EquipPanelInventoryHit(pt rl.Vector2, g core.GameState) (int, core.ItemKind, bool) {
	tileIdx := inventoryHitRect(pt)
	if tileIdx < 0 {
		return -1, core.ItemNone, false
	}
	kind := lastEquipLayout.InventoryEntries[tileIdx].Kind
	for j := range g.Inventory {
		if g.Inventory[j].Kind == kind && g.Inventory[j].Count > 0 {
			return j, kind, true
		}
	}
	return -1, core.ItemNone, false
}

// EquipPanelInventoryAreaHit reports whether `pt` is inside the
// inventory strip's overall bounds (the gutter, not a specific tile).
// Used by the input layer to detect "dropped on the inventory area" =
// unequip, even when the cursor doesn't land on a specific tile.
func EquipPanelInventoryAreaHit(pt rl.Vector2) bool {
	return inventoryHitRect(pt) >= 0
}

func drawPanelsEquipment(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	lastEquipLayout = equipPanelLayout{} // reset every frame
	if len(g.Party) == 0 {
		return
	}

	// Split the body into two regions: top for member columns, bottom
	// for the inventory strip, with a small gap. If the body is too
	// short to fit both at the canonical strip height + the minimum
	// column region, the strip shrinks instead of the columns.
	stripHeight := equipStripHeight
	if body.Height < stripHeight+equipMinColRegion {
		stripHeight = body.Height * 0.32
	}
	colsRegion := rl.NewRectangle(body.X, body.Y, body.Width, body.Height-stripHeight-equipStripGap)
	stripRegion := rl.NewRectangle(body.X, body.Y+colsRegion.Height+equipStripGap, body.Width, stripHeight)

	cols := memberColumnLayout(colsRegion, len(g.Party))
	slotRowH := equipSlotRowHeight
	totalSlots := len(g.Party) * int(core.EquipSlotCount)
	lastEquipLayout.SlotRects = make([]rl.Rectangle, 0, totalSlots)
	lastEquipLayout.SlotPartyIdx = make([]int, 0, totalSlots)
	lastEquipLayout.SlotIdx = make([]core.EquipSlotIndex, 0, totalSlots)

	for i, m := range g.Party {
		highlight := i == g.PanelsRowCursor
		contentY := drawPartyMemberCardHeader(font, m, cols[i], highlight)
		innerX, innerW := memberCardInner(cols[i])

		for s := core.EquipSlotIndex(0); s < core.EquipSlotCount; s++ {
			rowY := contentY + float32(int(s))*slotRowH
			slotRect := rl.NewRectangle(float32(innerX), rowY, float32(innerW), slotRowH-8)
			lastEquipLayout.SlotRects = append(lastEquipLayout.SlotRects, slotRect)
			lastEquipLayout.SlotPartyIdx = append(lastEquipLayout.SlotPartyIdx, i)
			lastEquipLayout.SlotIdx = append(lastEquipLayout.SlotIdx, s)

			// Compatible-target highlight: when a drag is active, slots
			// the held item can land on take a gilt outline. Resting
			// state is the standard glass bezel.
			compatible := false
			if g.EquipDrag.Source != core.EquipDragSourceNone {
				compatible = core.CanEquipInSlot(g.EquipDrag.Kind, s)
				// Suppress highlight on the drag origin so the source
				// slot doesn't visually accept-itself.
				if g.EquipDrag.Source == core.EquipDragSourceSlot &&
					g.EquipDrag.PartyIndex == i && g.EquipDrag.SlotIndex == s {
					compatible = false
				}
			}
			bg := fadeColor(glassDeep, 0.55)
			if compatible {
				bg = fadeColor(glassWarm, 0.65)
			}
			drawGlassPane(int32(slotRect.X), int32(slotRect.Y), int32(slotRect.Width), int32(slotRect.Height), bg)
			if compatible {
				rl.DrawRectangleLinesEx(slotRect, 1.5, giltBright)
			}

			equippedKind := m.Equipped[s]
			filled := equippedKind != core.ItemNone
			iconCol := fadeColor(woodAccent, 0.7)
			if filled {
				iconCol = giltBright
			}
			slotIconForType(s)(float32(innerX)+16, rowY+26, 11, iconCol)

			labelX := float32(innerX) + 40
			drawTextWithShadow(font, core.SlotIndexLabel(s), labelX, rowY+6, FontSmall, textMuted)
			value := "—"
			valCol := textDim
			if filled {
				value = core.ItemInfo(equippedKind).Name
				valCol = textPrimary
			}
			drawTextWithShadow(font, value, labelX, rowY+26, FontBody, valCol)
		}
	}

	// Inventory strip: tile every equippable item in the party
	// inventory. Consumables (cheese / jerky) stay off this strip —
	// they belong on the Items tab. Hit rects are stored so the
	// input layer can route drag-starts off these tiles.
	drawCard(int32(stripRegion.X), int32(stripRegion.Y), int32(stripRegion.Width), int32(stripRegion.Height), surfacePrimary, borderSoft, borderSoft)
	drawTextWithShadow(font, "EQUIPMENT INVENTORY", stripRegion.X+10, stripRegion.Y+8, FontSmall, textMuted)

	entries := equippableInventoryEntries(g.Inventory)
	lastEquipLayout.InventoryEntries = entries
	lastEquipLayout.InventoryRects = make([]rl.Rectangle, 0, len(entries))

	tileW := equipInventoryTileW
	tileH := equipInventoryTileH
	tileGap := equipInventoryTileGap
	tileX := stripRegion.X + 10
	tileY := stripRegion.Y + 24
	overflow := 0
	for entryIdx, entry := range entries {
		if tileX+tileW > stripRegion.X+stripRegion.Width-10 {
			tileX = stripRegion.X + 10
			tileY += tileH + tileGap
			if tileY+tileH > stripRegion.Y+stripRegion.Height-4 {
				// Out of vertical room — count what's left so the
				// player sees an honest "+N more" footer rather than
				// silently truncating to whatever fit.
				overflow = len(entries) - entryIdx
				break
			}
		}
		rect := rl.NewRectangle(tileX, tileY, tileW, tileH)
		lastEquipLayout.InventoryRects = append(lastEquipLayout.InventoryRects, rect)

		bg := fadeColor(glassMid, 0.85)
		// Highlight inventory tiles when the cursor is over them
		// during a drag-from-slot (so the player sees where the
		// dropped item will go).
		if g.EquipDrag.Source == core.EquipDragSourceSlot {
			if rl.CheckCollisionPointRec(rl.GetMousePosition(), rect) {
				bg = fadeColor(glassWarm, 0.85)
			}
		}
		drawGlassPane(int32(rect.X), int32(rect.Y), int32(rect.Width), int32(rect.Height), bg)
		def := core.ItemInfo(entry.Kind)
		slotIconForKind(def.Slot)(rect.X+18, rect.Y+rect.Height/2, 12, giltBright)
		drawTextWithShadow(font, def.Name, rect.X+42, rect.Y+12, FontSmall, textPrimary)
		if entry.Count > 1 {
			drawTextWithShadow(font, "x"+strconv.Itoa(entry.Count), rect.X+42, rect.Y+36, FontSmall, textHint)
		}
		// Bonus summary on the second line.
		bonus := equipBonusSummary(def)
		if bonus != "" {
			drawTextWithShadow(font, bonus, rect.X+42, rect.Y+rect.Height-22, FontSmall, inkAccent)
		}
		tileX += tileW + tileGap
	}
	if len(entries) == 0 {
		drawTextWithShadow(font, "No equipment held. Pickups from chests / steals land here.",
			stripRegion.X+10, stripRegion.Y+stripRegion.Height/2-6, FontSmall, textHint)
	}
	if overflow > 0 {
		// Honest truncation marker — drawn at the right edge of the
		// strip's header so it doesn't compete with the visible tiles
		// for click hit-tests. The hidden items are unreachable via
		// drag for now; a scroll affordance is the obvious follow-up.
		label := "+" + strconv.Itoa(overflow) + " more (not shown)"
		m := rl.MeasureTextEx(font, label, FontSmall, 1)
		drawTextWithShadow(font, label,
			stripRegion.X+stripRegion.Width-m.X-10,
			stripRegion.Y+8, FontSmall, inkAccent)
	}

	// Keyboard/controller focus outline — drawn only when the cursor
	// (not the mouse) is driving the panel. Brighter + thicker than the
	// drag's compatible-slot outline so "where am I" reads distinctly
	// from "where can this land."
	focusRect, focusValid := equipFocusRect(g)
	if focusValid {
		rl.DrawRectangleLinesEx(focusRect, 2.5, giltBright)
	}

	// Drag overlay — paints the held item as a tooltip. Anchored to the
	// focused cell when the cursor is driving, else trailing the mouse.
	if g.EquipDrag.Source != core.EquipDragSourceNone {
		drawEquipDragGhost(g, font, equipGhostAnchor(focusRect, focusValid, body))
	}

	footer := "Mouse: drag between strip and slots.  Pad/keys: move cursor, Confirm lifts/places, Back cancels.  Drop on the strip to unequip."
	drawTextWithShadow(font, footer, body.X, body.Y+body.Height-18, FontSmall, textHint)
}

// equipFocusRect returns the rectangle of the keyboard/controller focus
// cell (ok=false when the cursor is inactive or the cached layout
// doesn't yet describe the focused cell). Reads the same per-frame hit
// rects the mouse path uses so the outline lands exactly on the painted
// cell.
func equipFocusRect(g core.GameState) (rl.Rectangle, bool) {
	if !g.EquipCursorActive {
		return rl.Rectangle{}, false
	}
	if g.EquipCursor.OnInventory {
		i := g.EquipCursor.InvTile
		if i >= 0 && i < len(lastEquipLayout.InventoryRects) {
			return lastEquipLayout.InventoryRects[i], true
		}
		return rl.Rectangle{}, false
	}
	si := g.EquipCursor.Member*int(core.EquipSlotCount) + int(g.EquipCursor.Slot)
	if si >= 0 && si < len(lastEquipLayout.SlotRects) {
		return lastEquipLayout.SlotRects[si], true
	}
	return rl.Rectangle{}, false
}

// equipGhostAnchor returns the top-left for the held-item ghost. When
// the cursor owns the panel it floats just above the focused cell
// (flipping below / clamping left if that would spill out of the body);
// otherwise it trails the mouse like a classic drag tooltip.
func equipGhostAnchor(focus rl.Rectangle, focusValid bool, body rl.Rectangle) rl.Vector2 {
	if focusValid {
		pos := rl.NewVector2(focus.X+focus.Width-equipDragGhostW, focus.Y-equipDragGhostH-6)
		if pos.Y < body.Y {
			pos.Y = focus.Y + focus.Height + 6
		}
		if pos.X < body.X {
			pos.X = focus.X
		}
		return pos
	}
	m := rl.GetMousePosition()
	return rl.NewVector2(m.X+10, m.Y+4)
}

// equippableInventoryEntries filters g.Inventory down to items whose
// definition has a real equipment slot. Order preserves the slice
// order so the player's hand-picked inventory ordering stays stable
// across frames.
func equippableInventoryEntries(inv []core.ItemStack) []equipInventoryEntry {
	out := make([]equipInventoryEntry, 0, len(inv))
	for _, st := range inv {
		if st.Count <= 0 {
			continue
		}
		def, ok := core.ItemInfoOk(st.Kind)
		if !ok || def.Slot == core.SlotNone {
			continue
		}
		out = append(out, equipInventoryEntry{Kind: st.Kind, Count: st.Count})
	}
	return out
}

// equipBonusSummary returns the single-line "STR +2" / "Armor +1" /
// "MDef +2" copy painted under an item's tile. Compact, builds the
// shortest combination of bonuses authored on the def.
// equipBonusSummaryCache memoizes equipBonusSummary by item kind. The
// summary is built from the immutable ItemDefinition, so it's computed
// once per kind rather than rebuilding a []string + concatenated string
// for every visible equipment tile (and the drag ghost) every frame the
// Equipment tab is open. The "" result for no-bonus items is cached too.
var equipBonusSummaryCache = map[core.ItemKind]string{}

func equipBonusSummary(def core.ItemDefinition) string {
	if s, ok := equipBonusSummaryCache[def.Kind]; ok {
		return s
	}
	parts := []string{}
	// Lead with the weapon's mechanical class so the player can see at a
	// glance which stat governs its to-hit/damage and whether it strikes at
	// range — derived from the registry (core.WeaponAccuracyStat /
	// WeaponIsRanged), never re-authored in item prose.
	if def.Weapon != core.WeaponNone {
		tag := core.StatLabel(core.WeaponAccuracyStat(def.Weapon)) + " weapon"
		if core.WeaponIsRanged(def.Weapon) {
			tag = core.StatLabel(core.WeaponAccuracyStat(def.Weapon)) + " ranged"
		}
		parts = append(parts, tag)
	}
	if def.ArmorBonus != 0 {
		parts = append(parts, "Armor +"+strconv.Itoa(def.ArmorBonus))
	}
	if def.MDefBonus != 0 {
		parts = append(parts, "MDef +"+strconv.Itoa(def.MDefBonus))
	}
	for s := core.Stat(0); s < core.StatCount; s++ {
		v := def.StatBonus[s]
		if v == 0 {
			continue
		}
		sign := "+"
		if v < 0 {
			sign = ""
		}
		parts = append(parts, core.StatLabel(s)+" "+sign+strconv.Itoa(v))
	}
	out := ""
	if len(parts) > 0 {
		out = parts[0]
		for i := 1; i < len(parts); i++ {
			out += "  " + parts[i]
		}
	}
	equipBonusSummaryCache[def.Kind] = out
	return out
}

// drawEquipDragGhost paints the held item as a translucent tile at the
// given top-left position. Painted last so it floats above the rest of
// the panel. `pos` is the mouse (drag) or the focused cell (cursor),
// resolved by equipGhostAnchor.
func drawEquipDragGhost(g core.GameState, font rl.Font, pos rl.Vector2) {
	def := core.ItemInfo(g.EquipDrag.Kind)
	rect := rl.NewRectangle(pos.X, pos.Y, equipDragGhostW, equipDragGhostH)
	drawGlassPane(int32(rect.X), int32(rect.Y), int32(rect.Width), int32(rect.Height), fadeColor(glassWarm, 0.95))
	rl.DrawRectangleLinesEx(rect, 1.5, giltBright)
	slotIconForKind(def.Slot)(rect.X+16, rect.Y+rect.Height/2, 11, giltBright)
	drawTextWithShadow(font, def.Name, rect.X+34, rect.Y+8, FontSmall, textPrimary)
	bonus := equipBonusSummary(def)
	if bonus != "" {
		drawTextWithShadow(font, bonus, rect.X+34, rect.Y+26, FontSmall, inkAccent)
	}
}

// drawSlotIconSword paints a small upright longsword sigil for the
// Weapon equipment slot. Built from a vertical blade (tapered tip
// triangle + body rectangle + centre fuller stripe), a horizontal
// crossguard, and a round pommel. Sized by `r` (the icon's
// half-height).
func drawSlotIconSword(cx, cy, r float32, col rl.Color) {
	bladeHalfW := r * 0.18
	if bladeHalfW < 1.5 {
		bladeHalfW = 1.5
	}
	pommelY := cy - r + 1
	rl.DrawCircleV(rl.NewVector2(cx, pommelY), bladeHalfW*1.4, col)
	guardY := cy - r*0.55
	guardHalfW := r * 0.75
	rl.DrawRectangle(int32(cx-guardHalfW), int32(guardY), int32(guardHalfW*2), 2, col)
	rl.DrawRectangle(int32(cx-guardHalfW), int32(guardY), 2, 3, col)
	rl.DrawRectangle(int32(cx+guardHalfW-2), int32(guardY), 2, 3, col)
	bladeTop := guardY + 2
	bladeBottom := cy + r*0.62
	rl.DrawRectangle(int32(cx-bladeHalfW), int32(bladeTop), int32(bladeHalfW*2), int32(bladeBottom-bladeTop), col)
	fuller := fadeColor(col, 0.5)
	rl.DrawRectangle(int32(cx), int32(bladeTop+2), 1, int32(bladeBottom-bladeTop-4), fuller)
	tip := rl.NewVector2(cx, cy+r)
	left := rl.NewVector2(cx-bladeHalfW, bladeBottom)
	right := rl.NewVector2(cx+bladeHalfW, bladeBottom)
	drawTriangleCCW(tip, right, left, col)
}

// drawSlotIconShield paints a small heater-shield sigil for the
// Armor equipment slot. Built from a top rectangle (shoulders) + a
// bottom triangle (point) + a centre boss (gilt highlight disc).
// Sized by `r` (the icon's half-height).
func drawSlotIconShield(cx, cy, r float32, col rl.Color) {
	topW := r * 1.4
	topH := r * 0.7
	// Shoulders.
	rl.DrawRectangle(int32(cx-topW/2), int32(cy-r), int32(topW), int32(topH), col)
	// Tapered point: triangle from the bottom corners of the
	// shoulders down to a single tip.
	tip := rl.NewVector2(cx, cy+r)
	left := rl.NewVector2(cx-topW/2, cy-r+topH)
	right := rl.NewVector2(cx+topW/2, cy-r+topH)
	drawTriangleCCW(tip, right, left, col)
	// Centre boss — small inner disc + bright pip.
	rl.DrawCircleV(rl.NewVector2(cx, cy-r*0.05), r*0.32, fadeColor(col, 0.55))
	rl.DrawCircleV(rl.NewVector2(cx, cy-r*0.05), r*0.16, giltBright)
}

// drawSlotIconRing paints a small ring sigil with a gem cap for the
// Accessory equipment slot. Built from an annulus (outer circle
// minus inner circle) and a tiny gem dot at the top. Sized by `r`
// (the ring's outer radius).
func drawSlotIconRing(cx, cy, r float32, col rl.Color) {
	// Annulus via outer disc + inner punch-out using the slot's
	// background tone. The background here is the slot bezel
	// (fadeColor(glassDeep, 0.55)); approximate with a slightly
	// darker pure-glass disc so the ring reads hollow at small
	// sizes.
	rl.DrawCircleV(rl.NewVector2(cx, cy+1), r, col)
	rl.DrawCircleV(rl.NewVector2(cx, cy+1), r*0.55, fadeColor(glassDeep, 0.8))
	// Gem dot at top of the ring band — bright gilt highlight.
	rl.DrawCircleV(rl.NewVector2(cx, cy+1-r*0.85), r*0.32, giltBright)
}

// drawPanelsItems renders the Items tab as a clean ledger: a scrollable
// stack list on the left two-thirds, a description panel for the
// cursored item on the right third. Empty inventory falls through to
// a short note so the panel never reads as broken.
// panelsItemStacksBuf is the reused scratch slice for the Items tab's
// live-stack list — refilled each frame the tab is open instead of
// allocating a fresh slice.
var panelsItemStacksBuf []core.ItemStack

func drawPanelsItems(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	panelsItemStacksBuf = core.LiveStacksInto(g.Inventory, panelsItemStacksBuf)
	stacks := panelsItemStacksBuf
	if len(stacks) == 0 {
		drawTextWithShadow(font, "Your bags are empty.", body.X+14, body.Y+16, FontHeading, textMuted)
		drawTextWithShadow(font, "Loot from steals and chests will appear here.",
			body.X+14, body.Y+54, FontBody, textHint)
		return
	}

	// Two-pane layout: list on the left, detail card on the right.
	const gap = float32(16)
	listW := body.Width*0.62 - gap/2
	detailW := body.Width - listW - gap
	listRect := rl.NewRectangle(body.X, body.Y, listW, body.Height)
	detailRect := rl.NewRectangle(body.X+listW+gap, body.Y, detailW, body.Height)

	// List rows. Generous padding so each row reads as its own
	// ledger line, not a cramped table cell.
	rowH := float32(46)
	rowPad := float32(14)
	cursor := g.PanelsRowCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(stacks) {
		cursor = len(stacks) - 1
	}
	for i, stack := range stacks {
		y := listRect.Y + float32(i)*rowH
		if y+rowH > listRect.Y+listRect.Height {
			break
		}
		info := core.ItemInfo(stack.Kind)
		highlight := i == cursor
		if highlight {
			drawGlassPane(int32(listRect.X), int32(y), int32(listRect.Width), int32(rowH-4),
				core.MixColor(glassMid, glassWarm, 0.6))
			rl.DrawRectangle(int32(listRect.X)+2, int32(y)+4, 3, int32(rowH-12), giltBright)
		}
		nameCol := textMuted
		if highlight {
			nameCol = textPrimary
		}
		drawTextWithShadow(font, info.Name, listRect.X+rowPad, y+12, FontBody, nameCol)
		// Count on the right edge of the row as a small chip.
		countText := "x" + strconv.Itoa(stack.Count)
		cm := rl.MeasureTextEx(font, countText, FontBody, 1)
		drawTextWithShadow(font, countText, listRect.X+listRect.Width-cm.X-rowPad, y+12, FontBody, inkAccent)
	}

	// Detail card: name, type/effect summary, count owned, description
	// stub. Reads as the ledger's "current entry" pane.
	drawGlassPane(int32(detailRect.X), int32(detailRect.Y), int32(detailRect.Width), int32(detailRect.Height), glassMid)
	if cursor < len(stacks) {
		stack := stacks[cursor]
		info := core.ItemInfo(stack.Kind)
		dy := detailRect.Y + 14
		dx := detailRect.X + 14
		drawTextWithShadow(font, info.Name, dx, dy, FontHeading, textPrimary)
		dy += 38
		effect := panelsItemHealLabel(info.HealAmount)
		drawTextWithShadow(font, effect, dx, dy, FontBody, inkAccent)
		dy += 30
		owned := "Owned: " + strconv.Itoa(stack.Count)
		drawTextWithShadow(font, owned, dx, dy, FontBody, textMuted)
		dy += 36
		// Description placeholder (item registry doesn't carry one
		// today). Wrap a short canned hint so the panel doesn't
		// feel empty.
		hint := "Consumable. Use from the battle menu's Item action."
		drawTextWithShadow(font, hint, dx, dy, FontSmall, textHint)
	}
}

// skillCostMPLabel returns "<cost> MP" with the small cost range
// pre-formatted into a LUT. Skill costs are bounded by class design
// (currently 4-12 MP across all classes); the LUT cap of 32 absorbs
// any tuning slack. Used by both the action menu's skill submenu and
// the panels overlay's Skills tab.
func skillCostMPLabel(cost int) string {
	if cost >= 0 && cost < len(skillCostMPLabelCache) {
		return skillCostMPLabelCache[cost]
	}
	return strconv.Itoa(cost) + " MP"
}

var skillCostMPLabelCache = func() [32]string {
	var out [32]string
	for i := range out {
		out[i] = strconv.Itoa(i) + " MP"
	}
	return out
}()

// panelsItemHealLabelCache pre-formats "+N HP" for the small heal-
// amount range items currently roll in. The Items tab paints these
// every frame the overlay sits on it; without the cache the path
// runs fmt.Sprintf for every visible stack.
var panelsItemHealLabelCache = func() [64]string {
	var out [64]string
	for i := range out {
		out[i] = "+" + strconv.Itoa(i) + " HP"
	}
	return out
}()

func panelsItemHealLabel(amount int) string {
	if amount >= 0 && amount < len(panelsItemHealLabelCache) {
		return panelsItemHealLabelCache[amount]
	}
	return "+" + strconv.Itoa(amount) + " HP"
}

// panelsItemHealLabelMeasureCache memoizes MeasureTextEx for the
// right-aligned heal labels (bounded keys from the precomputed label LUT).
var panelsItemHealLabelMeasureCache measureCache

func measurePanelsItemHealLabel(font rl.Font, label string) rl.Vector2 {
	return panelsItemHealLabelMeasureCache.measure(font, label, FontSmall, 1)
}

// drawPanelsSkills renders the Skills tab as one card per party
// member, mirroring the Stats / Equipment layout. Inside each card
// the member's learned skills stack as compact rows: name, MP cost
// chip, short description below. The class skill currently armed for
// battle (m.SkillCursor) gets a gilt left spine so the player sees
// at a glance what the Skill action will fire.
func drawPanelsSkills(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	if len(g.Party) == 0 {
		return
	}
	cols := memberColumnLayout(body, len(g.Party))
	for i, m := range g.Party {
		highlight := i == g.PanelsRowCursor
		contentY := drawPartyMemberCardHeader(font, m, cols[i], highlight)
		innerX, innerW := memberCardInner(cols[i])

		skills := core.PartySkills(m)
		rowH := float32(66)
		for j, s := range skills {
			rowY := contentY + float32(j)*rowH
			if rowY+rowH-6 > cols[i].Y+cols[i].Height {
				break
			}
			armed := j == m.SkillCursor

			// Soft inset row. The armed skill gets a subtle warm
			// tint so the eye lands on the active loadout.
			rowBG := fadeColor(glassDeep, 0.55)
			if armed {
				rowBG = core.MixColor(rowBG, glassWarm, 0.65)
			}
			drawGlassPane(int32(innerX), int32(rowY), int32(innerW), int32(rowH-8), rowBG)
			if armed {
				rl.DrawRectangle(int32(innerX)+2, int32(rowY)+4, 3, int32(rowH-16), giltBright)
			}

			// Name on the left, MP cost chip on the right of the
			// header line.
			nameCol := textPrimary
			if !armed {
				nameCol = textMuted
			}
			drawTextWithShadow(font, core.SkillName(s), innerX+12, rowY+6, FontBody, nameCol)
			cost := core.SkillCost(s)
			if cost > 0 {
				costText := skillCostMPLabel(cost)
				cm := rl.MeasureTextEx(font, costText, FontSmall, 1)
				drawTextWithShadow(font, costText, innerX+innerW-cm.X-12, rowY+8, FontSmall, inkAccent)
			}
			// Description on the second line of the row.
			drawTextWithShadow(font, core.SkillDescription(s), innerX+12, rowY+34, FontSmall, textHint)
		}
	}
}

// drawPanelsMap renders the zoomable Map tab. Cells-on-screen comes
// from g.PanelsMapZoom (clamped by the input handler); explored tiles
// paint at full color, unexplored at a heavy fade so the player can
// see the silhouette of the area without ruining the discovery.
// Enemy packs only show on explored tiles (don't spoil unseen rooms).
func drawPanelsMap(g core.GameState, assets Resources, body rl.Rectangle) {
	font := assets.Font()
	m := g.Area
	if m.Width <= 0 || m.Height <= 0 {
		return
	}
	zoom := g.PanelsMapZoom
	if zoom <= 0 {
		zoom = core.PanelMapZoomDefault
	}
	// Cell size in pixels: fit `zoom` cells across the body, then use
	// the same size for the vertical so the map stays square. The
	// player is centered in the visible window.
	cellPx := body.Width / float32(zoom)
	if cellPx*float32(zoom) > body.Height {
		cellPx = body.Height / float32(zoom)
	}
	cellsX := int(body.Width / cellPx)
	cellsY := int(body.Height / cellPx)
	startX := g.Player.TileX - cellsX/2
	startZ := g.Player.TileZ - cellsY/2

	mapX := body.X + (body.Width-float32(cellsX)*cellPx)/2
	mapY := body.Y + (body.Height-float32(cellsY)*cellPx)/2

	for localZ := 0; localZ < cellsY; localZ++ {
		for localX := 0; localX < cellsX; localX++ {
			mx := startX + localX
			mz := startZ + localZ
			px := int32(mapX + float32(localX)*cellPx)
			py := int32(mapY + float32(localZ)*cellPx)
			pw := int32(cellPx)
			if pw < 1 {
				pw = 1
			}
			if !m.InBounds(mx, mz) {
				rl.DrawRectangle(px, py, pw, pw, mapTileFogColor)
				continue
			}
			// Unvisited tiles render the same flat darkness as out-of-
			// bounds — the map only reveals what the player has stepped
			// inside the SightRadius window of, not the underlying
			// authored layout.
			if !visitedAt(g, mx, mz) {
				rl.DrawRectangle(px, py, pw, pw, mapTileFogColor)
				continue
			}
			rl.DrawRectangle(px, py, pw, pw, minimapTileColor(m.Materials, m.TileAt(mx, mz)))
		}
	}

	// Pack markers are intentionally omitted: the map shows the
	// terrain you've seen, not who's standing on it. Enemies stay a
	// surprise — the player has to actually look at the world view
	// to know what's around. (Earlier passes painted red dots on
	// visited pack tiles; removed when the 3×3 fog-of-war reveal
	// landed, since the wider window made the minimap effectively
	// "where are all the enemies" radar.)

	// Chest markers — gold square, visited tiles only.
	for _, ch := range g.Chests {
		if !visitedAt(g, ch.TileX, ch.TileZ) {
			continue
		}
		lx := ch.TileX - startX
		lz := ch.TileZ - startZ
		if lx < 0 || lz < 0 || lx >= cellsX || lz >= cellsY {
			continue
		}
		px := int32(mapX + float32(lx)*cellPx + cellPx*0.25)
		py := int32(mapY + float32(lz)*cellPx + cellPx*0.25)
		pw := int32(cellPx * 0.5)
		if pw < 2 {
			pw = 2
		}
		col := mapChestMarkerColor
		if ch.Looted {
			col = mapChestLootedColor
		}
		rl.DrawRectangle(px, py, pw, pw, col)
	}

	// Door markers — wooden rectangle, visited tiles only.
	for _, d := range g.Doors {
		if !visitedAt(g, d.TileX, d.TileZ) {
			continue
		}
		lx := d.TileX - startX
		lz := d.TileZ - startZ
		if lx < 0 || lz < 0 || lx >= cellsX || lz >= cellsY {
			continue
		}
		px := int32(mapX + float32(lx)*cellPx + cellPx*0.30)
		py := int32(mapY + float32(lz)*cellPx + cellPx*0.10)
		pw := int32(cellPx * 0.40)
		ph := int32(cellPx * 0.80)
		if pw < 2 {
			pw = 2
		}
		if ph < 2 {
			ph = 2
		}
		rl.DrawRectangle(px, py, pw, ph, mapDoorMarkerColor)
	}

	// Player arrow at the center of the visible window.
	plx := g.Player.TileX - startX
	plz := g.Player.TileZ - startZ
	pcx := mapX + (float32(plx)+0.5)*cellPx
	pcy := mapY + (float32(plz)+0.5)*cellPx
	drawPanelsMapArrow(pcx, pcy, cellPx, g.Player.Facing)

	// Compass rose in the upper-right corner — the centerpiece D&D
	// cartography ornament. Sized small enough not to dominate but
	// large enough to read as 8-point.
	drawCompassRose(body.X+body.Width-46, body.Y+10, 28, font)

	// Map footer — area name + zoom indicator.
	footer := panelsMapFooterText(g.Area.Name, zoom)
	drawTextWithShadow(font, footer, body.X, body.Y+body.Height-20, FontSmall, textHint)
}

// drawCompassRose paints an 8-point compass rose at (cx, cy) within
// a diameter `d`. Long N/E/S/W points + short diagonal points + a
// gilt centre disc + a wood-tone outer ring + a tiny "N" letter at
// the north tip. The classic D&D treasure-map crest.
func drawCompassRose(cx, cy float32, d float32, font rl.Font) {
	outerR := d / 2
	innerR := outerR * 0.35
	// Outer wood ring — a circle with a slightly-inset glass
	// background so the rose sits inside its own medallion.
	rl.DrawCircleV(rl.NewVector2(cx, cy), outerR+1, woodDark)
	rl.DrawCircleV(rl.NewVector2(cx, cy), outerR, glassDeep)
	rl.DrawCircleLines(int32(cx), int32(cy), outerR, woodAccent)

	// Four long cardinal points (N E S W). Each is a kite drawn as
	// two triangles from the centre to the tip.
	cardinals := [4]struct {
		ax, ay, px, py float32
	}{
		{ax: 0, ay: -1, px: 1, py: 0},  // N
		{ax: 1, ay: 0, px: 0, py: 1},   // E
		{ax: 0, ay: 1, px: -1, py: 0},  // S
		{ax: -1, ay: 0, px: 0, py: -1}, // W
	}
	pointHalfW := outerR * 0.18
	for i, c := range cardinals {
		tip := rl.NewVector2(cx+c.ax*outerR*0.95, cy+c.ay*outerR*0.95)
		leftBase := rl.NewVector2(cx+c.px*pointHalfW, cy+c.py*pointHalfW)
		rightBase := rl.NewVector2(cx-c.px*pointHalfW, cy-c.py*pointHalfW)
		col := giltDim
		if i == 0 {
			// North gets the bright point — the heraldic convention.
			col = giltBright
		}
		drawTriangleCCW(rl.NewVector2(cx, cy), tip, leftBase, col)
		drawTriangleCCW(rl.NewVector2(cx, cy), rightBase, tip, col)
	}
	// Four short diagonal points (NE SE SW NW), in soft wood tone.
	diags := [4]struct{ ax, ay, px, py float32 }{
		{ax: sqrt2Inv, ay: -sqrt2Inv, px: sqrt2Inv, py: sqrt2Inv},
		{ax: sqrt2Inv, ay: sqrt2Inv, px: -sqrt2Inv, py: sqrt2Inv},
		{ax: -sqrt2Inv, ay: sqrt2Inv, px: -sqrt2Inv, py: -sqrt2Inv},
		{ax: -sqrt2Inv, ay: -sqrt2Inv, px: sqrt2Inv, py: -sqrt2Inv},
	}
	diagHalfW := outerR * 0.12
	diagReach := outerR * 0.65
	for _, c := range diags {
		tip := rl.NewVector2(cx+c.ax*diagReach, cy+c.ay*diagReach)
		leftBase := rl.NewVector2(cx+c.px*diagHalfW, cy+c.py*diagHalfW)
		rightBase := rl.NewVector2(cx-c.px*diagHalfW, cy-c.py*diagHalfW)
		drawTriangleCCW(rl.NewVector2(cx, cy), tip, leftBase, woodAccent)
		drawTriangleCCW(rl.NewVector2(cx, cy), rightBase, tip, woodAccent)
	}
	// Centre disc + bright pip.
	rl.DrawCircleV(rl.NewVector2(cx, cy), innerR, woodAccent)
	rl.DrawCircleV(rl.NewVector2(cx, cy), innerR*0.5, giltBright)
	// "N" letter just above the north point.
	nLetter := "N"
	nm := rl.MeasureTextEx(font, nLetter, FontTiny, 1)
	drawTextWithShadow(font, nLetter, cx-nm.X/2, cy-outerR-nm.Y-2, FontTiny, inkAccent)
}

// visitedAt reports whether the player has stepped on this tile. Helper
// so the map-panel markers don't open-code the bounds check at every
// call.
func visitedAt(g core.GameState, x, z int) bool {
	if g.Visited == nil || z < 0 || z >= len(g.Visited) {
		return false
	}
	if x < 0 || x >= len(g.Visited[z]) {
		return false
	}
	return g.Visited[z][x]
}

// drawPanelsMapArrow paints the player marker on the Map tab — a small
// spear-shaped triangle pointing in the player's current facing. Sized
// to a fraction of cellPx so the arrow scales with the zoom level.
// Uses drawFacingArrow (shared with the minimap) with a 1.0 : 0.7
// forward-to-sideways ratio so the silhouette reads as a directional
// spearhead rather than the equilateral compass the minimap draws.
func drawPanelsMapArrow(cx, cy, cellPx float32, facing int) {
	r := cellPx * 0.45
	if r < 5 {
		r = 5
	}
	drawFacingArrow(rl.NewVector2(cx, cy), r, r*0.7, facing, playerArrowColor)
}
