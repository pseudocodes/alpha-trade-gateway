// Package risk 风控运行时统计
package risk

import (
	"math"
	"strings"
	"sync"
	"time"
)

// RiskStats 风控运行时统计 (线程安全)
type RiskStats struct {
	mu sync.RWMutex

	// 每合约开仓次数
	openCounts map[string]int
	// 每合约开仓手数
	openVolumes map[string]int
	// 累计开仓手数 (按规则名)
	accOpenVolumes map[string]int
	// 每交易所操作频率 (滑动窗口)
	orderRates map[string]*rateWindow

	// 每合约报单次数
	insertOrderCounts map[string]int
	// 每合约撤单次数
	cancelOrderCounts map[string]int
	// 每合约成交手数
	tradeUnits map[string]int
	// 每合约净持仓
	netPositions map[string]int

	// 自成交统计: symbol → {highest_buy, lowest_sell, count, rejected}
	selfTradeStats map[string]*selfTradeStatsEntry

	// 频繁报撤单拒绝计数
	freqCancellationRejected map[string]int
	// 成交持仓比拒绝计数
	tradePositionRatioRejected map[string]int

	// 活跃订单 (用于自成交检测)
	activeOrders map[string]map[string]*activeOrder // symbol → orderID → order

	// 风控数据变更标记
	changedSymbols map[string]bool
}

type selfTradeStatsEntry struct {
	HighestBuyPrice float64
	LowestSellPrice float64
	SelfTradeCount  int
	RejectedCount   int
}

type activeOrder struct {
	OrderID   string
	Direction int64
	Offset    int64
	Price     float64
}

type rateWindow struct {
	counts []time.Time
}

// NewRiskStats 创建风控统计
func NewRiskStats() *RiskStats {
	return &RiskStats{
		openCounts:                 make(map[string]int),
		openVolumes:                make(map[string]int),
		accOpenVolumes:             make(map[string]int),
		orderRates:                 make(map[string]*rateWindow),
		insertOrderCounts:          make(map[string]int),
		cancelOrderCounts:          make(map[string]int),
		tradeUnits:                 make(map[string]int),
		netPositions:               make(map[string]int),
		selfTradeStats:             make(map[string]*selfTradeStatsEntry),
		freqCancellationRejected:   make(map[string]int),
		tradePositionRatioRejected: make(map[string]int),
		activeOrders:               make(map[string]map[string]*activeOrder),
		changedSymbols:             make(map[string]bool),
	}
}

// Reset 交易日切换时重置所有统计
func (s *RiskStats) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.openCounts = make(map[string]int)
	s.openVolumes = make(map[string]int)
	s.accOpenVolumes = make(map[string]int)
	s.orderRates = make(map[string]*rateWindow)
	s.insertOrderCounts = make(map[string]int)
	s.cancelOrderCounts = make(map[string]int)
	s.tradeUnits = make(map[string]int)
	s.netPositions = make(map[string]int)
	s.selfTradeStats = make(map[string]*selfTradeStatsEntry)
	s.freqCancellationRejected = make(map[string]int)
	s.tradePositionRatioRejected = make(map[string]int)
	s.activeOrders = make(map[string]map[string]*activeOrder)
	s.changedSymbols = make(map[string]bool)
}

// GetOpenCount 获取合约开仓次数
func (s *RiskStats) GetOpenCount(symbol string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.openCounts[symbol]
}

// IncrOpenCount 增加开仓次数
func (s *RiskStats) IncrOpenCount(symbol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openCounts[symbol]++
	s.changedSymbols[symbol] = true
}

// GetOpenVolume 获取合约开仓手数
func (s *RiskStats) GetOpenVolume(symbol string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.openVolumes[symbol]
}

// AddOpenVolume 增加开仓手数
func (s *RiskStats) AddOpenVolume(symbol string, volume int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openVolumes[symbol] += volume
	s.changedSymbols[symbol] = true
}

// GetAccOpenVolume 获取累计开仓手数
func (s *RiskStats) GetAccOpenVolume(name string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.accOpenVolumes[name]
}

// AddAccOpenVolume 增加累计开仓手数
func (s *RiskStats) AddAccOpenVolume(name string, volume int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accOpenVolumes[name] += volume
}

// GetOrderRate 获取交易所当前秒操作次数
func (s *RiskStats) GetOrderRate(exchange string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.orderRates[exchange]
	if !ok {
		return 0
	}
	now := time.Now()
	count := 0
	for _, t := range w.counts {
		if now.Sub(t) < time.Second {
			count++
		}
	}
	return count
}

// RecordOperation 记录一次操作 (用于频率限制)
func (s *RiskStats) RecordOperation(exchange string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.orderRates[exchange]
	if !ok {
		w = &rateWindow{}
		s.orderRates[exchange] = w
	}
	now := time.Now()
	// 清理超过1秒的记录
	var valid []time.Time
	for _, t := range w.counts {
		if now.Sub(t) < time.Second {
			valid = append(valid, t)
		}
	}
	w.counts = append(valid, now)
}

// GetInsertOrderCount 获取报单次数
func (s *RiskStats) GetInsertOrderCount(symbol string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.insertOrderCounts[symbol]
}

// IncrInsertOrderCount 增加报单次数
func (s *RiskStats) IncrInsertOrderCount(symbol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.insertOrderCounts[symbol]++
	s.changedSymbols[symbol] = true
}

// GetCancelOrderCount 获取撤单次数
func (s *RiskStats) GetCancelOrderCount(symbol string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cancelOrderCounts[symbol]
}

// IncrCancelOrderCount 增加撤单次数
func (s *RiskStats) IncrCancelOrderCount(symbol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelOrderCounts[symbol]++
	s.changedSymbols[symbol] = true
}

// GetTradeUnits 获取成交手数
func (s *RiskStats) GetTradeUnits(symbol string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tradeUnits[symbol]
}

// AddTradeUnits 增加成交手数
func (s *RiskStats) AddTradeUnits(symbol string, volume int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tradeUnits[symbol] += volume
	s.changedSymbols[symbol] = true
}

// GetNetPosition 获取净持仓
func (s *RiskStats) GetNetPosition(symbol string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.netPositions[symbol]
}

// IncrSelfTradeRejected 增加自成交拒绝计数
func (s *RiskStats) IncrSelfTradeRejected(symbol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.getOrCreateSelfTradeStats(symbol)
	entry.RejectedCount++
	s.changedSymbols[symbol] = true
}

// IncrFreqCancellationRejected 增加频繁报撤单拒绝计数
func (s *RiskStats) IncrFreqCancellationRejected(symbol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.freqCancellationRejected[symbol]++
	s.changedSymbols[symbol] = true
}

// IncrTradePositionRatioRejected 增加成交持仓比拒绝计数
func (s *RiskStats) IncrTradePositionRatioRejected(symbol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tradePositionRatioRejected[symbol]++
	s.changedSymbols[symbol] = true
}

// UpdateSelfTradePrice 更新自成交统计中的最高买价/最低卖价
// direction: protocol.DirectionBuy (1) / protocol.DirectionSell (-1)
func (s *RiskStats) UpdateSelfTradePrice(symbol string, direction int64, price float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.getOrCreateSelfTradeStats(symbol)
	if direction == 1 { // Buy
		if price > entry.HighestBuyPrice {
			entry.HighestBuyPrice = price
			s.changedSymbols[symbol] = true
		}
	} else { // Sell
		if entry.LowestSellPrice == 0 || price < entry.LowestSellPrice {
			entry.LowestSellPrice = price
			s.changedSymbols[symbol] = true
		}
	}
}

// getOrCreateSelfTradeStats 获取或创建自成交统计条目 (调用方需持有锁)
func (s *RiskStats) getOrCreateSelfTradeStats(symbol string) *selfTradeStatsEntry {
	entry, ok := s.selfTradeStats[symbol]
	if !ok {
		entry = &selfTradeStatsEntry{
			LowestSellPrice: math.MaxFloat64,
		}
		s.selfTradeStats[symbol] = entry
	}
	return entry
}

// AddActiveOrder 添加活跃订单
func (s *RiskStats) AddActiveOrder(symbol, orderID string, direction, offset int64, price float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.activeOrders[symbol]; !ok {
		s.activeOrders[symbol] = make(map[string]*activeOrder)
	}
	s.activeOrders[symbol][orderID] = &activeOrder{
		OrderID:   orderID,
		Direction: direction,
		Offset:    offset,
		Price:     price,
	}
}

// RemoveActiveOrder 移除活跃订单
func (s *RiskStats) RemoveActiveOrder(symbol, orderID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if orders, ok := s.activeOrders[symbol]; ok {
		delete(orders, orderID)
	}
}

// HasOppositeActiveOrder 检查是否存在反向活跃订单 (自成交检测)
func (s *RiskStats) HasOppositeActiveOrder(symbol string, direction int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	orders, ok := s.activeOrders[symbol]
	if !ok {
		return false
	}
	for _, o := range orders {
		if o.Direction != direction {
			return true
		}
	}
	return false
}

// BuildChangedRiskData 构建变更的风控数据 (用于推送给客户端)
// 输出嵌套结构对齐 tqsdk-python RiskManagementData
func (s *RiskStats) BuildChangedRiskData(dumpAll bool) map[string]*RiskManagementData {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make(map[string]*RiskManagementData)

	// 收集需要推送的合约
	symbols := make(map[string]bool)
	if dumpAll {
		// 全量: 收集所有有数据的合约
		for sym := range s.insertOrderCounts {
			symbols[sym] = true
		}
		for sym := range s.tradeUnits {
			symbols[sym] = true
		}
		for sym := range s.selfTradeStats {
			symbols[sym] = true
		}
	} else {
		// 增量: 只收集变更的合约
		for sym := range s.changedSymbols {
			symbols[sym] = true
		}
	}

	for sym := range symbols {
		// 解析 exchangeID 和 instrumentID
		exchangeID, instrumentID := splitSymbol(sym)

		// 构建嵌套结构
		selfTrade := &SelfTradeData{}
		if entry, ok := s.selfTradeStats[sym]; ok {
			selfTrade.HighestBuyPrice = entry.HighestBuyPrice
			lowestSell := entry.LowestSellPrice
			if lowestSell == math.MaxFloat64 {
				lowestSell = 0
			}
			selfTrade.LowestSellPrice = lowestSell
			selfTrade.SelfTradeCount = entry.SelfTradeCount
			selfTrade.RejectedCount = entry.RejectedCount
		}

		insertCount := s.insertOrderCounts[sym]
		cancelCount := s.cancelOrderCounts[sym]
		var cancelPercent float64
		if insertCount > 0 {
			cancelPercent = float64(cancelCount) / float64(insertCount) * 100
		}

		tradeU := s.tradeUnits[sym]
		netPos := s.netPositions[sym]
		var tpRatio float64
		if netPos != 0 {
			absNet := netPos
			if absNet < 0 {
				absNet = -absNet
			}
			tpRatio = float64(tradeU) / float64(absNet) * 100
		}

		data := &RiskManagementData{
			ExchangeID:   exchangeID,
			InstrumentID: instrumentID,
			SelfTrade:    selfTrade,
			FrequentCancellation: &FrequentCancellationData{
				InsertOrderCount:   insertCount,
				CancelOrderCount:   cancelCount,
				CancelOrderPercent: cancelPercent,
				RejectedCount:      s.freqCancellationRejected[sym],
			},
			TradePositionRatio: &TradePositionRatioData{
				TradeUnits:         tradeU,
				NetPositionUnits:   netPos,
				TradePositionRatio: tpRatio,
				RejectedCount:      s.tradePositionRatioRejected[sym],
			},
		}

		result[sym] = data
	}

	// 清除变更标记
	s.changedSymbols = make(map[string]bool)

	return result
}

// splitSymbol 将 "SHFE.rb2501" 拆分为 exchangeID 和 instrumentID
func splitSymbol(symbol string) (exchangeID, instrumentID string) {
	parts := strings.SplitN(symbol, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", symbol
}
