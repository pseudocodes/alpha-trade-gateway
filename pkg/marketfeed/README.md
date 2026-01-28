# Market Feed Client

行情客户端库，提供统一的接口支持多种行情数据源。

## 功能特性

- ✅ **统一接口**: `MarketClient` 接口抽象，方便切换数据源
- ✅ **多数据源支持**: 天勤行情（WebSocket）、CTP 行情（原生协议）
- ✅ 封装 CTP MdApi，提供简洁的 Go 接口
- ✅ 自动重连和重新订阅
- ✅ 行情数据缓存
- ✅ 支持多合约订阅
- ✅ 统一的 Quote 数据结构

## 快速开始

### 使用统一接口（推荐）

```go
import "alpha-trade-gateway/pkg/marketfeed"

// 创建行情客户端（使用工厂函数）
// 方式1: 天勤数据源
client := marketfeed.NewMarketClient(
    marketfeed.TqConfig{URL: ""},  // 使用默认 URL
    marketfeed.WithOnQuotes(func(quotes []*marketfeed.Quote) {
        for _, q := range quotes {
            log.Printf("%s: %.2f", q.InstrumentID, q.LastPrice)
        }
    }),
)

// 方式2: CTP 数据源
client := marketfeed.NewMarketClient(
    marketfeed.CtpConfig{
        FrontAddr: "tcp://182.254.243.31:40011",
        BrokerID:  "9999",
        UserID:    "your_user",
        Password:  "your_pass",
        FlowPath:  "./ctpmd_flow/",
    },
    marketfeed.WithOnQuotes(onQuotesHandler),
)

// 启动、订阅、获取行情（接口统一）
client.Start()
client.Subscribe("ag2601", "au2604")
quote := client.GetQuote("ag2601")
client.Close()
```

### 直接使用具体实现

#### CTP 行情客户端

```go
import "alpha-trade-gateway/pkg/marketfeed"

// 创建客户端
client := marketfeed.NewCtpMarketClient(
    "tcp://182.254.243.31:40011", // 行情前置地址
    "9999",                        // BrokerID
    "your_user_id",                // 用户名
    "your_password",               // 密码
    "./ctpmd_flow/",               // 流文件路径
)

// 设置回调
client.OnQuotes = func(quotes []*marketfeed.Quote) {
    for _, quote := range quotes {
        log.Printf("行情: %s, 价格: %.2f", quote.InstrumentID, quote.LastPrice)
    }
}

// 启动
client.Start()

// 订阅合约
client.Subscribe("ag2412", "au2412", "cu2412")

// 查询缓存的行情
quote := client.GetQuote("ag2412")
```

### 2. 运行示例

```bash
cd examples
go run ctp_market_example.go
```

### 3. 测试环境

**Simnow 7x24 环境**（可24小时测试）:
- 行情前置: `tcp://182.254.243.31:40011`
- BrokerID: `9999`
- 测试账号: 在 [Simnow](http://www.simnow.com.cn/) 注册

**Simnow 仿真环境**（交易时段同实盘）:
- 仿真1: `tcp://182.254.243.31:30011`
- 仿真2: `tcp://182.254.243.31:30012`

## API 说明

### MarketClient 接口

统一的行情客户端接口，`TqMarketClient` 和 `CtpMarketClient` 都实现了此接口。

```go
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
```

#### 工厂函数

```go
// 使用配置创建客户端
client := marketfeed.NewMarketClient(config, opts...)

// 配置类型
type TqConfig struct {
    URL   string  // WebSocket URL（可选）
    Token string  // 认证 Token（可选）
}

type CtpConfig struct {
    FrontAddr string  // 行情前置地址
    BrokerID  string  // 期货公司代码
    UserID    string  // 用户名
    Password  string  // 密码
    FlowPath  string  // 流文件路径
}

// 可选参数
marketfeed.WithOnQuotes(callback)  // 设置行情回调
```

### CtpMarketClient

主要的行情客户端类。

#### 方法

- `NewCtpMarketClient(frontAddr, brokerID, userID, password, flowPath)` - 创建客户端
- `Start() error` - 启动客户端，连接到 CTP 服务器
- `Subscribe(symbols ...string) error` - 订阅合约行情
- `Unsubscribe(symbols ...string) error` - 退订合约行情
- `GetQuote(symbol string) *Quote` - 获取缓存的行情数据
- `GetAllQuotes() []*Quote` - 获取所有缓存的行情
- `IsConnected() bool` - 是否已连接
- `IsLoggedIn() bool` - 是否已登录
- `GetTradingDay() string` - 获取当前交易日
- `Close()` - 关闭客户端

#### 回调

- `OnQuotes func([]*Quote)` - 行情更新回调

### CtpConnection

底层的 CTP MdApi 封装。

#### 方法

- `NewCtpConnection(frontAddr, brokerID, userID, password, flowPath)` - 创建连接
- `Connect() error` - 连接服务器
- `Subscribe(symbols ...string) error` - 订阅
- `Unsubscribe(symbols ...string) error` - 退订
- `Close()` - 关闭

#### 回调

- `OnConnected func()` - 连接成功
- `OnDisconnected func(reason int)` - 连接断开
- `OnLogin func(success bool, errorMsg string)` - 登录结果
- `OnMarketData func(quote *Quote)` - 行情数据
- `OnSubscribed func(symbol string, success bool)` - 订阅结果

### Quote 结构

统一的行情数据结构，兼容天勤和 CTP。

```go
type Quote struct {
    InstrumentID    string  // 合约代码
    Datetime        string  // 时间
    LastPrice       float64 // 最新价
    BidPrice1       float64 // 买一价
    BidVolume1      int     // 买一量
    AskPrice1       float64 // 卖一价
    AskVolume1      int     // 卖一量
    Volume          int     // 成交量
    OpenInterest    int     // 持仓量
    // ... 更多字段
}
```

## 架构说明

```
                    ┌─────────────────────────────────┐
                    │        MarketClient 接口         │
                    │  - Start() / Close()            │
                    │  - Subscribe() / Unsubscribe()  │
                    │  - GetQuote() / GetAllQuotes()  │
                    │  - SetOnQuotes()                │
                    └───────────────┬─────────────────┘
                                    │
            ┌───────────────────────┼───────────────────────┐
            │                       │                       │
            v                       v                       v
┌───────────────────────┐ ┌───────────────────────┐ ┌───────────────┐
│   TqMarketClient      │ │   CtpMarketClient     │ │   其他实现...  │
│  - WebSocket 连接     │ │  - CTP 原生协议       │ │               │
│  - 天勤行情           │ │  - 期货公司行情       │ │               │
└───────────┬───────────┘ └───────────┬───────────┘ └───────────────┘
            │                         │
            v                         v
┌───────────────────────┐ ┌───────────────────────┐
│   TqConnection        │ │   CtpConnection       │
│  - coder/websocket    │ │  - go2ctp/ctp         │
└───────────────────────┘ └───────────┬───────────┘
                                      │
                                      v
                          ┌───────────────────────┐
                          │  CTP MdApi (go2ctp)   │
                          │  - BaseMdSpi          │
                          │  - C++ API 绑定       │
                          └───────────────────────┘
```

## 与 TqMarketClient 的对比

| 特性 | TqMarketClient | CtpMarketClient |
|------|----------------|-----------------|
| 协议 | WebSocket | CTP 原生协议 |
| 连接方式 | 天勤行情服务器 | CTP 行情前置 |
| 认证 | Token/无需认证 | BrokerID + UserID + Password |
| 数据源 | 天勤行情 | 期货公司/模拟环境 |
| 依赖 | coder/websocket | go2ctp |
| 使用场景 | 快速开发、测试 | 生产环境、实盘交易 |

## 注意事项

1. **流文件路径**: CTP 会在指定路径生成流文件，用于断线重连，请确保有写权限
2. **自动重连**: 断线后会自动重连并重新订阅之前的合约
3. **合约代码**: 使用标准的期货合约代码，如 `ag2412`（白银2024年12月）
4. **价格精度**: CTP 返回的无效价格会被转换为 `NaN`
5. **并发安全**: 所有方法都是并发安全的

## 常见问题

### Q: 如何获取测试账号？

A: 访问 [Simnow](http://www.simnow.com.cn/) 注册模拟账号。

### Q: 为什么收不到行情？

A: 检查：
1. 是否成功登录（查看日志）
2. 合约代码是否正确
3. 是否在交易时段（非7x24环境）
4. 网络是否正常

### Q: 如何处理 GBK 编码？

A: CTP 返回的中文错误信息是 GBK 编码，需要转换为 UTF-8。可以使用 `golang.org/x/text/encoding/simplifiedchinese` 包。

### Q: 如何在生产环境使用？

A: 将 `frontAddr` 改为期货公司提供的生产行情前置地址，使用真实的账号密码。

## License

Apache License 2.0
