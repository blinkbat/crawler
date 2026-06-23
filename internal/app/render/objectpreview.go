package render

import (
	"math"

	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Object Browser preview support: per-cell 3D thumbnails for the editor's Object Browser.
// Driven by the world's own dispatch tables (propModelTable/decorModelTable/inlineProp/Decor) so a thumbnail matches the in-game look exactly.

// objectPreviewRT is the cached off-screen target shared by every thumbnail (one size per gallery).
var objectPreviewRT previewRT

// ObjectPreviewItem names one placeable object: layer char, label, and props-vs-decor layer. Char + IsProp is all DrawObjectPreview needs to route.
type ObjectPreviewItem struct {
	Char   byte
	Name   string
	IsProp bool
}

// ObjectPreviewItems enumerates every placeable object (props then decor). Footprint TAILS are skipped (they draw nothing alone). Derived from the renderer's char lists, so new props/decor appear automatically.
// objectPreviewItemsCache: lazily built once. Returned slice is read-only.
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

// CloseObjectPreview frees the cached off-screen texture. Idempotent.
func CloseObjectPreview() { objectPreviewRT.close() }

// objectPreviewDir is the fixed three-quarter view direction (eye = target + dir*distance).
var objectPreviewDir = rl.Vector3Normalize(rl.NewVector3(0.55, 0.62, 1.0))

// objectPreviewGroundSize is the thumbnail floor extent (tighter than visualizerGroundSize).
const objectPreviewGroundSize = float32(12)

// DrawObjectPreview renders item's object (lit, shadowed, animated as in-world) into rect, auto-framed and dollied by zoom (1=fit, >1 closer). Safe per frame; texture is cached.
func DrawObjectPreview(rect rl.Rectangle, assets Resources, item ObjectPreviewItem, zoom float32) {
	w, h := int32(rect.Width), int32(rect.Height)
	if w <= 0 || h <= 0 {
		return
	}
	if assets.lighting.shader.ID == 0 {
		// No lighting shader (zero-value Resources): bail rather than BeginShaderMode(0).
		return
	}
	if !objectPreviewRT.ensure(w, h) {
		return
	}
	// Feed the shared sway/flicker clock; without this thumbnails' foliage sway and torch flame freeze.
	worldFrameClock = float32(rl.GetTime())
	cam := objectPreviewCamera(objectPreviewBounds(assets, item), zoom)

	// Shared off-screen scene setup, with the tighter prop floor and no grid.
	objectPreviewRT.beginVisualizerScene(cam, objectPreviewGroundSize, false)

	// Bright outdoor profile, no torches, through the world's lighting shader to match a field map.
	assets.lighting.applyUniforms(cam, fieldLighting)
	assets.lighting.uploadTorches(nil)
	rl.BeginShaderMode(assets.lighting.shader)
	drawObjectPreviewModel(assets, item, rl.NewVector3(0, 0, 0))
	rl.EndShaderMode()

	rl.EndMode3D()
	rl.EndTextureMode()

	objectPreviewRT.blit(rect)
}

// drawObjectPreviewModel draws one object at center via the same tables drawWorld/drawDecor use. Footprints draw at origin; area-needing inline props get an empty AreaDefinition (torch faces south).
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

// objectPreviewCamera frames bb in three-quarter view: eye pulled back along objectPreviewDir so the bounding sphere fits the FOV with margin. zoom>1 dollies closer.
func objectPreviewCamera(bb rl.BoundingBox, zoom float32) rl.Camera3D {
	const fovy = previewFovy
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

// objectPreviewBounds returns world-space bounds for framing. Inline-handled objects map to the model the handler draws; falls back to a unit box.
func objectPreviewBounds(assets Resources, item ObjectPreviewItem) rl.BoundingBox {
	char := item.Char
	unit := rl.NewBoundingBox(rl.NewVector3(-0.5, 0, -0.5), rl.NewVector3(0.5, 1, 0.5))
	if item.IsProp {
		switch char {
		case core.TileTree, core.TileTreeXL, core.TileTreeTall, core.TileTreeYoung:
			return partsBoundsOr(assets.tree.models, assets.tree.parts, treePropScales[char], unit)
		case core.TileTreeTwin:
			// Two offset instances; widen horizontally to cover the pair's diagonal spread.
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

// partsBoundsOr returns the union AABB of a model's parts (offset + per-part scale, then whole-scaled), or fallback if empty. Part rotation ignored — the camera margin absorbs the slack.
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
