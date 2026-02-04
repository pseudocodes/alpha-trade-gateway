// Package trader CTP API 封装
// 对应 C++ tradectp 中的 CTP 相关逻辑
// 使用 go2ctp 库封装 CTP TraderApi
package trader

import (
	"bytes"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/pseudocodes/go2ctp/ctp"
	"github.com/pseudocodes/go2ctp/thost"
	"go.uber.org/zap"
	"golang.org/x/text/encoding/simplifiedchinese"

	"alpha-trade-gateway/pkg/config"
	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/protocol"
)

// CtpSpi CTP 回调处理
// 对应 C++ CThostFtdcTraderSpi 的实现
type CtpSpi struct {
	ctp.BaseTraderSpi

	trader *TraderCTP
	api    thost.TraderApi

	// CTP 登录信息
	brokerID    string
	userID      string
	password    string
	appID       string
	authCode    string
	productInfo string

	// 请求序号
	requestID atomic.Int32

	// 前置编号和会话编号
	frontID   int
	sessionID int

	// 报单引用序列号
	// 对应 C++ m_order_ref，登录时从 CTP MaxOrderRef 初始化
	// 每次下单时递增使用，确保不与已有订单冲突
	orderRefSeq atomic.Int32
}

// NewCtpSpi 创建 CTP SPI
func NewCtpSpi(trader *TraderCTP) *CtpSpi {
	return &CtpSpi{
		trader: trader,
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

// SetApi 设置 TraderApi
func (s *CtpSpi) SetApi(api thost.TraderApi) {
	s.api = api
}

// nextRequestID 获取下一个请求 ID
func (s *CtpSpi) nextRequestID() int {
	return int(s.requestID.Add(1))
}

// nextOrderRef 获取下一个报单引用
// 对应 C++ rkey.order_ref = std::to_string(m_order_ref++)
// 每次下单时调用，返回递增后的 OrderRef 字符串
func (s *CtpSpi) nextOrderRef() string {
	return fmt.Sprintf("%d", s.orderRefSeq.Add(1))
}

// OnFrontConnected 前置连接成功回调
// 对应 C++ traderctp::OnFrontConnected
func (s *CtpSpi) OnFrontConnected() {
	logger.Info("OnFrontConnected",
		zap.String("fun", "OnFrontConnected"),
	)
	s.trader.setState(StateConnected)
	if s.trader.isLoggedIn() {
		// 已登录，可能是重连
		// 对应 C++ OutputNotifyAllSycn(320,u8"已经重新连接到交易前置")
		logger.Info("already logged in, reconnecting")
		s.trader.outputNotifyAll(320, "已经重新连接到交易前置", "INFO")
	} else {
		// 首次连接
		// 对应 C++ OutputNotifySycn(m_loging_connectId,321,u8"已经连接到交易前置")
		s.trader.outputNotifyAll(321, "已经连接到交易前置", "INFO")
	}

	// 发送认证请求
	// 对应 C++ ReqAuthenticate
	s.reqAuthenticate()
}

// OnFrontDisconnected 前置断开回调
// 对应 C++ traderctp::OnFrontDisconnected
func (s *CtpSpi) OnFrontDisconnected(nReason int) {
	logger.Warn("OnFrontDisconnected",
		zap.String("fun", "OnFrontDisconnected"),
		zap.Int("reason", nReason),
	)
	if !s.trader.isLoggedIn() {
		// 未登录，直接返回
		s.trader.setState(StateInit)
	}
	// 通知客户端已断开连接
	// 对应 C++ OutputNotifyAllSycn(322,u8"已经断开与交易前置的连接")
	s.trader.outputNotifyAll(322, "已经断开与交易前置的连接", "INFO")
}

// OnHeartBeatWarning 心跳超时警告
func (s *CtpSpi) OnHeartBeatWarning(nTimeLapse int) {
	logger.Warn("CTP heartbeat warning",
		zap.Int("time_lapse", nTimeLapse),
	)
}

// reqAuthenticate 发送认证请求
// 对应 C++ traderctp 中的认证逻辑
func (s *CtpSpi) reqAuthenticate() {
	req := &thost.CThostFtdcReqAuthenticateField{}
	copy(req.BrokerID[:], s.brokerID)
	copy(req.UserID[:], s.userID)
	copy(req.AppID[:], s.appID)
	copy(req.AuthCode[:], s.authCode)

	ret := s.api.ReqAuthenticate(req, s.nextRequestID())
	logger.Info("ReqAuthenticate",
		zap.Int("ret", ret),
		zap.String("broker_id", s.brokerID),
		zap.String("user_id", s.userID),
	)
	s.trader.setState(StateAuthenticating)
}

// OnRspAuthenticate 认证响应
// 对应 C++ traderctp::OnRspAuthenticate
func (s *CtpSpi) OnRspAuthenticate(pRspAuthenticateField *thost.CThostFtdcRspAuthenticateField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if s.isErrorRspInfo(pRspInfo) {
		logger.Error("OnRspAuthenticate",
			zap.String("fun", "OnRspAuthenticate"),
			zap.Int("errid", int(pRspInfo.ErrorID)),
			zap.String("errmsg", gbkToUtf8(pRspInfo.ErrorMsg[:])),
			zap.Bool("is_last", bIsLast),
			zap.Int("request_id", nRequestID),
		)
		s.trader.outputNotifyAll(int64(pRspInfo.ErrorID), gbkToUtf8(pRspInfo.ErrorMsg[:]), "ERROR")
		return
	}

	logger.Info("OnRspAuthenticate",
		zap.String("fun", "OnRspAuthenticate"),
		zap.Bool("is_last", bIsLast),
		zap.Int("request_id", nRequestID),
	)
	s.trader.setState(StateAuthenticated)
	s.trader.tryReqAuthenticateTimes = 0

	// 发送登录请求
	s.reqUserLogin()
}

// reqUserLogin 发送登录请求
// 对应 C++ traderctp 中的登录逻辑
func (s *CtpSpi) reqUserLogin() {
	req := &thost.CThostFtdcReqUserLoginField{}
	copy(req.BrokerID[:], s.brokerID)
	copy(req.UserID[:], s.userID)
	copy(req.Password[:], s.password)

	// SE15 穿透式认证需要的额外字段
	if s.productInfo != "" {
		copy(req.UserProductInfo[:], s.productInfo)
	}

	ret := s.api.ReqUserLogin(req, s.nextRequestID())
	logger.Info("ReqUserLogin",
		zap.Int("ret", ret),
	)
	s.trader.setState(StateLoggingIn)
}

// OnRspUserLogin 登录响应
// 对应 C++ traderctp::OnRspUserLogin
func (s *CtpSpi) OnRspUserLogin(pRspUserLogin *thost.CThostFtdcRspUserLoginField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if s.isErrorRspInfo(pRspInfo) {
		logger.Error("OnRspUserLogin",
			zap.String("fun", "OnRspUserLogin"),
			zap.Int("errid", int(pRspInfo.ErrorID)),
			zap.String("errmsg", gbkToUtf8(pRspInfo.ErrorMsg[:])),
			zap.Bool("is_last", bIsLast),
			zap.Int("request_id", nRequestID),
		)
		errMsg := "交易服务器登录失败," + pRspInfo.ErrorMsg.GBString()
		s.trader.outputNotifyAll(int64(pRspInfo.ErrorID), gbkToUtf8([]byte(errMsg)), "WARNING")
		return
	}

	// 保存登录信息
	s.frontID = int(pRspUserLogin.FrontID)
	s.sessionID = int(pRspUserLogin.SessionID)

	// 从 CTP 获取当前最大报单引用并初始化序列号
	// 对应 C++ m_order_ref = atoi(pRspUserLogin->MaxOrderRef)
	// 这确保每次登录/重连后，OrderRef 从 CTP 返回的最大值开始递增，避免与已有订单冲突
	maxOrderRef := parseOrderRef(pRspUserLogin.MaxOrderRef[:])
	s.orderRefSeq.Store(int32(maxOrderRef))

	// 设置 notify 消息的 session_id
	// 对应 C++ m_session_id = pRspUserLogin->SessionID
	protocol.SetNotifySessionID(s.sessionID)

	tradingDay := bytesToString(pRspUserLogin.TradingDay[:])

	logger.Info("OnRspUserLogin",
		zap.String("fun", "OnRspUserLogin"),
		zap.Int("front_id", s.frontID),
		zap.Int("session_id", s.sessionID),
		zap.String("trading_day", tradingDay),
		zap.String("sys_version", bytesToString(pRspUserLogin.SysVersion[:])),
		zap.Bool("is_last", bIsLast),
		zap.Int("request_id", nRequestID),
	)
	if s.trader.getState() >= StateLoggedIn {
		// 已登录，可能是重连
		// 对应 C++ OutputNotifyAllSycn(323,u8"交易服务器重登录成功")
		logger.Info("already logged in, re-login success")
		s.trader.outputNotifyAll(323, "交易服务器重登录成功", "INFO")
	} else {
		s.trader.setState(StateLoggedIn)
	}

	// 检测交易日切换
	// 对应 C++ OnRspUserLogin 中 if (m_trading_day != trading_day) 逻辑 (line 288-360)
	s.trader.userMu.RLock()
	oldTradingDay := ""
	if s.trader.user != nil {
		oldTradingDay = s.trader.user.TradingDay
	}
	s.trader.userMu.RUnlock()

	if oldTradingDay != "" && oldTradingDay != tradingDay {
		// 交易日变化，需要清空上一交易日的动态数据
		logger.Info("trading day changed, clearing old data",
			zap.String("old_day", oldTradingDay),
			zap.String("new_day", tradingDay),
		)
		s.trader.clearOldDataForNewTradingDay(tradingDay)
	} else {
		// 初始化用户数据 (首次登录或同一交易日重连)
		s.trader.initUserData(s.userID, tradingDay)
	}
	// 通知客户端登录成功
	// 对应 C++ OutputNotifySycn(m_loging_connectId,324,u8"登录成功")
	s.trader.outputNotifyAll(324, "登录成功", "INFO")

	// 确认结算单
	s.trader.setState(StateSettlementQuerying)
	s.reqQrySettlementInfo()
}

// reqQrySettlementInfo 查询结算单
func (s *CtpSpi) reqQrySettlementInfo() {
	req := &thost.CThostFtdcQrySettlementInfoField{}
	copy(req.BrokerID[:], s.brokerID)
	copy(req.InvestorID[:], s.userID)

	ret := s.api.ReqQrySettlementInfo(req, s.nextRequestID())
	logger.Info("ReqQrySettlementInfo", zap.Int("ret", ret))
}

// OnRspQrySettlementInfo 查询结算单响应
// 对应 C++ traderctp::OnRspQrySettlementInfo
func (s *CtpSpi) OnRspQrySettlementInfo(pSettlementInfo *thost.CThostFtdcSettlementInfoField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if s.isErrorRspInfo(pRspInfo) {
		return
	}

	if pSettlementInfo != nil {
		content := gbkToUtf8(pSettlementInfo.Content[:])
		s.trader.appendSettlementInfo(content)
	}

	if bIsLast {
		logger.Info("settlement info query completed")
		s.trader.onSettlementInfoComplete()
	}
}

// reqSettlementInfoConfirm 确认结算单
func (s *CtpSpi) reqSettlementInfoConfirm() {
	if s.api == nil {
		logger.Warn("reqSettlementInfoConfirm: api is nil")
		return
	}

	req := &thost.CThostFtdcSettlementInfoConfirmField{}
	copy(req.BrokerID[:], s.brokerID)
	copy(req.InvestorID[:], s.userID)

	ret := s.api.ReqSettlementInfoConfirm(req, s.nextRequestID())
	logger.Info("ReqSettlementInfoConfirm", zap.Int("ret", ret))
	s.trader.setState(StateSettlementConfirming)
}

// OnRspSettlementInfoConfirm 确认结算单响应
// 对应 C++ traderctp::OnRspSettlementInfoConfirm
func (s *CtpSpi) OnRspSettlementInfoConfirm(pSettlementInfoConfirm *thost.CThostFtdcSettlementInfoConfirmField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if s.isErrorRspInfo(pRspInfo) {
		return
	}
	if bIsLast {
		logger.Info("settlement info confirmed")
		s.trader.onSettlementConfirmed()
	}
}

// ReqQrySettlementInfoConfirm 查询结算单确认状态
func (s *CtpSpi) ReqQrySettlementInfoConfirm() {
	req := &thost.CThostFtdcQrySettlementInfoConfirmField{}
	copy(req.BrokerID[:], s.brokerID)
	copy(req.InvestorID[:], s.userID)

	ret := s.api.ReqQrySettlementInfoConfirm(req, s.nextRequestID())
	logger.Info("ReqQrySettlementInfoConfirm", zap.Int("ret", ret))
}

// OnRspQrySettlementInfoConfirm 查询结算单确认响应
func (s *CtpSpi) OnRspQrySettlementInfoConfirm(pSettlementInfoConfirm *thost.CThostFtdcSettlementInfoConfirmField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if s.isErrorRspInfo(pRspInfo) {
		return
	}

	if pSettlementInfoConfirm != nil {
		confirmDate := bytesToString(pSettlementInfoConfirm.ConfirmDate[:])
		confirmTime := bytesToString(pSettlementInfoConfirm.ConfirmTime[:])
		logger.Info("settlement already confirmed",
			zap.String("confirm_date", confirmDate),
			zap.String("confirm_time", confirmTime),
		)
		s.trader.onSettlementConfirmed()
	}
}

// OnRspError 错误响应
func (s *CtpSpi) OnRspError(pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {
	if pRspInfo != nil {
		logger.Error("CTP error",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])),
		)
	}
}

// isErrorRspInfo 检查响应是否有错误
func (s *CtpSpi) isErrorRspInfo(pRspInfo *thost.CThostFtdcRspInfoField) bool {
	if pRspInfo == nil {
		return false
	}
	if pRspInfo.ErrorID != 0 {
		logger.Error("CTP response error",
			zap.Int("error_id", int(pRspInfo.ErrorID)),
			zap.String("error_msg", gbkToUtf8(pRspInfo.ErrorMsg[:])),
		)
		return true
	}
	return false
}

// 辅助函数

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

// ==================== 合约查询相关 ====================

// reqQryInstrument 查询全量合约信息
// 对应 C++ ReqQryInstrument
func (s *CtpSpi) reqQryInstrument() {
	if s.api == nil {
		logger.Error("api is nil when reqQryInstrument")
		return
	}

	req := &thost.CThostFtdcQryInstrumentField{}
	// 空的查询条件，查询所有合约
	reqID := s.nextRequestID()

	ret := s.api.ReqQryInstrument(req, reqID)
	logger.Info("ReqQryInstrument sent",
		zap.Int("request_id", reqID),
		zap.Int("ret", ret),
	)
}

// OnRspQryInstrument 合约查询响应
// 对应 C++ traderctp::OnRspQryInstrument
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
		s.processInstrument(pInstrument)
	}

	if bIsLast {
		logger.Info("instrument query completed")
		s.trader.onInstrumentQueryComplete()
	}
}

// processInstrument 处理单个合约信息
func (s *CtpSpi) processInstrument(pInstrument *thost.CThostFtdcInstrumentField) {
	exchangeID := bytesToString(pInstrument.ExchangeID[:])
	instrumentID := bytesToString(pInstrument.InstrumentID[:])
	// symbol := exchangeID + "." + instrumentID

	ins := &protocol.Instrument{
		VolumeMultiple: int64(pInstrument.VolumeMultiple),
		PriceTick:      float64(pInstrument.PriceTick),
		ExchangeID:     exchangeID,
		InstrumentID:   instrumentID,
	}

	// 设置产品类型
	switch pInstrument.ProductClass {
	case thost.THOST_FTDC_PC_Futures:
		ins.ProductClass = protocol.ProductClassFutures
	case thost.THOST_FTDC_PC_Options, thost.THOST_FTDC_PC_SpotOption:
		ins.ProductClass = protocol.ProductClassOptions
	case thost.THOST_FTDC_PC_Combination:
		ins.ProductClass = protocol.ProductClassCombination
	default:
		ins.ProductClass = protocol.ProductClassFutures
	}

	// 检查是否过期
	// 简化处理：如果是期货，检查到期日
	// if len(pInstrument.ExpireDate) > 0 {
	// 	expireDate := bytesToString(pInstrument.ExpireDate[:])
	// 	if expireDate != "" && expireDate < s.trader.user.TradingDay {
	// 		ins.Expired = true
	// 	}
	// }

	s.trader.setInstrument(instrumentID, ins)
}
