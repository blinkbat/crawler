package core

// Dialog speaker registry — named voices a node can be attributed to. The
// editor's dropdown reads DialogSpeakerIDs, so a voice added here shows up there.

// Canonical speaker ids; party-class ids match the class names.
const (
	SpeakerNarrator DialogSpeakerID = "narrator"
	SpeakerStranger DialogSpeakerID = "stranger"
	SpeakerWarrior  DialogSpeakerID = "warrior"
	SpeakerCleric   DialogSpeakerID = "cleric"
	SpeakerThief    DialogSpeakerID = "thief"
	SpeakerWizard   DialogSpeakerID = "wizard"
)

// dialogSpeakerList is the single source of truth, in display order. The lookup
// map and id-order slice are derived from it in init, so adding a voice is one
// edit here and the two views can't drift.
var dialogSpeakerList = []DialogSpeaker{
	{ID: SpeakerNarrator, Name: "Narrator", TintR: 198, TintG: 190, TintB: 170, TintA: 255},
	{ID: SpeakerStranger, Name: "Stranger", TintR: 150, TintG: 120, TintB: 90, TintA: 255},
	{ID: SpeakerWarrior, Name: "Warrior", TintR: 196, TintG: 80, TintB: 72, TintA: 255},
	{ID: SpeakerCleric, Name: "Cleric", TintR: 220, TintG: 200, TintB: 120, TintA: 255},
	{ID: SpeakerThief, Name: "Thief", TintR: 110, TintG: 170, TintB: 110, TintA: 255},
	{ID: SpeakerWizard, Name: "Wizard", TintR: 110, TintG: 130, TintB: 210, TintA: 255},
}

var (
	dialogSpeakers     = make(map[DialogSpeakerID]DialogSpeaker, len(dialogSpeakerList))
	dialogSpeakerOrder = make([]DialogSpeakerID, 0, len(dialogSpeakerList))
)

func init() {
	for _, sp := range dialogSpeakerList {
		if _, dup := dialogSpeakers[sp.ID]; dup {
			panic("core: duplicate dialog speaker id " + string(sp.ID))
		}
		dialogSpeakers[sp.ID] = sp
		dialogSpeakerOrder = append(dialogSpeakerOrder, sp.ID)
	}
	// Each party class needs a speaker whose id == its PartyClassSlug, else that
	// class has no dialog voice. Renaming a class display name silently shifts its
	// slug, so assert the mapping holds at boot rather than discovering a mute class
	// in-game. (Speaker ids above are spelled to match the current class names.)
	for _, class := range AllPartyClasses() {
		if _, ok := dialogSpeakers[DialogSpeakerID(PartyClassSlug(class))]; !ok {
			panic("core: party class " + PartyClassName(class) + " (slug " + PartyClassSlug(class) +
				") has no matching dialog speaker — add one to dialogSpeakerList or realign the SpeakerXxx id")
		}
	}
}

// DialogSpeakerByID returns the registered speaker, or (zero, false).
func DialogSpeakerByID(id DialogSpeakerID) (DialogSpeaker, bool) {
	sp, ok := dialogSpeakers[id]
	return sp, ok
}

// DialogSpeakerName resolves an id to its display name (falls back to the raw id, or "???" when empty).
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
