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
	"strconv"
)

// SampleRate is the procedural-audio bank's working sample rate. 22050 is
// a reasonable middle ground: low enough that each cue stays under a few
// KB once encoded, high enough that the sweep transitions don't audibly
// alias on the upper harmonics.
const SampleRate = 22050

// radiansPerSampleHz is the per-sample phase increment for a 1 Hz tone:
// multiply by a frequency to advance an oscillator's phase by one sample.
// Open-coded as `2 * math.Pi / float64(SampleRate)` in every synth loop
// before being named here.
const radiansPerSampleHz = 2 * math.Pi / float64(SampleRate)

// bellEnv is the bell-shaped amplitude envelope sin(pi*t) for t in [0, 1] —
// zero at both ends, peaks at t=0.5. Shared by the chord and chime cues so
// their ring-out shape stays identical.
func bellEnv(t float64) float64 {
	return math.Sin(math.Pi * t)
}

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

// WaveShapeCount is the number of WaveShape values — exported so the
// editor's wave picker derives its slider bound from the enum instead
// of hardcoding the highest index, and a fifth shape becomes reachable
// automatically.
const WaveShapeCount = int(waveShapeCount)

// waveShapeNames is the per-shape human label, indexed by WaveShape. A
// [waveShapeCount]string array (not a switch with a default) so a fifth
// shape leaves an empty slot the init assert catches — the editor's
// picker can't silently mislabel a new shape as "Sine".
var waveShapeNames = [waveShapeCount]string{
	WaveSine:     "Sine",
	WaveSquare:   "Square",
	WaveTriangle: "Triangle",
	WaveSaw:      "Saw",
}

func init() {
	for s := WaveShape(0); s < waveShapeCount; s++ {
		if waveShapeNames[s] == "" {
			panic("wavsynth: waveShapeNames missing label for a WaveShape")
		}
	}
}

// WaveShapeName returns a human label for a WaveShape value — used
// by the sound-editor row that exposes the shape picker. An out-of-range
// value (e.g. a stale persisted index) falls back to the Sine label.
func WaveShapeName(w WaveShape) string {
	if w < 0 || int(w) >= len(waveShapeNames) {
		return waveShapeNames[WaveSine]
	}
	return waveShapeNames[w]
}

// Musical note support. Lets the sound editor pick pitches as tempered
// notes instead of raw Hz, so authored cues can sit in tune with the
// procedural musical cues (the chord/chime bank). Equal temperament,
// A4 = 440 Hz. Index 0 = C2; NoteCount spans five octaves up to B6,
// covering the editor's pitch range musically.
const (
	noteA4Hz    = 440.0
	noteA4Index = 33 // semitones from C2 (index 0) up to A4: 2 octaves + 9 = 33
	// NoteCount is the number of addressable notes (C2..B6, five octaves).
	NoteCount = 60
)

var noteNames = [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

func clampNoteIndex(i int) int {
	if i < 0 {
		return 0
	}
	if i >= NoteCount {
		return NoteCount - 1
	}
	return i
}

// NoteHz returns the equal-tempered frequency of note index i (0 = C2).
func NoteHz(i int) float64 {
	i = clampNoteIndex(i)
	return noteA4Hz * math.Pow(2, float64(i-noteA4Index)/12.0)
}

// NoteName returns the scientific-pitch label of note index i, e.g.
// "A4", "C#5". Used by the editor's note-picker readout.
func NoteName(i int) string {
	i = clampNoteIndex(i)
	return noteNames[i%12] + strconv.Itoa(2+i/12)
}

// NearestNoteIndex returns the note index whose frequency is closest to
// hz — the inverse of NoteHz, used so the editor's note picker can show
// the right note for an arbitrary stored frequency (and round a freed-up
// Hz onto the tempered grid).
func NearestNoteIndex(hz float64) int {
	if hz <= 0 {
		return 0
	}
	return clampNoteIndex(int(math.Round(noteA4Index + 12*math.Log2(hz/noteA4Hz))))
}

// Voicing knobs for the FX params, named so the per-sample loop reads by
// intent and the slider ranges have one home for their "feel" scaling.
const (
	driveMaxGain   = 5.0  // Drive=1 → tanh pre-gain of 1+5 = 6× (heavy saturation)
	cutoffMinAlpha = 0.02 // floor on the one-pole LPF coefficient so Cutoff=0 stays a (very dark) tone, not silence
	crushMaxHold   = 24   // Crush=1 → sample-and-hold 25 samples (~882 Hz effective rate at 22050)
	maxDuration    = 30.0 // hard cap on a synth's length in seconds — sound effects are well under this; the ceiling only exists so a corrupt/hand-edited .snd sidecar can't drive a multi-GB make([]int16)
)

// ShapeParams is the full knob set for the procedural sound editor. It
// supersedes SynthShape's positional argument list; SynthShape is now a
// thin wrapper that fills the extra fields with their neutral (no-op)
// values, so existing callers and the golden tests are byte-unaffected.
//
// Neutral values (a "do nothing extra" sound): Decay 0, Sustain 1,
// PulseWidth 0.5, Cutoff 1 (filter open), Drive 0, Crush 0, Tremolo 0.
type ShapeParams struct {
	Duration     float64   `json:"duration"`      // seconds
	StartHz      float64   `json:"start_hz"`      // sweep start frequency
	EndHz        float64   `json:"end_hz"`        // sweep end frequency
	Volume       float64   `json:"volume"`        // peak amplitude [0,1]
	Attack       float64   `json:"attack"`        // ADSR attack, seconds
	Decay        float64   `json:"decay"`         // ADSR decay, seconds (0 = skip)
	Sustain      float64   `json:"sustain"`       // ADSR sustain level [0,1]
	Release      float64   `json:"release"`       // ADSR release, seconds
	Wave         WaveShape `json:"wave"`          // oscillator timbre
	PulseWidth   float64   `json:"pulse_width"`   // square duty cycle [0.01,0.99]; 0.5 = symmetric
	NoiseMix     float64   `json:"noise"`         // tone↔white-noise crossfade [0,1]
	VibHz        float64   `json:"vibrato_hz"`    // pitch-wobble rate
	VibDepth     float64   `json:"vibrato_depth"` // pitch-wobble depth (fraction of freq)
	TremoloHz    float64   `json:"tremolo_hz"`    // amplitude-wobble rate
	TremoloDepth float64   `json:"tremolo_depth"` // amplitude-wobble depth [0,1]
	Cutoff       float64   `json:"cutoff"`        // one-pole low-pass [0,1]; 1 = open (bypass)
	Drive        float64   `json:"drive"`         // soft-saturation amount [0,1]
	Crush        float64   `json:"crush"`         // sample-rate reduction [0,1]
}

// clamp01 pins a value into [0,1] — the common guard for the mix-style knobs.
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// adsrEnv computes the ADSR envelope level at elapsed time secs. With
// Decay 0 + Sustain 1 it reduces to the original attack/sustain/release
// shape SynthSweep used, so the neutral wrapper path is byte-identical.
func adsrEnv(secs, duration, attack, decay, sustain, release, releaseStart float64) float64 {
	switch {
	case attack > 0 && secs < attack:
		return secs / attack
	case decay > 0 && secs < attack+decay:
		return 1 - (1-sustain)*((secs-attack)/decay)
	case secs < releaseStart:
		return sustain
	default:
		if release <= 0 {
			return sustain
		}
		e := sustain * (duration - secs) / release
		if e < 0 {
			e = 0
		}
		return e
	}
}

// SynthShapeParams is the rich procedural sweep primitive. It generalises
// SynthSweep with a selectable oscillator (with pulse-width for the
// square), a full ADSR envelope, a tone↔noise crossfade, pitch vibrato +
// amplitude tremolo, a one-pole low-pass, soft-saturation drive, and a
// sample-rate-reduction bitcrush. The signal chain per sample is:
//
//	oscillator → noise mix → drive → low-pass → bitcrush → ×(ADSR·tremolo·volume)
func SynthShapeParams(p ShapeParams) []int16 {
	if p.Duration > maxDuration {
		p.Duration = maxDuration
	}
	samples := int(p.Duration * SampleRate)
	if samples <= 0 {
		samples = 1
	}
	noiseMix := clamp01(p.NoiseMix)
	sustain := clamp01(p.Sustain)
	tremDepth := clamp01(p.TremoloDepth)
	drive := clamp01(p.Drive)
	crush := clamp01(p.Crush)
	vibDepth := p.VibDepth
	if vibDepth < 0 {
		vibDepth = 0
	}
	pulse := p.PulseWidth
	if pulse < 0.01 {
		pulse = 0.01
	}
	if pulse > 0.99 {
		pulse = 0.99
	}
	holdLen := 1 + int(crush*crushMaxHold+0.5) // 1 = no crush
	releaseStart := p.Duration - p.Release

	pcm := make([]int16, samples)
	phase, vibPhase, tremPhase := 0.0, 0.0, 0.0
	// Deterministic noise seed so two consecutive previews of the same
	// params produce identical waveforms — important for the editor's
	// "did my slider change anything?" feedback loop.
	noiseRng := rand.New(rand.NewSource(0xC0FFEE_BABE))
	lpState := 0.0 // one-pole low-pass memory
	heldTone := 0.0
	holdCounter := 0
	for i := 0; i < samples; i++ {
		t := float64(i) / float64(samples)
		freq := p.StartHz + (p.EndHz-p.StartHz)*t
		// Vibrato — sinusoidal FM, phase-integrated so the wobble doesn't
		// click at sample edges.
		if p.VibHz > 0 && vibDepth > 0 {
			vibPhase += radiansPerSampleHz * p.VibHz
			freq += freq * vibDepth * math.Sin(vibPhase)
		}
		phase += radiansPerSampleHz * freq
		// Oscillator. Phase is unbounded, so the shaped waves wrap to
		// [0, 2π) (or [0,1) for the square's duty comparison).
		var tone float64
		switch p.Wave {
		case WaveSquare:
			pp := math.Mod(phase, 2*math.Pi)
			if pp < 0 {
				pp += 2 * math.Pi
			}
			if pp/(2*math.Pi) < pulse {
				tone = 1.0
			} else {
				tone = -1.0
			}
		case WaveTriangle:
			pq := math.Mod(phase, 2*math.Pi)
			if pq < 0 {
				pq += 2 * math.Pi
			}
			tone = 1.0 - 2.0*math.Abs(pq-math.Pi)/math.Pi
		case WaveSaw:
			pq := math.Mod(phase, 2*math.Pi)
			if pq < 0 {
				pq += 2 * math.Pi
			}
			tone = (pq - math.Pi) / math.Pi
		default:
			tone = math.Sin(phase)
		}
		// Noise mix — crossfade between tone and white noise.
		if noiseMix > 0 {
			noise := noiseRng.Float64()*2 - 1
			tone = tone*(1-noiseMix) + noise*noiseMix
		}
		// Drive — symmetric soft saturation, normalised so the peak stays
		// near unity. Gated at 0 so the neutral path is untouched.
		if drive > 0 {
			g := 1 + drive*driveMaxGain
			tone = math.Tanh(tone*g) / math.Tanh(g)
		}
		// Low-pass — one-pole. Bypassed exactly at Cutoff>=1 so the neutral
		// path can't introduce float rounding (keeps the square binary).
		if p.Cutoff < 1.0 {
			alpha := p.Cutoff
			if alpha < cutoffMinAlpha {
				alpha = cutoffMinAlpha
			}
			lpState += alpha * (tone - lpState)
			tone = lpState
		}
		// Bitcrush — sample-and-hold reduces the effective sample rate for a
		// retro/aliased grain. holdLen==1 (Crush 0) is a no-op.
		if holdLen > 1 {
			if holdCounter == 0 {
				heldTone = tone
			}
			tone = heldTone
			holdCounter++
			if holdCounter >= holdLen {
				holdCounter = 0
			}
		}
		secs := float64(i) / float64(SampleRate)
		env := adsrEnv(secs, p.Duration, p.Attack, p.Decay, sustain, p.Release, releaseStart)
		// Tremolo — sinusoidal amplitude modulation between (1-depth) and 1.
		if p.TremoloHz > 0 && tremDepth > 0 {
			tremPhase += radiansPerSampleHz * p.TremoloHz
			env *= 1 - tremDepth*(0.5+0.5*math.Sin(tremPhase))
		}
		pcm[i] = ClampToInt16(tone * env * p.Volume)
	}
	return pcm
}

// SynthShape is the backward-compat positional wrapper over
// SynthShapeParams: it fills the new knobs with their neutral values so a
// sine/square/etc. call produces exactly what it did before the rich
// params landed. The golden tests pin this byte-for-byte.
func SynthShape(duration, startHz, endHz, volume, attack, release float64,
	wave WaveShape, noiseMix, vibHz, vibDepth float64) []int16 {
	return SynthShapeParams(ShapeParams{
		Duration: duration, StartHz: startHz, EndHz: endHz, Volume: volume,
		Attack: attack, Decay: 0, Sustain: 1, Release: release,
		Wave: wave, PulseWidth: 0.5, NoiseMix: noiseMix,
		VibHz: vibHz, VibDepth: vibDepth,
		TremoloHz: 0, TremoloDepth: 0, Cutoff: 1, Drive: 0, Crush: 0,
	})
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
			phases[k] += radiansPerSampleHz * freq
			sum += math.Sin(phases[k])
		}
		sum /= float64(len(freqs))
		// Bell envelope: sin(pi*t) — zero at both ends, peaks at t=0.5.
		env := bellEnv(t)
		pcm[i] = ClampToInt16(sum * env * volume)
	}
	return pcm
}

// SynthWhistleTrill is a short, bright "trill whistle" — a pure sine that
// sweeps UPWARD while a fast vibrato warbles it, under a soft attack and a
// singing release, so it reads as a rewarding little tweet (the SMRPG-style
// success whistle) rather than a flat beep. The sine keeps it smooth and
// pleasing (no harsh harmonics); the fast shallow vibrato is what gives it the
// trill shimmer; the upward sweep is what makes it feel like a reward. Used by
// the "Great" timing cue.
func SynthWhistleTrill(duration, startHz, endHz, volume float64) []int16 {
	return SynthShapeParams(ShapeParams{
		Duration:   duration,
		StartHz:    startHz,
		EndHz:      endHz,
		Volume:     volume,
		Attack:     0.006,           // quick, soft onset — no click
		Decay:      0,               //
		Sustain:    1,               //
		Release:    duration * 0.45, // long tail so the whistle "sings" out
		Wave:       WaveSine,        // pure tone = clean whistle
		PulseWidth: 0.5,
		VibHz:      42,    // fast warble = the trill
		VibDepth:   0.035, // shallow shimmer, not a siren
		Cutoff:     1,     // filter open
	})
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
		phase += radiansPerSampleHz * freq
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
			phase += radiansPerSampleHz * freq
			t := float64(i) / float64(samplesPerNote)
			env := bellEnv(t)
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
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))                 // fmt chunk size
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))                  // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(wavChannels))        // channels — mono
	_ = binary.Write(&buf, binary.LittleEndian, uint32(rate))               // sample rate
	_ = binary.Write(&buf, binary.LittleEndian, uint32(rate*wavBlockAlign)) // byte rate (rate × blockAlign)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(wavBlockAlign))      // block align
	_ = binary.Write(&buf, binary.LittleEndian, uint16(wavBitsPerSample))   // bits per sample
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataSize))
	_ = binary.Write(&buf, binary.LittleEndian, pcm)
	return buf.Bytes()
}
