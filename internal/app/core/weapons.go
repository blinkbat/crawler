package core

import (
	"fmt"
	"math/rand"
)

// WeaponType classifies a hand weapon by its governing accuracy stat and
// its reach. The split is the same on both the melee and ranged sides:
// HEAVY weapons (greataxe, war hammer; crossbow, arbalest) are
// STR-governed, while LIGHT / finesse weapons (dagger, rapier; sling, bow,
// throwing knives) are DEX-governed — so a strong fighter spans a crossbow
// or swings a greataxe reliably while a nimble rogue lands a dagger or a
// short bow. Ranged is NOT uniformly DEX anymore: a heavy crossbow needs
// strength to span and steady, so it rolls off STR. The basic attack reads
// the wielder's equipped weapon to pick which stat rolls the to-hit (and
// scales the damage) — see MemberAttackHits / MemberAttackDamage.
// WeaponNone is unarmed / a non-weapon hand item (e.g. a shield): a STR
// melee strike (fists).
type WeaponType int

const (
	WeaponNone WeaponType = iota
	// Heavy melee — STR-governed.
	WeaponSword
	WeaponAxe
	WeaponSpear
	WeaponTwoHandedSword
	WeaponHalberd
	WeaponGreataxe
	WeaponClub
	WeaponHammer
	// Light / finesse melee — DEX-governed.
	WeaponDagger
	WeaponRapier
	// Light ranged — DEX-governed (quick, finesse).
	WeaponSling
	WeaponBow
	WeaponPistol
	WeaponThrowingKnives
	// Heavy ranged — STR-governed. A crossbow/arbalest needs strength to
	// span and steady, so its to-hit (and basic-attack damage) rolls off
	// STR, not DEX — the ranged mirror of the heavy-vs-light melee split.
	WeaponCrossbow
	WeaponArbalest
	WeaponTypeCount
)

// weaponSpec is one row of the weapon registry: display label, the stat
// that governs accuracy + basic-attack damage, and whether it strikes at
// range.
type weaponSpec struct {
	Label    string
	Accuracy Stat // StatSTR (heavy melee / heavy ranged) or StatDEX (light melee / light ranged)
	Ranged   bool
}

// weaponSpecs is the source of truth for every WeaponType. Fixed-size
// array keyed by the enum, so its length is compile-time-locked; the init
// assert below catches an unfilled row, so a new weapon can't ship
// without its accuracy / reach classification.
var weaponSpecs = [WeaponTypeCount]weaponSpec{
	WeaponNone:           {Label: "Unarmed", Accuracy: StatSTR, Ranged: false},
	WeaponSword:          {Label: "Sword", Accuracy: StatSTR, Ranged: false},
	WeaponAxe:            {Label: "Axe", Accuracy: StatSTR, Ranged: false},
	WeaponSpear:          {Label: "Spear", Accuracy: StatSTR, Ranged: false},
	WeaponTwoHandedSword: {Label: "Two-Handed Sword", Accuracy: StatSTR, Ranged: false},
	WeaponHalberd:        {Label: "Halberd", Accuracy: StatSTR, Ranged: false},
	WeaponGreataxe:       {Label: "Greataxe", Accuracy: StatSTR, Ranged: false},
	WeaponClub:           {Label: "Club", Accuracy: StatSTR, Ranged: false},
	WeaponHammer:         {Label: "Hammer", Accuracy: StatSTR, Ranged: false},
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
		// Accuracy is documented as STR (heavy/unarmed) or DEX (light/ranged)
		// only; anything else would silently govern to-hit off the wrong stat.
		if a := weaponSpecs[wt].Accuracy; a != StatSTR && a != StatDEX {
			panic(fmt.Sprintf("core: weaponSpecs[%d] Accuracy must be StatSTR or StatDEX, got %d", wt, a))
		}
	}
}

// validWeaponType reports whether wt indexes weaponSpecs. The single bounds
// rule the weapon lookups share so an out-of-range kind falls back uniformly.
func validWeaponType(wt WeaponType) bool {
	return wt >= 0 && wt < WeaponTypeCount
}

// WeaponAccuracyStat returns the stat that governs a weapon's to-hit and
// basic-attack damage (StatSTR for heavy / unarmed, StatDEX for light +
// ranged). Out-of-range falls back to STR (the unarmed default).
func WeaponAccuracyStat(wt WeaponType) Stat {
	if !validWeaponType(wt) {
		return StatSTR
	}
	return weaponSpecs[wt].Accuracy
}

// WeaponIsRanged reports whether the weapon strikes at range. Drives the
// to-hit stat via WeaponAccuracyStat AND, now, the flying-target rule via
// CanReachFlying — a ranged weapon ignores the melee-vs-flyer penalty. Still
// the seam for further ranged-only rules (line of sight, distinct VFX).
func WeaponIsRanged(wt WeaponType) bool {
	if !validWeaponType(wt) {
		return false
	}
	return weaponSpecs[wt].Ranged
}

// CanReachFlying reports whether a weapon can strike a Flying enemy without
// the melee penalty. Ranged weapons can; melee (and unarmed) can't. The
// single predicate the flying-target accuracy rule reads, so "what reaches a
// flyer?" lives in one place if the rule ever grows (e.g. reach polearms).
func CanReachFlying(wt WeaponType) bool {
	return WeaponIsRanged(wt)
}

// EquippedWeapon returns the WeaponType governing the member's basic attack.
// The right hand wins when it holds a real weapon; otherwise, since
// CanEquipInSlot lets a weapon sit in either hand, fall back to a weapon in
// the LEFT hand (e.g. a sword off-hand beside a shield-less right hand) so a
// left-equipped weapon isn't silently ignored. An empty slot resolves to
// ItemNone whose ItemDefinition leaves Weapon at WeaponNone, and a non-weapon
// hand item (a shield) is likewise WeaponNone — so when NEITHER hand holds a
// real weapon this returns WeaponNone (the existing unarmed/STR-fists case).
func EquippedWeapon(m PartyMember) WeaponType {
	if rh := ItemInfo(m.Equipped[EquipRightHand]).Weapon; rh != WeaponNone {
		return rh
	}
	if lh := ItemInfo(m.Equipped[EquipLeftHand]).Weapon; lh != WeaponNone {
		return lh
	}
	return WeaponNone
}

// WeaponHitVFX returns the impact VFX kind for a BASIC attack with this weapon,
// driving both the particle burst and the clarity glyph (impact ring vs slash
// stroke). Blunt strikes — unarmed fists, club, hammer — and ranged projectile
// hits read as a percussive VFXImpact ("thud"), while edged melee (sword, axe,
// spear, dagger, rapier, halberd, greataxe, two-hander) reads as a VFXSlash
// "cut." So an unarmed punch or a hammer blow shows an impact, not a slice. Out
// of range falls back to VFXImpact (the unarmed default, matching
// WeaponAccuracyStat's STR-fists fallback).
func WeaponHitVFX(wt WeaponType) VFXKind {
	if !validWeaponType(wt) {
		return VFXImpact
	}
	switch wt {
	case WeaponNone, WeaponClub, WeaponHammer:
		return VFXImpact
	}
	if WeaponIsRanged(wt) {
		return VFXImpact
	}
	return VFXSlash
}

// memberAttackAccuracy is the basic-attack hit chance [0,1] for a member:
// the shared accuracy curve over the governing stat of their equipped
// weapon (STR for heavy / unarmed, DEX for light + ranged) at the given
// timing grade. Skills are not gated; this is the basic attack only.
// Unexported — callers roll via MemberAttackHits.
func memberAttackAccuracy(m PartyMember, quality int) float64 {
	stat := WeaponAccuracyStat(EquippedWeapon(m))
	return accuracyFrom(StatValue(EffectiveStats(m), stat), quality)
}

// MemberAttackHits rolls the weapon-governed basic-attack hit chance.
func MemberAttackHits(rng *rand.Rand, m PartyMember, quality int) bool {
	return RollChance(rng, memberAttackAccuracy(m, quality))
}

// memberAttackAccuracyVs folds the flying-target penalty into the basic-
// attack hit chance: a melee swing at a Flying enemy loses
// FlyingMeleeAccuracyPenalty off the top (a ranged weapon shrugs it via
// CanReachFlying). Applied post-clamp, so it can pull even an Excellent
// press below a guaranteed hit — melee-vs-flyer is meant to be unreliable.
func memberAttackAccuracyVs(m PartyMember, flying bool, quality int) float64 {
	acc := memberAttackAccuracy(m, quality)
	if flying && !CanReachFlying(EquippedWeapon(m)) {
		acc = Clamp(acc-FlyingMeleeAccuracyPenalty, 0, 1)
	}
	return acc
}

// MemberAttackHitsTarget rolls the basic-attack hit chance against a known
// defender, applying the flying-target melee penalty. Prefer this over
// MemberAttackHits wherever the target enemy is in hand (battle's
// applyAttack); the bare MemberAttackHits stays for callers that only have
// the attacker (previews / tests).
func MemberAttackHitsTarget(rng *rand.Rand, m PartyMember, target Enemy, quality int) bool {
	return RollChance(rng, memberAttackAccuracyVs(m, EnemyInfoFor(target).Flying, quality))
}

// MemberMeleeReachesFlyer reports whether the member's basic attack can
// reach a Flying target without penalty — i.e. they're wielding a ranged
// weapon. Used by the battle layer to flavor a flyer whiff as "out of
// reach" rather than a plain miss.
func MemberMeleeReachesFlyer(m PartyMember) bool {
	return CanReachFlying(EquippedWeapon(m))
}

// MemberAttackDamage is the basic-attack pre-quality damage for a member:
// the governing stat of their equipped weapon plus base, read through
// EffectiveStats so equipment StatBonus folds in. A DEX weapon both hits
// AND hurts off finesse; a heavy weapon (or fists) off STR — so the basic
// attack's stat tracks the weapon, not a fixed STR. (Weapons carry no
// per-weapon base-damage value yet, so base is 0 today, matching the
// pre-weapon-types basic attack for the STR case.)
func MemberAttackDamage(m PartyMember, base int) int {
	stat := WeaponAccuracyStat(EquippedWeapon(m))
	return StatValue(EffectiveStats(m), stat) + base
}
