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
	RowCount // sentinel: number of formation ranks (battle swap navigation assumes 2)
)

// EnemyFrontRowCap / EnemyBackRowCap bound a foe pack's two ranks; EnemyPackCap is
// the resulting member ceiling. Authoring (editor) enforces these so a pack can't
// seat more than the formation grid renders. Back is wider than front, so the
// rightmost back slot (index 2) has no front column ahead of it — it's always
// exposed to melee (see EnemyColumnCovered).
const (
	EnemyFrontRowCap = 2
	EnemyBackRowCap  = 3
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

// ShuntEnemyFormation fills the enemy front row left-to-right: promote back-row
// enemies (in slice order = left-to-right slot order) until the front holds
// min(EnemyFrontRowCap, living total). Run at battle start AND after each death so
// foes always pack into the front before spilling to the back — an authored
// back-heavy pack (or a front wiped by combat) self-corrects. With a full 5-foe
// pack the front caps at 2 and the 3rd living back foe stays back (exposed).
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

// flipBinary returns the OTHER of two values (v==a → b, else a) — the "toggle one
// of two" primitive behind the 2×2 row/col flips. init asserts RowCount==ColCount==2.
func flipBinary[T comparable](v, a, b T) T {
	if v == a {
		return b
	}
	return a
}

func init() {
	if RowCount != 2 || ColCount != 2 {
		panic("core: formation flips assume RowCount==ColCount==2 — generalize FlipRow/FlipCol/Ambush* before extending the grid")
	}
	if PartyMemberCount != int(RowCount)*int(ColCount) {
		panic("core: PartyMemberCount must equal RowCount*ColCount — formationSlotsValid's [2][2] grid assumes one member per slot")
	}
}

// FlipRow / FlipCol return the other rank / column of the 2×2 — the single source
// for "the orthogonal neighbour" so the battle swap picker and the out-of-battle
// panel formation nav can't disagree. (Assumes RowCount==ColCount==2.)
func FlipRow(r Row) Row {
	return flipBinary(r, RowFront, RowBack)
}

func FlipCol(c Col) Col {
	return flipBinary(c, ColLeft, ColRight)
}

// SwapPlacesMessage is the shared "<A> and <B> swap places." log line so battle
// Swap and the out-of-battle panel Swap stay worded identically.
func SwapPlacesMessage(a, b string) string {
	return a + " and " + b + " swap places."
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

// defaultSlotCol returns the home column for the next member placed in `row`,
// packing each row into a 2×2 (first member left, second right). frontN/backN
// track placements so far in each row and are advanced in place. Shared by
// NewParty and NormalizePartyFormation so the default-seed rule lives in one spot.
func defaultSlotCol(row Row, frontN, backN *int) Col {
	n := backN
	if row == RowFront {
		n = frontN
	}
	col := Col(*n % int(ColCount))
	*n++
	return col
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
		col := defaultSlotCol(row, &frontCount, &backCount)
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
	ColCount // sentinel: number of formation columns (battle swap navigation assumes 2)
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
		return flipBinary(homeRow, RowFront, RowBack)
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

// AmbushLiveCol is the column companion to AmbushLiveRow: it rotates a member's
// home column into its live column for the same 2×2 turn, so a side/back ambush
// yields a valid, unique live grid. Column is cosmetic for party reach (only Row
// gates melee) but drives in-battle sprite placement.
func AmbushLiveCol(homeRow Row, homeCol Col, side EngageSide) Col {
	switch side {
	case EngageBack: // 180°: columns mirror
		return flipBinary(homeCol, ColLeft, ColRight)
	case EngageRight: // 90°: the old row decides the new column
		if homeRow == RowFront {
			return ColLeft
		}
		return ColRight
	case EngageLeft:
		if homeRow == RowFront {
			return ColRight
		}
		return ColLeft
	default: // EngageFront — home column stands
		return homeCol
	}
}

// SetBattleStartFormation seats every member's LIVE (Row,Col) from its home slot
// rotated by the engage side, then packs the living forward (ShuntPartyFormation)
// so a frontliner already downed before the fight doesn't hold the line. Home slots
// are untouched — recomputed fresh each battle, and the party reverts to Home when
// the fight ends.
func SetBattleStartFormation(party []PartyMember, side EngageSide) {
	for i := range party {
		party[i].Row = AmbushLiveRow(party[i].HomeRow, party[i].HomeCol, side)
		party[i].Col = AmbushLiveCol(party[i].HomeRow, party[i].HomeCol, side)
		party[i].SwapSlide = 0 // no carried-over slide into a fresh formation
	}
	ShuntPartyFormation(party)
}

// StampSwapSlide arms the formation-Swap glide on a member: it eases from (fromRow,
// fromCol) — its pre-swap slot — to its current live slot over SwapSlideDuration.
// Call AFTER the live slots are exchanged, with each member's OLD slot.
func StampSwapSlide(m *PartyMember, fromRow Row, fromCol Col) {
	m.SwapFromRow = fromRow
	m.SwapFromCol = fromCol
	m.SwapSlide = SwapSlideDuration
}

// ShuntPartyFormation pulls living members into the front row and sinks the downed
// to the back, swapping LIVE slots only (Home untouched, so the formation reverts
// after the fight). A downed front member trades with a living back member — same
// column first, else the first living back member. Idempotent and revive-safe: call
// it again after a Raise and the revived member is pulled forward if a front slot
// still needs manning.
func ShuntPartyFormation(party []PartyMember) {
	for col := ColLeft; col <= ColRight; col++ {
		front := liveSlot(party, RowFront, col)
		if front < 0 || party[front].HP > 0 {
			continue // empty slot or a living frontliner — leave it
		}
		partner := liveSlot(party, RowBack, col) // prefer the same column
		if partner < 0 || party[partner].HP <= 0 {
			partner = firstLivingInRow(party, RowBack) // else any living backliner
		}
		if partner >= 0 && party[partner].HP > 0 {
			SwapLiveSlots(party, front, partner)
		}
	}
}

// liveSlot returns the index of the member whose LIVE slot is (row,col), or -1.
func liveSlot(party []PartyMember, row Row, col Col) int {
	for i := range party {
		if party[i].Row == row && party[i].Col == col {
			return i
		}
	}
	return -1
}

// firstLivingInRow returns the first living member whose live Row is row, or -1.
func firstLivingInRow(party []PartyMember, row Row) int {
	for i := range party {
		if party[i].Row == row && party[i].HP > 0 {
			return i
		}
	}
	return -1
}

// SwapLiveSlots exchanges two members' LIVE (Row,Col); the home slot is left alone,
// so the rearrangement lasts only for the current fight. This is the in-battle
// tactical Swap (reverts next fight); SwapFormationSlots edits the persistent Home
// (preferred) formation instead.
func SwapLiveSlots(party []PartyMember, i, j int) {
	if i < 0 || j < 0 || i >= len(party) || j >= len(party) || i == j {
		return
	}
	party[i].Row, party[j].Row = party[j].Row, party[i].Row
	party[i].Col, party[j].Col = party[j].Col, party[i].Col
}

// defaultPartyRows is each class's starting formation row. Init-asserted complete
// (every PartyClass mapped) so a new class must declare its row, not inherit one.
var defaultPartyRows = map[PartyClass]Row{
	ClassWarrior: RowFront,
	ClassThief:   RowFront,
	ClassCleric:  RowBack,
	ClassWizard:  RowBack,
}

// DefaultPartyRow is a class's starting formation row: Warrior/Thief front,
// casters (Cleric, Wizard) back. Applies to a fresh party; the player can rearrange.
func DefaultPartyRow(c PartyClass) Row {
	return defaultPartyRows[c] // RowFront zero-value is unreachable: init asserts full coverage
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

// skillKindCount bounds the skillAttackClasses coverage assert (SkillKind starts at 1).
const skillKindCount = SkillKindUtility + 1

// skillAttackClasses maps a SkillKind to its reach. Init-asserted complete so a new
// kind must pick a reach instead of silently falling through to any-row Ranged.
var skillAttackClasses = map[SkillKind]AttackClass{
	SkillKindMelee:   AttackMelee,
	SkillKindMagic:   AttackMagic,
	SkillKindHeal:    AttackMagic,
	SkillKindUtility: AttackRanged, // any-row — not a melee swing
}

// SkillAttackClass classifies a skill for reach from its SkillKind: melee
// front-gated; magic/heal = magic; utility = ranged (any-row — not a melee swing).
func SkillAttackClass(k SkillKind) AttackClass {
	return skillAttackClasses[k] // AttackMelee zero-value unreachable: init asserts full coverage
}

func init() {
	for c := PartyClass(0); c < PartyClassCount; c++ {
		if _, ok := defaultPartyRows[c]; !ok {
			panic("core: defaultPartyRows missing a row for a PartyClass — add it")
		}
	}
	for k := SkillKindMelee; k < skillKindCount; k++ {
		if _, ok := skillAttackClasses[k]; !ok {
			panic("core: skillAttackClasses missing a reach for a SkillKind — add it")
		}
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

// EnemyColumnCovered reports whether back-row enemy i is shielded by a front-row foe
// in the SAME column. Foes pack left-to-right, so a back foe is covered iff its
// left-to-right slot among the back row is less than the front count (a front foe
// stands in that column). Front foes are never "covered" — they are the cover. The
// rightmost back slot of a full pack (slot 2, front caps at 2) is uncovered/meleeable.
//
// Occupancy counts foes that still hold a visible formation slot — alive OR mid
// death-fade — matching render's enemyRowPlacements, so the melee-cover columns agree
// with the on-screen ranks (a just-killed front foe keeps shielding the back until its
// corpse finishes fading, rather than the back becoming reachable a frame before it clears).
func EnemyColumnCovered(members []Enemy, i int) bool {
	if i < 0 || i >= len(members) || !members[i].Alive || members[i].Row != RowBack {
		return false
	}
	frontCount, backSlot := 0, 0
	for j := range members {
		if !enemyHoldsFormationSlot(members[j]) {
			continue
		}
		if members[j].Row == RowFront {
			frontCount++
		} else if j < i {
			backSlot++ // a foe still holding a slot to our left
		}
	}
	return backSlot < frontCount
}

// enemyHoldsFormationSlot reports whether an enemy still occupies a visible
// formation slot — alive, or mid death-fade (its corpse is still drawn). The single
// predicate shared by the melee-cover test and render's slot layout so they agree.
func enemyHoldsFormationSlot(e Enemy) bool {
	return e.Alive || e.DeathFade > 0
}

// EnemyInEffectiveFront reports whether enemy i is reachable by melee: front-row
// foes always, back-row foes only when their column has no front cover (front wiped
// in that column, or they're the uncovered overhang slot). The enemy-side mirror of
// PartyInEffectiveFront, now column-granular rather than all-or-nothing.
func EnemyInEffectiveFront(members []Enemy, i int) bool {
	if i < 0 || i >= len(members) {
		return false
	}
	return members[i].Row == RowFront || !EnemyColumnCovered(members, i)
}

// EnemyMeleeReachable reports whether a MELEE attack/skill can connect with enemy i:
// in the effective front row AND not Flying. Flyers are immune to melee — only a
// ranged weapon or magic touches them (a ranged attacker is Ranged-class, so it never
// consults this). The party→enemy melee-reach predicate. EnemyInEffectiveFront stays
// the pure row test, still used for the enemy→party gate — a flying foe can melee.
func EnemyMeleeReachable(members []Enemy, i int) bool {
	return EnemyInEffectiveFront(members, i) && !EnemyInfoFor(members[i]).Flying
}
