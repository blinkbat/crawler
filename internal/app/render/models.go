package render

import (
	"image/color"
	"math"

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
	treeMeshTrunk = iota
	treeMeshCanopyLow
	treeMeshCanopyHigh
	treeMeshCanopySide
	treeMeshCanopyAccent
)

func loadTreeModel(shader rl.Shader, barkTex, leafTex rl.Texture2D) treeModel {
	models := []rl.Model{
		treeMeshTrunk:        rl.LoadModelFromMesh(rl.GenMeshCylinder(0.18, 1.55, 12)),
		treeMeshCanopyLow:    rl.LoadModelFromMesh(rl.GenMeshSphere(0.92, 12, 16)),
		treeMeshCanopyHigh:   rl.LoadModelFromMesh(rl.GenMeshSphere(0.78, 12, 16)),
		treeMeshCanopySide:   rl.LoadModelFromMesh(rl.GenMeshSphere(0.55, 10, 14)),
		treeMeshCanopyAccent: rl.LoadModelFromMesh(rl.GenMeshSphere(0.38, 10, 12)),
	}
	for i := range models {
		tex := leafTex
		if i == treeMeshTrunk {
			tex = barkTex
		}
		setModelTexture(&models[i], tex)
		attachShader(&models[i], shader)
	}

	leafBase := color.RGBA{R: 196, G: 226, B: 198, A: 255}
	leafMid := color.RGBA{R: 168, G: 212, B: 168, A: 255}
	leafDeep := color.RGBA{R: 132, G: 182, B: 138, A: 255}
	leafGold := color.RGBA{R: 222, G: 232, B: 174, A: 255}

	return treeModel{
		models: models,
		parts: []treePart{
			{modelIdx: treeMeshTrunk, offset: rl.NewVector3(0, 0.06, 0), scale: rl.NewVector3(1, 1, 1), tint: rl.White},
			{modelIdx: treeMeshCanopyLow, offset: rl.NewVector3(0, 1.55, 0), scale: rl.NewVector3(1, 0.95, 1), tint: leafMid},
			{modelIdx: treeMeshCanopyHigh, offset: rl.NewVector3(-0.05, 2.05, 0.05), scale: rl.NewVector3(1, 1, 1), tint: leafBase},
			{modelIdx: treeMeshCanopySide, offset: rl.NewVector3(0.42, 1.78, 0.16), scale: rl.NewVector3(1, 1, 1), tint: leafDeep},
			{modelIdx: treeMeshCanopySide, offset: rl.NewVector3(-0.38, 1.62, -0.14), scale: rl.NewVector3(1, 1, 1), tint: leafMid},
			{modelIdx: treeMeshCanopyAccent, offset: rl.NewVector3(0.22, 2.32, -0.18), scale: rl.NewVector3(1, 1, 1), tint: leafGold},
			{modelIdx: treeMeshCanopyAccent, offset: rl.NewVector3(-0.18, 2.18, 0.22), scale: rl.NewVector3(1, 1, 1), tint: leafBase},
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
func (p propModel) draw(center rl.Vector3, scale, yaw float32) {
	if scale <= 0 {
		scale = 1
	}
	for _, part := range p.parts {
		offset := rotateOffsetY(part.offset, scale, yaw)
		position := rl.NewVector3(center.X+offset.X, center.Y+offset.Y, center.Z+offset.Z)
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

// loadRockProp builds a chunky polygonal boulder: a flat base with two or
// three faceted lumps fused on top at varied angles, all in close-grouped
// stone greys. The intent is "weathered rock outcrop you'd see in a
// fantasy field map" — low silhouette, jagged facets, no upward spires.
// Slice/ring counts are kept low (4–6) so the lumps look polygonal rather
// than billiard-ball smooth.
func loadRockProp(shader rl.Shader, rockTex rl.Texture2D) propModel {
	models := []rl.Model{
		rockMeshBase: rl.LoadModelFromMesh(rl.GenMeshCube(1.10, 0.36, 0.95)),
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

// loadBushProp builds a leaf-cluster bush (no trunk) from a few spheres.
// Same leaf texture as the trees so the foliage palette stays consistent.
// Scale 1.0 = "large" (blocks); ~0.5 reads as "small".
func loadBushProp(shader rl.Shader, leafTex rl.Texture2D) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.55, 10, 14)),
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.40, 10, 12)),
	}
	for i := range models {
		setModelTexture(&models[i], leafTex)
		attachShader(&models[i], shader)
	}
	leafBase := color.RGBA{R: 168, G: 212, B: 168, A: 255}
	leafDeep := color.RGBA{R: 122, G: 178, B: 130, A: 255}
	leafGold := color.RGBA{R: 218, G: 232, B: 170, A: 255}
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.45, 0), scale: rl.NewVector3(1, 0.92, 1), tint: leafBase},
			{modelIdx: 1, offset: rl.NewVector3(0.30, 0.62, 0.18), scale: rl.NewVector3(1, 1, 1), tint: leafDeep},
			{modelIdx: 1, offset: rl.NewVector3(-0.28, 0.58, -0.16), scale: rl.NewVector3(1, 1, 1), tint: leafGold},
		},
	}
}

// loadMushroomProp builds a tiny mushroom (cylinder stem + sphere cap). Each
// part relies on raylib's default white material texture and is colored by
// its tint, so we don't need to author a mushroom-specific texture. Scale 1.0
// reads as "small mushroom"; ~0.6 reads as "tiny mushroom".
func loadMushroomProp(shader rl.Shader) propModel {
	models := []rl.Model{
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.05, 0.16, 8)),
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.13, 8, 10)),
	}
	for i := range models {
		attachShader(&models[i], shader)
	}
	stem := color.RGBA{R: 240, G: 230, B: 210, A: 255}
	cap := color.RGBA{R: 188, G: 56, B: 56, A: 255}
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.02, 0), scale: rl.NewVector3(1, 1, 1), tint: stem},
			{modelIdx: 1, offset: rl.NewVector3(0, 0.18, 0), scale: rl.NewVector3(1, 0.78, 1), tint: cap},
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
	wood := rl.NewColor(178, 130, 78, 255)
	band := rl.NewColor(82, 56, 32, 255)
	corner := rl.NewColor(64, 44, 28, 255)
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
	wood := rl.NewColor(166, 116, 70, 255)
	hoop := rl.NewColor(58, 42, 28, 255)
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
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.36, 10, 14)),       // belly
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.18, 0.20, 16)),   // neck
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.23, 0.05, 18)),   // rim flare
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.20, 0.06, 14)),   // foot
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
		rl.LoadModelFromMesh(rl.GenMeshCube(0.72, 0.18, 0.72)),      // plinth
		rl.LoadModelFromMesh(rl.GenMeshCube(0.62, 0.10, 0.62)),      // base
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.26, 2.05, 18)),    // shaft
		rl.LoadModelFromMesh(rl.GenMeshCube(0.62, 0.16, 0.62)),      // echinus
		rl.LoadModelFromMesh(rl.GenMeshCube(0.74, 0.08, 0.74)),      // abacus
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
		rl.LoadModelFromMesh(rl.GenMeshCube(0.92, 0.24, 0.92)),       // pedestal
		rl.LoadModelFromMesh(rl.GenMeshCube(0.55, 0.14, 0.55)),       // statue base
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.20, 0.55, 14)),     // legs
		rl.LoadModelFromMesh(rl.GenMeshCube(0.48, 0.62, 0.30)),       // torso
		rl.LoadModelFromMesh(rl.GenMeshCube(0.66, 0.14, 0.34)),       // shoulders
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.18, 10, 12)),         // head
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
		rl.LoadModelFromMesh(rl.GenMeshCube(0.88, 0.14, 0.88)),       // base step
		rl.LoadModelFromMesh(rl.GenMeshCube(0.56, 2.20, 0.56)),       // shaft
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.40, 4, 6)),           // pyramid cap (low-slice)
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.08, 6, 6)),           // apex
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
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.78, 0.42, 24)),     // outer basin
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.66, 0.06, 22)),     // water disc
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.12, 0.45, 12)),     // central spout
		rl.LoadModelFromMesh(rl.GenMeshSphere(0.18, 10, 12)),         // splash bowl
		rl.LoadModelFromMesh(rl.GenMeshCylinder(0.82, 0.10, 24)),     // rim coping
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
	light := rl.NewColor(170, 212, 116, 255)
	mid := rl.NewColor(122, 184, 96, 255)
	deep := rl.NewColor(86, 142, 78, 255)
	gold := rl.NewColor(220, 220, 132, 255)
	return propModel{
		models: models,
		parts: []treePart{
			{modelIdx: 0, offset: rl.NewVector3(0, 0.17, 0), scale: rl.NewVector3(1, 1, 1), rotation: 6, rotationAxis: rl.NewVector3(0, 0, 1), tint: light},
			{modelIdx: 0, offset: rl.NewVector3(0.14, 0.16, 0.08), scale: rl.NewVector3(1, 0.92, 1), rotation: -10, rotationAxis: rl.NewVector3(1, 0, 0), tint: mid},
			{modelIdx: 0, offset: rl.NewVector3(-0.12, 0.15, 0.10), scale: rl.NewVector3(1, 0.86, 1), rotation: 14, rotationAxis: rl.NewVector3(1, 0, 1), tint: deep},
			{modelIdx: 0, offset: rl.NewVector3(0.08, 0.18, -0.14), scale: rl.NewVector3(1, 1.05, 1), rotation: -16, rotationAxis: rl.NewVector3(0, 0, 1), tint: mid},
			{modelIdx: 0, offset: rl.NewVector3(-0.10, 0.16, -0.06), scale: rl.NewVector3(1, 0.95, 1), rotation: 22, rotationAxis: rl.NewVector3(1, 0, 0), tint: gold},
		},
	}
}

// loadFlowerProp builds a small mixed-flower clump: four thin stems with
// small colored sphere blooms. The bloom colors stay in a tight warm
// palette (yellow, pink, white, lilac) so the patch reads as wildflowers
// rather than a costume jewelry display.
func loadFlowerProp(shader rl.Shader) propModel {
	stem := rl.LoadModelFromMesh(rl.GenMeshCube(0.022, 0.20, 0.022))
	bloom := rl.LoadModelFromMesh(rl.GenMeshSphere(0.055, 8, 10))
	leaf := rl.LoadModelFromMesh(rl.GenMeshCube(0.05, 0.02, 0.05))
	models := []rl.Model{stem, bloom, leaf}
	for i := range models {
		attachShader(&models[i], shader)
	}
	stemTint := rl.NewColor(80, 138, 78, 255)
	leafTint := rl.NewColor(96, 152, 86, 255)
	yellow := rl.NewColor(244, 222, 110, 255)
	pink := rl.NewColor(236, 142, 168, 255)
	white := rl.NewColor(250, 248, 240, 255)
	lilac := rl.NewColor(198, 156, 220, 255)
	return propModel{
		models: models,
		parts: []treePart{
			// Four stems with their blooms at the top. Stem and bloom share
			// the same xz offset so they line up.
			{modelIdx: 0, offset: rl.NewVector3(0.10, 0.10, 0.06), scale: rl.NewVector3(1, 1, 1), tint: stemTint},
			{modelIdx: 1, offset: rl.NewVector3(0.10, 0.22, 0.06), scale: rl.NewVector3(1, 1, 1), tint: yellow},
			{modelIdx: 0, offset: rl.NewVector3(-0.08, 0.10, 0.12), scale: rl.NewVector3(1, 1.05, 1), tint: stemTint},
			{modelIdx: 1, offset: rl.NewVector3(-0.08, 0.23, 0.12), scale: rl.NewVector3(1, 1, 1), tint: pink},
			{modelIdx: 0, offset: rl.NewVector3(0.04, 0.10, -0.14), scale: rl.NewVector3(1, 0.95, 1), tint: stemTint},
			{modelIdx: 1, offset: rl.NewVector3(0.04, 0.22, -0.14), scale: rl.NewVector3(1, 1, 1), tint: white},
			{modelIdx: 0, offset: rl.NewVector3(-0.14, 0.10, -0.04), scale: rl.NewVector3(1, 1, 1), tint: stemTint},
			{modelIdx: 1, offset: rl.NewVector3(-0.14, 0.22, -0.04), scale: rl.NewVector3(1, 1, 1), tint: lilac},
			// Two ground leaves to anchor the clump visually.
			{modelIdx: 2, offset: rl.NewVector3(0.02, 0.01, 0.01), scale: rl.NewVector3(1.2, 1, 1.2), tint: leafTint},
			{modelIdx: 2, offset: rl.NewVector3(-0.06, 0.01, -0.08), scale: rl.NewVector3(1.0, 1, 1.4), tint: leafTint},
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
	bark := rl.NewColor(132, 92, 56, 255)
	rings := rl.NewColor(184, 142, 92, 255)
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
	bark := rl.NewColor(118, 84, 52, 255)
	cut := rl.NewColor(168, 130, 84, 255)
	mossTint := rl.NewColor(118, 162, 108, 255)
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

