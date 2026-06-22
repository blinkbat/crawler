package core

import "slices"

// gridLayers returns pointers to the authored grid-layer slices in canonical
// order (walls, floor, decor, props, ceiling, elevation) — single enumeration
// for bulk ops, so a new layer is one row here. (The converters list fields explicitly.)
func (a *AreaDefinition) gridLayers() []*[]string {
	return []*[]string{&a.Walls, &a.Floor, &a.Decor, &a.Props, &a.Ceiling, &a.Elevation}
}

// layerBlank returns the open/blank char the loader fills an absent/short layer
// with: TileOpen for most, ceiling-open / elevation-ground for those two.
func (a *AreaDefinition) layerBlank(lp *[]string) byte {
	switch lp {
	case &a.Ceiling:
		return TileCeilingOpen
	case &a.Elevation:
		return ElevationGround
	default:
		return TileOpen
	}
}

// AreaContentEqual reports whether two areas have identical authorable content.
// Path is ignored: saving under a new file name must not mark data dirty.
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
		// Ceiling/Elevation are optional: an omitted layer must not read dirty vs
		// the loader's blank-filled form. Identify by pointer, compare absent==blank.
		if al[i] == &a.Ceiling || al[i] == &a.Elevation {
			if optionalLayerEqual(*al[i], *bl[i], a.Width, a.Height, a.layerBlank(al[i])) {
				continue
			}
		}
		return false
	}
	// Solids (a voxel stack) compared explicitly; solidsEqual uses
	// absent==derived-from-Elevation so a heightfield map (Solids nil) isn't dirty.
	if !solidsEqual(a, b) {
		return false
	}
	// PropLevels/DecorLevels compared explicitly (absent == all-auto).
	if !optionalLayerEqual(a.PropLevels, b.PropLevels, a.Width, a.Height, PropLevelAuto) ||
		!optionalLayerEqual(a.DecorLevels, b.DecorLevels, a.Width, a.Height, PropLevelAuto) {
		return false
	}
	if !faceOverridesEqual(a.FaceOverrides, b.FaceOverrides) {
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

// faceOverridesEqual compares face-override lists order-insensitively (they
// aren't sorted until encode), so a different authoring order isn't falsely dirty.
func faceOverridesEqual(a, b []FaceOverride) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[[2]int][4]byte, len(a))
	for _, o := range a {
		m[[2]int{o.X, o.Z}] = o.Skins
	}
	for _, o := range b {
		if s, ok := m[[2]int{o.X, o.Z}]; !ok || s != o.Skins {
			return false
		}
	}
	return true
}

// dialogsEqual deep-compares dialog lists. Nodes/choices carry a *DialogAction
// pointer, so those need a deref-aware walk rather than ==.
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

// optionalLayerEqual compares two optional grid layers, treating an absent/short
// layer as equal to a full (width,height) layer of the blank char.
func optionalLayerEqual(a, b []string, width, height int, blank byte) bool {
	return slices.Equal(
		normalizeOptionalLayer(a, width, height, blank),
		normalizeOptionalLayer(b, width, height, blank),
	)
}

// normalizeOptionalLayer returns a height×width layer, every row padded/truncated
// to width. An absent layer becomes a full blank layer; a ragged one is normalized.
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
	// Right row count: normalize each row's width vs the loader's full-width form.
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
		*dst[i] = cloneRows(*src[i])
	}
	// Solids isn't a gridLayers() member; CloneSolids is the nil-safe deep copy
	// (nil for an empty stack, so a heightfield area keeps Solids==nil).
	out.Solids = CloneSolids(a.Solids)
	if len(a.PropLevels) > 0 {
		out.PropLevels = cloneRows(a.PropLevels)
	}
	if len(a.DecorLevels) > 0 {
		out.DecorLevels = cloneRows(a.DecorLevels)
	}
	if len(a.FaceOverrides) > 0 {
		out.FaceOverrides = append([]FaceOverride(nil), a.FaceOverrides...)
	}
	// Drop the lazy face-override index so the clone rebuilds its own (the
	// `out := a` copy aliased the source map pointer).
	out.faceOverrideIdx = nil
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
	// CrystalSpawn is all-comparable, so a plain slice copy is a full deep copy.
	out.CrystalSpawns = append([]CrystalSpawn(nil), a.CrystalSpawns...)
	out.CustomEnemies = make([]CustomEnemyDef, len(a.CustomEnemies))
	for i, ce := range a.CustomEnemies {
		out.CustomEnemies[i] = ce
		out.CustomEnemies[i].Skills = append([]SkillID(nil), ce.Skills...)
	}
	out.Dialogs = CloneDialogs(a.Dialogs)
	// DialogTrigger is all-comparable, so a plain slice copy is a full deep copy.
	out.Triggers = append([]DialogTrigger(nil), a.Triggers...)
	return out
}

// CloneDialogs deep-copies a dialog list so the copy shares no backing slices/
// action pointers with the source.
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

// CloneDialogDef deep-copies a single definition (Nodes, Choices, Conditions,
// and the *DialogAction pointers) so the copy shares no mutable backing.
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
