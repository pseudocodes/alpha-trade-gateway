// Package main 主程序入口
// 对应 C++ open-trade-ctpse15/main.cpp
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/config"
	"alpha-trade-gateway/pkg/inslist"
	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/marketfeed"
	"alpha-trade-gateway/pkg/trader"
	"alpha-trade-gateway/pkg/websocket"
)

func main() {
	// 命令行参数
	// C++ 版本接收 key 作为用户标识，Go 版本作为单用户服务，通过 WebSocket 接收用户信息
	configPath := flag.String("config", "", "配置文件路径")
	flag.Parse()

	// 加载配置文件
	// 对应 C++ LoadConfig()
	if err := config.Load(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	if err := logger.Init(&config.Global.Log); err != nil {
		fmt.Fprintf(os.Stderr, "init logger failed: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("trade ctpse15-go starting",
		zap.String("host", config.Global.Host),
		zap.Int("port", config.Global.Port),
	)

	// 创建主 context，用于优雅关闭
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ====== 1. 创建 InstrumentService (合约信息服务) ======
	// InstrumentService 是行情数据的唯一来源，trader 通过它获取最新行情
	// 对应 C++ GetInstrument() 全局函数
	insServiceCfg := inslist.Config{
		Source:     inslist.SourceCTP, // trader 使用 CTP 查询填充合约信息
		TianqinURL: inslist.DefaultInsListURL,
	}
	insService := inslist.NewInstrumentService(insServiceCfg)

	// ====== 2. 启动 MarketFeed (行情服务) ======
	// MarketFeed 是基础服务，需要先于 WebSocket 启动
	marketClient := createMarketFeedClient()

	// 将 MarketFeed 的行情回调连接到 InstrumentService
	// InstrumentService 提供 OnQuotes 方法，可直接注册
	marketClient.SetOnQuotes(insService.OnQuotes)

	if err := marketClient.Start(); err != nil {
		logger.Error("start marketfeed failed", zap.Error(err))
		os.Exit(1)
	}
	logger.Info("marketfeed started", zap.String("type", config.Global.MarketFeed.Type))

	// ====== 3. 创建 TraderCTP 实例 ======
	// 对应 C++ traderctp tradeCtp(ioc, key)
	traderCTP := trader.New(ctx)
	// 注入 InstrumentService (trader 只与 InstrumentService 交互)
	traderCTP.SetInstrumentService(insService)
	// 注入 MarketClient (仅用于订阅行情，不用于获取行情数据)
	traderCTP.SetMarketClient(marketClient)

	// ====== 4. 启动 WebSocket 服务器 ======
	// 对应 C++ 的 IPC 消息队列，这里改为 WebSocket 服务
	wsServer := websocket.NewServer(ctx, config.Global.Host, config.Global.Port, traderCTP)

	// 启动 WebSocket 服务器
	go func() {
		if err := wsServer.Start(); err != nil {
			logger.Error("websocket server error", zap.Error(err))
			cancel()
		}
	}()

	// 信号处理
	// 对应 C++ signals_.add(SIGINT); signals_.add(SIGTERM); signals_.add(SIGQUIT);
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	sig := <-sigChan
	logger.Info("received signal, shutting down",
		zap.String("signal", sig.String()),
	)

	// 停止服务 (按启动的逆序关闭)
	// 对应 C++ tradeCtp.Stop()
	wsServer.Stop()
	traderCTP.Stop()
	marketClient.Close()

	logger.Info("tradectp exited")
}

// createMarketFeedClient 根据配置创建行情客户端
func createMarketFeedClient() marketfeed.MarketClient {
	cfg := marketfeed.MarketFeedConfig{
		Type:    config.Global.MarketFeed.Type,
		Symbols: config.Global.MarketFeed.Symbols,
		Tq: marketfeed.TqConfig{
			URL:   config.Global.MarketFeed.Tq.URL,
			Token: config.Global.MarketFeed.Tq.Token,
		},
		Ctp: marketfeed.CtpConfig{
			FrontAddr: config.Global.MarketFeed.Ctp.FrontAddr,
			BrokerID:  config.Global.MarketFeed.Ctp.BrokerID,
			UserID:    config.Global.MarketFeed.Ctp.UserID,
			Password:  config.Global.MarketFeed.Ctp.Password,
			FlowPath:  config.Global.MarketFeed.Ctp.FlowPath,
		},
	}

	// 如果类型为空，默认使用天勤
	if cfg.Type == "" {
		cfg.Type = "tq"
	}

	return marketfeed.NewMarketClientFromConfig(cfg)
}
