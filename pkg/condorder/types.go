// Package condorder 条件单管理模块
// 对应 C++ open-trade-common/condition_order_type.h
package condorder

// ContingentType 触发条件类型
type ContingentType int

const (
	ContingentTypeMarketOpen ContingentType = iota // 开盘触发
	ContingentTypeTime                             // 时间触发
	ContingentTypePrice                            // 价格触发
	ContingentTypePriceRange                       // 价格区间触发
	ContingentTypeBreakEven                        // 保本止盈触发
)

// String 返回触发类型的字符串表示
func (c ContingentType) String() string {
	switch c {
	case ContingentTypeMarketOpen:
		return "market_open"
	case ContingentTypeTime:
		return "time"
	case ContingentTypePrice:
		return "price"
	case ContingentTypePriceRange:
		return "price_range"
	case ContingentTypeBreakEven:
		return "break_even"
	default:
		return "unknown"
	}
}

// PriceRelationType 价格关系类型
type PriceRelationType int

const (
	PriceRelationGreater      PriceRelationType = iota // 大于 >
	PriceRelationGreaterEqual                          // 大于等于 >=
	PriceRelationLess                                  // 小于 <
	PriceRelationLessEqual                             // 小于等于 <=
)

// String 返回价格关系类型的字符串表示
func (p PriceRelationType) String() string {
	switch p {
	case PriceRelationGreater:
		return "G"
	case PriceRelationGreaterEqual:
		return "GE"
	case PriceRelationLess:
		return "L"
	case PriceRelationLessEqual:
		return "LE"
	default:
		return "G"
	}
}

// OrderDirection 买卖方向
type OrderDirection int

const (
	OrderDirectionBuy  OrderDirection = iota // 买
	OrderDirectionSell                       // 卖
)

// String 返回买卖方向的字符串表示
func (d OrderDirection) String() string {
	if d == OrderDirectionBuy {
		return "BUY"
	}
	return "SELL"
}

// OrderOffset 开平方向
type OrderOffset int

const (
	OrderOffsetOpen    OrderOffset = iota // 开仓
	OrderOffsetClose                      // 平仓
	OrderOffsetReverse                    // 反手
)

// String 返回开平方向的字符串表示
func (o OrderOffset) String() string {
	switch o {
	case OrderOffsetOpen:
		return "OPEN"
	case OrderOffsetClose:
		return "CLOSE"
	case OrderOffsetReverse:
		return "REVERSE"
	default:
		return "OPEN"
	}
}

// VolumeType 手数类型
type VolumeType int

const (
	VolumeTypeNum      VolumeType = iota // 具体手数
	VolumeTypeCloseAll                   // 全平
)

// String 返回手数类型的字符串表示
func (v VolumeType) String() string {
	if v == VolumeTypeCloseAll {
		return "CLOSE_ALL"
	}
	return "NUM"
}

// PriceType 价格类型
type PriceType int

const (
	PriceTypeContingent    PriceType = iota // 触发价格
	PriceTypeConsideration                  // 对价
	PriceTypeMarket                         // 市价
	PriceTypeOver                           // 超价
	PriceTypeLimit                          // 限价
)

// String 返回价格类型的字符串表示
func (p PriceType) String() string {
	switch p {
	case PriceTypeContingent:
		return "CONTINGENT"
	case PriceTypeConsideration:
		return "CONSIDERATION"
	case PriceTypeMarket:
		return "MARKET"
	case PriceTypeOver:
		return "OVER"
	case PriceTypeLimit:
		return "LIMIT"
	default:
		return "LIMIT"
	}
}

// LogicOperator 逻辑操作符
type LogicOperator int

const (
	LogicOperatorAnd LogicOperator = iota // 逻辑与
	LogicOperatorOr                       // 逻辑或
)

// String 返回逻辑操作符的字符串表示
func (l LogicOperator) String() string {
	if l == LogicOperatorOr {
		return "OR"
	}
	return "AND"
}

// TimeConditionType 有效期类型
type TimeConditionType int

const (
	TimeConditionGFD TimeConditionType = iota // 当日有效
	TimeConditionGTD                          // 指定日期前有效
	TimeConditionGTC                          // 撤销前有效
)

// String 返回有效期类型的字符串表示
func (t TimeConditionType) String() string {
	switch t {
	case TimeConditionGFD:
		return "GFD"
	case TimeConditionGTD:
		return "GTD"
	case TimeConditionGTC:
		return "GTC"
	default:
		return "GFD"
	}
}

// ConditionOrderStatus 条件单状态
type ConditionOrderStatus int

const (
	StatusLive    ConditionOrderStatus = iota // 有效
	StatusSuspend                             // 暂停
	StatusCancel                              // 已撤销
	StatusDiscard                             // 已作废
	StatusTouched                             // 已触发
)

// String 返回条件单状态的字符串表示
func (s ConditionOrderStatus) String() string {
	switch s {
	case StatusLive:
		return "live"
	case StatusSuspend:
		return "suspend"
	case StatusCancel:
		return "cancel"
	case StatusDiscard:
		return "discard"
	case StatusTouched:
		return "touched"
	default:
		return "live"
	}
}

// InstrumentStatus 合约交易状态
type InstrumentStatus int

const (
	InstrumentStatusBeforeTrading    InstrumentStatus = iota // 开盘前
	InstrumentStatusClosed                                   // 收盘
	InstrumentStatusNoTrading                                // 非交易时段
	InstrumentStatusAuctionOrdering                          // 集合竞价报单
	InstrumentStatusAuctionBalance                           // 集合竞价平衡
	InstrumentStatusAuctionMatch                             // 集合竞价撮合
	InstrumentStatusContinousTrading                         // 连续交易
)

// ContingentCondition 触发条件
type ContingentCondition struct {
	ContingentType ContingentType `json:"contingent_type"` // 条件类型
	ExchangeID     string         `json:"exchange_id"`     // 交易所ID
	InstrumentID   string         `json:"instrument_id"`   // 合约代码
	IsTouched      bool           `json:"is_touched"`      // 是否已触发

	// 价格触发相关
	ContingentPrice float64           `json:"contingent_price"` // 触发价格
	PriceRelation   PriceRelationType `json:"price_relation"`   // 价格关系

	// 时间触发相关
	ContingentTime int64 `json:"contingent_time"` // 触发时间(Unix时间戳,秒)

	// 价格区间触发相关
	ContingentPriceLeft  float64 `json:"contingent_price_range_left"`  // 区间左边界
	ContingentPriceRight float64 `json:"contingent_price_range_right"` // 区间右边界

	// 保本止盈相关
	BreakEvenPrice     float64        `json:"break_even_price"`     // 保本价格
	HasBreakEvent      bool           `json:"m_has_break_event"`    // 是否已突破
	BreakEvenDirection OrderDirection `json:"break_even_direction"` // 持仓方向
}

// Symbol 返回合约代码 (exchange.instrument)
func (c *ContingentCondition) Symbol() string {
	return c.ExchangeID + "." + c.InstrumentID
}

// ContingentOrder 触发后的委托单
type ContingentOrder struct {
	ExchangeID      string         `json:"exchange_id"`       // 交易所ID
	InstrumentID    string         `json:"instrument_id"`     // 合约代码
	Direction       OrderDirection `json:"direction"`         // 买卖方向
	Offset          OrderOffset    `json:"offset"`            // 开平方向
	CloseTodayPrior bool           `json:"close_today_prior"` // 平今优先
	VolumeType      VolumeType     `json:"volume_type"`       // 手数类型
	Volume          int            `json:"volume"`            // 具体手数
	PriceType       PriceType      `json:"price_type"`        // 价格类型
	LimitPrice      float64        `json:"limit_price"`       // 限价
}

// Symbol 返回合约代码 (exchange.instrument)
func (o *ContingentOrder) Symbol() string {
	return o.ExchangeID + "." + o.InstrumentID
}

// ConditionOrder 条件单
type ConditionOrder struct {
	OrderID               string                `json:"order_id"`                     // 条件单ID
	TradingDay            int                   `json:"trading_day"`                  // 交易日 (YYYYMMDD)
	InsertDateTime        int64                 `json:"insert_date_time"`             // 创建时间(Unix时间戳,秒)
	ConditionList         []ContingentCondition `json:"condition_list"`               // 触发条件列表
	ConditionsLogicOper   LogicOperator         `json:"conditions_logic_oper"`        // 条件间逻辑
	OrderList             []ContingentOrder     `json:"order_list"`                   // 触发后的委托列表
	TimeConditionType     TimeConditionType     `json:"time_condition_type"`          // 有效期类型
	GTDDate               int                   `json:"GTD_date"`                     // GTD日期 (YYYYMMDD)
	IsCancelOriCloseOrder bool                  `json:"is_cancel_origin_close_order"` // 可平不足时撤原挂单
	Status                ConditionOrderStatus  `json:"status"`                       // 状态
	TouchedTime           int64                 `json:"touched_time"`                 // 触发时间(Unix时间戳,秒)

	// 内部使用
	Changed bool `json:"-"` // 是否有变更
}

// ConditionOrderData 用户条件单数据
type ConditionOrderData struct {
	BrokerID        string                     `json:"broker_id"`
	UserID          string                     `json:"user_id"`
	UserPassword    string                     `json:"user_password"`
	TradingDay      string                     `json:"trading_day"`
	ConditionOrders map[string]*ConditionOrder `json:"condition_orders"`
}

// NewConditionOrderData 创建条件单数据实例
func NewConditionOrderData() *ConditionOrderData {
	return &ConditionOrderData{
		ConditionOrders: make(map[string]*ConditionOrder),
	}
}

// ConditionOrderHisData 历史条件单数据
type ConditionOrderHisData struct {
	BrokerID           string            `json:"broker_id"`
	UserID             string            `json:"user_id"`
	UserPassword       string            `json:"user_password"`
	TradingDay         string            `json:"trading_day"`
	HisConditionOrders []*ConditionOrder `json:"his_condition_orders"`
}

// NewConditionOrderHisData 创建历史条件单数据实例
func NewConditionOrderHisData() *ConditionOrderHisData {
	return &ConditionOrderHisData{
		HisConditionOrders: make([]*ConditionOrder, 0),
	}
}

// InstrumentStatusInfo 合约交易状态信息
type InstrumentStatusInfo struct {
	ExchangeID      string           `json:"exchange_id"`
	InstrumentID    string           `json:"instrument_id"`
	ServerEnterTime int64            `json:"server_enter_time"`
	LocalEnterTime  int64            `json:"local_enter_time"`
	Status          InstrumentStatus `json:"status"`
	IsDataReady     bool             `json:"is_data_ready"`
}

// ReqInsertConditionOrder 插入条件单请求
type ReqInsertConditionOrder struct {
	Aid                   string                `json:"aid"`
	UserID                string                `json:"user_id"`
	OrderID               string                `json:"order_id"`
	ConditionList         []ContingentCondition `json:"condition_list"`
	ConditionsLogicOper   LogicOperator         `json:"conditions_logic_operator"`
	OrderList             []ContingentOrder     `json:"order_list"`
	TimeConditionType     TimeConditionType     `json:"time_condition_type"`
	GTDDate               int                   `json:"GTD_date"`
	IsCancelOriCloseOrder bool                  `json:"is_cancel_origin_close_order"`
}

// ReqCancelConditionOrder 撤销条件单请求
type ReqCancelConditionOrder struct {
	Aid     string `json:"aid"`
	UserID  string `json:"user_id"`
	OrderID string `json:"order_id"`
}

// ReqPauseConditionOrder 暂停条件单请求
type ReqPauseConditionOrder struct {
	Aid     string `json:"aid"`
	UserID  string `json:"user_id"`
	OrderID string `json:"order_id"`
}

// ReqResumeConditionOrder 恢复条件单请求
type ReqResumeConditionOrder struct {
	Aid     string `json:"aid"`
	UserID  string `json:"user_id"`
	OrderID string `json:"order_id"`
}

// QryHistoryConditionOrder 查询历史条件单请求
type QryHistoryConditionOrder struct {
	Aid       string `json:"aid"`
	UserID    string `json:"user_id"`
	ActionDay int    `json:"action_day"` // 查询日期 YYYYMMDD
}

// Config 条件单配置
type Config struct {
	// Enabled 是否启用条件单服务
	Enabled bool `json:"enabled" mapstructure:"enabled"`

	// DataPath 数据存储路径
	DataPath string `json:"data_path" mapstructure:"data_path"`

	// MaxNewOrdersPerDay 每日最大新建条件单数量
	MaxNewOrdersPerDay int `json:"max_new_orders_per_day" mapstructure:"max_new_orders_per_day"`

	// MaxValidOrdersTotal 最大有效条件单总数
	MaxValidOrdersTotal int `json:"max_valid_orders_total" mapstructure:"max_valid_orders_total"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Enabled:             false,
		DataPath:            "./data/condition_orders",
		MaxNewOrdersPerDay:  100,
		MaxValidOrdersTotal: 500,
	}
}
