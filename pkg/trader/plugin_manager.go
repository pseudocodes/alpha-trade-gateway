// Package trader PluginManager 插件管理器
// 管理插件生命周期、消息路由表、数据提供者、用户数据注入器
package trader

import (
	"alpha-trade-gateway/pkg/logger"
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// coreAids 核心 aid 列表，不允许 Plugin 覆盖
var coreAids = map[string]bool{
	"req_login":            true,
	"confirm_settlement":   true,
	"insert_order":         true,
	"cancel_order":         true,
	"peek_message":         true,
	"change_password":      true,
	"req_transfer":         true,
	"qry_settlement_info":  true,
	"qry_account_info":     true,
	"qry_transfer_serial":  true,
	"qry_account_register": true,
	"req_reconnect_trade":  true,
	"req_start_ctp":        true,
	"req_stop_ctp":         true,
}

// PluginManager 插件管理器
type PluginManager struct {
	mu sync.RWMutex

	plugins  []Plugin
	eventBus *EventBus

	// 消息路由表: aid → MessageHandler
	msgHandlers map[string]msgRoute
	// 数据提供者列表 (独立数据帧，如 rtn_condition_orders)
	dataProviders []DataProvider
	// 用户数据注入器列表 (注入 rtn_data 数据树)
	userDataInjectors []UserDataInjector
}

// msgRoute 消息路由条目
type msgRoute struct {
	pluginName string
	handler    MessageHandler
}

// NewPluginManager 创建插件管理器
func NewPluginManager(eb *EventBus) *PluginManager {
	return &PluginManager{
		eventBus:    eb,
		msgHandlers: make(map[string]msgRoute),
	}
}

// Register 注册插件
func (pm *PluginManager) Register(plugin Plugin) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// 检查重名
	for _, p := range pm.plugins {
		if p.Name() == plugin.Name() {
			return fmt.Errorf("plugin %q already registered", plugin.Name())
		}
	}

	pm.plugins = append(pm.plugins, plugin)
	return nil
}

// NotifyReady 通知所有插件系统就绪
func (pm *PluginManager) NotifyReady() {
	pm.mu.RLock()
	plugins := make([]Plugin, len(pm.plugins))
	copy(plugins, pm.plugins)
	pm.mu.RUnlock()

	for _, p := range plugins {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("plugin OnReady panic",
						zap.String("plugin", p.Name()),
						zap.Any("panic", r),
					)
				}
			}()
			p.OnReady()
		}()
	}
}

// StopAll 停止所有插件
func (pm *PluginManager) StopAll() error {
	pm.mu.RLock()
	plugins := make([]Plugin, len(pm.plugins))
	copy(plugins, pm.plugins)
	pm.mu.RUnlock()

	var lastErr error
	// 逆序停止
	for i := len(plugins) - 1; i >= 0; i-- {
		if err := plugins[i].Stop(); err != nil {
			logger.Error("plugin stop failed",
				zap.String("plugin", plugins[i].Name()),
				zap.Error(err),
			)
			lastErr = err
		}
	}
	return lastErr
}

// RegisterMessageHandler 注册消息处理器
// 如果 aid 已被注册或是核心 aid，返回 error
func (pm *PluginManager) RegisterMessageHandler(pluginName, aid string, handler MessageHandler) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// 核心 aid 保护
	if coreAids[aid] {
		return fmt.Errorf("aid %q is a core aid, cannot be overridden by plugin %q", aid, pluginName)
	}

	// 冲突检测
	if existing, ok := pm.msgHandlers[aid]; ok {
		return fmt.Errorf("aid %q already registered by plugin %q, conflict with %q",
			aid, existing.pluginName, pluginName)
	}

	pm.msgHandlers[aid] = msgRoute{pluginName: pluginName, handler: handler}
	if logger.L != nil {
		logger.Info("plugin registered message handler",
			zap.String("plugin", pluginName),
			zap.String("aid", aid),
		)
	}
	return nil
}

// GetMessageHandler 查找 aid 对应的消息处理器
// 返回 nil 表示没有 Plugin 注册该 aid
func (pm *PluginManager) GetMessageHandler(aid string) MessageHandler {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if route, ok := pm.msgHandlers[aid]; ok {
		return route.handler
	}
	return nil
}

// RegisterDataProvider 注册数据提供者
func (pm *PluginManager) RegisterDataProvider(provider DataProvider) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.dataProviders = append(pm.dataProviders, provider)
	if logger.L != nil {
		logger.Info("plugin registered data provider", zap.String("name", provider.Name()))
	}
}

// RegisterUserDataInjector 注册用户数据注入器
func (pm *PluginManager) RegisterUserDataInjector(injector UserDataInjector) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.userDataInjectors = append(pm.userDataInjectors, injector)
	if logger.L != nil {
		logger.Info("plugin registered user data injector", zap.String("name", injector.Name()))
	}
}

// BuildPluginDataMsgs 收集所有 Plugin 的数据消息
// dumpAll: true 全量，false 增量
// 返回所有非空的数据消息列表
func (pm *PluginManager) BuildPluginDataMsgs(dumpAll bool) []string {
	pm.mu.RLock()
	providers := make([]DataProvider, len(pm.dataProviders))
	copy(providers, pm.dataProviders)
	pm.mu.RUnlock()

	var msgs []string
	for _, p := range providers {
		msg := p.BuildDataMsg(dumpAll)
		if msg != "" {
			msgs = append(msgs, msg)
		}
	}
	return msgs
}

// InjectAllTradeData 调用所有 UserDataInjector 将数据注入 tradeData 中间态
// 在 buildUserDataMsgInternal 构建 tradeData map 之后、序列化之前调用
// 返回 true 表示有任何 Plugin 注入了数据
func (pm *PluginManager) InjectAllTradeData(tradeData map[string]any, dumpAll bool) bool {
	pm.mu.RLock()
	injectors := make([]UserDataInjector, len(pm.userDataInjectors))
	copy(injectors, pm.userDataInjectors)
	pm.mu.RUnlock()

	hasChanges := false
	for _, inj := range injectors {
		if inj.InjectTradeData(tradeData, dumpAll) {
			hasChanges = true
		}
	}
	return hasChanges
}
