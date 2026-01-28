package marketfeed

import (
	"sync"
	"sync/atomic"

	"github.com/pseudocodes/go2ctp/ctp"
	"github.com/pseudocodes/go2ctp/thost"
	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/logger"
)

// CtpConnection CTP 行情连接
// 封装 CTP MdApi 和 MdSpi
type CtpConnection struct {
	mdapi     thost.MdApi
	brokerID  string
	userID    string
	password  string
	frontAddr string

	// 回调
	OnConnected    func()
	OnDisconnected func(reason int)
	OnLogin        func(success bool, errorMsg string)
	OnMarketData   func(quote *Quote)
	OnSubscribed   func(symbol string, success bool)

	// 状态
	connected atomic.Bool
	loggedIn  atomic.Bool
	closed    atomic.Bool

	// 订阅列表
	subscribedMu sync.RWMutex
	subscribed   map[string]bool

	requestID atomic.Int32
}

// NewCtpConnection 创建 CTP 行情连接
func NewCtpConnection(frontAddr, brokerID, userID, password string, flowPath string) *CtpConnection {
	if flowPath == "" {
		flowPath = "./ctpmd_flow/"
	}

	conn := &CtpConnection{
		brokerID:   brokerID,
		userID:     userID,
		password:   password,
		frontAddr:  frontAddr,
		subscribed: make(map[string]bool),
	}

	// 创建 MdApi
	conn.mdapi = ctp.CreateMdApi(
		ctp.MdFlowPath(flowPath),
		ctp.MdUsingUDP(false),
		ctp.MdMultiCast(false),
	)

	// 创建并注册 SPI
	spi := conn.createSpi()
	conn.mdapi.RegisterSpi(spi)

	return conn
}

// createSpi 创建 MdSpi 回调处理
func (c *CtpConnection) createSpi() thost.MdSpi {
	spi := &ctp.BaseMdSpi{}

	// 前置连接成功
	spi.OnFrontConnectedCallback = func() {
		logger.Info("CTP market front connected")
		c.connected.Store(true)

		// 自动登录
		c.login()

		if c.OnConnected != nil {
			c.OnConnected()
		}
	}

	// 前置断开
	spi.OnFrontDisconnectedCallback = func(nReason int) {
		logger.Warn("CTP market front disconnected", zap.Int("reason", nReason))
		c.connected.Store(false)
		c.loggedIn.Store(false)

		if c.OnDisconnected != nil {
			c.OnDisconnected(nReason)
		}
	}

	// 登录响应
	spi.OnRspUserLoginCallback = func(
		pRspUserLogin *thost.CThostFtdcRspUserLoginField,
		pRspInfo *thost.CThostFtdcRspInfoField,
		nRequestID int,
		bIsLast bool,
	) {
		if pRspInfo != nil && pRspInfo.ErrorID != 0 {
			errMsg := pRspInfo.ErrorMsg.GBString()
			logger.Error("CTP market login failed",
				zap.Int("error_id", int(pRspInfo.ErrorID)),
				zap.String("error_msg", errMsg),
			)
			if c.OnLogin != nil {
				c.OnLogin(false, errMsg)
			}
			return
		}

		logger.Info("CTP market login success",
			zap.String("trading_day", pRspUserLogin.TradingDay.String()),
			zap.String("login_time", pRspUserLogin.LoginTime.String()),
		)
		c.loggedIn.Store(true)

		if c.OnLogin != nil {
			c.OnLogin(true, "")
		}

		// 重新订阅之前的合约
		c.resubscribe()
	}

	// 订阅响应
	spi.OnRspSubMarketDataCallback = func(
		pSpecificInstrument *thost.CThostFtdcSpecificInstrumentField,
		pRspInfo *thost.CThostFtdcRspInfoField,
		nRequestID int,
		bIsLast bool,
	) {
		if pSpecificInstrument == nil {
			return
		}

		symbol := pSpecificInstrument.InstrumentID.String()
		success := pRspInfo == nil || pRspInfo.ErrorID == 0

		if success {
			logger.Debug("CTP market subscribed", zap.String("symbol", symbol))
			c.subscribedMu.Lock()
			c.subscribed[symbol] = true
			c.subscribedMu.Unlock()
		} else {
			errMsg := pRspInfo.ErrorMsg.GBString()
			logger.Error("CTP market subscribe failed",
				zap.String("symbol", symbol),
				zap.Int("error_id", int(pRspInfo.ErrorID)),
				zap.String("error_msg", errMsg),
			)
		}

		if c.OnSubscribed != nil {
			c.OnSubscribed(symbol, success)
		}
	}

	// 行情数据
	spi.OnRtnDepthMarketDataCallback = func(pDepthMarketData *thost.CThostFtdcDepthMarketDataField) {
		if pDepthMarketData == nil {
			return
		}

		quote := convertCtpQuoteToQuote(pDepthMarketData)

		logger.Debug("CTP market data received",
			zap.String("symbol", quote.InstrumentID),
			zap.String("exchange_id", quote.ExchangeID),
			zap.Float64("last_price", quote.LastPrice),
		)

		if c.OnMarketData != nil {
			c.OnMarketData(quote)
		}
	}

	// 错误响应
	spi.OnRspErrorCallback = func(
		pRspInfo *thost.CThostFtdcRspInfoField,
		nRequestID int,
		bIsLast bool,
	) {
		if pRspInfo != nil && pRspInfo.ErrorID != 0 {
			errMsg := pRspInfo.ErrorMsg.GBString()
			logger.Error("CTP market error",
				zap.Int("error_id", int(pRspInfo.ErrorID)),
				zap.String("error_msg", errMsg),
			)
		}
	}

	return spi
}

// Connect 连接到 CTP 行情服务器
func (c *CtpConnection) Connect() error {
	logger.Info("CTP market connecting",
		zap.String("front", c.frontAddr),
		zap.String("broker_id", c.brokerID),
		zap.String("user_id", c.userID),
	)

	// 注册前置地址
	c.mdapi.RegisterFront(c.frontAddr)

	// 初始化
	c.mdapi.Init()

	logger.Info("CTP market API version", zap.String("version", c.mdapi.GetApiVersion()))

	return nil
}

// login 登录
func (c *CtpConnection) login() {
	reqID := int(c.requestID.Add(1))

	loginReq := &thost.CThostFtdcReqUserLoginField{}
	copy(loginReq.BrokerID[:], c.brokerID)
	copy(loginReq.UserID[:], c.userID)
	copy(loginReq.Password[:], c.password)

	ret := c.mdapi.ReqUserLogin(loginReq, reqID)
	logger.Info("CTP market login request sent", zap.Int("ret", ret))
}

// Subscribe 订阅行情
func (c *CtpConnection) Subscribe(symbols ...string) error {
	if !c.loggedIn.Load() {
		logger.Warn("CTP market not logged in, cannot subscribe")
		// 保存订阅列表，登录后会自动订阅
		c.subscribedMu.Lock()
		for _, symbol := range symbols {
			c.subscribed[symbol] = false
		}
		c.subscribedMu.Unlock()
		return nil
	}

	logger.Info("CTP market subscribing", zap.Strings("symbols", symbols))

	ret := c.mdapi.SubscribeMarketData(symbols...)
	if ret != 0 {
		logger.Error("CTP market subscribe failed", zap.Int("ret", ret))
	}

	// 记录订阅意图
	c.subscribedMu.Lock()
	for _, symbol := range symbols {
		c.subscribed[symbol] = false
	}
	c.subscribedMu.Unlock()

	return nil
}

// Unsubscribe 退订行情
func (c *CtpConnection) Unsubscribe(symbols ...string) error {
	if !c.loggedIn.Load() {
		logger.Warn("CTP market not logged in, cannot unsubscribe")
		return nil
	}

	logger.Info("CTP market unsubscribing", zap.Strings("symbols", symbols))

	ret := c.mdapi.UnSubscribeMarketData(symbols...)
	if ret != 0 {
		logger.Error("CTP market unsubscribe failed", zap.Int("ret", ret))
	}

	// 从订阅列表移除
	c.subscribedMu.Lock()
	for _, symbol := range symbols {
		delete(c.subscribed, symbol)
	}
	c.subscribedMu.Unlock()

	return nil
}

// resubscribe 重新订阅（用于重连后）
func (c *CtpConnection) resubscribe() {
	c.subscribedMu.RLock()
	symbols := make([]string, 0, len(c.subscribed))
	for symbol := range c.subscribed {
		symbols = append(symbols, symbol)
	}
	c.subscribedMu.RUnlock()

	if len(symbols) > 0 {
		logger.Info("CTP market resubscribing", zap.Int("count", len(symbols)))
		c.Subscribe(symbols...)
	}
}

// IsConnected 是否已连接
func (c *CtpConnection) IsConnected() bool {
	return c.connected.Load()
}

// IsLoggedIn 是否已登录
func (c *CtpConnection) IsLoggedIn() bool {
	return c.loggedIn.Load()
}

// GetTradingDay 获取交易日
func (c *CtpConnection) GetTradingDay() string {
	return c.mdapi.GetTradingDay()
}

// Close 关闭连接
func (c *CtpConnection) Close() {
	if c.closed.Swap(true) {
		return
	}

	logger.Info("CTP market connection closing")

	if c.mdapi != nil {
		c.mdapi.Release()
	}
}

// trimZero 去除字节数组中的零字节
func trimZero(b []byte) string {
	for i, v := range b {
		if v == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// gbkToUtf8 GBK 转 UTF-8（简化版）
func gbkToUtf8(b []byte) string {
	// TODO: 实现 GBK 到 UTF-8 的转换
	// 暂时简单处理，去除零字节
	return trimZero(b)
}
