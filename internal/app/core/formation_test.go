package core

import (
	"testing"

	"crawler/internal/app/core/mapfile"
)

func TestAttackClassification(t *testing.T) {
	// Basic attacks: ranged weapon → Ranged, everything else (incl. unarmed) → Melee.
	if got := BasicAttackClass(WeaponSword); got != AttackMelee {
		t.Errorf("sword basic = %d, want Melee", got)
	}
	if got := BasicAttackClass(WeaponNone); got != AttackMelee {
		t.Errorf("unarmed basic = %d, want Melee", got)
	}
	if got := BasicAttackClass(WeaponBow); got != AttackRanged {
		t.Errorf("bow basic = %d, want Ranged", got)
	}
	// Skills classify by kind: melee front-gated, magic/heal = magic, utility = ranged.
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
	// FL, FR front (alive); BL, BR back (alive).
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
	// A pack authored with a back-row member should decode with that row, and
	// re-encode with BackCount intact (back members ordered last).
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
	// Re-encode and confirm the row count survives.
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
	// Right ambush: the right column becomes the front, regardless of home row.
	if AmbushLiveRow(RowBack, ColRight, EngageRight) != RowFront ||
		AmbushLiveRow(RowFront, ColLeft, EngageRight) != RowBack {
		t.Error("right ambush should bring the right column to the front")
	}
	// Left ambush: the left column becomes the front.
	if AmbushLiveRow(RowBack, ColLeft, EngageLeft) != RowFront ||
		AmbushLiveRow(RowFront, ColRight, EngageLeft) != RowBack {
		t.Error("left ambush should bring the left column to the front")
	}
}

func TestNewPartyFormationIs2x2(t *testing.T) {
	// Default party should form a proper 2×2: each row has one left + one right.
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
	// Find one front and one back member to swap across rows.
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
	// Live reach Row resyncs to the swapped home row.
	if party[front].Row != party[front].HomeRow || party[back].Row != party[back].HomeRow {
		t.Error("live Row should resync to HomeRow after a swap")
	}
	// Still a clean 2×2 — never three in one row.
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
	// No-op guards: self, and out-of-range indices, must not panic or mutate.
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
	// Repaired layout must match the fresh default (default row by class).
	for i := range party {
		want := DefaultPartyRow(party[i].Class)
		if party[i].HomeRow != want || party[i].Row != want {
			t.Errorf("member %d row = %d/%d, want %d (default by class)", i, party[i].HomeRow, party[i].Row, want)
		}
	}
	// A VALID custom layout (e.g. a swap) must be preserved untouched.
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
	// 3 front + 2 back; kill a front enemy → a back one is promoted to keep the
	// front packed at 3 (while ≥3 alive).
	members := []Enemy{
		{Alive: true, Row: RowFront},
		{Alive: true, Row: RowFront},
		{Alive: true, Row: RowFront},
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
	if front != 3 {
		t.Errorf("after shunt, living front = %d, want 3 (a back enemy should fill the gap)", front)
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
	// First melee target after the cursor must be a front-row member (skip back).
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
