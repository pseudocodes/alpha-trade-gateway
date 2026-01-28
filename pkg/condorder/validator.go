// Package condorder 条件单验证器
package condorder

import (
	"fmt"
	"math"

	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/protocol"
)

// Validator 条件单验证器
type Validator struct {
	callback            Callback
	checker             *ConditionChecker
	maxNewOrdersPerDay  int
	maxValidOrdersTotal int
}

// NewValidator 创建验证器
func NewValidator(callback Callback, config *Config) *Validator {
	return &Validator{
		callback:            callback,
		checker:             NewConditionChecker(),
		maxNewOrdersPerDay:  config.MaxNewOrdersPerDay,
		maxValidOrdersTotal: config.MaxValidOrdersTotal,
	}
}

// Validate 验证条件单
func (v *Validator) Validate(order *ConditionOrder, currentDayCount, currentValidCount int) error {
	// 检查条件数量限制
	if currentDayCount+1 > v.maxNewOrdersPerDay {
		v.callback.OutputNotify(ErrCodeExceedDailyLimit,
			fmt.Sprintf("条件单已被服务器拒绝,当前交易日新增条件单数量超过最大数量限制:%d", v.maxNewOrdersPerDay),
			"WARNING", "MESSAGE")
		return ErrExceedDailyLimit
	}

	if currentValidCount+1 > v.maxValidOrdersTotal {
		v.callback.OutputNotify(ErrCodeExceedTotalLimit,
			fmt.Sprintf("条件单已被服务器拒绝,当前有效条件单数量超过最大数量限制:%d", v.maxValidOrdersTotal),
			"WARNING", "MESSAGE")
		return ErrExceedTotalLimit
	}

	// 判断逻辑运算符
	logicIsOr := order.ConditionsLogicOper == LogicOperatorOr

	// 验证每个触发条件
	for i := range order.ConditionList {
		cond := &order.ConditionList[i]
		if err := v.validateCondition(cond, order, logicIsOr); err != nil {
			return err
		}
	}

	// 如果是AND逻辑，检查是否所有条件都已满足
	if !logicIsOr {
		allTouched := true
		for _, cond := range order.ConditionList {
			if !cond.IsTouched {
				allTouched = false
				break
			}
		}
		if allTouched {
			v.callback.OutputNotify(ErrCodeAllConditionsMet,
				"条件单已被服务器拒绝,当前所有条件都已满足,请重新设置",
				"WARNING", "MESSAGE")
			return ErrConditionAlreadyMet
		}
	}

	// 验证订单列表
	for i := range order.OrderList {
		if err := v.validateOrder(&order.OrderList[i]); err != nil {
			return err
		}
	}

	// 验证有效期
	if err := v.validateOrderTimeCondition(order); err != nil {
		return err
	}

	return nil
}

// validateCondition 验证单个触发条件
func (v *Validator) validateCondition(cond *ContingentCondition, order *ConditionOrder, logicIsOr bool) error {
	symbol := cond.ExchangeID + "." + cond.InstrumentID

	// 检查合约是否存在
	ins := v.callback.GetInstrument(symbol)
	if ins == nil {
		logger.Warn("invalid instrument in condition",
			zap.String("symbol", symbol),
			zap.String("order_id", order.OrderID))
		v.callback.OutputNotify(ErrCodeInvalidInstrument,
			"条件单已被服务器拒绝,条件单触发条件中的合约ID不存在:"+symbol,
			"WARNING", "MESSAGE")
		return ErrInvalidInstrument
	}

	switch cond.ContingentType {
	case ContingentTypeTime:
		return v.validateTimeTriggeredCondition(cond, order, logicIsOr)
	case ContingentTypePrice:
		return v.validatePriceCondition(cond, ins, order, logicIsOr)
	case ContingentTypePriceRange:
		return v.validatePriceRangeCondition(cond, ins, order, logicIsOr)
	case ContingentTypeBreakEven:
		return v.validateBreakEvenCondition(cond, ins, order, logicIsOr)
	case ContingentTypeMarketOpen:
		// 开盘触发条件无需额外验证
		return nil
	}

	return nil
}

// validateTimeTriggeredCondition 验证时间触发条件
func (v *Validator) validateTimeTriggeredCondition(cond *ContingentCondition, order *ConditionOrder, logicIsOr bool) error {
	exchangeTime := v.checker.GetExchangeTime(cond.ExchangeID)
	if cond.ContingentTime < exchangeTime {
		logger.Warn("time already passed",
			zap.String("order_id", order.OrderID),
			zap.Int64("contingent_time", cond.ContingentTime),
			zap.Int64("exchange_time", exchangeTime))
		v.callback.OutputNotify(ErrCodeTimeAlreadyPassed,
			"条件单已被服务器拒绝,时间触发条件指定的触发时间小于当前时间",
			"WARNING", "MESSAGE")
		return ErrTimeAlreadyPassed
	}
	return nil
}

// validatePriceCondition 验证价格触发条件
func (v *Validator) validatePriceCondition(cond *ContingentCondition, ins *protocol.Instrument, order *ConditionOrder, logicIsOr bool) error {
	// 检查触发价格是否有效
	if math.IsNaN(cond.ContingentPrice) || cond.ContingentPrice <= 0 {
		logger.Warn("invalid contingent price",
			zap.String("order_id", order.OrderID),
			zap.Float64("price", cond.ContingentPrice))
		v.callback.OutputNotify(ErrCodeInvalidPrice,
			"条件单已被服务器拒绝,价格触发条件指定的触发价格不合法",
			"WARNING", "MESSAGE")
		return ErrInvalidPrice
	}

	// 检查当前价格是否已满足条件
	lastPrice := ins.LastPrice
	if ins.ProductClass == protocol.ProductClassCombination && len(order.OrderList) > 0 {
		if order.OrderList[0].Direction == OrderDirectionBuy {
			lastPrice = ins.AskPrice1
		} else {
			lastPrice = ins.BidPrice1
		}
	}

	if !math.IsNaN(lastPrice) && lastPrice > 0 {
		satisfied := false
		switch cond.PriceRelation {
		case PriceRelationGreater:
			satisfied = lastPrice > cond.ContingentPrice
		case PriceRelationGreaterEqual:
			satisfied = lastPrice >= cond.ContingentPrice
		case PriceRelationLess:
			satisfied = lastPrice < cond.ContingentPrice
		case PriceRelationLessEqual:
			satisfied = lastPrice <= cond.ContingentPrice
		}

		if satisfied {
			if logicIsOr {
				logger.Warn("condition already met",
					zap.String("order_id", order.OrderID),
					zap.Float64("last_price", lastPrice),
					zap.Float64("contingent_price", cond.ContingentPrice))
				v.callback.OutputNotify(ErrCodeConditionAlreadyMet,
					"条件单已被服务器拒绝,当前价格已满足设定条件,请重新设置",
					"WARNING", "MESSAGE")
				return ErrConditionAlreadyMet
			} else {
				cond.IsTouched = true
			}
		}
	}

	return nil
}

// validatePriceRangeCondition 验证价格区间触发条件
func (v *Validator) validatePriceRangeCondition(cond *ContingentCondition, ins *protocol.Instrument, order *ConditionOrder, logicIsOr bool) error {
	// 检查价格区间是否有效
	if math.IsNaN(cond.ContingentPriceLeft) || math.IsNaN(cond.ContingentPriceRight) ||
		cond.ContingentPriceLeft > cond.ContingentPriceRight {
		logger.Warn("invalid price range",
			zap.String("order_id", order.OrderID),
			zap.Float64("left", cond.ContingentPriceLeft),
			zap.Float64("right", cond.ContingentPriceRight))
		v.callback.OutputNotify(ErrCodeInvalidPriceRange,
			"条件单已被服务器拒绝,价格区间触发条件指定的价格区间不合法",
			"WARNING", "MESSAGE")
		return ErrInvalidPriceRange
	}

	// 检查当前价格是否已在区间内
	lastPrice := ins.LastPrice
	if ins.ProductClass == protocol.ProductClassCombination && len(order.OrderList) > 0 {
		if order.OrderList[0].Direction == OrderDirectionBuy {
			lastPrice = ins.AskPrice1
		} else {
			lastPrice = ins.BidPrice1
		}
	}

	if !math.IsNaN(lastPrice) && lastPrice > 0 {
		if lastPrice >= cond.ContingentPriceLeft && lastPrice <= cond.ContingentPriceRight {
			if logicIsOr {
				logger.Warn("condition already met (price range)",
					zap.String("order_id", order.OrderID),
					zap.Float64("last_price", lastPrice))
				v.callback.OutputNotify(ErrCodeConditionAlreadyMet,
					"条件单已被服务器拒绝,当前价格已满足设定条件,请重新设置",
					"WARNING", "MESSAGE")
				return ErrConditionAlreadyMet
			} else {
				cond.IsTouched = true
			}
		}
	}

	return nil
}

// validateBreakEvenCondition 验证保本止盈触发条件
func (v *Validator) validateBreakEvenCondition(cond *ContingentCondition, ins *protocol.Instrument, order *ConditionOrder, logicIsOr bool) error {
	// 检查保本价格是否有效
	if math.IsNaN(cond.BreakEvenPrice) || cond.BreakEvenPrice <= 0 {
		logger.Warn("invalid break even price",
			zap.String("order_id", order.OrderID),
			zap.Float64("price", cond.BreakEvenPrice))
		v.callback.OutputNotify(ErrCodeInvalidBreakEvenPrice,
			"条件单已被服务器拒绝,固定价格止盈触发条件指定的固定价格不合法",
			"WARNING", "MESSAGE")
		return ErrInvalidPrice
	}

	// 检查当前价格是否已满足条件
	lastPrice := ins.LastPrice
	if ins.ProductClass == protocol.ProductClassCombination && len(order.OrderList) > 0 {
		if order.OrderList[0].Direction == OrderDirectionBuy {
			lastPrice = ins.AskPrice1
		} else {
			lastPrice = ins.BidPrice1
		}
	}

	if !math.IsNaN(lastPrice) && lastPrice > 0 {
		satisfied := false
		if cond.BreakEvenDirection == OrderDirectionBuy {
			// 多头：向上突破止盈价
			satisfied = lastPrice > cond.BreakEvenPrice
		} else {
			// 空头：向下突破止盈价
			satisfied = lastPrice < cond.BreakEvenPrice
		}

		if satisfied {
			if logicIsOr {
				logger.Warn("condition already met (break even)",
					zap.String("order_id", order.OrderID),
					zap.Float64("last_price", lastPrice))
				v.callback.OutputNotify(ErrCodeConditionAlreadyMet,
					"条件单已被服务器拒绝,当前价格已满足设定条件,请重新设置",
					"WARNING", "MESSAGE")
				return ErrConditionAlreadyMet
			} else {
				cond.IsTouched = true
			}
		}
	}

	return nil
}

// validateOrder 验证订单
func (v *Validator) validateOrder(order *ContingentOrder) error {
	symbol := order.ExchangeID + "." + order.InstrumentID

	// 检查合约是否存在
	ins := v.callback.GetInstrument(symbol)
	if ins == nil {
		logger.Warn("invalid instrument in order",
			zap.String("symbol", symbol))
		v.callback.OutputNotify(ErrCodeInvalidOrderInstrument,
			"条件单已被服务器拒绝,条件单触发的订单列表中的合约ID不存在:"+symbol,
			"WARNING", "MESSAGE")
		return ErrInvalidInstrument
	}

	// 检查手数
	if order.VolumeType == VolumeTypeNum && order.Volume <= 0 {
		logger.Warn("invalid volume",
			zap.String("symbol", symbol),
			zap.Int("volume", order.Volume))
		v.callback.OutputNotify(ErrCodeInvalidVolume,
			"条件单已被服务器拒绝,条件单触发的订单手数设置不合法",
			"WARNING", "MESSAGE")
		return ErrInvalidVolume
	}

	// 检查价格
	if order.PriceType == PriceTypeLimit {
		if math.IsNaN(order.LimitPrice) || order.LimitPrice <= 0 {
			logger.Warn("invalid limit price",
				zap.String("symbol", symbol),
				zap.Float64("limit_price", order.LimitPrice))
			v.callback.OutputNotify(ErrCodeInvalidOrderPrice,
				"条件单已被服务器拒绝,条件单触发的订单价格设置不合法",
				"WARNING", "MESSAGE")
			return ErrInvalidPrice
		}
	}

	return nil
}

// validateOrderTimeCondition 验证订单有效期
func (v *Validator) validateOrderTimeCondition(order *ConditionOrder) error {
	if order.TimeConditionType == TimeConditionGTD {
		if order.GTDDate < order.TradingDay {
			logger.Warn("invalid GTD date",
				zap.String("order_id", order.OrderID),
				zap.Int("GTD_date", order.GTDDate),
				zap.Int("trading_day", order.TradingDay))
			v.callback.OutputNotify(ErrCodeInvalidGTDDate,
				"条件单已被服务器拒绝,条件单有效日期设置不合法",
				"WARNING", "MESSAGE")
			return ErrInvalidGTDDate
		}
	}
	return nil
}

// SetChecker 设置检测器（用于共享交易所时间）
func (v *Validator) SetChecker(checker *ConditionChecker) {
	v.checker = checker
}
