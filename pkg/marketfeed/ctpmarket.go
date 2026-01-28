package marketfeed

import (
	"strings"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/logger"
)

// CtpMarketClient CTP 行情客户端
// 与 TqMarketClient 对应，但使用 CTP MdApi
type CtpMarketClient struct {
	conn     *CtpConnection
	quitC    chan struct{}
	closed   atomic.Bool
	OnQuotes func(quotes []*Quote)

	// 行情缓存
	quotesMap sync.Map // map[string]*Quote
}

// NewCtpMarketClient 创建 CTP 行情客户端
func NewCtpMarketClient(frontAddr, brokerID, userID, password, flowPath string) *CtpMarketClient {
	cli := &CtpMarketClient{
		conn:  NewCtpConnection(frontAddr, brokerID, userID, password, flowPath),
		quitC: make(chan struct{}),
	}

	// 设置回调
	cli.conn.OnConnected = func() {
		logger.Info("CTP market client connected")
	}

	cli.conn.OnDisconnected = func(reason int) {
		logger.Warn("CTP market client disconnected", zap.Int("reason", reason))
	}

	cli.conn.OnLogin = func(success bool, errorMsg string) {
		if success {
			logger.Info("CTP market client logged in")
		} else {
			logger.Error("CTP market client login failed", zap.String("error", errorMsg))
		}
	}

	cli.conn.OnMarketData = func(quote *Quote) {
		// 缓存行情
		cli.quotesMap.Store(quote.InstrumentID, quote)

		// 触发回调
		if cli.OnQuotes != nil {
			cli.OnQuotes([]*Quote{quote})
		}
	}

	cli.conn.OnSubscribed = func(symbol string, success bool) {
		if success {
			logger.Info("CTP market client subscribed", zap.String("symbol", symbol))
		} else {
			logger.Error("CTP market client subscribe failed", zap.String("symbol", symbol))
		}
	}

	return cli
}

// Start 启动客户端
func (c *CtpMarketClient) Start() error {
	logger.Info("CTP market client starting")
	return c.conn.Connect()
}

// Subscribe 订阅行情
// symbol 支持两种格式:
//   - 纯合约代码: "ag2601"
//   - 带交易所前缀: "SHFE.ag2601" (会自动去掉前缀)
func (c *CtpMarketClient) Subscribe(symbols ...string) error {
	ctpSymbols := normalizeSymbols(symbols)
	return c.conn.Subscribe(ctpSymbols...)
}

// Unsubscribe 退订行情
// symbol 支持两种格式:
//   - 纯合约代码: "ag2601"
//   - 带交易所前缀: "SHFE.ag2601" (会自动去掉前缀)
func (c *CtpMarketClient) Unsubscribe(symbols ...string) error {
	ctpSymbols := normalizeSymbols(symbols)
	return c.conn.Unsubscribe(ctpSymbols...)
}

// normalizeSymbols 规范化合约代码
// CTP 的合约代码不需要交易所前缀
// 如果传入 "SHFE.ag2601" 格式，会自动转换为 "ag2601"
func normalizeSymbols(symbols []string) []string {
	result := make([]string, 0, len(symbols))
	for _, s := range symbols {
		result = append(result, extractInstrumentID(s))
	}
	return result
}

// extractInstrumentID 从 symbol 中提取合约代码
// 输入: "SHFE.ag2601" 或 "ag2601"
// 输出: "ag2601"
func extractInstrumentID(symbol string) string {
	if idx := strings.LastIndex(symbol, "."); idx >= 0 {
		return symbol[idx+1:]
	}
	return symbol
}

// GetQuote 获取行情
// 对应 C++ InstrumentMap 中的合约数据获取
func (c *CtpMarketClient) GetQuote(symbol string) *Quote {
	if v, ok := c.quotesMap.Load(symbol); ok {
		return v.(*Quote)
	}
	return nil
}

// GetAllQuotes 获取所有缓存的行情
func (c *CtpMarketClient) GetAllQuotes() []*Quote {
	quotes := make([]*Quote, 0)
	c.quotesMap.Range(func(key, value interface{}) bool {
		quotes = append(quotes, value.(*Quote))
		return true
	})
	return quotes
}

// SetOnQuotes 设置行情回调函数
func (c *CtpMarketClient) SetOnQuotes(callback func(quotes []*Quote)) {
	c.OnQuotes = callback
}

// IsConnected 是否已连接
func (c *CtpMarketClient) IsConnected() bool {
	return c.conn.IsConnected()
}

// IsLoggedIn 是否已登录
func (c *CtpMarketClient) IsLoggedIn() bool {
	return c.conn.IsLoggedIn()
}

// GetTradingDay 获取交易日
func (c *CtpMarketClient) GetTradingDay() string {
	return c.conn.GetTradingDay()
}

// Close 关闭客户端
func (c *CtpMarketClient) Close() {
	if c.closed.Swap(true) {
		return
	}

	logger.Info("CTP market client closing")
	close(c.quitC)

	if c.conn != nil {
		c.conn.Close()
	}
}
