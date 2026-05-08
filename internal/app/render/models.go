package render

import (
	"image/color"

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

const (
	treeMeshRoot = iota
	treeMeshTrunk
	treeMeshCanopyLow
	treeMeshCanopyHigh
	treeMeshCanopySide
	treeMeshCanopyAccent
)

func loadTreeModel(shader rl.Shader, barkTex, leafTex rl.Texture2D) treeModel {
	models := []rl.Model{
		treeMeshRoot:         rl.LoadModelFromMesh(rl.GenMeshCylinder(0.32, 0.18, 10)),
		treeMeshTrunk:        rl.LoadModelFromMesh(rl.GenMeshCylinder(0.18, 1.55, 12)),
		treeMeshCanopyLow:    rl.LoadModelFromMesh(rl.GenMeshSphere(0.92, 12, 16)),
		treeMeshCanopyHigh:   rl.LoadModelFromMesh(rl.GenMeshSphere(0.78, 12, 16)),
		treeMeshCanopySide:   rl.LoadModelFromMesh(rl.GenMeshSphere(0.55, 10, 14)),
		treeMeshCanopyAccent: rl.LoadModelFromMesh(rl.GenMeshSphere(0.38, 10, 12)),
	}
	for i := range models {
		tex := leafTex
		if i == treeMeshRoot || i == treeMeshTrunk {
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
			{modelIdx: treeMeshRoot, offset: rl.NewVector3(0, 0.04, 0), scale: rl.NewVector3(1, 1, 1), tint: rl.White},
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

func (t treeModel) draw(center rl.Vector3, scale float32) {
	if scale <= 0 {
		scale = 1
	}
	for _, part := range t.parts {
		position := rl.NewVector3(
			center.X+part.offset.X*scale,
			center.Y+part.offset.Y*scale,
			center.Z+part.offset.Z*scale,
		)
		drawScale := rl.NewVector3(part.scale.X*scale, part.scale.Y*scale, part.scale.Z*scale)
		rl.DrawModelEx(t.models[part.modelIdx], position, partRotationAxis(part), part.rotation, drawScale, part.tint)
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

func (p propModel) draw(center rl.Vector3, scale float32) {
	if scale <= 0 {
		scale = 1
	}
	for _, part := range p.parts {
		position := rl.NewVector3(
			center.X+part.offset.X*scale,
			center.Y+part.offset.Y*scale,
			center.Z+part.offset.Z*scale,
		)
		drawScale := rl.NewVector3(part.scale.X*scale, part.scale.Y*scale, part.scale.Z*scale)
		rl.DrawModelEx(p.models[part.modelIdx], position, partRotationAxis(part), part.rotation, drawScale, part.tint)
	}
}

// drawXYZ renders the prop with a non-uniform scale so callers can squash a
// model's vertical extent independently from its footprint. Used by
// scattered small-rock decorations to produce low, walkable-looking pebbles
// from the same boulder mesh set.
func (p propModel) drawXYZ(center rl.Vector3, scale rl.Vector3) {
	for _, part := range p.parts {
		position := rl.NewVector3(
			center.X+part.offset.X*scale.X,
			center.Y+part.offset.Y*scale.Y,
			center.Z+part.offset.Z*scale.Z,
		)
		drawScale := rl.NewVector3(part.scale.X*scale.X, part.scale.Y*scale.Y, part.scale.Z*scale.Z)
		rl.DrawModelEx(p.models[part.modelIdx], position, partRotationAxis(part), part.rotation, drawScale, part.tint)
	}
}

func (p propModel) unload() {
	for i := range p.models {
		rl.UnloadModel(p.models[i])
	}
}

// Rock prop mesh indices. The "large rock" tile is now a crystal cluster —
// a jagged base with multiple low-poly cones jutting out at varied angles.
// Indexes are named so the parts list reads as a recipe instead of magic
// numbers; the small-pebble drawer in world.go also pulls rockMeshBase
// directly to recycle the base block as ground scatter.
const (
	rockMeshBase     = iota // wide low matrix block (the "rock the crystals grew on")
	rockMeshCrystalA        // tall hex-faceted main spire
	rockMeshCrystalB        // medium 6-sided crystal
	rockMeshCrystalC        // shorter 5-sided crystal (irregular feel)
	rockMeshCrystalD        // small 4-sided shard
	rockMeshShard           // long thin elongated cube — a slab/blade crystal
)

// loadRockProp builds a quartz-cluster prop: a low rough base studded with
// faceted cones at varied angles, sizes, and slice counts so each crystal
// reads as a distinct mineral spike rather than a copy. Some crystals tilt
// off-vertical via per-part rotation axes for a natural "growing in
// different directions" look you'd see on a real geode. The whole thing
// shares the rock wall texture so it ties to the field's matte-stone
// palette; the cool tints shift each crystal toward purple/blue so the
// cluster reads as crystalline and not just "another rock pile."
func loadRockProp(shader rl.Shader, rockTex rl.Texture2D) propModel {
	models := []rl.Model{
		rockMeshBase: rl.LoadModelFromMesh(rl.GenMeshCube(1.10, 0.36, 0.95)),
		// Cones: low slice counts make them faceted, which is what reads as
		// a crystal facet. Heights and radii are picked so the silhouettes
		// don't repeat each other.
		rockMeshCrystalA: rl.LoadModelFromMesh(rl.GenMeshCone(0.18, 1.18, 6)),
		rockMeshCrystalB: rl.LoadModelFromMesh(rl.GenMeshCone(0.15, 0.86, 6)),
		rockMeshCrystalC: rl.LoadModelFromMesh(rl.GenMeshCone(0.13, 0.68, 5)),
		rockMeshCrystalD: rl.LoadModelFromMesh(rl.GenMeshCone(0.10, 0.48, 4)),
		// A square-section "blade" crystal — long thin cube. Tilted into the
		// cluster so it reads as a fractured shard wedged in the matrix.
		rockMeshShard: rl.LoadModelFromMesh(rl.GenMeshCube(0.16, 0.78, 0.16)),
	}
	for i := range models {
		setModelTexture(&models[i], rockTex)
		attachShader(&models[i], shader)
	}

	// Crystal palette — cool purples and blues over the rock-warm base. Each
	// crystal gets its own tint so the cluster looks like multiple species
	// of mineral grew side by side.
	matrixTint := rl.NewColor(174, 168, 158, 255)
	mainTint := rl.NewColor(148, 130, 198, 255)   // amethyst
	midTint := rl.NewColor(172, 156, 220, 255)    // pale violet
	paleTint := rl.NewColor(204, 188, 230, 255)   // lavender
	blueTint := rl.NewColor(150, 168, 224, 255)   // sky-quartz
	bladeTint := rl.NewColor(210, 200, 226, 255)  // milky quartz blade
	smallTint := rl.NewColor(220, 200, 232, 255)  // accent

	return propModel{
		models: models,
		parts: []treePart{
			// Matrix base: low chunky block under the crystals.
			{modelIdx: rockMeshBase, offset: rl.NewVector3(0, 0.18, 0), scale: rl.NewVector3(1, 1, 1), rotation: 8, tint: matrixTint},

			// Main spire — tall, vertical, slightly off-center. The signature
			// piece you see first.
			{modelIdx: rockMeshCrystalA, offset: rl.NewVector3(-0.06, 0.30, 0.04), scale: rl.NewVector3(1, 1, 1), rotation: 20, tint: mainTint},

			// Mid crystal tilted toward +X using a non-Y axis. The axis
			// (1, 6, 0) keeps it mostly upright but leans the tip toward +X
			// for a "leaning into the wind" silhouette.
			{modelIdx: rockMeshCrystalB, offset: rl.NewVector3(0.30, 0.30, -0.10), scale: rl.NewVector3(1, 1, 1), rotation: 18, rotationAxis: rl.NewVector3(1, 6, 0), tint: midTint},

			// Shorter crystal tilted toward -Z. Different lean direction
			// from the previous crystal so the cluster doesn't look combed.
			{modelIdx: rockMeshCrystalC, offset: rl.NewVector3(-0.32, 0.30, -0.20), scale: rl.NewVector3(1, 1, 1), rotation: -22, rotationAxis: rl.NewVector3(0, 5, 1), tint: paleTint},

			// Tilted blue crystal toward +Z+x for variety. Slightly stronger
			// tilt so it reads as the "wild" crystal in the cluster.
			{modelIdx: rockMeshCrystalC, offset: rl.NewVector3(0.20, 0.32, 0.28), scale: rl.NewVector3(0.95, 0.85, 0.95), rotation: 28, rotationAxis: rl.NewVector3(1, 4, 1), tint: blueTint},

			// Long blade slab — thin elongated cube standing nearly vertical
			// but rotated around an axis with a touch of X for an asymmetric
			// fault-plane angle.
			{modelIdx: rockMeshShard, offset: rl.NewVector3(0.04, 0.50, -0.30), scale: rl.NewVector3(1, 1, 1), rotation: 14, rotationAxis: rl.NewVector3(1, 8, 0), tint: bladeTint},

			// Two smaller crystals at the base flanks — short, nearly
			// upright, slightly canted. They fill in the silhouette so the
			// cluster has visual density at every height.
			{modelIdx: rockMeshCrystalD, offset: rl.NewVector3(-0.34, 0.30, 0.22), scale: rl.NewVector3(1, 1, 1), rotation: -10, tint: smallTint},
			{modelIdx: rockMeshCrystalD, offset: rl.NewVector3(0.36, 0.30, 0.06), scale: rl.NewVector3(0.95, 0.92, 0.95), rotation: 36, rotationAxis: rl.NewVector3(0, 6, 1), tint: smallTint},

			// Tiny accent crystal up high, leaning toward camera-ish. Adds a
			// pointy peak above the main spire's shoulder.
			{modelIdx: rockMeshCrystalD, offset: rl.NewVector3(0.10, 0.60, -0.04), scale: rl.NewVector3(0.7, 0.8, 0.7), rotation: 50, rotationAxis: rl.NewVector3(2, 5, 0), tint: paleTint},
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

