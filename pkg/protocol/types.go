// Package protocol 基础类型定义
// 对应 C++ open-trade-common/types.h
package protocol

import "math"

// 期权类型常量
const (
	OptionClassCall = 1
	OptionClassPut  = 2
)

// 产品类型常量
const (
	ProductClassAll              = 0xFFFFFFFF
	ProductClassFutures          = 0x00000001
	ProductClassOptions          = 0x00000002
	ProductClassCombination      = 0x00000004
	ProductClassFOption          = 0x00000008
	ProductClassFutureIndex      = 0x00000010
	ProductClassFutureContinuous = 0x00000020
	ProductClassStock            = 0x00000040
	ProductClassFuturePack       = ProductClassFutures | ProductClassFutureIndex | ProductClassFutureContinuous
)

// 交易所常量
const (
	ExchangeShfe  = 0x00000001
	ExchangeCffex = 0x00000002
	ExchangeCzce  = 0x00000004
	ExchangeDce   = 0x00000008
	ExchangeIne   = 0x00000010
	ExchangeKq    = 0x00000020
	ExchangeSswe  = 0x00000040
	ExchangeUser  = 0x10000000
	ExchangeAll   = 0xFFFFFFFF
)

// 通知类型常量
const (
	NotifyTypeMessage = 1
	NotifyTypeText    = 2
)

// 订单类型常量
const (
	OrderTypeTrade   = 1
	OrderTypeSwap    = 2
	OrderTypeExecute = 3
	OrderTypeQuote   = 4
)

// 止盈止损类型
const (
	TradeTypeTakeProfit = 1
	TradeTypeStopLoss   = 2
)

// 方向常量
const (
	DirectionBuy     = 1
	DirectionSell    = -1
	DirectionUnknown = 0
)

// 开平常量
const (
	OffsetOpen       = 1
	OffsetClose      = -1
	OffsetCloseToday = -2
	OffsetUnknown    = 0
)

// 订单状态常量
const (
	OrderStatusAlive    = 1
	OrderStatusFinished = 2
)

// 价格类型常量
const (
	PriceTypeUnknown   = 0
	PriceTypeLimit     = 1
	PriceTypeAny       = 2
	PriceTypeBest      = 3
	PriceTypeFiveLevel = 4
)

// 成交量条件常量
const (
	OrderVolumeConditionAny = 1
	OrderVolumeConditionMin = 2
	OrderVolumeConditionAll = 3
)

// 有效期类型常量
const (
	OrderTimeConditionIOC = 1
	OrderTimeConditionGFS = 2
	OrderTimeConditionGFD = 3
	OrderTimeConditionGTD = 4
	OrderTimeConditionGTC = 5
	OrderTimeConditionGFA = 6
)

// 投机套保标志常量
const (
	HedgeFlagSpeculation = 1
	HedgeFlagArbitrage   = 2
	HedgeFlagHedge       = 3
	HedgeFlagMarketMaker = 4
)

// 触发条件常量
// 对应 C++ ctp_define.cpp:119-124 的 contingent_condition 枚举
const (
	ContingentConditionImmediately = 1
	ContingentConditionTouch       = 2
	ContingentConditionTouchProfit = 3
)

// 协议相关常量
const (
	UserProductInfoName = "SHINNYOTG"
)

// NaN 返回 NaN 值
func NaN() float64 {
	return math.NaN()
}

// Instrument 合约信息
type Instrument struct {
	Expired        bool    `json:"expired"`
	ProductClass   int64   `json:"product_class"`
	VolumeMultiple int64   `json:"volume_multiple"`
	Volume         int64   `json:"volume"`
	Margin         float64 `json:"margin"`
	Commission     float64 `json:"commission"`
	PriceTick      float64 `json:"price_tick"`
	LastPrice      float64 `json:"last_price"`
	PreSettlement  float64 `json:"pre_settlement"`
	UpperLimit     float64 `json:"upper_limit"`
	LowerLimit     float64 `json:"lower_limit"`
	AskPrice1      float64 `json:"ask_price1"`
	BidPrice1      float64 `json:"bid_price1"`
	Settlement     float64 `json:"settlement"`
	PreClose       float64 `json:"pre_close"`
	InstrumentID   string  `json:"instrument_id"`
	ExchangeID     string  `json:"exchange_id"`
}

// NewInstrument 创建新的 Instrument 实例，使用 NaN 初始化价格字段
func NewInstrument() *Instrument {
	return &Instrument{
		Expired:       false,
		ProductClass:  ProductClassFutures,
		PriceTick:     NaN(),
		LastPrice:     NaN(),
		PreSettlement: NaN(),
		UpperLimit:    NaN(),
		LowerLimit:    NaN(),
		AskPrice1:     NaN(),
		BidPrice1:     NaN(),
		Settlement:    NaN(),
		PreClose:      NaN(),
	}
}

// Order 委托单
type Order struct {
	// 委托单初始属性(由下单者在下单前确定, 不再改变)
	UserID       string `json:"user_id"`
	OrderID      string `json:"order_id"`
	ExchangeID   string `json:"exchange_id"`
	InstrumentID string `json:"instrument_id"`

	Direction       int64   `json:"direction"`
	Offset          int64   `json:"offset"`
	VolumeOrign     int     `json:"volume_orign"`
	PriceType       int64   `json:"price_type"`
	LimitPrice      float64 `json:"limit_price"`
	TimeCondition   int64   `json:"time_condition"`
	VolumeCondition int64   `json:"volume_condition"`

	// 下单后获得的信息(由期货公司返回, 不会改变)
	InsertDateTime  int64   `json:"insert_date_time"`
	ExchangeOrderID string  `json:"exchange_order_id"`
	FrozenMargin    float64 `json:"frozen_margin"`

	// 委托单当前状态
	Status     int64  `json:"status"`
	VolumeLeft int    `json:"volume_left"`
	LastMsg    string `json:"last_msg"`

	// 内部使用
	Seqno   int  `json:"-"`
	Changed bool `json:"-"`
}

// Symbol 返回合约代码
func (o *Order) Symbol() string {
	return o.ExchangeID + "." + o.InstrumentID
}

// NewOrder 创建新的 Order 实例
func NewOrder() *Order {
	return &Order{
		Direction:       DirectionBuy,
		Offset:          OffsetOpen,
		VolumeOrign:     0,
		PriceType:       PriceTypeLimit,
		LimitPrice:      0.0,
		VolumeCondition: OrderVolumeConditionAny,
		TimeCondition:   OrderTimeConditionGFD,
		InsertDateTime:  0,
		FrozenMargin:    0.0,
		Status:          OrderStatusAlive,
		VolumeLeft:      0,
		Changed:         true,
	}
}

// Trade 成交
type Trade struct {
	UserID          string `json:"user_id"`
	TradeID         string `json:"trade_id"`
	ExchangeID      string `json:"exchange_id"`
	InstrumentID    string `json:"instrument_id"`
	OrderID         string `json:"order_id"`
	ExchangeTradeID string `json:"exchange_trade_id"`

	Direction     int     `json:"direction"`
	Offset        int     `json:"offset"`
	Volume        int     `json:"volume"`
	Price         float64 `json:"price"`
	TradeDateTime int64   `json:"trade_date_time"` // epoch nano
	Commission    float64 `json:"commission"`

	// 内部使用
	Seqno   int  `json:"-"`
	Changed bool `json:"-"`
}

// Symbol 返回合约代码
func (t *Trade) Symbol() string {
	return t.ExchangeID + "." + t.InstrumentID
}

// NewTrade 创建新的 Trade 实例
func NewTrade() *Trade {
	return &Trade{
		Direction: DirectionBuy,
		Offset:    OffsetOpen,
		Changed:   true,
	}
}

// Position 持仓
type Position struct {
	// 交易所和合约代码
	UserID       string `json:"user_id"`
	ExchangeID   string `json:"exchange_id"`
	InstrumentID string `json:"instrument_id"`

	// 多头持仓手数
	VolumeLongToday int `json:"volume_long_today"`
	VolumeLongHis   int `json:"volume_long_his"`
	VolumeLong      int `json:"volume_long"`

	// 多头冻结手数
	VolumeLongFrozenToday int `json:"volume_long_frozen_today"`
	VolumeLongFrozenHis   int `json:"volume_long_frozen_his"`
	VolumeLongFrozen      int `json:"volume_long_frozen"`

	// 空头持仓手数
	VolumeShortToday int `json:"volume_short_today"`
	VolumeShortHis   int `json:"volume_short_his"`
	VolumeShort      int `json:"volume_short"`

	// 空头冻结手数
	VolumeShortFrozenToday int `json:"volume_short_frozen_today"`
	VolumeShortFrozenHis   int `json:"volume_short_frozen_his"`
	VolumeShortFrozen      int `json:"volume_short_frozen"`

	// 今日开盘前的双向持仓手数
	VolumeLongYd  int `json:"volume_long_yd"`
	VolumeShortYd int `json:"volume_short_yd"`

	// 今昨仓持仓情况
	PosLongHis    int `json:"pos_long_his"`
	PosLongToday  int `json:"pos_long_today"`
	PosShortHis   int `json:"pos_short_his"`
	PosShortToday int `json:"pos_short_today"`

	// 成本, 现价与盈亏
	OpenPriceLong  float64 `json:"open_price_long"`
	OpenPriceShort float64 `json:"open_price_short"`
	OpenCostLong   float64 `json:"open_cost_long"`
	OpenCostShort  float64 `json:"open_cost_short"`

	OpenCostLongHis    float64 `json:"open_cost_long_his"`
	OpenCostLongToday  float64 `json:"open_cost_long_today"`
	OpenCostShortToday float64 `json:"open_cost_short_today"`
	OpenCostShortHis   float64 `json:"open_cost_short_his"`

	PositionPriceLong  float64 `json:"position_price_long"`
	PositionPriceShort float64 `json:"position_price_short"`
	PositionCostLong   float64 `json:"position_cost_long"`
	PositionCostShort  float64 `json:"position_cost_short"`

	PositionCostLongToday  float64 `json:"position_cost_long_today"`
	PositionCostLongHis    float64 `json:"position_cost_long_his"`
	PositionCostShortToday float64 `json:"position_cost_short_today"`
	PositionCostShortHis   float64 `json:"position_cost_short_his"`

	LastPrice float64 `json:"last_price"`

	FloatProfitLong  float64 `json:"float_profit_long"`
	FloatProfitShort float64 `json:"float_profit_short"`
	FloatProfit      float64 `json:"float_profit"`

	PositionProfitLong  float64 `json:"position_profit_long"`
	PositionProfitShort float64 `json:"position_profit_short"`
	PositionProfit      float64 `json:"position_profit"`

	// 保证金占用
	MarginLong       float64 `json:"margin_long"`
	MarginShort      float64 `json:"margin_short"`
	MarginLongToday  float64 `json:"margin_long_today"`
	MarginShortToday float64 `json:"margin_short_today"`
	MarginLongHis    float64 `json:"margin_long_his"`
	MarginShortHis   float64 `json:"margin_short_his"`
	Margin           float64 `json:"margin"`
	FrozenMargin     float64 `json:"frozen_margin"`

	// 内部使用
	Ins          *Instrument `json:"-"`
	Changed      bool        `json:"-"`
	MarketStatus int         `json:"-"` // 0: 开盘前; 1: 交易中; 2: 收盘后
}

// Symbol 返回合约代码
func (p *Position) Symbol() string {
	return p.ExchangeID + "." + p.InstrumentID
}

// NewPosition 创建新的 Position 实例
func NewPosition() *Position {
	return &Position{
		Changed:      true,
		MarketStatus: 1,
	}
}

// Account 账户
type Account struct {
	// 账号
	UserID   string `json:"user_id"`
	Currency string `json:"currency"`

	// 本交易日开盘前状态(昨权益)
	PreBalance float64 `json:"pre_balance"`

	// 本交易日内已发生事件的影响
	Deposit       float64 `json:"deposit"`
	Withdraw      float64 `json:"withdraw"`
	CloseProfit   float64 `json:"close_profit"`
	Commission    float64 `json:"commission"`
	Premium       float64 `json:"premium"`
	StaticBalance float64 `json:"static_balance"`

	// 当前持仓盈亏
	PositionProfit float64 `json:"position_profit"`
	FloatProfit    float64 `json:"float_profit"`

	// 当前权益(动态权益)
	Balance float64 `json:"balance"`

	// 市值权益
	ValueBalance float64 `json:"value_balance"`

	// 保证金占用, 冻结及风险度
	Margin           float64 `json:"margin"`
	FrozenMargin     float64 `json:"frozen_margin"`
	FrozenCommission float64 `json:"frozen_commission"`
	FrozenPremium    float64 `json:"frozen_premium"`
	Available        float64 `json:"available"`
	RiskRatio        float64 `json:"risk_ratio"`

	// 内部使用
	Changed bool `json:"-"`
}

// NewAccount 创建新的 Account 实例
func NewAccount() *Account {
	return &Account{
		Changed: true,
	}
}

// Bank 银行
type Bank struct {
	BankID      string `json:"bank_id"`
	BankName    string `json:"bank_name"`
	BankAccount string `json:"bank_account,omitempty"` // 银行账号 (Phase 3: Banking)
	Changed     bool   `json:"-"`
}

// TransferLog 转账记录
type TransferLog struct {
	DateTime  string  `json:"datetime,omitempty"`  // 转账时间字符串 (Phase 3)
	Currency  string  `json:"currency"`
	Amount    float64 `json:"amount"`
	ErrorID   int64   `json:"error_id"`
	ErrorMsg  string  `json:"error_msg"`
	Direction string  `json:"direction,omitempty"` // 入金/出金 (Phase 3)
	Changed   bool    `json:"-"`
}

// User 用户数据
type User struct {
	UserID        string                  `json:"user_id"`
	TradingDay    string                  `json:"trading_day"`
	TradeMoreData bool                    `json:"-"`
	Accounts      map[string]*Account     `json:"accounts"`
	Positions     map[string]*Position    `json:"positions"`
	Orders        map[string]*Order       `json:"orders"`
	Trades        map[string]*Trade       `json:"trades"`
	Banks         map[string]*Bank        `json:"banks"`
	Transfers     map[string]*TransferLog `json:"transfers"`
}

// NewUser 创建新的 User 实例
func NewUser(userID string) *User {
	return &User{
		UserID:    userID,
		Accounts:  make(map[string]*Account),
		Positions: make(map[string]*Position),
		Orders:    make(map[string]*Order),
		Trades:    make(map[string]*Trade),
		Banks:     make(map[string]*Bank),
		Transfers: make(map[string]*TransferLog),
	}
}

// ActionInsertOrder 下单请求
type ActionInsertOrder struct {
	OrderID             string  `json:"order_id"`
	UserID              string  `json:"user_id"`
	ExchangeID          string  `json:"exchange_id"`
	InstrumentID        string  `json:"ins_id"`
	Direction           int64   `json:"direction"`
	Offset              int64   `json:"offset"`
	Volume              int     `json:"volume"`
	PriceType           int64   `json:"price_type"`
	LimitPrice          float64 `json:"limit_price"`
	VolumeCondition     int64   `json:"volume_condition"`
	TimeCondition       int64   `json:"time_condition"`
	HedgeFlag           int64   `json:"hedge_flag"`
	ContingentCondition int64   `json:"contingent_condition"` // 触发条件 (C++ ctp_define.cpp:119-124)
}

// ActionCancelOrder 撤单请求
type ActionCancelOrder struct {
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
}

// Notify 通知
type Notify struct {
	Type    int64  `json:"type"`
	Code    int64  `json:"code"`
	Content string `json:"content"`
}

// ReqLogin 登录请求
type ReqLogin struct {
	Aid              string `json:"aid"`
	Bid              string `json:"bid"`
	UserName         string `json:"user_name"`
	Password         string `json:"password"`
	ClientIP         string `json:"client_ip"`
	ClientPort       int    `json:"client_port"`
	ClientSystemInfo string `json:"client_system_info"`
	ClientAppID      string `json:"client_app_id"`
	// 次席支持
	BrokerID string `json:"broker_id"`
	Front    string `json:"front"`
}

// QrySettlementInfo 查询结算单请求
type QrySettlementInfo struct {
	Aid            string `json:"aid"`
	TradingDay     int    `json:"trading_day"`
	UserName       string `json:"user_name"`
	SettlementInfo string `json:"settlement_info"`
}

// BrokerLogin 登录时的经纪商配置 (用户可以强制输入)
type BrokerLogin struct {
	BrokerName    string   `json:"name,omitempty"`
	BrokerType    string   `json:"type,omitempty"`
	IsFens        bool     `json:"is_fens,omitempty"`
	CtpBrokerID   string   `json:"broker_id,omitempty"`
	TradingFronts []string `json:"trading_fronts,omitempty"`
	ProductInfo   string   `json:"product_info,omitempty"`
	AuthCode      string   `json:"auth_code,omitempty"`
}
