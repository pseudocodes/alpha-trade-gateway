#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Open Trade CTP SE15 Go - Python WebSocket 客户端示例

使用 websockets 库连接交易网关，支持登录、查询、下单、撤单等功能

安装依赖：
    pip install websockets

使用示例：
    python python_client.py
"""

import asyncio
import json
import sys
from datetime import datetime
from typing import Optional, Dict, Any

try:
    import websockets
except ImportError:
    print("错误: 未安装 websockets 库")
    print("请运行: pip install websockets")
    sys.exit(1)


class TraderClient:
    """交易客户端"""
    
    def __init__(self, host: str = "localhost", port: int = 7788):
        self.url = f"ws://{host}:{port}/"
        self.websocket: Optional[websockets.WebSocketClientProtocol] = None
        self.running = False
        self.user_data = {}
        self.logged_in = False
        
    async def connect(self):
        """连接到交易网关"""
        print(f"正在连接到 {self.url} ...")
        try:
            self.websocket = await websockets.connect(
                self.url,
                ping_interval=20,
                ping_timeout=10,
                close_timeout=5
            )
            print("✓ 连接成功")
            self.running = True
            return True
        except Exception as e:
            print(f"✗ 连接失败: {e}")
            return False
    
    async def disconnect(self):
        """断开连接"""
        self.running = False
        if self.websocket:
            await self.websocket.close()
            print("已断开连接")
    
    async def send_message(self, msg: Dict[str, Any]):
        """发送消息"""
        if not self.websocket:
            print("错误: 未连接")
            return
        
        msg_str = json.dumps(msg, ensure_ascii=False)
        await self.websocket.send(msg_str)
        print(f"→ 发送: {msg['aid']}")
    
    async def login(self, broker_id: str, user_name: str, password: str):
        """登录"""
        print(f"\n正在登录 {broker_id} / {user_name} ...")
        
        login_msg = {
            "aid": "req_login",
            "bid": broker_id,
            "user_name": user_name,
            "password": password
        }
        
        await self.send_message(login_msg)
    
    async def peek_message(self):
        """主动查询消息"""
        await self.send_message({"aid": "peek_message"})
    
    async def confirm_settlement(self):
        """确认结算单"""
        print("\n确认结算单...")
        await self.send_message({"aid": "confirm_settlement"})
    
    async def query_settlement_info(self, trading_day: Optional[int] = None):
        """查询结算单"""
        msg = {"aid": "qry_settlement_info"}
        if trading_day:
            msg["trading_day"] = trading_day
        await self.send_message(msg)
    
    async def insert_order(
        self,
        exchange_id: str,
        instrument_id: str,
        direction: int,  # 1=买, -1=卖
        offset: int,     # 1=开仓, -1=平仓, -2=平今
        volume: int,
        price_type: int = 1,  # 1=限价, 2=市价, 3=最优价
        limit_price: float = 0.0,
        time_condition: int = 3,  # 1=IOC, 3=GFD, 5=GTC
        volume_condition: int = 1,  # 1=任意, 2=最小, 3=全部
        hedge_flag: int = 1,  # 1=投机, 3=套保
    ):
        """下单
        
        Args:
            exchange_id: 交易所代码 (SHFE, DCE, CZCE, CFFEX, INE)
            instrument_id: 合约代码 (如 rb2505)
            direction: 方向 (1=买, -1=卖)
            offset: 开平 (1=开仓, -1=平仓, -2=平今)
            volume: 手数
            price_type: 价格类型 (1=限价, 2=市价, 3=最优价)
            limit_price: 限价价格
            time_condition: 有效期 (1=IOC, 3=GFD当日有效, 5=GTC)
            volume_condition: 成交量条件 (1=任意, 2=最小, 3=全部)
            hedge_flag: 投机套保 (1=投机, 3=套保)
        """
        order_id = f"order_{int(datetime.now().timestamp() * 1000)}"
        
        msg = {
            "aid": "insert_order",
            "order_id": order_id,
            "exchange_id": exchange_id,
            "instrument_id": instrument_id,
            "direction": direction,
            "offset": offset,
            "volume": volume,
            "price_type": price_type,
            "limit_price": limit_price,
            "time_condition": time_condition,
            "volume_condition": volume_condition,
            "hedge_flag": hedge_flag,
        }
        
        direction_str = "买" if direction == 1 else "卖"
        offset_str = {1: "开仓", -1: "平仓", -2: "平今"}.get(offset, "未知")
        
        print(f"\n下单: {exchange_id}.{instrument_id} {direction_str}{offset_str} {volume}手 @ {limit_price}")
        await self.send_message(msg)
        return order_id
    
    async def cancel_order(self, order_id: str):
        """撤单"""
        print(f"\n撤单: {order_id}")
        msg = {
            "aid": "cancel_order",
            "order_id": order_id
        }
        await self.send_message(msg)
    
    async def change_password(self, old_password: str, new_password: str):
        """修改密码"""
        print("\n修改密码...")
        msg = {
            "aid": "change_password",
            "old_password": old_password,
            "new_password": new_password
        }
        await self.send_message(msg)
    
    async def transfer(
        self,
        bank_id: str,
        amount: float,
        direction: str = "BANK_TO_FUTURE"  # BANK_TO_FUTURE 或 FUTURE_TO_BANK
    ):
        """银期转账"""
        print(f"\n银期转账: {direction} {amount}元")
        msg = {
            "aid": "req_transfer",
            "bank_id": bank_id,
            "amount": amount,
            "direction": direction
        }
        await self.send_message(msg)
    
    # ==================== 新增接口 ====================
    
    async def query_account_info(self):
        """查询资金账户信息
        对应 aid: qry_account_info
        """
        print("\n查询资金账户信息...")
        await self.send_message({"aid": "qry_account_info"})
    
    async def query_transfer_serial(self):
        """查询转账流水
        对应 aid: qry_transfer_serial
        """
        print("\n查询转账流水...")
        await self.send_message({"aid": "qry_transfer_serial"})
    
    async def query_account_register(self):
        """查询银期签约关系
        对应 aid: qry_account_register
        """
        print("\n查询银期签约关系...")
        await self.send_message({"aid": "qry_account_register"})
    
    async def change_trading_account_password(self, old_password: str, new_password: str):
        """修改资金密码
        对应 aid: change_trading_account_password
        """
        print("\n修改资金密码...")
        msg = {
            "aid": "change_trading_account_password",
            "old_password": old_password,
            "new_password": new_password
        }
        await self.send_message(msg)
    
    def process_message(self, msg: Dict[str, Any]):
        """处理服务器消息"""
        aid = msg.get("aid", "")
        
        if aid == "rtn_data":
            self._process_rtn_data(msg)
        elif aid == "rtn_brokers":
            self._process_brokers(msg)
        else:
            print(f"← 收到消息: {aid}")
    
    def _process_rtn_data(self, msg: Dict[str, Any]):
        """处理 rtn_data 消息"""
        data_list = msg.get("data", [])
        
        for data_item in data_list:
            # 处理通知
            if "notify" in data_item:
                self._process_notify(data_item["notify"])
            
            # 处理交易数据
            if "trade" in data_item:
                self._process_trade_data(data_item["trade"])
    
    def _process_notify(self, notify_dict: Dict[str, Any]):
        """处理通知消息"""
        for notify_id, notify in notify_dict.items():
            code = notify.get("code", 0)
            content = notify.get("content", "")
            level = notify.get("level", "INFO")
            
            # 根据 level 显示不同颜色（可选）
            if level == "ERROR":
                prefix = "✗ 错误"
            elif level == "WARNING":
                prefix = "⚠ 警告"
            else:
                prefix = "✓ 提示"
            
            print(f"\n{prefix}: [{code}] {content}")
            
            # 检查是否登录成功
            if code == 0 and "登录成功" in content:
                self.logged_in = True
    
    def _process_trade_data(self, trade_dict: Dict[str, Any]):
        """处理交易数据"""
        for user_id, user_data in trade_dict.items():
            # 更新本地数据
            if user_id not in self.user_data:
                self.user_data[user_id] = {}
            
            # 处理结算单
            if "settlement" in user_data:
                print("\n========== 结算单 ==========")
                print(user_data["settlement"])
                print("===========================\n")
            
            # 处理账户信息
            if "accounts" in user_data:
                self._process_accounts(user_data["accounts"])
            
            # 处理持仓
            if "positions" in user_data:
                self._process_positions(user_data["positions"])
            
            # 处理订单
            if "orders" in user_data:
                self._process_orders(user_data["orders"])
            
            # 处理成交
            if "trades" in user_data:
                self._process_trades(user_data["trades"])
            
            # 处理银行
            if "banks" in user_data:
                self._process_banks(user_data["banks"])
    
    def _process_accounts(self, accounts: Dict[str, Any]):
        """处理账户信息"""
        for account_id, account in accounts.items():
            print(f"\n========== 账户: {account_id} ==========")
            print(f"币种: {account.get('currency', 'CNY')}")
            print(f"昨权益: {account.get('pre_balance', 0):.2f}")
            print(f"动态权益: {account.get('balance', 0):.2f}")
            print(f"可用资金: {account.get('available', 0):.2f}")
            print(f"保证金: {account.get('margin', 0):.2f}")
            print(f"平仓盈亏: {account.get('close_profit', 0):.2f}")
            print(f"持仓盈亏: {account.get('position_profit', 0):.2f}")
            print(f"手续费: {account.get('commission', 0):.2f}")
            print(f"风险度: {account.get('risk_ratio', 0):.2%}")
            print("=" * 40)
    
    def _process_positions(self, positions: Dict[str, Any]):
        """处理持仓信息"""
        if not positions:
            return
        
        print("\n========== 持仓 ==========")
        for symbol, pos in positions.items():
            long_vol = pos.get('volume_long', 0)
            short_vol = pos.get('volume_short', 0)
            
            if long_vol > 0:
                print(f"{symbol} 多头 {long_vol}手 "
                      f"成本: {pos.get('open_price_long', 0):.2f} "
                      f"现价: {pos.get('last_price', 0):.2f} "
                      f"盈亏: {pos.get('float_profit_long', 0):.2f}")
            
            if short_vol > 0:
                print(f"{symbol} 空头 {short_vol}手 "
                      f"成本: {pos.get('open_price_short', 0):.2f} "
                      f"现价: {pos.get('last_price', 0):.2f} "
                      f"盈亏: {pos.get('float_profit_short', 0):.2f}")
        print("=" * 40)
    
    def _process_orders(self, orders: Dict[str, Any]):
        """处理订单信息"""
        if not orders:
            return
        
        # 只显示新订单或状态变化的订单
        print("\n========== 订单 ==========")
        for order_id, order in orders.items():
            if not order.get('changed'):
                continue
            
            direction = "买" if order.get('direction', 0) == 1 else "卖"
            offset_map = {1: "开", -1: "平", -2: "平今"}
            offset = offset_map.get(order.get('offset', 0), "?")
            status = "未完成" if order.get('status', 0) == 1 else "已完成"
            
            print(f"{order_id}: {order.get('instrument_id', '')} "
                  f"{direction}{offset} "
                  f"{order.get('volume_orign', 0)}手 "
                  f"@ {order.get('limit_price', 0):.2f} "
                  f"[{status}] 剩余: {order.get('volume_left', 0)}手")
        print("=" * 40)
    
    def _process_trades(self, trades: Dict[str, Any]):
        """处理成交信息"""
        if not trades:
            return
        
        # 只显示新成交
        for trade_id, trade in trades.items():
            if not trade.get('changed'):
                continue
            
            direction = "买" if trade.get('direction', 0) == 1 else "卖"
            offset_map = {1: "开", -1: "平", -2: "平今"}
            offset = offset_map.get(trade.get('offset', 0), "?")
            
            print(f"\n✓ 成交: {trade.get('instrument_id', '')} "
                  f"{direction}{offset} "
                  f"{trade.get('volume', 0)}手 "
                  f"@ {trade.get('price', 0):.2f}")
    
    def _process_banks(self, banks: Dict[str, Any]):
        """处理银行信息"""
        if banks:
            print("\n========== 可用银行 ==========")
            for bank_id, bank in banks.items():
                print(f"{bank_id}: {bank.get('bank_name', '')}")
            print("=" * 40)
    
    def _process_brokers(self, msg: Dict[str, Any]):
        """处理经纪商列表"""
        brokers = msg.get("brokers", [])
        if brokers:
            print("\n可用经纪商:")
            for broker in brokers:
                print(f"  - {broker}")
    
    async def message_loop(self):
        """消息接收循环"""
        if not self.websocket:
            return
        
        try:
            async for message in self.websocket:
                try:
                    msg = json.loads(message)
                    self.process_message(msg)
                except json.JSONDecodeError as e:
                    print(f"JSON 解析错误: {e}")
                except Exception as e:
                    print(f"处理消息出错: {e}")
        except websockets.exceptions.ConnectionClosed:
            print("连接已关闭")
            self.running = False
        except Exception as e:
            print(f"接收消息出错: {e}")
            self.running = False
    
    async def run(self, broker_id: str, user_name: str, password: str):
        """运行客户端"""
        # 连接
        if not await self.connect():
            return
        
        try:
            # 启动消息接收循环
            receive_task = asyncio.create_task(self.message_loop())
            
            # 等待连接稳定
            await asyncio.sleep(0.5)
            
            # 登录
            await self.login(broker_id, user_name, password)
            
            # 等待登录完成
            for _ in range(50):  # 最多等待 5 秒
                if self.logged_in:
                    break
                await asyncio.sleep(0.1)
            
            if not self.logged_in:
                print("登录超时")
                return
            
            # 进入交互模式
            await self.interactive_mode()
            
            # 等待接收任务完成
            await receive_task
            
        except KeyboardInterrupt:
            print("\n用户中断")
        finally:
            await self.disconnect()
    
    async def interactive_mode(self):
        """交互模式"""
        print("\n========== 交互模式 ==========")
        print("可用命令:")
        print("  q - 查询数据 (peek_message)")
        print("  a - 查询账户信息 (qry_account_info)")
        print("  r - 查询转账流水 (qry_transfer_serial)")
        print("  k - 查询银期签约 (qry_account_register)")
        print("  b - 买开仓")
        print("  s - 卖开仓")
        print("  c - 平仓")
        print("  x - 撤单")
        print("  p - 修改登录密码 (change_password)")
        print("  m - 修改资金密码 (change_trading_account_password)")
        print("  t - 银期转账")
        print("  h - 查询历史结算单")
        print("  f - 确认结算单")
        print("  exit - 退出")
        print("=" * 40)
        
        # 启动一个任务来读取用户输入
        while self.running:
            try:
                # 使用 run_in_executor 来异步读取输入
                loop = asyncio.get_event_loop()
                cmd = await loop.run_in_executor(None, input, "\n命令> ")
                
                if not cmd:
                    continue
                
                cmd = cmd.strip().lower()
                
                if cmd == 'exit' or cmd == 'quit':
                    print("退出中...")
                    break
                elif cmd == 'q':
                    await self.peek_message()
                elif cmd == 'a':
                    await self.query_account_info()
                elif cmd == 'r':
                    await self.query_transfer_serial()
                elif cmd == 'k':
                    await self.query_account_register()
                elif cmd == 'b':
                    # 买开仓示例
                    symbol = input("合约代码 (如 rb2505): ").strip()
                    volume = int(input("手数: ").strip() or "1")
                    price = float(input("价格: ").strip())
                    await self.insert_order("SHFE", symbol, 1, 1, volume, 1, price)
                elif cmd == 's':
                    # 卖开仓示例
                    symbol = input("合约代码 (如 rb2505): ").strip()
                    volume = int(input("手数: ").strip() or "1")
                    price = float(input("价格: ").strip())
                    await self.insert_order("SHFE", symbol, -1, 1, volume, 1, price)
                elif cmd == 'c':
                    # 平仓示例
                    symbol = input("合约代码: ").strip()
                    direction_input = input("方向 (买平/卖平): ").strip()
                    direction = 1 if '买' in direction_input else -1
                    volume = int(input("手数: ").strip() or "1")
                    price = float(input("价格: ").strip())
                    await self.insert_order("SHFE", symbol, direction, -1, volume, 1, price)
                elif cmd == 'x':
                    order_id = input("订单ID: ").strip()
                    await self.cancel_order(order_id)
                elif cmd == 'p':
                    old_pwd = input("旧密码: ").strip()
                    new_pwd = input("新密码: ").strip()
                    await self.change_password(old_pwd, new_pwd)
                elif cmd == 'm':
                    old_pwd = input("旧资金密码: ").strip()
                    new_pwd = input("新资金密码: ").strip()
                    await self.change_trading_account_password(old_pwd, new_pwd)
                elif cmd == 't':
                    bank_id = input("银行代码: ").strip()
                    amount = float(input("金额: ").strip())
                    direction = input("方向 (入金/出金): ").strip()
                    transfer_dir = "BANK_TO_FUTURE" if '入' in direction else "FUTURE_TO_BANK"
                    await self.transfer(bank_id, amount, transfer_dir)
                elif cmd == 'h':
                    trading_day = input("交易日 (留空查当日): ").strip()
                    if trading_day:
                        await self.query_settlement_info(int(trading_day))
                    else:
                        await self.query_settlement_info()
                elif cmd == 'f':
                    await self.confirm_settlement()
                else:
                    print(f"未知命令: {cmd}")
                    
            except EOFError:
                break
            except KeyboardInterrupt:
                print("\n退出中...")
                break
            except Exception as e:
                print(f"命令执行出错: {e}")
                continue


async def main():
    """主函数"""
    print("=" * 50)
    print("  Open Trade CTP SE15 Go - Python 客户端")
    print("=" * 50)
    
    # 配置参数
    HOST = "localhost"
    PORT = 7788
    
    # 登录信息 (需要修改为实际账号)
    BROKER_ID = "simnow"       # 经纪商ID (如 simnow)
    USER_NAME = "123456"       # 用户名
    PASSWORD = "your_password" # 密码
    
    # 检查是否需要从命令行读取
    if len(sys.argv) > 1:
        BROKER_ID = sys.argv[1]
    if len(sys.argv) > 2:
        USER_NAME = sys.argv[2]
    if len(sys.argv) > 3:
        PASSWORD = sys.argv[3]
    
    # 如果密码是默认值，从输入读取
    if PASSWORD == "your_password":
        print("\n请输入登录信息:")
        BROKER_ID = input(f"经纪商ID [{BROKER_ID}]: ").strip() or BROKER_ID
        USER_NAME = input(f"用户名 [{USER_NAME}]: ").strip() or USER_NAME
        
        # 尝试隐藏密码输入
        try:
            import getpass
            PASSWORD = getpass.getpass("密码: ")
        except:
            PASSWORD = input("密码: ").strip()
    
    # 创建客户端并运行
    client = TraderClient(HOST, PORT)
    await client.run(BROKER_ID, USER_NAME, PASSWORD)


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        print("\n程序退出")
    except Exception as e:
        print(f"程序异常: {e}")
        import traceback
        traceback.print_exc()
