// Package condorder 条件单错误码定义
package condorder

import "errors"

// 错误码常量
// 对应 C++ condition_order_manager.cpp 中的错误码
const (
	ErrCodeInvalidInstrument      = 501 // 合约不存在
	ErrCodeTimeAlreadyPassed      = 502 // 触发时间已过
	ErrCodeInvalidPrice           = 503 // 价格不合法
	ErrCodeConditionAlreadyMet    = 504 // 条件已满足
	ErrCodeInvalidPriceRange      = 505 // 价格区间不合法
	ErrCodeInvalidBreakEvenPrice  = 506 // 保本价格不合法
	ErrCodeInvalidOrderInstrument = 507 // 订单合约不存在
	ErrCodeInvalidVolume          = 508 // 手数不合法
	ErrCodeInvalidOrderPrice      = 509 // 订单价格不合法
	ErrCodeInvalidGTDDate         = 510 // 有效日期不合法
	ErrCodeExceedDailyLimit       = 511 // 超过每日限额
	ErrCodeExceedTotalLimit       = 512 // 超过总限额
	ErrCodeServiceStopped         = 513 // 服务已停止
	ErrCodeOrderIDDuplicate       = 514 // 订单号重复
	ErrCodeInvalidUserID          = 515 // 用户名错误
	ErrCodeInsertSuccess          = 516 // 下单成功
	ErrCodeCancelServiceStopped   = 517 // 撤单服务已停止
	ErrCodeCancelInvalidUserID    = 518 // 撤单用户名错误
	ErrCodeOrderNotFound          = 519 // 订单不存在
	ErrCodeOrderAlreadyTouched    = 520 // 条件单已触发
	ErrCodeOrderAlreadyCanceled   = 521 // 条件单已撤销
	ErrCodeOrderAlreadyDiscarded  = 522 // 条件单已作废
	ErrCodeCancelSuccess          = 523 // 撤单成功
	ErrCodePauseServiceStopped    = 524 // 暂停服务已停止
	ErrCodePauseInvalidUserID     = 525 // 暂停用户名错误
	ErrCodePauseOrderNotFound     = 526 // 暂停订单不存在
	ErrCodePauseAlreadyTouched    = 527 // 暂停条件单已触发
	ErrCodePauseAlreadyCanceled   = 528 // 暂停条件单已撤销
	ErrCodePauseAlreadyDiscarded  = 529 // 暂停条件单已作废
	ErrCodeOrderAlreadySuspended  = 530 // 条件单已暂停
	ErrCodePauseSuccess           = 531 // 暂停成功
	ErrCodeResumeServiceStopped   = 532 // 恢复服务已停止
	ErrCodeResumeInvalidUserID    = 533 // 恢复用户名错误
	ErrCodeResumeOrderNotFound    = 534 // 恢复订单不存在
	ErrCodeOrderNotSuspended      = 535 // 条件单未暂停
	ErrCodeResumeSuccess          = 536 // 恢复成功
	ErrCodeQryHisServiceStopped   = 537 // 查询历史服务已停止
	ErrCodeQryHisInvalidUserID    = 538 // 查询历史用户名错误
	ErrCodeQryHisInvalidDate      = 539 // 查询历史日期错误
	ErrCodeAllConditionsMet       = 540 // 所有条件都已满足
)

// 错误定义
var (
	// ErrServiceStopped 条件单服务已停止
	ErrServiceStopped = errors.New("condition order service stopped")

	// ErrOrderIDDuplicate 订单ID重复
	ErrOrderIDDuplicate = errors.New("order id duplicate")

	// ErrInvalidUserID 无效的用户ID
	ErrInvalidUserID = errors.New("invalid user id")

	// ErrOrderNotFound 订单不存在
	ErrOrderNotFound = errors.New("order not found")

	// ErrFileNotExist 文件不存在
	ErrFileNotExist = errors.New("file not exist")

	// ErrInvalidInstrument 无效的合约
	ErrInvalidInstrument = errors.New("invalid instrument")

	// ErrInvalidPrice 无效的价格
	ErrInvalidPrice = errors.New("invalid price")

	// ErrInvalidPriceRange 无效的价格区间
	ErrInvalidPriceRange = errors.New("invalid price range")

	// ErrInvalidVolume 无效的手数
	ErrInvalidVolume = errors.New("invalid volume")

	// ErrInvalidGTDDate 无效的GTD日期
	ErrInvalidGTDDate = errors.New("invalid GTD date")

	// ErrExceedDailyLimit 超过每日限额
	ErrExceedDailyLimit = errors.New("exceed daily limit")

	// ErrExceedTotalLimit 超过总限额
	ErrExceedTotalLimit = errors.New("exceed total limit")

	// ErrConditionAlreadyMet 条件已满足
	ErrConditionAlreadyMet = errors.New("condition already met")

	// ErrTimeAlreadyPassed 时间已过
	ErrTimeAlreadyPassed = errors.New("time already passed")

	// ErrOrderAlreadyTouched 条件单已触发
	ErrOrderAlreadyTouched = errors.New("order already touched")

	// ErrOrderAlreadyCanceled 条件单已撤销
	ErrOrderAlreadyCanceled = errors.New("order already canceled")

	// ErrOrderAlreadyDiscarded 条件单已作废
	ErrOrderAlreadyDiscarded = errors.New("order already discarded")

	// ErrOrderAlreadySuspended 条件单已暂停
	ErrOrderAlreadySuspended = errors.New("order already suspended")

	// ErrOrderNotSuspended 条件单未暂停
	ErrOrderNotSuspended = errors.New("order not suspended")

	// ErrParseRequest 解析请求失败
	ErrParseRequest = errors.New("parse request failed")
)

// ErrorMessage 错误码对应的消息
var ErrorMessage = map[int]string{
	ErrCodeInvalidInstrument:      "条件单已被服务器拒绝,条件单触发条件中的合约ID不存在",
	ErrCodeTimeAlreadyPassed:      "条件单已被服务器拒绝,时间触发条件指定的触发时间小于当前时间",
	ErrCodeInvalidPrice:           "条件单已被服务器拒绝,价格触发条件指定的触发价格不合法",
	ErrCodeConditionAlreadyMet:    "条件单已被服务器拒绝,当前价格已满足设定条件,请重新设置",
	ErrCodeInvalidPriceRange:      "条件单已被服务器拒绝,价格区间触发条件指定的价格区间不合法",
	ErrCodeInvalidBreakEvenPrice:  "条件单已被服务器拒绝,固定价格止盈触发条件指定的固定价格不合法",
	ErrCodeInvalidOrderInstrument: "条件单已被服务器拒绝,条件单触发的订单列表中的合约ID不存在",
	ErrCodeInvalidVolume:          "条件单已被服务器拒绝,条件单触发的订单手数设置不合法",
	ErrCodeInvalidOrderPrice:      "条件单已被服务器拒绝,条件单触发的订单价格设置不合法",
	ErrCodeInvalidGTDDate:         "条件单已被服务器拒绝,条件单有效日期设置不合法",
	ErrCodeExceedDailyLimit:       "条件单已被服务器拒绝,当前交易日新增条件单数量超过最大数量限制",
	ErrCodeExceedTotalLimit:       "条件单已被服务器拒绝,当前有效条件单数量超过最大数量限制",
	ErrCodeServiceStopped:         "条件单已被服务器拒绝,原因:条件单服务器已经暂时停止运行",
	ErrCodeOrderIDDuplicate:       "条件单已被服务器拒绝,原因:单号重复",
	ErrCodeInvalidUserID:          "条件单已被服务器拒绝,原因:下单指令中的用户名错误",
	ErrCodeInsertSuccess:          "条件单下单成功",
	ErrCodeCancelServiceStopped:   "条件单撤单请求已被服务器拒绝,原因:条件单服务器已经暂时停止运行",
	ErrCodeCancelInvalidUserID:    "条件单撤单请求已被服务器拒绝,原因:撤单请求中的用户名错误",
	ErrCodeOrderNotFound:          "条件单撤单请求已被服务器拒绝,原因:单号不存在",
	ErrCodeOrderAlreadyTouched:    "条件单撤单请求已被服务器拒绝,原因:条件单已触发",
	ErrCodeOrderAlreadyCanceled:   "条件单撤单请求已被服务器拒绝,原因:条件单已撤",
	ErrCodeOrderAlreadyDiscarded:  "条件单撤单请求已被服务器拒绝,原因:条件单是废单",
	ErrCodeCancelSuccess:          "条件单撤单成功",
	ErrCodePauseServiceStopped:    "条件单暂停请求已被服务器拒绝,原因:条件单服务器已经暂时停止运行",
	ErrCodePauseInvalidUserID:     "条件单暂停请求已被服务器拒绝,原因:暂停请求中的用户名错误",
	ErrCodePauseOrderNotFound:     "条件单暂停请求已被服务器拒绝,原因:单号不存在",
	ErrCodePauseAlreadyTouched:    "条件单暂停请求已被服务器拒绝,原因:条件单已触发",
	ErrCodePauseAlreadyCanceled:   "条件单暂停请求已被服务器拒绝,原因:条件单已撤",
	ErrCodePauseAlreadyDiscarded:  "条件单暂停请求已被服务器拒绝,原因:条件单是废单",
	ErrCodeOrderAlreadySuspended:  "条件单暂停请求已被服务器拒绝,原因:条件单已经暂停",
	ErrCodePauseSuccess:           "条件单暂停成功",
	ErrCodeResumeServiceStopped:   "条件单恢复请求已被服务器拒绝,原因:条件单服务器已经暂时停止运行",
	ErrCodeResumeInvalidUserID:    "条件单恢复请求已被服务器拒绝,原因:恢复请求中的用户名错误",
	ErrCodeResumeOrderNotFound:    "条件单恢复请求已被服务器拒绝,原因:单号不存在",
	ErrCodeOrderNotSuspended:      "条件单恢复请求已被服务器拒绝,原因:条件单不是处于暂停状态",
	ErrCodeResumeSuccess:          "条件单恢复成功",
	ErrCodeQryHisServiceStopped:   "历史条件单查询请求已被服务器拒绝,原因:条件单服务器已经暂时停止运行",
	ErrCodeQryHisInvalidUserID:    "历史条件单查询请求已被服务器拒绝,原因:查询请求中的用户名错误",
	ErrCodeQryHisInvalidDate:      "历史条件单查询请求已被服务器拒绝,原因:查询请求中的日期输入有误",
	ErrCodeAllConditionsMet:       "条件单已被服务器拒绝,当前所有条件都已满足,请重新设置",
}

// GetErrorMessage 获取错误码对应的消息
func GetErrorMessage(code int) string {
	if msg, ok := ErrorMessage[code]; ok {
		return msg
	}
	return "未知错误"
}
