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
	"math/rand"
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

// WaveShape selects the oscillator timbre for SynthShape. Sine is the
// round/pure default; Square is harsh/8-bit; Triangle is gentler than
// square but brighter than sine; Saw is buzzy/aggressive. Different
// shapes share the same fundamental frequency but emit very different
// harmonic content, which is the biggest single lever for "cool
// noise" variety in the sound editor.
type WaveShape int

const (
	WaveSine WaveShape = iota
	WaveSquare
	WaveTriangle
	WaveSaw
	waveShapeCount
)

// WaveShapeName returns a human label for a WaveShape value — used
// by the sound-editor row that exposes the shape picker.
func WaveShapeName(w WaveShape) string {
	switch w {
	case WaveSquare:
		return "Square"
	case WaveTriangle:
		return "Triangle"
	case WaveSaw:
		return "Saw"
	default:
		return "Sine"
	}
}

// SynthShape is the rich procedural sweep primitive. Generalises
// SynthSweep with:
//
//   - Selectable oscillator (sine / square / triangle / saw).
//   - Optional noise mix: blends in white-noise samples for grit /
//     wind / static textures. 0 = pure tone, 1 = pure noise.
//   - Optional vibrato: sinusoidal frequency modulation. vibHz sets
//     the wobble rate, vibDepth is the fraction of the base
//     frequency the wobble swings through (0..0.5 reads as natural
//     vibrato; higher gets sci-fi).
//
// All extras default to "off" — a zero-everything call to SynthShape
// produces the same output as SynthSweep with sine + no noise + no
// vibrato. SynthSweep itself is now a thin wrapper.
func SynthShape(duration, startHz, endHz, volume, attack, release float64,
	wave WaveShape, noiseMix, vibHz, vibDepth float64) []int16 {

	samples := int(duration * SampleRate)
	if samples <= 0 {
		samples = 1
	}
	if noiseMix < 0 {
		noiseMix = 0
	}
	if noiseMix > 1 {
		noiseMix = 1
	}
	if vibDepth < 0 {
		vibDepth = 0
	}
	pcm := make([]int16, samples)
	phase := 0.0
	vibPhase := 0.0
	// Deterministic noise seed so two consecutive previews of the
	// same params produce identical waveforms — important for the
	// editor's "did my slider change anything?" feedback loop.
	noiseRng := rand.New(rand.NewSource(0xC0FFEE_BABE))
	for i := 0; i < samples; i++ {
		t := float64(i) / float64(samples)
		freq := startHz + (endHz-startHz)*t
		// Vibrato — sinusoidal FM. Phase-integrated like the main
		// oscillator so the wobble doesn't click at sample edges.
		if vibHz > 0 && vibDepth > 0 {
			vibPhase += 2 * math.Pi * vibHz / float64(SampleRate)
			freq += freq * vibDepth * math.Sin(vibPhase)
		}
		phase += 2 * math.Pi * freq / float64(SampleRate)
		// Oscillator. Phase is unbounded, so we wrap to [0, 2π) for
		// the shaped waves that read the position rather than the
		// running sine.
		var tone float64
		switch wave {
		case WaveSquare:
			if math.Sin(phase) >= 0 {
				tone = 1.0
			} else {
				tone = -1.0
			}
		case WaveTriangle:
			p := math.Mod(phase, 2*math.Pi)
			if p < 0 {
				p += 2 * math.Pi
			}
			tone = 1.0 - 2.0*math.Abs(p-math.Pi)/math.Pi
		case WaveSaw:
			p := math.Mod(phase, 2*math.Pi)
			if p < 0 {
				p += 2 * math.Pi
			}
			tone = (p - math.Pi) / math.Pi
		default:
			tone = math.Sin(phase)
		}
		// Noise mix — crossfade between tone and noise. At 1.0 the
		// tone disappears, leaving pure white noise (still
		// envelope-shaped). At 0.0 the noise contributes nothing.
		if noiseMix > 0 {
			noise := noiseRng.Float64()*2 - 1
			tone = tone*(1-noiseMix) + noise*noiseMix
		}
		// ADSR envelope (same as the original SynthSweep).
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
		pcm[i] = ClampToInt16(tone * env * volume)
	}
	return pcm
}

// SynthSweep is the backward-compat wrapper: sine wave, no noise,
// no vibrato. Existing callers (the procedural soundCues defaults,
// older tests) still drive this and get exactly the same output as
// before SynthShape landed.
func SynthSweep(duration, startHz, endHz, volume, attack, release float64) []int16 {
	return SynthShape(duration, startHz, endHz, volume, attack, release, WaveSine, 0, 0, 0)
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

// SynthClick generates a short percussive transient — a pitched sine
// body that drops in frequency over the note's lifetime, blended with
// a white-noise burst for the "click" texture, under a hard-attack +
// exponential-decay envelope. The shape covers both "tick" (high
// pitchHz, large noise mix) and "thud" (low pitchHz, mostly sine)
// without needing two separate generators.
//
// Arguments:
//
//	duration   total seconds (typical 0.02 – 0.08 — past ~0.1 the
//	           ear stops parsing it as a transient)
//	pitchHz    fundamental of the pitched body at note start
//	pitchDrop  fraction of pitchHz to slide DOWN linearly over the
//	           note's life. 0 = no slide; 0.7 = ends at 0.3 * pitchHz.
//	           Larger values produce the "kick drum" pitch-pop.
//	noise      [0, 1] mix of white noise added to the sine
//	volume     peak scale [0, 1]
//
// The noise generator is seeded with a fixed value so the same call
// produces the same waveform every run — important for the bank's
// procedural defaults, which the user expects to sound identical
// across sessions. Callers that want variation should sum two clicks
// with different params rather than re-seed.
func SynthClick(duration, pitchHz, pitchDrop, noise, volume float64) []int16 {
	samples := int(duration * SampleRate)
	if samples <= 0 {
		samples = 1
	}
	pcm := make([]int16, samples)
	// Fixed seed: every call produces the same waveform. The bank
	// expects deterministic procedural defaults so the "Sounds" modal
	// preview matches what plays in battle.
	rng := rand.New(rand.NewSource(1))
	phase := 0.0
	// Hard attack: first attackSamples ramps from 0 → 1, then exponential
	// decay across the rest. 2ms attack keeps the transient crisp without
	// the click-at-zero artifact a pure-square envelope would produce.
	attackSamples := int(math.Round(0.002 * float64(SampleRate)))
	if attackSamples < 1 {
		attackSamples = 1
	}
	if attackSamples > samples {
		attackSamples = samples
	}
	// Decay constant tuned so the envelope reaches ~3% by note end —
	// past that the residual is below the noise floor for a short cue.
	decayK := -3.5
	for i := 0; i < samples; i++ {
		t := float64(i) / float64(samples)
		freq := pitchHz * (1 - pitchDrop*t)
		if freq < 0 {
			freq = 0
		}
		phase += 2 * math.Pi * freq / float64(SampleRate)
		sine := math.Sin(phase)
		// White noise in [-1, 1]; rand.Float64() is [0, 1).
		n := rng.Float64()*2 - 1
		sample := sine*(1-noise) + n*noise
		var env float64
		if i < attackSamples {
			env = float64(i) / float64(attackSamples)
		} else {
			// Exponential decay anchored at the end of the attack ramp.
			td := float64(i-attackSamples) / float64(samples-attackSamples)
			env = math.Exp(decayK * td)
		}
		pcm[i] = ClampToInt16(sample * env * volume)
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

// WAV format constants — every WAV this package writes is mono, 16-bit
// PCM. Named so the header fields below derive from one place instead of
// repeating bare 1 / 2 / 16 literals whose relationship (blockAlign =
// channels × bytesPerSample, byteRate = rate × blockAlign) was implicit.
const (
	wavChannels       = 1
	wavBitsPerSample  = 16
	wavBytesPerSample = wavBitsPerSample / 8
	wavBlockAlign     = wavChannels * wavBytesPerSample
)

// BuildWAV writes a canonical 16-bit mono PCM WAV file into a byte slice.
// Format: RIFF header → fmt subchunk (PCM=1, mono, rate, byterate,
// blockalign, bitspersample) → data subchunk. Hand-off into raylib's
// LoadWaveFromMemory.
func BuildWAV(pcm []int16, rate int) []byte {
	dataSize := len(pcm) * wavBytesPerSample
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))                  // fmt chunk size
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))                   // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(wavChannels))         // channels — mono
	_ = binary.Write(&buf, binary.LittleEndian, uint32(rate))                // sample rate
	_ = binary.Write(&buf, binary.LittleEndian, uint32(rate*wavBlockAlign))  // byte rate (rate × blockAlign)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(wavBlockAlign))       // block align
	_ = binary.Write(&buf, binary.LittleEndian, uint16(wavBitsPerSample))    // bits per sample
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataSize))
	_ = binary.Write(&buf, binary.LittleEndian, pcm)
	return buf.Bytes()
}
