package core

import "testing"

// TestPreviewRestorative_MatchesApply pins PreviewRestorative as the non-mutating
// twin of ApplyRestorative: for every member×item pair the projected result must
// equal what ApplyRestorative actually applies to a copy — including the feed-first
// starving-lift case (a big meal clears Starving so the same item's heal lands).
func TestPreviewRestorative_MatchesApply(t *testing.T) {
	members := map[string]PartyMember{
		"wounded, fed":       {HP: 8, MaxHP: 20, MP: 2, MaxMP: 10, Hunger: 0},
		"full HP, low MP":    {HP: 20, MaxHP: 20, MP: 1, MaxMP: 10, Hunger: 0},
		"topped off":         {HP: 20, MaxHP: 20, MP: 10, MaxMP: 10, Hunger: 0},
		"starving, wounded":  {HP: 8, MaxHP: 20, MP: 2, MaxMP: 10, Hunger: SatietyMax},
		"hungry, wounded":    {HP: 8, MaxHP: 20, MP: 2, MaxMP: 10, Hunger: SatietyMax / 2},
		"downed (no target)": {HP: 0, MaxHP: 20, MP: 0, MaxMP: 10, Hunger: SatietyMax},
	}
	items := []ItemKind{
		ItemHealthPotion, ItemMagicPhial, ItemBatJerky, ItemMagicalBerries, ItemCrustOfBread,
	}
	for mname, base := range members {
		for _, kind := range items {
			def := ItemInfo(kind)
			applied := base // copy — ApplyRestorative mutates
			want := ApplyRestorative(&applied, def)
			got := PreviewRestorative(base, def)
			if got != want {
				t.Errorf("%s + %s: preview %+v, apply %+v", mname, def.Name, got, want)
			}
		}
	}
}
