package render

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Render diagnostics log. Toggle from the pause-menu Debug submenu. When on,
// every DrawWorld appends a frame snapshot to crawler-render.log; OpenRenderLog
// also dumps a one-shot init banner. Lives in the user's cache dir, falls back
// to cwd; path is logged to stdout on open.

const (
	renderLogFilename = "crawler-render.log"
	// renderLogFrameStride throttles the per-frame line: 6 ≈ one snapshot/100ms at 60Hz.
	renderLogFrameStride = 6
)

var (
	renderLogMu sync.Mutex
	// renderLogActiveFlag mirrors "renderLogFile != nil" as a lock-free atomic so
	// the per-frame IsRenderLogActive gate is one load, not a mutex round-trip.
	// Written under renderLogMu in Open/CloseRenderLog; read lock-free elsewhere.
	renderLogActiveFlag atomic.Bool
	renderLogFile       *os.File
	renderLogFrameNo    int
	// renderLogPendingInit holds init lines that fired while the log was closed,
	// flushed on the next OpenRenderLog so a between-toggles rebuild is captured.
	renderLogPendingInit []string
)

// OpenRenderLog (re-)opens the diagnostics file in append mode and stamps the
// init banner. Safe to call repeatedly: a second call closes the previous file.
func OpenRenderLog() {
	renderLogMu.Lock()
	defer renderLogMu.Unlock()

	if renderLogFile != nil {
		_ = renderLogFile.Close()
		renderLogFile = nil
		renderLogActiveFlag.Store(false)
	}
	path := renderLogPath()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, core.AssetFileMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render: open %s: %v\n", path, err)
		return
	}
	renderLogFile = f
	renderLogActiveFlag.Store(true)
	renderLogFrameNo = 0

	// One-shot session banner + any pending init lines that fired while closed.
	stamp := time.Now().Format("2006-01-02 15:04:05.000")
	fmt.Fprintf(renderLogFile, "\n=== render log opened %s ===\n", stamp)
	fmt.Fprintf(renderLogFile, "go=%s os=%s arch=%s monitor=%q raylib=%q\n",
		runtime.Version(), runtime.GOOS, runtime.GOARCH,
		safeStr(rl.GetMonitorName(0)),
		"5.x",
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

// IsRenderLogActive reports whether the log file is open. Reads the lock-free
// renderLogActiveFlag so the per-frame drawWorld gate costs a single atomic load.
func IsRenderLogActive() bool {
	return renderLogActiveFlag.Load()
}

// renderLogPendingCap bounds the in-memory init/error backlog kept while the log
// is closed, sized well above the ~6 startup lines so nothing is silently dropped.
const renderLogPendingCap = 512

// trimPending evicts the oldest pending entry in place (copy + re-slice + nil the
// tail) so the dropped string releases for GC — buf[1:] would leave it anchored.
func trimPending() {
	n := len(renderLogPendingInit)
	if n == 0 {
		return
	}
	copy(renderLogPendingInit, renderLogPendingInit[1:])
	renderLogPendingInit[n-1] = ""
	renderLogPendingInit = renderLogPendingInit[:n-1]
}

// logRenderLine is the shared write-or-stash body behind LogRenderInit/Error.
// Writes immediately if the file is open, else stashes (bounded by renderLogPendingCap).
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

// LogRenderInit records a one-off init line (shader compile, model load count).
func LogRenderInit(format string, args ...interface{}) {
	logRenderLine("[init] ", format, args...)
}

// LogRenderError stamps a one-off error line; tagged distinctly for grep.
func LogRenderError(format string, args ...interface{}) {
	logRenderLine("[error] ", format, args...)
}

// renderFrameStats is the per-frame snapshot DrawWorld records when the log is
// on. Separating collect from write lets the hot path increment without the mutex.
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

// LogRenderFrame writes one frame snapshot to the log (if open), throttled by
// renderLogFrameStride.
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
	// dt is raylib's last-frame time; fps its reciprocal. Both from the snapshot.
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
	// No fsync here: it would stall the render thread for ~ms and corrupt the very
	// dt/fps it measures. The write() survives a process crash; CloseRenderLog flushes.
}

// renderLogPath resolves crawler-render.log's location: user cache dir, else cwd.
// Returns an absolute path so the stdout banner is copy-pastable.
func renderLogPath() string {
	if cache, err := os.UserCacheDir(); err == nil {
		dir := filepath.Join(cache, "crawler")
		if err := os.MkdirAll(dir, core.AssetDirMode); err == nil {
			return filepath.Join(dir, renderLogFilename)
		}
	}
	// Fallback: cwd.
	if abs, err := filepath.Abs(renderLogFilename); err == nil {
		return abs
	}
	return renderLogFilename
}

// safeStr coerces a possibly-empty raylib string for the log line.
func safeStr(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}
