package core

import "strings"

// Quest journal. A plain list in the char menu's Quests tab. No auto-completion
// engine — gameplay hooks call CompleteQuest / AddQuest. Save-persisted.

// QuestStatus is the lifecycle state of a quest entry.
type QuestStatus int

const (
	QuestActive QuestStatus = iota
	QuestComplete

	questStatusCount // sentinel: QuestStatus cardinality (assertAppendOnly coverage)
)

// init pins each QuestStatus's serialized int value (saved as Quest.Status and as
// the dialog "questStatus" condition field). A mid-enum insert renumbers saved
// statuses; this panics at startup instead. APPEND a new status, then pin it here.
func init() {
	assertAppendOnly("QuestStatus (renumbers saved quest statuses)", int(questStatusCount),
		QuestActive, QuestComplete)
}

// Valid reports whether s is a recognized QuestStatus — the single source for
// the legal set, so adding a third status is a one-line edit here.
func (s QuestStatus) Valid() bool {
	return s >= 0 && s < questStatusCount
}

// Quest is one journal entry. ID is a stable string key for CompleteQuest /
// QuestIndexByID; Title / Desc are player-facing.
type Quest struct {
	ID     string
	Title  string
	Desc   string
	Status QuestStatus
}

// IsComplete reports whether the quest is marked done.
func (q Quest) IsComplete() bool { return q.Status == QuestComplete }

// QuestInvestigateIsland is the stable ID of the opening quest, in one place
// rather than retyped at each call site.
const QuestInvestigateIsland = "investigate-island"

// StarterQuests is the journal a fresh game begins with.
func StarterQuests() []Quest {
	return []Quest{
		{
			ID:     QuestInvestigateIsland,
			Title:  "Investigate the Island",
			Desc:   "Explore the island and uncover what drew you here.",
			Status: QuestActive,
		},
	}
}

// QuestTitleFromID derives a readable player-facing title from a quest's stable
// slug ("rescue-the-elder" → "Rescue The Elder"). Used to seed quests that have
// no authored title (dialog-started quests carry only an ID), so the journal shows
// words, not the raw slug.
func QuestTitleFromID(id string) string {
	words := strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(id))
	for i, w := range words {
		r := []rune(w)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// QuestIndexByID returns the index of the quest with the given ID, or -1. The
// lookup seam every quest mutation goes through.
func QuestIndexByID(quests []Quest, id string) int {
	return indexByID(quests, id, func(q Quest) string { return q.ID })
}

// AddQuest appends a quest if its ID isn't already present and returns the
// updated slice. Idempotent on ID (a hook firing twice can't duplicate).
func AddQuest(quests []Quest, q Quest) []Quest {
	if QuestIndexByID(quests, q.ID) >= 0 {
		return quests
	}
	return append(quests, q)
}

// CompleteQuest marks the quest complete and reports whether it transitioned
// (false if unknown or already complete — gate a one-time fanfare on this).
func CompleteQuest(g *GameState, id string) bool {
	idx := QuestIndexByID(g.Quests, id)
	if idx < 0 || g.Quests[idx].Status == QuestComplete {
		return false
	}
	g.Quests[idx].Status = QuestComplete
	return true
}

// ActiveQuestCount / CompletedQuestCount summarize the log for the tab header.
func ActiveQuestCount(quests []Quest) int {
	return countWhere(quests, func(q Quest) bool { return q.Status == QuestActive })
}

func CompletedQuestCount(quests []Quest) int {
	return countWhere(quests, func(q Quest) bool { return q.Status == QuestComplete })
}
