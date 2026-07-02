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

// objectPreviewKind routes an item to the right draw + bounds path in the Object
// Browser: the two paint layers plus the standalone 3D entities.
type objectPreviewKind uint8

const (
	previewProp objectPreviewKind = iota
	previewDecor
	previewChest
	previewDoor
	previewCrystal
)

// ObjectPreviewItem names one placeable object: its layer char (grid layers only),
// label, and Kind. Kind (+ Char for grid layers) is all DrawObjectPreview needs to route.
type ObjectPreviewItem struct {
	Char byte
	Name string
	Kind objectPreviewKind
}

// ObjectPreviewItems enumerates every placeable object: props, decor, then the 3D
// entities (chest / door / crystal). Footprint TAILS are skipped (they draw nothing
// alone). Grid rows derive from the renderer's char lists, so new props/decor appear
// automatically. objectPreviewItemsCache: lazily built once; returned slice is read-only.
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
		out = append(out, ObjectPreviewItem{Char: c, Name: core.TileLabel(core.TileLayerProps, c), Kind: previewProp})
	}
	for _, c := range core.DecorTileChars() {
		if isDecorFootprintTail(c) {
			continue
		}
		out = append(out, ObjectPreviewItem{Char: c, Name: core.TileLabel(core.TileLayerDecor, c), Kind: previewDecor})
	}
	// Standalone 3D entities (not tile chars): one representative of each.
	out = append(out,
		ObjectPreviewItem{Name: "Chest", Kind: previewChest},
		ObjectPreviewItem{Name: "Door", Kind: previewDoor},
		ObjectPreviewItem{Name: "Crystal", Kind: previewCrystal},
	)
	objectPreviewItemsCache = out
	return objectPreviewItemsCache
}

// CloseObjectPreview frees the cached off-screen texture. Idempotent.
func CloseObjectPreview() { objectPreviewRT.close() }

// objectPreviewBaseYaw/Pitch is the default three-quarter view (radians); the
// Object Browser's per-item drag-rotate adds to these. Pitch is clamped so the
// object never flips under/over.
const (
	objectPreviewBaseYaw   = float32(0.52)
	objectPreviewBasePitch = float32(0.52)
	objectPreviewMinPitch  = float32(0.08)
	objectPreviewMaxPitch  = float32(1.45)
)

// objectPreviewGroundSize is the thumbnail floor extent (tighter than visualizerGroundSize).
const objectPreviewGroundSize = float32(12)

// previewFovy is the vertical FOV for the object (prop) preview diorama. (The
// foe/party previews derive their FOV from the battle tuning instead.)
const previewFovy = float32(46)

// DrawObjectPreview renders item's object (lit, shadowed, animated as in-world) into
// rect, framed three-quarter then orbited by (yaw,pitch) and dollied by zoom (1=fit,
// >1 closer). Safe per frame; texture is cached.
func DrawObjectPreview(rect rl.Rectangle, assets Resources, item ObjectPreviewItem, yaw, pitch, zoom float32) {
	w, h := int32(rect.Width), int32(rect.Height)
	if w <= 0 || h <= 0 {
		return
	}
	if assets.lighting.shader.ID == 0 {
		// No lighting shader (zero-value Resources): bail rather than BeginShaderMode(0).
		return
	}
	// ensureStable, not ensure: the rect tracks the window layout, so a resize
	// drag would otherwise Unload+Load the RT every frame (the DrawModelEx crash).
	if !objectPreviewRT.ensureStable(w, h) {
		return
	}
	// Feed the shared sway/flicker clock; without this thumbnails' foliage sway and torch flame freeze.
	worldFrameClock = float32(rl.GetTime())
	cam := objectPreviewCamera(objectPreviewBounds(assets, item), yaw, pitch, zoom)

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

// drawObjectPreviewModel draws one object at center via the same tables/entities
// drawWorld uses. Footprints draw at origin; area-needing inline props get an empty
// AreaDefinition (torch faces south). Entities reuse their in-world draw primitives.
func drawObjectPreviewModel(assets Resources, item ObjectPreviewItem, center rl.Vector3) {
	switch item.Kind {
	case previewChest:
		drawGroundShadow(center.X, center.Z, chestShadowRadius)
		assets.chestBody.draw(center, 1, 0)
		assets.chestLid.draw(rl.NewVector3(center.X, center.Y+chestGeo.BodyHeight, center.Z), 1, 0)
		return
	case previewDoor:
		style := clampTableIndex(core.DoorStyleBuilding, len(assets.doorProps), core.DoorStyleBuilding)
		assets.doorProps[style].draw(center, 1, 0)
		return
	case previewCrystal:
		// The gem draws unlit (immediate-mode cylinders), as in-world — drop the
		// lighting shader around it, then restore for any later draws. Static angle
		// (no idle spin) so the browser's drag-to-rotate is the only rotation.
		rl.EndShaderMode()
		drawCrystalGem(rl.NewVector3(center.X, center.Y+crystalGeo.HalfHeight, center.Z), true, 0)
		rl.BeginShaderMode(assets.lighting.shader)
		return
	}
	char := item.Char
	if item.Kind == previewProp {
		if handler := inlinePropTable[char]; handler != nil {
			handler(assets, &core.AreaDefinition{}, 0, 0, center, 0)
			return
		}
		if pm := &assets.propModelTable[char]; pm.registered() {
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
	if dm := &assets.decorModelTable[char]; dm.registered() {
		dm.draw(center, 1, 0)
	}
}

// objectPreviewCamera frames bb, then orbits the eye by (yaw,pitch) added to the
// default three-quarter view; the pulled-back distance fits the bounding sphere in
// the FOV with margin. zoom>1 dollies closer.
func objectPreviewCamera(bb rl.BoundingBox, yaw, pitch, zoom float32) rl.Camera3D {
	const fovy = previewFovy
	center := rl.Vector3Scale(rl.Vector3Add(bb.Min, bb.Max), 0.5)
	radius := 0.5 * rl.Vector3Length(rl.Vector3Subtract(bb.Max, bb.Min))
	if radius < 0.35 {
		radius = 0.35
	}
	dist := radius / float32(math.Sin(float64(fovy)*0.5*degToRad64)) // sin(fov/2)
	dist *= 1.25                                                     // breathing room around the silhouette
	if zoom > 0 {
		dist /= zoom
	}
	az := float64(objectPreviewBaseYaw + yaw)
	el := float64(core.Clamp(objectPreviewBasePitch+pitch, objectPreviewMinPitch, objectPreviewMaxPitch))
	dir := rl.NewVector3(
		float32(math.Cos(el)*math.Sin(az)),
		float32(math.Sin(el)),
		float32(math.Cos(el)*math.Cos(az)),
	)
	return rl.Camera3D{
		Position:   rl.Vector3Add(center, rl.Vector3Scale(dir, dist)),
		Target:     center,
		Up:         worldUp,
		Fovy:       fovy,
		Projection: rl.CameraPerspective,
	}
}

// objectPreviewBounds returns world-space bounds for framing. Inline-handled objects map to the model the handler draws; falls back to a unit box.
func objectPreviewBounds(assets Resources, item ObjectPreviewItem) rl.BoundingBox {
	char := item.Char
	unit := rl.NewBoundingBox(rl.NewVector3(-0.5, 0, -0.5), rl.NewVector3(0.5, 1, 0.5))
	switch item.Kind {
	case previewChest:
		return rl.NewBoundingBox(rl.NewVector3(-0.4, 0, -0.35), rl.NewVector3(0.4, chestGeo.BodyHeight+chestGeo.LidHeight, 0.35))
	case previewDoor:
		style := clampTableIndex(core.DoorStyleBuilding, len(assets.doorProps), core.DoorStyleBuilding)
		return partsBoundsOr(assets.doorProps[style].models, assets.doorProps[style].parts, 1, unit)
	case previewCrystal:
		r, hh := crystalGeo.WaistRadius, crystalGeo.HalfHeight
		return rl.NewBoundingBox(rl.NewVector3(-r, 0, -r), rl.NewVector3(r, 2*hh+0.2, r))
	}
	if item.Kind == previewProp {
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
		if pm := &assets.propModelTable[char]; pm.registered() {
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
	if dm := &assets.decorModelTable[char]; dm.registered() {
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
