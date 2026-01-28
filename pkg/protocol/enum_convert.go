// Package protocol 枚举转换函数
// 用于解析 C++ 风格的字符串枚举值，保持与 C++ 报文格式兼容
// 对应 C++ ctp_define.cpp 中的 AddItemEnum 宏定义
package protocol

import (
	"strings"

	"github.com/tidwall/gjson"
)

// ParseDirection 解析方向字符串
// 对应 C++ ctp_define.cpp:79-82 的 direction 枚举
func ParseDirection(s string) int64 {
	switch strings.ToUpper(s) {
	case "BUY":
		return DirectionBuy
	case "SELL":
		return DirectionSell
	default:
		return DirectionUnknown
	}
}

// ParseOffset 解析开平标志字符串
// 对应 C++ ctp_define.cpp:83-90 的 offset 枚举
func ParseOffset(s string) int64 {
	switch strings.ToUpper(s) {
	case "OPEN":
		return OffsetOpen
	case "CLOSE":
		return OffsetClose
	case "CLOSETODAY":
		return OffsetCloseToday
	default:
		return OffsetUnknown
	}
}

// ParsePriceType 解析价格类型字符串
// 对应 C++ ctp_define.cpp:93-98 的 price_type 枚举
func ParsePriceType(s string) int64 {
	switch strings.ToUpper(s) {
	case "LIMIT":
		return PriceTypeLimit
	case "ANY":
		return PriceTypeAny
	case "BEST":
		return PriceTypeBest
	case "FIVELEVEL":
		return PriceTypeFiveLevel
	default:
		return PriceTypeUnknown
	}
}

// ParseVolumeCondition 解析成交量条件字符串
// 对应 C++ ctp_define.cpp:99-103 的 volume_condition 枚举
func ParseVolumeCondition(s string) int64 {
	switch strings.ToUpper(s) {
	case "ANY":
		return OrderVolumeConditionAny
	case "MIN":
		return OrderVolumeConditionMin
	case "ALL":
		return OrderVolumeConditionAll
	default:
		return OrderVolumeConditionAny // 默认值
	}
}

// ParseTimeCondition 解析有效期类型字符串
// 对应 C++ ctp_define.cpp:104-111 的 time_condition 枚举
func ParseTimeCondition(s string) int64 {
	switch strings.ToUpper(s) {
	case "IOC":
		return OrderTimeConditionIOC
	case "GFS":
		return OrderTimeConditionGFS
	case "GFD":
		return OrderTimeConditionGFD
	case "GTD":
		return OrderTimeConditionGTD
	case "GTC":
		return OrderTimeConditionGTC
	case "GFA":
		return OrderTimeConditionGFA
	default:
		return OrderTimeConditionGFD // 默认值
	}
}

// ParseHedgeFlag 解析投机套保标志字符串
// 对应 C++ ctp_define.cpp:112-118 的 hedge_flag 枚举
func ParseHedgeFlag(s string) int64 {
	switch strings.ToUpper(s) {
	case "SPECULATION":
		return HedgeFlagSpeculation
	case "ARBITRAGE":
		return HedgeFlagArbitrage
	case "HEDGE":
		return HedgeFlagHedge
	case "MARKETMAKER":
		return HedgeFlagMarketMaker
	default:
		return HedgeFlagSpeculation // 默认值
	}
}

// ParseContingentCondition 解析触发条件字符串
// 对应 C++ ctp_define.cpp:119-124 的 contingent_condition 枚举
func ParseContingentCondition(s string) int64 {
	switch strings.ToUpper(s) {
	case "IMMEDIATELY":
		return ContingentConditionImmediately
	case "TOUCH":
		return ContingentConditionTouch
	case "TOUCHPROFIT":
		return ContingentConditionTouchProfit
	default:
		return ContingentConditionImmediately // 默认值
	}
}

// ParseEnumValue 通用枚举值解析函数
// 自动检测值类型（字符串或整数）并转换
// parseFunc: 字符串解析函数
// defaultVal: 值不存在时的默认值
func ParseEnumValue(val gjson.Result, parseFunc func(string) int64, defaultVal int64) int64 {
	if !val.Exists() {
		return defaultVal
	}
	if val.Type == gjson.String {
		return parseFunc(val.String())
	}
	return val.Int()
}
