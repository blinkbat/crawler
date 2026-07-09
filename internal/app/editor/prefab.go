package editor

import (
	"crawler/internal/app/core"
	"crawler/internal/app/render"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// prefab.go — a persistent stamp library (modalPrefabs, Assets ▸ Prefabs…). The
// session clipboard is single-slot and lost on exit; a prefab saves a copied region
// (tiles + entities) to maps/prefabs/<name>.prefab (JSON) to reuse across maps and
// sessions. Loading a prefab drops it back onto the clipboard for a normal Ctrl+V.

const prefabExt = ".prefab"

// prefabFile is the on-disk shape: the tile region plus the captured spawns (the
// regionEntities fields are unexported, so they're mirrored here with exported ones).
type prefabFile struct {
	Region   core.TileRegion
	Packs    []core.PackSpawn
	Chests   []core.ChestSpawn
	Doors    []core.DoorSpawn
	Crystals []core.CrystalSpawn
}

func prefabDir() string { return filepath.Join(core.MapsDir(), "prefabs") }
func prefabPath(name string) string {
	return filepath.Join(prefabDir(), core.SanitizeFilename(name, "prefab")+prefabExt)
}

// listPrefabs returns saved prefab file paths, sorted. Missing dir → empty.
func listPrefabs() []string {
	entries, err := os.ReadDir(prefabDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), prefabExt) {
			out = append(out, filepath.Join(prefabDir(), e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

func prefabIDFromPath(path string) string {
	return strings.TrimSuffix(filepath.Base(path), prefabExt)
}

// savePrefab writes the clipboard region + entities under name (JSON).
func savePrefab(name string, r core.TileRegion, e regionEntities) error {
	pf := prefabFile{Region: r, Packs: e.packs, Chests: e.chests, Doors: e.doors, Crystals: e.crystals}
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(prefabDir(), core.AssetDirMode); err != nil {
		return err
	}
	return os.WriteFile(prefabPath(name), data, core.AssetFileMode)
}

// loadPrefab reads a prefab file into a region + entities.
func loadPrefab(path string) (core.TileRegion, regionEntities, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return core.TileRegion{}, regionEntities{}, err
	}
	var pf prefabFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return core.TileRegion{}, regionEntities{}, err
	}
	return pf.Region, regionEntities{packs: pf.Packs, chests: pf.Chests, doors: pf.Doors, crystals: pf.Crystals}, nil
}

func openPrefabsModal(s *State) {
	s.modal = modalPrefabs
	s.prefabPaths = listPrefabs()
	s.prefabCursor = 0
	s.prefabNameFocus = false
	if s.prefabName == "" {
		s.prefabName = "room"
	}
}

const (
	prefabModalW  = float32(420)
	prefabRowH    = float32(26)
	prefabMaxRows = 9 // prefab rows shown at once; more scroll (mouse wheel)
)

type prefabLayout struct {
	card, nameField, saveBtn rl.Rectangle
	rows                     []prefabRowRects
	top, end                 int
}

type prefabRowRects struct{ row, load, del rl.Rectangle }

func prefabLayoutFor(s *State) prefabLayout {
	shown := min(len(s.prefabPaths), prefabMaxRows)
	card := centeredCardRect(prefabModalW, listModalHeight(shown, prefabRowH))
	x, w := cardContentBox(card)
	y := modalBodyTop(card)
	l := prefabLayout{card: card}
	l.nameField = rl.NewRectangle(x, y, w-110, textFieldH)
	l.saveBtn = rl.NewRectangle(x+w-100, y, 100, textFieldH)
	y += textFieldH + 12
	l.top, l.end = scrollWindow(s.prefabCursor, len(s.prefabPaths), shown)
	for i := l.top; i < l.end; i++ {
		row := rl.NewRectangle(x, y+float32(i-l.top)*prefabRowH, w, prefabRowH-4)
		load := rl.NewRectangle(row.X+row.Width-84, row.Y, 52, row.Height)
		del := rl.NewRectangle(row.X+row.Width-26, row.Y, 26, row.Height)
		l.rows = append(l.rows, prefabRowRects{row: row, load: load, del: del})
	}
	return l
}

func updatePrefabsModal(s *State) Action {
	l := prefabLayoutFor(s)
	if s.prefabNameFocus {
		pumpPrintableASCII(&s.prefabName, 40, acceptPrintableNoSpace, nil)
	}
	if editorCancelPressed() {
		closeModal(s)
		return ActionNone
	}
	if editorCommitPressed() && s.prefabNameFocus {
		savePrefabFromClipboard(s)
		return ActionNone
	}
	if w := rl.GetMouseWheelMove(); w != 0 && len(s.prefabPaths) > prefabMaxRows {
		s.prefabCursor = clampCursor(s.prefabCursor-int(w), len(s.prefabPaths))
	}
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		mp := rl.GetMousePosition()
		switch {
		case pointIn(mp, l.nameField):
			s.prefabNameFocus = true
		case pointIn(mp, l.saveBtn):
			savePrefabFromClipboard(s)
		case !pointIn(mp, l.card):
			closeModal(s)
		default:
			s.prefabNameFocus = false
			for i := l.top; i < l.end; i++ {
				r := l.rows[i-l.top]
				if pointIn(mp, r.del) {
					deletePrefabAt(s, i)
					return ActionNone
				}
				if pointIn(mp, r.load) || pointIn(mp, r.row) {
					loadPrefabAt(s, i)
					return ActionNone
				}
			}
		}
	}
	return ActionNone
}

func savePrefabFromClipboard(s *State) {
	if s.clipboard.Empty() {
		s.flash("Copy a region first (Select tool + Ctrl+C), then save it as a prefab")
		return
	}
	if strings.TrimSpace(s.prefabName) == "" {
		s.flash("Prefab name required")
		return
	}
	if err := savePrefab(s.prefabName, s.clipboard, s.clipEntities); err != nil {
		s.flashWarn("Save prefab failed: " + err.Error())
		return
	}
	s.prefabPaths = listPrefabs()
	s.flash("Saved prefab " + s.prefabName)
}

func loadPrefabAt(s *State, i int) {
	if i < 0 || i >= len(s.prefabPaths) {
		return
	}
	region, ents, err := loadPrefab(s.prefabPaths[i])
	if err != nil {
		s.flashWarn("Load prefab failed: " + err.Error())
		return
	}
	s.clipboard = region
	s.clipEntities = ents
	s.flash("Loaded prefab " + prefabIDFromPath(s.prefabPaths[i]) + " — Ctrl+V to stamp")
	closeModal(s)
}

func deletePrefabAt(s *State, i int) {
	if i < 0 || i >= len(s.prefabPaths) {
		return
	}
	path := s.prefabPaths[i]
	if !armOrConfirmDelete(s, "prefab:"+path, "Delete prefab "+prefabIDFromPath(path)+"? Click × again to confirm") {
		return
	}
	if err := os.Remove(path); err != nil {
		s.flashWarn("Delete failed: " + err.Error())
		return
	}
	s.prefabPaths = listPrefabs()
	s.prefabCursor = clampCursor(s.prefabCursor, len(s.prefabPaths))
	s.flash("Deleted prefab " + prefabIDFromPath(path))
}

func drawPrefabsModal(s *State, font rl.Font, theme render.Theme) {
	l := prefabLayoutFor(s)
	drawModalHeaderAt(font, theme, l.card, "PREFABS", theme.BorderActive)

	drawLabel(font, "Name (saves the current clipboard)", labelAbove(l.nameField))
	drawTextField(font, l.nameField, s.prefabName, s.prefabNameFocus)
	saveEnabled := !s.clipboard.Empty()
	if saveEnabled {
		drawButton(font, l.saveBtn, "Save", false)
	} else {
		drawButtonDisabled(font, l.saveBtn, "Save")
	}

	if len(s.prefabPaths) == 0 {
		render.DrawRichText(font, "No prefabs yet — copy a region, name it, and Save.",
			rl.NewVector2(l.card.X+modalContentInset, l.saveBtn.Y+textFieldH+16), editorFontHint, 1, theme.TextHint)
	}
	for i := l.top; i < l.end; i++ {
		r := l.rows[i-l.top]
		render.DrawRichText(font, prefabIDFromPath(s.prefabPaths[i]),
			rl.NewVector2(r.row.X+6, r.row.Y+4), editorFontBody, 1, theme.TextPrimary)
		drawButton(font, r.load, "Load", false)
		drawButton(font, r.del, "X", s.deleteArmed == "prefab:"+s.prefabPaths[i])
	}
	drawModalFooterHint(font, l.card, "Load drops a prefab on the clipboard · Esc close", theme)
}
