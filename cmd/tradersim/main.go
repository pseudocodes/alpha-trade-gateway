// Package main 模拟交易服务入口
// 对应 C++ open-trade-sim/main.cpp
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
	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/marketfeed"
	"alpha-trade-gateway/pkg/tradersim"
	"alpha-trade-gateway/pkg/websocket"
)

// Config 配置
type Config struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	UserFilePath string `json:"user_file_path"`
	BrokerID     string `json:"broker_id"`
	LogLevel     string `json:"log_level"`
	MarketURL    string `json:"market_url"`
}

var defaultConfig = Config{
	Host:         "0.0.0.0",
	Port:         7788,
	UserFilePath: "./data/sim",
	BrokerID:     "sim",
	LogLevel:     "info",
	MarketURL:    "", // 使用默认天勤地址
}

func main() {
	// 命令行参数
	host := flag.String("host", defaultConfig.Host, "监听地址")
	port := flag.Int("port", defaultConfig.Port, "监听端口")
	userFilePath := flag.String("data", defaultConfig.UserFilePath, "用户数据存储路径")
	brokerID := flag.String("broker", defaultConfig.BrokerID, "经纪商ID")
	logLevel := flag.String("log", defaultConfig.LogLevel, "日志级别 (debug/info/warn/error)")
	marketURL := flag.String("market", defaultConfig.MarketURL, "行情服务地址 (留空使用默认天勤)")
	flag.Parse()

	// 初始化日志
	logCfg := &config.LogConfig{
		Level:   *logLevel,
		Console: true,
	}
	if err := logger.Init(logCfg); err != nil {
		fmt.Fprintf(os.Stderr, "init logger failed: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("tradersim starting",
		zap.String("host", *host),
		zap.Int("port", *port),
		zap.String("data_path", *userFilePath),
		zap.String("broker_id", *brokerID),
	)

	// 创建主 context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ====== 1. 启动 MarketFeed (天勤行情) ======
	marketClient := marketfeed.NewTqMarketClient(*marketURL)
	if err := marketClient.Start(); err != nil {
		logger.Error("start marketfeed failed", zap.Error(err))
		os.Exit(1)
	}
	logger.Info("marketfeed started")

	// ====== 2. 创建 TraderSim 实例 ======
	simConfig := &tradersim.Config{
		UserFilePath: *userFilePath,
		BrokerID:     *brokerID,
	}
	traderSim := tradersim.New(ctx, simConfig)
	traderSim.SetMarketClient(marketClient)

	// 启动模拟交易器
	if err := traderSim.Start(); err != nil {
		logger.Error("start tradersim failed", zap.Error(err))
		os.Exit(1)
	}

	// ====== 3. 启动 WebSocket 服务器 ======
	wsServer := websocket.NewServer(ctx, *host, *port, traderSim)

	go func() {
		if err := wsServer.Start(); err != nil {
			logger.Error("websocket server error", zap.Error(err))
			cancel()
		}
	}()

	logger.Info("tradersim ready",
		zap.String("addr", fmt.Sprintf("ws://%s:%d", *host, *port)),
	)

	// 信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	sig := <-sigChan
	logger.Info("received signal, shutting down",
		zap.String("signal", sig.String()),
	)

	// 停止服务
	wsServer.Stop()
	traderSim.Stop()
	marketClient.Close()

	logger.Info("tradersim exited")
}
