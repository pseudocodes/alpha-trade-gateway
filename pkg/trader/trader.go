// Package trader CTP 交易核心逻辑
// 对应 C++ open-trade-ctpse15/tradectp.h/cpp
package trader

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/condorder"
	"alpha-trade-gateway/pkg/config"
	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/marketfeed"
	"alpha-trade-gateway/pkg/protocol"
)

// State 交易状态
type State int

const (
	StateInit State = iota
	StateConnecting
	StateConnected
	StateAuthenticating
	StateAuthenticated
	StateLoggingIn
	StateLoggedIn
	StateSettlementQuerying
	StateSettlementConfirming
	StateReady
	StateStopping
	StateStopped
)

// TraderCTP CTP 交易器
// 对应 C++ traderctp 类
type TraderCTP struct {
	ctx    context.Context
	cancel context.CancelFunc

	// 状态
	state     State
	stateMu   sync.RWMutex
	stateOnce sync.Once

	peekMessage atomic.Bool

	// 消息发送回调
	msgSender func(connID int, msg string)

	// 连接管理
	// 对应 C++ m_connections
	connections     map[int]*ConnInfo
	connectionsMu   sync.RWMutex
	loginConnection int // 第一个登录的连接

	// 用户数据
	// 对应 C++ m_data
	user   *protocol.User
	userMu sync.RWMutex

	// 登录信息
	reqLogin *protocol.ReqLogin
	broker   *config.BrokerConfig

	// 结算单
	settlementInfo      string
	settlementConfirmed bool

	// CTP API
	ctpSpi *CtpSpi
	ctpApi interface{} // thost.TraderApi

	// 合约信息 map
	// 对应 C++ GetInstrument / InstrumentMap
	// key: exchangeID.instrumentID, value: Instrument
	instrumentMap   map[string]*protocol.Instrument
	instrumentMapMu sync.RWMutex
	instrumentReady bool // 合约信息是否已加载完成

	// 行情数据 (使用 marketfeed)
	// 对应 C++ InstrumentMap
	marketClient marketfeed.MarketClient

	// 定时器
	positionUpdateTicker *time.Ticker
	idleTicker           *time.Ticker

	// 查询调度器 (OnIdle 机制)
	// 对应 C++ OnIdle 中的查询调度
	queryScheduler *QueryScheduler

	// 登录重试机制 (Phase 1: Reconnection)
	// 对应 C++ m_try_req_login_times, m_try_req_authenticate_times
	tryReqLoginTimes        int  // 登录重试次数
	tryReqAuthenticateTimes int  // 认证重试次数
	needReset               bool // 是否需要重置 CTP

	// 订单 Key 映射 (Phase 2: Persistence)
	// 对应 C++ m_ordermap_local_remote, m_ordermap_remote_local
	orderKeyManager *OrderKeyManager

	// 经纪商交易参数 (Phase 5: Query Features)
	// 对应 C++ m_Algorithm_Type
	algorithmType byte // thost.TThostFtdcAlgorithmType

	// 持仓初始化标志 (Phase 6: Position Adjustment)
	// 对应 C++ m_position_inited
	positionInited bool

	// 订单通知相关 (Phase 7: Order/Trade Notification)
	// 对应 C++ m_insert_order_set - 跟踪待确认的下单（key: OrderRef）
	insertOrderSet   map[string]bool
	insertOrderSetMu sync.RWMutex

	// 对应 C++ m_cancel_order_set - 跟踪待确认的撤单（key: order_id）
	cancelOrderSet   map[string]bool
	cancelOrderSetMu sync.RWMutex

	// 对应 C++ m_input_order_key_map - 跟踪订单的服务器端信息
	inputOrderKeyMap   map[string]*ServerOrderInfo
	inputOrderKeyMapMu sync.RWMutex

	// 条件单管理器
	condOrderMgr condorder.Manager
}

// ServerOrderInfo 服务器端订单信息
// 对应 C++ ServerOrderInfo 结构体
type ServerOrderInfo struct {
	ExchangeID   string
	InstrumentID string
	OrderRef     string
	OrderLocalID string
	OrderSysID   string
	VolumeLeft   int
	Direction    byte // THOST_FTDC_D_Buy / THOST_FTDC_D_Sell
	OffsetFlag   byte // THOST_FTDC_OF_xxx
	PriceType    byte // THOST_FTDC_OPT_xxx
	LimitPrice   float64
	VolumeTotal  int
}

// ConnInfo 连接信息
type ConnInfo struct {
	ConnID     int
	IsLoggedIn bool
	UserName   string
	LoginTime  int64
	MessageSeq int
}

// New 创建新的 TraderCTP 实例
// 对应 C++ traderctp 构造函数
func New(ctx context.Context) *TraderCTP {
	ctx, cancel := context.WithCancel(ctx)
	t := &TraderCTP{
		ctx:              ctx,
		cancel:           cancel,
		state:            StateInit,
		connections:      make(map[int]*ConnInfo),
		instrumentMap:    make(map[string]*protocol.Instrument),
		orderKeyManager:  NewOrderKeyManager(), // Phase 2: Persistence
		queryScheduler:   NewQueryScheduler(),  // OnIdle 查询调度
		insertOrderSet:   make(map[string]bool),
		cancelOrderSet:   make(map[string]bool),
		inputOrderKeyMap: make(map[string]*ServerOrderInfo),
	}
	return t
}

// SetMsgSender 设置消息发送回调
func (t *TraderCTP) SetMsgSender(sender func(connID int, msg string)) {
	t.msgSender = sender
}

// SetMarketClient 设置行情客户端
// MarketFeed 在 main.go 中作为独立服务启动，然后注入到 TraderCTP
func (t *TraderCTP) SetMarketClient(client marketfeed.MarketClient) {
	t.marketClient = client
}

// Stop 停止交易器
// 对应 C++ traderctp::Stop
func (t *TraderCTP) Stop() {
	t.stateOnce.Do(func() {
		t.setState(StateStopping)
		t.cancel()
		logger.Info("trader stopping")

		// 保存订单 Key 映射 (Phase 2: Persistence)
		t.saveToFile()

		// 停止持仓更新定时器
		if t.positionUpdateTicker != nil {
			t.positionUpdateTicker.Stop()
		}

		// 注意: marketClient 由 main.go 管理生命周期，这里不关闭

		// TODO: 断开 CTP 连接
		t.setState(StateStopped)
		logger.Info("trader stopped")
	})
}

// OnConnect 新连接建立
// 对应 C++ traderctp::ProcessNewConnection
func (t *TraderCTP) OnConnect(connID int) {
	t.connectionsMu.Lock()
	t.connections[connID] = &ConnInfo{
		ConnID: connID,
	}
	t.connectionsMu.Unlock()

	logger.Info("new connection", zap.Int("conn_id", connID))
}

// OnDisconnect 连接断开
// 对应 C++ traderctp::CloseConnection
func (t *TraderCTP) OnDisconnect(connID int) {
	t.connectionsMu.Lock()
	delete(t.connections, connID)
	t.connectionsMu.Unlock()

	logger.Info("connection closed", zap.Int("conn_id", connID))

	// 如果是登录连接断开，需要处理
	// if connID == t.loginConnection {
	// 	logger.Info("login connection closed, will stop CTP")
	// 	// TODO: 停止 CTP
	// }
}

// OnMessage 收到消息
// 对应 C++ traderctp::ProcessInMsg
func (t *TraderCTP) OnMessage(connID int, msg string) {
	logger.Debug("processing message",
		zap.Int("conn_id", connID),
		zap.String("msg_preview", truncateMsg(msg, 100)),
	)

	// 阶段 2 实现消息解析和处理
	t.processMessage(connID, msg)
}

// processMessage 在 message.go 中实现

// SendMsg 发送消息给指定连接
// 对应 C++ traderctp::SendMsg
func (t *TraderCTP) SendMsg(connID int, msg string) {
	if t.msgSender != nil {
		t.msgSender(connID, msg)
	}
}

// SendMsgAll 发送消息给所有连接
// 对应 C++ traderctp::SendMsgAll
func (t *TraderCTP) SendMsgAll(msg string) {
	t.connectionsMu.RLock()
	defer t.connectionsMu.RUnlock()

	for connID := range t.connections {
		t.SendMsg(connID, msg)
	}
}

// setState 设置状态
func (t *TraderCTP) setState(state State) {
	t.stateMu.Lock()
	t.state = state
	t.stateMu.Unlock()
}

// getState 获取状态
func (t *TraderCTP) getState() State {
	t.stateMu.RLock()
	defer t.stateMu.RUnlock()
	return t.state
}

// initUserData 初始化用户数据
// 对应 C++ traderctp 中的用户数据初始化
func (t *TraderCTP) initUserData(userID, tradingDay string) {
	t.userMu.Lock()
	defer t.userMu.Unlock()

	t.user = protocol.NewUser(userID)
	t.user.TradingDay = tradingDay

	logger.Info("user data initialized",
		zap.String("user_id", userID),
		zap.String("trading_day", tradingDay),
	)
}

// appendSettlementInfo 追加结算单内容
func (t *TraderCTP) appendSettlementInfo(content string) {
	t.settlementInfo += content
}

// onSettlementInfoComplete 结算单查询完成
func (t *TraderCTP) onSettlementInfoComplete() {
	logger.Info("settlement info complete",
		zap.Int("length", len(t.settlementInfo)),
	)

	// 如果配置了自动确认结算单，则自动确认
	if config.Global != nil && config.Global.AutoConfirmSettlement {
		t.ctpSpi.reqSettlementInfoConfirm()
	} else {
		// 等待用户确认
		t.sendSettlementInfo()
	}
}

// sendSettlementInfo 发送结算单给客户端
// 对应 C++ OutputNotifyAllSycn(325, m_settlement_info, "INFO", "SETTLEMENT")
func (t *TraderCTP) sendSettlementInfo() {
	// 构建结算单通知消息 (使用 SETTLEMENT 类型)
	msg := protocol.BuildSettlementNotifyMsg(325, t.settlementInfo)
	t.SendMsgAll(msg)
}

// onSettlementConfirmed 结算单确认完成
func (t *TraderCTP) onSettlementConfirmed() {
	t.settlementConfirmed = true

	// 加载订单 Key 映射 (Phase 2: Persistence)
	t.loadFromFile()

	// 重置登录重试计数器 (Phase 1: Reconnection)
	t.resetRetryCounters()

	// 1. 首先查询全量合约信息 (用于持仓盈亏计算)
	// 对应 C++ 中 GetInstrument 从 InstrumentMap 获取合约信息
	t.startQueryInstruments()

	// 2. 初始化行情回调 (MarketFeed 已在 main.go 中启动)
	t.initMarketFeedCallback()

	// 3. 初始化条件单管理器
	t.initConditionOrderManager()

}

// startQueryInstruments 查询全量合约信息
// 对应 C++ GenInstrumentExchangeIdMap
func (t *TraderCTP) startQueryInstruments() {
	logger.Info("starting instrument query")
	t.ctpSpi.reqQryInstrument()
}

// GetInstrument 获取合约信息
// 对应 C++ GetInstrument(symbol)
func (t *TraderCTP) GetInstrument(instrumentID string) *protocol.Instrument {
	t.instrumentMapMu.RLock()
	defer t.instrumentMapMu.RUnlock()

	if ins, ok := t.instrumentMap[instrumentID]; ok {
		return ins
	}
	return nil
}

// setInstrument 设置合约信息
func (t *TraderCTP) setInstrument(instrumentID string, ins *protocol.Instrument) {
	t.instrumentMapMu.Lock()
	defer t.instrumentMapMu.Unlock()

	t.instrumentMap[instrumentID] = ins
}

// onInstrumentQueryComplete 合约查询完成
func (t *TraderCTP) onInstrumentQueryComplete() {
	t.instrumentMapMu.Lock()
	t.instrumentReady = true
	count := len(t.instrumentMap)
	t.instrumentMapMu.Unlock()

	logger.Info("instrument query completed", zap.Int("count", count))

	// 为所有持仓绑定合约信息(not necessary)
	// t.bindPositionInstruments()

	//  然后查询账户和持仓
	t.startQueryData()
}

// bindPositionInstruments 为持仓绑定合约信息
func (t *TraderCTP) bindPositionInstruments() {
	t.userMu.Lock()
	defer t.userMu.Unlock()

	if t.user == nil {
		return
	}

	for symbol, pos := range t.user.Positions {
		if pos.Ins == nil {
			pos.Ins = t.GetInstrument(symbol)
			if pos.Ins == nil {
				logger.Warn("instrument not found for position",
					zap.String("symbol", symbol),
				)
			}
		}
	}
}

// startQueryData 开始查询数据
func (t *TraderCTP) startQueryData() {
	t.startQueryDataFull()
}

// initMarketFeedCallback 初始化行情回调
// MarketFeed 已在 main.go 中作为独立服务启动，这里只需设置回调并订阅合约
// 对应 C++ 中使用 InstrumentMap 获取行情的功能
func (t *TraderCTP) initMarketFeedCallback() {
	if t.marketClient == nil {
		logger.Warn("marketClient is nil, skip init callback")
		return
	}

	// 设置行情回调
	t.marketClient.SetOnQuotes(t.onQuotesUpdate)

	logger.Info("marketfeed callback initialized")

	// 订阅持仓合约
	t.subscribePositionSymbols()

	// 启动定时更新持仓盈亏
	// t.startPositionUpdateLoop()

	// 启动 OnIdle 循环 (查询调度)
	t.startIdleLoop()

	// 启动条件单检测
	t.startConditionOrderChecker()
}

// subscribePositionSymbols 订阅持仓合约行情
// Position 的 key 格式为 {exchangeID}.{instrumentID}，与 marketfeed 要求的格式一致
func (t *TraderCTP) subscribePositionSymbols() {
	if t.marketClient == nil {
		return
	}

	t.userMu.RLock()
	defer t.userMu.RUnlock()

	if t.user == nil || len(t.user.Positions) == 0 {
		return
	}

	// Position key 格式: SHFE.au2406, DCE.m2409 等
	var symbols []string
	for key := range t.user.Positions {
		symbols = append(symbols, key)
	}

	if len(symbols) > 0 {
		logger.Info("subscribing position symbols", zap.Int("count", len(symbols)), zap.Strings("symbols", symbols))
		if err := t.marketClient.Subscribe(symbols...); err != nil {
			logger.Error("subscribe position symbols failed", zap.Error(err))
		}
	}
}

// subscribeSymbolIfNeeded 订阅单个合约行情 (如果尚未订阅)
// 对应 C++ 中新持仓时需要从 InstrumentMap 获取行情
// symbol 格式要求: {exchangeID}.{instrumentID}，例如 SHFE.au2406
func (t *TraderCTP) subscribeSymbolIfNeeded(symbol string) {
	if t.marketClient == nil || symbol == "" {
		return
	}

	// 检查是否已经有该合约的行情
	if t.marketClient.GetQuote(symbol) != nil {
		return
	}

	logger.Debug("subscribing new symbol for position profit", zap.String("symbol", symbol))
	if err := t.marketClient.Subscribe(symbol); err != nil {
		logger.Error("subscribe symbol failed", zap.String("symbol", symbol), zap.Error(err))
	}
}

// onQuotesUpdate 行情更新回调
// 注意：此回调仅用于触发数据变化标记，不再进行盈亏计算
// 盈亏计算统一在 recalculatePositionAndAccountProfit() 中完成
// 对应 C++ 中 InstrumentMap 数据更新时的处理
func (t *TraderCTP) onQuotesUpdate(quotes []*marketfeed.Quote) {
	// 行情回调不再进行盈亏计算
	// 盈亏计算统一在 sendAllUserData() 调用时通过 recalculatePositionAndAccountProfit() 完成
	// 这样可以避免高频行情下的频繁计算
	for _, quote := range quotes {
		logger.Debug("quote received", zap.String("instrument_id", quote.InstrumentID), zap.String("exchange_id", quote.ExchangeID))
	}
}

// updatePositionProfitWithQuote 使用行情数据更新单个持仓盈亏 (内部函数，不加锁)
// 对应 C++ tradectp.cpp 中的持仓盈亏计算逻辑 (4048-4320行)
// 返回值: 是否有变化
func (t *TraderCTP) updatePositionProfitWithQuote(pos *protocol.Position, quote *marketfeed.Quote) bool {
	if pos == nil || quote == nil {
		return false
	}

	// 如果 pos.Ins 为空，尝试从 instrumentMap 获取
	// 对应 C++ ps.ins = GetInstrument(symbol)
	if pos.Ins == nil {
		pos.Ins = t.GetInstrument(pos.InstrumentID)
		if pos.Ins == nil {
			return false
		}
	}

	// 获取最新价
	lastPrice := quote.LastPrice
	if math.IsNaN(lastPrice) || lastPrice <= 0 {
		// 开盘前使用昨收或昨结算价
		lastPrice = quote.PreClose
		if math.IsNaN(lastPrice) || lastPrice <= 0 {
			lastPrice = quote.PreSettlement
		}
	}

	if math.IsNaN(lastPrice) || lastPrice <= 0 {
		return false
	}

	// 判断市场状态
	// 对应 C++ tradectp.cpp:4051-4308 中的市场状态判断
	// 0: 开盘前 - lastPrice 和 settlement 都无效
	// 1: 交易中 - lastPrice 有效，settlement 无效
	// 2: 收盘后 - lastPrice 和 settlement 都有效
	lastPriceValid := !math.IsNaN(quote.LastPrice) && quote.LastPrice > 0
	settlementValid := !math.IsNaN(quote.Settlement) && quote.Settlement > 0
	var newMarketStatus int
	if !lastPriceValid && !settlementValid {
		newMarketStatus = 0 // 开盘前
	} else if lastPriceValid && !settlementValid {
		newMarketStatus = 1 // 交易中
	} else {
		newMarketStatus = 2 // 收盘后
	}
	if pos.MarketStatus != newMarketStatus {
		pos.MarketStatus = newMarketStatus
		pos.Changed = true
	}

	// 获取合约乘数 (从 Instrument 或使用默认值)
	volumeMultiple := 10.0 // 默认值
	if pos.Ins != nil {
		volumeMultiple = float64(pos.Ins.VolumeMultiple)
	}
	if volumeMultiple <= 0 {
		volumeMultiple = 10.0
	}

	// 检查价格是否变化
	if math.Abs(lastPrice-pos.LastPrice) < 1e-9 {
		return false
	}

	pos.LastPrice = lastPrice

	// 计算浮动盈亏 (保留3位小数)
	// float_profit_long = last_price * volume_long * volume_multiple - open_cost_long
	pos.FloatProfitLong = round3(lastPrice*float64(pos.VolumeLong)*volumeMultiple - pos.OpenCostLong)
	// float_profit_short = open_cost_short - last_price * volume_short * volume_multiple
	pos.FloatProfitShort = round3(pos.OpenCostShort - lastPrice*float64(pos.VolumeShort)*volumeMultiple)
	pos.FloatProfit = round3(pos.FloatProfitLong + pos.FloatProfitShort)

	// 计算持仓盈亏 (保留3位小数)
	// 期权合约的持仓盈亏设为0，与C++一致
	isOption := pos.Ins != nil && pos.Ins.ProductClass == protocol.ProductClassOptions
	if isOption {
		pos.PositionProfitLong = 0
		pos.PositionProfitShort = 0
		pos.PositionProfit = 0
	} else {
		// position_profit_long = last_price * volume_long * volume_multiple - position_cost_long
		pos.PositionProfitLong = round3(lastPrice*float64(pos.VolumeLong)*volumeMultiple - pos.PositionCostLong)
		// position_profit_short = position_cost_short - last_price * volume_short * volume_multiple
		pos.PositionProfitShort = round3(pos.PositionCostShort - lastPrice*float64(pos.VolumeShort)*volumeMultiple)
		pos.PositionProfit = round3(pos.PositionProfitLong + pos.PositionProfitShort)
	}

	// 计算开仓均价和持仓均价 (保留3位小数)
	if pos.VolumeLong > 0 {
		pos.OpenPriceLong = round3(pos.OpenCostLong / (float64(pos.VolumeLong) * volumeMultiple))
		pos.PositionPriceLong = round3(pos.PositionCostLong / (float64(pos.VolumeLong) * volumeMultiple))
	}
	if pos.VolumeShort > 0 {
		pos.OpenPriceShort = round3(pos.OpenCostShort / (float64(pos.VolumeShort) * volumeMultiple))
		pos.PositionPriceShort = round3(pos.PositionCostShort / (float64(pos.VolumeShort) * volumeMultiple))
	}

	pos.Changed = true
	return true
}

// Algorithm_Type 常量定义
// 对应 C++ TThostFtdcAlgorithmType
const (
	algorithmTypeAll      = '1' // THOST_FTDC_AG_All: 全部盈亏计入可用
	algorithmTypeOnlyLost = '2' // THOST_FTDC_AG_OnlyLost: 只有亏损计入可用
	algorithmTypeOnlyGain = '3' // THOST_FTDC_AG_OnlyGain: 只有盈利计入可用
	algorithmTypeNone     = '4' // THOST_FTDC_AG_None: 盈亏不计入可用
)

// recalculatePositionAndAccountProfit 统一重算持仓盈亏和账户盈亏
// 对应 C++ SendUserData 中的盈亏计算逻辑 (tradectp.cpp:4048-4377)
// 类似 tradersim 的 recalculatePositionAndFloatProfit，在一次调用中完成所有计算
// 此函数应在 sendAllUserData() 中调用，而不是在行情回调中
// 注意：调用者需持有 userMu 锁
func (t *TraderCTP) recalculatePositionAndAccountProfit() {
	if t.user == nil || t.marketClient == nil {
		return
	}

	var totalPositionProfit, totalFloatProfit, totalOptionValue float64
	var somethingChanged bool

	// 遍历所有持仓，获取最新行情并计算盈亏
	for _, pos := range t.user.Positions {
		// 获取最新行情
		quote := t.marketClient.GetQuote(pos.InstrumentID)
		if quote == nil {
			// 没有行情数据，使用已有的盈亏值
			if !math.IsNaN(pos.PositionProfit) {
				totalPositionProfit += pos.PositionProfit
			}
			if !math.IsNaN(pos.FloatProfit) {
				totalFloatProfit += pos.FloatProfit
			}
			continue
		}

		// 更新持仓盈亏
		if t.updatePositionProfitWithQuote(pos, quote) {
			somethingChanged = true
		}

		// 累计盈亏
		isOption := pos.Ins != nil && pos.Ins.ProductClass == protocol.ProductClassOptions
		if isOption {
			// 期权价值单独计算，盈亏不计入账户汇总
			// 对应 C++ total_option_value 计算
			multiple := float64(1)
			if pos.Ins != nil && pos.Ins.VolumeMultiple > 0 {
				multiple = float64(pos.Ins.VolumeMultiple)
			}
			if !math.IsNaN(pos.LastPrice) && pos.LastPrice > 0 {
				totalOptionValue += float64(pos.VolumeLong) * multiple * pos.LastPrice
				totalOptionValue -= float64(pos.VolumeShort) * multiple * pos.LastPrice
			}
		} else {
			// 期货盈亏计入汇总
			if !math.IsNaN(pos.PositionProfit) {
				totalPositionProfit += pos.PositionProfit
			}
			if !math.IsNaN(pos.FloatProfit) {
				totalFloatProfit += pos.FloatProfit
			}
		}
	}

	// 更新账户盈亏
	if !somethingChanged {
		return
	}

	for _, acc := range t.user.Accounts {
		// 计算 Algorithm_Type 对可用资金的影响
		// 对应 C++ switch (m_Algorithm_Type) { ... }
		previousPositionProfit := acc.PositionProfit
		dv := totalPositionProfit - previousPositionProfit // balance 变化量

		var poOri, poCurr float64 // 原盈亏和当前盈亏 (用于可用资金计算)
		switch t.algorithmType {
		case algorithmTypeAll:
			// 全部盈亏计入可用
			poOri = previousPositionProfit
			poCurr = totalPositionProfit
		case algorithmTypeOnlyLost:
			// 只有亏损计入可用
			if previousPositionProfit < 0 {
				poOri = previousPositionProfit
			}
			if totalPositionProfit < 0 {
				poCurr = totalPositionProfit
			}
		case algorithmTypeOnlyGain:
			// 只有盈利计入可用
			if previousPositionProfit > 0 {
				poOri = previousPositionProfit
			}
			if totalPositionProfit > 0 {
				poCurr = totalPositionProfit
			}
		case algorithmTypeNone:
			// 盈亏不计入可用
			poOri = 0
			poCurr = 0
		default:
			// 默认行为：全部盈亏计入可用
			poOri = previousPositionProfit
			poCurr = totalPositionProfit
		}
		avDiff := poCurr - poOri // 可用资金变化量

		acc.PositionProfit = totalPositionProfit
		acc.FloatProfit = totalFloatProfit
		// 动态权益 = 原权益 + 盈亏变化
		acc.Balance += dv
		// 市值权益 = 动态权益 + 期权价值
		acc.ValueBalance = acc.Balance + totalOptionValue
		// 可用资金 = 原可用 + 可用变化量
		acc.Available += avDiff
		// 风险度
		if acc.Balance > 0 && !math.IsNaN(acc.Margin) {
			acc.RiskRatio = acc.Margin / acc.Balance
		} else {
			acc.RiskRatio = math.NaN()
		}
		acc.Changed = true
	}
}

// startPositionUpdateLoop 启动定时推送用户数据循环
// 注意：盈亏计算已由 onQuotesUpdate 行情回调驱动完成
// 此定时器的主要作用是定时推送变化的数据给客户端
func (t *TraderCTP) startPositionUpdateLoop() {
	// 500ms 推送一次，足够实时又不会太频繁
	t.positionUpdateTicker = time.NewTicker(500 * time.Millisecond)

	go func() {
		for {
			select {
			case <-t.ctx.Done():
				t.positionUpdateTicker.Stop()
				return
			case <-t.positionUpdateTicker.C:
				t.sendUserDataIfChanged()
			}
		}
	}()
}

// sendUserDataIfChanged 如果有数据变化则推送给客户端
// 这里不再重复计算盈亏，因为 onQuotesUpdate 已经完成了计算
func (t *TraderCTP) sendUserDataIfChanged() {
	// 直接发送变化的数据 (diff 模式会检查 changed 标志)
	t.sendAllUserData()
}

// escapeJSONString 转义 JSON 字符串
func escapeJSONString(s string) string {
	var result strings.Builder
	for _, c := range s {
		switch c {
		case '"':
			result.WriteString(`\"`)
		case '\\':
			result.WriteString(`\\`)
		case '\n':
			result.WriteString(`\n`)
		case '\r':
			result.WriteString(`\r`)
		case '\t':
			result.WriteString(`\t`)
		default:
			result.WriteRune(c)
		}
	}
	return result.String()
}

// 辅助函数
func truncateMsg(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen] + "..."
}

// startIdleLoop 启动 OnIdle 循环
// 对应 C++ OnIdle 轮询机制
func (t *TraderCTP) startIdleLoop() {
	t.idleTicker = time.NewTicker(100 * time.Millisecond)

	go func() {
		for {
			select {
			case <-t.ctx.Done():
				t.idleTicker.Stop()
				return
			case <-t.idleTicker.C:
				t.onIdle()
			}
		}
	}()

	logger.Info("idle loop started")
}

// onIdle 空闲处理
// 对应 C++ traderctp::OnIdle
func (t *TraderCTP) onIdle() {
	// 检查登录状态
	if t.ctpSpi == nil || t.getState() != StateReady {
		return
	}

	// 1. 保存文件
	if t.queryScheduler.NeedSaveFile() {
		t.saveToFile()
		t.queryScheduler.SetNeedSaveFile(false)
	}

	// 2. 发送用户数据
	if t.queryScheduler.CanSendData() && t.peekMessage.Load() {
		t.sendAllUserData()
	}

	// 3. 检查查询冷却
	if !t.queryScheduler.CanQuery() {
		return
	}

	// 4. 按优先级检查各类查询
	// 持仓查询
	if t.queryScheduler.NeedPositionQuery() {
		t.ctpSpi.ReqQryInvestorPosition()
		logger.Debug("idle: position query triggered")
		t.queryScheduler.DelayQueryTime(t.queryScheduler.queryInterval)
		return
	}

	// 经纪商参数查询
	if t.queryScheduler.NeedQueryBrokerParams() {
		t.ctpSpi.ReqQryBrokerTradingParams()
		logger.Debug("idle: broker params query triggered")
		t.queryScheduler.DelayQueryTime(t.queryScheduler.queryInterval)
		return
	}

	// 账户查询
	if t.queryScheduler.NeedAccountQuery() {
		t.ctpSpi.ReqQryTradingAccount()
		logger.Debug("idle: account query triggered")
		t.queryScheduler.DelayQueryTime(t.queryScheduler.queryInterval)
		return
	}

	// 银行查询
	if t.queryScheduler.NeedQueryBank() {
		t.ctpSpi.ReqQryContractBank()
		logger.Debug("idle: bank query triggered")
		t.queryScheduler.DelayQueryTime(t.queryScheduler.queryInterval)
		return
	}

	// 签约关系查询
	if t.queryScheduler.NeedQueryRegister() {
		t.ctpSpi.ReqQryAccountRegister()
		logger.Debug("idle: register query triggered")
		t.queryScheduler.DelayQueryTime(t.queryScheduler.queryInterval)
		return
	}
}

// requestRefreshAfterOrder 订单回报后请求刷新持仓和账户
// 对应 C++ ProcessRtnOrder 中的 m_req_position_id++; m_req_account_id++;
func (t *TraderCTP) requestRefreshAfterOrder() {
	if t.queryScheduler == nil {
		return
	}
	t.queryScheduler.RequestPositionQuery()
	t.queryScheduler.RequestAccountQuery()
	t.queryScheduler.SetNeedSaveFile(true) // 订单变化需要保存映射
}

// requestRefreshAfterTrade 成交回报后请求刷新持仓和账户
// 对应 C++ ProcessRtnTrade 中的资金刷新逻辑
func (t *TraderCTP) requestRefreshAfterTrade() {
	if t.queryScheduler == nil {
		return
	}
	t.queryScheduler.RequestPositionQuery()
	t.queryScheduler.RequestAccountQuery()
}

// requestRefreshAfterTransfer 转账成功后请求刷新账户
// 对应 C++ ProcessFromBankToFutureByFuture 中的 m_req_account_id++
func (t *TraderCTP) requestRefreshAfterTransfer() {
	if t.queryScheduler == nil {
		return
	}
	t.queryScheduler.RequestAccountQuery()
}

// initConditionOrderManager 初始化条件单管理器
func (t *TraderCTP) initConditionOrderManager() {
	if config.Global == nil || !config.Global.ConditionOrder.Enabled {
		logger.Info("condition order disabled")
		return
	}

	if t.user == nil || t.reqLogin == nil || t.broker == nil {
		logger.Warn("condition order init failed: user data not ready")
		return
	}

	// 创建条件单配置
	coConfig := &condorder.Config{
		Enabled:             config.Global.ConditionOrder.Enabled,
		DataPath:            config.Global.ConditionOrder.DataPath,
		MaxNewOrdersPerDay:  config.Global.ConditionOrder.MaxNewOrdersPerDay,
		MaxValidOrdersTotal: config.Global.ConditionOrder.MaxValidOrdersTotal,
	}

	// 创建条件单管理器
	userKey := t.broker.CtpBrokerID + "." + t.reqLogin.UserName
	t.condOrderMgr = condorder.NewManager(userKey, t, coConfig)

	// 加载条件单数据
	tradingDay := t.user.TradingDay
	if err := t.condOrderMgr.Load(t.broker.CtpBrokerID, t.reqLogin.UserName, t.reqLogin.Password, tradingDay); err != nil {
		logger.Error("load condition order failed", zap.Error(err))
		return
	}

	logger.Info("condition order manager initialized",
		zap.String("user_key", userKey),
		zap.String("trading_day", tradingDay))
}

// startConditionOrderChecker 启动条件单检测
func (t *TraderCTP) startConditionOrderChecker() {
	if t.condOrderMgr == nil {
		return
	}

	// 价格检测：每 200ms 一次
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if t.getState() == StateReady {
					t.condOrderMgr.OnCheckPrice()
				}
			case <-t.ctx.Done():
				return
			}
		}
	}()

	// 时间检测：每秒一次
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if t.getState() == StateReady {
					t.condOrderMgr.OnCheckTime()
				}
			case <-t.ctx.Done():
				return
			}
		}
	}()

	logger.Info("condition order checker started")
}

// 实现 condorder.Callback 接口

// OnTouchConditionOrder 条件单触发回调
func (t *TraderCTP) OnTouchConditionOrder(order *condorder.ConditionOrder) {
	logger.Info("condition order touched",
		zap.String("order_id", order.OrderID),
		zap.Int("order_count", len(order.OrderList)))

	// 遍历触发后的订单列表，发送实际委托
	for i := range order.OrderList {
		co := &order.OrderList[i]
		if err := t.insertOrderFromCondition(co); err != nil {
			logger.Error("insert order from condition failed",
				zap.String("order_id", order.OrderID),
				zap.Int("order_index", i),
				zap.Error(err))
		}
	}
}

// insertOrderFromCondition 根据条件单发送实际委托
func (t *TraderCTP) insertOrderFromCondition(co *condorder.ContingentOrder) error {
	symbol := co.ExchangeID + "." + co.InstrumentID

	// 获取合约信息
	ins := t.GetInstrument(symbol)
	if ins == nil {
		return fmt.Errorf("instrument not found: %s", symbol)
	}

	// 计算实际价格
	var price float64
	switch co.PriceType {
	case condorder.PriceTypeLimit:
		price = co.LimitPrice
	case condorder.PriceTypeMarket, condorder.PriceTypeContingent:
		// 市价单或触发价：使用对手价
		if co.Direction == condorder.OrderDirectionBuy {
			price = ins.AskPrice1
		} else {
			price = ins.BidPrice1
		}
	case condorder.PriceTypeConsideration:
		// 对价：使用对手价
		if co.Direction == condorder.OrderDirectionBuy {
			price = ins.AskPrice1
		} else {
			price = ins.BidPrice1
		}
	case condorder.PriceTypeOver:
		// 超价：对手价加一个价格跳动
		tick := ins.PriceTick
		if math.IsNaN(tick) || tick <= 0 {
			tick = 1.0
		}
		if co.Direction == condorder.OrderDirectionBuy {
			price = ins.AskPrice1 + tick
		} else {
			price = ins.BidPrice1 - tick
		}
	default:
		price = co.LimitPrice
	}

	// 计算实际手数
	volume := co.Volume
	if co.VolumeType == condorder.VolumeTypeCloseAll {
		// 全平：查询持仓
		t.userMu.RLock()
		if pos := t.user.Positions[symbol]; pos != nil {
			if co.Direction == condorder.OrderDirectionBuy {
				volume = pos.VolumeShort
			} else {
				volume = pos.VolumeLong
			}
		} else {
			volume = 0
		}
		t.userMu.RUnlock()
	}

	if volume <= 0 {
		logger.Warn("condition order volume is zero, skip",
			zap.String("symbol", symbol),
			zap.String("volume_type", co.VolumeType.String()))
		return nil
	}

	// 转换方向和开平
	var direction int64
	if co.Direction == condorder.OrderDirectionBuy {
		direction = protocol.DirectionBuy
	} else {
		direction = protocol.DirectionSell
	}

	var offset int64
	switch co.Offset {
	case condorder.OrderOffsetOpen:
		offset = protocol.OffsetOpen
	case condorder.OrderOffsetClose:
		offset = protocol.OffsetClose
	default:
		offset = protocol.OffsetOpen
	}

	// 发送委托（简化版本，实际应该调用 handleInsertOrder）
	logger.Info("insert order from condition",
		zap.String("symbol", symbol),
		zap.Int64("direction", direction),
		zap.Int64("offset", offset),
		zap.Int("volume", volume),
		zap.Float64("price", price))

	// TODO: 调用实际的下单函数
	// t.doInsertOrder(co.ExchangeID, co.InstrumentID, direction, offset, volume, price, co.CloseTodayPrior)

	return nil
}

// OnUserDataChange 用户数据变更通知（实现 condorder.Callback 接口）
func (t *TraderCTP) OnUserDataChange() {
	// 条件单数据变化时推送用户数据
	// 使用现有的 sendUserDataIfChanged 机制
}

// OutputNotify 发送通知（实现 condorder.Callback 接口）
func (t *TraderCTP) OutputNotify(code int, msg, level, msgType string) {
	t.outputNotifyAll(int64(code), msg, level)
}
