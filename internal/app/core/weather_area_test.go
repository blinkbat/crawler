package core

import (
	"bytes"
	"testing"

	"crawler/internal/app/core/mapfile"
)

// TestWeatherModeRoundTrip: an authored per-area weather mode survives the full
// Area → MapFile → encode → parse → Area round trip, and Auto stays the default
// (no weather: line written) so pre-weather maps are untouched.
func TestWeatherModeRoundTrip(t *testing.T) {
	base := AreaDefinition{
		Name:        "Weather",
		Width:       3,
		Height:      3,
		Walls:       []string{"...", "...", "..."},
		Floor:       []string{"ggg", "ggg", "ggg"},
		Decor:       []string{"...", "...", "..."},
		Props:       []string{"...", "...", "..."},
		Materials:   MaterialField,
		StartTileX:  1,
		StartTileZ:  1,
		StartFacing: East,
	}
	for _, mode := range []WeatherMode{WeatherModeAuto, WeatherModeClear, WeatherModeRain} {
		a := base
		a.WeatherMode = mode

		mf, err := MapFileFromArea(a)
		if err != nil {
			t.Fatalf("MapFileFromArea(%v): %v", mode, err)
		}
		var buf bytes.Buffer
		if err := mf.Encode(&buf); err != nil {
			t.Fatalf("encode(%v): %v", mode, err)
		}
		// Auto must not write a weather: line (byte-stable with legacy maps).
		if mode == WeatherModeAuto && bytes.Contains(buf.Bytes(), []byte("weather:")) {
			t.Errorf("Auto wrote a weather: line: %q", buf.String())
		}
		parsed, err := mapfile.Parse(&buf)
		if err != nil {
			t.Fatalf("parse(%v): %v", mode, err)
		}
		got, err := AreaFromMapFile(parsed, "x")
		if err != nil {
			t.Fatalf("AreaFromMapFile(%v): %v", mode, err)
		}
		if got.WeatherMode != mode {
			t.Errorf("round trip WeatherMode = %v, want %v", got.WeatherMode, mode)
		}
	}
}

// TestWeatherModeName pins the on-disk tokens (parse is total, Auto is empty).
func TestWeatherModeName(t *testing.T) {
	cases := map[WeatherMode]string{
		WeatherModeAuto:  "",
		WeatherModeClear: "clear",
		WeatherModeRain:  "rain",
	}
	for mode, name := range cases {
		if got := WeatherModeName(mode); got != name {
			t.Errorf("WeatherModeName(%v) = %q, want %q", mode, got, name)
		}
		if got := WeatherModeFromName(name); got != mode {
			t.Errorf("WeatherModeFromName(%q) = %v, want %v", name, got, mode)
		}
	}
	if WeatherModeFromName("auto") != WeatherModeAuto || WeatherModeFromName("bogus") != WeatherModeAuto {
		t.Error("unknown/auto tokens should map to WeatherModeAuto")
	}
}
