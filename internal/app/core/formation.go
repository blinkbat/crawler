package core

// Combat formation: front/back rows + attack-reach classification. MELEE is
// front-gated (attacker and target must both be in their effective front row);
// RANGED/MAGIC/AoE ignore rows. "Effective front": living front-row units, or the
// back row if the whole front has fallen (no melee-proof turtle). Shunting +
// reposition live elsewhere; this file is the pure classification + reach core.

// Row is a combatant's standing rank. Default RowFront — the pre-rows behaviour
// where everyone was reachable.
type Row uint8

const (
	RowFront Row = iota
	RowBack
)

// EnemyFrontRowCap / EnemyBackRowCap bound a foe pack's two ranks; EnemyPackCap is
// the resulting member ceiling. Authoring (editor) enforces these so a pack can't
// seat more than the formation grid renders.
const (
	EnemyFrontRowCap = 3
	EnemyBackRowCap  = 5
	EnemyPackCap     = EnemyFrontRowCap + EnemyBackRowCap
)

// PackRowCounts returns how many authored members sit in the front and back rows.
func PackRowCounts(members []PackMemberRef) (front, back int) {
	for _, m := range members {
		if m.Row == RowBack {
			back++
		} else {
			front++
		}
	}
	return front, back
}

// ShuntEnemyFormation repacks the enemy front row after a death: promote back-row
// enemies (in slice order) until the front holds min(EnemyFrontRowCap, living total).
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

// SwapFormationSlots exchanges two members' slot (HomeRow/HomeCol + live Row).
// A trade keeps a clean 2-front/2-back grid. The ONLY rearrange path; no-op on
// out-of-range or self.
func SwapFormationSlots(party []PartyMember, i, j int) {
	if i < 0 || j < 0 || i >= len(party) || j >= len(party) || i == j {
		return
	}
	party[i].HomeRow, party[j].HomeRow = party[j].HomeRow, party[i].HomeRow
	party[i].HomeCol, party[j].HomeCol = party[j].HomeCol, party[i].HomeCol
	// Resync the live reach row to the swapped home slot.
	party[i].Row = party[i].HomeRow
	party[j].Row = party[j].HomeRow
}

// formationSlotsValid reports whether the standing slots form a clean grid:
// every (HomeRow, HomeCol) in bounds and unique.
func formationSlotsValid(party []PartyMember) bool {
	var seen [2][2]bool
	for i := range party {
		r, c := party[i].HomeRow, party[i].HomeCol
		// Row/Col are unsigned — only the upper bound + uniqueness can fail.
		if r > RowBack || c > ColRight || seen[r][c] {
			return false
		}
		seen[r][c] = true
	}
	return true
}

// NormalizePartyFormation guarantees valid 2×2 slots: a clean grid is kept
// (custom swaps survive save/load); an invalid layout (e.g. a pre-formation save,
// all zero-value front-left) is re-seeded to the default (row by class, packed left-to-right).
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

// RowLabel is the human label for a formation row — the single UI-wording source.
func RowLabel(r Row) string {
	if r == RowBack {
		return "Back"
	}
	return "Front"
}

// ApplyMemberRows stamps Row from a FRONT-FIRST ordering: the last backCount
// entries become RowBack, the rest RowFront. Decode-side inverse of
// PartitionMembersByRow.
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

// PartitionMembersByRow returns the members reordered front-first plus the
// back-row count — the encode-side inverse of ApplyMemberRows.
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

// Col is a combatant's column in the 2×2 formation (left/right), stored alongside
// home Row so a side ambush can rotate the formation. Default ColLeft.
type Col uint8

const (
	ColLeft Col = iota
	ColRight
)

// EngageSide is which side the enemies strike from at battle start. A side/back
// ambush ROTATES the party so the attacked side becomes the front (see AmbushLiveRow).
type EngageSide uint8

const (
	EngageFront EngageSide = iota // player walked into them: home formation stands
	EngageRight
	EngageBack
	EngageLeft
)

// AmbushLiveRow returns a member's COMBAT row at battle start, rotating its home
// slot so the attacked side faces the enemy: Front unchanged; Back flips rows;
// Right/Left make the matching column the front (90° turn).
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

// DefaultPartyRow is a class's starting formation row: Warrior/Thief front,
// casters (Cleric, Wizard) back. Applies to a fresh party; the player can rearrange.
func DefaultPartyRow(c PartyClass) Row {
	switch c {
	case ClassCleric, ClassWizard:
		return RowBack
	default: // Warrior, Thief, and any future front-line class
		return RowFront
	}
}

// AttackClass classifies attack reach: only Melee is front-gated; Ranged/Magic
// reach any row. The 3-way split also feeds the future Flying specialty.
type AttackClass uint8

const (
	AttackMelee AttackClass = iota
	AttackRanged
	AttackMagic
)

// IsMelee reports whether this class is front-gated by the row rules.
func (c AttackClass) IsMelee() bool { return c == AttackMelee }

// BasicAttackClass classifies a weapon attack: ranged weapon = Ranged, else
// (incl. unarmed) Melee.
func BasicAttackClass(wt WeaponType) AttackClass {
	if WeaponIsRanged(wt) {
		return AttackRanged
	}
	return AttackMelee
}

// SkillAttackClass classifies a skill for reach from its SkillKind: melee
// front-gated; magic/heal = magic; utility = ranged (any-row — not a melee swing).
func SkillAttackClass(k SkillKind) AttackClass {
	switch k {
	case SkillKindMelee:
		return AttackMelee
	case SkillKindMagic, SkillKindHeal:
		return AttackMagic
	default: // SkillKindUtility (and future kinds) — any row
		return AttackRanged
	}
}

// EnemyBasicAttackClass classifies an enemy's basic attack — melee today; the
// seam lets a future ranged/caster enemy return Ranged/Magic.
func EnemyBasicAttackClass(_ EnemyKind) AttackClass {
	return AttackMelee
}

// PartyFrontHasLiving reports whether any LIVING member stands in the front row.
// When false, the back row is the effective front.
func PartyFrontHasLiving(party []PartyMember) bool {
	for i := range party {
		if party[i].HP > 0 && party[i].Row == RowFront {
			return true
		}
	}
	return false
}

// PartyInEffectiveFront reports whether member i sits in the effective front
// (own row front, or the front row fell). The single predicate behind melee
// REACH and OFFENSE.
func PartyInEffectiveFront(party []PartyMember, i int) bool {
	if i < 0 || i >= len(party) {
		return false
	}
	return party[i].Row == RowFront || !PartyFrontHasLiving(party)
}

// BackRowMeleeBlocked reports whether an attack of class ac launched from slot i
// can't connect — a melee swing while i sits in the still-protected back row. The
// single source for the back-row melee gate (combat gating + greyed-out UI rows).
func BackRowMeleeBlocked(ac AttackClass, party []PartyMember, i int) bool {
	return ac.IsMelee() && !PartyInEffectiveFront(party, i)
}

// EnemyFrontHasLiving reports whether any LIVING enemy stands in the front row.
func EnemyFrontHasLiving(members []Enemy) bool {
	for i := range members {
		if members[i].Alive && members[i].Row == RowFront {
			return true
		}
	}
	return false
}

// EnemyInEffectiveFront is the enemy-side mirror of PartyInEffectiveFront.
func EnemyInEffectiveFront(members []Enemy, i int) bool {
	if i < 0 || i >= len(members) {
		return false
	}
	return members[i].Row == RowFront || !EnemyFrontHasLiving(members)
}
