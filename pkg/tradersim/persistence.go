// Package tradersim 数据持久化、跨日结转
package tradersim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/protocol"
)

// saveUserDataFile 保存用户数据
// 对应 C++ SaveUserDataFile
func (t *TraderSim) saveUserDataFile() error {
	t.userMu.RLock()
	defer t.userMu.RUnlock()

	if t.user == nil {
		return nil
	}

	if t.userFilePath == "" {
		logger.Error("user file path is empty",
			zap.String("fun", "saveUserDataFile"),
			zap.String("key", t.brokerID),
			zap.String("bid", t.reqLogin.Bid),
			zap.String("user_name", t.reqLogin.UserName))
		return nil
	}

	// 构建文件路径: {user_file_path}/{broker_id}/{user_id}.json
	filePath := filepath.Join(t.userFilePath, t.brokerID, t.user.UserID+".json")

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	logger.Info("save user date file",
		zap.String("fun", "saveUserDataFile"),
		zap.String("key", t.brokerID),
		zap.String("user_name", t.reqLogin.UserName),
		zap.String("fileName", filePath))

	// 序列化用户数据
	data, err := json.MarshalIndent(t.user, "", "  ")
	if err != nil {
		logger.Error("save user date file fail!",
			zap.String("fun", "saveUserDataFile"),
			zap.String("key", t.brokerID),
			zap.String("user_name", t.reqLogin.UserName),
			zap.String("fileName", filePath),
			zap.Error(err))
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

// loadUserDataFile 加载用户数据
// 对应 C++ LoadUserDataFile
func (t *TraderSim) loadUserDataFile() error {
	if t.reqLogin == nil {
		return nil
	}

	if t.userFilePath == "" {
		logger.Error("m_user_file_path is empty",
			zap.String("fun", "loadUserDataFile"),
			zap.String("key", t.brokerID),
			zap.String("bid", t.reqLogin.Bid),
			zap.String("user_name", t.reqLogin.UserName))
		return nil
	}

	filePath := filepath.Join(t.userFilePath, t.brokerID, t.reqLogin.UserName+".json")

	logger.Info("load user data file",
		zap.String("fun", "loadUserDataFile"),
		zap.String("key", t.brokerID),
		zap.String("user_name", t.reqLogin.UserName),
		zap.String("fileName", filePath))

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 新用户，初始化数据
			t.initNewUser()
			return nil
		}
		logger.Warn("load user data file failed!",
			zap.String("fun", "loadUserDataFile"),
			zap.String("key", t.brokerID),
			zap.String("user_name", t.reqLogin.UserName),
			zap.String("fileName", filePath),
			zap.Error(err))
		return err
	}

	user := &protocol.User{}
	if err := json.Unmarshal(data, user); err != nil {
		return err
	}

	// 检查是否跨交易日
	currentTradingDay := t.getTradingDay()
	if user.TradingDay != currentTradingDay {
		logger.Info("diffrent trading day!",
			zap.String("fun", "loadUserDataFile"),
			zap.String("key", t.brokerID),
			zap.String("user_name", t.reqLogin.UserName),
			zap.String("old_trading_day", user.TradingDay),
			zap.String("trading_day", currentTradingDay))
		t.handleDayChange(user, currentTradingDay)
	} else {
		logger.Info("same trading day!",
			zap.String("fun", "loadUserDataFile"),
			zap.String("key", t.brokerID),
			zap.String("fileName", filePath),
			zap.String("trading_day", user.TradingDay))
	}

	t.user = user

	// 重建活跃订单集合
	t.aliveOrdersMu.Lock()
	for _, order := range t.user.Orders {
		if order.Status == protocol.OrderStatusAlive {
			t.aliveOrders[order.OrderID] = order
		}
	}
	t.aliveOrdersMu.Unlock()

	logger.Info("sim: user data loaded",
		zap.String("fun", "loadUserDataFile"),
		zap.String("key", t.brokerID),
		zap.String("user_id", user.UserID),
		zap.String("trading_day", user.TradingDay),
		zap.Int("positions", len(user.Positions)),
		zap.Int("orders", len(user.Orders)))

	return nil
}

// handleDayChange 处理跨交易日
// 对应 C++ LoadUserDataFile 中的跨交易日处理
func (t *TraderSim) handleDayChange(user *protocol.User, newTradingDay string) {
	// 1. 清空当日委托/成交/转账记录
	user.Orders = make(map[string]*protocol.Order)
	user.Trades = make(map[string]*protocol.Trade)
	user.Transfers = make(map[string]*protocol.TransferLog)

	// 2. 账户权益结转
	for _, acc := range user.Accounts {
		// pre_balance = balance (昨日动态权益)
		acc.PreBalance = acc.Balance
		// 重置当日发生额
		acc.Deposit = 0
		acc.Withdraw = 0
		acc.CloseProfit = 0
		acc.Commission = 0
		acc.StaticBalance = acc.PreBalance
		acc.PositionProfit = 0
		acc.FloatProfit = 0
		acc.FrozenMargin = 0
		acc.FrozenCommission = 0
		acc.Changed = true
	}

	// 3. 持仓结转
	for symbol, pos := range user.Positions {
		exchangeID := strings.Split(symbol, ".")[0]
		isSHFEorINE := exchangeID == "SHFE" || exchangeID == "INE"

		if isSHFEorINE {
			// 上期所/能源中心：今仓 → 昨仓
			pos.VolumeLongHis += pos.VolumeLongToday
			pos.VolumeLongToday = 0
			pos.VolumeShortHis += pos.VolumeShortToday
			pos.VolumeShortToday = 0

			// 成本结转
			pos.OpenCostLongHis += pos.OpenCostLongToday
			pos.OpenCostLongToday = 0
			pos.OpenCostShortHis += pos.OpenCostShortToday
			pos.OpenCostShortToday = 0
			pos.PositionCostLongHis += pos.PositionCostLongToday
			pos.PositionCostLongToday = 0
			pos.PositionCostShortHis += pos.PositionCostShortToday
			pos.PositionCostShortToday = 0
		}
		// 其他交易所：保持今仓不变

		// 清空冻结
		pos.VolumeLongFrozenToday = 0
		pos.VolumeLongFrozenHis = 0
		pos.VolumeShortFrozenToday = 0
		pos.VolumeShortFrozenHis = 0
		pos.VolumeLongFrozen = 0
		pos.VolumeShortFrozen = 0
		pos.FrozenMargin = 0
		pos.Changed = true
	}

	// 4. 删除空持仓
	for symbol, pos := range user.Positions {
		if pos.VolumeLong == 0 && pos.VolumeShort == 0 {
			delete(user.Positions, symbol)
		}
	}

	// 5. 更新交易日
	user.TradingDay = newTradingDay
}

// initNewUser 初始化新用户
// 对应 C++ OnInit
func (t *TraderSim) initNewUser() {
	userName := ""
	if t.reqLogin != nil {
		userName = t.reqLogin.UserName
	}

	t.user = protocol.NewUser(userName)
	t.user.TradingDay = t.getTradingDay()

	// 初始化资金账户 (100万初始资金)
	account := protocol.NewAccount()
	account.UserID = userName
	account.Currency = "CNY"
	account.PreBalance = 1000000.0
	account.Balance = 1000000.0
	account.StaticBalance = 1000000.0
	account.Available = 1000000.0
	t.user.Accounts["CNY"] = account

	// 初始化模拟银行
	bank := &protocol.Bank{
		BankID:   "SIM",
		BankName: "模拟银行",
	}
	t.user.Banks["SIM"] = bank

	logger.Info("sim init new balance",
		zap.String("fun", "initNewUser"),
		zap.String("key", t.brokerID),
		zap.String("bid", t.reqLogin.Bid),
		zap.String("user_name", userName),
		zap.Float64("balance", account.Balance))
}
