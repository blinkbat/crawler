package core

import (
	"fmt"
	"math/rand"
)

// WeaponType classifies a hand weapon by accuracy stat and reach: HEAVY is
// STR-governed, LIGHT/finesse DEX-governed (both melee and ranged). The basic
// attack reads it to pick the to-hit/damage stat. WeaponNone is unarmed (STR fists).
type WeaponType int

const (
	WeaponNone WeaponType = iota
	// Heavy melee — STR.
	WeaponSword
	WeaponAxe
	WeaponSpear
	WeaponTwoHandedSword
	WeaponHalberd
	WeaponGreataxe
	WeaponClub
	WeaponHammer
	// Light melee — DEX.
	WeaponDagger
	WeaponRapier
	// Light ranged — DEX.
	WeaponSling
	WeaponBow
	WeaponPistol
	WeaponThrowingKnives
	// Heavy ranged — STR.
	WeaponCrossbow
	WeaponArbalest
	WeaponTypeCount
)

// weaponSpec is one weapon-registry row: label, accuracy/damage stat, reach,
// and whether the strike is blunt (drives WeaponHitVFX — blunt/ranged thud vs
// edged slash).
type weaponSpec struct {
	Label    string
	Accuracy Stat // StatSTR (heavy) or StatDEX (light), melee or ranged
	Ranged   bool
	Blunt    bool // fists/maces — VFXImpact instead of VFXSlash on a melee hit
}

// weaponSpecs is the source of truth for every WeaponType. Fixed-size array
// keyed by the enum; the init assert catches an unfilled row.
var weaponSpecs = [WeaponTypeCount]weaponSpec{
	WeaponNone:           {Label: "Unarmed", Accuracy: StatSTR, Ranged: false, Blunt: true},
	WeaponSword:          {Label: "Sword", Accuracy: StatSTR, Ranged: false},
	WeaponAxe:            {Label: "Axe", Accuracy: StatSTR, Ranged: false},
	WeaponSpear:          {Label: "Spear", Accuracy: StatSTR, Ranged: false},
	WeaponTwoHandedSword: {Label: "Two-Handed Sword", Accuracy: StatSTR, Ranged: false},
	WeaponHalberd:        {Label: "Halberd", Accuracy: StatSTR, Ranged: false},
	WeaponGreataxe:       {Label: "Greataxe", Accuracy: StatSTR, Ranged: false},
	WeaponClub:           {Label: "Club", Accuracy: StatSTR, Ranged: false, Blunt: true},
	WeaponHammer:         {Label: "Hammer", Accuracy: StatSTR, Ranged: false, Blunt: true},
	WeaponDagger:         {Label: "Dagger", Accuracy: StatDEX, Ranged: false},
	WeaponRapier:         {Label: "Rapier", Accuracy: StatDEX, Ranged: false},
	WeaponSling:          {Label: "Sling", Accuracy: StatDEX, Ranged: true},
	WeaponBow:            {Label: "Bow", Accuracy: StatDEX, Ranged: true},
	WeaponPistol:         {Label: "Pistol", Accuracy: StatDEX, Ranged: true},
	WeaponThrowingKnives: {Label: "Throwing Knives", Accuracy: StatDEX, Ranged: true},
	WeaponCrossbow:       {Label: "Crossbow", Accuracy: StatSTR, Ranged: true},
	WeaponArbalest:       {Label: "Arbalest", Accuracy: StatSTR, Ranged: true},
}

func init() {
	for wt := WeaponType(0); wt < WeaponTypeCount; wt++ {
		if weaponSpecs[wt].Label == "" {
			panic(fmt.Sprintf("core: weaponSpecs missing a row for WeaponType %d — add its accuracy/reach", wt))
		}
		// Accuracy must be STR or DEX; anything else governs to-hit off the wrong stat.
		if a := weaponSpecs[wt].Accuracy; a != StatSTR && a != StatDEX {
			panic(fmt.Sprintf("core: weaponSpecs[%d] Accuracy must be StatSTR or StatDEX, got %d", wt, a))
		}
	}
}

// validWeaponType reports whether wt indexes weaponSpecs. Shared bounds rule for
// the weapon lookups.
func validWeaponType(wt WeaponType) bool {
	return wt >= 0 && wt < WeaponTypeCount
}

// WeaponAccuracyStat returns the to-hit/damage stat (STR heavy/unarmed, DEX
// light/ranged). Out-of-range falls back to STR.
func WeaponAccuracyStat(wt WeaponType) Stat {
	if !validWeaponType(wt) {
		return StatSTR
	}
	return weaponSpecs[wt].Accuracy
}

// WeaponIsRanged reports whether the weapon strikes at range. Drives the to-hit
// stat AND the flying-target rule (CanReachFlying).
func WeaponIsRanged(wt WeaponType) bool {
	if !validWeaponType(wt) {
		return false
	}
	return weaponSpecs[wt].Ranged
}

// CanReachFlying reports whether a weapon strikes a Flying enemy without the
// melee penalty (ranged can; melee/unarmed can't). Single home for the rule.
func CanReachFlying(wt WeaponType) bool {
	return WeaponIsRanged(wt)
}

// EquippedWeapon returns the WeaponType governing the basic attack: the right
// hand wins, else fall back to a LEFT-hand weapon (either hand is allowed).
// WeaponNone when neither holds a weapon (unarmed/STR-fists).
func EquippedWeapon(m PartyMember) WeaponType {
	if rh := ItemInfo(m.Equipped[EquipRightHand]).Weapon; rh != WeaponNone {
		return rh
	}
	if lh := ItemInfo(m.Equipped[EquipLeftHand]).Weapon; lh != WeaponNone {
		return lh
	}
	return WeaponNone
}

// WeaponHitVFX returns the basic-attack impact VFX. Blunt strikes (fists, club,
// hammer) and ranged hits are VFXImpact ("thud"); edged melee is VFXSlash. Out
// of range falls back to VFXImpact.
func WeaponHitVFX(wt WeaponType) VFXKind {
	if !validWeaponType(wt) {
		return VFXImpact
	}
	if weaponSpecs[wt].Blunt || weaponSpecs[wt].Ranged {
		return VFXImpact
	}
	return VFXSlash
}

// memberAttackAccuracy is the basic-attack hit chance [0,1]: the accuracy curve
// over the equipped weapon's governing stat at the given timing grade.
func memberAttackAccuracy(m PartyMember, quality int) float64 {
	stat := WeaponAccuracyStat(EquippedWeapon(m))
	return accuracyFrom(StatValue(EffectiveStats(m), stat), quality)
}

// MemberAttackHits rolls the weapon-governed basic-attack hit chance.
func MemberAttackHits(rng *rand.Rand, m PartyMember, quality int) bool {
	return RollChance(rng, memberAttackAccuracy(m, quality))
}

// memberAttackAccuracyVs folds the flying-target penalty in: a melee swing at a
// Flying enemy loses FlyingMeleeAccuracyPenalty (ranged shrugs it). Applied
// post-clamp, so it can pull even an Excellent press below a guaranteed hit.
func memberAttackAccuracyVs(m PartyMember, flying bool, quality int) float64 {
	acc := memberAttackAccuracy(m, quality)
	if flying && !CanReachFlying(EquippedWeapon(m)) {
		acc = Clamp(acc-FlyingMeleeAccuracyPenalty, 0, 1)
	}
	return acc
}

// MemberAttackHitsTarget rolls the basic-attack hit chance against a known
// defender, applying the flying-target penalty. Prefer this when the target is
// in hand; bare MemberAttackHits stays for attacker-only callers (previews/tests).
func MemberAttackHitsTarget(rng *rand.Rand, m PartyMember, target Enemy, quality int) bool {
	return RollChance(rng, memberAttackAccuracyVs(m, EnemyInfoFor(target).Flying, quality))
}

// MemberMeleeReachesFlyer reports whether the basic attack reaches a Flying
// target without penalty (ranged weapon). The battle layer flavors a flyer
// whiff as "out of reach".
func MemberMeleeReachesFlyer(m PartyMember) bool {
	return CanReachFlying(EquippedWeapon(m))
}

// MemberAttackDamage is the basic-attack pre-quality damage: the equipped
// weapon's governing stat (via EffectiveStats, so StatBonus folds in) plus base.
// (No per-weapon base-damage value yet, so base is 0 today.)
func MemberAttackDamage(m PartyMember, base int) int {
	stat := WeaponAccuracyStat(EquippedWeapon(m))
	return StatValue(EffectiveStats(m), stat) + base
}
