// Package inslist 合约信息服务
// 对应 C++ open-trade-common/ins_list.cpp
// 提供统一的合约信息访问接口，供 trader 和 tradersim 共用
package inslist

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tidwall/gjson"

	"alpha-trade-gateway/pkg/marketfeed"
	"alpha-trade-gateway/pkg/protocol"
)

const (
	// DefaultInsListURL 天勤合约列表默认 URL
	DefaultInsListURL = "https://openmd.shinnytech.com/t/md/symbols/latest.json"

	// SourceTianqin 数据来源：天勤
	SourceTianqin = "tianqin"
	// SourceCTP 数据来源：CTP
	SourceCTP = "ctp"
)

// Config 合约服务配置
type Config struct {
	// Source 数据来源: "tianqin" 或 "ctp"
	Source string `json:"source" mapstructure:"source"`

	// TianqinURL 天勤合约列表 URL (source=tianqin 时使用)
	TianqinURL string `json:"tianqin_url" mapstructure:"tianqin_url"`
}

// InstrumentInfo 合约信息
// 对应 C++ Instrument 结构，保持字段一致
type InstrumentInfo struct {
	// === C++ 原版字段 (完全对应) ===
	Expired        bool    `json:"expired"`         // 是否已过期
	ProductClass   int64   `json:"product_class"`   // 产品类型 (期货/期权)
	VolumeMultiple int64   `json:"volume_multiple"` // 合约乘数
	Volume         int64   `json:"volume"`          // 成交量 (来自行情)
	Margin         float64 `json:"margin"`          // 保证金率
	Commission     float64 `json:"commission"`      // 手续费率
	PriceTick      float64 `json:"price_tick"`      // 最小变动价位

	// 动态行情字段 (volatile in C++)
	LastPrice     float64 `json:"last_price"`
	PreSettlement float64 `json:"pre_settlement"`
	UpperLimit    float64 `json:"upper_limit"`
	LowerLimit    float64 `json:"lower_limit"`
	AskPrice1     float64 `json:"ask_price1"`
	BidPrice1     float64 `json:"bid_price1"`
	Settlement    float64 `json:"settlement"`
	PreClose      float64 `json:"pre_close"`

	// === Go 版本扩展字段 (便于使用) ===
	Symbol       string `json:"symbol"`        // 完整代码 "SHFE.au2406"
	ExchangeID   string `json:"exchange_id"`   // 交易所代码
	InstrumentID string `json:"instrument_id"` // 合约代码
}

// NewInstrumentInfo 创建新的合约信息实例，使用 NaN 初始化价格字段
func NewInstrumentInfo() *InstrumentInfo {
	return &InstrumentInfo{
		Expired:       false,
		ProductClass:  protocol.ProductClassFutures,
		PriceTick:     math.NaN(),
		LastPrice:     math.NaN(),
		PreSettlement: math.NaN(),
		UpperLimit:    math.NaN(),
		LowerLimit:    math.NaN(),
		AskPrice1:     math.NaN(),
		BidPrice1:     math.NaN(),
		Settlement:    math.NaN(),
		PreClose:      math.NaN(),
	}
}

// InstrumentService 合约信息服务
// 对应 C++ ins_list.cpp 的功能
type InstrumentService struct {
	// 配置
	config Config

	// 合约信息缓存 (对应 C++ InsMapType)
	instruments   map[string]*InstrumentInfo
	instrumentsMu sync.RWMutex

	// 合约代码到交易所的映射 (对应 C++ m_instrumentExchangeIdMap)
	instrumentExchangeMap   map[string]string
	instrumentExchangeMapMu sync.RWMutex

	// 是否已初始化
	initialized atomic.Bool
}

// NewInstrumentService 创建合约信息服务
func NewInstrumentService(cfg Config) *InstrumentService {
	if cfg.Source == "" {
		cfg.Source = SourceTianqin
	}
	if cfg.TianqinURL == "" {
		cfg.TianqinURL = DefaultInsListURL
	}

	return &InstrumentService{
		config:                cfg,
		instruments:           make(map[string]*InstrumentInfo),
		instrumentExchangeMap: make(map[string]string),
	}
}

// Init 初始化服务
// 对应 C++ GenInstrumentExchangeIdMap
// 根据配置的 Source 决定初始化方式：
// - tianqin: 从天勤 URL 加载合约列表
// - ctp: 不做任何操作，等待 SetInstrument 调用
func (s *InstrumentService) Init() error {
	if s.initialized.Load() {
		return nil
	}

	switch s.config.Source {
	case SourceTianqin:
		// 从天勤获取合约列表
		if err := s.loadFromTianqin(); err != nil {
			return err
		}
		// 生成合约代码到交易所的映射
		s.genInstrumentExchangeMap()
		s.initialized.Store(true)

	case SourceCTP:
		// CTP 模式：不做任何操作，等待 SetInstrument 调用
		// initialized 标志在 MarkReady() 中设置
	}

	return nil
}

// MarkReady 标记服务就绪（CTP 模式下，查询完成后调用）
func (s *InstrumentService) MarkReady() {
	s.genInstrumentExchangeMap()
	s.initialized.Store(true)
}

// IsReady 检查服务是否就绪
func (s *InstrumentService) IsReady() bool {
	return s.initialized.Load()
}

// loadFromTianqin 从天勤加载合约列表
func (s *InstrumentService) loadFromTianqin() error {
	resp, err := http.Get(s.config.TianqinURL)
	if err != nil {
		return fmt.Errorf("fetch instrument list failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read instrument list failed: %w", err)
	}

	// 解析 JSON
	s.instrumentsMu.Lock()
	defer s.instrumentsMu.Unlock()

	gjson.ParseBytes(body).ForEach(func(symbol, value gjson.Result) bool {
		ins := s.parseInstrumentFromTianqin(symbol.String(), value)
		if ins != nil {
			s.instruments[symbol.String()] = ins
		}
		return true
	})

	return nil
}

// genInstrumentExchangeMap 生成合约代码到交易所的映射
// 对应 C++ GenInstrumentExchangeIdMap
func (s *InstrumentService) genInstrumentExchangeMap() {
	s.instrumentsMu.RLock()
	defer s.instrumentsMu.RUnlock()

	s.instrumentExchangeMapMu.Lock()
	defer s.instrumentExchangeMapMu.Unlock()

	for _, ins := range s.instruments {
		// 只记录第一个出现的映射（与 C++ 行为一致）
		if _, exists := s.instrumentExchangeMap[ins.InstrumentID]; !exists {
			s.instrumentExchangeMap[ins.InstrumentID] = ins.ExchangeID
		}
	}
}

// parseInstrumentFromTianqin 解析天勤格式的合约信息
func (s *InstrumentService) parseInstrumentFromTianqin(symbol string, value gjson.Result) *InstrumentInfo {
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 {
		return nil
	}

	ins := NewInstrumentInfo()
	ins.Symbol = symbol
	ins.ExchangeID = parts[0]
	ins.InstrumentID = parts[1]

	// 解析静态字段
	if v := value.Get("class"); v.Exists() {
		switch v.String() {
		case "FUTURE":
			ins.ProductClass = protocol.ProductClassFutures
		case "OPTION":
			ins.ProductClass = protocol.ProductClassOptions
		case "FUTURE_OPTION":
			ins.ProductClass = protocol.ProductClassFOption
		case "INDEX":
			ins.ProductClass = protocol.ProductClassFutureIndex
		case "CONT":
			ins.ProductClass = protocol.ProductClassFutureContinuous
		default:
			ins.ProductClass = protocol.ProductClassFutures
		}
	}

	if v := value.Get("volume_multiple"); v.Exists() {
		ins.VolumeMultiple = v.Int()
	}
	if v := value.Get("price_tick"); v.Exists() {
		ins.PriceTick = v.Float()
	}
	if v := value.Get("margin"); v.Exists() {
		ins.Margin = v.Float()
	}
	if v := value.Get("commission"); v.Exists() {
		ins.Commission = v.Float()
	}
	if v := value.Get("expired"); v.Exists() {
		ins.Expired = v.Bool()
	}

	return ins
}

// GetInstrument 获取合约信息
// 对应 C++ GetInstrument
func (s *InstrumentService) GetInstrument(symbol string) *InstrumentInfo {
	s.instrumentsMu.RLock()
	defer s.instrumentsMu.RUnlock()

	if ins, exists := s.instruments[symbol]; exists {
		return ins
	}
	return nil
}

// SetInstrument 设置/更新合约信息
// 供 CTP OnRspQryInstrument 回调使用
func (s *InstrumentService) SetInstrument(symbol string, ins *InstrumentInfo) {
	s.instrumentsMu.Lock()
	defer s.instrumentsMu.Unlock()

	if existing, exists := s.instruments[symbol]; exists {
		// 更新静态字段，保留动态行情字段
		existing.Expired = ins.Expired
		existing.ProductClass = ins.ProductClass
		existing.VolumeMultiple = ins.VolumeMultiple
		existing.Margin = ins.Margin
		existing.Commission = ins.Commission
		existing.PriceTick = ins.PriceTick
	} else {
		s.instruments[symbol] = ins
	}
}

// UpdateQuote 更新单个合约的行情数据
// 供 MarketFeed 行情回调使用
func (s *InstrumentService) UpdateQuote(quote *marketfeed.Quote) {
	if quote == nil {
		return
	}

	// 构建 symbol
	symbol := quote.InstrumentID
	if quote.ExchangeID != "" && !strings.Contains(symbol, ".") {
		symbol = quote.ExchangeID + "." + quote.InstrumentID
	}

	s.instrumentsMu.Lock()
	defer s.instrumentsMu.Unlock()

	ins, exists := s.instruments[symbol]
	if !exists {
		// 如果合约不存在，创建一个新的（仅包含行情数据）
		ins = NewInstrumentInfo()
		ins.Symbol = symbol
		ins.ExchangeID = quote.ExchangeID
		ins.InstrumentID = quote.InstrumentID
		s.instruments[symbol] = ins
	}

	// 更新动态行情字段
	ins.LastPrice = quote.LastPrice
	ins.AskPrice1 = quote.AskPrice1
	ins.BidPrice1 = quote.BidPrice1
	ins.UpperLimit = quote.UpperLimit
	ins.LowerLimit = quote.LowerLimit
	ins.PreSettlement = quote.PreSettlement
	ins.PreClose = quote.PreClose
	ins.Settlement = quote.Settlement
	ins.Volume = int64(quote.Volume)
}

// UpdateQuotes 批量更新行情数据
func (s *InstrumentService) UpdateQuotes(quotes []*marketfeed.Quote) {
	for _, quote := range quotes {
		s.UpdateQuote(quote)
	}
}

// OnQuotes 行情回调函数，可直接注册到 MarketClient.SetOnQuotes
// 使用示例:
//
//	insService := inslist.NewInstrumentService(cfg)
//	marketClient := marketfeed.NewTqMarketClient("")
//	marketClient.SetOnQuotes(insService.OnQuotes)
func (s *InstrumentService) OnQuotes(quotes []*marketfeed.Quote) {
	s.UpdateQuotes(quotes)
}

// GuessExchangeID 根据合约代码猜测交易所
// 对应 C++ GuessExchangeId
func (s *InstrumentService) GuessExchangeID(instrumentID string) string {
	s.instrumentExchangeMapMu.RLock()
	defer s.instrumentExchangeMapMu.RUnlock()

	if exchangeID, exists := s.instrumentExchangeMap[instrumentID]; exists {
		return exchangeID
	}
	return "UNKNOWN"
}

// GetAllInstruments 获取所有合约
func (s *InstrumentService) GetAllInstruments() map[string]*InstrumentInfo {
	s.instrumentsMu.RLock()
	defer s.instrumentsMu.RUnlock()

	result := make(map[string]*InstrumentInfo, len(s.instruments))
	for k, v := range s.instruments {
		result[k] = v
	}
	return result
}

// GetFuturesInstruments 获取所有期货合约
func (s *InstrumentService) GetFuturesInstruments() map[string]*InstrumentInfo {
	s.instrumentsMu.RLock()
	defer s.instrumentsMu.RUnlock()

	result := make(map[string]*InstrumentInfo)
	for k, v := range s.instruments {
		if v.ProductClass == protocol.ProductClassFutures {
			result[k] = v
		}
	}
	return result
}

// IsFutures 判断是否为期货合约
func (s *InstrumentService) IsFutures(symbol string) bool {
	ins := s.GetInstrument(symbol)
	if ins == nil {
		return false
	}
	return ins.ProductClass == protocol.ProductClassFutures
}

// IsOption 判断是否为期权合约
func (s *InstrumentService) IsOption(symbol string) bool {
	ins := s.GetInstrument(symbol)
	if ins == nil {
		return false
	}
	return ins.ProductClass == protocol.ProductClassOptions ||
		ins.ProductClass == protocol.ProductClassFOption
}

// GetSymbol 根据合约代码获取完整 symbol
func (s *InstrumentService) GetSymbol(instrumentID string) string {
	exchangeID := s.GuessExchangeID(instrumentID)
	if exchangeID == "UNKNOWN" {
		return ""
	}
	return exchangeID + "." + instrumentID
}

// Count 返回合约数量
func (s *InstrumentService) Count() int {
	s.instrumentsMu.RLock()
	defer s.instrumentsMu.RUnlock()
	return len(s.instruments)
}
