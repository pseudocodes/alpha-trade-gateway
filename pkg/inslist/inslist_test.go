package inslist

import (
	"math"
	"testing"

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
	svc := NewInstrumentService(nil)

	if svc == nil {
		t.Fatal("NewInstrumentService returned nil")
	}

	if svc.instruments == nil {
		t.Error("instruments map should not be nil")
	}
	if svc.instrumentExchangeMap == nil {
		t.Error("instrumentExchangeMap should not be nil")
	}
	if svc.insListURL != DefaultInsListURL {
		t.Errorf("insListURL should be %s, got %s", DefaultInsListURL, svc.insListURL)
	}
}

// TestParseInstrument 测试解析合约信息
func TestParseInstrument(t *testing.T) {
	svc := NewInstrumentService(nil)

	tests := []struct {
		name         string
		symbol       string
		jsonStr      string
		wantExchange string
		wantInsID    string
		wantClass    int64
		wantMultiple int64
	}{
		{
			name:         "期货合约",
			symbol:       "SHFE.au2406",
			jsonStr:      `{"class":"FUTURE","volume_multiple":1000,"price_tick":0.02,"margin":0.08}`,
			wantExchange: "SHFE",
			wantInsID:    "au2406",
			wantClass:    protocol.ProductClassFutures,
			wantMultiple: 1000,
		},
		{
			name:         "期权合约",
			symbol:       "CFFEX.IO2406-C-4000",
			jsonStr:      `{"class":"OPTION","volume_multiple":100,"price_tick":0.2}`,
			wantExchange: "CFFEX",
			wantInsID:    "IO2406-C-4000",
			wantClass:    protocol.ProductClassOptions,
			wantMultiple: 100,
		},
		{
			name:         "指数合约",
			symbol:       "CFFEX.IF2406",
			jsonStr:      `{"class":"INDEX","volume_multiple":300}`,
			wantExchange: "CFFEX",
			wantInsID:    "IF2406",
			wantClass:    protocol.ProductClassFutureIndex,
			wantMultiple: 300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 使用 gjson 解析
			ins := svc.parseInstrumentFromJSON(tt.symbol, tt.jsonStr)

			if ins == nil {
				t.Fatal("parseInstrument returned nil")
			}
			if ins.Symbol != tt.symbol {
				t.Errorf("Symbol = %s, want %s", ins.Symbol, tt.symbol)
			}
			if ins.ExchangeID != tt.wantExchange {
				t.Errorf("ExchangeID = %s, want %s", ins.ExchangeID, tt.wantExchange)
			}
			if ins.InstrumentID != tt.wantInsID {
				t.Errorf("InstrumentID = %s, want %s", ins.InstrumentID, tt.wantInsID)
			}
			if ins.ProductClass != tt.wantClass {
				t.Errorf("ProductClass = %d, want %d", ins.ProductClass, tt.wantClass)
			}
			if ins.VolumeMultiple != tt.wantMultiple {
				t.Errorf("VolumeMultiple = %d, want %d", ins.VolumeMultiple, tt.wantMultiple)
			}
		})
	}
}

// TestParseInstrumentInvalidSymbol 测试无效 symbol
func TestParseInstrumentInvalidSymbol(t *testing.T) {
	svc := NewInstrumentService(nil)

	// 无效的 symbol (没有点号分隔)
	ins := svc.parseInstrumentFromJSON("invalid_symbol", `{"class":"FUTURE"}`)
	if ins != nil {
		t.Error("parseInstrument should return nil for invalid symbol")
	}
}

// TestGuessExchangeID 测试根据合约代码猜测交易所
func TestGuessExchangeID(t *testing.T) {
	svc := NewInstrumentService(nil)

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
	svc := NewInstrumentService(nil)

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
	svc := NewInstrumentService(nil)

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
	svc := NewInstrumentService(nil)

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
	svc := NewInstrumentService(nil)

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
	svc := NewInstrumentService(nil)

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
	svc := NewInstrumentService(nil)

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
