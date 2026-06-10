package editor

import (
	"crawler/internal/app/audio"
	"crawler/internal/app/core"
	"crawler/internal/app/input"
	"crawler/internal/app/render"
	"fmt"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// soundPanel identifies one column of the sound-creator modal. The cursor
// moves between columns with Tab (wrapping mod soundPanelCount); each
// column owns its own keyboard handler and set of click targets. Named so
// the column dispatch reads by intent instead of bare 0/1/2 and the wrap
// can't drift from a hardcoded "% 3".
type soundPanel int

const (
	soundPanelParams soundPanel = iota // synth-param sliders + name + actions
	soundPanelList                     // saved-sound list (Play/Delete rows)
	soundPanelAssign                   // built-in cue assignments
	soundPanelCount
)

// soundParamSet captures the tunables for a single sweep-style cue.
// Bounded ranges are documented per field; the sliders in the sound
// modal map their 0..1 normalized position into these ranges via
// soundParamSliders below. Defaults match a generic short input blip so
// the modal opens with something audible on first preview.
type soundParamSet struct {
	Duration     float64 // seconds, [0.03, 0.40]
	StartHz      float64 // Hz, [100, 1600]
	EndHz        float64 // Hz, [100, 1600]
	Volume       float64 // [0, 0.6]
	Attack       float64 // seconds, [0, 0.05]
	Release      float64 // seconds, [0, 0.30]
	Wave         int     // 0..audio.WaveShapeCount-1 — index into audio.WaveX (Sine/Square/Triangle/Saw)
	Noise        float64 // [0, 1] — noise mix amount
	VibratoHz    float64 // [0, 15] — vibrato wobble rate
	VibratoDepth float64 // [0, 0.5] — vibrato swing depth as a fraction of base Hz
}

// soundParamSliders describes the sound modal's slider column — one
// sliderField (slider.go) row per synth parameter. The cursor walks this
// slice; each row reads/writes its named field on the soundParamSet via
// the getter/setter callbacks.
var soundParamSliders = []sliderField[soundParamSet]{
	{
		Label: "Duration", Min: 0.03, Max: 0.40, Step: 0.01, Format: "%.2fs",
		Get: func(p *soundParamSet) float64 { return p.Duration },
		Set: func(p *soundParamSet, v float64) { p.Duration = v },
	},
	{
		Label: "Start Hz", Min: 100, Max: 1600, Step: 20, Format: "%.0f Hz",
		Get: func(p *soundParamSet) float64 { return p.StartHz },
		Set: func(p *soundParamSet, v float64) { p.StartHz = v },
	},
	{
		Label: "End Hz", Min: 100, Max: 1600, Step: 20, Format: "%.0f Hz",
		Get: func(p *soundParamSet) float64 { return p.EndHz },
		Set: func(p *soundParamSet, v float64) { p.EndHz = v },
	},
	{
		Label: "Volume", Min: 0.0, Max: 0.6, Step: 0.02, Format: "%.2f",
		Get: func(p *soundParamSet) float64 { return p.Volume },
		Set: func(p *soundParamSet, v float64) { p.Volume = v },
	},
	{
		Label: "Attack", Min: 0.0, Max: 0.05, Step: 0.005, Format: "%.3fs",
		Get: func(p *soundParamSet) float64 { return p.Attack },
		Set: func(p *soundParamSet, v float64) { p.Attack = v },
	},
	{
		Label: "Release", Min: 0.0, Max: 0.30, Step: 0.01, Format: "%.2fs",
		Get: func(p *soundParamSet) float64 { return p.Release },
		Set: func(p *soundParamSet, v float64) { p.Release = v },
	},
	{
		// Wave shape — discrete toggle dressed up as a slider. Step=1 +
		// Min/Max=0..WaveShapeCount-1 means Left/Right cycles through the
		// WaveX values one at a time, and the mouse-drag mapping rounds to
		// the nearest integer. Bounds derive from audio.WaveShapeCount so a
		// fifth shape becomes reachable without editing this row. Display
		// returns the human label so the readout shows "Sine" instead of "0".
		Label: "Wave", Min: 0, Max: float64(audio.WaveShapeCount - 1), Step: 1, Format: "%.0f",
		Get: func(p *soundParamSet) float64 { return float64(p.Wave) },
		Set: func(p *soundParamSet, v float64) {
			i := int(v + 0.5)
			if i < 0 {
				i = 0
			}
			if i > audio.WaveShapeCount-1 {
				i = audio.WaveShapeCount - 1
			}
			p.Wave = i
		},
		Display: func(v float64) string {
			return audio.WaveShapeName(audio.WaveShape(int(v + 0.5)))
		},
	},
	{
		Label: "Noise", Min: 0.0, Max: 1.0, Step: 0.05, Format: "%.2f",
		Get: func(p *soundParamSet) float64 { return p.Noise },
		Set: func(p *soundParamSet, v float64) { p.Noise = v },
	},
	{
		Label: "Vibrato Hz", Min: 0.0, Max: 15.0, Step: 0.5, Format: "%.1f Hz",
		Get: func(p *soundParamSet) float64 { return p.VibratoHz },
		Set: func(p *soundParamSet, v float64) { p.VibratoHz = v },
	},
	{
		Label: "Vibrato Depth", Min: 0.0, Max: 0.5, Step: 0.02, Format: "%.2f",
		Get: func(p *soundParamSet) float64 { return p.VibratoDepth },
		Set: func(p *soundParamSet, v float64) { p.VibratoDepth = v },
	},
}

// soundParamDefaults seeds the modal with a sane starter cue — a short
// rising blip at modest volume. Audible on first preview so the author
// gets feedback without first tuning anything. All 10 fields are listed
// explicitly so a future enum reordering (e.g., WaveSine moving off
// iota 0) doesn't silently shift the default timbre — the literal
// expresses intent.
func soundParamDefaults() soundParamSet {
	return soundParamSet{
		Duration:     0.10,
		StartHz:      440,
		EndHz:        660,
		Volume:       0.22,
		Attack:       0.005,
		Release:      0.04,
		Wave:         int(audio.WaveSine),
		Noise:        0,
		VibratoHz:    0,
		VibratoDepth: 0,
	}
}

// Sound modal layout constants. Sized larger than the previous pass so
// the labels are readable on a 1080p display without leaning in. Modal
// occupies a 900×560 card; three columns plus a hint footer.
//
// Three "body" font sizes (label / value / list) all collapse to one
// soundFontBody — they had drifted to the same 16pt value, so the
// distinction was misleading. Hint stays smaller for the footer; the
// SOUNDS heading is drawn through render.DrawHeading (no local size).
const (
	soundModalW     = float32(900)
	soundModalH     = float32(560)
	soundFontBody   = float32(16)
	soundFontHint   = float32(13)
	soundRowH       = float32(34) // slider row height
	soundListRowH   = float32(30)
	soundAssignRowH = float32(48) // two-line cue row (name + assigned)
	soundColGap     = float32(14)
	soundButtonH    = float32(32)
)

// assignableCueList is the fixed list of built-in cues the assignments
// column iterates. Cached as a package-level slice so the draw loop
// doesn't allocate a fresh slice every frame.
var assignableCueList = func() []audio.Sound {
	count := audio.SoundCount()
	out := make([]audio.Sound, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, audio.Sound(i))
	}
	return out
}()

// soundListRowRect bundles the row + per-row buttons for one saved
// sound. Replaces three parallel []rl.Rectangle slices that had to be
// kept in lockstep — now drift between row count and button count is
// impossible.
type soundListRowRect struct {
	Row    rl.Rectangle
	Play   rl.Rectangle
	Delete rl.Rectangle
}

// soundAssignRowRect bundles the row + per-row buttons for one built-in
// cue assignment. Same rationale as soundListRowRect.
type soundAssignRowRect struct {
	Row        rl.Rectangle
	Play       rl.Rectangle
	CycleLeft  rl.Rectangle
	CycleRight rl.Rectangle
}

// soundLayout caches the per-frame screen rectangles for every clickable
// element in the modal. Rebuilt every Update/Draw pair so resizes
// (windowed↔fullscreen toggle, layout knob changes) take effect on the
// next frame. The Update path reads these for hit-testing; the Draw
// path reads them to paint at the same positions.
type soundLayout struct {
	card         rl.Rectangle
	paramsCol    rl.Rectangle
	listCol      rl.Rectangle
	assignCol    rl.Rectangle
	sliderTracks []rl.Rectangle // per-slider clickable track; len == len(soundParamSliders)
	nameField    rl.Rectangle
	previewBtn   rl.Rectangle
	saveBtn      rl.Rectangle
	listRows     []soundListRowRect   // one entry per saved-sound row (rects only filled for the visible window)
	assignRows   []soundAssignRowRect // one entry per built-in cue row (rects only filled for the visible window)
	// listTopRow..listEnd is the saved-sounds scroll window (via scrollWindow)
	// so a long list can't overflow the column off the card bottom.
	listTopRow int
	listEnd    int
	// assignTopRow..assignEnd is the same scroll window for the cue
	// assignments column — 6 cues fit today, but the cue list grows with
	// the game and overflow rows would otherwise be unreachable.
	assignTopRow int
	assignEnd    int
}

// computeSoundLayout assembles the layout rectangles given the current
// screen size and the cached lists (saved sounds, cues). Pure function
// of inputs — both Update and Draw call this so they agree on
// hit-test geometry.
// soundListCursor is the saved-sounds row the scroll window should keep
// visible: the live cursor when the list panel is focused, else row 0 (so
// editing the params/assign columns doesn't yank the list to an unrelated row).
func soundListCursor(s *State) int {
	if s.soundLeftPanel == soundPanelList {
		return s.soundCursor
	}
	return 0
}

// soundAssignCursor mirrors soundListCursor for the assignments column.
func soundAssignCursor(s *State) int {
	if s.soundLeftPanel == soundPanelAssign {
		return s.soundCursor
	}
	return 0
}

func computeSoundLayout(savedSounds []string, listCursor, assignCursor int) soundLayout {
	card := centeredCardRect(soundModalW, soundModalH)
	colW := (modalContentWidth(card) - 2*soundColGap) / 3
	colY := card.Y + 56
	colH := card.Height - 110

	paramsCol := rl.NewRectangle(card.X+modalContentInset, colY, colW, colH)
	listCol := rl.NewRectangle(paramsCol.X+colW+soundColGap, colY, colW, colH)
	assignCol := rl.NewRectangle(listCol.X+colW+soundColGap, colY, colW, colH)

	l := soundLayout{card: card, paramsCol: paramsCol, listCol: listCol, assignCol: assignCol}

	// Params column rows. sliderTracks is sized off soundParamSliders so
	// adding a new slider doesn't require touching the layout struct.
	x := paramsCol.X + 12
	w := paramsCol.Width - 24
	y := paramsCol.Y + 30
	l.sliderTracks = make([]rl.Rectangle, len(soundParamSliders))
	for i := range soundParamSliders {
		l.sliderTracks[i] = rl.NewRectangle(x+90, y+8, w-180, 14)
		y += soundRowH
	}
	l.nameField = rl.NewRectangle(x+70, y, w-70, 28)
	y += soundRowH + 6
	l.previewBtn = rl.NewRectangle(x, y, (w-12)/2, soundButtonH)
	l.saveBtn = rl.NewRectangle(x+(w-12)/2+12, y, (w-12)/2, soundButtonH)

	// List column rows + per-row Play/× buttons. Only the visible window gets
	// real rects; off-window entries stay zero (so they neither draw nor
	// hit-test). The window keeps listCursor on screen.
	lx := listCol.X + 12
	lw := listCol.Width - 24
	ly := listCol.Y + 36
	listAreaH := listCol.Y + listCol.Height - 12 - ly
	maxListRows := int(listAreaH / soundListRowH)
	if maxListRows < 1 {
		maxListRows = 1
	}
	l.listTopRow, l.listEnd = scrollWindow(listCursor, len(savedSounds), maxListRows)
	l.listRows = make([]soundListRowRect, len(savedSounds))
	for i := l.listTopRow; i < l.listEnd; i++ {
		row := rl.NewRectangle(lx, ly+float32(i-l.listTopRow)*soundListRowH, lw, soundListRowH-4)
		l.listRows[i] = soundListRowRect{
			Row:    row,
			Play:   rl.NewRectangle(row.X+row.Width-78, row.Y+2, 32, row.Height-4),
			Delete: rl.NewRectangle(row.X+row.Width-40, row.Y+2, 32, row.Height-4),
		}
	}

	// Assignments column. Same visible-window scheme as the saved-sounds
	// list: only on-window cue rows get real rects, so a cue list that
	// outgrows the column scrolls instead of overflowing off the card.
	ax := assignCol.X + 12
	aw := assignCol.Width - 24
	ay := assignCol.Y + 36
	assignAreaH := assignCol.Y + assignCol.Height - 12 - ay
	maxAssignRows := int(assignAreaH / soundAssignRowH)
	if maxAssignRows < 1 {
		maxAssignRows = 1
	}
	l.assignTopRow, l.assignEnd = scrollWindow(assignCursor, len(assignableCueList), maxAssignRows)
	l.assignRows = make([]soundAssignRowRect, len(assignableCueList))
	for i := l.assignTopRow; i < l.assignEnd; i++ {
		row := rl.NewRectangle(ax, ay+float32(i-l.assignTopRow)*soundAssignRowH, aw, soundAssignRowH-4)
		l.assignRows[i] = soundAssignRowRect{
			Row:        row,
			Play:       rl.NewRectangle(row.X+row.Width-126, row.Y+12, 32, 24),
			CycleLeft:  rl.NewRectangle(row.X+row.Width-90, row.Y+12, 24, 24),
			CycleRight: rl.NewRectangle(row.X+row.Width-58, row.Y+12, 24, 24),
		}
	}

	return l
}

// soundDragState is set while the mouse is held on a slider track —
// motion updates the slider's value continuously. Released when the
// mouse button releases. Index < 0 means "no active drag."
type soundDragState struct {
	sliderIdx int
}

var soundDrag = soundDragState{sliderIdx: -1}

// openSoundsModal seeds the sound modal's defaults if this is the
// first time and switches state. Called by the Sounds topbar button.
func openSoundsModal(s *State) {
	if s.soundParams.Duration == 0 {
		s.soundParams = soundParamDefaults()
	}
	if s.soundName == "" {
		s.soundName = "new_sound"
	}
	s.modal = modalSounds
	s.soundCursor = 0
	s.soundLeftPanel = soundPanelParams
	// Seed the disk-backed caches once on open; they're refreshed only on
	// mutation thereafter, never per frame.
	refreshSoundCaches(s)
	// Reset any leftover drag from a prior session — a stale sliderIdx
	// would let the next mouse drag pop a different slider.
	soundDrag.sliderIdx = -1
}

// refreshSoundCaches re-reads the saved-sounds directory listing and the
// cue-assignment map from disk into State. The sound modal only mutates these
// through save / delete / assign actions in this package, so refreshing here
// (on open + after each mutation) keeps the per-frame draw/update off the disk
// entirely — previously the list ReadDir ran every Update frame and the
// assignment file was read+parsed once per cue every Draw frame.
func refreshSoundCaches(s *State) {
	s.soundSavedCache = audio.ListUserSounds()
	s.soundAssignCache = audio.AllAssignments()
}

// updateSoundsModal handles the sound modal's input. Mouse-first now:
// click sliders/buttons/list rows directly, keyboard remains as a
// secondary path (Tab between columns, arrows for fine adjustment, Esc
// to close).
func updateSoundsModal(s *State) Action {
	if editorCancelPressed() {
		closeModal(s)
		return ActionNone
	}
	// Read the saved-sounds list from the cache (populated on open + after
	// each save/delete via refreshSoundCaches) — no per-frame os.ReadDir.
	savedSounds := s.soundSavedCache
	layout := computeSoundLayout(savedSounds, soundListCursor(s), soundAssignCursor(s))

	mp := rl.GetMousePosition()
	mouseDown := rl.IsMouseButtonDown(rl.MouseLeftButton)
	mousePressed := rl.IsMouseButtonPressed(rl.MouseLeftButton)
	mouseReleased := rl.IsMouseButtonReleased(rl.MouseLeftButton)

	// Active slider drag: while held, map the mouse X within the track
	// to a value in the slider's range. Snap to the slider's Step grain
	// so dragging produces clean readouts instead of fractional noise.
	if soundDrag.sliderIdx >= 0 {
		if !mouseDown {
			soundDrag.sliderIdx = -1
		} else if soundDrag.sliderIdx < len(soundParamSliders) {
			info := soundParamSliders[soundDrag.sliderIdx]
			track := layout.sliderTracks[soundDrag.sliderIdx]
			info.Set(&s.soundParams, sliderSnap(info.Min, info.Max, info.Step, track.X, track.Width, mp.X))
			s.soundLeftPanel = soundPanelParams
			s.soundCursor = soundDrag.sliderIdx
		}
	}

	if mousePressed && pointIn(mp, layout.card) {
		s.soundLeftPanel, s.soundCursor = handleSoundMouseClick(s, mp, &layout, savedSounds)
	}
	if mouseReleased {
		soundDrag.sliderIdx = -1
	}

	// Name field typing: only while the focus is on the name row of the
	// params column. The user has to click the field (which sets cursor
	// to the name row) before keystrokes register — that fixes the old
	// "Space types into the name" trap.
	if s.soundLeftPanel == soundPanelParams && s.soundCursor == soundNameCursorIdx() {
		// No-space filter so the user can hit Space for Preview without
		// also typing a space into the sound name. Shared pump from
		// input.go — backspace handled there too.
		pumpPrintableASCII(&s.soundName, 32, acceptPrintableNoSpace, nil)
	}

	// Keyboard fallbacks.
	if editorTabPressed() {
		s.soundLeftPanel = core.WrapEnum(s.soundLeftPanel, 1, int(soundPanelCount))
		s.soundCursor = 0
		return ActionNone
	}
	switch s.soundLeftPanel {
	case soundPanelParams:
		updateSoundsParamsKeys(s)
	case soundPanelList:
		updateSoundsListKeys(s, savedSounds)
	case soundPanelAssign:
		updateSoundsAssignKeys(s)
	default:
		panic("editor: updateSoundsModal missing key handler for soundPanel")
	}
	return ActionNone
}

// soundNameCursorIdx returns the cursor row that represents the name
// field in the params column. Anything past the last slider but before
// the Preview/Save action row.
func soundNameCursorIdx() int   { return len(soundParamSliders) }
func soundActionCursorIdx() int { return len(soundParamSliders) + 1 }

// handleSoundMouseClick dispatches a left-mouse-down inside the modal.
// Returns the new (leftPanel, cursor) so the caller can use them to
// drive subsequent draws and keyboard input. Returning early without
// changing the cursor means "click was on a non-cursor target."
func handleSoundMouseClick(s *State, mp rl.Vector2, l *soundLayout, savedSounds []string) (soundPanel, int) {
	// Slider tracks — start a drag and immediately set the value at the
	// click position so single-click also adjusts (not just drag).
	for i := range soundParamSliders {
		if pointIn(mp, l.sliderTracks[i]) {
			soundDrag.sliderIdx = i
			return soundPanelParams, i
		}
	}
	// Name field — sets focus so keystrokes type into the name.
	if pointIn(mp, l.nameField) {
		return soundPanelParams, soundNameCursorIdx()
	}
	// Preview / Save action buttons.
	if pointIn(mp, l.previewBtn) {
		previewSoundParams(s.soundParams)
		return soundPanelParams, soundActionCursorIdx()
	}
	if pointIn(mp, l.saveBtn) {
		saveCurrentSound(s)
		return soundPanelParams, soundActionCursorIdx()
	}
	// Saved-sounds list — clickable rows + per-row Play/× buttons (visible window only).
	for i := l.listTopRow; i < l.listEnd; i++ {
		r := l.listRows[i]
		if pointIn(mp, r.Play) {
			audio.PreviewFile(savedSounds[i])
			return soundPanelList, i
		}
		if pointIn(mp, r.Delete) {
			confirmSoundDelete(s, savedSounds[i])
			return soundPanelList, i
		}
		if pointIn(mp, r.Row) {
			return soundPanelList, i
		}
	}
	// Assignments column (visible window only — off-window rects are zero).
	for i := l.assignTopRow; i < l.assignEnd; i++ {
		r := l.assignRows[i]
		if pointIn(mp, r.Play) {
			audio.Play(assignableCueList[i])
			return soundPanelAssign, i
		}
		if pointIn(mp, r.CycleLeft) {
			cycleCueAssignment(s, assignableCueList[i], -1)
			return soundPanelAssign, i
		}
		if pointIn(mp, r.CycleRight) {
			cycleCueAssignment(s, assignableCueList[i], +1)
			return soundPanelAssign, i
		}
		if pointIn(mp, r.Row) {
			return soundPanelAssign, i
		}
	}
	return s.soundLeftPanel, s.soundCursor
}

// updateSoundsParamsKeys drives the synth-params column via keyboard:
// arrows move the cursor + adjust sliders. Space previews. Enter saves
// (cursor on the action row) or types nothing into the name (handled
// elsewhere).
func updateSoundsParamsKeys(s *State) {
	rowCount := len(soundParamSliders) + 2 // sliders + name + actions
	if s.soundCursor == soundNameCursorIdx() {
		// On the Name row every printable key (incl. W/S) feeds the name
		// buffer, so navigate off it with arrow/pad-only up-down — otherwise
		// typing a name containing those letters also scrolls the cursor away
		// (and once off the name row, keystrokes stop reaching the buffer).
		s.soundCursor = input.CursorUpDownTextSafe(s.soundCursor, rowCount)
	} else {
		s.soundCursor = input.CursorUpDown(s.soundCursor, rowCount)
	}
	if s.soundCursor < len(soundParamSliders) {
		slider := soundParamSliders[s.soundCursor]
		if delta := input.CursorLeftRight(); delta != 0 {
			v := slider.Get(&s.soundParams) + float64(delta)*slider.Step
			slider.Set(&s.soundParams, clampRange(v, slider.Min, slider.Max))
		}
		if rl.IsKeyPressed(rl.KeySpace) {
			previewSoundParams(s.soundParams)
		}
		return
	}
	if s.soundCursor == soundActionCursorIdx() {
		if rl.IsKeyPressed(rl.KeySpace) {
			previewSoundParams(s.soundParams)
		}
		if editorCommitPressed() {
			saveCurrentSound(s)
		}
	}
	// On the Name row: keystrokes feed into the name buffer (handled by
	// updateSoundsModal); Enter saves.
	if s.soundCursor == soundNameCursorIdx() && editorCommitPressed() {
		saveCurrentSound(s)
	}
}

func updateSoundsListKeys(s *State, names []string) {
	if len(names) == 0 {
		return
	}
	if s.soundCursor >= len(names) {
		s.soundCursor = len(names) - 1
	}
	s.soundCursor = input.CursorUpDown(s.soundCursor, len(names))
	if editorCommitPressed() || rl.IsKeyPressed(rl.KeySpace) {
		audio.PreviewFile(names[s.soundCursor])
	}
	if rl.IsKeyPressed(rl.KeyX) {
		confirmSoundDelete(s, names[s.soundCursor])
	}
}

// confirmSoundDelete is a two-press guard on the irreversible saved-sound
// delete: the first request arms `name` (and flashes a confirm prompt);
// the second request for the SAME name performs the on-disk delete. A
// different name re-arms instead of deleting, so a single misclick on the
// × never destroys a .wav.
func confirmSoundDelete(s *State, name string) {
	if s.soundDeleteArmed != name {
		s.soundDeleteArmed = name
		s.flash("Delete " + name + "? Click × again (or press X) to confirm")
		return
	}
	s.soundDeleteArmed = ""
	if err := audio.DeleteUserSound(name); err != nil {
		s.flash("Delete failed: " + err.Error())
	} else {
		s.flash("Deleted " + name)
		refreshSoundCaches(s)
	}
}

func updateSoundsAssignKeys(s *State) {
	cues := assignableCueList
	if s.soundCursor < 0 {
		s.soundCursor = 0
	}
	if s.soundCursor >= len(cues) {
		s.soundCursor = len(cues) - 1
	}
	s.soundCursor = input.CursorUpDown(s.soundCursor, len(cues))
	cue := cues[s.soundCursor]
	if delta := input.CursorLeftRight(); delta != 0 {
		cycleCueAssignment(s, cue, delta)
	}
	if editorCommitPressed() || rl.IsKeyPressed(rl.KeySpace) {
		audio.Play(cue)
	}
}

// cycleCueAssignment advances the cue's assignment through:
//
//	"(default)" → user_sound_1 → user_sound_2 → … → "(default)" → …
//
// in the user-sounds list's sorted order. The visible label in the UI
// is fed by audio.CurrentAssignment.
func cycleCueAssignment(s *State, cue audio.Sound, delta int) {
	options := []string{""} // first slot = revert-to-default
	options = append(options, audio.ListUserSounds()...)
	current := audio.CurrentAssignment(cue)
	idx := 0
	for i, opt := range options {
		if opt == current {
			idx = i
			break
		}
	}
	idx = core.WrapIndex(idx+delta, len(options))
	failed, err := audio.AssignUserSound(cue, options[idx])
	if err != nil {
		s.flash("Assign failed: " + err.Error())
		return
	}
	if len(failed) > 0 {
		s.flash("Saved assignment but reload failed for: " + strings.Join(failed, ", "))
	}
	refreshSoundCaches(s)
}

// previewSoundParams synthesizes the current slider settings into PCM
// and plays it through audio's in-memory preview ring — no disk write,
// so the saved-sounds list isn't polluted and a user who saves a real
// cue named "preview" doesn't clash.
func previewSoundParams(p soundParamSet) {
	pcm := audio.SynthShape(p.Duration, p.StartHz, p.EndHz, p.Volume, p.Attack, p.Release,
		audio.WaveShape(p.Wave), p.Noise, p.VibratoHz, p.VibratoDepth)
	audio.PreviewPCM(pcm)
}

// saveCurrentSound writes the slider state to maps/sounds/<name>.wav and
// flashes a status message. audio.SaveUserSound now sanitizes the name
// itself and returns the final on-disk form, so the editor reports
// exactly what landed on disk (handles a user typing "My Cue!" → file
// becomes "my_cue.wav").
func saveCurrentSound(s *State) {
	if strings.TrimSpace(s.soundName) == "" {
		s.flash("Sound name required")
		return
	}
	pcm := audio.SynthShape(s.soundParams.Duration, s.soundParams.StartHz, s.soundParams.EndHz,
		s.soundParams.Volume, s.soundParams.Attack, s.soundParams.Release,
		audio.WaveShape(s.soundParams.Wave), s.soundParams.Noise,
		s.soundParams.VibratoHz, s.soundParams.VibratoDepth)
	saved, err := audio.SaveUserSound(s.soundName, pcm)
	if err != nil {
		s.flash("Save failed: " + err.Error())
		return
	}
	s.soundName = saved
	s.flash("Saved " + saved + audio.WavExt)
	refreshSoundCaches(s)
}

// drawSoundsModal renders the three-column sound editor with mouse-first
// affordances: clickable slider tracks, name field, action buttons, and
// per-row Play/×/cycle controls. Keyboard focus still highlights the
// active column + cursor.
func drawSoundsModal(s *State, font rl.Font, theme render.Theme) {
	// Read the cached saved-sounds list that updateSoundsModal already
	// populated this frame — no second ReadDir per frame. Fall back to
	// a fresh read if the cache is nil (defensive; would mean Draw fired
	// without a prior Update, which shouldn't happen for a modal).
	savedSounds := s.soundSavedCache
	if savedSounds == nil {
		savedSounds = audio.ListUserSounds()
	}
	l := computeSoundLayout(savedSounds, soundListCursor(s), soundAssignCursor(s))
	// Both computeSoundLayout and drawModalHeader center the card through
	// the shared centeredCardRect, so the rect drawModalHeader paints is
	// identical to l.card — visual + hit-test stay in sync without
	// duplicating the card-draw call.
	_ = drawModalHeader(font, theme, soundModalW, soundModalH, "SOUNDS", theme.BorderActive)

	drawSoundsParamsCol(s, font, theme, &l)
	drawSoundsListCol(s, font, theme, &l, savedSounds)
	drawSoundsAssignCol(s, font, theme, &l)

	hint := "Click sliders to drag   Click Play/Save/X/Prev/Next   Tab cycle column   Esc close"
	rl.DrawTextEx(font, hint,
		rl.NewVector2(l.card.X+18, l.card.Y+l.card.Height-26),
		soundFontHint, 1, theme.TextHint)
}

func drawSoundsParamsCol(s *State, font rl.Font, theme render.Theme, l *soundLayout) {
	drawSoundsColumnFrame(theme, l.paramsCol, s.soundLeftPanel == soundPanelParams)
	render.DrawSubHeading(font, "Synth params", l.paramsCol.X+12, l.paramsCol.Y+8, theme.BorderActive)

	for i, slider := range soundParamSliders {
		focused := s.soundLeftPanel == soundPanelParams && s.soundCursor == i
		drawSoundsSlider(font, theme, l.paramsCol.X+12, l.sliderTracks[i].Y-8, l.paramsCol.Width-24, slider, s.soundParams, l.sliderTracks[i], focused)
	}
	// Name field. The "Name" label sits to the left of the text field;
	// drawTextField paints the field itself with the shared editor palette.
	nameFocused := s.soundLeftPanel == soundPanelParams && s.soundCursor == soundNameCursorIdx()
	nameLabelCol := theme.TextMuted
	if nameFocused {
		nameLabelCol = theme.BorderActive
	}
	rl.DrawTextEx(font, "Name", rl.NewVector2(l.paramsCol.X+12, l.nameField.Y+6), soundFontBody, 1, nameLabelCol)
	drawTextField(font, l.nameField, s.soundName, nameFocused)
	// Action buttons.
	actionFocused := s.soundLeftPanel == soundPanelParams && s.soundCursor == soundActionCursorIdx()
	drawButton(font, l.previewBtn, "Preview (Space)", actionFocused)
	drawButton(font, l.saveBtn, "Save", actionFocused)
}

func drawSoundsSlider(font rl.Font, theme render.Theme, x, y, w float32, info sliderField[soundParamSet], p soundParamSet, track rl.Rectangle, focused bool) {
	// Numeric readout to the right of the track. Display callback
	// overrides the fmt.Sprintf path for rows that render a label
	// instead of a number (the Wave row's "Sine"/"Square"/etc.).
	value := info.Get(&p)
	var val string
	if info.Display != nil {
		val = info.Display(value)
	} else {
		val = fmt.Sprintf(info.Format, value)
	}
	drawSlider(font, theme, info.Label, val, value, info.Min, info.Max,
		rl.NewVector2(x, y), rl.NewVector2(x+w-78, y),
		soundFontBody, track, 7, focused)
}

func drawSoundsListCol(s *State, font rl.Font, theme render.Theme, l *soundLayout, names []string) {
	drawSoundsColumnFrame(theme, l.listCol, s.soundLeftPanel == soundPanelList)
	render.DrawSubHeading(font, "Saved sounds", l.listCol.X+12, l.listCol.Y+8, theme.BorderActive)
	if len(names) == 0 {
		rl.DrawTextEx(font, "(no saved sounds yet)",
			rl.NewVector2(l.listCol.X+12, l.listCol.Y+44), soundFontBody, 1, theme.TextHint)
		rl.DrawTextEx(font, "Save one from the left column.",
			rl.NewVector2(l.listCol.X+12, l.listCol.Y+68), soundFontHint, 1, theme.TextHint)
		return
	}
	for i := l.listTopRow; i < l.listEnd; i++ {
		r := l.listRows[i]
		if s.soundLeftPanel == soundPanelList && s.soundCursor == i {
			render.DrawSelectedRow(r.Row)
		}
		render.DrawTextWithShadow(font, names[i],
			r.Row.X+8, r.Row.Y+6, soundFontBody, theme.TextMuted)
		drawButton(font, r.Play, ">", false)
		drawButton(font, r.Delete, "X", false)
	}
	if l.listTopRow > 0 {
		rl.DrawTextEx(font, "▲ more", rl.NewVector2(l.listCol.X+l.listCol.Width-70, l.listCol.Y+10), soundFontHint, 1, theme.TextHint)
	}
	if l.listEnd < len(names) {
		rl.DrawTextEx(font, "▼ more", rl.NewVector2(l.listCol.X+l.listCol.Width-70, l.listCol.Y+l.listCol.Height-20), soundFontHint, 1, theme.TextHint)
	}
}

func drawSoundsAssignCol(s *State, font rl.Font, theme render.Theme, l *soundLayout) {
	drawSoundsColumnFrame(theme, l.assignCol, s.soundLeftPanel == soundPanelAssign)
	render.DrawSubHeading(font, "Built-in cue assignments", l.assignCol.X+12, l.assignCol.Y+8, theme.BorderActive)
	for i := l.assignTopRow; i < l.assignEnd; i++ {
		cue := assignableCueList[i]
		r := l.assignRows[i]
		if s.soundLeftPanel == soundPanelAssign && s.soundCursor == i {
			render.DrawSelectedRow(r.Row)
		}
		render.DrawTextWithShadow(font, audio.SoundName(cue),
			r.Row.X+8, r.Row.Y+4, soundFontBody, theme.TextMuted)
		assigned := s.soundAssignCache[audio.SoundCanonicalName(cue)]
		assignedLabel := "(default)"
		if assigned != "" {
			assignedLabel = "→ " + assigned
		}
		rl.DrawTextEx(font, assignedLabel,
			rl.NewVector2(r.Row.X+8, r.Row.Y+24),
			soundFontHint, 1, theme.TextHint)
		drawButton(font, r.Play, ">", false)
		drawButton(font, r.CycleLeft, "<", false)
		drawButton(font, r.CycleRight, ">", false)
	}
	if l.assignTopRow > 0 {
		rl.DrawTextEx(font, "▲ more", rl.NewVector2(l.assignCol.X+l.assignCol.Width-70, l.assignCol.Y+10), soundFontHint, 1, theme.TextHint)
	}
	if l.assignEnd < len(assignableCueList) {
		rl.DrawTextEx(font, "▼ more", rl.NewVector2(l.assignCol.X+l.assignCol.Width-70, l.assignCol.Y+l.assignCol.Height-20), soundFontHint, 1, theme.TextHint)
	}
}

func drawSoundsColumnFrame(theme render.Theme, r rl.Rectangle, focused bool) {
	// SurfacePrimary at full alpha for the column background — slightly
	// darker than the modal card so the columns read as inset, matching
	// the editor's other panel-inside-panel surfaces (combat log inset,
	// metadata column).
	bg := theme.SurfacePrimary
	bg.A = 255
	rl.DrawRectangleRec(r, bg)
	border := theme.BorderDim
	if focused {
		border = theme.BorderActive
	}
	rl.DrawRectangleLinesEx(r, 1, border)
}
