package render

import (
	"crawler/internal/app/core"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Skill-tree modal: the Diablo-2-style sub-dialog raised from the Skills
// tab (Confirm on a party member). It paints the member's three trees as
// side-by-side columns of stacked nodes with connector lines, rank pips,
// and lock / available / maxed states, plus a detail strip describing the
// focused node. Investing a node's first rank LEARNS its skill (it appears
// in the battle Skill menu) and further ranks upgrade it through the
// SkillTiers ladder (see core.BuySkillNode / core.LearnedSkills).

// Tree-column geometry: narrow columns with generous spacing between them.
// The card is sized to fit exactly these (smaller than a screen fraction),
// then clamped down on a small screen.
const (
	skillTreeColW    = float32(248) // narrow tree column
	skillTreeColGap  = float32(50)  // generous spacing between trees
	skillTreeSidePad = float32(34)  // side gutter — kept roomy so columns never kiss the frame
	// skillTreeCardInset is the modal's content gutter — the left/right margin
	// the header balance, footer hints, and detail strip all inset by. One
	// token so those uses (and the width derivation, 2× it) can't drift.
	skillTreeCardInset = float32(24)
	// Header left gutters: the class glyph sits in from the frame, the title
	// past it (kept off the wood mitre — the "breathing room" pass).
	skillTreeHeaderGlyphX = float32(34)
	skillTreeHeaderTitleX = float32(58)
	// Per-column node-ladder geometry (drawSkillTreeColumn). Named so a "nodes
	// too tall / too cramped" tune is one edit, matching skillTreeColW/Gap above.
	skillNodeColHeaderH = float32(32) // tree-name + gilt-rule header reserved above the ladder
	skillNodeGap        = float32(12) // vertical gap between node plates
	skillNodeMaxH       = float32(82) // node-plate height cap so short trees don't stretch
	// Ornament size gates: a node plate earns corner pips only once it's at
	// least this big, and a ranked plate earns the bottom fleuron only when wide.
	skillNodePipMinW     = float32(96)
	skillNodePipMinH     = float32(40)
	skillNodeFleuronMinW = float32(130)
	// Modal screen-fraction sizing — this is the only modal sized inline (it
	// paints an opaque backdrop before drawVeiledCard, so it can't reuse the
	// shared drawScreenFractionScaffold). Named to match the
	// *ModalWidthFrac/HeightFrac token family the other modals use.
	skillTreeMaxWidthFrac = float32(0.92) // columns shrink to fit rather than overflow this
	skillTreeHeightFrac   = float32(0.74) // card height as a fraction of the screen
	// Header / body vertical offsets from the card top (the "breathing room" pass).
	skillTreeHeaderTitleY  = float32(24) // title baseline
	skillTreeHeaderGlyphY  = float32(38) // class crest baseline (lower, off the mitre)
	skillTreeHeaderSPY     = float32(28) // right-aligned skill-point balance baseline
	skillTreeBodyTopY      = float32(74) // tree-column block top
	skillTreeBodyBottomPad = float32(22) // gap above the detail strip + footer
	skillTreeDetailH       = float32(84) // detail strip height
	skillTreeFooterH       = float32(42) // footer hint-bar reserve
)

// DrawSkillTreeModal paints the skill-tree modal on top of the panels
// overlay. No-op when the modal isn't open or the focused member is out of
// range. Reads SkillTreeMember / SkillTreeCol / SkillTreeRow for the
// cursor; the input side (explore/skilltree.go) drives those.
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
		// Tiny screen: shrink the columns to fit rather than overflow.
		cardW = maxW
		colW = (cardW - skillTreeSidePad*2 - skillTreeColGap*float32(n-1)) / float32(n)
	}
	cardH := sh * skillTreeHeightFrac

	// Opaque backdrop painted at the card's spot BEFORE the veiled card, so
	// the glass body composites over solid dark instead of the world/overlay
	// behind it. drawVeiledCard centers the card identically, so the rects
	// align; its wood frame + corner filigree still draw on top.
	_, screenH := screenSize()
	cw, ch := int32(cardW), int32(cardH)
	bx := centerX(cw)
	by := screenH/2 - ch/2
	backdrop := rl.NewRectangle(float32(bx), float32(by), float32(cw), float32(ch))
	rl.DrawRectangleRounded(backdrop, fixedRoundnessFor(cw, ch, cornerRadius), 8, surfaceCardBackdrop)
	card := drawVeiledCard(cw, ch, borderActive, woodAccent, woodAccent)

	// Header: class crest + "<name> — Skill Trees" on the left, the
	// spendable SkillPoint balance on the right.
	classCol := classAccent(m.Class)
	// Header sits lower off the top frame (was +16/+30) so the title isn't
	// crowding the wood mitre — part of the "more breathing room" pass.
	drawClassGlyph(card.X+skillTreeHeaderGlyphX, card.Y+skillTreeHeaderGlyphY, 12, m.Class, classCol)
	drawEngravedText(font, m.Name+" — Skill Trees", card.X+skillTreeHeaderTitleX, card.Y+skillTreeHeaderTitleY, FontHeading, textPrimary)
	spText := skillPointsLabel(m.SkillPoints)
	spCol := textMuted
	if m.SkillPoints > 0 {
		spCol = inkAccent
	}
	drawTextRightAligned(font, spText, card.X+card.Width-skillTreeCardInset, card.Y+skillTreeHeaderSPY, FontBody, spCol)

	// Body region for the tree columns, above the detail strip. Columns are
	// a fixed narrow width, centered as a block so the spacing reads evenly.
	// Deeper footer reserve + a wider gap above it keep the hint bar and the
	// detail strip off the bottom frame (the "too tight / too cluttered" pass).
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

// drawSkillTreeColumn paints one tree as a labelled column: name + gilt
// rule header, then the node ladder with vertical connector lines (lit
// when the upper node holds a rank). treeIdx is compared against the
// cursor's column so the focused node in the active column draws its frame.
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
	step := nodeH + nodeGap

	// Connector lines first so the node panes paint over their ends.
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

// drawSkillTreeNode paints a single tree node: a glass pane tinted by
// state (locked = dim, invested = warm, available = neutral), the node
// name, a rank-pip strip, and a right-aligned status chip (cost / MAX /
// locked). The focused node gets the shared bright gilt frame.
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

func drawSkillNodePlate(rect rl.Rectangle, bg rl.Color, rank int, unlocked, focused bool) {
	drawPaneDropShadow(rect)
	drawGlassPane(int32(rect.X), int32(rect.Y), int32(rect.Width), int32(rect.Height), bg)
	outline := fadeColor(woodAccent, 0.42)
	if !unlocked {
		outline = fadeColor(borderDim, 0.62)
	}
	if rank > 0 {
		outline = fadeColor(giltDim, 0.72)
	}
	if focused {
		outline = fadeColor(giltBright, 0.90)
	}
	rl.DrawRectangleLinesEx(rect, 1, outline)
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

// drawSkillTreeDetail paints the bottom strip describing the focused node:
// its name + current rank on the top line, its full blurb below, and a
// state footer (locked-because, fully-invested, or the invest prompt).
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

	drawTextWithShadow(font, node.Name, x+12, y+8, FontBody, textPrimary)
	rank := core.TreeNodeRank(m, node.ID)
	state := "Rank " + strconv.Itoa(rank) + " / " + strconv.Itoa(node.MaxRank)
	drawTextRightAligned(font, state, x+w-12, y+10, FontSmall, inkAccent)

	drawTextWithShadow(font, node.Desc, x+12, y+34, FontSmall, textHint)

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
	drawTextWithShadow(font, foot, x+12, y+detailH-22, FontSmall, footCol)
}
