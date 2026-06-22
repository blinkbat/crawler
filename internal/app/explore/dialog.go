package explore

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// startFirstAreaDialog is the Debug ▸ Start Dialog launcher: opens the area's
// first conversation for testing (status line when there are none).
func startFirstAreaDialog(g *core.GameState) {
	if len(g.Area.Dialogs) == 0 {
		g.SetStatusMessage("No dialogs authored in this area.")
		return
	}
	g.DebugMenuOpen = false
	core.StartDialog(g, g.Area.Dialogs[0].ID)
}

// updateDialogModal drives the conversation overlay: Back skips it; with choices
// Up/Down move the cursor and Confirm selects; on a no-choice node Confirm
// continues / ends. core/dialog.go owns navigation + end-action firing.
func updateDialogModal(g *core.GameState) {
	if !g.DialogOpen {
		return
	}
	// No resolvable current node means the graph is broken; close out.
	if _, ok := core.CurrentDialogNode(g); !ok {
		core.CloseDialog(g)
		return
	}

	if input.BackPressed() {
		core.CloseDialog(g)
		return
	}

	views := core.DialogChoiceViews(g)
	if len(views) == 0 {
		// No-choice node: Confirm advances / ends.
		if input.ConfirmPressed() {
			audio.Play(audio.SoundInputHit)
			core.ContinueDialog(g)
		}
		return
	}

	core.ClampDialogCursor(g) // backstop for a stale cursor from a prior node
	// MoveDialogCursor SKIPS disabled (greyed) rows, never parking on a no-op.
	switch {
	case input.UpPressed():
		core.MoveDialogCursor(g, -1)
	case input.DownPressed():
		core.MoveDialogCursor(g, 1)
	}
	if input.ConfirmPressed() {
		// Act only on an in-range, ENABLED choice (a greyed one is a no-op).
		if g.Dialog.ChoiceCursor >= 0 && g.Dialog.ChoiceCursor < len(views) && !views[g.Dialog.ChoiceCursor].Disabled {
			audio.Play(audio.SoundInputHit)
			core.SelectDialogChoice(g, g.Dialog.ChoiceCursor)
		}
	}
}
