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

// validMember is the explore-side bounds guard: it resolves idx to a live
// pointer into g.Party, returning ok=false when idx is out of range. The
// `idx < 0 || idx >= len(g.Party)` check was open-coded at every panels /
// skill-tree caster lookup; centralizing it keeps the slice-access contract
// in one place. Callers layer their own HP / affordability checks on top —
// this only answers "is the index a real party seat" and hands back the
// pointer so the caller doesn't re-index.
func validMember(g *core.GameState, idx int) (*core.PartyMember, bool) {
	if idx < 0 || idx >= len(g.Party) {
		return nil, false
	}
	return &g.Party[idx], true
}

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
	def := core.ItemInfo(kind)
	if !core.ItemIsRestorative(def) {
		audio.Play(audio.SoundInputMiss) // equipment / no restorative effect
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
	m, ok := validMember(g, caster)
	if !ok {
		return
	}
	if m.HP <= 0 {
		// A member downed in a won battle keeps HP=0 with MP intact into
		// exploration; the Skills-tab cursor can still land on its column.
		// Don't let a corpse cast.
		audio.Play(audio.SoundInputMiss)
		return
	}
	// The Skills tab is a tree summary now (Confirm opens the tree modal), so
	// there's no per-skill cursor. Gather the member's out-of-battle heals and:
	// none → refuse; one → cast it directly; several (Cleric's Prayer + Mass
	// Mend) → pop a chooser so both stay reachable. Keep only the heals the
	// caster can currently afford so the chooser never lists a cast beginHealCast
	// would just refuse (and so an unaffordable two-heal Cleric falls through to
	// the "no usable heal" miss ping instead of a dead-end chooser).
	heals := affordableOutOfBattleHeals(*m)
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

// affordableOutOfBattleHeals returns the member's out-of-battle heals they
// can currently afford the MP for. The chooser (tryUseSkill) and its driver
// (updateHealPicker) both go through this ONE helper so the list the chooser
// opened with and the list the cursor walks can't diverge.
// Reusable buffers for the per-frame picker paths (heal chooser + ally
// target picker run this every frame they're open). Single-threaded
// update loop; the returned slices are valid until the next call.
var (
	outOfBattleHealsBuf []core.SkillID
	affordableHealsBuf  []core.SkillID
	useTargetLivingBuf  []int
)

func affordableOutOfBattleHeals(m core.PartyMember) []core.SkillID {
	outOfBattleHealsBuf = core.OutOfBattleHealsInto(outOfBattleHealsBuf, m)
	affordableHealsBuf = affordableHealsBuf[:0]
	for _, h := range outOfBattleHealsBuf {
		if core.CanAffordSkill(m, h) {
			affordableHealsBuf = append(affordableHealsBuf, h)
		}
	}
	return affordableHealsBuf
}

// beginHealCast resolves a chosen out-of-battle heal for `caster`: a
// single-target heal (Prayer) opens the ally-target picker; a party-wide heal
// (Mass Mend) applies to everyone immediately. Either way the caster pays the
// MP (the single-target path bills it on apply, in applyUseToMember). A short
// MP / corpse re-check guards the gap between choosing and casting.
func beginHealCast(g *core.GameState, caster int, skill core.SkillID) {
	m, ok := validMember(g, caster)
	if !ok || m.HP <= 0 {
		audio.Play(audio.SoundInputMiss)
		return
	}
	if !core.CanAffordSkill(*m, skill) {
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
	// Party-wide heal (Mass Mend): no target step. Refuse the cast if no
	// member can benefit (everyone alive is already at full HP) so it can't
	// silently drain the caster's MP for zero effect — the same intent the
	// item path enforces via core.ItemHelpsTarget. HealMember still no-ops on
	// the dead/ingested and clamps at MaxHP, so the fan-out stays safe.
	if !anyMemberBelowFull(g) {
		audio.Play(audio.SoundInputMiss)
		return
	}
	amount := core.SkillHealFor(m, skill)
	core.HealWholeParty(g, amount)
	core.SpendSkillMP(m, skill)
	audio.Play(audio.SoundHeal)
}

// anyMemberBelowFull reports whether any party member can still benefit from
// an HP heal — the precondition for a party-wide heal to do anything. Routes
// through the canonical core.MemberCanBeHealed predicate so this guard and
// the item path's core.ItemHelpsTarget can't drift on the HP rule.
func anyMemberBelowFull(g *core.GameState) bool {
	for i := range g.Party {
		if core.MemberCanBeHealed(g.Party[i]) {
			return true
		}
	}
	return false
}

// closeHealPick dismisses the out-of-battle heal chooser. Called on apply, on
// Back, and on overlay close / tab switch.
func closeHealPick(g *core.GameState) {
	g.HealPickOpen = false
	// noCaster, not 0: both readers (updateHealPicker, render.drawHealPicker)
	// treat a caster index < 0 as "no caster" and bail, matching closeUseTarget's
	// sentinel. Resetting to 0 left a valid party index sitting in a closed
	// picker.
	g.HealPickCaster = noCaster
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
	m, ok := validMember(g, caster)
	if !ok {
		closeHealPick(g)
		return
	}
	heals := affordableOutOfBattleHeals(*m)
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

// applyUseToMember resolves the pending use against the chosen member:
// an item consumes one stack and heals; a skill heals and bills the
// caster's MP. Either way the picker closes afterward.
func applyUseToMember(g *core.GameState, member int) {
	switch {
	case g.UsePendingItem != core.ItemNone:
		kind := g.UsePendingItem
		def := core.ItemInfo(kind)
		m := &g.Party[member]
		// Don't burn a restorative on a full ally — shared rule with the
		// battle-side applyItem guard (core.ItemHelpsTarget).
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
			// Caster out of range, or died between opening the picker and
			// confirming — a corpse can't pay MP or cast.
			break
		}
		// Don't burn MP healing an ally who can't benefit — the canonical
		// core.MemberCanBeHealed predicate (same HP rule the item path uses
		// via core.ItemHelpsTarget). Check before spending MP so a no-op cast
		// costs nothing. Bounds-checked first so the index can't panic.
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

// closeUseTarget dismisses the ally-target picker and clears its pending
// state. Called on apply, on Back, and on overlay close / tab switch.
func closeUseTarget(g *core.GameState) {
	g.UseTargetOpen = false
	g.UseTargetCursor = 0
	g.UsePendingItem = core.ItemNone
	g.UsePendingSkill = core.SkillNone
	g.UsePendingCaster = noCaster
}
