//go:build !debug

package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// DebugBuildEnabled is the build-flag mirror of the //go:build debug tag.
// debug.go (the +debug-tagged file) sets it to true; this release file
// sets it to false. The pause menu reads it to annotate the Debug Overlay
// row so the user knows whether the toggle is live in this build.
const DebugBuildEnabled = false

// DrawDebugOverlay is a no-op in release builds. The full implementation
// lives in debug.go behind a `debug` build tag so the tile-label sort,
// per-frame label buffer, and font/measure calls don't ship in the
// release binary. To re-enable, build with `go build -tags debug`.
//
// g.DebugOverlay is preserved on GameState so the pause menu can still
// toggle the flag without conditional compilation — the toggle just
// becomes inert in this build.
func DrawDebugOverlay(camera rl.Camera3D, g core.GameState, assets Resources) {}
