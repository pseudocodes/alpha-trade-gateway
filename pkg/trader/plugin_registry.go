// Package trader Plugin 注册表
// 采用工厂注册表模式，每个 Plugin 包在 init() 中注册自己的工厂函数。
package trader

import (
	"fmt"
	"sync"
)

// PluginFactory Plugin 工厂函数
type PluginFactory func() Plugin

// pluginRegistry 全局 Plugin 注册表
var pluginRegistry = &registry{
	factories: make(map[string]PluginFactory),
}

type registry struct {
	mu        sync.Mutex
	factories map[string]PluginFactory
}

// RegisterPlugin 注册 Plugin 工厂 (由各 Plugin 包的 init() 调用)
func RegisterPlugin(name string, factory PluginFactory) {
	pluginRegistry.mu.Lock()
	defer pluginRegistry.mu.Unlock()
	if _, exists := pluginRegistry.factories[name]; exists {
		panic(fmt.Sprintf("plugin %q already registered", name))
	}
	pluginRegistry.factories[name] = factory
}

// Create 根据名称创建 Plugin 实例
func (r *registry) Create(name string) (Plugin, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	factory, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("unknown plugin: %q", name)
	}
	return factory(), nil
}
