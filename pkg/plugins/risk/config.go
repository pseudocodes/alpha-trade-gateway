// Package risk 风控插件配置
package risk

// RiskPluginConfig 风控插件内部配置 (来自 config.json plugins[].settings)
type RiskPluginConfig struct {
	// 开仓次数限制规则列表
	OpenCountsRules []OpenCountsRuleConfig `json:"open_counts_rules"`
	// 开仓手数限制规则列表
	OpenVolumesRules []OpenVolumesRuleConfig `json:"open_volumes_rules"`
	// 累计开仓手数限制规则列表
	AccOpenVolumesRules []AccOpenVolumesRuleConfig `json:"acc_open_volumes_rules"`
	// 每秒操作频率限制 (按交易所)
	OrderRateRules []OrderRateRuleConfig `json:"order_rate_rules"`

	// 是否启用自成交检测
	SelfTradeCheck bool `json:"self_trade_check"`
	// 是否启用频繁报撤单检测
	FrequentCancellationCheck bool `json:"frequent_cancellation_check"`
	// 是否启用成交持仓比检测
	TradePositionRatioCheck bool `json:"trade_position_ratio_check"`

	// 默认风控规则 (推送给客户端的初始值)
	DefaultRules map[string]*RiskManagementRule `json:"default_rules"`
}

// OpenCountsRuleConfig 开仓次数限制配置
type OpenCountsRuleConfig struct {
	Symbols []string `json:"symbols"` // 受限合约列表
	Limit   int      `json:"limit"`   // 最大开仓次数
}

// OpenVolumesRuleConfig 开仓手数限制配置
type OpenVolumesRuleConfig struct {
	Symbols []string `json:"symbols"` // 受限合约列表
	Limit   int      `json:"limit"`   // 最大开仓手数
}

// AccOpenVolumesRuleConfig 累计开仓手数限制配置
type AccOpenVolumesRuleConfig struct {
	Name    string   `json:"name"`    // 规则名称
	Symbols []string `json:"symbols"` // 合约列表
	Limit   int      `json:"limit"`   // 累计上限
}

// OrderRateRuleConfig 操作频率限制配置
type OrderRateRuleConfig struct {
	Exchanges []string `json:"exchanges"` // 受限交易所列表
	Limit     int      `json:"limit"`     // 每秒最大操作次数
}

// accVolumeRule 累计手数规则的运行时索引
type accVolumeRule struct {
	symbols map[string]bool
	limit   int
}

// parseRiskPluginConfig 从 Plugin settings 解析配置
func parseRiskPluginConfig(settings map[string]any) *RiskPluginConfig {
	cfg := &RiskPluginConfig{}
	if settings == nil {
		return cfg
	}

	// 简化实现: 直接从 map 中提取配置
	// 实际可用 mapstructure 或 json marshal/unmarshal
	if v, ok := settings["self_trade_check"].(bool); ok {
		cfg.SelfTradeCheck = v
	}
	if v, ok := settings["frequent_cancellation_check"].(bool); ok {
		cfg.FrequentCancellationCheck = v
	}
	if v, ok := settings["trade_position_ratio_check"].(bool); ok {
		cfg.TradePositionRatioCheck = v
	}

	return cfg
}

// buildIndex 构建规则快速查找索引
func (cfg *RiskPluginConfig) buildIndex() (
	openCountsIndex map[string]int,
	openVolumesIndex map[string]int,
	accVolumesIndex map[string]*accVolumeRule,
	orderRateIndex map[string]int,
) {
	openCountsIndex = make(map[string]int)
	for _, r := range cfg.OpenCountsRules {
		for _, sym := range r.Symbols {
			openCountsIndex[sym] = r.Limit
		}
	}

	openVolumesIndex = make(map[string]int)
	for _, r := range cfg.OpenVolumesRules {
		for _, sym := range r.Symbols {
			openVolumesIndex[sym] = r.Limit
		}
	}

	accVolumesIndex = make(map[string]*accVolumeRule)
	for _, r := range cfg.AccOpenVolumesRules {
		symbols := make(map[string]bool)
		for _, s := range r.Symbols {
			symbols[s] = true
		}
		accVolumesIndex[r.Name] = &accVolumeRule{symbols: symbols, limit: r.Limit}
	}

	orderRateIndex = make(map[string]int)
	for _, r := range cfg.OrderRateRules {
		for _, ex := range r.Exchanges {
			orderRateIndex[ex] = r.Limit
		}
	}

	return
}
