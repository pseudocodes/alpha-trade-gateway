// Package trader 其他功能
// 对应 C++ traderctp 中的其他功能
package trader

import (
	"strconv"

	"github.com/pseudocodes/go2ctp/thost"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/logger"
)

// formatFloat 格式化浮点数
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', 2, 64)
}

// handleChangePasswordFull 处理密码修改请求
// 对应 C++ traderctp::OnClientReqChangePassword
func (t *TraderCTP) handleChangePasswordFull(connID int, msg string) {
	if !t.isLoggedIn() {
		t.outputNotify(connID, 2, "请先登录", "ERROR")
		return
	}

	oldPassword := gjson.Get(msg, "old_password").String()
	newPassword := gjson.Get(msg, "new_password").String()

	if oldPassword == "" || newPassword == "" {
		t.outputNotify(connID, 1, "密码不能为空", "ERROR")
		return
	}

	logger.Info("change password request", zap.Int("conn_id", connID))

	t.ctpSpi.ReqUserPasswordUpdate(oldPassword, newPassword)
}

// ReqUserPasswordUpdate 修改用户密码
func (s *CtpSpi) ReqUserPasswordUpdate(oldPassword, newPassword string) {
	req := &thost.CThostFtdcUserPasswordUpdateField{}
	copy(req.BrokerID[:], s.brokerID)
	copy(req.UserID[:], s.userID)
	copy(req.OldPassword[:], oldPassword)
	copy(req.NewPassword[:], newPassword)

	ret := s.api.ReqUserPasswordUpdate(req, s.nextRequestID())
	logger.Info("ReqUserPasswordUpdate", zap.Int("ret", ret))
}

// OnRspUserPasswordUpdate 密码修改响应
func (s *CtpSpi) OnRspUserPasswordUpdate(pUserPasswordUpdate *thost.CThostFtdcUserPasswordUpdateField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if s.isErrorRspInfo(pRspInfo) {
		s.trader.outputNotifyAll(int64(pRspInfo.ErrorID), gbkToUtf8(pRspInfo.ErrorMsg[:]), "ERROR")
		return
	}

	logger.Info("password changed successfully")
	s.trader.outputNotifyAll(0, "密码修改成功", "INFO")
}

// OnRtnTradingNotice 交易通知
// 对应 C++ traderctp::OnRtnTradingNotice
func (s *CtpSpi) OnRtnTradingNotice(pTradingNoticeInfo *thost.CThostFtdcTradingNoticeInfoField) {
	if pTradingNoticeInfo == nil {
		return
	}

	content := gbkToUtf8(pTradingNoticeInfo.FieldContent[:])
	logger.Info("trading notice", zap.String("content", content))

	s.trader.outputNotifyAll(326, content, "INFO")
}

// OnRtnInstrumentStatus 合约状态通知
// 对应 C++ traderctp::OnRtnInstrumentStatus
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
}

// handleConfirmSettlementFull 处理确认结算单请求
// 对应 C++ aid == "confirm_settlement" 处理
func (t *TraderCTP) handleConfirmSettlementFull(connID int) {
	loginState := t.getState()
	if loginState == StateInit || loginState == StateStopped || loginState == StateStopping {
		t.outputNotify(connID, 336, "请先登录", "WARNING")
		return
	}

	logger.Info("confirm settlement request", zap.Int("conn_id", connID))
	t.ctpSpi.reqSettlementInfoConfirm()
}

// handleQrySettlementInfoFull 处理查询结算单请求
// 对应 C++ aid == "qry_settlement_info" 处理
func (t *TraderCTP) handleQrySettlementInfoFull(connID int, msg string) {
	if !t.isLoggedIn() {
		t.outputNotify(connID, 337, "请先登录", "WARNING")
		return
	}

	tradingDay := gjson.Get(msg, "trading_day").Int()
	logger.Info("query settlement info request",
		zap.Int("conn_id", connID),
		zap.Int64("trading_day", tradingDay),
	)

	// 历史结算单查询
	if tradingDay > 0 {
		t.ctpSpi.ReqQrySettlementInfoHistory(int(tradingDay))
	} else {
		// 当日结算单已在登录时查询
		t.sendSettlementInfo()
	}
}

// ReqQrySettlementInfoHistory 查询历史结算单
func (s *CtpSpi) ReqQrySettlementInfoHistory(tradingDay int) {
	req := &thost.CThostFtdcQrySettlementInfoField{}
	copy(req.BrokerID[:], s.brokerID)
	copy(req.InvestorID[:], s.userID)

	// 设置交易日
	tradingDayStr := formatTradingDay(tradingDay)
	copy(req.TradingDay[:], tradingDayStr)

	ret := s.api.ReqQrySettlementInfo(req, s.nextRequestID())
	logger.Info("ReqQrySettlementInfo (history)",
		zap.Int("ret", ret),
		zap.String("trading_day", tradingDayStr),
	)
}

// formatTradingDay 格式化交易日
func formatTradingDay(day int) string {
	if day <= 0 {
		return ""
	}
	// 格式: YYYYMMDD
	return formatInt(day)
}

// formatInt 格式化整数
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + formatInt(-n)
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// ==================== 资金密码修改 (Phase 5: Query Features) ====================

// handleChangeTradingAccountPassword 处理资金密码修改请求
// 对应 C++ traderctp::OnClientReqChangePassword (类型为资金密码时)
func (t *TraderCTP) handleChangeTradingAccountPassword(connID int, msg string) {
	if !t.isLoggedIn() {
		t.outputNotify(connID, 2, "请先登录", "ERROR")
		return
	}

	oldPassword := gjson.Get(msg, "old_password").String()
	newPassword := gjson.Get(msg, "new_password").String()

	if oldPassword == "" || newPassword == "" {
		t.outputNotify(connID, 1, "密码不能为空", "ERROR")
		return
	}

	logger.Info("change trading account password request", zap.Int("conn_id", connID))

	t.ctpSpi.ReqTradingAccountPasswordUpdate(oldPassword, newPassword)
}

// ReqTradingAccountPasswordUpdate 修改资金账户密码
// 对应 C++ traderctp::ReqChangeTradingAccountPassword
func (s *CtpSpi) ReqTradingAccountPasswordUpdate(oldPassword, newPassword string) {
	req := &thost.CThostFtdcTradingAccountPasswordUpdateField{}
	copy(req.BrokerID[:], s.brokerID)
	copy(req.AccountID[:], s.userID)
	copy(req.OldPassword[:], oldPassword)
	copy(req.NewPassword[:], newPassword)
	copy(req.CurrencyID[:], "CNY")

	ret := s.api.ReqTradingAccountPasswordUpdate(req, s.nextRequestID())
	logger.Info("ReqTradingAccountPasswordUpdate", zap.Int("ret", ret))
}

// OnRspTradingAccountPasswordUpdate 资金密码修改响应
// 对应 C++ traderctp::OnRspTradingAccountPasswordUpdate
func (s *CtpSpi) OnRspTradingAccountPasswordUpdate(pTradingAccountPasswordUpdate *thost.CThostFtdcTradingAccountPasswordUpdateField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if s.isErrorRspInfo(pRspInfo) {
		s.trader.outputNotifyAll(int64(pRspInfo.ErrorID), gbkToUtf8(pRspInfo.ErrorMsg[:]), "ERROR")
		return
	}

	logger.Info("trading account password changed successfully")
	s.trader.outputNotifyAll(0, "资金密码修改成功", "INFO")
}
