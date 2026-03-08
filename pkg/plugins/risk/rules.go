// Package risk 风控规则检查逻辑
package risk

import (
	"fmt"

	"github.com/tidwall/gjson"

	"alpha-trade-gateway/pkg/protocol"
	"alpha-trade-gateway/pkg/trader"
)

// === UserDataInjector 接口实现 ===

// InjectTradeData 将风控数据注入 tradeData 中间态
func (p *RiskPlugin) InjectTradeData(tradeData map[string]any, dumpAll bool) bool {
	hasChanges := false

	// 注入 risk_management_rule
	changedRules := make(map[string]*RiskManagementRule)
	for exchangeID, rule := range p.rules {
		if dumpAll || rule.Changed {
			changedRules[exchangeID] = rule
			rule.Changed = false
			hasChanges = true
		}
	}
	if len(changedRules) > 0 {
		tradeData["risk_management_rule"] = changedRules
	}

	// 注入 risk_management_data
	changedData := p.stats.BuildChangedRiskData(dumpAll)
	if len(changedData) > 0 {
		tradeData["risk_management_data"] = changedData
		hasChanges = true
	}

	return hasChanges
}

// === 客户端消息处理 ===

func (p *RiskPlugin) handleSetRiskManagementRule(connID int, msg string) {
	exchangeID := gjson.Get(msg, "exchange_id").String()
	enable := gjson.Get(msg, "enable").Bool()

	rule, exists := p.rules[exchangeID]
	if !exists {
		rule = &RiskManagementRule{
			UserID:               gjson.Get(msg, "user_id").String(),
			ExchangeID:           exchangeID,
			SelfTrade:            &SelfTradeRule{},
			FrequentCancellation: &FrequentCancellationRule{},
			TradePositionRatio:   &TradePositionRatioRule{},
		}
		p.rules[exchangeID] = rule
	}

	rule.Enable = enable

	if st := gjson.Get(msg, "self_trade"); st.Exists() {
		if v := st.Get("count_limit"); v.Exists() {
			rule.SelfTrade.CountLimit = int(v.Int())
		}
	}
	if fc := gjson.Get(msg, "frequent_cancellation"); fc.Exists() {
		if v := fc.Get("insert_order_count_limit"); v.Exists() {
			rule.FrequentCancellation.InsertOrderCountLimit = int(v.Int())
		}
		if v := fc.Get("cancel_order_count_limit"); v.Exists() {
			rule.FrequentCancellation.CancelOrderCountLimit = int(v.Int())
		}
		if v := fc.Get("cancel_order_percent_limit"); v.Exists() {
			rule.FrequentCancellation.CancelOrderPercentLimit = v.Float()
		}
	}
	if tp := gjson.Get(msg, "trade_position_ratio"); tp.Exists() {
		if v := tp.Get("trade_units_limit"); v.Exists() {
			rule.TradePositionRatio.TradeUnitsLimit = int(v.Int())
		}
		if v := tp.Get("trade_position_ratio_limit"); v.Exists() {
			rule.TradePositionRatio.TradePositionRatioLimit = v.Float()
		}
	}

	rule.Changed = true
	p.ctx.TriggerUserDataPush()
}

// === 拦截器实现 ===

func (p *RiskPlugin) handleBeforeInsertOrder(event *trader.Event) (bool, string) {
	req, ok := event.Data.(*trader.InsertOrderRequest)
	if !ok || req == nil {
		return true, ""
	}
	action := req.Action
	symbol := action.ExchangeID + "." + action.InstrumentID

	// 规则1: 自成交检测
	if p.config.SelfTradeCheck {
		if p.stats.HasOppositeActiveOrder(symbol, action.Direction) {
			p.stats.IncrSelfTradeRejected(symbol)
			return false, fmt.Sprintf("触发风控: 合约 %s 存在反向活跃订单，可能自成交", symbol)
		}
	}

	// 规则2: 开仓次数限制
	if action.Offset == protocol.OffsetOpen {
		if limit, ok := p.openCountsIndex[symbol]; ok {
			current := p.stats.GetOpenCount(symbol)
			if current+1 > limit {
				return false, fmt.Sprintf("触发风控: 合约 %s 开仓次数达到上限 %d", symbol, limit)
			}
		}
	}

	// 规则3: 开仓手数限制
	if action.Offset == protocol.OffsetOpen {
		if limit, ok := p.openVolumesIndex[symbol]; ok {
			current := p.stats.GetOpenVolume(symbol)
			if current+action.Volume > limit {
				return false, fmt.Sprintf("触发风控: 合约 %s 开仓手数达到上限 %d", symbol, limit)
			}
		}
	}

	// 规则4: 累计开仓手数限制
	if action.Offset == protocol.OffsetOpen {
		for name, rule := range p.accVolumesIndex {
			if rule.symbols[symbol] {
				current := p.stats.GetAccOpenVolume(name)
				if current+action.Volume > rule.limit {
					return false, fmt.Sprintf("触发风控: %s 累计开仓手数达到上限 %d", name, rule.limit)
				}
			}
		}
	}

	// 规则5: 订单频率限制
	if limit, ok := p.orderRateIndex[action.ExchangeID]; ok {
		current := p.stats.GetOrderRate(action.ExchangeID)
		if current+1 > limit {
			return false, fmt.Sprintf("触发风控: 交易所 %s 每秒操作次数超过上限 %d", action.ExchangeID, limit)
		}
	}

	// 规则6: 频繁报撤单检测 (基于 RiskManagementRule)
	if p.config.FrequentCancellationCheck {
		if rule, ok := p.rules[action.ExchangeID]; ok && rule.Enable {
			fc := rule.FrequentCancellation
			insertCount := p.stats.GetInsertOrderCount(symbol)
			if insertCount+1 > fc.InsertOrderCountLimit && fc.InsertOrderCountLimit > 0 {
				cancelCount := p.stats.GetCancelOrderCount(symbol)
				cancelPercent := float64(cancelCount) / float64(insertCount+1) * 100
				if cancelPercent > fc.CancelOrderPercentLimit {
					p.stats.IncrFreqCancellationRejected(symbol)
					return false, fmt.Sprintf("触发风控: 合约 %s 撤单比例 %.1f%% 超过限额 %.1f%%",
						symbol, cancelPercent, fc.CancelOrderPercentLimit)
				}
			}
		}
	}

	// 规则7: 成交持仓比检测 (基于 RiskManagementRule)
	if p.config.TradePositionRatioCheck {
		if rule, ok := p.rules[action.ExchangeID]; ok && rule.Enable {
			tp := rule.TradePositionRatio
			tradeUnits := p.stats.GetTradeUnits(symbol)
			if tradeUnits > tp.TradeUnitsLimit && tp.TradeUnitsLimit > 0 {
				netPos := p.stats.GetNetPosition(symbol)
				if netPos != 0 {
					absNet := netPos
					if absNet < 0 {
						absNet = -absNet
					}
					ratio := float64(tradeUnits) / float64(absNet) * 100
					if ratio > tp.TradePositionRatioLimit {
						p.stats.IncrTradePositionRatioRejected(symbol)
						return false, fmt.Sprintf("触发风控: 合约 %s 成交持仓比 %.1f%% 超过限额 %.1f%%",
							symbol, ratio, tp.TradePositionRatioLimit)
					}
				}
			}
		}
	}

	// 放行后更新统计
	if action.Offset == protocol.OffsetOpen {
		p.stats.IncrOpenCount(symbol)
	}
	p.stats.RecordOperation(action.ExchangeID)

	return true, ""
}

func (p *RiskPlugin) handleBeforeCancelOrder(event *trader.Event) (bool, string) {
	req, ok := event.Data.(*trader.CancelOrderRequest)
	if !ok || req == nil {
		return true, ""
	}

	// 撤单也计入操作频率
	exchangeID := p.getExchangeFromOrder(req.OrderID)
	if exchangeID != "" {
		if limit, ok := p.orderRateIndex[exchangeID]; ok {
			current := p.stats.GetOrderRate(exchangeID)
			if current+1 > limit {
				return false, fmt.Sprintf("触发风控: 交易所 %s 每秒操作次数超过上限 %d", exchangeID, limit)
			}
		}
		p.stats.RecordOperation(exchangeID)
	}

	return true, ""
}

// === 柜台回报处理 ===

func (p *RiskPlugin) processOrderReturn(event *trader.Event) {
	data, ok := event.Data.(*trader.OrderReturnData)
	if !ok || data == nil {
		return
	}
	symbol := data.ExchangeID + "." + data.InstrumentID

	if data.VolumeLeft > 0 && data.Status != protocol.OrderStatusFinished {
		p.stats.AddActiveOrder(symbol, data.OrderID, data.Direction, data.Offset, data.LimitPrice)
		// 更新自成交统计中的最高买价/最低卖价
		p.stats.UpdateSelfTradePrice(symbol, data.Direction, data.LimitPrice)
	} else {
		p.stats.RemoveActiveOrder(symbol, data.OrderID)
		if data.Status == protocol.OrderStatusFinished && data.VolumeLeft > 0 {
			// 撤单成功
			p.stats.IncrCancelOrderCount(symbol)
		}
	}

	p.stats.IncrInsertOrderCount(symbol)
}

func (p *RiskPlugin) processTradeReturn(event *trader.Event) {
	data, ok := event.Data.(*trader.TradeReturnData)
	if !ok || data == nil {
		return
	}
	symbol := data.ExchangeID + "." + data.InstrumentID

	p.stats.AddTradeUnits(symbol, data.Volume)

	if data.Offset == protocol.OffsetOpen {
		if _, ok := p.openVolumesIndex[symbol]; ok {
			p.stats.AddOpenVolume(symbol, data.Volume)
		}
		for name, rule := range p.accVolumesIndex {
			if rule.symbols[symbol] {
				p.stats.AddAccOpenVolume(name, data.Volume)
			}
		}
	}
}

func (p *RiskPlugin) processTradingDayChanged(event *trader.Event) {
	p.stats.Reset()
}

// getExchangeFromOrder 从订单数据中获取交易所代码
func (p *RiskPlugin) getExchangeFromOrder(orderID string) string {
	userData := p.ctx.UserData()
	if userData == nil {
		return ""
	}
	if order, ok := userData.Orders[orderID]; ok {
		return order.ExchangeID
	}
	return ""
}
