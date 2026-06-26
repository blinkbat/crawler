package explore

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
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
	if n <= 0 {
		return 0
	}
	return core.Clamp(cursor, 0, n-1)
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
// battle support skill (heal or cure). Single-ally opens the picker; party-wide
// (Mass Mend) and self (Second Wind) apply immediately and pay MP now.
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
	// Affordable out-of-battle skills: none → refuse; one → cast; several → chooser
	// (filtered to affordable so it never lists a cast beginSkillCast would refuse).
	skills := affordableOutOfBattleSkills(m)
	switch len(skills) {
	case 0:
		audio.Play(audio.SoundInputMiss) // no out-of-battle support skill
	case 1:
		beginSkillCast(g, caster, skills[0])
	default:
		g.HealPickOpen = true
		g.HealPickCaster = caster
		g.HealPickCursor = 0
	}
}

// affordableOutOfBattleSkills returns the member's affordable out-of-battle support
// skills. The chooser and its driver share it so their lists can't diverge.
// Reusable per-frame buffers; returned slices valid until the next call.
var (
	outOfBattleSkillsBuf []core.SkillID
	affordableSkillsBuf  []core.SkillID
	useTargetLivingBuf   []int
)

func affordableOutOfBattleSkills(m *core.PartyMember) []core.SkillID {
	outOfBattleSkillsBuf = core.OutOfBattleSupportSkillsInto(outOfBattleSkillsBuf, m)
	affordableSkillsBuf = affordableSkillsBuf[:0]
	for _, s := range outOfBattleSkillsBuf {
		if core.CanAffordSkill(m, s) {
			affordableSkillsBuf = append(affordableSkillsBuf, s)
		}
	}
	return affordableSkillsBuf
}

// beginSkillCast resolves a chosen support skill the way battle does: a single-ally
// skill opens the picker (MP billed on apply); a party-wide heal or a self skill
// applies now. Re-checks MP/corpse and that it'll do something before casting.
func beginSkillCast(g *core.GameState, caster int, skill core.SkillID) {
	m, ok := validMember(g, caster)
	if !ok || m.HP <= 0 {
		audio.Play(audio.SoundInputMiss)
		return
	}
	if !core.CanAffordSkill(m, skill) {
		audio.Play(audio.SoundInputMiss) // not enough MP
		return
	}
	switch core.OutOfBattleSkillScopeFor(skill) {
	case core.SkillScopeAlly:
		// Single-ally heal/cure: pick the recipient (benefit re-checked on apply).
		openUseTargetForSkill(g, caster, skill)
	case core.SkillScopeParty:
		// Party-wide heal (Mass Mend): no target step. Refuse if no member can
		// benefit so it can't drain MP for zero effect.
		if !anyMemberBelowFull(g) {
			audio.Play(audio.SoundInputMiss)
			return
		}
		core.HealWholeParty(g, core.SkillHealFor(m, skill))
		core.SpendSkillMP(m, skill)
		audio.Play(audio.SoundHeal)
	default: // SkillScopeSelf (Second Wind): heal the caster.
		if !core.MemberCanBeHealed(*m) {
			audio.Play(audio.SoundInputMiss)
			return
		}
		core.HealMember(m, core.SkillHealFor(m, skill))
		core.SpendSkillMP(m, skill)
		audio.Play(audio.SoundHeal)
	}
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
	caster := g.HealPickCaster
	m, ok := validMember(g, caster)
	if !ok {
		closeHealPick(g)
		return
	}
	skills := affordableOutOfBattleSkills(m)
	updateListPicker(&g.HealPickCursor, len(skills), func() { closeHealPick(g) }, func(item int) {
		skill := skills[item]
		closeHealPick(g)
		beginSkillCast(g, caster, skill)
	})
}

// updateUseTargetPicker drives the shared ally-target sub-modal: Up/Down walk the
// living members, Confirm applies the pending item/skill, Back closes the picker.
func updateUseTargetPicker(g *core.GameState) {
	living := core.LivingPartyIndicesInto(useTargetLivingBuf, g.Party)
	useTargetLivingBuf = living
	updateListPicker(&g.UseTargetCursor, len(living), func() { closeUseTarget(g) }, func(item int) {
		applyUseToMember(g, living[item])
	})
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
		// core.ApplyRestorative owns the feed→heal→restore order (shared with battle).
		res := core.ApplyRestorative(m, def)
		// Consuming an item out of combat is a real action — log it like the battle path.
		g.LogMessageCat(core.ItemUseMessage(m.Name, def, res), core.RestorativeUseCategory(res))
		audio.Play(audio.SoundHeal)
	case g.UsePendingSkill != core.SkillNone:
		skill := g.UsePendingSkill
		c, ok := validMember(g, g.UsePendingCaster)
		if !ok || c.HP <= 0 {
			// Caster out of range or died between open and confirm — a corpse
			// can't pay MP or cast.
			break
		}
		recipient, ok := validMember(g, member)
		if !ok {
			break
		}
		// Don't burn MP on a cast that does nothing: a cure needs a curable debuff,
		// a heal needs missing HP. Checked before spending MP (mirrors the battle guards).
		cure := core.SkillCuresDebuffs(skill)
		if (cure && !core.HasCurableDebuff(recipient)) || (!cure && !core.MemberCanBeHealed(*recipient)) {
			audio.Play(audio.SoundInputMiss)
			break
		}
		if !core.SpendSkillMP(c, skill) {
			audio.Play(audio.SoundInputMiss) // MP drained between open and confirm
			break
		}
		if cure {
			core.CureDebuffs(recipient)
		} else {
			core.HealMember(recipient, core.SkillHealFor(c, skill))
		}
		audio.Play(audio.SoundHeal)
	}
	closeUseTarget(g)
}

// setUseTarget is the single seam for the five ally-target picker fields; open/
// close and the item/skill variants all route through it so they can't diverge.
func setUseTarget(g *core.GameState, open bool, cursor int, item core.ItemKind, skill core.SkillID, caster int) {
	g.UseTargetOpen = open
	g.UseTargetCursor = cursor
	g.UsePendingItem = item
	g.UsePendingSkill = skill
	g.UsePendingCaster = caster
}

// openUseTargetForItem opens the ally-target picker carrying a pending item.
func openUseTargetForItem(g *core.GameState, kind core.ItemKind) {
	setUseTarget(g, true, 0, kind, core.SkillNone, noCaster)
}

// openUseTargetForSkill opens the picker carrying a pending single-target heal
// by `caster`.
func openUseTargetForSkill(g *core.GameState, caster int, skill core.SkillID) {
	setUseTarget(g, true, 0, core.ItemNone, skill, caster)
}

// closeUseTarget dismisses the ally-target picker and clears its pending state.
// Called on apply, Back, and overlay close / tab switch.
func closeUseTarget(g *core.GameState) {
	setUseTarget(g, false, 0, core.ItemNone, core.SkillNone, noCaster)
}
