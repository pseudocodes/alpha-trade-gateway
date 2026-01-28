package marketfeed

// 编译时接口检查，确保实现正确
var (
	_ MarketClient = (*TqMarketClient)(nil)
	_ MarketClient = (*CtpMarketClient)(nil)
)

// MarketClient 行情客户端统一接口
// 抽象 TqMarketClient 和 CtpMarketClient 的公共行为
type MarketClient interface {
	// Start 启动客户端，连接到行情服务器
	Start() error

	// Subscribe 订阅合约行情
	Subscribe(symbols ...string) error

	// Unsubscribe 退订合约行情
	Unsubscribe(symbols ...string) error

	// GetQuote 获取指定合约的缓存行情
	GetQuote(symbol string) *Quote

	// GetAllQuotes 获取所有缓存的行情
	GetAllQuotes() []*Quote

	// SetOnQuotes 设置行情回调函数
	SetOnQuotes(callback func(quotes []*Quote))

	// Close 关闭客户端
	Close()
}

// MarketClientOption 行情客户端配置选项
type MarketClientOption func(*marketClientConfig)

type marketClientConfig struct {
	onQuotes func(quotes []*Quote)
}

// WithOnQuotes 设置行情回调
func WithOnQuotes(callback func(quotes []*Quote)) MarketClientOption {
	return func(cfg *marketClientConfig) {
		cfg.onQuotes = callback
	}
}

// MarketClientType 行情客户端类型
type MarketClientType string

const (
	// MarketClientTypeTq 天勤行情客户端
	MarketClientTypeTq MarketClientType = "tq"
	// MarketClientTypeCtp CTP 行情客户端
	MarketClientTypeCtp MarketClientType = "ctp"
)

// TqConfig 天勤行情客户端配置
type TqConfig struct {
	// WebSocket URL，为空则使用默认地址
	URL string
	// 认证 Token（可选）
	Token string
}

// CtpConfig CTP 行情客户端配置
type CtpConfig struct {
	// 行情前置地址，如 "tcp://182.254.243.31:40011"
	FrontAddr string
	// 期货公司代码
	BrokerID string
	// 用户名
	UserID string
	// 密码
	Password string
	// 流文件存储路径
	FlowPath string
}

// NewMarketClient 根据类型创建行情客户端
// 这是一个工厂函数，根据配置类型返回对应的实现
//
// 示例:
//
//	// 创建天勤客户端
//	client := NewMarketClient(TqConfig{URL: ""})
//
//	// 创建 CTP 客户端
//	client := NewMarketClient(CtpConfig{
//	    FrontAddr: "tcp://182.254.243.31:40011",
//	    BrokerID:  "9999",
//	    UserID:    "user",
//	    Password:  "pass",
//	})
func NewMarketClient(config interface{}, opts ...MarketClientOption) MarketClient {
	cfg := &marketClientConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	switch c := config.(type) {
	case TqConfig:
		client := NewTqMarketClient(c.URL)
		if cfg.onQuotes != nil {
			client.OnQuotes = cfg.onQuotes
		}
		return client
	case CtpConfig:
		client := NewCtpMarketClient(c.FrontAddr, c.BrokerID, c.UserID, c.Password, c.FlowPath)
		if cfg.onQuotes != nil {
			client.OnQuotes = cfg.onQuotes
		}
		return client
	default:
		panic("unsupported market client config type")
	}
}

// MarketFeedConfig 行情服务配置（用于从配置文件加载）
// 与 config.MarketFeedConfig 对应
type MarketFeedConfig struct {
	Type    string    `json:"type" mapstructure:"type"`
	Symbols []string  `json:"symbols" mapstructure:"symbols"`
	Tq      TqConfig  `json:"tq" mapstructure:"tq"`
	Ctp     CtpConfig `json:"ctp" mapstructure:"ctp"`
}

// NewMarketClientFromConfig 从配置创建行情客户端
// 根据配置中的 Type 字段选择创建对应类型的客户端
//
// 示例:
//
//	cfg := marketfeed.MarketFeedConfig{
//	    Type: "tq",
//	    Tq: marketfeed.TqConfig{URL: ""},
//	}
//	client := marketfeed.NewMarketClientFromConfig(cfg)
func NewMarketClientFromConfig(cfg MarketFeedConfig, opts ...MarketClientOption) MarketClient {
	switch MarketClientType(cfg.Type) {
	case MarketClientTypeTq:
		return NewMarketClient(cfg.Tq, opts...)
	case MarketClientTypeCtp:
		return NewMarketClient(cfg.Ctp, opts...)
	default:
		// 默认使用天勤
		return NewMarketClient(cfg.Tq, opts...)
	}
}
