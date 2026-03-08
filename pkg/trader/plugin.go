// Package trader Plugin 接口定义
// 定义 Plugin、PluginContext、DataProvider、UserDataInjector 等核心接口
package trader

import (
	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/config"
	"alpha-trade-gateway/pkg/marketfeed"
	"alpha-trade-gateway/pkg/protocol"
)

// Plugin 插件接口
type Plugin interface {
	// Name 插件名称
	Name() string

	// Init 初始化插件
	// 在此方法中注册事件订阅、消息处理器、数据提供者等
	Init(ctx PluginContext) error

	// OnReady 系统就绪通知 (登录完成、数据初始化完毕)
	OnReady()

	// Stop 停止插件
	Stop() error
}

// PluginContext 插件上下文
// 提供插件所需的系统能力
type PluginContext interface {
	// EventBus 获取事件总线 (订阅事件)
	EventBus() *EventBus

	// CtpApiOps 获取 CTP API 操作集合 (发送请求)
	// 提供业务级便捷方法 (InsertOrder/CancelOrder/QryXxx...)
	// 注意: 仅在 OnReady 之后可用，Init 阶段 CTP 尚未连接
	CtpApiOps() *CtpApiOps

	// UserData 获取用户数据 (只读快照)
	UserData() *UserDataSnapshot

	// InstrumentService 获取合约信息服务
	InstrumentService() InstrumentProvider

	// MarketData 获取行情数据
	MarketData() MarketDataProvider

	// Config 获取全局配置
	Config() *config.Config

	// PluginSettings 获取当前 Plugin 的自定义配置
	// 返回 config.json 中 plugins[].settings 对应的 map
	PluginSettings() map[string]any

	// LoginInfo 获取当前登录信息 (brokerID, userName, password)
	// 仅在 OnReady 之后有效
	LoginInfo() (brokerID, userName, password string)

	// === 客户端通信 ===

	// Notify 发送通知给所有客户端连接
	Notify(code int64, msg, level string)

	// NotifyTo 发送通知给指定连接
	NotifyTo(connID int, code int64, msg, level string)

	// SendMsg 向指定连接发送任意格式的消息
	SendMsg(connID int, msg string)

	// SendMsgAll 向所有连接广播任意格式的消息
	SendMsgAll(msg string)

	// === 协议交互注册 ===

	// RegisterMessageHandler 注册客户端消息处理器
	// Plugin 在 Init 中调用，将指定 aid 类型的消息路由到 Plugin 自己的处理函数
	// 如果 aid 已被其他 Plugin 注册，返回 error
	RegisterMessageHandler(aid string, handler MessageHandler) error

	// RegisterDataProvider 注册数据提供者
	// 当 TraderCore 执行 sendAllUserData / sendUserData 时，
	// 会调用所有已注册的 DataProvider，将 Plugin 的数据帧一并推送给客户端
	RegisterDataProvider(provider DataProvider)

	// RegisterUserDataInjector 注册用户数据注入器
	// 将数据注入到 buildUserDataMsgInternal 构建的 tradeData 中间态 map 中，
	// 由 TraderCore 统一序列化为 rtn_data 的一部分推送
	// protocol.User 不需要感知 Plugin 数据，保持纯粹性
	RegisterUserDataInjector(injector UserDataInjector)

	// TriggerUserDataPush 触发一次用户数据推送
	// Plugin 在数据变更后调用，通知 TraderCore 尽快执行 sendAllUserData
	TriggerUserDataPush()

	// Logger 获取带插件名前缀的 logger
	Logger() *zap.Logger
}

// MessageHandler Plugin 消息处理函数
// connID: 客户端连接 ID
// msg: 原始 JSON 消息
type MessageHandler func(connID int, msg string)

// DataProvider Plugin 数据提供者接口
type DataProvider interface {
	// Name 数据提供者名称 (用于日志和调试)
	Name() string

	// BuildDataMsg 构建 Plugin 的数据消息
	// dumpAll: true 表示全量推送 (peek_message 场景)，false 表示增量推送
	// 返回序列化后的 JSON 字符串，如果没有数据需要推送则返回空字符串
	BuildDataMsg(dumpAll bool) string
}

// UserDataInjector Plugin 用户数据注入器接口
// Plugin 将自己的数据注入到 rtn_data 的 tradeData 中间态 map 中，
// 由 TraderCore 统一序列化推送。protocol.User 保持纯粹，不需要感知 Plugin 数据。
type UserDataInjector interface {
	// Name 注入器名称
	Name() string

	// InjectTradeData 将 Plugin 数据注入 tradeData 中间态
	// tradeData 对应 rtn_data.data[0].trade.{user_id} 这一层。
	// Plugin 直接往 tradeData 中写入自己的 key
	// dumpAll: true 注入全量数据，false 仅注入 changed 数据
	// 返回 true 表示有数据注入，false 表示无变更
	InjectTradeData(tradeData map[string]any, dumpAll bool) bool
}

// UserDataSnapshot 用户数据快照 (线程安全的只读视图)
type UserDataSnapshot struct {
	UserID     string
	TradingDay string
	Accounts   map[string]*protocol.Account
	Positions  map[string]*protocol.Position
	Orders     map[string]*protocol.Order
	Trades     map[string]*protocol.Trade
}

// InstrumentProvider 合约信息提供者接口
type InstrumentProvider interface {
	GetInstrument(symbol string) *protocol.Instrument
}

// MarketDataProvider 行情数据提供者接口
type MarketDataProvider interface {
	GetQuote(symbol string) *marketfeed.Quote
	Subscribe(symbols ...string) error
}
