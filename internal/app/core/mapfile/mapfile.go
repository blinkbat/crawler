// Package mapfile is the on-disk representation of an explorable area. The
// format is plain text — header lines, then a literal ASCII grid for the
// layout, then a list of enemy spawns — chosen so a map diffs cleanly in git
// and can be glanced at in any editor without parsing.
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

// MapFile round-trips losslessly with Encode/Parse. Layout rows use the tile
// chars defined in core/map.go (. # T X O B).
type MapFile struct {
	Name      string
	Materials string
	Quiet     string
	Width     int
	Height    int
	StartX    int
	StartZ    int
	StartFace string
	Layout    []string
	Enemies   []MapEnemy
}

type MapEnemy struct {
	Kind string
	X    int
	Z    int
}

// Parse reads a .map file from r. Errors pinpoint the first malformed line.
func Parse(r io.Reader) (MapFile, error) {
	mf := MapFile{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	state := "header"
	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := sc.Text()
		// Tolerate CRLF (e.g. a .map opened and resaved in Notepad). Scanner
		// strips \n but leaves \r; that would push layout rows one char wider
		// than declared and fail validation with a confusing message.
		raw = strings.TrimRight(raw, "\r")
		switch state {
		case "header":
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			if line == "layout:" {
				state = "layout"
				continue
			}
			if line == "enemies:" {
				state = "enemies"
				continue
			}
			key, val, ok := splitKV(line)
			if !ok {
				return mf, fmt.Errorf("line %d: expected 'key: value' or section header, got %q", lineNo, raw)
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
					return mf, fmt.Errorf("line %d: %w", lineNo, err)
				}
				mf.Width, mf.Height = w, h
			case "start":
				x, z, face, err := parseStart(val)
				if err != nil {
					return mf, fmt.Errorf("line %d: %w", lineNo, err)
				}
				mf.StartX, mf.StartZ, mf.StartFace = x, z, face
			default:
				return mf, fmt.Errorf("line %d: unknown header key %q", lineNo, key)
			}
		case "layout":
			if strings.TrimSpace(raw) == "enemies:" {
				state = "enemies"
				continue
			}
			if len(mf.Layout) >= mf.Height {
				if strings.TrimSpace(raw) == "" {
					continue
				}
			}
			mf.Layout = append(mf.Layout, raw)
		case "enemies":
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) != 3 {
				return mf, fmt.Errorf("line %d: expected '<kind> <x> <z>', got %q", lineNo, raw)
			}
			x, err := strconv.Atoi(fields[1])
			if err != nil {
				return mf, fmt.Errorf("line %d: bad enemy x %q", lineNo, fields[1])
			}
			z, err := strconv.Atoi(fields[2])
			if err != nil {
				return mf, fmt.Errorf("line %d: bad enemy z %q", lineNo, fields[2])
			}
			mf.Enemies = append(mf.Enemies, MapEnemy{Kind: fields[0], X: x, Z: z})
		}
	}
	if err := sc.Err(); err != nil {
		return mf, err
	}
	if err := mf.validate(); err != nil {
		return mf, err
	}
	return mf, nil
}

func (mf MapFile) validate() error {
	if mf.Width <= 0 || mf.Height <= 0 {
		return fmt.Errorf("size must be >0x0; got %dx%d", mf.Width, mf.Height)
	}
	if len(mf.Layout) != mf.Height {
		return fmt.Errorf("layout has %d rows, size declares %d", len(mf.Layout), mf.Height)
	}
	for i, row := range mf.Layout {
		if len(row) != mf.Width {
			return fmt.Errorf("layout row %d has %d cols, size declares %d", i, len(row), mf.Width)
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

// Encode writes mf in the canonical .map format.
func (mf MapFile) Encode(w io.Writer) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "name: %s\n", mf.Name)
	fmt.Fprintf(bw, "materials: %s\n", mf.Materials)
	fmt.Fprintf(bw, "quiet: %s\n", mf.Quiet)
	fmt.Fprintf(bw, "size: %dx%d\n", mf.Width, mf.Height)
	fmt.Fprintf(bw, "start: %d %d %s\n", mf.StartX, mf.StartZ, mf.StartFace)
	fmt.Fprintln(bw, "layout:")
	for _, row := range mf.Layout {
		fmt.Fprintln(bw, row)
	}
	fmt.Fprintln(bw, "enemies:")
	for _, e := range mf.Enemies {
		fmt.Fprintf(bw, "%s %d %d\n", e.Kind, e.X, e.Z)
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
