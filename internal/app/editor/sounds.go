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

// soundPanel identifies one column of the sound-creator modal. Tab moves between
// columns (wrapping mod soundPanelCount); each owns its keyboard handler + clicks.
type soundPanel int

const (
	soundPanelParams soundPanel = iota // synth-param sliders + name + actions
	soundPanelList                     // saved-sound list (Play/Delete rows)
	soundPanelAssign                   // built-in cue assignments
	soundPanelCount
)

// soundParamSet is an alias of audio.ShapeParams so the editor, synth, and .snd
// sidecar share ONE param shape (lossless round-trip back into the sliders).
type soundParamSet = audio.ShapeParams

// soundSection groups consecutive soundParamSliders rows under a heading. Counts
// are asserted to sum to len(soundParamSliders) at init so an orphan row panics.
type soundSection struct {
	Title string
	Count int
}

var soundSections = []soundSection{
	{"Oscillator", 5}, // Wave, Start/End note, Pulse Width, Noise
	{"Envelope", 6},   // Duration, Attack, Decay, Sustain, Release, Volume
	{"FX", 7},         // Vibrato Hz/Depth, Tremolo Hz/Depth, Cutoff, Drive, Crush
}

// noteSlider builds a note-picker row over an Hz field: the stored value stays a
// frequency, but the control reads/writes tempered note indices ("A4 (440)").
func noteSlider(label string, get func(*soundParamSet) float64, set func(*soundParamSet, float64)) sliderField[soundParamSet] {
	return sliderField[soundParamSet]{
		Label: label, Min: 0, Max: float64(audio.NoteCount - 1), Step: 1, Format: "%.0f",
		Get: func(p *soundParamSet) float64 { return float64(audio.NearestNoteIndex(get(p))) },
		Set: func(p *soundParamSet, v float64) { set(p, audio.NoteHz(core.RoundToInt(v))) },
		Display: func(v float64) string {
			i := core.RoundToInt(v)
			return fmt.Sprintf("%s (%.0f)", audio.NoteName(i), audio.NoteHz(i))
		},
	}
}

// soundParamSliders: one sliderField row per synth param, in soundSections order.
var soundParamSliders = []sliderField[soundParamSet]{
	// — Oscillator —
	{
		// Wave shape — discrete toggle as a slider; bounds derive from
		// WaveShapeCount so a new shape is reachable without editing this row.
		Label: "Wave", Min: 0, Max: float64(audio.WaveShapeCount - 1), Step: 1, Format: "%.0f",
		Get: func(p *soundParamSet) float64 { return float64(p.Wave) },
		Set: func(p *soundParamSet, v float64) {
			i := core.Clamp(core.RoundToInt(v), 0, audio.WaveShapeCount-1)
			p.Wave = audio.WaveShape(i)
		},
		Display: func(v float64) string {
			return audio.WaveShapeName(audio.WaveShape(core.RoundToInt(v)))
		},
	},
	noteSlider("Start Note", func(p *soundParamSet) float64 { return p.StartHz }, func(p *soundParamSet, v float64) { p.StartHz = v }),
	noteSlider("End Note", func(p *soundParamSet) float64 { return p.EndHz }, func(p *soundParamSet, v float64) { p.EndHz = v }),
	{
		Label: "Pulse Width", Min: 0.05, Max: 0.95, Step: 0.05, Format: "%.2f",
		Get: func(p *soundParamSet) float64 { return p.PulseWidth },
		Set: func(p *soundParamSet, v float64) { p.PulseWidth = v },
	},
	{
		Label: "Noise", Min: 0.0, Max: 1.0, Step: 0.05, Format: "%.2f",
		Get: func(p *soundParamSet) float64 { return p.NoiseMix },
		Set: func(p *soundParamSet, v float64) { p.NoiseMix = v },
	},
	// — Envelope —
	{
		Label: "Duration", Min: 0.03, Max: 0.80, Step: 0.01, Format: "%.2fs",
		Get: func(p *soundParamSet) float64 { return p.Duration },
		Set: func(p *soundParamSet, v float64) { p.Duration = v },
	},
	{
		Label: "Attack", Min: 0.0, Max: 0.20, Step: 0.005, Format: "%.3fs",
		Get: func(p *soundParamSet) float64 { return p.Attack },
		Set: func(p *soundParamSet, v float64) { p.Attack = v },
	},
	{
		Label: "Decay", Min: 0.0, Max: 0.30, Step: 0.01, Format: "%.2fs",
		Get: func(p *soundParamSet) float64 { return p.Decay },
		Set: func(p *soundParamSet, v float64) { p.Decay = v },
	},
	{
		Label: "Sustain", Min: 0.0, Max: 1.0, Step: 0.05, Format: "%.2f",
		Get: func(p *soundParamSet) float64 { return p.Sustain },
		Set: func(p *soundParamSet, v float64) { p.Sustain = v },
	},
	{
		Label: "Release", Min: 0.0, Max: 0.40, Step: 0.01, Format: "%.2fs",
		Get: func(p *soundParamSet) float64 { return p.Release },
		Set: func(p *soundParamSet, v float64) { p.Release = v },
	},
	{
		Label: "Volume", Min: 0.0, Max: 0.6, Step: 0.02, Format: "%.2f",
		Get: func(p *soundParamSet) float64 { return p.Volume },
		Set: func(p *soundParamSet, v float64) { p.Volume = v },
	},
	// — FX —
	{
		Label: "Vibrato Hz", Min: 0.0, Max: 15.0, Step: 0.5, Format: "%.1f Hz",
		Get: func(p *soundParamSet) float64 { return p.VibHz },
		Set: func(p *soundParamSet, v float64) { p.VibHz = v },
	},
	{
		Label: "Vibrato Depth", Min: 0.0, Max: 0.5, Step: 0.02, Format: "%.2f",
		Get: func(p *soundParamSet) float64 { return p.VibDepth },
		Set: func(p *soundParamSet, v float64) { p.VibDepth = v },
	},
	{
		Label: "Tremolo Hz", Min: 0.0, Max: 20.0, Step: 0.5, Format: "%.1f Hz",
		Get: func(p *soundParamSet) float64 { return p.TremoloHz },
		Set: func(p *soundParamSet, v float64) { p.TremoloHz = v },
	},
	{
		Label: "Tremolo Depth", Min: 0.0, Max: 1.0, Step: 0.05, Format: "%.2f",
		Get: func(p *soundParamSet) float64 { return p.TremoloDepth },
		Set: func(p *soundParamSet, v float64) { p.TremoloDepth = v },
	},
	{
		// Cutoff — one-pole low-pass; 1.0 = fully open.
		Label: "Cutoff", Min: 0.0, Max: 1.0, Step: 0.05, Format: "%.2f",
		Get: func(p *soundParamSet) float64 { return p.Cutoff },
		Set: func(p *soundParamSet, v float64) { p.Cutoff = v },
	},
	{
		Label: "Drive", Min: 0.0, Max: 1.0, Step: 0.05, Format: "%.2f",
		Get: func(p *soundParamSet) float64 { return p.Drive },
		Set: func(p *soundParamSet, v float64) { p.Drive = v },
	},
	{
		Label: "Crush", Min: 0.0, Max: 1.0, Step: 0.05, Format: "%.2f",
		Get: func(p *soundParamSet) float64 { return p.Crush },
		Set: func(p *soundParamSet, v float64) { p.Crush = v },
	},
}

func init() {
	sum := 0
	for _, sec := range soundSections {
		sum += sec.Count
	}
	if sum != len(soundParamSliders) {
		panic("editor: soundSections counts must sum to len(soundParamSliders)")
	}
}

// soundParamDefaults seeds a short rising blip with every extra knob neutral.
// All fields listed explicitly so a field/enum reorder can't shift the timbre.
func soundParamDefaults() soundParamSet {
	return soundParamSet{
		Duration:     0.10,
		StartHz:      440, // A4
		EndHz:        660, // ~E5
		Volume:       0.22,
		Attack:       0.005,
		Decay:        0,
		Sustain:      1,
		Release:      0.04,
		Wave:         audio.WaveSine,
		PulseWidth:   0.5,
		NoiseMix:     0,
		VibHz:        0,
		VibDepth:     0,
		TremoloHz:    0,
		TremoloDepth: 0,
		Cutoff:       1, // filter open
		Drive:        0,
		Crush:        0,
	}
}

// Sound modal layout constants. Three columns plus a hint footer. Fonts alias
// the shared editor type scale (palette.go) so the modal can't drift off it.
const (
	soundModalW     = float32(960)
	soundModalH     = float32(600)
	soundFontBody   = editorFontBody
	soundFontHint   = editorFontAccent
	soundRowH       = float32(34) // slider row height
	soundListRowH   = float32(30)
	soundAssignRowH = float32(48) // two-line cue row (name + assigned)
	soundColGap     = float32(14)
	soundButtonH    = float32(32)
	// Scrollable params column: fixed sub-header band, then a scrolling body of
	// section headers (soundGroupHeaderH each) + slider rows under a fixed footer.
	paramsHeaderReserve = float32(30)
	soundGroupHeaderH   = float32(22)
	soundNameMaxLen     = 32 // sound-name field cap (pumped directly, not via textFieldConfigs)
	// Shared column geometry — all three columns (params/list/assign) inset their
	// content by these, so they can't drift apart. soundColInsetX is the left
	// inset (content width loses 2× it); the others are vertical bands.
	soundColInsetX       = float32(12)  // per-column left inset for content
	soundColHeaderOffset = float32(36)  // list/assign body top below the column rect
	soundColBottomMargin = float32(12)  // gap below the column body
	soundColTop          = float32(56)  // columns' top Y below the card top
	soundColHeightInset  = float32(110) // total height removed from the card for columns
	// Per-row action-button widths, shared by the saved-sounds list and the
	// assignments column so the two right-anchored button groups size identically.
	soundRowEditBtnW   = float32(38) // wider "Edit" label button
	soundRowCueBtnW    = float32(32) // square Play / Delete cue button
	soundRowAssignBtnW = float32(70) // "Assign ▼" cue-assignment picker button
)

// soundSliderMetrics is the sound creator's slider-row geometry — an implicit layout↔draw
// contract (the layout places the track, drawSoundsSlider places the label/value against the
// same x/w). Shares the sliderRowMetrics type with the Foe/Party visualizer (foeSliderMetrics).
// Track X = x+labelW; track W = w-trackReserve(2); value X = x+w-valueW.
var soundSliderMetrics = sliderRowMetrics{labelW: 96, valueW: 78, trackH: 14, thumbR: 7}

// assignableCueList is the fixed built-in cue list for the assignments column,
// cached package-level so the draw loop doesn't allocate per frame.
var assignableCueList = func() []audio.Sound {
	count := audio.SoundCount()
	out := make([]audio.Sound, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, audio.Sound(i))
	}
	return out
}()

// rowButtonSpec is one button in a right-anchored row group: width + gap to the
// next button (gapAfter ignored for the rightmost). Used by rightButtonRow.
type rowButtonSpec struct {
	w        float32
	gapAfter float32
}

// rightButtonRow lays the specs left-to-right as a block right-anchored inside
// `row` (rightmost edge at rightInset, each yOff down, height h). One placement
// formula for both the list and assignment button groups. Returns rects in order.
func rightButtonRow(row rl.Rectangle, yOff, h, rightInset float32, specs ...rowButtonSpec) []rl.Rectangle {
	var total float32
	for i, sp := range specs {
		total += sp.w
		if i < len(specs)-1 {
			total += sp.gapAfter
		}
	}
	x := row.X + row.Width - rightInset - total
	rects := make([]rl.Rectangle, len(specs))
	for i, sp := range specs {
		rects[i] = rl.NewRectangle(x, row.Y+yOff, sp.w, h)
		x += sp.w + sp.gapAfter
	}
	return rects
}

// soundListRowRect bundles the row + per-row buttons for one saved sound (vs.
// three parallel slices that had to stay in lockstep).
type soundListRowRect struct {
	Row    rl.Rectangle
	Edit   rl.Rectangle
	Play   rl.Rectangle
	Delete rl.Rectangle
}

// soundAssignRowRect bundles the row + per-row buttons for one cue assignment.
type soundAssignRowRect struct {
	Row    rl.Rectangle
	Play   rl.Rectangle
	Assign rl.Rectangle // opens the assignment dropdown (ddSoundAssign)
}

// soundLayout caches the per-frame rects for every clickable element. Rebuilt
// each Update/Draw pair (Update hit-tests, Draw paints at the same positions).
type soundLayout struct {
	card         rl.Rectangle
	paramsCol    rl.Rectangle
	listCol      rl.Rectangle
	assignCol    rl.Rectangle
	sliderTracks []rl.Rectangle // per-slider clickable track (scrolled); len == len(soundParamSliders)
	// paramsViewport is the clipped scroll region; sectionHeaderY[i] is the
	// scrolled screen Y of soundSections[i]'s heading; paramContentH is the body
	// height, paramMaxScroll the scroll ceiling.
	paramsViewport rl.Rectangle
	sectionHeaderY []float32
	paramContentH  float32
	paramMaxScroll float32
	nameField      rl.Rectangle
	previewBtn     rl.Rectangle
	saveBtn        rl.Rectangle
	listRows       []soundListRowRect   // per saved-sound row (rects filled only for the visible window)
	assignRows     []soundAssignRowRect // per cue row (rects filled only for the visible window)
	listTopRow     int                  // saved-sounds scroll window
	listEnd        int
	assignTopRow   int // cue-assignments scroll window
	assignEnd      int
}

// soundListCursor is the saved-sounds row the scroll window keeps visible: the
// live cursor when the list panel is focused, else row 0.
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

func computeSoundLayout(savedSounds []string, listCursor, assignCursor int, paramScroll float32) soundLayout {
	card := centeredCardRect(soundModalW, soundModalH)
	colW := (modalContentWidth(card) - 2*soundColGap) / 3
	colY := card.Y + soundColTop
	colH := card.Height - soundColHeightInset

	paramsCol := rl.NewRectangle(card.X+modalContentInset, colY, colW, colH)
	listCol := rl.NewRectangle(paramsCol.X+colW+soundColGap, colY, colW, colH)
	assignCol := rl.NewRectangle(listCol.X+colW+soundColGap, colY, colW, colH)

	l := soundLayout{card: card, paramsCol: paramsCol, listCol: listCol, assignCol: assignCol}

	// Params column: fixed sub-header band, scrollable body (section headers +
	// sliders), fixed footer (name + Preview/Save) that stays reachable.
	x := paramsCol.X + soundColInsetX
	w := paramsCol.Width - 2*soundColInsetX
	footerH := soundRowH + 6 + soundButtonH + 8 // name row + gap + buttons + pad
	vpY := paramsCol.Y + paramsHeaderReserve
	vpH := paramsCol.Height - paramsHeaderReserve - footerH
	if vpH < soundRowH {
		vpH = soundRowH
	}
	l.paramsViewport = rl.NewRectangle(paramsCol.X+1, vpY, paramsCol.Width-2, vpH)

	// First pass: unscrolled offsets, to learn the full content height.
	sliderOff := make([]float32, len(soundParamSliders))
	headerOff := make([]float32, len(soundSections))
	contentY := float32(0)
	gi := 0
	for si, sec := range soundSections {
		headerOff[si] = contentY
		contentY += soundGroupHeaderH
		for k := 0; k < sec.Count; k++ {
			sliderOff[gi] = contentY
			contentY += soundRowH
			gi++
		}
	}
	l.paramContentH = contentY
	l.paramMaxScroll = contentY - vpH
	if l.paramMaxScroll < 0 {
		l.paramMaxScroll = 0
	}
	sc := core.Clamp(paramScroll, 0, l.paramMaxScroll)

	// Second pass: place tracks + headers at scrolled screen positions.
	l.sliderTracks = make([]rl.Rectangle, len(soundParamSliders))
	for i := range soundParamSliders {
		l.sliderTracks[i] = rl.NewRectangle(x+soundSliderMetrics.labelW, vpY-sc+sliderOff[i]+8, w-soundSliderMetrics.trackReserve(2), soundSliderMetrics.trackH)
	}
	l.sectionHeaderY = make([]float32, len(soundSections))
	for si := range soundSections {
		l.sectionHeaderY[si] = vpY - sc + headerOff[si]
	}

	// Fixed footer below the viewport.
	fy := vpY + vpH + 4
	l.nameField = rl.NewRectangle(x+70, fy, w-70, textFieldH)
	fy += soundRowH + 6
	l.previewBtn = rl.NewRectangle(x, fy, (w-12)/2, soundButtonH)
	l.saveBtn = rl.NewRectangle(x+(w-12)/2+12, fy, (w-12)/2, soundButtonH)

	// List column rows + per-row buttons. Only the visible window gets real rects
	// (off-window entries stay zero, so they neither draw nor hit-test).
	lx := listCol.X + soundColInsetX
	lw := listCol.Width - 2*soundColInsetX
	ly := listCol.Y + soundColHeaderOffset
	listAreaH := listCol.Y + listCol.Height - soundColBottomMargin - ly
	var listBaseRows []rl.Rectangle
	l.listTopRow, l.listEnd, listBaseRows = windowedRowList(lx, ly, lw, soundListRowH-4, soundListRowH, listCursor, len(savedSounds), listAreaH)
	l.listRows = make([]soundListRowRect, len(savedSounds))
	for i := l.listTopRow; i < l.listEnd; i++ {
		row := listBaseRows[i]
		btns := rightButtonRow(row, 2, row.Height-4, 8,
			rowButtonSpec{soundRowEditBtnW, 2}, // Edit
			rowButtonSpec{soundRowCueBtnW, 6},  // Play
			rowButtonSpec{soundRowCueBtnW, 0},  // Delete
		)
		l.listRows[i] = soundListRowRect{
			Row:    row,
			Edit:   btns[0],
			Play:   btns[1],
			Delete: btns[2],
		}
	}

	// Assignments column. Same visible-window scheme as the saved-sounds list.
	ax := assignCol.X + soundColInsetX
	aw := assignCol.Width - 2*soundColInsetX
	ay := assignCol.Y + soundColHeaderOffset
	assignAreaH := assignCol.Y + assignCol.Height - soundColBottomMargin - ay
	var assignBaseRows []rl.Rectangle
	l.assignTopRow, l.assignEnd, assignBaseRows = windowedRowList(ax, ay, aw, soundAssignRowH-4, soundAssignRowH, assignCursor, len(assignableCueList), assignAreaH)
	l.assignRows = make([]soundAssignRowRect, len(assignableCueList))
	for i := l.assignTopRow; i < l.assignEnd; i++ {
		row := assignBaseRows[i]
		btns := rightButtonRow(row, 12, 24, 34,
			rowButtonSpec{soundRowCueBtnW, 6},    // Play
			rowButtonSpec{soundRowAssignBtnW, 0}, // Assign ▼ (opens the dropdown)
		)
		l.assignRows[i] = soundAssignRowRect{
			Row:    row,
			Play:   btns[0],
			Assign: btns[1],
		}
	}

	return l
}

// soundDrag holds the active slider-track drag (shared sliderDragState; idx < 0
// means none). See foeview.go for the type + update protocol.
var soundDrag = noSliderDrag

// openSoundsModal seeds first-time defaults and enters the sound modal.
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
	// Seed the disk-backed caches once; refreshed only on mutation thereafter.
	refreshSoundCaches(s)
	soundDrag = noSliderDrag // a stale index would pop a different slider
}

// refreshSoundCaches re-reads the saved-sounds listing + cue-assignment map from
// disk into State. Called on open + after each mutation, keeping draw/update off
// the disk (these were previously re-read every frame).
func refreshSoundCaches(s *State) {
	s.soundSavedCache = audio.ListUserSounds()
	s.soundAssignCache = audio.AllAssignments()
}

// updateSoundsModal handles the sound modal's input (mouse-first; keyboard is the
// secondary path: Tab columns, arrows adjust, Esc closes).
func updateSoundsModal(s *State) Action {
	if editorCancelPressed() {
		closeModal(s)
		return ActionNone
	}
	// Saved-sounds list from the cache — no per-frame os.ReadDir.
	savedSounds := s.soundSavedCache
	layout := computeSoundLayout(savedSounds, soundListCursor(s), soundAssignCursor(s), s.soundParamScroll)

	mp := rl.GetMousePosition()
	mouseDown := rl.IsMouseButtonDown(rl.MouseLeftButton)
	mousePressed := rl.IsMouseButtonPressed(rl.MouseLeftButton)
	mouseReleased := rl.IsMouseButtonReleased(rl.MouseLeftButton)

	// Wheel scrolls the params slider body when the pointer is over it. Suppressed
	// mid-drag: scrolling would shift the dragged track under the cursor (its geometry
	// is this frame's pre-scroll layout), snapping the value.
	if wheel := rl.GetMouseWheelMove(); wheel != 0 && soundDrag.idx < 0 && pointIn(mp, layout.paramsViewport) {
		s.soundParamScroll = core.Clamp(s.soundParamScroll-wheel*soundRowH, 0, layout.paramMaxScroll)
	}

	// Active slider drag: map mouse X within the track to a value, snapped to Step.
	soundDrag.update(mouseDown, len(soundParamSliders), func(idx int) {
		info := soundParamSliders[idx]
		track := layout.sliderTracks[idx]
		info.Set(&s.soundParams, sliderSnap(info.Min, info.Max, info.Step, track.X, track.Width, mp.X))
		s.soundLeftPanel = soundPanelParams
		s.soundCursor = idx
	})

	if mousePressed && pointIn(mp, layout.card) {
		s.soundLeftPanel, s.soundCursor = handleSoundMouseClick(s, mp, &layout, savedSounds)
	}
	if mouseReleased {
		soundDrag = noSliderDrag
	}

	// Name typing only while focus is the name row (clicking the field sets it).
	if s.soundLeftPanel == soundPanelParams && s.soundCursor == soundNameCursorIdx() {
		// No-space filter so Space can preview without typing into the name.
		pumpPrintableASCII(&s.soundName, soundNameMaxLen, acceptPrintableNoSpace, nil)
	}

	// Keyboard fallbacks.
	if editorTabPressed() {
		s.soundLeftPanel = core.WrapEnum(s.soundLeftPanel, 1, int(soundPanelCount))
		s.soundCursor = 0
		return ActionNone
	}
	switch s.soundLeftPanel {
	case soundPanelParams:
		updateSoundsParamsKeys(s, &layout)
	case soundPanelList:
		updateSoundsListKeys(s, savedSounds)
	case soundPanelAssign:
		updateSoundsAssignKeys(s)
	default:
		panic("editor: updateSoundsModal missing key handler for soundPanel")
	}
	return ActionNone
}

// soundNameCursorIdx / soundActionCursorIdx are the name + action rows, just past
// the last slider.
func soundNameCursorIdx() int   { return len(soundParamSliders) }
func soundActionCursorIdx() int { return len(soundParamSliders) + 1 }

// handleSoundMouseClick dispatches a left-mouse-down, returning the new
// (leftPanel, cursor). Unchanged cursor means the click missed a cursor target.
func handleSoundMouseClick(s *State, mp rl.Vector2, l *soundLayout, savedSounds []string) (soundPanel, int) {
	// Slider tracks — gated to the viewport so a track scrolled behind the
	// header/footer can't catch their clicks.
	for i := range soundParamSliders {
		if pointIn(mp, l.paramsViewport) && pointIn(mp, l.sliderTracks[i]) {
			soundDrag.idx = i
			// Apply the value at the click X so a single click adjusts, not just
			// a drag (soundDrag.update ran before this dispatch).
			info := soundParamSliders[i]
			track := l.sliderTracks[i]
			info.Set(&s.soundParams, sliderSnap(info.Min, info.Max, info.Step, track.X, track.Width, mp.X))
			return soundPanelParams, i
		}
	}
	if pointIn(mp, l.nameField) {
		return soundPanelParams, soundNameCursorIdx()
	}
	if pointIn(mp, l.previewBtn) {
		previewSoundParams(s.soundParams)
		return soundPanelParams, soundActionCursorIdx()
	}
	if pointIn(mp, l.saveBtn) {
		saveCurrentSound(s)
		return soundPanelParams, soundActionCursorIdx()
	}
	// Saved-sounds list — rows + per-row buttons (visible window only).
	for i := l.listTopRow; i < l.listEnd; i++ {
		r := l.listRows[i]
		if pointIn(mp, r.Edit) {
			loadSoundForEdit(s, savedSounds[i])
			return soundPanelList, i
		}
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
	// Assignments column (visible window only).
	for i := l.assignTopRow; i < l.assignEnd; i++ {
		r := l.assignRows[i]
		if pointIn(mp, r.Play) {
			audio.Play(assignableCueList[i])
			return soundPanelAssign, i
		}
		if pointIn(mp, r.Assign) {
			s.soundCursor = i // the builder reads the cue from soundCursor
			openDropdownBelow(s, ddSoundAssign, r.Assign)
			return soundPanelAssign, i
		}
		if pointIn(mp, r.Row) {
			return soundPanelAssign, i
		}
	}
	return s.soundLeftPanel, s.soundCursor
}

// updateSoundsParamsKeys drives the params column via keyboard: arrows move the
// cursor + adjust sliders, Space previews, Enter saves on the action row.
func updateSoundsParamsKeys(s *State, l *soundLayout) {
	rowCount := len(soundParamSliders) + 2 // sliders + name + actions
	if s.soundCursor == soundNameCursorIdx() {
		// Name row feeds every printable key (incl. W/S) into the buffer, so use
		// arrow/pad-only up-down to navigate off it.
		s.soundCursor = input.CursorUpDownTextSafe(s.soundCursor, rowCount)
	} else {
		s.soundCursor = input.CursorUpDown(s.soundCursor, rowCount)
	}
	if s.soundCursor < len(soundParamSliders) {
		ensureSliderVisible(s, l)
		slider := soundParamSliders[s.soundCursor]
		if delta := input.CursorLeftRight(); delta != 0 {
			v := slider.Get(&s.soundParams) + float64(delta)*slider.Step
			slider.Set(&s.soundParams, core.Clamp(v, slider.Min, slider.Max))
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
	// Name row: keystrokes feed the buffer (in updateSoundsModal); Enter saves.
	if s.soundCursor == soundNameCursorIdx() && editorCommitPressed() {
		saveCurrentSound(s)
	}
}

// ensureSliderVisible scrolls the params body so the cursor's slider row stays in
// the viewport after a keyboard move. No-op off a slider row.
func ensureSliderVisible(s *State, l *soundLayout) {
	if s.soundCursor < 0 || s.soundCursor >= len(l.sliderTracks) {
		return
	}
	vp := l.paramsViewport
	rowTop := l.sliderTracks[s.soundCursor].Y - 8
	if rowTop < vp.Y {
		s.soundParamScroll -= vp.Y - rowTop
	} else if rowTop+soundRowH > vp.Y+vp.Height {
		s.soundParamScroll += rowTop + soundRowH - (vp.Y + vp.Height)
	}
	s.soundParamScroll = core.Clamp(s.soundParamScroll, 0, l.paramMaxScroll)
}

func updateSoundsListKeys(s *State, names []string) {
	if len(names) == 0 {
		return
	}
	if s.soundCursor >= len(names) {
		s.soundCursor = len(names) - 1
	}
	if s.soundCursor < 0 {
		s.soundCursor = 0 // guard before indexing names[soundCursor]
	}
	s.soundCursor = input.CursorUpDown(s.soundCursor, len(names))
	if editorCommitPressed() || rl.IsKeyPressed(rl.KeySpace) {
		audio.PreviewFile(names[s.soundCursor])
	}
	if editorEditPressed() {
		loadSoundForEdit(s, names[s.soundCursor])
	}
	if editorDeletePressed() {
		confirmSoundDelete(s, names[s.soundCursor])
	}
}

// confirmSoundDelete is a two-press guard on the irreversible delete: first call
// arms `name`, the next for the SAME name deletes. A different name re-arms.
func confirmSoundDelete(s *State, name string) {
	if !armOrConfirmDelete(s, "sound:"+name, "Delete "+name+"? Click × again (or press X) to confirm") {
		return
	}
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
	if rl.IsKeyPressed(rl.KeySpace) {
		audio.Play(cue)
	}
	if editorCommitPressed() {
		openSoundAssignDropdown(s) // Enter opens the assignment picker (generic Up/Down/Enter/Esc)
	}
}

// openSoundAssignDropdown arms the cue-assignment picker anchored on the current
// row's Assign button (falls back to the column when scrolled off-window).
func openSoundAssignDropdown(s *State) {
	l := computeSoundLayout(s.soundSavedCache, soundListCursor(s), soundAssignCursor(s), s.soundParamScroll)
	if s.soundCursor >= l.assignTopRow && s.soundCursor < l.assignEnd {
		openDropdownBelow(s, ddSoundAssign, l.assignRows[s.soundCursor].Assign)
		return
	}
	openDropdownBelow(s, ddSoundAssign, l.assignCol)
}

// soundAssignEntries builds the cue-assignment picker: "(default)" then every saved
// user sound, choosing binds it to the cue at soundCursor. Marks the current pick.
func soundAssignEntries(s *State) []dropdownEntry {
	cues := assignableCueList
	if s.soundCursor < 0 || s.soundCursor >= len(cues) {
		return nil
	}
	cue := cues[s.soundCursor]
	current := s.soundAssignCache[audio.SoundCanonicalName(cue)]
	out := []dropdownEntry{{
		label:  "(default)",
		active: func(*State) bool { return current == "" },
		apply:  func(s *State) { setCueAssignment(s, cue, "") },
	}}
	for _, name := range s.soundSavedCache {
		name := name
		out = append(out, dropdownEntry{
			label:  name,
			active: func(*State) bool { return current == name },
			apply:  func(s *State) { setCueAssignment(s, cue, name) },
		})
	}
	return out
}

// setCueAssignment binds cue to the named user sound ("" = revert to default),
// surfacing errors and refreshing the caches.
func setCueAssignment(s *State, cue audio.Sound, name string) {
	failed, err := audio.AssignUserSound(cue, name)
	if err != nil {
		s.flash("Assign failed: " + err.Error())
		return
	}
	if len(failed) > 0 {
		s.flash("Saved assignment but reload failed for: " + strings.Join(failed, ", "))
	}
	refreshSoundCaches(s)
}

// previewSoundParams synthesizes the slider settings to PCM and plays it through
// audio's in-memory preview ring — no disk write.
func previewSoundParams(p soundParamSet) {
	audio.PreviewPCM(audio.SynthShapeParams(p))
}

// loadSoundForEdit pulls a saved sound's knobs back into the params column for
// re-tuning. Sounds with no .snd sidecar can't be reconstructed — flash and bail.
func loadSoundForEdit(s *State, name string) {
	p, ok := audio.LoadUserSoundParams(name)
	if !ok {
		s.flash(name + " has no editable params (only its .wav exists)")
		return
	}
	s.soundParams = p
	s.soundName = name
	s.soundLeftPanel = soundPanelParams
	s.soundCursor = 0
	s.soundParamScroll = 0
	s.flash("Editing " + name)
}

// saveCurrentSound writes the slider state to maps/sounds/<name>.wav + its .snd
// sidecar. SaveUserSoundParams sanitizes the name and returns the on-disk form
// (reported back). Saving onto an existing name overwrites it.
func saveCurrentSound(s *State) {
	if strings.TrimSpace(s.soundName) == "" {
		s.flash("Sound name required")
		return
	}
	saved, err := audio.SaveUserSoundParams(s.soundName, s.soundParams)
	if err != nil {
		s.flash("Save failed: " + err.Error())
		return
	}
	s.soundName = saved
	s.flash("Saved " + saved + audio.WavExt)
	refreshSoundCaches(s)
}

// drawSoundsModal renders the three-column sound editor.
func drawSoundsModal(s *State, font rl.Font, theme render.Theme) {
	// Cached saved-sounds list from this frame's Update; fall back to a fresh
	// read if nil (defensive — Draw without a prior Update shouldn't happen).
	savedSounds := s.soundSavedCache
	if savedSounds == nil {
		savedSounds = audio.ListUserSounds()
	}
	l := computeSoundLayout(savedSounds, soundListCursor(s), soundAssignCursor(s), s.soundParamScroll)
	// drawModalHeader centers via the same centeredCardRect as l.card, so visual
	// + hit-test stay in sync.
	_ = drawModalHeader(font, theme, soundModalW, soundModalH, "SOUNDS", theme.BorderActive)

	drawSoundsParamsCol(s, font, theme, &l)
	drawSoundsListCol(s, font, theme, &l, savedSounds)
	drawSoundsAssignCol(s, font, theme, &l)

	hint := "Drag sliders · scroll params · Edit a saved sound to reload its knobs · Save overwrites · Tab cycles column · Esc closes"
	// Shared footer baseline/inset (soundFontHint keeps this modal's larger hint font).
	render.DrawRichText(font, hint,
		rl.NewVector2(l.card.X+modalContentInset, l.card.Y+l.card.Height-modalFooterHintDY),
		soundFontHint, 1, theme.TextHint)
}

func drawSoundsParamsCol(s *State, font rl.Font, theme render.Theme, l *soundLayout) {
	drawSoundsColumnFrame(theme, l.paramsCol, s.soundLeftPanel == soundPanelParams)
	render.DrawSubHeading(font, "Synth params", l.paramsCol.X+soundColInsetX, l.paramsCol.Y+8, theme.BorderActive)

	// Clip the slider body to the viewport so scrolled rows don't paint over the
	// sub-header/footer (palette/metadata scissor pattern).
	vp := l.paramsViewport
	rl.BeginScissorMode(int32(vp.X), int32(vp.Y), int32(vp.Width), int32(vp.Height))
	gi := 0
	for si, sec := range soundSections {
		hy := l.sectionHeaderY[si]
		if hy+soundGroupHeaderH > vp.Y && hy < vp.Y+vp.Height { // header in band
			render.DrawRichText(font, sec.Title, rl.NewVector2(l.paramsCol.X+soundColInsetX, hy+4), soundFontHint, 1, theme.TextLabel)
			lineY := hy + soundGroupHeaderH - 3
			rl.DrawLineEx(rl.NewVector2(l.paramsCol.X+soundColInsetX, lineY), rl.NewVector2(l.paramsCol.X+l.paramsCol.Width-soundColInsetX, lineY), 1, theme.BorderDim)
		}
		for k := 0; k < sec.Count; k++ {
			track := l.sliderTracks[gi]
			// Skip rows fully outside the viewport (avoids draw work).
			if track.Y+soundRowH > vp.Y && track.Y-8 < vp.Y+vp.Height {
				focused := s.soundLeftPanel == soundPanelParams && s.soundCursor == gi
				drawSoundsSlider(font, theme, l.paramsCol.X+soundColInsetX, track.Y-8, l.paramsCol.Width-2*soundColInsetX, soundParamSliders[gi], s.soundParams, track, focused)
			}
			gi++
		}
	}
	rl.EndScissorMode()

	// Scroll affordances when the body overflows.
	if l.paramMaxScroll > 0 {
		if s.soundParamScroll > 0 {
			drawScrollArrow(font, true, rl.NewVector2(l.paramsCol.X+l.paramsCol.Width-22, vp.Y+2), soundFontHint, theme.TextHint)
		}
		if s.soundParamScroll < l.paramMaxScroll {
			drawScrollArrow(font, false, rl.NewVector2(l.paramsCol.X+l.paramsCol.Width-22, vp.Y+vp.Height-18), soundFontHint, theme.TextHint)
		}
	}

	// Name field (fixed footer).
	nameFocused := s.soundLeftPanel == soundPanelParams && s.soundCursor == soundNameCursorIdx()
	nameLabelCol := theme.TextMuted
	if nameFocused {
		nameLabelCol = theme.BorderActive
	}
	render.DrawRichText(font, "Name", rl.NewVector2(l.paramsCol.X+soundColInsetX, l.nameField.Y+6), soundFontBody, 1, nameLabelCol)
	drawTextField(font, l.nameField, s.soundName, nameFocused)
	// Action buttons (fixed footer).
	actionFocused := s.soundLeftPanel == soundPanelParams && s.soundCursor == soundActionCursorIdx()
	drawButton(font, l.previewBtn, "Preview (Space)", actionFocused)
	drawButton(font, l.saveBtn, "Save", actionFocused)
}

func drawSoundsSlider(font rl.Font, theme render.Theme, x, y, w float32, info sliderField[soundParamSet], p soundParamSet, track rl.Rectangle, focused bool) {
	// Display callback overrides the numeric readout for label rows (e.g. Wave).
	drawSliderField(font, theme, info, &p,
		rl.NewVector2(x, y), rl.NewVector2(x+w-soundSliderMetrics.valueW, y),
		soundFontBody, track, soundSliderMetrics.thumbR, focused)
}

func drawSoundsListCol(s *State, font rl.Font, theme render.Theme, l *soundLayout, names []string) {
	drawSoundsColumnFrame(theme, l.listCol, s.soundLeftPanel == soundPanelList)
	render.DrawSubHeading(font, "Saved sounds", l.listCol.X+soundColInsetX, l.listCol.Y+8, theme.BorderActive)
	if len(names) == 0 {
		render.DrawRichText(font, "(no saved sounds yet)",
			rl.NewVector2(l.listCol.X+soundColInsetX, l.listCol.Y+44), soundFontBody, 1, theme.TextHint)
		render.DrawRichText(font, "Save one from the left column.",
			rl.NewVector2(l.listCol.X+soundColInsetX, l.listCol.Y+68), soundFontHint, 1, theme.TextHint)
		return
	}
	for i := l.listTopRow; i < l.listEnd; i++ {
		r := l.listRows[i]
		if s.soundLeftPanel == soundPanelList && s.soundCursor == i {
			drawSelectedListRow(r.Row)
		}
		render.DrawTextWithShadow(font, names[i],
			r.Row.X+8, r.Row.Y+6, soundFontBody, theme.TextMuted)
		drawButton(font, r.Edit, "Edit", false)
		drawButton(font, r.Play, ">", false)
		drawButton(font, r.Delete, "X", false)
	}
	drawSoundColumnScrollHints(font, theme, l.listCol, l.listTopRow, len(names)-l.listEnd)
}

// drawSoundColumnScrollHints paints the "▲/▼ N more" captions for a column's
// off-window rows (each no-ops at count 0).
func drawSoundColumnScrollHints(font rl.Font, theme render.Theme, col rl.Rectangle, hiddenAbove, hiddenBelow int) {
	x := col.X + col.Width - 70
	drawScrollMoreHint(font, theme, x, col.Y+10, hiddenAbove, true)
	drawScrollMoreHint(font, theme, x, col.Y+col.Height-20, hiddenBelow, false)
}

func drawSoundsAssignCol(s *State, font rl.Font, theme render.Theme, l *soundLayout) {
	drawSoundsColumnFrame(theme, l.assignCol, s.soundLeftPanel == soundPanelAssign)
	render.DrawSubHeading(font, "Built-in cue assignments", l.assignCol.X+soundColInsetX, l.assignCol.Y+8, theme.BorderActive)
	for i := l.assignTopRow; i < l.assignEnd; i++ {
		cue := assignableCueList[i]
		r := l.assignRows[i]
		if s.soundLeftPanel == soundPanelAssign && s.soundCursor == i {
			drawSelectedListRow(r.Row)
		}
		render.DrawTextWithShadow(font, audio.SoundName(cue),
			r.Row.X+8, r.Row.Y+4, soundFontBody, theme.TextMuted)
		assigned := s.soundAssignCache[audio.SoundCanonicalName(cue)]
		assignedLabel := "(default)"
		if assigned != "" {
			assignedLabel = "→ " + assigned
		}
		render.DrawRichText(font, assignedLabel,
			rl.NewVector2(r.Row.X+8, r.Row.Y+24),
			soundFontHint, 1, theme.TextHint)
		drawButton(font, r.Play, ">", false)
		drawButton(font, r.Assign, "Assign"+dropdownArrowSuffix, s.dropdown.owner == ddSoundAssign && s.soundCursor == i)
	}
	drawSoundColumnScrollHints(font, theme, l.assignCol, l.assignTopRow, len(assignableCueList)-l.assignEnd)
}

func drawSoundsColumnFrame(theme render.Theme, r rl.Rectangle, focused bool) {
	// SurfacePrimary at full alpha — darker than the card so columns read as inset.
	bg := theme.SurfacePrimary
	bg.A = 255
	rl.DrawRectangleRec(r, bg)
	border := theme.BorderDim
	if focused {
		border = theme.BorderActive
	}
	rl.DrawRectangleLinesEx(r, 1, border)
}
