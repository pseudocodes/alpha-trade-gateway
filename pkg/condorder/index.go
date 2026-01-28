// Package condorder 条件单索引
package condorder

// ConditionOrderIndex 条件单索引
// 用于优化条件检测性能，避免遍历所有条件单
type ConditionOrderIndex struct {
	// 开盘触发索引: symbol -> []orderID
	marketOpenOrders map[string][]string

	// 时间触发索引: orderID set
	timeOrders map[string]struct{}

	// 价格触发索引: symbol -> []orderID
	priceOrders map[string][]string
}

// NewConditionOrderIndex 创建索引
func NewConditionOrderIndex() *ConditionOrderIndex {
	return &ConditionOrderIndex{
		marketOpenOrders: make(map[string][]string),
		timeOrders:       make(map[string]struct{}),
		priceOrders:      make(map[string][]string),
	}
}

// Clear 清空索引
func (idx *ConditionOrderIndex) Clear() {
	idx.marketOpenOrders = make(map[string][]string)
	idx.timeOrders = make(map[string]struct{})
	idx.priceOrders = make(map[string][]string)
}

// AddMarketOpenOrder 添加开盘触发订单
func (idx *ConditionOrderIndex) AddMarketOpenOrder(symbol, orderID string) {
	idx.marketOpenOrders[symbol] = append(idx.marketOpenOrders[symbol], orderID)
}

// AddTimeOrder 添加时间触发订单
func (idx *ConditionOrderIndex) AddTimeOrder(orderID string) {
	idx.timeOrders[orderID] = struct{}{}
}

// AddPriceOrder 添加价格触发订单
func (idx *ConditionOrderIndex) AddPriceOrder(symbol, orderID string) {
	idx.priceOrders[symbol] = append(idx.priceOrders[symbol], orderID)
}

// GetMarketOpenOrders 获取指定合约的开盘触发订单
func (idx *ConditionOrderIndex) GetMarketOpenOrders(symbol string) []string {
	return idx.marketOpenOrders[symbol]
}

// GetPriceOrders 获取指定合约的价格触发订单
func (idx *ConditionOrderIndex) GetPriceOrders(symbol string) []string {
	return idx.priceOrders[symbol]
}

// GetMarketOpenSymbols 获取所有需要监听开盘的合约
func (idx *ConditionOrderIndex) GetMarketOpenSymbols() []string {
	symbols := make([]string, 0, len(idx.marketOpenOrders))
	for symbol := range idx.marketOpenOrders {
		symbols = append(symbols, symbol)
	}
	return symbols
}

// GetPriceSymbols 获取所有需要监听价格的合约
func (idx *ConditionOrderIndex) GetPriceSymbols() []string {
	symbols := make([]string, 0, len(idx.priceOrders))
	for symbol := range idx.priceOrders {
		symbols = append(symbols, symbol)
	}
	return symbols
}

// HasTimeOrders 是否有时间触发订单
func (idx *ConditionOrderIndex) HasTimeOrders() bool {
	return len(idx.timeOrders) > 0
}

// GetTimeOrders 获取所有时间触发订单ID
func (idx *ConditionOrderIndex) GetTimeOrders() []string {
	orders := make([]string, 0, len(idx.timeOrders))
	for orderID := range idx.timeOrders {
		orders = append(orders, orderID)
	}
	return orders
}
