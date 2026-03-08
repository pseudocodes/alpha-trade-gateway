// Package condorder 条件单插件
// 将现有 pkg/condorder 包装为 Plugin
package condorder

import (
	"fmt"

	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/trader"
)

func init() {
	trader.RegisterPlugin("condorder", func() trader.Plugin {
		return &CondOrderPlugin{}
	})
}

// CondOrderPlugin 条件单插件
type CondOrderPlugin struct {
	ctx    trader.PluginContext
	logger *zap.Logger

	adapter *CondOrderAdapter
}

func (p *CondOrderPlugin) Name() string { return "condorder" }

func (p *CondOrderPlugin) Init(ctx trader.PluginContext) error {
	p.ctx = ctx
	p.logger = ctx.Logger()

	eb := ctx.EventBus()

	// 监听行情更新，用于价格条件检测
	eb.Subscribe(trader.EventQuoteUpdate, p.processQuoteUpdate, trader.PriorityNormal)

	// 监听订单回报 (关联条件单与实际订单)
	eb.Subscribe(trader.EventOrderReturn, p.processOrderReturn, trader.PriorityNormal)

	// 监听合约状态变化，用于开盘触发
	eb.Subscribe(trader.EventInstrumentStatus, p.processInstrumentStatus, trader.PriorityNormal)

	// 注册客户端消息处理
	handlers := map[string]trader.MessageHandler{
		"insert_condition_order":  p.handleInsertConditionOrder,
		"cancel_condition_order":  p.handleCancelConditionOrder,
		"pause_condition_order":   p.handlePauseConditionOrder,
		"resume_condition_order":  p.handleResumeConditionOrder,
		"qry_condition_order":     p.handleQryConditionOrder,
		"qry_his_condition_order": p.handleQryHisConditionOrder,
	}
	for aid, handler := range handlers {
		if err := ctx.RegisterMessageHandler(aid, handler); err != nil {
			return fmt.Errorf("register %s: %w", aid, err)
		}
	}

	// 注册数据提供者 (参与 peek_message 和定时推送)
	ctx.RegisterDataProvider(p)

	p.logger.Info("condorder plugin initialized")
	return nil
}

func (p *CondOrderPlugin) OnReady() {
	p.logger.Info("condorder plugin ready, initializing adapter")

	// 在系统就绪后创建 adapter (此时 CTP 已连接，用户数据已初始化)
	p.adapter = NewCondOrderAdapter(p.ctx)
	if p.adapter != nil {
		p.adapter.Start()
	}
}

func (p *CondOrderPlugin) Stop() error {
	if p.adapter != nil {
		p.adapter.Stop()
	}
	p.logger.Info("condorder plugin stopped")
	return nil
}

// === DataProvider 接口实现 ===

// BuildDataMsg 构建条件单数据消息
func (p *CondOrderPlugin) BuildDataMsg(dumpAll bool) string {
	if p.adapter == nil {
		return ""
	}
	return p.adapter.BuildConditionOrderMsg(dumpAll)
}

// === 客户端消息处理 → handle 前缀 ===

func (p *CondOrderPlugin) handleInsertConditionOrder(connID int, msg string) {
	if p.adapter == nil {
		p.ctx.NotifyTo(connID, 500, "条件单功能未就绪", "WARNING")
		return
	}
	if err := p.adapter.InsertConditionOrder(msg); err != nil {
		p.logger.Error("insert condition order failed", zap.Error(err))
		return
	}
	p.pushConditionOrderData()
}

func (p *CondOrderPlugin) handleCancelConditionOrder(connID int, msg string) {
	if p.adapter == nil {
		p.ctx.NotifyTo(connID, 500, "条件单功能未就绪", "WARNING")
		return
	}
	if err := p.adapter.CancelConditionOrder(msg); err != nil {
		p.logger.Error("cancel condition order failed", zap.Error(err))
		return
	}
	p.pushConditionOrderData()
}

func (p *CondOrderPlugin) handlePauseConditionOrder(connID int, msg string) {
	if p.adapter == nil {
		p.ctx.NotifyTo(connID, 500, "条件单功能未就绪", "WARNING")
		return
	}
	if err := p.adapter.PauseConditionOrder(msg); err != nil {
		p.logger.Error("pause condition order failed", zap.Error(err))
	}
}

func (p *CondOrderPlugin) handleResumeConditionOrder(connID int, msg string) {
	if p.adapter == nil {
		p.ctx.NotifyTo(connID, 500, "条件单功能未就绪", "WARNING")
		return
	}
	if err := p.adapter.ResumeConditionOrder(msg); err != nil {
		p.logger.Error("resume condition order failed", zap.Error(err))
	}
}

func (p *CondOrderPlugin) handleQryConditionOrder(connID int, msg string) {
	if p.adapter == nil {
		p.ctx.NotifyTo(connID, 500, "条件单功能未就绪", "WARNING")
		return
	}
	data := p.adapter.BuildConditionOrderMsg(true)
	if data != "" {
		p.ctx.SendMsg(connID, data)
	}
}

func (p *CondOrderPlugin) handleQryHisConditionOrder(connID int, msg string) {
	if p.adapter == nil {
		p.ctx.NotifyTo(connID, 500, "条件单功能未就绪", "WARNING")
		return
	}
	if _, err := p.adapter.QueryHistoryConditionOrders(msg); err != nil {
		p.logger.Error("query history condition orders failed", zap.Error(err))
	}
}

// === CTP 回报事件处理 → process 前缀 ===

func (p *CondOrderPlugin) processOrderReturn(event *trader.Event) {
	// 条件单触发后的订单回报关联
	// 当前由 adapter 内部的 condorder.Manager 处理
}

// processQuoteUpdate 行情更新时检测价格条件
func (p *CondOrderPlugin) processQuoteUpdate(event *trader.Event) {
	if p.adapter == nil {
		return
	}
	// 行情更新触发条件单价格检测
	// adapter 内部的 priceTicker 也会定时检测，这里提供事件驱动的补充
	p.adapter.OnQuoteUpdate()
}

// processInstrumentStatus 合约状态变化 (如开盘) 时通知条件单
func (p *CondOrderPlugin) processInstrumentStatus(event *trader.Event) {
	// 合约状态变化可用于开盘触发等场景
	// 当前由 adapter 内部的 timeTicker 定时检测覆盖
}

// pushConditionOrderData 条件单数据变更后即时推送
func (p *CondOrderPlugin) pushConditionOrderData() {
	msg := p.BuildDataMsg(false)
	if msg != "" {
		p.ctx.SendMsgAll(msg)
	}
}
