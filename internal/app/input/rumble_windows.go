//go:build windows

package input

import (
	"sync"
	"syscall"
	"unsafe"

	"crawler/internal/app/core"
)

// Windows controller rumble via XInput (raylib's GLFW backend has no vibration).
// XInputSetState on a disconnected index is a safe no-op.

// xinputPrimaryUser is the XInput user index vibrated, derived from gamepadID so
// the two can't drift (XInput user N == raylib gamepad N).
const xinputPrimaryUser = uint32(gamepadID)

var (
	xinputOnce     sync.Once
	xinputSetState *syscall.LazyProc
	xinputReady    bool
)

// loadXInput resolves XInputSetState, trying runtimes in order (1_4 = Win8+,
// 1_3 = legacy DX redist, 9_1_0 = Vista/7). None present leaves rumble a no-op.
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

// xinputMaxMotorSpeed is the full-scale XINPUT_VIBRATION motor speed (uint16 ceiling).
const xinputMaxMotorSpeed = 65535

// xinputVibration mirrors XINPUT_VIBRATION (left = heavy/low-freq, right = light/high).
type xinputVibration struct {
	leftMotor  uint16
	rightMotor uint16
}

// setGamepadRumble drives both primary-controller motors at `level` (0..1).
func setGamepadRumble(level float32) {
	xinputOnce.Do(loadXInput)
	if !xinputReady {
		return
	}
	level = core.Clamp(level, 0, 1)
	speed := uint16(level * xinputMaxMotorSpeed)
	vib := xinputVibration{leftMotor: speed, rightMotor: speed}
	// Ignore the DWORD return — a disconnected pad is a harmless no-op.
	xinputSetState.Call(uintptr(xinputPrimaryUser), uintptr(unsafe.Pointer(&vib)))
}
