package render

import (
	"image/color"
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// treeModel bundles the procedurally-generated meshes that make up a single
// blocking tree. Unique meshes live in `models`; `parts` reference them by
// index so a single mesh can be reused at multiple positions/tints without
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
	// rotationAxis lets a part tilt off the default vertical axis. Zero
	// vector means "use world up (0,1,0)" so existing parts that don't set
	// it keep their old behavior. Set to e.g. {1, 4, 0} to tilt a crystal
	// toward +X while still mostly pointing up.
	rotationAxis rl.Vector3
	tint         color.RGBA
	// sway is the per-part wind-displacement factor (0..1). Default 0
	// means the part is rigid — used for stems / trunks / statue
	// bases that should stay planted. Upper foliage parts of
	// tall-grass, flowers, bushes, reeds, ferns, and clover set
	// sway >= 1 so they ride the global wind animation. propModel.draw
	// adds a time-based horizontal offset proportional to this value
	// so foliage breathes while statues stay still.
	sway float32
}

// partRotationAxis is an internal helper: returns the axis a part rotates
// around for its DrawModelEx call, falling back to world-up when the part
// left rotationAxis at the zero value. Package-internal — propModel.draw
// and treeModel.draw are the only callers.
func partRotationAxis(p treePart) rl.Vector3 {
	if p.rotationAxis.X == 0 && p.rotationAxis.Y == 0 && p.rotationAxis.Z == 0 {
		return rl.NewVector3(0, 1, 0)
	}
	return p.rotationAxis
}

// isVerticalAxis reports whether a part rotates around the world-up axis
// (either explicitly or via the zero-vector default). Used by the prop /
// tree draw paths to decide whether the prop's overall yaw can be folded
// into the part's own rotation by simple addition (only valid when the
// two rotations share an axis).
func isVerticalAxis(axis rl.Vector3) bool {
	if axis.X == 0 && axis.Y == 0 && axis.Z == 0 {
		return true
	}
	return axis.X == 0 && axis.Z == 0 && axis.Y != 0
}

// rotateOffsetY scales an offset and rotates it around the world-up axis
// by `yawDeg` degrees. Used to reorient a prop's parts around its own
// vertical centerline when the whole prop is yawed.
func rotateOffsetY(offset rl.Vector3, scale, yawDeg float32) rl.Vector3 {
	scaled := rl.NewVector3(offset.X*scale, offset.Y*scale, offset.Z*scale)
	if yawDeg == 0 {
		return scaled
	}
	return rl.Vector3RotateByAxisAngle(scaled, rl.NewVector3(0, 1, 0), yawDeg*float32(math.Pi)/180)
}

const (
	treeMeshRoot = iota // wide squat cylinder at the base — reads as the trunk's root flare
	treeMeshTrunk
	treeMeshCanopyLow
	treeMeshCanopyHigh
	treeMeshCanopySide
	treeMeshCanopyAccent
)

func loadTreeModel(shader rl.Shader, barkTex, leafTex rl.Texture2D) treeModel {
	models := []rl.Model{
		treeMeshRoot: rl.LoadModelFromMesh(rl.GenMeshCylinder(0.32, 0.18, 10)),
		// Trunk lengthened from 1.55 → 2.55 so the silhouette reads
		// as a grown tree rather than a stubby shrub.
		treeMeshTrunk: rl.LoadModelFromMesh(rl.GenMeshCylinder(0.18, 2.55, 12)),
		// Canopy lumps enlarged from the prior 0.92/0.78/0.55/0.38
		// pass — pushes the silhouette toward a Wind-Waker storybook
		// "puff" dome. The Low lump is now the dominant base mass;
		// High sits over it as a brighter crown; Side lumps spread
		// laterally; Accent gives the gilt highlights.
		treeMeshCanopyLow:    rl.LoadModelFromMesh(rl.GenMeshSphere(1.18, 14, 18)),
		treeMeshCanopyHigh:   rl.LoadModelFromMesh(rl.GenMeshSphere(0.96, 14, 18)),
		treeMeshCanopySide:   rl.LoadModelFromMesh(rl.GenMeshSphere(0.68, 12, 16)),
		treeMeshCanopyAccent: rl.LoadModelFromMesh(rl.GenMeshSphere(0.46, 10, 14)),
	}
	for i := range models {
		tex := leafTex
		if i == treeMeshRoot || i == treeMeshTrunk {
			tex = barkTex
		}
		setModelTexture(&models[i], tex)
		attachShader(&models[i], shader)
	}

	// Pastel-but-saturated canopy — soft spring green with cream
	// and lemon-gold accents, matched to the leaf texture's
	// bumped chroma so the puffy dome reads lush against the
	// blue sky.
	leafBase := color.RGBA{R: 146, G: 204, B: 114, A: 255}
	leafMid := color.RGBA{R: 124, G: 188, B: 106, A: 255}
	leafDeep := color.RGBA{R: 98, G: 160, B: 96, A: 255}
	leafGold := color.RGBA{R: 230, G: 220, B: 142, A: 255}
	leafBloom := color.RGBA{R: 212, G: 232, B: 160, A: 255}

	return treeModel{
		models: models,
		parts: []treePart{
			{modelIdx: treeMeshTrunk, offset: rl.NewVector3(0, 0.06, 0), scale: rl.NewVector3(1, 1, 1), tint: rl.White},
			// Dominant low canopy — broad mass anchoring the dome.
			{modelIdx: treeMeshCanopyLow, offset: rl.NewVector3(0, 2.55, 0), scale: rl.NewVector3(1, 0.95, 1), tint: leafMid},
			// Crown — slightly offset upward, brighter, catches the
			// sky tint.
			{modelIdx: treeMeshCanopyHigh, offset: rl.NewVector3(-0.05, 3.20, 0.05), scale: rl.NewVector3(1, 1, 1), tint: leafBase},
			// Side lumps spreading wide — the painterly puff
			// shoulders.
			{modelIdx: treeMeshCanopySide, offset: rl.NewVector3(0.55, 2.90, 0.18), scale: rl.NewVector3(1, 1, 1), tint: leafDeep},
			{modelIdx: treeMeshCanopySide, offset: rl.NewVector3(-0.50, 2.72, -0.18), scale: rl.NewVector3(1, 1, 1), tint: leafMid},
			// Top bloom — bright cream lump kissing the very top.
			// This is the Wind Waker "sunlight catches the crown"
			// signature highlight.
			{modelIdx: treeMeshCanopyAccent, offset: rl.NewVector3(0.08, 3.62, -0.06), scale: rl.NewVector3(1, 0.9, 1), tint: leafBloom},
			// Two gold accents scattered around the canopy as
			// sun-dappled gilt highlights.
			{modelIdx: treeMeshCanopyAccent, offset: rl.NewVector3(0.30, 3.40, -0.22), scale: rl.NewVector3(1, 1, 1), tint: leafGold},
			{modelIdx: treeMeshCanopyAccent, offset: rl.NewVector3(-0.26, 3.26, 0.28), scale: rl.NewVector3(1, 1, 1), tint: leafGold},
		},
	}
}

// draw renders the tree at center, scaled, with `yaw` degrees of rotation
// around the vertical axis through the tree's trunk. Yaw rotates each part's
// offset around the prop center so the whole canopy/lump arrangement
// reorients with the prop. Parts whose own rotationAxis is the default
// vertical also have yaw added to their part.rotation so their geometry
// (cylinders, etc.) spins in step. Tilted-axis parts can't compose cleanly
// with yaw without matrix math, so they keep their tilted spin and only
// the offset rotates — visually fine because most tilted parts in the
// codebase are sphere-ish lumps that read symmetrically anyway.
func (t treeModel) draw(center rl.Vector3, scale, yaw float32) {
	if scale <= 0 {
		scale = 1
	}
	for _, part := range t.parts {
		offset := rotateOffsetY(part.offset, scale, yaw)
		position := rl.NewVector3(center.X+offset.X, center.Y+offset.Y, center.Z+offset.Z)
		drawScale := rl.NewVector3(part.scale.X*scale, part.scale.Y*scale, part.scale.Z*scale)
		rotation := part.rotation
		if isVerticalAxis(part.rotationAxis) {
			rotation += yaw
		}
		rl.DrawModelEx(t.models[part.modelIdx], position, partRotationAxis(part), rotation, drawScale, part.tint)
	}
}

// drawVaried renders the tree with per-tile shape variance seeded from
// `seed` so a grove of identical-char tiles no longer stamps the same
// silhouette across the field. Variance is bounded so the difference
// reads as "this tree grew a little differently" — not "this is a
// different species" — and stays inside the tile footprint:
//
//   - Overall scale walks ±10% off `scale`.
//   - Canopy parts get an independent per-part scale jitter (±14% per
//     axis) plus a small horizontal nudge so foliage lumps sit in
//     different relative positions tree-to-tree.
//   - Each canopy lump's tint walks ±14 in R/G/B (alpha preserved) so
//     leaf color shifts per tile without authoring extra materials.
//   - One side-canopy lump is dropped at random (~25%) so some trees
//     read fuller and others sparser.
//   - Trunk parts ride the overall scale jitter only — kept plumb so
//     they read as varied, not broken.
//
// seed==0 still produces deterministic output, so callers that don't
// care about per-tile variance can pass 0 and get a stable tree. The
// legacy treeModel.draw is preserved for menu/title/preview call sites
// that aren't tile-positioned.
func (t treeModel) drawVaried(center rl.Vector3, scale, yaw float32, seed uint32) {
	if scale <= 0 {
		scale = 1
	}
	// Mix once so per-byte slices below are decorrelated even when
	// neighboring tiles' seeds differ by only a few bits.
	mix := seed*2654435761 ^ 0xC2B2AE3D
	mix ^= mix >> 16
	mix *= 0x85EBCA6B
	mix ^= mix >> 13
	frac := func(b byte) float32 { return (float32(int(b)) - 128) / 128 }

	overall := scale * (1 + frac(byte(mix))*0.10)
	// Height-only stretch: independent of overall scale so some trees
	// read as noticeably taller / shorter than their neighbors even
	// when the trunk girth is similar. Range ±28% — wide enough to be
	// readable from the player's POV but bounded so a stretched canopy
	// doesn't punch through the ceiling on indoor maps.
	heightStretch := 1 + frac(byte(mix>>4))*0.28

	// Drop one side-canopy lump ~25% of the time. dropIdx == -1 means
	// "draw everything." Walks parts to locate side canopies so this
	// rule survives part-list reshuffles in loadTreeModel.
	dropIdx := -1
	if byte(mix>>8) < 64 {
		nthSide := int(byte(mix>>16)) % 2
		seen := 0
		for i, part := range t.parts {
			if part.modelIdx == treeMeshCanopySide {
				if seen == nthSide {
					dropIdx = i
					break
				}
				seen++
			}
		}
	}

	// Per-tile tree species pick. ~10 % of trees render with a soft
	// painted-pink "blossom" canopy, ~10 % with a warm autumn
	// rust-gold canopy, the rest stay in the muted green family.
	// All three families come from the same leaf texture so we get
	// variety without authoring extra atlases — only the tint
	// changes through speciesCanopyTint below.
	speciesRoll := byte(mix >> 20)
	species := treeSpeciesGreen
	switch {
	case speciesRoll < 26:
		species = treeSpeciesBlossom
	case speciesRoll < 52:
		species = treeSpeciesAutumn
	}

	// Canopy sway — gentle wind animation. Each tree gets a unique
	// phase offset from its seed so a stand of trees breathes
	// asynchronously instead of stamping the same rocking motion
	// across the grid. Amplitude stays small (~0.05 world units)
	// so the silhouette reads as "leaves drifting in a breeze" not
	// "the tree is about to fall." Trunks are NOT swayed — only
	// the foliage rides the wind, the way Wind Waker trees do.
	swayTime := float32(rl.GetTime())
	swayPhase := frac(byte(mix>>22)) * 6.283185
	swayX := float32(math.Sin(float64(swayTime*0.85+swayPhase))) * 0.05
	swayZ := float32(math.Sin(float64(swayTime*0.72+swayPhase+1.3))) * 0.04
	swayY := float32(math.Sin(float64(swayTime*1.05+swayPhase*0.7))) * 0.025

	for i, part := range t.parts {
		if i == dropIdx {
			continue
		}
		isCanopy := part.modelIdx != treeMeshRoot && part.modelIdx != treeMeshTrunk

		var sx, sy, sz float32 = 1, 1, 1
		var nudgeX, nudgeZ float32
		if isCanopy {
			sx = 1 + frac(byte(mix>>uint(3+i*3)))*0.14
			sy = 1 + frac(byte(mix>>uint(5+i*5)))*0.14
			sz = 1 + frac(byte(mix>>uint(7+i*7)))*0.14
			nudgeX = frac(byte(mix>>uint(11+i*11))) * 0.20
			nudgeZ = frac(byte(mix>>uint(13+i*13))) * 0.20
		}

		// Height stretch lifts canopy parts proportionally to their
		// authored Y offset so the foliage rides on top of the
		// stretched trunk, and stretches the trunk mesh itself so the
		// bark cylinder fills the gap. Trunk Y scale and canopy lift
		// use the same factor so they grow / shrink in lockstep.
		yOffset := part.offset.Y * heightStretch
		trunkYScale := float32(1.0)
		if part.modelIdx == treeMeshTrunk {
			trunkYScale = heightStretch
		}

		offX := part.offset.X + nudgeX
		offZ := part.offset.Z + nudgeZ
		// Canopy lumps lean in the wind; higher lumps lean more
		// than lower lumps (canopyLean scales the sway by the
		// part's Y offset relative to a reference height) so the
		// crown drifts further than the under-canopy mass — the
		// "tree breathing in a breeze" feel rather than the whole
		// silhouette sliding sideways. Trunk and root are skipped.
		offsetY := yOffset
		if isCanopy {
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
		drawScale := rl.NewVector3(part.scale.X*sx*overall, part.scale.Y*sy*trunkYScale*overall, part.scale.Z*sz*overall)
		rotation := part.rotation
		if isVerticalAxis(part.rotationAxis) {
			rotation += yaw
		}
		tint := part.tint
		if isCanopy {
			// Pick the species palette first (green / blossom /
			// autumn), then apply the standard ±14 per-channel
			// jitter on top so lumps within one tree still walk
			// in tone but the family colour is preserved.
			tint = jitterTint(speciesCanopyTint(part.tint, species), mix>>uint(7+i*4), 14)
		}
		rl.DrawModelEx(t.models[part.modelIdx], position, partRotationAxis(part), rotation, drawScale, tint)
	}
}

// treeSpecies labels a per-tile painted-canopy palette. The seed bit
// pick in drawVaried rolls one of three options so a forest reads as
// mixed-species rather than a monoculture.
type treeSpecies int

const (
	treeSpeciesGreen treeSpecies = iota
	treeSpeciesBlossom
	treeSpeciesAutumn
)

// speciesCanopyTint returns the per-species painted-canopy colour for
// a given authored leaf tint. Green leaves pass through unchanged
// (the muted forest palette); blossom remaps to a soft painted-pink
// family; autumn remaps to a warm rust-gold family. Brightness of the
// original tint is broadly preserved so highlight lumps stay
// highlight-bright and shadow lumps stay shadow-deep, just hue-shifted.
func speciesCanopyTint(orig color.RGBA, species treeSpecies) color.RGBA {
	if species == treeSpeciesGreen {
		return orig
	}
	// Brightness key — average of the green channel (which carries
	// most of the original leaf-luminance) with a touch of R for
	// the highlight lumps.
	key := (int(orig.G)*2 + int(orig.R)) / 3
	switch species {
	case treeSpeciesBlossom:
		// Soft painted blossom — rose pink. Cream-leaning highlights,
		// rosier midtones, dusky maroon shadows. R lifted slightly,
		// G dropped, B nudged toward warm.
		return color.RGBA{
			R: core.ClampByte(key + 56),
			G: core.ClampByte(key - 22),
			B: core.ClampByte(key + 4),
			A: orig.A,
		}
	case treeSpeciesAutumn:
		// Warm rust-gold — autumn-foliage palette. R bumped, G
		// pulled toward amber, B dropped so the canopy reads as
		// burnt orange against the muted green field.
		return color.RGBA{
			R: core.ClampByte(key + 52),
			G: core.ClampByte(key - 6),
			B: core.ClampByte(key - 40),
			A: orig.A,
		}
	}
	return orig
}

// jitterTint shifts an RGBA's R/G/B channels by up to ±amp using bytes
// pulled from `bits`. Alpha is preserved. Channel clamping goes through
// core.ClampByte so the [0, 255] rule lives in one place.
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

// propModel is a generic multi-mesh prop (boulders, bushes, mushrooms) built
// from a small set of unique meshes referenced by index. Same shape as
// treeModel, just decoupled so each prop family can own its texture/tint set
// without entangling.
type propModel struct {
	models []rl.Model
	parts  []treePart // reuse the same per-part record (modelIdx, offset, scale, tint)
}

// draw renders the prop with a uniform scale and a yaw rotation around its
// vertical axis. See treeModel.draw for the rationale on yaw composition.
//
// Parts that carry a non-zero `sway` get a small time-based horizontal
// offset added to their position so foliage props breathe in the wind
// while rigid props (statues, pillars, crates) stay planted. Phase is
// hashed from the prop's world position so a row of grass clumps
// doesn't ripple in lockstep — adjacent tiles drift independently.
func (p propModel) draw(center rl.Vector3, scale, yaw float32) {
	if scale <= 0 {
		scale = 1
	}
	swayTime := float32(rl.GetTime())
	// Position-derived phase: hash the rounded tile coords so each
	// prop tile lands on a different point in the sway cycle.
	posPhase := float32(math.Mod(float64(center.X)*0.73+float64(center.Z)*1.31, 6.283185))
	swayX := float32(math.Sin(float64(swayTime*1.10+posPhase))) * 0.035
	swayZ := float32(math.Sin(float64(swayTime*0.95+posPhase+1.4))) * 0.030
	for _, part := range p.parts {
		offset := rotateOffsetY(part.offset, scale, yaw)
		position := rl.NewVector3(center.X+offset.X, center.Y+offset.Y, center.Z+offset.Z)
		if part.sway > 0 {
			// Lean scales with Y so taller parts of a clump drift
			// further than the base — natural "grass bending in
			// the wind" shape.
			lean := part.sway * (0.4 + part.offset.Y*1.5)
			position.X += swayX * lean
			position.Z += swayZ * lean
		}
		drawScale := rl.NewVector3(part.scale.X*scale, part.scale.Y*scale, part.scale.Z*scale)
		rotation := part.rotation
		if isVerticalAxis(part.rotationAxis) {
			rotation += yaw
		}
		rl.DrawModelEx(p.models[part.modelIdx], position, partRotationAxis(part), rotation, drawScale, part.tint)
	}
}

func (p propModel) unload() {
	for i := range p.models {
		rl.UnloadModel(p.models[i])
	}
}

// Rock prop mesh indices. The boulder is built from two sizes of low-poly
// faceted spheres — the low slice/ring counts give a jagged polygonal
// silhouette that reads as a chunky stone rather than a smooth river
// pebble. rockMeshBase still exists as models[0] because world.go's
// drawPebbleCluster consumes it directly for ground scatter, but the
// boulder itself no longer draws a cube base under the lumps (it looked
// like a pedestal — see the parts list below).
const (
	rockMeshBase  = iota // flat cube — used by the pebble drawer, not the boulder
	rockMeshLump         // medium faceted lump (5 rings × 6 slices)
	rockMeshChunk        // small faceted chunk (4 rings × 5 slices)
)

// RockMeshBaseHeight is the Y dimension passed to GenMeshCube for the
// rockMeshBase model (line below). Exported so drawPebbleCluster in
// world.go can compute its y-anchor as RockMeshBaseHeight/2 * hght —
// "lift the cube half its scaled height so it sits flat on the
// ground" — without baking the literal twice. Changing the cube's
// mesh height now requires editing this constant only.
const RockMeshBaseHeight = float32(0.36)

// RockMeshBaseHalfHeight is the y-anchor scale used by ground-scatter
// draws of rockMeshBase. drawPebbleCluster multiplies it by the
// pebble's per-instance height scale (`hght`) so the cube's bottom
// face lands flush with the floor regardless of the height jitter.
const RockMeshBaseHalfHeight = RockMeshBaseHeight / 2

// loadRockProp builds a chunky polygonal boulder: a flat base with two or
// three faceted lumps fused on top at varied angles, all in close-grouped
// stone greys. The intent is "weathered rock outcrop you'd see in a
// fantasy field map" — low silhouette, jagged facets, no upward spires.
// Slice/ring counts are kept low (4–6) so the lumps look polygonal rather
// than billiard-ball smooth.
func loadRockProp(shader rl.Shader, rockTex rl.Texture2D) propModel {
	models := []rl.Model{
		rockMeshBase: rl.LoadModelFromMesh(rl.GenMeshCube(1.10, RockMeshBaseHeight, 0.95)),
		// Sphere with low ring/slice count: each face is large enough to
		// catch a distinct lighting value, which is what reads as "rock"
		// vs "ball." 5×6 and 4×5 are the sweet spot — fewer looks like a
		// die, more smooths into a pillow.
		rockMeshLump:  rl.LoadModelFromMesh(rl.GenMeshSphere(0.55, 5, 6)),
		rockMeshChunk: rl.LoadModelFromMesh(rl.GenMeshSphere(0.36, 4, 5)),
	}
	for i := range models {
		setModelTexture(&models[i], rockTex)
		attachShader(&models[i], shader)
	}

	// Stone palette — close-grouped pale greys with slight warm/cool
	// variation so the parts read as one boulder broken at fault lines,
	// not separate rocks stacked together. Lighter than the previous pass
	// (which read as charcoal) so the boulder pops from the field's grass
	// floor and the wall texture.
	warm := rl.NewColor(214, 204, 188, 255)
	cool := rl.NewColor(196, 198, 202, 255)
	dark := rl.NewColor(176, 172, 164, 255)
	light := rl.NewColor(232, 224, 210, 255)

	return propModel{
		models: models,
		parts: []treePart{
			// Main mass: biggest lump, sitting directly on the ground.
			// Squashed vertically (y scale 0.85) so the silhouette reads
			// as a stout stone. Tilted on a (1,4,1) axis so facets don't
			// align to world axes — looks naturally weathered.
			{modelIdx: rockMeshLump, offset: rl.NewVector3(-0.10, 0.40, 0.05), scale: rl.NewVector3(1.25, 0.85, 1.20), rotation: 17, rotationAxis: rl.NewVector3(1, 4, 1), tint: warm},

			// Side mass fused into the main lump: smaller, rotated on a
			// different tilted axis so its facets break the main lump's.
			{modelIdx: rockMeshLump, offset: rl.NewVector3(0.38, 0.32, -0.22), scale: rl.NewVector3(1.0, 0.75, 1.10), rotation: -28, rotationAxis: rl.NewVector3(2, 5, 1), tint: cool},

			// Top chunk: gives the boulder an asymmetric peak. Modest
			// height keeps the silhouette earthbound — no spires.
			{modelIdx: rockMeshChunk, offset: rl.NewVector3(-0.22, 0.62, 0.12), scale: rl.NewVector3(1.15, 0.7, 1.15), rotation: 41, rotationAxis: rl.NewVector3(1, 5, 0), tint: dark},

			// Back chip: breaks the silhouette on the +Z side so the
			// boulder reads differently from each angle.
			{modelIdx: rockMeshChunk, offset: rl.NewVector3(0.06, 0.26, 0.42), scale: rl.NewVector3(1.05, 0.75, 1.05), rotation: 11, rotationAxis: rl.NewVector3(0, 6, 1), tint: dark},

			// Broken-off pebble at the base flank — visual interest at
			// ground level, makes the boulder feel weathered (eroded
			// chunks settling at its foot) instead of freshly placed.
			{modelIdx: rockMeshChunk, offset: rl.NewVector3(-0.52, 0.13, 0.08), scale: rl.NewVector3(0.65, 0.45, 0.65), rotation: 65, rotationAxis: rl.NewVector3(1, 3, 0), tint: light},
		},
	}
}

// loadRockCairnProp builds a 1-tile stacked-stone cairn — three faceted
// lumps fused on top of each other, taller than the squat boulder so the
// silhouette reads as "built by hands" rather than "natural rock." Same
// stone palette as the boulder so they sit comfortably together in the
// plaza.
func loadRockCairnProp(shader rl.Shader, rockTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.42, 5, 7)),
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.32, 5, 6)),
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.22, 4, 5)),
	}
	for i := range models {
		setModelTexture(&models[i], rockTex)
		attachShader(&models[i], shader)
	}
	warm := rl.NewColor(214, 204, 188, 255)
	cool := rl.NewColor(196, 198, 202, 255)
	dark := rl.NewColor(176, 172, 164, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.34, 0), scale: rl.NewVector3(1.1, 0.85, 1.1), rotation: 13, rotationAxis: rl.NewVector3(1, 4, 1), tint: warm},
			{modelIdx: 1, offset: rl.NewVector3(0.04, 0.78, -0.06), scale: rl.NewVector3(1.0, 0.95, 1.0), rotation: -22, rotationAxis: rl.NewVector3(2, 5, 1), tint: cool},
			{modelIdx: 2, offset: rl.NewVector3(-0.05, 1.10, 0.04), scale: rl.NewVector3(1.0, 0.95, 1.0), rotation: 38, rotationAxis: rl.NewVector3(1, 5, 0), tint: dark},
		},
	}
}

// loadRockFormationProp builds a 2×2 footprint rock formation — a knot of
// several large lumps fused together to span four tiles. Offsets put the
// model's origin at the CENTER of the 2×2 footprint, which means the
// renderer draws this at (anchorX+0.5, anchorZ+0.5) tile-space (= one
// TileSize east + south of the anchor's tile center). Lumps spread ~1.6
// world units (~0.8 tile widths each direction) so the silhouette fills
// the 2-tile span comfortably.
func loadRockFormationProp(shader rl.Shader, rockTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.95, 6, 7)),
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.70, 5, 6)),
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.55, 5, 6)),
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.40, 4, 5)),
	}
	for i := range models {
		setModelTexture(&models[i], rockTex)
		attachShader(&models[i], shader)
	}
	warm := rl.NewColor(214, 204, 188, 255)
	cool := rl.NewColor(196, 198, 202, 255)
	dark := rl.NewColor(176, 172, 164, 255)
	light := rl.NewColor(232, 224, 210, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Central mass — biggest lump anchoring the cluster.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.75, 0), scale: rl.NewVector3(1.05, 0.95, 1.05), rotation: 17, rotationAxis: rl.NewVector3(1, 4, 1), tint: warm},
			// NE shoulder pushing into the +X/+Z quadrant.
			{modelIdx: 1, offset: rl.NewVector3(0.62, 0.55, 0.55), scale: rl.NewVector3(1.0, 0.85, 1.0), rotation: -28, rotationAxis: rl.NewVector3(2, 5, 1), tint: cool},
			// SW chunk, lower profile.
			{modelIdx: 2, offset: rl.NewVector3(-0.55, 0.42, -0.45), scale: rl.NewVector3(1.0, 0.8, 1.0), rotation: 41, rotationAxis: rl.NewVector3(1, 5, 0), tint: dark},
			// NW protrusion.
			{modelIdx: 2, offset: rl.NewVector3(-0.45, 0.50, 0.58), scale: rl.NewVector3(0.9, 0.75, 0.9), rotation: -52, rotationAxis: rl.NewVector3(0, 6, 1), tint: warm},
			// SE buttress.
			{modelIdx: 2, offset: rl.NewVector3(0.58, 0.46, -0.52), scale: rl.NewVector3(0.95, 0.8, 0.95), rotation: 11, rotationAxis: rl.NewVector3(1, 3, 0), tint: cool},
			// Crown lump on top.
			{modelIdx: 3, offset: rl.NewVector3(0.05, 1.18, -0.08), scale: rl.NewVector3(1.0, 0.85, 1.0), rotation: 65, rotationAxis: rl.NewVector3(1, 4, 0), tint: light},
			// Cap accent — slight asymmetric peak.
			{modelIdx: 3, offset: rl.NewVector3(-0.18, 1.32, 0.10), scale: rl.NewVector3(0.8, 0.75, 0.8), rotation: -25, rotationAxis: rl.NewVector3(2, 4, 1), tint: dark},
		},
	}
}

// loadArchwayDecor builds a stone archway whose footprint spans 1×2 tiles
// along +X. Two vertical pillars sit at (−1, 0) and (+1, 0) relative to
// the model origin (so the model origin lands BETWEEN the two tiles — the
// renderer offsets by +0.5 tile east of the anchor). A keystone block
// spans the top, completing the arch shape. Marble palette to match the
// existing pillar/statue stonework.
func loadArchwayDecor(shader rl.Shader, marbleTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCube(0.42, 2.10, 0.42)), // pillar shaft
		rl.LoadModelFromMesh(rl.GenMeshCube(0.58, 0.20, 0.58)), // pillar capital
		rl.LoadModelFromMesh(rl.GenMeshCube(2.20, 0.38, 0.46)), // arch keystone slab
		rl.LoadModelFromMesh(rl.GenMeshCube(0.48, 0.18, 0.48)), // base plinth
	}
	for i := range models {
		setModelTexture(&models[i], marbleTex)
		attachShader(&models[i], shader)
	}
	stone := rl.NewColor(220, 214, 198, 255)
	stoneCool := rl.NewColor(204, 196, 174, 255)
	stoneDark := rl.NewColor(178, 170, 152, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Left pillar: plinth, shaft, capital — stacked at -1 tile X.
			{modelIdx: 3, offset: rl.NewVector3(-1.00, 0.09, 0), scale: rl.NewVector3(1, 1, 1), tint: stoneDark},
			{modelIdx: 0, offset: rl.NewVector3(-1.00, 1.20, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			{modelIdx: 1, offset: rl.NewVector3(-1.00, 2.35, 0), scale: rl.NewVector3(1, 1, 1), tint: stoneCool},
			// Right pillar: mirror at +1 tile X.
			{modelIdx: 3, offset: rl.NewVector3(1.00, 0.09, 0), scale: rl.NewVector3(1, 1, 1), tint: stoneDark},
			{modelIdx: 0, offset: rl.NewVector3(1.00, 1.20, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			{modelIdx: 1, offset: rl.NewVector3(1.00, 2.35, 0), scale: rl.NewVector3(1, 1, 1), tint: stoneCool},
			// Keystone slab spanning both pillars at the top.
			{modelIdx: 2, offset: rl.NewVector3(0, 2.65, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
		},
	}
}

// loadBushProp builds a leaf-cluster bush with tiny flower blooms
// dotted across the top. Leaves share the tree's leaf texture; bloom
// spheres use the default white material tinted with the flower
// palette so the bush reads as a small, lively shrub catching
// wildflowers. Scale 1.0 = "large" (blocks); ~0.5 = "small."
func loadBushProp(shader rl.Shader, leafTex rl.Texture2D) propModel {
	leafLump := rl.LoadModelFromMesh(rl.GenMeshSphere(0.62, 12, 16))
	leafLumpSm := rl.LoadModelFromMesh(rl.GenMeshSphere(0.46, 10, 14))
	bloom := rl.LoadModelFromMesh(rl.GenMeshSphere(0.085, 8, 10))
	models := []rl.Model{leafLump, leafLumpSm, bloom}
	setModelTexture(&models[0], leafTex)
	setModelTexture(&models[1], leafTex)
	for i := range models {
		attachShader(&models[i], shader)
	}
	// Pastel-but-saturated bush — spring-green leaf lumps with
	// soft colourful blooms.
	leafBase := color.RGBA{R: 142, G: 200, B: 112, A: 255}
	leafDeep := color.RGBA{R: 102, G: 164, B: 100, A: 255}
	leafGold := color.RGBA{R: 224, G: 230, B: 152, A: 255}
	bloomYellow := color.RGBA{R: 242, G: 216, B: 122, A: 255}
	bloomWhite := color.RGBA{R: 244, G: 240, B: 226, A: 255}
	bloomPink := color.RGBA{R: 238, G: 176, B: 198, A: 255}
	return propModel{
		models: models,
		parts: []treePart{
			// Three overlapping leaf lumps — a dominant base with
			// two side lumps for the painterly "cluster of round
			// bunches" silhouette. Sway is modest so the bush
			// breathes without losing its rooted feel.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.52, 0), scale: rl.NewVector3(1, 0.92, 1), tint: leafBase, sway: 0.55},
			{modelIdx: 1, offset: rl.NewVector3(0.34, 0.68, 0.20), scale: rl.NewVector3(1, 1, 1), tint: leafDeep, sway: 0.65},
			{modelIdx: 1, offset: rl.NewVector3(-0.32, 0.64, -0.18), scale: rl.NewVector3(1, 1, 1), tint: leafGold, sway: 0.65},
			// Wildflower blooms dotted across the upper hemisphere
			// of the bush. Three colours so the patch reads as
			// mixed wildflowers, not a costume bouquet. Blooms
			// sway slightly more than the leaves below them.
			{modelIdx: 2, offset: rl.NewVector3(0.08, 0.96, 0.10), scale: rl.NewVector3(1, 1, 1), tint: bloomYellow, sway: 0.85},
			{modelIdx: 2, offset: rl.NewVector3(-0.22, 0.84, 0.04), scale: rl.NewVector3(1, 1, 1), tint: bloomWhite, sway: 0.85},
			{modelIdx: 2, offset: rl.NewVector3(0.20, 0.88, -0.18), scale: rl.NewVector3(1, 1, 1), tint: bloomPink, sway: 0.85},
		},
	}
}

// loadMushroomProp builds a small mushroom trio: one prominent storybook
// toadstool (red cap + paper-white spots) plus two smaller companion
// caps in pale cream and apricot so the patch reads as a forest-floor
// fairy-ring rather than a single solitary fungus. All parts ride on
// raylib's default white material and rely on tint colour.
func loadMushroomProp(shader rl.Shader) propModel {
	stem := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.05, 0.16, 8))
	capDome := rl.LoadModelFromMesh(rl.GenMeshSphere(0.15, 10, 12))
	spot := rl.LoadModelFromMesh(rl.GenMeshSphere(0.028, 6, 8))
	smallStem := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.03, 0.10, 8))
	smallCap := rl.LoadModelFromMesh(rl.GenMeshSphere(0.085, 8, 10))
	models := []rl.Model{stem, capDome, spot, smallStem, smallCap}
	for i := range models {
		attachShader(&models[i], shader)
	}
	stemTint := color.RGBA{R: 224, G: 218, B: 200, A: 255}
	stemDarker := color.RGBA{R: 200, G: 192, B: 172, A: 255}
	capRed := color.RGBA{R: 188, G: 92, B: 86, A: 255}
	capCream := color.RGBA{R: 218, G: 198, B: 160, A: 255}
	capApricot := color.RGBA{R: 210, G: 162, B: 132, A: 255}
	spotWhite := color.RGBA{R: 228, G: 224, B: 212, A: 255}
	return propModel{
		models: models,
		parts: []treePart{
			// Main toadstool — tall stem, domed red cap, four
			// painted-white spots scattered across the cap's
			// upper hemisphere.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.02, 0), scale: rl.NewVector3(1, 1, 1), tint: stemTint},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.18, 0), scale: rl.NewVector3(1, 0.72, 1), tint: capRed},
			{modelIdx: 2, offset: rl.NewVector3(0.05, 0.245, 0.02), scale: rl.NewVector3(1, 1, 1), tint: spotWhite},
			{modelIdx: 2, offset: rl.NewVector3(-0.04, 0.24, 0.05), scale: rl.NewVector3(1, 1, 1), tint: spotWhite},
			{modelIdx: 2, offset: rl.NewVector3(0.02, 0.255, -0.06), scale: rl.NewVector3(1, 1, 1), tint: spotWhite},
			{modelIdx: 2, offset: rl.NewVector3(-0.06, 0.23, -0.03), scale: rl.NewVector3(1, 1, 1), tint: spotWhite},
			// Companion 1 — small cream cap nestled to the side.
			{modelIdx: 3, offset: rl.NewVector3(0.18, 0.01, 0.12), scale: rl.NewVector3(1, 1, 1), tint: stemDarker},
			{modelIdx: 4, offset: rl.NewVector3(0.18, 0.11, 0.12), scale: rl.NewVector3(1, 0.74, 1), tint: capCream},
			// Companion 2 — apricot cap on the other side.
			{modelIdx: 3, offset: rl.NewVector3(-0.16, 0.01, -0.14), scale: rl.NewVector3(0.95, 0.95, 0.95), tint: stemDarker},
			{modelIdx: 4, offset: rl.NewVector3(-0.16, 0.10, -0.14), scale: rl.NewVector3(0.92, 0.72, 0.92), tint: capApricot},
		},
	}
}

// loadChestBodyProp builds the wooden chest body with painted metal
// hardware: four vertical corner straps, two horizontal hoop bands
// (top + bottom), a lockplate centred on the front face, and a small
// jewel pip at the centre of the lockplate. Bark texture for the
// wood; raylib's default material on the metal parts so the tint
// reads as cast bronze rather than wood-grain.
//
// Designed to read as a Wind-Waker treasure chest at any approach
// angle — the corner straps and hoop bands sell the "ironbound
// wooden box" silhouette; the lockplate + jewel sells "this is
// loot, come open me." Dimensions roughly match the prior raw-cube
// chest so the existing prompt anchor / shadow radius / collision
// don't need to retune.
func loadChestBodyProp(shader rl.Shader, barkTex rl.Texture2D) propModel {
	wood := rl.LoadModelFromMesh(rl.GenMeshCube(0.62, 0.46, 0.50))
	setModelTexture(&wood, barkTex)
	strap := rl.LoadModelFromMesh(rl.GenMeshCube(0.06, 0.48, 0.06))
	hoop := rl.LoadModelFromMesh(rl.GenMeshCube(0.66, 0.06, 0.54))
	lockplate := rl.LoadModelFromMesh(rl.GenMeshCube(0.20, 0.22, 0.04))
	jewel := rl.LoadModelFromMesh(rl.GenMeshSphere(0.045, 8, 10))
	models := []rl.Model{wood, strap, hoop, lockplate, jewel}
	for i := range models {
		attachShader(&models[i], shader)
	}
	woodTint := chestBodyColor
	// Muted brass / bronze for the iron banding — pulled into the
	// same warm metal family without flaring against the muted
	// wood beneath.
	metalDark := rl.NewColor(140, 108, 64, 255)
	metalBright := rl.NewColor(182, 148, 86, 255)
	jewelTint := rl.NewColor(198, 92, 80, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Wood body — sits at y = bodyHeight/2 so its base
			// flushes against the floor.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.23, 0), scale: rl.NewVector3(1, 1, 1), tint: woodTint},
			// Four vertical corner straps at the body's vertical
			// edges. Slightly proud of the wood so they read as
			// raised ironwork.
			{modelIdx: 1, offset: rl.NewVector3(0.31, 0.24, 0.26), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			{modelIdx: 1, offset: rl.NewVector3(-0.31, 0.24, 0.26), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			{modelIdx: 1, offset: rl.NewVector3(0.31, 0.24, -0.26), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			{modelIdx: 1, offset: rl.NewVector3(-0.31, 0.24, -0.26), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			// Bottom + top hoop bands hugging the body's
			// horizontal edges. The top hoop catches the lid
			// seam visually.
			{modelIdx: 2, offset: rl.NewVector3(0, 0.05, 0), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.43, 0), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			// Front-face lockplate — bright brass, sits just
			// proud of the wood so it reads as a mounted plate.
			{modelIdx: 3, offset: rl.NewVector3(0, 0.28, 0.27), scale: rl.NewVector3(1, 1, 1), tint: metalBright},
			// Lock jewel — small bright pip on the lockplate.
			// The "treasure inside" cue.
			{modelIdx: 4, offset: rl.NewVector3(0, 0.30, 0.30), scale: rl.NewVector3(1, 1, 1), tint: jewelTint},
		},
	}
}

// loadChestLidProp builds the wooden chest lid with painted metal
// hardware: four corner caps, a single bottom hoop where the lid
// meets the body, and the lid wood itself (slightly wider than the
// body so the lip overshoots the corner straps).
//
// Drawn separately from the body so the looted-chest path can lift
// the lid straight up + tilt it backward without disturbing the
// body's pose. Centre is at the lid's vertical midpoint (y = lid
// half-height) so the caller positions it by passing the body top
// as the centre Y.
func loadChestLidProp(shader rl.Shader, barkTex rl.Texture2D) propModel {
	wood := rl.LoadModelFromMesh(rl.GenMeshCube(0.68, 0.18, 0.56))
	setModelTexture(&wood, barkTex)
	cornerCap := rl.LoadModelFromMesh(rl.GenMeshCube(0.10, 0.20, 0.10))
	hoop := rl.LoadModelFromMesh(rl.GenMeshCube(0.70, 0.05, 0.58))
	models := []rl.Model{wood, cornerCap, hoop}
	for i := range models {
		attachShader(&models[i], shader)
	}
	woodTint := chestLidColor
	metalDark := rl.NewColor(140, 108, 64, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Wood lid — centred so passing the body-top Y as
			// the position centres the lid on the seam.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.09, 0), scale: rl.NewVector3(1, 1, 1), tint: woodTint},
			// Four corner caps at the lid's vertical edges.
			{modelIdx: 1, offset: rl.NewVector3(0.31, 0.10, 0.25), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			{modelIdx: 1, offset: rl.NewVector3(-0.31, 0.10, 0.25), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			{modelIdx: 1, offset: rl.NewVector3(0.31, 0.10, -0.25), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			{modelIdx: 1, offset: rl.NewVector3(-0.31, 0.10, -0.25), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
			// Bottom hoop band — hugs the lid's bottom edge,
			// catches the body's top hoop visually so the two
			// pieces read as ringed together.
			{modelIdx: 2, offset: rl.NewVector3(0, 0.025, 0), scale: rl.NewVector3(1, 1, 1), tint: metalDark},
		},
	}
}

// --- Inhabited / ruined props ---------------------------------------------
//
// The next nine loaders cover the "this place wasn't always empty" set:
// crates, barrels, urns, stalagmites, pillars (intact and broken), statues,
// obelisks, and fountains. They share three goals:
//
//   - Read at a glance from across the room — silhouette first, detail second.
//   - Use the existing low-poly, faceted vocabulary (cubes, cylinders, low-
//     slice spheres) so they fit the boulder/tree aesthetic.
//   - Take their diffuse textures as parameters so resources.go owns the
//     texture lifetime — no model holds a borrowed texture handle.

// loadCrateProp builds a wooden crate: chunky main cube wrapped in thin
// darker trim cubes along the top, bottom, and corner edges so it reads as
// banded boards instead of a solid block. Bark texture stands in for wood
// grain — the existing bark already has the right palette.
func loadCrateProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCube(0.86, 0.86, 0.86)),
		rl.LoadModelFromMesh(rl.GenMeshCube(0.92, 0.08, 0.92)),
		rl.LoadModelFromMesh(rl.GenMeshCube(0.08, 0.86, 0.08)),
	}
	for i := range models {
		setModelTexture(&models[i], woodTex)
		attachShader(&models[i], shader)
	}
	// Pastel crate wood — soft pecan matching the bark texture,
	// with warm-brown banding (not near-black) so the crate reads
	// gently even in the spooky dungeon lighting.
	wood := rl.NewColor(184, 144, 102, 255)
	band := rl.NewColor(124, 92, 60, 255)
	corner := rl.NewColor(104, 76, 50, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Main box: sits flush on the ground (its half-height equals
			// its y offset).
			{modelIdx: 0, offset: rl.NewVector3(0, 0.43, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			// Top and bottom rim — the wide thin cube sticks proud of the
			// crate's faces by 0.03 so it reads as raised iron banding.
			{modelIdx: 1, offset: rl.NewVector3(0, 0.84, 0), scale: rl.NewVector3(1, 1, 1), tint: band},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.04, 0), scale: rl.NewVector3(1, 1, 1), tint: band},
			// Four vertical corner straps. The corner cube is intentionally
			// taller than the main box so its ends overshoot the rim banding
			// — reads as a corner reinforcement bracket.
			{modelIdx: 2, offset: rl.NewVector3(0.43, 0.43, 0.43), scale: rl.NewVector3(1, 1, 1), tint: corner},
			{modelIdx: 2, offset: rl.NewVector3(-0.43, 0.43, 0.43), scale: rl.NewVector3(1, 1, 1), tint: corner},
			{modelIdx: 2, offset: rl.NewVector3(0.43, 0.43, -0.43), scale: rl.NewVector3(1, 1, 1), tint: corner},
			{modelIdx: 2, offset: rl.NewVector3(-0.43, 0.43, -0.43), scale: rl.NewVector3(1, 1, 1), tint: corner},
		},
	}
}

// loadBarrelProp builds a banded barrel: a tall cylinder with three
// dark hoop bands and a slightly proud top/bottom cap so the silhouette
// reads as a wooden barrel rather than a smooth canister.
func loadBarrelProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.42, 1.0, 18)),
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.45, 0.07, 20)),
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.44, 0.06, 18)),
	}
	for i := range models {
		setModelTexture(&models[i], woodTex)
		attachShader(&models[i], shader)
	}
	wood := rl.NewColor(192, 150, 104, 255)
	hoop := rl.NewColor(110, 80, 52, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.05, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			// Top cap and bottom cap — slightly wider, dark, reads as lid + base ring.
			{modelIdx: 1, offset: rl.NewVector3(0, 1.04, 0), scale: rl.NewVector3(1, 1, 1), tint: hoop},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.02, 0), scale: rl.NewVector3(1, 1, 1), tint: hoop},
			// Three hoop bands climbing the staves.
			{modelIdx: 2, offset: rl.NewVector3(0, 0.22, 0), scale: rl.NewVector3(1, 1, 1), tint: hoop},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.54, 0), scale: rl.NewVector3(1, 1, 1), tint: hoop},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.86, 0), scale: rl.NewVector3(1, 1, 1), tint: hoop},
		},
	}
}

// loadUrnProp builds a belly-shouldered ceramic urn: a flattened sphere
// body sitting on a small foot, with a narrow cylinder neck and a wider
// rim cylinder so the silhouette reads "amphora" rather than "vase."
// Terracotta texture warms the clay tone.
func loadUrnProp(shader rl.Shader, terracottaTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.36, 10, 14)),     // belly
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.18, 0.20, 16)), // neck
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.23, 0.05, 18)), // rim flare
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.20, 0.06, 14)), // foot
	}
	for i := range models {
		setModelTexture(&models[i], terracottaTex)
		attachShader(&models[i], shader)
	}
	clay := rl.NewColor(196, 122, 80, 255)
	clayDeep := rl.NewColor(140, 78, 52, 255)
	rim := rl.NewColor(112, 60, 36, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Foot first (sits flush to ground).
			{modelIdx: 3, offset: rl.NewVector3(0, 0.02, 0), scale: rl.NewVector3(1, 1, 1), tint: clayDeep},
			// Belly: scaled vertically just under 1 so the silhouette is
			// chubby-shouldered rather than perfectly round.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.44, 0), scale: rl.NewVector3(1, 0.92, 1), tint: clay},
			// Neck cylinder.
			{modelIdx: 1, offset: rl.NewVector3(0, 0.78, 0), scale: rl.NewVector3(1, 1, 1), tint: clayDeep},
			// Rim flare at the top of the neck — the small "lip" of an amphora.
			{modelIdx: 2, offset: rl.NewVector3(0, 0.97, 0), scale: rl.NewVector3(1, 1, 1), tint: rim},
		},
	}
}

// loadStalagmiteProp builds a tapered stone spire: four faceted spheres
// stacked with shrinking radius so the silhouette narrows to a point.
// Low slice counts keep the facets visible — the boulder/rock aesthetic.
func loadStalagmiteProp(shader rl.Shader, stoneTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.45, 5, 7)),
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.32, 5, 7)),
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.20, 5, 6)),
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.10, 5, 6)),
	}
	for i := range models {
		setModelTexture(&models[i], stoneTex)
		attachShader(&models[i], shader)
	}
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

// loadPillarProp builds a full Doric-ish column: square base + cylindrical
// shaft + square capital + thin abacus slab. Marble texture sells the
// "weathered temple stone" read; the slight tint walk from base to capital
// implies dust settled toward the bottom.
func loadPillarProp(shader rl.Shader, marbleTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCube(0.72, 0.18, 0.72)),   // plinth
		rl.LoadModelFromMesh(rl.GenMeshCube(0.62, 0.10, 0.62)),   // base
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.26, 2.05, 18)), // shaft
		rl.LoadModelFromMesh(rl.GenMeshCube(0.62, 0.16, 0.62)),   // echinus
		rl.LoadModelFromMesh(rl.GenMeshCube(0.74, 0.08, 0.74)),   // abacus
	}
	for i := range models {
		setModelTexture(&models[i], marbleTex)
		attachShader(&models[i], shader)
	}
	baseTint := rl.NewColor(206, 200, 184, 255)
	shaftTint := rl.NewColor(220, 214, 198, 255)
	capTint := rl.NewColor(228, 222, 206, 255)
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

// loadBrokenPillarProp builds a toppled / broken pillar stub. Same plinth
// as the intact pillar so adjacent pairs read as "this used to be a row,"
// but the shaft is cut chest-high and topped with a small jagged rubble
// cube tilted off-axis.
func loadBrokenPillarProp(shader rl.Shader, marbleTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCube(0.72, 0.18, 0.72)),
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.26, 0.90, 18)),
		rl.LoadModelFromMesh(rl.GenMeshCube(0.40, 0.18, 0.34)),
	}
	for i := range models {
		setModelTexture(&models[i], marbleTex)
		attachShader(&models[i], shader)
	}
	baseTint := rl.NewColor(196, 188, 170, 255)
	shaftTint := rl.NewColor(214, 206, 188, 255)
	rubbleTint := rl.NewColor(168, 160, 144, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.09, 0), scale: rl.NewVector3(1, 1, 1), tint: baseTint},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.22, 0), scale: rl.NewVector3(1, 1, 1), tint: shaftTint},
			// Jagged break: a thin cube tilted off vertical so the silhouette
			// reads as a sheared top, not a clean cut.
			{modelIdx: 2, offset: rl.NewVector3(0.04, 1.18, 0.03), scale: rl.NewVector3(1, 1, 1), rotation: 12, rotationAxis: rl.NewVector3(1, 0, 2), tint: rubbleTint},
		},
	}
}

// loadStatueProp builds a roughed-in humanoid statue on a pedestal:
// pedestal + boots + legs + torso + shoulders + head. No arms cast as
// separate parts — the shoulders slab covers the silhouette enough at
// the camera's distance, and adding arm cylinders that don't read at
// scale just adds clutter. Marble texture so adjacent pillars and the
// statue share a stone family.
func loadStatueProp(shader rl.Shader, marbleTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCube(0.92, 0.24, 0.92)),   // pedestal
		rl.LoadModelFromMesh(rl.GenMeshCube(0.55, 0.14, 0.55)),   // statue base
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.20, 0.55, 14)), // legs
		rl.LoadModelFromMesh(rl.GenMeshCube(0.48, 0.62, 0.30)),   // torso
		rl.LoadModelFromMesh(rl.GenMeshCube(0.66, 0.14, 0.34)),   // shoulders
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.18, 10, 12)),     // head
	}
	for i := range models {
		setModelTexture(&models[i], marbleTex)
		attachShader(&models[i], shader)
	}
	pedTint := rl.NewColor(192, 184, 168, 255)
	bodyTint := rl.NewColor(220, 214, 198, 255)
	headTint := rl.NewColor(228, 222, 206, 255)
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

// loadObeliskProp builds a tall, narrow four-sided shaft capped by a
// pyramid. The pyramid uses a low-slice sphere — 4 slices makes a
// rotational silhouette that reads as a four-sided peak from any angle.
// Granite texture distinguishes it from the marble of the pillars and
// statue: an obelisk should feel like a different stone class.
func loadObeliskProp(shader rl.Shader, graniteTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCube(0.88, 0.14, 0.88)), // base step
		rl.LoadModelFromMesh(rl.GenMeshCube(0.56, 2.20, 0.56)), // shaft
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.40, 4, 6)),     // pyramid cap (low-slice)
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.08, 6, 6)),     // apex
	}
	for i := range models {
		setModelTexture(&models[i], graniteTex)
		attachShader(&models[i], shader)
	}
	baseTint := rl.NewColor(70, 74, 86, 255)
	shaftTint := rl.NewColor(92, 96, 110, 255)
	capTint := rl.NewColor(126, 130, 146, 255)
	apexTint := rl.NewColor(186, 188, 198, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.07, 0), scale: rl.NewVector3(1, 1, 1), tint: baseTint},
			{modelIdx: 1, offset: rl.NewVector3(0, 1.22, 0), scale: rl.NewVector3(1, 1, 1), tint: shaftTint},
			// Pyramid cap: flatten vertically just slightly so it reads as a
			// tall pyramid (height ~0.55) rather than a hemispherical lid.
			{modelIdx: 2, offset: rl.NewVector3(0, 2.55, 0), scale: rl.NewVector3(0.85, 0.65, 0.85), rotation: 45, tint: capTint},
			{modelIdx: 3, offset: rl.NewVector3(0, 2.86, 0), scale: rl.NewVector3(1, 1, 1), tint: apexTint},
		},
	}
}

// loadFountainProp builds a round stone fountain: outer basin, water disc,
// and a central spout column with a small splash sphere. Uses the marble
// texture for the stone parts and leaves the water disc on raylib's
// default texture so the water tint reads cleanly.
func loadFountainProp(shader rl.Shader, marbleTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.78, 0.42, 24)), // outer basin
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.66, 0.06, 22)), // water disc
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.12, 0.45, 12)), // central spout
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.18, 10, 12)),     // splash bowl
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.82, 0.10, 24)), // rim coping
	}
	// Marble texture on all stone parts; water disc stays raylib-default so
	// its tint stays unmuddied by stone grain.
	for _, i := range []int{0, 2, 3, 4} {
		setModelTexture(&models[i], marbleTex)
	}
	for i := range models {
		attachShader(&models[i], shader)
	}
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
//
// Decor props are small (well under a tile), passable, and built from the
// cheapest possible primitive set. The renderer scatters auto-decor onto
// roughly 16% of plain floor tiles already; these new pieces are author-
// placed via the decor layer.

// loadTallGrassProp builds a clump of upright grass blades from five thin
// tall cubes spread across the tile and tilted outward at varied angles.
// Bright green at the tips, deeper green at the base — only one tint per
// blade since the slabs are too thin for per-vertex shading to matter.
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

// loadFlowerProp builds a wildflower clump: four stems each carrying a
// bigger painted bloom over a wider petal halo, with a yellow pistil
// pip on top. Three ground leaves anchor the cluster. The bloom palette
// (sun-yellow, rose-pink, paper-white, lilac) stays in a tight warm
// range so the patch reads as Wind-Waker wildflowers rather than a
// costume jewelry case.
func loadFlowerProp(shader rl.Shader) propModel {
	stem := rl.LoadModelFromMesh(rl.GenMeshCube(0.026, 0.24, 0.026))
	// Petal halo — flattened disc that sits under the bloom. Cube
	// scaled flat reads as the painted "open palm" of petals.
	petal := rl.LoadModelFromMesh(rl.GenMeshCube(0.12, 0.018, 0.12))
	bloom := rl.LoadModelFromMesh(rl.GenMeshSphere(0.075, 10, 12))
	// Pistil — tiny gold pip on top of each bloom catching the
	// "sunkissed" highlight.
	pistil := rl.LoadModelFromMesh(rl.GenMeshSphere(0.022, 6, 8))
	leaf := rl.LoadModelFromMesh(rl.GenMeshCube(0.055, 0.022, 0.075))
	models := []rl.Model{stem, petal, bloom, pistil, leaf}
	for i := range models {
		attachShader(&models[i], shader)
	}
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
			// Bloom 1 — yellow. Stems take a light sway, blooms +
			// petals take a heavier one so the flower head nods in
			// the wind above an only-slightly-bending stalk. Ground
			// leaves stay rigid.
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
			// Ground leaves — three of them now, rotated so the
			// patch reads as a deliberate clump.
			{modelIdx: 4, offset: rl.NewVector3(0.02, 0.012, 0.01), scale: rl.NewVector3(1.4, 1, 1.4), rotation: 20, tint: leafTint},
			{modelIdx: 4, offset: rl.NewVector3(-0.06, 0.012, -0.08), scale: rl.NewVector3(1.2, 1, 1.6), rotation: -45, tint: leafTint},
			{modelIdx: 4, offset: rl.NewVector3(0.10, 0.012, -0.12), scale: rl.NewVector3(1.1, 1, 1.4), rotation: 60, tint: leafTint},
		},
	}
}

// loadCloverProp builds a low ground-hugging clover patch: six flattened
// spheres tightly clustered at floor level. Bright green so it pops
// against a darker grass floor.
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

// loadReedProp builds a cluster of tall water reeds: six narrow tall cubes
// with very slight outward tilts. Used by water tiles and damp edges;
// the cooler olive tints distinguish them from the warmer tall grass.
func loadReedProp(shader rl.Shader) propModel {
	reed := rl.LoadModelFromMesh(rl.GenMeshCube(0.025, 0.62, 0.025))
	tip := rl.LoadModelFromMesh(rl.GenMeshCube(0.04, 0.07, 0.04))
	models := []rl.Model{reed, tip}
	for i := range models {
		attachShader(&models[i], shader)
	}
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

// loadBoneProp builds a small bone scatter: a skull-ish sphere and three
// long-bone cylinders lying across each other on the ground. White-yellow
// tints with deeper accents at the joints sell the "old, weathered" read.
func loadBoneProp(shader rl.Shader) propModel {
	skull := rl.LoadModelFromMesh(rl.GenMeshSphere(0.09, 8, 10))
	jaw := rl.LoadModelFromMesh(rl.GenMeshCube(0.10, 0.05, 0.07))
	longBone := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.025, 0.32, 8))
	knuckle := rl.LoadModelFromMesh(rl.GenMeshSphere(0.045, 6, 8))
	models := []rl.Model{skull, jaw, longBone, knuckle}
	for i := range models {
		attachShader(&models[i], shader)
	}
	bone := rl.NewColor(228, 220, 198, 255)
	stain := rl.NewColor(178, 162, 132, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Skull and detached jaw.
			{modelIdx: 0, offset: rl.NewVector3(-0.08, 0.08, -0.08), scale: rl.NewVector3(1, 0.85, 1), tint: bone},
			{modelIdx: 1, offset: rl.NewVector3(-0.04, 0.04, -0.04), scale: rl.NewVector3(1, 1, 1), tint: stain},
			// Three long bones lying flat at varied angles. rotationAxis is
			// non-vertical so the cylinder tips onto its side (cylinders are
			// vertical at scale (1,1,1)).
			{modelIdx: 2, offset: rl.NewVector3(0.10, 0.025, 0.06), scale: rl.NewVector3(1, 1, 1), rotation: 90, rotationAxis: rl.NewVector3(1, 0, 0), tint: bone},
			{modelIdx: 3, offset: rl.NewVector3(0.10, 0.045, 0.22), scale: rl.NewVector3(1, 1, 1), tint: bone},
			{modelIdx: 2, offset: rl.NewVector3(0.02, 0.025, -0.10), scale: rl.NewVector3(1, 1, 1), rotation: 70, rotationAxis: rl.NewVector3(0, 0, 1), tint: stain},
			{modelIdx: 2, offset: rl.NewVector3(-0.12, 0.025, 0.12), scale: rl.NewVector3(1, 1, 1), rotation: 110, rotationAxis: rl.NewVector3(1, 0, 1), tint: bone},
		},
	}
}

// loadScorchProp builds a flat dark disc — a burn mark on the floor. One
// thin cylinder at near-ground height with a faded inner cylinder for the
// ring effect. Almost invisible from a distance, but unmistakable when you
// walk over it; pairs well with bone piles for telling stories.
func loadScorchProp(shader rl.Shader) propModel {
	outer := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.42, 0.02, 20))
	inner := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.24, 0.02, 18))
	models := []rl.Model{outer, inner}
	for i := range models {
		attachShader(&models[i], shader)
	}
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

// loadBloodProp builds a dried-bloodstain decal: an irregular cluster of
// flat low cylinders in tarnished red. The slight tint walk between the
// three discs reads as a smear rather than a perfect circle.
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

// loadCobwebProp builds a small corner cobweb: a single thin slanted slab
// off-center on the tile, suggesting a web stretched between an unseen
// wall corner and the floor. Very low contrast — the eye finds it without
// being shouted at.
func loadCobwebProp(shader rl.Shader) propModel {
	panel := rl.LoadModelFromMesh(rl.GenMeshCube(0.42, 0.012, 0.42))
	strand := rl.LoadModelFromMesh(rl.GenMeshCube(0.34, 0.008, 0.020))
	models := []rl.Model{panel, strand}
	for i := range models {
		attachShader(&models[i], shader)
	}
	web := rl.NewColor(220, 222, 226, 200)
	wisp := rl.NewColor(196, 200, 208, 220)
	return propModel{
		models: models,
		parts: []treePart{
			// Main slanted disc — tilt around z so the silhouette reads as
			// a web caught at an angle, not a floor sticker.
			{modelIdx: 0, offset: rl.NewVector3(-0.28, 0.16, -0.28), scale: rl.NewVector3(1, 1, 1), rotation: 35, rotationAxis: rl.NewVector3(1, 0, 1), tint: web},
			// Two thinner strands radiating out.
			{modelIdx: 1, offset: rl.NewVector3(-0.10, 0.12, -0.18), scale: rl.NewVector3(1, 1, 1), rotation: 30, rotationAxis: rl.NewVector3(0, 1, 0), tint: wisp},
			{modelIdx: 1, offset: rl.NewVector3(-0.20, 0.08, -0.30), scale: rl.NewVector3(1, 1, 1), rotation: -20, rotationAxis: rl.NewVector3(0, 1, 0), tint: wisp},
		},
	}
}

// loadStumpProp builds a weathered tree stump: a short fat cylinder for
// the trunk and a slightly darker disc on top for the cut growth-ring
// face. Bark texture wraps the sides so a stump next to a tree reads as
// belonging to the same family of wood.
func loadStumpProp(shader rl.Shader, barkTex rl.Texture2D) propModel {
	body := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.34, 0.34, 14))
	face := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.32, 0.04, 14))
	models := []rl.Model{body, face}
	setModelTexture(&models[0], barkTex)
	for i := range models {
		attachShader(&models[i], shader)
	}
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

// loadLogProp builds a fallen log lying on its side. The trunk is a
// cylinder tipped 90° around X so its long axis runs along the tile's
// world Z. Two end-cap discs cover the cut faces. Mossy patches read as
// "this log has been here a while."
func loadLogProp(shader rl.Shader, barkTex, leafTex rl.Texture2D) propModel {
	trunk := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.20, 1.05, 14))
	cap := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.20, 0.02, 14))
	moss := rl.LoadModelFromMesh(rl.GenMeshSphere(0.16, 8, 10))
	models := []rl.Model{trunk, cap, moss}
	setModelTexture(&models[0], barkTex)
	setModelTexture(&models[2], leafTex)
	for i := range models {
		attachShader(&models[i], shader)
	}
	// Pastel pecan log with pale cut faces + soft mint moss.
	bark := rl.NewColor(170, 130, 94, 255)
	cut := rl.NewColor(210, 178, 138, 255)
	mossTint := rl.NewColor(156, 192, 140, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Trunk lying on its side: cylinder's native axis is +Y, so a
			// 90° rotation around +X tips it onto -Z. Then the cylinder's
			// length runs along z and its center sits ~half its radius
			// above the ground.
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

// loadDoorProp builds an area-transition door frame: two vertical
// wooden posts, a lintel across the top, and a darker rectangular
// "doorway" panel between them. The tile underneath stays walkable —
// stepping onto it triggers the transition in the explore loop. The
// frame is centered on the tile and faces along world Z by default;
// the renderer rotates it by the door's authored facing so the player
// always sees the opening from the side they approach.
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
	for i := range models {
		attachShader(&models[i], shader)
	}
	// Muted wood family — matches the bark / crate / stump palette
	// so the door sits in the same painted material world as the
	// trees around it.
	wood := rl.NewColor(118, 84, 56, 255)
	woodDark := rl.NewColor(84, 60, 42, 255)
	doorPanel := rl.NewColor(98, 70, 50, 255)
	plankDark := rl.NewColor(70, 48, 32, 255)
	brass := rl.NewColor(178, 142, 78, 255)
	brassBright := rl.NewColor(206, 168, 96, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Two posts flanking a doorway.
			{modelIdx: 0, offset: rl.NewVector3(-0.35, 0.70, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			{modelIdx: 0, offset: rl.NewVector3(0.35, 0.70, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			// Lintel across the top — slightly wider than the post spread.
			{modelIdx: 1, offset: rl.NewVector3(0, 1.44, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			// Threshold stone at the base — reads as the worn
			// step you walk over on the way through.
			{modelIdx: 8, offset: rl.NewVector3(0, 0.03, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			// Recessed plank-faced door panel between the posts.
			{modelIdx: 2, offset: rl.NewVector3(0, 0.65, 0), scale: rl.NewVector3(1, 1, 1), tint: doorPanel},
			// Horizontal plank ridges across the door panel —
			// reads as boarded construction instead of a slab.
			{modelIdx: 3, offset: rl.NewVector3(0, 0.32, 0.025), scale: rl.NewVector3(1, 1, 1), tint: plankDark},
			{modelIdx: 3, offset: rl.NewVector3(0, 0.66, 0.025), scale: rl.NewVector3(1, 1, 1), tint: plankDark},
			{modelIdx: 3, offset: rl.NewVector3(0, 1.00, 0.025), scale: rl.NewVector3(1, 1, 1), tint: plankDark},
			// Brass corner studs on the panel — four small domes
			// catching the gilt highlight.
			{modelIdx: 5, offset: rl.NewVector3(-0.24, 0.16, 0.045), scale: rl.NewVector3(1, 1, 1), tint: brassBright},
			{modelIdx: 5, offset: rl.NewVector3(0.24, 0.16, 0.045), scale: rl.NewVector3(1, 1, 1), tint: brassBright},
			{modelIdx: 5, offset: rl.NewVector3(-0.24, 1.14, 0.045), scale: rl.NewVector3(1, 1, 1), tint: brassBright},
			{modelIdx: 5, offset: rl.NewVector3(0.24, 1.14, 0.045), scale: rl.NewVector3(1, 1, 1), tint: brassBright},
			// Two iron hinges flanking the left edge of the panel.
			{modelIdx: 6, offset: rl.NewVector3(-0.30, 0.32, 0.045), scale: rl.NewVector3(1, 1, 1), tint: brass},
			{modelIdx: 6, offset: rl.NewVector3(-0.30, 0.98, 0.045), scale: rl.NewVector3(1, 1, 1), tint: brass},
			// Brass doorknob on the right side, mid-height — the
			// "this is interactive" cue.
			{modelIdx: 7, offset: rl.NewVector3(0.20, 0.62, 0.07), scale: rl.NewVector3(1, 1, 1), tint: brassBright},
			// Keystone / lintel crown — slightly forward of the
			// lintel for sculpted relief.
			{modelIdx: 4, offset: rl.NewVector3(0, 1.52, 0.02), scale: rl.NewVector3(1, 1, 1), tint: brass},
		},
	}
}

// loadCaveDoorProp builds a rough stone archway: two chunky rock jambs,
// a thick stone lintel slab, a few irregular boulders stacked at the
// jamb feet, and a dark recessed opening — reads as a mouth hewn / fallen
// into the rock rather than a built timber frame. Same footprint and
// facing convention as loadDoorProp so it drops into the door table.
func loadCaveDoorProp(shader rl.Shader, stoneTex rl.Texture2D) propModel {
	jamb := rl.LoadModelFromMesh(rl.GenMeshCube(0.22, 1.45, 0.22))
	lintel := rl.LoadModelFromMesh(rl.GenMeshCube(0.96, 0.26, 0.26))
	opening := rl.LoadModelFromMesh(rl.GenMeshCube(0.56, 1.20, 0.06))
	boulder := rl.LoadModelFromMesh(rl.GenMeshSphere(0.18, 8, 8))
	threshold := rl.LoadModelFromMesh(rl.GenMeshCube(0.86, 0.08, 0.24))
	models := []rl.Model{jamb, lintel, opening, boulder, threshold}
	// Stone-textured parts: jambs, lintel, boulders, threshold (not the
	// dark opening, which is a flat unlit-looking void).
	for _, i := range []int{0, 1, 3, 4} {
		setModelTexture(&models[i], stoneTex)
	}
	for i := range models {
		attachShader(&models[i], shader)
	}
	stone := rl.NewColor(150, 146, 138, 255)
	stoneDark := rl.NewColor(112, 108, 102, 255)
	stoneShade := rl.NewColor(96, 92, 88, 255)
	mouth := rl.NewColor(28, 26, 30, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Two rough jambs, splayed slightly out at the top by tint
			// banding (the boulders below sell the lean visually).
			{modelIdx: 0, offset: rl.NewVector3(-0.37, 0.72, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			{modelIdx: 0, offset: rl.NewVector3(0.37, 0.72, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			// Heavy lintel slab bridging the jambs.
			{modelIdx: 1, offset: rl.NewVector3(0, 1.46, 0), scale: rl.NewVector3(1, 1, 1), tint: stoneDark},
			// Worn stone threshold underfoot.
			{modelIdx: 4, offset: rl.NewVector3(0, 0.04, 0), scale: rl.NewVector3(1, 1, 1), tint: stoneShade},
			// Dark recessed opening between the jambs.
			{modelIdx: 2, offset: rl.NewVector3(0, 0.64, 0), scale: rl.NewVector3(1, 1, 1), tint: mouth},
			// Tumbled boulders clustered at the jamb feet — breaks the
			// clean rectangle so it reads as natural rock.
			{modelIdx: 3, offset: rl.NewVector3(-0.42, 0.16, 0.10), scale: rl.NewVector3(1, 0.9, 1), tint: stoneDark},
			{modelIdx: 3, offset: rl.NewVector3(-0.30, 0.12, -0.12), scale: rl.NewVector3(0.7, 0.7, 0.7), tint: stoneShade},
			{modelIdx: 3, offset: rl.NewVector3(0.44, 0.18, -0.08), scale: rl.NewVector3(1.1, 0.95, 1.1), tint: stoneDark},
			{modelIdx: 3, offset: rl.NewVector3(0.32, 0.12, 0.12), scale: rl.NewVector3(0.65, 0.65, 0.65), tint: stoneShade},
			// A capstone boulder perched on the lintel.
			{modelIdx: 3, offset: rl.NewVector3(0.08, 1.66, 0), scale: rl.NewVector3(1.2, 0.8, 1.0), tint: stone},
		},
	}
}

// loadFieldDoorProp builds an open trail gateway: two slender leaning
// posts, a light crossbeam, no solid door panel (you can see "through"
// it), with a small hanging sign plank and a couple of grass tufts at
// the base — reads as a way-marker between open areas rather than a
// sealed door. Same footprint + facing convention as loadDoorProp.
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
	for i := range models {
		attachShader(&models[i], shader)
	}
	wood := rl.NewColor(150, 112, 72, 255)
	woodDark := rl.NewColor(112, 82, 52, 255)
	signWood := rl.NewColor(168, 132, 90, 255)
	capTint := rl.NewColor(96, 70, 46, 255)
	grass := rl.NewColor(120, 168, 92, 255)
	grassDark := rl.NewColor(96, 142, 78, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Two leaning posts (slight outward splay via offset).
			{modelIdx: 0, offset: rl.NewVector3(-0.40, 0.65, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			{modelIdx: 0, offset: rl.NewVector3(0.40, 0.65, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			// Crossbeam across the top — the open lintel.
			{modelIdx: 1, offset: rl.NewVector3(0, 1.28, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			// Short diagonal-feel braces under each beam end.
			{modelIdx: 2, offset: rl.NewVector3(-0.26, 1.16, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			{modelIdx: 2, offset: rl.NewVector3(0.26, 1.16, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			// Post caps.
			{modelIdx: 4, offset: rl.NewVector3(-0.40, 1.32, 0), scale: rl.NewVector3(1, 1, 1), tint: capTint},
			{modelIdx: 4, offset: rl.NewVector3(0.40, 1.32, 0), scale: rl.NewVector3(1, 1, 1), tint: capTint},
			// Hanging trail sign under the beam center.
			{modelIdx: 3, offset: rl.NewVector3(0, 1.04, 0.02), scale: rl.NewVector3(1, 1, 1), tint: signWood},
			// Grass tufts at the post feet so it sits in the meadow.
			{modelIdx: 5, offset: rl.NewVector3(-0.44, 0.09, 0.06), scale: rl.NewVector3(1, 1, 1), tint: grass},
			{modelIdx: 5, offset: rl.NewVector3(0.46, 0.09, -0.05), scale: rl.NewVector3(0.8, 0.8, 0.8), tint: grassDark},
		},
	}
}

// loadLilypadProp builds a flat floating lilypad: a thin wide disc for
// the pad, a smaller offset disc to suggest a partner leaf, and a tiny
// pink bloom at the center. Pure decor — sits just above floor level so
// it reads as floating on water without z-fighting the floor tile.
func loadLilypadProp(shader rl.Shader) propModel {
	pad := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.42, 0.015, 16))
	smallPad := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.22, 0.015, 14))
	bloom := rl.LoadModelFromMesh(rl.GenMeshSphere(0.06, 8, 8))
	bud := rl.LoadModelFromMesh(rl.GenMeshSphere(0.035, 6, 8))
	models := []rl.Model{pad, smallPad, bloom, bud}
	for i := range models {
		attachShader(&models[i], shader)
	}
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

// loadLeafPileProp builds a low pile of fallen leaves: a flat fat
// cylinder for the main heap with two smaller domes sitting on top for
// volume. The leaf texture from the tree model carries the leaf vein
// noise, so a leaf pile and a tree canopy share the same surface family.
func loadLeafPileProp(shader rl.Shader, leafTex rl.Texture2D) propModel {
	base := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.48, 0.10, 16))
	mound := rl.LoadModelFromMesh(rl.GenMeshSphere(0.22, 10, 12))
	models := []rl.Model{base, mound}
	for i := range models {
		setModelTexture(&models[i], leafTex)
		attachShader(&models[i], shader)
	}
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

// ---------------------------------------------------------------------------
// Outdoor / field tileset additions (Turn B).
// All single-tile blockers. Each builder allocates its own model handles and
// the propModel.unload() call in Resources.Unload frees them.
// ---------------------------------------------------------------------------

// loadWellProp builds a stone-ringed well: a fat short cylinder rim
// with a dark water disc inset and a small pole/winch above. Reads as
// "village well" silhouette from any angle. Uses the rock texture for
// the ring so it blends with stone walls.
func loadWellProp(shader rl.Shader, rockTex rl.Texture2D) propModel {
	rim := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.42, 0.40, 18))
	water := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.34, 0.04, 16))
	post := rl.LoadModelFromMesh(rl.GenMeshCube(0.06, 0.80, 0.06))
	beam := rl.LoadModelFromMesh(rl.GenMeshCube(0.78, 0.06, 0.06))
	bucket := rl.LoadModelFromMesh(rl.GenMeshCube(0.16, 0.16, 0.16))
	models := []rl.Model{rim, water, post, beam, bucket}
	setModelTexture(&models[0], rockTex)
	for i := range models {
		attachShader(&models[i], shader)
	}
	stone := rl.NewColor(170, 168, 156, 255)
	stoneDark := rl.NewColor(110, 108, 100, 255)
	waterCol := rl.NewColor(56, 96, 138, 255)
	wood := rl.NewColor(110, 78, 50, 255)
	woodDark := rl.NewColor(74, 52, 34, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.20, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.36, 0), scale: rl.NewVector3(1, 1, 1), tint: waterCol},
			// Posts on each side hold up the winch beam.
			{modelIdx: 2, offset: rl.NewVector3(-0.30, 0.80, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			{modelIdx: 2, offset: rl.NewVector3(0.30, 0.80, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			{modelIdx: 3, offset: rl.NewVector3(0, 1.20, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			// Bucket dangling on a rope (no rope geom — bucket alone).
			{modelIdx: 4, offset: rl.NewVector3(0.18, 0.95, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			// Rim shading accent — dark base ring near the floor.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.04, 0), scale: rl.NewVector3(1.02, 0.18, 1.02), tint: stoneDark},
		},
	}
}

// loadGravestoneProp builds a weathered tombstone: a thick flat slab
// tilted slightly forward with a rounded top, plus a smaller mound at
// the base to read as the grave proper. Cool grey palette so it
// stands apart from the warmer well rim.
func loadGravestoneProp(shader rl.Shader, rockTex rl.Texture2D) propModel {
	slab := rl.LoadModelFromMesh(rl.GenMeshCube(0.50, 0.85, 0.12))
	cap := rl.LoadModelFromMesh(rl.GenMeshSphere(0.26, 8, 10))
	mound := rl.LoadModelFromMesh(rl.GenMeshSphere(0.36, 8, 10))
	models := []rl.Model{slab, cap, mound}
	for i := range models {
		setModelTexture(&models[i], rockTex)
		attachShader(&models[i], shader)
	}
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

// loadSignPostProp builds a wooden sign: a tall thin post with a
// horizontal plank near the top, slightly off-axis so it reads from
// any angle.
func loadSignPostProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	post := rl.LoadModelFromMesh(rl.GenMeshCube(0.08, 1.10, 0.08))
	board := rl.LoadModelFromMesh(rl.GenMeshCube(0.66, 0.34, 0.06))
	models := []rl.Model{post, board}
	for i := range models {
		setModelTexture(&models[i], woodTex)
		attachShader(&models[i], shader)
	}
	wood := rl.NewColor(150, 102, 60, 255)
	woodDark := rl.NewColor(96, 64, 40, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.55, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			{modelIdx: 1, offset: rl.NewVector3(0.18, 0.95, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			// Lighter front-face on the board so it reads as carved.
			{modelIdx: 1, offset: rl.NewVector3(0.18, 0.95, 0.04), scale: rl.NewVector3(0.92, 0.85, 0.4), tint: wood},
		},
	}
}

// loadHayBaleProp builds a fat short cylinder of bound straw lying on
// its side. Warm yellow tones with subtle darker bands for the binding.
func loadHayBaleProp(shader rl.Shader) propModel {
	bale := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.45, 0.70, 14))
	band := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.47, 0.04, 14))
	models := []rl.Model{bale, band}
	for i := range models {
		attachShader(&models[i], shader)
	}
	straw := rl.NewColor(216, 184, 110, 255)
	strawDark := rl.NewColor(168, 132, 76, 255)
	cord := rl.NewColor(118, 86, 52, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Lay the cylinder on its side: rotate 90° around X so its
			// length runs along world Z.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.45, 0), scale: rl.NewVector3(1, 1, 1), rotation: 90, rotationAxis: rl.NewVector3(1, 0, 0), tint: straw},
			// Darker shading hint underneath.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.45, 0), scale: rl.NewVector3(0.92, 0.98, 0.92), rotation: 90, rotationAxis: rl.NewVector3(1, 0, 0), tint: strawDark},
			// Two binding rings around the bale.
			{modelIdx: 1, offset: rl.NewVector3(0, 0.45, -0.22), scale: rl.NewVector3(1, 1, 1), rotation: 90, rotationAxis: rl.NewVector3(1, 0, 0), tint: cord},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.45, 0.22), scale: rl.NewVector3(1, 1, 1), rotation: 90, rotationAxis: rl.NewVector3(1, 0, 0), tint: cord},
		},
	}
}

// loadScarecrowProp builds a cross-frame scarecrow: a vertical pole, a
// horizontal arm beam, a sackcloth head sphere, and an angular torso
// blob suggesting layered shirt/straw stuffing.
func loadScarecrowProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	pole := rl.LoadModelFromMesh(rl.GenMeshCube(0.08, 1.55, 0.08))
	arm := rl.LoadModelFromMesh(rl.GenMeshCube(0.90, 0.07, 0.07))
	head := rl.LoadModelFromMesh(rl.GenMeshSphere(0.16, 8, 10))
	torso := rl.LoadModelFromMesh(rl.GenMeshCube(0.40, 0.50, 0.20))
	hat := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.20, 0.16, 12))
	models := []rl.Model{pole, arm, head, torso, hat}
	for i := range models {
		attachShader(&models[i], shader)
	}
	setModelTexture(&models[0], woodTex)
	setModelTexture(&models[1], woodTex)
	wood := rl.NewColor(110, 78, 50, 255)
	sack := rl.NewColor(196, 162, 96, 255)
	sackDark := rl.NewColor(140, 110, 64, 255)
	hatCol := rl.NewColor(72, 52, 32, 255)
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

// ---------------------------------------------------------------------------
// Dungeon-interior tileset additions (Turn B).
// ---------------------------------------------------------------------------

// loadBookshelfProp builds a tall thin shelf with three book-row bands.
// Stone-grey backing with multicolored book bands so each shelf reads
// distinctly.
func loadBookshelfProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	frame := rl.LoadModelFromMesh(rl.GenMeshCube(0.78, 1.50, 0.30))
	shelf := rl.LoadModelFromMesh(rl.GenMeshCube(0.82, 0.04, 0.34))
	books := rl.LoadModelFromMesh(rl.GenMeshCube(0.66, 0.32, 0.20))
	models := []rl.Model{frame, shelf, books}
	for i := range models {
		setModelTexture(&models[i], woodTex)
		attachShader(&models[i], shader)
	}
	wood := rl.NewColor(112, 78, 48, 255)
	woodDark := rl.NewColor(72, 52, 32, 255)
	bookRed := rl.NewColor(160, 64, 60, 255)
	bookBlue := rl.NewColor(64, 96, 156, 255)
	bookGreen := rl.NewColor(96, 132, 80, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.75, 0), scale: rl.NewVector3(1, 1, 1), tint: wood},
			// Three shelves with book rows perched on top of each.
			{modelIdx: 1, offset: rl.NewVector3(0, 0.30, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.48, 0.04), scale: rl.NewVector3(1, 1, 1), tint: bookRed},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.74, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			{modelIdx: 2, offset: rl.NewVector3(0, 0.92, 0.04), scale: rl.NewVector3(1, 1, 1), tint: bookBlue},
			{modelIdx: 1, offset: rl.NewVector3(0, 1.18, 0), scale: rl.NewVector3(1, 1, 1), tint: woodDark},
			{modelIdx: 2, offset: rl.NewVector3(0, 1.36, 0.04), scale: rl.NewVector3(1, 1, 1), tint: bookGreen},
		},
	}
}

// loadTableProp builds a wooden table: a flat rectangular top on four
// shorter legs. Sized so the player reads it as "you could rest a mug
// on this."
func loadTableProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	top := rl.LoadModelFromMesh(rl.GenMeshCube(0.90, 0.10, 0.60))
	leg := rl.LoadModelFromMesh(rl.GenMeshCube(0.10, 0.60, 0.10))
	models := []rl.Model{top, leg}
	for i := range models {
		setModelTexture(&models[i], woodTex)
		attachShader(&models[i], shader)
	}
	wood := rl.NewColor(160, 116, 72, 255)
	woodDark := rl.NewColor(110, 78, 50, 255)
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

// loadBedProp builds a wood-frame bed with a pillow and bedding.
// Single-tile but visibly bedlike from the side.
func loadBedProp(shader rl.Shader, woodTex rl.Texture2D) propModel {
	frame := rl.LoadModelFromMesh(rl.GenMeshCube(0.84, 0.20, 0.50))
	mattress := rl.LoadModelFromMesh(rl.GenMeshCube(0.78, 0.14, 0.46))
	headboard := rl.LoadModelFromMesh(rl.GenMeshCube(0.84, 0.42, 0.06))
	pillow := rl.LoadModelFromMesh(rl.GenMeshCube(0.30, 0.08, 0.36))
	models := []rl.Model{frame, mattress, headboard, pillow}
	setModelTexture(&models[0], woodTex)
	setModelTexture(&models[2], woodTex)
	for i := range models {
		attachShader(&models[i], shader)
	}
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

// loadBrazierProp builds a metal brazier on a tripod with a flame.
// Three legs, an open bowl, and a flickery flame sphere/cone on top.
func loadBrazierProp(shader rl.Shader) propModel {
	leg := rl.LoadModelFromMesh(rl.GenMeshCube(0.06, 0.60, 0.06))
	bowl := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.28, 0.18, 14))
	flame := rl.LoadModelFromMesh(rl.GenMeshSphere(0.18, 8, 10))
	tip := rl.LoadModelFromMesh(rl.GenMeshSphere(0.10, 6, 8))
	models := []rl.Model{leg, bowl, flame, tip}
	// Iron stand (legs + bowl) is lit by the world shader. The
	// flame + tip are LEFT on raylib's default material shader so
	// they render emissive — full fire colour, unaffected by the
	// near-black dungeon ambient — and read as the glowing source
	// the torch point light pours out of. (Same default-shader
	// trick the ground-shadow disc uses to stay unlit.)
	attachShader(&models[0], shader)
	attachShader(&models[1], shader)
	iron := rl.NewColor(60, 56, 52, 255)
	ironLight := rl.NewColor(98, 92, 84, 255)
	fire := rl.NewColor(248, 150, 64, 255)
	fireBright := rl.NewColor(255, 230, 150, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Three legs splayed out.
			{modelIdx: 0, offset: rl.NewVector3(-0.18, 0.30, -0.10), scale: rl.NewVector3(1, 1, 1), rotation: 15, rotationAxis: rl.NewVector3(0, 0, 1), tint: iron},
			{modelIdx: 0, offset: rl.NewVector3(0.18, 0.30, -0.10), scale: rl.NewVector3(1, 1, 1), rotation: -15, rotationAxis: rl.NewVector3(0, 0, 1), tint: iron},
			{modelIdx: 0, offset: rl.NewVector3(0, 0.30, 0.20), scale: rl.NewVector3(1, 1, 1), rotation: 15, rotationAxis: rl.NewVector3(1, 0, 0), tint: iron},
			// Bowl rim.
			{modelIdx: 1, offset: rl.NewVector3(0, 0.66, 0), scale: rl.NewVector3(1, 1, 1), tint: iron},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.72, 0), scale: rl.NewVector3(0.94, 0.2, 0.94), tint: ironLight},
			// Flame stack — broad base, smaller bright tip.
			{modelIdx: 2, offset: rl.NewVector3(0, 0.90, 0), scale: rl.NewVector3(1, 1.4, 1), tint: fire},
			{modelIdx: 3, offset: rl.NewVector3(0, 1.10, 0), scale: rl.NewVector3(1, 1.6, 1), tint: fireBright},
		},
	}
}

// loadSarcophagusProp builds a stone sarcophagus: a heavy rectangular
// base with a lid sitting flush on top. Authored as a single-tile prop
// (cramped for a real burial chamber, but reads at this scale).
func loadSarcophagusProp(shader rl.Shader, rockTex rl.Texture2D) propModel {
	base := rl.LoadModelFromMesh(rl.GenMeshCube(0.92, 0.46, 0.50))
	lid := rl.LoadModelFromMesh(rl.GenMeshCube(0.96, 0.10, 0.54))
	carving := rl.LoadModelFromMesh(rl.GenMeshCube(0.30, 0.36, 0.04))
	models := []rl.Model{base, lid, carving}
	for i := range models {
		setModelTexture(&models[i], rockTex)
		attachShader(&models[i], shader)
	}
	stone := rl.NewColor(200, 192, 174, 255)
	stoneDark := rl.NewColor(140, 132, 116, 255)
	carved := rl.NewColor(108, 96, 84, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.23, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			{modelIdx: 0, offset: rl.NewVector3(0, 0.23, 0), scale: rl.NewVector3(0.96, 1.04, 0.96), tint: stoneDark},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.51, 0), scale: rl.NewVector3(1, 1, 1), tint: stone},
			// Faux carving on the lid (humanoid silhouette suggestion).
			{modelIdx: 2, offset: rl.NewVector3(0, 0.40, 0.28), scale: rl.NewVector3(1, 1, 1), tint: carved},
		},
	}
}

// ---------------------------------------------------------------------------
// Decor additions (Turn B). All single-tile, non-blocking. Each builder
// lives in decorModels and is dispatched by drawDecor's char switch.
// ---------------------------------------------------------------------------

// loadRugProp builds a flat woven rug: a thin wide cube laid flush
// on the floor with a tasseled border. Pure decor — never blocks.
func loadRugProp(shader rl.Shader) propModel {
	pad := rl.LoadModelFromMesh(rl.GenMeshCube(0.78, 0.02, 0.58))
	border := rl.LoadModelFromMesh(rl.GenMeshCube(0.84, 0.025, 0.64))
	models := []rl.Model{pad, border}
	for i := range models {
		attachShader(&models[i], shader)
	}
	rug := rl.NewColor(176, 84, 68, 255)
	rugDark := rl.NewColor(120, 56, 48, 255)
	trim := rl.NewColor(232, 196, 124, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 1, offset: rl.NewVector3(0, 0.012, 0), scale: rl.NewVector3(1, 1, 1), tint: trim},
			{modelIdx: 0, offset: rl.NewVector3(0, 0.020, 0), scale: rl.NewVector3(1, 1, 1), tint: rug},
			// Inner darker stripe so the rug isn't a flat slab.
			{modelIdx: 0, offset: rl.NewVector3(0, 0.024, 0), scale: rl.NewVector3(0.6, 0.5, 0.6), tint: rugDark},
		},
	}
}

// loadCandleProp builds a stubby candle with a small flame tip,
// sitting in a tiny pool of melted wax. Reads from any angle.
func loadCandleProp(shader rl.Shader) propModel {
	puddle := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.10, 0.02, 10))
	candle := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.05, 0.16, 8))
	flame := rl.LoadModelFromMesh(rl.GenMeshSphere(0.04, 6, 8))
	tip := rl.LoadModelFromMesh(rl.GenMeshSphere(0.02, 6, 6))
	models := []rl.Model{puddle, candle, flame, tip}
	for i := range models {
		attachShader(&models[i], shader)
	}
	wax := rl.NewColor(244, 220, 156, 255)
	waxDark := rl.NewColor(196, 168, 108, 255)
	fire := rl.NewColor(232, 144, 64, 255)
	fireBright := rl.NewColor(252, 230, 148, 255)
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

// loadBootprintsProp builds two small flat impressions on the floor.
// Each "print" is a tiny shallow cube; pair them in a forward-stride
// layout to suggest someone walked past.
func loadBootprintsProp(shader rl.Shader) propModel {
	print := rl.LoadModelFromMesh(rl.GenMeshCube(0.10, 0.015, 0.18))
	heel := rl.LoadModelFromMesh(rl.GenMeshCube(0.10, 0.015, 0.06))
	models := []rl.Model{print, heel}
	for i := range models {
		attachShader(&models[i], shader)
	}
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

// loadAshHeapProp builds a small cool-grey ash mound. Distinct from
// DecorScorch (a flat black ring) — the heap has volume so it reads as
// "campfire site remnant" rather than "burn mark."
func loadAshHeapProp(shader rl.Shader) propModel {
	heap := rl.LoadModelFromMesh(rl.GenMeshSphere(0.16, 8, 8))
	dust := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.22, 0.02, 12))
	models := []rl.Model{heap, dust}
	for i := range models {
		attachShader(&models[i], shader)
	}
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

// loadPuddleProp builds a shallow water puddle: an irregular flat
// cylinder with a brighter rim suggesting wet stone reflection.
func loadPuddleProp(shader rl.Shader) propModel {
	disc := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.26, 0.015, 14))
	highlight := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.18, 0.02, 12))
	models := []rl.Model{disc, highlight}
	for i := range models {
		attachShader(&models[i], shader)
	}
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

// loadRootClusterProp builds gnarled roots poking from the floor: a
// few low arches of brown cylinders at varied tilts. Reads as
// "something grew through the floor here" without blocking.
func loadRootClusterProp(shader rl.Shader, barkTex rl.Texture2D) propModel {
	arch := rl.LoadModelFromMesh(rl.GenMeshCylinder(0.04, 0.30, 8))
	knob := rl.LoadModelFromMesh(rl.GenMeshSphere(0.05, 6, 8))
	models := []rl.Model{arch, knob}
	for i := range models {
		setModelTexture(&models[i], barkTex)
		attachShader(&models[i], shader)
	}
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
