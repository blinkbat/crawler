package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// customEnemyLayout precomputes the modal's clickable regions each
// frame so the draw + click paths stay in lockstep. Rebuilt per-frame
// against current screen size so a resize keeps the modal centered.
type customEnemyLayout struct {
	card     rl.Rectangle
	listArea rl.Rectangle
	// listTop is the Y of the first visible row; listTopRow..listEnd is the
	// scroll window (via scrollWindow) so a long custom-enemy list no longer
	// overflows the card. Row rects are computed on demand by listRowRect.
	listTop    float32
	listTopRow int
	listEnd    int
	addBtn     rl.Rectangle

	nameField rl.Rectangle
	baseLabel rl.Rectangle
	basePrev  rl.Rectangle
	baseNext  rl.Rectangle

	// statRows is keyed by statID; each row has a value rect + minus / plus
	// buttons. Centralized so the draw loop reads the same rects the click
	// handler uses.
	statRows []customStatRow
	// coreStatRows are the STR/DEX/INT/WIS/VIT/SPD adjusters routed
	// through core.statTable so a new core Stat surfaces automatically.
	coreStatRows []customStatRow

	skillRows []rl.Rectangle

	deleteBtn rl.Rectangle
	closeBtn  rl.Rectangle
}

type customStatRow struct {
	id    customStatID
	label string
	rect  rl.Rectangle
	minus rl.Rectangle
	plus  rl.Rectangle
}

// customStatID enumerates the numeric stat columns on the edit form.
// Step sizes and label strings live in customStatRowSpec so a new
// column is one row, not a switch update.
type customStatID int

const (
	csHP customStatID = iota
	csMP
	csAttack
	csArmor
	csMDef
	csXP
	csTier
	csSpellPower
	csSkillChance
)

type customStatSpec struct {
	id    customStatID
	label string
	step  int // for SkillChance, this is the step ×100 (0.05 → 5)
	// field returns the address of the int stat this row edits. nil for
	// csSkillChance — the lone float field, handled specially by the
	// value formatter and the adjuster. Having the accessor on the spec
	// lets customStatValueString / adjustCustomStat be table walks rather
	// than two parallel switches that drift when a column is added.
	field func(*core.CustomEnemyDef) *int
}

var customStatSpecs = []customStatSpec{
	{csHP, "HP", 1, func(d *core.CustomEnemyDef) *int { return &d.HP }},
	{csMP, "MP (n/a)", 1, func(d *core.CustomEnemyDef) *int { return &d.MP }},
	{csAttack, "Attack", 1, func(d *core.CustomEnemyDef) *int { return &d.AttackDamage }},
	{csArmor, "Armor", 1, func(d *core.CustomEnemyDef) *int { return &d.Armor }},
	{csMDef, "MDef", 1, func(d *core.CustomEnemyDef) *int { return &d.MDef }},
	{csXP, "XP", 1, func(d *core.CustomEnemyDef) *int { return &d.XPValue }},
	{csTier, "Tier", 1, func(d *core.CustomEnemyDef) *int { return &d.Tier }},
	{csSpellPower, "SpellPwr", 1, func(d *core.CustomEnemyDef) *int { return &d.SpellPower }},
	{csSkillChance, "SkillCh%", 5, nil},
}

// customStatSpecFor returns the spec row for a stat id.
func customStatSpecFor(id customStatID) (customStatSpec, bool) {
	for _, spec := range customStatSpecs {
		if spec.id == id {
			return spec, true
		}
	}
	return customStatSpec{}, false
}

const (
	customCardWidth  = float32(720)
	customCardHeight = float32(560)
	customListWidth  = float32(220)
	customRowHeight  = float32(28)
)

func customEnemyModalLayout(s *State) customEnemyLayout {
	card := centeredCardRect(customCardWidth, customCardHeight)

	l := customEnemyLayout{card: card}
	// List column on the left, below the heading reserve.
	listX := card.X + modalContentInset
	listY := card.Y + 56
	listH := card.Height - 56 - 56 // leave footer for Delete/Close
	l.listArea = rl.NewRectangle(listX, listY, customListWidth, listH)
	l.listTop = listY
	// Reserve the bottom strip for "+ Add new"; the rows above it scroll so a
	// long list can't run off the card. The window follows the selected entry.
	rowsAreaH := listH - 36
	maxRows := int(rowsAreaH / customRowHeight)
	if maxRows < 1 {
		maxRows = 1
	}
	l.listTopRow, l.listEnd = scrollWindow(s.modalCustomIdx, len(s.area.CustomEnemies), maxRows)
	l.addBtn = rl.NewRectangle(listX, listY+listH-30, customListWidth, 26)

	// Right column: form. Anchored to the right of the list.
	formX := listX + customListWidth + 16
	formY := listY
	formW := card.X + card.Width - formX - 16
	l.nameField = rl.NewRectangle(formX+72, formY, formW-72, 30)
	formY += 38

	l.baseLabel = rl.NewRectangle(formX, formY, 100, 30)
	l.basePrev = rl.NewRectangle(formX+72, formY, 30, 30)
	l.baseNext = rl.NewRectangle(formX+formW-30, formY, 30, 30)
	formY += 38

	l.statRows = make([]customStatRow, len(customStatSpecs))
	for i, spec := range customStatSpecs {
		rowY := formY + float32(i)*36
		value, minus, plus := stepperRow(formX+82, rowY, 76, 4)
		l.statRows[i] = customStatRow{
			id:    spec.id,
			label: spec.label,
			rect:  value,
			minus: minus,
			plus:  plus,
		}
	}
	formY += float32(len(customStatSpecs)) * 36

	// Stats grid: 3 columns × 2 rows for STR/DEX/INT/WIS/VIT/SPD,
	// routed through core.statTable so a new core Stat surfaces here
	// automatically. customStatRow.id stays at -1-relative to mark
	// these as "use the parallel Stat enum index from coreStatRow.coreStat"
	// — kept on a separate slice so the existing customStatSpecs loop
	// in updateCustomEnemiesModal isn't perturbed.
	const csCols = 3
	cellW := float32(132)
	cellH := float32(30)
	l.coreStatRows = make([]customStatRow, int(core.StatCount))
	for i := 0; i < int(core.StatCount); i++ {
		col := i % csCols
		row := i / csCols
		cellX := formX + float32(col)*(cellW+tightBtnGap)
		cellY := formY + float32(row)*(cellH+tightBtnGap)
		l.coreStatRows[i] = customStatRow{
			label: core.StatLabel(core.Stat(i)),
			rect:  rl.NewRectangle(cellX+34, cellY, 40, cellH),
			minus: rl.NewRectangle(cellX+78, cellY, 24, cellH),
			plus:  rl.NewRectangle(cellX+104, cellY, 24, cellH),
		}
	}
	formY += float32((int(core.StatCount)+csCols-1)/csCols) * (cellH + tightBtnGap)

	skills := core.EnemyCastableSkills()
	l.skillRows = make([]rl.Rectangle, len(skills))
	for i := range skills {
		l.skillRows[i] = rl.NewRectangle(formX, formY+float32(i)*24, formW, 22)
	}

	// Footer. Delete uses the shared wide-button size (matches the door
	// modal); Close stays narrower at its own width.
	const closeW = float32(80)
	btnY := card.Y + card.Height - modalBtnH - modalBottomInset
	l.deleteBtn = rl.NewRectangle(card.X+modalContentInset, btnY, modalWideBtnW, modalBtnH)
	l.closeBtn = rl.NewRectangle(card.X+card.Width-closeW-modalContentInset, btnY, closeW, modalBtnH)
	return l
}

// listRowRect returns the on-screen rect for absolute list index i. Only
// meaningful for i in [listTopRow, listEnd) — the visible scroll window.
func (l customEnemyLayout) listRowRect(i int) rl.Rectangle {
	return rl.NewRectangle(l.listArea.X, l.listTop+float32(i-l.listTopRow)*customRowHeight, customListWidth, customRowHeight-2)
}

// activeCustomEnemy returns a pointer to the selected entry or nil
// when modalCustomIdx is out of range. Used by both draw and update;
// keeps the OOB guard in one place so a transient mid-frame delete
// can't crash the dispatcher.
func activeCustomEnemy(s *State) *core.CustomEnemyDef {
	if s.modalCustomIdx < 0 || s.modalCustomIdx >= len(s.area.CustomEnemies) {
		return nil
	}
	return &s.area.CustomEnemies[s.modalCustomIdx]
}

func openCustomEnemiesModal(s *State) {
	s.modal = modalCustomEnemies
	s.modalCursor = 0 // reset the form-row cursor for keyboard nav
	if len(s.area.CustomEnemies) > 0 && s.modalCustomIdx < 0 {
		s.modalCustomIdx = 0
	}
}

// customEnemyRowCount is the number of keyboard-navigable form rows: the
// base sprite, each numeric stat row, the six core stats, then each skill
// toggle — in the same order they're drawn. The flat keyboard cursor
// (s.modalCursor) indexes into this; customEnemyRowAt / applyCustomEnemyRow
// and the draw highlight share the mapping so input and display can't drift.
func customEnemyRowCount() int {
	return 1 + len(customStatSpecs) + int(core.StatCount) + len(core.EnemyCastableSkills())
}

// customEnemySection names which group of the custom-enemy form a row
// belongs to. The flat keyboard cursor and the mouse hit-test both
// resolve to a (section, sub-index) pair so the form's row order lives in
// ONE place (customEnemyRowAt) — the count, the cursor highlight, and the
// adjust target can't desync.
type customEnemySection int

const (
	cesBase  customEnemySection = iota // base sprite picker
	cesStat                            // sub = index into customStatSpecs
	cesCore                            // sub = index into the core stats (Stat)
	cesSkill                           // sub = index into EnemyCastableSkills()
)

// customEnemyRowAt maps the flat cursor index to its (section, sub). The
// single source of truth for the form's row layout; customEnemyRowCount,
// customEnemyCursorRect, and the keyboard handler all go through it.
func customEnemyRowAt(cursor int) (customEnemySection, int) {
	if cursor <= 0 {
		return cesBase, 0
	}
	i := cursor - 1
	if i < len(customStatSpecs) {
		return cesStat, i
	}
	i -= len(customStatSpecs)
	if i < int(core.StatCount) {
		return cesCore, i
	}
	return cesSkill, i - int(core.StatCount)
}

// applyCustomEnemyRow applies an edit to one form row — shared by the
// keyboard nav AND the mouse +/- / click-to-toggle handlers so the
// per-section semantics (cycle base, step stat, step core, toggle skill)
// live once and can't diverge. dir is ±1; toggle flips a skill.
func applyCustomEnemyRow(def *core.CustomEnemyDef, section customEnemySection, sub, dir int, toggle bool) {
	switch section {
	case cesBase:
		if dir != 0 {
			def.BaseKind = cycleEnemyKind(def.BaseKind, dir)
		}
	case cesStat:
		if dir != 0 && sub >= 0 && sub < len(customStatSpecs) {
			adjustCustomStat(def, customStatSpecs[sub].id, dir*customStatSpecs[sub].step)
		}
	case cesCore:
		if dir != 0 && sub >= 0 && sub < int(core.StatCount) {
			core.AdjustStat(&def.Stats, core.Stat(sub), dir)
		}
	case cesSkill:
		skills := core.EnemyCastableSkills()
		if (toggle || dir != 0) && sub >= 0 && sub < len(skills) {
			toggleCustomSkill(def, skills[sub])
		}
	}
}

// customEnemyCursorRect returns the rect to outline for the keyboard
// cursor's current row, resolved through the same customEnemyRowAt
// mapping the adjust path uses.
func customEnemyCursorRect(l customEnemyLayout, cursor int) (rl.Rectangle, bool) {
	section, sub := customEnemyRowAt(cursor)
	switch section {
	case cesBase:
		return rl.NewRectangle(l.basePrev.X, l.basePrev.Y, l.baseNext.X+l.baseNext.Width-l.basePrev.X, l.basePrev.Height), true
	case cesStat:
		if sub < len(l.statRows) {
			return l.statRows[sub].rect, true
		}
	case cesCore:
		if sub < len(l.coreStatRows) {
			return l.coreStatRows[sub].rect, true
		}
	case cesSkill:
		if sub >= 0 && sub < len(l.skillRows) {
			return l.skillRows[sub], true
		}
	}
	return rl.Rectangle{}, false
}

func customEnemyNameFieldRect(s *State) rl.Rectangle {
	return customEnemyModalLayout(s).nameField
}

func drawCustomEnemiesModal(s *State, font rl.Font, theme render.Theme) {
	l := customEnemyModalLayout(s)
	drawModalHeaderAt(font, theme, l.card, "CUSTOM ENEMIES", theme.BorderStrong)

	mp := rl.GetMousePosition()

	// Left list. Only the [listTopRow, listEnd) window is painted; the
	// selected row uses the shared gilt DrawSelectedRow treatment (matching
	// the sound modal's saved-list) instead of a bespoke bgActive fill.
	rl.DrawRectangleRec(l.listArea, bgFieldInset)
	rl.DrawRectangleLinesEx(l.listArea, 1, editorBorderInactive)
	for i := l.listTopRow; i < l.listEnd; i++ {
		r := l.listRowRect(i)
		if i == s.modalCustomIdx {
			render.DrawSelectedRow(r)
		} else {
			bg := bgEntry
			if pointIn(mp, r) {
				bg = bgRowHover
			}
			rl.DrawRectangleRec(r, bg)
		}
		name := s.area.CustomEnemies[i].Name
		if name == "" {
			name = "(unnamed)"
		}
		rl.DrawTextEx(font, name, rl.NewVector2(r.X+8, r.Y+(r.Height-editorFontLabel)/2), editorFontLabel, 1, textEntry)
	}
	if l.listTopRow > 0 {
		rl.DrawTextEx(font, "▲ more", rl.NewVector2(l.listArea.X+8, l.listArea.Y-14), editorFontTiny, 1, theme.TextHint)
	}
	if l.listEnd < len(s.area.CustomEnemies) {
		rl.DrawTextEx(font, "▼ more", rl.NewVector2(l.listArea.X+8, l.addBtn.Y-16), editorFontTiny, 1, theme.TextHint)
	}
	drawButton(font, l.addBtn, "+ Add new", false)

	// Right form: if nothing selected, show a placeholder hint and stop.
	def := activeCustomEnemy(s)
	if def == nil {
		formX := l.listArea.X + l.listArea.Width + 16
		rl.DrawTextEx(font, "Select an entry on the left, or click + Add new.",
			rl.NewVector2(formX, l.listArea.Y+8), editorFontLabel, 1, theme.TextHint)
		drawButton(font, l.closeBtn, "Close", false)
		return
	}

	// Name field.
	drawLabel(font, "Name", rl.NewRectangle(l.nameField.X-72, l.nameField.Y+8, 64, 18))
	drawTextField(font, l.nameField, def.Name, s.focus == focusCustomEnemyName)

	// Base sprite picker.
	drawLabel(font, "Base sprite", rl.NewRectangle(l.baseLabel.X, l.baseLabel.Y+8, 72, 18))
	baseName := "?"
	if name, ok := core.EnemyKindName(def.BaseKind); ok {
		baseName = name
	}
	drawButton(font, l.basePrev, "<", false)
	rl.DrawRectangleRec(rl.NewRectangle(l.basePrev.X+l.basePrev.Width+4, l.basePrev.Y, l.baseNext.X-l.basePrev.X-l.basePrev.Width-8, l.basePrev.Height), bgFieldInset)
	rl.DrawTextEx(font, baseName+"  ▼", // ▼ = click the name to pick a base kind from all kinds
		rl.NewVector2(l.basePrev.X+l.basePrev.Width+12, l.basePrev.Y+(l.basePrev.Height-editorFontLabel)/2),
		editorFontLabel, 1, textEntry)
	drawButton(font, l.baseNext, ">", false)

	// Stat rows.
	for _, row := range l.statRows {
		drawLabel(font, row.label, rl.NewRectangle(row.rect.X-78, row.rect.Y+8, 76, 18))
		drawReadonlyValue(font, row.rect, customStatValueString(def, row.id))
		drawStepperButtons(font, row.minus, row.plus)
	}

	// Core stat grid (STR/DEX/INT/WIS/VIT/SPD).
	for i, row := range l.coreStatRows {
		drawLabel(font, row.label, rl.NewRectangle(row.rect.X-32, row.rect.Y+8, 32, 18))
		drawReadonlyValue(font, row.rect, fmt.Sprintf("%d", core.StatValue(def.Stats, core.Stat(i))))
		drawStepperButtons(font, row.minus, row.plus)
	}

	// Skill toggles.
	allSkills := core.EnemyCastableSkills()
	for i, sid := range allSkills {
		row := l.skillRows[i]
		active := false
		for _, have := range def.Skills {
			if have == sid {
				active = true
				break
			}
		}
		bg := bgEntry
		if active {
			bg = bgActive
		} else if pointIn(mp, row) {
			bg = bgRowHover
		}
		rl.DrawRectangleRec(row, bg)
		mark := "[ ]"
		if active {
			mark = "[x]"
		}
		rl.DrawTextEx(font, mark+" "+core.SkillName(sid),
			rl.NewVector2(row.X+8, row.Y+(row.Height-editorFontAccent)/2),
			editorFontAccent, 1, textEntry)
	}

	// Keyboard-cursor highlight on the focused form row (hidden while the
	// name field is captured, since arrows then edit nothing).
	if s.focus != focusCustomEnemyName {
		if rect, ok := customEnemyCursorRect(l, s.modalCursor); ok {
			rl.DrawRectangleLinesEx(rect, 2, theme.BorderActive)
		}
	}

	// Footer buttons. The Delete button highlights while a delete is armed (the
	// first of the two-press confirm) so the primed state stays visible after the
	// flash toast fades.
	delArmed := false
	if def := activeCustomEnemy(s); def != nil {
		delArmed = s.deleteArmed == "custom:"+def.Name
	}
	drawButton(font, l.deleteBtn, "Delete", delArmed)
	drawButton(font, l.closeBtn, "Close", false)
}

// customStatValueString formats the stat's current value for the
// readonly value cell. SkillCastChance renders as a percent; everything
// else as a plain integer.
func customStatValueString(def *core.CustomEnemyDef, id customStatID) string {
	if id == csSkillChance {
		return fmt.Sprintf("%d%%", int(def.SkillCastChance*100+0.5))
	}
	if spec, ok := customStatSpecFor(id); ok && spec.field != nil {
		return fmt.Sprintf("%d", *spec.field(def))
	}
	return "?"
}

// adjustCustomStat applies `delta` to the stat identified by id. Step
// size for SkillChance is the spec's `step` value treated as percent
// (5 → 0.05). Clamps non-negative on the int fields; SkillChance to
// [0, 1].
func adjustCustomStat(def *core.CustomEnemyDef, id customStatID, delta int) {
	if id == csSkillChance {
		def.SkillCastChance = core.Clamp(def.SkillCastChance+float64(delta)/100, 0, 1)
		return
	}
	if spec, ok := customStatSpecFor(id); ok && spec.field != nil {
		v := spec.field(def)
		*v += delta
		if *v < 0 {
			*v = 0
		}
	}
}

// nextCustomEnemyName picks an unused "customN" name so "+ Add new"
// drops the form in a known state. Authors typically rename right
// after, but the auto-name keeps the row clickable from the moment
// it lands.
func nextCustomEnemyName(defs []core.CustomEnemyDef) string {
	taken := make(map[string]bool, len(defs))
	for _, d := range defs {
		taken[d.Name] = true
	}
	return firstUnusedName(taken, "custom%d")
}

// uniqueCustomEnemyName resolves a typed-in name to one that doesn't
// collide with any OTHER entry in defs. Self-comparison is skipped via
// the editingIdx parameter so renaming an entry to itself is a no-op.
// Empty input falls back to a fresh "customN" slot — duplicate empty
// strings would otherwise all map to the same blank row key on save.
// Collisions get "_2", "_3", ... suffixes until free.
func uniqueCustomEnemyName(defs []core.CustomEnemyDef, editingIdx int, name string) string {
	clean := core.SanitizeCustomEnemyName(name)
	if clean == "" {
		return nextCustomEnemyName(defs)
	}
	taken := make(map[string]bool, len(defs))
	for i, d := range defs {
		if i == editingIdx {
			continue
		}
		taken[d.Name] = true
	}
	if !taken[clean] {
		return clean
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", clean, i)
		if !taken[candidate] {
			return candidate
		}
	}
}

// finalizeFocusedField runs any per-field commit hook before focus
// drops. Used by both the in-modal Enter/Tab path AND the click-
// outside-defocus path in editor.Update so a stray click can't
// strand mid-edit state that the modal's own commit would have
// finalized. Today only the custom-enemy name field has a non-
// trivial hook (whitespace sanitize + uniqueness resolve); future
// text fields plug in via the same switch.
func finalizeFocusedField(s *State) {
	switch s.focus {
	case focusCustomEnemyName:
		if def := activeCustomEnemy(s); def != nil {
			oldName := def.Name
			def.Name = uniqueCustomEnemyName(s.area.CustomEnemies, s.modalCustomIdx, def.Name)
			renameCustomEnemyReferences(s, oldName, def.Name)
		}
	case focusWidth, focusHeight:
		// Metadata Width/Height buffer into s.numericBuf and only apply on
		// commit. The Enter/Tab paths call commitNumericInput directly; the
		// click-outside-defocus path goes through here, so without this the
		// typed dimension is silently discarded on click-away.
		commitNumericInput(s)
	}
}

func updateCustomEnemiesModal(s *State) Action {
	if editorCancelPressed() {
		closeModal(s)
		return ActionNone
	}

	// Text input wins while the name field is focused — let the user
	// type letters without them triggering Up/Down list nav etc.
	// Defocus also runs the uniqueness guard: a duplicate name would
	// collide with another entry's row key in the mapfile encoder,
	// so we auto-suffix "_2" / "_3" / ... until the name is free.
	if s.focus == focusCustomEnemyName {
		def := activeCustomEnemy(s)
		if def != nil {
			pumpFocusField(s, &def.Name)
		}
		if editorCommitPressed() || editorTabPressed() {
			finalizeFocusedField(s)
			s.focus = focusNone
		}
	}

	// Keyboard nav/adjust (when not typing the name): Up/Down move a row
	// cursor over base + stats + core + skills; Left/Right adjust the
	// focused numeric/base row; Enter/Space toggles a focused skill. Keeps
	// the modal fully operable without a mouse, matching the editor's
	// keyboard-first pack / chest / door modals.
	if s.focus != focusCustomEnemyName {
		if def := activeCustomEnemy(s); def != nil {
			s.modalCursor = input.CursorUpDown(s.modalCursor, customEnemyRowCount())
			// Left/Right adjusts the focused stat; routed through the input
			// package (keyboard + pad + stick) like the sibling sound modal,
			// rather than raw arrow-key reads.
			dir := input.CursorLeftRight()
			// Enter / pad A toggles a boolean row; Space is kept as a
			// keyboard convenience (safe — this branch is gated off the
			// name-field focus, so it can't eat a typed space).
			toggle := input.EditorConfirmPressed() || rl.IsKeyPressed(rl.KeySpace)
			section, sub := customEnemyRowAt(s.modalCursor)
			// Only snapshot undo + mark dirty when the row actually changes:
			// base/stat/core rows respond to Left/Right (dir); only skill rows
			// toggle on Confirm. Without this, Confirm on a non-skill row added a
			// phantom undo step and falsely raised the unsaved-changes prompt.
			if dir != 0 || (toggle && section == cesSkill) {
				pushUndo(s)
				applyCustomEnemyRow(def, section, sub, dir, toggle)
				s.dirty = true
			}
		}
	}

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		l := customEnemyModalLayout(s)
		// List clicks (visible window only).
		for i := l.listTopRow; i < l.listEnd; i++ {
			if pointIn(mp, l.listRowRect(i)) {
				s.modalCustomIdx = i
				s.focus = focusNone
				return ActionNone
			}
		}
		if pointIn(mp, l.addBtn) {
			pushUndo(s)
			name := nextCustomEnemyName(s.area.CustomEnemies)
			s.area.CustomEnemies = append(s.area.CustomEnemies, core.DefaultCustomEnemy(name, core.EnemyRat))
			s.modalCustomIdx = len(s.area.CustomEnemies) - 1
			s.dirty = true
			return ActionNone
		}
		if pointIn(mp, l.closeBtn) {
			closeModal(s)
			return ActionNone
		}
		def := activeCustomEnemy(s)
		if def == nil {
			return ActionNone
		}
		if pointIn(mp, l.deleteBtn) {
			// Two-press confirm: deleting a custom enemy also rewrites every pack
			// reference to it (blast radius), so guard the first click. The def-name
			// token means a different entry re-arms (can't confirm the wrong delete).
			if !armOrConfirmDelete(s, "custom:"+def.Name, "Delete custom enemy "+def.Name+"? Click Delete again to confirm") {
				return ActionNone
			}
			// Finalize first: if the name field was mid-edit, def.Name holds
			// the uncommitted text while pack-member refs are still keyed on
			// the last-finalized name. Finalizing sanitizes/uniquifies and
			// rewrites refs to def.Name, so the deletedName we read below
			// matches what removeCustomEnemyReferences must clear.
			finalizeFocusedField(s)
			pushUndo(s)
			deletedName := def.Name
			s.area.CustomEnemies = removeModalListItem(s.area.CustomEnemies, s.modalCustomIdx)
			removeCustomEnemyReferences(s, deletedName)
			if s.modalCustomIdx >= len(s.area.CustomEnemies) {
				s.modalCustomIdx = len(s.area.CustomEnemies) - 1
			}
			s.dirty = true
			s.focus = focusNone
			s.flash("Deleted custom enemy " + deletedName)
			return ActionNone
		}
		if pointIn(mp, l.nameField) {
			s.focus = focusCustomEnemyName
			return ActionNone
		}
		// Base / stat / core / skill edits all route through the shared
		// applyCustomEnemyRow (same as the keyboard path) so the per-row
		// semantics live once: a clicked −/+ maps to dir ∓1, a clicked
		// skill row to a toggle.
		if pointIn(mp, l.basePrev) {
			pushUndo(s)
			applyCustomEnemyRow(def, cesBase, 0, -1, false)
			s.dirty = true
			return ActionNone
		}
		if pointIn(mp, l.baseNext) {
			pushUndo(s)
			applyCustomEnemyRow(def, cesBase, 0, +1, false)
			s.dirty = true
			return ActionNone
		}
		// Click the base NAME (between the < > arrows) to open a dropdown of every
		// kind — jump instead of cycling. The arrows still step prev/next.
		if baseSpan := nameSpanBetween(l.basePrev, l.baseNext); pointIn(mp, baseSpan) {
			openDropdownBelow(s, ddCustomBase, baseSpan)
			return ActionNone
		}
		for i, row := range l.statRows {
			if pointIn(mp, row.minus) {
				pushUndo(s)
				applyCustomEnemyRow(def, cesStat, i, -1, false)
				s.dirty = true
				return ActionNone
			}
			if pointIn(mp, row.plus) {
				pushUndo(s)
				applyCustomEnemyRow(def, cesStat, i, +1, false)
				s.dirty = true
				return ActionNone
			}
		}
		for i, row := range l.coreStatRows {
			if pointIn(mp, row.minus) {
				pushUndo(s)
				applyCustomEnemyRow(def, cesCore, i, -1, false)
				s.dirty = true
				return ActionNone
			}
			if pointIn(mp, row.plus) {
				pushUndo(s)
				applyCustomEnemyRow(def, cesCore, i, +1, false)
				s.dirty = true
				return ActionNone
			}
		}
		for i := range l.skillRows {
			if pointIn(mp, l.skillRows[i]) {
				pushUndo(s)
				applyCustomEnemyRow(def, cesSkill, i, 0, true)
				s.dirty = true
				return ActionNone
			}
		}
	}
	return ActionNone
}

func renameCustomEnemyReferences(s *State, oldName, newName string) {
	if oldName == "" || oldName == newName {
		return
	}
	for i := range s.area.PackSpawns {
		sp := &s.area.PackSpawns[i]
		for j := range sp.Members {
			if core.PackMemberCustomName(*sp, j) == oldName {
				sp.Members[j].CustomName = newName
			}
		}
	}
}

func removeCustomEnemyReferences(s *State, name string) {
	if name == "" {
		return
	}
	for i := range s.area.PackSpawns {
		sp := &s.area.PackSpawns[i]
		for j := range sp.Members {
			if core.PackMemberCustomName(*sp, j) == name {
				sp.Members[j].CustomName = ""
			}
		}
	}
}

// cycleEnemyKind walks the enemy registry by delta (+1 / -1), wrapping
// at the ends. Skips nothing — every registered kind is a valid base
// sprite choice. Used by the modal's < / > buttons next to the Base
// label.
func cycleEnemyKind(cur core.EnemyKind, delta int) core.EnemyKind {
	defs := core.EnemyKinds()
	if len(defs) == 0 {
		return cur
	}
	idx := 0
	for i, def := range defs {
		if def.Kind == cur {
			idx = i
			break
		}
	}
	idx = core.WrapIndex(idx+delta, len(defs))
	return defs[idx].Kind
}

// toggleCustomSkill flips a skill's presence in def.Skills. Adding an
// already-present skill is a no-op; removing an absent skill is too.
// Centralized so the modal click handler doesn't open-code the
// contains-check + slice-remove dance.
func toggleCustomSkill(def *core.CustomEnemyDef, id core.SkillID) {
	for i, have := range def.Skills {
		if have == id {
			def.Skills = removeModalListItem(def.Skills, i)
			return
		}
	}
	def.Skills = append(def.Skills, id)
}
