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
	skills := core.PartySkills(g.Party[caster])
	row := g.PanelsSkillRow
	if row < 0 || row >= len(skills) {
		return
	}
	skill := skills[row]
	if !core.SkillHealableOutOfBattle(skill) {
		audio.Play(audio.SoundInputMiss) // not a heal / not castable out of battle
		return
	}
	if g.Party[caster].MP < core.SkillCost(skill) {
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
	g.Party[caster].MP -= core.SkillCost(skill)
	audio.Play(audio.SoundHeal)
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
		inv, ok := core.ConsumeItem(g.Inventory, kind)
		if !ok {
			audio.Play(audio.SoundInputMiss)
			break
		}
		g.Inventory = inv
		core.HealMember(&g.Party[member], core.ItemInfo(kind).HealAmount)
		audio.Play(audio.SoundHeal)
	case g.UsePendingSkill != core.SkillNone:
		caster := g.UsePendingCaster
		skill := g.UsePendingSkill
		if caster < 0 || caster >= len(g.Party) || g.Party[caster].HP <= 0 {
			// Caster out of range, or died between opening the picker and
			// confirming — a corpse can't pay MP or cast.
			break
		}
		cost := core.SkillCost(skill)
		if g.Party[caster].MP < cost {
			audio.Play(audio.SoundInputMiss) // MP drained between open and confirm
			break
		}
		core.HealMember(&g.Party[member], core.SkillHealFor(&g.Party[caster], skill))
		g.Party[caster].MP -= cost
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
