// Package trader 查询调度器
// 对应 C++ OnIdle 中的查询调度机制
// 实现查询节流 (1100ms 冷却) 和异步查询触发
package trader

import (
	"alpha-trade-gateway/pkg/logger"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// QueryScheduler 查询调度器
// 对应 C++ m_req_xxx_id / m_rsp_xxx_id 机制
type QueryScheduler struct {
	// 持仓查询 ID
	reqPositionID atomic.Int32
	rspPositionID atomic.Int32

	// 账户查询 ID
	reqAccountID atomic.Int32
	rspAccountID atomic.Int32

	// 经纪商参数查询标志
	needQueryBrokerParams atomic.Bool

	// 银行查询标志
	needQueryBank atomic.Bool

	// 签约关系查询标志
	needQueryRegister atomic.Bool

	// 需要保存文件标志
	needSaveFile atomic.Bool

	// 查询冷却时间
	lastQueryTime time.Time
	queryInterval time.Duration
	queryMu       sync.Mutex

	// 发送数据冷却
	lastSendTime time.Time
	sendInterval time.Duration

	// 停止信号
	stopCh chan struct{}
}

// NewQueryScheduler 创建查询调度器
func NewQueryScheduler() *QueryScheduler {
	return &QueryScheduler{
		queryInterval: 1100 * time.Millisecond, // CTP 查询节流
		sendInterval:  100 * time.Millisecond,  // 数据发送间隔
		stopCh:        make(chan struct{}),
	}
}

// RequestPositionQuery 请求持仓查询
// 对应 C++ m_req_position_id++
func (s *QueryScheduler) RequestPositionQuery() {
	s.reqPositionID.Add(1)
	logger.Debug("position query requested",
		zap.Int32("req_id", s.reqPositionID.Load()),
		zap.Int32("rsp_id", s.rspPositionID.Load()),
	)
}

// RequestAccountQuery 请求账户查询
// 对应 C++ m_req_account_id++
func (s *QueryScheduler) RequestAccountQuery() {
	s.reqAccountID.Add(1)
	logger.Debug("account query requested",
		zap.Int32("req_id", s.reqAccountID.Load()),
		zap.Int32("rsp_id", s.rspAccountID.Load()),
	)
}

// OnPositionQueryComplete 持仓查询完成
// 对应 C++ m_rsp_position_id = nRequestID
func (s *QueryScheduler) OnPositionQueryComplete() {
	s.rspPositionID.Store(s.reqPositionID.Load())
}

// OnAccountQueryComplete 账户查询完成
// 对应 C++ m_rsp_account_id = nRequestID
func (s *QueryScheduler) OnAccountQueryComplete() {
	s.rspAccountID.Store(s.reqAccountID.Load())
}

// NeedPositionQuery 是否需要查询持仓
func (s *QueryScheduler) NeedPositionQuery() bool {
	return s.reqPositionID.Load() > s.rspPositionID.Load()
}

// NeedAccountQuery 是否需要查询账户
func (s *QueryScheduler) NeedAccountQuery() bool {
	return s.reqAccountID.Load() > s.rspAccountID.Load()
}

// SetNeedQueryBrokerParams 设置需要查询经纪商参数
func (s *QueryScheduler) SetNeedQueryBrokerParams(need bool) {
	s.needQueryBrokerParams.Store(need)
}

// NeedQueryBrokerParams 是否需要查询经纪商参数
func (s *QueryScheduler) NeedQueryBrokerParams() bool {
	return s.needQueryBrokerParams.Load()
}

// SetNeedQueryBank 设置需要查询银行
func (s *QueryScheduler) SetNeedQueryBank(need bool) {
	s.needQueryBank.Store(need)
}

// NeedQueryBank 是否需要查询银行
func (s *QueryScheduler) NeedQueryBank() bool {
	return s.needQueryBank.Load()
}

// SetNeedQueryRegister 设置需要查询签约关系
func (s *QueryScheduler) SetNeedQueryRegister(need bool) {
	s.needQueryRegister.Store(need)
}

// NeedQueryRegister 是否需要查询签约关系
func (s *QueryScheduler) NeedQueryRegister() bool {
	return s.needQueryRegister.Load()
}

// SetNeedSaveFile 设置需要保存文件
func (s *QueryScheduler) SetNeedSaveFile(need bool) {
	s.needSaveFile.Store(need)
}

// NeedSaveFile 是否需要保存文件
func (s *QueryScheduler) NeedSaveFile() bool {
	return s.needSaveFile.Load()
}

// CanQuery 检查是否可以发起查询 (冷却检查)
// 对应 C++ m_next_qry_dt 检查
func (s *QueryScheduler) CanQuery() bool {
	s.queryMu.Lock()
	defer s.queryMu.Unlock()

	now := time.Now()
	if now.Sub(s.lastQueryTime) < s.queryInterval {
		return false
	}
	s.lastQueryTime = now
	return true
}

// DelayQueryTime 将 lastQueryTime 向后延迟一个 queryInterval
// 用于在需要时延长下一次查询的等待时间
func (s *QueryScheduler) DelayQueryTime(d time.Duration) {
	s.queryMu.Lock()
	defer s.queryMu.Unlock()
	now := time.Now()
	s.lastQueryTime = now.Add(d)
}

// CanSendData 检查是否可以发送数据 (冷却检查)
func (s *QueryScheduler) CanSendData() bool {
	s.queryMu.Lock()
	defer s.queryMu.Unlock()

	now := time.Now()
	if now.Sub(s.lastSendTime) < s.sendInterval {
		return false
	}
	s.lastSendTime = now
	return true
}

// Reset 重置所有查询状态
func (s *QueryScheduler) Reset() {
	s.reqPositionID.Store(0)
	s.rspPositionID.Store(0)
	s.reqAccountID.Store(0)
	s.rspAccountID.Store(0)
	s.needQueryBrokerParams.Store(false)
	s.needQueryBank.Store(false)
	s.needQueryRegister.Store(false)
	s.needSaveFile.Store(false)
}

// Stop 停止调度器
func (s *QueryScheduler) Stop() {
	close(s.stopCh)
}
