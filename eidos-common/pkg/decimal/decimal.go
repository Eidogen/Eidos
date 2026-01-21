package decimal

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

// 常量定义
const (
	// DefaultPrecision 默认精度
	DefaultPrecision = 18
	// DefaultScale 默认小数位数
	DefaultScale = 8
)

// Decimal 高精度数值类型
type Decimal struct {
	decimal.Decimal
}

// 预定义常量
var (
	Zero    = Decimal{decimal.Zero}
	One     = Decimal{decimal.NewFromInt(1)}
	Ten     = Decimal{decimal.NewFromInt(10)}
	Hundred = Decimal{decimal.NewFromInt(100)}
	Neg1    = Decimal{decimal.NewFromInt(-1)}
)

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

// Sum 计算总和
func Sum(values ...Decimal) Decimal {
	result := Zero
	for _, v := range values {
		result = result.Add(v)
	}
	return result
}

// Avg 计算平均值
func Avg(values ...Decimal) Decimal {
	if len(values) == 0 {
		return Zero
	}
	sum := Sum(values...)
	return sum.Div(NewFromInt(int64(len(values))))
}

// NewFromUint64 从 uint64 创建
func NewFromUint64(i uint64) Decimal {
	return Decimal{decimal.NewFromInt(int64(i))}
}

// NewFromBigInt 从大整数字符串创建
func NewFromBigInt(s string) (Decimal, error) {
	return NewFromString(s)
}

// Pow 幂运算
func (d Decimal) Pow(exp Decimal) Decimal {
	return Decimal{d.Decimal.Pow(exp.Decimal)}
}

// PowInt 整数幂运算
func (d Decimal) PowInt(exp int) Decimal {
	if exp == 0 {
		return One
	}
	if exp < 0 {
		return One.Div(d.PowInt(-exp))
	}

	result := One
	base := d
	for exp > 0 {
		if exp%2 == 1 {
			result = result.Mul(base)
		}
		base = base.Mul(base)
		exp /= 2
	}
	return result
}

// Sqrt 平方根 (使用牛顿法)
func (d Decimal) Sqrt() Decimal {
	if d.IsNegative() {
		return Zero
	}
	if d.IsZero() {
		return Zero
	}

	// 使用 float64 近似值作为初始猜测
	f, _ := d.Float64()
	guess := NewFromFloat(math.Sqrt(f))

	// 牛顿迭代
	two := NewFromInt(2)
	for i := 0; i < 50; i++ {
		next := guess.Add(d.Div(guess)).Div(two)
		if next.Equal(guess) {
			break
		}
		guess = next
	}

	return guess
}

// Mod 取模
func (d Decimal) Mod(other Decimal) Decimal {
	return Decimal{d.Decimal.Mod(other.Decimal)}
}

// Floor 向下取整
func (d Decimal) Floor() Decimal {
	return Decimal{d.Decimal.Floor()}
}

// Ceil 向上取整
func (d Decimal) Ceil() Decimal {
	return Decimal{d.Decimal.Ceil()}
}

// Sign 返回符号: -1, 0, 1
func (d Decimal) Sign() int {
	return d.Decimal.Sign()
}

// Float64 转换为 float64
func (d Decimal) Float64() (float64, bool) {
	f, exact := d.Decimal.Float64()
	return f, exact
}

// Int64 转换为 int64 (截断小数部分)
func (d Decimal) Int64() int64 {
	return d.Decimal.IntPart()
}

// IntPart 返回整数部分
func (d Decimal) IntPart() int64 {
	return d.Decimal.IntPart()
}

// Exponent 返回指数
func (d Decimal) Exponent() int32 {
	return d.Decimal.Exponent()
}

// Coefficient 返回系数
func (d Decimal) Coefficient() int64 {
	return d.Decimal.Coefficient().Int64()
}

// NumDigits 返回有效数字位数
func (d Decimal) NumDigits() int {
	return d.Decimal.NumDigits()
}

// Shift 移位
func (d Decimal) Shift(shift int32) Decimal {
	return Decimal{d.Decimal.Shift(shift)}
}

// RoundBank 银行家舍入法
func (d Decimal) RoundBank(places int32) Decimal {
	return Decimal{d.Decimal.RoundBank(places)}
}

// RoundCash 现金舍入 (到指定间隔)
func (d Decimal) RoundCash(interval uint8) Decimal {
	return Decimal{d.Decimal.RoundCash(interval)}
}

// RoundUp 向上舍入
func (d Decimal) RoundUp(places int32) Decimal {
	multiplier := decimal.NewFromInt(10).Pow(decimal.NewFromInt32(places))
	shifted := d.Decimal.Mul(multiplier)
	if shifted.Sign() >= 0 {
		shifted = shifted.Ceil()
	} else {
		shifted = shifted.Floor()
	}
	return Decimal{shifted.Div(multiplier)}
}

// RoundDown 向下舍入
func (d Decimal) RoundDown(places int32) Decimal {
	return d.Truncate(places)
}

// InRange 检查是否在范围内 [min, max]
func (d Decimal) InRange(min, max Decimal) bool {
	return d.GreaterThanOrEqual(min) && d.LessThanOrEqual(max)
}

// Clamp 限制在范围内
func (d Decimal) Clamp(min, max Decimal) Decimal {
	if d.LessThan(min) {
		return min
	}
	if d.GreaterThan(max) {
		return max
	}
	return d
}

// Copy 复制
func (d Decimal) Copy() Decimal {
	return Decimal{d.Decimal.Copy()}
}

// MarshalJSON JSON 序列化
func (d Decimal) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON JSON 反序列化
func (d *Decimal) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		// 尝试解析为数字
		var f float64
		if err := json.Unmarshal(data, &f); err != nil {
			return err
		}
		d.Decimal = decimal.NewFromFloat(f)
		return nil
	}

	dec, err := decimal.NewFromString(s)
	if err != nil {
		return err
	}
	d.Decimal = dec
	return nil
}

// ParseAmount 解析金额字符串，支持逗号分隔
func ParseAmount(s string) (Decimal, error) {
	// 移除逗号
	cleaned := ""
	for _, c := range s {
		if c != ',' {
			cleaned += string(c)
		}
	}
	return NewFromString(cleaned)
}

// FormatAmount 格式化金额，添加千位分隔符
func (d Decimal) FormatAmount(places int32) string {
	s := d.StringFixed(places)

	// 分离整数和小数部分
	dotIndex := -1
	for i, c := range s {
		if c == '.' {
			dotIndex = i
			break
		}
	}

	var intPart, decPart string
	if dotIndex >= 0 {
		intPart = s[:dotIndex]
		decPart = s[dotIndex:]
	} else {
		intPart = s
		decPart = ""
	}

	// 处理负号
	negative := false
	if len(intPart) > 0 && intPart[0] == '-' {
		negative = true
		intPart = intPart[1:]
	}

	// 添加千位分隔符
	result := ""
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}

	if negative {
		result = "-" + result
	}

	return result + decPart
}

// Percentage 计算百分比
func (d Decimal) Percentage(percent Decimal) Decimal {
	return d.Mul(percent).Div(Hundred)
}

// PercentageOf 计算占比 (返回百分比值)
func (d Decimal) PercentageOf(total Decimal) Decimal {
	if total.IsZero() {
		return Zero
	}
	return d.Div(total).Mul(Hundred)
}

// ToWei 转换为最小单位 (如以太坊的 wei)
func (d Decimal) ToWei(decimals int32) Decimal {
	multiplier := NewFromInt(10).PowInt(int(decimals))
	return d.Mul(multiplier).Floor()
}

// FromWei 从最小单位转换
func FromWei(wei Decimal, decimals int32) Decimal {
	divisor := NewFromInt(10).PowInt(int(decimals))
	return wei.Div(divisor)
}

// Compare 比较两个值
func Compare(a, b Decimal) int {
	return a.Cmp(b)
}

// NullDecimal 可空的 Decimal
type NullDecimal struct {
	Decimal Decimal
	Valid   bool
}

// NewNullDecimal 创建可空 Decimal
func NewNullDecimal(d Decimal, valid bool) NullDecimal {
	return NullDecimal{
		Decimal: d,
		Valid:   valid,
	}
}

// NewNullDecimalFromString 从字符串创建可空 Decimal
func NewNullDecimalFromString(s string) NullDecimal {
	if s == "" {
		return NullDecimal{Valid: false}
	}
	d, err := NewFromString(s)
	if err != nil {
		return NullDecimal{Valid: false}
	}
	return NullDecimal{Decimal: d, Valid: true}
}

// Value 实现 driver.Valuer 接口
func (n NullDecimal) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.Decimal.String(), nil
}

// Scan 实现 sql.Scanner 接口
func (n *NullDecimal) Scan(value interface{}) error {
	if value == nil {
		n.Valid = false
		return nil
	}

	var d Decimal
	if err := d.Scan(value); err != nil {
		return err
	}

	n.Decimal = d
	n.Valid = true
	return nil
}

// MarshalJSON JSON 序列化
func (n NullDecimal) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return json.Marshal(nil)
	}
	return n.Decimal.MarshalJSON()
}

// UnmarshalJSON JSON 反序列化
func (n *NullDecimal) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		n.Valid = false
		return nil
	}

	if err := n.Decimal.UnmarshalJSON(data); err != nil {
		return err
	}
	n.Valid = true
	return nil
}
