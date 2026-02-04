package inslist

import (
	"math"
	"testing"

	"alpha-trade-gateway/pkg/marketfeed"
	"alpha-trade-gateway/pkg/protocol"
)

// TestNewInstrumentInfo 测试创建合约信息实例
func TestNewInstrumentInfo(t *testing.T) {
	ins := NewInstrumentInfo()

	if ins == nil {
		t.Fatal("NewInstrumentInfo returned nil")
	}

	// 检查默认值
	if ins.Expired != false {
		t.Errorf("Expired should be false, got %v", ins.Expired)
	}
	if ins.ProductClass != protocol.ProductClassFutures {
		t.Errorf("ProductClass should be %d, got %d", protocol.ProductClassFutures, ins.ProductClass)
	}

	// 检查 NaN 初始化
	if !math.IsNaN(ins.PriceTick) {
		t.Errorf("PriceTick should be NaN, got %v", ins.PriceTick)
	}
	if !math.IsNaN(ins.LastPrice) {
		t.Errorf("LastPrice should be NaN, got %v", ins.LastPrice)
	}
	if !math.IsNaN(ins.PreSettlement) {
		t.Errorf("PreSettlement should be NaN, got %v", ins.PreSettlement)
	}
	if !math.IsNaN(ins.UpperLimit) {
		t.Errorf("UpperLimit should be NaN, got %v", ins.UpperLimit)
	}
	if !math.IsNaN(ins.LowerLimit) {
		t.Errorf("LowerLimit should be NaN, got %v", ins.LowerLimit)
	}
	if !math.IsNaN(ins.AskPrice1) {
		t.Errorf("AskPrice1 should be NaN, got %v", ins.AskPrice1)
	}
	if !math.IsNaN(ins.BidPrice1) {
		t.Errorf("BidPrice1 should be NaN, got %v", ins.BidPrice1)
	}
}

// TestNewInstrumentService 测试创建合约信息服务
func TestNewInstrumentService(t *testing.T) {
	cfg := Config{
		Source:     SourceTianqin,
		TianqinURL: DefaultInsListURL,
	}
	svc := NewInstrumentService(cfg)

	if svc == nil {
		t.Fatal("NewInstrumentService returned nil")
	}

	if svc.instruments == nil {
		t.Error("instruments map should not be nil")
	}
	if svc.instrumentExchangeMap == nil {
		t.Error("instrumentExchangeMap should not be nil")
	}
	if svc.config.TianqinURL != DefaultInsListURL {
		t.Errorf("TianqinURL should be %s, got %s", DefaultInsListURL, svc.config.TianqinURL)
	}
}

// TestNewInstrumentServiceDefaults 测试默认配置
func TestNewInstrumentServiceDefaults(t *testing.T) {
	// 空配置应该使用默认值
	svc := NewInstrumentService(Config{})

	if svc.config.Source != SourceTianqin {
		t.Errorf("Source should default to %s, got %s", SourceTianqin, svc.config.Source)
	}
	if svc.config.TianqinURL != DefaultInsListURL {
		t.Errorf("TianqinURL should default to %s, got %s", DefaultInsListURL, svc.config.TianqinURL)
	}
}

// TestGuessExchangeID 测试根据合约代码猜测交易所
func TestGuessExchangeID(t *testing.T) {
	svc := NewInstrumentService(Config{Source: SourceCTP})

	// 手动添加一些合约
	svc.instrumentsMu.Lock()
	svc.instruments["SHFE.au2406"] = &InstrumentInfo{
		Symbol:       "SHFE.au2406",
		ExchangeID:   "SHFE",
		InstrumentID: "au2406",
	}
	svc.instruments["DCE.m2409"] = &InstrumentInfo{
		Symbol:       "DCE.m2409",
		ExchangeID:   "DCE",
		InstrumentID: "m2409",
	}
	svc.instruments["CFFEX.IF2406"] = &InstrumentInfo{
		Symbol:       "CFFEX.IF2406",
		ExchangeID:   "CFFEX",
		InstrumentID: "IF2406",
	}
	svc.instrumentsMu.Unlock()

	// 生成映射
	svc.genInstrumentExchangeMap()

	tests := []struct {
		instrumentID string
		wantExchange string
	}{
		{"au2406", "SHFE"},
		{"m2409", "DCE"},
		{"IF2406", "CFFEX"},
		{"unknown", "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.instrumentID, func(t *testing.T) {
			got := svc.GuessExchangeID(tt.instrumentID)
			if got != tt.wantExchange {
				t.Errorf("GuessExchangeID(%s) = %s, want %s", tt.instrumentID, got, tt.wantExchange)
			}
		})
	}
}

// TestGetSymbol 测试根据合约代码获取完整 symbol
func TestGetSymbol(t *testing.T) {
	svc := NewInstrumentService(Config{Source: SourceCTP})

	// 手动添加合约并生成映射
	svc.instrumentsMu.Lock()
	svc.instruments["SHFE.au2406"] = &InstrumentInfo{
		Symbol:       "SHFE.au2406",
		ExchangeID:   "SHFE",
		InstrumentID: "au2406",
	}
	svc.instrumentsMu.Unlock()
	svc.genInstrumentExchangeMap()

	// 测试已知合约
	symbol := svc.GetSymbol("au2406")
	if symbol != "SHFE.au2406" {
		t.Errorf("GetSymbol(au2406) = %s, want SHFE.au2406", symbol)
	}

	// 测试未知合约
	symbol = svc.GetSymbol("unknown")
	if symbol != "" {
		t.Errorf("GetSymbol(unknown) = %s, want empty string", symbol)
	}
}

// TestIsFutures 测试判断是否为期货合约
func TestIsFutures(t *testing.T) {
	svc := NewInstrumentService(Config{Source: SourceCTP})

	// 添加测试数据
	svc.instrumentsMu.Lock()
	svc.instruments["SHFE.au2406"] = &InstrumentInfo{
		Symbol:       "SHFE.au2406",
		ProductClass: protocol.ProductClassFutures,
	}
	svc.instruments["CFFEX.IO2406-C-4000"] = &InstrumentInfo{
		Symbol:       "CFFEX.IO2406-C-4000",
		ProductClass: protocol.ProductClassOptions,
	}
	svc.instrumentsMu.Unlock()

	if !svc.IsFutures("SHFE.au2406") {
		t.Error("IsFutures(SHFE.au2406) should be true")
	}
	if svc.IsFutures("CFFEX.IO2406-C-4000") {
		t.Error("IsFutures(CFFEX.IO2406-C-4000) should be false")
	}
	if svc.IsFutures("unknown") {
		t.Error("IsFutures(unknown) should be false")
	}
}

// TestIsOption 测试判断是否为期权合约
func TestIsOption(t *testing.T) {
	svc := NewInstrumentService(Config{Source: SourceCTP})

	// 添加测试数据
	svc.instrumentsMu.Lock()
	svc.instruments["SHFE.au2406"] = &InstrumentInfo{
		Symbol:       "SHFE.au2406",
		ProductClass: protocol.ProductClassFutures,
	}
	svc.instruments["CFFEX.IO2406-C-4000"] = &InstrumentInfo{
		Symbol:       "CFFEX.IO2406-C-4000",
		ProductClass: protocol.ProductClassOptions,
	}
	svc.instruments["SHFE.au2406C5000"] = &InstrumentInfo{
		Symbol:       "SHFE.au2406C5000",
		ProductClass: protocol.ProductClassFOption,
	}
	svc.instrumentsMu.Unlock()

	if svc.IsOption("SHFE.au2406") {
		t.Error("IsOption(SHFE.au2406) should be false")
	}
	if !svc.IsOption("CFFEX.IO2406-C-4000") {
		t.Error("IsOption(CFFEX.IO2406-C-4000) should be true")
	}
	if !svc.IsOption("SHFE.au2406C5000") {
		t.Error("IsOption(SHFE.au2406C5000) should be true for FOption")
	}
}

// TestGetAllInstruments 测试获取所有合约
func TestGetAllInstruments(t *testing.T) {
	svc := NewInstrumentService(Config{Source: SourceCTP})

	// 添加测试数据
	svc.instrumentsMu.Lock()
	svc.instruments["SHFE.au2406"] = &InstrumentInfo{Symbol: "SHFE.au2406"}
	svc.instruments["DCE.m2409"] = &InstrumentInfo{Symbol: "DCE.m2409"}
	svc.instrumentsMu.Unlock()

	all := svc.GetAllInstruments()
	if len(all) != 2 {
		t.Errorf("GetAllInstruments() returned %d instruments, want 2", len(all))
	}
}

// TestGetFuturesInstruments 测试获取所有期货合约
func TestGetFuturesInstruments(t *testing.T) {
	svc := NewInstrumentService(Config{Source: SourceCTP})

	// 添加测试数据
	svc.instrumentsMu.Lock()
	svc.instruments["SHFE.au2406"] = &InstrumentInfo{
		Symbol:       "SHFE.au2406",
		ProductClass: protocol.ProductClassFutures,
	}
	svc.instruments["DCE.m2409"] = &InstrumentInfo{
		Symbol:       "DCE.m2409",
		ProductClass: protocol.ProductClassFutures,
	}
	svc.instruments["CFFEX.IO2406-C-4000"] = &InstrumentInfo{
		Symbol:       "CFFEX.IO2406-C-4000",
		ProductClass: protocol.ProductClassOptions,
	}
	svc.instrumentsMu.Unlock()

	futures := svc.GetFuturesInstruments()
	if len(futures) != 2 {
		t.Errorf("GetFuturesInstruments() returned %d instruments, want 2", len(futures))
	}
}

// TestCount 测试合约数量
func TestCount(t *testing.T) {
	svc := NewInstrumentService(Config{Source: SourceCTP})

	if svc.Count() != 0 {
		t.Errorf("Count() = %d, want 0", svc.Count())
	}

	svc.instrumentsMu.Lock()
	svc.instruments["SHFE.au2406"] = &InstrumentInfo{}
	svc.instruments["DCE.m2409"] = &InstrumentInfo{}
	svc.instrumentsMu.Unlock()

	if svc.Count() != 2 {
		t.Errorf("Count() = %d, want 2", svc.Count())
	}
}

// TestSetInstrument 测试设置合约信息
func TestSetInstrument(t *testing.T) {
	svc := NewInstrumentService(Config{Source: SourceCTP})

	ins := &InstrumentInfo{
		Symbol:         "SHFE.au2406",
		ExchangeID:     "SHFE",
		InstrumentID:   "au2406",
		ProductClass:   protocol.ProductClassFutures,
		VolumeMultiple: 1000,
		PriceTick:      0.02,
		Margin:         0.08,
	}

	svc.SetInstrument("SHFE.au2406", ins)

	got := svc.GetInstrument("SHFE.au2406")
	if got == nil {
		t.Fatal("GetInstrument returned nil after SetInstrument")
	}
	if got.VolumeMultiple != 1000 {
		t.Errorf("VolumeMultiple = %d, want 1000", got.VolumeMultiple)
	}
	if got.PriceTick != 0.02 {
		t.Errorf("PriceTick = %f, want 0.02", got.PriceTick)
	}
}

// TestUpdateQuote 测试更新行情数据
func TestUpdateQuote(t *testing.T) {
	svc := NewInstrumentService(Config{Source: SourceCTP})

	// 先添加合约
	svc.SetInstrument("SHFE.au2406", &InstrumentInfo{
		Symbol:         "SHFE.au2406",
		ExchangeID:     "SHFE",
		InstrumentID:   "au2406",
		VolumeMultiple: 1000,
	})

	// 更新行情
	quote := &marketfeed.Quote{
		InstrumentID:  "au2406",
		ExchangeID:    "SHFE",
		LastPrice:     500.0,
		AskPrice1:     500.1,
		BidPrice1:     499.9,
		UpperLimit:    550.0,
		LowerLimit:    450.0,
		PreSettlement: 498.0,
		Volume:        12345,
	}
	svc.UpdateQuote(quote)

	ins := svc.GetInstrument("SHFE.au2406")
	if ins == nil {
		t.Fatal("GetInstrument returned nil")
	}
	if ins.LastPrice != 500.0 {
		t.Errorf("LastPrice = %f, want 500.0", ins.LastPrice)
	}
	if ins.AskPrice1 != 500.1 {
		t.Errorf("AskPrice1 = %f, want 500.1", ins.AskPrice1)
	}
	if ins.BidPrice1 != 499.9 {
		t.Errorf("BidPrice1 = %f, want 499.9", ins.BidPrice1)
	}
	if ins.Volume != 12345 {
		t.Errorf("Volume = %d, want 12345", ins.Volume)
	}
	// 静态字段应该保留
	if ins.VolumeMultiple != 1000 {
		t.Errorf("VolumeMultiple = %d, want 1000 (should be preserved)", ins.VolumeMultiple)
	}
}

// TestUpdateQuoteNewInstrument 测试更新不存在的合约行情
func TestUpdateQuoteNewInstrument(t *testing.T) {
	svc := NewInstrumentService(Config{Source: SourceCTP})

	// 更新一个不存在的合约
	quote := &marketfeed.Quote{
		InstrumentID:  "ag2406",
		ExchangeID:    "SHFE",
		LastPrice:     6000.0,
		AskPrice1:     6001.0,
		BidPrice1:     5999.0,
	}
	svc.UpdateQuote(quote)

	// 应该自动创建
	ins := svc.GetInstrument("SHFE.ag2406")
	if ins == nil {
		t.Fatal("GetInstrument returned nil, should auto-create")
	}
	if ins.LastPrice != 6000.0 {
		t.Errorf("LastPrice = %f, want 6000.0", ins.LastPrice)
	}
	if ins.Symbol != "SHFE.ag2406" {
		t.Errorf("Symbol = %s, want SHFE.ag2406", ins.Symbol)
	}
}

// TestOnQuotes 测试 OnQuotes 回调函数
func TestOnQuotes(t *testing.T) {
	svc := NewInstrumentService(Config{Source: SourceCTP})

	quotes := []*marketfeed.Quote{
		{
			InstrumentID: "au2406",
			ExchangeID:   "SHFE",
			LastPrice:    500.0,
		},
		{
			InstrumentID: "ag2406",
			ExchangeID:   "SHFE",
			LastPrice:    6000.0,
		},
	}

	// 使用 OnQuotes 回调
	svc.OnQuotes(quotes)

	// 验证两个合约都被更新
	ins1 := svc.GetInstrument("SHFE.au2406")
	if ins1 == nil || ins1.LastPrice != 500.0 {
		t.Error("OnQuotes failed to update SHFE.au2406")
	}

	ins2 := svc.GetInstrument("SHFE.ag2406")
	if ins2 == nil || ins2.LastPrice != 6000.0 {
		t.Error("OnQuotes failed to update SHFE.ag2406")
	}
}

// TestMarkReady 测试 CTP 模式的就绪标记
func TestMarkReady(t *testing.T) {
	svc := NewInstrumentService(Config{Source: SourceCTP})

	if svc.IsReady() {
		t.Error("IsReady should be false before MarkReady")
	}

	// 添加一些合约
	svc.SetInstrument("SHFE.au2406", &InstrumentInfo{
		Symbol:       "SHFE.au2406",
		ExchangeID:   "SHFE",
		InstrumentID: "au2406",
	})

	// 标记就绪
	svc.MarkReady()

	if !svc.IsReady() {
		t.Error("IsReady should be true after MarkReady")
	}

	// 验证交易所映射已生成
	exchangeID := svc.GuessExchangeID("au2406")
	if exchangeID != "SHFE" {
		t.Errorf("GuessExchangeID(au2406) = %s, want SHFE", exchangeID)
	}
}
