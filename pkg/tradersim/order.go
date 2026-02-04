// Package tradersim 下单撤单、撮合引擎、成交处理
package tradersim

import (
	"fmt"
	"math"
	"time"

	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/inslist"
	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/protocol"
)

// handleInsertOrder 处理下单请求
// 对应 C++ OnClientReqInsertOrder
func (t *TraderSim) handleInsertOrder(connID int, msg string) {
	if !t.isLoggedIn.Load() {
		return
	}

	// 解析下单请求
	action := t.parseActionOrder(msg)
	if action == nil {
		return
	}

	t.userMu.Lock()
	defer t.userMu.Unlock()

	// 验证订单ID唯一性
	if _, exists := t.user.Orders[action.OrderID]; exists {
		logger.Info("下单, 已被服务器拒绝,原因:单号重复",
			zap.String("fun", "handleInsertOrder"),
			zap.String("key", t.brokerID),
			zap.String("user_name", t.reqLogin.UserName),
			zap.String("order_id", action.OrderID))
		t.outputNotifyAll(NotifyCodeDuplicateOrderID, "下单, 已被服务器拒绝,原因:单号重复", NotifyLevelWarning)
		return
	}

	// 获取合约信息
	symbol := action.ExchangeID + "." + action.InstrumentID
	ins := t.getInstrument(symbol)

	// 验证合约有效性
	if ins == nil {
		logger.Info("下单, 已被服务器拒绝, 原因:合约不合法",
			zap.String("fun", "handleInsertOrder"),
			zap.String("key", t.brokerID),
			zap.String("user_name", t.reqLogin.UserName),
			zap.String("symbol", symbol))
		t.outputNotifyAll(NotifyCodeInvalidInstrument, "下单, 已被服务器拒绝, 原因:合约不合法", NotifyLevelWarning)
		return
	}

	// 仅支持期货合约 (原版 C++ 明确限制)
	if ins.ProductClass != protocol.ProductClassFutures {
		logger.Info("下单, 已被服务器拒绝, 原因:模拟交易只支持期货合约",
			zap.String("fun", "handleInsertOrder"),
			zap.String("key", t.brokerID),
			zap.String("user_name", t.reqLogin.UserName),
			zap.String("symbol", symbol))
		t.outputNotifyAll(NotifyCodeOnlyFutures, "下单, 已被服务器拒绝, 原因:模拟交易只支持期货合约", NotifyLevelWarning)
		return
	}

	// 验证下单数量
	if action.Volume <= 0 {
		logger.Info("下单, 已被服务器拒绝, 原因:下单手数应该大于0",
			zap.String("fun", "handleInsertOrder"),
			zap.String("key", t.brokerID),
			zap.String("user_name", t.reqLogin.UserName),
			zap.Int("volume", action.Volume))
		t.outputNotifyAll(NotifyCodeInvalidVolume, "下单, 已被服务器拒绝, 原因:下单手数应该大于0", NotifyLevelWarning)
		return
	}

	// 验证价格精度
	if !math.IsNaN(ins.PriceTick) && ins.PriceTick > 0 {
		if !t.validatePriceTick(action.LimitPrice, ins.PriceTick) {
			logger.Info("下单,已被服务器拒绝, 原因:下单价格不是价格单位的整倍数",
				zap.String("fun", "handleInsertOrder"),
				zap.String("key", t.brokerID),
				zap.String("user_name", t.reqLogin.UserName),
				zap.Float64("price", action.LimitPrice),
				zap.Float64("price_tick", ins.PriceTick))
			t.outputNotifyAll(NotifyCodeInvalidPriceTick, "下单,已被服务器拒绝, 原因:下单价格不是价格单位的整倍数", NotifyLevelWarning)
			return
		}
	}

	// 创建订单 (先创建用于后续检查)
	order := t.createOrder(action, symbol)

	// 开仓检查 - 保证金是否充足
	if action.Offset == protocol.OffsetOpen {
		if !t.checkMarginForOpen(action, ins) {
			logger.Info("下单,已被服务器拒绝,原因:开仓保证金不足",
				zap.String("fun", "handleInsertOrder"),
				zap.String("key", t.brokerID),
				zap.String("user_name", t.reqLogin.UserName))
			t.outputNotifyAll(NotifyCodeInsufficientMargin, "下单,已被服务器拒绝,原因:开仓保证金不足", NotifyLevelWarning)
			order.Status = protocol.OrderStatusFinished
			return
		}
	}

	// 平仓检查 - 可平量是否充足
	if action.Offset != protocol.OffsetOpen {
		closeCheckResult := t.checkVolumeForCloseDetailed(action, symbol)
		if closeCheckResult != 0 {
			var notifyCode int64
			var notifyMsg string
			switch closeCheckResult {
			case 1: // 平今超量
				notifyCode = NotifyCodeCloseTodayExceed
				notifyMsg = "下单,已被服务器拒绝,原因:平今手数超过今仓持仓量"
			case 2: // 平昨超量
				notifyCode = NotifyCodeCloseYesterdayExceed
				notifyMsg = "下单,已被服务器拒绝,原因:平昨手数超过昨仓持仓量"
			default: // 平仓超量
				notifyCode = NotifyCodeCloseExceed
				notifyMsg = "下单,已被服务器拒绝,原因:平仓手数超过持仓量"
			}
			logger.Info(notifyMsg,
				zap.String("fun", "handleInsertOrder"),
				zap.String("key", t.brokerID),
				zap.String("user_name", t.reqLogin.UserName))
			t.outputNotifyAll(notifyCode, notifyMsg, NotifyLevelWarning)
			order.Status = protocol.OrderStatusFinished
			return
		}
	}

	// 加入用户订单
	t.user.Orders[order.OrderID] = order

	// 加入活跃订单集合
	t.aliveOrdersMu.Lock()
	t.aliveOrders[order.OrderID] = order
	t.aliveOrdersMu.Unlock()

	// 冻结资金/持仓
	t.freezeForOrder(order, ins, symbol)

	t.somethingChanged.Store(true)

	logger.Info("下单成功",
		zap.String("fun", "handleInsertOrder"),
		zap.String("key", t.brokerID),
		zap.String("user_name", t.reqLogin.UserName),
		zap.String("order_id", order.OrderID),
		zap.String("symbol", symbol),
		zap.Int64("direction", order.Direction),
		zap.Int64("offset", order.Offset),
		zap.Int("volume", order.VolumeOrign))

	t.outputNotifyAll(NotifyCodeOrderSuccess, "下单成功", NotifyLevelInfo)

	// 立即尝试撮合
	t.tryOrderMatchNoLock()

	// 保存用户数据
	if err := t.saveUserDataFile(); err != nil {
		logger.Error("sim: save user data failed",
			zap.String("fun", "handleInsertOrder"),
			zap.String("key", t.brokerID),
			zap.Error(err))
	}
}

// handleCancelOrder 处理撤单请求
// 对应 C++ OnClientReqCancelOrder
func (t *TraderSim) handleCancelOrder(connID int, msg string) {
	if !t.isLoggedIn.Load() {
		return
	}

	orderID := gjson.Get(msg, "order_id").String()
	if orderID == "" {
		return
	}

	t.userMu.Lock()
	defer t.userMu.Unlock()

	order, exists := t.user.Orders[orderID]
	if !exists {
		logger.Info("要撤销的单不存在",
			zap.String("fun", "handleCancelOrder"),
			zap.String("key", t.brokerID),
			zap.String("user_name", t.reqLogin.UserName),
			zap.String("order_id", orderID))
		t.outputNotifyAll(NotifyCodeOrderNotExist, "要撤销的单不存在", NotifyLevelWarning)
		return
	}

	if order.Status != protocol.OrderStatusAlive {
		logger.Info("要撤销的单不存在",
			zap.String("fun", "handleCancelOrder"),
			zap.String("key", t.brokerID),
			zap.String("user_name", t.reqLogin.UserName),
			zap.String("order_id", orderID))
		t.outputNotifyAll(NotifyCodeOrderNotExist, "要撤销的单不存在", NotifyLevelWarning)
		return
	}

	// 撤单
	order.Status = protocol.OrderStatusFinished
	order.LastMsg = "已撤单"
	order.Changed = true

	// 从活跃订单中移除
	t.aliveOrdersMu.Lock()
	delete(t.aliveOrders, orderID)
	t.aliveOrdersMu.Unlock()

	// 解冻资金/持仓
	t.unfreezeForOrder(order)

	t.somethingChanged.Store(true)

	logger.Info("撤单成功",
		zap.String("fun", "handleCancelOrder"),
		zap.String("key", t.brokerID),
		zap.String("user_name", t.reqLogin.UserName),
		zap.String("order_id", orderID))

	t.outputNotifyAll(NotifyCodeCancelSuccess, "撤单成功", NotifyLevelInfo)

	// 保存用户数据
	if err := t.saveUserDataFile(); err != nil {
		logger.Error("sim: save user data failed",
			zap.String("fun", "handleCancelOrder"),
			zap.String("key", t.brokerID),
			zap.Error(err))
	}
}

// parseActionOrder 解析下单请求
func (t *TraderSim) parseActionOrder(msg string) *protocol.ActionInsertOrder {
	return &protocol.ActionInsertOrder{
		OrderID:         gjson.Get(msg, "order_id").String(),
		UserID:          gjson.Get(msg, "user_id").String(),
		ExchangeID:      gjson.Get(msg, "exchange_id").String(),
		InstrumentID:    gjson.Get(msg, "instrument_id").String(),
		Direction:       gjson.Get(msg, "direction").Int(),
		Offset:          gjson.Get(msg, "offset").Int(),
		Volume:          int(gjson.Get(msg, "volume").Int()),
		PriceType:       gjson.Get(msg, "price_type").Int(),
		LimitPrice:      gjson.Get(msg, "limit_price").Float(),
		VolumeCondition: gjson.Get(msg, "volume_condition").Int(),
		TimeCondition:   gjson.Get(msg, "time_condition").Int(),
		HedgeFlag:       gjson.Get(msg, "hedge_flag").Int(),
	}
}

// validatePriceTick 验证价格是否是最小变动价位的整数倍
func (t *TraderSim) validatePriceTick(price, priceTick float64) bool {
	if priceTick <= 0 || math.IsNaN(priceTick) {
		return true
	}
	remainder := math.Mod(price, priceTick)
	return math.Abs(remainder) < 1e-9 || math.Abs(remainder-priceTick) < 1e-9
}

// checkMarginForOpen 检查开仓保证金是否充足
func (t *TraderSim) checkMarginForOpen(action *protocol.ActionInsertOrder, ins *inslist.InstrumentInfo) bool {
	account := t.getAccountNoLock()
	if account == nil {
		return false
	}

	// 计算所需保证金
	price := action.LimitPrice
	volume := float64(action.Volume)

	var volumeMultiple float64 = 10 // 默认值
	var marginRate float64 = 0.1    // 默认 10%

	if ins != nil {
		if ins.VolumeMultiple > 0 {
			volumeMultiple = float64(ins.VolumeMultiple)
		}
		if ins.Margin > 0 {
			marginRate = ins.Margin
		}
	}

	requiredMargin := price * volume * volumeMultiple * marginRate

	return account.Available >= requiredMargin
}

// checkVolumeForClose 检查平仓可平量是否充足
func (t *TraderSim) checkVolumeForClose(action *protocol.ActionInsertOrder, symbol string) bool {
	return t.checkVolumeForCloseDetailed(action, symbol) == 0
}

// checkVolumeForCloseDetailed 检查平仓可平量是否充足 (返回详细错误码)
// 返回值: 0=OK, 1=平今超量, 2=平昨超量, 3=平仓超量
func (t *TraderSim) checkVolumeForCloseDetailed(action *protocol.ActionInsertOrder, symbol string) int {
	pos, ok := t.user.Positions[symbol]
	if !ok {
		return 3
	}

	volume := action.Volume
	exchangeID := action.ExchangeID
	isSHFEorINE := exchangeID == "SHFE" || exchangeID == "INE"

	if action.Direction == protocol.DirectionBuy {
		// 买平 (平空仓)
		if isSHFEorINE {
			if action.Offset == protocol.OffsetCloseToday {
				if pos.VolumeShortToday-pos.VolumeShortFrozenToday < volume {
					return 1 // 平今超量
				}
				return 0
			}
			if pos.VolumeShortHis-pos.VolumeShortFrozenHis < volume {
				return 2 // 平昨超量
			}
			return 0
		}
		if pos.VolumeShort-pos.VolumeShortFrozen < volume {
			return 3 // 平仓超量
		}
		return 0
	}

	// 卖平 (平多仓)
	if isSHFEorINE {
		if action.Offset == protocol.OffsetCloseToday {
			if pos.VolumeLongToday-pos.VolumeLongFrozenToday < volume {
				return 1 // 平今超量
			}
			return 0
		}
		if pos.VolumeLongHis-pos.VolumeLongFrozenHis < volume {
			return 2 // 平昨超量
		}
		return 0
	}
	if pos.VolumeLong-pos.VolumeLongFrozen < volume {
		return 3 // 平仓超量
	}
	return 0
}

// createOrder 创建订单
func (t *TraderSim) createOrder(action *protocol.ActionInsertOrder, symbol string) *protocol.Order {
	order := protocol.NewOrder()
	order.UserID = t.user.UserID
	order.OrderID = action.OrderID
	order.ExchangeID = action.ExchangeID
	order.InstrumentID = action.InstrumentID
	order.Direction = action.Direction
	order.Offset = action.Offset
	order.VolumeOrign = action.Volume
	order.VolumeLeft = action.Volume
	order.PriceType = action.PriceType
	order.LimitPrice = action.LimitPrice
	order.VolumeCondition = action.VolumeCondition
	order.TimeCondition = action.TimeCondition
	order.InsertDateTime = time.Now().UnixNano()
	order.Status = protocol.OrderStatusAlive
	order.Seqno = int(t.orderSeq.Add(1))
	order.Changed = true

	return order
}

// freezeForOrder 冻结资金/持仓
func (t *TraderSim) freezeForOrder(order *protocol.Order, ins *inslist.InstrumentInfo, symbol string) {
	if order.Offset == protocol.OffsetOpen {
		// 开仓：冻结保证金
		account := t.getAccountNoLock()
		if account == nil {
			return
		}

		price := order.LimitPrice
		volume := float64(order.VolumeLeft)

		var volumeMultiple float64 = 10
		var marginRate float64 = 0.1

		if ins != nil {
			if ins.VolumeMultiple > 0 {
				volumeMultiple = float64(ins.VolumeMultiple)
			}
			if ins.Margin > 0 {
				marginRate = ins.Margin
			}
		}

		frozenMargin := price * volume * volumeMultiple * marginRate
		order.FrozenMargin = frozenMargin
		account.FrozenMargin += frozenMargin
		account.Available -= frozenMargin
		account.Changed = true
	} else {
		// 平仓：冻结持仓
		pos := t.user.Positions[symbol]
		if pos == nil {
			return
		}

		volume := order.VolumeLeft
		exchangeID := order.ExchangeID
		isSHFEorINE := exchangeID == "SHFE" || exchangeID == "INE"

		if order.Direction == protocol.DirectionBuy {
			// 买平 (平空仓)
			if isSHFEorINE {
				if order.Offset == protocol.OffsetCloseToday {
					pos.VolumeShortFrozenToday += volume
				} else {
					pos.VolumeShortFrozenHis += volume
				}
			}
			pos.VolumeShortFrozen += volume
		} else {
			// 卖平 (平多仓)
			if isSHFEorINE {
				if order.Offset == protocol.OffsetCloseToday {
					pos.VolumeLongFrozenToday += volume
				} else {
					pos.VolumeLongFrozenHis += volume
				}
			}
			pos.VolumeLongFrozen += volume
		}
		pos.Changed = true
	}
}

// unfreezeForOrder 解冻资金/持仓 (撤单时调用)
func (t *TraderSim) unfreezeForOrder(order *protocol.Order) {
	symbol := order.Symbol()

	if order.Offset == protocol.OffsetOpen {
		// 开仓：解冻保证金
		account := t.getAccountNoLock()
		if account != nil && order.FrozenMargin > 0 {
			account.FrozenMargin -= order.FrozenMargin
			account.Available += order.FrozenMargin
			account.Changed = true
		}
	} else {
		// 平仓：解冻持仓
		pos := t.user.Positions[symbol]
		if pos == nil {
			return
		}

		volume := order.VolumeLeft
		exchangeID := order.ExchangeID
		isSHFEorINE := exchangeID == "SHFE" || exchangeID == "INE"

		if order.Direction == protocol.DirectionBuy {
			if isSHFEorINE {
				if order.Offset == protocol.OffsetCloseToday {
					pos.VolumeShortFrozenToday -= volume
				} else {
					pos.VolumeShortFrozenHis -= volume
				}
			}
			pos.VolumeShortFrozen -= volume
		} else {
			if isSHFEorINE {
				if order.Offset == protocol.OffsetCloseToday {
					pos.VolumeLongFrozenToday -= volume
				} else {
					pos.VolumeLongFrozenHis -= volume
				}
			}
			pos.VolumeLongFrozen -= volume
		}
		pos.Changed = true
	}
}

// tryOrderMatch 尝试撮合所有活跃订单
// 对应 C++ TryOrderMatch
func (t *TraderSim) tryOrderMatch() {
	t.userMu.Lock()
	defer t.userMu.Unlock()
	t.tryOrderMatchNoLock()
}

// tryOrderMatchNoLock 尝试撮合 (不加锁)
func (t *TraderSim) tryOrderMatchNoLock() {
	t.aliveOrdersMu.Lock()
	defer t.aliveOrdersMu.Unlock()

	for _, order := range t.aliveOrders {
		t.checkOrderTrade(order)
	}
}

// checkOrderTrade 检查单个订单是否可以成交
// 对应 C++ CheckOrderTrade
func (t *TraderSim) checkOrderTrade(order *protocol.Order) {
	symbol := order.Symbol()
	ins := t.getInstrument(symbol)

	// 从 marketfeed 获取行情
	if t.marketClient == nil {
		return
	}
	quote := t.marketClient.GetQuote(symbol)
	if quote == nil {
		return
	}

	// 检查行情有效性
	if math.IsNaN(quote.AskPrice1) || math.IsNaN(quote.BidPrice1) {
		return
	}
	if quote.AskPrice1 <= 0 || quote.BidPrice1 <= 0 {
		return
	}

	// 限价单价格检查
	if order.PriceType == protocol.PriceTypeLimit {
		// 获取涨跌停价
		upperLimit := quote.UpperLimit
		lowerLimit := quote.LowerLimit

		// 超过涨停价 - 拒绝
		if order.Direction == protocol.DirectionBuy &&
			!math.IsNaN(upperLimit) && upperLimit > 0 &&
			order.LimitPrice > upperLimit {
			t.rejectOrder(order, NotifyCodePriceAboveUpperLimit, "下单,已被服务器拒绝,原因:已撤单报单被拒绝价格超出涨停板")
			return
		}
		// 低于跌停价 - 拒绝
		if order.Direction == protocol.DirectionSell &&
			!math.IsNaN(lowerLimit) && lowerLimit > 0 &&
			order.LimitPrice < lowerLimit {
			t.rejectOrder(order, NotifyCodePriceBelowLowerLimit, "下单,已被服务器拒绝,原因:已撤单报单被拒绝价格跌破跌停板")
			return
		}
	}

	// 买单撮合条件
	if order.Direction == protocol.DirectionBuy {
		if order.PriceType == protocol.PriceTypeAny ||
			order.LimitPrice >= quote.AskPrice1 {
			t.doTrade(order, order.VolumeLeft, quote.AskPrice1, ins)
		}
	}

	// 卖单撮合条件
	if order.Direction == protocol.DirectionSell {
		if order.PriceType == protocol.PriceTypeAny ||
			order.LimitPrice <= quote.BidPrice1 {
			t.doTrade(order, order.VolumeLeft, quote.BidPrice1, ins)
		}
	}
}

// rejectOrder 拒绝订单
func (t *TraderSim) rejectOrder(order *protocol.Order, code int64, reason string) {
	order.Status = protocol.OrderStatusFinished
	order.LastMsg = reason
	order.Changed = true

	delete(t.aliveOrders, order.OrderID)

	// 解冻
	t.unfreezeForOrder(order)

	t.somethingChanged.Store(true)

	logger.Info(reason,
		zap.String("fun", "rejectOrder"),
		zap.String("key", t.brokerID),
		zap.String("user_name", t.reqLogin.UserName),
		zap.String("order_id", order.OrderID))

	t.outputNotifyAll(code, reason, NotifyLevelWarning)
}

// doTrade 执行成交
// 对应 C++ DoTrade
func (t *TraderSim) doTrade(order *protocol.Order, volume int, price float64, ins *inslist.InstrumentInfo) {
	symbol := order.Symbol()

	var volumeMultiple float64 = 10
	if ins != nil && ins.VolumeMultiple > 0 {
		volumeMultiple = float64(ins.VolumeMultiple)
	}

	// 1. 创建成交记录
	trade := t.createTrade(order, volume, price)
	t.user.Trades[trade.TradeID] = trade

	// 2. 调整委托单
	order.VolumeLeft -= volume
	if order.VolumeLeft <= 0 {
		order.Status = protocol.OrderStatusFinished
		delete(t.aliveOrders, order.OrderID)
	}
	order.Changed = true

	// 3. 获取或创建持仓
	pos := t.getOrCreatePositionNoLock(symbol, order.ExchangeID, order.InstrumentID)

	// 4. 调整持仓数据
	if order.Offset == protocol.OffsetOpen {
		t.handleOpenTrade(pos, order, volume, price, volumeMultiple)
	} else {
		t.handleCloseTrade(pos, order, volume, price, volumeMultiple, ins)
	}

	// 5. 计算手续费
	commission := t.calculateCommission(ins, volume, price)
	trade.Commission = commission

	// 6. 调整账户资金
	account := t.getAccountNoLock()
	if account != nil {
		account.Commission += commission

		// 解冻开仓保证金
		if order.Offset == protocol.OffsetOpen && order.FrozenMargin > 0 {
			unfreezeMargin := order.FrozenMargin * float64(volume) / float64(order.VolumeOrign)
			account.FrozenMargin -= unfreezeMargin
			order.FrozenMargin -= unfreezeMargin
		}

		account.Changed = true
	}

	t.somethingChanged.Store(true)

	// 发送成交通知 (对应 C++ 406)
	tradeNotifyMsg := fmt.Sprintf("成交通知,合约:%s.%s,手数:%d", order.ExchangeID, order.InstrumentID, volume)
	logger.Info(tradeNotifyMsg,
		zap.String("fun", "doTrade"),
		zap.String("key", t.brokerID),
		zap.String("user_name", t.reqLogin.UserName),
		zap.String("trade_id", trade.TradeID),
		zap.String("order_id", order.OrderID),
		zap.Int("volume", volume),
		zap.Float64("price", price))

	t.outputNotifyAll(NotifyCodeTradeNotify, tradeNotifyMsg, NotifyLevelInfo)
}

// createTrade 创建成交记录
func (t *TraderSim) createTrade(order *protocol.Order, volume int, price float64) *protocol.Trade {
	trade := protocol.NewTrade()
	trade.UserID = order.UserID
	trade.TradeID = fmt.Sprintf("SIM%d", t.tradeSeq.Add(1))
	trade.ExchangeID = order.ExchangeID
	trade.InstrumentID = order.InstrumentID
	trade.OrderID = order.OrderID
	trade.ExchangeTradeID = trade.TradeID
	trade.Direction = int(order.Direction)
	trade.Offset = int(order.Offset)
	trade.Volume = volume
	trade.Price = price
	trade.TradeDateTime = time.Now().UnixNano()
	trade.Changed = true

	return trade
}

// handleOpenTrade 处理开仓成交
func (t *TraderSim) handleOpenTrade(pos *protocol.Position, order *protocol.Order,
	volume int, price float64, volumeMultiple float64) {

	cost := price * float64(volume) * volumeMultiple

	if order.Direction == protocol.DirectionBuy {
		// 多开
		pos.VolumeLongToday += volume
		pos.VolumeLong += volume
		pos.OpenCostLong += cost
		pos.OpenCostLongToday += cost
		pos.PositionCostLong += cost
		pos.PositionCostLongToday += cost
	} else {
		// 空开
		pos.VolumeShortToday += volume
		pos.VolumeShort += volume
		pos.OpenCostShort += cost
		pos.OpenCostShortToday += cost
		pos.PositionCostShort += cost
		pos.PositionCostShortToday += cost
	}

	// 计算保证金
	var marginRate float64 = 0.1
	ins := t.getInstrument(pos.Symbol())
	if ins != nil && ins.Margin > 0 {
		marginRate = ins.Margin
	}
	pos.Margin += cost * marginRate

	pos.Changed = true

	// 订阅新合约行情
	if t.marketClient != nil {
		t.marketClient.Subscribe(pos.Symbol())
	}
}

// handleCloseTrade 处理平仓成交
func (t *TraderSim) handleCloseTrade(pos *protocol.Position, order *protocol.Order,
	volume int, price float64, volumeMultiple float64, ins *inslist.InstrumentInfo) {

	account := t.getAccountNoLock()
	exchangeID := order.ExchangeID
	isSHFEorINE := exchangeID == "SHFE" || exchangeID == "INE"

	if order.Direction == protocol.DirectionBuy {
		// 买平 (平空仓)
		closeProfit := t.calculateCloseProfitShort(pos, volume, price, volumeMultiple)
		if account != nil {
			account.CloseProfit += closeProfit
		}

		if isSHFEorINE {
			if order.Offset == protocol.OffsetCloseToday {
				t.closeShortToday(pos, volume, price, volumeMultiple)
			} else {
				t.closeShortHis(pos, volume, price, volumeMultiple)
			}
		} else {
			t.closeShortAuto(pos, volume, price, volumeMultiple)
		}

		// 解冻
		if isSHFEorINE {
			if order.Offset == protocol.OffsetCloseToday {
				pos.VolumeShortFrozenToday -= volume
			} else {
				pos.VolumeShortFrozenHis -= volume
			}
		}
		pos.VolumeShortFrozen -= volume
	} else {
		// 卖平 (平多仓)
		closeProfit := t.calculateCloseProfitLong(pos, volume, price, volumeMultiple)
		if account != nil {
			account.CloseProfit += closeProfit
		}

		if isSHFEorINE {
			if order.Offset == protocol.OffsetCloseToday {
				t.closeLongToday(pos, volume, price, volumeMultiple)
			} else {
				t.closeLongHis(pos, volume, price, volumeMultiple)
			}
		} else {
			t.closeLongAuto(pos, volume, price, volumeMultiple)
		}

		// 解冻
		if isSHFEorINE {
			if order.Offset == protocol.OffsetCloseToday {
				pos.VolumeLongFrozenToday -= volume
			} else {
				pos.VolumeLongFrozenHis -= volume
			}
		}
		pos.VolumeLongFrozen -= volume
	}

	if account != nil {
		account.Changed = true
	}
	pos.Changed = true
}

// calculateCommission 计算手续费
func (t *TraderSim) calculateCommission(ins *inslist.InstrumentInfo, volume int, price float64) float64 {
	if ins == nil || ins.Commission <= 0 {
		return 0
	}

	// 简化：按手数计算
	return ins.Commission * float64(volume)
}

// calculateCloseProfitLong 计算平多仓盈亏
func (t *TraderSim) calculateCloseProfitLong(pos *protocol.Position, volume int, price float64, volumeMultiple float64) float64 {
	if pos.VolumeLong <= 0 {
		return 0
	}
	avgCost := pos.PositionCostLong / (float64(pos.VolumeLong) * volumeMultiple)
	return (price - avgCost) * float64(volume) * volumeMultiple
}

// calculateCloseProfitShort 计算平空仓盈亏
func (t *TraderSim) calculateCloseProfitShort(pos *protocol.Position, volume int, price float64, volumeMultiple float64) float64 {
	if pos.VolumeShort <= 0 {
		return 0
	}
	avgCost := pos.PositionCostShort / (float64(pos.VolumeShort) * volumeMultiple)
	return (avgCost - price) * float64(volume) * volumeMultiple
}

// closeLongToday 平多今仓
func (t *TraderSim) closeLongToday(pos *protocol.Position, volume int, price float64, volumeMultiple float64) {
	if pos.VolumeLongToday < volume {
		volume = pos.VolumeLongToday
	}
	if volume <= 0 {
		return
	}

	cost := price * float64(volume) * volumeMultiple
	ratio := float64(volume) / float64(pos.VolumeLongToday)

	pos.VolumeLongToday -= volume
	pos.VolumeLong -= volume
	pos.OpenCostLongToday -= pos.OpenCostLongToday * ratio
	pos.OpenCostLong -= pos.OpenCostLongToday * ratio
	pos.PositionCostLongToday -= pos.PositionCostLongToday * ratio
	pos.PositionCostLong -= pos.PositionCostLongToday * ratio
	pos.Margin -= cost * 0.1 // 简化保证金计算
}

// closeLongHis 平多昨仓
func (t *TraderSim) closeLongHis(pos *protocol.Position, volume int, price float64, volumeMultiple float64) {
	if pos.VolumeLongHis < volume {
		volume = pos.VolumeLongHis
	}
	if volume <= 0 {
		return
	}

	cost := price * float64(volume) * volumeMultiple
	ratio := float64(volume) / float64(pos.VolumeLongHis)

	pos.VolumeLongHis -= volume
	pos.VolumeLong -= volume
	pos.OpenCostLongHis -= pos.OpenCostLongHis * ratio
	pos.OpenCostLong -= pos.OpenCostLongHis * ratio
	pos.PositionCostLongHis -= pos.PositionCostLongHis * ratio
	pos.PositionCostLong -= pos.PositionCostLongHis * ratio
	pos.Margin -= cost * 0.1
}

// closeLongAuto 自动平多仓 (优先平今)
func (t *TraderSim) closeLongAuto(pos *protocol.Position, volume int, price float64, volumeMultiple float64) {
	// 优先平今
	todayClose := volume
	if todayClose > pos.VolumeLongToday {
		todayClose = pos.VolumeLongToday
	}
	if todayClose > 0 {
		t.closeLongToday(pos, todayClose, price, volumeMultiple)
		volume -= todayClose
	}

	// 再平昨
	if volume > 0 {
		t.closeLongHis(pos, volume, price, volumeMultiple)
	}
}

// closeShortToday 平空今仓
func (t *TraderSim) closeShortToday(pos *protocol.Position, volume int, price float64, volumeMultiple float64) {
	if pos.VolumeShortToday < volume {
		volume = pos.VolumeShortToday
	}
	if volume <= 0 {
		return
	}

	cost := price * float64(volume) * volumeMultiple
	ratio := float64(volume) / float64(pos.VolumeShortToday)

	pos.VolumeShortToday -= volume
	pos.VolumeShort -= volume
	pos.OpenCostShortToday -= pos.OpenCostShortToday * ratio
	pos.OpenCostShort -= pos.OpenCostShortToday * ratio
	pos.PositionCostShortToday -= pos.PositionCostShortToday * ratio
	pos.PositionCostShort -= pos.PositionCostShortToday * ratio
	pos.Margin -= cost * 0.1
}

// closeShortHis 平空昨仓
func (t *TraderSim) closeShortHis(pos *protocol.Position, volume int, price float64, volumeMultiple float64) {
	if pos.VolumeShortHis < volume {
		volume = pos.VolumeShortHis
	}
	if volume <= 0 {
		return
	}

	cost := price * float64(volume) * volumeMultiple
	ratio := float64(volume) / float64(pos.VolumeShortHis)

	pos.VolumeShortHis -= volume
	pos.VolumeShort -= volume
	pos.OpenCostShortHis -= pos.OpenCostShortHis * ratio
	pos.OpenCostShort -= pos.OpenCostShortHis * ratio
	pos.PositionCostShortHis -= pos.PositionCostShortHis * ratio
	pos.PositionCostShort -= pos.PositionCostShortHis * ratio
	pos.Margin -= cost * 0.1
}

// closeShortAuto 自动平空仓 (优先平今)
func (t *TraderSim) closeShortAuto(pos *protocol.Position, volume int, price float64, volumeMultiple float64) {
	// 优先平今
	todayClose := volume
	if todayClose > pos.VolumeShortToday {
		todayClose = pos.VolumeShortToday
	}
	if todayClose > 0 {
		t.closeShortToday(pos, todayClose, price, volumeMultiple)
		volume -= todayClose
	}

	// 再平昨
	if volume > 0 {
		t.closeShortHis(pos, volume, price, volumeMultiple)
	}
}
