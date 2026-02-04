// Package trader 银期转账
// 对应 C++ traderctp 中的转账相关逻辑
package trader

import (
	"strconv"

	"github.com/pseudocodes/go2ctp/thost"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/protocol"
)

// handleReqTransferFull 处理转账请求
// 对应 C++ traderctp::OnClientReqTransfer
func (t *TraderCTP) handleReqTransferFull(connID int, msg string) {
	if !t.isLoggedIn() {
		// 对应 C++ OutputNotifyAllSycn(335, u8"当前时间不支持转账!", "WARNING")
		t.outputNotify(connID, 335, "当前时间不支持转账!", "WARNING")
		return
	}

	bankID := gjson.Get(msg, "bank_id").String()
	bankPassword := gjson.Get(msg, "bank_password").String()
	futurePassword := gjson.Get(msg, "future_password").String()
	currency := gjson.Get(msg, "currency").String()
	amount := gjson.Get(msg, "amount").Float()

	if bankID == "" {
		t.outputNotify(connID, 1, "银行代码不能为空", "ERROR")
		return
	}

	logger.Info("transfer request",
		zap.String("bank_id", bankID),
		zap.Float64("amount", amount),
	)

	if amount > 0 {
		// 银行转期货
		t.ctpSpi.ReqFromBankToFutureByFuture(bankID, bankPassword, futurePassword, currency, amount)
	} else {
		// 期货转银行
		t.ctpSpi.ReqFromFutureToBankByFuture(bankID, bankPassword, futurePassword, currency, -amount)
	}
}

// ReqFromBankToFutureByFuture 银行转期货
// 对应 C++ traderctp 中的银转期逻辑
func (s *CtpSpi) ReqFromBankToFutureByFuture(bankID, bankPassword, futurePassword, currency string, amount float64) {
	req := &thost.CThostFtdcReqTransferField{}

	copy(req.BrokerID[:], s.brokerID)
	copy(req.AccountID[:], s.userID)
	copy(req.BankID[:], bankID)
	copy(req.BankPassWord[:], bankPassword)
	copy(req.Password[:], futurePassword)
	copy(req.CurrencyID[:], currency)
	req.TradeAmount = thost.TThostFtdcTradeAmountType(amount)
	req.SecuPwdFlag = thost.THOST_FTDC_BPWDF_BlankCheck
	req.BankPwdFlag = thost.THOST_FTDC_BPWDF_NoCheck

	ret := s.api.ReqFromBankToFutureByFuture(req, s.nextRequestID())
	logger.Info("ReqFromBankToFutureByFuture",
		zap.Int("ret", ret),
		zap.Float64("amount", amount),
	)

	// 对应 C++ if (0 != r) OutputNotifyAllSycn(352, u8"银期转账请求发送失败!", "WARNING")
	if ret != 0 {
		s.trader.outputNotifyAll(352, "银期转账请求发送失败!", "WARNING")
	}
}

// ReqFromFutureToBankByFuture 期货转银行
func (s *CtpSpi) ReqFromFutureToBankByFuture(bankID, bankPassword, futurePassword, currency string, amount float64) {
	req := &thost.CThostFtdcReqTransferField{}

	copy(req.BrokerID[:], s.brokerID)
	copy(req.AccountID[:], s.userID)
	copy(req.BankID[:], bankID)
	copy(req.BankPassWord[:], bankPassword)
	copy(req.Password[:], futurePassword)
	copy(req.CurrencyID[:], currency)
	req.TradeAmount = thost.TThostFtdcTradeAmountType(amount)
	req.SecuPwdFlag = thost.THOST_FTDC_BPWDF_BlankCheck
	req.BankPwdFlag = thost.THOST_FTDC_BPWDF_NoCheck

	ret := s.api.ReqFromFutureToBankByFuture(req, s.nextRequestID())
	logger.Info("ReqFromFutureToBankByFuture",
		zap.Int("ret", ret),
		zap.Float64("amount", amount),
	)

	// 对应 C++ if (0 != r) OutputNotifyAllSycn(352,u8"银期转账请求发送失败!","WARNING")
	if ret != 0 {
		s.trader.outputNotifyAll(352, "银期转账请求发送失败!", "WARNING")
	}
}

// OnRtnFromBankToFutureByFuture 银转期通知
// 对应 C++ traderctp::OnRtnFromBankToFutureByFuture
func (s *CtpSpi) OnRtnFromBankToFutureByFuture(pRspTransfer *thost.CThostFtdcRspTransferField) {
	s.handleTransferResult(pRspTransfer, true)
}

// OnRtnFromFutureToBankByFuture 期转银通知
// 对应 C++ traderctp::OnRtnFromFutureToBankByFuture
func (s *CtpSpi) OnRtnFromFutureToBankByFuture(pRspTransfer *thost.CThostFtdcRspTransferField) {
	s.handleTransferResult(pRspTransfer, false)
}

// handleTransferResult 处理转账结果
func (s *CtpSpi) handleTransferResult(pRspTransfer *thost.CThostFtdcRspTransferField, isBankToFuture bool) {
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
		logger.Error(funName,
			zap.String("fun", funName),
			zap.String("direction", direction),
			zap.Float64("amount", amount),
			zap.Int("errid", errorID),
			zap.String("errmsg", errorMsg),
		)
		// 对应 C++ OutputNotifyAllSycn(pRspTransfer->ErrorID, u8"银期错误," + GBKToUTF8(pRspTransfer->ErrorMsg), "WARNING")
		s.trader.outputNotifyAll(int64(errorID), "银期错误,"+errorMsg, "WARNING")
	} else {
		logger.Info(funName,
			zap.String("fun", funName),
			zap.String("direction", direction),
			zap.Float64("amount", amount),
		)
		// 对应 C++ OutputNotifyAllSycn(327,u8"转账成功")
		s.trader.outputNotifyAll(327, "转账成功", "INFO")

		// 请求刷新账户 (对应 C++ m_req_account_id++)
		go s.trader.requestRefreshAfterTransfer()

		// 更新转账记录
		s.trader.updateTransfer(pRspTransfer)
	}
}

// updateTransfer 更新转账记录
func (t *TraderCTP) updateTransfer(pTransfer *thost.CThostFtdcRspTransferField) {
	t.userMu.Lock()
	defer t.userMu.Unlock()

	if t.user == nil {
		return
	}

	serialNo := strconv.FormatInt(int64(pTransfer.PlateSerial), 10)
	transfer := &protocol.TransferLog{
		Currency: bytesToString(pTransfer.CurrencyID[:]),
		Amount:   float64(pTransfer.TradeAmount),
		ErrorID:  int64(pTransfer.ErrorID),
		ErrorMsg: gbkToUtf8(pTransfer.ErrorMsg[:]),
		Changed:  true,
	}

	t.user.Transfers[serialNo] = transfer
}

// ReqQryContractBank 查询签约银行
func (s *CtpSpi) ReqQryContractBank() {
	req := &thost.CThostFtdcQryContractBankField{}
	copy(req.BrokerID[:], s.brokerID)

	ret := s.api.ReqQryContractBank(req, s.nextRequestID())
	logger.Info("ReqQryContractBank", zap.Int("ret", ret))
}

// OnRspQryContractBank 查询签约银行响应
func (s *CtpSpi) OnRspQryContractBank(pContractBank *thost.CThostFtdcContractBankField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if s.isErrorRspInfo(pRspInfo) {
		return
	}

	if pContractBank != nil {
		s.trader.updateBank(pContractBank)
	}

	if bIsLast {
		logger.Info("contract bank query completed")
		s.trader.queryScheduler.SetNeedQueryBank(false)
	}
}

// updateBank 更新银行信息
func (t *TraderCTP) updateBank(pBank *thost.CThostFtdcContractBankField) {
	t.userMu.Lock()
	defer t.userMu.Unlock()

	if t.user == nil {
		return
	}

	bankID := bytesToString(pBank.BankID[:])
	bank := &protocol.Bank{
		BankID:   bankID,
		BankName: gbkToUtf8(pBank.BankName[:]),
		Changed:  true,
	}

	t.user.Banks[bankID] = bank
}

// formatTransferAmount 格式化转账金额
func formatTransferAmount(amount float64) string {
	return formatFloat(amount) + "元"
}

// ==================== 转账错误回报 (Phase 1: Error Handling) ====================

// OnErrRtnBankToFutureByFuture 银转期错误回报
// 对应 C++ traderctp::OnErrRtnBankToFutureByFuture
func (s *CtpSpi) OnErrRtnBankToFutureByFuture(pReqTransfer *thost.CThostFtdcReqTransferField,
	pRspInfo *thost.CThostFtdcRspInfoField) {
	s.handleTransferError(pReqTransfer, pRspInfo, true)
}

// OnErrRtnFutureToBankByFuture 期转银错误回报
// 对应 C++ traderctp::OnErrRtnFutureToBankByFuture
func (s *CtpSpi) OnErrRtnFutureToBankByFuture(pReqTransfer *thost.CThostFtdcReqTransferField,
	pRspInfo *thost.CThostFtdcRspInfoField) {
	s.handleTransferError(pReqTransfer, pRspInfo, false)
}

// handleTransferError 处理转账错误
func (s *CtpSpi) handleTransferError(pReqTransfer *thost.CThostFtdcReqTransferField,
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

	amount := float64(pReqTransfer.TradeAmount)
	errorID := 0
	errorMsg := "未知错误"

	if pRspInfo != nil {
		errorID = int(pRspInfo.ErrorID)
		errorMsg = gbkToUtf8(pRspInfo.ErrorMsg[:])
	}

	logger.Error(funName,
		zap.String("fun", funName),
		zap.String("direction", direction),
		zap.Float64("amount", amount),
		zap.Int("errid", errorID),
		zap.String("errmsg", errorMsg),
	)

	// 对应 C++ OutputNotifyAllSycn(pRspInfo->ErrorID, u8"银行资金转期货错误," / u8"期货资金转银行错误," + GBKToUTF8(pRspInfo->ErrorMsg), "WARNING")
	s.trader.outputNotifyAll(int64(errorID), direction+"错误,"+errorMsg, "WARNING")
}

// ==================== 银期签约关系查询 (Phase 3: Banking Features) ====================

// ReqQryAccountRegister 查询银期签约关系
// 对应 C++ traderctp::ReqQryAccountRegister
func (s *CtpSpi) ReqQryAccountRegister() {
	req := &thost.CThostFtdcQryAccountregisterField{}
	copy(req.BrokerID[:], s.brokerID)

	ret := s.api.ReqQryAccountregister(req, s.nextRequestID())
	logger.Info("ReqQryAccountRegister", zap.Int("ret", ret))
}

// OnRspQryAccountregister 查询银期签约关系响应
// 对应 C++ traderctp::OnRspQryAccountregister
func (s *CtpSpi) OnRspQryAccountregister(pAccountregister *thost.CThostFtdcAccountregisterField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if s.isErrorRspInfo(pRspInfo) {
		return
	}

	if pAccountregister != nil {
		s.trader.updateAccountRegister(pAccountregister)
	}

	if bIsLast {
		logger.Info("account register query completed")
		s.trader.queryScheduler.SetNeedQueryRegister(false)
	}
}

// updateAccountRegister 更新银期签约关系
func (t *TraderCTP) updateAccountRegister(pReg *thost.CThostFtdcAccountregisterField) {
	t.userMu.Lock()
	defer t.userMu.Unlock()

	if t.user == nil {
		return
	}

	bankID := bytesToString(pReg.BankID[:])
	if bank, ok := t.user.Banks[bankID]; ok {
		// 更新现有银行信息
		bank.BankAccount = bytesToString(pReg.BankAccount[:])
		bank.Changed = true
	} else {
		// 创建新的银行信息
		t.user.Banks[bankID] = &protocol.Bank{
			BankID:      bankID,
			BankName:    bankID, // AccountRegister 不包含 BankName，使用 BankID 作为默认名称
			BankAccount: bytesToString(pReg.BankAccount[:]),
			Changed:     true,
		}
	}
}

// ==================== 转账流水查询 (Phase 3: Banking Features) ====================

// ReqQryTransferSerial 查询转账流水
// 对应 C++ traderctp::ReqQryTransferSerial
func (s *CtpSpi) ReqQryTransferSerial() {
	req := &thost.CThostFtdcQryTransferSerialField{}
	copy(req.BrokerID[:], s.brokerID)
	copy(req.AccountID[:], s.userID)

	ret := s.api.ReqQryTransferSerial(req, s.nextRequestID())
	logger.Info("ReqQryTransferSerial", zap.Int("ret", ret))
}

// OnRspQryTransferSerial 查询转账流水响应
// 对应 C++ traderctp::OnRspQryTransferSerial
func (s *CtpSpi) OnRspQryTransferSerial(pTransferSerial *thost.CThostFtdcTransferSerialField,
	pRspInfo *thost.CThostFtdcRspInfoField, nRequestID int, bIsLast bool) {

	if s.isErrorRspInfo(pRspInfo) {
		return
	}

	if pTransferSerial != nil {
		s.trader.updateTransferSerial(pTransferSerial)
	}

	if bIsLast {
		logger.Info("transfer serial query completed")
	}
}

// updateTransferSerial 更新转账流水
func (t *TraderCTP) updateTransferSerial(pSerial *thost.CThostFtdcTransferSerialField) {
	t.userMu.Lock()
	defer t.userMu.Unlock()

	if t.user == nil {
		return
	}

	serialNo := strconv.FormatInt(int64(pSerial.PlateSerial), 10)
	direction := "入金"
	// TradeCode 是 [7]byte，检查第一个字节
	tradeCode := bytesToString(pSerial.TradeCode[:])
	if len(tradeCode) > 0 && (tradeCode[0] == '2' || tradeCode == "202001" || tradeCode == "202002") {
		direction = "出金" // 期货转银行
	}

	transfer := &protocol.TransferLog{
		Currency:  bytesToString(pSerial.CurrencyID[:]),
		Amount:    float64(pSerial.TradeAmount),
		DateTime:  bytesToString(pSerial.TradeDate[:]) + " " + bytesToString(pSerial.TradeTime[:]),
		ErrorID:   int64(pSerial.ErrorID),
		ErrorMsg:  gbkToUtf8(pSerial.ErrorMsg[:]),
		Direction: direction,
		Changed:   true,
	}

	t.user.Transfers[serialNo] = transfer
}
