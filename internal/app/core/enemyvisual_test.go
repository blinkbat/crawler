package core

import "testing"

// TestSlugify pins the slug derivation (PNG filename + visuals.json key).
func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Feral Rat", "feral_rat"},
		{"Will-o'-Wisp", "will_o_wisp"},
		{"Goblin Mage", "goblin_mage"},
		{"Stone Golem", "stone_golem"},
		{"  Spaced  Out  ", "spaced_out"},
		{"Already_slug", "already_slug"},
		{"UPPER", "upper"},
	}
	for _, c := range cases {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestEnemySlugRatMatchesAsset guards the feral_rat.png filename so a Name change fails here.
func TestEnemySlugRatMatchesAsset(t *testing.T) {
	if got := EnemySlug(EnemyRat); got != "feral_rat" {
		t.Fatalf("EnemySlug(EnemyRat) = %q, want %q (the feral_rat.png asset key)", got, "feral_rat")
	}
}
