package core

import "slices"

// gridLayers returns pointers to the area's six authored grid-layer
// slices in canonical order (walls, floor, decor, props, ceiling,
// elevation). The single place that enumerates the grid layers for bulk
// operations — CloneArea and AreaContentEqual walk this instead of
// hand-listing the fields, so a new layer is one row here, not several
// lockstep edits. (The MapFile↔Area converters still list the fields
// explicitly: they pair each layer with the separate mapfile.MapFile type
// and apply the ceiling / elevation blank-default, which this accessor
// can't model.)
func (a *AreaDefinition) gridLayers() []*[]string {
	return []*[]string{&a.Walls, &a.Floor, &a.Decor, &a.Props, &a.Ceiling, &a.Elevation}
}

// AreaContentEqual reports whether two areas have identical authorable
// content. Path is intentionally ignored: saving a map under a new file name
// should not mark the tile/entity data dirty.
func AreaContentEqual(a, b AreaDefinition) bool {
	if a.Name != b.Name || a.Width != b.Width || a.Height != b.Height ||
		a.Materials != b.Materials ||
		a.StartTileX != b.StartTileX || a.StartTileZ != b.StartTileZ ||
		a.StartFacing != b.StartFacing ||
		a.CrystalsAuthored != b.CrystalsAuthored ||
		a.QuietMessage != b.QuietMessage {
		return false
	}
	al, bl := a.gridLayers(), b.gridLayers()
	for i := range al {
		if slices.Equal(*al[i], *bl[i]) {
			continue
		}
		// Ceiling / Elevation are optional layers: the loader fills an
		// absent one with a canonical blank, so an area with them omitted
		// must not read as "dirty" vs one filled to that default. Identify
		// them by pointer (reorder-safe vs gridLayers' order) and compare
		// with absent==blank semantics.
		if al[i] == &a.Ceiling {
			if optionalLayerEqual(*al[i], *bl[i], a.Width, a.Height, TileCeilingOpen) {
				continue
			}
		} else if al[i] == &a.Elevation {
			if optionalLayerEqual(*al[i], *bl[i], a.Width, a.Height, ElevationGround) {
				continue
			}
		}
		return false
	}
	if !packSpawnsEqual(a.PackSpawns, b.PackSpawns) ||
		!chestSpawnsEqual(a.ChestSpawns, b.ChestSpawns) ||
		!slices.Equal(a.DoorSpawns, b.DoorSpawns) ||
		!slices.Equal(a.CrystalSpawns, b.CrystalSpawns) ||
		!customEnemiesEqual(a.CustomEnemies, b.CustomEnemies) ||
		!dialogsEqual(a.Dialogs, b.Dialogs) ||
		!slices.Equal(a.Triggers, b.Triggers) {
		return false
	}
	return true
}

// dialogsEqual deep-compares two areas' dialog lists. DialogChoiceCondition
// is an all-comparable struct (slices.Equal works), but nodes/choices carry a
// *DialogAction pointer, so those need a deref-aware walk rather than ==.
func dialogsEqual(a, b []DialogDefinition) bool {
	return slices.EqualFunc(a, b, func(ad, bd DialogDefinition) bool {
		return ad.ID == bd.ID && ad.StartNodeID == bd.StartNodeID &&
			slices.EqualFunc(ad.Nodes, bd.Nodes, dialogNodeEqual)
	})
}

func dialogNodeEqual(a, b DialogNode) bool {
	return a.ID == b.ID && a.SpeakerID == b.SpeakerID && a.Text == b.Text &&
		a.NextNodeID == b.NextNodeID && a.ContinueLabel == b.ContinueLabel &&
		a.IsMenuNode == b.IsMenuNode &&
		dialogActionEqual(a.EndAction, b.EndAction) &&
		slices.EqualFunc(a.Choices, b.Choices, dialogChoiceEqual)
}

func dialogChoiceEqual(a, b DialogChoice) bool {
	return a.ID == b.ID && a.Label == b.Label && a.NextNodeID == b.NextNodeID &&
		dialogActionEqual(a.EndAction, b.EndAction) &&
		slices.Equal(a.Conditions, b.Conditions)
}

func dialogActionEqual(a, b *DialogAction) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// optionalLayerEqual compares two optional grid layers (Ceiling, Elevation),
// treating an absent / short layer as equal to a full layer of (width,
// height) filled with the canonical blank char. Keeps the dirty-check stable
// when one side omits a layer the loader would have blank-filled.
func optionalLayerEqual(a, b []string, width, height int, blank byte) bool {
	return slices.Equal(
		normalizeOptionalLayer(a, width, height, blank),
		normalizeOptionalLayer(b, width, height, blank),
	)
}

// normalizeOptionalLayer returns a height×width layer with every row padded /
// truncated to width using the canonical blank char. An absent layer (wrong row
// count) becomes a full blank layer; a present-but-ragged layer (right row
// count, short rows) has each row normalized — so an absent layer and one the
// loader would blank-fill to the same shape compare equal rather than dirty.
func normalizeOptionalLayer(layer []string, width, height int, blank byte) []string {
	blankRow := func() string {
		buf := make([]byte, width)
		for i := range buf {
			buf[i] = blank
		}
		return string(buf)
	}
	if len(layer) != height {
		row := blankRow()
		out := make([]string, height)
		for i := range out {
			out[i] = row
		}
		return out
	}
	// Right row count: normalize each row's width so a ragged row doesn't read
	// as unequal vs the loader's blank-filled (full-width) form.
	ragged := false
	for _, r := range layer {
		if len(r) != width {
			ragged = true
			break
		}
	}
	if !ragged {
		return layer
	}
	out := make([]string, height)
	for i, r := range layer {
		buf := []byte(r)
		for len(buf) < width {
			buf = append(buf, blank)
		}
		out[i] = string(buf[:width])
	}
	return out
}

func packSpawnsEqual(a, b []PackSpawn) bool {
	return slices.EqualFunc(a, b, func(ap, bp PackSpawn) bool {
		return ap.TileX == bp.TileX && ap.TileZ == bp.TileZ &&
			ap.AI == bp.AI &&
			slices.Equal(ap.Members, bp.Members)
	})
}

func chestSpawnsEqual(a, b []ChestSpawn) bool {
	return slices.EqualFunc(a, b, func(ap, bp ChestSpawn) bool {
		return ap.TileX == bp.TileX && ap.TileZ == bp.TileZ &&
			slices.Equal(ap.Items, bp.Items)
	})
}

func customEnemiesEqual(a, b []CustomEnemyDef) bool {
	return slices.EqualFunc(a, b, func(ap, bp CustomEnemyDef) bool {
		return ap.Name == bp.Name && ap.BaseKind == bp.BaseKind &&
			ap.HP == bp.HP && ap.MP == bp.MP &&
			ap.Stats == bp.Stats && ap.Armor == bp.Armor && ap.MDef == bp.MDef &&
			ap.XPValue == bp.XPValue && ap.Tier == bp.Tier &&
			ap.AttackDamage == bp.AttackDamage &&
			ap.SkillCastChance == bp.SkillCastChance &&
			ap.SpellPower == bp.SpellPower &&
			slices.Equal(ap.Skills, bp.Skills)
	})
}

// CloneArea deep-copies an AreaDefinition for editor undo/redo snapshots.
func CloneArea(a AreaDefinition) AreaDefinition {
	out := a
	dst := out.gridLayers()
	src := a.gridLayers()
	for i := range dst {
		*dst[i] = append([]string(nil), *src[i]...)
	}
	out.PackSpawns = make([]PackSpawn, len(a.PackSpawns))
	for i, sp := range a.PackSpawns {
		out.PackSpawns[i] = PackSpawn{
			TileX:   sp.TileX,
			TileZ:   sp.TileZ,
			Members: append([]PackMemberRef(nil), sp.Members...),
			AI:      sp.AI,
		}
	}
	out.ChestSpawns = make([]ChestSpawn, len(a.ChestSpawns))
	for i, sp := range a.ChestSpawns {
		out.ChestSpawns[i] = ChestSpawn{
			TileX: sp.TileX,
			TileZ: sp.TileZ,
			Items: append([]ItemKind(nil), sp.Items...),
		}
	}
	out.DoorSpawns = append([]DoorSpawn(nil), a.DoorSpawns...)
	// CrystalSpawn is all-comparable (no nested slices), so a plain slice
	// copy is a full deep copy — matching the DoorSpawn line above.
	out.CrystalSpawns = append([]CrystalSpawn(nil), a.CrystalSpawns...)
	out.CustomEnemies = make([]CustomEnemyDef, len(a.CustomEnemies))
	for i, ce := range a.CustomEnemies {
		out.CustomEnemies[i] = ce
		out.CustomEnemies[i].Skills = append([]SkillID(nil), ce.Skills...)
	}
	out.Dialogs = CloneDialogs(a.Dialogs)
	// DialogTrigger is all-comparable (no slices/pointers), so a plain slice
	// copy is a full deep copy — no per-element walk needed (unlike Dialogs).
	out.Triggers = append([]DialogTrigger(nil), a.Triggers...)
	return out
}

// CloneDialogs deep-copies a dialog list so an editor undo snapshot (or a
// runtime StartDialog copy) shares no backing slices / action pointers with
// the source. Exported so StartDialog and the editor can take an independent
// copy through one helper.
func CloneDialogs(in []DialogDefinition) []DialogDefinition {
	if len(in) == 0 {
		return nil
	}
	out := make([]DialogDefinition, len(in))
	for i, d := range in {
		out[i] = CloneDialogDef(d)
	}
	return out
}

// CloneDialogDef deep-copies a single definition — its Nodes slice, each
// node's Choices slice, every Conditions slice, and the *DialogAction pointers
// — so the copy shares no mutable backing with the source. StartDialog uses it
// to take a self-contained runtime copy of the area's authored definition.
func CloneDialogDef(d DialogDefinition) DialogDefinition {
	out := DialogDefinition{
		ID:          d.ID,
		StartNodeID: d.StartNodeID,
		Nodes:       make([]DialogNode, len(d.Nodes)),
	}
	for j, n := range d.Nodes {
		cn := n
		cn.EndAction = cloneDialogAction(n.EndAction)
		if len(n.Choices) == 0 {
			cn.Choices = nil
		} else {
			cn.Choices = make([]DialogChoice, len(n.Choices))
			for k, c := range n.Choices {
				cc := c
				cc.EndAction = cloneDialogAction(c.EndAction)
				cc.Conditions = append([]DialogChoiceCondition(nil), c.Conditions...)
				cn.Choices[k] = cc
			}
		}
		out.Nodes[j] = cn
	}
	return out
}

func cloneDialogAction(a *DialogAction) *DialogAction {
	if a == nil {
		return nil
	}
	cp := *a
	return &cp
}
