// Package risk 风控插件
// 实现服务端风控拦截 + 风控数据推送 (对齐 tqsdk-python 协议)
package risk

import (
	"fmt"

	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/trader"
)

func init() {
	trader.RegisterPlugin("risk", func() trader.Plugin {
		return &RiskPlugin{}
	})
}

// RiskPlugin 风控插件
type RiskPlugin struct {
	ctx    trader.PluginContext
	config *RiskPluginConfig
	stats  *RiskStats
	logger *zap.Logger

	// 协议层风控规则 (按交易所)
	rules map[string]*RiskManagementRule

	// 规则快速查找索引
	openCountsIndex  map[string]int
	openVolumesIndex map[string]int
	accVolumesIndex  map[string]*accVolumeRule
	orderRateIndex   map[string]int
}

func (p *RiskPlugin) Name() string { return "risk" }

func (p *RiskPlugin) Init(ctx trader.PluginContext) error {
	p.ctx = ctx
	p.logger = ctx.Logger()
	p.stats = NewRiskStats()
	p.rules = make(map[string]*RiskManagementRule)

	// 解析配置
	p.config = parseRiskPluginConfig(ctx.PluginSettings())
	p.openCountsIndex, p.openVolumesIndex, p.accVolumesIndex, p.orderRateIndex = p.config.buildIndex()

	// 初始化默认风控规则
	p.initDefaultRules()

	eb := ctx.EventBus()

	// 拦截下单/撤单
	eb.SubscribeInterceptor(trader.EventBeforeInsertOrder, p.handleBeforeInsertOrder, trader.PriorityHigh)
	eb.SubscribeInterceptor(trader.EventBeforeCancelOrder, p.handleBeforeCancelOrder, trader.PriorityHigh)

	// 监听柜台回报
	eb.Subscribe(trader.EventOrderReturn, p.processOrderReturn, trader.PriorityHigh)
	eb.Subscribe(trader.EventTradeReturn, p.processTradeReturn, trader.PriorityHigh)
	eb.Subscribe(trader.EventTradingDayChanged, p.processTradingDayChanged, trader.PriorityHigh)

	// 注册客户端消息处理
	if err := ctx.RegisterMessageHandler("set_risk_management_rule", p.handleSetRiskManagementRule); err != nil {
		return fmt.Errorf("register set_risk_management_rule: %w", err)
	}

	// 注册数据注入器
	ctx.RegisterUserDataInjector(p)

	p.logger.Info("risk plugin initialized")
	return nil
}

func (p *RiskPlugin) OnReady() {
	p.logger.Info("risk plugin ready")
}

func (p *RiskPlugin) Stop() error {
	p.logger.Info("risk plugin stopped")
	return nil
}

// initDefaultRules 从配置初始化默认风控规则
func (p *RiskPlugin) initDefaultRules() {
	if p.config.DefaultRules == nil {
		return
	}
	for exchangeID, rule := range p.config.DefaultRules {
		p.rules[exchangeID] = rule
		rule.Changed = true
	}
}
