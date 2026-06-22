package core

// Dialog speaker registry — named voices a node can be attributed to (four
// party classes + generic NPC voices). The editor's speaker dropdown reads
// DialogSpeakerIDs, so a voice added here shows up there automatically.

// Canonical speaker ids; party-class ids match the class names.
const (
	SpeakerNarrator DialogSpeakerID = "narrator"
	SpeakerStranger DialogSpeakerID = "stranger"
	SpeakerWarrior  DialogSpeakerID = "warrior"
	SpeakerCleric   DialogSpeakerID = "cleric"
	SpeakerThief    DialogSpeakerID = "thief"
	SpeakerWizard   DialogSpeakerID = "wizard"
)

// dialogSpeakerOrder is the canonical display order; dialogSpeakers is the
// id→speaker lookup. The init guard keeps the two in lockstep.
var dialogSpeakerOrder = []DialogSpeakerID{
	SpeakerNarrator,
	SpeakerStranger,
	SpeakerWarrior,
	SpeakerCleric,
	SpeakerThief,
	SpeakerWizard,
}

var dialogSpeakers = map[DialogSpeakerID]DialogSpeaker{
	SpeakerNarrator: {ID: SpeakerNarrator, Name: "Narrator", TintR: 198, TintG: 190, TintB: 170, TintA: 255},
	SpeakerStranger: {ID: SpeakerStranger, Name: "Stranger", TintR: 150, TintG: 120, TintB: 90, TintA: 255},
	SpeakerWarrior:  {ID: SpeakerWarrior, Name: "Warrior", TintR: 196, TintG: 80, TintB: 72, TintA: 255},
	SpeakerCleric:   {ID: SpeakerCleric, Name: "Cleric", TintR: 220, TintG: 200, TintB: 120, TintA: 255},
	SpeakerThief:    {ID: SpeakerThief, Name: "Thief", TintR: 110, TintG: 170, TintB: 110, TintA: 255},
	SpeakerWizard:   {ID: SpeakerWizard, Name: "Wizard", TintR: 110, TintG: 130, TintB: 210, TintA: 255},
}

func init() {
	if len(dialogSpeakerOrder) != len(dialogSpeakers) {
		panic("core: dialogSpeakerOrder and dialogSpeakers must list the same speakers")
	}
	for _, id := range dialogSpeakerOrder {
		if _, ok := dialogSpeakers[id]; !ok {
			panic("core: dialogSpeakerOrder lists " + string(id) + " but dialogSpeakers has no entry")
		}
	}
}

// DialogSpeakerByID returns the registered speaker, or (zero, false).
func DialogSpeakerByID(id DialogSpeakerID) (DialogSpeaker, bool) {
	sp, ok := dialogSpeakers[id]
	return sp, ok
}

// DialogSpeakerName resolves an id to its display name, falling back to the
// raw id (or "???" when empty) so a typo'd speaker still draws something.
func DialogSpeakerName(id DialogSpeakerID) string {
	if sp, ok := dialogSpeakers[id]; ok {
		return sp.Name
	}
	if id == "" {
		return "???"
	}
	return string(id)
}

// DialogSpeakerIDs returns the speaker ids in canonical display order.
func DialogSpeakerIDs() []DialogSpeakerID {
	return append([]DialogSpeakerID(nil), dialogSpeakerOrder...)
}
