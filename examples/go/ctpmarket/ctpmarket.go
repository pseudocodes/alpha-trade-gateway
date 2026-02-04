package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/marketfeed"
)

/*
使用 CTP 行情客户端的示例

Simnow 模拟环境：
7x24环境：
  交易前置: tcp://182.254.243.31:40001
  行情前置: tcp://182.254.243.31:40011

仿真环境1：交易时段同实盘
  交易前置: tcp://182.254.243.31:30001
  行情前置: tcp://182.254.243.31:30011

运行方式：
  cd examples/ctpmarket
  go run ctpmarket.go
*/

const (
	// Simnow 7x24 环境
	MdFront = "tcp://182.254.243.31:40011"
	// MdFront = "tcp://182.254.243.31:30011"

	BrokerID = "9999"
	UserID   = "046111" // 使用你的 Simnow 账号
	Password = "046111" // 使用你的 Simnow 密码
)

func main() {
	// 初始化日志
	logger.InitDefault()

	log.Println("=== CTP Market Feed Client Example ===")

	// 创建 CTP 行情客户端
	client := marketfeed.NewCtpMarketClient(
		MdFront,
		BrokerID,
		UserID,
		Password,
		"./ctpmd_flow/", // 流文件存储路径
	)

	// 设置行情回调
	client.OnQuotes = func(quotes []*marketfeed.Quote) {
		for _, quote := range quotes {
			log.Printf("📊 行情更新: %s | 最新价: %.2f | 买一: %.2f x %d | 卖一: %.2f x %d | 成交量: %d | 持仓: %d",
				quote.InstrumentID,
				quote.LastPrice,
				quote.BidPrice1,
				quote.BidVolume1,
				quote.AskPrice1,
				quote.AskVolume1,
				quote.Volume,
				quote.OpenInterest,
			)
		}
	}

	// 启动客户端
	if err := client.Start(); err != nil {
		log.Fatalf("启动客户端失败: %v", err)
	}

	log.Println("✅ 客户端已启动，等待连接...")

	// 等待登录成功
	for i := 0; i < 30; i++ {
		if client.IsLoggedIn() {
			log.Println("✅ 登录成功！")
			break
		}
		time.Sleep(time.Second)
		if i == 29 {
			log.Fatal("❌ 登录超时")
		}
	}

	// 订阅行情
	symbols := []string{"ag2604", "au2604"}
	log.Printf("📡 订阅合约: %v", symbols)

	if err := client.Subscribe(symbols...); err != nil {
		log.Fatalf("订阅失败: %v", err)
	}

	// 等待订阅成功
	time.Sleep(2 * time.Second)

	log.Println("✅ 订阅成功，开始接收行情数据...")
	log.Printf("📅 交易日: %s", client.GetTradingDay())

	// 定期查询缓存的行情
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for range ticker.C {
			log.Println("\n=== 缓存行情查询 ===")
			for _, symbol := range symbols {
				quote := client.GetQuote(symbol)
				if quote != nil {
					log.Printf("  %s: 最新价=%.2f, 涨跌=%.2f (%.2f%%), 时间=%s",
						quote.InstrumentID,
						quote.LastPrice,
						quote.LastPrice-quote.PreSettlement,
						(quote.LastPrice-quote.PreSettlement)/quote.PreSettlement*100,
						quote.Datetime,
					)
				} else {
					log.Printf("  %s: 暂无数据", symbol)
				}
			}
		}
	}()

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("\n👋 收到退出信号，正在关闭...")

	ticker.Stop()
	client.Close()

	log.Println("✅ 程序已退出")
}
