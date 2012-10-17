package layout

import (
	"fmt"
	"math"

	"github.com/BurntSushi/xgbutil/xrect"

	"github.com/BurntSushi/wingo/misc"
)

const (
	fullPortion proportion = 1.0
	epsilon                = 0.0001
)

type proportion float64

// portion takes a proportion of size.
func (p proportion) portion(size int) int {
	return misc.Round(float64(proportion(size) * p))
}

func (p1 proportion) equal(p2 proportion) bool {
	return math.Abs(float64(p1-p2)) < epsilon
}

type tree struct {
	child node
}

type node interface {
	MoveResize(x, y, width, height int)
	Proportion() proportion
	SetProportion(p proportion)
	Parent() node
	SetParent(n node)
	ValidDims(w, h, minw, minh, maxw, maxh int) bool
	VisitLeafNodes(f func(lf *leaf) bool) bool
}

type hsplit struct {
	split
}

type vsplit struct {
	split
}

type split struct {
	parent   node
	children []node
	prop     proportion
}

type leaf struct {
	parent node // can never be a leaf
	client Client
	prop   proportion
}

func newTree() *tree {
	return &tree{
		child: nil,
	}
}

func (t *tree) place(geom xrect.Rect) {
	if t.child == nil {
		return
	}

	x, y, w, h := geom.X(), geom.Y(), geom.Width(), geom.Height()
	if !t.child.ValidDims(w, h, 1, 1, w, h) {
		return
	}
	t.child.MoveResize(x, y, w, h)
}

func (t *tree) setChild(n node) {
	t.child = n
}

func (t *tree) switchClients(c1, c2 Client) {
	lf1 := t.findLeaf(c1)
	lf2 := t.findLeaf(c2)
	if lf1 == nil || lf2 == nil {
		return
	}
	lf1.client, lf2.client = lf2.client, lf1.client
}

func (t *tree) findLeaf(c Client) *leaf {
	if t.child == nil {
		return nil
	}
	var lf *leaf
	t.child.VisitLeafNodes(func(visit *leaf) bool {
		if visit.client == c {
			lf = visit
			return false // stop visiting
		}
		return true // keep going
	})
	return lf
}

func newLeaf(parent node, client Client) *leaf {
	return &leaf{
		parent: parent,
		client: client,
	}
}

func newHSplit(parent node) *hsplit {
	return &hsplit{split{
		parent:   parent,
		children: make([]node, 0),
	}}
}

func newVSplit(parent node) *vsplit {
	return &vsplit{split{
		parent:   parent,
		children: make([]node, 0),
	}}
}

func (s *split) Proportion() proportion {
	return s.prop
}

func (s *split) SetProportion(p proportion) {
	s.prop = p
}

func (s *split) checkPortions() {
	if len(s.children) == 0 {
		return
	}

	sum := proportion(0)
	for _, child := range s.children {
		sum += child.Proportion()
	}
	if !sum.equal(fullPortion) {
		panic(fmt.Sprintf("portions not equal: %f != %f", sum, fullPortion))
	}
}

func (s *split) Parent() node {
	return s.parent
}

func (s *split) SetParent(n node) {
	s.parent = n
}

func (s *split) VisitLeafNodes(f func(lf *leaf) bool) bool {
	for _, child := range s.children {
		if !child.VisitLeafNodes(f) {
			return false
		}
	}
	return true
}

func (s *split) addNode(n node, last bool) {
	// Get the proportion of the new leaf.
	newProp := fullPortion / proportion(len(s.children)+1)

	// Now push everything else over by an even amount.
	if len(s.children) > 0 {
		chop := newProp / proportion(len(s.children))
		for _, child := range s.children {
			child.SetProportion(child.Proportion() - chop)
		}
	}

	n.SetProportion(newProp)

	if last {
		s.children = append(s.children, n)
	} else {
		s.children = append([]node{n}, s.children...)
	}

	s.checkPortions()
}

func (s *split) removeNode(n node) {
	// Remove it from the list of children.
	removed := false
	for i, child := range s.children {
		if child == n {
			s.children = append(s.children[:i], s.children[i+1:]...)
			removed = true
		}
	}
	if !removed {
		panic(fmt.Sprintf("The node '%s' is not in the split '%s'.", n, s))
	}

	// Distribute this node's portion to the rest.
	if len(s.children) > 0 {
		leftovers := n.Proportion() / proportion(len(s.children))
		for _, child := range s.children {
			child.SetProportion(child.Proportion() + leftovers)
		}

		s.checkPortions()
	}
}

func (hs *hsplit) MoveResize(x, y, width, height int) {
	// In hsplits, y and height remain constant. Width varies based on the
	// proportion, and x is derived from width.
	nextx := x
	for _, child := range hs.children {
		w := child.Proportion().portion(width)
		child.MoveResize(nextx, y, w, height)
		nextx += w
	}
}

func (hs *hsplit) ValidDims(w, h, minw, minh, maxw, maxh int) bool {
	for _, child := range hs.children {
		childw := child.Proportion().portion(w)
		if !child.ValidDims(childw, h, minw, minh, maxw, maxh) {
			return false
		}
	}
	return true
}

func (vs *vsplit) MoveResize(x, y, width, height int) {
	// In vsplits, x and width remain constant. Height varies based on the
	// proportion, and y is derived from height.
	nexty := y
	for _, child := range vs.children {
		h := child.Proportion().portion(height)
		child.MoveResize(x, nexty, width, h)
		nexty += h
	}
}

func (vs *vsplit) ValidDims(w, h, minw, minh, maxw, maxh int) bool {
	for _, child := range vs.children {
		childh := child.Proportion().portion(h)
		if !child.ValidDims(w, childh, minw, minh, maxw, maxh) {
			return false
		}
	}
	return true
}

func (lf *leaf) MoveResize(x, y, width, height int) {
	lf.client.FrameTile()
	lf.client.MoveResize(x, y, width, height)
}

func (lf *leaf) Proportion() proportion {
	return lf.prop
}

func (lf *leaf) SetProportion(p proportion) {
	lf.prop = p
}

func (lf *leaf) Parent() node {
	return lf.parent
}

func (lf *leaf) SetParent(n node) {
	lf.parent = n
}

func (lf *leaf) ValidDims(w, h, minw, minh, maxw, maxh int) bool {
	return w >= minw && h >= minh && w <= maxw && h <= maxh
}

func (lf *leaf) VisitLeafNodes(f func(visit *leaf) bool) bool {
	return f(lf)
}