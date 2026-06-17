package render

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
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
	renderLogMu      sync.Mutex
	renderLogFile    *os.File
	renderLogFrameNo int
	renderLogTickCnt int
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
	}
	path := renderLogPath()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render: open %s: %v\n", path, err)
		return
	}
	renderLogFile = f
	renderLogFrameNo = 0
	// Reset the throttle counter too so the every-10th-tick flush gate starts
	// from a known phase on each (re)open rather than at an arbitrary offset
	// carried over from a previous session.
	renderLogTickCnt = 0

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
}

// IsRenderLogActive reports whether the log file is currently open.
// Per-frame call sites use this to short-circuit the snapshot work
// (camera + counts) when logging is off.
//
// LOCKING CONTRACT: This function takes renderLogMu. Callers MUST
// NOT hold renderLogMu when calling — sync.Mutex is not reentrant
// and a "log if active" wrapper that called this from inside
// LogRenderInit / LogRenderFrame would self-deadlock. If a future
// helper needs the same predicate from inside a logging call, split
// out an isRenderLogActiveLocked() that reads `renderLogFile`
// directly.
func IsRenderLogActive() bool {
	renderLogMu.Lock()
	defer renderLogMu.Unlock()
	return renderLogFile != nil
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

// LogRenderInit records a one-off init line (shader compile result,
// model load count, etc.). If the log file is open it writes
// immediately; otherwise it stashes the line in renderLogPendingInit
// to be flushed the next time OpenRenderLog runs. The pending
// buffer is bounded by renderLogPendingCap — past that the oldest
// gets dropped so a long session without the log toggled on can't
// grow the buffer unboundedly.
func LogRenderInit(format string, args ...interface{}) {
	line := fmt.Sprintf("[init] "+format, args...)
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

// LogRenderError stamps a one-off error line. Same write-or-stash
// semantics as LogRenderInit; tagged differently so a grep on the
// resulting log can separate errors from init noise.
func LogRenderError(format string, args ...interface{}) {
	line := fmt.Sprintf("[error] "+format, args...)
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

// renderFrameStats is the per-frame snapshot recorded by DrawWorld
// when the log is on. Captured by the world loop, written via
// logRenderFrame at the bottom of the draw call. Splitting "collect"
// from "write" lets the DrawWorld hot path increment counters
// directly without touching the mutex per tile.
type renderFrameStats struct {
	MapW, MapH       int
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
	PlayerYaw       float32
	PlayerLookYaw   float32
	PlayerLookPitch float32
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
	fmt.Fprintf(renderLogFile,
		"[%s f=%d] map=%dx%d iter=%d cull=%d walls=%d floor=%d ceil=%d decor=%d props=%d torches=%d cam=(%.2f,%.2f,%.2f) dir=(%.2f,%.2f,%.2f) fov=%.1f yaw=%.2f look=(%.2f,%.2f) step=%d shader=L%d/B%d fog=%.3f@(%.2f,%.2f,%.2f) amb=(%.2f,%.2f,%.2f) sun=(%.2f,%.2f,%.2f) battle=%v\n",
		t, renderLogFrameNo,
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
	// Flush every Nth throttled line so a crash or window-close still
	// leaves the most recent context on disk.
	renderLogTickCnt++
	if renderLogTickCnt%10 == 0 {
		_ = renderLogFile.Sync()
	}
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
