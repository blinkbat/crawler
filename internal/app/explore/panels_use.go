package explore

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// Out-of-battle "use" actions on the panels overlay (Items consumables, Skills
// heals). When a recipient is needed both route through one shared ally-target
// sub-modal of the living members; party-wide heals skip it and apply to everyone.

// noCaster is the "no pending caster" sentinel for g.UsePendingCaster (an item is
// pending). A real caster is a valid party seat (>= 0).
const noCaster = -1

// validMember resolves idx to a live *g.Party pointer (ok=false when out of range).
func validMember(g *core.GameState, idx int) (*core.PartyMember, bool) {
	if !core.PartyIndexInRange(g.Party, idx) {
		return nil, false
	}
	return &g.Party[idx], true
}

// stackAtCursor is the materialized-list cursor guard shared by the panels/shop/
// chest paths (generic over element type): bounds-check cursor, return the row.
func stackAtCursor[T any](stacks []T, cursor int) (T, bool) {
	if cursor < 0 || cursor >= len(stacks) {
		var zero T
		return zero, false
	}
	return stacks[cursor], true
}

// clampCursorToLen keeps a cursor in range after the list shrank (last row, or 0
// for empty). Shared by the shop-sell and chest-take paths.
func clampCursorToLen(cursor, n int) int {
	if cursor >= n {
		cursor = n - 1
	}
	if cursor < 0 {
		cursor = 0
	}
	return cursor
}

// tryUseItem handles a use press on the Items tab: a restorative opens the
// ally-target picker; equipment / no-effect rows ping miss.
func tryUseItem(g *core.GameState) {
	stacks := core.LiveStacks(g.Inventory)
	stack, ok := stackAtCursor(stacks, g.PanelsRowCursor)
	if !ok {
		return
	}
	kind := stack.Kind
	def := core.ItemInfo(kind)
	if !core.ItemIsRestorative(def) {
		audio.Play(audio.SoundInputMiss) // equipment / no restorative effect
		return
	}
	openUseTargetForItem(g, kind)
}

// tryUseSkill handles a Use press on the Skills tab: cast an affordable out-of-
// battle heal. Single-target opens the ally picker; party-wide (Mass Mend)
// applies immediately and pays MP now.
func tryUseSkill(g *core.GameState) {
	caster := g.PanelsRowCursor
	m, ok := validMember(g, caster)
	if !ok {
		return
	}
	if m.HP <= 0 {
		audio.Play(audio.SoundInputMiss) // a downed member keeps HP=0 into exploration; no corpse cast
		return
	}
	// Affordable out-of-battle heals: none → refuse; one → cast; several → chooser
	// (filtered to affordable so it never lists a cast beginHealCast would refuse).
	heals := affordableOutOfBattleHeals(m)
	switch len(heals) {
	case 0:
		audio.Play(audio.SoundInputMiss) // no out-of-battle heal
	case 1:
		beginHealCast(g, caster, heals[0])
	default:
		g.HealPickOpen = true
		g.HealPickCaster = caster
		g.HealPickCursor = 0
	}
}

// affordableOutOfBattleHeals returns the member's affordable out-of-battle heals.
// The chooser and its driver share it so their lists can't diverge.
// Reusable per-frame buffers; returned slices valid until the next call.
var (
	outOfBattleHealsBuf []core.SkillID
	affordableHealsBuf  []core.SkillID
	useTargetLivingBuf  []int
)

func affordableOutOfBattleHeals(m *core.PartyMember) []core.SkillID {
	outOfBattleHealsBuf = core.OutOfBattleHealsInto(outOfBattleHealsBuf, m)
	affordableHealsBuf = affordableHealsBuf[:0]
	for _, h := range outOfBattleHealsBuf {
		if core.CanAffordSkill(m, h) {
			affordableHealsBuf = append(affordableHealsBuf, h)
		}
	}
	return affordableHealsBuf
}

// beginHealCast resolves a chosen heal: single-target opens the ally picker (MP
// billed on apply); party-wide applies now. Re-checks MP/corpse before casting.
func beginHealCast(g *core.GameState, caster int, skill core.SkillID) {
	m, ok := validMember(g, caster)
	if !ok || m.HP <= 0 {
		audio.Play(audio.SoundInputMiss)
		return
	}
	if !core.CanAffordSkill(m, skill) {
		audio.Play(audio.SoundInputMiss) // not enough MP
		return
	}
	if core.SkillTargetMode(skill) == core.ActionPartyTarget {
		// Single-target heal: pick the recipient.
		openUseTargetForSkill(g, caster, skill)
		return
	}
	// Party-wide heal: no target step. Refuse if no member can benefit so it
	// can't drain MP for zero effect.
	if !anyMemberBelowFull(g) {
		audio.Play(audio.SoundInputMiss)
		return
	}
	amount := core.SkillHealFor(m, skill)
	core.HealWholeParty(g, amount)
	core.SpendSkillMP(m, skill)
	audio.Play(audio.SoundHeal)
}

// anyMemberBelowFull reports whether any member can benefit from an HP heal (via
// core.MemberCanBeHealed, shared with the item path).
func anyMemberBelowFull(g *core.GameState) bool {
	for i := range g.Party {
		if core.MemberCanBeHealed(g.Party[i]) {
			return true
		}
	}
	return false
}

// closeHealPick dismisses the heal chooser. Called on apply, Back, and overlay
// close / tab switch.
func closeHealPick(g *core.GameState) {
	g.HealPickOpen = false
	// noCaster, not 0: readers treat caster < 0 as "no caster" and bail; 0 is a
	// valid party index that would linger in a closed picker.
	g.HealPickCaster = noCaster
	g.HealPickCursor = 0
}

// updateHealPicker drives the heal-skill chooser: Up/Down walk the caster's
// heals, Confirm commits into beginHealCast, Back cancels.
func updateHealPicker(g *core.GameState) {
	if input.BackPressed() {
		closeHealPick(g)
		return
	}
	caster := g.HealPickCaster
	m, ok := validMember(g, caster)
	if !ok {
		closeHealPick(g)
		return
	}
	heals := affordableOutOfBattleHeals(m)
	if len(heals) == 0 {
		closeHealPick(g)
		return
	}
	g.HealPickCursor = input.CursorUpDown(g.HealPickCursor, len(heals))
	if input.ConfirmPressed() && g.HealPickCursor >= 0 && g.HealPickCursor < len(heals) {
		skill := heals[g.HealPickCursor]
		closeHealPick(g)
		beginHealCast(g, caster, skill)
	}
}

// updateUseTargetPicker drives the shared ally-target sub-modal: Up/Down walk the
// living members, Confirm applies the pending item/skill, Back closes the picker.
func updateUseTargetPicker(g *core.GameState) {
	if input.BackPressed() {
		closeUseTarget(g)
		return
	}
	living := core.LivingPartyIndicesInto(useTargetLivingBuf, g.Party)
	useTargetLivingBuf = living
	if len(living) == 0 {
		closeUseTarget(g)
		return
	}
	g.UseTargetCursor = input.CursorUpDown(g.UseTargetCursor, len(living))
	if input.ConfirmPressed() && g.UseTargetCursor >= 0 && g.UseTargetCursor < len(living) {
		applyUseToMember(g, living[g.UseTargetCursor])
	}
}

// applyUseToMember resolves the pending use against the chosen member: an item
// consumes one stack and heals; a skill heals and bills MP. Closes the picker.
func applyUseToMember(g *core.GameState, member int) {
	switch {
	case g.UsePendingItem != core.ItemNone:
		kind := g.UsePendingItem
		def := core.ItemInfo(kind)
		m := &g.Party[member]
		// Don't burn a restorative on a full ally (core.ItemHelpsTarget, shared
		// with the battle-side applyItem guard).
		if !core.ItemHelpsTarget(def, *m) {
			audio.Play(audio.SoundInputMiss)
			break
		}
		inv, ok := core.ConsumeItem(g.Inventory, kind)
		if !ok {
			audio.Play(audio.SoundInputMiss)
			break
		}
		g.Inventory = inv
		core.HealMember(m, def.HealAmount)
		core.RestoreMP(m, def.MPAmount)
		audio.Play(audio.SoundHeal)
	case g.UsePendingSkill != core.SkillNone:
		skill := g.UsePendingSkill
		c, ok := validMember(g, g.UsePendingCaster)
		if !ok || c.HP <= 0 {
			// Caster out of range or died between open and confirm — a corpse
			// can't pay MP or cast.
			break
		}
		// Don't burn MP healing an ally who can't benefit (core.MemberCanBeHealed).
		// Checked before spending MP, and bounds-checked first so it can't panic.
		recipient, ok := validMember(g, member)
		if !ok || !core.MemberCanBeHealed(*recipient) {
			audio.Play(audio.SoundInputMiss)
			break
		}
		heal := core.SkillHealFor(c, skill)
		if !core.SpendSkillMP(c, skill) {
			audio.Play(audio.SoundInputMiss) // MP drained between open and confirm
			break
		}
		core.HealMember(recipient, heal)
		audio.Play(audio.SoundHeal)
	}
	closeUseTarget(g)
}

// openUseTargetForItem opens the ally-target picker carrying a pending item
// (inverse of closeUseTarget — both set the same five fields).
func openUseTargetForItem(g *core.GameState, kind core.ItemKind) {
	g.UseTargetOpen = true
	g.UseTargetCursor = 0
	g.UsePendingItem = kind
	g.UsePendingSkill = core.SkillNone
	g.UsePendingCaster = noCaster
}

// openUseTargetForSkill opens the picker carrying a pending single-target heal
// by `caster`. Mirror of openUseTargetForItem / closeUseTarget.
func openUseTargetForSkill(g *core.GameState, caster int, skill core.SkillID) {
	g.UseTargetOpen = true
	g.UseTargetCursor = 0
	g.UsePendingItem = core.ItemNone
	g.UsePendingSkill = skill
	g.UsePendingCaster = caster
}

// closeUseTarget dismisses the ally-target picker and clears its pending state.
// Called on apply, Back, and overlay close / tab switch.
func closeUseTarget(g *core.GameState) {
	g.UseTargetOpen = false
	g.UseTargetCursor = 0
	g.UsePendingItem = core.ItemNone
	g.UsePendingSkill = core.SkillNone
	g.UsePendingCaster = noCaster
}
