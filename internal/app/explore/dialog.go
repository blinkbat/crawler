package explore

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
)

// startFirstAreaDialog is the Debug ▸ Start Dialog launcher: it opens the
// current area's first authored conversation so the overlay can be tested
// without an in-world trigger. Closes the debug submenu on success; reports a
// status line when the area has no dialogs.
func startFirstAreaDialog(g *core.GameState) {
	if len(g.Area.Dialogs) == 0 {
		g.SetStatusMessage("No dialogs authored in this area.")
		return
	}
	g.DebugMenuOpen = false
	core.StartDialog(g, g.Area.Dialogs[0].ID)
}

// updateDialogModal drives the branching-conversation overlay. Gamepad-first:
// Back skips the whole conversation; on a node WITH choices Up/Down move the
// choice cursor and Confirm selects; on a no-choice node Confirm continues to
// the next line (or ends). The runtime state machine (core/dialog.go) owns
// the actual navigation + end-action firing — this layer only translates
// input into those calls.
func updateDialogModal(g *core.GameState) {
	if !g.DialogOpen {
		return
	}
	// A node with no resolvable current node shouldn't strand the player —
	// CurrentDialogNode failing means the graph is broken; close out.
	if _, ok := core.CurrentDialogNode(g); !ok {
		core.CloseDialog(g)
		return
	}

	// Back skips the conversation entirely (the bg2 "Esc to skip dialog").
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

	core.ClampDialogCursor(g) // backstop in case a prior node left a stale cursor
	// Step by direction through MoveDialogCursor so the cursor SKIPS disabled
	// (greyed, un-confirmable) rows instead of parking on one where Confirm is a
	// silent no-op and the renderer shows no focus highlight.
	switch {
	case input.UpPressed():
		core.MoveDialogCursor(g, -1)
	case input.DownPressed():
		core.MoveDialogCursor(g, 1)
	}
	if input.ConfirmPressed() {
		// Only act on an in-range, ENABLED choice — don't rely on
		// SelectDialogChoice to silently ignore a disabled / out-of-range pick.
		// A Confirm on a greyed-out choice is a no-op (no sound, no select).
		if g.Dialog.ChoiceCursor >= 0 && g.Dialog.ChoiceCursor < len(views) && !views[g.Dialog.ChoiceCursor].Disabled {
			audio.Play(audio.SoundInputHit)
			core.SelectDialogChoice(g, g.Dialog.ChoiceCursor)
		}
	}
}
