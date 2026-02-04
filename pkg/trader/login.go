// Package trader 登录流程处理
// 对应 C++ traderctp::ProcessReqLogIn
package trader

import (
	"time"

	"github.com/pseudocodes/go2ctp/ctp"
	"github.com/pseudocodes/go2ctp/thost"
	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/config"
	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/protocol"
)

// CTPLoginStatus 登录状态
// 对应 C++ ECTPLoginStatus
type CTPLoginStatus int

const (
	LoginStatusInit                           CTPLoginStatus = 340
	LoginStatusConnecting                     CTPLoginStatus = 201
	LoginStatusConnected                      CTPLoginStatus = 202
	LoginStatusReqAuthenFail                  CTPLoginStatus = 341
	LoginStatusRspAuthenFail                  CTPLoginStatus = 342
	LoginStatusRegSystemInfoFail              CTPLoginStatus = 343 // SE15 穿透式监管终端信息注册失败 (Phase 4)
	LoginStatusReqLoginFail                   CTPLoginStatus = 344
	LoginStatusRspLoginFail                   CTPLoginStatus = 345
	LoginStatusRspLoginSuccess                CTPLoginStatus = 350
	LoginStatusReqLoginTimeOut                CTPLoginStatus = 347
	LoginStatusRspLoginFailNeedModifyPassword CTPLoginStatus = 346
)

// 登录重试配置 (Phase 1: Reconnection)
const (
	maxLoginRetryTimes   = 10 // 最大登录重试次数
	minRetryDelaySeconds = 10 // 最小重试延迟秒数
	maxRetryDelaySeconds = 60 // 最大重试延迟秒数
	loginTimeoutSeconds  = 30 // 登录超时秒数
)

// processReqLoginFull 完整的登录处理流程
// 对应 C++ traderctp::ProcessReqLogIn
func (t *TraderCTP) processReqLoginFull(connID int, req *protocol.ReqLogin) {
	logger.Info("processing login request",
		zap.Int("conn_id", connID),
		zap.String("bid", req.Bid),
		zap.String("user_name", req.UserName),
	)

	// 如果已经登录
	if t.isLoggedIn() {
		t.handleAlreadyLoggedIn(connID, req)
		return
	}

	// 保存登录请求
	t.reqLogin = req
	t.loginConnection = connID

	// 获取经纪商配置
	broker, ok := config.GetBroker(req.Bid)
	if !ok {
		logger.Warn("broker not found", zap.String("bid", req.Bid))
		t.outputNotify(connID, 1, "未找到经纪商配置: "+req.Bid, "ERROR")
		return
	}
	t.broker = &broker

	// 支持次席：如果请求中指定了 broker_id 和 front，使用请求中的值
	if req.BrokerID != "" && req.Front != "" {
		logger.Info("using custom front and broker_id",
			zap.String("front", req.Front),
			zap.String("broker_id", req.BrokerID),
		)
		t.broker.CtpBrokerID = req.BrokerID
		t.broker.TradingFronts = []string{req.Front}
	}

	// 初始化 CTP API
	t.initCtpApi()
}

// handleAlreadyLoggedIn 处理已登录情况
// 对应 C++ 中 m_b_login.load() == true 的情况
func (t *TraderCTP) handleAlreadyLoggedIn(connID int, req *protocol.ReqLogin) {
	// 检查是否重复登录
	t.connectionsMu.RLock()
	conn, exists := t.connections[connID]
	t.connectionsMu.RUnlock()

	if exists && conn.IsLoggedIn {
		t.outputNotify(connID, 338, "重复发送登录请求!", "WARNING")
		return
	}

	// 检查登录凭证是否匹配
	if t.reqLogin != nil &&
		t.reqLogin.Bid == req.Bid &&
		t.reqLogin.UserName == req.UserName &&
		t.reqLogin.Password == req.Password {

		// 登录凭证匹配，允许登录
		t.connectionsMu.Lock()
		if c, ok := t.connections[connID]; ok {
			c.IsLoggedIn = true
			c.UserName = req.UserName
			c.LoginTime = time.Now().UnixNano()
		}
		t.connectionsMu.Unlock()

		logger.Info("connection logged in with existing session",
			zap.Int("conn_id", connID),
		)

		t.outputNotify(connID, 324, "登录成功", "INFO")

		// 发送会话信息
		t.sendSessionInfo(connID)

		// 发送用户数据
		t.sendUserData(connID)

		// 重发结算单信息
		if !t.settlementConfirmed {
			t.sendSettlementInfo()
		}
	} else {
		t.outputNotify(connID, 339, "账户和密码不匹配!", "ERROR")
	}
}

// initCtpApi 初始化 CTP API
// 对应 C++ traderctp::InitTdApi
func (t *TraderCTP) initCtpApi() {
	logger.Info("initializing CTP API")

	// 创建 CTP SPI
	t.ctpSpi = NewCtpSpi(t)
	t.ctpSpi.SetLoginInfo(t.broker, t.reqLogin.UserName, t.reqLogin.Password)

	// 创建 CTP API
	flowPath := "./data/ctp_flow/"
	if config.Global != nil && config.Global.CTP.FlowPath != "" {
		flowPath = config.Global.CTP.FlowPath
	}

	api := ctp.CreateTraderApi(ctp.TraderFlowPath(flowPath))
	t.ctpSpi.SetApi(api)
	t.ctpApi = api

	// 注册 SPI
	api.RegisterSpi(t.ctpSpi)

	// 订阅私有流和公有流
	api.SubscribePrivateTopic(thost.THOST_TERT_QUICK)
	api.SubscribePublicTopic(thost.THOST_TERT_QUICK)

	// 注册前置地址
	for _, front := range t.broker.TradingFronts {
		logger.Info("registering front", zap.String("front", front))
		api.RegisterFront(front)
	}

	// 设置状态
	t.setState(StateConnecting)

	// 初始化并等待登录
	go t.waitForLogin(api)
}

// waitForLogin 等待登录完成
func (t *TraderCTP) waitForLogin(api thost.TraderApi) {
	// 初始化 API
	api.Init()

	// 等待登录完成或超时
	timeout := time.After(15 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-timeout:
			logger.Error("login timeout")
			t.outputNotifyAll(int64(LoginStatusReqLoginTimeOut), "登录超时", "ERROR")
			t.stopCtpApi(api)
			return
		case <-ticker.C:
			state := t.getState()
			if state >= StateLoggedIn {
				logger.Info("login completed successfully")
				return
			}
			if state == StateStopped || state == StateStopping {
				return
			}
		}
	}
}

// stopCtpApi 停止 CTP API
func (t *TraderCTP) stopCtpApi(api thost.TraderApi) {
	// 先设置状态，阻止新的请求
	t.setState(StateStopping)

	// 清理 ctpSpi 中的 api 引用
	if t.ctpSpi != nil {
		t.ctpSpi.SetApi(nil)
	}

	if api != nil {
		api.Release()
	}
	t.ctpApi = nil
}

// sendSessionInfo 发送会话信息
// 对应 C++ 中发送 session 信息的逻辑
func (t *TraderCTP) sendSessionInfo(connID int) {
	if t.user == nil {
		return
	}

	msg := `{"aid":"rtn_data","data":[{"trade":{"` + t.user.UserID + `":{"session":{` +
		`"user_id":"` + t.user.UserID + `",` +
		`"trading_day":"` + t.user.TradingDay + `"` +
		`}}}}]}`
	t.SendMsg(connID, msg)
}

// ==================== 重连机制 (Phase 1: Reconnection) ====================

// reinitCtp 重新初始化 CTP
// 对应 C++ traderctp::ReinitCtp
func (t *TraderCTP) reinitCtp() {
	logger.Info("reinitializing CTP connection")

	// 停止当前 CTP API
	if t.ctpApi != nil {
		if api, ok := t.ctpApi.(thost.TraderApi); ok {
			t.stopCtpApi(api)
		}
	}

	// 等待一段时间再重连
	delay := t.getRetryDelay(maxRetryDelaySeconds)
	logger.Info("waiting before reconnect", zap.Duration("delay", delay))

	select {
	case <-t.ctx.Done():
		return
	case <-time.After(delay):
	}

	// 清理旧数据并重新登录
	if t.reqLogin != nil {
		t.clearOldData()
		t.initCtpApi()
	}
}

// clearOldData 清理旧数据
// 对应 C++ traderctp::ClearOldData
// 完整清理所有状态，用于重连或重新登录场景
func (t *TraderCTP) clearOldData() {
	logger.Info("clearing old data before reconnect")

	// 1. 保存文件 (如需要)
	// 对应 C++ if (m_need_save_file.load()) SaveToFile();
	if t.queryScheduler != nil && t.queryScheduler.NeedSaveFile() {
		t.saveToFile()
	}

	t.userMu.Lock()
	defer t.userMu.Unlock()

	// 2. 重置状态
	// 对应 C++ m_b_login.store(false); _logIn_status = init;
	t.setState(StateInit)
	t.settlementInfo = ""
	t.settlementConfirmed = false

	// 3. 清空用户数据 (完整清理)
	// 对应 C++ m_data.m_accounts/banks/orders/positions/trades/transfers.clear()
	if t.user != nil {
		t.user.Positions = make(map[string]*protocol.Position)
		t.user.Accounts = make(map[string]*protocol.Account)
		t.user.Orders = make(map[string]*protocol.Order)
		t.user.Trades = make(map[string]*protocol.Trade)
		t.user.Transfers = make(map[string]*protocol.TransferLog)
		t.user.Banks = make(map[string]*protocol.Bank)
	}

	// 4. 重置查询调度器
	// 对应 C++ m_req_account_id/m_rsp_account_id 等归零
	if t.queryScheduler != nil {
		t.queryScheduler.Reset()
	}

	// 5. 重置登录计数器
	// 对应 C++ m_try_req_authenticate_times = 0; m_try_req_login_times = 0;
	t.tryReqLoginTimes = 0
	t.tryReqAuthenticateTimes = 0

	// 6. 重置持仓初始化标志
	// 对应 C++ m_position_inited.store(false);
	t.positionInited = false

	// 7. 重置经纪商参数
	// 对应 C++ m_Algorithm_Type = THOST_FTDC_AG_None;
	t.algorithmType = 0

	// 8. 重置订单通知相关数据结构
	// 对应 C++ m_insert_order_set.clear(); m_cancel_order_set.clear(); m_input_order_key_map.clear();
	t.insertOrderSetMu.Lock()
	t.insertOrderSet = make(map[string]bool)
	t.insertOrderSetMu.Unlock()

	t.cancelOrderSetMu.Lock()
	t.cancelOrderSet = make(map[string]bool)
	t.cancelOrderSetMu.Unlock()

	t.inputOrderKeyMapMu.Lock()
	t.inputOrderKeyMap = make(map[string]*ServerOrderInfo)
	t.inputOrderKeyMapMu.Unlock()

	logger.Info("old data cleared successfully")
}

// clearOldDataForNewTradingDay 交易日切换时清理旧数据
// 对应 C++ OnRspUserLogin 中检测到 m_trading_day != trading_day 时的清理逻辑
// 与 clearOldData 不同的是：不停止 CTP API，不重置登录状态，只清理动态数据
func (t *TraderCTP) clearOldDataForNewTradingDay(newTradingDay string) {
	logger.Info("clearing old data for new trading day",
		zap.String("new_trading_day", newTradingDay),
	)

	t.userMu.Lock()
	defer t.userMu.Unlock()

	// 1. 清空用户数据中的动态数据
	// 对应 C++ 中的 m_data.m_accounts/banks/orders/positions/trades/transfers.clear()
	if t.user != nil {
		oldTradingDay := t.user.TradingDay

		// 清空所有动态数据
		t.user.Positions = make(map[string]*protocol.Position)
		t.user.Accounts = make(map[string]*protocol.Account)
		t.user.Orders = make(map[string]*protocol.Order)
		t.user.Trades = make(map[string]*protocol.Trade)
		t.user.Transfers = make(map[string]*protocol.TransferLog)
		t.user.Banks = make(map[string]*protocol.Bank)

		// 更新交易日
		t.user.TradingDay = newTradingDay

		logger.Info("trading day changed, old data cleared",
			zap.String("old_day", oldTradingDay),
			zap.String("new_day", newTradingDay),
		)
	}

	// 2. 清空结算单
	t.settlementInfo = ""
	t.settlementConfirmed = false

	// 3. 重置查询调度器
	if t.queryScheduler != nil {
		t.queryScheduler.Reset()
	}

	// 4. 重置持仓初始化标志
	t.positionInited = false

	// 5. 重置经纪商参数
	t.algorithmType = 0

	// 6. 重置订单通知相关数据结构
	// 对应 C++ m_insert_order_set.clear(); m_cancel_order_set.clear(); m_input_order_key_map.clear();
	// 交易日切换后，上一交易日的待确认订单/撤单不再有效
	t.insertOrderSetMu.Lock()
	t.insertOrderSet = make(map[string]bool)
	t.insertOrderSetMu.Unlock()

	t.cancelOrderSetMu.Lock()
	t.cancelOrderSet = make(map[string]bool)
	t.cancelOrderSetMu.Unlock()

	t.inputOrderKeyMapMu.Lock()
	t.inputOrderKeyMap = make(map[string]*ServerOrderInfo)
	t.inputOrderKeyMapMu.Unlock()
}

// getRetryDelay 获取重试延迟时间
// 对应 C++ SendLoginRequest 中的延迟逻辑
func (t *TraderCTP) getRetryDelay(reqTimes int) time.Duration {
	// if t.tryReqLoginTimes <= 0 {
	// 	return 0
	// }
	if reqTimes > 0 {
		return 0
	}

	// 延迟时间随重试次数增加: 10 + n 秒, 最大 60 秒
	seconds := minRetryDelaySeconds + reqTimes
	if seconds > maxRetryDelaySeconds {
		seconds = maxRetryDelaySeconds
	}

	return time.Duration(seconds) * time.Second
}

// sendLoginRequest 发送登录请求 (带延迟重试)
// 对应 C++ traderctp::SendLoginRequest
func (t *TraderCTP) sendLoginRequest() {
	// 检查重试次数
	if t.tryReqLoginTimes >= maxLoginRetryTimes {
		logger.Error("max login retry times exceeded",
			zap.Int("max_times", maxLoginRetryTimes))
		t.outputNotifyAll(100, "登录重试次数超限", "ERROR")
		return
	}

	// 延迟重试
	delay := t.getRetryDelay(t.tryReqLoginTimes)
	if delay > 0 {
		logger.Info("waiting before login retry",
			zap.Int("retry_times", t.tryReqLoginTimes),
			zap.Duration("delay", delay),
		)

		select {
		case <-t.ctx.Done():
			return
		case <-time.After(delay):
		}
	}

	t.tryReqLoginTimes++
	logger.Info("sending login request",
		zap.Int("retry_times", t.tryReqLoginTimes),
	)

	// 发起认证请求
	t.ctpSpi.reqUserLogin()
}

func (t *TraderCTP) sendAuthRequest() {
	// 检查重试次数
	if t.tryReqAuthenticateTimes >= maxLoginRetryTimes {
		logger.Error("max login retry times exceeded",
			zap.Int("max_times", maxLoginRetryTimes))
		t.outputNotifyAll(100, "登录重试次数超限", "ERROR")
		return
	}

	// 延迟重试
	delay := t.getRetryDelay(t.tryReqAuthenticateTimes)
	if delay > 0 {
		logger.Info("waiting before login retry",
			zap.Int("retry_times", t.tryReqAuthenticateTimes),
			zap.Duration("delay", delay),
		)

		select {
		case <-t.ctx.Done():
			return
		case <-time.After(delay):
		}
	}

	t.tryReqAuthenticateTimes++
	logger.Info("sending login request",
		zap.Int("retry_times", t.tryReqAuthenticateTimes),
	)

	// 发起认证请求
	t.ctpSpi.reqAuthenticate()
}

// resetRetryCounters 重置重试计数器
// 在登录成功后调用
func (t *TraderCTP) resetRetryCounters() {
	t.tryReqLoginTimes = 0
	t.tryReqAuthenticateTimes = 0
	t.needReset = false
}

// needReconnect 检查是否需要重连
// 对应 C++ traderctp::NeedReset
func (t *TraderCTP) needReconnect() bool {
	return t.needReset
}
