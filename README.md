# alpha-trade-gateway（Open Trade CTP SE15 Go）

一个以 **WebSocket** 形式对外提供交易能力的 CTP 交易网关（Td），并内置可切换的行情源（天勤 TQ WebSocket / CTP MdApi），支持登录、查询、下单/撤单、结算通知，以及可选的**条件单**能力。

本项目同时提供**模拟交易模块（TraderSim）**，可独立运行，用于开发测试和策略验证。

- 入口程序：[cmd/trader/main.go](cmd/trader/main.go)
- 模拟交易入口：[cmd/tradersim/main.go](cmd/tradersim/main.go)
- WebSocket 服务器：[`websocket.NewServer`](pkg/websocket/server.go)、[`websocket.Server.Start`](pkg/websocket/server.go)
- 配置加载：[`config.Load`](pkg/config/config.go)、[`config.LoadFromDir`](pkg/config/config.go)
- 行情统一接口：[`marketfeed.MarketClient`](pkg/marketfeed/interface.go)、[`marketfeed.NewMarketClientFromConfig`](pkg/marketfeed/interface.go)
- 条件单（可选）：[`condorder.Manager`](pkg/condorder/manager.go)、[`condorder.DefaultConfig`](pkg/condorder/types.go)

---

## 架构概览

启动顺序在入口中固定为：

1. 启动 MarketFeed（行情基础服务）：[`createMarketFeedClient`](cmd/trader/main.go) → [`marketfeed.NewMarketClientFromConfig`](pkg/marketfeed/interface.go)
2. 创建交易处理器（CTP Trader）：[`trader.New`](pkg/trader)（由 [cmd/trader/main.go](cmd/trader/main.go) 调用）
3. 启动 WebSocket 服务：[`websocket.NewServer`](pkg/websocket/server.go) → [`websocket.Server.Start`](pkg/websocket/server.go)

---

## 功能特性

- WebSocket 交易服务（默认 `0.0.0.0:7788`，见 [`config.setDefaults`](pkg/config/config.go)）
- 登录流程（含"次席"自定义 `broker_id/front` 支持）：[`TraderCTP.processReqLoginFull`](pkg/trader/login.go)
- 行情源可切换：
  - 天勤（`tq`）/ CTP 行情（`ctp`）：[`marketfeed.MarketClientType`](pkg/marketfeed/interface.go)
  - Quote 结构统一：[`marketfeed.Quote`](pkg/marketfeed/quote.go)
  - 行情回调：[`marketfeed.WithOnQuotes`](pkg/marketfeed/interface.go)
- 条件单（可配置启用）：
  - 初始化挂载：[`TraderCTP.initConditionOrderManager`](pkg/trader/trader.go)
  - 请求结构：[`condorder.ReqInsertConditionOrder`](pkg/condorder/types.go)、[`condorder.ReqCancelConditionOrder`](pkg/condorder/types.go)
- **模拟交易（TraderSim）**：
  - 独立运行的模拟交易服务
  - 支持期货合约下单/撤单/撮合
  - 持仓盈亏实时计算
  - 用户数据持久化与跨日结转

---

## 目录结构

```text
cmd/
  trader/                   # CTP 交易网关进程
  tradersim/                # 模拟交易服务进程
config/                     # 配置模板与 broker 列表
examples/                   # Go/Python 示例 + 协议说明
pkg/
  config/                   # 配置加载与 broker 列表
  websocket/                # WebSocket server/connection
  trader/                   # CTP Trader 实现（登录、下单、查询、持久化等）
  tradersim/                # 模拟交易实现（撮合引擎、持仓管理、数据持久化）
  protocol/                 # JSON 编解码与协议结构体
  marketfeed/               # 行情接口 + TQ/CTP 实现
  inslist/                  # 合约信息服务（从天勤获取合约列表）
  condorder/                # 条件单（可选）
```

---

## 模拟交易模块（TraderSim）

模拟交易模块是从 C++ `open-trade-sim` 移植而来的 Go 实现，提供完整的期货模拟交易功能。

### 功能特性

- **仅支持期货合约**（期权合约会被拒绝，错误码 410）
- **撮合引擎**：基于实时行情的限价单/市价单撮合
- **持仓管理**：支持上期所/能源中心的今昨仓分开平仓
- **资金管理**：保证金计算、冻结/解冻、盈亏计算
- **数据持久化**：用户数据 JSON 文件存储
- **跨日结转**：自动处理交易日切换时的持仓和资金结转

### 启动模拟交易服务

```sh
# 构建
go build -o bin/tradersim ./cmd/tradersim

# 运行
./bin/tradersim -host 0.0.0.0 -port 7799 -data ./data -broker simnow
```

命令行参数：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-host` | `0.0.0.0` | 监听地址 |
| `-port` | `7799` | 监听端口 |
| `-data` | `./data` | 用户数据存储目录 |
| `-broker` | `sim` | 经纪商 ID |
| `-log` | `info` | 日志级别 |
| `-market` | `tq` | 行情源类型（tq/ctp） |

### 模块结构

```text
pkg/tradersim/
  tradersim.go    # 核心结构、生命周期、消息处理、登录
  order.go        # 下单/撤单、撮合引擎、成交处理
  position.go     # 持仓管理、盈亏计算、数据推送
  persistence.go  # 数据持久化、跨日结转
```

### Notify 错误码

模拟交易使用与 C++ 版本一致的错误码：

| 错误码 | 说明 |
|--------|------|
| 400 | 重复发送登录请求 |
| 401 | 登录成功 |
| 406 | 成交通知 |
| 407 | 单号重复 |
| 409 | 合约不合法 |
| 410 | 模拟交易只支持期货合约 |
| 411 | 下单手数应该大于0 |
| 412 | 下单价格不是价格单位的整倍数 |
| 413 | 开仓保证金不足 |
| 414 | 平今手数超过今仓持仓量 |
| 415 | 平昨手数超过昨仓持仓量 |
| 416 | 平仓手数超过持仓量 |
| 417 | 下单成功 |
| 419 | 撤单成功 |
| 420 | 要撤销的单不存在 |
| 421 | 转账成功 |
| 422 | 转账失败 |

### 设计文档

详细设计请参考：[SIM_MODULE_DESIGN.md](SIM_MODULE_DESIGN.md)

---

## 合约信息服务（InsListService）

合约信息服务从天勤获取完整的合约列表，提供合约查询、交易所猜测等功能。

### 功能特性

- 从天勤 WebSocket 获取合约列表
- 支持按 symbol 查询合约信息
- 支持根据合约代码猜测交易所
- 区分期货/期权合约

### 使用示例

```go
import "alpha-trade-gateway/pkg/inslist"

// 创建服务
service := inslist.NewInstrumentService(marketClient)

// 初始化（从天勤加载合约列表）
if err := service.Init(); err != nil {
    log.Fatal(err)
}

// 查询合约
ins := service.GetInstrument("SHFE.au2406")
if ins != nil {
    fmt.Printf("合约乘数: %d\n", ins.VolumeMultiple)
    fmt.Printf("最小变动价位: %f\n", ins.PriceTick)
}

// 猜测交易所
exchange := inslist.GuessExchangeID("au2406") // 返回 "SHFE"
```

---

## tqsdk-python 对接（重点）

如果你希望在 Python 里继续使用天勤 `tqsdk` 的 API/生态，但把**交易通道**切到本项目的 WebSocket 网关，可以参考示例脚本：

- [examples/t41.py](examples/t41.py)

该示例的核心是把 `TqApi(..., _td_url="ws://127.0.0.1:7788")` 指向本项目网关地址。

### 运行步骤

1) 启动网关（确保 `host/port` 可访问；默认 `0.0.0.0:7788`）：

```sh
go run ./cmd/trader -config ./config/config.json
```

2) 安装 tqsdk（建议使用虚拟环境）：

```sh
python -m pip install tqsdk
```

3) 设置示例脚本所需环境变量并运行：

```sh
export SHINNYTECH_PW="<你的天勤密码>"
python examples/t41.py
```

### 关键参数说明

- `_td_url`：tqsdk 交易通道的地址，必须指向本网关，例如 `ws://127.0.0.1:7788`。

### 账号与凭证说明

示例脚本当前 `TqAccount("simnow", ...)` 账号写在代码里；脚本也预留了 `SIMNOW_USER_ID` / `SIMNOW_USER_PASSWD` 环境变量，但目前未使用。
如果你希望用环境变量驱动账号，请在 [examples/t41.py](examples/t41.py) 中把账号/密码替换为读取 `SIMNOW_USER_ID` / `SIMNOW_USER_PASSWD`。

---

## 快速开始

### 1) 准备配置

推荐从示例复制：

- 示例配置：[config/config.json.example](config/config.json.example)
- 示例 broker 列表：[config/broker_list.json.example](config/broker_list.json.example)

项目也提供一个可直接用于运行的配置样例（注意其中包含更偏开发/本地路径的配置项）：
- [cmd/trader/config.json](cmd/trader/config.json)
- [cmd/trader/broker_list.json](cmd/trader/broker_list.json)

> 配置读取逻辑：[`config.Load`](pkg/config/config.go)  
> broker 列表从 `broker_list_path` 加载：[`config.loadBrokerList`](pkg/config/config.go)

### 2) 启动网关

```sh
go run ./cmd/trader -config ./config/config.json
```

或构建后运行：

```sh
go build -o trader ./cmd/trader
./trader -config ./config/config.json
```

### 3) 测试连接（Python）

```sh
cd examples
pip install -r requirements.txt
python test_connection.py localhost 7788
```

脚本：[examples/test_connection.py](examples/test_connection.py)

---

## 配置说明（核心字段）

全局配置结构：[`config.Config`](pkg/config/config.go)

常用配置文件：

- JSON： [config/config.json](config/config.json)
- TOML 示例： [config/config.toml.example](config/config.toml.example)

### 行情源（marketfeed）

配置结构：[`config.MarketFeedConfig`](pkg/config/config.go)

- `marketfeed.type`: `"tq"` 或 `"ctp"`
- `marketfeed.symbols`: 默认订阅合约列表（例如：`"SHFE.ag2601"`）

创建逻辑位于入口：[`createMarketFeedClient`](cmd/trader/main.go)

---

## WebSocket 协议与示例

协议说明与请求示例集中在：

- [examples/README.md](examples/README.md)

常见请求 `aid`（以示例文档为准）：

- 登录：`req_login`（结构：[`protocol.ReqLogin`](pkg/protocol/types.go)）
- 下单：`insert_order`（结构：[`protocol.ActionInsertOrder`](pkg/protocol/types.go)）
- 撤单：`cancel_order`（结构：[`protocol.ActionCancelOrder`](pkg/protocol/types.go)）
- 主动拉取：`peek_message`

服务端 JSON 编解码：[`protocol.MarshalString`](pkg/protocol/json.go)、[`protocol.UnmarshalString`](pkg/protocol/json.go)  
通知类消息：[`protocol.BuildNotifyMsg`](pkg/protocol/json.go)、[`protocol.BuildSettlementNotifyMsg`](pkg/protocol/json.go)  
broker 列表推送：[`protocol.BuildBrokerListMsg`](pkg/protocol/json.go)

---

## 行情组件（MarketFeed）

行情库单独文档：

- [pkg/marketfeed/README.md](pkg/marketfeed/README.md)

示例：

- Go：CTP 行情订阅示例：[examples/ctpmarket/ctpmarket.go](examples/ctpmarket/ctpmarket.go)
- Go：统一接口切换行情源：[examples/marketinterface/marketinterface.go](examples/marketinterface/marketinterface.go)

---

## 条件单（Condition Order）

默认关闭（见 [`config.setDefaults`](pkg/config/config.go) 中 `condition_order.enabled`），启用后会在 Trader 初始化阶段加载/恢复并启动检测：

- 初始化：[`TraderCTP.initConditionOrderManager`](pkg/trader/trader.go)
- 配置结构：[`config.ConditionOrderConfig`](pkg/config/config.go)、默认值：[`condorder.DefaultConfig`](pkg/condorder/types.go)

---

## 备注

- 示例客户端：
  - 交互式 Python 客户端：[examples/python_client.py](examples/python_client.py)
  - 最简示例：[examples/simple_example.py](examples/simple_example.py)
