package core

// Hunger / satiety: a per-member meter that climbs as the party crawls and is
// pushed back down only by eating food. PartyMember.Hunger is the stored value
// (0 = Full, SatietyMax = Starving); everything here reads or moves it. Stored
// INVERTED (hunger, not satiety) so a zero value — a new member OR a save written
// before hunger existed — reads as Full instead of Starving.

// SatietyStage is the five-rung ladder shown in the UI. Only SatietyStarving has
// mechanical bite (the stat penalty + the no-healing rule, see EffectiveStatsPtr /
// HealMember); the rest are warnings the player watches climb.
type SatietyStage int

const (
	SatietyFull SatietyStage = iota
	SatietySated
	SatietyHungry
	SatietyFamished
	SatietyStarving
	SatietyStageCount
)

const (
	// SatietyMax is the Hunger value at which a member starves — three in-game days
	// of crawling (StepsPerCycle is one day) from a full belly to empty.
	SatietyMax = StepsPerCycle * 3
	// HungerPerStep is the Hunger a conscious member gains per landed step.
	HungerPerStep = 1
	// satietyStageSpan is the Hunger width of each of the five even stage bands.
	satietyStageSpan = SatietyMax / int(SatietyStageCount)
)

// init guards the integer division above: if SatietyMax stops being a clean
// multiple of SatietyStageCount (e.g. StepsPerCycle is retuned), the top band
// silently widens and StageForHunger drifts. Fail loudly at startup instead.
func init() {
	if SatietyMax%int(SatietyStageCount) != 0 {
		panic("core: SatietyMax must be a multiple of SatietyStageCount — stage bands would be uneven")
	}
}

// StageForHunger maps a stored Hunger value to its SatietyStage band.
func StageForHunger(hunger int) SatietyStage {
	if hunger <= 0 {
		return SatietyFull
	}
	s := hunger / satietyStageSpan
	if s >= int(SatietyStageCount) {
		return SatietyStarving
	}
	return SatietyStage(s)
}

// MemberStage is StageForHunger for a member.
func MemberStage(m PartyMember) SatietyStage { return StageForHunger(m.Hunger) }

// MemberStarving reports whether a member has bottomed out the meter — the gate
// for the starving stat penalty and the no-healing rule.
func MemberStarving(m PartyMember) bool { return StageForHunger(m.Hunger) == SatietyStarving }

// Satiety is the meter as the player reads it: SatietyMax (full) down to 0
// (starving) — the inverse of stored Hunger, for the UI gauge.
func Satiety(m PartyMember) int { return SatietyMax - m.Hunger }

var satietyStageLabels = [SatietyStageCount]string{
	SatietyFull:     "FULL",
	SatietySated:    "SATED",
	SatietyHungry:   "HUNGRY",
	SatietyFamished: "FAMISHED",
	SatietyStarving: "STARVING",
}

func init() { assertNoEmptyLabels("satietyStageLabels", satietyStageLabels[:]) }

// SatietyStageLabel returns the short uppercase label for a stage.
func SatietyStageLabel(s SatietyStage) string { return enumLabel(satietyStageLabels[:], s) }

// SatietyHungerPhrase humanizes a satiety amount as the noun phrase of hunger it
// covers, measured against a day of crawling (StepsPerCycle) so the ladder scales
// if the day length changes. Empty for a non-food (amount <= 0). Shared by the
// item detail card (nominal SatietyGain) and the eat log line (actual restored),
// so both read the same. Pair with "Heals " for a full sentence.
// multiDayHungerThreshold: satiety covering this many days of crawling reads as
// the plural "days' worth" band (the rest of the ladder uses fractions of a day).
const multiDayHungerThreshold = 2

func SatietyHungerPhrase(amount int) string {
	day := StepsPerCycle
	switch {
	case amount <= 0:
		return ""
	case amount >= multiDayHungerThreshold*day:
		return "days' worth of hunger"
	case amount >= day*7/8:
		return "a day's worth of hunger"
	case amount >= day/2:
		return "half a day's hunger"
	case amount >= day/4:
		return "a little hunger"
	default:
		return "a touch of hunger"
	}
}

// TickHungerStep raises every CONSCIOUS member's Hunger one step toward SatietyMax.
// Downed/ingested members don't burn food. Call once per landed exploration step
// (battles don't advance hunger, mirroring the day cycle).
func TickHungerStep(g *GameState) {
	if g == nil {
		return
	}
	for i := range g.Party {
		if !partyAvailable(g.Party[i]) {
			continue
		}
		GainUpTo(&g.Party[i].Hunger, SatietyMax, HungerPerStep)
	}
}

// FeedMember lowers a member's Hunger by amount (clamped at 0 = Full) and returns
// the satiety actually restored. Food is the ONLY thing that moves Hunger down —
// crystals and heals can't (see RestorePartyFully / HealMember).
func FeedMember(m *PartyMember, amount int) int {
	if m == nil || amount <= 0 {
		return 0
	}
	before := m.Hunger
	SubFloorZero(&m.Hunger, amount)
	return before - m.Hunger
}

// StarvingStatPenalty is the flat hit a Starving member takes on each of the SIX
// core stats — STR/DEX/INT/WIS/VIT/SPD (floored at 0 in the Effective* fold). It
// reaches everything those stats drive: damage, accuracy, magic, turn order, and
// stat-derived defenses (WIS→MDef). It does NOT touch flat physical Armor, which is
// not a Stat and so stays at full value while starving.
const StarvingStatPenalty = 3

// starvingPenalty returns the stat delta to fold while Starving (zero otherwise),
// so EffectiveStats and EffectiveDefenses apply the debuff from one definition.
func starvingPenalty(m *PartyMember) Stats {
	if !MemberStarving(*m) {
		return Stats{}
	}
	p := -StarvingStatPenalty
	return Stats{STR: p, DEX: p, INT: p, WIS: p, VIT: p, SPD: p}
}
