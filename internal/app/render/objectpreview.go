package render

import (
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Object Browser preview support. The editor's Object Browser modal
// (editor/objectview.go) calls DrawObjectPreview per visible cell to show a
// live 3D thumbnail of every decor/prop object the map can place, so the
// author can spot-check the whole prop/decor set at a glance — the same
// off-screen-RenderTexture trick the Foe/Party Visualizers use, but driven by
// the world's OWN dispatch tables (propModelTable / decorModelTable /
// inlinePropTable / inlineDecorTable) so a thumbnail draws exactly what the
// world draws, with no parallel object list to keep in sync.

// objectPreviewRT is the cached off-screen target shared by every thumbnail —
// all cells in a gallery are the same size, so it (re)allocates once per modal
// session and is reused for each cell's render-and-blit.
var objectPreviewRT previewRT

// ObjectPreviewItem names one placeable object for the gallery: its layer char,
// human label (from core.TileLabel), and whether it lives on the props layer
// (vs the decor layer). The Char + IsProp pair is all DrawObjectPreview needs
// to route through the same tables the world renderer uses.
type ObjectPreviewItem struct {
	Char   byte
	Name   string
	IsProp bool
}

// ObjectPreviewItems enumerates every renderable object the editor can place:
// the blocking props first (trees, pillars, furniture, …), then the floor
// decor (grass, flowers, blood, rugs, …). Multi-tile footprint TAILS are
// skipped — they render nothing on their own (the anchor draws the spanning
// mesh), so a tail thumbnail would be blank. Derived from the same char lists
// the renderer asserts coverage against, so a newly-registered prop/decor shows
// up here automatically.
// objectPreviewItemsCache memoizes the (static) object list. The prop/decor
// char lists and their TileLabels are compile-time constants, so the result
// never changes for the process — but the Object Browser asks for it twice per
// frame (modal update + draw), so building it fresh each time append-grew a new
// slice 60+ times/sec. Built once, lazily. Callers MUST treat the returned
// slice as read-only (the browser only indexes it).
var objectPreviewItemsCache []ObjectPreviewItem

func ObjectPreviewItems() []ObjectPreviewItem {
	if objectPreviewItemsCache != nil {
		return objectPreviewItemsCache
	}
	var out []ObjectPreviewItem
	for _, c := range core.PropTileChars() {
		if isFootprintTail(c) {
			continue
		}
		out = append(out, ObjectPreviewItem{Char: c, Name: core.TileLabel(core.TileLayerProps, c), IsProp: true})
	}
	for _, c := range core.DecorTileChars() {
		if isDecorFootprintTail(c) {
			continue
		}
		out = append(out, ObjectPreviewItem{Char: c, Name: core.TileLabel(core.TileLayerDecor, c), IsProp: false})
	}
	objectPreviewItemsCache = out
	return objectPreviewItemsCache
}

// CloseObjectPreview frees the cached off-screen texture; the editor calls it
// when the Object Browser closes so the GPU handle isn't held for the session.
// Idempotent.
func CloseObjectPreview() { objectPreviewRT.close() }

// objectPreviewDir is the fixed three-quarter view direction every thumbnail is
// framed from (eye = target + dir*distance). A gentle elevation + offset so the
// object reads as a 3D form rather than a flat front elevation.
var objectPreviewDir = normalizeVec3(rl.NewVector3(0.55, 0.62, 1.0))

// DrawObjectPreview renders item's object — lit, ground-shadowed, and animated
// (foliage sway, torch flame, fountain water) exactly as in the world — into
// rect, auto-framed to the object's bounds and dollied by zoom (1 = fit,
// >1 closer). Safe to call every frame: the off-screen texture is cached and
// only reallocated when the cell size changes.
func DrawObjectPreview(rect rl.Rectangle, assets Resources, item ObjectPreviewItem, zoom float32) {
	w, h := int32(rect.Width), int32(rect.Height)
	if w <= 0 || h <= 0 {
		return
	}
	if assets.lighting.shader.ID == 0 {
		// Lighting shader not built (zero-value Resources): the diorama draws
		// through it, so bail rather than BeginShaderMode(0) and write uniforms
		// to invalid locations. Mirrors DrawFoePreview gating its shader work.
		return
	}
	if !objectPreviewRT.ensure(w, h) {
		return
	}
	cam := objectPreviewCamera(objectPreviewBounds(assets, item), zoom)

	rl.BeginTextureMode(objectPreviewRT.rt)
	rl.ClearBackground(foePreviewBG)
	rl.BeginMode3D(cam)
	rl.DrawPlane(rl.NewVector3(0, 0, 0), rl.NewVector2(12, 12), foePreviewGround)

	// Light the diorama with the bright outdoor profile and no torches, then
	// draw the object through the world's lighting shader so its painted look
	// matches an actual field map.
	assets.lighting.applyUniforms(cam, fieldLighting)
	assets.lighting.uploadTorches(nil)
	rl.BeginShaderMode(assets.lighting.shader)
	drawObjectPreviewModel(assets, item, rl.NewVector3(0, 0, 0))
	rl.EndShaderMode()

	rl.EndMode3D()
	rl.EndTextureMode()

	// RenderTextures are stored flipped, so the source height is negated.
	rl.DrawTextureRec(objectPreviewRT.rt.Texture,
		rl.NewRectangle(0, 0, float32(w), -float32(h)),
		rl.NewVector2(rect.X, rect.Y),
		rl.White)
}

// drawObjectPreviewModel draws one object centered at `center`, routing through
// the SAME tables the world's drawWorld/drawDecor use (inlinePropTable /
// propModelTable / inlineDecorTable / decorModelTable) so a thumbnail can't
// drift from the in-game look. Footprint props/decor draw at the origin (not
// their multi-tile anchor offset) so they sit centered in the cell. Inline
// props that need an area (the wall torch) get an empty AreaDefinition, which
// makes the torch face south toward the camera.
func drawObjectPreviewModel(assets Resources, item ObjectPreviewItem, center rl.Vector3) {
	char := item.Char
	if item.IsProp {
		if handler := inlinePropTable[char]; handler != nil {
			handler(assets, &core.AreaDefinition{}, 0, 0, center, 0)
			return
		}
		if pm := &assets.propModelTable[char]; len(pm.parts) > 0 {
			if r := propShadowRadiusTable[char]; r > 0 {
				drawGroundShadow(center.X, center.Z, r)
			}
			pm.draw(center, 1, 0)
		}
		return
	}
	if handler := inlineDecorTable[char]; handler != nil {
		handler(assets, 0, 0, center.X, center.Z, center.Y)
		return
	}
	if dm := &assets.decorModelTable[char]; len(dm.parts) > 0 {
		dm.draw(center, 1, 0)
	}
}

// objectPreviewCamera frames bb (the object's world-space bounds) in a
// three-quarter view: target the bounds center, then pull the eye back along
// objectPreviewDir far enough that the bounding sphere fits the vertical FOV,
// with a margin. zoom>1 dollies closer.
func objectPreviewCamera(bb rl.BoundingBox, zoom float32) rl.Camera3D {
	const fovy = float32(46)
	center := rl.Vector3Scale(rl.Vector3Add(bb.Min, bb.Max), 0.5)
	radius := 0.5 * rl.Vector3Length(rl.Vector3Subtract(bb.Max, bb.Min))
	if radius < 0.35 {
		radius = 0.35
	}
	dist := radius / float32(math.Sin(float64(fovy)*math.Pi/360)) // sin(fov/2)
	dist *= 1.25                                                  // breathing room around the silhouette
	if zoom > 0 {
		dist /= zoom
	}
	return rl.Camera3D{
		Position:   rl.Vector3Add(center, rl.Vector3Scale(objectPreviewDir, dist)),
		Target:     center,
		Up:         rl.NewVector3(0, 1, 0),
		Fovy:       fovy,
		Projection: rl.CameraPerspective,
	}
}

// objectPreviewBounds returns the world-space bounds used to frame an object's
// thumbnail. Table-dispatched props/decor measure their own meshes; the
// inline-handled ones (trees at their per-char scale, the big rock/bush, the
// small bush/mushroom, the wall torch, the pebble scatter) map to the model the
// handler actually draws so the framing matches. Falls back to a unit box.
func objectPreviewBounds(assets Resources, item ObjectPreviewItem) rl.BoundingBox {
	char := item.Char
	unit := rl.NewBoundingBox(rl.NewVector3(-0.5, 0, -0.5), rl.NewVector3(0.5, 1, 0.5))
	if item.IsProp {
		switch char {
		case core.TileTree, core.TileTreeXL, core.TileTreeTall, core.TileTreeYoung:
			return partsBoundsOr(assets.tree.models, assets.tree.parts, treePropScales[char], unit)
		case core.TileTreeTwin:
			// Two offset instances; the bigger (0.82) dominates — widen the box
			// horizontally to cover the ±0.32 diagonal spread of the pair.
			bb := partsBoundsOr(assets.tree.models, assets.tree.parts, 0.82, unit)
			bb.Min.X, bb.Min.Z = bb.Min.X-0.4, bb.Min.Z-0.4
			bb.Max.X, bb.Max.Z = bb.Max.X+0.4, bb.Max.Z+0.4
			return bb
		case core.TileRockLarge:
			return partsBoundsOr(assets.rockProp.models, assets.rockProp.parts, 1, unit)
		case core.TileBushLarge:
			return partsBoundsOr(assets.bushProp.models, assets.bushProp.parts, 1.3, unit)
		case core.TileTorch:
			return rl.NewBoundingBox(rl.NewVector3(-0.35, 0, -0.6), rl.NewVector3(0.35, 1.55, 0.25))
		}
		if pm := &assets.propModelTable[char]; len(pm.parts) > 0 {
			return partsBoundsOr(pm.models, pm.parts, 1, unit)
		}
		return unit
	}
	switch char {
	case core.DecorBush:
		return partsBoundsOr(assets.bushProp.models, assets.bushProp.parts, 0.75, unit)
	case core.DecorMushroom:
		return partsBoundsOr(assets.mushroomProp.models, assets.mushroomProp.parts, 1, unit)
	case core.DecorPebble:
		return rl.NewBoundingBox(rl.NewVector3(-0.5, 0, -0.5), rl.NewVector3(0.5, 0.3, 0.5))
	}
	if dm := &assets.decorModelTable[char]; len(dm.parts) > 0 {
		return partsBoundsOr(dm.models, dm.parts, 1, unit)
	}
	return unit
}

// partsBoundsOr returns the union AABB of a multi-mesh model's parts (each
// part's mesh bounds scaled by the part scale, translated by its offset, then
// the whole scaled by `scale`), or `fallback` when the part list is empty.
// Part rotation is ignored — an axis-aligned approximation is fine for framing,
// and the camera margin absorbs the small slack. Shared by every object family
// (treeModel and propModel use the same {models, parts} shape).
func partsBoundsOr(models []rl.Model, parts []treePart, scale float32, fallback rl.BoundingBox) rl.BoundingBox {
	if scale <= 0 {
		scale = 1
	}
	mn := rl.NewVector3(math.MaxFloat32, math.MaxFloat32, math.MaxFloat32)
	mx := rl.NewVector3(-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32)
	found := false
	for _, part := range parts {
		if part.modelIdx < 0 || part.modelIdx >= len(models) {
			continue
		}
		bb := rl.GetModelBoundingBox(models[part.modelIdx])
		lo := rl.NewVector3(
			part.offset.X+bb.Min.X*part.scale.X, part.offset.Y+bb.Min.Y*part.scale.Y, part.offset.Z+bb.Min.Z*part.scale.Z)
		hi := rl.NewVector3(
			part.offset.X+bb.Max.X*part.scale.X, part.offset.Y+bb.Max.Y*part.scale.Y, part.offset.Z+bb.Max.Z*part.scale.Z)
		mn = rl.Vector3Min(mn, rl.Vector3Min(lo, hi))
		mx = rl.Vector3Max(mx, rl.Vector3Max(lo, hi))
		found = true
	}
	if !found {
		return fallback
	}
	return rl.NewBoundingBox(rl.Vector3Scale(mn, scale), rl.Vector3Scale(mx, scale))
}
