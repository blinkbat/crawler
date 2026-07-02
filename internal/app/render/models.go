package render

import (
	"fmt"
	"image/color"
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// textureAndShade applies one diffuse texture to every model and binds the
// lighting shader. Loaders that skin meshes individually keep their own loop.
func textureAndShade(models []rl.Model, shader rl.Shader, tex rl.Texture2D) {
	for i := range models {
		setModelTexture(&models[i], tex)
		attachShader(&models[i], shader)
	}
}

// shadeAll binds the lighting shader to every model without touching textures —
// for loaders that texture a subset (or none) and tint the rest at part level.
func shadeAll(models []rl.Model, shader rl.Shader) {
	for i := range models {
		attachShader(&models[i], shader)
	}
}

// skinExceptMoss textures + shades every model EXCEPT mossIdx (moss reads as
// granite if textured, so it stays untextured and tinted at part level). Shared
// by the rock boulder/cairn/formation loaders.
func skinExceptMoss(models []rl.Model, mossIdx int, shader rl.Shader, tex rl.Texture2D) {
	for i := range models {
		if i != mossIdx {
			setModelTexture(&models[i], tex)
		}
		attachShader(&models[i], shader)
	}
}

// treeModel bundles the meshes for one blocking tree. Unique meshes live in
// `models`; `parts` reference them by index so a mesh can be reused without
// double-freeing on unload.
type treeModel struct {
	models []rl.Model
	parts  []treePart
}

type treePart struct {
	modelIdx int
	offset   rl.Vector3
	scale    rl.Vector3
	rotation float32
	// rotationAxis tilts a part off vertical. Zero vector means world up (0,1,0).
	rotationAxis rl.Vector3
	tint         color.RGBA
	// sway is the per-part wind-displacement factor (0..1); 0 = rigid. Foliage
	// parts set sway >= 1 to ride the global wind animation (propModel.draw adds
	// a time-based horizontal offset proportional to this).
	sway float32
}

// partRotationAxis returns a part's rotation axis, defaulting to world-up
// when rotationAxis is the zero value.
func partRotationAxis(p treePart) rl.Vector3 {
	if p.rotationAxis.X == 0 && p.rotationAxis.Y == 0 && p.rotationAxis.Z == 0 {
		return worldUp
	}
	return p.rotationAxis
}

// isVerticalAxis reports whether a part rotates around world-up. When true, the
// prop's yaw can be folded into the part's rotation by addition (only valid when
// the two rotations share an axis).
func isVerticalAxis(axis rl.Vector3) bool {
	if axis.X == 0 && axis.Y == 0 && axis.Z == 0 {
		return true
	}
	return axis.X == 0 && axis.Z == 0 && axis.Y != 0
}

// rotateOffsetY scales an offset and rotates it around world-up by yawDeg degrees.
func rotateOffsetY(offset rl.Vector3, scale, yawDeg float32) rl.Vector3 {
	scaled := rl.NewVector3(offset.X*scale, offset.Y*scale, offset.Z*scale)
	if yawDeg == 0 {
		return scaled
	}
	return rl.Vector3RotateByAxisAngle(scaled, worldUp, yawDeg*degToRad)
}

const (
	treeMeshRoot = iota // root flare cone at the trunk's foot
	treeMeshTrunk
	treeMeshCanopyLow
	treeMeshCanopyHigh
	treeMeshCanopySide
	treeMeshCanopyAccent
	// treeMeshBranch is the bough cone bridging trunk → canopy. A BARK part —
	// the variance pass excludes it from canopy jitter/species-tint like root+trunk.
	treeMeshBranch
)

func loadTreeModel(shader rl.Shader, barkTex, leafTex rl.Texture2D) treeModel {
	models := []rl.Model{
		// Root flare cone hugging the base, merging the root toes into one mass.
		treeMeshRoot: rl.LoadModelFromMesh(rl.GenMeshCone(0.30, 0.40, 10)),
		// Trunk: ONE tall cone (continuous taper, no end-cap ledges catching sun
		// as bright rings). Tip buried deep in the canopy; tilted ~3° at the
		// GROUND (cone base at origin) like a real tree leans.
		treeMeshTrunk: rl.LoadModelFromMesh(rl.GenMeshCone(0.20, 3.4, 12)),
		// Canopy lumps: Low is the dominant base mass, High the brighter crown,
		// Side spread laterally, Accent the gilt highlights.
		treeMeshCanopyLow:    rl.LoadModelFromMesh(rl.GenMeshSphere(1.18, 14, 18)),
		treeMeshCanopyHigh:   rl.LoadModelFromMesh(rl.GenMeshSphere(0.96, 14, 18)),
		treeMeshCanopySide:   rl.LoadModelFromMesh(rl.GenMeshSphere(0.68, 12, 16)),
		treeMeshCanopyAccent: rl.LoadModelFromMesh(rl.GenMeshSphere(0.46, 10, 14)),
		// Bough segment — a CONE (limbs taper). Each bough chains two into an elbow.
		treeMeshBranch: rl.LoadModelFromMesh(rl.GenMeshCone(0.082, 0.55, 7)),
	}
	for i := range models {
		tex := leafTex
		if i == treeMeshRoot || i == treeMeshTrunk || i == treeMeshBranch {
			tex = barkTex
		}
		setModelTexture(&models[i], tex)
		attachShader(&models[i], shader)
	}

	// Pastel-saturated canopy — spring green with cream and lemon-gold accents.
	leafBase := color.RGBA{R: 146, G: 204, B: 114, A: 255}
	leafMid := color.RGBA{R: 124, G: 188, B: 106, A: 255}
	leafDeep := color.RGBA{R: 98, G: 160, B: 96, A: 255}
	leafGold := color.RGBA{R: 230, G: 220, B: 142, A: 255}
	leafBloom := color.RGBA{R: 212, G: 232, B: 160, A: 255}

	return treeModel{
		models: models,
		parts: []treePart{
			// Trunk — tapering cone tilted ~3° at the ground; the tilt axis rotates
			// with per-tile yaw so every tree leans its own way.
			{modelIdx: treeMeshTrunk, offset: rl.NewVector3(0, 0.01, 0), scale: rl.NewVector3(1, 1, 1), rotation: 3, rotationAxis: rl.NewVector3(1, 0, 0.4), tint: rl.White},
			// Dominant low canopy, nudged toward the trunk's lean so the crown
			// loads over the bole's top.
			{modelIdx: treeMeshCanopyLow, offset: rl.NewVector3(0.06, 2.55, 0.01), scale: rl.NewVector3(1, 0.95, 1), tint: leafMid},
			// Crown — brighter, catches the sky tint.
			{modelIdx: treeMeshCanopyHigh, offset: rl.NewVector3(-0.05, 3.20, 0.05), scale: rl.NewVector3(1, 1, 1), tint: leafBase},
			// Side lumps spreading wide.
			{modelIdx: treeMeshCanopySide, offset: rl.NewVector3(0.55, 2.90, 0.18), scale: rl.NewVector3(1, 1, 1), tint: leafDeep},
			{modelIdx: treeMeshCanopySide, offset: rl.NewVector3(-0.50, 2.72, -0.18), scale: rl.NewVector3(1, 1, 1), tint: leafMid},
			// Top bloom — bright cream lump at the very top.
			{modelIdx: treeMeshCanopyAccent, offset: rl.NewVector3(0.08, 3.62, -0.06), scale: rl.NewVector3(1, 0.9, 1), tint: leafBloom},
			// Two gold sun-dapple accents.
			{modelIdx: treeMeshCanopyAccent, offset: rl.NewVector3(0.30, 3.40, -0.22), scale: rl.NewVector3(1, 1, 1), tint: leafGold},
			{modelIdx: treeMeshCanopyAccent, offset: rl.NewVector3(-0.26, 3.26, 0.28), scale: rl.NewVector3(1, 1, 1), tint: leafGold},

			// ---- FORM PASS (appended so the canopy lumps above keep their
			// per-index variance) ----
			// No under-canopy occlusion ball — its equator poked past the canopy
			// as a dark saucer ring. The under-shadow comes from the sun shader's
			// NdotL, which is orientation-correct. Each bough is TWO cones meeting
			// at an elbow; joint/tip positions are computed from the axis-angle
			// rotation, with upper segments seated ~0.08 BACK so elbows stay closed
			// across the ±28% height stretch.

			// Base flare — the ONLY root geometry (radiating root cones read as
			// spikes from the player's low angle); ground grip is this + the shadow.
			{modelIdx: treeMeshRoot, offset: rl.NewVector3(0, 0, 0), scale: rl.NewVector3(1, 1, 1), tint: rl.White},

			// Bough A — toward +Z. Lower segment at 30°, upper at 58°, thinner.
			{modelIdx: treeMeshBranch, offset: rl.NewVector3(0.05, 1.30, 0.02), scale: rl.NewVector3(1, 1, 1), rotation: 30, rotationAxis: rl.NewVector3(1, 0, 0.3), tint: rl.White},
			{modelIdx: treeMeshBranch, offset: rl.NewVector3(-0.02, 1.71, 0.25), scale: rl.NewVector3(0.73, 1.13, 0.73), rotation: 58, rotationAxis: rl.NewVector3(1, 0, 0.3), tint: color.RGBA{R: 234, G: 224, B: 212, A: 255}},
			// Bough B — toward +X, higher, mirrored arc.
			{modelIdx: treeMeshBranch, offset: rl.NewVector3(-0.04, 1.48, -0.02), scale: rl.NewVector3(0.98, 0.91, 0.98), rotation: -26, rotationAxis: rl.NewVector3(0.25, 0, 1), tint: rl.White},
			{modelIdx: treeMeshBranch, offset: rl.NewVector3(0.14, 1.86, -0.06), scale: rl.NewVector3(0.67, 1.05, 0.67), rotation: -54, rotationAxis: rl.NewVector3(0.25, 0, 1), tint: color.RGBA{R: 230, G: 220, B: 208, A: 255}},
			// Back stub toward −Z so the crotch reads from every approach. Base
			// seated on the trunk AXIS (an off-axis −Z base would exit the cone).
			{modelIdx: treeMeshBranch, offset: rl.NewVector3(0, 1.58, 0), scale: rl.NewVector3(0.73, 0.78, 0.73), rotation: -38, rotationAxis: rl.NewVector3(1, 0, -0.2), tint: color.RGBA{R: 226, G: 216, B: 202, A: 255}},

			// Leaf tufts on each bough's tip — canopy family (sway + species tint)
			// so no bough ends in a naked spike.
			{modelIdx: treeMeshCanopyAccent, offset: rl.NewVector3(-0.17, 2.06, 0.74), scale: rl.NewVector3(0.78, 0.70, 0.78), tint: leafDeep},
			{modelIdx: treeMeshCanopyAccent, offset: rl.NewVector3(0.60, 2.22, -0.17), scale: rl.NewVector3(0.72, 0.66, 0.72), tint: leafMid},
			{modelIdx: treeMeshCanopyAccent, offset: rl.NewVector3(-0.05, 1.94, -0.27), scale: rl.NewVector3(0.55, 0.50, 0.55), tint: leafDeep},
		},
	}
}

// draw renders the tree at center, scaled, yawed around the trunk's vertical
// axis. Vertical-axis parts get yaw added to their rotation; tilted parts
// compose via yawedTiltAxis so a bough and its glued-on tuft swing together.
func (t treeModel) draw(center rl.Vector3, scale, yaw float32) {
	if scale <= 0 {
		scale = 1
	}
	for _, part := range t.parts {
		offset := rotateOffsetY(part.offset, scale, yaw)
		position := rl.NewVector3(center.X+offset.X, center.Y+offset.Y, center.Z+offset.Z)
		drawScale := rl.NewVector3(part.scale.X*scale, part.scale.Y*scale, part.scale.Z*scale)
		axis := partRotationAxis(part)
		rotation := part.rotation
		if isVerticalAxis(part.rotationAxis) {
			rotation += yaw
		} else {
			axis = yawedTiltAxis(axis, yaw)
		}
		rl.DrawModelEx(t.models[part.modelIdx], position, axis, rotation, drawScale, part.tint)
	}
}

// yawedTiltAxis rotates a part's tilt axis around world-up by yaw degrees.
// For meshes rotationally symmetric about +Y (every mesh here), this equals
// yaw∘tilt up to an invisible pre-spin — the correct yaw composition with one
// vector rotate instead of matrix stacking.
func yawedTiltAxis(axis rl.Vector3, yaw float32) rl.Vector3 {
	if yaw == 0 {
		return axis
	}
	return rl.Vector3RotateByAxisAngle(axis, worldUp, yaw*degToRad)
}

// treePartVariance is the per-part static jitter for one seed: canopy scale
// wobble, nudge offset, and final (species + jitter) tint. Seed-derived and
// frame-invariant, so computed once and cached.
type treePartVariance struct {
	sx, sy, sz     float32
	nudgeX, nudgeZ float32
	tint           color.RGBA
	isCanopy       bool
}

// treeVariance is the seed-dependent part of a tree's shape — NOT scale, yaw,
// or per-frame wind sway. Cached by seed so a forest doesn't re-derive the hash
// mix + per-part jitter + species tint every frame.
type treeVariance struct {
	scaleFactor   float32 // overall = tileScale * scaleFactor
	heightStretch float32
	dropIdx       int
	swayPhase     float32
	parts         []treePartVariance
}

// treeVarianceCache memoizes treeVariance by seed (tileHash(x,z) — deterministic,
// so entries never need invalidation). Touched only from the single-threaded
// render path; one treeModel feeds drawVaried, so keying on seed alone is safe.
var treeVarianceCache = map[uint32]*treeVariance{}

func (t treeModel) variance(seed uint32) *treeVariance {
	if v, ok := treeVarianceCache[seed]; ok {
		return v
	}
	// Mix once so per-byte slices decorrelate even for near-identical seeds.
	mix := seed*hashSalt ^ 0xC2B2AE3D
	mix ^= mix >> 16
	mix *= 0x85EBCA6B
	mix ^= mix >> 13
	frac := func(b byte) float32 { return (float32(int(b)) - 128) / 128 }

	v := treeVariance{
		// Overall girth wobble (±10%), multiplied by tile scale at draw.
		scaleFactor: 1 + frac(byte(mix))*0.10,
		// Height-only stretch (±28%), independent of girth. Bounded so a
		// stretched canopy doesn't punch through the ceiling on indoor maps.
		heightStretch: 1 + frac(byte(mix>>4))*0.28,
		dropIdx:       -1,
		swayPhase:     frac(byte(mix>>22)) * tau,
		parts:         make([]treePartVariance, len(t.parts)),
	}

	// Drop one side-canopy lump ~25% of the time (dropIdx == -1 = draw all).
	// Walks parts to locate side canopies so it survives part-list reshuffles.
	if byte(mix>>8) < 64 {
		nthSide := int(byte(mix>>16)) % 2
		seen := 0
		for i, part := range t.parts {
			if part.modelIdx == treeMeshCanopySide {
				if seen == nthSide {
					v.dropIdx = i
					break
				}
				seen++
			}
		}
	}

	// Per-tile species: ~10% blossom (pink), ~10% autumn (rust-gold), rest green.
	// All share one leaf texture; only the tint changes (speciesCanopyTint).
	speciesRoll := byte(mix >> 20)
	species := treeSpeciesGreen
	switch {
	case speciesRoll < 26:
		species = treeSpeciesBlossom
	case speciesRoll < 52:
		species = treeSpeciesAutumn
	}

	for i, part := range t.parts {
		// Bark parts (root, trunk, boughs) are rigid: no jitter, lean, or
		// species remap — a blossom tree's branches don't turn pink.
		isBark := part.modelIdx == treeMeshRoot || part.modelIdx == treeMeshTrunk ||
			part.modelIdx == treeMeshBranch
		isCanopy := !isBark
		pv := treePartVariance{sx: 1, sy: 1, sz: 1, tint: part.tint, isCanopy: isCanopy}
		if isCanopy {
			// Shifts wrap mod 25 to stay inside the uint32 — unwrapped i*11
			// shifts overflowed (>>33 is 0 in Go), freezing every higher-index
			// lump's jitter at the same constant on every tree.
			pv.sx = 1 + frac(byte(mix>>uint((3+i*5)%25)))*0.14
			pv.sy = 1 + frac(byte(mix>>uint((7+i*9)%25)))*0.14
			pv.sz = 1 + frac(byte(mix>>uint((11+i*13)%25)))*0.14
			pv.nudgeX = frac(byte(mix>>uint((5+i*11)%25))) * 0.20
			pv.nudgeZ = frac(byte(mix>>uint((13+i*17)%25))) * 0.20
			// Species palette first, then ±14 per-channel jitter on top so lumps
			// walk in tone while the family colour is preserved.
			pv.tint = jitterTint(speciesCanopyTint(part.tint, species), mix>>uint((7+i*4)%23), 14)
		}
		v.parts[i] = pv
	}

	treeVarianceCache[seed] = &v
	return &v
}

func (t treeModel) drawVaried(center rl.Vector3, scale, yaw float32, seed uint32) {
	if scale <= 0 {
		scale = 1
	}
	v := t.variance(seed)
	overall := scale * v.scaleFactor

	// Canopy sway — per-seed phase offset so a stand breathes asynchronously.
	// Small amplitude (~0.05); trunks are not swayed, only foliage.
	swayTime := worldFrameClock
	swayX := float32(math.Sin(float64(swayTime*0.85+v.swayPhase))) * 0.05
	swayZ := float32(math.Sin(float64(swayTime*0.72+v.swayPhase+1.3))) * 0.04
	swayY := float32(math.Sin(float64(swayTime*1.05+v.swayPhase*0.7))) * 0.025

	for i, part := range t.parts {
		if i == v.dropIdx {
			continue
		}
		pv := v.parts[i]

		// Height stretch lifts canopy Y offsets and scales the trunk mesh by the
		// same factor so foliage rides on top of the stretched trunk.
		yOffset := part.offset.Y * v.heightStretch
		trunkYScale := float32(1.0)
		if part.modelIdx == treeMeshTrunk {
			trunkYScale = v.heightStretch
		}

		offX := part.offset.X + pv.nudgeX
		offZ := part.offset.Z + pv.nudgeZ
		// Canopy lumps lean in the wind, higher lumps more (lean scales sway by
		// Y offset, capped at 1.4) so the crown drifts further. Trunk/root skipped.
		offsetY := yOffset
		if pv.isCanopy {
			lean := part.offset.Y / 3.0
			if lean > 1.4 {
				lean = 1.4
			}
			offX += swayX * lean
			offZ += swayZ * lean
			offsetY += swayY * lean
		}
		offset := rotateOffsetY(rl.NewVector3(offX, offsetY, offZ), overall, yaw)
		position := rl.NewVector3(center.X+offset.X, center.Y+offset.Y, center.Z+offset.Z)
		drawScale := rl.NewVector3(part.scale.X*pv.sx*overall, part.scale.Y*pv.sy*trunkYScale*overall, part.scale.Z*pv.sz*overall)
		axis := partRotationAxis(part)
		rotation := part.rotation
		if isVerticalAxis(part.rotationAxis) {
			rotation += yaw
		} else {
			// Tilted parts compose yaw via the rotated tilt axis (yawedTiltAxis)
			// so they swing with the tree and tip tufts stay glued to their boughs.
			axis = yawedTiltAxis(axis, yaw)
		}
		rl.DrawModelEx(t.models[part.modelIdx], position, axis, rotation, drawScale, pv.tint)
	}
}

// treeSpecies labels a per-tile painted-canopy palette (drawVaried rolls one
// of three so a forest reads as mixed-species).
type treeSpecies int

const (
	treeSpeciesGreen treeSpecies = iota
	treeSpeciesBlossom
	treeSpeciesAutumn
)

// speciesCanopyTint hue-shifts a leaf tint per species (green passes through),
// preserving brightness so highlight/shadow lumps stay relatively bright/deep.
func speciesCanopyTint(orig color.RGBA, species treeSpecies) color.RGBA {
	// Brightness key — green channel (most of the leaf luminance) plus a touch of R.
	key := (int(orig.G)*2 + int(orig.R)) / 3
	switch species {
	case treeSpeciesGreen:
		return orig
	case treeSpeciesBlossom:
		// Rose pink: R up, G down, B toward warm.
		return color.RGBA{
			R: core.ClampByte(key + 56),
			G: core.ClampByte(key - 22),
			B: core.ClampByte(key + 4),
			A: orig.A,
		}
	case treeSpeciesAutumn:
		// Rust-gold: R up, G toward amber, B down for burnt orange.
		return color.RGBA{
			R: core.ClampByte(key + 52),
			G: core.ClampByte(key - 6),
			B: core.ClampByte(key - 40),
			A: orig.A,
		}
	default:
		panic(fmt.Sprintf("speciesCanopyTint: unhandled treeSpecies %d", species))
	}
}

// jitterTint shifts R/G/B by up to ±amp using bytes from bits; alpha preserved.
func jitterTint(c color.RGBA, bits uint32, amp int) color.RGBA {
	shift := func(v byte, b byte) byte {
		delta := int(b)%(2*amp+1) - amp
		return core.ClampByte(int(v) + delta)
	}
	return color.RGBA{
		R: shift(c.R, byte(bits)),
		G: shift(c.G, byte(bits>>8)),
		B: shift(c.B, byte(bits>>16)),
		A: c.A,
	}
}

func (t treeModel) unload() {
	for i := range t.models {
		rl.UnloadModel(t.models[i])
	}
}

// propModel is a generic multi-mesh prop (boulders, bushes, mushrooms). Same
// shape as treeModel, decoupled so each prop family owns its texture/tint set.
type propModel struct {
	models []rl.Model
	parts  []treePart // reuse the per-part record
}

// registered reports whether this slot holds a built model — the single home for
// the "has parts" presence test used at every prop/decor table lookup + coverage assert.
func (p propModel) registered() bool { return len(p.parts) > 0 }

// draw renders the prop with uniform scale and yaw (see treeModel.draw for yaw
// composition). Parts with non-zero sway get a time-based horizontal offset;
// phase is hashed from world position so adjacent tiles drift independently.
func (p propModel) draw(center rl.Vector3, scale, yaw float32) {
	if scale <= 0 {
		scale = 1
	}
	// Sway is computed lazily the first time a sway>0 part is hit, so rigid props
	// never touch the Mod + two Sin calls. swayTime is the once-per-frame clock.
	var swayX, swayZ float32
	swayReady := false
	for _, part := range p.parts {
		offset := rotateOffsetY(part.offset, scale, yaw)
		position := rl.NewVector3(center.X+offset.X, center.Y+offset.Y, center.Z+offset.Z)
		if part.sway > 0 {
			if !swayReady {
				swayTime := worldFrameClock
				// Position-derived phase so each prop tile lands on a different
				// point in the sway cycle.
				posPhase := float32(math.Mod(float64(center.X)*0.73+float64(center.Z)*1.31, tau))
				swayX = float32(math.Sin(float64(swayTime*1.10+posPhase))) * 0.035
				swayZ = float32(math.Sin(float64(swayTime*0.95+posPhase+1.4))) * 0.030
				swayReady = true
			}
			// Lean scales with Y so taller parts drift further than the base.
			lean := part.sway * (0.4 + part.offset.Y*1.5)
			position.X += swayX * lean
			position.Z += swayZ * lean
		}
		drawScale := rl.NewVector3(part.scale.X*scale, part.scale.Y*scale, part.scale.Z*scale)
		axis := partRotationAxis(part)
		rotation := part.rotation
		if isVerticalAxis(part.rotationAxis) {
			rotation += yaw
		} else {
			// Tilted parts must yaw their tilt AXIS too, else the mesh keeps a
			// world-fixed orientation while its position swings. Mirrors treeModel.
			axis = yawedTiltAxis(axis, yaw)
		}
		rl.DrawModelEx(p.models[part.modelIdx], position, axis, rotation, drawScale, part.tint)
	}
}

func (p propModel) unload() {
	for i := range p.models {
		rl.UnloadModel(p.models[i])
	}
}

// Rock prop mesh indices. rockMeshBase exists as models[0] for world.go's
// drawPebbleCluster (ground scatter); the boulder itself doesn't draw it.
const (
	rockMeshBase  = iota // flat cube — pebble drawer only, not the boulder
	rockMeshLump         // medium faceted lump (5 rings × 6 slices)
	rockMeshChunk        // small faceted chunk (4 rings × 5 slices)
	rockMeshMoss         // untextured moss cushion (tinted at part level)
)

// RockMeshBaseHeight is the GenMeshCube Y dimension for rockMeshBase. Exported
// so drawPebbleCluster computes its y-anchor without baking the literal twice.
const RockMeshBaseHeight = float32(0.36)

// RockMeshBaseHalfHeight is the y-anchor scale for ground-scatter draws of
// rockMeshBase, multiplied by per-instance height so the bottom face lands flush.
const RockMeshBaseHalfHeight = RockMeshBaseHeight / 2

// loadRockProp builds a chunky polygonal boulder: faceted lumps fused at varied
// angles in close-grouped stone greys. Low slice/ring counts (4–6) keep the
// lumps polygonal, not billiard-ball smooth.
func loadRockProp(shader rl.Shader, rockTex rl.Texture2D) propModel {
	models := []rl.Model{
		rockMeshBase: rl.LoadModelFromMesh(rl.GenMeshCube(1.10, RockMeshBaseHeight, 0.95)),
		// Low ring/slice spheres: 5×6 and 4×5 read as rock, not ball.
		rockMeshLump:  rl.LoadModelFromMesh(rl.GenMeshSphere(0.55, 5, 6)),
		rockMeshChunk: rl.LoadModelFromMesh(rl.GenMeshSphere(0.36, 4, 5)),
		// Moss cushion — UNtextured (grain would read as granite), tinted at part level.
		rockMeshMoss: rl.LoadModelFromMesh(rl.GenMeshSphere(0.30, 5, 7)),
	}
	skinExceptMoss(models, rockMeshMoss, shader, rockTex)

	warm, cool, dark, light := stonePaletteWarm, stonePaletteCool, stonePaletteDark, stonePaletteLight

	return propModel{
		models: models,
		parts: []treePart{
			// Main mass: biggest lump, squashed (y 0.85), tilted off world axes.
			{modelIdx: rockMeshLump, offset: rl.NewVector3(-0.10, 0.40, 0.05), scale: rl.NewVector3(1.25, 0.85, 1.20), rotation: 17, rotationAxis: rl.NewVector3(1, 4, 1), tint: warm},

			// Side mass, different tilt axis so its facets break the main lump's.
			{modelIdx: rockMeshLump, offset: rl.NewVector3(0.38, 0.32, -0.22), scale: rl.NewVector3(1.0, 0.75, 1.10), rotation: -28, rotationAxis: rl.NewVector3(2, 5, 1), tint: cool},

			// Top chunk — asymmetric peak, low so the silhouette stays earthbound.
			{modelIdx: rockMeshChunk, offset: rl.NewVector3(-0.22, 0.62, 0.12), scale: rl.NewVector3(1.15, 0.7, 1.15), rotation: 41, rotationAxis: rl.NewVector3(1, 5, 0), tint: dark},

			// Back chip — breaks the +Z silhouette.
			{modelIdx: rockMeshChunk, offset: rl.NewVector3(0.06, 0.26, 0.42), scale: rl.NewVector3(1.05, 0.75, 1.05), rotation: 11, rotationAxis: rl.NewVector3(0, 6, 1), tint: dark},

			// Broken-off pebble at the base flank for weathered ground interest.
			{modelIdx: rockMeshChunk, offset: rl.NewVector3(-0.52, 0.13, 0.08), scale: rl.NewVector3(0.65, 0.45, 0.65), rotation: 65, rotationAxis: rl.NewVector3(1, 3, 0), tint: light},

			// Moss cushions — bright pad on the sun-side shoulder, deeper patch in
			// the crevice between lumps.
			{modelIdx: rockMeshMoss, offset: rl.NewVector3(-0.18, 0.74, 0.10), scale: rl.NewVector3(1.15, 0.34, 1.10), rotation: 24, rotationAxis: rl.NewVector3(1, 6, 0), tint: mossPaletteBright},
			{modelIdx: rockMeshMoss, offset: rl.NewVector3(0.26, 0.54, -0.10), scale: rl.NewVector3(0.85, 0.30, 0.80), rotation: -38, rotationAxis: rl.NewVector3(0, 6, 1), tint: mossPaletteDeep},
		},
	}
}

// loadRockCairnProp builds a 1-tile stacked-stone cairn — three faceted lumps,
// taller than the boulder so it reads as built rather than natural.
func loadRockCairnProp(shader rl.Shader, rockTex rl.Texture2D) propModel {
	// Named mesh indices so reordering re-skins parts in lockstep (the moss-skip
	// stays on the moss mesh).
	const (
		cairnMeshBottom = iota
		cairnMeshMiddle
		cairnMeshTop
		cairnMeshMoss
	)
	models := []rl.Model{
		cairnMeshBottom: rl.LoadModelFromMesh(rl.GenMeshSphere(0.42, 5, 7)),
		cairnMeshMiddle: rl.LoadModelFromMesh(rl.GenMeshSphere(0.32, 5, 6)),
		cairnMeshTop:    rl.LoadModelFromMesh(rl.GenMeshSphere(0.22, 4, 5)),
		cairnMeshMoss:   rl.LoadModelFromMesh(rl.GenMeshSphere(0.30, 5, 7)), // moss cushion (untextured)
	}
	skinExceptMoss(models, cairnMeshMoss, shader, rockTex)
	warm, cool, dark, light := stonePaletteWarm, stonePaletteCool, stonePaletteDark, stonePaletteLight
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: cairnMeshBottom, offset: rl.NewVector3(0, 0.34, 0), scale: rl.NewVector3(1.1, 0.85, 1.1), rotation: 13, rotationAxis: rl.NewVector3(1, 4, 1), tint: warm},
			{modelIdx: cairnMeshMiddle, offset: rl.NewVector3(0.04, 0.78, -0.06), scale: rl.NewVector3(1.0, 0.95, 1.0), rotation: -22, rotationAxis: rl.NewVector3(2, 5, 1), tint: cool},
			{modelIdx: cairnMeshTop, offset: rl.NewVector3(-0.05, 1.10, 0.04), scale: rl.NewVector3(1.0, 0.95, 1.0), rotation: 38, rotationAxis: rl.NewVector3(1, 5, 0), tint: dark},
			// Moss bonnet on the middle stone's shoulder (cairns weather at joints).
			{modelIdx: cairnMeshMoss, offset: rl.NewVector3(-0.10, 0.92, 0.06), scale: rl.NewVector3(0.80, 0.28, 0.75), rotation: 31, rotationAxis: rl.NewVector3(1, 6, 0), tint: mossPaletteDeep},
			// Settled contact pebbles at the foot — grounds the column.
			{modelIdx: cairnMeshTop, offset: rl.NewVector3(0.34, 0.09, 0.16), scale: rl.NewVector3(0.50, 0.35, 0.50), rotation: 57, rotationAxis: rl.NewVector3(1, 3, 0), tint: light},
			{modelIdx: cairnMeshTop, offset: rl.NewVector3(-0.30, 0.07, -0.20), scale: rl.NewVector3(0.42, 0.30, 0.42), rotation: -19, rotationAxis: rl.NewVector3(0, 4, 1), tint: cool},
		},
	}
}

// loadRockFormationProp builds a 2×2 footprint rock formation. Offsets put the
// origin at the CENTER of the 2×2 footprint, so the renderer draws it at
// (anchorX+0.5, anchorZ+0.5). Lumps spread ~1.6 world units to fill the span.
func loadRockFormationProp(shader rl.Shader, rockTex rl.Texture2D) propModel {
	// Named mesh indices so reordering re-skins parts in lockstep.
	const (
		formationMeshCore = iota
		formationMeshShoulder
		formationMeshChunk
		formationMeshCrown
		formationMeshMoss
	)
	models := []rl.Model{
		formationMeshCore:     rl.LoadModelFromMesh(rl.GenMeshSphere(0.95, 6, 7)),
		formationMeshShoulder: rl.LoadModelFromMesh(rl.GenMeshSphere(0.70, 5, 6)),
		formationMeshChunk:    rl.LoadModelFromMesh(rl.GenMeshSphere(0.55, 5, 6)),
		formationMeshCrown:    rl.LoadModelFromMesh(rl.GenMeshSphere(0.40, 4, 5)),
		formationMeshMoss:     rl.LoadModelFromMesh(rl.GenMeshSphere(0.34, 5, 7)), // moss cushion (untextured)
	}
	skinExceptMoss(models, formationMeshMoss, shader, rockTex)
	warm, cool, dark, light := stonePaletteWarm, stonePaletteCool, stonePaletteDark, stonePaletteLight
	return propModel{
		models: models,
		parts: []treePart{
			// Central mass — biggest lump anchoring the cluster.
			{modelIdx: formationMeshCore, offset: rl.NewVector3(0, 0.75, 0), scale: rl.NewVector3(1.05, 0.95, 1.05), rotation: 17, rotationAxis: rl.NewVector3(1, 4, 1), tint: warm},
			// NE shoulder pushing into the +X/+Z quadrant.
			{modelIdx: formationMeshShoulder, offset: rl.NewVector3(0.62, 0.55, 0.55), scale: rl.NewVector3(1.0, 0.85, 1.0), rotation: -28, rotationAxis: rl.NewVector3(2, 5, 1), tint: cool},
			// SW chunk, lower profile.
			{modelIdx: formationMeshChunk, offset: rl.NewVector3(-0.55, 0.42, -0.45), scale: rl.NewVector3(1.0, 0.8, 1.0), rotation: 41, rotationAxis: rl.NewVector3(1, 5, 0), tint: dark},
			// NW protrusion.
			{modelIdx: formationMeshChunk, offset: rl.NewVector3(-0.45, 0.50, 0.58), scale: rl.NewVector3(0.9, 0.75, 0.9), rotation: -52, rotationAxis: rl.NewVector3(0, 6, 1), tint: warm},
			// SE buttress.
			{modelIdx: formationMeshChunk, offset: rl.NewVector3(0.58, 0.46, -0.52), scale: rl.NewVector3(0.95, 0.8, 0.95), rotation: 11, rotationAxis: rl.NewVector3(1, 3, 0), tint: cool},
			// Crown lump on top.
			{modelIdx: formationMeshCrown, offset: rl.NewVector3(0.05, 1.18, -0.08), scale: rl.NewVector3(1.0, 0.85, 1.0), rotation: 65, rotationAxis: rl.NewVector3(1, 4, 0), tint: light},
			// Cap accent — slight asymmetric peak.
			{modelIdx: formationMeshCrown, offset: rl.NewVector3(-0.18, 1.32, 0.10), scale: rl.NewVector3(0.8, 0.75, 0.8), rotation: -25, rotationAxis: rl.NewVector3(2, 4, 1), tint: dark},
			// Moss in the joint crevices (not on peaks): main/NE seam, SW pocket,
			// high where the crown meets the central mass.
			{modelIdx: formationMeshMoss, offset: rl.NewVector3(0.36, 0.92, 0.30), scale: rl.NewVector3(1.05, 0.32, 1.00), rotation: 18, rotationAxis: rl.NewVector3(1, 6, 0), tint: mossPaletteBright},
			{modelIdx: formationMeshMoss, offset: rl.NewVector3(-0.42, 0.66, -0.30), scale: rl.NewVector3(0.90, 0.28, 0.85), rotation: -44, rotationAxis: rl.NewVector3(0, 6, 1), tint: mossPaletteDeep},
			{modelIdx: formationMeshMoss, offset: rl.NewVector3(-0.06, 1.24, 0.02), scale: rl.NewVector3(0.70, 0.26, 0.66), rotation: 52, rotationAxis: rl.NewVector3(1, 5, 1), tint: mossPaletteDeep},
		},
	}
}

// loadArchwayDecor builds a stone archway spanning 1×2 tiles along +X. Pillars
// sit at (−1,0) and (+1,0) so the origin lands BETWEEN the tiles (renderer
// offsets +0.5 tile east). Marble palette to match the pillars/statues.
// Named mesh indices so reordering the slice re-skins the right mesh.
const (
	archMeshShaft    = iota // pillar shaft
	archMeshCapital         // pillar capital
	archMeshKeystone        // arch keystone slab
	archMeshPlinth          // base plinth
)

func loadArchwayDecor(shader rl.Shader, marbleTex rl.Texture2D) propModel {
	models := []rl.Model{
		archMeshShaft:    rl.LoadModelFromMesh(rl.GenMeshCube(0.42, 2.10, 0.42)),
		archMeshCapital:  rl.LoadModelFromMesh(rl.GenMeshCube(0.58, 0.20, 0.58)),
		archMeshKeystone: rl.LoadModelFromMesh(rl.GenMeshCube(2.20, 0.38, 0.46)),
		archMeshPlinth:   rl.LoadModelFromMesh(rl.GenMeshCube(0.48, 0.18, 0.48)),
	}
	textureAndShade(models, shader, marbleTex)
	stone := marblePaleBody
	stoneCool := rl.NewColor(204, 196, 174, 255)
	stoneDark := rl.NewColor(178, 170, 152, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Left pillar: plinth, shaft, capital — stacked at -1 tile X.
			{modelIdx: archMeshPlinth, offset: rl.NewVector3(-1.00, 0.09, 0), scale: rl.NewVector3(1, 1, 1), tint: stoneDark},
			{modelIdx: archMeshShaft, offset: rl.NewVector3(-1.00, 1.20, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			{modelIdx: archMeshCapital, offset: rl.NewVector3(-1.00, 2.35, 0), scale: rl.NewVector3(1, 1, 1), tint: stoneCool},
			// Right pillar: mirror at +1 tile X.
			{modelIdx: archMeshPlinth, offset: rl.NewVector3(1.00, 0.09, 0), scale: rl.NewVector3(1, 1, 1), tint: stoneDark},
			{modelIdx: archMeshShaft, offset: rl.NewVector3(1.00, 1.20, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			{modelIdx: archMeshCapital, offset: rl.NewVector3(1.00, 2.35, 0), scale: rl.NewVector3(1, 1, 1), tint: stoneCool},
			// Keystone slab spanning both pillars at the top.
			{modelIdx: archMeshKeystone, offset: rl.NewVector3(0, 2.65, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
		},
	}
}

// loadBushProp builds a leaf-cluster bush with flower blooms across the top.
// Scale 1.0 = large (blocks); ~0.5 = small.
func loadBushProp(shader rl.Shader, leafTex rl.Texture2D) propModel {
	leafLump := rl.LoadModelFromMesh(rl.GenMeshSphere(0.62, 12, 16))
	leafLumpSm := rl.LoadModelFromMesh(rl.GenMeshSphere(0.46, 10, 14))
	bloom := rl.LoadModelFromMesh(rl.GenMeshSphere(0.085, 8, 10))
	// Twig core — visible in the gaps between low lumps, sells a woody heart.
	twig := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.07, 0.34, 7))
	models := []rl.Model{leafLump, leafLumpSm, bloom, twig}
	setModelTexture(&models[0], leafTex)
	setModelTexture(&models[1], leafTex)
	shadeAll(models, shader)
	// Pastel-saturated bush — spring-green lumps with soft blooms.
	leafBase := color.RGBA{R: 142, G: 200, B: 112, A: 255}
	leafDeep := color.RGBA{R: 102, G: 164, B: 100, A: 255}
	leafGold := color.RGBA{R: 224, G: 230, B: 152, A: 255}
	leafShadow := color.RGBA{R: 76, G: 122, B: 82, A: 255}
	twigBrown := color.RGBA{R: 112, G: 84, B: 58, A: 255}
	bloomYellow := color.RGBA{R: 242, G: 216, B: 122, A: 255}
	bloomWhite := color.RGBA{R: 244, G: 240, B: 226, A: 255}
	bloomPink := color.RGBA{R: 238, G: 176, B: 198, A: 255}
	return propModel{
		models: models,
		parts: []treePart{
			// Woody heart first: rigid twig, then a deep-shadow inner lump (core
			// shadow so the bright lumps read as one shrub, not three balloons).
			{modelIdx: 3, offset: rl.NewVector3(0, 0, 0), scale: rl.NewVector3(1, 1, 1), tint: twigBrown},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.40, 0), scale: rl.NewVector3(1.25, 0.85, 1.25), tint: leafShadow, sway: 0.40},
			// Three overlapping leaf lumps — dominant base + two sides.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.52, 0), scale: rl.NewVector3(1, 0.92, 1), tint: leafBase, sway: 0.55},
			{modelIdx: 1, offset: rl.NewVector3(0.34, 0.68, 0.20), scale: rl.NewVector3(1, 1, 1), tint: leafDeep, sway: 0.65},
			{modelIdx: 1, offset: rl.NewVector3(-0.32, 0.64, -0.18), scale: rl.NewVector3(1, 1, 1), tint: leafGold, sway: 0.65},
			// Ground skirt — two flattened lumps so the silhouette pools at the soil.
			{modelIdx: 1, offset: rl.NewVector3(0.26, 0.24, -0.28), scale: rl.NewVector3(0.95, 0.55, 0.95), tint: leafDeep, sway: 0.30},
			{modelIdx: 1, offset: rl.NewVector3(-0.30, 0.22, 0.24), scale: rl.NewVector3(0.90, 0.50, 0.90), tint: leafShadow, sway: 0.30},
			// Wildflower blooms — three colours, sway more than the leaves.
			{modelIdx: 2, offset: rl.NewVector3(0.08, 0.96, 0.10), scale: rl.NewVector3(1, 1, 1), tint: bloomYellow, sway: 0.85},
			{modelIdx: 2, offset: rl.NewVector3(-0.22, 0.84, 0.04), scale: rl.NewVector3(1, 1, 1), tint: bloomWhite, sway: 0.85},
			{modelIdx: 2, offset: rl.NewVector3(0.20, 0.88, -0.18), scale: rl.NewVector3(1, 1, 1), tint: bloomPink, sway: 0.85},
		},
	}
}

// loadMushroomProp builds a mushroom trio: one red-cap toadstool plus two
// smaller cream/apricot companions for a fairy-ring read. Tint-only (no texture).
func loadMushroomProp(shader rl.Shader) propModel {
	stem := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.05, 0.16, 8))
	capDome := rl.LoadModelFromMesh(rl.GenMeshSphere(0.15, 10, 12))
	spot := rl.LoadModelFromMesh(rl.GenMeshSphere(0.028, 6, 8))
	smallStem := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.03, 0.10, 8))
	smallCap := rl.LoadModelFromMesh(rl.GenMeshSphere(0.085, 8, 10))
	// Gill plate under the cap rim — the underside is half of what the player
	// sees at their low angle; without it the cap floats on a bare stick.
	gill := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.125, 0.035, 12))
	models := []rl.Model{stem, capDome, spot, smallStem, smallCap, gill}
	shadeAll(models, shader)
	stemTint := color.RGBA{R: 224, G: 218, B: 200, A: 255}
	stemDarker := color.RGBA{R: 200, G: 192, B: 172, A: 255}
	capRed := color.RGBA{R: 188, G: 92, B: 86, A: 255}
	capCream := color.RGBA{R: 218, G: 198, B: 160, A: 255}
	capApricot := color.RGBA{R: 210, G: 162, B: 132, A: 255}
	spotWhite := color.RGBA{R: 228, G: 224, B: 212, A: 255}
	gillShade := color.RGBA{R: 186, G: 172, B: 146, A: 255}
	return propModel{
		models: models,
		parts: []treePart{
			// Main toadstool — stem, gill plate, domed red cap (tilted a few
			// degrees), four white spots across the dome.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.02, 0), scale: rl.NewVector3(1, 1, 1), tint: stemTint},
			{modelIdx: 5, offset: rl.NewVector3(0, 0.155, 0), scale: rl.NewVector3(1, 1, 1), rotation: 7, rotationAxis: rl.NewVector3(1, 8, 0), tint: gillShade},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.19, 0), scale: rl.NewVector3(1, 0.72, 1), rotation: 7, rotationAxis: rl.NewVector3(1, 8, 0), tint: capRed},
			{modelIdx: 2, offset: rl.NewVector3(0.05, 0.245, 0.02), scale: rl.NewVector3(1, 1, 1), tint: spotWhite},
			{modelIdx: 2, offset: rl.NewVector3(-0.04, 0.24, 0.05), scale: rl.NewVector3(1, 1, 1), tint: spotWhite},
			{modelIdx: 2, offset: rl.NewVector3(0.02, 0.255, -0.06), scale: rl.NewVector3(1, 1, 1), tint: spotWhite},
			{modelIdx: 2, offset: rl.NewVector3(-0.06, 0.23, -0.03), scale: rl.NewVector3(1, 1, 1), tint: spotWhite},
			// Companion 1 — small cream cap with its own gill disc.
			{modelIdx: 3, offset: rl.NewVector3(0.18, 0.01, 0.12), scale: rl.NewVector3(1, 1, 1), tint: stemDarker},
			{modelIdx: 5, offset: rl.NewVector3(0.18, 0.085, 0.12), scale: rl.NewVector3(0.60, 0.8, 0.60), tint: gillShade},
			{modelIdx: 4, offset: rl.NewVector3(0.18, 0.11, 0.12), scale: rl.NewVector3(1, 0.74, 1), rotation: -6, rotationAxis: rl.NewVector3(0, 8, 1), tint: capCream},
			// Companion 2 — apricot cap on the other side.
			{modelIdx: 3, offset: rl.NewVector3(-0.16, 0.01, -0.14), scale: rl.NewVector3(0.95, 0.95, 0.95), tint: stemDarker},
			{modelIdx: 5, offset: rl.NewVector3(-0.16, 0.078, -0.14), scale: rl.NewVector3(0.55, 0.8, 0.55), tint: gillShade},
			{modelIdx: 4, offset: rl.NewVector3(-0.16, 0.10, -0.14), scale: rl.NewVector3(0.92, 0.72, 0.92), rotation: 9, rotationAxis: rl.NewVector3(1, 8, 0.4), tint: capApricot},
		},
	}
}

// loadChestBodyProp builds the wooden chest body: corner straps, two hoop
// bands, a front lockplate, and a jewel pip. Bark texture on wood; default
// material on metal parts so they read as cast bronze. Dimensions match the
// prior raw-cube chest so prompt anchor / shadow / collision don't retune.
func loadChestBodyProp(shader rl.Shader, barkTex rl.Texture2D) propModel {
	wood := rl.LoadModelFromMesh(rl.GenMeshCube(0.62, 0.46, 0.50))
	setModelTexture(&wood, barkTex)
	strap := rl.LoadModelFromMesh(rl.GenMeshCube(0.06, 0.48, 0.06))
	hoop := rl.LoadModelFromMesh(rl.GenMeshCube(0.66, 0.06, 0.54))
	lockplate := rl.LoadModelFromMesh(rl.GenMeshCube(0.20, 0.22, 0.04))
	jewel := rl.LoadModelFromMesh(rl.GenMeshSphere(0.045, 8, 10))
	models := []rl.Model{wood, strap, hoop, lockplate, jewel}
	shadeAll(models, shader)
	woodTint := chestBodyColor
	// Brass banding shared with the lid via chestMetal* so the pieces can't drift.
	metalDark := chestMetalDark
	metalBright := chestMetalBright
	jewelTint := rl.NewColor(198, 92, 80, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Wood body — base flush against the floor.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.23, 0), scale: rl.NewVector3(1, 1, 1), tint: woodTint},
			// Four corner straps, slightly proud so they read as raised ironwork.
			{modelIdx: 1, offset: rl.NewVector3(0.31, 0.24, 0.26), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			{modelIdx: 1, offset: rl.NewVector3(-0.31, 0.24, 0.26), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			{modelIdx: 1, offset: rl.NewVector3(0.31, 0.24, -0.26), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			{modelIdx: 1, offset: rl.NewVector3(-0.31, 0.24, -0.26), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			// Bottom + top hoop bands; the top catches the lid seam.
			{modelIdx: 2, offset: rl.NewVector3(0, 0.05, 0), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.43, 0), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			// Front lockplate.
			{modelIdx: 3, offset: rl.NewVector3(0, 0.28, 0.27), scale: rl.NewVector3(1, 1, 1), tint: metalBright},
			// Lock jewel — the "treasure inside" cue.
			{modelIdx: 4, offset: rl.NewVector3(0, 0.30, 0.30), scale: rl.NewVector3(1, 1, 1), tint: jewelTint},
		},
	}
}

// loadChestLidProp builds the chest lid (corner caps, a bottom hoop, lid wood
// wider than the body). Drawn separately so the looted path can lift+tilt it.
// Centre is at the lid's vertical midpoint, positioned by passing the body top Y.
func loadChestLidProp(shader rl.Shader, barkTex rl.Texture2D) propModel {
	wood := rl.LoadModelFromMesh(rl.GenMeshCube(0.68, 0.18, 0.56))
	setModelTexture(&wood, barkTex)
	cornerCap := rl.LoadModelFromMesh(rl.GenMeshCube(0.10, 0.20, 0.10))
	hoop := rl.LoadModelFromMesh(rl.GenMeshCube(0.70, 0.05, 0.58))
	models := []rl.Model{wood, cornerCap, hoop}
	shadeAll(models, shader)
	woodTint := chestLidColor
	metalDark := chestMetalDark
	return propModel{
		models: models,
		parts: []treePart{
			// Wood lid — centred so the body-top Y centres it on the seam.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.09, 0), scale: rl.NewVector3(1, 1, 1), tint: woodTint},
			// Four corner caps.
			{modelIdx: 1, offset: rl.NewVector3(0.31, 0.10, 0.25), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			{modelIdx: 1, offset: rl.NewVector3(-0.31, 0.10, 0.25), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			{modelIdx: 1, offset: rl.NewVector3(0.31, 0.10, -0.25), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			{modelIdx: 1, offset: rl.NewVector3(-0.31, 0.10, -0.25), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			// Bottom hoop — catches the body's top hoop so they read as ringed.
			{modelIdx: 2, offset: rl.NewVector3(0, 0.025, 0), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
		},
	}
}

// --- Inhabited / ruined props ---------------------------------------------
// Crates, barrels, urns, stalagmites, pillars, statues, obelisks, fountains.
// Low-poly faceted vocabulary; diffuse textures passed in so resources.go owns
// the texture lifetime.

// loadCrateProp builds a wooden crate: main cube wrapped in darker trim cubes
// (top/bottom/corner edges) so it reads as banded boards. Bark texture for grain.
func loadCrateProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCube(0.86, 0.86, 0.86)),
		rl.LoadModelFromMesh(rl.GenMeshCube(0.92, 0.08, 0.92)),
		rl.LoadModelFromMesh(rl.GenMeshCube(0.08, 0.86, 0.08)),
	}
	textureAndShade(models, shader, woodTex)
	// Pastel pecan crate wood with warm-brown banding (not near-black) so it
	// reads gently in spooky dungeon lighting.
	wood := rl.NewColor(184, 144, 102, 255)
	band := rl.NewColor(124, 92, 60, 255)
	corner := rl.NewColor(104, 76, 50, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Main box, flush on the ground.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.43, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			// Top + bottom rim, proud of the faces so it reads as raised banding.
			{modelIdx: 1, offset: rl.NewVector3(0, 0.84, 0), scale: rl.NewVector3(1, 1, 1), tint: band},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.04, 0), scale: rl.NewVector3(1, 1, 1), tint: band},
			// Four corner straps, taller than the box so they read as brackets.
			{modelIdx: 2, offset: rl.NewVector3(0.43, 0.43, 0.43), scale: rl.NewVector3(1, 1, 1), tint: corner},
			{modelIdx: 2, offset: rl.NewVector3(-0.43, 0.43, 0.43), scale: rl.NewVector3(1, 1, 1), tint: corner},
			{modelIdx: 2, offset: rl.NewVector3(0.43, 0.43, -0.43), scale: rl.NewVector3(1, 1, 1), tint: corner},
			{modelIdx: 2, offset: rl.NewVector3(-0.43, 0.43, -0.43), scale: rl.NewVector3(1, 1, 1), tint: corner},
		},
	}
}

// loadBarrelProp builds a banded barrel: a tall cylinder with three hoop bands
// and proud top/bottom caps so it reads as a barrel, not a smooth canister.
func loadBarrelProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.42, 1.0, 18)),
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.45, 0.07, 20)),
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.44, 0.06, 18)),
	}
	textureAndShade(models, shader, woodTex)
	wood := rl.NewColor(192, 150, 104, 255)
	hoop := rl.NewColor(110, 80, 52, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.05, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			// Top + bottom caps — lid + base ring.
			{modelIdx: 1, offset: rl.NewVector3(0, 1.04, 0), scale: rl.NewVector3(1, 1, 1), tint: hoop},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.02, 0), scale: rl.NewVector3(1, 1, 1), tint: hoop},
			// Three hoop bands climbing the staves.
			{modelIdx: 2, offset: rl.NewVector3(0, 0.22, 0), scale: rl.NewVector3(1, 1, 1), tint: hoop},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.54, 0), scale: rl.NewVector3(1, 1, 1), tint: hoop},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.86, 0), scale: rl.NewVector3(1, 1, 1), tint: hoop},
		},
	}
}

// loadUrnProp builds a belly-shouldered ceramic urn (flattened sphere body,
// foot, neck, rim flare) reading as "amphora" not "vase". Terracotta texture.
func loadUrnProp(shader rl.Shader, terracottaTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.36, 10, 14)),     // belly
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.18, 0.20, 16)), // neck
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.23, 0.05, 18)), // rim flare
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.20, 0.06, 14)), // foot
	}
	textureAndShade(models, shader, terracottaTex)
	clay := rl.NewColor(196, 122, 80, 255)
	clayDeep := rl.NewColor(140, 78, 52, 255)
	rim := rl.NewColor(112, 60, 36, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Foot, flush to ground.
			{modelIdx: 3, offset: rl.NewVector3(0, 0.02, 0), scale: rl.NewVector3(1, 1, 1), tint: clayDeep},
			// Belly, squashed (y 0.92) for chubby shoulders.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.44, 0), scale: rl.NewVector3(1, 0.92, 1), tint: clay},
			// Neck.
			{modelIdx: 1, offset: rl.NewVector3(0, 0.78, 0), scale: rl.NewVector3(1, 1, 1), tint: clayDeep},
			// Rim flare — the amphora lip.
			{modelIdx: 2, offset: rl.NewVector3(0, 0.97, 0), scale: rl.NewVector3(1, 1, 1), tint: rim},
		},
	}
}

// loadStalagmiteProp builds a tapered stone spire: four faceted spheres of
// shrinking radius narrowing to a point.
func loadStalagmiteProp(shader rl.Shader, stoneTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.45, 5, 7)),
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.32, 5, 7)),
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.20, 5, 6)),
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.10, 5, 6)),
	}
	textureAndShade(models, shader, stoneTex)
	baseTint := rl.NewColor(206, 200, 188, 255)
	midTint := rl.NewColor(216, 210, 196, 255)
	highTint := rl.NewColor(228, 222, 208, 255)
	tipTint := rl.NewColor(232, 226, 214, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.40, 0), scale: rl.NewVector3(1.0, 0.85, 1.0), rotation: 13, rotationAxis: rl.NewVector3(1, 4, 0), tint: baseTint},
			{modelIdx: 1, offset: rl.NewVector3(0.04, 0.85, -0.02), scale: rl.NewVector3(1.0, 0.95, 1.0), rotation: -7, rotationAxis: rl.NewVector3(0, 5, 1), tint: midTint},
			{modelIdx: 2, offset: rl.NewVector3(-0.02, 1.18, 0.03), scale: rl.NewVector3(1.0, 1.0, 1.0), rotation: 21, rotationAxis: rl.NewVector3(1, 5, 0), tint: highTint},
			{modelIdx: 3, offset: rl.NewVector3(0.01, 1.40, 0), scale: rl.NewVector3(1.0, 1.1, 1.0), tint: tipTint},
		},
	}
}

// loadPillarProp builds a Doric-ish column: base, cylindrical shaft, capital,
// abacus slab. Marble texture; tint walks base→capital (dust settles low).
func loadPillarProp(shader rl.Shader, marbleTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCube(0.72, 0.18, 0.72)),   // plinth
		rl.LoadModelFromMesh(rl.GenMeshCube(0.62, 0.10, 0.62)),   // base
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.26, 2.05, 18)), // shaft
		rl.LoadModelFromMesh(rl.GenMeshCube(0.62, 0.16, 0.62)),   // echinus
		rl.LoadModelFromMesh(rl.GenMeshCube(0.74, 0.08, 0.74)),   // abacus
	}
	textureAndShade(models, shader, marbleTex)
	baseTint := rl.NewColor(206, 200, 184, 255)
	shaftTint := marblePaleBody
	capTint := marblePaleCap
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.09, 0), scale: rl.NewVector3(1, 1, 1), tint: baseTint},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.23, 0), scale: rl.NewVector3(1, 1, 1), tint: baseTint},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.30, 0), scale: rl.NewVector3(1, 1, 1), tint: shaftTint},
			{modelIdx: 3, offset: rl.NewVector3(0, 2.40, 0), scale: rl.NewVector3(1, 1, 1), tint: capTint},
			{modelIdx: 4, offset: rl.NewVector3(0, 2.52, 0), scale: rl.NewVector3(1, 1, 1), tint: capTint},
		},
	}
}

// loadBrokenPillarProp builds a broken pillar stub: same plinth as the intact
// pillar, shaft cut chest-high, topped with an off-axis rubble cube.
func loadBrokenPillarProp(shader rl.Shader, marbleTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCube(0.72, 0.18, 0.72)),
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.26, 0.90, 18)),
		rl.LoadModelFromMesh(rl.GenMeshCube(0.40, 0.18, 0.34)),
	}
	textureAndShade(models, shader, marbleTex)
	baseTint := rl.NewColor(196, 188, 170, 255)
	shaftTint := rl.NewColor(214, 206, 188, 255)
	rubbleTint := rl.NewColor(168, 160, 144, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.09, 0), scale: rl.NewVector3(1, 1, 1), tint: baseTint},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.22, 0), scale: rl.NewVector3(1, 1, 1), tint: shaftTint},
			// Jagged break — off-vertical cube so the top reads as sheared.
			{modelIdx: 2, offset: rl.NewVector3(0.04, 1.18, 0.03), scale: rl.NewVector3(1, 1, 1), rotation: 12, rotationAxis: rl.NewVector3(1, 0, 2), tint: rubbleTint},
		},
	}
}

// loadStatueProp builds a humanoid statue on a pedestal. No separate arms —
// the shoulders slab covers the silhouette at camera distance. Marble texture.
func loadStatueProp(shader rl.Shader, marbleTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCube(0.92, 0.24, 0.92)),   // pedestal
		rl.LoadModelFromMesh(rl.GenMeshCube(0.55, 0.14, 0.55)),   // statue base
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.20, 0.55, 14)), // legs
		rl.LoadModelFromMesh(rl.GenMeshCube(0.48, 0.62, 0.30)),   // torso
		rl.LoadModelFromMesh(rl.GenMeshCube(0.66, 0.14, 0.34)),   // shoulders
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.18, 10, 12)),     // head
	}
	textureAndShade(models, shader, marbleTex)
	pedTint := rl.NewColor(192, 184, 168, 255)
	bodyTint := marblePaleBody
	headTint := marblePaleCap
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.12, 0), scale: rl.NewVector3(1, 1, 1), tint: pedTint},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.31, 0), scale: rl.NewVector3(1, 1, 1), tint: pedTint},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.38, 0), scale: rl.NewVector3(1, 1, 1), tint: bodyTint},
			{modelIdx: 3, offset: rl.NewVector3(0, 1.24, 0), scale: rl.NewVector3(1, 1, 1), tint: bodyTint},
			{modelIdx: 4, offset: rl.NewVector3(0, 1.62, 0), scale: rl.NewVector3(1, 1, 1), tint: bodyTint},
			{modelIdx: 5, offset: rl.NewVector3(0, 1.85, 0), scale: rl.NewVector3(1, 1, 1), tint: headTint},
		},
	}
}

// loadObeliskProp builds a four-sided shaft capped by a pyramid (4-slice sphere
// reads as a four-sided peak). Granite texture sets it apart from the marble props.
func loadObeliskProp(shader rl.Shader, graniteTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCube(0.88, 0.14, 0.88)), // base step
		rl.LoadModelFromMesh(rl.GenMeshCube(0.56, 2.20, 0.56)), // shaft
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.40, 4, 6)),     // pyramid cap (low-slice)
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.08, 6, 6)),     // apex
	}
	textureAndShade(models, shader, graniteTex)
	baseTint := rl.NewColor(70, 74, 86, 255)
	shaftTint := rl.NewColor(92, 96, 110, 255)
	capTint := rl.NewColor(126, 130, 146, 255)
	apexTint := rl.NewColor(186, 188, 198, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.07, 0), scale: rl.NewVector3(1, 1, 1), tint: baseTint},
			{modelIdx: 1, offset: rl.NewVector3(0, 1.22, 0), scale: rl.NewVector3(1, 1, 1), tint: shaftTint},
			// Pyramid cap, flattened so it reads as a tall pyramid not a lid.
			{modelIdx: 2, offset: rl.NewVector3(0, 2.55, 0), scale: rl.NewVector3(0.85, 0.65, 0.85), rotation: 45, tint: capTint},
			{modelIdx: 3, offset: rl.NewVector3(0, 2.86, 0), scale: rl.NewVector3(1, 1, 1), tint: apexTint},
		},
	}
}

// loadFountainProp builds a round stone fountain: basin, water disc, spout
// column, splash sphere. Marble on stone; water disc stays default-textured.
func loadFountainProp(shader rl.Shader, marbleTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.78, 0.42, 24)), // outer basin
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.66, 0.06, 22)), // water disc
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.12, 0.45, 12)), // central spout
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.18, 10, 12)),     // splash bowl
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.82, 0.10, 24)), // rim coping
	}
	// Marble on stone parts; water disc stays default so its tint isn't muddied.
	for _, i := range []int{0, 2, 3, 4} {
		setModelTexture(&models[i], marbleTex)
	}
	shadeAll(models, shader)
	stone := rl.NewColor(208, 202, 188, 255)
	rim := rl.NewColor(186, 178, 164, 255)
	water := rl.NewColor(100, 168, 222, 245)
	highlight := rl.NewColor(196, 226, 244, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.02, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			{modelIdx: 4, offset: rl.NewVector3(0, 0.42, 0), scale: rl.NewVector3(1, 1, 1), tint: rim},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.40, 0), scale: rl.NewVector3(1, 1, 1), tint: water},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.44, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			{modelIdx: 3, offset: rl.NewVector3(0, 0.96, 0), scale: rl.NewVector3(1, 1, 1), tint: highlight},
		},
	}
}

// --- Soft decor (non-blocking) --------------------------------------------
// Small (sub-tile), passable, cheapest primitives. Author-placed via the decor layer.

// loadTallGrassProp builds a clump of five thin tall cubes tilted outward.
func loadTallGrassProp(shader rl.Shader) propModel {
	blade := rl.LoadModelFromMesh(rl.GenMeshCube(0.04, 0.34, 0.04))
	attachShader(&blade, shader)
	models := []rl.Model{blade}
	// Muted grass-blade tints to match the new ground palette.
	light := rl.NewColor(148, 184, 112, 255)
	mid := rl.NewColor(112, 158, 96, 255)
	deep := rl.NewColor(80, 124, 78, 255)
	gold := rl.NewColor(196, 196, 134, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.17, 0), scale: rl.NewVector3(1, 1, 1), rotation: 6, rotationAxis: rl.NewVector3(0, 0, 1), tint: light, sway: 1.0},
			{modelIdx: 0, offset: rl.NewVector3(0.14, 0.16, 0.08), scale: rl.NewVector3(1, 0.92, 1), rotation: -10, rotationAxis: rl.NewVector3(1, 0, 0), tint: mid, sway: 1.0},
			{modelIdx: 0, offset: rl.NewVector3(-0.12, 0.15, 0.10), scale: rl.NewVector3(1, 0.86, 1), rotation: 14, rotationAxis: rl.NewVector3(1, 0, 1), tint: deep, sway: 1.0},
			{modelIdx: 0, offset: rl.NewVector3(0.08, 0.18, -0.14), scale: rl.NewVector3(1, 1.05, 1), rotation: -16, rotationAxis: rl.NewVector3(0, 0, 1), tint: mid, sway: 1.0},
			{modelIdx: 0, offset: rl.NewVector3(-0.10, 0.16, -0.06), scale: rl.NewVector3(1, 0.95, 1), rotation: 22, rotationAxis: rl.NewVector3(1, 0, 0), tint: gold, sway: 1.0},
		},
	}
}

// loadFlowerProp builds a wildflower clump: four stems with blooms over petal
// halos, pistil pips, and three ground leaves. Tight warm palette.
func loadFlowerProp(shader rl.Shader) propModel {
	stem := rl.LoadModelFromMesh(rl.GenMeshCube(0.026, 0.24, 0.026))
	// Petal halo — flat cube reading as the open palm of petals under the bloom.
	petal := rl.LoadModelFromMesh(rl.GenMeshCube(0.12, 0.018, 0.12))
	bloom := rl.LoadModelFromMesh(rl.GenMeshSphere(0.075, 10, 12))
	// Pistil — gold pip on top of each bloom.
	pistil := rl.LoadModelFromMesh(rl.GenMeshSphere(0.022, 6, 8))
	leaf := rl.LoadModelFromMesh(rl.GenMeshCube(0.055, 0.022, 0.075))
	models := []rl.Model{stem, petal, bloom, pistil, leaf}
	shadeAll(models, shader)
	stemTint := rl.NewColor(98, 138, 88, 255)
	leafTint := rl.NewColor(118, 158, 100, 255)
	yellow := rl.NewColor(214, 196, 116, 255)
	yellowPetal := rl.NewColor(224, 208, 144, 255)
	pink := rl.NewColor(208, 152, 174, 255)
	pinkPetal := rl.NewColor(220, 184, 198, 255)
	white := rl.NewColor(228, 224, 214, 255)
	whitePetal := rl.NewColor(228, 220, 206, 255)
	lilac := rl.NewColor(180, 152, 200, 255)
	lilacPetal := rl.NewColor(198, 178, 214, 255)
	pistilGold := rl.NewColor(220, 208, 156, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Bloom 1 — yellow. Stems sway lightly, heads heavier so they nod
			// above a barely-bending stalk; ground leaves stay rigid.
			{modelIdx: 0, offset: rl.NewVector3(0.10, 0.12, 0.06), scale: rl.NewVector3(1, 1, 1), tint: stemTint, sway: 0.6},
			{modelIdx: 1, offset: rl.NewVector3(0.10, 0.23, 0.06), scale: rl.NewVector3(1, 1, 1), rotation: 12, tint: yellowPetal, sway: 1.0},
			{modelIdx: 2, offset: rl.NewVector3(0.10, 0.26, 0.06), scale: rl.NewVector3(1, 1, 1), tint: yellow, sway: 1.0},
			{modelIdx: 3, offset: rl.NewVector3(0.10, 0.295, 0.06), scale: rl.NewVector3(1, 1, 1), tint: pistilGold, sway: 1.0},
			// Bloom 2 — pink.
			{modelIdx: 0, offset: rl.NewVector3(-0.09, 0.12, 0.13), scale: rl.NewVector3(1, 1.05, 1), tint: stemTint, sway: 0.6},
			{modelIdx: 1, offset: rl.NewVector3(-0.09, 0.24, 0.13), scale: rl.NewVector3(1, 1, 1), rotation: -22, tint: pinkPetal, sway: 1.0},
			{modelIdx: 2, offset: rl.NewVector3(-0.09, 0.27, 0.13), scale: rl.NewVector3(1, 1, 1), tint: pink, sway: 1.0},
			{modelIdx: 3, offset: rl.NewVector3(-0.09, 0.305, 0.13), scale: rl.NewVector3(1, 1, 1), tint: pistilGold, sway: 1.0},
			// Bloom 3 — white.
			{modelIdx: 0, offset: rl.NewVector3(0.04, 0.11, -0.14), scale: rl.NewVector3(1, 0.95, 1), tint: stemTint, sway: 0.6},
			{modelIdx: 1, offset: rl.NewVector3(0.04, 0.22, -0.14), scale: rl.NewVector3(1, 1, 1), rotation: 35, tint: whitePetal, sway: 1.0},
			{modelIdx: 2, offset: rl.NewVector3(0.04, 0.25, -0.14), scale: rl.NewVector3(1, 1, 1), tint: white, sway: 1.0},
			{modelIdx: 3, offset: rl.NewVector3(0.04, 0.285, -0.14), scale: rl.NewVector3(1, 1, 1), tint: pistilGold, sway: 1.0},
			// Bloom 4 — lilac.
			{modelIdx: 0, offset: rl.NewVector3(-0.14, 0.12, -0.04), scale: rl.NewVector3(1, 1, 1), tint: stemTint, sway: 0.6},
			{modelIdx: 1, offset: rl.NewVector3(-0.14, 0.23, -0.04), scale: rl.NewVector3(1, 1, 1), rotation: 8, tint: lilacPetal, sway: 1.0},
			{modelIdx: 2, offset: rl.NewVector3(-0.14, 0.26, -0.04), scale: rl.NewVector3(1, 1, 1), tint: lilac, sway: 1.0},
			{modelIdx: 3, offset: rl.NewVector3(-0.14, 0.295, -0.04), scale: rl.NewVector3(1, 1, 1), tint: pistilGold, sway: 1.0},
			// Three ground leaves, rotated so the patch reads as a clump.
			{modelIdx: 4, offset: rl.NewVector3(0.02, 0.012, 0.01), scale: rl.NewVector3(1.4, 1, 1.4), rotation: 20, tint: leafTint},
			{modelIdx: 4, offset: rl.NewVector3(-0.06, 0.012, -0.08), scale: rl.NewVector3(1.2, 1, 1.6), rotation: -45, tint: leafTint},
			{modelIdx: 4, offset: rl.NewVector3(0.10, 0.012, -0.12), scale: rl.NewVector3(1.1, 1, 1.4), rotation: 60, tint: leafTint},
		},
	}
}

// loadCloverProp builds a ground-hugging clover patch: six flattened spheres.
func loadCloverProp(shader rl.Shader) propModel {
	leaf := rl.LoadModelFromMesh(rl.GenMeshSphere(0.10, 8, 10))
	models := []rl.Model{leaf}
	attachShader(&models[0], shader)
	leafA := rl.NewColor(124, 186, 102, 255)
	leafB := rl.NewColor(96, 156, 84, 255)
	leafC := rl.NewColor(148, 200, 116, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0.10, 0.04, 0.04), scale: rl.NewVector3(1, 0.45, 1), tint: leafA},
			{modelIdx: 0, offset: rl.NewVector3(-0.06, 0.04, 0.10), scale: rl.NewVector3(1, 0.45, 1), tint: leafB},
			{modelIdx: 0, offset: rl.NewVector3(0.02, 0.04, -0.10), scale: rl.NewVector3(1, 0.45, 1), tint: leafC},
			{modelIdx: 0, offset: rl.NewVector3(-0.12, 0.04, -0.04), scale: rl.NewVector3(1, 0.45, 1), tint: leafA},
			{modelIdx: 0, offset: rl.NewVector3(0.14, 0.04, -0.08), scale: rl.NewVector3(1, 0.45, 1), tint: leafB},
			{modelIdx: 0, offset: rl.NewVector3(-0.02, 0.04, -0.16), scale: rl.NewVector3(1, 0.45, 1), tint: leafC},
		},
	}
}

// loadReedProp builds a cluster of tall water reeds. Cooler olive tints than
// the warmer tall grass; for water tiles and damp edges.
func loadReedProp(shader rl.Shader) propModel {
	reed := rl.LoadModelFromMesh(rl.GenMeshCube(0.025, 0.62, 0.025))
	tip := rl.LoadModelFromMesh(rl.GenMeshCube(0.04, 0.07, 0.04))
	models := []rl.Model{reed, tip}
	shadeAll(models, shader)
	stem := rl.NewColor(110, 132, 90, 255)
	stemDark := rl.NewColor(82, 102, 70, 255)
	pod := rl.NewColor(132, 96, 56, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0.10, 0.31, 0.04), scale: rl.NewVector3(1, 1, 1), tint: stem},
			{modelIdx: 1, offset: rl.NewVector3(0.10, 0.65, 0.04), scale: rl.NewVector3(1, 1, 1), tint: pod},
			{modelIdx: 0, offset: rl.NewVector3(-0.06, 0.30, 0.12), scale: rl.NewVector3(1, 0.95, 1), tint: stemDark},
			{modelIdx: 1, offset: rl.NewVector3(-0.06, 0.62, 0.12), scale: rl.NewVector3(1, 1, 1), tint: pod},
			{modelIdx: 0, offset: rl.NewVector3(0.02, 0.31, -0.10), scale: rl.NewVector3(1, 1.05, 1), tint: stem},
			{modelIdx: 1, offset: rl.NewVector3(0.02, 0.66, -0.10), scale: rl.NewVector3(1, 1, 1), tint: pod},
			{modelIdx: 0, offset: rl.NewVector3(-0.12, 0.30, -0.04), scale: rl.NewVector3(1, 0.92, 1), tint: stemDark},
			{modelIdx: 0, offset: rl.NewVector3(0.12, 0.31, -0.08), scale: rl.NewVector3(1, 1, 1), tint: stem},
			{modelIdx: 0, offset: rl.NewVector3(-0.02, 0.31, 0.14), scale: rl.NewVector3(1, 1.02, 1), tint: stem},
		},
	}
}

// loadExoticFlowerProp builds a funky bloom on a tall stalk: two offset petal
// rings (8-point star from above), domed center, pistil, two ground leaves.
// Vivid out-of-palette colors. Non-blocking (core.PropIsNonBlocking).
func loadExoticFlowerProp(shader rl.Shader) propModel {
	stem := rl.LoadModelFromMesh(rl.GenMeshCube(0.04, 0.5, 0.04))
	petalOuter := rl.LoadModelFromMesh(rl.GenMeshCube(0.26, 0.02, 0.26))
	petalInner := rl.LoadModelFromMesh(rl.GenMeshCube(0.17, 0.022, 0.17))
	bloom := rl.LoadModelFromMesh(rl.GenMeshSphere(0.07, 10, 12))
	pistil := rl.LoadModelFromMesh(rl.GenMeshSphere(0.045, 8, 10))
	leaf := rl.LoadModelFromMesh(rl.GenMeshCube(0.07, 0.02, 0.12))
	models := []rl.Model{stem, petalOuter, petalInner, bloom, pistil, leaf}
	shadeAll(models, shader)
	stemTint := rl.NewColor(86, 132, 80, 255)
	leafTint := rl.NewColor(104, 150, 92, 255)
	magenta := rl.NewColor(206, 92, 168, 255)
	orange := rl.NewColor(232, 150, 78, 255)
	teal := rl.NewColor(96, 196, 188, 255)
	gold := rl.NewColor(232, 206, 120, 255)
	yAxis := worldUp
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.25, 0), scale: rl.NewVector3(1, 1, 1), tint: stemTint, sway: 0.5},
			// Two petal rings, second rotated 45° into a starburst. Head sways
			// heavier than the stem.
			{modelIdx: 1, offset: rl.NewVector3(0, 0.50, 0), scale: rl.NewVector3(1, 1, 1), rotation: 12, rotationAxis: yAxis, tint: magenta, sway: 1.0},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.515, 0), scale: rl.NewVector3(0.82, 1, 0.82), rotation: 45, rotationAxis: yAxis, tint: orange, sway: 1.0},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.53, 0), scale: rl.NewVector3(1, 1, 1), rotation: 22, rotationAxis: yAxis, tint: teal, sway: 1.0},
			{modelIdx: 3, offset: rl.NewVector3(0, 0.55, 0), scale: rl.NewVector3(1, 1, 1), tint: gold, sway: 1.0},
			{modelIdx: 4, offset: rl.NewVector3(0, 0.575, 0), scale: rl.NewVector3(1, 1, 1), tint: magenta, sway: 1.0},
			{modelIdx: 5, offset: rl.NewVector3(0.08, 0.012, 0.02), scale: rl.NewVector3(1.4, 1, 1.6), rotation: 25, rotationAxis: yAxis, tint: leafTint},
			{modelIdx: 5, offset: rl.NewVector3(-0.07, 0.012, -0.06), scale: rl.NewVector3(1.3, 1, 1.5), rotation: -50, rotationAxis: yAxis, tint: leafTint},
		},
	}
}

// loadTallFernProp builds a clump of arching fronds (flat boxes fanned at varied
// tilts). Cool layered greens, gentle sway. Non-blocking.
func loadTallFernProp(shader rl.Shader) propModel {
	frond := rl.LoadModelFromMesh(rl.GenMeshCube(0.05, 0.55, 0.16))
	models := []rl.Model{frond}
	attachShader(&models[0], shader)
	deep := rl.NewColor(58, 108, 58, 255)
	mid := rl.NewColor(82, 140, 76, 255)
	light := rl.NewColor(120, 170, 98, 255)
	zAxis := rl.NewVector3(0, 0, 1)
	xAxis := rl.NewVector3(1, 0, 0)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.28, 0), scale: rl.NewVector3(0.9, 1.08, 0.9), rotation: 6, rotationAxis: zAxis, tint: light, sway: 0.9},
			{modelIdx: 0, offset: rl.NewVector3(0.12, 0.26, 0.05), scale: rl.NewVector3(1, 0.92, 1), rotation: 26, rotationAxis: zAxis, tint: deep, sway: 0.9},
			{modelIdx: 0, offset: rl.NewVector3(-0.12, 0.25, 0.06), scale: rl.NewVector3(1, 0.9, 1), rotation: -28, rotationAxis: zAxis, tint: deep, sway: 0.9},
			{modelIdx: 0, offset: rl.NewVector3(0.05, 0.27, -0.12), scale: rl.NewVector3(1, 1.02, 1), rotation: 20, rotationAxis: xAxis, tint: light, sway: 0.9},
			{modelIdx: 0, offset: rl.NewVector3(-0.06, 0.26, 0.12), scale: rl.NewVector3(1, 0.95, 1), rotation: -22, rotationAxis: xAxis, tint: mid, sway: 0.9},
			{modelIdx: 0, offset: rl.NewVector3(0.02, 0.27, -0.02), scale: rl.NewVector3(1, 1, 1), rotation: 10, rotationAxis: zAxis, tint: mid, sway: 0.9},
		},
	}
}

// loadGrassTuftProp builds a tall grass tuft: a fan of blades in warm greens
// with gold tips, full sway. A placeable cousin of the tall-grass scatter.
// Non-blocking.
func loadGrassTuftProp(shader rl.Shader) propModel {
	blade := rl.LoadModelFromMesh(rl.GenMeshCube(0.045, 0.5, 0.045))
	models := []rl.Model{blade}
	attachShader(&models[0], shader)
	light := rl.NewColor(150, 186, 110, 255)
	mid := rl.NewColor(116, 162, 98, 255)
	deep := rl.NewColor(84, 128, 80, 255)
	gold := rl.NewColor(198, 196, 132, 255)
	zAxis := rl.NewVector3(0, 0, 1)
	xAxis := rl.NewVector3(1, 0, 0)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.25, 0), scale: rl.NewVector3(1, 1.05, 1), rotation: 5, rotationAxis: zAxis, tint: light, sway: 1.0},
			{modelIdx: 0, offset: rl.NewVector3(0.10, 0.24, 0.05), scale: rl.NewVector3(1, 0.95, 1), rotation: 18, rotationAxis: zAxis, tint: mid, sway: 1.0},
			{modelIdx: 0, offset: rl.NewVector3(-0.10, 0.24, 0.06), scale: rl.NewVector3(1, 0.92, 1), rotation: -20, rotationAxis: zAxis, tint: mid, sway: 1.0},
			{modelIdx: 0, offset: rl.NewVector3(0.06, 0.25, -0.10), scale: rl.NewVector3(1, 1.02, 1), rotation: 16, rotationAxis: xAxis, tint: deep, sway: 1.0},
			{modelIdx: 0, offset: rl.NewVector3(-0.07, 0.24, -0.08), scale: rl.NewVector3(1, 0.9, 1), rotation: -22, rotationAxis: xAxis, tint: deep, sway: 1.0},
			{modelIdx: 0, offset: rl.NewVector3(0.13, 0.23, -0.04), scale: rl.NewVector3(1, 0.88, 1), rotation: 26, rotationAxis: zAxis, tint: gold, sway: 1.0},
			{modelIdx: 0, offset: rl.NewVector3(-0.13, 0.24, 0.02), scale: rl.NewVector3(1, 0.9, 1), rotation: -26, rotationAxis: zAxis, tint: gold, sway: 1.0},
			{modelIdx: 0, offset: rl.NewVector3(0.0, 0.26, 0.12), scale: rl.NewVector3(1, 1.08, 1), rotation: 12, rotationAxis: xAxis, tint: light, sway: 1.0},
			{modelIdx: 0, offset: rl.NewVector3(0.02, 0.25, -0.02), scale: rl.NewVector3(0.9, 1, 0.9), rotation: -8, rotationAxis: zAxis, tint: mid, sway: 1.0},
		},
	}
}

// loadBoneProp builds a bone scatter: a skull sphere and long-bone cylinders
// lying across each other. Weathered white-yellow tints.
func loadBoneProp(shader rl.Shader) propModel {
	skull := rl.LoadModelFromMesh(rl.GenMeshSphere(0.09, 8, 10))
	jaw := rl.LoadModelFromMesh(rl.GenMeshCube(0.10, 0.05, 0.07))
	longBone := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.025, 0.32, 8))
	knuckle := rl.LoadModelFromMesh(rl.GenMeshSphere(0.045, 6, 8))
	models := []rl.Model{skull, jaw, longBone, knuckle}
	shadeAll(models, shader)
	bone := rl.NewColor(228, 220, 198, 255)
	stain := rl.NewColor(178, 162, 132, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Skull and detached jaw.
			{modelIdx: 0, offset: rl.NewVector3(-0.08, 0.08, -0.08), scale: rl.NewVector3(1, 0.85, 1), tint: bone},
			{modelIdx: 1, offset: rl.NewVector3(-0.04, 0.04, -0.04), scale: rl.NewVector3(1, 1, 1), tint: stain},
			// Three long bones flat at varied angles (non-vertical axis tips the
			// cylinder onto its side).
			{modelIdx: 2, offset: rl.NewVector3(0.10, 0.025, 0.06), scale: rl.NewVector3(1, 1, 1), rotation: 90, rotationAxis: rl.NewVector3(1, 0, 0), tint: bone},
			{modelIdx: 3, offset: rl.NewVector3(0.10, 0.045, 0.22), scale: rl.NewVector3(1, 1, 1), tint: bone},
			{modelIdx: 2, offset: rl.NewVector3(0.02, 0.025, -0.10), scale: rl.NewVector3(1, 1, 1), rotation: 70, rotationAxis: rl.NewVector3(0, 0, 1), tint: stain},
			{modelIdx: 2, offset: rl.NewVector3(-0.12, 0.025, 0.12), scale: rl.NewVector3(1, 1, 1), rotation: 110, rotationAxis: rl.NewVector3(1, 0, 1), tint: bone},
		},
	}
}

// loadScorchProp builds a flat floor burn mark: a thin cylinder with a faded
// inner ring.
func loadScorchProp(shader rl.Shader) propModel {
	outer := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.42, 0.02, 20))
	inner := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.24, 0.02, 18))
	models := []rl.Model{outer, inner}
	shadeAll(models, shader)
	ash := rl.NewColor(44, 38, 36, 255)
	char := rl.NewColor(22, 18, 16, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.005, 0), scale: rl.NewVector3(1, 1, 1), tint: ash},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.012, 0), scale: rl.NewVector3(1, 1, 1), tint: char},
		},
	}
}

// loadBloodProp builds a dried-bloodstain decal: three flat low cylinders in
// tarnished red, tint-walked so they read as a smear.
func loadBloodProp(shader rl.Shader) propModel {
	disc := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.18, 0.015, 18))
	models := []rl.Model{disc}
	attachShader(&models[0], shader)
	a := rl.NewColor(124, 38, 36, 255)
	b := rl.NewColor(96, 28, 30, 255)
	c := rl.NewColor(78, 22, 24, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.008, 0), scale: rl.NewVector3(1.5, 1, 1.5), tint: a},
			{modelIdx: 0, offset: rl.NewVector3(0.16, 0.012, 0.08), scale: rl.NewVector3(0.9, 1, 0.9), tint: b},
			{modelIdx: 0, offset: rl.NewVector3(-0.10, 0.011, -0.12), scale: rl.NewVector3(0.7, 1, 0.7), tint: c},
		},
	}
}

// loadCobwebProp builds a corner cobweb: a slanted slab plus two strands,
// off-center and low-contrast.
func loadCobwebProp(shader rl.Shader) propModel {
	panel := rl.LoadModelFromMesh(rl.GenMeshCube(0.42, 0.012, 0.42))
	strand := rl.LoadModelFromMesh(rl.GenMeshCube(0.34, 0.008, 0.020))
	models := []rl.Model{panel, strand}
	shadeAll(models, shader)
	web := rl.NewColor(220, 222, 226, 200)
	wisp := rl.NewColor(196, 200, 208, 220)
	return propModel{
		models: models,
		parts: []treePart{
			// Main slanted disc — tilted so it reads as a web at an angle.
			{modelIdx: 0, offset: rl.NewVector3(-0.28, 0.16, -0.28), scale: rl.NewVector3(1, 1, 1), rotation: 35, rotationAxis: rl.NewVector3(1, 0, 1), tint: web},
			// Two thinner strands.
			{modelIdx: 1, offset: rl.NewVector3(-0.10, 0.12, -0.18), scale: rl.NewVector3(1, 1, 1), rotation: 30, rotationAxis: worldUp, tint: wisp},
			{modelIdx: 1, offset: rl.NewVector3(-0.20, 0.08, -0.30), scale: rl.NewVector3(1, 1, 1), rotation: -20, rotationAxis: worldUp, tint: wisp},
		},
	}
}

// loadStumpProp builds a tree stump: a fat cylinder trunk with a cut-ring disc
// on top. Bark texture shares the wood family with the trees.
func loadStumpProp(shader rl.Shader, barkTex rl.Texture2D) propModel {
	body := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.34, 0.34, 14))
	face := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.32, 0.04, 14))
	models := []rl.Model{body, face}
	setModelTexture(&models[0], barkTex)
	shadeAll(models, shader)
	// Pastel pecan stump with pale-cream cut-face rings.
	bark := rl.NewColor(172, 132, 96, 255)
	rings := rl.NewColor(214, 184, 144, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.01, 0), scale: rl.NewVector3(1, 1, 1), tint: bark},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.34, 0), scale: rl.NewVector3(1, 1, 1), tint: rings},
		},
	}
}

// loadLogProp builds a fallen log on its side: cylinder tipped 90° around X so
// its long axis runs along world Z, with end-cap discs and moss patches.
func loadLogProp(shader rl.Shader, barkTex, leafTex rl.Texture2D) propModel {
	trunk := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.20, 1.05, 14))
	cap := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.20, 0.02, 14))
	moss := rl.LoadModelFromMesh(rl.GenMeshSphere(0.16, 8, 10))
	models := []rl.Model{trunk, cap, moss}
	setModelTexture(&models[0], barkTex)
	setModelTexture(&models[2], leafTex)
	shadeAll(models, shader)
	// Pastel pecan log with pale cut faces + soft mint moss.
	bark := rl.NewColor(170, 130, 94, 255)
	cut := rl.NewColor(210, 178, 138, 255)
	mossTint := rl.NewColor(156, 192, 140, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Trunk on its side: 90° around +X tips the +Y cylinder onto -Z, so
			// its length runs along z, center ~half its radius above ground.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.20, 0), scale: rl.NewVector3(1, 1, 1), rotation: 90, rotationAxis: rl.NewVector3(1, 0, 0), tint: bark},
			// End caps at z = ±half-length.
			{modelIdx: 1, offset: rl.NewVector3(0, 0.20, 0.52), scale: rl.NewVector3(1, 1, 1), rotation: 90, rotationAxis: rl.NewVector3(1, 0, 0), tint: cut},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.20, -0.52), scale: rl.NewVector3(1, 1, 1), rotation: 90, rotationAxis: rl.NewVector3(1, 0, 0), tint: cut},
			// Moss patches squashed on top of the trunk.
			{modelIdx: 2, offset: rl.NewVector3(0.05, 0.34, 0.18), scale: rl.NewVector3(0.85, 0.40, 0.85), tint: mossTint},
			{modelIdx: 2, offset: rl.NewVector3(-0.04, 0.34, -0.22), scale: rl.NewVector3(0.70, 0.35, 0.70), tint: mossTint},
		},
	}
}

// loadDoorProp builds an area-transition door frame (posts, lintel, panel).
// The tile stays walkable; stepping on it triggers the transition. Faces world
// Z by default; the renderer rotates it by the door's authored facing.
func loadDoorProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	post := rl.LoadModelFromMesh(rl.GenMeshCube(0.10, 1.40, 0.10))
	lintel := rl.LoadModelFromMesh(rl.GenMeshCube(0.84, 0.14, 0.12))
	panel := rl.LoadModelFromMesh(rl.GenMeshCube(0.60, 1.20, 0.04))
	plank := rl.LoadModelFromMesh(rl.GenMeshCube(0.56, 0.04, 0.05))
	keystone := rl.LoadModelFromMesh(rl.GenMeshCube(0.20, 0.20, 0.16))
	stud := rl.LoadModelFromMesh(rl.GenMeshSphere(0.028, 8, 10))
	hinge := rl.LoadModelFromMesh(rl.GenMeshCube(0.10, 0.06, 0.07))
	knob := rl.LoadModelFromMesh(rl.GenMeshSphere(0.045, 10, 12))
	threshold := rl.LoadModelFromMesh(rl.GenMeshCube(0.80, 0.06, 0.18))
	models := []rl.Model{post, lintel, panel, plank, keystone, stud, hinge, knob, threshold}
	// Wood-textured parts: posts, lintel, panel, plank ridges, keystone, threshold.
	for _, i := range []int{0, 1, 2, 3, 4, 8} {
		setModelTexture(&models[i], woodTex)
	}
	shadeAll(models, shader)
	// Muted wood family matching the bark/crate/stump palette.
	wood := rl.NewColor(118, 84, 56, 255)
	woodDark := rl.NewColor(84, 60, 42, 255)
	doorPanel := rl.NewColor(98, 70, 50, 255)
	plankDark := rl.NewColor(70, 48, 32, 255)
	brass := rl.NewColor(178, 142, 78, 255)
	brassBright := rl.NewColor(206, 168, 96, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Two posts flanking the doorway.
			{modelIdx: 0, offset: rl.NewVector3(-0.35, 0.70, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			{modelIdx: 0, offset: rl.NewVector3(0.35, 0.70, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			// Lintel across the top.
			{modelIdx: 1, offset: rl.NewVector3(0, 1.44, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			// Threshold step at the base.
			{modelIdx: 8, offset: rl.NewVector3(0, 0.03, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			// Recessed plank-faced door panel.
			{modelIdx: 2, offset: rl.NewVector3(0, 0.65, 0), scale: rl.NewVector3(1, 1, 1), tint: doorPanel},
			// Horizontal plank ridges so it reads as boarded, not a slab.
			{modelIdx: 3, offset: rl.NewVector3(0, 0.32, 0.025), scale: rl.NewVector3(1, 1, 1), tint: plankDark},
			{modelIdx: 3, offset: rl.NewVector3(0, 0.66, 0.025), scale: rl.NewVector3(1, 1, 1), tint: plankDark},
			{modelIdx: 3, offset: rl.NewVector3(0, 1.00, 0.025), scale: rl.NewVector3(1, 1, 1), tint: plankDark},
			// Four brass corner studs.
			{modelIdx: 5, offset: rl.NewVector3(-0.24, 0.16, 0.045), scale: rl.NewVector3(1, 1, 1), tint: brassBright},
			{modelIdx: 5, offset: rl.NewVector3(0.24, 0.16, 0.045), scale: rl.NewVector3(1, 1, 1), tint: brassBright},
			{modelIdx: 5, offset: rl.NewVector3(-0.24, 1.14, 0.045), scale: rl.NewVector3(1, 1, 1), tint: brassBright},
			{modelIdx: 5, offset: rl.NewVector3(0.24, 1.14, 0.045), scale: rl.NewVector3(1, 1, 1), tint: brassBright},
			// Two iron hinges on the panel's left edge.
			{modelIdx: 6, offset: rl.NewVector3(-0.30, 0.32, 0.045), scale: rl.NewVector3(1, 1, 1), tint: brass},
			{modelIdx: 6, offset: rl.NewVector3(-0.30, 0.98, 0.045), scale: rl.NewVector3(1, 1, 1), tint: brass},
			// Brass doorknob — the interactive cue.
			{modelIdx: 7, offset: rl.NewVector3(0.20, 0.62, 0.07), scale: rl.NewVector3(1, 1, 1), tint: brassBright},
			// Keystone crown, forward of the lintel for relief.
			{modelIdx: 4, offset: rl.NewVector3(0, 1.52, 0.02), scale: rl.NewVector3(1, 1, 1), tint: brass},
		},
	}
}

// loadCaveDoorProp builds a rough stone archway: rock jambs, lintel slab,
// boulders at the feet, dark recessed opening. Same footprint/facing as loadDoorProp.
func loadCaveDoorProp(shader rl.Shader, stoneTex rl.Texture2D) propModel {
	jamb := rl.LoadModelFromMesh(rl.GenMeshCube(0.22, 1.45, 0.22))
	lintel := rl.LoadModelFromMesh(rl.GenMeshCube(0.96, 0.26, 0.26))
	opening := rl.LoadModelFromMesh(rl.GenMeshCube(0.56, 1.20, 0.06))
	boulder := rl.LoadModelFromMesh(rl.GenMeshSphere(0.18, 8, 8))
	threshold := rl.LoadModelFromMesh(rl.GenMeshCube(0.86, 0.08, 0.24))
	models := []rl.Model{jamb, lintel, opening, boulder, threshold}
	// Stone-textured: jambs, lintel, boulders, threshold (not the dark opening).
	for _, i := range []int{0, 1, 3, 4} {
		setModelTexture(&models[i], stoneTex)
	}
	shadeAll(models, shader)
	stone := rl.NewColor(150, 146, 138, 255)
	stoneDark := rl.NewColor(112, 108, 102, 255)
	stoneShade := rl.NewColor(96, 92, 88, 255)
	mouth := rl.NewColor(28, 26, 30, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Two rough jambs.
			{modelIdx: 0, offset: rl.NewVector3(-0.37, 0.72, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			{modelIdx: 0, offset: rl.NewVector3(0.37, 0.72, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			// Heavy lintel slab bridging the jambs.
			{modelIdx: 1, offset: rl.NewVector3(0, 1.46, 0), scale: rl.NewVector3(1, 1, 1), tint: stoneDark},
			// Worn stone threshold underfoot.
			{modelIdx: 4, offset: rl.NewVector3(0, 0.04, 0), scale: rl.NewVector3(1, 1, 1), tint: stoneShade},
			// Dark recessed opening between the jambs.
			{modelIdx: 2, offset: rl.NewVector3(0, 0.64, 0), scale: rl.NewVector3(1, 1, 1), tint: mouth},
			// Boulders at the jamb feet — breaks the clean rectangle.
			{modelIdx: 3, offset: rl.NewVector3(-0.42, 0.16, 0.10), scale: rl.NewVector3(1, 0.9, 1), tint: stoneDark},
			{modelIdx: 3, offset: rl.NewVector3(-0.30, 0.12, -0.12), scale: rl.NewVector3(0.7, 0.7, 0.7), tint: stoneShade},
			{modelIdx: 3, offset: rl.NewVector3(0.44, 0.18, -0.08), scale: rl.NewVector3(1.1, 0.95, 1.1), tint: stoneDark},
			{modelIdx: 3, offset: rl.NewVector3(0.32, 0.12, 0.12), scale: rl.NewVector3(0.65, 0.65, 0.65), tint: stoneShade},
			// A capstone boulder perched on the lintel.
			{modelIdx: 3, offset: rl.NewVector3(0.08, 1.66, 0), scale: rl.NewVector3(1.2, 0.8, 1.0), tint: stone},
		},
	}
}

// loadFieldDoorProp builds an open trail gateway: leaning posts, a crossbeam,
// no solid panel, a hanging sign, grass tufts. Same footprint/facing as loadDoorProp.
func loadFieldDoorProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	post := rl.LoadModelFromMesh(rl.GenMeshCube(0.09, 1.30, 0.09))
	beam := rl.LoadModelFromMesh(rl.GenMeshCube(0.92, 0.10, 0.10))
	brace := rl.LoadModelFromMesh(rl.GenMeshCube(0.40, 0.06, 0.06))
	sign := rl.LoadModelFromMesh(rl.GenMeshCube(0.34, 0.20, 0.04))
	cap := rl.LoadModelFromMesh(rl.GenMeshSphere(0.06, 8, 8))
	tuft := rl.LoadModelFromMesh(rl.GenMeshCube(0.10, 0.18, 0.10))
	models := []rl.Model{post, beam, brace, sign, cap, tuft}
	// Wood-textured: posts, beam, brace, sign.
	for _, i := range []int{0, 1, 2, 3} {
		setModelTexture(&models[i], woodTex)
	}
	shadeAll(models, shader)
	wood := rl.NewColor(150, 112, 72, 255)
	woodDark := rl.NewColor(112, 82, 52, 255)
	signWood := rl.NewColor(168, 132, 90, 255)
	capTint := rl.NewColor(96, 70, 46, 255)
	grass := rl.NewColor(120, 168, 92, 255)
	grassDark := rl.NewColor(96, 142, 78, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Two leaning posts.
			{modelIdx: 0, offset: rl.NewVector3(-0.40, 0.65, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			{modelIdx: 0, offset: rl.NewVector3(0.40, 0.65, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			// Crossbeam — the open lintel.
			{modelIdx: 1, offset: rl.NewVector3(0, 1.28, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			// Braces under each beam end.
			{modelIdx: 2, offset: rl.NewVector3(-0.26, 1.16, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			{modelIdx: 2, offset: rl.NewVector3(0.26, 1.16, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			// Post caps.
			{modelIdx: 4, offset: rl.NewVector3(-0.40, 1.32, 0), scale: rl.NewVector3(1, 1, 1), tint: capTint},
			{modelIdx: 4, offset: rl.NewVector3(0.40, 1.32, 0), scale: rl.NewVector3(1, 1, 1), tint: capTint},
			// Hanging trail sign.
			{modelIdx: 3, offset: rl.NewVector3(0, 1.04, 0.02), scale: rl.NewVector3(1, 1, 1), tint: signWood},
			// Grass tufts at the post feet.
			{modelIdx: 5, offset: rl.NewVector3(-0.44, 0.09, 0.06), scale: rl.NewVector3(1, 1, 1), tint: grass},
			{modelIdx: 5, offset: rl.NewVector3(0.46, 0.09, -0.05), scale: rl.NewVector3(0.8, 0.8, 0.8), tint: grassDark},
		},
	}
}

// loadLilypadProp builds a floating lilypad: a wide disc, a smaller partner
// disc, a pink bloom. Sits just above floor level to avoid z-fighting.
func loadLilypadProp(shader rl.Shader) propModel {
	pad := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.42, 0.015, 16))
	smallPad := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.22, 0.015, 14))
	bloom := rl.LoadModelFromMesh(rl.GenMeshSphere(0.06, 8, 8))
	bud := rl.LoadModelFromMesh(rl.GenMeshSphere(0.035, 6, 8))
	models := []rl.Model{pad, smallPad, bloom, bud}
	shadeAll(models, shader)
	leaf := rl.NewColor(72, 138, 78, 255)
	leafDark := rl.NewColor(54, 108, 60, 255)
	flower := rl.NewColor(244, 180, 210, 255)
	flowerCore := rl.NewColor(252, 240, 168, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0.04, 0.005, 0.02), scale: rl.NewVector3(1, 1, 1), tint: leaf},
			{modelIdx: 1, offset: rl.NewVector3(-0.22, 0.008, -0.18), scale: rl.NewVector3(1, 1, 1), tint: leafDark},
			{modelIdx: 2, offset: rl.NewVector3(0.04, 0.032, 0.02), scale: rl.NewVector3(1, 1, 1), tint: flower},
			{modelIdx: 3, offset: rl.NewVector3(0.04, 0.052, 0.02), scale: rl.NewVector3(1, 1, 1), tint: flowerCore},
		},
	}
}

// loadLeafPileProp builds a leaf pile: a flat cylinder heap with two domes on
// top. Shares the tree's leaf texture.
func loadLeafPileProp(shader rl.Shader, leafTex rl.Texture2D) propModel {
	base := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.48, 0.10, 16))
	mound := rl.LoadModelFromMesh(rl.GenMeshSphere(0.22, 10, 12))
	models := []rl.Model{base, mound}
	textureAndShade(models, shader, leafTex)
	leafA := rl.NewColor(196, 142, 80, 255)
	leafB := rl.NewColor(168, 110, 64, 255)
	leafC := rl.NewColor(220, 178, 96, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.02, 0), scale: rl.NewVector3(1, 1, 1), tint: leafA},
			{modelIdx: 1, offset: rl.NewVector3(0.08, 0.16, 0.06), scale: rl.NewVector3(1, 0.55, 1), tint: leafB},
			{modelIdx: 1, offset: rl.NewVector3(-0.10, 0.14, -0.08), scale: rl.NewVector3(1, 0.45, 1), tint: leafC},
		},
	}
}

// --- Outdoor / field tileset additions (Turn B). Single-tile blockers. ---

// loadWellProp builds a stone-ringed well: rim cylinder, water disc, pole/winch.
// Rock texture on the ring.
func loadWellProp(shader rl.Shader, rockTex rl.Texture2D) propModel {
	rim := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.42, 0.40, 18))
	water := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.34, 0.04, 16))
	post := rl.LoadModelFromMesh(rl.GenMeshCube(0.06, 0.80, 0.06))
	beam := rl.LoadModelFromMesh(rl.GenMeshCube(0.78, 0.06, 0.06))
	bucket := rl.LoadModelFromMesh(rl.GenMeshCube(0.16, 0.16, 0.16))
	models := []rl.Model{rim, water, post, beam, bucket}
	setModelTexture(&models[0], rockTex)
	shadeAll(models, shader)
	stone := rl.NewColor(170, 168, 156, 255)
	stoneDark := rl.NewColor(110, 108, 100, 255)
	waterCol := rl.NewColor(56, 96, 138, 255)
	wood := woodPaletteWarm
	woodDark := rl.NewColor(74, 52, 34, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.20, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.36, 0), scale: rl.NewVector3(1, 1, 1), tint: waterCol},
			// Posts holding the winch beam.
			{modelIdx: 2, offset: rl.NewVector3(-0.30, 0.80, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			{modelIdx: 2, offset: rl.NewVector3(0.30, 0.80, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			{modelIdx: 3, offset: rl.NewVector3(0, 1.20, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			// Bucket (no rope geom).
			{modelIdx: 4, offset: rl.NewVector3(0.18, 0.95, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			// Dark base ring near the floor.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.04, 0), scale: rl.NewVector3(1.02, 0.18, 1.02), tint: stoneDark},
		},
	}
}

// loadGravestoneProp builds a tombstone: a slab tilted forward with a rounded
// top, plus a base mound. Cool grey palette.
func loadGravestoneProp(shader rl.Shader, rockTex rl.Texture2D) propModel {
	slab := rl.LoadModelFromMesh(rl.GenMeshCube(0.50, 0.85, 0.12))
	cap := rl.LoadModelFromMesh(rl.GenMeshSphere(0.26, 8, 10))
	mound := rl.LoadModelFromMesh(rl.GenMeshSphere(0.36, 8, 10))
	models := []rl.Model{slab, cap, mound}
	textureAndShade(models, shader, rockTex)
	stone := rl.NewColor(168, 162, 152, 255)
	stoneDark := rl.NewColor(112, 108, 100, 255)
	earth := rl.NewColor(86, 64, 48, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 2, offset: rl.NewVector3(0, 0.08, 0.18), scale: rl.NewVector3(1, 0.35, 1), tint: earth},
			{modelIdx: 0, offset: rl.NewVector3(0, 0.50, -0.10), scale: rl.NewVector3(1, 1, 1), rotation: 8, rotationAxis: rl.NewVector3(1, 0, 0), tint: stone},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.90, -0.10), scale: rl.NewVector3(1, 0.55, 1), tint: stoneDark},
		},
	}
}

// loadSignPostProp builds a wooden sign: a post with an off-axis plank near the top.
func loadSignPostProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	post := rl.LoadModelFromMesh(rl.GenMeshCube(0.08, 1.10, 0.08))
	board := rl.LoadModelFromMesh(rl.GenMeshCube(0.66, 0.34, 0.06))
	models := []rl.Model{post, board}
	textureAndShade(models, shader, woodTex)
	wood := rl.NewColor(150, 102, 60, 255)
	woodDark := rl.NewColor(96, 64, 40, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.55, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			{modelIdx: 1, offset: rl.NewVector3(0.18, 0.95, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			// Lighter front-face so the board reads as carved.
			{modelIdx: 1, offset: rl.NewVector3(0.18, 0.95, 0.04), scale: rl.NewVector3(0.92, 0.85, 0.4), tint: wood},
		},
	}
}

// loadHayBaleProp builds a fat straw cylinder on its side with binding bands.
func loadHayBaleProp(shader rl.Shader) propModel {
	bale := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.45, 0.70, 14))
	band := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.47, 0.04, 14))
	models := []rl.Model{bale, band}
	shadeAll(models, shader)
	straw := rl.NewColor(216, 184, 110, 255)
	strawDark := rl.NewColor(168, 132, 76, 255)
	cord := rl.NewColor(118, 86, 52, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Cylinder on its side (90° around X, length along world Z).
			{modelIdx: 0, offset: rl.NewVector3(0, 0.45, 0), scale: rl.NewVector3(1, 1, 1), rotation: 90, rotationAxis: rl.NewVector3(1, 0, 0), tint: straw},
			// Darker shading underneath.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.45, 0), scale: rl.NewVector3(0.92, 0.98, 0.92), rotation: 90, rotationAxis: rl.NewVector3(1, 0, 0), tint: strawDark},
			// Two binding rings.
			{modelIdx: 1, offset: rl.NewVector3(0, 0.45, -0.22), scale: rl.NewVector3(1, 1, 1), rotation: 90, rotationAxis: rl.NewVector3(1, 0, 0), tint: cord},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.45, 0.22), scale: rl.NewVector3(1, 1, 1), rotation: 90, rotationAxis: rl.NewVector3(1, 0, 0), tint: cord},
		},
	}
}

// loadScarecrowProp builds a cross-frame scarecrow: pole, arm beam, sackcloth
// head, torso blob.
func loadScarecrowProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	pole := rl.LoadModelFromMesh(rl.GenMeshCube(0.08, 1.55, 0.08))
	arm := rl.LoadModelFromMesh(rl.GenMeshCube(0.90, 0.07, 0.07))
	head := rl.LoadModelFromMesh(rl.GenMeshSphere(0.16, 8, 10))
	torso := rl.LoadModelFromMesh(rl.GenMeshCube(0.40, 0.50, 0.20))
	hat := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.20, 0.16, 12))
	models := []rl.Model{pole, arm, head, torso, hat}
	shadeAll(models, shader)
	setModelTexture(&models[0], woodTex)
	setModelTexture(&models[1], woodTex)
	wood := woodPaletteWarm
	sack := rl.NewColor(196, 162, 96, 255)
	sackDark := rl.NewColor(140, 110, 64, 255)
	hatCol := woodPaletteDark
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.78, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			{modelIdx: 1, offset: rl.NewVector3(0, 1.15, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			{modelIdx: 3, offset: rl.NewVector3(0, 0.85, 0), scale: rl.NewVector3(1, 1, 1), tint: sack},
			{modelIdx: 3, offset: rl.NewVector3(0, 0.85, 0), scale: rl.NewVector3(0.96, 0.7, 0.96), tint: sackDark},
			{modelIdx: 2, offset: rl.NewVector3(0, 1.32, 0), scale: rl.NewVector3(1, 1, 1), tint: sack},
			{modelIdx: 4, offset: rl.NewVector3(0, 1.50, 0), scale: rl.NewVector3(1, 1, 1), tint: hatCol},
		},
	}
}

// --- Dungeon-interior tileset additions (Turn B). ---

// loadBookshelfProp builds a tall shelf with three multicolored book-row bands.
func loadBookshelfProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	frame := rl.LoadModelFromMesh(rl.GenMeshCube(0.78, 1.50, 0.30))
	shelf := rl.LoadModelFromMesh(rl.GenMeshCube(0.82, 0.04, 0.34))
	books := rl.LoadModelFromMesh(rl.GenMeshCube(0.66, 0.32, 0.20))
	models := []rl.Model{frame, shelf, books}
	textureAndShade(models, shader, woodTex)
	wood := rl.NewColor(112, 78, 48, 255)
	woodDark := woodPaletteDark
	bookRed := rl.NewColor(160, 64, 60, 255)
	bookBlue := rl.NewColor(64, 96, 156, 255)
	bookGreen := rl.NewColor(96, 132, 80, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.75, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			// Three shelves with book rows.
			{modelIdx: 1, offset: rl.NewVector3(0, 0.30, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.48, 0.04), scale: rl.NewVector3(1, 1, 1), tint: bookRed},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.74, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.92, 0.04), scale: rl.NewVector3(1, 1, 1), tint: bookBlue},
			{modelIdx: 1, offset: rl.NewVector3(0, 1.18, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			{modelIdx: 2, offset: rl.NewVector3(0, 1.36, 0.04), scale: rl.NewVector3(1, 1, 1), tint: bookGreen},
		},
	}
}

// loadTableProp builds a wooden table: a flat top on four legs.
func loadTableProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	top := rl.LoadModelFromMesh(rl.GenMeshCube(0.90, 0.10, 0.60))
	leg := rl.LoadModelFromMesh(rl.GenMeshCube(0.10, 0.60, 0.10))
	models := []rl.Model{top, leg}
	textureAndShade(models, shader, woodTex)
	wood := rl.NewColor(160, 116, 72, 255)
	woodDark := woodPaletteWarm
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.65, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			{modelIdx: 1, offset: rl.NewVector3(-0.35, 0.30, -0.22), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			{modelIdx: 1, offset: rl.NewVector3(0.35, 0.30, -0.22), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			{modelIdx: 1, offset: rl.NewVector3(-0.35, 0.30, 0.22), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			{modelIdx: 1, offset: rl.NewVector3(0.35, 0.30, 0.22), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
		},
	}
}

// loadBedProp builds a wood-frame bed with pillow and bedding.
func loadBedProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	frame := rl.LoadModelFromMesh(rl.GenMeshCube(0.84, 0.20, 0.50))
	mattress := rl.LoadModelFromMesh(rl.GenMeshCube(0.78, 0.14, 0.46))
	headboard := rl.LoadModelFromMesh(rl.GenMeshCube(0.84, 0.42, 0.06))
	pillow := rl.LoadModelFromMesh(rl.GenMeshCube(0.30, 0.08, 0.36))
	models := []rl.Model{frame, mattress, headboard, pillow}
	setModelTexture(&models[0], woodTex)
	setModelTexture(&models[2], woodTex)
	shadeAll(models, shader)
	wood := rl.NewColor(112, 78, 50, 255)
	bedding := rl.NewColor(176, 90, 96, 255)
	beddingDark := rl.NewColor(132, 64, 70, 255)
	pillowCol := rl.NewColor(232, 218, 196, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.10, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.28, 0), scale: rl.NewVector3(1, 1, 1), tint: bedding},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.32, 0), scale: rl.NewVector3(0.94, 0.4, 0.94), tint: beddingDark},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.42, -0.28), scale: rl.NewVector3(1, 1, 1), tint: wood},
			{modelIdx: 3, offset: rl.NewVector3(0, 0.38, -0.16), scale: rl.NewVector3(1, 1, 1), tint: pillowCol},
		},
	}
}

// loadBrazierProp builds a metal brazier on a tripod: three legs, a bowl, flame.
func loadBrazierProp(shader rl.Shader) propModel {
	leg := rl.LoadModelFromMesh(rl.GenMeshCube(0.06, 0.60, 0.06))
	bowl := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.28, 0.18, 14))
	flame := rl.LoadModelFromMesh(rl.GenMeshSphere(0.18, 8, 10))
	tip := rl.LoadModelFromMesh(rl.GenMeshSphere(0.10, 6, 8))
	models := []rl.Model{leg, bowl, flame, tip}
	// Stand (legs + bowl) lit by the world shader; flame + tip stay on the
	// default shader so they render emissive (unaffected by dungeon ambient).
	attachShader(&models[0], shader)
	attachShader(&models[1], shader)
	// Shared torch/flame palette (world.go) so iron + fire track the wall torch.
	iron := torchIron
	ironLight := torchIronLight
	fire := torchFlameTints[1]
	fireBright := torchFlameTints[0]
	return propModel{
		models: models,
		parts: []treePart{
			// Three legs.
			{modelIdx: 0, offset: rl.NewVector3(-0.18, 0.30, -0.10), scale: rl.NewVector3(1, 1, 1), rotation: 15, rotationAxis: rl.NewVector3(0, 0, 1), tint: iron},
			{modelIdx: 0, offset: rl.NewVector3(0.18, 0.30, -0.10), scale: rl.NewVector3(1, 1, 1), rotation: -15, rotationAxis: rl.NewVector3(0, 0, 1), tint: iron},
			{modelIdx: 0, offset: rl.NewVector3(0, 0.30, 0.20), scale: rl.NewVector3(1, 1, 1), rotation: 15, rotationAxis: rl.NewVector3(1, 0, 0), tint: iron},
			// Bowl rim.
			{modelIdx: 1, offset: rl.NewVector3(0, 0.66, 0), scale: rl.NewVector3(1, 1, 1), tint: iron},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.72, 0), scale: rl.NewVector3(0.94, 0.2, 0.94), tint: ironLight},
			// Flame stack — broad base, bright tip.
			{modelIdx: 2, offset: rl.NewVector3(0, 0.90, 0), scale: rl.NewVector3(1, 1.4, 1), tint: fire},
			{modelIdx: 3, offset: rl.NewVector3(0, 1.10, 0), scale: rl.NewVector3(1, 1.6, 1), tint: fireBright},
		},
	}
}

// loadSarcophagusProp builds a stone sarcophagus: base with a lid flush on top.
func loadSarcophagusProp(shader rl.Shader, rockTex rl.Texture2D) propModel {
	base := rl.LoadModelFromMesh(rl.GenMeshCube(0.92, 0.46, 0.50))
	lid := rl.LoadModelFromMesh(rl.GenMeshCube(0.96, 0.10, 0.54))
	carving := rl.LoadModelFromMesh(rl.GenMeshCube(0.30, 0.36, 0.04))
	models := []rl.Model{base, lid, carving}
	textureAndShade(models, shader, rockTex)
	stone := rl.NewColor(200, 192, 174, 255)
	stoneDark := rl.NewColor(140, 132, 116, 255)
	carved := rl.NewColor(108, 96, 84, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.23, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			{modelIdx: 0, offset: rl.NewVector3(0, 0.23, 0), scale: rl.NewVector3(0.96, 1.04, 0.96), tint: stoneDark},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.51, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			// Faux carving on the lid.
			{modelIdx: 2, offset: rl.NewVector3(0, 0.40, 0.28), scale: rl.NewVector3(1, 1, 1), tint: carved},
		},
	}
}

// --- Decor additions (Turn B). Single-tile, non-blocking, in decorModels. ---

// loadRugProp builds a flat woven rug: a thin wide cube with a tasseled border.
func loadRugProp(shader rl.Shader) propModel {
	pad := rl.LoadModelFromMesh(rl.GenMeshCube(0.78, 0.02, 0.58))
	border := rl.LoadModelFromMesh(rl.GenMeshCube(0.84, 0.025, 0.64))
	models := []rl.Model{pad, border}
	shadeAll(models, shader)
	rug := rl.NewColor(176, 84, 68, 255)
	rugDark := rl.NewColor(120, 56, 48, 255)
	trim := rl.NewColor(232, 196, 124, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 1, offset: rl.NewVector3(0, 0.012, 0), scale: rl.NewVector3(1, 1, 1), tint: trim},
			{modelIdx: 0, offset: rl.NewVector3(0, 0.020, 0), scale: rl.NewVector3(1, 1, 1), tint: rug},
			// Inner darker stripe.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.024, 0), scale: rl.NewVector3(0.6, 0.5, 0.6), tint: rugDark},
		},
	}
}

// loadCandleProp builds a stubby candle with a flame tip in a wax pool.
func loadCandleProp(shader rl.Shader) propModel {
	puddle := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.10, 0.02, 10))
	candle := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.05, 0.16, 8))
	flame := rl.LoadModelFromMesh(rl.GenMeshSphere(0.04, 6, 8))
	tip := rl.LoadModelFromMesh(rl.GenMeshSphere(0.02, 6, 6))
	models := []rl.Model{puddle, candle, flame, tip}
	shadeAll(models, shader)
	wax := rl.NewColor(244, 220, 156, 255)
	waxDark := rl.NewColor(196, 168, 108, 255)
	// Shared torch/flame palette (world.go) so the candle flame matches the brazier.
	fire := torchFlameTints[1]
	fireBright := torchFlameTints[0]
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.008, 0), scale: rl.NewVector3(1, 1, 1), tint: waxDark},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.10, 0), scale: rl.NewVector3(1, 1, 1), tint: wax},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.22, 0), scale: rl.NewVector3(1, 1.8, 1), tint: fire},
			{modelIdx: 3, offset: rl.NewVector3(0, 0.28, 0), scale: rl.NewVector3(1, 2, 1), tint: fireBright},
		},
	}
}

// loadBootprintsProp builds two flat floor impressions in a forward-stride layout.
func loadBootprintsProp(shader rl.Shader) propModel {
	print := rl.LoadModelFromMesh(rl.GenMeshCube(0.10, 0.015, 0.18))
	heel := rl.LoadModelFromMesh(rl.GenMeshCube(0.10, 0.015, 0.06))
	models := []rl.Model{print, heel}
	shadeAll(models, shader)
	mud := rl.NewColor(90, 68, 44, 255)
	mudDark := rl.NewColor(60, 44, 28, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(-0.10, 0.008, -0.06), scale: rl.NewVector3(1, 1, 1), tint: mud},
			{modelIdx: 1, offset: rl.NewVector3(-0.10, 0.008, -0.18), scale: rl.NewVector3(1, 1, 1), tint: mudDark},
			{modelIdx: 0, offset: rl.NewVector3(0.10, 0.008, 0.08), scale: rl.NewVector3(1, 1, 1), tint: mud},
			{modelIdx: 1, offset: rl.NewVector3(0.10, 0.008, -0.04), scale: rl.NewVector3(1, 1, 1), tint: mudDark},
		},
	}
}

// loadAshHeapProp builds a cool-grey ash mound. Unlike DecorScorch (a flat
// ring) it has volume, reading as a campfire-site remnant.
func loadAshHeapProp(shader rl.Shader) propModel {
	heap := rl.LoadModelFromMesh(rl.GenMeshSphere(0.16, 8, 8))
	dust := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.22, 0.02, 12))
	models := []rl.Model{heap, dust}
	shadeAll(models, shader)
	ash := rl.NewColor(132, 124, 116, 255)
	ashDark := rl.NewColor(80, 72, 64, 255)
	dustTone := rl.NewColor(160, 150, 138, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 1, offset: rl.NewVector3(0, 0.008, 0), scale: rl.NewVector3(1, 1, 1), tint: dustTone},
			{modelIdx: 0, offset: rl.NewVector3(0, 0.06, 0), scale: rl.NewVector3(1, 0.45, 1), tint: ash},
			{modelIdx: 0, offset: rl.NewVector3(0.04, 0.04, -0.04), scale: rl.NewVector3(1, 0.3, 1), tint: ashDark},
		},
	}
}

// loadPuddleProp builds a shallow puddle: a flat cylinder with a brighter rim.
func loadPuddleProp(shader rl.Shader) propModel {
	disc := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.26, 0.015, 14))
	highlight := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.18, 0.02, 12))
	models := []rl.Model{disc, highlight}
	shadeAll(models, shader)
	water := rl.NewColor(108, 154, 188, 255)
	waterBright := rl.NewColor(180, 214, 232, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.006, 0), scale: rl.NewVector3(1, 1, 1), tint: water},
			{modelIdx: 1, offset: rl.NewVector3(-0.04, 0.012, -0.04), scale: rl.NewVector3(1, 1, 1), tint: waterBright},
		},
	}
}

// loadRootClusterProp builds gnarled roots poking from the floor: brown
// cylinder arches at varied tilts.
func loadRootClusterProp(shader rl.Shader, barkTex rl.Texture2D) propModel {
	arch := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.04, 0.30, 8))
	knob := rl.LoadModelFromMesh(rl.GenMeshSphere(0.05, 6, 8))
	models := []rl.Model{arch, knob}
	textureAndShade(models, shader, barkTex)
	rootCol := rl.NewColor(92, 68, 44, 255)
	rootDark := rl.NewColor(60, 44, 30, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0.06, 0.04, 0.04), scale: rl.NewVector3(1, 1, 1), rotation: 70, rotationAxis: rl.NewVector3(1, 0, 1), tint: rootCol},
			{modelIdx: 0, offset: rl.NewVector3(-0.08, 0.04, -0.02), scale: rl.NewVector3(1, 1, 1), rotation: 60, rotationAxis: rl.NewVector3(1, 0, -1), tint: rootDark},
			{modelIdx: 0, offset: rl.NewVector3(0.04, 0.04, -0.10), scale: rl.NewVector3(1, 0.85, 1), rotation: 80, rotationAxis: rl.NewVector3(0, 0, 1), tint: rootCol},
			{modelIdx: 1, offset: rl.NewVector3(0.12, 0.05, 0.04), scale: rl.NewVector3(1, 1, 1), tint: rootDark},
			{modelIdx: 1, offset: rl.NewVector3(-0.08, 0.06, -0.10), scale: rl.NewVector3(1, 1, 1), tint: rootCol},
		},
	}
}
