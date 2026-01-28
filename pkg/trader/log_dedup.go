// Package trader 日志去重
// 对应 C++ traderctp 中的日志去重映射表
// Phase 6: Log Deduplication
package trader

import (
	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/protocol"
	"sync"

	"go.uber.org/zap"
)

// LogDeduplicator 日志去重器
// 对应 C++ m_rtn_order_log_map, m_rtn_trade_log_map 等
type LogDeduplicator struct {
	// 订单日志去重
	orderLogMap sync.Map
	// 成交日志去重
	tradeLogMap sync.Map
	// 转账日志去重
	transferLogMap sync.Map
	// 错误日志去重
	errorLogMap sync.Map
}

// NewLogDeduplicator 创建日志去重器
func NewLogDeduplicator() *LogDeduplicator {
	return &LogDeduplicator{}
}

// ShouldLogOrder 检查是否应该记录订单日志
// 返回 true 表示这是新的日志内容，应该记录
// 返回 false 表示这是重复的日志内容，应该跳过
func (d *LogDeduplicator) ShouldLogOrder(orderKey string, content string) bool {
	if existing, loaded := d.orderLogMap.LoadOrStore(orderKey, content); loaded {
		// 如果已存在且内容相同，跳过
		if existing.(string) == content {
			return false
		}
		// 内容不同，更新并返回 true
		d.orderLogMap.Store(orderKey, content)
	}
	return true
}

// ShouldLogTrade 检查是否应该记录成交日志
func (d *LogDeduplicator) ShouldLogTrade(tradeKey string, content string) bool {
	if existing, loaded := d.tradeLogMap.LoadOrStore(tradeKey, content); loaded {
		if existing.(string) == content {
			return false
		}
		d.tradeLogMap.Store(tradeKey, content)
	}
	return true
}

// ShouldLogTransfer 检查是否应该记录转账日志
func (d *LogDeduplicator) ShouldLogTransfer(transferKey string, content string) bool {
	if existing, loaded := d.transferLogMap.LoadOrStore(transferKey, content); loaded {
		if existing.(string) == content {
			return false
		}
		d.transferLogMap.Store(transferKey, content)
	}
	return true
}

// ShouldLogError 检查是否应该记录错误日志
// 对应 C++ m_err_rtn_order_insert_log_map 等
func (d *LogDeduplicator) ShouldLogError(errorKey string, content string) bool {
	if existing, loaded := d.errorLogMap.LoadOrStore(errorKey, content); loaded {
		if existing.(string) == content {
			return false
		}
		d.errorLogMap.Store(errorKey, content)
	}
	return true
}

// Clear 清空所有日志记录
// 在重连或新交易日时调用
func (d *LogDeduplicator) Clear() {
	d.orderLogMap = sync.Map{}
	d.tradeLogMap = sync.Map{}
	d.transferLogMap = sync.Map{}
	d.errorLogMap = sync.Map{}
}

// ==================== 持仓调整 (Phase 6: Position Adjustment) ====================

// initPositionVolume 初始化持仓数量
// 对应 C++ traderctp::InitPositionVolume
// 在查询完持仓后调用，用于初始化今昨仓位
func (t *TraderCTP) initPositionVolume() {
	t.userMu.Lock()
	defer t.userMu.Unlock()

	if t.user == nil {
		return
	}

	for _, pos := range t.user.Positions {
		// 初始化：昨仓 = volume_yd, 今仓 = 0
		// 对应 C++: pos_long_his = volume_long_yd; pos_long_today = 0;
		pos.PosLongToday = 0
		pos.PosLongHis = pos.VolumeLongYd
		pos.PosShortToday = 0
		pos.PosShortHis = pos.VolumeShortYd
		pos.Changed = true
	}
}

// replayTradesBySeqno 按 seqno 排序重放成交调整持仓
// 对应 C++ InitPositionVolume 中的成交排序和重放逻辑
func (t *TraderCTP) replayTradesBySeqno() {
	t.userMu.Lock()
	defer t.userMu.Unlock()

	if t.user == nil {
		return
	}

	// 收集所有成交并按 seqno 排序
	type tradeWithSeq struct {
		seqno int
		trade *protocol.Trade
	}

	trades := make([]tradeWithSeq, 0, len(t.user.Trades))
	for _, trade := range t.user.Trades {
		trades = append(trades, tradeWithSeq{
			seqno: trade.Seqno,
			trade: trade,
		})
	}

	// 按 seqno 排序
	for i := 0; i < len(trades)-1; i++ {
		for j := i + 1; j < len(trades); j++ {
			if trades[i].seqno > trades[j].seqno {
				trades[i], trades[j] = trades[j], trades[i]
			}
		}
	}

	// 按时序重放成交调整持仓
	for _, tw := range trades {
		t.adjustPositionByTradeInternal(tw.trade)
	}

	logger.Info("trades replayed by seqno",
		zap.Int("trade_count", len(trades)),
	)
}

// adjustPositionByTrade 根据成交调整持仓 (带锁版本)
// 对应 C++ traderctp::AdjustPositionByTrade
func (t *TraderCTP) adjustPositionByTrade(trade *protocol.Trade) {
	t.userMu.Lock()
	defer t.userMu.Unlock()
	t.adjustPositionByTradeInternal(trade)
}

// adjustPositionByTradeInternal 内部版本 (不带锁)
// 对应 C++ traderctp::AdjustPositionByTrade
func (t *TraderCTP) adjustPositionByTradeInternal(trade *protocol.Trade) {
	if t.user == nil || trade == nil {
		return
	}

	posKey := trade.ExchangeID + "." + trade.InstrumentID
	pos, exists := t.user.Positions[posKey]
	if !exists {
		// 新持仓
		pos = protocol.NewPosition()
		pos.UserID = t.user.UserID
		pos.ExchangeID = trade.ExchangeID
		pos.InstrumentID = trade.InstrumentID
		t.user.Positions[posKey] = pos
	}

	volume := trade.Volume

	if trade.Offset == protocol.OffsetOpen {
		// 开仓
		if trade.Direction == protocol.DirectionBuy {
			pos.PosLongToday += volume
		} else {
			pos.PosShortToday += volume
		}
	} else {
		// 平仓
		// 上期所/能源中心非平今 -> 平昨仓
		isSHFEorINE := trade.ExchangeID == "SHFE" || trade.ExchangeID == "INE"
		isCloseYd := isSHFEorINE && trade.Offset != protocol.OffsetCloseToday

		if trade.Direction == protocol.DirectionBuy {
			// 买平 -> 减少空仓
			if isCloseYd {
				pos.PosShortHis -= volume
			} else {
				pos.PosShortToday -= volume
			}
		} else {
			// 卖平 -> 减少多仓
			if isCloseYd {
				pos.PosLongHis -= volume
			} else {
				pos.PosLongToday -= volume
			}
		}

		// 检查负数持仓并借调 (对应 C++ 负数持仓处理)
		if pos.PosShortToday+pos.PosShortHis < 0 || pos.PosLongToday+pos.PosLongHis < 0 {
			logger.Error("position went negative after trade adjustment",
				zap.String("position_key", posKey),
				zap.Int("pos_short_today", pos.PosShortToday),
				zap.Int("pos_short_his", pos.PosShortHis),
				zap.Int("pos_long_today", pos.PosLongToday),
				zap.Int("pos_long_his", pos.PosLongHis),
			)
			return
		}

		// 借调处理
		if pos.PosShortToday < 0 {
			pos.PosShortHis += pos.PosShortToday
			pos.PosShortToday = 0
		}
		if pos.PosShortHis < 0 {
			pos.PosShortToday += pos.PosShortHis
			pos.PosShortHis = 0
		}
		if pos.PosLongToday < 0 {
			pos.PosLongHis += pos.PosLongToday
			pos.PosLongToday = 0
		}
		if pos.PosLongHis < 0 {
			pos.PosLongToday += pos.PosLongHis
			pos.PosLongHis = 0
		}
	}

	pos.Changed = true

	// 仅在已初始化后记录日志
	if t.positionInited {
		logger.Debug("position adjusted by trade",
			zap.String("position_key", posKey),
			zap.Int("pos_long_today", pos.PosLongToday),
			zap.Int("pos_long_his", pos.PosLongHis),
			zap.Int("pos_short_today", pos.PosShortToday),
			zap.Int("pos_short_his", pos.PosShortHis),
		)
	}
}

