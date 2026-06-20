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

// soundParamSet captures the tunables for a single editor-authored cue.
// It is an alias of audio.ShapeParams (the synth's own input struct) so
// the editor, the synth, and the on-disk .snd sidecar all share ONE
// param shape — no parallel struct to keep in lockstep, and a saved
// sound round-trips losslessly back into the sliders. The sliders in
// soundParamSliders map their 0..1 normalized position into the field
// ranges; soundParamDefaults seeds an audible starter cue.
type soundParamSet = audio.ShapeParams

// soundSection groups consecutive soundParamSliders rows under a heading
// in the (now scrollable) params column. Count is how many sliders the
// section owns; the sum is asserted == len(soundParamSliders) at init so
// a row added without a home is caught at startup, not silently hidden.
type soundSection struct {
	Title string
	Count int
}

var soundSections = []soundSection{
	{"Oscillator", 5}, // Wave, Start/End note, Pulse Width, Noise
	{"Envelope", 6},   // Duration, Attack, Decay, Sustain, Release, Volume
	{"FX", 7},         // Vibrato Hz/Depth, Tremolo Hz/Depth, Cutoff, Drive, Crush
}

// noteSlider builds a discrete note-picker row over an Hz field: the
// underlying value stays a frequency (so the synth + sidecar are
// unchanged), but the control reads/writes tempered note indices and the
// readout shows "A4 (440)". Shared by the Start/End pitch rows so authored
// pitches land on the same tempered grid as the procedural musical cues.
func noteSlider(label string, get func(*soundParamSet) float64, set func(*soundParamSet, float64)) sliderField[soundParamSet] {
	return sliderField[soundParamSet]{
		Label: label, Min: 0, Max: float64(audio.NoteCount - 1), Step: 1, Format: "%.0f",
		Get: func(p *soundParamSet) float64 { return float64(audio.NearestNoteIndex(get(p))) },
		Set: func(p *soundParamSet, v float64) { set(p, audio.NoteHz(int(v+0.5))) },
		Display: func(v float64) string {
			i := int(v + 0.5)
			return fmt.Sprintf("%s (%.0f)", audio.NoteName(i), audio.NoteHz(i))
		},
	}
}

// soundParamSliders describes the sound modal's slider column — one
// sliderField (slider.go) row per synth parameter, in soundSections
// order. The cursor walks this slice; each row reads/writes its named
// field on the soundParamSet via the getter/setter callbacks.
var soundParamSliders = []sliderField[soundParamSet]{
	// — Oscillator —
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
			p.Wave = audio.WaveShape(i)
		},
		Display: func(v float64) string {
			return audio.WaveShapeName(audio.WaveShape(int(v + 0.5)))
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
		// Cutoff — one-pole low-pass; 1.0 = fully open (no filtering).
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

// soundParamDefaults seeds the modal with a sane starter cue — a short
// rising blip at modest volume with every "extra" knob at its neutral
// (no-op) value: full sustain, open filter, no decay/tremolo/drive/crush.
// All fields are listed explicitly so a future field/enum reordering
// doesn't silently shift the default timbre — the literal expresses intent.
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

// Sound modal layout constants. Sized larger than the previous pass so
// the labels are readable on a 1080p display without leaning in. Modal
// occupies a 900×560 card; three columns plus a hint footer.
//
// Three "body" font sizes (label / value / list) all collapse to one
// soundFontBody — they had drifted to the same 16pt value, so the
// distinction was misleading. Both alias the shared editor type scale
// (palette.go) so the sound modal can't drift off it: body == editorFontBody,
// the smaller footer hint == editorFontAccent. The SOUNDS heading is drawn
// through render.DrawHeading (no local size).
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
	// paramsHeaderReserve / soundGroupHeaderH size the scrollable params
	// column: a fixed sub-header band at the top, then a scrolling body of
	// section headers (soundGroupHeaderH each) + slider rows under a fixed
	// name/actions footer. The body scrolls because the grouped slider list
	// is taller than the column.
	paramsHeaderReserve = float32(30)
	soundGroupHeaderH   = float32(22)
	// soundNameMaxLen caps the sound-name text field. The sound modal pumps
	// this field directly (no-space filter) rather than through input.go's
	// textFieldConfigs table, so its cap lives here next to the modal's other
	// layout constants instead of as a bare literal at the pump call.
	soundNameMaxLen = 32
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

// rowButtonSpec describes one button in a right-anchored per-row button group:
// its width and the gap to the NEXT button on its right (gapAfter is ignored
// for the rightmost spec). Used by rightButtonRow.
type rowButtonSpec struct {
	w        float32
	gapAfter float32
}

// rightButtonRow lays the specs out left-to-right as a block right-anchored
// inside `row`: the rightmost button's right edge sits rightInset px from the
// row's right edge, each button is yOff px down from the row top with height h.
// The per-row Edit/Play/Delete (saved-sounds list) and Play/◂/▸ (cue
// assignments) groups were placed by hand-derived right-edge offsets that had
// to stay in lockstep with their widths; this is the one placement formula for
// both. Returns rects in spec (left-to-right) order.
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

// soundListRowRect bundles the row + per-row buttons for one saved
// sound. Replaces three parallel []rl.Rectangle slices that had to be
// kept in lockstep — now drift between row count and button count is
// impossible.
type soundListRowRect struct {
	Row    rl.Rectangle
	Edit   rl.Rectangle
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
	sliderTracks []rl.Rectangle // per-slider clickable track (scrolled); len == len(soundParamSliders)
	// paramsViewport is the clipped scroll region the slider body draws/
	// hit-tests inside; sectionHeaderY[i] is the scrolled screen Y of
	// soundSections[i]'s heading. paramContentH is the body's full height
	// and paramMaxScroll the clamped scroll ceiling.
	paramsViewport rl.Rectangle
	sectionHeaderY []float32
	paramContentH  float32
	paramMaxScroll float32
	nameField      rl.Rectangle
	previewBtn     rl.Rectangle
	saveBtn        rl.Rectangle
	listRows       []soundListRowRect   // one entry per saved-sound row (rects only filled for the visible window)
	assignRows     []soundAssignRowRect // one entry per built-in cue row (rects only filled for the visible window)
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

func computeSoundLayout(savedSounds []string, listCursor, assignCursor int, paramScroll float32) soundLayout {
	card := centeredCardRect(soundModalW, soundModalH)
	colW := (modalContentWidth(card) - 2*soundColGap) / 3
	colY := card.Y + 56
	colH := card.Height - 110

	paramsCol := rl.NewRectangle(card.X+modalContentInset, colY, colW, colH)
	listCol := rl.NewRectangle(paramsCol.X+colW+soundColGap, colY, colW, colH)
	assignCol := rl.NewRectangle(listCol.X+colW+soundColGap, colY, colW, colH)

	l := soundLayout{card: card, paramsCol: paramsCol, listCol: listCol, assignCol: assignCol}

	// Params column: a fixed sub-header band, a scrollable body of section
	// headers + slider rows, and a fixed footer (name + Preview/Save). The
	// footer stays reachable no matter how far the body scrolls.
	x := paramsCol.X + 12
	w := paramsCol.Width - 24
	footerH := soundRowH + 6 + soundButtonH + 8 // name row + gap + buttons + pad
	vpY := paramsCol.Y + paramsHeaderReserve
	vpH := paramsCol.Height - paramsHeaderReserve - footerH
	if vpH < soundRowH {
		vpH = soundRowH
	}
	l.paramsViewport = rl.NewRectangle(paramsCol.X+1, vpY, paramsCol.Width-2, vpH)

	// First pass: unscrolled content offsets for each section header + slider,
	// so we know the full content height before clamping the scroll.
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

	// Second pass: place tracks + headers at their scrolled screen positions.
	l.sliderTracks = make([]rl.Rectangle, len(soundParamSliders))
	for i := range soundParamSliders {
		l.sliderTracks[i] = rl.NewRectangle(x+96, vpY-sc+sliderOff[i]+8, w-176, 14)
	}
	l.sectionHeaderY = make([]float32, len(soundSections))
	for si := range soundSections {
		l.sectionHeaderY[si] = vpY - sc + headerOff[si]
	}

	// Fixed footer below the viewport.
	fy := vpY + vpH + 4
	l.nameField = rl.NewRectangle(x+70, fy, w-70, 28)
	fy += soundRowH + 6
	l.previewBtn = rl.NewRectangle(x, fy, (w-12)/2, soundButtonH)
	l.saveBtn = rl.NewRectangle(x+(w-12)/2+12, fy, (w-12)/2, soundButtonH)

	// List column rows + per-row Play/× buttons. Only the visible window gets
	// real rects; off-window entries stay zero (so they neither draw nor
	// hit-test). The window keeps listCursor on screen.
	lx := listCol.X + 12
	lw := listCol.Width - 24
	ly := listCol.Y + 36
	listAreaH := listCol.Y + listCol.Height - 12 - ly
	var listBaseRows []rl.Rectangle
	l.listTopRow, l.listEnd, listBaseRows = windowedRowList(lx, ly, lw, soundListRowH-4, soundListRowH, listCursor, len(savedSounds), listAreaH)
	l.listRows = make([]soundListRowRect, len(savedSounds))
	for i := l.listTopRow; i < l.listEnd; i++ {
		row := listBaseRows[i]
		btns := rightButtonRow(row, 2, row.Height-4, 8,
			rowButtonSpec{38, 2}, // Edit
			rowButtonSpec{32, 6}, // Play
			rowButtonSpec{32, 0}, // Delete
		)
		l.listRows[i] = soundListRowRect{
			Row:    row,
			Edit:   btns[0],
			Play:   btns[1],
			Delete: btns[2],
		}
	}

	// Assignments column. Same visible-window scheme as the saved-sounds
	// list: only on-window cue rows get real rects, so a cue list that
	// outgrows the column scrolls instead of overflowing off the card.
	ax := assignCol.X + 12
	aw := assignCol.Width - 24
	ay := assignCol.Y + 36
	assignAreaH := assignCol.Y + assignCol.Height - 12 - ay
	var assignBaseRows []rl.Rectangle
	l.assignTopRow, l.assignEnd, assignBaseRows = windowedRowList(ax, ay, aw, soundAssignRowH-4, soundAssignRowH, assignCursor, len(assignableCueList), assignAreaH)
	l.assignRows = make([]soundAssignRowRect, len(assignableCueList))
	for i := l.assignTopRow; i < l.assignEnd; i++ {
		row := assignBaseRows[i]
		btns := rightButtonRow(row, 12, 24, 34,
			rowButtonSpec{32, 4}, // Play
			rowButtonSpec{24, 8}, // CycleLeft
			rowButtonSpec{24, 0}, // CycleRight
		)
		l.assignRows[i] = soundAssignRowRect{
			Row:        row,
			Play:       btns[0],
			CycleLeft:  btns[1],
			CycleRight: btns[2],
		}
	}

	return l
}

// soundDrag is set while the mouse is held on a slider track — motion updates
// the slider's value continuously. Released when the mouse button releases.
// Uses the shared sliderDragState (idx < 0 means "no active drag"); see
// foeview.go for the type + its update protocol.
var soundDrag = noSliderDrag

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
	// Reset any leftover drag from a prior session — a stale index
	// would let the next mouse drag pop a different slider.
	soundDrag = noSliderDrag
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
	layout := computeSoundLayout(savedSounds, soundListCursor(s), soundAssignCursor(s), s.soundParamScroll)

	mp := rl.GetMousePosition()
	mouseDown := rl.IsMouseButtonDown(rl.MouseLeftButton)
	mousePressed := rl.IsMouseButtonPressed(rl.MouseLeftButton)
	mouseReleased := rl.IsMouseButtonReleased(rl.MouseLeftButton)

	// Wheel scrolls the params slider body when the pointer is over it.
	if wheel := rl.GetMouseWheelMove(); wheel != 0 && pointIn(mp, layout.paramsViewport) {
		s.soundParamScroll = core.Clamp(s.soundParamScroll-wheel*soundRowH, 0, layout.paramMaxScroll)
	}

	// Active slider drag: while held, map the mouse X within the track
	// to a value in the slider's range. Snap to the slider's Step grain
	// so dragging produces clean readouts instead of fractional noise.
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

	// Name field typing: only while the focus is on the name row of the
	// params column. The user has to click the field (which sets cursor
	// to the name row) before keystrokes register — that fixes the old
	// "Space types into the name" trap.
	if s.soundLeftPanel == soundPanelParams && s.soundCursor == soundNameCursorIdx() {
		// No-space filter so the user can hit Space for Preview without
		// also typing a space into the sound name. Shared pump from
		// input.go — backspace handled there too.
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
	// click position so single-click also adjusts (not just drag). Gated to
	// the scroll viewport so a track scrolled behind the header/footer can't
	// catch a click meant for those fixed elements.
	for i := range soundParamSliders {
		if pointIn(mp, l.paramsViewport) && pointIn(mp, l.sliderTracks[i]) {
			soundDrag.idx = i
			// Apply the value at the click X immediately so a single click
			// adjusts, not just a drag. soundDrag.update runs BEFORE this
			// dispatch in the frame, so without setting here a click-release
			// with no motion would leave the value unchanged (the comment
			// above promised single-click works; this makes it true).
			info := soundParamSliders[i]
			track := l.sliderTracks[i]
			info.Set(&s.soundParams, sliderSnap(info.Min, info.Max, info.Step, track.X, track.Width, mp.X))
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
func updateSoundsParamsKeys(s *State, l *soundLayout) {
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
	// On the Name row: keystrokes feed into the name buffer (handled by
	// updateSoundsModal); Enter saves.
	if s.soundCursor == soundNameCursorIdx() && editorCommitPressed() {
		saveCurrentSound(s)
	}
}

// ensureSliderVisible nudges the params scroll so the cursor's slider row
// sits inside the viewport — called after keyboard cursor moves so arrowing
// down a long param list scrolls the body to follow instead of stranding the
// cursor off-screen. No-op when the cursor isn't on a slider row.
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
		s.soundCursor = 0 // guard both ends before indexing names[soundCursor] below
	}
	s.soundCursor = input.CursorUpDown(s.soundCursor, len(names))
	if editorCommitPressed() || rl.IsKeyPressed(rl.KeySpace) {
		audio.PreviewFile(names[s.soundCursor])
	}
	if rl.IsKeyPressed(rl.KeyE) {
		loadSoundForEdit(s, names[s.soundCursor])
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
	audio.PreviewPCM(audio.SynthShapeParams(p))
}

// loadSoundForEdit pulls a saved sound's knobs back into the params
// column so it can be re-tuned and re-saved (overwriting). Sounds with no
// .snd sidecar (hand-dropped .wav, or saved before sidecars existed)
// can't be reconstructed — flash and leave the current params untouched.
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

// saveCurrentSound writes the slider state to maps/sounds/<name>.wav plus
// its <name>.snd editing sidecar, and flashes a status message.
// audio.SaveUserSoundParams sanitizes the name itself and returns the
// final on-disk form, so the editor reports exactly what landed on disk
// (handles a user typing "My Cue!" → file becomes "my_cue.wav"). Saving
// onto an existing name overwrites it — that's the "edit then re-save" path.
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
	l := computeSoundLayout(savedSounds, soundListCursor(s), soundAssignCursor(s), s.soundParamScroll)
	// Both computeSoundLayout and drawModalHeader center the card through
	// the shared centeredCardRect, so the rect drawModalHeader paints is
	// identical to l.card — visual + hit-test stay in sync without
	// duplicating the card-draw call.
	_ = drawModalHeader(font, theme, soundModalW, soundModalH, "SOUNDS", theme.BorderActive)

	drawSoundsParamsCol(s, font, theme, &l)
	drawSoundsListCol(s, font, theme, &l, savedSounds)
	drawSoundsAssignCol(s, font, theme, &l)

	hint := "Drag sliders · scroll params · Edit a saved sound to reload its knobs · Save overwrites · Tab cycles column · Esc closes"
	render.DrawRichText(font, hint,
		rl.NewVector2(l.card.X+18, l.card.Y+l.card.Height-26),
		soundFontHint, 1, theme.TextHint)
}

func drawSoundsParamsCol(s *State, font rl.Font, theme render.Theme, l *soundLayout) {
	drawSoundsColumnFrame(theme, l.paramsCol, s.soundLeftPanel == soundPanelParams)
	render.DrawSubHeading(font, "Synth params", l.paramsCol.X+12, l.paramsCol.Y+8, theme.BorderActive)

	// Scrollable slider body: clip to the viewport so section headers / rows
	// scrolled past the top or bottom don't paint over the sub-header or the
	// fixed footer. Mirrors the palette/metadata scissor pattern.
	vp := l.paramsViewport
	rl.BeginScissorMode(int32(vp.X), int32(vp.Y), int32(vp.Width), int32(vp.Height))
	gi := 0
	for si, sec := range soundSections {
		hy := l.sectionHeaderY[si]
		// Only draw a header that falls within the viewport band.
		if hy+soundGroupHeaderH > vp.Y && hy < vp.Y+vp.Height {
			render.DrawRichText(font, sec.Title, rl.NewVector2(l.paramsCol.X+12, hy+4), soundFontHint, 1, theme.TextLabel)
			lineY := hy + soundGroupHeaderH - 3
			rl.DrawLineEx(rl.NewVector2(l.paramsCol.X+12, lineY), rl.NewVector2(l.paramsCol.X+l.paramsCol.Width-12, lineY), 1, theme.BorderDim)
		}
		for k := 0; k < sec.Count; k++ {
			track := l.sliderTracks[gi]
			// Skip rows fully outside the viewport (scissor would clip them
			// anyway; this just avoids the draw work).
			if track.Y+soundRowH > vp.Y && track.Y-8 < vp.Y+vp.Height {
				focused := s.soundLeftPanel == soundPanelParams && s.soundCursor == gi
				drawSoundsSlider(font, theme, l.paramsCol.X+12, track.Y-8, l.paramsCol.Width-24, soundParamSliders[gi], s.soundParams, track, focused)
			}
			gi++
		}
	}
	rl.EndScissorMode()

	// Scroll affordances when the body overflows.
	if l.paramMaxScroll > 0 {
		if s.soundParamScroll > 0 {
			render.DrawRichText(font, "▲", rl.NewVector2(l.paramsCol.X+l.paramsCol.Width-22, vp.Y+2), soundFontHint, 1, theme.TextHint)
		}
		if s.soundParamScroll < l.paramMaxScroll {
			render.DrawRichText(font, "▼", rl.NewVector2(l.paramsCol.X+l.paramsCol.Width-22, vp.Y+vp.Height-18), soundFontHint, 1, theme.TextHint)
		}
	}

	// Name field (fixed footer). The "Name" label sits to the left of the
	// text field; drawTextField paints the field with the shared palette.
	nameFocused := s.soundLeftPanel == soundPanelParams && s.soundCursor == soundNameCursorIdx()
	nameLabelCol := theme.TextMuted
	if nameFocused {
		nameLabelCol = theme.BorderActive
	}
	render.DrawRichText(font, "Name", rl.NewVector2(l.paramsCol.X+12, l.nameField.Y+6), soundFontBody, 1, nameLabelCol)
	drawTextField(font, l.nameField, s.soundName, nameFocused)
	// Action buttons (fixed footer).
	actionFocused := s.soundLeftPanel == soundPanelParams && s.soundCursor == soundActionCursorIdx()
	drawButton(font, l.previewBtn, "Preview (Space)", actionFocused)
	drawButton(font, l.saveBtn, "Save", actionFocused)
}

func drawSoundsSlider(font rl.Font, theme render.Theme, x, y, w float32, info sliderField[soundParamSet], p soundParamSet, track rl.Rectangle, focused bool) {
	// Display-aware row draw (shared drawSliderField): the Display callback
	// overrides the fmt.Sprintf path for rows that render a label instead of a
	// number (the Wave row's "Sine"/"Square"/etc.). The numeric readout sits to
	// the right of the track.
	drawSliderField(font, theme, info, &p,
		rl.NewVector2(x, y), rl.NewVector2(x+w-78, y),
		soundFontBody, track, 7, focused)
}

func drawSoundsListCol(s *State, font rl.Font, theme render.Theme, l *soundLayout, names []string) {
	drawSoundsColumnFrame(theme, l.listCol, s.soundLeftPanel == soundPanelList)
	render.DrawSubHeading(font, "Saved sounds", l.listCol.X+12, l.listCol.Y+8, theme.BorderActive)
	if len(names) == 0 {
		render.DrawRichText(font, "(no saved sounds yet)",
			rl.NewVector2(l.listCol.X+12, l.listCol.Y+44), soundFontBody, 1, theme.TextHint)
		render.DrawRichText(font, "Save one from the left column.",
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
		drawButton(font, r.Edit, "Edit", false)
		drawButton(font, r.Play, ">", false)
		drawButton(font, r.Delete, "X", false)
	}
	drawSoundColumnScrollHints(font, theme, l.listCol, l.listTopRow, len(names)-l.listEnd)
}

// drawSoundColumnScrollHints paints the "▲ N more"/"▼ N more" captions in a
// sounds column's top-right / bottom-right when its row list has hiddenAbove /
// hiddenBelow rows outside the visible window. Routes through the shared
// drawScrollMoreHint so the glyph + "N more" format match every other
// scrollable list in the editor (each call no-ops when its count is 0).
func drawSoundColumnScrollHints(font rl.Font, theme render.Theme, col rl.Rectangle, hiddenAbove, hiddenBelow int) {
	x := col.X + col.Width - 70
	drawScrollMoreHint(font, theme, x, col.Y+10, hiddenAbove, true)
	drawScrollMoreHint(font, theme, x, col.Y+col.Height-20, hiddenBelow, false)
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
		render.DrawRichText(font, assignedLabel,
			rl.NewVector2(r.Row.X+8, r.Row.Y+24),
			soundFontHint, 1, theme.TextHint)
		drawButton(font, r.Play, ">", false)
		drawButton(font, r.CycleLeft, "<", false)
		drawButton(font, r.CycleRight, ">", false)
	}
	drawSoundColumnScrollHints(font, theme, l.assignCol, l.assignTopRow, len(assignableCueList)-l.assignEnd)
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
