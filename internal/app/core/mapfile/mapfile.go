// Package mapfile is the on-disk representation of an explorable area. The
// format is plain text, multi-section: header lines, then one ASCII grid
// per editing layer (walls / floor / decor / props), then a list of enemy
// spawns. Chosen so a map diffs cleanly in git, can be glanced at in any
// editor without parsing, and gives the editor's layer system a 1:1
// mapping with on-disk structure.
//
// Layer character conventions:
//
//	walls  : '.' open, '#' wall
//	floor  : '.' auto-variant (per-tile hash), 'g' grass, 'd' dirt,
//	         'k' dark grass, 's' stone
//	decor  : '.' auto-scatter, '_' force-empty, 'b' bush, 'm' mushroom,
//	         'p' pebble cluster
//	props  : '.' empty, 'T' tree, 'X' tree XL, 'O' boulder, 'B' bush (large)
package mapfile

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// MapFile round-trips losslessly with Encode/Parse.
type MapFile struct {
	Name      string
	Materials string
	Quiet     string
	Width     int
	Height    int
	StartX    int
	StartZ    int
	StartFace string
	Walls     []string
	Floor     []string
	Decor     []string
	Props     []string
	Packs     []MapPack
}

// MapPack is one authored pack at a tile. Members is a non-empty list of
// enemy-kind names; on-disk format is "kind[,kind...] X Z" so a single-
// member pack reads the same as the legacy "kind X Z" form.
type MapPack struct {
	Members []string
	X       int
	Z       int
}

// layerSlot is the parser's notion of "which grid is the upcoming N rows
// going into." Lets the section dispatch share one collection loop.
type layerSlot int

const (
	slotNone layerSlot = iota
	slotWalls
	slotFloor
	slotDecor
	slotProps
	slotEnemies
)

// Parse reads a .map file from r. Errors pinpoint the first malformed line.
func Parse(r io.Reader) (MapFile, error) {
	mf := MapFile{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	state := slotNone
	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := strings.TrimRight(sc.Text(), "\r")

		// Section headers can appear at any point and switch state. Always
		// check before treating a line as content.
		if next, ok := sectionFor(raw); ok {
			state = next
			continue
		}

		if state == slotNone {
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			if err := parseHeaderLine(&mf, line, lineNo); err != nil {
				return mf, err
			}
			continue
		}

		if state == slotEnemies {
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) != 3 {
				return mf, fmt.Errorf("line %d: expected '<kind[,kind...]> <x> <z>', got %q", lineNo, raw)
			}
			members := strings.Split(fields[0], ",")
			for i, m := range members {
				m = strings.TrimSpace(m)
				if m == "" {
					return mf, fmt.Errorf("line %d: empty pack member at position %d", lineNo, i)
				}
				members[i] = m
			}
			x, err := strconv.Atoi(fields[1])
			if err != nil {
				return mf, fmt.Errorf("line %d: bad pack x %q", lineNo, fields[1])
			}
			z, err := strconv.Atoi(fields[2])
			if err != nil {
				return mf, fmt.Errorf("line %d: bad pack z %q", lineNo, fields[2])
			}
			mf.Packs = append(mf.Packs, MapPack{Members: members, X: x, Z: z})
			continue
		}

		// Layer grid line. Once Height rows are collected, blank lines are
		// tolerated (some editors auto-insert one before the next section
		// header) but a non-blank overflow row is a structural error — the
		// validator would catch it later, but reporting it on the offending
		// line gives a better diagnostic.
		target := layerSlice(&mf, state)
		if target == nil {
			continue
		}
		if len(*target) >= mf.Height {
			if strings.TrimSpace(raw) == "" {
				continue
			}
			return mf, fmt.Errorf("line %d: extra row past declared height %d", lineNo, mf.Height)
		}
		*target = append(*target, raw)
	}
	if err := sc.Err(); err != nil {
		return mf, err
	}
	if err := mf.validate(); err != nil {
		return mf, err
	}
	return mf, nil
}

func sectionFor(raw string) (layerSlot, bool) {
	switch strings.TrimSpace(raw) {
	case "walls:":
		return slotWalls, true
	case "floor:":
		return slotFloor, true
	case "decor:":
		return slotDecor, true
	case "props:":
		return slotProps, true
	case "enemies:":
		return slotEnemies, true
	}
	return slotNone, false
}

func layerSlice(mf *MapFile, slot layerSlot) *[]string {
	switch slot {
	case slotWalls:
		return &mf.Walls
	case slotFloor:
		return &mf.Floor
	case slotDecor:
		return &mf.Decor
	case slotProps:
		return &mf.Props
	}
	return nil
}

func parseHeaderLine(mf *MapFile, line string, lineNo int) error {
	key, val, ok := splitKV(line)
	if !ok {
		return fmt.Errorf("line %d: expected 'key: value' or section header, got %q", lineNo, line)
	}
	switch key {
	case "name":
		mf.Name = val
	case "materials":
		mf.Materials = val
	case "quiet":
		mf.Quiet = val
	case "size":
		w, h, err := parseSize(val)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		mf.Width, mf.Height = w, h
	case "start":
		x, z, face, err := parseStart(val)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		mf.StartX, mf.StartZ, mf.StartFace = x, z, face
	default:
		return fmt.Errorf("line %d: unknown header key %q", lineNo, key)
	}
	return nil
}

func (mf MapFile) validate() error {
	if mf.Width <= 0 || mf.Height <= 0 {
		return fmt.Errorf("size must be >0x0; got %dx%d", mf.Width, mf.Height)
	}
	for _, layer := range []struct {
		name string
		rows []string
	}{
		{"walls", mf.Walls},
		{"floor", mf.Floor},
		{"decor", mf.Decor},
		{"props", mf.Props},
	} {
		if len(layer.rows) != mf.Height {
			return fmt.Errorf("%s layer has %d rows, size declares %d", layer.name, len(layer.rows), mf.Height)
		}
		for i, row := range layer.rows {
			if len(row) != mf.Width {
				return fmt.Errorf("%s layer row %d has %d cols, size declares %d", layer.name, i, len(row), mf.Width)
			}
		}
	}
	if mf.StartX < 0 || mf.StartX >= mf.Width || mf.StartZ < 0 || mf.StartZ >= mf.Height {
		return fmt.Errorf("start (%d,%d) outside map", mf.StartX, mf.StartZ)
	}
	return nil
}

func splitKV(line string) (string, string, bool) {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
}

func parseSize(val string) (int, int, error) {
	parts := strings.SplitN(val, "x", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("size must be WxH, got %q", val)
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("size must be WxH integers, got %q", val)
	}
	return w, h, nil
}

func parseStart(val string) (int, int, string, error) {
	fields := strings.Fields(val)
	if len(fields) != 3 {
		return 0, 0, "", fmt.Errorf("start must be 'X Z facing', got %q", val)
	}
	x, err1 := strconv.Atoi(fields[0])
	z, err2 := strconv.Atoi(fields[1])
	if err1 != nil || err2 != nil {
		return 0, 0, "", fmt.Errorf("start coordinates must be integers, got %q", val)
	}
	face := strings.ToLower(fields[2])
	switch face {
	case "north", "east", "south", "west":
	default:
		return 0, 0, "", fmt.Errorf("start facing must be north/east/south/west, got %q", fields[2])
	}
	return x, z, face, nil
}

// Encode writes mf in the canonical .map format. Layers are emitted in a
// fixed order so encoded maps diff cleanly across edits.
func (mf MapFile) Encode(w io.Writer) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "name: %s\n", mf.Name)
	fmt.Fprintf(bw, "materials: %s\n", mf.Materials)
	fmt.Fprintf(bw, "quiet: %s\n", mf.Quiet)
	fmt.Fprintf(bw, "size: %dx%d\n", mf.Width, mf.Height)
	fmt.Fprintf(bw, "start: %d %d %s\n", mf.StartX, mf.StartZ, mf.StartFace)
	for _, layer := range []struct {
		name string
		rows []string
	}{
		{"walls", mf.Walls},
		{"floor", mf.Floor},
		{"decor", mf.Decor},
		{"props", mf.Props},
	} {
		fmt.Fprintf(bw, "%s:\n", layer.name)
		for _, row := range layer.rows {
			fmt.Fprintln(bw, row)
		}
	}
	fmt.Fprintln(bw, "enemies:")
	for _, p := range mf.Packs {
		// Single-member packs encode the same as the legacy "kind X Z" line
		// so maps without grouped packs stay byte-identical across the
		// format change.
		fmt.Fprintf(bw, "%s %d %d\n", strings.Join(p.Members, ","), p.X, p.Z)
	}
	return bw.Flush()
}

func Load(path string) (MapFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return MapFile{}, err
	}
	defer f.Close()
	return Parse(f)
}

func Save(path string, mf MapFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return mf.Encode(f)
}

// List returns the .map files in dir, sorted alphabetically. Missing dir
// returns an empty slice (first-run convenience).
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".map") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

// BlankLayer returns a Width × Height grid filled with `c`. Editor uses it
// to seed fresh layers when creating a new map or resizing an existing one.
func BlankLayer(width, height int, c byte) []string {
	rows := make([]string, height)
	row := strings.Repeat(string(c), width)
	for i := range rows {
		rows[i] = row
	}
	return rows
}
