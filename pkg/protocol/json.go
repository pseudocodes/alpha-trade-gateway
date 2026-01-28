// Package protocol JSON 序列化工具
// 使用 sonic 进行高性能 JSON 序列化
package protocol

import (
	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/tidwall/sjson"
)

// Marshal 序列化为 JSON
func Marshal(v interface{}) ([]byte, error) {
	return sonic.Marshal(v)
}

// Unmarshal 反序列化 JSON
func Unmarshal(data []byte, v interface{}) error {
	return sonic.Unmarshal(data, v)
}

// MarshalString 序列化为 JSON 字符串
func MarshalString(v interface{}) (string, error) {
	data, err := sonic.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// UnmarshalString 反序列化 JSON 字符串
func UnmarshalString(s string, v interface{}) error {
	return sonic.UnmarshalString(s, v)
}

// SetPath 在 JSON 中设置路径值
func SetPath(json, path string, value interface{}) (string, error) {
	return sjson.Set(json, path, value)
}

// RtnDataMsg 构建 rtn_data 消息
type RtnDataMsg struct {
	Aid  string        `json:"aid"`
	Data []interface{} `json:"data"`
}

// NewRtnDataMsg 创建新的 rtn_data 消息
func NewRtnDataMsg() *RtnDataMsg {
	return &RtnDataMsg{
		Aid:  "rtn_data",
		Data: make([]interface{}, 0),
	}
}

// AddData 添加数据
func (m *RtnDataMsg) AddData(data interface{}) {
	m.Data = append(m.Data, data)
}

// String 返回 JSON 字符串
func (m *RtnDataMsg) String() string {
	s, _ := MarshalString(m)
	return s
}

// NotifyType 常量定义
// 对应 C++ 消息类型
const (
	NotifyTypeMsgStr        = "MESSAGE"    // 普通消息
	NotifyTypeSettlementStr = "SETTLEMENT" // 结算单
)

// NotifyData 通知数据
type NotifyData struct {
	Notify map[string]*NotifyItem `json:"notify,omitempty"`
}

// NotifyItem 单个通知项
// 对应 C++ traderctp::OutputNotifySycn 消息格式
type NotifyItem struct {
	Type      string `json:"type"`       // 消息类型: "MESSAGE" 或 "SETTLEMENT"
	Level     string `json:"level"`      // 级别: "INFO", "WARNING", "ERROR"
	Code      int64  `json:"code"`       // 通知码
	SessionID int    `json:"session_id"` // 会话ID (对应 C++ m_session_id)
	Content   string `json:"content"`    // 消息内容
}

// notifySessionID 全局 session ID (由登录时设置)
// 对应 C++ m_session_id
var notifySessionID int

// SetNotifySessionID 设置通知消息的 session ID
// 在 CTP 登录成功后调用
func SetNotifySessionID(sessionID int) {
	notifySessionID = sessionID
}

// GetNotifySessionID 获取当前 session ID
func GetNotifySessionID() int {
	return notifySessionID
}

// generateNotifyKey 生成通知 key (GUID)
// 对应 C++ GenerateGuid()
func generateNotifyKey() string {
	return uuid.New().String()
}

// BuildNotifyMsg 构建通知消息
// 对应 C++ traderctp::OutputNotifySycn
func BuildNotifyMsg(code int64, content, level string) string {
	return BuildNotifyMsgWithType(code, content, level, NotifyTypeMsgStr)
}

// BuildNotifyMsgWithType 构建指定类型的通知消息
// 对应 C++ traderctp::OutputNotifySycn (带 type 参数)
func BuildNotifyMsgWithType(code int64, content, level, notifyType string) string {
	msg := NewRtnDataMsg()
	notify := &NotifyData{
		Notify: map[string]*NotifyItem{
			generateNotifyKey(): {
				Type:      notifyType,
				Level:     level,
				Code:      code,
				SessionID: notifySessionID,
				Content:   content,
			},
		},
	}
	msg.AddData(notify)
	return msg.String()
}

// BuildSettlementNotifyMsg 构建结算单通知消息
// 对应 C++ OutputNotifyAllSycn(325, settlement_info, "INFO", "SETTLEMENT")
func BuildSettlementNotifyMsg(code int64, content string) string {
	return BuildNotifyMsgWithType(code, content, "INFO", NotifyTypeSettlementStr)
}

// BrokerListMsg 构建经纪商列表消息
type BrokerListMsg struct {
	Aid     string   `json:"aid"`
	Brokers []string `json:"brokers"`
}

// BuildBrokerListMsg 构建经纪商列表消息
func BuildBrokerListMsg(brokers []string) string {
	msg := &BrokerListMsg{
		Aid:     "rtn_brokers",
		Brokers: brokers,
	}
	s, _ := MarshalString(msg)
	return s
}
