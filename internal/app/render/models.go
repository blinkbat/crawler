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

// drawXYZ renders the prop with a non-uniform scale so callers can squash a
// model's vertical extent independently from its footprint. Used by
// scattered small-rock decorations to produce low, walkable-looking pebbles
// from the same boulder mesh set.
func (p propModel) drawXYZ(center rl.Vector3, scale rl.Vector3, yaw float32) {
	for _, part := range p.parts {
		offset := rl.NewVector3(part.offset.X*scale.X, part.offset.Y*scale.Y, part.offset.Z*scale.Z)
		if yaw != 0 {
			offset = rl.Vector3RotateByAxisAngle(offset, rl.NewVector3(0, 1, 0), yaw*float32(math.Pi)/180)
		}
		position := rl.NewVector3(center.X+offset.X, center.Y+offset.Y, center.Z+offset.Z)
		drawScale := rl.NewVector3(part.scale.X*scale.X, part.scale.Y*scale.Y, part.scale.Z*scale.Z)
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

