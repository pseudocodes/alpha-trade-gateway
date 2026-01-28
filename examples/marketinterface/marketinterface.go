package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/marketfeed"
)

/*
使用 MarketClient 统一接口的示例

支持通过命令行参数切换不同的行情数据源：
  - tq: 天勤行情（WebSocket）
  - ctp: CTP 行情（原生协议）

运行方式：
  cd examples/marketinterface
  go run marketinterface.go -source=tq
  go run marketinterface.go -source=ctp
*/

var (
	source   = flag.String("source", "tq", "行情数据源: tq 或 ctp")
	symbols  = flag.String("symbols", "SHFE.ag2601,SHFE.au2604", "订阅合约列表，逗号分隔")
	brokerID = flag.String("broker", "9999", "CTP BrokerID")
	userID   = flag.String("user", "046111", "CTP UserID")
	password = flag.String("pass", "046111", "CTP Password")
	mdFront  = flag.String("front", "tcp://182.254.243.31:40011", "CTP 行情前置地址")
	tqURL    = flag.String("tqurl", "", "天勤 WebSocket 地址（留空使用默认）")
)

func main() {
	flag.Parse()

	// 初始化日志
	logger.InitDefault()

	log.Printf("=== MarketClient 统一接口示例 ===")
	log.Printf("数据源: %s", *source)

	// 解析订阅合约列表
	symbolList := parseSymbols(*symbols)
	log.Printf("订阅合约: %v", symbolList)

	// 根据数据源创建行情客户端
	var client marketfeed.MarketClient

	switch *source {
	case "tq":
		log.Println("使用天勤行情数据源")
		client = marketfeed.NewMarketClient(
			marketfeed.TqConfig{
				URL: *tqURL,
			},
			marketfeed.WithOnQuotes(onQuotesHandler),
		)
	case "ctp":
		log.Println("使用 CTP 行情数据源")
		client = marketfeed.NewMarketClient(
			marketfeed.CtpConfig{
				FrontAddr: *mdFront,
				BrokerID:  *brokerID,
				UserID:    *userID,
				Password:  *password,
				FlowPath:  "./ctpmd_flow/",
			},
			marketfeed.WithOnQuotes(onQuotesHandler),
		)
	default:
		log.Fatalf("不支持的数据源: %s", *source)
	}

	// 启动客户端
	if err := client.Start(); err != nil {
		log.Fatalf("启动客户端失败: %v", err)
	}
	log.Println("✅ 客户端已启动")

	// 等待一段时间让连接建立
	time.Sleep(3 * time.Second)

	// 订阅行情
	if err := client.Subscribe(symbolList...); err != nil {
		log.Printf("订阅失败: %v", err)
	}
	log.Println("✅ 订阅请求已发送")

	// 定期打印缓存的行情
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for range ticker.C {
			printCachedQuotes(client, symbolList)
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

// onQuotesHandler 行情回调处理
func onQuotesHandler(quotes []*marketfeed.Quote) {
	for _, quote := range quotes {
		log.Printf("📊 [%s] 最新: %.2f | 买一: %.2f x %d | 卖一: %.2f x %d | 量: %d",
			quote.InstrumentID,
			quote.LastPrice,
			quote.BidPrice1,
			quote.BidVolume1,
			quote.AskPrice1,
			quote.AskVolume1,
			quote.Volume,
		)
	}
}

// printCachedQuotes 打印缓存的行情
func printCachedQuotes(client marketfeed.MarketClient, symbols []string) {
	log.Println("\n=== 缓存行情查询 ===")

	// 方式1: 查询指定合约
	for _, symbol := range symbols {
		quote := client.GetQuote(symbol)
		if quote != nil {
			log.Printf("  [%s] 最新: %.2f | 时间: %s",
				quote.InstrumentID,
				quote.LastPrice,
				quote.Datetime,
			)
		} else {
			log.Printf("  [%s] 暂无数据", symbol)
		}
	}

	// 方式2: 获取所有缓存的行情
	allQuotes := client.GetAllQuotes()
	log.Printf("  共缓存 %d 个合约行情", len(allQuotes))
}

// parseSymbols 解析合约列表
func parseSymbols(s string) []string {
	var result []string
	for _, sym := range splitAndTrim(s, ",") {
		if sym != "" {
			result = append(result, sym)
		}
	}
	return result
}

// splitAndTrim 分割并去除空白
func splitAndTrim(s string, sep string) []string {
	var result []string
	for _, part := range split(s, sep) {
		trimmed := trim(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func split(s string, sep string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if string(s[i]) == sep {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trim(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
