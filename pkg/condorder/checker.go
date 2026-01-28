// Package condorder 条件检测器
package condorder

import (
	"math"
	"strings"
	"time"

	"alpha-trade-gateway/pkg/protocol"
)

// ConditionChecker 条件检测器
type ConditionChecker struct {
	// 各交易所时间
	localTime int64
	shfeTime  int64
	dceTime   int64
	ineTime   int64
	ffexTime  int64
	czceTime  int64

	// 合约交易状态
	instrumentStatus map[string]InstrumentStatus
}

// NewConditionChecker 创建条件检测器
func NewConditionChecker() *ConditionChecker {
	return &ConditionChecker{
		instrumentStatus: make(map[string]InstrumentStatus),
	}
}

// CheckPriceCondition 检查价格条件
func (c *ConditionChecker) CheckPriceCondition(cond *ContingentCondition, ins *protocol.Instrument, order *ConditionOrder) bool {
	if ins == nil {
		return false
	}

	// 检查合约交易状态
	instID := trimDigits(cond.InstrumentID)
	if status, ok := c.instrumentStatus[instID]; ok {
		if status != InstrumentStatusContinousTrading {
			return false
		}
	}

	// 获取最新价格
	lastPrice := ins.LastPrice

	// 组合合约使用买一/卖一价
	if ins.ProductClass == protocol.ProductClassCombination && len(order.OrderList) > 0 {
		if order.OrderList[0].Direction == OrderDirectionBuy {
			lastPrice = ins.AskPrice1
		} else {
			lastPrice = ins.BidPrice1
		}
	}

	// 检查价格是否有效
	if math.IsNaN(lastPrice) || lastPrice <= 0 {
		return false
	}

	switch cond.ContingentType {
	case ContingentTypePrice:
		return c.checkSinglePrice(cond, lastPrice)
	case ContingentTypePriceRange:
		return c.checkPriceRange(cond, lastPrice)
	case ContingentTypeBreakEven:
		return c.checkBreakEven(cond, lastPrice)
	}

	return false
}

// checkSinglePrice 检查单一价格条件
func (c *ConditionChecker) checkSinglePrice(cond *ContingentCondition, lastPrice float64) bool {
	switch cond.PriceRelation {
	case PriceRelationGreater:
		return lastPrice > cond.ContingentPrice
	case PriceRelationGreaterEqual:
		return lastPrice >= cond.ContingentPrice
	case PriceRelationLess:
		return lastPrice < cond.ContingentPrice
	case PriceRelationLessEqual:
		return lastPrice <= cond.ContingentPrice
	}
	return false
}

// checkPriceRange 检查价格区间条件
func (c *ConditionChecker) checkPriceRange(cond *ContingentCondition, lastPrice float64) bool {
	return lastPrice >= cond.ContingentPriceLeft && lastPrice <= cond.ContingentPriceRight
}

// checkBreakEven 检查保本止盈条件
func (c *ConditionChecker) checkBreakEven(cond *ContingentCondition, lastPrice float64) bool {
	if cond.BreakEvenDirection == OrderDirectionBuy {
		// 多头：先向上突破止盈价，再回到保本价
		if cond.HasBreakEvent {
			return lastPrice <= cond.BreakEvenPrice
		} else {
			if lastPrice > cond.BreakEvenPrice {
				cond.HasBreakEvent = true
			}
			return false
		}
	} else {
		// 空头：先向下突破止盈价，再回到保本价
		if cond.HasBreakEvent {
			return lastPrice >= cond.BreakEvenPrice
		} else {
			if lastPrice < cond.BreakEvenPrice {
				cond.HasBreakEvent = true
			}
			return false
		}
	}
}

// IsAllConditionsTouched 检查所有条件是否满足
func (c *ConditionChecker) IsAllConditionsTouched(order *ConditionOrder) bool {
	if len(order.ConditionList) == 0 {
		return false
	}

	if len(order.ConditionList) == 1 {
		return order.ConditionList[0].IsTouched
	}

	if order.ConditionsLogicOper == LogicOperatorAnd {
		for _, cond := range order.ConditionList {
			if !cond.IsTouched {
				return false
			}
		}
		return true
	} else {
		// OR 逻辑
		for _, cond := range order.ConditionList {
			if cond.IsTouched {
				return true
			}
		}
		return false
	}
}

// SetExchangeTime 设置各交易所时间
func (c *ConditionChecker) SetExchangeTime(localTime, shfeTime, dceTime, ineTime, ffexTime, czceTime int64) {
	c.localTime = localTime
	c.shfeTime = shfeTime
	c.dceTime = dceTime
	c.ineTime = ineTime
	c.ffexTime = ffexTime
	c.czceTime = czceTime
}

// GetExchangeTime 获取指定交易所时间
func (c *ConditionChecker) GetExchangeTime(exchangeID string) int64 {
	now := time.Now().Unix()
	delta := now - c.localTime

	switch exchangeID {
	case "SHFE":
		return c.shfeTime + delta
	case "INE":
		return c.ineTime + delta
	case "CZCE":
		return c.czceTime + delta
	case "DCE":
		return c.dceTime + delta
	default:
		return c.ffexTime + delta
	}
}

// UpdateInstrumentStatus 更新合约交易状态
func (c *ConditionChecker) UpdateInstrumentStatus(instrumentID string, status InstrumentStatus) {
	c.instrumentStatus[instrumentID] = status
}

// trimDigits 去除合约代码的数字部分
// 例如：rb2505 -> rb
func trimDigits(instrumentID string) string {
	result := strings.Builder{}
	for _, r := range instrumentID {
		if r >= '0' && r <= '9' {
			break
		}
		result.WriteRune(r)
	}
	return result.String()
}

// isCombinationInstrument 判断是否为组合合约
func isCombinationInstrument(instrumentID string) bool {
	// 组合合约通常包含特殊字符，如 & 或 SPD
	return strings.Contains(instrumentID, "&") || strings.Contains(instrumentID, "SPD")
}
