// Package config 配置文件读写
// 对应 C++ open-trade-common/config.h/cpp
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// BrokerConfig 经纪商配置
// 对应 C++ BrokerConfig 结构体
type BrokerConfig struct {
	BrokerName    string   `json:"name" mapstructure:"name"`
	BrokerType    string   `json:"type" mapstructure:"type"`
	IsFens        bool     `json:"is_fens" mapstructure:"is_fens"`
	CtpBrokerID   string   `json:"broker_id" mapstructure:"broker_id"`
	TradingFronts []string `json:"trading_fronts" mapstructure:"trading_fronts"`
	AppID         string   `json:"app_id" mapstructure:"app_id"`
	ProductInfo   string   `json:"product_info" mapstructure:"product_info"`
	AuthCode      string   `json:"auth_code" mapstructure:"auth_code"`
}

// Config 全局配置
// 对应 C++ Config 结构体
type Config struct {
	// 服务 IP 及端口号
	Host string `json:"host" mapstructure:"host"`
	Port int    `json:"port" mapstructure:"port"`

	// 用户配置文件路径
	UserFilePath string `json:"user_file_path" mapstructure:"user_file_path"`

	// 是否要求用户确认结算单
	AutoConfirmSettlement bool `json:"auto_confirm_settlement" mapstructure:"auto_confirm_settlement"`

	// 是否打印行情日志
	LogPriceInfo bool `json:"log_price_info" mapstructure:"log_price_info"`

	// 经纪商配置
	Brokers map[string]BrokerConfig `json:"-"`

	// 经纪商列表 JSON 字符串 (用于发送给客户端)
	BrokerListStr string `json:"-"`

	// 当前交易日
	TradingDay string `json:"-"`

	// 日志配置
	Log LogConfig `json:"log" mapstructure:"log"`

	// CTP 相关配置
	CTP CTPConfig `json:"ctp" mapstructure:"ctp"`

	// 行情服务配置
	MarketFeed MarketFeedConfig `json:"marketfeed" mapstructure:"marketfeed"`

	// 条件单配置
	ConditionOrder ConditionOrderConfig `json:"condition_order" mapstructure:"condition_order"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level    string `json:"level" mapstructure:"level"`
	Filename string `json:"filename" mapstructure:"filename"`
	Console  bool   `json:"console" mapstructure:"console"`
}

// CTPConfig CTP 相关配置
type CTPConfig struct {
	FlowPath       string `json:"flow_path" mapstructure:"flow_path"`
	UseDynamicLib  bool   `json:"use_dynamic_lib" mapstructure:"use_dynamic_lib"`
	DynamicLibPath string `json:"dynamic_lib_path" mapstructure:"dynamic_lib_path"`
}

// MarketFeedConfig 行情服务配置
// 支持两种行情数据源：tq（天勤）和 ctp（CTP原生）
type MarketFeedConfig struct {
	// Type 行情数据源类型: "tq" 或 "ctp"
	Type string `json:"type" mapstructure:"type"`

	// Symbols 默认订阅的合约列表
	Symbols []string `json:"symbols" mapstructure:"symbols"`

	// Tq 天勤行情配置（当 Type 为 "tq" 时使用）
	Tq TqMarketFeedConfig `json:"tq" mapstructure:"tq"`

	// Ctp CTP 行情配置（当 Type 为 "ctp" 时使用）
	Ctp CtpMarketFeedConfig `json:"ctp" mapstructure:"ctp"`
}

// TqMarketFeedConfig 天勤行情配置
type TqMarketFeedConfig struct {
	// URL WebSocket 地址，留空使用默认地址
	URL string `json:"url" mapstructure:"url"`
	// Token 认证 Token（可选）
	Token string `json:"token" mapstructure:"token"`
}

// CtpMarketFeedConfig CTP 行情配置
type CtpMarketFeedConfig struct {
	// FrontAddr 行情前置地址，如 "tcp://182.254.243.31:40011"
	FrontAddr string `json:"front_addr" mapstructure:"front_addr"`
	// BrokerID 期货公司代码
	BrokerID string `json:"broker_id" mapstructure:"broker_id"`
	// UserID 用户名
	UserID string `json:"user_id" mapstructure:"user_id"`
	// Password 密码
	Password string `json:"password" mapstructure:"password"`
	// FlowPath 流文件存储路径
	FlowPath string `json:"flow_path" mapstructure:"flow_path"`
}

// ConditionOrderConfig 条件单配置
type ConditionOrderConfig struct {
	// Enabled 是否启用条件单功能
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	// DataPath 数据存储路径
	DataPath string `json:"data_path" mapstructure:"data_path"`
	// MaxNewOrdersPerDay 每日最大新建条件单数量
	MaxNewOrdersPerDay int `json:"max_new_orders_per_day" mapstructure:"max_new_orders_per_day"`
	// MaxValidOrdersTotal 最大有效条件单总数
	MaxValidOrdersTotal int `json:"max_valid_orders_total" mapstructure:"max_valid_orders_total"`
}

// Global 全局配置实例
var Global *Config

// Load 加载配置文件
// 对应 C++ LoadConfig() 函数
// 支持 JSON 和 TOML 格式，优先 JSON
func Load(configPath string) error {
	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		// 默认配置路径
		viper.SetConfigName("config")
		viper.AddConfigPath("./config")
		viper.AddConfigPath("/etc/open-trade-gateway")
	}

	// 设置默认值
	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	Global = &Config{}
	if err := viper.Unmarshal(Global); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	// 初始化 Brokers map
	Global.Brokers = make(map[string]BrokerConfig)

	// 加载 broker_list.json
	if err := loadBrokerList(); err != nil {
		// broker 列表加载失败不是致命错误
		fmt.Printf("warning: load broker list: %v\n", err)
	}

	return nil
}

// LoadFromDir 从目录加载配置 (优先 JSON, 其次 TOML)
func LoadFromDir(configDir string) error {
	viper.AddConfigPath(configDir)

	// 优先尝试 JSON
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	if err := viper.ReadInConfig(); err == nil {
		return unmarshalConfig()
	}

	// 其次尝试 TOML
	viper.SetConfigType("toml")
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	return unmarshalConfig()
}

func unmarshalConfig() error {
	setDefaults()
	Global = &Config{}
	if err := viper.Unmarshal(Global); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}
	Global.Brokers = make(map[string]BrokerConfig)
	return loadBrokerList()
}

func setDefaults() {
	viper.SetDefault("host", "0.0.0.0")
	viper.SetDefault("port", 7788)
	viper.SetDefault("auto_confirm_settlement", true)
	viper.SetDefault("log_price_info", false)
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.console", true)
	viper.SetDefault("ctp.flow_path", "./data/")
	viper.SetDefault("ctp.use_dynamic_lib", false)

	// 行情服务默认配置
	viper.SetDefault("marketfeed.type", "tq")
	viper.SetDefault("marketfeed.tq.url", "wss://openmd.shinnytech.com/t/md/front/mobile")
	viper.SetDefault("marketfeed.ctp.flow_path", "./ctpmd_flow/")

	// 条件单默认配置
	viper.SetDefault("condition_order.enabled", false)
	viper.SetDefault("condition_order.data_path", "./data/condition_orders")
	viper.SetDefault("condition_order.max_new_orders_per_day", 100)
	viper.SetDefault("condition_order.max_valid_orders_total", 500)
}

// loadBrokerList 加载经纪商列表
// broker_list.json 是纯 JSON 数组格式，需要用 encoding/json 直接读取
func loadBrokerList() error {
	brokerListPath := viper.GetString("broker_list_path")
	if brokerListPath == "" {
		// 尝试默认路径
		configDir := filepath.Dir(viper.ConfigFileUsed())
		brokerListPath = filepath.Join(configDir, "broker_list.json")
	}

	data, err := os.ReadFile(brokerListPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // broker 列表文件不存在，不是错误
		}
		return fmt.Errorf("read broker list file: %w", err)
	}

	var brokerList []BrokerConfig
	if err := json.Unmarshal(data, &brokerList); err != nil {
		return fmt.Errorf("unmarshal broker list: %w", err)
	}

	for _, b := range brokerList {
		Global.Brokers[b.BrokerName] = b
	}

	// 生成 broker_list_str (用于发送给客户端)
	Global.BrokerListStr = generateBrokerListStr(brokerList)

	return nil
}

func generateBrokerListStr(brokers []BrokerConfig) string {
	var names []string
	for _, b := range brokers {
		names = append(names, b.BrokerName)
	}
	// 简化版本，实际应该生成完整的 JSON
	return fmt.Sprintf(`{"aid":"rtn_brokers","brokers":[%s]}`,
		`"`+strings.Join(names, `","`)+`"`)
}

// GetBroker 获取经纪商配置
func GetBroker(brokerName string) (BrokerConfig, bool) {
	if Global == nil {
		return BrokerConfig{}, false
	}
	b, ok := Global.Brokers[brokerName]
	return b, ok
}
