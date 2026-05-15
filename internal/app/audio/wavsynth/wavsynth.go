// Package wavsynth holds the pure procedural-audio helpers — sine sweeps,
// chord sums, two-note chimes, and a canonical 16-bit-mono-PCM WAV header
// builder. Split out from the parent audio package so the helpers can be
// unit-tested without pulling raylib into the test binary's load path
// (raylib's purego init opens raylib.dll at package init time, which isn't
// available from `go test`'s temp build directory on Windows).
//
// The audio package consumes this via SynthSweep / SynthChord / SynthChime,
// wraps the raw PCM through BuildWAV, and hands the bytes to
// rl.LoadWaveFromMemory.
package wavsynth

import (
	"bytes"
	"encoding/binary"
	"math"
)

// SampleRate is the procedural-audio bank's working sample rate. 22050 is
// a reasonable middle ground: low enough that each cue stays under a few
// KB once encoded, high enough that the sweep transitions don't audibly
// alias on the upper harmonics.
const SampleRate = 22050

// ClampToInt16 converts a float in [-1, 1] to a 16-bit PCM sample with
// soft clipping. Out-of-range floats get pinned to the int16 endpoints
// rather than wrapping — wrapping would produce harsh "ring modulator"
// artifacts instead of clean clipping.
func ClampToInt16(sample float64) int16 {
	if sample > 1.0 {
		sample = 1.0
	}
	if sample < -1.0 {
		sample = -1.0
	}
	return int16(sample * 32767)
}

// SynthSweep builds a sine-wave note that linearly sweeps between two
// frequencies. The envelope is a simple ADSR with the attack and release
// args; the sustain is whatever's left after attack and release fit inside
// the total duration. Volume is multiplied into the sample before
// clamping, so 0.2 lands around -14 dBFS — quiet enough for a UI cue.
func SynthSweep(duration, startHz, endHz, volume, attack, release float64) []int16 {
	samples := int(duration * SampleRate)
	if samples <= 0 {
		samples = 1
	}
	pcm := make([]int16, samples)
	phase := 0.0
	for i := 0; i < samples; i++ {
		t := float64(i) / float64(samples)
		freq := startHz + (endHz-startHz)*t
		// Integrate frequency over time so the wave stays continuous even
		// as the freq sweeps — otherwise the phase jumps and you hear
		// clicks at the sample boundaries.
		phase += 2 * math.Pi * freq / float64(SampleRate)
		sample := math.Sin(phase)
		// ADSR envelope.
		secs := float64(i) / float64(SampleRate)
		env := 1.0
		switch {
		case secs < attack:
			env = secs / attack
		case secs > duration-release:
			env = (duration - secs) / release
			if env < 0 {
				env = 0
			}
		}
		pcm[i] = ClampToInt16(sample * env * volume)
	}
	return pcm
}

// SynthChord sums several sine waves at the given frequencies into one
// note, then applies a bell-shaped envelope. Used by the "Great" cue
// where stacked harmonics give a richer ring than a single sine.
func SynthChord(duration float64, freqs []float64, volume float64) []int16 {
	samples := int(duration * SampleRate)
	if samples <= 0 || len(freqs) == 0 {
		return SynthSweep(duration, 440, 440, volume, 0.005, 0.02)
	}
	pcm := make([]int16, samples)
	phases := make([]float64, len(freqs))
	for i := 0; i < samples; i++ {
		t := float64(i) / float64(samples)
		sum := 0.0
		for k, freq := range freqs {
			phases[k] += 2 * math.Pi * freq / float64(SampleRate)
			sum += math.Sin(phases[k])
		}
		sum /= float64(len(freqs))
		// Bell envelope: sin(pi*t) — zero at both ends, peaks at t=0.5.
		env := math.Sin(math.Pi * t)
		pcm[i] = ClampToInt16(sum * env * volume)
	}
	return pcm
}

// SynthChime plays two sweep notes back-to-back, the first at firstHz then
// the second at secondHz. Each note runs for noteDuration seconds. Used
// by the heal cue so the two-tone "ding-ding" reads clearly.
func SynthChime(noteDuration, firstHz, secondHz, volume float64) []int16 {
	samplesPerNote := int(noteDuration * SampleRate)
	if samplesPerNote <= 0 {
		samplesPerNote = 1
	}
	total := samplesPerNote * 2
	pcm := make([]int16, total)
	for note := 0; note < 2; note++ {
		freq := firstHz
		if note == 1 {
			freq = secondHz
		}
		phase := 0.0
		for i := 0; i < samplesPerNote; i++ {
			phase += 2 * math.Pi * freq / float64(SampleRate)
			t := float64(i) / float64(samplesPerNote)
			env := math.Sin(math.Pi * t)
			pcm[note*samplesPerNote+i] = ClampToInt16(math.Sin(phase) * env * volume)
		}
	}
	return pcm
}

// BuildWAV writes a canonical 16-bit mono PCM WAV file into a byte slice.
// Format: RIFF header → fmt subchunk (PCM=1, mono, rate, byterate,
// blockalign, bitspersample) → data subchunk. Hand-off into raylib's
// LoadWaveFromMemory.
func BuildWAV(pcm []int16, rate int) []byte {
	dataSize := len(pcm) * 2
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))     // fmt chunk size
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))      // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))      // channels — mono
	_ = binary.Write(&buf, binary.LittleEndian, uint32(rate))   // sample rate
	_ = binary.Write(&buf, binary.LittleEndian, uint32(rate*2)) // byte rate (rate * channels * bytesPerSample)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))      // block align
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))     // bits per sample
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataSize))
	_ = binary.Write(&buf, binary.LittleEndian, pcm)
	return buf.Bytes()
}
