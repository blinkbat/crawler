package core

import "testing"

// TestSlugify pins the slug derivation. The enemy sprite PNG filename and the
// visuals.json override key both derive from EnemySlug → slugify(Name), so a
// regression here silently breaks sprite loading and orphans saved overrides.
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

// TestEnemySlugRatMatchesAsset guards the specific filename the render layer
// loads (maps/sprites/feral_rat.png). loadEnemyVisuals now builds that name
// from EnemySlug(EnemyRat); if the rat's display Name ever changes, this test
// fails loudly instead of the rat silently falling back to procedural art.
func TestEnemySlugRatMatchesAsset(t *testing.T) {
	if got := EnemySlug(EnemyRat); got != "feral_rat" {
		t.Fatalf("EnemySlug(EnemyRat) = %q, want %q (the feral_rat.png asset key)", got, "feral_rat")
	}
}
