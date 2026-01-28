package marketfeed

import (
	"fmt"
	"math"
	"strings"

	"github.com/pseudocodes/go2ctp/thost"
)

type Quote struct {
	InstrumentID    string  `json:"instrument_id"`
	Datetime        string  `json:"datetime"`
	AskPrice5       float64 `json:"ask_price5"`
	AskVolume5      int     `json:"ask_volume5"`
	AskPrice4       float64 `json:"ask_price4"`
	AskVolume4      int     `json:"ask_volume4"`
	AskPrice3       float64 `json:"ask_price3"`
	AskVolume3      int     `json:"ask_volume3"`
	AskPrice2       float64 `json:"ask_price2"`
	AskVolume2      int     `json:"ask_volume2"`
	AskPrice1       float64 `json:"ask_price1"`
	AskVolume1      int     `json:"ask_volume1"`
	BidPrice1       float64 `json:"bid_price1"`
	BidVolume1      int     `json:"bid_volume1"`
	BidPrice2       float64 `json:"bid_price2"`
	BidVolume2      int     `json:"bid_volume2"`
	BidPrice3       float64 `json:"bid_price3"`
	BidVolume3      int     `json:"bid_volume3"`
	BidPrice4       float64 `json:"bid_price4"`
	BidVolume4      int     `json:"bid_volume4"`
	BidPrice5       float64 `json:"bid_price5"`
	BidVolume5      int     `json:"bid_volume5"`
	LastPrice       float64 `json:"last_price"`
	Highest         float64 `json:"highest"`
	Lowest          float64 `json:"lowest"`
	Open            float64 `json:"open"`
	Close           float64 `json:"close"`
	Average         float64 `json:"average"`
	Volume          int     `json:"volume"`
	Amount          float64 `json:"amount"`
	OpenInterest    int     `json:"open_interest"`
	Settlement      float64 `json:"settlement"`
	UpperLimit      float64 `json:"upper_limit"`
	LowerLimit      float64 `json:"lower_limit"`
	PreOpenInterest int     `json:"pre_open_interest"`
	PreSettlement   float64 `json:"pre_settlement"`
	PreClose        float64 `json:"pre_close"`
	ExchangeID      string  `json:"exchange_id"`
}

func NewQuote() *Quote {
	return &Quote{
		LastPrice:     math.NaN(),
		Settlement:    math.NaN(),
		PreSettlement: math.NaN(),
		PreClose:      math.NaN(),
	}
}

// convertCtpQuoteToQuote 从 CTP 深度行情数据转换为 Quote
func convertCtpQuoteToQuote(pDepthMarketData *thost.CThostFtdcDepthMarketDataField) *Quote {
	quote := NewQuote()

	// 合约信息
	quote.InstrumentID = strings.TrimRight(string(pDepthMarketData.InstrumentID[:]), "\x00")
	quote.ExchangeID = strings.TrimRight(string(pDepthMarketData.ExchangeID[:]), "\x00")

	// 时间信息
	tradingDay := strings.TrimRight(string(pDepthMarketData.TradingDay[:]), "\x00")
	updateTime := strings.TrimRight(string(pDepthMarketData.UpdateTime[:]), "\x00")
	quote.Datetime = fmt.Sprintf("%s %s.%d", tradingDay, updateTime, pDepthMarketData.UpdateMillisec)

	// 价格信息
	quote.LastPrice = checkPrice(float64(pDepthMarketData.LastPrice))
	quote.Open = checkPrice(float64(pDepthMarketData.OpenPrice))
	quote.Highest = checkPrice(float64(pDepthMarketData.HighestPrice))
	quote.Lowest = checkPrice(float64(pDepthMarketData.LowestPrice))
	quote.Close = checkPrice(float64(pDepthMarketData.ClosePrice))
	quote.PreClose = checkPrice(float64(pDepthMarketData.PreClosePrice))
	quote.Settlement = checkPrice(float64(pDepthMarketData.SettlementPrice))
	quote.PreSettlement = checkPrice(float64(pDepthMarketData.PreSettlementPrice))
	quote.UpperLimit = checkPrice(float64(pDepthMarketData.UpperLimitPrice))
	quote.LowerLimit = checkPrice(float64(pDepthMarketData.LowerLimitPrice))
	quote.Average = checkPrice(float64(pDepthMarketData.AveragePrice))

	// 买价和买量
	quote.BidPrice1 = checkPrice(float64(pDepthMarketData.BidPrice1))
	quote.BidVolume1 = int(pDepthMarketData.BidVolume1)
	quote.BidPrice2 = checkPrice(float64(pDepthMarketData.BidPrice2))
	quote.BidVolume2 = int(pDepthMarketData.BidVolume2)
	quote.BidPrice3 = checkPrice(float64(pDepthMarketData.BidPrice3))
	quote.BidVolume3 = int(pDepthMarketData.BidVolume3)
	quote.BidPrice4 = checkPrice(float64(pDepthMarketData.BidPrice4))
	quote.BidVolume4 = int(pDepthMarketData.BidVolume4)
	quote.BidPrice5 = checkPrice(float64(pDepthMarketData.BidPrice5))
	quote.BidVolume5 = int(pDepthMarketData.BidVolume5)

	// 卖价和卖量
	quote.AskPrice1 = checkPrice(float64(pDepthMarketData.AskPrice1))
	quote.AskVolume1 = int(pDepthMarketData.AskVolume1)
	quote.AskPrice2 = checkPrice(float64(pDepthMarketData.AskPrice2))
	quote.AskVolume2 = int(pDepthMarketData.AskVolume2)
	quote.AskPrice3 = checkPrice(float64(pDepthMarketData.AskPrice3))
	quote.AskVolume3 = int(pDepthMarketData.AskVolume3)
	quote.AskPrice4 = checkPrice(float64(pDepthMarketData.AskPrice4))
	quote.AskVolume4 = int(pDepthMarketData.AskVolume4)
	quote.AskPrice5 = checkPrice(float64(pDepthMarketData.AskPrice5))
	quote.AskVolume5 = int(pDepthMarketData.AskVolume5)

	// 成交量和持仓量
	quote.Volume = int(pDepthMarketData.Volume)
	quote.Amount = float64(pDepthMarketData.Turnover)
	quote.OpenInterest = int(pDepthMarketData.OpenInterest)
	quote.PreOpenInterest = int(pDepthMarketData.PreOpenInterest)

	return quote
}

// checkPrice 检查价格是否有效
// CTP API 中无效价格使用 DBL_MAX (1.7976931348623157e+308) 表示
func checkPrice(price float64) float64 {
	// CTP 使用 1.7976931348623157e+308 表示无效价格
	if price >= 1e308 || price <= -1e308 || math.IsNaN(price) || math.IsInf(price, 0) {
		return math.NaN()
	}
	return price
}
