package decimal

import (
	"database/sql/driver"
	"fmt"

	"github.com/shopspring/decimal"
)

// Decimal 高精度数值类型
type Decimal struct {
	decimal.Decimal
}

var Zero = Decimal{decimal.Zero}

// NewFromString 从字符串创建
func NewFromString(s string) (Decimal, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return Zero, err
	}
	return Decimal{d}, nil
}

// MustFromString 从字符串创建，失败 panic
func MustFromString(s string) Decimal {
	d, err := NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

// NewFromInt 从整数创建
func NewFromInt(i int64) Decimal {
	return Decimal{decimal.NewFromInt(i)}
}

// NewFromFloat 从浮点数创建
func NewFromFloat(f float64) Decimal {
	return Decimal{decimal.NewFromFloat(f)}
}

// Add 加法
func (d Decimal) Add(other Decimal) Decimal {
	return Decimal{d.Decimal.Add(other.Decimal)}
}

// Sub 减法
func (d Decimal) Sub(other Decimal) Decimal {
	return Decimal{d.Decimal.Sub(other.Decimal)}
}

// Mul 乘法
func (d Decimal) Mul(other Decimal) Decimal {
	return Decimal{d.Decimal.Mul(other.Decimal)}
}

// Div 除法
func (d Decimal) Div(other Decimal) Decimal {
	return Decimal{d.Decimal.Div(other.Decimal)}
}

// Neg 取负
func (d Decimal) Neg() Decimal {
	return Decimal{d.Decimal.Neg()}
}

// Abs 绝对值
func (d Decimal) Abs() Decimal {
	return Decimal{d.Decimal.Abs()}
}

// Cmp 比较: -1 小于, 0 等于, 1 大于
func (d Decimal) Cmp(other Decimal) int {
	return d.Decimal.Cmp(other.Decimal)
}

// Equal 相等
func (d Decimal) Equal(other Decimal) bool {
	return d.Decimal.Equal(other.Decimal)
}

// GreaterThan 大于
func (d Decimal) GreaterThan(other Decimal) bool {
	return d.Decimal.GreaterThan(other.Decimal)
}

// GreaterThanOrEqual 大于等于
func (d Decimal) GreaterThanOrEqual(other Decimal) bool {
	return d.Decimal.GreaterThanOrEqual(other.Decimal)
}

// LessThan 小于
func (d Decimal) LessThan(other Decimal) bool {
	return d.Decimal.LessThan(other.Decimal)
}

// LessThanOrEqual 小于等于
func (d Decimal) LessThanOrEqual(other Decimal) bool {
	return d.Decimal.LessThanOrEqual(other.Decimal)
}

// IsZero 是否为零
func (d Decimal) IsZero() bool {
	return d.Decimal.IsZero()
}

// IsPositive 是否为正
func (d Decimal) IsPositive() bool {
	return d.Decimal.IsPositive()
}

// IsNegative 是否为负
func (d Decimal) IsNegative() bool {
	return d.Decimal.IsNegative()
}

// Round 四舍五入
func (d Decimal) Round(places int32) Decimal {
	return Decimal{d.Decimal.Round(places)}
}

// Truncate 截断
func (d Decimal) Truncate(places int32) Decimal {
	return Decimal{d.Decimal.Truncate(places)}
}

// String 转字符串
func (d Decimal) String() string {
	return d.Decimal.String()
}

// StringFixed 固定小数位
func (d Decimal) StringFixed(places int32) string {
	return d.Decimal.StringFixed(places)
}

// Value 实现 driver.Valuer 接口
func (d Decimal) Value() (driver.Value, error) {
	return d.String(), nil
}

// Scan 实现 sql.Scanner 接口
func (d *Decimal) Scan(value interface{}) error {
	if value == nil {
		d.Decimal = decimal.Zero
		return nil
	}

	var str string
	switch v := value.(type) {
	case []byte:
		str = string(v)
	case string:
		str = v
	default:
		return fmt.Errorf("cannot scan type %T into Decimal", value)
	}

	dec, err := decimal.NewFromString(str)
	if err != nil {
		return err
	}
	d.Decimal = dec
	return nil
}

// Min 返回较小值
func Min(a, b Decimal) Decimal {
	if a.LessThan(b) {
		return a
	}
	return b
}

// Max 返回较大值
func Max(a, b Decimal) Decimal {
	if a.GreaterThan(b) {
		return a
	}
	return b
}
