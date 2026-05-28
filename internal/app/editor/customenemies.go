package editor

import (
	"crawler/internal/app/core"
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
	listRows []rl.Rectangle
	addBtn   rl.Rectangle

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
	{csMP, "MP", 1, func(d *core.CustomEnemyDef) *int { return &d.MP }},
	{csAttack, "Attack", 1, func(d *core.CustomEnemyDef) *int { return &d.AttackDamage }},
	{csArmor, "Armor", 1, func(d *core.CustomEnemyDef) *int { return &d.Armor }},
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
	sw, sh := render.ScreenSizeF()
	card := rl.NewRectangle((sw-customCardWidth)/2, (sh-customCardHeight)/2, customCardWidth, customCardHeight)

	l := customEnemyLayout{card: card}
	// List column on the left, below the heading reserve.
	listX := card.X + 16
	listY := card.Y + 56
	listH := card.Height - 56 - 56 // leave footer for Delete/Close
	l.listArea = rl.NewRectangle(listX, listY, customListWidth, listH)
	l.listRows = make([]rl.Rectangle, len(s.area.CustomEnemies))
	for i := range s.area.CustomEnemies {
		l.listRows[i] = rl.NewRectangle(listX, listY+float32(i)*customRowHeight, customListWidth, customRowHeight-2)
	}
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
		l.statRows[i] = customStatRow{
			id:    spec.id,
			label: spec.label,
			rect:  rl.NewRectangle(formX+82, rowY, 76, 30),
			minus: rl.NewRectangle(formX+162, rowY, 30, 30),
			plus:  rl.NewRectangle(formX+196, rowY, 30, 30),
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
		cellX := formX + float32(col)*(cellW+6)
		cellY := formY + float32(row)*(cellH+6)
		l.coreStatRows[i] = customStatRow{
			label: core.StatLabel(core.Stat(i)),
			rect:  rl.NewRectangle(cellX+34, cellY, 40, cellH),
			minus: rl.NewRectangle(cellX+78, cellY, 24, cellH),
			plus:  rl.NewRectangle(cellX+104, cellY, 24, cellH),
		}
	}
	formY += float32((int(core.StatCount)+csCols-1)/csCols) * (cellH + 6)

	skills := core.EnemyCastableSkills()
	l.skillRows = make([]rl.Rectangle, len(skills))
	for i := range skills {
		l.skillRows[i] = rl.NewRectangle(formX, formY+float32(i)*24, formW, 22)
	}

	// Footer.
	btnY := card.Y + card.Height - 44
	l.deleteBtn = rl.NewRectangle(card.X+16, btnY, 110, 30)
	l.closeBtn = rl.NewRectangle(card.X+card.Width-94, btnY, 80, 30)
	return l
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
	if len(s.area.CustomEnemies) > 0 && s.modalCustomIdx < 0 {
		s.modalCustomIdx = 0
	}
}

func customEnemyNameFieldRect(s *State) rl.Rectangle {
	return customEnemyModalLayout(s).nameField
}

func drawCustomEnemiesModal(s *State, font rl.Font, theme render.Theme) {
	l := customEnemyModalLayout(s)
	drawModalVeil(theme)
	render.DrawCard(int32(l.card.X), int32(l.card.Y), int32(l.card.Width), int32(l.card.Height),
		theme.SurfacePrimary, theme.BorderSoft, theme.BorderStrong)
	render.DrawHeading(font, "CUSTOM ENEMIES", int32(l.card.X+16), int32(l.card.Y+12), theme.BorderStrong)

	mp := rl.GetMousePosition()

	// Left list.
	rl.DrawRectangleRec(l.listArea, bgFieldInset)
	rl.DrawRectangleLinesEx(l.listArea, 1, editorBorderInactive)
	for i, r := range l.listRows {
		bg := bgEntry
		if i == s.modalCustomIdx {
			bg = bgActive
		} else if pointIn(mp, r) {
			bg = bgRowHover
		}
		rl.DrawRectangleRec(r, bg)
		name := s.area.CustomEnemies[i].Name
		if name == "" {
			name = "(unnamed)"
		}
		rl.DrawTextEx(font, name, rl.NewVector2(r.X+8, r.Y+(r.Height-14)/2), 14, 1, textEntry)
	}
	drawButton(font, l.addBtn, "+ Add new", false)

	// Right form: if nothing selected, show a placeholder hint and stop.
	def := activeCustomEnemy(s)
	if def == nil {
		formX := l.listArea.X + l.listArea.Width + 16
		rl.DrawTextEx(font, "Select an entry on the left, or click + Add new.",
			rl.NewVector2(formX, l.listArea.Y+8), 14, 1, theme.TextHint)
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
	rl.DrawTextEx(font, baseName,
		rl.NewVector2(l.basePrev.X+l.basePrev.Width+12, l.basePrev.Y+(l.basePrev.Height-14)/2),
		14, 1, textEntry)
	drawButton(font, l.baseNext, ">", false)

	// Stat rows.
	for _, row := range l.statRows {
		drawLabel(font, row.label, rl.NewRectangle(row.rect.X-78, row.rect.Y+8, 76, 18))
		drawReadonlyValue(font, row.rect, customStatValueString(def, row.id))
		drawButton(font, row.minus, "-", false)
		drawButton(font, row.plus, "+", false)
	}

	// Core stat grid (STR/DEX/INT/WIS/VIT/SPD).
	for i, row := range l.coreStatRows {
		drawLabel(font, row.label, rl.NewRectangle(row.rect.X-32, row.rect.Y+8, 32, 18))
		drawReadonlyValue(font, row.rect, fmt.Sprintf("%d", core.StatValue(def.Stats, core.Stat(i))))
		drawButton(font, row.minus, "-", false)
		drawButton(font, row.plus, "+", false)
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
			rl.NewVector2(row.X+8, row.Y+(row.Height-13)/2),
			13, 1, textEntry)
	}

	// Footer buttons.
	drawButton(font, l.deleteBtn, "Delete", false)
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

// stepSizeFor returns the per-click numeric step for a stat row.
// SkillChance steps by 5 (interpreted as percent → 0.05); everything
// else by 1. Single seam so the spec can grow without the click handler
// having to know the math.
func stepSizeFor(id customStatID) int {
	for _, spec := range customStatSpecs {
		if spec.id == id {
			return spec.step
		}
	}
	return 1
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
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("custom%d", i)
		if !taken[candidate] {
			return candidate
		}
	}
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

	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		l := customEnemyModalLayout(s)
		// List clicks.
		for i, r := range l.listRows {
			if pointIn(mp, r) {
				s.modalCustomIdx = i
				s.focus = focusNone
				return ActionNone
			}
		}
		if pointIn(mp, l.addBtn) {
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
			// Finalize first: if the name field was mid-edit, def.Name holds
			// the uncommitted text while pack-member refs are still keyed on
			// the last-finalized name. Finalizing sanitizes/uniquifies and
			// rewrites refs to def.Name, so the deletedName we read below
			// matches what removeCustomEnemyReferences must clear.
			finalizeFocusedField(s)
			deletedName := def.Name
			s.area.CustomEnemies = append(s.area.CustomEnemies[:s.modalCustomIdx], s.area.CustomEnemies[s.modalCustomIdx+1:]...)
			removeCustomEnemyReferences(s, deletedName)
			if s.modalCustomIdx >= len(s.area.CustomEnemies) {
				s.modalCustomIdx = len(s.area.CustomEnemies) - 1
			}
			s.dirty = true
			s.focus = focusNone
			return ActionNone
		}
		if pointIn(mp, l.nameField) {
			s.focus = focusCustomEnemyName
			return ActionNone
		}
		if pointIn(mp, l.basePrev) {
			def.BaseKind = cycleEnemyKind(def.BaseKind, -1)
			s.dirty = true
			return ActionNone
		}
		if pointIn(mp, l.baseNext) {
			def.BaseKind = cycleEnemyKind(def.BaseKind, +1)
			s.dirty = true
			return ActionNone
		}
		for _, row := range l.statRows {
			step := stepSizeFor(row.id)
			if pointIn(mp, row.minus) {
				adjustCustomStat(def, row.id, -step)
				s.dirty = true
				return ActionNone
			}
			if pointIn(mp, row.plus) {
				adjustCustomStat(def, row.id, step)
				s.dirty = true
				return ActionNone
			}
		}
		for i, row := range l.coreStatRows {
			if pointIn(mp, row.minus) {
				core.AdjustStat(&def.Stats, core.Stat(i), -1)
				s.dirty = true
				return ActionNone
			}
			if pointIn(mp, row.plus) {
				core.AdjustStat(&def.Stats, core.Stat(i), +1)
				s.dirty = true
				return ActionNone
			}
		}
		allSkills := core.EnemyCastableSkills()
		for i, r := range l.skillRows {
			if pointIn(mp, r) {
				toggleCustomSkill(def, allSkills[i])
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
	idx = (idx + delta + len(defs)) % len(defs)
	return defs[idx].Kind
}

// toggleCustomSkill flips a skill's presence in def.Skills. Adding an
// already-present skill is a no-op; removing an absent skill is too.
// Centralized so the modal click handler doesn't open-code the
// contains-check + slice-remove dance.
func toggleCustomSkill(def *core.CustomEnemyDef, id core.SkillID) {
	for i, have := range def.Skills {
		if have == id {
			def.Skills = append(def.Skills[:i], def.Skills[i+1:]...)
			return
		}
	}
	def.Skills = append(def.Skills, id)
}
