// Package wavsynth holds pure procedural-audio helpers — sweeps, chords, chimes,
// and a 16-bit-mono-PCM WAV header builder. Split out so it unit-tests without
// raylib on the load path (purego opens raylib.dll at init).
package wavsynth

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/rand"
	"strconv"
)

// SampleRate is the working sample rate — small encoded cues, no audible
// aliasing on the sweeps.
const SampleRate = 22050

// radiansPerSampleHz is the per-sample phase increment for 1 Hz; multiply by a
// frequency to advance an oscillator one sample.
const radiansPerSampleHz = 2 * math.Pi / float64(SampleRate)

// bellEnv is the envelope sin(pi*t) for t in [0,1] — zero at both ends, peaks
// at 0.5. Shared by chord and chime so their ring-out matches.
func bellEnv(t float64) float64 {
	return math.Sin(math.Pi * t)
}

// clampF64 pins v into [lo,hi]. Package-local (not core.Clamp) to keep wavsynth
// raylib-free on the load path — see the package doc.
func clampF64(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ClampToInt16 converts a float in [-1,1] to 16-bit PCM, pinning out-of-range
// values to the endpoints (wrapping would cause ring-modulator artifacts).
func ClampToInt16(sample float64) int16 {
	return int16(clampF64(sample, -1, 1) * 32767)
}

// WaveShape selects the oscillator timbre: Sine (pure), Square (harsh/8-bit),
// Triangle (gentler), Saw (buzzy).
type WaveShape int

const (
	WaveSine WaveShape = iota
	WaveSquare
	WaveTriangle
	WaveSaw
	waveShapeCount
)

// WaveShapeCount is the number of WaveShape values, for the editor's wave picker.
const WaveShapeCount = int(waveShapeCount)

// waveShapeNames: per-shape label. Array (not switch) so a new shape leaves an
// empty slot the init assert catches.
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

// WaveShapeName returns a label for a WaveShape; out-of-range falls back to Sine.
func WaveShapeName(w WaveShape) string {
	if w < 0 || int(w) >= len(waveShapeNames) {
		return waveShapeNames[WaveSine]
	}
	return waveShapeNames[w]
}

// Musical note support — lets the editor pick tempered notes instead of raw Hz.
// Equal temperament, A4 = 440 Hz, index 0 = C2.
const (
	noteA4Hz    = 440.0
	noteA4Index = 33 // semitones from C2 to A4: 2 octaves + 9
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

// NoteName returns the scientific-pitch label of note index i (e.g. "A4", "C#5").
func NoteName(i int) string {
	i = clampNoteIndex(i)
	return noteNames[i%12] + strconv.Itoa(2+i/12)
}

// NearestNoteIndex returns the note index closest to hz — the inverse of NoteHz.
func NearestNoteIndex(hz float64) int {
	if hz <= 0 {
		return 0
	}
	return clampNoteIndex(int(math.Round(noteA4Index + 12*math.Log2(hz/noteA4Hz))))
}

// FX voicing knobs — "feel" scaling for the per-sample loop.
const (
	driveMaxGain   = 5.0  // Drive=1 → tanh pre-gain 6×
	cutoffMinAlpha = 0.02 // LPF floor so Cutoff=0 stays a dark tone, not silence
	crushMaxHold   = 24   // Crush=1 → sample-and-hold 25 samples (~882 Hz)
	maxDuration    = 30.0 // length cap (sec) so a corrupt .snd can't drive a huge alloc
	shortSynthFloor = 0.02 // floor (sec) for a non-positive duration: a short blip, not a 30s tone
)

// ShapeParams is the full knob set for the sound editor; SynthShape wraps it
// with neutral values: Decay 0, Sustain 1, PulseWidth 0.5, Cutoff 1, Drive/Crush/Tremolo 0.
type ShapeParams struct {
	Duration     float64   `json:"duration"`      // seconds
	StartHz      float64   `json:"start_hz"`      // sweep start frequency
	EndHz        float64   `json:"end_hz"`        // sweep end frequency
	Volume       float64   `json:"volume"`        // peak amplitude [0,1]
	Attack       float64   `json:"attack"`        // ADSR attack, sec
	Decay        float64   `json:"decay"`         // ADSR decay, sec (0 = skip)
	Sustain      float64   `json:"sustain"`       // ADSR sustain [0,1]
	Release      float64   `json:"release"`       // ADSR release, sec
	Wave         WaveShape `json:"wave"`          // oscillator timbre
	PulseWidth   float64   `json:"pulse_width"`   // square duty [0.01,0.99]; 0.5 = symmetric
	NoiseMix     float64   `json:"noise"`         // tone↔noise crossfade [0,1]
	VibHz        float64   `json:"vibrato_hz"`    // pitch-wobble rate
	VibDepth     float64   `json:"vibrato_depth"` // pitch-wobble depth (frac of freq)
	TremoloHz    float64   `json:"tremolo_hz"`    // amp-wobble rate
	TremoloDepth float64   `json:"tremolo_depth"` // amp-wobble depth [0,1]
	Cutoff       float64   `json:"cutoff"`        // one-pole LPF [0,1]; 1 = open
	Drive        float64   `json:"drive"`         // soft saturation [0,1]
	Crush        float64   `json:"crush"`         // sample-rate reduction [0,1]
}

// clamp01 pins a value into [0,1].
func clamp01(v float64) float64 {
	return clampF64(v, 0, 1)
}

// wrap2Pi folds an unbounded phase into [0, 2π).
func wrap2Pi(phase float64) float64 {
	p := math.Mod(phase, 2*math.Pi)
	if p < 0 {
		p += 2 * math.Pi
	}
	return p
}

// sanitizeDuration is the shared guard head for every synth path: NaN/±Inf are
// non-finite (finite=false, clamped=0) because int(d*SampleRate) is unspecified for
// them and could dodge the samples<=0 floor to drive a giant alloc; a finite duration
// over maxDuration is capped so no path allocs unbounded. The <=0 / non-finite TAIL
// diverges per caller (1 sample / shortSynthFloor / nil), so callers keep their own.
func sanitizeDuration(duration float64) (clamped float64, finite bool) {
	if math.IsNaN(duration) || math.IsInf(duration, 0) {
		return 0, false
	}
	if duration > maxDuration {
		duration = maxDuration
	}
	return duration, true
}

// samplesFor is duration×SampleRate, floored to at least 1 (a non-positive
// duration must still produce one sample, not zero).
func samplesFor(duration float64) int {
	d, finite := sanitizeDuration(duration)
	if !finite {
		return 1 // degenerate: one sample, never zero
	}
	samples := int(d * SampleRate)
	if samples <= 0 {
		samples = 1
	}
	return samples
}

// neutralShape is the ShapeParams identity for the positional wrappers: no
// decay, full sustain, symmetric pulse, open filter. Callers override fields.
func neutralShape() ShapeParams {
	return ShapeParams{Decay: 0, Sustain: 1, PulseWidth: 0.5, Cutoff: 1}
}

// adsrEnv computes the ADSR level at elapsed time secs. Decay 0 + Sustain 1
// reduces to SynthSweep's attack/sustain/release shape (byte-identical).
func adsrEnv(secs, duration, attack, decay, sustain, release, releaseStart float64) float64 {
	// Never let the decay window eat into the release: with a long attack+decay it
	// could otherwise preempt the release branch, so the fade wouldn't span `release`
	// seconds and the level would jump discontinuously (a click). Clamping the decay
	// end to releaseStart keeps the release tail intact. (Decay==0 skips this branch
	// entirely, so the neutral SynthSweep shape stays byte-identical.)
	decayEnd := attack + decay
	if decayEnd > releaseStart {
		decayEnd = releaseStart
	}
	switch {
	case attack > 0 && secs < attack:
		return secs / attack
	case decay > 0 && secs < decayEnd:
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

// SynthShapeParams is the rich procedural sweep primitive. Signal chain:
//
//	oscillator → noise mix → drive → low-pass → bitcrush → ×(ADSR·tremolo·volume)
func SynthShapeParams(p ShapeParams) []int16 {
	// Pin the length before any math: sanitizeDuration bounds NaN/Inf/over-cap; here
	// non-finite folds to maxDuration (a full tone, not silence) while a non-positive
	// duration floors to a short blip (like SynthChord's "no time → minimal") so a
	// corrupt "0" sidecar can't emit a 30s buffer, keeping the envelope math finite.
	switch d, finite := sanitizeDuration(p.Duration); {
	case !finite:
		p.Duration = maxDuration
	case d <= 0:
		p.Duration = shortSynthFloor
	default:
		p.Duration = d
	}
	samples := samplesFor(p.Duration)
	noiseMix := clamp01(p.NoiseMix)
	sustain := clamp01(p.Sustain)
	tremDepth := clamp01(p.TremoloDepth)
	drive := clamp01(p.Drive)
	crush := clamp01(p.Crush)
	vibDepth := math.Max(0, p.VibDepth)
	pulse := clampF64(p.PulseWidth, 0.01, 0.99)
	holdLen := 1 + int(crush*crushMaxHold+0.5) // 1 = no crush
	releaseStart := p.Duration - p.Release
	if releaseStart < 0 {
		// Release longer than the whole note: clamp so the sustain branch can still
		// fire (sample 0) instead of opening mid-release below the sustain level.
		releaseStart = 0
	}

	pcm := make([]int16, samples)
	phase, vibPhase, tremPhase := 0.0, 0.0, 0.0
	// Fixed seed so identical params produce identical waveforms (editor feedback).
	noiseRng := rand.New(rand.NewSource(0xC0FFEE_BABE))
	lpState := 0.0 // one-pole low-pass memory
	heldTone := 0.0
	holdCounter := 0
	for i := 0; i < samples; i++ {
		t := float64(i) / float64(samples)
		freq := p.StartHz + (p.EndHz-p.StartHz)*t
		// Vibrato — phase-integrated FM so the wobble doesn't click at edges.
		if p.VibHz > 0 && vibDepth > 0 {
			vibPhase += radiansPerSampleHz * p.VibHz
			freq += freq * vibDepth * math.Sin(vibPhase)
		}
		phase += radiansPerSampleHz * freq
		// Oscillator — phase is unbounded, so shaped waves wrap to [0, 2π).
		var tone float64
		switch p.Wave {
		case WaveSquare:
			if wrap2Pi(phase)/(2*math.Pi) < pulse {
				tone = 1.0
			} else {
				tone = -1.0
			}
		case WaveTriangle:
			tone = 1.0 - 2.0*math.Abs(wrap2Pi(phase)-math.Pi)/math.Pi
		case WaveSaw:
			tone = (wrap2Pi(phase) - math.Pi) / math.Pi
		default:
			tone = math.Sin(phase)
		}
		// Noise mix — crossfade tone↔white noise.
		if noiseMix > 0 {
			noise := noiseRng.Float64()*2 - 1
			tone = tone*(1-noiseMix) + noise*noiseMix
		}
		// Drive — symmetric soft saturation, normalised to ~unity peak.
		if drive > 0 {
			g := 1 + drive*driveMaxGain
			tone = math.Tanh(tone*g) / math.Tanh(g)
		}
		// Low-pass — one-pole. Bypassed at Cutoff>=1 so the neutral path
		// can't introduce float rounding (keeps the square binary).
		if p.Cutoff < 1.0 {
			alpha := p.Cutoff
			if alpha < cutoffMinAlpha {
				alpha = cutoffMinAlpha
			}
			lpState += alpha * (tone - lpState)
			tone = lpState
		}
		// Bitcrush — sample-and-hold for a retro/aliased grain. holdLen==1 is a no-op.
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
		// Tremolo — amplitude modulation between (1-depth) and 1.
		if p.TremoloHz > 0 && tremDepth > 0 {
			tremPhase += radiansPerSampleHz * p.TremoloHz
			env *= 1 - tremDepth*(0.5+0.5*math.Sin(tremPhase))
		}
		pcm[i] = ClampToInt16(tone * env * p.Volume)
	}
	return pcm
}

// SynthShape is the positional wrapper over SynthShapeParams, filling the new
// knobs with neutral values. Golden tests pin this byte-for-byte.
func SynthShape(duration, startHz, endHz, volume, attack, release float64,
	wave WaveShape, noiseMix, vibHz, vibDepth float64) []int16 {
	p := neutralShape()
	p.Duration, p.StartHz, p.EndHz, p.Volume = duration, startHz, endHz, volume
	p.Attack, p.Release = attack, release
	p.Wave, p.NoiseMix, p.VibHz, p.VibDepth = wave, noiseMix, vibHz, vibDepth
	return SynthShapeParams(p)
}

// SynthSweep is the sine, no-noise, no-vibrato wrapper.
func SynthSweep(duration, startHz, endHz, volume, attack, release float64) []int16 {
	return SynthShape(duration, startHz, endHz, volume, attack, release, WaveSine, 0, 0, 0)
}

// SynthChord sums sines at the given frequencies into one note under a bell
// envelope.
func SynthChord(duration float64, freqs []float64, volume float64) []int16 {
	// sanitizeDuration bounds NaN/Inf/over-cap; a non-finite duration here degenerates
	// to nil (mirrors SynthShapeParams' shared head, but with SynthChord's own tail).
	d, finite := sanitizeDuration(duration)
	if !finite {
		return nil
	}
	duration = d
	samples := int(duration * SampleRate)
	if samples <= 0 {
		// No time → no samples. (A non-positive duration must NOT fall through to
		// SynthSweep, whose SynthShapeParams pins duration<=0 to a 30s maxDuration.)
		return nil
	}
	if len(freqs) == 0 {
		// Has time but no tones: a default sine so the cue isn't silent.
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
		env := bellEnv(t)
		pcm[i] = ClampToInt16(sum * env * volume)
	}
	return pcm
}

// SynthWhistleTrill is the SMRPG-style "Great" cue — a sine sweeping up with a
// fast shallow vibrato, soft attack, singing release.
func SynthWhistleTrill(duration, startHz, endHz, volume float64) []int16 {
	p := neutralShape()
	p.Duration, p.StartHz, p.EndHz, p.Volume = duration, startHz, endHz, volume
	p.Attack = 0.006            // soft onset, no click
	p.Release = duration * 0.45 // long "singing" tail
	p.Wave = WaveSine
	p.VibHz = 42       // fast warble = the trill
	p.VibDepth = 0.035 // shallow shimmer, not a siren
	return SynthShapeParams(p)
}

// SynthClick is a short percussive transient — a pitched sine dropping in
// frequency, blended with noise, under hard-attack/exp-decay. Covers tick and thud.
//
//	duration   total sec (typical 0.02–0.08)
//	pitchHz    body fundamental at note start
//	pitchDrop  fraction to slide DOWN over the note (0.7 → ends at 0.3×; kick-drum pop)
//	noise      [0,1] noise mix
//	volume     peak scale [0,1]
//
// Fixed seed → identical waveform every run. Sum two clicks for variation.
func SynthClick(duration, pitchHz, pitchDrop, noise, volume float64) []int16 {
	samples := samplesFor(duration)
	pcm := make([]int16, samples)
	// Fixed seed: deterministic so the modal preview matches battle playback.
	rng := rand.New(rand.NewSource(1))
	phase := 0.0
	// Hard attack ramps 0→1 over attackSamples, then exponential decay. 2ms
	// keeps the transient crisp without a click-at-zero artifact.
	attackSamples := int(math.Round(0.002 * float64(SampleRate)))
	if attackSamples < 1 {
		attackSamples = 1
	}
	if attackSamples > samples {
		attackSamples = samples
	}
	// Tuned so the envelope reaches ~3% by note end (below the noise floor).
	decayK := -3.5
	for i := 0; i < samples; i++ {
		t := float64(i) / float64(samples)
		freq := pitchHz * (1 - pitchDrop*t)
		if freq < 0 {
			freq = 0
		}
		phase += radiansPerSampleHz * freq
		sine := math.Sin(phase)
		// White noise in [-1, 1].
		n := rng.Float64()*2 - 1
		sample := sine*(1-noise) + n*noise
		var env float64
		if i < attackSamples {
			env = float64(i) / float64(attackSamples)
		} else {
			// Exponential decay anchored at the attack ramp's end.
			td := float64(i-attackSamples) / float64(samples-attackSamples)
			env = math.Exp(decayK * td)
		}
		pcm[i] = ClampToInt16(sample * env * volume)
	}
	return pcm
}

// SynthChime plays two notes back-to-back (firstHz then secondHz), each
// noteDuration seconds. The heal cue's "ding-ding".
func SynthChime(noteDuration, firstHz, secondHz, volume float64) []int16 {
	samplesPerNote := samplesFor(noteDuration)
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

// WAV format constants — every WAV this package writes is mono, 16-bit PCM.
// blockAlign = channels × bytesPerSample; byteRate = rate × blockAlign.
const (
	wavChannels       = 1
	wavBitsPerSample  = 16
	wavBytesPerSample = wavBitsPerSample / 8
	wavBlockAlign     = wavChannels * wavBytesPerSample
)

// BuildWAV writes a canonical 16-bit mono PCM WAV into a byte slice (RIFF → fmt
// → data) for raylib's LoadWaveFromMemory. Always at SampleRate.
func BuildWAV(pcm []int16) []byte {
	dataSize := len(pcm) * wavBytesPerSample
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))                       // fmt size
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))                        // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(wavChannels))              // channels
	_ = binary.Write(&buf, binary.LittleEndian, uint32(SampleRate))               // sample rate
	_ = binary.Write(&buf, binary.LittleEndian, uint32(SampleRate*wavBlockAlign)) // byte rate
	_ = binary.Write(&buf, binary.LittleEndian, uint16(wavBlockAlign))            // block align
	_ = binary.Write(&buf, binary.LittleEndian, uint16(wavBitsPerSample))         // bits/sample
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataSize))
	_ = binary.Write(&buf, binary.LittleEndian, pcm)
	return buf.Bytes()
}
