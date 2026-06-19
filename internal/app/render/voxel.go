package render

import (
	"image/color"
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"

	"crawler/internal/app/core"
)

// Voxel-stack rendering — the draw path for a map whose AreaDefinition carries
// an explicit Solids stack (a gapped column: a floating cube/deck over air). A
// pure heightfield (Solids nil) never reaches here; it keeps the original
// per-tile floor+cliff path in world.go unchanged, so existing maps render
// pixel-identically. This file draws the extra geometry the heightfield model
// can't: multiple walkable floors per column, side faces per solid run, and the
// undersides of floating cubes.
//
// Cube convention (matches the heightfield's face math): a cube whose top
// surface is at level L occupies world Y ∈ [ElevationWorldY(L-1), ElevationWorldY(L)].
// So a contiguous exposed solid run [Lbot..Ltop] draws ONE stretched face quad
// with its base at ElevationWorldY(Lbot-1) and height (Ltop-Lbot+1) levels —
// identical to drawCliffFace for the single-run heightfield case.

var underQuadPins [][]float32

// buildUnderQuadModel builds a horizontal quad (TileSize × TileSize) in the XZ
// plane at model-space y=0, wound so its normal faces -Y (downward) — the
// underside of a floating cube, visible to a player standing beneath it.
// drawVoxelColumn translates it to (cx, bottomY, cz); no rotation/scale.
func buildUnderQuadModel(pixels []color.RGBA, shader rl.Shader) rl.Model {
	const half = float32(core.TileSize) / 2
	// CCW as seen from BELOW (-Y side) so the downward face survives back-face
	// culling. Looking up the -Y axis, +X right and +Z down, CCW is
	// (-x,-z)->(+x,-z)->(+x,+z).
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

// voxelNeighborSolid reports whether the neighbour column presents solid
// material at level L across a shared edge — the per-level analogue of
// core.NeighbourEdgeLevel's off-map rule. Off-map reads as solid up to the
// baseline (so an above-baseline cube shows a clean 1-high lip at the map edge
// rather than a face plunging to the bottom of the range), matching the
// heightfield renderer's off-map==baseline convention.
func voxelNeighborSolid(m *core.AreaDefinition, nx, nz, L int) bool {
	if !m.InBounds(nx, nz) {
		return L <= core.ElevationBaseline
	}
	_, solid := m.SolidAt(nx, L, nz)
	return solid
}

// voxelSolidScratch is the reused per-column solidity buffer drawVoxelColumn
// resolves once before its floors/faces/undersides passes. Safe as package state
// because raylib draw is single-threaded.
var voxelSolidScratch []bool

// drawVoxelColumn renders column (x,z) from the voxel stack: a floor on every
// standable surface (authored Floor on the lowest/ground surface, generic
// material floor on raised decks), one stretched rock face per contiguous
// exposed solid run on each visible edge, and a downward face under every
// floating run. Returns the face count for the render-log tally.
func drawVoxelColumn(camPos rl.Vector3, material worldMaterialResources, assets Resources, m *core.AreaDefinition, x, z int, cx, cz float32) int {
	const half = float32(core.TileSize) / 2
	h := m.SolidStackHeight()

	// Resolve each level's solidity ONCE into the reused scratch column. The three
	// passes below (and the four per-direction face sub-passes) would otherwise
	// each re-read SolidAt per level — the self-column re-read alone is ~5×h
	// redundant reads per column per frame. standable(L) and the ground level then
	// derive from the scratch instead of calling Standable / LowestStandableLevel
	// (each its own stack walk). SolidAt stays the only read of the neighbour
	// columns (genuinely per-direction). Equivalent to core.Standable: cube L
	// solid AND L+1 air (a level past the top reads as air).
	if cap(voxelSolidScratch) < h {
		voxelSolidScratch = make([]bool, h)
	}
	solid := voxelSolidScratch[:h]
	for L := 0; L < h; L++ {
		_, solid[L] = m.SolidAt(x, L, z)
	}
	// Floors on every standable surface. Walking ascending, the FIRST standable
	// level is the column's lowest surface — fold that detection into this single
	// pass instead of a separate pre-scan for `lowest`.
	lowestDone := false
	for L := 0; L < h; L++ {
		if !(solid[L] && (L+1 >= h || !solid[L+1])) {
			continue
		}
		topY := core.ElevationWorldY(L)
		if !lowestDone {
			lowestDone = true
			// The authored ground surface keeps its floor char (grass/stone/
			// water/ramp); reuse the heightfield floor path verbatim.
			drawFloorTile(material, assets, m.Floor[z][x], x, z, cx, cz, topY)
		} else {
			// Raised decks are generic material floor (scope cap: upper surfaces
			// have no authored floor type — see voxel plan).
			drawTileCube(material.floorModel, cx, -0.03+topY, cz, tileYawDeg(x, z))
		}
	}

	drawn := 0
	// Side faces: per visible edge, one stretched quad per contiguous exposed
	// solid run.
	for _, d := range core.CardinalDirs {
		dx, dz := core.FacingVector(d)
		fdx, fdz := float32(dx), float32(dz)
		// CPU backface cull — a vertical face is only visible from its outward
		// side (same test as drawCliffFaces).
		if (camPos.X-(cx+fdx*half))*fdx+(camPos.Z-(cz+fdz*half))*fdz <= 0 {
			continue
		}
		nx, nz := x+dx, z+dz
		yaw := faceYaw(d)
		// Per-DIRECTION skin: a tile can wear a different face skin on each side
		// (FaceSkinForDir falls back to the base skin when no override), so the
		// N/E/S/W faces of one cube can differ. Resolved LAZILY on the first
		// exposed level so a fully-buried edge (no visible face) pays neither the
		// FaceSkinForDir lookup nor the variant-table probe.
		skin := material.faceModel
		skinResolved := false
		// One face quad PER LEVEL, each a single cube tall, so the wall texture
		// TILES per voxel instead of stretching one copy across a multi-level run
		// (drawCliffFace scales the model by its level count, which stretches the
		// UVs). A run of three exposed cubes now reads as three stacked cubes.
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

	// Undersides: a downward quad beneath every floating run bottom — a solid
	// cube with air directly below it. L starts at 1 since a level-0 cube rests
	// on the world floor and has no visible underside.
	for L := 1; L < h; L++ {
		if !solid[L] {
			continue
		}
		if solid[L-1] {
			continue // resting on the cube beneath — not a floating bottom
		}
		rl.DrawModelEx(assets.underModel,
			rl.NewVector3(cx, core.ElevationWorldY(L-1), cz),
			rl.NewVector3(0, 1, 0), 0,
			rl.NewVector3(1, 1, 1), rl.White)
	}
	return drawn
}
