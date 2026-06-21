package core

// Quest journal. A plain list of entries the player reads from the char
// menu's Quests tab. Deliberately simple: each quest carries a title, a
// one-line description, and an Active/Complete status. There's no
// auto-completion engine — gameplay hooks call CompleteQuest / AddQuest when
// a condition is met. A fresh game seeds the opening objective via
// StarterQuests (so the Journal tab isn't empty); the system is wired and
// save-persisted, ready for more objectives later.

// QuestStatus is the lifecycle state of a quest entry.
type QuestStatus int

const (
	QuestActive QuestStatus = iota
	QuestComplete
)

// Valid reports whether s is a recognized QuestStatus. The single source of
// truth for the legal status set — save-load's clamp defers to it so adding a
// third status (e.g. QuestFailed) is a one-line edit here, not a hunt for every
// hand-rolled `!= QuestActive && != QuestComplete` check.
func (s QuestStatus) Valid() bool {
	return s == QuestActive || s == QuestComplete
}

// Quest is one journal entry. ID is a stable string key for CompleteQuest /
// QuestIndexByID lookups (so gameplay code references a quest without a
// fragile slice index); Title / Desc are the player-facing text.
type Quest struct {
	ID     string
	Title  string
	Desc   string
	Status QuestStatus
}

// IsComplete reports whether the quest has been marked done.
func (q Quest) IsComplete() bool { return q.Status == QuestComplete }

// QuestInvestigateIsland is the stable ID of the opening quest every fresh
// game begins with. Referenced by gameplay hooks (and dialog quest actions)
// that complete it, so the string lives in one place rather than being
// retyped at each call site.
const QuestInvestigateIsland = "investigate-island"

// StarterQuests is the journal a fresh game begins with. Seeds the opening
// objective so a new run's Journal tab isn't empty; gameplay hooks (or a
// dialog's quest action) call CompleteQuest(QuestInvestigateIsland) when the
// island is explored.
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

// QuestIndexByID returns the index of the quest with the given ID, or -1 if
// the log has no such entry. The lookup seam every quest mutation goes
// through so "which slot is this quest?" lives in one place.
func QuestIndexByID(quests []Quest, id string) int {
	for i := range quests {
		if quests[i].ID == id {
			return i
		}
	}
	return -1
}

// AddQuest appends a quest to the log if its ID isn't already present, and
// returns the updated slice. Idempotent on ID so a gameplay hook that fires
// twice can't add a duplicate journal entry.
func AddQuest(quests []Quest, q Quest) []Quest {
	if QuestIndexByID(quests, q.ID) >= 0 {
		return quests
	}
	return append(quests, q)
}

// CompleteQuest marks the quest with the given ID complete and reports
// whether it actually transitioned (false if the quest is unknown or was
// already complete — so a caller can gate a one-time "quest complete!"
// fanfare on the return value).
func CompleteQuest(g *GameState, id string) bool {
	idx := QuestIndexByID(g.Quests, id)
	if idx < 0 || g.Quests[idx].Status == QuestComplete {
		return false
	}
	g.Quests[idx].Status = QuestComplete
	return true
}

// ActiveQuestCount / CompletedQuestCount summarize the log for the Quests
// tab's header.
func ActiveQuestCount(quests []Quest) int {
	return countWhere(quests, func(q Quest) bool { return q.Status == QuestActive })
}

func CompletedQuestCount(quests []Quest) int {
	return countWhere(quests, func(q Quest) bool { return q.Status == QuestComplete })
}
