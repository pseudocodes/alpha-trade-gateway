// Package trader 消息处理
// 对应 C++ traderctp::ProcessInMsg
package trader

import (
	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/protocol"
)

// Aid 消息类型常量
// 对应 C++ aid 字段值
const (
	AidReqLogin                     = "req_login"
	AidChangePassword               = "change_password"
	AidPeekMessage                  = "peek_message"
	AidInsertOrder                  = "insert_order"
	AidCancelOrder                  = "cancel_order"
	AidReqTransfer                  = "req_transfer"
	AidConfirmSettlement            = "confirm_settlement"
	AidQrySettlementInfo            = "qry_settlement_info"
	AidReqReconnectTrade            = "req_reconnect_trade"
	AidQryTransferSerial            = "qry_transfer_serial"             // 新增
	AidQryAccountInfo               = "qry_account_info"                // 新增
	AidQryAccountRegister           = "qry_account_register"            // 新增
	AidChangeTradingAccountPassword = "change_trading_account_password" // 新增
	AidReqStartCtp                  = "req_start_ctp"                   // 新增
	AidReqStopCtp                   = "req_stop_ctp"                    // 新增

	// 条件单相关
	AidInsertConditionOrder = "insert_condition_order"
	AidCancelConditionOrder = "cancel_condition_order"
	AidPauseConditionOrder  = "pause_condition_order"
	AidResumeConditionOrder = "resume_condition_order"
	AidQryConditionOrder    = "qry_condition_order"
	AidQryHisConditionOrder = "qry_his_condition_order"

	// 响应类型
	AidRtnData    = "rtn_data"
	AidRtnBrokers = "rtn_brokers"
)

// processMessage 处理消息
// 对应 C++ traderctp::ProcessInMsg
func (t *TraderCTP) processMessage(connID int, msg string) {
	// 解析 JSON 获取 aid 字段
	aid := gjson.Get(msg, "aid").String()
	if aid == "" {
		logger.Warn("message missing aid field",
			zap.Int("conn_id", connID),
			zap.String("msg_preview", truncateMsg(msg, 100)),
		)
		return
	}

	logger.Debug("processing message",
		zap.Int("conn_id", connID),
		zap.String("aid", aid),
	)

	switch aid {
	case AidReqLogin:
		t.handleReqLogin(connID, msg)

	case AidChangePassword:
		t.handleChangePassword(connID, msg)

	case AidPeekMessage:
		t.handlePeekMessage(connID)

	case AidInsertOrder:
		t.handleInsertOrder(connID, msg)

	case AidCancelOrder:
		t.handleCancelOrder(connID, msg)

	case AidReqTransfer:
		t.handleReqTransfer(connID, msg)

	case AidConfirmSettlement:
		t.handleConfirmSettlement(connID, msg)

	case AidQrySettlementInfo:
		t.handleQrySettlementInfo(connID, msg)

	case AidReqReconnectTrade:
		t.handleReqReconnectTrade(connID)

	case AidQryTransferSerial:
		t.handleQryTransferSerial(connID)

	case AidQryAccountInfo:
		t.handleQryAccountInfo(connID)

	case AidQryAccountRegister:
		t.handleQryAccountRegister(connID)

	case AidChangeTradingAccountPassword:
		// 对应 C++ if (nullptr == m_pTdApi) OutputNotifyAllSycn(362, u8"当前时间不支持修改资金密码!", "WARNING")
		if t.ctpApi == nil {
			t.outputNotifyAll(362, "当前时间不支持修改资金密码!", "WARNING")
			return
		}
		t.handleChangeTradingAccountPassword(connID, msg)

	case AidReqStartCtp:
		t.handleReqStartCtp(connID)

	case AidReqStopCtp:
		t.handleReqStopCtp(connID)

	// 条件单处理
	case AidInsertConditionOrder:
		t.handleInsertConditionOrder(connID, msg)
	case AidCancelConditionOrder:
		t.handleCancelConditionOrder(connID, msg)
	case AidPauseConditionOrder:
		t.handlePauseConditionOrder(connID, msg)
	case AidResumeConditionOrder:
		t.handleResumeConditionOrder(connID, msg)
	case AidQryConditionOrder:
		t.handleQryConditionOrder(connID)
	case AidQryHisConditionOrder:
		t.handleQryHisConditionOrder(connID, msg)

	default:
		logger.Warn("unknown message type",
			zap.Int("conn_id", connID),
			zap.String("aid", aid),
		)
	}
}

// handleReqLogin 处理登录请求
// 对应 C++ traderctp::ProcessReqLogIn
func (t *TraderCTP) handleReqLogin(connID int, msg string) {
	logger.Info("handling login request", zap.Int("conn_id", connID))

	// 解析登录请求
	req := &protocol.ReqLogin{
		Aid:      gjson.Get(msg, "aid").String(),
		Bid:      gjson.Get(msg, "bid").String(),
		UserName: gjson.Get(msg, "user_name").String(),
		Password: gjson.Get(msg, "password").String(),
	}

	// 验证必要字段
	if req.Bid == "" || req.UserName == "" || req.Password == "" {
		logger.Warn("login request missing required fields",
			zap.Int("conn_id", connID),
		)
		t.outputNotify(connID, 1, "登录请求参数不完整", "ERROR")
		return
	}

	logger.Info("login request received",
		zap.Int("conn_id", connID),
		zap.String("bid", req.Bid),
		zap.String("user_name", req.UserName),
	)

	// 阶段 4 实现：CTP 登录流程
	t.processReqLogin(connID, req)
}

// handleChangePassword 处理密码修改请求
// 对应 C++ aid == "change_password" 处理
func (t *TraderCTP) handleChangePassword(connID int, msg string) {
	t.handleChangePasswordFull(connID, msg)
}

// handlePeekMessage 处理 peek_message 请求
// 对应 C++ OnClientPeekMessage
func (t *TraderCTP) handlePeekMessage(connID int) {
	if !t.isLoggedIn() {
		return
	}
	t.peekMessage.Store(true)
	// 发送用户数据
	t.sendAllUserData()
}

// handleInsertOrder 处理下单请求
// 对应 C++ aid == "insert_order" 处理
func (t *TraderCTP) handleInsertOrder(connID int, msg string) {
	// 对应 C++ if (nullptr == m_pTdApi) OutputNotifyAllSycn(333, u8"当前时间不支持下单!", "WARNING")
	if t.ctpApi == nil {
		t.outputNotifyAll(333, "当前时间不支持下单!", "WARNING")
		return
	}
	t.handleInsertOrderFull(connID, msg)
}

// handleCancelOrder 处理撤单请求
// 对应 C++ aid == "cancel_order" 处理
func (t *TraderCTP) handleCancelOrder(connID int, msg string) {
	// 对应 C++ if (nullptr == m_pTdApi) OutputNotifyAllSycn(334, u8"当前时间不支持撤单!", "WARNING")
	if t.ctpApi == nil {
		t.outputNotifyAll(334, "当前时间不支持撤单!", "WARNING")
		return
	}
	t.handleCancelOrderFull(connID, msg)
}

// handleReqTransfer 处理转账请求
// 对应 C++ aid == "req_transfer" 处理
func (t *TraderCTP) handleReqTransfer(connID int, msg string) {
	// 对应 C++ if (nullptr == m_pTdApi) OutputNotifyAllSycn(335, u8"当前时间不支持转账!", "WARNING")
	if t.ctpApi == nil {
		t.outputNotifyAll(335, "当前时间不支持转账!", "WARNING")
		return
	}
	t.handleReqTransferFull(connID, msg)
}

// handleConfirmSettlement 处理确认结算单请求
// 对应 C++ aid == "confirm_settlement" 处理
func (t *TraderCTP) handleConfirmSettlement(connID int, msg string) {
	// 对应 C++ if (nullptr == m_pTdApi) OutputNotifyAllSycn(336, u8"当前时间不支持确认结算单!", "WARNING")
	if t.ctpApi == nil {
		t.outputNotifyAll(336, "当前时间不支持确认结算单!", "WARNING")
		return
	}
	t.handleConfirmSettlementFull(connID)
}

// handleQrySettlementInfo 处理查询结算单请求
// 对应 C++ aid == "qry_settlement_info" 处理
func (t *TraderCTP) handleQrySettlementInfo(connID int, msg string) {
	// 对应 C++ if (nullptr == m_pTdApi) OutputNotifyAllSycn(337, u8"当前时间不支持查询历史结算单!", "WARNING")
	if t.ctpApi == nil {
		t.outputNotifyAll(337, "当前时间不支持查询历史结算单!", "WARNING")
		return
	}
	t.handleQrySettlementInfoFull(connID, msg)
}

// processReqLogin 处理登录流程
// 对应 C++ traderctp::ProcessReqLogIn
func (t *TraderCTP) processReqLogin(connID int, req *protocol.ReqLogin) {
	t.processReqLoginFull(connID, req)
}

// isLoggedIn 检查是否已登录
func (t *TraderCTP) isLoggedIn() bool {
	return t.getState() >= StateLoggedIn
}

// sendUserData 发送用户数据给指定连接
// 对应 C++ traderctp::SendUserDataImd
func (t *TraderCTP) sendUserData(connID int) {
	t.userMu.Lock()
	defer t.userMu.Unlock()

	if t.user == nil {
		return
	}

	// 先更新所有持仓的盈亏 (使用最新行情)
	// 对应 C++ SendUserDataImd 中重算持仓盈亏的逻辑 (4453-4749行)
	// 重算账户盈亏
	// 对应 C++ SendUserDataImd 中重算资金账户逻辑 (4751-4807行)
	t.recalculatePositionAndAccountProfit()

	// 构建完整的用户数据消息
	// 对应 C++ SendUserDataImd 中构建数据包逻辑 (4810-4831行)
	msg := t.buildUserDataMsgFull()

	logger.Debug("sendUserData",
		zap.Int("conn_id", connID),
		zap.Int("msg_len", len(msg)),
	)

	t.SendMsg(connID, msg)
	t.peekMessage.Store(false)
}

// outputNotify 发送通知消息
// 对应 C++ traderctp::OutputNotifySycn
func (t *TraderCTP) outputNotify(connID int, code int64, content string, level string) {
	msg := protocol.BuildNotifyMsg(code, content, level)
	t.SendMsg(connID, msg)
}

// outputNotifyAll 发送通知给所有连接
// 对应 C++ traderctp::OutputNotifyAllSycn
func (t *TraderCTP) outputNotifyAll(code int64, content string, level string) {
	msg := protocol.BuildNotifyMsg(code, content, level)
	t.SendMsgAll(msg)
}

// ==================== 缺失的客户端请求处理 (补充实现) ====================

// handleQryAccountInfo 处理查询资金账户请求
// 对应 C++ aid == "qry_account_info" 处理 (line 5280-5297)
func (t *TraderCTP) handleQryAccountInfo(connID int) {
	logger.Info("handling qry_account_info request", zap.Int("conn_id", connID))

	// 对应 C++ if (nullptr == m_pTdApi) OutputNotifyAllSycn(360, u8"当前时间不支持查询资金账号!", "WARNING")
	if t.ctpApi == nil {
		t.outputNotifyAll(360, "当前时间不支持查询资金账号!", "WARNING")
		return
	}

	// 对应 C++ m_req_account_id++ - 触发账户查询
	t.queryScheduler.RequestAccountQuery()
}

// handleQryTransferSerial 处理查询转账流水请求
// 对应 C++ aid == "qry_transfer_serial" 处理 (line 5268-5278)
func (t *TraderCTP) handleQryTransferSerial(connID int) {
	logger.Info("handling qry_transfer_serial request", zap.Int("conn_id", connID))

	// 对应 C++ if (nullptr == m_pTdApi) OutputNotifyAllSycn(359, u8"当前时间不支持查询转账记录!", "WARNING")
	if t.ctpApi == nil {
		t.outputNotifyAll(359, "当前时间不支持查询转账记录!", "WARNING")
		return
	}

	// 对应 C++ ReqQryTransferSerial()
	t.ctpSpi.ReqQryTransferSerial()
}

// handleQryAccountRegister 处理查询银期签约关系请求
// 对应 C++ aid == "qry_account_register" 处理 (line 5299-5320)
func (t *TraderCTP) handleQryAccountRegister(connID int) {
	logger.Info("handling qry_account_register request", zap.Int("conn_id", connID))

	// 对应 C++ if (nullptr == m_pTdApi) OutputNotifyAllSycn(361, u8"当前时间不支持查询银期签约关系!", "WARNING")
	if t.ctpApi == nil {
		t.outputNotifyAll(361, "当前时间不支持查询银期签约关系!", "WARNING")
		return
	}

	// 对应 C++ m_data.m_banks.clear(); m_banks.clear();
	t.userMu.Lock()
	if t.user != nil {
		t.user.Banks = make(map[string]*protocol.Bank)
	}
	t.userMu.Unlock()

	// 对应 C++ m_need_query_bank.store(true); m_need_query_register.store(true);
	t.queryScheduler.SetNeedQueryBank(true)
	t.queryScheduler.SetNeedQueryRegister(true)
}

// handleReqReconnectTrade 处理重连交易请求
// 对应 C++ aid == "req_reconnect_trade" 处理 (line 5156-5177)
// 仅处理来自内部的系统消息 (connID == 0)
func (t *TraderCTP) handleReqReconnectTrade(connID int) {
	logger.Info("handling req_reconnect_trade request", zap.Int("conn_id", connID))

	// 对应 C++ if (connId != 0) return;
	if connID != 0 {
		return
	}

	// 对应 C++ 中从消息解析 connIds 并添加到 m_logined_connIds
	// Go 实现中使用 connMap 管理连接，此处主要用于条件单服务通知交易服务恢复连接
	// 当前简化实现：记录日志，实际的连接恢复由 WebSocket 层管理
	logger.Info("req_reconnect_trade processed - connection recovery initiated")
}

// handleReqStartCtp 处理启动 CTP 请求
// 对应 C++ aid == "req_start_ctp" 处理 (line 5391-5406) -> OnReqStartCTP (line 5564-5624)
// 仅处理来自内部的系统消息 (connID == 0)
func (t *TraderCTP) handleReqStartCtp(connID int) {
	logger.Info("handling req_start_ctp request", zap.Int("conn_id", connID))

	// 对应 C++ if (connId != 0) return;
	if connID != 0 {
		return
	}

	// 对应 C++ OnReqStartCTP
	if t.isLoggedIn() {
		// 如果 CTP 已经登录成功，清理旧数据后重新登录
		logger.Info("req_start_ctp: already logged in, clearing old data and reconnecting")
		t.clearOldData()
	} else {
		logger.Info("req_start_ctp: not logged in, initiating login")
	}

	// 重新发起登录
	if t.reqLogin != nil {
		t.processReqLoginFull(0, t.reqLogin)
	}
}

// handleReqStopCtp 处理停止 CTP 请求
// 对应 C++ aid == "req_stop_ctp" 处理 (line 5408-5423) -> OnReqStopCTP (line 5627-5679)
// 仅处理来自内部的系统消息 (connID == 0)
func (t *TraderCTP) handleReqStopCtp(connID int) {
	logger.Info("handling req_stop_ctp request", zap.Int("conn_id", connID))

	// 对应 C++ if (connId != 0) return;
	if connID != 0 {
		return
	}

	// 对应 C++ OnReqStopCTP
	if t.isLoggedIn() {
		logger.Info("req_stop_ctp: logged in, saving file if needed and stopping")

		// 对应 C++ if (m_need_save_file.load()) SaveToFile();
		if t.queryScheduler.NeedSaveFile() {
			t.saveToFile()
		}
	} else {
		logger.Info("req_stop_ctp: not logged in, stopping CTP API")
	}

	// 停止 CTP API
	t.Stop()
}

// ==================== 条件单消息处理 ====================

// handleInsertConditionOrder 处理插入条件单请求
func (t *TraderCTP) handleInsertConditionOrder(connID int, msg string) {
	logger.Info("handling insert_condition_order request", zap.Int("conn_id", connID))

	if !t.isLoggedIn() {
		t.outputNotify(connID, 500, "请先登录", "WARNING")
		return
	}

	if t.condOrderMgr == nil {
		t.outputNotify(connID, 500, "条件单功能未启用", "WARNING")
		return
	}

	if err := t.condOrderMgr.InsertConditionOrder(msg); err != nil {
		logger.Error("insert condition order failed",
			zap.Int("conn_id", connID),
			zap.Error(err))
	}
}

// handleCancelConditionOrder 处理撤销条件单请求
func (t *TraderCTP) handleCancelConditionOrder(connID int, msg string) {
	logger.Info("handling cancel_condition_order request", zap.Int("conn_id", connID))

	if !t.isLoggedIn() {
		t.outputNotify(connID, 500, "请先登录", "WARNING")
		return
	}

	if t.condOrderMgr == nil {
		t.outputNotify(connID, 500, "条件单功能未启用", "WARNING")
		return
	}

	if err := t.condOrderMgr.CancelConditionOrder(msg); err != nil {
		logger.Error("cancel condition order failed",
			zap.Int("conn_id", connID),
			zap.Error(err))
	}
}

// handlePauseConditionOrder 处理暂停条件单请求
func (t *TraderCTP) handlePauseConditionOrder(connID int, msg string) {
	logger.Info("handling pause_condition_order request", zap.Int("conn_id", connID))

	if !t.isLoggedIn() {
		t.outputNotify(connID, 500, "请先登录", "WARNING")
		return
	}

	if t.condOrderMgr == nil {
		t.outputNotify(connID, 500, "条件单功能未启用", "WARNING")
		return
	}

	if err := t.condOrderMgr.PauseConditionOrder(msg); err != nil {
		logger.Error("pause condition order failed",
			zap.Int("conn_id", connID),
			zap.Error(err))
	}
}

// handleResumeConditionOrder 处理恢复条件单请求
func (t *TraderCTP) handleResumeConditionOrder(connID int, msg string) {
	logger.Info("handling resume_condition_order request", zap.Int("conn_id", connID))

	if !t.isLoggedIn() {
		t.outputNotify(connID, 500, "请先登录", "WARNING")
		return
	}

	if t.condOrderMgr == nil {
		t.outputNotify(connID, 500, "条件单功能未启用", "WARNING")
		return
	}

	if err := t.condOrderMgr.ResumeConditionOrder(msg); err != nil {
		logger.Error("resume condition order failed",
			zap.Int("conn_id", connID),
			zap.Error(err))
	}
}

// handleQryConditionOrder 处理查询条件单请求
func (t *TraderCTP) handleQryConditionOrder(connID int) {
	logger.Info("handling qry_condition_order request", zap.Int("conn_id", connID))

	if !t.isLoggedIn() {
		t.outputNotify(connID, 500, "请先登录", "WARNING")
		return
	}

	if t.condOrderMgr == nil {
		t.outputNotify(connID, 500, "条件单功能未启用", "WARNING")
		return
	}

	// 查询当前条件单通过 sendUserData 发送（条件单数据包含在用户数据中）
	t.sendUserData(connID)
}

// handleQryHisConditionOrder 处理查询历史条件单请求
func (t *TraderCTP) handleQryHisConditionOrder(connID int, msg string) {
	logger.Info("handling qry_his_condition_order request", zap.Int("conn_id", connID))

	if !t.isLoggedIn() {
		t.outputNotify(connID, 500, "请先登录", "WARNING")
		return
	}

	if t.condOrderMgr == nil {
		t.outputNotify(connID, 500, "条件单功能未启用", "WARNING")
		return
	}

	if _, err := t.condOrderMgr.QueryHistoryConditionOrders(msg); err != nil {
		logger.Error("query history condition orders failed",
			zap.Int("conn_id", connID),
			zap.Error(err))
	}
}
