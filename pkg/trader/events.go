// Package trader 事件类型和数据结构定义
// 事件系统是整个 Plugin 架构的基础。
// 所有 CTP SPI 回调和关键业务节点都通过事件分发。
package trader

import (
	"time"

	"github.com/pseudocodes/go2ctp/thost"

	"alpha-trade-gateway/pkg/protocol"
)

// EventType 事件类型
type EventType int

const (
	// === CTP 连接事件 ===
	EventFrontConnected    EventType = 100
	EventFrontDisconnected EventType = 101

	// === 登录认证事件 (每个事件 1:1 对应一个 CTP SPI 回调) ===
	EventRspAuthenticate          EventType = 200 // OnRspAuthenticate (成功/失败统一事件)
	EventRspUserLogin             EventType = 201 // OnRspUserLogin (成功/失败统一事件)
	EventSettlementConfirmed      EventType = 203 // OnRspSettlementInfoConfirm / OnRspQrySettlementInfoConfirm
	EventSettlementInfoReceived   EventType = 204 // OnRspQrySettlementInfo 结算单内容片段
	EventSettlementInfoComplete   EventType = 205 // OnRspQrySettlementInfo (bIsLast=true) 结算单查询完成
	EventInstrumentQueryComplete  EventType = 207 // OnRspQryInstrument (bIsLast=true) 合约查询完成

	// === 订单/成交回报事件 (来自 CTP SPI) ===
	EventOrderReturn      EventType = 300 // OnRtnOrder
	EventTradeReturn      EventType = 301 // OnRtnTrade
	EventOrderInsertError EventType = 302 // OnErrRtnOrderInsert
	EventOrderActionError EventType = 303 // OnErrRtnOrderAction

	// === 查询响应事件 ===
	EventAccountUpdated          EventType = 400
	EventPositionUpdated         EventType = 401
	EventInstrumentUpdated       EventType = 402
	EventBankUpdated             EventType = 403
	EventTransferResult          EventType = 404
	EventTransferError           EventType = 405
	EventAccountRegisterUpdated  EventType = 406
	EventTransferSerialUpdated   EventType = 407
	EventBrokerTradingParams     EventType = 408
	EventOrderQueried            EventType = 409
	EventTradeQueried            EventType = 410
	EventPasswordChanged         EventType = 411
	EventTradingNotice           EventType = 412
	EventInstrumentStatus        EventType = 413

	// === 行情事件 ===
	EventQuoteUpdate EventType = 500

	// === 业务拦截事件 (Before 前缀 = 可拦截) ===
	EventBeforeInsertOrder EventType = 600 // 下单前，Plugin 可拦截
	EventBeforeCancelOrder EventType = 601 // 撤单前，Plugin 可拦截
	EventBeforeTransfer    EventType = 602 // 转账前，Plugin 可拦截

	// === 状态变更事件 ===
	EventStateChanged      EventType = 700
	EventTradingDayChanged EventType = 701
	EventReady             EventType = 702 // 数据初始化完成，系统就绪
)

// eventTypeNames 事件类型名称映射 (用于日志)
var eventTypeNames = map[EventType]string{
	EventFrontConnected:      "FrontConnected",
	EventFrontDisconnected:   "FrontDisconnected",
	EventRspAuthenticate:     "RspAuthenticate",
	EventRspUserLogin:        "RspUserLogin",
	EventSettlementConfirmed: "SettlementConfirmed",
	EventSettlementInfoReceived:  "SettlementInfoReceived",
	EventSettlementInfoComplete:  "SettlementInfoComplete",
	EventInstrumentQueryComplete: "InstrumentQueryComplete",
	EventOrderReturn:         "OrderReturn",
	EventTradeReturn:            "TradeReturn",
	EventOrderInsertError:       "OrderInsertError",
	EventOrderActionError:       "OrderActionError",
	EventAccountUpdated:         "AccountUpdated",
	EventPositionUpdated:        "PositionUpdated",
	EventInstrumentUpdated:      "InstrumentUpdated",
	EventBankUpdated:            "BankUpdated",
	EventTransferResult:         "TransferResult",
	EventTransferError:          "TransferError",
	EventAccountRegisterUpdated: "AccountRegisterUpdated",
	EventTransferSerialUpdated:  "TransferSerialUpdated",
	EventBrokerTradingParams:    "BrokerTradingParams",
	EventOrderQueried:           "OrderQueried",
	EventTradeQueried:           "TradeQueried",
	EventPasswordChanged:        "PasswordChanged",
	EventTradingNotice:          "TradingNotice",
	EventInstrumentStatus:       "InstrumentStatus",
	EventQuoteUpdate:            "QuoteUpdate",
	EventBeforeInsertOrder:      "BeforeInsertOrder",
	EventBeforeCancelOrder:      "BeforeCancelOrder",
	EventBeforeTransfer:         "BeforeTransfer",
	EventStateChanged:           "StateChanged",
	EventTradingDayChanged:      "TradingDayChanged",
	EventReady:               "Ready",
}

// String 返回事件类型名称
func (t EventType) String() string {
	if name, ok := eventTypeNames[t]; ok {
		return name
	}
	return "Unknown"
}

// Event 事件
type Event struct {
	Type      EventType
	Timestamp time.Time
	Data      any    // 事件载荷
	Rejected  bool   // 是否被插件拒绝 (仅 Before 事件有效)
	RejectMsg string // 拒绝原因
}

// NewEvent 创建事件
func NewEvent(eventType EventType, data any) *Event {
	return &Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	}
}

// OrderReturnData 订单回报事件数据
type OrderReturnData struct {
	OrderID        string
	ExchangeID     string
	InstrumentID   string
	Direction      int64
	Offset         int64
	VolumeOriginal int
	VolumeLeft     int
	LimitPrice     float64
	Status         int // protocol.OrderStatusXxx
	StatusMsg      string
	InsertDateTime int64
	// 保留原始 CTP 结构体供高级插件使用
	Raw *thost.CThostFtdcOrderField
}

// TradeReturnData 成交回报事件数据
type TradeReturnData struct {
	TradeID       string
	OrderID       string
	ExchangeID    string
	InstrumentID  string
	Direction     int64
	Offset        int64
	Volume        int
	Price         float64
	TradeDateTime int64
	Raw           *thost.CThostFtdcTradeField
}

// InsertOrderRequest 下单请求事件数据
type InsertOrderRequest struct {
	ConnID int
	Action *protocol.ActionInsertOrder
}

// CancelOrderRequest 撤单请求事件数据
type CancelOrderRequest struct {
	ConnID  int
	OrderID string
}

// === 查询响应事件数据 ===

// AccountUpdateData 账户更新事件数据
type AccountUpdateData struct {
	Raw    *thost.CThostFtdcTradingAccountField
	IsLast bool
}

// PositionUpdateData 持仓更新事件数据
type PositionUpdateData struct {
	Raw    *thost.CThostFtdcInvestorPositionField
	IsLast bool
}

// OrderQueryData 订单查询事件数据 (初始化查询用)
type OrderQueryData struct {
	Raw    *thost.CThostFtdcOrderField
	IsLast bool
}

// TradeQueryData 成交查询事件数据 (初始化查询用)
type TradeQueryData struct {
	Raw    *thost.CThostFtdcTradeField
	IsLast bool
}

// === 转账事件数据 ===

// TransferResultData 转账结果事件数据
type TransferResultData struct {
	Raw             *thost.CThostFtdcRspTransferField
	IsBankToFuture  bool
	ErrorID         int
	ErrorMsg        string
}

// TransferErrorData 转账错误事件数据
type TransferErrorData struct {
	Raw             *thost.CThostFtdcReqTransferField
	IsBankToFuture  bool
	ErrorID         int
	ErrorMsg        string
}

// === 银行/签约查询事件数据 ===

// BankUpdateData 银行信息更新事件数据
type BankUpdateData struct {
	Raw    *thost.CThostFtdcContractBankField
	IsLast bool
}

// AccountRegisterData 签约关系更新事件数据
type AccountRegisterData struct {
	Raw    *thost.CThostFtdcAccountregisterField
	IsLast bool
}

// TransferSerialData 转账流水事件数据
type TransferSerialData struct {
	Raw    *thost.CThostFtdcTransferSerialField
	IsLast bool
}

// BrokerTradingParamsData 经纪商交易参数事件数据
type BrokerTradingParamsData struct {
	Raw    *thost.CThostFtdcBrokerTradingParamsField
	IsLast bool
}

// === 连接/登录事件数据 ===

// FrontConnectedData 前置连接事件数据
type FrontConnectedData struct {
	IsReconnect bool // true=重连, false=首次连接
}

// FrontDisconnectedData 前置断开事件数据
type FrontDisconnectedData struct {
	Reason int
}

// RspAuthenticateData 认证响应事件数据 (对应 OnRspAuthenticate)
// ErrorID==0 表示成功，否则表示失败
type RspAuthenticateData struct {
	ErrorID  int
	ErrorMsg string
}

// RspUserLoginData 登录响应事件数据 (对应 OnRspUserLogin)
// ErrorID==0 表示成功，否则表示失败
type RspUserLoginData struct {
	FrontID     int
	SessionID   int
	MaxOrderRef int
	TradingDay  string
	ErrorID     int
	ErrorMsg    string
}

// SettlementInfoData 结算单事件数据
type SettlementInfoData struct {
	Content string
	IsLast  bool
}

// SettlementConfirmData 结算单确认事件数据
type SettlementConfirmData struct {
	ConfirmDate string
	ConfirmTime string
}

// InstrumentQueryCompleteData 合约查询完成事件数据
type InstrumentQueryCompleteData struct{}

// === 通知事件数据 ===

// TradingNoticeData 交易通知事件数据
type TradingNoticeData struct {
	Content string
}

// InstrumentStatusData 合约状态事件数据
type InstrumentStatusData struct {
	ExchangeID   string
	InstrumentID string
	Status       byte
}

// PasswordChangeData 密码修改结果事件数据
type PasswordChangeData struct {
	IsSuccess bool
	ErrorID   int
	ErrorMsg  string
	IsTrading bool // true=资金密码, false=登录密码
}

// OrderInsertErrorData 下单错误回报事件数据
type OrderInsertErrorData struct {
	Raw      *thost.CThostFtdcInputOrderField
	ErrorID  int
	ErrorMsg string
}

// OrderActionErrorData 撤单错误回报事件数据
type OrderActionErrorData struct {
	Raw      *thost.CThostFtdcOrderActionField
	ErrorID  int
	ErrorMsg string
}
