//go:build windows

package input

import (
	"sync"
	"syscall"
	"unsafe"
)

// Windows controller rumble via XInput. raylib's GLFW desktop backend has NO
// vibration (its SetGamepadVibration is a no-op that logs a warning), so we
// drive the motors directly through XInput — the native Windows gamepad API
// that Xbox controllers (and the many XInput-compatible third-party / Steam-
// mapped pads) speak. Detection still goes through raylib (gamepadConnected);
// only the haptic OUTPUT lives here.
//
// We target XInput user index 0 (the primary controller), which matches
// raylib's gamepadID 0 for the common single-pad case. XInputSetState on a
// disconnected index returns ERROR_DEVICE_NOT_CONNECTED and does nothing, so an
// unplugged or non-XInput pad is a safe no-op rather than an error.

const xinputPrimaryUser = 0

var (
	xinputOnce     sync.Once
	xinputSetState *syscall.LazyProc
	xinputReady    bool
)

// loadXInput resolves XInputSetState from whichever XInput runtime is present.
// 1_4 ships with Windows 8+, 1_3 with the legacy DirectX SDK redist, 9_1_0 is
// the Vista/7 baseline — trying them in order keeps rumble working across
// Windows versions without a build-time dependency. If none load (XInput
// absent), setGamepadRumble becomes a silent no-op.
func loadXInput() {
	for _, name := range []string{"xinput1_4.dll", "xinput1_3.dll", "xinput9_1_0.dll"} {
		dll := syscall.NewLazyDLL(name)
		if dll.Load() != nil {
			continue
		}
		proc := dll.NewProc("XInputSetState")
		if proc.Find() != nil {
			continue
		}
		xinputSetState = proc
		xinputReady = true
		return
	}
}

// xinputMaxMotorSpeed is the full-scale value for an XINPUT_VIBRATION motor
// speed (the uint16 ceiling); a normalized [0,1] level scales up to it.
const xinputMaxMotorSpeed = 65535

// xinputVibration mirrors the C XINPUT_VIBRATION struct: two motor speeds in
// [0, xinputMaxMotorSpeed] (left = low-frequency/heavy, right = high-frequency/light).
type xinputVibration struct {
	leftMotor  uint16
	rightMotor uint16
}

// setGamepadRumble drives both motors of the primary controller at `level`
// (0..1). Clamped defensively; the caller (ApplyRumble) already bounds it.
func setGamepadRumble(level float32) {
	xinputOnce.Do(loadXInput)
	if !xinputReady {
		return
	}
	if level < 0 {
		level = 0
	} else if level > 1 {
		level = 1
	}
	speed := uint16(level * xinputMaxMotorSpeed)
	vib := xinputVibration{leftMotor: speed, rightMotor: speed}
	// LazyProc.Call keeps vib alive across the call; the uintptr(unsafe.Pointer)
	// is the standard syscall-argument idiom. Ignore the DWORD return (success /
	// ERROR_DEVICE_NOT_CONNECTED) — a disconnected pad is a harmless no-op.
	xinputSetState.Call(uintptr(xinputPrimaryUser), uintptr(unsafe.Pointer(&vib)))
}
