// Package trader PluginContext 实现
// 为 Plugin 提供系统能力的具体实现
package trader

import (
	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/config"
	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/marketfeed"
	"alpha-trade-gateway/pkg/protocol"
)

// pluginContextImpl PluginContext 的具体实现
type pluginContextImpl struct {
	trader     *TraderCore
	pluginName string
	settings   map[string]any
	logger     *zap.Logger
}

// newPluginContext 创建 PluginContext (内部使用)
// pluginName 和 settings 在 InitAll 时按 Plugin 设置
func newPluginContext(trader *TraderCore, pluginName string, settings map[string]any) PluginContext {
	return &pluginContextImpl{
		trader:     trader,
		pluginName: pluginName,
		settings:   settings,
		logger:     logger.L.Named(pluginName),
	}
}

func (c *pluginContextImpl) EventBus() *EventBus {
	return c.trader.eventBus
}

func (c *pluginContextImpl) CtpApiOps() *CtpApiOps {
	return c.trader.ctpApiOps
}

func (c *pluginContextImpl) UserData() *UserDataSnapshot {
	c.trader.userMu.RLock()
	defer c.trader.userMu.RUnlock()

	if c.trader.user == nil {
		return nil
	}

	// 返回浅拷贝快照
	snap := &UserDataSnapshot{
		UserID:     c.trader.user.UserID,
		TradingDay: c.trader.user.TradingDay,
		Accounts:   make(map[string]*protocol.Account, len(c.trader.user.Accounts)),
		Positions:  make(map[string]*protocol.Position, len(c.trader.user.Positions)),
		Orders:     make(map[string]*protocol.Order, len(c.trader.user.Orders)),
		Trades:     make(map[string]*protocol.Trade, len(c.trader.user.Trades)),
	}
	for k, v := range c.trader.user.Accounts {
		snap.Accounts[k] = v
	}
	for k, v := range c.trader.user.Positions {
		snap.Positions[k] = v
	}
	for k, v := range c.trader.user.Orders {
		snap.Orders[k] = v
	}
	for k, v := range c.trader.user.Trades {
		snap.Trades[k] = v
	}
	return snap
}

func (c *pluginContextImpl) InstrumentService() InstrumentProvider {
	return &instrumentProviderAdapter{trader: c.trader}
}

func (c *pluginContextImpl) MarketData() MarketDataProvider {
	return &marketDataProviderAdapter{trader: c.trader}
}

func (c *pluginContextImpl) Config() *config.Config {
	return config.Global
}

func (c *pluginContextImpl) PluginSettings() map[string]any {
	return c.settings
}

func (c *pluginContextImpl) LoginInfo() (brokerID, userName, password string) {
	t := c.trader
	if t.broker != nil {
		brokerID = t.broker.CtpBrokerID
	}
	if t.reqLogin != nil {
		userName = t.reqLogin.UserName
		password = t.reqLogin.Password
	}
	return
}

func (c *pluginContextImpl) Notify(code int64, msg, level string) {
	c.trader.outputNotifyAll(code, msg, level)
}

func (c *pluginContextImpl) NotifyTo(connID int, code int64, msg, level string) {
	c.trader.outputNotify(connID, code, msg, level)
}

func (c *pluginContextImpl) SendMsg(connID int, msg string) {
	c.trader.SendMsg(connID, msg)
}

func (c *pluginContextImpl) SendMsgAll(msg string) {
	c.trader.SendMsgAll(msg)
}

func (c *pluginContextImpl) RegisterMessageHandler(aid string, handler MessageHandler) error {
	return c.trader.pluginManager.RegisterMessageHandler(c.pluginName, aid, handler)
}

func (c *pluginContextImpl) RegisterDataProvider(provider DataProvider) {
	c.trader.pluginManager.RegisterDataProvider(provider)
}

func (c *pluginContextImpl) RegisterUserDataInjector(injector UserDataInjector) {
	c.trader.pluginManager.RegisterUserDataInjector(injector)
}

func (c *pluginContextImpl) TriggerUserDataPush() {
	// 触发一次数据推送
	go c.trader.sendUserDataIfChanged()
}

func (c *pluginContextImpl) Logger() *zap.Logger {
	return c.logger
}

// === 适配器 ===

// instrumentProviderAdapter 合约信息适配器
type instrumentProviderAdapter struct {
	trader *TraderCore
}

func (a *instrumentProviderAdapter) GetInstrument(symbol string) *protocol.Instrument {
	return a.trader.GetInstrument(symbol)
}

// marketDataProviderAdapter 行情数据适配器
type marketDataProviderAdapter struct {
	trader *TraderCore
}

func (a *marketDataProviderAdapter) GetQuote(symbol string) *marketfeed.Quote {
	if a.trader.marketClient == nil {
		return nil
	}
	return a.trader.marketClient.GetQuote(symbol)
}

func (a *marketDataProviderAdapter) Subscribe(symbols ...string) error {
	if a.trader.marketClient == nil {
		return nil
	}
	return a.trader.marketClient.Subscribe(symbols...)
}
