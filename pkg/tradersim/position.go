// Package tradersim 持仓管理、盈亏计算、资金管理
package tradersim

import (
	"alpha-trade-gateway/pkg/inslist"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// recalculatePositionAndFloatProfit 重算持仓盈亏
// 对应 C++ RecaculatePositionAndFloatProfit
func (t *TraderSim) recalculatePositionAndFloatProfit() {
	t.userMu.Lock()
	defer t.userMu.Unlock()

	if t.user == nil {
		return
	}

	var totalPositionProfit, totalFloatProfit float64
	var totalMargin float64
	changed := false

	for symbol, pos := range t.user.Positions {
		ins := t.getInstrument(symbol)

		// 获取最新价
		lastPrice := t.getLastPrice(symbol, ins)
		if math.IsNaN(lastPrice) || lastPrice <= 0 {
			continue
		}

		// 检查价格是否变化
		if math.Abs(lastPrice-pos.LastPrice) < 1e-9 {
			totalPositionProfit += pos.PositionProfit
			totalFloatProfit += pos.FloatProfit
			totalMargin += pos.Margin
			continue
		}

		var volumeMultiple float64 = 10
		if ins != nil && ins.VolumeMultiple > 0 {
			volumeMultiple = float64(ins.VolumeMultiple)
		}

		pos.LastPrice = lastPrice

		// 计算多头持仓盈亏
		pos.PositionProfitLong = lastPrice*float64(pos.VolumeLong)*volumeMultiple - pos.PositionCostLong

		// 计算空头持仓盈亏
		pos.PositionProfitShort = pos.PositionCostShort - lastPrice*float64(pos.VolumeShort)*volumeMultiple

		pos.PositionProfit = pos.PositionProfitLong + pos.PositionProfitShort

		// 计算多头浮动盈亏
		pos.FloatProfitLong = lastPrice*float64(pos.VolumeLong)*volumeMultiple - pos.OpenCostLong

		// 计算空头浮动盈亏
		pos.FloatProfitShort = pos.OpenCostShort - lastPrice*float64(pos.VolumeShort)*volumeMultiple

		pos.FloatProfit = pos.FloatProfitLong + pos.FloatProfitShort

		// 计算保证金
		var marginRate float64 = 0.1
		if ins != nil && ins.Margin > 0 {
			marginRate = ins.Margin
		}
		pos.Margin = lastPrice * float64(pos.VolumeLong+pos.VolumeShort) * volumeMultiple * marginRate

		// 计算开仓均价和持仓均价
		if pos.VolumeLong > 0 {
			pos.OpenPriceLong = pos.OpenCostLong / (float64(pos.VolumeLong) * volumeMultiple)
			pos.PositionPriceLong = pos.PositionCostLong / (float64(pos.VolumeLong) * volumeMultiple)
		}
		if pos.VolumeShort > 0 {
			pos.OpenPriceShort = pos.OpenCostShort / (float64(pos.VolumeShort) * volumeMultiple)
			pos.PositionPriceShort = pos.PositionCostShort / (float64(pos.VolumeShort) * volumeMultiple)
		}

		// 累计
		totalPositionProfit += pos.PositionProfit
		totalFloatProfit += pos.FloatProfit
		totalMargin += pos.Margin

		pos.Changed = true
		changed = true
	}

	// 重算资金账户
	if changed {
		account := t.getAccountNoLock()
		if account != nil {
			account.PositionProfit = totalPositionProfit
			account.FloatProfit = totalFloatProfit
			account.Balance = account.StaticBalance + totalFloatProfit + account.CloseProfit - account.Commission
			account.Margin = totalMargin
			account.Available = account.Balance - account.Margin - account.FrozenMargin
			if account.Balance > 0 {
				account.RiskRatio = account.Margin / account.Balance
			}
			account.Changed = true
		}
		t.somethingChanged.Store(true)
	}
}

// getLastPrice 获取最新价
func (t *TraderSim) getLastPrice(symbol string, ins *inslist.InstrumentInfo) float64 {
	if t.marketClient == nil {
		return math.NaN()
	}

	quote := t.marketClient.GetQuote(symbol)
	if quote == nil {
		return math.NaN()
	}

	// 交易中：使用最新价
	if !math.IsNaN(quote.LastPrice) && quote.LastPrice > 0 {
		return quote.LastPrice
	}

	// 开盘前：使用昨收盘价或昨结算价
	if !math.IsNaN(quote.PreClose) && quote.PreClose > 0 {
		return quote.PreClose
	}
	if !math.IsNaN(quote.PreSettlement) && quote.PreSettlement > 0 {
		return quote.PreSettlement
	}

	// 从合约信息获取
	if ins != nil {
		if !math.IsNaN(ins.PreSettlement) && ins.PreSettlement > 0 {
			return ins.PreSettlement
		}
	}

	return math.NaN()
}

// sendUserData 发送用户数据给指定连接
func (t *TraderSim) sendUserData(connID int) {
	t.userMu.RLock()
	defer t.userMu.RUnlock()

	if t.user == nil {
		return
	}

	msg := t.buildUserDataMsg()
	t.SendMsg(connID, msg)
}

// sendUserDataAll 发送用户数据给所有连接
func (t *TraderSim) sendUserDataAll() {
	t.userMu.RLock()
	defer t.userMu.RUnlock()

	if t.user == nil {
		return
	}

	msg := t.buildUserDataMsg()
	t.SendMsgAll(msg)

	// 重置 changed 标志
	for _, acc := range t.user.Accounts {
		acc.Changed = false
	}
	for _, pos := range t.user.Positions {
		pos.Changed = false
	}
	for _, order := range t.user.Orders {
		order.Changed = false
	}
	for _, trade := range t.user.Trades {
		trade.Changed = false
	}
}

// buildUserDataMsg 构建用户数据消息
func (t *TraderSim) buildUserDataMsg() string {
	var sb strings.Builder
	sb.WriteString(`{"aid":"rtn_data","data":[{"trade":{`)
	sb.WriteString(fmt.Sprintf(`"%s":{`, t.user.UserID))

	// 账户
	sb.WriteString(`"accounts":{`)
	first := true
	for key, acc := range t.user.Accounts {
		if !first {
			sb.WriteString(",")
		}
		accJSON, _ := json.Marshal(acc)
		sb.WriteString(fmt.Sprintf(`"%s":%s`, key, string(accJSON)))
		first = false
	}
	sb.WriteString(`},`)

	// 持仓
	sb.WriteString(`"positions":{`)
	first = true
	for key, pos := range t.user.Positions {
		if !first {
			sb.WriteString(",")
		}
		posJSON, _ := json.Marshal(pos)
		sb.WriteString(fmt.Sprintf(`"%s":%s`, key, string(posJSON)))
		first = false
	}
	sb.WriteString(`},`)

	// 订单
	sb.WriteString(`"orders":{`)
	first = true
	for key, order := range t.user.Orders {
		if !first {
			sb.WriteString(",")
		}
		orderJSON, _ := json.Marshal(order)
		sb.WriteString(fmt.Sprintf(`"%s":%s`, key, string(orderJSON)))
		first = false
	}
	sb.WriteString(`},`)

	// 成交
	sb.WriteString(`"trades":{`)
	first = true
	for key, trade := range t.user.Trades {
		if !first {
			sb.WriteString(",")
		}
		tradeJSON, _ := json.Marshal(trade)
		sb.WriteString(fmt.Sprintf(`"%s":%s`, key, string(tradeJSON)))
		first = false
	}
	sb.WriteString(`}`)

	sb.WriteString(`}}}]}`)
	return sb.String()
}
