package render

import (
	"crawler/internal/app/core"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Skill-tree modal raised from the Skills tab. Paints the member's trees as
// side-by-side node columns + a detail strip. Investing a node's first rank
// learns its skill; further ranks upgrade it (core.BuySkillNode / LearnedSkills).

// Tree-column geometry; card sized to fit these, clamped on a small screen.
const (
	skillTreeColW    = float32(248)
	skillTreeColGap  = float32(50)
	skillTreeSidePad = float32(34)
	// skillTreeCardInset: content gutter for header/footer/detail; width derives as 2×.
	skillTreeCardInset    = float32(24)
	skillTreeHeaderGlyphX = float32(34)
	skillTreeHeaderTitleX = float32(58)
	skillNodeColHeaderH   = float32(32) // header reserved above the ladder
	skillNodeGap          = float32(12)
	skillNodeMaxH         = float32(82) // height cap so short trees don't stretch
	skillNodeMinH         = float32(8)  // height floor so many nodes can't go negative
	// Ornament size gates: corner pips need a min size, the bottom fleuron a min width.
	skillNodePipMinW     = float32(96)
	skillNodePipMinH     = float32(40)
	skillNodeFleuronMinW = float32(130)
	// Card sized inline (custom column geometry); chrome routes through drawPickerCardEx
	// with an opaque backdrop so the glass body composites over solid dark.
	skillTreeMaxWidthFrac  = float32(0.92) // columns shrink to fit rather than overflow
	skillTreeHeightFrac    = float32(0.74)
	skillTreeHeaderTitleY  = float32(24)
	skillTreeHeaderGlyphY  = float32(38) // crest lower, off the mitre
	skillTreeHeaderSPY     = float32(28)
	skillTreeBodyTopY      = float32(74)
	skillTreeBodyBottomPad = float32(22)
	skillTreeDetailH       = float32(84)
	skillTreeFooterH       = float32(42)
)

// DrawSkillTreeModal paints the skill-tree modal over the panels overlay. Reads
// SkillTreeMember/Col/Row for the cursor (driven by explore/skilltree.go).
func DrawSkillTreeModal(g *core.GameState, assets Resources) {
	if !g.SkillTreeOpen {
		return
	}
	font := assets.Font()
	member := g.SkillTreeMember
	if member < 0 || member >= len(g.Party) {
		return
	}
	m := g.Party[member]
	trees := core.SkillTreesFor(m.Class)
	if len(trees) == 0 {
		return
	}

	n := len(trees)
	sw, sh := screenSizeF()
	colW := skillTreeColW
	cardW := skillTreeColW*float32(n) + skillTreeColGap*float32(n-1) + skillTreeSidePad*2
	if maxW := sw * skillTreeMaxWidthFrac; cardW > maxW {
		// Tiny screen: shrink columns to fit.
		cardW = maxW
		colW = (cardW - skillTreeSidePad*2 - skillTreeColGap*float32(n-1)) / float32(n)
	}
	cardH := sh * skillTreeHeightFrac

	// Shared picker chrome with an opaque backdrop first (the body must composite over
	// solid dark, not the lit scene) + the title shifted right of the class crest.
	card := drawPickerCardEx(font, cardW, cardH, m.Name+" — Skill Trees",
		skillTreeHeaderTitleX, skillTreeHeaderTitleY, true)

	// Header: class crest left of the title, SkillPoint balance right.
	classCol := classAccent(m.Class)
	drawClassGlyph(card.X+skillTreeHeaderGlyphX, card.Y+skillTreeHeaderGlyphY, 12, m.Class, classCol)
	spText := skillPointsLabel(m.SkillPoints)
	spCol := accentIfPositive(m.SkillPoints, inkAccent)
	drawTextRightAligned(font, spText, card.X+card.Width-skillTreeCardInset, card.Y+skillTreeHeaderSPY, FontBody, spCol)

	// Tree-column body above the detail strip; columns centered as a block.
	bodyTop := card.Y + skillTreeBodyTopY
	bodyBottom := card.Y + card.Height - skillTreeDetailH - skillTreeFooterH - skillTreeBodyBottomPad
	blockW := colW*float32(n) + skillTreeColGap*float32(n-1)
	startX := card.X + (card.Width-blockW)/2
	for ti, tr := range trees {
		colX := startX + float32(ti)*(colW+skillTreeColGap)
		drawSkillTreeColumn(font, g, &m, tr, ti, colX, bodyTop, colW, bodyBottom-bodyTop)
	}

	drawSkillTreeDetail(font, g, &m, trees, card, skillTreeDetailH, skillTreeFooterH)

	drawModalFooterGlyphsLeft(font, card, card.X+skillTreeCardInset, []HintSeg{
		Hint("Tree", GlyphLeftRight),
		Hint("Node", GlyphUpDown),
		Hint("Invest", GlyphA),
		Hint("Close", GlyphB),
	})
}

// drawSkillTreeColumn paints one tree: header, then the node ladder with
// connector lines (lit when the upper node holds a rank). treeIdx vs the cursor
// column selects the focused node's frame.
func drawSkillTreeColumn(font rl.Font, g *core.GameState, m *core.PartyMember, tr core.SkillTreeDef, treeIdx int, x, y, w, h float32) {
	drawTextWithShadow(font, tr.Name, x+6, y, FontBody, textPrimary)
	drawGiltRule(int32(x+6), int32(y+24), int32(w-12), 2, 0.8)

	nodes := tr.Nodes
	n := len(nodes)
	if n == 0 {
		return
	}
	nodesTop := y + skillNodeColHeaderH
	nodeGap := skillNodeGap
	nodeH := (h - skillNodeColHeaderH - nodeGap*float32(n-1)) / float32(n)
	if nodeH > skillNodeMaxH {
		nodeH = skillNodeMaxH
	}
	if nodeH < skillNodeMinH {
		nodeH = skillNodeMinH // floor so many nodes can't yield negative-height plates
	}
	step := nodeH + nodeGap

	// Connector lines first so node panes paint over their ends.
	cx := x + w/2
	for i := 0; i < n-1; i++ {
		ay := nodesTop + float32(i)*step + nodeH
		by := nodesTop + float32(i+1)*step
		lineCol := fadeColor(giltBright, 0.18)
		if core.TreeNodeRank(m, nodes[i].ID) >= 1 {
			lineCol = fadeColor(giltBright, 0.7)
		}
		rl.DrawRectangle(int32(cx-1), int32(ay), 2, int32(by-ay), lineCol)
		drawDiamondPip(cx, ay, 2.2, lineCol)
		drawDiamondPip(cx, by, 2.2, lineCol)
	}

	for i, node := range nodes {
		ny := nodesTop + float32(i)*step
		rect := rl.NewRectangle(x, ny, w, nodeH)
		focused := treeIdx == g.SkillTreeCol && i == g.SkillTreeRow
		drawSkillTreeNode(font, m, node, rect, focused)
	}
}

// drawSkillTreeNode paints one node: state-tinted glass pane (locked/invested/
// available), name, rank pips, and a status chip (cost / MAX / locked).
func drawSkillTreeNode(font rl.Font, m *core.PartyMember, node core.SkillTreeNode, rect rl.Rectangle, focused bool) {
	rank := core.TreeNodeRank(m, node.ID)
	unlocked := core.SkillNodeUnlocked(m, node)
	maxed := core.SkillNodeMaxed(m, node)

	bg := glassMid
	switch {
	case !unlocked:
		bg = fadeColor(glassDeep, 0.6)
	case rank > 0:
		bg = selectedGlassTint(glassMid, 0.5)
	}
	drawSkillNodePlate(rect, bg, rank, unlocked, focused)
	if focused {
		drawGiltFocusRing(rect)
	}

	nameCol := rowTextColor(unlocked, !unlocked, textDim)
	drawTextWithShadow(font, node.Name, rect.X+10, rect.Y+8, FontSmall, nameCol)
	drawSkillTierPips(rect.X+10, rect.Y+rect.Height-14, rank, node.MaxRank)

	var chip string
	chipCol := textDim
	switch {
	case maxed:
		chip = "MAX"
		chipCol = fadeColor(giltBright, 0.85)
	case !unlocked:
		chip = "locked"
		chipCol = textDim
	default:
		chip = skillPointsLabel(node.Cost)
		chipCol = giltBright
		if m.SkillPoints < node.Cost {
			chipCol = textMuted
		}
	}
	drawTextRightAligned(font, chip, rect.X+rect.Width-10, rect.Y+rect.Height-16, FontTiny, chipCol)
}

// drawSkillTierPips paints `total` diamond pips at (x, y), the first `filled`
// bright, the rest dim — a compact "2 of 3 bought" read.
func drawSkillTierPips(x, y float32, filled, total int) {
	const pipR = float32(5)
	const pipGap = float32(16)
	for i := 0; i < total; i++ {
		cx := x + pipR + float32(i)*pipGap
		col := fadeColor(giltBright, 0.22)
		if i < filled {
			col = giltBright
		}
		drawDiamondPip(cx, y, pipR, col)
	}
}

func drawSkillNodePlate(rect rl.Rectangle, bg rl.Color, rank int, unlocked, focused bool) {
	drawPaneDropShadow(rect)
	drawGlassPaneRect(rect, bg)
	outline := woodAccentOutline
	if !unlocked {
		outline = fadeColor(borderDim, 0.62)
	}
	if rank > 0 {
		outline = fadeColor(giltDim, 0.72)
	}
	if focused {
		outline = fadeColor(giltBright, 0.90)
	}
	// Rounded outline matching the glass body's corner radius (was square-cornered).
	drawPanelOutline(int32(rect.X), int32(rect.Y), int32(rect.Width), int32(rect.Height), outline)
	if rect.Width >= skillNodePipMinW && rect.Height >= skillNodePipMinH {
		pip := fadeColor(outline, 0.82)
		drawDiamondPip(rect.X+8, rect.Y+8, 1.8, pip)
		drawDiamondPip(rect.X+rect.Width-8, rect.Y+8, 1.8, pip)
		drawDiamondPip(rect.X+8, rect.Y+rect.Height-8, 1.8, pip)
		drawDiamondPip(rect.X+rect.Width-8, rect.Y+rect.Height-8, 1.8, pip)
	}
	if rank > 0 && rect.Width >= skillNodeFleuronMinW {
		drawFleuron(rect.X+rect.Width/2, rect.Y+rect.Height-12, 2.1, fadeColor(giltDim, 0.42))
	}
}

// drawSkillTreeDetail paints the bottom strip for the focused node: name + rank,
// blurb, and a state footer (locked-because / fully-invested / invest prompt).
func drawSkillTreeDetail(font rl.Font, g *core.GameState, m *core.PartyMember, trees []core.SkillTreeDef, card rl.Rectangle, detailH, footerH float32) {
	col := g.SkillTreeCol
	row := g.SkillTreeRow
	if col < 0 || col >= len(trees) {
		return
	}
	nodes := trees[col].Nodes
	if row < 0 || row >= len(nodes) {
		return
	}
	node := nodes[row]

	x := card.X + skillTreeCardInset
	w := card.Width - skillTreeCardInset*2
	y := card.Y + card.Height - footerH - detailH - 4
	drawGlassPane(int32(x), int32(y), int32(w), int32(detailH), glassDeep)

	md := cardDetailRowMetrics
	drawTextWithShadow(font, node.Name, x+md.insetX, y+md.titleY, FontBody, textPrimary)
	rank := core.TreeNodeRank(m, node.ID)
	state := "Rank " + formatRatioSpaced(rank, node.MaxRank)
	drawTextRightAligned(font, state, x+w-md.insetX, y+md.valueY, FontSmall, inkAccent)

	drawTextWithShadow(font, node.Desc, x+md.insetX, y+md.subY, FontSmall, textDim)

	var foot string
	footCol := textMuted
	switch {
	case !core.SkillNodeUnlocked(m, node):
		foot = "Locked"
		if req, ok := core.SkillTreeNodeName(m.Class, node.Requires); ok {
			foot = "Locked — requires " + req
		}
	case core.SkillNodeMaxed(m, node):
		foot = "Fully invested"
		footCol = fadeColor(giltBright, 0.8)
	case m.SkillPoints < node.Cost:
		foot = "Not enough skill points"
	default:
		foot = "Confirm to invest (" + skillPointsLabel(node.Cost) + ")"
		footCol = giltBright
	}
	drawTextWithShadow(font, foot, x+md.insetX, y+detailH-22, FontSmall, footCol)
}
