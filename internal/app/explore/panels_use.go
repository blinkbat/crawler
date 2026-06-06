package explore

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// Out-of-battle "use" actions on the panels overlay: using a consumable
// from the Items tab and casting a heal skill from the Skills tab. When
// a recipient must be chosen, both route through one shared ally-target
// sub-modal (UseTargetOpen) listing the living party members — you can't
// heal the dead out of battle. Party-wide heals (Mass Mend) skip the
// picker and apply to everyone at once.

// noCaster is the "no pending caster" sentinel for g.UsePendingCaster —
// set when an item (not a skill) is pending, and on close. A real caster
// is always a valid party seat index (>= 0).
const noCaster = -1

// tryUseItem handles a use press on the Items tab. The cursored stack
// must be a healing consumable; it opens the ally-target picker carrying
// that item. Equipment / no-effect rows ping the miss cue.
func tryUseItem(g *core.GameState) {
	stacks := core.LiveStacks(g.Inventory)
	idx := g.PanelsRowCursor
	if idx < 0 || idx >= len(stacks) {
		return
	}
	kind := stacks[idx].Kind
	if core.ItemInfo(kind).HealAmount <= 0 {
		audio.Play(audio.SoundInputMiss) // equipment / non-healing item
		return
	}
	g.UseTargetOpen = true
	g.UseTargetCursor = 0
	g.UsePendingItem = kind
	g.UsePendingSkill = core.SkillNone
	g.UsePendingCaster = noCaster
}

// tryUseSkill handles a Use press on the Skills tab. The cursored skill
// must be a heal castable out of battle (Prayer / Mass Mend) and the
// member must afford its MP. Single-target heals open the ally picker;
// party-wide heals (Mass Mend) apply to the whole party immediately and
// pay the MP now.
func tryUseSkill(g *core.GameState) {
	caster := g.PanelsRowCursor
	if caster < 0 || caster >= len(g.Party) {
		return
	}
	if g.Party[caster].HP <= 0 {
		// A member downed in a won battle keeps HP=0 with MP intact into
		// exploration; the Skills-tab cursor can still land on its column.
		// Don't let a corpse cast.
		audio.Play(audio.SoundInputMiss)
		return
	}
	// The Skills tab is a tree summary now (Confirm opens the tree modal), so
	// there's no per-skill cursor. Gather the member's out-of-battle heals and:
	// none → refuse; one → cast it directly; several (Cleric's Prayer + Mass
	// Mend) → pop a chooser so both stay reachable.
	heals := core.OutOfBattleHeals(g.Party[caster])
	switch len(heals) {
	case 0:
		audio.Play(audio.SoundInputMiss) // this member has no out-of-battle heal
	case 1:
		beginHealCast(g, caster, heals[0])
	default:
		g.HealPickOpen = true
		g.HealPickCaster = caster
		g.HealPickCursor = 0
	}
}

// beginHealCast resolves a chosen out-of-battle heal for `caster`: a
// single-target heal (Prayer) opens the ally-target picker; a party-wide heal
// (Mass Mend) applies to everyone immediately. Either way the caster pays the
// MP (the single-target path bills it on apply, in applyUseToMember). A short
// MP / corpse re-check guards the gap between choosing and casting.
func beginHealCast(g *core.GameState, caster int, skill core.SkillID) {
	if caster < 0 || caster >= len(g.Party) || g.Party[caster].HP <= 0 {
		audio.Play(audio.SoundInputMiss)
		return
	}
	if !core.CanAffordSkill(g.Party[caster], skill) {
		audio.Play(audio.SoundInputMiss) // not enough MP
		return
	}
	if core.SkillTargetMode(skill) == core.ActionPartyTarget {
		// Single-target heal (Prayer): pick the recipient.
		g.UseTargetOpen = true
		g.UseTargetCursor = 0
		g.UsePendingItem = core.ItemNone
		g.UsePendingSkill = skill
		g.UsePendingCaster = caster
		return
	}
	// Party-wide heal (Mass Mend): no target step. HealMember no-ops on
	// the dead and clamps at MaxHP, so this is safe to fan across all.
	amount := core.SkillHealFor(&g.Party[caster], skill)
	for i := range g.Party {
		core.HealMember(&g.Party[i], amount)
	}
	core.SpendSkillMP(&g.Party[caster], skill)
	audio.Play(audio.SoundHeal)
}

// closeHealPick dismisses the out-of-battle heal chooser. Called on apply, on
// Back, and on overlay close / tab switch.
func closeHealPick(g *core.GameState) {
	g.HealPickOpen = false
	g.HealPickCaster = 0
	g.HealPickCursor = 0
}

// updateHealPicker drives the heal-skill chooser. Up/Down walk the caster's
// out-of-battle heals, Confirm commits the chosen one into beginHealCast, Back
// cancels. It owns panel input while open (gated in updatePanels).
func updateHealPicker(g *core.GameState) {
	if input.BackPressed() {
		closeHealPick(g)
		return
	}
	caster := g.HealPickCaster
	if caster < 0 || caster >= len(g.Party) {
		closeHealPick(g)
		return
	}
	heals := core.OutOfBattleHeals(g.Party[caster])
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

// updateUseTargetPicker drives the shared ally-target sub-modal. Up/Down
// walk the living members, Confirm applies the pending item/skill to the
// focused member, Back cancels. It owns panel input while open (gated in
// updatePanels), so Back closes only the picker.
func updateUseTargetPicker(g *core.GameState) {
	if input.BackPressed() {
		closeUseTarget(g)
		return
	}
	living := core.LivingPartyIndices(g.Party)
	if len(living) == 0 {
		closeUseTarget(g)
		return
	}
	g.UseTargetCursor = input.CursorUpDown(g.UseTargetCursor, len(living))
	if input.ConfirmPressed() && g.UseTargetCursor >= 0 && g.UseTargetCursor < len(living) {
		applyUseToMember(g, living[g.UseTargetCursor])
	}
}

// applyUseToMember resolves the pending use against the chosen member:
// an item consumes one stack and heals; a skill heals and bills the
// caster's MP. Either way the picker closes afterward.
func applyUseToMember(g *core.GameState, member int) {
	switch {
	case g.UsePendingItem != core.ItemNone:
		kind := g.UsePendingItem
		def := core.ItemInfo(kind)
		// Don't burn a heal item on a full-HP ally — parity with the battle-side
		// applyItem guard; without it the stack is consumed for zero gain
		// (HealMember clamps at MaxHP).
		if def.HealAmount > 0 && g.Party[member].HP >= g.Party[member].MaxHP {
			audio.Play(audio.SoundInputMiss)
			break
		}
		inv, ok := core.ConsumeItem(g.Inventory, kind)
		if !ok {
			audio.Play(audio.SoundInputMiss)
			break
		}
		g.Inventory = inv
		core.HealMember(&g.Party[member], def.HealAmount)
		audio.Play(audio.SoundHeal)
	case g.UsePendingSkill != core.SkillNone:
		caster := g.UsePendingCaster
		skill := g.UsePendingSkill
		if caster < 0 || caster >= len(g.Party) || g.Party[caster].HP <= 0 {
			// Caster out of range, or died between opening the picker and
			// confirming — a corpse can't pay MP or cast.
			break
		}
		heal := core.SkillHealFor(&g.Party[caster], skill)
		if !core.SpendSkillMP(&g.Party[caster], skill) {
			audio.Play(audio.SoundInputMiss) // MP drained between open and confirm
			break
		}
		core.HealMember(&g.Party[member], heal)
		audio.Play(audio.SoundHeal)
	}
	closeUseTarget(g)
}

// closeUseTarget dismisses the ally-target picker and clears its pending
// state. Called on apply, on Back, and on overlay close / tab switch.
func closeUseTarget(g *core.GameState) {
	g.UseTargetOpen = false
	g.UseTargetCursor = 0
	g.UsePendingItem = core.ItemNone
	g.UsePendingSkill = core.SkillNone
	g.UsePendingCaster = noCaster
}
