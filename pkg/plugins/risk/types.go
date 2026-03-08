// Package risk 风控插件协议数据结构
// 严格对齐 tqsdk-python objs.py 中的 RiskManagementRule / RiskManagementData
package risk

// ==================== 风控规则 (对应 tqsdk RiskManagementRule) ====================
// 数据路径: trade.{user_id}.risk_management_rule.{exchange_id}

// RiskManagementRule 风控规则 (按交易所维度)
// 对应 tqsdk-python objs.py RiskManagementRule
type RiskManagementRule struct {
	UserID     string `json:"user_id"`
	ExchangeID string `json:"exchange_id"`
	Enable     bool   `json:"enable"`

	SelfTrade            *SelfTradeRule            `json:"self_trade,omitempty"`
	FrequentCancellation *FrequentCancellationRule  `json:"frequent_cancellation,omitempty"`
	TradePositionRatio   *TradePositionRatioRule    `json:"trade_position_ratio,omitempty"`

	Changed bool `json:"-"` // 内部标记，不序列化
}

// SelfTradeRule 自成交风控规则
// 对应 tqsdk-python objs.py SelfTradeRule
type SelfTradeRule struct {
	CountLimit int `json:"count_limit"` // 最大自成交次数限制
}

// FrequentCancellationRule 频繁报撤单风控规则
// 对应 tqsdk-python objs.py FrequentCancellationRule
type FrequentCancellationRule struct {
	InsertOrderCountLimit   int     `json:"insert_order_count_limit"`   // 频繁报撤单起算报单次数
	CancelOrderCountLimit   int     `json:"cancel_order_count_limit"`   // 频繁报撤单起算撤单次数
	CancelOrderPercentLimit float64 `json:"cancel_order_percent_limit"` // 频繁报撤单撤单比例限额(百分比)
}

// TradePositionRatioRule 成交持仓比风控规则
// 对应 tqsdk-python objs.py TradePositionRatioRule
type TradePositionRatioRule struct {
	TradeUnitsLimit         int     `json:"trade_units_limit"`          // 成交持仓比起算成交手数
	TradePositionRatioLimit float64 `json:"trade_position_ratio_limit"` // 成交持仓比例限额(百分比)
}

// ==================== 风控统计数据 (对应 tqsdk RiskManagementData) ====================
// 数据路径: trade.{user_id}.risk_management_data.{symbol}

// RiskManagementData 风控统计数据 (按合约维度)
// 对应 tqsdk-python objs.py RiskManagementData
type RiskManagementData struct {
	UserID       string `json:"user_id"`
	ExchangeID   string `json:"exchange_id"`
	InstrumentID string `json:"instrument_id"`

	SelfTrade            *SelfTradeData            `json:"self_trade"`
	FrequentCancellation *FrequentCancellationData `json:"frequent_cancellation"`
	TradePositionRatio   *TradePositionRatioData   `json:"trade_position_ratio"`

	Changed bool `json:"-"` // 内部标记，不序列化
}

// SelfTradeData 自成交统计数据
// 对应 tqsdk-python objs.py SelfTrade
type SelfTradeData struct {
	HighestBuyPrice float64 `json:"highest_buy_price"`  // 当前最高买价
	LowestSellPrice float64 `json:"lowest_sell_price"`  // 当前最低卖价
	SelfTradeCount  int     `json:"self_trade_count"`   // 当天已发生的自成交次数
	RejectedCount   int     `json:"rejected_count"`     // 当天因自成交被拒的报单次数
}

// FrequentCancellationData 频繁报撤单统计数据
// 对应 tqsdk-python objs.py FrequentCancellation
type FrequentCancellationData struct {
	InsertOrderCount   int     `json:"insert_order_count"`   // 当天已发生的报单次数
	CancelOrderCount   int     `json:"cancel_order_count"`   // 当天已发生的撤单次数
	CancelOrderPercent float64 `json:"cancel_order_percent"` // 当天的撤单比例(百分比)
	RejectedCount      int     `json:"rejected_count"`       // 当天因撤单比例超限被拒的次数
}

// TradePositionRatioData 成交持仓比统计数据
// 对应 tqsdk-python objs.py TradePositionRatio
type TradePositionRatioData struct {
	TradeUnits         int     `json:"trade_units"`          // 当天已发生的成交手数
	NetPositionUnits   int     `json:"net_position_units"`   // 当前净持仓手数
	TradePositionRatio float64 `json:"trade_position_ratio"` // 当前成交持仓比(百分比)
	RejectedCount      int     `json:"rejected_count"`       // 当天因成交持仓比超限被拒的次数
}
