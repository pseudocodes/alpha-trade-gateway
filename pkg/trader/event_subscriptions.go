// Package trader TraderCore 事件订阅和 process 处理器
// TraderCore 自身作为事件消费者，在 New 中订阅需要的事件
// 命名约定: process 前缀 = CTP 柜台回报 (内→外)
package trader

import (
	"time"

	"github.com/pseudocodes/go2ctp/thost"
	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/config"
	"alpha-trade-gateway/pkg/inslist"
	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/protocol"
)

// initEventSubscriptions TraderCore 自身订阅事件
// 在 New() 中调用，此时 EventBus 已就绪
// 当 SPI 回调发布事件后，TraderCore 通过这些订阅处理业务逻辑
func (t *TraderCore) initEventSubscriptions() {
	eb := t.eventBus

	// === 连接/登录/结算 生命周期事件 → process 前缀 ===
	eb.Subscribe(EventFrontConnected, t.processFrontConnected, PriorityHigh)
	eb.Subscribe(EventFrontDisconnected, t.processFrontDisconnected, PriorityHigh)
	eb.Subscribe(EventRspAuthenticate, t.processRspAuthenticate, PriorityHigh)
	eb.Subscribe(EventRspUserLogin, t.processRspUserLogin, PriorityHigh)
	eb.Subscribe(EventSettlementInfoReceived, t.processSettlementInfoReceived, PriorityHigh)
	eb.Subscribe(EventSettlementInfoComplete, t.processSettlementInfoComplete, PriorityHigh)
	eb.Subscribe(EventSettlementConfirmed, t.processSettlementConfirmed, PriorityHigh)
	eb.Subscribe(EventInstrumentQueryComplete, t.processInstrumentQueryComplete, PriorityHigh)

	// === CTP SPI 回报事件 → process 前缀 (柜台回报) ===

	// 订单/成交回报
	eb.Subscribe(EventOrderReturn, t.processOrderReturn, PriorityNormal)
	eb.Subscribe(EventTradeReturn, t.processTradeReturn, PriorityNormal)
	eb.Subscribe(EventOrderInsertError, t.processOrderInsertError, PriorityNormal)
	eb.Subscribe(EventOrderActionError, t.processOrderActionError, PriorityNormal)

	// 查询响应 (初始化查询 + OnIdle 刷新)
	eb.Subscribe(EventAccountUpdated, t.processAccountUpdated, PriorityNormal)
	eb.Subscribe(EventPositionUpdated, t.processPositionUpdated, PriorityNormal)
	eb.Subscribe(EventOrderQueried, t.processOrderQueried, PriorityNormal)
	eb.Subscribe(EventTradeQueried, t.processTradeQueried, PriorityNormal)
	eb.Subscribe(EventBrokerTradingParams, t.processBrokerTradingParams, PriorityNormal)

	// 合约查询
	eb.Subscribe(EventInstrumentUpdated, t.processInstrumentUpdated, PriorityNormal)

	// 银行/转账
	eb.Subscribe(EventBankUpdated, t.processBankUpdated, PriorityNormal)
	eb.Subscribe(EventAccountRegisterUpdated, t.processAccountRegisterUpdated, PriorityNormal)
	eb.Subscribe(EventTransferSerialUpdated, t.processTransferSerialUpdated, PriorityNormal)
	eb.Subscribe(EventTransferResult, t.processTransferResult, PriorityNormal)
	eb.Subscribe(EventTransferError, t.processTransferError, PriorityNormal)

	// 通知/状态
	eb.Subscribe(EventPasswordChanged, t.processPasswordChanged, PriorityNormal)
	eb.Subscribe(EventTradingNotice, t.processTradingNotice, PriorityNormal)
}

// ==================== 连接/登录/结算 生命周期处理器 ====================

// processFrontConnected 处理前置连接事件
// 对应 C++ OnFrontConnected → ProcessOnFrontConnected
// 首次连接: 通知 "已经连接到交易前置" + 发起认证
// 重连: 通知 "已经重新连接到交易前置" + 发起认证
func (t *TraderCore) processFrontConnected(event *Event) {
	t.setState(StateConnected)
	if t.isLoggedIn() {
		// 重连场景 (对应 C++ m_b_login.load() == true 分支)
		t.outputNotifyAll(320, "已经重新连接到交易前置", "INFO")
	} else {
		// 首次连接 (对应 C++ m_b_login.load() == false 分支)
		t.outputNotifyAll(321, "已经连接到交易前置", "INFO")
	}
	t.sendAuthRequest()
}

// processFrontDisconnected 处理前置断开事件
// 对应 C++ OnFrontDisconnected → ProcessOnFrontDisconnected
func (t *TraderCore) processFrontDisconnected(event *Event) {
	if !t.isLoggedIn() {
		t.setState(StateInit)
	}
	t.outputNotifyAll(322, "已经断开与交易前置的连接", "INFO")
}

// processRspAuthenticate 处理认证响应事件 (成功/失败统一处理)
// 对应 C++ OnRspAuthenticate → ProcessOnRspAuthenticate
// 成功: 重置重试计数 → 发起登录请求 (SendLoginRequest)
// 失败: ErrorID==7 时重新初始化 CTP，其他情况通知错误
func (t *TraderCore) processRspAuthenticate(event *Event) {
	data, ok := event.Data.(*RspAuthenticateData)
	if !ok || data == nil {
		return
	}

	if data.ErrorID != 0 {
		// 认证失败
		logger.Error("authenticate failed",
			zap.Int("error_id", data.ErrorID),
			zap.String("error_msg", data.ErrorMsg),
		)
		t.outputNotifyAll(int64(data.ErrorID), "交易服务器认证失败,"+data.ErrorMsg, "WARNING")

		// 对应 C++ ProcessOnRspAuthenticate: ErrorID==7 时重新初始化
		if data.ErrorID == 7 {
			go t.reinitCtp()
		}
		return
	}

	// 认证成功
	t.tryReqAuthenticateTimes = 0
	t.setState(StateAuthenticated)
	t.sendLoginRequest()
}

// processRspUserLogin 处理登录响应事件 (成功/失败统一处理)
// 对应 C++ OnRspUserLogin → ProcessOnRspUserLogin
// 首次登录成功: 保存会话信息 → 通知 "登录成功" → AfterLogin
// 重连登录成功: 检查交易日是否变化 → 清理/恢复数据
// 登录失败: 通知错误，ErrorID==7 时重新初始化
func (t *TraderCore) processRspUserLogin(event *Event) {
	data, ok := event.Data.(*RspUserLoginData)
	if !ok || data == nil {
		return
	}

	if data.ErrorID != 0 {
		// 登录失败
		logger.Error("login failed",
			zap.Int("error_id", data.ErrorID),
			zap.String("error_msg", data.ErrorMsg),
		)

		if t.isLoggedIn() {
			// 重连场景登录失败 (对应 C++ ProcessOnRspUserLogin 失败分支)
			t.outputNotifyAll(int64(data.ErrorID), "交易服务器重登录失败,"+data.ErrorMsg, "WARNING")
			// 对应 C++ ErrorID==7 时 ReinitCtp
			if data.ErrorID == 7 {
				go t.reinitCtp()
			}
		} else {
			// 首次登录失败 (对应 C++ OnRspUserLogin 中 m_b_login==false 分支)
			t.outputNotifyAll(int64(data.ErrorID), "交易服务器登录失败,"+data.ErrorMsg, "WARNING")
		}
		return
	}

	// 登录成功
	t.tryReqLoginTimes = 0
	SetNotifySessionID(data.SessionID)

	if t.isLoggedIn() {
		// 重连场景登录成功 (对应 C++ ProcessOnRspUserLogin 成功分支)
		oldTradingDay := t.getUserTradingDay()
		if oldTradingDay != "" && oldTradingDay != data.TradingDay {
			// 新交易日的重连 → 清理旧数据并重新初始化
			// 对应 C++ m_trading_day != trading_day 分支
			logger.Info("trading day changed on reconnect, clearing old data",
				zap.String("old_day", oldTradingDay),
				zap.String("new_day", data.TradingDay),
			)
			t.clearOldDataForNewTradingDay(data.TradingDay)
			t.afterLogin()
		} else {
			// 同一交易日的正常重连
			// 对应 C++ else 分支: 更新 front_id/session_id, 通知 "重登录成功"
			t.outputNotifyAll(323, "交易服务器重登录成功", "INFO")
			// 重连后刷新持仓和账户
			if t.queryScheduler != nil {
				t.queryScheduler.RequestPositionQuery()
				t.queryScheduler.RequestAccountQuery()
			}
		}
	} else {
		// 首次登录成功 (对应 C++ OnRspUserLogin 中 m_b_login==false 成功分支)
		t.setState(StateLoggedIn)

		oldTradingDay := t.getUserTradingDay()
		if oldTradingDay != "" && oldTradingDay != data.TradingDay {
			t.clearOldDataForNewTradingDay(data.TradingDay)
		} else {
			t.initUserData(t.reqLogin.UserName, data.TradingDay)
		}

		t.outputNotifyAll(324, "登录成功", "INFO")
		t.afterLogin()
	}
}

// afterLogin 登录成功后的处理
// 对应 C++ traderctp::AfterLogin
// 1. 处理结算单 (自动确认 or 查询确认状态)
// 2. 请求查询持仓/账户/银行/经纪商参数
func (t *TraderCore) afterLogin() {
	t.setState(StateSettlementQuerying)

	if config.Global != nil && config.Global.AutoConfirmSettlement {
		// 自动确认结算单 (对应 C++ g_config.auto_confirm_settlement)
		t.ctpApiOps.ConfirmSettlementInfo()
	} else if t.settlementInfo == "" {
		// 查询结算单确认状态 (对应 C++ ReqQrySettlementInfoConfirm)
		t.ctpApiOps.QrySettlementInfoConfirm()
	}

	// 请求查询持仓和账户 (对应 C++ m_req_position_id++; m_req_account_id++)
	if t.queryScheduler != nil {
		t.queryScheduler.RequestPositionQuery()
		t.queryScheduler.RequestAccountQuery()
		t.queryScheduler.SetNeedQueryBank(true)
		t.queryScheduler.SetNeedQueryRegister(true)
		t.queryScheduler.SetNeedQueryBrokerParams(true)
	}
}

// processSettlementInfoReceived 处理结算单内容片段事件
func (t *TraderCore) processSettlementInfoReceived(event *Event) {
	data, ok := event.Data.(*SettlementInfoData)
	if !ok || data == nil {
		return
	}
	t.appendSettlementInfo(data.Content)
}

// processSettlementInfoComplete 处理结算单查询完成事件
// 对应 C++ ProcessQrySettlementInfo (bIsLast=true)
// 查询完成后: 如果未确认，发送结算单给客户端等待确认
func (t *TraderCore) processSettlementInfoComplete(event *Event) {
	logger.Info("settlement info query complete",
		zap.Int("length", len(t.settlementInfo)),
	)

	// 对应 C++ ProcessQrySettlementInfo: 如果 m_confirm_settlement_status==0，发送给客户端
	// 如果是自动确认模式，AfterLogin 已经发了 ReqConfirmSettlement，这里不需要再发
	if !t.settlementConfirmed {
		t.sendSettlementInfo()
	}
}

// processSettlementConfirmed 处理结算单确认事件
// 对应 C++ ProcessQrySettlementInfoConfirm 和 ProcessSettlementInfoConfirm
// 来自 OnRspQrySettlementInfoConfirm: 检查 ConfirmDate 判断是否已确认
// 来自 OnRspSettlementInfoConfirm: 确认操作完成
func (t *TraderCore) processSettlementConfirmed(event *Event) {
	data, ok := event.Data.(*SettlementConfirmData)
	if ok && data != nil && data.ConfirmDate != "" {
		// 来自 OnRspQrySettlementInfoConfirm
		// 对应 C++ ProcessQrySettlementInfoConfirm
		tradingDay := t.getUserTradingDay()
		if data.ConfirmDate >= tradingDay {
			// 已经确认过结算单，直接进入下一步
			logger.Info("settlement already confirmed",
				zap.String("confirm_date", data.ConfirmDate),
				zap.String("trading_day", tradingDay),
			)
			t.onSettlementConfirmed()
		} else {
			// 未确认，需要查询结算单内容
			logger.Info("settlement not confirmed, querying settlement info")
			if t.ctpApiOps != nil {
				t.ctpApiOps.QrySettlementInfo()
			}
		}
	} else {
		// 来自 OnRspSettlementInfoConfirm (确认操作完成)
		t.onSettlementConfirmed()
	}
}

// processInstrumentQueryComplete 处理合约查询完成事件
func (t *TraderCore) processInstrumentQueryComplete(event *Event) {
	t.onInstrumentQueryComplete()
}

// ==================== 订单/成交回报处理器 ====================

// processOrderReturn 处理订单回报事件
func (t *TraderCore) processOrderReturn(event *Event) {
	data, ok := event.Data.(*OrderReturnData)
	if !ok || data == nil || data.Raw == nil {
		return
	}
	t.updateOrder(data.Raw)
}

// processTradeReturn 处理成交回报事件
func (t *TraderCore) processTradeReturn(event *Event) {
	data, ok := event.Data.(*TradeReturnData)
	if !ok || data == nil || data.Raw == nil {
		return
	}
	t.updateTrade(data.Raw)
}

// processOrderInsertError 处理下单错误回报事件
func (t *TraderCore) processOrderInsertError(event *Event) {
	data, ok := event.Data.(*OrderInsertErrorData)
	if !ok || data == nil {
		return
	}
	if data.ErrorID != 0 {
		t.outputNotifyAll(int64(data.ErrorID), data.ErrorMsg, "ERROR")
	}
}

// processOrderActionError 处理撤单错误回报事件
func (t *TraderCore) processOrderActionError(event *Event) {
	data, ok := event.Data.(*OrderActionErrorData)
	if !ok || data == nil {
		return
	}
	if data.ErrorID != 0 {
		t.outputNotifyAll(int64(data.ErrorID), data.ErrorMsg, "ERROR")
	}
}

// ==================== 查询响应处理器 (含初始化查询链) ====================

// processAccountUpdated 处理账户查询事件
// 初始化查询链: Account(IsLast) → Position
func (t *TraderCore) processAccountUpdated(event *Event) {
	data, ok := event.Data.(*AccountUpdateData)
	if !ok || data == nil || data.Raw == nil {
		return
	}
	t.updateAccount(data.Raw)

	// 初始化查询链: Account 查完 → 查 Position
	if data.IsLast && !t.isPositionInited() {
		logger.Info("init query chain: account done, querying position")
		go func() {
			time.Sleep(time.Second)
			t.ctpApiOps.QryInvestorPosition()
		}()
	}
}

// processPositionUpdated 处理持仓查询事件
// 初始化查询链: Position(IsLast) → Order
func (t *TraderCore) processPositionUpdated(event *Event) {
	data, ok := event.Data.(*PositionUpdateData)
	if !ok || data == nil || data.Raw == nil {
		return
	}
	t.updatePosition(data.Raw)

	// 初始化查询链: Position 查完 → 查 Order
	if data.IsLast && !t.isPositionInited() {
		logger.Info("init query chain: position done, querying order")
		go func() {
			time.Sleep(time.Second)
			t.ctpApiOps.QryOrder()
		}()
	}
}

// processOrderQueried 处理订单查询事件 (初始化查询)
// 初始化查询链: Order(IsLast) → Trade
func (t *TraderCore) processOrderQueried(event *Event) {
	data, ok := event.Data.(*OrderQueryData)
	if !ok || data == nil || data.Raw == nil {
		return
	}
	t.updateOrder(data.Raw)

	// 初始化查询链: Order 查完 → 查 Trade
	if data.IsLast && !t.isPositionInited() {
		logger.Info("init query chain: order done, querying trade")
		go func() {
			t.ctpApiOps.QryTrade()
		}()
	}
}

// processTradeQueried 处理成交查询事件 (初始化查询)
// 初始化查询链终点: Trade(IsLast) → 初始化持仓 → 设置 Ready
func (t *TraderCore) processTradeQueried(event *Event) {
	data, ok := event.Data.(*TradeQueryData)
	if !ok || data == nil || data.Raw == nil {
		return
	}
	t.updateTrade(data.Raw)

	// 初始化查询链终点
	if data.IsLast && !t.isPositionInited() {
		logger.Info("init query chain complete: initializing position volume")

		t.initPositionVolume()
		t.replayTradesBySeqno()
		t.setPositionInited(true)
		logger.Info("position volume initialized")

		t.setQuerySchedulerNeedQueryBrokerParams(true)
		t.setQuerySchedulerNeedQueryBank(true)
		t.setQuerySchedulerNeedQueryRegister(true)

		t.setState(StateReady)
		t.subscribePositionSymbols()
		t.sendAllUserData()

		// 通知所有 Plugin 系统就绪
		if t.pluginManager != nil {
			t.pluginManager.NotifyReady()
		}
	}
}

// processBrokerTradingParams 处理经纪商交易参数事件
func (t *TraderCore) processBrokerTradingParams(event *Event) {
	data, ok := event.Data.(*BrokerTradingParamsData)
	if !ok || data == nil || data.Raw == nil {
		return
	}
	t.updateBrokerTradingParams(data.Raw)
	if data.IsLast {
		t.setQuerySchedulerNeedQueryBrokerParams(false)
	}
}

// ==================== 合约查询处理器 ====================

// processInstrumentUpdated 处理合约查询事件
// 对应原 ctp_wrapper.go 中的 processInstrument 逻辑
func (t *TraderCore) processInstrumentUpdated(event *Event) {
	data, ok := event.Data.(*thost.CThostFtdcInstrumentField)
	if !ok || data == nil {
		return
	}

	if t.insService == nil {
		return
	}

	instrumentID := bytesToString(data.InstrumentID[:])
	exchangeID := bytesToString(data.ExchangeID[:])
	symbol := exchangeID + "." + instrumentID

	ins := inslist.NewInstrumentInfo()
	ins.Symbol = symbol
	ins.ExchangeID = exchangeID
	ins.InstrumentID = instrumentID
	ins.VolumeMultiple = int64(data.VolumeMultiple)
	ins.PriceTick = float64(data.PriceTick)

	// 产品类型
	switch data.ProductClass {
	case thost.THOST_FTDC_PC_Options, thost.THOST_FTDC_PC_SpotOption:
		ins.ProductClass = protocol.ProductClassOptions
	default:
		ins.ProductClass = protocol.ProductClassFutures
	}

	// 是否过期
	ins.Expired = data.IsTrading == 0

	t.insService.SetInstrument(symbol, ins)
}

// ==================== 银行/转账处理器 ====================

// processBankUpdated 处理银行信息更新事件
func (t *TraderCore) processBankUpdated(event *Event) {
	data, ok := event.Data.(*BankUpdateData)
	if !ok || data == nil || data.Raw == nil {
		return
	}
	t.updateBank(data.Raw)
	if data.IsLast {
		t.setQuerySchedulerNeedQueryBank(false)
	}
}

// processAccountRegisterUpdated 处理签约关系更新事件
func (t *TraderCore) processAccountRegisterUpdated(event *Event) {
	data, ok := event.Data.(*AccountRegisterData)
	if !ok || data == nil || data.Raw == nil {
		return
	}
	t.updateAccountRegister(data.Raw)
	if data.IsLast {
		t.setQuerySchedulerNeedQueryRegister(false)
	}
}

// processTransferSerialUpdated 处理转账流水更新事件
func (t *TraderCore) processTransferSerialUpdated(event *Event) {
	data, ok := event.Data.(*TransferSerialData)
	if !ok || data == nil || data.Raw == nil {
		return
	}
	t.updateTransferSerial(data.Raw)
}

// processTransferResult 处理转账结果事件
func (t *TraderCore) processTransferResult(event *Event) {
	data, ok := event.Data.(*TransferResultData)
	if !ok || data == nil {
		return
	}

	if data.ErrorID != 0 {
		t.outputNotifyAll(int64(data.ErrorID), "银期错误,"+data.ErrorMsg, "WARNING")
	} else {
		t.outputNotifyAll(327, "转账成功", "INFO")
		go t.requestRefreshAfterTransfer()
		if data.Raw != nil {
			t.updateTransfer(data.Raw)
		}
	}
}

// processTransferError 处理转账错误事件
func (t *TraderCore) processTransferError(event *Event) {
	data, ok := event.Data.(*TransferErrorData)
	if !ok || data == nil {
		return
	}

	direction := "银行资金转期货"
	if !data.IsBankToFuture {
		direction = "期货资金转银行"
	}
	t.outputNotifyAll(int64(data.ErrorID), direction+"错误,"+data.ErrorMsg, "WARNING")
}

// ==================== 通知/状态处理器 ====================

// processPasswordChanged 处理密码修改结果事件
func (t *TraderCore) processPasswordChanged(event *Event) {
	data, ok := event.Data.(*PasswordChangeData)
	if !ok || data == nil {
		return
	}

	if data.IsTrading {
		if data.IsSuccess {
			logger.Info("trading account password changed successfully")
			t.outputNotifyAll(363, "修改资金密码成功", "INFO")
		} else {
			t.outputNotifyAll(int64(data.ErrorID), "修改资金密码失败,"+data.ErrorMsg, "WARNING")
		}
	} else {
		if data.IsSuccess {
			logger.Info("password changed successfully")
			t.outputNotifyAll(326, "修改密码成功", "INFO")
		} else {
			t.outputNotifyAll(int64(data.ErrorID), "修改密码失败,"+data.ErrorMsg, "WARNING")
		}
	}
}

// processTradingNotice 处理交易通知事件
func (t *TraderCore) processTradingNotice(event *Event) {
	data, ok := event.Data.(*TradingNoticeData)
	if !ok || data == nil {
		return
	}
	if data.Content != "" {
		t.outputNotifyAll(332, data.Content, "INFO")
	}
}
