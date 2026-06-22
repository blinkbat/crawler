package render

import (
	"image/color"
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"

	"crawler/internal/app/core"
)

// Voxel-stack rendering — draw path for a map with an explicit Solids stack (gapped columns: floating cubes/decks over air). Pure heightfield (Solids nil) never reaches here. Draws what the heightfield can't: multiple floors per column, per-run side faces, and floating-cube undersides.
//
// Cube convention (matches the heightfield face math): a cube with top at level L occupies world Y ∈ [ElevationWorldY(L-1), ElevationWorldY(L)].

var underQuadPins [][]float32

// buildUnderQuadModel builds a TileSize×TileSize XZ quad at model y=0, normal facing -Y (a floating cube's underside). drawVoxelColumn translates it to (cx, bottomY, cz).
func buildUnderQuadModel(pixels []color.RGBA, shader rl.Shader) rl.Model {
	const half = float32(core.TileSize) / 2
	// CCW seen from below (-Y) so the downward face survives back-face culling.
	a := rl.NewVector3(-half, 0, -half)
	b := rl.NewVector3(half, 0, -half)
	c := rl.NewVector3(half, 0, half)
	d := rl.NewVector3(-half, 0, half)
	verts := []float32{
		a.X, a.Y, a.Z, b.X, b.Y, b.Z, c.X, c.Y, c.Z, // tri 1
		a.X, a.Y, a.Z, c.X, c.Y, c.Z, d.X, d.Y, d.Z, // tri 2
	}
	normals := make([]float32, 0, len(verts))
	for i := 0; i < 6; i++ {
		normals = append(normals, 0, -1, 0) // -Y downward
	}
	uvs := []float32{
		0, 0, 1, 0, 1, 1,
		0, 0, 1, 1, 0, 1,
	}
	underQuadPins = append(underQuadPins, verts, normals, uvs)
	mesh := rl.Mesh{
		VertexCount:   int32(len(verts) / 3),
		TriangleCount: int32(len(verts) / 9),
	}
	mesh.Vertices = (*float32)(unsafe.Pointer(&verts[0]))
	mesh.Normals = (*float32)(unsafe.Pointer(&normals[0]))
	mesh.Texcoords = (*float32)(unsafe.Pointer(&uvs[0]))
	rl.UploadMesh(&mesh, false)
	model := rl.LoadModelFromMesh(mesh)
	setModelTexture(&model, loadTiledTexture(pixels))
	attachShader(&model, shader)
	return model
}

// voxelNeighborSolid reports whether the neighbour column is solid at level L. Off-map reads solid up to the baseline (clean 1-high lip at map edges), matching the heightfield's off-map==baseline rule.
func voxelNeighborSolid(m *core.AreaDefinition, nx, nz, L int) bool {
	if !m.InBounds(nx, nz) {
		return L <= core.ElevationBaseline
	}
	_, solid := m.SolidAt(nx, L, nz)
	return solid
}

// voxelSolidScratch is the reused per-column solidity buffer. Safe as package state because raylib draw is single-threaded.
var voxelSolidScratch []bool

// drawVoxelColumn renders column (x,z): a floor on each standable surface, one face quad per exposed level on each visible edge, and a downward face under each floating run. Returns the face count.
func drawVoxelColumn(camPos rl.Vector3, material worldMaterialResources, assets Resources, m *core.AreaDefinition, x, z int, cx, cz float32) int {
	const half = float32(core.TileSize) / 2
	h := m.SolidStackHeight()

	// Resolve each level's solidity ONCE into scratch; the passes below would otherwise re-read SolidAt per level (~5×h redundant self-column reads/frame). standable(L) = cube L solid AND L+1 air.
	if cap(voxelSolidScratch) < h {
		voxelSolidScratch = make([]bool, h)
	}
	solid := voxelSolidScratch[:h]
	for L := 0; L < h; L++ {
		_, solid[L] = m.SolidAt(x, L, z)
	}
	// Floors on every standable surface; ascending, the first standable level is the column's lowest surface.
	lowestDone := false
	for L := 0; L < h; L++ {
		if !(solid[L] && (L+1 >= h || !solid[L+1])) {
			continue
		}
		topY := core.ElevationWorldY(L)
		if !lowestDone {
			lowestDone = true
			// Authored ground surface keeps its floor char; reuse the heightfield floor path.
			drawFloorTile(material, assets, m.Floor[z][x], x, z, cx, cz, topY)
		} else {
			// Raised decks are generic material floor (upper surfaces have no authored floor type).
			drawTileCube(material.floorModel, cx, -0.03+topY, cz, tileYawDeg(x, z))
		}
	}

	drawn := 0
	// Side faces, per visible edge.
	for _, d := range core.CardinalDirs {
		dx, dz := core.FacingVector(d)
		fdx, fdz := float32(dx), float32(dz)
		// CPU backface cull — a vertical face is only visible from its outward side.
		if faceBackfaceCulled(camPos, cx, cz, fdx, fdz, half) {
			continue
		}
		nx, nz := x+dx, z+dz
		yaw := faceYaw(d)
		// Per-direction skin (FaceSkinForDir falls back to base), resolved lazily so a fully-buried edge pays no lookup.
		skin := material.faceModel
		skinResolved := false
		// One quad PER LEVEL (one cube tall) so the texture tiles per voxel instead of stretching across a multi-level run.
		for L := 0; L < h; L++ {
			if !solid[L] || voxelNeighborSolid(m, nx, nz, L) {
				continue
			}
			if !skinResolved {
				skinResolved = true
				if sc := m.FaceSkinForDir(x, z, d); assets.faceVariantTable.present[sc] {
					skin = assets.faceVariantTable.model[sc]
				}
			}
			drawCliffFace(skin, cx, core.ElevationWorldY(L-1), cz, yaw, 1)
			drawn++
		}
	}

	// Undersides: a downward quad under each floating run bottom. L starts at 1 (level-0 cubes rest on the world floor).
	for L := 1; L < h; L++ {
		if !solid[L] {
			continue
		}
		if solid[L-1] {
			continue // resting on the cube beneath — not floating
		}
		drawTileCube(assets.underModel, cx, core.ElevationWorldY(L-1), cz, 0)
	}
	return drawn
}
