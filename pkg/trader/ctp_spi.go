// Package trader CTP SPI 回调集中实现
// 所有 OnXxx 回调集中到此文件，回调内部只做两件事：
// 1. 解析 CTP 结构体为 Go 事件数据
// 2. 发布事件到 EventBus
//
// 设计约束：CtpSpi 不持有 TraderCore 引用，不处理任何业务逻辑，
// 所有业务逻辑由 TraderCore 的 process 前缀方法通过事件订阅处理
package trader

import (
	"alpha-trade-gateway/pkg/config"
	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/protocol"
	"bytes"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/pseudocodes/go2ctp/ctp"
	"github.com/pseudocodes/go2ctp/thost"
	"go.uber.org/zap"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// CtpSpi CTP 回调处理
// 对应 C++ CThostFtdcTraderSpi 的实现
// 所有 SPI 回调集中在此文件，只做解析+发布事件
type CtpSpi struct {
	ctp.BaseTraderSpi

	eventBus *EventBus  // 事件总线 (发布事件)
	apiOps   *CtpApiOps // CTP API 操作集合 (共享会话信息)

	// CTP 登录信息
	brokerID    string
	userID      string
	password    string
	appID       string
	authCode    string
	productInfo string

	// 请求序号
	requestID atomic.Int32

	// 前置编号和会话编号 (登录成功后由 OnRspUserLogin 设置)
	frontID   int
	sessionID int

	// 报单引用序列号
	orderRefSeq atomic.Int32
}

// NewCtpSpi 创建 CTP SPI
// eventBus: 事件总线，SPI 回调通过它发布事件
func NewCtpSpi(eventBus *EventBus) *CtpSpi {
	return &CtpSpi{
		eventBus: eventBus,
	}
}

// SetLoginInfo 设置登录信息
func (s *CtpSpi) SetLoginInfo(broker *config.BrokerConfig, userID, password string) {
	s.brokerID = broker.CtpBrokerID
	s.userID = userID
	s.password = password
	s.appID = broker.AppID
	s.authCode = broker.AuthCode
	s.productInfo = broker.ProductInfo
}

// SetApiOps 设置 CTP API 操作集合
func (s *CtpSpi) SetApiOps(apiOps *CtpApiOps) {
	s.apiOps = apiOps
}

// nextRequestID 获取下一个请求 ID
func (s *CtpSpi) nextRequestID() int {
	return int(s.requestID.Add(1))
}

// nextOrderRef 获取下一个报单引用
func (s *CtpSpi) nextOrderRef() string {
	return fmt.Sprintf("%d", s.orderRefSeq.Add(1))
}

// makeOrderID 生成订单 ID
func (s *CtpSpi) makeOrderID(pOrder *thost.CThostFtdcOrderField) string {
	return fmt.Sprintf("%d_%d_%s",
		pOrder.FrontID,
		pOrder.SessionID,
		bytesToString(pOrder.OrderRef[:]))
}

// ==================== 连接/登录/结算 回调 ====================

// OnFrontConnected 前置连接成功回调 → 发布 EventFrontConnected
func (s *CtpSpi) OnFrontConnected() {
	logger.Info("OnFrontConnected")
	s.eventBus.Publish(NewEvent(EventFrontConnected, &FrontConnectedData{}))
}

// OnFrontDisconnected 前置断开回调 → 发布 EventFrontDisconnected
func (s *CtpSpi) OnFrontDisconnected(nReason int) {
	logger.Warn("OnFrontDisconnected", zap.Int("reason", nReason))
	s.eventBus.Publish(NewEvent(EventFrontDisconnected, &FrontDisconnectedData{
		Reason: nReason,
	}))
}

// OnHeartBeatWarning 心跳超时警告 (仅日志，无需事件)
func (s *CtpSpi) OnHeartBeatWarning(nTimeLapse int) {
	logger.Warn("CTP heartbeat warning", zap.Int("time_lapse", nTimeLapse))
}

// OnRspAuthenticate 认证响应 → 发布 EventRspAuthenticate (成功/失败统一事件)
func (s *CtpSpi) OnRspAuthenticate(pRspAuthenticateField *thost.CThostFtdcRspAuthenticateField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	errorID := 0
	errorMsg := ""
	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		errorID = int(pRspInfo.ErrorID)
		errorMsg = gbkToUtf8(pRspInfo.ErrorMsg[:])
		logger.Error("OnRspAuthenticate",
			zap.Int("errid", errorID),
			zap.String("errmsg", errorMsg),
		)
	} else {
		logger.Info("OnRspAuthenticate", zap.Bool("is_last", bIsLast))
	}

	s.eventBus.Publish(NewEvent(EventRspAuthenticate, &RspAuthenticateData{
		ErrorID:  errorID,
		ErrorMsg: errorMsg,
	}))
}

// OnRspUserLogin 登录响应 → 发布 EventRspUserLogin (成功/失败统一事件)
func (s *CtpSpi) OnRspUserLogin(pRspUserLogin *thost.CThostFtdcRspUserLoginField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	errorID := 0
	errorMsg := ""
	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		errorID = int(pRspInfo.ErrorID)
		errorMsg = gbkToUtf8(pRspInfo.ErrorMsg[:])
		logger.Error("OnRspUserLogin",
			zap.Int("errid", errorID),
			zap.String("errmsg", errorMsg),
		)
	}

	data := &RspUserLoginData{
		ErrorID:  errorID,
		ErrorMsg: errorMsg,
	}

	// 仅在成功时解析登录信息
	if errorID == 0 && pRspUserLogin != nil {
		s.frontID = int(pRspUserLogin.FrontID)
		s.sessionID = int(pRspUserLogin.SessionID)
		maxOrderRef := parseOrderRef(pRspUserLogin.MaxOrderRef[:])
		s.orderRefSeq.Store(int32(maxOrderRef))

		// 同步会话信息到 apiOps
		if s.apiOps != nil {
			s.apiOps.SetSessionInfo(s.frontID, s.sessionID, maxOrderRef)
		}

		data.FrontID = s.frontID
		data.SessionID = s.sessionID
		data.MaxOrderRef = maxOrderRef
		data.TradingDay = bytesToString(pRspUserLogin.TradingDay[:])

		logger.Info("OnRspUserLogin",
			zap.Int("front_id", s.frontID),
			zap.Int("session_id", s.sessionID),
			zap.String("trading_day", data.TradingDay),
		)
	}

	s.eventBus.Publish(NewEvent(EventRspUserLogin, data))
}

// OnRspQrySettlementInfo 查询结算单响应 → 发布 EventSettlementInfoReceived / EventSettlementInfoComplete
func (s *CtpSpi) OnRspQrySettlementInfo(pSettlementInfo *thost.CThostFtdcSettlementInfoField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		logger.Error("OnRspQrySettlementInfo",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])))
		return
	}
	if pSettlementInfo != nil {
		content := gbkToUtf8(pSettlementInfo.Content[:])
		s.eventBus.Publish(NewEvent(EventSettlementInfoReceived, &SettlementInfoData{
			Content: content,
			IsLast:  bIsLast,
		}))
	}
	if bIsLast {
		logger.Info("settlement info query completed")
		s.eventBus.Publish(NewEvent(EventSettlementInfoComplete, nil))
	}
}

// OnRspSettlementInfoConfirm 确认结算单响应 → 发布 EventSettlementConfirmed
func (s *CtpSpi) OnRspSettlementInfoConfirm(pSettlementInfoConfirm *thost.CThostFtdcSettlementInfoConfirmField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		logger.Error("OnRspSettlementInfoConfirm",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])))
		return
	}
	if bIsLast {
		logger.Info("settlement info confirmed")
		s.eventBus.Publish(NewEvent(EventSettlementConfirmed, &SettlementConfirmData{}))
	}
}

// OnRspQrySettlementInfoConfirm 查询结算单确认响应 → 发布 EventSettlementConfirmed
func (s *CtpSpi) OnRspQrySettlementInfoConfirm(pSettlementInfoConfirm *thost.CThostFtdcSettlementInfoConfirmField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		logger.Error("OnRspQrySettlementInfoConfirm",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])))
		return
	}
	if pSettlementInfoConfirm != nil {
		confirmDate := bytesToString(pSettlementInfoConfirm.ConfirmDate[:])
		confirmTime := bytesToString(pSettlementInfoConfirm.ConfirmTime[:])
		logger.Info("settlement already confirmed",
			zap.String("confirm_date", confirmDate),
			zap.String("confirm_time", confirmTime),
		)
		s.eventBus.Publish(NewEvent(EventSettlementConfirmed, &SettlementConfirmData{
			ConfirmDate: confirmDate,
			ConfirmTime: confirmTime,
		}))
	}
}

// OnRspError 错误响应 (仅日志，无需事件)
func (s *CtpSpi) OnRspError(pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {
	if pRspInfo != nil {
		logger.Error("CTP error",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])),
		)
	}
}

// ==================== 合约查询 ====================

// OnRspQryInstrument 合约查询响应 → 发布 EventInstrumentUpdated / EventInstrumentQueryComplete
func (s *CtpSpi) OnRspQryInstrument(pInstrument *thost.CThostFtdcInstrumentField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		logger.Error("OnRspQryInstrument error",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])),
		)
		return
	}
	if pInstrument != nil {
		cp := *pInstrument
		s.eventBus.Publish(NewEvent(EventInstrumentUpdated, &cp))
	}
	if bIsLast {
		logger.Info("instrument query completed")
		s.eventBus.Publish(NewEvent(EventInstrumentQueryComplete, &InstrumentQueryCompleteData{}))
	}
}

// ==================== 订单/成交回报 ====================

// OnRtnOrder 订单回报 → 发布 EventOrderReturn
func (s *CtpSpi) OnRtnOrder(pOrder *thost.CThostFtdcOrderField) {
	if pOrder == nil {
		return
	}

	orderID := s.makeOrderID(pOrder)
	instrumentID := bytesToString(pOrder.InstrumentID[:])

	logger.Info("order return",
		zap.String("order_id", orderID),
		zap.String("instrument_id", instrumentID),
		zap.Int32("volume_total", int32(pOrder.VolumeTotalOriginal)),
		zap.Uint8("status", uint8(pOrder.OrderStatus)),
	)

	s.eventBus.Publish(NewEvent(EventOrderReturn, &OrderReturnData{
		OrderID:        orderID,
		ExchangeID:     bytesToString(pOrder.ExchangeID[:]),
		InstrumentID:   instrumentID,
		Direction:      protocol.CTPDirectionToProtocol(byte(pOrder.Direction)),
		Offset:         protocol.CTPOffsetToProtocol(pOrder.CombOffsetFlag[0]),
		VolumeOriginal: int(int32(pOrder.VolumeTotalOriginal)),
		VolumeLeft:     int(int32(pOrder.VolumeTotal)),
		LimitPrice:     float64(pOrder.LimitPrice),
		Status:         protocol.CTPOrderStatusToProtocol(byte(pOrder.OrderStatus)),
		StatusMsg:      gbkToUtf8(pOrder.StatusMsg[:]),
		InsertDateTime: protocol.ParseCTPDateTime(bytesToString(pOrder.InsertDate[:]), bytesToString(pOrder.InsertTime[:])),
		Raw:            copyOrder(pOrder),
	}))
}

// OnRtnTrade 成交回报 → 发布 EventTradeReturn
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

	s.eventBus.Publish(NewEvent(EventTradeReturn, &TradeReturnData{
		TradeID:       tradeID,
		OrderID:       fmt.Sprintf("%d_%d_%s", s.frontID, s.sessionID, bytesToString(pTrade.OrderRef[:])),
		ExchangeID:    bytesToString(pTrade.ExchangeID[:]),
		InstrumentID:  instrumentID,
		Direction:     protocol.CTPDirectionToProtocol(byte(pTrade.Direction)),
		Offset:        protocol.CTPOffsetToProtocol(byte(pTrade.OffsetFlag)),
		Volume:        int(int32(pTrade.Volume)),
		Price:         float64(pTrade.Price),
		TradeDateTime: protocol.ParseCTPDateTime(bytesToString(pTrade.TradeDate[:]), bytesToString(pTrade.TradeTime[:])),
		Raw:           copyTrade(pTrade),
	}))
}

// OnRspOrderInsert 下单响应（仅在错误时调用）→ 发布 EventOrderInsertError
func (s *CtpSpi) OnRspOrderInsert(pInputOrder *thost.CThostFtdcInputOrderField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	errorID := 0
	errorMsg := ""
	if pRspInfo != nil {
		errorID = int(pRspInfo.ErrorID)
		errorMsg = gbkToUtf8(pRspInfo.ErrorMsg[:])
	}

	if pInputOrder != nil {
		logger.Error("OnRspOrderInsert",
			zap.String("instrument_id", bytesToString(pInputOrder.InstrumentID[:])),
			zap.String("order_ref", bytesToString(pInputOrder.OrderRef[:])),
			zap.Int("errid", errorID),
			zap.String("errmsg", errorMsg),
		)
	}

	if errorID != 0 {
		s.eventBus.Publish(NewEvent(EventOrderInsertError, &OrderInsertErrorData{
			Raw:      copyInputOrder(pInputOrder),
			ErrorID:  errorID,
			ErrorMsg: errorMsg,
		}))
	}
}

// OnErrRtnOrderInsert 下单错误回报 → 发布 EventOrderInsertError
func (s *CtpSpi) OnErrRtnOrderInsert(pInputOrder *thost.CThostFtdcInputOrderField,
	pRspInfo *thost.CThostFtdcRspInfoField) {

	errorID := -999
	errorMsg := ""
	if pRspInfo != nil {
		errorID = int(pRspInfo.ErrorID)
		errorMsg = gbkToUtf8(pRspInfo.ErrorMsg[:])
	}

	if pInputOrder != nil {
		logger.Error("OnErrRtnOrderInsert",
			zap.String("instrument_id", bytesToString(pInputOrder.InstrumentID[:])),
			zap.String("order_ref", bytesToString(pInputOrder.OrderRef[:])),
			zap.Int("errid", errorID),
			zap.String("errmsg", errorMsg),
		)
	}

	s.eventBus.Publish(NewEvent(EventOrderInsertError, &OrderInsertErrorData{
		Raw:      copyInputOrder(pInputOrder),
		ErrorID:  errorID,
		ErrorMsg: errorMsg,
	}))
}

// OnRspOrderAction 撤单响应（仅在错误时调用）→ 发布 EventOrderActionError
func (s *CtpSpi) OnRspOrderAction(pInputOrderAction *thost.CThostFtdcInputOrderActionField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	errorID := 0
	errorMsg := ""
	if pRspInfo != nil {
		errorID = int(pRspInfo.ErrorID)
		errorMsg = gbkToUtf8(pRspInfo.ErrorMsg[:])
	}

	if errorID != 0 {
		logger.Error("OnRspOrderAction",
			zap.Int("errid", errorID),
			zap.String("errmsg", errorMsg),
		)
		s.eventBus.Publish(NewEvent(EventOrderActionError, &OrderActionErrorData{
			ErrorID:  errorID,
			ErrorMsg: errorMsg,
		}))
	}
}

// OnErrRtnOrderAction 撤单错误回报 → 发布 EventOrderActionError
func (s *CtpSpi) OnErrRtnOrderAction(pOrderAction *thost.CThostFtdcOrderActionField,
	pRspInfo *thost.CThostFtdcRspInfoField) {

	errorID := 0
	errorMsg := ""
	if pRspInfo != nil {
		errorID = int(pRspInfo.ErrorID)
		errorMsg = gbkToUtf8(pRspInfo.ErrorMsg[:])
	}

	if errorID != 0 {
		logger.Error("OnErrRtnOrderAction",
			zap.Int("errid", errorID),
			zap.String("errmsg", errorMsg),
		)
	}

	s.eventBus.Publish(NewEvent(EventOrderActionError, &OrderActionErrorData{
		Raw:      copyOrderAction(pOrderAction),
		ErrorID:  errorID,
		ErrorMsg: errorMsg,
	}))
}

// ==================== 查询响应回调 ====================

// OnRspQryTradingAccount 查询资金账户响应 → 发布 EventAccountUpdated
func (s *CtpSpi) OnRspQryTradingAccount(pTradingAccount *thost.CThostFtdcTradingAccountField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		logger.Error("OnRspQryTradingAccount",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])))
		return
	}
	if pTradingAccount != nil {
		cp := *pTradingAccount
		s.eventBus.Publish(NewEvent(EventAccountUpdated, &AccountUpdateData{
			Raw:    &cp,
			IsLast: bIsLast,
		}))
	}
}

// OnRspQryInvestorPosition 查询持仓响应 → 发布 EventPositionUpdated
func (s *CtpSpi) OnRspQryInvestorPosition(pInvestorPosition *thost.CThostFtdcInvestorPositionField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		logger.Error("OnRspQryInvestorPosition",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])))
		return
	}
	if pInvestorPosition != nil {
		cp := *pInvestorPosition
		s.eventBus.Publish(NewEvent(EventPositionUpdated, &PositionUpdateData{
			Raw:    &cp,
			IsLast: bIsLast,
		}))
	}
}

// OnRspQryOrder 查询订单响应 → 发布 EventOrderQueried
func (s *CtpSpi) OnRspQryOrder(pOrder *thost.CThostFtdcOrderField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		logger.Error("OnRspQryOrder",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])))
		return
	}
	if pOrder != nil {
		s.eventBus.Publish(NewEvent(EventOrderQueried, &OrderQueryData{
			Raw:    copyOrder(pOrder),
			IsLast: bIsLast,
		}))
	}
}

// OnRspQryTrade 查询成交响应 → 发布 EventTradeQueried
func (s *CtpSpi) OnRspQryTrade(pTrade *thost.CThostFtdcTradeField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		logger.Error("OnRspQryTrade",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])))
		return
	}
	if pTrade != nil {
		s.eventBus.Publish(NewEvent(EventTradeQueried, &TradeQueryData{
			Raw:    copyTrade(pTrade),
			IsLast: bIsLast,
		}))
	}
}

// OnRspQryBrokerTradingParams 经纪商交易参数查询响应 → 发布 EventBrokerTradingParams
func (s *CtpSpi) OnRspQryBrokerTradingParams(pBrokerTradingParams *thost.CThostFtdcBrokerTradingParamsField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		logger.Error("OnRspQryBrokerTradingParams",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])))
		return
	}
	if pBrokerTradingParams != nil {
		cp := *pBrokerTradingParams
		s.eventBus.Publish(NewEvent(EventBrokerTradingParams, &BrokerTradingParamsData{
			Raw:    &cp,
			IsLast: bIsLast,
		}))
	}
}

// ==================== 转账回调 ====================

// OnRtnFromBankToFutureByFuture 银转期通知 → 发布 EventTransferResult
func (s *CtpSpi) OnRtnFromBankToFutureByFuture(pRspTransfer *thost.CThostFtdcRspTransferField) {
	s.publishTransferResult(pRspTransfer, true)
}

// OnRtnFromFutureToBankByFuture 期转银通知 → 发布 EventTransferResult
func (s *CtpSpi) OnRtnFromFutureToBankByFuture(pRspTransfer *thost.CThostFtdcRspTransferField) {
	s.publishTransferResult(pRspTransfer, false)
}

// publishTransferResult 解析转账结果 → 发布 EventTransferResult
func (s *CtpSpi) publishTransferResult(pRspTransfer *thost.CThostFtdcRspTransferField, isBankToFuture bool) {
	if pRspTransfer == nil {
		return
	}

	direction := "银行转期货"
	funName := "OnRtnFromBankToFutureByFuture"
	if !isBankToFuture {
		direction = "期货转银行"
		funName = "OnRtnFromFutureToBankByFuture"
	}

	amount := float64(pRspTransfer.TradeAmount)
	errorID := int(pRspTransfer.ErrorID)
	errorMsg := gbkToUtf8(pRspTransfer.ErrorMsg[:])

	if errorID != 0 {
		logger.Error(funName, zap.String("direction", direction),
			zap.Float64("amount", amount), zap.Int("errid", errorID), zap.String("errmsg", errorMsg))
	} else {
		logger.Info(funName, zap.String("direction", direction), zap.Float64("amount", amount))
	}

	s.eventBus.Publish(NewEvent(EventTransferResult, &TransferResultData{
		Raw:            copyRspTransfer(pRspTransfer),
		IsBankToFuture: isBankToFuture,
		ErrorID:        errorID,
		ErrorMsg:       errorMsg,
	}))
}

// OnErrRtnBankToFutureByFuture 银转期错误回报 → 发布 EventTransferError
func (s *CtpSpi) OnErrRtnBankToFutureByFuture(pReqTransfer *thost.CThostFtdcReqTransferField,
	pRspInfo *thost.CThostFtdcRspInfoField) {
	s.publishTransferError(pReqTransfer, pRspInfo, true)
}

// OnErrRtnFutureToBankByFuture 期转银错误回报 → 发布 EventTransferError
func (s *CtpSpi) OnErrRtnFutureToBankByFuture(pReqTransfer *thost.CThostFtdcReqTransferField,
	pRspInfo *thost.CThostFtdcRspInfoField) {
	s.publishTransferError(pReqTransfer, pRspInfo, false)
}

// publishTransferError 解析转账错误 → 发布 EventTransferError
func (s *CtpSpi) publishTransferError(pReqTransfer *thost.CThostFtdcReqTransferField,
	pRspInfo *thost.CThostFtdcRspInfoField, isBankToFuture bool) {
	if pReqTransfer == nil {
		return
	}

	direction := "银行资金转期货"
	funName := "OnErrRtnBankToFutureByFuture"
	if !isBankToFuture {
		direction = "期货资金转银行"
		funName = "OnErrRtnFutureToBankByFuture"
	}

	errorID := 0
	errorMsg := "未知错误"
	if pRspInfo != nil {
		errorID = int(pRspInfo.ErrorID)
		errorMsg = gbkToUtf8(pRspInfo.ErrorMsg[:])
	}

	logger.Error(funName, zap.String("direction", direction),
		zap.Float64("amount", float64(pReqTransfer.TradeAmount)),
		zap.Int("errid", errorID), zap.String("errmsg", errorMsg))

	s.eventBus.Publish(NewEvent(EventTransferError, &TransferErrorData{
		Raw:            copyReqTransfer(pReqTransfer),
		IsBankToFuture: isBankToFuture,
		ErrorID:        errorID,
		ErrorMsg:       errorMsg,
	}))
}

// OnRspQryContractBank 查询签约银行响应 → 发布 EventBankUpdated
func (s *CtpSpi) OnRspQryContractBank(pContractBank *thost.CThostFtdcContractBankField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		logger.Error("OnRspQryContractBank",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])))
		return
	}
	if pContractBank != nil {
		cp := *pContractBank
		s.eventBus.Publish(NewEvent(EventBankUpdated, &BankUpdateData{
			Raw:    &cp,
			IsLast: bIsLast,
		}))
	}
}

// OnRspQryAccountregister 查询银期签约关系响应 → 发布 EventAccountRegisterUpdated
func (s *CtpSpi) OnRspQryAccountregister(pAccountregister *thost.CThostFtdcAccountregisterField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		logger.Error("OnRspQryAccountregister",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])))
		return
	}
	if pAccountregister != nil {
		cp := *pAccountregister
		s.eventBus.Publish(NewEvent(EventAccountRegisterUpdated, &AccountRegisterData{
			Raw:    &cp,
			IsLast: bIsLast,
		}))
	}
}

// OnRspQryTransferSerial 查询转账流水响应 → 发布 EventTransferSerialUpdated
func (s *CtpSpi) OnRspQryTransferSerial(pTransferSerial *thost.CThostFtdcTransferSerialField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		logger.Error("OnRspQryTransferSerial",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])))
		return
	}
	if pTransferSerial != nil {
		cp := *pTransferSerial
		s.eventBus.Publish(NewEvent(EventTransferSerialUpdated, &TransferSerialData{
			Raw:    &cp,
			IsLast: bIsLast,
		}))
	}
}

// ==================== 密码/通知/状态 回调 ====================

// OnRspUserPasswordUpdate 密码修改响应 → 发布 EventPasswordChanged
func (s *CtpSpi) OnRspUserPasswordUpdate(pUserPasswordUpdate *thost.CThostFtdcUserPasswordUpdateField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	isSuccess := pRspInfo == nil || pRspInfo.ErrorID == 0
	errorID := 0
	errorMsg := ""
	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		errorID = int(pRspInfo.ErrorID)
		errorMsg = gbkToUtf8(pRspInfo.ErrorMsg[:])
	}

	s.eventBus.Publish(NewEvent(EventPasswordChanged, &PasswordChangeData{
		IsSuccess: isSuccess,
		ErrorID:   errorID,
		ErrorMsg:  errorMsg,
		IsTrading: false,
	}))
}

// OnRspTradingAccountPasswordUpdate 资金密码修改响应 → 发布 EventPasswordChanged
func (s *CtpSpi) OnRspTradingAccountPasswordUpdate(pTradingAccountPasswordUpdate *thost.CThostFtdcTradingAccountPasswordUpdateField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	isSuccess := pRspInfo == nil || pRspInfo.ErrorID == 0
	errorID := 0
	errorMsg := ""
	if pRspInfo != nil && pRspInfo.ErrorID != 0 {
		errorID = int(pRspInfo.ErrorID)
		errorMsg = gbkToUtf8(pRspInfo.ErrorMsg[:])
	}

	s.eventBus.Publish(NewEvent(EventPasswordChanged, &PasswordChangeData{
		IsSuccess: isSuccess,
		ErrorID:   errorID,
		ErrorMsg:  errorMsg,
		IsTrading: true,
	}))
}

// OnRtnTradingNotice 交易通知 → 发布 EventTradingNotice
func (s *CtpSpi) OnRtnTradingNotice(pTradingNoticeInfo *thost.CThostFtdcTradingNoticeInfoField) {
	if pTradingNoticeInfo == nil {
		return
	}
	content := gbkToUtf8(pTradingNoticeInfo.FieldContent[:])
	logger.Info("trading notice", zap.String("content", content))
	if content != "" {
		s.eventBus.Publish(NewEvent(EventTradingNotice, &TradingNoticeData{
			Content: content,
		}))
	}
}

// OnRtnInstrumentStatus 合约状态通知 → 发布 EventInstrumentStatus
func (s *CtpSpi) OnRtnInstrumentStatus(pInstrumentStatus *thost.CThostFtdcInstrumentStatusField) {
	if pInstrumentStatus == nil {
		return
	}
	exchangeID := bytesToString(pInstrumentStatus.ExchangeID[:])
	instrumentID := bytesToString(pInstrumentStatus.InstrumentID[:])
	status := pInstrumentStatus.InstrumentStatus

	logger.Debug("instrument status",
		zap.String("exchange_id", exchangeID),
		zap.String("instrument_id", instrumentID),
		zap.Uint8("status", uint8(status)),
	)

	s.eventBus.Publish(NewEvent(EventInstrumentStatus, &InstrumentStatusData{
		ExchangeID:   exchangeID,
		InstrumentID: instrumentID,
		Status:       byte(status),
	}))
}

// ==================== CTP 结构体拷贝辅助函数 ====================
// CTP SPI 回调中的指针数据在回调返回后可能被底层回收/覆盖，
// 必须在回调中 copy 一份再通过 Event 传递。

// copyOrder 拷贝订单结构体
func copyOrder(p *thost.CThostFtdcOrderField) *thost.CThostFtdcOrderField {
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}

// copyTrade 拷贝成交结构体
func copyTrade(p *thost.CThostFtdcTradeField) *thost.CThostFtdcTradeField {
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}

// copyInputOrder 拷贝报单录入结构体
func copyInputOrder(p *thost.CThostFtdcInputOrderField) *thost.CThostFtdcInputOrderField {
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}

// copyOrderAction 拷贝撤单结构体
func copyOrderAction(p *thost.CThostFtdcOrderActionField) *thost.CThostFtdcOrderActionField {
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}

// copyRspTransfer 拷贝转账响应结构体
func copyRspTransfer(p *thost.CThostFtdcRspTransferField) *thost.CThostFtdcRspTransferField {
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}

// copyReqTransfer 拷贝转账请求结构体
func copyReqTransfer(p *thost.CThostFtdcReqTransferField) *thost.CThostFtdcReqTransferField {
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}


// bytesToString 将字节数组转换为字符串（去除尾部 0）
func bytesToString(b []byte) string {
	before, _, _ := bytes.Cut(b, []byte{'\x00'})
	if len(before) > 0 {
		return string(before)
	}
	return ""
}

// gbkToUtf8 GBK 转 UTF-8
func gbkToUtf8(b []byte) string {
	before, _, _ := bytes.Cut(b, []byte{'\x00'})
	msg, _ := simplifiedchinese.GB18030.NewDecoder().Bytes(before)
	return strings.TrimRight(string(msg), "\x00")
}

// parseOrderRef 解析报单引用
func parseOrderRef(b []byte) int {
	s := bytesToString(b)
	if s == "" {
		return 0
	}
	var ref int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			ref = ref*10 + int(c-'0')
		}
	}
	return ref
}

// SetNotifySessionID 设置 notify 消息的 session_id (委托给 protocol 包)
func SetNotifySessionID(sessionID int) {
	protocol.SetNotifySessionID(sessionID)
}
