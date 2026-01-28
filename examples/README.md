# 示例代码

这里提供了 Go 和 Python 示例代码。

## 目录结构

```
examples/
├── ctpmarket/              # Go: CTP 行情客户端示例
│   └── ctpmarket.go
├── marketinterface/        # Go: MarketClient 统一接口示例
│   └── marketinterface.go
├── python_client.py        # Python: 完整交易客户端
├── simple_example.py       # Python: 简单示例
└── test_connection.py      # Python: 连接测试
```

---

## Go 示例

### 1. ctpmarket - CTP 行情客户端

使用 CTP MdApi 订阅期货行情。

**运行**:
```bash
cd examples/ctpmarket
go run ctpmarket.go
```

**功能**:
- 连接 CTP 行情前置
- 登录认证
- 订阅合约行情
- 实时接收行情数据
- 行情数据缓存

### 2. marketinterface - 统一接口示例

展示 `MarketClient` 统一接口的使用，支持切换天勤/CTP 数据源。

**运行**:
```bash
cd examples/marketinterface

# 使用天勤数据源
go run marketinterface.go -source=tq

# 使用 CTP 数据源
go run marketinterface.go -source=ctp

# 自定义参数
go run marketinterface.go -source=ctp -symbols="ag2601,au2604" -front="tcp://182.254.243.31:40011"
```

**命令行参数**:
- `-source`: 数据源类型 (`tq` 或 `ctp`)
- `-symbols`: 订阅合约列表，逗号分隔
- `-broker`: CTP BrokerID
- `-user`: CTP UserID
- `-pass`: CTP Password
- `-front`: CTP 行情前置地址
- `-tqurl`: 天勤 WebSocket 地址

---

## Python 示例

### 安装依赖

```bash
pip install websockets
```

### 1. simple_example.py - 简单示例

最基础的连接、登录和接收数据示例，适合快速入门。

**运行**:
```bash
python simple_example.py
```

**功能**:
- WebSocket 连接
- 登录认证
- 接收并打印服务器消息
- 显示账户、持仓等基本信息

### 2. python_client.py - 完整客户端

功能完整的交易客户端，支持交互式操作。

**运行**:
```bash
# 直接运行（会提示输入登录信息）
python python_client.py

# 或者通过命令行参数
python python_client.py simnow 123456 your_password
```

**功能**:
- ✅ 登录认证
- ✅ 查询账户、持仓、订单、成交
- ✅ 下单（限价单、市价单）
- ✅ 撤单
- ✅ 银期转账
- ✅ 修改密码
- ✅ 实时推送数据
- ✅ 交互式命令行界面

**交互命令**:
- `q` - 查询数据
- `b` - 买开仓
- `s` - 卖开仓
- `c` - 平仓
- `x` - 撤单
- `p` - 修改密码
- `t` - 银期转账
- `exit` - 退出

## 使用示例

### 基本连接和登录

```python
import asyncio
import json
import websockets

async def main():
    url = "ws://localhost:7788/"
    
    async with websockets.connect(url) as ws:
        # 登录
        login_msg = {
            "aid": "req_login",
            "bid": "simnow",
            "user_name": "123456",
            "password": "your_password"
        }
        await ws.send(json.dumps(login_msg))
        
        # 接收消息
        message = await ws.recv()
        print(json.loads(message))

asyncio.run(main())
```

### 下单示例

```python
# 限价买开仓
order_msg = {
    "aid": "insert_order",
    "order_id": "my_order_1",
    "exchange_id": "SHFE",      # 交易所
    "instrument_id": "rb2505",  # 合约代码
    "direction": 1,             # 1=买, -1=卖
    "offset": 1,                # 1=开仓, -1=平仓, -2=平今
    "volume": 1,                # 手数
    "price_type": 1,            # 1=限价, 2=市价
    "limit_price": 3500.0       # 价格
}
await websocket.send(json.dumps(order_msg))
```

### 撤单示例

```python
cancel_msg = {
    "aid": "cancel_order",
    "order_id": "my_order_1"
}
await websocket.send(json.dumps(cancel_msg))
```

### 查询数据

```python
# 主动查询
peek_msg = {"aid": "peek_message"}
await websocket.send(json.dumps(peek_msg))
```

## 消息协议

### 请求消息格式

所有请求消息都是 JSON 格式，包含 `aid` 字段表示操作类型。

#### 登录 (req_login)
```json
{
  "aid": "req_login",
  "bid": "simnow",
  "user_name": "123456",
  "password": "your_password"
}
```

#### 下单 (insert_order)
```json
{
  "aid": "insert_order",
  "order_id": "order_001",
  "exchange_id": "SHFE",
  "instrument_id": "rb2505",
  "direction": 1,
  "offset": 1,
  "volume": 1,
  "price_type": 1,
  "limit_price": 3500.0,
  "time_condition": 3,
  "volume_condition": 1,
  "hedge_flag": 1
}
```

#### 撤单 (cancel_order)
```json
{
  "aid": "cancel_order",
  "order_id": "order_001"
}
```

#### 查询数据 (peek_message)
```json
{
  "aid": "peek_message"
}
```

#### 确认结算单 (confirm_settlement)
```json
{
  "aid": "confirm_settlement"
}
```

### 响应消息格式

服务器返回的消息主要是 `rtn_data` 类型：

```json
{
  "aid": "rtn_data",
  "data": [
    {
      "notify": {
        "N0": {
          "type": 1,
          "code": 0,
          "content": "登录成功",
          "level": "INFO"
        }
      },
      "trade": {
        "123456": {
          "user_id": "123456",
          "trading_day": "20260109",
          "accounts": {
            "123456": {
              "balance": 100000.0,
              "available": 95000.0,
              "margin": 5000.0,
              "position_profit": 0.0
            }
          },
          "positions": {
            "SHFE.rb2505": {
              "volume_long": 1,
              "volume_short": 0,
              "last_price": 3510.0,
              "float_profit": 100.0
            }
          },
          "orders": {},
          "trades": {}
        }
      }
    }
  ]
}
```

## 字段说明

### 方向 (direction)
- `1` - 买
- `-1` - 卖

### 开平标志 (offset)
- `1` - 开仓
- `-1` - 平仓
- `-2` - 平今

### 价格类型 (price_type)
- `1` - 限价单
- `2` - 市价单（任意价）
- `3` - 最优价

### 有效期类型 (time_condition)
- `1` - IOC (立即成交否则撤销)
- `3` - GFD (当日有效)
- `5` - GTC (取消前有效)

### 成交量条件 (volume_condition)
- `1` - 任意数量
- `2` - 最小数量
- `3` - 全部数量

### 投机套保标志 (hedge_flag)
- `1` - 投机
- `3` - 套保

### 订单状态 (status)
- `1` - 未完成（活动）
- `2` - 已完成（终结）

## 交易所代码

- `SHFE` - 上海期货交易所
- `DCE` - 大连商品交易所
- `CZCE` - 郑州商品交易所
- `CFFEX` - 中国金融期货交易所
- `INE` - 上海国际能源交易中心

## 注意事项

1. **修改登录信息**: 运行前请修改代码中的 `broker_id`, `user_name`, `password`
2. **网关地址**: 默认连接 `localhost:7788`，如需修改请调整 `HOST` 和 `PORT`
3. **Simnow 账号**: 可以在 http://www.simnow.com.cn/ 申请仿真账号测试
4. **异步编程**: 示例使用 `asyncio`，所有 I/O 操作都是异步的
5. **错误处理**: 生产环境请添加完善的错误处理和重连机制

## 进阶使用

### 添加日志

```python
import logging

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)

logger = logging.getLogger(__name__)
logger.info("客户端启动")
```

### 添加重连机制

```python
async def connect_with_retry(url, max_retries=5):
    for i in range(max_retries):
        try:
            return await websockets.connect(url)
        except Exception as e:
            print(f"连接失败 ({i+1}/{max_retries}): {e}")
            if i < max_retries - 1:
                await asyncio.sleep(2)
    raise Exception("超过最大重试次数")
```

### 心跳保持

```python
async def heartbeat(websocket, interval=20):
    """定期发送心跳"""
    while True:
        try:
            await asyncio.sleep(interval)
            pong = await websocket.ping()
            await pong
        except:
            break
```

## 常见问题

### Q: 连接被拒绝
A: 检查交易网关是否已启动，端口是否正确（默认 7788）

### Q: 登录失败
A: 检查经纪商 ID、用户名、密码是否正确，或查看服务器日志

### Q: 收不到数据
A: 确认登录成功后，可以主动发送 `peek_message` 查询数据

### Q: 下单失败
A: 检查合约代码、交易所代码是否正确，账户是否有足够资金

## 更多示例

更多示例和文档请参考:
- [项目文档](../ARCHITECTURE.md)
- [Go 版本源码](../pkg/)
- [天勤协议文档](https://doc.shinnytech.com/tqsdk/latest/reference/tq_protocol.html)

## License

与主项目保持一致
