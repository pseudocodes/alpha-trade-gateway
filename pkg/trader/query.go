// Package trader 查询功能
// 对应 C++ traderctp 中的查询相关逻辑
package trader

import (
	"time"

	"github.com/pseudocodes/go2ctp/thost"
	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/protocol"
)

// startQueryDataFull 开始查询数据
// 对应 C++ traderctp 中的数据初始化查询
func (t *TraderCTP) startQueryDataFull() {
	logger.Info("starting data query")
	time.Sleep(time.Second)
	// 查询资金账户
	t.ctpSpi.ReqQryTradingAccount()

}

// ReqQryTradingAccount 查询资金账户
// 对应 C++ traderctp::ReqQryAccount
func (s *CtpSpi) ReqQryTradingAccount() {
	req := &thost.CThostFtdcQryTradingAccountField{}
	copy(req.BrokerID[:], s.brokerID)
	copy(req.InvestorID[:], s.userID)

	ret := s.api.ReqQryTradingAccount(req, s.nextRequestID())
	logger.Info("ReqQryTradingAccount", zap.Int("ret", ret))
}

// OnRspQryTradingAccount 查询资金账户响应
// 对应 C++ traderctp::OnRspQryTradingAccount -> ProcessQryTradingAccount
func (s *CtpSpi) OnRspQryTradingAccount(pTradingAccount *thost.CThostFtdcTradingAccountField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if s.isErrorRspInfo(pRspInfo) {
		return
	}

	if pTradingAccount != nil {
		s.trader.updateAccount(pTradingAccount)
	}

	if bIsLast {
		logger.Info("trading account query completed")
		// 查询持仓
		time.Sleep(time.Second)
		s.ReqQryInvestorPosition()
	}
}

// ReqQryInvestorPosition 查询持仓
// 对应 C++ traderctp::ReqQryPosition
func (s *CtpSpi) ReqQryInvestorPosition() {
	req := &thost.CThostFtdcQryInvestorPositionField{}
	copy(req.BrokerID[:], s.brokerID)
	copy(req.InvestorID[:], s.userID)

	ret := s.api.ReqQryInvestorPosition(req, s.nextRequestID())
	logger.Info("ReqQryInvestorPosition", zap.Int("ret", ret))
}

// OnRspQryInvestorPosition 查询持仓响应
// 对应 C++ traderctp::OnRspQryInvestorPosition
func (s *CtpSpi) OnRspQryInvestorPosition(pInvestorPosition *thost.CThostFtdcInvestorPositionField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if s.isErrorRspInfo(pRspInfo) {
		return
	}

	if pInvestorPosition != nil {
		s.trader.updatePosition(pInvestorPosition)
	}

	if bIsLast {
		logger.Info("investor position query completed")
		// 查询订单
		time.Sleep(time.Second)
		s.ReqQryOrder()
	}
}

// ReqQryOrder 查询订单
func (s *CtpSpi) ReqQryOrder() {
	req := &thost.CThostFtdcQryOrderField{}
	copy(req.BrokerID[:], s.brokerID)
	copy(req.InvestorID[:], s.userID)

	ret := s.api.ReqQryOrder(req, s.nextRequestID())
	logger.Info("ReqQryOrder", zap.Int("ret", ret))
}

// OnRspQryOrder 查询订单响应
func (s *CtpSpi) OnRspQryOrder(pOrder *thost.CThostFtdcOrderField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if s.isErrorRspInfo(pRspInfo) {
		return
	}

	if pOrder != nil {
		s.trader.updateOrder(pOrder)
	}

	if bIsLast {
		logger.Info("order query completed")
		// 查询成交
		s.ReqQryTrade()
	}
}

// ReqQryTrade 查询成交
func (s *CtpSpi) ReqQryTrade() {
	req := &thost.CThostFtdcQryTradeField{}
	copy(req.BrokerID[:], s.brokerID)
	copy(req.InvestorID[:], s.userID)

	ret := s.api.ReqQryTrade(req, s.nextRequestID())
	logger.Info("ReqQryTrade", zap.Int("ret", ret))
}

// OnRspQryTrade 查询成交响应
func (s *CtpSpi) OnRspQryTrade(pTrade *thost.CThostFtdcTradeField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if s.isErrorRspInfo(pRspInfo) {
		return
	}

	if pTrade != nil {
		s.trader.updateTrade(pTrade)
	}

	if bIsLast {
		logger.Info("trade query completed, all data ready")

		// 首次查询完成后初始化持仓
		// 对应 C++ ProcessQryInvestorPosition 中的 InitPositionVolume 调用
		// 由于 Go 版使用 THOST_TERT_QUICK 模式不会主动推送 Order/Trade
		// 所以需要在成交查询完成后进行持仓初始化
		if !s.trader.positionInited {
			s.trader.initPositionVolume()
			s.trader.replayTradesBySeqno() // 按 seqno 重放成交调整持仓
			s.trader.positionInited = true
			logger.Info("position volume initialized")

			s.trader.queryScheduler.SetNeedQueryBrokerParams(true)
			s.trader.queryScheduler.SetNeedQueryBank(true)
			s.trader.queryScheduler.SetNeedQueryRegister(true)
		}
		s.trader.setState(StateReady)
		// 订阅持仓合约行情 (用于计算持仓盈亏)
		s.trader.subscribePositionSymbols()
		// 发送用户数据给客户端
		s.trader.sendAllUserData()

	}
}

// updateAccount 更新账户数据
// 对应 C++ traderctp::ProcessQryTradingAccount
func (t *TraderCTP) updateAccount(pAccount *thost.CThostFtdcTradingAccountField) {
	t.userMu.Lock()
	defer t.userMu.Unlock()

	if t.user == nil {
		return
	}

	accountID := bytesToString(pAccount.AccountID[:])
	currency := bytesToString(pAccount.CurrencyID[:])
	if currency == "" {
		currency = "CNY"
	}

	account, exists := t.user.Accounts[accountID]
	if !exists {
		account = protocol.NewAccount()
		account.UserID = t.user.UserID
		t.user.Accounts[currency] = account
	}

	account.Currency = currency
	account.PreBalance = float64(pAccount.PreBalance)
	account.Deposit = float64(pAccount.Deposit)
	account.Withdraw = float64(pAccount.Withdraw)
	account.CloseProfit = float64(pAccount.CloseProfit)
	account.Commission = float64(pAccount.Commission)
	account.StaticBalance = float64(pAccount.PreBalance + pAccount.Deposit - pAccount.Withdraw)
	account.PositionProfit = float64(pAccount.PositionProfit)
	account.FloatProfit = float64(pAccount.PositionProfit)
	account.Balance = float64(pAccount.Balance)
	account.Margin = float64(pAccount.CurrMargin)
	account.FrozenMargin = float64(pAccount.FrozenMargin)
	account.FrozenCommission = float64(pAccount.FrozenCommission)
	account.FrozenPremium = float64(pAccount.FrozenCash)
	account.Available = float64(pAccount.Available)

	// 计算风险度
	if account.Balance > 0 {
		account.RiskRatio = account.Margin / account.Balance
	}

	account.Changed = true

	logger.Debug("account updated",
		zap.String("account_id", accountID),
		zap.Float64("balance", account.Balance),
		zap.Float64("available", account.Available),
	)
}

// updatePosition 更新持仓数据
// 对应 C++ traderctp::ProcessQryInvestorPosition
// 修复：按 PositionDate 区分今仓/昨仓，支持 SHFE/INE 交易所特殊处理
func (t *TraderCTP) updatePosition(pPosition *thost.CThostFtdcInvestorPositionField) {
	t.userMu.Lock()
	defer t.userMu.Unlock()

	if t.user == nil {
		return
	}

	exchangeID := pPosition.ExchangeID.String()
	instrumentID := pPosition.InstrumentID.String()
	positionKey := exchangeID + "." + instrumentID

	position, exists := t.user.Positions[positionKey]
	if !exists {
		position = protocol.NewPosition()
		position.UserID = t.user.UserID
		position.ExchangeID = exchangeID
		position.InstrumentID = instrumentID
		t.user.Positions[positionKey] = position
	}

	// 上期所/能源中心需要区分今昨仓
	hasTdYdDistinct := exchangeID == "SHFE" || exchangeID == "INE"
	isToday := pPosition.PositionDate == thost.THOST_FTDC_PSD_Today

	// 根据多空方向和日期更新持仓
	if pPosition.PosiDirection == thost.THOST_FTDC_PD_Long {
		// 多头持仓
		if !hasTdYdDistinct {
			// 非上期所/能源中心：直接使用 YdPosition
			position.VolumeLongYd = int(int32(pPosition.YdPosition))
		} else {
			// 上期所/能源中心：仅在昨仓记录中设置 VolumeLongYd
			if !isToday {
				position.VolumeLongYd = int(int32(pPosition.YdPosition))
			}
		}

		if isToday {
			// 今仓记录
			position.VolumeLongToday = int(int32(pPosition.Position))
			position.VolumeLongFrozenToday = int(int32(pPosition.ShortFrozen))
			position.PositionCostLongToday = float64(pPosition.PositionCost)
			position.OpenCostLongToday = float64(pPosition.OpenCost)
			position.MarginLongToday = float64(pPosition.UseMargin)
		} else {
			// 昨仓记录
			position.VolumeLongHis = int(int32(pPosition.Position))
			position.VolumeLongFrozenHis = int(int32(pPosition.ShortFrozen))
			position.PositionCostLongHis = float64(pPosition.PositionCost)
			position.OpenCostLongHis = float64(pPosition.OpenCost)
			position.MarginLongHis = float64(pPosition.UseMargin)
		}

		// 合计
		position.VolumeLong = position.VolumeLongToday + position.VolumeLongHis
		position.PositionCostLong = position.PositionCostLongToday + position.PositionCostLongHis
		position.OpenCostLong = position.OpenCostLongToday + position.OpenCostLongHis
		position.MarginLong = position.MarginLongToday + position.MarginLongHis

	} else if pPosition.PosiDirection == thost.THOST_FTDC_PD_Short {
		// 空头持仓
		if !hasTdYdDistinct {
			position.VolumeShortYd = int(int32(pPosition.YdPosition))
		} else {
			if !isToday {
				position.VolumeShortYd = int(int32(pPosition.YdPosition))
			}
		}

		if isToday {
			// 今仓记录
			position.VolumeShortToday = int(int32(pPosition.Position))
			position.VolumeShortFrozenToday = int(int32(pPosition.LongFrozen))
			position.PositionCostShortToday = float64(pPosition.PositionCost)
			position.OpenCostShortToday = float64(pPosition.OpenCost)
			position.MarginShortToday = float64(pPosition.UseMargin)
		} else {
			// 昨仓记录
			position.VolumeShortHis = int(int32(pPosition.Position))
			position.VolumeShortFrozenHis = int(int32(pPosition.LongFrozen))
			position.PositionCostShortHis = float64(pPosition.PositionCost)
			position.OpenCostShortHis = float64(pPosition.OpenCost)
			position.MarginShortHis = float64(pPosition.UseMargin)
		}

		// 合计
		position.VolumeShort = position.VolumeShortToday + position.VolumeShortHis
		position.PositionCostShort = position.PositionCostShortToday + position.PositionCostShortHis
		position.OpenCostShort = position.OpenCostShortToday + position.OpenCostShortHis
		position.MarginShort = position.MarginShortToday + position.MarginShortHis
	}

	position.Margin = position.MarginLong + position.MarginShort
	position.Changed = true

	// 绑定合约信息 (如果还没有绑定)
	if position.Ins == nil {
		position.Ins = t.GetInstrument(instrumentID)
	}

	logger.Debug("position updated",
		zap.String("position_key", positionKey),
		zap.Bool("is_today", isToday),
		zap.Int("volume_long", position.VolumeLong),
		zap.Int("volume_long_today", position.VolumeLongToday),
		zap.Int("volume_long_his", position.VolumeLongHis),
		zap.Int("volume_short", position.VolumeShort),
	)
}

// sendAllUserData 发送用户数据给所有客户端 (diff 模式)
// 对应 C++ traderctp::SendUserData
// 只发送 changed = true 的数据，发送后重置 changed = false
func (t *TraderCTP) sendAllUserData() {
	t.userMu.Lock()
	defer t.userMu.Unlock()

	if t.user == nil {
		return
	}

	// 统一重算持仓盈亏和账户盈亏
	// 类似 tradersim 的 recalculatePositionAndFloatProfit
	// 在发送数据前统一计算，而不是在行情回调中频繁计算
	t.recalculatePositionAndAccountProfit()

	// 构建 diff 消息 (只包含变化的数据)
	msg := t.buildUserDataMsg()

	// 如果没有变化，不发送
	if msg == "" {
		logger.Debug("no data changed, not sending")
		return
	}

	logger.Debug("sendAllUserData (diff)", zap.Int("msg_len", len(msg)))
	t.SendMsgAll(msg)
}

// buildUserDataMsg 构建用户数据消息 (diff 模式)
// 对应 C++ SerializerTradeBase 的 dump_all = false 模式
// 只序列化 changed = true 的对象，序列化后重置 changed = false
func (t *TraderCTP) buildUserDataMsg() string {
	return t.buildUserDataMsgInternal(false)
}

// buildUserDataMsgFull 构建完整用户数据消息 (全量模式)
// 对应 C++ SerializerTradeBase 的 dump_all = true 模式
// 序列化所有对象，不检查 changed 标志
func (t *TraderCTP) buildUserDataMsgFull() string {
	return t.buildUserDataMsgInternal(true)
}

// buildUserDataMsgInternal 构建用户数据消息
// dumpAll: true 序列化所有对象，false 只序列化 changed = true 的对象
// 对应 C++ SerializerTradeBase::FilterMapItem 逻辑
func (t *TraderCTP) buildUserDataMsgInternal(dumpAll bool) string {
	if t.user == nil {
		return `{"aid":"rtn_data","data":[]}`
	}

	// 构建 trade 数据结构
	tradeData := map[string]interface{}{
		"user_id":         t.user.UserID,
		"trading_day":     t.user.TradingDay,
		"trade_more_data": t.user.TradeMoreData,
	}

	hasChanges := false

	// 添加账户数据 (只添加 changed = true 的)
	changedAccounts := make(map[string]*protocol.Account)
	for k, acc := range t.user.Accounts {
		if dumpAll || acc.Changed {
			changedAccounts[k] = acc
			acc.Changed = false // 重置 changed 标志
			hasChanges = true
		}
	}
	if len(changedAccounts) >= 0 {
		tradeData["accounts"] = changedAccounts
	}

	// 添加持仓数据 (只添加 changed = true 的)
	changedPositions := make(map[string]*protocol.Position)
	for k, pos := range t.user.Positions {
		if dumpAll || pos.Changed {
			changedPositions[k] = pos
			pos.Changed = false // 重置 changed 标志
			hasChanges = true
		}
	}
	if len(changedPositions) >= 0 {
		tradeData["positions"] = changedPositions
	}

	// 添加订单数据 (只添加 changed = true 的)
	changedOrders := make(map[string]*protocol.Order)
	for k, ord := range t.user.Orders {
		if dumpAll || ord.Changed {
			changedOrders[k] = ord
			ord.Changed = false // 重置 changed 标志
			hasChanges = true
		}
	}
	if len(changedOrders) >= 0 {
		tradeData["orders"] = changedOrders
	}

	// 添加成交数据 (只添加 changed = true 的)
	changedTrades := make(map[string]*protocol.Trade)
	for k, trd := range t.user.Trades {
		if dumpAll || trd.Changed {
			changedTrades[k] = trd
			trd.Changed = false // 重置 changed 标志
			hasChanges = true
		}
	}
	if len(changedTrades) >= 0 {
		tradeData["trades"] = changedTrades
	}

	// 添加银行数据 (只添加 changed = true 的)
	changedBanks := make(map[string]*protocol.Bank)
	for k, bank := range t.user.Banks {
		if dumpAll || bank.Changed {
			changedBanks[k] = bank
			bank.Changed = false // 重置 changed 标志
			hasChanges = true
		}
	}
	if len(changedBanks) >= 0 {
		tradeData["banks"] = changedBanks
	}

	// 添加转账记录 (只添加 changed = true 的)
	changedTransfers := make(map[string]*protocol.TransferLog)
	for k, trans := range t.user.Transfers {
		if dumpAll || trans.Changed {
			changedTransfers[k] = trans
			trans.Changed = false // 重置 changed 标志
			hasChanges = true
		}
	}
	if len(changedTransfers) >= 0 {
		tradeData["transfers"] = changedTransfers
	}

	// 如果没有任何变化，返回空字符串
	if !dumpAll && !hasChanges {
		return ""
	}

	// 构建完整消息结构
	// 格式: {"aid":"rtn_data","data":[{"trade":{"user_id":{...}}}]}
	msg := map[string]interface{}{
		"aid": "rtn_data",
		"data": []interface{}{
			map[string]interface{}{
				"trade": map[string]interface{}{
					t.user.UserID: tradeData,
				},
			},
		},
	}

	// 使用 sonic 序列化
	result, err := protocol.MarshalString(msg)
	if err != nil {
		logger.Error("marshal user data failed", zap.Error(err))
		return `{"aid":"rtn_data","data":[]}`
	}

	return result
}

// ==================== 经纪商交易参数查询 (Phase 5: Query Features) ====================

// ReqQryBrokerTradingParams 查询经纪商交易参数
// 对应 C++ traderctp::ReqQryBrokerTradingParams
func (s *CtpSpi) ReqQryBrokerTradingParams() {
	req := &thost.CThostFtdcQryBrokerTradingParamsField{}
	copy(req.BrokerID[:], s.brokerID)
	copy(req.InvestorID[:], s.userID)

	ret := s.api.ReqQryBrokerTradingParams(req, s.nextRequestID())
	logger.Info("ReqQryBrokerTradingParams", zap.Int("ret", ret))
}

// OnRspQryBrokerTradingParams 经纪商交易参数查询响应
// 对应 C++ traderctp::OnRspQryBrokerTradingParams
func (s *CtpSpi) OnRspQryBrokerTradingParams(pBrokerTradingParams *thost.CThostFtdcBrokerTradingParamsField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if s.isErrorRspInfo(pRspInfo) {
		return
	}

	if pBrokerTradingParams != nil {
		s.trader.updateBrokerTradingParams(pBrokerTradingParams)
	}

	if bIsLast {
		logger.Info("broker trading params query completed")
		s.trader.queryScheduler.SetNeedQueryBrokerParams(false)
	}
}

// updateBrokerTradingParams 更新经纪商交易参数
// 对应 C++ traderctp::ProcessQryBrokerTradingParams
func (t *TraderCTP) updateBrokerTradingParams(params *thost.CThostFtdcBrokerTradingParamsField) {
	// 保存算法类型用于盈亏计算
	t.algorithmType = byte(params.Algorithm)

	logger.Info("broker trading params updated",
		zap.Uint8("algorithm", uint8(params.Algorithm)),
		zap.Uint8("margin_price_type", uint8(params.MarginPriceType)),
	)
}
