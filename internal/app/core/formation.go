package core

// Combat formation: front/back rows + attack-reach classification.
//
// Every combatant (party member, enemy) stands in a ROW — front or back. The
// rule that makes rows matter: a MELEE attack is "front-gated" — the attacker
// must be in the (effective) front row, and it can only strike a target in the
// target side's (effective) front row. RANGED and MAGIC attacks ignore rows
// entirely (any → any). AoE ignores rows too (it hits everything).
//
// "Effective front" handles the falls/turtle cases: if a side has any LIVING
// front-row unit, that's its front; if the whole front row has fallen, the back
// row becomes the front (so melee can reach it, and back-row melee units can
// finally swing — no melee-proof turtle by hiding everyone in back).
//
// Per-unit shunting (a back unit sliding up to fill a dead front slot) and the
// per-character reposition action live elsewhere; this file is the pure,
// testable core: how a unit is classified and who melee can reach.

// Row is a combatant's standing rank. Default RowFront (zero value) so a
// combatant with no explicit formation reads as front — i.e. the pre-rows
// behaviour where everyone was reachable.
type Row uint8

const (
	RowFront Row = iota
	RowBack
)

// EnemyFrontRowCap is how many enemies stand in the front row of a pack
// formation (the rest are back). The shunt keeps the front packed to this many
// while enough enemies are alive.
const EnemyFrontRowCap = 3

// ShuntEnemyFormation repacks the enemy front row after a death: while the front
// row has fewer living members than it should (min of EnemyFrontRowCap and the
// living total), promote living back-row enemies forward to fill it — so the
// front stays packed and melee always has a front target while ≥cap enemies
// live. Promotion currently walks slice order (left→right ≈ the visual
// left-to-right); the precise "nearest the emptied slot" choice is a cosmetic
// refinement that lands with the staggered render. No-op when the front is
// already full or there's no back-row enemy to pull up.
func ShuntEnemyFormation(members []Enemy) {
	livingFront, livingTotal := 0, 0
	for i := range members {
		if !members[i].Alive {
			continue
		}
		livingTotal++
		if members[i].Row == RowFront {
			livingFront++
		}
	}
	want := EnemyFrontRowCap
	if livingTotal < want {
		want = livingTotal
	}
	for i := range members {
		if livingFront >= want {
			return
		}
		if members[i].Alive && members[i].Row == RowBack {
			members[i].Row = RowFront
			livingFront++
		}
	}
}

// SwapFormationSlots exchanges two party members' standing 2×2 slot — their
// HomeRow/HomeCol AND the live combat Row that mirrors it. Because the two
// members trade positions (rather than one moving independently), the formation
// always stays a clean 2-front/2-back grid: a swap can never leave three units
// in one row. This is the ONLY formation-rearrange path (the in-battle Swap
// action and the out-of-combat Character-tab swap both route through it). No-op
// on an out-of-range or self pairing.
func SwapFormationSlots(party []PartyMember, i, j int) {
	if i < 0 || j < 0 || i >= len(party) || j >= len(party) || i == j {
		return
	}
	party[i].HomeRow, party[j].HomeRow = party[j].HomeRow, party[i].HomeRow
	party[i].HomeCol, party[j].HomeCol = party[j].HomeCol, party[i].HomeCol
	// Resync the live reach row to the (now swapped) home slot for both.
	party[i].Row = party[i].HomeRow
	party[j].Row = party[j].HomeRow
}

// formationSlotsValid reports whether the party's standing slots form a clean
// grid: every (HomeRow, HomeCol) in bounds and unique. A valid layout is left
// untouched by NormalizePartyFormation so the player's custom swaps persist.
func formationSlotsValid(party []PartyMember) bool {
	var seen [2][2]bool
	for i := range party {
		r, c := party[i].HomeRow, party[i].HomeCol
		// Row/Col are unsigned, so the lower bound (>= RowFront/ColLeft) holds by
		// type — only the upper bound and the uniqueness check can fail.
		if r > RowBack || c > ColRight || seen[r][c] {
			return false
		}
		seen[r][c] = true
	}
	return true
}

// NormalizePartyFormation guarantees the party's standing 2×2 slots are valid.
// A party whose HomeRow/HomeCol already form a clean grid keeps them (custom
// swaps survive a save/load round-trip). An INVALID layout — most importantly a
// pre-formation save, where every slot decoded to the zero value (front-left) —
// is re-seeded to the default formation: each member's default row by class,
// columns packed left-to-right in array order (mirrors NewParty), with the live
// Row resynced to the home row. Without this, a stale save would stack every
// party card in one slot now that the ribbon and 3D battlefield key off the
// formation slot.
func NormalizePartyFormation(party []PartyMember) {
	if formationSlotsValid(party) {
		return
	}
	frontCount, backCount := 0, 0
	for i := range party {
		row := DefaultPartyRow(party[i].Class)
		col := ColLeft
		if row == RowFront {
			if frontCount%2 == 1 {
				col = ColRight
			}
			frontCount++
		} else {
			if backCount%2 == 1 {
				col = ColRight
			}
			backCount++
		}
		party[i].HomeRow = row
		party[i].HomeCol = col
		party[i].Row = row
	}
}

// RowLabel is the human label for a formation row — the single source for UI
// captions/tags (the editor's Row button, the pack-member list tag, any future
// readout) so wording can't drift across call sites.
func RowLabel(r Row) string {
	if r == RowBack {
		return "Back"
	}
	return "Front"
}

// ApplyMemberRows stamps Row onto pack members from a FRONT-FIRST ordering: the
// last backCount entries become RowBack, the rest RowFront. The decode-side
// inverse of PartitionMembersByRow — both express the "members are ordered
// front-first, the trailing backCount are back row" invariant in ONE place so
// the (de)serialization conversions can't drift.
func ApplyMemberRows(members []PackMemberRef, backCount int) {
	backStart := len(members) - backCount
	for i := range members {
		if i >= backStart {
			members[i].Row = RowBack
		} else {
			members[i].Row = RowFront
		}
	}
}

// PartitionMembersByRow returns the members reordered front-first (front row in
// original order, then back row in original order) and the back-row count — the
// encode-side inverse of ApplyMemberRows.
func PartitionMembersByRow(members []PackMemberRef) (ordered []PackMemberRef, backCount int) {
	ordered = make([]PackMemberRef, 0, len(members))
	for _, m := range members {
		if m.Row != RowBack {
			ordered = append(ordered, m)
		}
	}
	frontCount := len(ordered)
	for _, m := range members {
		if m.Row == RowBack {
			ordered = append(ordered, m)
		}
	}
	return ordered, len(ordered) - frontCount
}

// Col is a combatant's column in the 2×2 party formation (left/right). Stored
// alongside the home Row so a side ambush can rotate the formation. Default
// ColLeft (zero value).
type Col uint8

const (
	ColLeft Col = iota
	ColRight
)

// EngageSide is which side of the party the enemies strike from when a battle
// begins. A side/back ambush ROTATES the party so the attacked side becomes the
// front — the rank now exposed to melee — which the player then fixes by
// repositioning (see AmbushLiveRow).
type EngageSide uint8

const (
	EngageFront EngageSide = iota // player walked into them: home formation stands
	EngageRight
	EngageBack
	EngageLeft
)

// AmbushLiveRow returns a member's COMBAT row at battle start, rotating its home
// 2×2 slot (homeRow, homeCol) so the attacked side faces the enemy:
//   - Front: home row unchanged (you met them head-on).
//   - Back: rows flip — your back rank is now the exposed front.
//   - Right/Left: the matching column becomes the front row (90° turn).
func AmbushLiveRow(homeRow Row, homeCol Col, side EngageSide) Row {
	switch side {
	case EngageBack:
		if homeRow == RowFront {
			return RowBack
		}
		return RowFront
	case EngageRight:
		if homeCol == ColRight {
			return RowFront
		}
		return RowBack
	case EngageLeft:
		if homeCol == ColLeft {
			return RowFront
		}
		return RowBack
	default: // EngageFront
		return homeRow
	}
}

// DefaultPartyRow is a class's starting formation row: melee/skirmisher classes
// (Warrior, Thief) hold the front; casters (Cleric, Wizard) start in the back
// where melee can't reach them. The player can rearrange afterward (the row is a
// standing, persisted choice). The default applies to a fresh party; old saves
// load whatever they stored (zero value RowFront).
func DefaultPartyRow(c PartyClass) Row {
	switch c {
	case ClassCleric, ClassWizard:
		return RowBack
	default: // Warrior, Thief, and any future front-line class
		return RowFront
	}
}

// AttackClass classifies how an attack reaches its target, for the row system.
// Melee is the only front-gated class; Ranged and Magic reach any row. The
// 3-way split (rather than a bare melee bool) also feeds the Flying specialty
// later — a flyer dodges melee and certain magic elements, but not ranged.
type AttackClass uint8

const (
	AttackMelee AttackClass = iota
	AttackRanged
	AttackMagic
)

// IsMelee reports whether this class is front-gated by the row rules.
func (c AttackClass) IsMelee() bool { return c == AttackMelee }

// BasicAttackClass classifies a basic (weapon) attack: a ranged weapon strikes
// at range, everything else — including unarmed — is melee.
func BasicAttackClass(wt WeaponType) AttackClass {
	if WeaponIsRanged(wt) {
		return AttackRanged
	}
	return AttackMelee
}

// SkillAttackClass classifies a skill for reach, derived from its existing
// SkillKind so a new skill is classified by its kind alone: melee skills are
// front-gated; magic and heal are magic; utility reaches any row (treated as
// ranged for reach — a Steal/Scan/Cripple isn't a melee swing). The deferred
// Flying/element work can refine utility per-skill if needed.
func SkillAttackClass(k SkillKind) AttackClass {
	switch k {
	case SkillKindMelee:
		return AttackMelee
	case SkillKindMagic, SkillKindHeal:
		return AttackMagic
	default: // SkillKindUtility (and any future kind) — reaches any row
		return AttackRanged
	}
}

// EnemyBasicAttackClass classifies an enemy's basic attack. Enemy basics are
// melee (claw/bite/slam) today; the seam exists so a future ranged/caster enemy
// kind can return Ranged/Magic and bypass the front gate.
func EnemyBasicAttackClass(_ EnemyKind) AttackClass {
	return AttackMelee
}

// --- Effective-front reach (party) -----------------------------------------

// PartyFrontHasLiving reports whether any LIVING party member stands in the
// front row. When false the front row has fallen and the back row is the
// effective front.
func PartyFrontHasLiving(party []PartyMember) bool {
	for i := range party {
		if party[i].HP > 0 && party[i].Row == RowFront {
			return true
		}
	}
	return false
}

// PartyInEffectiveFront reports whether party member i sits in the side's
// effective front — its own row is front, or the front row has fallen (so the
// back row is now the front). This is the single predicate both melee REACH
// (can an enemy melee this member?) and melee OFFENSE (can this member, if a
// melee attacker, swing at all?) key off, so the two can't drift.
func PartyInEffectiveFront(party []PartyMember, i int) bool {
	if i < 0 || i >= len(party) {
		return false
	}
	return party[i].Row == RowFront || !PartyFrontHasLiving(party)
}

// --- Effective-front reach (enemies) ---------------------------------------

// EnemyFrontHasLiving reports whether any LIVING enemy stands in the front row.
func EnemyFrontHasLiving(members []Enemy) bool {
	for i := range members {
		if members[i].Alive && members[i].Row == RowFront {
			return true
		}
	}
	return false
}

// EnemyInEffectiveFront reports whether enemy i sits in the side's effective
// front (own row front, or the front row has fallen). Mirror of
// PartyInEffectiveFront for the enemy side.
func EnemyInEffectiveFront(members []Enemy, i int) bool {
	if i < 0 || i >= len(members) {
		return false
	}
	return members[i].Row == RowFront || !EnemyFrontHasLiving(members)
}
