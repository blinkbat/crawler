package render

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Render diagnostics log. Toggle from the pause-menu Debug submenu
// (DebugMenuRenderLog row). When on, every DrawWorld appends a one-
// line frame snapshot to crawler-render.log; OpenRenderLog also
// dumps a one-shot init banner (raylib version, GPU info, shader
// IDs, resource counts) so the log captures both startup state and
// per-frame behaviour.
//
// The log lives in the user's chosen cache dir (so a packaged build
// doesn't pollute Program Files). Falls back to the cwd if cache
// resolution fails. The path is logged to stdout on open so the
// user can find it.

const (
	renderLogFilename = "crawler-render.log"
	// renderLogFrameStride throttles the per-frame line so a 60-Hz
	// session doesn't generate a 60-line/sec wall of text. 6 means
	// one snapshot every ~100ms at 60 Hz; tunable here in one place.
	renderLogFrameStride = 6
)

var (
	renderLogMu sync.Mutex
	// renderLogActiveFlag mirrors "renderLogFile != nil" as a lock-free atomic
	// so the per-frame IsRenderLogActive gate (drawWorld calls it EVERY frame,
	// even when the log is off) is a single atomic load instead of a mutex
	// round-trip. Written under renderLogMu in Open/CloseRenderLog so it can't
	// disagree with renderLogFile; read lock-free everywhere else.
	renderLogActiveFlag atomic.Bool
	renderLogFile       *os.File
	renderLogFrameNo    int
	// renderLogPendingInit is the init banner that gets stamped once on
	// open + every shader / resource load that fires while the log
	// is closed (so a Resources rebuild between toggles still gets
	// captured the next time the user reopens the log).
	renderLogPendingInit []string
)

// OpenRenderLog (re-)opens the diagnostics file in append mode and
// stamps the init banner. Safe to call multiple times: the second
// call closes the previous file first.
func OpenRenderLog() {
	renderLogMu.Lock()
	defer renderLogMu.Unlock()

	if renderLogFile != nil {
		_ = renderLogFile.Close()
		renderLogFile = nil
		renderLogActiveFlag.Store(false)
	}
	path := renderLogPath()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render: open %s: %v\n", path, err)
		return
	}
	renderLogFile = f
	renderLogActiveFlag.Store(true)
	renderLogFrameNo = 0

	// One-shot session banner + any pending init lines that fired
	// while the log was closed (e.g., shader load during NewResources
	// before the first toggle). Banner uses Fprintln so the file is
	// written even if the user never enables the per-frame log.
	stamp := time.Now().Format("2006-01-02 15:04:05.000")
	fmt.Fprintf(renderLogFile, "\n=== render log opened %s ===\n", stamp)
	fmt.Fprintf(renderLogFile, "go=%s os=%s arch=%s gpu=%q glsl=%q\n",
		runtime.Version(), runtime.GOOS, runtime.GOARCH,
		safeStr(rl.GetMonitorName(0)),
		"(raylib_5.x)",
	)
	for _, line := range renderLogPendingInit {
		fmt.Fprintln(renderLogFile, line)
	}
	renderLogPendingInit = nil
	_ = renderLogFile.Sync()
}

// CloseRenderLog flushes + closes the file. Idempotent.
func CloseRenderLog() {
	renderLogMu.Lock()
	defer renderLogMu.Unlock()
	if renderLogFile == nil {
		return
	}
	fmt.Fprintf(renderLogFile, "=== render log closed %s ===\n", time.Now().Format("2006-01-02 15:04:05.000"))
	_ = renderLogFile.Sync()
	_ = renderLogFile.Close()
	renderLogFile = nil
	renderLogActiveFlag.Store(false)
}

// IsRenderLogActive reports whether the log file is currently open.
// Per-frame call sites use this to short-circuit the snapshot work
// (camera + counts) when logging is off. Reads the lock-free
// renderLogActiveFlag rather than taking renderLogMu, so the
// once-per-frame gate in drawWorld costs a single atomic load even
// when the log is off — no mutex traffic on the hot path. Safe to
// call from inside a logging call (no lock to self-deadlock on).
func IsRenderLogActive() bool {
	return renderLogActiveFlag.Load()
}

// renderLogPendingCap bounds the in-memory init/error backlog
// retained while the log file is closed. Sized comfortably above
// the actual startup line count (~6 init lines today: lighting
// shader + locs + billboard fog + locs + resources + flat tables)
// with headroom for future texture/material init dumps. Smaller
// caps risked silently dropping the very lines the log is designed
// to capture for render-bug bisection.
const renderLogPendingCap = 512

// trimPending evicts the oldest pending entry in place (copy +
// re-slice the same array) so the dropped string headers actually
// release for GC — `buf = buf[1:]` would advance the header but
// leave the discarded element anchored in the underlying array.
func trimPending() {
	n := len(renderLogPendingInit)
	if n == 0 {
		return
	}
	copy(renderLogPendingInit, renderLogPendingInit[1:])
	renderLogPendingInit[n-1] = ""
	renderLogPendingInit = renderLogPendingInit[:n-1]
}

// logRenderLine is the shared write-or-stash body behind LogRenderInit /
// LogRenderError. If the log file is open it writes the tagged line immediately;
// otherwise it stashes the line in renderLogPendingInit to be flushed the next
// time OpenRenderLog runs. The pending buffer is bounded by renderLogPendingCap —
// past that the oldest gets dropped so a long session without the log toggled on
// can't grow the buffer unboundedly.
func logRenderLine(tag, format string, args ...interface{}) {
	line := fmt.Sprintf(tag+format, args...)
	renderLogMu.Lock()
	defer renderLogMu.Unlock()
	if renderLogFile != nil {
		fmt.Fprintln(renderLogFile, line)
		_ = renderLogFile.Sync()
		return
	}
	if len(renderLogPendingInit) >= renderLogPendingCap {
		trimPending()
	}
	renderLogPendingInit = append(renderLogPendingInit, line)
}

// LogRenderInit records a one-off init line (shader compile result,
// model load count, etc.).
func LogRenderInit(format string, args ...interface{}) {
	logRenderLine("[init] ", format, args...)
}

// LogRenderError stamps a one-off error line. Same write-or-stash semantics as
// LogRenderInit; tagged differently so a grep on the resulting log can separate
// errors from init noise.
func LogRenderError(format string, args ...interface{}) {
	logRenderLine("[error] ", format, args...)
}

// renderFrameStats is the per-frame snapshot recorded by DrawWorld
// when the log is on. Captured by the world loop, written via
// logRenderFrame at the bottom of the draw call. Splitting "collect"
// from "write" lets the DrawWorld hot path increment counters
// directly without touching the mutex per tile.
type renderFrameStats struct {
	MapW, MapH       int
	FrameDT          float32 // raylib frame time (s); printed as ms + derived fps
	TilesIterated    int
	TilesCulled      int
	WallsDrawn       int
	FloorsDrawn      int
	CeilingsDrawn    int
	DecorDrawn       int
	PropsDrawn       int
	TorchCount       int
	CamPos           rl.Vector3
	CamDir           rl.Vector3
	CamFOV           float32
	PlayerYaw        float32
	PlayerLookYaw    float32
	PlayerLookPitch  float32
	StepCount        int
	LightingShaderID uint32
	BillboardFogID   uint32
	FogDensity       float32
	FogColor         rl.Vector3
	AmbientColor     rl.Vector3
	SunColor         rl.Vector3
	BattleActive     bool
}

// LogRenderFrame writes one frame snapshot to the log file (if open).
// Throttled by renderLogFrameStride so a 60Hz session writes ~10
// lines/sec. Caller passes a populated renderFrameStats — most
// fields come from the world draw, the rest from the GameState.
func LogRenderFrame(stats renderFrameStats) {
	renderLogMu.Lock()
	defer renderLogMu.Unlock()
	if renderLogFile == nil {
		return
	}
	renderLogFrameNo++
	if renderLogFrameNo%renderLogFrameStride != 0 {
		return
	}
	t := time.Now().Format("15:04:05.000")
	// dt is raylib's last-frame time; fps is its reciprocal. Both come straight
	// from the stat snapshot — no clock call here — so the line cost is just the
	// format + buffered write below.
	dtMS := stats.FrameDT * 1000
	fps := float32(0)
	if stats.FrameDT > 0 {
		fps = 1 / stats.FrameDT
	}
	fmt.Fprintf(renderLogFile,
		"[%s f=%d] dt=%.2fms fps=%.0f map=%dx%d iter=%d cull=%d walls=%d floor=%d ceil=%d decor=%d props=%d torches=%d cam=(%.2f,%.2f,%.2f) dir=(%.2f,%.2f,%.2f) fov=%.1f yaw=%.2f look=(%.2f,%.2f) step=%d shader=L%d/B%d fog=%.3f@(%.2f,%.2f,%.2f) amb=(%.2f,%.2f,%.2f) sun=(%.2f,%.2f,%.2f) battle=%v\n",
		t, renderLogFrameNo,
		dtMS, fps,
		stats.MapW, stats.MapH, stats.TilesIterated, stats.TilesCulled,
		stats.WallsDrawn, stats.FloorsDrawn, stats.CeilingsDrawn, stats.DecorDrawn, stats.PropsDrawn,
		stats.TorchCount,
		stats.CamPos.X, stats.CamPos.Y, stats.CamPos.Z,
		stats.CamDir.X, stats.CamDir.Y, stats.CamDir.Z,
		stats.CamFOV,
		stats.PlayerYaw,
		stats.PlayerLookYaw, stats.PlayerLookPitch,
		stats.StepCount,
		stats.LightingShaderID, stats.BillboardFogID,
		stats.FogDensity,
		stats.FogColor.X, stats.FogColor.Y, stats.FogColor.Z,
		stats.AmbientColor.X, stats.AmbientColor.Y, stats.AmbientColor.Z,
		stats.SunColor.X, stats.SunColor.Y, stats.SunColor.Z,
		stats.BattleActive,
	)
	// No fsync here. Sync() is a disk flush that can stall the main render
	// thread for ~ms — a hitch that would both cost frame time and corrupt the
	// dt/fps numbers this line exists to measure. The Fprintf above is a cheap
	// write() into the OS page cache, which already survives a process crash
	// (only an OS crash / power loss could lose it, irrelevant for a debug log).
	// Durable flush still happens on the graceful CloseRenderLog path.
}

// renderLogPath resolves the on-disk location for crawler-render.log.
// Prefers the user's cache dir; falls back to cwd if that fails so
// the log is always somewhere predictable. Returns the absolute
// path so the stdout banner the user sees on toggle is copy-pastable.
func renderLogPath() string {
	if cache, err := os.UserCacheDir(); err == nil {
		dir := filepath.Join(cache, "crawler")
		if err := os.MkdirAll(dir, 0755); err == nil {
			return filepath.Join(dir, renderLogFilename)
		}
	}
	// Fallback: cwd. Best-effort absolute path so the banner reads cleanly.
	if abs, err := filepath.Abs(renderLogFilename); err == nil {
		return abs
	}
	return renderLogFilename
}

// safeStr coerces a possibly-empty raylib string into something the
// log line can print without breaking.
func safeStr(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}
