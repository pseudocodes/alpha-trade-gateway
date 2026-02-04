package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"alpha-trade-gateway/pkg/inslist"
	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/marketfeed"
)

/*
InstrumentService 使用示例

本示例演示如何使用 InstrumentService 作为合约信息和行情数据的统一来源：
1. 创建 InstrumentService（从天勤加载合约列表）
2. 创建 MarketFeed（天勤行情）
3. 将 MarketFeed 的行情回调连接到 InstrumentService
4. 通过 InstrumentService 获取合约信息和最新行情

架构说明：
  - InstrumentService 是行情数据的唯一来源
  - MarketFeed 负责订阅和接收行情，但不知道 InstrumentService 的存在
  - 启动层负责将两者连接起来

运行方式：
  cd examples/insservice
  go run insservice.go
*/

func main() {
	// 初始化日志
	logger.InitDefault()

	log.Println("=== InstrumentService Example ===")

	// ====== 1. 创建 InstrumentService ======
	// 使用天勤模式，从天勤 URL 加载合约列表
	insServiceCfg := inslist.Config{
		Source:     inslist.SourceTianqin,
		TianqinURL: inslist.DefaultInsListURL,
	}
	insService := inslist.NewInstrumentService(insServiceCfg)

	// 初始化（从天勤加载合约列表）
	log.Println("📥 正在从天勤加载合约列表...")
	if err := insService.Init(); err != nil {
		log.Fatalf("初始化 InstrumentService 失败: %v", err)
	}
	log.Printf("✅ 合约列表加载完成，共 %d 个合约", insService.Count())

	// 查看一些合约信息
	showInstrumentInfo(insService, "SHFE.au2606")
	showInstrumentInfo(insService, "DCE.m2609")
	showInstrumentInfo(insService, "CFFEX.IF2606")

	mdUrl := os.Getenv("MD_URL")
	// ====== 2. 创建 MarketFeed ======
	marketClient := marketfeed.NewTqMarketClient(mdUrl)

	// ====== 3. 将 MarketFeed 的行情回调连接到 InstrumentService ======
	// 这是关键：InstrumentService 提供 OnQuotes 方法，可直接注册
	marketClient.SetOnQuotes(insService.OnQuotes)

	// 启动行情客户端
	log.Println("📡 正在连接天勤行情服务...")
	if err := marketClient.Start(); err != nil {
		log.Fatalf("启动行情客户端失败: %v", err)
	}
	log.Println("✅ 行情服务已连接")

	// ====== 4. 订阅行情 ======
	symbols := []string{"SHFE.au2606", "SHFE.ag2606", "DCE.m2609"}
	log.Printf("📡 订阅合约: %v", symbols)
	if err := marketClient.Subscribe(symbols...); err != nil {
		log.Fatalf("订阅失败: %v", err)
	}

	// 等待行情数据
	time.Sleep(3 * time.Second)

	// ====== 5. 通过 InstrumentService 获取最新行情 ======
	log.Println("\n=== 通过 InstrumentService 获取行情 ===")
	for _, symbol := range symbols {
		showInstrumentWithQuote(insService, symbol)
	}

	// 定期显示行情更新
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			log.Println("\n=== 行情更新 ===")
			for _, symbol := range symbols {
				ins := insService.GetInstrument(symbol)
				if ins != nil && ins.LastPrice > 0 {
					log.Printf("  %s: 最新价=%.2f, 买一=%.2f, 卖一=%.2f",
						symbol, ins.LastPrice, ins.BidPrice1, ins.AskPrice1)
				}
			}
		}
	}()

	// 演示其他功能
	log.Println("\n=== 其他功能演示 ===")

	// 根据合约代码猜测交易所
	instrumentID := "au2606"
	exchangeID := insService.GuessExchangeID(instrumentID)
	log.Printf("GuessExchangeID(%s) = %s", instrumentID, exchangeID)

	// 获取完整 symbol
	fullSymbol := insService.GetSymbol(instrumentID)
	log.Printf("GetSymbol(%s) = %s", instrumentID, fullSymbol)

	// 判断合约类型
	log.Printf("IsFutures(SHFE.au2606) = %v", insService.IsFutures("SHFE.au2606"))
	log.Printf("IsOption(SHFE.au2606) = %v", insService.IsOption("SHFE.au2606"))

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("\n👋 收到退出信号，正在关闭...")
	ticker.Stop()
	marketClient.Close()
	log.Println("✅ 程序已退出")
}

// showInstrumentInfo 显示合约静态信息
func showInstrumentInfo(svc *inslist.InstrumentService, symbol string) {
	ins := svc.GetInstrument(symbol)
	if ins == nil {
		log.Printf("❌ 合约 %s 不存在", symbol)
		return
	}

	log.Printf("📋 合约信息: %s", symbol)
	log.Printf("   交易所: %s, 合约代码: %s", ins.ExchangeID, ins.InstrumentID)
	log.Printf("   合约乘数: %d, 最小变动: %.4f", ins.VolumeMultiple, ins.PriceTick)
	log.Printf("   保证金: %.2f, 手续费: %.4f", ins.Margin, ins.Commission)

	productClass := "未知"
	switch ins.ProductClass {
	case 1:
		productClass = "期货"
	case 2:
		productClass = "期权"
	case 3:
		productClass = "组合"
	case 4:
		productClass = "即期"
	case 5:
		productClass = "期转现"
	}
	log.Printf("   产品类型: %s, 是否过期: %v", productClass, ins.Expired)
}

// showInstrumentWithQuote 显示合约信息和最新行情
func showInstrumentWithQuote(svc *inslist.InstrumentService, symbol string) {
	ins := svc.GetInstrument(symbol)
	if ins == nil {
		log.Printf("❌ 合约 %s 不存在", symbol)
		return
	}

	// 静态信息
	fmt.Printf("\n📊 %s (%s.%s)\n", symbol, ins.ExchangeID, ins.InstrumentID)
	fmt.Printf("   合约乘数: %d, 最小变动: %.4f\n", ins.VolumeMultiple, ins.PriceTick)

	// 动态行情
	if ins.LastPrice > 0 {
		fmt.Printf("   最新价: %.2f\n", ins.LastPrice)
		fmt.Printf("   买一: %.2f, 卖一: %.2f\n", ins.BidPrice1, ins.AskPrice1)
		fmt.Printf("   涨停: %.2f, 跌停: %.2f\n", ins.UpperLimit, ins.LowerLimit)
		fmt.Printf("   昨结算: %.2f, 昨收盘: %.2f\n", ins.PreSettlement, ins.PreClose)
	} else {
		fmt.Printf("   (暂无行情数据)\n")
	}
}
