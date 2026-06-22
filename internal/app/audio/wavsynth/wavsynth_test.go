package wavsynth

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// TestBuildWAV_HeaderLayout pins the WAV byte structure (RIFF/WAVE/fmt/data).
// Field drift silently breaks raylib's LoadWaveFromMemory — Init and Play both
// succeed even on a malformed wave.
func TestBuildWAV_HeaderLayout(t *testing.T) {
	samples := []int16{0, 100, -100, 32767, -32768, 0}
	out := BuildWAV(samples, 22050)

	// 44 header bytes + 2 per sample.
	wantLen := 44 + len(samples)*2
	if len(out) != wantLen {
		t.Fatalf("WAV total length = %d, want %d", len(out), wantLen)
	}

	if string(out[0:4]) != "RIFF" {
		t.Errorf("missing RIFF magic, got %q", out[0:4])
	}
	// File-size field [4:8] = 36 + dataSize.
	fileSize := binary.LittleEndian.Uint32(out[4:8])
	wantFileSize := uint32(36 + len(samples)*2)
	if fileSize != wantFileSize {
		t.Errorf("RIFF file size = %d, want %d", fileSize, wantFileSize)
	}
	if string(out[8:12]) != "WAVE" {
		t.Errorf("missing WAVE magic, got %q", out[8:12])
	}
	if string(out[12:16]) != "fmt " {
		t.Errorf("missing 'fmt ' subchunk, got %q", out[12:16])
	}
	if got := binary.LittleEndian.Uint32(out[16:20]); got != 16 {
		t.Errorf("fmt subchunk size = %d, want 16", got)
	}
	if got := binary.LittleEndian.Uint16(out[20:22]); got != 1 {
		t.Errorf("audio format = %d, want 1 (PCM)", got)
	}
	if got := binary.LittleEndian.Uint16(out[22:24]); got != 1 {
		t.Errorf("channels = %d, want 1 (mono)", got)
	}
	if got := binary.LittleEndian.Uint32(out[24:28]); got != 22050 {
		t.Errorf("sample rate = %d, want 22050", got)
	}
	if got := binary.LittleEndian.Uint32(out[28:32]); got != 22050*2 {
		t.Errorf("byte rate = %d, want %d", got, 22050*2)
	}
	if got := binary.LittleEndian.Uint16(out[32:34]); got != 2 {
		t.Errorf("block align = %d, want 2", got)
	}
	if got := binary.LittleEndian.Uint16(out[34:36]); got != 16 {
		t.Errorf("bits per sample = %d, want 16", got)
	}
	if string(out[36:40]) != "data" {
		t.Errorf("missing 'data' subchunk, got %q", out[36:40])
	}
	if got := binary.LittleEndian.Uint32(out[40:44]); got != uint32(len(samples)*2) {
		t.Errorf("data subchunk size = %d, want %d", got, len(samples)*2)
	}

	// Payload round-trips exactly.
	rdr := bytes.NewReader(out[44:])
	for i, want := range samples {
		var got int16
		if err := binary.Read(rdr, binary.LittleEndian, &got); err != nil {
			t.Fatalf("reading sample %d: %v", i, err)
		}
		if got != want {
			t.Errorf("sample %d = %d, want %d", i, got, want)
		}
	}
}

func TestBuildWAV_EmptySamples(t *testing.T) {
	out := BuildWAV(nil, 22050)
	if len(out) != 44 {
		t.Fatalf("empty samples should yield 44-byte header, got %d", len(out))
	}
	if got := binary.LittleEndian.Uint32(out[40:44]); got != 0 {
		t.Errorf("empty data chunk size = %d, want 0", got)
	}
}

// TestClampToInt16_Bounds catches soft-clip: past ±1 pins to the int16
// endpoints rather than wrapping (a wrap would cause ring-modulator glitches).
func TestClampToInt16_Bounds(t *testing.T) {
	cases := []struct {
		in   float64
		want int16
	}{
		{0, 0},
		{1.0, 32767},
		{-1.0, -32767},
		{2.5, 32767},   // clip up
		{-3.0, -32767}, // clip down
		{0.5, 16383},
	}
	for _, tc := range cases {
		if got := ClampToInt16(tc.in); got != tc.want {
			t.Errorf("ClampToInt16(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestSynthSweep_LengthMatchesDuration sanity-checks the sample-count math (100
// ms at 22050 Hz = 2205 samples). Variable, not const, so truncation matches
// the helper's runtime math.
func TestSynthSweep_LengthMatchesDuration(t *testing.T) {
	duration := 0.1
	pcm := SynthSweep(duration, 440, 880, 0.2, 0.01, 0.05)
	want := int(duration * float64(SampleRate))
	if len(pcm) != want {
		t.Errorf("SynthSweep length = %d, want %d", len(pcm), want)
	}
}

func TestSynthChord_LengthMatchesDuration(t *testing.T) {
	duration := 0.05
	pcm := SynthChord(duration, []float64{440, 660}, 0.2)
	want := int(duration * float64(SampleRate))
	if len(pcm) != want {
		t.Errorf("SynthChord length = %d, want %d", len(pcm), want)
	}
}

func TestSynthChime_TotalIsTwoNotes(t *testing.T) {
	duration := 0.04
	pcm := SynthChime(duration, 440, 660, 0.2)
	perNote := int(duration * float64(SampleRate))
	want := perNote * 2
	if len(pcm) != want {
		t.Errorf("SynthChime length = %d, want %d (two notes of %d samples)",
			len(pcm), want, perNote)
	}
}

// TestSynthShape_SineMatchesSynthSweep asserts SynthShape (sine, no noise/vib)
// is byte-identical to SynthSweep; any drift is a wrapper-layer bug.
func TestSynthShape_SineMatchesSynthSweep(t *testing.T) {
	duration, startHz, endHz, vol, atk, rel := 0.08, 440.0, 880.0, 0.20, 0.01, 0.04
	a := SynthSweep(duration, startHz, endHz, vol, atk, rel)
	b := SynthShape(duration, startHz, endHz, vol, atk, rel, WaveSine, 0, 0, 0)
	if len(a) != len(b) {
		t.Fatalf("length drift: SynthSweep=%d, SynthShape(sine)=%d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs: SynthSweep=%d, SynthShape(sine)=%d", i, a[i], b[i])
		}
	}
}

// TestSynthShape_SquareValuesAreBinary asserts the square path emits only two
// amplitude poles (no attack/release, so the ramps don't dilute the check).
func TestSynthShape_SquareValuesAreBinary(t *testing.T) {
	pcm := SynthShape(0.10, 440, 440, 0.5, 0, 0, WaveSquare, 0, 0, 0)
	if len(pcm) == 0 {
		t.Fatal("empty PCM")
	}
	// Constant freq + no attack/release should yield exactly ±16383.
	wantPos := ClampToInt16(0.5)
	wantNeg := ClampToInt16(-0.5)
	for i, s := range pcm {
		if s != wantPos && s != wantNeg {
			t.Fatalf("sample %d = %d, expected ±%d for square wave", i, s, wantPos)
		}
	}
}

// TestSynthShape_NoiseProducesVariance asserts the noise-mix path shifts output
// off the tonal baseline.
func TestSynthShape_NoiseProducesVariance(t *testing.T) {
	tone := SynthShape(0.05, 440, 440, 0.4, 0, 0, WaveSine, 0, 0, 0)
	noisy := SynthShape(0.05, 440, 440, 0.4, 0, 0, WaveSine, 1.0, 0, 0)
	if len(tone) != len(noisy) {
		t.Fatalf("length mismatch: tone=%d, noisy=%d", len(tone), len(noisy))
	}
	diffs := 0
	for i := range tone {
		if tone[i] != noisy[i] {
			diffs++
		}
	}
	if diffs < len(tone)/2 {
		t.Fatalf("expected noise to perturb majority of samples, got %d diffs of %d", diffs, len(tone))
	}
}

// TestSynthShape_VibratoModulatesFrequency asserts vibrato shifts output off
// the non-vibrato baseline at the same base frequency.
func TestSynthShape_VibratoModulatesFrequency(t *testing.T) {
	plain := SynthShape(0.10, 440, 440, 0.3, 0, 0, WaveSine, 0, 0, 0)
	vibrato := SynthShape(0.10, 440, 440, 0.3, 0, 0, WaveSine, 0, 8, 0.10)
	if len(plain) != len(vibrato) {
		t.Fatalf("length mismatch: plain=%d, vibrato=%d", len(plain), len(vibrato))
	}
	diffs := 0
	for i := range plain {
		if plain[i] != vibrato[i] {
			diffs++
		}
	}
	if diffs == 0 {
		t.Fatalf("vibrato should perturb output but plain and vibrato PCM matched sample-for-sample")
	}
}
