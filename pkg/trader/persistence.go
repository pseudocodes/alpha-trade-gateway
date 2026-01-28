// Package trader 数据持久化
// 对应 C++ traderctp 中的 LoadFromFile/SaveToFile 逻辑
// Phase 2: Data Persistence
package trader

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/config"
	"alpha-trade-gateway/pkg/logger"
)

// LocalOrderKey 本地订单标识
// 对应 C++ LocalOrderKey
type LocalOrderKey struct {
	UserID  string `json:"user_id"`
	OrderID string `json:"order_id"`
}

// RemoteOrderKey CTP 订单标识
// 对应 C++ RemoteOrderKey
type RemoteOrderKey struct {
	ExchangeID   string `json:"exchange_id"`
	InstrumentID string `json:"instrument_id"`
	FrontID      int32  `json:"front_id"`
	SessionID    int32  `json:"session_id"`
	OrderRef     string `json:"order_ref"`
	OrderSysID   string `json:"order_sys_id"`
}

// OrderKeyPair 订单 Key 映射对
type OrderKeyPair struct {
	LocalKey  LocalOrderKey  `json:"local_key"`
	RemoteKey RemoteOrderKey `json:"remote_key"`
}

// OrderKeyFile 订单 Key 持久化文件结构
// 对应 C++ OrderKeyFile
type OrderKeyFile struct {
	TradingDay string         `json:"trading_day"`
	Items      []OrderKeyPair `json:"items"`
}

// OrderKeyManager 订单 Key 映射管理器
type OrderKeyManager struct {
	localToRemote map[LocalOrderKey]RemoteOrderKey
	remoteToLocal map[string]LocalOrderKey // key: RemoteOrderKey.String()
	mu            sync.RWMutex
}

// NewOrderKeyManager 创建订单 Key 管理器
func NewOrderKeyManager() *OrderKeyManager {
	return &OrderKeyManager{
		localToRemote: make(map[LocalOrderKey]RemoteOrderKey),
		remoteToLocal: make(map[string]LocalOrderKey),
	}
}

// Add 添加订单 Key 映射
func (m *OrderKeyManager) Add(local LocalOrderKey, remote RemoteOrderKey) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.localToRemote[local] = remote
	m.remoteToLocal[remote.String()] = local
}

// GetRemote 根据本地 Key 获取远程 Key
func (m *OrderKeyManager) GetRemote(local LocalOrderKey) (RemoteOrderKey, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	r, ok := m.localToRemote[local]
	return r, ok
}

// GetLocal 根据远程 Key 获取本地 Key
func (m *OrderKeyManager) GetLocal(remote RemoteOrderKey) (LocalOrderKey, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	l, ok := m.remoteToLocal[remote.String()]
	return l, ok
}

// Clear 清空映射
func (m *OrderKeyManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.localToRemote = make(map[LocalOrderKey]RemoteOrderKey)
	m.remoteToLocal = make(map[string]LocalOrderKey)
}

// ToSlice 转换为切片 (用于持久化)
func (m *OrderKeyManager) ToSlice() []OrderKeyPair {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pairs := make([]OrderKeyPair, 0, len(m.localToRemote))
	for local, remote := range m.localToRemote {
		pairs = append(pairs, OrderKeyPair{
			LocalKey:  local,
			RemoteKey: remote,
		})
	}
	return pairs
}

// LoadFromSlice 从切片加载 (用于恢复)
func (m *OrderKeyManager) LoadFromSlice(pairs []OrderKeyPair) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.localToRemote = make(map[LocalOrderKey]RemoteOrderKey, len(pairs))
	m.remoteToLocal = make(map[string]LocalOrderKey, len(pairs))

	for _, pair := range pairs {
		m.localToRemote[pair.LocalKey] = pair.RemoteKey
		m.remoteToLocal[pair.RemoteKey.String()] = pair.LocalKey
	}
}

// String RemoteOrderKey 转字符串
func (k RemoteOrderKey) String() string {
	return k.ExchangeID + "_" + k.InstrumentID + "_" +
		string(rune(k.FrontID)) + "_" + string(rune(k.SessionID)) + "_" +
		k.OrderRef + "_" + k.OrderSysID
}

// ==================== TraderCTP 持久化方法 ====================

// getOrderFilePath 获取订单映射文件路径
// 支持配置文件指定目录和自定义路径
func (t *TraderCTP) getOrderFilePath() string {
	// 默认路径
	basePath := "./data/ctp_flow/"

	// 优先使用配置文件指定的路径
	if config.Global != nil && config.Global.CTP.FlowPath != "" {
		basePath = config.Global.CTP.FlowPath
	}

	// 确保目录存在
	if err := os.MkdirAll(basePath, 0755); err != nil {
		logger.Warn("failed to create persistence directory", 
			zap.String("path", basePath),
			zap.Error(err))
	}

	// 用户特定的文件名
	userID := ""
	if t.user != nil {
		userID = t.user.UserID
	} else if t.reqLogin != nil {
		userID = t.reqLogin.UserName
	}

	if userID == "" {
		userID = "default"
	}

	return filepath.Join(basePath, userID+"_orderkey.json")
}

// saveToFile 保存订单 Key 映射到文件
// 对应 C++ traderctp::SaveToFile
func (t *TraderCTP) saveToFile() {
	if t.orderKeyManager == nil {
		return
	}

	filePath := t.getOrderFilePath()

	// 获取交易日
	tradingDay := ""
	if t.user != nil {
		tradingDay = t.user.TradingDay
	}

	// 构建文件内容
	file := OrderKeyFile{
		TradingDay: tradingDay,
		Items:      t.orderKeyManager.ToSlice(),
	}

	// 序列化
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		logger.Error("failed to marshal order key file",
			zap.Error(err))
		return
	}

	// 写入文件
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		logger.Error("failed to write order key file",
			zap.String("path", filePath),
			zap.Error(err))
		return
	}

	logger.Info("order key file saved",
		zap.String("path", filePath),
		zap.Int("count", len(file.Items)))
}

// loadFromFile 从文件加载订单 Key 映射
// 对应 C++ traderctp::LoadFromFile
func (t *TraderCTP) loadFromFile() {
	filePath := t.getOrderFilePath()

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		logger.Debug("order key file not found, starting fresh",
			zap.String("path", filePath))
		return
	}

	// 读取文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		logger.Warn("failed to read order key file",
			zap.String("path", filePath),
			zap.Error(err))
		return
	}

	// 反序列化
	var file OrderKeyFile
	if err := json.Unmarshal(data, &file); err != nil {
		logger.Warn("failed to unmarshal order key file",
			zap.String("path", filePath),
			zap.Error(err))
		return
	}

	// 检查交易日是否匹配
	currentTradingDay := ""
	if t.user != nil {
		currentTradingDay = t.user.TradingDay
	}

	if file.TradingDay != "" && file.TradingDay != currentTradingDay {
		logger.Info("trading day changed, clearing old order keys",
			zap.String("old_trading_day", file.TradingDay),
			zap.String("new_trading_day", currentTradingDay))
		return
	}

	// 初始化管理器
	if t.orderKeyManager == nil {
		t.orderKeyManager = NewOrderKeyManager()
	}

	// 加载映射
	t.orderKeyManager.LoadFromSlice(file.Items)

	logger.Info("order key file loaded",
		zap.String("path", filePath),
		zap.Int("count", len(file.Items)))
}

// needSaveOrderKey 标记需要保存订单映射
func (t *TraderCTP) needSaveOrderKey() {
	// 使用 needReset 标志复用，或者可以添加专门的 needSaveFile 字段
	// 这里简单实现为立即保存
	go t.saveToFile()
}
