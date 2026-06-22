//go:build !windows

package input

// Non-Windows rumble stub: no portable haptics path, so it's a no-op off Windows.
// (An SDL build could route through rl.SetGamepadVibration via a //go:build sdl variant.)
func setGamepadRumble(level float32) {}
