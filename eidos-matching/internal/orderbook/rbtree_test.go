// Package orderbook 红黑树单元测试
package orderbook

import (
	"math/rand"
	"testing"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestNewRBTree(t *testing.T) {
	// 测试升序树 (卖单)
	ascTree := NewRBTree(true)
	assert.Nil(t, ascTree.Root)
	assert.Equal(t, 0, ascTree.Size)

	// 测试降序树 (买单)
	descTree := NewRBTree(false)
	assert.Nil(t, descTree.Root)
	assert.Equal(t, 0, descTree.Size)
}

func TestRBTree_Insert_Ascending(t *testing.T) {
	tree := NewRBTree(true) // 升序 (卖单)

	prices := []float64{100, 102, 98, 105, 95, 101}
	for _, p := range prices {
		pl := model.NewPriceLevel(decimal.NewFromFloat(p))
		tree.Insert(pl)
	}

	assert.Equal(t, len(prices), tree.Size)
	assert.NotNil(t, tree.Root)

	// 验证升序排列: 最优价应该是最低价
	first := tree.First()
	assert.True(t, decimal.NewFromFloat(95).Equal(first.Price))
}

func TestRBTree_Insert_Descending(t *testing.T) {
	tree := NewRBTree(false) // 降序 (买单)

	prices := []float64{100, 102, 98, 105, 95, 101}
	for _, p := range prices {
		pl := model.NewPriceLevel(decimal.NewFromFloat(p))
		tree.Insert(pl)
	}

	assert.Equal(t, len(prices), tree.Size)

	// 验证降序排列: 最优价应该是最高价
	first := tree.First()
	assert.True(t, decimal.NewFromFloat(105).Equal(first.Price))
}

func TestRBTree_Find(t *testing.T) {
	tree := NewRBTree(true)

	prices := []float64{100, 102, 98}
	for _, p := range prices {
		pl := model.NewPriceLevel(decimal.NewFromFloat(p))
		tree.Insert(pl)
	}

	// 查找存在的价格
	found := tree.Find(decimal.NewFromFloat(100))
	assert.NotNil(t, found)
	assert.True(t, decimal.NewFromFloat(100).Equal(found.Price))

	// 查找不存在的价格
	notFound := tree.Find(decimal.NewFromFloat(99))
	assert.Nil(t, notFound)
}

func TestRBTree_Delete(t *testing.T) {
	tree := NewRBTree(true)

	pl1 := model.NewPriceLevel(decimal.NewFromFloat(100))
	pl2 := model.NewPriceLevel(decimal.NewFromFloat(102))
	pl3 := model.NewPriceLevel(decimal.NewFromFloat(98))

	tree.Insert(pl1)
	tree.Insert(pl2)
	tree.Insert(pl3)
	assert.Equal(t, 3, tree.Size)

	// 删除中间节点
	tree.Delete(pl1)
	assert.Equal(t, 2, tree.Size)
	assert.Nil(t, tree.Find(decimal.NewFromFloat(100)))

	// 删除后其他节点应该还在
	assert.NotNil(t, tree.Find(decimal.NewFromFloat(102)))
	assert.NotNil(t, tree.Find(decimal.NewFromFloat(98)))

	// 删除所有节点
	tree.Delete(pl2)
	tree.Delete(pl3)
	assert.Equal(t, 0, tree.Size)
	assert.True(t, tree.Empty())
}

func TestRBTree_InOrder_Ascending(t *testing.T) {
	tree := NewRBTree(true) // 升序

	prices := []float64{100, 102, 98, 105, 95}
	for _, p := range prices {
		tree.Insert(model.NewPriceLevel(decimal.NewFromFloat(p)))
	}

	// 中序遍历应该按升序返回
	var result []float64
	tree.InOrder(func(pl *model.PriceLevel) bool {
		f, _ := pl.Price.Float64()
		result = append(result, f)
		return true
	})

	expected := []float64{95, 98, 100, 102, 105}
	assert.Equal(t, expected, result)
}

func TestRBTree_InOrder_Descending(t *testing.T) {
	tree := NewRBTree(false) // 降序

	prices := []float64{100, 102, 98, 105, 95}
	for _, p := range prices {
		tree.Insert(model.NewPriceLevel(decimal.NewFromFloat(p)))
	}

	// 中序遍历应该按降序返回
	var result []float64
	tree.InOrder(func(pl *model.PriceLevel) bool {
		f, _ := pl.Price.Float64()
		result = append(result, f)
		return true
	})

	expected := []float64{105, 102, 100, 98, 95}
	assert.Equal(t, expected, result)
}

func TestRBTree_InOrder_EarlyStop(t *testing.T) {
	tree := NewRBTree(true)

	for i := 1; i <= 10; i++ {
		tree.Insert(model.NewPriceLevel(decimal.NewFromInt(int64(i))))
	}

	// 只获取前 3 个
	var result []int64
	tree.InOrder(func(pl *model.PriceLevel) bool {
		result = append(result, pl.Price.IntPart())
		return len(result) < 3
	})

	assert.Equal(t, []int64{1, 2, 3}, result)
}

func TestRBTree_EmptyAndLen(t *testing.T) {
	tree := NewRBTree(true)

	assert.True(t, tree.Empty())
	assert.Equal(t, 0, tree.Len())

	tree.Insert(model.NewPriceLevel(decimal.NewFromFloat(100)))
	assert.False(t, tree.Empty())
	assert.Equal(t, 1, tree.Len())
}

func TestRBTree_DuplicateInsert(t *testing.T) {
	tree := NewRBTree(true)

	pl := model.NewPriceLevel(decimal.NewFromFloat(100))
	tree.Insert(pl)
	tree.Insert(pl) // 重复插入

	// 应该只有一个节点
	assert.Equal(t, 1, tree.Size)
}

// 压力测试: 大量插入删除
func TestRBTree_Stress(t *testing.T) {
	tree := NewRBTree(true)

	// 插入 10000 个随机价格
	priceLevels := make([]*model.PriceLevel, 10000)
	for i := 0; i < 10000; i++ {
		price := decimal.NewFromFloat(rand.Float64() * 10000)
		pl := model.NewPriceLevel(price)
		priceLevels[i] = pl
		tree.Insert(pl)
	}

	// 删除一半
	for i := 0; i < 5000; i++ {
		tree.Delete(priceLevels[i])
	}

	// 验证剩余数量
	assert.Equal(t, 5000, tree.Size)

	// 验证中序遍历仍然有序
	var prev decimal.Decimal
	tree.InOrder(func(pl *model.PriceLevel) bool {
		if !prev.IsZero() {
			assert.True(t, pl.Price.GreaterThanOrEqual(prev), "InOrder should be sorted")
		}
		prev = pl.Price
		return true
	})
}

// 测试红黑树性质
func TestRBTree_Properties(t *testing.T) {
	tree := NewRBTree(true)

	// 插入多个价格
	prices := []float64{10, 20, 30, 40, 50, 25, 15, 35, 45, 5}
	for _, p := range prices {
		tree.Insert(model.NewPriceLevel(decimal.NewFromFloat(p)))
	}

	// 验证根节点是黑色
	assert.Equal(t, Black, tree.Root.Color, "Root should be black")

	// 验证红黑树性质
	blackHeight := verifyRBProperties(t, tree.Root, 0)
	assert.Greater(t, blackHeight, 0)
}

// 辅助函数: 验证红黑树性质
func verifyRBProperties(t *testing.T, node *RBNode, blackCount int) int {
	if node == nil {
		return blackCount
	}

	// 红色节点的子节点必须是黑色
	if node.Color == Red {
		if node.Left != nil {
			assert.Equal(t, Black, node.Left.Color, "Red node's left child should be black")
		}
		if node.Right != nil {
			assert.Equal(t, Black, node.Right.Color, "Red node's right child should be black")
		}
	}

	// 更新黑色计数
	if node.Color == Black {
		blackCount++
	}

	// 验证左右子树的黑高度相同
	leftBlackHeight := verifyRBProperties(t, node.Left, blackCount)
	rightBlackHeight := verifyRBProperties(t, node.Right, blackCount)

	if node.Left != nil && node.Right != nil {
		assert.Equal(t, leftBlackHeight, rightBlackHeight, "Black height should be equal")
	}

	return leftBlackHeight
}
