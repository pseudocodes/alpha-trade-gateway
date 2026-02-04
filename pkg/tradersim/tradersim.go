// Package tradersim 模拟交易实现
// 对应 C++ open-trade-sim/tradersim.cpp
package tradersim

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/inslist"
	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/marketfeed"
	"alpha-trade-gateway/pkg/protocol"
)

// Notify 错误码常量 (与 C++ 保持一致)
const (
	NotifyCodeDuplicateLogin       = 400 // 重复发送登录请求!
	NotifyCodeLoginSuccess         = 401 // 登录成功
	NotifyCodePasswordMismatch     = 402 // 账户和密码不匹配!
	NotifyCodeLoginFailed          = 403 // 用户登录失败!
	NotifyCodePriceAboveUpperLimit = 404 // 下单,已被服务器拒绝,原因:已撤单报单被拒绝价格超出涨停板
	NotifyCodePriceBelowLowerLimit = 405 // 下单,已被服务器拒绝,原因:已撤单报单被拒绝价格跌破跌停板
	NotifyCodeTradeNotify          = 406 // 成交通知
	NotifyCodeDuplicateOrderID     = 407 // 下单, 已被服务器拒绝,原因:单号重复
	NotifyCodeInvalidUserName      = 408 // 下单, 已被服务器拒绝, 原因:下单指令中的用户名错误
	NotifyCodeInvalidInstrument    = 409 // 下单, 已被服务器拒绝, 原因:合约不合法
	NotifyCodeOnlyFutures          = 410 // 下单, 已被服务器拒绝, 原因:模拟交易只支持期货合约
	NotifyCodeInvalidVolume        = 411 // 下单, 已被服务器拒绝, 原因:下单手数应该大于0
	NotifyCodeInvalidPriceTick     = 412 // 下单,已被服务器拒绝, 原因:下单价格不是价格单位的整倍数
	NotifyCodeInsufficientMargin   = 413 // 下单,已被服务器拒绝,原因:开仓保证金不足
	NotifyCodeCloseTodayExceed     = 414 // 下单,已被服务器拒绝,原因:平今手数超过今仓持仓量
	NotifyCodeCloseYesterdayExceed = 415 // 下单,已被服务器拒绝,原因:平昨手数超过昨仓持仓量
	NotifyCodeCloseExceed          = 416 // 下单,已被服务器拒绝,原因:平仓手数超过持仓量
	NotifyCodeOrderSuccess         = 417 // 下单成功
	NotifyCodeCancelInvalidUser    = 418 // 撤单,已被服务器拒绝,原因:撤单指令中的用户名错误
	NotifyCodeCancelSuccess        = 419 // 撤单成功
	NotifyCodeOrderNotExist        = 420 // 要撤销的单不存在
	NotifyCodeTransferSuccess      = 421 // 转账成功
	NotifyCodeTransferFailed       = 422 // 转账失败
)

// Notify Level 常量
const (
	NotifyLevelInfo    = "INFO"
	NotifyLevelWarning = "WARNING"
	NotifyLevelError   = "ERROR"
)

// Notify Type 常量
const (
	NotifyTypeMessage = "MESSAGE"
)

// TraderSim 模拟交易器
// 对应 C++ tradersim 类
type TraderSim struct {
	ctx    context.Context
	cancel context.CancelFunc

	// 状态管理
	isLoggedIn atomic.Bool
	reqLogin   *protocol.ReqLogin
	sessionID  int64 // 会话ID (对应 C++ m_session_id)

	// 消息发送回调
	msgSender func(connID int, msg string)

	// 连接管理
	connections   map[int]*ConnInfo
	connectionsMu sync.RWMutex

	// 用户数据 (复用 protocol.User)
	user   *protocol.User
	userMu sync.RWMutex

	// 活跃订单集合 (用于撮合)
	aliveOrders   map[string]*protocol.Order
	aliveOrdersMu sync.RWMutex

	// 合约信息服务 (公共模块)
	insService *inslist.InstrumentService

	// 行情客户端
	marketClient marketfeed.MarketClient

	// 数据变更标志
	somethingChanged atomic.Bool
	orderSeq         atomic.Int64
	tradeSeq         atomic.Int64

	// 配置
	userFilePath string
	brokerID     string

	// 定时器
	idleTicker *time.Ticker
}

// ConnInfo 连接信息
type ConnInfo struct {
	ConnID     int
	IsLoggedIn bool
	UserName   string
	LoginTime  int64
}

// Config 模拟交易配置
type Config struct {
	UserFilePath string // 用户数据文件路径
	BrokerID     string // 经纪商 ID
	InsListURL   string // 合约列表 URL (可选)
}

// New 创建新的 TraderSim 实例
func New(ctx context.Context, cfg *Config) *TraderSim {
	ctx, cancel := context.WithCancel(ctx)
	t := &TraderSim{
		ctx:          ctx,
		cancel:       cancel,
		connections:  make(map[int]*ConnInfo),
		aliveOrders:  make(map[string]*protocol.Order),
		userFilePath: cfg.UserFilePath,
		brokerID:     cfg.BrokerID,
	}
	return t
}

// SetMsgSender 设置消息发送回调
func (t *TraderSim) SetMsgSender(sender func(connID int, msg string)) {
	t.msgSender = sender
}

// SetMarketClient 设置行情客户端
func (t *TraderSim) SetMarketClient(client marketfeed.MarketClient) {
	t.marketClient = client
}

// SetInstrumentService 设置合约信息服务
func (t *TraderSim) SetInstrumentService(svc *inslist.InstrumentService) {
	t.insService = svc
}

// Start 启动模拟交易器
func (t *TraderSim) Start() error {
	// 初始化合约信息服务
	if t.insService != nil {
		if err := t.insService.Init(); err != nil {
			logger.Warn("init instrument service failed, will use default values",
				zap.Error(err))
		} else {
			logger.Info("instrument service initialized",
				zap.Int("count", t.insService.Count()))
		}
	}

	logger.Info("tradersim started")
	return nil
}

// Stop 停止模拟交易器
func (t *TraderSim) Stop() {
	t.cancel()

	// 停止定时器
	if t.idleTicker != nil {
		t.idleTicker.Stop()
	}

	// 保存用户数据
	if t.user != nil {
		if err := t.saveUserDataFile(); err != nil {
			logger.Error("save user data failed", zap.Error(err))
		}
	}

	logger.Info("tradersim stopped")
}

// OnConnect 新连接建立
func (t *TraderSim) OnConnect(connID int) {
	t.connectionsMu.Lock()
	t.connections[connID] = &ConnInfo{
		ConnID: connID,
	}
	t.connectionsMu.Unlock()

	logger.Info("sim: new connection", zap.Int("conn_id", connID))
}

// OnDisconnect 连接断开
func (t *TraderSim) OnDisconnect(connID int) {
	t.connectionsMu.Lock()
	delete(t.connections, connID)
	t.connectionsMu.Unlock()

	logger.Info("sim: connection closed", zap.Int("conn_id", connID))
}

// OnMessage 收到消息
func (t *TraderSim) OnMessage(connID int, msg string) {
	t.processMessage(connID, msg)
}

// processMessage 处理消息
func (t *TraderSim) processMessage(connID int, msg string) {
	aid := gjson.Get(msg, "aid").String()

	switch aid {
	case "req_login":
		t.handleLogin(connID, msg)
	case "peek_message":
		t.handlePeekMessage(connID)
	case "insert_order":
		t.handleInsertOrder(connID, msg)
	case "cancel_order":
		t.handleCancelOrder(connID, msg)
	case "req_transfer":
		t.handleTransfer(connID, msg)
	default:
		logger.Debug("sim: unknown message", zap.String("aid", aid))
	}
}

// handleLogin 处理登录请求
func (t *TraderSim) handleLogin(connID int, msg string) {
	// 检查是否重复登录
	if t.isLoggedIn.Load() {
		logger.Info("tradersim ProcessReqLogIn",
			zap.String("fun", "handleLogin"),
			zap.String("key", t.brokerID),
			zap.Int("connId", connID),
			zap.String("msg", "duplicate login request"))
		t.outputNotify(connID, NotifyCodeDuplicateLogin, "重复发送登录请求!", NotifyLevelWarning)
		return
	}

	// 解析登录请求
	t.reqLogin = &protocol.ReqLogin{
		Bid:      gjson.Get(msg, "bid").String(),
		UserName: gjson.Get(msg, "user_name").String(),
		Password: gjson.Get(msg, "password").String(),
	}

	logger.Info("tradersim ProcessReqLogIn",
		zap.String("fun", "handleLogin"),
		zap.String("key", t.brokerID),
		zap.String("bid", t.reqLogin.Bid),
		zap.String("user_name", t.reqLogin.UserName),
		zap.Int("connId", connID))

	// 生成会话ID
	t.sessionID = time.Now().UnixNano()

	// 更新连接信息
	t.connectionsMu.Lock()
	if conn, ok := t.connections[connID]; ok {
		conn.IsLoggedIn = true
		conn.UserName = t.reqLogin.UserName
		conn.LoginTime = t.sessionID
	}
	t.connectionsMu.Unlock()

	// 加载用户数据
	if err := t.loadUserDataFile(); err != nil {
		logger.Error("sim: load user data failed",
			zap.String("fun", "handleLogin"),
			zap.String("key", t.brokerID),
			zap.String("user_name", t.reqLogin.UserName),
			zap.Error(err))
		// 初始化新用户
		t.initNewUser()
	}

	t.isLoggedIn.Store(true)

	logger.Info("trade sim login success",
		zap.String("fun", "handleLogin"),
		zap.String("key", t.brokerID),
		zap.String("bid", t.reqLogin.Bid),
		zap.String("user_name", t.reqLogin.UserName),
		zap.Int("connId", connID),
		zap.Int("loginstatus", 0))

	// 订阅持仓合约行情
	t.subscribePositionSymbols()

	// 启动 OnIdle 循环
	t.startIdleLoop()

	// 发送登录成功通知
	t.outputNotify(connID, NotifyCodeLoginSuccess, "登录成功", NotifyLevelInfo)

	// 发送用户数据
	t.sendUserData(connID)
}

// handlePeekMessage 处理 peek_message 请求
func (t *TraderSim) handlePeekMessage(connID int) {
	if !t.isLoggedIn.Load() {
		return
	}
	// 发送用户数据
	t.sendUserData(connID)
}

// handleTransfer 处理转账请求
func (t *TraderSim) handleTransfer(connID int, msg string) {
	if !t.isLoggedIn.Load() {
		t.outputNotify(connID, NotifyCodeTransferFailed, "转账失败", NotifyLevelWarning)
		return
	}

	currency := gjson.Get(msg, "currency").String()
	if currency == "" {
		currency = "CNY"
	}
	amount := gjson.Get(msg, "amount").Float()

	t.userMu.Lock()
	defer t.userMu.Unlock()

	account := t.getAccountNoLock()
	if account == nil {
		t.outputNotify(connID, NotifyCodeTransferFailed, "转账失败", NotifyLevelWarning)
		return
	}

	if amount > 0 {
		// 入金
		account.Deposit += amount
		account.Balance += amount
		account.Available += amount
		account.StaticBalance += amount
	} else {
		// 出金
		if account.Available < -amount {
			logger.Info("转账失败",
				zap.String("fun", "handleTransfer"),
				zap.String("key", t.brokerID),
				zap.String("user_name", t.reqLogin.UserName),
				zap.String("currency", currency),
				zap.Float64("amount", amount))
			t.outputNotify(connID, NotifyCodeTransferFailed, "转账失败", NotifyLevelWarning)
			return
		}
		account.Withdraw += -amount
		account.Balance += amount
		account.Available += amount
		account.StaticBalance += amount
	}
	account.Changed = true

	// 记录转账日志
	transferID := fmt.Sprintf("T%d", time.Now().UnixNano())
	t.user.Transfers[transferID] = &protocol.TransferLog{
		DateTime: time.Now().Format("2006-01-02 15:04:05"),
		Currency: currency,
		Amount:   amount,
		ErrorID:  0,
		ErrorMsg: "",
		Changed:  true,
	}

	t.somethingChanged.Store(true)

	// 保存用户数据
	if err := t.saveUserDataFile(); err != nil {
		logger.Error("sim: save user data failed",
			zap.String("fun", "handleTransfer"),
			zap.String("key", t.brokerID),
			zap.Error(err))
	}

	logger.Info("转账成功",
		zap.String("fun", "handleTransfer"),
		zap.String("key", t.brokerID),
		zap.String("user_name", t.reqLogin.UserName),
		zap.String("currency", currency),
		zap.Float64("amount", amount))
	t.outputNotifyAll(NotifyCodeTransferSuccess, "转账成功", NotifyLevelInfo)
}

// subscribePositionSymbols 订阅持仓合约行情
func (t *TraderSim) subscribePositionSymbols() {
	if t.marketClient == nil {
		return
	}

	t.userMu.RLock()
	defer t.userMu.RUnlock()

	if t.user == nil || len(t.user.Positions) == 0 {
		return
	}

	var symbols []string
	for key := range t.user.Positions {
		symbols = append(symbols, key)
	}

	if len(symbols) > 0 {
		logger.Info("sim: subscribing position symbols",
			zap.Int("count", len(symbols)))
		if err := t.marketClient.Subscribe(symbols...); err != nil {
			logger.Error("sim: subscribe failed", zap.Error(err))
		}
	}
}

// startIdleLoop 启动 OnIdle 循环
func (t *TraderSim) startIdleLoop() {
	t.idleTicker = time.NewTicker(100 * time.Millisecond)

	go func() {
		for {
			select {
			case <-t.ctx.Done():
				return
			case <-t.idleTicker.C:
				t.onIdle()
			}
		}
	}()

	logger.Info("sim: idle loop started")
}

// onIdle 空闲处理
// 对应 C++ OnIdle
func (t *TraderSim) onIdle() {
	if !t.isLoggedIn.Load() {
		return
	}

	// 1. 尝试撮合
	t.tryOrderMatch()

	// 2. 重算持仓盈亏
	t.recalculatePositionAndFloatProfit()

	// 3. 定时推送数据
	if t.somethingChanged.Load() {
		t.sendUserDataAll()
		t.somethingChanged.Store(false)
	}
}

// SendMsg 发送消息给指定连接
func (t *TraderSim) SendMsg(connID int, msg string) {
	if t.msgSender != nil {
		t.msgSender(connID, msg)
	}
}

// SendMsgAll 发送消息给所有连接
func (t *TraderSim) SendMsgAll(msg string) {
	t.connectionsMu.RLock()
	defer t.connectionsMu.RUnlock()

	for connID := range t.connections {
		t.SendMsg(connID, msg)
	}
}

// outputNotify 发送通知消息
// 对应 C++ OutputNotifySycn
func (t *TraderSim) outputNotify(connID int, code int64, content, level string) {
	notifyKey := generateGUID()
	msg := fmt.Sprintf(`{"aid":"rtn_data","data":[{"notify":{"%s":{"type":"%s","level":"%s","code":%d,"session_id":%d,"content":"%s"}}}]}`,
		notifyKey, NotifyTypeMessage, level, code, t.sessionID, content)
	t.SendMsg(connID, msg)
}

// outputNotifyAll 发送通知消息给所有连接
// 对应 C++ OutputNotifyAllSycn
func (t *TraderSim) outputNotifyAll(code int64, content, level string) {
	t.connectionsMu.RLock()
	defer t.connectionsMu.RUnlock()

	notifyKey := generateGUID()
	msg := fmt.Sprintf(`{"aid":"rtn_data","data":[{"notify":{"%s":{"type":"%s","level":"%s","code":%d,"session_id":%d,"content":"%s"}}}]}`,
		notifyKey, NotifyTypeMessage, level, code, t.sessionID, content)

	for connID := range t.connections {
		t.SendMsg(connID, msg)
	}
}

// generateGUID 生成 GUID
func generateGUID() string {
	return uuid.New().String()
}

// getInstrument 获取合约信息
func (t *TraderSim) getInstrument(symbol string) *inslist.InstrumentInfo {
	if t.insService == nil {
		return nil
	}
	return t.insService.GetInstrument(symbol)
}

// getAccountNoLock 获取账户 (不加锁，调用者需持有锁)
func (t *TraderSim) getAccountNoLock() *protocol.Account {
	if t.user == nil {
		return nil
	}
	if acc, ok := t.user.Accounts["CNY"]; ok {
		return acc
	}
	return nil
}

// getOrCreatePositionNoLock 获取或创建持仓 (不加锁)
func (t *TraderSim) getOrCreatePositionNoLock(symbol, exchangeID, instrumentID string) *protocol.Position {
	if t.user == nil {
		return nil
	}

	pos, ok := t.user.Positions[symbol]
	if !ok {
		pos = protocol.NewPosition()
		pos.UserID = t.user.UserID
		pos.ExchangeID = exchangeID
		pos.InstrumentID = instrumentID
		t.user.Positions[symbol] = pos
	}
	return pos
}

// getTradingDay 获取交易日
func (t *TraderSim) getTradingDay() string {
	now := time.Now()
	hour := now.Hour()

	// 简单规则：18:00 后算下一个交易日
	if hour >= 18 {
		now = now.AddDate(0, 0, 1)
	}

	// 跳过周末
	for now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		now = now.AddDate(0, 0, 1)
	}

	return now.Format("20060102")
}
