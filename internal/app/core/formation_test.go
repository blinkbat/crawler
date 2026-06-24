package core

import (
	"testing"

	"crawler/internal/app/core/mapfile"
)

func TestAttackClassification(t *testing.T) {
	// Basic attacks: ranged weapon → Ranged, else (incl. unarmed) → Melee.
	if got := BasicAttackClass(WeaponSword); got != AttackMelee {
		t.Errorf("sword basic = %d, want Melee", got)
	}
	if got := BasicAttackClass(WeaponNone); got != AttackMelee {
		t.Errorf("unarmed basic = %d, want Melee", got)
	}
	if got := BasicAttackClass(WeaponBow); got != AttackRanged {
		t.Errorf("bow basic = %d, want Ranged", got)
	}
	// Skills classify by kind: melee→melee, magic/heal→magic, utility→ranged.
	cases := []struct {
		k    SkillKind
		want AttackClass
	}{
		{SkillKindMelee, AttackMelee},
		{SkillKindMagic, AttackMagic},
		{SkillKindHeal, AttackMagic},
		{SkillKindUtility, AttackRanged},
	}
	for _, c := range cases {
		if got := SkillAttackClass(c.k); got != c.want {
			t.Errorf("SkillAttackClass(%d) = %d, want %d", c.k, got, c.want)
		}
	}
	if !AttackMelee.IsMelee() || AttackRanged.IsMelee() || AttackMagic.IsMelee() {
		t.Error("IsMelee should be true only for AttackMelee")
	}
}

func TestNewPartyDefaultRows(t *testing.T) {
	for _, m := range NewParty() {
		want := RowFront
		if m.Class == ClassCleric || m.Class == ClassWizard {
			want = RowBack
		}
		if m.Row != want {
			t.Errorf("%s default row = %d, want %d", m.Name, m.Row, want)
		}
	}
}

func TestPartyEffectiveFront(t *testing.T) {
	party := []PartyMember{
		{HP: 10, Row: RowFront},
		{HP: 10, Row: RowFront},
		{HP: 10, Row: RowBack},
		{HP: 10, Row: RowBack},
	}
	if !PartyFrontHasLiving(party) {
		t.Fatal("front row should have living members")
	}
	if !PartyInEffectiveFront(party, 0) || !PartyInEffectiveFront(party, 1) {
		t.Error("front members should be in the effective front")
	}
	if PartyInEffectiveFront(party, 2) || PartyInEffectiveFront(party, 3) {
		t.Error("back members should NOT be reachable while the front row stands")
	}
	// Front row falls → back row becomes the effective front (anti-turtle).
	party[0].HP, party[1].HP = 0, 0
	if PartyFrontHasLiving(party) {
		t.Fatal("front row should have fallen")
	}
	if !PartyInEffectiveFront(party, 2) || !PartyInEffectiveFront(party, 3) {
		t.Error("with the front row down, the back row should be the effective front")
	}
}

func TestPackMemberRowRoundTrip(t *testing.T) {
	// A back-row member decodes with that row and re-encodes with BackCount intact.
	mf := mapfile.MapFile{
		Name: "Row Test", Materials: "dungeon", Width: 4, Height: 4,
		StartX: 0, StartZ: 0, StartFace: "east",
		Walls: []string{"....", "....", "....", "...."},
		Floor: []string{"....", "....", "....", "...."},
		Decor: []string{"....", "....", "....", "...."},
		Props: []string{"....", "....", "....", "...."},
		Packs: []mapfile.MapPack{
			{Members: []string{"bat", "bat", "bat"}, BackCount: 1, X: 2, Z: 1},
		},
	}
	area, err := AreaFromMapFile(mf, "")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := area.PackSpawns[0].Members
	if len(got) != 3 {
		t.Fatalf("want 3 members, got %d", len(got))
	}
	nBack := 0
	for _, m := range got {
		if m.Row == RowBack {
			nBack++
		}
	}
	if nBack != 1 {
		t.Errorf("decoded back-row count = %d, want 1", nBack)
	}
	if got[len(got)-1].Row != RowBack {
		t.Error("the back member should be ordered last")
	}
	enc, err := MapFileFromArea(area)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if enc.Packs[0].BackCount != 1 {
		t.Errorf("re-encoded BackCount = %d, want 1", enc.Packs[0].BackCount)
	}
}

func TestAmbushLiveRow(t *testing.T) {
	// Front engage: home rows stand.
	if AmbushLiveRow(RowFront, ColLeft, EngageFront) != RowFront ||
		AmbushLiveRow(RowBack, ColLeft, EngageFront) != RowBack {
		t.Error("front engage should leave the home row unchanged")
	}
	// Back ambush: rows flip (back becomes the exposed front).
	if AmbushLiveRow(RowFront, ColLeft, EngageBack) != RowBack ||
		AmbushLiveRow(RowBack, ColLeft, EngageBack) != RowFront {
		t.Error("back ambush should flip the rows")
	}
	if AmbushLiveRow(RowBack, ColRight, EngageRight) != RowFront ||
		AmbushLiveRow(RowFront, ColLeft, EngageRight) != RowBack {
		t.Error("right ambush should bring the right column to the front")
	}
	if AmbushLiveRow(RowBack, ColLeft, EngageLeft) != RowFront ||
		AmbushLiveRow(RowFront, ColRight, EngageLeft) != RowBack {
		t.Error("left ambush should bring the left column to the front")
	}
}

func TestNewPartyFormationIs2x2(t *testing.T) {
	seen := map[[2]int]int{}
	for _, m := range NewParty() {
		seen[[2]int{int(m.HomeRow), int(m.HomeCol)}]++
	}
	for row := 0; row < 2; row++ {
		for col := 0; col < 2; col++ {
			if seen[[2]int{row, col}] != 1 {
				t.Errorf("slot (row %d, col %d) occupied %d times, want 1", row, col, seen[[2]int{row, col}])
			}
		}
	}
}

func TestSwapFormationSlots(t *testing.T) {
	party := NewParty() // a clean 2×2 to start
	front, back := -1, -1
	for i := range party {
		if party[i].HomeRow == RowFront && front < 0 {
			front = i
		}
		if party[i].HomeRow == RowBack && back < 0 {
			back = i
		}
	}
	if front < 0 || back < 0 {
		t.Fatal("default party should have both a front and a back member")
	}
	fRow, fCol := party[front].HomeRow, party[front].HomeCol
	bRow, bCol := party[back].HomeRow, party[back].HomeCol
	SwapFormationSlots(party, front, back)
	if party[front].HomeRow != bRow || party[front].HomeCol != bCol {
		t.Error("front member should take the back member's slot")
	}
	if party[back].HomeRow != fRow || party[back].HomeCol != fCol {
		t.Error("back member should take the front member's slot")
	}
	if party[front].Row != party[front].HomeRow || party[back].Row != party[back].HomeRow {
		t.Error("live Row should resync to HomeRow after a swap")
	}
	if !formationSlotsValid(party) {
		t.Error("formation should remain a valid 2×2 after a swap")
	}
	frontCount := 0
	for i := range party {
		if party[i].HomeRow == RowFront {
			frontCount++
		}
	}
	if frontCount != 2 {
		t.Errorf("front count = %d after swap, want 2 (a swap can never put three up front)", frontCount)
	}
	// No-op guards: self + out-of-range must not panic or mutate.
	SwapFormationSlots(party, 1, 1)
	SwapFormationSlots(party, -1, 2)
	SwapFormationSlots(party, 0, 99)
	if !formationSlotsValid(party) {
		t.Error("no-op swaps should leave the formation valid")
	}
}

func TestNormalizePartyFormation(t *testing.T) {
	// A pre-formation save: every slot decoded to the zero value (front-left).
	party := NewParty()
	for i := range party {
		party[i].HomeRow, party[i].HomeCol = RowFront, ColLeft
	}
	if formationSlotsValid(party) {
		t.Fatal("an all-front-left layout should read as invalid")
	}
	NormalizePartyFormation(party)
	if !formationSlotsValid(party) {
		t.Error("normalize should repair an invalid layout into a clean 2×2")
	}
	for i := range party {
		want := DefaultPartyRow(party[i].Class)
		if party[i].HomeRow != want || party[i].Row != want {
			t.Errorf("member %d row = %d/%d, want %d (default by class)", i, party[i].HomeRow, party[i].Row, want)
		}
	}
	// A valid custom layout (a swap) must be preserved untouched.
	custom := NewParty()
	SwapFormationSlots(custom, 0, len(custom)-1)
	snapshot := make([][2]Row, len(custom))
	for i := range custom {
		snapshot[i] = [2]Row{custom[i].HomeRow, Row(custom[i].HomeCol)}
	}
	NormalizePartyFormation(custom)
	for i := range custom {
		if custom[i].HomeRow != snapshot[i][0] || Row(custom[i].HomeCol) != snapshot[i][1] {
			t.Errorf("normalize must not disturb a valid custom layout at member %d", i)
		}
	}
}

func TestShuntEnemyFormation(t *testing.T) {
	// 2 front + 3 back; killing a front enemy promotes a back one (front packed at 2 while ≥2 alive).
	members := []Enemy{
		{Alive: true, Row: RowFront},
		{Alive: true, Row: RowFront},
		{Alive: true, Row: RowBack},
		{Alive: true, Row: RowBack},
		{Alive: true, Row: RowBack},
	}
	members[1].Alive = false // a front enemy dies
	ShuntEnemyFormation(members)
	front := 0
	for i := range members {
		if members[i].Alive && members[i].Row == RowFront {
			front++
		}
	}
	if front != 2 {
		t.Errorf("after shunt, living front = %d, want 2 (a back enemy should fill the gap)", front)
	}
	// Drop below the cap: only 2 alive total → front can't exceed 2.
	members[0].Alive, members[2].Alive, members[3].Alive = false, false, false
	ShuntEnemyFormation(members)
	front = 0
	for i := range members {
		if members[i].Alive && members[i].Row == RowFront {
			front++
		}
	}
	if front != 1 { // one living enemy left → it's the front
		t.Errorf("with 1 enemy alive, living front = %d, want 1", front)
	}
}

func TestPeekNextMeleeEnemyTarget(t *testing.T) {
	g := &GameState{Party: []PartyMember{
		{HP: 10, Row: RowFront},
		{HP: 10, Row: RowBack},
		{HP: 10, Row: RowFront},
	}}
	g.Battle.EnemyAttackCursor = -1
	if got := PeekNextMeleeEnemyTarget(g); got != 0 {
		t.Errorf("melee target = %d, want 0 (front)", got)
	}
	g.Battle.EnemyAttackCursor = 0
	if got := PeekNextMeleeEnemyTarget(g); got != 2 {
		t.Errorf("melee target after slot 0 = %d, want 2 (skip back-row slot 1)", got)
	}
	// Front row falls → back row becomes reachable.
	g.Party[0].HP, g.Party[2].HP = 0, 0
	g.Battle.EnemyAttackCursor = -1
	if got := PeekNextMeleeEnemyTarget(g); got != 1 {
		t.Errorf("with front down, melee target = %d, want 1 (back now front)", got)
	}
}

// TestPeekEnemyAttackerTarget_FrontGateSkillReachTaunt locks the shared forecast/
// commit peek: a basic swing is front-gated, a pending skill reaches any row, and a
// live Taunt overrides the gate. Regression for the incoming-hit marker pointing at
// a covered back-row member a melee foe can't actually reach.
func TestPeekEnemyAttackerTarget_FrontGateSkillReachTaunt(t *testing.T) {
	newG := func() *GameState {
		g := &GameState{Party: []PartyMember{
			{HP: 10, Row: RowFront},
			{HP: 10, Row: RowBack},
			{HP: 10, Row: RowFront},
		}}
		g.Battle.EnemyAttackCursor = 0 // next pick scans from slot 1
		return g
	}
	// Basic attack (SkillNone) is melee → must skip the back-row slot 1.
	g := newG()
	g.Battle.EnemyPendingSkill = SkillNone
	if got := PeekEnemyAttackerTarget(g); got != 2 {
		t.Errorf("basic-attack target = %d, want 2 (front-gated, skip back-row slot 1)", got)
	}
	// A pending skill casts at any row → the back-row slot 1 is reachable.
	g = newG()
	g.Battle.EnemyPendingSkill = SkillFirebolt
	if got := PeekEnemyAttackerTarget(g); got != 1 {
		t.Errorf("skill target = %d, want 1 (any row)", got)
	}
	// A live Taunt forces even a melee attacker onto the back-row taunter.
	g = newG()
	g.Battle.EnemyPendingSkill = SkillNone
	g.Packs = []Pack{{Members: []Enemy{{Alive: true, HP: 5, TauntTurns: 2, TauntedBy: 1}}}}
	g.Battle.ActivePack = 0
	g.Battle.EnemyAttacker = 0
	if got := PeekEnemyAttackerTarget(g); got != 1 {
		t.Errorf("taunted target = %d, want 1 (taunt overrides front gate)", got)
	}
}

// TestAoEReachesEnemy_MeleeFrontGatedRangedAll locks the shared AoE reach predicate
// behind the Swipe chevron/hit: a melee AoE is front-gated, a ranged/magic AoE
// sweeps any row, dead foes never count. Regression for Swipe chevroning the back row.
func TestAoEReachesEnemy_MeleeFrontGatedRangedAll(t *testing.T) {
	members := []Enemy{
		{Alive: true, Row: RowFront},
		{Alive: true, Row: RowBack},
	}
	if !AoEReachesEnemy(members, SkillSwipe, 0) {
		t.Error("melee AoE (Swipe) should reach the front-row foe")
	}
	if AoEReachesEnemy(members, SkillSwipe, 1) {
		t.Error("melee AoE (Swipe) must not reach the covered back-row foe")
	}
	if !AoEReachesEnemy(members, SkillFireball, 1) {
		t.Error("ranged AoE (Fireball) should reach the back-row foe (any row)")
	}
	members[0].Alive = false
	if AoEReachesEnemy(members, SkillFireball, 0) {
		t.Error("a dead foe must not be reachable")
	}
}

func TestEnemyEffectiveFront(t *testing.T) {
	members := []Enemy{
		{Alive: true, Row: RowFront},
		{Alive: true, Row: RowBack},
	}
	if !EnemyInEffectiveFront(members, 0) {
		t.Error("front enemy should be reachable")
	}
	if EnemyInEffectiveFront(members, 1) {
		t.Error("back enemy should be protected while the front stands")
	}
	members[0].Alive = false
	if !EnemyInEffectiveFront(members, 1) {
		t.Error("with the front enemy dead, the back enemy should be the effective front")
	}
}

func TestEnemyColumnCoverFullPack(t *testing.T) {
	// A full 2-front / 3-back pack. Back slots 0–1 sit behind a front foe (covered,
	// melee-proof); the 3rd back foe (slot 2) has no front column ahead → exposed.
	members := []Enemy{
		{Alive: true, Row: RowFront}, {Alive: true, Row: RowFront},
		{Alive: true, Row: RowBack}, {Alive: true, Row: RowBack}, {Alive: true, Row: RowBack},
	}
	for i := 2; i <= 3; i++ { // first two back foes are covered
		if !EnemyColumnCovered(members, i) {
			t.Errorf("back slot %d should be covered by a front foe", i-2)
		}
		if EnemyInEffectiveFront(members, i) {
			t.Errorf("covered back slot %d must not be meleeable", i-2)
		}
	}
	if EnemyColumnCovered(members, 4) {
		t.Error("3rd back foe (slot 2) has no front column ahead — must be uncovered")
	}
	if !EnemyInEffectiveFront(members, 4) {
		t.Error("uncovered 3rd back foe must be meleeable")
	}
}

// make2x2 builds a full, living 2×2 party with matching live + home slots.
func make2x2() []PartyMember {
	return []PartyMember{
		{HP: 10, HomeRow: RowFront, HomeCol: ColLeft, Row: RowFront, Col: ColLeft},
		{HP: 10, HomeRow: RowFront, HomeCol: ColRight, Row: RowFront, Col: ColRight},
		{HP: 10, HomeRow: RowBack, HomeCol: ColLeft, Row: RowBack, Col: ColLeft},
		{HP: 10, HomeRow: RowBack, HomeCol: ColRight, Row: RowBack, Col: ColRight},
	}
}

func TestSetBattleStartFormationSinksDownedFrontliner(t *testing.T) {
	// Front-left already downed before the fight. EngageFront (no rotation): the live
	// shunt sinks the dead one back and pulls the same-column backliner up. Home stays.
	party := make2x2()
	party[0].HP = 0
	SetBattleStartFormation(party, EngageFront)
	if party[0].Row != RowBack {
		t.Errorf("downed front-left should sink to back, live Row=%d", party[0].Row)
	}
	occ := liveSlot(party, RowFront, ColLeft)
	if occ < 0 || party[occ].HP <= 0 {
		t.Error("front-left should be manned by a living member after the shunt")
	}
	if party[0].HomeRow != RowFront || party[0].HomeCol != ColLeft {
		t.Error("home slot must not change — the formation reverts to it after battle")
	}
}

func TestShuntPartyFormationSameColumnSwap(t *testing.T) {
	party := make2x2()
	party[1].HP = 0 // front-right down; same-column backliner is member 3 (back-right)
	ShuntPartyFormation(party)
	if party[1].Row != RowBack || party[1].Col != ColRight {
		t.Errorf("downed front-right should sink to back-right, got (%d,%d)", party[1].Row, party[1].Col)
	}
	if party[3].Row != RowFront || party[3].Col != ColRight {
		t.Errorf("same-column backliner should rise to front-right, got (%d,%d)", party[3].Row, party[3].Col)
	}
}

func TestShuntPartyFormationCrossColumnFallback(t *testing.T) {
	party := make2x2()
	party[0].HP = 0 // front-left down
	party[2].HP = 0 // back-left (same column) ALSO down → must pull the other column's backliner
	ShuntPartyFormation(party)
	if occ := liveSlot(party, RowFront, ColLeft); occ != 3 {
		t.Errorf("front-left should be filled by the first living backliner (member 3), got member %d", occ)
	}
}

func TestAmbushLiveSlotUnique(t *testing.T) {
	homes := [][2]uint8{{0, 0}, {0, 1}, {1, 0}, {1, 1}} // (row, col) over the 2×2
	for _, side := range []EngageSide{EngageFront, EngageRight, EngageBack, EngageLeft} {
		seen := map[[2]uint8]bool{}
		for _, h := range homes {
			r := AmbushLiveRow(Row(h[0]), Col(h[1]), side)
			c := AmbushLiveCol(Row(h[0]), Col(h[1]), side)
			key := [2]uint8{uint8(r), uint8(c)}
			if seen[key] {
				t.Errorf("side %d: live slot (%d,%d) collides — rotation must stay a unique 2×2", side, r, c)
			}
			seen[key] = true
		}
		if len(seen) != 4 {
			t.Errorf("side %d: expected 4 unique live slots, got %d", side, len(seen))
		}
	}
}
