package marketfeed

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/logger"
)

const (
	MarketWSURL = "wss://openmd.shinnytech.com/t/md/front/mobile"
	PeekMsg     = `{"aid": "peek_message"}`
)

// TqMarketClient 天勤行情客户端
// 对应 C++ InstrumentMap 中的行情数据获取
type TqMarketClient struct {
	targetURL string
	quitC     chan struct{}
	closed    atomic.Bool

	tqconn *TqConnection
	// subSet 已订阅的合约集合
	// 天勤协议不支持增量订阅，需要维护完整的订阅列表
	// 使用 map 类型方便去重和增删操作
	subSet   map[string]struct{}
	OnQuotes func(quotes []*Quote)

	// 行情缓存
	quotesMap sync.Map // map[string]*Quote
}

func NewTqMarketClient(wsurl string) *TqMarketClient {
	if wsurl == "" {
		wsurl = MarketWSURL
	}
	cli := &TqMarketClient{
		targetURL: wsurl,
		tqconn:    NewTqConnection(wsurl),
		quitC:     make(chan struct{}),
		subSet:    make(map[string]struct{}),
	}
	cli.tqconn.OnReconnect = func() {
		// 重连后直接发送已保存的订阅列表
		if len(cli.subSet) > 0 {
			cli.sendSubscribeRequest()
		}
	}

	return cli
}

func (c *TqMarketClient) Start() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	headers := http.Header{}
	headers.Add("Accept", "application/json")

	if err := c.tqconn.Connect(ctx, headers); err != nil {
		return err
	}

	go c.doRecv()
	return nil
}

// StartWithToken 使用 token 启动
func (c *TqMarketClient) StartWithToken(token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+token)
	headers.Add("Accept", "application/json")

	if err := c.tqconn.Connect(ctx, headers); err != nil {
		return err
	}

	go c.doRecv()
	return nil
}

func (c *TqMarketClient) doRecv() {
	for {
		select {
		case msg := <-c.tqconn.Rx():
			gr := gjson.Get(msg, "aid")
			if !gr.Exists() {
				logger.Error("aid not exists in message")
				continue
			}
			aid := gr.String()
			switch aid {
			case "rsp_login":
				logger.Debug("rsp_login received", zap.String("msg", msg))
			case "rtn_data":
				jsdata := gjson.Get(msg, "data.0.quotes")
				if !jsdata.Exists() {
					c.tqconn.txC <- PeekMsg
					continue
				}
				quotes := []*Quote{}
				jsdata.ForEach(func(symbol, value gjson.Result) bool {
					quote := c.parseQuote(symbol.String(), value)
					quotes = append(quotes, quote)
					// 缓存行情
					c.quotesMap.Store(symbol.String(), quote)
					return true
				})
				if c.OnQuotes != nil {
					c.OnQuotes(quotes)
				}
				c.tqconn.txC <- PeekMsg
			}
		case <-c.quitC:
			logger.Info("marketfeed recv loop quit")
			return
		}
	}
}

func (c *TqMarketClient) parseQuote(symbol string, value gjson.Result) *Quote {
	quote := NewQuote()
	quote.InstrumentID = symbol
	// logger.Warn("parsing quote", zap.String("symbol", symbol), zap.String("value", value.Raw))
	value.ForEach(func(k, v gjson.Result) bool {
		if v.Type == gjson.Null {
			return true
		}
		switch k.String() {
		case "ask_price1":
			quote.AskPrice1 = v.Float()
		case "bid_price1":
			quote.BidPrice1 = v.Float()
		case "last_price":
			if v.Type == gjson.Number {
				quote.LastPrice = v.Float()
			}
		case "close":
			if v.Type == gjson.Number {
				quote.Close = v.Float()
			}
		case "volume":
			if v.Type == gjson.Number {
				quote.Volume = int(v.Int())
			}
		case "settlement":
			if v.Type == gjson.Number {
				quote.Settlement = v.Float()
			}
		case "upper_limit":
			if v.Type == gjson.Number {
				quote.UpperLimit = v.Float()
			}
		case "lower_limit":
			if v.Type == gjson.Number {
				quote.LowerLimit = v.Float()
			}
		case "pre_settlement":
			if v.Type == gjson.Number {
				quote.PreSettlement = v.Float()
			}
		case "pre_close":
			if v.Type == gjson.Number {
				quote.PreClose = v.Float()
			}
		case "highest":
			if v.Type == gjson.Number {
				quote.Highest = v.Float()
			}
		case "lowest":
			if v.Type == gjson.Number {
				quote.Lowest = v.Float()
			}
		case "open":
			if v.Type == gjson.Number {
				quote.Open = v.Float()
			}
		case "open_interest":
			if v.Type == gjson.Number {
				quote.OpenInterest = int(v.Int())
			}
		default:
			// logger.Warn("unknown quote field", zap.String("field", k.String()), zap.String("value", v.String()))
		}
		return true
	})
	return quote
}

// Subscribe 订阅行情
// 天勤协议的订阅机制：
//   - 服务端不支持增量订阅，每次都需要发送完整的订阅列表
//   - 本方法会将新合约添加到 subSet 集合中（自动去重）
//   - 然后将完整的订阅列表发送给服务端
func (c *TqMarketClient) Subscribe(symbols ...string) error {
	// 将新合约添加到订阅集合（map 自动去重）
	for _, s := range symbols {
		c.subSet[s] = struct{}{}
	}

	// 发送完整的订阅列表到服务端
	c.sendSubscribeRequest()
	return nil
}

// Unsubscribe 退订行情
// 天勤协议的退订机制：
//   - 服务端不支持单独退订某个合约
//   - 本方法会从 subSet 集合中移除指定的合约
//   - 然后将剩余的合约列表重新发送给服务端
func (c *TqMarketClient) Unsubscribe(symbols ...string) error {
	// 从订阅集合中移除指定合约
	for _, s := range symbols {
		delete(c.subSet, s)
	}

	// 重新发送剩余的订阅列表到服务端
	// 如果列表为空，仍需发送空列表以清空服务端订阅
	c.sendSubscribeRequest()
	return nil
}

// sendSubscribeRequest 发送订阅请求到服务端
func (c *TqMarketClient) sendSubscribeRequest() {
	// 从 map 构建合约列表
	symbols := make([]string, 0, len(c.subSet))
	for s := range c.subSet {
		symbols = append(symbols, s)
	}

	inslistStr := strings.Join(symbols, ",")
	subquote := `{"aid": "subscribe_quote", "ins_list": "` + inslistStr + `"}`
	logger.Debug("subscribe request", zap.String("sub", subquote), zap.Int("count", len(symbols)))
	c.tqconn.Tx() <- subquote
	c.tqconn.Tx() <- PeekMsg
}

// GetQuote 获取行情
// 对应 C++ InstrumentMap 中的合约数据获取
func (c *TqMarketClient) GetQuote(symbol string) *Quote {
	if v, ok := c.quotesMap.Load(symbol); ok {
		return v.(*Quote)
	}
	return nil
}

// GetAllQuotes 获取所有缓存的行情
func (c *TqMarketClient) GetAllQuotes() []*Quote {
	quotes := make([]*Quote, 0)
	c.quotesMap.Range(func(key, value interface{}) bool {
		quotes = append(quotes, value.(*Quote))
		return true
	})
	return quotes
}

// SetOnQuotes 设置行情回调函数
func (c *TqMarketClient) SetOnQuotes(callback func(quotes []*Quote)) {
	c.OnQuotes = callback
}

// Close 关闭客户端
func (c *TqMarketClient) Close() {
	close(c.quitC)

	if c.tqconn != nil {
		c.tqconn.Close()
	}
	c.closed.Store(true)
}
