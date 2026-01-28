# alpha-trade-gateway

基于 CTP 的交易网关（Go 实现），通过 WebSocket 对外提供登录、查询、下单/撤单、结算单、银期转账等交易能力；并内置可切换的行情服务（天勤 WebSocket / CTP 原生 MdApi）。

- 入口程序：[/cmd/trader/main.go](cmd/trader/main.go)
- WebSocket 服务：[/pkg/websocket/server.go](pkg/websocket/server.go)
- 配置定义：[/pkg/config/config.go](pkg/config/config.go)
- 协议/JSON 工具：[/pkg/protocol](pkg/protocol)
- 示例（含协议说明）：[/examples/README.md](examples/README.md)
- 行情组件说明：[/pkg/marketfeed/README.md](pkg/marketfeed/README.md)

## 功能

- WebSocket JSON 协议（通过 `aid` 字段区分请求类型）
- CTP 交易：认证、登录、查询、下单、撤单、结算单确认/查询、转账等
- 行情服务（`marketfeed.type` 选择数据源）：
  - `tq`：天勤 WebSocket 行情
  - `ctp`：CTP 原生行情（go2ctp MdApi）
- 条件单（可配置启用）：`condition_order.*`

## 目录结构

```text
cmd/trader/            # 主程序（启动 MarketFeed + Trader + WebSocket）
config/                # 配置示例、broker 列表
examples/              # Go/Python 示例（含协议说明）
pkg/                   # 核心实现：config / websocket / trader / marketfeed / protocol / condorder
```

## 环境要求

- Go：`go.mod` 指定 `go 1.23`
- go2ctp：本仓库通过 `replace github.com/pseudocodes/go2ctp => ../go2ctp` 引用
  - 如果你不是以“同级目录”方式放置 `../go2ctp`，需要：
    - 把 `../go2ctp` 放到本仓库同级；或
    - 修改 [/go.mod](go.mod) 的 `replace` 为你的实际路径；或
    - 去掉 `replace` 并使用可拉取的 go2ctp 版本（若你有对应 tag/commit）。
- 若使用 CTP 原生行情/交易：需要具备对应 CTP API 动态库与运行环境（不同平台/券商版本不同）。

## 快速开始

### 1) 准备配置

推荐从示例复制：

- JSON 示例：[/config/config.example.json](config/config.example.json)
- TOML 示例：[/config/config.toml.example](config/config.toml.example)
- broker 列表：[/config/broker_list.json.example](config/broker_list.json.example)

最小化起步（使用天勤行情 `tq`，不依赖 CTP 行情前置）：

```bash
cp config/config.example.json config/config.json
# 如需 broker 列表
cp config/broker_list.json.example config/broker_list.json
```

说明：
- 若未在配置中显式指定 `broker_list_path`，程序会默认在“配置文件所在目录”寻找 `broker_list.json`（见 [/pkg/config/config.go](pkg/config/config.go)）。

### 2) 启动网关

```bash
go run ./cmd/trader -config ./config/config.json
```

或构建后运行：

```bash
go build -o trader ./cmd/trader
./trader -config ./config/config.json
```

默认监听地址由配置决定（默认值：`host=0.0.0.0`、`port=7788`，见 [/pkg/config/config.go](pkg/config/config.go)）。

WebSocket 地址：

- `ws://<host>:<port>/`

### 3) 测试连接（Python）

```bash
cd examples
pip install -r requirements.txt
python test_connection.py localhost 7788
```

脚本：[/examples/test_connection.py](examples/test_connection.py)

## 配置字段速览

完整结构见 [/pkg/config/config.go](pkg/config/config.go)。以下列出最常用字段：

- `host` / `port`：WebSocket 监听地址
- `auto_confirm_settlement`：是否自动确认结算单
- `log.*`：日志配置
- `ctp.*`：CTP 相关（流文件路径、是否使用动态库等）
- `marketfeed.*`：行情服务
  - `marketfeed.type`：`tq` 或 `ctp`
  - `marketfeed.symbols`：启动后默认订阅合约列表
  - `marketfeed.tq.url` / `marketfeed.tq.token`
  - `marketfeed.ctp.front_addr` / `broker_id` / `user_id` / `password` / `flow_path`
- `condition_order.*`：条件单开关与数据存储限制

## 示例与协议

- 协议说明与 `aid` 列表：[/examples/README.md](examples/README.md)
- Python 交互式客户端：[/examples/python_client.py](examples/python_client.py)
- Go 行情示例：[/examples/ctpmarket/ctpmarket.go](examples/ctpmarket/ctpmarket.go)
- Go 行情统一接口示例：[/examples/marketinterface/marketinterface.go](examples/marketinterface/marketinterface.go)

服务端内部消息类型常量集中在：[/pkg/trader/message.go](pkg/trader/message.go)

## 行情组件（MarketFeed）

行情统一接口与数据源选择见：[/pkg/marketfeed/README.md](pkg/marketfeed/README.md)

## 常见问题

- 端口未监听：确认配置 `host/port`，以及进程是否启动
- 构建失败（go2ctp）：确认 `../go2ctp` 路径存在，或调整 [/go.mod](go.mod) 的 `replace`
- CTP 相关报错：确认 CTP 前置、账号权限、以及本机 CTP API 运行依赖
