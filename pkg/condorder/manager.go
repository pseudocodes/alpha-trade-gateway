// Package condorder 条件单管理器
package condorder

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/protocol"
)

// Callback 条件单回调接口
type Callback interface {
	// OnTouchConditionOrder 条件单触发时回调，由 TraderCTP 实现
	OnTouchConditionOrder(order *ConditionOrder)

	// OnUserDataChange 用户数据变更通知
	OnUserDataChange()

	// OutputNotify 发送通知消息
	OutputNotify(code int, msg, level, msgType string)

	// GetInstrument 获取合约信息（用于价格检测）
	GetInstrument(symbol string) *protocol.Instrument
}

// Manager 条件单管理器接口
type Manager interface {
	Load(brokerID, userID, password, tradingDay string) error
	Close() error
	InsertConditionOrder(msg string) error
	CancelConditionOrder(msg string) error
	PauseConditionOrder(msg string) error
	ResumeConditionOrder(msg string) error
	OnCheckPrice()
	OnCheckTime()
	OnMarketOpen(symbol string, status InstrumentStatus)
	OnUpdateInstrumentStatus(info *InstrumentStatusInfo)
	SetExchangeTime(localTime, shfeTime, dceTime, ineTime, ffexTime, czceTime int64)
	GetConditionOrders() map[string]*ConditionOrder
	QueryHistoryConditionOrders(msg string) ([]*ConditionOrder, error)
	NotifyPasswordUpdate(oldPassword, newPassword string)
}

// ConditionOrderManager 条件单管理器实现
type ConditionOrderManager struct {
	mu sync.RWMutex

	userKey   string
	data      *ConditionOrderData
	hisData   *ConditionOrderHisData
	storage   *Storage
	index     *ConditionOrderIndex
	checker   *ConditionChecker
	validator *Validator
	callback  Callback

	running                bool
	currentDayOrderCount   int
	currentValidOrderCount int
	config                 *Config
}

// NewManager 创建条件单管理器
func NewManager(userKey string, callback Callback, config *Config) *ConditionOrderManager {
	m := &ConditionOrderManager{
		userKey:  userKey,
		callback: callback,
		data:     NewConditionOrderData(),
		hisData:  NewConditionOrderHisData(),
		running:  true,
		config:   config,
	}

	m.storage = NewStorage(config.DataPath)
	m.index = NewConditionOrderIndex()
	m.checker = NewConditionChecker()
	m.validator = NewValidator(callback, config)
	m.validator.SetChecker(m.checker) // 共享检测器

	return m
}

// Load 加载条件单数据
func (m *ConditionOrderManager) Load(brokerID, userID, password, tradingDay string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 加载当前条件单数据
	if err := m.storage.LoadCurrent(m.userKey, m.data); err != nil {
		logger.Warn("load condition order data failed, initializing empty",
			zap.String("user_key", m.userKey),
			zap.Error(err))
		m.initEmptyData(brokerID, userID, password, tradingDay)
	} else {
		// 交易日切换处理
		if m.data.TradingDay != tradingDay {
			m.handleTradingDayChange(tradingDay)
		}
	}

	// 加载历史条件单数据
	if err := m.storage.LoadHistory(m.userKey, m.hisData); err != nil {
		logger.Warn("load history condition order data failed",
			zap.String("user_key", m.userKey),
			zap.Error(err))
		m.hisData = &ConditionOrderHisData{
			BrokerID:     brokerID,
			UserID:       userID,
			UserPassword: password,
			TradingDay:   tradingDay,
		}
	}

	// 构建索引
	m.rebuildIndex()

	// 统计当前有效条件单数量
	m.countValidOrders()

	logger.Info("condition order manager loaded",
		zap.String("user_key", m.userKey),
		zap.Int("order_count", len(m.data.ConditionOrders)),
		zap.Int("history_count", len(m.hisData.HisConditionOrders)))

	return nil
}

// Close 关闭管理器
func (m *ConditionOrderManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.running = false
	return nil
}

// InsertConditionOrder 插入条件单
func (m *ConditionOrderManager) InsertConditionOrder(msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		m.callback.OutputNotify(ErrCodeServiceStopped,
			GetErrorMessage(ErrCodeServiceStopped), "WARNING", "MESSAGE")
		return ErrServiceStopped
	}

	// 解析请求
	var req ReqInsertConditionOrder
	if err := json.Unmarshal([]byte(msg), &req); err != nil {
		return fmt.Errorf("parse insert request failed: %w", err)
	}

	// 生成订单ID
	if req.OrderID == "" {
		req.OrderID = generateOrderID()
	}

	// 检查订单ID是否重复
	if _, exists := m.data.ConditionOrders[req.OrderID]; exists {
		m.callback.OutputNotify(ErrCodeOrderIDDuplicate,
			GetErrorMessage(ErrCodeOrderIDDuplicate), "WARNING", "MESSAGE")
		return ErrOrderIDDuplicate
	}

	// 检查用户名
	if !checkUserID(req.UserID, m.data.UserID) {
		m.callback.OutputNotify(ErrCodeInvalidUserID,
			GetErrorMessage(ErrCodeInvalidUserID), "WARNING", "MESSAGE")
		return ErrInvalidUserID
	}

	// 构建条件单
	order := &ConditionOrder{
		OrderID:               req.OrderID,
		TradingDay:            parseTradingDay(m.data.TradingDay),
		InsertDateTime:        time.Now().Unix(),
		ConditionList:         req.ConditionList,
		ConditionsLogicOper:   req.ConditionsLogicOper,
		OrderList:             req.OrderList,
		TimeConditionType:     req.TimeConditionType,
		GTDDate:               req.GTDDate,
		IsCancelOriCloseOrder: req.IsCancelOriCloseOrder,
		Status:                StatusLive,
		Changed:               true,
	}

	// 验证条件单
	if err := m.validator.Validate(order, m.currentDayOrderCount, m.currentValidOrderCount); err != nil {
		order.Status = StatusDiscard
		logger.Warn("condition order validation failed",
			zap.String("order_id", order.OrderID),
			zap.Error(err))
		return err
	}

	// 保存条件单
	m.data.ConditionOrders[order.OrderID] = order
	m.currentDayOrderCount++
	m.currentValidOrderCount++

	// 持久化
	if err := m.storage.SaveCurrent(m.userKey, m.data); err != nil {
		logger.Error("save condition order failed", zap.Error(err))
	}

	// 更新索引
	m.rebuildIndex()

	// 通知
	m.callback.OutputNotify(ErrCodeInsertSuccess,
		GetErrorMessage(ErrCodeInsertSuccess), "INFO", "MESSAGE")
	m.callback.OnUserDataChange()

	logger.Info("condition order inserted",
		zap.String("order_id", order.OrderID),
		zap.Int("condition_count", len(order.ConditionList)))

	return nil
}

// CancelConditionOrder 撤销条件单
func (m *ConditionOrderManager) CancelConditionOrder(msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		m.callback.OutputNotify(ErrCodeCancelServiceStopped,
			GetErrorMessage(ErrCodeCancelServiceStopped), "WARNING", "MESSAGE")
		return ErrServiceStopped
	}

	// 解析请求
	var req ReqCancelConditionOrder
	if err := json.Unmarshal([]byte(msg), &req); err != nil {
		return fmt.Errorf("parse cancel request failed: %w", err)
	}

	// 检查用户名
	if !checkUserID(req.UserID, m.data.UserID) {
		m.callback.OutputNotify(ErrCodeCancelInvalidUserID,
			GetErrorMessage(ErrCodeCancelInvalidUserID), "WARNING", "MESSAGE")
		return ErrInvalidUserID
	}

	// 查找订单
	order, exists := m.data.ConditionOrders[req.OrderID]
	if !exists {
		m.callback.OutputNotify(ErrCodeOrderNotFound,
			GetErrorMessage(ErrCodeOrderNotFound), "WARNING", "MESSAGE")
		return ErrOrderNotFound
	}

	// 检查状态
	switch order.Status {
	case StatusTouched:
		m.callback.OutputNotify(ErrCodeOrderAlreadyTouched,
			GetErrorMessage(ErrCodeOrderAlreadyTouched), "WARNING", "MESSAGE")
		return ErrOrderAlreadyTouched
	case StatusCancel:
		m.callback.OutputNotify(ErrCodeOrderAlreadyCanceled,
			GetErrorMessage(ErrCodeOrderAlreadyCanceled), "WARNING", "MESSAGE")
		return ErrOrderAlreadyCanceled
	case StatusDiscard:
		m.callback.OutputNotify(ErrCodeOrderAlreadyDiscarded,
			GetErrorMessage(ErrCodeOrderAlreadyDiscarded), "WARNING", "MESSAGE")
		return ErrOrderAlreadyDiscarded
	}

	// 更新状态
	order.Status = StatusCancel
	order.TouchedTime = time.Now().Unix()
	order.Changed = true

	m.currentValidOrderCount--

	// 持久化
	if err := m.storage.SaveCurrent(m.userKey, m.data); err != nil {
		logger.Error("save condition order failed", zap.Error(err))
	}

	// 更新索引
	m.rebuildIndex()

	// 通知
	m.callback.OutputNotify(ErrCodeCancelSuccess,
		GetErrorMessage(ErrCodeCancelSuccess), "INFO", "MESSAGE")
	m.callback.OnUserDataChange()

	logger.Info("condition order canceled",
		zap.String("order_id", order.OrderID))

	return nil
}

// PauseConditionOrder 暂停条件单
func (m *ConditionOrderManager) PauseConditionOrder(msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		m.callback.OutputNotify(ErrCodePauseServiceStopped,
			GetErrorMessage(ErrCodePauseServiceStopped), "WARNING", "MESSAGE")
		return ErrServiceStopped
	}

	// 解析请求
	var req ReqPauseConditionOrder
	if err := json.Unmarshal([]byte(msg), &req); err != nil {
		return fmt.Errorf("parse pause request failed: %w", err)
	}

	// 检查用户名
	if !checkUserID(req.UserID, m.data.UserID) {
		m.callback.OutputNotify(ErrCodePauseInvalidUserID,
			GetErrorMessage(ErrCodePauseInvalidUserID), "WARNING", "MESSAGE")
		return ErrInvalidUserID
	}

	// 查找订单
	order, exists := m.data.ConditionOrders[req.OrderID]
	if !exists {
		m.callback.OutputNotify(ErrCodePauseOrderNotFound,
			GetErrorMessage(ErrCodePauseOrderNotFound), "WARNING", "MESSAGE")
		return ErrOrderNotFound
	}

	// 检查状态
	switch order.Status {
	case StatusTouched:
		m.callback.OutputNotify(ErrCodePauseAlreadyTouched,
			GetErrorMessage(ErrCodePauseAlreadyTouched), "WARNING", "MESSAGE")
		return ErrOrderAlreadyTouched
	case StatusCancel:
		m.callback.OutputNotify(ErrCodePauseAlreadyCanceled,
			GetErrorMessage(ErrCodePauseAlreadyCanceled), "WARNING", "MESSAGE")
		return ErrOrderAlreadyCanceled
	case StatusDiscard:
		m.callback.OutputNotify(ErrCodePauseAlreadyDiscarded,
			GetErrorMessage(ErrCodePauseAlreadyDiscarded), "WARNING", "MESSAGE")
		return ErrOrderAlreadyDiscarded
	case StatusSuspend:
		m.callback.OutputNotify(ErrCodeOrderAlreadySuspended,
			GetErrorMessage(ErrCodeOrderAlreadySuspended), "WARNING", "MESSAGE")
		return ErrOrderAlreadySuspended
	}

	// 更新状态
	order.Status = StatusSuspend
	order.TouchedTime = time.Now().Unix()
	order.Changed = true

	// 持久化
	if err := m.storage.SaveCurrent(m.userKey, m.data); err != nil {
		logger.Error("save condition order failed", zap.Error(err))
	}

	// 更新索引
	m.rebuildIndex()

	// 通知
	m.callback.OutputNotify(ErrCodePauseSuccess,
		GetErrorMessage(ErrCodePauseSuccess), "INFO", "MESSAGE")
	m.callback.OnUserDataChange()

	logger.Info("condition order paused",
		zap.String("order_id", order.OrderID))

	return nil
}

// ResumeConditionOrder 恢复条件单
func (m *ConditionOrderManager) ResumeConditionOrder(msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		m.callback.OutputNotify(ErrCodeResumeServiceStopped,
			GetErrorMessage(ErrCodeResumeServiceStopped), "WARNING", "MESSAGE")
		return ErrServiceStopped
	}

	// 解析请求
	var req ReqResumeConditionOrder
	if err := json.Unmarshal([]byte(msg), &req); err != nil {
		return fmt.Errorf("parse resume request failed: %w", err)
	}

	// 检查用户名
	if !checkUserID(req.UserID, m.data.UserID) {
		m.callback.OutputNotify(ErrCodeResumeInvalidUserID,
			GetErrorMessage(ErrCodeResumeInvalidUserID), "WARNING", "MESSAGE")
		return ErrInvalidUserID
	}

	// 查找订单
	order, exists := m.data.ConditionOrders[req.OrderID]
	if !exists {
		m.callback.OutputNotify(ErrCodeResumeOrderNotFound,
			GetErrorMessage(ErrCodeResumeOrderNotFound), "WARNING", "MESSAGE")
		return ErrOrderNotFound
	}

	// 检查状态
	if order.Status != StatusSuspend {
		m.callback.OutputNotify(ErrCodeOrderNotSuspended,
			GetErrorMessage(ErrCodeOrderNotSuspended), "WARNING", "MESSAGE")
		return ErrOrderNotSuspended
	}

	// 更新状态
	order.Status = StatusLive
	order.TouchedTime = time.Now().Unix()
	order.Changed = true

	// 持久化
	if err := m.storage.SaveCurrent(m.userKey, m.data); err != nil {
		logger.Error("save condition order failed", zap.Error(err))
	}

	// 更新索引
	m.rebuildIndex()

	// 通知
	m.callback.OutputNotify(ErrCodeResumeSuccess,
		GetErrorMessage(ErrCodeResumeSuccess), "INFO", "MESSAGE")
	m.callback.OnUserDataChange()

	logger.Info("condition order resumed",
		zap.String("order_id", order.OrderID))

	return nil
}

// OnCheckPrice 价格检测
func (m *ConditionOrderManager) OnCheckPrice() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	changed := false

	// 遍历价格触发索引
	for symbol, orderIDs := range m.index.priceOrders {
		ins := m.callback.GetInstrument(symbol)
		if ins == nil {
			continue
		}

		for _, orderID := range orderIDs {
			order, exists := m.data.ConditionOrders[orderID]
			if !exists || order.Status != StatusLive {
				continue
			}

			// 检测每个价格条件
			for i := range order.ConditionList {
				cond := &order.ConditionList[i]
				if cond.IsTouched {
					continue
				}

				if m.checker.CheckPriceCondition(cond, ins, order) {
					cond.IsTouched = true
					changed = true
				}
			}

			// 检查是否所有条件都已满足
			if m.checker.IsAllConditionsTouched(order) {
				order.Status = StatusTouched
				order.TouchedTime = time.Now().Unix()
				order.Changed = true
				changed = true

				// 触发回调
				m.callback.OnTouchConditionOrder(order)
				m.currentValidOrderCount--
			}
		}
	}

	if changed {
		m.storage.SaveCurrent(m.userKey, m.data)
		m.rebuildIndex()
		m.callback.OnUserDataChange()
	}
}

// OnCheckTime 时间检测
func (m *ConditionOrderManager) OnCheckTime() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	changed := false

	for _, orderID := range m.index.GetTimeOrders() {
		order, exists := m.data.ConditionOrders[orderID]
		if !exists || order.Status != StatusLive {
			continue
		}

		for i := range order.ConditionList {
			cond := &order.ConditionList[i]
			if cond.ContingentType != ContingentTypeTime || cond.IsTouched {
				continue
			}

			exchangeTime := m.checker.GetExchangeTime(cond.ExchangeID)
			// 允许100秒的时间误差
			if exchangeTime >= cond.ContingentTime && exchangeTime < cond.ContingentTime+100 {
				cond.IsTouched = true
				changed = true
			}
		}

		if m.checker.IsAllConditionsTouched(order) {
			order.Status = StatusTouched
			order.TouchedTime = time.Now().Unix()
			order.Changed = true
			changed = true
			m.callback.OnTouchConditionOrder(order)
			m.currentValidOrderCount--
		}
	}

	if changed {
		m.storage.SaveCurrent(m.userKey, m.data)
		m.rebuildIndex()
		m.callback.OnUserDataChange()
	}
}

// OnMarketOpen 开盘触发
func (m *ConditionOrderManager) OnMarketOpen(symbol string, status InstrumentStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	orderIDs := m.index.GetMarketOpenOrders(symbol)
	if len(orderIDs) == 0 {
		return
	}

	changed := false

	for _, orderID := range orderIDs {
		order, exists := m.data.ConditionOrders[orderID]
		if !exists || order.Status != StatusLive {
			continue
		}

		for i := range order.ConditionList {
			cond := &order.ConditionList[i]
			if cond.ContingentType != ContingentTypeMarketOpen || cond.IsTouched {
				continue
			}

			// 组合合约在集合竞价时不触发
			if isCombinationInstrument(cond.InstrumentID) && status == InstrumentStatusAuctionOrdering {
				continue
			}

			condSymbol := cond.ExchangeID + "." + trimDigits(cond.InstrumentID)
			if condSymbol == symbol {
				cond.IsTouched = true
				changed = true
			}
		}

		if m.checker.IsAllConditionsTouched(order) {
			order.Status = StatusTouched
			order.TouchedTime = time.Now().Unix()
			order.Changed = true
			changed = true
			m.callback.OnTouchConditionOrder(order)
			m.currentValidOrderCount--
		}
	}

	if changed {
		m.storage.SaveCurrent(m.userKey, m.data)
		m.rebuildIndex()
		m.callback.OnUserDataChange()
	}
}

// OnUpdateInstrumentStatus 更新合约交易状态
func (m *ConditionOrderManager) OnUpdateInstrumentStatus(info *InstrumentStatusInfo) {
	m.checker.UpdateInstrumentStatus(info.InstrumentID, info.Status)

	// 检验状态
	if info.Status != InstrumentStatusAuctionOrdering &&
		info.Status != InstrumentStatusContinousTrading {
		return
	}

	// 如果数据未就绪，不是正常的状态切换
	if !info.IsDataReady {
		return
	}

	// 检验品种
	symbol := info.ExchangeID + "." + info.InstrumentID
	m.OnMarketOpen(symbol, info.Status)
}

// SetExchangeTime 设置各交易所时间
func (m *ConditionOrderManager) SetExchangeTime(localTime, shfeTime, dceTime, ineTime, ffexTime, czceTime int64) {
	m.checker.SetExchangeTime(localTime, shfeTime, dceTime, ineTime, ffexTime, czceTime)
}

// GetConditionOrders 获取所有条件单
func (m *ConditionOrderManager) GetConditionOrders() map[string]*ConditionOrder {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.data.ConditionOrders
}

// QueryHistoryConditionOrders 查询历史条件单
func (m *ConditionOrderManager) QueryHistoryConditionOrders(msg string) ([]*ConditionOrder, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		m.callback.OutputNotify(ErrCodeQryHisServiceStopped,
			GetErrorMessage(ErrCodeQryHisServiceStopped), "WARNING", "MESSAGE")
		return nil, ErrServiceStopped
	}

	// 解析请求
	var req QryHistoryConditionOrder
	if err := json.Unmarshal([]byte(msg), &req); err != nil {
		return nil, fmt.Errorf("parse query request failed: %w", err)
	}

	// 检查用户名
	if !checkUserID(req.UserID, m.hisData.UserID) {
		m.callback.OutputNotify(ErrCodeQryHisInvalidUserID,
			GetErrorMessage(ErrCodeQryHisInvalidUserID), "WARNING", "MESSAGE")
		return nil, ErrInvalidUserID
	}

	// 检查日期
	if req.ActionDay <= 0 {
		m.callback.OutputNotify(ErrCodeQryHisInvalidDate,
			GetErrorMessage(ErrCodeQryHisInvalidDate), "WARNING", "MESSAGE")
		return nil, fmt.Errorf("invalid action day: %d", req.ActionDay)
	}

	// 查找对应日期的条件单
	var result []*ConditionOrder
	for _, order := range m.hisData.HisConditionOrders {
		insertDay := time.Unix(order.InsertDateTime, 0).Format("20060102")
		actionDayStr := strconv.Itoa(req.ActionDay)
		if insertDay == actionDayStr {
			result = append(result, order)
		}
	}

	return result, nil
}

// NotifyPasswordUpdate 密码更新通知
func (m *ConditionOrderManager) NotifyPasswordUpdate(oldPassword, newPassword string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if oldPassword == m.data.UserPassword && newPassword != m.data.UserPassword {
		m.data.UserPassword = newPassword
		m.storage.SaveCurrent(m.userKey, m.data)
		logger.Info("condition order password updated",
			zap.String("user_key", m.userKey))
	}
}

// 私有方法

// initEmptyData 初始化空数据
func (m *ConditionOrderManager) initEmptyData(brokerID, userID, password, tradingDay string) {
	m.data = &ConditionOrderData{
		BrokerID:        brokerID,
		UserID:          userID,
		UserPassword:    password,
		TradingDay:      tradingDay,
		ConditionOrders: make(map[string]*ConditionOrder),
	}
}

// handleTradingDayChange 处理交易日切换
func (m *ConditionOrderManager) handleTradingDayChange(newTradingDay string) {
	// 将旧条件单移到历史
	for _, order := range m.data.ConditionOrders {
		if order.Status == StatusLive || order.Status == StatusSuspend {
			// 未触发的条件单根据有效期类型处理
			switch order.TimeConditionType {
			case TimeConditionGFD:
				// 当日有效，过期
				order.Status = StatusDiscard
				order.Changed = true
			case TimeConditionGTD:
				// 检查是否超过GTD日期
				newDay := parseTradingDay(newTradingDay)
				if newDay > order.GTDDate {
					order.Status = StatusDiscard
					order.Changed = true
				}
			case TimeConditionGTC:
				// 撤销前有效，保留
			}
		}
		m.hisData.HisConditionOrders = append(m.hisData.HisConditionOrders, order)
	}

	// 清空当前条件单
	m.data.ConditionOrders = make(map[string]*ConditionOrder)
	m.data.TradingDay = newTradingDay
	m.currentDayOrderCount = 0
	m.currentValidOrderCount = 0

	// 保存
	m.storage.SaveCurrent(m.userKey, m.data)
	m.storage.SaveHistory(m.userKey, m.hisData)

	logger.Info("trading day changed",
		zap.String("new_trading_day", newTradingDay))
}

// rebuildIndex 重建条件单索引
func (m *ConditionOrderManager) rebuildIndex() {
	m.index.Clear()

	for orderID, order := range m.data.ConditionOrders {
		if order.Status != StatusLive {
			continue
		}

		for _, cond := range order.ConditionList {
			if cond.IsTouched {
				continue
			}

			switch cond.ContingentType {
			case ContingentTypeMarketOpen:
				symbol := cond.ExchangeID + "." + trimDigits(cond.InstrumentID)
				m.index.AddMarketOpenOrder(symbol, orderID)
			case ContingentTypeTime:
				m.index.AddTimeOrder(orderID)
			case ContingentTypePrice, ContingentTypePriceRange, ContingentTypeBreakEven:
				symbol := cond.ExchangeID + "." + cond.InstrumentID
				m.index.AddPriceOrder(symbol, orderID)
			}
		}
	}
}

// countValidOrders 统计当前有效条件单数量
func (m *ConditionOrderManager) countValidOrders() {
	m.currentDayOrderCount = 0
	m.currentValidOrderCount = 0

	currentDay := parseTradingDay(m.data.TradingDay)

	for _, order := range m.data.ConditionOrders {
		if order.TradingDay == currentDay {
			m.currentDayOrderCount++
		}
		if order.Status == StatusLive || order.Status == StatusSuspend {
			m.currentValidOrderCount++
		}
	}
}

// 辅助函数

// generateOrderID 生成订单ID
func generateOrderID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

// checkUserID 检查用户ID是否匹配
func checkUserID(reqUserID, dataUserID string) bool {
	return strings.HasPrefix(reqUserID, dataUserID)
}

// parseTradingDay 解析交易日字符串为整数
func parseTradingDay(tradingDay string) int {
	day, _ := strconv.Atoi(tradingDay)
	return day
}
