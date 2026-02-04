#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
简单示例 - 连接、登录、查询数据

安装: pip install websockets
运行: python simple_example.py
"""

import asyncio
import json
import websockets


async def trader_demo():
    """交易演示"""
    
    # 连接配置
    url = "ws://localhost:7788/"
    
    # 登录信息 (请修改为实际账号)
    # broker_id = "simnow_7x24"
    broker_id = "simnow"
    
    user_name = "046056"
    password = "123@simnow"
    
    print(f"正在连接 {url} ...")
    
    async with websockets.connect(url) as websocket:
        print("✓ 连接成功\n")
        
        # 1. 发送登录请求
        login_msg = {
            "aid": "req_login",
            "bid": broker_id,
            "user_name": user_name,
            "password": password
        }
        
        print(f"正在登录 {broker_id} / {user_name} ...")
        await websocket.send(json.dumps(login_msg))
        
        # 2. 接收消息
        while True:
            try:
                message = await asyncio.wait_for(websocket.recv(), timeout=30)
                msg = json.loads(message)
                
                aid = msg.get("aid", "")
                print(f"\n收到消息: {aid}")
                
                # 处理 rtn_data
                if aid == "rtn_data":
                    data_list = msg.get("data", [])
                    for data_item in data_list:
                        # 打印通知
                        if "notify" in data_item:
                            for notify_id, notify in data_item["notify"].items():
                                content = notify.get("content", "")
                                level = notify.get("level", "INFO")
                                print(f"  [{level}] {content}")
                        
                        # 打印交易数据
                        if "trade" in data_item:
                            for user_id, user_data in data_item["trade"].items():
                                # 账户
                                if "accounts" in user_data:
                                    for acc_id, acc in user_data["accounts"].items():
                                        print(f"\n  账户 {acc_id}:")
                                        print(f"    权益: {acc.get('balance', 0):.2f}")
                                        print(f"    可用: {acc.get('available', 0):.2f}")
                                        print(f"    保证金: {acc.get('margin', 0):.2f}")
                                
                                # 持仓
                                if "positions" in user_data:
                                    positions = user_data["positions"]
                                    if positions:
                                        print(f"\n  持仓 ({len(positions)} 个):")
                                        for symbol, pos in positions.items():
                                            long_vol = pos.get('volume_long', 0)
                                            short_vol = pos.get('volume_short', 0)
                                            if long_vol > 0 or short_vol > 0:
                                                print(f"    {symbol}: 多{long_vol} 空{short_vol}")
                                
                                # 订单
                                if "orders" in user_data:
                                    orders = user_data["orders"]
                                    if orders:
                                        print(f"\n  订单 ({len(orders)} 个)")
                                
                                # 成交
                                if "trades" in user_data:
                                    trades = user_data["trades"]
                                    if trades:
                                        print(f"\n  成交 ({len(trades)} 笔)")
                
                # 登录成功后可以发送其他请求
                # 例如查询数据
                # await websocket.send(json.dumps({"aid": "peek_message"}))
                
                # 例如下单
                # order_msg = {
                #     "aid": "insert_order",
                #     "order_id": "test_order_1",
                #     "exchange_id": "SHFE",
                #     "instrument_id": "rb2505",
                #     "direction": 1,  # 买
                #     "offset": 1,     # 开仓
                #     "volume": 1,
                #     "price_type": 1, # 限价
                #     "limit_price": 3500.0
                # }
                # await websocket.send(json.dumps(order_msg))
                
            except asyncio.TimeoutError:
                print("接收超时")
                break
            except KeyboardInterrupt:
                print("\n用户中断")
                break


if __name__ == "__main__":
    try:
        asyncio.run(trader_demo())
    except KeyboardInterrupt:
        print("\n退出")
