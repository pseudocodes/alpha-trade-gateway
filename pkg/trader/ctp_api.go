// Package trader CTP API 操作集合
// 封装 brokerID/userID/requestID 等上下文，提供便捷的业务级调用
// 所有 ReqXxx 方法从 CtpSpi 迁移至此
package trader

import (
	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/protocol"
	"fmt"
	"sync/atomic"

	"github.com/pseudocodes/go2ctp/thost"
	"go.uber.org/zap"
)

// CtpApiOps CTP API 操作集合
// 封装 brokerID/userID/requestID 等上下文，提供便捷的业务级调用
// 不另定义 interface — thost.TraderApi 本身就是 interface
type CtpApiOps struct {
	api      thost.TraderApi // 直接持有 thost.TraderApi interface
	brokerID string
	userID   string
	password string
	appID    string
	authCode string

	requestID   atomic.Int32
	orderRefSeq atomic.Int32
	frontID     int
	sessionID   int

	// trader 引用 (用于访问 insertOrderSet/cancelOrderSet/inputOrderKeyMap 等)
	trader *TraderCore
}

// NewCtpApiOps 创建 CTP API 操作集合
func NewCtpApiOps(trader *TraderCore) *CtpApiOps {
	return &CtpApiOps{
		trader: trader,
	}
}

// SetLoginInfo 设置登录信息
func (ops *CtpApiOps) SetLoginInfo(brokerID, userID, password, appID, authCode string) {
	ops.brokerID = brokerID
	ops.userID = userID
	ops.password = password
	ops.appID = appID
	ops.authCode = authCode
}

// SetApi 设置 TraderApi
func (ops *CtpApiOps) SetApi(api thost.TraderApi) {
	ops.api = api
}

// Api 获取底层 thost.TraderApi (供高级用途)
func (ops *CtpApiOps) Api() thost.TraderApi {
	return ops.api
}

// SetSessionInfo 设置会话信息 (登录成功后调用)
func (ops *CtpApiOps) SetSessionInfo(frontID, sessionID int, maxOrderRef int) {
	ops.frontID = frontID
	ops.sessionID = sessionID
	ops.orderRefSeq.Store(int32(maxOrderRef))
}

// FrontID 获取前置编号
func (ops *CtpApiOps) FrontID() int { return ops.frontID }

// SessionID 获取会话编号
func (ops *CtpApiOps) SessionID() int { return ops.sessionID }

// BrokerID 获取经纪商代码
func (ops *CtpApiOps) BrokerID() string { return ops.brokerID }

// UserID 获取用户代码
func (ops *CtpApiOps) UserID() string { return ops.userID }

// Password 获取密码
func (ops *CtpApiOps) Password() string { return ops.password }

// AppID 获取 AppID
func (ops *CtpApiOps) AppID() string { return ops.appID }

// AuthCode 获取 AuthCode
func (ops *CtpApiOps) AuthCode() string { return ops.authCode }

// nextRequestID 获取下一个请求 ID
func (ops *CtpApiOps) nextRequestID() int {
	return int(ops.requestID.Add(1))
}

// NextOrderRef 获取下一个报单引用
func (ops *CtpApiOps) NextOrderRef() string {
	return fmt.Sprintf("%d", ops.orderRefSeq.Add(1))
}

// MakeOrderID 生成订单 ID
func (ops *CtpApiOps) MakeOrderID(pOrder *thost.CThostFtdcOrderField) string {
	return fmt.Sprintf("%d_%d_%s",
		pOrder.FrontID,
		pOrder.SessionID,
		bytesToString(pOrder.OrderRef[:]))
}

// ==================== 认证/登录 ====================

// Authenticate 发送认证请求
func (ops *CtpApiOps) Authenticate() int {
	req := &thost.CThostFtdcReqAuthenticateField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.UserID[:], ops.userID)
	copy(req.AppID[:], ops.appID)
	copy(req.AuthCode[:], ops.authCode)

	ret := ops.api.ReqAuthenticate(req, ops.nextRequestID())
	logger.Info("ReqAuthenticate",
		zap.Int("ret", ret),
		zap.String("broker_id", ops.brokerID),
		zap.String("user_id", ops.userID),
	)
	return ret
}

// UserLogin 发送登录请求
func (ops *CtpApiOps) UserLogin(productInfo string) int {
	req := &thost.CThostFtdcReqUserLoginField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.UserID[:], ops.userID)
	copy(req.Password[:], ops.password)
	if productInfo != "" {
		copy(req.UserProductInfo[:], productInfo)
	}

	ret := ops.api.ReqUserLogin(req, ops.nextRequestID())
	logger.Info("ReqUserLogin", zap.Int("ret", ret))
	return ret
}

// ==================== 结算单 ====================

// QrySettlementInfo 查询结算单
func (ops *CtpApiOps) QrySettlementInfo() int {
	req := &thost.CThostFtdcQrySettlementInfoField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.InvestorID[:], ops.userID)

	ret := ops.api.ReqQrySettlementInfo(req, ops.nextRequestID())
	logger.Info("ReqQrySettlementInfo", zap.Int("ret", ret))
	return ret
}

// ConfirmSettlementInfo 确认结算单
func (ops *CtpApiOps) ConfirmSettlementInfo() int {
	if ops.api == nil {
		logger.Warn("ConfirmSettlementInfo: api is nil")
		return -1
	}
	req := &thost.CThostFtdcSettlementInfoConfirmField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.InvestorID[:], ops.userID)

	ret := ops.api.ReqSettlementInfoConfirm(req, ops.nextRequestID())
	logger.Info("ReqSettlementInfoConfirm", zap.Int("ret", ret))
	return ret
}

// QrySettlementInfoConfirm 查询结算单确认状态
func (ops *CtpApiOps) QrySettlementInfoConfirm() int {
	req := &thost.CThostFtdcQrySettlementInfoConfirmField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.InvestorID[:], ops.userID)

	ret := ops.api.ReqQrySettlementInfoConfirm(req, ops.nextRequestID())
	logger.Info("ReqQrySettlementInfoConfirm", zap.Int("ret", ret))
	return ret
}

// QrySettlementInfoHistory 查询历史结算单
func (ops *CtpApiOps) QrySettlementInfoHistory(tradingDay int) int {
	req := &thost.CThostFtdcQrySettlementInfoField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.InvestorID[:], ops.userID)
	tradingDayStr := formatTradingDay(tradingDay)
	copy(req.TradingDay[:], tradingDayStr)

	ret := ops.api.ReqQrySettlementInfo(req, ops.nextRequestID())
	logger.Info("ReqQrySettlementInfo (history)",
		zap.Int("ret", ret),
		zap.String("trading_day", tradingDayStr),
	)
	return ret
}

// ==================== 合约查询 ====================

// QryInstrument 查询全量合约信息
func (ops *CtpApiOps) QryInstrument() int {
	if ops.api == nil {
		logger.Error("api is nil when QryInstrument")
		return -1
	}
	req := &thost.CThostFtdcQryInstrumentField{}
	ret := ops.api.ReqQryInstrument(req, ops.nextRequestID())
	logger.Info("ReqQryInstrument", zap.Int("ret", ret))
	return ret
}

// ==================== 订单操作 ====================

// InsertOrder 下单 (构造 CTP 结构体 + 调用 api.ReqOrderInsert)
// 返回 (orderRef, strKey, ret)
func (ops *CtpApiOps) InsertOrder(order *protocol.ActionInsertOrder) (string, string, int) {
	req := &thost.CThostFtdcInputOrderField{}

	copy(req.BrokerID[:], ops.brokerID)
	copy(req.InvestorID[:], ops.userID)
	copy(req.ExchangeID[:], order.ExchangeID)
	copy(req.InstrumentID[:], order.InstrumentID)

	orderRef := ops.NextOrderRef()
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

	// 投机套保标志
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
		req.CombHedgeFlag[0] = byte(thost.THOST_FTDC_HF_Speculation)
	}

	// 价格类型
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

	req.VolumeTotalOriginal = thost.TThostFtdcVolumeType(order.Volume)

	// 有效期类型
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
		req.TimeCondition = thost.THOST_FTDC_TC_GFD
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

	// 触发条件
	switch order.ContingentCondition {
	case protocol.ContingentConditionImmediately:
		req.ContingentCondition = thost.THOST_FTDC_CC_Immediately
	case protocol.ContingentConditionTouch:
		req.ContingentCondition = thost.THOST_FTDC_CC_Touch
	case protocol.ContingentConditionTouchProfit:
		req.ContingentCondition = thost.THOST_FTDC_CC_TouchProfit
	default:
		req.ContingentCondition = thost.THOST_FTDC_CC_Immediately
	}

	req.ForceCloseReason = thost.THOST_FTDC_FCC_NotForceClose
	req.IsAutoSuspend = 0
	req.UserForceClose = 0

	// 跟踪待确认的下单
	ops.trader.insertOrderSetMu.Lock()
	ops.trader.insertOrderSet[orderRef] = true
	ops.trader.insertOrderSetMu.Unlock()

	// 构建订单 key 并添加到 inputOrderKeyMap
	strKey := fmt.Sprintf("%d_%d_%s", ops.frontID, ops.sessionID, orderRef)
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
	ops.trader.inputOrderKeyMapMu.Lock()
	ops.trader.inputOrderKeyMap[strKey] = serverOrderInfo
	ops.trader.inputOrderKeyMapMu.Unlock()

	ret := ops.api.ReqOrderInsert(req, ops.nextRequestID())
	logger.Info("ReqOrderInsert",
		zap.Int("ret", ret),
		zap.String("order_ref", orderRef),
		zap.String("instrument_id", order.InstrumentID),
		zap.String("order_key", strKey),
	)
	return orderRef, strKey, ret
}

// CancelOrder 撤单
func (ops *CtpApiOps) CancelOrder(order *protocol.Order) int {
	req := &thost.CThostFtdcInputOrderActionField{}

	copy(req.BrokerID[:], ops.brokerID)
	copy(req.InvestorID[:], ops.userID)
	copy(req.ExchangeID[:], order.ExchangeID)
	copy(req.InstrumentID[:], order.InstrumentID)

	parts := parseOrderID(order.OrderID)
	if len(parts) >= 3 {
		req.FrontID = thost.TThostFtdcFrontIDType(parts[0])
		req.SessionID = thost.TThostFtdcSessionIDType(parts[1])
		copy(req.OrderRef[:], fmt.Sprintf("%d", parts[2]))
	}

	req.ActionFlag = thost.THOST_FTDC_AF_Delete

	// 跟踪待确认的撤单
	ops.trader.cancelOrderSetMu.Lock()
	ops.trader.cancelOrderSet[order.OrderID] = true
	ops.trader.cancelOrderSetMu.Unlock()

	ret := ops.api.ReqOrderAction(req, ops.nextRequestID())
	logger.Info("ReqOrderAction",
		zap.Int("ret", ret),
		zap.String("order_id", order.OrderID),
	)
	return ret
}

// ==================== 查询操作 ====================

// QryTradingAccount 查询资金账户
func (ops *CtpApiOps) QryTradingAccount() int {
	req := &thost.CThostFtdcQryTradingAccountField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.InvestorID[:], ops.userID)

	ret := ops.api.ReqQryTradingAccount(req, ops.nextRequestID())
	logger.Info("ReqQryTradingAccount", zap.Int("ret", ret))
	return ret
}

// QryInvestorPosition 查询持仓
func (ops *CtpApiOps) QryInvestorPosition() int {
	req := &thost.CThostFtdcQryInvestorPositionField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.InvestorID[:], ops.userID)

	ret := ops.api.ReqQryInvestorPosition(req, ops.nextRequestID())
	logger.Info("ReqQryInvestorPosition", zap.Int("ret", ret))
	return ret
}

// QryOrder 查询订单
func (ops *CtpApiOps) QryOrder() int {
	req := &thost.CThostFtdcQryOrderField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.InvestorID[:], ops.userID)

	ret := ops.api.ReqQryOrder(req, ops.nextRequestID())
	logger.Info("ReqQryOrder", zap.Int("ret", ret))
	return ret
}

// QryTrade 查询成交
func (ops *CtpApiOps) QryTrade() int {
	req := &thost.CThostFtdcQryTradeField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.InvestorID[:], ops.userID)

	ret := ops.api.ReqQryTrade(req, ops.nextRequestID())
	logger.Info("ReqQryTrade", zap.Int("ret", ret))
	return ret
}

// QryBrokerTradingParams 查询经纪商交易参数
func (ops *CtpApiOps) QryBrokerTradingParams() int {
	req := &thost.CThostFtdcQryBrokerTradingParamsField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.InvestorID[:], ops.userID)

	ret := ops.api.ReqQryBrokerTradingParams(req, ops.nextRequestID())
	logger.Info("ReqQryBrokerTradingParams", zap.Int("ret", ret))
	return ret
}

// ==================== 转账操作 ====================

// FromBankToFuture 银行转期货
func (ops *CtpApiOps) FromBankToFuture(bankID, bankPassword, futurePassword, currency string, amount float64) int {
	req := &thost.CThostFtdcReqTransferField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.AccountID[:], ops.userID)
	copy(req.BankID[:], bankID)
	copy(req.BankPassWord[:], bankPassword)
	copy(req.Password[:], futurePassword)
	copy(req.CurrencyID[:], currency)
	req.TradeAmount = thost.TThostFtdcTradeAmountType(amount)
	req.SecuPwdFlag = thost.THOST_FTDC_BPWDF_BlankCheck
	req.BankPwdFlag = thost.THOST_FTDC_BPWDF_NoCheck

	ret := ops.api.ReqFromBankToFutureByFuture(req, ops.nextRequestID())
	logger.Info("ReqFromBankToFutureByFuture",
		zap.Int("ret", ret), zap.Float64("amount", amount))
	return ret
}

// FromFutureToBank 期货转银行
func (ops *CtpApiOps) FromFutureToBank(bankID, bankPassword, futurePassword, currency string, amount float64) int {
	req := &thost.CThostFtdcReqTransferField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.AccountID[:], ops.userID)
	copy(req.BankID[:], bankID)
	copy(req.BankPassWord[:], bankPassword)
	copy(req.Password[:], futurePassword)
	copy(req.CurrencyID[:], currency)
	req.TradeAmount = thost.TThostFtdcTradeAmountType(amount)
	req.SecuPwdFlag = thost.THOST_FTDC_BPWDF_BlankCheck
	req.BankPwdFlag = thost.THOST_FTDC_BPWDF_NoCheck

	ret := ops.api.ReqFromFutureToBankByFuture(req, ops.nextRequestID())
	logger.Info("ReqFromFutureToBankByFuture",
		zap.Int("ret", ret), zap.Float64("amount", amount))
	return ret
}

// QryContractBank 查询签约银行
func (ops *CtpApiOps) QryContractBank() int {
	req := &thost.CThostFtdcQryContractBankField{}
	copy(req.BrokerID[:], ops.brokerID)

	ret := ops.api.ReqQryContractBank(req, ops.nextRequestID())
	logger.Info("ReqQryContractBank", zap.Int("ret", ret))
	return ret
}

// QryAccountRegister 查询银期签约关系
func (ops *CtpApiOps) QryAccountRegister() int {
	req := &thost.CThostFtdcQryAccountregisterField{}
	copy(req.BrokerID[:], ops.brokerID)

	ret := ops.api.ReqQryAccountregister(req, ops.nextRequestID())
	logger.Info("ReqQryAccountRegister", zap.Int("ret", ret))
	return ret
}

// QryTransferSerial 查询转账流水
func (ops *CtpApiOps) QryTransferSerial() int {
	req := &thost.CThostFtdcQryTransferSerialField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.AccountID[:], ops.userID)

	ret := ops.api.ReqQryTransferSerial(req, ops.nextRequestID())
	logger.Info("ReqQryTransferSerial", zap.Int("ret", ret))
	return ret
}

// ==================== 密码修改 ====================

// UserPasswordUpdate 修改用户密码
func (ops *CtpApiOps) UserPasswordUpdate(oldPassword, newPassword string) int {
	req := &thost.CThostFtdcUserPasswordUpdateField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.UserID[:], ops.userID)
	copy(req.OldPassword[:], oldPassword)
	copy(req.NewPassword[:], newPassword)

	ret := ops.api.ReqUserPasswordUpdate(req, ops.nextRequestID())
	logger.Info("ReqUserPasswordUpdate", zap.Int("ret", ret))
	return ret
}

// TradingAccountPasswordUpdate 修改资金账户密码
func (ops *CtpApiOps) TradingAccountPasswordUpdate(oldPassword, newPassword string) int {
	req := &thost.CThostFtdcTradingAccountPasswordUpdateField{}
	copy(req.BrokerID[:], ops.brokerID)
	copy(req.AccountID[:], ops.userID)
	copy(req.OldPassword[:], oldPassword)
	copy(req.NewPassword[:], newPassword)
	copy(req.CurrencyID[:], "CNY")

	ret := ops.api.ReqTradingAccountPasswordUpdate(req, ops.nextRequestID())
	logger.Info("ReqTradingAccountPasswordUpdate", zap.Int("ret", ret))
	return ret
}
