// Package trader 订单管理
// 对应 C++ traderctp 中的订单相关逻辑
package trader

import (
	"fmt"
	"strconv"
	"time"

	"github.com/pseudocodes/go2ctp/thost"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/protocol"
)

// handleInsertOrderFull 处理下单请求
// 对应 C++ traderctp::OnClientReqInsertOrder
// 支持 C++ 风格的字符串枚举值和整数枚举值两种报文格式
func (t *TraderCTP) handleInsertOrderFull(connID int, msg string) {
	if !t.isLoggedIn() {
		t.outputNotify(connID, 333, "请先登录", "WARNING")
		return
	}

	// 解析下单请求 - 兼容字符串枚举值 (C++ 风格) 和整数枚举值
	order := &protocol.ActionInsertOrder{
		OrderID:      gjson.Get(msg, "order_id").String(),
		UserID:       gjson.Get(msg, "user_id").String(),
		ExchangeID:   gjson.Get(msg, "exchange_id").String(),
		InstrumentID: gjson.Get(msg, "instrument_id").String(),
		Volume:       int(gjson.Get(msg, "volume").Int()),
		LimitPrice:   gjson.Get(msg, "limit_price").Float(),
	}

	// 方向 - 支持 "BUY"/"SELL" 字符串或 1/-1 整数
	order.Direction = protocol.ParseEnumValue(
		gjson.Get(msg, "direction"),
		protocol.ParseDirection,
		protocol.DirectionUnknown,
	)

	// 开平 - 支持 "OPEN"/"CLOSE"/"CLOSETODAY" 字符串或 1/-1/-2 整数
	order.Offset = protocol.ParseEnumValue(
		gjson.Get(msg, "offset"),
		protocol.ParseOffset,
		protocol.OffsetUnknown,
	)

	// 价格类型 - 支持 "LIMIT"/"ANY"/"BEST"/"FIVELEVEL" 字符串或 1/2/3/4 整数
	order.PriceType = protocol.ParseEnumValue(
		gjson.Get(msg, "price_type"),
		protocol.ParsePriceType,
		protocol.PriceTypeLimit,
	)

	// 成交量条件 - 支持 "ANY"/"MIN"/"ALL" 字符串或 1/2/3 整数
	order.VolumeCondition = protocol.ParseEnumValue(
		gjson.Get(msg, "volume_condition"),
		protocol.ParseVolumeCondition,
		protocol.OrderVolumeConditionAny,
	)

	// 有效期类型 - 支持 "IOC"/"GFS"/"GFD"/"GTD"/"GTC"/"GFA" 字符串或 1-6 整数
	order.TimeCondition = protocol.ParseEnumValue(
		gjson.Get(msg, "time_condition"),
		protocol.ParseTimeCondition,
		protocol.OrderTimeConditionGFD,
	)

	// 投机套保标志 - 支持 "SPECULATION"/"ARBITRAGE"/"HEDGE"/"MARKETMAKER" 字符串或 1-4 整数
	order.HedgeFlag = protocol.ParseEnumValue(
		gjson.Get(msg, "hedge_flag"),
		protocol.ParseHedgeFlag,
		protocol.HedgeFlagSpeculation,
	)

	// 触发条件 - 支持 "IMMEDIATELY"/"TOUCH"/"TOUCHPROFIT" 字符串或 1/2/3 整数
	order.ContingentCondition = protocol.ParseEnumValue(
		gjson.Get(msg, "contingent_condition"),
		protocol.ParseContingentCondition,
		protocol.ContingentConditionImmediately,
	)

	// 验证必要字段
	if order.InstrumentID == "" {
		t.outputNotify(connID, 1, "合约代码不能为空", "ERROR")
		return
	}
	if order.Volume <= 0 {
		t.outputNotify(connID, 1, "下单数量必须大于0", "ERROR")
		return
	}

	logger.Info("insert order request",
		zap.String("order_id", order.OrderID),
		zap.String("instrument_id", order.InstrumentID),
		zap.Int64("direction", order.Direction),
		zap.Int64("offset", order.Offset),
		zap.Int("volume", order.Volume),
		zap.Float64("limit_price", order.LimitPrice),
	)

	// 发送 CTP 下单请求
	t.ctpSpi.ReqOrderInsert(order)
}

// ReqOrderInsert 发送下单请求
// 对应 C++ traderctp 中的下单逻辑
func (s *CtpSpi) ReqOrderInsert(order *protocol.ActionInsertOrder) {
	req := &thost.CThostFtdcInputOrderField{}

	copy(req.BrokerID[:], s.brokerID)
	copy(req.InvestorID[:], s.userID)
	copy(req.ExchangeID[:], order.ExchangeID)
	copy(req.InstrumentID[:], order.InstrumentID)

	// 设置订单引用
	// 使用 CtpSpi 的 nextOrderRef 方法，确保从 CTP MaxOrderRef 开始递增
	orderRef := s.nextOrderRef()
	copy(req.OrderRef[:], orderRef)

	// 方向
	if order.Direction == protocol.DirectionBuy {
		req.Direction = thost.THOST_FTDC_D_Buy
	} else {
		req.Direction = thost.THOST_FTDC_D_Sell
	}

	// 开平标志
	switch order.Offset {
	case protocol.OffsetOpen:
		req.CombOffsetFlag[0] = byte(thost.THOST_FTDC_OF_Open)
	case protocol.OffsetClose:
		req.CombOffsetFlag[0] = byte(thost.THOST_FTDC_OF_Close)
	case protocol.OffsetCloseToday:
		req.CombOffsetFlag[0] = byte(thost.THOST_FTDC_OF_CloseToday)
	default:
		req.CombOffsetFlag[0] = byte(thost.THOST_FTDC_OF_Close)
	}

	// 投机套保标志 - 对应 C++ ctp_define.cpp:112-118
	switch order.HedgeFlag {
	case protocol.HedgeFlagSpeculation:
		req.CombHedgeFlag[0] = byte(thost.THOST_FTDC_HF_Speculation)
	case protocol.HedgeFlagArbitrage:
		req.CombHedgeFlag[0] = byte(thost.THOST_FTDC_HF_Arbitrage)
	case protocol.HedgeFlagHedge:
		req.CombHedgeFlag[0] = byte(thost.THOST_FTDC_HF_Hedge)
	case protocol.HedgeFlagMarketMaker:
		req.CombHedgeFlag[0] = byte(thost.THOST_FTDC_HF_MarketMaker)
	default:
		req.CombHedgeFlag[0] = byte(thost.THOST_FTDC_HF_Speculation) // 默认值
	}

	// 价格类型 - 对应 C++ ctp_define.cpp:93-98
	switch order.PriceType {
	case protocol.PriceTypeLimit:
		req.OrderPriceType = thost.THOST_FTDC_OPT_LimitPrice
		req.LimitPrice = thost.TThostFtdcPriceType(order.LimitPrice)
	case protocol.PriceTypeAny:
		req.OrderPriceType = thost.THOST_FTDC_OPT_AnyPrice
		req.LimitPrice = 0
	case protocol.PriceTypeBest:
		req.OrderPriceType = thost.THOST_FTDC_OPT_BestPrice
		req.LimitPrice = 0
	case protocol.PriceTypeFiveLevel:
		req.OrderPriceType = thost.THOST_FTDC_OPT_FiveLevelPrice
		req.LimitPrice = 0
	default:
		req.OrderPriceType = thost.THOST_FTDC_OPT_LimitPrice
		req.LimitPrice = thost.TThostFtdcPriceType(order.LimitPrice)
	}

	// 数量
	req.VolumeTotalOriginal = thost.TThostFtdcVolumeType(order.Volume)

	// 有效期类型 - 对应 C++ ctp_define.cpp:104-111
	switch order.TimeCondition {
	case protocol.OrderTimeConditionIOC:
		req.TimeCondition = thost.THOST_FTDC_TC_IOC
	case protocol.OrderTimeConditionGFS:
		req.TimeCondition = thost.THOST_FTDC_TC_GFS
	case protocol.OrderTimeConditionGFD:
		req.TimeCondition = thost.THOST_FTDC_TC_GFD
	case protocol.OrderTimeConditionGTD:
		req.TimeCondition = thost.THOST_FTDC_TC_GTD
	case protocol.OrderTimeConditionGTC:
		req.TimeCondition = thost.THOST_FTDC_TC_GTC
	case protocol.OrderTimeConditionGFA:
		req.TimeCondition = thost.THOST_FTDC_TC_GFA
	default:
		req.TimeCondition = thost.THOST_FTDC_TC_GFD // 默认值
	}

	// 成交量类型
	switch order.VolumeCondition {
	case protocol.OrderVolumeConditionAny:
		req.VolumeCondition = thost.THOST_FTDC_VC_AV
	case protocol.OrderVolumeConditionMin:
		req.VolumeCondition = thost.THOST_FTDC_VC_MV
	case protocol.OrderVolumeConditionAll:
		req.VolumeCondition = thost.THOST_FTDC_VC_CV
	default:
		req.VolumeCondition = thost.THOST_FTDC_VC_AV
	}

	req.MinVolume = 1

	// 触发条件 - 对应 C++ ctp_define.cpp:119-124
	switch order.ContingentCondition {
	case protocol.ContingentConditionImmediately:
		req.ContingentCondition = thost.THOST_FTDC_CC_Immediately
	case protocol.ContingentConditionTouch:
		req.ContingentCondition = thost.THOST_FTDC_CC_Touch
	case protocol.ContingentConditionTouchProfit:
		req.ContingentCondition = thost.THOST_FTDC_CC_TouchProfit
	default:
		req.ContingentCondition = thost.THOST_FTDC_CC_Immediately // 默认值
	}

	req.ForceCloseReason = thost.THOST_FTDC_FCC_NotForceClose
	req.IsAutoSuspend = 0
	req.UserForceClose = 0

	// 添加到 insertOrderSet (跟踪待确认的下单)
	// 对应 C++ m_insert_order_set.insert(f.OrderRef)
	s.trader.insertOrderSetMu.Lock()
	s.trader.insertOrderSet[orderRef] = true
	s.trader.insertOrderSetMu.Unlock()

	// 构建订单 key 并添加到 inputOrderKeyMap
	// 对应 C++ strKey 和 m_input_order_key_map
	strKey := fmt.Sprintf("%d_%d_%s", s.frontID, s.sessionID, orderRef)
	serverOrderInfo := &ServerOrderInfo{
		ExchangeID:   order.ExchangeID,
		InstrumentID: order.InstrumentID,
		OrderRef:     orderRef,
		Direction:    byte(req.Direction),
		OffsetFlag:   req.CombOffsetFlag[0],
		PriceType:    byte(req.OrderPriceType),
		LimitPrice:   order.LimitPrice,
		VolumeLeft:   order.Volume,
		VolumeTotal:  order.Volume,
	}
	s.trader.inputOrderKeyMapMu.Lock()
	s.trader.inputOrderKeyMap[strKey] = serverOrderInfo
	s.trader.inputOrderKeyMapMu.Unlock()

	ret := s.api.ReqOrderInsert(req, s.nextRequestID())
	logger.Info("ReqOrderInsert",
		zap.Int("ret", ret),
		zap.String("order_ref", orderRef),
		zap.String("instrument_id", order.InstrumentID),
		zap.String("order_key", strKey),
	)
}

// OnRspOrderInsert 下单响应（仅在错误时调用）
// 对应 C++ traderctp::OnRspOrderInsert
func (s *CtpSpi) OnRspOrderInsert(pInputOrder *thost.CThostFtdcInputOrderField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if pInputOrder != nil {
		instrumentID := bytesToString(pInputOrder.InstrumentID[:])
		orderRef := bytesToString(pInputOrder.OrderRef[:])
		logger.Error("order insert failed",
			zap.String("instrument_id", instrumentID),
			zap.String("order_ref", orderRef),
		)
	}

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		s.trader.outputNotifyAll(int64(pRspInfo.ErrorID), gbkToUtf8(pRspInfo.ErrorMsg[:]), "ERROR")
	}
}

// OnRtnOrder 订单回报
// 对应 C++ traderctp::OnRtnOrder -> ProcessRtnOrder
func (s *CtpSpi) OnRtnOrder(pOrder *thost.CThostFtdcOrderField) {
	if pOrder == nil {
		return
	}

	orderID := s.makeOrderID(pOrder)
	exchangeOrderID := bytesToString(pOrder.OrderSysID[:])
	instrumentID := bytesToString(pOrder.InstrumentID[:])

	logger.Info("order return",
		zap.String("order_id", orderID),
		zap.String("exchange_order_id", exchangeOrderID),
		zap.String("instrument_id", instrumentID),
		zap.Int32("volume_total", int32(pOrder.VolumeTotalOriginal)),
		zap.Int32("volume_traded", int32(pOrder.VolumeTraded)),
		zap.Int32("volume_total_original", int32(pOrder.VolumeTotalOriginal)),
		zap.Uint8("status", uint8(pOrder.OrderStatus)),
	)

	// 更新订单数据
	s.trader.updateOrder(pOrder)
}

// OnRtnTrade 成交回报
// 对应 C++ traderctp::OnRtnTrade -> ProcessRtnTrade
func (s *CtpSpi) OnRtnTrade(pTrade *thost.CThostFtdcTradeField) {
	if pTrade == nil {
		return
	}

	tradeID := bytesToString(pTrade.TradeID[:])
	instrumentID := bytesToString(pTrade.InstrumentID[:])

	logger.Info("trade return",
		zap.String("trade_id", tradeID),
		zap.String("instrument_id", instrumentID),
		zap.Int32("volume", int32(pTrade.Volume)),
		zap.Float64("price", float64(pTrade.Price)),
	)

	// 更新成交数据
	s.trader.updateTrade(pTrade)
}

// OnErrRtnOrderInsert 下单错误回报
// 对应 C++ traderctp::OnErrRtnOrderInsert
func (s *CtpSpi) OnErrRtnOrderInsert(pInputOrder *thost.CThostFtdcInputOrderField,
	pRspInfo *thost.CThostFtdcRspInfoField) {

	if pInputOrder != nil {
		instrumentID := bytesToString(pInputOrder.InstrumentID[:])
		logger.Error("order insert error",
			zap.String("instrument_id", instrumentID),
		)
	}

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		s.trader.outputNotifyAll(int64(pRspInfo.ErrorID), gbkToUtf8(pRspInfo.ErrorMsg[:]), "ERROR")
	}
}

// makeOrderID 生成订单 ID
func (s *CtpSpi) makeOrderID(pOrder *thost.CThostFtdcOrderField) string {
	return fmt.Sprintf("%d_%d_%s",
		pOrder.FrontID,
		pOrder.SessionID,
		bytesToString(pOrder.OrderRef[:]))
}

// handleCancelOrderFull 处理撤单请求
// 对应 C++ traderctp::OnClientReqCancelOrder
func (t *TraderCTP) handleCancelOrderFull(connID int, msg string) {
	if !t.isLoggedIn() {
		t.outputNotify(connID, 334, "请先登录", "WARNING")
		return
	}

	orderID := gjson.Get(msg, "order_id").String()
	if orderID == "" {
		t.outputNotify(connID, 1, "订单ID不能为空", "ERROR")
		return
	}

	logger.Info("cancel order request",
		zap.String("order_id", orderID),
	)

	// 查找订单
	t.userMu.RLock()
	order, exists := t.user.Orders[orderID]
	t.userMu.RUnlock()

	if !exists {
		t.outputNotify(connID, 1, "未找到订单", "ERROR")
		return
	}

	t.ctpSpi.ReqOrderAction(order)
}

// ReqOrderAction 发送撤单请求
func (s *CtpSpi) ReqOrderAction(order *protocol.Order) {
	req := &thost.CThostFtdcInputOrderActionField{}

	copy(req.BrokerID[:], s.brokerID)
	copy(req.InvestorID[:], s.userID)
	copy(req.ExchangeID[:], order.ExchangeID)
	copy(req.InstrumentID[:], order.InstrumentID)

	// 解析订单引用
	parts := parseOrderID(order.OrderID)
	if len(parts) >= 3 {
		req.FrontID = thost.TThostFtdcFrontIDType(parts[0])
		req.SessionID = thost.TThostFtdcSessionIDType(parts[1])
		copy(req.OrderRef[:], strconv.Itoa(parts[2]))
	}

	req.ActionFlag = thost.THOST_FTDC_AF_Delete

	// 添加到 cancelOrderSet (跟踪待确认的撤单)
	// 对应 C++ m_cancel_order_set.insert(order.order_id)
	s.trader.cancelOrderSetMu.Lock()
	s.trader.cancelOrderSet[order.OrderID] = true
	s.trader.cancelOrderSetMu.Unlock()

	ret := s.api.ReqOrderAction(req, s.nextRequestID())
	logger.Info("ReqOrderAction",
		zap.Int("ret", ret),
		zap.String("order_id", order.OrderID),
	)
}

// parseOrderID 解析订单 ID (格式: frontID_sessionID_orderRef)
func parseOrderID(orderID string) []int {
	var result []int
	var current int
	for _, c := range orderID {
		if c == '_' {
			result = append(result, current)
			current = 0
		} else if c >= '0' && c <= '9' {
			current = current*10 + int(c-'0')
		}
	}
	result = append(result, current)
	return result
}

// OnRspOrderAction 撤单响应（仅在错误时调用）
func (s *CtpSpi) OnRspOrderAction(pInputOrderAction *thost.CThostFtdcInputOrderActionField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		logger.Error("order action failed",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])),
		)
		s.trader.outputNotifyAll(int64(pRspInfo.ErrorID), gbkToUtf8(pRspInfo.ErrorMsg[:]), "ERROR")
	}
}

// OnErrRtnOrderAction 撤单错误回报
func (s *CtpSpi) OnErrRtnOrderAction(pOrderAction *thost.CThostFtdcOrderActionField,
	pRspInfo *thost.CThostFtdcRspInfoField) {

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		s.trader.outputNotifyAll(int64(pRspInfo.ErrorID), gbkToUtf8(pRspInfo.ErrorMsg[:]), "ERROR")
	}
}

// updateOrder 更新订单数据
// 对应 C++ traderctp::ProcessRtnOrder
func (t *TraderCTP) updateOrder(pOrder *thost.CThostFtdcOrderField) {
	t.userMu.Lock()
	defer t.userMu.Unlock()

	if t.user == nil {
		return
	}

	orderID := t.ctpSpi.makeOrderID(pOrder)
	orderRef := bytesToString(pOrder.OrderRef[:])
	exchangeID := bytesToString(pOrder.ExchangeID[:])
	instrumentID := bytesToString(pOrder.InstrumentID[:])
	orderSysID := bytesToString(pOrder.OrderSysID[:])
	orderLocalID := bytesToString(pOrder.OrderLocalID[:])
	lastMsg := gbkToUtf8(pOrder.StatusMsg[:])

	// 构建订单 key
	strKey := fmt.Sprintf("%d_%d_%s", pOrder.FrontID, pOrder.SessionID, orderRef)

	order, exists := t.user.Orders[orderID]
	if !exists {
		order = protocol.NewOrder()
		order.OrderID = orderID
		t.user.Orders[orderID] = order
	}

	// 更新订单信息
	order.UserID = t.user.UserID
	order.ExchangeID = exchangeID
	order.InstrumentID = instrumentID
	order.ExchangeOrderID = orderSysID

	// 方向
	if pOrder.Direction == thost.THOST_FTDC_D_Buy {
		order.Direction = protocol.DirectionBuy
	} else {
		order.Direction = protocol.DirectionSell
	}

	// 开平
	switch thost.TThostFtdcOffsetFlagType(pOrder.CombOffsetFlag[0]) {
	case thost.THOST_FTDC_OF_Open:
		order.Offset = protocol.OffsetOpen
	case thost.THOST_FTDC_OF_Close:
		order.Offset = protocol.OffsetClose
	case thost.THOST_FTDC_OF_CloseToday:
		order.Offset = protocol.OffsetCloseToday
	}

	order.VolumeOrign = int(int32(pOrder.VolumeTotalOriginal))
	order.VolumeLeft = int(int32(pOrder.VolumeTotal))
	order.LimitPrice = float64(pOrder.LimitPrice)

	// 订单状态
	switch pOrder.OrderStatus {
	case thost.THOST_FTDC_OST_AllTraded,
		thost.THOST_FTDC_OST_Canceled:
		order.Status = protocol.OrderStatusFinished
	default:
		order.Status = protocol.OrderStatusAlive
	}

	order.LastMsg = lastMsg

	// 解析时间
	insertDate := bytesToString(pOrder.InsertDate[:])
	insertTime := bytesToString(pOrder.InsertTime[:])
	order.InsertDateTime = parseDatetime(insertDate, insertTime)

	order.Changed = true

	// ==================== 发送下单成功通知 ====================
	// 对应 C++ tradectp.cpp:2649-2743
	if pOrder.OrderStatus != thost.THOST_FTDC_OST_Canceled &&
		pOrder.OrderStatus != thost.THOST_FTDC_OST_Unknown &&
		pOrder.OrderStatus != thost.THOST_FTDC_OST_NoTradeNotQueueing &&
		pOrder.OrderStatus != thost.THOST_FTDC_OST_PartTradedNotQueueing {

		t.insertOrderSetMu.Lock()
		if _, found := t.insertOrderSet[orderRef]; found {
			delete(t.insertOrderSet, orderRef)
			t.insertOrderSetMu.Unlock()

			// 构建并发送下单成功通知
			notifyMsg := buildOrderNotifyMessage(pOrder, "下单成功")
			t.outputNotifyAll(328, notifyMsg, "INFO")
		} else {
			t.insertOrderSetMu.Unlock()
		}

		// 更新 inputOrderKeyMap 中的 OrderLocalID 和 OrderSysID
		t.inputOrderKeyMapMu.Lock()
		if serverInfo, ok := t.inputOrderKeyMap[strKey]; ok {
			serverInfo.OrderLocalID = orderLocalID
			serverInfo.OrderSysID = orderSysID
		}
		t.inputOrderKeyMapMu.Unlock()
	}

	// ==================== 发送撤单成功/下单失败通知 ====================
	// 对应 C++ tradectp.cpp:2745-2920
	if pOrder.OrderStatus == thost.THOST_FTDC_OST_Canceled && pOrder.VolumeTotal > 0 {
		// 先检查是否是撤单成功
		t.cancelOrderSetMu.Lock()
		if _, found := t.cancelOrderSet[orderID]; found {
			delete(t.cancelOrderSet, orderID)
			t.cancelOrderSetMu.Unlock()

			// 撤单成功通知 (code 329)
			notifyMsg := buildOrderNotifyMessage(pOrder, "撤单成功")
			t.outputNotifyAll(329, notifyMsg, "INFO")

			// TODO: CheckConditionOrderCancelOrderTask(order.order_id)

			// 删除 inputOrderKeyMap 中的记录
			t.inputOrderKeyMapMu.Lock()
			delete(t.inputOrderKeyMap, strKey)
			t.inputOrderKeyMapMu.Unlock()
		} else {
			t.cancelOrderSetMu.Unlock()

			// 检查是否是下单失败
			t.insertOrderSetMu.Lock()
			if _, found := t.insertOrderSet[orderRef]; found {
				delete(t.insertOrderSet, orderRef)
				t.insertOrderSetMu.Unlock()

				// 下单失败通知 (code 330, WARNING)
				notifyMsg := "下单失败," + lastMsg + "，" + buildOrderNotifyMessageBody(pOrder)
				t.outputNotifyAll(330, notifyMsg, "WARNING")
			} else {
				t.insertOrderSetMu.Unlock()
			}

			// 删除 inputOrderKeyMap 中的记录
			t.inputOrderKeyMapMu.Lock()
			delete(t.inputOrderKeyMap, strKey)
			t.inputOrderKeyMapMu.Unlock()
		}
	}

	// 请求刷新持仓和账户 (对应 C++ m_req_position_id++; m_req_account_id++;)
	go t.requestRefreshAfterOrder()

	// 通知客户端
	t.notifyOrderUpdate()
}

// updateTrade 更新成交数据
// 对应 C++ traderctp::ProcessRtnTrade
func (t *TraderCTP) updateTrade(pTrade *thost.CThostFtdcTradeField) {
	t.userMu.Lock()
	defer t.userMu.Unlock()

	if t.user == nil {
		return
	}

	exchangeID := bytesToString(pTrade.ExchangeID[:])
	instrumentID := bytesToString(pTrade.InstrumentID[:])
	orderSysID := bytesToString(pTrade.OrderSysID[:])

	tradeID := t.makeTradeID(pTrade)
	trade, exists := t.user.Trades[tradeID]
	if !exists {
		trade = protocol.NewTrade()
		trade.TradeID = tradeID
		t.user.Trades[tradeID] = trade
	}

	trade.UserID = t.user.UserID
	trade.ExchangeID = exchangeID
	trade.InstrumentID = instrumentID
	trade.OrderID = bytesToString(pTrade.OrderRef[:])
	trade.ExchangeTradeID = bytesToString(pTrade.TradeID[:])

	// 方向
	if pTrade.Direction == thost.THOST_FTDC_D_Buy {
		trade.Direction = protocol.DirectionBuy
	} else {
		trade.Direction = protocol.DirectionSell
	}

	// 开平
	switch pTrade.OffsetFlag {
	case thost.THOST_FTDC_OF_Open:
		trade.Offset = protocol.OffsetOpen
	case thost.THOST_FTDC_OF_Close:
		trade.Offset = protocol.OffsetClose
	case thost.THOST_FTDC_OF_CloseToday:
		trade.Offset = protocol.OffsetCloseToday
	}

	trade.Volume = int(int32(pTrade.Volume))
	trade.Price = float64(pTrade.Price)

	// 解析时间 (带 DCE/CZCE 夜盘特殊处理)
	tradeDate := bytesToString(pTrade.TradeDate[:])
	tradeTime := bytesToString(pTrade.TradeTime[:])
	trade.TradeDateTime = parseTradeDatetime(trade.ExchangeID, tradeDate, tradeTime)

	trade.Changed = true

	// ==================== 发送成交通知 ====================
	// 对应 C++ tradectp.cpp:2974-3037
	t.inputOrderKeyMapMu.Lock()
	for strKey, serverInfo := range t.inputOrderKeyMap {
		if serverInfo.ExchangeID == exchangeID && serverInfo.OrderSysID == orderSysID {
			// 构建成交通知消息
			notifyMsg := buildTradeNotifyMessage(serverInfo, pTrade)
			t.outputNotifyAll(331, notifyMsg, "INFO")

			// 更新剩余数量
			serverInfo.VolumeLeft -= int(pTrade.Volume)
			if serverInfo.VolumeLeft <= 0 {
				delete(t.inputOrderKeyMap, strKey)
			}
			break
		}
	}
	t.inputOrderKeyMapMu.Unlock()

	// 请求刷新持仓和账户 (对应 C++ m_req_position_id++; m_req_account_id++;)
	go t.requestRefreshAfterTrade()

	// 调整持仓 (已初始化后才实时调整)
	if t.positionInited {
		go t.adjustPositionByTrade(trade)
	}

	// 订阅该合约的行情 (用于计算持仓盈亏)
	symbol := trade.ExchangeID + "." + trade.InstrumentID
	t.subscribeSymbolIfNeeded(symbol)

	// 通知客户端
	t.notifyTradeUpdate()
}

// makeTradeID 生成成交 ID
func (t *TraderCTP) makeTradeID(pTrade *thost.CThostFtdcTradeField) string {
	return fmt.Sprintf("%s_%s_%c",
		bytesToString(pTrade.ExchangeID[:]),
		bytesToString(pTrade.TradeID[:]),
		byte(pTrade.Direction))
}

// parseDatetime 解析日期时间
// 对应 C++ ProcessRtnTrade 中的日期时间解析逻辑 (line 3105-3145)
func parseDatetime(date, tm string) int64 {
	if date == "" || tm == "" {
		return 0
	}
	layout := "20060102150405"
	t, err := time.Parse(layout, date+tm)
	if err != nil {
		return 0
	}
	return t.UnixNano()
}

// parseTradeDatetime 解析成交日期时间 (带 DCE/CZCE 夜盘特殊处理)
// 对应 C++ ProcessRtnTrade 中的日期时间解析逻辑 (line 3105-3145)
// DCE/CZCE 夜盘成交时间特殊处理：
// - 夜盘时间 (20:30-23:59) 如果当前已是白盘时间，需要回退到上一个工作日
func parseTradeDatetime(exchangeID, tradeDate, tradeTime string) int64 {
	if tradeTime == "" {
		return 0
	}

	// 解析成交时间 HH:MM:SS
	var hour, minute, second int
	fmt.Sscanf(tradeTime, "%02d:%02d:%02d", &hour, &minute, &second)

	isDCEorCZCE := exchangeID == "CZCE" || exchangeID == "DCE"

	if isDCEorCZCE {
		nTime := hour*100 + minute
		// 夜盘时间判断 (20:30 - 23:59)
		if nTime > 2030 && nTime < 2359 {
			now := time.Now()
			nLocalTime := now.Hour()*100 + now.Minute()

			// 现在还是夜盘时间
			if nLocalTime > 2030 && nLocalTime <= 2359 {
				// 使用今天日期
				year, month, day := now.Year(), int(now.Month()), now.Day()
				t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)
				return t.UnixNano()
			} else {
				// 现在已经是白盘时间了，成交发生在上一个工作日
				// 跳到上一个工作日
				prevDay := moveDateByWorkday(now, -1)
				year, month, day := prevDay.Year(), int(prevDay.Month()), prevDay.Day()
				t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)
				return t.UnixNano()
			}
		}
	}

	// 非夜盘或非 DCE/CZCE：使用 tradeDate
	if tradeDate == "" {
		return 0
	}
	var year, month, day int
	fmt.Sscanf(tradeDate, "%04d%02d%02d", &year, &month, &day)
	t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)
	return t.UnixNano()
}

// moveDateByWorkday 按工作日移动日期
// 对应 C++ MoveDateByWorkday
func moveDateByWorkday(t time.Time, days int) time.Time {
	if days == 0 {
		return t
	}

	step := 1
	if days < 0 {
		step = -1
		days = -days
	}

	for days > 0 {
		t = t.AddDate(0, 0, step)
		// 跳过周末
		weekday := t.Weekday()
		if weekday == time.Saturday || weekday == time.Sunday {
			continue
		}
		days--
	}
	return t
}

// notifyOrderUpdate 通知订单更新
func (t *TraderCTP) notifyOrderUpdate() {
	// 订单数据已在 buildUserDataMsg 中发送
	// 此处可以添加额外的通知逻辑（如推送到条件单服务等）
}

// notifyTradeUpdate 通知成交更新
func (t *TraderCTP) notifyTradeUpdate() {
	// 成交数据已在 buildUserDataMsg 中发送
	// 此处可以添加额外的通知逻辑
}

// ==================== 通知消息构建辅助函数 ====================
// 对应 C++ tradectp.cpp 中的通知消息构建逻辑

// buildOrderNotifyMessage 构建订单通知消息
// 对应 C++ tradectp.cpp:2661-2731 的通知消息构建
func buildOrderNotifyMessage(pOrder *thost.CThostFtdcOrderField, prefix string) string {
	return prefix + "，" + buildOrderNotifyMessageBody(pOrder)
}

// buildOrderNotifyMessageBody 构建订单通知消息体
// 对应 C++ tradectp.cpp:2661-2731 ss_1 构建的部分
func buildOrderNotifyMessageBody(pOrder *thost.CThostFtdcOrderField) string {
	exchangeID := bytesToString(pOrder.ExchangeID[:])
	instrumentID := bytesToString(pOrder.InstrumentID[:])

	// 合约代码
	strInstrumentID := fmt.Sprintf("合约代码:%s.%s", exchangeID, instrumentID)

	// 下单方向
	var strDirection string
	if pOrder.Direction == thost.THOST_FTDC_D_Buy {
		strDirection = "下单方向:买"
	} else {
		strDirection = "下单方向:卖"
	}

	// 开平标志
	strOffsetFlag := getOffsetFlagText(pOrder.CombOffsetFlag[0])

	// 委托价格
	strPrice := getPriceTypeText(pOrder.OrderPriceType, float64(pOrder.LimitPrice))

	// 委托手数
	strVolume := fmt.Sprintf("委托手数:%d", pOrder.VolumeTotalOriginal)

	return fmt.Sprintf("%s，%s，%s，%s，%s",
		strInstrumentID, strDirection, strOffsetFlag, strPrice, strVolume)
}

// buildTradeNotifyMessage 构建成交通知消息
// 对应 C++ tradectp.cpp:2981-3027
func buildTradeNotifyMessage(serverInfo *ServerOrderInfo, pTrade *thost.CThostFtdcTradeField) string {
	// 合约代码
	strInstrumentID := fmt.Sprintf("合约代码:%s.%s", serverInfo.ExchangeID, serverInfo.InstrumentID)

	// 下单方向
	var strDirection string
	if pTrade.Direction == thost.THOST_FTDC_D_Buy {
		strDirection = "下单方向:买"
	} else {
		strDirection = "下单方向:卖"
	}

	// 开平标志
	strOffsetFlag := getOffsetFlagTextFromTrade(pTrade.OffsetFlag)

	// 成交价格和手数
	strPriceVolume := fmt.Sprintf("成交价格:%v,成交手数:%d",
		float64(pTrade.Price), pTrade.Volume)

	return fmt.Sprintf("成交通知,%s,%s,%s,%s",
		strInstrumentID, strDirection, strOffsetFlag, strPriceVolume)
}

// getOffsetFlagText 获取开平标志文本
// 对应 C++ tradectp.cpp:2673-2704 的开平标志映射
func getOffsetFlagText(offsetFlag byte) string {
	switch thost.TThostFtdcOffsetFlagType(offsetFlag) {
	case thost.THOST_FTDC_OF_Open:
		return "开平标志:开仓"
	case thost.THOST_FTDC_OF_Close:
		return "开平标志:平仓"
	case thost.THOST_FTDC_OF_ForceClose:
		return "开平标志:强平"
	case thost.THOST_FTDC_OF_CloseToday:
		return "开平标志:平今"
	case thost.THOST_FTDC_OF_CloseYesterday:
		return "开平标志:平昨"
	case thost.THOST_FTDC_OF_ForceOff:
		return "开平标志:强减"
	case thost.THOST_FTDC_OF_LocalForceClose:
		return "开平标志:本地强平"
	default:
		return "开平标志:未知"
	}
}

// getOffsetFlagTextFromTrade 从成交记录获取开平标志文本
// 对应 C++ tradectp.cpp:2991-3022 的开平标志映射
func getOffsetFlagTextFromTrade(offsetFlag thost.TThostFtdcOffsetFlagType) string {
	switch offsetFlag {
	case thost.THOST_FTDC_OF_Open:
		return "开平标志:开仓"
	case thost.THOST_FTDC_OF_Close:
		return "开平标志:平仓"
	case thost.THOST_FTDC_OF_ForceClose:
		return "开平标志:强平"
	case thost.THOST_FTDC_OF_CloseToday:
		return "开平标志:平今"
	case thost.THOST_FTDC_OF_CloseYesterday:
		return "开平标志:平昨"
	case thost.THOST_FTDC_OF_ForceOff:
		return "开平标志:强减"
	case thost.THOST_FTDC_OF_LocalForceClose:
		return "开平标志:本地强平"
	default:
		return "开平标志:未知"
	}
}

// getPriceTypeText 获取价格类型文本
// 对应 C++ tradectp.cpp:2707-2726 的价格类型映射
func getPriceTypeText(priceType thost.TThostFtdcOrderPriceTypeType, limitPrice float64) string {
	switch priceType {
	case thost.THOST_FTDC_OPT_AnyPrice:
		return "委托价格:市价"
	case thost.THOST_FTDC_OPT_BestPrice:
		return "委托价格:最优价"
	case thost.THOST_FTDC_OPT_FiveLevelPrice:
		return "委托价格:五档最优"
	case thost.THOST_FTDC_OPT_LimitPrice:
		return fmt.Sprintf("委托价格:%v", limitPrice)
	default:
		return "委托价格:未知"
	}
}
