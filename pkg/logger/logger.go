// Package logger 日志操作
// 对应 C++ open-trade-common/log.h/cpp
// 使用 zap 实现结构化日志
//
// 使用方式：
//
//	import "alpha-trade-gateway/pkg/logger"
//	logger.L.Info("message", zap.String("key", "value"))
//	logger.S.Infof("formatted %s", "message")
package logger

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"alpha-trade-gateway/pkg/config"
)

var (
	// L 全局 logger 实例，直接使用: logger.L.Info("msg", zap.String("k", "v"))
	L *zap.Logger
	// S 全局 sugar logger 实例: logger.S.Infof("msg %s", val)
	S *zap.SugaredLogger
	// wrapper 内部使用，带 CallerSkip 的 logger，用于包装函数
	wrapper *zap.Logger
)

// Init 初始化日志
func Init(cfg *config.LogConfig) error {
	level := parseLevel(cfg.Level)
	// 基础编码配置
	baseEncoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	var cores []zapcore.Core

	// 控制台输出（带颜色）
	if cfg.Console {
		consoleEncoderConfig := baseEncoderConfig
		consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder // 使用彩色级别
		consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderConfig)
		consoleSyncer := zapcore.AddSync(os.Stdout)
		cores = append(cores, zapcore.NewCore(consoleEncoder, consoleSyncer, level))
	}

	// 文件输出（不带颜色，使用 JSON 格式）
	if cfg.Filename != "" {
		dir := filepath.Dir(cfg.Filename)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		file, err := os.OpenFile(cfg.Filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		fileEncoder := zapcore.NewJSONEncoder(baseEncoderConfig)
		fileSyncer := zapcore.AddSync(file)
		cores = append(cores, zapcore.NewCore(fileEncoder, fileSyncer, level))
	}

	// 合并多个 core
	core := zapcore.NewTee(cores...)
	L = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	S = L.Sugar()
	// wrapper 跳过一层调用栈，用于包装函数
	wrapper = L.WithOptions(zap.AddCallerSkip(1))

	return nil
}

// InitDefault 使用默认配置初始化日志
func InitDefault() {
	cfg := &config.LogConfig{
		Level:   "info",
		Console: true,
	}
	_ = Init(cfg)
}

func parseLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// Field 便捷函数，对应 C++ WithField
func Field(key string, val interface{}) zap.Field {
	return zap.Any(key, val)
}

// Debug 记录 debug 级别日志
// 使用 wrapper (带 CallerSkip) 确保 caller 显示真实调用位置
func Debug(msg string, fields ...zap.Field) {
	wrapper.Debug(msg, fields...)
}

// Info 记录 info 级别日志
func Info(msg string, fields ...zap.Field) {
	wrapper.Info(msg, fields...)
}

// Warn 记录 warn 级别日志
func Warn(msg string, fields ...zap.Field) {
	wrapper.Warn(msg, fields...)
}

// Error 记录 error 级别日志
func Error(msg string, fields ...zap.Field) {
	wrapper.Error(msg, fields...)
}

// Fatal 记录 fatal 级别日志并退出
func Fatal(msg string, fields ...zap.Field) {
	wrapper.Fatal(msg, fields...)
}

// With 创建带有固定字段的 logger
func With(fields ...zap.Field) *zap.Logger {
	return L.With(fields...)
}

// Sync 同步日志缓冲
func Sync() {
	if L != nil {
		L.Sync()
	}
}
