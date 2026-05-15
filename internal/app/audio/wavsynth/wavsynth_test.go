package wavsynth

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// TestBuildWAV_HeaderLayout pins the byte structure of the procedural WAV
// the audio package hands to raylib. The header is canonical 16-bit mono
// PCM — RIFF / WAVE / fmt / data — and any drift in field offsets or
// sizes will silently break LoadWaveFromMemory in raylib, producing
// either no sound or undefined audio glitches that aren't caught by the
// audio package's Init or Play paths (both succeed even with a malformed
// wave).
func TestBuildWAV_HeaderLayout(t *testing.T) {
	samples := []int16{0, 100, -100, 32767, -32768, 0}
	out := BuildWAV(samples, 22050)

	// Total length sanity: 44 header bytes + 2 bytes per sample.
	wantLen := 44 + len(samples)*2
	if len(out) != wantLen {
		t.Fatalf("WAV total length = %d, want %d", len(out), wantLen)
	}

	// RIFF magic.
	if string(out[0:4]) != "RIFF" {
		t.Errorf("missing RIFF magic, got %q", out[0:4])
	}
	// File-size field is at [4:8] and equals 36 + dataSize.
	fileSize := binary.LittleEndian.Uint32(out[4:8])
	wantFileSize := uint32(36 + len(samples)*2)
	if fileSize != wantFileSize {
		t.Errorf("RIFF file size = %d, want %d", fileSize, wantFileSize)
	}
	// WAVE magic.
	if string(out[8:12]) != "WAVE" {
		t.Errorf("missing WAVE magic, got %q", out[8:12])
	}
	// fmt subchunk header.
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
	// data subchunk header.
	if string(out[36:40]) != "data" {
		t.Errorf("missing 'data' subchunk, got %q", out[36:40])
	}
	if got := binary.LittleEndian.Uint32(out[40:44]); got != uint32(len(samples)*2) {
		t.Errorf("data subchunk size = %d, want %d", got, len(samples)*2)
	}

	// Sample payload should round-trip exactly.
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

// TestClampToInt16_Bounds catches the soft-clip behavior: anything past +/-1
// pins to the int16 endpoints rather than wrapping. A wrap here would
// produce "ring modulator" glitches instead of clean clipping, which is
// audible and obnoxious.
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

// TestSynthSweep_LengthMatchesDuration is a lightweight sanity check on
// the sample-count math — a 100 ms sweep at 22050 Hz should produce
// exactly 2205 samples. (Variable, not const, so the int truncation
// happens at runtime via the same math the synth helper uses, not at
// compile time where non-integer products are a compile error.)
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
