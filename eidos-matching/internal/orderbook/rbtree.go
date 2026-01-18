// Package orderbook 实现订单簿数据结构
package orderbook

import (
	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/shopspring/decimal"
)

// Color 红黑树节点颜色
type Color bool

const (
	Red   Color = false
	Black Color = true
)

// RBNode 红黑树节点
type RBNode struct {
	PriceLevel *model.PriceLevel
	Color      Color
	Left       *RBNode
	Right      *RBNode
	Parent     *RBNode
}

// RBTree 红黑树
// 用于按价格索引订单
type RBTree struct {
	Root *RBNode
	Size int

	// 比较函数: 返回 -1 (a < b), 0 (a == b), 1 (a > b)
	// 买单: 价格降序 (高价优先)
	// 卖单: 价格升序 (低价优先)
	compare func(a, b decimal.Decimal) int
}

// NewRBTree 创建红黑树
// ascending: true 表示升序 (卖单), false 表示降序 (买单)
func NewRBTree(ascending bool) *RBTree {
	var compare func(a, b decimal.Decimal) int
	if ascending {
		compare = func(a, b decimal.Decimal) int {
			return a.Cmp(b)
		}
	} else {
		// 降序: 反转比较结果
		compare = func(a, b decimal.Decimal) int {
			return b.Cmp(a)
		}
	}
	return &RBTree{
		compare: compare,
	}
}

// Insert 插入价格档位
func (t *RBTree) Insert(pl *model.PriceLevel) {
	node := &RBNode{
		PriceLevel: pl,
		Color:      Red,
	}

	if t.Root == nil {
		node.Color = Black
		t.Root = node
		t.Size++
		return
	}

	// 二叉搜索树插入
	current := t.Root
	var parent *RBNode
	for current != nil {
		parent = current
		cmp := t.compare(pl.Price, current.PriceLevel.Price)
		if cmp < 0 {
			current = current.Left
		} else if cmp > 0 {
			current = current.Right
		} else {
			// 价格已存在，不应发生
			return
		}
	}

	node.Parent = parent
	if t.compare(pl.Price, parent.PriceLevel.Price) < 0 {
		parent.Left = node
	} else {
		parent.Right = node
	}

	t.Size++
	t.insertFixup(node)
}

// insertFixup 插入后修复红黑树性质
func (t *RBTree) insertFixup(node *RBNode) {
	for node.Parent != nil && node.Parent.Color == Red {
		if node.Parent == node.Parent.Parent.Left {
			uncle := node.Parent.Parent.Right
			if uncle != nil && uncle.Color == Red {
				// Case 1: 叔节点为红色
				node.Parent.Color = Black
				uncle.Color = Black
				node.Parent.Parent.Color = Red
				node = node.Parent.Parent
			} else {
				if node == node.Parent.Right {
					// Case 2: 叔节点为黑色，当前节点为右子节点
					node = node.Parent
					t.leftRotate(node)
				}
				// Case 3: 叔节点为黑色，当前节点为左子节点
				node.Parent.Color = Black
				node.Parent.Parent.Color = Red
				t.rightRotate(node.Parent.Parent)
			}
		} else {
			// 对称情况
			uncle := node.Parent.Parent.Left
			if uncle != nil && uncle.Color == Red {
				node.Parent.Color = Black
				uncle.Color = Black
				node.Parent.Parent.Color = Red
				node = node.Parent.Parent
			} else {
				if node == node.Parent.Left {
					node = node.Parent
					t.rightRotate(node)
				}
				node.Parent.Color = Black
				node.Parent.Parent.Color = Red
				t.leftRotate(node.Parent.Parent)
			}
		}
	}
	t.Root.Color = Black
}

// Delete 删除价格档位
func (t *RBTree) Delete(pl *model.PriceLevel) {
	node := t.findNode(pl.Price)
	if node == nil {
		return
	}
	t.deleteNode(node)
}

// findNode 查找节点
func (t *RBTree) findNode(price decimal.Decimal) *RBNode {
	current := t.Root
	for current != nil {
		cmp := t.compare(price, current.PriceLevel.Price)
		if cmp < 0 {
			current = current.Left
		} else if cmp > 0 {
			current = current.Right
		} else {
			return current
		}
	}
	return nil
}

// deleteNode 删除节点
func (t *RBTree) deleteNode(z *RBNode) {
	y := z
	yOriginalColor := y.Color
	var x *RBNode
	var xParent *RBNode

	if z.Left == nil {
		x = z.Right
		xParent = z.Parent
		t.transplant(z, z.Right)
	} else if z.Right == nil {
		x = z.Left
		xParent = z.Parent
		t.transplant(z, z.Left)
	} else {
		y = t.minimum(z.Right)
		yOriginalColor = y.Color
		x = y.Right
		if y.Parent == z {
			xParent = y
		} else {
			xParent = y.Parent
			t.transplant(y, y.Right)
			y.Right = z.Right
			y.Right.Parent = y
		}
		t.transplant(z, y)
		y.Left = z.Left
		y.Left.Parent = y
		y.Color = z.Color
	}

	t.Size--
	if yOriginalColor == Black {
		t.deleteFixup(x, xParent)
	}
}

// deleteFixup 删除后修复红黑树性质
func (t *RBTree) deleteFixup(x *RBNode, xParent *RBNode) {
	for x != t.Root && (x == nil || x.Color == Black) {
		if x == xParent.Left {
			w := xParent.Right
			if w != nil && w.Color == Red {
				w.Color = Black
				xParent.Color = Red
				t.leftRotate(xParent)
				w = xParent.Right
			}
			if w == nil || ((w.Left == nil || w.Left.Color == Black) &&
				(w.Right == nil || w.Right.Color == Black)) {
				if w != nil {
					w.Color = Red
				}
				x = xParent
				xParent = x.Parent
			} else {
				if w.Right == nil || w.Right.Color == Black {
					if w.Left != nil {
						w.Left.Color = Black
					}
					w.Color = Red
					t.rightRotate(w)
					w = xParent.Right
				}
				w.Color = xParent.Color
				xParent.Color = Black
				if w.Right != nil {
					w.Right.Color = Black
				}
				t.leftRotate(xParent)
				x = t.Root
			}
		} else {
			w := xParent.Left
			if w != nil && w.Color == Red {
				w.Color = Black
				xParent.Color = Red
				t.rightRotate(xParent)
				w = xParent.Left
			}
			if w == nil || ((w.Right == nil || w.Right.Color == Black) &&
				(w.Left == nil || w.Left.Color == Black)) {
				if w != nil {
					w.Color = Red
				}
				x = xParent
				xParent = x.Parent
			} else {
				if w.Left == nil || w.Left.Color == Black {
					if w.Right != nil {
						w.Right.Color = Black
					}
					w.Color = Red
					t.leftRotate(w)
					w = xParent.Left
				}
				w.Color = xParent.Color
				xParent.Color = Black
				if w.Left != nil {
					w.Left.Color = Black
				}
				t.rightRotate(xParent)
				x = t.Root
			}
		}
	}
	if x != nil {
		x.Color = Black
	}
}

// transplant 用 v 替换 u
func (t *RBTree) transplant(u, v *RBNode) {
	if u.Parent == nil {
		t.Root = v
	} else if u == u.Parent.Left {
		u.Parent.Left = v
	} else {
		u.Parent.Right = v
	}
	if v != nil {
		v.Parent = u.Parent
	}
}

// leftRotate 左旋
func (t *RBTree) leftRotate(x *RBNode) {
	y := x.Right
	x.Right = y.Left
	if y.Left != nil {
		y.Left.Parent = x
	}
	y.Parent = x.Parent
	if x.Parent == nil {
		t.Root = y
	} else if x == x.Parent.Left {
		x.Parent.Left = y
	} else {
		x.Parent.Right = y
	}
	y.Left = x
	x.Parent = y
}

// rightRotate 右旋
func (t *RBTree) rightRotate(x *RBNode) {
	y := x.Left
	x.Left = y.Right
	if y.Right != nil {
		y.Right.Parent = x
	}
	y.Parent = x.Parent
	if x.Parent == nil {
		t.Root = y
	} else if x == x.Parent.Right {
		x.Parent.Right = y
	} else {
		x.Parent.Left = y
	}
	y.Right = x
	x.Parent = y
}

// minimum 获取子树最小节点
func (t *RBTree) minimum(node *RBNode) *RBNode {
	for node.Left != nil {
		node = node.Left
	}
	return node
}

// Find 查找价格档位
func (t *RBTree) Find(price decimal.Decimal) *model.PriceLevel {
	node := t.findNode(price)
	if node != nil {
		return node.PriceLevel
	}
	return nil
}

// First 获取第一个价格档位 (最优价)
// 买单树: 最高价 (实际是树的最小节点，因为使用降序比较)
// 卖单树: 最低价 (树的最小节点)
func (t *RBTree) First() *model.PriceLevel {
	if t.Root == nil {
		return nil
	}
	node := t.minimum(t.Root)
	return node.PriceLevel
}

// Empty 树是否为空
func (t *RBTree) Empty() bool {
	return t.Root == nil
}

// Len 返回节点数量
func (t *RBTree) Len() int {
	return t.Size
}

// InOrder 中序遍历
func (t *RBTree) InOrder(fn func(pl *model.PriceLevel) bool) {
	t.inOrderTraversal(t.Root, fn)
}

func (t *RBTree) inOrderTraversal(node *RBNode, fn func(pl *model.PriceLevel) bool) bool {
	if node == nil {
		return true
	}
	if !t.inOrderTraversal(node.Left, fn) {
		return false
	}
	if !fn(node.PriceLevel) {
		return false
	}
	return t.inOrderTraversal(node.Right, fn)
}
