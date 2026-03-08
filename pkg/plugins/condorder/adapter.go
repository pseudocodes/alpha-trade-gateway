// Package condorder CondOrderAdapter 适配器
// 将现有 pkg/condorder 包装为 Plugin 可用的形式
// 实现 condorder.Callback 接口，将回调转换为 PluginContext 操作
package condorder

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/condorder"
	"alpha-trade-gateway/pkg/config"
	"alpha-trade-gateway/pkg/protocol"
	"alpha-trade-gateway/pkg/trader"
)

// CondOrderAdapter 条件单适配器
// 封装 condorder.Manager，实现 condorder.Callback 接口
type CondOrderAdapter struct {
	ctx     trader.PluginContext
	manager condorder.Manager
	logger  *zap.Logger

	// 定时检测 ticker
	priceTicker *time.Ticker
	timeTicker  *time.Ticker
	stopCh      chan struct{}
}

// NewCondOrderAdapter 创建条件单适配器
// 在 OnReady 时调用，此时 CTP 已连接，用户数据已初始化
func NewCondOrderAdapter(ctx trader.PluginContext) *CondOrderAdapter {
	cfg := config.Global
	if cfg == nil || !cfg.ConditionOrder.Enabled {
		ctx.Logger().Info("condition order disabled in config")
		return nil
	}

	userData := ctx.UserData()
	if userData == nil {
		ctx.Logger().Warn("condition order adapter: user data not ready")
		return nil
	}

	coConfig := &condorder.Config{
		Enabled:             cfg.ConditionOrder.Enabled,
		DataPath:            cfg.ConditionOrder.DataPath,
		MaxNewOrdersPerDay:  cfg.ConditionOrder.MaxNewOrdersPerDay,
		MaxValidOrdersTotal: cfg.ConditionOrder.MaxValidOrdersTotal,
	}

	adapter := &CondOrderAdapter{
		ctx:    ctx,
		logger: ctx.Logger(),
		stopCh: make(chan struct{}),
	}

	brokerID, userName, password := ctx.LoginInfo()

	userKey := brokerID + "." + userName
	adapter.manager = condorder.NewManager(userKey, adapter, coConfig)

	if err := adapter.manager.Load(brokerID, userName, password, userData.TradingDay); err != nil {
		ctx.Logger().Error("load condition order failed", zap.Error(err))
		return nil
	}

	ctx.Logger().Info("condition order adapter initialized",
		zap.String("user_key", userKey),
		zap.String("trading_day", userData.TradingDay))

	return adapter
}

// Start 启动价格/时间检测循环
func (a *CondOrderAdapter) Start() {
	// 价格检测: 每 200ms
	a.priceTicker = time.NewTicker(200 * time.Millisecond)
	go func() {
		for {
			select {
			case <-a.priceTicker.C:
				a.manager.OnCheckPrice()
			case <-a.stopCh:
				return
			}
		}
	}()

	// 时间检测: 每秒
	a.timeTicker = time.NewTicker(time.Second)
	go func() {
		for {
			select {
			case <-a.timeTicker.C:
				a.manager.OnCheckTime()
			case <-a.stopCh:
				return
			}
		}
	}()

	a.logger.Info("condition order checker started")
}

// Stop 停止检测循环并关闭管理器
func (a *CondOrderAdapter) Stop() {
	close(a.stopCh)
	if a.priceTicker != nil {
		a.priceTicker.Stop()
	}
	if a.timeTicker != nil {
		a.timeTicker.Stop()
	}
	if a.manager != nil {
		a.manager.Close()
	}
	a.logger.Info("condition order adapter stopped")
}

// === 委托方法 (转发到 condorder.Manager) ===

func (a *CondOrderAdapter) InsertConditionOrder(msg string) error {
	return a.manager.InsertConditionOrder(msg)
}

func (a *CondOrderAdapter) CancelConditionOrder(msg string) error {
	return a.manager.CancelConditionOrder(msg)
}

func (a *CondOrderAdapter) PauseConditionOrder(msg string) error {
	return a.manager.PauseConditionOrder(msg)
}

func (a *CondOrderAdapter) ResumeConditionOrder(msg string) error {
	return a.manager.ResumeConditionOrder(msg)
}

func (a *CondOrderAdapter) QueryHistoryConditionOrders(msg string) ([]*condorder.ConditionOrder, error) {
	return a.manager.QueryHistoryConditionOrders(msg)
}

// BuildConditionOrderMsg 构建 rtn_condition_orders 消息帧
func (a *CondOrderAdapter) BuildConditionOrderMsg(dumpAll bool) string {
	orders := a.manager.GetConditionOrders()
	if len(orders) == 0 && !dumpAll {
		return ""
	}

	var changed []*condorder.ConditionOrder
	for _, order := range orders {
		if dumpAll || order.Changed {
			changed = append(changed, order)
			order.Changed = false
		}
	}
	if len(changed) == 0 {
		return ""
	}

	// 构建 rtn_condition_orders 帧
	type rtnMsg struct {
		Aid             string                     `json:"aid"`
		ConditionOrders []*condorder.ConditionOrder `json:"condition_orders"`
	}
	msg := rtnMsg{
		Aid:             "rtn_condition_orders",
		ConditionOrders: changed,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		a.logger.Error("marshal condition orders failed", zap.Error(err))
		return ""
	}
	return string(data)
}

// === condorder.Callback 接口实现 ===

// OnTouchConditionOrder 条件单触发回调
// 通过 CtpApiOps 发送实际委托
func (a *CondOrderAdapter) OnTouchConditionOrder(order *condorder.ConditionOrder) {
	a.logger.Info("condition order touched",
		zap.String("order_id", order.OrderID),
		zap.Int("order_count", len(order.OrderList)))

	ops := a.ctx.CtpApiOps()
	if ops == nil {
		a.logger.Error("ctp api ops not available")
		return
	}

	for i := range order.OrderList {
		co := &order.OrderList[i]
		action, err := a.buildOrderAction(co)
		if err != nil {
			a.logger.Error("build order action failed",
				zap.String("order_id", order.OrderID),
				zap.Int("index", i),
				zap.Error(err))
			continue
		}
		ops.InsertOrder(action)
	}

	// 触发后推送状态变更
	a.pushData()
}

// OnUserDataChange 用户数据变更通知
func (a *CondOrderAdapter) OnUserDataChange() {
	a.pushData()
}

// OutputNotify 发送通知消息
func (a *CondOrderAdapter) OutputNotify(code int, msg, level, msgType string) {
	a.ctx.Notify(int64(code), msg, level)
}

// GetInstrument 获取合约信息 (用于价格检测)
func (a *CondOrderAdapter) GetInstrument(symbol string) *protocol.Instrument {
	insSvc := a.ctx.InstrumentService()
	if insSvc == nil {
		return nil
	}
	return insSvc.GetInstrument(symbol)
}

// === 内部方法 ===

// buildOrderAction 将条件单委托转换为 protocol.ActionInsertOrder
func (a *CondOrderAdapter) buildOrderAction(co *condorder.ContingentOrder) (*protocol.ActionInsertOrder, error) {
	symbol := co.ExchangeID + "." + co.InstrumentID

	// 获取合约信息
	ins := a.GetInstrument(symbol)
	if ins == nil {
		return nil, fmt.Errorf("instrument not found: %s", symbol)
	}

	// 计算实际价格
	var price float64
	switch co.PriceType {
	case condorder.PriceTypeLimit:
		price = co.LimitPrice
	case condorder.PriceTypeMarket, condorder.PriceTypeContingent:
		if co.Direction == condorder.OrderDirectionBuy {
			price = ins.AskPrice1
		} else {
			price = ins.BidPrice1
		}
	case condorder.PriceTypeConsideration:
		if co.Direction == condorder.OrderDirectionBuy {
			price = ins.AskPrice1
		} else {
			price = ins.BidPrice1
		}
	case condorder.PriceTypeOver:
		tick := ins.PriceTick
		if math.IsNaN(tick) || tick <= 0 {
			tick = 1.0
		}
		if co.Direction == condorder.OrderDirectionBuy {
			price = ins.AskPrice1 + tick
		} else {
			price = ins.BidPrice1 - tick
		}
	default:
		price = co.LimitPrice
	}

	// 计算实际手数
	volume := co.Volume
	if co.VolumeType == condorder.VolumeTypeCloseAll {
		userData := a.ctx.UserData()
		if userData != nil {
			if pos, ok := userData.Positions[symbol]; ok {
				if co.Direction == condorder.OrderDirectionBuy {
					volume = pos.VolumeShort
				} else {
					volume = pos.VolumeLong
				}
			} else {
				volume = 0
			}
		}
	}

	if volume <= 0 {
		return nil, fmt.Errorf("volume is zero for %s", symbol)
	}

	// 转换方向
	var direction int64
	if co.Direction == condorder.OrderDirectionBuy {
		direction = protocol.DirectionBuy
	} else {
		direction = protocol.DirectionSell
	}

	// 转换开平
	var offset int64
	switch co.Offset {
	case condorder.OrderOffsetOpen:
		offset = protocol.OffsetOpen
	case condorder.OrderOffsetClose:
		offset = protocol.OffsetClose
	default:
		offset = protocol.OffsetOpen
	}

	return &protocol.ActionInsertOrder{
		ExchangeID:   co.ExchangeID,
		InstrumentID: co.InstrumentID,
		Direction:    direction,
		Offset:       offset,
		Volume:       volume,
		PriceType:    protocol.PriceTypeLimit,
		LimitPrice:   price,
	}, nil
}

// pushData 推送条件单数据变更
func (a *CondOrderAdapter) pushData() {
	msg := a.BuildConditionOrderMsg(false)
	if msg != "" {
		a.ctx.SendMsgAll(msg)
	}
}

// OnQuoteUpdate 行情更新时触发价格检测
func (a *CondOrderAdapter) OnQuoteUpdate() {
	if a.manager != nil {
		a.manager.OnCheckPrice()
	}
}
