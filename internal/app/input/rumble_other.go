//go:build !windows

package input

// Non-Windows rumble stub. raylib's GLFW desktop backend has no vibration, and
// there's no portable cross-platform haptics path here, so rumble is a no-op
// off Windows. (A raylib SDL build — `-tags sdl` — could route through
// rl.SetGamepadVibration instead; add an `//go:build sdl` variant here if that
// backend is ever adopted.) ApplyRumble's level/clamp/idle-skip logic is shared
// and platform-agnostic; only this motor-output call differs per platform.
func setGamepadRumble(level float32) {}
